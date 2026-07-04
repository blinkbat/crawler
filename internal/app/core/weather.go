package core

import "math/rand"

// WeatherPhase is the ambient-rain state machine's stage (outdoors-only, see
// AreaIsOutdoor). Cycle: Clear -> Building -> Raining -> Clearing -> Clear.
type WeatherPhase int

const (
	WeatherClear    WeatherPhase = iota // dry; Cooldown counts down before rain may roll again
	WeatherBuilding                     // tint ramping in before the rain falls
	WeatherRaining                      // tint held + rain falling; RainStepsLeft counts down
	WeatherClearing                     // rain stopped, tint ramping back out
)

// WeatherMode is an authored per-area override of the ambient-rain system.
// Serialized in the map header (weather:); zero value Auto keeps the legacy
// roof-gated behavior, so pre-weather maps read unchanged.
type WeatherMode int

const (
	WeatherModeAuto  WeatherMode = iota // roof-gated: outdoors may storm, indoors never (default)
	WeatherModeClear                    // never rains, even outdoors
	WeatherModeRain                     // always storming, even under a roof
	weatherModeCount                    // coverage sentinel (init-asserts weatherModeDefs)
)

// weatherModeDef is one weather-mode registry row: enum value, on-disk token
// ("" for Auto, omitted from maps), and editor-facing label. One row per mode,
// mirroring the material/facing registries — name/label/options all derive here.
type weatherModeDef struct {
	value WeatherMode
	name  string // on-disk token; MUST be lowercase (decodeByName case-folds)
	label string
}

var weatherModeDefs = []weatherModeDef{
	{WeatherModeAuto, "", "Auto"},
	{WeatherModeClear, "clear", "Clear"},
	{WeatherModeRain, "rain", "Rain"},
}

func init() {
	if len(weatherModeDefs) != int(weatherModeCount) {
		panic("core: weatherModeDefs must have one row per WeatherMode — add a row when adding a mode")
	}
}

func findWeatherModeDef(m WeatherMode) (weatherModeDef, bool) {
	return findByValue(weatherModeDefs, m, func(d weatherModeDef) WeatherMode { return d.value })
}

// WeatherModeName is the on-disk token for a mode ("" for Auto / out of range).
func WeatherModeName(m WeatherMode) string {
	d, _ := findWeatherModeDef(m)
	return d.name
}

// WeatherModeLabel is the editor-facing label (Auto shown explicitly; unknown → "Auto").
func WeatherModeLabel(m WeatherMode) string {
	if d, ok := findWeatherModeDef(m); ok {
		return d.label
	}
	return "Auto"
}

// WeatherModeFromName parses an on-disk token ("" → Auto; unknown → Auto).
func WeatherModeFromName(s string) WeatherMode {
	if m, ok := decodeByName(weatherModeDefs, s,
		func(d weatherModeDef) string { return d.name },
		func(i int) WeatherMode { return weatherModeDefs[i].value }); ok {
		return m
	}
	return WeatherModeAuto
}

// WeatherModeOptions is the ordered picker list (editor buttons), derived from the table.
var WeatherModeOptions = buildWeatherModeOptions()

func buildWeatherModeOptions() []WeatherMode {
	opts := make([]WeatherMode, len(weatherModeDefs))
	for i, d := range weatherModeDefs {
		opts[i] = d.value
	}
	return opts
}

// RainKind is a storm's flavor, rolled once when it begins; only the heavy kind throws lightning.
type RainKind int

const (
	RainLight  RainKind = iota // faint mist, lightest filter
	RainNormal                 // steady rain
	RainHeavy                  // dense downpour + occasional lightning
)

// RainKindCount is the parallel-table modulus; render's rainVisuals is sized by it.
const RainKindCount = int(RainHeavy) + 1

// rainKindWeights biases which storm rolls; relative weights, not probabilities.
var rainKindWeights = [RainKindCount]float32{
	RainLight:  0.30,
	RainNormal: 0.40,
	RainHeavy:  0.30,
}

// rollRainKind picks a storm flavor weighted by rainKindWeights.
func rollRainKind(rng *rand.Rand) RainKind {
	total := float32(0)
	for _, w := range rainKindWeights {
		total += w
	}
	r := rng.Float32() * total
	for k := 0; k < RainKindCount; k++ {
		if r < rainKindWeights[k] {
			return RainKind(k)
		}
		r -= rainKindWeights[k]
	}
	// Fall through to the last NON-ZERO-weight bucket (float32 rounding can put
	// a roll past the final threshold); zeroing a weight never resurrects it.
	for k := RainKindCount - 1; k >= 0; k-- {
		if rainKindWeights[k] > 0 {
			return RainKind(k)
		}
	}
	return RainLight
}

// WeatherState is the rain runtime state. Lives on GameState so it survives
// area transitions. Advances on landed steps (TickWeatherStep); Intensity eases
// per frame (TickWeather) so the tint applies/lifts gradually.
type WeatherState struct {
	Phase WeatherPhase
	// Kind rolled once at Clear->Building, held for the storm's life.
	Kind RainKind
	// RainStepsLeft counts the downpour down; seeded RainMinSteps..RainMaxSteps.
	RainStepsLeft int
	// Cooldown: outdoor steps in Clear before rain may roll again.
	Cooldown int
	// Intensity is the 0..1 smoothed strength of tint + rainfall.
	Intensity float32
	// Flash is the 0..1 lightning brightness (heavy only); NextFlash is seconds to the next bolt.
	Flash     float32
	NextFlash float32
}

// AreaIsOutdoor reports whether the area is open to the sky. Enclosed (NOT
// outdoor) when ceiling slabs roof more than OutdoorCeilingThreshold of
// in-bounds tiles. Scans the ceiling layer — a per-frame caller should memoize.
func AreaIsOutdoor(m *AreaDefinition) bool {
	covered, total := 0, 0
	for z := 0; z < m.Height; z++ {
		for x := 0; x < m.Width; x++ {
			if !m.InBounds(x, z) {
				continue
			}
			total++
			if m.CeilingAt(x, z) {
				covered++
			}
		}
	}
	return !(total > 0 && float64(covered)/float64(total) > OutdoorCeilingThreshold)
}

// outdoorVerdictCache memoizes AreaIsOutdoor for the current area so the
// per-step weather tick doesn't rescan the whole ceiling grid each step.
// Recomputed only on entering a different area; mirrors render's enclosureCache.
var outdoorVerdictCache struct {
	name          string
	width, height int
	hash          uint64
	primed        bool
	outdoor       bool
}

// CeilingFingerprint hashes the FULL ceiling layer (FNV-1a over every row) so
// two same-named, same-sized areas with different roofs — including a middle-row
// edit in the editor — can't share a stale verdict. Shared by
// outdoorVerdictCache + render's caches.
func CeilingFingerprint(m *AreaDefinition) uint64 {
	h := FNVOffset64
	for _, row := range m.Ceiling {
		h = FoldLayerRow(h, row)
	}
	return h
}

func areaIsOutdoorCached(m *AreaDefinition) bool {
	c := &outdoorVerdictCache
	hash := CeilingFingerprint(m)
	if c.primed && c.name == m.Name && c.width == m.Width && c.height == m.Height &&
		c.hash == hash {
		return c.outdoor
	}
	c.outdoor = AreaIsOutdoor(m)
	c.name, c.width, c.height = m.Name, m.Width, m.Height
	c.hash, c.primed = hash, true
	return c.outdoor
}

func TickWeatherStep(g *GameState) {
	w := &g.Weather
	// Authored per-area override takes precedence over the roof gate.
	switch g.Area.WeatherMode {
	case WeatherModeClear:
		if w.Phase == WeatherBuilding || w.Phase == WeatherRaining {
			w.Phase = WeatherClearing
			w.RainStepsLeft = 0
		}
		return
	case WeatherModeRain:
		// Force a persistent storm: start one from a dry/clearing state, and keep the
		// step counter topped up so an active storm never lapses to Clearing.
		switch w.Phase {
		case WeatherClear, WeatherClearing:
			w.Phase = WeatherBuilding
			w.Kind = rollRainKind(g.Rand())
		case WeatherRaining:
			w.RainStepsLeft = RainMaxSteps
		}
		return
	}
	if !areaIsOutdoorCached(&g.Area) {
		// Roofed/underground: no rain. A storm in progress drops to Clearing.
		if w.Phase == WeatherBuilding || w.Phase == WeatherRaining {
			w.Phase = WeatherClearing
			w.RainStepsLeft = 0
		}
		return
	}
	switch w.Phase {
	case WeatherClear:
		if w.Cooldown > 0 {
			w.Cooldown--
			return
		}
		if g.Rand().Float32() < RainStartChance {
			w.Phase = WeatherBuilding
			w.Kind = rollRainKind(g.Rand())
		}
	case WeatherRaining:
		if w.RainStepsLeft > 0 {
			w.RainStepsLeft--
		}
		if w.RainStepsLeft <= 0 {
			w.Phase = WeatherClearing
		}
	}
}

// TickWeather eases rain Intensity per frame and fires the intensity-gated
// transitions: Building->Raining once the tint darkens, Clearing->Clear once it
// lifts. Step-gated transitions live in TickWeatherStep. Safe every frame.
func TickWeather(g *GameState, dt float32) {
	w := &g.Weather
	target := float32(0)
	if w.Phase == WeatherBuilding || w.Phase == WeatherRaining {
		target = 1
	}
	w.Intensity = Approach(w.Intensity, target, WeatherRampSpeed*dt)
	switch w.Phase {
	case WeatherBuilding:
		if w.Intensity >= WeatherRainStartLevel {
			w.Phase = WeatherRaining
			w.RainStepsLeft = RandRangeI(g.Rand(), RainMinSteps, RainMaxSteps)
			if w.Kind == RainHeavy {
				// Arm the first bolt a few seconds in, not at rain start.
				w.NextFlash = g.RandRangeF(LightningIntervalMin, LightningIntervalMax)
			}
		}
	case WeatherClearing:
		if w.Intensity <= WeatherIntensityNearZero {
			w.Intensity = 0
			w.Phase = WeatherClear
			w.Cooldown = RandRangeI(g.Rand(), RainCooldownMin, RainCooldownMax)
		}
	}
	tickLightning(g, dt)
}

// tickLightning drives heavy-storm lightning: a scheduled bolt snaps Flash to
// full, then it decays. Only a heavy storm in Raining schedules bolts, drawing
// the RNG once per bolt (not per frame). Decay runs unconditionally.
func tickLightning(g *GameState, dt float32) {
	w := &g.Weather
	if w.Kind == RainHeavy && w.Phase == WeatherRaining {
		w.NextFlash -= dt
		if w.NextFlash <= 0 {
			w.Flash = 1
			w.NextFlash = g.RandRangeF(LightningIntervalMin, LightningIntervalMax)
			return // full brightness this frame; decay starts next tick
		}
	}
	w.Flash = Approach(w.Flash, 0, LightningDecayPerSec*dt)
}

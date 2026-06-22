package core

import "math/rand"

// WeatherPhase is the ambient-rain state machine's stage. Rain is atmospheric
// and outdoors-only (see AreaIsOutdoor). Cycle: Clear -> Building (tint
// darkening in) -> Raining (tint held + rain falling) -> Clearing (tint
// lifting) -> Clear.
type WeatherPhase int

const (
	WeatherClear    WeatherPhase = iota // dry; Cooldown counts down before rain may roll again
	WeatherBuilding                     // tint ramping in before the rain falls
	WeatherRaining                      // tint held + rain falling; RainStepsLeft counts down
	WeatherClearing                     // rain stopped, tint ramping back out
)

// RainKind is a storm's flavor, rolled once when it begins. It scales how
// heavy the rain reads; only the heavy kind throws lightning. Render maps each
// kind to wash darkness / streak density / alpha (render's rainVisuals).
type RainKind int

const (
	RainLight  RainKind = iota // faint mist, lightest filter
	RainNormal                 // steady rain
	RainHeavy                  // dense downpour + occasional lightning
)

// RainKindCount is the parallel-table modulus (mirrors TimeOfDayCount).
// Bump by adding a RainKind above this line — render's rainVisuals is sized by
// it and asserted full at init.
const RainKindCount = int(RainHeavy) + 1

// rainKindWeights biases which storm rolls when one begins. Indexed by
// RainKind; relative weights, not probabilities (summed + sampled by rollRainKind).
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
	// a top-of-range roll past the final threshold); zeroing a weight never
	// resurrects that kind here.
	for k := RainKindCount - 1; k >= 0; k-- {
		if rainKindWeights[k] > 0 {
			return RainKind(k)
		}
	}
	return RainLight
}

// WeatherState is the rain system's runtime state. Lives on GameState so it
// survives area transitions (a storm doesn't reset on walking through a door).
// Advances on landed player steps (TickWeatherStep); Intensity eases per frame
// (TickWeather) so the tint applies/lifts gradually instead of snapping.
type WeatherState struct {
	Phase WeatherPhase
	// Kind is this storm's flavor, rolled once at Clear->Building and held for
	// the storm's life. Only RainHeavy throws lightning.
	Kind RainKind
	// RainStepsLeft counts the downpour down while Raining; seeded to a random
	// RainMinSteps..RainMaxSteps span when the rain begins.
	RainStepsLeft int
	// Cooldown is the outdoor steps that must pass in Clear before rain may
	// roll again — keeps storms from bunching up.
	Cooldown int
	// Intensity is the 0..1 smoothed strength of tint + rainfall, eased per
	// frame toward the phase's target.
	Intensity float32
	// Flash is the 0..1 lightning brightness (heavy storms only): a bolt snaps
	// it to 1, then it decays. NextFlash is seconds until the next bolt.
	Flash     float32
	NextFlash float32
}

// AreaIsOutdoor reports whether the area is open to the sky — the gate for
// ambient weather and the inverse of the enclosed-dungeon lighting override.
// Enclosed (NOT outdoor) when ceiling slabs roof more than
// OutdoorCeilingThreshold of in-bounds tiles. Scans the ceiling layer, so a
// per-frame caller should memoize per area.
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
	rows          int
	top, bot      string
	primed        bool
	outdoor       bool
}

// CeilingFingerprint is a cheap discriminator of an area's ceiling layer (row
// count + first/last row) for the per-area "has a roof?" caches. Keyed on TOP
// of name+dims so two same-named, same-sized areas with different roofs don't
// share a stale verdict. Shared by outdoorVerdictCache + render's caches.
func CeilingFingerprint(m *AreaDefinition) (rows int, top, bot string) {
	rows = len(m.Ceiling)
	if rows > 0 {
		top, bot = m.Ceiling[0], m.Ceiling[rows-1]
	}
	return rows, top, bot
}

func areaIsOutdoorCached(m *AreaDefinition) bool {
	c := &outdoorVerdictCache
	rows, top, bot := CeilingFingerprint(m)
	if c.primed && c.name == m.Name && c.width == m.Width && c.height == m.Height &&
		c.rows == rows && c.top == top && c.bot == bot {
		return c.outdoor
	}
	c.outdoor = AreaIsOutdoor(m)
	c.name, c.width, c.height = m.Name, m.Width, m.Height
	c.rows, c.top, c.bot, c.primed = rows, top, bot, true
	return c.outdoor
}

func TickWeatherStep(g *GameState) {
	w := &g.Weather
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

// TickWeather eases rain Intensity toward the phase's target each frame and
// fires the two intensity-gated transitions: Building->Raining once the tint
// darkens (seeding downpour length) and Clearing->Clear once it lifts (seeding
// the next cooldown). Step-gated transitions live in TickWeatherStep. Safe to
// call every adventure frame, including during battle.
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

// tickLightning drives heavy-storm lightning. A scheduled bolt snaps Flash to
// full, then it decays each frame. Only a heavy storm in Raining schedules new
// bolts, drawing the RNG once per bolt (not per frame) so it doesn't churn the
// shared stream. Decay runs unconditionally so a flash still lifts after the
// storm transitions out.
func tickLightning(g *GameState, dt float32) {
	w := &g.Weather
	if w.Kind == RainHeavy && w.Phase == WeatherRaining {
		w.NextFlash -= dt
		if w.NextFlash <= 0 {
			w.Flash = 1
			w.NextFlash = g.RandRangeF(LightningIntervalMin, LightningIntervalMax)
		}
	}
	w.Flash = Approach(w.Flash, 0, LightningDecayPerSec*dt)
}

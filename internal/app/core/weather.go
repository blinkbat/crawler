package core

import "math/rand"

// WeatherPhase is the ambient-rain state machine's current stage. Rain
// is purely atmospheric and only occurs outdoors (open-sky areas — see
// AreaIsOutdoor). The cycle is: Clear (counting down a cooldown, then
// rolling to start) -> Building (bluegray tint darkening in) -> Raining
// (tint held + rain falling for a step-counted spell) -> Clearing (tint
// lifting back out) -> Clear.
type WeatherPhase int

const (
	WeatherClear    WeatherPhase = iota // dry; Cooldown counts down before rain may roll again
	WeatherBuilding                     // tint ramping in (darkening) before the rain falls
	WeatherRaining                      // tint held + rain falling; RainStepsLeft counts the downpour down
	WeatherClearing                     // rain stopped, tint ramping back out
)

// RainKind is the flavor of a storm, rolled once when it begins. It scales
// how heavy the rain reads — a faint shower vs a proper downpour — and
// only the heavy kind throws lightning. The render layer maps each kind to
// its wash darkness / streak density / streak alpha (render's rainVisuals).
type RainKind int

const (
	RainLight  RainKind = iota // faint mist, lightest filter
	RainNormal                 // steady rain
	RainHeavy                  // dense downpour + occasional lightning
)

// RainKindCount is the parallel-table modulus (mirrors TimeOfDayCount).
// Bump by adding a RainKind above this line — render's rainVisuals table
// is sized by it and asserted full at init.
const RainKindCount = int(RainHeavy) + 1

// rainKindWeights biases which storm rolls when one begins — normal rain
// the baseline, with heavy storms a frequent rather than rare outcome.
// Indexed by RainKind; summed and sampled by rollRainKind, so the values
// are relative weights, not probabilities.
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
	// Fall through to the last NON-ZERO-weight bucket: float32 rounding can
	// leave a top-of-range roll just past the final threshold. Using the last
	// non-zero weight (rather than a hardcoded last index) means zeroing a
	// kind's weight — including the last one — never resurrects it here.
	for k := RainKindCount - 1; k >= 0; k-- {
		if rainKindWeights[k] > 0 {
			return RainKind(k)
		}
	}
	return RainLight
}

// WeatherState is the rain system's runtime state. It lives on GameState
// so it travels with the session (and survives area transitions — a storm
// doesn't reset just because you walked through a door, mirroring how
// StepCount / the day-night phase carry across). The state machine
// advances on landed player steps (TickWeatherStep — trigger roll,
// downpour length, cooldown), while Intensity eases smoothly per frame
// (TickWeather) so the tint "applies" and "lifts" gradually instead of
// snapping per step.
type WeatherState struct {
	Phase WeatherPhase
	// Kind is this storm's flavor (light / normal / heavy), rolled once at
	// the Clear->Building transition and held for the storm's life. Render
	// scales the overlay strength by it; only RainHeavy throws lightning.
	Kind RainKind
	// RainStepsLeft counts the downpour down while Raining; seeded to a
	// random RainMinSteps..RainMaxSteps span when the rain begins.
	RainStepsLeft int
	// Cooldown is the number of outdoor steps that must pass in Clear
	// before rain may roll again — keeps storms from bunching up right
	// after one another.
	Cooldown int
	// Intensity is the 0..1 smoothed strength of the tint + rainfall,
	// eased per frame toward the phase's target. Render reads it for both
	// the bluegray wash alpha and the rain-streak density.
	Intensity float32
	// Flash is the 0..1 lightning brightness (heavy storms only): a bolt
	// snaps it to 1 and it decays per frame. NextFlash is the seconds left
	// until the next bolt is scheduled. Both ticked in TickWeather.
	Flash     float32
	NextFlash float32
}

// AreaIsOutdoor reports whether the area is open to the sky — the gate for
// ambient weather (rain only falls outdoors) and the inverse of the
// "enclosed dungeon" lighting override. An area counts as enclosed (NOT
// outdoor) when ceiling slabs roof more than OutdoorCeilingThreshold of
// its in-bounds tiles; a field or a roofless forest has few/no ceilings
// and reads as outdoor. Scans the ceiling layer, so a per-frame caller
// should memoize per area (render's enclosureCache does).
func AreaIsOutdoor(m AreaDefinition) bool {
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

// TickWeatherStep advances the rain state machine by one landed player
// step. Indoors, any active storm is told to recede (the tint lifts as you
// head underground) and no new rain rolls. Outdoors, it counts the
// cooldown / downpour down and rolls to begin a fresh storm. The two
// tint-gated handoffs (Building->Raining once darkened, Clearing->Clear
// once lifted) live in TickWeather, since they track the per-frame ease,
// not steps.
// outdoorVerdictCache memoizes AreaIsOutdoor for the current area so the
// per-step weather tick doesn't rescan the whole ceiling grid on every landed
// step. The verdict is fixed for an area's lifetime, so it's recomputed only on
// entering a different area — mirrors render's enclosureCache (same name+dims
// area key) for the lighting gate; the two share one definition of "has a roof"
// via AreaIsOutdoor and now also share the once-per-area memoization shape.
var outdoorVerdictCache struct {
	name          string
	width, height int
	rows          int
	top, bot      string
	primed        bool
	outdoor       bool
}

// CeilingFingerprint is a cheap discriminator of an area's ceiling layer —
// row count + first/last row — for the per-area "has a roof?" caches. Keyed
// on TOP of name+dims, it stops two distinct same-named, same-sized areas
// with different roofs from sharing a stale verdict (e.g. two still-"untitled"
// editor maps of the same size). Comparing two short rows is far cheaper than
// the full AreaIsOutdoor ceiling scan it gates. Shared by this package's
// outdoorVerdictCache AND render's enclosure/torch caches so they can't drift.
func CeilingFingerprint(m AreaDefinition) (rows int, top, bot string) {
	rows = len(m.Ceiling)
	if rows > 0 {
		top, bot = m.Ceiling[0], m.Ceiling[rows-1]
	}
	return rows, top, bot
}

func areaIsOutdoorCached(m AreaDefinition) bool {
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
	if !areaIsOutdoorCached(g.Area) {
		// Roofed / underground: no rain here. A storm in progress drops
		// to Clearing so the tint eases off via the per-frame follow.
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

// TickWeather eases the rain Intensity toward the current phase's target
// each frame and fires the two intensity-gated transitions: rain begins
// once the tint has fully darkened (Building -> Raining, seeding the
// downpour length) and the cycle resets once the tint has fully lifted
// (Clearing -> Clear, seeding the next cooldown). Step-gated transitions
// live in TickWeatherStep; this is purely the smooth visual follow plus
// the "wait for the tint" handoffs. Safe to call every adventure frame,
// including during battle (the phase doesn't change there, so the tint
// just holds steady).
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
				// Arm the first bolt a few seconds in, not the instant
				// the rain starts.
				w.NextFlash = g.RandRangeF(LightningIntervalMin, LightningIntervalMax)
			}
		}
	case WeatherClearing:
		if w.Intensity <= 0.01 {
			w.Intensity = 0
			w.Phase = WeatherClear
			w.Cooldown = RandRangeI(g.Rand(), RainCooldownMin, RainCooldownMax)
		}
	}
	tickLightning(g, dt)
}

// tickLightning drives the heavy-storm lightning. A scheduled bolt snaps
// Flash to full; the flash then decays every frame. Only a heavy storm in
// the Raining phase schedules new bolts — and it draws the RNG once per
// bolt (at the scheduled moment), never per frame, so it doesn't churn the
// shared gameplay stream. The decay runs unconditionally so a flash that's
// still fading when the storm transitions out still lifts cleanly.
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

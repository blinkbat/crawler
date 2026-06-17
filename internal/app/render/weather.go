package render

import (
	"fmt"
	"math"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Ambient-rain overlay tuning. Purely presentation — the storm's state
// (whether it rains, how hard, the fade) is owned by core.WeatherState;
// here we just paint it. A bluegray wash darkens the world view and
// animated streaks fall over it, both scaled by Weather.Intensity so the
// effect thickens as the storm peaks and thins as it lifts.
const (
	// Wash + streak colors are shared across rain kinds; the per-kind table
	// (rainVisuals) supplies the peak wash alpha, streak count, and streak
	// alpha that distinguish a faint shower from a heavy downpour.
	rainWashR = 44
	rainWashG = 56
	rainWashB = 80

	rainStreakR = 178
	rainStreakG = 200
	rainStreakB = 226

	rainThickMin  = float32(0.7)  // px line width — thinnest streak
	rainThickMax  = float32(2.2)  // px line width — thickest streak
	rainSlant     = float32(6)    // px horizontal drift over a streak (wind lean)
	rainStreakMin = float32(14)   // px shortest streak
	rainStreakMax = float32(30)   // px longest streak
	rainFallMin   = float32(820)  // px/sec slowest streak
	rainFallMax   = float32(1320) // px/sec fastest streak

	// Lightning flash (heavy storms) — a near-white, faintly blue blanking
	// layer over the world view whose alpha tracks Weather.Flash.
	lightningR        = 212
	lightningG        = 224
	lightningB        = 248
	lightningMaxAlpha = 185

	// weatherIntensityEpsilon is the "effectively off" cutoff for the rain
	// intensity / lightning flash envelopes — below it there's nothing worth
	// painting, so DrawWeather bails. One named threshold instead of the bare
	// 0.001 repeated at each guard.
	weatherIntensityEpsilon = float32(0.001)
)

// rainVisual is the per-kind peak strength of the overlay at full
// Intensity: how dark the wash gets, how many streaks fall, and how
// visible each streak is. Scaled down by the live Intensity envelope so
// the same row drives the fade-in / fade-out too.
type rainVisual struct {
	washAlpha   float32 // peak wash alpha (0..255)
	streaks     float32 // peak streak count
	streakAlpha float32 // peak per-streak alpha (0..255)
}

// rainVisuals indexed by core.RainKind — a light shower reads as a faint
// mist with the lightest filter, a heavy storm as a dense, darker
// downpour. Sized by core.RainKindCount and asserted full at init.
var rainVisuals = [core.RainKindCount]rainVisual{
	core.RainLight:  {washAlpha: 40, streaks: 75, streakAlpha: 40},
	core.RainNormal: {washAlpha: 72, streaks: 175, streakAlpha: 55},
	core.RainHeavy:  {washAlpha: 112, streaks: 300, streakAlpha: 82},
}

// rainStreakTrait holds one streak's screen- and time-independent traits.
// All five are pure functions of the streak index, so they're baked once at
// init into rainStreakTraits and reused every frame — the draw loop used to
// recompute 5 hash01s + derived math per streak per frame (up to 300 streaks
// at RainHeavy → ~1500 hash calls/frame avoided).
type rainStreakTrait struct {
	colFrac float32
	length  float32
	speed   float32
	phase   float32
	thick   float32
}

// rainStreakTraits is sized to the largest rainVisuals streak count so every
// rain kind indexes within it. Built in init (after the table is validated).
var rainStreakTraits []rainStreakTrait

func init() {
	maxStreaks := 0
	for k := 0; k < core.RainKindCount; k++ {
		if rainVisuals[k] == (rainVisual{}) {
			panic(fmt.Sprintf("render: rainVisuals missing a row for RainKind %d — add its wash/streak strength", k))
		}
		if s := int(rainVisuals[k].streaks); s > maxStreaks {
			maxStreaks = s
		}
	}
	rainStreakTraits = make([]rainStreakTrait, maxStreaks)
	for i := range rainStreakTraits {
		u := uint32(i)
		rainStreakTraits[i] = rainStreakTrait{
			colFrac: hash01(u*2 + 1),
			length:  rainStreakMin + hash01(u*2+9)*(rainStreakMax-rainStreakMin),
			speed:   rainFallMin + hash01(u*7+3)*(rainFallMax-rainFallMin),
			phase:   hash01(u*13 + 5),
			thick:   rainThickMin + hash01(u*5+11)*(rainThickMax-rainThickMin),
		}
	}
}

// DrawWeather paints the ambient-rain overlay in screen space, above the
// 3D world but below the HUD (so menus / combat text stay crisp). No-op
// when the storm is fully clear. Stateless: rl.GetTime() drives the fall
// and each streak's traits come from hash01(index), so nothing is
// retained between frames.
func DrawWeather(g *core.GameState) {
	w := g.Weather
	// Intensity and Flash are eased into [0,1] upstream (TickWeather/tickLightning);
	// clamp at the consumption site so the uint8 alpha casts below can never wrap
	// even if that invariant ever drifts (a >1 value would alias to near-zero alpha).
	intensity := core.Clamp(w.Intensity, 0, 1)
	w.Flash = core.Clamp(w.Flash, 0, 1)
	// A lightning flash can outlast the rain's fade by a blink, so it's
	// checked independently of intensity. Nothing to paint otherwise.
	if intensity <= weatherIntensityEpsilon && w.Flash <= weatherIntensityEpsilon {
		return
	}
	sw, sh := screenSizeF()

	if intensity > weatherIntensityEpsilon {
		// Defensive clamp: every other enum-keyed table in render guards its
		// access. w.Kind is only ever set by rollRainKind today, but a corrupt
		// save or future path with an out-of-range Kind would panic indexing
		// the fixed-size table.
		kind := w.Kind
		if kind < 0 || int(kind) >= core.RainKindCount {
			kind = core.RainLight
		}
		vis := rainVisuals[kind]
		// Overcast wash over the world view — darkness scales with kind.
		rl.DrawRectangle(0, 0, int32(sw), int32(sh),
			rl.NewColor(rainWashR, rainWashG, rainWashB, uint8(intensity*vis.washAlpha)))

		if n := int(intensity * vis.streaks); n > 0 {
			if n > len(rainStreakTraits) {
				n = len(rainStreakTraits)
			}
			t := float32(rl.GetTime())
			streakCol := rl.NewColor(rainStreakR, rainStreakG, rainStreakB, uint8(intensity*vis.streakAlpha))
			for i := 0; i < n; i++ {
				tr := rainStreakTraits[i]
				// y wraps from above the top to below the bottom over `span`;
				// the per-streak phase offsets each so they don't fall in lockstep.
				// Only y/x depend on time + screen size; the rest is precomputed.
				span := sh + tr.length
				y := float32(math.Mod(float64(t*tr.speed+tr.phase*span), float64(span))) - tr.length
				x := tr.colFrac * sw
				rl.DrawLineEx(
					rl.NewVector2(x, y),
					rl.NewVector2(x+rainSlant, y+tr.length),
					tr.thick,
					streakCol,
				)
			}
		}
	}

	// Lightning blink — painted last so it lights the rain it falls
	// through. Flash is only ever non-zero during a heavy storm.
	if w.Flash > weatherIntensityEpsilon {
		rl.DrawRectangle(0, 0, int32(sw), int32(sh),
			rl.NewColor(lightningR, lightningG, lightningB, uint8(w.Flash*lightningMaxAlpha)))
	}
}

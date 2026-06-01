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

func init() {
	for k := 0; k < core.RainKindCount; k++ {
		if rainVisuals[k] == (rainVisual{}) {
			panic(fmt.Sprintf("render: rainVisuals missing a row for RainKind %d — add its wash/streak strength", k))
		}
	}
}

// weatherHash01 maps an integer to a stable pseudo-random float in [0,1)
// (the "lowbias32" finalizer). Lets each rain streak derive a fixed column
// / length / speed / phase from its index, so the rainfall is stateless —
// no per-frame particle pool, just index + clock.
func weatherHash01(n uint32) float32 {
	n ^= n >> 16
	n *= 0x7feb352d
	n ^= n >> 15
	n *= 0x846ca68b
	n ^= n >> 16
	return float32(n&0xffffff) / float32(0x1000000)
}

// DrawWeather paints the ambient-rain overlay in screen space, above the
// 3D world but below the HUD (so menus / combat text stay crisp). No-op
// when the storm is fully clear. Stateless: rl.GetTime() drives the fall
// and each streak's traits come from weatherHash01(index), so nothing is
// retained between frames.
func DrawWeather(g core.GameState) {
	w := g.Weather
	intensity := w.Intensity
	// A lightning flash can outlast the rain's fade by a blink, so it's
	// checked independently of intensity. Nothing to paint otherwise.
	if intensity <= 0.001 && w.Flash <= 0.001 {
		return
	}
	sw, sh := screenSizeF()

	if intensity > 0.001 {
		vis := rainVisuals[w.Kind]
		// Overcast wash over the world view — darkness scales with kind.
		rl.DrawRectangle(0, 0, int32(sw), int32(sh),
			rl.NewColor(rainWashR, rainWashG, rainWashB, uint8(intensity*vis.washAlpha)))

		if n := int(intensity * vis.streaks); n > 0 {
			t := float32(rl.GetTime())
			streakCol := rl.NewColor(rainStreakR, rainStreakG, rainStreakB, uint8(intensity*vis.streakAlpha))
			for i := 0; i < n; i++ {
				u := uint32(i)
				colFrac := weatherHash01(u*2 + 1)
				length := rainStreakMin + weatherHash01(u*2+9)*(rainStreakMax-rainStreakMin)
				speed := rainFallMin + weatherHash01(u*7+3)*(rainFallMax-rainFallMin)
				phase := weatherHash01(u*13 + 5)
				thick := rainThickMin + weatherHash01(u*5+11)*(rainThickMax-rainThickMin)
				// y wraps from above the top to below the bottom over `span`;
				// the per-streak phase offsets each so they don't fall in lockstep.
				span := sh + length
				y := float32(math.Mod(float64(t*speed+phase*span), float64(span))) - length
				x := colFrac * sw
				rl.DrawLineEx(
					rl.NewVector2(x, y),
					rl.NewVector2(x+rainSlant, y+length),
					thick,
					streakCol,
				)
			}
		}
	}

	// Lightning blink — painted last so it lights the rain it falls
	// through. Flash is only ever non-zero during a heavy storm.
	if w.Flash > 0.001 {
		rl.DrawRectangle(0, 0, int32(sw), int32(sh),
			rl.NewColor(lightningR, lightningG, lightningB, uint8(w.Flash*lightningMaxAlpha)))
	}
}

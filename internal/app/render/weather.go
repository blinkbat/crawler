package render

import (
	"fmt"
	"math"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Ambient-rain overlay tuning. Storm state owned by core.WeatherState; this only paints it.
const (
	rainThickMin  = float32(0.7)  // px line width — thinnest streak
	rainThickMax  = float32(2.2)  // px line width — thickest streak
	rainSlant     = float32(6)    // px horizontal drift over a streak (wind lean)
	rainStreakMin = float32(14)   // px shortest streak
	rainStreakMax = float32(30)   // px longest streak
	rainFallMin   = float32(820)  // px/sec slowest streak
	rainFallMax   = float32(1320) // px/sec fastest streak

	lightningMaxAlpha = 185 // lightning flash (heavy storms) peak alpha; tracks Weather.Flash

	// weatherIntensityEpsilon: "effectively off" cutoff for intensity/flash; below it DrawWeather bails.
	weatherIntensityEpsilon = float32(0.001)
)

// Overlay base colors (alpha applied per-frame via colorWithAlpha). Wash + streak
// are shared across rain kinds; rainVisuals supplies the per-kind strengths.
// Lightning is the heavy-storm near-white blanking layer.
var (
	rainWashColor   = rl.NewColor(44, 56, 80, 255)
	rainStreakColor = rl.NewColor(178, 200, 226, 255)
	lightningColor  = rl.NewColor(212, 224, 248, 255)
)

// rainVisual is the per-kind peak overlay strength at full Intensity; scaled by the live envelope.
type rainVisual struct {
	washAlpha   float32 // peak wash alpha (0..255)
	streaks     float32 // peak streak count
	streakAlpha float32 // peak per-streak alpha (0..255)
}

// rainVisuals indexed by core.RainKind; asserted full at init.
var rainVisuals = [core.RainKindCount]rainVisual{
	core.RainLight:  {washAlpha: 40, streaks: 75, streakAlpha: 40},
	core.RainNormal: {washAlpha: 72, streaks: 175, streakAlpha: 55},
	core.RainHeavy:  {washAlpha: 112, streaks: 300, streakAlpha: 82},
}

// rainStreakTrait holds one streak's screen/time-independent traits — pure functions of the
// streak index, baked once at init into rainStreakTraits to avoid per-frame hash recompute.
type rainStreakTrait struct {
	colFrac float32
	length  float32
	speed   float32
	phase   float32
	thick   float32
}

// rainStreakTraits sized to the largest rainVisuals streak count so every kind indexes within it.
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

// DrawWeather paints the ambient-rain overlay in screen space, above the 3D world but below the
// HUD. No-op when clear. Stateless: rl.GetTime() drives the fall, traits come from hash01(index).
func DrawWeather(g *core.GameState) {
	w := g.Weather
	// Clamp at the consumption site so the uint8 alpha casts can't wrap if the [0,1] invariant drifts.
	intensity := core.Clamp(w.Intensity, 0, 1)
	flash := core.Clamp(w.Flash, 0, 1)
	// Flash can outlast the rain's fade, so it's checked independently of intensity.
	if intensity <= weatherIntensityEpsilon && flash <= weatherIntensityEpsilon {
		return
	}
	sw, sh := screenSizeF()

	if intensity > weatherIntensityEpsilon {
		// Defensive clamp: an out-of-range Kind (corrupt save) would panic indexing the fixed table.
		kind := w.Kind
		if kind < 0 || int(kind) >= core.RainKindCount {
			kind = core.RainLight
		}
		vis := rainVisuals[kind]
		// Overcast wash over the world view.
		rl.DrawRectangle(0, 0, int32(sw), int32(sh),
			colorWithAlpha(rainWashColor, uint8(intensity*vis.washAlpha)))

		if n := int(intensity * vis.streaks); n > 0 {
			if n > len(rainStreakTraits) {
				n = len(rainStreakTraits)
			}
			t := float32(rl.GetTime())
			streakCol := colorWithAlpha(rainStreakColor, uint8(intensity*vis.streakAlpha))
			for i := 0; i < n; i++ {
				tr := rainStreakTraits[i]
				// y wraps over `span`; per-streak phase offsets each so they don't fall in lockstep.
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

	// Lightning blink — painted last so it lights the rain it falls through.
	if flash > weatherIntensityEpsilon {
		rl.DrawRectangle(0, 0, int32(sw), int32(sh),
			colorWithAlpha(lightningColor, uint8(flash*lightningMaxAlpha)))
	}
}

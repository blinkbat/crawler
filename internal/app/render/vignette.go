package render

import (
	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Battle danger vignette — a soft claret breathing at the screen edges while
// an enemy is mid-swing (the defend-timing window). The whole frame leans
// threatening for exactly the beat the player must react to, the same way the
// rumble and the red incoming-attack marker do — peripheral pressure, not a
// new HUD element. Screen-space, drawn after the world + weather and below
// the HUD so panels and the defend bar stay clean over it.

// dangerVignetteDepth is the fraction of the screen's smaller dimension each
// edge gradient reaches inward. Shallow — the center stays untinted so the
// enemy formation and the timing bar keep full contrast.
const dangerVignetteDepth = float32(0.16)

// dangerVignetteAlpha is the peak edge alpha (0-255 scale) before the
// breathing modulation. Deliberately faint: the cue should be felt before
// it's noticed.
const dangerVignetteAlpha = float32(34)

// DrawBattleDangerVignette paints the edge vignette while the battle is in
// the enemy attack-timing phase. No-op in every other phase, so the cost is
// one branch outside combat. Breathes at the canonical status-flicker rate
// (UI_STANDARDS "Pulse / breathing") — the same urgency rhythm the status
// pills flicker with.
func DrawBattleDangerVignette(g core.GameState) {
	if g.Battle.Phase != core.BattleEnemyTiming {
		return
	}
	sw, sh := screenSize()
	if sw <= 0 || sh <= 0 {
		return
	}
	minDim := sw
	if sh < minDim {
		minDim = sh
	}
	depth := int32(float32(minDim) * dangerVignetteDepth)
	if depth < 8 {
		return
	}
	a := dangerVignetteAlpha * (0.62 + 0.38*pulseFlicker())
	edge := colorWithAlpha(borderDanger, uint8(a))
	clear := colorWithAlpha(borderDanger, 0)
	// Four edge gradients, each fading from the claret edge to transparent
	// toward the center. Corners double-cover (left/right bands span the full
	// height), which reads as a natural corner darkening rather than a seam.
	rl.DrawRectangleGradientV(0, 0, sw, depth, edge, clear)        // top
	rl.DrawRectangleGradientV(0, sh-depth, sw, depth, clear, edge) // bottom
	rl.DrawRectangleGradientH(0, 0, depth, sh, edge, clear)        // left
	rl.DrawRectangleGradientH(sw-depth, 0, depth, sh, clear, edge) // right
}

package render

import (
	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Battle danger vignette: soft claret breathing at the screen edges during the
// defend-timing window. Screen-space, drawn after world+weather and below the HUD.

// dangerVignetteDepth: fraction of the smaller screen dimension each edge gradient
// reaches inward. Shallow so the center keeps full contrast.
const dangerVignetteDepth = float32(0.16)

// dangerVignetteAlpha: peak edge alpha (0-255) before breathing. Deliberately faint.
const dangerVignetteAlpha = float32(34)

// dangerVignettePulseBase/Swing split the breathing: alpha rides
// base + swing*pulseFlicker(), never fully fading (base), peaking at base+swing.
const (
	dangerVignettePulseBase  = float32(0.62)
	dangerVignettePulseSwing = float32(0.38)
)

// DrawBattleDangerVignette paints the edge vignette during the enemy attack-timing
// phase (no-op otherwise). Breathes at the canonical status-flicker rate.
func DrawBattleDangerVignette(g *core.GameState) {
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
	a := dangerVignetteAlpha * (dangerVignettePulseBase + dangerVignettePulseSwing*pulseFlicker())
	edge := colorWithAlpha(borderDanger, uint8(a))
	clear := colorWithAlpha(borderDanger, 0)
	// Four edge gradients fading to transparent toward center; left/right bands
	// span full height so corners double-cover into natural darkening, not a seam.
	rl.DrawRectangleGradientV(0, 0, sw, depth, edge, clear)        // top
	rl.DrawRectangleGradientV(0, sh-depth, sw, depth, clear, edge) // bottom
	rl.DrawRectangleGradientH(0, 0, depth, sh, edge, clear)        // left
	rl.DrawRectangleGradientH(sw-depth, 0, depth, sh, clear, edge) // right
}

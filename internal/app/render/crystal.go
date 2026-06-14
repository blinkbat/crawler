package render

import (
	"math"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// DrawCrystals paints every healing crystal in the current area as a floating,
// gently-bobbing gem (a six-sided bipyramid) at its tile center. A CHARGED
// crystal glows bright cyan and pulses; a spent one sits dim and still, so the
// player can read its state at a glance. The lighting shader bound by DrawWorld
// is still active here (same contract DrawChests / DrawDoors rely on), so the
// gem picks up the area profile; callers must invoke this inside the 3D pass.
func DrawCrystals(camera rl.Camera3D, g core.GameState, assets Resources) {
	if len(g.Crystals) == 0 {
		return
	}
	forward := horizontalForward(camera)
	t := rl.GetTime()
	for _, c := range g.Crystals {
		base := tileWorldPos(c.TileX, c.TileZ, 0)
		if behindCull(camera.Position, forward, base) {
			continue
		}
		// Float the gem above the floor with a slow vertical bob.
		bob := float32(math.Sin(t*2.0)) * 0.05
		midY := base.Y + 0.55 + bob
		mid := rl.NewVector3(base.X, midY, base.Z)
		top := rl.NewVector3(base.X, midY+0.38, base.Z)
		bot := rl.NewVector3(base.X, midY-0.38, base.Z)

		col := crystalColor(c.Charged, t)
		// Two stacked cones tip-to-tip form the gem: upper point (full radius at
		// the waist → 0 at the top), lower point (0 at the bottom → full radius).
		rl.DrawCylinderEx(mid, top, 0.20, 0.0, 6, col)
		rl.DrawCylinderEx(bot, mid, 0.0, 0.20, 6, col)
		// Faceted wire outline so it reads as cut crystal, not a solid cone.
		rl.DrawCylinderWiresEx(mid, top, 0.20, 0.0, 6, crystalEdge(c.Charged))
		rl.DrawCylinderWiresEx(bot, mid, 0.0, 0.20, 6, crystalEdge(c.Charged))
	}
}

// crystalColor is the gem body tint: a pulsing bright cyan while charged, a flat
// dim slate while dormant.
func crystalColor(charged bool, t float64) rl.Color {
	if !charged {
		return rl.NewColor(70, 92, 110, 190)
	}
	// Pulse the brightness between ~0.75 and 1.0 so a charged crystal "breathes."
	pulse := 0.75 + 0.25*float32(math.Sin(t*3.0)*0.5+0.5)
	return rl.NewColor(uint8(120*pulse), uint8(230*pulse), 255, 235)
}

// crystalEdge is the faceted wire tint paired with crystalColor.
func crystalEdge(charged bool) rl.Color {
	if !charged {
		return rl.NewColor(110, 130, 150, 150)
	}
	return rl.NewColor(210, 250, 255, 220)
}

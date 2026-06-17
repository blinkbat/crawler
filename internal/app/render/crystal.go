package render

import (
	"math"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// crystalGeometry bundles the world-unit dimensions the crystal gem is built
// from, so the draw (DrawCrystals) and the floating prompt (DrawCrystalPrompt)
// read the same numbers and can't drift — the same pattern as chestGeo. FloatY
// is the gem's mid-point lift above the floor; HalfHeight is the waist-to-tip
// distance of each cone; WaistRadius is the gem's widest radius; PromptHeadroom
// is the extra lift from the top tip to the floating "Rest" cue.
type crystalGeometry struct {
	FloatY         float32
	HalfHeight     float32
	WaistRadius    float32
	PromptHeadroom float32
}

var crystalGeo = crystalGeometry{
	FloatY:         0.55,
	HalfHeight:     0.38,
	WaistRadius:    0.20,
	PromptHeadroom: 0.3,
}

// DrawCrystals paints every healing crystal in the current area as a floating,
// gently-bobbing gem (a six-sided bipyramid) at its tile center. A CHARGED
// crystal glows bright cyan and pulses; a spent one sits dim and still, so the
// player can read its state at a glance. The lighting shader bound by DrawWorld
// is still active here (same contract DrawChests / DrawDoors rely on), so the
// gem picks up the area profile; callers must invoke this inside the 3D pass.
func DrawCrystals(camera rl.Camera3D, g *core.GameState, assets Resources) {
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
		midY := base.Y + crystalGeo.FloatY + bob
		mid := rl.NewVector3(base.X, midY, base.Z)
		top := rl.NewVector3(base.X, midY+crystalGeo.HalfHeight, base.Z)
		bot := rl.NewVector3(base.X, midY-crystalGeo.HalfHeight, base.Z)

		col := crystalColor(c.Charged, t)
		r := crystalGeo.WaistRadius
		// Two stacked cones tip-to-tip form the gem: upper point (full radius at
		// the waist → 0 at the top), lower point (0 at the bottom → full radius).
		rl.DrawCylinderEx(mid, top, r, 0.0, 6, col)
		rl.DrawCylinderEx(bot, mid, 0.0, r, 6, col)
		// Faceted wire outline so it reads as cut crystal, not a solid cone.
		rl.DrawCylinderWiresEx(mid, top, r, 0.0, 6, crystalEdge(c.Charged))
		rl.DrawCylinderWiresEx(bot, mid, 0.0, r, 6, crystalEdge(c.Charged))
	}
}

// DrawCrystalPrompt paints the floating "press Enter to rest" cue over the
// charged crystal the player is currently adjacent to (mirrors DrawChestPrompt).
// Only charged crystals are interactable, so a spent one shows no prompt. Drawn
// AFTER rl.EndMode3D so the prompt text renders in screen space — see
// drawAdventureScene for the call order.
func DrawCrystalPrompt(camera rl.Camera3D, g *core.GameState, assets Resources) {
	idx := core.AdjacentChargedCrystalIndex(g.Crystals, g.Player.TileX, g.Player.TileZ)
	if idx < 0 {
		return
	}
	c := g.Crystals[idx]
	// Anchor above the gem's top point (FloatY + HalfHeight) plus a little
	// headroom, matching the bob-free rest pose — read from crystalGeo so it
	// tracks the gem geometry.
	world := tileWorldPos(c.TileX, c.TileZ, crystalGeo.FloatY+crystalGeo.HalfHeight+crystalGeo.PromptHeadroom)
	drawFloatingInteractPrompt(camera, world, "Rest", assets)
}

// crystalColor is the gem body tint: a pulsing bright cyan while charged, a flat
// dim slate while dormant.
func crystalColor(charged bool, t float64) rl.Color {
	if !charged {
		return rl.NewColor(70, 92, 110, 190)
	}
	// Pulse the brightness between ~0.75 and 1.0 so a charged crystal "breathes."
	// R/G ride the shared crystalCyanBase (theme.go) so the gem and the editor
	// marker can't drift; the blue channel stays pinned full + a fixed alpha for
	// the bright cut-crystal read.
	pulse := 0.75 + 0.25*float32(math.Sin(t*3.0)*0.5+0.5)
	return rl.NewColor(uint8(float32(crystalCyanBase.R)*pulse), uint8(float32(crystalCyanBase.G)*pulse), 255, 235)
}

// crystalEdge is the faceted wire tint paired with crystalColor.
func crystalEdge(charged bool) rl.Color {
	if !charged {
		return rl.NewColor(110, 130, 150, 150)
	}
	return rl.NewColor(210, 250, 255, 220)
}

package render

import (
	"math"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// crystalGeometry shares the gem's world-unit dimensions between DrawCrystals
// and DrawCrystalPrompt so they can't drift. FloatY: mid-point lift above floor;
// HalfHeight: waist-to-tip of each cone; WaistRadius: widest radius;
// PromptHeadroom: extra lift from top tip to the "Rest" cue.
type crystalGeometry struct {
	FloatY         float32
	HalfHeight     float32
	WaistRadius    float32
	PromptHeadroom float32
}

var crystalGeo = crystalGeometry{
	FloatY:         1.65,
	HalfHeight:     1.14,
	WaistRadius:    0.60,
	PromptHeadroom: 0.3,
}

// DrawCrystals paints each healing crystal as a floating, bobbing six-sided
// bipyramid: charged ones pulse bright cyan, spent ones sit dim and still.
// Drawn unlit (default shader) — must be called inside the 3D pass (BeginMode3D),
// after DrawWorld's EndShaderMode (same as DrawChests/DrawDoors).
func DrawCrystals(camera rl.Camera3D, g *core.GameState, assets Resources) {
	if len(g.Crystals) == 0 {
		return
	}
	vc := newViewCull(camera)
	t := rl.GetTime()
	for _, c := range g.Crystals {
		base := tileWorldPos(c.TileX, c.TileZ, g.Area.StandGroundY(c.TileX, c.TileZ))
		if vc.cull(base) {
			continue
		}
		// Float above floor with a slow vertical bob.
		bob := float32(math.Sin(t*2.0)) * 0.05
		midY := base.Y + crystalGeo.FloatY + bob
		mid := rl.NewVector3(base.X, midY, base.Z)
		top := rl.NewVector3(base.X, midY+crystalGeo.HalfHeight, base.Z)
		bot := rl.NewVector3(base.X, midY-crystalGeo.HalfHeight, base.Z)

		col := crystalColor(c.Charged)
		r := crystalGeo.WaistRadius
		// Two stacked cones tip-to-tip form the gem.
		rl.DrawCylinderEx(mid, top, r, 0.0, 6, col)
		rl.DrawCylinderEx(bot, mid, 0.0, r, 6, col)
		// Faceted wire outline so it reads as cut crystal.
		rl.DrawCylinderWiresEx(mid, top, r, 0.0, 6, crystalEdge(c.Charged))
		rl.DrawCylinderWiresEx(bot, mid, 0.0, r, 6, crystalEdge(c.Charged))
	}
}

// DrawCrystalPrompt paints the "rest" cue over the adjacent charged crystal
// (only charged ones are interactable). Drawn AFTER rl.EndMode3D (screen space).
func DrawCrystalPrompt(camera rl.Camera3D, g *core.GameState, assets Resources) {
	idx := core.AdjacentChargedCrystalIndex(g.Crystals, g.Player.TileX, g.Player.TileZ)
	if idx < 0 {
		return
	}
	c := g.Crystals[idx]
	// Anchor above the gem's top point plus headroom (bob-free rest pose).
	world := tileWorldPos(c.TileX, c.TileZ, g.Area.StandGroundY(c.TileX, c.TileZ)+crystalGeo.FloatY+crystalGeo.HalfHeight+crystalGeo.PromptHeadroom)
	drawFloatingInteractPrompt(camera, world, "Rest", assets)
}

// crystalPulseHz is the charged crystal's breathe rate: legacy 3 rad/s in Hz (≈0.48 Hz).
const crystalPulseHz = 3.0 / (2 * math.Pi)

// crystalColor: pulsing bright cyan while charged, dim slate while dormant.
func crystalColor(charged bool) rl.Color {
	if !charged {
		return crystalDormantBody
	}
	// Breathe brightness 0.75-1.0; modulate only R/G (blue/alpha pinned to match editor marker).
	breathe := 0.75 + 0.25*pulse(crystalPulseHz)
	return rl.NewColor(uint8(float32(crystalChargedBody.R)*breathe), uint8(float32(crystalChargedBody.G)*breathe), crystalChargedBody.B, crystalChargedBody.A)
}

// crystalEdge is the faceted wire tint paired with crystalColor.
func crystalEdge(charged bool) rl.Color {
	if !charged {
		return crystalEdgeDormant
	}
	return crystalEdgeCharged
}

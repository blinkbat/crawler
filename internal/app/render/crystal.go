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
	FloatY:         1.25, // low hover — bottom tip rests just above the floor
	HalfHeight:     1.14,
	WaistRadius:    0.60,
	PromptHeadroom: 0.3,
}

// crystalFacets is the cut-gem side count for the bipyramid cones + wire.
const crystalFacets = 8

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
		// Charged gems wear a faint oversized halo (drawn first so the body sits on
		// top and only the aura's fringe shows) — a cool glow around the cut stone.
		if c.Charged {
			glow := crystalChargedGlow
			gr := r * 1.4
			gTop := rl.NewVector3(base.X, midY+crystalGeo.HalfHeight*1.18, base.Z)
			gBot := rl.NewVector3(base.X, midY-crystalGeo.HalfHeight*1.18, base.Z)
			rl.DrawCylinderEx(mid, gTop, gr, 0.0, crystalFacets, glow)
			rl.DrawCylinderEx(gBot, mid, 0.0, gr, crystalFacets, glow)
		}
		// Two stacked cones tip-to-tip form the gem.
		rl.DrawCylinderEx(mid, top, r, 0.0, crystalFacets, col)
		rl.DrawCylinderEx(bot, mid, 0.0, r, crystalFacets, col)
		// Faceted wire outline so it reads as cut crystal.
		rl.DrawCylinderWiresEx(mid, top, r, 0.0, crystalFacets, crystalEdge(c.Charged))
		rl.DrawCylinderWiresEx(bot, mid, 0.0, r, crystalFacets, crystalEdge(c.Charged))
		// Bright glint spike off the top tip — a moving shine that sells "shiny".
		if c.Charged {
			glintTip := rl.NewVector3(base.X, midY+crystalGeo.HalfHeight+0.14, base.Z)
			rl.DrawCylinderEx(top, glintTip, 0.07, 0.0, crystalFacets, crystalCoreColor())
		}
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

// crystalGlintHz is the faster shimmer of the tip glint — quicker than the body
// breathe so the shine twinkles rather than pulsing in lockstep with the gem.
const crystalGlintHz = 1.1

// crystalColor: pulsing bright cyan while charged, dim slate while dormant.
func crystalColor(charged bool) rl.Color {
	if !charged {
		return crystalDormantBody
	}
	// Breathe brightness 0.82-1.0; modulate only R/G (blue/alpha pinned to match editor marker).
	breathe := 0.82 + 0.18*pulse(crystalPulseHz)
	return rl.NewColor(uint8(float32(crystalChargedBody.R)*breathe), uint8(float32(crystalChargedBody.G)*breathe), crystalChargedBody.B, crystalChargedBody.A)
}

// crystalCoreColor is the bright near-white tip glint, twinkling on crystalGlintHz.
func crystalCoreColor() rl.Color {
	glint := 0.7 + 0.3*pulse(crystalGlintHz)
	return rl.NewColor(uint8(float32(crystalChargedCore.R)*glint), uint8(float32(crystalChargedCore.G)*glint), crystalChargedCore.B, crystalChargedCore.A)
}

// crystalEdge is the faceted wire tint paired with crystalColor.
func crystalEdge(charged bool) rl.Color {
	if !charged {
		return crystalEdgeDormant
	}
	return crystalEdgeCharged
}

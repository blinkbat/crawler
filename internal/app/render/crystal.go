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
// player can read its state at a glance. Drawn unlit via immediate-mode
// DrawCylinderEx under raylib's default shader — DrawWorld has already called
// EndShaderMode by the time the caller reaches here (same as DrawChests /
// DrawDoors); callers must still invoke this inside the 3D pass (BeginMode3D).
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
		// Float the gem above the floor with a slow vertical bob.
		bob := float32(math.Sin(t*2.0)) * 0.05
		midY := base.Y + crystalGeo.FloatY + bob
		mid := rl.NewVector3(base.X, midY, base.Z)
		top := rl.NewVector3(base.X, midY+crystalGeo.HalfHeight, base.Z)
		bot := rl.NewVector3(base.X, midY-crystalGeo.HalfHeight, base.Z)

		col := crystalColor(c.Charged)
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
	world := tileWorldPos(c.TileX, c.TileZ, g.Area.StandGroundY(c.TileX, c.TileZ)+crystalGeo.FloatY+crystalGeo.HalfHeight+crystalGeo.PromptHeadroom)
	drawFloatingInteractPrompt(camera, world, "Rest", assets)
}

// crystalPulseHz is the breathe rate of a charged crystal's body tint — the
// legacy sin(t*3.0) angular rate (3 rad/s) expressed in Hz for the shared pulse()
// helper, ≈0.48 Hz.
const crystalPulseHz = 3.0 / (2 * math.Pi)

// crystalColor is the gem body tint: a pulsing bright cyan while charged, a flat
// dim slate while dormant.
func crystalColor(charged bool) rl.Color {
	if !charged {
		return crystalDormantBody
	}
	// Pulse the brightness between 0.75 and 1.0 so a charged crystal "breathes" —
	// the same 0.75 + 0.25*pulse() idiom the timing-bar throb uses. The body tint
	// (crystalChargedBody, theme.go) keeps R/G in lockstep with the editor marker
	// and pins the blue/alpha; we modulate only its R/G here.
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

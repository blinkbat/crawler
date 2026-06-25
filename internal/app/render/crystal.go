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

// crystalSpinDegPerSec is the gem's gentle idle rotation about its vertical axis.
const crystalSpinDegPerSec = 24.0

// crystalLightColor is the charged crystal's point-light tint: the gem's cyan
// (crystalCyanBase) lifted to a torch-comparable HDR intensity so a live crystal
// actually pools cool light on the nearby stone. Fed into the torch light path
// (collectTorches) — a live crystal is just another point light, Grimrock-style.
// The fake halo aura this replaces was removed (it read as a second gem, not a glow).
var crystalLightColor = rl.NewVector3(
	float32(crystalCyanBase.R)/255*crystalLightGain,
	float32(crystalCyanBase.G)/255*crystalLightGain,
	float32(crystalCyanBase.B)/255*crystalLightGain,
)

const crystalLightGain = 1.6

// crystalSpinAngle is the gem's Y rotation (degrees): a continuous slow idle spin,
// plus a one-shot fast burst (CrystalSpinBurstTurns turns, ease-out) that plays down
// as the touch-armed SpinBurst countdown drains. The burst total is a whole-turn
// multiple so it rejoins the idle spin seamlessly when it ends.
func crystalSpinAngle(t float32, c core.Crystal) float32 {
	angle := t * crystalSpinDegPerSec
	if c.SpinBurst > 0 {
		p := 1 - c.SpinBurst/core.CrystalSpinBurstDuration // 0→1 over the burst
		ease := 1 - (1-p)*(1-p)*(1-p)                      // ease-out cubic — fast then settle
		angle += 360 * core.CrystalSpinBurstTurns * ease
	}
	return angle
}

// DrawCrystals paints each healing crystal as a floating, bobbing, slowly-spinning
// six-sided bipyramid: charged ones pulse bright cyan (and cast a real cyan point
// light via collectTorches), spent ones sit dim. Drawn unlit (default shader) — must
// be called inside the 3D pass (BeginMode3D), after DrawWorld's EndShaderMode.
func DrawCrystals(camera rl.Camera3D, g *core.GameState, assets Resources) {
	if len(g.Crystals) == 0 {
		return
	}
	vc := newViewCull(camera)
	t := float32(rl.GetTime())
	for _, c := range g.Crystals {
		base := tileWorldPos(c.TileX, c.TileZ, g.Area.StandGroundY(c.TileX, c.TileZ))
		if vc.cull(base) {
			continue
		}
		// Float above floor with a slow vertical bob.
		bob := float32(math.Sin(float64(t)*2.0)) * 0.05
		midY := base.Y + crystalGeo.FloatY + bob
		col := crystalColor(c.Charged)
		r := crystalGeo.WaistRadius
		hh := crystalGeo.HalfHeight
		// Build the gem around a local origin and spin it about Y via the rlgl matrix
		// stack (idle spin + touch burst). Light comes from collectTorches, not here.
		mid := rl.NewVector3(0, 0, 0)
		top := rl.NewVector3(0, hh, 0)
		bot := rl.NewVector3(0, -hh, 0)
		rl.PushMatrix()
		rl.Translatef(base.X, midY, base.Z)
		rl.Rotatef(crystalSpinAngle(t, c), 0, 1, 0)
		// Two stacked cones tip-to-tip form the gem.
		rl.DrawCylinderEx(mid, top, r, 0.0, crystalFacets, col)
		rl.DrawCylinderEx(bot, mid, 0.0, r, crystalFacets, col)
		// Faceted wire outline so it reads as cut crystal.
		rl.DrawCylinderWiresEx(mid, top, r, 0.0, crystalFacets, crystalEdge(c.Charged))
		rl.DrawCylinderWiresEx(bot, mid, 0.0, r, crystalFacets, crystalEdge(c.Charged))
		// Bright glint spike off the top tip — a moving shine that sells "shiny".
		if c.Charged {
			glintTip := rl.NewVector3(0, hh+0.14, 0)
			rl.DrawCylinderEx(top, glintTip, 0.07, 0.0, crystalFacets, crystalCoreColor())
		}
		rl.PopMatrix()
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

// scaleRG scales c's R/G by f (the breathing/glint factor), pinning B and A — the
// crystal's "ride R/G, keep the blue/alpha to match the editor marker" pattern.
func scaleRG(c rl.Color, f float32) rl.Color {
	return rl.NewColor(uint8(float32(c.R)*f), uint8(float32(c.G)*f), c.B, c.A)
}

// crystalBreathe is the 0.82..1.0 body-brightness factor shared by crystalColor
// and the crystal light pool (world.go) so the gem and its glow pulse in lockstep.
func crystalBreathe() float32 {
	return 0.82 + 0.18*pulse(crystalPulseHz)
}

// crystalColor: pulsing bright cyan while charged, dim slate while dormant.
func crystalColor(charged bool) rl.Color {
	if !charged {
		return crystalDormantBody
	}
	// Modulate only R/G (blue/alpha pinned to match editor marker).
	return scaleRG(crystalChargedBody, crystalBreathe())
}

// crystalCoreColor is the bright near-white tip glint, twinkling on crystalGlintHz.
func crystalCoreColor() rl.Color {
	glint := 0.7 + 0.3*pulse(crystalGlintHz)
	return scaleRG(crystalChargedCore, glint)
}

// crystalEdge is the faceted wire tint paired with crystalColor.
func crystalEdge(charged bool) rl.Color {
	if !charged {
		return crystalEdgeDormant
	}
	return crystalEdgeCharged
}

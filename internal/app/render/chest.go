package render

import (
	"crawler/internal/app/core"
	"math"
	"strconv"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// chestGeometry bundles the world-unit dimensions DrawChestPrompt
// uses to anchor its floating prompt — these match the painted
// chest meshes built in loadChestBodyProp / loadChestLidProp so the
// "Press Enter" cue floats above the chest's actual silhouette.
type chestGeometry struct {
	BodyHeight float32
	LidHeight  float32
}

var chestGeo = chestGeometry{
	BodyHeight: 0.46,
	LidHeight:  0.18,
}

// chestLidLootedLift is how far above the body top a looted chest's
// lid drifts. The lid also tilts backward to read as "thrown open"
// rather than "lifted off and floating."
const (
	chestLidLootedLift    = float32(0.34)
	chestLidLootedTiltDeg = float32(-58)
	// chestLidHingeZOffset is the Z of the lid's pivot relative to the
	// chest centre. The looted-lid rotation references it twice (the
	// hinge anchor and the per-part relative-Z); a single const keeps
	// the two in lockstep so the lid can't pivot around the wrong line.
	chestLidHingeZOffset = float32(-0.25)
)

// DrawChests renders every chest as a two-piece painted prop —
// wooden body with brass corner straps + hoop bands + lockplate +
// jewel, capped by a wooden lid with corner caps + a hoop band.
// Closed chests stack the lid flush on the body; looted chests
// hinge the lid back around its rear edge so it reads as "thrown
// open" rather than "lid floating in the air." Bark texture +
// lighting shader give the whole prop the painted-storybook feel
// of the trees and bushes around it.
//
// Called after DrawWorld so chests draw under the lighting shader
// still bound. The body / lid propModels live on Resources so the
// meshes load once at startup and unload at game exit.
func DrawChests(camera rl.Camera3D, g *core.GameState, assets Resources) {
	forward := horizontalForward(camera)
	for _, ch := range g.Chests {
		base := tileWorldPos(ch.TileX, ch.TileZ, g.Area.StandGroundY(ch.TileX, ch.TileZ))
		// Skip chests behind the camera — same generous slack the world tile
		// loop uses, so a chest you just turned from doesn't pop out.
		if behindCull(camera.Position, forward, base) {
			continue
		}
		drawGroundShadowAt(base.X, base.Y+groundShadowFloorClearance, base.Z, 0.40)

		// Body — sits at floor level. propModel.draw owns the
		// per-part offsets, scales, and tints.
		assets.chestBody.draw(base, 1.0, 0)

		// Lid — flush atop the body when closed; hinge-tilted
		// backward off the rear edge when looted.
		lidCentreY := base.Y + chestGeo.BodyHeight
		if ch.Looted {
			drawChestLidLooted(assets, base, lidCentreY)
		} else {
			lidPos := rl.NewVector3(base.X, lidCentreY, base.Z)
			assets.chestLid.draw(lidPos, 1.0, 0)
		}
	}
}

// drawChestLidLooted paints the lid in its "thrown open" pose —
// pivoted around the rear top edge of the body so the lid reads as
// hinged backward, not floating off. Each lid part is positioned
// relative to the hinge, rotated by chestLidLootedTiltDeg around the
// world-X axis, then drawn through the lighting shader. The same
// chestLid propModel parts list drives the rendering; only the
// per-part transform differs.
func drawChestLidLooted(assets Resources, base rl.Vector3, lidCentreY float32) {
	hingeZ := base.Z + chestLidHingeZOffset
	tiltRad := float64(chestLidLootedTiltDeg) * math.Pi / 180
	cosT := float32(math.Cos(tiltRad))
	sinT := float32(math.Sin(tiltRad))
	for _, part := range assets.chestLid.parts {
		// Authored offset relative to the lid's own centre, lifted
		// to the body top + the looted lift.
		offX := part.offset.X
		offY := part.offset.Y
		offZ := part.offset.Z
		// Relative-to-hinge coords (subtract hinge Z; lid centre Y
		// becomes the hinge Y after adding the lift).
		relY := offY + chestLidLootedLift
		relZ := offZ - chestLidHingeZOffset // hinge sits at chestLidHingeZOffset relative to chest centre, so the part's authored Z already lines up
		// Rotate around X axis through the hinge: (y, z) → (y',
		// z') = (y·cos − z·sin, y·sin + z·cos). With our negative
		// tilt the lid pivots backward (z grows negative as y
		// climbs), tipping the front of the lid up and away from
		// the player.
		ry := relY*cosT - relZ*sinT
		rz := relY*sinT + relZ*cosT
		// World position: hinge centre + rotated offset.
		position := rl.NewVector3(base.X+offX, lidCentreY+ry, hingeZ+rz)
		drawScale := part.scale
		// drawModelEx with the part's own rotation axis still
		// applies (e.g. for parts spun around Y); the tilt around
		// X applies on top via a second pass — but to keep the
		// math simple we let raylib's rotation apply only the
		// per-part rotation, and bake the lid-tilt into the
		// position via the offset rotation above. The visible
		// result is the lid hinge-open with corner caps and band
		// rotating in lockstep with the wood.
		rl.DrawModelEx(assets.chestLid.models[part.modelIdx], position, partRotationAxis(part), part.rotation, drawScale, part.tint)
	}
}

// DrawChestPrompt paints the floating "press Enter to open" cue over
// the chest the player is currently adjacent to. Skipped while the
// chest modal is open (the modal itself is the prompt) and skipped for
// already-looted chests. Drawn AFTER rl.EndMode3D so the prompt text
// renders in screen space — see drawAdventureScene for the call order.
func DrawChestPrompt(camera rl.Camera3D, g *core.GameState, assets Resources) {
	if g.ChestOpen >= 0 {
		return
	}
	idx := core.AdjacentInteractableChestIndex(g.Chests, g.Player.TileX, g.Player.TileZ)
	if idx < 0 {
		return
	}
	ch := g.Chests[idx]
	world := tileWorldPos(ch.TileX, ch.TileZ, chestGeo.BodyHeight+chestGeo.LidHeight+0.4)
	if behindCamera(camera, world) {
		return
	}
	screen := rl.GetWorldToScreen(world, camera)
	// Controller-first prompt: the confirm glyph + the verb (no spelled-out
	// keys). gamepad-first per UI_STANDARDS.md.
	y := screen.Y - glyphBoxH(FontBody) - 8
	drawGlyphPrompt(assets.Font(), GlyphA, "Open", screen.X, y, FontBody)
}

// DrawChestModal paints the chest-open dialog: a card with the item
// list, a Take All row, and a hint footer. Rendered after the world so
// it sits on top of everything. Cursor row uses the same selection
// style as the pause menu.
func DrawChestModal(g *core.GameState, assets Resources) {
	if g.ChestOpen < 0 || g.ChestOpen >= len(g.Chests) {
		return
	}
	chest := g.Chests[g.ChestOpen]
	stacks := core.LiveStacks(chest.Items)

	font := assets.Font()
	rowH := int32(34)
	// Header dropped — the chest model still sits behind the veil and
	// the player walked up to it to open this; titling the modal
	// "TREASURE" was tautological. Card height now budgets for rows +
	// Take All + footer only.
	cardH := int32(48 + rowH*(int32(len(stacks))+1) + 32)
	if cardH < 200 {
		cardH = 200
	}
	card := drawModalScaffold(font, overlayCardWidthSmall, cardH, "")
	cardX, cardY := int32(card.X), int32(card.Y)
	cardW := int32(card.Width)

	rowY := cardY + 28
	rowX := cardX + 20
	rowW := cardW - 40
	if len(stacks) == 0 {
		drawTextWithShadow(font, "(empty)", float32(rowX), float32(rowY), FontBody, textMuted)
		rowY += rowH
	}
	rowRect := func(y int32) rl.Rectangle {
		return SelectionRowRect(rowX, y, rowW, rowH)
	}
	for i, st := range stacks {
		focused := g.ChestMenuIndex == i
		if focused {
			DrawSelectedRow(rowRect(rowY))
		}
		col := textMuted
		if focused {
			col = textPrimary
		}
		label := core.ItemInfo(st.Kind).Name + "  x" + strconv.Itoa(st.Count)
		drawTextWithShadow(font, label, float32(rowX), float32(rowY), FontBody, col)
		rowY += rowH
	}
	// "Take All" row sits below the items. Always present even when the
	// list is empty so the keyboard never lands on an unselectable row.
	// ChestTakeAllRow keeps render + explore in sync on which cursor
	// value means "Take All."
	{
		focused := g.ChestMenuIndex == core.ChestTakeAllRow(len(stacks))
		if focused {
			DrawSelectedRow(rowRect(rowY))
		}
		col := textMuted
		if focused {
			col = textPrimary
		}
		drawTextWithShadow(font, "Take All", float32(rowX), float32(rowY), FontBody, col)
	}
	drawModalFooterGlyphs(font, card, []HintSeg{
		Hint("Move", GlyphUpDown),
		Hint("Take", GlyphA),
		Hint("Close", GlyphB),
	})
}

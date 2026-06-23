package render

import (
	"crawler/internal/app/core"
	"math"
	"strconv"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// chestGeometry — world-unit dims for anchoring the floating prompt; must match the meshes in loadChestBodyProp/loadChestLidProp.
type chestGeometry struct {
	BodyHeight float32
	LidHeight  float32
}

var chestGeo = chestGeometry{
	BodyHeight: 0.46,
	LidHeight:  0.18,
}

// Looted-lid pose: lift above body top + backward tilt to read as "thrown open".
const (
	chestLidLootedLift    = float32(0.34)
	chestLidLootedTiltDeg = float32(-58)
	// chestLidHingeZOffset: lid pivot Z relative to chest centre. Referenced twice (hinge anchor + per-part Z); one const keeps them in lockstep.
	chestLidHingeZOffset = float32(-0.25)
)

// Chest modal geometry.
const (
	chestRowH       = modalListRowH // item / Take All row height
	chestCardTopPad = int32(48)     // card top edge → first row baseline budget
	chestCardBotPad = int32(32)     // last row → card bottom budget
	chestRowInsetX  = int32(20)     // row left/right inset; modalContentInsetX intent, tighter (20 not 22) for the narrow item card
	chestRowInsetY  = int32(28)     // first row top inset from the card top
)

// DrawChests renders each chest as a two-piece prop (body + lid; closed = flush, looted = hinged open).
// Must be called after DrawWorld so the lighting shader is still bound.
func DrawChests(camera rl.Camera3D, g *core.GameState, assets Resources) {
	vc := newViewCull(camera)
	for _, ch := range g.Chests {
		base := tileWorldPos(ch.TileX, ch.TileZ, g.Area.StandGroundY(ch.TileX, ch.TileZ))
		if vc.cull(base) {
			continue
		}
		drawGroundShadowAt(base.X, base.Y+groundShadowFloorClearance, base.Z, 0.40)

		assets.chestBody.draw(base, 1.0, 0)

		lidCentreY := base.Y + chestGeo.BodyHeight
		if ch.Looted {
			drawChestLidLooted(assets, base, lidCentreY)
		} else {
			lidPos := rl.NewVector3(base.X, lidCentreY, base.Z)
			assets.chestLid.draw(lidPos, 1.0, 0)
		}
	}
}

// drawChestLidLooted paints the lid pivoted around the body's rear top edge ("thrown open"), each part tilted by chestLidLootedTiltDeg about world-X.
func drawChestLidLooted(assets Resources, base rl.Vector3, lidCentreY float32) {
	hingeZ := base.Z + chestLidHingeZOffset
	tiltRad := float64(chestLidLootedTiltDeg) * math.Pi / 180
	cosT := float32(math.Cos(tiltRad))
	sinT := float32(math.Sin(tiltRad))
	for _, part := range assets.chestLid.parts {
		offX := part.offset.X
		offY := part.offset.Y
		offZ := part.offset.Z
		relY := offY + chestLidLootedLift
		relZ := offZ - chestLidHingeZOffset
		// Rotate (y,z) around X through the hinge; negative tilt pivots the lid backward.
		ry := relY*cosT - relZ*sinT
		rz := relY*sinT + relZ*cosT
		position := rl.NewVector3(base.X+offX, lidCentreY+ry, hingeZ+rz)
		drawScale := part.scale
		// raylib's rotation applies only the per-part rotation; the lid tilt is baked into position above.
		rl.DrawModelEx(assets.chestLid.models[part.modelIdx], position, partRotationAxis(part), part.rotation, drawScale, part.tint)
	}
}

// DrawChestPrompt paints the floating "open" cue over an adjacent unlooted chest. Must be called after rl.EndMode3D (screen space).
func DrawChestPrompt(camera rl.Camera3D, g *core.GameState, assets Resources) {
	if g.ChestOpen >= 0 {
		return
	}
	idx := core.AdjacentInteractableChestIndex(g.Chests, g.Player.TileX, g.Player.TileZ)
	if idx < 0 {
		return
	}
	ch := g.Chests[idx]
	// Anchor above the lid; must add StandGroundY (body draws there) or it detaches on raised tiles.
	world := tileWorldPos(ch.TileX, ch.TileZ, g.Area.StandGroundY(ch.TileX, ch.TileZ)+chestGeo.BodyHeight+chestGeo.LidHeight+0.4)
	drawFloatingInteractPrompt(camera, world, "Open", assets)
}

// drawFloatingInteractPrompt projects world to screen and paints the "[A] verb" cue above it (shared by chest/crystal prompts). Must run after rl.EndMode3D.
func drawFloatingInteractPrompt(camera rl.Camera3D, world rl.Vector3, verb string, assets Resources) {
	if behindCamera(camera, world) {
		return
	}
	screen := rl.GetWorldToScreen(world, camera)
	y := screen.Y - glyphBoxH(FontBody) - 8
	drawGlyphPrompt(assets.Font(), GlyphA, verb, screen.X, y, FontBody)
}

// DrawChestModal paints the chest-open dialog: item list, Take All row, hint footer.
func DrawChestModal(g *core.GameState, assets Resources) {
	if g.ChestOpen < 0 || g.ChestOpen >= len(g.Chests) {
		return
	}
	chest := g.Chests[g.ChestOpen]
	stacks := core.LiveStacks(chest.Items)

	font := assets.Font()
	rowH := chestRowH
	// No header; card height budgets rows + Take All + footer only.
	cardH := chestCardTopPad + rowH*(int32(len(stacks))+1) + chestCardBotPad
	if cardH < modalMinCardH {
		cardH = modalMinCardH
	}
	card := drawModalScaffold(font, overlayCardWidthSmall, cardH, "")
	cardX, cardY := int32(card.X), int32(card.Y)
	cardW := int32(card.Width)

	rowY := cardY + chestRowInsetY
	rowX := cardX + chestRowInsetX
	rowW := cardW - 2*chestRowInsetX
	if len(stacks) == 0 {
		drawTextWithShadow(font, "(empty)", float32(rowX), float32(rowY), FontBody, textMuted)
		rowY += rowH
	}
	for i, st := range stacks {
		focused := g.ChestMenuIndex == i
		col := rowTextColor(focused, false, textMuted)
		label := core.ItemInfo(st.Kind).Name + "  x" + strconv.Itoa(st.Count)
		y := rowY
		drawModalListRow(rowX, y, rowW, rowH, focused, func() {
			drawTextWithShadow(font, label, float32(rowX), float32(y), FontBody, col)
		})
		rowY += rowH
	}
	// "Take All" row, always present so the cursor never lands on an unselectable row. ChestTakeAllRow keeps render + explore in sync.
	{
		focused := g.ChestMenuIndex == core.ChestTakeAllRow(len(stacks))
		col := rowTextColor(focused, false, textMuted)
		drawModalListRow(rowX, rowY, rowW, rowH, focused, func() {
			drawTextWithShadow(font, "Take All", float32(rowX), float32(rowY), FontBody, col)
		})
	}
	drawModalFooterGlyphs(font, card, []HintSeg{
		Hint("Move", GlyphUpDown),
		Hint("Take", GlyphA),
		Hint("Close", GlyphB),
	})
}

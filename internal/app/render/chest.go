package render

import (
	"crawler/internal/app/core"
	"strconv"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// chestGeometry — world-unit dims for anchoring the floating prompt; must match the meshes in loadChestBodyProp/loadChestLidProp.
// PromptHeadroom: extra lift from the lid top to the "Open" cue (mirrors crystalGeometry).
type chestGeometry struct {
	BodyHeight     float32
	LidHeight      float32
	PromptHeadroom float32
}

var chestGeo = chestGeometry{
	BodyHeight:     0.46,
	LidHeight:      0.18,
	PromptHeadroom: 0.4,
}

// chestShadowRadius is the chest's ground-shadow half-extent (world units); kept
// beside chestGeo rather than in propShadowRadius since a chest isn't a prop tile.
const chestShadowRadius = float32(0.40)

// Open-box "mouth": a dark recessed slab flush with the body top so an opened or
// looted chest reads as a hole inside (no lid for now). Inset within the body
// footprint (0.62 × 0.50) so a wood rim frames the opening; the camera peek tilts
// down to look into it. chestMouthLift sits the top face a hair above the wood to
// avoid z-fighting.
const (
	chestMouthW    = float32(0.46)
	chestMouthD    = float32(0.34)
	chestMouthH    = float32(0.10)
	chestMouthLift = float32(0.005)
)

// glyphPromptRise lifts the floating in-world interact cue this many px above its
// projected anchor (so the "[A] verb" clears the chest/crystal lid).
const glyphPromptRise = float32(8)

// Chest modal geometry.
const (
	chestRowH       = modalListRowH    // item / Take All row height
	chestCardTopPad = int32(48)        // card top edge → first row baseline budget
	chestCardBotPad = int32(32)        // last row → card bottom budget
	chestRowInsetX  = modalGutterTight // row left/right inset; tighter than modalContentInsetX for the narrow item card
	chestRowInsetY  = int32(28)        // first row top inset from the card top
)

// DrawChests renders each chest body, then either a closed lid or — when the chest
// is the one being viewed (ChestOpen) or already looted — an open box with a dark
// hole inside (no lid for now). Must be called after DrawWorld so the lighting
// shader is still bound.
func DrawChests(camera rl.Camera3D, g *core.GameState, assets Resources) {
	vc := newViewCull(camera)
	for i := range g.Chests {
		ch := g.Chests[i]
		base := tileWorldPos(ch.TileX, ch.TileZ, g.Area.StandGroundY(ch.TileX, ch.TileZ))
		if vc.cull(base) {
			continue
		}
		drawGroundShadowAt(base.X, base.Y+groundShadowFloorClearance, base.Z, chestShadowRadius)

		assets.chestBody.draw(base, 1.0, 0)

		if i == g.ChestOpen || ch.Looted {
			drawChestOpenMouth(base)
		} else {
			lidPos := rl.NewVector3(base.X, base.Y+chestGeo.BodyHeight, base.Z)
			assets.chestLid.draw(lidPos, 1.0, 0)
		}
	}
}

// drawChestOpenMouth paints the dark inset opening flush with the body top so an
// open chest reads as a hollow box (no lid). The slab sinks into the body; only its
// top face shows, framed by the wood rim — the camera peek looks down into it.
func drawChestOpenMouth(base rl.Vector3) {
	center := rl.NewVector3(base.X, base.Y+chestGeo.BodyHeight-chestMouthH*0.5+chestMouthLift, base.Z)
	rl.DrawCubeV(center, rl.NewVector3(chestMouthW, chestMouthH, chestMouthD), chestInteriorColor)
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
	world := tileWorldPos(ch.TileX, ch.TileZ, g.Area.StandGroundY(ch.TileX, ch.TileZ)+chestGeo.BodyHeight+chestGeo.LidHeight+chestGeo.PromptHeadroom)
	drawFloatingInteractPrompt(camera, world, "Open", assets)
}

// drawFloatingInteractPrompt projects world to screen and paints the "[A] verb" cue above it (shared by chest/crystal prompts). Must run after rl.EndMode3D.
func drawFloatingInteractPrompt(camera rl.Camera3D, world rl.Vector3, verb string, assets Resources) {
	if behindCamera(camera, world) {
		return
	}
	screen := rl.GetWorldToScreen(world, camera)
	y := screen.Y - glyphBoxH(FontBody) - glyphPromptRise
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
	// No header; card height budgets rows + Take All + footer only. An empty chest
	// draws an "(empty)" line that occupies a list row, so floor the list at 1.
	listRows := int32(len(stacks))
	if listRows < 1 {
		listRows = 1
	}
	cardH := chestCardTopPad + rowH*(listRows+1) + chestCardBotPad
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
	drawModalFooterGlyphs(font, card, chestHints)
}

// chestHints is the chest overlay's footer, built once (like shopHints) so the
// panel doesn't reallocate a hint bar every frame.
var chestHints = []HintSeg{
	Hint("Move", GlyphUpDown),
	Hint("Take", GlyphA),
	Hint("Close", GlyphB),
}

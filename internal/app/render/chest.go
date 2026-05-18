package render

import (
	"crawler/internal/app/core"
	"fmt"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// chestGeometry bundles every world-unit dimension for the chest model.
// One struct keeps body, lid, and lockplate dimensions adjacent so a
// chest-resize is one literal edit instead of five package-level
// constants drifting independently.
type chestGeometry struct {
	BodyWidth  float32
	BodyHeight float32
	BodyDepth  float32
	LidHeight  float32
	LidLift    float32 // looted chests render the lid floating this far above the body
	MetalBand  float32 // thickness of the metal lockplate strip
}

var chestGeo = chestGeometry{
	BodyWidth:  0.60,
	BodyHeight: 0.45,
	BodyDepth:  0.45,
	LidHeight:  0.16,
	LidLift:    0.55,
	MetalBand:  0.06,
}

// DrawChests renders every chest in the game state as a two-piece box
// (body + lid) at its tile center. Looted chests get a deeper body
// color and a lid that sits open behind the body; unopened chests get
// the closed-lid silhouette plus a soft "press Enter" prompt floating
// over the chest when the player is one step away. Called after
// DrawWorld so chests draw under the same lighting shader still bound.
func DrawChests(camera rl.Camera3D, g core.GameState, _ Resources) {
	g_ := chestGeo
	for _, ch := range g.Chests {
		// Body sits at BodyHeight/2 so its base flushes against the
		// floor (y = 0 is the floor's top surface in this engine).
		body := tileWorldPos(ch.TileX, ch.TileZ, g_.BodyHeight/2)
		col := chestBodyColor
		if ch.Looted {
			col = chestBodyLooted
		}
		rl.DrawCube(body, g_.BodyWidth, g_.BodyHeight, g_.BodyDepth, col)
		rl.DrawCubeWires(body, g_.BodyWidth, g_.BodyHeight, g_.BodyDepth, rl.Black)

		// Lid placement: closed chests stack the lid right on top of the
		// body; looted chests float the lid upward + tilt by drawing it
		// higher and slightly larger to read as "lifted off."
		lidY := g_.BodyHeight + g_.LidHeight/2
		lidW := g_.BodyWidth
		lidD := g_.BodyDepth
		if ch.Looted {
			lidY += g_.LidLift
			lidW *= 1.05
			lidD *= 1.05
		}
		lid := tileWorldPos(ch.TileX, ch.TileZ, lidY)
		rl.DrawCube(lid, lidW, g_.LidHeight, lidD, chestLidColor)
		rl.DrawCubeWires(lid, lidW, g_.LidHeight, lidD, rl.Black)

		// Metal lockplate band: thin strip across the front of the body
		// + lid seam. Offsets the band slightly past the front face so
		// it doesn't z-fight the body.
		band := tileWorldPos(ch.TileX, ch.TileZ, g_.BodyHeight-g_.MetalBand/2)
		band.Z += g_.BodyDepth/2 + 0.01
		rl.DrawCube(band, g_.BodyWidth*0.5, g_.MetalBand, 0.02, chestMetalColor)
	}
	_ = camera
}

// DrawChestPrompt paints the floating "press Enter to open" cue over
// the chest the player is currently adjacent to. Skipped while the
// chest modal is open (the modal itself is the prompt) and skipped for
// already-looted chests. Drawn AFTER rl.EndMode3D so the prompt text
// renders in screen space — see drawAdventureScene for the call order.
func DrawChestPrompt(camera rl.Camera3D, g core.GameState, assets Resources) {
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
	label := "Press Enter to open"
	font := assets.Font()
	size := float32(18)
	m := rl.MeasureTextEx(font, label, size, 1)
	x := screen.X - m.X/2
	y := screen.Y - m.Y - 8
	drawTextWithShadow(font, label, x, y, size, borderActive)
}

// DrawChestModal paints the chest-open dialog: a card with the item
// list, a Take All row, and a hint footer. Rendered after the world so
// it sits on top of everything. Cursor row uses the same selection
// style as the pause menu.
func DrawChestModal(g core.GameState, assets Resources) {
	if g.ChestOpen < 0 || g.ChestOpen >= len(g.Chests) {
		return
	}
	chest := g.Chests[g.ChestOpen]
	stacks := core.LiveStacks(chest.Items)

	font := assets.Font()
	screenW, screenH := screenSize()
	cardW := int32(360)
	cardH := int32(96 + 30*(int32(len(stacks))+1))
	if cardH < 180 {
		cardH = 180
	}
	cardX := centerX(cardW)
	cardY := screenH/2 - cardH/2

	rl.DrawRectangle(0, 0, screenW, screenH, surfaceVeil)
	drawCard(cardX, cardY, cardW, cardH, surfacePrimary, borderSoft, borderActive)
	drawHeading(font, "TREASURE", cardX+18, cardY+14, borderActive)

	rowY := cardY + 56
	rowH := int32(28)
	rowX := cardX + 18
	rowW := cardW - 36
	if len(stacks) == 0 {
		drawTextWithShadow(font, "(empty)", float32(rowX), float32(rowY), 18, textMuted)
		rowY += rowH
	}
	rowRect := func(y int32) rl.Rectangle {
		return rl.NewRectangle(float32(rowX-6), float32(y-4), float32(rowW+12), float32(rowH))
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
		label := fmt.Sprintf("%s  x%d", core.ItemInfo(st.Kind).Name, st.Count)
		drawTextWithShadow(font, label, float32(rowX), float32(rowY), 18, col)
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
		drawTextWithShadow(font, "Take All", float32(rowX), float32(rowY), 18, col)
	}
	DrawFooterHint(font, "Up/Down move   Enter take   Esc close",
		float32(cardX+cardW/2), float32(cardY+cardH-22), 13)
}

package render

import (
	"image/color"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

func drawTurnPanel(g core.GameState, assets Resources) {
	turns := core.TurnForecast(g, 9)
	if len(turns) == 0 {
		return
	}
	screenW, _ := screenSize()
	w := int32(272)
	x := screenW - w - 22
	y := int32(110)
	rowH := int32(42)
	headerH := int32(62)
	padBottom := int32(16)
	h := headerH + int32(len(turns))*rowH + padBottom

	drawCard(x, y, w, h, surfacePrimary, borderSoft, borderStrong)
	drawHeading(assets.hudFont, "TURN ORDER", x+22, y+18, borderStrong)

	for i, turn := range turns {
		rowY := y + headerH + int32(i)*rowH
		col := turnEntryColor(turn)

		rowX := x + 14
		rowW := w - 28
		rowInnerH := rowH - 8

		if i == 0 {
			tint := rl.NewColor(col.R, col.G, col.B, 86)
			drawSmallPanel(rowX, rowY, rowW, rowInnerH, tint)
			drawSmallPanelOutline(rowX, rowY, rowW, rowInnerH, rl.NewColor(col.R, col.G, col.B, 230))
			cx := float32(rowX + 14)
			cy := float32(rowY) + float32(rowInnerH)/2
			rl.DrawTriangle(
				rl.NewVector2(cx-4, cy-8),
				rl.NewVector2(cx+6, cy),
				rl.NewVector2(cx-4, cy+8),
				col,
			)
		} else {
			tint := rl.NewColor(col.R, col.G, col.B, 36)
			drawSmallPanel(rowX, rowY, rowW, rowInnerH, tint)
			rl.DrawRectangle(rowX+8, rowY+6, 4, rowInnerH-12, rl.NewColor(col.R, col.G, col.B, 210))
		}

		labelX := rowX + 28
		labelY := rowY + (rowInnerH-22)/2 - 1
		drawTextWithShadow(assets.hudFont, turn.Label, float32(labelX), float32(labelY), 22, col)
	}
}

func turnEntryColor(turn core.TurnEntry) color.RGBA {
	if turn.Enemy {
		return rl.NewColor(245, 100, 92, 255)
	}
	return partyClassPresentationFor(turn.Class).turnColor
}

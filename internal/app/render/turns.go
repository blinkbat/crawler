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
	screenW := int32(rl.GetScreenWidth())
	w := int32(216)
	x := screenW - w - 22
	y := int32(96)
	rowH := int32(32)
	headerH := int32(50)
	padBottom := int32(14)
	h := headerH + int32(len(turns))*rowH + padBottom

	drawCard(x, y, w, h, surfacePrimary, borderSoft, borderStrong)
	drawHeading(assets.hudFont, "TURN ORDER", x+18, y+14, borderStrong)

	for i, turn := range turns {
		rowY := y + headerH + int32(i)*rowH
		col := turnEntryColor(turn)

		rowX := x + 12
		rowW := w - 24
		rowInnerH := rowH - 6

		if i == 0 {
			tint := rl.NewColor(col.R, col.G, col.B, 86)
			drawSmallPanel(rowX, rowY, rowW, rowInnerH, tint)
			drawSmallPanelOutline(rowX, rowY, rowW, rowInnerH, rl.NewColor(col.R, col.G, col.B, 230))
			cx := float32(rowX + 12)
			cy := float32(rowY) + float32(rowInnerH)/2
			rl.DrawTriangle(
				rl.NewVector2(cx-3, cy-6),
				rl.NewVector2(cx+5, cy),
				rl.NewVector2(cx-3, cy+6),
				col,
			)
		} else {
			tint := rl.NewColor(col.R, col.G, col.B, 36)
			drawSmallPanel(rowX, rowY, rowW, rowInnerH, tint)
			rl.DrawRectangle(rowX+6, rowY+5, 3, rowInnerH-10, rl.NewColor(col.R, col.G, col.B, 210))
		}

		labelX := rowX + 22
		labelY := rowY + (rowInnerH-18)/2 - 1
		drawTextWithShadow(assets.hudFont, turn.Label, float32(labelX), float32(labelY), 18, col)
	}
}

func turnEntryColor(turn core.TurnEntry) color.RGBA {
	if turn.Enemy {
		return rl.NewColor(245, 100, 92, 255)
	}
	return partyClassPresentationFor(turn.Class).turnColor
}

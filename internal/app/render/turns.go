package render

import (
	"image/color"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// turnPanelW is the on-screen width of the turn-order panel, and
// turnPanelRightMargin is the gap between the panel's right edge and the
// screen edge. Exposed via TurnPanelLayoutWidth so the action panel (which
// reserves the right side of the screen for the turn panel) can derive
// its own width from one source instead of duplicating the literal.
const (
	turnPanelW           = int32(272)
	turnPanelRightMargin = int32(22)
)

// TurnPanelLayoutWidth returns the total horizontal space the turn-order
// panel occupies (panel + right margin), so adjacent HUD panels can lay
// themselves out without hardcoding the figure.
func TurnPanelLayoutWidth() int32 {
	return turnPanelW + turnPanelRightMargin
}

func drawTurnPanel(g core.GameState, assets Resources) {
	turns := core.TurnForecast(g, 9)
	if len(turns) == 0 {
		return
	}
	screenW, _ := screenSize()
	w := turnPanelW
	x := screenW - w - turnPanelRightMargin
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
			drawSmallPanel(rowX, rowY, rowW, rowInnerH, colorWithAlpha(col, 86))
			drawSmallPanelOutline(rowX, rowY, rowW, rowInnerH, colorWithAlpha(col, 230))
			cx := float32(rowX + 14)
			cy := float32(rowY) + float32(rowInnerH)/2
			drawArrowMarker(rl.NewVector2(cx-4, cy), 10, 0, 8, col)
		} else {
			drawSmallPanel(rowX, rowY, rowW, rowInnerH, colorWithAlpha(col, 36))
			rl.DrawRectangle(rowX+8, rowY+6, 4, rowInnerH-12, colorWithAlpha(col, 210))
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

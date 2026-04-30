package render

import (
	"image/color"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

func drawTurnPanel(g core.GameState, assets Resources) {
	x := int32(rl.GetScreenWidth() - 196)
	y := int32(96)
	w := int32(174)
	h := int32(286)
	drawRoundedRect(x, y, w, h, 0.08, rl.NewColor(7, 12, 22, 175))
	drawRoundedRectLines(x, y, w, h, 0.08, rl.NewColor(77, 208, 232, 180))
	drawHUDText(assets.hudFont, "TURN", x+14, y+12, 22)
	turns := core.TurnForecast(g, 9)
	for i, turn := range turns {
		rowY := y + 46 + int32(i)*25
		col := turnEntryColor(turn)
		soft := rl.NewColor(col.R, col.G, col.B, 38)
		strong := rl.NewColor(col.R, col.G, col.B, 210)
		if i == 0 {
			drawRoundedRect(x+8, rowY-2, w-16, 23, 0.22, rl.NewColor(col.R, col.G, col.B, 72))
			drawRoundedRectLines(x+8, rowY-2, w-16, 23, 0.22, strong)
		} else {
			drawRoundedRect(x+8, rowY-1, w-16, 21, 0.22, soft)
		}
		rl.DrawRectangle(x+11, rowY+2, 4, 15, strong)
		drawHUDTextColor(assets.hudFont, turn.Label, x+22, rowY, 18, col)
	}
}

func turnEntryColor(turn core.TurnEntry) color.RGBA {
	if turn.Enemy {
		return rl.NewColor(245, 92, 82, 255)
	}
	switch turn.Class {
	case core.ClassWarrior:
		return rl.NewColor(235, 88, 78, 255)
	case core.ClassCleric:
		return rl.NewColor(244, 222, 138, 255)
	case core.ClassThief:
		return rl.NewColor(94, 214, 148, 255)
	case core.ClassWizard:
		return rl.NewColor(120, 152, 255, 255)
	default:
		return rl.RayWhite
	}
}

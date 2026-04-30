package render

import (
	"crawler/internal/app/core"
	rl "github.com/gen2brain/raylib-go/raylib"
)

func drawMenuOverlay(g core.GameState, assets Resources) {
	screenW := int32(rl.GetScreenWidth())
	screenH := int32(rl.GetScreenHeight())
	panelW := int32(360)
	panelH := int32(206)
	panelX := screenW/2 - panelW/2
	panelY := screenH/2 - panelH/2

	rl.DrawRectangle(0, 0, screenW, screenH, rl.NewColor(0, 0, 0, 110))
	drawRoundedRect(panelX, panelY, panelW, panelH, 0.08, rl.NewColor(8, 13, 25, 190))
	drawRoundedRectLines(panelX, panelY, panelW, panelH, 0.08, rl.NewColor(77, 208, 232, 210))
	drawHUDText(assets.hudFont, "MENU", panelX+26, panelY+22, 30)

	drawMenuOption(assets.hudFont, "Restart", panelX+44, panelY+86, g.MenuIndex == 0)
	drawMenuOption(assets.hudFont, "Quit", panelX+44, panelY+130, g.MenuIndex == 1)
	drawHUDText(assets.hudFont, "Esc closes", panelX+218, panelY+166, 16)
}

func drawMenuOption(font rl.Font, text string, x, y int32, selected bool) {
	if selected {
		drawRoundedRect(x-18, y-5, 248, 34, 0.25, rl.NewColor(72, 76, 110, 145))
		rl.DrawTriangle(
			rl.NewVector2(float32(x-7), float32(y+12)),
			rl.NewVector2(float32(x-15), float32(y+4)),
			rl.NewVector2(float32(x-15), float32(y+20)),
			rl.NewColor(118, 235, 136, 255),
		)
	}
	drawHUDText(font, text, x+10, y, 24)
}

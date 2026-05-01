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

	drawOption(assets.hudFont, "Restart", panelX+44, panelY+86, g.MenuIndex == 0, menuOptionStyle)
	drawOption(assets.hudFont, "Quit", panelX+44, panelY+130, g.MenuIndex == 1, menuOptionStyle)
	drawHUDText(assets.hudFont, "Esc closes", panelX+218, panelY+166, 16)
}

package render

import (
	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

func drawMenuOverlay(g core.GameState, assets Resources) {
	screenW := int32(rl.GetScreenWidth())
	screenH := int32(rl.GetScreenHeight())
	panelW := int32(420)
	panelH := int32(252)
	panelX := screenW/2 - panelW/2
	panelY := screenH/2 - panelH/2

	rl.DrawRectangle(0, 0, screenW, screenH, surfaceVeil)
	drawCard(panelX, panelY, panelW, panelH, surfacePrimary, borderSoft, borderStrong)

	drawHeading(assets.hudFont, "PAUSED", panelX+34, panelY+24, borderStrong)
	drawTextWithShadow(assets.hudFont, "MENU", float32(panelX+34), float32(panelY+50), 34, textPrimary)

	drawOption(assets.hudFont, "Restart", panelX+58, panelY+118, g.MenuIndex == 0, menuOptionStyle)
	drawOption(assets.hudFont, "Quit", panelX+58, panelY+170, g.MenuIndex == 1, menuOptionStyle)

	drawTextWithShadow(assets.hudFont, "Esc closes    -    W/S choose    -    Enter confirm", float32(panelX+34), float32(panelY+panelH-32), 14, textHint)
}

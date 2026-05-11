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

	drawMenuRow(assets.hudFont, "Restart", panelX+58, panelY+118, g.MenuIndex == 0)
	drawMenuRow(assets.hudFont, "Quit", panelX+58, panelY+170, g.MenuIndex == 1)
}

// drawMenuRow paints one entry in the pause menu: a selection panel +
// arrow marker when active, then the label. Heavier shadow (+2,+2 alpha
// 190) than the in-game HUD's drawTextWithShadow because the menu text
// sits over an unbusy veil at a larger size and reads better with weight.
func drawMenuRow(font rl.Font, text string, x, y int32, selected bool) {
	if selected {
		drawSmallPanel(x-18, y-6, 316, 40, surfaceActiveTint)
		drawSmallPanelOutline(x-18, y-6, 316, 40, borderActive)
		centerY := y - 6 + 40/2
		rl.DrawTriangle(
			rl.NewVector2(float32(x-7), float32(centerY)),
			rl.NewVector2(float32(x-16), float32(centerY-9)),
			rl.NewVector2(float32(x-16), float32(centerY+9)),
			borderActive,
		)
	}
	pos := rl.NewVector2(float32(x+12), float32(y))
	shadow := rl.NewVector2(pos.X+2, pos.Y+2)
	rl.DrawTextEx(font, text, shadow, 26, 1, rl.NewColor(0, 0, 0, 190))
	rl.DrawTextEx(font, text, pos, 26, 1, textPrimary)
}

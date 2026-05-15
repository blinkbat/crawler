package render

import (
	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

func drawMenuOverlay(g core.GameState, assets Resources) {
	screenW, screenH := screenSize()
	panelW := int32(420)
	panelH := int32(308)
	panelX := centerX(panelW)
	panelY := screenH/2 - panelH/2

	rl.DrawRectangle(0, 0, screenW, screenH, surfaceVeil)
	drawCard(panelX, panelY, panelW, panelH, surfacePrimary, borderSoft, borderStrong)

	drawHeading(assets.hudFont, "PAUSED", panelX+34, panelY+24, borderStrong)
	drawTextWithShadow(assets.hudFont, "MENU", float32(panelX+34), float32(panelY+50), 34, textPrimary)

	debugLabel := "Debug Overlay: Off"
	if g.DebugOverlay {
		debugLabel = "Debug Overlay: On"
	}
	if !DebugBuildEnabled {
		// In release builds DrawDebugOverlay is a no-op, so the toggle
		// flag has no visible effect. Label the row so the player knows
		// they need `go build -tags debug` for it to do anything.
		debugLabel += " (debug build only)"
	}
	drawMenuRow(assets.hudFont, "Restart", panelX+58, panelY+118, g.MenuIndex == int(core.PauseMenuRestart))
	drawMenuRow(assets.hudFont, debugLabel, panelX+58, panelY+170, g.MenuIndex == int(core.PauseMenuDebug))
	drawMenuRow(assets.hudFont, "Quit", panelX+58, panelY+222, g.MenuIndex == int(core.PauseMenuQuit))
}

// drawMenuRow paints one entry in the pause menu: a selection panel +
// arrow marker when active, then the label. Heavier shadow (+2,+2) than the
// in-game HUD's drawTextWithShadow because the menu text sits over an unbusy
// veil at a larger size and reads better with weight.
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
	drawTextWithShadowStyle(font, text, float32(x+12), float32(y), 26, textPrimary, shadowMid, 2, 2)
}

package render

import (
	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// pauseMenuRow binds a PauseMenuItem to a label producer. The row order in
// pauseMenuRows is the draw order; the Item field is the enum the input
// layer compares against when the cursor confirms. Keeping the binding
// here (instead of indexing into a parallel labels slice) means
// reordering rows is a single edit and adding a row is a single appended
// struct — no risk of the render's row at index 2 firing the input
// layer's enum value 3.
type pauseMenuRow struct {
	Item  core.PauseMenuItem
	Label func(g core.GameState) string
}

var pauseMenuRows = []pauseMenuRow{
	{Item: core.PauseMenuRestart, Label: func(core.GameState) string { return "Restart" }},
	{Item: core.PauseMenuStats, Label: func(core.GameState) string { return "Party Stats" }},
	{Item: core.PauseMenuDebug, Label: debugRowLabel},
	{Item: core.PauseMenuDisplay, Label: func(core.GameState) string { return DisplayMenuRowLabel() }},
	{Item: core.PauseMenuJukebox, Label: func(core.GameState) string { return JukeboxRowLabel() }},
	{Item: core.PauseMenuQuit, Label: func(core.GameState) string { return "Quit" }},
}

func debugRowLabel(g core.GameState) string {
	if g.DebugOverlay {
		return "Debug Overlay: On"
	}
	return "Debug Overlay: Off"
}

// Pause menu layout. Panel is centered on screen; rows stack at a fixed
// stride below a header band. inner row width is derived from the panel
// width so the highlight rectangle resizes with the panel automatically.
const (
	pauseMenuPanelW      = int32(420)
	pauseMenuHeaderH     = int32(118) // top of panel → first row baseline
	pauseMenuRowH        = int32(40)  // selection-highlight rect height
	pauseMenuRowGap      = int32(12)  // gap between rows (stride = rowH + rowGap)
	pauseMenuFootH       = int32(18)  // bottom pad below last row
	pauseMenuRowInsetX   = int32(58)  // distance from panel left edge to row label x
	pauseMenuRowRightPad = int32(46)  // distance from row right edge to panel right edge
)

func pauseMenuPanelH() int32 {
	stride := pauseMenuRowH + pauseMenuRowGap
	return pauseMenuHeaderH + stride*int32(len(pauseMenuRows)) + pauseMenuFootH
}

// pauseMenuRowInnerW is the highlight-rectangle width, derived from the
// panel width minus the symmetric left/right gutters. Was previously a
// hardcoded 316; this expression keeps the rectangle aligned if the panel
// resizes.
func pauseMenuRowInnerW() int32 {
	return pauseMenuPanelW - pauseMenuRowInsetX - pauseMenuRowRightPad
}

func drawMenuOverlay(g core.GameState, assets Resources) {
	screenW, screenH := screenSize()
	panelW := pauseMenuPanelW
	panelH := pauseMenuPanelH()
	panelX := centerX(panelW)
	panelY := screenH/2 - panelH/2

	rl.DrawRectangle(0, 0, screenW, screenH, surfaceVeil)
	drawCard(panelX, panelY, panelW, panelH, surfacePrimary, borderSoft, borderStrong)

	drawHeading(assets.hudFont, "PAUSED", panelX+34, panelY+24, borderStrong)
	drawTextWithShadow(assets.hudFont, "MENU", float32(panelX+34), float32(panelY+50), 34, textPrimary)

	stride := pauseMenuRowH + pauseMenuRowGap
	rowY := pauseMenuHeaderH
	rowX := panelX + pauseMenuRowInsetX
	for _, row := range pauseMenuRows {
		drawMenuRow(assets.hudFont, row.Label(g), rowX, panelY+rowY, g.MenuIndex == int(row.Item))
		rowY += stride
	}
}

// drawMenuRow paints one entry in the pause menu: a selection panel +
// arrow marker when active, then the label. Heavier shadow (+2,+2) than the
// in-game HUD's drawTextWithShadow because the menu text sits over an unbusy
// veil at a larger size and reads better with weight.
func drawMenuRow(font rl.Font, text string, x, y int32, selected bool) {
	if selected {
		DrawSelectedRowI(x-18, y-6, pauseMenuRowInnerW(), pauseMenuRowH)
		centerY := y - 6 + pauseMenuRowH/2
		drawArrowMarker(rl.NewVector2(float32(x-16), float32(centerY)), 9, 0, 9, borderActive)
	}
	drawTextWithShadowStyle(font, text, float32(x+12), float32(y), 26, textPrimary, shadowMid, 2, 2)
}

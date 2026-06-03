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
	{Item: core.PauseMenuOptions, Label: func(core.GameState) string { return "Options ▸" }},
	{Item: core.PauseMenuDebug, Label: func(core.GameState) string { return "Debug ▸" }},
	{Item: core.PauseMenuQuit, Label: func(core.GameState) string { return "Quit" }},
}

// optionsMenuRow / debugMenuRow bind a submenu item enum to its label
// producer — same shape as pauseMenuRow so every menu reuses drawMenuRow
// / drawTitledMenuCard layout without a second renderer. The ▸ on the
// pause-menu rows above marks "descends into a submenu."
type optionsMenuRow struct {
	Item  core.OptionsMenuItem
	Label func(g core.GameState) string
}

var optionsMenuRows = []optionsMenuRow{
	{Item: core.OptionsMenuDisplay, Label: func(core.GameState) string { return DisplayMenuRowLabel() }},
	{Item: core.OptionsMenuStats, Label: func(core.GameState) string { return "Party Stats" }},
	{Item: core.OptionsMenuQuests, Label: func(core.GameState) string { return "Quests" }},
	{Item: core.OptionsMenuSave, Label: func(core.GameState) string { return "Save Game" }},
	{Item: core.OptionsMenuRestart, Label: func(core.GameState) string { return "Restart" }},
	{Item: core.OptionsMenuClose, Label: func(core.GameState) string { return "Close" }},
}

type debugMenuRow struct {
	Item  core.DebugMenuItem
	Label func(g core.GameState) string
}

var debugMenuRows = []debugMenuRow{
	{Item: core.DebugMenuToggle, Label: func(g core.GameState) string {
		return "Debug Mode: " + onOff(g.DebugOverlay)
	}},
	{Item: core.DebugMenuEnemies, Label: func(g core.GameState) string {
		return "Enemies: " + onOff(!g.EnemiesDisabled)
	}},
	{Item: core.DebugMenuAdvanceTime, Label: func(g core.GameState) string {
		phase, _ := core.PhaseAtStep(g.StepCount)
		return "Advance Time (" + core.PhaseName(phase) + ")"
	}},
	{Item: core.DebugMenuEasyQuit, Label: func(g core.GameState) string {
		if g.EasyBattleQuit {
			return "Easy Battle Quit: On (Bksp)"
		}
		return "Easy Battle Quit: Off"
	}},
	{Item: core.DebugMenuRenderLog, Label: func(g core.GameState) string {
		return "Render Log: " + onOff(g.RenderLogEnabled)
	}},
	{Item: core.DebugMenuJukebox, Label: func(core.GameState) string { return JukeboxRowLabel() }},
	{Item: core.DebugMenuClose, Label: func(core.GameState) string { return "Close" }},
}

func onOff(b bool) string {
	if b {
		return "On"
	}
	return "Off"
}

// drawOptionsMenuOverlay paints the Options submenu via the shared
// menu-card chrome, titled "OPTIONS".
func drawOptionsMenuOverlay(g core.GameState, assets Resources) {
	drawTitledMenuCard(assets, "OPTIONS", len(optionsMenuRows),
		func(i int) string { return optionsMenuRows[i].Label(g) },
		func(i int) bool { return g.OptionsMenuIndex == int(optionsMenuRows[i].Item) })
}

// drawDebugMenuOverlay paints the debug submenu via the shared menu-card
// chrome, titled "DEBUG". The card height tracks the debug row count.
func drawDebugMenuOverlay(g core.GameState, assets Resources) {
	drawTitledMenuCard(assets, "DEBUG", len(debugMenuRows),
		func(i int) string { return debugMenuRows[i].Label(g) },
		func(i int) bool { return g.DebugMenuIndex == int(debugMenuRows[i].Item) })
}

// Pause menu layout. Panel is centered on screen; rows stack at a fixed
// stride below a header band. inner row width is derived from the panel
// width so the highlight rectangle resizes with the panel automatically.
//
// pauseMenuHeaderH shrank when the redundant "PAUSED" tick was dropped —
// the centred "MENU" title (FontTitle) is the only header now.
const (
	pauseMenuPanelW      = int32(420)
	pauseMenuHeaderH     = int32(90)
	pauseMenuRowH        = int32(40)
	pauseMenuRowGap      = int32(12)
	pauseMenuFootH       = int32(20)
	pauseMenuRowInsetX   = int32(58)
	pauseMenuRowRightPad = int32(46)
)

// pauseMenuRowInnerW is the highlight-rectangle width, derived from the
// panel width minus the symmetric left/right gutters. Was previously a
// hardcoded 316; this expression keeps the rectangle aligned if the panel
// resizes.
func pauseMenuRowInnerW() int32 {
	return pauseMenuPanelW - pauseMenuRowInsetX - pauseMenuRowRightPad
}

// drawTitledMenuCard paints the shared pause/debug menu chrome: the veil,
// the gilt-framed card sized to rowCount, the centred FontTitle title with
// flanking fleurons, and each row via drawMenuRow. selected reports whether
// a given row index is the cursor. Returns nothing — the pause and debug
// overlays differ only in title string, row labels, and the cursor field,
// which this captures via the closures.
func drawTitledMenuCard(assets Resources, title string, rowCount int, label func(i int) string, selected func(i int) bool) {
	panelW := pauseMenuPanelW
	stride := pauseMenuRowH + pauseMenuRowGap
	panelH := pauseMenuHeaderH + stride*int32(rowCount) + pauseMenuFootH
	rect := drawVeiledCard(panelW, panelH, borderSoft, borderSoft, giltDim)
	panelX := int32(rect.X)
	panelY := int32(rect.Y)

	// Centred title near the top of the card with flanking gilt fleurons
	// on the title's vertical midline (~22px outside each text edge) — the
	// 90s grimoire feel: ◆──── TITLE ────◆.
	titleMeasure := rl.MeasureTextEx(assets.hudFont, title, FontTitle, FontSpacingTitle)
	titleX := float32(panelX) + float32(panelW)/2 - titleMeasure.X/2
	titleY := float32(panelY + 24)
	drawTextWithShadowStyle(assets.hudFont, title, titleX, titleY,
		FontTitle, FontSpacingTitle, textPrimary, shadowStrong, 1, 1)
	flCY := titleY + titleMeasure.Y/2
	drawFleuronsFlanking(titleX, titleMeasure.X, 22, flCY, 5, giltDim)

	rowY := pauseMenuHeaderH
	rowX := panelX + pauseMenuRowInsetX
	for i := 0; i < rowCount; i++ {
		drawMenuRow(assets.hudFont, label(i), rowX, panelY+rowY, selected(i))
		rowY += stride
	}
}

// drawTitledCardHeader draws a centered veiled overlay card with a
// fleuron-flanked FontTitle title, and returns the card's top-left origin
// plus the Y just below the title (where a subtitle / first row starts).
// Shared by the shop and quest-journal overlays — those need a custom body
// (two-column rows, tab header) so they can't use drawTitledMenuCard, but
// the card-chrome + title preamble is identical, so it lives here once.
func drawTitledCardHeader(assets Resources, title string, panelW, panelH int32) (panelX, panelY int32, belowTitleY float32) {
	font := assets.hudFont
	rect := drawVeiledCard(panelW, panelH, borderSoft, borderSoft, giltDim)
	panelX = int32(rect.X)
	panelY = int32(rect.Y)
	tm := rl.MeasureTextEx(font, title, FontTitle, FontSpacingTitle)
	titleX := float32(panelX) + float32(panelW)/2 - tm.X/2
	titleY := float32(panelY + 18)
	drawTextWithShadowStyle(font, title, titleX, titleY, FontTitle, FontSpacingTitle, textPrimary, shadowStrong, 1, 1)
	drawFleuronsFlanking(titleX, tm.X, 22, titleY+tm.Y/2, 5, giltDim)
	return panelX, panelY, titleY + tm.Y
}

func drawMenuOverlay(g core.GameState, assets Resources) {
	drawTitledMenuCard(assets, "MENU", len(pauseMenuRows),
		func(i int) string { return pauseMenuRows[i].Label(g) },
		func(i int) bool { return g.MenuIndex == int(pauseMenuRows[i].Item) })
}

// drawMenuRow paints one entry in the pause menu: the canonical
// gilt-spine + underline selection panel from DrawSelectedRowI when
// active, then the label. Heavier shadow (+2,+2) than the in-game
// HUD's drawTextWithShadow because the menu text sits over an
// unbusy veil at a larger size and reads better with weight.
//
// Earlier passes added an arrow marker beside the spine; removed
// because the spine + gilt underline already names the selection
// per UI_STANDARDS.md "Row > Selected." The arrow was the OLD
// selection cue and visually competed with the new spine when
// both were painted.
func drawMenuRow(font rl.Font, text string, x, y int32, selected bool) {
	if selected {
		DrawSelectedRowI(x-18, y-6, pauseMenuRowInnerW(), pauseMenuRowH)
	}
	drawTextWithShadowStyle(font, text, float32(x+12), float32(y), FontHeading, FontSpacingHeading, textPrimary, shadowMid, 2, 2)
}

package render

import (
	"fmt"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// pauseMenuRow binds a PauseMenuItem to a label producer. Row order is draw
// order; Item is the enum the input layer matches on confirm — binding both here
// (not a parallel labels slice) keeps render row and input enum from desyncing.
type pauseMenuRow struct {
	Item  core.PauseMenuItem
	Label func(g *core.GameState) string
}

var pauseMenuRows = []pauseMenuRow{
	{Item: core.PauseMenuOptions, Label: func(*core.GameState) string { return "Options ▸" }},
	{Item: core.PauseMenuDebug, Label: func(*core.GameState) string { return "Debug ▸" }},
	{Item: core.PauseMenuQuit, Label: func(*core.GameState) string { return "Quit" }},
}

// optionsMenuRow / debugMenuRow are the submenu twins of pauseMenuRow, so every
// menu reuses drawMenuRow / drawTitledMenuCard. The ▸ marks "descends into a submenu."
type optionsMenuRow struct {
	Item  core.OptionsMenuItem
	Label func(g *core.GameState) string
}

var optionsMenuRows = []optionsMenuRow{
	{Item: core.OptionsMenuDisplay, Label: func(*core.GameState) string { return DisplayMenuRowLabel() }},
	{Item: core.OptionsMenuVibration, Label: func(g *core.GameState) string { return "Vibration: " + onOff(g.RumbleEnabled) }},
	{Item: core.OptionsMenuSave, Label: func(*core.GameState) string { return "Save Game" }},
	{Item: core.OptionsMenuRestart, Label: func(*core.GameState) string { return "Restart" }},
	{Item: core.OptionsMenuClose, Label: func(*core.GameState) string { return "Close" }},
}

type debugMenuRow struct {
	Item  core.DebugMenuItem
	Label func(g *core.GameState) string
}

var debugMenuRows = []debugMenuRow{
	{Item: core.DebugMenuToggle, Label: func(g *core.GameState) string {
		return "Debug Mode: " + onOff(g.DebugOverlay)
	}},
	{Item: core.DebugMenuEnemies, Label: func(g *core.GameState) string {
		return "Enemies: " + onOff(!g.EnemiesDisabled)
	}},
	{Item: core.DebugMenuAdvanceTime, Label: func(g *core.GameState) string {
		phase, _ := core.PhaseAtStep(g.StepCount)
		return "Advance Time (" + core.PhaseName(phase) + ")"
	}},
	{Item: core.DebugMenuEasyQuit, Label: func(g *core.GameState) string {
		// Controller-first cue: in-battle quit is Select/Share, not a key name.
		if g.EasyBattleQuit {
			return "Easy Battle Quit: On (Select)"
		}
		return "Easy Battle Quit: Off"
	}},
	{Item: core.DebugMenuRenderLog, Label: func(g *core.GameState) string {
		return "Render Log: " + onOff(g.RenderLogEnabled)
	}},
	{Item: core.DebugMenuJukebox, Label: func(*core.GameState) string { return JukeboxRowLabel() }},
	{Item: core.DebugMenuAllSkills, Label: func(g *core.GameState) string {
		return "All Skills: " + onOff(g.DebugAllSkills)
	}},
	{Item: core.DebugMenuBoostStats, Label: func(*core.GameState) string {
		return fmt.Sprintf("Boost Stats (+%d)", core.DebugStatBoost)
	}},
	{Item: core.DebugMenuSkipBattles, Label: func(g *core.GameState) string {
		return "Skip Battles: " + onOff(g.DebugSkipBattles)
	}},
	{Item: core.DebugMenuTestRumble, Label: func(*core.GameState) string { return "Test Rumble" }},
	{Item: core.DebugMenuRetro, Label: func(g *core.GameState) string {
		if core.AnyRetroFilterActive(&g.RetroFilters) {
			return "Retro Filters ▸ (On)"
		}
		return "Retro Filters ▸"
	}},
	{Item: core.DebugMenuStartDialog, Label: func(*core.GameState) string { return "Start Dialog" }},
	{Item: core.DebugMenuCombatTune, Label: func(*core.GameState) string { return "Combat Tuning ▸" }},
	{Item: core.DebugMenuClose, Label: func(*core.GameState) string { return "Close" }},
}

// retroMenuRowLabel formats one Retro Filters submenu row as PLAIN TEXT (the
// adjust affordance is primitive-drawn by drawRetroMenuOverlay, not font glyphs).
// A function, not a row table, because the row set is positional: the first
// RetroFilterCount cursor slots ARE the filter kinds.
func retroMenuRowLabel(g *core.GameState, i int) string {
	switch {
	case i < int(core.RetroFilterCount):
		name := core.RetroFilterName(core.RetroFilterKind(i))
		v := g.RetroFilters[i]
		if v <= 0 {
			return name + ": Off"
		}
		return fmt.Sprintf("%s: %.0f%%", name, v*100)
	case i == core.RetroMenuSkyToggle:
		return "Filter Skybox: " + onOff(g.RetroFilterSky)
	case i == core.RetroMenuSpriteToggle:
		return "Filter Sprites: " + onOff(g.RetroFilterSprites)
	case i == core.RetroMenuResetAll:
		return "Reset to Default"
	case i == core.RetroMenuAllOff:
		return "All Off"
	default:
		return "Close"
	}
}

// init asserts each row slice has exactly one row per enum value. Rows are
// matched to the cursor by .Item, not slice index, so drift would silently
// mis-pair a row rather than crash; this catches it at startup.
func init() {
	if len(pauseMenuRows) != core.PauseMenuCount {
		panic(fmt.Sprintf("pauseMenuRows length %d != PauseMenuCount %d", len(pauseMenuRows), core.PauseMenuCount))
	}
	if len(optionsMenuRows) != core.OptionsMenuCount {
		panic(fmt.Sprintf("optionsMenuRows length %d != OptionsMenuCount %d", len(optionsMenuRows), core.OptionsMenuCount))
	}
	if len(debugMenuRows) != core.DebugMenuCount {
		panic(fmt.Sprintf("debugMenuRows length %d != DebugMenuCount %d", len(debugMenuRows), core.DebugMenuCount))
	}
	// retroMenuRowLabel is a positional switch: RetroFilterCount filter slots, then
	// 5 trailing slots (SkyToggle/SpriteToggle/ResetAll/AllOff/Close). Assert the
	// count so a new RetroMenu* value trips here instead of rendering as "Close".
	if core.RetroMenuCount != int(core.RetroFilterCount)+5 {
		panic(fmt.Sprintf("retroMenuRowLabel switch handles RetroFilterCount+5 slots but RetroMenuCount is %d (RetroFilterCount %d)", core.RetroMenuCount, core.RetroFilterCount))
	}
}

func onOff(b bool) string {
	if b {
		return "On"
	}
	return "Off"
}

// OnOffLabel is the exported twin of onOff for callers outside render.
func OnOffLabel(b bool) string { return onOff(b) }

// drawOptionsMenuOverlay paints the Options submenu via the shared menu-card chrome.
func drawOptionsMenuOverlay(g *core.GameState, assets Resources) {
	drawTitledMenuCard(assets, "OPTIONS", pauseMenuPanelW, len(optionsMenuRows),
		func(i int) string { return optionsMenuRows[i].Label(g) },
		func(i int) bool { return g.OptionsMenuIndex == int(optionsMenuRows[i].Item) })
}

// drawDebugMenuOverlay paints the debug submenu via the shared menu-card chrome.
func drawDebugMenuOverlay(g *core.GameState, assets Resources) {
	drawTitledMenuCard(assets, "DEBUG", pauseMenuPanelW, len(debugMenuRows),
		func(i int) string { return debugMenuRows[i].Label(g) },
		func(i int) bool { return g.DebugMenuIndex == int(debugMenuRows[i].Item) })
}

// Retro-menu slider chrome geometry: the intensity bar at each filter row's
// right edge + the adjust arrows on the cursored row. All primitive-drawn, no glyphs.
const (
	retroBarW      = int32(74)
	retroBarH      = int32(10)
	retroBarTextDY = int32(13) // bar's vertical center below the row's text top
	retroArrowGap  = float32(13)
)

// drawRetroMenuOverlay paints the Retro Filters submenu. Rows are positional
// (cursor slot == filter kind). On the shared card it draws each slider row's
// intensity gauge and, on the cursored row, the Left/Right adjust arrows.
func drawRetroMenuOverlay(g *core.GameState, assets Resources) {
	panelX, panelY := drawTitledMenuCard(assets, "RETRO FILTERS", retroMenuPanelW, core.RetroMenuCount,
		func(i int) string { return retroMenuRowLabel(g, i) },
		func(i int) bool { return g.RetroMenuIndex == i })

	rowX := panelX + pauseMenuRowInsetX
	barX := rowX + menuRowInnerW(retroMenuPanelW) - retroBarW - 14
	flick := candleFlicker()
	for i := 0; i < int(core.RetroFilterCount); i++ {
		rowTextY := menuRowTop(panelY, i)
		barY := rowTextY + retroBarTextDY - retroBarH/2
		v := g.RetroFilters[i]
		// Track + fill via the shared drawSmallPanel gauge primitives (same
		// glass-tube language as the HP gauges, at chip scale). Fill is gilt.
		drawSmallPanel(barX, barY, retroBarW, retroBarH, barTrack)
		if v > 0 {
			fillW := int32(float64(retroBarW-2) * v)
			if fillW > 0 {
				rl.DrawRectangle(barX+1, barY+1, fillW, retroBarH-2, fadeColor(giltBright, 0.55+0.45*float32(v)))
			}
		}
		drawSmallPanelOutline(barX, barY, retroBarW, retroBarH, fadeColor(woodLight, 0.6))
		if g.RetroMenuIndex == i {
			// Drawn ◂ ▸ affordance — triangles, not font runes.
			cy := float32(barY + retroBarH/2)
			col := fadeColor(giltBright, 0.65+0.35*flick)
			drawArrowMarker(rl.NewVector2(float32(barX)-retroArrowGap, cy), -7, 0, 6, col)
			drawArrowMarker(rl.NewVector2(float32(barX+retroBarW)+retroArrowGap, cy), 7, 0, 6, col)
		}
	}
}

// Pause menu layout: centered panel, rows at a fixed stride below a header band;
// inner row width derives from panel width so the highlight resizes with it.
const (
	pauseMenuPanelW      = int32(420)
	pauseMenuHeaderH     = int32(90)
	pauseMenuRowH        = int32(40)
	pauseMenuRowGap      = int32(12)
	pauseMenuFootH       = int32(20)
	pauseMenuRowInsetX   = int32(58)
	pauseMenuRowRightPad = int32(46)
)

// menuRowInnerW is the highlight width for a card of panelW (panel minus
// symmetric gutters). Parameterized so the wider retro-filters card fits.
func menuRowInnerW(panelW int32) int32 {
	return panelW - pauseMenuRowInsetX - pauseMenuRowRightPad
}

// menuRowStride is the row pitch; menuRowTop returns a row's top-Y for a card at
// panelY. Both the row loop and the retro overlay's bar placement go through
// menuRowTop so the bars can't drift from the rows.
func menuRowStride() int32 { return pauseMenuRowH + pauseMenuRowGap }
func menuRowTop(panelY int32, i int) int32 {
	return panelY + pauseMenuHeaderH + menuRowStride()*int32(i)
}

// retroMenuPanelW is the Retro Filters card width — wider than the pause card so
// the longest label + its intensity bar fit on one row.
const retroMenuPanelW = int32(560)

// drawCardTitle paints the centred FontTitle title flanked by gilt fleurons (the
// grimoire ◆──── TITLE ────◆ look). Shared by drawTitledMenuCard and
// drawTitledCardHeader, differing only by topInset. Returns the Y below the title.
// cardTitleMeasureCache memoizes the constant card-title widths so centring isn't
// a per-frame cgo round-trip.
var cardTitleMeasureCache measureCache

func drawCardTitle(font rl.Font, title string, panelX, panelY, panelW int32, topInset float32) (belowTitleY float32) {
	tm := cardTitleMeasureCache.measure(font, title, FontTitle, FontSpacingTitle)
	titleX := float32(panelX) + float32(panelW)/2 - tm.X/2
	titleY := float32(panelY) + topInset
	// drawEngravedText tracks at canonicalSpacing(FontTitle) == FontSpacingTitle,
	// so the centering above stays exact.
	drawEngravedText(font, title, titleX, titleY, FontTitle, textPrimary)
	drawFleuronsFlanking(titleX, tm.X, 22, titleY+tm.Y/2, 5, giltDim)
	return titleY + tm.Y
}

// drawTitledMenuCard paints the shared pause/debug menu chrome: veil, gilt card
// sized to rowCount, fleuron-flanked title, and each row via drawMenuRow. The
// overlays differ only in title/labels/cursor, captured via the closures.
func drawTitledMenuCard(assets Resources, title string, panelW int32, rowCount int, label func(i int) string, selected func(i int) bool) (panelX, panelY int32) {
	panelH := pauseMenuHeaderH + menuRowStride()*int32(rowCount) + pauseMenuFootH
	rect := drawVeiledCard(panelW, panelH, borderSoft, borderSoft, giltDim)
	panelX = int32(rect.X)
	panelY = int32(rect.Y)

	drawCardTitle(assets.hudFont, title, panelX, panelY, panelW, 24)

	rowInnerW := menuRowInnerW(panelW)
	rowX := panelX + pauseMenuRowInsetX
	for i := 0; i < rowCount; i++ {
		drawMenuRow(assets.hudFont, label(i), rowX, menuRowTop(panelY, i), rowInnerW, selected(i))
	}
	// Geometry returned so a caller can overlay row decorations (retro bars/
	// arrows) without re-deriving and drifting from the rows.
	return panelX, panelY
}

// drawTitledCardHeader draws a veiled card with a fleuron-flanked title and
// returns its top-left origin + the Y below the title. For the shop overlay,
// which needs a custom body but shares the card-chrome + title preamble.
func drawTitledCardHeader(assets Resources, title string, panelW, panelH int32) (panelX, panelY int32, belowTitleY float32) {
	rect := drawVeiledCard(panelW, panelH, borderSoft, borderSoft, giltDim)
	panelX = int32(rect.X)
	panelY = int32(rect.Y)
	belowTitleY = drawCardTitle(assets.hudFont, title, panelX, panelY, panelW, 18)
	return panelX, panelY, belowTitleY
}

func drawMenuOverlay(g *core.GameState, assets Resources) {
	drawTitledMenuCard(assets, "MENU", pauseMenuPanelW, len(pauseMenuRows),
		func(i int) string { return pauseMenuRows[i].Label(g) },
		func(i int) bool { return g.MenuIndex == int(pauseMenuRows[i].Item) })
}

// drawMenuRow paints one pause-menu entry: the gilt-spine + underline selection
// panel (DrawSelectedRowI) when active, then the label.
func drawMenuRow(font rl.Font, text string, x, y, innerW int32, selected bool) {
	if selected {
		DrawSelectedRowI(x-menuRowInsetX, y-menuRowInsetY, innerW, pauseMenuRowH)
	}
	drawEngravedText(font, text, float32(x+12), float32(y), FontHeading, textPrimary)
}

// Quit-confirm card layout — a lighter, fleuron-free confirm than the titled
// menu chrome, with its own band so it tunes independently.
const (
	quitConfirmHeaderInsetY = float32(22)
	quitConfirmBodyInsetY   = float32(78)
	quitConfirmFooterInsetY = float32(40)
)

// DrawQuitConfirm paints the quit-confirm modal (g.QuitConfirmOpen): engraved
// title, warning body, controller-first Quit/Cancel hints. No-op when closed.
// Drawn last in the explore overlay pass (highest-priority modal).
func DrawQuitConfirm(g *core.GameState, assets Resources) {
	if !g.QuitConfirmOpen {
		return
	}
	panelW := int32(440)
	panelH := int32(168)
	rect := drawVeiledCard(panelW, panelH, borderSoft, borderSoft, giltDim)
	panelX := float32(rect.X)
	panelY := float32(rect.Y)

	title := "QUIT GAME"
	// Canonical heading spacing so centering matches drawEngravedText.
	tm := cardTitleMeasureCache.measure(assets.hudFont, title, FontHeading, FontSpacingHeading)
	drawEngravedText(assets.hudFont, title,
		panelX+float32(panelW)/2-tm.X/2, panelY+quitConfirmHeaderInsetY,
		FontHeading, textPrimary)

	cardCenterX := panelX + float32(panelW)/2
	drawTextCentered(assets.hudFont, "Unsaved progress will be lost.",
		cardCenterX, panelY+quitConfirmBodyInsetY, FontBody, textMuted)
	// Controller-first affordances (no spelled-out keys).
	DrawHintBar(assets.hudFont, []HintSeg{
		Hint("Quit", GlyphA),
		Hint("Cancel", GlyphB),
	}, cardCenterX, panelY+float32(panelH)-quitConfirmFooterInsetY, FontSmall)
}

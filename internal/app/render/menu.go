package render

import (
	"fmt"

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
	Label func(g *core.GameState) string
}

var pauseMenuRows = []pauseMenuRow{
	{Item: core.PauseMenuOptions, Label: func(*core.GameState) string { return "Options ▸" }},
	{Item: core.PauseMenuDebug, Label: func(*core.GameState) string { return "Debug ▸" }},
	{Item: core.PauseMenuQuit, Label: func(*core.GameState) string { return "Quit" }},
}

// optionsMenuRow / debugMenuRow bind a submenu item enum to its label
// producer — same shape as pauseMenuRow so every menu reuses drawMenuRow
// / drawTitledMenuCard layout without a second renderer. The ▸ on the
// pause-menu rows above marks "descends into a submenu."
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
		// Controller-first cue (gamepad-first contract): the in-battle quit is
		// Select/Share (input.DebugFleePressed, also Backspace), not a key name.
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
	{Item: core.DebugMenuClose, Label: func(*core.GameState) string { return "Close" }},
}

// retroMenuRowLabel formats one Retro Filters submenu row. Slider rows show
// the filter name + its live intensity as PLAIN TEXT ("Pixelate: 60%", or
// "Off") — the adjust affordance (intensity bar + left/right arrows) is
// DRAWN with primitives by drawRetroMenuOverlay, never font glyphs, so it
// can't fall out of the atlas and render as "?". Kept as a function (not a
// row table) because the row set is positional — the first RetroFilterCount
// cursor slots ARE the filter kinds, per core's contract.
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
	case i == core.RetroMenuResetAll:
		return "Reset to Default"
	case i == core.RetroMenuAllOff:
		return "All Off"
	default:
		return "Close"
	}
}

// init asserts each pause/submenu row slice has exactly one row per enum value
// (its wrap-modulus count). The rows are matched to the cursor by their .Item
// enum value, not by slice index, so a missing/extra/reordered row wouldn't
// crash — it would silently drop a row's highlight or mis-pair one. This
// length check catches that drift at startup, mirroring the registry-invariant
// asserts elsewhere (statTable, timingGrades, partyStatusVisuals, …).
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
	// retroMenuRowLabel is a positional switch (no slice to length-check): the
	// first RetroFilterCount slots are filter kinds, then SkyToggle / ResetAll /
	// AllOff, then Close via the default arm. This asserts the menu has exactly
	// those four trailing slots, so inserting a new RetroMenu* enum value bumps
	// RetroMenuCount and trips here at startup instead of silently rendering as
	// "Close" through the default arm. Mirrors the row-slice asserts above.
	if core.RetroMenuCount != int(core.RetroFilterCount)+4 {
		panic(fmt.Sprintf("retroMenuRowLabel switch handles RetroFilterCount+4 slots but RetroMenuCount is %d (RetroFilterCount %d)", core.RetroMenuCount, core.RetroFilterCount))
	}
}

func onOff(b bool) string {
	if b {
		return "On"
	}
	return "Off"
}

// OnOffLabel is the exported twin of onOff for callers outside the render
// package (e.g. the editor's dialog tooling) that need the same On/Off text.
func OnOffLabel(b bool) string { return onOff(b) }

// drawOptionsMenuOverlay paints the Options submenu via the shared
// menu-card chrome, titled "OPTIONS".
func drawOptionsMenuOverlay(g *core.GameState, assets Resources) {
	drawTitledMenuCard(assets, "OPTIONS", pauseMenuPanelW, len(optionsMenuRows),
		func(i int) string { return optionsMenuRows[i].Label(g) },
		func(i int) bool { return g.OptionsMenuIndex == int(optionsMenuRows[i].Item) })
}

// drawDebugMenuOverlay paints the debug submenu via the shared menu-card
// chrome, titled "DEBUG". The card height tracks the debug row count.
func drawDebugMenuOverlay(g *core.GameState, assets Resources) {
	drawTitledMenuCard(assets, "DEBUG", pauseMenuPanelW, len(debugMenuRows),
		func(i int) string { return debugMenuRows[i].Label(g) },
		func(i int) bool { return g.DebugMenuIndex == int(debugMenuRows[i].Item) })
}

// Retro-menu slider chrome geometry — the drawn intensity bar each filter
// row carries at its right edge, and the adjust arrows that flank the bar on
// the cursored row. ALL of it is primitive-drawn (rects + triangles), no
// font glyphs, per the "construct glyphs with pixels" rule.
const (
	retroBarW      = int32(74)
	retroBarH      = int32(10)
	retroBarTextDY = int32(13) // bar's vertical center below the row's text top
	retroArrowGap  = float32(13)
)

// drawRetroMenuOverlay paints the Retro Filters sub-submenu, titled "RETRO
// FILTERS". Rows are positional (cursor slot == filter kind), so the
// highlight matches the cursor index directly. On top of the shared card it
// draws each slider row's intensity gauge (track + gilt fill + bezel ticks)
// and, on the cursored slider row, candle-bright left/right arrow triangles
// — the Left/Right-to-adjust affordance.
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
		// Track + fill: the same dark-glass-tube language as the HP gauges,
		// at chip scale — routed through the shared drawSmallPanel /
		// drawSmallPanelOutline gauge primitives (rounded glass body + bevel +
		// rounded outline) rather than bare rectangles. Fill is gilt — "how
		// much of this dial is turned."
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

// menuRowInnerW is the highlight-rectangle width for a menu card of the
// given panel width — the panel minus the symmetric left/right gutters.
// Parameterized (was pauseMenuRowInnerW, fixed to pauseMenuPanelW) because
// the retro-filters card runs wider so its drawn intensity bars never
// collide with the longer slider labels.
func menuRowInnerW(panelW int32) int32 {
	return panelW - pauseMenuRowInsetX - pauseMenuRowRightPad
}

// menuRowStride is the vertical pitch between menu rows; menuRowTop returns a
// row's top-Y for a card whose top-left is at panelY. Both drawTitledMenuCard's
// row loop AND the retro overlay's per-row intensity-bar placement go through
// menuRowTop so the bar decorations can't drift from the rows (the retro
// overlay used to re-derive the stride and the row-Y formula inline).
func menuRowStride() int32 { return pauseMenuRowH + pauseMenuRowGap }
func menuRowTop(panelY int32, i int) int32 {
	return panelY + pauseMenuHeaderH + menuRowStride()*int32(i)
}

// retroMenuPanelW is the Retro Filters card width — wider than the standard
// pause card so "Chroma Fringe: 100%" plus the right-aligned intensity bar
// fit on one row without overlapping.
const retroMenuPanelW = int32(560)

// drawCardTitle paints the centred FontTitle card title with flanking gilt
// fleurons on its vertical midline (~22px outside each text edge) — the
// 90s grimoire ◆──── TITLE ────◆ look. The shared preamble extracted from
// drawTitledMenuCard and drawTitledCardHeader: both measured
// the title, centred it across panelW, shadow-drew it, and flanked it with
// fleurons, differing ONLY by the title's top inset (+24 vs +18) — now the
// `topInset` parameter. Returns the Y just below the title (titleY +
// measured height) so the shop header can place its subtitle row.
// cardTitleMeasureCache memoizes the (constant) card-title widths so the
// centring measure isn't a per-frame cgo round-trip for every open titled
// modal (pause/debug menu, shop header). Titles are fixed strings, so this is
// a near-permanent cache — mirrors the engraved-text / panel-heading caches.
var cardTitleMeasureCache measureCache

func drawCardTitle(font rl.Font, title string, panelX, panelY, panelW int32, topInset float32) (belowTitleY float32) {
	tm := cardTitleMeasureCache.measure(font, title, FontTitle, FontSpacingTitle)
	titleX := float32(panelX) + float32(panelW)/2 - tm.X/2
	titleY := float32(panelY) + topInset
	// Engraved title-tier lettering. Centering above stays exact:
	// drawEngravedText tracks at canonicalSpacing(FontTitle) == FontSpacingTitle.
	drawEngravedText(font, title, titleX, titleY, FontTitle, textPrimary)
	drawFleuronsFlanking(titleX, tm.X, 22, titleY+tm.Y/2, 5, giltDim)
	return titleY + tm.Y
}

// drawTitledMenuCard paints the shared pause/debug menu chrome: the veil,
// the gilt-framed card sized to rowCount, the centred FontTitle title with
// flanking fleurons, and each row via drawMenuRow. selected reports whether
// a given row index is the cursor. Returns nothing — the pause and debug
// overlays differ only in title string, row labels, and the cursor field,
// which this captures via the closures.
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
	// Geometry returned so a caller can overlay row decorations (the retro
	// menu's drawn intensity bars / adjust arrows) without re-deriving —
	// and without the overlay drifting from the rows if this layout changes.
	return panelX, panelY
}

// drawTitledCardHeader draws a centered veiled overlay card with a
// fleuron-flanked FontTitle title, and returns the card's top-left origin
// plus the Y just below the title (where a subtitle / first row starts).
// Used by the shop overlay — it needs a custom body (two-column rows, tab
// header) so it can't use drawTitledMenuCard, but the card-chrome + title
// preamble is identical, so it lives here once (via drawCardTitle).
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
func drawMenuRow(font rl.Font, text string, x, y, innerW int32, selected bool) {
	if selected {
		DrawSelectedRowI(x-menuRowInsetX, y-menuRowInsetY, innerW, pauseMenuRowH)
	}
	// Engraved heading-tier rows — drawEngravedText's own +2 drop shadow
	// supplies the weight the old hand-set (+2,+2) shadow carried.
	drawEngravedText(font, text, float32(x+12), float32(y), FontHeading, textPrimary)
}

// Quit-confirm card layout — same veiled-card shape as the door prompt (a
// lighter, fleuron-free confirm than the titled menu chrome), with its own band
// so the two confirms can be tuned independently.
const (
	quitConfirmHeaderInsetY = float32(22)
	quitConfirmBodyInsetY   = float32(78)
	quitConfirmFooterInsetY = float32(40)
)

// DrawQuitConfirm paints the "Quit — unsaved progress will be lost?" confirm
// modal (g.QuitConfirmOpen). Centered glass card with an engraved title, the
// warning body, and controller-first Quit/Cancel hints. No-op when the prompt
// isn't open. Drawn last in the explore overlay pass (highest-priority modal).
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
	// Cache the constant title's measure (drawn every frame the prompt is open)
	// at the canonical heading spacing so centering matches drawEngravedText.
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

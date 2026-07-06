package editor

import (
	"crawler/internal/app/core"
	"crawler/internal/app/core/mapfile"
	"crawler/internal/app/render"
	"fmt"
	"image/color"
	"math"
	"strconv"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// glyphStr maps a byte to its one-char string. Built once so drawTileGlyph
// doesn't allocate per visible tile per frame (thousands/frame zoomed out).
var glyphStr = func() [256]string {
	var t [256]string
	for i := range t {
		t[i] = string([]byte{byte(i)})
	}
	return t
}()

// gridTickStride is the grid-coordinate interval the axis ticks are labeled at
// (every Nth cell). The label table, its index math, and the values all derive
// from it, so retuning the tick density is one edit.
const gridTickStride = 5

// tickLabels holds pre-formatted labels for every gridTickStride'th grid coordinate.
// Built once so the axis-tick draw indexes coord/gridTickStride instead of
// Sprintf'ing each frame.
var tickLabels = func() []string {
	t := make([]string, core.MaxMapDimension/gridTickStride+2)
	for i := range t {
		t[i] = strconv.Itoa(i * gridTickStride)
	}
	return t
}()

// tickLabel returns the pre-formatted label for coordinate c, formatting fresh
// if c lands past the table.
func tickLabel(c int) string {
	if i := c / gridTickStride; i >= 0 && i < len(tickLabels) {
		return tickLabels[i]
	}
	return strconv.Itoa(c)
}

const (
	topbarH = float32(48)
	// menuBarBtnY/H are the menu-bar strip's vertical inset + height, shared by
	// the draw, hit-test, dropdown re-open hit-test, and pull-down anchor so they
	// can't drift (else the open dropdown detaches from its button).
	menuBarBtnY = float32(6)
	menuBarBtnH = topbarH - 12
	toolbarH    = float32(38) // action button row beneath the topbar
	paletteW    = float32(220)
	metadataW   = float32(360)
	gridMargin  = float32(8)
	layerTabH   = float32(32)
	// layerSelectH is the left-column strip that holds the active-layer dropdown,
	// sitting directly above the Levels panel. Tall so the big color-coded label reads.
	layerSelectH = float32(54)
)

// Entity-list (chest/pack contents) geometry, shared by entityModalLayoutFor
// and drawEntityListWindow so painted rows and hit-rects can't drift.
const (
	entityListTop  = float32(52)
	entityListRowH = float32(22)
	// entityListTextInset: row text start — modalContentInset plus room for "> ".
	entityListTextInset = modalContentInset + 8
)

// layout recomputes screen rectangles each frame from the window size. Cell px
// is the auto-fit size scaled by s.zoom; pan offsets drag the plot off-center.
func (s *State) layout() {
	w, h := render.ScreenSizeF()

	s.rect.topbar = rl.NewRectangle(0, 0, w, topbarH)
	// Action toolbar beneath the menu bar; everything below derives from contentTop.
	s.rect.toolbar = rl.NewRectangle(0, topbarH, w, toolbarH)
	contentTop := topbarH + toolbarH
	// Layer select sits in the left column directly above the Levels panel.
	tabsHeight := layerSelectH
	s.rect.layerTabs = rl.NewRectangle(0, contentTop, paletteW, tabsHeight)
	// Levels panel: a header row (label + −/+) then one row per level 0..topLevel
	// (capped to maxVisibleLevelRows).
	levelsY := contentTop + tabsHeight
	s.rect.levels = rl.NewRectangle(0, levelsY, paletteW, levelsPanelHeight(s))
	paletteY := levelsY + s.rect.levels.Height
	s.rect.palette = rl.NewRectangle(0, paletteY, paletteW, h-paletteY)
	s.rect.metadata = rl.NewRectangle(w-metadataW, contentTop, metadataW, h-contentTop)
	s.rect.grid = rl.NewRectangle(paletteW, contentTop, w-paletteW-metadataW, h-contentTop)

	if s.area.Width == 0 || s.area.Height == 0 {
		s.rect.cellPx = 0
		return
	}
	mw := s.area.Width
	mh := s.area.Height
	availW := s.rect.grid.Width - 2*gridMargin
	availH := s.rect.grid.Height - 2*gridMargin
	cellW := availW / float32(mw)
	cellH := availH / float32(mh)
	cell := cellW
	if cellH < cell {
		cell = cellH
	}
	cell *= s.zoom
	if cell < 4 {
		cell = 4
	}
	s.rect.cellPx = cell
	totalW := cell * float32(mw)
	totalH := cell * float32(mh)
	s.rect.gridW = totalW
	s.rect.gridH = totalH
	// baseX/baseY are gridX/gridY at pan==0 (map centered). Clamp the pan against
	// these so a drag or a stale pan can't fling the map off-screen; self-heals on
	// zoom-out. Fits-inside when small, edge+overscroll when overflowing.
	baseX := s.rect.grid.X + (s.rect.grid.Width-totalW)/2
	baseY := s.rect.grid.Y + (s.rect.grid.Height-totalH)/2
	s.panX = core.ClampPanAxis(s.panX, baseX, s.rect.grid.X, s.rect.grid.Width, totalW, panOverscroll)
	s.panY = core.ClampPanAxis(s.panY, baseY, s.rect.grid.Y, s.rect.grid.Height, totalH, panOverscroll)
	s.rect.gridX = baseX + s.panX
	s.rect.gridY = baseY + s.panY
}

// panOverscroll is how far past a map edge the pan may push when the map
// overflows, so edge tiles aren't jammed against the panels.
const panOverscroll = float32(48)

// cellAt converts a screen mouse position into a (x,z) tile, or -1,-1 if outside.
func (s *State) cellAt(p rl.Vector2) (int, int) {
	if s.rect.cellPx <= 0 {
		return -1, -1
	}
	// Iso preview is read-only: top-down screen→tile math doesn't hold under iso.
	if s.isoView {
		return -1, -1
	}
	// Reject points outside the grid VIEWPORT, not just the origin: panned/zoomed
	// tiles can extend under the metadata panel, and hover-driven paths
	// (paste-at-cursor, test-from-cursor) lack their own rect gate.
	if !pointIn(p, s.rect.grid) {
		return -1, -1
	}
	if p.X < s.rect.gridX || p.Y < s.rect.gridY {
		return -1, -1
	}
	x := int((p.X - s.rect.gridX) / s.rect.cellPx)
	z := int((p.Y - s.rect.gridY) / s.rect.cellPx)
	if x < 0 || z < 0 || x >= s.area.Width || z >= s.area.Height {
		return -1, -1
	}
	return x, z
}

// frameMouse caches rl.GetMousePosition() for one Draw() pass so per-widget
// hover checks don't each cross the CGo boundary. Rewritten at the top of Draw.
var frameMouse rl.Vector2

// frameAssets stashes the frame's render.Resources so modal handlers (whose
// signatures lack it) can reach loaded enemy textures. Same rewritten-at-top-of-
// Draw discipline; an Update-phase reader sees the previous frame's (stable) bundle.
var frameAssets render.Resources

func Draw(s *State, assets render.Resources) {
	font := assets.Font()
	theme := assets.Theme()
	frameMouse = rl.GetMousePosition()
	frameAssets = assets
	s.layout()
	rl.ClearBackground(bgWindow)
	drawTopbar(s, font, theme)
	drawToolbar(s, font, theme)
	// Layer select: left-column strip directly above the Levels panel.
	drawLayerMenuButton(s, font, theme)
	drawLevelsPanel(s, font, theme)
	drawPalette(s, font, theme)
	drawMetadata(s, font, theme)
	drawGrid(s, font)
	// Coverage heatmap: distance-from-start tint over the top-down grid.
	drawHeatmapOverlay(s, font, theme)
	// Overview minimap: grid bottom-right, below scrollbars/status/modals.
	drawMinimap(s)
	drawMinimapLegend(s, font, theme)
	// Recent-brush quick-pick row, bottom-left of the grid.
	drawBrushRecents(s, font)
	// Scrollbars: over panels + grid, below status toasts and modals.
	drawScrollbars(s)
	if len(s.statusLog) > 0 {
		drawStatus(s, font, theme)
	}
	if s.showStatusLog {
		drawStatusHistory(s, font, theme)
	}
	// Ruler readout floats at the cursor while measuring (both views).
	drawMeasureReadout(s, font, theme)
	// Toolbar hover tooltip: late (over the canvas), not inside drawToolbar.
	drawToolbarTooltip(s, font, theme)
	drawPaletteTooltip(s, font, theme)
	if h, ok := modalHandlers[s.modal]; ok && h.draw != nil {
		h.draw(s, font, theme)
	}
	// A modal's picker dropdown paints on top, once here (also the right-click
	// context menu — the ddContext owner). No-op when none open.
	drawDropdown(s, font, theme)
}

// modalHandler bundles a modal's draw + update functions in one table row.
type modalHandler struct {
	draw   func(*State, rl.Font, render.Theme)
	update func(*State) Action
}

var modalHandlers = map[modalKind]modalHandler{
	modalOpen:              {draw: drawOpenModal, update: updateOpenModal},
	modalSaveAs:            {draw: drawSaveAsModal, update: updateSaveAsModal},
	modalConfirmDirty:      {draw: drawConfirmDirtyModal, update: updateConfirmDirtyModal},
	modalPackEdit:          {draw: drawPackEditModal, update: updatePackEditModal},
	modalChestEdit:         {draw: drawChestEditModal, update: updateChestEditModal},
	modalSounds:            {draw: drawSoundsModal, update: updateSoundsModal},
	modalDoorEdit:          {draw: drawDoorEditModal, update: updateDoorEditModal},
	modalValidate:          {draw: drawValidateModal, update: updateValidateModal},
	modalEntityList:        {draw: drawEntityListModal, update: updateEntityListModal},
	modalNew:               {draw: drawNewMapModal, update: updateNewMapModal},
	modalEscMenu:           {draw: drawEscMenuModal, update: updateEscMenuModal},
	modalFoeView:           {draw: drawFoeViewModal, update: updateFoeViewModal},
	modalPartyView:         {draw: drawPartyViewModal, update: updatePartyViewModal},
	modalHitGlyphs:         {draw: drawHitGlyphsModal, update: updateHitGlyphsModal},
	modalObjectView:        {draw: drawObjectViewModal, update: updateObjectViewModal},
	modalWallFaces:         {draw: drawWallFacesModal, update: updateWallFacesModal},
	modalDialogList:        {draw: drawDialogListModal, update: updateDialogListModal},
	modalDialogNodes:       {draw: drawDialogNodesModal, update: updateDialogNodesModal},
	modalDialogNodeEdit:    {draw: drawDialogNodeEditModal, update: updateDialogNodeEditModal},
	modalDialogChoiceEdit:  {draw: drawDialogChoiceEditModal, update: updateDialogChoiceEditModal},
	modalDialogActionEdit:  {draw: drawDialogActionEditModal, update: updateDialogActionEditModal},
	modalDialogCondEdit:    {draw: drawDialogCondEditModal, update: updateDialogCondEditModal},
	modalDialogTriggerList: {draw: drawDialogTriggerListModal, update: updateDialogTriggerListModal},
	modalDialogTriggerEdit: {draw: drawDialogTriggerEditModal, update: updateDialogTriggerEditModal},
	modalWallFeatureEdit:   {draw: drawWallFeatureEditModal, update: updateWallFeatureEditModal},
	modalLocationEdit:      {draw: drawLocationEditModal, update: updateLocationEditModal},
	modalHelp:              {draw: drawHelpModal, update: updateHelpModal},
	modalCrystalEdit:       {draw: drawCrystalEditModal, update: updateCrystalEditModal},
	modalGoto:              {draw: drawGotoModal, update: updateGotoModal},
	modalStats:             {draw: drawStatsModal, update: updateStatsModal},
	modalPrefabs:           {draw: drawPrefabsModal, update: updatePrefabsModal},
}

// init asserts every dispatchable modalKind (excluding modalNone/modalCount) has
// a handler row with BOTH draw and update set, so a new modal without one panics
// at startup instead of silently no-op'ing or freezing.
func init() {
	for m := modalNone + 1; m < modalCount; m++ {
		h, ok := modalHandlers[m]
		if !ok {
			panic(fmt.Sprintf("editor: modalKind %d has no modalHandlers entry — register draw + update functions", int(m)))
		}
		if h.draw == nil {
			panic(fmt.Sprintf("editor: modalKind %d has a nil draw func in its modalHandlers entry", int(m)))
		}
		if h.update == nil {
			panic(fmt.Sprintf("editor: modalKind %d has a nil update func in its modalHandlers entry", int(m)))
		}
	}
}

// doorEditHitTarget enumerates the door edit modal's clickable regions.
type doorEditHitTarget int

const (
	doorHitOutside doorEditHitTarget = iota
	doorHitName
	doorHitTargetMap
	doorHitTargetMapPick
	doorHitTargetDoor
	doorHitTargetDoorPick
	doorHitFacing
	doorHitStyle
	doorHitDelete
	doorHitClose
)

// doorEditHit pairs the hit kind with optional payload (unused now that facing +
// style are dropdown pickers; kept for the target enum).
type doorEditHit struct {
	kind doorEditHitTarget
}

// doorEditLayout holds the rects for the door edit modal's clickable regions
// so update and draw stay in sync. Facing + style are single picker buttons
// (each opens a dropdown), not per-value button rows.
type doorEditLayout struct {
	card        rl.Rectangle
	nameField   rl.Rectangle
	mapField    rl.Rectangle
	mapPickBtn  rl.Rectangle // ▼ opens the target-map picker (all .map ids on disk)
	doorField   rl.Rectangle
	doorPickBtn rl.Rectangle // ▼ opens the target-door picker (target map's doors)
	facingBtn   rl.Rectangle
	styleBtn    rl.Rectangle
	deleteBtn   rl.Rectangle
	closeBtn    rl.Rectangle
}

// doorPickBtnW is the width of the ▼ picker button trailing the target-map /
// target-door text fields; doorPickBtnGap is the breath between field and button.
const (
	doorPickBtnW   = float32(46)
	doorPickBtnGap = float32(6)
)

func doorEditLayoutFor() doorEditLayout {
	r := centeredCardRect(doorEditModalW, doorEditModalH)
	// Field-stack metrics shared with the dialog editors so field height / header
	// inset / row pitch can't drift.
	x, fw := cardContentBox(r)
	y := r.Y + dialogHeaderInset
	fieldH := dialogFieldH
	rowGap := dialogRowGap
	fields := stackRows(x, y, fw, fieldH, rowGap, 3)
	nameField := fields[0]
	// Target map / door fields keep free-text entry but trade their right edge for a
	// ▼ picker button (pick from disk, or still type a not-yet-created id).
	fieldW := fw - doorPickBtnW - doorPickBtnGap
	mapField := rl.NewRectangle(fields[1].X, fields[1].Y, fieldW, fields[1].Height)
	mapPickBtn := rl.NewRectangle(fields[1].X+fieldW+doorPickBtnGap, fields[1].Y, doorPickBtnW, fields[1].Height)
	doorField := rl.NewRectangle(fields[2].X, fields[2].Y, fieldW, fields[2].Height)
	doorPickBtn := rl.NewRectangle(fields[2].X+fieldW+doorPickBtnGap, fields[2].Y, doorPickBtnW, fields[2].Height)
	y += 3*rowGap + dialogStackTailGap
	// Facing + style picker buttons (full-width, one per row) — each opens a dropdown.
	facingBtn := rl.NewRectangle(x, y, fw, fieldH)
	y += rowGap
	styleBtn := rl.NewRectangle(x, y, fw, fieldH)
	deleteBtn := bottomLeftBtn(r)
	closeBtn := bottomRightBtn(r)
	return doorEditLayout{
		card:        r,
		nameField:   nameField,
		mapField:    mapField,
		mapPickBtn:  mapPickBtn,
		doorField:   doorField,
		doorPickBtn: doorPickBtn,
		facingBtn:   facingBtn,
		styleBtn:    styleBtn,
		deleteBtn:   deleteBtn,
		closeBtn:    closeBtn,
	}
}

// doorEditHitTest reports which region p falls in (doorHitOutside default).
func doorEditHitTest(s *State, p rl.Vector2) doorEditHit {
	l := doorEditLayoutFor()
	if !pointIn(p, l.card) {
		return doorEditHit{kind: doorHitOutside}
	}
	switch {
	case pointIn(p, l.nameField):
		return doorEditHit{kind: doorHitName}
	case pointIn(p, l.mapField):
		return doorEditHit{kind: doorHitTargetMap}
	case pointIn(p, l.mapPickBtn):
		return doorEditHit{kind: doorHitTargetMapPick}
	case pointIn(p, l.doorField):
		return doorEditHit{kind: doorHitTargetDoor}
	case pointIn(p, l.doorPickBtn):
		return doorEditHit{kind: doorHitTargetDoorPick}
	case pointIn(p, l.facingBtn):
		return doorEditHit{kind: doorHitFacing}
	case pointIn(p, l.styleBtn):
		return doorEditHit{kind: doorHitStyle}
	case pointIn(p, l.deleteBtn):
		return doorEditHit{kind: doorHitDelete}
	case pointIn(p, l.closeBtn):
		return doorEditHit{kind: doorHitClose}
	}
	// Click inside the card but on no region: no-op (don't dismiss).
	return doorEditHit{kind: doorHitOutside}
}

// --- Top bar ---------------------------------------------------------------

// topbarBtn pairs a label with the action to run on click.
type topbarBtn struct {
	label  string
	action func(*State)
	// active, when set, draws the button highlighted while it returns true (toggles).
	active func(*State) bool
	// enabled, when set, gates a context-sensitive control: always DRAWN in place
	// (row never reflows) but grayed + click-ignored unless it returns true. nil = always.
	enabled func(*State) bool
	// help, when set, is a one-line hover tooltip. "" = none.
	help string
	// width, when > 0, overrides the auto-sized button width (for labels the
	// auto-sizer under/over-shoots). 0 = auto-size from the label.
	width float32
}

// The top menu bar (File/Edit/View/Assets/Map) lives in menus.go as menuBarBtns.

// Context-visibility predicates (topbarBtn.enabled): grid-painting controls are
// dead on Entities; the elevation cluster only acts on Elevation. Graying on the
// wrong layer declutters the toolbar.
func onGridLayer(s *State) bool      { return s.layer != LayerEntities }
func onElevationLayer(s *State) bool { return s.layer == LayerElevation }

// onScatterLayer gates the Scatter toolbar button to the cosmetic-scatter layers.
func onScatterLayer(s *State) bool { return s.layer == LayerDecor || s.layer == LayerProps }

// canFillLayer gates the Edit ▸ Fill Layer command: a grid layer that isn't a
// per-floor prop/decor stack (which floodFill/fillEntireLayer refuse — see
// layerBlocksBulkFill). Keeps the menu row from showing enabled then no-op-flashing.
func canFillLayer(s *State) bool { return onGridLayer(s) && !layerBlocksBulkFill(s) }

// toolbarActionBtns are the constant-reach controls kept out of the menus:
// undo/redo, brush-size steppers, and the elevation cluster. Undo/Redo gray out
// on an empty stack; the rest gate on the active layer.
var toolbarActionBtns = []topbarBtn{
	{label: "Undo", action: undoOne, enabled: func(s *State) bool { return len(s.undo) > 0 }, help: "Step back one change (Ctrl+Z)."},
	{label: "Redo", action: redoOne, enabled: func(s *State) bool { return len(s.redo) > 0 }, help: "Re-apply the last undone change (Ctrl+Y)."},
	{label: "Brush -", action: func(s *State) { stepBrushSize(s, -1) }, enabled: onGridLayer, help: "Shrink the brush footprint.", width: 78},
	{label: "Brush +", action: func(s *State) { stepBrushSize(s, +1) }, enabled: onGridLayer, help: "Grow the brush footprint.", width: 78},
	{label: "Lvl -", action: func(s *State) { stepEditLevel(s, -1) }, help: "Lower the active level (the floor paints build onto). Also PgDn / the Levels panel."},
	{label: "Lvl +", action: func(s *State) { stepEditLevel(s, +1) }, help: "Raise the active level (the floor paints build onto). Also PgUp / the Levels panel."},
	{label: "Ramp",
		action:  func(s *State) { s.rampMode = !s.rampMode; s.sculptMode = false },
		active:  func(s *State) bool { return s.rampMode },
		enabled: onElevationLayer,
		help:    "Ramp mode: paint sloped transitions between elevation levels."},
	{label: "Sculpt",
		action:  func(s *State) { s.sculptMode = !s.sculptMode; s.rampMode = false },
		active:  func(s *State) bool { return s.sculptMode },
		enabled: onElevationLayer,
		help:    "Sculpt mode: left-drag raises columns +1, right-click lowers −1 (relative terracing)."},
	{label: "Scatter",
		action:  cycleScatterDensity,
		active:  func(s *State) bool { return s.scatterDensity > 0 },
		enabled: onScatterLayer,
		help:    "Scatter mode: a size>1 brush stamps decor/props at random density (click cycles Off/25/50/75%)."},
	{label: "Mirror ↔",
		action:  func(s *State) { s.mirrorX = !s.mirrorX },
		active:  func(s *State) bool { return s.mirrorX },
		enabled: onGridLayer,
		help:    "Mirror every stroke across the map's vertical center axis (live symmetry)."},
	{label: "Mirror ↕",
		action:  func(s *State) { s.mirrorZ = !s.mirrorZ },
		active:  func(s *State) bool { return s.mirrorZ },
		enabled: onGridLayer,
		help:    "Mirror every stroke across the map's horizontal center axis (live symmetry)."},
}

// toolbarBtns is the full action row: the tool-select group then the editing
// commands. Assembled at init from toolModeLabels.
var toolbarBtns []topbarBtn

func init() {
	for m, label := range toolModeLabels {
		mode := toolMode(m)
		toolbarBtns = append(toolbarBtns, topbarBtn{
			label:  label,
			action: func(s *State) { s.tool = mode },
			active: func(s *State) bool { return s.tool == mode },
			help:   toolModeHelp[m],
		})
	}
	toolbarBtns = append(toolbarBtns, toolbarActionBtns...)
}

// buttonStripHit / drawButtonStrip are the shared left-to-right button walk for
// the menu bar and toolbar, keeping their draw and hit-test in lockstep.
const buttonStripStartX = float32(8)

// stripButtonRect returns button i's on-strip rect, advancing by topbarBtnWidth +
// tightBtnGap exactly as drawButtonStrip/buttonStripHit do — so a dropdown anchor
// (menuAnchorRect) can't desync from where a width-overridden button actually draws.
func stripButtonRect(btns []topbarBtn, i int, y, h float32) rl.Rectangle {
	x := buttonStripStartX
	for j := 0; j < i && j < len(btns); j++ {
		x += topbarBtnWidth(btns[j]) + tightBtnGap
	}
	w := float32(0)
	if i >= 0 && i < len(btns) {
		w = topbarBtnWidth(btns[i])
	}
	return rl.NewRectangle(x, y, w, h)
}

func buttonStripHit(btns []topbarBtn, y, h float32, p rl.Vector2) int {
	x := buttonStripStartX
	for i, b := range btns {
		w := topbarBtnWidth(b)
		if pointIn(p, rl.NewRectangle(x, y, w, h)) {
			return i
		}
		x += w + tightBtnGap
	}
	return -1
}

func drawButtonStrip(font rl.Font, s *State, btns []topbarBtn, y, h float32) {
	x := buttonStripStartX
	for _, b := range btns {
		w := topbarBtnWidth(b)
		r := rl.NewRectangle(x, y, w, h)
		if b.enabled != nil && !b.enabled(s) {
			drawButtonDisabled(font, r, b.label) // context-inactive: grayed in place
		} else {
			drawButton(font, r, b.label, b.active != nil && b.active(s))
		}
		x += w + tightBtnGap
	}
}

func toolbarButtonAt(s *State, p rl.Vector2) int {
	if !pointIn(p, s.rect.toolbar) {
		return -1
	}
	return buttonStripHit(toolbarBtns, s.rect.toolbar.Y+6, toolbarH-12, p)
}

func drawToolbar(s *State, font rl.Font, theme render.Theme) {
	rl.DrawRectangleRec(s.rect.toolbar, bgWindow)
	rl.DrawLineEx(
		rl.NewVector2(0, s.rect.toolbar.Y+toolbarH),
		rl.NewVector2(s.rect.toolbar.Width, s.rect.toolbar.Y+toolbarH),
		1, outlineHard)
	// Active-level readout (right-aligned): the floor paints build onto. [RAMP]
	// flags ramp tool-mode (only meaningful on Elevation). Measured first so the
	// button strip can be clipped to its left — never overpainting it on a narrow window.
	label := fmt.Sprintf("Active level: %d", s.editLevel)
	if s.layer == LayerElevation && s.rampMode {
		label += "  [RAMP]"
	}
	sz := editorFontLabel
	m := render.MeasureRichText(font, label, sz, 1)
	readoutLeft := s.rect.toolbar.Width - m.X - 12
	stripW := int32(readoutLeft - 6)
	if stripW < 0 {
		stripW = 0
	}
	rl.BeginScissorMode(0, int32(s.rect.toolbar.Y), stripW, int32(toolbarH))
	drawButtonStrip(font, s, toolbarBtns, s.rect.toolbar.Y+6, toolbarH-12)
	rl.EndScissorMode()
	render.DrawRichText(font, label,
		rl.NewVector2(readoutLeft, s.rect.toolbar.Y+(toolbarH-sz)/2),
		sz, 1, editorActiveLevelText)
	// NB: hover tooltip is drawn LATE (drawToolbarTooltip); here it'd be overdrawn by the canvas.
}

// drawToolbarTooltip paints the hovered toolbar button's help bubble, late in
// Draw so it layers over the canvas. Suppressed while a modal/menu is up.
func drawToolbarTooltip(s *State, font rl.Font, theme render.Theme) {
	if s.modal != modalNone || s.dropdownOpen() {
		return
	}
	mp := frameMouse // Draw()-cached; avoids a per-frame CGo poll
	if !pointIn(mp, s.rect.toolbar) {
		return
	}
	if hit := toolbarButtonAt(s, mp); hit >= 0 && toolbarBtns[hit].help != "" {
		drawButtonTooltip(font, theme, toolbarBtns[hit].help, mp)
	}
}

// drawTooltipCard paints a hover bubble of lines near mp, clamped inside clamp
// (first line tooltipHeading, rest tooltipText). Shared by both editor tooltips.
func drawTooltipCard(font rl.Font, lines []string, fontSize, lineH float32, mp rl.Vector2, clamp rl.Rectangle) {
	if len(lines) == 0 {
		return
	}
	const pad = float32(6)
	var tw float32
	for _, l := range lines {
		if m := render.MeasureRichText(font, l, fontSize, 1).X; m > tw {
			tw = m
		}
	}
	w := tw + 2*pad
	h := float32(len(lines))*lineH + 2*pad
	x, y := mp.X+14, mp.Y+14
	if x+w > clamp.X+clamp.Width-4 {
		x = mp.X - w - 8
	}
	if y+h > clamp.Y+clamp.Height-4 {
		y = mp.Y - h - 8
	}
	r := rl.NewRectangle(x, y, w, h)
	rl.DrawRectangleRec(r, tooltipBG)
	rl.DrawRectangleLinesEx(r, 1, editorBorderActive)
	for i, l := range lines {
		col := tooltipText
		if i == 0 {
			col = tooltipHeading
		}
		render.DrawRichText(font, l, rl.NewVector2(r.X+pad, r.Y+pad+float32(i)*lineH), fontSize, 1, col)
	}
}

// drawPaletteTooltip paints a hover bubble for the palette brush under the cursor —
// the per-brush parity with the toolbar's hover help. Derived facts only (blocking,
// footprint, ramp), so it stays quiet on the plain floor/ceiling brushes.
func drawPaletteTooltip(s *State, font rl.Font, theme render.Theme) {
	if s.modal != modalNone || s.dropdownOpen() {
		return
	}
	mp := frameMouse
	if !pointIn(mp, s.rect.palette) {
		return
	}
	palette := layerBrushes[s.layer]
	visStart, visEnd := visiblePaletteRange(s, len(palette))
	for i := visStart; i < visEnd; i++ {
		if !pointIn(mp, paletteEntryRect(s, i)) {
			continue
		}
		if lines := brushTooltipLines(s.layer, palette[i]); len(lines) > 0 {
			sw, sh := render.ScreenSizeF()
			drawTooltipCard(font, lines, editorFontHint, tooltipLineH, mp, rl.NewRectangle(0, 0, sw, sh))
		}
		return
	}
}

// brushTooltipLines derives the palette hover help for a brush: blocking / footprint /
// ramp notes pulled from core so they can't drift from the tile's real behavior. Empty
// = no bubble (plain floors, ceiling).
func brushTooltipLines(layer Layer, b Brush) []string {
	if b.Erase {
		return []string{"Erase — reset this cell to empty"}
	}
	switch layer {
	case LayerElevation:
		return []string{"Set Height", "Paints one cube at the active level (gaps make bridges)."}
	case LayerProps:
		note := "Walk-through"
		if core.PropBlockHeight(b.Char) > 0 {
			note = "Blocks movement"
		}
		if fp := core.PropFootprint(b.Char); len(fp) > 1 {
			note += fmt.Sprintf(" · %d-tile footprint", len(fp))
		}
		return []string{b.Name, note}
	case LayerDecor:
		note := "Never blocks"
		if fp := core.DecorFootprint(b.Char); len(fp) > 1 {
			note += fmt.Sprintf(" · %d-tile", len(fp))
		}
		return []string{b.Name, note}
	case LayerFloor:
		switch {
		case core.IsRampChar(b.Char):
			return []string{b.Name, "Walkable slope — bridges up one level"}
		case core.IsBlockingFloor(b.Char):
			return []string{b.Name, "Blocks movement (renders flat)"}
		}
	}
	return nil
}

// drawButtonTooltip paints a one-line help bubble near the cursor for a toolbar button.
func drawButtonTooltip(font rl.Font, theme render.Theme, text string, mp rl.Vector2) {
	_ = theme // styled with the shared tooltip* tokens
	sw, sh := render.ScreenSizeF()
	drawTooltipCard(font, []string{text}, editorFontHint, tooltipLineH, mp, rl.NewRectangle(0, 0, sw, sh))
}

// topbarButtonAt returns the index of the menu-bar label under p, or -1.
func topbarButtonAt(s *State, p rl.Vector2) int {
	if !pointIn(p, s.rect.topbar) {
		return -1
	}
	return buttonStripHit(menuBarBtns, menuBarBtnY, menuBarBtnH, p)
}

// topbarInfoKey captures everything the topbar readouts derive from; unchanged
// frame-to-frame, drawTopbar reuses the cached strings + measures.
type topbarInfoKey struct {
	epoch          uint64
	hoverX, hoverZ int
	layer          Layer
	brushSize      int
	zoom           float32
	phase          core.TimeOfDay
	undoLen        int
	dirty          bool
	path           string
}

var (
	topbarInfoKeyCache topbarInfoKey
	topbarInfoReady    bool
	topbarNameLabel    string
	topbarNameMeasure  rl.Vector2
	topbarInfoLabel    string
	topbarInfoMeasure  rl.Vector2
)

func drawTopbar(s *State, font rl.Font, theme render.Theme) {
	rl.DrawRectangleRec(s.rect.topbar, theme.SurfacePrimary)
	rl.DrawLineEx(rl.NewVector2(0, topbarH), rl.NewVector2(s.rect.topbar.Width, topbarH), 1, outlineHard)

	drawButtonStrip(font, s, menuBarBtns, menuBarBtnY, menuBarBtnH)

	key := topbarInfoKey{
		epoch:     s.contentEpoch,
		hoverX:    s.hoverX,
		hoverZ:    s.hoverZ,
		layer:     s.layer,
		brushSize: s.brushSize,
		zoom:      s.zoom,
		phase:     s.previewPhase,
		undoLen:   len(s.undo),
		dirty:     s.dirty,
		path:      s.area.Path,
	}
	if !topbarInfoReady || key != topbarInfoKeyCache {
		id := core.MapIDFromPath(s.area.Path)
		if id == "" {
			id = "(unsaved)"
		}
		dirtyMark := ""
		if s.dirty {
			dirtyMark = " *"
		}
		topbarNameLabel = id + dirtyMark
		topbarNameMeasure = render.MeasureRichText(font, topbarNameLabel, editorFontTopbar, 1)

		coord := "—"
		hoverDesc := ""
		if s.hoverX >= 0 {
			coord = core.TileCoord(s.hoverX, s.hoverZ)
			hoverDesc = core.AreaTileSummary(&s.area, s.hoverX, s.hoverZ)
		}
		topbarInfoLabel = fmt.Sprintf("cell %s   %s   layer %s   brush %dx%d   zoom %.0f%%   phase %s (T)   undo %d/%d   redo %d",
			coord, hoverDesc, layerName(s.layer), s.brushSize, s.brushSize, s.zoom*100, core.PhaseName(s.previewPhase), len(s.undo), undoLimit, len(s.redo))
		topbarInfoMeasure = render.MeasureRichText(font, topbarInfoLabel, editorFontLabel, 1)

		topbarInfoKeyCache = key
		topbarInfoReady = true
	}

	// Positioning re-reads the live window width each frame; only the strings +
	// their measures are memoized.
	labelX := s.rect.topbar.Width - topbarNameMeasure.X - 10
	render.DrawTextWithShadow(font, topbarNameLabel,
		labelX, (topbarH-topbarNameMeasure.Y)/2,
		editorFontTopbar, theme.TextMuted)
	// Info line sits left of the map name; skip it when it would collide with the
	// menu-bar labels (narrow window) rather than overpainting them. The name always shows.
	infoX := labelX - topbarInfoMeasure.X - 24
	menuEndX := stripButtonRect(menuBarBtns, len(menuBarBtns), menuBarBtnY, menuBarBtnH).X
	if infoX >= menuEndX+12 {
		render.DrawTextWithShadow(font, topbarInfoLabel, infoX, (topbarH-topbarInfoMeasure.Y)/2, editorFontLabel, theme.TextHint)
	}
}

// layerMenuBtnRect is the active-layer dropdown button — the left column's top
// strip (s.rect.layerTabs), directly above the Levels panel. Single source for
// draw + hit-test + dropdown anchor.
func layerMenuBtnRect(s *State) rl.Rectangle {
	t := s.rect.layerTabs
	const pad = float32(5)
	return rl.NewRectangle(t.X+pad, t.Y+pad, t.Width-2*pad, t.Height-2*pad)
}

// drawLayerMenuButton paints the active-layer dropdown trigger: a big label, a
// color chip + border in the active layer's accent (color-coded so the layer
// reads at a glance). Click opens ddLayer, whose rows carry the per-layer eye.
func drawLayerMenuButton(s *State, font rl.Font, theme render.Theme) {
	r := layerMenuBtnRect(s)
	accent := layerAccent(s.layer)
	bg := bgActive
	if pointIn(frameMouse, r) {
		bg = bgEntryHover
	}
	rl.DrawRectangleRec(r, bg)
	rl.DrawRectangleLinesEx(r, 3, accent)
	// Color chip in the accent, then the big label beside it.
	chip := rl.NewRectangle(r.X+8, r.Y+8, 16, r.Height-16)
	drawSwatch(chip, accent)
	label := "Layer: " + layerName(s.layer) + dropdownArrowSuffix
	render.DrawTextWithShadow(font, label, chip.X+chip.Width+10, r.Y+(r.Height-editorFontTopbar)/2, editorFontTopbar, theme.TextPrimary)
}

// drawSwatch paints a color chip — a filled rect with the dim editor outline.
// Shared by the layer-accent chrome (Layer button + dropdown rows) so they match.
func drawSwatch(rect rl.Rectangle, fill rl.Color) {
	rl.DrawRectangleRec(rect, fill)
	rl.DrawRectangleLinesEx(rect, 1, editorBorderDim)
}

// approxTextWidth estimates a label's pixel width without a font handle (~0.5px
// per char per point). Shared by the button + context-menu sizers, which lay out
// before the font is loaded.
func approxTextWidth(label string, fontSize float32) float32 {
	return float32(len(label)) * fontSize * 0.5
}

// buttonWidth auto-sizes a button to its label so long captions don't overflow,
// floored at 72. Deterministic from the string, so a modal's draw and hit-test agree.
// A topbarBtn with an explicit width overrides this via topbarBtnWidth.
func buttonWidth(label string) float32 {
	w := approxTextWidth(label, editorFontBody) + buttonLabelPadX
	if w < 72 {
		w = 72
	}
	return w
}

// topbarBtnWidth is a strip button's laid-out width: its explicit width override if
// set, else the auto-sized label width. Keeps the width beside the button definition
// instead of a separate label-keyed map that drifts when a caption is renamed.
func topbarBtnWidth(b topbarBtn) float32 {
	if b.width > 0 {
		return b.width
	}
	return buttonWidth(b.label)
}

// Modal button-layout tunables, shared by every modal's button helpers.
const (
	modalBtnH         = float32(30)  // action button height
	modalContentInset = float32(16)  // left/right card padding (body width = card.Width - 2*inset)
	modalBtnGap       = float32(8)   // gap between stacked / row modal buttons
	modalBottomInset  = float32(14)  // gap from the card's bottom edge to the button block
	tightBtnGap       = float32(6)   // gap for dense strips: the topbar/toolbar, the wrapped add grid, equal-width rows
	modalWideBtnW     = float32(110) // width of the Delete / Close affordance shared by the door + dialog edit modals
	buttonLabelPadX   = float32(28)  // horizontal padding added around a measured label to size an auto-width button (buttonWidth + context menu)
	textFieldH        = float32(28)  // single-line text-field / input row height (shared by rename, Save As, sound name, dialog rows)
	modalFooterHintDY = float32(26)  // dismissal/help hint baseline up from the card's bottom edge
	// modalSubheadingDY is the baseline of the first subheading/hint line below a
	// modal's title (down from the card top) — the header-relative mirror of
	// modalFooterHintDY. Named so the sub-title line shares one baseline instead of
	// re-typing 40 per modal. Font/color stay per-call (the modals differ there).
	modalSubheadingDY = float32(40)
)

// drawModalFooterHint paints a one-line dismissal/help hint at the modal card's
// bottom-left (editorFontHint), at the shared baseline every modal uses.
func drawModalFooterHint(font rl.Font, card rl.Rectangle, text string, theme render.Theme) {
	render.DrawRichText(font, text,
		rl.NewVector2(card.X+modalContentInset, card.Y+card.Height-modalFooterHintDY),
		editorFontHint, 1, theme.TextHint)
}

// modalContentWidth is the usable inner width of a modal card.
func modalContentWidth(card rl.Rectangle) float32 { return card.Width - 2*modalContentInset }

// cardContentBox returns a modal card's content origin x and inner width — the
// opening line of every dialog *LayoutFor.
func cardContentBox(card rl.Rectangle) (x, fw float32) {
	return card.X + modalContentInset, modalContentWidth(card)
}

// List-modal chrome shared by the goto / prefab layouts: modalBodyTop is the first
// content baseline below the header; listModalHeight sizes a card whose only
// variable is a fixed-pitch row list (rows × rowH atop the fixed chrome budget).
const (
	modalBodyTopDY   = float32(44)
	modalListChromeH = float32(150) // header + input/button row + footer reserve
)

func modalBodyTop(card rl.Rectangle) float32       { return card.Y + modalBodyTopDY }
func listModalHeight(rows int, rowH float32) float32 { return modalListChromeH + float32(rows)*rowH }

// drawSelectedListRow fills a list row's cursor plate with the editor's shared
// bgActive tone. One home for the selected-row look so every editor list highlights
// the cursor identically (the sound modal used the game-panel gilt style before).
func drawSelectedListRow(rect rl.Rectangle) { rl.DrawRectangleRec(rect, bgActive) }

// modalFooterButtonY is the bottom button row's Y: modalBottomInset up from the
// card bottom. Shared by modalButtonRow + the gallery modals.
func modalFooterButtonY(card rl.Rectangle) float32 {
	return card.Y + card.Height - modalBtnH - modalBottomInset
}

// modalGridBottom is where a gallery grid stops to clear the footer row: one
// modalBottomInset above the buttons. Derived from modalFooterButtonY so they can't drift.
func modalGridBottom(card rl.Rectangle) float32 {
	return modalFooterButtonY(card) - modalBottomInset
}

// modalButtonRow lays auto-width buttons left-to-right along a card's
// bottom-left, returning their rects in order.
func modalButtonRow(card rl.Rectangle, labels []string) []rl.Rectangle {
	return buttonRowAt(card.X+modalContentInset, card.Y+card.Height-modalBtnH-modalBottomInset, labels)
}

// buttonRowAt lays a row of auto-width modal buttons left-to-right from (x, y).
// Callers with a bespoke anchor place the same row shape without re-deriving math.
func buttonRowAt(x, y float32, labels []string) []rl.Rectangle {
	rects := make([]rl.Rectangle, len(labels))
	for i, lbl := range labels {
		w := buttonWidth(lbl)
		rects[i] = rl.NewRectangle(x, y, w, modalBtnH)
		x += w + modalBtnGap
	}
	return rects
}

// buttonRowWidth is the total span buttonRowAt occupies — for right-anchoring.
func buttonRowWidth(labels []string) float32 {
	var w float32
	for i, lbl := range labels {
		if i > 0 {
			w += modalBtnGap
		}
		w += buttonWidth(lbl)
	}
	return w
}

// modalButtonStack lays full-width buttons vertically, bottom-anchored, returned
// top-to-bottom. Full-width can't overflow — used for menus / confirm dialogs.
func modalButtonStack(card rl.Rectangle, labels []string) []rl.Rectangle {
	n := len(labels)
	rects := make([]rl.Rectangle, n)
	x := card.X + modalContentInset
	w := modalContentWidth(card)
	bottom := card.Y + card.Height - modalBottomInset
	for i := n - 1; i >= 0; i-- {
		bottom -= modalBtnH
		rects[i] = rl.NewRectangle(x, bottom, w, modalBtnH)
		bottom -= modalBtnGap
	}
	return rects
}

// drawModalButtons paints a computed rect set; modalButtonHit returns the
// clicked index. (Geometry comes from modalButtonRow/Stack/buttonGrid.)
func drawModalButtons(font rl.Font, rects []rl.Rectangle, labels []string) {
	drawModalButtonsSel(font, rects, labels, -1)
}

// drawModalButtonsSel paints the button set, highlighting the keyboard-selected row
// (sel < 0 = none, e.g. mouse/hotkey-only modals).
func drawModalButtonsSel(font rl.Font, rects []rl.Rectangle, labels []string, sel int) {
	for i, r := range rects {
		drawButton(font, r, labels[i], i == sel)
	}
}

// firstRectHit returns the index of the first rect containing mp, or -1. Shared by
// the modal-button / thumbnail / visualizer-tab hit loops.
func firstRectHit(mp rl.Vector2, rects []rl.Rectangle) int {
	for i, r := range rects {
		if pointIn(mp, r) {
			return i
		}
	}
	return -1
}

func modalButtonHit(rects []rl.Rectangle) int {
	if !rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		return -1
	}
	return firstRectHit(rl.GetMousePosition(), rects)
}

// modalCmd is a labeled action for a modal's buttons: label + action on one row,
// so caption and action can't drift. hot is an optional accelerator firing .run;
// run returns the editor Action to propagate.
type modalCmd struct {
	label string
	hot   func() bool
	run   func() Action
}

// runModalCmds fires the cmd under a left-click or whose hot accelerator is
// pressed, returning its Action plus true.
func runModalCmds(cmds []modalCmd, rects []rl.Rectangle) (Action, bool) {
	click := modalButtonHit(rects)
	for i, c := range cmds {
		if i == click || (c.hot != nil && c.hot()) {
			return c.run(), true
		}
	}
	return ActionNone, false
}

func cmdLabels(cmds []modalCmd) []string {
	out := make([]string, len(cmds))
	for i, c := range cmds {
		out[i] = c.label
	}
	return out
}

// buttonGrid lays auto-width buttons left-to-right within [x, x+maxW], wrapping
// to a new row when the next exceeds maxW. Returns rects in label order.
func buttonGrid(x, y, maxW float32, labels []string) []rl.Rectangle {
	rects := make([]rl.Rectangle, len(labels))
	cx, cy := x, y
	for i, lbl := range labels {
		w := buttonWidth(lbl)
		if cx > x && cx+w > x+maxW {
			cx = x
			cy += modalBtnH + tightBtnGap
		}
		rects[i] = rl.NewRectangle(cx, cy, w, modalBtnH)
		cx += w + tightBtnGap
	}
	return rects
}

// equalButtonRow splits width w into n equal-width button rects at (x, y),
// height h, with tightBtnGap between them.
func equalButtonRow(x, y, w, h float32, n int) []rl.Rectangle {
	if n <= 0 {
		return nil
	}
	bw := (w - float32(n-1)*tightBtnGap) / float32(n)
	rects := make([]rl.Rectangle, n)
	for i := 0; i < n; i++ {
		rects[i] = rl.NewRectangle(x+float32(i)*(bw+tightBtnGap), y, bw, h)
	}
	return rects
}

// buttonGridHeight is the total vertical span buttonGrid uses for labels (0 if empty).
func buttonGridHeight(maxW float32, labels []string) float32 {
	if len(labels) == 0 {
		return 0
	}
	rects := buttonGrid(0, 0, maxW, labels)
	last := rects[len(rects)-1]
	return last.Y + last.Height
}

// entityModalLayout is the shared geometry for the pack/chest editors: scrolled
// list window, action-button row, and wrapped add grid, derived once.
type entityModalLayout struct {
	card          rl.Rectangle
	listTop, rowH float32
	topRow, end   int
	actRects      []rl.Rectangle
	addRects      []rl.Rectangle
}

func entityModalLayoutFor(cursor, count int, addLabels, actLabels []string) entityModalLayout {
	card := centeredCardRect(entityEditModalW, entityEditModalH)
	gridW := modalContentWidth(card)
	gridX := card.X + modalContentInset
	addH := buttonGridHeight(gridW, addLabels)
	actH := buttonGridHeight(gridW, actLabels)
	addTop := card.Y + card.Height - modalBottomInset - addH
	actTop := addTop - modalBtnGap - actH
	addRects := buttonGrid(gridX, addTop, gridW, addLabels)
	actRects := buttonGrid(gridX, actTop, gridW, actLabels)
	listTop := card.Y + entityListTop
	listBottom := actTop - modalBtnGap
	topRow, end := scrollWindow(cursor, count, visibleRowCount(listBottom-listTop, entityListRowH))
	return entityModalLayout{card, listTop, entityListRowH, topRow, end, actRects, addRects}
}

// entityRowAt returns the list index whose row is under p, or -1.
func entityRowAt(lay entityModalLayout, p rl.Vector2) int {
	for i := lay.topRow; i < lay.end; i++ {
		row := rl.NewRectangle(lay.card.X+modalContentInset, lay.listTop+float32(i-lay.topRow)*lay.rowH, modalContentWidth(lay.card), lay.rowH)
		if pointIn(p, row) {
			return i
		}
	}
	return -1
}

// drawEntityListWindow paints the pack/chest list rows for a layout window,
// highlighting the cursor row, with ▲/▼ "N more" clip indicators.
func drawEntityListWindow(font rl.Font, theme render.Theme, lay entityModalLayout, count, cursor int, emptyText string, rowText func(int) string) {
	if count == 0 {
		render.DrawRichText(font, emptyText, rl.NewVector2(lay.card.X+modalContentInset, lay.listTop), editorFontLabel, 1, theme.TextHint)
		return
	}
	rows := make([]rl.Rectangle, 0, lay.end-lay.topRow)
	for i := lay.topRow; i < lay.end; i++ {
		rows = append(rows, rl.NewRectangle(lay.card.X, lay.listTop+float32(i-lay.topRow)*lay.rowH, 0, 0))
	}
	// math.MaxInt32 truncLen makes drawScrollList's per-row trim a no-op (this list doesn't truncate).
	drawScrollList(font, theme, rows, lay.topRow, count, cursor, math.MaxInt32,
		lay.card.X+entityListTextInset, lay.listTop+float32(lay.end-lay.topRow)*lay.rowH, rowText)
}

// scrollArrowGlyph returns the up/down scroll caret; single-sourced so the two
// scroll-hint helpers can't drift on the glyph.
func scrollArrowGlyph(up bool) string {
	if up {
		return "▲"
	}
	return "▼"
}

// drawScrollArrow paints a bare ▲/▼ scroll affordance (no count) at pos; caller
// owns the corner-offset math. For the "N more" counted form use drawScrollMoreHint.
func drawScrollArrow(font rl.Font, up bool, pos rl.Vector2, fontSize float32, col rl.Color) {
	render.DrawRichText(font, scrollArrowGlyph(up), pos, fontSize, 1, col)
}

// drawCenteredRichLabel paints label centered on both axes within r at editorFontBody.
func drawCenteredRichLabel(font rl.Font, r rl.Rectangle, label string, col rl.Color) {
	measure := render.MeasureRichText(font, label, editorFontBody, 1)
	render.DrawRichText(font, label,
		rl.NewVector2(r.X+(r.Width-measure.X)/2, r.Y+(r.Height-measure.Y)/2),
		editorFontBody, 1, col)
}

func drawButton(font rl.Font, r rl.Rectangle, label string, active bool) {
	bg := bgButton
	border := editorBorderMid
	text := textBright
	if active {
		bg = bgActive
		border = editorBorderActive
	}
	if pointIn(frameMouse, r) {
		bg = bgRowHover
	}
	rl.DrawRectangleRec(rl.NewRectangle(r.X+2, r.Y+2, r.Width, r.Height), render.FadeColor(rl.Black, 0.18))
	top := render.FadeColor(editorBorderActive, 0.18)
	bot := render.FadeColor(rl.Black, 0.16)
	rl.DrawRectangleRec(r, bg)
	if r.Width > 4 && r.Height > 4 {
		rl.DrawRectangleGradientV(int32(r.X+2), int32(r.Y+2), int32(r.Width-4), int32(r.Height-4), top, bot)
	}
	rl.DrawRectangleLinesEx(r, 1, border)
	if active {
		rl.DrawRectangleRec(rl.NewRectangle(r.X+4, r.Y+5, 3, r.Height-10), editorBorderActive)
	}
	if r.Width >= 44 && r.Height >= 24 {
		rl.DrawCircleV(rl.NewVector2(r.X+r.Width-8, r.Y+8), 1.5, render.FadeColor(editorBorderActive, 0.55))
	}
	drawCenteredRichLabel(font, r, label, text)
}

// drawButtonDisabled paints a context-inactive toolbar button: same footprint
// (no reflow), dimmed fill + faded text, no hover.
func drawButtonDisabled(font rl.Font, r rl.Rectangle, label string) {
	rl.DrawRectangleRec(r, render.FadeColor(bgButton, 0.45))
	rl.DrawRectangleLinesEx(r, 1, render.FadeColor(editorBorderMid, 0.5))
	drawCenteredRichLabel(font, r, label, render.FadeColor(textBright, 0.38))
}

// drawStepperButtons paints the shared "−"/"+" adjuster pair for a numeric
// stepper row (the value cell differs per caller).
func drawStepperButtons(font rl.Font, minus, plus rl.Rectangle) {
	drawButton(font, minus, "-", false)
	drawButton(font, plus, "+", false)
}

// spawnLevelStepW / spawnLevelStepH size the entity editors' Floor [-]/[+] buttons.
const (
	spawnLevelStepW = float32(26)
	spawnLevelStepH = float32(22)
)

// spawnLevelStepperRects returns the Floor [-]/[+] rects at a modal card's top-right,
// and whether to show them. Shown only on multi-level maps — on a flat map every
// spawn sits on floor 0 and the control would be noise.
func spawnLevelStepperRects(s *State, card rl.Rectangle) (minus, plus rl.Rectangle, show bool) {
	lo, hi, found := areaLevelSpan(&s.area)
	if !found || hi == lo {
		return rl.Rectangle{}, rl.Rectangle{}, false
	}
	y := card.Y + float32(modalHeadingInsetY) - 2
	plus = rl.NewRectangle(card.X+card.Width-modalContentInset-spawnLevelStepW, y, spawnLevelStepW, spawnLevelStepH)
	minus = rl.NewRectangle(plus.X-spawnLevelStepW-4, y, spawnLevelStepW, spawnLevelStepH)
	return minus, plus, true
}

// drawSpawnLevelStepper paints "Floor <signed>  [-] [+]" at the card's top-right for
// a placed spawn's Level (multi-level maps only).
func drawSpawnLevelStepper(s *State, font rl.Font, theme render.Theme, card rl.Rectangle, level int) {
	minus, plus, show := spawnLevelStepperRects(s, card)
	if !show {
		return
	}
	label := "Floor " + signedLevelLabel(clampLevel(level))
	lw := render.MeasureRichText(font, label, editorFontHint, 1).X
	render.DrawRichText(font, label, rl.NewVector2(minus.X-lw-8, minus.Y+4), editorFontHint, 1, theme.TextMuted)
	drawStepperButtons(font, minus, plus)
}

// handleSpawnLevelClick runs the Floor [-]/[+] steppers for *level, returning true
// when it consumed the click (multi-level maps only).
func handleSpawnLevelClick(s *State, card rl.Rectangle, level *int, mp rl.Vector2) bool {
	minus, plus, show := spawnLevelStepperRects(s, card)
	if !show {
		return false
	}
	switch {
	case pointIn(mp, minus):
		adjustSpawnLevel(s, level, -1)
		return true
	case pointIn(mp, plus):
		adjustSpawnLevel(s, level, +1)
		return true
	}
	return false
}

// stepperButtonPair lays out the "−"/"+" square button pair (each btnW×btnH,
// gap apart) starting at (x,y). Shared by every numeric stepper so the −/+ sizing
// can't drift between callers (stepperRow's value-left form + stepperFor's
// right-anchored form).
func stepperButtonPair(x, y, btnW, btnH, gap float32) (minus, plus rl.Rectangle) {
	minus = rl.NewRectangle(x, y, btnW, btnH)
	plus = rl.NewRectangle(minus.X+minus.Width+gap, y, btnW, btnH)
	return minus, plus
}

// dimStepperValueW is the value-cell width of the width/height dimension steppers,
// shared by the Map panel and the New Map modal so the two read identically.
const dimStepperValueW = float32(96)

// stepperRow lays out a numeric stepper at (x,y): a value cell of width valueW,
// then two square "−"/"+" buttons each preceded by gap.
func stepperRow(x, y, valueW, gap float32) (value, minus, plus rl.Rectangle) {
	value = rl.NewRectangle(x, y, valueW, modalBtnH)
	minus, plus = stepperButtonPair(value.X+value.Width+gap, y, modalBtnH, modalBtnH, gap)
	return value, minus, plus
}

// doorSpawnByName finds the door spawn with the given name.
func doorSpawnByName(spawns []core.DoorSpawn, name string) (core.DoorSpawn, bool) {
	for _, d := range spawns {
		if d.Name == name {
			return d, true
		}
	}
	return core.DoorSpawn{}, false
}

// drawDoorLinks renders the door-link diagnostic overlay: a connector to each
// door's same-map target, a warning ring on unresolved targets, a neutral ring
// on cross-map doors. Doors are few, so this isn't culled.
func drawDoorLinks(s *State, cell float32) {
	// Empty target = unset (self-link); otherwise use the self-portal test.
	sameMap := func(target string) bool { return target == "" || core.IsSelfPortal(s.area, target) }
	for _, d := range s.area.DoorSpawns {
		cx, cy := s.rect.tileCenter(d.TileX, d.TileZ)
		if sameMap(d.TargetMap) {
			if tgt, ok := doorSpawnByName(s.area.DoorSpawns, d.TargetDoor); ok && d.TargetDoor != "" {
				tx, ty := s.rect.tileCenter(tgt.TileX, tgt.TileZ)
				rl.DrawLineEx(rl.NewVector2(cx, cy), rl.NewVector2(tx, ty), 2, doorLinkColor)
				rl.DrawCircleV(rl.NewVector2(tx, ty), 3.5, doorLinkColor)
			} else {
				rl.DrawCircleLines(int32(cx), int32(cy), cell*0.46, doorLinkWarnColor)
			}
		} else {
			rl.DrawCircleLines(int32(cx), int32(cy), cell*0.42, doorLinkExternalColor)
		}
	}
}

// overlayGutterPad insets grid-corner overlays (minimap, brush recents) far
// enough to clear the canvas scrollbar gutters: scrollbarThickness + slack.
// Derived so a future scrollbar bump can't silently break the clearance.
const overlayGutterPad = scrollbarThickness + 5

// overlayMinGridDim is the minimum grid (canvas) dimension below which the
// grid-corner overlays hide — the shared width gate for the minimap and the
// recent-brush row, so the two visibility checks can't drift apart.
const overlayMinGridDim = float32(260)

// overlayMinGridHeight gates grid-corner overlays that need vertical room (the
// recent-brush swatch row), companion to overlayMinGridDim's width gate.
const overlayMinGridHeight = float32(200)

// overlayBackingInset expands a floating-overlay's content rect into its backing
// card; shared so the minimap, its legend, and the brush-recents row can't drift.
const overlayBackingInset = float32(4)

// drawOverlayBacking paints the translucent card + dim border behind a floating
// grid overlay (minimap / legend / brush-recents), wrapping the content rect by
// overlayBackingInset so tone and inset stay identical across all three.
func drawOverlayBacking(inner rl.Rectangle) {
	r := rl.NewRectangle(inner.X-overlayBackingInset, inner.Y-overlayBackingInset,
		inner.Width+2*overlayBackingInset, inner.Height+2*overlayBackingInset)
	rl.DrawRectangleRec(r, withAlpha(panelBackingColor, 214))
	rl.DrawRectangleLinesEx(r, 1, editorBorderDim)
}

// minimapRect computes the overview minimap's on-screen rect (bottom-right of
// the grid) and whether it shows (hidden when no map / grid too small). Shared
// by draw and click-to-jump.
func minimapRect(s *State) (rl.Rectangle, bool) {
	if s.area.Width == 0 || s.area.Height == 0 || s.rect.cellPx <= 0 {
		return rl.Rectangle{}, false
	}
	if s.rect.grid.Width < overlayMinGridDim || s.rect.grid.Height < overlayMinGridDim {
		return rl.Rectangle{}, false
	}
	const maxDim = float32(150)
	const pad = overlayGutterPad // clears the canvas scrollbar gutters
	aw, ah := float32(s.area.Width), float32(s.area.Height)
	scale := maxDim / aw
	if h := maxDim / ah; h < scale {
		scale = h
	}
	mw, mh := aw*scale, ah*scale
	gx := s.rect.grid.X + s.rect.grid.Width - mw - pad
	gy := s.rect.grid.Y + s.rect.grid.Height - mh - pad
	return rl.NewRectangle(gx, gy, mw, mh), true
}

// drawMinimap paints the overview: floor base, wall pixels, entity dots, and a
// viewport frame. Pixel-space iteration bounds cost to ~150² regardless of map
// size. Click-to-jump is in updateMouse.
func drawMinimap(s *State) {
	mr, ok := minimapRect(s)
	if !ok {
		return
	}
	scale := mr.Width / float32(s.area.Width)
	// Reuse the epoch-keyed column-top cache rather than re-walking each voxel column
	// per pixel (and re-walking the SAME tile once per covering pixel when the map is
	// smaller than the minimap). columnTopLevel needs refreshElevGrid to have run this
	// frame; the 3D path never calls it, so ensure it here — idempotent + cheap when
	// already current for this contentEpoch.
	refreshElevGrid(s)
	drawOverlayBacking(mr)
	rl.DrawRectangleRec(mr, minimapFloorCol)

	wallCol := minimapWallCol
	wpx, hpx := int(mr.Width), int(mr.Height)
	for py := 0; py < hpx; py++ {
		tz := int(float32(py) / scale)
		if tz >= s.area.Height {
			break
		}
		for px := 0; px < wpx; px++ {
			tx := int(float32(px) / scale)
			if tx >= s.area.Width {
				break
			}
			// Paint a pixel where a tile rises above the walkable baseline
			// (cliff/wall = structure); pits below stay blank.
			if columnTopLevel(tx, tz) > core.ElevationBaseline {
				rl.DrawPixel(int32(mr.X)+int32(px), int32(mr.Y)+int32(py), wallCol)
			}
		}
	}

	dot := func(tx, tz int, col rl.Color) {
		rl.DrawRectangle(int32(mr.X+float32(tx)*scale), int32(mr.Y+float32(tz)*scale), 2, 2, col)
	}
	for _, m := range minimapMarkers {
		m.each(s, func(tx, tz int) { dot(tx, tz, m.col) })
	}

	// Viewport frame — the slice currently visible in the grid pane.
	w, h := float32(s.area.Width), float32(s.area.Height)
	vx0 := core.Clamp((s.rect.grid.X-s.rect.gridX)/s.rect.cellPx, 0, w)
	vx1 := core.Clamp((s.rect.grid.X+s.rect.grid.Width-s.rect.gridX)/s.rect.cellPx, 0, w)
	vz0 := core.Clamp((s.rect.grid.Y-s.rect.gridY)/s.rect.cellPx, 0, h)
	vz1 := core.Clamp((s.rect.grid.Y+s.rect.grid.Height-s.rect.gridY)/s.rect.cellPx, 0, h)
	rl.DrawRectangleLinesEx(
		rl.NewRectangle(mr.X+vx0*scale, mr.Y+vz0*scale, (vx1-vx0)*scale, (vz1-vz0)*scale),
		1, minimapViewportFrame)
}

// minimapMarker is one overview marker kind: its legend label, dot color, and a
// visitor that plots each tile of that kind. drawMinimap and drawMinimapLegend
// BOTH walk this one table, so a marker can't be added to the dots and forgotten
// in the legend (or vice-versa). Order is shared draw + legend order.
type minimapMarker struct {
	label string
	col   rl.Color
	each  func(s *State, plot func(tx, tz int))
}

var minimapMarkers = []minimapMarker{
	{"Start", render.MarkerStart, func(s *State, plot func(tx, tz int)) {
		plot(s.area.StartTileX, s.area.StartTileZ)
	}},
	{"Pack", markerPackDot, func(s *State, plot func(tx, tz int)) {
		for _, p := range s.area.PackSpawns {
			if len(p.Members) > 0 {
				plot(p.TileX, p.TileZ)
			}
		}
	}},
	{"Chest", render.MarkerChest, func(s *State, plot func(tx, tz int)) {
		for _, c := range s.area.ChestSpawns {
			plot(c.TileX, c.TileZ)
		}
	}},
	{"Door", render.MarkerDoor, func(s *State, plot func(tx, tz int)) {
		for _, d := range s.area.DoorSpawns {
			plot(d.TileX, d.TileZ)
		}
	}},
	{"Crystal", render.MarkerCrystal, func(s *State, plot func(tx, tz int)) {
		for _, c := range s.area.CrystalSpawns {
			plot(c.TileX, c.TileZ)
		}
	}},
}

// drawMinimapLegend paints the marker-color key to the left of the minimap (a small
// card), so the overview dots aren't unlabeled. Hidden whenever the minimap is.
func drawMinimapLegend(s *State, font rl.Font, theme render.Theme) {
	mr, ok := minimapRect(s)
	if !ok {
		return
	}
	const rowH = float32(15)
	const padX = float32(8)
	const swatch = float32(9)
	w := float32(78)
	h := float32(len(minimapMarkers))*rowH + 10
	x := mr.X - w - 8
	y := mr.Y + mr.Height - h // bottom-aligned with the minimap
	if x < s.rect.grid.X+4 {
		return // no room to the left on a narrow canvas — drop the legend, keep the map
	}
	drawOverlayBacking(rl.NewRectangle(x, y, w, h))
	for i, e := range minimapMarkers {
		ry := y + 5 + float32(i)*rowH
		rl.DrawRectangle(int32(x+padX), int32(ry+2), int32(swatch), int32(swatch), e.col)
		rl.DrawRectangleLines(int32(x+padX), int32(ry+2), int32(swatch), int32(swatch), entityMarkerOutline)
		render.DrawRichText(font, e.label, rl.NewVector2(x+padX+swatch+6, ry), editorFontHint, 1, theme.TextHint)
	}
}

// brushRecentsVisible reports whether the recent-brush swatch row should show.
func brushRecentsVisible(s *State) bool {
	return len(s.recentBrushes) > 0 && s.rect.grid.Width >= overlayMinGridDim && s.rect.grid.Height >= overlayMinGridHeight
}

// brushRecentRect is the i-th recent-brush swatch rect (grid bottom-left).
// Shared by draw + click hit-test.
func brushRecentRect(s *State, i int) rl.Rectangle {
	const sw, gap = float32(26), float32(4)
	const pad = overlayGutterPad // clears the bottom scrollbar gutter
	x0 := s.rect.grid.X + pad
	y := s.rect.grid.Y + s.rect.grid.Height - sw - pad
	return rl.NewRectangle(x0+float32(i)*(sw+gap), y, sw, sw)
}

func recentSwatchColor(ref brushRef) rl.Color {
	palette := layerBrushes[ref.layer]
	if ref.idx < 0 || ref.idx >= len(palette) {
		return recentSwatchFallback
	}
	return palette[ref.idx].Color
}

// drawBrushRecents paints the recent-brush quick-pick row (newest at left).
// Each swatch shows the brush color + layer initial; clicking jumps to it.
func drawBrushRecents(s *State, font rl.Font) {
	if !brushRecentsVisible(s) {
		return
	}
	n := len(s.recentBrushes)
	first := brushRecentRect(s, 0)
	last := brushRecentRect(s, n-1)
	drawOverlayBacking(rl.NewRectangle(first.X, first.Y, (last.X+last.Width)-first.X, first.Height))
	mp := frameMouse
	for i, ref := range s.recentBrushes {
		r := brushRecentRect(s, i)
		rl.DrawRectangleRec(r, recentSwatchColor(ref))
		border := editorBorderDim
		if pointIn(mp, r) {
			border = editorBorderActive
		}
		rl.DrawRectangleLinesEx(r, 1, border)
		if ln := layerName(ref.layer); len(ln) > 0 {
			render.DrawTextWithShadow(font, ln[:1], r.X+3, r.Y+1, editorFontTick, charGlyphFG)
		}
	}
}

// eyeBoxRect is the visibility-toggle box at the right edge of a Levels-panel row.
func eyeBoxRect(r rl.Rectangle) rl.Rectangle {
	const eye = float32(20)
	return rl.NewRectangle(r.X+r.Width-6-eye-6, r.Y+(r.Height-eye)/2, eye, eye)
}

// drawLayerEye paints the visibility toggle as an almond eye: two lids meeting
// at the corners, with a pupil when shown or a strike-through when hidden.
func drawLayerEye(r rl.Rectangle, open, hover bool) {
	cx := r.X + r.Width/2
	cy := r.Y + r.Height/2
	col := layerEyeNormal
	if !open {
		col = layerEyeDim
	}
	if hover {
		col = layerEyeHover
	}
	hw := r.Width * 0.40 // half-width: corners sit hw from center
	h := r.Width * 0.26  // lid bulge above/below the centerline
	const seg = 10
	// Lids are symmetric sine arcs (zero at corners, peak at middle).
	var prevTop, prevBot rl.Vector2
	for i := 0; i <= seg; i++ {
		t := float32(i) / float32(seg)
		x := cx - hw + t*2*hw
		off := h * float32(math.Sin(float64(t)*math.Pi))
		top := rl.NewVector2(x, cy-off)
		bot := rl.NewVector2(x, cy+off)
		if i > 0 {
			rl.DrawLineEx(prevTop, top, 1.6, col)
			rl.DrawLineEx(prevBot, bot, 1.6, col)
		}
		prevTop, prevBot = top, bot
	}
	if open {
		rl.DrawCircleV(rl.NewVector2(cx, cy), r.Width*0.15, col) // pupil
	} else {
		// Strike-through corner to corner = hidden.
		rl.DrawLineEx(rl.NewVector2(cx-hw-2, cy-h-2), rl.NewVector2(cx+hw+2, cy+h+2), 2, col)
	}
}

// --- Levels panel ----------------------------------------------------------
//
// Elevation-level panel: a header (label + −/+ range steppers) then one row per
// level 0..topLevel, each with a visibility eye + active-level highlight.
// Clicking a row makes that level active; clicking its eye hides/shows it.

const maxVisibleLevelRows = 8

// visibleLevelRows is how many level rows the panel shows: bottomLevel..topLevel,
// capped at maxVisibleLevelRows (the window then scrolls — see levelScrollBase).
func visibleLevelRows(s *State) int {
	n := s.topLevel - s.bottomLevel + 1
	if n < 1 {
		n = 1
	}
	if n > maxVisibleLevelRows {
		n = maxVisibleLevelRows
	}
	return n
}

// levelsPanelHeight is the header row plus the visible level rows.
func levelsPanelHeight(s *State) float32 {
	return float32(1+visibleLevelRows(s)) * layerTabH
}

// levelScrollBase is the level in the panel's FIRST row (row i shows base+i).
// Scrolls to keep the active level on screen; clamped to the span.
func levelScrollBase(s *State) int {
	rows := visibleLevelRows(s)
	base := s.bottomLevel
	if s.editLevel > base+rows-1 {
		base = s.editLevel - rows + 1 // scroll up to reveal the active level
	}
	if hi := s.topLevel - rows + 1; base > hi {
		base = hi
	}
	if base < s.bottomLevel {
		base = s.bottomLevel
	}
	return base
}

func levelHeaderRect(s *State) rl.Rectangle {
	return rl.NewRectangle(s.rect.levels.X, s.rect.levels.Y, s.rect.levels.Width, layerTabH)
}

// levelStepperRects returns the header's range steppers.
func levelStepperRects(s *State) (minus, plus rl.Rectangle) {
	h := levelHeaderRect(s)
	const bw = float32(22)
	plus = rl.NewRectangle(h.X+h.Width-6-bw, h.Y+(h.Height-bw)/2, bw, bw)
	minus = rl.NewRectangle(plus.X-4-bw, h.Y+(h.Height-bw)/2, bw, bw)
	return
}

func levelRowRect(s *State, i int) rl.Rectangle {
	return rl.NewRectangle(
		s.rect.levels.X,
		s.rect.levels.Y+layerTabH+float32(i)*layerTabH,
		s.rect.levels.Width,
		layerTabH,
	)
}

func levelEyeRect(s *State, i int) rl.Rectangle {
	return eyeBoxRect(levelRowRect(s, i))
}

// levelRowAt returns the LEVEL under p in the panel's row area, or -1
// (mapped through levelScrollBase so a scrolled panel selects right).
func levelRowAt(s *State, p rl.Vector2) int {
	base := levelScrollBase(s)
	for i := 0; i < visibleLevelRows(s); i++ {
		if pointIn(p, levelRowRect(s, i)) {
			return base + i
		}
	}
	return -1
}

func drawLevelsPanel(s *State, font rl.Font, theme render.Theme) {
	rl.DrawRectangleRec(s.rect.levels, bgWindow)
	mp := frameMouse
	hdr := levelHeaderRect(s)
	render.DrawTextWithShadow(font, "Levels", hdr.X+10, hdr.Y+(hdr.Height-bodyLineH)/2, editorFontBody, theme.TextPrimary)
	minus, plus := levelStepperRects(s)
	drawLevelStepper(font, minus, "-", pointIn(mp, minus))
	drawLevelStepper(font, plus, "+", pointIn(mp, plus))
	base := levelScrollBase(s)
	for i := 0; i < visibleLevelRows(s); i++ {
		lvl := base + i
		r := levelRowRect(s, i)
		active := lvl == s.editLevel
		hidden := s.levelHidden[lvl]
		bg := bgPanel
		border := editorBorderDim
		text := theme.TextMuted
		if active {
			bg = bgActive
			border = editorBorderActive
			text = theme.TextPrimary
		} else if pointIn(mp, r) {
			bg = bgEntryHover
		}
		if hidden {
			text = hiddenTabTextColor
		}
		inner := rl.NewRectangle(r.X+6, r.Y+3, r.Width-12, r.Height-6)
		rl.DrawRectangleRec(inner, bg)
		rl.DrawRectangleLinesEx(inner, 1, border)
		label := "Ground"
		if lvl != core.ElevationBaseline {
			label = "Level " + signedLevelLabel(lvl)
		}
		render.DrawTextWithShadow(font, label, inner.X+10, inner.Y+(inner.Height-bodyLineH)/2, editorFontBody, text)
		eye := levelEyeRect(s, i)
		// The active level is always shown, so its eye is locked-open.
		drawLevelEye(eye, !hidden, pointIn(mp, eye) && !active, active)
	}
}

// drawLevelEye draws a Levels-panel row's visibility eye. The active level's eye
// is LOCKED (always shown), drawn open + non-hover.
func drawLevelEye(r rl.Rectangle, open, hover, locked bool) {
	if locked {
		drawLayerEye(r, true, false)
		return
	}
	drawLayerEye(r, open, hover)
}

func drawLevelStepper(font rl.Font, r rl.Rectangle, label string, hover bool) {
	bg := bgPanel
	if hover {
		bg = bgEntryHover
	}
	rl.DrawRectangleRec(r, bg)
	rl.DrawRectangleLinesEx(r, 1, editorBorderDim)
	m := render.MeasureRichText(font, label, editorFontBody, 1)
	render.DrawRichText(font, label, rl.NewVector2(r.X+(r.Width-m.X)/2, r.Y+(r.Height-float32(editorFontBody))/2), editorFontBody, 1, tooltipText)
}

// --- Palette ---------------------------------------------------------------

func paletteToolAt(s *State, p rl.Vector2) int {
	if !pointIn(p, s.rect.palette) {
		return -1
	}
	// Reject clicks in the heading band: drawPalette scissor-clips entries below
	// it, so a scrolled entry overlapping the heading is hidden but would still hit-test.
	if p.Y < s.rect.palette.Y+headerReserve {
		return -1
	}
	palette := layerBrushes[s.layer]
	for i := range palette {
		r := paletteEntryRect(s, i)
		if pointIn(p, r) {
			return i
		}
	}
	return -1
}

const (
	paletteRowH      = float32(32)
	paletteRowStride = paletteRowH + 4 // rowH + row spacing
	// headerReserve is the panel heading band (BRUSHES / MAP); body + scissor clip
	// sit below it. Must clear the heading underline tick from render.DrawHeading.
	headerReserve = float32(40)
	// paletteHintGap is the gap below the brush list before the hint footer;
	// paletteHintStride is the per-hint-line baseline pitch. Shared by
	// paletteContentHeight and drawBrushHints so the footer can't drift.
	paletteHintGap    = float32(12)
	paletteHintStride = float32(16)
)

// paletteHints is the keyboard-shortcut cheat sheet below the brush list.
// paletteContentHeight reads len(paletteHints), so the layout can't drift.
var paletteHints = []string{
	"?: all shortcuts",
	"L-drag: paint",
	"Alt+click: eyedropper",
	"R-click: tile menu",
	"R-click entity: edit/move",
	"Shift+drag: rect",
	"Ctrl+click: fill region",
	"1..9 / Sh+1..9: brush",
	"[ ] brush size",
	"Tab: next layer",
	"Ctrl+C/V/X: copy/paste/cut",
	"Ctrl+Z undo / Y redo",
	"wheel or +/-: zoom",
	"arrows / R-drag: pan",
	"I: 3D view · Q/E: turn",
	"F5 playtest",
	"Esc back",
}

func paletteEntryRect(s *State, i int) rl.Rectangle {
	y := s.rect.palette.Y + headerReserve + float32(i)*paletteRowStride - s.paletteScroll[s.layer]
	return rl.NewRectangle(s.rect.palette.X+8, y, s.rect.palette.Width-16, paletteRowH)
}

// visiblePaletteRange returns the [start, end) index range of palette entries
// whose rect intersects the visible band, so drawPalette iterates only those.
// Clamped to [0, n].
func visiblePaletteRange(s *State, n int) (int, int) {
	if n <= 0 {
		return 0, 0
	}
	scroll := s.paletteScroll[s.layer]
	top := s.rect.palette.Y + headerReserve
	bot := s.rect.palette.Y + s.rect.palette.Height
	// First i where (entryStart + paletteRowH) >= top:
	startF := (scroll - paletteRowH) / paletteRowStride
	endF := (scroll + (bot - top)) / paletteRowStride
	start := int(startF)
	end := int(endF) + 1
	if start < 0 {
		start = 0
	}
	if end > n {
		end = n
	}
	if start > end {
		start = end
	}
	return start, end
}

// paletteContentHeight is the pixel height to render the active layer's full
// brush list + hint footer. Used by ScrollPalette to clamp the offset.
func paletteContentHeight(s *State) float32 {
	palette := layerBrushes[s.layer]
	return headerReserve + float32(len(palette))*paletteRowStride + paletteHintGap + float32(len(paletteHints))*paletteHintStride + paletteHintStride
}

// clampScroll advances *off by dy (positive = down), clamping to [0, content-panelH]
// (0 when the content fits). No-op for a zero-height panel — content is only measured
// then. Single home for the palette/metadata scroll-clamp body.
func clampScroll(off *float32, dy, panelH float32, content func() float32) {
	if panelH <= 0 {
		return
	}
	max := content() - panelH
	if max < 0 {
		max = 0
	}
	v := *off + dy
	if v < 0 {
		v = 0
	}
	if v > max {
		v = max
	}
	*off = v
}

// ScrollPalette adjusts the active layer's palette scroll by dy px (positive =
// down), clamped to [0, max].
func ScrollPalette(s *State, dy float32) {
	clampScroll(&s.paletteScroll[s.layer], dy, s.rect.palette.Height, func() float32 { return paletteContentHeight(s) })
}

func drawPalette(s *State, font rl.Font, theme render.Theme) {
	rl.DrawRectangleRec(s.rect.palette, bgPaletteCol)
	rl.DrawLineEx(
		rl.NewVector2(s.rect.palette.X+s.rect.palette.Width, s.rect.palette.Y),
		rl.NewVector2(s.rect.palette.X+s.rect.palette.Width, s.rect.palette.Y+s.rect.palette.Height),
		1, outlineHard)

	render.DrawHeading(font, "BRUSHES", int32(s.rect.palette.X)+panelHeadingInsetX, int32(s.rect.palette.Y)+panelHeadingInsetY, theme.BorderStrong)

	// Reclamp scroll (entry count can change between frames).
	ScrollPalette(s, 0)

	// Clip the palette region so off-screen entries (and the hint footer) don't
	// paint over the topbar or grid panel.
	rl.BeginScissorMode(int32(s.rect.palette.X), int32(s.rect.palette.Y+headerReserve),
		int32(s.rect.palette.Width), int32(s.rect.palette.Height-headerReserve))
	defer rl.EndScissorMode()

	palette := layerBrushes[s.layer]
	labels := paletteLabels[s.layer]
	mp := frameMouse // Draw()-cached; avoids a per-entry CGo poll
	// Window iteration to rows that can actually render (props is ~30 entries).
	visStart, visEnd := visiblePaletteRange(s, len(palette))
	for i := visStart; i < visEnd; i++ {
		b := palette[i]
		r := paletteEntryRect(s, i)
		active := s.brushIdx[s.layer] == i
		hovered := pointIn(mp, r)
		drawBrushSwatchRow(font, r, labels[i], s.layer, b, active, hovered, 16)
	}

	y := s.rect.palette.Y + headerReserve + float32(len(palette))*paletteRowStride + paletteHintGap - s.paletteScroll[s.layer]
	for _, h := range paletteHints {
		render.DrawRichText(font, h, rl.NewVector2(s.rect.palette.X+12, y), editorFontAccent, 1, theme.TextHint)
		y += paletteHintStride
	}
}

// Brush-swatch row geometry: the color box is inset brushSwatchInset on all
// sides, brushSwatchW wide; the label starts brushLabelInsetX from the row left.
const (
	brushSwatchInset = float32(6)
	brushSwatchW     = float32(20)
	brushLabelInsetX = float32(34)
)

// drawBrushSwatchRow renders one selectable brush entry: row bg (active/hover),
// the colored swatch box (sentinel hatch when applicable), and a label.
func drawBrushSwatchRow(font rl.Font, r rl.Rectangle, label string, layer Layer, brush Brush, active, hovered bool, labelSize float32) {
	bg := bgEntry
	if active {
		bg = bgActive
	} else if hovered {
		bg = bgRowHover
	}
	rl.DrawRectangleRec(r, bg)
	border := editorBorderDim
	if active {
		border = editorBorderActive
	}
	rl.DrawRectangleLinesEx(r, 1, border)

	swatch := rl.NewRectangle(r.X+brushSwatchInset, r.Y+brushSwatchInset, brushSwatchW, r.Height-2*brushSwatchInset)
	rl.DrawRectangleRec(swatch, brush.Color)
	sentinel := isSentinelBrush(layer, brush.Char)
	if sentinel {
		drawSentinelHatch(swatch)
	}
	rl.DrawRectangleLinesEx(swatch, 1, swatchEdge)

	nameCol := textEntry
	if sentinel {
		nameCol = sentinelLabelColor
	}
	render.DrawRichText(font, label,
		rl.NewVector2(r.X+brushLabelInsetX, r.Y+(r.Height-labelSize)/2),
		labelSize, 1, nameCol)
}

// isSentinelBrush reports whether (layer, char) is a "semantic" brush (Auto /
// Force-empty / None) that paints no visible tile — the palette hatches those.
// Per layer via layerDefs in layerdef.go.
func isSentinelBrush(layer Layer, char byte) bool {
	return layerDefs[layer].isSentinel(char)
}

// drawSentinelHatch overlays diagonal stripes onto a swatch so it reads as
// "semantic" rather than a literal color.
func drawSentinelHatch(r rl.Rectangle) {
	stripe := sentinelHatchStripe
	// Scissor to the swatch so the diagonal strokes can't bleed past its edges
	// (DrawLineEx doesn't clip; the per-endpoint clamp left a ~1px corner overflow).
	rl.BeginScissorMode(int32(r.X), int32(r.Y), int32(r.Width), int32(r.Height))
	steps := int(r.Width + r.Height)
	for i := 0; i < steps; i += 4 {
		rl.DrawLineEx(rl.NewVector2(r.X+float32(i), r.Y), rl.NewVector2(r.X, r.Y+float32(i)), 1, stripe)
	}
	rl.EndScissorMode()
}

// --- Metadata panel --------------------------------------------------------

type metaRect struct {
	nameLabel, nameField                 rl.Rectangle
	matLabel                             rl.Rectangle
	matButtons                           []rl.Rectangle
	weatherLabel                         rl.Rectangle
	weatherButtons                       []rl.Rectangle
	quietLabel, quietField               rl.Rectangle
	dimsLabel                            rl.Rectangle
	widthValue, widthMinus, widthPlus    rl.Rectangle
	heightValue, heightMinus, heightPlus rl.Rectangle
	pathLabel, pathValue                 rl.Rectangle
	// reachLabel + reachArea bound the clickable reachability badge; reachArea
	// covers the whole fill so any click there opens the Validate modal.
	reachLabel, reachArea rl.Rectangle
}

// Metadata-sidebar row metrics — the strides metadataRects stacks controls at.
const (
	metaLabelH     = float32(18) // field-caption height
	metaLabelGap   = float32(22) // caption row → the field/control it labels
	metaRowGap     = float32(42) // a labeled block → the next caption
	metaStepperGap = float32(38) // between the stacked width / height steppers
	metaFieldH     = float32(30) // text-field height (name / quiet / path)
	// Trailing layout (path block → reachability badge → bottom margin); named so
	// metadataContentHeight derives the panel height from the SAME numbers
	// metadataRects lays out with, no measuring build needed.
	metaPathBlockH       = float32(64)  // path label-top → reachability badge top
	metaReachAreaH       = float32(140) // reachability badge clickable region
	metaContentBottomPad = float32(16)  // slack below the badge
)

// Per-cell zoom thresholds (px) below which a piece of grid chrome turns off
// (would no longer fit / reads as noise).
const (
	charOverlayMinCell = float32(14) // active-layer per-tile glyph overlay
	axisTickMinCell    = float32(18) // top/left axis tick-number labels
)

// Shared entity-marker radius/inset fractions (of cell size); the drag ghost reads the
// SAME fraction as the live marker so the preview can't drift. Chest/door/crystal carry
// distinct shapes, so each names its own fraction here rather than scattering literals.
const (
	packMarkerRadiusFrac  = float32(0.32) // pack circle radius — live marker + drag ghost
	startMarkerRadiusFrac = float32(0.36) // player-start circle radius — live marker + drag ghost
	chestMarkerInsetFrac  = float32(0.25) // chest square inset from the tile edge
	doorMarkerInsetXFrac  = float32(0.30) // door rectangle horizontal inset
	doorMarkerInsetYFrac  = float32(0.12) // door rectangle vertical inset (taller than wide)
	crystalMarkerRadFrac  = float32(0.30) // crystal diamond radius
	entityGhostInsetFrac  = float32(0.22) // chest/door drag-move ghost square inset
	decorCellInsetFrac    = float32(0.28) // per-cell decor square inset from the tile edge
	propCellRadiusFrac    = float32(0.36) // per-cell prop circle radius
	doorFacingArrowFrac   = float32(0.28) // door facing arrow length (fraction of cell)
	startFacingArrowFrac  = float32(0.42) // player-start facing arrow length (fraction of cell)
	packLabelFontFrac     = float32(0.42) // pack-marker leader-initial font size (fraction of cell)
	packBadgeFontFrac     = float32(0.28) // pack-marker "xN" count badge font size (fraction of cell)
)

// drawFacingArrow strokes a facing arrow from tile center (cx,cy) toward `facing`,
// its length a fraction of the cell. Shared by the door + player-start markers.
func drawFacingArrow(cx, cy float32, facing int, cell, lenFrac, thick float32, col rl.Color) {
	dx, dz := core.FacingVector(facing)
	tipX := cx + float32(dx)*cell*lenFrac
	tipY := cy + float32(dz)*cell*lenFrac
	rl.DrawLineEx(rl.NewVector2(cx, cy), rl.NewVector2(tipX, tipY), thick, col)
}

// metaRect geometry depends only on the panel rect + scroll (material count is
// fixed at init), so cache it: rebuilds only when the sidebar moves/resizes or
// scrolls, sparing the per-frame matButtons slice alloc on the draw/click paths.
var (
	metaRectCache                                   metaRect
	metaRectReady                                   bool
	metaRectX, metaRectY, metaRectW, metaRectScroll float32
)

func metadataRects(s *State) metaRect {
	if metaRectReady && metaRectX == s.rect.metadata.X && metaRectY == s.rect.metadata.Y &&
		metaRectW == s.rect.metadata.Width && metaRectScroll == s.metadataScroll {
		return metaRectCache
	}
	x := s.rect.metadata.X + 14
	w := s.rect.metadata.Width - 28
	y := s.rect.metadata.Y + headerReserve - s.metadataScroll

	r := metaRect{}

	r.nameLabel = rl.NewRectangle(x, y, w, metaLabelH)
	y += metaLabelGap
	r.nameField = rl.NewRectangle(x, y, w, metaFieldH)
	y += metaRowGap

	r.matLabel = rl.NewRectangle(x, y, w, metaLabelH)
	y += metaLabelGap
	r.matButtons = equalButtonRow(x, y, w, modalBtnH, len(core.MaterialOptions))
	y += metaRowGap

	r.weatherLabel = rl.NewRectangle(x, y, w, metaLabelH)
	y += metaLabelGap
	r.weatherButtons = equalButtonRow(x, y, w, modalBtnH, len(core.WeatherModeOptions))
	y += metaRowGap

	r.quietLabel = rl.NewRectangle(x, y, w, metaLabelH)
	y += metaLabelGap
	r.quietField = rl.NewRectangle(x, y, w, metaFieldH)
	y += metaRowGap

	r.dimsLabel = rl.NewRectangle(x, y, w, metaLabelH)
	y += metaLabelGap
	r.widthValue, r.widthMinus, r.widthPlus = stepperRow(x, y, dimStepperValueW, tightBtnGap)
	y += metaStepperGap
	r.heightValue, r.heightMinus, r.heightPlus = stepperRow(x, y, dimStepperValueW, tightBtnGap)
	y += metaRowGap

	// On-disk path readout (area-wide; player start + door facing are per-entity now).
	r.pathLabel = rl.NewRectangle(x, y, w, metaLabelH)
	r.pathValue = rl.NewRectangle(x, y+metaLabelGap, w, metaFieldH)
	// Reachability badge: label a row below the path; the clickable region covers
	// the OK/warning panel below it.
	reachY := y + metaPathBlockH
	r.reachLabel = rl.NewRectangle(x, reachY, w, metaLabelH)
	r.reachArea = rl.NewRectangle(x, reachY, w, metaReachAreaH)

	metaRectCache = r
	metaRectReady = true
	metaRectX, metaRectY = s.rect.metadata.X, s.rect.metadata.Y
	metaRectW, metaRectScroll = s.rect.metadata.Width, s.metadataScroll
	return r
}

// metadataContentHeight is the pixel height to render the full metadata panel.
// Used by ScrollMetadata to clamp the offset. Derived from the same stride
// constants metadataRects lays out with — no measuring build, so it never
// thrashes the (scroll-keyed) metaRect cache.
func metadataContentHeight(s *State) float32 {
	// Unscrolled span from the panel top: header + the four label/field blocks
	// + the path-block-to-badge gap + the badge + bottom slack.
	return headerReserve +
		5*metaLabelGap + 5*metaRowGap + metaStepperGap +
		metaPathBlockH + metaReachAreaH + metaContentBottomPad
}

// metadataRowStride is the wheel-scroll step (~one field per notch).
const metadataRowStride = float32(42)

// ScrollMetadata adjusts the metadata panel's scroll by dy px (positive = down),
// clamped to [0, max].
func ScrollMetadata(s *State, dy float32) {
	clampScroll(&s.metadataScroll, dy, s.rect.metadata.Height, func() float32 { return metadataContentHeight(s) })
}

func handleMetadataClick(s *State, p rl.Vector2) bool {
	if !pointIn(p, s.rect.metadata) {
		return false
	}
	// Reject clicks in the MAP heading band (matches the drawMetadata scissor).
	if p.Y < s.rect.metadata.Y+headerReserve {
		return true
	}
	mr := metadataRects(s)
	// Reachability badge: any click in the region opens the validate modal.
	// BEFORE the field-focus checks so it wins even if a future field overlaps.
	if pointIn(p, mr.reachArea) {
		openValidateModal(s)
		return true
	}
	if pointIn(p, mr.nameField) {
		s.focus = focusName
		return true
	}
	if pointIn(p, mr.quietField) {
		s.focus = focusQuiet
		return true
	}
	for i, br := range mr.matButtons {
		if pointIn(p, br) {
			setIfChanged(s, &s.area.Materials, core.MaterialOptions[i])
			return true
		}
	}
	for i, br := range mr.weatherButtons {
		if pointIn(p, br) {
			setIfChanged(s, &s.area.WeatherMode, core.WeatherModeOptions[i])
			return true
		}
	}
	if pointIn(p, mr.widthValue) {
		s.focus = focusWidth
		s.numericBuf = ""
		return true
	}
	if pointIn(p, mr.heightValue) {
		s.focus = focusHeight
		s.numericBuf = ""
		return true
	}
	if pointIn(p, mr.widthMinus) {
		resize(s, s.area.Width-1, s.area.Height)
		return true
	}
	if pointIn(p, mr.widthPlus) {
		resize(s, s.area.Width+1, s.area.Height)
		return true
	}
	if pointIn(p, mr.heightMinus) {
		resize(s, s.area.Width, s.area.Height-1)
		return true
	}
	if pointIn(p, mr.heightPlus) {
		resize(s, s.area.Width, s.area.Height+1)
		return true
	}
	return false
}

func (s *State) activeFieldRect() rl.Rectangle {
	switch s.focus {
	case focusNewWidth, focusNewHeight:
		return newMapFieldRect(s)
	case focusFilename:
		return saveAsFieldRect(s)
	}
	mr := metadataRects(s)
	switch s.focus {
	case focusName:
		return mr.nameField
	case focusQuiet:
		return mr.quietField
	case focusWidth:
		return mr.widthValue
	case focusHeight:
		return mr.heightValue
	}
	return rl.Rectangle{}
}

// Metadata-panel reachability badge geometry. reachBadgeMaxRows caps the row
// count so the badge doesn't reflow (full list is in the Validate modal,
// openValidateModal); reachBadgeTopGap insets the badge below its label;
// reachBadgeRowH is the per-warning row pitch; reachBadgeOKRowH the OK pill height.
const (
	reachBadgeMaxRows = 4
	reachBadgeTopGap  = float32(22)
	reachBadgeRowH    = float32(22)
	reachBadgeOKRowH  = float32(30)
)

func drawMetadata(s *State, font rl.Font, theme render.Theme) {
	rl.DrawRectangleRec(s.rect.metadata, bgPaletteCol)
	rl.DrawLineEx(
		rl.NewVector2(s.rect.metadata.X, s.rect.metadata.Y),
		rl.NewVector2(s.rect.metadata.X, s.rect.metadata.Y+s.rect.metadata.Height),
		1, outlineHard)

	render.DrawHeading(font, "MAP", int32(s.rect.metadata.X)+panelHeadingInsetX, int32(s.rect.metadata.Y)+panelHeadingInsetY, theme.BorderStrong)

	// Reclamp scroll (content height varies with the badge's row count).
	ScrollMetadata(s, 0)

	// Clip the body so scrolled content can't paint into the MAP heading or below.
	rl.BeginScissorMode(int32(s.rect.metadata.X), int32(s.rect.metadata.Y+headerReserve),
		int32(s.rect.metadata.Width), int32(s.rect.metadata.Height-headerReserve))
	defer rl.EndScissorMode()

	mr := metadataRects(s)

	drawLabel(font, "Name", mr.nameLabel)
	drawTextField(font, mr.nameField, s.area.Name, s.focus == focusName)

	drawLabel(font, "Materials", mr.matLabel)
	for i, br := range mr.matButtons {
		active := s.area.Materials == core.MaterialOptions[i]
		// MaterialName is total over MaterialOptions, so ok=false is unreachable;
		// fall back to "" rather than panic in the per-frame draw loop.
		name, _ := core.MaterialName(core.MaterialOptions[i])
		drawButton(font, br, name, active)
	}

	drawLabel(font, "Weather", mr.weatherLabel)
	for i, br := range mr.weatherButtons {
		mode := core.WeatherModeOptions[i]
		drawButton(font, br, core.WeatherModeLabel(mode), s.area.WeatherMode == mode)
	}

	drawLabel(font, "Quiet message", mr.quietLabel)
	drawTextField(font, mr.quietField, s.area.QuietMessage, s.focus == focusQuiet)

	drawLabel(font, "Dimensions (click to type)", mr.dimsLabel)
	wText := fmt.Sprintf("W: %d", s.area.Width)
	if s.focus == focusWidth {
		wText = "W: " + s.numericBuf
	}
	hText := fmt.Sprintf("H: %d", s.area.Height)
	if s.focus == focusHeight {
		hText = "H: " + s.numericBuf
	}
	drawTextField(font, mr.widthValue, wText, s.focus == focusWidth)
	drawStepperButtons(font, mr.widthMinus, mr.widthPlus)
	drawTextField(font, mr.heightValue, hText, s.focus == focusHeight)
	drawStepperButtons(font, mr.heightMinus, mr.heightPlus)

	// Player start coord + facing live on the PlayerStart entity now (right-click
	// the start tile on the Entities layer); per-door facing overrides it.

	// Path readout (readonly): "(unsaved)" before the first save, else the path.
	drawLabel(font, "On-disk path", mr.pathLabel)
	pathText := s.area.Path
	if pathText == "" {
		pathText = "(unsaved)"
	}
	drawReadonlyValue(font, mr.pathValue, pathText)

	// Reachability badge: red whenever the area would fail a save-time check
	// (updated per-frame so it reflects the current edit).
	warnings := s.ReachabilityWarnings()
	drawLabel(font, "Reachability (click to validate)", mr.reachLabel)
	if len(warnings) == 0 {
		badgeValue := rl.NewRectangle(mr.reachArea.X, mr.reachArea.Y+reachBadgeTopGap, mr.reachArea.Width, reachBadgeOKRowH)
		rl.DrawRectangleRec(badgeValue, editorReachOKFill)
		rl.DrawRectangleLinesEx(badgeValue, 1, editorReachOK)
		render.DrawRichText(font, "OK", rl.NewVector2(badgeValue.X+8, badgeValue.Y+(badgeValue.Height-bodyLineH)/2), editorFontBody, 1, editorReachOKText)
	} else {
		// One row per warning, red panel + outline.
		rows := warnings
		if len(rows) > reachBadgeMaxRows {
			rows = rows[:reachBadgeMaxRows] // cap so the panel doesn't reflow
		}
		h := 10 + reachBadgeRowH*float32(len(rows))
		box := rl.NewRectangle(mr.reachArea.X, mr.reachArea.Y+reachBadgeTopGap, mr.reachArea.Width, h)
		rl.DrawRectangleRec(box, editorReachWarnFill)
		rl.DrawRectangleLinesEx(box, 1, editorReachWarn)
		for i, w := range rows {
			render.DrawRichText(font, "! "+w,
				rl.NewVector2(box.X+6, box.Y+5+float32(i)*reachBadgeRowH),
				editorFontLabel, 1, editorReachWarnText)
		}
		if len(warnings) > len(rows) {
			render.DrawRichText(font, fmt.Sprintf("(+%d more)", len(warnings)-len(rows)),
				rl.NewVector2(box.X+6, box.Y+h-18),
				editorFontAccent, 1, withAlpha(editorReachWarnText, 220))
		}
	}
}

// drawMeasureReadout floats the ruler readout beside the cursor while a Measure
// drag is active (view-independent — the box outline is top-down only).
func drawMeasureReadout(s *State, font rl.Font, theme render.Theme) {
	if s.drag != dragMeasure || s.hoverX < 0 {
		return
	}
	txt := measureLabel(s.rectAnchorX, s.rectAnchorZ, s.hoverX, s.hoverZ)
	mp := rl.GetMousePosition()
	tw := render.MeasureRichText(font, txt, editorFontBody, 1).X
	pad := float32(6)
	box := rl.NewRectangle(mp.X+14, mp.Y+14, tw+2*pad, bodyLineH+2*pad)
	render.DrawCard(int32(box.X), int32(box.Y), int32(box.Width), int32(box.Height), theme.SurfacePrimary, theme.BorderSoft, theme.BorderActive)
	render.DrawRichText(font, txt, rl.NewVector2(box.X+pad, box.Y+pad), editorFontBody, 1, theme.TextPrimary)
}

func drawLabel(font rl.Font, text string, r rl.Rectangle) {
	render.DrawRichText(font, text, rl.NewVector2(r.X, r.Y), editorFontLabel, 1, editorLabelColor)
}

func drawTextField(font rl.Font, r rl.Rectangle, text string, focused bool) {
	border := editorBorderDim
	if focused {
		border = editorBorderActive
	}
	rl.DrawRectangleRec(r, bgFieldInset)
	rl.DrawRectangleLinesEx(r, 1, border)

	display := text
	if focused {
		if math.Mod(rl.GetTime(), 1.0) > 0.5 {
			display += "_"
		} else {
			display += " "
		}
	}
	// Fit the string to the field's interior so a long value can't overrun the box
	// (modal cards carry no panel scissor). Focused fields keep the tail + caret
	// visible; unfocused ones show the head with an ellipsis.
	display = fitTextFieldText(font, display, r.Width-2*fieldTextInsetX, focused)
	render.DrawRichText(font, display, rl.NewVector2(r.X+fieldTextInsetX, r.Y+(r.Height-bodyLineH)/2), editorFontBody, 1, textEntry)
}

// fitTextFieldText trims s to fit innerW px in the body font. Focused: drop leading
// runes so the caret/tail stays on screen. Unfocused: keep the head, append "…".
func fitTextFieldText(font rl.Font, s string, innerW float32, focused bool) string {
	if innerW <= 0 || render.MeasureRichText(font, s, editorFontBody, 1).X <= innerW {
		return s
	}
	r := []rune(s)
	if focused {
		for len(r) > 1 {
			r = r[1:]
			if render.MeasureRichText(font, string(r), editorFontBody, 1).X <= innerW {
				break
			}
		}
		return string(r)
	}
	for len(r) > 0 {
		r = r[:len(r)-1]
		if render.MeasureRichText(font, string(r)+"…", editorFontBody, 1).X <= innerW {
			return string(r) + "…"
		}
	}
	return "…"
}

func drawReadonlyValue(font rl.Font, r rl.Rectangle, text string) {
	rl.DrawRectangleRec(r, bgFieldInset)
	rl.DrawRectangleLinesEx(r, 1, editorBorderInactive)
	text = fitTextFieldText(font, text, r.Width-2*fieldTextInsetX, false)
	render.DrawRichText(font, text, rl.NewVector2(r.X+fieldTextInsetX, r.Y+(r.Height-bodyLineH)/2), editorFontBody, 1, textReadonly)
}

// --- Grid ------------------------------------------------------------------

// drawGrid paints the flat-color layers stacked (floor → walls → decor → props →
// ceiling hash → entities), non-active layers dimmed.
// Column-top level cache. ElevationLevelAt walks the voxel column per call; the
// top-down grid needs it for EVERY visible cell every frame. Rebuilt only when
// the area mutates (contentEpoch) or resizes, so steady-state draws index a flat
// slice instead of re-walking thousands of columns. hasRamps rides the same scan
// so ramp-free maps skip the per-cell ramp probes entirely.
var (
	elevGridCache []int
	elevGridEpoch uint64
	elevGridW     int
	elevGridH     int
	elevGridReady bool
	elevGridRamps bool
)

func refreshElevGrid(s *State) {
	w, h := s.area.Width, s.area.Height
	if elevGridReady && elevGridEpoch == s.contentEpoch && elevGridW == w && elevGridH == h {
		return
	}
	if cap(elevGridCache) < w*h {
		elevGridCache = make([]int, w*h)
	} else {
		elevGridCache = elevGridCache[:w*h]
	}
	ramps := false
	for z := 0; z < h; z++ {
		for x := 0; x < w; x++ {
			elevGridCache[z*w+x] = s.area.ElevationLevelAt(x, z)
			if !ramps {
				if fc, ok := cellAt(s.area.Floor, x, z); ok && core.IsRampChar(fc) {
					ramps = true
				}
			}
		}
	}
	elevGridEpoch = s.contentEpoch
	elevGridW, elevGridH = w, h
	elevGridRamps = ramps
	elevGridReady = true
}

// columnTopLevel returns the cached ElevationLevelAt for an in-bounds cell.
// refreshElevGrid must have run this frame.
func columnTopLevel(x, z int) int { return elevGridCache[z*elevGridW+x] }

func drawGrid(s *State, font rl.Font) {
	rl.DrawRectangleRec(s.rect.grid, bgFieldInset)
	// Iso preview takes over the whole canvas (read-only — see iso.go), sizing
	// itself before the top-down cellPx gate.
	if s.isoView {
		drawGridIso(s, font)
		return
	}
	if s.rect.cellPx <= 0 {
		return
	}
	cell := s.rect.cellPx

	floorAlpha := layerAlpha(s, LayerFloor)
	wallAlpha := layerAlpha(s, LayerWalls)
	decorAlpha := layerAlpha(s, LayerDecor)
	propAlpha := layerAlpha(s, LayerProps)
	ceilingAlpha := layerAlpha(s, LayerCeiling)
	entityAlpha := layerAlpha(s, LayerEntities)

	// Frustum-cull tiles outside the visible grid panel (a 200×200 map is 40k
	// cells × ~6 draws otherwise). Compute the visible [xMin,xMax)×[zMin,zMax) window.
	panelX0, panelY0 := s.rect.grid.X, s.rect.grid.Y
	panelX1, panelY1 := s.rect.grid.X+s.rect.grid.Width, s.rect.grid.Y+s.rect.grid.Height
	xMin := int((panelX0 - s.rect.gridX) / cell)
	xMax := int((panelX1-s.rect.gridX)/cell) + 1
	zMin := int((panelY0 - s.rect.gridY) / cell)
	zMax := int((panelY1-s.rect.gridY)/cell) + 1
	if xMin < 0 {
		xMin = 0
	}
	if zMin < 0 {
		zMin = 0
	}
	if xMax > s.area.Width {
		xMax = s.area.Width
	}
	if zMax > s.area.Height {
		zMax = s.area.Height
	}
	// inCullWindow: is a tile inside the visible window (entity loops skip off-screen).
	inCullWindow := func(tx, tz int) bool {
		return tx >= xMin && tx < xMax && tz >= zMin && tz < zMax
	}
	// markerCulled: shared skip test for every spawn-marker loop (layer hidden or
	// off-screen), so the guard can't drift between the pack/chest/door/crystal loops.
	markerCulled := func(tx, tz int) bool {
		return s.layerHidden[LayerEntities] || !inCullWindow(tx, tz)
	}

	// Active-layer char overlay: when a glyph fits, paint the active layer's
	// tile-char per cell (only the active layer, to avoid noise). ALT-tap toggles
	// it; empty sentinels produce no glyph.
	showCharOverlay := cell >= charOverlayMinCell && s.showTileGlyphs && !s.layerHidden[s.layer]
	charFontSize := cell * 0.55
	charShadow := glyphShadow
	charFG := charGlyphFG

	// Cache column-top levels (and the has-ramps flag) for this frame; the inner
	// loop then indexes a slice instead of walking each voxel column.
	refreshElevGrid(s)

	// Per-layer visibility is read directly off s.layerHidden at each draw site
	// (indexed by Layer) — never hand-listed, so a newly-added layer can't be
	// silently omitted here. Array reads are cheap enough for the inner loop.
	for z := zMin; z < zMax; z++ {
		for x := xMin; x < xMax; x++ {
			r := s.rect.tileRect(x, z)
			// Level visibility: a tile on a hidden level isn't drawn — EXCEPT the
			// ACTIVE level (always shown) and a ramp connecting to it.
			// level (you always see the floor you're editing, so a paint can never
			// vanish behind its own hidden toggle) and a ramp that connects to the
			// active level (so transitions across a hidden floor stay routable).
			lvl := columnTopLevel(x, z)
			if lvl >= 0 && lvl <= maxEditLevel && lvl != s.editLevel &&
				s.levelHidden[lvl] && !(elevGridRamps && rampTouchesActiveLevel(s, x, z, lvl)) {
				continue
			}
			// Off-level tiles fade with distance from the active level (context).
			levelFade := levelDistanceFade(s, lvl)
			// Floor is the base, always painted. Read via cellAt like the rest of the
			// package — a ragged (short) row returns ok=false rather than panicking.
			if fc, ok := cellAt(s.area.Floor, x, z); ok && !s.layerHidden[LayerFloor] {
				rl.DrawRectangleRec(r, fadeAlpha(floorColor(fc), floorAlpha*levelFade))
			}
			// Elevation is a voxel grid: fill cells solid at the active level.
			// Scrub levels (Levels panel / PgUp-PgDn) to see each floor.
			if s.layer == LayerElevation {
				if _, solid := s.area.SolidAt(x, s.editLevel, z); solid {
					rl.DrawRectangleRec(r, fadeAlpha(elevationLevelColor(s.editLevel), 0.6))
				}
			}
			if w, ok := cellAt(s.area.Walls, x, z); ok && !s.layerHidden[LayerWalls] && core.IsFaceSkinChar(w) {
				// Overlay only where an explicit face skin is assigned; '.' draws
				// nothing (matters only once elevation exposes a face).
				rl.DrawRectangleRec(r, fadeAlpha(tileColor(LayerWalls, w), wallAlpha*levelFade))
			}
			// Per-floor: show the active floor's decor/prop (falls back to any floor's
			// content). On a legacy nil-stack map these resolve to the single grid char.
			if d := s.area.DecorForDisplay(x, z, s.editLevel); !s.layerHidden[LayerDecor] && d != core.DecorAuto && d != core.DecorEmpty {
				df := levelDistanceFade(s, s.area.DecorLevelAt(x, z))
				rl.DrawRectangleRec(insetRect(r, cell*decorCellInsetFrac), fadeAlpha(decorColor(d), decorAlpha*df))
			}
			if p := s.area.PropForDisplay(x, z, s.editLevel); !s.layerHidden[LayerProps] && core.IsPropChar(p) {
				// A prop fades by ITS OWN level, not the column top.
				pf := levelDistanceFade(s, s.area.PropLevelAt(x, z))
				rl.DrawCircle(int32(r.X+cell/2), int32(r.Y+cell/2), cell*propCellRadiusFrac, fadeAlpha(propColor(p), propAlpha*pf))
			}
			// Ceiling hash overlay: diagonal stripes so the cell reads as "covered".
			if !s.layerHidden[LayerCeiling] && s.area.CeilingAt(x, z) {
				drawCeilingHash(r, cell, fadeAlpha(ceilingColor(), ceilingAlpha*levelFade))
			}
			if showCharOverlay {
				if ch, ok := currentLayerGlyph(s, x, z, lvl); ok {
					drawTileGlyph(font, r, cell, charFontSize, ch, fadeAlpha(charFG, levelFade), fadeAlpha(charShadow, levelFade))
				}
			}
			// Ramp connector arrow on every ramp tile (the Levels panel carries the
			// "which floor" read). Ramps touching the active level show even when hidden.
			if elevGridRamps {
				if fc, ok := cellAt(s.area.Floor, x, z); ok {
					drawRampConnector(font, r, cell, fc)
				}
			}
		}
	}

	// Grid lines, every 5th darker (gridLineMajor). Same cull window as the cells.
	lineXMax := xMax + 1
	if lineXMax > s.area.Width+1 {
		lineXMax = s.area.Width + 1
	}
	lineZMax := zMax + 1
	if lineZMax > s.area.Height+1 {
		lineZMax = s.area.Height + 1
	}
	for x := xMin; x < lineXMax; x++ {
		px := s.rect.gridX + float32(x)*cell
		col := gridLineCol
		if x%gridTickStride == 0 {
			col = gridLineMajor
		}
		rl.DrawLineEx(rl.NewVector2(px, s.rect.gridY), rl.NewVector2(px, s.rect.gridY+s.rect.gridH), 1, col)
	}
	for z := zMin; z < lineZMax; z++ {
		py := s.rect.gridY + float32(z)*cell
		col := gridLineCol
		if z%gridTickStride == 0 {
			col = gridLineMajor
		}
		rl.DrawLineEx(rl.NewVector2(s.rect.gridX, py), rl.NewVector2(s.rect.gridX+s.rect.gridW, py), 1, col)
	}

	// Outline around current-level tile groups (after grid lines, on top).
	drawCurrentLevelOutline(s, xMin, xMax, zMin, zMax, cell)

	// Axis tick labels every 5 cells, only when cells fit a digit.
	if cell >= axisTickMinCell {
		tickCol := gridTickColor
		// Top axis: column numbers.
		for x := (xMin / gridTickStride) * gridTickStride; x < lineXMax; x += gridTickStride {
			label := tickLabel(x)
			m := render.MeasureRichText(font, label, editorFontTick, 1)
			px := s.rect.gridX + float32(x)*cell - m.X/2
			py := s.rect.gridY - m.Y - 2
			if py < s.rect.grid.Y+2 {
				continue
			}
			render.DrawRichText(font, label, rl.NewVector2(px, py), editorFontTick, 1, tickCol)
		}
		// Left axis: row numbers.
		for z := (zMin / gridTickStride) * gridTickStride; z < lineZMax; z += gridTickStride {
			label := tickLabel(z)
			m := render.MeasureRichText(font, label, editorFontTick, 1)
			px := s.rect.gridX - m.X - 4
			py := s.rect.gridY + float32(z)*cell - m.Y/2
			if px < s.rect.grid.X+2 {
				continue
			}
			render.DrawRichText(font, label, rl.NewVector2(px, py), editorFontTick, 1, tickCol)
		}
	}

	// Pack markers: a circle tinted by the leader's brush color + its initial,
	// with an "xN" pack-size badge.
	for _, sp := range s.area.PackSpawns {
		if len(sp.Members) == 0 {
			continue
		}
		// Cull spawns outside the visible window (skip off-screen leader-lookup + measure).
		if markerCulled(sp.TileX, sp.TileZ) {
			continue
		}
		cx, cy := s.rect.tileCenter(sp.TileX, sp.TileZ)
		leaderSlot := core.PackSpawnLeaderSlot(sp)
		leader := packSpawnLeaderKind(sp)
		col := fadeAlpha(packMarkerColor(leader), entityAlpha)
		rl.DrawCircle(int32(cx), int32(cy), cell*packMarkerRadiusFrac, col)
		rl.DrawCircleLines(int32(cx), int32(cy), cell*packMarkerRadiusFrac, fadeAlpha(entityMarkerOutline, entityAlpha))
		label := packMarkerInitial(core.PackMemberDisplayName(sp, leaderSlot))
		measure := render.MeasureRichText(font, label, cell*packLabelFontFrac, 1)
		render.DrawRichText(font, label,
			rl.NewVector2(cx-measure.X/2, cy-measure.Y/2),
			cell*packLabelFontFrac, 1, fadeAlpha(entityMarkerOutline, entityAlpha))
		if len(sp.Members) > 1 {
			badge := fmt.Sprintf("x%d", len(sp.Members))
			bsize := cell * packBadgeFontFrac
			bm := render.MeasureRichText(font, badge, bsize, 1)
			bx := cx + cell*0.18
			by := cy - cell*0.42
			rl.DrawRectangleRounded(
				rl.NewRectangle(bx-2, by-1, bm.X+6, bm.Y+2),
				0.4, 4,
				fadeAlpha(packBadgeBG, entityAlpha))
			render.DrawRichText(font, badge,
				rl.NewVector2(bx+1, by),
				bsize, 1, fadeAlpha(packBadgeText, entityAlpha))
		}
	}

	// Chest markers: a small filled square (distinct from pack circles).
	for _, c := range s.area.ChestSpawns {
		if markerCulled(c.TileX, c.TileZ) {
			continue
		}
		gx, gy := s.rect.tileCorner(c.TileX, c.TileZ)
		inset := cell * chestMarkerInsetFrac
		rl.DrawRectangleRec(
			rl.NewRectangle(gx+inset, gy+inset, cell-2*inset, cell-2*inset),
			fadeAlpha(render.MarkerChest, entityAlpha))
		rl.DrawRectangleLinesEx(
			rl.NewRectangle(gx+inset, gy+inset, cell-2*inset, cell-2*inset),
			1, fadeAlpha(entityMarkerOutline, entityAlpha))
	}

	// Door markers: a tall rectangle + a facing arrowhead (distinct from chests).
	for _, d := range s.area.DoorSpawns {
		if markerCulled(d.TileX, d.TileZ) {
			continue
		}
		gx, gy := s.rect.tileCorner(d.TileX, d.TileZ)
		insetX := cell * doorMarkerInsetXFrac
		insetY := cell * doorMarkerInsetYFrac
		rl.DrawRectangleRec(
			rl.NewRectangle(gx+insetX, gy+insetY, cell-2*insetX, cell-2*insetY),
			fadeAlpha(render.MarkerDoor, entityAlpha))
		rl.DrawRectangleLinesEx(
			rl.NewRectangle(gx+insetX, gy+insetY, cell-2*insetX, cell-2*insetY),
			1, fadeAlpha(entityMarkerOutline, entityAlpha))
		// Facing arrow inside the door rectangle.
		drawFacingArrow(gx+cell*0.5, gy+cell*0.5, d.Facing, cell, doorFacingArrowFrac, 2, fadeAlpha(doorFacingArrowColor, entityAlpha))
	}

	// Crystal markers: a small cyan diamond (distinct from the other markers).
	for _, c := range s.area.CrystalSpawns {
		if markerCulled(c.TileX, c.TileZ) {
			continue
		}
		cx, cy := s.rect.tileCenter(c.TileX, c.TileZ)
		rad := cell * crystalMarkerRadFrac
		rl.DrawPoly(rl.NewVector2(cx, cy), 4, rad, 45, fadeAlpha(render.MarkerCrystal, entityAlpha))
		rl.DrawPolyLinesEx(rl.NewVector2(cx, cy), 4, rad, 45, 1, fadeAlpha(entityMarkerOutline, entityAlpha))
	}

	// Player start marker (part of the Entities layer, so it hides with it).
	if !s.layerHidden[LayerEntities] {
		sx, sy := s.rect.tileCenter(s.area.StartTileX, s.area.StartTileZ)
		startCol := fadeAlpha(render.MarkerStart, entityAlpha)
		rl.DrawCircle(int32(sx), int32(sy), cell*startMarkerRadiusFrac, startCol)
		rl.DrawCircleLines(int32(sx), int32(sy), cell*startMarkerRadiusFrac, fadeAlpha(entityMarkerOutline, entityAlpha))
		drawFacingArrow(sx, sy, s.area.StartFacing, cell, startFacingArrowFrac, 3, fadeAlpha(startFacingArrowColor, entityAlpha))
	}

	// Door-link overlay (its own toggle, above markers).
	if s.showDoorLinks {
		drawDoorLinks(s, cell)
	}

	// Named regions (Locations): labeled translucent rects, level-faded.
	drawLocations(s, font)

	// Brush ghost / hover highlight.
	hoverPx := s.hoverX
	hoverPz := s.hoverZ
	if hoverPx >= 0 {
		// Multi-tile footprint preview: outline every cell tinted by placeability
		// (green = clear, red = blocked) so the full shape shows before clicking.
		if fp := activeFootprint(s); fp != nil {
			ok := footprintPlaceable(s, hoverPx, hoverPz, fp)
			outline := withAlpha(editorPlaceOK, 220)
			fill := withAlpha(editorPlaceOK, 60)
			if !ok {
				outline = withAlpha(editorPlaceWarn, 230)
				fill = withAlpha(editorPlaceWarn, 80)
			}
			for _, off := range fp {
				fx, fz := hoverPx+off.DX, hoverPz+off.DZ
				if !s.area.InBounds(fx, fz) {
					continue
				}
				r := s.rect.tileRect(fx, fz)
				rl.DrawRectangleRec(r, fill)
				rl.DrawRectangleLinesEx(r, 2, outline)
			}
		} else {
			half := s.brushSize / 2
			if !isGridLayer(s.layer) {
				half = 0
			}
			x0 := hoverPx - half
			z0 := hoverPz - half
			side := float32(half*2 + 1)
			cx, cy := s.rect.tileCorner(x0, z0)
			r := rl.NewRectangle(cx, cy, cell*side, cell*side)
			rl.DrawRectangleLinesEx(r, 2, selectionOutline)
		}
	}

	// Rectangle drag preview.
	if s.drag == dragRect && s.hoverX >= 0 {
		x0, x1 := min(s.rectAnchorX, s.hoverX), max(s.rectAnchorX, s.hoverX)
		z0, z1 := min(s.rectAnchorZ, s.hoverZ), max(s.rectAnchorZ, s.hoverZ)
		cx, cy := s.rect.tileCorner(x0, z0)
		r := rl.NewRectangle(cx, cy, float32(x1-x0+1)*cell, float32(z1-z0+1)*cell)
		// Box previews outline-only; Rect fills.
		if !s.rectHollow {
			rl.DrawRectangleRec(r, withAlpha(brushPreviewColor(s), 110))
		}
		rl.DrawRectangleLinesEx(r, 2, selectionOutline)
	}

	// Ruler drag (Measure tool): outline the spanned box + a readout at the cursor.
	if s.drag == dragMeasure && s.hoverX >= 0 {
		x0, x1 := min(s.rectAnchorX, s.hoverX), max(s.rectAnchorX, s.hoverX)
		z0, z1 := min(s.rectAnchorZ, s.hoverZ), max(s.rectAnchorZ, s.hoverZ)
		cx, cy := s.rect.tileCorner(x0, z0)
		r := rl.NewRectangle(cx, cy, float32(x1-x0+1)*cell, float32(z1-z0+1)*cell)
		rl.DrawRectangleLinesEx(r, 2, selectionOutline)
	}

	// Region marquee (Select tool): live drag, else the committed selection.
	// Amber to read apart from the white ghost above.
	if s.drag == dragSelect && s.hoverX >= 0 {
		x0, x1 := min(s.rectAnchorX, s.hoverX), max(s.rectAnchorX, s.hoverX)
		z0, z1 := min(s.rectAnchorZ, s.hoverZ), max(s.rectAnchorZ, s.hoverZ)
		cx, cy := s.rect.tileCorner(x0, z0)
		r := rl.NewRectangle(cx, cy, float32(x1-x0+1)*cell, float32(z1-z0+1)*cell)
		rl.DrawRectangleRec(r, marqueeFill)
		rl.DrawRectangleLinesEx(r, 2, marqueeOutline)
	} else if s.selActive {
		cx, cy := s.rect.tileCorner(s.selX0, s.selZ0)
		r := rl.NewRectangle(cx, cy, float32(s.selX1-s.selX0+1)*cell, float32(s.selZ1-s.selZ0+1)*cell)
		rl.DrawRectangleRec(r, marqueeFill)
		rl.DrawRectangleLinesEx(r, 2, marqueeOutline)
	}

	// Paste ghost: with the Select tool and a full clipboard, outline where Ctrl+V
	// would land (clipboard footprint at the cursor) so a paste can be aligned first.
	if s.tool == toolSelect && s.hoverX >= 0 && !s.clipboard.Empty() &&
		s.drag != dragSelect && s.drag != dragSelectMove {
		r := rl.NewRectangle(0, 0, float32(s.clipboard.W)*cell, float32(s.clipboard.H)*cell)
		r.X, r.Y = s.rect.tileCorner(s.hoverX, s.hoverZ)
		rl.DrawRectangleRec(r, pasteGhostFill)
		rl.DrawRectangleLinesEx(r, 2, pasteGhostOutline)
	}
	// Move preview: while dragging a committed marquee, outline the clamped destination.
	if s.drag == dragSelectMove && s.hoverX >= 0 {
		w, h := s.selX1-s.selX0+1, s.selZ1-s.selZ0+1
		nx := core.Clamp(s.selX0+(s.hoverX-s.rectAnchorX), 0, s.area.Width-w)
		nz := core.Clamp(s.selZ0+(s.hoverZ-s.rectAnchorZ), 0, s.area.Height-h)
		cx, cy := s.rect.tileCorner(nx, nz)
		r := rl.NewRectangle(cx, cy, float32(w)*cell, float32(h)*cell)
		rl.DrawRectangleRec(r, pasteGhostFill)
		rl.DrawRectangleLinesEx(r, 2, pasteGhostOutline)
	}

	// Line drag preview: anchor tile to hovered tile.
	if s.drag == dragLine && s.hoverX >= 0 {
		ax, ay := s.rect.tileCenter(s.rectAnchorX, s.rectAnchorZ)
		hx, hy := s.rect.tileCenter(s.hoverX, s.hoverZ)
		rl.DrawLineEx(rl.NewVector2(ax, ay), rl.NewVector2(hx, hy), 3, selectionOutline)
	}

	if s.drag == dragStart && s.hoverX >= 0 {
		gx, gy := s.rect.tileCenter(s.hoverX, s.hoverZ)
		ghost := withAlpha(render.MarkerStart, 220)
		rl.DrawCircleLines(int32(gx), int32(gy), cell*startMarkerRadiusFrac, ghost)
	}
	if s.drag == dragPack && s.hoverX >= 0 && s.dragPackIdx >= 0 && s.dragPackIdx < len(s.area.PackSpawns) {
		gx, gy := s.rect.tileCenter(s.hoverX, s.hoverZ)
		rl.DrawCircleLines(int32(gx), int32(gy), cell*packMarkerRadiusFrac, selectionOutline)
	}
	// Chest/door drag-move ghosts: a square outline at the destination tile.
	if (s.drag == dragChest || s.drag == dragDoor) && s.hoverX >= 0 {
		gx, gy := s.rect.tileCorner(s.hoverX, s.hoverZ)
		inset := cell * entityGhostInsetFrac
		rl.DrawRectangleLinesEx(rl.NewRectangle(gx+inset, gy+inset, cell-2*inset, cell-2*inset), 2, selectionOutline)
	}

	// Hover tooltip: a card listing what's on the hovered entity tile.
	if s.hoverX >= 0 && s.drag == dragNone {
		drawHoverTooltip(s, font)
	}

	// Compass (top-right). Top-down is fixed north-up; the 3D view's rotates.
	drawEditorCompass(s, font)
}

// drawHoverTooltip paints the entity contents at (hoverX, hoverZ) near the
// mouse; no-op on an empty tile. Memoizes the line slice by (contentEpoch, x, z)
// so it isn't rebuilt every frame the cursor rests on a tile.
var (
	tooltipKeyEpoch uint64
	tooltipKeyX     int = -1
	tooltipKeyZ     int = -1
	tooltipReady    bool
	tooltipLines    []string
)

func drawHoverTooltip(s *State, font rl.Font) {
	x, z := s.hoverX, s.hoverZ
	// Memo the line slice (rebuilding it allocates) so it isn't re-derived per frame.
	if !tooltipReady || tooltipKeyEpoch != s.contentEpoch || tooltipKeyX != x || tooltipKeyZ != z {
		tooltipLines = tooltipLinesFor(s, x, z)
		tooltipKeyEpoch = s.contentEpoch
		tooltipKeyX, tooltipKeyZ = x, z
		tooltipReady = true
	}
	drawTooltipCard(font, tooltipLines, editorFontTiny, tooltipLineH, frameMouse, s.rect.grid)
}

// tooltipLinesFor builds the hover tooltip body for tile (x, z), nil if empty.
func tooltipLinesFor(s *State, x, z int) []string {
	if !s.area.InBounds(x, z) {
		return nil
	}
	var out []string
	out = append(out, core.TileCoord(x, z))
	if s.area.StartTileX == x && s.area.StartTileZ == z {
		face, _ := core.FacingName(s.area.StartFacing)
		out = append(out, "Player start (facing "+face+")")
	}
	if idx := core.PackSpawnIndexAt(s.area.PackSpawns, x, z); idx >= 0 {
		sp := s.area.PackSpawns[idx]
		if len(sp.Members) == 0 {
			out = append(out, "Pack: (empty)")
		} else {
			counts := map[string]int{}
			order := []string{}
			for i := range sp.Members {
				name := core.PackMemberDisplayName(sp, i)
				if _, ok := counts[name]; !ok {
					order = append(order, name)
				}
				counts[name]++
			}
			out = append(out, fmt.Sprintf("Pack (%d):", len(sp.Members)))
			for _, name := range order {
				if counts[name] > 1 {
					out = append(out, fmt.Sprintf("  %dx %s", counts[name], name))
				} else {
					out = append(out, "  "+name)
				}
			}
		}
	}
	if idx := core.ChestSpawnIndexAt(s.area.ChestSpawns, x, z); idx >= 0 {
		ch := s.area.ChestSpawns[idx]
		if len(ch.Items) == 0 {
			out = append(out, "Chest: (empty)")
		} else {
			out = append(out, fmt.Sprintf("Chest (%d):", len(ch.Items)))
			counts := map[core.ItemKind]int{}
			order := []core.ItemKind{}
			for _, k := range ch.Items {
				if _, ok := counts[k]; !ok {
					order = append(order, k)
				}
				counts[k]++
			}
			for _, k := range order {
				info := core.ItemInfo(k)
				if counts[k] > 1 {
					out = append(out, fmt.Sprintf("  %dx %s", counts[k], info.Name))
				} else {
					out = append(out, "  "+info.Name)
				}
			}
		}
	}
	if idx := core.DoorSpawnIndexAt(s.area.DoorSpawns, x, z); idx >= 0 {
		d := s.area.DoorSpawns[idx]
		face, _ := core.FacingName(d.Facing)
		out = append(out, "Door: "+d.Name)
		tgt := d.TargetMap
		if tgt == "" {
			tgt = "(no target)"
		}
		out = append(out, "  -> "+tgt+"/"+d.TargetDoor)
		out = append(out, "  facing "+face)
	}
	if core.CrystalSpawnIndexAt(s.area.CrystalSpawns, x, z) >= 0 {
		out = append(out, "Crystal")
	}
	// Only the coord line → skip (noise on blank floor).
	if len(out) <= 1 {
		return nil
	}
	return out
}

// packMarkerColor returns a pack marker's canvas color: the leader's swatch, else
// the grey fallback (via entityBrushColor).
func packMarkerColor(kind core.EnemyKind) rl.Color {
	return entityBrushColor(kind)
}

// packMarkerInitial returns the uppercase letter for a pack's marker center.
func packMarkerInitial(name string) string {
	if len(name) == 0 {
		return "?"
	}
	c := name[0]
	if c >= 'a' && c <= 'z' {
		c = c - 'a' + 'A'
	}
	return string(c)
}

// layerAlpha returns a layer's opacity: 1 if active, else ~0.55.
func layerAlpha(s *State, l Layer) float32 {
	if s.layer == l {
		return 1
	}
	return 0.55
}

// Level-distance fade tunables. Each level away multiplies opacity by
// levelFadeFalloff, floored at levelFadeMin so far floors stay legible context.
const (
	levelFadeFalloff = float32(0.6)
	levelFadeMin     = float32(0.4)
)

// levelFadeTable[d] is levelFadeFalloff^d (floored at levelFadeMin), precomputed
// so levelDistanceFade is a table lookup, not a per-tile math.Pow.
var levelFadeTable = func() [maxEditLevel + 1]float32 {
	var t [maxEditLevel + 1]float32
	for d := range t {
		f := float32(math.Pow(float64(levelFadeFalloff), float64(d)))
		if f < levelFadeMin {
			f = levelFadeMin
		}
		t[d] = f
	}
	return t
}()

// levelDistanceFade returns the opacity multiplier for a tile on level lvl
// (1.0 on the active level, falling off with distance). Table lookup.
func levelDistanceFade(s *State, lvl int) float32 {
	return core.DistanceFade(lvl-s.editLevel, levelFadeTable[:], levelFadeMin)
}

// drawCurrentLevelOutline strokes the perimeter of each active-level tile group:
// for each active tile, the edges facing a different-level neighbour (or map
// edge). Each boundary edge is drawn once (only the active side strokes it).
func drawCurrentLevelOutline(s *State, xMin, xMax, zMin, zMax int, cell float32) {
	// Double-stroke: dark underlay (thicker, haloes) under a gold core.
	const coreThick = float32(2)
	const underThick = float32(4)
	// Scan one tile past the cull window so a group edge at the viewport boundary
	// still gets stroked.
	if xMin > 0 {
		xMin--
	}
	if zMin > 0 {
		zMin--
	}
	if xMax < s.area.Width {
		xMax++
	}
	if zMax < s.area.Height {
		zMax++
	}
	// onActive: does (x,z) have a tile on the active level.
	onActive := func(x, z int) bool {
		if x < 0 || z < 0 || x >= s.area.Width || z >= s.area.Height {
			return false
		}
		_, solid := s.area.SolidAt(x, s.editLevel, z)
		return solid
	}
	// offLevel: (x,z) off the active slice (out of bounds or no tile there).
	offLevel := func(x, z int) bool {
		return !onActive(x, z)
	}
	// edge strokes underlay then core for one cell edge.
	edge := func(ax, ay, bx, by float32) {
		rl.DrawLineEx(rl.NewVector2(ax, ay), rl.NewVector2(bx, by), underThick, currentLevelOutlineUnderlay)
		rl.DrawLineEx(rl.NewVector2(ax, ay), rl.NewVector2(bx, by), coreThick, currentLevelOutlineColor)
	}
	for z := zMin; z < zMax; z++ {
		for x := xMin; x < xMax; x++ {
			if !onActive(x, z) {
				continue
			}
			r := s.rect.tileRect(x, z)
			if offLevel(x-1, z) { // left
				edge(r.X, r.Y, r.X, r.Y+cell)
			}
			if offLevel(x+1, z) { // right
				edge(r.X+cell, r.Y, r.X+cell, r.Y+cell)
			}
			if offLevel(x, z-1) { // top
				edge(r.X, r.Y, r.X+cell, r.Y)
			}
			if offLevel(x, z+1) { // bottom
				edge(r.X, r.Y+cell, r.X+cell, r.Y+cell)
			}
		}
	}
}

// fadeAlpha scales c's alpha by a 0..1 multiplier. Thin alias over render.FadeColor.
func fadeAlpha(c rl.Color, alpha float32) rl.Color {
	return render.FadeColor(c, alpha)
}

// padRect grows r by (dx, dy) on each side; a negative delta shrinks it. The signed
// rect-inflation primitive shared by the fat click-bands (foeview sliders) and the
// uniform-shrink insetRect shorthand.
func padRect(r rl.Rectangle, dx, dy float32) rl.Rectangle {
	return rl.NewRectangle(r.X-dx, r.Y-dy, r.Width+2*dx, r.Height+2*dy)
}

func insetRect(r rl.Rectangle, inset float32) rl.Rectangle {
	return padRect(r, -inset, -inset)
}

// brushPreviewColor returns a tint for the active brush so the rect-drag preview
// hints at what's painted (per layer via layerDefs in layerdef.go).
func brushPreviewColor(s *State) rl.Color {
	return layerDefs[s.layer].previewColor(s, s.activeBrush())
}

// tileColorByChar is the per-layer per-char swatch color, a [layerCount][256]
// array so the per-cell lookup is one indexed read. Built at init from
// layerBrushes; each row pre-filled with the layer fallback, then palette chars
// overwrite (so an unknown char reads as fallback with no branch).
var tileColorByChar = buildTileColorTable()

func buildTileColorTable() [layerCount][256]rl.Color {
	var out [layerCount][256]rl.Color
	for layer := 0; layer < layerCount; layer++ {
		fallback := editorFallbackColor
		if c, ok := tileColorFallback[Layer(layer)]; ok {
			fallback = c
		}
		for c := range out[layer] {
			out[layer][c] = fallback
		}
		for _, b := range layerBrushes[layer] {
			if b.Char != 0 {
				out[layer][b.Char] = b.Color
			}
		}
	}
	return out
}

// tileColorFallback is the swatch for a char not in the brush palette.
var tileColorFallback = map[Layer]rl.Color{
	LayerFloor:   floorAutoColor,
	LayerDecor:   floorAutoColor,
	LayerProps:   editorFallbackColor,
	LayerCeiling: ceilingFallbackColor,
}

func tileColor(layer Layer, c byte) rl.Color {
	if layer < 0 || int(layer) >= layerCount {
		return editorFallbackColor
	}
	// Fallback is pre-baked into every row, so this index covers all chars.
	return tileColorByChar[layer][c]
}

// elevationLevelColor maps a level to an earthy swatch (dark low → light high).
func elevationLevelColor(level int) rl.Color {
	level = clampLevel(level)
	t := float32(level) / float32(maxEditLevel)
	return rl.NewColor(uint8(92+t*120), uint8(72+t*108), uint8(56+t*66), 255)
}

// drawRampConnector draws a ramp tile's connective arrow (non-ramp tiles draw
// nothing). Takes the floor char directly + uses the pure RampAscentFacing switch
// to avoid RampAt's per-tile InBounds + layer index.
func drawRampConnector(font rl.Font, r rl.Rectangle, cell float32, floorChar byte) {
	facing, ok := core.RampAscentFacing(floorChar)
	if !ok {
		return
	}
	rl.DrawRectangleLinesEx(r, 2, rampConnectorColor)
	drawTileGlyph(font, r, cell, cell*0.62, core.RampCharForFacing(facing), rampGlyphColor, glyphShadow)
}

// rampTouchesActiveLevel reports whether the ramp at (x,z) connects the active
// level (its low level is editLevel or editLevel-1). Such ramps stay visible
// even when their own level is hidden.
// low is the column-top level at (x,z), passed in to reuse the caller's cached
// lookup instead of re-walking the voxel column.
func rampTouchesActiveLevel(s *State, x, z, low int) bool {
	if _, ok := s.area.RampAt(x, z); !ok {
		return false
	}
	return low == s.editLevel || low == s.editLevel-1
}

func wallColor() color.RGBA        { return tileColor(LayerWalls, core.TileRock) }
func floorColor(c byte) color.RGBA { return tileColor(LayerFloor, c) }
func decorColor(c byte) color.RGBA { return tileColor(LayerDecor, c) }
func propColor(c byte) color.RGBA  { return tileColor(LayerProps, c) }
func ceilingColor() color.RGBA     { return tileColor(LayerCeiling, core.TileCeilingSolid) }

// drawCeilingHash paints two diagonal stripes so a ceiling cell reads as "roofed".
func drawCeilingHash(r rl.Rectangle, cell float32, col color.RGBA) {
	t := cell * 0.10
	// Both diagonals.
	rl.DrawLineEx(rl.NewVector2(r.X, r.Y), rl.NewVector2(r.X+cell, r.Y+cell), t, col)
	rl.DrawLineEx(rl.NewVector2(r.X+cell, r.Y), rl.NewVector2(r.X, r.Y+cell), t, col)
}

// currentLayerGlyph returns the char to overlay for the active layer's cell at
// (x, z), ok==false when empty or the layer has no per-tile chars (Entities).
func currentLayerGlyph(s *State, x, z, lvl int) (byte, bool) {
	return layerDefs[s.layer].glyph(s, x, z, lvl)
}

// tileGlyphMeasure memoizes each tile glyph's measured size at the current cell
// font size — the grid can paint thousands of glyphs/frame, so the cgo
// MeasureTextEx was the biggest text cost. Caches by (char, fontSize); a size
// change invalidates.
var tileGlyphMeasure struct {
	fontSize float32
	w, h     [256]float32
	done     [256]bool
}

func tileGlyphSize(font rl.Font, ch byte, fontSize float32) (w, h float32) {
	c := &tileGlyphMeasure
	if c.fontSize != fontSize {
		c.fontSize = fontSize
		c.done = [256]bool{}
	}
	if !c.done[ch] {
		m := rl.MeasureTextEx(font, glyphStr[ch], fontSize, 1)
		c.w[ch], c.h[ch], c.done[ch] = m.X, m.Y, true
	}
	return c.w[ch], c.h[ch]
}

// drawTileGlyph paints a glyph centered in r with a 1px drop shadow. Draws via
// rl.DrawTextEx + the size cache (no rich-text scan / uncached measure per tile).
func drawTileGlyph(font rl.Font, r rl.Rectangle, cell, fontSize float32, ch byte, fg, shadow rl.Color) {
	text := glyphStr[ch]
	mx, my := tileGlyphSize(font, ch, fontSize)
	px := r.X + (cell-mx)/2
	py := r.Y + (cell-my)/2
	rl.DrawTextEx(font, text, rl.NewVector2(px+1, py+1), fontSize, 1, shadow)
	rl.DrawTextEx(font, text, rl.NewVector2(px, py), fontSize, 1, fg)
}

// scrollWindow returns the [top, end) bounds of a visible window of rowsVisible
// over total entries that keeps cursor in view (both 0 when empty). Shared scroll
// helper for the modal lists.
func scrollWindow(cursor, total, rowsVisible int) (top, end int) {
	if total <= 0 || rowsVisible <= 0 {
		return 0, 0
	}
	if cursor >= rowsVisible {
		top = cursor - rowsVisible + 1
	}
	if top > total-rowsVisible {
		top = total - rowsVisible
	}
	if top < 0 {
		top = 0
	}
	end = top + rowsVisible
	if end > total {
		end = total
	}
	return top, end
}

// windowedRowList lays out a scrolling fixed-pitch list of count rows in a band
// at (x,y), w wide, areaH tall. Returns a length-count slice where only on-window
// rows [topRow,end) carry real rects (off-window stay zero, so they don't draw/hit).
// visibleRowCount is how many fixed-pitch rows fit in a band areaH tall (floor 1) —
// the shared list-window sizing behind windowedRowList / entityModalLayoutFor /
// openModalListGeom so the three can't drift.
func visibleRowCount(areaH, pitch float32) int {
	if n := int(areaH / pitch); n > 1 {
		return n
	}
	return 1
}

func windowedRowList(x, y, w, rowH, pitch float32, cursor, count int, areaH float32) (topRow, end int, rects []rl.Rectangle) {
	topRow, end = scrollWindow(cursor, count, visibleRowCount(areaH, pitch))
	rects = make([]rl.Rectangle, count)
	for i := topRow; i < end; i++ {
		rects[i] = rl.NewRectangle(x, y+float32(i-topRow)*pitch, w, rowH)
	}
	return topRow, end, rects
}

// --- Status & modals -------------------------------------------------------

// gridCornerCard computes the bottom-left status-card rect (maxW-wide content, rH
// tall) inside the grid pane and paints its card frame, returning the rect. Shared
// by the transient toast (drawStatus) and the recall panel (drawStatusHistory) so
// their corner geometry + framing can't drift.
func gridCornerCard(s *State, theme render.Theme, maxW, rH float32) rl.Rectangle {
	r := rl.NewRectangle(s.rect.grid.X+12, s.rect.grid.Y+s.rect.grid.Height-rH-8, maxW+24, rH)
	render.DrawCard(int32(r.X), int32(r.Y), int32(r.Width), int32(r.Height),
		theme.SurfacePrimary, theme.BorderSoft, theme.BorderStrong)
	return r
}

func drawStatus(s *State, font rl.Font, theme render.Theme) {
	const lineH = 22
	const pad = 12
	maxW := float32(0)
	for _, e := range s.statusLog {
		m := render.MeasureRichText(font, e.msg, editorFontLabel, 1)
		if m.X > maxW {
			maxW = m.X
		}
	}
	rH := float32(len(s.statusLog))*lineH + pad
	r := gridCornerCard(s, theme, maxW, rH)
	for i, e := range s.statusLog {
		y := r.Y + pad/2 + float32(i)*lineH
		alpha := core.Clamp(e.timer/statusLogLifetime, 0, 1)
		col := theme.TextPrimary
		if e.warn {
			col = theme.BorderDanger
		}
		col.A = uint8(float32(col.A) * (0.4 + 0.6*alpha))
		render.DrawTextWithShadow(font, e.msg, r.X+12, y, editorFontLabel, col)
	}
}

// drawStatusHistory paints the recall panel (L toggle): the last messages, newest at
// the bottom, above the transient toast so an expired message can still be read.
func drawStatusHistory(s *State, font rl.Font, theme render.Theme) {
	const lineH = 20
	const pad = 10
	const maxRows = 14
	hist := s.statusHistory
	if len(hist) > maxRows {
		hist = hist[len(hist)-maxRows:]
	}
	title := "Recent messages (L to close)"
	maxW := render.MeasureRichText(font, title, editorFontHint, 1).X
	for _, m := range hist {
		if w := render.MeasureRichText(font, m, editorFontLabel, 1).X; w > maxW {
			maxW = w
		}
	}
	rH := float32(len(hist)+1)*lineH + 2*pad
	r := gridCornerCard(s, theme, maxW, rH)
	render.DrawRichText(font, title, rl.NewVector2(r.X+12, r.Y+pad), editorFontHint, 1, theme.TextHint)
	for i, m := range hist {
		y := r.Y + pad + float32(i+1)*lineH
		col := theme.TextPrimary
		if len(m) > 0 && m[0] == '!' {
			col = theme.BorderDanger
		}
		render.DrawTextWithShadow(font, m, r.X+12, y, editorFontLabel, col)
	}
	if len(hist) == 0 {
		render.DrawRichText(font, "(no messages yet)", rl.NewVector2(r.X+12, r.Y+pad+lineH), editorFontLabel, 1, theme.TextMuted)
	}
}

func drawModalVeil(theme render.Theme) {
	w, h := render.ScreenSize()
	rl.DrawRectangle(0, 0, w, h, theme.SurfaceVeil)
}

// centeredCardRect returns the screen-centered rect for a modal card.
func centeredCardRect(pw, ph float32) rl.Rectangle {
	w, h := render.ScreenSizeF()
	return rl.NewRectangle((w-pw)/2, (h-ph)/2, pw, ph)
}

func drawModalCard(theme render.Theme, pw, ph float32, accent rl.Color) rl.Rectangle {
	r := centeredCardRect(pw, ph)
	render.DrawCard(int32(r.X), int32(r.Y), int32(r.Width), int32(r.Height),
		theme.SurfacePrimary, theme.BorderSoft, accent)
	return r
}

// drawModalHeader is the standard modal opening (veil + card + heading),
// returning the card rect for body layout.
func drawModalHeader(font rl.Font, theme render.Theme, pw, ph float32, title string, accent rl.Color) rl.Rectangle {
	r := centeredCardRect(pw, ph)
	drawModalHeaderAt(font, theme, r, title, accent)
	return r
}

// drawModalHeaderAt is drawModalHeader for callers that already computed the card
// rect (custom-layout modals that need it for hit-testing first).
func drawModalHeaderAt(font rl.Font, theme render.Theme, card rl.Rectangle, title string, accent rl.Color) {
	drawModalVeil(theme)
	render.DrawCard(int32(card.X), int32(card.Y), int32(card.Width), int32(card.Height),
		theme.SurfacePrimary, theme.BorderSoft, accent)
	render.DrawHeading(font, title, int32(card.X+modalContentInset), int32(card.Y)+modalHeadingInsetY, accent)
}

// openModalPromptDY is the Y (up from the card bottom) of the open-modal sub-mode
// prompt row — the rename field and the delete-confirm line share it so the two
// sub-modes can't drift.
const openModalPromptDY = float32(86)

// openModalListGeom returns the open-map list geometry (card, first row Y, row
// height, visible window), shared by draw and hit-test.
func openModalListGeom(s *State) (card rl.Rectangle, listTop, rowH float32, topRow, end int) {
	card = centeredCardRect(openModalW, openModalH)
	rowH = entityListRowH
	listTop = card.Y + openModalListTop
	listBottom := card.Y + card.Height - openModalListBottomPad // room for the button row
	rowsVisible := int((listBottom - listTop) / rowH)
	if rowsVisible < 1 {
		rowsVisible = 1
	}
	topRow, end = scrollWindow(s.modalCursor, len(openVisiblePaths(s)), rowsVisible)
	return
}

// openModalRowAt returns the path index whose list row is under p, or -1.
func openModalRowAt(s *State, p rl.Vector2) int {
	card, listTop, rowH, topRow, end := openModalListGeom(s)
	for i := topRow; i < end; i++ {
		row := rl.NewRectangle(card.X+12, listTop+float32(i-topRow)*rowH, card.Width-24, rowH)
		if pointIn(p, row) {
			return i
		}
	}
	return -1
}

func drawOpenModal(s *State, font rl.Font, theme render.Theme) {
	header := "OPEN MAP"
	if s.modalRenamingActive {
		header = "RENAME MAP"
	} else if s.modalConfirmDelete {
		header = "DELETE MAP"
	}
	r := drawModalHeader(font, theme, openModalW, openModalH, header, theme.BorderStrong)

	if len(s.modalPaths) == 0 {
		render.DrawRichText(font, "(no .map files in maps/)", rl.NewVector2(r.X+modalContentInset, r.Y+50), editorFontLabel, 1, theme.TextMuted)
		drawModalFooterHint(font, r, "Esc / click outside to close", theme)
		return
	}

	// Live filter caption (main view only), so type-to-filter is discoverable.
	if !s.modalRenamingActive && !s.modalConfirmDelete {
		cap := "Type to filter…"
		if s.openFilter != "" {
			cap = "Filter: " + s.openFilter
		}
		render.DrawRichText(font, cap, rl.NewVector2(r.X+modalContentInset, r.Y+30), editorFontHint, 1, theme.TextHint)
	}

	vis := openVisiblePaths(s)
	if len(vis) == 0 {
		render.DrawRichText(font, "(no matches)", rl.NewVector2(r.X+modalContentInset, r.Y+50), editorFontLabel, 1, theme.TextMuted)
		drawModalFooterHint(font, r, "Backspace to edit filter   Esc to clear", theme)
		return
	}

	_, listTop, rowH, topRow, end := openModalListGeom(s)
	for i := topRow; i < end; i++ {
		path := vis[i]
		text := core.MapIDFromPath(path)
		col := theme.TextMuted
		if i == s.modalCursor {
			col = theme.BorderActive
			text = "> " + text
		}
		render.DrawTextWithShadow(font, text, r.X+18, listTop+float32(i-topRow)*rowH, editorFontBody, col)
	}
	// Scroll hint when the list overflows.
	if topRow > 0 || end < len(vis) {
		more := fmt.Sprintf("(%d / %d)", s.modalCursor+1, len(vis))
		measure := render.MeasureRichText(font, more, editorFontHint, 1)
		render.DrawRichText(font, more,
			rl.NewVector2(r.X+r.Width-measure.X-16, r.Y+30),
			editorFontHint, 1, theme.TextHint)
	}

	if s.modalRenamingActive {
		fieldR := rl.NewRectangle(r.X+modalContentInset, r.Y+r.Height-openModalPromptDY, r.Width-2*modalContentInset, textFieldH)
		drawTextField(font, fieldR, s.modalRenaming, true)
		labels := cmdLabels(openRenameCmds(s))
		drawModalButtons(font, modalButtonRow(r, labels), labels)
		return
	}
	if s.modalConfirmDelete {
		path := selectedOpenPath(s)
		render.DrawRichText(font, fmt.Sprintf("Delete %s? This is permanent.", core.MapIDFromPath(path)),
			rl.NewVector2(r.X+modalContentInset, r.Y+r.Height-openModalPromptDY), editorFontLabel, 1, theme.BorderDanger)
		labels := cmdLabels(openDeleteConfirmCmds(s))
		drawModalButtons(font, modalButtonRow(r, labels), labels)
		return
	}

	// Main view: click a row to select, then an action button.
	labels := cmdLabels(openModalActionCmds(s))
	drawModalButtons(font, modalButtonRow(r, labels), labels)
}

// Modal card dimensions. (Validate sizes its height from the row count, so only
// its width is a const.)
const (
	saveAsModalW = float32(420)
	saveAsModalH = float32(160)

	// modalCardW is the standard editor card width; the door-edit and entity/dialog
	// list modals share it. The Open card is intentionally narrower (openModalW).
	modalCardW     = float32(480)
	doorEditModalW = modalCardW
	doorEditModalH = float32(424)
	openModalW     = float32(460)
	openModalH     = float32(460)
	// Open-map list: rows start openModalListTop below the card top, with
	// openModalListBottomPad reserved for the button row. Pitch is entityListRowH.
	openModalListTop       = float32(50)
	openModalListBottomPad = float32(52)
	entityEditModalW       = modalCardW // shared by the entity + dialog list modals
	entityEditModalH       = float32(440)
	escMenuModalW          = float32(380)
	escMenuModalH          = float32(178)
	confirmDirtyModalW     = float32(460)
	confirmDirtyModalH     = float32(212) // clears the hint above the button stack
	validateModalW         = float32(560)
)

func saveAsFieldRect(s *State) rl.Rectangle {
	r := centeredCardRect(saveAsModalW, saveAsModalH)
	return rl.NewRectangle(r.X+modalContentInset, r.Y+58, saveAsModalW-2*modalContentInset, textFieldH)
}

func drawSaveAsModal(s *State, font rl.Font, theme render.Theme) {
	accent := theme.BorderStrong
	title := "SAVE MAP AS"
	if s.awaitingOverwrite {
		accent = theme.BorderDanger
		title = "FILE EXISTS"
	}
	r := drawModalHeader(font, theme, saveAsModalW, saveAsModalH, title, accent)

	if s.awaitingOverwrite {
		render.DrawRichText(font, fmt.Sprintf("Overwrite %s?", core.MapPath(s.modalFilename)),
			rl.NewVector2(r.X+modalContentInset, r.Y+44), editorFontLabel, 1, theme.TextPrimary)
		labels := cmdLabels(saveAsOverwriteCmds(s))
		drawModalButtons(font, modalButtonStack(r, labels), labels)
		return
	}

	render.DrawRichText(font, "Filename (without .map):", rl.NewVector2(r.X+modalContentInset, r.Y+modalSubheadingDY), editorFontHint, 1, theme.TextLabel)

	field := saveAsFieldRect(s)
	drawTextField(font, field, s.modalFilename, true)
	// Preview the sanitized final path so the user sees what they'll get, and
	// flag divergence from what they typed.
	sanitized := sanitizeFilename(s.modalFilename)
	previewPath := core.MapPath(sanitized)
	render.DrawRichText(font, fmt.Sprintf("Will save to: %s", previewPath),
		rl.NewVector2(r.X+modalContentInset, r.Y+96), editorFontHint, 1, theme.TextMuted)
	if sanitized != strings.TrimSuffix(strings.TrimSuffix(s.modalFilename, mapfile.Ext), strings.ToUpper(mapfile.Ext)) {
		render.DrawRichText(font, "(Punctuation and spaces are stripped)",
			rl.NewVector2(r.X+modalContentInset, r.Y+112), editorFontTiny, 1, theme.BorderDanger)
	}
	drawModalFooterHint(font, r, "Enter save   Esc cancel", theme)
}

// drawPackEditModal renders the inline pack editor: header, scrollable member
// list, and a shortcut hint row.
func drawPackEditModal(s *State, font rl.Font, theme render.Theme) {
	if s.modalPackIdx < 0 || s.modalPackIdx >= len(s.area.PackSpawns) {
		return
	}
	pack := s.area.PackSpawns[s.modalPackIdx]
	r := drawModalHeader(font, theme, entityEditModalW, entityEditModalH,
		"PACK AT "+core.TileCoord(pack.TileX, pack.TileZ),
		theme.BorderActive)

	// Leader hint: the highest-tier member is whose silhouette shows in-world.
	leaderText := "Leader: —"
	if len(pack.Members) > 0 {
		leaderIdx := core.PackSpawnLeaderSlot(pack)
		if leaderIdx >= 0 && leaderIdx < len(pack.Members) {
			leaderText = "Leader (highest tier): " + core.PackMemberDisplayName(pack, leaderIdx)
		}
	}
	render.DrawTextWithShadow(font, leaderText, r.X+modalContentInset, r.Y+38, editorFontHint, theme.TextMuted)
	render.DrawTextWithShadow(font, "Up/Down select · Enter add · X remove · K/J reorder · R row · A cycle AI · Esc close",
		r.X+modalContentInset, r.Y+54, editorFontTiny, theme.TextHint)
	drawSpawnLevelStepper(s, font, theme, r, pack.Level)

	adds, actions := packEditCmds(s)
	lay := entityModalLayoutFor(s.modalCursor, len(pack.Members), cmdLabels(adds), cmdLabels(actions))
	drawEntityListWindow(font, theme, lay, len(pack.Members), s.modalCursor,
		"(empty — close to drop)",
		func(i int) string {
			return core.PackMemberDisplayName(pack, i) + " · " + core.RowLabel(pack.Members[i].Row)
		})
	drawModalButtons(font, lay.actRects, cmdLabels(actions))
	drawModalButtons(font, lay.addRects, cmdLabels(adds))
}

// drawChestEditModal renders the inline chest editor (mirrors drawPackEditModal).
func drawChestEditModal(s *State, font rl.Font, theme render.Theme) {
	if s.modalChestIdx < 0 || s.modalChestIdx >= len(s.area.ChestSpawns) {
		return
	}
	chest := s.area.ChestSpawns[s.modalChestIdx]
	r := drawModalHeader(font, theme, entityEditModalW, entityEditModalH,
		"CHEST AT "+core.TileCoord(chest.TileX, chest.TileZ),
		theme.BorderActive)
	render.DrawTextWithShadow(font, "Up/Down select · Enter add · X remove · Esc close",
		r.X+modalContentInset, r.Y+modalSubheadingDY, editorFontTiny, theme.TextHint)
	drawSpawnLevelStepper(s, font, theme, r, chest.Level)

	adds, actions := chestEditCmds(s)
	lay := entityModalLayoutFor(s.modalCursor, len(chest.Items), cmdLabels(adds), cmdLabels(actions))
	drawEntityListWindow(font, theme, lay, len(chest.Items), s.modalCursor,
		"(empty — adds reveal it as pre-looted in game)",
		func(i int) string { return core.ItemInfo(chest.Items[i]).Name })
	drawModalButtons(font, lay.actRects, cmdLabels(actions))
	drawModalButtons(font, lay.addRects, cmdLabels(adds))
}

// drawDoorEditModal renders the per-door editor: three text fields, a facing row,
// a style row, and delete/close buttons.
func drawDoorEditModal(s *State, font rl.Font, theme render.Theme) {
	if s.modalDoorIdx < 0 || s.modalDoorIdx >= len(s.area.DoorSpawns) {
		return
	}
	door := s.area.DoorSpawns[s.modalDoorIdx]
	l := doorEditLayoutFor()
	header := "DOOR AT " + core.TileCoord(door.TileX, door.TileZ)
	drawModalHeaderAt(font, theme, l.card, header, theme.BorderActive)
	drawSpawnLevelStepper(s, font, theme, l.card, door.Level)

	drawLabel(font, "Name (unique on this map)", labelAbove(l.nameField))
	drawTextField(font, l.nameField, door.Name, s.focus == focusDoorName)

	drawLabel(font, "Target map (bare id, or 'self')", labelAbove(l.mapField))
	drawTextField(font, l.mapField, door.TargetMap, s.focus == focusDoorTargetMap)
	drawButton(font, l.mapPickBtn, "▼", s.dropdown.owner == ddDoorTargetMap)

	drawLabel(font, "Target door (Name on destination map)", labelAbove(l.doorField))
	drawTextField(font, l.doorField, door.TargetDoor, s.focus == focusDoorTargetDoor)
	drawButton(font, l.doorPickBtn, "▼", s.dropdown.owner == ddDoorTargetDoor)

	// Facing picker (opens ddDoorFacing).
	facingName, _ := core.FacingName(door.Facing)
	drawLabel(font, "Facing / wall to affix to (player walks out this way)", labelAbove(l.facingBtn))
	drawButton(font, l.facingBtn, facingName+dropdownArrowSuffix, s.dropdown.owner == ddDoorFacing)

	// Style picker (opens ddDoorStyle).
	drawLabel(font, "Style (visual fixture)", labelAbove(l.styleBtn))
	drawButton(font, l.styleBtn, core.DoorStyleLabel(door.Style)+dropdownArrowSuffix, s.dropdown.owner == ddDoorStyle)

	// Delete highlights while armed (first of the two-press confirm).
	drawButton(font, l.deleteBtn, "Delete door (X)", s.deleteArmed == "door")
	drawButton(font, l.closeBtn, "Done (Esc)", false)

	hint := "Tab cycle fields   X delete   Esc done"
	render.DrawRichText(font, hint,
		rl.NewVector2(l.card.X+modalContentInset, l.card.Y+l.card.Height-72),
		editorFontTiny, 1, theme.TextHint)
}

// crystalEditModal card dimensions (small — crystals carry only a floor + delete).
const (
	crystalEditModalW = float32(400)
	crystalEditModalH = float32(180)
)

// drawCrystalEditModal renders the per-crystal editor: a floor stepper (multi-level
// maps) plus delete / done buttons.
func drawCrystalEditModal(s *State, font rl.Font, theme render.Theme) {
	if s.modalCrystalIdx < 0 || s.modalCrystalIdx >= len(s.area.CrystalSpawns) {
		return
	}
	cr := s.area.CrystalSpawns[s.modalCrystalIdx]
	card := drawModalHeader(font, theme, crystalEditModalW, crystalEditModalH,
		"CRYSTAL AT "+core.TileCoord(cr.TileX, cr.TileZ), theme.BorderActive)
	drawSpawnLevelStepper(s, font, theme, card, cr.Level)
	render.DrawRichText(font, "Healing crystal — restores the party. Floor is its only per-instance setting.",
		rl.NewVector2(card.X+modalContentInset, card.Y+56), editorFontHint, 1, theme.TextMuted)
	drawButton(font, bottomLeftBtn(card), "Delete crystal (X)", false)
	drawButton(font, bottomRightBtn(card), "Done (Esc)", false)
}

// drawValidateModal renders the reachability + cross-map warning list (read-only).
func drawValidateModal(s *State, font rl.Font, theme render.Theme) {
	rows := s.modalValidateRows
	pw := validateModalW
	ph := 56 + float32(len(rows))*reachBadgeRowH + 56
	if ph < 160 {
		ph = 160
	}
	_, sh := render.ScreenSizeF()
	if ph > sh-40 {
		ph = sh - 40
	}
	r := drawModalHeader(font, theme, pw, ph, "VALIDATE MAP", theme.BorderActive)
	if len(rows) == 0 {
		render.DrawRichText(font, "All checks pass.",
			rl.NewVector2(r.X+modalContentInset, r.Y+50), editorFontBody, 1, theme.BorderStrong)
	} else {
		y := r.Y + 50
		for _, line := range rows {
			render.DrawRichText(font, "! "+line,
				rl.NewVector2(r.X+modalContentInset, y), editorFontAccent, 1, theme.BorderDanger)
			y += reachBadgeRowH
		}
	}
	drawModalFooterHint(font, r, "Esc / Enter / click   close", theme)
}

// entityKindRow tags an entity-list row by what it points at.
type entityKindRow int

const (
	elStart entityKindRow = iota
	elPack
	elChest
	elDoor
	elCrystal
)

// entityListRow is one Objects-index row: label + jump tile + which editor it opens.
type entityListRow struct {
	label string
	x, z  int
	kind  entityKindRow
	idx   int // spawn index for pack/chest/door (unused for start)
}

const (
	entityListModalW  = float32(580)
	objectsRowH       = float32(24)
	entityListVisible = 14
	// Content-sized Objects modal: header band + footer reserve bracket the row
	// stack, clamped to a minimum card height; rows start entityListHeaderH down.
	entityListHeaderH    = float32(56) // card top → first row
	entityListFooterPad  = float32(36) // last row → card bottom (button room)
	entityListMinH       = float32(150)
	entityListContentTop = float32(46) // first-row text Y past the card top
)

// entityListRows builds the Objects index fresh (player start, then packs/chests/
// doors/crystals), so indices always match the live spawn slices.
func entityListRows(s *State) []entityListRow {
	rows := []entityListRow{{
		label: "Player start  —  " + core.TileCoord(s.area.StartTileX, s.area.StartTileZ),
		x:     s.area.StartTileX, z: s.area.StartTileZ, kind: elStart,
	}}
	for i, p := range s.area.PackSpawns {
		name := "(empty)"
		if len(p.Members) > 0 {
			name = core.PackMemberDisplayName(p, core.PackSpawnLeaderSlot(p))
		}
		rows = append(rows, entityListRow{
			label: fmt.Sprintf("Pack x%d  %s  —  %s", len(p.Members), name, core.TileCoord(p.TileX, p.TileZ)),
			x:     p.TileX, z: p.TileZ, kind: elPack, idx: i,
		})
	}
	for i, c := range s.area.ChestSpawns {
		rows = append(rows, entityListRow{
			label: fmt.Sprintf("Chest  %d items  —  %s", len(c.Items), core.TileCoord(c.TileX, c.TileZ)),
			x:     c.TileX, z: c.TileZ, kind: elChest, idx: i,
		})
	}
	for i, d := range s.area.DoorSpawns {
		target := d.TargetDoor
		if d.TargetMap != "" && d.TargetMap != core.SelfMapToken {
			target = d.TargetMap + "/" + d.TargetDoor
		}
		rows = append(rows, entityListRow{
			label: fmt.Sprintf("Door %s -> %s  —  %s", d.Name, target, core.TileCoord(d.TileX, d.TileZ)),
			x:     d.TileX, z: d.TileZ, kind: elDoor, idx: i,
		})
	}
	for i, c := range s.area.CrystalSpawns {
		rows = append(rows, entityListRow{
			label: "Crystal  —  " + core.TileCoord(c.TileX, c.TileZ),
			x:     c.TileX, z: c.TileZ, kind: elCrystal, idx: i,
		})
	}
	return rows
}

func entityRowColor(k entityKindRow) rl.Color {
	switch k {
	case elPack:
		return markerPackDot
	case elChest:
		return render.MarkerChest
	case elDoor:
		return render.MarkerDoor
	case elCrystal:
		return render.MarkerCrystal
	case elStart:
		return render.MarkerStart
	}
	// A new entityKindRow must add its dot color above — fail loud rather than
	// silently painting it start-gold via a catch-all default.
	panic("editor: entityRowColor missing a case for entity row kind")
}

// entityListGeom is the shared Objects-modal layout (card, first row Y, rows,
// visible window) for draw + hit-test.
func entityListGeom(s *State) (card rl.Rectangle, listTop float32, rows []entityListRow, top, end int) {
	rows = entityListRows(s)
	shown := len(rows)
	if shown > entityListVisible {
		shown = entityListVisible
	}
	ph := entityListHeaderH + float32(shown)*objectsRowH + entityListFooterPad
	if ph < entityListMinH {
		ph = entityListMinH
	}
	card = centeredCardRect(entityListModalW, ph)
	listTop = card.Y + entityListContentTop
	top, end = scrollWindow(s.modalCursor, len(rows), entityListVisible)
	return card, listTop, rows, top, end
}

// entityListRowRect is the clickable rect for the screenRow-th visible row.
func entityListRowRect(card rl.Rectangle, listTop float32, screenRow int) rl.Rectangle {
	return rl.NewRectangle(card.X+modalContentInset, listTop+float32(screenRow)*objectsRowH,
		card.Width-2*modalContentInset, objectsRowH)
}

func drawEntityListModal(s *State, font rl.Font, theme render.Theme) {
	card, listTop, rows, top, end := entityListGeom(s)
	drawModalHeaderAt(font, theme, card, "OBJECTS", theme.BorderActive)
	if len(rows) == 0 {
		render.DrawRichText(font, "No objects placed.",
			rl.NewVector2(card.X+modalContentInset, listTop), editorFontBody, 1, theme.TextMuted)
	}
	mp := frameMouse
	for i := top; i < end; i++ {
		rr := entityListRowRect(card, listTop, i-top)
		if i == s.modalCursor {
			drawSelectedListRow(rr)
		} else if pointIn(mp, rr) {
			rl.DrawRectangleRec(rr, bgEntryHover)
		}
		rl.DrawCircleV(rl.NewVector2(rr.X+8, rr.Y+rr.Height/2), 4, entityRowColor(rows[i].kind))
		col := theme.TextMuted
		if i == s.modalCursor {
			col = theme.TextPrimary
		}
		render.DrawRichText(font, rows[i].label, rl.NewVector2(rr.X+22, rr.Y+(rr.Height-16)/2), editorFontBody, 1, col)
	}
	drawModalFooterHint(font, card, fmt.Sprintf("%d objects   ·   Up/Down + Enter or click a row to jump + edit   ·   Esc close", len(rows)), theme)
}

// drawEscMenuModal paints the editor's pause-style menu (Display / Continue /
// Exit). Buttons come from the shared []modalCmd builder so labels + actions can't drift.
func drawEscMenuModal(s *State, font rl.Font, theme render.Theme) {
	r := drawModalHeader(font, theme, escMenuModalW, escMenuModalH, "EDITOR MENU", theme.BorderActive)
	labels := cmdLabels(escMenuCmds(s))
	drawModalButtonsSel(font, modalButtonStack(r, labels), labels, s.modalCursor)
	render.DrawTextWithShadow(font, "(↑↓ select · Enter confirm · D/C/E hotkeys · Esc close)",
		r.X+modalContentInset, r.Y+modalSubheadingDY, editorFontHint, theme.TextHint)
}

func drawConfirmDirtyModal(s *State, font rl.Font, theme render.Theme) {
	r := drawModalHeader(font, theme, confirmDirtyModalW, confirmDirtyModalH, "UNSAVED CHANGES", theme.BorderActive)

	id := core.MapIDFromPath(s.area.Path)
	if id == "" {
		id = "(unsaved)"
	}
	body := fmt.Sprintf("%s has unsaved edits.", id)
	saveLabel := "S  Save and exit"
	discardLabel := "D  Discard and exit"
	switch s.pending {
	case pendingNew:
		body = fmt.Sprintf("%s has unsaved edits. Discarding for new map.", id)
		saveLabel = "S  Save then start new map"
		discardLabel = "D  Discard then start new map"
	case pendingOpen:
		body = fmt.Sprintf("%s has unsaved edits. Discarding to open another.", id)
		saveLabel = "S  Save then pick another map"
		discardLabel = "D  Discard then pick another map"
	}

	render.DrawRichText(font, body, rl.NewVector2(r.X+modalContentInset, r.Y+44), editorFontLabel, 1, theme.TextPrimary)
	// Contextual hint above the buttons (what Save/Discard do for this action).
	render.DrawTextWithShadow(font, hintForPending(saveLabel, discardLabel), r.X+modalContentInset, r.Y+66, editorFontHint, theme.TextHint)

	labels := cmdLabels(confirmDirtyCmds(s))
	drawModalButtons(font, modalButtonStack(r, labels), labels)
}

// hintForPending strips the "S  " / "D  " accelerator prefix off the save/discard
// captions so the hint reads as prose.
func hintForPending(saveLabel, discardLabel string) string {
	trim := func(s string) string {
		if i := strings.Index(s, "  "); i >= 0 {
			return strings.TrimSpace(s[i:])
		}
		return s
	}
	return trim(saveLabel) + " · " + trim(discardLabel)
}

// --- Coverage heatmap (View ▸ Coverage Heatmap) --------------------------------
//
// Distance-from-start field, cached on contentEpoch + start tile so the BFS runs
// once per edit, not per frame. dist < 0 = unreachable walkable tile (a pocket).
var (
	heatDist              []int
	heatEpoch             uint64
	heatStartX, heatStartZ int
	heatW, heatH          int
	heatMax               int
	heatReady             bool
)

func refreshHeatField(s *State) {
	a := &s.area
	w, h := a.Width, a.Height
	if heatReady && heatEpoch == s.contentEpoch && heatStartX == a.StartTileX && heatStartZ == a.StartTileZ && heatW == w && heatH == h {
		return
	}
	dist := make([]int, w*h)
	for i := range dist {
		dist[i] = -1
	}
	max := 0
	if a.InBounds(a.StartTileX, a.StartTileZ) && !a.BlockedAt(a.StartTileX, a.StartTileZ) {
		start := a.StartTileZ*w + a.StartTileX
		dist[start] = 0
		queue := [][2]int{{a.StartTileX, a.StartTileZ}}
		for len(queue) > 0 {
			p := queue[0]
			queue = queue[1:]
			px, pz := p[0], p[1]
			d := dist[pz*w+px]
			for _, dir := range [4]int{core.East, core.West, core.South, core.North} {
				if !a.StepElevationOK(px, pz, dir) {
					continue
				}
				dx, dz := core.FacingVector(dir)
				nx, nz := px+dx, pz+dz
				if nx < 0 || nx >= w || nz < 0 || nz >= h {
					continue
				}
				ni := nz*w + nx
				if dist[ni] != -1 || a.BlockedAt(nx, nz) {
					continue
				}
				dist[ni] = d + 1
				if d+1 > max {
					max = d + 1
				}
				queue = append(queue, [2]int{nx, nz})
			}
		}
	}
	heatDist, heatMax = dist, max
	heatEpoch, heatStartX, heatStartZ, heatW, heatH = s.contentEpoch, a.StartTileX, a.StartTileZ, w, h
	heatReady = true
}

// drawHeatmapOverlay tints each top-down tile by its distance from the start (green
// near → red far); walkable-but-unreachable pockets flag magenta. No-op in 3D.
func drawHeatmapOverlay(s *State, font rl.Font, theme render.Theme) {
	if !s.showHeatmap || s.isoView || s.rect.cellPx <= 0 {
		return
	}
	refreshHeatField(s)
	g := s.rect.grid
	rl.BeginScissorMode(int32(g.X), int32(g.Y), int32(g.Width), int32(g.Height))
	for z := 0; z < s.area.Height; z++ {
		for x := 0; x < s.area.Width; x++ {
			if s.area.WallAt(x, z) {
				continue // walls carry no coverage meaning
			}
			var tint rl.Color
			d := heatDist[z*s.area.Width+x]
			if d < 0 {
				if s.area.BlockedAt(x, z) {
					continue // deep water etc. — not a walkable pocket
				}
				tint = rl.NewColor(220, 40, 220, 120) // unreachable pocket
			} else {
				t := float32(0)
				if heatMax > 0 {
					t = float32(d) / float32(heatMax)
				}
				tint = rl.NewColor(uint8(60+195*t), uint8(200-150*t), 70, 105)
			}
			r := s.rect.tileRect(x, z)
			rl.DrawRectangleRec(r, tint)
		}
	}
	rl.EndScissorMode()
	// Legend caption at the grid's top-left.
	render.DrawRichText(font, "Coverage: green=near · red=far · magenta=unreachable",
		rl.NewVector2(g.X+8, g.Y+8), editorFontHint, 1, theme.TextPrimary)
}

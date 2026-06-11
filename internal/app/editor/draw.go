package editor

import (
	"crawler/internal/app/core"
	"crawler/internal/app/render"
	"fmt"
	"image/color"
	"math"
	"strconv"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// glyphStr maps a byte to its one-character string. Built once so
// drawTileGlyph doesn't allocate `string([]byte{ch})` per visible tile per
// frame — on a zoomed-out big map with the glyph overlay / Floors lens on,
// that was thousands of heap allocations every frame.
var glyphStr = func() [256]string {
	var t [256]string
	for i := range t {
		t[i] = string([]byte{byte(i)})
	}
	return t
}()

// tickLabels holds pre-formatted decimal labels for every 5th grid
// coordinate, up to the max map dimension. Built once so the axis-tick draw
// indexes coord/5 instead of fmt.Sprintf-ing each visible tick every frame.
var tickLabels = func() []string {
	t := make([]string, core.MaxMapDimension/5+2)
	for i := range t {
		t[i] = strconv.Itoa(i * 5)
	}
	return t
}()

// tickLabel returns the pre-formatted label for coordinate c (a multiple of
// 5), falling back to a fresh format if c somehow lands past the table.
func tickLabel(c int) string {
	if i := c / 5; i >= 0 && i < len(tickLabels) {
		return tickLabels[i]
	}
	return strconv.Itoa(c)
}

const (
	topbarH    = float32(48)
	toolbarH   = float32(38) // action button row beneath the topbar
	paletteW   = float32(220)
	metadataW  = float32(360)
	gridMargin = float32(8)
	layerTabH  = float32(32)
)

// Entity-list (chest/pack contents) geometry, used by entityModalLayoutFor
// and drawEntityListWindow so the painted rows and the click hit-rects
// can't drift: rows start entityListTop px below the card's top and step
// entityListRowH px apart.
const (
	entityListTop  = float32(52)
	entityListRowH = float32(22)
	// entityListTextInset is where a row's TEXT starts — the card gutter
	// (modalContentInset) plus a few px so the "> " cursor caret has room.
	// Derived from modalContentInset (defined lower in this file) so a gutter
	// change carries the row text with it instead of stranding a bare 24.
	entityListTextInset = modalContentInset + 8
)

// layout recomputes screen rectangles each frame from the current window
// size. Cell pixel size is the auto-fit size scaled by s.zoom; pan offsets
// nudge the plot off-center so users can drag around large maps.
func (s *State) layout() {
	w, h := render.ScreenSizeF()

	s.rect.topbar = rl.NewRectangle(0, 0, w, topbarH)
	// Action toolbar row directly beneath the topbar menu bar; everything
	// below starts at contentTop so adding the row just pushes the work
	// area down (all regions derive from this baseline, so the grid /
	// palette / metadata cascade automatically).
	s.rect.toolbar = rl.NewRectangle(0, topbarH, w, toolbarH)
	contentTop := topbarH + toolbarH
	// Layer tabs sit at the top of the palette column.
	tabsHeight := float32(layerCount) * layerTabH
	s.rect.layerTabs = rl.NewRectangle(0, contentTop, paletteW, tabsHeight)
	paletteY := contentTop + tabsHeight
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
	// baseX/baseY are gridX/gridY at pan==0 — the map centered in the grid
	// viewport. Clamp the pan against these so a drag (or a stale pan left
	// over from a higher zoom) can't fling the map off-screen: when the map
	// fits it's kept fully inside; when it overflows, panning reaches each
	// edge plus a little overscroll. This self-heals on zoom-out, where an
	// old large pan would otherwise shove the now-smaller map into a corner.
	baseX := s.rect.grid.X + (s.rect.grid.Width-totalW)/2
	baseY := s.rect.grid.Y + (s.rect.grid.Height-totalH)/2
	s.panX = core.ClampPanAxis(s.panX, baseX, s.rect.grid.X, s.rect.grid.Width, totalW, panOverscroll)
	s.panY = core.ClampPanAxis(s.panY, baseY, s.rect.grid.Y, s.rect.grid.Height, totalH, panOverscroll)
	s.rect.gridX = baseX + s.panX
	s.rect.gridY = baseY + s.panY
}

// panOverscroll is how far past a map edge the canvas pan may push when the
// map overflows the viewport — a small slack so edge tiles aren't jammed flush
// against the palette / metadata panels.
const panOverscroll = float32(48)

// cellAt converts a screen-space mouse position into a (x,z) tile, or -1,-1
// if the position is outside the grid plot.
func (s *State) cellAt(p rl.Vector2) (int, int) {
	if s.rect.cellPx <= 0 {
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

// Draw paints the editor view. Must be called inside Begin/EndDrawing.
// frameMouse caches rl.GetMousePosition() for the duration of a single
// Draw() pass. Helpers like drawButton fire many hover checks per
// frame; reading from this var keeps the CGo poll cost at one call per
// frame instead of one per widget. Safe because the editor draw path
// is single-threaded and the value is rewritten at the top of every
// Draw() before anything reads it.
var frameMouse rl.Vector2

// frameAssets stashes the current frame's render.Resources so modal handlers
// (whose signatures only carry *State / font / theme) can reach the loaded
// enemy textures the Foe Visualizer's live 3D preview needs. Same single-
// threaded, rewritten-at-top-of-Draw discipline as frameMouse. Update runs
// before Draw, so an Update-phase reader sees the PREVIOUS frame's bundle —
// fine, because Resources is the immutable loaded-asset set, stable across
// frames.
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
	drawLayerTabs(s, font, theme)
	drawPalette(s, font, theme)
	drawMetadata(s, font, theme)
	drawGrid(s, font)
	// Overview minimap sits in the grid's bottom-right corner, on top of the
	// grid but below scrollbars / status / modals.
	drawMinimap(s)
	// Recent-brush quick-pick row, bottom-left of the grid.
	drawBrushRecents(s, font)
	// Scrollbars paint on top of the panels + grid they scroll, but below
	// status toasts and modals.
	drawScrollbars(s)
	if len(s.statusLog) > 0 {
		drawStatus(s, font, theme)
	}
	if h, ok := modalHandlers[s.modal]; ok && h.draw != nil {
		h.draw(s, font, theme)
	}
	// Right-click context menu paints last so it sits over the grid and
	// any modal that happens to coexist with it (today the menu closes
	// before a modal opens — but cheap to keep this order future-proof).
	drawContextMenu(s, font, theme)
}

// modalHandler bundles a modal's draw + update functions. Replaces
// the prior parallel modalDrawers / modalUpdaters maps that had to
// stay in lockstep — adding a modal used to be two edits in two
// files. One table now: one row, both halves required.
type modalHandler struct {
	draw   func(*State, rl.Font, render.Theme)
	update func(*State) Action
}

var modalHandlers = map[modalKind]modalHandler{
	modalOpen:          {draw: drawOpenModal, update: updateOpenModal},
	modalSaveAs:        {draw: drawSaveAsModal, update: updateSaveAsModal},
	modalConfirmDirty:  {draw: drawConfirmDirtyModal, update: updateConfirmDirtyModal},
	modalPackEdit:      {draw: drawPackEditModal, update: updatePackEditModal},
	modalChestEdit:     {draw: drawChestEditModal, update: updateChestEditModal},
	modalSounds:        {draw: drawSoundsModal, update: updateSoundsModal},
	modalDoorEdit:      {draw: drawDoorEditModal, update: updateDoorEditModal},
	modalValidate:      {draw: drawValidateModal, update: updateValidateModal},
	modalEntityList:    {draw: drawEntityListModal, update: updateEntityListModal},
	modalNew:           {draw: drawNewMapModal, update: updateNewMapModal},
	modalCustomEnemies: {draw: drawCustomEnemiesModal, update: updateCustomEnemiesModal},
	modalEscMenu:       {draw: drawEscMenuModal, update: updateEscMenuModal},
	modalFoeView:       {draw: drawFoeViewModal, update: updateFoeViewModal},
}

// init asserts every dispatchable modalKind (modalNone and modalCount
// excluded — the former is "no modal open," the latter is the count
// sentinel) has a handler row. Mirrors the panic-at-init pattern
// AGENTS.md mandates for skill / tile / prop registries — a new
// modalFoo constant added to editor.go without a handler row now
// panics at startup instead of silently no-op'ing the dispatch.
func init() {
	for m := modalNone + 1; m < modalCount; m++ {
		if _, ok := modalHandlers[m]; !ok {
			panic(fmt.Sprintf("editor: modalKind %d has no modalHandlers entry — register draw + update functions", int(m)))
		}
	}
}

// doorEditHitTarget enumerates the clickable regions of the door edit
// modal. Mirrors the soundLayout hit-test shape but inline as an enum
// because the door modal only has a handful of stable targets.
type doorEditHitTarget int

const (
	doorHitOutside doorEditHitTarget = iota
	doorHitName
	doorHitTargetMap
	doorHitTargetDoor
	doorHitFacing
	doorHitStyle
	doorHitDelete
	doorHitClose
)

// doorEditHit pairs the hit kind with optional payload (the facing value
// when kind == doorHitFacing, the style value when kind == doorHitStyle).
type doorEditHit struct {
	kind   doorEditHitTarget
	facing int
	style  core.DoorStyle
}

// doorEditLayout returns the rectangles for every clickable region of
// the door edit modal so update and draw stay in sync. Pure function of
// screen size + position.
type doorEditLayout struct {
	card      rl.Rectangle
	nameField rl.Rectangle
	mapField  rl.Rectangle
	doorField rl.Rectangle
	facing    [core.FacingCount]rl.Rectangle
	style     [core.DoorStyleCount]rl.Rectangle
	deleteBtn rl.Rectangle
	closeBtn  rl.Rectangle
}

func doorEditLayoutFor() doorEditLayout {
	r := centeredCardRect(doorEditModalW, doorEditModalH)
	x := r.X + modalContentInset
	fw := r.Width - 32
	y := r.Y + 56
	fieldH := float32(28)
	rowGap := float32(48)
	nameField := rl.NewRectangle(x, y, fw, fieldH)
	y += rowGap
	mapField := rl.NewRectangle(x, y, fw, fieldH)
	y += rowGap
	doorField := rl.NewRectangle(x, y, fw, fieldH)
	y += rowGap + 6
	// Facing row: one equal-width button per Facing (mirrors the style row
	// below so a new facing scales the layout instead of clipping past 4).
	var facing [core.FacingCount]rl.Rectangle
	copy(facing[:], equalButtonRow(x, y, fw, fieldH, int(core.FacingCount)))
	y += rowGap
	// Style row: one button per DoorStyle.
	var style [core.DoorStyleCount]rl.Rectangle
	copy(style[:], equalButtonRow(x, y, fw, fieldH, int(core.DoorStyleCount)))
	y = r.Y + r.Height - modalBtnH - modalBottomInset
	deleteBtn := rl.NewRectangle(x, y, modalWideBtnW, modalBtnH)
	closeBtn := rl.NewRectangle(r.X+r.Width-modalWideBtnW-modalContentInset, y, modalWideBtnW, modalBtnH)
	return doorEditLayout{
		card:      r,
		nameField: nameField,
		mapField:  mapField,
		doorField: doorField,
		facing:    facing,
		style:     style,
		deleteBtn: deleteBtn,
		closeBtn:  closeBtn,
	}
}

// doorEditHitTest reports which region the mouse position p falls in.
// Used by updateDoorEditModal; doorHitOutside is the default so the
// caller can branch on it explicitly.
func doorEditHitTest(s *State, p rl.Vector2) doorEditHit {
	l := doorEditLayoutFor()
	if !pointIn(p, l.card) {
		return doorEditHit{kind: doorHitOutside}
	}
	if pointIn(p, l.nameField) {
		return doorEditHit{kind: doorHitName}
	}
	if pointIn(p, l.mapField) {
		return doorEditHit{kind: doorHitTargetMap}
	}
	if pointIn(p, l.doorField) {
		return doorEditHit{kind: doorHitTargetDoor}
	}
	for i, fr := range l.facing {
		if pointIn(p, fr) {
			return doorEditHit{kind: doorHitFacing, facing: i}
		}
	}
	for i, sr := range l.style {
		if pointIn(p, sr) {
			return doorEditHit{kind: doorHitStyle, style: core.DoorStyle(i)}
		}
	}
	if pointIn(p, l.deleteBtn) {
		return doorEditHit{kind: doorHitDelete}
	}
	if pointIn(p, l.closeBtn) {
		return doorEditHit{kind: doorHitClose}
	}
	// Click inside card but not on a clickable region — treat as a no-op
	// so a stray click inside the card doesn't dismiss the modal.
	return doorEditHit{kind: doorHitOutside}
}

// --- Top bar ---------------------------------------------------------------

// topbarBtn pairs the displayed label with the action func to run on
// click. Replaces the prior string-id + separate `switch name` dispatch
// in handleTopbarButton — that pattern had to stay in lockstep across
// draw.go and input.go (adding a button took two edits, and a typo
// in the id was silent). Now the action lives on the row.
type topbarBtn struct {
	label  string
	action func(*State)
	// activeFn, when set, draws the button highlighted while it returns
	// true — used for toggle actions (e.g. the glyph overlay) so the
	// button reads as "on" without a separate indicator.
	active func(*State) bool
}

var topbarBtns = []topbarBtn{
	{label: "New", action: newMap},
	{label: "Open", action: requestOpen},
	{label: "Save", action: saveCurrent},
	{label: "Save As", action: openSaveAsModal},
	{label: "Sounds", action: openSoundsModal},
	{label: "Enemies", action: openCustomEnemiesModal},
	{label: "Foes", action: openFoeViewModal},
	{label: "Objects", action: openEntityListModal},
	{label: "Validate", action: openValidateModal},
	{label: "Back", action: func(s *State) { s.exitRequested = true }},
}

// toolbarBtns is the action row beneath the topbar — the editing
// commands that used to be keyboard-only (the hotkeys still work as
// accelerators, but every one now has a button so the editor is
// navigable by mouse alone). Each action reuses the same handler the
// hotkey calls, so the two can't drift. Layer switching lives in the
// left layer-tabs column and brush selection in the palette, so those
// aren't repeated here.
// toolbarActionBtns are the editing-command buttons. The full toolbar
// (toolbarBtns) is the tool-select group followed by these — assembled in
// init below so the tool group reads from toolModeLabels and a new tool needs
// no button wiring.
var toolbarActionBtns = []topbarBtn{
	{label: "Undo", action: undoOne},
	{label: "Redo", action: redoOne},
	{label: "Fill", action: fillEntireLayer},
	{label: "Brush -", action: func(s *State) { stepBrushSize(s, -1) }},
	{label: "Brush +", action: func(s *State) { stepBrushSize(s, +1) }},
	{label: "Center", action: func(s *State) { centerViewOnTile(s, s.area.StartTileX, s.area.StartTileZ) }},
	{label: "Reset View", action: resetView},
	{label: "Lvl -", action: func(s *State) { stepEditLevel(s, -1) }},
	{label: "Lvl +", action: func(s *State) { stepEditLevel(s, +1) }},
	{label: "Floors",
		action: toggleLevelFocus,
		active: func(s *State) bool { return s.levelFocus }},
	{label: "Ramp",
		action: func(s *State) { s.rampMode = !s.rampMode },
		active: func(s *State) bool { return s.rampMode }},
	{label: "Glyphs",
		action: toggleTileGlyphs,
		active: func(s *State) bool { return s.showTileGlyphs }},
	{label: "Links",
		action: func(s *State) { s.showDoorLinks = !s.showDoorLinks },
		active: func(s *State) bool { return s.showDoorLinks }},
	{label: "Phase", action: cyclePreviewPhase},
	{label: "Test", action: func(s *State) { s.testRequested = true }},
}

// toolbarBtns is the full action row: the tool-select group (Brush / Line /
// Rect / Box / Flood / Pick, highlighted to show the active tool) followed by
// the editing commands. Assembled once at init from toolModeLabels so adding a
// tool is one enum row + label — no button wiring.
var toolbarBtns []topbarBtn

func init() {
	for m, label := range toolModeLabels {
		mode := toolMode(m)
		toolbarBtns = append(toolbarBtns, topbarBtn{
			label:  label,
			action: func(s *State) { s.tool = mode },
			active: func(s *State) bool { return s.tool == mode },
		})
	}
	toolbarBtns = append(toolbarBtns, toolbarActionBtns...)
}

// toolbarButtonAt returns the index of the toolbar button under p, or
// buttonStripHit / drawButtonStrip are the shared left-to-right button
// walk for the topbar (menu bar) and the toolbar (action row) — both are
// `[]topbarBtn` strips at a fixed Y/height, so one walk keeps their draw
// and hit-test in lockstep instead of four hand-maintained copies.
const buttonStripStartX = float32(8)

func buttonStripHit(btns []topbarBtn, y, h float32, p rl.Vector2) int {
	x := buttonStripStartX
	for i, b := range btns {
		w := buttonWidth(b.label)
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
		w := buttonWidth(b.label)
		drawButton(font, rl.NewRectangle(x, y, w, h), b.label, b.active != nil && b.active(s))
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
	_ = theme
	rl.DrawRectangleRec(s.rect.toolbar, bgWindow)
	rl.DrawLineEx(
		rl.NewVector2(0, s.rect.toolbar.Y+toolbarH),
		rl.NewVector2(s.rect.toolbar.Width, s.rect.toolbar.Y+toolbarH),
		1, outlineHard)
	drawButtonStrip(font, s, toolbarBtns, s.rect.toolbar.Y+6, toolbarH-12)
	// Height-selector readout (right-aligned): the level the Elevation brush
	// stamps + the slice-view focus; flags Ramp tool-mode when active.
	label := fmt.Sprintf("Height: %d", s.editLevel)
	if s.rampMode {
		label += "  [RAMP]"
	}
	sz := editorFontLabel
	m := rl.MeasureTextEx(font, label, sz, 1)
	rl.DrawTextEx(font, label,
		rl.NewVector2(s.rect.toolbar.Width-m.X-12, s.rect.toolbar.Y+(toolbarH-sz)/2),
		sz, 1, rl.NewColor(220, 210, 180, 255))
}

// topbarButtonAt returns the index of the button under p, or -1.
// Integer index pairs with topbarBtns so the caller can fire the
// action directly without a stringly-typed indirection.
func topbarButtonAt(s *State, p rl.Vector2) int {
	if !pointIn(p, s.rect.topbar) {
		return -1
	}
	return buttonStripHit(topbarBtns, 6, topbarH-12, p)
}

// topbarInfoKey captures everything the topbar's name + info readouts are
// derived from. When it's unchanged frame-to-frame, drawTopbar reuses the
// cached strings + measures instead of re-running MapIDFromPath, AreaTileSummary
// (which allocates), several Sprintfs, and three MeasureTextEx every frame.
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

	drawButtonStrip(font, s, topbarBtns, 6, topbarH-12)

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
		topbarNameMeasure = rl.MeasureTextEx(font, topbarNameLabel, editorFontTopbar, 1)

		coord := "—"
		hoverDesc := ""
		if s.hoverX >= 0 {
			coord = core.TileCoord(s.hoverX, s.hoverZ)
			hoverDesc = core.AreaTileSummary(s.area, s.hoverX, s.hoverZ)
		}
		topbarInfoLabel = fmt.Sprintf("cell %s   %s   layer %s   brush %dx%d   zoom %.0f%%   phase %s (T)   undo %d/%d",
			coord, hoverDesc, layerName(s.layer), s.brushSize, s.brushSize, s.zoom*100, core.PhaseName(s.previewPhase), len(s.undo), undoLimit)
		topbarInfoMeasure = rl.MeasureTextEx(font, topbarInfoLabel, editorFontLabel, 1)

		topbarInfoKeyCache = key
		topbarInfoReady = true
	}

	// Positioning re-reads the live window width each frame (it can change on
	// resize without invalidating the cached strings); only the strings +
	// their measured sizes are memoized.
	labelX := s.rect.topbar.Width - topbarNameMeasure.X - 10
	render.DrawTextWithShadow(font, topbarNameLabel,
		labelX, (topbarH-topbarNameMeasure.Y)/2,
		editorFontTopbar, theme.TextMuted)
	infoX := labelX - topbarInfoMeasure.X - 24
	render.DrawTextWithShadow(font, topbarInfoLabel, infoX, (topbarH-topbarInfoMeasure.Y)/2, editorFontLabel, theme.TextHint)
}

// topbarBtnWidths overrides the default 64-px topbar button width for
// labels that need extra space. Adding a wider button is a one-row
// edit; missing entries fall through to the default.
var topbarBtnWidths = map[string]float32{
	"Save As":    90,
	"Validate":   90,
	"Back":       70,
	"Reset View": 96,
	"Brush -":    78,
	"Brush +":    78,
}

// approxTextWidth estimates a label's pixel width without a font handle —
// ~0.5px per character per font point (so editorFontBody=16 ⇒ 8px/char).
// The button and context-menu sizers lay out before they have the loaded
// font, so both share this one heuristic instead of two disagreeing
// per-char constants.
func approxTextWidth(label string, fontSize float32) float32 {
	return float32(len(label)) * fontSize * 0.5
}

func buttonWidth(label string) float32 {
	if w, ok := topbarBtnWidths[label]; ok {
		return w
	}
	// Auto-size to the label so long captions ("Discard", "Overwrite",
	// "Exit to Title") don't overflow a fixed-width button. ~8px/char at
	// editorFontBody plus padding, floored at 72 so short labels stay
	// tidy. Deterministic from the string, so a modal's draw and its
	// click hit-test (both via modalButtonRow) agree without measuring.
	w := approxTextWidth(label, editorFontBody) + 28
	if w < 72 {
		w = 72
	}
	return w
}

// Modal button-layout tunables, shared by every modal's button helpers so
// the card padding / button size is one source instead of bare literals
// repeated across the row / stack / grid layouts and their hit-tests.
const (
	modalBtnH         = float32(30)  // action button height
	modalContentInset = float32(16)  // left/right card padding (body width = card.Width - 2*inset)
	modalBtnGap       = float32(8)   // gap between stacked / row modal buttons
	modalBottomInset  = float32(14)  // gap from the card's bottom edge to the button block
	tightBtnGap       = float32(6)   // gap for dense strips: the topbar/toolbar, the wrapped add grid, equal-width rows
	modalWideBtnW     = float32(110) // width of the Delete / Close affordance shared by the door + custom-enemy modals
)

// modalContentWidth is the usable inner width of a modal card.
func modalContentWidth(card rl.Rectangle) float32 { return card.Width - 2*modalContentInset }

// modalButtonRow lays buttons out left-to-right along the bottom-left of a
// modal card (auto-width per label) and returns their rects in order.
// Used where a few short actions fit one line (e.g. the open-map modal's
// Open / Rename / Delete / Duplicate).
func modalButtonRow(card rl.Rectangle, labels []string) []rl.Rectangle {
	return buttonRowAt(card.X+modalContentInset, card.Y+card.Height-modalBtnH-modalBottomInset, labels)
}

// buttonRowAt lays a row of auto-width modal buttons left-to-right from
// (x, y) using the shared modalBtnH / modalBtnGap spec. modalButtonRow
// anchors it to a card's bottom-left; callers with a bespoke anchor (the
// Foe Visualizer's right column, the new-map modal's right-aligned pair)
// place the same row shape elsewhere without re-deriving size/gap math.
func buttonRowAt(x, y float32, labels []string) []rl.Rectangle {
	rects := make([]rl.Rectangle, len(labels))
	for i, lbl := range labels {
		w := buttonWidth(lbl)
		rects[i] = rl.NewRectangle(x, y, w, modalBtnH)
		x += w + modalBtnGap
	}
	return rects
}

// buttonRowWidth is the total horizontal span buttonRowAt would occupy
// for labels — used to right-anchor a row against a card edge.
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

// modalButtonStack lays full-width buttons out vertically, anchored to the
// bottom of the card (so header + body text own the top) and returns them
// in top-to-bottom order. Full-width means they can never overflow the
// card horizontally — the robust choice for menus / confirm dialogs.
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

// modalButtonRow, modalButtonStack, and buttonGrid (the wrapped variant
// the entity modals use) are the geometry sources a modal's draw and its
// click handler share so the two can't drift — the same role
// doorEditLayoutFor plays for the door modal. drawModalButtons paints a
// computed rect set; modalButtonHit returns the clicked index.
func drawModalButtons(font rl.Font, rects []rl.Rectangle, labels []string) {
	for i, r := range rects {
		drawButton(font, r, labels[i], false)
	}
}

func modalButtonHit(rects []rl.Rectangle) int {
	if !rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		return -1
	}
	mp := rl.GetMousePosition()
	for i, r := range rects {
		if pointIn(mp, r) {
			return i
		}
	}
	return -1
}

// modalCmd is a labeled action for a modal's buttons. The same builder
// produces the cmds for both draw (reads .label) and the click handler
// (runs .run), so a button's caption and its action live on one row and
// can't drift — this is what lets the confirm/menu modals drop their
// fragile "hit==2 means Cancel" index switches. `hot` is an optional
// keyboard accelerator (Esc / Y / D …) that fires the same .run; `run`
// returns the editor Action to propagate (ActionNone for most).
type modalCmd struct {
	label string
	hot   func() bool
	run   func() Action
}

// runModalCmds fires the cmd under a left-click (rects come from
// modalButtonStack/Row over the same labels) OR whose `hot` accelerator
// is pressed, and returns its Action plus true. The label↔action pairing
// lives in the cmd row, so there's no index-to-meaning switch to keep in
// lockstep with the label order.
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

// buttonGrid lays auto-width buttons left-to-right within [x, x+maxW],
// wrapping to a new row when the next would exceed maxW, growing downward
// from y. Returns rects in label order — the geometry source shared by an
// entity modal's draw and click hit-test. (A short label set that fits one
// line simply doesn't wrap.)
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
// height h, with tightBtnGap between them. The "divide a row into N equal
// buttons + a gap" math was hand-rolled at the door facing/style rows, the
// metadata material buttons, and the custom-enemy base-sprite row — this
// is the one source for all of them.
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

// buttonGridHeight is the total vertical span buttonGrid uses for labels
// at the given width (0 for an empty set).
func buttonGridHeight(maxW float32, labels []string) float32 {
	if len(labels) == 0 {
		return 0
	}
	rects := buttonGrid(0, 0, maxW, labels)
	last := rects[len(rects)-1]
	return last.Y + last.Height
}

// entityModalLayout is the shared geometry for the pack/chest editors: the
// scrolled list window plus the action-button row and the wrapped add
// grid, all derived once so draw and the click handler agree.
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
	maxRows := int((listBottom - listTop) / entityListRowH)
	if maxRows < 1 {
		maxRows = 1
	}
	topRow, end := scrollWindow(cursor, count, maxRows)
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

// drawEntityListWindow paints the pack/chest list rows for a precomputed
// layout window, highlighting the cursor row, with ▲/▼ "N more" clip
// indicators.
func drawEntityListWindow(font rl.Font, theme render.Theme, lay entityModalLayout, count, cursor int, emptyText string, rowText func(int) string) {
	if count == 0 {
		rl.DrawTextEx(font, emptyText, rl.NewVector2(lay.card.X+modalContentInset, lay.listTop), editorFontLabel, 1, theme.TextHint)
		return
	}
	y := lay.listTop
	if lay.topRow > 0 {
		rl.DrawTextEx(font, fmt.Sprintf("▲ %d more", lay.topRow), rl.NewVector2(lay.card.X+entityListTextInset, y-16), editorFontHint, 1, theme.TextHint)
	}
	for i := lay.topRow; i < lay.end; i++ {
		text := rowText(i)
		col := theme.TextMuted
		if i == cursor {
			col = theme.BorderActive
			text = "> " + text
		}
		render.DrawTextWithShadow(font, text, lay.card.X+entityListTextInset, y, editorFontBody, col)
		y += lay.rowH
	}
	if lay.end < count {
		rl.DrawTextEx(font, fmt.Sprintf("▼ %d more", count-lay.end), rl.NewVector2(lay.card.X+entityListTextInset, y), editorFontHint, 1, theme.TextHint)
	}
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
	measure := rl.MeasureTextEx(font, label, editorFontBody, 1)
	rl.DrawTextEx(font, label,
		rl.NewVector2(r.X+(r.Width-measure.X)/2, r.Y+(r.Height-measure.Y)/2),
		editorFontBody, 1, text)
}

// drawStepperButtons paints the shared "−" / "+" adjuster pair for a
// numeric stepper row (sidebar dims, new-map dims, custom-enemy stats).
// The value cell differs per caller — an editable drawTextField vs a
// drawReadonlyValue — so only the button pair, which was hand-repeated
// at every stepper, is shared here.
func drawStepperButtons(font rl.Font, minus, plus rl.Rectangle) {
	drawButton(font, minus, "-", false)
	drawButton(font, plus, "+", false)
}

// stepperRow lays out a numeric stepper at (x,y): a value cell of width
// valueW, then two modalBtnH-square "−"/"+" buttons each preceded by gap.
// Parallels drawStepperButtons (which paints the pair) so the three stepper
// modals (sidebar dims, new-map dims, custom-enemy stats) share one
// placement formula instead of each hand-deriving the +offsets.
func stepperRow(x, y, valueW, gap float32) (value, minus, plus rl.Rectangle) {
	value = rl.NewRectangle(x, y, valueW, modalBtnH)
	minus = rl.NewRectangle(value.X+value.Width+gap, y, modalBtnH, modalBtnH)
	plus = rl.NewRectangle(minus.X+minus.Width+gap, y, modalBtnH, modalBtnH)
	return value, minus, plus
}

// Door-link overlay colors.
var (
	doorLinkColor         = rl.NewColor(120, 200, 255, 220) // resolved same-map link
	doorLinkWarnColor     = rl.NewColor(255, 90, 90, 235)   // dangling: target_door doesn't resolve
	doorLinkExternalColor = rl.NewColor(186, 162, 255, 205) // cross-map link (target in another file)
)

// markerPackDot is the flat pack color for the small dots in the Objects list
// and the minimap (distinct from the per-kind packMarkerColor used on the
// canvas). One source so the two dot sites can't drift.
var markerPackDot = rl.NewColor(222, 92, 80, 255)

// doorSpawnByName finds the door spawn with the given name in this map's
// spawn list. Used by the door-link overlay to resolve same-map targets.
func doorSpawnByName(spawns []core.DoorSpawn, name string) (core.DoorSpawn, bool) {
	for _, d := range spawns {
		if d.Name == name {
			return d, true
		}
	}
	return core.DoorSpawn{}, false
}

// drawDoorLinks renders the door-link diagnostic overlay (the "Links" toolbar
// toggle): a connector from each door to its same-map target door, a warning
// ring on doors whose target_door doesn't resolve in this map, and a neutral
// ring on cross-map doors (their target lives in another file — the Validate
// modal checks those). Doors are few, so this isn't culled.
func drawDoorLinks(s *State, cell float32) {
	// An empty target is an unset door (drawn as a self-link); otherwise defer
	// to the canonical self-portal test (SelfMapToken or this map's own id).
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

// clampF clamps a float to [lo, hi].
func clampF(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// minimapRect computes the on-screen rectangle of the overview minimap — the
// downscaled whole-map thumbnail pinned to the grid pane's bottom-right corner
// — and whether it should show (hidden when there's no map or the grid pane is
// too small to spare the room). Shared by the draw and the click-to-jump
// hit-test so they can't drift.
func minimapRect(s *State) (rl.Rectangle, bool) {
	if s.area.Width == 0 || s.area.Height == 0 || s.rect.cellPx <= 0 {
		return rl.Rectangle{}, false
	}
	if s.rect.grid.Width < 260 || s.rect.grid.Height < 260 {
		return rl.Rectangle{}, false
	}
	const maxDim = float32(150)
	// pad ≥ scrollbarThickness (+ backing slack) so the minimap clears the
	// canvas scrollbar gutters on the grid's right + bottom edges when zoomed in.
	const pad = float32(16)
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

// drawMinimap paints the overview minimap: a floor base, wall pixels, entity
// dots, and a frame marking the slice of the map currently visible in the grid
// pane. Pixel-space iteration bounds the cost to the minimap's own area
// (~150² max), independent of map size. Click-to-jump is in updateMouse.
func drawMinimap(s *State) {
	mr, ok := minimapRect(s)
	if !ok {
		return
	}
	scale := mr.Width / float32(s.area.Width)
	rl.DrawRectangleRec(rl.NewRectangle(mr.X-4, mr.Y-4, mr.Width+8, mr.Height+8), rl.NewColor(12, 14, 20, 214))
	rl.DrawRectangleLinesEx(rl.NewRectangle(mr.X-4, mr.Y-4, mr.Width+8, mr.Height+8), 1, editorBorderDim)
	rl.DrawRectangleRec(mr, rl.NewColor(58, 56, 50, 255))

	wallCol := rl.NewColor(150, 152, 160, 255)
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
			if s.area.Walls[tz][tx] == core.TileRock {
				rl.DrawPixel(int32(mr.X)+int32(px), int32(mr.Y)+int32(py), wallCol)
			}
		}
	}

	dot := func(tx, tz int, col rl.Color) {
		rl.DrawRectangle(int32(mr.X+float32(tx)*scale), int32(mr.Y+float32(tz)*scale), 2, 2, col)
	}
	for _, p := range s.area.PackSpawns {
		if len(p.Members) > 0 {
			dot(p.TileX, p.TileZ, markerPackDot)
		}
	}
	for _, c := range s.area.ChestSpawns {
		dot(c.TileX, c.TileZ, render.MarkerChest)
	}
	for _, d := range s.area.DoorSpawns {
		dot(d.TileX, d.TileZ, render.MarkerDoor)
	}
	dot(s.area.StartTileX, s.area.StartTileZ, render.MarkerStart)

	// Viewport frame — the slice of the map currently visible in the grid pane.
	w, h := float32(s.area.Width), float32(s.area.Height)
	vx0 := clampF((s.rect.grid.X-s.rect.gridX)/s.rect.cellPx, 0, w)
	vx1 := clampF((s.rect.grid.X+s.rect.grid.Width-s.rect.gridX)/s.rect.cellPx, 0, w)
	vz0 := clampF((s.rect.grid.Y-s.rect.gridY)/s.rect.cellPx, 0, h)
	vz1 := clampF((s.rect.grid.Y+s.rect.grid.Height-s.rect.gridY)/s.rect.cellPx, 0, h)
	rl.DrawRectangleLinesEx(
		rl.NewRectangle(mr.X+vx0*scale, mr.Y+vz0*scale, (vx1-vx0)*scale, (vz1-vz0)*scale),
		1, rl.NewColor(255, 240, 180, 230))
}

// brushRecentsVisible reports whether the recent-brush swatch row should show.
func brushRecentsVisible(s *State) bool {
	return len(s.recentBrushes) > 0 && s.rect.grid.Width >= 260 && s.rect.grid.Height >= 200
}

// brushRecentRect is the i-th recent-brush swatch rectangle, laid out left to
// right in the grid pane's bottom-left corner. Shared by draw + click hit-test
// so they can't drift.
func brushRecentRect(s *State, i int) rl.Rectangle {
	// pad ≥ scrollbarThickness (+ slack) so the row clears the bottom canvas
	// scrollbar gutter when zoomed in.
	const sw, pad, gap = float32(26), float32(16), float32(4)
	x0 := s.rect.grid.X + pad
	y := s.rect.grid.Y + s.rect.grid.Height - sw - pad
	return rl.NewRectangle(x0+float32(i)*(sw+gap), y, sw, sw)
}

func recentSwatchColor(ref brushRef) rl.Color {
	palette := layerBrushes[ref.layer]
	if ref.idx < 0 || ref.idx >= len(palette) {
		return rl.NewColor(80, 80, 90, 255)
	}
	return palette[ref.idx].Color
}

// drawBrushRecents paints the recent-brush quick-pick row (newest at left).
// Each swatch shows the brush color + its layer's initial; clicking one (see
// updateMouse) jumps to that layer + brush.
func drawBrushRecents(s *State, font rl.Font) {
	if !brushRecentsVisible(s) {
		return
	}
	n := len(s.recentBrushes)
	first := brushRecentRect(s, 0)
	last := brushRecentRect(s, n-1)
	bg := rl.NewRectangle(first.X-4, first.Y-4, (last.X+last.Width)-first.X+8, first.Height+8)
	rl.DrawRectangleRec(bg, rl.NewColor(12, 14, 20, 205))
	rl.DrawRectangleLinesEx(bg, 1, editorBorderDim)
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
			render.DrawTextWithShadow(font, ln[:1], r.X+3, r.Y+1, editorFontTick, rl.NewColor(245, 245, 250, 255))
		}
	}
}

// --- Layer tabs ------------------------------------------------------------

func layerTabRect(s *State, i int) rl.Rectangle {
	return rl.NewRectangle(
		s.rect.layerTabs.X,
		s.rect.layerTabs.Y+float32(i)*layerTabH,
		s.rect.layerTabs.Width,
		layerTabH,
	)
}

func layerTabAt(s *State, p rl.Vector2) int {
	if !pointIn(p, s.rect.layerTabs) {
		return -1
	}
	for i := 0; i < layerCount; i++ {
		if pointIn(p, layerTabRect(s, i)) {
			return i
		}
	}
	return -1
}

// layerEyeRect is the small visibility-toggle box at the right edge of a
// layer tab. Clicking it toggles the layer's hidden flag (Alt-click solos);
// it's hit-tested before the tab-select so the eye doesn't also switch layers.
func layerEyeRect(s *State, i int) rl.Rectangle {
	r := layerTabRect(s, i)
	const eye = float32(20)
	return rl.NewRectangle(r.X+r.Width-6-eye-6, r.Y+(r.Height-eye)/2, eye, eye)
}

// drawLayerEye paints the open/closed visibility eye for a layer tab.
func drawLayerEye(r rl.Rectangle, open, hover bool) {
	cx := r.X + r.Width/2
	cy := r.Y + r.Height/2
	col := rl.NewColor(176, 182, 196, 255)
	if !open {
		col = rl.NewColor(98, 102, 112, 255)
	}
	if hover {
		col = rl.NewColor(232, 236, 246, 255)
	}
	if open {
		rl.DrawCircleLines(int32(cx), int32(cy), r.Width*0.34, col)
		rl.DrawCircleV(rl.NewVector2(cx, cy), r.Width*0.14, col)
	} else {
		rl.DrawLineEx(rl.NewVector2(r.X+r.Width*0.18, cy), rl.NewVector2(r.X+r.Width*0.82, cy), 2, col)
	}
}

func drawLayerTabs(s *State, font rl.Font, theme render.Theme) {
	rl.DrawRectangleRec(s.rect.layerTabs, bgWindow)
	// Re-use the Draw()-cached frameMouse instead of calling
	// rl.GetMousePosition per-tab — the call crosses CGo.
	mp := frameMouse
	for i := 0; i < layerCount; i++ {
		r := layerTabRect(s, i)
		active := Layer(i) == s.layer
		hidden := s.layerHidden[i]
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
			// Dim the label so the hidden state reads across the whole tab,
			// not just the eye.
			text = rl.NewColor(112, 116, 126, 255)
		}
		// Inset the tab so consecutive tabs don't share a border.
		inner := rl.NewRectangle(r.X+6, r.Y+3, r.Width-12, r.Height-6)
		rl.DrawRectangleRec(inner, bg)
		rl.DrawRectangleLinesEx(inner, 1, border)
		label := fmt.Sprintf("%d %s", i+1, layerName(Layer(i)))
		render.DrawTextWithShadow(font, label, inner.X+10, inner.Y+(inner.Height-16)/2, editorFontBody, text)
		eye := layerEyeRect(s, i)
		drawLayerEye(eye, !hidden, pointIn(mp, eye))
	}
}

// --- Palette ---------------------------------------------------------------

func paletteToolAt(s *State, p rl.Vector2) int {
	if !pointIn(p, s.rect.palette) {
		return -1
	}
	// Reject clicks in the heading band. drawPalette scissor-clips the entry
	// list to below palette.Y+headerReserve, so a scrolled entry whose rect
	// overlaps the heading is visually hidden yet would otherwise still
	// hit-test here. Mirrors handleMetadataClick's header-band guard.
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

// paletteRowStride is rowH + the vertical spacing between rows. Used for
// both hit-testing (paletteEntryRect) and laying out the hint block below
// the palette so they stay in sync.
const (
	paletteRowH      = float32(32)
	paletteRowStride = paletteRowH + 4
	// headerReserve is the vertical space inside the panel reserved for
	// the panel heading (BRUSHES / MAP); body content sits below this
	// band and the scissor clip below the heading uses it so scrolled
	// content can't paint into the heading row. Must clear the heading
	// underline tick painted by render.DrawHeading.
	headerReserve = float32(40)
)

// paletteHints is the keyboard-shortcut cheat sheet rendered below
// the brush list. Promoted from a hand-counted const + open-coded
// slice literal to a single source of truth: paletteContentHeight
// computes scroll bounds from len(paletteHints), so adding or
// removing a hint can never drift from the layout math.
var paletteHints = []string{
	"L-drag: paint",
	"R-click: erase",
	"R-click entity: edit/move/face",
	"Shift+drag: rect",
	"Ctrl+click: fill region",
	"Ctrl+Shift+F: fill all",
	"Tab: next layer",
	"Alt+1..7: jump layer",
	"1..9 / Shift+1..9: brush",
	"[ ] brush size",
	"arrows: cursor",
	"space: paint, bksp: erase",
	"G: center on start",
	"wheel: zoom",
	"mid-drag: pan",
	"home: reset view",
	"Ctrl+S save",
	"Ctrl+O open",
	"Ctrl+Z undo / Y redo",
	"Ctrl+N new",
	"F5 playtest",
	"Ctrl+F5: test here",
	"T cycle phase",
	"R rotate start",
	"Esc back",
}

func paletteEntryRect(s *State, i int) rl.Rectangle {
	y := s.rect.palette.Y + headerReserve + float32(i)*paletteRowStride - s.paletteScroll[s.layer]
	return rl.NewRectangle(s.rect.palette.X+8, y, s.rect.palette.Width-16, paletteRowH)
}

// visiblePaletteRange returns the half-open index range [start, end)
// of palette entries whose rect intersects the visible band of the
// palette panel. Lets drawPalette iterate only the rows that will
// actually render instead of every entry. Clamped to [0, n] so an
// out-of-range scroll value during a transient resize can't index past
// the slice. Pure arithmetic — no raylib calls, safe to compute once
// per frame.
func visiblePaletteRange(s *State, n int) (int, int) {
	if n <= 0 {
		return 0, 0
	}
	scroll := s.paletteScroll[s.layer]
	top := s.rect.palette.Y + headerReserve
	bot := s.rect.palette.Y + s.rect.palette.Height
	// Each entry starts at top + i*stride - scroll. Solving for the
	// first i where (start + paletteRowH) >= top gives:
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

// paletteContentHeight returns the pixel height required to render the
// active layer's full brush list (including the top/bottom padding and
// the hint footer). Used by ScrollPalette to clamp the scroll offset
// so the last row stays visible. Reads len(paletteHints) directly so
// adding a shortcut row updates both the rendered list AND the
// scroll bound in one edit.
func paletteContentHeight(s *State) float32 {
	palette := layerBrushes[s.layer]
	return headerReserve + float32(len(palette))*paletteRowStride + 12 + float32(len(paletteHints))*16 + 16
}

// ScrollPalette adjusts the active layer's palette scroll offset by
// dy pixels (positive = scroll down / show later entries). Clamps to
// [0, max] where max keeps the content end visible. Exposed for
// updateMouse / hotkey wheel handlers in input.go.
func ScrollPalette(s *State, dy float32) {
	if s.rect.palette.Height <= 0 {
		return
	}
	max := paletteContentHeight(s) - s.rect.palette.Height
	if max < 0 {
		max = 0
	}
	s.paletteScroll[s.layer] += dy
	if s.paletteScroll[s.layer] < 0 {
		s.paletteScroll[s.layer] = 0
	}
	if s.paletteScroll[s.layer] > max {
		s.paletteScroll[s.layer] = max
	}
}

func drawPalette(s *State, font rl.Font, theme render.Theme) {
	rl.DrawRectangleRec(s.rect.palette, bgPaletteCol)
	rl.DrawLineEx(
		rl.NewVector2(s.rect.palette.X+s.rect.palette.Width, s.rect.palette.Y),
		rl.NewVector2(s.rect.palette.X+s.rect.palette.Width, s.rect.palette.Y+s.rect.palette.Height),
		1, outlineHard)

	render.DrawHeading(font, "BRUSHES", int32(s.rect.palette.X+12), int32(s.rect.palette.Y+8), theme.BorderStrong)

	// Clamp scroll to current content bounds — entry count can change
	// between frames (a brush table reload would be unusual, but the
	// clamp keeps us honest about not scrolling past the last entry).
	ScrollPalette(s, 0)

	// Clip the palette region so off-screen entries (above and below)
	// don't draw over the topbar/tabs or the grid panel. The clip lives
	// for the duration of this call; the BeginScissorMode/End pair
	// also catches the hint footer that follows the entries.
	rl.BeginScissorMode(int32(s.rect.palette.X), int32(s.rect.palette.Y+headerReserve),
		int32(s.rect.palette.Width), int32(s.rect.palette.Height-headerReserve))
	defer rl.EndScissorMode()

	palette := layerBrushes[s.layer]
	labels := paletteLabels[s.layer]
	// Re-use the Draw()-cached frameMouse instead of crossing the CGo
	// boundary per palette entry — long brush lists call this loop
	// every frame the editor is open.
	mp := frameMouse
	// Window the iteration to rows that can actually render. The old
	// code walked every entry and culled by Y; on a long palette
	// (props is ~30) that's wasted entryRect + pointIn work for rows
	// the scissor clips anyway.
	visStart, visEnd := visiblePaletteRange(s, len(palette))
	for i := visStart; i < visEnd; i++ {
		b := palette[i]
		r := paletteEntryRect(s, i)
		active := s.brushIdx[s.layer] == i
		hovered := pointIn(mp, r)
		drawBrushSwatchRow(font, r, labels[i], s.layer, b, active, hovered, 16)
	}

	y := s.rect.palette.Y + headerReserve + float32(len(palette))*paletteRowStride + 12 - s.paletteScroll[s.layer]
	for _, h := range paletteHints {
		rl.DrawTextEx(font, h, rl.NewVector2(s.rect.palette.X+12, y), editorFontAccent, 1, theme.TextHint)
		y += 16
	}
}

// drawBrushSwatchRow renders one selectable brush entry: row background
// with active / hover highlight, the brush's colored swatch box (with
// the sentinel hatch overlay when applicable), and a label string at
// the configurable text size. Replaces the open-coded rect+fill+
// border+hatch+label block that the palette list and the new-map
// floor picker each carried — adding a third call site (the custom-
// enemy modal's base-sprite picker) would have made it three.
func drawBrushSwatchRow(font rl.Font, r rl.Rectangle, label string, layer Layer, brush Brush, active, hovered bool, labelSize float32) {
	bg := bgEntry
	if active {
		bg = bgActive
	} else if hovered {
		bg = bgButtonHover
	}
	rl.DrawRectangleRec(r, bg)
	border := editorBorderDim
	if active {
		border = editorBorderActive
	}
	rl.DrawRectangleLinesEx(r, 1, border)

	swatch := rl.NewRectangle(r.X+6, r.Y+6, 20, r.Height-12)
	rl.DrawRectangleRec(swatch, brush.Color)
	sentinel := isSentinelBrush(layer, brush.Char)
	if sentinel {
		drawSentinelHatch(swatch)
	}
	rl.DrawRectangleLinesEx(swatch, 1, swatchEdge)

	nameCol := textEntry
	if sentinel {
		nameCol = rl.NewColor(190, 200, 220, 255)
	}
	rl.DrawTextEx(font, label,
		rl.NewVector2(r.X+34, r.Y+(r.Height-labelSize)/2),
		labelSize, 1, nameCol)
}

// isSentinelBrush reports whether (layer, char) is a "semantic" brush —
// Auto / Force-empty / None — that doesn't paint a visible tile. Used by
// the palette to render those swatches distinctly so the author doesn't
// confuse "let the renderer scatter" with "paint THIS particular look".
// Every authored layer must be enumerated below so a future ceiling
// sentinel (or another new layer) gets a real answer instead of falling
// through to false — silent "not a sentinel" would render the swatch
// without hatching and mislead the author about what the brush does.
func isSentinelBrush(layer Layer, char byte) bool {
	switch layer {
	case LayerFloor:
		return char == core.FloorAuto
	case LayerDecor:
		return char == core.DecorAuto || char == core.DecorEmpty
	case LayerProps:
		return char == core.TilePropEmpty
	case LayerWalls:
		return char == core.TileOpen
	case LayerCeiling:
		// Ceiling uses '.' for "no slab" — the same TileOpen char the
		// walls layer reads, but conceptually a sentinel (let sky
		// show through), not a paintable look.
		return char == core.TileCeilingOpen
	case LayerElevation:
		// Ground level ('0') is the implicit "flat" default; treat it as a
		// sentinel so it reads as the no-op level in the palette.
		return char == core.ElevationGround
	case LayerEntities:
		// Entities aren't tile chars — no sentinel concept applies.
		return false
	}
	panic(fmt.Sprintf("editor: isSentinelBrush missing case for layer %d", int(layer)))
}

// drawSentinelHatch overlays a diagonal stripe pattern onto a swatch
// rectangle so the swatch reads as "semantic" rather than literal. Five
// thin stripes at 45° catch the eye without obscuring the underlying
// color (which still hints at what auto-scatter would land on).
func drawSentinelHatch(r rl.Rectangle) {
	stripe := rl.NewColor(0, 0, 0, 110)
	steps := int(r.Width + r.Height)
	for i := 0; i < steps; i += 4 {
		x1 := r.X + float32(i)
		y1 := r.Y
		x2 := r.X
		y2 := r.Y + float32(i)
		// Clip to swatch bounds — rl.DrawLineEx doesn't itself clip but
		// the stripes that fall past the corners just get cropped at the
		// next round when raylib paints outside the swatch... so we
		// inline a quick clamp.
		if x1 > r.X+r.Width {
			y1 += x1 - (r.X + r.Width)
			x1 = r.X + r.Width
		}
		if y2 > r.Y+r.Height {
			x2 += y2 - (r.Y + r.Height)
			y2 = r.Y + r.Height
		}
		if x2 > r.X+r.Width || y1 > r.Y+r.Height {
			continue
		}
		rl.DrawLineEx(rl.NewVector2(x1, y1), rl.NewVector2(x2, y2), 1, stripe)
	}
}

// --- Metadata panel --------------------------------------------------------

type metaRect struct {
	nameLabel, nameField                 rl.Rectangle
	matLabel                             rl.Rectangle
	matButtons                           []rl.Rectangle
	quietLabel, quietField               rl.Rectangle
	dimsLabel                            rl.Rectangle
	widthValue, widthMinus, widthPlus    rl.Rectangle
	heightValue, heightMinus, heightPlus rl.Rectangle
	pathLabel, pathValue                 rl.Rectangle
	// reachLabel + reachArea bound the clickable reachability badge.
	// reachArea covers the badge fill region (not just the label) so
	// the metadata click handler can route any click in that zone to
	// the Validate modal. drawMetadata renders inside reachArea.
	reachLabel, reachArea rl.Rectangle
}

func metadataRects(s *State) metaRect {
	x := s.rect.metadata.X + 14
	w := s.rect.metadata.Width - 28
	y := s.rect.metadata.Y + headerReserve - s.metadataScroll

	r := metaRect{}

	r.nameLabel = rl.NewRectangle(x, y, w, 18)
	y += 22
	r.nameField = rl.NewRectangle(x, y, w, 30)
	y += 42

	r.matLabel = rl.NewRectangle(x, y, w, 18)
	y += 22
	r.matButtons = equalButtonRow(x, y, w, modalBtnH, len(core.MaterialOptions))
	y += 42

	r.quietLabel = rl.NewRectangle(x, y, w, 18)
	y += 22
	r.quietField = rl.NewRectangle(x, y, w, 30)
	y += 42

	r.dimsLabel = rl.NewRectangle(x, y, w, 18)
	y += 22
	r.widthValue, r.widthMinus, r.widthPlus = stepperRow(x, y, 96, 6)
	y += 38
	r.heightValue, r.heightMinus, r.heightPlus = stepperRow(x, y, 96, 6)
	y += 42

	// On-disk path readout. Player start tile + facing used to live
	// here, but those are properties of the PlayerStart entity instance
	// (and the per-door Facing on each DoorSpawn), not area-wide
	// settings — they're now edited from the right-click context menu
	// on the entities layer. Path stays in the sidebar because the
	// on-disk path IS area-wide.
	r.pathLabel = rl.NewRectangle(x, y, w, 18)
	r.pathValue = rl.NewRectangle(x, y+22, w, 30)
	// Reachability badge. Label sits a row below the path readout; the
	// clickable badge region extends past the label to cover the
	// "OK" / warning panel that follows underneath.
	reachY := y + 64
	r.reachLabel = rl.NewRectangle(x, reachY, w, 18)
	r.reachArea = rl.NewRectangle(x, reachY, w, 140)
	return r
}

// metadataContentHeight returns the pixel height required to render the
// full metadata panel (heading reserve + every laid-out row through the
// reachability badge). Used by ScrollMetadata to clamp the scroll offset
// so the bottom of the panel can scroll into view but no further.
func metadataContentHeight(s *State) float32 {
	// Recompute against an unscrolled metadataRects by temporarily
	// zeroing the scroll. Avoids hand-summing every stride above and
	// keeps this in lockstep with the actual layout.
	save := s.metadataScroll
	s.metadataScroll = 0
	mr := metadataRects(s)
	s.metadataScroll = save
	return mr.reachArea.Y + mr.reachArea.Height + 16 - s.rect.metadata.Y
}

// ScrollMetadata adjusts the metadata panel's vertical scroll offset by
// dy pixels (positive = scroll down). Clamps to [0, max] so the bottom
// of the content stays reachable but can't scroll past it. Mirrors
// ScrollPalette.
func ScrollMetadata(s *State, dy float32) {
	if s.rect.metadata.Height <= 0 {
		return
	}
	max := metadataContentHeight(s) - s.rect.metadata.Height
	if max < 0 {
		max = 0
	}
	s.metadataScroll += dy
	if s.metadataScroll < 0 {
		s.metadataScroll = 0
	}
	if s.metadataScroll > max {
		s.metadataScroll = max
	}
}

func handleMetadataClick(s *State, p rl.Vector2) bool {
	if !pointIn(p, s.rect.metadata) {
		return false
	}
	// Reject clicks landing inside the MAP heading band so a field
	// scrolled up behind the heading isn't activated by clicking the
	// header. Matches the scissor used in drawMetadata.
	if p.Y < s.rect.metadata.Y+headerReserve {
		return true
	}
	mr := metadataRects(s)
	// Reachability badge: clicking anywhere on the label + warning
	// list region opens the full validate modal. Keep this BEFORE the
	// field-focus checks so a click on the badge reliably opens
	// validate even if the badge happens to overlap a future field
	// added below.
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
			if s.area.Materials != core.MaterialOptions[i] {
				pushUndo(s)
				s.area.Materials = core.MaterialOptions[i]
				s.dirty = true
			}
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
	case focusCustomEnemyName:
		return customEnemyNameFieldRect(s)
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

func drawMetadata(s *State, font rl.Font, theme render.Theme) {
	rl.DrawRectangleRec(s.rect.metadata, bgPaletteCol)
	rl.DrawLineEx(
		rl.NewVector2(s.rect.metadata.X, s.rect.metadata.Y),
		rl.NewVector2(s.rect.metadata.X, s.rect.metadata.Y+s.rect.metadata.Height),
		1, outlineHard)

	render.DrawHeading(font, "MAP", int32(s.rect.metadata.X+12), int32(s.rect.metadata.Y+8), theme.BorderStrong)

	// Clamp scroll to current content bounds — content height varies with
	// the reachability badge's per-frame row count, so reclamp each frame.
	ScrollMetadata(s, 0)

	// Clip the metadata body so scrolled content can't paint into the MAP
	// heading band or below the panel. Mirrors the palette scissor.
	rl.BeginScissorMode(int32(s.rect.metadata.X), int32(s.rect.metadata.Y+headerReserve),
		int32(s.rect.metadata.Width), int32(s.rect.metadata.Height-headerReserve))
	defer rl.EndScissorMode()

	mr := metadataRects(s)

	drawLabel(font, "Name", mr.nameLabel)
	drawTextField(font, mr.nameField, s.area.Name, s.focus == focusName)

	drawLabel(font, "Materials", mr.matLabel)
	for i, br := range mr.matButtons {
		active := s.area.Materials == core.MaterialOptions[i]
		// MaterialName is total over MaterialOptions (which is the registry's
		// own enum list), so the ok=false branch is unreachable here — we
		// fall back to an empty string rather than panicking inside the
		// per-frame draw loop.
		name, _ := core.MaterialName(core.MaterialOptions[i])
		drawButton(font, br, name, active)
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

	// Player start coord + facing intentionally live on the PlayerStart
	// entity instance now, not in the area-wide sidebar. Right-click the
	// start tile on the Entities layer to edit; per-door facing on each
	// DoorSpawn overrides this fallback when the player arrives via a
	// door.

	// Path readout — readonly. Shows "(unsaved)" before the first save,
	// or the relative on-disk path once known.
	drawLabel(font, "On-disk path", mr.pathLabel)
	pathText := s.area.Path
	if pathText == "" {
		pathText = "(unsaved)"
	}
	drawReadonlyValue(font, mr.pathValue, pathText)

	// Reachability warnings badge: latches red whenever the area would
	// fail a save-time reachability check (unreachable packs, empty
	// rosters, packs that don't fit). Updates per-frame so the badge
	// reflects the current edit without waiting for a save.
	warnings := s.ReachabilityWarnings()
	drawLabel(font, "Reachability (click to validate)", mr.reachLabel)
	if len(warnings) == 0 {
		badgeValue := rl.NewRectangle(mr.reachArea.X, mr.reachArea.Y+22, mr.reachArea.Width, 30)
		rl.DrawRectangleRec(badgeValue, rl.NewColor(14, 22, 18, 255))
		rl.DrawRectangleLinesEx(badgeValue, 1, editorReachOK)
		rl.DrawTextEx(font, "OK", rl.NewVector2(badgeValue.X+8, badgeValue.Y+(badgeValue.Height-16)/2), editorFontBody, 1, rl.NewColor(150, 220, 180, 255))
	} else {
		// Stack one row per warning so the author can read them all
		// without hover/click. Red panel + outline so the badge pops
		// against the metadata column's neutral background.
		rows := warnings
		if len(rows) > 4 {
			rows = rows[:4] // cap so we don't reflow the panel
		}
		h := float32(10 + 22*len(rows))
		box := rl.NewRectangle(mr.reachArea.X, mr.reachArea.Y+22, mr.reachArea.Width, h)
		rl.DrawRectangleRec(box, rl.NewColor(38, 16, 18, 255))
		rl.DrawRectangleLinesEx(box, 1, editorReachWarn)
		for i, w := range rows {
			rl.DrawTextEx(font, "! "+w,
				rl.NewVector2(box.X+6, box.Y+5+float32(i)*22),
				editorFontLabel, 1, rl.NewColor(240, 180, 180, 255))
		}
		if len(warnings) > len(rows) {
			rl.DrawTextEx(font, fmt.Sprintf("(+%d more)", len(warnings)-len(rows)),
				rl.NewVector2(box.X+6, box.Y+h-18),
				13, 1, rl.NewColor(240, 180, 180, 220))
		}
	}
}

func drawLabel(font rl.Font, text string, r rl.Rectangle) {
	rl.DrawTextEx(font, text, rl.NewVector2(r.X, r.Y), editorFontLabel, 1, editorLabelColor)
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
	rl.DrawTextEx(font, display, rl.NewVector2(r.X+8, r.Y+(r.Height-16)/2), editorFontBody, 1, textEntry)
}

func drawReadonlyValue(font rl.Font, r rl.Rectangle, text string) {
	rl.DrawRectangleRec(r, bgFieldInset)
	rl.DrawRectangleLinesEx(r, 1, editorBorderInactive)
	rl.DrawTextEx(font, text, rl.NewVector2(r.X+8, r.Y+(r.Height-16)/2), editorFontBody, 1, textReadonly)
}

// --- Grid ------------------------------------------------------------------

// drawGrid paints the four flat-color grid layers (floor → walls → decor →
// props) stacked, then the ceiling hash overlay and the entity overlays
// (start + spawns). Layers other than the active one are dimmed so the focus
// is on what the next click will affect. Order: floor → walls → decor →
// props → ceiling (hash) → entities.
func drawGrid(s *State, font rl.Font) {
	rl.DrawRectangleRec(s.rect.grid, bgFieldInset)
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

	// Frustum-cull tiles outside the visible grid panel. A 200×200
	// map would be 40k iterations × ~6 raylib draws per cell without
	// this — fine at small maps but a frame-rate cliff at MaxMapDimension.
	// Compute the [zMin, zMax) × [xMin, xMax) window from gridX/gridY +
	// cellPx and the panel's screen rect, then iterate only cells whose
	// projected rect intersects the panel.
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
	// inCullWindow reports whether a tile is inside the visible grid window —
	// the shared test the entity-marker loops use to skip off-screen spawns.
	inCullWindow := func(tx, tz int) bool {
		return tx >= xMin && tx < xMax && tz >= zMin && tz < zMax
	}

	// Active-layer char overlay: at zooms where a glyph fits, paint the
	// tile-char of the CURRENTLY SELECTED layer on each cell so the author
	// can read what's authored on the layer they're editing without every
	// other layer's chars competing for attention (which read as noise).
	// Off by default; ALT-tap toggles it (see showTileGlyphs). Empty
	// sentinels produce no glyph. Threshold matches the axis-tick label
	// threshold so the chrome turns on together.
	const charOverlayMinCell = float32(14)
	showCharOverlay := cell >= charOverlayMinCell && s.showTileGlyphs && !s.layerHidden[s.layer]
	charFontSize := cell * 0.55
	charShadow := glyphShadow
	charFG := rl.NewColor(248, 250, 252, 235)

	// Per-layer visibility (the layer-tab eye toggles): hidden layers are
	// skipped in the cell + marker draws so the author can isolate what they're
	// working on. Hoisted out of the inner loop so it's a cheap bool per cell.
	showFloor := !s.layerHidden[LayerFloor]
	showWalls := !s.layerHidden[LayerWalls]
	showDecor := !s.layerHidden[LayerDecor]
	showProps := !s.layerHidden[LayerProps]
	showCeiling := !s.layerHidden[LayerCeiling]
	showElevation := !s.layerHidden[LayerElevation]
	showEntities := !s.layerHidden[LayerEntities]

	for z := zMin; z < zMax; z++ {
		for x := xMin; x < xMax; x++ {
			r := s.rect.tileRect(x, z)
			// Floor is the base — always painted (except under a wall, where
			// the wall covers it).
			if showFloor {
				rl.DrawRectangleRec(r, fadeAlpha(floorColor(s.area.Floor[z][x]), floorAlpha))
			}
			if showWalls && s.area.Walls[z][x] == core.TileRock {
				rl.DrawRectangleRec(r, fadeAlpha(wallColor(), wallAlpha))
			}
			if d := s.area.Decor[z][x]; showDecor && d != core.DecorAuto {
				rl.DrawRectangleRec(insetRect(r, cell*0.28), fadeAlpha(decorColor(d), decorAlpha))
			}
			if p := s.area.Props[z][x]; showProps && core.IsPropChar(p) {
				rl.DrawCircle(int32(r.X+cell/2), int32(r.Y+cell/2), cell*0.36, fadeAlpha(propColor(p), propAlpha))
			}
			// Ceiling hash overlay: shown only when the Ceiling layer is
			// active or the cell holds a ceiling. Two diagonal stripes
			// inside the cell so it reads as "covered" without obscuring
			// the layer underneath.
			if showCeiling && s.area.CeilingAt(x, z) {
				drawCeilingHash(r, cell, fadeAlpha(ceilingColor(), ceilingAlpha))
			}
			if showCharOverlay {
				if ch, ok := currentLayerGlyph(s, x, z); ok {
					drawTileGlyph(font, r, cell, charFontSize, ch, charFG, charShadow)
				}
			}
			// Height slice-view: overlay each cell's level (tint + digit) and
			// connective ramp arrows so the heightmap is legible in the flat
			// editor grid. Shown while the Elevation layer is active OR while
			// the Floors lens is on (then it rides on top of every layer as a
			// true overlay — the active floor crisp, others ghosted).
			if showElevation && (s.levelFocus || s.layer == LayerElevation) {
				drawElevationSlice(s, font, r, cell, x, z)
			}
		}
	}

	// Grid lines. Every 5 cells draws a slightly darker line (gridLineMajor)
	// so the author can eyeball coordinates at a glance — matches the
	// "tick every 5" convention common in tile editors. Same cull
	// window as the cell loop so a big map doesn't draw lines outside
	// the visible panel.
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
		if x%5 == 0 {
			col = gridLineMajor
		}
		rl.DrawLineEx(rl.NewVector2(px, s.rect.gridY), rl.NewVector2(px, s.rect.gridY+s.rect.gridH), 1, col)
	}
	for z := zMin; z < lineZMax; z++ {
		py := s.rect.gridY + float32(z)*cell
		col := gridLineCol
		if z%5 == 0 {
			col = gridLineMajor
		}
		rl.DrawLineEx(rl.NewVector2(s.rect.gridX, py), rl.NewVector2(s.rect.gridX+s.rect.gridW, py), 1, col)
	}

	// Axis tick labels every 5 cells. Only at zoom levels where cells are
	// big enough to comfortably fit a tick digit — at very small zooms the
	// labels would overlap and read as visual noise.
	if cell >= 18 {
		tickCol := rl.NewColor(220, 224, 232, 180)
		// Top axis: column numbers.
		for x := (xMin / 5) * 5; x < lineXMax; x += 5 {
			label := tickLabel(x)
			m := rl.MeasureTextEx(font, label, editorFontTick, 1)
			px := s.rect.gridX + float32(x)*cell - m.X/2
			py := s.rect.gridY - m.Y - 2
			if py < s.rect.grid.Y+2 {
				continue
			}
			rl.DrawTextEx(font, label, rl.NewVector2(px, py), editorFontTick, 1, tickCol)
		}
		// Left axis: row numbers.
		for z := (zMin / 5) * 5; z < lineZMax; z += 5 {
			label := tickLabel(z)
			m := rl.MeasureTextEx(font, label, editorFontTick, 1)
			px := s.rect.gridX - m.X - 4
			py := s.rect.gridY + float32(z)*cell - m.Y/2
			if px < s.rect.grid.X+2 {
				continue
			}
			rl.DrawTextEx(font, label, rl.NewVector2(px, py), editorFontTick, 1, tickCol)
		}
	}

	// Pack markers. Each pack draws one circle tinted by the leader's
	// brush color (from entityBrushColors so editor swatch and field marker
	// match) plus an initial letter from the leader's SingularName so the
	// author can tell Rat from Goblin from Mantrap at a glance. The "xN"
	// badge shows the pack size — matching the field-render contract that
	// the player only sees the leader from afar.
	for _, sp := range s.area.PackSpawns {
		if len(sp.Members) == 0 {
			continue
		}
		// Cull spawns outside the visible grid window (same window the tile
		// loop uses) so a big map with many packs doesn't pay leader-lookup +
		// string-format + MeasureTextEx for markers panned off-screen.
		if !showEntities || !inCullWindow(sp.TileX, sp.TileZ) {
			continue
		}
		cx, cy := s.rect.tileCenter(sp.TileX, sp.TileZ)
		leaderSlot := core.PackSpawnLeaderSlot(s.area, sp)
		leader := packSpawnLeaderKind(s.area, sp)
		col := fadeAlpha(packMarkerColor(leader), entityAlpha)
		rl.DrawCircle(int32(cx), int32(cy), cell*0.32, col)
		rl.DrawCircleLines(int32(cx), int32(cy), cell*0.32, fadeAlpha(entityMarkerOutline, entityAlpha))
		label := packMarkerInitial(core.PackMemberDisplayName(s.area, sp, leaderSlot))
		measure := rl.MeasureTextEx(font, label, cell*0.42, 1)
		rl.DrawTextEx(font, label,
			rl.NewVector2(cx-measure.X/2, cy-measure.Y/2),
			cell*0.42, 1, fadeAlpha(entityMarkerOutline, entityAlpha))
		if len(sp.Members) > 1 {
			badge := fmt.Sprintf("x%d", len(sp.Members))
			bsize := cell * 0.28
			bm := rl.MeasureTextEx(font, badge, bsize, 1)
			bx := cx + cell*0.18
			by := cy - cell*0.42
			rl.DrawRectangleRounded(
				rl.NewRectangle(bx-2, by-1, bm.X+6, bm.Y+2),
				0.4, 4,
				fadeAlpha(rl.NewColor(20, 20, 24, 230), entityAlpha))
			rl.DrawTextEx(font, badge,
				rl.NewVector2(bx+1, by),
				bsize, 1, fadeAlpha(rl.NewColor(240, 240, 240, 255), entityAlpha))
		}
	}

	// Chest markers — a small filled square inset into the tile, distinct
	// from the round enemy-pack circles so the author can tell at a
	// glance which entity sits where.
	for _, c := range s.area.ChestSpawns {
		if !showEntities || !inCullWindow(c.TileX, c.TileZ) {
			continue
		}
		gx, gy := s.rect.tileCorner(c.TileX, c.TileZ)
		inset := cell * 0.25
		rl.DrawRectangleRec(
			rl.NewRectangle(gx+inset, gy+inset, cell-2*inset, cell-2*inset),
			fadeAlpha(render.MarkerChest, entityAlpha))
		rl.DrawRectangleLinesEx(
			rl.NewRectangle(gx+inset, gy+inset, cell-2*inset, cell-2*inset),
			1, fadeAlpha(entityMarkerOutline, entityAlpha))
	}

	// Door markers — a tall thin rectangle in the warm wood tone, with a
	// small arrowhead indicating the post-transition facing so the
	// author can verify pairing at a glance. Distinct silhouette from
	// chests (door = tall + arrow, chest = small square + lid).
	for _, d := range s.area.DoorSpawns {
		if !showEntities || !inCullWindow(d.TileX, d.TileZ) {
			continue
		}
		gx, gy := s.rect.tileCorner(d.TileX, d.TileZ)
		insetX := cell * 0.30
		insetY := cell * 0.12
		rl.DrawRectangleRec(
			rl.NewRectangle(gx+insetX, gy+insetY, cell-2*insetX, cell-2*insetY),
			fadeAlpha(render.MarkerDoor, entityAlpha))
		rl.DrawRectangleLinesEx(
			rl.NewRectangle(gx+insetX, gy+insetY, cell-2*insetX, cell-2*insetY),
			1, fadeAlpha(entityMarkerOutline, entityAlpha))
		// Facing arrow inside the door rectangle.
		cx := gx + cell*0.5
		cy := gy + cell*0.5
		dx, dz := core.FacingVector(d.Facing)
		tipX := cx + float32(dx)*cell*0.28
		tipY := cy + float32(dz)*cell*0.28
		rl.DrawLineEx(rl.NewVector2(cx, cy), rl.NewVector2(tipX, tipY), 2, fadeAlpha(rl.NewColor(40, 24, 12, 255), entityAlpha))
	}

	// Player start marker (part of the Entities layer, so it hides with it).
	if showEntities {
		sx, sy := s.rect.tileCenter(s.area.StartTileX, s.area.StartTileZ)
		startCol := fadeAlpha(render.MarkerStart, entityAlpha)
		rl.DrawCircle(int32(sx), int32(sy), cell*0.36, startCol)
		rl.DrawCircleLines(int32(sx), int32(sy), cell*0.36, fadeAlpha(entityMarkerOutline, entityAlpha))
		dx, dz := core.FacingVector(s.area.StartFacing)
		tx := sx + float32(dx)*cell*0.42
		ty := sy + float32(dz)*cell*0.42
		rl.DrawLineEx(rl.NewVector2(sx, sy), rl.NewVector2(tx, ty), 3, fadeAlpha(rl.NewColor(20, 14, 0, 255), entityAlpha))
	}

	// Door-link overlay: when toggled on, draw a connector from each door to
	// its same-map target door, and ring doors whose target_door doesn't
	// resolve. Drawn above markers so the links read clearly. Independent of
	// the entities-layer hide (it's its own diagnostic toggle).
	if s.showDoorLinks {
		drawDoorLinks(s, cell)
	}

	// Brush ghost / hover highlight.
	hoverPx := s.hoverX
	hoverPz := s.hoverZ
	if s.gridCursorX >= 0 {
		hoverPx, hoverPz = s.gridCursorX, s.gridCursorZ
	}
	if hoverPx >= 0 {
		// Multi-tile brush footprint preview: when the active brush is a
		// J/A anchor, outline EVERY footprint cell tinted by whether it
		// can actually be placed there. Green = clear, red = blocked.
		// This way the author sees the full 2×2 / 1×2 shape before clicking
		// and never lands on a half-painted footprint.
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
		x0, x1 := s.rectAnchorX, s.hoverX
		z0, z1 := s.rectAnchorZ, s.hoverZ
		if x0 > x1 {
			x0, x1 = x1, x0
		}
		if z0 > z1 {
			z0, z1 = z1, z0
		}
		cx, cy := s.rect.tileCorner(x0, z0)
		r := rl.NewRectangle(cx, cy, float32(x1-x0+1)*cell, float32(z1-z0+1)*cell)
		// Box tool previews as an outline only; Rect tool fills.
		if !s.rectHollow {
			rl.DrawRectangleRec(r, withAlpha(brushPreviewColor(s), 110))
		}
		rl.DrawRectangleLinesEx(r, 2, selectionOutline)
	}

	// Line drag preview — a segment from the anchor tile to the hovered tile.
	if s.drag == dragLine && s.hoverX >= 0 {
		ax, ay := s.rect.tileCenter(s.rectAnchorX, s.rectAnchorZ)
		hx, hy := s.rect.tileCenter(s.hoverX, s.hoverZ)
		rl.DrawLineEx(rl.NewVector2(ax, ay), rl.NewVector2(hx, hy), 3, selectionOutline)
	}

	if s.drag == dragStart && s.hoverX >= 0 {
		gx, gy := s.rect.tileCenter(s.hoverX, s.hoverZ)
		ghost := withAlpha(render.MarkerStart, 220)
		rl.DrawCircleLines(int32(gx), int32(gy), cell*0.36, ghost)
	}
	if s.drag == dragPack && s.hoverX >= 0 && s.dragPackIdx >= 0 && s.dragPackIdx < len(s.area.PackSpawns) {
		gx, gy := s.rect.tileCenter(s.hoverX, s.hoverZ)
		rl.DrawCircleLines(int32(gx), int32(gy), cell*0.32, selectionOutline)
	}
	// Chest / door drag-move ghosts: a square outline at the hovered
	// destination tile so the relocation reads before release (mirrors the
	// pack circle).
	if (s.drag == dragChest || s.drag == dragDoor) && s.hoverX >= 0 {
		gx, gy := s.rect.tileCorner(s.hoverX, s.hoverZ)
		inset := cell * 0.22
		rl.DrawRectangleLinesEx(rl.NewRectangle(gx+inset, gy+inset, cell-2*inset, cell-2*inset), 2, selectionOutline)
	}

	// Rich hover tooltip: when the cursor is over a tile that holds a
	// pack / chest / door / start, render a small card near the mouse
	// listing what's inside. Layer labels alone (in the topbar) don't
	// say which enemies are in a pack or which items in a chest — that
	// information was modal-only before this card.
	if s.hoverX >= 0 && s.drag == dragNone {
		drawHoverTooltip(s, font)
	}
}

// drawHoverTooltip paints a small panel near the mouse with the entity
// contents at (hoverX, hoverZ). No-op when the tile is empty so cursor
// noise doesn't follow the mouse across blank floor.
// Hover-tooltip memo: the contents + measured width only change when the map
// mutates (contentEpoch) or the cursor moves to a different tile, so we don't
// rebuild the line slice (which allocates maps + slices) and re-measure every
// frame the cursor merely rests on an entity tile.
var (
	tooltipKeyEpoch uint64
	tooltipKeyX     int = -1
	tooltipKeyZ     int = -1
	tooltipReady    bool
	tooltipLines    []string
	tooltipWidth    float32
)

func drawHoverTooltip(s *State, font rl.Font) {
	x, z := s.hoverX, s.hoverZ
	if !tooltipReady || tooltipKeyEpoch != s.contentEpoch || tooltipKeyX != x || tooltipKeyZ != z {
		tooltipLines = tooltipLinesFor(s, x, z)
		tooltipWidth = 0
		for _, l := range tooltipLines {
			m := rl.MeasureTextEx(font, l, 11, 1)
			if m.X > tooltipWidth {
				tooltipWidth = m.X
			}
		}
		tooltipKeyEpoch = s.contentEpoch
		tooltipKeyX, tooltipKeyZ = x, z
		tooltipReady = true
	}
	lines := tooltipLines
	if len(lines) == 0 {
		return
	}
	const padding = float32(6)
	const lineH = float32(14)
	w := tooltipWidth + padding*2
	h := float32(len(lines))*lineH + padding*2
	mp := frameMouse
	tx := mp.X + 14
	ty := mp.Y + 14
	if tx+w > s.rect.grid.X+s.rect.grid.Width {
		tx = mp.X - w - 8
	}
	if ty+h > s.rect.grid.Y+s.rect.grid.Height {
		ty = mp.Y - h - 8
	}
	r := rl.NewRectangle(tx, ty, w, h)
	rl.DrawRectangleRec(r, tooltipBG)
	rl.DrawRectangleLinesEx(r, 1, editorBorderActive)
	for i, l := range lines {
		col := tooltipText
		if i == 0 {
			col = tooltipHeading
		}
		rl.DrawTextEx(font, l,
			rl.NewVector2(r.X+padding, r.Y+padding+float32(i)*lineH),
			11, 1, col)
	}
}

// tooltipLinesFor builds the hover tooltip body for tile (x, z). Returns
// nil when nothing interesting sits there — the caller short-circuits in
// that case.
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
				name := core.PackMemberDisplayName(s.area, sp, i)
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
	// If nothing more than the coord line is present, skip the tooltip —
	// noise on blank floor.
	if len(out) <= 1 {
		return nil
	}
	return out
}

// packMarkerColor returns the grid color used for a pack's marker on
// the canvas — leader's entityBrushColors entry if known, else the
// neutral grey fallback. Keeps the editor swatch column and the
// in-grid marker color in lockstep.
func packMarkerColor(kind core.EnemyKind) rl.Color {
	if col, ok := entityBrushColors[kind]; ok {
		return col
	}
	return entityFallbackColor
}

// packMarkerInitial returns the single uppercase letter drawn at the
// center of a pack's marker. Sources from EnemyKindName so it stays in
// sync with the canonical short name. Strips a "diseased_" / "venus_"
// prefix when picking the letter so "Diseased Rat" reads as "D" rather
// than colliding with a future "Demon."
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

// layerAlpha returns the per-layer rendering opacity given the active
// layer. Active layer is fully visible; others are dimmed to ~55%.
func layerAlpha(s *State, l Layer) float32 {
	if s.layer == l {
		return 1
	}
	return 0.55
}

// fadeAlpha scales c's existing alpha by the 0..1 multiplier (clamped).
// Thin alias over render.FadeColor so the multiply-and-clamp lives once in
// render/theme.go — the editor canvas fades brush / marker colors through
// the same helper the HUD uses (rl.Color is a color.RGBA alias).
func fadeAlpha(c rl.Color, alpha float32) rl.Color {
	return render.FadeColor(c, alpha)
}

func insetRect(r rl.Rectangle, inset float32) rl.Rectangle {
	return rl.NewRectangle(r.X+inset, r.Y+inset, r.Width-2*inset, r.Height-2*inset)
}

// brushPreviewColor returns a representative tint for the active brush so
// the rectangle drag preview hints at what's about to be painted. Covers
// every Layer the editor exposes — silent grey fallback for a missing
// case mismatches the palette swatch and confuses the drag preview, so
// new layers must register a real preview tint here.
func brushPreviewColor(s *State) rl.Color {
	b := s.activeBrush()
	switch s.layer {
	case LayerWalls:
		if b.Char == core.TileRock {
			return wallColor()
		}
		return floorColor(core.FloorAuto)
	case LayerFloor:
		return floorColor(b.Char)
	case LayerDecor:
		if b.Char == core.DecorAuto {
			return rl.NewColor(180, 168, 140, 255)
		}
		return decorColor(b.Char)
	case LayerProps:
		if core.IsPropChar(b.Char) {
			return propColor(b.Char)
		}
		return floorColor(core.FloorAuto)
	case LayerCeiling:
		return ceilingColor()
	case LayerElevation:
		// Tint the drag-rect preview by the selected level so a region paint
		// reads as "this height."
		return elevationLevelColor(s.editLevel)
	case LayerEntities:
		// Entity layer paints a marker, not a tile char — use the
		// fallback so the drag-rect preview reads as neutral. The
		// entity layer doesn't currently support rect-drag painting
		// anyway; this is for the future.
		return editorFallbackColor
	}
	panic(fmt.Sprintf("editor: brushPreviewColor missing case for layer %d", int(s.layer)))
}

// tileColorByChar is the per-layer, per-tile-char swatch color for the editor
// grid, flattened to a [layerCount][256] array (was a map[Layer]map[byte]) so
// the per-cell lookup in drawGrid is a single indexed read instead of two map
// hashes — the grid repaints every visible cell every frame. Built once at
// init from layerBrushes (editor.go) so the palette UI and the grid preview
// can't drift on color; adding a tile char stays one row in layerBrushes.
// Each layer's row is pre-filled with that layer's fallback (tileColorFallback),
// then palette chars overwrite — so an unrecognized char reads as the fallback
// with no per-cell branch.
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

// tileColorFallback is the swatch color used when a layer holds a char
// that isn't in the brush palette (corrupt save, future char dropped from
// the palette). Floor/decor share a desaturated tan; props use neutral
// grey to read as "unrecognized prop."
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
	// Fallback is pre-baked into every row, so this single index covers both
	// palette chars and unrecognized ones — no map hash, no branch.
	return tileColorByChar[layer][c]
}

// elevationLevelColor maps an elevation level to an earthy swatch (dark low →
// light high) for the brush preview + the height-selector label.
func elevationLevelColor(level int) rl.Color {
	level = clampLevel(level)
	t := float32(level) / float32(maxEditLevel)
	return rl.NewColor(uint8(92+t*120), uint8(72+t*108), uint8(56+t*66), 255)
}

// drawElevationSlice overlays the height "slice view" on a cell while the
// Elevation layer is active: a translucent tint keyed to the cell's level
// relative to the selected editLevel (the current level pops, lower levels go
// cool, higher go warm), the level digit, and — on ramp tiles — the ramp arrow
// highlighted as a connector.
func drawElevationSlice(s *State, font rl.Font, r rl.Rectangle, cell float32, x, z int) {
	lvl := s.area.ElevationLevelAt(x, z)
	rl.DrawRectangleRec(r, elevationSliceTint(lvl, s.editLevel))
	shadow := glyphShadow
	if facing, ok := s.area.RampAt(x, z); ok {
		rl.DrawRectangleLinesEx(r, 2, rl.NewColor(120, 230, 140, 220))
		drawTileGlyph(font, r, cell, cell*0.62, core.RampCharForFacing(facing), rl.NewColor(150, 240, 165, 245), shadow)
		return
	}
	if cell >= 12 {
		drawTileGlyph(font, r, cell, cell*0.5, core.ElevationChar(lvl), rl.NewColor(245, 245, 250, 230), shadow)
	}
}

// elevationSliceTint tints a cell by its level relative to the selected edit
// level so the heightmap reads at a glance in the flat grid: selected level a
// soft green wash, lower levels cool blue (deeper = denser), higher warm orange.
func elevationSliceTint(level, sel int) rl.Color {
	switch {
	case level == sel:
		return rl.NewColor(120, 200, 130, 60)
	case level < sel:
		a := 60 + (sel-level)*30
		if a > 170 {
			a = 170
		}
		return rl.NewColor(40, 70, 150, uint8(a))
	default:
		a := 50 + (level-sel)*28
		if a > 160 {
			a = 160
		}
		return rl.NewColor(210, 130, 50, uint8(a))
	}
}

func wallColor() color.RGBA        { return tileColor(LayerWalls, core.TileRock) }
func floorColor(c byte) color.RGBA { return tileColor(LayerFloor, c) }
func decorColor(c byte) color.RGBA { return tileColor(LayerDecor, c) }
func propColor(c byte) color.RGBA  { return tileColor(LayerProps, c) }
func ceilingColor() color.RGBA     { return tileColor(LayerCeiling, core.TileCeilingSolid) }

// drawCeilingHash paints two diagonal stripes inside the tile so a
// ceiling-flagged cell reads as "roofed" without fully hiding the
// floor/wall/prop underneath. Width is a fixed fraction of cell so the
// stripe stays visible at small zoom levels.
func drawCeilingHash(r rl.Rectangle, cell float32, col color.RGBA) {
	t := cell * 0.10
	// Top-left → bottom-right and top-right → bottom-left diagonals.
	rl.DrawLineEx(rl.NewVector2(r.X, r.Y), rl.NewVector2(r.X+cell, r.Y+cell), t, col)
	rl.DrawLineEx(rl.NewVector2(r.X+cell, r.Y), rl.NewVector2(r.X, r.Y+cell), t, col)
}

// currentLayerGlyph returns the tile char to overlay for the ACTIVE
// layer's cell at (x, z), or ok==false when the cell is empty (or the
// active layer carries no per-tile chars — Entities, whose content is
// drawn as markers). Showing only the active layer keeps the overlay
// readable; the old "topmost char across every layer" version was too
// noisy. Empty sentinels (non-rock walls / FloorAuto / DecorAuto /
// DecorEmpty / no-prop / no-ceiling) yield ok==false so blank cells
// stay blank instead of dotting the grid.
func currentLayerGlyph(s *State, x, z int) (byte, bool) {
	switch s.layer {
	case LayerWalls:
		if w := s.area.Walls[z][x]; w == core.TileRock {
			return w, true
		}
	case LayerFloor:
		if f := s.area.Floor[z][x]; f != core.FloorAuto && f != 0 {
			return f, true
		}
	case LayerDecor:
		if d := s.area.Decor[z][x]; d != core.DecorAuto && d != core.DecorEmpty {
			return d, true
		}
	case LayerProps:
		if p := s.area.Props[z][x]; core.IsPropChar(p) {
			return p, true
		}
	case LayerCeiling:
		if s.area.CeilingAt(x, z) {
			return s.area.Ceiling[z][x], true
		}
	}
	return 0, false
}

// drawTileGlyph paints a single-character glyph centered in `r` with a
// 1px-offset drop shadow for legibility against any cell color. Font
// size is sized to the cell so the glyph scales with zoom; shadow stays
// 1px so it doesn't blur out at small zooms.
func drawTileGlyph(font rl.Font, r rl.Rectangle, cell, fontSize float32, ch byte, fg, shadow rl.Color) {
	text := glyphStr[ch]
	m := rl.MeasureTextEx(font, text, fontSize, 1)
	px := r.X + (cell-m.X)/2
	py := r.Y + (cell-m.Y)/2
	rl.DrawTextEx(font, text, rl.NewVector2(px+1, py+1), fontSize, 1, shadow)
	rl.DrawTextEx(font, text, rl.NewVector2(px, py), fontSize, 1, fg)
}

// scrollWindow clamps a cursor-in-range to a visible window of size
// `rowsVisible` over `total` entries. Returns the [top, end) slice
// bounds that keep cursor in view: end == top+rowsVisible when there's
// enough content; both are 0 when the list is empty.
//
// Used by the Open modal's path list today; lifted out as a shared
// helper so the pack-edit member list and the sound modal's saved-
// sound list can plug into the same scroll semantics when they grow
// past their visible windows.
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

// --- Status & modals -------------------------------------------------------

func drawStatus(s *State, font rl.Font, theme render.Theme) {
	const lineH = 22
	const pad = 12
	maxW := float32(0)
	for _, e := range s.statusLog {
		m := rl.MeasureTextEx(font, e.msg, editorFontLabel, 1)
		if m.X > maxW {
			maxW = m.X
		}
	}
	rH := float32(len(s.statusLog))*lineH + pad
	r := rl.NewRectangle(s.rect.grid.X+12, s.rect.grid.Y+s.rect.grid.Height-rH-8, maxW+24, rH)
	render.DrawCard(int32(r.X), int32(r.Y), int32(r.Width), int32(r.Height),
		theme.SurfacePrimary, theme.BorderSoft, theme.BorderStrong)
	for i, e := range s.statusLog {
		y := r.Y + pad/2 + float32(i)*lineH
		alpha := e.timer / statusLogLifetime
		if alpha < 0 {
			alpha = 0
		}
		if alpha > 1 {
			alpha = 1
		}
		col := theme.TextPrimary
		if e.warn {
			col = theme.BorderDanger
		}
		col.A = uint8(float32(col.A) * (0.4 + 0.6*alpha))
		render.DrawTextWithShadow(font, e.msg, r.X+12, y, editorFontLabel, col)
	}
}

func drawModalVeil(theme render.Theme) {
	w, h := render.ScreenSize()
	rl.DrawRectangle(0, 0, w, h, theme.SurfaceVeil)
}

// centeredCardRect returns the screen-centered rect for a modal card of
// the given size. The (screen − card)/2 centering math lived in a
// half-dozen modal layout helpers (each of which needs the rect before
// drawing, for hit-testing); one helper keeps them from drifting and
// recenters cleanly on window resize.
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

// drawModalHeader is the standard modal opening: veil the screen, paint
// the card, and stamp a heading at the top-left inset. Returns the card
// rect so the caller can lay out body content. Replaces the three-call
// `drawModalVeil → drawModalCard → render.DrawHeading` boilerplate that
// every modal opened with; keeping the trio atomic means a missing veil
// or a drifted heading offset can't slip in modal-by-modal.
func drawModalHeader(font rl.Font, theme render.Theme, pw, ph float32, title string, accent rl.Color) rl.Rectangle {
	r := centeredCardRect(pw, ph)
	drawModalHeaderAt(font, theme, r, title, accent)
	return r
}

// drawModalHeaderAt is drawModalHeader for callers that already computed
// the card rect — the custom-layout modals (door / new-map / custom-
// enemies) that need the rect for hit-testing before drawing. Same
// veil → card → heading trio, just with a caller-supplied rect instead
// of a freshly-centered one, so those modals can't open-code (and drift
// on) the trio.
func drawModalHeaderAt(font rl.Font, theme render.Theme, card rl.Rectangle, title string, accent rl.Color) {
	drawModalVeil(theme)
	render.DrawCard(int32(card.X), int32(card.Y), int32(card.Width), int32(card.Height),
		theme.SurfacePrimary, theme.BorderSoft, accent)
	render.DrawHeading(font, title, int32(card.X+modalContentInset), int32(card.Y+12), accent)
}

// openModalListGeom returns the open-map list geometry — the card, the
// first visible row's Y, the row height, and the visible [topRow, end)
// window — shared by drawOpenModal and openModalRowAt so the painted rows
// and the click hit-rects line up.
func openModalListGeom(s *State) (card rl.Rectangle, listTop, rowH float32, topRow, end int) {
	card = centeredCardRect(openModalW, openModalH)
	rowH = 22
	listTop = card.Y + 50
	listBottom := card.Y + card.Height - 52 // room for the action button row
	rowsVisible := int((listBottom - listTop) / rowH)
	if rowsVisible < 1 {
		rowsVisible = 1
	}
	topRow, end = scrollWindow(s.modalCursor, len(s.modalPaths), rowsVisible)
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
	if s.modalRenaming != "" {
		header = "RENAME MAP"
	} else if s.modalConfirmDelete {
		header = "DELETE MAP"
	}
	r := drawModalHeader(font, theme, openModalW, openModalH, header, theme.BorderStrong)

	if len(s.modalPaths) == 0 {
		rl.DrawTextEx(font, "(no .map files in maps/)", rl.NewVector2(r.X+modalContentInset, r.Y+50), editorFontLabel, 1, theme.TextMuted)
		rl.DrawTextEx(font, "Esc / click outside to close", rl.NewVector2(r.X+modalContentInset, r.Y+r.Height-26), editorFontHint, 1, theme.TextHint)
		return
	}

	_, listTop, rowH, topRow, end := openModalListGeom(s)
	for i := topRow; i < end; i++ {
		path := s.modalPaths[i]
		text := core.MapIDFromPath(path)
		col := theme.TextMuted
		if i == s.modalCursor {
			col = theme.BorderActive
			text = "> " + text
		}
		render.DrawTextWithShadow(font, text, r.X+18, listTop+float32(i-topRow)*rowH, editorFontBody, col)
	}
	// Scroll hint when the list extends past the visible window.
	if topRow > 0 || end < len(s.modalPaths) {
		more := fmt.Sprintf("(%d / %d)", s.modalCursor+1, len(s.modalPaths))
		measure := rl.MeasureTextEx(font, more, editorFontHint, 1)
		rl.DrawTextEx(font, more,
			rl.NewVector2(r.X+r.Width-measure.X-16, r.Y+30),
			editorFontHint, 1, theme.TextHint)
	}

	if s.modalRenaming != "" {
		fieldR := rl.NewRectangle(r.X+modalContentInset, r.Y+r.Height-86, r.Width-32, 28)
		drawTextField(font, fieldR, s.modalRenaming, true)
		labels := cmdLabels(openRenameCmds(s))
		drawModalButtons(font, modalButtonRow(r, labels), labels)
		return
	}
	if s.modalConfirmDelete {
		path := s.modalPaths[s.modalCursor]
		rl.DrawTextEx(font, fmt.Sprintf("Delete %s? This is permanent.", core.MapIDFromPath(path)),
			rl.NewVector2(r.X+modalContentInset, r.Y+r.Height-86), editorFontLabel, 1, theme.BorderDanger)
		labels := cmdLabels(openDeleteConfirmCmds(s))
		drawModalButtons(font, modalButtonRow(r, labels), labels)
		return
	}

	// Main view: click a row to select, then an action button (or the
	// keyboard accelerators) to act.
	labels := cmdLabels(openModalActionCmds(s))
	drawModalButtons(font, modalButtonRow(r, labels), labels)
}

// Modal card dimensions. saveAs is named because its field-rect helper
// and draw call BOTH need the exact size (they must stay in lockstep, or
// the input field lands off the card); the rest are named so the modal
// sizes live with the other chrome constants instead of as bare literals
// scattered across the draw functions. The Validate modal sizes its
// height dynamically from the row count, so only its width is a const.
const (
	saveAsModalW = float32(420)
	saveAsModalH = float32(160)

	doorEditModalW     = float32(480)
	doorEditModalH     = float32(424)
	openModalW         = float32(460)
	openModalH         = float32(460)
	entityEditModalW   = float32(480) // pack-edit + chest-edit share this size
	entityEditModalH   = float32(440)
	escMenuModalW      = float32(380)
	escMenuModalH      = float32(178)
	confirmDirtyModalW = float32(460)
	confirmDirtyModalH = float32(212) // tall enough that the contextual hint clears the bottom-anchored button stack
	validateModalW     = float32(560)
)

func saveAsFieldRect(s *State) rl.Rectangle {
	r := centeredCardRect(saveAsModalW, saveAsModalH)
	return rl.NewRectangle(r.X+modalContentInset, r.Y+58, saveAsModalW-32, 28)
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
		rl.DrawTextEx(font, fmt.Sprintf("Overwrite %s?", core.MapPath(s.modalFilename)),
			rl.NewVector2(r.X+modalContentInset, r.Y+44), editorFontLabel, 1, theme.TextPrimary)
		labels := cmdLabels(saveAsOverwriteCmds(s))
		drawModalButtons(font, modalButtonStack(r, labels), labels)
		return
	}

	rl.DrawTextEx(font, "Filename (without .map):", rl.NewVector2(r.X+modalContentInset, r.Y+40), editorFontHint, 1, theme.TextLabel)

	field := saveAsFieldRect(s)
	drawTextField(font, field, s.modalFilename, true)
	// Preview the sanitized path: MapPath strips a trailing .map, and the
	// disk store goes through sanitizeFilename on commit. Show the
	// final-form path so the user knows what they'll actually get — and
	// flag any divergence between what they typed and what will land.
	sanitized := sanitizeFilename(s.modalFilename)
	previewPath := core.MapPath(sanitized)
	rl.DrawTextEx(font, fmt.Sprintf("Will save to: %s", previewPath),
		rl.NewVector2(r.X+modalContentInset, r.Y+96), editorFontHint, 1, theme.TextMuted)
	if sanitized != strings.TrimSuffix(strings.TrimSuffix(s.modalFilename, ".map"), ".MAP") {
		rl.DrawTextEx(font, "(Punctuation and spaces are stripped)",
			rl.NewVector2(r.X+modalContentInset, r.Y+112), editorFontTiny, 1, theme.BorderDanger)
	}
	rl.DrawTextEx(font, "Enter save   Esc cancel",
		rl.NewVector2(r.X+modalContentInset, r.Y+r.Height-26), editorFontHint, 1, theme.TextHint)
}

// drawPackEditModal renders the inline pack editor: header with pack
// coords, scrollable member list with the cursor highlighting one entry,
// and a hint row of keyboard shortcuts at the bottom. The member kind
// names come from core.EnemyKindName so adding a new EnemyKind appears
// here for free.
func drawPackEditModal(s *State, font rl.Font, theme render.Theme) {
	if s.modalPackIdx < 0 || s.modalPackIdx >= len(s.area.PackSpawns) {
		return
	}
	pack := s.area.PackSpawns[s.modalPackIdx]
	r := drawModalHeader(font, theme, entityEditModalW, entityEditModalH,
		"PACK AT "+core.TileCoord(pack.TileX, pack.TileZ),
		theme.BorderActive)

	// Leader hint near the top: the rendered field icon is the highest-
	// Tier member, so the author knows whose silhouette shows in-world.
	leaderText := "Leader: —"
	if len(pack.Members) > 0 {
		leaderIdx := core.PackSpawnLeaderSlot(s.area, pack)
		if leaderIdx >= 0 && leaderIdx < len(pack.Members) {
			leaderText = "Leader (highest tier): " + core.PackMemberDisplayName(s.area, pack, leaderIdx)
		}
	}
	render.DrawTextWithShadow(font, leaderText, r.X+modalContentInset, r.Y+38, editorFontHint, theme.TextMuted)
	render.DrawTextWithShadow(font, "Up/Down select · Enter add · X remove · K/J reorder · A cycle AI · Esc close",
		r.X+modalContentInset, r.Y+54, editorFontTiny, theme.TextHint)

	adds, actions := packEditCmds(s)
	lay := entityModalLayoutFor(s.modalCursor, len(pack.Members), cmdLabels(adds), cmdLabels(actions))
	drawEntityListWindow(font, theme, lay, len(pack.Members), s.modalCursor,
		"(empty — close to drop)",
		func(i int) string { return core.PackMemberDisplayName(s.area, pack, i) })
	drawModalButtons(font, lay.actRects, cmdLabels(actions))
	drawModalButtons(font, lay.addRects, cmdLabels(adds))
	drawDropdown(s, font, theme) // add-member list, drawn on top when open
}

// drawChestEditModal renders the inline chest editor: header with chest
// coords, the authored item list with the cursor highlighting one entry, the
// add / remove buttons, and the add-item dropdown on top when open. Mirrors
// drawPackEditModal so the two entity editors read as one visual family.
func drawChestEditModal(s *State, font rl.Font, theme render.Theme) {
	if s.modalChestIdx < 0 || s.modalChestIdx >= len(s.area.ChestSpawns) {
		return
	}
	chest := s.area.ChestSpawns[s.modalChestIdx]
	r := drawModalHeader(font, theme, entityEditModalW, entityEditModalH,
		"CHEST AT "+core.TileCoord(chest.TileX, chest.TileZ),
		theme.BorderActive)
	render.DrawTextWithShadow(font, "Up/Down select · Enter add · X remove · Esc close",
		r.X+modalContentInset, r.Y+40, editorFontTiny, theme.TextHint)

	adds, actions := chestEditCmds(s)
	lay := entityModalLayoutFor(s.modalCursor, len(chest.Items), cmdLabels(adds), cmdLabels(actions))
	drawEntityListWindow(font, theme, lay, len(chest.Items), s.modalCursor,
		"(empty — adds reveal it as pre-looted in game)",
		func(i int) string { return core.ItemInfo(chest.Items[i]).Name })
	drawModalButtons(font, lay.actRects, cmdLabels(actions))
	drawModalButtons(font, lay.addRects, cmdLabels(adds))
	drawDropdown(s, font, theme) // add-item list, drawn on top when open
}

// drawDoorEditModal renders the per-door editor. Mirrors the save-as
// modal's text-field plumbing but with three fields instead of one,
// plus a facing-buttons row and a delete affordance.
func drawDoorEditModal(s *State, font rl.Font, theme render.Theme) {
	if s.modalDoorIdx < 0 || s.modalDoorIdx >= len(s.area.DoorSpawns) {
		return
	}
	door := s.area.DoorSpawns[s.modalDoorIdx]
	l := doorEditLayoutFor()
	header := "DOOR AT " + core.TileCoord(door.TileX, door.TileZ)
	drawModalHeaderAt(font, theme, l.card, header, theme.BorderActive)

	// Name field.
	drawLabel(font, "Name (unique on this map)",
		rl.NewRectangle(l.nameField.X, l.nameField.Y-16, l.nameField.Width, 14))
	drawTextField(font, l.nameField, door.Name, s.focus == focusDoorName)

	// Target map field.
	drawLabel(font, "Target map (bare id, or 'self')",
		rl.NewRectangle(l.mapField.X, l.mapField.Y-16, l.mapField.Width, 14))
	drawTextField(font, l.mapField, door.TargetMap, s.focus == focusDoorTargetMap)

	// Target door field.
	drawLabel(font, "Target door (Name on destination map)",
		rl.NewRectangle(l.doorField.X, l.doorField.Y-16, l.doorField.Width, 14))
	drawTextField(font, l.doorField, door.TargetDoor, s.focus == focusDoorTargetDoor)

	// Facing row.
	lastFacing := l.facing[core.FacingCount-1]
	drawLabel(font, "Facing / wall to affix to (player walks out this way)",
		rl.NewRectangle(l.facing[0].X, l.facing[0].Y-16, lastFacing.X+lastFacing.Width-l.facing[0].X, 14))
	for i, fr := range l.facing {
		drawButton(font, fr, core.FacingShortLabels[i], door.Facing == i)
	}

	// Style row.
	lastStyle := l.style[core.DoorStyleCount-1]
	drawLabel(font, "Style (visual fixture)",
		rl.NewRectangle(l.style[0].X, l.style[0].Y-16, lastStyle.X+lastStyle.Width-l.style[0].X, 14))
	for i, sr := range l.style {
		drawButton(font, sr, core.DoorStyleLabels[i], door.Style == core.DoorStyle(i))
	}

	// Delete + Close buttons.
	drawButton(font, l.deleteBtn, "Delete door (X)", false)
	drawButton(font, l.closeBtn, "Done (Esc)", false)

	// Footer hint string mirrors the other modals' tiny hint row.
	hint := "Tab cycle fields   N/E/S/W facing   1/2/3 style   X delete   Esc done"
	rl.DrawTextEx(font, hint,
		rl.NewVector2(l.card.X+modalContentInset, l.card.Y+l.card.Height-72),
		11, 1, theme.TextHint)
}

// drawValidateModal renders the full reachability + cross-map door
// warning list captured at modal-open time. Read-only viewer; any
// keystroke dismisses.
func drawValidateModal(s *State, font rl.Font, theme render.Theme) {
	rows := s.modalValidateRows
	pw := validateModalW
	ph := float32(56 + float32(len(rows))*22 + 56)
	if ph < 160 {
		ph = 160
	}
	_, sh := render.ScreenSizeF()
	if ph > sh-40 {
		ph = sh - 40
	}
	r := drawModalHeader(font, theme, pw, ph, "VALIDATE MAP", theme.BorderActive)
	if len(rows) == 0 {
		rl.DrawTextEx(font, "All checks pass.",
			rl.NewVector2(r.X+modalContentInset, r.Y+50), editorFontBody, 1, theme.BorderStrong)
	} else {
		y := r.Y + 50
		for _, line := range rows {
			rl.DrawTextEx(font, "! "+line,
				rl.NewVector2(r.X+modalContentInset, y), editorFontAccent, 1, theme.BorderDanger)
			y += 22
		}
	}
	rl.DrawTextEx(font, "Esc / Enter / click   close",
		rl.NewVector2(r.X+modalContentInset, r.Y+r.Height-26), editorFontHint, 1, theme.TextHint)
}

// entityKindRow tags an entity-list row by what it points at.
type entityKindRow int

const (
	elStart entityKindRow = iota
	elPack
	elChest
	elDoor
)

// entityListRow is one row of the Objects index — a label plus the tile to
// jump to and which editor the row opens.
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
)

// entityListRows builds the Objects index fresh: player start, then every
// pack / chest / door with a one-line summary + tile coord. Rebuilt on demand
// (entities are few), so indices always match the live spawn slices.
func entityListRows(s *State) []entityListRow {
	rows := []entityListRow{{
		label: "Player start  —  " + core.TileCoord(s.area.StartTileX, s.area.StartTileZ),
		x:     s.area.StartTileX, z: s.area.StartTileZ, kind: elStart,
	}}
	for i, p := range s.area.PackSpawns {
		name := "(empty)"
		if len(p.Members) > 0 {
			name = core.PackMemberDisplayName(s.area, p, core.PackSpawnLeaderSlot(s.area, p))
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
	default:
		return render.MarkerStart
	}
}

// entityListGeom is the shared layout for the Objects modal — the card, the
// first row's Y, the full row list, and the visible [top, end) window — so the
// draw and the click hit-test can't drift.
func entityListGeom(s *State) (card rl.Rectangle, listTop float32, rows []entityListRow, top, end int) {
	rows = entityListRows(s)
	shown := len(rows)
	if shown > entityListVisible {
		shown = entityListVisible
	}
	ph := 56 + float32(shown)*objectsRowH + 36
	if ph < 150 {
		ph = 150
	}
	card = centeredCardRect(entityListModalW, ph)
	listTop = card.Y + 46
	top, end = scrollWindow(s.modalCursor, len(rows), entityListVisible)
	return card, listTop, rows, top, end
}

// entityListRowRect is the clickable rect for the screenRow-th visible row
// (0-based within the window).
func entityListRowRect(card rl.Rectangle, listTop float32, screenRow int) rl.Rectangle {
	return rl.NewRectangle(card.X+modalContentInset, listTop+float32(screenRow)*objectsRowH,
		card.Width-2*modalContentInset, objectsRowH)
}

func drawEntityListModal(s *State, font rl.Font, theme render.Theme) {
	card, listTop, rows, top, end := entityListGeom(s)
	drawModalHeaderAt(font, theme, card, "OBJECTS", theme.BorderActive)
	if len(rows) == 0 {
		rl.DrawTextEx(font, "No objects placed.",
			rl.NewVector2(card.X+modalContentInset, listTop), editorFontBody, 1, theme.TextMuted)
	}
	mp := frameMouse
	for i := top; i < end; i++ {
		rr := entityListRowRect(card, listTop, i-top)
		if i == s.modalCursor {
			rl.DrawRectangleRec(rr, bgActive)
		} else if pointIn(mp, rr) {
			rl.DrawRectangleRec(rr, bgEntryHover)
		}
		rl.DrawCircleV(rl.NewVector2(rr.X+8, rr.Y+rr.Height/2), 4, entityRowColor(rows[i].kind))
		col := theme.TextMuted
		if i == s.modalCursor {
			col = theme.TextPrimary
		}
		rl.DrawTextEx(font, rows[i].label, rl.NewVector2(rr.X+22, rr.Y+(rr.Height-16)/2), editorFontBody, 1, col)
	}
	rl.DrawTextEx(font, fmt.Sprintf("%d objects   ·   Up/Down + Enter or click a row to jump + edit   ·   Esc close", len(rows)),
		rl.NewVector2(card.X+modalContentInset, card.Y+card.Height-26), editorFontHint, 1, theme.TextHint)
}

// drawEscMenuModal paints the editor's pause-style menu. Three rows:
//   - Display: <Fullscreen|Windowed> — toggles via D
//   - Continue editing — closes the menu (C / Esc)
//   - Exit to Title — E; routes through the standard dirty-bounce
//
// Body is intentionally minimal so the menu doesn't cover the area
// the author was just looking at; sits centered.
// The confirm/menu modals build their buttons as []modalCmd (label +
// keyboard accelerator + action, all on one row) via the *Cmds funcs in
// input.go; both the draw (cmdLabels) and the handler (runModalCmds) call
// the same builder so captions and actions can't drift.

func drawEscMenuModal(s *State, font rl.Font, theme render.Theme) {
	r := drawModalHeader(font, theme, escMenuModalW, escMenuModalH, "EDITOR MENU", theme.BorderActive)
	labels := cmdLabels(escMenuCmds(s))
	drawModalButtons(font, modalButtonStack(r, labels), labels)
	render.DrawTextWithShadow(font, "(D display · C continue · E exit · Esc close)",
		r.X+modalContentInset, r.Y+40, editorFontHint, theme.TextHint)
}

func drawConfirmDirtyModal(s *State, font rl.Font, theme render.Theme) {
	r := drawModalHeader(font, theme, confirmDirtyModalW, confirmDirtyModalH, "UNSAVED CHANGES", theme.BorderActive)

	id := core.MapIDFromPath(s.area.Path)
	if id == "" {
		id = "(unsaved)"
	}
	body := fmt.Sprintf("%s has unsaved edits.", id)
	switch s.pending {
	case pendingNew:
		body = fmt.Sprintf("%s has unsaved edits. Discarding for new map.", id)
	case pendingOpen:
		body = fmt.Sprintf("%s has unsaved edits. Discarding to open another.", id)
	}
	saveLabel := "S  Save and exit"
	discardLabel := "D  Discard and exit"
	switch s.pending {
	case pendingNew:
		saveLabel = "S  Save then start new map"
		discardLabel = "D  Discard then start new map"
	case pendingOpen:
		saveLabel = "S  Save then pick another map"
		discardLabel = "D  Discard then pick another map"
	}

	rl.DrawTextEx(font, body, rl.NewVector2(r.X+modalContentInset, r.Y+44), editorFontLabel, 1, theme.TextPrimary)
	// Contextual hint above the buttons explains what Save/Discard do for
	// this pending action (new map / open / exit); the buttons stay short.
	render.DrawTextWithShadow(font, hintForPending(saveLabel, discardLabel), r.X+modalContentInset, r.Y+66, editorFontHint, theme.TextHint)

	labels := cmdLabels(confirmDirtyCmds(s))
	drawModalButtons(font, modalButtonStack(r, labels), labels)
}

// hintForPending strips the leading "S  " / "D  " accelerator prefix off
// the contextual save/discard captions so the hint reads as prose under
// the body line.
func hintForPending(saveLabel, discardLabel string) string {
	trim := func(s string) string {
		if i := strings.Index(s, "  "); i >= 0 {
			return strings.TrimSpace(s[i:])
		}
		return s
	}
	return trim(saveLabel) + " · " + trim(discardLabel)
}

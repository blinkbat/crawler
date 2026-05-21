package editor

import (
	"crawler/internal/app/core"
	"crawler/internal/app/render"
	"fmt"
	"image/color"
	"math"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"
)

const (
	topbarH    = float32(40)
	paletteW   = float32(170)
	metadataW  = float32(290)
	gridMargin = float32(8)
	layerTabH  = float32(28)
)

// layout recomputes screen rectangles each frame from the current window
// size. Cell pixel size is the auto-fit size scaled by s.zoom; pan offsets
// nudge the plot off-center so users can drag around large maps.
func (s *State) layout() {
	w, h := render.ScreenSizeF()

	s.rect.topbar = rl.NewRectangle(0, 0, w, topbarH)
	// Layer tabs sit at the top of the palette column.
	tabsHeight := float32(layerCount) * layerTabH
	s.rect.layerTabs = rl.NewRectangle(0, topbarH, paletteW, tabsHeight)
	paletteY := topbarH + tabsHeight
	s.rect.palette = rl.NewRectangle(0, paletteY, paletteW, h-paletteY)
	s.rect.metadata = rl.NewRectangle(w-metadataW, topbarH, metadataW, h-topbarH)
	s.rect.grid = rl.NewRectangle(paletteW, topbarH, w-paletteW-metadataW, h-topbarH)

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
	s.rect.gridX = s.rect.grid.X + (s.rect.grid.Width-totalW)/2 + s.panX
	s.rect.gridY = s.rect.grid.Y + (s.rect.grid.Height-totalH)/2 + s.panY
	s.rect.gridW = totalW
	s.rect.gridH = totalH
}

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
func Draw(s *State, assets render.Resources) {
	font := assets.Font()
	theme := assets.Theme()
	s.layout()
	rl.ClearBackground(bgWindow)
	drawTopbar(s, font, theme)
	drawLayerTabs(s, font, theme)
	drawPalette(s, font, theme)
	drawMetadata(s, font, theme)
	drawGrid(s, font)
	if len(s.statusLog) > 0 {
		drawStatus(s, font, theme)
	}
	if drawer, ok := modalDrawers[s.modal]; ok {
		drawer(s, font, theme)
	}
}

// modalDrawers maps each modalKind to its renderer. Replaces the prior
// chain of `if s.modal == modalX { drawXModal(...) }` checks — adding a
// new modal is one row in this table plus one in modalUpdaters
// (input.go) instead of editing two switch-like chains in lockstep.
var modalDrawers = map[modalKind]func(*State, rl.Font, render.Theme){
	modalOpen:         drawOpenModal,
	modalSaveAs:       drawSaveAsModal,
	modalConfirmDirty: drawConfirmDirtyModal,
	modalPackEdit:     drawPackEditModal,
	modalChestEdit:    drawChestEditModal,
	modalSounds:       drawSoundsModal,
	modalDoorEdit:     drawDoorEditModal,
	modalValidate:     drawValidateModal,
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
	doorHitDelete
	doorHitClose
)

// doorEditHit pairs the hit kind with optional payload (the facing value
// when kind == doorHitFacing).
type doorEditHit struct {
	kind   doorEditHitTarget
	facing int
}

// doorEditLayout returns the rectangles for every clickable region of
// the door edit modal so update and draw stay in sync. Pure function of
// screen size + position.
type doorEditLayout struct {
	card       rl.Rectangle
	nameField  rl.Rectangle
	mapField   rl.Rectangle
	doorField  rl.Rectangle
	facing     [4]rl.Rectangle
	deleteBtn  rl.Rectangle
	closeBtn   rl.Rectangle
}

func doorEditLayoutFor() doorEditLayout {
	w, h := render.ScreenSizeF()
	pw := float32(480)
	ph := float32(360)
	r := rl.NewRectangle((w-pw)/2, (h-ph)/2, pw, ph)
	x := r.X + 16
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
	// Facing row: four equal-width buttons.
	bw := (fw - 18) / 4
	var facing [4]rl.Rectangle
	for i := 0; i < 4; i++ {
		facing[i] = rl.NewRectangle(x+float32(i)*(bw+6), y, bw, fieldH)
	}
	y = r.Y + r.Height - 44
	deleteBtn := rl.NewRectangle(x, y, 110, 30)
	closeBtn := rl.NewRectangle(r.X+r.Width-110-16, y, 110, 30)
	return doorEditLayout{
		card:      r,
		nameField: nameField,
		mapField:  mapField,
		doorField: doorField,
		facing:    facing,
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

type topbarBtn struct {
	id    string
	label string
}

var topbarBtns = []topbarBtn{
	{"new", "New"},
	{"open", "Open"},
	{"save", "Save"},
	{"saveas", "Save As"},
	{"sounds", "Sounds"},
	{"validate", "Validate"},
	{"back", "Back"},
}

func topbarButtonAt(s *State, p rl.Vector2) string {
	if !pointIn(p, s.rect.topbar) {
		return ""
	}
	x := float32(8)
	for _, b := range topbarBtns {
		w := buttonWidth(b.label)
		r := rl.NewRectangle(x, 6, w, topbarH-12)
		if pointIn(p, r) {
			return b.id
		}
		x += w + 6
	}
	return ""
}

func drawTopbar(s *State, font rl.Font, theme render.Theme) {
	rl.DrawRectangleRec(s.rect.topbar, theme.SurfacePrimary)
	rl.DrawLineEx(rl.NewVector2(0, topbarH), rl.NewVector2(s.rect.topbar.Width, topbarH), 1, outlineHard)

	x := float32(8)
	for _, b := range topbarBtns {
		w := buttonWidth(b.label)
		r := rl.NewRectangle(x, 6, w, topbarH-12)
		drawButton(font, r, b.label, false)
		x += w + 6
	}

	id := core.MapIDFromPath(s.area.Path)
	if id == "" {
		id = "(unsaved)"
	}
	dirtyMark := ""
	if s.dirty {
		dirtyMark = " *"
	}
	label := fmt.Sprintf("%s%s", id, dirtyMark)
	measure := rl.MeasureTextEx(font, label, 16, 1)
	labelX := s.rect.topbar.Width - measure.X - 10
	render.DrawTextWithShadow(font, label,
		labelX, (topbarH-measure.Y)/2,
		16, theme.TextMuted)

	coord := "—"
	hoverDesc := ""
	if s.hoverX >= 0 {
		coord = core.TileCoord(s.hoverX, s.hoverZ)
		hoverDesc = core.AreaTileSummary(s.area, s.hoverX, s.hoverZ)
	}
	infoLabel := fmt.Sprintf("cell %s   %s   layer %s   brush %dx%d   zoom %.0f%%   phase %s (T)   undo %d/%d",
		coord, hoverDesc, layerName(s.layer), s.brushSize, s.brushSize, s.zoom*100, core.PhaseName(s.previewPhase), len(s.undo), undoLimit)
	infoMeasure := rl.MeasureTextEx(font, infoLabel, 13, 1)
	infoX := labelX - infoMeasure.X - 24
	render.DrawTextWithShadow(font, infoLabel, infoX, (topbarH-infoMeasure.Y)/2, 13, theme.TextHint)
}

// topbarBtnWidths overrides the default 64-px topbar button width for
// labels that need extra space. Adding a wider button is a one-row
// edit; missing entries fall through to the default.
var topbarBtnWidths = map[string]float32{
	"Save As":  80,
	"Validate": 80,
	"Back":     60,
}

func buttonWidth(label string) float32 {
	if w, ok := topbarBtnWidths[label]; ok {
		return w
	}
	return 64
}

func drawButton(font rl.Font, r rl.Rectangle, label string, active bool) {
	bg := bgButton
	border := editorBorderMid
	text := textBright
	if active {
		bg = bgActive
		border = editorBorderActive
	}
	if pointIn(rl.GetMousePosition(), r) {
		bg = bgRowHover
	}
	rl.DrawRectangleRec(r, bg)
	rl.DrawRectangleLinesEx(r, 1, border)
	measure := rl.MeasureTextEx(font, label, 14, 1)
	rl.DrawTextEx(font, label,
		rl.NewVector2(r.X+(r.Width-measure.X)/2, r.Y+(r.Height-measure.Y)/2),
		14, 1, text)
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

func drawLayerTabs(s *State, font rl.Font, theme render.Theme) {
	rl.DrawRectangleRec(s.rect.layerTabs, bgWindow)
	for i := 0; i < layerCount; i++ {
		r := layerTabRect(s, i)
		active := Layer(i) == s.layer
		bg := bgPanel
		border := editorBorderDim
		text := theme.TextMuted
		if active {
			bg = bgActive
			border = editorBorderActive
			text = theme.TextPrimary
		} else if pointIn(rl.GetMousePosition(), r) {
			bg = bgEntryHover
		}
		// Inset the tab so consecutive tabs don't share a border.
		inner := rl.NewRectangle(r.X+6, r.Y+3, r.Width-12, r.Height-6)
		rl.DrawRectangleRec(inner, bg)
		rl.DrawRectangleLinesEx(inner, 1, border)
		label := fmt.Sprintf("%d %s", i+1, layerName(Layer(i)))
		render.DrawTextWithShadow(font, label, inner.X+10, inner.Y+(inner.Height-14)/2, 14, text)
	}
}

// --- Palette ---------------------------------------------------------------

func paletteToolAt(s *State, p rl.Vector2) int {
	if !pointIn(p, s.rect.palette) {
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
	paletteRowH      = float32(28)
	paletteRowStride = paletteRowH + 3
	// headerReserve is the vertical space inside the palette panel
	// reserved for the "BRUSHES" heading; entry / hint rendering sits
	// below this band and the scissor clip below the heading uses it
	// so scrolled content can't paint into the heading row.
	headerReserve = float32(34)
)

// paletteHints is the keyboard-shortcut cheat sheet rendered below
// the brush list. Promoted from a hand-counted const + open-coded
// slice literal to a single source of truth: paletteContentHeight
// computes scroll bounds from len(paletteHints), so adding or
// removing a hint can never drift from the layout math.
var paletteHints = []string{
	"L-drag: paint",
	"R-click: erase",
	"Shift+drag: rect",
	"Ctrl+click: fill region",
	"Ctrl+Shift+F: fill all",
	"Tab: next layer",
	"Alt+1..6: jump layer",
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
	y := s.rect.palette.Y + 12 + float32(i)*paletteRowStride - s.paletteScroll[s.layer]
	return rl.NewRectangle(s.rect.palette.X+8, y, s.rect.palette.Width-16, paletteRowH)
}

// paletteContentHeight returns the pixel height required to render the
// active layer's full brush list (including the top/bottom padding and
// the hint footer). Used by ScrollPalette to clamp the scroll offset
// so the last row stays visible. Reads len(paletteHints) directly so
// adding a shortcut row updates both the rendered list AND the
// scroll bound in one edit.
func paletteContentHeight(s *State) float32 {
	palette := layerBrushes[s.layer]
	return 12 + float32(len(palette))*paletteRowStride + 12 + float32(len(paletteHints))*14 + 16
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
	for i, b := range palette {
		r := paletteEntryRect(s, i)
		// Skip entries entirely outside the visible band — cheap, and
		// also avoids drawing brush hover highlights for entries the
		// mouse can't actually reach.
		if r.Y+r.Height < s.rect.palette.Y || r.Y > s.rect.palette.Y+s.rect.palette.Height {
			continue
		}
		active := s.brushIdx[s.layer] == i
		bg := bgEntry
		if active {
			bg = bgActive
		}
		if pointIn(rl.GetMousePosition(), r) {
			bg = bgButtonHover
		}
		rl.DrawRectangleRec(r, bg)
		border := editorBorderDim
		if active {
			border = editorBorderActive
		}
		rl.DrawRectangleLinesEx(r, 1, border)

		swatch := rl.NewRectangle(r.X+6, r.Y+6, 20, r.Height-12)
		rl.DrawRectangleRec(swatch, b.Color)
		// Sentinel brushes (Auto / None / Force-empty) paint a SEMANTIC
		// value, not a visible tile — overlay a diagonal-stripe pattern
		// on their swatches so the author can tell at a glance they're
		// not picking a "blue water" vs "gold sand" color but rather a
		// "let the renderer decide" or "do not draw" sentinel.
		if isSentinelBrush(s.layer, b.Char) {
			drawSentinelHatch(swatch)
		}
		rl.DrawRectangleLinesEx(swatch, 1, swatchEdge)

		nameCol := textEntry
		if isSentinelBrush(s.layer, b.Char) {
			nameCol = rl.NewColor(190, 200, 220, 255)
		}
		txt := fmt.Sprintf("%d %s", i+1, b.Name)
		rl.DrawTextEx(font, txt, rl.NewVector2(r.X+34, r.Y+(r.Height-14)/2), 14, 1, nameCol)
	}

	y := s.rect.palette.Y + 16 + float32(len(palette))*paletteRowStride + 12
	for _, h := range paletteHints {
		rl.DrawTextEx(font, h, rl.NewVector2(s.rect.palette.X+12, y), 11, 1, theme.TextHint)
		y += 14
	}
}

// isSentinelBrush reports whether (layer, char) is a "semantic" brush —
// Auto / Force-empty / None — that doesn't paint a visible tile. Used by
// the palette to render those swatches distinctly so the author doesn't
// confuse "let the renderer scatter" with "paint THIS particular look".
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
	}
	return false
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
	startLabel, startInfo                rl.Rectangle
	facingLabel                          rl.Rectangle
	facingButtons                        []rl.Rectangle
	pathLabel, pathValue                 rl.Rectangle
	// reachLabel + reachArea bound the clickable reachability badge.
	// reachArea covers the badge fill region (not just the label) so
	// the metadata click handler can route any click in that zone to
	// the Validate modal. drawMetadata renders inside reachArea.
	reachLabel, reachArea rl.Rectangle
}

func metadataRects(s *State) metaRect {
	x := s.rect.metadata.X + 12
	w := s.rect.metadata.Width - 24
	y := s.rect.metadata.Y + 12

	r := metaRect{}

	r.nameLabel = rl.NewRectangle(x, y, w, 14)
	y += 18
	r.nameField = rl.NewRectangle(x, y, w, 26)
	y += 36

	r.matLabel = rl.NewRectangle(x, y, w, 14)
	y += 18
	r.matButtons = make([]rl.Rectangle, len(core.MaterialOptions))
	bw := (w - 6) / float32(len(core.MaterialOptions))
	for i := range core.MaterialOptions {
		r.matButtons[i] = rl.NewRectangle(x+float32(i)*(bw+6), y, bw, 26)
	}
	y += 36

	r.quietLabel = rl.NewRectangle(x, y, w, 14)
	y += 18
	r.quietField = rl.NewRectangle(x, y, w, 26)
	y += 36

	r.dimsLabel = rl.NewRectangle(x, y, w, 14)
	y += 18
	r.widthValue = rl.NewRectangle(x, y, 80, 26)
	r.widthMinus = rl.NewRectangle(x+86, y, 26, 26)
	r.widthPlus = rl.NewRectangle(x+118, y, 26, 26)
	y += 32
	r.heightValue = rl.NewRectangle(x, y, 80, 26)
	r.heightMinus = rl.NewRectangle(x+86, y, 26, 26)
	r.heightPlus = rl.NewRectangle(x+118, y, 26, 26)
	y += 36

	r.startLabel = rl.NewRectangle(x, y, w, 14)
	y += 18
	r.startInfo = rl.NewRectangle(x, y, w, 26)
	y += 36

	r.facingLabel = rl.NewRectangle(x, y, w, 14)
	y += 18
	r.facingButtons = make([]rl.Rectangle, 4)
	fbw := (w - 18) / 4
	for i := 0; i < 4; i++ {
		r.facingButtons[i] = rl.NewRectangle(x+float32(i)*(fbw+6), y, fbw, 26)
	}
	// On-disk path readout rows. Spacing matches the per-row stride
	// the rest of the metadata panel uses (60px from facing baseline
	// to path label) so the badge below stays at its visual anchor.
	y = r.facingLabel.Y + 60
	r.pathLabel = rl.NewRectangle(x, y, w, 14)
	r.pathValue = rl.NewRectangle(x, y+18, w, 26)
	// Reachability badge. Label sits 56 px below pathLabel; the
	// clickable badge region extends past the label to cover the
	// "OK" / warning panel that follows underneath.
	reachY := y + 56
	r.reachLabel = rl.NewRectangle(x, reachY, w, 14)
	r.reachArea = rl.NewRectangle(x, reachY, w, 120)
	return r
}

func handleMetadataClick(s *State, p rl.Vector2) bool {
	if !pointIn(p, s.rect.metadata) {
		return false
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
			s.area.Materials = core.MaterialOptions[i]
			s.dirty = true
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
	for i, br := range mr.facingButtons {
		if pointIn(p, br) {
			s.area.StartFacing = i
			s.dirty = true
			return true
		}
	}
	return false
}

func (s *State) activeFieldRect() rl.Rectangle {
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
	case focusFilename:
		return saveAsFieldRect(s)
	}
	return rl.Rectangle{}
}

func drawMetadata(s *State, font rl.Font, theme render.Theme) {
	rl.DrawRectangleRec(s.rect.metadata, rl.NewColor(24, 28, 38, 255))
	rl.DrawLineEx(
		rl.NewVector2(s.rect.metadata.X, s.rect.metadata.Y),
		rl.NewVector2(s.rect.metadata.X, s.rect.metadata.Y+s.rect.metadata.Height),
		1, outlineHard)

	render.DrawHeading(font, "MAP", int32(s.rect.metadata.X+12), int32(s.rect.metadata.Y+8), theme.BorderStrong)

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
	drawButton(font, mr.widthMinus, "-", false)
	drawButton(font, mr.widthPlus, "+", false)
	drawTextField(font, mr.heightValue, hText, s.focus == focusHeight)
	drawButton(font, mr.heightMinus, "-", false)
	drawButton(font, mr.heightPlus, "+", false)

	drawLabel(font, "Player start", mr.startLabel)
	drawReadonlyValue(font, mr.startInfo, core.TileCoord(s.area.StartTileX, s.area.StartTileZ))

	drawLabel(font, "Facing (R cycles)", mr.facingLabel)
	for i, br := range mr.facingButtons {
		drawButton(font, br, core.FacingShortLabels[i], s.area.StartFacing == i)
	}

	// Path readout — readonly. Anchored below the facing row so it doesn't
	// reshuffle the rest of the panel. Shows "(unsaved)" before the first
	// save, or the relative on-disk path once known.
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
		badgeValue := rl.NewRectangle(mr.reachArea.X, mr.reachArea.Y+18, mr.reachArea.Width, 26)
		rl.DrawRectangleRec(badgeValue, rl.NewColor(14, 22, 18, 255))
		rl.DrawRectangleLinesEx(badgeValue, 1, rl.NewColor(70, 130, 100, 255))
		rl.DrawTextEx(font, "OK", rl.NewVector2(badgeValue.X+8, badgeValue.Y+(badgeValue.Height-14)/2), 14, 1, rl.NewColor(150, 220, 180, 255))
	} else {
		// Stack one row per warning at 22px stride so the author can
		// read them all without hover/click. Red panel + outline so the
		// badge pops against the metadata column's neutral background.
		rows := warnings
		if len(rows) > 4 {
			rows = rows[:4] // cap so we don't reflow the panel
		}
		h := float32(8 + 18*len(rows))
		box := rl.NewRectangle(mr.reachArea.X, mr.reachArea.Y+18, mr.reachArea.Width, h)
		rl.DrawRectangleRec(box, rl.NewColor(38, 16, 18, 255))
		rl.DrawRectangleLinesEx(box, 1, rl.NewColor(180, 80, 80, 255))
		for i, w := range rows {
			rl.DrawTextEx(font, "! "+w,
				rl.NewVector2(box.X+6, box.Y+4+float32(i)*18),
				12, 1, rl.NewColor(240, 180, 180, 255))
		}
		if len(warnings) > len(rows) {
			rl.DrawTextEx(font, fmt.Sprintf("(+%d more)", len(warnings)-len(rows)),
				rl.NewVector2(box.X+6, box.Y+h-16),
				11, 1, rl.NewColor(240, 180, 180, 220))
		}
	}
}

func drawLabel(font rl.Font, text string, r rl.Rectangle) {
	rl.DrawTextEx(font, text, rl.NewVector2(r.X, r.Y), 12, 1, rl.NewColor(138, 160, 188, 220))
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
	rl.DrawTextEx(font, display, rl.NewVector2(r.X+8, r.Y+(r.Height-14)/2), 14, 1, textEntry)
}

func drawReadonlyValue(font rl.Font, r rl.Rectangle, text string) {
	rl.DrawRectangleRec(r, bgFieldInset)
	rl.DrawRectangleLinesEx(r, 1, editorBorderInactive)
	rl.DrawTextEx(font, text, rl.NewVector2(r.X+8, r.Y+(r.Height-14)/2), 14, 1, textReadonly)
}

// --- Grid ------------------------------------------------------------------

// drawGrid paints all four grid layers stacked, then the entity overlays.
// Layers other than the active one are dimmed so the focus is on what
// the next click will affect. Order: floor → walls → decor → props →
// entities (start + spawns).
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

	for z := 0; z < s.area.Height; z++ {
		for x := 0; x < s.area.Width; x++ {
			r := rl.NewRectangle(s.rect.gridX+float32(x)*cell, s.rect.gridY+float32(z)*cell, cell, cell)
			// Floor is the base — always painted (except under a wall, where
			// the wall covers it).
			rl.DrawRectangleRec(r, fadeAlpha(floorColor(s.area.Floor[z][x]), floorAlpha))
			if s.area.Walls[z][x] == core.TileRock {
				rl.DrawRectangleRec(r, fadeAlpha(wallColor(), wallAlpha))
			}
			if d := s.area.Decor[z][x]; d != core.DecorAuto {
				rl.DrawRectangleRec(insetRect(r, cell*0.28), fadeAlpha(decorColor(d), decorAlpha))
			}
			if p := s.area.Props[z][x]; core.IsPropChar(p) {
				rl.DrawCircle(int32(r.X+cell/2), int32(r.Y+cell/2), cell*0.36, fadeAlpha(propColor(p), propAlpha))
			}
			// Ceiling hash overlay: shown only when the Ceiling layer is
			// active or the cell holds a ceiling. Two diagonal stripes
			// inside the cell so it reads as "covered" without obscuring
			// the layer underneath.
			if s.area.CeilingAt(x, z) {
				drawCeilingHash(r, cell, fadeAlpha(ceilingColor(), ceilingAlpha))
			}
		}
	}

	// Grid lines. Every 5 cells draws a slightly darker line (gridLineMajor)
	// so the author can eyeball coordinates at a glance — matches the
	// "tick every 5" convention common in tile editors.
	for x := 0; x <= s.area.Width; x++ {
		px := s.rect.gridX + float32(x)*cell
		col := gridLineCol
		if x%5 == 0 {
			col = gridLineMajor
		}
		rl.DrawLineEx(rl.NewVector2(px, s.rect.gridY), rl.NewVector2(px, s.rect.gridY+s.rect.gridH), 1, col)
	}
	for z := 0; z <= s.area.Height; z++ {
		py := s.rect.gridY + float32(z)*cell
		col := gridLineCol
		if z%5 == 0 {
			col = gridLineMajor
		}
		rl.DrawLineEx(rl.NewVector2(s.rect.gridX, py), rl.NewVector2(s.rect.gridX+s.rect.gridW, py), 1, col)
	}

	// Axis tick labels every 5 cells. Only at zoom levels where cells are
	// big enough to comfortably fit a 10pt digit — at very small zooms the
	// labels would overlap and read as visual noise.
	if cell >= 18 {
		tickCol := rl.NewColor(220, 224, 232, 180)
		// Top axis: column numbers.
		for x := 0; x <= s.area.Width; x += 5 {
			label := fmt.Sprintf("%d", x)
			m := rl.MeasureTextEx(font, label, 10, 1)
			px := s.rect.gridX + float32(x)*cell - m.X/2
			py := s.rect.gridY - m.Y - 2
			if py < s.rect.grid.Y+2 {
				continue
			}
			rl.DrawTextEx(font, label, rl.NewVector2(px, py), 10, 1, tickCol)
		}
		// Left axis: row numbers.
		for z := 0; z <= s.area.Height; z += 5 {
			label := fmt.Sprintf("%d", z)
			m := rl.MeasureTextEx(font, label, 10, 1)
			px := s.rect.gridX - m.X - 4
			py := s.rect.gridY + float32(z)*cell - m.Y/2
			if px < s.rect.grid.X+2 {
				continue
			}
			rl.DrawTextEx(font, label, rl.NewVector2(px, py), 10, 1, tickCol)
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
		cx := s.rect.gridX + (float32(sp.TileX)+0.5)*cell
		cy := s.rect.gridY + (float32(sp.TileZ)+0.5)*cell
		leader := packSpawnLeaderKind(sp)
		col := fadeAlpha(packMarkerColor(leader), entityAlpha)
		rl.DrawCircle(int32(cx), int32(cy), cell*0.32, col)
		rl.DrawCircleLines(int32(cx), int32(cy), cell*0.32, fadeAlpha(entityMarkerOutline, entityAlpha))
		label := packMarkerInitial(leader)
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
		gx := s.rect.gridX + float32(c.TileX)*cell
		gy := s.rect.gridY + float32(c.TileZ)*cell
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
		gx := s.rect.gridX + float32(d.TileX)*cell
		gy := s.rect.gridY + float32(d.TileZ)*cell
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

	// Player start marker.
	sx := s.rect.gridX + (float32(s.area.StartTileX)+0.5)*cell
	sy := s.rect.gridY + (float32(s.area.StartTileZ)+0.5)*cell
	startCol := fadeAlpha(render.MarkerStart, entityAlpha)
	rl.DrawCircle(int32(sx), int32(sy), cell*0.36, startCol)
	rl.DrawCircleLines(int32(sx), int32(sy), cell*0.36, fadeAlpha(entityMarkerOutline, entityAlpha))
	dx, dz := core.FacingVector(s.area.StartFacing)
	tx := sx + float32(dx)*cell*0.42
	ty := sy + float32(dz)*cell*0.42
	rl.DrawLineEx(rl.NewVector2(sx, sy), rl.NewVector2(tx, ty), 3, fadeAlpha(rl.NewColor(20, 14, 0, 255), entityAlpha))

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
			outline := rl.NewColor(120, 240, 140, 220)
			fill := rl.NewColor(120, 240, 140, 60)
			if !ok {
				outline = rl.NewColor(240, 110, 110, 230)
				fill = rl.NewColor(240, 110, 110, 80)
			}
			for _, off := range fp {
				fx, fz := hoverPx+off.DX, hoverPz+off.DZ
				if !s.area.InBounds(fx, fz) {
					continue
				}
				r := rl.NewRectangle(s.rect.gridX+float32(fx)*cell, s.rect.gridY+float32(fz)*cell, cell, cell)
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
			r := rl.NewRectangle(s.rect.gridX+float32(x0)*cell, s.rect.gridY+float32(z0)*cell, cell*side, cell*side)
			rl.DrawRectangleLinesEx(r, 2, rl.NewColor(255, 255, 255, 200))
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
		r := rl.NewRectangle(
			s.rect.gridX+float32(x0)*cell,
			s.rect.gridY+float32(z0)*cell,
			float32(x1-x0+1)*cell,
			float32(z1-z0+1)*cell)
		fill := brushPreviewColor(s)
		fill.A = 110
		rl.DrawRectangleRec(r, fill)
		rl.DrawRectangleLinesEx(r, 2, rl.NewColor(255, 255, 255, 220))
	}

	if s.drag == dragStart && s.hoverX >= 0 {
		gx := s.rect.gridX + (float32(s.hoverX)+0.5)*cell
		gy := s.rect.gridY + (float32(s.hoverZ)+0.5)*cell
		ghost := render.MarkerStart
		ghost.A = 220
		rl.DrawCircleLines(int32(gx), int32(gy), cell*0.36, ghost)
	}
	if s.drag == dragPack && s.hoverX >= 0 && s.dragPackIdx >= 0 && s.dragPackIdx < len(s.area.PackSpawns) {
		gx := s.rect.gridX + (float32(s.hoverX)+0.5)*cell
		gy := s.rect.gridY + (float32(s.hoverZ)+0.5)*cell
		rl.DrawCircleLines(int32(gx), int32(gy), cell*0.32, rl.NewColor(255, 255, 255, 220))
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
func drawHoverTooltip(s *State, font rl.Font) {
	x, z := s.hoverX, s.hoverZ
	lines := tooltipLinesFor(s, x, z)
	if len(lines) == 0 {
		return
	}
	const padding = float32(6)
	const lineH = float32(14)
	width := float32(0)
	for _, l := range lines {
		m := rl.MeasureTextEx(font, l, 11, 1)
		if m.X > width {
			width = m.X
		}
	}
	w := width + padding*2
	h := float32(len(lines))*lineH + padding*2
	mp := rl.GetMousePosition()
	tx := mp.X + 14
	ty := mp.Y + 14
	if tx+w > s.rect.grid.X+s.rect.grid.Width {
		tx = mp.X - w - 8
	}
	if ty+h > s.rect.grid.Y+s.rect.grid.Height {
		ty = mp.Y - h - 8
	}
	r := rl.NewRectangle(tx, ty, w, h)
	bg := rl.NewColor(18, 22, 30, 230)
	rl.DrawRectangleRec(r, bg)
	rl.DrawRectangleLinesEx(r, 1, editorBorderActive)
	for i, l := range lines {
		col := rl.NewColor(220, 224, 234, 255)
		if i == 0 {
			col = rl.NewColor(255, 220, 124, 255)
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
			counts := map[core.EnemyKind]int{}
			order := []core.EnemyKind{}
			for _, k := range sp.Members {
				if _, ok := counts[k]; !ok {
					order = append(order, k)
				}
				counts[k]++
			}
			out = append(out, fmt.Sprintf("Pack (%d):", len(sp.Members)))
			for _, k := range order {
				name, ok := core.EnemyKindName(k)
				if !ok {
					name = "?"
				}
				if counts[k] > 1 {
					out = append(out, fmt.Sprintf("  %dx %s", counts[k], name))
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
		out = append(out, "  → "+tgt+"/"+d.TargetDoor)
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
	return rl.NewColor(180, 180, 180, 255)
}

// packMarkerInitial returns the single uppercase letter drawn at the
// center of a pack's marker. Sources from EnemyKindName so it stays in
// sync with the canonical short name. Strips a "diseased_" / "venus_"
// prefix when picking the letter so "Diseased Rat" reads as "D" rather
// than colliding with a future "Demon."
func packMarkerInitial(kind core.EnemyKind) string {
	name, ok := core.EnemyKindName(kind)
	if !ok || len(name) == 0 {
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

func fadeAlpha(c rl.Color, alpha float32) rl.Color {
	a := float32(c.A) * alpha
	if a < 0 {
		a = 0
	}
	if a > 255 {
		a = 255
	}
	out := c
	out.A = uint8(a)
	return out
}

func insetRect(r rl.Rectangle, inset float32) rl.Rectangle {
	return rl.NewRectangle(r.X+inset, r.Y+inset, r.Width-2*inset, r.Height-2*inset)
}

// brushPreviewColor returns a representative tint for the active brush so
// the rectangle drag preview hints at what's about to be painted.
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
	}
	return editorFallbackColor
}

// tileColorByChar maps each grid layer's tile chars to swatch colors,
// built once at init from layerBrushes (editor.go) so the palette UI and
// the grid-cell preview can't drift on color. Adding a new tile char is
// one row in layerBrushes — both the brush palette and the grid preview
// pick it up. Unknown chars fall through to tileColorFallback.
var tileColorByChar = buildTileColorMaps()

func buildTileColorMaps() map[Layer]map[byte]rl.Color {
	out := make(map[Layer]map[byte]rl.Color, len(layerBrushes))
	for layer, brushes := range layerBrushes {
		m := make(map[byte]rl.Color, len(brushes))
		for _, b := range brushes {
			if b.Char != 0 {
				m[b.Char] = b.Color
			}
		}
		out[Layer(layer)] = m
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
	if m, ok := tileColorByChar[layer]; ok {
		if col, ok := m[c]; ok {
			return col
		}
	}
	if col, ok := tileColorFallback[layer]; ok {
		return col
	}
	return editorFallbackColor
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
		m := rl.MeasureTextEx(font, e.msg, 14, 1)
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
		col.A = uint8(float32(col.A) * (0.4 + 0.6*alpha))
		render.DrawTextWithShadow(font, e.msg, r.X+12, y, 14, col)
	}
}

func drawModalVeil(theme render.Theme) {
	w, h := render.ScreenSize()
	rl.DrawRectangle(0, 0, w, h, theme.SurfaceVeil)
}

// joinHintLabels builds the modal hint footer string by concatenating
// each rule's Label with a trailing "Esc close", separated by spaces.
// Shared by drawPackEditModal and drawChestEditModal so a future entity
// editor only writes its add-rule labels and trails through this helper.
// `labels` is the per-rule label list (typically built by ranging over an
// add-rules table); `trail` are tokens appended after the labels in
// order.
func joinHintLabels(labels []string, trail ...string) string {
	all := make([]string, 0, len(labels)+len(trail))
	all = append(all, labels...)
	all = append(all, trail...)
	return strings.Join(all, "   ")
}

func drawModalCard(theme render.Theme, pw, ph float32, accent rl.Color) rl.Rectangle {
	w, h := render.ScreenSizeF()
	r := rl.NewRectangle((w-pw)/2, (h-ph)/2, pw, ph)
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
	drawModalVeil(theme)
	r := drawModalCard(theme, pw, ph, accent)
	render.DrawHeading(font, title, int32(r.X+16), int32(r.Y+12), accent)
	return r
}

func drawOpenModal(s *State, font rl.Font, theme render.Theme) {
	header := "OPEN MAP"
	if s.modalRenaming != "" {
		header = "RENAME MAP"
	} else if s.modalConfirmDelete {
		header = "DELETE MAP"
	}
	r := drawModalHeader(font, theme, 460, 460, header, theme.BorderStrong)

	if len(s.modalPaths) == 0 {
		rl.DrawTextEx(font, "(no .map files in maps/)", rl.NewVector2(r.X+16, r.Y+50), 14, 1, theme.TextMuted)
		rl.DrawTextEx(font, "Esc to close", rl.NewVector2(r.X+16, r.Y+r.Height-26), 12, 1, theme.TextHint)
		return
	}
	// Visible window over the path list so a maps/ dir with more than the
	// modal can show doesn't clip off the bottom (the hint text overdraws
	// the tail rows). scrollWindow handles the cursor-keep-visible math.
	const rowH = float32(22)
	listTop := r.Y + 50
	listBottom := r.Y + r.Height - 32 // leave room for hint row
	rowsVisible := int((listBottom - listTop) / rowH)
	if rowsVisible < 1 {
		rowsVisible = 1
	}
	topRow, end := scrollWindow(s.modalCursor, len(s.modalPaths), rowsVisible)
	for i := topRow; i < end; i++ {
		path := s.modalPaths[i]
		text := core.MapIDFromPath(path)
		col := theme.TextMuted
		if i == s.modalCursor {
			col = theme.BorderActive
			text = "> " + text
		}
		render.DrawTextWithShadow(font, text, r.X+18, listTop+float32(i-topRow)*rowH, 16, col)
	}
	// Scroll hint when the list extends past the visible window.
	if len(s.modalPaths) > rowsVisible {
		more := fmt.Sprintf("(%d / %d)", s.modalCursor+1, len(s.modalPaths))
		measure := rl.MeasureTextEx(font, more, 12, 1)
		rl.DrawTextEx(font, more,
			rl.NewVector2(r.X+r.Width-measure.X-16, r.Y+30),
			12, 1, theme.TextHint)
	}

	if s.modalRenaming != "" {
		fieldR := rl.NewRectangle(r.X+16, r.Y+r.Height-72, r.Width-32, 28)
		drawTextField(font, fieldR, s.modalRenaming, true)
		rl.DrawTextEx(font, "New name (without .map)   Enter rename   Esc cancel",
			rl.NewVector2(r.X+16, r.Y+r.Height-26), 12, 1, theme.TextHint)
		return
	}
	if s.modalConfirmDelete {
		path := s.modalPaths[s.modalCursor]
		rl.DrawTextEx(font, fmt.Sprintf("Delete %s? This is permanent.", core.MapIDFromPath(path)),
			rl.NewVector2(r.X+16, r.Y+r.Height-72), 14, 1, theme.BorderDanger)
		rl.DrawTextEx(font, "Y delete   N / Esc cancel",
			rl.NewVector2(r.X+16, r.Y+r.Height-26), 12, 1, theme.TextHint)
		return
	}

	rl.DrawTextEx(font, "Up/Down nav   Enter open   R rename   D delete   C duplicate   Esc cancel",
		rl.NewVector2(r.X+16, r.Y+r.Height-26), 12, 1, theme.TextHint)
}

func saveAsFieldRect(s *State) rl.Rectangle {
	w, h := render.ScreenSizeF()
	pw := float32(420)
	ph := float32(160)
	r := rl.NewRectangle((w-pw)/2, (h-ph)/2, pw, ph)
	return rl.NewRectangle(r.X+16, r.Y+58, pw-32, 28)
}

func drawSaveAsModal(s *State, font rl.Font, theme render.Theme) {
	accent := theme.BorderStrong
	title := "SAVE MAP AS"
	if s.awaitingOverwrite {
		accent = theme.BorderDanger
		title = "FILE EXISTS"
	}
	r := drawModalHeader(font, theme, 420, 160, title, accent)

	if s.awaitingOverwrite {
		rl.DrawTextEx(font, fmt.Sprintf("Overwrite %s?", core.MapPath(s.modalFilename)),
			rl.NewVector2(r.X+16, r.Y+44), 14, 1, theme.TextPrimary)
		render.DrawTextWithShadow(font, "Y  Overwrite", r.X+24, r.Y+78, 14, theme.BorderDanger)
		render.DrawTextWithShadow(font, "N / Esc  Pick a different name", r.X+24, r.Y+100, 14, theme.TextMuted)
		return
	}

	rl.DrawTextEx(font, "Filename (without .map):", rl.NewVector2(r.X+16, r.Y+40), 12, 1, theme.TextLabel)

	field := saveAsFieldRect(s)
	drawTextField(font, field, s.modalFilename, true)
	// Preview the sanitized path: MapPath strips a trailing .map, and the
	// disk store goes through sanitizeFilename on commit. Show the
	// final-form path so the user knows what they'll actually get — and
	// flag any divergence between what they typed and what will land.
	sanitized := sanitizeFilename(s.modalFilename)
	previewPath := core.MapPath(sanitized)
	rl.DrawTextEx(font, fmt.Sprintf("Will save to: %s", previewPath),
		rl.NewVector2(r.X+16, r.Y+96), 12, 1, theme.TextMuted)
	if sanitized != strings.TrimSuffix(strings.TrimSuffix(s.modalFilename, ".map"), ".MAP") {
		rl.DrawTextEx(font, "(Punctuation and spaces are stripped)",
			rl.NewVector2(r.X+16, r.Y+112), 11, 1, theme.BorderDanger)
	}
	rl.DrawTextEx(font, "Enter save   Esc cancel",
		rl.NewVector2(r.X+16, r.Y+r.Height-26), 12, 1, theme.TextHint)
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
	r := drawModalHeader(font, theme, 380, 320,
		"PACK AT "+core.TileCoord(pack.TileX, pack.TileZ),
		theme.BorderActive)

	if len(pack.Members) == 0 {
		rl.DrawTextEx(font, "(empty — close to drop)",
			rl.NewVector2(r.X+16, r.Y+52), 14, 1, theme.TextHint)
	}
	const rowH = float32(22)
	for i, kind := range pack.Members {
		text := "?"
		if name, ok := core.EnemyKindName(kind); ok {
			text = name
		}
		col := theme.TextMuted
		if i == s.modalCursor {
			col = theme.BorderActive
			text = "> " + text
		}
		render.DrawTextWithShadow(font, text,
			r.X+24, r.Y+52+float32(i)*rowH,
			16, col)
	}

	// Leader hint: the rendered field icon for the pack is the
	// highest-Tier member (core.PackLeaderKindSlot). Tell the author so
	// they can understand which member's silhouette shows in-world.
	leaderText := "Leader: —"
	if len(pack.Members) > 0 {
		leaderIdx := core.PackLeaderKindSlot(pack.Members)
		if leaderIdx >= 0 && leaderIdx < len(pack.Members) {
			if name, ok := core.EnemyKindName(pack.Members[leaderIdx]); ok {
				leaderText = "Leader (highest tier): " + name
			}
		}
	}
	rl.DrawTextEx(font, leaderText,
		rl.NewVector2(r.X+16, r.Y+r.Height-60), 12, 1, theme.TextMuted)

	rl.DrawTextEx(font, hintPackEditNav,
		rl.NewVector2(r.X+16, r.Y+r.Height-42), 12, 1, theme.TextHint)
	// Build the add-shortcuts hint from packAddRules so display stays in
	// sync with the input handler; adding a new enemy kind is one row
	// in packAddRules and both the keymap and this label update.
	addLabels := make([]string, 0, len(packAddRules))
	for _, rule := range packAddRules {
		addLabels = append(addLabels, rule.Label)
	}
	rl.DrawTextEx(font, joinHintLabels(addLabels, hintEscClose),
		rl.NewVector2(r.X+16, r.Y+r.Height-24), 12, 1, theme.TextHint)
}

// drawChestEditModal renders the inline chest editor: header with
// chest coords, the authored item list with the cursor highlighting
// one entry, and a hint row at the bottom showing the add-item keys
// from chestAddRules. Mirrors drawPackEditModal so the two entity
// editors read as one visual family.
func drawChestEditModal(s *State, font rl.Font, theme render.Theme) {
	if s.modalChestIdx < 0 || s.modalChestIdx >= len(s.area.ChestSpawns) {
		return
	}
	chest := s.area.ChestSpawns[s.modalChestIdx]
	r := drawModalHeader(font, theme, 380, 320,
		"CHEST AT "+core.TileCoord(chest.TileX, chest.TileZ),
		theme.BorderActive)

	if len(chest.Items) == 0 {
		rl.DrawTextEx(font, "(empty — adds reveal it as pre-looted in game)",
			rl.NewVector2(r.X+16, r.Y+52), 14, 1, theme.TextHint)
	}
	const rowH = float32(22)
	for i, kind := range chest.Items {
		info := core.ItemInfo(kind)
		text := info.Name
		col := theme.TextMuted
		if i == s.modalCursor {
			col = theme.BorderActive
			text = "> " + text
		}
		render.DrawTextWithShadow(font, text,
			r.X+24, r.Y+52+float32(i)*rowH,
			16, col)
	}

	rl.DrawTextEx(font, hintChestEditNav,
		rl.NewVector2(r.X+16, r.Y+r.Height-42), 12, 1, theme.TextHint)
	addLabels := make([]string, 0, len(chestAddRules))
	for _, rule := range chestAddRules {
		addLabels = append(addLabels, rule.Label)
	}
	rl.DrawTextEx(font, joinHintLabels(addLabels, hintEscClose),
		rl.NewVector2(r.X+16, r.Y+r.Height-24), 12, 1, theme.TextHint)
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
	drawModalVeil(theme)
	render.DrawCard(int32(l.card.X), int32(l.card.Y), int32(l.card.Width), int32(l.card.Height),
		theme.SurfacePrimary, theme.BorderSoft, theme.BorderActive)
	header := "DOOR AT " + core.TileCoord(door.TileX, door.TileZ)
	render.DrawHeading(font, header, int32(l.card.X+16), int32(l.card.Y+12), theme.BorderActive)

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
	drawLabel(font, "Facing on exit (player walks out heading this way)",
		rl.NewRectangle(l.facing[0].X, l.facing[0].Y-16, l.facing[3].X+l.facing[3].Width-l.facing[0].X, 14))
	for i, fr := range l.facing {
		drawButton(font, fr, core.FacingShortLabels[i], door.Facing == i)
	}

	// Delete + Close buttons.
	drawButton(font, l.deleteBtn, "Delete door (X)", false)
	drawButton(font, l.closeBtn, "Done (Esc)", false)

	// Footer hint string mirrors the other modals' tiny hint row.
	hint := "Tab cycle fields   N/E/S/W set facing   X delete   Esc / Enter done"
	rl.DrawTextEx(font, hint,
		rl.NewVector2(l.card.X+16, l.card.Y+l.card.Height-72),
		11, 1, theme.TextHint)
}

// drawValidateModal renders the full reachability + cross-map door
// warning list captured at modal-open time. Read-only viewer; any
// keystroke dismisses.
func drawValidateModal(s *State, font rl.Font, theme render.Theme) {
	rows := s.modalValidateRows
	pw := float32(560)
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
			rl.NewVector2(r.X+16, r.Y+50), 16, 1, theme.BorderStrong)
	} else {
		y := r.Y + 50
		for _, line := range rows {
			rl.DrawTextEx(font, "! "+line,
				rl.NewVector2(r.X+16, y), 13, 1, theme.BorderDanger)
			y += 22
		}
	}
	rl.DrawTextEx(font, "Esc / Enter / click   close",
		rl.NewVector2(r.X+16, r.Y+r.Height-26), 12, 1, theme.TextHint)
}

func drawConfirmDirtyModal(s *State, font rl.Font, theme render.Theme) {
	r := drawModalHeader(font, theme, 460, 188, "UNSAVED CHANGES", theme.BorderActive)

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

	rl.DrawTextEx(font, body, rl.NewVector2(r.X+16, r.Y+44), 14, 1, theme.TextPrimary)

	render.DrawTextWithShadow(font, saveLabel, r.X+24, r.Y+80, 14, theme.BorderStrong)
	render.DrawTextWithShadow(font, discardLabel, r.X+24, r.Y+102, 14, theme.BorderDanger)
	render.DrawTextWithShadow(font, "Esc  Cancel", r.X+24, r.Y+124, 14, theme.TextMuted)
}

package editor

import (
	"crawler/internal/app/core"
	"crawler/internal/app/render"
	"fmt"
	"image/color"
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
)

const (
	topbarH    = float32(40)
	paletteW   = float32(170)
	metadataW  = float32(290)
	gridMargin = float32(8)
)

// layout recomputes screen rectangles each frame from the current window
// size. Cell pixel size is the auto-fit size scaled by s.zoom; pan offsets
// nudge the plot off-center so users can drag around large maps.
func (s *State) layout() {
	w := float32(rl.GetScreenWidth())
	h := float32(rl.GetScreenHeight())

	s.rect.topbar = rl.NewRectangle(0, 0, w, topbarH)
	s.rect.palette = rl.NewRectangle(0, topbarH, paletteW, h-topbarH)
	s.rect.metadata = rl.NewRectangle(w-metadataW, topbarH, metadataW, h-topbarH)
	s.rect.grid = rl.NewRectangle(paletteW, topbarH, w-paletteW-metadataW, h-topbarH)

	cols := len(s.area.Layout)
	if cols == 0 {
		s.rect.cellPx = 0
		return
	}
	mw := len(s.area.Layout[0])
	mh := len(s.area.Layout)
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
	if x < 0 || z < 0 || x >= len(s.area.Layout[0]) || z >= len(s.area.Layout) {
		return -1, -1
	}
	return x, z
}

// Draw paints the editor view. Must be called inside Begin/EndDrawing.
func Draw(s *State, assets render.Resources) {
	font := assets.Font()
	theme := assets.Theme()
	s.layout()
	rl.ClearBackground(rl.NewColor(20, 22, 30, 255))
	drawTopbar(s, font, theme)
	drawPalette(s, font, theme)
	drawMetadata(s, font, theme)
	drawGrid(s, font)
	if len(s.statusLog) > 0 {
		drawStatus(s, font, theme)
	}
	if s.modal == modalOpen {
		drawOpenModal(s, font, theme)
	}
	if s.modal == modalSaveAs {
		drawSaveAsModal(s, font, theme)
	}
	if s.modal == modalConfirmDirty {
		drawConfirmDirtyModal(s, font, theme)
	}
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
	rl.DrawLineEx(rl.NewVector2(0, topbarH), rl.NewVector2(s.rect.topbar.Width, topbarH), 1, rl.NewColor(8, 10, 14, 255))

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

	// Hover coords + brush size + zoom on the right-of-buttons strip.
	coord := "—"
	if s.hoverX >= 0 {
		coord = fmt.Sprintf("(%d, %d)", s.hoverX, s.hoverZ)
	}
	infoLabel := fmt.Sprintf("cell %s   brush %dx%d   zoom %.0f%%", coord, s.brushSize, s.brushSize, s.zoom*100)
	infoMeasure := rl.MeasureTextEx(font, infoLabel, 13, 1)
	infoX := labelX - infoMeasure.X - 24
	render.DrawTextWithShadow(font, infoLabel, infoX, (topbarH-infoMeasure.Y)/2, 13, theme.TextHint)
}

func buttonWidth(label string) float32 {
	switch label {
	case "Save As":
		return 80
	case "Back":
		return 60
	default:
		return 64
	}
}

func drawButton(font rl.Font, r rl.Rectangle, label string, active bool) {
	bg := rl.NewColor(48, 54, 70, 255)
	border := rl.NewColor(96, 108, 132, 255)
	text := rl.NewColor(220, 230, 245, 255)
	if active {
		bg = rl.NewColor(72, 88, 130, 255)
		border = rl.NewColor(180, 220, 244, 255)
	}
	if pointIn(rl.GetMousePosition(), r) {
		bg = rl.NewColor(60, 70, 90, 255)
	}
	rl.DrawRectangleRec(r, bg)
	rl.DrawRectangleLinesEx(r, 1, border)
	measure := rl.MeasureTextEx(font, label, 14, 1)
	rl.DrawTextEx(font, label,
		rl.NewVector2(r.X+(r.Width-measure.X)/2, r.Y+(r.Height-measure.Y)/2),
		14, 1, text)
}

// --- Palette ---------------------------------------------------------------

func paletteToolAt(s *State, p rl.Vector2) int {
	if !pointIn(p, s.rect.palette) {
		return -1
	}
	for i := range toolEntries {
		r := paletteEntryRect(s, i)
		if pointIn(p, r) {
			return i
		}
	}
	return -1
}

func paletteEntryRect(s *State, i int) rl.Rectangle {
	const rowH = float32(36)
	y := s.rect.palette.Y + 12 + float32(i)*(rowH+4)
	return rl.NewRectangle(s.rect.palette.X+8, y, s.rect.palette.Width-16, rowH)
}

func drawPalette(s *State, font rl.Font, theme render.Theme) {
	rl.DrawRectangleRec(s.rect.palette, rl.NewColor(24, 28, 38, 255))
	rl.DrawLineEx(
		rl.NewVector2(s.rect.palette.X+s.rect.palette.Width, s.rect.palette.Y),
		rl.NewVector2(s.rect.palette.X+s.rect.palette.Width, s.rect.palette.Y+s.rect.palette.Height),
		1, rl.NewColor(8, 10, 14, 255))

	render.DrawHeading(font, "TOOLS", int32(s.rect.palette.X+12), int32(s.rect.palette.Y+8), theme.BorderStrong)

	for i, t := range toolEntries {
		r := paletteEntryRect(s, i)
		active := s.tool == t.tool
		bg := rl.NewColor(36, 40, 52, 255)
		if active {
			bg = rl.NewColor(72, 88, 130, 255)
		}
		if pointIn(rl.GetMousePosition(), r) {
			bg = rl.NewColor(48, 56, 72, 255)
		}
		rl.DrawRectangleRec(r, bg)
		border := rl.NewColor(70, 80, 100, 255)
		if active {
			border = rl.NewColor(180, 220, 244, 255)
		}
		rl.DrawRectangleLinesEx(r, 1, border)

		// Color swatch on the left edge so the player learns which color
		// each brush paints on the grid.
		swatch := rl.NewRectangle(r.X+6, r.Y+6, 20, r.Height-12)
		rl.DrawRectangleRec(swatch, t.color)
		rl.DrawRectangleLinesEx(swatch, 1, rl.NewColor(0, 0, 0, 200))

		txt := fmt.Sprintf("%d %s", i+1, t.label)
		rl.DrawTextEx(font, txt, rl.NewVector2(r.X+34, r.Y+(r.Height-14)/2), 14, 1, rl.NewColor(230, 234, 244, 255))
	}

	hints := []string{
		"L-drag: paint",
		"R-click: erase",
		"Shift+drag: rect",
		"Ctrl+click: fill",
		"[ ] brush size",
		"arrows: cursor",
		"space: paint",
		"wheel: zoom",
		"mid-drag: pan",
		"home: reset view",
		"Ctrl+S save",
		"Ctrl+O open",
		"Ctrl+Z undo",
		"Ctrl+Y redo",
		"Ctrl+N new",
		"F5 playtest",
		"R rotate start",
		"Esc back",
	}
	y := s.rect.palette.Y + 16 + float32(len(toolEntries))*40 + 12
	for _, h := range hints {
		rl.DrawTextEx(font, h, rl.NewVector2(s.rect.palette.X+12, y), 11, 1, theme.TextHint)
		y += 14
	}
}

// --- Metadata panel --------------------------------------------------------

type metaRect struct {
	nameLabel, nameField     rl.Rectangle
	matLabel                 rl.Rectangle
	matButtons               []rl.Rectangle
	quietLabel, quietField   rl.Rectangle
	dimsLabel                rl.Rectangle
	widthValue, widthMinus, widthPlus    rl.Rectangle
	heightValue, heightMinus, heightPlus rl.Rectangle
	startLabel, startInfo                rl.Rectangle
	facingLabel                          rl.Rectangle
	facingButtons                        []rl.Rectangle
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
	return r
}

func handleMetadataClick(s *State, p rl.Vector2) bool {
	if !pointIn(p, s.rect.metadata) {
		return false
	}
	mr := metadataRects(s)
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
	mw := len(s.area.Layout[0])
	mh := len(s.area.Layout)
	// Clicking the W or H value enters numeric input mode for direct typing.
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
		resize(s, mw-1, mh)
		return true
	}
	if pointIn(p, mr.widthPlus) {
		resize(s, mw+1, mh)
		return true
	}
	if pointIn(p, mr.heightMinus) {
		resize(s, mw, mh-1)
		return true
	}
	if pointIn(p, mr.heightPlus) {
		resize(s, mw, mh+1)
		return true
	}
	for i, br := range mr.facingButtons {
		if pointIn(p, br) {
			s.area.StartFacing = i // North=0, East=1, South=2, West=3
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
		1, rl.NewColor(8, 10, 14, 255))

	render.DrawHeading(font, "MAP", int32(s.rect.metadata.X+12), int32(s.rect.metadata.Y+8), theme.BorderStrong)

	mr := metadataRects(s)

	drawLabel(font, "Name", mr.nameLabel)
	drawTextField(font, mr.nameField, s.area.Name, s.focus == focusName)

	drawLabel(font, "Materials", mr.matLabel)
	for i, br := range mr.matButtons {
		active := s.area.Materials == core.MaterialOptions[i]
		drawButton(font, br, core.MaterialName(core.MaterialOptions[i]), active)
	}

	drawLabel(font, "Quiet message", mr.quietLabel)
	drawTextField(font, mr.quietField, s.area.QuietMessage, s.focus == focusQuiet)

	mw := len(s.area.Layout[0])
	mh := len(s.area.Layout)
	drawLabel(font, "Dimensions (click to type)", mr.dimsLabel)
	wText := fmt.Sprintf("W: %d", mw)
	if s.focus == focusWidth {
		wText = "W: " + s.numericBuf
	}
	hText := fmt.Sprintf("H: %d", mh)
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
	drawReadonlyValue(font, mr.startInfo, fmt.Sprintf("(%d, %d)", s.area.StartTileX, s.area.StartTileZ))

	drawLabel(font, "Facing (R cycles)", mr.facingLabel)
	labels := []string{"N", "E", "S", "W"}
	for i, br := range mr.facingButtons {
		drawButton(font, br, labels[i], s.area.StartFacing == i)
	}
}

func drawLabel(font rl.Font, text string, r rl.Rectangle) {
	rl.DrawTextEx(font, text, rl.NewVector2(r.X, r.Y), 12, 1, rl.NewColor(138, 160, 188, 220))
}

func drawTextField(font rl.Font, r rl.Rectangle, text string, focused bool) {
	bg := rl.NewColor(14, 16, 22, 255)
	border := rl.NewColor(70, 80, 100, 255)
	if focused {
		border = rl.NewColor(180, 220, 244, 255)
	}
	rl.DrawRectangleRec(r, bg)
	rl.DrawRectangleLinesEx(r, 1, border)

	render := text
	if focused {
		// Half-second blink. Alternates "_" and " " (same width) so the
		// trailing visual cursor doesn't shift the rest of the field.
		if math.Mod(rl.GetTime(), 1.0) > 0.5 {
			render += "_"
		} else {
			render += " "
		}
	}
	rl.DrawTextEx(font, render, rl.NewVector2(r.X+8, r.Y+(r.Height-14)/2), 14, 1, rl.NewColor(230, 234, 244, 255))
}

func drawReadonlyValue(font rl.Font, r rl.Rectangle, text string) {
	rl.DrawRectangleRec(r, rl.NewColor(14, 16, 22, 255))
	rl.DrawRectangleLinesEx(r, 1, rl.NewColor(50, 58, 76, 255))
	rl.DrawTextEx(font, text, rl.NewVector2(r.X+8, r.Y+(r.Height-14)/2), 14, 1, rl.NewColor(200, 210, 230, 255))
}

// --- Grid ------------------------------------------------------------------

func drawGrid(s *State, font rl.Font) {
	rl.DrawRectangleRec(s.rect.grid, rl.NewColor(14, 16, 22, 255))
	if s.rect.cellPx <= 0 {
		return
	}
	cell := s.rect.cellPx
	for z, row := range s.area.Layout {
		for x := 0; x < len(row); x++ {
			r := rl.NewRectangle(s.rect.gridX+float32(x)*cell, s.rect.gridY+float32(z)*cell, cell, cell)
			rl.DrawRectangleRec(r, tileColor(row[x]))
		}
	}
	// Light grid lines so cell boundaries are visible at any zoom.
	gridLine := rl.NewColor(0, 0, 0, 80)
	for x := 0; x <= len(s.area.Layout[0]); x++ {
		px := s.rect.gridX + float32(x)*cell
		rl.DrawLineEx(rl.NewVector2(px, s.rect.gridY), rl.NewVector2(px, s.rect.gridY+s.rect.gridH), 1, gridLine)
	}
	for z := 0; z <= len(s.area.Layout); z++ {
		py := s.rect.gridY + float32(z)*cell
		rl.DrawLineEx(rl.NewVector2(s.rect.gridX, py), rl.NewVector2(s.rect.gridX+s.rect.gridW, py), 1, gridLine)
	}

	// Enemy spawn dots
	for _, sp := range s.area.EnemySpawns {
		cx := s.rect.gridX + (float32(sp.TileX)+0.5)*cell
		cy := s.rect.gridY + (float32(sp.TileZ)+0.5)*cell
		col := rl.NewColor(220, 156, 96, 255)
		if sp.Kind == core.EnemyBat {
			col = rl.NewColor(160, 130, 220, 255)
		}
		rl.DrawCircle(int32(cx), int32(cy), cell*0.32, col)
		rl.DrawCircleLines(int32(cx), int32(cy), cell*0.32, rl.NewColor(0, 0, 0, 220))
		// Letter label on top.
		label := "R"
		if sp.Kind == core.EnemyBat {
			label = "B"
		}
		measure := rl.MeasureTextEx(font, label, cell*0.42, 1)
		rl.DrawTextEx(font, label,
			rl.NewVector2(cx-measure.X/2, cy-measure.Y/2),
			cell*0.42, 1, rl.NewColor(0, 0, 0, 230))
	}

	// Player start marker — diamond with a facing tick.
	sx := s.rect.gridX + (float32(s.area.StartTileX)+0.5)*cell
	sy := s.rect.gridY + (float32(s.area.StartTileZ)+0.5)*cell
	startCol := rl.NewColor(255, 220, 124, 255)
	rl.DrawCircle(int32(sx), int32(sy), cell*0.36, startCol)
	rl.DrawCircleLines(int32(sx), int32(sy), cell*0.36, rl.NewColor(0, 0, 0, 220))
	dx, dz := core.FacingVector(s.area.StartFacing)
	tx := sx + float32(dx)*cell*0.42
	ty := sy + float32(dz)*cell*0.42
	rl.DrawLineEx(rl.NewVector2(sx, sy), rl.NewVector2(tx, ty), 3, rl.NewColor(20, 14, 0, 255))

	// Brush ghost / hover highlight. Shows the area that the next click
	// (or Space, when keyboard cursor active) will paint.
	hoverPx := s.hoverX
	hoverPz := s.hoverZ
	if s.gridCursorX >= 0 {
		hoverPx, hoverPz = s.gridCursorX, s.gridCursorZ
	}
	if hoverPx >= 0 {
		half := s.brushSize / 2
		if !isTileBrush(s.tool) {
			half = 0
		}
		x0 := hoverPx - half
		z0 := hoverPz - half
		side := float32(half*2 + 1)
		r := rl.NewRectangle(s.rect.gridX+float32(x0)*cell, s.rect.gridY+float32(z0)*cell, cell*side, cell*side)
		rl.DrawRectangleLinesEx(r, 2, rl.NewColor(255, 255, 255, 200))
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
		fill := tileColor(toolEntries[s.tool].tileByte)
		fill.A = 110
		rl.DrawRectangleRec(r, fill)
		rl.DrawRectangleLinesEx(r, 2, rl.NewColor(255, 255, 255, 220))
	}

	// Drag-move ghost for player start / enemy.
	if s.drag == dragStart && s.hoverX >= 0 {
		gx := s.rect.gridX + (float32(s.hoverX)+0.5)*cell
		gy := s.rect.gridY + (float32(s.hoverZ)+0.5)*cell
		rl.DrawCircleLines(int32(gx), int32(gy), cell*0.36, rl.NewColor(255, 220, 124, 220))
	}
	if s.drag == dragEnemy && s.hoverX >= 0 && s.dragSpawnIdx >= 0 && s.dragSpawnIdx < len(s.area.EnemySpawns) {
		gx := s.rect.gridX + (float32(s.hoverX)+0.5)*cell
		gy := s.rect.gridY + (float32(s.hoverZ)+0.5)*cell
		rl.DrawCircleLines(int32(gx), int32(gy), cell*0.32, rl.NewColor(255, 255, 255, 220))
	}
}

func tileColor(b byte) color.RGBA {
	switch b {
	case core.TileFloor:
		return rl.NewColor(180, 168, 140, 255)
	case core.TileRock:
		return rl.NewColor(96, 96, 110, 255)
	case core.TileTree:
		return rl.NewColor(64, 140, 80, 255)
	case core.TileTreeXL:
		return rl.NewColor(36, 96, 56, 255)
	case core.TileRockLarge:
		return rl.NewColor(132, 110, 90, 255)
	case core.TileBushLarge:
		return rl.NewColor(112, 142, 70, 255)
	}
	return rl.NewColor(40, 40, 50, 255)
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
		// Fade out as the timer drains so the user can see it expiring.
		alpha := e.timer / 1.5
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
	w := int32(rl.GetScreenWidth())
	h := int32(rl.GetScreenHeight())
	rl.DrawRectangle(0, 0, w, h, theme.SurfaceVeil)
}

func drawModalCard(theme render.Theme, pw, ph float32, accent rl.Color) rl.Rectangle {
	w := float32(rl.GetScreenWidth())
	h := float32(rl.GetScreenHeight())
	r := rl.NewRectangle((w-pw)/2, (h-ph)/2, pw, ph)
	render.DrawCard(int32(r.X), int32(r.Y), int32(r.Width), int32(r.Height),
		theme.SurfacePrimary, theme.BorderSoft, accent)
	return r
}

func drawOpenModal(s *State, font rl.Font, theme render.Theme) {
	drawModalVeil(theme)
	r := drawModalCard(theme, 460, 460, theme.BorderStrong)

	header := "OPEN MAP"
	if s.modalRenaming != "" {
		header = "RENAME MAP"
	} else if s.modalConfirmDelete {
		header = "DELETE MAP"
	}
	render.DrawHeading(font, header, int32(r.X+16), int32(r.Y+12), theme.BorderStrong)

	if len(s.modalPaths) == 0 {
		rl.DrawTextEx(font, "(no .map files in maps/)", rl.NewVector2(r.X+16, r.Y+50), 14, 1, theme.TextMuted)
		rl.DrawTextEx(font, "Esc to close", rl.NewVector2(r.X+16, r.Y+r.Height-26), 12, 1, theme.TextHint)
		return
	}
	for i, path := range s.modalPaths {
		text := core.MapIDFromPath(path)
		col := theme.TextMuted
		if i == s.modalCursor {
			col = theme.BorderActive
			text = "> " + text
		}
		render.DrawTextWithShadow(font, text, r.X+18, r.Y+50+float32(i)*22, 16, col)
	}

	if s.modalRenaming != "" {
		// Inline rename input drawn below the list, anchored to bottom.
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
	w := float32(rl.GetScreenWidth())
	h := float32(rl.GetScreenHeight())
	pw := float32(420)
	ph := float32(160)
	r := rl.NewRectangle((w-pw)/2, (h-ph)/2, pw, ph)
	return rl.NewRectangle(r.X+16, r.Y+58, pw-32, 28)
}

func drawSaveAsModal(s *State, font rl.Font, theme render.Theme) {
	drawModalVeil(theme)
	accent := theme.BorderStrong
	if s.awaitingOverwrite {
		accent = theme.BorderDanger
	}
	r := drawModalCard(theme, 420, 160, accent)

	if s.awaitingOverwrite {
		render.DrawHeading(font, "FILE EXISTS", int32(r.X+16), int32(r.Y+12), theme.BorderDanger)
		rl.DrawTextEx(font, fmt.Sprintf("Overwrite %s?", core.MapPath(s.modalFilename)),
			rl.NewVector2(r.X+16, r.Y+44), 14, 1, theme.TextPrimary)
		render.DrawTextWithShadow(font, "Y  Overwrite", r.X+24, r.Y+78, 14, theme.BorderDanger)
		render.DrawTextWithShadow(font, "N / Esc  Pick a different name", r.X+24, r.Y+100, 14, theme.TextMuted)
		return
	}

	render.DrawHeading(font, "SAVE MAP AS", int32(r.X+16), int32(r.Y+12), theme.BorderStrong)
	rl.DrawTextEx(font, "Filename (without .map):", rl.NewVector2(r.X+16, r.Y+40), 12, 1, theme.TextLabel)

	field := saveAsFieldRect(s)
	drawTextField(font, field, s.modalFilename, true)
	rl.DrawTextEx(font, fmt.Sprintf("Will save to: %s", core.MapPath(s.modalFilename)),
		rl.NewVector2(r.X+16, r.Y+96), 12, 1, theme.TextMuted)
	rl.DrawTextEx(font, "Enter save   Esc cancel",
		rl.NewVector2(r.X+16, r.Y+r.Height-26), 12, 1, theme.TextHint)
}

func drawConfirmDirtyModal(s *State, font rl.Font, theme render.Theme) {
	drawModalVeil(theme)
	r := drawModalCard(theme, 460, 188, theme.BorderActive)

	render.DrawHeading(font, "UNSAVED CHANGES", int32(r.X+16), int32(r.Y+12), theme.BorderActive)

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

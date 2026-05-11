package editor

import (
	"crawler/internal/app/core"
	"crawler/internal/app/core/mapfile"
	"os"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// updateHotkeys handles keyboard shortcuts when no text field is focused.
func updateHotkeys(s *State) {
	// 1..9 select a brush within the active layer's palette. Layers with
	// fewer than 9 brushes simply ignore the higher numbers.
	palette := layerBrushes[s.layer]
	for i, b := range palette {
		if rl.IsKeyPressed(b.Hotkey) {
			s.brushIdx[s.layer] = i
		}
	}

	ctrl := rl.IsKeyDown(rl.KeyLeftControl) || rl.IsKeyDown(rl.KeyRightControl)
	shift := rl.IsKeyDown(rl.KeyLeftShift) || rl.IsKeyDown(rl.KeyRightShift)

	switch {
	case ctrl && shift && rl.IsKeyPressed(rl.KeyZ):
		redoOne(s)
	case ctrl && rl.IsKeyPressed(rl.KeyZ):
		undoOne(s)
	case ctrl && rl.IsKeyPressed(rl.KeyY):
		redoOne(s)
	case ctrl && rl.IsKeyPressed(rl.KeyS):
		saveCurrent(s)
	case ctrl && rl.IsKeyPressed(rl.KeyO):
		requestOpen(s)
	case ctrl && rl.IsKeyPressed(rl.KeyN):
		newMap(s)
	}

	// Tab cycles to the next layer (Shift+Tab to the previous).
	if !ctrl && rl.IsKeyPressed(rl.KeyTab) {
		dir := 1
		if shift {
			dir = -1
		}
		s.layer = Layer((int(s.layer) + dir + layerCount) % layerCount)
	}

	// F5: launch a playtest of the current in-memory area without saving.
	if rl.IsKeyPressed(rl.KeyF5) {
		s.testRequested = true
	}

	// Brush size cycling (only for grid layers — non-grid is always size 1).
	if !ctrl && rl.IsKeyPressed(rl.KeyLeftBracket) {
		s.brushSize = stepBrush(s.brushSize, -1)
	}
	if !ctrl && rl.IsKeyPressed(rl.KeyRightBracket) {
		s.brushSize = stepBrush(s.brushSize, +1)
	}

	if !ctrl && rl.IsKeyPressed(rl.KeyHome) {
		s.zoom = 1
		s.panX, s.panY = 0, 0
	}

	// Cycle starting facing for the player-start brush with R. Gated to
	// that brush so R doesn't silently rotate the start while the user
	// thinks they're in another layer.
	if !ctrl && rl.IsKeyPressed(rl.KeyR) && s.layer == LayerEntities && s.activeBrush().Entity == entityPlayerStart {
		s.area.StartFacing = core.NormalizeFacing(s.area.StartFacing + 1)
		s.dirty = true
	}

	// T cycles the day/night preview phase. Shows in the top bar and seeds
	// StepCount on F5 so the playtest drops into that phase.
	if !ctrl && rl.IsKeyPressed(rl.KeyT) {
		s.previewPhase = core.TimeOfDay((int(s.previewPhase) + 1) % 6)
		s.flash("Preview: " + core.PhaseName(s.previewPhase))
	}

	updateGridCursor(s)
}

func stepBrush(cur, dir int) int {
	steps := []int{1, 3, 5}
	idx := 0
	for i, v := range steps {
		if v == cur {
			idx = i
			break
		}
	}
	idx += dir
	if idx < 0 {
		idx = 0
	}
	if idx >= len(steps) {
		idx = len(steps) - 1
	}
	return steps[idx]
}

func updateGridCursor(s *State) {
	if s.area.Width == 0 || s.area.Height == 0 {
		return
	}
	mw := s.area.Width
	mh := s.area.Height
	moved := false
	if rl.IsKeyPressed(rl.KeyLeft) {
		s.gridCursorX, s.gridCursorZ = activateCursor(s, mw, mh)
		s.gridCursorX = clampInt(s.gridCursorX-1, 0, mw-1)
		moved = true
	}
	if rl.IsKeyPressed(rl.KeyRight) {
		s.gridCursorX, s.gridCursorZ = activateCursor(s, mw, mh)
		s.gridCursorX = clampInt(s.gridCursorX+1, 0, mw-1)
		moved = true
	}
	if rl.IsKeyPressed(rl.KeyUp) {
		s.gridCursorX, s.gridCursorZ = activateCursor(s, mw, mh)
		s.gridCursorZ = clampInt(s.gridCursorZ-1, 0, mh-1)
		moved = true
	}
	if rl.IsKeyPressed(rl.KeyDown) {
		s.gridCursorX, s.gridCursorZ = activateCursor(s, mw, mh)
		s.gridCursorZ = clampInt(s.gridCursorZ+1, 0, mh-1)
		moved = true
	}
	if moved && s.gridCursorX >= 0 {
		s.hoverX, s.hoverZ = s.gridCursorX, s.gridCursorZ
	}
	if s.gridCursorX < 0 {
		return
	}
	if rl.IsKeyPressed(rl.KeySpace) {
		pushUndo(s)
		applyToolBrushed(s, s.gridCursorX, s.gridCursorZ)
	}
	if rl.IsKeyPressed(rl.KeyBackspace) {
		pushUndo(s)
		eraseAt(s, s.gridCursorX, s.gridCursorZ)
	}
}

func activateCursor(s *State, mw, mh int) (int, int) {
	if s.gridCursorX >= 0 {
		return s.gridCursorX, s.gridCursorZ
	}
	x := clampInt(s.area.StartTileX, 0, mw-1)
	z := clampInt(s.area.StartTileZ, 0, mh-1)
	return x, z
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// updateMouse processes top-bar / palette / metadata clicks and grid
// painting. Called every frame outside of modals and text-focus mode.
func updateMouse(s *State) {
	mp := rl.GetMousePosition()

	hx, hz := s.cellAt(mp)
	s.hoverX, s.hoverZ = hx, hz

	if pointIn(mp, s.rect.grid) {
		w := rl.GetMouseWheelMove()
		if w != 0 {
			zoomBy(s, mp, 1+0.12*w)
		}
	}

	if rl.IsMouseButtonPressed(rl.MouseMiddleButton) && pointIn(mp, s.rect.grid) {
		s.panning = true
	}
	if s.panning && rl.IsMouseButtonDown(rl.MouseMiddleButton) {
		d := rl.GetMouseDelta()
		s.panX += d.X
		s.panY += d.Y
	}
	if rl.IsMouseButtonReleased(rl.MouseMiddleButton) {
		s.panning = false
	}

	if rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		if hit := topbarButtonAt(s, mp); hit != "" {
			handleTopbarButton(s, hit)
			return
		}
		if hit := layerTabAt(s, mp); hit >= 0 {
			s.layer = Layer(hit)
			return
		}
		if hit := paletteToolAt(s, mp); hit >= 0 {
			s.brushIdx[s.layer] = hit
			return
		}
		if handleMetadataClick(s, mp) {
			return
		}
	}

	if pointIn(mp, s.rect.grid) && hx >= 0 {
		ctrl := rl.IsKeyDown(rl.KeyLeftControl) || rl.IsKeyDown(rl.KeyRightControl)
		shift := rl.IsKeyDown(rl.KeyLeftShift) || rl.IsKeyDown(rl.KeyRightShift)

		if rl.IsMouseButtonPressed(rl.MouseLeftButton) {
			startDrag(s, hx, hz, ctrl, shift)
		}
		if rl.IsMouseButtonDown(rl.MouseLeftButton) {
			continueDrag(s, hx, hz)
		}
		if rl.IsMouseButtonPressed(rl.MouseRightButton) {
			pushUndo(s)
			eraseAt(s, hx, hz)
		}
	}

	if rl.IsMouseButtonReleased(rl.MouseLeftButton) {
		finishDrag(s)
	}
}

// startDrag picks a drag kind based on layer + cell contents + modifiers.
// Grid-layer brushes default to paint; Shift switches to rectangle.
// LayerEntities brushes grab the entity under the cursor for drag-move
// when there is one. Ctrl+click on a grid layer is flood fill.
func startDrag(s *State, x, z int, ctrl, shift bool) {
	gridLayer := isGridLayer(s.layer)

	if gridLayer && ctrl {
		pushUndo(s)
		floodFill(s, x, z, s.activeBrush().Char)
		s.drag = dragNone
		return
	}

	if s.layer == LayerEntities {
		brush := s.activeBrush()
		switch brush.Entity {
		case entityPlayerStart:
			if s.area.StartTileX == x && s.area.StartTileZ == z {
				s.drag = dragStart
				return
			}
		case entityAddRat, entityAddBat:
			// Click on an existing pack picks it up for drag-move; click on
			// an empty tile falls through to "add a member" via applyTool.
			if idx := packIndexAt(s.area.PackSpawns, x, z); idx >= 0 {
				s.drag = dragPack
				s.dragPackIdx = idx
				return
			}
		}
		// Fall through: click on empty cell places a fresh entity.
		s.drag = dragPaint
		s.dragSnapshotDone = false
		s.lastPaintX, s.lastPaintZ = -1, -1
		pushUndo(s)
		s.dragSnapshotDone = true
		applyTool(s, x, z)
		s.lastPaintX, s.lastPaintZ = x, z
		return
	}

	if gridLayer && shift {
		s.drag = dragRect
		s.rectAnchorX, s.rectAnchorZ = x, z
		return
	}

	s.drag = dragPaint
	s.dragSnapshotDone = false
	s.lastPaintX, s.lastPaintZ = -1, -1
	pushUndo(s)
	s.dragSnapshotDone = true
	applyToolBrushed(s, x, z)
	s.lastPaintX, s.lastPaintZ = x, z
}

func continueDrag(s *State, x, z int) {
	switch s.drag {
	case dragPaint:
		if x == s.lastPaintX && z == s.lastPaintZ {
			return
		}
		applyToolBrushed(s, x, z)
		s.lastPaintX, s.lastPaintZ = x, z
	}
	// dragStart / dragPack / dragRect are commit-on-release; the live
	// preview lives in draw.go.
}

func finishDrag(s *State) {
	switch s.drag {
	case dragStart:
		if s.hoverX >= 0 && (s.hoverX != s.area.StartTileX || s.hoverZ != s.area.StartTileZ) {
			if s.area.BlockedAt(s.hoverX, s.hoverZ) {
				s.flash("Player start must be on an open cell")
			} else {
				pushUndo(s)
				s.area.StartTileX = s.hoverX
				s.area.StartTileZ = s.hoverZ
				s.dirty = true
			}
		}
	case dragPack:
		if s.hoverX >= 0 && s.dragPackIdx >= 0 && s.dragPackIdx < len(s.area.PackSpawns) {
			sp := s.area.PackSpawns[s.dragPackIdx]
			if sp.TileX != s.hoverX || sp.TileZ != s.hoverZ {
				if s.area.BlockedAt(s.hoverX, s.hoverZ) {
					s.flash("Packs need an open cell")
				} else if s.area.StartTileX == s.hoverX && s.area.StartTileZ == s.hoverZ {
					s.flash("Cell holds the player start")
				} else {
					pushUndo(s)
					// Drop any pack that was already at the destination cell
					// (dragging this one onto another replaces the existing).
					// Then locate the dragged pack by its old coords and move
					// it to the destination. The old-coords lookup works
					// because addPackMember keeps at most one pack per cell.
					s.area.PackSpawns = removePackAt(s.area.PackSpawns, s.hoverX, s.hoverZ)
					idx := packIndexAt(s.area.PackSpawns, sp.TileX, sp.TileZ)
					if idx >= 0 {
						s.area.PackSpawns[idx].TileX = s.hoverX
						s.area.PackSpawns[idx].TileZ = s.hoverZ
					}
					s.dirty = true
				}
			}
		}
	case dragRect:
		if s.hoverX >= 0 {
			pushUndo(s)
			paintRect(s, s.rectAnchorX, s.rectAnchorZ, s.hoverX, s.hoverZ)
		}
	}
	s.drag = dragNone
	s.dragPackIdx = -1
}

// applyToolBrushed runs the active brush over the brush-size square
// centered on (x,z). Entity-layer brushes always collapse to a single
// cell since stamping multiple starts/spawns isn't meaningful.
func applyToolBrushed(s *State, x, z int) {
	half := s.brushSize / 2
	if !isGridLayer(s.layer) || s.brushSize <= 1 {
		applyTool(s, x, z)
		return
	}
	for dz := -half; dz <= half; dz++ {
		for dx := -half; dx <= half; dx++ {
			applyTool(s, x+dx, z+dz)
		}
	}
}

func isGridLayer(l Layer) bool {
	return l != LayerEntities
}

func zoomBy(s *State, anchor rl.Vector2, factor float32) {
	prev := s.zoom
	next := prev * factor
	if next < 0.5 {
		next = 0.5
	}
	if next > 4 {
		next = 4
	}
	if next == prev {
		return
	}
	if s.rect.cellPx > 0 {
		dx := anchor.X - s.rect.gridX
		dy := anchor.Y - s.rect.gridY
		s.panX += dx * (1 - next/prev)
		s.panY += dy * (1 - next/prev)
	}
	s.zoom = next
}

func packIndexAt(packs []core.PackSpawn, x, z int) int {
	for i, sp := range packs {
		if sp.TileX == x && sp.TileZ == z {
			return i
		}
	}
	return -1
}

func handleTopbarButton(s *State, name string) {
	switch name {
	case "new":
		newMap(s)
	case "open":
		requestOpen(s)
	case "save":
		saveCurrent(s)
	case "saveas":
		s.modalFilename = mapStem(s.area.Path)
		s.modal = modalSaveAs
		s.focus = focusFilename
	case "back":
		s.exitRequested = true
	}
}

// pumpPrintableASCII drains queued printable-ASCII characters into target
// (capped at maxLen) and consumes one backspace press. Used by both the
// metadata text fields and the modal rename field — onChange fires once
// per accepted character or backspace and may be nil when no caller-side
// effect is needed.
func pumpPrintableASCII(target *string, maxLen int, onChange func()) {
	for {
		c := rl.GetCharPressed()
		if c == 0 {
			break
		}
		if c >= 32 && c < 127 && len(*target) < maxLen {
			*target += string(rune(c))
			if onChange != nil {
				onChange()
			}
		}
	}
	if rl.IsKeyPressed(rl.KeyBackspace) && len(*target) > 0 {
		*target = (*target)[:len(*target)-1]
		if onChange != nil {
			onChange()
		}
	}
}

func updateTextInput(s *State) {
	if s.focus == focusWidth || s.focus == focusHeight {
		updateNumericInput(s)
		return
	}
	target := activeTextTarget(s)
	if target == nil {
		return
	}
	pumpPrintableASCII(target, 96, s.markDirty)
	if rl.IsKeyPressed(rl.KeyTab) {
		cycleFocus(s)
		return
	}
	if rl.IsKeyPressed(rl.KeyEnter) {
		if s.focus == focusFilename {
			confirmModal(s)
			return
		}
		s.focus = focusNone
	}
	if rl.IsKeyPressed(rl.KeyEscape) {
		if s.focus == focusFilename {
			s.modal = modalNone
		}
		s.focus = focusNone
	}
}

func updateNumericInput(s *State) {
	for {
		c := rl.GetCharPressed()
		if c == 0 {
			break
		}
		if c >= '0' && c <= '9' && len(s.numericBuf) < 4 {
			s.numericBuf += string(rune(c))
		}
	}
	if rl.IsKeyPressed(rl.KeyBackspace) && len(s.numericBuf) > 0 {
		s.numericBuf = s.numericBuf[:len(s.numericBuf)-1]
	}
	if rl.IsKeyPressed(rl.KeyTab) {
		commitNumericInput(s)
		cycleFocus(s)
		return
	}
	if rl.IsKeyPressed(rl.KeyEnter) {
		commitNumericInput(s)
		s.focus = focusNone
	}
	if rl.IsKeyPressed(rl.KeyEscape) {
		s.numericBuf = ""
		s.focus = focusNone
	}
}

func commitNumericInput(s *State) {
	if s.numericBuf == "" {
		return
	}
	v := 0
	for _, c := range s.numericBuf {
		v = v*10 + int(c-'0')
	}
	if v < 1 {
		v = 1
	}
	if v > 200 {
		v = 200
	}
	if s.focus == focusWidth {
		resize(s, v, s.area.Height)
	} else if s.focus == focusHeight {
		resize(s, s.area.Width, v)
	}
	s.numericBuf = ""
}

func cycleFocus(s *State) {
	if s.focus == focusFilename {
		return
	}
	switch s.focus {
	case focusName:
		s.focus = focusQuiet
	case focusQuiet:
		s.focus = focusWidth
		s.numericBuf = ""
	case focusWidth:
		s.focus = focusHeight
		s.numericBuf = ""
	case focusHeight:
		s.focus = focusName
	default:
		s.focus = focusName
	}
}

func (s *State) markDirty() {
	if s.focus == focusFilename {
		return
	}
	s.dirty = true
}

func activeTextTarget(s *State) *string {
	switch s.focus {
	case focusName:
		return &s.area.Name
	case focusQuiet:
		return &s.area.QuietMessage
	case focusFilename:
		return &s.modalFilename
	}
	return nil
}

func updateModal(s *State) Action {
	switch s.modal {
	case modalOpen:
		return updateOpenModal(s)
	case modalSaveAs:
		return updateSaveAsModal(s)
	case modalConfirmDirty:
		return updateConfirmDirtyModal(s)
	}
	return ActionNone
}

func updateOpenModal(s *State) Action {
	if s.modalRenaming != "" {
		return updateOpenRename(s)
	}
	if s.modalConfirmDelete {
		return updateOpenConfirmDelete(s)
	}

	if rl.IsKeyPressed(rl.KeyEscape) {
		s.modal = modalNone
		return ActionNone
	}
	if len(s.modalPaths) == 0 {
		return ActionNone
	}
	if rl.IsKeyPressed(rl.KeyUp) || rl.IsKeyPressed(rl.KeyW) {
		s.modalCursor = core.WrapIndex(s.modalCursor-1, len(s.modalPaths))
	}
	if rl.IsKeyPressed(rl.KeyDown) {
		s.modalCursor = core.WrapIndex(s.modalCursor+1, len(s.modalPaths))
	}

	if rl.IsKeyPressed(rl.KeyR) {
		s.modalRenaming = core.MapIDFromPath(s.modalPaths[s.modalCursor])
		return ActionNone
	}
	if rl.IsKeyPressed(rl.KeyD) {
		s.modalConfirmDelete = true
		return ActionNone
	}
	if rl.IsKeyPressed(rl.KeyC) {
		newPath, err := duplicateMapFile(s.modalPaths[s.modalCursor])
		if err != nil {
			s.flash("Duplicate failed: " + err.Error())
			return ActionNone
		}
		refreshOpenList(s)
		for i, p := range s.modalPaths {
			if p == newPath {
				s.modalCursor = i
				break
			}
		}
		s.flash("Duplicated to " + core.MapIDFromPath(newPath))
		return ActionNone
	}

	if rl.IsKeyPressed(rl.KeyEnter) {
		path := s.modalPaths[s.modalCursor]
		mf, err := mapfile.Load(path)
		if err != nil {
			s.flash("Open failed: " + err.Error())
			s.modal = modalNone
			return ActionNone
		}
		area, err := core.AreaFromMapFile(mf, path)
		if err != nil {
			s.flash("Open failed: " + err.Error())
			s.modal = modalNone
			return ActionNone
		}
		s.area = area
		s.baseline = cloneArea(area)
		s.undo = nil
		s.redo = nil
		s.dirty = false
		s.modal = modalNone
		s.flash("Opened " + core.MapIDFromPath(path))
	}
	return ActionNone
}

func updateOpenRename(s *State) Action {
	pumpPrintableASCII(&s.modalRenaming, 64, nil)
	if rl.IsKeyPressed(rl.KeyEscape) {
		s.modalRenaming = ""
		return ActionNone
	}
	if rl.IsKeyPressed(rl.KeyEnter) {
		oldPath := s.modalPaths[s.modalCursor]
		newPath, err := renameMapFile(oldPath, s.modalRenaming)
		s.modalRenaming = ""
		if err != nil {
			s.flash("Rename failed: " + err.Error())
			return ActionNone
		}
		if s.area.Path == oldPath {
			s.area.Path = newPath
		}
		refreshOpenList(s)
		for i, p := range s.modalPaths {
			if p == newPath {
				s.modalCursor = i
				break
			}
		}
		s.flash("Renamed to " + core.MapIDFromPath(newPath))
	}
	return ActionNone
}

func updateOpenConfirmDelete(s *State) Action {
	if rl.IsKeyPressed(rl.KeyEscape) || rl.IsKeyPressed(rl.KeyN) {
		s.modalConfirmDelete = false
		return ActionNone
	}
	if rl.IsKeyPressed(rl.KeyY) {
		path := s.modalPaths[s.modalCursor]
		if err := os.Remove(path); err != nil {
			s.flash("Delete failed: " + err.Error())
			s.modalConfirmDelete = false
			return ActionNone
		}
		if s.area.Path == path {
			s.area.Path = ""
		}
		s.flash("Deleted " + core.MapIDFromPath(path))
		refreshOpenList(s)
		if s.modalCursor >= len(s.modalPaths) {
			s.modalCursor = len(s.modalPaths) - 1
		}
		if s.modalCursor < 0 {
			s.modalCursor = 0
		}
		s.modalConfirmDelete = false
	}
	return ActionNone
}

func refreshOpenList(s *State) {
	paths, _ := mapfile.List(core.MapsDir())
	s.modalPaths = paths
}

func updateSaveAsModal(s *State) Action {
	if s.awaitingOverwrite {
		if rl.IsKeyPressed(rl.KeyY) {
			s.awaitingOverwrite = false
			confirmModalForce(s)
			if s.modal == modalNone {
				return runPendingAction(s)
			}
			return ActionNone
		}
		if rl.IsKeyPressed(rl.KeyN) || rl.IsKeyPressed(rl.KeyEscape) {
			s.awaitingOverwrite = false
			s.focus = focusFilename
			return ActionNone
		}
		return ActionNone
	}

	if rl.IsKeyPressed(rl.KeyEscape) {
		s.modal = modalNone
		s.focus = focusNone
		s.pending = pendingNone
		return ActionNone
	}
	updateTextInput(s)
	if s.modal == modalNone {
		return runPendingAction(s)
	}
	return ActionNone
}

func updateConfirmDirtyModal(s *State) Action {
	if rl.IsKeyPressed(rl.KeyEscape) || rl.IsKeyPressed(rl.KeyC) {
		s.modal = modalNone
		s.pending = pendingNone
		return ActionNone
	}
	if rl.IsKeyPressed(rl.KeyD) {
		s.modal = modalNone
		return runPendingAction(s)
	}
	if rl.IsKeyPressed(rl.KeyS) {
		if s.area.Path == "" {
			s.modalFilename = sanitizeFilename(s.area.Name)
			s.modal = modalSaveAs
			s.focus = focusFilename
			return ActionNone
		}
		if err := mapfile.Save(s.area.Path, core.MapFileFromArea(s.area)); err != nil {
			s.flash("Save failed: " + err.Error())
			s.modal = modalNone
			s.pending = pendingNone
			return ActionNone
		}
		s.baseline = cloneArea(s.area)
		s.dirty = false
		s.modal = modalNone
		return runPendingAction(s)
	}
	return ActionNone
}

func runPendingAction(s *State) Action {
	p := s.pending
	s.pending = pendingNone
	switch p {
	case pendingExitToTitle:
		return ActionExitToTitle
	case pendingNew:
		performNewMap(s)
	case pendingOpen:
		openModal(s, modalOpen)
	}
	return ActionNone
}

func confirmModal(s *State) {
	if s.modal != modalSaveAs {
		return
	}
	name := s.modalFilename
	if name == "" {
		s.flash("Filename required")
		return
	}
	path := core.MapPath(name)
	if path != s.area.Path && fileExists(path) {
		s.awaitingOverwrite = true
		s.focus = focusNone
		return
	}
	saveTo(s, name, path)
}

func confirmModalForce(s *State) {
	name := s.modalFilename
	if name == "" {
		s.flash("Filename required")
		return
	}
	saveTo(s, name, core.MapPath(name))
}

func saveTo(s *State, name, path string) {
	if err := mapfile.Save(path, core.MapFileFromArea(s.area)); err != nil {
		s.flash("Save failed: " + err.Error())
		return
	}
	s.area.Path = path
	s.baseline = cloneArea(s.area)
	s.dirty = false
	s.modal = modalNone
	s.focus = focusNone
	s.flash("Saved " + name)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func pointIn(p rl.Vector2, r rl.Rectangle) bool {
	return p.X >= r.X && p.Y >= r.Y && p.X < r.X+r.Width && p.Y < r.Y+r.Height
}

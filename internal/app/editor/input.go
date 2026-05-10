package editor

import (
	"crawler/internal/app/core"
	"crawler/internal/app/core/mapfile"
	"os"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// updateHotkeys handles keyboard shortcuts when no text field is focused.
func updateHotkeys(s *State) {
	for _, t := range toolEntries {
		if rl.IsKeyPressed(t.hotkey) {
			s.tool = t.tool
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

	// F5: launch a playtest of the current in-memory area without saving.
	if rl.IsKeyPressed(rl.KeyF5) {
		s.testRequested = true
	}

	// Brush size cycling. [ shrinks, ] grows. Steps in 1, 3, 5.
	if !ctrl && rl.IsKeyPressed(rl.KeyLeftBracket) {
		s.brushSize = stepBrush(s.brushSize, -1)
	}
	if !ctrl && rl.IsKeyPressed(rl.KeyRightBracket) {
		s.brushSize = stepBrush(s.brushSize, +1)
	}

	// Reset zoom + pan to the auto-fit default.
	if !ctrl && rl.IsKeyPressed(rl.KeyHome) {
		s.zoom = 1
		s.panX, s.panY = 0, 0
	}

	// Cycle starting facing for the player-start tool with R. Gated to that
	// tool so R doesn't silently rotate the start while the user thinks
	// they're using a paint brush.
	if !ctrl && s.tool == ToolPlayerStart && rl.IsKeyPressed(rl.KeyR) {
		s.area.StartFacing = core.NormalizeFacing(s.area.StartFacing + 1)
		s.dirty = true
	}

	// Keyboard grid navigation: arrows move a logical cursor, space paints
	// at it, backspace erases. Inactive (gridCursor == -1) until the user
	// presses an arrow key for the first time, so mouse-driven workflows
	// don't accidentally show a stray cursor.
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
	if len(s.area.Layout) == 0 {
		return
	}
	mw := len(s.area.Layout[0])
	mh := len(s.area.Layout)
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
		// Mirror keyboard cursor into hover so the same "current cell" status
		// shows for both input modes.
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

// activateCursor seeds the keyboard cursor to the player start the first
// time the user reaches for the arrow keys. Subsequent calls are no-ops.
func activateCursor(s *State, mw, mh int) (int, int) {
	if s.gridCursorX >= 0 {
		return s.gridCursorX, s.gridCursorZ
	}
	x := s.area.StartTileX
	z := s.area.StartTileZ
	x = clampInt(x, 0, mw-1)
	z = clampInt(z, 0, mh-1)
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

	// Track hover so other systems (coord readout, brush ghost) can reuse it.
	hx, hz := s.cellAt(mp)
	s.hoverX, s.hoverZ = hx, hz

	// Mouse wheel zooms the grid plot. Centered on the current mouse cell
	// so zooming feels anchored, not abstract.
	if pointIn(mp, s.rect.grid) {
		w := rl.GetMouseWheelMove()
		if w != 0 {
			zoomBy(s, mp, 1+0.12*w)
		}
	}

	// Middle-button drag pans the grid plot.
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
		if hit := paletteToolAt(s, mp); hit >= 0 {
			s.tool = toolEntries[hit].tool
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

// startDrag picks a drag kind based on tool + cell contents + modifiers.
// Tile brushes default to paint; Shift switches to rectangle. Placement
// tools grab the entity under the cursor for drag-move when there is one.
// Ctrl+click on a tile brush is flood fill (one-shot, not a drag).
func startDrag(s *State, x, z int, ctrl, shift bool) {
	cur := toolEntries[s.tool]
	tileBrush := cur.tileByte != 0

	if tileBrush && ctrl {
		// Flood fill: replace the connected region of like-tiles starting
		// at (x,z) with the brush's tile. Single shot — no drag.
		pushUndo(s)
		floodFill(s, x, z, cur.tileByte)
		s.drag = dragNone
		return
	}

	switch s.tool {
	case ToolPlayerStart:
		if s.area.StartTileX == x && s.area.StartTileZ == z {
			s.drag = dragStart
			return
		}
	case ToolSpawnRat, ToolSpawnBat:
		if idx := spawnIndexAt(s.area.EnemySpawns, x, z); idx >= 0 {
			s.drag = dragEnemy
			s.dragSpawnIdx = idx
			return
		}
	}

	if tileBrush && shift {
		s.drag = dragRect
		s.rectAnchorX, s.rectAnchorZ = x, z
		return
	}

	// Default: regular paint stroke. snapshot lazily once the user actually
	// changes a cell so a single down-up doesn't burn an undo entry.
	s.drag = dragPaint
	s.dragSnapshotDone = false
	s.lastPaintX, s.lastPaintZ = -1, -1
	// Apply once at the start so a single click paints (not just drag).
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
	case dragStart:
		// Move ghost handled in draw; commit on release. We could do live
		// updates, but committing on release means undo is one snapshot,
		// not 30 across the drag.
	case dragEnemy:
		// Same: ghost preview during drag, commit on release.
	case dragRect:
		// Preview rect rendered during drag; commit on release.
	}
}

func finishDrag(s *State) {
	switch s.drag {
	case dragStart:
		if s.hoverX >= 0 && (s.hoverX != s.area.StartTileX || s.hoverZ != s.area.StartTileZ) {
			if isBlockingByte(s.area.Layout[s.hoverZ][s.hoverX]) {
				s.flash("Player start must be on a floor tile")
			} else {
				pushUndo(s)
				s.area.StartTileX = s.hoverX
				s.area.StartTileZ = s.hoverZ
				s.dirty = true
			}
		}
	case dragEnemy:
		if s.hoverX >= 0 && s.dragSpawnIdx >= 0 && s.dragSpawnIdx < len(s.area.EnemySpawns) {
			sp := s.area.EnemySpawns[s.dragSpawnIdx]
			if sp.TileX != s.hoverX || sp.TileZ != s.hoverZ {
				if isBlockingByte(s.area.Layout[s.hoverZ][s.hoverX]) {
					s.flash("Spawns must be on a floor tile")
				} else if s.area.StartTileX == s.hoverX && s.area.StartTileZ == s.hoverZ {
					s.flash("Cell holds the player start")
				} else {
					pushUndo(s)
					// Drop any spawn already at the destination, then move.
					s.area.EnemySpawns = removeSpawnAt(s.area.EnemySpawns, s.hoverX, s.hoverZ)
					// removeSpawnAt may have shifted our index — find again.
					idx := -1
					for i, e := range s.area.EnemySpawns {
						if e.TileX == sp.TileX && e.TileZ == sp.TileZ {
							idx = i
							break
						}
					}
					if idx >= 0 {
						s.area.EnemySpawns[idx].TileX = s.hoverX
						s.area.EnemySpawns[idx].TileZ = s.hoverZ
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
	s.dragSpawnIdx = -1
}

// applyToolBrushed runs the active tile brush over the brush-size square
// centered on (x,z). For non-tile tools (start, spawn) the size collapses
// to 1 since stamping multiple starts/spawns isn't meaningful.
func applyToolBrushed(s *State, x, z int) {
	half := s.brushSize / 2
	if !isTileBrush(s.tool) || s.brushSize <= 1 {
		applyTool(s, x, z)
		return
	}
	for dz := -half; dz <= half; dz++ {
		for dx := -half; dx <= half; dx++ {
			applyTool(s, x+dx, z+dz)
		}
	}
}

func isTileBrush(t Tool) bool {
	switch t {
	case ToolFloor, ToolWall, ToolTree, ToolTreeXL, ToolBoulder, ToolBush:
		return true
	}
	return false
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
	// Re-anchor pan so the cell under the cursor stays under the cursor.
	if s.rect.cellPx > 0 {
		dx := anchor.X - s.rect.gridX
		dy := anchor.Y - s.rect.gridY
		s.panX += dx*(1-next/prev)
		s.panY += dy*(1-next/prev)
	}
	s.zoom = next
}

func spawnIndexAt(spawns []core.EnemySpawn, x, z int) int {
	for i, sp := range spawns {
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
		openModal(s, modalOpen)
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

// updateTextInput appends typed chars to the focused field and handles
// backspace / enter / escape. The field's storage is decided by the focus
// enum. focusWidth and focusHeight accept digits only and commit a resize
// on Enter.
func updateTextInput(s *State) {
	if s.focus == focusWidth || s.focus == focusHeight {
		updateNumericInput(s)
		return
	}
	target := activeTextTarget(s)
	if target == nil {
		return
	}
	for {
		c := rl.GetCharPressed()
		if c == 0 {
			break
		}
		// ASCII printable only — we don't render unicode glyphs anyway, and
		// the .map format is expected to round-trip through plain text.
		if c >= 32 && c < 127 && len(*target) < 96 {
			*target += string(rune(c))
			s.markDirty()
		}
	}
	if rl.IsKeyPressed(rl.KeyBackspace) && len(*target) > 0 {
		*target = (*target)[:len(*target)-1]
		s.markDirty()
	}
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
		// Cap at 200 to avoid pathologically huge layouts that would chew
		// through memory + render time. Plenty for an Etrian-style map.
		v = 200
	}
	mw := len(s.area.Layout[0])
	mh := len(s.area.Layout)
	if s.focus == focusWidth {
		resize(s, v, mh)
	} else if s.focus == focusHeight {
		resize(s, mw, v)
	}
	s.numericBuf = ""
}

// cycleFocus moves Tab focus through the metadata text fields in a stable
// order (Name → Quiet → Width → Height → back to Name).
func cycleFocus(s *State) {
	if s.focus == focusFilename {
		// Save As is its own one-field flow; Tab does nothing useful there.
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

// markDirty avoids flagging name/quiet edits when the focused field is the
// modal filename — that one is for the save dialog, not in-place metadata.
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

// updateModal handles input in the open / save-as / confirm-exit dialogs.
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
	// Inline-rename takes the keyboard while it's active.
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
		// Begin inline rename of the highlighted file. Pre-populate with
		// the current id so the user only edits what they want to change.
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
		// Move the cursor onto the duplicate so the next Enter opens it.
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
		s.undo = nil
		s.redo = nil
		s.dirty = false
		s.modal = modalNone
		s.flash("Opened " + core.MapIDFromPath(path))
	}
	return ActionNone
}

func updateOpenRename(s *State) Action {
	for {
		c := rl.GetCharPressed()
		if c == 0 {
			break
		}
		if c >= 32 && c < 127 && len(s.modalRenaming) < 64 {
			s.modalRenaming += string(rune(c))
		}
	}
	if rl.IsKeyPressed(rl.KeyBackspace) && len(s.modalRenaming) > 0 {
		s.modalRenaming = s.modalRenaming[:len(s.modalRenaming)-1]
	}
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
		// If the renamed file is the one currently being edited, update its
		// path so subsequent saves go to the new file rather than re-creating
		// the old one.
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
		// If we just deleted the file currently being edited, drop the path
		// so the editor's next save prompts for a new name.
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
	// Overwrite confirmation has its own input set: Y proceeds, N or Esc
	// returns to typing the filename. We branch first so a typed 'y' / 'n'
	// during the prompt doesn't fall through to filename editing.
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
	// confirmModal closes the modal on success. Fire any pending deferred
	// action (exit / new / open) on the same frame.
	if s.modal == modalNone {
		return runPendingAction(s)
	}
	return ActionNone
}

// updateConfirmDirtyModal handles the "save before destructive action?"
// prompt. Reads s.pending to know which action to run on Discard / Save.
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
		s.dirty = false
		s.modal = modalNone
		return runPendingAction(s)
	}
	return ActionNone
}

// runPendingAction performs whatever destructive action the dirty-prompt
// was gating, clearing s.pending. Returns the Action the run loop should
// see (ActionExitToTitle for exit, ActionNone for in-editor effects).
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
	// Refuse to silently clobber an existing file unless it's the very file
	// we already have open (a no-name-change save). Switch into Y/N prompt;
	// the modal sticks around so the user can confirm or back out.
	if path != s.area.Path && fileExists(path) {
		s.awaitingOverwrite = true
		s.focus = focusNone
		return
	}
	saveTo(s, name, path)
}

// confirmModalForce is the post-overwrite-prompt save: skips the existence
// check (the user already said yes) and lands the file.
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
	s.dirty = false
	s.modal = modalNone
	s.focus = focusNone
	s.flash("Saved " + name)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// pointIn is a tiny convenience so click-tests read like English.
func pointIn(p rl.Vector2, r rl.Rectangle) bool {
	return p.X >= r.X && p.Y >= r.Y && p.X < r.X+r.Width && p.Y < r.Y+r.Height
}

package editor

import (
	"crawler/internal/app/core"
	"crawler/internal/app/core/mapfile"
	"crawler/internal/app/input"
	"os"
	"strconv"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// updateHotkeys handles keyboard shortcuts when no text field is focused.
func updateHotkeys(s *State) {
	ctrl := rl.IsKeyDown(rl.KeyLeftControl) || rl.IsKeyDown(rl.KeyRightControl)
	shift := rl.IsKeyDown(rl.KeyLeftShift) || rl.IsKeyDown(rl.KeyRightShift)
	alt := rl.IsKeyDown(rl.KeyLeftAlt) || rl.IsKeyDown(rl.KeyRightAlt)

	// Alt+1..6 jumps directly to a layer — saves Tab-cycling when the
	// author knows which layer they want. Number row only; the keypad
	// equivalents aren't bound to keep the binding compact.
	if alt {
		layerKeys := []int32{rl.KeyOne, rl.KeyTwo, rl.KeyThree, rl.KeyFour, rl.KeyFive, rl.KeySix}
		for i, k := range layerKeys {
			if rl.IsKeyPressed(k) {
				s.layer = Layer(i)
				return
			}
		}
	}

	// 1..9 select a brush within the active layer's palette. Shift+1..9
	// picks brushes 9..17 in the same layer so the second row of long
	// palettes (props, decor) is keyboard-reachable without scrolling.
	// Brushes past index 17 stay mouse-only.
	palette := layerBrushes[s.layer]
	if !ctrl && !alt {
		offset := 0
		if shift {
			offset = 9
		}
		numKeys := []int32{rl.KeyOne, rl.KeyTwo, rl.KeyThree, rl.KeyFour, rl.KeyFive, rl.KeySix, rl.KeySeven, rl.KeyEight, rl.KeyNine}
		for i, k := range numKeys {
			idx := i + offset
			if idx >= len(palette) {
				break
			}
			if rl.IsKeyPressed(k) {
				s.brushIdx[s.layer] = idx
			}
		}
	}

	switch {
	case ctrl && shift && rl.IsKeyPressed(rl.KeyZ):
		redoOne(s)
	case ctrl && shift && rl.IsKeyPressed(rl.KeyF):
		// Ctrl+Shift+F: fill the entire active grid layer with the
		// active brush. Quick way to lay a base material (e.g. all
		// stone floor) or wipe a layer back to a sentinel.
		fillEntireLayer(s)
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

	// G centers the view on the player start so authors can jump back
	// after panning into a far corner. Skip when on the entity layer's
	// player-start brush (G isn't currently consumed there but reserving
	// for future brush-specific hotkeys).
	if !ctrl && !alt && rl.IsKeyPressed(rl.KeyG) {
		centerViewOnTile(s, s.area.StartTileX, s.area.StartTileZ)
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
	// Ctrl+F5: same, but temporarily set StartTileX/Z to the grid cursor
	// (or hover) so the playtest drops you AT the cursor instead of the
	// authored start. The author's saved StartTile is restored on return
	// from the playtest. Lets you iterate on a far room without walking
	// to it every test.
	if rl.IsKeyPressed(rl.KeyF5) {
		if ctrl {
			tx, tz := -1, -1
			if s.gridCursorX >= 0 {
				tx, tz = s.gridCursorX, s.gridCursorZ
			} else if s.hoverX >= 0 {
				tx, tz = s.hoverX, s.hoverZ
			}
			if tx >= 0 && !s.area.BlockedAt(tx, tz) {
				s.testStartOverrideX = tx
				s.testStartOverrideZ = tz
				s.testStartOverride = true
				s.flash("Test-from-cursor at " + core.TileCoord(tx, tz))
			} else {
				s.flash("Cursor cell is blocked or unset; using authored start")
			}
		}
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
		s.previewPhase = core.TimeOfDay((int(s.previewPhase) + 1) % core.TimeOfDayCount)
		s.flash("Preview: " + core.PhaseName(s.previewPhase))
	}

	updateGridCursor(s)
}

// brushSizeSteps is the discrete set of brush widths cycled with [ / ].
// Lifted out of stepBrush so a future tuning pass that wants a 7-cell brush
// (or removes the 3-cell middle) edits one line. applyToolBrushed reads
// brushSize / 2 as the radius — keep these odd so the brush is centered on
// the cursor cell.
var brushSizeSteps = []int{1, 3, 5}

func stepBrush(cur, dir int) int {
	idx := 0
	for i, v := range brushSizeSteps {
		if v == cur {
			idx = i
			break
		}
	}
	idx += dir
	if idx < 0 {
		idx = 0
	}
	if idx >= len(brushSizeSteps) {
		idx = len(brushSizeSteps) - 1
	}
	return brushSizeSteps[idx]
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
		s.gridCursorX = core.ClampInt(s.gridCursorX-1, 0, mw-1)
		moved = true
	}
	if rl.IsKeyPressed(rl.KeyRight) {
		s.gridCursorX, s.gridCursorZ = activateCursor(s, mw, mh)
		s.gridCursorX = core.ClampInt(s.gridCursorX+1, 0, mw-1)
		moved = true
	}
	if rl.IsKeyPressed(rl.KeyUp) {
		s.gridCursorX, s.gridCursorZ = activateCursor(s, mw, mh)
		s.gridCursorZ = core.ClampInt(s.gridCursorZ-1, 0, mh-1)
		moved = true
	}
	if rl.IsKeyPressed(rl.KeyDown) {
		s.gridCursorX, s.gridCursorZ = activateCursor(s, mw, mh)
		s.gridCursorZ = core.ClampInt(s.gridCursorZ+1, 0, mh-1)
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
	x := core.ClampInt(s.area.StartTileX, 0, mw-1)
	z := core.ClampInt(s.area.StartTileZ, 0, mh-1)
	return x, z
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
	} else if pointIn(mp, s.rect.palette) {
		// Wheel over the palette scrolls the brush list. One notch
		// moves about one and a half rows so the user can step
		// through long palettes without dragging a scrollbar.
		w := rl.GetMouseWheelMove()
		if w != 0 {
			ScrollPalette(s, -w*paletteRowStride*1.5)
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
		case entityAddEnemy:
			// Click on an existing pack picks it up for drag-move (drag to
			// new tile) OR opens the pack editor modal (release on the
			// same tile — see finishDrag's dragPack branch). Click on an
			// empty tile falls through to "place a new pack with one
			// member of this kind" via applyTool.
			if idx := core.PackSpawnIndexAt(s.area.PackSpawns, x, z); idx >= 0 {
				s.drag = dragPack
				s.dragPackIdx = idx
				return
			}
		case entityPlaceChest:
			// Click on an existing chest opens the chest-edit modal so
			// the author can change the loot. Click on an empty tile
			// falls through to applyTool which plants a new chest with
			// the default starter loot.
			if idx := core.ChestSpawnIndexAt(s.area.ChestSpawns, x, z); idx >= 0 {
				s.modal = modalChestEdit
				s.modalChestIdx = idx
				s.modalCursor = 0
				return
			}
		case entityPlaceDoor:
			// Click on an existing door opens the door-edit modal so
			// the author can set its target_map / target_door / facing
			// without having to hand-edit the .map. Click on an empty
			// tile falls through to applyTool which plants a fresh
			// placeholder door.
			if idx := core.DoorSpawnIndexAt(s.area.DoorSpawns, x, z); idx >= 0 {
				openDoorEditModal(s, idx)
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
			} else if core.ChestSpawnIndexAt(s.area.ChestSpawns, s.hoverX, s.hoverZ) >= 0 {
				s.flash("Player start can't share a tile with a chest")
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
				} else if core.ChestSpawnIndexAt(s.area.ChestSpawns, s.hoverX, s.hoverZ) >= 0 {
					s.flash("Cell holds a chest — clear it first")
				} else {
					pushUndo(s)
					// Drop any pack that was already at the destination cell
					// (dragging this one onto another replaces the existing).
					// Then locate the dragged pack by its old coords and move
					// it to the destination. The old-coords lookup works
					// because addPackMember keeps at most one pack per cell.
					s.area.PackSpawns = removePackAt(s.area.PackSpawns, s.hoverX, s.hoverZ)
					idx := core.PackSpawnIndexAt(s.area.PackSpawns, sp.TileX, sp.TileZ)
					if idx >= 0 {
						s.area.PackSpawns[idx].TileX = s.hoverX
						s.area.PackSpawns[idx].TileZ = s.hoverZ
					}
					s.dirty = true
				}
			} else {
				// Click without drag (release on the same tile we picked
				// up) → open the inline pack editor instead of silently
				// no-op'ing. Lets the author manage member list /
				// reorder / remove without the awkward "use the diseased
				// rat brush to add to a rat pack" workaround.
				s.modal = modalPackEdit
				s.modalPackIdx = s.dragPackIdx
				s.modalCursor = 0
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
	case "sounds":
		openSoundsModal(s)
	case "validate":
		openValidateModal(s)
	case "back":
		s.exitRequested = true
	}
}

// openValidateModal snapshots the current reachability and cross-map
// door warnings into the modal so the user can read the full list at
// once instead of the 4-row metadata-panel cap.
func openValidateModal(s *State) {
	rows := append([]string{}, reachabilityWarnings(s.area)...)
	rows = append(rows, crossMapDoorWarnings(s.area)...)
	s.modalValidateRows = rows
	s.modal = modalValidate
}

// pumpPrintableASCII drains queued printable-ASCII characters into
// target (capped at maxLen) and consumes one backspace press. The
// accept predicate filters which runes land in the buffer — callers
// pass nil for "any printable ASCII" or a custom filter for "no
// space" (sound-name input), "digits only" (numeric resize), etc. so
// one pump function backs every text-field flavor in the editor.
// onChange fires once per accepted character or backspace and may be
// nil when no caller-side effect is needed.
func pumpPrintableASCII(target *string, maxLen int, accept func(rune) bool, onChange func()) {
	for {
		c := rl.GetCharPressed()
		if c == 0 {
			break
		}
		if c < 32 || c >= 127 {
			continue
		}
		if accept != nil && !accept(c) {
			continue
		}
		if len(*target) >= maxLen {
			continue
		}
		*target += string(rune(c))
		if onChange != nil {
			onChange()
		}
	}
	if rl.IsKeyPressed(rl.KeyBackspace) && len(*target) > 0 {
		*target = (*target)[:len(*target)-1]
		if onChange != nil {
			onChange()
		}
	}
}

// acceptPrintable is the default accept-rune filter for pumpPrintableASCII
// — accepts every printable ASCII character. Use when the caller wants
// the historical behavior of "any printable rune".
func acceptPrintable(r rune) bool { return true }

// acceptPrintableNoSpace excludes ASCII space so the sound-modal Name
// field can coexist with Space-as-Preview. Other callers that want
// space-free input (filenames, etc.) can reuse this.
func acceptPrintableNoSpace(r rune) bool { return r != ' ' }

func updateTextInput(s *State) {
	if s.focus == focusWidth || s.focus == focusHeight {
		updateNumericInput(s)
		return
	}
	target := activeTextTarget(s)
	if target == nil {
		return
	}
	pumpPrintableASCII(target, 96, acceptPrintable, s.markDirty)
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
	if v > core.MaxMapDimension {
		v = core.MaxMapDimension
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
	if updater, ok := modalUpdaters[s.modal]; ok {
		return updater(s)
	}
	return ActionNone
}

// modalUpdaters maps each modalKind to its input handler. Sibling of
// modalDrawers in draw.go — adding a new modal touches both tables.
var modalUpdaters = map[modalKind]func(*State) Action{
	modalOpen:         updateOpenModal,
	modalSaveAs:       updateSaveAsModal,
	modalConfirmDirty: updateConfirmDirtyModal,
	modalPackEdit:     updatePackEditModal,
	modalChestEdit:    updateChestEditModal,
	modalSounds:       updateSoundsModal,
	modalDoorEdit:     updateDoorEditModal,
	modalValidate:     updateValidateModal,
}

// closeModal is the single seam every modal updater goes through to
// dismiss its dialog. Clears the modal kind plus every modal-scoped
// cursor / index field so a future modal can't read a stale value from
// the prior one. Replaces ~18 hand-typed `s.modal = modalNone; s.modalXxxIdx
// = -1` snippets that drifted per modal — the chest-edit updater was
// missing a modalCursor reset under the previous shape.
func closeModal(s *State) {
	s.modal = modalNone
	s.modalCursor = 0
	s.modalPackIdx = -1
	s.modalChestIdx = -1
	s.modalDoorIdx = -1
	s.modalValidateRows = nil
	s.modalConfirmDelete = false
	s.modalRenaming = ""
	soundDrag.sliderIdx = -1
	// Door-edit text focus survives outside the modal in pumpPrintableASCII's
	// flow, so explicitly drop it here too.
	if s.focus == focusDoorName || s.focus == focusDoorTargetMap || s.focus == focusDoorTargetDoor {
		s.focus = focusNone
	}
}

// openDoorEditModal opens the per-door editor for spawn index idx and
// parks focus on the Name field. Mirrors the pack / chest path.
func openDoorEditModal(s *State, idx int) {
	if idx < 0 || idx >= len(s.area.DoorSpawns) {
		return
	}
	s.modal = modalDoorEdit
	s.modalDoorIdx = idx
	s.modalCursor = 0
	s.focus = focusDoorName
}

// updateDoorEditModal drives the door-edit modal: three text fields (Name,
// TargetMap, TargetDoor) plus four facing buttons. Tab cycles fields, the
// individual fields accept printable ASCII via pumpPrintableASCII, the
// arrow keys (when no field is focused) move between facing buttons and
// Space confirms. Esc closes; X deletes the door entirely.
func updateDoorEditModal(s *State) Action {
	if s.modalDoorIdx < 0 || s.modalDoorIdx >= len(s.area.DoorSpawns) {
		closeModal(s)
		return ActionNone
	}
	door := &s.area.DoorSpawns[s.modalDoorIdx]

	// Mouse handling — clicking a field focuses it; clicking a facing
	// button sets the facing; clicking Delete drops the door.
	if rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		mp := rl.GetMousePosition()
		hit := doorEditHitTest(s, mp)
		switch hit.kind {
		case doorHitName:
			s.focus = focusDoorName
			return ActionNone
		case doorHitTargetMap:
			s.focus = focusDoorTargetMap
			return ActionNone
		case doorHitTargetDoor:
			s.focus = focusDoorTargetDoor
			return ActionNone
		case doorHitFacing:
			pushUndo(s)
			door.Facing = hit.facing
			s.dirty = true
			s.focus = focusNone
			return ActionNone
		case doorHitDelete:
			pushUndo(s)
			s.area.DoorSpawns = append(s.area.DoorSpawns[:s.modalDoorIdx], s.area.DoorSpawns[s.modalDoorIdx+1:]...)
			s.dirty = true
			closeModal(s)
			return ActionNone
		case doorHitClose:
			closeModal(s)
			return ActionNone
		case doorHitOutside:
			// Click outside the card commits and closes — matches pack /
			// chest modals that just need Esc.
		}
	}

	// Keyboard: while a text field is focused, route every keystroke into
	// its buffer via pumpPrintableASCII. Tab cycles to the next field;
	// Enter confirms current field; Esc closes.
	switch s.focus {
	case focusDoorName, focusDoorTargetMap, focusDoorTargetDoor:
		target := doorEditTextTarget(s)
		if target != nil {
			before := *target
			pumpPrintableASCII(target, 96, acceptPrintable, nil)
			if *target != before {
				s.dirty = true
			}
		}
		if rl.IsKeyPressed(rl.KeyTab) {
			cycleDoorFocus(s)
			return ActionNone
		}
		if rl.IsKeyPressed(rl.KeyEnter) {
			s.focus = focusNone
			return ActionNone
		}
		if rl.IsKeyPressed(rl.KeyEscape) {
			closeModal(s)
			return ActionNone
		}
		return ActionNone
	}

	// No text field focused — keyboard shortcuts for facing + delete.
	if rl.IsKeyPressed(rl.KeyEscape) || rl.IsKeyPressed(rl.KeyEnter) {
		closeModal(s)
		return ActionNone
	}
	if rl.IsKeyPressed(rl.KeyTab) {
		s.focus = focusDoorName
		return ActionNone
	}
	if rl.IsKeyPressed(rl.KeyX) {
		pushUndo(s)
		s.area.DoorSpawns = append(s.area.DoorSpawns[:s.modalDoorIdx], s.area.DoorSpawns[s.modalDoorIdx+1:]...)
		s.dirty = true
		closeModal(s)
		return ActionNone
	}
	// N / E / S / W set facing directly. Updates only run while the
	// modal is open and updateHotkeys's global Ctrl+S Save handler
	// doesn't fire during modals, so the 'S' binding is free here.
	facingKeys := []struct {
		key    int32
		facing int
	}{
		{rl.KeyN, core.North},
		{rl.KeyE, core.East},
		{rl.KeyS, core.South},
		{rl.KeyW, core.West},
	}
	for _, fk := range facingKeys {
		if rl.IsKeyPressed(fk.key) {
			pushUndo(s)
			door.Facing = fk.facing
			s.dirty = true
			return ActionNone
		}
	}
	return ActionNone
}

// doorEditTextTarget returns the address of whichever DoorSpawn string
// field the current focus targets. Mirrors activeTextTarget in shape.
func doorEditTextTarget(s *State) *string {
	if s.modalDoorIdx < 0 || s.modalDoorIdx >= len(s.area.DoorSpawns) {
		return nil
	}
	d := &s.area.DoorSpawns[s.modalDoorIdx]
	switch s.focus {
	case focusDoorName:
		return &d.Name
	case focusDoorTargetMap:
		return &d.TargetMap
	case focusDoorTargetDoor:
		return &d.TargetDoor
	}
	return nil
}

func cycleDoorFocus(s *State) {
	switch s.focus {
	case focusDoorName:
		s.focus = focusDoorTargetMap
	case focusDoorTargetMap:
		s.focus = focusDoorTargetDoor
	case focusDoorTargetDoor:
		s.focus = focusDoorName
	}
}

// updateValidateModal: any key closes; it's a read-only viewer.
func updateValidateModal(s *State) Action {
	if rl.IsKeyPressed(rl.KeyEscape) || rl.IsKeyPressed(rl.KeyEnter) || rl.IsKeyPressed(rl.KeySpace) {
		closeModal(s)
	}
	if rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		closeModal(s)
	}
	return ActionNone
}

// updatePackEditModal drives the inline pack editor: arrow keys / W-S
// navigate the member list, X removes the highlighted member, J/K moves
// it down/up in the list (J = "down arrow", K = "up arrow" mnemonic but
// also matches Vim's J/K convention), R/B/D appends a new Rat/Bat/
// Diseased rat. Esc/Enter close. If the pack disappears (e.g. user
// removed the last member), the modal closes and the pack is dropped.
func updatePackEditModal(s *State) Action {
	if s.modalPackIdx < 0 || s.modalPackIdx >= len(s.area.PackSpawns) {
		closeModal(s)
		return ActionNone
	}
	pack := &s.area.PackSpawns[s.modalPackIdx]
	memberCount := len(pack.Members)
	if s.modalCursor >= memberCount {
		s.modalCursor = memberCount - 1
	}
	if s.modalCursor < 0 {
		s.modalCursor = 0
	}

	if input.ModalClosePressed() {
		closeModal(s)
		return ActionNone
	}
	if memberCount > 0 {
		s.modalCursor = input.CursorUpDown(s.modalCursor, memberCount)
		if rl.IsKeyPressed(rl.KeyX) {
			pushUndo(s)
			pack.Members = append(pack.Members[:s.modalCursor], pack.Members[s.modalCursor+1:]...)
			s.dirty = true
			if len(pack.Members) == 0 {
				s.area.PackSpawns = append(s.area.PackSpawns[:s.modalPackIdx], s.area.PackSpawns[s.modalPackIdx+1:]...)
				closeModal(s)
				return ActionNone
			}
		}
		// K = move highlighted member up; J = move down. Mirrors the
		// natural arrow-up/down direction in the rendered list.
		if rl.IsKeyPressed(rl.KeyK) && s.modalCursor > 0 {
			pushUndo(s)
			pack.Members[s.modalCursor-1], pack.Members[s.modalCursor] = pack.Members[s.modalCursor], pack.Members[s.modalCursor-1]
			s.modalCursor--
			s.dirty = true
		}
		if rl.IsKeyPressed(rl.KeyJ) && s.modalCursor < memberCount-1 {
			pushUndo(s)
			pack.Members[s.modalCursor+1], pack.Members[s.modalCursor] = pack.Members[s.modalCursor], pack.Members[s.modalCursor+1]
			s.modalCursor++
			s.dirty = true
		}
	}
	// Add-kind shortcuts driven by the packAddRules table — adding a
	// new enemy kind is one row in that slice instead of three hand-
	// typed `if rl.IsKeyPressed(...)` blocks. The hint row in the modal
	// reads its label out of the same table so display stays in sync.
	for _, rule := range packAddRules {
		if rl.IsKeyPressed(rule.Key) {
			pushUndo(s)
			pack.Members = append(pack.Members, rule.Kind)
			s.modalCursor = len(pack.Members) - 1
			s.dirty = true
		}
	}
	return ActionNone
}

// Hint-row string constants for editor modal footers. Defined here next
// to the modal updaters so the keybindings ("Esc close", "X remove",
// "K/J move…") sit beside the actual rl.KeyEscape / rl.KeyX / rl.KeyK
// handlers that listen for them. Drift between key handler and label is
// the thing the constants are meant to prevent — longer hints are
// composed from the shorter tokens so renaming a key ("Up/Down" →
// "↑/↓") is a one-line edit.
const (
	hintSep          = "   "
	hintEscClose     = "Esc close"
	hintXRemove      = "X remove"
	hintUpDownNav    = "Up/Down nav"
	hintKJReorder    = "K/J move up/down"
	hintPackEditNav  = hintUpDownNav + hintSep + hintXRemove + hintSep + hintKJReorder
	hintChestEditNav = hintUpDownNav + hintSep + hintXRemove
)

// packAddRule binds a keyboard shortcut to the EnemyKind it adds in the
// pack-edit modal. The slice is built at init from core.EnemyKinds() +
// a positional hotkey pool — adding a new enemy is one row in core's
// enemyDefinitions and the modal picks up the hotkey automatically.
type packAddRule struct {
	Key   int32
	Kind  core.EnemyKind
	Label string // appears in the modal's hint row, e.g. "R add Rat"
}

// packAddHotkeys is the positional pool: entry i in core.EnemyKinds()
// gets pool[i]. Keys past pool length are 0 (no binding, mouse-only).
// The existing keys are preserved in registry order (R, B, D, G, M, N)
// so muscle memory survives the refactor.
var packAddHotkeys = []int32{
	rl.KeyR, rl.KeyB, rl.KeyD, rl.KeyG, rl.KeyM, rl.KeyN, rl.KeyV, rl.KeyZ,
}

// init asserts the add-rule hotkey pools cover the current registries.
// The pack-edit and chest-edit modals are keyboard-only for "add a
// kind" — an entry with Key=0 (rl.KeyNull) would silently be
// unauthorable. Failing closed at startup is cheaper than shipping a
// map editor where the new enemy / item is invisible to the author.
func init() {
	if got, max := len(core.EnemyKinds()), len(packAddHotkeys); got > max {
		panic("editor: packAddHotkeys pool too small (" + strconv.Itoa(got) + " enemies, " + strconv.Itoa(max) + " keys)")
	}
	if got, max := len(core.AllItems()), len(chestAddHotkeys); got > max {
		panic("editor: chestAddHotkeys pool too small (" + strconv.Itoa(got) + " items, " + strconv.Itoa(max) + " keys)")
	}
}

var packAddRules = buildPackAddRules()

func buildPackAddRules() []packAddRule {
	defs := core.EnemyKinds()
	out := make([]packAddRule, 0, len(defs))
	for i, def := range defs {
		key := int32(0)
		if i < len(packAddHotkeys) {
			key = packAddHotkeys[i]
		}
		out = append(out, packAddRule{
			Key:   key,
			Kind:  def.Kind,
			Label: addRuleLabel(key, def.SingularName),
		})
	}
	return out
}

// chestAddRule binds a keyboard shortcut to the ItemKind it appends in
// the chest-edit modal. Built at init from core.AllItems() + the
// positional hotkey pool below — same shape as packAddRules.
type chestAddRule struct {
	Key   int32
	Kind  core.ItemKind
	Label string
}

// chestAddHotkeys is the positional pool for the chest-edit modal.
// Existing keys (C for Cheese, J for Jerky) preserved in registry
// order. Extend as items are added; entries past pool length are
// mouse-only.
var chestAddHotkeys = []int32{
	rl.KeyC, rl.KeyJ, rl.KeyP, rl.KeyU, rl.KeyW, rl.KeyQ,
}

var chestAddRules = buildChestAddRules()

func buildChestAddRules() []chestAddRule {
	defs := core.AllItems()
	out := make([]chestAddRule, 0, len(defs))
	for i, def := range defs {
		key := int32(0)
		if i < len(chestAddHotkeys) {
			key = chestAddHotkeys[i]
		}
		out = append(out, chestAddRule{
			Key:   key,
			Kind:  def.Kind,
			Label: addRuleLabel(key, shortItemLabel(def.Name)),
		})
	}
	return out
}

// addRuleLabel formats a modal-footer hint row from a hotkey + a noun.
// "R add Rat" when the entry has a hotkey; just "add Rat" when it
// doesn't — better than "? add Rat" which suggested a missing label
// before the init guard caught pool overflow. Used by both
// buildPackAddRules and buildChestAddRules.
func addRuleLabel(key int32, name string) string {
	if key == 0 {
		return "add " + name
	}
	return hotkeyChar(key) + " add " + name
}

// shortItemLabel returns the last whitespace-separated word of an item
// Name. "Morsel of Cheese" → "Cheese", "Bat Jerky" → "Jerky" — keeps
// the hint footer compact instead of "C add Morsel of Cheese."
func shortItemLabel(name string) string {
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == ' ' {
			return name[i+1:]
		}
	}
	return name
}

// hotkeyChar converts an rl.KeyX constant to the printable char it
// represents ('A'-'Z' for letter keys). Used by the rule-builder to
// fill the "X add Y" label prefix automatically. Returns "?" for any
// key outside the letter range — callers should only pass letter keys.
func hotkeyChar(key int32) string {
	if key >= rl.KeyA && key <= rl.KeyZ {
		return string(rune(int32('A') + (key - rl.KeyA)))
	}
	return "?"
}

// updateChestEditModal drives the inline chest editor: arrow keys
// navigate the item list, X removes the highlighted item. Item add
// shortcuts come from chestAddRules so adding an authored item kind
// to the editor is one row. Esc/Enter close. If every item gets
// removed the chest stays — an empty chest is a valid authored shape
// (e.g. flavor decoration) and the runtime renders it pre-looted.
func updateChestEditModal(s *State) Action {
	if s.modalChestIdx < 0 || s.modalChestIdx >= len(s.area.ChestSpawns) {
		closeModal(s)
		return ActionNone
	}
	chest := &s.area.ChestSpawns[s.modalChestIdx]
	itemCount := len(chest.Items)
	if s.modalCursor >= itemCount {
		s.modalCursor = itemCount - 1
	}
	if s.modalCursor < 0 {
		s.modalCursor = 0
	}

	if input.ModalClosePressed() {
		closeModal(s)
		return ActionNone
	}
	if itemCount > 0 {
		s.modalCursor = input.CursorUpDown(s.modalCursor, itemCount)
		if rl.IsKeyPressed(rl.KeyX) {
			pushUndo(s)
			chest.Items = append(chest.Items[:s.modalCursor], chest.Items[s.modalCursor+1:]...)
			s.dirty = true
		}
	}
	for _, rule := range chestAddRules {
		if rl.IsKeyPressed(rule.Key) {
			pushUndo(s)
			chest.Items = append(chest.Items, rule.Kind)
			s.modalCursor = len(chest.Items) - 1
			s.dirty = true
		}
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
	s.modalCursor = input.CursorUpDown(s.modalCursor, len(s.modalPaths))

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
	pumpPrintableASCII(&s.modalRenaming, 64, acceptPrintable, nil)
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
	paths, _ := mapfile.ListByModTime(core.MapsDir())
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
		mf, err := core.MapFileFromArea(s.area)
		if err != nil {
			s.flash("Save failed: " + err.Error())
			s.modal = modalNone
			s.pending = pendingNone
			return ActionNone
		}
		if err := mapfile.Save(s.area.Path, mf); err != nil {
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
	// Sanitize at commit so the disk filename is always known-good (lower
	// ascii + _-) regardless of what the user typed. The Save As field's
	// preview already shows this sanitized form, so the user has seen
	// what's about to land.
	name := sanitizeFilename(s.modalFilename)
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
	name := sanitizeFilename(s.modalFilename)
	if name == "" {
		s.flash("Filename required")
		return
	}
	saveTo(s, name, core.MapPath(name))
}

func saveTo(s *State, name, path string) {
	mf, err := core.MapFileFromArea(s.area)
	if err != nil {
		s.flash("Save failed: " + err.Error())
		return
	}
	if err := mapfile.Save(path, mf); err != nil {
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

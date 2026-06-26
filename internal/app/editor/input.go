package editor

import (
	"crawler/internal/app/core"
	"crawler/internal/app/core/mapfile"
	"crawler/internal/app/input"
	"crawler/internal/app/render"
	"os"
	"strconv"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// editorCommitPressed / editorCancelPressed / editorTabPressed are the editor's
// commit / cancel / focus-cycle edges, delegating to input.Editor* so bindings
// live in one place. The editor uses Enter (not the Z/Space chord) to commit so
// it can't collide with typing into a Name field.
func editorCommitPressed() bool {
	return input.EditorConfirmPressed()
}

func editorCancelPressed() bool {
	return input.EditorCancelPressed()
}

func editorTabPressed() bool {
	return input.EditorTabPressed()
}

// editorAddPressed / editorDeletePressed are the editor's list-modal verb keys
// (A add, X delete), in one place so the list modals (packs, chests, doors, dialog
// nodes/choices/conditions, locations, sounds) can't drift on the mnemonic. (M is
// NOT centralized — it's a per-modal toggle, not a uniform verb.) Editor is
// keyboard-exempt, so raw rl reads are allowed here.
func editorAddPressed() bool    { return rl.IsKeyPressed(rl.KeyA) }
func editorDeletePressed() bool { return rl.IsKeyPressed(rl.KeyX) }

// runCardCmdsNav is runCardCmds plus keyboard navigation: Up/Down walk s.modalCursor
// over the buttons and Enter fires the selected one. Mouse clicks + the per-cmd hot
// accelerators still work; the matching draw highlights s.modalCursor. For the
// stacked pause/confirm menus so they're operable without knowing the mnemonics.
func runCardCmdsNav(s *State, w, h float32, stack bool, cmds []modalCmd) (Action, bool) {
	if len(cmds) > 0 {
		s.modalCursor = input.CursorUpDown(s.modalCursor, len(cmds))
		if editorCommitPressed() && s.modalCursor >= 0 && s.modalCursor < len(cmds) {
			return cmds[s.modalCursor].run(), true
		}
	}
	return runCardCmds(w, h, stack, cmds)
}

// runCardCmds is the shared tail of the confirm/menu modal updaters: center a
// (w, h) card, lay the cmd buttons (row or stack), and dispatch via runModalCmds.
func runCardCmds(w, h float32, stack bool, cmds []modalCmd) (Action, bool) {
	card := centeredCardRect(w, h)
	var rects []rl.Rectangle
	if stack {
		rects = modalButtonStack(card, cmdLabels(cmds))
	} else {
		rects = modalButtonRow(card, cmdLabels(cmds))
	}
	return runModalCmds(cmds, rects)
}

// modifiers snapshots the three chord keys (either side counts). Centralized so
// callers can't drift on which sides they check.
func modifiers() (ctrl, shift, alt bool) {
	ctrl = rl.IsKeyDown(rl.KeyLeftControl) || rl.IsKeyDown(rl.KeyRightControl)
	shift = rl.IsKeyDown(rl.KeyLeftShift) || rl.IsKeyDown(rl.KeyRightShift)
	alt = rl.IsKeyDown(rl.KeyLeftAlt) || rl.IsKeyDown(rl.KeyRightAlt)
	return ctrl, shift, alt
}

// updateHotkeys handles keyboard shortcuts when no text field is focused.
func updateHotkeys(s *State) {
	ctrl, shift, alt := modifiers()

	// ALT tap (released without a chord) toggles the glyph overlay. Any key
	// pressed during the Alt hold sets altChordUsed so Alt+1..6 etc. don't toggle.
	altPressed := rl.IsKeyPressed(rl.KeyLeftAlt) || rl.IsKeyPressed(rl.KeyRightAlt)
	altReleased := rl.IsKeyReleased(rl.KeyLeftAlt) || rl.IsKeyReleased(rl.KeyRightAlt)
	if altPressed {
		s.altChordUsed = false
	}
	if alt && rl.GetKeyPressed() != 0 {
		// A key pressed this frame while Alt held = a chord, not a tap. Drain the
		// queue.
		s.altChordUsed = true
		// raylib's key-press queue is bounded (MAX_KEY_PRESSED_QUEUE == 16); cap the
		// drain anyway so a backend that never returns 0 can't wedge this frame.
		for i := 0; i < 64 && rl.GetKeyPressed() != 0; i++ {
		}
	}
	if altReleased && !s.altChordUsed {
		toggleTileGlyphs(s)
	}

	// Alt+1..N jumps directly to a selectable layer. Number row only.
	if alt {
		for i := 0; i < len(selectableLayers) && i < len(numberRowKeys); i++ {
			if rl.IsKeyPressed(numberRowKeys[i]) {
				s.layer = selectableLayers[i]
				return
			}
		}
	}

	// 1..9 select a palette brush; Shift+1..9 picks 9..17. Past index 17 is mouse-only.
	palette := layerBrushes[s.layer]
	if !ctrl && !alt {
		offset := 0
		if shift {
			offset = len(numberRowKeys) // Shift shifts the whole row past the unshifted range
		}
		for i, k := range numberRowKeys {
			idx := i + offset
			if idx >= len(palette) {
				break
			}
			if rl.IsKeyPressed(k) {
				s.brushIdx[s.layer] = idx
				recordRecentBrush(s)
			}
		}
	}

	switch {
	case ctrl && shift && rl.IsKeyPressed(rl.KeyZ):
		redoOne(s)
	case ctrl && shift && rl.IsKeyPressed(rl.KeyF):
		fillEntireLayer(s)
	case ctrl && rl.IsKeyPressed(rl.KeyZ):
		undoOne(s)
	case ctrl && rl.IsKeyPressed(rl.KeyY):
		redoOne(s)
	case ctrl && rl.IsKeyPressed(rl.KeyC):
		copySelection(s)
	case ctrl && rl.IsKeyPressed(rl.KeyV):
		pasteSelection(s, s.hoverX, s.hoverZ)
	case rl.IsKeyPressed(rl.KeyEscape) && s.selActive:
		s.selActive = false
		s.flash("Selection cleared")
	case ctrl && rl.IsKeyPressed(rl.KeyS):
		saveCurrent(s)
	case ctrl && rl.IsKeyPressed(rl.KeyO):
		requestOpen(s)
	case ctrl && rl.IsKeyPressed(rl.KeyN):
		newMap(s)
	}

	// G centers the view on the player start.
	if !ctrl && !alt && rl.IsKeyPressed(rl.KeyG) {
		centerViewOnTile(s, s.area.StartTileX, s.area.StartTileZ)
	}

	// Tab / Shift+Tab cycle selectable layers (skips Walls — faces via right-click).
	if !ctrl && editorTabPressed() {
		dir := 1
		if shift {
			dir = -1
		}
		s.layer = cycleSelectableLayer(s.layer, dir)
	}

	// I toggles the isometric preview vs the top-down grid.
	if !ctrl && !alt && rl.IsKeyPressed(rl.KeyI) {
		toggleIsoView(s)
	}

	// F5 playtests the in-memory area. Ctrl+F5 overrides StartTile to the cursor
	// (restored on return) so you can test a far room without walking to it.
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

	// Brush size cycling (grid layers only).
	if !ctrl && rl.IsKeyPressed(rl.KeyLeftBracket) {
		stepBrushSize(s, -1)
	}
	if !ctrl && rl.IsKeyPressed(rl.KeyRightBracket) {
		stepBrushSize(s, +1)
	}

	// PgUp/PgDn step the active level (toolbar Lvl -/+ accelerators).
	if rl.IsKeyPressed(rl.KeyPageUp) {
		stepEditLevel(s, +1)
	}
	if rl.IsKeyPressed(rl.KeyPageDown) {
		stepEditLevel(s, -1)
	}

	if !ctrl && rl.IsKeyPressed(rl.KeyHome) {
		resetView(s)
	}

	// R cycles start facing, gated to the player-start brush.
	if !ctrl && rl.IsKeyPressed(rl.KeyR) && s.layer == LayerEntities && s.activeBrush().Entity == entityPlayerStart {
		s.area.StartFacing = core.NormalizeFacing(s.area.StartFacing + 1)
		s.dirty = true
	}

	// T cycles the day/night preview phase (seeds StepCount on F5).
	if !ctrl && rl.IsKeyPressed(rl.KeyT) {
		cyclePreviewPhase(s)
	}

	updateGridCursor(s)
}

// brushSizeSteps are the brush widths cycled with [ / ]. Keep odd —
// applyToolBrushed uses brushSize/2 as the radius, centered on the cursor.
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

// Toolbar buttons and their hotkey twins share these helpers so they can't drift.

// stepBrushSize cycles the brush width one step in dir.
func stepBrushSize(s *State, dir int) {
	s.brushSize = stepBrush(s.brushSize, dir)
}

// growLevelRange extends the Levels-panel span outward to include lvl and reveals
// it — the single home for "make this level visible in the panel".
func growLevelRange(s *State, lvl int) {
	if lvl > s.topLevel {
		s.topLevel = lvl
	}
	if lvl < s.bottomLevel {
		s.bottomLevel = lvl
	}
	s.levelHidden[lvl] = false
}

// stepEditLevel changes the active level by dir (clamped), grows the panel range
// to include it, and flashes. Shared by the Levels panel ±, toolbar, and PgUp/Dn.
func stepEditLevel(s *State, dir int) {
	s.editLevel = clampLevel(s.editLevel + dir)
	growLevelRange(s, s.editLevel)
	s.flash("Active level " + signedLevelLabel(s.editLevel))
}

// maxAreaLevel / minAreaLevel return the highest / lowest elevation level any
// tile uses, clamped to range; both default to the ground baseline for a flat map.
func maxAreaLevel(a core.AreaDefinition) int {
	hi := core.ElevationBaseline
	for z := 0; z < a.Height; z++ {
		for x := 0; x < a.Width; x++ {
			if l := a.ElevationLevelAt(x, z); l > hi {
				hi = l
			}
		}
	}
	return clampLevel(hi)
}

func minAreaLevel(a core.AreaDefinition) int {
	lo := core.ElevationBaseline
	for z := 0; z < a.Height; z++ {
		for x := 0; x < a.Width; x++ {
			if l := a.ElevationLevelAt(x, z); l < lo {
				lo = l
			}
		}
	}
	return clampLevel(lo)
}

// handleLevelsPanelClick dispatches a left-click in the Levels panel: range
// steppers, a per-level eye (Alt-click solos), or a row-select to the active floor.
func handleLevelsPanelClick(s *State, mp rl.Vector2) {
	// Header −/+ step the active level (same as toolbar Floor −/+ and PgDn/PgUp).
	minus, plus := levelStepperRects(s)
	if pointIn(mp, minus) {
		stepEditLevel(s, -1)
		return
	}
	if pointIn(mp, plus) {
		stepEditLevel(s, +1)
		return
	}
	// Eye toggles first so it doesn't also re-select. Row i maps to level base+i.
	base := levelScrollBase(s)
	for i := 0; i < visibleLevelRows(s); i++ {
		if pointIn(mp, levelEyeRect(s, i)) {
			lvl := base + i
			if _, _, alt := modifiers(); alt {
				// Solo toggle: reveal all if lvl is already the only one shown, else
				// hide all but lvl. editLevel is force-revealed below, so the probe
				// must ignore it — otherwise it reads as a second visible level and
				// solo never detects the soloed state.
				soloed := !s.levelHidden[lvl]
				for j := range s.levelHidden {
					if j != lvl && j != s.editLevel && !s.levelHidden[j] {
						soloed = false
						break
					}
				}
				for j := range s.levelHidden {
					s.levelHidden[j] = !soloed && j != lvl
				}
				s.levelHidden[s.editLevel] = false
			} else if lvl != s.editLevel {
				// drawGrid always shows the active level, so refuse to mark it hidden
				// — keeps the eye icon honest.
				s.levelHidden[lvl] = !s.levelHidden[lvl]
			}
			return
		}
	}
	if i := levelRowAt(s, mp); i >= 0 {
		s.editLevel = clampLevel(i)
		growLevelRange(s, s.editLevel) // selecting a floor reveals it
	}
}

// resetView snaps the canvas to 1× zoom, centered.
func resetView(s *State) {
	s.zoom = 1
	s.panX, s.panY = 0, 0
}

// toggleTileGlyphs flips the per-tile glyph overlay.
func toggleTileGlyphs(s *State) {
	s.showTileGlyphs = !s.showTileGlyphs
}

// toggleLayerVisibility flips layer i's hidden flag. solo (Alt-click) isolates i,
// or reveals all again if i is already the only visible layer.
func toggleLayerVisibility(s *State, i int, solo bool) {
	if i < 0 || i >= layerCount {
		return
	}
	if !solo {
		s.layerHidden[i] = !s.layerHidden[i]
		return
	}
	soloed := !s.layerHidden[i]
	for j := 0; j < layerCount; j++ {
		if j != i && !s.layerHidden[j] {
			soloed = false
			break
		}
	}
	if soloed {
		for j := range s.layerHidden {
			s.layerHidden[j] = false
		}
		s.flash("All layers shown")
	} else {
		for j := range s.layerHidden {
			s.layerHidden[j] = j != i
		}
		s.flash("Soloed " + layerName(Layer(i)))
	}
}

// cyclePreviewPhase advances the day/night preview phase one step.
func cyclePreviewPhase(s *State) {
	s.previewPhase = core.WrapEnum(s.previewPhase, 1, core.TimeOfDayCount)
	s.flash("Preview: " + core.PhaseName(s.previewPhase))
}

// gridCursorDirs pairs each cursor-step delta with its input predicate. Package-level
// so updateGridCursor doesn't rebuild the slice every frame.
var gridCursorDirs = [...]struct {
	pressed func() bool
	dx, dz  int
}{
	{input.ArrowLeftPressed, -1, 0},
	{input.ArrowRightPressed, 1, 0},
	{input.ArrowUpPressed, 0, -1},
	{input.ArrowDownPressed, 0, 1},
}

func updateGridCursor(s *State) {
	if s.area.Width == 0 || s.area.Height == 0 {
		return
	}
	mw := s.area.Width
	mh := s.area.Height
	moved := false
	// Arrow / D-pad / left stick walk the grid cursor (clamp, not wrap).
	// Table-driven so the four directions share the activate→clamp→moved step.
	// gridCursorDirs is a package var (not a per-frame slice literal) — updateHotkeys
	// runs this every steady-state editing frame.
	for _, dir := range gridCursorDirs {
		if !dir.pressed() {
			continue
		}
		s.gridCursorX, s.gridCursorZ = activateCursor(s, mw, mh)
		s.gridCursorX = core.Clamp(s.gridCursorX+dir.dx, 0, mw-1)
		s.gridCursorZ = core.Clamp(s.gridCursorZ+dir.dz, 0, mh-1)
		moved = true
	}
	if moved && s.gridCursorX >= 0 {
		s.hoverX, s.hoverZ = s.gridCursorX, s.gridCursorZ
	}
	if s.gridCursorX < 0 {
		return
	}
	if input.EditorPaintPressed() {
		keyboardMutate(s, func() { applyToolBrushed(s, s.gridCursorX, s.gridCursorZ) })
	}
	if input.EditorErasePressed() {
		keyboardMutate(s, func() { eraseAt(s, s.gridCursorX, s.gridCursorZ) })
	}
}

// keyboardMutate runs a single-cell keyboard paint/erase and banks undo lazily —
// only when the cell actually changed — repairing applyTool's optimistic dirty
// flip on a no-op. Mirrors strokePaint's mouse-path guard.
func keyboardMutate(s *State, apply func()) {
	wasDirty := s.dirty
	before := core.CloneArea(s.area)
	apply()
	if core.AreaContentEqual(s.area, before) {
		s.dirty = wasDirty
		return
	}
	commitUndoSnapshot(s, before)
}

func activateCursor(s *State, mw, mh int) (int, int) {
	if s.gridCursorX >= 0 {
		return s.gridCursorX, s.gridCursorZ
	}
	x := core.Clamp(s.area.StartTileX, 0, mw-1)
	z := core.Clamp(s.area.StartTileZ, 0, mh-1)
	return x, z
}

// updateMouse processes top-bar / palette / metadata clicks and grid painting.
func updateMouse(s *State) {
	mp := rl.GetMousePosition()

	hx, hz := s.cellAt(mp)
	s.hoverX, s.hoverZ = hx, hz

	// 3D view owns the canvas (cellAt returned -1 in iso, so top-down paint is
	// inert). Side panels are mouse-inert here; `I` returns to top-down.
	if s.isoView {
		updateIsoCanvas(s, mp)
		return
	}

	// Context menu absorbs all input while open.
	if updateContextMenu(s) {
		return
	}

	if pointIn(mp, s.rect.grid) {
		w := rl.GetMouseWheelMove()
		if w != 0 {
			zoomBy(s, mp, 1+canvasZoomWheelRate*w)
		}
	} else if pointIn(mp, s.rect.palette) {
		// Wheel scrolls the brush list (~1.5 rows/notch).
		w := rl.GetMouseWheelMove()
		if w != 0 {
			ScrollPalette(s, -w*paletteRowStride*paletteWheelRows)
		}
	} else if pointIn(mp, s.rect.metadata) {
		// Wheel scrolls the MAP panel (~1 row/notch).
		w := rl.GetMouseWheelMove()
		if w != 0 {
			ScrollMetadata(s, -w*metadataRowStride)
		}
	} else if pointIn(mp, s.rect.levels) {
		// Wheel steps the active floor (grows + scrolls the window).
		if w := rl.GetMouseWheelMove(); w > 0 {
			stepEditLevel(s, +1)
		} else if w < 0 {
			stepEditLevel(s, -1)
		}
	}

	// Scrollbars run before paint/pan so grabbing a thumb doesn't bleed into them.
	if s.updateScrollbars(mp) {
		return
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
		if hit := topbarButtonAt(s, mp); hit >= 0 {
			menuBarBtns[hit].action(s) // opens that menu's pull-down (menus.go)
			return
		}
		// Top-bar layer dropdown: the active-layer picker (rows carry the eye).
		if pointIn(mp, layerMenuBtnRect(s)) {
			openDropdownBelow(s, ddLayer, layerMenuBtnRect(s))
			return
		}
		if hit := toolbarButtonAt(s, mp); hit >= 0 {
			// Disabled buttons swallow the click without firing.
			if b := toolbarBtns[hit]; b.enabled == nil || b.enabled(s) {
				b.action(s)
			}
			return
		}
		// Levels panel, checked before the palette so its column isn't swallowed.
		if pointIn(mp, s.rect.levels) {
			handleLevelsPanelClick(s, mp)
			return
		}
		if hit := paletteToolAt(s, mp); hit >= 0 {
			s.brushIdx[s.layer] = hit
			recordRecentBrush(s)
			return
		}
		if handleMetadataClick(s, mp) {
			return
		}
	}

	// Minimap click-to-jump recenters the view. Checked before grid-paint since
	// the minimap overlaps the grid pane.
	if mr, ok := minimapRect(s); ok && rl.IsMouseButtonPressed(rl.MouseLeftButton) && pointIn(mp, mr) {
		scale := mr.Width / float32(s.area.Width)
		tx := core.Clamp(int((mp.X-mr.X)/scale), 0, s.area.Width-1)
		tz := core.Clamp(int((mp.Y-mr.Y)/scale), 0, s.area.Height-1)
		centerViewOnTile(s, tx, tz)
		return
	}

	// Recent-brush quick-pick: a swatch click jumps to that layer + brush.
	if brushRecentsVisible(s) && rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		for i := range s.recentBrushes {
			if pointIn(mp, brushRecentRect(s, i)) {
				ref := s.recentBrushes[i]
				s.layer = ref.layer
				if ref.idx >= 0 && ref.idx < len(layerBrushes[ref.layer]) {
					s.brushIdx[ref.layer] = ref.idx
				}
				recordRecentBrush(s)
				return
			}
		}
	}

	if pointIn(mp, s.rect.grid) && hx >= 0 {
		ctrl, shift, alt := modifiers()

		if rl.IsMouseButtonPressed(rl.MouseLeftButton) {
			// Eyedropper: Alt+click samples the cell's char into the active
			// brush. Mark the Alt chord used so the release doesn't toggle glyphs.
			if alt && isGridLayer(s.layer) {
				sampleBrushAt(s, hx, hz)
				s.altChordUsed = true
				return
			}
			startDrag(s, hx, hz, ctrl, shift)
		}
		if rl.IsMouseButtonDown(rl.MouseLeftButton) {
			continueDrag(s, hx, hz)
		}
		if rl.IsMouseButtonPressed(rl.MouseRightButton) {
			// Ramp mode: right-click clears a ramp (floor → auto), keeping the
			// elevation digit so the cliff stays. No-op on a non-ramp tile.
			if s.rampMode {
				if _, ok := s.area.RampAt(hx, hz); ok {
					pushUndo(s)
					setLayerCell(&s.area.Floor, hx, hz, core.FloorAuto)
					s.dirty = true
				}
				return
			}
			// Right-click opens the context menu (erasing is a selectable brush).
			openContextMenu(s, mp.X, mp.Y, hx, hz)
		}
	}

	if rl.IsMouseButtonReleased(rl.MouseLeftButton) {
		finishDrag(s)
	}
}

// startDrag picks a drag kind from layer + cell contents + modifiers (grid
// default paint; Shift rect; Ctrl flood; entity brushes grab an entity to move).
func startDrag(s *State, x, z int, ctrl, shift bool) {
	// Ramp mode: left-click drops a ramp (direction + low level from neighbors).
	// placeRamp snapshots undo on success.
	if s.rampMode {
		placeRamp(s, x, z)
		s.drag = dragNone
		return
	}
	// Entity layer: grab an existing entity for drag-move, or place a fresh one.
	if s.layer == LayerEntities {
		brush := s.activeBrush()
		switch brush.Entity {
		case entityPlayerStart:
			if s.area.StartTileX == x && s.area.StartTileZ == z {
				s.drag = dragStart
				return
			}
		case entityAddEnemy:
			// On an existing pack: grab for drag-move, or open its editor on
			// release-in-place (finishDrag's dragPack branch). Empty: applyTool places.
			if idx := core.PackSpawnIndexAt(s.area.PackSpawns, x, z); idx >= 0 {
				s.drag = dragPack
				s.dragPackIdx = idx
				return
			}
		case entityPlaceChest:
			// Same drag-move / release-to-edit flow as packs.
			if idx := core.ChestSpawnIndexAt(s.area.ChestSpawns, x, z); idx >= 0 {
				s.drag = dragChest
				s.dragChestIdx = idx
				return
			}
		case entityPlaceDoor:
			// Same drag-move / release-to-edit flow as packs.
			if idx := core.DoorSpawnIndexAt(s.area.DoorSpawns, x, z); idx >= 0 {
				s.drag = dragDoor
				s.dragDoorIdx = idx
				return
			}
		}
		// Empty cell: place a fresh entity.
		beginPaintStroke(s)
		strokePaint(s, x, z)
		s.lastPaintX, s.lastPaintZ = x, z
		return
	}

	// Grid layers: the active tool decides. Under Brush, Ctrl=Flood / Shift=Rect
	// still override; a picked tool is honored as-is. (Alt=Pick handled earlier.)
	tool := s.tool
	if tool == toolBrush {
		if ctrl {
			tool = toolFlood
		} else if shift {
			tool = toolRect
		}
	}
	switch tool {
	case toolPick:
		sampleBrushAt(s, x, z)
		s.drag = dragNone
	case toolFlood:
		// floodFill snapshots undo itself (only on change). The eraser brush has
		// Char==0, so resolve the layer's erase sentinel instead of writing NUL.
		b := s.activeBrush()
		fill := b.Char
		if b.Erase {
			fill = eraseSentinel(s.layer)
		}
		floodFill(s, x, z, fill, b.Erase)
		s.drag = dragNone
	case toolRect:
		s.drag = dragRect
		s.rectHollow = false
		s.rectAnchorX, s.rectAnchorZ = x, z
	case toolBox:
		s.drag = dragRect
		s.rectHollow = true
		s.rectAnchorX, s.rectAnchorZ = x, z
	case toolLine:
		s.drag = dragLine
		s.rectAnchorX, s.rectAnchorZ = x, z
	case toolSelect:
		s.drag = dragSelect
		s.rectAnchorX, s.rectAnchorZ = x, z
	default: // toolBrush
		beginPaintStroke(s)
		strokePaint(s, x, z)
		s.lastPaintX, s.lastPaintZ = x, z
	}
}

// beginPaintStroke arms a paint/place drag, recording the pre-stroke area for one
// lazy undo step — committed only when a cell actually changes.
func beginPaintStroke(s *State) {
	s.drag = dragPaint
	s.dragSnapshotDone = false
	s.dragUndoBefore = core.CloneArea(s.area)
	s.lastPaintX, s.lastPaintZ = -1, -1
}

// strokePaint applies the active tool at (x,z) as one stroke cell, committing the
// stroke's single undo snapshot lazily (once, only on real change) and repairing
// applyTool's optimistic dirty flip when the brush refused the cell.
func strokePaint(s *State, x, z int) {
	wasDirty := s.dirty
	applyToolBrushed(s, x, z)
	if s.dragSnapshotDone {
		return // already banked this stroke's snapshot
	}
	if core.AreaContentEqual(s.area, s.dragUndoBefore) {
		s.dirty = wasDirty // refused / no-op cell: undo the optimistic dirty flip
		return
	}
	commitUndoSnapshot(s, s.dragUndoBefore)
	s.dragSnapshotDone = true
}

func continueDrag(s *State, x, z int) {
	switch s.drag {
	case dragPaint:
		if x == s.lastPaintX && z == s.lastPaintZ {
			return
		}
		// Interpolate from the last painted cell so a fast sweep lays a
		// continuous stroke. First cell (lastPaint == -1) just stamps.
		if s.lastPaintX >= 0 {
			paintLineBetween(s, s.lastPaintX, s.lastPaintZ, x, z)
		} else {
			strokePaint(s, x, z)
		}
		s.lastPaintX, s.lastPaintZ = x, z
	}
	// dragStart / dragPack / dragChest / dragDoor / dragRect commit on release.
}

// paintLineBetween stamps the brush from (x0,z0) to (x1,z1), skipping the start.
// Each step goes through strokePaint (shared Bresenham walkLine, ops.go).
func paintLineBetween(s *State, x0, z0, x1, z1 int) {
	walkLine(x0, z0, x1, z1, func(cx, cz int) {
		if cx == x0 && cz == z0 {
			return // start already painted
		}
		strokePaint(s, cx, cz)
	})
}

// finishEntityDragRelease is the shared release path for the pack / chest / door
// drag branches: release on the pick-up tile opens that entity's edit modal;
// elsewhere it runs place-blockers (flashing the first) then banks undo, moves,
// and marks dirty. The varying parts (validity, current tile, blockers, modal,
// move) come in as closures. No-op when off-map or the index is stale.
func finishEntityDragRelease(s *State, valid bool, curX, curZ int, blockers func() string, openModal func(), move func()) {
	if s.hoverX < 0 || !valid {
		return
	}
	if curX == s.hoverX && curZ == s.hoverZ {
		openModal()
		return
	}
	if msg := blockers(); msg != "" {
		s.flash(msg)
		return
	}
	pushUndo(s)
	move()
	s.dirty = true
}

func finishDrag(s *State) {
	switch s.drag {
	case dragStart:
		if s.hoverX >= 0 && (s.hoverX != s.area.StartTileX || s.hoverZ != s.area.StartTileZ) {
			// Shared startBlockers so the drag path can't drift from the
			// entity-brush / right-click paths (it once missed the door check).
			if msg := firstBlocker(startBlockers(&s.area, s.hoverX, s.hoverZ)...); msg != "" {
				s.flash(msg)
			} else {
				pushUndo(s)
				s.area.StartTileX = s.hoverX
				s.area.StartTileZ = s.hoverZ
				s.dirty = true
			}
		}
	case dragPack:
		sp := core.PackSpawn{}
		valid := s.dragPackIdx >= 0 && s.dragPackIdx < len(s.area.PackSpawns)
		if valid {
			sp = s.area.PackSpawns[s.dragPackIdx]
		}
		finishEntityDragRelease(s, valid, sp.TileX, sp.TileZ,
			// Shared packPlaceBlockers so it can't drift from the brush path.
			func() string { return firstBlocker(packPlaceBlockers(&s.area, s.hoverX, s.hoverZ)...) },
			// Release-in-place opens the inline pack editor.
			func() { openPackEditModal(s, s.dragPackIdx) },
			func() {
				// Replace any pack at the destination, then move the dragged pack
				// (located by old coords — one pack per cell).
				s.area.PackSpawns = removePackAt(s.area.PackSpawns, s.hoverX, s.hoverZ)
				if idx := core.PackSpawnIndexAt(s.area.PackSpawns, sp.TileX, sp.TileZ); idx >= 0 {
					s.area.PackSpawns[idx].TileX = s.hoverX
					s.area.PackSpawns[idx].TileZ = s.hoverZ
				}
			})
	case dragChest:
		valid := s.dragChestIdx >= 0 && s.dragChestIdx < len(s.area.ChestSpawns)
		c := core.ChestSpawn{}
		if valid {
			c = s.area.ChestSpawns[s.dragChestIdx]
		}
		finishEntityDragRelease(s, valid, c.TileX, c.TileZ,
			func() string { return firstBlocker(chestPlaceBlockers(&s.area, s.hoverX, s.hoverZ)...) },
			// Release-in-place opens the loot editor.
			func() { openChestEditModal(s, s.dragChestIdx) },
			func() {
				s.area.ChestSpawns[s.dragChestIdx].TileX = s.hoverX
				s.area.ChestSpawns[s.dragChestIdx].TileZ = s.hoverZ
			})
	case dragDoor:
		valid := s.dragDoorIdx >= 0 && s.dragDoorIdx < len(s.area.DoorSpawns)
		d := core.DoorSpawn{}
		if valid {
			d = s.area.DoorSpawns[s.dragDoorIdx]
		}
		finishEntityDragRelease(s, valid, d.TileX, d.TileZ,
			func() string { return firstBlocker(doorPlaceBlockers(&s.area, s.hoverX, s.hoverZ)...) },
			func() { openDoorEditModal(s, s.dragDoorIdx) },
			func() {
				s.area.DoorSpawns[s.dragDoorIdx].TileX = s.hoverX
				s.area.DoorSpawns[s.dragDoorIdx].TileZ = s.hoverZ
			})
	case dragRect:
		if s.hoverX >= 0 {
			// Snapshot-then-compare (not eager pushUndo): an empty / all-refused
			// rect must not bank a junk undo. Mirrors strokePaint's lazy commit.
			wasDirty := s.dirty
			before := core.CloneArea(s.area)
			if s.rectHollow {
				paintRectOutline(s, s.rectAnchorX, s.rectAnchorZ, s.hoverX, s.hoverZ)
			} else {
				paintRect(s, s.rectAnchorX, s.rectAnchorZ, s.hoverX, s.hoverZ)
			}
			if core.AreaContentEqual(s.area, before) {
				s.dirty = wasDirty // no-op rect — undo the optimistic dirty flip
			} else {
				commitUndoSnapshot(s, before)
			}
		}
	case dragLine:
		if s.hoverX >= 0 {
			wasDirty := s.dirty
			before := core.CloneArea(s.area)
			paintLine(s, s.rectAnchorX, s.rectAnchorZ, s.hoverX, s.hoverZ)
			if core.AreaContentEqual(s.area, before) {
				s.dirty = wasDirty // no-op line — don't bank undo / clobber redo
			} else {
				commitUndoSnapshot(s, before)
			}
		}
	case dragSelect:
		// Commit the marquee as the active selection (normalized inclusive bounds).
		if s.hoverX >= 0 {
			s.selX0, s.selX1 = min(s.rectAnchorX, s.hoverX), max(s.rectAnchorX, s.hoverX)
			s.selZ0, s.selZ1 = min(s.rectAnchorZ, s.hoverZ), max(s.rectAnchorZ, s.hoverZ)
			s.selActive = true
		}
	}
	s.drag = dragNone
	s.rectHollow = false
	s.dragPackIdx = -1
	s.dragChestIdx = -1
	s.dragDoorIdx = -1
}

// applyToolBrushed runs the brush over the brush-size square at (x,z). Entity
// brushes collapse to a single cell.
func applyToolBrushed(s *State, x, z int) {
	half := s.brushSize / 2
	if !isGridLayer(s.layer) || s.brushSize <= 1 || brushHasMultiTileFootprint(s) {
		applyTool(s, x, z)
		return
	}
	for dz := -half; dz <= half; dz++ {
		for dx := -half; dx <= half; dx++ {
			applyTool(s, x+dx, z+dz)
		}
	}
}

// brushHasMultiTileFootprint reports whether the active Props/Decor brush stamps
// a multi-tile footprint — those collapse to a single anchor stamp under size>1.
func brushHasMultiTileFootprint(s *State) bool {
	c := s.activeBrush().Char
	switch s.layer {
	case LayerProps:
		return core.PropFootprint(c) != nil
	case LayerDecor:
		return core.DecorFootprint(c) != nil
	}
	return false
}

func isGridLayer(l Layer) bool {
	return l != LayerEntities
}

// recordRecentBrush pushes the selected (layer, brush) onto the recents list
// (newest first, deduped, capped). Called at every brush-select site.
func recordRecentBrush(s *State) {
	ref := brushRef{s.layer, s.brushIdx[s.layer]}
	next := make([]brushRef, 0, maxRecentBrushes)
	next = append(next, ref)
	for _, r := range s.recentBrushes {
		if r == ref {
			continue
		}
		if len(next) >= maxRecentBrushes {
			break
		}
		next = append(next, r)
	}
	s.recentBrushes = next
}

// sampleBrushAt is the eyedropper (Alt+click): selects the palette brush matching
// the active layer's char at (x,z). On Elevation it picks the height instead.
// Flashes when no palette brush matches.
func sampleBrushAt(s *State, x, z int) {
	if !s.area.InBounds(x, z) {
		return
	}
	if s.layer == LayerElevation {
		// ElevationLevelAt is Solids-aware (voxel column top), so this picks the
		// real height, not the stale legacy Elevation char.
		lvl := clampLevel(s.area.ElevationLevelAt(x, z))
		s.editLevel = lvl
		growLevelRange(s, lvl) // reveal a panel row for this level
		s.flash("Picked level " + signedLevelLabel(lvl))
		return
	}
	ch, ok := activeLayerCharAt(s, x, z)
	if !ok {
		return
	}
	// On Walls the unskinned cell is '.', which reads as plain rock — pick Rock
	// rather than flashing "no brush matches" (there's no '.' brush).
	if s.layer == LayerWalls && ch == core.TileOpen {
		ch = core.TileRock
	}
	for i, b := range layerBrushes[s.layer] {
		if b.Char == ch {
			s.brushIdx[s.layer] = i
			recordRecentBrush(s)
			s.flash("Picked " + b.Name)
			return
		}
	}
	s.flash("No brush matches this tile")
}

// cellAt safely reads a grid layer at (x,z): ok is false outside the actual
// backing rows (a ragged area can be shorter than Width/Height, panicking raw
// indexing even after InBounds). Shared reader for editor-side layer reads.
func cellAt(layer []string, x, z int) (byte, bool) {
	if z < 0 || z >= len(layer) || x < 0 || x >= len(layer[z]) {
		return 0, false
	}
	return layer[z][x], true
}

// activeLayerCharAt returns the raw char at (x,z) on the active grid layer
// (including sentinels). ok is false for Entities, which has no per-tile char.
func activeLayerCharAt(s *State, x, z int) (byte, bool) {
	switch s.layer {
	case LayerWalls:
		return cellAt(s.area.Walls, x, z)
	case LayerFloor:
		return cellAt(s.area.Floor, x, z)
	case LayerDecor:
		return cellAt(s.area.Decor, x, z)
	case LayerProps:
		return cellAt(s.area.Props, x, z)
	case LayerCeiling:
		return cellAt(s.area.Ceiling, x, z)
	case LayerElevation:
		return cellAt(s.area.Elevation, x, z)
	}
	return 0, false
}

// minZoom / maxZoom bound the editor canvas zoom.
const (
	minZoom = float32(0.5)
	maxZoom = float32(4)
	// canvasZoomWheelRate: zoom factor per wheel notch (1 ± rate·notch).
	canvasZoomWheelRate = float32(0.12)
	// paletteWheelRows: brush-list rows scrolled per wheel notch.
	paletteWheelRows = float32(1.5)
)

func zoomBy(s *State, anchor rl.Vector2, factor float32) {
	prev := s.zoom
	next := prev * factor
	if next < minZoom {
		next = minZoom
	}
	if next > maxZoom {
		next = maxZoom
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

// openSaveAsModal pops the Save As dialog. Single seam for every Save As entry point.
func openSaveAsModal(s *State) {
	// Default the filename to the saved stem, or the area title for an unsaved map.
	stem := core.MapIDFromPath(s.area.Path)
	if stem == "" {
		stem = sanitizeFilename(s.area.Name)
	}
	s.modalFilename = stem
	s.modal = modalSaveAs
	s.focus = focusFilename
}

// openValidateModal snapshots reachability + cross-map door + dialog warnings
// into the modal (vs. the metadata-panel's reachBadgeMaxRows cap).
func openValidateModal(s *State) {
	rows := append([]string{}, reachabilityWarnings(s.area)...)
	rows = append(rows, crossMapDoorWarnings(s.area)...)
	rows = append(rows, dialogWarnings(s.area)...)
	s.modalValidateRows = rows
	s.modal = modalValidate
}

// textFieldConfig is the rune-budget + accept-filter for one focusable text
// field. textFieldConfigs is the single source of truth (one row per focusField).
type textFieldConfig struct {
	MaxLen int
	Accept func(rune) bool
}

// defaultTextFieldMaxLen is the rune budget for general-purpose editor text
// fields (names, filenames, door target paths).
const defaultTextFieldMaxLen = 96

// textFieldConfigs maps each focusField to its rune-budget + accept rule. Foci
// not here (incl. the numeric width/height, handled by updateNumericInput) fall
// back via configForFocus.
var textFieldConfigs = map[focusField]textFieldConfig{
	focusName:         {defaultTextFieldMaxLen, acceptPrintable},
	focusLocationName: {defaultTextFieldMaxLen, acceptPrintable},
	focusQuiet:        {defaultTextFieldMaxLen, acceptPrintable},
	focusFilename:     {defaultTextFieldMaxLen, acceptPrintable},
	// Door identifier fields reject spaces — the .map door row is space-delimited.
	focusDoorName:       {defaultTextFieldMaxLen, acceptPrintableNoSpace},
	focusDoorTargetMap:  {defaultTextFieldMaxLen, acceptPrintableNoSpace},
	focusDoorTargetDoor: {defaultTextFieldMaxLen, acceptPrintableNoSpace},
	// Dialog: prose fields allow spaces; id-target fields reject them.
	focusDialogNodeText:     {dialogProseMaxLen, acceptPrintable},
	focusDialogNodeNext:     {defaultTextFieldMaxLen, acceptPrintableNoSpace},
	focusDialogNodeContinue: {defaultTextFieldMaxLen, acceptPrintable},
	focusDialogChoiceLabel:  {dialogProseMaxLen, acceptPrintable},
	focusDialogChoiceNext:   {defaultTextFieldMaxLen, acceptPrintableNoSpace},
	focusDialogActionID:     {defaultTextFieldMaxLen, acceptPrintableNoSpace}, // quest/event-id key
	focusDialogCondQuestID:  {defaultTextFieldMaxLen, acceptPrintableNoSpace},
	focusDialogCondMessage:  {dialogProseMaxLen, acceptPrintable},
	focusDialogCondGold:     {dialogNumFieldMaxLen, acceptDigit},
	focusDialogCondFoeKills: {dialogNumFieldMaxLen, acceptDigit},
	focusDialogCondTileX:    {dialogNumFieldMaxLen, acceptDigit},
	focusDialogCondTileZ:    {dialogNumFieldMaxLen, acceptDigit},
	focusDialogTrigTileX:    {dialogNumFieldMaxLen, acceptDigit},
	focusDialogTrigTileZ:    {dialogNumFieldMaxLen, acceptDigit},
	focusDialogTrigFoeKills: {dialogNumFieldMaxLen, acceptDigit},
}

// dialogNumFieldMaxLen caps the shared numeric edit buffer (gold gates, coords).
const dialogNumFieldMaxLen = 6

// dialogProseMaxLen is the rune budget for dialog body text + choice labels.
const dialogProseMaxLen = 280

func configForFocus(f focusField) textFieldConfig {
	if cfg, ok := textFieldConfigs[f]; ok {
		return cfg
	}
	// Defensive default for a focus wired up before its row exists.
	return textFieldConfig{MaxLen: defaultTextFieldMaxLen, Accept: acceptPrintable}
}

// pumpFocusField pumps printable runes into `target` using s.focus's config.
func pumpFocusField(s *State, target *string) {
	cfg := configForFocus(s.focus)
	pumpPrintableASCII(target, cfg.MaxLen, cfg.Accept, s.markDirty)
}

// pumpPrintableASCII drains queued printable-ASCII into target (capped at maxLen,
// filtered by accept, nil = any) and consumes one backspace. onChange fires per
// accepted char/backspace (may be nil). Prefer pumpFocusField for focus-keyed inputs.
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

// acceptPrintable accepts every printable ASCII rune (the default filter).
func acceptPrintable(r rune) bool { return true }

// acceptPrintableNoSpace excludes ASCII space (so Space can drive Preview, etc.).
func acceptPrintableNoSpace(r rune) bool { return r != ' ' }

// acceptDigit accepts ASCII digits only.
func acceptDigit(r rune) bool { return r >= '0' && r <= '9' }

// numericFieldMaxLen caps the resize numeric buffer (map dims ≤ 4 digits).
const numericFieldMaxLen = 4

// isNumericFocus reports whether f is one of the digit-only dimension fields that
// route through updateNumericInput (map + new-map width/height).
func isNumericFocus(f focusField) bool {
	switch f {
	case focusWidth, focusHeight, focusNewWidth, focusNewHeight:
		return true
	}
	return false
}

func updateTextInput(s *State) {
	if isNumericFocus(s.focus) {
		updateNumericInput(s)
		return
	}
	target := activeTextTarget(s)
	if target == nil {
		return
	}
	pumpFocusField(s, target)
	if editorTabPressed() {
		cycleFocus(s)
		return
	}
	if editorCommitPressed() {
		if s.focus == focusFilename {
			confirmModal(s)
			return
		}
		s.focus = focusNone
	}
	if editorCancelPressed() {
		if s.focus == focusFilename {
			closeModal(s)
		}
		s.focus = focusNone
	}
}

// updateNumericInput is the special-case path for the map width/height fields:
// clamps to ClampMapDimension and drives a live resize. Plain int fields use
// pumpDialogNumeric (editor/dialog.go).
func updateNumericInput(s *State) {
	pumpPrintableASCII(&s.numericBuf, numericFieldMaxLen, acceptDigit, nil)
	if editorTabPressed() {
		commitNumericInput(s)
		cycleFocus(s)
		return
	}
	if editorCommitPressed() {
		commitNumericInput(s)
		s.focus = focusNone
	}
	if editorCancelPressed() {
		s.numericBuf = ""
		s.focus = focusNone
	}
}

func commitNumericInput(s *State) {
	if s.numericBuf == "" {
		return
	}
	// Digit-only + capped, so Atoi can't realistically fail; bail if it does.
	v, err := strconv.Atoi(s.numericBuf)
	if err != nil {
		s.numericBuf = ""
		return
	}
	v = core.ClampMapDimension(v)
	switch s.focus {
	case focusWidth:
		resize(s, v, s.area.Height)
	case focusHeight:
		resize(s, s.area.Width, v)
	case focusNewWidth:
		s.modalNewWidth = v
	case focusNewHeight:
		s.modalNewHeight = v
	}
	s.numericBuf = ""
}

// finalizeFocusedField runs a per-field commit hook before focus drops, so a
// click-away can't strand mid-edit state. Only the buffered Width/Height numeric
// fields need it today (without it the typed dimension is discarded on click-away).
func finalizeFocusedField(s *State) {
	switch s.focus {
	case focusWidth, focusHeight:
		commitNumericInput(s)
	}
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
	case focusNewWidth:
		// New-map dialog has only width↔height to cycle.
		s.focus = focusNewHeight
		s.numericBuf = ""
	case focusNewHeight:
		s.focus = focusNewWidth
		s.numericBuf = ""
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
	// Close the modal if its referenced entity went out of bounds (deleted /
	// undone past the open frame), so the next frame can't deref a stale index.
	validateModalState(s)
	// A picker dropdown owns input while up — handled once here so the modal
	// behind it stays inert. No-op when closed.
	if s.dropdownOpen() {
		updateDropdown(s)
		return ActionNone
	}
	if h, ok := modalHandlers[s.modal]; ok && h.update != nil {
		return h.update(s)
	}
	return ActionNone
}

// validateModalState closes the active modal when its referenced entity has gone
// out of bounds. Single source of truth for "is this modal still pointing at
// something real?".
func validateModalState(s *State) {
	switch s.modal {
	case modalPackEdit:
		if s.modalPackIdx < 0 || s.modalPackIdx >= len(s.area.PackSpawns) {
			closeModal(s)
		}
	case modalChestEdit:
		if s.modalChestIdx < 0 || s.modalChestIdx >= len(s.area.ChestSpawns) {
			closeModal(s)
		}
	case modalDoorEdit:
		if s.modalDoorIdx < 0 || s.modalDoorIdx >= len(s.area.DoorSpawns) {
			closeModal(s)
		}
	case modalDialogList:
		// Always valid (indexes the whole slice); cursor clamped in the updater.
	case modalDialogNodes:
		if s.modalDialogIdx < 0 || s.modalDialogIdx >= len(s.area.Dialogs) {
			closeModal(s)
		}
	case modalDialogNodeEdit:
		if !dialogNodeInRange(s) {
			closeModal(s)
		}
	case modalDialogChoiceEdit:
		if !dialogChoiceInRange(s) {
			closeModal(s)
		}
	case modalDialogCondEdit:
		if !dialogCondInRange(s) {
			closeModal(s)
		}
	case modalDialogActionEdit:
		if currentDialogActionHolder(s) == nil {
			closeModal(s)
		}
	case modalDialogTriggerList:
		if s.modalDialogTriggerIdx < -1 || s.modalDialogTriggerIdx >= len(s.area.Triggers) {
			s.modalDialogTriggerIdx = -1 // list always valid; -1 = no selection (see dialog.go currentTrigger)
		}
	case modalDialogTriggerEdit:
		if !dialogTriggerInRange(s) {
			closeModal(s)
		}
	case modalLocationEdit:
		if s.modalLocationIdx < 0 || s.modalLocationIdx >= len(s.area.Locations) {
			closeModal(s)
		}
	}
}

// armOrConfirmDelete is the shared two-press delete guard: first call for a token
// arms it (flashing msg, returns false); the next for the SAME token returns true.
// A different token re-arms. Cleared on modal close.
func armOrConfirmDelete(s *State, token, msg string) bool {
	if s.deleteArmed != token {
		s.deleteArmed = token
		s.flash(msg)
		return false
	}
	s.deleteArmed = ""
	return true
}

// closeModal is the single dismiss seam: clears the modal kind + every
// modal-scoped cursor/index so a later modal can't read a stale value.
func closeModal(s *State) {
	// Free the Visualizers' cached GPU handles here too (not just their own
	// buttons) so any dismiss path — e.g. validateModalState — can't leak across
	// reopen. Idempotent.
	switch s.modal {
	case modalFoeView:
		render.CloseFoePreview()
		render.ClearAssetPreview()
	case modalPartyView:
		render.ClosePartyPreview()
		render.ClearAssetPreview()
	case modalObjectView:
		render.CloseObjectPreview()
	}
	s.modal = modalNone
	s.modalCursor = 0
	s.modalPackIdx = -1
	s.modalChestIdx = -1
	s.modalDoorIdx = -1
	s.modalDialogIdx = -1
	s.modalDialogNodeIdx = -1
	s.modalDialogChoiceIdx = -1
	s.modalDialogCondIdx = -1
	s.modalDialogTriggerIdx = -1
	s.modalDialogActionOnChoice = false
	clearDialogFocus(s)
	closeDropdown(s) // picker must not survive its parent modal
	s.modalValidateRows = nil
	s.modalConfirmDelete = false
	s.modalRenaming = ""
	s.deleteArmed = ""
	soundDrag = noSliderDrag
	// Drop modal-scoped focus (door fields, new-map numeric) so it can't carry over.
	if s.focus == focusDoorName || s.focus == focusDoorTargetMap || s.focus == focusDoorTargetDoor ||
		s.focus == focusNewWidth || s.focus == focusNewHeight || s.focus == focusLocationName {
		s.focus = focusNone
		s.numericBuf = ""
	}
}

// openPackEditModal opens the per-pack editor for spawn index idx.
func openPackEditModal(s *State, idx int) {
	if idx < 0 || idx >= len(s.area.PackSpawns) {
		return
	}
	s.modal = modalPackEdit
	s.modalPackIdx = idx
	s.modalCursor = 0
}

// openChestEditModal opens the per-chest editor for spawn index idx.
func openChestEditModal(s *State, idx int) {
	if idx < 0 || idx >= len(s.area.ChestSpawns) {
		return
	}
	s.modal = modalChestEdit
	s.modalChestIdx = idx
	s.modalCursor = 0
}

// openDoorEditModal opens the per-door editor for idx, focusing the Name field.
func openDoorEditModal(s *State, idx int) {
	if idx < 0 || idx >= len(s.area.DoorSpawns) {
		return
	}
	s.modal = modalDoorEdit
	s.modalDoorIdx = idx
	s.modalCursor = 0
	s.focus = focusDoorName
}

// updateDoorEditModal drives the door-edit modal: three text fields + facing
// buttons. Tab cycles fields; Esc closes; X deletes.
func updateDoorEditModal(s *State) Action {
	if s.modalDoorIdx < 0 || s.modalDoorIdx >= len(s.area.DoorSpawns) {
		closeModal(s)
		return ActionNone
	}
	door := &s.area.DoorSpawns[s.modalDoorIdx]

	// Mouse: click focuses a field, sets a facing/style, or deletes.
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
		case doorHitStyle:
			pushUndo(s)
			door.Style = hit.style
			s.dirty = true
			s.focus = focusNone
			return ActionNone
		case doorHitDelete:
			deleteDoorAt(s, s.modalDoorIdx)
			return ActionNone
		case doorHitClose:
			closeModal(s)
			return ActionNone
		case doorHitOutside:
			// No-op (not a close) so a stray click can't lose in-progress edits.
		}
	}

	// Keyboard: a focused text field takes every keystroke. Tab cycles, Enter
	// confirms, Esc closes.
	switch s.focus {
	case focusDoorName, focusDoorTargetMap, focusDoorTargetDoor:
		target := doorEditTextTarget(s)
		if target != nil {
			// pumpFocusField reads the cap from textFieldConfigs and its onChange
			// is s.markDirty, so no second dirty guard is needed.
			pumpFocusField(s, target)
		}
		if editorTabPressed() {
			cycleDoorFocus(s)
			return ActionNone
		}
		if editorCommitPressed() {
			s.focus = focusNone
			return ActionNone
		}
		if editorCancelPressed() {
			closeModal(s)
			return ActionNone
		}
		return ActionNone
	}

	// No field focused — facing + delete shortcuts.
	if editorCancelPressed() || editorCommitPressed() {
		closeModal(s)
		return ActionNone
	}
	if editorTabPressed() {
		s.focus = focusDoorName
		return ActionNone
	}
	if editorDeletePressed() {
		deleteDoorAt(s, s.modalDoorIdx)
		return ActionNone
	}
	// N/E/S/W set facing ('S' is free here — Ctrl+S Save doesn't fire in modals).
	for _, fk := range doorFacingKeys {
		if rl.IsKeyPressed(fk.key) {
			pushUndo(s)
			door.Facing = fk.facing
			s.dirty = true
			return ActionNone
		}
	}
	// 1/2/3 set the door style (building / cave / field).
	for _, sk := range doorStyleKeys {
		if rl.IsKeyPressed(sk.key) {
			pushUndo(s)
			door.Style = sk.style
			s.dirty = true
			return ActionNone
		}
	}
	return ActionNone
}

// doorFacingKeys / doorStyleKeys are the door modal's direct-set hotkey tables.
// Package-level so the per-frame updater doesn't rebuild them.
var doorFacingKeys = []struct {
	key    int32
	facing int
}{
	{rl.KeyN, core.North},
	{rl.KeyE, core.East},
	{rl.KeyS, core.South},
	{rl.KeyW, core.West},
}

// Keys source from numberRowKeys so "key 1/2/3" stays in lockstep with the number row.
var doorStyleKeys = []struct {
	key   int32
	style core.DoorStyle
}{
	{numberRowKeys[0], core.DoorStyleBuilding},
	{numberRowKeys[1], core.DoorStyleCave},
	{numberRowKeys[2], core.DoorStyleField},
}

// init panics if the door-modal hotkey tables drift from the core enums they bind.
func init() {
	if len(doorFacingKeys) != core.FacingCount {
		panic("editor: doorFacingKeys length must match core.FacingCount — add a row when extending the facing enum")
	}
	if len(doorStyleKeys) != int(core.DoorStyleCount) {
		panic("editor: doorStyleKeys length must match core.DoorStyleCount — add a row when extending DoorStyle")
	}
}

// deleteDoorAt removes the door at idx (undo, dirty, close). Shared by the Delete
// button and the X key.
func deleteDoorAt(s *State, idx int) {
	if idx < 0 || idx >= len(s.area.DoorSpawns) {
		return
	}
	// Two-press confirm — a door may carry hand-authored cross-map links.
	if !armOrConfirmDelete(s, "door", "Delete this door? Click Delete (or press X) again to confirm") {
		return
	}
	pushUndo(s)
	s.area.DoorSpawns = removeModalListItem(s.area.DoorSpawns, idx)
	s.dirty = true
	closeModal(s)
}

// doorEditTextTarget returns the DoorSpawn string field the focus targets.
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

// openEntityListModal opens the Objects index (packs / chests / doors + start).
func openEntityListModal(s *State) {
	s.modal = modalEntityList
	s.modalCursor = 0
}

// updateEntityListModal drives the Objects index: Up/Down, Enter/row-click jumps
// to the entity and opens its editor, Esc / click-outside closes.
func updateEntityListModal(s *State) Action {
	rows := entityListRows(s)
	n := len(rows)
	s.modalCursor = input.CursorUpDown(s.modalCursor, n)
	if editorCancelPressed() {
		closeModal(s)
		return ActionNone
	}
	if rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		card, listTop, _, top, end := entityListGeom(s)
		mp := rl.GetMousePosition()
		if !pointIn(mp, card) {
			closeModal(s)
			return ActionNone
		}
		for i := top; i < end; i++ {
			if pointIn(mp, entityListRowRect(card, listTop, i-top)) {
				activateEntityRow(s, rows[i])
				return ActionNone
			}
		}
		return ActionNone
	}
	if editorCommitPressed() && s.modalCursor >= 0 && s.modalCursor < n {
		activateEntityRow(s, rows[s.modalCursor])
	}
	return ActionNone
}

// activateEntityRow recenters on the row's entity and opens its editor (the start
// row just recenters and closes).
func activateEntityRow(s *State, row entityListRow) {
	centerViewOnTile(s, row.x, row.z)
	switch row.kind {
	case elPack:
		openPackEditModal(s, row.idx)
	case elChest:
		openChestEditModal(s, row.idx)
	case elDoor:
		openDoorEditModal(s, row.idx)
	default:
		closeModal(s)
	}
}

// updateValidateModal: any key closes; it's a read-only viewer.
func updateValidateModal(s *State) Action {
	if editorCancelPressed() || editorCommitPressed() || rl.IsKeyPressed(rl.KeySpace) {
		closeModal(s)
	}
	if rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		closeModal(s)
	}
	return ActionNone
}

// updatePackEditModal drives the inline pack editor: Up/Down navigate, Enter opens
// the add-member dropdown, X removes, K/J reorder, R toggles row, A cycles AI,
// Esc closes. Removing the last member drops the pack and closes.
func updatePackEditModal(s *State) Action {
	if s.modalPackIdx < 0 || s.modalPackIdx >= len(s.area.PackSpawns) {
		closeModal(s)
		return ActionNone
	}
	pack := &s.area.PackSpawns[s.modalPackIdx]
	memberCount := len(pack.Members)
	if !updateEntityListCursor(s, memberCount) {
		return ActionNone
	}

	// Mouse: click a row to select, or a button (from packEditCmds — same as draw).
	if handleEntityModalClick(s, memberCount, packEditCmds) {
		return ActionNone
	}

	// Keyboard: Enter opens the Add dropdown; X/K/J/R/A act on the selection.
	if editorCommitPressed() {
		openPackAddDropdown(s)
		return ActionNone
	}
	if memberCount > 0 {
		if editorDeletePressed() {
			packRemoveSelected(s, pack)
			return ActionNone
		}
		if rl.IsKeyPressed(rl.KeyK) {
			packMoveSelected(s, pack, -1)
		}
		if rl.IsKeyPressed(rl.KeyJ) {
			packMoveSelected(s, pack, +1)
		}
		if rl.IsKeyPressed(rl.KeyR) {
			packToggleSelectedRow(s, pack)
			return ActionNone
		}
	}
	if rl.IsKeyPressed(rl.KeyA) {
		pushUndo(s)
		pack.AI = core.WrapEnum(pack.AI, 1, core.PackAICount)
		s.dirty = true
		s.flash("Pack AI: " + core.PackAILabel(pack.AI))
	}
	return ActionNone
}

// handleEntityModalClick processes a left-click in a pack/chest editor: select a
// row or run an add/action button. `builder` (packEditCmds / chestEditCmds) is
// the same one the draw uses. Returns true when consumed. Shared so the two
// editors can't drift on the row→actions→adds hit order.
func handleEntityModalClick(s *State, count int, builder func(*State) (adds, actions []modalCmd)) bool {
	if !rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		return false
	}
	adds, actions := builder(s)
	lay := entityModalLayoutFor(s.modalCursor, count, cmdLabels(adds), cmdLabels(actions))
	mp := rl.GetMousePosition()
	if idx := entityRowAt(lay, mp); idx >= 0 {
		s.modalCursor = idx
		return true
	}
	for i, rect := range lay.actRects {
		if pointIn(mp, rect) {
			actions[i].run()
			return true
		}
	}
	for i, rect := range lay.addRects {
		if pointIn(mp, rect) {
			adds[i].run()
			return true
		}
	}
	return false
}

// packEditCmds builds the pack editor's buttons (+ Add member, Remove/Up/Down/
// Row/AI), shared by draw and the click handler. Caller must have validated
// s.modalPackIdx.
func packEditCmds(s *State) (adds, actions []modalCmd) {
	pack := &s.area.PackSpawns[s.modalPackIdx]
	adds = []modalCmd{
		{label: "+ Add member  (Enter)", run: func() Action { openPackAddDropdown(s); return ActionNone }},
	}
	actions = []modalCmd{
		{label: "Remove", run: func() Action { packRemoveSelected(s, pack); return ActionNone }},
		{label: "Up", run: func() Action { packMoveSelected(s, pack, -1); return ActionNone }},
		{label: "Down", run: func() Action { packMoveSelected(s, pack, +1); return ActionNone }},
		{label: "Row: " + packRowLabel(pack, s.modalCursor), run: func() Action { packToggleSelectedRow(s, pack); return ActionNone }},
		{label: "AI: " + core.PackAILabel(pack.AI) + dropdownArrowSuffix, run: func() Action { openPackAIDropdown(s); return ActionNone }},
	}
	return adds, actions
}

// packRowLabel is the Row button's caption — "Front"/"Back", or "—" when empty.
func packRowLabel(pack *core.PackSpawn, idx int) string {
	if idx < 0 || idx >= len(pack.Members) {
		return "—"
	}
	return core.RowLabel(pack.Members[idx].Row)
}

// packToggleSelectedRow flips the cursored member between the front/back rows.
func packToggleSelectedRow(s *State, pack *core.PackSpawn) {
	if s.modalCursor < 0 || s.modalCursor >= len(pack.Members) {
		return
	}
	m := &pack.Members[s.modalCursor]
	// Refuse a flip that would overfill the target rank (max 3 front / 5 back).
	front, back := core.PackRowCounts(pack.Members)
	if m.Row == core.RowBack {
		if front >= core.EnemyFrontRowCap {
			s.flash("Front row is full (max " + strconv.Itoa(core.EnemyFrontRowCap) + ")")
			return
		}
		pushUndo(s)
		m.Row = core.RowFront
	} else {
		if back >= core.EnemyBackRowCap {
			s.flash("Back row is full (max " + strconv.Itoa(core.EnemyBackRowCap) + ")")
			return
		}
		pushUndo(s)
		m.Row = core.RowBack
	}
	s.dirty = true
	s.flash(core.PackMemberDisplayName(s.area, *pack, s.modalCursor) + " → " + packRowLabel(pack, s.modalCursor) + " row")
}

// openPackAIDropdown arms the AI-mode dropdown anchored on the "AI:" button.
func openPackAIDropdown(s *State) {
	if s.modalPackIdx < 0 || s.modalPackIdx >= len(s.area.PackSpawns) {
		return
	}
	pack := &s.area.PackSpawns[s.modalPackIdx]
	adds, actions := packEditCmds(s)
	lay := entityModalLayoutFor(s.modalCursor, len(pack.Members), cmdLabels(adds), cmdLabels(actions))
	// AI is the last action button.
	anchor := lay.card
	if len(lay.actRects) > 0 {
		anchor = lay.actRects[len(lay.actRects)-1]
	}
	openDropdown(s, ddPackAI, anchor)
}

// openPackAddDropdown arms the add-member dropdown anchored on "+ Add member"
// (recomputing the layout to find its rect, identical to the draw).
func openPackAddDropdown(s *State) {
	if s.modalPackIdx < 0 || s.modalPackIdx >= len(s.area.PackSpawns) {
		return
	}
	pack := &s.area.PackSpawns[s.modalPackIdx]
	adds, actions := packEditCmds(s)
	lay := entityModalLayoutFor(s.modalCursor, len(pack.Members), cmdLabels(adds), cmdLabels(actions))
	openDropdown(s, ddPackAdd, addButtonAnchor(lay))
}

// addButtonAnchor returns the "+ Add" button rect (or the card as fallback).
// Shared by the pack + chest open paths.
func addButtonAnchor(lay entityModalLayout) rl.Rectangle {
	if len(lay.addRects) > 0 {
		return lay.addRects[0]
	}
	return lay.card
}

// packRemoveSelected removes the cursored member, dropping the pack (and closing)
// if it empties. Shared by the Remove button and X.
func packRemoveSelected(s *State, pack *core.PackSpawn) {
	if len(pack.Members) == 0 {
		return
	}
	pushUndo(s)
	core.RemovePackMember(pack, s.modalCursor)
	s.dirty = true
	if len(pack.Members) == 0 {
		s.area.PackSpawns = removeModalListItem(s.area.PackSpawns, s.modalPackIdx)
		closeModal(s)
		return
	}
	if s.modalCursor >= len(pack.Members) {
		s.modalCursor = len(pack.Members) - 1
	}
}

// packMoveSelected swaps the cursored member with its dir neighbor (no-op at the
// ends). Shared by the Up/Down buttons and K/J.
func packMoveSelected(s *State, pack *core.PackSpawn, dir int) {
	j := s.modalCursor + dir
	if j < 0 || j >= len(pack.Members) {
		return
	}
	pushUndo(s)
	core.SwapPackMembers(pack, s.modalCursor, j)
	s.modalCursor = j
	s.dirty = true
}

// updateEntityListCursor clamps the row cursor, closes on Esc/pad B, and moves it
// with Up/Down. Close is Esc-only so Enter is free to open the add dropdown.
// Returns false when the modal closed.
func updateEntityListCursor(s *State, count int) bool {
	if s.modalCursor >= count {
		s.modalCursor = count - 1
	}
	if s.modalCursor < 0 {
		s.modalCursor = 0
	}
	if editorCancelPressed() {
		closeModal(s)
		return false
	}
	if count > 0 {
		s.modalCursor = input.CursorUpDown(s.modalCursor, count)
	}
	return true
}

func removeModalListItem[T any](items []T, idx int) []T {
	if idx < 0 || idx >= len(items) {
		return items
	}
	// Copy into a fresh slice, not in-place append-shift: the in-place form
	// mutates the shared backing array and would corrupt an aliased undo snapshot.
	out := make([]T, 0, len(items)-1)
	out = append(out, items[:idx]...)
	out = append(out, items[idx+1:]...)
	return out
}

// updateChestEditModal drives the inline chest editor: Up/Down, Enter opens the
// add-item dropdown, X removes, Esc closes. An emptied chest stays (valid shape,
// rendered pre-looted).
func updateChestEditModal(s *State) Action {
	if s.modalChestIdx < 0 || s.modalChestIdx >= len(s.area.ChestSpawns) {
		closeModal(s)
		return ActionNone
	}
	chest := &s.area.ChestSpawns[s.modalChestIdx]
	itemCount := len(chest.Items)
	if !updateEntityListCursor(s, itemCount) {
		return ActionNone
	}

	// Mouse: click an item row to select, or a button to add / remove.
	if handleEntityModalClick(s, itemCount, chestEditCmds) {
		return ActionNone
	}

	// Keyboard: Enter opens the Add dropdown; X removes the selected item.
	if editorCommitPressed() {
		openChestAddDropdown(s)
		return ActionNone
	}
	if itemCount > 0 && editorDeletePressed() {
		chestRemoveSelected(s, chest)
		return ActionNone
	}
	return ActionNone
}

// chestEditCmds builds the chest editor's buttons (+ Add item, Remove), shared by
// draw and the click handler. Caller must have validated s.modalChestIdx.
func chestEditCmds(s *State) (adds, actions []modalCmd) {
	chest := &s.area.ChestSpawns[s.modalChestIdx]
	adds = []modalCmd{
		{label: "+ Add item  (Enter)", run: func() Action { openChestAddDropdown(s); return ActionNone }},
	}
	actions = []modalCmd{
		{label: "Remove", run: func() Action { chestRemoveSelected(s, chest); return ActionNone }},
	}
	return adds, actions
}

// openChestAddDropdown arms the add-item dropdown anchored on "+ Add item".
func openChestAddDropdown(s *State) {
	if s.modalChestIdx < 0 || s.modalChestIdx >= len(s.area.ChestSpawns) {
		return
	}
	chest := &s.area.ChestSpawns[s.modalChestIdx]
	adds, actions := chestEditCmds(s)
	lay := entityModalLayoutFor(s.modalCursor, len(chest.Items), cmdLabels(adds), cmdLabels(actions))
	openDropdown(s, ddChestAdd, addButtonAnchor(lay))
}

// chestRemoveSelected removes the cursored item (an emptied chest stays). Shared
// by the Remove button and X.
func chestRemoveSelected(s *State, chest *core.ChestSpawn) {
	if len(chest.Items) == 0 {
		return
	}
	pushUndo(s)
	chest.Items = removeModalListItem(chest.Items, s.modalCursor)
	s.dirty = true
	if s.modalCursor >= len(chest.Items) {
		s.modalCursor = len(chest.Items) - 1
	}
	if s.modalCursor < 0 {
		s.modalCursor = 0
	}
}

func updateOpenModal(s *State) Action {
	if s.modalRenaming != "" {
		return updateOpenRename(s)
	}
	if s.modalConfirmDelete {
		return updateOpenConfirmDelete(s)
	}

	if editorCancelPressed() {
		closeModal(s)
		return ActionNone
	}
	if len(s.modalPaths) == 0 {
		return ActionNone
	}
	s.modalCursor = input.CursorUpDown(s.modalCursor, len(s.modalPaths))

	// Mouse: click a list row to select it (buttons handled below).
	if rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		if idx := openModalRowAt(s, rl.GetMousePosition()); idx >= 0 {
			s.modalCursor = idx
			return ActionNone
		}
	}
	cmds := openModalActionCmds(s)
	if act, ran := runCardCmds(openModalW, openModalH, false, cmds); ran {
		return act
	}
	return ActionNone
}

// openModalActionCmds: 0=Open (Enter), 1=Rename (R), 2=Delete (D),
// 3=Duplicate (C). The row-click selection is handled separately in
// updateOpenModal (it isn't a button).
func openModalActionCmds(s *State) []modalCmd {
	return []modalCmd{
		{label: "Open", hot: editorCommitPressed, run: func() Action { return openSelectedMap(s) }},
		{label: "Rename", hot: keyHot(rl.KeyR), run: func() Action {
			s.modalRenaming = core.MapIDFromPath(s.modalPaths[s.modalCursor])
			return ActionNone
		}},
		{label: "Delete", hot: keyHot(rl.KeyD), run: func() Action { s.modalConfirmDelete = true; return ActionNone }},
		{label: "Duplicate", hot: keyHot(rl.KeyC), run: func() Action { openDuplicateSelected(s); return ActionNone }},
	}
}

// openSelectedMap loads the cursored map. Shared by the Open button and Enter.
func openSelectedMap(s *State) Action {
	path := s.modalPaths[s.modalCursor]
	mf, err := mapfile.Load(path)
	if err != nil {
		s.flash("Open failed: " + err.Error())
		closeModal(s)
		return ActionNone
	}
	area, err := core.AreaFromMapFile(mf, path)
	if err != nil {
		s.flash("Open failed: " + err.Error())
		closeModal(s)
		return ActionNone
	}
	area = materializeEntranceCrystal(area)
	s.area = area
	s.baseline = core.CloneArea(area)
	s.undo = nil
	s.redo = nil
	s.dirty = false
	clearSelection(s) // different map — old selection coords no longer apply
	// Surface every level the map uses, and open on the start tile's floor (not
	// level 0, which is now a pit below the baseline).
	s.topLevel = maxAreaLevel(area)
	s.bottomLevel = minAreaLevel(area)
	s.editLevel = clampLevel(area.ElevationLevelAt(area.StartTileX, area.StartTileZ))
	s.levelHidden = [maxEditLevel + 1]bool{}
	// Area replaced wholesale — invalidate content-derived caches (like
	// performNewMap / undo / redo) or stale reachability/tooltip data lingers.
	s.reachValid = false
	s.contentEpoch++
	closeModal(s)
	s.flash("Opened " + core.MapIDFromPath(path))
	return ActionNone
}

// openDuplicateSelected copies the cursored map on disk and selects the copy.
func openDuplicateSelected(s *State) {
	newPath, err := duplicateMapFile(s.modalPaths[s.modalCursor])
	if err != nil {
		s.flash("Duplicate failed: " + err.Error())
		return
	}
	refreshOpenList(s)
	for i, p := range s.modalPaths {
		if p == newPath {
			s.modalCursor = i
			break
		}
	}
	// Defensive clamp if the copy isn't in the refreshed list.
	if s.modalCursor >= len(s.modalPaths) {
		s.modalCursor = len(s.modalPaths) - 1
	}
	if s.modalCursor < 0 {
		s.modalCursor = 0
	}
	s.flash("Duplicated to " + core.MapIDFromPath(newPath))
}

func updateOpenRename(s *State) Action {
	pumpPrintableASCII(&s.modalRenaming, defaultTextFieldMaxLen, acceptPrintable, nil)
	cmds := openRenameCmds(s)
	if act, ran := runCardCmds(openModalW, openModalH, false, cmds); ran {
		return act
	}
	return ActionNone
}

// openRenameCmds: 0=Rename (Enter), 1=Cancel (Esc).
func openRenameCmds(s *State) []modalCmd {
	return []modalCmd{
		{label: "Rename", hot: editorCommitPressed, run: func() Action { openRenameCommit(s); return ActionNone }},
		{label: "Cancel", hot: editorCancelPressed, run: func() Action { s.modalRenaming = ""; return ActionNone }},
	}
}

func openRenameCommit(s *State) {
	oldPath := s.modalPaths[s.modalCursor]
	newPath, err := renameMapFile(oldPath, s.modalRenaming)
	s.modalRenaming = ""
	if err != nil {
		s.flash("Rename failed: " + err.Error())
		return
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

func updateOpenConfirmDelete(s *State) Action {
	cmds := openDeleteConfirmCmds(s)
	if act, ran := runCardCmds(openModalW, openModalH, false, cmds); ran {
		return act
	}
	return ActionNone
}

// openDeleteConfirmCmds: 0=Delete (Y), 1=Cancel (Esc/N).
func openDeleteConfirmCmds(s *State) []modalCmd {
	return []modalCmd{
		{label: "Delete", hot: keyHot(rl.KeyY), run: func() Action { openDeleteSelected(s); return ActionNone }},
		{label: "Cancel", hot: cancelOr(rl.KeyN),
			run: func() Action { s.modalConfirmDelete = false; return ActionNone }},
	}
}

func openDeleteSelected(s *State) {
	path := s.modalPaths[s.modalCursor]
	if err := os.Remove(path); err != nil {
		s.flash("Delete failed: " + err.Error())
		s.modalConfirmDelete = false
		return
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

func refreshOpenList(s *State) {
	paths, _ := mapfile.ListByModTime(core.MapsDir())
	s.modalPaths = paths
}

func updateSaveAsModal(s *State) Action {
	if s.awaitingOverwrite {
		cmds := saveAsOverwriteCmds(s)
		if act, ran := runCardCmds(saveAsModalW, saveAsModalH, true, cmds); ran {
			return act
		}
		return ActionNone
	}

	if editorCancelPressed() {
		closeModal(s)
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

// saveAsOverwriteCmds: 0=Overwrite (Y), 1=Cancel / pick a different name
// (N / Esc). Only used while s.awaitingOverwrite.
func saveAsOverwriteCmds(s *State) []modalCmd {
	return []modalCmd{
		{label: "Overwrite", hot: keyHot(rl.KeyY), run: func() Action {
			s.awaitingOverwrite = false
			confirmModalForce(s)
			if s.modal == modalNone {
				return runPendingAction(s)
			}
			return ActionNone
		}},
		{label: "Cancel", hot: cancelOr(rl.KeyN),
			run: func() Action {
				s.awaitingOverwrite = false
				s.focus = focusFilename
				return ActionNone
			}},
	}
}

// updateEscMenuModal handles the editor's pause-style menu (Display / Continue /
// Exit to Title). Exit routes through modalConfirmDirty when there are unsaved edits.
func updateEscMenuModal(s *State) Action {
	cmds := escMenuCmds(s)
	if act, ran := runCardCmdsNav(s, escMenuModalW, escMenuModalH, true, cmds); ran {
		return act
	}
	return ActionNone
}

// escMenuCmds: 0=Display (D), 1=Continue (Esc/C), 2=Exit to Title (E).
func escMenuCmds(s *State) []modalCmd {
	return []modalCmd{
		{label: render.DisplayMenuRowLabel(), hot: keyHot(rl.KeyD),
			run: func() Action { render.ToggleDisplayMode(); return ActionNone }},
		{label: "Continue editing", hot: cancelOr(rl.KeyC),
			run: func() Action { closeModal(s); return ActionNone }},
		{label: "Exit to Title", hot: keyHot(rl.KeyE), run: func() Action {
			closeModal(s)
			if s.dirty {
				openConfirmDirtyModal(s, pendingExitToTitle)
				return ActionNone
			}
			return ActionExitToTitle
		}},
	}
}

func updateConfirmDirtyModal(s *State) Action {
	cmds := confirmDirtyCmds(s)
	if act, ran := runCardCmds(confirmDirtyModalW, confirmDirtyModalH, true, cmds); ran {
		return act
	}
	return ActionNone
}

// confirmDirtyCmds: 0=Save (S), 1=Discard (D), 2=Cancel (Esc/C).
func confirmDirtyCmds(s *State) []modalCmd {
	return []modalCmd{
		{label: "Save", hot: keyHot(rl.KeyS), run: func() Action { return confirmDirtySave(s) }},
		{label: "Discard", hot: keyHot(rl.KeyD), run: func() Action { closeModal(s); return runPendingAction(s) }},
		{label: "Cancel", hot: cancelOr(rl.KeyC), run: func() Action {
			closeModal(s)
			s.pending = pendingNone
			return ActionNone
		}},
	}
}

// confirmDirtySave persists the map (or opens Save As when it has no path), then
// runs the pending action.
func confirmDirtySave(s *State) Action {
	if s.area.Path == "" {
		openSaveAsModal(s)
		return ActionNone
	}
	if err := writeAreaTo(s, s.area.Path); err != nil {
		s.flash("Save failed: " + err.Error())
		closeModal(s)
		s.pending = pendingNone
		return ActionNone
	}
	closeModal(s)
	return runPendingAction(s)
}

// keyHot / cancelOr build modalCmd accelerator predicates. cancelOr is the "back"
// edge (Esc / pad B) plus one extra letter key (C for Continue, N for No).
func keyHot(k int32) func() bool { return func() bool { return rl.IsKeyPressed(k) } }
func cancelOr(k int32) func() bool {
	return func() bool { return editorCancelPressed() || rl.IsKeyPressed(k) }
}

func runPendingAction(s *State) Action {
	p := s.pending
	s.pending = pendingNone
	switch p {
	case pendingExitToTitle:
		return ActionExitToTitle
	case pendingNew:
		openNewMapModal(s)
	case pendingOpen:
		openModal(s, modalOpen)
	}
	return ActionNone
}

func confirmModal(s *State) {
	if s.modal != modalSaveAs {
		return
	}
	// Sanitize at commit so the disk filename is known-good (the field preview
	// already shows this form).
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
	if err := writeAreaTo(s, path); err != nil {
		s.flash("Save failed: " + err.Error())
		return
	}
	closeModal(s)
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

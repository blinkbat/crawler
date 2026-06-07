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

// editorCommitPressed / editorCancelPressed / editorTabPressed name the
// editor-side commit / cancel / focus-cycle edges used by every modal
// updater. They delegate to the input package (input.Editor* predicates)
// so the bindings live in the one remappable place per AGENTS.md, while
// the editor diverges from explore's `input.ConfirmPressed` (Z / Space /
// Enter / pad A) on purpose: modal text fields use Tab to cycle focus and
// Enter (alone) to commit, so the Z / Space chord would collide with
// typing into a Name field. The pad A / B face buttons commit / cancel.
func editorCommitPressed() bool {
	return input.EditorConfirmPressed()
}

func editorCancelPressed() bool {
	return input.EditorCancelPressed()
}

func editorTabPressed() bool {
	return input.EditorTabPressed()
}

// updateHotkeys handles keyboard shortcuts when no text field is focused.
func updateHotkeys(s *State) {
	ctrl := rl.IsKeyDown(rl.KeyLeftControl) || rl.IsKeyDown(rl.KeyRightControl)
	shift := rl.IsKeyDown(rl.KeyLeftShift) || rl.IsKeyDown(rl.KeyRightShift)
	alt := rl.IsKeyDown(rl.KeyLeftAlt) || rl.IsKeyDown(rl.KeyRightAlt)

	// ALT tap-toggles the per-tile glyph overlay (off by default; when on
	// it shows the ACTIVE layer's chars only). The detection is "key
	// released without a chord" so it never fights Alt+1..6 (layer jump)
	// or any future Alt+key binding: if ANY key is pressed during the Alt
	// hold we mark altChordUsed and the release fires no toggle. Edge
	// press of Alt resets the flag.
	altPressed := rl.IsKeyPressed(rl.KeyLeftAlt) || rl.IsKeyPressed(rl.KeyRightAlt)
	altReleased := rl.IsKeyReleased(rl.KeyLeftAlt) || rl.IsKeyReleased(rl.KeyRightAlt)
	if altPressed {
		s.altChordUsed = false
	}
	if alt && rl.GetKeyPressed() != 0 {
		// GetKeyPressed pops queued key-press events; non-zero means a
		// key was pressed THIS frame while Alt was held, so the user
		// intended a chord, not a tap-toggle. Drain the queue so a
		// later updater isn't surprised by an empty buffer (no other
		// caller in the editor reads from this queue today).
		s.altChordUsed = true
		for rl.GetKeyPressed() != 0 {
		}
	}
	if altReleased && !s.altChordUsed {
		s.showTileGlyphs = !s.showTileGlyphs
	}

	// Alt+1..6 jumps directly to a layer — saves Tab-cycling when the
	// author knows which layer they want. Number row only; the keypad
	// equivalents aren't bound to keep the binding compact.
	if alt {
		for i := 0; i < layerCount; i++ {
			if rl.IsKeyPressed(numberRowKeys[i]) {
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
		for i, k := range numberRowKeys {
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
	if !ctrl && editorTabPressed() {
		dir := 1
		if shift {
			dir = -1
		}
		s.layer = core.WrapEnum(s.layer, dir, layerCount)
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

	// Height selector (the elevation level the Set Height brush stamps + the
	// slice-view focus). PgUp/PgDn are the keyboard accelerators for the
	// toolbar's Lvl -/+ buttons.
	if rl.IsKeyPressed(rl.KeyPageUp) {
		s.editLevel = clampLevel(s.editLevel + 1)
	}
	if rl.IsKeyPressed(rl.KeyPageDown) {
		s.editLevel = clampLevel(s.editLevel - 1)
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
		s.previewPhase = core.WrapEnum(s.previewPhase, 1, core.TimeOfDayCount)
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
	// Arrow keys / D-pad / left stick walk the grid cursor (input.Arrow*
	// includes the pad + stick edges, so the canvas navigates with a
	// controller too). Clamp-not-wrap: the cursor stops at the map edge.
	if input.ArrowLeftPressed() {
		s.gridCursorX, s.gridCursorZ = activateCursor(s, mw, mh)
		s.gridCursorX = core.Clamp(s.gridCursorX-1, 0, mw-1)
		moved = true
	}
	if input.ArrowRightPressed() {
		s.gridCursorX, s.gridCursorZ = activateCursor(s, mw, mh)
		s.gridCursorX = core.Clamp(s.gridCursorX+1, 0, mw-1)
		moved = true
	}
	if input.ArrowUpPressed() {
		s.gridCursorX, s.gridCursorZ = activateCursor(s, mw, mh)
		s.gridCursorZ = core.Clamp(s.gridCursorZ-1, 0, mh-1)
		moved = true
	}
	if input.ArrowDownPressed() {
		s.gridCursorX, s.gridCursorZ = activateCursor(s, mw, mh)
		s.gridCursorZ = core.Clamp(s.gridCursorZ+1, 0, mh-1)
		moved = true
	}
	if moved && s.gridCursorX >= 0 {
		s.hoverX, s.hoverZ = s.gridCursorX, s.gridCursorZ
	}
	if s.gridCursorX < 0 {
		return
	}
	if input.EditorPaintPressed() {
		pushUndo(s)
		applyToolBrushed(s, s.gridCursorX, s.gridCursorZ)
	}
	if input.EditorErasePressed() {
		pushUndo(s)
		eraseAt(s, s.gridCursorX, s.gridCursorZ)
	}
}

func activateCursor(s *State, mw, mh int) (int, int) {
	if s.gridCursorX >= 0 {
		return s.gridCursorX, s.gridCursorZ
	}
	x := core.Clamp(s.area.StartTileX, 0, mw-1)
	z := core.Clamp(s.area.StartTileZ, 0, mh-1)
	return x, z
}

// updateMouse processes top-bar / palette / metadata clicks and grid
// painting. Called every frame outside of modals and text-focus mode.
func updateMouse(s *State) {
	mp := rl.GetMousePosition()

	hx, hz := s.cellAt(mp)
	s.hoverX, s.hoverZ = hx, hz

	// Context menu absorbs all mouse / keyboard input while open so a
	// stray click on the grid behind the menu doesn't double-act.
	if updateContextMenu(s) {
		return
	}

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
	} else if pointIn(mp, s.rect.metadata) {
		// Wheel over the right-hand MAP panel scrolls its content.
		// One notch moves ~one row of fields so a short window can
		// still reach the reachability badge at the bottom.
		w := rl.GetMouseWheelMove()
		if w != 0 {
			ScrollMetadata(s, -w*42)
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
		if hit := topbarButtonAt(s, mp); hit >= 0 {
			topbarBtns[hit].action(s)
			return
		}
		if hit := toolbarButtonAt(s, mp); hit >= 0 {
			toolbarBtns[hit].action(s)
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
			// Ramp tool-mode: right-click clears a ramp at the tile (its floor
			// arrow → auto floor), leaving the elevation digit so the cliff
			// stays. No-op (no undo snapshot) on a non-ramp tile.
			if s.rampMode {
				if _, ok := s.area.RampAt(hx, hz); ok {
					pushUndo(s)
					setLayerCell(&s.area.Floor, hx, hz, core.FloorAuto)
					s.dirty = true
				}
				return
			}
			// On the Entities layer, right-click opens the context menu
			// over the tile so the author can Edit / Delete / move-start
			// without having to switch brushes. Empty entity cells fall
			// through to the legacy erase (no-op on empties) so right-
			// click stays a recoverable action either way.
			if s.layer == LayerEntities {
				if items := contextItemsAt(s, hx, hz); len(items) > 0 {
					openContextMenu(s, mp.X, mp.Y, hx, hz)
					return
				}
			}
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
	// Ramp tool-mode: a left-click drops a connective ramp via the smart
	// tool (derives direction + low level from the neighbors), short-
	// circuiting normal painting. placeRamp snapshots undo on success.
	if s.rampMode {
		placeRamp(s, x, z)
		s.drag = dragNone
		return
	}
	gridLayer := isGridLayer(s.layer)

	if gridLayer && ctrl {
		// floodFill snapshots undo itself (only when the fill actually
		// changes cells), so a no-op Ctrl+click leaves the undo stack alone.
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
				openChestEditModal(s, idx)
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
				} else if core.DoorSpawnIndexAt(s.area.DoorSpawns, s.hoverX, s.hoverZ) >= 0 {
					// Mirror placeDoorAt's door/pack exclusion on the drag path
					// too — a pack sharing a door tile races the transition.
					s.flash("Cell holds a door — clear it first")
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
				openPackEditModal(s, s.dragPackIdx)
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

// openSaveAsModal pops the Save As dialog seeded with the current
// map's file stem (or empty for an unsaved area). Extracted so the
// topbar table-driven dispatch can point at a function pointer
// instead of inlining the three-line set up — single seam for any
// future "Save As" entry points (Ctrl+Shift+S, command palette, ...).
func openSaveAsModal(s *State) {
	// Default the filename to the saved map's stem; for an as-yet-unsaved
	// area (no path) fall back to the area's title so every Save As entry
	// point — topbar button, Ctrl+S on an unnamed map, the confirm-dirty
	// "Save" branch — pre-fills the same sensible name instead of one path
	// showing the title and another showing an empty field.
	stem := mapStem(s.area.Path)
	if stem == "" {
		stem = sanitizeFilename(s.area.Name)
	}
	s.modalFilename = stem
	s.modal = modalSaveAs
	s.focus = focusFilename
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

// textFieldConfig declares the rune-budget and accept-filter for a
// single focusable text field. The Editor used to call
// pumpPrintableASCII directly with bespoke (maxLen, accept, onChange)
// triples at every site, which made "is this field's config the
// canonical one or a typo?" a 5-file grep. This table is the single
// source of truth — adding a new focusField is one row here plus a
// case in activeTextTarget.
type textFieldConfig struct {
	MaxLen int
	Accept func(rune) bool
}

// defaultTextFieldMaxLen is the rune budget shared by the general-purpose
// editor text fields (names, filenames, door target paths). One named source
// so the limit tunes in a single place instead of being repeated as a literal
// per field + in the defensive default below. Fields that need a different cap
// (e.g. focusCustomEnemyName) override it explicitly.
const defaultTextFieldMaxLen = 96

// textFieldConfigs maps each focusField to its rune-budget +
// acceptance rule. Foci NOT in this table (focusNone, focusWidth /
// Height — those are numeric inputs handled by updateNumericInput,
// not pumpPrintableASCII) reuse the default below via
// configForFocus.
var textFieldConfigs = map[focusField]textFieldConfig{
	focusName:            {defaultTextFieldMaxLen, acceptPrintable},
	focusQuiet:           {defaultTextFieldMaxLen, acceptPrintable},
	focusFilename:        {defaultTextFieldMaxLen, acceptPrintable},
	// Door identifier fields reject spaces: the .map door row is
	// space-delimited, so a space here would corrupt the round-trip
	// (validate() also backstops this at save time).
	focusDoorName:        {defaultTextFieldMaxLen, acceptPrintableNoSpace},
	focusDoorTargetMap:   {defaultTextFieldMaxLen, acceptPrintableNoSpace},
	focusDoorTargetDoor:  {defaultTextFieldMaxLen, acceptPrintableNoSpace},
	focusCustomEnemyName: {24, acceptPrintable},
}

func configForFocus(f focusField) textFieldConfig {
	if cfg, ok := textFieldConfigs[f]; ok {
		return cfg
	}
	// Defensive default: a permissive field at the shared cap. Used by future
	// text foci that get wired up before someone remembers to add a
	// row; pump still bounds the buffer so the failure mode is bounded.
	return textFieldConfig{MaxLen: defaultTextFieldMaxLen, Accept: acceptPrintable}
}

// pumpFocusField pumps printable runes into `target` using the
// config registered for the current s.focus. Replaces the bespoke
// pumpPrintableASCII calls that used to pick (maxLen, accept) per
// site — callers say "this is the active text target, route input
// at the rate the table says."
func pumpFocusField(s *State, target *string) {
	cfg := configForFocus(s.focus)
	pumpPrintableASCII(target, cfg.MaxLen, cfg.Accept, s.markDirty)
}

// pumpPrintableASCII drains queued printable-ASCII characters into
// target (capped at maxLen) and consumes one backspace press. The
// accept predicate filters which runes land in the buffer — callers
// pass nil for "any printable ASCII" or a custom filter for "no
// space" (sound-name input), "digits only" (numeric resize), etc. so
// one pump function backs every text-field flavor in the editor.
// onChange fires once per accepted character or backspace and may be
// nil when no caller-side effect is needed.
//
// Prefer pumpFocusField for s.focus-keyed inputs — that path looks
// up the per-field rate from textFieldConfigs so the config table
// stays the single source of truth. Direct callers (the sound-name
// modal, the open-modal rename buffer) carry their own configs
// because their target isn't focus-keyed.
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

// acceptDigit accepts ASCII digits only — the filter for the numeric
// resize fields so they share pumpPrintableASCII instead of a parallel
// hand-rolled char-drain loop.
func acceptDigit(r rune) bool { return r >= '0' && r <= '9' }

// numericFieldMaxLen caps the resize numeric buffer — map dimensions are
// at most four digits. Named so the cap isn't a bare literal in the pump.
const numericFieldMaxLen = 4

func updateTextInput(s *State) {
	if s.focus == focusWidth || s.focus == focusHeight ||
		s.focus == focusNewWidth || s.focus == focusNewHeight {
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
	// Buffer is digit-only (acceptDigit) and capped at numericFieldMaxLen,
	// so Atoi can't realistically fail; bail cleanly if it ever does.
	v, err := strconv.Atoi(s.numericBuf)
	if err != nil {
		s.numericBuf = ""
		return
	}
	// Floor at MinMapDimension (not 1) so the metadata field can't
	// produce a 2-wide map that `resize` would then re-clamp anyway.
	// One clamp helper used everywhere — see core.ClampMapDimension.
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
		// Stay within the new-map dialog — its only other text field is
		// the height. modalNew has no Name / Quiet fields to cycle to.
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
	// validateModalState runs every frame BEFORE the modal's own
	// updater. If the entity referenced by the modal (pack / chest /
	// door / custom enemy) has been deleted from the underlying
	// slice — for example by an ops path elsewhere, or by an undo
	// that reverted past the modal's open frame — close the modal
	// and clear any cursor so the next frame doesn't dereference a
	// stale index. Same defense the modal draw paths used to need
	// inline; centralizing it here means the draw/update pair can
	// trust their indices.
	validateModalState(s)
	if h, ok := modalHandlers[s.modal]; ok && h.update != nil {
		return h.update(s)
	}
	return ActionNone
}

// validateModalState closes the active modal when its referenced
// entity has gone out of bounds. Single source of truth for the
// "is this modal still pointing at something real?" rule — added
// rows live alongside each modal's open path so a future modal
// that holds an index plugs into the same check.
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
	case modalCustomEnemies:
		// Clamp the custom-enemy cursor to the slice instead of
		// closing — the modal lists every def and the cursor is
		// internal navigation, not a hard reference to a single
		// row that has to exist.
		if n := len(s.area.CustomEnemies); n == 0 {
			s.modalCustomIdx = 0
		} else if s.modalCustomIdx < 0 {
			s.modalCustomIdx = 0
		} else if s.modalCustomIdx >= n {
			s.modalCustomIdx = n - 1
		}
	}
}

// modalUpdaters used to be a sibling of modalDrawers — both kept the
// same modalKind keyed entries in lockstep across two files. They're
// now folded into modalHandlers (draw.go) so adding a modal is one
// row in one place.

// closeModal is the single seam every modal updater goes through to
// dismiss its dialog. Clears the modal kind plus every modal-scoped
// cursor / index field so a future modal can't read a stale value from
// the prior one. Replaces ~18 hand-typed `s.modal = modalNone; s.modalXxxIdx
// = -1` snippets that drifted per modal — the chest-edit updater was
// missing a modalCursor reset under the previous shape.
func closeModal(s *State) {
	// The Foe Visualizer caches an off-screen RenderTexture2D. Free it from
	// this central seam (not only the modal's own Close/cancel buttons) so
	// any path that dismisses a modal via closeModal can't leak the GPU
	// handle across reopen. Idempotent when nothing is allocated.
	if s.modal == modalFoeView {
		render.CloseFoePreview()
	}
	s.modal = modalNone
	s.modalCursor = 0
	s.modalPackIdx = -1
	s.modalChestIdx = -1
	s.modalDoorIdx = -1
	s.modalCustomIdx = -1
	s.modalValidateRows = nil
	s.modalConfirmDelete = false
	s.modalRenaming = ""
	s.soundDeleteArmed = ""
	soundDrag.sliderIdx = -1
	// Door-edit text focus survives outside the modal in pumpPrintableASCII's
	// flow, so explicitly drop it here too. The new-map dialog's numeric
	// foci and the custom-enemy name field are similarly modal-scoped —
	// they must not carry over.
	if s.focus == focusDoorName || s.focus == focusDoorTargetMap || s.focus == focusDoorTargetDoor ||
		s.focus == focusNewWidth || s.focus == focusNewHeight ||
		s.focus == focusCustomEnemyName {
		s.focus = focusNone
		s.numericBuf = ""
	}
}

// openPackEditModal opens the per-pack editor for spawn index idx.
// Mirrors the chest / door path so every entry point (context menu,
// click-without-drag) shares the same modal + index + cursor setup.
func openPackEditModal(s *State, idx int) {
	if idx < 0 || idx >= len(s.area.PackSpawns) {
		return
	}
	s.modal = modalPackEdit
	s.modalPackIdx = idx
	s.modalCursor = 0
}

// openChestEditModal opens the per-chest editor for spawn index idx.
// Mirrors the pack / door path.
func openChestEditModal(s *State, idx int) {
	if idx < 0 || idx >= len(s.area.ChestSpawns) {
		return
	}
	s.modal = modalChestEdit
	s.modalChestIdx = idx
	s.modalCursor = 0
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
			// Click outside the card is a no-op (NOT a close): the door
			// modal — like the pack / chest modals — is dismissed only via
			// Esc, Enter, or the Done button, so a stray click can't lose
			// in-progress field edits.
		}
	}

	// Keyboard: while a text field is focused, route every keystroke into
	// its buffer via pumpPrintableASCII. Tab cycles to the next field;
	// Enter confirms current field; Esc closes.
	switch s.focus {
	case focusDoorName, focusDoorTargetMap, focusDoorTargetDoor:
		target := doorEditTextTarget(s)
		if target != nil {
			// Route through pumpFocusField so the door fields read
			// their rune-budget from textFieldConfigs (defaultTextFieldMaxLen) —
			// keeps the door modal in sync with the editor-chrome
			// fields without a second copy of the cap. The pump's
			// onChange is s.markDirty, which sets s.dirty=true
			// outside the filename-focus exception, so no second
			// dirty guard is needed here.
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

	// No text field focused — keyboard shortcuts for facing + delete.
	if editorCancelPressed() || editorCommitPressed() {
		closeModal(s)
		return ActionNone
	}
	if editorTabPressed() {
		s.focus = focusDoorName
		return ActionNone
	}
	if rl.IsKeyPressed(rl.KeyX) {
		deleteDoorAt(s, s.modalDoorIdx)
		return ActionNone
	}
	// N / E / S / W set facing directly. Updates only run while the
	// modal is open and updateHotkeys's global Ctrl+S Save handler
	// doesn't fire during modals, so the 'S' binding is free here.
	for _, fk := range doorFacingKeys {
		if rl.IsKeyPressed(fk.key) {
			pushUndo(s)
			door.Facing = fk.facing
			s.dirty = true
			return ActionNone
		}
	}
	// 1 / 2 / 3 set the door style (building / cave / field). Number-row
	// keys don't collide with the facing letters or the save shortcut.
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

// doorFacingKeys / doorStyleKeys are the door-edit modal's direct-set
// hotkey tables (N/E/S/W → facing, 1/2/3 → style). Package-level so the
// per-frame modal updater doesn't rebuild the slices every call.
var doorFacingKeys = []struct {
	key    int32
	facing int
}{
	{rl.KeyN, core.North},
	{rl.KeyE, core.East},
	{rl.KeyS, core.South},
	{rl.KeyW, core.West},
}

var doorStyleKeys = []struct {
	key   int32
	style core.DoorStyle
}{
	{rl.KeyOne, core.DoorStyleBuilding},
	{rl.KeyTwo, core.DoorStyleCave},
	{rl.KeyThree, core.DoorStyleField},
}

// init guards the door-modal hotkey tables against drift with the
// core enums they bind. If a new DoorStyle or cardinal facing lands
// without a row here, startup panics — mirrors the lockstep guard
// modalHandlers and entityBrushColors already use.
func init() {
	if len(doorFacingKeys) != core.FacingCount {
		panic("editor: doorFacingKeys length must match core.FacingCount — add a row when extending the facing enum")
	}
	if len(doorStyleKeys) != int(core.DoorStyleCount) {
		panic("editor: doorStyleKeys length must match core.DoorStyleCount — add a row when extending DoorStyle")
	}
}

// deleteDoorAt removes the door spawn at idx (pushing undo, marking
// dirty, and closing the modal). Shared by the door modal's click-Delete
// button and its X-key shortcut, which open-coded the same append-splice.
func deleteDoorAt(s *State, idx int) {
	if idx < 0 || idx >= len(s.area.DoorSpawns) {
		return
	}
	pushUndo(s)
	s.area.DoorSpawns = removeModalListItem(s.area.DoorSpawns, idx)
	s.dirty = true
	closeModal(s)
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
	if editorCancelPressed() || editorCommitPressed() || rl.IsKeyPressed(rl.KeySpace) {
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
	if !updateEntityListCursor(s, memberCount) {
		return ActionNone
	}

	// Mouse: click a member row to select, or a button to add / remove /
	// reorder / cycle AI. Buttons + their actions come from the same
	// packEditCmds the draw renders, so click index == button.
	if handleEntityModalClick(s, memberCount, packEditCmds) {
		return ActionNone
	}

	// Keyboard accelerators (mirror the buttons). X remove, K/J reorder,
	// the packAddRules letters add a kind, C adds the selected custom
	// enemy, A cycles the movement-AI mode.
	if memberCount > 0 {
		if rl.IsKeyPressed(rl.KeyX) {
			packRemoveSelected(s, pack)
			return ActionNone
		}
		if rl.IsKeyPressed(rl.KeyK) {
			packMoveSelected(s, pack, -1)
		}
		if rl.IsKeyPressed(rl.KeyJ) {
			packMoveSelected(s, pack, +1)
		}
	}
	for _, rule := range packAddRules {
		if rl.IsKeyPressed(rule.Key) {
			pushUndo(s)
			core.AppendBuiltinPackMember(pack, rule.Kind)
			s.modalCursor = len(pack.Members) - 1
			s.dirty = true
			return ActionNone
		}
	}
	if rl.IsKeyPressed(rl.KeyC) {
		if def, ok := selectedCustomEnemyForPack(s); ok {
			pushUndo(s)
			core.AppendCustomPackMember(pack, def)
			s.modalCursor = len(pack.Members) - 1
			s.dirty = true
		} else {
			s.flash("No custom enemies defined")
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

// handleEntityModalClick processes a left-click in a pack/chest edit
// modal: select the clicked list row, or run the clicked add/action
// button's command. `builder` supplies the modal's (adds, actions) cmd
// lists (packEditCmds / chestEditCmds) — the same builder the draw uses,
// so click index == button. Returns true when the click was consumed (the
// caller should return). Shared so the two editors can't drift on the
// row-then-actions-then-adds hit order.
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

// packEditCmds builds the pack-edit modal's add buttons (one per
// packAddRules kind + the selected custom enemy) and action buttons
// (Remove / Up / Down / AI). Called by both the draw (for labels) and the
// click handler (runs the clicked .run), so buttons and actions can't
// drift. Caller must have validated s.modalPackIdx.
func packEditCmds(s *State) (adds, actions []modalCmd) {
	pack := &s.area.PackSpawns[s.modalPackIdx]
	for _, rule := range packAddRules {
		rule := rule
		adds = append(adds, modalCmd{
			label: "+ " + core.EnemyInfo(rule.Kind).SingularName,
			run: func() Action {
				pushUndo(s)
				core.AppendBuiltinPackMember(pack, rule.Kind)
				s.modalCursor = len(pack.Members) - 1
				s.dirty = true
				return ActionNone
			},
		})
	}
	if def, ok := selectedCustomEnemyForPack(s); ok {
		def := def
		adds = append(adds, modalCmd{
			label: "+ " + def.Name,
			run: func() Action {
				pushUndo(s)
				core.AppendCustomPackMember(pack, def)
				s.modalCursor = len(pack.Members) - 1
				s.dirty = true
				return ActionNone
			},
		})
	}
	actions = []modalCmd{
		{label: "Remove", run: func() Action { packRemoveSelected(s, pack); return ActionNone }},
		{label: "Up", run: func() Action { packMoveSelected(s, pack, -1); return ActionNone }},
		{label: "Down", run: func() Action { packMoveSelected(s, pack, +1); return ActionNone }},
		{label: "AI: " + core.PackAILabel(pack.AI), run: func() Action {
			pushUndo(s)
			pack.AI = core.WrapEnum(pack.AI, 1, core.PackAICount)
			s.dirty = true
			s.flash("Pack AI: " + core.PackAILabel(pack.AI))
			return ActionNone
		}},
	}
	return adds, actions
}

// packRemoveSelected removes the cursored member, dropping the whole pack
// (and closing the modal) if it empties. Shared by the Remove button and
// the X accelerator.
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

// packMoveSelected swaps the cursored member with its neighbor in dir
// (-1 up / +1 down), no-op at the ends. Shared by the Up/Down buttons and
// the K/J accelerators.
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

func updateEntityListCursor(s *State, count int) bool {
	if s.modalCursor >= count {
		s.modalCursor = count - 1
	}
	if s.modalCursor < 0 {
		s.modalCursor = 0
	}
	if input.ModalClosePressed() {
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
	return append(items[:idx], items[idx+1:]...)
}

func selectedCustomEnemyForPack(s *State) (core.CustomEnemyDef, bool) {
	if len(s.area.CustomEnemies) == 0 {
		return core.CustomEnemyDef{}, false
	}
	idx := s.modalCustomIdx
	if idx < 0 || idx >= len(s.area.CustomEnemies) {
		idx = 0
	}
	return s.area.CustomEnemies[idx], true
}

// packAddRule binds a keyboard shortcut to the EnemyKind it adds in the
// pack-edit modal. The slice is built at init from core.EnemyKinds() +
// a positional hotkey pool — adding a new enemy is one row in core's
// enemyDefinitions and the modal picks up the hotkey automatically.
type packAddRule struct {
	Key   int32
	Kind  core.EnemyKind
	Label string // appears in the modal's hint row, e.g. "R add Rat"
}

// packAddHotkeys binds each enemy kind to the key that appends it in the
// pack-edit modal. Keyed by EnemyKind (NOT slice position) so reordering
// enemyDefinitions can't silently reshuffle the bindings — the old
// positional pool drifted when EnemyAmoeba landed last in the slice
// (≠ its enum position), putting add-keys on top of the modal's own
// member-op controls. Every key must avoid packModalReservedKeys; the
// init assert below fails closed on a collision, a duplicate, or an
// unbound kind. Most letters are first-letter mnemonics; the three marked
// below moved off reserved control keys.
var packAddHotkeys = map[core.EnemyKind]int32{
	core.EnemyRat:          rl.KeyR,
	core.EnemyBat:          rl.KeyB,
	core.EnemyDiseasedRat:  rl.KeyD,
	core.EnemyGoblin:       rl.KeyG,
	core.EnemyGoblinMage:   rl.KeyM,
	core.EnemyVenusMantrap: rl.KeyN, // N — maNtrap
	core.EnemyCaveSpider:   rl.KeyV,
	core.EnemyVampireBat:   rl.KeyZ,
	core.EnemyWisp:         rl.KeyP,
	core.EnemyStoneGolem:   rl.KeyF,
	core.EnemyNecromancer:  rl.KeyE, // E — nEcromancer (was W: collided with cursor-up)
	core.EnemySkeleton:     rl.KeyL, // L — skeLeton (was K: collided with member move-up)
	core.EnemyAmoeba:       rl.KeyO, // O — ooze/blob (was X: collided with member remove)
}

// packModalReservedKeys are the pack-edit modal's non-add controls: the
// entity-list cursor nav (Up/Down, which input.CursorUpDown also reads as
// W/S), member ops (X remove, K up, J down), custom-enemy add (C), pack-AI
// cycle (A), and close (Esc/Enter). An enemy add-key landing on any of
// these would fire two actions on one keypress — the bug the init assert
// guards against.
var packModalReservedKeys = map[int32]bool{
	rl.KeyUp: true, rl.KeyDown: true, rl.KeyW: true, rl.KeyS: true,
	rl.KeyX: true, rl.KeyK: true, rl.KeyJ: true,
	rl.KeyC: true, rl.KeyA: true,
	rl.KeyEscape: true, rl.KeyEnter: true, rl.KeyKpEnter: true,
}

// init asserts the add-rule hotkey wiring. The pack-edit and chest-edit
// modals are keyboard-only for "add a kind" — a missing binding (Key=0)
// would silently be unauthorable, and an add-key that collides with a
// reserved modal control fires two actions at once. Failing closed at
// startup is cheaper than shipping a broken editor.
func init() {
	seen := map[int32]bool{}
	for _, def := range core.EnemyKinds() {
		key, ok := packAddHotkeys[def.Kind]
		if !ok || key == 0 {
			panic("editor: packAddHotkeys missing a key for enemy kind " + def.SingularName)
		}
		if packModalReservedKeys[key] {
			panic("editor: packAddHotkeys key for " + def.SingularName + " collides with a reserved pack-modal control")
		}
		if seen[key] {
			panic("editor: packAddHotkeys key for " + def.SingularName + " duplicates another enemy's add-key")
		}
		seen[key] = true
	}
	if got, max := len(core.AllItems()), len(chestAddHotkeys); got > max {
		panic("editor: chestAddHotkeys pool too small (" + strconv.Itoa(got) + " items, " + strconv.Itoa(max) + " keys)")
	}
}

var packAddRules = buildPackAddRules()

// buildPackAddRules walks core.EnemyKinds() (slice order — drives the hint
// row's display order) and looks each kind's key up in packAddHotkeys, so
// the binding is by-kind even though the rules render in registry order.
func buildPackAddRules() []packAddRule {
	defs := core.EnemyKinds()
	out := make([]packAddRule, 0, len(defs))
	for _, def := range defs {
		key := packAddHotkeys[def.Kind]
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
// Index i is the hotkey for the i-th item in core.AllItems registry
// order. Mnemonic letters where they don't collide with existing
// editor bindings: C=Cheese, J=Jerky, F=bread (Fare — B/R/E/A/D are all
// taken), S=Sword, H=sHield, L=Leather, R=Ring, M=aMulet, then the
// sample weapons D=Dagger, E=rapiEr, B=Bow, G=slinG, A=Axe, W=War
// hammer. The order must track the itemDefinitions slice — bread sits
// after jerky there, so its key sits at the matching index here.
// Extend as items are added; the init check at the bottom of this file
// panics if the pool is shorter than the registry so that's caught at
// startup, not deep in the editor.
var chestAddHotkeys = []int32{
	rl.KeyC, rl.KeyJ, rl.KeyF, rl.KeyS, rl.KeyH, rl.KeyL, rl.KeyR, rl.KeyM,
	rl.KeyD, rl.KeyE, rl.KeyB, rl.KeyG, rl.KeyA, rl.KeyW,
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
	if !updateEntityListCursor(s, itemCount) {
		return ActionNone
	}

	// Mouse: click an item row to select, or a button to add / remove.
	if handleEntityModalClick(s, itemCount, chestEditCmds) {
		return ActionNone
	}

	// Keyboard accelerators (mirror the buttons): X removes, the
	// chestAddRules letters add an item.
	if itemCount > 0 && rl.IsKeyPressed(rl.KeyX) {
		chestRemoveSelected(s, chest)
		return ActionNone
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

// chestEditCmds builds the chest-edit modal's add buttons (one per
// chestAddRules item) and the Remove action button. Shared by draw and
// the click handler. Caller must have validated s.modalChestIdx.
func chestEditCmds(s *State) (adds, actions []modalCmd) {
	chest := &s.area.ChestSpawns[s.modalChestIdx]
	for _, rule := range chestAddRules {
		rule := rule
		adds = append(adds, modalCmd{
			label: "+ " + shortItemLabel(core.ItemInfo(rule.Kind).Name),
			run: func() Action {
				pushUndo(s)
				chest.Items = append(chest.Items, rule.Kind)
				s.modalCursor = len(chest.Items) - 1
				s.dirty = true
				return ActionNone
			},
		})
	}
	actions = []modalCmd{
		{label: "Remove", run: func() Action { chestRemoveSelected(s, chest); return ActionNone }},
	}
	return adds, actions
}

// chestRemoveSelected removes the cursored item (an empty chest is a valid
// authored shape, so it stays). Shared by the Remove button and X.
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

	// Mouse: click a list row to select it (action buttons handled below).
	if rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		if idx := openModalRowAt(s, rl.GetMousePosition()); idx >= 0 {
			s.modalCursor = idx
			return ActionNone
		}
	}
	// Action buttons + their keyboard accelerators.
	cmds := openModalActionCmds(s)
	rects := modalButtonRow(centeredCardRect(openModalW, openModalH), cmdLabels(cmds))
	if act, ran := runModalCmds(cmds, rects); ran {
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

// openSelectedMap loads the cursored map into the editor. Shared by the
// Open button and the Enter accelerator.
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
	s.area = area
	s.baseline = core.CloneArea(area)
	s.undo = nil
	s.redo = nil
	s.dirty = false
	closeModal(s)
	s.flash("Opened " + core.MapIDFromPath(path))
	return ActionNone
}

// openDuplicateSelected copies the cursored map on disk and selects the
// copy. Shared by the Duplicate button and the C accelerator.
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
	// Defensive clamp (mirrors updateOpenConfirmDelete): if the copy
	// somehow isn't in the refreshed list, the cursor keeps its old value
	// — keep it in range so the next row index / draw can't run off the end.
	if s.modalCursor >= len(s.modalPaths) {
		s.modalCursor = len(s.modalPaths) - 1
	}
	if s.modalCursor < 0 {
		s.modalCursor = 0
	}
	s.flash("Duplicated to " + core.MapIDFromPath(newPath))
}

func updateOpenRename(s *State) Action {
	pumpPrintableASCII(&s.modalRenaming, 64, acceptPrintable, nil)
	cmds := openRenameCmds(s)
	rects := modalButtonRow(centeredCardRect(openModalW, openModalH), cmdLabels(cmds))
	if act, ran := runModalCmds(cmds, rects); ran {
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
	rects := modalButtonRow(centeredCardRect(openModalW, openModalH), cmdLabels(cmds))
	if act, ran := runModalCmds(cmds, rects); ran {
		return act
	}
	return ActionNone
}

// openDeleteConfirmCmds: 0=Delete (Y), 1=Cancel (Esc/N).
func openDeleteConfirmCmds(s *State) []modalCmd {
	return []modalCmd{
		{label: "Delete", hot: keyHot(rl.KeyY), run: func() Action { openDeleteSelected(s); return ActionNone }},
		{label: "Cancel", hot: func() bool { return editorCancelPressed() || rl.IsKeyPressed(rl.KeyN) },
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
		rects := modalButtonStack(centeredCardRect(saveAsModalW, saveAsModalH), cmdLabels(cmds))
		if act, ran := runModalCmds(cmds, rects); ran {
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
		{label: "Cancel", hot: func() bool { return rl.IsKeyPressed(rl.KeyN) || editorCancelPressed() },
			run: func() Action {
				s.awaitingOverwrite = false
				s.focus = focusFilename
				return ActionNone
			}},
	}
}

// updateEscMenuModal handles input for the editor's pause-style menu.
//   - Esc / C: close menu, resume editing.
//   - D: toggle display mode (fullscreen ↔ windowed). Menu stays open
//     so the author can verify the swap before continuing.
//   - E: exit to title. Routes through modalConfirmDirty when the
//     area has unsaved edits so save/discard/cancel still works —
//     same flow the old "Esc = exit" path used.
func updateEscMenuModal(s *State) Action {
	cmds := escMenuCmds(s)
	rects := modalButtonStack(centeredCardRect(escMenuModalW, escMenuModalH), cmdLabels(cmds))
	if act, ran := runModalCmds(cmds, rects); ran {
		return act
	}
	return ActionNone
}

// escMenuCmds: 0=Display (D), 1=Continue (Esc/C), 2=Exit to Title (E).
func escMenuCmds(s *State) []modalCmd {
	return []modalCmd{
		{label: render.DisplayMenuRowLabel(), hot: keyHot(rl.KeyD),
			run: func() Action { render.ToggleDisplayMode(); return ActionNone }},
		{label: "Continue editing", hot: cancelHot,
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
	rects := modalButtonStack(centeredCardRect(confirmDirtyModalW, confirmDirtyModalH), cmdLabels(cmds))
	if act, ran := runModalCmds(cmds, rects); ran {
		return act
	}
	return ActionNone
}

// confirmDirtyCmds: 0=Save (S), 1=Discard (D), 2=Cancel (Esc/C).
func confirmDirtyCmds(s *State) []modalCmd {
	return []modalCmd{
		{label: "Save", hot: keyHot(rl.KeyS), run: func() Action { return confirmDirtySave(s) }},
		{label: "Discard", hot: keyHot(rl.KeyD), run: func() Action { closeModal(s); return runPendingAction(s) }},
		{label: "Cancel", hot: cancelHot, run: func() Action {
			closeModal(s)
			s.pending = pendingNone
			return ActionNone
		}},
	}
}

// confirmDirtySave persists the current map (or opens Save As when it has
// no path yet), then runs the pending action. Shared by the Save button
// and the S accelerator.
func confirmDirtySave(s *State) Action {
	if s.area.Path == "" {
		openSaveAsModal(s)
		return ActionNone
	}
	mf, err := core.MapFileFromArea(s.area)
	if err != nil {
		s.flash("Save failed: " + err.Error())
		closeModal(s)
		s.pending = pendingNone
		return ActionNone
	}
	if err := mapfile.Save(s.area.Path, mf); err != nil {
		s.flash("Save failed: " + err.Error())
		closeModal(s)
		s.pending = pendingNone
		return ActionNone
	}
	s.baseline = core.CloneArea(s.area)
	s.dirty = false
	closeModal(s)
	return runPendingAction(s)
}

// keyHot / cancelHot build modalCmd accelerator predicates. cancelHot is
// the editor's "back" edge (Esc / pad B) plus the C key several confirm
// modals also accept.
func keyHot(k int32) func() bool { return func() bool { return rl.IsKeyPressed(k) } }
func cancelHot() bool            { return editorCancelPressed() || rl.IsKeyPressed(rl.KeyC) }

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
	s.baseline = core.CloneArea(s.area)
	s.dirty = false
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

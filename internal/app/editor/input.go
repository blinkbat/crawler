package editor

import (
	"crawler/internal/app/core"
	"crawler/internal/app/core/mapfile"
	"crawler/internal/app/input"
	"crawler/internal/app/render"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"

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

// editorAddPressed / editorDeletePressed / editorEditPressed are the editor's
// list-modal verb keys (A add, X delete, E edit), in one place so the list modals
// (packs, chests, doors, dialog nodes/choices/conditions, locations, sounds) can't
// drift on the mnemonic. (M is NOT centralized — it's a per-modal toggle, not a
// uniform verb.) Editor is keyboard-exempt, so raw rl reads are allowed here.
func editorAddPressed() bool    { return rl.IsKeyPressed(rl.KeyA) }
func editorDeletePressed() bool { return rl.IsKeyPressed(rl.KeyX) }
func editorEditPressed() bool   { return rl.IsKeyPressed(rl.KeyE) }

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

// anyDismissPressed reports whether the user pressed anything that should close a
// read-only viewer modal: confirm, cancel, Space, or a left click. Shared by the
// validate + hit-glyphs viewers so their dismiss set can't drift.
func anyDismissPressed() bool {
	return editorCancelPressed() || editorCommitPressed() || rl.IsKeyPressed(rl.KeySpace) ||
		rl.IsMouseButtonPressed(rl.MouseLeftButton)
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

	// Undo/redo must not fire mid-stroke: a drag banks its single lazy snapshot on
	// the first changed cell, so popping it while the button is still held would let
	// the rest of the stroke mutate cells with no undo entry and desync redo. Defer
	// until the drag releases (finishDrag).
	canHistory := s.drag == dragNone
	switch {
	case canHistory && ctrl && shift && rl.IsKeyPressed(rl.KeyZ):
		redoOne(s)
	case ctrl && shift && rl.IsKeyPressed(rl.KeyS):
		openSaveAsModal(s) // must precede the plain Ctrl+S (Save) case below
	case ctrl && shift && rl.IsKeyPressed(rl.KeyF):
		fillEntireLayer(s)
	case canHistory && ctrl && rl.IsKeyPressed(rl.KeyZ):
		undoOne(s)
	case canHistory && ctrl && rl.IsKeyPressed(rl.KeyY):
		redoOne(s)
	case ctrl && rl.IsKeyPressed(rl.KeyC):
		copySelection(s)
	case ctrl && rl.IsKeyPressed(rl.KeyV):
		pasteSelection(s, s.hoverX, s.hoverZ)
	case ctrl && rl.IsKeyPressed(rl.KeyX):
		cutSelection(s)
	case ctrl && rl.IsKeyPressed(rl.KeyA):
		selectWholeMap(s)
	case ctrl && rl.IsKeyPressed(rl.KeyD):
		duplicateSelection(s)
	case rl.IsKeyPressed(rl.KeyEscape) && s.selActive:
		s.selActive = false
		s.cancelHandled = true
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
			if s.hoverX >= 0 {
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

	// Brush size cycling (grid layers only — entity/multi-tile brushes ignore
	// brushSize, so let the keys pass through rather than silently mutate it).
	if !ctrl && isGridLayer(s.layer) {
		if rl.IsKeyPressed(rl.KeyLeftBracket) {
			stepBrushSize(s, -1)
		}
		if rl.IsKeyPressed(rl.KeyRightBracket) {
			stepBrushSize(s, +1)
		}
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

	// +/- zoom the active view (keyboard/trackpad twin of the wheel); Ctrl+0 fits
	// the whole map to the canvas. '=' shares the '+' key on most layouts.
	if !ctrl && !alt {
		if rl.IsKeyPressed(rl.KeyEqual) || rl.IsKeyPressed(rl.KeyKpAdd) {
			keyboardZoom(s, +1)
		}
		if rl.IsKeyPressed(rl.KeyMinus) || rl.IsKeyPressed(rl.KeyKpSubtract) {
			keyboardZoom(s, -1)
		}
	}
	if ctrl && (rl.IsKeyPressed(rl.KeyZero) || rl.IsKeyPressed(rl.KeyKp0)) {
		zoomToFit(s)
	}

	// R rotates. On the Props layer it cycles the hovered prop's facing (or the pending
	// brush facing); on Entities with the player-start brush it cycles the start facing.
	// setStartFacing banks undo (+bumps contentEpoch) like the context menu.
	if !ctrl && !alt && rl.IsKeyPressed(rl.KeyR) {
		switch {
		case s.layer == LayerProps:
			rotatePropFacing(s)
		case s.layer == LayerEntities && s.activeBrush().Entity == entityPlayerStart:
			setStartFacing(s, core.NormalizeFacing(s.area.StartFacing+1))
		}
	}

	// T cycles the day/night preview phase (seeds StepCount on F5).
	if !ctrl && rl.IsKeyPressed(rl.KeyT) {
		cyclePreviewPhase(s)
	}

	// L toggles the recent-messages recall panel (so an expired toast can be re-read).
	if !ctrl && !alt && rl.IsKeyPressed(rl.KeyL) {
		s.showStatusLog = !s.showStatusLog
	}

	// ? (Shift+/) opens the full keyboard-shortcut reference.
	if rl.IsKeyPressed(rl.KeySlash) && shift {
		openHelpModal(s)
	}

	// Shift+Arrows nudge the active selection's contents (or, with the Player Start
	// brush, the start tile) one tile per press — the desktop-editor twin of dragging.
	// Plain arrows still pan (updateArrowPan skips itself while Shift is held).
	if shift && !ctrl && !alt {
		if dx, dz := arrowPressedDelta(); dx != 0 || dz != 0 {
			nudgeSelectionOrStart(s, dx, dz)
		}
	}

	updateArrowPan(s)
}

// arrowPressedDelta returns the one-tile step from arrow keys pressed THIS frame
// (discrete, unlike updateArrowPan's held-pan). (0,0) when none pressed.
func arrowPressedDelta() (dx, dz int) {
	if rl.IsKeyPressed(rl.KeyRight) {
		dx++
	}
	if rl.IsKeyPressed(rl.KeyLeft) {
		dx--
	}
	if rl.IsKeyPressed(rl.KeyDown) {
		dz++
	}
	if rl.IsKeyPressed(rl.KeyUp) {
		dz--
	}
	return dx, dz
}

// nudgeSelectionOrStart shifts the committed marquee by (dx,dz), or moves the player
// start when the Player Start entity brush is active and no marquee is up. No-op
// otherwise (so Shift+Arrow with nothing to move is silently inert, not a pan).
func nudgeSelectionOrStart(s *State, dx, dz int) {
	switch {
	case s.selActive:
		moveSelectionBy(s, dx, dz)
	case s.layer == LayerEntities && s.activeBrush().Entity == entityPlayerStart:
		commitPaintIfChanged(s, func() { moveStartTo(s, s.area.StartTileX+dx, s.area.StartTileZ+dz) })
	}
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

// surfaceLevelSpan returns the clamped highest/lowest levels for the Levels-panel
// range: the ground baseline is always included (a flat/empty map collapses both to
// it), so the panel never opens below/above the terrain. One scan via areaLevelSpan.
func surfaceLevelSpan(a *core.AreaDefinition) (top, bottom int) {
	lo, hi, found := areaLevelSpan(a)
	if !found {
		return clampLevel(core.ElevationBaseline), clampLevel(core.ElevationBaseline)
	}
	if hi < core.ElevationBaseline {
		hi = core.ElevationBaseline
	}
	if lo > core.ElevationBaseline {
		lo = core.ElevationBaseline
	}
	return clampLevel(hi), clampLevel(lo)
}

// surfaceAreaLevels sets the Levels-panel range to span every level the area
// uses and puts the active floor on the start tile (not level 0, a pit below the
// baseline), revealing all levels. Shared by Open and the default-map load.
func surfaceAreaLevels(s *State) {
	s.topLevel, s.bottomLevel = surfaceLevelSpan(&s.area)
	s.editLevel = clampLevel(s.area.ElevationLevelAt(s.area.StartTileX, s.area.StartTileZ))
	s.levelHidden = [maxEditLevel + 1]bool{}
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
	// Home must also re-home the 3D orbit (yaw/pitch/zoom/target) — else it's inert
	// in the default iso view. Shares the freshState camera defaults.
	s.isoYaw = isoDefaultYaw
	s.isoPitch = isoDefaultPitch
	s.isoZoom = 1
	s.isoTargetX, s.isoTargetZ = 0, 0
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

// Navigation tunables. arrowPanStep is the top-down px the map pans per held
// frame; isoArrowPanPx the equivalent 3D screen-delta fed to isoPanTarget;
// panDragThreshold the px a right-drag must exceed to count as a drag (vs a
// context-menu click).
const (
	arrowPanStep     = float32(40)
	isoArrowPanPx    = float32(58)
	panDragThreshold = float32(4)
)

// updateArrowPan pans the map while an arrow key is held — the keyboard twin of
// the mouse pan-drag. A held arrow is treated as a drag in that screen direction;
// top-down shares the mouse pan rule outright, and 3D shares it horizontally but
// inverts vertical (arrows push the camera, not grab the world — see below).
// Replaces the retired arrow→grid-cursor keyboard-paint nav.
func updateArrowPan(s *State) {
	// Shift+Arrows nudge the selection/start (updateHotkeys) — don't also pan.
	if _, shift, _ := modifiers(); shift {
		return
	}
	left, right := rl.IsKeyDown(rl.KeyLeft), rl.IsKeyDown(rl.KeyRight)
	up, down := rl.IsKeyDown(rl.KeyUp), rl.IsKeyDown(rl.KeyDown)
	if !(left || right || up || down) {
		return
	}
	var dx, dy float32 // screen-space drag equivalent (dx = right, dy = down)
	if right {
		dx += 1
	}
	if left {
		dx -= 1
	}
	if down {
		dy += 1
	}
	if up {
		dy -= 1
	}
	if s.isoView {
		// Vertical is negated vs. the mouse drag-pan: Up/Down move the CAMERA over
		// the map (push the view up/down), the opposite sense from grab-and-drag.
		s.isoPanTarget(dx*isoArrowPanPx, -dy*isoArrowPanPx)
		return
	}
	// Top-down pan-drag is panX/panY += delta; mirror it so arrows match dragging.
	s.panX += dx * arrowPanStep
	s.panY += dy * arrowPanStep
}

// updateRightDrag arms right-button click-vs-drag discrimination: a press records
// the start and clears the moved flag; holding past panDragThreshold sets it.
// Shared by both canvases so the threshold semantics can't drift — a release with
// rightDragMoved==false is a click (opens the context menu); a drag pans/orbits.
func (s *State) updateRightDrag(mp rl.Vector2) {
	if rl.IsMouseButtonPressed(rl.MouseRightButton) {
		s.rightDragStart = mp
		s.rightDragMoved = false
	}
	if rl.IsMouseButtonDown(rl.MouseRightButton) && !s.rightDragMoved &&
		rl.Vector2Distance(mp, s.rightDragStart) > panDragThreshold {
		s.rightDragMoved = true
	}
}

// updateMouse processes top-bar / palette / metadata clicks and grid painting.
func updateMouse(s *State) {
	mp := rl.GetMousePosition()

	// Screen-space chrome (menu bar, toolbar, layer dropdown, Levels panel,
	// palette, metadata, minimap, recents) works in BOTH the top-down and 3D
	// views — only the grid canvas itself differs. Running it here keeps the
	// Levels panel (active floor), palette and menus fully usable while editing
	// in 3D, instead of being mouse-inert.
	if handleChromeInput(s, mp) {
		return
	}

	// 3D view: the grid canvas is the orbiting world — updateIsoCanvas owns
	// ray-picking, camera, and per-tile editing there.
	if s.isoView {
		updateIsoCanvas(s, mp)
		return
	}

	// --- Top-down grid canvas ---
	hx, hz := s.cellAt(mp)
	s.hoverX, s.hoverZ = hx, hz

	inGrid := pointIn(mp, s.rect.grid)
	if inGrid {
		if w := rl.GetMouseWheelMove(); w != 0 {
			zoomBy(s, mp, 1+canvasZoomWheelRate*w)
		}
	}

	// Right-drag pans; a right-click (no drag past the threshold) opens the context
	// menu on release. The mousewheel BUTTON is intentionally unbound everywhere —
	// right-drag replaced the old middle-drag pan. Left button stays paint.
	s.updateRightDrag(mp)
	if rl.IsMouseButtonPressed(rl.MouseRightButton) && inGrid {
		s.panning = true
	}
	if s.panning && rl.IsMouseButtonDown(rl.MouseRightButton) {
		d := rl.GetMouseDelta()
		s.panX += d.X
		s.panY += d.Y
	}
	if rl.IsMouseButtonReleased(rl.MouseRightButton) {
		clicked := s.panning && !s.rightDragMoved
		s.panning = false
		if clicked && inGrid && hx >= 0 {
			// Ramp mode: a click clears a ramp (floor → auto), keeping the elevation
			// digit so the cliff stays. Sculpt mode: a click lowers the column −1.
			// Else open the context menu.
			switch {
			case s.rampMode:
				isoClearRamp(s, hx, hz)
			case s.sculptMode && s.layer == LayerElevation:
				sculptLowerAt(s, hx, hz)
			default:
				openContextMenu(s, mp.X, mp.Y, hx, hz)
			}
		}
	}

	if inGrid && hx >= 0 {
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
	}

	if rl.IsMouseButtonReleased(rl.MouseLeftButton) {
		finishDrag(s)
	}
}

// handleChromeInput processes the editor's screen-space chrome — panel wheels,
// scrollbars, the menu bar / toolbar / layer dropdown, the Levels panel, palette
// and metadata clicks, minimap jump, and recent-brush swatches. It is identical
// in the top-down and 3D views (only the grid canvas differs), so both call it;
// returns true when it consumed the input and the caller should stop.
func handleChromeInput(s *State, mp rl.Vector2) bool {
	// Panel wheel (the grid-canvas wheel is view-specific, handled by the caller).
	if pointIn(mp, s.rect.palette) {
		if w := rl.GetMouseWheelMove(); w != 0 {
			ScrollPalette(s, -w*paletteRowStride*paletteWheelRows)
		}
	} else if pointIn(mp, s.rect.metadata) {
		if w := rl.GetMouseWheelMove(); w != 0 {
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

	// Scrollbars run before clicks so grabbing a thumb doesn't bleed into them.
	if s.updateScrollbars(mp) {
		return true
	}

	if rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		if hit := topbarButtonAt(s, mp); hit >= 0 {
			menuBarBtns[hit].action(s) // opens that menu's pull-down (menus.go)
			return true
		}
		// Top-bar layer dropdown: the active-layer picker (rows carry the eye).
		if pointIn(mp, layerMenuBtnRect(s)) {
			openDropdownBelow(s, ddLayer, layerMenuBtnRect(s))
			return true
		}
		if hit := toolbarButtonAt(s, mp); hit >= 0 {
			// Disabled buttons swallow the click without firing.
			if b := toolbarBtns[hit]; b.enabled == nil || b.enabled(s) {
				b.action(s)
			}
			return true
		}
		// Levels panel, checked before the palette so its column isn't swallowed.
		if pointIn(mp, s.rect.levels) {
			handleLevelsPanelClick(s, mp)
			return true
		}
		if hit := paletteToolAt(s, mp); hit >= 0 {
			s.brushIdx[s.layer] = hit
			recordRecentBrush(s)
			return true
		}
		if handleMetadataClick(s, mp) {
			return true
		}
	}

	// Minimap click-and-drag recenters the view. A press that starts on the minimap
	// begins a drag that keeps recentering while held (even off the minimap rect), so
	// the viewport frame can be dragged around. Checked before grid-paint since the
	// minimap overlaps the grid pane.
	if rl.IsMouseButtonReleased(rl.MouseLeftButton) {
		s.minimapDragging = false
	}
	if mr, ok := minimapRect(s); ok {
		if rl.IsMouseButtonPressed(rl.MouseLeftButton) && pointIn(mp, mr) {
			s.minimapDragging = true
		}
		if s.minimapDragging && rl.IsMouseButtonDown(rl.MouseLeftButton) {
			scale := mr.Width / float32(s.area.Width)
			tx := core.Clamp(int((mp.X-mr.X)/scale), 0, s.area.Width-1)
			tz := core.Clamp(int((mp.Y-mr.Y)/scale), 0, s.area.Height-1)
			centerViewOnTile(s, tx, tz)
			return true
		}
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
				return true
			}
		}
	}
	return false
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
	// Measure tool: a non-editing ruler drag, available on any layer (checked before
	// the entity-layer branch so it works there too).
	if s.tool == toolMeasure {
		s.drag = dragMeasure
		s.rectAnchorX, s.rectAnchorZ = x, z
		return
	}
	// Sculpt mode (Elevation): a freehand stroke that raises columns +1, regardless of
	// the selected tool. applyTool routes the per-cell raise; lowering is right-click.
	if s.sculptMode && s.layer == LayerElevation {
		beginPaintStroke(s)
		strokePaint(s, x, z)
		s.lastPaintX, s.lastPaintZ = x, z
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
		// Press inside a committed marquee grabs it to MOVE its contents; elsewhere
		// starts a fresh marquee.
		if s.selActive && inRect(x, z, s.selX0, s.selZ0, s.selX1, s.selZ1) {
			s.drag = dragSelectMove
		} else {
			s.drag = dragSelect
		}
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
	wasHidden := s.layerHidden[s.layer]
	applyToolBrushed(s, x, z)
	if s.dragSnapshotDone {
		// Snapshot already banked, but later cells in the drag still mutate content —
		// invalidate the caches commitUndoSnapshot would otherwise refresh on cell one only.
		invalidateContentCaches(s)
		return
	}
	if core.AreaContentEqual(s.area, s.dragUndoBefore) {
		// Refused / no-op cell: undo applyTool's optimistic dirty AND layer-reveal
		// flips (matches the context-menu erase path) so nothing changed visibly.
		s.dirty = wasDirty
		s.layerHidden[s.layer] = wasHidden
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
// Each step goes through strokePaint (shared Bresenham walkLine, ops.go). In the 3D
// view the interpolated cells are level-gated the same way the live hover is (iso.go):
// a sweep that crosses a wrong-level column must NOT stamp those intervening cells,
// or the gate that blocks editing raised/lowered columns is defeated between two
// valid endpoints.
func paintLineBetween(s *State, x0, z0, x1, z1 int) {
	walkLine(x0, z0, x1, z1, func(cx, cz int) {
		if cx == x0 && cz == z0 {
			return // start already painted
		}
		if s.isoView && s.isoWrongLevel(cx, cz) {
			return // 3D: skip columns not on the active edit level (leaves a gap, by design)
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

// commitPaintIfChanged runs paint under a lazy-undo guard: it snapshots first and,
// if paint changed nothing, reverts the optimistic dirty + layer-reveal flips its
// callees make; otherwise it banks ONE undo step. Shared by the rect/line drag commits
// and the context-menu erase so an empty/all-refused bulk op can't leave a junk undo.
func commitPaintIfChanged(s *State, paint func()) {
	wasDirty := s.dirty
	wasHidden := s.layerHidden[s.layer]
	before := core.CloneArea(s.area)
	paint()
	if core.AreaContentEqual(s.area, before) {
		s.dirty = wasDirty
		s.layerHidden[s.layer] = wasHidden
	} else {
		commitUndoSnapshot(s, before)
	}
}

func finishDrag(s *State) {
	switch s.drag {
	case dragStart:
		if s.hoverX >= 0 && (s.hoverX != s.area.StartTileX || s.hoverZ != s.area.StartTileZ) {
			// moveStartTo no longer self-banks — wrap the drag release in the lazy seam.
			commitPaintIfChanged(s, func() { moveStartTo(s, s.hoverX, s.hoverZ) })
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
			// Lazy commit (not eager pushUndo): an empty / all-refused rect must not bank
			// a junk undo. Shared with the line drag + context erase via commitPaintIfChanged.
			commitPaintIfChanged(s, func() {
				if s.rectHollow {
					paintRectOutline(s, s.rectAnchorX, s.rectAnchorZ, s.hoverX, s.hoverZ)
				} else {
					paintRect(s, s.rectAnchorX, s.rectAnchorZ, s.hoverX, s.hoverZ)
				}
			})
		}
	case dragLine:
		if s.hoverX >= 0 {
			commitPaintIfChanged(s, func() {
				paintLine(s, s.rectAnchorX, s.rectAnchorZ, s.hoverX, s.hoverZ)
			})
		}
	case dragSelect:
		// Commit the marquee as the active selection (normalized inclusive bounds).
		if s.hoverX >= 0 {
			s.selX0, s.selX1 = min(s.rectAnchorX, s.hoverX), max(s.rectAnchorX, s.hoverX)
			s.selZ0, s.selZ1 = min(s.rectAnchorZ, s.hoverZ), max(s.rectAnchorZ, s.hoverZ)
			s.selActive = true
		}
	case dragSelectMove:
		// Shift the selection's contents by the drag delta (grid + entities, one undo).
		if s.hoverX >= 0 {
			moveSelectionBy(s, s.hoverX-s.rectAnchorX, s.hoverZ-s.rectAnchorZ)
		}
	case dragMeasure:
		// Ruler: commit nothing; flash the final reading so it survives the mouse-up.
		if s.hoverX >= 0 {
			s.flash("Measured " + measureLabel(s.rectAnchorX, s.rectAnchorZ, s.hoverX, s.hoverZ))
		}
	}
	s.drag = dragNone
	s.rectHollow = false
	s.dragPackIdx = -1
	s.dragChestIdx = -1
	s.dragDoorIdx = -1
}

// applyToolBrushed runs the brush over the brush-size square at (x,z). Entity
// brushes collapse to a single cell. In scatter mode (Decor/Props, size > 1, simple
// brush) each covered cell stamps only with probability scatterDensity, laying an
// organic field instead of a solid block.
func applyToolBrushed(s *State, x, z int) {
	scatter := s.scatterDensity > 0 && onScatterLayer(s) && s.brushSize > 1 && !brushHasMultiTileFootprint(s)
	forEachBrushCell(s, x, z, func(cx, cz int) {
		if scatter && rand.Float32() >= s.scatterDensity {
			return
		}
		applyTool(s, cx, cz)
	})
}

// scatterDensitySteps cycles the scatter probability (0 = off) via the toolbar button.
var scatterDensitySteps = []float32{0, 0.25, 0.5, 0.75}

// cycleScatterDensity advances the scatter density one step (wrapping) and flashes it.
func cycleScatterDensity(s *State) {
	idx := 0
	for i, v := range scatterDensitySteps {
		if v == s.scatterDensity {
			idx = i
			break
		}
	}
	s.scatterDensity = scatterDensitySteps[(idx+1)%len(scatterDensitySteps)]
	if s.scatterDensity == 0 {
		s.flash("Scatter off")
	} else {
		s.flash(fmt.Sprintf("Scatter %d%% (size>1 brush)", int(s.scatterDensity*100)))
	}
}

// forEachBrushCell invokes fn for each cell the current brush covers when stamped
// at (cx,cz): a single cell for entity / size-1 / multi-tile-footprint brushes,
// else the brushSize×brushSize block centered on it. Shared by the paint path
// (applyToolBrushed) and the 3D hover preview (drawIsoBrushPreview) so the covered
// set can't drift.
func forEachBrushCell(s *State, cx, cz int, fn func(x, z int)) {
	half := s.brushSize / 2
	if !isGridLayer(s.layer) || s.brushSize <= 1 || brushHasMultiTileFootprint(s) {
		fn(cx, cz)
		return
	}
	for dz := -half; dz <= half; dz++ {
		for dx := -half; dx <= half; dx++ {
			fn(cx+dx, cz+dz)
		}
	}
}

// brushHasMultiTileFootprint reports whether the active Props/Decor brush stamps
// a multi-tile footprint — those collapse to a single anchor stamp under size>1.
func brushHasMultiTileFootprint(s *State) bool {
	return activeFootprint(s) != nil
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
// (including sentinels). ok is false for Entities (no per-tile char). Per layer via
// layerDefs in layerdef.go.
func activeLayerCharAt(s *State, x, z int) (byte, bool) {
	return layerDefs[s.layer].charAt(s, x, z)
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

// wheelZoom applies a multiplicative wheel-notch zoom — cur·(1 + rate·wheel) —
// clamped to [min,max]. Seeds a zero/negative cur to 1 so multiplicative zoom can't
// get stuck at 0. Shared by the iso 3D view + the object-browser previews so the
// gesture reads identically (zoomBy takes a precomputed factor; the foe/party
// visualizers zoom additively — both stay their own shape).
func wheelZoom(cur, wheel, rate, min, max float32) float32 {
	if cur <= 0 {
		cur = 1
	}
	return core.Clamp(cur*(1+rate*wheel), min, max)
}

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

// keyboardZoomStep is the per-keypress zoom notch for the +/- keys — a touch
// coarser than a wheel notch (canvasZoomWheelRate) so a keyboard zoom moves faster.
const keyboardZoomStep = float32(3)

// keyboardZoom zooms the active view one step (dir +1 in / -1 out), anchored at the
// canvas center in top-down and via isoZoom in 3D, so +/- read like the wheel in
// whichever view is active.
func keyboardZoom(s *State, dir float32) {
	if s.isoView {
		s.isoZoom = wheelZoom(s.isoZoom, dir*keyboardZoomStep, canvasZoomWheelRate, isoMinZoom, isoMaxZoom)
		return
	}
	center := rl.NewVector2(s.rect.gridX+s.rect.gridW/2, s.rect.gridY+s.rect.gridH/2)
	zoomBy(s, center, 1+canvasZoomWheelRate*dir*keyboardZoomStep)
}

// zoomToFit sizes the top-down canvas so the target fits, re-centered — the active
// marquee SELECTION when one is set, else the whole map. In 3D it re-homes the orbit
// (fit has no single meaning there), centering on the selection when present.
func zoomToFit(s *State) {
	// Target bounds: the selection if active, else the full map.
	x0, z0, x1, z1 := 0, 0, s.area.Width-1, s.area.Height-1
	if s.selActive {
		x0, z0, x1, z1 = s.selX0, s.selZ0, s.selX1, s.selZ1
	}
	w, h := x1-x0+1, z1-z0+1
	cx, cz := (x0+x1)/2, (z0+z1)/2

	if s.isoView {
		if s.selActive {
			centerViewOnTile(s, cx, cz)
		} else {
			resetView(s)
		}
		return
	}
	base := float32(0)
	if s.zoom > 0 {
		base = s.rect.cellPx / s.zoom // px-per-cell at 1× zoom
	}
	if base <= 0 || w <= 0 || h <= 0 {
		return
	}
	fitCell := s.rect.gridW / float32(w)
	if hh := s.rect.gridH / float32(h); hh < fitCell {
		fitCell = hh
	}
	s.zoom = core.Clamp(fitCell/base, minZoom, maxZoom)
	s.panX, s.panY = 0, 0
	if s.selActive {
		s.layout() // re-lay at the new zoom so the follow-on centering uses fresh cellPx
		centerViewOnTile(s, cx, cz)
	}
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
	focusDialogCondQuestID:  {defaultTextFieldMaxLen, acceptPrintableNoSpace},
	focusDialogCondMessage:  {dialogProseMaxLen, acceptPrintable},
	// Numeric foci — every field reachable from dialogNumericTarget must be here with
	// acceptDigit, else it falls back to the printable/96-char default (init asserts this).
	focusDialogCondGold:     {dialogNumFieldMaxLen, acceptDigit},
	focusDialogCondFoeKills: {dialogNumFieldMaxLen, acceptDigit},
	focusDialogCondTileX:    {dialogNumFieldMaxLen, acceptDigit},
	focusDialogCondTileZ:    {dialogNumFieldMaxLen, acceptDigit},
	focusDialogTrigTileX:    {dialogNumFieldMaxLen, acceptDigit},
	focusDialogTrigTileZ:    {dialogNumFieldMaxLen, acceptDigit},
	focusDialogTrigFoeKills: {dialogNumFieldMaxLen, acceptDigit},
	focusDialogTrigLevel:    {dialogNumFieldMaxLen, acceptDigit},
	focusTrigActTileX:       {dialogNumFieldMaxLen, acceptDigit},
	focusTrigActTileZ:       {dialogNumFieldMaxLen, acceptDigit},
	focusTrigActCount:       {dialogNumFieldMaxLen, acceptDigit},
	focusTrigActLevel:       {dialogNumFieldMaxLen, acceptDigit},
}

// dialogNumericFoci lists every focus dialogNumericTarget edits as an int; keep it in
// lockstep with that switch. init asserts each has a numeric textFieldConfigs row so a
// new numeric field can't silently inherit the printable/96-char text default.
var dialogNumericFoci = []focusField{
	focusDialogCondGold, focusDialogCondFoeKills, focusDialogCondTileX, focusDialogCondTileZ,
	focusDialogTrigTileX, focusDialogTrigTileZ, focusDialogTrigFoeKills, focusDialogTrigLevel,
	focusTrigActTileX, focusTrigActTileZ, focusTrigActCount, focusTrigActLevel,
}

func init() {
	for _, f := range dialogNumericFoci {
		cfg, ok := textFieldConfigs[f]
		if !ok || cfg.MaxLen != dialogNumFieldMaxLen {
			panic(fmt.Sprintf("editor: numeric focus %d missing acceptDigit textFieldConfigs row", f))
		}
	}
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

// pumpFocusField pumps printable runes into `target` using s.focus's config. Arms
// the prose-text-field lazy undo first (a no-op for non-area fields).
func pumpFocusField(s *State, target *string) {
	s.armTextUndo()
	cfg := configForFocus(s.focus)
	pumpPrintableASCII(target, cfg.MaxLen, cfg.Accept, s.onFocusedTextEdit)
}

// focusIsAreaText reports whether f is a PROSE field whose keystrokes mutate the
// area directly (so it wants lazy undo). Excludes the filename field (doesn't touch
// the map) and every numeric field (map dims + dialog numerics carry their own undo
// via commitNumericInput / pumpDialogNumeric).
func focusIsAreaText(f focusField) bool {
	switch f {
	case focusName, focusQuiet, focusLocationName,
		focusDoorName, focusDoorTargetMap, focusDoorTargetDoor,
		focusDialogNodeText, focusDialogNodeNext, focusDialogNodeContinue,
		focusDialogChoiceLabel, focusDialogChoiceNext,
		focusDialogCondQuestID, focusDialogCondMessage,
		focusTrigCondText, focusTrigActText, focusWallFeatureSwitch:
		return true
	}
	return false
}

// armTextUndo snapshots the pre-edit area the frame a prose text field gains focus,
// so onFocusedTextEdit can bank one lazy undo step per focus session. Keyed on focus
// identity (disarmed to focusNone in Update when the field defocuses). No-op for
// non-area fields.
func (s *State) armTextUndo() {
	if !focusIsAreaText(s.focus) {
		return
	}
	s.textUndo.armFor(s, s.focus)
}

// onFocusedTextEdit is pumpFocusField's per-keystroke onChange: dirty + cache
// invalidation (markDirty), plus ONE lazy undo step for area-mutating prose fields.
func (s *State) onFocusedTextEdit() {
	if focusIsAreaText(s.focus) {
		s.textUndo.commitOnce(s)
	}
	s.markDirty()
}

// pumpFocusedTextField runs the shared focused-text-field control loop used by
// every editor modal with an inline text field: drain keystrokes into target,
// then Tab cycles focus (onTab; nil = no Tab handling), Enter defocuses, Esc backs
// out (onCancel). A nil target means the field vanished — drop focus defensively.
// The caller returns ActionNone afterward; the focused field owns the frame.
func pumpFocusedTextField(s *State, target *string, onTab, onCancel func()) {
	if target == nil {
		s.focus = focusNone
		return
	}
	pumpFocusField(s, target)
	if onTab != nil && editorTabPressed() {
		onTab()
		return
	}
	if editorCommitPressed() {
		s.focus = focusNone
		return
	}
	if editorCancelPressed() && onCancel != nil {
		onCancel()
	}
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
	// A focused-field edit (door targets especially) can change reachability and the
	// hover tooltip's content; invalidate the lazy caches so neither shows stale data.
	invalidateContentCaches(s)
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
	case modalWallFeatureEdit:
		if currentWallFeature(s) == nil {
			closeModal(s)
		}
	case modalLocationEdit:
		if s.modalLocationIdx < 0 || s.modalLocationIdx >= len(s.area.Locations) {
			closeModal(s)
		}
	case modalCrystalEdit:
		if s.modalCrystalIdx < 0 || s.modalCrystalIdx >= len(s.area.CrystalSpawns) {
			closeModal(s)
		}
	default:
		// Intentionally no arm: the remaining modals (foe/party/object views, new-map,
		// save, confirm-dirty) reference no spawn index, so there's nothing to invalidate.
		// A new INDEX-backed modal MUST add its own stale-index guard above.
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
	s.doorPickMaps = nil
	s.doorPickDoors = nil
	s.modalDialogIdx = -1
	s.modalDialogNodeIdx = -1
	s.modalDialogChoiceIdx = -1
	s.modalDialogCondIdx = -1
	s.modalDialogTriggerIdx = -1
	s.modalWallFeatureIdx = -1
	s.modalCrystalIdx = -1
	s.modalDialogActionOnChoice = false
	clearDialogFocus(s)
	closeDropdown(s) // picker must not survive its parent modal
	s.modalValidateRows = nil
	s.modalConfirmDelete = false
	s.modalRenaming = ""
	s.modalRenamingActive = false
	s.openFilter = ""
	s.prefabNameFocus = false
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

	// Mouse: click focuses a field, opens the facing/style picker, or deletes.
	if rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		mp := rl.GetMousePosition()
		// Floor stepper (top-right, multi-level maps): re-assign this door's voxel floor.
		if handleSpawnLevelClick(s, doorEditLayoutFor().card, &s.area.DoorSpawns[s.modalDoorIdx].Level, mp) {
			return ActionNone
		}
		hit := doorEditHitTest(s, mp)
		switch hit.kind {
		case doorHitName:
			s.focus = focusDoorName
			return ActionNone
		case doorHitTargetMap:
			s.focus = focusDoorTargetMap
			return ActionNone
		case doorHitTargetMapPick:
			openDoorTargetMapPicker(s, doorEditLayoutFor().mapPickBtn)
			return ActionNone
		case doorHitTargetDoor:
			s.focus = focusDoorTargetDoor
			return ActionNone
		case doorHitTargetDoorPick:
			openDoorTargetDoorPicker(s, doorEditLayoutFor().doorPickBtn)
			return ActionNone
		case doorHitFacing:
			openFieldDropdown(s, ddDoorFacing, doorEditLayoutFor().facingBtn)
			return ActionNone
		case doorHitStyle:
			openFieldDropdown(s, ddDoorStyle, doorEditLayoutFor().styleBtn)
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
		// pumpFocusField (inside the helper) arms the lazy undo + marks dirty, so no
		// second guard is needed.
		pumpFocusedTextField(s, doorEditTextTarget(s), func() { cycleDoorFocus(s) }, func() { closeModal(s) })
		return ActionNone
	}

	// No field focused — Esc/Enter close, Tab focuses the first field, X deletes.
	// Facing + style are dropdown pickers (generic Up/Down/Enter/Esc), not hotkeys.
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
	return ActionNone
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

// openCrystalEditModal opens the per-crystal editor for spawn index idx.
func openCrystalEditModal(s *State, idx int) {
	if idx < 0 || idx >= len(s.area.CrystalSpawns) {
		return
	}
	s.modal = modalCrystalEdit
	s.modalCrystalIdx = idx
	s.modalCursor = 0
}

// updateCrystalEditModal drives the crystal editor: the floor stepper + delete/close.
func updateCrystalEditModal(s *State) Action {
	if s.modalCrystalIdx < 0 || s.modalCrystalIdx >= len(s.area.CrystalSpawns) {
		closeModal(s)
		return ActionNone
	}
	card := centeredCardRect(crystalEditModalW, crystalEditModalH)
	if rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		mp := rl.GetMousePosition()
		if handleSpawnLevelClick(s, card, &s.area.CrystalSpawns[s.modalCrystalIdx].Level, mp) {
			return ActionNone
		}
		if pointIn(mp, bottomLeftBtn(card)) {
			deleteCrystalAt(s, s.modalCrystalIdx)
			return ActionNone
		}
		if pointIn(mp, bottomRightBtn(card)) {
			closeModal(s)
			return ActionNone
		}
		// Click inside the card but on no control: no-op (don't dismiss).
	}
	if editorCancelPressed() || editorCommitPressed() {
		closeModal(s)
		return ActionNone
	}
	if editorDeletePressed() {
		deleteCrystalAt(s, s.modalCrystalIdx)
		return ActionNone
	}
	return ActionNone
}

// deleteCrystalAt removes the crystal at idx (undo, dirty, close). Crystals carry no
// hand-authored links, so a single press deletes (undo restores it).
func deleteCrystalAt(s *State, idx int) {
	if idx < 0 || idx >= len(s.area.CrystalSpawns) {
		return
	}
	pushUndo(s)
	s.area.CrystalSpawns = removeModalListItem(s.area.CrystalSpawns, idx)
	s.dirty = true
	closeModal(s)
}

// openEntityListModal opens the Objects index (packs / chests / doors + start).
func openEntityListModal(s *State) {
	s.modal = modalEntityList
	s.modalCursor = 0
	s.entityListFilter = ""
}

// updateEntityListModal drives the Objects index: Up/Down, Enter/row-click jumps
// to the entity and opens its editor, Esc / click-outside closes.
func updateEntityListModal(s *State) Action {
	// Type-to-filter: printable chars extend the query, Backspace trims it (cursor
	// snaps to the top match so the filtered view + modalCursor can't drift).
	for {
		r := rl.GetCharPressed()
		if r == 0 {
			break
		}
		if r >= 32 && r < 127 {
			s.entityListFilter += string(rune(r))
			s.modalCursor = 0
		}
	}
	if rl.IsKeyPressed(rl.KeyBackspace) && len(s.entityListFilter) > 0 {
		s.entityListFilter = s.entityListFilter[:len(s.entityListFilter)-1]
		s.modalCursor = 0
	}
	rows := entityListRows(s)
	n := len(rows)
	s.modalCursor = input.CursorUpDown(s.modalCursor, n)
	if editorCancelPressed() {
		if s.entityListFilter != "" {
			s.entityListFilter = "" // first Esc clears the filter, second closes
			s.modalCursor = 0
			return ActionNone
		}
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
	case elCrystal:
		openCrystalEditModal(s, row.idx)
	default:
		closeModal(s)
	}
}

// updateValidateModal: any key closes; it's a read-only viewer.
func updateValidateModal(s *State) Action {
	if anyDismissPressed() {
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

	// Floor stepper (top-right, multi-level maps): re-assign this pack's voxel floor.
	if rl.IsMouseButtonPressed(rl.MouseLeftButton) &&
		handleSpawnLevelClick(s, centeredCardRect(entityEditModalW, entityEditModalH), &pack.Level, rl.GetMousePosition()) {
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
	s.flash(core.PackMemberDisplayName(*pack, s.modalCursor) + " → " + packRowLabel(pack, s.modalCursor) + " row")
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
	clampModalCursor(s, count)
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

	// Floor stepper (top-right, multi-level maps): re-assign this chest's voxel floor.
	if rl.IsMouseButtonPressed(rl.MouseLeftButton) &&
		handleSpawnLevelClick(s, centeredCardRect(entityEditModalW, entityEditModalH), &chest.Level, rl.GetMousePosition()) {
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
	clampModalCursor(s, len(chest.Items))
}

func updateOpenModal(s *State) Action {
	if s.modalRenamingActive {
		return updateOpenRename(s)
	}
	if s.modalConfirmDelete {
		return updateOpenConfirmDelete(s)
	}

	// Type-to-filter (dropdown-style): printable keys narrow the list, Backspace
	// deletes. Runs before the action cmds so the letter keys type instead of firing
	// the (now click-only) Rename/Delete/Duplicate buttons.
	pumpPrintableASCII(&s.openFilter, defaultTextFieldMaxLen, acceptPrintable, func() { s.modalCursor = 0 })

	if editorCancelPressed() {
		// Esc clears a live filter first, then closes on a second press.
		if s.openFilter != "" {
			s.openFilter = ""
			s.modalCursor = 0
			return ActionNone
		}
		closeModal(s)
		return ActionNone
	}
	vis := openVisiblePaths(s)
	if len(vis) == 0 {
		return ActionNone
	}
	s.modalCursor = input.CursorUpDown(s.modalCursor, len(vis))

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

// openVisiblePaths returns modalPaths filtered by the live openFilter query
// (case-insensitive map-id substring); the full list when the query is empty.
// The single seam both the updater and draw read so the cursor can't drift from
// what's shown.
func openVisiblePaths(s *State) []string {
	q := strings.TrimSpace(s.openFilter)
	if q == "" {
		return s.modalPaths
	}
	lq := strings.ToLower(q)
	out := make([]string, 0, len(s.modalPaths))
	for _, p := range s.modalPaths {
		if strings.Contains(strings.ToLower(core.MapIDFromPath(p)), lq) {
			out = append(out, p)
		}
	}
	return out
}

// selectedOpenPath returns the cursored path in the filtered view, or "" when the
// view is empty (e.g. a query matching nothing).
func selectedOpenPath(s *State) string {
	vis := openVisiblePaths(s)
	if s.modalCursor < 0 || s.modalCursor >= len(vis) {
		return ""
	}
	return vis[s.modalCursor]
}

// selectOpenPath moves the cursor to path in the filtered view (clamped if it's
// filtered out). Used after rename / duplicate so the cursor follows the result.
func selectOpenPath(s *State, path string) {
	vis := openVisiblePaths(s)
	for i, p := range vis {
		if p == path {
			s.modalCursor = i
			return
		}
	}
	clampModalCursor(s, len(vis))
}

// openModalActionCmds: 0=Open (Enter), then click-only Rename / Delete / Duplicate.
// The letter accelerators were dropped so the list supports type-to-filter; the
// buttons stay clickable. Row-click selection is handled separately in updateOpenModal.
func openModalActionCmds(s *State) []modalCmd {
	return []modalCmd{
		{label: "Open", hot: editorCommitPressed, run: func() Action { return openSelectedMap(s) }},
		{label: "Rename", run: func() Action {
			if p := selectedOpenPath(s); p != "" {
				s.modalRenaming = core.MapIDFromPath(p)
				s.modalRenamingActive = true
			}
			return ActionNone
		}},
		{label: "Delete", run: func() Action {
			if selectedOpenPath(s) != "" {
				s.modalConfirmDelete = true
			}
			return ActionNone
		}},
		{label: "Duplicate", run: func() Action { openDuplicateSelected(s); return ActionNone }},
	}
}

// openSelectedMap loads the cursored map. Shared by the Open button and Enter.
func openSelectedMap(s *State) Action {
	if p := selectedOpenPath(s); p != "" {
		loadAreaFromPath(s, p)
	}
	closeModal(s)
	return ActionNone
}

// loadAreaFromPath loads path into the editor, replacing the current area (undo/
// redo/dirty reset, caches invalidated, recent-list updated). Flashes on failure
// and leaves the current area intact. Shared by the Open modal + recent-file menu.
func loadAreaFromPath(s *State, path string) {
	mf, err := mapfile.Load(path)
	if err != nil {
		s.flashWarn("Open failed: " + err.Error())
		return
	}
	area, err := core.AreaFromMapFile(mf, path)
	if err != nil {
		s.flashWarn("Open failed: " + err.Error())
		return
	}
	area = materializeEntranceCrystal(area)
	s.area = area
	s.baseline = core.CloneArea(area)
	s.undo = nil
	s.redo = nil
	s.undoTrimmed = false
	s.dirty = false
	s.autosaveTimer = 0
	s.bookmarks = bookmarksForMap(path) // restore this map's persisted view bookmarks
	clearRecovery()                     // loaded a clean map — discard any stale recovery snapshot
	clearSelection(s)                   // different map — old selection coords no longer apply
	surfaceAreaLevels(s)
	// Area replaced wholesale — invalidate content-derived caches (like
	// performNewMap / undo / redo) or stale reachability/tooltip data lingers.
	invalidateContentCaches(s)
	rememberLastMap(path) // reopen here next session (NewDefault) + File ▸ recent
	s.flash("Opened " + core.MapIDFromPath(path))
}

// openRecentPath opens a File-menu recent entry. Non-destructive: refuses while the
// current map has unsaved changes (save/discard first) so a stray click can't lose work.
func openRecentPath(s *State, path string) {
	if s.dirty {
		s.flash("Unsaved changes — save or discard before opening a recent map")
		return
	}
	loadAreaFromPath(s, path)
}

// openDuplicateSelected copies the cursored map on disk and selects the copy.
func openDuplicateSelected(s *State) {
	src := selectedOpenPath(s)
	if src == "" {
		return
	}
	newPath, err := duplicateMapFile(src)
	if err != nil {
		s.flashWarn("Duplicate failed: " + err.Error())
		return
	}
	refreshOpenList(s)
	selectOpenPath(s, newPath) // follow the copy (clamps if the filter hides it)
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
		{label: "Cancel", hot: editorCancelPressed, run: func() Action { s.modalRenaming = ""; s.modalRenamingActive = false; return ActionNone }},
	}
}

func openRenameCommit(s *State) {
	oldPath := selectedOpenPath(s)
	if oldPath == "" {
		s.modalRenaming = ""
		s.modalRenamingActive = false
		return
	}
	newPath, err := renameMapFile(oldPath, s.modalRenaming)
	s.modalRenaming = ""
	s.modalRenamingActive = false
	if err != nil {
		s.flashWarn("Rename failed: " + err.Error())
		return
	}
	if s.area.Path == oldPath {
		s.area.Path = newPath
	}
	refreshOpenList(s)
	selectOpenPath(s, newPath)
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
	path := selectedOpenPath(s)
	if path == "" {
		s.modalConfirmDelete = false
		return
	}
	if err := os.Remove(path); err != nil {
		s.flashWarn("Delete failed: " + err.Error())
		s.modalConfirmDelete = false
		return
	}
	if s.area.Path == path {
		s.area.Path = ""
	}
	s.flash("Deleted " + core.MapIDFromPath(path))
	refreshOpenList(s)
	clampModalCursor(s, len(openVisiblePaths(s)))
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
		s.flashWarn("Save failed: " + err.Error())
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
		clearRecovery() // leaving with edits saved or discarded — no recovery to keep
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
		s.flashWarn("Save failed: " + err.Error())
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

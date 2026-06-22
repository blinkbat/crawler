package editor

import (
	"crawler/internal/app/core"
	"crawler/internal/app/render"
	"fmt"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// ctxItemKind enumerates the right-click menu's actions, dispatched by
// runContextItem against (tileX, tileZ). A new kind needs a runContextItem case.
type ctxItemKind int

const (
	ctxItemNone ctxItemKind = iota
	ctxItemEditPack
	ctxItemDeletePack
	ctxItemEditChest
	ctxItemDeleteChest
	ctxItemEditDoor
	ctxItemDeleteDoor
	ctxItemDeleteCrystal // no edit counterpart — crystals carry no per-instance data
	ctxItemMoveStartHere
	// ctxItemStartFacing sets StartFacing (the spawn fallback; per-door Facing
	// overrides it). The direction rides in ctxItem.facing — one kind, four rows.
	ctxItemStartFacing
	ctxItemSetWallFaces // opens the per-tile wall-faces modal (base + N/E/S/W)
	ctxItemEraseTile    // resets the ACTIVE layer's cell here
	// Location (region) actions — New always offered; Edit/Delete when a region on
	// the active level sits under the cursor.
	ctxItemNewLocation
	ctxItemEditLocation
	ctxItemDeleteLocation
)

// ctxItem is one right-click menu row, built fresh by contextItemsAt per tile.
type ctxItem struct {
	label string
	kind  ctxItemKind
	// facing is the payload for ctxItemStartFacing; ignored by other kinds.
	facing int
}

// contextMenuState is the in-State data for an open right-click menu (open=false
// when none). Recomputed on open; dismissed on click-outside / Esc / item-pick.
type contextMenuState struct {
	open         bool
	x, y         float32
	tileX, tileZ int
	items        []ctxItem
}

// isDelete reports whether a row is a destructive delete (drawn red).
func (k ctxItemKind) isDelete() bool {
	return k == ctxItemDeletePack || k == ctxItemDeleteChest || k == ctxItemDeleteDoor ||
		k == ctxItemDeleteCrystal || k == ctxItemDeleteLocation
}

// contextItemsAt builds the menu rows from what occupies (x,z) (pack/chest/door
// are mutually exclusive in practice).
func contextItemsAt(s *State, x, z int) []ctxItem {
	if !s.area.InBounds(x, z) {
		return nil
	}
	items := []ctxItem{}
	if core.PackSpawnIndexAt(s.area.PackSpawns, x, z) >= 0 {
		items = append(items,
			ctxItem{label: "Edit pack", kind: ctxItemEditPack},
			ctxItem{label: "Delete pack", kind: ctxItemDeletePack},
		)
	}
	if core.ChestSpawnIndexAt(s.area.ChestSpawns, x, z) >= 0 {
		items = append(items,
			ctxItem{label: "Edit chest", kind: ctxItemEditChest},
			ctxItem{label: "Delete chest", kind: ctxItemDeleteChest},
		)
	}
	if core.DoorSpawnIndexAt(s.area.DoorSpawns, x, z) >= 0 {
		items = append(items,
			ctxItem{label: "Edit door", kind: ctxItemEditDoor},
			ctxItem{label: "Delete door", kind: ctxItemDeleteDoor},
		)
	}
	if core.CrystalSpawnIndexAt(s.area.CrystalSpawns, x, z) >= 0 {
		// No per-instance data, so Delete only.
		items = append(items,
			ctxItem{label: "Delete crystal", kind: ctxItemDeleteCrystal},
		)
	}
	// Player-start tile: facing controls; else "Move start here" (move legality
	// enforced by runContextItem, which flashes why a blocked move was refused).
	if s.area.StartTileX == x && s.area.StartTileZ == z {
		// One row per facing, driven by core.FacingCount.
		for dir := 0; dir < int(core.FacingCount); dir++ {
			marker := "  "
			if s.area.StartFacing == dir {
				marker = "* "
			}
			items = append(items, ctxItem{
				label:  marker + "Face " + core.FacingShortLabels[dir],
				kind:   ctxItemStartFacing,
				facing: dir,
			})
		}
	} else {
		items = append(items,
			ctxItem{label: "Move start here", kind: ctxItemMoveStartHere},
		)
	}
	// Regions: edit/delete the one under the cursor (on the active level), and
	// always offer to create a new region anchored here.
	if idx := core.LocationIndexAt(s.area.Locations, x, z, s.editLevel); idx >= 0 {
		items = append(items,
			ctxItem{label: "Edit location: " + locationLabel(s.area.Locations[idx]), kind: ctxItemEditLocation},
			ctxItem{label: "Delete location", kind: ctxItemDeleteLocation},
		)
	}
	items = append(items, ctxItem{label: "New location here", kind: ctxItemNewLocation})
	// Wall-faces modal, only when the tile exposes a vertical face (same core
	// rule the renderer uses), so a flat tile doesn't offer a no-op row.
	if core.TileExposesFace(&s.area, x, z) {
		items = append(items, ctxItem{label: "Set wall faces…", kind: ctxItemSetWallFaces})
	}
	items = append(items, ctxItem{label: "Erase " + layerName(s.layer) + " here", kind: ctxItemEraseTile})
	return items
}

// openContextMenu pops the right-click menu at (clickX, clickY) over (tileX,
// tileZ). Items are rebuilt at open time; the dispatcher no-ops on a deleted target.
func openContextMenu(s *State, clickX, clickY float32, tileX, tileZ int) {
	items := contextItemsAt(s, tileX, tileZ)
	if len(items) == 0 {
		// Only reachable for an out-of-bounds tile (in-bounds always offers Erase).
		s.contextMenu = contextMenuState{}
		return
	}
	// Cancel any in-flight drag (updateContextMenu absorbs input until close, so
	// finishDrag never fires). Reset all three drag-index slots, not just pack.
	s.drag = dragNone
	s.dragPackIdx = -1
	s.dragChestIdx = -1
	s.dragDoorIdx = -1
	s.dragSnapshotDone = false
	s.contextMenu = contextMenuState{
		open:  true,
		x:     clickX,
		y:     clickY,
		tileX: tileX,
		tileZ: tileZ,
		items: items,
	}
}

func closeContextMenu(s *State) {
	s.contextMenu = contextMenuState{}
}

// Context-menu geometry. Cousin of the dropdown selector (shares dropdownPad);
// rows run taller/wider than the dropdown's for bigger click targets.
const (
	contextMenuRowH     = float32(28)
	contextMenuMinWidth = float32(180)
)

// contextMenuLayout returns the open menu's per-row rects + background rect,
// recomputed each frame so resizes/list edits reflow.
func contextMenuLayout(s *State) (rl.Rectangle, []rl.Rectangle) {
	if !s.contextMenu.open {
		return rl.Rectangle{}, nil
	}
	const rowH = contextMenuRowH
	const padding = dropdownPad // same gutter as the dropdown selector
	width := contextMenuMinWidth
	// Approximate label width via per-char average (no font handle here). Doesn't
	// share computeDropdownLayout's measure: the two surfaces size on different
	// fonts/pads, so a shared helper would change one's sizing.
	for i, it := range s.contextMenu.items {
		// Row 0 gets the tile-coord suffix at draw time; measure that widened
		// string (no scissor clips the menu). Must mirror drawContextMenu's row 0.
		label := it.label
		if i == 0 {
			label = fmt.Sprintf("%s  (%s)", label, core.TileCoord(s.contextMenu.tileX, s.contextMenu.tileZ))
		}
		w := approxTextWidth(label, editorFontLabel) + buttonLabelPadX
		if w > width {
			width = w
		}
	}
	height := padding*2 + float32(len(s.contextMenu.items))*rowH
	x := s.contextMenu.x
	y := s.contextMenu.y
	// Clamp within the window so an edge click doesn't push it off-screen.
	sw, sh := render.ScreenSizeF()
	if x+width > sw {
		x = sw - width
	}
	if y+height > sh {
		y = sh - height
	}
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	bg := rl.NewRectangle(x, y, width, height)
	rows := make([]rl.Rectangle, len(s.contextMenu.items))
	for i := range s.contextMenu.items {
		rows[i] = rl.NewRectangle(bg.X+padding, bg.Y+padding+float32(i)*rowH, bg.Width-2*padding, rowH)
	}
	return bg, rows
}

func drawContextMenu(s *State, font rl.Font, theme render.Theme) {
	if !s.contextMenu.open {
		return
	}
	bg, rows := contextMenuLayout(s)
	// Opaque backing first: theme.SurfacePrimary is a translucent HUD surface and the
	// map bleeds through it. Lay it over a solid editor-window tone so rows stay readable.
	rl.DrawRectangleRec(bg, bgWindow)
	rl.DrawRectangleRec(bg, theme.SurfacePrimary)
	rl.DrawRectangleLinesEx(bg, 1, theme.BorderStrong)
	mp := rl.GetMousePosition()
	for i, r := range rows {
		hovered := pointIn(mp, r)
		if hovered {
			rl.DrawRectangleRec(r, bgRowHover)
		}
		// Tile coord on row 0 confirms which cell the menu refers to.
		label := s.contextMenu.items[i].label
		if i == 0 {
			label = fmt.Sprintf("%s  (%s)", label, core.TileCoord(s.contextMenu.tileX, s.contextMenu.tileZ))
		}
		col := theme.TextPrimary
		if !hovered {
			col = theme.TextMuted
		}
		// Destructive rows read red.
		if s.contextMenu.items[i].kind.isDelete() {
			col = theme.BorderDanger
		}
		render.DrawRichText(font, label,
			rl.NewVector2(r.X+8, r.Y+(r.Height-editorFontLabel)/2),
			editorFontLabel, 1, col)
	}
}

// updateContextMenu drives the open menu: row-click fires, outside-click/Esc
// dismisses. Returns true when it absorbed the frame's input.
func updateContextMenu(s *State) bool {
	if !s.contextMenu.open {
		return false
	}
	if editorCancelPressed() {
		closeContextMenu(s)
		return true
	}
	mp := rl.GetMousePosition()
	if rl.IsMouseButtonPressed(rl.MouseLeftButton) || rl.IsMouseButtonPressed(rl.MouseRightButton) {
		_, rows := contextMenuLayout(s)
		for i, r := range rows {
			if pointIn(mp, r) {
				runContextItem(s, s.contextMenu.items[i])
				closeContextMenu(s)
				return true
			}
		}
		// Click outside — dismiss without firing.
		closeContextMenu(s)
		return true
	}
	return true
}

func runContextItem(s *State, item ctxItem) {
	kind := item.kind
	x, z := s.contextMenu.tileX, s.contextMenu.tileZ
	// Map may have shrunk since the menu was built — reject stale coords.
	if !s.area.InBounds(x, z) {
		return
	}
	switch kind {
	case ctxItemEditPack:
		if idx := core.PackSpawnIndexAt(s.area.PackSpawns, x, z); idx >= 0 {
			openPackEditModal(s, idx)
		}
	case ctxItemDeletePack:
		deleteSpawnAt(s, x, z, "pack", func() bool {
			before := len(s.area.PackSpawns)
			s.area.PackSpawns = removePackAt(s.area.PackSpawns, x, z)
			return len(s.area.PackSpawns) != before
		})
	case ctxItemEditChest:
		if idx := core.ChestSpawnIndexAt(s.area.ChestSpawns, x, z); idx >= 0 {
			openChestEditModal(s, idx)
		}
	case ctxItemDeleteChest:
		deleteSpawnAt(s, x, z, "chest", func() bool {
			before := len(s.area.ChestSpawns)
			s.area.ChestSpawns = removeChestSpawnAt(s.area.ChestSpawns, x, z)
			return len(s.area.ChestSpawns) != before
		})
	case ctxItemEditDoor:
		if idx := core.DoorSpawnIndexAt(s.area.DoorSpawns, x, z); idx >= 0 {
			openDoorEditModal(s, idx)
		}
	case ctxItemDeleteDoor:
		deleteSpawnAt(s, x, z, "door", func() bool {
			before := len(s.area.DoorSpawns)
			s.area.DoorSpawns = removeDoorAt(s.area.DoorSpawns, x, z)
			return len(s.area.DoorSpawns) != before
		})
	case ctxItemDeleteCrystal:
		deleteSpawnAt(s, x, z, "crystal", func() bool {
			before := len(s.area.CrystalSpawns)
			s.area.CrystalSpawns = removeCrystalSpawnAt(s.area.CrystalSpawns, x, z)
			return len(s.area.CrystalSpawns) != before
		})
	case ctxItemMoveStartHere:
		// Shared startBlockers so this and the entity-brush start tool can't drift.
		if msg := firstBlocker(startBlockers(&s.area, x, z)...); msg != "" {
			s.flash(msg)
			return
		}
		pushUndo(s)
		s.area.StartTileX = x
		s.area.StartTileZ = z
		s.dirty = true
	case ctxItemStartFacing:
		setStartFacing(s, item.facing)
	case ctxItemNewLocation:
		createLocationAt(s, x, z)
		return
	case ctxItemEditLocation:
		if idx := core.LocationIndexAt(s.area.Locations, x, z, s.editLevel); idx >= 0 {
			openLocationEditModal(s, idx)
		}
		return
	case ctxItemDeleteLocation:
		deleteSpawnAt(s, x, z, "location", func() bool {
			idx := core.LocationIndexAt(s.area.Locations, x, z, s.editLevel)
			if idx < 0 {
				return false
			}
			s.area.Locations = append(s.area.Locations[:idx], s.area.Locations[idx+1:]...)
			return true
		})
	case ctxItemSetWallFaces:
		openWallFacesModal(s, x, z)
		return
	case ctxItemEraseTile:
		// Snapshot, commit only if changed — a no-op erase banks no undo.
		before := core.CloneArea(s.area)
		wasDirty := s.dirty
		eraseAt(s, x, z)
		if core.AreaContentEqual(s.area, before) {
			s.dirty = wasDirty // eraseAt flips dirty unconditionally — undo a no-op
		} else {
			commitUndoSnapshot(s, before)
		}
	default:
		// Fail closed so a new kind's menu row can't silently no-op.
		panic(fmt.Sprintf("editor: runContextItem has no case for ctxItemKind %d", int(kind)))
	}
}

// deleteSpawnAt is the shared delete protocol for every context-menu spawn kind:
// snapshot, run the kind-specific remove (closure reports whether the slice
// shrank), and on a real removal mark dirty + flash. Adding a deletable spawn is
// one closure, not another copy of this bookkeeping.
func deleteSpawnAt(s *State, x, z int, noun string, remove func() (changed bool)) {
	// Commit undo only if remove actually shrank the slice — a no-op delete
	// shouldn't bank an empty undo or wipe redo.
	before := core.CloneArea(s.area)
	if remove() {
		commitUndoSnapshot(s, before)
		s.dirty = true
		s.flash("Deleted " + noun + " at " + core.TileCoord(x, z))
	}
}

// setStartFacing writes StartFacing. No-op on no change (no undo/dirty churn).
func setStartFacing(s *State, dir int) {
	if s.area.StartFacing == dir {
		return
	}
	pushUndo(s)
	s.area.StartFacing = dir
	s.dirty = true
}

package editor

import (
	"crawler/internal/app/core"
	"fmt"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// context.go builds the grid right-click menu. It's a dropdown owner (ddContext):
// layout, drawing, hover, screen-clamp, and dismiss all live in dropdown.go — this
// file only supplies the per-tile rows. Each row carries its own action (apply),
// so adding a row is one entry here, no parallel enum + dispatch switch.

// contextMenuRowH gives the right-click menu taller rows than the stock dropdown
// (dropdownRowH) for bigger click targets — passed to openDropdownAt at open time.
const contextMenuRowH = float32(28)

// contextSpawnKind is one deletable/editable tile occupant (pack/chest/door/crystal).
// indexAt/openEdit/remove close over the kind's concrete spawn slice so a new spawn
// kind is one table row, not another copy of the Edit/Delete block below.
type contextSpawnKind struct {
	noun     string
	indexAt  func(s *State, x, z int) int
	openEdit func(s *State, idx int)
	remove   func(s *State, x, z int) bool
}

var contextSpawnKinds = []contextSpawnKind{
	{"pack", func(s *State, x, z int) int { return core.PackSpawnIndexAt(s.area.PackSpawns, x, z) },
		openPackEditModal, func(s *State, x, z int) bool { return deleteSpawnSlice(&s.area.PackSpawns, x, z) }},
	{"chest", func(s *State, x, z int) int { return core.ChestSpawnIndexAt(s.area.ChestSpawns, x, z) },
		openChestEditModal, func(s *State, x, z int) bool { return deleteSpawnSlice(&s.area.ChestSpawns, x, z) }},
	{"door", func(s *State, x, z int) int { return core.DoorSpawnIndexAt(s.area.DoorSpawns, x, z) },
		openDoorEditModal, func(s *State, x, z int) bool { return deleteSpawnSlice(&s.area.DoorSpawns, x, z) }},
	{"crystal", func(s *State, x, z int) int { return core.CrystalSpawnIndexAt(s.area.CrystalSpawns, x, z) },
		openCrystalEditModal, func(s *State, x, z int) bool { return deleteSpawnSlice(&s.area.CrystalSpawns, x, z) }},
}

// contextItemsAt builds the menu rows from what occupies (x,z) (pack/chest/door
// are mutually exclusive in practice). danger rows draw red.
func contextItemsAt(s *State, x, z int) []dropdownEntry {
	if !s.area.InBounds(x, z) {
		return nil
	}
	items := []dropdownEntry{}
	for _, k := range contextSpawnKinds {
		if k.indexAt(s, x, z) < 0 {
			continue
		}
		items = append(items,
			dropdownEntry{label: "Edit " + k.noun, apply: func(s *State) {
				if idx := k.indexAt(s, x, z); idx >= 0 {
					k.openEdit(s, idx)
				}
			}},
			dropdownEntry{label: "Delete " + k.noun, danger: true, apply: func(s *State) {
				deleteSpawnAt(s, x, z, k.noun, func() bool { return k.remove(s, x, z) })
			}},
		)
	}
	// Player-start tile: facing controls; else "Move start here" (move legality
	// enforced at apply time, which flashes why a blocked move was refused).
	if s.area.StartTileX == x && s.area.StartTileZ == z {
		// One row per facing, driven by core.FacingCount.
		for dir := 0; dir < int(core.FacingCount); dir++ {
			marker := "  "
			if s.area.StartFacing == dir {
				marker = "* "
			}
			items = append(items, dropdownEntry{
				label: marker + "Face " + core.FacingShortLabels[dir],
				apply: func(s *State) { setStartFacing(s, dir) },
			})
		}
	} else {
		items = append(items, dropdownEntry{label: "Move start here", apply: func(s *State) {
			// moveStartTo no longer banks its own undo — wrap so this direct path gets one.
			commitPaintIfChanged(s, func() { moveStartTo(s, x, z) })
		}})
	}
	// Regions: edit/delete the one under the cursor (on the active level), and
	// always offer to create a new region anchored here.
	if idx := core.LocationIndexAt(s.area.Locations, x, z, s.editLevel); idx >= 0 {
		items = append(items,
			dropdownEntry{label: "Edit location: " + locationLabel(s.area.Locations[idx]), apply: func(s *State) {
				if idx := core.LocationIndexAt(s.area.Locations, x, z, s.editLevel); idx >= 0 {
					openLocationEditModal(s, idx)
				}
			}},
			dropdownEntry{label: "Delete location", danger: true, apply: func(s *State) {
				deleteSpawnAt(s, x, z, "location", func() bool {
					idx := core.LocationIndexAt(s.area.Locations, x, z, s.editLevel)
					if idx < 0 {
						return false
					}
					// Fresh-slice removal, not in-place append-shift, to avoid mutating a
					// backing array an undo snapshot could alias (see removeModalListItem).
					s.area.Locations = removeModalListItem(s.area.Locations, idx)
					return true
				})
			}},
		)
	}
	items = append(items, dropdownEntry{label: "New location here", apply: func(s *State) { createLocationAt(s, x, z) }})
	// Wall-faces modal, only when the tile exposes a vertical face (same core
	// rule the renderer uses), so a flat tile doesn't offer a no-op row.
	if core.TileExposesFace(&s.area, x, z) {
		items = append(items, dropdownEntry{label: "Set wall faces…", apply: func(s *State) { openWallFacesModal(s, x, z) }})
	}
	// Wall feature (switch / bombable / secret): edit the one here, else place a new one.
	if idx := core.WallFeatureAnyAt(s.area.WallFeatures, x, z); idx >= 0 {
		items = append(items,
			dropdownEntry{label: "Edit wall feature…", apply: func(s *State) {
				if idx := core.WallFeatureAnyAt(s.area.WallFeatures, x, z); idx >= 0 {
					openWallFeatureEditModal(s, idx)
				}
			}},
			dropdownEntry{label: "Delete wall feature", danger: true, apply: func(s *State) {
				if idx := core.WallFeatureAnyAt(s.area.WallFeatures, x, z); idx >= 0 {
					pushUndo(s)
					s.area.WallFeatures = removeModalListItem(s.area.WallFeatures, idx)
					s.dirty = true
				}
			}},
		)
	} else {
		items = append(items, dropdownEntry{label: "Add wall feature…", apply: func(s *State) { addWallFeatureAt(s, x, z) }})
	}
	items = append(items, dropdownEntry{label: "Erase " + layerName(s.layer) + " here", apply: func(s *State) {
		// Commit only if changed — a no-op erase banks no undo (shared lazy-commit tail).
		commitPaintIfChanged(s, func() { eraseAt(s, x, z) })
	}})
	return items
}

// contextEntries is the ddContext dropdown builder: the rows for the tile the menu
// was opened over (ctxTile*), rebuilt each frame so live state (start facing, a
// deleted target) reflects. Row 0 carries the tile-coord suffix; every action is
// guarded against a tile that fell out of bounds since open (the map may shrink).
func contextEntries(s *State) []dropdownEntry {
	tileX, tileZ := s.ctxTileX, s.ctxTileZ
	entries := contextItemsAt(s, tileX, tileZ)
	if len(entries) == 0 {
		return nil
	}
	entries[0].label = fmt.Sprintf("%s  (%s)", entries[0].label, core.TileCoord(tileX, tileZ))
	for i := range entries {
		inner := entries[i].apply
		entries[i].apply = func(s *State) {
			if !s.area.InBounds(tileX, tileZ) {
				return
			}
			inner(s)
		}
	}
	return entries
}

// openContextMenu pops the right-click menu at (clickX, clickY) over (tileX,
// tileZ). Rows are rebuilt each frame by contextEntries; the dispatcher no-ops on
// a deleted target.
func openContextMenu(s *State, clickX, clickY float32, tileX, tileZ int) {
	s.ctxTileX, s.ctxTileZ = tileX, tileZ
	if len(contextItemsAt(s, tileX, tileZ)) == 0 {
		// Only reachable for an out-of-bounds tile (in-bounds always offers Erase).
		closeDropdown(s)
		return
	}
	// Cancel any in-flight drag (the open menu absorbs input until close, so
	// finishDrag never fires). Reset all three drag-index slots, not just pack.
	s.drag = dragNone
	s.dragPackIdx = -1
	s.dragChestIdx = -1
	s.dragDoorIdx = -1
	s.dragSnapshotDone = false
	openDropdownAt(s, ddContext, rl.NewVector2(clickX, clickY), contextMenuRowH)
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
	setIfChanged(s, &s.area.StartFacing, dir)
}

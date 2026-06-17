package editor

import (
	"crawler/internal/app/core"
	"crawler/internal/app/render"
	"fmt"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// ctxItemKind enumerates the actions the right-click context menu can
// fire. Each kind is a single discrete action the dispatcher in
// runContextItem applies against (s.contextMenu.tileX, tileZ); kinds
// added here also need a row in runContextItem's switch so the menu
// stays in lockstep with the actions it can perform.
type ctxItemKind int

const (
	ctxItemNone ctxItemKind = iota
	ctxItemEditPack
	ctxItemDeletePack
	ctxItemEditChest
	ctxItemDeleteChest
	ctxItemEditDoor
	ctxItemDeleteDoor
	// ctxItemDeleteCrystal removes the crystal at the right-clicked tile.
	// There's no edit counterpart — crystals carry no per-instance data.
	ctxItemDeleteCrystal
	ctxItemMoveStartHere
	// ctxItemStartFacing sets the PlayerStart instance's facing (stored as
	// AreaDefinition.StartFacing). This is the fallback facing for initial
	// spawn — per-door Facing on each DoorSpawn overrides it when the player
	// arrives via a door. Surfaced only in the right-click menu on the start
	// tile; the sidebar no longer exposes it since "where the player faces on
	// spawn" is an instance attribute, not an area-wide setting. The specific
	// facing (core.North/East/South/West) rides in ctxItem.facing — one kind,
	// four rows — mirroring how doorEdit carries its facing as a payload
	// rather than enumerating a kind per direction.
	ctxItemStartFacing
)

// ctxItem is one row in the right-click context menu. Built fresh by
// contextItemsAt for whatever sits at the right-clicked tile so the
// menu reads "Edit chest" instead of a generic "Edit" when a chest is
// the only thing there.
type ctxItem struct {
	label string
	kind  ctxItemKind
	// facing is the payload for ctxItemStartFacing (core.North/East/South/West);
	// ignored by every other kind. Lets one facing kind cover all four rows
	// instead of a kind per direction.
	facing int
}

// contextMenuState is the in-State data for an open right-click menu.
// Empty (open=false) when no menu is up. Recomputed when the user
// opens a new menu, dismissed on click-outside / Esc / item-pick.
type contextMenuState struct {
	open         bool
	x, y         float32
	tileX, tileZ int
	items        []ctxItem
}

// isDelete reports whether a row is a destructive delete (drawn red so it can't
// be mistaken for the Edit row sitting right above it).
func (k ctxItemKind) isDelete() bool {
	return k == ctxItemDeletePack || k == ctxItemDeleteChest || k == ctxItemDeleteDoor || k == ctxItemDeleteCrystal
}

// contextItemsAt builds the menu's row list based on what occupies
// (x,z). Multi-entity tiles (currently impossible — placement rules
// already prevent overlap) would list all applicable rows; today the
// dispatch is mutually exclusive (pack OR chest OR door).
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
		// Crystals have no edit modal (no per-instance data), so only Delete.
		items = append(items,
			ctxItem{label: "Delete crystal", kind: ctxItemDeleteCrystal},
		)
	}
	// Player-start tile: surface the facing controls here (the sidebar
	// no longer carries them — facing is an instance attribute of this
	// PlayerStart). Otherwise, offer "Move start here" so the author
	// can relocate the start instance with one right-click. The actual
	// movement rules (no walls / props / deep water) are enforced by
	// runContextItem; surfacing the row regardless lets the flash
	// error explain why it didn't take.
	if s.area.StartTileX == x && s.area.StartTileZ == z {
		// One row per facing, driven by core.FacingCount (mirrors the door-edit
		// modal's facing loop) so a future facing scales the menu instead of
		// needing a hand-added row + enum kind.
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
	return items
}

// openContextMenu pops the right-click menu at (clickX, clickY) over
// the tile (tileX, tileZ). Items are rebuilt from the tile's contents
// at open time; if the underlying entity is deleted before the menu is
// dismissed, the dispatcher gracefully no-ops on the now-missing
// target.
func openContextMenu(s *State, clickX, clickY float32, tileX, tileZ int) {
	items := contextItemsAt(s, tileX, tileZ)
	if len(items) == 0 {
		// No actionable contents — fall back to a silent close so the
		// user doesn't see a one-row "nothing here" placeholder menu.
		s.contextMenu = contextMenuState{}
		return
	}
	// Cancel any in-flight left-button drag so a right-click that opens
	// this menu mid-drag doesn't leave stale drag state. updateContextMenu
	// absorbs all subsequent input until the menu closes — finishDrag would
	// never get a chance to fire on its own, so reset ALL three drag-index
	// slots (pack/chest/door), not just the pack one.
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

// contextMenuLayout returns the per-row rectangles + the full background
// rectangle of the open menu. Recomputed each frame so a screen resize
// or item-list edit (kinds rebuilt) reflows naturally.
func contextMenuLayout(s *State) (rl.Rectangle, []rl.Rectangle) {
	if !s.contextMenu.open {
		return rl.Rectangle{}, nil
	}
	const rowH = float32(28)
	const padding = float32(6)
	width := float32(180)
	// Measure widest label so a long "Move start here" doesn't get clipped.
	// MeasureTextEx with the default font would force the renderer to
	// reach for it; the editor draws with the loaded font handed into
	// drawContextMenu — we approximate width via a per-char average so
	// the layout pass doesn't need the font handle here.
	for _, it := range s.contextMenu.items {
		w := approxTextWidth(it.label, editorFontLabel) + buttonLabelPadX
		if w > width {
			width = w
		}
	}
	height := padding*2 + float32(len(s.contextMenu.items))*rowH
	x := s.contextMenu.x
	y := s.contextMenu.y
	// Clamp the menu within the window so a click near the edge doesn't
	// push the menu off-screen.
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
	rl.DrawRectangleRec(bg, theme.SurfacePrimary)
	rl.DrawRectangleLinesEx(bg, 1, theme.BorderStrong)
	mp := rl.GetMousePosition()
	for i, r := range rows {
		hovered := pointIn(mp, r)
		if hovered {
			rl.DrawRectangleRec(r, bgRowHover)
		}
		// Show the tile coord on the first row so the author can confirm
		// the menu refers to the cell they intended — useful when the
		// click lands near a grid edge.
		label := s.contextMenu.items[i].label
		if i == 0 {
			label = fmt.Sprintf("%s  (%s)", label, core.TileCoord(s.contextMenu.tileX, s.contextMenu.tileZ))
		}
		col := theme.TextPrimary
		if !hovered {
			col = theme.TextMuted
		}
		// Destructive rows read red so "Delete pack" can't be misclicked for the
		// "Edit pack" row directly above it.
		if s.contextMenu.items[i].kind.isDelete() {
			col = theme.BorderDanger
		}
		render.DrawRichText(font, label,
			rl.NewVector2(r.X+8, r.Y+(r.Height-editorFontLabel)/2),
			editorFontLabel, 1, col)
	}
}

// updateContextMenu drives the context menu while it's open: click on
// a row fires it, click outside or Esc dismisses. Returns true when
// the menu absorbed the frame's input so the normal grid/paint paths
// in updateMouse can skip themselves.
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
		// Click outside the menu — dismiss without firing anything.
		closeContextMenu(s)
		return true
	}
	return true
}

func runContextItem(s *State, item ctxItem) {
	kind := item.kind
	x, z := s.contextMenu.tileX, s.contextMenu.tileZ
	// The menu was built against an earlier snapshot of the area. If the
	// map shrank in between (via the sidebar dim −/+ buttons or a numeric
	// commit), the captured coords may be out of bounds — reject before
	// touching any layer array.
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
		// Shared player-start rule set (see ops.startBlockers) so this path
		// and the entity-brush start tool can't drift on legality or wording.
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
	default:
		// Every ctxItemKind needs a case here (see the kind enum's doc).
		// Fail closed like the layer switches (applyTool / activeGrid)
		// rather than letting a new kind's menu row silently no-op.
		panic(fmt.Sprintf("editor: runContextItem has no case for ctxItemKind %d", int(kind)))
	}
}

// deleteSpawnAt is the shared delete protocol for every entity-spawn kind the
// context menu removes (pack / chest / door / crystal): snapshot for undo, run
// the kind-specific remove (passed as a closure that reports whether the slice
// actually shrank), and on a real removal mark dirty + flash "Deleted <noun> at
// <tile>". Centralizing it means the undo/dirty/flash bookkeeping can't drift
// between the kinds — adding a new deletable spawn is one closure, not another
// copy of this block.
func deleteSpawnAt(s *State, x, z int, noun string, remove func() (changed bool)) {
	// Snapshot first, commit the undo only if the remove actually shrank the
	// slice — a no-op delete (the tile's entity vanished between menu-open and
	// click) shouldn't bank an empty undo or wipe the redo stack. Mirrors
	// keyboardMutate's capture-then-commit-if-changed protocol.
	before := core.CloneArea(s.area)
	if remove() {
		commitUndoSnapshot(s, before)
		s.dirty = true
		s.flash("Deleted " + noun + " at " + core.TileCoord(x, z))
	}
}

// setStartFacing writes the PlayerStart instance's facing. No-op when
// the value didn't change so identical clicks don't pollute the undo
// stack or trip the dirty flag.
func setStartFacing(s *State, dir int) {
	if s.area.StartFacing == dir {
		return
	}
	pushUndo(s)
	s.area.StartFacing = dir
	s.dirty = true
}

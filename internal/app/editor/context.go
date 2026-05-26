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
	ctxItemMoveStartHere
	// ctxItemStartFacing{N,E,S,W} set the PlayerStart instance's facing
	// (stored as AreaDefinition.StartFacing). This is the fallback
	// facing for initial spawn — per-door Facing on each DoorSpawn
	// overrides it when the player arrives via a door. Surfaced only
	// in the right-click menu on the start tile; the sidebar no longer
	// exposes it since "where the player faces on spawn" is an instance
	// attribute, not an area-wide setting.
	ctxItemStartFacingN
	ctxItemStartFacingE
	ctxItemStartFacingS
	ctxItemStartFacingW
)

// ctxItem is one row in the right-click context menu. Built fresh by
// contextItemsAt for whatever sits at the right-clicked tile so the
// menu reads "Edit chest" instead of a generic "Edit" when a chest is
// the only thing there.
type ctxItem struct {
	label string
	kind  ctxItemKind
}

// contextMenuState is the in-State data for an open right-click menu.
// Empty (open=false) when no menu is up. Recomputed when the user
// opens a new menu, dismissed on click-outside / Esc / item-pick.
type contextMenuState struct {
	open         bool
	x, y         float32
	tileX, tileZ int
	items        []ctxItem
	hoverIdx     int
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
	// Player-start tile: surface the facing controls here (the sidebar
	// no longer carries them — facing is an instance attribute of this
	// PlayerStart). Otherwise, offer "Move start here" so the author
	// can relocate the start instance with one right-click. The actual
	// movement rules (no walls / props / deep water) are enforced by
	// runContextItem; surfacing the row regardless lets the flash
	// error explain why it didn't take.
	if s.area.StartTileX == x && s.area.StartTileZ == z {
		facingLabel := func(dir int) string {
			marker := "  "
			if s.area.StartFacing == dir {
				marker = "* "
			}
			return marker + "Face " + core.FacingShortLabels[dir]
		}
		items = append(items,
			ctxItem{label: facingLabel(core.North), kind: ctxItemStartFacingN},
			ctxItem{label: facingLabel(core.East), kind: ctxItemStartFacingE},
			ctxItem{label: facingLabel(core.South), kind: ctxItemStartFacingS},
			ctxItem{label: facingLabel(core.West), kind: ctxItemStartFacingW},
		)
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
	// this menu mid-drag doesn't leave stale s.drag / dragPackIdx state.
	// updateContextMenu absorbs all subsequent input until the menu
	// closes — finishDrag would never get a chance to fire on its own.
	s.drag = dragNone
	s.dragPackIdx = -1
	s.dragSnapshotDone = false
	s.contextMenu = contextMenuState{
		open:     true,
		x:        clickX,
		y:        clickY,
		tileX:    tileX,
		tileZ:    tileZ,
		items:    items,
		hoverIdx: -1,
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
		w := float32(len(it.label))*9 + 28
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
		rl.DrawTextEx(font, label,
			rl.NewVector2(r.X+8, r.Y+(r.Height-14)/2),
			14, 1, col)
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
				runContextItem(s, s.contextMenu.items[i].kind)
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

func runContextItem(s *State, kind ctxItemKind) {
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
			s.modal = modalPackEdit
			s.modalPackIdx = idx
			s.modalCursor = 0
		}
	case ctxItemDeletePack:
		pushUndo(s)
		before := len(s.area.PackSpawns)
		s.area.PackSpawns = removePackAt(s.area.PackSpawns, x, z)
		if len(s.area.PackSpawns) != before {
			s.dirty = true
		}
	case ctxItemEditChest:
		if idx := core.ChestSpawnIndexAt(s.area.ChestSpawns, x, z); idx >= 0 {
			s.modal = modalChestEdit
			s.modalChestIdx = idx
			s.modalCursor = 0
		}
	case ctxItemDeleteChest:
		pushUndo(s)
		before := len(s.area.ChestSpawns)
		s.area.ChestSpawns = removeChestSpawnAt(s.area.ChestSpawns, x, z)
		if len(s.area.ChestSpawns) != before {
			s.dirty = true
		}
	case ctxItemEditDoor:
		if idx := core.DoorSpawnIndexAt(s.area.DoorSpawns, x, z); idx >= 0 {
			openDoorEditModal(s, idx)
		}
	case ctxItemDeleteDoor:
		pushUndo(s)
		before := len(s.area.DoorSpawns)
		s.area.DoorSpawns = removeDoorAt(s.area.DoorSpawns, x, z)
		if len(s.area.DoorSpawns) != before {
			s.dirty = true
		}
	case ctxItemMoveStartHere:
		if s.area.Walls[z][x] == core.TileRock {
			s.flash("Player start needs an open cell")
			return
		}
		if core.IsPropChar(s.area.Props[z][x]) {
			s.flash("Cell is occupied by a prop")
			return
		}
		if s.area.Floor[z][x] == core.FloorDeepWater {
			s.flash("Player start can't sit on deep water")
			return
		}
		pushUndo(s)
		s.area.StartTileX = x
		s.area.StartTileZ = z
		s.dirty = true
	case ctxItemStartFacingN:
		setStartFacing(s, core.North)
	case ctxItemStartFacingE:
		setStartFacing(s, core.East)
	case ctxItemStartFacingS:
		setStartFacing(s, core.South)
	case ctxItemStartFacingW:
		setStartFacing(s, core.West)
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

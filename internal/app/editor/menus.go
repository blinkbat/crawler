package editor

import (
	"crawler/internal/app/input"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// menus.go groups the editor's top-level commands into FIVE pull-down menus
// (File / Edit / View / Assets / Map) so the menu bar shows five labels instead
// of a flat wall of ~10 buttons, and the second row keeps only the paint tools +
// their contextual controls. Each menu opens the shared dropdown widget (ddMenu
// owner — see dropdown.go), so a menu row IS a dropdownEntry: same label / apply
// / hotkey / desc / enabled / active fields any picker row has. Adding a command
// is one row here — no button wiring, no layout math.
//
// What lives WHERE, and why: modal-openers and view toggles (rare, or
// set-and-forget) belong in menus; the things you do constantly WHILE painting
// (tool select, undo/redo, brush size, the elevation cluster) stay on the
// toolbar row where the hand expects them (see toolbarActionBtns in draw.go).

type menuGroup struct {
	label string
	items []dropdownEntry
}

// editorMenus is the menu-bar model — order is left-to-right on the bar. The
// hotkey strings are display-only mirrors of the accelerators in updateHotkeys;
// the action they name is the same handler the key calls.
var editorMenus = []menuGroup{
	{label: "File", items: []dropdownEntry{
		{label: "New Map", apply: newMap, desc: "Start a fresh blank map (prompts for size)."},
		{label: "Open…", apply: requestOpen, hotkey: "Ctrl+O", desc: "Load a map from disk."},
		{label: "Save", apply: saveCurrent, hotkey: "Ctrl+S", desc: "Save to the current file."},
		{label: "Save As…", apply: openSaveAsModal, desc: "Save under a new file name."},
		{label: "Exit Editor", apply: func(s *State) { s.exitRequested = true }, desc: "Leave the editor (asks first if there are unsaved changes)."},
	}},
	{label: "Edit", items: []dropdownEntry{
		{label: "Undo", apply: undoOne, hotkey: "Ctrl+Z", desc: "Step back one change.", enabled: func(s *State) bool { return len(s.undo) > 0 }},
		{label: "Redo", apply: redoOne, hotkey: "Ctrl+Y", desc: "Re-apply the last undone change.", enabled: func(s *State) bool { return len(s.redo) > 0 }},
		{label: "Fill Layer", apply: fillEntireLayer, hotkey: "Ctrl+Shift+F", desc: "Flood the whole active layer with the current brush.", enabled: onGridLayer},
	}},
	{label: "View", items: []dropdownEntry{
		{label: "Center on Start", apply: func(s *State) { centerViewOnTile(s, s.area.StartTileX, s.area.StartTileZ) }, desc: "Scroll the canvas to the player start tile."},
		{label: "Reset View", apply: resetView, desc: "Reset zoom to 100% and re-center the canvas."},
		{label: "Tile Glyphs", apply: toggleTileGlyphs, desc: "Overlay each tile's letter code on the canvas.", active: func(s *State) bool { return s.showTileGlyphs }},
		{label: "Door Links", apply: func(s *State) { s.showDoorLinks = !s.showDoorLinks }, desc: "Draw lines connecting linked doors.", active: func(s *State) bool { return s.showDoorLinks }},
		{label: "Cycle Day Phase", apply: cyclePreviewPhase, hotkey: "T", desc: "Preview the map lit at the next time of day."},
	}},
	{label: "Assets", items: []dropdownEntry{
		{label: "Sounds…", apply: openSoundsModal, desc: "Create sound effects and bind them to game cues."},
		{label: "Custom Enemies…", apply: openCustomEnemiesModal, desc: "Author map-specific enemy templates."},
		{label: "Foe Visuals…", apply: openFoeViewModal, desc: "Tune a foe's sprite, placement, and tint — or import a PNG."},
		{label: "Hit Glyphs…", apply: openHitGlyphsModal, desc: "Preview the combat hit symbols (slash, impact, frost, …)."},
		{label: "Object List…", apply: openEntityListModal, desc: "Jump to any pack, chest, or door on the map."},
	}},
	{label: "Map", items: []dropdownEntry{
		{label: "Validate", apply: openValidateModal, desc: "Check the map for reachability and setup problems."},
		{label: "Playtest", apply: func(s *State) { s.testRequested = true }, desc: "Launch the map in-game from its start tile."},
	}},
}

// menuEntries returns the rows of the currently-open menu (ddMenu owner), or nil
// when the open-menu index is somehow out of range.
func menuEntries(s *State) []dropdownEntry {
	if s.dropdown.menu < 0 || s.dropdown.menu >= len(editorMenus) {
		return nil
	}
	return editorMenus[s.dropdown.menu].items
}

// menuBarBtns is the top-row strip: one button per menu group, each opening its
// pull-down. Built once from editorMenus so adding a menu needs no extra wiring;
// active() lights the label of the menu that's currently open.
var menuBarBtns []topbarBtn

func init() {
	for i := range editorMenus {
		i := i
		menuBarBtns = append(menuBarBtns, topbarBtn{
			label:  editorMenus[i].label,
			action: func(s *State) { openMenu(s, i) },
			active: func(s *State) bool { return s.dropdown.owner == ddMenu && s.dropdown.menu == i },
		})
	}
}

// menuAnchorRect is the on-bar rect of menu i — the anchor its pull-down drops
// from. Walks the same widths drawButtonStrip uses so the list lines up under
// its label.
func menuAnchorRect(i int) rl.Rectangle {
	x := buttonStripStartX
	for j := 0; j < i && j < len(menuBarBtns); j++ {
		x += buttonWidth(menuBarBtns[j].label) + tightBtnGap
	}
	return rl.NewRectangle(x, 6, buttonWidth(menuBarBtns[i].label), topbarH-12)
}

// openMenu opens menu i's pull-down — or closes it if it's already the open one,
// so clicking a label toggles it. The list drops DOWN from the menu-bar label.
func openMenu(s *State, i int) {
	if i < 0 || i >= len(editorMenus) {
		return
	}
	if s.dropdown.owner == ddMenu && s.dropdown.menu == i {
		closeDropdown(s)
		return
	}
	s.dropdown = dropdownState{owner: ddMenu, menu: i, anchor: menuAnchorRect(i), growDown: true}
	input.ResetStickEdges()
}

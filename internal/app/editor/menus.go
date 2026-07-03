package editor

import (
	"crawler/internal/app/input"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// menus.go: top-level commands in five pull-down menus (File/Edit/View/Assets/
// Map). Each opens the shared dropdown (ddMenu owner); a row IS a dropdownEntry.
// Constant-while-painting controls stay on the toolbar (draw.go) instead.

type menuGroup struct {
	label string
	items []dropdownEntry
}

// editorMenus is the menu-bar model (left-to-right); hotkey strings mirror updateHotkeys.
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
		{label: "Isometric View", apply: func(s *State) { setIsoView(s, true) }, hotkey: "I", desc: "3D block view showing elevation (default). I toggles.", active: func(s *State) bool { return s.isoView }},
		{label: "Top Down View", apply: func(s *State) { setIsoView(s, false) }, desc: "Flat top-down grid view.", active: func(s *State) bool { return !s.isoView }},
		{label: "Object Animation", apply: func(s *State) { s.animateObjects = !s.animateObjects }, desc: "Animate foliage sway & torch flicker in 3D (off = still, faster).", active: func(s *State) bool { return s.animateObjects }},
		{label: "Tile Glyphs", apply: toggleTileGlyphs, desc: "Overlay each tile's letter code on the canvas.", active: func(s *State) bool { return s.showTileGlyphs }},
		{label: "Door Links", apply: func(s *State) { s.showDoorLinks = !s.showDoorLinks }, desc: "Draw lines connecting linked doors.", active: func(s *State) bool { return s.showDoorLinks }},
		{label: "Cycle Day Phase", apply: cyclePreviewPhase, hotkey: "T", desc: "Preview the map lit at the next time of day."},
	}},
	{label: "Assets", items: []dropdownEntry{
		{label: "Sounds…", apply: openSoundsModal, desc: "Create sound effects and bind them to game cues."},
		{label: "Foe Visuals…", apply: openFoeViewModal, desc: "Tune a foe's sprite, placement, and tint — or import a PNG."},
		{label: "Party Visuals…", apply: openPartyViewModal, desc: "Tune a party class's sprite, placement, and tint — or import a PNG."},
		{label: "Hit Glyphs…", apply: openHitGlyphsModal, desc: "Preview the combat hit symbols (slash, impact, frost, …)."},
		{label: "Object Browser…", apply: openObjectViewModal, desc: "Spot-check every decor & prop as live 3D thumbnails."},
		{label: "Object List…", apply: openEntityListModal, desc: "Jump to any pack, chest, or door on the map."},
	}},
	{label: "Map", items: []dropdownEntry{
		{label: "Dialogs…", apply: openDialogListModal, desc: "Author the area's branching conversations."},
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

// menuBarBtns is the top-row strip, one button per menu group (built from editorMenus).
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

// menuAnchorRect is the on-bar rect menu i's pull-down drops from — via the shared
// stripButtonRect so it uses the SAME per-button widths (honoring any width override)
// the bar draws with, not a label-only recompute that would misplace the dropdown.
func menuAnchorRect(i int) rl.Rectangle {
	return stripButtonRect(menuBarBtns, i, menuBarBtnY, menuBarBtnH)
}

// openMenu toggles menu i's pull-down (drops down from the bar label).
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

package editor

import (
	"crawler/internal/app/core"
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
		{label: "New Map", apply: newMap, hotkey: "Ctrl+N", desc: "Start a fresh blank map (prompts for size)."},
		{label: "Open…", apply: requestOpen, hotkey: "Ctrl+O", desc: "Load a map from disk."},
		{label: "Save", apply: saveCurrent, hotkey: "Ctrl+S", desc: "Save to the current file."},
		{label: "Save As…", apply: openSaveAsModal, hotkey: "Ctrl+Shift+S", desc: "Save under a new file name."},
		{label: "Revert", apply: revertToSaved, desc: "Discard unsaved edits, restoring the last saved version.", enabled: func(s *State) bool { return s.dirty }},
		{label: "Exit Editor", apply: func(s *State) { s.exitRequested = true }, desc: "Leave the editor (asks first if there are unsaved changes)."},
	}},
	{label: "Edit", items: []dropdownEntry{
		{label: "Undo", apply: undoOne, hotkey: "Ctrl+Z", desc: "Step back one change.", enabled: func(s *State) bool { return len(s.undo) > 0 }},
		{label: "Redo", apply: redoOne, hotkey: "Ctrl+Y", desc: "Re-apply the last undone change.", enabled: func(s *State) bool { return len(s.redo) > 0 }},
		{label: "Fill Layer", apply: fillEntireLayer, hotkey: "Ctrl+Shift+F", desc: "Flood the whole active layer with the current brush.", enabled: canFillLayer},
		{label: "Select All", apply: selectWholeMap, hotkey: "Ctrl+A", desc: "Select the whole map as a region."},
		{label: "Copy Region", apply: copySelection, hotkey: "Ctrl+C", desc: "Copy the selected region (tiles + entities) to the clipboard.", enabled: hasSelection},
		{label: "Cut Region", apply: cutSelection, hotkey: "Ctrl+X", desc: "Copy the selected region, then clear it.", enabled: hasSelection},
		{label: "Paste Region", apply: menuPaste, hotkey: "Ctrl+V", desc: "Paste the clipboard at the cursor (or map center).", enabled: hasClipboard},
		{label: "Duplicate", apply: duplicateSelection, hotkey: "Ctrl+D", desc: "Copy the selection and paste it one tile down-right.", enabled: hasSelection},
		{label: "Flip Clipboard ↔", apply: flipClipboardH, desc: "Mirror the copied region left-right (remaps facings).", enabled: hasClipboard},
		{label: "Flip Clipboard ↕", apply: flipClipboardV, desc: "Mirror the copied region top-bottom (remaps facings).", enabled: hasClipboard},
		{label: "Rotate Clipboard 90°", apply: rotateClipboardCW, desc: "Rotate the copied region 90° clockwise (remaps facings).", enabled: hasClipboard},
	}},
	{label: "View", items: []dropdownEntry{
		{label: "Center on Start", apply: func(s *State) { centerViewOnTile(s, s.area.StartTileX, s.area.StartTileZ) }, hotkey: "G", desc: "Scroll the canvas to the player start tile."},
		{label: "Go to Tile…", apply: openGotoModal, desc: "Jump the view to a typed X,Z or a saved bookmark."},
		{label: "Reset View", apply: resetView, hotkey: "Home", desc: "Reset zoom to 100% and re-center the canvas."},
		{label: "Isometric View", apply: func(s *State) { setIsoView(s, true) }, hotkey: "I", desc: "3D block view showing elevation (default). I toggles.", active: func(s *State) bool { return s.isoView }},
		{label: "Top Down View", apply: func(s *State) { setIsoView(s, false) }, desc: "Flat top-down grid view.", active: func(s *State) bool { return !s.isoView }},
		{label: "Object Animation", apply: func(s *State) { s.animateObjects = !s.animateObjects }, desc: "Animate foliage sway & torch flicker in 3D (off = still, faster).", active: func(s *State) bool { return s.animateObjects }},
		{label: "Tile Glyphs", apply: toggleTileGlyphs, desc: "Overlay each tile's letter code on the canvas.", active: func(s *State) bool { return s.showTileGlyphs }},
		{label: "Door Links", apply: func(s *State) { s.showDoorLinks = !s.showDoorLinks }, desc: "Draw lines connecting linked doors.", active: func(s *State) bool { return s.showDoorLinks }},
		{label: "Coverage Heatmap", apply: func(s *State) { s.showHeatmap = !s.showHeatmap }, desc: "Top-down: tint tiles by distance from start; flag unreachable pockets.", active: func(s *State) bool { return s.showHeatmap }},
		{label: "Cycle Day Phase", apply: cyclePreviewPhase, hotkey: "T", desc: "Preview the map lit at the next time of day."},
	}},
	{label: "Assets", items: []dropdownEntry{
		{label: "Sounds…", apply: openSoundsModal, desc: "Create sound effects and bind them to game cues."},
		{label: "Foe Visuals…", apply: openFoeViewModal, desc: "Tune a foe's sprite, placement, and tint — or import a PNG."},
		{label: "Party Visuals…", apply: openPartyViewModal, desc: "Tune a party class's sprite, placement, and tint — or import a PNG."},
		{label: "Hit Glyphs…", apply: openHitGlyphsModal, desc: "Preview the combat hit symbols (slash, impact, frost, …)."},
		{label: "Object Browser…", apply: openObjectViewModal, desc: "Spot-check every decor & prop as live 3D thumbnails."},
		{label: "Object List…", apply: openEntityListModal, desc: "Jump to any pack, chest, or door on the map."},
		{label: "Prefabs…", apply: openPrefabsModal, desc: "Save the copied region to a reusable stamp, or load one onto the clipboard."},
	}},
	{label: "Map", items: []dropdownEntry{
		{label: "Dialogs…", apply: openDialogListModal, desc: "Author the area's branching conversations."},
		{label: "Map Stats…", apply: openStatsModal, desc: "Tile mix, content counts, and a rough encounter budget."},
		{label: "Validate", apply: openValidateModal, desc: "Check the map for reachability and setup problems."},
		{label: "Playtest", apply: func(s *State) { s.testRequested = true }, hotkey: "F5", desc: "Launch the map in-game from its start tile."},
	}},
	{label: "Help", items: []dropdownEntry{
		{label: "Keyboard Shortcuts…", apply: openHelpModal, hotkey: "?", desc: "Show the full list of editor shortcuts."},
	}},
}

// menuEntries returns the rows of the currently-open menu (ddMenu owner), or nil
// when the open-menu index is somehow out of range. The File menu appends the
// recent-maps list dynamically (it changes at runtime, so it can't be a static row).
func menuEntries(s *State) []dropdownEntry {
	if s.dropdown.menu < 0 || s.dropdown.menu >= len(editorMenus) {
		return nil
	}
	items := editorMenus[s.dropdown.menu].items
	if editorMenus[s.dropdown.menu].label == "File" {
		if recent := recentMapEntries(s); len(recent) > 0 {
			out := make([]dropdownEntry, 0, len(items)+len(recent))
			out = append(out, items...)
			out = append(out, recent...)
			return out
		}
	}
	return items
}

// recentMapEntries builds the File-menu recent-maps rows: a disabled header, then
// one row per recent map (skipping the one already open). Opening is non-destructive
// (openRecentPath guards unsaved changes).
func recentMapEntries(s *State) []dropdownEntry {
	paths := s.recentSnapshot // snapshot taken in openMenu; avoids a per-frame prefs re-read
	if len(paths) == 0 {
		return nil
	}
	cur := s.area.Path
	var out []dropdownEntry
	for _, p := range paths {
		if p == cur {
			continue // already open
		}
		path := p
		out = append(out, dropdownEntry{
			label: core.MapIDFromPath(path),
			apply: func(s *State) { openRecentPath(s, path) },
			desc:  "Open " + path,
		})
	}
	if len(out) == 0 {
		return nil
	}
	// Lead with a non-selectable header so the list reads as a group.
	header := dropdownEntry{label: "— Recent —", enabled: func(*State) bool { return false }}
	return append([]dropdownEntry{header}, out...)
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
	s.recentSnapshot = RecentMaps() // read prefs once per open, not per frame while the menu shows
	input.ResetStickEdges()
}

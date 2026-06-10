package explore

import (
	"crawler/internal/app/audio"
	"crawler/internal/app/core"
	"crawler/internal/app/input"
	"crawler/internal/app/render"
)

// Equipment tab input. The tab works like the Items menu rather than a
// drag-and-drop board: a 2-D cursor walks members (columns) and equip
// slots (rows), and Confirm / a mouse click on a slot opens a smaller
// sub-modal — the item picker — listing every inventory item eligible
// for that slot plus an "Unequip" row. Picking a row equips (swapping
// the displaced item back to inventory) or unequips, then closes the
// picker. While the picker is open it owns all panel input (see
// updatePanels), so Back closes just the picker, not the whole overlay.

// updateEquipmentTab routes one frame of Equipment-tab input while the
// slot picker is CLOSED. Left/Right picks the member column (shared
// PanelsRowCursor, so the card header highlights it), Up/Down picks the
// slot row, and Confirm — or a left-click on a slot — opens that slot's
// item picker. The Up/Down and Left/Right reads touch disjoint
// (vertical vs horizontal) edge memory, so calling both per frame can't
// double-consume an analog edge.
func updateEquipmentTab(g *core.GameState) {
	if len(g.Party) == 0 {
		return
	}
	// Mouse: a click on a slot focuses it and opens its picker.
	if input.ClickPressed() {
		if member, slot, ok := render.EquipPanelSlotHit(input.PointerPos()); ok {
			g.PanelsRowCursor = member
			g.EquipSlotCursor = int(slot)
			openEquipPicker(g)
			return
		}
	}
	g.PanelsRowCursor = input.CursorLeftRightWrap(g.PanelsRowCursor, len(g.Party))
	g.EquipSlotCursor = input.CursorUpDown(g.EquipSlotCursor, int(core.EquipSlotCount))
	if input.ConfirmPressed() {
		openEquipPicker(g)
	}
}

// updateEquipPicker drives the slot's item-picker sub-modal. Up/Down
// walks the rows, Confirm equips/unequips the focused row, Back closes
// the picker (returning to slot navigation — NOT closing the overlay).
// Mouse: click a row to pick it, or click outside the picker card to
// dismiss. The member + slot are read from the frozen cursors (input
// can't move them while the picker owns the frame).
// equipPickerRowsBuf is updateEquipPicker's reusable row buffer — the
// picker recomputes its rows every frame it's open (cursor clamp + click
// resolution), so without this each open-picker frame allocates an
// inventory-sized slice. Single-threaded update loop; valid per frame.
var equipPickerRowsBuf []core.EquipPickerRow

func updateEquipPicker(g *core.GameState) {
	member := g.PanelsRowCursor
	slot := core.EquipSlotIndex(g.EquipSlotCursor)
	if member < 0 || member >= len(g.Party) {
		closeEquipPicker(g)
		return
	}
	if input.BackPressed() {
		closeEquipPicker(g)
		return
	}
	rows := core.EquipPickerRowsInto(equipPickerRowsBuf, g, member, slot)
	equipPickerRowsBuf = rows
	if input.ClickPressed() {
		pt := input.PointerPos()
		if row, ok := render.EquipPanelPickerRowHit(pt); ok {
			applyEquipPick(g, member, slot, rows, row)
			return
		}
		if render.EquipPanelClickOutsidePicker(pt) {
			closeEquipPicker(g)
			return
		}
	}
	g.EquipPickerCursor = input.CursorUpDown(g.EquipPickerCursor, len(rows))
	if input.ConfirmPressed() {
		applyEquipPick(g, member, slot, rows, g.EquipPickerCursor)
	}
}

// applyEquipPick resolves the chosen picker row: the Unequip row routes
// the slot's item back to inventory; any other row equips that item
// (swapping the displaced one back). A landed change pings the gilt
// cue, a no-op (empty pick / refused) pings the miss cue, and either
// way the picker closes so the player drops back to the slot list.
func applyEquipPick(g *core.GameState, member int, slot core.EquipSlotIndex, rows []core.EquipPickerRow, idx int) {
	if idx < 0 || idx >= len(rows) {
		closeEquipPicker(g)
		return
	}
	row := rows[idx]
	var ok bool
	if row.Unequip {
		ok = core.UnequipToInventory(g, member, slot)
	} else {
		ok = core.EquipFromInventory(g, member, slot, row.Kind)
	}
	if ok {
		audio.Play(audio.SoundInputGreat)
	} else {
		audio.Play(audio.SoundInputMiss)
	}
	closeEquipPicker(g)
}

// openEquipPicker raises the slot picker on the focused slot, parking
// its cursor on the first row.
func openEquipPicker(g *core.GameState) {
	g.EquipPickerOpen = true
	g.EquipPickerCursor = 0
}

// closeEquipPicker dismisses the picker; thin wrapper over the core
// reset so the explore-side input reads one verb.
func closeEquipPicker(g *core.GameState) {
	core.CloseEquipPicker(g)
}

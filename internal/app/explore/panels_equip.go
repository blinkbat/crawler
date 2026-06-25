package explore

import (
	"crawler/internal/app/audio"
	"crawler/internal/app/core"
	"crawler/internal/app/input"
	"crawler/internal/app/render"
)

// Equipment tab input. A 2-D cursor walks members × slots; Confirm / click opens
// the item picker (eligible items + an Unequip row), which then owns all panel
// input so Back closes just the picker.

// updateEquipmentTab routes input while the slot picker is CLOSED: Left/Right picks
// the member column, Up/Down the slot row, Confirm / click opens the picker.
func updateEquipmentTab(g *core.GameState) {
	if len(g.Party) == 0 {
		return
	}
	// Mouse: a click on a slot focuses it and opens the picker.
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

// updateEquipPicker drives the slot's item-picker sub-modal: Up/Down walks rows,
// Confirm equips/unequips, Back closes the picker (to slot nav, NOT the overlay);
// click a row to pick or outside the card to dismiss. Member+slot from the frozen cursors.
// equipPickerRowsBuf is the reusable row buffer (rows recompute every open frame).
var equipPickerRowsBuf []core.EquipPickerRow

func updateEquipPicker(g *core.GameState) {
	member := g.PanelsRowCursor
	slot := core.EquipSlotIndex(g.EquipSlotCursor)
	if !core.PartyIndexInRange(g.Party, member) {
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

// applyEquipPick resolves the chosen row: Unequip routes the slot's item back to
// inventory; any other row equips it (swapping the displaced one back). Pings
// gilt on a change, miss on a no-op; the picker closes either way.
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
	audio.PlayResult(ok)
	closeEquipPicker(g)
}

// openEquipPicker raises the slot picker on the focused slot, cursor at row 0.
func openEquipPicker(g *core.GameState) {
	g.EquipPickerOpen = true
	g.EquipPickerCursor = 0
}

// closeEquipPicker dismisses the picker (thin wrapper over the core reset).
func closeEquipPicker(g *core.GameState) {
	core.CloseEquipPicker(g)
}

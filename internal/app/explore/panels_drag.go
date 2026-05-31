package explore

import (
	"crawler/internal/app/core"
	"crawler/internal/app/input"
	"crawler/internal/app/render"
)

// updateEquipmentDrag handles mouse drag-and-drop on the Equipment
// tab. LMB-down on an inventory tile or a filled equipped slot
// starts a drag; LMB-up tries to land it on a compatible slot or the
// inventory strip. Cancel-on-overlay-close is the caller's job (see
// closePanels).
//
// The hit rects we test against are written by drawPanelsEquipment
// the previous frame and exposed via render's EquipPanel* lookups —
// single seam so render owns the layout and input just reads it.
func updateEquipmentDrag(g *core.GameState) {
	mouse := input.PointerPos()

	// Drag start — left mouse pressed, no existing drag.
	if input.DragStartPressed() && g.EquipDrag.Source == core.EquipDragSourceNone {
		// Slot drag-start: only if the slot holds something.
		if pi, slot, ok := render.EquipPanelSlotHit(mouse); ok {
			if pi >= 0 && pi < len(g.Party) {
				kind := g.Party[pi].Equipped[slot]
				if kind != core.ItemNone {
					g.EquipDrag = core.NewSlotDrag(kind, pi, slot)
					return
				}
			}
		}
		// Inventory drag-start: any tile in the strip.
		if invIdx, kind, ok := render.EquipPanelInventoryHit(mouse, *g); ok && kind != core.ItemNone {
			g.EquipDrag = core.NewInventoryDrag(kind, invIdx)
			return
		}
	}

	// Drag release — left mouse released, drag in progress.
	if input.DragReleased() && g.EquipDrag.Source != core.EquipDragSourceNone {
		defer core.ClearEquipDrag(g)
		// Dropped on a slot?
		if pi, slot, ok := render.EquipPanelSlotHit(mouse); ok {
			tryEquipDrop(g, pi, slot)
			return
		}
		// Dropped on the inventory strip — only meaningful when
		// dragging from a slot (unequip path).
		if g.EquipDrag.Source == core.EquipDragSourceSlot && render.EquipPanelInventoryAreaHit(mouse) {
			unequipBackToInventory(g, g.EquipDrag.PartyIndex, g.EquipDrag.SlotIndex)
			return
		}
		// Anywhere else — cancel; ClearEquipDrag fires via the defer.
	}
}

// tryEquipDrop resolves "dropped on slot (pi, slot)" given the active
// drag state. Handles three shapes:
//
//   - Inventory → slot: consume one from inventory, equip into slot.
//     If slot held something, return it to inventory (no net loss).
//   - Slot → slot: swap or move between two slots on the same / different
//     party members. Same item can move freely; type compat is checked.
//   - Slot → same slot: no-op (treated as a drop-cancel).
//
// Incompatible drops (wrong slot type) are silently ignored so the
// player can drag-cancel by dropping over an invalid target.
func tryEquipDrop(g *core.GameState, pi int, slot core.EquipSlotIndex) {
	if pi < 0 || pi >= len(g.Party) {
		return
	}
	if !core.CanEquipInSlot(g.EquipDrag.Kind, slot) {
		return
	}
	target := &g.Party[pi]
	switch g.EquipDrag.Source {
	case core.EquipDragSourceInventory:
		// Re-resolve by kind rather than by the cached InventoryIndex —
		// the inventory CAN shrink/grow between drag-start and drag-
		// release (a future inline-consumable hotkey, a steal pickup
		// during a long drag, etc.) and an invalid cached index would
		// otherwise silently bail even though the kind is still
		// findable. ConsumeItem's own `ok` return covers the
		// "couldn't find it" case downstream.
		if findInventoryIndex(g.Inventory, g.EquipDrag.Kind) < 0 {
			return
		}
		var ok bool
		g.Inventory, ok = core.ConsumeItem(g.Inventory, g.EquipDrag.Kind)
		if !ok {
			return
		}
		prev, equipOk := core.EquipItem(target, slot, g.EquipDrag.Kind)
		if !equipOk {
			// Equip refused — put the consumed item back.
			g.Inventory = core.AddItem(g.Inventory, g.EquipDrag.Kind, 1)
			return
		}
		if prev != core.ItemNone {
			g.Inventory = core.AddItem(g.Inventory, prev, 1)
		}
	case core.EquipDragSourceSlot:
		// Slot→slot: same member or cross-member. Same exact slot =
		// no-op cancel. Different slot = swap contents (if target
		// slot is occupied) or move (if target is empty).
		if g.EquipDrag.PartyIndex == pi && g.EquipDrag.SlotIndex == slot {
			return
		}
		if g.EquipDrag.PartyIndex < 0 || g.EquipDrag.PartyIndex >= len(g.Party) {
			return
		}
		source := &g.Party[g.EquipDrag.PartyIndex]
		// Also check the swapped item can land in source's slot — a
		// hand item swapping into a hand slot is fine, but a hand
		// item trying to land in an armor slot must be refused.
		incoming := target.Equipped[slot]
		if incoming != core.ItemNone && !core.CanEquipInSlot(incoming, g.EquipDrag.SlotIndex) {
			return
		}
		moved := core.UnequipItem(source, g.EquipDrag.SlotIndex)
		prev, equipOk := core.EquipItem(target, slot, moved)
		if !equipOk {
			// Refused — put the held item back where it came from.
			source.Equipped[g.EquipDrag.SlotIndex] = moved
			return
		}
		// Swap back if the target was holding something.
		if prev != core.ItemNone {
			source.Equipped[g.EquipDrag.SlotIndex] = prev
		}
	}
}

// unequipBackToInventory moves an equipped item back into the shared
// inventory. Used when the player drags a slot tile into the
// inventory strip.
func unequipBackToInventory(g *core.GameState, pi int, slot core.EquipSlotIndex) {
	if pi < 0 || pi >= len(g.Party) {
		return
	}
	kind := core.UnequipItem(&g.Party[pi], slot)
	if kind != core.ItemNone {
		g.Inventory = core.AddItem(g.Inventory, kind, 1)
	}
}

// findInventoryIndex returns the index of the first stack matching
// `kind` with a positive count, or -1. Used to confirm the dragged
// item still exists in inventory before consuming it.
func findInventoryIndex(inv []core.ItemStack, kind core.ItemKind) int {
	for i, st := range inv {
		if st.Kind == kind && st.Count > 0 {
			return i
		}
	}
	return -1
}

// updateEquipmentTab routes one frame of Equipment-tab input. The tab
// supports BOTH mouse drag-and-drop AND a keyboard/controller cursor;
// g.EquipCursorActive tracks which device last drove the panel so the
// two never fight. Mouse motion / clicks hand control to the drag path;
// a directional or Confirm edge wakes the cursor. The member-column
// highlight (PanelsRowCursor) tracks the cursor's member so the shared
// card header lights up the focused column.
func updateEquipmentTab(g *core.GameState) {
	// Read the cursor edges ONCE this frame — UpPressed / DownPressed /
	// CursorLeftRight touch the analog-stick edge memory, so calling
	// them twice would consume the edge and drop the second read.
	up := input.UpPressed()
	down := input.DownPressed()
	lr := input.CursorLeftRight()
	confirm := input.ConfirmPressed()

	// Device arbitration. Mouse movement or a held click hands control
	// to the drag path; any cursor edge wakes the keyboard/controller
	// cursor.
	if input.PointerMoved() || input.DragHeld() {
		g.EquipCursorActive = false
	}
	if up || down || lr != 0 || confirm {
		g.EquipCursorActive = true
	}

	if g.EquipCursorActive {
		updateEquipCursorNav(g, up, down, lr, confirm)
	} else {
		updateEquipmentDrag(g)
	}

	// Keep the member-column highlight on the cursor's member so the
	// shared card header brightens the focused column.
	if g.EquipCursor.Member >= 0 && g.EquipCursor.Member < len(g.Party) {
		g.PanelsRowCursor = g.EquipCursor.Member
	}
}

// updateEquipCursorNav moves the Equipment-tab focus cell and resolves
// a Confirm. Members are columns (Left/Right) and equip slots are rows
// (Up/Down); pressing Down past the last slot drops the focus into the
// inventory strip, and Up from the strip returns to the slot column.
// The edges are passed in (already read once by updateEquipmentTab) to
// avoid double-consuming the analog-stick edge memory.
func updateEquipCursorNav(g *core.GameState, up, down bool, lr int, confirm bool) {
	if len(g.Party) == 0 {
		return
	}
	cur := &g.EquipCursor
	if cur.Member < 0 {
		cur.Member = 0
	}
	if cur.Member >= len(g.Party) {
		cur.Member = len(g.Party) - 1
	}
	invCount := render.EquipPanelInventoryVisibleCount()

	if cur.OnInventory {
		// Strip focus: Left/Right walk tiles; Up returns to the slot
		// column at the bottom slot of the current member.
		if lr != 0 && invCount > 0 {
			cur.InvTile = core.WrapIndex(cur.InvTile+lr, invCount)
		}
		if up {
			cur.OnInventory = false
			cur.Slot = core.EquipSlotCount - 1
		}
	} else {
		// Slot focus: Left/Right change member column; Up/Down move
		// slot rows; Down past the last slot drops into the strip.
		if lr != 0 {
			cur.Member = core.WrapIndex(cur.Member+lr, len(g.Party))
		}
		if up && cur.Slot > 0 {
			cur.Slot--
		}
		if down {
			if int(cur.Slot) < int(core.EquipSlotCount)-1 {
				cur.Slot++
			} else if invCount > 0 {
				cur.OnInventory = true
			}
		}
	}

	// Re-clamp the strip cursor in case the inventory shrank (e.g. the
	// last equippable item just got equipped).
	if cur.OnInventory {
		switch {
		case invCount <= 0:
			cur.OnInventory = false
		case cur.InvTile >= invCount:
			cur.InvTile = invCount - 1
		case cur.InvTile < 0:
			cur.InvTile = 0
		}
	}

	if confirm {
		equipCursorConfirm(g)
	}
}

// equipCursorConfirm performs the keyboard/controller "lift or place"
// at the focused cell, reusing the exact drag-drop rules so the cursor
// and mouse paths can never diverge. With nothing held it lifts the
// focused item into g.EquipDrag; while holding it lands the item on the
// focused slot (tryEquipDrop) or unequips onto the strip, then clears
// the held state — mirroring the mouse release block.
func equipCursorConfirm(g *core.GameState) {
	cur := g.EquipCursor
	if g.EquipDrag.Source == core.EquipDragSourceNone {
		// Lift the focused item.
		if cur.OnInventory {
			kind, ok := render.EquipPanelInventoryEntryKind(cur.InvTile)
			if !ok || kind == core.ItemNone {
				return
			}
			idx := findInventoryIndex(g.Inventory, kind)
			if idx < 0 {
				return
			}
			g.EquipDrag = core.NewInventoryDrag(kind, idx)
			return
		}
		if cur.Member < 0 || cur.Member >= len(g.Party) {
			return
		}
		kind := g.Party[cur.Member].Equipped[cur.Slot]
		if kind == core.ItemNone {
			return
		}
		g.EquipDrag = core.NewSlotDrag(kind, cur.Member, cur.Slot)
		return
	}
	// Place the held item — clear the drag on every exit path, same as
	// the mouse release's deferred ClearEquipDrag.
	defer core.ClearEquipDrag(g)
	if cur.OnInventory {
		// Only a slot-sourced hold means anything on the strip (unequip);
		// an inventory-sourced hold dropped back on the strip just
		// cancels (the item was never removed from inventory on lift).
		if g.EquipDrag.Source == core.EquipDragSourceSlot {
			unequipBackToInventory(g, g.EquipDrag.PartyIndex, g.EquipDrag.SlotIndex)
		}
		return
	}
	tryEquipDrop(g, cur.Member, cur.Slot)
}

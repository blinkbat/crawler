package explore

import (
	"crawler/internal/app/core"
	"crawler/internal/app/render"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// updateEquipmentDrag handles mouse drag-and-drop on the Equipment
// tab. LMB-down on an inventory tile or a filled equipped slot
// starts a drag; LMB-up tries to land it on a compatible slot or the
// inventory strip. Cancel-on-overlay-close is the caller's job (see
// closePanels).
//
// The hit rects we test against are written by drawPanelsEquipment
// the previous frame and exposed via render.LastEquipPanelLayout /
// EquipPanel* lookups — single seam so render owns the layout and
// input just reads it.
func updateEquipmentDrag(g *core.GameState) {
	mouse := rl.GetMousePosition()

	// Drag start — left mouse pressed, no existing drag.
	if rl.IsMouseButtonPressed(rl.MouseLeftButton) && g.EquipDrag.Source == core.EquipDragSourceNone {
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
	if rl.IsMouseButtonReleased(rl.MouseLeftButton) && g.EquipDrag.Source != core.EquipDragSourceNone {
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

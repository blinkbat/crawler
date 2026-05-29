package core

// Equipment slot helpers. The runtime party member carries an
// [EquipSlotCount]ItemKind array; this file owns the rules for moving
// items between that array and GameState.Inventory.

// CanEquipInSlot reports whether `item` fits `slot`. SlotNone items
// (cheese / jerky) never fit anywhere — they're inventory-only.
// Cross-slot fit is decided by EquipmentSlotType: a Hand-typed item
// fits either hand slot, an Accessory fits either accessory slot, an
// Armor item fits only the armor slot.
func CanEquipInSlot(kind ItemKind, slot EquipSlotIndex) bool {
	if kind == ItemNone {
		return false
	}
	def, ok := ItemInfoOk(kind)
	if !ok || def.Slot == SlotNone {
		return false
	}
	return def.Slot == SlotIndexType(slot)
}

// EquipItem stamps `kind` into the named slot on `m` (no inventory
// math — the caller is responsible for removing the item from the
// inventory and putting any displaced item back). Returns the kind
// previously occupying the slot so the caller can return it to the
// inventory. Refuses incompatible (kind, slot) combos: returns
// ItemNone and leaves the slot unchanged.
func EquipItem(m *PartyMember, slot EquipSlotIndex, kind ItemKind) (ItemKind, bool) {
	if m == nil || slot < 0 || slot >= EquipSlotCount {
		return ItemNone, false
	}
	if !CanEquipInSlot(kind, slot) {
		return ItemNone, false
	}
	prev := m.Equipped[slot]
	m.Equipped[slot] = kind
	return prev, true
}

// UnequipItem clears the named slot and returns whatever was sitting
// in it. The caller routes that kind back into the inventory.
func UnequipItem(m *PartyMember, slot EquipSlotIndex) ItemKind {
	if m == nil || slot < 0 || slot >= EquipSlotCount {
		return ItemNone
	}
	prev := m.Equipped[slot]
	m.Equipped[slot] = ItemNone
	return prev
}

// walkEquipped invokes `fn` once for every equipped item on `m`. Empty
// slots and unknown kinds are skipped. The three Effective* readers
// share this so a new "per-equipment contribution" reader (e.g. a
// future EffectiveSpeed cap or HP-bonus accumulator) lives in one
// loop shape; today's three were near-identical iterate-and-accumulate
// patterns that would silently drift if a slot rule (e.g. "two-handed
// weapons block off-hand") needed to be added.
func walkEquipped(m PartyMember, fn func(def ItemDefinition)) {
	for i := 0; i < int(EquipSlotCount); i++ {
		kind := m.Equipped[i]
		if kind == ItemNone {
			continue
		}
		def, ok := ItemInfoOk(kind)
		if !ok {
			continue
		}
		fn(def)
	}
}

// EffectiveArmor sums the member's base Armor with the ArmorBonus of
// every equipped item. Read by the damage path so equipped armor
// stacks on top of base — ApplyArmor never needs to know about the
// Equipped array directly.
func EffectiveArmor(m PartyMember) int {
	armor := m.Armor
	walkEquipped(m, func(def ItemDefinition) { armor += def.ArmorBonus })
	if armor < 0 {
		armor = 0
	}
	return armor
}

// EffectiveMDef returns the magic-defense value used by
// ApplyMagicDefense — base derived from WIS plus any MDefBonus on
// equipped items. Floor at 0.
func EffectiveMDef(m PartyMember) int {
	mdef := MagicDefense(m.Stats)
	walkEquipped(m, func(def ItemDefinition) { mdef += def.MDefBonus })
	if mdef < 0 {
		mdef = 0
	}
	return mdef
}

// EffectiveStats returns the member's base stats with equipped item
// StatBonus values folded in. Used wherever combat / UI reads stats
// for display or rolls — keeps the base Stats block clean (level-up
// spends always edit the base) while equipment effectively re-renders
// the stat sheet. Loops over the Stat enum (instead of hand-unrolling
// the six fields) so a new Stat constant + statTable row automatically
// picks up its equipment bonus without a parallel edit here.
func EffectiveStats(m PartyMember) Stats {
	out := m.Stats
	walkEquipped(m, func(def ItemDefinition) {
		for s := Stat(0); s < StatCount; s++ {
			delta := def.StatBonus[s]
			if delta == 0 {
				continue
			}
			cur := statTable[s].Get(out)
			statSetters[s](&out, cur+delta)
		}
	})
	return out
}

// EquipDragState tracks an in-progress drag-and-drop on the Equipment
// panel. PartyIndex / SlotIndex point to the source if the drag began
// from a slot (Source == EquipDragSourceSlot); InventoryIndex points
// to the source if it began from the inventory list
// (Source == EquipDragSourceInventory). Kind is the item being moved
// so the renderer can paint a tooltip on the cursor without
// re-resolving the source every frame.
//
// EquipDragSourceNone means no drag is active; the panel renders
// resting state.
type EquipDragSource int

const (
	EquipDragSourceNone EquipDragSource = iota
	EquipDragSourceInventory
	EquipDragSourceSlot
)

type EquipDragState struct {
	Source         EquipDragSource
	Kind           ItemKind
	PartyIndex     int
	SlotIndex      EquipSlotIndex
	InventoryIndex int
}

// NewSlotDrag builds an EquipDragSourceSlot state with the source
// member + slot populated and the inventory index zeroed. Use
// instead of building the literal by hand so a future invariant
// (e.g. cross-member swap gates) lives in one constructor — and
// the InventoryIndex field can never accidentally leak a stale
// value from an earlier inventory drag.
func NewSlotDrag(kind ItemKind, partyIndex int, slot EquipSlotIndex) EquipDragState {
	return EquipDragState{
		Source:     EquipDragSourceSlot,
		Kind:       kind,
		PartyIndex: partyIndex,
		SlotIndex:  slot,
	}
}

// NewInventoryDrag builds an EquipDragSourceInventory state with the
// inventory index populated and slot/party fields zeroed. Symmetric
// sibling of NewSlotDrag.
func NewInventoryDrag(kind ItemKind, inventoryIndex int) EquipDragState {
	return EquipDragState{
		Source:         EquipDragSourceInventory,
		Kind:           kind,
		InventoryIndex: inventoryIndex,
	}
}

// ClearEquipDrag resets the drag state to "not dragging." Called on
// drop, drag-cancel, and when the panels overlay closes.
func ClearEquipDrag(g *GameState) {
	g.EquipDrag = EquipDragState{}
}

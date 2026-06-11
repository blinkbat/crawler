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
	// Bound the slot before SlotIndexType indexes the fixed equipSlotInfo
	// array — an out-of-range slot must return false, not panic. EquipItem /
	// UnequipItem guard the slot before they reach here, but EquipFromInventory
	// (and any future caller) routes an unguarded slot straight in.
	if slot < 0 || slot >= EquipSlotCount {
		return false
	}
	def, ok := ItemInfoOk(kind)
	if !ok || def.Slot == SlotNone {
		return false
	}
	return def.Slot == SlotIndexType(slot)
}

// ItemIsTwoHanded reports whether `kind` is a two-handed hand weapon (one
// that occupies both hand slots). ItemNone and unregistered kinds are
// false. The off-hand exclusion the equip path enforces reads through this
// so "what's two-handed?" lives in one place.
func ItemIsTwoHanded(kind ItemKind) bool {
	if kind == ItemNone {
		return false
	}
	def, ok := ItemInfoOk(kind)
	return ok && def.TwoHanded
}

// otherHand returns the opposite hand slot for a hand slot, or the slot
// itself (with ok=false) for a non-hand slot.
func otherHand(slot EquipSlotIndex) (EquipSlotIndex, bool) {
	switch slot {
	case EquipRightHand:
		return EquipLeftHand, true
	case EquipLeftHand:
		return EquipRightHand, true
	default:
		return slot, false
	}
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
			next := cur + delta
			// Floor at 0 so a negative StatBonus (a cursed / debuff item)
			// can't drive an effective stat below zero, which would feed
			// negative values into MaxHPFor / damage / accuracy math. Mirrors
			// the 0-clamp AdjustStat applies to base-stat edits.
			if next < 0 {
				next = 0
			}
			statSetters[s](&out, next)
		}
	})
	// An active stat buff (Cleric's Bless) folds on top of equipment, on the
	// same per-stat floor-at-0 rule. Combat-only and per-turn-ticked, so this
	// read re-renders the boosted sheet only while BuffTurns is running; a
	// member with no buff (the common case) skips the loop entirely.
	if m.BuffTurns > 0 {
		for s := Stat(0); s < StatCount; s++ {
			delta := statTable[s].Get(m.BuffStats)
			if delta == 0 {
				continue
			}
			next := statTable[s].Get(out) + delta
			if next < 0 {
				next = 0
			}
			statSetters[s](&out, next)
		}
	}
	return out
}

// EquipPickerRow is one selectable row in an equip slot's item picker
// (the sub-modal the Equipment tab opens on a slot). A Kind == ItemNone
// row with Unequip set is the synthetic "take it off" row, present only
// when the slot is currently filled; every other row is an inventory
// item that fits the slot, with its stack Count for the "xN" badge.
type EquipPickerRow struct {
	Kind    ItemKind
	Count   int
	Unequip bool
}

// EquipPickerRows builds the ordered row list the slot picker shows for
// (member, slot): an "Unequip" row first when the slot holds something,
// then every inventory item that CanEquipInSlot, in inventory order.
// Render draws this list and the input layer resolves the chosen
// cursor/click index against it, so the two share ONE ordering and
// can't drift (the same single-source-of-truth pattern the old
// drag-drop rules used). Nil-safe.
func EquipPickerRows(g *GameState, member int, slot EquipSlotIndex) []EquipPickerRow {
	return EquipPickerRowsInto(nil, g, member, slot)
}

// EquipPickerRowsInto is EquipPickerRows into a caller-owned buffer
// (re-sliced to length 0) — the allocation-free variant for the slot
// picker's per-frame update/draw paths, which would otherwise allocate
// an inventory-sized row slice twice per frame while the picker is open.
// The returned slice aliases buf's backing array and is valid until the
// caller's next reuse of it. Nil-safe.
func EquipPickerRowsInto(buf []EquipPickerRow, g *GameState, member int, slot EquipSlotIndex) []EquipPickerRow {
	buf = buf[:0]
	if g == nil {
		return buf
	}
	// Guard slot the same way member is guarded below: Equipped is a
	// [EquipSlotCount]ItemKind array and CanEquipInSlot -> SlotIndexType
	// now panics on an out-of-range slot, so an exported caller passing a
	// bad slot would crash rather than get an empty list.
	if slot < 0 || slot >= EquipSlotCount {
		return buf
	}
	if member >= 0 && member < len(g.Party) && g.Party[member].Equipped[slot] != ItemNone {
		buf = append(buf, EquipPickerRow{Unequip: true})
	}
	for _, st := range g.Inventory {
		if st.Count > 0 && CanEquipInSlot(st.Kind, slot) {
			buf = append(buf, EquipPickerRow{Kind: st.Kind, Count: st.Count})
		}
	}
	return buf
}

// EquipFromInventory equips one `kind` from the shared inventory into
// the member's slot, returning any displaced item to the inventory (no
// net item loss). Returns false and changes nothing when the member is
// out of range, the kind doesn't fit the slot, or the kind isn't in
// inventory. Centralizes the consume → equip → return-prev sequence the
// Equipment-tab picker drives.
func EquipFromInventory(g *GameState, member int, slot EquipSlotIndex, kind ItemKind) bool {
	if g == nil || member < 0 || member >= len(g.Party) {
		return false
	}
	if !CanEquipInSlot(kind, slot) {
		return false
	}
	inv, ok := ConsumeItem(g.Inventory, kind)
	if !ok {
		return false
	}
	g.Inventory = inv
	m := &g.Party[member]
	// Two-handed weapons occupy BOTH hands, so they can't share a hand-pair
	// with any other hand item. Equipping a two-hander clears the other hand
	// back to inventory; equipping anything into a hand while the other hand
	// holds a two-hander clears that two-hander. This keeps a two-handed
	// weapon from ever co-existing with an off-hand item (which would stack
	// its StatBonus twice across both slots). The freed item routes back to
	// the shared inventory just like a normal displaced item.
	if other, isHand := otherHand(slot); isHand {
		if ItemIsTwoHanded(kind) || ItemIsTwoHanded(m.Equipped[other]) {
			if freed := UnequipItem(m, other); freed != ItemNone {
				g.Inventory = AddItem(g.Inventory, freed, 1)
			}
		}
	}
	prev, equipOk := EquipItem(m, slot, kind)
	if !equipOk {
		// Equip refused — put the consumed item back so it isn't lost.
		g.Inventory = AddItem(g.Inventory, kind, 1)
		return false
	}
	if prev != ItemNone {
		g.Inventory = AddItem(g.Inventory, prev, 1)
	}
	return true
}

// UnequipToInventory clears the member's slot and routes whatever was
// in it back into the shared inventory. Returns false when the slot was
// already empty (or the member is out of range).
func UnequipToInventory(g *GameState, member int, slot EquipSlotIndex) bool {
	if g == nil || member < 0 || member >= len(g.Party) {
		return false
	}
	kind := UnequipItem(&g.Party[member], slot)
	if kind == ItemNone {
		return false
	}
	g.Inventory = AddItem(g.Inventory, kind, 1)
	return true
}

// ResetEquipPanels parks the Equipment-tab cursors and closes the slot
// picker. Called on overlay open and on switching INTO the Equipment
// tab so a re-entry starts on the first slot with no stale picker open.
func ResetEquipPanels(g *GameState) {
	g.EquipSlotCursor = 0
	g.EquipPickerOpen = false
	g.EquipPickerCursor = 0
}

// CloseEquipPicker dismisses the slot picker without touching the slot
// cursor. Used by the Back handler and by anything that needs to tear
// the sub-modal down (e.g. an area transition firing while it's open).
func CloseEquipPicker(g *GameState) {
	g.EquipPickerOpen = false
	g.EquipPickerCursor = 0
}

package core

// Equipment slot helpers. The party member carries an [EquipSlotCount]ItemKind
// array; this file owns moving items between it and GameState.Inventory.

// validEquipSlot reports whether slot is a real equip slot. Shared bounds
// predicate — load-bearing because SlotIndexType indexes the fixed equipSlotInfo
// array and panics out of range.
func validEquipSlot(slot EquipSlotIndex) bool {
	return slot >= 0 && slot < EquipSlotCount
}

// validPartyMember reports whether `member` indexes a live party slot.
func validPartyMember(g *GameState, member int) bool {
	return PartyIndexInRange(g.Party, member)
}

// CanEquipInSlot reports whether `item` fits `slot`. SlotNone items never fit;
// cross-slot fit is by EquipmentSlotType (a Hand item fits either hand, etc.).
func CanEquipInSlot(kind ItemKind, slot EquipSlotIndex) bool {
	if kind == ItemNone {
		return false
	}
	// Bound before SlotIndexType indexes equipSlotInfo (panics out of range);
	// EquipFromInventory and future callers may route an unguarded slot in.
	if !validEquipSlot(slot) {
		return false
	}
	def, ok := ItemInfoOk(kind)
	if !ok || def.Slot == SlotNone {
		return false
	}
	return def.Slot == SlotIndexType(slot)
}

// ItemIsTwoHanded reports whether `kind` is a two-handed weapon (occupies both
// hand slots). ItemNone/unregistered are false. Single home for the off-hand exclusion.
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

// EquipItem stamps `kind` into the slot on `m` (no inventory math — the caller
// handles that) and returns the displaced kind. Refuses incompatible (kind,
// slot): returns ItemNone, slot unchanged.
func EquipItem(m *PartyMember, slot EquipSlotIndex, kind ItemKind) (ItemKind, bool) {
	if m == nil || !validEquipSlot(slot) {
		return ItemNone, false
	}
	if !CanEquipInSlot(kind, slot) {
		return ItemNone, false
	}
	prev := m.Equipped[slot]
	m.Equipped[slot] = kind
	return prev, true
}

// UnequipItem clears the slot and returns what was in it (caller routes it back
// to inventory).
func UnequipItem(m *PartyMember, slot EquipSlotIndex) ItemKind {
	if m == nil || !validEquipSlot(slot) {
		return ItemNone
	}
	prev := m.Equipped[slot]
	m.Equipped[slot] = ItemNone
	return prev
}

// foldEquipment walks the equipped items ONCE, summing StatBonus/ArmorBonus/
// MDefBonus (empty/unknown slots skipped). Shared by the Effective* readers so
// combat folds equipment once. Per-stat adds are hand-unrolled — hot combat math.
func foldEquipment(m *PartyMember) (stats Stats, armor, mdef int) {
	for i := 0; i < int(EquipSlotCount); i++ {
		kind := m.Equipped[i]
		if kind == ItemNone {
			continue
		}
		def, ok := ItemInfoOk(kind)
		if !ok {
			continue
		}
		armor += def.ArmorBonus
		mdef += def.MDefBonus
		stats.STR += def.StatBonus[StatSTR]
		stats.DEX += def.StatBonus[StatDEX]
		stats.INT += def.StatBonus[StatINT]
		stats.WIS += def.StatBonus[StatWIS]
		stats.VIT += def.StatBonus[StatVIT]
		stats.SPD += def.StatBonus[StatSPD]
	}
	return stats, armor, mdef
}

// EffectiveArmor is base Armor plus equipped + buff ArmorBonus. Delegates to
// EffectiveDefenses so the fold lives in one place (one extra MDef walk).
func EffectiveArmor(m PartyMember) int {
	armor, _ := EffectiveDefenses(m)
	return armor
}

// EffectiveMDef is the magic-defense for ApplyMagicDefense. The WIS-derived base
// reads EFFECTIVE WIS (so a +WIS item/buff hardens it), plus the MDefBonus
// channel and Ice Armor; floored at 0. Delegates to EffectiveDefenses.
func EffectiveMDef(m PartyMember) int {
	_, mdef := EffectiveDefenses(m)
	return mdef
}

// EffectiveDefenses returns Armor AND MDef from a SINGLE equipment+buff walk —
// for the per-hit damage path, which needs both and would otherwise walk twice.
func EffectiveDefenses(m PartyMember) (armor, mdef int) {
	equipDelta, equipArmor, equipMDef := foldEquipment(&m)
	buffStats, buffArmor, buffMDef := SumStatusMods(m.Buffs)
	armor = floorInt(m.Armor + equipArmor + buffArmor)
	// WIS-derived MDef reads effective WIS (base + equip + buff).
	eff := addStatsFloored(addStatsFloored(m.Stats, equipDelta), buffStats)
	mdef = MagicDefense(eff) + equipMDef + buffMDef
	if m.IceArmorTurns > 0 {
		mdef += IceArmorMDef
	}
	return armor, floorInt(mdef)
}

// EffectiveStats returns base stats with equipped StatBonus folded in. Read
// wherever combat/UI needs stats; keeps the base block clean (level-ups edit base).
func EffectiveStats(m PartyMember) Stats {
	return EffectiveStatsPtr(&m)
}

// EffectiveStatsPtr is the pointer form of EffectiveStats, for hot-path callers
// (actorSpeed) that shouldn't pay to copy the whole PartyMember. Only reads m.
func EffectiveStatsPtr(m *PartyMember) Stats {
	// Fold equipment into one delta, add on top of base (floored at 0 so a
	// negative StatBonus can't drive a stat below zero into combat math).
	equipDelta, _, _ := foldEquipment(m)
	out := addStatsFloored(m.Stats, equipDelta)
	// Active stat buffs fold on top, same floor-at-0; an un-buffed member is a no-op.
	buffStats, _, _ := SumStatusMods(m.Buffs)
	return addStatsFloored(out, buffStats)
}

// EquipPickerRow is one selectable row in an equip slot's item picker. A Kind ==
// ItemNone row with Unequip set is the synthetic "take it off" row (present only
// when the slot is filled); other rows are inventory items with their Count.
type EquipPickerRow struct {
	Kind    ItemKind
	Count   int
	Unequip bool
}

// EquipPickerRows builds the slot picker's ordered rows for (member, slot): an
// "Unequip" row first when the slot is filled, then every CanEquipInSlot
// inventory item in order. Render and input share this ONE ordering. Nil-safe.
func EquipPickerRows(g *GameState, member int, slot EquipSlotIndex) []EquipPickerRow {
	return EquipPickerRowsInto(nil, g, member, slot)
}

// EquipPickerRowsInto is EquipPickerRows into a caller-owned buffer (re-sliced to
// 0) — the alloc-free per-frame variant. Returned slice aliases buf. Nil-safe.
func EquipPickerRowsInto(buf []EquipPickerRow, g *GameState, member int, slot EquipSlotIndex) []EquipPickerRow {
	buf = buf[:0]
	if g == nil {
		return buf
	}
	// Guard slot (CanEquipInSlot -> SlotIndexType panics out of range).
	if !validEquipSlot(slot) {
		return buf
	}
	if validPartyMember(g, member) && g.Party[member].Equipped[slot] != ItemNone {
		buf = append(buf, EquipPickerRow{Unequip: true})
	}
	for _, st := range g.Inventory {
		if st.Count > 0 && CanEquipInSlot(st.Kind, slot) {
			buf = append(buf, EquipPickerRow{Kind: st.Kind, Count: st.Count})
		}
	}
	return buf
}

// EquipFromInventory equips one `kind` from the inventory into the member's
// slot, returning any displaced item (no net item loss). False/no-op if the
// member is out of range, the kind doesn't fit, or isn't in inventory.
func EquipFromInventory(g *GameState, member int, slot EquipSlotIndex, kind ItemKind) bool {
	if g == nil || !validPartyMember(g, member) {
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
	// Two-handers occupy BOTH hands: equipping one clears the other hand, and
	// equipping into a hand beside a two-hander clears that two-hander — else its
	// StatBonus would stack twice. The freed item routes back to inventory.
	if other, isHand := otherHand(slot); isHand {
		if ItemIsTwoHanded(kind) || ItemIsTwoHanded(m.Equipped[other]) {
			if freed := UnequipItem(m, other); freed != ItemNone {
				g.Inventory = AddItem(g.Inventory, freed, 1)
			}
		}
	}
	prev, equipOk := EquipItem(m, slot, kind)
	if !equipOk {
		// Equip refused — put the consumed item back.
		g.Inventory = AddItem(g.Inventory, kind, 1)
		return false
	}
	if prev != ItemNone {
		g.Inventory = AddItem(g.Inventory, prev, 1)
	}
	return true
}

// UnequipToInventory clears the slot and routes its item back to inventory.
// False when the slot was empty (or the member is out of range).
func UnequipToInventory(g *GameState, member int, slot EquipSlotIndex) bool {
	if g == nil || !validPartyMember(g, member) {
		return false
	}
	kind := UnequipItem(&g.Party[member], slot)
	if kind == ItemNone {
		return false
	}
	g.Inventory = AddItem(g.Inventory, kind, 1)
	return true
}

// ResetEquipPanels parks the Equipment-tab cursors and closes the picker — on
// overlay open and on switching INTO the tab, so re-entry has no stale picker.
func ResetEquipPanels(g *GameState) {
	g.EquipSlotCursor = 0
	g.EquipPickerOpen = false
	g.EquipPickerCursor = 0
}

// CloseEquipPicker dismisses the slot picker without touching the slot cursor.
func CloseEquipPicker(g *GameState) {
	g.EquipPickerOpen = false
	g.EquipPickerCursor = 0
}

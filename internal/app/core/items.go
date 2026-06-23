package core

import "crawler/internal/app/core/mapfile"

// ItemKind identifies an item. Stack-counted in GameState.Inventory: one
// ItemStack per kind with a Count.
type ItemKind int

const (
	ItemNone ItemKind = iota
	ItemCheese
	ItemBatJerky
	// Equipment: each carries a SlotType + per-stat bonuses; the Equipment
	// panel moves them between inventory and an equip slot.
	ItemIronSword
	ItemWoodenShield
	ItemLeatherCap
	ItemSilverRing
	ItemBrassAmulet
	// Sample weapons across the WeaponType taxonomy (weapons.go).
	ItemDagger
	ItemRapier
	ItemShortBow
	ItemSling
	ItemBattleAxe
	ItemWarHammer
	// APPEND new kinds HERE, never insert above — ItemKind serializes as its int
	// in save files, so a mid-enum insert renumbers later items and corrupts saves.
	ItemCrustOfBread // small healing consumable (Slot == SlotNone)
	ItemMagicPhial   // small MP-restore consumable (Slot == SlotNone)
	ItemThrowingKnives
	ItemCrossbow
	ItemArbalest

	itemKindCount // sentinel: ItemKind cardinality (assertAppendOnly coverage)
)

// init pins every ItemKind's serialized int value (the on-disk contract). A
// mid-enum insert renumbers later kinds and would corrupt saves; this panic
// trips at startup instead. APPEND a new kind, then add one pinned line here.
func init() {
	assertAppendOnly("ItemKind (renumbers saved items)", int(itemKindCount),
		ItemNone, ItemCheese, ItemBatJerky,
		ItemIronSword, ItemWoodenShield, ItemLeatherCap,
		ItemSilverRing, ItemBrassAmulet, ItemDagger,
		ItemRapier, ItemShortBow, ItemSling,
		ItemBattleAxe, ItemWarHammer, ItemCrustOfBread,
		ItemMagicPhial, ItemThrowingKnives, ItemCrossbow,
		ItemArbalest,
	)
}

// EquipmentSlotType classifies what slot an item can go into. SlotNone means a
// consumable (inventory-only). Hand items fit either hand; accessory items fit
// either accessory slot.
type EquipmentSlotType int

const (
	SlotNone EquipmentSlotType = iota
	SlotHand
	SlotArmor
	SlotAccessory
)

// EquipSlotIndex enumerates the five equip slots, indexing PartyMember.Equipped.
// Order is on-screen order (right hand, left hand, armor, two accessories).
type EquipSlotIndex int

const (
	EquipRightHand EquipSlotIndex = iota
	EquipLeftHand
	EquipArmor
	EquipAccessory1
	EquipAccessory2
	EquipSlotCount
)

// equipSlotInfo is the single source for each slot's on-screen label AND the
// EquipmentSlotType it accepts, indexed by EquipSlotIndex. One row per slot so
// label and gate-type can't drift; the init length-check catches a missing row.
var equipSlotInfo = [EquipSlotCount]struct {
	Label string
	Type  EquipmentSlotType
}{
	EquipRightHand:  {"R. HAND", SlotHand},
	EquipLeftHand:   {"L. HAND", SlotHand},
	EquipArmor:      {"ARMOR", SlotArmor},
	EquipAccessory1: {"ACCESSORY 1", SlotAccessory},
	EquipAccessory2: {"ACCESSORY 2", SlotAccessory},
}

func init() {
	// Guard against a zero-value (empty label / SlotNone) row slipping in when a
	// slot is added without its entry — CanEquipInSlot reads SlotIndexType, and a
	// SlotNone row would silently become un-equippable.
	for i := EquipSlotIndex(0); i < EquipSlotCount; i++ {
		if equipSlotInfo[i].Label == "" || equipSlotInfo[i].Type == SlotNone {
			panic("core: equipSlotInfo missing a row for an EquipSlotIndex")
		}
	}
}

// SlotIndexLabel returns the on-screen label for a slot index.
func SlotIndexLabel(i EquipSlotIndex) string {
	return equipSlotInfo[i].Label
}

// SlotIndexType reports the EquipmentSlotType a slot accepts — EquipItem's
// "can this item fit here?" gate.
func SlotIndexType(i EquipSlotIndex) EquipmentSlotType {
	return equipSlotInfo[i].Type
}

// ItemDefinition is the static registry data for an item kind. Effect is
// resolved by the battle consume path, not here.
type ItemDefinition struct {
	Kind ItemKind
	// Name shows in the log/picker AND is the on-disk identifier for chest loot
	// (via ItemKindByName), so renaming means re-saving any map that stocks it.
	Name string
	// HealAmount is HP restored on use; 0 = none.
	HealAmount int
	// MPAmount is MP restored on use (Magic Phial); 0 = none. The use paths skip
	// only when NEITHER axis would help the target.
	MPAmount int
	// Description is the flavor/tooltip line — authored but not yet read by any
	// UI. Keep populating it for the eventual tooltip.
	Description string
	// Slot is the equip slot type; SlotNone = consumable (usable, not equippable).
	Slot EquipmentSlotType
	// ArmorBonus/MDefBonus add to mitigation when equipped (via ApplyArmor /
	// ApplyMagicDefense).
	ArmorBonus int
	MDefBonus  int
	// StatBonus is the per-stat additive applied while equipped, indexed by Stat.
	StatBonus [StatCount]int
	// Weapon classifies a SlotHand weapon (WeaponType); WeaponNone for non-weapons
	// (unarmed STR melee). Drives the basic attack's to-hit stat and damage.
	Weapon WeaponType
	// TwoHanded marks a hand weapon occupying BOTH hands — can't co-exist with an
	// off-hand item (EquipFromInventory clears the other hand), else its StatBonus
	// would stack twice. SlotHand-only.
	TwoHanded bool
	// Price is the shop buy cost; 0 = not for sale (excluded from ShopCatalog and
	// SellableStacks). Sell-back is ShopSellPrice(Price).
	Price int
}

var itemDefinitions = []ItemDefinition{
	{Kind: ItemCheese, Name: "Morsel of Cheese", HealAmount: 4, Price: 6, Description: "A bite of stale cheese. Better than nothing."},
	{Kind: ItemBatJerky, Name: "Bat Jerky", HealAmount: 9, Price: 12, Description: "Stringy, oddly satisfying. A traveler's lunch."},
	{Kind: ItemCrustOfBread, Name: "Crust of Bread", HealAmount: 3, Price: 3, Description: "A dry heel of bread. A small bite back to your feet."},
	{Kind: ItemMagicPhial, Name: "Magic Phial", MPAmount: 8, Price: 14, Description: "A vial of cold blue draught. Restores a little MP."},

	// Equipment. Bonuses are modest — a starting kit, not a power spike.
	{Kind: ItemIronSword, Name: "Iron Sword", Description: "A plain iron longsword. +2 STR.",
		Slot:      SlotHand,
		Weapon:    WeaponSword, // heavy melee → STR
		Price:     40,
		StatBonus: [StatCount]int{StatSTR: 2}},
	{Kind: ItemWoodenShield, Name: "Wooden Shield", Description: "Plywood-grade. +2 Armor.",
		Slot:       SlotHand,
		Price:      30,
		ArmorBonus: 2},
	{Kind: ItemLeatherCap, Name: "Leather Cap", Description: "Worn leather. +1 Armor.",
		Slot:       SlotArmor,
		Price:      20,
		ArmorBonus: 1},
	{Kind: ItemSilverRing, Name: "Silver Ring", Description: "A nicked silver band. +1 DEX.",
		Slot:      SlotAccessory,
		Price:     25,
		StatBonus: [StatCount]int{StatDEX: 1}},
	{Kind: ItemBrassAmulet, Name: "Brass Amulet", Description: "A tarnished charm. +2 MDef.",
		Slot:      SlotAccessory,
		Price:     35,
		MDefBonus: 2},

	// Sample weapons. Weapon type picks the basic-attack stat; StatBonus is an
	// on-equip sweetener. The class is rendered from the data by equipBonusSummary.
	{Kind: ItemDagger, Name: "Dagger", Description: "A quick finesse blade. +1 DEX.",
		Slot:      SlotHand,
		Weapon:    WeaponDagger,
		Price:     25,
		StatBonus: [StatCount]int{StatDEX: 1}},
	{Kind: ItemRapier, Name: "Rapier", Description: "A slender thrusting sword. +1 DEX.",
		Slot:      SlotHand,
		Weapon:    WeaponRapier,
		Price:     35,
		StatBonus: [StatCount]int{StatDEX: 1}},
	{Kind: ItemShortBow, Name: "Short Bow", Description: "A simple short bow. +1 DEX.",
		Slot:      SlotHand,
		Weapon:    WeaponBow,
		Price:     35,
		StatBonus: [StatCount]int{StatDEX: 1}},
	{Kind: ItemSling, Name: "Sling", Description: "A leather sling.",
		Slot:   SlotHand,
		Weapon: WeaponSling,
		Price:  15},
	{Kind: ItemBattleAxe, Name: "Battle Axe", Description: "A broad battle axe. +1 STR.",
		Slot:      SlotHand,
		Weapon:    WeaponAxe,
		Price:     45,
		StatBonus: [StatCount]int{StatSTR: 1}},
	{Kind: ItemWarHammer, Name: "War Hammer", Description: "A heavy two-handed maul. +2 STR.",
		Slot:      SlotHand,
		Weapon:    WeaponHammer,
		TwoHanded: true, // fills both hands
		Price:     55,
		StatBonus: [StatCount]int{StatSTR: 2}},

	// Ranged weapons. Light (DEX) vs heavy (STR) mirrors the melee split; all
	// three reach flyers without the melee penalty (WeaponIsRanged).
	{Kind: ItemThrowingKnives, Name: "Throwing Knives", Description: "A bandolier of balanced knives. +1 DEX.",
		Slot:      SlotHand,
		Weapon:    WeaponThrowingKnives, // light ranged → DEX
		Price:     28,
		StatBonus: [StatCount]int{StatDEX: 1}},
	{Kind: ItemCrossbow, Name: "Crossbow", Description: "A spanned crossbow. +1 STR.",
		Slot:      SlotHand,
		Weapon:    WeaponCrossbow, // heavy ranged → STR
		Price:     45,
		StatBonus: [StatCount]int{StatSTR: 1}},
	{Kind: ItemArbalest, Name: "Arbalest", Description: "A heavy steel arbalest. Two-handed. +2 STR.",
		Slot:      SlotHand,
		Weapon:    WeaponArbalest, // heavy ranged → STR
		TwoHanded: true,
		Price:     60,
		StatBonus: [StatCount]int{StatSTR: 2}},
}

// ItemStack is one inventory slot: a kind plus a count. AddItem refuses to create
// ItemNone or non-positive stacks; zero-count entries are pruned later by
// ConsumeItem (drops the entry at zero) and filtered out by LiveStacks.
type ItemStack struct {
	Kind  ItemKind
	Count int
}

// itemByKind / itemByName are the O(1) lookup maps for itemDefinitions, built at
// init (per-frame picker render reads them).
var (
	itemByKind = BuildRegistry(itemDefinitions, func(d ItemDefinition) ItemKind { return d.Kind })
	itemByName = buildItemByName()
)

// reservedItemNames are tokens the .map chest parser uses as sentinels. An item
// named one of these would silently collide; the init guard panics instead.
var reservedItemNames = map[string]struct{}{
	mapfile.EmptyChestToken: {},
}

// Guard item names against the chest parser's sentinels (init side effect, so
// the test suite catches a collision when a new ItemDefinition is added).
func init() {
	for _, def := range itemDefinitions {
		if _, reserved := reservedItemNames[def.Name]; reserved {
			panic("core/items: item name " + def.Name + " is reserved by the mapfile chest parser")
		}
	}
}

// buildItemByName stays a one-off: it stores ItemKind under string keys, not the
// full def, so BuildRegistry (key→def) doesn't fit.
func buildItemByName() map[string]ItemKind {
	m := make(map[string]ItemKind, len(itemDefinitions))
	for _, def := range itemDefinitions {
		m[def.Name] = def.Kind
	}
	return m
}

// ItemInfo returns the definition for a kind, or a generic fallback for an
// unknown kind so the UI doesn't crash.
func ItemInfo(kind ItemKind) ItemDefinition {
	if def, ok := itemByKind[kind]; ok {
		return def
	}
	return ItemDefinition{Kind: kind, Name: "Unknown Item"}
}

// ItemInfoOk is the registry-validating sibling of ItemInfo: (zero, false) for
// an unregistered kind, so callers can reject one without string-comparing the
// "Unknown Item" fallback.
func ItemInfoOk(kind ItemKind) (ItemDefinition, bool) {
	def, ok := itemByKind[kind]
	return def, ok
}

// AllItems returns the registry in declaration order (defensive copy). The
// editor's chest-edit modal builds its add-rules table from it.
func AllItems() []ItemDefinition {
	out := make([]ItemDefinition, len(itemDefinitions))
	copy(out, itemDefinitions)
	return out
}

// ItemKindByName looks up a kind from the name used in EnemyDefinition.Item.
// Returns ItemNone on no match (caller decides what to do).
func ItemKindByName(name string) ItemKind {
	if kind, ok := itemByName[name]; ok {
		return kind
	}
	return ItemNone
}

// AddItem inserts a stack, merging with an existing matching-kind slot, and
// returns the updated slice. ItemNone is silently dropped.
func AddItem(inv []ItemStack, kind ItemKind, count int) []ItemStack {
	if kind == ItemNone || count <= 0 {
		return inv
	}
	for i := range inv {
		if inv[i].Kind == kind {
			inv[i].Count += count
			return inv
		}
	}
	return append(inv, ItemStack{Kind: kind, Count: count})
}

// ConsumeItem decrements one of the given kind, removing the entry at zero.
// Returns the updated slice and false if the item wasn't present.
func ConsumeItem(inv []ItemStack, kind ItemKind) ([]ItemStack, bool) {
	for i := range inv {
		if inv[i].Kind != kind {
			continue
		}
		if inv[i].Count <= 0 {
			return inv, false
		}
		inv[i].Count--
		if inv[i].Count == 0 {
			inv = append(inv[:i], inv[i+1:]...)
		}
		return inv, true
	}
	return inv, false
}

// LiveStacks returns the positive-count entries (no "0x" zombie rows). Both
// input and render call it so picker indices line up with drawn rows.
func LiveStacks(inv []ItemStack) []ItemStack {
	return LiveStacksInto(inv, nil)
}

// LiveStacksInto is the buffer-reusing form of LiveStacks (filters into `buf`,
// truncated first) for per-frame callers. Pass nil to allocate.
func LiveStacksInto(inv, buf []ItemStack) []ItemStack {
	return filterInto(buf, inv, func(s ItemStack) bool { return s.Count > 0 })
}

// LiveStackCount is the no-alloc count of positive-count entries, for callers
// that need only the row count.
func LiveStackCount(inv []ItemStack) int {
	n := 0
	for _, s := range inv {
		if s.Count > 0 {
			n++
		}
	}
	return n
}

// isConsumable reports whether a kind is a battle-usable consumable (no equip
// slot). Single home for "what's usable as an item?" — equipment shares the
// inventory but applyItem would consume it for 0 heal.
func isConsumable(kind ItemKind) bool {
	return ItemInfo(kind).Slot == SlotNone
}

// liveConsumable is the shared "usable consumable right now" predicate —
// positive count AND no equip slot. Single source for the battle Item menu.
func liveConsumable(s ItemStack) bool {
	return s.Count > 0 && isConsumable(s.Kind)
}

// ItemIsRestorative reports whether an item restores HP or MP — the single
// definition of "restorative" the use paths gate on.
func ItemIsRestorative(def ItemDefinition) bool {
	return def.HealAmount > 0 || def.MPAmount > 0
}

// ItemHelpsTarget reports whether using a restorative on m would do anything
// (restores HP and m isn't full, or MP and m isn't full). A non-restorative
// returns true (a deliberate action). Both use paths gate on this.
func ItemHelpsTarget(def ItemDefinition, m PartyMember) bool {
	if !ItemIsRestorative(def) {
		return true
	}
	hpUseful := def.HealAmount > 0 && m.HP < m.MaxHP
	mpUseful := def.MPAmount > 0 && m.MP < m.MaxMP
	return hpUseful || mpUseful
}

// MemberCanBeHealed is the "is an HP heal not wasted?" gate for out-of-battle
// target selection: alive (heals don't revive), not ingested, not full HP. The
// HP-only mirror of ItemHelpsTarget (no MP axis), so an HP-only heal picker
// doesn't accept an ally who's only low on MP.
func MemberCanBeHealed(m PartyMember) bool {
	return partyAvailable(m) && m.HP < m.MaxHP
}

// LiveConsumables returns the positive-count consumable entries — the battle
// Item menu's eligible set (equipment filtered out). Both input and render call
// it so picker indices line up with drawn rows.
func LiveConsumables(inv []ItemStack) []ItemStack {
	return LiveConsumablesInto(inv, nil)
}

// LiveConsumablesInto is the buffer-reusing form of LiveConsumables. Pass nil to allocate.
func LiveConsumablesInto(inv, buf []ItemStack) []ItemStack {
	return filterInto(buf, inv, liveConsumable)
}

// HasConsumable reports whether the inventory holds any usable consumable — the
// "Item" action's enabled-state check (no alloc).
func HasConsumable(inv []ItemStack) bool {
	for _, s := range inv {
		if liveConsumable(s) {
			return true
		}
	}
	return false
}

// ConsumableCount sums the positive-count consumable stacks — single source for
// the "Item xN" badge, matching LiveConsumables.
func ConsumableCount(inv []ItemStack) int {
	n := 0
	for _, s := range inv {
		if liveConsumable(s) {
			n += s.Count
		}
	}
	return n
}

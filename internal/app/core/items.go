package core

import "crawler/internal/app/core/mapfile"

// ItemKind identifies a consumable item. Items are stack-counted in
// GameState.Inventory: one ItemStack per kind with a Count.
type ItemKind int

const (
	ItemNone ItemKind = iota
	ItemCheese
	ItemBatJerky
	// Equipment items follow. Each one carries a SlotType +
	// per-stat bonuses on its ItemDefinition. Inventory stores them
	// like any other stack; the Equipment panel moves them between
	// inventory and an equip slot via the slot picker.
	ItemIronSword
	ItemWoodenShield
	ItemLeatherCap
	ItemSilverRing
	ItemBrassAmulet
	// Sample weapons exercising the WeaponType taxonomy (weapons.go):
	// DEX-governed light + ranged, STR-governed heavy. Stats are starter-
	// kit modest and easy to retune.
	ItemDagger
	ItemRapier
	ItemShortBow
	ItemSling
	ItemBattleAxe
	ItemWarHammer
	// New ItemKinds are appended HERE, at the end, not inserted above —
	// ItemKind serializes as its integer value in save files
	// (SaveData.Inventory), so inserting mid-enum would renumber every
	// later item and corrupt existing saves. Registry/display order is
	// the itemDefinitions slice order, which is independent of this value.
	ItemCrustOfBread // small healing consumable (Slot == SlotNone)
	ItemMagicPhial   // small MP-restore consumable (Slot == SlotNone)
	// Ranged weapons split light/heavy like the melee tier (weapons.go):
	// Throwing Knives are light DEX; the Crossbow + Arbalest are heavy STR.
	ItemThrowingKnives
	ItemCrossbow
	ItemArbalest
)

// init pins every ItemKind's serialized integer value. ItemKind is stored as
// its int in save files (SaveData.Inventory), so inserting a kind mid-enum
// renumbers every later kind and silently corrupts existing saves. These
// explicit literals are the on-disk contract: a mid-enum insert shifts a
// kind's iota value away from its pinned literal and trips this panic at
// startup, instead of corrupting saves silently. APPENDING a new kind at the
// end is the only safe edit — add the kind above, then one pinned line here.
func init() {
	pinned := [...]struct {
		kind ItemKind
		val  int
	}{
		{ItemNone, 0}, {ItemCheese, 1}, {ItemBatJerky, 2},
		{ItemIronSword, 3}, {ItemWoodenShield, 4}, {ItemLeatherCap, 5},
		{ItemSilverRing, 6}, {ItemBrassAmulet, 7}, {ItemDagger, 8},
		{ItemRapier, 9}, {ItemShortBow, 10}, {ItemSling, 11},
		{ItemBattleAxe, 12}, {ItemWarHammer, 13}, {ItemCrustOfBread, 14},
		{ItemMagicPhial, 15}, {ItemThrowingKnives, 16}, {ItemCrossbow, 17},
		{ItemArbalest, 18},
	}
	for _, p := range pinned {
		if int(p.kind) != p.val {
			panic("core: ItemKind serialization value drifted — never insert mid-enum (it renumbers saved items); append new kinds at the end and pin them in items.go's init")
		}
	}
}

// EquipmentSlotType classifies what equipment slot an item can go into.
// SlotNone marks a consumable (cheese, jerky) and means "inventory-only,
// can't be equipped." Hand items fit either hand slot; armor items fit
// the body slot; accessory items fit either accessory slot.
type EquipmentSlotType int

const (
	SlotNone EquipmentSlotType = iota
	SlotHand
	SlotArmor
	SlotAccessory
)

// EquipSlotIndex enumerates the five concrete equip slots on a party
// member. Used as the index into PartyMember.Equipped. Order is the
// on-screen order (right hand, left hand, armor, two accessories).
type EquipSlotIndex int

const (
	EquipRightHand EquipSlotIndex = iota
	EquipLeftHand
	EquipArmor
	EquipAccessory1
	EquipAccessory2
	EquipSlotCount
)

// equipSlotInfo is the single source for each equip slot's on-screen
// label AND the EquipmentSlotType it accepts, indexed by EquipSlotIndex.
// One row per slot so the label and the equip-gate type can't drift, and
// adding a slot is one row caught by the init length-check below (rather
// than two parallel switches that could each miss a case).
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
	// Sized [EquipSlotCount], so a missing/extra slot is a compile error;
	// guard against a zero-value (empty label / SlotNone) row slipping in
	// when a new slot is added without filling its entry — CanEquipInSlot
	// reads SlotIndexType, and a SlotNone row would silently become
	// un-equippable.
	for i := EquipSlotIndex(0); i < EquipSlotCount; i++ {
		if equipSlotInfo[i].Label == "" || equipSlotInfo[i].Type == SlotNone {
			panic("core: equipSlotInfo missing a row for an EquipSlotIndex")
		}
	}
}

// SlotIndexLabel returns the on-screen label for a slot index. Single
// seam so the panel and tooltips don't drift on naming.
func SlotIndexLabel(i EquipSlotIndex) string {
	return equipSlotInfo[i].Label
}

// SlotIndexType reports the EquipmentSlotType an equip slot accepts.
// Used by EquipItem to gate "can this item fit here?" — a Hand item
// goes in RightHand/LeftHand, an Armor item in Armor, etc.
func SlotIndexType(i EquipSlotIndex) EquipmentSlotType {
	return equipSlotInfo[i].Type
}

// ItemDefinition is the static registry data for an item kind. Effect is
// resolved by the consume path in the battle code, not here.
type ItemDefinition struct {
	Kind ItemKind
	// Name is what shows in the log and the picker UI. It's also the
	// on-disk identifier for chest loot: chest spawns in .map files name
	// their item by this string and AreaFromMapFile resolves it via
	// ItemKindByName, so renaming an item means re-saving any map that
	// stocks it. (Enemy steal loot is keyed by ItemKind now, not name.)
	Name string
	// HealAmount is the HP restored when used. 0 means "restores no HP".
	HealAmount int
	// MPAmount is the MP restored when used (the Magic Phial). 0 means
	// "restores no MP". An item may set either field (or, in principle,
	// both); the use paths skip the use only when NEITHER would help the
	// target. Mirrors HealAmount on the MP axis.
	MPAmount int
	// Description is the item's flavor/tooltip line. It's authored for every
	// item in itemDefinitions but NOT yet read by any UI — wire it into the
	// item picker / tooltip when that surface lands. (Authored-but-unconsumed,
	// not dead: keep populating it so the eventual tooltip has copy.)
	Description string
	// Slot is the equipment slot type this item fits into. SlotNone
	// means it's a consumable — usable from the battle Item action but
	// not equippable. Non-None items show up in the Equipment panel's
	// slot picker.
	Slot EquipmentSlotType
	// ArmorBonus and MDefBonus add to the wearer's mitigation when
	// this item is equipped. Both phys/magic flow through the same
	// helpers (ApplyArmor / ApplyMagicDefense), so equipping armor
	// effectively raises the corresponding cap.
	ArmorBonus int
	MDefBonus  int
	// StatBonus is the per-stat additive applied while this item is
	// equipped. Indexed by Stat (STR/DEX/INT/WIS/VIT/SPD); zero in any
	// slot means no contribution.
	StatBonus [StatCount]int
	// Weapon classifies a SlotHand weapon (see WeaponType / weaponSpecs).
	// WeaponNone for non-weapons — consumables, armor, accessories, and
	// off-hand items like a shield — which leaves the wielder unarmed
	// (STR melee) for basic-attack purposes. Drives which stat the basic
	// attack rolls to-hit and scales damage off.
	Weapon WeaponType
	// TwoHanded marks a hand weapon that occupies BOTH hands. A two-handed
	// weapon can't co-exist with any off-hand item: EquipFromInventory clears
	// the other hand when one is equipped (and clears the two-hander when an
	// off-hand item is equipped beside it). Without this, the same weapon
	// could sit in both hand slots and stack its StatBonus twice. Only
	// meaningful for SlotHand items; ignored elsewhere.
	TwoHanded bool
	// Price is the gold cost to buy this item at the shop. 0 means "not
	// for sale" — such an item never appears in the shop's Buy catalog
	// (ShopCatalog) and can't be sold back (SellableStacks filters it
	// out). Sell-back value is ShopSellPrice(Price). Every starter item
	// carries a price so the shop has stock and the inventory is liquid.
	Price int
}

var itemDefinitions = []ItemDefinition{
	{Kind: ItemCheese, Name: "Morsel of Cheese", HealAmount: 4, Price: 6, Description: "A bite of stale cheese. Better than nothing."},
	// Bat jerky heals more than cheese — bats are tougher to fight and
	// harder to steal from, so the loot pays off the difficulty.
	{Kind: ItemBatJerky, Name: "Bat Jerky", HealAmount: 9, Price: 12, Description: "Stringy, oddly satisfying. A traveler's lunch."},
	// Crust of bread — the humblest heal, the party's starting ration.
	// Smaller than cheese on purpose (it's a crust).
	{Kind: ItemCrustOfBread, Name: "Crust of Bread", HealAmount: 3, Price: 3, Description: "A dry heel of bread. A small bite back to your feet."},
	// Magic Phial — the MP counterpart to the food rations. Restores a small
	// pool of MP so a caster isn't stranded between fights. Consumable
	// (Slot == SlotNone) so it lists in the Item picker like the food.
	{Kind: ItemMagicPhial, Name: "Magic Phial", MPAmount: 8, Price: 14, Description: "A vial of cold blue draught. Restores a little MP."},

	// Equipment. Bonuses are intentionally modest so the very-basic
	// system reads as a starting kit rather than a power spike. STR
	// from a sword, defensive layering from a shield + cap, a small
	// stat ring, an MDef amulet.
	{Kind: ItemIronSword, Name: "Iron Sword", Description: "A plain iron longsword. +2 STR.",
		Slot:      SlotHand,
		Weapon:    WeaponSword, // heavy melee → STR-governed to-hit + damage
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

	// Sample weapons. The Weapon type picks which stat the basic attack
	// rolls to-hit + scales damage off (DEX for light/ranged, STR for
	// heavy); the StatBonus is a small on-equip sweetener. Tune freely.
	// The weapon class (DEX/STR, ranged) is rendered from the data by
	// equipBonusSummary, so it stays out of the prose; the "+N STAT"
	// sweetener is kept in the description to match the other starter
	// items' convention.
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
		TwoHanded: true, // a two-handed maul fills both hands — no off-hand item beside it
		Price:     55,
		StatBonus: [StatCount]int{StatSTR: 2}},

	// Ranged weapons. Light (DEX) vs heavy (STR) mirrors the melee split:
	// throwing knives are a quick finesse throw; the crossbow + arbalest
	// need strength to span and steady, so they roll to-hit + basic-attack
	// damage off STR. The arbalest is a two-handed siege piece. All three
	// reach flyers without the melee-vs-flyer penalty (WeaponIsRanged).
	{Kind: ItemThrowingKnives, Name: "Throwing Knives", Description: "A bandolier of balanced knives. +1 DEX.",
		Slot:      SlotHand,
		Weapon:    WeaponThrowingKnives, // light ranged → DEX-governed
		Price:     28,
		StatBonus: [StatCount]int{StatDEX: 1}},
	{Kind: ItemCrossbow, Name: "Crossbow", Description: "A spanned crossbow. +1 STR.",
		Slot:      SlotHand,
		Weapon:    WeaponCrossbow, // heavy ranged → STR-governed
		Price:     45,
		StatBonus: [StatCount]int{StatSTR: 1}},
	{Kind: ItemArbalest, Name: "Arbalest", Description: "A heavy steel arbalest. Two-handed. +2 STR.",
		Slot:      SlotHand,
		Weapon:    WeaponArbalest, // heavy ranged → STR-governed
		TwoHanded: true,
		Price:     60,
		StatBonus: [StatCount]int{StatSTR: 2}},
}

// ItemStack is one inventory slot: a kind plus how many the player owns.
// Empty kinds (ItemNone) and zero counts are pruned by addItem so the
// slice doesn't accumulate dead entries.
type ItemStack struct {
	Kind  ItemKind
	Count int
}

// itemByKind / itemByName are the O(1) lookup maps for itemDefinitions,
// built once at init. ItemInfo / ItemKindByName get called from per-frame
// item picker render — the map matches the partyClassByID / skillByID
// pattern in party.go.
var (
	itemByKind = BuildRegistry(itemDefinitions, func(d ItemDefinition) ItemKind { return d.Kind })
	itemByName = buildItemByName()
)

// reservedItemNames are tokens the .map chest parser uses as sentinels
// ("(empty)" for a no-loot chest row). Any item with one of these names
// would silently collide with the parser — panicking at init is far
// less surprising than a future "Chest authored with one item but
// loads with zero" bug after someone added an item named "(empty)".
var reservedItemNames = map[string]struct{}{
	mapfile.EmptyChestToken: {},
}

// Guard the item registry against names that collide with the chest
// parser's sentinels. Runs as a package-init side effect so the test
// suite catches a collision the moment a new ItemDefinition is added.
func init() {
	for _, def := range itemDefinitions {
		if _, reserved := reservedItemNames[def.Name]; reserved {
			panic("core/items: item name " + def.Name + " is reserved by the mapfile chest parser")
		}
	}
}

// buildItemByName stays a one-off because it stores ItemKind (a small
// value) under string keys, not the full ItemDefinition — BuildRegistry
// assumes key→def, this is key→key-of-def.
func buildItemByName() map[string]ItemKind {
	m := make(map[string]ItemKind, len(itemDefinitions))
	for _, def := range itemDefinitions {
		m[def.Name] = def.Kind
	}
	return m
}

// ItemInfo returns the definition for a kind, or a generic fallback for an
// unknown kind so the UI doesn't crash on a future item not in the registry.
func ItemInfo(kind ItemKind) ItemDefinition {
	if def, ok := itemByKind[kind]; ok {
		return def
	}
	return ItemDefinition{Kind: kind, Name: "Unknown Item"}
}

// ItemInfoOk is the registry-validating sibling of ItemInfo. Returns
// (definition, true) when the kind is registered, (zero, false) when
// it isn't — callers that need to reject unknown kinds (the .map
// writer's chest serializer) use this instead of brittle-comparing
// against the "Unknown Item" fallback string.
func ItemInfoOk(kind ItemKind) (ItemDefinition, bool) {
	def, ok := itemByKind[kind]
	return def, ok
}

// AllItems returns the item registry in declaration order. Used by the
// editor's chest-edit modal to build its add-rules table at init —
// adding a new item kind is one row in itemDefinitions and the modal
// picks up a hotkey automatically. Returns a defensive copy so callers
// can't mutate the registry.
func AllItems() []ItemDefinition {
	out := make([]ItemDefinition, len(itemDefinitions))
	copy(out, itemDefinitions)
	return out
}

// ItemKindByName looks up an item kind from the human-readable name used in
// EnemyDefinition.Item. Returns ItemNone if the name doesn't match a known
// item — caller decides what to do with that (drop the steal silently, log
// a debug warning, etc.).
func ItemKindByName(name string) ItemKind {
	if kind, ok := itemByName[name]; ok {
		return kind
	}
	return ItemNone
}

// AddItem inserts a stack into the inventory, merging with an existing
// matching-kind slot if one's already there. Returns the updated slice so
// the caller can assign back. ItemNone is silently dropped — keeps the
// caller from having to guard each call site.
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

// ConsumeItem decrements one of the given kind from the inventory, removing
// the stack entry when its count hits zero. Returns the updated slice and a
// bool — false if the item wasn't in the inventory, so callers can refuse
// the action without separately checking HasItem.
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

// LiveStacks returns just the inventory entries with a positive count, so
// the picker UI never shows a "0x …" zombie row. Both the input layer
// (battle/menu.go) and the renderer (render/battle.go) call into this so
// the indices used to navigate the picker line up exactly with the rows
// being drawn.
func LiveStacks(inv []ItemStack) []ItemStack {
	return LiveStacksInto(inv, nil)
}

// LiveStacksInto is the buffer-reusing form of LiveStacks: it filters
// into `buf` (truncated first) and returns it, so a per-frame caller
// (the panels Items tab, the battle item menu) keeps one scratch slice
// across frames instead of allocating a fresh slice each frame. Pass nil
// to allocate. The filtered content is identical to LiveStacks, so picker
// indices still line up with drawn rows regardless of which form is used.
func LiveStacksInto(inv, buf []ItemStack) []ItemStack {
	return filterInto(buf, inv, func(s ItemStack) bool { return s.Count > 0 })
}

// LiveStackCount returns how many inventory entries have a positive
// count, without allocating — for callers (cursor-wrap math) that only
// need the row count, not the rows.
func LiveStackCount(inv []ItemStack) int {
	n := 0
	for _, s := range inv {
		if s.Count > 0 {
			n++
		}
	}
	return n
}

// InventoryEmpty is a convenience for the "Item" menu option's enabled
// state: returns true when there are no usable items at all. Walks the
// slice directly (cheaper than allocating via LiveStacks just to check
// length) but shares the same "positive count wins" rule.
func InventoryEmpty(inv []ItemStack) bool {
	return LiveStackCount(inv) == 0
}

// isConsumable reports whether a kind is a battle-usable consumable: a
// positive-count item with no equip slot (cheese, jerky). Equipment
// (Slot != SlotNone) shares the global inventory but can't be "used" in
// combat — applyItem would consume it for 0 heal. The inverse of the
// Equipment-tab picker's CanEquipInSlot / Slot != SlotNone test, kept
// here so the "what's usable as an item?" rule lives in one place.
func isConsumable(kind ItemKind) bool {
	return ItemInfo(kind).Slot == SlotNone
}

// liveConsumable is the shared "this stack is a usable consumable right now"
// predicate — positive count AND no equip slot. Single source for the battle
// Item menu's eligible set, its enabled-state check, and its count badge, so
// the three can't drift on what "consumable" means.
func liveConsumable(s ItemStack) bool {
	return s.Count > 0 && isConsumable(s.Kind)
}

// ItemHelpsTarget reports whether using a restorative item on m would do
// anything — it restores HP and m isn't at full HP, OR restores MP and m
// isn't at full MP. A non-restorative item (HealAmount==0 && MPAmount==0)
// returns true (using it is a deliberate action, not a wasted heal). Both
// use paths — battle applyItem and explore applyUseToMember — gate on this so
// the "don't burn a restorative on a target full on what it gives" rule lives
// in one place instead of duplicated per call site.
// ItemIsRestorative reports whether an item restores HP or MP (vs equipment or
// a pure-flavor consumable). The single definition of "restorative" — the
// HealAmount/MPAmount field set that the use paths gate on — so the rule lives
// in one place instead of being open-coded per call site.
func ItemIsRestorative(def ItemDefinition) bool {
	return def.HealAmount > 0 || def.MPAmount > 0
}

func ItemHelpsTarget(def ItemDefinition, m PartyMember) bool {
	if !ItemIsRestorative(def) {
		return true // not a restorative — using it isn't "wasted"
	}
	hpUseful := def.HealAmount > 0 && m.HP < m.MaxHP
	mpUseful := def.MPAmount > 0 && m.MP < m.MaxMP
	return hpUseful || mpUseful
}

// MemberCanBeHealed is the canonical "is an HP heal not wasted on this ally?"
// gate for out-of-battle heal-target selection: the member must be alive
// (HP > 0 — heals don't revive), not ingested by a mantrap (out of reach,
// same skip HealMember applies), and not already at full HP. Mirrors the HP
// branch of ItemHelpsTarget but with no MP axis, so callers that only offer
// an HP heal (heal-skill target pickers) don't accept an ally who's full on
// HP just because they're low on MP. ItemHelpsTarget is left as-is — its MP
// axis is load-bearing for MP-restoring items.
func MemberCanBeHealed(m PartyMember) bool {
	return m.HP > 0 && !m.Ingested && m.HP < m.MaxHP
}

// LiveConsumables returns the positive-count inventory entries that are
// consumables — the battle Item menu's eligible set. Equipment is filtered
// out so the picker can't list (and applyItem can't destroy) gear. Both
// the input layer (updateItemMenu) and the renderer (drawItemMenuList)
// filter through this so picker indices line up with drawn rows.
func LiveConsumables(inv []ItemStack) []ItemStack {
	return LiveConsumablesInto(inv, nil)
}

// LiveConsumablesInto is the buffer-reusing form of LiveConsumables
// (mirrors LiveStacksInto) for the per-frame renderer. Pass nil to allocate.
func LiveConsumablesInto(inv, buf []ItemStack) []ItemStack {
	return filterInto(buf, inv, liveConsumable)
}

// HasConsumable reports whether the inventory holds any battle-usable
// consumable — the enabled-state check for the "Item" action. Walks
// directly (no alloc), same rule as LiveConsumables. Replaces a bare
// InventoryEmpty check, which counted equipment as "have items."
func HasConsumable(inv []ItemStack) bool {
	for _, s := range inv {
		if liveConsumable(s) {
			return true
		}
	}
	return false
}

// ConsumableCount sums the positive-count consumable stacks — the single
// source for "how many usable items" so the battle action menu's "Item xN"
// badge can't drift from the LiveConsumables picker's definition of consumable.
func ConsumableCount(inv []ItemStack) int {
	n := 0
	for _, s := range inv {
		if liveConsumable(s) {
			n += s.Count
		}
	}
	return n
}

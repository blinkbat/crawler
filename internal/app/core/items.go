package core

import (
	"fmt"

	"crawler/internal/app/core/mapfile"
)

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
	ItemHealthPotion   // HP-restore consumable (Slot == SlotNone)
	ItemMagicalBerries // HP-restore + a little satiety (Slot == SlotNone)

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
		ItemArbalest, ItemHealthPotion, ItemMagicalBerries,
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
	// SatietyGain is the Hunger restored on eating (FeedMember); 0 = not food. Food
	// may ALSO carry a HealAmount — both apply on use (satiety first, so a meal big
	// enough to lift Starving lets the heal land).
	SatietyGain int
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
	// CONTRACT: bonuses fold into EffectiveStats (combat math — damage/accuracy/magic/
	// defenses/turn order) but NOT into the derived pools MaxHP/MaxMP, which are
	// computed from BASE stats only (MaxHPFor/MaxMPFor at level-up/load, never on
	// equip). So a StatVIT bonus is effectively INERT (VIT's only consumer is MaxHP),
	// and a StatINT bonus raises magic damage but NOT the MP pool. Shipped gear grants
	// only STR/DEX (which fully work); granting VIT/INT is a design decision (recompute
	// pools on equip + clamp) that hasn't been made — don't add such an item expecting
	// the pool to grow without wiring that first.
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
	// Consumables. Foods carry SatietyGain (and a small heal); the phial/potion/
	// berries are the dedicated restoratives.
	{Kind: ItemCheese, Name: "Morsel of Cheese", HealAmount: 4, SatietyGain: 60, Price: 6,
		Description: "A waxed nub of cheese, sweated soft and gone sharp at the rind. Hardly a feast, but it quiets a growling belly for a while."},
	{Kind: ItemBatJerky, Name: "Bat Jerky", HealAmount: 9, SatietyGain: 150, Price: 12,
		Description: "Salt-cured strips of cave bat — chewy, smoky, and shockingly filling. A whole traveler's lunch wrapped in one greasy fistful."},
	{Kind: ItemCrustOfBread, Name: "Crust of Bread", HealAmount: 3, SatietyGain: 90, Price: 3,
		Description: "The dry heel of a loaf, hard at the edges and dusted with grit. Gnaw it long enough and it fills a corner of an empty stomach."},
	{Kind: ItemMagicPhial, Name: "Magic Phial", MPAmount: 8, Price: 14,
		Description: "A thimble of cold blue draught that beads like frost on the glass. One bitter swallow and spent mana comes trickling back."},
	{Kind: ItemHealthPotion, Name: "Health Potion", HealAmount: 14, Price: 18,
		Description: "A stoppered vial of ruby tonic, warm against the palm and tasting of iron and crushed herbs. It knits cuts closed as you drink."},
	{Kind: ItemMagicalBerries, Name: "Sundew Berries", HealAmount: 8, SatietyGain: 45, Price: 16,
		Description: "A cupped handful of dusk-glowing berries off a deep-grove bramble. They burst sweet on the tongue and mend you as they go down."},

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

// ItemKindByName looks up a kind from a chest-loot name token (areas.go chest loading).
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
	return countWhere(inv, func(s ItemStack) bool { return s.Count > 0 })
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
	return def.HealAmount > 0 || def.MPAmount > 0 || def.SatietyGain > 0
}

// ItemHelpsTarget reports whether using a restorative on m would do anything
// (restores HP and m isn't full, or MP and m isn't full). A non-restorative
// returns true (a deliberate action). Both use paths gate on this.
func ItemHelpsTarget(def ItemDefinition, m PartyMember) bool {
	if !ItemIsRestorative(def) {
		return true
	}
	// A starving member can't gain HP by any means but food, so a pure HP-heal does
	// NOT help them — but food still does (satietyUseful), and feeding lifts Starving
	// so the same item's heal lands afterward.
	// HP/MP restore require partyAvailable (mirrors restorativeDeltas' gates), so the
	// "helps" verdict matches what ApplyRestorative can actually deliver — a downed /
	// ingested member gains nothing from a heal. Feeding has no availability gate.
	hpUseful := def.HealAmount > 0 && m.HP < m.MaxHP && partyAvailable(m) && !MemberStarving(m)
	mpUseful := def.MPAmount > 0 && m.MP < m.MaxMP && partyAvailable(m)
	satietyUseful := def.SatietyGain > 0 && m.Hunger > 0
	return hpUseful || mpUseful || satietyUseful
}

// RestorativeResult reports what a consumed restorative actually applied
// (post-clamp), so call sites can log / flash by the real amounts.
type RestorativeResult struct {
	HP, MP, Satiety int
}

// ApplyRestorative applies a consumable's def to m in the canonical order — feed
// first (a big enough meal lifts Starving so the item's own heal can land), then
// heal HP, then restore MP — and returns the actual amounts. The single home for
// the restorative effect sequence; the explore and battle item paths both use it
// so the ordering can't drift between them.
func ApplyRestorative(m *PartyMember, def ItemDefinition) RestorativeResult {
	if m == nil {
		return RestorativeResult{}
	}
	// Compute the exact deltas once, then apply them — the mutation can't diverge from
	// the preview because both read the SAME restorativeDeltas source.
	res := restorativeDeltas(*m, def)
	m.Hunger -= res.Satiety // deltas already clamp (see restorativeDeltas)
	m.HP += res.HP
	m.MP += res.MP
	return res
}

// PreviewRestorative computes what def WOULD restore for m WITHOUT mutating it — the
// non-applying twin of ApplyRestorative, so the item-target UI can show the projected
// +HP/+MP/feed before the player commits. Same source as the apply path, so they can't drift.
func PreviewRestorative(m PartyMember, def ItemDefinition) RestorativeResult {
	return restorativeDeltas(m, def)
}

// restorativeDeltas is the single source for what def restores on m — the clamped,
// post-gate HP/MP/Satiety amounts. Both ApplyRestorative (which then mutates by these
// deltas) and PreviewRestorative consume it, so the projected and applied results are
// identical by construction. Feed-first ordering: the local Hunger is dropped by the
// modeled meal BEFORE the starving-gated HP check, so a meal big enough to lift
// Starving lets the same item's heal land. Per-axis gates mirror the FeedMember/
// HealMember/RestoreMP helpers: feeding has NO availability gate (a downed member can
// still be fed), while HP/MP restore require partyAvailable.
func restorativeDeltas(m PartyMember, def ItemDefinition) RestorativeResult {
	sat := 0
	if def.SatietyGain > 0 {
		sat = min(def.SatietyGain, m.Hunger) // FeedMember floors Hunger at 0
		m.Hunger -= sat                      // model the feed on the local copy for the gate below
	}
	hp := 0
	if def.HealAmount > 0 && partyAvailable(m) && !MemberStarving(m) {
		hp = max(min(def.HealAmount, m.MaxHP-m.HP), 0)
	}
	mp := 0
	if def.MPAmount > 0 && partyAvailable(m) {
		mp = max(min(def.MPAmount, m.MaxMP-m.MP), 0)
	}
	return RestorativeResult{HP: hp, MP: mp, Satiety: sat}
}

// ItemUseMessage formats the consumed-item log line by what it restored (HP/MP/
// both/neither). res carries ACTUAL post-clamp amounts so the log can't overclaim.
// Shared by the battle and explore item-use paths.
func ItemUseMessage(targetName string, def ItemDefinition, res RestorativeResult) string {
	switch {
	case res.HP > 0 && res.MP > 0:
		return fmt.Sprintf("%s uses %s (+%d HP, +%d MP).", targetName, def.Name, res.HP, res.MP)
	case res.HP > 0 && res.Satiety > 0:
		return fmt.Sprintf("%s eats %s (+%d HP, heals %s).", targetName, def.Name, res.HP, SatietyHungerPhrase(res.Satiety))
	case res.HP > 0:
		return fmt.Sprintf("%s eats %s (+%d HP).", targetName, def.Name, res.HP)
	case res.MP > 0:
		return fmt.Sprintf("%s drinks %s (+%d MP).", targetName, def.Name, res.MP)
	case res.Satiety > 0:
		return fmt.Sprintf("%s eats %s — heals %s.", targetName, def.Name, SatietyHungerPhrase(res.Satiety))
	default:
		// Name the recipient like the +HP/+MP branches (the item lands on the target).
		return fmt.Sprintf("%s uses %s.", targetName, def.Name)
	}
}

// RestorativeUseCategory is LogHeal when a restorative landed anything, else
// LogInfo (a no-op use reads as neutral). Shared by both item-use paths.
func RestorativeUseCategory(res RestorativeResult) LogCategory {
	if res.HP > 0 || res.MP > 0 || res.Satiety > 0 {
		return LogHeal
	}
	return LogInfo
}

// MemberCanBeHealed is the "is an HP heal not wasted?" gate for out-of-battle
// target selection: alive (heals don't revive), not ingested, not full HP, not
// starving (starving blocks all HP recovery — only food helps). The HP-only mirror
// of ItemHelpsTarget (no MP axis), so an HP-only heal picker doesn't accept an ally
// who's only low on MP.
func MemberCanBeHealed(m PartyMember) bool {
	return partyAvailable(m) && m.HP < m.MaxHP && !MemberStarving(m)
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

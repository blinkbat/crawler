package core

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
)

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

// SlotIndexLabel returns the on-screen label for a slot index. Single
// seam so the panel and tooltips don't drift on naming.
func SlotIndexLabel(i EquipSlotIndex) string {
	switch i {
	case EquipRightHand:
		return "R. HAND"
	case EquipLeftHand:
		return "L. HAND"
	case EquipArmor:
		return "ARMOR"
	case EquipAccessory1:
		return "ACCESSORY 1"
	case EquipAccessory2:
		return "ACCESSORY 2"
	default:
		panic("core: SlotIndexLabel missing case for EquipSlotIndex")
	}
}

// SlotIndexType reports the EquipmentSlotType an equip slot accepts.
// Used by EquipItem to gate "can this item fit here?" — a Hand item
// goes in RightHand/LeftHand, an Armor item in Armor, etc.
func SlotIndexType(i EquipSlotIndex) EquipmentSlotType {
	switch i {
	case EquipRightHand, EquipLeftHand:
		return SlotHand
	case EquipArmor:
		return SlotArmor
	case EquipAccessory1, EquipAccessory2:
		return SlotAccessory
	default:
		// Load-bearing: CanEquipInSlot gates equipping on this result, so
		// a new slot that forgets a case here would silently become
		// un-equippable (SlotNone matches no item). Fail loudly instead.
		panic("core: SlotIndexType missing case for EquipSlotIndex")
	}
}

// ItemDefinition is the static registry data for an item kind. Effect is
// resolved by the consume path in the battle code, not here.
type ItemDefinition struct {
	Kind ItemKind
	// Name is what shows in the log and the picker UI. Match the strings
	// in EnemyDefinition.Item exactly so steal pickups can find their
	// item kind by name.
	Name string
	// HealAmount is the HP restored when used in battle. 0 means "non-
	// healing item" (none yet, but we keep the field general).
	HealAmount int
	// Description is reserved for a future tooltip. Fill it in when we add
	// item descriptions in the UI.
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
		Price:     55,
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
	"(empty)": {},
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
	buf = buf[:0]
	for _, s := range inv {
		if s.Count > 0 {
			buf = append(buf, s)
		}
	}
	return buf
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
	for _, s := range inv {
		if s.Count > 0 {
			return false
		}
	}
	return true
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
	buf = buf[:0]
	for _, s := range inv {
		if s.Count > 0 && isConsumable(s.Kind) {
			buf = append(buf, s)
		}
	}
	return buf
}

// HasConsumable reports whether the inventory holds any battle-usable
// consumable — the enabled-state check for the "Item" action. Walks
// directly (no alloc), same rule as LiveConsumables. Replaces a bare
// InventoryEmpty check, which counted equipment as "have items."
func HasConsumable(inv []ItemStack) bool {
	for _, s := range inv {
		if s.Count > 0 && isConsumable(s.Kind) {
			return true
		}
	}
	return false
}

package core

import "math/rand"

// Gold, loot drops, and the shop catalog. The economy loop: enemies pay
// out gold + roll item drops on defeat (AwardBattleLoot, called from
// winBattle), and the player will spend gold at a shop once openShop is wired
// to a merchant/tile (currently no entry point). Item prices live on
// ItemDefinition.Price; per-enemy gold + drop tables live on EnemyDefinition.

// ItemDrop is one possible loot item an enemy yields on death, with the
// per-defeat probability it actually drops. Distinct from the steal Item
// (robbed mid-fight, one per enemy): drops land in the shared inventory on
// victory. Chance rides the standard [0, 1] contract enforced in
// enemies.go's init.
type ItemDrop struct {
	Kind   ItemKind
	Chance float64
}

// ShopTab indexes the shop overlay's two columns: the Buy catalog and the
// Sell-back list of the player's own inventory.
type ShopTab int

const (
	ShopTabBuy ShopTab = iota
	ShopTabSell
	// ShopTabCount is the wrap modulus for paging the shop's tabs.
	ShopTabCount
)

// ShopTabLabel is the on-screen header for a shop tab.
func ShopTabLabel(t ShopTab) string {
	switch t {
	case ShopTabBuy:
		return "Buy"
	case ShopTabSell:
		return "Sell"
	default:
		// Mirror PanelTabLabel: a new ShopTab that forgets a label fails
		// loudly instead of silently rendering "?" in the tab strip.
		panic("core: ShopTabLabel missing case for ShopTab")
	}
}

// ShopSellPrice is the gold recovered for selling one unit of an item:
// its catalog Price / ShopSellDivisor, floored at 1 so a priced item is
// always worth something. Items with no Price (Price <= 0) aren't sellable
// — callers gate on SellableStacks, which filters them out.
func ShopSellPrice(price int) int {
	if price <= 0 {
		return 0
	}
	if part := price / ShopSellDivisor; part > 0 {
		return part
	}
	return 1
}

// shopCatalogCache holds the priced-item list, computed once on first use.
// The item registry is immutable, so the catalog never changes for the
// life of the process — the shop overlay queries it every frame while open
// (input + render), so caching avoids re-scanning + re-allocating each
// frame. Read-only: callers (shopRowCount, buyShopItem, shopRows) only read.
var shopCatalogCache []ItemDefinition

// ShopCatalog returns every purchasable item — registry entries with a
// positive Price — in declaration order. Drives the shop's Buy list, so a
// new priced item in items.go shows up for sale automatically. Returns the
// shared cached slice; treat it as read-only.
func ShopCatalog() []ItemDefinition {
	if shopCatalogCache == nil {
		out := make([]ItemDefinition, 0, len(itemDefinitions))
		for _, def := range itemDefinitions {
			if itemForSale(def) {
				out = append(out, def)
			}
		}
		shopCatalogCache = out
	}
	return shopCatalogCache
}

// itemForSale is the shared "this item is buyable / sellable" predicate —
// a positive Price (0 == not for sale, per ItemDefinition.Price). Single
// source for the buy catalog and the sell-list filters so they can't drift.
func itemForSale(def ItemDefinition) bool { return def.Price > 0 }

// sellableStack is the inventory-stack form of itemForSale: a positive-count
// stack whose item is for sale.
func sellableStack(s ItemStack) bool { return s.Count > 0 && itemForSale(ItemInfo(s.Kind)) }

// SellableStacks returns the inventory stacks the player can sell — live
// stacks whose item carries a positive Price. The shop's Sell list reads
// this so cursor rows line up with drawn rows (mirrors LiveStacks).
func SellableStacks(inv []ItemStack) []ItemStack {
	return SellableStacksInto(inv, nil)
}

// SellableStacksInto is the buffer-reusing form of SellableStacks (mirrors
// LiveStacksInto): it filters into `buf` (truncated first) and returns it, so
// the per-frame Sell-tab caller keeps one scratch slice across frames instead
// of allocating each frame. Pass nil to allocate. The filtered content is
// identical to SellableStacks, so cursor rows still line up with drawn rows.
func SellableStacksInto(inv, buf []ItemStack) []ItemStack {
	return filterInto(buf, inv, sellableStack)
}

// SellableCount is the no-alloc count of sellable stacks — same predicate
// as SellableStacks without materializing the slice. The shop's row-count
// path runs every frame the Sell tab is open and only needs the length,
// so it calls this instead of len(SellableStacks(...)) (mirrors
// LiveStackCount vs LiveStacks).
func SellableCount(inv []ItemStack) int {
	n := 0
	for _, s := range inv {
		if sellableStack(s) {
			n++
		}
	}
	return n
}

// AwardBattleLoot grants the defeated pack's gold + item drops to the
// party. Gold is the sum of a uniform roll in each member's [GoldMin,
// GoldMax]; item drops roll each member's Drops table independently. Adds
// directly to g.Gold / g.Inventory and returns the totals for the battle
// log. Called from winBattle right after AwardBattleXP; returns zero when
// no pack is engaged (BattleMembers reports the active pack or nil).
//
// Only DEFEATED (!Alive) members pay out — you loot what you killed. At the
// sole caller (winBattle) every member is already dead so this changes
// nothing today, but it makes the award correct if ever called with a
// partially-alive pack rather than over-paying for survivors.
func AwardBattleLoot(g *GameState) (gold int, drops []ItemKind) {
	members := BattleMembers(g)
	if len(members) == 0 {
		return 0, nil
	}
	rng := g.Rand()
	for _, m := range members {
		// Pay out anything that's dead by EITHER measure. Death normally clears
		// Alive (the canonical flag), and the custom-enemy loader refuses HP<=0
		// rows, so the HP guard is belt-and-suspenders: it ensures a degenerate
		// "alive corpse" (HP<=0 yet Alive) still yields its gold/drops on a win
		// rather than being silently skipped as if it were a living survivor.
		if m.Alive && m.HP > 0 {
			continue
		}
		def := EnemyInfoFor(m)
		gold += rollGold(rng, def.GoldMin, def.GoldMax)
		for _, d := range def.Drops {
			if RollChance(rng, d.Chance) {
				drops = append(drops, d.Kind)
			}
		}
	}
	if gold > 0 {
		g.Gold += gold
	}
	for _, kind := range drops {
		g.Inventory = AddItem(g.Inventory, kind, 1)
	}
	return gold, drops
}

// rollGold returns a uniform int in [lo, hi], tolerant of unset (0, 0) and
// inverted bounds so an authoring slip can't panic the loot award.
//
// Degenerate-bounds policy: SWAP inverted bounds, then clamp the result to
// >= 0 (an authoring slip can't pay out negative gold). This intentionally
// differs from its two siblings — RandRangeI (util.go) just returns lo on
// hi <= lo, and rollDuration (party.go) returns 0 on min <= 0 || max < min.
func rollGold(rng *rand.Rand, lo, hi int) int {
	if hi < lo {
		lo, hi = hi, lo
	}
	if hi <= 0 {
		return 0
	}
	if lo < 0 {
		lo = 0
	}
	return RandRangeI(rng, lo, hi)
}

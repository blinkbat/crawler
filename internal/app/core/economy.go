package core

// Gold, loot drops, and the shop catalog. Enemies pay gold + roll item drops on
// defeat (AwardBattleLoot); prices on ItemDefinition.Price, gold/drops on EnemyDefinition.

// ItemDrop is one loot item an enemy yields on death, with its per-defeat drop
// probability (Chance is [0, 1], enforced in enemies.go init).
type ItemDrop struct {
	Kind   ItemKind
	Chance float64
}

// ShopTab indexes the shop overlay's two columns (Buy catalog, Sell-back list).
type ShopTab int

const (
	ShopTabBuy ShopTab = iota
	ShopTabSell
	// ShopTabCount is the wrap modulus for paging the tabs.
	ShopTabCount
)

// shopTabLabels is the on-screen header per shop tab, indexed by ShopTab; a
// missing label leaves a "" entry the init() below catches at startup.
var shopTabLabels = [ShopTabCount]string{
	ShopTabBuy:  "Buy",
	ShopTabSell: "Sell",
}

func init() {
	for t := ShopTab(0); t < ShopTabCount; t++ {
		if shopTabLabels[t] == "" {
			panic("core: shopTabLabels has an empty entry — label every ShopTab")
		}
	}
}

// ShopTabLabel is the on-screen header for a shop tab.
func ShopTabLabel(t ShopTab) string {
	if t < 0 || int(t) >= len(shopTabLabels) {
		return ""
	}
	return shopTabLabels[t]
}

// ShopSellPrice is the gold for selling one unit: Price / ShopSellDivisor,
// floored at 1 so a priced item is always worth something. Price <= 0 = not
// sellable (callers gate on SellableStacks).
func ShopSellPrice(price int) int {
	if price <= 0 {
		return 0
	}
	if part := price / ShopSellDivisor; part > 0 {
		return part
	}
	return 1
}

// shopCatalogCache holds the priced-item list, computed once. The registry is
// immutable, so the per-frame shop overlay avoids re-scanning. Read-only.
var shopCatalogCache []ItemDefinition

// ShopCatalog returns every purchasable item (positive Price) in declaration
// order — the shop's Buy list. Shared cached slice; treat as read-only.
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

// itemForSale is the shared buyable/sellable predicate (positive Price). Single
// source for the buy catalog and sell-list filters.
func itemForSale(def ItemDefinition) bool { return def.Price > 0 }

// sellableStack is the inventory-stack form of itemForSale: positive count, item for sale.
func sellableStack(s ItemStack) bool { return s.Count > 0 && itemForSale(ItemInfo(s.Kind)) }

// SellableStacks returns the live stacks with a positive Price — the shop's
// Sell list, so cursor rows line up with drawn rows.
func SellableStacks(inv []ItemStack) []ItemStack {
	return SellableStacksInto(inv, nil)
}

// SellableStacksInto is the buffer-reusing form of SellableStacks (filters into
// `buf`, truncated first) for the per-frame Sell tab. Pass nil to allocate.
func SellableStacksInto(inv, buf []ItemStack) []ItemStack {
	return filterInto(buf, inv, sellableStack)
}

// SellableCount is the no-alloc count of sellable stacks — for the per-frame
// row-count path that needs only the length.
func SellableCount(inv []ItemStack) int {
	return countWhere(inv, sellableStack)
}

// ShopRowCount is the selectable-row count on the active shop tab — the single
// source the explore input wrap (updateShop) and the renderer's row list both
// agree on (the latter guarded by TestBuildShopRowsMatchesCatalogOrder).
func ShopRowCount(g *GameState) int {
	switch g.ShopTab {
	case ShopTabBuy:
		return len(ShopCatalog())
	case ShopTabSell:
		// No-alloc count — called every frame, needs only the count.
		return SellableCount(g.Inventory)
	default:
		panic("core: ShopRowCount missing case for ShopTab")
	}
}

// AwardBattleLoot grants the defeated pack's gold + drops (gold sums a uniform
// roll per member, drops roll each Drops table), adds to g, and returns totals.
// Only DEFEATED members pay out (correctness for a partially-alive pack; at the
// sole caller winBattle all are dead). Zero when no pack is engaged.
func AwardBattleLoot(g *GameState) (gold int, drops []ItemKind) {
	members := BattleMembers(g)
	if len(members) == 0 {
		return 0, nil
	}
	rng := g.Rand()
	for _, m := range members {
		// Dead by EITHER measure pays out; the HP guard is belt-and-suspenders
		// so a degenerate "alive corpse" (HP<=0 yet Alive) still yields loot.
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

// rollGold lives in util.go beside RandRangeI / rollDuration (the three uniform
// range draws with deliberately different degenerate-bounds policies).

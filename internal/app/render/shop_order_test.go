package render

import (
	"fmt"
	"testing"

	"crawler/internal/app/core"
)

// TestBuildShopRowsMatchesCatalogOrder pins the contract documented on shopRows:
// the drawn row order MUST match the order explore/shop.go's cursor indexes
// (core.ShopCatalog / core.SellableStacks). If buildShopRows ever reorders or
// filters differently, the cursor would select the wrong item — this catches it.
func TestBuildShopRowsMatchesCatalogOrder(t *testing.T) {
	// Buy tab: rows track core.ShopCatalog() one-for-one, in order.
	g := &core.GameState{ShopTab: core.ShopTabBuy, Gold: 1 << 30}
	rows := buildShopRows(g, nil)
	catalog := core.ShopCatalog()
	if len(rows) != len(catalog) {
		t.Fatalf("Buy rows = %d, ShopCatalog = %d (cursor would desync)", len(rows), len(catalog))
	}
	for i, def := range catalog {
		if rows[i].name != def.Name {
			t.Errorf("Buy row %d = %q, catalog = %q", i, rows[i].name, def.Name)
		}
	}

	// Sell tab: rows track core.SellableStacks(inventory), in order.
	inv := []core.ItemStack{
		{Kind: core.ItemCheese, Count: 3},
		{Kind: core.ItemNone, Count: 9}, // priceless — must be filtered, like the cursor list
		{Kind: core.ItemBatJerky, Count: 1},
	}
	g = &core.GameState{ShopTab: core.ShopTabSell, Inventory: inv}
	rows = buildShopRows(g, nil)
	stacks := core.SellableStacks(inv)
	if len(rows) != len(stacks) {
		t.Fatalf("Sell rows = %d, SellableStacks = %d (cursor would desync)", len(rows), len(stacks))
	}
	for i, s := range stacks {
		want := fmt.Sprintf("%s  x%d", core.ItemInfo(s.Kind).Name, s.Count)
		if rows[i].name != want {
			t.Errorf("Sell row %d = %q, want %q", i, rows[i].name, want)
		}
	}
}

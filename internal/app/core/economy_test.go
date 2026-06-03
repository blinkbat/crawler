package core

import (
	"math/rand"
	"testing"
)

func TestShopSellPrice(t *testing.T) {
	cases := []struct{ price, want int }{
		{0, 0},   // not for sale
		{1, 1},   // half floors to 1, not 0
		{2, 1},   // 2/2
		{40, 20}, // even
		{55, 27}, // odd rounds down
	}
	for _, c := range cases {
		if got := ShopSellPrice(c.price); got != c.want {
			t.Errorf("ShopSellPrice(%d) = %d, want %d", c.price, got, c.want)
		}
	}
}

func TestSellableStacksFiltersPriceless(t *testing.T) {
	// ItemNone has no registry price; cheese does.
	inv := []ItemStack{
		{Kind: ItemCheese, Count: 2},
		{Kind: ItemNone, Count: 5},     // priceless sentinel — excluded
		{Kind: ItemBatJerky, Count: 0}, // zero count — excluded
	}
	got := SellableStacks(inv)
	if len(got) != 1 || got[0].Kind != ItemCheese {
		t.Fatalf("SellableStacks = %+v, want only cheese", got)
	}
}

func TestShopCatalogAllPriced(t *testing.T) {
	cat := ShopCatalog()
	if len(cat) == 0 {
		t.Fatal("ShopCatalog is empty — no priced items?")
	}
	for _, def := range cat {
		if def.Price <= 0 {
			t.Errorf("catalog item %s has non-positive price %d", def.Name, def.Price)
		}
	}
}

func TestRollGoldBounds(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	if g := rollGold(rng, 0, 0); g != 0 {
		t.Errorf("rollGold(0,0) = %d, want 0", g)
	}
	for i := 0; i < 200; i++ {
		g := rollGold(rng, 3, 6)
		if g < 3 || g > 6 {
			t.Fatalf("rollGold(3,6) = %d, out of [3,6]", g)
		}
	}
	// Inverted bounds are tolerated (swapped), not panicked.
	for i := 0; i < 50; i++ {
		g := rollGold(rng, 6, 3)
		if g < 3 || g > 6 {
			t.Fatalf("rollGold(6,3) = %d, out of [3,6]", g)
		}
	}
}

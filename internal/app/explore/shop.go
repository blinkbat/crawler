package explore

import (
	"fmt"

	"crawler/internal/app/audio"
	"crawler/internal/app/core"
	"crawler/internal/app/input"
)

// Shop overlay. Two tabs: Buy from a fixed catalog, Sell inventory back at half
// value. L1/L2/R1/R2 page tabs, d-pad/stick the cursor, Confirm transacts one,
// Back closes. Carries its own per-tab list math (not via updateLeafMenu).
// DESIGN: IN-UNIVERSE — opened by a merchant tile, not a menu. Fully built;
// openShop has no caller yet (lands with the merchant work).

// openShop raises the overlay on the Buy tab. Drops the pause menu defensively
// so a future menu-adjacent caller can't leave both up.
func openShop(g *core.GameState) {
	openSubmenu(&g.MenuOpen, &g.ShopOpen, &g.ShopCursor)
	g.ShopTab = core.ShopTabBuy
}

// updateShop drives the overlay's input. Tab paging resets the cursor (the tabs
// have independent list lengths). Confirm dispatches to the active tab.
func updateShop(g *core.GameState) {
	if input.BackPressed() {
		g.ShopOpen = false
		return
	}
	if next, changed := input.PagedTab(g.ShopTab, int(core.ShopTabCount)); changed {
		g.ShopTab = next
		g.ShopCursor = 0
	}
	g.ShopCursor = input.CursorUpDown(g.ShopCursor, core.ShopRowCount(g))
	if input.ConfirmPressed() {
		switch g.ShopTab {
		case core.ShopTabBuy:
			buyShopItem(g)
		case core.ShopTabSell:
			sellShopItem(g)
		default:
			// Hand-maintained enum; a missing arm would silently confirm nothing.
			panic(fmt.Sprintf("explore: updateShop missing confirm case for ShopTab %d", g.ShopTab))
		}
	}
}

// buyShopItem purchases one unit of the cursored item if affordable (miss ping
// off the list or short on gold).
func buyShopItem(g *core.GameState) {
	catalog := core.ShopCatalog()
	def, ok := stackAtCursor(catalog, g.ShopCursor)
	if !ok {
		audio.Play(audio.SoundInputMiss)
		return
	}
	if g.Gold < def.Price {
		audio.Play(audio.SoundInputMiss)
		return
	}
	g.Gold -= def.Price
	g.Inventory = core.AddItem(g.Inventory, def.Kind, 1)
	audio.Play(audio.SoundInputGreat)
}

// sellShopItem sells one unit of the cursored stack at its half-price value.
// Reads the same SellableStacks list the renderer draws so the rows match.
func sellShopItem(g *core.GameState) {
	stacks := core.SellableStacks(g.Inventory)
	stack, ok := stackAtCursor(stacks, g.ShopCursor)
	if !ok {
		audio.Play(audio.SoundInputMiss)
		return
	}
	kind := stack.Kind
	inv, ok := core.ConsumeItem(g.Inventory, kind)
	if !ok {
		audio.Play(audio.SoundInputMiss)
		return
	}
	g.Inventory = inv
	g.Gold += core.ShopSellPrice(core.ItemInfo(kind).Price)
	audio.Play(audio.SoundInputGreat)
	// Selling the last unit shrinks the list; keep the cursor in range.
	g.ShopCursor = core.ClampIndex(g.ShopCursor, len(core.SellableStacks(g.Inventory)))
}

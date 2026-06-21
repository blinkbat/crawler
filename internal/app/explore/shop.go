package explore

import (
	"fmt"

	"crawler/internal/app/audio"
	"crawler/internal/app/core"
	"crawler/internal/app/input"
)

// Shop overlay. Two tabs: Buy from a fixed catalog, Sell back the player's
// own priced inventory at half value. Gamepad-first — L1/L2/R1/R2 (or
// Tab/Shift+Tab) page the Buy/Sell tabs, the d-pad/stick moves the row cursor,
// Confirm transacts one unit, Back closes. Mirrors the leaf-submenu shape in
// movement.go but carries its own per-tab list math, so it isn't routed
// through updateLeafMenu.
//
// DESIGN: shops are IN-UNIVERSE — this overlay is meant to be opened by a
// merchant / shop tile in the world, NOT from a menu. The overlay is fully
// built and functional; openShop just has no caller yet (the in-world entry
// point lands with the merchant/tile work). It is deliberately NOT wired to
// any pause-menu / panels row.

// openShop raises the shop overlay on the Buy tab with the cursor at the top.
// Drops the pause menu defensively (a no-op when triggered in-world, where the
// menu isn't open) so a future menu-adjacent caller can't leave both up.
func openShop(g *core.GameState) {
	g.MenuOpen = false
	g.ShopOpen = true
	g.ShopTab = core.ShopTabBuy
	g.ShopCursor = 0
}

// updateShop drives the shop overlay's input. Tab paging resets the cursor
// (the two tabs have independent list lengths, so a stale cursor could land
// off the end). Confirm dispatches to the active tab's transaction.
func updateShop(g *core.GameState) {
	if input.BackPressed() {
		g.ShopOpen = false
		return
	}
	if next, changed := input.PagedTab(g.ShopTab, int(core.ShopTabCount)); changed {
		g.ShopTab = next
		g.ShopCursor = 0
	}
	rows := shopRowCount(g)
	g.ShopCursor = input.CursorUpDown(g.ShopCursor, rows)
	if input.ConfirmPressed() {
		switch g.ShopTab {
		case core.ShopTabBuy:
			buyShopItem(g)
		case core.ShopTabSell:
			sellShopItem(g)
		default:
			// ShopTab is a hand-maintained enum; a new tab without a
			// transaction arm here would silently confirm nothing. Fail loudly,
			// matching updatePanels' / Update's missing-case panics.
			panic(fmt.Sprintf("explore: updateShop missing confirm case for ShopTab %d", g.ShopTab))
		}
	}
}

// shopRowCount is the number of selectable rows on the active tab — the
// catalog length on Buy, the sellable-stack count on Sell. Drives the
// cursor wrap so navigation can't run off the list.
func shopRowCount(g *core.GameState) int {
	switch g.ShopTab {
	case core.ShopTabBuy:
		return len(core.ShopCatalog())
	case core.ShopTabSell:
		// No-alloc count — updateShop calls this every frame the Sell tab
		// is open and only needs the row count, not the materialized slice.
		return core.SellableCount(g.Inventory)
	default:
		panic(fmt.Sprintf("explore: shopRowCount missing case for ShopTab %d", g.ShopTab))
	}
}

// buyShopItem purchases one unit of the cursored catalog item when the
// party can afford it. A gilt ping confirms; a miss ping refuses (off the
// list or short on gold).
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

// sellShopItem sells one unit of the cursored inventory stack for its
// half-price sell value. Reads the same SellableStacks list the renderer
// draws so the cursor row matches; clamps the cursor when the last unit of
// a stack is sold and the list shrinks underneath it.
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
	// Selling the last unit shrinks the list; keep the cursor in range so
	// it lands on the next row (or the new last row) instead of off the end.
	g.ShopCursor = clampCursorToLen(g.ShopCursor, len(core.SellableStacks(g.Inventory)))
}

package render

import (
	"fmt"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Shop overlay layout. Sized to the active tab's row count (no scroll).
const (
	shopPanelW      = int32(520)
	shopHeaderH     = int32(132)
	shopRowH        = modalListRowH
	shopRowGap      = int32(8)
	shopFootH       = int32(50)
	shopRowInsetX   = int32(40)
	shopRowTextDY   = int32(4) // baseline drop shared by label/name/price columns
	shopPriceInsetX = modalValueInsetX
)

// shopRow is one drawable shop row. affordable false (unaffordable Buy) renders muted.
type shopRow struct {
	name       string
	price      string
	affordable bool
}

// drawShopOverlay paints the pause-menu shop: a two-column row list + tab header.
func drawShopOverlay(g *core.GameState, assets Resources) {
	font := assets.hudFont
	rows := shopRows(g)
	stride := shopRowH + shopRowGap
	// Window the rows so a long tab (the Buy catalog runs 20+ items) can't grow the
	// card past the screen — the sibling lists (journal/items/pickers) all window; the
	// shop was the lone unwindowed one, drawing bottom rows + footer off a 768p screen.
	_, sh := screenSizeF()
	maxVisible := int((int32(sh*0.82) - shopHeaderH - shopFootH) / stride)
	if maxVisible < 1 {
		maxVisible = 1
	}
	visibleRows := len(rows)
	if visibleRows < 1 {
		visibleRows = 1 // reserve a line for the placeholder
	}
	if visibleRows > maxVisible {
		visibleRows = maxVisible
	}
	panelH := shopHeaderH + stride*int32(visibleRows) + shopFootH
	panelX, panelY, belowTitleY := drawTitledCardHeader(assets, "SHOP", shopPanelW, panelH)

	// Gold readout (right) and Buy/Sell tabs (left) share the sub-title row.
	subY := belowTitleY + float32(uiGapAfterTitle)
	drawTextRightAligned(font, goldLabelFull(g.Gold), float32(panelX+shopPanelW)-float32(shopRowInsetX), subY, FontBody, borderActive)
	drawShopTabs(font, g.ShopTab, float32(panelX+shopRowInsetX), subY)

	// Rows.
	rowX := panelX + shopRowInsetX
	rowY := panelY + shopHeaderH
	innerW := shopPanelW - shopRowInsetX*2
	if len(rows) == 0 {
		drawTextWithShadow(font, shopEmptyLabel(g.ShopTab), float32(rowX), float32(rowY+shopRowTextDY), FontBody, textMuted)
	}
	// Scroll the window around the cursor (same helper the pickers use).
	first := journalScrollFirst(g.ShopCursor, len(rows), visibleRows)
	for i := first; i < len(rows) && i < first+visibleRows; i++ {
		r := rows[i]
		if i == g.ShopCursor {
			DrawSelectedRowI(rowX-focusPlateInsetX, rowY-focusPlateInsetY, innerW, shopRowH)
		}
		nameCol := rowTextColor(r.affordable, !r.affordable, textMuted)
		drawTextWithShadow(font, r.name, float32(rowX), float32(rowY+shopRowTextDY), FontBody, nameCol)
		drawTextRightAligned(font, r.price, float32(rowX)+float32(innerW)-float32(shopPriceInsetX), float32(rowY+shopRowTextDY), FontBody, textLabel)
		rowY += stride
	}

	shopCard := rl.NewRectangle(float32(panelX), float32(panelY), float32(shopPanelW), float32(panelH))
	drawModalFooterGlyphsSized(font, shopCard, shopHints, FontSmall)
}

// shopHints is the shop's "Buy/Sell · Confirm · Back" footer, built once (like
// confirmBackHints) so the panel doesn't reallocate a hint bar every frame.
var shopHints = []HintSeg{
	Hint("Buy / Sell", GlyphLB, GlyphRB),
	Hint("Confirm", GlyphA),
	Hint("Back", GlyphB),
}

// shopRowsCache memoizes the active tab's rows to avoid a slice alloc + per-row
// Sprintf at 60 Hz. Rebuilt when the tab, gold (Buy affordability), or inventory
// fingerprint (Sell contents) changes.
var shopRowsCache struct {
	primed bool
	tab    core.ShopTab
	gold   int
	invFP  uint64
	rows   []shopRow
}

// inventoryFingerprint folds the bag's (kind, count) pairs into a uint64
// (FNV-1a) so shopRows can detect a Sell-affecting change without rebuilding.
func inventoryFingerprint(inv []core.ItemStack) uint64 {
	h := core.FNVOffset64
	for _, s := range inv {
		h = (h ^ uint64(s.Kind)) * core.FNVPrime64
		h = (h ^ uint64(uint32(s.Count))) * core.FNVPrime64
	}
	return h
}

// shopRows returns the active tab's rows (cached). Order matches the input
// handler's slices (core.ShopCatalog / SellableStacks) so cursor and rows align;
// TestBuildShopRowsMatchesCatalogOrder guards against the two drifting.
func shopRows(g *core.GameState) []shopRow {
	fp := inventoryFingerprint(g.Inventory)
	c := &shopRowsCache
	if c.primed && c.tab == g.ShopTab && c.gold == g.Gold && c.invFP == fp {
		return c.rows
	}
	c.rows = buildShopRows(g, c.rows[:0])
	c.tab, c.gold, c.invFP, c.primed = g.ShopTab, g.Gold, fp, true
	return c.rows
}

// buildShopRows appends the active tab's rows into buf. Buy reads the catalog
// (affordability vs gold); Sell reads priced inventory at half value.
func buildShopRows(g *core.GameState, buf []shopRow) []shopRow {
	switch g.ShopTab {
	case core.ShopTabSell:
		stacks := core.SellableStacks(g.Inventory)
		for _, s := range stacks {
			def := core.ItemInfo(s.Kind)
			buf = append(buf, shopRow{
				name:       stackLabel(def.Name, s.Count),
				price:      fmt.Sprintf("%dg", core.ShopSellPrice(def.Price)),
				affordable: true,
			})
		}
		return buf
	case core.ShopTabBuy:
		catalog := core.ShopCatalog()
		for _, def := range catalog {
			buf = append(buf, shopRow{
				name:       def.Name,
				price:      fmt.Sprintf("%dg", def.Price),
				affordable: g.Gold >= def.Price,
			})
		}
		return buf
	default:
		// Fail loudly on an unhandled ShopTab — mirrors core.ShopTabLabel.
		panic("render: shopRows missing case for ShopTab")
	}
}

// shopEmptyLabel is the no-rows placeholder (only reachable on Sell; Buy is always stocked).
func shopEmptyLabel(tab core.ShopTab) string {
	if tab == core.ShopTabSell {
		return "Nothing to sell."
	}
	return "Nothing for sale."
}

// drawShopTabs paints the "Buy   Sell" header (active gilt, other muted) via drawTextTabStrip.
func drawShopTabs(font rl.Font, active core.ShopTab, x, y float32) {
	drawTextTabStrip(font, x, y, int(core.ShopTabCount), int(active),
		func(i int) string { return core.ShopTabLabel(core.ShopTab(i)) },
		tabLabelMeasurer(&shopTabMeasureCache, font),
		borderActive, shopTabStripGap, false)
}

// shopTabStripGap is the inter-tab spacing for the Buy/Sell header strip.
const shopTabStripGap = float32(28)

// shopTabMeasureCache memoizes tab-label widths to avoid re-shaping via cgo each frame.
var shopTabMeasureCache measureCache

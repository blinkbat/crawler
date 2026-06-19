package render

import (
	"fmt"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Shop overlay layout. A gilt veiled card centered on screen: title + gold
// readout, a Buy/Sell tab header, the active tab's item rows (name left,
// price right), and a controller-first footer hint. Sized to the active
// tab's row count (no scroll — the catalog and the sellable inventory are
// both small).
const (
	shopPanelW    = int32(520)
	shopHeaderH   = int32(132)
	shopRowH      = int32(34)
	shopRowGap    = int32(8)
	shopFootH     = int32(50)
	shopRowInsetX = int32(40)
	// shopRowTextDY is the baseline drop from a row's top edge to its text, shared
	// by the empty-label / name / price columns so they sit on one line.
	shopRowTextDY = int32(4)
	// shopPriceInsetX is the right-edge inset of the price column inside the row.
	shopPriceInsetX = int32(12)
	// shopHintDrop nudges the footer hint down INTO the reserved shopFootH band
	// (which starts at panelH-shopFootH) so the centered glyph strip seats in the
	// middle of the footer rather than at its top edge. The shop hand-centers its
	// hint at FontSmall rather than routing through drawModalFooterGlyphs, which
	// would force FontTiny + the modal footer offset and shift this layout.
	shopHintDrop = int32(16)
)

// shopRow is one drawable row of the shop list: the item label, its
// right-aligned price string, and whether the party can transact it (Buy
// rows the party can't afford render muted; Sell rows are always
// affordable). Built per-frame from the active tab.
type shopRow struct {
	name       string
	price      string
	affordable bool
}

// drawShopOverlay paints the pause-menu shop. Mirrors the menu-card chrome
// (drawVeiledCard + flanking fleurons) but lays out a two-column row list
// and a tab header rather than the plain submenu rows.
func drawShopOverlay(g *core.GameState, assets Resources) {
	font := assets.hudFont
	rows := shopRows(g)
	visibleRows := len(rows)
	if visibleRows < 1 {
		visibleRows = 1 // reserve a line for the "(nothing)" placeholder
	}
	stride := shopRowH + shopRowGap
	panelH := shopHeaderH + stride*int32(visibleRows) + shopFootH
	panelX, panelY, belowTitleY := drawTitledCardHeader(assets, "SHOP", shopPanelW, panelH)

	// Gold readout (right) and Buy/Sell tabs (left) share the sub-title row.
	subY := belowTitleY + 12
	drawTextRightAligned(font, goldLabelFull(g.Gold), float32(panelX+shopPanelW)-float32(shopRowInsetX), subY, FontBody, borderActive)
	drawShopTabs(font, g.ShopTab, float32(panelX+shopRowInsetX), subY)

	// Rows.
	rowX := panelX + shopRowInsetX
	rowY := panelY + shopHeaderH
	innerW := shopPanelW - shopRowInsetX*2
	if len(rows) == 0 {
		drawTextWithShadow(font, shopEmptyLabel(g.ShopTab), float32(rowX), float32(rowY+shopRowTextDY), FontBody, textMuted)
	}
	for i, r := range rows {
		if i == g.ShopCursor {
			DrawSelectedRowI(rowX-focusPlateInsetX, rowY-focusPlateInsetY, innerW, shopRowH)
		}
		nameCol := rowTextColor(r.affordable, !r.affordable, textMuted)
		drawTextWithShadow(font, r.name, float32(rowX), float32(rowY+shopRowTextDY), FontBody, nameCol)
		drawTextRightAligned(font, r.price, float32(rowX)+float32(innerW)-float32(shopPriceInsetX), float32(rowY+shopRowTextDY), FontBody, textLabel)
		rowY += stride
	}

	DrawHintBar(font, []HintSeg{
		Hint("Buy / Sell", GlyphLB, GlyphRB),
		Hint("Confirm", GlyphA),
		Hint("Back", GlyphB),
	}, float32(panelX)+float32(shopPanelW)/2, float32(panelY+panelH-shopFootH+shopHintDrop), FontSmall)
}

// shopRowsCache memoizes the active tab's drawable rows so drawShopOverlay
// doesn't make() a fresh slice + Sprintf every price label at 60 Hz while the
// shop is open. Rebuilt only when an input that shapes the rows changes: the
// active tab, the gold total (Buy affordability), or the inventory (Sell
// contents, via a cheap O(stacks) fingerprint). Mirrors goldReadout's
// single-entry HUD cache. Modal-only, but keeps the one remaining per-frame
// Sprintf-in-a-loop out of the draw path.
var shopRowsCache struct {
	primed bool
	tab    core.ShopTab
	gold   int
	invFP  uint64
	rows   []shopRow
}

// inventoryFingerprint folds the bag's (kind, count) pairs into a single
// uint64 (FNV-1a) so shopRows can detect a Sell-affecting inventory change
// without rebuilding (and re-allocating) the row list every frame. No
// allocation; the loop is over the handful of held stacks.
func inventoryFingerprint(inv []core.ItemStack) uint64 {
	h := uint64(1469598103934665603)
	for _, s := range inv {
		h = (h ^ uint64(s.Kind)) * 1099511628211
		h = (h ^ uint64(uint32(s.Count))) * 1099511628211
	}
	return h
}

// shopRows returns the active tab's drawable rows, served from shopRowsCache
// when the tab/gold/inventory are unchanged since the last build. The slices
// match the ones the input handler walks (core.ShopCatalog /
// core.SellableStacks) so cursor and rows align.
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

// buildShopRows appends the active tab's rows into buf (the reused cache
// buffer, already truncated). Buy reads the catalog (affordability gated on
// current gold); Sell reads the player's priced inventory at half value.
func buildShopRows(g *core.GameState, buf []shopRow) []shopRow {
	switch g.ShopTab {
	case core.ShopTabSell:
		stacks := core.SellableStacks(g.Inventory)
		for _, s := range stacks {
			def := core.ItemInfo(s.Kind)
			buf = append(buf, shopRow{
				name:       fmt.Sprintf("%s  x%d", def.Name, s.Count),
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
		// A new ShopTab that forgets a rows case fails loudly instead of
		// silently rendering the Buy list — mirrors core.ShopTabLabel.
		panic("render: shopRows missing case for ShopTab")
	}
}

// shopEmptyLabel is the placeholder shown when the active tab has no rows —
// only reachable on the Sell tab with nothing priced to sell (the Buy
// catalog is always stocked).
func shopEmptyLabel(tab core.ShopTab) string {
	if tab == core.ShopTabSell {
		return "Nothing to sell."
	}
	return "Nothing for sale."
}

// drawShopTabs paints the "Buy   Sell" header, the active tab gilt and the
// other muted. Anchored at (x, y); shares the simple text-tab rhythm with the
// Journal sub-tabs via drawTextTabStrip (no underline here).
func drawShopTabs(font rl.Font, active core.ShopTab, x, y float32) {
	drawTextTabStrip(font, x, y, int(core.ShopTabCount), int(active),
		func(i int) string { return core.ShopTabLabel(core.ShopTab(i)) },
		tabLabelMeasurer(&shopTabMeasureCache, font),
		borderActive, 28, false)
}

// shopTabMeasureCache memoizes the Buy/Sell tab-label widths so the tab strip
// doesn't re-shape them via cgo every frame the shop is open — mirroring the
// Journal sub-tab strip's journalMeasureCache.
var shopTabMeasureCache measureCache

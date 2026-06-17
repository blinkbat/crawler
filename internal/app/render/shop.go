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
		drawTextWithShadow(font, shopEmptyLabel(g.ShopTab), float32(rowX), float32(rowY+4), FontBody, textMuted)
	}
	for i, r := range rows {
		if i == g.ShopCursor {
			DrawSelectedRowI(rowX-focusPlateInsetX, rowY-focusPlateInsetY, innerW, shopRowH)
		}
		nameCol := rowTextColor(r.affordable, !r.affordable, textMuted)
		drawTextWithShadow(font, r.name, float32(rowX), float32(rowY+4), FontBody, nameCol)
		drawTextRightAligned(font, r.price, float32(rowX)+float32(innerW)-12, float32(rowY+4), FontBody, textLabel)
		rowY += stride
	}

	DrawHintBar(font, []HintSeg{
		Hint("Buy / Sell", GlyphLB, GlyphRB),
		Hint("Confirm", GlyphA),
		Hint("Back", GlyphB),
	}, float32(panelX)+float32(shopPanelW)/2, float32(panelY+panelH-shopFootH+shopHintDrop), FontSmall)
}

// shopRows builds the active tab's drawable rows. Buy reads the catalog
// (affordability gated on current gold); Sell reads the player's priced
// inventory at half value. The slices match the ones the input handler
// walks (core.ShopCatalog / core.SellableStacks) so cursor and rows align.
func shopRows(g *core.GameState) []shopRow {
	switch g.ShopTab {
	case core.ShopTabSell:
		stacks := core.SellableStacks(g.Inventory)
		rows := make([]shopRow, 0, len(stacks))
		for _, s := range stacks {
			def := core.ItemInfo(s.Kind)
			rows = append(rows, shopRow{
				name:       fmt.Sprintf("%s  x%d", def.Name, s.Count),
				price:      fmt.Sprintf("%dg", core.ShopSellPrice(def.Price)),
				affordable: true,
			})
		}
		return rows
	default: // ShopTabBuy
		catalog := core.ShopCatalog()
		rows := make([]shopRow, 0, len(catalog))
		for _, def := range catalog {
			rows = append(rows, shopRow{
				name:       def.Name,
				price:      fmt.Sprintf("%dg", def.Price),
				affordable: g.Gold >= def.Price,
			})
		}
		return rows
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
		func(s string) float32 { return rl.MeasureTextEx(font, s, FontBody, FontSpacingBody).X },
		borderActive, 28, false)
}

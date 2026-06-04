package render

import (
	"fmt"

	"crawler/internal/app/core"
)

func DrawOverlay(g core.GameState, assets Resources) {
	if g.MenuOpen {
		drawMenuOverlay(g, assets)
		return
	}
	if g.OptionsMenuOpen {
		drawOptionsMenuOverlay(g, assets)
		return
	}
	if g.ShopOpen {
		drawShopOverlay(g, assets)
		return
	}
	if g.DebugMenuOpen {
		drawDebugMenuOverlay(g, assets)
		return
	}
	if g.Battle.Active() {
		// Compute the turn forecast once per frame; TurnPanelBottomY
		// (called from the combat log) and drawTurnPanel both read
		// the cached slice instead of re-running TurnForecast.
		CacheTurnForecastForFrame(&g)
		drawBattleHUD(g, assets)
		drawTurnPanel(g, assets)
		drawMinimap(g.Area, g, assets)
		DrawPartyRibbon(g, assets)
		drawTimingBar(g, assets)
		drawBattleSplash(g, assets)
		return
	}
	drawMinimap(g.Area, g, assets)
	DrawPartyRibbon(g, assets)
	drawGoldReadout(g, assets)
}

// goldReadout caches the formatted "<n> G" label so the per-frame
// exploration draw doesn't re-Sprintf an unchanged gold total. Gold only
// changes on loot / shop transactions, so the string is rebuilt rarely.
var goldReadout = struct {
	last int
	str  string
}{last: -1}

// goldReadoutMeasureCache memoizes the gold label's MeasureTextEx. The label
// string is already cached above; this caches its measurement too so the
// per-frame exploration HUD doesn't make a CGO measure call for an unchanged
// width. Keyed on the label text, so it refreshes when the gold total does.
var goldReadoutMeasureCache measureCache

// drawGoldReadout paints a small gilt gold chip in the top-left corner
// during free exploration. A glass pane backs the "<n> G" label so it
// reads over busy world geometry. Kept out of the battle / overlay paths —
// the shop shows its own gold total, and combat has no spend.
func drawGoldReadout(g core.GameState, assets Resources) {
	font := assets.hudFont
	if g.Gold != goldReadout.last {
		goldReadout.last = g.Gold
		goldReadout.str = fmt.Sprintf("%d G", g.Gold)
	}
	label := goldReadout.str
	m := goldReadoutMeasureCache.measure(font, label, FontBody, FontSpacingBody)
	padX, padY := float32(12), float32(6)
	x, y := hudEdgePad, hudEdgePad
	w := int32(m.X + padX*2)
	h := int32(m.Y + padY*2)
	drawGlassPane(x, y, w, h, glassDeep)
	drawTextWithShadow(font, label, float32(x)+padX, float32(y)+padY, FontBody, borderActive)
}

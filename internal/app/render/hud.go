package render

import (
	"crawler/internal/app/core"
	rl "github.com/gen2brain/raylib-go/raylib"
)

func DrawOverlay(g *core.GameState, assets Resources) {
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
	if g.RetroMenuOpen {
		drawRetroMenuOverlay(g, assets)
		return
	}
	if g.DebugMenuOpen {
		drawDebugMenuOverlay(g, assets)
		return
	}
	if g.Battle.Active() {
		// Compute the turn forecast once per frame; TurnPanelBottomY
		// (called from the action log) and drawTurnPanel both read
		// the cached slice instead of re-running TurnForecast.
		CacheTurnForecastForFrame(g)
		drawBattleHUD(g, assets)
		drawTurnPanel(g, assets)
		drawMinimap(&g.Area, g, assets)
		DrawPartyRibbon(g, assets)
		drawTimingBar(g, assets)
		drawBattleSplash(g, assets)
		// Post-victory spoils card draws on top of the dimmed battle scene.
		// No-ops outside the won-battle results window (gates internally on
		// phase + the dance beat), so it's safe to call every battle frame.
		DrawVictorySpoils(g, assets)
		return
	}
	drawMinimap(&g.Area, g, assets)
	// The action log persists out of combat (bottom-left), so exploration shows
	// the same rolling pane as battle — saves, crystal rests, the last fight's
	// result, etc. Reads g.ActionLog, which is no longer reset between fights.
	drawActionLogPanel(g, assets)
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
func drawGoldReadout(g *core.GameState, assets Resources) {
	font := assets.hudFont
	if g.Gold != goldReadout.last {
		goldReadout.last = g.Gold
		goldReadout.str = goldLabelShort(g.Gold)
	}
	label := goldReadout.str
	m := goldReadoutMeasureCache.measure(font, label, FontBody, FontSpacingBody)
	padX, padY := float32(12), float32(6)
	iconW := float32(28)
	x := hudEdgePad + MinimapWidth() + hudColumnGap
	y := hudEdgePad
	w := int32(m.X + padX*2 + iconW)
	h := int32(m.Y + padY*2)
	screenW, _ := screenSize()
	if x+w > screenW-hudEdgePad {
		x = screenW - hudEdgePad - w
	}
	// Plain wood frame — no gilt accent stripe (the bright giltBright spine read
	// as a "yellow highlight border" around the little chip). The coin glyph +
	// gilt label already carry the gold theme; the frame stays neutral wood.
	drawCard(x, y, w, h, glassWarm, borderSoft, noAccent)
	cy := float32(y) + float32(h)/2
	drawCoinGlyph(float32(x)+padX+10, cy, 8)
	drawTextWithShadow(font, label, float32(x)+padX+iconW, float32(y)+padY, FontBody, borderActive)
}

func drawCoinGlyph(cx, cy, r float32) {
	rl.DrawCircleV(rl.NewVector2(cx, cy), r+2, fadeColor(woodDark, 0.85))
	rl.DrawCircleV(rl.NewVector2(cx, cy), r, coinFace)
	rl.DrawCircleV(rl.NewVector2(cx, cy), r*0.62, coinShade)
	rl.DrawCircleV(rl.NewVector2(cx-r*0.28, cy-r*0.30), r*0.22, fadeColor(giltBright, 0.85))
	drawDiamondPip(cx+r*0.38, cy+r*0.32, 1.5, fadeColor(giltBright, 0.65))
}

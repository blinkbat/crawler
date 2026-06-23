package render

import (
	"crawler/internal/app/core"
	rl "github.com/gen2brain/raylib-go/raylib"
)

// DrawOverlay paints the 2D overlay: HUD and any open top-level menu, cross-faded
// by tickMenuFade. The no-fade steady state (progress 0 or 1) costs no render target.
func DrawOverlay(g *core.GameState, assets Resources) {
	tickMenuFade(g)
	hudAlpha := 1 - menuFade.progress
	menuAlpha := menuFade.progress
	if hudAlpha > 0.001 {
		withFadeAlpha(hudAlpha, func() { drawSceneHUD(g, assets) })
	}
	if menuAlpha > 0.001 && menuFade.drawer != nil {
		withFadeAlpha(menuAlpha, func() { menuFade.drawer(g, assets) })
	}
}

// drawSceneHUD paints the in-world HUD (battle or exploration) with no menu;
// split from DrawOverlay so withFadeAlpha captures it as one layer.
func drawSceneHUD(g *core.GameState, assets Resources) {
	if g.Battle.Active() {
		// Cache the turn forecast once per frame; TurnPanelBottomY and drawTurnPanel both read it.
		CacheTurnForecastForFrame(g)
		drawBattleHUD(g, assets)
		drawTurnPanel(g, assets)
		drawMinimap(&g.Area, g, assets)
		DrawPartyRibbon(g, assets)
		drawTimingBar(g, assets)
		drawBattleSplash(g, assets)
		// Overlay-based wipe FX (Tint/Flash/Vignette) sit on top during the entry
		// window; camera-based kinds rode the camera already.
		DrawBattleWipeOverlay(g, assets)
		// Spoils card gates internally on the won-battle window; safe to call every frame.
		DrawVictorySpoils(g, assets)
		return
	}
	drawMinimap(&g.Area, g, assets)
	// Action log persists out of combat; g.ActionLog is no longer reset between fights.
	drawActionLogPanel(g, assets)
	DrawPartyRibbon(g, assets)
	drawGoldReadout(g, assets)
	// Debug ▸ Screen Wipe FX preview plays over the field (overlay kinds).
	DrawBattleWipeOverlay(g, assets)
}

// goldReadout caches the formatted "<n> G" label so the per-frame draw doesn't re-Sprintf an unchanged total.
var goldReadout = struct {
	last int
	str  string
}{last: -1}

// goldReadoutMeasureCache memoizes the gold label's MeasureTextEx (avoids a per-frame CGO call).
var goldReadoutMeasureCache measureCache

// drawGoldReadout paints the gilt gold chip in the top-left during exploration.
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
	// Plain wood frame, no gilt accent stripe (it read as a yellow highlight border on the chip).
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

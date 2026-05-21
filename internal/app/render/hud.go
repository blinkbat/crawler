package render

import (
	"crawler/internal/app/core"
)

func DrawOverlay(g core.GameState, assets Resources) {
	if g.MenuOpen {
		drawMenuOverlay(g, assets)
		return
	}
	if g.Battle.Active() {
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
}

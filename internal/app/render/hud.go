package render

import (
	"crawler/internal/app/core"
)

func DrawOverlay(g core.GameState, assets Resources) {
	if g.MenuOpen {
		drawMenuOverlay(g, assets)
		return
	}
	if g.Battle.Phase != core.BattleNone {
		drawBattleHUD(g, assets)
		drawTurnPanel(g, assets)
		drawMinimap(g.Map, g, assets)
		DrawPartyRibbon(g, assets)
		drawTimingBar(g, assets)
		drawBattleSplash(g, assets)
		return
	}
	drawMinimap(g.Map, g, assets)
	DrawPartyRibbon(g, assets)
}

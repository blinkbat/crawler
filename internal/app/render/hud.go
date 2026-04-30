package render

import (
	"fmt"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

func DrawOverlay(g core.GameState, assets Resources) {
	if g.MenuOpen {
		drawMenuOverlay(g, assets)
		return
	}
	drawMinimap(g.Map, g)
	if g.Battle.Phase != core.BattleNone {
		drawBattleSplash(g, assets)
		drawBattleOverlay(g, assets)
		drawTurnPanel(g, assets)
		drawTargetTooltip(g, assets)
		return
	}
	p := g.Player
	partyHP, partyMaxHP := core.PartyHPTotals(g.Party)
	screenH := int32(rl.GetScreenHeight())
	drawHUDText(assets.hudFont, fmt.Sprintf("Tile:%d,%d  Facing:%s  Party HP:%d/%d", p.TileX, p.TileZ, core.FacingName(p.Facing), partyHP, partyMaxHP), 12, screenH-62, 21)
	drawHUDText(assets.hudFont, "W/S step  A/D strafe  Q/E or arrows turn  Right-drag free-look", 12, screenH-34, 20)
}

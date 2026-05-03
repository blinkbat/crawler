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
	if g.Battle.Phase != core.BattleNone {
		drawBattleHUD(g, assets)
		drawTurnPanel(g, assets)
		drawMinimap(g.Map, g, assets)
		DrawPartyRibbon(g, assets)
		drawBattleSplash(g, assets)
		return
	}
	drawMinimap(g.Map, g, assets)
	drawLocationChip(g, assets)
	drawControlsChip(assets)
	DrawPartyRibbon(g, assets)
}

// drawLocationChip renders a small bottom-right pill showing the player's tile,
// facing, and party HP totals. It sits above the party ribbon.
func drawLocationChip(g core.GameState, assets Resources) {
	p := g.Player
	partyHP, partyMaxHP := core.PartyHPTotals(g.Party)
	primary := fmt.Sprintf("Tile  %d, %d", p.TileX, p.TileZ)
	secondary := fmt.Sprintf("%s   -   Party  %d / %d", core.FacingName(p.Facing), partyHP, partyMaxHP)

	font := assets.hudFont
	primarySize := float32(17)
	secondarySize := float32(14)
	primaryMeasure := rl.MeasureTextEx(font, primary, primarySize, 1)
	secondaryMeasure := rl.MeasureTextEx(font, secondary, secondarySize, 1)
	w := primaryMeasure.X
	if secondaryMeasure.X > w {
		w = secondaryMeasure.X
	}
	w += 36
	h := float32(64)

	screenW := float32(rl.GetScreenWidth())
	x := screenW - w - 22
	y := PartyRibbonTopY() - h - 14

	ix, iy, iw, ih := int32(x), int32(y), int32(w), int32(h)
	drawCard(ix, iy, iw, ih, surfacePrimary, borderSoft, borderStrong)

	drawTextWithShadow(font, primary, x+16, y+12, primarySize, textPrimary)
	drawTextWithShadow(font, secondary, x+16, y+36, secondarySize, textLabel)
}

// drawControlsChip renders a thin top-right control hint card.
func drawControlsChip(assets Resources) {
	font := assets.hudFont
	text := "W/S step    A/D strafe    Q/E turn    Right-drag look"
	size := float32(14)
	measure := rl.MeasureTextEx(font, text, size, 1)
	w := int32(measure.X + 32)
	h := int32(34)
	screenW := int32(rl.GetScreenWidth())
	x := screenW - w - 22
	y := int32(22)
	drawCard(x, y, w, h, surfacePrimary, borderDim, borderSoft)
	drawTextWithShadow(font, text, float32(x+16), float32(y)+(float32(h)-measure.Y)/2, size, textHint)
}

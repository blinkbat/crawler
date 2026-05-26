package render

import (
	"image/color"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// turnPanelW is the on-screen width of the turn-order panel. Sized
// to match the minimap that sits above it on the left edge so the
// HUD reads as a stacked column. Compact rows + smaller text keep
// the panel from dominating the left third now that it's anchored
// to the corner.
const (
	turnPanelW = int32(220)
)

// turnForecastBuf is reused across frames so the per-frame forecast
// doesn't allocate. CacheTurnForecastForFrame populates it once at the
// top of the battle HUD pass; TurnPanelBottomY and drawTurnPanel then
// read the same slice, eliminating the duplicate TurnForecast call
// (combat-log layout used to invoke it via TurnPanelBottomY, and the
// turn panel itself reinvoked it on the very next line).
var turnForecastBuf = make([]core.TurnEntry, 0, 7)

// CacheTurnForecastForFrame computes the turn forecast once and stores
// it for downstream HUD consumers. Called from DrawOverlay's battle
// branch before either drawBattleHUD or drawTurnPanel runs.
func CacheTurnForecastForFrame(g *core.GameState) {
	turnForecastBuf = core.TurnForecastInto(g, turnForecastBuf, 7)
}

// turnPanelTopPad / turnPanelBottomPad bracket the rows inside the
// panel. The old header band ("TURN ORDER" + underline) was dropped —
// the class-tinted rows already name themselves; the title was just
// tautological chrome.
const (
	turnPanelTopPad    = int32(12)
	turnPanelBottomPad = int32(10)
)

// TurnPanelBottomY returns the Y screen coordinate of the bottom
// edge of the turn-order panel — used by the combat-log panel that
// docks below it on the same left edge. Returns the minimap bottom
// when the queue is empty (no panel painted, so combat log slots up).
func TurnPanelBottomY(g core.GameState) int32 {
	turns := turnForecastBuf
	if len(turns) == 0 {
		return MinimapBottomY()
	}
	rowH := int32(28)
	h := turnPanelTopPad + int32(len(turns))*rowH + turnPanelBottomPad
	return MinimapBottomY() + hudColumnGap + h
}

func drawTurnPanel(g core.GameState, assets Resources) {
	turns := turnForecastBuf
	if len(turns) == 0 {
		return
	}
	w := turnPanelW
	x := hudEdgePad
	y := MinimapBottomY() + hudColumnGap
	rowH := int32(28)
	h := turnPanelTopPad + int32(len(turns))*rowH + turnPanelBottomPad

	drawCard(x, y, w, h, surfacePrimary, borderSoft, borderSoft)

	for i, turn := range turns {
		rowY := y + turnPanelTopPad + int32(i)*rowH
		col := turnEntryColor(turn)

		rowX := x + 10
		rowW := w - 20
		rowInnerH := rowH - 4

		if i == 0 {
			drawSmallPanel(rowX, rowY, rowW, rowInnerH, colorWithAlpha(col, 96))
			drawSmallPanelOutline(rowX, rowY, rowW, rowInnerH, colorWithAlpha(col, 235))
			cx := float32(rowX + 10)
			cy := float32(rowY) + float32(rowInnerH)/2
			drawArrowMarker(rl.NewVector2(cx-2, cy), 8, 0, 6, col)
		} else {
			rl.DrawRectangle(rowX+6, rowY+4, 3, rowInnerH-8, colorWithAlpha(col, 220))
		}

		labelX := rowX + 22
		labelSize := FontSmall
		labelMeasure := measureTurnLabel(assets.hudFont, turn.Label)
		labelY := float32(rowY) + (float32(rowInnerH)-labelMeasure.Y)/2 - 1
		drawTextWithShadow(assets.hudFont, turn.Label, float32(labelX), labelY, labelSize, col)
	}
}

// turnLabelMeasureCache memoizes rl.MeasureTextEx for the small set of
// turn-row labels (party names + enemy SingularName strings). The
// panel paints up to 7 rows every battle frame; without this cache
// each row costs a cgo round-trip even though the labels only change
// when an actor enters or leaves the queue.
var turnLabelMeasureCache = make(map[string]rl.Vector2, 16)
var turnLabelMeasureCacheFontID uint32

func measureTurnLabel(font rl.Font, label string) rl.Vector2 {
	if font.Texture.ID != turnLabelMeasureCacheFontID {
		for k := range turnLabelMeasureCache {
			delete(turnLabelMeasureCache, k)
		}
		turnLabelMeasureCacheFontID = font.Texture.ID
	}
	if v, ok := turnLabelMeasureCache[label]; ok {
		return v
	}
	v := rl.MeasureTextEx(font, label, FontSmall, 1)
	turnLabelMeasureCache[label] = v
	return v
}

func turnEntryColor(turn core.TurnEntry) color.RGBA {
	if turn.Enemy {
		return rl.NewColor(245, 100, 92, 255)
	}
	return partyClassPresentationFor(turn.Class).turnColor
}

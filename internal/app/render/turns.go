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
// (action-log layout used to invoke it via TurnPanelBottomY, and the
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
	turnPanelRowH      = int32(28)
)

// turnPanelHeight is the panel's pixel height for n forecast rows. Shared by
// the draw (drawTurnPanel) and the docked action-log's bottom-edge read
// (TurnPanelBottomY) so the two can't drift on the row height / pad math.
func turnPanelHeight(n int) int32 {
	return turnPanelTopPad + int32(n)*turnPanelRowH + turnPanelBottomPad
}

// TurnPanelBottomY returns the Y screen coordinate of the bottom
// edge of the turn-order panel — used by the action-log panel that
// docks below it on the same left edge. Returns the minimap bottom
// when the queue is empty (no panel painted, so action log slots up).
func TurnPanelBottomY(g core.GameState) int32 {
	turns := turnForecastBuf
	if len(turns) == 0 {
		return MinimapBottomY()
	}
	return MinimapBottomY() + hudColumnGap + turnPanelHeight(len(turns))
}

func drawTurnPanel(g core.GameState, assets Resources) {
	turns := turnForecastBuf
	if len(turns) == 0 {
		return
	}
	w := turnPanelW
	x := hudEdgePad
	y := MinimapBottomY() + hudColumnGap
	rowH := turnPanelRowH
	h := turnPanelHeight(len(turns))

	drawCard(x, y, w, h, surfacePrimary, borderSoft, borderSoft)

	// Sequence thread — a faint vertical line stitched down through the row
	// markers (under them), tying the forecast into one strand the way a
	// lineage chart threads its entries. Static chrome: it runs from the
	// acting row's marker to the last row so the queue reads as "this, then
	// these," not seven disconnected chips. Skipped for a single row (nothing
	// to thread).
	if len(turns) > 1 {
		threadX := x + 10 + 7 // center of the per-row tick column
		threadTop := y + turnPanelTopPad + rowH/2
		threadH := int32(len(turns)-1) * rowH
		rl.DrawRectangle(threadX, threadTop, 1, threadH, fadeColor(inkDim, 0.32))
		drawDiamondPip(float32(threadX), float32(threadTop+threadH), 2, fadeColor(inkDim, 0.5))
	}

	for i, turn := range turns {
		rowY := y + turnPanelTopPad + int32(i)*rowH
		col := turnEntryColor(turn)

		rowX := x + 10
		rowW := w - 20
		rowInnerH := rowH - 4

		if i == 0 {
			drawGlassPane(rowX, rowY, rowW, rowInnerH, colorWithAlpha(col, 96))
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
var turnLabelMeasureCache measureCache

func measureTurnLabel(font rl.Font, label string) rl.Vector2 {
	return turnLabelMeasureCache.measure(font, label, FontSmall, 1)
}

func turnEntryColor(turn core.TurnEntry) color.RGBA {
	if turn.Enemy {
		return turnEnemyColor
	}
	return classAccent(turn.Class)
}

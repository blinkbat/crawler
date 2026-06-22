package render

import (
	"image/color"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// turnPanelW is the turn-order panel width, bound to the minimap above it so the
// HUD column can't drift (both derive from minimapPanelW).
const (
	turnPanelW = minimapPanelW
)

// turnForecastMax is the lookahead depth. Shared by the buffer cap and the forecast call.
const turnForecastMax = 10

// turnForecastBuf is reused across frames (no per-frame alloc). Populated once by CacheTurnForecastForFrame; read by TurnPanelBottomY and drawTurnPanel.
var turnForecastBuf = make([]core.TurnEntry, 0, turnForecastMax)

// CacheTurnForecastForFrame computes the forecast once for downstream HUD consumers. Called before drawBattleHUD / drawTurnPanel.
func CacheTurnForecastForFrame(g *core.GameState) {
	turnForecastBuf = core.TurnForecastInto(g, turnForecastBuf, turnForecastMax)
}

// Title band + row padding for the turn panel.
const (
	turnPanelHeaderH   = int32(26)
	turnPanelTopPad    = int32(10)
	turnPanelBottomPad = int32(10)
	turnPanelRowH      = int32(28)
)

// Per-row layout offsets: inset margin, active-row marker X, inactive-row spine tick, label left edge.
const (
	turnRowInset   = int32(10)
	turnRowMarkerX = int32(10)
	turnRowSpineX  = int32(6)
	turnRowSpineW  = int32(4)
	turnRowLabelX  = int32(22)
)

// Per-row fill / outline alphas: active row glass fill + outline; inactive row spine tick.
const (
	turnRowActiveFillAlpha    = uint8(96)
	turnRowActiveOutlineAlpha = uint8(235)
	turnRowSpineAlpha         = uint8(220)
)

// turnPanelHeight is the panel's pixel height for n rows. Shared by drawTurnPanel and TurnPanelBottomY so they can't drift.
func turnPanelHeight(n int) int32 {
	return turnPanelHeaderH + turnPanelTopPad + int32(n)*turnPanelRowH + turnPanelBottomPad
}

// TurnPanelBottomY returns the panel's bottom-edge Y for the action-log docking below it. Out of combat turnForecastBuf is stale, but no panel is painted then, so report the minimap bottom.
func TurnPanelBottomY(g *core.GameState) int32 {
	turns := turnForecastBuf
	if !g.Battle.Active() || len(turns) == 0 {
		return MinimapBottomY()
	}
	return MinimapBottomY() + hudColumnGap + turnPanelHeight(len(turns))
}

func drawTurnPanel(g *core.GameState, assets Resources) {
	turns := turnForecastBuf
	if len(turns) == 0 {
		return
	}
	w := turnPanelW
	x := hudEdgePad
	y := MinimapBottomY() + hudColumnGap
	rowH := turnPanelRowH
	h := turnPanelHeight(len(turns))

	drawPanelCard(x, y, w, h)

	// Title band.
	drawTextWithShadow(assets.hudFont, "Turn Order", float32(x+turnRowInset), float32(y+5), FontSmall, textHint)
	drawGiltRule(x+turnRowInset, y+turnPanelHeaderH-4, w-2*turnRowInset, 1, 0.4)

	rowsTop := y + turnPanelHeaderH + turnPanelTopPad

	// Actor the player is aiming at, so the target lights up here too.
	targetEnemy, targetAlly := -1, -1
	if targetingEnemy(g) {
		targetEnemy = core.SelectedEnemySlot(g)
	} else if targetingAlly(g) {
		targetAlly = g.Battle.PartyTarget
	}

	// Sequence thread — a faint vertical line through the row markers, tying the queue into one strand. Skipped for a single row.
	if len(turns) > 1 {
		threadX := x + turnRowInset + turnRowSpineX + turnRowSpineW/2 // center of the tick column
		threadTop := rowsTop + rowH/2
		threadH := int32(len(turns)-1) * rowH
		rl.DrawRectangle(threadX, threadTop, 1, threadH, fadeColor(inkDim, 0.32))
		drawDiamondPip(float32(threadX), float32(threadTop+threadH), 2, fadeColor(inkDim, 0.5))
	}

	for i, turn := range turns {
		rowY := rowsTop + int32(i)*rowH
		col := turnEntryColor(turn)

		rowX := x + turnRowInset
		rowW := w - 2*turnRowInset
		rowInnerH := rowH - 4

		if i == 0 {
			drawGlassPane(rowX, rowY, rowW, rowInnerH, colorWithAlpha(col, turnRowActiveFillAlpha))
			drawSmallPanelOutline(rowX, rowY, rowW, rowInnerH, colorWithAlpha(col, turnRowActiveOutlineAlpha))
			cx := float32(rowX + turnRowMarkerX)
			cy := float32(rowY) + float32(rowInnerH)/2
			drawArrowMarker(rl.NewVector2(cx-2, cy), 8, 0, 6, col)
		} else {
			drawClassRail(rowX+turnRowSpineX, rowY+4, turnRowSpineW, rowInnerH-8, colorWithAlpha(col, turnRowSpineAlpha))
		}

		// Aim cue: the targeted actor gets a bright gilt ring + right-edge marker.
		if (turn.Enemy && turn.Index == targetEnemy) || (!turn.Enemy && turn.Index == targetAlly) {
			drawSmallPanelOutline(rowX, rowY, rowW, rowInnerH, fadeColor(giltBright, pulseHalo()))
			drawDiamondPip(float32(rowX+rowW)-7, float32(rowY)+float32(rowInnerH)/2, 3, giltBright)
		}

		labelX := rowX + turnRowLabelX
		labelSize := FontSmall
		labelMeasure := measureTurnLabel(assets.hudFont, turn.Label)
		labelY := float32(rowY) + (float32(rowInnerH)-labelMeasure.Y)/2 - 1
		drawTextWithShadow(assets.hudFont, turn.Label, float32(labelX), labelY, labelSize, col)
	}
}

// turnLabelMeasureCache memoizes MeasureTextEx for turn-row labels (panel paints rows every battle frame; labels change only on queue changes).
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

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

// turnForecastMax is how many upcoming turns the panel projects — the
// lookahead depth. Shared by the buffer capacity and the forecast call so
// the two can't drift.
const turnForecastMax = 10

// turnForecastBuf is reused across frames so the per-frame forecast
// doesn't allocate. CacheTurnForecastForFrame populates it once at the
// top of the battle HUD pass; TurnPanelBottomY and drawTurnPanel then
// read the same slice, eliminating the duplicate TurnForecast call
// (action-log layout used to invoke it via TurnPanelBottomY, and the
// turn panel itself reinvoked it on the very next line).
var turnForecastBuf = make([]core.TurnEntry, 0, turnForecastMax)

// CacheTurnForecastForFrame computes the turn forecast once and stores
// it for downstream HUD consumers. Called from DrawOverlay's battle
// branch before either drawBattleHUD or drawTurnPanel runs.
func CacheTurnForecastForFrame(g *core.GameState) {
	turnForecastBuf = core.TurnForecastInto(g, turnForecastBuf, turnForecastMax)
}

// turnPanelHeaderH is the title band ("Turn Order" + underline) atop the
// rows. turnPanelTopPad / turnPanelBottomPad bracket the rows beneath it.
const (
	turnPanelHeaderH   = int32(26)
	turnPanelTopPad    = int32(10)
	turnPanelBottomPad = int32(10)
	turnPanelRowH      = int32(28)
)

// Per-row layout offsets inside the turn panel. turnRowInset is the left/right
// margin from the panel edge to the row rect (the row width is the panel width
// minus 2×inset); turnRowMarkerX seats the active-row arrow marker; the spine
// pair places the inactive-row class-color tick; turnRowLabelX is the label's
// left edge. Named so the row geometry tunes in one block instead of as bare
// +N offsets scattered through drawTurnPanel.
const (
	turnRowInset   = int32(10)
	turnRowMarkerX = int32(10)
	turnRowSpineX  = int32(6)
	turnRowSpineW  = int32(4)
	turnRowLabelX  = int32(22)
)

// Per-row fill / outline alphas (uint8) for the turn panel. The active row
// gets a translucent class-tinted glass fill under a near-opaque outline; the
// inactive rows get a class-tinted spine tick. Named so the row chrome's
// alpha intent reads at a glance.
const (
	turnRowActiveFillAlpha    = uint8(96)
	turnRowActiveOutlineAlpha = uint8(235)
	turnRowSpineAlpha         = uint8(220)
)

// turnPanelHeight is the panel's pixel height for n forecast rows. Shared by
// the draw (drawTurnPanel) and the docked action-log's bottom-edge read
// (TurnPanelBottomY) so the two can't drift on the row height / pad math.
func turnPanelHeight(n int) int32 {
	return turnPanelHeaderH + turnPanelTopPad + int32(n)*turnPanelRowH + turnPanelBottomPad
}

// TurnPanelBottomY returns the Y screen coordinate of the bottom
// edge of the turn-order panel — used by the action-log panel that
// docks below it on the same left edge. Returns the minimap bottom
// when the queue is empty (no panel painted, so action log slots up).
//
// turnForecastBuf is only refreshed during the battle HUD pass, so out
// of combat it holds the last fight's stale forecast. The turn panel is
// likewise only painted in battle (drawTurnPanel runs from DrawOverlay's
// battle branch), so when no battle is active there's no panel to dock
// below — report the minimap bottom regardless of the stale buffer.
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

	// Title band — "Turn Order" over a gilt hairline so the forecast names
	// itself rather than relying on the reader to infer it from the rows.
	drawTextWithShadow(assets.hudFont, "Turn Order", float32(x+turnRowInset), float32(y+5), FontSmall, textHint)
	drawGiltRule(x+turnRowInset, y+turnPanelHeaderH-4, w-2*turnRowInset, 1, 0.4)

	rowsTop := y + turnPanelHeaderH + turnPanelTopPad

	// Which actor (if any) the player is currently aiming at, so the targeted
	// enemy / ally lights up here too — not just on the roster / in the world.
	targetEnemy, targetAlly := -1, -1
	if targetingEnemy(g) {
		targetEnemy = core.SelectedEnemySlot(g)
	} else if targetingAlly(g) {
		targetAlly = g.Battle.PartyTarget
	}

	// Sequence thread — a faint vertical line stitched down through the row
	// markers (under them), tying the forecast into one strand the way a
	// lineage chart threads its entries. Static chrome: it runs from the
	// acting row's marker to the last row so the queue reads as "this, then
	// these," not seven disconnected chips. Skipped for a single row (nothing
	// to thread).
	if len(turns) > 1 {
		threadX := x + turnRowInset + turnRowSpineX + turnRowSpineW/2 // center of the per-row tick column
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

		// Aim cue: the actor the player is currently targeting gets a bright
		// gilt ring + a right-edge marker, spotlighting them in the forecast.
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

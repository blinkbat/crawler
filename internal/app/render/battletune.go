package render

import (
	"fmt"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Combat Tuning panel geometry — a left-edge column (not a centered card) so the
// centered foe/party stage stays visible while values are dialed live.
const (
	tunePanelX   = float32(12)
	tunePanelTop = float32(40)
	tunePanelW   = float32(312)
	tuneRowH     = float32(20)
	tuneHeaderH  = float32(26)
	tuneFootH    = float32(20)
	tuneBarW     = float32(58)
)

// drawCombatTuneOverlay paints the Debug ▸ Combat Tuning panel: one slider row per
// adjustable value (label · number · gauge, with ◂▸ arrows on the cursored row),
// then Reset / Dump / Close. The numbers are shown so a dialed-in look can be read
// back off-screen (or dumped to file) and baked into DefaultBattleTuning.
func drawCombatTuneOverlay(g *core.GameState, assets Resources) {
	font := assets.hudFont
	rows := core.BattleTuneMenuCount()
	h := tuneHeaderH + tuneRowH*float32(rows) + tuneFootH
	panel := rl.NewRectangle(tunePanelX, tunePanelTop, tunePanelW, h)
	rl.DrawRectangleRec(panel, fadeColor(rl.Black, 0.74))
	rl.DrawRectangleLinesEx(panel, 1, fadeColor(giltBright, 0.5))

	drawTextWithShadow(font, "COMBAT TUNING", tunePanelX+12, tunePanelTop+7, FontSmall, giltBright)

	sliders := core.BattleTuneSliderCount()
	for i := 0; i < rows; i++ {
		y := tunePanelTop + tuneHeaderH + tuneRowH*float32(i)
		sel := g.CombatTuneIndex == i
		if sel {
			rl.DrawRectangleRec(rl.NewRectangle(tunePanelX+3, y-1, tunePanelW-6, tuneRowH-1), fadeColor(giltBright, 0.18))
		}
		textCol := fadeColor(textPrimary, 0.62)
		if sel {
			textCol = textPrimary
		}
		if i >= sliders { // trailing action rows
			drawTextWithShadow(font, tuneActionLabel(i), tunePanelX+12, y+3, FontTiny, textCol)
			continue
		}
		s, _ := core.BattleTuneSliderAt(i)
		drawTextWithShadow(font, s.Label, tunePanelX+12, y+3, FontTiny, textCol)
		// Numeric value, right-aligned just left of the gauge.
		barX := tunePanelX + tunePanelW - tuneBarW - 12
		val := fmt.Sprintf("%.2f", core.BattleTuneValue(&g.BattleTuning, i))
		drawTextRightAligned(font, val, barX-8, y+3, FontTiny, textCol)
		// Gauge: track + gilt fill + outline (same chrome as the retro sliders).
		barY := y + 5
		fw := int32((tuneBarW - 2) * core.BattleTuneFrac(&g.BattleTuning, i))
		drawIntensityGauge(int32(barX), int32(barY), int32(tuneBarW), 9, fw, fadeColor(giltBright, 0.72))
		if sel {
			cy := barY + 4.5
			col := fadeColor(giltBright, 0.85)
			drawArrowMarker(rl.NewVector2(barX-8, cy), -6, 0, 5, col)
			drawArrowMarker(rl.NewVector2(barX+tuneBarW+8, cy), 6, 0, 5, col)
		}
	}
	// Footer hint via the shared glyph system (controller-first), left-anchored in the
	// footer band like the rest of this debug column.
	DrawHintBarLeft(font, []HintSeg{
		Hint("Adjust", GlyphLeftRight),
		Hint("Row", GlyphUpDown),
		Hint("Confirm", GlyphA),
		Hint("Close", GlyphB),
	}, tunePanelX+12, tunePanelTop+h-tuneFootH+2, FontTiny)
}

// tuneActionLabel is the caption for the trailing (non-slider) Combat Tuning rows.
func tuneActionLabel(i int) string {
	switch {
	case i == core.BattleTuneResetRow():
		return "Reset to defaults"
	case i == core.BattleTuneDumpRow():
		return "Dump values → " + core.BattleTuneDumpFileName
	case i == core.BattleTuneCloseRow():
		return "Close"
	default:
		// An inserted row not wired here — surface it rather than mislabel "Close".
		return "?"
	}
}

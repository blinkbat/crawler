package render

import (
	"fmt"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// drawPanelsQuests renders the journal inside the char menu's Quests tab
// (the panels overlay supplies the card chrome + tab strip; this just fills
// the body rect). Read-only: a tally header, then a two-line row per quest
// (title + muted description) with the PanelsRowCursor row highlighted;
// completed quests render muted with a "— Complete" suffix. The journal is
// empty for now, so the common case is the placeholder line.
func drawPanelsQuests(g core.GameState, assets Resources, body rl.Rectangle) {
	font := assets.hudFont
	quests := g.Quests
	if len(quests) == 0 {
		drawTextWithShadow(font, "No quests yet.", body.X+8, body.Y+8, FontBody, textMuted)
		return
	}

	tally := fmt.Sprintf("%d active   %d complete",
		core.ActiveQuestCount(quests), core.CompletedQuestCount(quests))
	drawTextWithShadow(font, tally, body.X+8, body.Y+4, FontSmall, textLabel)

	const rowH = float32(56)
	rowY := body.Y + 30
	for i, q := range quests {
		if rowY+rowH > body.Y+body.Height {
			break // don't overflow the body rect
		}
		if i == g.PanelsRowCursor {
			DrawSelectedRowI(int32(body.X), int32(rowY-2), int32(body.Width), int32(rowH-6))
		}
		titleCol := textPrimary
		titleText := q.Title
		if q.IsComplete() {
			titleCol = textMuted
			titleText = q.Title + "  — Complete"
		}
		drawTextWithShadow(font, titleText, body.X+8, rowY+2, FontBody, titleCol)
		drawTextWithShadow(font, q.Desc, body.X+8, rowY+26, FontSmall, textMuted)
		rowY += rowH
	}
}

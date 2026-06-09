package render

import (
	"fmt"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// drawPanelsQuests renders the Journal tab inside the char menu. The journal
// hosts two sub-views — the quest log and the bestiary — switched with
// Left/Right (g.JournalTab). The panels overlay supplies the card chrome +
// top tab strip; this draws the sub-tab header strip, then fills the rest of
// the body with the active sub-view. Both views are read-only; PanelsRowCursor
// scrolls whichever list is showing.
func drawPanelsQuests(g core.GameState, assets Resources, body rl.Rectangle) {
	font := assets.hudFont
	headerH := drawJournalSubtabHeader(font, g.JournalTab, body)
	inner := rl.NewRectangle(body.X, body.Y+headerH, body.Width, body.Height-headerH)
	switch g.JournalTab {
	case core.JournalBestiary:
		drawJournalBestiary(g, font, inner)
	default:
		drawJournalQuests(g, font, inner)
	}
}

// drawJournalSubtabHeader paints the "Quests | Bestiary" sub-tab strip at the
// top of the Journal body — active label in primary ink with an accent
// underline, the other muted. Returns the vertical space it consumed so the
// caller can place the sub-view below it.
func drawJournalSubtabHeader(font rl.Font, active core.JournalSubtab, body rl.Rectangle) float32 {
	x := body.X + 8
	const gap = float32(22)
	for s := core.JournalSubtab(0); s < core.JournalSubtabCount; s++ {
		label := core.JournalSubtabLabel(s)
		col := textMuted
		if s == active {
			col = textPrimary
		}
		drawTextWithShadow(font, label, x, body.Y+2, FontBody, col)
		w := rl.MeasureTextEx(font, label, FontBody, 1).X
		if s == active {
			rl.DrawRectangle(int32(x), int32(body.Y+2+FontBody+2), int32(w), 2, inkAccent)
		}
		x += w + gap
	}
	return FontBody + 14
}

// journalScrollFirst returns the index of the first row a journal list
// should draw so the cursored row stays inside a window of `visible` rows —
// the input side walks the full list, so without this the highlight scrolls
// off the bottom of the body rect.
func journalScrollFirst(cursor, count, visible int) int {
	if visible < 1 {
		visible = 1
	}
	first := cursor - visible + 1
	if maxFirst := count - visible; first > maxFirst {
		first = maxFirst
	}
	if first < 0 {
		first = 0
	}
	return first
}

// drawJournalQuests fills the journal body with the quest log: a tally header
// then a two-line row per quest (title + muted description), the cursor row
// highlighted; completed quests render muted with a "— Complete" suffix. The
// journal is empty for now, so the common case is the placeholder line.
func drawJournalQuests(g core.GameState, font rl.Font, body rl.Rectangle) {
	quests := g.Quests
	if len(quests) == 0 {
		drawTextWithShadow(font, "No quests yet.", body.X+8, body.Y+8, FontBody, textMuted)
		return
	}

	tally := fmt.Sprintf("%d active   %d complete",
		core.ActiveQuestCount(quests), core.CompletedQuestCount(quests))
	drawTextWithShadow(font, tally, body.X+8, body.Y+4, FontSmall, textLabel)

	const rowH = float32(56)
	listTop := body.Y + 30
	visible := int((body.Y + body.Height - listTop) / rowH)
	first := journalScrollFirst(g.PanelsRowCursor, len(quests), visible)
	rowY := listTop
	for i := first; i < len(quests); i++ {
		q := quests[i]
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

// drawJournalBestiary fills the journal body with the bestiary: one row per
// foe the party has SEEN (defeated at least once or scanned — never-met kinds
// stay hidden so the list isn't a spoiler). A known kind (5 kills or scanned)
// shows its real HP in claret and an "identified" tag; an unidentified kind
// shows "HP ???" and progress toward the 5-kill threshold. Read-only;
// PanelsRowCursor highlights the focused row.
func drawJournalBestiary(g core.GameState, font rl.Font, body rl.Rectangle) {
	seen := g.Bestiary.SeenKinds()
	if len(seen) == 0 {
		drawTextWithShadow(font, "No foes recorded yet — defeat or Scan enemies to fill the bestiary.",
			body.X+8, body.Y+8, FontBody, textMuted)
		return
	}

	tally := fmt.Sprintf("%d of %d kinds recorded", len(seen), len(core.EnemyKinds()))
	drawTextWithShadow(font, tally, body.X+8, body.Y+4, FontSmall, textLabel)

	const rowH = float32(48)
	listTop := body.Y + 28
	visible := int((body.Y + body.Height - listTop) / rowH)
	first := journalScrollFirst(g.PanelsRowCursor, len(seen), visible)
	rowY := listTop
	for i := first; i < len(seen); i++ {
		kind := seen[i]
		if rowY+rowH > body.Y+body.Height {
			break // don't overflow the body rect
		}
		if i == g.PanelsRowCursor {
			DrawSelectedRowI(int32(body.X), int32(rowY-2), int32(body.Width), int32(rowH-6))
		}
		def := core.EnemyInfo(kind)
		entry := g.Bestiary.Entry(kind)
		drawTextWithShadow(font, def.Name, body.X+8, rowY+2, FontBody, textPrimary)

		if g.Bestiary.Knows(kind) {
			tag := "studied"
			if entry.Scanned {
				tag = "scanned"
			}
			hp := fmt.Sprintf("HP %d", def.MaxHP)
			drawTextWithShadow(font, hp, body.X+8, rowY+24, FontSmall, barEnemyHP)
			meta := fmt.Sprintf("   •   defeated %d   •   identified (%s)", entry.Kills, tag)
			hpW := rl.MeasureTextEx(font, hp, FontSmall, 1).X
			drawTextWithShadow(font, meta, body.X+8+hpW, rowY+24, FontSmall, textMuted)
		} else {
			detail := fmt.Sprintf("HP ???   •   defeated %d / %d to identify", entry.Kills, core.BestiaryIDKills)
			drawTextWithShadow(font, detail, body.X+8, rowY+24, FontSmall, textMuted)
		}
		rowY += rowH
	}
}

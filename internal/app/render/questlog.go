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
func drawPanelsQuests(g *core.GameState, assets Resources, body rl.Rectangle) {
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
	drawTextTabStrip(font, body.X+journalRowInsetX, body.Y+2, int(core.JournalSubtabCount), int(active),
		func(i int) string { return core.JournalSubtabLabel(core.JournalSubtab(i)) },
		func(s string) float32 { return journalMeasureCache.measure(font, s, FontBody, 1).X },
		textPrimary, 22, true)
	return FontBody + 14
}

// Journal list rhythm — ONE set of metrics shared by both sub-views (Quests
// and Bestiary). Both lists draw the identical two-line row anatomy (FontBody
// title at +2, FontSmall detail at journalRowDetailDY) under the same sub-tab
// header, so they must page at the same stride; they had drifted apart on
// copy-tuned literals (rowH 56 vs 48, listTop +30 vs +28, detail +26 vs +24),
// which made flipping between Quests and Bestiary subtly "jump." Title spans
// 2..22, detail 26..42, leaving 10px of air under the 46px selection plate
// (journalRowH - 6).
const (
	journalRowH        = float32(52)
	journalListTopDY   = float32(30) // tally line is FontSmall at +4; list starts below it
	journalRowDetailDY = float32(26)
	// journalRowInsetX is the left inset shared by the sub-tab header, the tally
	// line, the row text, and the selection plate so they all align on one edge
	// (was a bare body.X+8 repeated across the sub-views).
	journalRowInsetX = float32(8)
)

// The Journal tab re-measures the same handful of stable strings and
// re-fmt.Sprintf's the same tally / row text every frame it's open. These
// caches mirror the package's measureCache + enemyHPLabelCache conventions so
// the open panel doesn't burn a cgo text-measure and two heap-allocating
// Sprintfs per visible row per frame; all change only on quest/bestiary events.
var journalMeasureCache measureCache

// journalTally memoizes the two tally-header strings ("N active M complete" /
// "S of T kinds recorded") per count pair, keyed by (a, b, bestiary) so the two
// formats can't collide. Bounded by the small set of count pairs seen in play.
var journalTallyCache = map[[3]int]string{}

func journalTally(a, b int, bestiary bool) string {
	flag := 0
	if bestiary {
		flag = 1
	}
	k := [3]int{a, b, flag}
	if s, ok := journalTallyCache[k]; ok {
		return s
	}
	var s string
	if bestiary {
		s = fmt.Sprintf("%d of %d kinds recorded", a, b)
	} else {
		s = fmt.Sprintf("%d active   %d complete", a, b)
	}
	journalTallyCache[k] = s
	return s
}

// bestiaryRowText holds a bestiary row's pre-formatted detail strings. `hp` is
// drawn first (claret when identified, muted "HP ???" when not); `segs` are the
// muted follow-on facts, each drawn after a small DRAWN diamond pip separator
// (the old inline "•" bullets fell out of the procedural font atlas and rendered
// as "?", so the separators are now pixel symbols, not glyphs).
type bestiaryRowText struct {
	hp   string
	segs []string
}

type bestiaryRowKey struct {
	kind    core.EnemyKind
	kills   int
	scanned bool
	known   bool
}

// bestiaryRowStrings memoizes a row's formatted strings per (kind, kills,
// scanned, known) so the per-row fmt.Sprintf calls don't run every frame the
// Bestiary sub-tab is open. Bounded: enemy kinds × kill counts actually seen.
var bestiaryRowCache = map[bestiaryRowKey]bestiaryRowText{}

// drawBestiaryRowDetail paints a row's HP value (in hpCol) followed by each meta
// segment in muted type, each preceded by a small DRAWN diamond pip — the pixel-
// symbol replacement for the old "•" bullets that the font atlas couldn't render
// (they showed as "?"). Pips are font-independent, so they always read.
func drawBestiaryRowDetail(font rl.Font, t bestiaryRowText, x, y float32, hpCol rl.Color) {
	drawTextWithShadow(font, t.hp, x, y, FontSmall, hpCol)
	cursor := x + journalMeasureCache.measure(font, t.hp, FontSmall, 1).X
	const sepGap = float32(9)
	midY := y + FontSmall/2
	for _, seg := range t.segs {
		cursor += sepGap
		drawDiamondPip(cursor, midY, 2.2, fadeColor(giltDim, 0.7))
		cursor += sepGap
		drawTextWithShadow(font, seg, cursor, y, FontSmall, textMuted)
		cursor += journalMeasureCache.measure(font, seg, FontSmall, 1).X
	}
}

func bestiaryRowStrings(kind core.EnemyKind, maxHP, kills int, scanned, known bool) bestiaryRowText {
	k := bestiaryRowKey{kind: kind, kills: kills, scanned: scanned, known: known}
	if t, ok := bestiaryRowCache[k]; ok {
		return t
	}
	var t bestiaryRowText
	if known {
		tag := "studied"
		if scanned {
			tag = "scanned"
		}
		t.hp = fmt.Sprintf("HP %d", maxHP)
		t.segs = []string{
			fmt.Sprintf("defeated %d", kills),
			fmt.Sprintf("identified (%s)", tag),
		}
	} else {
		t.hp = "HP ???"
		t.segs = []string{fmt.Sprintf("defeated %d / %d to identify", kills, core.BestiaryIDKills)}
	}
	bestiaryRowCache[k] = t
	return t
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
func drawJournalQuests(g *core.GameState, font rl.Font, body rl.Rectangle) {
	quests := g.Quests
	if len(quests) == 0 {
		drawEmptyLedgerNote(font, body, "No quests yet.",
			"Deeds taken on will be recorded here.")
		return
	}

	tally := journalTally(core.ActiveQuestCount(quests), core.CompletedQuestCount(quests), false)
	drawTextWithShadow(font, tally, body.X+journalRowInsetX, body.Y+4, FontSmall, textLabel)

	forEachJournalRow(body, g.PanelsRowCursor, len(quests), func(i int, rowY float32) {
		q := quests[i]
		titleCol := textPrimary
		titleText := q.Title
		if q.IsComplete() {
			titleCol = textMuted
			titleText = q.Title + "  — Complete"
		}
		drawTextWithShadow(font, titleText, body.X+journalRowInsetX, rowY+2, FontBody, titleCol)
		drawTextWithShadow(font, q.Desc, body.X+journalRowInsetX, rowY+journalRowDetailDY, FontSmall, textMuted)
	})
}

// forEachJournalRow walks the visible window of a journal list — the Quests
// and Bestiary sub-views share it — painting the selection plate on the cursor
// row and calling fn(i, rowY) to draw each row's content. Centralizes the
// paging math, the body-overflow guard, and the -2/-6 selection-plate insets
// so the two sub-views can't drift on copy-tuned literals.
func forEachJournalRow(body rl.Rectangle, cursor, count int, fn func(i int, rowY float32)) {
	listTop := body.Y + journalListTopDY
	visible := int((body.Y + body.Height - listTop) / journalRowH)
	first := journalScrollFirst(cursor, count, visible)
	rowY := listTop
	for i := first; i < count; i++ {
		if rowY+journalRowH > body.Y+body.Height {
			break // don't overflow the body rect
		}
		if i == cursor {
			DrawSelectedRowI(int32(body.X), int32(rowY)-focusPlateInsetY, int32(body.Width), int32(journalRowH-6))
		}
		fn(i, rowY)
		rowY += journalRowH
	}
}

// drawJournalBestiary fills the journal body with the bestiary: one row per
// foe the party has SEEN (defeated at least once or scanned — never-met kinds
// stay hidden so the list isn't a spoiler). A known kind (5 kills or scanned)
// shows its real HP in claret and an "identified" tag; an unidentified kind
// shows "HP ???" and progress toward the 5-kill threshold. Read-only;
// PanelsRowCursor highlights the focused row.
// bestiarySeenBuf is the reused scratch slice for the per-frame Bestiary tab
// draw, so SeenKindsInto doesn't allocate a fresh []EnemyKind every frame.
var bestiarySeenBuf []core.EnemyKind

func drawJournalBestiary(g *core.GameState, font rl.Font, body rl.Rectangle) {
	seen := g.Bestiary.SeenKindsInto(bestiarySeenBuf)
	bestiarySeenBuf = seen[:0]
	if len(seen) == 0 {
		drawEmptyLedgerNote(font, body, "No foes recorded yet.",
			"Defeat or Scan enemies to fill the bestiary.")
		return
	}

	tally := journalTally(len(seen), core.EnemyKindCount(), true)
	drawTextWithShadow(font, tally, body.X+journalRowInsetX, body.Y+4, FontSmall, textLabel)

	forEachJournalRow(body, g.PanelsRowCursor, len(seen), func(i int, rowY float32) {
		kind := seen[i]
		def := core.EnemyInfo(kind)
		entry := g.Bestiary.Entry(kind)
		drawTextWithShadow(font, def.Name, body.X+journalRowInsetX, rowY+2, FontBody, textPrimary)

		known := g.Bestiary.Knows(kind)
		rowText := bestiaryRowStrings(kind, def.MaxHP, entry.Kills, entry.Scanned, known)
		hpCol := textMuted
		if known {
			hpCol = barEnemyHP
		}
		drawBestiaryRowDetail(font, rowText, body.X+journalRowInsetX, rowY+journalRowDetailDY, hpCol)
	})
}

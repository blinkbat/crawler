package render

import (
	"fmt"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// drawPanelsQuests renders the Journal tab: a sub-tab header (Quests/Bestiary, switched via g.JournalTab) then the active read-only sub-view.
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

// drawJournalSubtabHeader paints the "Quests | Bestiary" sub-tab strip and returns the vertical space it consumed.
func drawJournalSubtabHeader(font rl.Font, active core.JournalSubtab, body rl.Rectangle) float32 {
	drawTextTabStrip(font, body.X+journalRowInsetX, body.Y+2, int(core.JournalSubtabCount), int(active),
		func(i int) string { return core.JournalSubtabLabel(core.JournalSubtab(i)) },
		tabLabelMeasurer(&journalMeasureCache, font),
		textPrimary, journalSubtabStripGap, true)
	return FontBody + 14
}

// Journal list rhythm — ONE metric set shared by both sub-views so they page at the same stride and don't "jump" when flipping.
const (
	journalRowH           = float32(52)
	journalListTopDY      = float32(30) // tally line is FontSmall at +4; list starts below it
	journalRowDetailDY    = float32(26)
	journalRowInsetX      = float32(8)  // shared left inset for header, tally, rows, selection plate
	journalSubtabStripGap = float32(22) // inter-tab spacing for the Quests/Bestiary sub-tab strip
)

// Caches so the open Journal tab doesn't re-measure stable strings / re-Sprintf tally + row text every frame; all change only on quest/bestiary events.
var journalMeasureCache measureCache

// journalTallyCache memoizes the two tally-header strings, keyed (a, b, bestiary) so the formats can't collide.
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

// bestiaryRowText holds a row's pre-formatted detail strings: hp first, then muted segs separated by drawn diamond pips ("•" glyphs render as "?" in the font atlas).
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

// bestiaryRowCache memoizes a row's strings per (kind, kills, scanned, known) so the per-row Sprintfs don't run every frame.
var bestiaryRowCache = map[bestiaryRowKey]bestiaryRowText{}

// drawBestiaryRowDetail paints HP (hpCol) then each muted seg preceded by a drawn diamond pip (font-independent, unlike "•").
func drawBestiaryRowDetail(font rl.Font, t bestiaryRowText, x, y float32, hpCol rl.Color) {
	drawTextWithShadow(font, t.hp, x, y, FontSmall, hpCol)
	cursor := x + journalMeasureCache.measure(font, t.hp, FontSmall, canonicalSpacing(FontSmall)).X
	const sepGap = float32(9)
	midY := y + FontSmall/2
	for _, seg := range t.segs {
		cursor += sepGap
		drawDiamondPip(cursor, midY, 2.2, fadeColor(giltDim, 0.7))
		cursor += sepGap
		drawTextWithShadow(font, seg, cursor, y, FontSmall, textMuted)
		cursor += journalMeasureCache.measure(font, seg, FontSmall, canonicalSpacing(FontSmall)).X
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

// journalScrollFirst returns the first row index to draw so the cursor stays inside a window of `visible` rows.
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

// completedQuestTitleCache memoizes the "<title>  — Complete" row label so the
// Journal Quests tab doesn't concat a fresh string per completed row every frame
// it's open. Bounded by the distinct quest titles seen.
var completedQuestTitleCache = map[string]string{}

func completedQuestTitle(title string) string {
	if s, ok := completedQuestTitleCache[title]; ok {
		return s
	}
	s := title + "  — Complete"
	completedQuestTitleCache[title] = s
	return s
}

// drawJournalQuests fills the body with the quest log: tally header then a two-line row per quest; completed quests are muted with a "— Complete" suffix.
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
			titleText = completedQuestTitle(q.Title)
		}
		drawTextWithShadow(font, titleText, body.X+journalRowInsetX, rowY+2, FontBody, titleCol)
		drawTextWithShadow(font, q.Desc, body.X+journalRowInsetX, rowY+journalRowDetailDY, FontSmall, textMuted)
	})
}

// forEachJournalRow walks the visible window (shared by both sub-views), painting the cursor row's selection plate and calling fn(i, rowY) per row.
func forEachJournalRow(body rl.Rectangle, cursor, count int, fn func(i int, rowY float32)) {
	listTop := body.Y + journalListTopDY
	visible := int((body.Y + body.Height - listTop) / journalRowH)
	if visible < 1 {
		visible = 1 // always show at least the cursor row
	}
	first := journalScrollFirst(cursor, count, visible)
	// Bound the loop by the same floored `visible` used for paging so a too-short body still renders one row.
	rowY := listTop
	for i := first; i < count && i < first+visible; i++ {
		if i == cursor {
			DrawSelectedRowI(int32(body.X), int32(rowY)-focusPlateInsetY, int32(body.Width), int32(journalRowH)-selectionPlateShrinkY)
		}
		fn(i, rowY)
		rowY += journalRowH
	}
}

// drawJournalBestiary fills the body with one row per SEEN foe (never-met kinds stay hidden, no spoilers). Known kinds show real HP + tag; unidentified show "HP ???" and kill progress.
// bestiarySeenBuf is reused scratch so SeenKindsInto doesn't allocate per frame.
var bestiarySeenBuf []core.EnemyKind

func drawJournalBestiary(g *core.GameState, font rl.Font, body rl.Rectangle) {
	seen := g.Bestiary.SeenKindsInto(bestiarySeenBuf)
	bestiarySeenBuf = seen
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

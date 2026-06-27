package render

import (
	"crawler/internal/app/core"
	"fmt"
	"strconv"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// selectedGlassTint blends base toward glassWarm by t, so every highlighted glass surface shares one warm target.
func selectedGlassTint(base rl.Color, t float64) rl.Color {
	return core.MixColor(base, glassWarm, t)
}

// panelStatMeasureCache memoizes Stats-tab right-aligned value measures (change only on level-up/HP-spend/status).
var panelStatMeasureCache measureCache

func measurePanelStatValue(font rl.Font, text string, size float32) rl.Vector2 {
	// canonicalSpacing pairs this with drawTextWithShadow's tracking.
	return panelStatMeasureCache.measure(font, text, size, canonicalSpacing(size))
}

// panelsMapFooterCache memoizes the Map-tab "zoom: N cells" footer (zoom changes only on user action).
var panelsMapFooterCache struct {
	zoom int
	text string
}

func panelsMapFooterText(zoom int) string {
	if panelsMapFooterCache.text != "" && panelsMapFooterCache.zoom == zoom {
		return panelsMapFooterCache.text
	}
	panelsMapFooterCache.text = fmt.Sprintf("zoom: %d cells", zoom)
	panelsMapFooterCache.zoom = zoom
	return panelsMapFooterCache.text
}

// panelTabInfo is one tab's contract: body drawer + footer hint bar. Single
// per-tab row (was two parallel [PanelTabCount] arrays) so a new tab is one edit.
// footer values are static, built once at init (the segs never change), so the
// per-frame panels draw doesn't reallocate a hint bar + glyph slices each frame.
type panelTabInfo struct {
	draw   func(*core.GameState, Resources, rl.Rectangle)
	footer []HintSeg
}

// footerHintMapTab is the Map tab's controls — shown ONLY in the bottom footer bar
// (no duplicate hint inside the map body). Pan = left stick, zoom = right stick.
var footerHintMapTab = []HintSeg{
	Hint("Tabs", GlyphLB, GlyphRB),
	Hint("Pan", GlyphLeftStick),
	Hint("Zoom", GlyphRightStick),
	Hint("Close", GlyphB),
}

// footerHintCharacterTab adds the formation Swap to the member-tab hints.
var footerHintCharacterTab = []HintSeg{
	Hint("Tabs", GlyphLB, GlyphRB),
	Hint("Move", GlyphUpDown, GlyphLeftRight),
	Hint("Swap", GlyphX),
	Hint("Close", GlyphB),
}

// Sub-picker footer hints, built once (static) so an open picker doesn't realloc
// its hint bar each frame.
var (
	equipPickerHints     = []HintSeg{Hint("Equip", GlyphA), Hint("Cancel", GlyphB)}
	useTargetPickerHints = []HintSeg{Hint("Use", GlyphA), Hint("Cancel", GlyphB)}
	healPickerHints      = []HintSeg{Hint("Cast", GlyphA), Hint("Cancel", GlyphB)}
)

// panelTabs is the per-tab drawer + footer registry, indexed by tab (init asserts
// every row has both). Adding a tab = one row here, not two coordinated arrays.
var panelTabs = [core.PanelTabCount]panelTabInfo{
	core.PanelTabStats: {draw: drawPanelsStats, footer: footerHintCharacterTab},
	core.PanelTabEquipment: {draw: drawPanelsEquipment, footer: []HintSeg{
		Hint("Tabs", GlyphLB, GlyphRB),
		Hint("Member", GlyphLeftRight),
		Hint("Slot", GlyphUpDown),
		Hint("Change gear", GlyphA),
		Hint("Close", GlyphB),
	}},
	core.PanelTabItems: {draw: drawPanelsItems, footer: []HintSeg{
		Hint("Tabs", GlyphLB, GlyphRB),
		Hint("Item", GlyphUpDown),
		Hint("Use", GlyphX),
		Hint("Close", GlyphB),
	}},
	core.PanelTabSkills: {draw: drawPanelsSkills, footer: []HintSeg{
		Hint("Tabs", GlyphLB, GlyphRB),
		Hint("Member", GlyphLeftRight),
		Hint("Open trees", GlyphA),
		Hint("Use skill", GlyphX),
		Hint("Close", GlyphB),
	}},
	core.PanelTabQuests: {draw: drawPanelsQuests, footer: []HintSeg{
		Hint("Tabs", GlyphLB, GlyphRB),
		Hint("Quests / Bestiary", GlyphLeftRight),
		Hint("Scroll", GlyphUpDown),
		Hint("Close", GlyphB),
	}},
	core.PanelTabMap: {draw: drawPanelsMap, footer: footerHintMapTab},
}

func init() {
	for t := core.PanelTab(0); t < core.PanelTabCount; t++ {
		if panelTabs[t].draw == nil {
			panic(fmt.Sprintf("render/panels: panelTabs missing a drawer for tab %d", int(t)))
		}
		if panelTabs[t].footer == nil {
			panic(fmt.Sprintf("render/panels: panelTabs missing a footer hint for tab %d", int(t)))
		}
	}
}

// drawPanelsBody paints the six-tab game-panels overlay, routing by g.PanelsTab. Open-gate lives in menuFadeDrawer.
func drawPanelsBody(g *core.GameState, assets Resources) {
	font := assets.Font()
	// No heading band — the tab strip IS the heading. Screen-relative card.
	card := drawScreenFractionScaffold(font, panelsOverlayWidthFrac, panelsOverlayHeightFrac, "")
	cardX, cardY := int32(card.X), int32(card.Y)
	cardW, cardH := int32(card.Width), int32(card.Height)
	drawTomeBinding(cardX, cardY, cardW, cardH)

	// Tab strip: flat glass-tile labels; active tab gets brighter glass + a gilt underline.
	tabH := overlayTabHeight + 4
	tabRowY := cardY + 14
	tabPad := overlayTabPadding
	tabW := (cardW - 24 - tabPad*int32(core.PanelTabCount-1)) / int32(core.PanelTabCount)
	for t := core.PanelTab(0); t < core.PanelTabCount; t++ {
		tx := cardX + 12 + int32(t)*(tabW+tabPad)
		active := t == g.PanelsTab
		bg := glassMid
		txt := textMuted
		if active {
			bg = selectedGlassTint(glassMid, 0.65)
			txt = textPrimary
		}
		drawGlassPane(tx, tabRowY, tabW, tabH, bg)
		if active {
			drawGiltRule(tx+8, tabRowY+tabH-3, tabW-16, 2, 1.0) // gilt "you're here" underline
		}
		label := core.PanelTabLabel(t)
		m := measureTabLabel(font, label)
		drawTextWithShadow(font, label,
			float32(tx)+float32(tabW)/2-m.X/2,
			float32(tabRowY)+float32(tabH)/2-m.Y/2-1,
			FontBody, txt)
	}

	// Info strip on every tab: area name left, gold right. Shared chrome so it's always visible.
	const panelsInfoStripH = int32(22)
	// panelsStripGutter is the info-strip's side inset — modalGutterWide(24), a hair
	// wider than hudContentInsetX(22): the header band's chrome sits wider than the
	// body content inset below it.
	const panelsStripGutter = modalGutterWide
	infoY := tabRowY + tabH + 4
	areaName := g.Area.Name
	if areaName == "" {
		areaName = "Unknown"
	}
	drawTextWithShadow(font, areaName, float32(cardX+panelsStripGutter), float32(infoY), FontSmall, textPrimary)
	drawTextRightAligned(font, goldLabelFull(g.Gold), float32(cardX+cardW-panelsStripGutter), float32(infoY), FontSmall, borderActive)
	// Header rule under the info strip — wood-accent hairline with diamond termini.
	stripRuleY := infoY + panelsInfoStripH
	stripRuleCol := woodAccentRule
	drawPipCappedRule(cardX+panelsStripGutter, stripRuleY, cardW-2*panelsStripGutter, stripRuleCol, 1.8, stripRuleCol)

	bodyY := infoY + panelsInfoStripH + 6
	bodyRect := rl.NewRectangle(float32(cardX+hudContentInsetX), float32(bodyY),
		float32(cardW-2*hudContentInsetX), float32(cardY+cardH-panelsBodyBottomReserve-bodyY-overlayFooterReserve))

	tab := panelTabs[core.PanelTabStats]
	if int(g.PanelsTab) >= 0 && int(g.PanelsTab) < len(panelTabs) {
		tab = panelTabs[g.PanelsTab]
	}
	tab.draw(g, assets, bodyRect)
	drawModalFooterGlyphs(font, card, tab.footer)

	// Sub-modals painted on top of the whole overlay.
	if g.PanelsTab == core.PanelTabEquipment && g.EquipPickerOpen {
		drawEquipPicker(g, assets)
	}
	if g.HealPickOpen {
		drawHealPicker(g, assets)
	}
	if g.UseTargetOpen {
		drawUseTargetPicker(g, assets)
	}
	if g.SkillTreeOpen {
		DrawSkillTreeModal(g, assets)
	}
}

func drawTomeBinding(cardX, cardY, cardW, cardH int32) {
	if cardW < 520 || cardH < 360 {
		return
	}
	inset := int32(uiCellInsetX)
	leftPage := rl.NewRectangle(float32(cardX+inset), float32(cardY+inset), float32(cardW/2-inset-6), float32(cardH-inset*2))
	rightPage := rl.NewRectangle(float32(cardX+cardW/2+6), float32(cardY+inset), float32(cardW/2-inset-6), float32(cardH-inset*2))
	rl.DrawRectangleGradientEx(leftPage,
		fadeColor(inkPrimary, 0.035), fadeColor(shadowHeavy, 0.035),
		fadeColor(shadowHeavy, 0.055), fadeColor(shadowHeavy, 0.075))
	rl.DrawRectangleGradientEx(rightPage,
		fadeColor(shadowHeavy, 0.055), fadeColor(shadowHeavy, 0.075),
		fadeColor(inkPrimary, 0.035), fadeColor(shadowHeavy, 0.035))

	// Faint dappled ledger ruling across each page — the same painterly alpha-speckle
	// the HUD pane edges use (speckleHairline), so the tome reads as ruled parchment.
	ruleCol := fadeColor(woodInlay, 0.10)
	const tomeRulePitch = int32(34)
	for _, pg := range [2]rl.Rectangle{leftPage, rightPage} {
		rx := int32(pg.X) + 10
		rw := int32(pg.Width) - 20
		if rw < 2 {
			continue
		}
		for ry := int32(pg.Y) + tomeRulePitch; ry < int32(pg.Y+pg.Height)-6; ry += tomeRulePitch {
			speckleHairline(rx, ry, rw, 1, ruleCol)
		}
	}

	spineX := cardX + cardW/2
	spineTop := cardY + 26
	spineH := cardH - 52
	rl.DrawRectangleGradientH(spineX-8, spineTop, 16, spineH,
		fadeColor(shadowHeavy, 0.05), fadeColor(woodDark, 0.28))
	rl.DrawRectangle(spineX-1, spineTop+8, 2, spineH-16, fadeColor(woodInlay, 0.72))
	rl.DrawRectangle(spineX+3, spineTop+18, 1, spineH-36, fadeColor(giltDim, 0.24))

	studCol := fadeColor(giltDim, 0.55)
	for i := 0; i < 5; i++ {
		t := float32(i+1) / 6
		cy := float32(spineTop) + float32(spineH)*t
		drawDiamondPip(float32(spineX), cy, 2.4, studCol)
	}
}

// tabLabelMeasureCache memoizes panel-tab label measurements (fills once, stays warm).
var tabLabelMeasureCache measureCache

func measureTabLabel(font rl.Font, label string) rl.Vector2 {
	return tabLabelMeasureCache.measure(font, label, FontBody, 1)
}

// memberCardGutter is the single per-member-card layout unit: inter-column gap AND content inset, so they can't drift apart.
const memberCardGutter = float32(20)

// Reused per-cell classifier grids for drawPanelsMap (single-threaded; each frame overwrites the range it slices).
// Contents are cached across frames keyed on panelsMapClassCache (same scheme as the corner minimap) so a
// stationary player reuses the grids instead of reclassifying the whole window via mapSliceCell every frame.
var (
	panelsMapSliceBuf []bool
	panelsMapSeenBuf  []bool
	panelsMapRampBuf  []int8
	panelsMapColBuf   []rl.Color
)

// panelsMapClassKey fingerprints everything the Map-tab cell classification + tiling depends on: area (Path),
// player tile/level, pan offset, and the cell-grid dimensions (zoom). Like the minimap, fog reveals land on the
// step that moves the player, so the player tile covers fog freshness. The pixel rects are re-drawn every frame
// regardless (cheap); only the mapSliceCell classifications are gated on this key.
type panelsMapClassKey struct {
	path                       string
	tileX, tileZ, level        int
	panX, panZ, cellsX, cellsY int
	valid                      bool
}

var panelsMapClassCache panelsMapClassKey

// memberColumnBuf backs memberColumnLayout's returned slice (single-threaded, one consuming tab per frame).
var memberColumnBuf []rl.Rectangle

// memberColumnLayout returns the per-member column rects for the Stats/Equipment/Skills tabs.
func memberColumnLayout(body rl.Rectangle, count int) []rl.Rectangle {
	if count <= 0 {
		return nil
	}
	total := body.Width - memberCardGutter*float32(count-1)
	colW := total / float32(count)
	if cap(memberColumnBuf) < count {
		memberColumnBuf = make([]rl.Rectangle, count)
	}
	cols := memberColumnBuf[:count]
	for i := 0; i < count; i++ {
		cols[i] = rl.NewRectangle(body.X+float32(i)*(colW+memberCardGutter), body.Y, colW, body.Height)
	}
	return cols
}

// memberCardInner returns the writable inner region (X origin + width) of a member card column.
func memberCardInner(col rl.Rectangle) (innerX, innerW float32) {
	return col.X + memberCardGutter, col.Width - 2*memberCardGutter
}

// Shared identity-block geometry (the class sigil + name row of a member card).
const (
	cardTopInsetY    = float32(16) // first content row below the card top
	cardGlyphRadius  = float32(12) // class sigil radius (party-ribbon scale)
	cardNameGlyphGap = float32(12) // gap between the sigil and the name
)

// cardIdentityMetrics holds the per-tab vertical steps for the identity+vitals block.
// The Equipment-tab header and the Character-tab formation card pack at different
// pitches, so the steps are passed in rather than hard-coded in the shared helper.
type cardIdentityMetrics struct {
	nameStep, subStep, hpStep, mpStep float32
}

// drawCardIdentity draws the shared per-member identity + vitals block (class sigil,
// engraved name, "Lv N · row" sub-line, HP + MP bars) starting at (x, y0) across width w,
// returning the Y after the MP bar. metrics carries the per-tab vertical steps (the two
// party-tab cards differ only in how tightly they pack these rows).
func drawCardIdentity(font rl.Font, m core.PartyMember, x, w, y0 float32, nameCol, classCol rl.Color, metrics cardIdentityMetrics) float32 {
	drawClassGlyph(x+cardGlyphRadius, y0+FontHeading/2, cardGlyphRadius, m.Class, classCol)
	drawEngravedText(font, m.Name, x+cardGlyphRadius*2+cardNameGlyphGap, y0, FontHeading, nameCol)
	y := y0 + metrics.nameStep
	drawTextWithShadow(font, "Lv "+strconv.Itoa(m.Level)+" · "+core.RowLabel(m.HomeRow), x, y, FontBody, textMuted)
	y += metrics.subStep
	drawBar(font, x, y, w, barHeightCompact, "HP", m.HP, m.MaxHP, hpFillColor(m.HP, m.MaxHP), m.HP <= 0)
	y += metrics.hpStep
	drawBar(font, x, y, w, barHeightCompact, "MP", m.MP, m.MaxMP, barMP, m.HP <= 0)
	y += metrics.mpStep
	return y
}

// cardSubLineStep is the vertical step from a card's header/lead line to the
// sub-line beneath it — shared by the member-card metrics and the Items/Skills
// detail cards so the sub-line rhythm stays in lockstep.
const cardSubLineStep = float32(30)

// Equipment-tab card-header steps: the name row is one FontHeading line tall; the MP step
// is uiRowPitch (the card body below starts on the standard row grid).
var memberCardHeaderMetrics = cardIdentityMetrics{
	nameStep: FontHeading, // 36 — name's own line height, no extra gap
	subStep:  cardSubLineStep,
	hpStep:   36,
	mpStep:   float32(uiRowPitch), // 42
}

// cardDetailRowMetrics is the shared header rhythm for a glass detail card: content
// inset, title baseline, right-aligned value baseline, and sub-line baseline (all from
// the card's top-left). footUp is the foot-line baseline measured UP from the card
// bottom; belowGap is the breath between the strip and the footer hint below it (skill
// -tree detail strip only). Shared by the Skills-tab tree summary + that detail strip.
var cardDetailRowMetrics = struct{ insetX, titleY, valueY, subY, footUp, belowGap float32 }{
	insetX: 12, titleY: 8, valueY: 10, subY: 34, footUp: 22, belowGap: 4,
}

// Member-card column split: the right (vitals) column starts at memberCardBarSplit of
// the card width and spans memberCardBarFracW. Two tokens (not one + 1-x) so the
// float32 widths stay bit-identical to the original literals. Shared by the formation
// card + the Use target picker.
const (
	memberCardBarSplit = float32(0.46)
	memberCardBarFracW = float32(0.54)
)

// Character-tab formation-card horizontal spacing. Wider than the shared split so the
// card breathes: a roomier edge inset (left of the bars / right of the values), a clear
// gap between the HP/MP bars and the stat grid, and a gutter between the two stat columns.
const (
	formationCardInsetX  = float32(28)   // left/right content inset from the card edge
	formationBarsEndFrac = float32(0.42) // HP/MP bars right edge (frac of card width)
	formationGridSplit   = float32(0.50) // stat grid left edge (frac) — gap to the bars
	formationStatColGap  = float32(16)   // gutter between the two stat columns
)

// Character-tab formation-card steps: tighter than the header so the stat grid clears the pill.
var formationCardMetrics = cardIdentityMetrics{
	nameStep: 34,
	subStep:  cardSubLineStep,
	hpStep:   34,
	mpStep:   38,
}

// drawMemberCardShell paints the shared per-member card preamble: glass pane (washed
// when highlighted), the left class rail, and the gilt focus ring on highlight.
func drawMemberCardShell(rect rl.Rectangle, classCol rl.Color, highlight bool) {
	cardBG := glassMid
	if highlight {
		cardBG = selectedGlassTint(glassMid, 0.9)
	}
	drawGlassPaneRect(rect, cardBG)
	// Class accent rail flush to the left edge.
	drawClassRail(int32(rect.X), int32(rect.Y)+6, stripeWidth, int32(rect.Height)-12, classCol)
	if highlight {
		drawGiltFocusRing(rect)
	}
}

// drawPartyMemberCardHeader paints the shared per-member card header (class rail, name, "Lv N · row" sub-label,
// HP+MP bars) and returns the Y where tab content starts. highlight (cursored column) brightens name + washes body.
func drawPartyMemberCardHeader(font rl.Font, m core.PartyMember, col rl.Rectangle, highlight bool) float32 {
	classCol := classAccent(m.Class)
	drawMemberCardShell(col, classCol, highlight)

	innerX, innerW := memberCardInner(col)

	nameCol := textPrimary
	if !highlight {
		nameCol = textMuted
	}
	// Shared identity + vitals block (sigil, name, Lv·row, HP/MP); returns the next Y
	// (mpStep already advances past the MP bar to where tab content starts).
	return drawCardIdentity(font, m, innerX, innerW, col.Y+cardTopInsetY, nameCol, classCol, memberCardHeaderMetrics)
}

// drawPanelsStats renders the Character tab as a 2×2 FORMATION grid: each member's
// landscape card sits in its HomeRow/HomeCol quadrant (front rank on top, back on the
// bottom), so the panel reads as the formation you arrange (Use/□ picks up + swaps
// slots). Each card: identity + HP/MP on the left, stat grid + armor/XP/status right.
func drawPanelsStats(g *core.GameState, assets Resources, body rl.Rectangle) {
	font := assets.Font()
	if len(g.Party) == 0 {
		return
	}
	// Grid dims come from core (locked to 2×2 by its RowCount==ColCount==2 init
	// assert), so the layout tracks the formation instead of assuming party size 4.
	gut := memberCardGutter
	cols, rows := float32(core.ColCount), float32(core.RowCount)
	quadW := (body.Width - gut*(cols-1)) / cols
	quadH := (body.Height - gut*(rows-1)) / rows
	quadRect := func(row core.Row, col core.Col) rl.Rectangle {
		x := body.X + float32(col)*(quadW+gut)
		y := body.Y + float32(row)*(quadH+gut)
		return rl.NewRectangle(x, y, quadW, quadH)
	}
	for i := range g.Party {
		drawFormationCard(font, g, i, quadRect(g.Party[i].HomeRow, g.Party[i].HomeCol))
	}
}

// satietyStageColors tints the character-sheet satiety chip per stage — a green→red
// famine ramp reusing the shared palette (no new magic colors).
var satietyStageColors = [core.SatietyStageCount]rl.Color{
	core.SatietyFull:     statusRegen,
	core.SatietySated:    condEnemyScuffed,
	core.SatietyHungry:   condEnemyInjured,
	core.SatietyFamished: condEnemyBadlyWounded,
	core.SatietyStarving: statusStarving,
}

// drawFormationCard paints one member's landscape card in its 2×2 quadrant: class
// glyph + name + Lv·row + HP/MP on the left; stat grid + armor/XP + status on the
// right. Cursor = gilt focus ring; swap source (awaiting a partner) = green outline.
func drawFormationCard(font rl.Font, g *core.GameState, i int, quad rl.Rectangle) {
	m := g.Party[i]
	highlight := i == g.PanelsRowCursor
	classCol := classAccent(m.Class)

	drawMemberCardShell(quad, classCol, highlight)
	if i == g.PanelSwapSource {
		drawPanelOutline(int32(quad.X)-2, int32(quad.Y)-2, int32(quad.Width)+4, int32(quad.Height)+4, borderTarget)
	}

	leftX := quad.X + formationCardInsetX
	leftW := quad.Width*formationBarsEndFrac - formationCardInsetX
	rightX := quad.X + quad.Width*formationGridSplit
	rightW := quad.X + quad.Width - formationCardInsetX - rightX

	// --- Left: identity + vitals (shared block; formation packs tighter than the header) ---
	nameCol := textPrimary
	if !highlight {
		nameCol = textMuted
	}
	y := drawCardIdentity(font, m, leftX, leftW, quad.Y+cardTopInsetY, nameCol, classCol, formationCardMetrics)
	// Skip Starving here: the satiety chip below always shows it, so drawing it as a
	// status pill too would double "STARVING" on this card. Any higher-priority status
	// (Poisoned, Stunned, …) still outranks Starving in PartyStatus and shows normally.
	if kind, turns := core.PartyStatus(&g.Party[i]); kind != core.PartyStatusNone && kind != core.PartyStatusStarving {
		label := partyStatusTurnLabel(kind, turns)
		chipW := measurePanelStatValue(font, label, FontSmall).X + 20
		col, _ := partyStatusVisual(kind)
		drawStatusPill(font, leftX, y, chipW, 26, fadeColor(col, 0.28), fadeColor(col, 0.85), label, col, false)
	}
	// Satiety stage chip below the status pill — always shown so the player can watch
	// hunger climb the ladder before Starving bites (which is omitted from the pill above).
	stage := core.MemberStage(m)
	satLabel := core.SatietyStageLabel(stage)
	satCol := satietyStageColors[stage]
	satW := measurePanelStatValue(font, satLabel, FontSmall).X + 20
	// Sit below the status pill, but clamp inside the card so a short window can't
	// push the chip past the quadrant's bottom edge.
	const satChipH = float32(24)
	satY := y + 32
	if maxY := quad.Y + quad.Height - satChipH - 4; satY > maxY {
		satY = maxY
	}
	drawStatusPill(font, leftX, satY, satW, satChipH, fadeColor(satCol, 0.24), fadeColor(satCol, 0.82), satLabel, satCol, false)

	// --- Right: stat grid (2 cols × ceil(StatCount/2) rows) + armor/XP ---
	statColW := (rightW - formationStatColGap) / 2
	colPitch := statColW + formationStatColGap // column-1 starts a gutter past column-0
	rowH := barHeightCompact
	sy := quad.Y + cardTopInsetY
	// Effective stats fold in gear, combat buffs, and the starving penalty; show the
	// EFFECTIVE value and tint it vs base — green when raised, red when lowered.
	effStats := core.EffectiveStats(m)
	for s := core.Stat(0); s < core.StatCount; s++ {
		cellX := rightX + float32(int(s)%2)*colPitch
		cellY := sy + float32(int(s)/2)*rowH
		drawStatIcon(s, cellX+9, cellY+13, 9, woodAccentIconBright)
		drawTextWithShadow(font, core.StatLabel(s), cellX+24, cellY, FontBody, textMuted)
		base, eff := core.StatValue(m.Stats, s), core.StatValue(effStats, s)
		statCol := textPrimary
		switch {
		case eff > base:
			statCol = statBuffed
		case eff < base:
			statCol = statDebuffed
		}
		drawTextRightAligned(font, smallIntLabel(eff), cellX+statColW-statValueInsetX, cellY, FontBody, statCol)
	}
	ay := sy + float32((core.StatCount+1)/2)*rowH + 6
	drawTextWithShadow(font, "ARM", rightX, ay, FontSmall, textMuted)
	drawTextRightAligned(font, smallIntLabel(m.Armor), rightX+statColW-statValueInsetX, ay, FontSmall, textPrimary)
	drawTextWithShadow(font, "XP", rightX+colPitch, ay, FontSmall, textMuted)
	drawTextRightAligned(font, formatRatioSpaced(m.XP, core.XPForLevel(m.Level)), rightX+rightW, ay, FontSmall, textPrimary)

	// Allocate CTA — cursored member with something to spend.
	if highlight && (m.PendingLevelUps > 0 || m.SkillPoints > 0) {
		// formationCTAInset, not footerBaselineY: this CTA sits inside a dense
		// formation quadrant and rides tighter to the bottom than a modal footer hint.
		hintY := quad.Y + quad.Height - formationCTAInset
		if m.PendingLevelUps > 0 {
			drawHintSegs(font, []HintSeg{Hint("allocate "+strconv.Itoa(m.PendingLevelUps)+" stat pt"+plural(m.PendingLevelUps), GlyphA)}, rightX, hintY, FontSmall, inkAccent, 1)
		} else if m.SkillPoints > 0 {
			drawTextWithShadow(font, strconv.Itoa(m.SkillPoints)+" skill pt"+plural(m.SkillPoints)+" (Skills tab)", rightX, hintY, FontSmall, inkAccent)
		}
	}
}

// plural returns the "s" suffix when n != 1.
func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// equipSlotRowHeight is the per-slot row height in a member's Equipment-tab card (five slots stacked).
const equipSlotRowHeight = float32(66)

// slotIconForType returns the icon-draw function for an EquipSlotIndex (per-slot row variant).
func slotIconForType(slot core.EquipSlotIndex) func(cx, cy, r float32, col rl.Color) {
	return slotIconForKind(core.SlotIndexType(slot))
}

// slotIconForKind returns the icon-draw function for an EquipmentSlotType (picker-row / slot-type variant).
func slotIconForKind(t core.EquipmentSlotType) func(cx, cy, r float32, col rl.Color) {
	switch t {
	case core.SlotHand:
		return drawSlotIconSword
	case core.SlotArmor:
		return drawSlotIconShield
	case core.SlotAccessory:
		return drawSlotIconRing
	default:
		// Loud-fail on an unmapped slot type (dispatch-coverage convention) rather than silently rendering a ring.
		panic(fmt.Sprintf("render: EquipmentSlotType %d has no slotIconForKind entry", int(t)))
	}
}

// equipSlotHit is one clickable equip-slot row: its rect plus the (member, slot) it
// routes to. One struct so the three can't desync on rebuild.
type equipSlotHit struct {
	Rect   rl.Rectangle
	Member int
	Idx    core.EquipSlotIndex
}

// equipPanelLayout caches the Equipment tab's hit rects so the input layer can route a click without
// re-running layout. Slots is flattened [member][slot]. PickerRects parallels core.EquipPickerRows;
// PickerBounds is the card (click-outside dismiss), gated by PickerValid.
type equipPanelLayout struct {
	Slots        []equipSlotHit
	PickerRects  []rl.Rectangle
	PickerBounds rl.Rectangle
	PickerValid  bool
}

// lastEquipLayout is the most recently drawn Equipment-tab layout, written AFTER drawing so the hit rects
// match what was painted (single-threaded renderer + input, no sync needed).
var lastEquipLayout equipPanelLayout

// ResetEquipPanelLayout zeroes the cached hit rects (on overlay close / tab switch) so a stale click can't route.
func ResetEquipPanelLayout() { lastEquipLayout = equipPanelLayout{} }

// resetEquipLayoutKeepBufs clears the per-frame cache but RETAINS the backing arrays, so the every-frame
// rebuild re-slices the same memory. ResetEquipPanelLayout still fully zeroes, releasing buffers between visits.
func resetEquipLayoutKeepBufs() {
	lastEquipLayout = equipPanelLayout{
		Slots:       lastEquipLayout.Slots[:0],
		PickerRects: lastEquipLayout.PickerRects[:0],
	}
}

// Per-frame scratch buffers for the picker draw paths; reused across frames, valid only within the frame.
var (
	equipPickerRowsDrawBuf []core.EquipPickerRow
	useTargetLivingDrawBuf []int
	healPickerHealsDrawBuf []core.SkillID
)

// EquipPanelSlotHit returns (member, slot, true) if pt is inside a slot rect, else (-1, 0, false).
func EquipPanelSlotHit(pt rl.Vector2) (int, core.EquipSlotIndex, bool) {
	for _, s := range lastEquipLayout.Slots {
		if rl.CheckCollisionPointRec(pt, s.Rect) {
			return s.Member, s.Idx, true
		}
	}
	return -1, 0, false
}

// EquipPanelPickerRowHit returns (rowIndex, true) if pt is inside a picker row rect (index aligns with core.EquipPickerRows), else (-1, false).
func EquipPanelPickerRowHit(pt rl.Vector2) (int, bool) {
	for i, r := range lastEquipLayout.PickerRects {
		if rl.CheckCollisionPointRec(pt, r) {
			return i, true
		}
	}
	return -1, false
}

// EquipPanelClickOutsidePicker reports whether pt falls outside the open picker card (dismiss signal); false if none drawn this frame.
func EquipPanelClickOutsidePicker(pt rl.Vector2) bool {
	return lastEquipLayout.PickerValid && !rl.CheckCollisionPointRec(pt, lastEquipLayout.PickerBounds)
}

func drawPanelsEquipment(g *core.GameState, assets Resources, body rl.Rectangle) {
	font := assets.Font()
	resetEquipLayoutKeepBufs() // reset every frame, retaining buffers
	if len(g.Party) == 0 {
		return
	}

	// One card per member, each listing its five equip slots as rows. Gear is chosen in drawEquipPicker.
	cols := memberColumnLayout(body, len(g.Party))
	slotRowH := equipSlotRowHeight

	for i, m := range g.Party {
		memberHL := i == g.PanelsRowCursor
		contentY := drawPartyMemberCardHeader(font, m, cols[i], memberHL)
		innerX, innerW := memberCardInner(cols[i])

		for s := core.EquipSlotIndex(0); s < core.EquipSlotCount; s++ {
			rowY := contentY + float32(int(s))*slotRowH
			slotRect := rl.NewRectangle(float32(innerX), rowY, float32(innerW), slotRowH-8)
			lastEquipLayout.Slots = append(lastEquipLayout.Slots, equipSlotHit{Rect: slotRect, Member: i, Idx: s})

			// Focused slot (cursored member + slot, picker closed): shared focusable-row treatment.
			focused := memberHL && int(s) == g.EquipSlotCursor && !g.EquipPickerOpen
			drawFocusableRow(slotRect, focused)

			equippedKind := m.Equipped[s]
			filled := equippedKind != core.ItemNone
			iconCol := woodAccentIconSoft
			if filled {
				iconCol = giltBright
			}
			slotIconForType(s)(float32(innerX)+16, rowY+26, 11, iconCol)

			labelX := float32(innerX) + 40
			// Focused slot's label brightens so the cursor reads against the muted slot names.
			labelCol := textMuted
			if focused {
				labelCol = textPrimary
			}
			drawTextWithShadow(font, core.SlotIndexLabel(s), labelX, rowY+6, FontSmall, labelCol)
			value := "(empty)"
			valCol := textDim
			if filled {
				value = core.ItemInfo(equippedKind).Name
				valCol = textPrimary
			}
			drawTextWithShadow(font, value, labelX, rowY+26, FontBody, valCol)
			// Only the FOCUSED slot expands to show its bonus, so the narrow columns aren't cluttered across all 20 rows.
			if focused && filled {
				if bonus := equipBonusSummary(core.ItemInfo(equippedKind)); bonus != "" {
					drawTextWithShadow(font, bonus, labelX, rowY+42, FontSmall, inkAccent)
				}
			}
		}
	}
	// Footer is painted once by DrawPanelsOverlay from panelTabs[tab].footer — no per-tab inline footer here.
}

// Shared picker sub-modal geometry. use-target + heal pickers are visually identical and share these;
// the equip picker keeps its OWN taller header (extra "Equipped: …" sub-title line) + rows as equipPicker* below.
const (
	pickerRowH    = float32(44)
	pickerHeaderH = float32(56)
	pickerFooterH = float32(32)
	// Per-picker card widths + the use-target picker's taller row (name + status pill +
	// HP/MP bars). Heal picker rows use the stock pickerRowH.
	useTargetPickerCardW = float32(430)
	useTargetPickerRowH  = float32(58)
	healPickerCardW      = float32(360)
)

// equipPicker* are the equip picker's own geometry (taller header for the "Equipped: …" sub-title); see above.
const (
	equipPickerCardW   = float32(440)
	equipPickerRowH    = float32(46)
	equipPickerHeaderH = float32(70)
	equipPickerFooterH = float32(34)
	// equipPickerSubtitleDY is the "Equipped: …" sub-title baseline, kept next to the header tokens so it can't drift.
	equipPickerSubtitleDY = float32(52)
)

// uiCellInsetX is the standard body inset for a Stats/detail cell, so the two below
// share one rhythm instead of two bare 14s that can silently drift apart. Give either
// its own literal if it must diverge.
const uiCellInsetX = float32(14)

// statValueInsetX is the right-edge inset for a right-aligned value in a Stats-tab cell, so the number column shares one gutter.
const statValueInsetX = uiCellInsetX

// detailCardInsetX is the content inset (top-left) for a glass detail card's body text.
const detailCardInsetX = uiCellInsetX

// pickerCardLeftInset is the shared left gutter for a picker's title + footer hint.
// 26, not hudContentInsetX=22: the picker's engraved title sits a touch further off the
// frame than body content; kept distinct so retuning the body inset doesn't shift titles.
const pickerCardLeftInset = float32(26)

// pickerTitleTopInset is the shared top inset for a picker's FontHeading title.
const pickerTitleTopInset = float32(20)

// drawPickerCard paints the shared picker chrome (veiled wood-and-glass card + left-aligned title) and returns
// the card rect, consolidating the drawVeiledCard + title preamble the three pickers and the skill-tree modal repeated.
func drawPickerCard(font rl.Font, cardW, cardH float32, title string) rl.Rectangle {
	return drawPickerCardEx(font, cardW, cardH, title, pickerCardLeftInset, pickerTitleTopInset, false)
}

// drawPickerCardEx is drawPickerCard's parameterized core: titleX/titleY are the engraved
// title's inset from the card top-left (the skill-tree modal shifts the title right of a
// class crest); opaqueBackdrop paints a solid fill behind the card BEFORE the veiled glass
// (the skill-tree modal needs the body to composite over solid dark, not the lit scene).
func drawPickerCardEx(font rl.Font, cardW, cardH float32, title string, titleX, titleY float32, opaqueBackdrop bool) rl.Rectangle {
	if opaqueBackdrop {
		// drawVeiledCard centers identically, so this rect aligns with the card it precedes.
		cw, ch := int32(cardW), int32(cardH)
		_, screenH := screenSize()
		bx := centerX(cw)
		by := screenH/2 - ch/2
		backdrop := rl.NewRectangle(float32(bx), float32(by), float32(cw), float32(ch))
		rl.DrawRectangleRounded(backdrop, fixedRoundnessFor(cw, ch, cornerRadius), 8, surfaceCardBackdrop)
	}
	card := drawVeiledCard(int32(cardW), int32(cardH), borderActive, woodAccent, woodAccent)
	drawEngravedText(font, title, card.X+titleX, card.Y+titleY, FontHeading, textPrimary)
	return card
}

// Picker list-row geometry, shared by the three picker sub-modals: each row insets pickerRowInsetX from both
// card edges and leaves pickerRowGap below itself in its rowH slot.
const (
	pickerRowInsetX = float32(16)
	pickerRowGap    = float32(8)
)

// pickerRowRect returns row i's rect in a picker list starting at listY (single geometry source for all three).
func pickerRowRect(card rl.Rectangle, listY float32, i int, rowH float32) rl.Rectangle {
	return rl.NewRectangle(card.X+pickerRowInsetX, listY+float32(i)*rowH, card.Width-2*pickerRowInsetX, rowH-pickerRowGap)
}

// drawPickerList lays out a picker's body shared by the three picker sub-modals: the row
// loop (rect + focus highlight) below the header band, then the left-aligned footer hint
// bar. drawRow renders row i's content into its rect (and may capture the rect for clicks).
func drawPickerList(font rl.Font, card rl.Rectangle, headerH, rowH float32, count, focused int, hints []HintSeg, drawRow func(i int, rect rl.Rectangle)) {
	listY := card.Y + headerH
	for i := 0; i < count; i++ {
		rect := pickerRowRect(card, listY, i, rowH)
		drawFocusableRow(rect, i == focused)
		drawRow(i, rect)
	}
	drawModalFooterGlyphsLeft(font, card, card.X+pickerCardLeftInset, hints)
}

// drawEquipPicker paints the slot's item-picker sub-modal on top of the overlay: items eligible for the focused
// slot plus an "Unequip" row when filled, cursored row gilded. Row rects + card bounds cached on lastEquipLayout for clicks.
func drawEquipPicker(g *core.GameState, assets Resources) {
	font := assets.Font()
	member := g.PanelsRowCursor
	if member < 0 || member >= len(g.Party) {
		return
	}
	if g.EquipSlotCursor < 0 || g.EquipSlotCursor >= int(core.EquipSlotCount) {
		return // cursor indexes the fixed-size Equipped array — guard it like member
	}
	slot := core.EquipSlotIndex(g.EquipSlotCursor)
	rows := core.EquipPickerRowsInto(equipPickerRowsDrawBuf, g, member, slot)
	equipPickerRowsDrawBuf = rows

	const rowH = equipPickerRowH
	const headerH = equipPickerHeaderH
	visibleRows := len(rows)
	if visibleRows < 1 {
		visibleRows = 1 // reserve a line for the "no eligible items" note
	}
	_, sh := screenSizeF()
	cardW := equipPickerCardW
	cardH := headerH + float32(visibleRows)*rowH + equipPickerFooterH
	if maxH := sh * 0.78; cardH > maxH {
		cardH = maxH
	}

	// Centered card + title via the shared picker chrome, then lay the picker out in the returned rect.
	title := core.SlotIndexLabel(slot) + " — " + g.Party[member].Name
	card := drawPickerCard(font, cardW, cardH, title)

	curKind := g.Party[member].Equipped[slot]
	curText := "Equipped: —"
	if curKind != core.ItemNone {
		curText = "Equipped: " + core.ItemInfo(curKind).Name
	}
	drawTextWithShadow(font, curText, card.X+pickerCardLeftInset, card.Y+equipPickerSubtitleDY, FontSmall, textMuted)

	lastEquipLayout.PickerRects = lastEquipLayout.PickerRects[:0]
	lastEquipLayout.PickerBounds = card
	lastEquipLayout.PickerValid = true

	if len(rows) == 0 {
		drawTextWithShadow(font, "No eligible items in inventory.", card.X+pickerCardLeftInset, card.Y+headerH+8, FontBody, textDim)
	}
	drawPickerList(font, card, headerH, rowH, len(rows), g.EquipPickerCursor, equipPickerHints, func(i int, rect rl.Rectangle) {
		lastEquipLayout.PickerRects = append(lastEquipLayout.PickerRects, rect)
		row := rows[i]
		if row.Unequip {
			drawTextWithShadow(font, "Unequip", rect.X+14, rect.Y+rect.Height/2-10, FontBody, inkAccent)
			return
		}
		def := core.ItemInfo(row.Kind)
		slotIconForKind(def.Slot)(rect.X+18, rect.Y+rect.Height/2, 11, giltBright)
		name := def.Name
		if row.Count > 1 {
			name = stackLabel(name, row.Count)
		}
		drawTextWithShadow(font, name, rect.X+38, rect.Y+4, FontSmall, textPrimary)
		if bonus := equipBonusSummary(def); bonus != "" {
			drawTextWithShadow(font, bonus, rect.X+38, rect.Y+rect.Height-20, FontSmall, inkAccent)
		}
	})
}

// drawUseTargetPicker paints the shared ally-target sub-modal for out-of-battle "use" actions (Items/Skills tab):
// living members with HP, focused row is the recipient. Controller-driven (UseTargetCursor); no mouse hit rects.
func drawUseTargetPicker(g *core.GameState, assets Resources) {
	font := assets.Font()
	living := core.LivingPartyIndicesInto(useTargetLivingDrawBuf, g.Party)
	useTargetLivingDrawBuf = living

	title := "Use"
	switch {
	case g.UsePendingItem != core.ItemNone:
		title = "Use " + core.ItemInfo(g.UsePendingItem).Name
	case g.UsePendingSkill != core.SkillNone:
		title = "Cast " + core.SkillName(g.UsePendingSkill)
	}

	// Taller rows than the stock picker: name + status pill + live HP/MP bars (same readout the party ribbon shows).
	const rowH = useTargetPickerRowH
	const headerH = pickerHeaderH
	visibleRows := len(living)
	if visibleRows < 1 {
		visibleRows = 1
	}
	cardW := useTargetPickerCardW
	cardH := headerH + float32(visibleRows)*rowH + pickerFooterH
	card := drawPickerCard(font, cardW, cardH, title)

	if len(living) == 0 {
		drawTextWithShadow(font, "No one can be healed.", card.X+pickerCardLeftInset, card.Y+headerH, FontBody, textDim)
	}
	drawPickerList(font, card, headerH, rowH, len(living), g.UseTargetCursor, useTargetPickerHints, func(i int, rect rl.Rectangle) {
		m := &g.Party[living[i]]
		classCol := classAccent(m.Class)
		// Left column: class sigil + name.
		drawClassGlyph(rect.X+24, rect.Y+22, 10, m.Class, classCol)
		nameX := rect.X + 46
		drawTextWithShadow(font, m.Name, nameX, rect.Y+8, FontBody, textPrimary)
		// Status pill beside the name (poison etc. survive out of battle), same glyph + accent as the party cards.
		if kind, _ := core.PartyStatus(m); kind != core.PartyStatusNone {
			col, flicker := partyStatusVisual(kind)
			if flicker {
				col = fadeColor(col, pulseFlicker())
			}
			drawPartyStatusIcon(nameX+8, rect.Y+38, 7, kind, col)
		}
		// Right column: compact HP over MP bars (drawBarLive keyed by the stable member name).
		barX := rect.X + rect.Width*memberCardBarSplit
		barW := rect.Width*memberCardBarFracW - 16
		drawBarLive(font, "use:hp:"+m.Name, barX, rect.Y+8, barW, barHeightMini, "HP", m.HP, m.MaxHP, hpFillColor(m.HP, m.MaxHP), false)
		drawBar(font, barX, rect.Y+30, barW, barHeightMini, "MP", m.MP, m.MaxMP, barMP, false)
	})
}

// drawHealPicker paints the out-of-battle support-skill chooser: the caster's heals/cures with MP cost, cursored row gilded.
// Raised only when a member has more than one such skill; a single skill casts directly. Controller-driven (HealPickCursor).
func drawHealPicker(g *core.GameState, assets Resources) {
	font := assets.Font()
	caster := g.HealPickCaster
	if caster < 0 || caster >= len(g.Party) {
		return
	}
	skills := core.OutOfBattleSupportSkillsInto(healPickerHealsDrawBuf, &g.Party[caster])
	healPickerHealsDrawBuf = skills
	if len(skills) == 0 {
		return
	}

	const rowH = pickerRowH
	const headerH = pickerHeaderH
	cardW := healPickerCardW
	cardH := headerH + float32(len(skills))*rowH + pickerFooterH
	card := drawPickerCard(font, cardW, cardH, "Use Skill — "+g.Party[caster].Name)

	drawPickerList(font, card, headerH, rowH, len(skills), g.HealPickCursor, healPickerHints, func(i int, rect rl.Rectangle) {
		s := skills[i]
		drawTextWithShadow(font, core.SkillName(s), rect.X+14, rect.Y+rect.Height/2-10, FontBody, textPrimary)
		costText := skillCostMPLabel(core.SkillCost(s))
		drawTextRightAligned(font, costText, rect.X+rect.Width-12, rect.Y+rect.Height/2-8, FontSmall, inkAccent)
	})
}

// statBonusPart formats one signed stat-bonus chip ("STR +2", "Armor -1") with the
// correct sign for negatives — one source so every arm (Armor/MDef/stat loop) agrees.
func statBonusPart(label string, value int) string {
	sign := "+"
	if value < 0 {
		sign = "" // strconv already prints the leading '-'
	}
	return label + " " + sign + strconv.Itoa(value)
}

// equipBonusSummary returns the single-line "STR +2" / "Armor +1" bonus copy under an item's tile.
// equipBonusSummaryCache memoizes it by kind (built from the immutable ItemDefinition); "" is cached too.
var equipBonusSummaryCache = map[core.ItemKind]string{}

func equipBonusSummary(def core.ItemDefinition) string {
	if s, ok := equipBonusSummaryCache[def.Kind]; ok {
		return s
	}
	parts := []string{}
	// Lead with the weapon's accuracy stat + range, derived from the registry, never re-authored in prose.
	if def.Weapon != core.WeaponNone {
		tag := core.StatLabel(core.WeaponAccuracyStat(def.Weapon)) + " weapon"
		if core.WeaponIsRanged(def.Weapon) {
			tag = core.StatLabel(core.WeaponAccuracyStat(def.Weapon)) + " ranged"
		}
		parts = append(parts, tag)
	}
	if def.ArmorBonus != 0 {
		parts = append(parts, statBonusPart("Armor", def.ArmorBonus))
	}
	if def.MDefBonus != 0 {
		parts = append(parts, statBonusPart("MDef", def.MDefBonus))
	}
	for s := core.Stat(0); s < core.StatCount; s++ {
		v := def.StatBonus[s]
		if v == 0 {
			continue
		}
		parts = append(parts, statBonusPart(core.StatLabel(s), v))
	}
	out := ""
	if len(parts) > 0 {
		out = parts[0]
		for i := 1; i < len(parts); i++ {
			out += "  " + parts[i]
		}
	}
	equipBonusSummaryCache[def.Kind] = out
	return out
}

// drawSlotIconSword paints a small dagger sigil for the Weapon slot (drawDaggerGlyph, no pommel hi, thicker guard). r is the half-height.
func drawSlotIconSword(cx, cy, r float32, col rl.Color) {
	drawDaggerGlyph(cx, cy, r, col, daggerGlyphStyle{
		minBladeHalfW: 1.5,
		guardH:        2,
		fullerXOff:    0,
		pommelHi:      false,
	})
}

// drawSlotIconShield paints a small heater-shield sigil for the Armor slot (shoulders rect + point triangle + boss). r is the half-height.
func drawSlotIconShield(cx, cy, r float32, col rl.Color) {
	topW := r * 1.4
	topH := r * 0.7
	// Shoulders.
	rl.DrawRectangle(int32(cx-topW/2), int32(cy-r), int32(topW), int32(topH), col)
	// Tapered point: triangle from the shoulders' bottom corners to a single tip.
	tip := rl.NewVector2(cx, cy+r)
	left := rl.NewVector2(cx-topW/2, cy-r+topH)
	right := rl.NewVector2(cx+topW/2, cy-r+topH)
	drawTriangleCCW(tip, right, left, col)
	// Centre boss — small inner disc + bright pip.
	rl.DrawCircleV(rl.NewVector2(cx, cy-r*0.05), r*0.32, fadeColor(col, 0.55))
	rl.DrawCircleV(rl.NewVector2(cx, cy-r*0.05), r*0.16, giltBright)
}

// drawSlotIconRing paints a small ring sigil with a gem cap for the Accessory slot. r is the ring's outer radius.
func drawSlotIconRing(cx, cy, r float32, col rl.Color) {
	// Annulus via outer disc + inner punch-out; the inner disc approximates the slot bezel so the ring reads hollow.
	rl.DrawCircleV(rl.NewVector2(cx, cy+1), r, col)
	rl.DrawCircleV(rl.NewVector2(cx, cy+1), r*0.55, fadeColor(glassDeep, 0.8))
	// Gem dot at top of the ring band.
	rl.DrawCircleV(rl.NewVector2(cx, cy+1-r*0.85), r*0.32, giltBright)
}

// drawPanelsItems renders the Items tab as a ledger: scrollable stack list on the left, detail panel on the right.
// panelsItemStacksBuf is the reused scratch slice for the live-stack list, refilled each frame.
var panelsItemStacksBuf []core.ItemStack

// Items-tab list metrics (mirror the Journal tab's naming). itemsRowH is the per-row stride; itemsRowInsetX the text inset.
const (
	itemsRowH      = float32(46)
	itemsRowInsetX = float32(14)
)

func drawPanelsItems(g *core.GameState, assets Resources, body rl.Rectangle) {
	font := assets.Font()
	panelsItemStacksBuf = core.LiveStacksInto(g.Inventory, panelsItemStacksBuf)
	stacks := panelsItemStacksBuf
	if len(stacks) == 0 {
		drawEmptyLedgerNote(font, body, "Your bags are empty.",
			"Loot from steals and chests will appear here.")
		return
	}

	// Two-pane layout: list on the left, detail card on the right.
	const gap = float32(16)
	listW := body.Width*0.62 - gap/2
	detailW := body.Width - listW - gap
	listRect := rl.NewRectangle(body.X, body.Y, listW, body.Height)
	detailRect := rl.NewRectangle(body.X+listW+gap, body.Y, detailW, body.Height)

	// List rows.
	rowH := itemsRowH
	rowPad := itemsRowInsetX
	cursor := g.PanelsRowCursor
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= len(stacks) {
		cursor = len(stacks) - 1
	}
	for i, stack := range stacks {
		y := listRect.Y + float32(i)*rowH
		if y+rowH > listRect.Y+listRect.Height {
			break
		}
		info := core.ItemInfo(stack.Kind)
		highlight := i == cursor
		if highlight {
			drawFocusableRow(rl.NewRectangle(listRect.X, y, listRect.Width, rowH-4), true) // shared focused-row look
		}
		nameCol := textMuted
		if highlight {
			nameCol = textPrimary
		}
		drawTextWithShadow(font, info.Name, listRect.X+rowPad, y+12, FontBody, nameCol)
		// Count chip on the right edge.
		countText := panelsItemCountLabel(stack.Count)
		drawTextRightAligned(font, countText, listRect.X+listRect.Width-rowPad, y+12, FontBody, inkAccent)
	}

	// Detail card: name, effect summary, count owned, description stub.
	drawGlassPaneRect(detailRect, glassMid)
	if cursor < len(stacks) {
		stack := stacks[cursor]
		info := core.ItemInfo(stack.Kind)
		dy := detailRect.Y + detailCardInsetX
		dx := detailRect.X + detailCardInsetX
		drawEngravedText(font, info.Name, dx, dy, FontHeading, textPrimary)
		dy += 38
		drawTextWithShadow(font, panelsItemEffectLabel(info), dx, dy, FontBody, inkAccent)
		dy += cardSubLineStep
		owned := "Owned: " + strconv.Itoa(stack.Count)
		drawTextWithShadow(font, owned, dx, dy, FontBody, textMuted)
		dy += 36
		// Description placeholder — the item registry carries none today. Lead with
		// the Use glyph so the affordance reads controller-first (gamepad contract),
		// never a bare "Press Use".
		gx := dx + drawInputGlyph(font, GlyphX, dx, dy, FontSmall, 1) + glyphLabelGap
		drawTextWithShadow(font, "applies this consumable to an ally — here or in battle.", gx, dy, FontSmall, textDim)
	}
}

// skillCostMPLabel returns "<cost> MP" from a LUT (cap 32 absorbs the bounded skill-cost range).
func skillCostMPLabel(cost int) string {
	if cost >= 0 && cost < len(skillCostMPLabelCache) {
		return skillCostMPLabelCache[cost]
	}
	return strconv.Itoa(cost) + " MP"
}

var skillCostMPLabelCache = func() [32]string {
	var out [32]string
	for i := range out {
		out[i] = strconv.Itoa(i) + " MP"
	}
	return out
}()

// goldLabelFull / goldLabelShort: the two gold-readout formats, one source each. The Tome/shop show "Gold: N";
// the HUD chip shows "N G". No LUT (unbounded range); the per-frame draws route already-cached values.
func goldLabelFull(n int) string  { return fmt.Sprintf("Gold: %d", n) }
func goldLabelShort(n int) string { return fmt.Sprintf("%d G", n) }

// goldGainLabel formats a reward gain ("Gold  +N") — the victory-spoils sibling of the two above.
func goldGainLabel(n int) string { return fmt.Sprintf("Gold  +%d", n) }

// stackLabel renders an item-stack label with the shared "  x<count>" convention
// (chest, shop, equip picker). One home so the two-space-x format can't drift;
// TestBuildShopRowsMatchesCatalogOrder pins the literal format against this.
func stackLabel(name string, count int) string { return name + "  x" + strconv.Itoa(count) }

// skillPointsLabel returns "<n> SP" from a LUT — the SP sibling of skillCostMPLabel, shared across the Skills tab + tree modal.
func skillPointsLabel(n int) string {
	if n >= 0 && n < len(skillPointsLabelCache) {
		return skillPointsLabelCache[n]
	}
	return strconv.Itoa(n) + " SP"
}

var skillPointsLabelCache = func() [32]string {
	var out [32]string
	for i := range out {
		out[i] = strconv.Itoa(i) + " SP"
	}
	return out
}()

// panelsItemHealLabelCache pre-formats "+N HP" so the Items tab doesn't fmt.Sprintf per visible stack per frame.
var panelsItemHealLabelCache = func() [64]string {
	var out [64]string
	for i := range out {
		out[i] = "+" + strconv.Itoa(i) + " HP"
	}
	return out
}()

func panelsItemHealLabel(amount int) string {
	if amount >= 0 && amount < len(panelsItemHealLabelCache) {
		return panelsItemHealLabelCache[amount]
	}
	return "+" + strconv.Itoa(amount) + " HP"
}

// smallIntLabel returns a small non-negative int's decimal string from a LUT (Stats-tab values/armor, per frame).
var smallIntLabelCache = func() [256]string {
	var out [256]string
	for i := range out {
		out[i] = strconv.Itoa(i)
	}
	return out
}()

func smallIntLabel(n int) string {
	if n >= 0 && n < len(smallIntLabelCache) {
		return smallIntLabelCache[n]
	}
	return strconv.Itoa(n)
}

// panelsItemCountLabel pre-formats the "xN" stack-count chips the Items tab paints per row per frame.
var panelsItemCountLabelCache = func() [256]string {
	var out [256]string
	for i := range out {
		out[i] = "x" + strconv.Itoa(i)
	}
	return out
}()

func panelsItemCountLabel(n int) string {
	if n >= 0 && n < len(panelsItemCountLabelCache) {
		return panelsItemCountLabelCache[n]
	}
	return "x" + strconv.Itoa(n)
}

// formatRatioSpaced memoizes the canonical spaced "cur / total" readout shared by the
// Skills tree, XP line, and rank state. (Narrow gauges use the space-free formatBarValue.)
var ratioSpacedCache = map[[2]int]string{}

func formatRatioSpaced(cur, total int) string {
	k := [2]int{cur, total}
	if s, ok := ratioSpacedCache[k]; ok {
		return s
	}
	s := strconv.Itoa(cur) + " / " + strconv.Itoa(total)
	ratioSpacedCache[k] = s
	return s
}

// panelsItemEffectLabel is the Items-tab detail line for a consumable: "+N HP", "+N MP", a humanized hunger clause for food, or a no-effect note.
func panelsItemEffectLabel(info core.ItemDefinition) string {
	effect := ""
	if info.HealAmount > 0 {
		effect = panelsItemHealLabel(info.HealAmount)
	}
	if info.MPAmount > 0 {
		if effect != "" {
			effect += "   "
		}
		effect += "+" + strconv.Itoa(info.MPAmount) + " MP"
	}
	if info.SatietyGain > 0 {
		if effect != "" {
			effect += "   "
		}
		effect += "Heals " + core.SatietyHungerPhrase(info.SatietyGain) + "."
	}
	if effect == "" {
		return "No restorative effect"
	}
	return effect
}

// drawPanelsSkills renders the Skills tab: per member, a SUMMARY of the three skill trees (SkillPoints balance,
// then one row per tree with name + invested/total + theme). Confirm opens DrawSkillTreeModal where points are spent.
func drawPanelsSkills(g *core.GameState, assets Resources, body rl.Rectangle) {
	font := assets.Font()
	if len(g.Party) == 0 {
		return
	}
	cols := memberColumnLayout(body, len(g.Party))
	for i, m := range g.Party {
		highlight := i == g.PanelsRowCursor
		contentY := drawPartyMemberCardHeader(font, m, cols[i], highlight)
		innerX, innerW := memberCardInner(cols[i])

		// Skill-point balance — bright when there's something to spend, muted at zero.
		drawTextWithShadow(font, "SKILL POINTS", innerX, contentY, FontSmall, textMuted)
		drawSkillPointBalance(font, m.SkillPoints, innerX+innerW, contentY, FontSmall)
		contentY += cardSubLineStep

		// One summary panel per tree: name + invested/total + theme.
		trees := core.SkillTreesFor(m.Class)
		rowH := float32(70)
		for ti, tr := range trees {
			rowY := contentY + float32(ti)*rowH
			if rowY+rowH-10 > cols[i].Y+cols[i].Height {
				break
			}
			rect := rl.NewRectangle(innerX, rowY, innerW, rowH-10)
			drawGlassPaneRect(rect, glassMid)

			md := cardDetailRowMetrics
			drawTextWithShadow(font, tr.Name, rect.X+md.insetX, rect.Y+md.titleY, FontBody, textPrimary)
			invested := core.TreeInvestedRanks(&m, tr)
			ratio := formatRatioSpaced(invested, core.TreeMaxRanks(tr))
			ratioCol := accentIfPositive(invested, giltBright)
			drawTextRightAligned(font, ratio, rect.X+rect.Width-md.insetX, rect.Y+md.valueY, FontSmall, ratioCol)
			drawTextWithShadow(font, tr.Theme, rect.X+md.insetX, rect.Y+md.subY, FontSmall, textDim)
		}

		// Cursored member: Confirm opens the trees.
		if highlight {
			// skillTreeCTAInset, not footerBaselineY: this hint clears the tree-ratio
			// rows stacked above it in the Skills-tab card, so it rides higher.
			hintY := cols[i].Y + cols[i].Height - skillTreeCTAInset
			DrawHintBar(font, []HintSeg{Hint("Open skill trees", GlyphA)}, cols[i].X+cols[i].Width/2, hintY, FontSmall)
		}
	}
}

// In-card footer insets: a hint/readout that rides INSIDE a dense panel sits this
// many px up from the card bottom — intentionally tighter than
// footerBaselineY(…, FontSmall) (= uiFooterMargin+FontSmall = 35px), which is for
// modal footers. Each tracks a different content density, so they're distinct
// named values rather than one shared token.
const (
	mapFooterBottomInset = float32(20) // zoom readout, tucked low against the map edge
	formationCTAInset    = float32(28) // allocate CTA inside a dense formation quadrant
	skillTreeCTAInset    = float32(46) // skills hint clearing the tree-ratio rows
)

// drawPanelsMap renders the zoomable Map tab. Cells-on-screen comes from g.PanelsMapZoom; explored tiles
// paint full-color, unexplored at a heavy fade (silhouette without spoiling discovery).
func drawPanelsMap(g *core.GameState, assets Resources, body rl.Rectangle) {
	font := assets.Font()
	m := &g.Area
	if m.Width <= 0 || m.Height <= 0 {
		return
	}
	// A degenerate body would make cellPx 0, and int(body.Width/0) is int(NaN) — a garbage huge loop bound.
	if body.Width <= 0 || body.Height <= 0 {
		return
	}
	zoom := g.PanelsMapZoom
	if zoom <= 0 {
		zoom = core.PanelMapZoomDefault
	}
	// Cell size: fit `zoom` cells across the body, same size vertically so the map stays square.
	cellPx := body.Width / float32(zoom)
	if cellPx*float32(zoom) > body.Height {
		cellPx = body.Height / float32(zoom)
	}
	// Floor at 1px/cell so a future loosened zoom clamp can't drive a runaway DrawRectangle loop.
	if cellPx < 1 {
		cellPx = 1
	}
	cellsX := int(body.Width / cellPx)
	cellsY := int(body.Height / cellPx)
	// Pan offset (PanelsMapPanX/Z, d-pad) shifts the view center off the player; zero = centered.
	startX := g.Player.TileX - cellsX/2 + g.PanelsMapPanX
	startZ := g.Player.TileZ - cellsY/2 + g.PanelsMapPanZ

	mapX := body.X + (body.Width-float32(cellsX)*cellPx)/2
	mapY := body.Y + (body.Height-float32(cellsY)*cellPx)/2

	// One MaterialIsIndoor lookup for the whole grid (per-area constant), passed into each cell.
	indoor := core.MaterialIsIndoor(m.Materials)
	// One mapSliceCell pass over the window + a one-cell border, feeding the seen-wall outline below.
	// Shared classifier + outline with the corner minimap so the two can't drift on fog/slice/suppression/border.
	gw := cellsX + 2
	// Reused scratch grids; the loop writes every window+border index each pass, so reuse needs no clearing.
	// Held separate from the corner minimap's buffers so a cross-fade frame drawing both can't clash.
	n := gw * (cellsY + 2)
	if cap(panelsMapSliceBuf) < n {
		panelsMapSliceBuf = make([]bool, n)
		panelsMapSeenBuf = make([]bool, n)
		panelsMapRampBuf = make([]int8, n)
		panelsMapColBuf = make([]rl.Color, n)
		panelsMapClassCache.valid = false // fresh grids — force a reclassify
	}
	slice := panelsMapSliceBuf[:n]
	seen := panelsMapSeenBuf[:n]
	ramp := panelsMapRampBuf[:n]
	colGrid := panelsMapColBuf[:n]
	// Reclassify only when the fingerprint changes (player moved / changed level / panned / re-zoomed /
	// area changed); otherwise last frame's grids still hold and the window+border mapSliceCell pass is skipped.
	key := panelsMapClassKey{
		path: m.Path, tileX: g.Player.TileX, tileZ: g.Player.TileZ, level: g.Player.Level,
		panX: g.PanelsMapPanX, panZ: g.PanelsMapPanZ, cellsX: cellsX, cellsY: cellsY, valid: true,
	}
	if key != panelsMapClassCache {
		for localZ := -1; localZ <= cellsY; localZ++ {
			for localX := -1; localX <= cellsX; localX++ {
				col, onSlice, seenWall, rampDir := mapSliceCell(m, g, indoor, startX+localX, startZ+localZ)
				i := borderIdx(gw, localX, localZ)
				slice[i], seen[i], ramp[i], colGrid[i] = onSlice, seenWall, rampDir, col
			}
		}
		panelsMapClassCache = key
	}
	// Pixel rects are cheap and depend on the body position (mapX/mapY), so they re-draw every frame from the
	// cached colors. Derive each cell's rect from consecutive truncated edges (this cell's left, the next cell's
	// left), not origin+size — with a fractional cellPx that left 1px seams. Edge-to-edge tiling abuts cleanly.
	for localZ := 0; localZ < cellsY; localZ++ {
		for localX := 0; localX < cellsX; localX++ {
			px := int32(mapX + float32(localX)*cellPx)
			py := int32(mapY + float32(localZ)*cellPx)
			pw := int32(mapX+float32(localX+1)*cellPx) - px
			ph := int32(mapY+float32(localZ+1)*cellPx) - py
			if pw < 1 {
				pw = 1
			}
			if ph < 1 {
				ph = 1
			}
			rl.DrawRectangle(px, py, pw, ph, colGrid[borderIdx(gw, localX, localZ)])
		}
	}

	// Faint tile grid (graph-paper ruling), low-alpha so tiles read as cells without fighting the terrain fills.
	gridW := float32(cellsX) * cellPx
	gridH := float32(cellsY) * cellPx
	gridCol := woodAccentGrid
	for gx := 0; gx <= cellsX; gx++ {
		px := int32(mapX + float32(gx)*cellPx)
		rl.DrawRectangle(px, int32(mapY), 1, int32(gridH), gridCol)
	}
	for gz := 0; gz <= cellsY; gz++ {
		py := int32(mapY + float32(gz)*cellPx)
		rl.DrawRectangle(int32(mapX), py, int32(gridW), 1, gridCol)
	}
	// Seen-wall border over the grid (same rule as the corner minimap): muted line where explored floor abuts a seen wall.
	drawMapLevelOutline(slice, seen, gw, cellsX, cellsY, mapX, mapY, cellPx, cellPx)
	// Up/down stair glyphs on ramp cells.
	drawMapStairIcons(ramp, gw, cellsX, cellsY, mapX, mapY, cellPx, cellPx)

	// Pack markers intentionally omitted: the map shows terrain, not who's on it; enemies stay a surprise.

	// Chest markers — gold square, visited tiles only.
	for _, ch := range g.Chests {
		if !visitedAt(g, ch.TileX, ch.TileZ) {
			continue
		}
		lx := ch.TileX - startX
		lz := ch.TileZ - startZ
		if lx < 0 || lz < 0 || lx >= cellsX || lz >= cellsY {
			continue
		}
		px := int32(mapX + float32(lx)*cellPx + cellPx*0.25)
		py := int32(mapY + float32(lz)*cellPx + cellPx*0.25)
		pw := int32(cellPx * 0.5)
		if pw < 2 {
			pw = 2
		}
		col := mapChestMarkerColor
		if ch.Looted {
			col = mapChestLootedColor
		}
		rl.DrawRectangle(px, py, pw, pw, col)
	}

	// Door markers — wooden rectangle, visited tiles only.
	for _, d := range g.Doors {
		if !visitedAt(g, d.TileX, d.TileZ) {
			continue
		}
		lx := d.TileX - startX
		lz := d.TileZ - startZ
		if lx < 0 || lz < 0 || lx >= cellsX || lz >= cellsY {
			continue
		}
		px := int32(mapX + float32(lx)*cellPx + cellPx*0.30)
		py := int32(mapY + float32(lz)*cellPx + cellPx*0.10)
		pw := int32(cellPx * 0.40)
		ph := int32(cellPx * 0.80)
		if pw < 2 {
			pw = 2
		}
		if ph < 2 {
			ph = 2
		}
		rl.DrawRectangle(px, py, pw, ph, mapDoorMarkerColor)
	}

	// Player arrow — window center when un-panned, off-center as it pans; drawn only while the tile is in-window.
	plx := g.Player.TileX - startX
	plz := g.Player.TileZ - startZ
	if plx >= 0 && plz >= 0 && plx < cellsX && plz < cellsY {
		pcx := mapX + (float32(plx)+0.5)*cellPx
		pcy := mapY + (float32(plz)+0.5)*cellPx
		drawPanelsMapArrow(pcx, pcy, cellPx, g.Player.Facing)
	}

	// Compass rose, inset into the upper-right; faint dark backing disc keeps it legible over terrain.
	crX := body.X + body.Width - 66
	crY := body.Y + 50
	rl.DrawCircleV(rl.NewVector2(crX, crY), 31, fadeColor(shadowHeavy, 0.34))
	drawCompassRose(crX, crY, 48, font)

	// Map footer — zoom indicator only (area name is in the top info strip; the pan/
	// zoom CONTROLS live in the shared bottom footer bar, not duplicated here).
	footerY := body.Y + body.Height - mapFooterBottomInset
	drawTextWithShadow(font, panelsMapFooterText(zoom), body.X, footerY, FontSmall, textDim)
}

// drawCompassRose paints an 8-point compass rose at (cx,cy) within diameter d: cardinal + diagonal points,
// gilt center, wood ring, "N" letter at the north tip.
func drawCompassRose(cx, cy float32, d float32, font rl.Font) {
	outerR := d / 2
	innerR := outerR * 0.30
	center := rl.NewVector2(cx, cy)
	// Medallion: dark seat, recessed glass face, two concentric gilt ring-lines for a brass bezel.
	rl.DrawCircleV(center, outerR+2, woodDark)
	rl.DrawCircleV(center, outerR, glassDeep)
	rl.DrawCircleLines(int32(cx), int32(cy), outerR, woodAccent)
	rl.DrawCircleLines(int32(cx), int32(cy), outerR*0.72, fadeColor(woodAccent, 0.5))

	// Each point splits along its axis into a LIT half + SHADOW half, so the star reads as faceted brass, not a flat kite.
	diagLit := fadeColor(woodAccent, 0.95)
	diagShadow := fadeColor(woodDark, 0.9)

	// Four short diagonal points (NE SE SW NW) first, under the cardinals.
	diags := [4]struct{ ax, ay, px, py float32 }{
		{ax: sqrt2Inv, ay: -sqrt2Inv, px: sqrt2Inv, py: sqrt2Inv},
		{ax: sqrt2Inv, ay: sqrt2Inv, px: -sqrt2Inv, py: sqrt2Inv},
		{ax: -sqrt2Inv, ay: sqrt2Inv, px: -sqrt2Inv, py: -sqrt2Inv},
		{ax: -sqrt2Inv, ay: -sqrt2Inv, px: sqrt2Inv, py: -sqrt2Inv},
	}
	diagHalfW := outerR * 0.11
	diagReach := outerR * 0.62
	for _, c := range diags {
		tip := rl.NewVector2(cx+c.ax*diagReach, cy+c.ay*diagReach)
		leftBase := rl.NewVector2(cx+c.px*diagHalfW, cy+c.py*diagHalfW)
		rightBase := rl.NewVector2(cx-c.px*diagHalfW, cy-c.py*diagHalfW)
		drawTriangleCCW(center, tip, leftBase, diagLit)
		drawTriangleCCW(center, rightBase, tip, diagShadow)
	}

	// Four long cardinal points (N E S W) — North fully bright (heraldic).
	cardinals := [4]struct {
		ax, ay, px, py float32
	}{
		{ax: 0, ay: -1, px: 1, py: 0},  // N
		{ax: 1, ay: 0, px: 0, py: 1},   // E
		{ax: 0, ay: 1, px: -1, py: 0},  // S
		{ax: -1, ay: 0, px: 0, py: -1}, // W
	}
	pointHalfW := outerR * 0.17
	for i, c := range cardinals {
		tip := rl.NewVector2(cx+c.ax*outerR*0.96, cy+c.ay*outerR*0.96)
		leftBase := rl.NewVector2(cx+c.px*pointHalfW, cy+c.py*pointHalfW)
		rightBase := rl.NewVector2(cx-c.px*pointHalfW, cy-c.py*pointHalfW)
		hi := fadeColor(giltBright, 0.82)
		if i == 0 {
			hi = giltBright
		}
		lo := fadeColor(giltDim, 0.9)
		drawTriangleCCW(center, tip, leftBase, hi)
		drawTriangleCCW(center, rightBase, tip, lo)
	}

	// Centre hub — a socketed gilt boss (dark seat, wood ring, bright pip).
	rl.DrawCircleV(center, innerR+1.5, woodDark)
	rl.DrawCircleV(center, innerR, woodAccent)
	rl.DrawCircleV(center, innerR*0.55, giltBright)
	// "N" letter above the north point, measured through the shared cache (avoids a per-frame cgo measure).
	nLetter := "N"
	nm := compassMeasureCache.measure(font, nLetter, FontTiny, 1)
	drawTextWithShadow(font, nLetter, cx-nm.X/2, cy-outerR-nm.Y-2, FontTiny, inkAccent)
}

var compassMeasureCache measureCache

// visitedAt reports whether the player has stepped on this tile (bounds-checked).
func visitedAt(g *core.GameState, x, z int) bool {
	if g.Visited == nil || z < 0 || z >= len(g.Visited) {
		return false
	}
	if x < 0 || x >= len(g.Visited[z]) {
		return false
	}
	return g.Visited[z][x]
}

// drawPanelsMapArrow paints the player marker — a spear-shaped triangle facing the player, sized to cellPx.
// Uses drawFacingArrow at a 1.0:0.7 forward:sideways ratio so it reads as a spearhead, not the minimap's equilateral.
func drawPanelsMapArrow(cx, cy, cellPx float32, facing int) {
	r := cellPx * 0.45
	if r < 5 {
		r = 5
	}
	drawFacingArrow(rl.NewVector2(cx, cy), r, r*0.7, facing, playerArrowColor)
}

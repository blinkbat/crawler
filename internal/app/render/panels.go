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

// panelsMapFooterMeasureCache memoizes the footer width so the hint bar's start X isn't re-shaped per frame.
var panelsMapFooterMeasureCache measureCache

// panelTabDrawers dispatches by tab index to the per-tab body drawer (init asserts none nil).
var panelTabDrawers = [core.PanelTabCount]func(*core.GameState, Resources, rl.Rectangle){
	core.PanelTabStats:     drawPanelsStats,
	core.PanelTabEquipment: drawPanelsEquipment,
	core.PanelTabItems:     drawPanelsItems,
	core.PanelTabSkills:    drawPanelsSkills,
	core.PanelTabQuests:    drawPanelsQuests,
	core.PanelTabMap:       drawPanelsMap,
}

// footerHintMemberTabs is the shared hint for member-cursor-only tabs (Stats/Quests/Map).
func footerHintMemberTabs() []HintSeg {
	return []HintSeg{
		Hint("Tabs", GlyphLB, GlyphRB),
		Hint("Member", GlyphLeftRight),
		Hint("Close", GlyphB),
	}
}

// footerHintCharacterTab adds the formation Swap to the member-tab hints.
func footerHintCharacterTab() []HintSeg {
	return []HintSeg{
		Hint("Tabs", GlyphLB, GlyphRB),
		Hint("Member", GlyphLeftRight),
		Hint("Swap", GlyphX),
		Hint("Close", GlyphB),
	}
}

// panelTabFooterHints is the per-tab footer hint, parallel to panelTabDrawers. Functions (not values) so each call rebuilds fresh segs.
var panelTabFooterHints = [core.PanelTabCount]func() []HintSeg{
	core.PanelTabStats: footerHintCharacterTab,
	core.PanelTabEquipment: func() []HintSeg {
		return []HintSeg{
			Hint("Tabs", GlyphLB, GlyphRB),
			Hint("Member", GlyphLeftRight),
			Hint("Slot", GlyphUpDown),
			Hint("Change gear", GlyphA),
			Hint("Close", GlyphB),
		}
	},
	core.PanelTabItems: func() []HintSeg {
		return []HintSeg{
			Hint("Tabs", GlyphLB, GlyphRB),
			Hint("Item", GlyphUpDown),
			Hint("Use", GlyphX),
			Hint("Close", GlyphB),
		}
	},
	core.PanelTabSkills: func() []HintSeg {
		return []HintSeg{
			Hint("Tabs", GlyphLB, GlyphRB),
			Hint("Member", GlyphLeftRight),
			Hint("Open trees", GlyphA),
			Hint("Cast heal", GlyphX),
			Hint("Close", GlyphB),
		}
	},
	core.PanelTabQuests: func() []HintSeg {
		return []HintSeg{
			Hint("Tabs", GlyphLB, GlyphRB),
			Hint("Quests / Bestiary", GlyphLeftRight),
			Hint("Scroll", GlyphUpDown),
			Hint("Close", GlyphB),
		}
	},
	core.PanelTabMap: footerHintMemberTabs,
}

func init() {
	for t := core.PanelTab(0); t < core.PanelTabCount; t++ {
		if panelTabDrawers[t] == nil {
			panic(fmt.Sprintf("render/panels: panelTabDrawers missing a drawer for tab %d", int(t)))
		}
		if panelTabFooterHints[t] == nil {
			panic(fmt.Sprintf("render/panels: panelTabFooterHints missing a hint for tab %d", int(t)))
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
	infoY := tabRowY + tabH + 4
	areaName := g.Area.Name
	if areaName == "" {
		areaName = "Unknown"
	}
	drawTextWithShadow(font, areaName, float32(cardX+24), float32(infoY), FontSmall, textPrimary)
	drawTextRightAligned(font, goldLabelFull(g.Gold), float32(cardX+cardW-24), float32(infoY), FontSmall, borderActive)
	// Header rule under the info strip — wood-accent hairline with diamond termini.
	stripRuleY := infoY + panelsInfoStripH
	stripRuleCol := woodAccentRule
	drawPipCappedRule(cardX+24, stripRuleY, cardW-48, stripRuleCol, 1.8, stripRuleCol)

	bodyY := infoY + panelsInfoStripH + 6
	bodyRect := rl.NewRectangle(float32(cardX+hudContentInsetX), float32(bodyY),
		float32(cardW-2*hudContentInsetX), float32(cardY+cardH-26-bodyY-overlayFooterReserve))

	if int(g.PanelsTab) >= 0 && int(g.PanelsTab) < len(panelTabDrawers) {
		panelTabDrawers[g.PanelsTab](g, assets, bodyRect)
	}

	footerHint := panelTabFooterHints[core.PanelTabStats]
	if int(g.PanelsTab) >= 0 && int(g.PanelsTab) < len(panelTabFooterHints) {
		footerHint = panelTabFooterHints[g.PanelsTab]
	}
	drawModalFooterGlyphs(font, card, footerHint())

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
	inset := int32(14)
	leftPage := rl.NewRectangle(float32(cardX+inset), float32(cardY+inset), float32(cardW/2-inset-6), float32(cardH-inset*2))
	rightPage := rl.NewRectangle(float32(cardX+cardW/2+6), float32(cardY+inset), float32(cardW/2-inset-6), float32(cardH-inset*2))
	rl.DrawRectangleGradientEx(leftPage,
		fadeColor(inkPrimary, 0.035), fadeColor(shadowHeavy, 0.035),
		fadeColor(shadowHeavy, 0.055), fadeColor(shadowHeavy, 0.075))
	rl.DrawRectangleGradientEx(rightPage,
		fadeColor(shadowHeavy, 0.055), fadeColor(shadowHeavy, 0.075),
		fadeColor(inkPrimary, 0.035), fadeColor(shadowHeavy, 0.035))

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
var (
	panelsMapSliceBuf []bool
	panelsMapSeenBuf  []bool
	panelsMapRampBuf  []int8
)

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

// drawPartyMemberCardHeader paints the shared per-member card header (class rail, name, "Lv N · row" sub-label,
// HP+MP bars) and returns the Y where tab content starts. highlight (cursored column) brightens name + washes body.
func drawPartyMemberCardHeader(font rl.Font, m core.PartyMember, col rl.Rectangle, highlight bool) float32 {
	classCol := classAccent(m.Class)

	cardBG := glassMid
	if highlight {
		cardBG = selectedGlassTint(glassMid, 0.9)
	}
	drawGlassPaneRect(col, cardBG)
	// Class accent rail flush to the left edge.
	drawClassRail(int32(col.X), int32(col.Y)+6, stripeWidth, int32(col.Height)-12, classCol)
	if highlight {
		drawGiltFocusRing(rl.NewRectangle(col.X, col.Y, col.Width, col.Height))
	}

	innerX, innerW := memberCardInner(col)

	y := col.Y + 16
	nameCol := textPrimary
	if !highlight {
		nameCol = textMuted
	}
	// Class sigil flanks the name (party-ribbon iconography, larger radius).
	glyphR := float32(12)
	glyphCX := innerX + glyphR
	glyphCY := y + FontHeading/2
	drawClassGlyph(glyphCX, glyphCY, glyphR, m.Class, classCol)
	nameOffset := glyphR*2 + 12
	drawEngravedText(font, m.Name, innerX+nameOffset, y, FontHeading, nameCol)
	y += 36

	// Name doubles as the class label, so the sub-line carries level + formation row (the swap tool lives on this tab).
	sub := "Lv " + strconv.Itoa(m.Level) + " · " + core.RowLabel(m.HomeRow)
	drawTextWithShadow(font, sub, innerX, y, FontBody, textMuted)
	y += 30

	hpFill := hpFillColor(m.HP, m.MaxHP)
	drawBar(font, innerX, y, innerW, barHeightCompact, "HP", m.HP, m.MaxHP, hpFill, m.HP <= 0)
	y += 36
	drawBar(font, innerX, y, innerW, barHeightCompact, "MP", m.MP, m.MaxMP, barMP, m.HP <= 0)
	y += 42

	return y
}

// drawPanelsStats renders the Stats tab: per member, header → 2-col stat grid → armor/XP row → status chip → allocate hints.
func drawPanelsStats(g *core.GameState, assets Resources, body rl.Rectangle) {
	font := assets.Font()
	if len(g.Party) == 0 {
		return
	}
	cols := memberColumnLayout(body, len(g.Party))
	for i, m := range g.Party {
		highlight := i == g.PanelsRowCursor
		contentY := drawPartyMemberCardHeader(font, m, cols[i], highlight)
		// Swap source (awaiting a partner) gets a green outline, distinct from the cursor's gilt ring.
		if i == g.PanelSwapSource {
			drawPanelOutline(int32(cols[i].X)-2, int32(cols[i].Y)-2, int32(cols[i].Width)+4, int32(cols[i].Height)+4, borderTarget)
		}
		innerX, innerW := memberCardInner(cols[i])

		// Stat grid: 2 cols, ceil(StatCount/2) rows. Each cell: "[icon] LBL  value".
		statColW := innerW / 2
		rowH := float32(30)
		statRows := (core.StatCount + 1) / 2
		statIconCol := woodAccentIconBright
		for s := core.Stat(0); s < core.StatCount; s++ {
			row := int(s) / 2
			col := int(s) % 2
			cellX := innerX + float32(col)*statColW
			cellY := contentY + float32(row)*rowH
			label := core.StatLabel(s)
			value := smallIntLabel(core.StatValue(m.Stats, s))
			drawStatIcon(s, cellX+9, cellY+13, 9, statIconCol)
			drawTextWithShadow(font, label, cellX+24, cellY, FontBody, textMuted)
			drawTextRightAligned(font, value, cellX+statColW-statValueInsetX, cellY, FontBody, textPrimary)
		}
		contentY += float32(statRows) * rowH

		// Armor + XP secondary row, muted.
		contentY += 8
		drawTextWithShadow(font, "ARM", innerX, contentY, FontSmall, textMuted)
		armVal := smallIntLabel(m.Armor)
		drawTextRightAligned(font, armVal, innerX+statColW-statValueInsetX, contentY, FontSmall, textPrimary)

		nextXP := core.XPForLevel(m.Level)
		xpText := strconv.Itoa(m.XP) + " / " + strconv.Itoa(nextXP)
		drawTextWithShadow(font, "XP", innerX+statColW, contentY, FontSmall, textMuted)
		drawTextRightAligned(font, xpText, innerX+innerW, contentY, FontSmall, textPrimary)
		contentY += 28

		// Status chip — pill in the per-status accent color.
		if kind, turns := core.PartyStatus(&g.Party[i]); kind != core.PartyStatusNone {
			label := partyStatusTurnLabel(kind, turns)
			lm := measurePanelStatValue(font, label, FontSmall)
			chipW := lm.X + 20
			chipH := float32(26)
			chipX := innerX
			col, _ := partyStatusVisual(kind)
			// Shares drawStatusPill with the enemy-roster pill; left-aligned.
			drawStatusPill(font, chipX, contentY, chipW, chipH,
				fadeColor(col, 0.28), fadeColor(col, 0.85), label, col, false)
			contentY += chipH + 8
		}

		// Allocate hint: cursored member only, only when there's something to spend; bottom-of-card CTA.
		if highlight && (m.PendingLevelUps > 0 || m.SkillPoints > 0) {
			hintY := cols[i].Y + cols[i].Height - 60
			if m.PendingLevelUps > 0 {
				// Gamepad-first: CTA reads as the Confirm glyph (A/Z opens the level-up modal — explore/panels.go).
				label := "allocate " + strconv.Itoa(m.PendingLevelUps) + " stat pt" + plural(m.PendingLevelUps)
				drawHintSegs(font, []HintSeg{Hint(label, GlyphA)}, innerX, hintY, FontSmall, inkAccent, 1)
				hintY += 24
			}
			if m.SkillPoints > 0 {
				hint := strconv.Itoa(m.SkillPoints) + " skill pt" + plural(m.SkillPoints) + "  (Skills tab)"
				drawTextWithShadow(font, hint, innerX, hintY, FontSmall, inkAccent)
			}
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

// equipPanelLayout caches the Equipment tab's hit rects so the input layer can route a click without
// re-running layout. SlotRects is flattened [member][slot] (SlotMember/SlotIdx parallel it). PickerRects
// parallels core.EquipPickerRows; PickerBounds is the card (click-outside dismiss), gated by PickerValid.
type equipPanelLayout struct {
	SlotRects    []rl.Rectangle
	SlotMember   []int
	SlotIdx      []core.EquipSlotIndex
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
		SlotRects:   lastEquipLayout.SlotRects[:0],
		SlotMember:  lastEquipLayout.SlotMember[:0],
		SlotIdx:     lastEquipLayout.SlotIdx[:0],
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
	for i, r := range lastEquipLayout.SlotRects {
		if rl.CheckCollisionPointRec(pt, r) {
			return lastEquipLayout.SlotMember[i], lastEquipLayout.SlotIdx[i], true
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
			lastEquipLayout.SlotRects = append(lastEquipLayout.SlotRects, slotRect)
			lastEquipLayout.SlotMember = append(lastEquipLayout.SlotMember, i)
			lastEquipLayout.SlotIdx = append(lastEquipLayout.SlotIdx, s)

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
	// Footer is painted once by DrawPanelsOverlay from panelTabFooterHints — no per-tab inline footer here.
}

// Shared picker sub-modal geometry. use-target + heal pickers are visually identical and share these;
// the equip picker keeps its OWN taller header (extra "Equipped: …" sub-title line) + rows as equipPicker* below.
const (
	pickerRowH    = float32(44)
	pickerHeaderH = float32(56)
	pickerFooterH = float32(32)
)

// equipPicker* are the equip picker's own geometry (taller header for the "Equipped: …" sub-title); see above.
const (
	equipPickerRowH    = float32(46)
	equipPickerHeaderH = float32(70)
	equipPickerFooterH = float32(34)
	// equipPickerSubtitleDY is the "Equipped: …" sub-title baseline, kept next to the header tokens so it can't drift.
	equipPickerSubtitleDY = float32(52)
)

// statValueInsetX is the right-edge inset for a right-aligned value in a Stats-tab cell, so the number column shares one gutter.
const statValueInsetX = float32(14)

// pickerCardLeftInset is the shared left gutter for a picker's title + footer hint.
const pickerCardLeftInset = float32(26)

// pickerTitleTopInset is the shared top inset for a picker's FontHeading title.
const pickerTitleTopInset = float32(20)

// drawPickerCard paints the shared picker chrome (veiled wood-and-glass card + left-aligned title) and returns
// the card rect, consolidating the drawVeiledCard + title preamble the three pickers and the skill-tree modal repeated.
func drawPickerCard(font rl.Font, cardW, cardH float32, title string) rl.Rectangle {
	card := drawVeiledCard(int32(cardW), int32(cardH), borderActive, woodAccent, woodAccent)
	drawEngravedText(font, title, card.X+pickerCardLeftInset, card.Y+pickerTitleTopInset, FontHeading, textPrimary)
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
	cardW := float32(440)
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
		drawTextWithShadow(font, "No eligible items in inventory.", card.X+pickerCardLeftInset, card.Y+headerH+8, FontBody, textHint)
	}
	listY := card.Y + headerH
	for i, row := range rows {
		rect := pickerRowRect(card, listY, i, rowH)
		lastEquipLayout.PickerRects = append(lastEquipLayout.PickerRects, rect)
		focused := i == g.EquipPickerCursor
		drawFocusableRow(rect, focused)
		if row.Unequip {
			drawTextWithShadow(font, "Unequip", rect.X+14, rect.Y+rect.Height/2-10, FontBody, inkAccent)
			continue
		}
		def := core.ItemInfo(row.Kind)
		slotIconForKind(def.Slot)(rect.X+18, rect.Y+rect.Height/2, 11, giltBright)
		name := def.Name
		if row.Count > 1 {
			name += "  x" + strconv.Itoa(row.Count)
		}
		drawTextWithShadow(font, name, rect.X+38, rect.Y+4, FontSmall, textPrimary)
		if bonus := equipBonusSummary(def); bonus != "" {
			drawTextWithShadow(font, bonus, rect.X+38, rect.Y+rect.Height-20, FontSmall, inkAccent)
		}
	}

	drawModalFooterGlyphsLeft(font, card, card.X+pickerCardLeftInset, []HintSeg{
		Hint("Equip", GlyphA),
		Hint("Cancel", GlyphB),
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
	const rowH = float32(58)
	const headerH = pickerHeaderH
	visibleRows := len(living)
	if visibleRows < 1 {
		visibleRows = 1
	}
	cardW := float32(430)
	cardH := headerH + float32(visibleRows)*rowH + pickerFooterH
	card := drawPickerCard(font, cardW, cardH, title)

	if len(living) == 0 {
		drawTextWithShadow(font, "No one can be healed.", card.X+pickerCardLeftInset, card.Y+headerH, FontBody, textHint)
	}
	listY := card.Y + headerH
	for i, mi := range living {
		rect := pickerRowRect(card, listY, i, rowH)
		drawFocusableRow(rect, i == g.UseTargetCursor)
		m := &g.Party[mi]
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
		barX := rect.X + rect.Width*0.46
		barW := rect.Width*0.54 - 16
		drawBarLive(font, "use:hp:"+m.Name, barX, rect.Y+8, barW, barHeightMini, "HP", m.HP, m.MaxHP, hpFillColor(m.HP, m.MaxHP), false)
		drawBar(font, barX, rect.Y+30, barW, barHeightMini, "MP", m.MP, m.MaxMP, barMP, false)
	}

	drawModalFooterGlyphsLeft(font, card, card.X+pickerCardLeftInset, []HintSeg{
		Hint("Use", GlyphA),
		Hint("Cancel", GlyphB),
	})
}

// drawHealPicker paints the out-of-battle heal-skill chooser: the caster's heals with MP cost, cursored row gilded.
// Raised only when a member has more than one such heal; a single heal casts directly. Controller-driven (HealPickCursor).
func drawHealPicker(g *core.GameState, assets Resources) {
	font := assets.Font()
	caster := g.HealPickCaster
	if caster < 0 || caster >= len(g.Party) {
		return
	}
	heals := core.OutOfBattleHealsInto(healPickerHealsDrawBuf, &g.Party[caster])
	healPickerHealsDrawBuf = heals
	if len(heals) == 0 {
		return
	}

	const rowH = pickerRowH
	const headerH = pickerHeaderH
	cardW := float32(360)
	cardH := headerH + float32(len(heals))*rowH + pickerFooterH
	card := drawPickerCard(font, cardW, cardH, "Cast Heal — "+g.Party[caster].Name)

	listY := card.Y + headerH
	for i, s := range heals {
		rect := pickerRowRect(card, listY, i, rowH)
		drawFocusableRow(rect, i == g.HealPickCursor)
		drawTextWithShadow(font, core.SkillName(s), rect.X+14, rect.Y+rect.Height/2-10, FontBody, textPrimary)
		costText := skillCostMPLabel(core.SkillCost(s))
		drawTextRightAligned(font, costText, rect.X+rect.Width-12, rect.Y+rect.Height/2-8, FontSmall, inkAccent)
	}

	drawModalFooterGlyphsLeft(font, card, card.X+pickerCardLeftInset, []HintSeg{
		Hint("Cast", GlyphA),
		Hint("Cancel", GlyphB),
	})
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
		parts = append(parts, "Armor +"+strconv.Itoa(def.ArmorBonus))
	}
	if def.MDefBonus != 0 {
		parts = append(parts, "MDef +"+strconv.Itoa(def.MDefBonus))
	}
	for s := core.Stat(0); s < core.StatCount; s++ {
		v := def.StatBonus[s]
		if v == 0 {
			continue
		}
		sign := "+"
		if v < 0 {
			sign = ""
		}
		parts = append(parts, core.StatLabel(s)+" "+sign+strconv.Itoa(v))
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
		dy := detailRect.Y + 14
		dx := detailRect.X + 14
		drawEngravedText(font, info.Name, dx, dy, FontHeading, textPrimary)
		dy += 38
		drawTextWithShadow(font, panelsItemEffectLabel(info), dx, dy, FontBody, inkAccent)
		dy += 30
		owned := "Owned: " + strconv.Itoa(stack.Count)
		drawTextWithShadow(font, owned, dx, dy, FontBody, textMuted)
		dy += 36
		// Description placeholder — the item registry carries none today.
		hint := "Consumable. Use from the battle menu's Item action."
		drawTextWithShadow(font, hint, dx, dy, FontSmall, textHint)
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

// treeRatioLabel memoizes the Skills-tab "invested / total" strings (bounded operands, so the memo can't grow unbounded).
var treeRatioLabelCache = map[[2]int]string{}

func treeRatioLabel(invested, total int) string {
	k := [2]int{invested, total}
	if s, ok := treeRatioLabelCache[k]; ok {
		return s
	}
	s := strconv.Itoa(invested) + " / " + strconv.Itoa(total)
	treeRatioLabelCache[k] = s
	return s
}

// panelsItemEffectLabel is the Items-tab detail line for a consumable: cached "+N HP", "+N MP", both, or a no-effect note.
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
		spText := skillPointsLabel(m.SkillPoints)
		spCol := textMuted
		if m.SkillPoints > 0 {
			spCol = inkAccent
		}
		drawTextWithShadow(font, "SKILL POINTS", innerX, contentY, FontSmall, textMuted)
		drawTextRightAligned(font, spText, innerX+innerW, contentY, FontSmall, spCol)
		contentY += 30

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

			drawTextWithShadow(font, tr.Name, rect.X+12, rect.Y+8, FontBody, textPrimary)
			invested := core.TreeInvestedRanks(&m, tr)
			ratio := treeRatioLabel(invested, core.TreeMaxRanks(tr))
			ratioCol := textMuted
			if invested > 0 {
				ratioCol = giltBright
			}
			drawTextRightAligned(font, ratio, rect.X+rect.Width-12, rect.Y+10, FontSmall, ratioCol)
			drawTextWithShadow(font, tr.Theme, rect.X+12, rect.Y+34, FontSmall, textHint)
		}

		// Cursored member: Confirm opens the trees.
		if highlight {
			hintY := cols[i].Y + cols[i].Height - 46
			DrawHintBar(font, []HintSeg{Hint("Open skill trees", GlyphA)}, cols[i].X+cols[i].Width/2, hintY, FontSmall)
		}
	}
}

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
	}
	slice := panelsMapSliceBuf[:n]
	seen := panelsMapSeenBuf[:n]
	ramp := panelsMapRampBuf[:n]
	for localZ := -1; localZ <= cellsY; localZ++ {
		for localX := -1; localX <= cellsX; localX++ {
			col, onSlice, seenWall, rampDir := mapSliceCell(m, g, indoor, startX+localX, startZ+localZ)
			i := (localZ+1)*gw + (localX + 1)
			slice[i], seen[i], ramp[i] = onSlice, seenWall, rampDir
			if localX < 0 || localX >= cellsX || localZ < 0 || localZ >= cellsY {
				continue
			}
			// Derive each cell's rect from consecutive truncated edges (this cell's left, the next cell's left),
			// not origin+size independently — with a fractional cellPx that left 1px seams. Edge-to-edge tiling abuts cleanly.
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
			rl.DrawRectangle(px, py, pw, ph, col)
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

	// Map footer — zoom indicator (area name is already in the top info strip), then the control hint.
	footerY := body.Y + body.Height - 20
	footer := panelsMapFooterText(zoom)
	drawTextWithShadow(font, footer, body.X, footerY, FontSmall, textHint)
	footerW := panelsMapFooterMeasureCache.measure(font, footer, FontSmall, canonicalSpacing(FontSmall)).X
	DrawHintBarLeft(font, []HintSeg{Hint("Pan", GlyphLeftRight), Hint("Zoom", GlyphUpDown)}, body.X+footerW+hintSegGap, footerY, FontSmall)
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

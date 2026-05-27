package render

import (
	"crawler/internal/app/core"
	"fmt"
	"strconv"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// panelsMapOutOfBoundsColor is the dim fill used for cells outside
// the area bounds on the panels-overlay Map tab. Lifted to a package
// var so the per-cell loop doesn't build the rl.Color struct from
// the same literal every iteration (the panels map iterates every
// visible cell each frame the overlay is open — ~450 cells worst
// case).
var panelsMapOutOfBoundsColor = rl.NewColor(6, 8, 14, 235)

// panelsMapFooterCache memoizes the "AreaName   zoom: N cells   ..."
// footer string drawn on the Map tab. Both inputs change only on
// user action (open the overlay, change zoom, transition area), so
// rebuilding via fmt.Sprintf every frame is pure waste.
var panelsMapFooterCache struct {
	areaName string
	zoom     int
	text     string
}

func panelsMapFooterText(areaName string, zoom int) string {
	if panelsMapFooterCache.areaName == areaName && panelsMapFooterCache.zoom == zoom {
		return panelsMapFooterCache.text
	}
	panelsMapFooterCache.text = fmt.Sprintf("%s   zoom: %d cells   Up/Down to zoom", areaName, zoom)
	panelsMapFooterCache.areaName = areaName
	panelsMapFooterCache.zoom = zoom
	return panelsMapFooterCache.text
}

// DrawPanelsOverlay paints the game-panels modal — the five-tab overlay
// raised by the gamepad middle button / keyboard I. Routes by
// g.PanelsTab to the per-tab body drawer; the tab strip + footer hint
// are drawn once around all of them so the chrome stays consistent.
// No-op when the overlay isn't open.
func DrawPanelsOverlay(g core.GameState, assets Resources) {
	if !g.PanelsOpen {
		return
	}
	font := assets.Font()
	// Panels overlay skips drawModalScaffold's heading band — the tab
	// strip IS the heading. We do reuse the veil + centered card.
	card := drawModalScaffold(font, overlayCardWidthHuge, overlayCardHeightLarge, "")
	cardX, cardY := int32(card.X), int32(card.Y)
	cardW, cardH := int32(card.Width), int32(card.Height)

	// Tab strip across the top. Each tab is a flat label on a soft
	// glass tile; the active tab gets a brighter glass + a thick gilt
	// underline so the eye lands on it before the body content. No
	// wood frame per tab — they read as ledger dividers, not nested
	// panels-in-a-panel.
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
			bg = core.MixColor(glassMid, glassWarm, 0.65)
			txt = textPrimary
		}
		drawGlassPane(tx, tabRowY, tabW, tabH, bg)
		if active {
			// Gilt underline strip at the bottom of the active tab —
			// the same "you're here" mark the list rows use, scaled
			// to the tab width.
			rl.DrawRectangle(tx+8, tabRowY+tabH-3, tabW-16, 2, giltBright)
		}
		label := core.PanelTabLabel(t)
		m := measureTabLabel(font, label)
		drawTextWithShadow(font, label,
			float32(tx)+float32(tabW)/2-m.X/2,
			float32(tabRowY)+float32(tabH)/2-m.Y/2-1,
			FontSmall, txt)
	}

	bodyY := tabRowY + tabH + 18
	bodyRect := rl.NewRectangle(float32(cardX+22), float32(bodyY),
		float32(cardW-44), float32(cardY+cardH-26-bodyY-overlayFooterReserve))

	switch g.PanelsTab {
	case core.PanelTabStats:
		drawPanelsStats(g, assets, bodyRect)
	case core.PanelTabEquipment:
		drawPanelsEquipment(g, assets, bodyRect)
	case core.PanelTabItems:
		drawPanelsItems(g, assets, bodyRect)
	case core.PanelTabSkills:
		drawPanelsSkills(g, assets, bodyRect)
	case core.PanelTabMap:
		drawPanelsMap(g, assets, bodyRect)
	}

	DrawFooterHint(font, "L1/R1 tabs   Left/Right pick member   X close",
		float32(cardX+cardW/2), float32(cardY+cardH-22), FontTiny)
}

// tabLabelMeasureCache memoizes panel-tab label measurements. Tab labels
// come from a fixed core registry and never change at runtime, so this
// cache fills once and stays warm.
var tabLabelMeasureCache = make(map[string]rl.Vector2, 8)
var tabLabelMeasureCacheFontID uint32

func measureTabLabel(font rl.Font, label string) rl.Vector2 {
	if font.Texture.ID != tabLabelMeasureCacheFontID {
		tabLabelMeasureCache = make(map[string]rl.Vector2, 8)
		tabLabelMeasureCacheFontID = font.Texture.ID
	}
	if v, ok := tabLabelMeasureCache[label]; ok {
		return v
	}
	v := rl.MeasureTextEx(font, label, FontSmall, 1)
	tabLabelMeasureCache[label] = v
	return v
}

// memberColumnLayout returns the per-member column rectangles for any
// tab that paints one card per party member (Stats / Equipment /
// Skills). Equal-width columns with a small gap so the grid reads as
// "ledger of party members" rather than a single dense slab.
func memberColumnLayout(body rl.Rectangle, count int) ([]rl.Rectangle, float32) {
	const gap = float32(14)
	if count <= 0 {
		return nil, 0
	}
	total := body.Width - gap*float32(count-1)
	colW := total / float32(count)
	cols := make([]rl.Rectangle, count)
	for i := 0; i < count; i++ {
		cols[i] = rl.NewRectangle(body.X+float32(i)*(colW+gap), body.Y, colW, body.Height)
	}
	return cols, colW
}

// drawPartyMemberCardHeader paints the shared header chrome every
// per-member tab card uses: class accent stripe on the left edge, the
// member's name, a "Class · Lv N" sub-label, and the HP+MP bars. The
// returned float32 is the Y coordinate immediately below the bars —
// where tab-specific content (stat grid, equipment slots, skill list)
// should start drawing.
//
// `highlight` is true for the currently-cursored column; it brightens
// the name and tints the card body so the active member pops without
// adding a heavy second selection chrome.
func drawPartyMemberCardHeader(font rl.Font, m core.PartyMember, col rl.Rectangle, highlight bool) float32 {
	classCol := partyClassPresentationFor(m.Class).turnColor

	// Soft inset glass body. We don't paint a full wood-framed card per
	// member (would compete with the outer modal's frame); a small
	// rounded panel + a class stripe gives enough separation.
	cardBG := glassMid
	if highlight {
		cardBG = core.MixColor(glassMid, glassWarm, 0.55)
	}
	drawGlassPane(int32(col.X), int32(col.Y), int32(col.Width), int32(col.Height), cardBG)
	rl.DrawRectangle(int32(col.X), int32(col.Y)+6, 3, int32(col.Height)-12, classCol)

	innerX := col.X + 14
	innerW := col.Width - 24

	y := col.Y + 12
	nameCol := textPrimary
	if !highlight {
		nameCol = textMuted
	}
	// Class sigil flanks the name — same iconography as the party
	// ribbon's card, but at a slightly larger radius so it reads
	// as a banner crest in this fuller pane.
	glyphR := float32(9)
	glyphCX := innerX + glyphR
	glyphCY := y + FontBody/2
	drawClassGlyph(glyphCX, glyphCY, glyphR, m.Class, classCol)
	nameOffset := glyphR*2 + 10
	drawTextWithShadow(font, m.Name, innerX+nameOffset, y, FontBody, nameCol)
	y += 26

	// PartyMember.Name doubles as the class label in this build, so the
	// sub-line just carries the level — no need to repeat the class.
	sub := "Lv " + strconv.Itoa(m.Level)
	drawTextWithShadow(font, sub, innerX, y, FontSmall, textMuted)
	y += 24

	hpFill := hpFillColor(m.HP, m.MaxHP)
	drawBar(font, innerX, y, innerW, 22, "HP", m.HP, m.MaxHP, hpFill, m.HP <= 0)
	y += 28
	drawBar(font, innerX, y, innerW, 22, "MP", m.MP, m.MaxMP, barMP, m.HP <= 0)
	y += 32

	return y
}

// drawPanelsStats renders the Stats tab as one card per party member.
// Each card stacks: class accent + name + level + HP/MP bars (shared
// header) → 2-column stat grid (STR/DEX/INT/WIS/VIT/SPD) → armor / XP
// row → status pill chip → allocate hints for the cursored member.
func drawPanelsStats(g core.GameState, assets Resources, body rl.Rectangle) {
	font := assets.Font()
	if len(g.Party) == 0 {
		return
	}
	cols, colW := memberColumnLayout(body, len(g.Party))
	for i, m := range g.Party {
		highlight := i == g.PanelsRowCursor
		contentY := drawPartyMemberCardHeader(font, m, cols[i], highlight)
		innerX := cols[i].X + 14
		innerW := colW - 28

		// Stat grid: 2 columns, ceil(StatCount/2) rows. Each cell paints
		// "[icon] LBL  value" with the icon in soft gilt to the left of
		// the label, the label muted, and the value bright so the eye
		// scans the numbers without losing the sigil row anchor.
		statColW := innerW / 2
		rowH := float32(22)
		statRows := (core.StatCount + 1) / 2
		statIconCol := fadeColor(woodAccent, 0.9)
		for s := core.Stat(0); s < core.StatCount; s++ {
			row := int(s) / 2
			col := int(s) % 2
			cellX := innerX + float32(col)*statColW
			cellY := contentY + float32(row)*rowH
			label := core.StatLabel(s)
			value := strconv.Itoa(core.StatValue(m.Stats, s))
			drawStatIcon(s, cellX+7, cellY+9, 7, statIconCol)
			drawTextWithShadow(font, label, cellX+18, cellY, FontSmall, textMuted)
			// Value right-aligned within the cell so the column
			// of numbers lines up no matter the label width.
			vm := rl.MeasureTextEx(font, value, FontSmall, 1)
			drawTextWithShadow(font, value, cellX+statColW-vm.X-12, cellY, FontSmall, textPrimary)
		}
		contentY += float32(statRows) * rowH

		// Armor + XP secondary row, slightly muted so they
		// don't compete with the stat grid above.
		contentY += 6
		drawTextWithShadow(font, "ARM", innerX, contentY, FontTiny, textMuted)
		armVal := strconv.Itoa(m.Armor)
		am := rl.MeasureTextEx(font, armVal, FontTiny, 1)
		drawTextWithShadow(font, armVal, innerX+statColW-am.X-12, contentY, FontTiny, textPrimary)

		nextXP := core.XPForLevel(m.Level)
		xpText := strconv.Itoa(m.XP) + " / " + strconv.Itoa(nextXP)
		drawTextWithShadow(font, "XP", innerX+statColW, contentY, FontTiny, textMuted)
		xm := rl.MeasureTextEx(font, xpText, FontTiny, 1)
		drawTextWithShadow(font, xpText, innerX+innerW-xm.X, contentY, FontTiny, textPrimary)
		contentY += 22

		// Status chip — bright pill in the per-status accent
		// color so afflicted members read at a glance.
		if kind, turns := core.PartyStatus(m); kind != core.PartyStatusNone {
			label := partyStatusTurnLabel(kind, turns)
			lm := measurePartyStatusLabel(font, label)
			chipW := lm.X + 18
			chipH := float32(20)
			chipX := innerX
			col, _ := partyStatusVisual(kind)
			drawSmallPanel(int32(chipX), int32(contentY), int32(chipW), int32(chipH), fadeColor(col, 0.28))
			drawSmallPanelOutline(int32(chipX), int32(contentY), int32(chipW), int32(chipH), fadeColor(col, 0.85))
			drawTextWithShadow(font, label, chipX+9, contentY+2, FontTiny, col)
			contentY += chipH + 8
		}

		// Allocate hint: only on the cursored member, only when
		// there's something to spend. Painted near the bottom of
		// the card so it reads as a call-to-action footer.
		if highlight && (m.PendingLevelUps > 0 || m.SkillPoints > 0) {
			hintY := cols[i].Y + cols[i].Height - 44
			if m.PendingLevelUps > 0 {
				hint := "Z   allocate " + strconv.Itoa(m.PendingLevelUps) + " stat pt" + plural(m.PendingLevelUps)
				drawTextWithShadow(font, hint, innerX, hintY, FontTiny, inkAccent)
				hintY += 16
			}
			if m.SkillPoints > 0 {
				hint := strconv.Itoa(m.SkillPoints) + " skill pt" + plural(m.SkillPoints) + "  (Skills tab)"
				drawTextWithShadow(font, hint, innerX, hintY, FontTiny, inkAccent)
			}
		}
	}
}

// plural returns the "s" suffix when n != 1 so labels like "1 pt"
// / "2 pts" don't grow special-case branches. Tiny helper kept
// next to the only caller for now.
func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// drawPanelsEquipment renders the Equipment tab. The equipment system
// isn't authored yet, so the slots read as placeholders — but the
// layout matches the Stats tab so the eye doesn't have to retrain on
// every tab switch. Each slot row inside the card shows the label
// (FontSmall, muted) above the value (FontBody, primary or dim
// depending on whether the slot is filled).
func drawPanelsEquipment(g core.GameState, assets Resources, body rl.Rectangle) {
	font := assets.Font()
	if len(g.Party) == 0 {
		return
	}
	cols, colW := memberColumnLayout(body, len(g.Party))
	slotLabels := []string{"WEAPON", "ARMOR", "ACCESSORY"}
	for i, m := range g.Party {
		highlight := i == g.PanelsRowCursor
		contentY := drawPartyMemberCardHeader(font, m, cols[i], highlight)
		innerX := cols[i].X + 14
		innerW := colW - 28

		slotRowH := float32(46)
		for slotIdx, label := range slotLabels {
			rowY := contentY + float32(slotIdx)*slotRowH
			// Slot bezel: very faint inset so the three rows
			// read as a stack without competing with the card.
			drawGlassPane(int32(innerX), int32(rowY), int32(innerW), int32(slotRowH-8), fadeColor(glassDeep, 0.55))

			// Slot sigil — small pictograph beside the label. The
			// equipment system isn't authored yet so the sigil
			// renders dim; once an item slots in, a future pass
			// can swap the colour to the item rarity tone.
			iconCol := fadeColor(woodAccent, 0.7)
			filled := slotIdx == 1 && m.Armor > 0
			if filled {
				iconCol = giltBright
			}
			iconCX := innerX + 14
			iconCY := rowY + 18
			switch slotIdx {
			case 0:
				drawSlotIconSword(iconCX, iconCY, 9, iconCol)
			case 1:
				drawSlotIconShield(iconCX, iconCY, 9, iconCol)
			case 2:
				drawSlotIconRing(iconCX, iconCY, 8, iconCol)
			}

			labelX := innerX + 32
			drawTextWithShadow(font, label, labelX, rowY+4, FontTiny, textMuted)
			value := "—"
			valCol := textDim
			if filled {
				value = "Armor +" + strconv.Itoa(m.Armor)
				valCol = textPrimary
			}
			drawTextWithShadow(font, value, labelX, rowY+18, FontSmall, valCol)
		}
	}
	footer := "Equipment system pending — values shown reflect the base armor stat only."
	drawTextWithShadow(font, footer, body.X, body.Y+body.Height-16, FontTiny, textHint)
}

// drawSlotIconSword paints a small upright longsword sigil for the
// Weapon equipment slot. Built from a vertical blade (tapered tip
// triangle + body rectangle + centre fuller stripe), a horizontal
// crossguard, and a round pommel. Sized by `r` (the icon's
// half-height).
func drawSlotIconSword(cx, cy, r float32, col rl.Color) {
	bladeHalfW := r * 0.18
	if bladeHalfW < 1.5 {
		bladeHalfW = 1.5
	}
	pommelY := cy - r + 1
	rl.DrawCircleV(rl.NewVector2(cx, pommelY), bladeHalfW*1.4, col)
	guardY := cy - r*0.55
	guardHalfW := r * 0.75
	rl.DrawRectangle(int32(cx-guardHalfW), int32(guardY), int32(guardHalfW*2), 2, col)
	rl.DrawRectangle(int32(cx-guardHalfW), int32(guardY), 2, 3, col)
	rl.DrawRectangle(int32(cx+guardHalfW-2), int32(guardY), 2, 3, col)
	bladeTop := guardY + 2
	bladeBottom := cy + r*0.62
	rl.DrawRectangle(int32(cx-bladeHalfW), int32(bladeTop), int32(bladeHalfW*2), int32(bladeBottom-bladeTop), col)
	fuller := fadeColor(col, 0.5)
	rl.DrawRectangle(int32(cx), int32(bladeTop+2), 1, int32(bladeBottom-bladeTop-4), fuller)
	tip := rl.NewVector2(cx, cy+r)
	left := rl.NewVector2(cx-bladeHalfW, bladeBottom)
	right := rl.NewVector2(cx+bladeHalfW, bladeBottom)
	drawTriangleCCW(tip, right, left, col)
}

// drawSlotIconShield paints a small heater-shield sigil for the
// Armor equipment slot. Built from a top rectangle (shoulders) + a
// bottom triangle (point) + a centre boss (gilt highlight disc).
// Sized by `r` (the icon's half-height).
func drawSlotIconShield(cx, cy, r float32, col rl.Color) {
	topW := r * 1.4
	topH := r * 0.7
	// Shoulders.
	rl.DrawRectangle(int32(cx-topW/2), int32(cy-r), int32(topW), int32(topH), col)
	// Tapered point: triangle from the bottom corners of the
	// shoulders down to a single tip.
	tip := rl.NewVector2(cx, cy+r)
	left := rl.NewVector2(cx-topW/2, cy-r+topH)
	right := rl.NewVector2(cx+topW/2, cy-r+topH)
	drawTriangleCCW(tip, right, left, col)
	// Centre boss — small inner disc + bright pip.
	rl.DrawCircleV(rl.NewVector2(cx, cy-r*0.05), r*0.32, fadeColor(col, 0.55))
	rl.DrawCircleV(rl.NewVector2(cx, cy-r*0.05), r*0.16, giltBright)
}

// drawSlotIconRing paints a small ring sigil with a gem cap for the
// Accessory equipment slot. Built from an annulus (outer circle
// minus inner circle) and a tiny gem dot at the top. Sized by `r`
// (the ring's outer radius).
func drawSlotIconRing(cx, cy, r float32, col rl.Color) {
	// Annulus via outer disc + inner punch-out using the slot's
	// background tone. The background here is the slot bezel
	// (fadeColor(glassDeep, 0.55)); approximate with a slightly
	// darker pure-glass disc so the ring reads hollow at small
	// sizes.
	rl.DrawCircleV(rl.NewVector2(cx, cy+1), r, col)
	rl.DrawCircleV(rl.NewVector2(cx, cy+1), r*0.55, fadeColor(glassDeep, 0.8))
	// Gem dot at top of the ring band — bright gilt highlight.
	rl.DrawCircleV(rl.NewVector2(cx, cy+1-r*0.85), r*0.32, giltBright)
}

// drawPanelsItems renders the Items tab as a clean ledger: a scrollable
// stack list on the left two-thirds, a description panel for the
// cursored item on the right third. Empty inventory falls through to
// a short note so the panel never reads as broken.
func drawPanelsItems(g core.GameState, assets Resources, body rl.Rectangle) {
	font := assets.Font()
	stacks := core.LiveStacks(g.Inventory)
	if len(stacks) == 0 {
		drawTextWithShadow(font, "Your bags are empty.", body.X+12, body.Y+12, FontBody, textMuted)
		drawTextWithShadow(font, "Loot from steals and chests will appear here.",
			body.X+12, body.Y+44, FontSmall, textHint)
		return
	}

	// Two-pane layout: list on the left, detail card on the right.
	const gap = float32(16)
	listW := body.Width*0.62 - gap/2
	detailW := body.Width - listW - gap
	listRect := rl.NewRectangle(body.X, body.Y, listW, body.Height)
	detailRect := rl.NewRectangle(body.X+listW+gap, body.Y, detailW, body.Height)

	// List rows. Generous padding so each row reads as its own
	// ledger line, not a cramped table cell.
	rowH := float32(36)
	rowPad := float32(12)
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
			drawGlassPane(int32(listRect.X), int32(y), int32(listRect.Width), int32(rowH-4),
				core.MixColor(glassMid, glassWarm, 0.6))
			rl.DrawRectangle(int32(listRect.X)+2, int32(y)+4, 3, int32(rowH-12), giltBright)
		}
		nameCol := textMuted
		if highlight {
			nameCol = textPrimary
		}
		drawTextWithShadow(font, info.Name, listRect.X+rowPad, y+8, FontSmall, nameCol)
		// Count on the right edge of the row as a small chip.
		countText := "x" + strconv.Itoa(stack.Count)
		cm := rl.MeasureTextEx(font, countText, FontSmall, 1)
		drawTextWithShadow(font, countText, listRect.X+listRect.Width-cm.X-rowPad, y+8, FontSmall, inkAccent)
	}

	// Detail card: name, type/effect summary, count owned, description
	// stub. Reads as the ledger's "current entry" pane.
	drawGlassPane(int32(detailRect.X), int32(detailRect.Y), int32(detailRect.Width), int32(detailRect.Height), glassMid)
	if cursor < len(stacks) {
		stack := stacks[cursor]
		info := core.ItemInfo(stack.Kind)
		dy := detailRect.Y + 14
		dx := detailRect.X + 14
		drawTextWithShadow(font, info.Name, dx, dy, FontBody, textPrimary)
		dy += 28
		effect := panelsItemHealLabel(info.HealAmount)
		drawTextWithShadow(font, effect, dx, dy, FontSmall, inkAccent)
		dy += 22
		owned := "Owned: " + strconv.Itoa(stack.Count)
		drawTextWithShadow(font, owned, dx, dy, FontSmall, textMuted)
		dy += 28
		// Description placeholder (item registry doesn't carry one
		// today). Wrap a short canned hint so the panel doesn't
		// feel empty.
		hint := "Consumable. Use from the battle menu's Item action."
		drawTextWithShadow(font, hint, dx, dy, FontTiny, textHint)
	}
}

// skillCostMPLabel returns "<cost> MP" with the small cost range
// pre-formatted into a LUT. Skill costs are bounded by class design
// (currently 4-12 MP across all classes); the LUT cap of 32 absorbs
// any tuning slack. Used by both the action menu's skill submenu and
// the panels overlay's Skills tab.
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

// panelsItemHealLabelCache pre-formats "+N HP" for the small heal-
// amount range items currently roll in. The Items tab paints these
// every frame the overlay sits on it; without the cache the path
// runs fmt.Sprintf for every visible stack.
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

// panelsItemHealLabelMeasureCache memoizes MeasureTextEx for the
// right-aligned heal labels. The strings come from the precomputed
// label cache so map keys are bounded; the per-row hot path is two
// map reads instead of two cgo calls + an alloc.
var panelsItemHealLabelMeasureCache = make(map[string]rl.Vector2, 32)
var panelsItemHealLabelMeasureCacheFontID uint32

func measurePanelsItemHealLabel(font rl.Font, label string) rl.Vector2 {
	if font.Texture.ID != panelsItemHealLabelMeasureCacheFontID {
		for k := range panelsItemHealLabelMeasureCache {
			delete(panelsItemHealLabelMeasureCache, k)
		}
		panelsItemHealLabelMeasureCacheFontID = font.Texture.ID
	}
	if v, ok := panelsItemHealLabelMeasureCache[label]; ok {
		return v
	}
	v := rl.MeasureTextEx(font, label, FontSmall, 1)
	panelsItemHealLabelMeasureCache[label] = v
	return v
}

// drawPanelsSkills renders the Skills tab as one card per party
// member, mirroring the Stats / Equipment layout. Inside each card
// the member's learned skills stack as compact rows: name, MP cost
// chip, short description below. The class skill currently armed for
// battle (m.SkillCursor) gets a gilt left spine so the player sees
// at a glance what the Skill action will fire.
func drawPanelsSkills(g core.GameState, assets Resources, body rl.Rectangle) {
	font := assets.Font()
	if len(g.Party) == 0 {
		return
	}
	cols, colW := memberColumnLayout(body, len(g.Party))
	for i, m := range g.Party {
		highlight := i == g.PanelsRowCursor
		contentY := drawPartyMemberCardHeader(font, m, cols[i], highlight)
		innerX := cols[i].X + 14
		innerW := colW - 28

		skills := core.PartySkills(m)
		rowH := float32(54)
		for j, s := range skills {
			rowY := contentY + float32(j)*rowH
			if rowY+rowH-6 > cols[i].Y+cols[i].Height {
				break
			}
			armed := j == m.SkillCursor

			// Soft inset row. The armed skill gets a subtle warm
			// tint so the eye lands on the active loadout.
			rowBG := fadeColor(glassDeep, 0.55)
			if armed {
				rowBG = core.MixColor(rowBG, glassWarm, 0.65)
			}
			drawGlassPane(int32(innerX), int32(rowY), int32(innerW), int32(rowH-8), rowBG)
			if armed {
				rl.DrawRectangle(int32(innerX)+2, int32(rowY)+4, 3, int32(rowH-16), giltBright)
			}

			// Name on the left, MP cost chip on the right of the
			// header line.
			nameCol := textPrimary
			if !armed {
				nameCol = textMuted
			}
			drawTextWithShadow(font, core.SkillName(s), innerX+10, rowY+4, FontSmall, nameCol)
			cost := core.SkillCost(s)
			if cost > 0 {
				costText := skillCostMPLabel(cost)
				cm := rl.MeasureTextEx(font, costText, FontTiny, 1)
				drawTextWithShadow(font, costText, innerX+innerW-cm.X-10, rowY+6, FontTiny, inkAccent)
			}
			// Description on the second line of the row.
			drawTextWithShadow(font, core.SkillDescription(s), innerX+10, rowY+24, FontTiny, textHint)
		}
	}
}

// drawPanelsMap renders the zoomable Map tab. Cells-on-screen comes
// from g.PanelsMapZoom (clamped by the input handler); explored tiles
// paint at full color, unexplored at a heavy fade so the player can
// see the silhouette of the area without ruining the discovery.
// Enemy packs only show on explored tiles (don't spoil unseen rooms).
func drawPanelsMap(g core.GameState, assets Resources, body rl.Rectangle) {
	font := assets.Font()
	m := g.Area
	if m.Width <= 0 || m.Height <= 0 {
		return
	}
	zoom := g.PanelsMapZoom
	if zoom <= 0 {
		zoom = panelsMapZoomDefaultFallback
	}
	// Cell size in pixels: fit `zoom` cells across the body, then use
	// the same size for the vertical so the map stays square. The
	// player is centered in the visible window.
	cellPx := body.Width / float32(zoom)
	if cellPx*float32(zoom) > body.Height {
		cellPx = body.Height / float32(zoom)
	}
	cellsX := int(body.Width / cellPx)
	cellsY := int(body.Height / cellPx)
	startX := g.Player.TileX - cellsX/2
	startZ := g.Player.TileZ - cellsY/2

	mapX := body.X + (body.Width-float32(cellsX)*cellPx)/2
	mapY := body.Y + (body.Height-float32(cellsY)*cellPx)/2

	for localZ := 0; localZ < cellsY; localZ++ {
		for localX := 0; localX < cellsX; localX++ {
			mx := startX + localX
			mz := startZ + localZ
			px := int32(mapX + float32(localX)*cellPx)
			py := int32(mapY + float32(localZ)*cellPx)
			pw := int32(cellPx)
			if pw < 1 {
				pw = 1
			}
			if !m.InBounds(mx, mz) {
				rl.DrawRectangle(px, py, pw, pw, panelsMapOutOfBoundsColor)
				continue
			}
			visited := visitedAt(g, mx, mz)
			rl.DrawRectangle(px, py, pw, pw, tileColorWithFog(m.Materials, m.TileAt(mx, mz), visited))
		}
	}

	// Pack markers are intentionally omitted: the map shows the
	// terrain you've seen, not who's standing on it. Enemies stay a
	// surprise — the player has to actually look at the world view
	// to know what's around. (Earlier passes painted red dots on
	// visited pack tiles; removed when the 3×3 fog-of-war reveal
	// landed, since the wider window made the minimap effectively
	// "where are all the enemies" radar.)

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

	// Player arrow at the center of the visible window.
	plx := g.Player.TileX - startX
	plz := g.Player.TileZ - startZ
	pcx := mapX + (float32(plx)+0.5)*cellPx
	pcy := mapY + (float32(plz)+0.5)*cellPx
	drawPanelsMapArrow(pcx, pcy, cellPx, g.Player.Facing)

	// Compass rose in the upper-right corner — the centerpiece D&D
	// cartography ornament. Sized small enough not to dominate but
	// large enough to read as 8-point.
	drawCompassRose(body.X+body.Width-46, body.Y+10, 28, font)

	// Map footer — area name + zoom indicator.
	footer := panelsMapFooterText(g.Area.Name, zoom)
	drawTextWithShadow(font, footer, body.X, body.Y+body.Height-18, FontTiny, textHint)
}

// drawCompassRose paints an 8-point compass rose at (cx, cy) within
// a diameter `d`. Long N/E/S/W points + short diagonal points + a
// gilt centre disc + a wood-tone outer ring + a tiny "N" letter at
// the north tip. The classic D&D treasure-map crest.
func drawCompassRose(cx, cy float32, d float32, font rl.Font) {
	outerR := d / 2
	innerR := outerR * 0.35
	// Outer wood ring — a circle with a slightly-inset glass
	// background so the rose sits inside its own medallion.
	rl.DrawCircleV(rl.NewVector2(cx, cy), outerR+1, woodDark)
	rl.DrawCircleV(rl.NewVector2(cx, cy), outerR, glassDeep)
	rl.DrawCircleLines(int32(cx), int32(cy), outerR, woodAccent)

	// Four long cardinal points (N E S W). Each is a kite drawn as
	// two triangles from the centre to the tip.
	cardinals := [4]struct {
		ax, ay, px, py float32
	}{
		{ax: 0, ay: -1, px: 1, py: 0}, // N
		{ax: 1, ay: 0, px: 0, py: 1},  // E
		{ax: 0, ay: 1, px: -1, py: 0}, // S
		{ax: -1, ay: 0, px: 0, py: -1}, // W
	}
	pointHalfW := outerR * 0.18
	for i, c := range cardinals {
		tip := rl.NewVector2(cx+c.ax*outerR*0.95, cy+c.ay*outerR*0.95)
		leftBase := rl.NewVector2(cx+c.px*pointHalfW, cy+c.py*pointHalfW)
		rightBase := rl.NewVector2(cx-c.px*pointHalfW, cy-c.py*pointHalfW)
		col := giltDim
		if i == 0 {
			// North gets the bright point — the heraldic convention.
			col = giltBright
		}
		drawTriangleCCW(rl.NewVector2(cx, cy), tip, leftBase, col)
		drawTriangleCCW(rl.NewVector2(cx, cy), rightBase, tip, col)
	}
	// Four short diagonal points (NE SE SW NW), in soft wood tone.
	diags := [4]struct{ ax, ay, px, py float32 }{
		{ax: sqrt2Inv, ay: -sqrt2Inv, px: sqrt2Inv, py: sqrt2Inv},
		{ax: sqrt2Inv, ay: sqrt2Inv, px: -sqrt2Inv, py: sqrt2Inv},
		{ax: -sqrt2Inv, ay: sqrt2Inv, px: -sqrt2Inv, py: -sqrt2Inv},
		{ax: -sqrt2Inv, ay: -sqrt2Inv, px: sqrt2Inv, py: -sqrt2Inv},
	}
	diagHalfW := outerR * 0.12
	diagReach := outerR * 0.65
	for _, c := range diags {
		tip := rl.NewVector2(cx+c.ax*diagReach, cy+c.ay*diagReach)
		leftBase := rl.NewVector2(cx+c.px*diagHalfW, cy+c.py*diagHalfW)
		rightBase := rl.NewVector2(cx-c.px*diagHalfW, cy-c.py*diagHalfW)
		drawTriangleCCW(rl.NewVector2(cx, cy), tip, leftBase, woodAccent)
		drawTriangleCCW(rl.NewVector2(cx, cy), rightBase, tip, woodAccent)
	}
	// Centre disc + bright pip.
	rl.DrawCircleV(rl.NewVector2(cx, cy), innerR, woodAccent)
	rl.DrawCircleV(rl.NewVector2(cx, cy), innerR*0.5, giltBright)
	// "N" letter just above the north point.
	nLetter := "N"
	nm := rl.MeasureTextEx(font, nLetter, FontTiny, 1)
	drawTextWithShadow(font, nLetter, cx-nm.X/2, cy-outerR-nm.Y-2, FontTiny, inkAccent)
}

// panelsMapZoomDefaultFallback is the safety value when GameState was
// built by a struct literal (tests) that didn't seed PanelsMapZoom.
const panelsMapZoomDefaultFallback = 14

// visitedAt reports whether the player has stepped on this tile. Helper
// so the map-panel markers don't open-code the bounds check at every
// call.
func visitedAt(g core.GameState, x, z int) bool {
	if g.Visited == nil || z < 0 || z >= len(g.Visited) {
		return false
	}
	if x < 0 || x >= len(g.Visited[z]) {
		return false
	}
	return g.Visited[z][x]
}

// drawPanelsMapArrow paints the player marker on the Map tab — a small
// spear-shaped triangle pointing in the player's current facing. Sized
// to a fraction of cellPx so the arrow scales with the zoom level.
// Uses drawFacingArrow (shared with the minimap) with a 1.0 : 0.7
// forward-to-sideways ratio so the silhouette reads as a directional
// spearhead rather than the equilateral compass the minimap draws.
func drawPanelsMapArrow(cx, cy, cellPx float32, facing int) {
	r := cellPx * 0.45
	if r < 5 {
		r = 5
	}
	drawFacingArrow(rl.NewVector2(cx, cy), r, r*0.7, facing, playerArrowColor)
}

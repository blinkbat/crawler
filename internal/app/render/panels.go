package render

import (
	"crawler/internal/app/core"
	"fmt"
	"strconv"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// selectedGlassTint is the cursored / active glass wash used across the HUD:
// a blend from a `base` glass color toward glassWarm by `t`. Most panel
// surfaces wash from glassMid; the member-card-active and level-up surfaces
// wash from the deeper glassDeep/surfacePrimary. Centralized so every
// "highlighted glass surface" shares one warm target instead of open-coding
// MixColor(…, glassWarm, …) and drifting apart.
func selectedGlassTint(base rl.Color, t float64) rl.Color {
	return core.MixColor(base, glassWarm, t)
}

// panelStatMeasureCache memoizes rl.MeasureTextEx for the Stats-tab's
// right-aligned value strings (stat values, ARM, XP, status chip). These
// change only on level-up / HP-spend / status change — not 60 Hz — but
// drawPanelsStats was re-shaping ~8 strings per member every frame the
// Stats tab is open (~32 cgo calls for a 4-member party). Keyed by
// the tab mixes FontBody and FontSmall values (the shared measureCache
// keys on size, so both coexist).
var panelStatMeasureCache measureCache

func measurePanelStatValue(font rl.Font, text string, size float32) rl.Vector2 {
	return panelStatMeasureCache.measure(font, text, size, 1)
}

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

// panelTabDrawers dispatches by tab index to the per-tab body drawer.
// Indexed array (not a map) so the literal length-checks at compile
// time against PanelTabCount — adding a 6th tab is a single new
// PanelTab const + a new drawer + adding its slot here; forgetting
// one is a compile error, not a silent no-op like the old switch.
var panelTabDrawers = [core.PanelTabCount]func(core.GameState, Resources, rl.Rectangle){
	core.PanelTabStats:     drawPanelsStats,
	core.PanelTabEquipment: drawPanelsEquipment,
	core.PanelTabItems:     drawPanelsItems,
	core.PanelTabSkills:    drawPanelsSkills,
	core.PanelTabQuests:    drawPanelsQuests,
	core.PanelTabMap:       drawPanelsMap,
}

// panelTabFooterHints is the control-hint strip shown along the bottom of
// the overlay, per active tab. Parallel to panelTabDrawers and sized
// [PanelTabCount], so adding a tab forces a hint slot (compile error if
// missed) instead of silently inheriting a generic hint via a switch's
// fall-through. init asserts none is empty.
var panelTabFooterHints = [core.PanelTabCount]string{
	core.PanelTabStats:     "L1/R1 tabs   Left/Right pick member   X close",
	core.PanelTabEquipment: "L1/R1 tabs   Left/Right pick member   X close",
	core.PanelTabItems:     "L1/R1 tabs   Up/Down item   Confirm / F use   X close",
	core.PanelTabSkills:    "L1/R1 tabs   Left/Right member   Confirm open trees   F cast heal   X close",
	core.PanelTabQuests:    "L1/R1 tabs   Left/Right pick member   X close",
	core.PanelTabMap:       "L1/R1 tabs   Left/Right pick member   X close",
}

func init() {
	for t := core.PanelTab(0); t < core.PanelTabCount; t++ {
		if panelTabFooterHints[t] == "" {
			panic(fmt.Sprintf("render/panels: panelTabFooterHints missing a hint for tab %d", int(t)))
		}
	}
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
	// strip IS the heading. We reuse the veil + centered card, sized
	// screen-relative so the character menus fill most of the display and
	// each per-member column has room for body-size text.
	card := drawScreenFractionScaffold(font, panelsOverlayWidthFrac, panelsOverlayHeightFrac, "")
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
			bg = selectedGlassTint(glassMid, 0.65)
			txt = textPrimary
		}
		drawGlassPane(tx, tabRowY, tabW, tabH, bg)
		if active {
			// Gilt underline strip at the bottom of the active tab —
			// the same "you're here" mark the list rows use, scaled
			// to the tab width.
			drawGiltRule(tx+8, tabRowY+tabH-3, tabW-16, 2, 1.0)
		}
		label := core.PanelTabLabel(t)
		m := measureTabLabel(font, label)
		drawTextWithShadow(font, label,
			float32(tx)+float32(tabW)/2-m.X/2,
			float32(tabRowY)+float32(tabH)/2-m.Y/2-1,
			FontBody, txt)
	}

	// Persistent info strip, shown on EVERY tab: current location (area
	// name) on the left, party gold on the right. Drawn here in the shared
	// chrome (not per-tab) so it's always visible regardless of the active
	// tab, and the body is pushed below it.
	const panelsInfoStripH = int32(22)
	infoY := tabRowY + tabH + 4
	areaName := g.Area.Name
	if areaName == "" {
		areaName = "Unknown"
	}
	drawTextWithShadow(font, areaName, float32(cardX+24), float32(infoY), FontSmall, textPrimary)
	goldStr := fmt.Sprintf("Gold: %d", g.Gold)
	gm := rl.MeasureTextEx(font, goldStr, FontSmall, 1)
	drawTextWithShadow(font, goldStr, float32(cardX+cardW-24)-gm.X, float32(infoY), FontSmall, borderActive)

	bodyY := infoY + panelsInfoStripH + 6
	bodyRect := rl.NewRectangle(float32(cardX+22), float32(bodyY),
		float32(cardW-44), float32(cardY+cardH-26-bodyY-overlayFooterReserve))

	if int(g.PanelsTab) >= 0 && int(g.PanelsTab) < len(panelTabDrawers) {
		panelTabDrawers[g.PanelsTab](g, assets, bodyRect)
	}

	footerHint := panelTabFooterHints[core.PanelTabStats]
	if int(g.PanelsTab) >= 0 && int(g.PanelsTab) < len(panelTabFooterHints) {
		footerHint = panelTabFooterHints[g.PanelsTab]
	}
	drawModalFooter(font, card, footerHint)

	// Sub-modals painted on top of the whole overlay (frame + footer) so
	// they read as "above" everything.
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

// tabLabelMeasureCache memoizes panel-tab label measurements (fixed core
// registry, fills once and stays warm).
var tabLabelMeasureCache measureCache

func measureTabLabel(font rl.Font, label string) rl.Vector2 {
	return tabLabelMeasureCache.measure(font, label, FontBody, 1)
}

// memberCardGutter is the single per-member-card layout unit: both the
// gap between member columns and the content inset inside each card.
// Centralized so the column layout, the card-content inset, and the card
// header can't drift apart (they previously hardcoded 14 / 28 / 24).
const memberCardGutter = float32(14)

// memberColumnLayout returns the per-member column rectangles for any
// tab that paints one card per party member (Stats / Equipment /
// Skills). Equal-width columns with a small gap so the grid reads as
// "ledger of party members" rather than a single dense slab.
func memberColumnLayout(body rl.Rectangle, count int) []rl.Rectangle {
	if count <= 0 {
		return nil
	}
	total := body.Width - memberCardGutter*float32(count-1)
	colW := total / float32(count)
	cols := make([]rl.Rectangle, count)
	for i := 0; i < count; i++ {
		cols[i] = rl.NewRectangle(body.X+float32(i)*(colW+memberCardGutter), body.Y, colW, body.Height)
	}
	return cols
}

// memberCardInner returns the inner content inset (X origin + width) for
// a per-member card column — the writable region inside the gutter the
// Stats / Equipment / Skills body drawers paint into. Single seam for the
// gutter the three used to hardcode as `col.X + 14` / `colW - 28`.
func memberCardInner(col rl.Rectangle) (innerX, innerW float32) {
	return col.X + memberCardGutter, col.Width - 2*memberCardGutter
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
	// rounded panel + a class stripe gives enough separation. The
	// SELECTED member gets a much stronger treatment (warm wash + bold
	// gilt frame below) — the old faint tint read as "nothing selected"
	// across the Character / Skills / Equipment tabs.
	cardBG := glassMid
	if highlight {
		cardBG = selectedGlassTint(glassMid, 0.9)
	}
	drawGlassPane(int32(col.X), int32(col.Y), int32(col.Width), int32(col.Height), cardBG)
	// Class accent stripe, flush to the pane's left edge (not the inset
	// drawAccentStripe layout, which sits a few px in from a full card's
	// border). Shares stripeWidth so the bar weight tracks the rest of
	// the accent stripes even though the inset differs.
	rl.DrawRectangle(int32(col.X), int32(col.Y)+6, stripeWidth, int32(col.Height)-12, classCol)
	if highlight {
		// Bold gilt frame around the active card — the same "you're
		// here" gilt the tab underline / armed-skill spine use, scaled
		// up to an unmistakable full-card border. Rounded to match the
		// glass body's own corner radius so it hugs the pane.
		rect := rl.NewRectangle(col.X, col.Y, col.Width, col.Height)
		roundness := fixedRoundnessFor(int32(col.Width), int32(col.Height), cornerRadius)
		rl.DrawRectangleRoundedLinesEx(rect, roundness, 8, 3, giltBright)
	}

	innerX, innerW := memberCardInner(col)

	y := col.Y + 16
	nameCol := textPrimary
	if !highlight {
		nameCol = textMuted
	}
	// Class sigil flanks the name — same iconography as the party
	// ribbon's card, but at a larger radius so it reads as a banner
	// crest in this fuller pane.
	glyphR := float32(12)
	glyphCX := innerX + glyphR
	glyphCY := y + FontHeading/2
	drawClassGlyph(glyphCX, glyphCY, glyphR, m.Class, classCol)
	nameOffset := glyphR*2 + 12
	drawTextWithShadow(font, m.Name, innerX+nameOffset, y, FontHeading, nameCol)
	y += 36

	// PartyMember.Name doubles as the class label in this build, so the
	// sub-line just carries the level — no need to repeat the class.
	sub := "Lv " + strconv.Itoa(m.Level)
	drawTextWithShadow(font, sub, innerX, y, FontBody, textMuted)
	y += 30

	hpFill := hpFillColor(m.HP, m.MaxHP)
	drawBar(font, innerX, y, innerW, 28, "HP", m.HP, m.MaxHP, hpFill, m.HP <= 0)
	y += 36
	drawBar(font, innerX, y, innerW, 28, "MP", m.MP, m.MaxMP, barMP, m.HP <= 0)
	y += 42

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
	cols := memberColumnLayout(body, len(g.Party))
	for i, m := range g.Party {
		highlight := i == g.PanelsRowCursor
		contentY := drawPartyMemberCardHeader(font, m, cols[i], highlight)
		innerX, innerW := memberCardInner(cols[i])

		// Stat grid: 2 columns, ceil(StatCount/2) rows. Each cell paints
		// "[icon] LBL  value" with the icon in soft gilt to the left of
		// the label, the label muted, and the value bright so the eye
		// scans the numbers without losing the sigil row anchor.
		statColW := innerW / 2
		rowH := float32(30)
		statRows := (core.StatCount + 1) / 2
		statIconCol := fadeColor(woodAccent, 0.9)
		for s := core.Stat(0); s < core.StatCount; s++ {
			row := int(s) / 2
			col := int(s) % 2
			cellX := innerX + float32(col)*statColW
			cellY := contentY + float32(row)*rowH
			label := core.StatLabel(s)
			value := strconv.Itoa(core.StatValue(m.Stats, s))
			drawStatIcon(s, cellX+9, cellY+13, 9, statIconCol)
			drawTextWithShadow(font, label, cellX+24, cellY, FontBody, textMuted)
			// Value right-aligned within the cell so the column
			// of numbers lines up no matter the label width.
			vm := measurePanelStatValue(font, value, FontBody)
			drawTextWithShadow(font, value, cellX+statColW-vm.X-14, cellY, FontBody, textPrimary)
		}
		contentY += float32(statRows) * rowH

		// Armor + XP secondary row, slightly muted so they
		// don't compete with the stat grid above.
		contentY += 8
		drawTextWithShadow(font, "ARM", innerX, contentY, FontSmall, textMuted)
		armVal := strconv.Itoa(m.Armor)
		am := measurePanelStatValue(font, armVal, FontSmall)
		drawTextWithShadow(font, armVal, innerX+statColW-am.X-14, contentY, FontSmall, textPrimary)

		nextXP := core.XPForLevel(m.Level)
		xpText := strconv.Itoa(m.XP) + " / " + strconv.Itoa(nextXP)
		drawTextWithShadow(font, "XP", innerX+statColW, contentY, FontSmall, textMuted)
		xm := measurePanelStatValue(font, xpText, FontSmall)
		drawTextWithShadow(font, xpText, innerX+innerW-xm.X, contentY, FontSmall, textPrimary)
		contentY += 28

		// Status chip — bright pill in the per-status accent
		// color so afflicted members read at a glance.
		if kind, turns := core.PartyStatus(m); kind != core.PartyStatusNone {
			label := partyStatusTurnLabel(kind, turns)
			lm := measurePanelStatValue(font, label, FontSmall)
			chipW := lm.X + 20
			chipH := float32(26)
			chipX := innerX
			col, _ := partyStatusVisual(kind)
			drawSmallPanel(int32(chipX), int32(contentY), int32(chipW), int32(chipH), fadeColor(col, 0.28))
			drawSmallPanelOutline(int32(chipX), int32(contentY), int32(chipW), int32(chipH), fadeColor(col, 0.85))
			drawTextWithShadow(font, label, chipX+10, contentY+4, FontSmall, col)
			contentY += chipH + 8
		}

		// Allocate hint: only on the cursored member, only when
		// there's something to spend. Painted near the bottom of
		// the card so it reads as a call-to-action footer.
		if highlight && (m.PendingLevelUps > 0 || m.SkillPoints > 0) {
			hintY := cols[i].Y + cols[i].Height - 60
			if m.PendingLevelUps > 0 {
				hint := "Z   allocate " + strconv.Itoa(m.PendingLevelUps) + " stat pt" + plural(m.PendingLevelUps)
				drawTextWithShadow(font, hint, innerX, hintY, FontSmall, inkAccent)
				hintY += 24
			}
			if m.SkillPoints > 0 {
				hint := strconv.Itoa(m.SkillPoints) + " skill pt" + plural(m.SkillPoints) + "  (Skills tab)"
				drawTextWithShadow(font, hint, innerX, hintY, FontSmall, inkAccent)
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

// equipSlotRowHeight is the per-slot row height inside a member's
// Equipment-tab card (R.HAND / L.HAND / ARMOR / ACC1 / ACC2 stacked).
// The tab works like the Items menu — navigate slots, Confirm to open
// the item picker — so there's no inventory strip or drag ghost to size
// anymore; just the slot rows.
const equipSlotRowHeight = float32(56)

// slotIconForType returns the icon-draw function for an EquipSlotIndex
// — the per-slot row variant.
func slotIconForType(slot core.EquipSlotIndex) func(cx, cy, r float32, col rl.Color) {
	return slotIconForKind(core.SlotIndexType(slot))
}

// slotIconForKind returns the icon-draw function for an
// EquipmentSlotType — the picker-row / slot-type icon variant.
// One mapping, one place; the historical sword/shield/ring icons
// stay if the slot type set ever expands (e.g. SlotConsumable).
func slotIconForKind(t core.EquipmentSlotType) func(cx, cy, r float32, col rl.Color) {
	switch t {
	case core.SlotHand:
		return drawSlotIconSword
	case core.SlotArmor:
		return drawSlotIconShield
	case core.SlotAccessory:
		return drawSlotIconRing
	}
	return drawSlotIconRing
}

// equipPanelLayout caches the hit-test rectangles laid down each frame
// by the Equipment tab so the input layer can route a mouse click
// without re-running the layout math. SlotRects is flattened
// [member][slot] in row-major order (SlotMember / SlotIdx parallel it).
// PickerRects holds the slot-picker sub-modal's row rects (parallel to
// core.EquipPickerRows' order); PickerBounds is the whole picker card,
// used to detect a click-outside dismiss, and PickerValid gates that so
// a click can't dismiss a picker that wasn't drawn this frame.
type equipPanelLayout struct {
	SlotRects    []rl.Rectangle
	SlotMember   []int
	SlotIdx      []core.EquipSlotIndex
	PickerRects  []rl.Rectangle
	PickerBounds rl.Rectangle
	PickerValid  bool
}

// lastEquipLayout is the most recently drawn Equipment-tab layout. Read
// by the input layer in the same frame; render writes it AFTER drawing
// so the hit rects match what was painted. Single-threaded renderer +
// input means no synchronisation needed.
var lastEquipLayout equipPanelLayout

// ResetEquipPanelLayout zeroes the cached hit rects. Called from the
// input layer on overlay close / tab switch so the first frame after a
// transition can't route a click against stale geometry.
func ResetEquipPanelLayout() { lastEquipLayout = equipPanelLayout{} }

// EquipPanelSlotHit returns (member, slot, true) if `pt` is inside a
// slot rect, else (-1, 0, false). The Equipment tab opens that slot's
// item picker on a click.
func EquipPanelSlotHit(pt rl.Vector2) (int, core.EquipSlotIndex, bool) {
	for i, r := range lastEquipLayout.SlotRects {
		if rl.CheckCollisionPointRec(pt, r) {
			return lastEquipLayout.SlotMember[i], lastEquipLayout.SlotIdx[i], true
		}
	}
	return -1, 0, false
}

// EquipPanelPickerRowHit returns (rowIndex, true) if `pt` is inside a
// slot-picker row rect, else (-1, false). The index lines up with
// core.EquipPickerRows so the input layer acts on the clicked row.
func EquipPanelPickerRowHit(pt rl.Vector2) (int, bool) {
	for i, r := range lastEquipLayout.PickerRects {
		if rl.CheckCollisionPointRec(pt, r) {
			return i, true
		}
	}
	return -1, false
}

// EquipPanelClickOutsidePicker reports whether `pt` falls outside the
// open picker card — the signal to dismiss the sub-modal on a stray
// click. False when no picker was drawn this frame.
func EquipPanelClickOutsidePicker(pt rl.Vector2) bool {
	return lastEquipLayout.PickerValid && !rl.CheckCollisionPointRec(pt, lastEquipLayout.PickerBounds)
}

func drawPanelsEquipment(g core.GameState, assets Resources, body rl.Rectangle) {
	font := assets.Font()
	lastEquipLayout = equipPanelLayout{} // reset every frame
	if len(g.Party) == 0 {
		return
	}

	// One card per member, each listing its five equip slots as rows.
	// No inventory strip — choosing gear happens in the slot picker
	// sub-modal (drawEquipPicker), opened by Confirm / a click on a slot.
	cols := memberColumnLayout(body, len(g.Party))
	slotRowH := equipSlotRowHeight
	totalSlots := len(g.Party) * int(core.EquipSlotCount)
	lastEquipLayout.SlotRects = make([]rl.Rectangle, 0, totalSlots)
	lastEquipLayout.SlotMember = make([]int, 0, totalSlots)
	lastEquipLayout.SlotIdx = make([]core.EquipSlotIndex, 0, totalSlots)

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

			// The focused slot (cursored member + slot row, picker
			// closed) takes the shared focusable-row treatment so the
			// player sees which slot Confirm will open the picker for.
			focused := memberHL && int(s) == g.EquipSlotCursor && !g.EquipPickerOpen
			drawFocusableRow(slotRect, focused)

			equippedKind := m.Equipped[s]
			filled := equippedKind != core.ItemNone
			iconCol := fadeColor(woodAccent, 0.7)
			if filled {
				iconCol = giltBright
			}
			slotIconForType(s)(float32(innerX)+16, rowY+26, 11, iconCol)

			labelX := float32(innerX) + 40
			// The focused slot's label brightens so the cursor's location
			// reads at a glance against the column of muted slot names.
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
			// The FOCUSED slot expands to show its item's bonus — contextual
			// detail for the row you're on, so the player can read what's
			// equipped without opening the picker, and without cluttering all
			// 20 rows with bonus text in the narrow per-member columns.
			if focused && filled {
				if bonus := equipBonusSummary(core.ItemInfo(equippedKind)); bonus != "" {
					drawTextWithShadow(font, bonus, labelX, rowY+42, FontSmall, inkAccent)
				}
			}
		}
	}

	footer := "Up/Down slot   Left/Right member   Confirm / click: change gear"
	drawTextWithShadow(font, footer, body.X, body.Y+body.Height-18, FontSmall, textHint)
}

// drawEquipPicker paints the slot's item-picker sub-modal: a smaller
// card centered on screen, drawn ON TOP of the panels overlay, listing
// the inventory items eligible for the focused slot plus an "Unequip"
// row when the slot is filled. Mirrors the Items-menu feel — one row
// per option, the cursored row gilded. The row rects + card bounds are
// cached on lastEquipLayout so a click can pick a row (or dismiss when
// it lands outside the card).
func drawEquipPicker(g core.GameState, assets Resources) {
	font := assets.Font()
	member := g.PanelsRowCursor
	if member < 0 || member >= len(g.Party) {
		return
	}
	slot := core.EquipSlotIndex(g.EquipSlotCursor)
	rows := core.EquipPickerRows(&g, member, slot)

	const rowH = float32(46)
	const headerH = float32(70)
	const footerH = float32(34)
	visibleRows := len(rows)
	if visibleRows < 1 {
		visibleRows = 1 // reserve a line for the "no eligible items" note
	}
	_, sh := screenSizeF()
	cardW := float32(440)
	cardH := headerH + float32(visibleRows)*rowH + footerH
	if maxH := sh * 0.78; cardH > maxH {
		cardH = maxH
	}

	// Veil the overlay behind + draw the centered card via the shared
	// modal scaffold (same veil tone + corner filigree as the title /
	// pause / door modals), then lay the picker out in the returned rect.
	card := drawVeiledCard(int32(cardW), int32(cardH), borderActive, woodAccent, woodAccent)

	title := core.SlotIndexLabel(slot) + " — " + g.Party[member].Name
	drawTextWithShadow(font, title, card.X+18, card.Y+14, FontHeading, textPrimary)
	curKind := g.Party[member].Equipped[slot]
	curText := "Equipped: —"
	if curKind != core.ItemNone {
		curText = "Equipped: " + core.ItemInfo(curKind).Name
	}
	drawTextWithShadow(font, curText, card.X+18, card.Y+46, FontSmall, textMuted)

	lastEquipLayout.PickerRects = make([]rl.Rectangle, 0, len(rows))
	lastEquipLayout.PickerBounds = card
	lastEquipLayout.PickerValid = true

	if len(rows) == 0 {
		drawTextWithShadow(font, "No eligible items in inventory.", card.X+18, card.Y+headerH+8, FontBody, textHint)
	}
	listY := card.Y + headerH
	for i, row := range rows {
		ry := listY + float32(i)*rowH
		rect := rl.NewRectangle(card.X+10, ry, card.Width-20, rowH-6)
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

	hint := "Confirm: equip   Back: cancel"
	drawTextWithShadow(font, hint, card.X+18, card.Y+card.Height-26, FontSmall, textHint)
}

// drawUseTargetPicker paints the shared ally-target sub-modal for the
// out-of-battle "use" actions (a consumable from the Items tab, a heal
// skill from the Skills tab). A small veiled card lists the living party
// members with their HP; the focused row is the recipient Confirm will
// apply to. Title names what's being used. Keyboard/controller-driven
// (UseTargetCursor) — no mouse hit rects, since nothing here was asked
// to be clickable and the overlay stays controller-first.
func drawUseTargetPicker(g core.GameState, assets Resources) {
	font := assets.Font()
	living := core.LivingPartyIndices(g.Party)

	title := "Use"
	switch {
	case g.UsePendingItem != core.ItemNone:
		title = "Use " + core.ItemInfo(g.UsePendingItem).Name
	case g.UsePendingSkill != core.SkillNone:
		title = "Cast " + core.SkillName(g.UsePendingSkill)
	}

	const rowH = float32(44)
	const headerH = float32(56)
	const footerH = float32(32)
	visibleRows := len(living)
	if visibleRows < 1 {
		visibleRows = 1
	}
	cardW := float32(380)
	cardH := headerH + float32(visibleRows)*rowH + footerH
	card := drawVeiledCard(int32(cardW), int32(cardH), borderActive, woodAccent, woodAccent)

	drawTextWithShadow(font, title, card.X+18, card.Y+16, FontHeading, textPrimary)
	if len(living) == 0 {
		drawTextWithShadow(font, "No one can be healed.", card.X+18, card.Y+headerH, FontBody, textHint)
	}
	listY := card.Y + headerH
	for i, mi := range living {
		ry := listY + float32(i)*rowH
		rect := rl.NewRectangle(card.X+10, ry, card.Width-20, rowH-6)
		drawFocusableRow(rect, i == g.UseTargetCursor)
		m := g.Party[mi]
		classCol := partyClassPresentationFor(m.Class).turnColor
		drawClassGlyph(rect.X+20, rect.Y+rect.Height/2, 9, m.Class, classCol)
		drawTextWithShadow(font, m.Name, rect.X+40, rect.Y+rect.Height/2-10, FontBody, textPrimary)
		hp := "HP " + formatBarValue(m.HP, m.MaxHP)
		hm := rl.MeasureTextEx(font, hp, FontSmall, 1)
		drawTextWithShadow(font, hp, rect.X+rect.Width-hm.X-12, rect.Y+rect.Height/2-8, FontSmall, hpFillColor(m.HP, m.MaxHP))
	}

	hint := "Confirm: use   Back: cancel"
	drawTextWithShadow(font, hint, card.X+18, card.Y+card.Height-26, FontSmall, textHint)
}

// drawHealPicker paints the out-of-battle heal-skill chooser — a small veiled
// card listing the caster's out-of-battle heals (e.g. the Cleric's Prayer +
// Mass Mend) with their MP cost, the cursored row gilded. Raised only when a
// member has more than one such heal (HealPickOpen); a single heal casts
// directly without this step. Controller-driven (HealPickCursor).
func drawHealPicker(g core.GameState, assets Resources) {
	font := assets.Font()
	caster := g.HealPickCaster
	if caster < 0 || caster >= len(g.Party) {
		return
	}
	heals := core.OutOfBattleHeals(g.Party[caster])
	if len(heals) == 0 {
		return
	}

	const rowH = float32(44)
	const headerH = float32(56)
	const footerH = float32(32)
	cardW := float32(360)
	cardH := headerH + float32(len(heals))*rowH + footerH
	card := drawVeiledCard(int32(cardW), int32(cardH), borderActive, woodAccent, woodAccent)

	drawTextWithShadow(font, "Cast Heal — "+g.Party[caster].Name, card.X+18, card.Y+16, FontHeading, textPrimary)

	listY := card.Y + headerH
	for i, s := range heals {
		ry := listY + float32(i)*rowH
		rect := rl.NewRectangle(card.X+10, ry, card.Width-20, rowH-6)
		drawFocusableRow(rect, i == g.HealPickCursor)
		drawTextWithShadow(font, core.SkillName(s), rect.X+14, rect.Y+rect.Height/2-10, FontBody, textPrimary)
		costText := skillCostMPLabel(core.SkillCost(s))
		cm := rl.MeasureTextEx(font, costText, FontSmall, 1)
		drawTextWithShadow(font, costText, rect.X+rect.Width-cm.X-12, rect.Y+rect.Height/2-8, FontSmall, inkAccent)
	}

	hint := "Confirm: cast   Back: cancel"
	drawTextWithShadow(font, hint, card.X+18, card.Y+card.Height-26, FontSmall, textHint)
}

// equipBonusSummary returns the single-line "STR +2" / "Armor +1" /
// "MDef +2" copy painted under an item's tile. Compact, builds the
// shortest combination of bonuses authored on the def.
// equipBonusSummaryCache memoizes equipBonusSummary by item kind. The
// summary is built from the immutable ItemDefinition, so it's computed
// once per kind rather than rebuilding a []string + concatenated string
// for every visible picker row every frame the Equipment tab is open.
// The "" result for no-bonus items is cached too.
var equipBonusSummaryCache = map[core.ItemKind]string{}

func equipBonusSummary(def core.ItemDefinition) string {
	if s, ok := equipBonusSummaryCache[def.Kind]; ok {
		return s
	}
	parts := []string{}
	// Lead with the weapon's mechanical class so the player can see at a
	// glance which stat governs its to-hit/damage and whether it strikes at
	// range — derived from the registry (core.WeaponAccuracyStat /
	// WeaponIsRanged), never re-authored in item prose.
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
// panelsItemStacksBuf is the reused scratch slice for the Items tab's
// live-stack list — refilled each frame the tab is open instead of
// allocating a fresh slice.
var panelsItemStacksBuf []core.ItemStack

func drawPanelsItems(g core.GameState, assets Resources, body rl.Rectangle) {
	font := assets.Font()
	panelsItemStacksBuf = core.LiveStacksInto(g.Inventory, panelsItemStacksBuf)
	stacks := panelsItemStacksBuf
	if len(stacks) == 0 {
		drawTextWithShadow(font, "Your bags are empty.", body.X+14, body.Y+16, FontHeading, textMuted)
		drawTextWithShadow(font, "Loot from steals and chests will appear here.",
			body.X+14, body.Y+54, FontBody, textHint)
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
	rowH := float32(46)
	rowPad := float32(14)
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
			// Shared focused-row look (same as the Equipment / Skills
			// tabs in this overlay) so the panels tabs read consistently
			// instead of this list painting its own glass + gilt spine.
			drawFocusableRow(rl.NewRectangle(listRect.X, y, listRect.Width, rowH-4), true)
		}
		nameCol := textMuted
		if highlight {
			nameCol = textPrimary
		}
		drawTextWithShadow(font, info.Name, listRect.X+rowPad, y+12, FontBody, nameCol)
		// Count on the right edge of the row as a small chip.
		countText := "x" + strconv.Itoa(stack.Count)
		cm := rl.MeasureTextEx(font, countText, FontBody, 1)
		drawTextWithShadow(font, countText, listRect.X+listRect.Width-cm.X-rowPad, y+12, FontBody, inkAccent)
	}

	// Detail card: name, type/effect summary, count owned, description
	// stub. Reads as the ledger's "current entry" pane.
	drawGlassPane(int32(detailRect.X), int32(detailRect.Y), int32(detailRect.Width), int32(detailRect.Height), glassMid)
	if cursor < len(stacks) {
		stack := stacks[cursor]
		info := core.ItemInfo(stack.Kind)
		dy := detailRect.Y + 14
		dx := detailRect.X + 14
		drawTextWithShadow(font, info.Name, dx, dy, FontHeading, textPrimary)
		dy += 38
		effect := panelsItemHealLabel(info.HealAmount)
		drawTextWithShadow(font, effect, dx, dy, FontBody, inkAccent)
		dy += 30
		owned := "Owned: " + strconv.Itoa(stack.Count)
		drawTextWithShadow(font, owned, dx, dy, FontBody, textMuted)
		dy += 36
		// Description placeholder (item registry doesn't carry one
		// today). Wrap a short canned hint so the panel doesn't
		// feel empty.
		hint := "Consumable. Use from the battle menu's Item action."
		drawTextWithShadow(font, hint, dx, dy, FontSmall, textHint)
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
// right-aligned heal labels (bounded keys from the precomputed label LUT).
var panelsItemHealLabelMeasureCache measureCache

func measurePanelsItemHealLabel(font rl.Font, label string) rl.Vector2 {
	return panelsItemHealLabelMeasureCache.measure(font, label, FontSmall, 1)
}

// drawPanelsSkills renders the Skills tab as one card per party member,
// mirroring the Stats / Equipment layout. Each card is a SUMMARY of the
// member's three Diablo-2-style skill trees: the spendable SkillPoints
// balance up top, then one row per tree showing the tree name, an
// invested/total rank read, and its theme blurb. Confirm on the cursored
// member opens the full skill-tree modal (DrawSkillTreeModal) where points
// are actually spent — this tab just gives the at-a-glance overview.
func drawPanelsSkills(g core.GameState, assets Resources, body rl.Rectangle) {
	font := assets.Font()
	if len(g.Party) == 0 {
		return
	}
	cols := memberColumnLayout(body, len(g.Party))
	for i, m := range g.Party {
		highlight := i == g.PanelsRowCursor
		contentY := drawPartyMemberCardHeader(font, m, cols[i], highlight)
		innerX, innerW := memberCardInner(cols[i])

		// Skill-point balance — the currency the trees spend. Bright when
		// there's something to spend, muted at zero so a "nothing to do
		// here" card reads quietly.
		spText := strconv.Itoa(m.SkillPoints) + " SP"
		spCol := textMuted
		if m.SkillPoints > 0 {
			spCol = inkAccent
		}
		drawTextWithShadow(font, "SKILL POINTS", innerX, contentY, FontSmall, textMuted)
		sm := rl.MeasureTextEx(font, spText, FontSmall, 1)
		drawTextWithShadow(font, spText, innerX+innerW-sm.X, contentY, FontSmall, spCol)
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
			drawGlassPane(int32(rect.X), int32(rect.Y), int32(rect.Width), int32(rect.Height), glassMid)

			drawTextWithShadow(font, tr.Name, rect.X+12, rect.Y+8, FontBody, textPrimary)
			invested := core.TreeInvestedRanks(&m, tr)
			ratio := strconv.Itoa(invested) + " / " + strconv.Itoa(core.TreeMaxRanks(tr))
			rm := rl.MeasureTextEx(font, ratio, FontSmall, 1)
			ratioCol := textMuted
			if invested > 0 {
				ratioCol = giltBright
			}
			drawTextWithShadow(font, ratio, rect.X+rect.Width-rm.X-12, rect.Y+10, FontSmall, ratioCol)
			drawTextWithShadow(font, tr.Theme, rect.X+12, rect.Y+34, FontSmall, textHint)
		}

		// Call-to-action on the cursored member: Confirm opens the trees.
		if highlight {
			hintY := cols[i].Y + cols[i].Height - 30
			drawTextWithShadow(font, "Confirm: open skill trees", innerX, hintY, FontSmall, inkAccent)
		}
	}
}

// drawSkillTierPips paints `total` diamond pips left-to-right at
// (x, y) — the first `filled` in bright gilt, the rest as dim hollows —
// giving the Skills tab a compact "2 of 3 upgrades bought" read for a
// skill's investment.
func drawSkillTierPips(x, y float32, filled, total int) {
	const pipR = float32(5)
	const pipGap = float32(16)
	for i := 0; i < total; i++ {
		cx := x + pipR + float32(i)*pipGap
		col := fadeColor(giltBright, 0.22)
		if i < filled {
			col = giltBright
		}
		drawDiamondPip(cx, y, pipR, col)
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
		zoom = core.PanelMapZoomDefault
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
			// Derive each cell's pixel rect from the difference of
			// consecutive truncated edges (this cell's left, the next
			// cell's left) rather than truncating origin AND size
			// independently. With a fractional cellPx the old
			// `pw := int32(cellPx)` left 1px seams that opened and
			// closed across the grid as the fraction accumulated; tiling
			// edge-to-edge guarantees neighbours abut with no gap or
			// overlap, so the grid always reads as a solid sheet.
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
			// Unvisited / out-of-bounds tiles render flat fog; revealed
			// tiles show their material color. Shared fog rule with the
			// corner minimap via mapCellFillColor so the two map surfaces
			// can't drift on what lifts the fog.
			rl.DrawRectangle(px, py, pw, ph, mapCellFillColor(m, g, mx, mz))
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
	drawTextWithShadow(font, footer, body.X, body.Y+body.Height-20, FontSmall, textHint)
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
		{ax: 0, ay: -1, px: 1, py: 0},  // N
		{ax: 1, ay: 0, px: 0, py: 1},   // E
		{ax: 0, ay: 1, px: -1, py: 0},  // S
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

// mapCellFillColor is the shared fog-of-war fill rule for both the corner
// minimap and the panels Map tab: a tile reveals its material color only
// once it's in bounds AND the player has stepped within reveal range of
// it; otherwise it paints flat fog (the same fill as out-of-bounds).
// Single source so the two map surfaces can't drift on what lifts the fog.
func mapCellFillColor(m core.AreaDefinition, g core.GameState, x, z int) rl.Color {
	if m.InBounds(x, z) && visitedAt(g, x, z) {
		return minimapTileColor(m.Materials, m.TileAt(x, z))
	}
	return mapTileFogColor
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

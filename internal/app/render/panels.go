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
		drawSmallPanel(tx, tabRowY, tabW, tabH, bg)
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
	drawSmallPanel(int32(col.X), int32(col.Y), int32(col.Width), int32(col.Height), cardBG)
	rl.DrawRectangle(int32(col.X), int32(col.Y)+6, 3, int32(col.Height)-12, classCol)

	innerX := col.X + 14
	innerW := col.Width - 24

	y := col.Y + 12
	nameCol := textPrimary
	if !highlight {
		nameCol = textMuted
	}
	drawTextWithShadow(font, m.Name, innerX, y, FontBody, nameCol)
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

		// Stat grid: 3 rows × 2 columns. Each cell paints
		// "LBL  value" with the label muted and value bright so
		// the eye scans the numbers, not the labels.
		statColW := innerW / 2
		rowH := float32(22)
		for s := core.Stat(0); s < core.StatCount; s++ {
			row := int(s) / 2
			col := int(s) % 2
			cellX := innerX + float32(col)*statColW
			cellY := contentY + float32(row)*rowH
			label := core.StatLabel(s)
			value := strconv.Itoa(core.StatValue(m.Stats, s))
			drawTextWithShadow(font, label, cellX, cellY, FontSmall, textMuted)
			// Value right-aligned within the cell so the column
			// of numbers lines up no matter the label width.
			vm := rl.MeasureTextEx(font, value, FontSmall, 1)
			drawTextWithShadow(font, value, cellX+statColW-vm.X-12, cellY, FontSmall, textPrimary)
		}
		contentY += 3 * rowH

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
			drawSmallPanel(int32(innerX), int32(rowY), int32(innerW), int32(slotRowH-8), fadeColor(glassDeep, 0.55))

			drawTextWithShadow(font, label, innerX+8, rowY+4, FontTiny, textMuted)
			value := "—"
			valCol := textDim
			if slotIdx == 1 && m.Armor > 0 {
				value = "Armor +" + strconv.Itoa(m.Armor)
				valCol = textPrimary
			}
			drawTextWithShadow(font, value, innerX+8, rowY+18, FontSmall, valCol)
		}
	}
	footer := "Equipment system pending — values shown reflect the base armor stat only."
	drawTextWithShadow(font, footer, body.X, body.Y+body.Height-16, FontTiny, textHint)
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
			drawSmallPanel(int32(listRect.X), int32(y), int32(listRect.Width), int32(rowH-4),
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
	drawSmallPanel(int32(detailRect.X), int32(detailRect.Y), int32(detailRect.Width), int32(detailRect.Height), glassMid)
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
			drawSmallPanel(int32(innerX), int32(rowY), int32(innerW), int32(rowH-8), rowBG)
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

	// Map footer — area name + zoom indicator.
	footer := panelsMapFooterText(g.Area.Name, zoom)
	drawTextWithShadow(font, footer, body.X, body.Y+body.Height-18, FontTiny, textHint)
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

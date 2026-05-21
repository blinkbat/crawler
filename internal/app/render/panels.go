package render

import (
	"crawler/internal/app/core"
	"fmt"

	rl "github.com/gen2brain/raylib-go/raylib"
)

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
	// Panels overlay skips drawModalScaffold's heading band because
	// the tab strip below takes that visual slot — the active tab's
	// label IS the heading.
	card := drawModalScaffold(font, overlayCardWidthHuge, overlayCardHeightLarge, "")
	cardX, cardY := int32(card.X), int32(card.Y)
	cardW, cardH := int32(card.Width), int32(card.Height)

	// Tab strip across the top — selected tab highlighted, the rest
	// rendered as dimmer pills. Width per tab is fixed so re-ordering
	// the PanelTab enum doesn't shift the layout.
	tabH := overlayTabHeight
	tabRowY := cardY + 12
	tabPad := overlayTabPadding
	tabW := (cardW - 24 - tabPad*int32(core.PanelTabCount-1)) / int32(core.PanelTabCount)
	for t := core.PanelTab(0); t < core.PanelTabCount; t++ {
		tx := cardX + 12 + int32(t)*(tabW+tabPad)
		active := t == g.PanelsTab
		bg := surfacePrimary
		border := borderSoft
		txt := textMuted
		if active {
			bg = core.MixColor(surfacePrimary, surfaceActiveTint, 0.55)
			border = borderActive
			txt = textPrimary
		}
		drawCard(tx, tabRowY, tabW, tabH, bg, border, border)
		label := core.PanelTabLabel(t)
		m := rl.MeasureTextEx(font, label, 16, 1)
		drawTextWithShadow(font, label,
			float32(tx)+float32(tabW)/2-m.X/2,
			float32(tabRowY)+float32(tabH)/2-m.Y/2,
			16, txt)
	}

	bodyY := tabRowY + tabH + 12
	bodyRect := rl.NewRectangle(float32(cardX+18), float32(bodyY),
		float32(cardW-36), float32(cardY+cardH-22-bodyY-overlayFooterReserve))

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

	DrawFooterHint(font, "L1/R1 tabs   Up/Down cursor   Esc / Big-Start close",
		float32(cardX+cardW/2), float32(cardY+cardH-22), 13)
}

// drawPanelsStats renders the Stats tab — four party columns with
// level, HP/MP, all six stats, armor, XP. Identical content to
// DrawPartyStatsScreen but laid out inside the panels body rect.
func drawPanelsStats(g core.GameState, assets Resources, body rl.Rectangle) {
	font := assets.Font()
	if len(g.Party) == 0 {
		return
	}
	colW := body.Width / float32(len(g.Party))
	for i, m := range g.Party {
		colX := body.X + float32(i)*colW
		y := body.Y
		highlight := i == g.PanelsRowCursor
		nameCol := textPrimary
		if !highlight {
			nameCol = textMuted
		}
		drawTextWithShadow(font, m.Name, colX, y, 20, nameCol)
		y += 26
		drawTextWithShadow(font, fmt.Sprintf("Lv %d", m.Level), colX, y, 14, textMuted)
		y += 18
		drawTextWithShadow(font, fmt.Sprintf("HP %d/%d", m.HP, m.MaxHP), colX, y, 14, textMuted)
		y += 16
		drawTextWithShadow(font, fmt.Sprintf("MP %d/%d", m.MP, m.MaxMP), colX, y, 14, textMuted)
		y += 22
		for s := core.Stat(0); s < core.StatCount; s++ {
			line := fmt.Sprintf("%s %d", core.StatLabel(s), core.StatValue(m.Stats, s))
			drawTextWithShadow(font, line, colX, y, 13, textMuted)
			y += 14
		}
		y += 6
		drawTextWithShadow(font, fmt.Sprintf("ARM %d", m.Armor), colX, y, 13, textMuted)
		y += 18
		nextXP := core.XPForLevel(m.Level)
		drawTextWithShadow(font, fmt.Sprintf("XP %d/%d", m.XP, nextXP), colX, y, 13, textHint)
		// Status badges (Poisoned / Sleeping / Ingested / Defending /
		// Down) so the panel surfaces what each member is dealing with
		// at a glance — without the player having to peek at the
		// battle ribbon.
		y += 18
		if badge := memberStatusBadge(m); badge != "" {
			drawTextWithShadow(font, badge, colX, y, 13, textPrimary)
		}
	}
}

// memberStatusBadge returns the short status string surfaced under each
// member on the Stats panel. Priority matches the party-card status
// labels in render/party.go so the two surfaces stay consistent.
func memberStatusBadge(m core.PartyMember) string {
	switch {
	case m.HP <= 0:
		return "DOWN"
	case m.Ingested:
		return "INGESTED"
	case m.PoisonTurns > 0:
		return fmt.Sprintf("POISONED (%d)", m.PoisonTurns)
	case m.SleepTurns > 0:
		return fmt.Sprintf("ASLEEP (%d)", m.SleepTurns)
	case m.Defending:
		return "DEFENDING"
	}
	return ""
}

// drawPanelsEquipment renders the Equipment tab. There's no equipment
// system in core yet — this is a stub showing each member's current
// armor (the only "equipped" value) plus placeholder rows for the
// future Weapon / Armor / Accessory slots so the UI is in place when
// equipment lands.
func drawPanelsEquipment(g core.GameState, assets Resources, body rl.Rectangle) {
	font := assets.Font()
	if len(g.Party) == 0 {
		return
	}
	colW := body.Width / float32(len(g.Party))
	slotLabels := []string{"Weapon", "Armor", "Accessory"}
	for i, m := range g.Party {
		colX := body.X + float32(i)*colW
		y := body.Y
		highlight := i == g.PanelsRowCursor
		nameCol := textPrimary
		if !highlight {
			nameCol = textMuted
		}
		drawTextWithShadow(font, m.Name, colX, y, 20, nameCol)
		y += 28
		for slotIdx, label := range slotLabels {
			value := "(empty)"
			if slotIdx == 1 && m.Armor > 0 {
				value = fmt.Sprintf("Armor +%d", m.Armor)
			}
			drawTextWithShadow(font, label, colX, y, 14, textMuted)
			y += 16
			drawTextWithShadow(font, value, colX+10, y, 13, textHint)
			y += 22
		}
	}
	footer := "Equipment items not yet authored — Armor reflects the per-member base value."
	drawTextWithShadow(font, footer, body.X, body.Y+body.Height-18, 12, textHint)
}

// drawPanelsItems renders the Items tab — a read-only listing of every
// stack in the shared inventory. Cursor highlight follows g.PanelsRowCursor
// (set by the up/down handler). Empty inventory falls through to a
// "your bags are empty" note so the panel never reads as broken.
func drawPanelsItems(g core.GameState, assets Resources, body rl.Rectangle) {
	font := assets.Font()
	stacks := core.LiveStacks(g.Inventory)
	if len(stacks) == 0 {
		drawTextWithShadow(font, "Your bags are empty.", body.X, body.Y, 16, textMuted)
		drawTextWithShadow(font, "Loot dropped from steals and chests will appear here.",
			body.X, body.Y+24, 13, textHint)
		return
	}
	rowH := float32(28)
	for i, stack := range stacks {
		y := body.Y + float32(i)*rowH
		if y+rowH > body.Y+body.Height {
			break
		}
		info := core.ItemInfo(stack.Kind)
		highlight := i == g.PanelsRowCursor
		if highlight {
			DrawSelectedRow(rl.NewRectangle(body.X-6, y-2, body.Width+12, rowH-2))
		}
		col := textMuted
		if highlight {
			col = textPrimary
		}
		left := fmt.Sprintf("%s ×%d", info.Name, stack.Count)
		drawTextWithShadow(font, left, body.X, y+4, 16, col)
		right := fmt.Sprintf("+%d HP", info.HealAmount)
		m := rl.MeasureTextEx(font, right, 14, 1)
		drawTextWithShadow(font, right, body.X+body.Width-m.X-4, y+6, 14, textHint)
	}
}

// drawPanelsSkills renders the Skills tab — each party member's class
// skill with its MP cost, kind, and a one-line description. Read-only;
// future "spend skill points" interaction lives in this tab when
// it exists.
func drawPanelsSkills(g core.GameState, assets Resources, body rl.Rectangle) {
	font := assets.Font()
	if len(g.Party) == 0 {
		return
	}
	rowH := float32(64)
	for i, m := range g.Party {
		y := body.Y + float32(i)*rowH
		if y+rowH > body.Y+body.Height {
			break
		}
		highlight := i == g.PanelsRowCursor
		if highlight {
			DrawSelectedRow(rl.NewRectangle(body.X-6, y-2, body.Width+12, rowH-4))
		}
		nameCol := textMuted
		if highlight {
			nameCol = textPrimary
		}
		skill := core.PartySkill(m)
		title := fmt.Sprintf("%s — %s", m.Name, core.SkillName(skill))
		drawTextWithShadow(font, title, body.X, y+4, 18, nameCol)
		cost := core.SkillCost(skill)
		costLine := fmt.Sprintf("MP %d", cost)
		drawTextWithShadow(font, costLine, body.X, y+28, 13, textHint)
		drawTextWithShadow(font, skillFlavor(skill), body.X+72, y+28, 13, textHint)
	}
}

// skillFlavor returns the one-line description shown under each skill
// row. Kept here (not on the skill registry) because it's pure UX text
// that may change without affecting gameplay numbers.
func skillFlavor(s core.SkillID) string {
	switch s {
	case core.SkillSwipe:
		return "STR-scaled cleave through every living enemy in the pack."
	case core.SkillPrayer:
		return "WIS-scaled single-ally heal. Charge bar — release at peak."
	case core.SkillSteal:
		return "Pickpocket the target. DEX + timing drive the chance."
	case core.SkillFirebolt:
		return "INT-scaled magic damage. Chance to inflict Burn."
	}
	return ""
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
				rl.DrawRectangle(px, py, pw, pw, rl.NewColor(6, 8, 14, 235))
				continue
			}
			visited := visitedAt(g, mx, mz)
			rl.DrawRectangle(px, py, pw, pw, tileColorWithFog(m.Materials, m.TileAt(mx, mz), visited))
		}
	}

	// Pack markers — only on explored tiles, so unseen rooms stay
	// mysterious. Same red dot as the corner minimap.
	for _, pack := range g.Packs {
		if !core.PackAlive(pack) {
			continue
		}
		if !visitedAt(g, pack.TileX, pack.TileZ) {
			continue
		}
		lx := pack.TileX - startX
		lz := pack.TileZ - startZ
		if lx < 0 || lz < 0 || lx >= cellsX || lz >= cellsY {
			continue
		}
		cx := int32(mapX + (float32(lx)+0.5)*cellPx)
		cy := int32(mapY + (float32(lz)+0.5)*cellPx)
		r := cellPx * 0.35
		if r < 3 {
			r = 3
		}
		rl.DrawCircle(cx, cy, r, mapPackMarkerColor)
		rl.DrawCircleLines(cx, cy, r+1, mapPackMarkerOutline)
	}

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
	footer := fmt.Sprintf("%s   zoom: %d cells   Up/Down to zoom", g.Area.Name, zoom)
	drawTextWithShadow(font, footer, body.X, body.Y+body.Height-18, 12, textHint)
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

package render

import (
	"crawler/internal/app/core"
	"fmt"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// DrawLevelUpModal paints the post-battle stat-spend dialog. Shows the
// current member's name, level, remaining points, and the six spendable
// stats with current values. Selected row is highlighted via
// DrawSelectedRow so the visual style matches every other in-game list
// modal. Rendered after the world / HUD so it sits on top.
func DrawLevelUpModal(g core.GameState, assets Resources) {
	if !g.LevelUpOpen {
		return
	}
	if g.LevelUpMember < 0 || g.LevelUpMember >= len(g.Party) {
		return
	}
	m := g.Party[g.LevelUpMember]

	font := assets.Font()
	screenW, screenH := screenSize()
	cardW := int32(420)
	cardH := int32(380)
	cardX := centerX(cardW)
	cardY := screenH/2 - cardH/2

	rl.DrawRectangle(0, 0, screenW, screenH, surfaceVeil)
	drawCard(cardX, cardY, cardW, cardH, surfacePrimary, borderSoft, borderActive)
	header := fmt.Sprintf("LEVEL UP — %s", m.Name)
	drawHeading(font, header, cardX+18, cardY+14, borderActive)

	// Sub-header: level + remaining points.
	sub := fmt.Sprintf("Level %d — %d points to spend", m.Level, m.PendingLevelUps)
	drawTextWithShadow(font, sub, float32(cardX+18), float32(cardY+44), 16, textMuted)

	// Stat rows.
	rowY := cardY + 80
	rowH := int32(36)
	rowX := cardX + 24
	rowW := cardW - 48
	for s := core.Stat(0); s < core.StatCount; s++ {
		focused := s == g.LevelUpStat
		rect := rl.NewRectangle(float32(rowX-6), float32(rowY-4), float32(rowW+12), float32(rowH))
		if focused {
			DrawSelectedRow(rect)
		}
		col := textMuted
		if focused {
			col = textPrimary
		}
		left := core.StatLabel(s)
		right := fmt.Sprintf("%d", core.StatValue(m.Stats, s))
		drawTextWithShadow(font, left, float32(rowX), float32(rowY), 20, col)
		rm := rl.MeasureTextEx(font, right, 20, 1)
		drawTextWithShadow(font, right, float32(rowX)+float32(rowW)-rm.X-6, float32(rowY), 20, col)
		rowY += rowH
	}

	// VIT spend note: callers should know levels of VIT immediately raise
	// MaxHP + heal the difference, so a fresh level-up isn't a slow grind
	// back to full.
	note := "VIT raises MaxHP and heals the difference."
	drawTextWithShadow(font, note, float32(cardX+18), float32(cardY+cardH-50), 12, textHint)
	DrawFooterHint(font, "Up/Down pick   Enter spend",
		float32(cardX+cardW/2), float32(cardY+cardH-22), 13)
}

// DrawPartyStatsScreen paints the pause-menu's Party Stats overlay —
// all four members side by side with Class / Level / HP / MP / Stats /
// Armor / XP / next-XP. Read-only; Esc closes (handled by explore).
func DrawPartyStatsScreen(g core.GameState, assets Resources) {
	if !g.StatsScreenOpen {
		return
	}
	font := assets.Font()
	screenW, screenH := screenSize()
	cardW := int32(680)
	cardH := int32(380)
	cardX := centerX(cardW)
	cardY := screenH/2 - cardH/2

	rl.DrawRectangle(0, 0, screenW, screenH, surfaceVeil)
	drawCard(cardX, cardY, cardW, cardH, surfacePrimary, borderSoft, borderActive)
	drawHeading(font, "PARTY STATS", cardX+18, cardY+14, borderActive)

	colW := (cardW - 36) / int32(len(g.Party))
	if colW < 1 {
		colW = 1
	}
	for i, m := range g.Party {
		colX := cardX + 18 + int32(i)*colW
		y := cardY + 50
		drawTextWithShadow(font, m.Name, float32(colX), float32(y), 18, textPrimary)
		y += 22
		drawTextWithShadow(font, fmt.Sprintf("Lv %d", m.Level), float32(colX), float32(y), 14, textMuted)
		y += 18
		drawTextWithShadow(font, fmt.Sprintf("HP %d/%d", m.HP, m.MaxHP), float32(colX), float32(y), 14, textMuted)
		y += 16
		drawTextWithShadow(font, fmt.Sprintf("MP %d/%d", m.MP, m.MaxMP), float32(colX), float32(y), 14, textMuted)
		y += 22
		for s := core.Stat(0); s < core.StatCount; s++ {
			line := fmt.Sprintf("%s %d", core.StatLabel(s), core.StatValue(m.Stats, s))
			drawTextWithShadow(font, line, float32(colX), float32(y), 13, textMuted)
			y += 14
		}
		y += 6
		drawTextWithShadow(font, fmt.Sprintf("ARM %d", m.Armor), float32(colX), float32(y), 13, textMuted)
		y += 18
		nextXP := core.XPForLevel(m.Level)
		drawTextWithShadow(font, fmt.Sprintf("XP %d/%d", m.XP, nextXP), float32(colX), float32(y), 13, textHint)
	}

	DrawFooterHint(font, "Esc close",
		float32(cardX+cardW/2), float32(cardY+cardH-22), 13)
}

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
	header := fmt.Sprintf("LEVEL UP — %s", m.Name)
	card := drawModalScaffold(font, overlayCardWidthMedium, overlayCardHeightSmall, header)
	cardX, cardY := int32(card.X), int32(card.Y)
	cardW, cardH := int32(card.Width), int32(card.Height)

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

// (DrawPartyStatsScreen was retired in favor of the panels overlay's
// Stats tab — DrawPanelsOverlay handles the multi-tab dashboard,
// including the same per-member Stats / Level / HP / MP / XP layout
// plus Equipment / Items / Skills / Map.)

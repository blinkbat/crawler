package render

import (
	"crawler/internal/app/core"
	"strconv"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// DrawLevelUpModal paints the post-battle stat-spend dialog. Each
// stat row shows label + description + the running "current → new"
// preview that reflects the player's staged pending picks. A skill-
// point row sits below the stats; a final Apply row commits the
// staged changes. Nothing actually lands on the member's stat block
// until Apply is confirmed.
func DrawLevelUpModal(g core.GameState, assets Resources) {
	if !g.LevelUpOpen {
		return
	}
	if g.LevelUpMember < 0 || g.LevelUpMember >= len(g.Party) {
		return
	}
	m := g.Party[g.LevelUpMember]

	font := assets.Font()
	header := "LEVEL UP — " + m.Name
	card := drawModalScaffold(font, overlayCardWidthLarge, overlayCardHeightLarge, header)
	cardX, cardY := int32(card.X), int32(card.Y)
	cardW, cardH := int32(card.Width), int32(card.Height)

	// Sub-header: a single bright readout of the stat-point budget,
	// with the skill-point reminder dimmed to a second line so the
	// primary action (spend stats) stays the loudest signal.
	staged := core.SumStatPending(g.LevelUpPending)
	statRemaining := m.PendingLevelUps - staged
	primary := "Stat points: " + strconv.Itoa(statRemaining) + " remaining"
	if staged > 0 {
		primary += "   ·   " + strconv.Itoa(staged) + " staged"
	}
	drawTextWithShadow(font, primary, float32(cardX+22), float32(cardY+46), FontBody, textPrimary)
	if m.SkillPoints > 0 {
		secondary := strconv.Itoa(m.SkillPoints) + " skill pt" + plural(m.SkillPoints) + " banked — spend in the Skills tab"
		drawTextWithShadow(font, secondary, float32(cardX+22), float32(cardY+72), FontTiny, inkAccent)
	}

	// Stat rows. Each row is taller (52px) so the label, description,
	// and preview don't collide at smaller widths. Layout per row:
	//   left: LABEL (FontBody)
	//   left + indent: description (FontTiny, dim)
	//   right: current → new (FontBody, bright when staged)
	rowY := cardY + 102
	rowH := int32(52)
	rowX := cardX + 24
	rowW := cardW - 48
	for s := core.Stat(0); s < core.StatCount; s++ {
		focused := g.LevelUpRowCursor == int(s)
		rect := rl.NewRectangle(float32(rowX-6), float32(rowY-4), float32(rowW+12), float32(rowH-6))
		if focused {
			DrawSelectedRow(rect)
		} else {
			drawGlassPane(int32(rect.X), int32(rect.Y), int32(rect.Width), int32(rect.Height), fadeColor(glassDeep, 0.45))
		}
		col := textMuted
		if focused {
			col = textPrimary
		}
		label := core.StatLabel(s)
		cur := core.StatValue(m.Stats, s)
		pending := g.LevelUpPending[s]

		// Stat sigil — small icon in the left gutter of the row.
		// Brightens on focus so the focused row reads as "lit"
		// without leaning entirely on the gilt selection chrome.
		iconCol := fadeColor(woodAccent, 0.85)
		if focused {
			iconCol = giltBright
		}
		drawStatIcon(s, float32(rowX)+14, float32(rowY)+18, 10, iconCol)
		drawTextWithShadow(font, label, float32(rowX+34), float32(rowY+2), FontBody, col)
		if desc := core.StatDescription(s); desc != "" {
			drawTextWithShadow(font, desc, float32(rowX+86), float32(rowY+26), FontTiny, textHint)
		}

		var preview string
		previewCol := col
		if pending > 0 {
			preview = strconv.Itoa(cur) + "  →  " + strconv.Itoa(cur+pending) + "   (+" + strconv.Itoa(pending) + ")"
			previewCol = inkAccent
		} else {
			preview = strconv.Itoa(cur)
		}
		rm := rl.MeasureTextEx(font, preview, FontBody, 1)
		drawTextWithShadow(font, preview, float32(rowX)+float32(rowW)-rm.X-10, float32(rowY+2), FontBody, previewCol)
		rowY += rowH
	}

	// Apply button row. Tinted warm to read as a commit action.
	{
		rowY += 6
		focused := g.LevelUpRowCursor == core.LevelUpApplyRowIndex
		rect := rl.NewRectangle(float32(rowX-6), float32(rowY-4), float32(rowW+12), float32(rowH-6))
		applyBG := core.MixColor(glassDeep, glassWarm, 0.45)
		drawGlassPane(int32(rect.X), int32(rect.Y), int32(rect.Width), int32(rect.Height), applyBG)
		if focused {
			DrawSelectedRow(rect)
		}
		col := textMuted
		if focused {
			col = textPrimary
		}
		label := "Apply changes"
		if statRemaining > 0 {
			label = "Apply changes — " + strconv.Itoa(statRemaining) + " unspent"
		}
		// Fleuron sits on each side of the Apply row label — the
		// "this is the commit gate" cue. Drawn in gilt so the
		// player's eye lands on it even when the row isn't
		// focused.
		labelW := rl.MeasureTextEx(font, label, FontBody, 1).X
		labelX := float32(rowX + 4)
		labelY := float32(rowY + 10)
		drawTextWithShadow(font, label, labelX, labelY, FontBody, col)
		flCY := labelY + FontBody/2
		drawFleuronsFlanking(labelX, labelW, 16, flCY, 4, giltDim)
	}

	// VIT note removed — the per-stat description column already
	// surfaces "Max HP (+2 per point)" on the VIT row itself.
	DrawFooterHint(font, "Up/Down pick   Z stage   X undo   Enter apply",
		float32(cardX+cardW/2), float32(cardY+cardH-22), FontTiny)
}

// levelUpStagedTotal retired — core.SumStatPending is the single seam
// shared with explore.updateLevelUpModal.

// (DrawPartyStatsScreen was retired in favor of the panels overlay's
// Stats tab — DrawPanelsOverlay handles the multi-tab dashboard,
// including the same per-member Stats / Level / HP / MP / XP layout
// plus Equipment / Items / Skills / Map.)

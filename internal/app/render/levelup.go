package render

import (
	"crawler/internal/app/core"
	"strconv"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Level-up modal layout — header positions and the per-stat row grid.
// Named (mirrors shop.go's shop* block) so a "rows too cramped" retune is
// one edit here instead of bare offsets scattered through the draw loop.
// Header/row offsets are relative to the card; column/baseline offsets are
// relative to each row's origin.
const (
	levelUpHeaderX    = int32(22)   // header text inset from card left
	levelUpHeaderY    = int32(46)   // primary readout baseline from card top
	levelUpHeaderSubY = int32(76)   // skill-point reminder baseline from card top
	levelUpRowTop     = int32(112)  // first stat row's top from card top
	levelUpRowH       = int32(64)   // per-stat row height
	levelUpRowX       = int32(24)   // row inset from card left
	levelUpRowMargin  = int32(48)   // total horizontal row margin (2× the inset)
	levelUpIconX      = float32(16) // stat sigil x from row left
	levelUpIconY      = float32(24) // stat sigil y from row top
	levelUpLabelX     = int32(44)   // label x from row left
	levelUpLabelY     = int32(6)    // label / value baseline y from row top
	levelUpSubX       = int32(96)   // sub-text x from row left
	levelUpSubY       = int32(36)   // sub-text baseline y from row top
	levelUpValueInset = float32(12) // right-aligned value inset from row right edge
)

// DrawLevelUpModal paints the post-battle stat-spend dialog. Each
// stat row shows label + description + the running "current → new"
// preview that reflects the player's staged pending picks. A skill-
// point row sits below the stats; a final Apply row commits the
// staged changes. Nothing actually lands on the member's stat block
// until Apply is confirmed.
func DrawLevelUpModal(g *core.GameState, assets Resources) {
	if !g.LevelUpOpen {
		return
	}
	if g.LevelUpMember < 0 || g.LevelUpMember >= len(g.Party) {
		return
	}
	m := g.Party[g.LevelUpMember]

	font := assets.Font()
	header := "LEVEL UP — " + m.Name
	// Screen-relative so the stat-allocation menu reads large like the
	// rest of the character menus (matches the panels overlay's sizing).
	card := drawScreenFractionScaffold(font, levelUpModalWidthFrac, levelUpModalHeightFrac, header)
	cardX, cardY := int32(card.X), int32(card.Y)
	cardW := int32(card.Width)

	// Sub-header: a single bright readout of the stat-point budget,
	// with the skill-point reminder dimmed to a second line so the
	// primary action (spend stats) stays the loudest signal.
	staged := core.SumStatPending(g.LevelUpPending)
	statRemaining := m.PendingLevelUps - staged
	primary := "Stat points: " + strconv.Itoa(statRemaining) + " remaining"
	if staged > 0 {
		primary += "   ·   " + strconv.Itoa(staged) + " staged"
	}
	drawTextWithShadow(font, primary, float32(cardX+levelUpHeaderX), float32(cardY+levelUpHeaderY), FontBody, textPrimary)
	if m.SkillPoints > 0 {
		secondary := strconv.Itoa(m.SkillPoints) + " skill pt" + plural(m.SkillPoints) + " banked — spend in the Skills tab"
		drawTextWithShadow(font, secondary, float32(cardX+levelUpHeaderX), float32(cardY+levelUpHeaderSubY), FontSmall, inkAccent)
	}

	// Stat rows. Each row is taller (64px) so the label, description,
	// and preview don't collide at smaller widths. Layout per row:
	//   left: LABEL (FontBody)
	//   left + indent: description (FontTiny, dim)
	//   right: current → new (FontBody, bright when staged)
	rowY := cardY + levelUpRowTop
	rowH := levelUpRowH
	rowX := cardX + levelUpRowX
	rowW := cardW - levelUpRowMargin
	for s := core.Stat(0); s < core.StatCount; s++ {
		focused := g.LevelUpRowCursor == int(s)
		rect := SelectionRowRect(rowX, rowY, rowW, rowH-6)
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
		drawStatIcon(s, float32(rowX)+levelUpIconX, float32(rowY)+levelUpIconY, 12, iconCol)
		drawEngravedText(font, label, float32(rowX+levelUpLabelX), float32(rowY+levelUpLabelY), FontHeading, col)
		// When the player has staged a spend on this row, swap the
		// static description for the computed before→after preview so
		// the row tells you what the point actually BUYS instead of
		// just what stat it touches. Falls through to the static
		// description when nothing is staged.
		subText := core.StatPreviewLine(s, m.Stats, pending, core.WeaponAccuracyStat(core.EquippedWeapon(m)))
		subCol := textHint
		if subText != "" {
			subCol = inkAccent
		} else {
			subText = core.StatDescription(s)
		}
		if subText != "" {
			drawTextWithShadow(font, subText, float32(rowX+levelUpSubX), float32(rowY+levelUpSubY), FontSmall, subCol)
		}

		var preview string
		previewCol := col
		if pending > 0 {
			preview = strconv.Itoa(cur) + "  →  " + strconv.Itoa(cur+pending) + "   (+" + strconv.Itoa(pending) + ")"
			previewCol = inkAccent
		} else {
			preview = strconv.Itoa(cur)
		}
		drawTextRightAligned(font, preview, float32(rowX)+float32(rowW)-levelUpValueInset, float32(rowY+levelUpLabelY), FontHeading, previewCol)
		rowY += rowH
	}

	// Apply button row. Tinted warm to read as a commit action.
	{
		rowY += 6
		focused := g.LevelUpRowCursor == core.LevelUpApplyRowIndex
		rect := SelectionRowRect(rowX, rowY, rowW, rowH-6)
		applyBG := selectedGlassTint(glassDeep, 0.45)
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
		labelW := rl.MeasureTextEx(font, label, FontHeading, FontSpacingHeading).X
		labelX := float32(rowX + 6)
		labelY := float32(rowY + 14)
		drawEngravedText(font, label, labelX, labelY, FontHeading, col)
		flCY := labelY + FontHeading/2
		drawFleuronsFlanking(labelX, labelW, 18, flCY, 5, giltDim)
	}

	// VIT note removed — the per-stat description column already
	// surfaces "Max HP (+2 per point)" on the VIT row itself.
	// Confirm both stages a stat point (on a stat row) and applies (on the
	// Apply row); Back undoes a staged point. Controller glyphs only.
	drawModalFooterGlyphs(font, card, []HintSeg{
		Hint("Pick", GlyphUpDown),
		Hint("Stage / Apply", GlyphA),
		Hint("Undo", GlyphB),
	})
}

// levelUpStagedTotal retired — core.SumStatPending is the single seam
// shared with explore.updateLevelUpModal.

// (DrawPartyStatsScreen was retired in favor of the panels overlay's
// Stats tab — DrawPanelsOverlay handles the multi-tab dashboard,
// including the same per-member Stats / Level / HP / MP / XP layout
// plus Equipment / Items / Skills / Map.)

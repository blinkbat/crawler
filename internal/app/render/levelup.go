package render

import (
	"crawler/internal/app/core"
	"strconv"
)

// Level-up modal layout. Header/row offsets are relative to the card; column/baseline offsets to each row's origin.
const (
	levelUpHeaderX    = hudContentInsetX // header text inset from card left (shared HUD content gutter)
	levelUpHeaderY    = int32(46)        // primary readout baseline from card top
	levelUpHeaderSubY = int32(76)        // skill-point reminder baseline from card top
	levelUpRowTop     = int32(112)       // first stat row's top from card top
	levelUpRowH       = int32(64)        // per-stat row height
	levelUpRowX       = int32(24)        // row inset from card left (row margin is 2× this)
	levelUpIconX      = float32(16)      // stat sigil x from row left
	levelUpIconY      = float32(24)      // stat sigil y from row top
	levelUpLabelX     = int32(44)        // label x from row left
	levelUpLabelY     = int32(6)         // label / value baseline y from row top
	levelUpSubX       = int32(96)        // sub-text x from row left
	levelUpSubY       = int32(36)        // sub-text baseline y from row top
	levelUpValueInset = float32(12)      // right-aligned value inset from row right edge
	// levelUpRowGlassAlpha: glass fill shared by unfocused stat rows and the Apply tint. Untyped to satisfy both fadeColor (float32) and selectedGlassTint (float64).
	levelUpRowGlassAlpha = 0.45
)

// DrawLevelUpModal paints the post-battle stat-spend dialog. Staged picks show a "current → new"
// preview per stat; nothing lands on the member's stats until the Apply row is confirmed.
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
	// Screen-relative so it reads large like the other character menus.
	card := drawScreenFractionScaffold(font, levelUpModalWidthFrac, levelUpModalHeightFrac, header)
	cardX, cardY := int32(card.X), int32(card.Y)
	cardW := int32(card.Width)

	// Sub-header: bright stat-point budget; skill-point reminder dimmed to a second line.
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

	// Stat rows. Per row: left LABEL, indented description, right "current → new".
	rowY := cardY + levelUpRowTop
	rowH := levelUpRowH
	rowX := cardX + levelUpRowX
	rowW := cardW - 2*levelUpRowX
	for s := core.Stat(0); s < core.StatCount; s++ {
		focused := g.LevelUpRowCursor == int(s)
		rect := SelectionRowRect(rowX, rowY, rowW, rowH-selectionPlateShrinkY)
		if focused {
			DrawSelectedRow(rect)
		} else {
			drawGlassPaneRect(rect, fadeColor(glassDeep, levelUpRowGlassAlpha))
		}
		col := rowTextColor(focused, false, textMuted)
		label := core.StatLabel(s)
		cur := core.StatValue(m.Stats, s)
		pending := g.LevelUpPending[s]

		// Stat sigil in the left gutter; brightens on focus.
		iconCol := woodAccentIcon
		if focused {
			iconCol = giltBright
		}
		drawStatIcon(s, float32(rowX)+levelUpIconX, float32(rowY)+levelUpIconY, 12, iconCol)
		drawEngravedText(font, label, float32(rowX+levelUpLabelX), float32(rowY+levelUpLabelY), FontHeading, col)
		// Staged: show the before→after preview (what the point buys); else the static description.
		subText := core.StatPreviewLine(s, m.Stats, pending, core.WeaponAccuracyStat(core.EquippedWeapon(m)))
		subCol := textDim
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

	// Apply button row.
	{
		rowY += 6
		focused := g.LevelUpRowCursor == core.LevelUpApplyRowIndex
		rect := SelectionRowRect(rowX, rowY, rowW, rowH-selectionPlateShrinkY)
		applyBG := selectedGlassTint(glassDeep, levelUpRowGlassAlpha)
		drawGlassPaneRect(rect, applyBG)
		if focused {
			DrawSelectedRow(rect)
		}
		col := rowTextColor(focused, false, textMuted)
		label := "Apply changes"
		if statRemaining > 0 {
			label = "Apply changes — " + strconv.Itoa(statRemaining) + " unspent"
		}
		// Fleurons flank the Apply label as the "commit gate" cue, gilt even when unfocused.
		labelW := levelUpApplyMeasureCache.measure(font, label, FontHeading, FontSpacingHeading).X
		labelX := float32(rowX + 6)
		labelY := float32(rowY + 14)
		drawEngravedText(font, label, labelX, labelY, FontHeading, col)
		flCY := labelY + FontHeading/2
		drawFleuronsFlanking(labelX, labelW, 18, flCY, 5, giltDim)
	}

	// A stages (stat row) or applies (Apply row); B undoes a staged point.
	drawModalFooterGlyphs(font, card, []HintSeg{
		Hint("Pick", GlyphUpDown),
		Hint("Stage / Apply", GlyphA),
		Hint("Undo", GlyphB),
	})
}

// levelUpApplyMeasureCache memoizes the Apply-row label width (the fleurons flank it every frame).
var levelUpApplyMeasureCache measureCache

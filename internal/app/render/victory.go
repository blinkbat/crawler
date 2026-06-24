package render

import (
	"strconv"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Victory spoils screen — post-battle results card. All animation is a pure
// function of g.Battle.VictoryElapsed (core timing helpers), so the renderer
// holds no state; updateVictorySpoils is the sole driver.

const (
	// Fixed WIDTH (victoryWidthFrac), content-sized HEIGHT (sums only rows drawn).
	victoryMemberRowH = float32(54)
	victoryNameRowH   = float32(30) // name line → XP bar vertical step
	victoryNameInsetX = float32(22) // name X past the class rail in the gutter
	victoryBarH       = float32(22)
	victoryContentPad = float32(modalContentInsetX) // shared modal side gutter (22)
	victoryHeaderH    = float32(70)                 // heading band → first member row
	victoryRuleGap    = float32(22)                 // member rows → loot rule + breath
	victoryLootRowH   = float32(28)                 // one loot / gold / summary line
	victoryFooterH    = float32(50)                 // footer-hint reserve at card bottom (bespoke height budget; the hint baseline lands via footerBaselineY/uiFooterMargin). Tuning this shifts the footer — leave at 50.

	// victoryMaxLootRows caps loot rows before folding the rest into a "+N more"
	// tail so a huge haul can't overrun the summary/footer.
	victoryMaxLootRows = 5
)

// DrawVictorySpoils paints the spoils card once the dance beat has played.
func DrawVictorySpoils(g *core.GameState, assets Resources) {
	b := &g.Battle
	if !b.Spoils.Active || b.Phase != core.BattleWon {
		return
	}
	// Let the victory pose read solo for the dance beat before the card cuts in.
	if b.VictoryElapsed <= core.VictoryDanceBeat {
		return
	}
	font := assets.Font()

	// Content-sized card: height sums exactly the rows drawn + header + footer.
	n := len(b.Spoils.Members)
	lootRows := len(b.Spoils.Drops)
	overflow := lootRows > victoryMaxLootRows
	if overflow {
		lootRows = victoryMaxLootRows
	}
	hasGold := b.Spoils.Gold > 0
	statPts, skillPts := spoilsRewardTotals(b)
	hasSummary := statPts > 0 || skillPts > 0

	ledgerRows := lootRows // item rows
	if overflow {
		ledgerRows++ // the "…+N more" tail
	}
	if ledgerRows == 0 && !hasGold {
		ledgerRows = 1 // the "No loot." line
	}
	if hasGold {
		ledgerRows++
	}

	contentH := victoryHeaderH + float32(n)*victoryMemberRowH + victoryRuleGap
	contentH += victoryLootRowH // the "SPOILS" label row
	contentH += float32(ledgerRows) * victoryLootRowH
	if hasSummary {
		contentH += victoryLootRowH
	}
	contentH += victoryFooterH

	sw, _ := screenSize()
	card := drawModalScaffold(font, int32(float32(sw)*victoryWidthFrac), int32(contentH), "VICTORY")
	contentX := card.X + victoryContentPad
	contentW := card.Width - victoryContentPad*2

	fill := core.VictoryFillProgress(b.VictoryElapsed)

	rowY := card.Y + victoryHeaderH
	for _, ms := range b.Spoils.Members {
		drawSpoilsMemberRow(g, font, ms, contentX, rowY, contentW, fill)
		rowY += victoryMemberRowH
	}

	// Loot ledger beneath the party; drawSpoilsLoot returns its end-Y.
	rowY += 4
	drawGiltRule(int32(contentX), int32(rowY), int32(contentW), 2, 0.7)
	rowY += 18
	rowY = drawSpoilsLoot(b, font, contentX, rowY, contentW, fill)

	// Level-up rewards summary, revealed once the bars settle.
	if hasSummary && core.VictorySpoilsAnimDone(b.VictoryElapsed) {
		msg := "Earned +" + strconv.Itoa(statPts) + " stat · +" + strconv.Itoa(skillPts) + " skill — spend in the Tome"
		drawTextCentered(font, msg, card.X+card.Width/2, rowY+6, FontSmall, inkAccent)
	}

	// Footer: Skip while filling, Continue once settled.
	verb := "Skip"
	if core.VictorySpoilsAnimDone(b.VictoryElapsed) {
		verb = "Continue"
	}
	drawModalFooterGlyphs(font, card, []HintSeg{Hint(verb, GlyphA)})
}

// spoilsRewardTotals sums banked stat + skill points (gained levels × per-level
// grants). Mirrors AddXP's payout so the advertised number matches what landed.
func spoilsRewardTotals(b *core.Battle) (stat, skill int) {
	for _, ms := range b.Spoils.Members {
		if levels := ms.AfterLvl - ms.BeforeLvl; levels > 0 {
			stat += levels * core.LevelStatPoints
			skill += levels * core.LevelSkillPoints
		}
	}
	return stat, skill
}

// drawSpoilsMemberRow paints one member's name line + animated XP bar; a pulsing
// "LEVEL UP!" flag rides the name line on a gained level. Dead members render muted.
func drawSpoilsMemberRow(g *core.GameState, font rl.Font, ms core.MemberSpoils, x, y, w, fill float32) {
	if ms.Slot < 0 || ms.Slot >= len(g.Party) {
		return
	}
	m := g.Party[ms.Slot]
	dead := m.HP <= 0

	addedF := ms.AddedAt(fill)
	lvl, xp, _ := ms.ProjectAt(fill)
	need := core.XPForLevel(lvl)
	// Carry addedF's fractional part onto integer XP so the bar glides rather
	// than stepping per whole XP; readout text still shows whole numbers.
	barFrac := (float32(xp) + (addedF - float32(int(addedF)))) / float32(need)

	// Class-colored identity rail in the gutter; name muted when down.
	accent := classAccent(m.Class)
	nameCol := textPrimary
	if dead {
		accent = barMutedFill
		nameCol = textMuted
	}
	drawClassRail(int32(x), int32(y)+2, stripeWidth, int32(victoryNameRowH+victoryBarH-2), accent)
	drawTextWithShadow(font, m.Name, x+victoryNameInsetX, y, FontBody, nameCol)

	if lvl > ms.BeforeLvl {
		// Pulsing gilt "LEVEL UP! Lv before → now" with a leading fleuron, right-anchored.
		flag := fadeColor(giltBright, 0.72+0.28*pulse(2.4))
		badge := "LEVEL UP!  Lv " + strconv.Itoa(ms.BeforeLvl) + " → " + strconv.Itoa(lvl)
		bw := measureRichText(font, badge, FontBody, canonicalSpacing(FontBody)).X
		drawTextRightAligned(font, badge, x+w, y, FontBody, flag)
		drawFleuron(x+w-bw-16, y+FontBody/2, 4, flag)
	}

	drawBarFraction(font, x, y+victoryNameRowH, w, victoryBarH, "Lv "+strconv.Itoa(lvl), formatBarValue(xp, need), barFrac, xpGainColor, dead)
}

// drawSpoilsLoot paints the gold tally + cascading item rows, returning the Y
// past the last row. Each row advances by victoryLootRowH (matches content-sizing).
func drawSpoilsLoot(b *core.Battle, font rl.Font, x, y, w, fill float32) float32 {
	drawTextWithShadow(font, "SPOILS", x, y, FontSmall, inkAccent)
	y += victoryLootRowH

	if b.Spoils.Gold > 0 {
		shown := int(float32(b.Spoils.Gold) * fill)
		drawTextWithShadow(font, goldGainLabel(shown), x, y, FontBody, coinFace)
		y += victoryLootRowH
	}

	if len(b.Spoils.Drops) == 0 {
		if b.Spoils.Gold == 0 {
			drawTextWithShadow(font, "No loot.", x, y, FontSmall, textMuted)
			y += victoryLootRowH
		}
		return y
	}

	revealed := core.VictoryLootRevealed(b.VictoryElapsed, len(b.Spoils.Drops))
	for i, stack := range b.Spoils.Drops {
		if i >= revealed {
			break
		}
		// Overflow folds into a "+N more" tail (items already banked in inventory).
		if i >= victoryMaxLootRows {
			drawTextWithShadow(font, "…+"+strconv.Itoa(len(b.Spoils.Drops)-victoryMaxLootRows)+" more", x, y, FontSmall, textMuted)
			y += victoryLootRowH
			break
		}
		// Per-row slide+fade: row i opens at danceBeat + i·stagger.
		rowStart := core.VictoryDanceBeat + float32(i)*core.VictoryLootStagger
		p := (b.VictoryElapsed - rowStart) / core.VictoryLootFadeDuration
		if p < 0 {
			p = 0
		} else if p > 1 {
			p = 1
		}
		ease := core.Smoothstep(p)
		label := core.ItemInfo(stack.Kind).Name
		if stack.Count > 1 {
			label += "  ×" + strconv.Itoa(stack.Count)
		}
		drawTextWithShadow(font, label, x+(1-ease)*16, y, FontSmall, fadeColor(textPrimary, ease))
		y += victoryLootRowH
	}
	return y
}

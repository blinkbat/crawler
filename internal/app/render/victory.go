package render

import (
	"strconv"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Victory spoils screen — the post-battle results card (FF / SMRPG style):
// each member's XP bar fills from its pre-battle value across any level
// thresholds the fight crossed, gold ticks up, and loot rows cascade in.
// All animation is a pure function of g.Battle.VictoryElapsed via the core
// timing helpers (core.VictoryFillProgress / VictoryLootRevealed), so the
// renderer holds no state and the battle update loop (updateVictorySpoils)
// stays the single driver — it advances the clock, rings the cues, and exits
// on Confirm. Drawn on top of the dimmed battle scene; see hud.go.

const (
	// The card is a modest fixed WIDTH (victoryWidthFrac, in theme.go) but a
	// content-sized HEIGHT, so a light haul doesn't float in a big near-empty
	// panel (the height sums only the rows actually drawn). See DrawVictorySpoils.
	victoryMemberRowH = float32(54)
	victoryNameRowH   = float32(30) // name line → XP bar vertical step
	victoryNameInsetX = float32(22) // name X past the class rail in the gutter
	victoryBarH       = float32(22)
	victoryContentPad = float32(22)
	victoryHeaderH    = float32(70) // heading band → first member row
	victoryRuleGap    = float32(22) // member rows → loot rule + breath
	victoryLootRowH   = float32(28) // one loot / gold / summary line
	victoryFooterH    = float32(50) // reserve for the footer hint at card bottom

	// victoryMaxLootRows caps how many loot rows the card lists before
	// folding the rest into a "+N more" tail, so a huge haul can't overrun
	// the summary/footer (realistic drops are 0–3, this is a safety net).
	victoryMaxLootRows = 5
)

// DrawVictorySpoils paints the spoils card once the victory pose (dance
// beat) has played. No-op outside the won-battle results window.
func DrawVictorySpoils(g *core.GameState, assets Resources) {
	b := &g.Battle
	if !b.Spoils.Active || b.Phase != core.BattleWon {
		return
	}
	// Let the party's victory pose read solo for the dance beat before the
	// card cuts in over the scene.
	if b.VictoryElapsed <= core.VictoryDanceBeat {
		return
	}
	font := assets.Font()

	// Content-sized card: width is a modest fraction, height sums exactly the
	// rows we'll draw (members + the loot ledger + an optional rewards line)
	// plus the header band and footer reserve — so the panel hugs its content.
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

	// Loot ledger beneath the party. A thin gilt rule separates it from the
	// XP rows, then the gold tally (ticking up with the fill) and the item
	// rows that cascade in one by one. drawSpoilsLoot returns its end-Y so the
	// rewards line flows directly beneath it.
	rowY += 4
	drawGiltRule(int32(contentX), int32(rowY), int32(contentW), 2, 0.7)
	rowY += 18
	rowY = drawSpoilsLoot(b, font, contentX, rowY, contentW, fill)

	// Level-up rewards summary, revealed once the bars settle — tells the
	// player what's now banked for the Tome (reinforcing the party-card "+"
	// badge they'll see back in the dungeon).
	if hasSummary && core.VictorySpoilsAnimDone(b.VictoryElapsed) {
		msg := "Earned +" + strconv.Itoa(statPts) + " stat · +" + strconv.Itoa(skillPts) + " skill — spend in the Tome"
		drawTextCentered(font, msg, card.X+card.Width/2, rowY+6, FontSmall, inkAccent)
	}

	// Footer: invite the skip while filling, then the dismiss once settled.
	verb := "Skip"
	if core.VictorySpoilsAnimDone(b.VictoryElapsed) {
		verb = "Continue"
	}
	drawModalFooterGlyphs(font, card, []HintSeg{Hint(verb, GlyphA)})
}

// spoilsRewardTotals sums the stat + skill points the party banked this
// battle (gained levels × the per-level grants), for the rewards summary
// line. Mirrors AddXP's payout so the number the screen advertises matches
// what actually landed on the members.
func spoilsRewardTotals(b *core.Battle) (stat, skill int) {
	for _, ms := range b.Spoils.Members {
		if levels := ms.AfterLvl - ms.BeforeLvl; levels > 0 {
			stat += levels * core.LevelStatPoints
			skill += levels * core.LevelSkillPoints
		}
	}
	return stat, skill
}

// drawSpoilsMemberRow paints one party member's name line + animated XP bar.
// The bar fills from the member's pre-battle XP toward its post-battle total,
// the bar's "Lv N" label ticks up live as a threshold is crossed, and a
// pulsing "LEVEL UP!" flag rides the name line for any member who gained a
// level by the current fill point. Dead members render muted with no gain.
func drawSpoilsMemberRow(g *core.GameState, font rl.Font, ms core.MemberSpoils, x, y, w, fill float32) {
	if ms.Slot < 0 || ms.Slot >= len(g.Party) {
		return
	}
	m := g.Party[ms.Slot]
	dead := m.HP <= 0

	addedF := ms.AddedAt(fill)
	lvl, xp, _ := ms.ProjectAt(fill)
	need := core.XPForLevel(lvl)
	// Continuous fill fraction: carry the sub-unit remainder (addedF's
	// fractional part) onto the integer XP so the bar GLIDES rather than
	// stepping one pixel per whole XP — the readout text still shows whole
	// numbers.
	barFrac := (float32(xp) + (addedF - float32(int(addedF)))) / float32(need)

	// Class-colored identity rail in the gutter — the SAME embellished 3-D
	// rail (drawClassRail) the turn-order rows, panels columns, and card
	// accents use, so each spoils row reads as "whose" at a glance and matches
	// the rest of the UI. The name stays high-contrast cream (muted when down).
	accent := classAccent(m.Class)
	nameCol := textPrimary
	if dead {
		accent = barMutedFill
		nameCol = textMuted
	}
	drawClassRail(int32(x), int32(y)+2, stripeWidth, int32(victoryNameRowH+victoryBarH-2), accent)
	drawTextWithShadow(font, m.Name, x+victoryNameInsetX, y, FontBody, nameCol)

	if lvl > ms.BeforeLvl {
		// The dopamine beat: a pulsing gilt "LEVEL UP!" with the actual climb
		// (Lv before → now, the → drawn by the procedural symbol layer) and a
		// fleuron sigil leading it — right-anchored on the name line.
		flag := fadeColor(giltBright, 0.72+0.28*pulse(2.4))
		badge := "LEVEL UP!  Lv " + strconv.Itoa(ms.BeforeLvl) + " → " + strconv.Itoa(lvl)
		bw := measureRichText(font, badge, FontBody, canonicalSpacing(FontBody)).X
		drawTextRightAligned(font, badge, x+w, y, FontBody, flag)
		drawFleuron(x+w-bw-16, y+FontBody/2, 4, flag)
	}

	drawBarFraction(font, x, y+victoryNameRowH, w, victoryBarH, "Lv "+strconv.Itoa(lvl), formatBarValue(xp, need), barFrac, xpGainColor, dead)
}

// drawSpoilsLoot paints the gold tally and the cascading item rows, returning
// the Y just past the last row so the caller can flow the rewards line under
// it. Gold counts up with the same fill fraction as the XP bars; item rows
// reveal one per VictoryLootStagger and fade+slide in as they appear. Every
// row advances by victoryLootRowH so the card's content-sizing math matches.
func drawSpoilsLoot(b *core.Battle, font rl.Font, x, y, w, fill float32) float32 {
	drawTextWithShadow(font, "SPOILS", x, y, FontSmall, inkAccent)
	y += victoryLootRowH

	if b.Spoils.Gold > 0 {
		shown := int(float32(b.Spoils.Gold) * fill)
		drawTextWithShadow(font, "Gold  +"+strconv.Itoa(shown), x, y, FontBody, coinFace)
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
		// Cap the rows so a pathological haul can't run the list off the card
		// and into the summary/footer — the overflow folds into a "+N more"
		// tail (the items themselves are already banked in the inventory).
		if i >= victoryMaxLootRows {
			drawTextWithShadow(font, "…+"+strconv.Itoa(len(b.Spoils.Drops)-victoryMaxLootRows)+" more", x, y, FontSmall, textMuted)
			y += victoryLootRowH
			break
		}
		// Per-row slide+fade: row i opens at danceBeat + i·stagger and eases
		// in over a short window, so the list cascades rather than snapping.
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

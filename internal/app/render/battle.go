package render

import (
	"crawler/internal/app/core"
	"fmt"
	"image/color"
	"math"
	"strconv"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// drawBattleHUD orchestrates the in-combat HUD. Each panel owns one screen
// region so they never overlap. The action log stays pinned through the timing
// minigame; the action menu yields to the bar.
func drawBattleHUD(g *core.GameState, assets Resources) {
	drawEnemyRoster(g, assets)
	drawActionLogPanel(g, assets)
	if !timingActive(g) {
		drawActionMenuPanel(g, assets)
	}
}

// timingActive reports whether the timed-hit bar is the HUD focus (hides panels sharing its strip).
func timingActive(g *core.GameState) bool {
	return g.Battle.Phase == core.BattleAttackTiming || g.Battle.Phase == core.BattleEnemyTiming
}

// inPlayerTurn reports whether the player is acting (menu/picker or timing bar) — kept true
// through the bar so active-actor/target indicators persist instead of flickering.
func inPlayerTurn(g *core.GameState) bool {
	return g.Battle.Phase == core.BattlePlayer || g.Battle.Phase == core.BattleAttackTiming
}

// targetingEnemy reports the "choose an enemy" phase (BattlePlayer + ActionEnemyTarget).
// Single source for the yellow-cursor gates (in-world chevron + roster highlight).
func targetingEnemy(g *core.GameState) bool {
	return g.Battle.Phase == core.BattlePlayer && g.Battle.ActionMode == core.ActionEnemyTarget
}

// targetingAlly is true when cursoring a party member (heal/item target, or Swap partner).
// Gates the friendly selection marker across all three modes.
func targetingAlly(g *core.GameState) bool {
	return g.Battle.ActionMode == core.ActionPartyTarget ||
		g.Battle.ActionMode == core.ActionItemTarget ||
		g.Battle.ActionMode == core.ActionSwapTarget
}

type enemyStatusKind int

const (
	enemyStatusBurn enemyStatusKind = iota
	enemyStatusSleep
	enemyStatusPoison
	enemyStatusBleed
	enemyStatusStun
	enemyStatusCount
)

type enemyStatusPillVisual struct {
	turns   func(*core.Enemy) int
	fill    rl.Color
	outline rl.Color
	// glyph reuses the party cards' drawStatusGlyph* so both surfaces share icons.
	glyph   func(cx, cy, r float32, col rl.Color)
	flicker bool
}

var enemyStatusPillVisuals = [enemyStatusCount]enemyStatusPillVisual{
	// Burn/Bleed are enemy-only (local fill+glyph). Sleep/Poison/Stun draw fill+glyph from
	// sharedStatusVisuals (party.go). flicker is NOT shared (static on pills, pulses on cards);
	// outline + turns reader are pill-only.
	enemyStatusBurn:   {turns: func(e *core.Enemy) int { return e.BurnTurns }, fill: statusBurn, outline: statusBurnOutline, glyph: drawStatusGlyphBurn, flicker: true},
	enemyStatusSleep:  {turns: func(e *core.Enemy) int { return e.SleepTurns }, fill: sharedStatusVisuals[core.PartyStatusAsleep].Col, outline: statusSleepOutline, glyph: sharedStatusVisuals[core.PartyStatusAsleep].Glyph},
	enemyStatusPoison: {turns: func(e *core.Enemy) int { return e.PoisonTurns }, fill: sharedStatusVisuals[core.PartyStatusPoisoned].Col, outline: statusPoisonOutline, glyph: sharedStatusVisuals[core.PartyStatusPoisoned].Glyph, flicker: true},
	enemyStatusBleed:  {turns: func(e *core.Enemy) int { return e.BleedTurns }, fill: statusBleed, outline: statusBleedOutline, glyph: drawStatusGlyphBleed, flicker: true},
	enemyStatusStun:   {turns: func(e *core.Enemy) int { return e.StunTurns }, fill: sharedStatusVisuals[core.PartyStatusStunned].Col, outline: statusStunOutline, glyph: sharedStatusVisuals[core.PartyStatusStunned].Glyph},
}

// assertTableComplete panics if isMissing reports a gap in [0, count). Shared init-time
// coverage check for the package's parallel tables — a new enum value with no row trips it.
func assertTableComplete(name string, count int, isMissing func(i int) bool) {
	for i := 0; i < count; i++ {
		if isMissing(i) {
			panic(fmt.Sprintf("render: %s has no entry for index %d — add the row", name, i))
		}
	}
}

func init() {
	assertTableComplete("enemyStatusPillVisuals", int(enemyStatusCount), func(i int) bool {
		v := enemyStatusPillVisuals[i]
		return v.turns == nil || v.glyph == nil
	})
}

// drawEnemyRoster shows the active pack at the top of the screen.
func drawEnemyRoster(g *core.GameState, assets Resources) {
	if g.Battle.Phase == core.BattleWon || g.Battle.Phase == core.BattleLost {
		return
	}
	slots := visibleRosterSlots(g)
	if len(slots) == 0 {
		return
	}

	rowH := rosterRowH
	topPad := rosterTopPad
	padBottom := rosterBottomPad
	w := rosterW
	if len(slots) <= 1 {
		w = rosterWSingle
	}
	h := topPad + int32(len(slots))*rowH + padBottom
	x := centerX(w)
	y := hudEdgePad + hudEdgePad/2

	drawCard(x, y, w, h, surfacePrimary, borderSoft, borderEnemy)

	// targetable gates the per-row yellow highlight (shares targetingEnemy with the chevron).
	targetable := targetingEnemy(g)
	members := core.BattleMembers(g)
	selectedSlot := core.SelectedEnemySlot(g)
	// While picking a melee target, out-of-reach foes (back row, front still up) grey out;
	// the cursor can land on them but confirming buzzes. Ranged/magic reaches everyone.
	meleeTargeting := targetable && core.BattlePendingAttackIsMelee(g)

	for i, slot := range slots {
		enemy := &members[slot] // pointer: avoid copying the 496-byte Enemy per row per frame
		rowY := y + topPad + int32(i)*rowH
		// HP shows once the kind is identified (5 kills or a Scan) — a kind-level fact.
		known := g.Bestiary.Knows(enemy.Kind)
		reachable := !meleeTargeting || core.EnemyInEffectiveFront(members, slot)
		drawEnemyRosterRow(assets.hudFont, enemy, x+14, rowY, w-28, rowH-8, targetable && slot == selectedSlot, !enemy.Alive, known, reachable)
	}
}

// rosterSlotsBuf is the reused buffer for visibleRosterSlots (single-threaded draw).
var rosterSlotsBuf = make([]int, 0, 16)

func visibleRosterSlots(g *core.GameState) []int {
	rosterSlotsBuf = rosterSlotsBuf[:0]
	// Index-range: value-range would copy ~496 bytes per member just to read two bools.
	members := core.BattleMembers(g)
	for i := range members {
		if !members[i].Alive && members[i].DeathFade <= 0 {
			continue
		}
		rosterSlotsBuf = append(rosterSlotsBuf, i)
	}
	return rosterSlotsBuf
}

func drawEnemyRosterRow(font rl.Font, enemy *core.Enemy, x, y, w, h int32, targeted, fading, known, reachable bool) {
	bg := surfaceRosterRow
	border := borderRosterRow
	nameCol := textPrimary
	// A living-but-unreachable foe greys out like a dying one, so "can't hit this" reads.
	if fading || !reachable {
		bg = surfaceRosterRowFaded
		border = borderDim
		nameCol = textDim
	}
	// Bright target fill + halo only for a REACHABLE selected foe; an unreachable one
	// keeps its grey look but shows a dim cursor arrow below.
	if targeted && reachable {
		bg = core.MixColor(bg, surfaceEnemyTint, 0.9)
		border = borderEnemy
	}
	drawGlassPane(x, y, w, h, bg)
	if targeted && reachable {
		// Shared drawSelectionHalo with the party ribbon's active card.
		drawSelectionHalo(x, y, w, h, borderEnemy, pulseHalo(), true)
	} else {
		drawSmallPanelOutline(x, y, w, h, border)
	}

	leftPad := hudContentInsetX
	if targeted {
		leftPad = rosterTargetedNameInset
		bx := float32(x) + rosterArrowMarkerInsetX
		cy := float32(y) + float32(h)/2
		col := fadeColor(borderEnemy, pulseHalo())
		if !reachable {
			col = borderDim // muted cursor on an unreachable foe
		}
		drawArrowMarker(rl.NewVector2(bx, cy), rosterArrowMarkerTipDx, 0, rosterArrowMarkerHalf, col)
	}

	condition, condCol := enemyHealthStyle(enemy)

	nameX := float32(x + leftPad)
	nameY := y + 10
	displayName := core.EnemyName(enemy)
	drawEngravedText(font, displayName, nameX, float32(nameY), FontHeading, nameCol)

	// Wound-state word always; real HP in claret only when known. No HP bar.
	condSize := FontSmall
	// Stack the condition below the name (bottom-anchoring collided with the tall name).
	condY := float32(nameY) + FontHeading + 2
	drawTextWithShadow(font, condition, nameX, condY, condSize, condCol)
	if known {
		condW := rosterCondMeasureCache.measure(font, condition, condSize, canonicalSpacing(condSize)).X
		drawTextWithShadow(font, enemyHPLabel(enemy.HP, enemy.MaxHP), nameX+condW+12, condY, condSize, barEnemyHP)
	}

	// Status pills, anchored to the right edge (no HP bar to tuck beside).
	pillW := rosterStatusPillW
	pillH := rosterStatusPillH
	rightEdge := float32(x+w) - rosterStatusRightPad
	pillX := rightEdge - pillW
	pillBaseY := float32(y) + (float32(h)-pillH)/2

	// Status pills stack upward from pillBaseY, walking the init-asserted visual table.
	slot := 0
	for _, p := range enemyStatusPillVisuals {
		turns := p.turns(enemy)
		if turns <= 0 {
			continue
		}
		fill := p.fill
		if p.flicker {
			fill = fadeColor(fill, pulseFlicker())
		}
		pillY := pillBaseY - float32(slot)*(pillH+4)
		drawEnemyStatusPill(font, pillX, pillY, pillW, pillH,
			fill, p.outline, p.glyph, statusTurnsLabel(turns))
		slot++
	}
}

// statusTurnsLabel returns the bare turn-count pill label (numbers only; identity is glyph+fill).
// Reads the shared statusTurnDigit cache to avoid per-frame Sprintf.
func statusTurnsLabel(turns int) string {
	return statusTurnDigit(turns)
}

// rosterCondMeasureCache memoizes MeasureTextEx for the wound-state words the HP readout offsets against.
var rosterCondMeasureCache measureCache

// enemyHPLabelCache memoizes "HP/MaxHP" per (hp,max). Bounded to pairs seen in play.
var enemyHPLabelCache = map[[2]int]string{}

func enemyHPLabel(hp, max int) string {
	k := [2]int{hp, max}
	if s, ok := enemyHPLabelCache[k]; ok {
		return s
	}
	s := fmt.Sprintf("%d/%d", hp, max)
	enemyHPLabelCache[k] = s
	return s
}


// drawStatusPill paints the shared status-pill silhouette (fill + outline + FontSmall label),
// used by the enemy roster and the Stats-tab chip. centered true centers the label (+2 top);
// false left-aligns it (+10/+4). labelCol picks the tone.
func drawStatusPill(font rl.Font, x, y, w, h float32, fill, outline rl.Color, label string, labelCol rl.Color, centered bool) {
	drawSmallPanel(int32(x), int32(y), int32(w), int32(h), fill)
	drawSmallPanelOutline(int32(x), int32(y), int32(w), int32(h), outline)
	if centered {
		drawTextCentered(font, label, x+w/2, y+2, FontSmall, labelCol)
	} else {
		drawTextWithShadow(font, label, x+10, y+4, FontSmall, labelCol)
	}
}

// drawEnemyStatusPill paints one status pill: fill + outline, a left glyph (shared
// drawStatusGlyph*), and the turn count on the right, both in dark glyph ink.
func drawEnemyStatusPill(font rl.Font, x, y, w, h float32, fill, outline rl.Color, glyph func(cx, cy, r float32, col rl.Color), turnsLabel string) {
	// Silhouette from drawStatusPill (empty label — glyph + count anchored below).
	drawStatusPill(font, x, y, w, h, fill, outline, "", statusGlyphDark, true)
	if glyph != nil {
		glyph(x+w*0.30, y+h*0.5, h*0.28, statusGlyphDark)
	}
	drawTextCentered(font, turnsLabel, x+w*0.72, y+2, FontSmall, statusGlyphDark)
}

// actionLogTextPad is the text inset for the action-log; the wrap width and per-line X
// both read it so the inset can't drift between the two seams.
const actionLogTextPad = int32(10)

// actionLogSpineInset is the spine stripe's top/bottom inset (stripe runs panelH - 2×inset).
const actionLogSpineInset = int32(18)

// Action-log spine wood-accent alphas: main binding stripe and dimmer binding ties.
const (
	actionLogSpineAlpha = float32(0.75)
	actionLogTieAlpha   = float32(0.45)
)

// drawActionLogSpine paints the ledger-spine ornament down the left inside edge of the
// action-log pane: a wood-accent stripe, gilt fleurons at both ends, a mid pip with binding ties.
func drawActionLogSpine(panelX, panelY, panelH int32) {
	stripeX := panelX + actionLogTextPad
	stripeY := panelY + actionLogSpineInset
	stripeH := panelH - 2*actionLogSpineInset
	rl.DrawRectangle(stripeX, stripeY, 2, stripeH, fadeColor(woodAccent, actionLogSpineAlpha))
	centreX := float32(stripeX) + 1
	drawFleuron(centreX, float32(stripeY)-2, 3, giltDim)
	drawFleuron(centreX, float32(stripeY+stripeH)+2, 3, giltDim)
	midY := float32(stripeY) + float32(stripeH)*0.5
	drawDiamondPip(centreX, midY, 2.5, giltDim)
	tieCol := fadeColor(woodAccent, actionLogTieAlpha)
	rl.DrawRectangle(stripeX-4, int32(midY), 4, 1, tieCol)
	rl.DrawRectangle(stripeX+2, int32(midY), 4, 1, tieCol)
}

// actionLogVisualLine is one wrapped+styled source log line (package-scope for the cache).
type actionLogVisualLine struct {
	text  string
	fresh bool
}

// actionLogCache memoizes the wrapped log between frames. Invalidates on log length,
// last-line content, or panel-geometry change.
var actionLogCache struct {
	visible      []actionLogVisualLine
	lastLogLen   int
	lastLastLine string
	lastInnerW   int32
	lastMaxLines int
}

// statusLineScratch is the reused 1-slot backing for the empty-log StatusMessage path.
var statusLineScratch [1]string

// shrinkPinnedToBottom shrinks a bottom-pinned pane (floored at hudPanelMinH) to clear
// topLimit while keeping its bottom edge at bottomY. Shared by the two bottom HUD panes.
func shrinkPinnedToBottom(bottomY, topLimit int32) (h, y int32) {
	h = bottomY - topLimit
	if h < hudPanelMinH {
		h = hudPanelMinH
	}
	return h, bottomY - h
}

// drawActionLogPanel paints the rolling ACTION LOG (bottom-left, in combat and exploration).
func drawActionLogPanel(g *core.GameState, assets Resources) {
	// Bottom edge pins to the screen bottom; stretches up toward the turn panel, floored at 160px.
	w := actionLogW
	h := actionLogH
	_, screenH := screenSize()
	x := hudEdgePad
	bottomY := screenH - hudEdgePad
	y := bottomY - h

	// Top collision guard against the turn panel: shrink to clear it, bottom edge pinned.
	if turnBottom := TurnPanelBottomY(g) + hudColumnGap; y < turnBottom {
		h, y = shrinkPinnedToBottom(bottomY, turnBottom)
	}

	drawPanelCard(x, y, w, h)
	drawActionLogSpine(x, y, h)

	// Inner inset: frame ~6px + 8px off the bevel; +6px on the left clears the spine.
	innerInset := int32(14)
	innerX := x + innerInset + 6
	innerY := y + innerInset
	innerW := w - 2*innerInset - 6
	innerH := h - 2*innerInset

	lineH := int32(22)
	lineSize := FontSmall

	// Ruled-parchment hairlines at each line slot's bottom, on the same -6 footing as
	// the text loop so entries sit on their rule. Inset from both edges.
	ruleX := innerX + 2
	ruleW := innerW - 10
	for ry := innerY + innerH - 6; ry > innerY+lineH/2; ry -= lineH {
		rl.DrawRectangle(ruleX, ry, ruleW, 1, fadeColor(inkDim, 0.13))
	}

	lines := g.ActionLog
	if len(lines) == 0 && g.StatusMessage != "" {
		// Reuse the package 1-slot buffer rather than allocating per frame.
		statusLineScratch[0] = g.StatusMessage
		lines = statusLineScratch[:]
	}
	if len(lines) == 0 {
		return
	}
	maxLines := int(innerH / lineH)
	if maxLines < 1 {
		maxLines = 1
	}

	wrapW := float32(innerW - 2*actionLogTextPad)
	visible := wrappedActionLogLines(assets.hudFont, lines, innerW, maxLines, lineSize, wrapW)
	startY := innerY + innerH - int32(len(visible))*lineH - 6
	n := len(visible)
	for i, vl := range visible {
		// Fade-to-top: newest at full alpha, oldest at 0.5 (0.18 floor faded too much).
		var alpha float32 = 1
		if n > 1 {
			posT := float32(i) / float32(n-1) // 0 at top, 1 at bottom
			alpha = 0.5 + 0.5*posT
		}
		base := textMuted
		if vl.fresh {
			base = textPrimary
		}
		col := fadeColor(base, alpha)
		drawTextWithShadow(assets.hudFont, vl.text, float32(innerX+actionLogTextPad), float32(startY+int32(i)*lineH), lineSize, col)
	}
}

// wrappedActionLogLines returns the visible wrapped log lines, reusing the cache when
// (length, last-line, innerW, maxLines) are unchanged.
func wrappedActionLogLines(font rl.Font, lines []string, innerW int32, maxLines int, lineSize, wrapW float32) []actionLogVisualLine {
	lastLine := ""
	if len(lines) > 0 {
		lastLine = lines[len(lines)-1]
	}
	if actionLogCache.lastLogLen == len(lines) &&
		actionLogCache.lastLastLine == lastLine &&
		actionLogCache.lastInnerW == innerW &&
		actionLogCache.lastMaxLines == maxLines {
		return actionLogCache.visible
	}

	// Walk from the newest entry backward, wrapping in reverse, stopping once the panel
	// is full — caps work at ~maxLines instead of wrapping all 40 sources then slicing.
	reversed := actionLogCache.visible[:0]
	if cap(reversed) < maxLines {
		reversed = make([]actionLogVisualLine, 0, maxLines)
	}
	for i := len(lines) - 1; i >= 0 && len(reversed) < maxLines; i-- {
		fresh := i == len(lines)-1
		src := lines[i]
		wraps := wrapTextLines(font, src, lineSize, wrapW)
		if len(wraps) == 0 {
			// Empty source line preserved as a blank row.
			reversed = append(reversed, actionLogVisualLine{text: "", fresh: fresh})
			continue
		}
		// Append in reverse (reversed[] stays newest-first); the pass below restores order.
		for j := len(wraps) - 1; j >= 0; j-- {
			reversed = append(reversed, actionLogVisualLine{text: wraps[j], fresh: fresh})
			if len(reversed) >= maxLines {
				break
			}
		}
	}
	// Reverse in place into chronological order (avoids a second slice alloc).
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	actionLogCache.visible = reversed
	actionLogCache.lastLogLen = len(lines)
	actionLogCache.lastLastLine = lastLine
	actionLogCache.lastInnerW = innerW
	actionLogCache.lastMaxLines = maxLines
	return reversed
}

// arrowPrompt caches the "A → B" target prompt to avoid per-frame Sprintf as the player
// cycles targets. Shared by both target modes (only one is active at a time).
var arrowPromptCache struct{ a, b, text string }

func arrowPrompt(a, b string) string {
	if a != arrowPromptCache.a || b != arrowPromptCache.b {
		arrowPromptCache.a, arrowPromptCache.b = a, b
		arrowPromptCache.text = a + " → " + b
	}
	return arrowPromptCache.text
}

// drawAllyTargetPrompt paints the "verb → ally / Choose an ally" prompt shared by the
// skill- and item-target arms. verb is the skill/item name; ally falls back to "Ally".
func drawAllyTargetPrompt(g *core.GameState, assets Resources, verb string, contentX, contentY, subY int32) {
	targetName := "Ally"
	if g.Battle.PartyTarget >= 0 && g.Battle.PartyTarget < len(g.Party) {
		targetName = g.Party[g.Battle.PartyTarget].Name
	}
	drawTextWithShadow(assets.hudFont, arrowPrompt(verb, targetName), float32(contentX), float32(contentY), FontBody, textPrimary)
	drawTextWithShadow(assets.hudFont, "Choose an ally", float32(contentX), float32(subY), FontSmall, textLabel)
}

func drawActionMenuPanel(g *core.GameState, assets Resources) {
	if g.Battle.Phase != core.BattlePlayer {
		return
	}
	if g.Battle.CurrentParty < 0 || g.Battle.CurrentParty >= len(g.Party) {
		return
	}
	member := g.Party[g.Battle.CurrentParty]
	if member.HP <= 0 {
		return
	}

	screenW, screenH := screenSize()
	w := actionMenuW
	// Pins to the bottom-right corner, hudEdgePad from each edge.
	h := actionMenuH
	x := screenW - w - hudEdgePad
	bottomY := screenH - hudEdgePad
	y := bottomY - h
	// Short-window guard: shrink to clear the top edge, bottom pinned.
	if y < hudEdgePad {
		h, y = shrinkPinnedToBottom(bottomY, hudEdgePad)
	}

	classCol := classAccent(member.Class)
	drawCard(x, y, w, h, surfacePrimary, borderActive, classCol)

	contentX := x + hudContentInsetX
	// rightX: content inset from the right edge; rows stretch their highlight to it.
	rightX := x + w - hudContentInsetX
	// Active member's name as the header, in class color, over a gilt divider rule.
	drawEngravedText(assets.hudFont, member.Name, float32(contentX), float32(y+14), FontHeading, classCol)
	ruleY := y + 48
	drawPipCappedRule(x+18, ruleY, w-36, fadeColor(giltBright, 0.5), 2.4, fadeColor(giltDim, 0.85))
	contentY := y + 58
	// subY: sub-prompt/list start, via bodyBelowHeading for a consistent heading→body gap.
	subY := bodyBelowHeading(contentY, FontHeading)

	switch g.Battle.ActionMode {
	case core.ActionEnemyTarget:
		actionLabel := "Attack"
		if g.Battle.PendingSkill != core.SkillNone {
			actionLabel = core.SkillName(g.Battle.PendingSkill)
		}
		drawEngravedText(assets.hudFont, actionLabel, float32(contentX), float32(contentY), FontHeading, textPrimary)
		drawTextWithShadow(assets.hudFont, "Choose a target", float32(contentX), float32(subY), FontSmall, textLabel)
	case core.ActionPartyTarget:
		drawAllyTargetPrompt(g, assets, core.SkillName(g.Battle.PendingSkill), contentX, contentY, subY)
	case core.ActionItemMenu:
		drawEngravedText(assets.hudFont, "Items", float32(contentX), float32(contentY), FontHeading, textPrimary)
		drawItemMenuList(g, assets, contentX, subY, rightX)
	case core.ActionSkillMenu:
		drawEngravedText(assets.hudFont, "Skills", float32(contentX), float32(contentY), FontHeading, textPrimary)
		drawSkillMenuList(g, assets, contentX, subY, rightX)
	case core.ActionItemTarget:
		drawAllyTargetPrompt(g, assets, core.ItemInfo(g.Battle.PendingItem).Name, contentX, contentY, subY)
	case core.ActionFleeConfirm:
		drawEngravedText(assets.hudFont, "Flee", float32(contentX), float32(contentY), FontHeading, textPrimary)
		drawTextWithShadow(assets.hudFont, "Retreat from this battle?", float32(contentX), float32(subY), FontSmall, textLabel)
	default:
		// Transient status line (setBattleStatus) for validation errors, e.g. "Swipe needs more MP."
		if status := transientStatus(g); status != "" {
			drawTextWithShadow(assets.hudFont, status, float32(contentX), float32(contentY), FontSmall, classCol)
			contentY += 26
		}
		drawActionMenuOptions(g, assets, contentX, contentY, rightX, member)
	}

	// Confirm/Back hint footer over a gilt rule; skipped when the panel is shrunk short.
	if h >= actionMenuHintMinH {
		hintY := footerBaselineY(y+h, FontSmall)
		drawGiltRule(x+18, hintY-12, w-36, 1, 0.3)
		DrawHintBarLeft(assets.hudFont, []HintSeg{
			Hint("Confirm", GlyphA),
			Hint("Back", GlyphB),
		}, float32(contentX), float32(hintY), FontSmall)
	}
}

// transientStatus returns StatusMessage when it's unlogged (set via setBattleStatus);
// "" when empty or matching the last log entry, so messages don't render twice.
func transientStatus(g *core.GameState) string {
	msg := g.StatusMessage
	if msg == "" {
		return ""
	}
	if n := len(g.ActionLog); n > 0 && g.ActionLog[n-1] == msg {
		return ""
	}
	return msg
}

func drawActionMenuOptions(g *core.GameState, assets Resources, x, y, rightX int32, member core.PartyMember) {
	_ = member
	cursor := core.ActionRow(g.Battle.MenuIndex)
	// Labels pushed right for the left icon column; rows pitch by uiRowPitch and stretch
	// to rightX. Driven by the init-asserted actionRowLabels table.
	labelX := x + 26
	for row := core.ActionRow(0); int(row) < core.ActionRowCount; row++ {
		drawActionMenuRow(assets.hudFont, row, x, labelX, y+int32(row)*uiRowPitch, rightX, actionRowLabels[row], "", cursor == row)
	}
}

// drawActionMenuRow wraps drawActionRow with a per-action sigil medallion in the left gutter.
func drawActionMenuRow(font rl.Font, row core.ActionRow, iconX, labelX, y, rightX int32, label, suffix string, selected bool) {
	drawActionRow(font, labelX, y, rightX, label, suffix, selected)
	iconCX := float32(iconX) + 9
	iconCY := float32(y) + 13
	drawIconMedallion(iconCX, iconCY, selected)
	iconCol := giltDim
	if selected {
		iconCol = giltBright
	}
	drawActionIcon(row, iconCX, iconCY, 7, iconCol)
}

// drawMedallion paints the shared socketed-medallion primitive (used by the action rivet and
// the class badge): optional shadow disc, dark outer seat, gilt ring, recessed face. shadowR>0
// paints a contact shadow first; pip, if non-nil, runs last for caller embellishment.
func drawMedallion(cx, cy, outerR, ringR, faceR float32, outerCol, ringCol, faceCol rl.Color, shadowR float32, pip func()) {
	if shadowR > 0 {
		rl.DrawCircleV(rl.NewVector2(cx+1, cy+2), shadowR, fadeColor(shadowHeavy, 0.24))
	}
	rl.DrawCircleV(rl.NewVector2(cx, cy), outerR, outerCol)
	rl.DrawCircleV(rl.NewVector2(cx, cy), ringR, ringCol)
	rl.DrawCircleV(rl.NewVector2(cx, cy), faceR, faceCol)
	if pip != nil {
		pip()
	}
}

// drawIconMedallion paints the action-sigil rivet (gilt ring candle-lit when selected).
func drawIconMedallion(cx, cy float32, selected bool) {
	ring := fadeColor(giltDim, 0.85)
	if selected {
		ring = fadeColor(giltBright, 0.9*candleFlicker())
	}
	drawMedallion(cx, cy, 12, 11, 9.5,
		fadeColor(woodDark, 0.95), ring, fadeColor(glassDeep, 0.96), 0, nil)
}

// actionIconDrawers maps each action-menu row to its sigil drawer. Fixed-size array (init
// asserts no nil entry). Attack reuses the warrior class glyph.
var actionIconDrawers = [core.ActionRowCount]func(cx, cy, r float32, col rl.Color){
	core.ActionRowAttack: drawClassGlyphWarrior,
	core.ActionRowSkill:  drawActionIconSkill,
	core.ActionRowItem:   drawActionIconItem,
	core.ActionRowDefend: drawActionIconDefend,
	core.ActionRowSwap:   drawActionIconSwap,
	core.ActionRowFlee:   drawActionIconFlee,
}

// actionRowLabels is the player-facing label per action-menu row. Fixed-size, init-asserted non-empty.
var actionRowLabels = [core.ActionRowCount]string{
	core.ActionRowAttack: "Attack",
	core.ActionRowSkill:  "Skill",
	core.ActionRowItem:   "Item",
	core.ActionRowDefend: "Defend",
	core.ActionRowSwap:   "Swap",
	core.ActionRowFlee:   "Flee",
}

func init() {
	for row := core.ActionRow(0); int(row) < core.ActionRowCount; row++ {
		if actionIconDrawers[row] == nil {
			panic(fmt.Sprintf("render: ActionRow %d has no actionIconDrawers entry", int(row)))
		}
		if actionRowLabels[row] == "" {
			panic(fmt.Sprintf("render: ActionRow %d has no actionRowLabels entry", int(row)))
		}
	}
}

// drawActionIcon dispatches to the per-action sigil drawer (sized by r, tinted by col).
func drawActionIcon(row core.ActionRow, cx, cy, r float32, col rl.Color) {
	actionIconDrawers[row](cx, cy, r, col)
}

// drawActionIconSkill paints the Skill action's arcane-spark starburst.
func drawActionIconSkill(cx, cy, r float32, col rl.Color) {
	rayHalf := r * 0.22
	// Vertical ray.
	drawTriangleCCW(rl.NewVector2(cx, cy-r), rl.NewVector2(cx-rayHalf, cy), rl.NewVector2(cx+rayHalf, cy), col)
	drawTriangleCCW(rl.NewVector2(cx, cy+r), rl.NewVector2(cx+rayHalf, cy), rl.NewVector2(cx-rayHalf, cy), col)
	// Horizontal ray.
	drawTriangleCCW(rl.NewVector2(cx-r, cy), rl.NewVector2(cx, cy+rayHalf), rl.NewVector2(cx, cy-rayHalf), col)
	drawTriangleCCW(rl.NewVector2(cx+r, cy), rl.NewVector2(cx, cy-rayHalf), rl.NewVector2(cx, cy+rayHalf), col)
	// Diagonal short rays.
	dr := r * 0.55
	for _, sign := range [4][2]float32{{1, 1}, {1, -1}, {-1, 1}, {-1, -1}} {
		dx := sign[0] * sqrt2Inv * dr
		dy := sign[1] * sqrt2Inv * dr
		drawDiamondPip(cx+dx, cy+dy, 1.5, col)
	}
	rl.DrawCircleV(rl.NewVector2(cx, cy), r*0.22, giltBright)
}

// drawActionIconItem paints the Item action's apothecary-flask silhouette.
func drawActionIconItem(cx, cy, r float32, col rl.Color) {
	// Stopper at top.
	stopperHalfW := r * 0.32
	rl.DrawRectangle(int32(cx-stopperHalfW), int32(cy-r), int32(stopperHalfW*2), int32(2), col)
	// Neck.
	neckHalfW := r * 0.22
	neckTop := cy - r + 2
	neckBottom := cy - r*0.45
	rl.DrawRectangle(int32(cx-neckHalfW), int32(neckTop), int32(neckHalfW*2), int32(neckBottom-neckTop), col)
	// Body.
	bodyHalfW := r * 0.65
	bodyTop := neckBottom
	bodyBottom := cy + r*0.85
	rl.DrawRectangle(int32(cx-bodyHalfW), int32(bodyTop+3), int32(bodyHalfW*2), int32(bodyBottom-bodyTop-3), col)
	// Shoulders.
	drawTriangleCCW(rl.NewVector2(cx-bodyHalfW, bodyTop+3), rl.NewVector2(cx-neckHalfW, bodyTop), rl.NewVector2(cx-neckHalfW, bodyTop+3), col)
	drawTriangleCCW(rl.NewVector2(cx+bodyHalfW, bodyTop+3), rl.NewVector2(cx+neckHalfW, bodyTop+3), rl.NewVector2(cx+neckHalfW, bodyTop), col)
	// Liquid line.
	rl.DrawRectangle(int32(cx-bodyHalfW+2), int32(bodyBottom-r*0.35), int32(bodyHalfW*2-4), 2, giltBright)
}

// drawActionIconDefend paints the Defend action's tower-shield sigil (distinct from the
// equipment-slot heater so the two surfaces don't read alike).
func drawActionIconDefend(cx, cy, r float32, col rl.Color) {
	topW := r * 1.2
	topH := r * 0.5
	rl.DrawRectangle(int32(cx-topW/2), int32(cy-r), int32(topW), int32(topH), col)
	// Body taper.
	tip := rl.NewVector2(cx, cy+r*0.95)
	left := rl.NewVector2(cx-topW/2, cy-r+topH)
	right := rl.NewVector2(cx+topW/2, cy-r+topH)
	drawTriangleCCW(tip, right, left, col)
	// Centre band.
	bandHalfW := r * 0.10
	rl.DrawRectangle(int32(cx-bandHalfW), int32(cy-r+topH-1), int32(bandHalfW*2), int32(r*1.05), fadeColor(col, 0.6))
	// Boss + pip.
	rl.DrawCircleV(rl.NewVector2(cx, cy-r*0.05), r*0.26, fadeColor(col, 0.6))
	rl.DrawCircleV(rl.NewVector2(cx, cy-r*0.05), r*0.12, giltBright)
}

// drawActionIconSwap paints the Swap action's up+down arrow pair.
func drawActionIconSwap(cx, cy, r float32, col rl.Color) {
	thick := r * 0.3
	head := r * 0.42
	// Left shaft UP, right shaft DOWN.
	lx, rx := cx-r*0.42, cx+r*0.42
	rl.DrawLineEx(rl.NewVector2(lx, cy+r), rl.NewVector2(lx, cy-r), thick, col)
	rl.DrawLineEx(rl.NewVector2(lx, cy-r), rl.NewVector2(lx-head, cy-r+head), thick, col)
	rl.DrawLineEx(rl.NewVector2(lx, cy-r), rl.NewVector2(lx+head, cy-r+head), thick, col)
	rl.DrawLineEx(rl.NewVector2(rx, cy-r), rl.NewVector2(rx, cy+r), thick, col)
	rl.DrawLineEx(rl.NewVector2(rx, cy+r), rl.NewVector2(rx-head, cy+r-head), thick, col)
	rl.DrawLineEx(rl.NewVector2(rx, cy+r), rl.NewVector2(rx+head, cy+r-head), thick, col)
}

// drawActionIconFlee draws the Flee action's double-chevron dash sigil (leading chevron brighter).
func drawActionIconFlee(cx, cy, r float32, col rl.Color) {
	thick := r * 0.34
	h := r * 0.62
	chevron := func(tipX float32, c rl.Color) {
		tip := rl.NewVector2(tipX, cy)
		rl.DrawLineEx(rl.NewVector2(tipX-h, cy-h), tip, thick, c)
		rl.DrawLineEx(tip, rl.NewVector2(tipX-h, cy+h), thick, c)
	}
	chevron(cx-r*0.15, fadeColor(col, 0.6)) // trailing, dimmer
	chevron(cx+r*0.5, col)                  // leading
}

// drawSkillMenuList renders the skill submenu (one row per learned skill, MP cost right).
// Reads the prebuilt g.Battle.SkillMenuList.
func drawSkillMenuList(g *core.GameState, assets Resources, x, y, rightX int32) {
	skills := g.Battle.SkillMenuList
	if len(skills) == 0 {
		drawTextWithShadow(assets.hudFont, "(no skills)", float32(x), float32(y), FontSmall, textDim)
		return
	}
	for i, s := range skills {
		label := core.SkillName(s)
		suffix := ""
		if cost := core.SkillCost(s); cost > 0 {
			suffix = skillCostMPLabel(cost)
		}
		drawActionRow(assets.hudFont, x, y+int32(i)*uiRowPitch, rightX, label, suffix, g.Battle.SkillMenuIndex == i)
	}
}

// drawItemMenuList renders the inventory picker ("Name xCount" rows). Reads the prebuilt
// g.Battle.ItemMenuList (consumables only, matching updateItemMenu).
func drawItemMenuList(g *core.GameState, assets Resources, x, y, rightX int32) {
	living := g.Battle.ItemMenuList
	if len(living) == 0 {
		drawTextWithShadow(assets.hudFont, "(no items)", float32(x), float32(y), FontSmall, textDim)
		return
	}
	for i, slot := range living {
		def := core.ItemInfo(slot.Kind)
		label := def.Name
		suffix := "x" + strconv.Itoa(slot.Count)
		drawActionRow(assets.hudFont, x, y+int32(i)*uiRowPitch, rightX, label, suffix, g.Battle.ItemMenuIndex == i)
	}
}

// drawActionRow paints one key-plate row (selected gets the gilt plate, else dark glass).
// The plate spans x-8 to rightX so the highlight reaches the content edge regardless of text start.
func drawActionRow(font rl.Font, x, y, rightX int32, label, suffix string, selected bool) {
	plateX := x - 8
	plateW := rightX - plateX
	if selected {
		DrawSelectedRowI(plateX, y-4, plateW, uiRowH)
	} else {
		drawGlassPane(plateX, y-4, plateW, uiRowH, fadeColor(glassDeep, 0.5))
		drawSmallPanelOutline(plateX, y-4, plateW, uiRowH, fadeColor(woodMid, 0.45))
	}
	drawTextWithShadow(font, label, float32(x), float32(y), FontBody, textPrimary)
	if suffix != "" {
		size := FontSmall
		measure := measureActionRowSuffix(font, suffix)
		sx := float32(rightX) - measure.X - 12
		sy := float32(y) + 5
		drawTextWithShadow(font, suffix, sx, sy, size, textLabel)
	}
}

// actionRowSuffixMeasureCache memoizes drawActionRow's right-side suffix measurements.
var actionRowSuffixMeasureCache measureCache

func measureActionRowSuffix(font rl.Font, suffix string) rl.Vector2 {
	return actionRowSuffixMeasureCache.measure(font, suffix, FontSmall, 1)
}

// enemyConditionColors is the roster wound-state tint, indexed by core.EnemyCondition
// (init asserts no zero-alpha gap).
var enemyConditionColors = [core.EnemyConditionCount]color.RGBA{
	core.EnemyUnharmed:     condEnemyUnharmed,
	core.EnemyScuffed:      condEnemyScuffed,
	core.EnemyInjured:      condEnemyInjured,
	core.EnemyBadlyWounded: condEnemyBadlyWounded,
	core.EnemyNearDeath:    condEnemyNearDeath,
}

func init() {
	for c, col := range enemyConditionColors {
		if col.A == 0 {
			panic(fmt.Sprintf("render: enemyConditionColors missing entry for condition %d", c))
		}
	}
}

func enemyHealthStyle(enemy *core.Enemy) (string, color.RGBA) {
	condition := core.EnemyConditionFor(enemy)
	return core.EnemyConditionLabel(condition), enemyConditionColors[condition]
}

// splashBgColor / splashTitleColor now live in theme.go's palette block.

// Splash ease windows (seconds, within core.BattleSplashDuration): enter lead-in, exit tail-out.
const (
	splashEnterDur = float32(0.18)
	splashExitDur  = float32(0.32)
)

// drawBattleSplash slams the encounter-title banner in at the top on battle start.
func drawBattleSplash(g *core.GameState, assets Resources) {
	members := core.BattleMembers(g)
	if g.Battle.Splash <= 0 || g.Battle.EnemyIndex < 0 || g.Battle.EnemyIndex >= len(members) {
		return
	}
	progress := core.BattleSplashDuration - g.Battle.Splash
	if progress < 0 {
		progress = 0
	}
	enterT := progress / splashEnterDur
	if enterT > 1 {
		enterT = 1
	}
	exitT := float32(1)
	if g.Battle.Splash < splashExitDur {
		exitT = g.Battle.Splash / splashExitDur
	}
	intro := easeOutBack(enterT)
	overall := exitT

	text := core.BattleEncounterTitle(g)
	subtitle := splashSubtitle(g)
	// FontTitle name / FontBody subtitle (highest-emphasis transient surface, UI_STANDARDS.md).
	titleSize := FontTitle
	subSize := FontBody
	spacing := FontSpacingTitle

	// Strings stable across the ~40-frame splash, so measure once (at base size) and scale.
	titleMeasure := splashTitleMeasureCache.measure(assets.hudFont, text, titleSize, spacing)
	subMeasure := rl.NewVector2(0, 0)
	if subtitle != "" {
		subMeasure = splashSubMeasureCache.measure(assets.hudFont, subtitle, subSize, 1)
	}

	scale := 0.86 + 0.14*intro
	titleW := titleMeasure.X * scale
	titleH := titleMeasure.Y * scale
	contentW := titleW
	if subMeasure.X > contentW {
		contentW = subMeasure.X
	}

	padX := float32(40)
	padY := float32(22)
	gap := float32(0)
	if subtitle != "" {
		gap = 8
	}

	bgW := contentW + padX*2
	bgH := titleH + subMeasure.Y + gap + padY*2

	sw, sh := screenSizeF()
	cx := sw / 2
	cy := sh*0.42 + (1-intro)*-26

	bgX := cx - bgW/2
	bgY := cy - bgH/2

	bgAlpha := uint8(220 * overall)
	titleAlpha := uint8(255 * overall)
	subAlpha := uint8(220 * overall)

	drawPanel(int32(bgX), int32(bgY), int32(bgW), int32(bgH), colorWithAlpha(splashBgColor, bgAlpha))
	drawPanelOutline(int32(bgX), int32(bgY), int32(bgW), int32(bgH), fadeColor(borderEnemy, overall))

	titleX := cx - titleW/2
	titleY := bgY + padY
	// Fade-driven shadow alpha + heavier 3px drop + title spacing via drawTextWithShadowStyle.
	drawTextWithShadowStyle(assets.hudFont, text, titleX, titleY, titleSize*scale, spacing*scale,
		colorWithAlpha(splashTitleColor, titleAlpha), colorWithAlpha(shadowBase, titleAlpha), 3, 3)

	if subtitle != "" {
		subX := cx - subMeasure.X/2
		subY := titleY + titleH + gap
		// Gilt rule + centred fleuron between title and subtitle; width 60% of subtitle, alpha rides the fade.
		ruleW := subMeasure.X * 0.6
		ruleY := subY - 4
		ruleCol := fadeColor(giltDim, overall)
		drawSplitRule(cx-ruleW/2, cx+ruleW/2, cx, ruleY, 8, ruleCol)
		drawFleuron(cx, ruleY, 3, fadeColor(giltBright, overall))
		drawTextWithShadowStyle(assets.hudFont, subtitle, subX, subY, subSize, 1,
			colorWithAlpha(borderEnemy, subAlpha), colorWithAlpha(shadowBase, subAlpha), 1, 1)
	}
}

// splashTitleMeasureCache / splashSubMeasureCache memoize the title + subtitle measures.
var (
	splashTitleMeasureCache measureCache
	splashSubMeasureCache   measureCache
)

// splashSubtitleCache memoizes the subtitle by (count, noun) to skip per-frame Sprintf.
var splashSubtitleCache struct {
	count int
	noun  string
	text  string
}

func splashSubtitle(g *core.GameState) string {
	count := len(core.BattleMembers(g))
	if count <= 1 {
		return "Hostile encounter"
	}
	def := core.BattleEnemyInfo(g)
	if splashSubtitleCache.count != count || splashSubtitleCache.noun != def.PluralNoun {
		splashSubtitleCache.count = count
		splashSubtitleCache.noun = def.PluralNoun
		splashSubtitleCache.text = fmt.Sprintf("%d %s closing in", count, def.PluralNoun)
	}
	return splashSubtitleCache.text
}

func easeOutBack(t float32) float32 {
	if t <= 0 {
		return 0
	}
	if t >= 1 {
		return 1
	}
	const c1 = 1.70158
	const c3 = c1 + 1
	x := float64(t) - 1
	return float32(1 + c3*math.Pow(x, 3) + c1*math.Pow(x, 2))
}

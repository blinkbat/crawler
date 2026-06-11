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
// region (top-center roster, bottom-left log, bottom-center action, top-right
// turn order) so they never compete for the same real estate. The combat
// log is pinned bottom-left through every phase — including the timing
// minigame — so the player can keep reading the last few events while
// they press. The action menu yields to the bar (it has nothing useful
// to show during press/charge anyway).
func drawBattleHUD(g core.GameState, assets Resources) {
	drawEnemyRoster(g, assets)
	drawCombatLogPanel(g, assets)
	if !timingActive(g) {
		drawActionMenuPanel(g, assets)
	}
}

// timingActive reports whether the timed-hit bar is currently the focus of
// the HUD. Used to hide panels that share its strip.
func timingActive(g core.GameState) bool {
	return g.Battle.Phase == core.BattleAttackTiming || g.Battle.Phase == core.BattleEnemyTiming
}

// inPlayerTurn reports whether the current phase is "the player is acting" —
// either the menu/target picker or the resolving timing bar. Visual indicators
// for the active actor + chosen target should persist through the bar so the
// player keeps their bearings, instead of flickering off the moment the bar
// arms and back on when it resolves.
func inPlayerTurn(g core.GameState) bool {
	return g.Battle.Phase == core.BattlePlayer || g.Battle.Phase == core.BattleAttackTiming
}

// targetingEnemy reports whether the player is currently in the
// "choose an enemy" target phase — Phase MUST be BattlePlayer (drops
// the moment the timing bar arms) and ActionMode == ActionEnemyTarget.
// Single source for the two render gates that overlay a "yellow cursor"
// on the targeted enemy: the in-world chevron and the enemy-roster
// row highlight. Keeping the predicate in one place prevents them
// from drifting when the targeting rule changes.
func targetingEnemy(g core.GameState) bool {
	return g.Battle.Phase == core.BattlePlayer && g.Battle.ActionMode == core.ActionEnemyTarget
}

// targetingAlly is true when the player is choosing a party member to act
// on — either a heal-skill target or an item target. Used by the renderer
// to gate the friendly selection marker so it appears in both modes
// (audit-3 caught Item targeting silently dropping the marker because the
// check was specific to ActionPartyTarget).
func targetingAlly(g core.GameState) bool {
	return g.Battle.ActionMode == core.ActionPartyTarget || g.Battle.ActionMode == core.ActionItemTarget
}

type enemyStatusKind int

const (
	enemyStatusBurn enemyStatusKind = iota
	enemyStatusSleep
	enemyStatusPoison
	enemyStatusStun
	enemyStatusCount
)

type enemyStatusPillVisual struct {
	turns   func(*core.Enemy) int
	fill    rl.Color
	outline rl.Color
	prefix  string
	flicker bool
}

var enemyStatusPillVisuals = [enemyStatusCount]enemyStatusPillVisual{
	enemyStatusBurn:   {turns: func(e *core.Enemy) int { return e.BurnTurns }, fill: statusBurn, outline: statusBurnOutline, flicker: true},
	enemyStatusSleep:  {turns: func(e *core.Enemy) int { return e.SleepTurns }, fill: statusSleep, outline: statusSleepOutline, prefix: "Z"},
	enemyStatusPoison: {turns: func(e *core.Enemy) int { return e.PoisonTurns }, fill: statusPoison, outline: statusPoisonOutline, prefix: "P", flicker: true},
	enemyStatusStun:   {turns: func(e *core.Enemy) int { return e.StunTurns }, fill: statusStun, outline: statusStunOutline, prefix: "S"},
}

func init() {
	if len(enemyStatusPillVisuals) != int(enemyStatusCount) {
		panic(fmt.Sprintf("enemyStatusPillVisuals length %d != enemyStatusCount %d", len(enemyStatusPillVisuals), enemyStatusCount))
	}
	for i, v := range enemyStatusPillVisuals {
		if v.turns == nil {
			panic(fmt.Sprintf("enemyStatusPillVisuals[%d] has no turns reader", i))
		}
	}
}

// drawEnemyRoster shows the active pack at the top of the screen.
// Replaces the legacy floating target tooltip and the dense enemy info line
// that used to sit atop the bottom panel.
func drawEnemyRoster(g core.GameState, assets Resources) {
	if g.Battle.Phase == core.BattleWon || g.Battle.Phase == core.BattleLost {
		return
	}
	slots := visibleRosterSlots(g)
	if len(slots) == 0 {
		return
	}

	rowH := int32(60)
	// Inner pad replaces the old header band — the row content names
	// the enemies and shows their wound state; a tautological "GOBLINS 3/5"
	// title above them was just chrome.
	topPad := int32(18)
	padBottom := int32(18)
	w := int32(560)
	if len(slots) <= 1 {
		w = 440
	}
	h := topPad + int32(len(slots))*rowH + padBottom
	x := centerX(w)
	y := hudEdgePad + hudEdgePad/2

	drawCard(x, y, w, h, surfacePrimary, borderSoft, borderEnemy)

	// `targetable` controls the per-row yellow highlight in the enemy
	// roster. Shares the targetingEnemy predicate with the in-world
	// chevron so both yellow indicators turn on and off together —
	// when the timing bar arms (Phase → BattleAttackTiming), both go
	// dark, honouring "yellow cursor only when targeting."
	targetable := targetingEnemy(g)
	members := core.BattleMembers(&g)
	selectedSlot := core.SelectedEnemySlot(&g)

	for i, slot := range slots {
		// &members[slot] — pass the enemy by pointer so the per-row draw
		// doesn't copy the 496-byte Enemy (it embeds a full DefinitionOverride)
		// once per roster row per frame.
		enemy := &members[slot]
		rowY := y + topPad + int32(i)*rowH
		// HP is revealed once the kind is identified in the bestiary (5
		// kills or a Scan) — a kind-level fact, so every instance of a
		// known kind shows its numbers, not just the one that was scanned.
		known := g.Bestiary.Knows(enemy.Kind)
		drawEnemyRosterRow(assets.hudFont, enemy, x+14, rowY, w-28, rowH-8, targetable && slot == selectedSlot, !enemy.Alive, known)
	}
}

// rosterSlotsBuf is a package-private reusable buffer for visibleRosterSlots
// so the per-frame roster draw doesn't allocate a fresh slice every tick.
// raylib's draw loop is single-threaded, so re-slicing this isn't racy.
var rosterSlotsBuf = make([]int, 0, 16)

func visibleRosterSlots(g core.GameState) []int {
	rosterSlotsBuf = rosterSlotsBuf[:0]
	// Index-range: Enemy embeds a full DefinitionOverride, so a value-range
	// would copy ~496 bytes per member per frame just to read two bools.
	members := core.BattleMembers(&g)
	for i := range members {
		if !members[i].Alive && members[i].DeathFade <= 0 {
			continue
		}
		rosterSlotsBuf = append(rosterSlotsBuf, i)
	}
	return rosterSlotsBuf
}

func drawEnemyRosterRow(font rl.Font, enemy *core.Enemy, x, y, w, h int32, targeted, fading, known bool) {
	// Roster row tints follow the glass-token family — translucent
	// glass over the (also translucent) outer card body, so the
	// world hints through.
	bg := surfaceRosterRow
	border := borderRosterRow
	nameCol := textPrimary
	if fading {
		bg = surfaceRosterRowFaded
		border = borderDim
		nameCol = textDim
	}
	if targeted {
		// Brighter enemy-tinted fill than before so the current target
		// clearly stands apart from the idle rows.
		bg = core.MixColor(bg, surfaceEnemyTint, 0.9)
		border = borderEnemy
	}
	drawGlassPane(x, y, w, h, bg)
	if targeted {
		// Solid inner ring + pulsing wider ring — the same "this is the live
		// selection" treatment the party ribbon's active card uses, shared via
		// drawSelectionHalo so the two can't drift.
		drawSelectionHalo(x, y, w, h, borderEnemy, pulseHalo(), true)
	} else {
		drawSmallPanelOutline(x, y, w, h, border)
	}

	leftPad := int32(22)
	if targeted {
		leftPad = 34
		bx := float32(x) + 9
		cy := float32(y) + float32(h)/2
		col := fadeColor(borderEnemy, pulseHalo())
		drawArrowMarker(rl.NewVector2(bx, cy), 13, 0, 10, col)
	}

	condition, condCol := enemyHealthStyle(enemy)

	nameX := float32(x + leftPad)
	displayName := core.EnemyDisplayName(*enemy)
	drawTextWithShadow(font, displayName, nameX, float32(y+10), FontHeading, nameCol)

	// Health reads from the qualitative wound-state word by default —
	// exact enemy HP stays hidden until the kind is identified in the
	// bestiary (5 kills or a Scan), which the caller passes as `known`. A
	// known foe shows its real HP in claret just after the word; unknown
	// foes show the word alone. So: no HP bar; the condition word, plus the
	// revealed number once the party has earned (or scanned) the knowledge.
	condSize := FontSmall
	condY := float32(y) + float32(h) - condSize - 9
	drawTextWithShadow(font, condition, nameX, condY, condSize, condCol)
	if known {
		condW := rosterCondMeasureCache.measure(font, condition, condSize, 1).X
		drawTextWithShadow(font, enemyHPLabel(enemy.HP, enemy.MaxHP), nameX+condW+12, condY, condSize, barEnemyHP)
	}

	// Status pills, anchored to the right edge (no HP bar to tuck beside).
	pillW := float32(34)
	pillH := float32(28)
	rightEdge := float32(x+w) - 16
	pillX := rightEdge - pillW
	pillBaseY := float32(y) + (float32(h)-pillH)/2

	// Slot-stacked status pills. Walking the init-asserted visual table is what
	// lets a future fifth enemy status land as one appended row without
	// re-tuning any per-pill geometry. Limit to 4 visible — the panel doesn't
	// have vertical room for more without colliding with the row above.
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
			fill, p.outline, statusTurnsLabel(p.prefix, turns))
		slot++
	}
}

// statusTurnsLabel returns the pill label string for a status with N
// turns remaining. Burn / Sleep / Poison / Stun all render through
// these labels each frame for every afflicted enemy; pre-formatting
// the common 1..9 range and ""+1..9 variants avoids the per-frame
// fmt.Sprintf alloc on the enemy roster hot path (up to 4 statuses ×
// 6 enemies = 24 strings/frame in heavy combat).
func statusTurnsLabel(prefix string, turns int) string {
	if turns >= 0 && turns < len(statusTurnsCache) {
		labels := &statusTurnsCache[turns]
		switch prefix {
		case "":
			return labels.plain
		case "Z":
			return labels.zPrefix
		case "P":
			return labels.pPrefix
		case "S":
			return labels.sPrefix
		}
	}
	return fmt.Sprintf("%s%d", prefix, turns)
}

// rosterCondMeasureCache memoizes rl.MeasureTextEx for the handful of
// wound-state condition words an identified enemy's HP readout offsets
// against — without it every known enemy costs a cgo measure round-trip
// per roster frame.
var rosterCondMeasureCache measureCache

// enemyHPLabelCache memoizes the identified-enemy "HP/MaxHP" roster string
// per (hp, max) pair so the readout doesn't fmt.Sprintf per enemy per
// battle frame. Bounded: keys only span (0..MaxHP, MaxHP) pairs actually
// seen in play — a few hundred tiny strings across all kinds.
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

// statusTurnsCache holds pre-formatted "N", "ZN", "PN", "SN" strings
// for the small turn-count range that covers every realistic case
// plus tuning slack. Widened to 20 so a future tuning bump (longer
// burn/sleep ceilings) doesn't drop the path back into fmt.Sprintf.
var statusTurnsCache = func() [20]struct{ plain, zPrefix, pPrefix, sPrefix string } {
	var out [20]struct{ plain, zPrefix, pPrefix, sPrefix string }
	for i := range out {
		out[i].plain = fmt.Sprintf("%d", i)
		out[i].zPrefix = fmt.Sprintf("Z%d", i)
		out[i].pPrefix = fmt.Sprintf("P%d", i)
		out[i].sPrefix = fmt.Sprintf("S%d", i)
	}
	return out
}()

// drawStatusPill paints the shared status-pill silhouette: a small
// rounded fill pane + matching outline + a FontSmall single-line label.
// The "fill + outline + label" core both status surfaces use — the
// enemy-roster pill (drawEnemyStatusPill) and the Tome Stats-tab chip
// (render/panels.go) — extracted so the two pill silhouettes can't drift
// (FINDING #18). The two callers anchor + color the label differently
// (enemy: centered inkPrimary; Stats chip: left-aligned status tint), so
// the label placement is parameterized: `centered` true centers the
// label (with the enemy pill's +2 top inset); false left-aligns it with
// the Stats chip's +10/+4 inset. `labelCol` lets each pick its tone.
func drawStatusPill(font rl.Font, x, y, w, h float32, fill, outline rl.Color, label string, labelCol rl.Color, centered bool) {
	drawSmallPanel(int32(x), int32(y), int32(w), int32(h), fill)
	drawSmallPanelOutline(int32(x), int32(y), int32(w), int32(h), outline)
	if centered {
		drawTextCentered(font, label, x+w/2, y+2, FontSmall, labelCol)
	} else {
		drawTextWithShadow(font, label, x+10, y+4, FontSmall, labelCol)
	}
}

// drawEnemyStatusPill paints one rounded-rect status pill at the
// given coords with a centered single-line label. Single helper so
// every status (Burn / Sleep / Poison / Stun and future additions)
// shares the same silhouette — earlier code repeated the
// drawSmallPanel + drawSmallPanelOutline + drawTextCentered triple
// inline per status, drifting on font size and outline tone. Thin
// wrapper over drawStatusPill that pins the enemy pill's centered
// inkPrimary label so its appearance is unchanged.
func drawEnemyStatusPill(font rl.Font, x, y, w, h float32, fill, outline rl.Color, label string) {
	drawStatusPill(font, x, y, w, h, fill, outline, label, inkPrimary, true)
}

// combatLogTextPad is the horizontal inset between the combat-log
// inner panel edge and the rendered text. Both the wrap width
// (subtracts 2× pad) and the per-line text X (adds 1× pad) read
// this so the inset can't drift between the two seams — earlier
// the wrap used `innerW - 20` and the draw used `innerX + 10`
// with the coupling implicit.
const combatLogTextPad = int32(10)

// drawCombatLogSpine paints the binding-edge ornament along the left
// inside of the combat log pane: a thin wood-accent stripe terminated
// by gilt fleurons at both ends with a middle diamond pip flanked by
// horizontal "binding ties". Reads as a scribe's ledger spine — the
// dressing that ties the rolling text to the rest of the
// wood-and-glass HUD.
func drawCombatLogSpine(panelX, panelY, panelH int32) {
	const inset = int32(10)
	stripeX := panelX + inset
	stripeY := panelY + 18
	stripeH := panelH - 36
	rl.DrawRectangle(stripeX, stripeY, 2, stripeH, fadeColor(woodAccent, 0.75))
	centreX := float32(stripeX) + 1
	// Top + bottom fleurons mark the spine's termini — the
	// chapter-divider sigils anchoring the ledger.
	drawFleuron(centreX, float32(stripeY)-2, 3, giltDim)
	drawFleuron(centreX, float32(stripeY+stripeH)+2, 3, giltDim)
	// Mid-stripe diamond pip flanked by short horizontal binding
	// ties — reads as a leather thong wrapping the spine.
	midY := float32(stripeY) + float32(stripeH)*0.5
	drawDiamondPip(centreX, midY, 2.5, giltDim)
	tieCol := fadeColor(woodAccent, 0.45)
	rl.DrawRectangle(stripeX-4, int32(midY), 4, 1, tieCol)
	rl.DrawRectangle(stripeX+2, int32(midY), 4, 1, tieCol)
}

// combatLogVisualLine is the wrapped+styled product of one source log
// line. Lifted to package scope so the persistent cache can hold it
// across frames.
type combatLogVisualLine struct {
	text  string
	fresh bool
}

// combatLogCache memoizes the wrapped combat log between frames. The
// log only changes on setBattleMessage; without this cache,
// drawCombatLogPanel re-runs wrapTextLines + MeasureTextEx every
// frame even when nothing's new. Invalidates on log length change,
// last-line content change, or panel-geometry change.
var combatLogCache struct {
	visible      []combatLogVisualLine
	lastLogLen   int
	lastLastLine string
	lastInnerW   int32
	lastMaxLines int
}

func drawCombatLogPanel(g core.GameState, assets Resources) {
	// Combat log is the bottom-left HUD pane: tall, soft-edged glass
	// that the world bleeds through. No header label — the rolling
	// text is self-evident. The pane stretches to almost reach the
	// turn panel above, then floors at 160 px so it stays usable on
	// very short windows.
	w := int32(320)
	h := int32(300)
	x := hudEdgePad
	y := int32(PartyRibbonTopY()) - h - hudColumnGap

	if turnBottom := TurnPanelBottomY(g) + hudColumnGap; y < turnBottom {
		shrunkH := h - (turnBottom - y)
		if shrunkH < 160 {
			shrunkH = 160
		}
		h = shrunkH
		y = int32(PartyRibbonTopY()) - h - hudColumnGap
	}

	drawCard(x, y, w, h, surfacePrimary, borderSoft, borderSoft)
	// Ledger spine — a thin wood-accent stripe down the left inside
	// edge, dotted with three small pips. Reads as the bound-edge of
	// a scribe's ledger, anchoring the rolling text against the
	// world bleed-through.
	drawCombatLogSpine(x, y, h)

	// Without the header band the inner content fills the full pane
	// minus a small symmetric inset. Wood frame eats ~6 px on each
	// edge; an extra 8 px keeps text off the bevel; the spine on the
	// left adds another 6 px so the first column of text doesn't
	// collide with it.
	innerInset := int32(14)
	innerX := x + innerInset + 6
	innerY := y + innerInset
	innerW := w - 2*innerInset - 6
	innerH := h - 2*innerInset

	lines := g.Battle.Log
	if len(lines) == 0 && g.Battle.Message != "" {
		lines = []string{g.Battle.Message}
	}
	if len(lines) == 0 {
		return
	}

	lineH := int32(22)
	lineSize := FontSmall
	maxLines := int(innerH / lineH)
	if maxLines < 1 {
		maxLines = 1
	}

	wrapW := float32(innerW - 2*combatLogTextPad)
	visible := wrappedCombatLogLines(assets.hudFont, lines, innerW, maxLines, lineSize, wrapW)
	startY := innerY + innerH - int32(len(visible))*lineH - 6
	n := len(visible)
	for i, vl := range visible {
		// Fade-to-top: bottom line at full alpha (newest), top line
		// at ~0.18. Linear ramp in between so older entries gently
		// dissolve into the glass rather than getting cut off hard.
		var alpha float32 = 1
		if n > 1 {
			posT := float32(i) / float32(n-1) // 0 at top, 1 at bottom
			alpha = 0.18 + 0.82*posT
		}
		base := textMuted
		if vl.fresh {
			base = textPrimary
		}
		col := fadeColor(base, alpha)
		drawTextWithShadow(assets.hudFont, vl.text, float32(innerX+combatLogTextPad), float32(startY+int32(i)*lineH), lineSize, col)
	}
}

// wrappedCombatLogLines returns the visible wrapped log lines for the
// given source slice, reusing the cached result when the inputs are
// unchanged. The cache invalidates on (length, last-line, innerW,
// maxLines) — covering the two ways a log mutates (append, or
// trim+append on overflow) and any panel-geometry shift caused by the
// turn-panel collision guard at the top of drawCombatLogPanel.
func wrappedCombatLogLines(font rl.Font, lines []string, innerW int32, maxLines int, lineSize, wrapW float32) []combatLogVisualLine {
	lastLine := ""
	if len(lines) > 0 {
		lastLine = lines[len(lines)-1]
	}
	if combatLogCache.lastLogLen == len(lines) &&
		combatLogCache.lastLastLine == lastLine &&
		combatLogCache.lastInnerW == innerW &&
		combatLogCache.lastMaxLines == maxLines {
		return combatLogCache.visible
	}

	// Wrap each source line to the inner content width. We walk from
	// the NEWEST entry backward, building wraps in reverse, and stop
	// once we have enough visual lines to fill the panel. This avoids
	// re-wrapping older log lines that would just be sliced away —
	// with BattleLogMaxLines=40 sources averaging ~10 words, the old
	// "wrap-everything-then-slice" path made ~400 MeasureTextEx calls
	// per frame; this caps the work at ~maxLines × per-source words.
	reversed := combatLogCache.visible[:0]
	if cap(reversed) < maxLines {
		reversed = make([]combatLogVisualLine, 0, maxLines)
	}
	for i := len(lines) - 1; i >= 0 && len(reversed) < maxLines; i-- {
		fresh := i == len(lines)-1
		src := lines[i]
		wraps := wrapTextLines(font, src, lineSize, wrapW)
		if len(wraps) == 0 {
			// Empty source line — preserve as a blank gap so logged
			// "" entries (if any) still take a row.
			reversed = append(reversed, combatLogVisualLine{text: "", fresh: fresh})
			continue
		}
		// Append wraps in REVERSE so reversed[] stays "newest first."
		// Final reverse pass below restores chronological order.
		for j := len(wraps) - 1; j >= 0; j-- {
			reversed = append(reversed, combatLogVisualLine{text: wraps[j], fresh: fresh})
			if len(reversed) >= maxLines {
				break
			}
		}
	}
	// Reverse in place into chronological order for top-to-bottom
	// rendering. Doing this in place avoids a second slice allocation
	// (the prior code allocated `visible := make([]visualLine, len)`
	// just to reverse).
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	combatLogCache.visible = reversed
	combatLogCache.lastLogLen = len(lines)
	combatLogCache.lastLastLine = lastLine
	combatLogCache.lastInnerW = innerW
	combatLogCache.lastMaxLines = maxLines
	return reversed
}

// actionMenuSubLabelGap is the vertical gap (px) from a mode's verb
// heading to the sub-prompt / picker list beneath it in the action-menu
// panel. Named so the five action-mode arms share one offset.
const actionMenuSubLabelGap = 34

// Action-row geometry — the key-plate size + selection inset shared by the
// action menu and the skill/item picker rows so the two row styles align.
const (
	actionRowW = int32(284)
	actionRowH = int32(32)
)

// itemMenuSuffix caches the "x<N>  >" badge on the action menu's Item row,
// rebuilt only when the consumable count changes — not every frame the player
// sits in the (steady-state) action menu. Returns ">" when empty.
var itemSuffixCache struct {
	total int
	text  string
}

func itemMenuSuffix(total int) string {
	if total <= 0 {
		return ">"
	}
	if total != itemSuffixCache.total {
		itemSuffixCache.total = total
		itemSuffixCache.text = "x" + strconv.Itoa(total) + "  >"
	}
	return itemSuffixCache.text
}

// arrowPrompt caches the "A -> B" target prompt so the per-frame draw doesn't
// fmt.Sprintf while the player cycles the target. The two target modes
// (skill→ally, item→ally) share it since only one is active at a time.
var arrowPromptCache struct{ a, b, text string }

func arrowPrompt(a, b string) string {
	if a != arrowPromptCache.a || b != arrowPromptCache.b {
		arrowPromptCache.a, arrowPromptCache.b = a, b
		arrowPromptCache.text = a + " -> " + b
	}
	return arrowPromptCache.text
}

func drawActionMenuPanel(g core.GameState, assets Resources) {
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

	screenW, _ := screenSize()
	w := int32(340)
	// Taller panel — a name header now sits atop 4 action rows
	// (Attack/Skill/Defend/Item), and the item picker mode reuses this
	// same panel for its list. Anchors to the right edge using hudEdgePad
	// so the menu lines up with the other right-side HUD chrome. Turn
	// order is no longer to its right (moved to the left column under the
	// minimap), so the action panel is the rightmost battle-HUD element.
	h := int32(312)
	x := screenW - w - hudEdgePad
	y := int32(PartyRibbonTopY()) - h - hudColumnGap
	// Vertical collision guard: on a short-window resolution the
	// 280px panel might slip behind the top edge (y < 16). Floor the
	// top edge at hudEdgePad and shrink height to whatever fits
	// between hudEdgePad and PartyRibbonTopY. Same defensive pattern
	// the combat log uses against the left column. Floor height at
	// 160 so the action rows stay readable.
	if y < int32(hudEdgePad) {
		topPad := int32(hudEdgePad)
		bottomPad := int32(PartyRibbonTopY()) - int32(hudColumnGap)
		shrunkH := bottomPad - topPad
		if shrunkH < 160 {
			shrunkH = 160
		}
		h = shrunkH
		y = bottomPad - h
	}

	classCol := classAccent(member.Class)
	drawCard(x, y, w, h, surfacePrimary, borderActive, classCol)

	contentX := x + 22
	// Active member's name as the panel header, in their class color, so
	// whose turn it is is spelled out right where the player picks the
	// action — reinforcing the lifted/haloed party card and the glowing
	// sprite. A thin gilt rule divides the header from the action rows.
	drawTextWithShadow(assets.hudFont, member.Name, float32(contentX), float32(y+14), FontHeading, classCol)
	ruleY := y + 48
	drawGiltRule(x+18, ruleY, w-36, 1, 0.5)
	drawDiamondPip(float32(x+18), float32(ruleY), 2.4, fadeColor(giltDim, 0.85))
	drawDiamondPip(float32(x+w-18), float32(ruleY), 2.4, fadeColor(giltDim, 0.85))
	contentY := y + 58
	// subY is the baseline for the sub-prompt / picker list under the
	// mode's verb heading — one offset so the five action-mode arms below
	// can't drift on the heading-to-sublabel gap.
	subY := contentY + actionMenuSubLabelGap

	switch g.Battle.ActionMode {
	case core.ActionEnemyTarget:
		actionLabel := "Attack"
		if g.Battle.PendingSkill != core.SkillNone {
			actionLabel = core.SkillName(g.Battle.PendingSkill)
		}
		drawTextWithShadow(assets.hudFont, actionLabel, float32(contentX), float32(contentY), FontHeading, textPrimary)
		drawTextWithShadow(assets.hudFont, "Choose a target", float32(contentX), float32(subY), FontSmall, textLabel)
	case core.ActionPartyTarget:
		targetName := "Ally"
		if g.Battle.PartyTarget >= 0 && g.Battle.PartyTarget < len(g.Party) {
			targetName = g.Party[g.Battle.PartyTarget].Name
		}
		drawTextWithShadow(assets.hudFont, arrowPrompt(core.SkillName(g.Battle.PendingSkill), targetName), float32(contentX), float32(contentY), FontBody, textPrimary)
		drawTextWithShadow(assets.hudFont, "Choose an ally", float32(contentX), float32(subY), FontSmall, textLabel)
	case core.ActionItemMenu:
		drawTextWithShadow(assets.hudFont, "Items", float32(contentX), float32(contentY), FontHeading, textPrimary)
		drawItemMenuList(g, assets, contentX, subY)
	case core.ActionSkillMenu:
		drawTextWithShadow(assets.hudFont, "Skills", float32(contentX), float32(contentY), FontHeading, textPrimary)
		drawSkillMenuList(g, assets, contentX, subY, member)
	case core.ActionItemTarget:
		itemName := core.ItemInfo(g.Battle.PendingItem).Name
		targetName := "Ally"
		if g.Battle.PartyTarget >= 0 && g.Battle.PartyTarget < len(g.Party) {
			targetName = g.Party[g.Battle.PartyTarget].Name
		}
		drawTextWithShadow(assets.hudFont, arrowPrompt(itemName, targetName), float32(contentX), float32(contentY), FontBody, textPrimary)
		drawTextWithShadow(assets.hudFont, "Choose an ally", float32(contentX), float32(subY), FontSmall, textLabel)
	default:
		// Transient status line — populated by setBattleStatus to surface
		// validation errors that aren't real combat-log events (e.g.
		// "Swipe needs more MP."). Picker modes use their own hardcoded
		// prompt so we only render this in the action menu itself.
		if status := transientStatus(g); status != "" {
			drawTextWithShadow(assets.hudFont, status, float32(contentX), float32(contentY), FontSmall, classCol)
			contentY += 26
		}
		drawActionMenuOptions(g, assets, contentX, contentY, member)
	}

	// Input-hint footer (gamepad-first): the confirm/back affordances seated
	// over a faint gilt rule, so the action surface reads as an input prompt.
	// Skipped when the panel is shrunk on a short window (would collide with
	// the rows). Submenu entry is already cued by the per-row "▸" suffix.
	if h >= 260 {
		hintY := y + h - 28
		drawGiltRule(x+18, hintY-12, w-36, 1, 0.3)
		drawTextWithShadow(assets.hudFont, "A  Confirm       B  Back", float32(contentX), float32(hintY), FontSmall, textDim)
	}
}

// transientStatus returns Battle.Message when it's a "status" string that
// hasn't been logged yet (i.e. set via setBattleStatus, not setBattleMessage).
// Returns "" when Message is empty or matches the most recent log entry, so
// result/log messages don't render twice.
func transientStatus(g core.GameState) string {
	msg := g.Battle.Message
	if msg == "" {
		return ""
	}
	if n := len(g.Battle.Log); n > 0 && g.Battle.Log[n-1] == msg {
		return ""
	}
	return msg
}

func drawActionMenuOptions(g core.GameState, assets Resources, x, y int32, member core.PartyMember) {
	rowSpacing := int32(40)
	cursor := core.ActionRow(g.Battle.MenuIndex)
	// Push labels right so each row has a small icon column on the
	// left — the action-sigil that names what the row does without
	// reading the text.
	labelX := x + 26
	drawActionMenuRow(assets.hudFont, core.ActionRowAttack, x, labelX, y+int32(core.ActionRowAttack)*rowSpacing, "Attack", "", cursor == core.ActionRowAttack)
	_ = member
	drawActionMenuRow(assets.hudFont, core.ActionRowSkill, x, labelX, y+int32(core.ActionRowSkill)*rowSpacing, "Skill", ">", cursor == core.ActionRowSkill)
	itemSuffix := itemMenuSuffix(core.ConsumableCount(g.Inventory))
	drawActionMenuRow(assets.hudFont, core.ActionRowItem, x, labelX, y+int32(core.ActionRowItem)*rowSpacing, "Item", itemSuffix, cursor == core.ActionRowItem)
	drawActionMenuRow(assets.hudFont, core.ActionRowDefend, x, labelX, y+int32(core.ActionRowDefend)*rowSpacing, "Defend", "", cursor == core.ActionRowDefend)
}

// drawActionMenuRow wraps drawActionRow with an action-specific icon
// painted in a small left-side column. The icon picks the
// corresponding sigil — crossed blades (Attack), starburst (Skill),
// potion flask (Item), tower shield (Defend). Icon glyphs are
// procedural (no asset dependency) so they read crisp at any DPI.
func drawActionMenuRow(font rl.Font, row core.ActionRow, iconX, labelX, y int32, label, suffix string, selected bool) {
	// The "key" plate + label + suffix come from the shared drawActionRow (also
	// used by the skill/item submenu lists, so the whole action-input surface
	// reads as one stack-of-keys family). This row adds the icon medallion on
	// top, in the left gutter — the action sigil seated in a gilt-ringed rivet.
	drawActionRow(font, labelX, y, label, suffix, selected)
	iconCX := float32(iconX) + 9
	iconCY := float32(y) + 13
	drawIconMedallion(iconCX, iconCY, selected)
	iconCol := giltDim
	if selected {
		iconCol = giltBright
	}
	drawActionIcon(row, iconCX, iconCY, 7, iconCol)
}

// drawIconMedallion paints a small socketed medallion behind an action sigil:
// a dark seat, a gilt ring (candle-lit when selected), and a recessed dark face
// the icon draws onto — the "rivet with an engraved badge" look, built from
// stacked filled discs (no thin outline) so it reads as a solid fixture.
func drawIconMedallion(cx, cy float32, selected bool) {
	ring := fadeColor(giltDim, 0.85)
	if selected {
		ring = fadeColor(giltBright, 0.9*candleFlicker())
	}
	rl.DrawCircleV(rl.NewVector2(cx, cy), 12, fadeColor(woodDark, 0.95))
	rl.DrawCircleV(rl.NewVector2(cx, cy), 11, ring)
	rl.DrawCircleV(rl.NewVector2(cx, cy), 9.5, fadeColor(glassDeep, 0.96))
}

// actionIconDrawers dispatches each action-menu row to its sigil
// drawer. A fixed [core.ActionRowCount] array (not a switch) so adding
// a row forces a slot and the init below panics on a nil entry at
// startup, instead of a switch that silently draws a blank icon. Attack
// reuses the warrior class glyph ("strike" without text).
var actionIconDrawers = [core.ActionRowCount]func(cx, cy, r float32, col rl.Color){
	core.ActionRowAttack: drawClassGlyphWarrior,
	core.ActionRowSkill:  drawActionIconSkill,
	core.ActionRowItem:   drawActionIconItem,
	core.ActionRowDefend: drawActionIconDefend,
}

func init() {
	for row := core.ActionRow(0); int(row) < core.ActionRowCount; row++ {
		if actionIconDrawers[row] == nil {
			panic(fmt.Sprintf("render: ActionRow %d has no actionIconDrawers entry", int(row)))
		}
	}
}

// drawActionIcon dispatches to the per-action sigil drawer. Each glyph
// is sized by `r` (its half-extent) and tinted by `col` so the
// selection state propagates without duplicating a switch.
func drawActionIcon(row core.ActionRow, cx, cy, r float32, col rl.Color) {
	if int(row) < 0 || int(row) >= len(actionIconDrawers) {
		return
	}
	actionIconDrawers[row](cx, cy, r, col)
}

// drawActionIconSkill paints a four-rayed starburst with a bright
// inner pip — the "arcane spark" sigil for the Skill action.
func drawActionIconSkill(cx, cy, r float32, col rl.Color) {
	// Four cardinal rays — thin diamond shards from centre.
	rayHalf := r * 0.22
	// Vertical ray.
	drawTriangleCCW(rl.NewVector2(cx, cy-r), rl.NewVector2(cx-rayHalf, cy), rl.NewVector2(cx+rayHalf, cy), col)
	drawTriangleCCW(rl.NewVector2(cx, cy+r), rl.NewVector2(cx+rayHalf, cy), rl.NewVector2(cx-rayHalf, cy), col)
	// Horizontal ray.
	drawTriangleCCW(rl.NewVector2(cx-r, cy), rl.NewVector2(cx, cy+rayHalf), rl.NewVector2(cx, cy-rayHalf), col)
	drawTriangleCCW(rl.NewVector2(cx+r, cy), rl.NewVector2(cx, cy-rayHalf), rl.NewVector2(cx, cy+rayHalf), col)
	// Diagonal short rays — fainter, in the same colour but smaller.
	dr := r * 0.55
	for _, sign := range [4][2]float32{{1, 1}, {1, -1}, {-1, 1}, {-1, -1}} {
		dx := sign[0] * sqrt2Inv * dr
		dy := sign[1] * sqrt2Inv * dr
		drawDiamondPip(cx+dx, cy+dy, 1.5, col)
	}
	rl.DrawCircleV(rl.NewVector2(cx, cy), r*0.22, giltBright)
}

// drawActionIconItem paints a small apothecary-flask silhouette: a
// rectangular body, a tapered neck, and a stopper cap. Reads as
// "consumable" without needing a label.
func drawActionIconItem(cx, cy, r float32, col rl.Color) {
	// Stopper at top.
	stopperHalfW := r * 0.32
	rl.DrawRectangle(int32(cx-stopperHalfW), int32(cy-r), int32(stopperHalfW*2), int32(2), col)
	// Neck — narrower than the body, sitting under the stopper.
	neckHalfW := r * 0.22
	neckTop := cy - r + 2
	neckBottom := cy - r*0.45
	rl.DrawRectangle(int32(cx-neckHalfW), int32(neckTop), int32(neckHalfW*2), int32(neckBottom-neckTop), col)
	// Body — wider rounded rectangle (drawn as a square + caps via
	// triangles for the shoulders so the flask reads bulbous).
	bodyHalfW := r * 0.65
	bodyTop := neckBottom
	bodyBottom := cy + r*0.85
	rl.DrawRectangle(int32(cx-bodyHalfW), int32(bodyTop+3), int32(bodyHalfW*2), int32(bodyBottom-bodyTop-3), col)
	// Shoulders — two triangles smoothing the neck-to-body
	// transition.
	drawTriangleCCW(rl.NewVector2(cx-bodyHalfW, bodyTop+3), rl.NewVector2(cx-neckHalfW, bodyTop), rl.NewVector2(cx-neckHalfW, bodyTop+3), col)
	drawTriangleCCW(rl.NewVector2(cx+bodyHalfW, bodyTop+3), rl.NewVector2(cx+neckHalfW, bodyTop+3), rl.NewVector2(cx+neckHalfW, bodyTop), col)
	// Liquid line — a small bright cap inside the flask so it
	// reads as "potion" not "empty bottle."
	rl.DrawRectangle(int32(cx-bodyHalfW+2), int32(bodyBottom-r*0.35), int32(bodyHalfW*2-4), 2, giltBright)
}

// drawActionIconDefend paints a small tower-shield silhouette: a
// kite shape with a thicker top and a tapered point, plus a centre
// boss. Distinct from the equipment-slot heater so the action menu
// and the equipment screen don't read as the same sigil.
func drawActionIconDefend(cx, cy, r float32, col rl.Color) {
	// Slightly taller than the equipment heater. Top edge curves
	// inward via two small notches; the body tapers to a tip.
	topW := r * 1.2
	topH := r * 0.5
	// Top rectangle with notched corners (small triangle bites on
	// each top corner so the shield reads as "decorated").
	rl.DrawRectangle(int32(cx-topW/2), int32(cy-r), int32(topW), int32(topH), col)
	// Body taper.
	tip := rl.NewVector2(cx, cy+r*0.95)
	left := rl.NewVector2(cx-topW/2, cy-r+topH)
	right := rl.NewVector2(cx+topW/2, cy-r+topH)
	drawTriangleCCW(tip, right, left, col)
	// Vertical band running down the centre of the shield — gives
	// the silhouette structure beyond a plain heater.
	bandHalfW := r * 0.10
	rl.DrawRectangle(int32(cx-bandHalfW), int32(cy-r+topH-1), int32(bandHalfW*2), int32(r*1.05), fadeColor(col, 0.6))
	// Centre boss + bright pip.
	rl.DrawCircleV(rl.NewVector2(cx, cy-r*0.05), r*0.26, fadeColor(col, 0.6))
	rl.DrawCircleV(rl.NewVector2(cx, cy-r*0.05), r*0.12, giltBright)
}

// drawSkillMenuList renders the skill submenu — one row per learned
// skill with the MP cost on the right. Mirrors drawItemMenuList so the
// two submenus read as the same widget family.
// skillMenuSkillsBuf is the reused scratch slice for the in-battle skill
// picker's learned-skill list — refilled each frame the menu is up via
// LearnedSkillsInto instead of allocating a fresh slice + map, mirroring
// itemMenuStacksBuf.
var skillMenuSkillsBuf []core.SkillID

func drawSkillMenuList(g core.GameState, assets Resources, x, y int32, member core.PartyMember) {
	rowSpacing := int32(32)
	skillMenuSkillsBuf = core.LearnedSkillsInto(&member, skillMenuSkillsBuf)
	skills := skillMenuSkillsBuf
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
		drawActionRow(assets.hudFont, x, y+int32(i)*rowSpacing, label, suffix, g.Battle.SkillMenuIndex == i)
	}
}

// drawItemMenuList renders the inventory picker as a vertical list of
// "Name x Count" rows with the highlighted entry tinted by the selection
// border. Empty inventory falls through to a single "(no items)" hint row
// so the panel doesn't look broken if the player gets here somehow.
// itemMenuStacksBuf is the reused scratch slice for the in-battle item
// picker's live-consumable list — refilled each frame the menu is up
// instead of allocating fresh. Filtered to consumables so it lines up
// with updateItemMenu's LiveConsumables (equipment isn't usable in combat).
var itemMenuStacksBuf []core.ItemStack

func drawItemMenuList(g core.GameState, assets Resources, x, y int32) {
	// 32 (not 28) so each row's "key" plate (actionRowH=32, drawn by
	// drawActionRow) has clearance and the plates don't overlap.
	rowSpacing := int32(32)
	itemMenuStacksBuf = core.LiveConsumablesInto(g.Inventory, itemMenuStacksBuf)
	living := itemMenuStacksBuf
	if len(living) == 0 {
		drawTextWithShadow(assets.hudFont, "(no items)", float32(x), float32(y), FontSmall, textDim)
		return
	}
	for i, slot := range living {
		def := core.ItemInfo(slot.Kind)
		label := def.Name
		suffix := "x" + strconv.Itoa(slot.Count)
		drawActionRow(assets.hudFont, x, y+int32(i)*rowSpacing, label, suffix, g.Battle.ItemMenuIndex == i)
	}
}

func drawActionRow(font rl.Font, x, y int32, label, suffix string, selected bool) {
	// Every action / skill / item row is an engraved "key" plate so the whole
	// action-input surface reads as one stack-of-keys family: the selected one
	// gets the warm gilt selection plate (gilt spine + underline via
	// DrawSelectedRow), the rest a dark glass key with a soft wood rim.
	if selected {
		DrawSelectedRowI(x-8, y-4, actionRowW, actionRowH)
	} else {
		drawGlassPane(x-8, y-4, actionRowW, actionRowH, fadeColor(glassDeep, 0.5))
		drawSmallPanelOutline(x-8, y-4, actionRowW, actionRowH, fadeColor(woodMid, 0.45))
	}
	drawTextWithShadow(font, label, float32(x), float32(y), FontBody, textPrimary)
	if suffix != "" {
		size := FontSmall
		measure := measureActionRowSuffix(font, suffix)
		sx := float32(x) + float32(actionRowW) - measure.X - 22
		sy := float32(y) + 5
		drawTextWithShadow(font, suffix, sx, sy, size, textLabel)
	}
}

// actionRowSuffixMeasureCache memoizes the right-side suffix measurements
// drawActionRow takes on every menu row every frame ("▶", "5 MP", stack
// counts).
var actionRowSuffixMeasureCache measureCache

func measureActionRowSuffix(font rl.Font, suffix string) rl.Vector2 {
	return actionRowSuffixMeasureCache.measure(font, suffix, FontSmall, 1)
}

// enemyConditionColors is the wound-state tint for the enemy roster's
// condition label, indexed by core.EnemyCondition. A table (not a switch)
// so a newly-added condition surfaces as a zero-alpha (invisible) entry
// that the init assert below catches, rather than silently inheriting the
// default green.
var enemyConditionColors = [core.EnemyConditionCount]color.RGBA{
	core.EnemyUnharmed:     rl.NewColor(126, 231, 170, 255),
	core.EnemyScuffed:      rl.NewColor(208, 226, 128, 255),
	core.EnemyInjured:      rl.NewColor(246, 196, 91, 255),
	core.EnemyBadlyWounded: rl.NewColor(244, 126, 75, 255),
	core.EnemyNearDeath:    rl.NewColor(255, 78, 88, 255),
}

func init() {
	for c, col := range enemyConditionColors {
		if col.A == 0 {
			panic(fmt.Sprintf("render: enemyConditionColors missing entry for condition %d", c))
		}
	}
}

func enemyHealthStyle(enemy *core.Enemy) (string, color.RGBA) {
	condition := core.EnemyConditionFor(*enemy)
	return core.EnemyConditionLabel(condition), enemyConditionColors[condition]
}

// drawBattleSplash slams a banner with the encounter title at the top of the
// screen during the opening of a battle. Slides + scales in for impact.
func drawBattleSplash(g core.GameState, assets Resources) {
	members := core.BattleMembers(&g)
	if g.Battle.Splash <= 0 || g.Battle.EnemyIndex < 0 || g.Battle.EnemyIndex >= len(members) {
		return
	}
	progress := core.BattleSplashDuration - g.Battle.Splash
	if progress < 0 {
		progress = 0
	}
	enterT := progress / 0.18
	if enterT > 1 {
		enterT = 1
	}
	exitT := float32(1)
	if g.Battle.Splash < 0.32 {
		exitT = g.Battle.Splash / 0.32
	}
	intro := easeOutBack(enterT)
	overall := exitT

	text := core.BattleEncounterTitle(&g)
	subtitle := splashSubtitle(g)
	// Battle splash uses FontTitle for the encounter name and
	// FontBody for the subtitle — per UI_STANDARDS.md "Type" the
	// splash banner is the highest-emphasis transient surface.
	titleSize := FontTitle
	subSize := FontBody
	spacing := FontSpacingTitle

	titleMeasure := rl.MeasureTextEx(assets.hudFont, text, titleSize, spacing)
	subMeasure := rl.NewVector2(0, 0)
	if subtitle != "" {
		subMeasure = rl.MeasureTextEx(assets.hudFont, subtitle, subSize, 1)
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

	drawPanel(int32(bgX), int32(bgY), int32(bgW), int32(bgH), rl.NewColor(8, 10, 16, bgAlpha))
	drawPanelOutline(int32(bgX), int32(bgY), int32(bgW), int32(bgH), fadeColor(borderEnemy, overall))

	titleX := cx - titleW/2
	titleY := bgY + padY
	// Splash needs fade-driven shadow alphas (titleAlpha/subAlpha track the
	// banner's overall opacity) plus a heavier 3px drop offset and the title
	// letter-spacing — drawTextWithShadowStyle takes all three (custom shadow
	// color, offset, spacing), which is exactly what it documents itself for.
	drawTextWithShadowStyle(assets.hudFont, text, titleX, titleY, titleSize*scale, spacing*scale,
		rl.NewColor(248, 232, 198, titleAlpha), rl.NewColor(0, 0, 0, titleAlpha), 3, 3)

	if subtitle != "" {
		subX := cx - subMeasure.X/2
		subY := titleY + titleH + gap
		// Gilt rule with a centred fleuron between the encounter
		// title and the subtitle — the chapter-divider flourish
		// 90s D&D PC RPGs used between an event banner and its
		// body line. Width = 60 % of the subtitle for taste; alpha
		// rides the splash's overall fade so it disappears with
		// the rest of the banner.
		ruleW := subMeasure.X * 0.6
		ruleY := subY - 4
		ruleCol := fadeColor(giltDim, overall)
		drawSplitRule(cx-ruleW/2, cx+ruleW/2, cx, ruleY, 8, ruleCol)
		drawFleuron(cx, ruleY, 3, fadeColor(giltBright, overall))
		drawTextWithShadowStyle(assets.hudFont, subtitle, subX, subY, subSize, 1,
			colorWithAlpha(borderEnemy, subAlpha), rl.NewColor(0, 0, 0, subAlpha), 1, 1)
	}
}

// splashSubtitleCache memoizes the formatted subtitle by (count, noun) so
// the ~40 frames a splash is visible don't each pay a fmt.Sprintf — same
// rebuild-on-change pattern as hud.go's goldReadout cache.
var splashSubtitleCache struct {
	count int
	noun  string
	text  string
}

func splashSubtitle(g core.GameState) string {
	count := len(core.BattleMembers(&g))
	if count <= 1 {
		return "Hostile encounter"
	}
	def := core.BattleEnemyInfo(&g)
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

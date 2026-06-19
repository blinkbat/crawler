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
// region (top-center roster, bottom-left log, bottom-center action, left column
// turn order) so they never compete for the same real estate. The action
// log is pinned bottom-left through every phase — including the timing
// minigame — so the player can keep reading the last few events while
// they press. The action menu yields to the bar (it has nothing useful
// to show during press/charge anyway).
func drawBattleHUD(g *core.GameState, assets Resources) {
	drawEnemyRoster(g, assets)
	drawActionLogPanel(g, assets)
	if !timingActive(g) {
		drawActionMenuPanel(g, assets)
	}
}

// timingActive reports whether the timed-hit bar is currently the focus of
// the HUD. Used to hide panels that share its strip.
func timingActive(g *core.GameState) bool {
	return g.Battle.Phase == core.BattleAttackTiming || g.Battle.Phase == core.BattleEnemyTiming
}

// inPlayerTurn reports whether the current phase is "the player is acting" —
// either the menu/target picker or the resolving timing bar. Visual indicators
// for the active actor + chosen target should persist through the bar so the
// player keeps their bearings, instead of flickering off the moment the bar
// arms and back on when it resolves.
func inPlayerTurn(g *core.GameState) bool {
	return g.Battle.Phase == core.BattlePlayer || g.Battle.Phase == core.BattleAttackTiming
}

// targetingEnemy reports whether the player is currently in the
// "choose an enemy" target phase — Phase MUST be BattlePlayer (drops
// the moment the timing bar arms) and ActionMode == ActionEnemyTarget.
// Single source for the two render gates that overlay a "yellow cursor"
// on the targeted enemy: the in-world chevron and the enemy-roster
// row highlight. Keeping the predicate in one place prevents them
// from drifting when the targeting rule changes.
func targetingEnemy(g *core.GameState) bool {
	return g.Battle.Phase == core.BattlePlayer && g.Battle.ActionMode == core.ActionEnemyTarget
}

// targetingAlly is true when the player is choosing a party member to act
// on — either a heal-skill target or an item target. Used by the renderer
// to gate the friendly selection marker so it appears in both modes
// (audit-3 caught Item targeting silently dropping the marker because the
// check was specific to ActionPartyTarget).
func targetingAlly(g *core.GameState) bool {
	return g.Battle.ActionMode == core.ActionPartyTarget || g.Battle.ActionMode == core.ActionItemTarget
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
	// glyph paints the status's symbol, reusing the SAME vector glyphs the
	// party cards draw (drawStatusGlyph*) so both surfaces read with icons,
	// not a per-side letter code. Dark ink on the bright pill fill.
	glyph   func(cx, cy, r float32, col rl.Color)
	flicker bool
}

var enemyStatusPillVisuals = [enemyStatusCount]enemyStatusPillVisual{
	enemyStatusBurn:   {turns: func(e *core.Enemy) int { return e.BurnTurns }, fill: statusBurn, outline: statusBurnOutline, glyph: drawStatusGlyphBurn, flicker: true},
	enemyStatusSleep:  {turns: func(e *core.Enemy) int { return e.SleepTurns }, fill: statusSleep, outline: statusSleepOutline, glyph: drawStatusGlyphAsleep},
	enemyStatusPoison: {turns: func(e *core.Enemy) int { return e.PoisonTurns }, fill: statusPoison, outline: statusPoisonOutline, glyph: drawStatusGlyphPoisoned, flicker: true},
	enemyStatusBleed:  {turns: func(e *core.Enemy) int { return e.BleedTurns }, fill: statusBleed, outline: statusBleedOutline, glyph: drawStatusGlyphBleed, flicker: true},
	enemyStatusStun:   {turns: func(e *core.Enemy) int { return e.StunTurns }, fill: statusStun, outline: statusStunOutline, glyph: drawStatusGlyphStunned},
}

func init() {
	if len(enemyStatusPillVisuals) != int(enemyStatusCount) {
		panic(fmt.Sprintf("enemyStatusPillVisuals length %d != enemyStatusCount %d", len(enemyStatusPillVisuals), enemyStatusCount))
	}
	for i, v := range enemyStatusPillVisuals {
		if v.turns == nil {
			panic(fmt.Sprintf("enemyStatusPillVisuals[%d] has no turns reader", i))
		}
		if v.glyph == nil {
			panic(fmt.Sprintf("enemyStatusPillVisuals[%d] has no glyph — add the row", i))
		}
	}
}

// drawEnemyRoster shows the active pack at the top of the screen.
// Replaces the legacy floating target tooltip and the dense enemy info line
// that used to sit atop the bottom panel.
func drawEnemyRoster(g *core.GameState, assets Resources) {
	if g.Battle.Phase == core.BattleWon || g.Battle.Phase == core.BattleLost {
		return
	}
	slots := visibleRosterSlots(g)
	if len(slots) == 0 {
		return
	}

	rowH := rosterRowH
	// Inner pad replaces the old header band — the row content names
	// the enemies and shows their wound state; a tautological "GOBLINS 3/5"
	// title above them was just chrome.
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

	// `targetable` controls the per-row yellow highlight in the enemy
	// roster. Shares the targetingEnemy predicate with the in-world
	// chevron so both yellow indicators turn on and off together —
	// when the timing bar arms (Phase → BattleAttackTiming), both go
	// dark, honouring "yellow cursor only when targeting."
	targetable := targetingEnemy(g)
	members := core.BattleMembers(g)
	selectedSlot := core.SelectedEnemySlot(g)

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

func visibleRosterSlots(g *core.GameState) []int {
	rosterSlotsBuf = rosterSlotsBuf[:0]
	// Index-range: Enemy embeds a full DefinitionOverride, so a value-range
	// would copy ~496 bytes per member per frame just to read two bools.
	members := core.BattleMembers(g)
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

	leftPad := hudContentInsetX
	if targeted {
		leftPad = 34
		bx := float32(x) + 9
		cy := float32(y) + float32(h)/2
		col := fadeColor(borderEnemy, pulseHalo())
		drawArrowMarker(rl.NewVector2(bx, cy), 13, 0, 10, col)
	}

	condition, condCol := enemyHealthStyle(enemy)

	nameX := float32(x + leftPad)
	displayName := core.EnemyName(enemy)
	drawEngravedText(font, displayName, nameX, float32(y+10), FontHeading, nameCol)

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
	// lets a new enemy status land as one appended row without re-tuning any
	// per-pill geometry. Pills stack upward from pillBaseY; in practice an enemy
	// shows 1-2 at once (the five kinds rarely co-occur — Sleep/Stun skip turns,
	// so a fully-stacked Burn+Sleep+Poison+Bleed+Stun is unreachable in play).
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

// statusTurnsLabel returns the bare turn-count pill label for a status with N
// turns remaining. Burn / Sleep / Poison / Stun all render through this each
// frame for every afflicted enemy (up to 4 statuses × 6 enemies = 24
// strings/frame in heavy combat), so it reads the shared statusTurnDigit cache
// rather than re-Sprintf'ing. Status identity is conveyed by the glyph + fill,
// not a letter prefix, so the label is numbers only.
func statusTurnsLabel(turns int) string {
	return statusTurnDigit(turns)
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

// drawEnemyStatusPill paints one rounded-rect status pill: the colored
// fill + outline (status identity by color, unchanged silhouette), a vector
// GLYPH on the left — the same drawStatusGlyph* symbol the party cards use, so
// the two surfaces read alike instead of the roster spelling out letter codes —
// and the bare turn count on the right. Glyph + number are drawn in the dark
// glyph ink (statusGlyphDark) for contrast against the bright status fill.
func drawEnemyStatusPill(font rl.Font, x, y, w, h float32, fill, outline rl.Color, glyph func(cx, cy, r float32, col rl.Color), turnsLabel string) {
	// Fill + outline silhouette comes from the shared drawStatusPill core (empty
	// label — this pill anchors its glyph + turn count itself, not centered).
	drawStatusPill(font, x, y, w, h, fill, outline, "", statusGlyphDark, true)
	if glyph != nil {
		glyph(x+w*0.30, y+h*0.5, h*0.28, statusGlyphDark)
	}
	drawTextCentered(font, turnsLabel, x+w*0.72, y+2, FontSmall, statusGlyphDark)
}

// actionLogTextPad is the horizontal inset between the action-log
// inner panel edge and the rendered text. Both the wrap width
// (subtracts 2× pad) and the per-line text X (adds 1× pad) read
// this so the inset can't drift between the two seams — earlier
// the wrap used `innerW - 20` and the draw used `innerX + 10`
// with the coupling implicit.
const actionLogTextPad = int32(10)

// actionLogSpineInset is the symmetric top/bottom inset of the binding-edge
// spine stripe from the action-log pane (the stripe runs panelH - 2×inset).
// Named so the spine's vertical margin isn't a pair of bare 18 / 36 literals.
const actionLogSpineInset = int32(18)

// Wood-accent alphas for the action-log spine: the main binding stripe and the
// dimmer "binding tie" cross-marks. Named so the spine's two fade levels tune
// in one place instead of as inline fadeColor magic numbers.
const (
	actionLogSpineAlpha = float32(0.75)
	actionLogTieAlpha   = float32(0.45)
)

// drawActionLogSpine paints the binding-edge ornament along the left
// inside of the action log pane: a thin wood-accent stripe terminated
// by gilt fleurons at both ends with a middle diamond pip flanked by
// horizontal "binding ties". Reads as a scribe's ledger spine — the
// dressing that ties the rolling text to the rest of the
// wood-and-glass HUD.
func drawActionLogSpine(panelX, panelY, panelH int32) {
	stripeX := panelX + actionLogTextPad
	stripeY := panelY + actionLogSpineInset
	stripeH := panelH - 2*actionLogSpineInset
	rl.DrawRectangle(stripeX, stripeY, 2, stripeH, fadeColor(woodAccent, actionLogSpineAlpha))
	centreX := float32(stripeX) + 1
	// Top + bottom fleurons mark the spine's termini — the
	// chapter-divider sigils anchoring the ledger.
	drawFleuron(centreX, float32(stripeY)-2, 3, giltDim)
	drawFleuron(centreX, float32(stripeY+stripeH)+2, 3, giltDim)
	// Mid-stripe diamond pip flanked by short horizontal binding
	// ties — reads as a leather thong wrapping the spine.
	midY := float32(stripeY) + float32(stripeH)*0.5
	drawDiamondPip(centreX, midY, 2.5, giltDim)
	tieCol := fadeColor(woodAccent, actionLogTieAlpha)
	rl.DrawRectangle(stripeX-4, int32(midY), 4, 1, tieCol)
	rl.DrawRectangle(stripeX+2, int32(midY), 4, 1, tieCol)
}

// actionLogVisualLine is the wrapped+styled product of one source log
// line. Lifted to package scope so the persistent cache can hold it
// across frames.
type actionLogVisualLine struct {
	text  string
	fresh bool
}

// actionLogCache memoizes the wrapped action log between frames. The
// log only changes on setBattleMessage; without this cache,
// drawActionLogPanel re-runs wrapTextLines + MeasureTextEx every
// frame even when nothing's new. Invalidates on log length change,
// last-line content change, or panel-geometry change.
var actionLogCache struct {
	visible      []actionLogVisualLine
	lastLogLen   int
	lastLastLine string
	lastInnerW   int32
	lastMaxLines int
}

// statusLineScratch is the reusable 1-slot backing for the "show StatusMessage
// when the log is empty" path, so drawActionLogPanel doesn't allocate a slice
// per frame. Safe as package state because raylib draw is single-threaded.
var statusLineScratch [1]string

// shrinkPinnedToBottom resolves the height/top-edge for a bottom-pinned HUD pane
// whose top would collide with topLimit: it shrinks the pane (floored at
// hudPanelMinH) while keeping its bottom edge at bottomY. The two bottom HUD
// panes (action log vs action menu) share this; only their top limit differs.
func shrinkPinnedToBottom(bottomY, topLimit int32) (h, y int32) {
	h = bottomY - topLimit
	if h < hudPanelMinH {
		h = hudPanelMinH
	}
	return h, bottomY - h
}

// drawActionLogPanel paints the rolling ACTION LOG — the bottom-left HUD pane
// shown both in combat and during exploration (g.ActionLog persists across the
// two). The name is historical; it's no longer combat-only.
func drawActionLogPanel(g *core.GameState, assets Resources) {
	// Bottom-left HUD pane: tall, soft-edged glass that the world bleeds
	// through. No header label — the rolling text is self-evident. The pane's
	// BOTTOM edge pins to the screen bottom (hudEdgePad margin); it stretches
	// up toward the turn panel, then floors at 160 px so it stays usable on
	// very short windows.
	w := actionLogW
	h := actionLogH
	_, screenH := screenSize()
	x := hudEdgePad
	bottomY := screenH - hudEdgePad
	y := bottomY - h

	// Top collision guard against the turn panel above: if the pane would
	// overlap it, shrink height (floored at hudPanelMinH) while keeping the
	// bottom edge pinned to the screen bottom.
	if turnBottom := TurnPanelBottomY(g) + hudColumnGap; y < turnBottom {
		h, y = shrinkPinnedToBottom(bottomY, turnBottom)
	}

	drawPanelCard(x, y, w, h)
	// Ledger spine — a thin wood-accent stripe down the left inside
	// edge, dotted with three small pips. Reads as the bound-edge of
	// a scribe's ledger, anchoring the rolling text against the
	// world bleed-through.
	drawActionLogSpine(x, y, h)

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

	lineH := int32(22)
	lineSize := FontSmall

	// Ruled-parchment lines — a whisper-faint hairline at the BOTTOM of each
	// line slot, so the pane reads as a scribe's ruled ledger page even before
	// (and after) text fills it. Bottom-anchored at the same -6 footing the
	// text loop uses, so entries always sit exactly ON their rule no matter
	// how many lines are visible. Inset a touch from both edges so the rules
	// read as page ruling, not frame strokes.
	ruleX := innerX + 2
	ruleW := innerW - 10
	for ry := innerY + innerH - 6; ry > innerY+lineH/2; ry -= lineH {
		rl.DrawRectangle(ruleX, ry, ruleW, 1, fadeColor(inkDim, 0.13))
	}

	lines := g.ActionLog
	if len(lines) == 0 && g.StatusMessage != "" {
		// Reuse a package-level 1-slot buffer rather than allocating a new slice
		// each frame the log is empty but a status prompt is up (draw is single-
		// threaded, so the shared scratch is safe).
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
		// Fade-to-top: bottom line (newest) at full alpha, oldest line at 0.5.
		// Gentle linear ramp so older entries recede into the glass without
		// becoming unreadable — the floor was 0.18, which faded the top lines
		// nearly to nothing (they "fade too much").
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

// wrappedActionLogLines returns the visible wrapped log lines for the
// given source slice, reusing the cached result when the inputs are
// unchanged. The cache invalidates on (length, last-line, innerW,
// maxLines) — covering the two ways a log mutates (append, or
// trim+append on overflow) and any panel-geometry shift caused by the
// turn-panel collision guard at the top of drawActionLogPanel.
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

	// Wrap each source line to the inner content width. We walk from
	// the NEWEST entry backward, building wraps in reverse, and stop
	// once we have enough visual lines to fill the panel. This avoids
	// re-wrapping older log lines that would just be sliced away —
	// with ActionLogMaxLines=40 sources averaging ~10 words, the old
	// "wrap-everything-then-slice" path made ~400 MeasureTextEx calls
	// per frame; this caps the work at ~maxLines × per-source words.
	reversed := actionLogCache.visible[:0]
	if cap(reversed) < maxLines {
		reversed = make([]actionLogVisualLine, 0, maxLines)
	}
	for i := len(lines) - 1; i >= 0 && len(reversed) < maxLines; i-- {
		fresh := i == len(lines)-1
		src := lines[i]
		wraps := wrapTextLines(font, src, lineSize, wrapW)
		if len(wraps) == 0 {
			// Empty source line — preserve as a blank gap so logged
			// "" entries (if any) still take a row.
			reversed = append(reversed, actionLogVisualLine{text: "", fresh: fresh})
			continue
		}
		// Append wraps in REVERSE so reversed[] stays "newest first."
		// Final reverse pass below restores chronological order.
		for j := len(wraps) - 1; j >= 0; j-- {
			reversed = append(reversed, actionLogVisualLine{text: wraps[j], fresh: fresh})
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
	actionLogCache.visible = reversed
	actionLogCache.lastLogLen = len(lines)
	actionLogCache.lastLastLine = lastLine
	actionLogCache.lastInnerW = innerW
	actionLogCache.lastMaxLines = maxLines
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

// arrowPrompt caches the "A → B" target prompt so the per-frame draw doesn't
// fmt.Sprintf while the player cycles the target. The two target modes
// (skill→ally, item→ally) share it since only one is active at a time. The
// arrow is the → glyph (richtext.go's symGlyphs draws it procedurally via
// drawTextWithShadow) rather than a spelled-out "->", per the glyph-first hints.
var arrowPromptCache struct{ a, b, text string }

func arrowPrompt(a, b string) string {
	if a != arrowPromptCache.a || b != arrowPromptCache.b {
		arrowPromptCache.a, arrowPromptCache.b = a, b
		arrowPromptCache.text = a + " → " + b
	}
	return arrowPromptCache.text
}

// drawAllyTargetPrompt paints the "verb -> ally / Choose an ally" two-line
// prompt shared by the skill-target and item-target action arms: it resolves the
// selected ally's name (falling back to "Ally" when PartyTarget is out of range)
// and renders the arrowPrompt headline above the sub-label. `verb` is the skill
// or item name driving the arrow prompt.
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
	// Taller panel — a name header now sits atop 4 action rows
	// (Attack/Skill/Defend/Item), and the item picker mode reuses this
	// same panel for its list. Pins to the bottom-RIGHT corner: right edge at
	// hudEdgePad from the screen edge, bottom edge at hudEdgePad from the
	// screen bottom.
	h := actionMenuH
	x := screenW - w - hudEdgePad
	bottomY := screenH - hudEdgePad
	y := bottomY - h
	// Vertical collision guard: on a short-window resolution the panel might
	// slip behind the top edge. Floor the top edge at hudEdgePad and shrink
	// height while keeping the bottom edge pinned to the screen bottom. Floor
	// height at hudPanelMinH so the action rows stay readable.
	if y < hudEdgePad {
		h, y = shrinkPinnedToBottom(bottomY, hudEdgePad)
	}

	classCol := classAccent(member.Class)
	drawCard(x, y, w, h, surfacePrimary, borderActive, classCol)

	contentX := x + hudContentInsetX
	// Active member's name as the panel header, in their class color, so
	// whose turn it is is spelled out right where the player picks the
	// action — reinforcing the lifted/haloed party card and the glowing
	// sprite. A thin gilt rule divides the header from the action rows.
	drawEngravedText(assets.hudFont, member.Name, float32(contentX), float32(y+14), FontHeading, classCol)
	ruleY := y + 48
	drawPipCappedRule(x+18, ruleY, w-36, fadeColor(giltBright, 0.5), 2.4, fadeColor(giltDim, 0.85))
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
		drawEngravedText(assets.hudFont, actionLabel, float32(contentX), float32(contentY), FontHeading, textPrimary)
		drawTextWithShadow(assets.hudFont, "Choose a target", float32(contentX), float32(subY), FontSmall, textLabel)
	case core.ActionPartyTarget:
		drawAllyTargetPrompt(g, assets, core.SkillName(g.Battle.PendingSkill), contentX, contentY, subY)
	case core.ActionItemMenu:
		drawEngravedText(assets.hudFont, "Items", float32(contentX), float32(contentY), FontHeading, textPrimary)
		drawItemMenuList(g, assets, contentX, subY)
	case core.ActionSkillMenu:
		drawEngravedText(assets.hudFont, "Skills", float32(contentX), float32(contentY), FontHeading, textPrimary)
		drawSkillMenuList(g, assets, contentX, subY)
	case core.ActionItemTarget:
		drawAllyTargetPrompt(g, assets, core.ItemInfo(g.Battle.PendingItem).Name, contentX, contentY, subY)
	case core.ActionFleeConfirm:
		drawEngravedText(assets.hudFont, "Flee", float32(contentX), float32(contentY), FontHeading, textPrimary)
		drawTextWithShadow(assets.hudFont, "Retreat from this battle?", float32(contentX), float32(subY), FontSmall, textLabel)
	default:
		// Transient status line — populated by setBattleStatus to surface
		// validation errors that aren't real action-log events (e.g.
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
	if h >= actionMenuHintMinH {
		hintY := y + h - 28
		drawGiltRule(x+18, hintY-12, w-36, 1, 0.3)
		DrawHintBarLeft(assets.hudFont, []HintSeg{
			Hint("Confirm", GlyphA),
			Hint("Back", GlyphB),
		}, float32(contentX), float32(hintY), FontSmall)
	}
}

// transientStatus returns StatusMessage when it's a "status" string that
// hasn't been logged yet (i.e. set via setBattleStatus, not setBattleMessage).
// Returns "" when Message is empty or matches the most recent log entry, so
// result/log messages don't render twice.
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

func init() {
	// drawActionMenuOptions hand-lists exactly the five action rows because each
	// paints a bespoke per-row icon (so it isn't a generic count-driven loop). If
	// a new ActionRow is ever added, fail loudly at startup — the menu must be
	// updated rather than silently omit the row. (Matches the parallel-table
	// init-assert discipline used elsewhere in the codebase.)
	if core.ActionRowCount != 5 {
		panic(fmt.Sprintf("render: drawActionMenuOptions lists 5 rows but core.ActionRowCount == %d — add the new row to the menu", core.ActionRowCount))
	}
}

func drawActionMenuOptions(g *core.GameState, assets Resources, x, y int32, member core.PartyMember) {
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
	drawActionMenuRow(assets.hudFont, core.ActionRowFlee, x, labelX, y+int32(core.ActionRowFlee)*rowSpacing, "Flee", "", cursor == core.ActionRowFlee)
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

// drawMedallion paints the shared socketed-medallion primitive both the action
// sigil rivet (drawIconMedallion) and the party class badge (drawClassMedallion)
// are built on: an optional contact-shadow disc, a dark woodDark outer seat, a
// gilt ring, and a recessed glass face the caller draws its sigil onto. The
// three band radii are passed explicitly so each caller keeps its own
// proportions; a shadowR > 0 paints a contact-shadow disc at that radius first;
// `pip`, if non-nil, runs last for caller-specific embellishment (the class
// badge's highlight + corner pip). Built from stacked filled discs (no thin
// outline) so it reads as a solid fixture.
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

// drawIconMedallion paints a small socketed medallion behind an action sigil:
// a dark seat, a gilt ring (candle-lit when selected), and a recessed dark face
// the icon draws onto — the "rivet with an engraved badge" look.
func drawIconMedallion(cx, cy float32, selected bool) {
	ring := fadeColor(giltDim, 0.85)
	if selected {
		ring = fadeColor(giltBright, 0.9*candleFlicker())
	}
	drawMedallion(cx, cy, 12, 11, 9.5,
		fadeColor(woodDark, 0.95), ring, fadeColor(glassDeep, 0.96), 0, nil)
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
	core.ActionRowFlee:   drawActionIconFlee,
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
// selection state propagates without duplicating a switch. row indexes
// the init-asserted [ActionRowCount] table directly — an out-of-range
// row panics on the bounds check (loud, like slotIconForKind) rather
// than silently drawing nothing.
func drawActionIcon(row core.ActionRow, cx, cy, r float32, col rl.Color) {
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

// drawActionIconFlee draws a "dash away" sigil — a double chevron pointing right
// (>>), reading as "break away / exit fast." Thick line segments so it stays
// crisp at any DPI, matching the procedural-glyph family; the leading chevron is
// brighter for a sense of motion.
func drawActionIconFlee(cx, cy, r float32, col rl.Color) {
	thick := r * 0.34
	h := r * 0.62
	chevron := func(tipX float32, c rl.Color) {
		tip := rl.NewVector2(tipX, cy)
		rl.DrawLineEx(rl.NewVector2(tipX-h, cy-h), tip, thick, c)
		rl.DrawLineEx(tip, rl.NewVector2(tipX-h, cy+h), thick, c)
	}
	chevron(cx-r*0.15, fadeColor(col, 0.6)) // trailing chevron, dimmer
	chevron(cx+r*0.5, col)                  // leading chevron
}

// drawSkillMenuList renders the skill submenu — one row per learned
// skill with the MP cost on the right. Mirrors drawItemMenuList so the
// two submenus read as the same widget family. The list itself is built
// by the battle update path into g.Battle.SkillMenuList (the same frame,
// before this draw); reading it here avoids re-walking the skill tree a
// second time.
func drawSkillMenuList(g *core.GameState, assets Resources, x, y int32) {
	rowSpacing := int32(32)
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
		drawActionRow(assets.hudFont, x, y+int32(i)*rowSpacing, label, suffix, g.Battle.SkillMenuIndex == i)
	}
}

// drawItemMenuList renders the inventory picker as a vertical list of
// "Name x Count" rows with the highlighted entry tinted by the selection
// border. Empty inventory falls through to a single "(no items)" hint row
// so the panel doesn't look broken if the player gets here somehow. The
// live-consumable list is built by the battle update path into
// g.Battle.ItemMenuList (the same frame, before this draw); reading it here
// avoids a second inventory scan. Filtered to consumables there so it lines
// up with updateItemMenu's picker (equipment isn't usable in combat).
func drawItemMenuList(g *core.GameState, assets Resources, x, y int32) {
	// 32 (not 28) so each row's "key" plate (actionRowH=32, drawn by
	// drawActionRow) has clearance and the plates don't overlap.
	rowSpacing := int32(32)
	living := g.Battle.ItemMenuList
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

// Splash ease windows, in seconds, carved out of core.BattleSplashDuration:
// splashEnterDur is the lead-in over which the banner eases + scales in from
// the top; splashExitDur is the tail-out window (measured from remaining
// Splash time) over which it fades back out. Named here rather than left as
// bare 0.18 / 0.32 floats threaded through the entry/exit math below.
const (
	splashEnterDur = float32(0.18)
	splashExitDur  = float32(0.32)
)

// drawBattleSplash slams a banner with the encounter title at the top of the
// screen during the opening of a battle. Slides + scales in for impact.
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
	// Battle splash uses FontTitle for the encounter name and
	// FontBody for the subtitle — per UI_STANDARDS.md "Type" the
	// splash banner is the highest-emphasis transient surface.
	titleSize := FontTitle
	subSize := FontBody
	spacing := FontSpacingTitle

	// The title/subtitle strings are stable for the splash's ~40-frame
	// lifetime, so route both measures through measureCache rather than
	// paying a cgo MeasureTextEx round-trip per frame (mirrors every other
	// per-frame measure in the package). The scale animates the title, but
	// we measure at the fixed base size and scale the result, like the
	// damage-popup caches do.
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
	// Splash needs fade-driven shadow alphas (titleAlpha/subAlpha track the
	// banner's overall opacity) plus a heavier 3px drop offset and the title
	// letter-spacing — drawTextWithShadowStyle takes all three (custom shadow
	// color, offset, spacing), which is exactly what it documents itself for.
	drawTextWithShadowStyle(assets.hudFont, text, titleX, titleY, titleSize*scale, spacing*scale,
		colorWithAlpha(splashTitleColor, titleAlpha), colorWithAlpha(shadowBase, titleAlpha), 3, 3)

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
			colorWithAlpha(borderEnemy, subAlpha), colorWithAlpha(shadowBase, subAlpha), 1, 1)
	}
}

// splashTitleMeasureCache / splashSubMeasureCache memoize rl.MeasureTextEx for
// the encounter title + subtitle, which are stable across the splash's ~40-frame
// lifetime — same per-frame-measure pattern as rosterCondMeasureCache.
var (
	splashTitleMeasureCache measureCache
	splashSubMeasureCache   measureCache
)

// splashSubtitleCache memoizes the formatted subtitle by (count, noun) so
// the ~40 frames a splash is visible don't each pay a fmt.Sprintf — same
// rebuild-on-change pattern as hud.go's goldReadout cache.
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

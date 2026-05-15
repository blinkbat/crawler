package render

import (
	"crawler/internal/app/core"
	"fmt"
	"image/color"
	"math"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// drawBattleHUD orchestrates the in-combat HUD. Each panel owns one screen
// region (top-center roster, bottom-left log, bottom-center action, top-right
// turn order) so they never compete for the same real estate. During the
// timing minigame the log and action panels yield their strip to the bar.
func drawBattleHUD(g core.GameState, assets Resources) {
	drawEnemyRoster(g, assets)
	if !timingActive(g) {
		drawCombatLogPanel(g, assets)
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

// targetingAlly is true when the player is choosing a party member to act
// on — either a heal-skill target or an item target. Used by the renderer
// to gate the friendly selection marker so it appears in both modes
// (audit-3 caught Item targeting silently dropping the marker because the
// check was specific to ActionPartyTarget).
func targetingAlly(g core.GameState) bool {
	return g.Battle.ActionMode == core.ActionPartyTarget || g.Battle.ActionMode == core.ActionItemTarget
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
	headerH := int32(70)
	padBottom := int32(18)
	w := int32(560)
	if len(slots) <= 1 {
		w = 440
	}
	h := headerH + int32(len(slots))*rowH + padBottom
	x := centerX(w)
	y := int32(34)

	drawCard(x, y, w, h, surfacePrimary, borderSoft, borderEnemy)

	header := rosterHeader(g)
	drawHeading(assets.hudFont, header, x+22, y+18, borderEnemy)

	targetable := g.Battle.ActionMode == core.ActionEnemyTarget && inPlayerTurn(g)
	members := core.BattleMembers(&g)
	selectedSlot := core.SelectedEnemySlot(&g)

	for i, slot := range slots {
		enemy := members[slot]
		rowY := y + headerH + int32(i)*rowH
		drawEnemyRosterRow(assets.hudFont, enemy, x+14, rowY, w-28, rowH-8, targetable && slot == selectedSlot, !enemy.Alive)
	}
}

// rosterSlotsBuf is a package-private reusable buffer for visibleRosterSlots
// so the per-frame roster draw doesn't allocate a fresh slice every tick.
// raylib's draw loop is single-threaded, so re-slicing this isn't racy.
var rosterSlotsBuf = make([]int, 0, 16)

func visibleRosterSlots(g core.GameState) []int {
	rosterSlotsBuf = rosterSlotsBuf[:0]
	for i, m := range core.BattleMembers(&g) {
		if !m.Alive && m.DeathFade <= 0 {
			continue
		}
		rosterSlotsBuf = append(rosterSlotsBuf, i)
	}
	return rosterSlotsBuf
}

func drawEnemyRosterRow(font rl.Font, enemy core.Enemy, x, y, w, h int32, targeted, fading bool) {
	bg := rl.NewColor(20, 14, 22, 200)
	border := rl.NewColor(96, 60, 64, 140)
	nameCol := textPrimary
	if fading {
		bg = rl.NewColor(28, 20, 24, 130)
		border = borderDim
		nameCol = textDim
	}
	if targeted {
		bg = core.MixColor(bg, surfaceEnemyTint, 0.7)
		border = borderEnemy
	}
	drawSmallPanel(x, y, w, h, bg)
	drawSmallPanelOutline(x, y, w, h, border)

	leftPad := int32(22)
	if targeted {
		leftPad = 32
		bx := float32(x) + 8
		cy := float32(y) + float32(h)/2
		col := fadeColor(borderEnemy, 0.7+0.3*pulse(2.4))
		drawArrowMarker(rl.NewVector2(bx, cy), 12, 0, 9, col)
	}

	condition, condCol := enemyHealthStyle(enemy)

	nameX := float32(x + leftPad)
	displayName := core.EnemyDisplayName(enemy)
	drawTextWithShadow(font, displayName, nameX, float32(y+10), 24, nameCol)

	condSize := float32(16)
	condY := float32(y) + float32(h) - condSize - 9
	drawTextWithShadow(font, condition, nameX, condY, condSize, condCol)

	// HP bar on the right, vertically centered.
	barW := float32(200)
	barH := float32(28)
	barX := float32(x+w) - barW - 16
	barY := float32(y) + (float32(h)-barH)/2
	drawBar(font, barX, barY, barW, barH, "HP", enemy.HP, enemy.MaxHP, barEnemyHP, fading)

	// Burn indicator immediately left of HP bar.
	if enemy.BurnTurns > 0 {
		burnW := float32(34)
		burnH := barH
		burnX := barX - burnW - 10
		burnY := barY
		flicker := 0.55 + 0.45*pulse(3.4)
		drawSmallPanel(int32(burnX), int32(burnY), int32(burnW), int32(burnH), fadeColor(barBurn, flicker))
		drawSmallPanelOutline(int32(burnX), int32(burnY), int32(burnW), int32(burnH), rl.NewColor(255, 200, 120, 220))
		drawTextCentered(font, fmt.Sprintf("%d", enemy.BurnTurns), burnX+burnW/2, burnY+2, 18, rl.RayWhite)
	}
}

func drawCombatLogPanel(g core.GameState, assets Resources) {
	w := int32(460)
	h := int32(170)
	x := int32(22)
	y := int32(PartyRibbonTopY()) - h - 14

	drawCard(x, y, w, h, surfacePrimary, borderSoft, borderStrong)
	drawHeading(assets.hudFont, "COMBAT LOG", x+20, y+14, borderStrong)

	innerX := x + 16
	innerY := y + 42
	innerW := w - 32
	innerH := h - 56

	drawSmallPanel(innerX, innerY, innerW, innerH, surfaceLog)

	lines := g.Battle.Log
	if len(lines) == 0 && g.Battle.Message != "" {
		lines = []string{g.Battle.Message}
	}
	if len(lines) == 0 {
		return
	}

	lineH := int32(24)
	lineSize := float32(17)
	maxLines := int(innerH / lineH)
	if maxLines < 1 {
		maxLines = 1
	}
	start := len(lines) - maxLines
	if start < 0 {
		start = 0
	}
	visible := lines[start:]
	startY := innerY + innerH - int32(len(visible))*lineH - 6
	for i, line := range visible {
		col := textMuted
		if i == len(visible)-1 {
			col = textPrimary
		} else {
			alpha := 0.55 + 0.45*float32(i)/float32(len(visible))
			col = fadeColor(textMuted, alpha)
		}
		drawTextWithShadow(assets.hudFont, line, float32(innerX+10), float32(startY+int32(i)*lineH), lineSize, col)
	}
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
	// Taller panel — 4 action rows now (Attack/Skill/Defend/Item) and the
	// item picker mode reuses this same panel for its list.
	h := int32(280)
	// Right of the combat log, above the party ribbon, left of the turn order
	// (turn panel is 216 wide with a 22 right margin; leave a 20px gap).
	const turnReserve = int32(258)
	x := screenW - w - turnReserve
	y := int32(PartyRibbonTopY()) - h - 14

	classCol := partyClassPresentationFor(member.Class).turnColor
	drawCard(x, y, w, h, surfacePrimary, borderActive, classCol)

	header := strings.ToUpper(member.Name + "'S TURN")
	drawHeading(assets.hudFont, header, x+20, y+14, classCol)

	contentX := x + 20
	contentY := y + 48

	switch g.Battle.ActionMode {
	case core.ActionEnemyTarget:
		actionLabel := "Attack"
		if g.Battle.PendingSkill != core.SkillNone {
			actionLabel = core.SkillName(g.Battle.PendingSkill)
		}
		drawTextWithShadow(assets.hudFont, actionLabel, float32(contentX), float32(contentY), 24, textPrimary)
		drawTextWithShadow(assets.hudFont, "Choose a target", float32(contentX), float32(contentY+34), 16, textLabel)
	case core.ActionPartyTarget:
		targetName := "Ally"
		if g.Battle.PartyTarget >= 0 && g.Battle.PartyTarget < len(g.Party) {
			targetName = g.Party[g.Battle.PartyTarget].Name
		}
		drawTextWithShadow(assets.hudFont, fmt.Sprintf("%s -> %s", core.SkillName(g.Battle.PendingSkill), targetName), float32(contentX), float32(contentY), 23, textPrimary)
		drawTextWithShadow(assets.hudFont, "Choose an ally", float32(contentX), float32(contentY+34), 16, textLabel)
	case core.ActionItemMenu:
		drawTextWithShadow(assets.hudFont, "Items", float32(contentX), float32(contentY), 24, textPrimary)
		drawItemMenuList(g, assets, contentX, contentY+34)
	case core.ActionItemTarget:
		itemName := core.ItemInfo(g.Battle.PendingItem).Name
		targetName := "Ally"
		if g.Battle.PartyTarget >= 0 && g.Battle.PartyTarget < len(g.Party) {
			targetName = g.Party[g.Battle.PartyTarget].Name
		}
		drawTextWithShadow(assets.hudFont, fmt.Sprintf("%s -> %s", itemName, targetName), float32(contentX), float32(contentY), 22, textPrimary)
		drawTextWithShadow(assets.hudFont, "Choose an ally", float32(contentX), float32(contentY+34), 16, textLabel)
	default:
		// Transient status line — populated by setBattleStatus to surface
		// validation errors that aren't real combat-log events (e.g.
		// "Swipe needs more MP."). Picker modes use their own hardcoded
		// prompt so we only render this in the action menu itself.
		if status := transientStatus(g); status != "" {
			drawTextWithShadow(assets.hudFont, status, float32(contentX), float32(contentY), 15, classCol)
			contentY += 26
		}
		drawActionMenuOptions(g, assets, contentX, contentY, member)
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
	skill := core.PartySkill(member)
	skillName := core.SkillName(skill)
	skillCost := core.SkillCost(skill)

	rowSpacing := int32(40)
	cursor := core.ActionRow(g.Battle.MenuIndex)

	drawActionRow(assets.hudFont, x, y+int32(core.ActionRowAttack)*rowSpacing, "Attack", "", cursor == core.ActionRowAttack)

	costLabel := ""
	if skillCost > 0 {
		costLabel = fmt.Sprintf("%d MP", skillCost)
	}
	drawActionRow(assets.hudFont, x, y+int32(core.ActionRowSkill)*rowSpacing, skillName, costLabel, cursor == core.ActionRowSkill)
	drawActionRow(assets.hudFont, x, y+int32(core.ActionRowDefend)*rowSpacing, "Defend", "", cursor == core.ActionRowDefend)
	// Item row: shows total stack count as a hint so the player knows the
	// menu has anything in it before opening the picker. Empty inventory
	// renders the row dimmed by hint text rather than disabled, since the
	// menu code already shows a "No items." status if you confirm on it.
	itemSuffix := ""
	if total := totalItemCount(g.Inventory); total > 0 {
		itemSuffix = fmt.Sprintf("x%d", total)
	}
	drawActionRow(assets.hudFont, x, y+int32(core.ActionRowItem)*rowSpacing, "Item", itemSuffix, cursor == core.ActionRowItem)
}

// drawItemMenuList renders the inventory picker as a vertical list of
// "Name x Count" rows with the highlighted entry tinted by the selection
// border. Empty inventory falls through to a single "(no items)" hint row
// so the panel doesn't look broken if the player gets here somehow.
func drawItemMenuList(g core.GameState, assets Resources, x, y int32) {
	rowSpacing := int32(28)
	living := core.LiveStacks(g.Inventory)
	if len(living) == 0 {
		drawTextWithShadow(assets.hudFont, "(no items)", float32(x), float32(y), 16, textDim)
		return
	}
	for i, slot := range living {
		def := core.ItemInfo(slot.Kind)
		label := def.Name
		suffix := fmt.Sprintf("x%d", slot.Count)
		drawActionRow(assets.hudFont, x, y+int32(i)*rowSpacing, label, suffix, g.Battle.ItemMenuIndex == i)
	}
}

// totalItemCount sums all the inventory's stack counts. Used by the action
// menu's "Item xN" hint label.
func totalItemCount(inv []core.ItemStack) int {
	n := 0
	for _, s := range inv {
		if s.Count > 0 {
			n += s.Count
		}
	}
	return n
}

func drawActionRow(font rl.Font, x, y int32, label, suffix string, selected bool) {
	rowW := int32(284)
	rowH := int32(32)
	if selected {
		drawSmallPanel(x-8, y-4, rowW, rowH, surfaceActiveTint)
		drawSmallPanelOutline(x-8, y-4, rowW, rowH, borderActive)
		cx := float32(x - 16)
		cy := float32(y) + 12
		rl.DrawTriangle(
			rl.NewVector2(cx, cy-7),
			rl.NewVector2(cx+8, cy),
			rl.NewVector2(cx, cy+7),
			borderActive,
		)
	}
	drawTextWithShadow(font, label, float32(x), float32(y), 21, textPrimary)
	if suffix != "" {
		size := float32(15)
		measure := rl.MeasureTextEx(font, suffix, size, 1)
		sx := float32(x) + float32(rowW) - measure.X - 22
		sy := float32(y) + 5
		drawTextWithShadow(font, suffix, sx, sy, size, textLabel)
	}
}

func enemyHealthStyle(enemy core.Enemy) (string, color.RGBA) {
	condition := core.EnemyConditionFor(enemy)
	switch condition {
	case core.EnemyScuffed:
		return core.EnemyConditionLabel(condition), rl.NewColor(208, 226, 128, 255)
	case core.EnemyInjured:
		return core.EnemyConditionLabel(condition), rl.NewColor(246, 196, 91, 255)
	case core.EnemyBadlyWounded:
		return core.EnemyConditionLabel(condition), rl.NewColor(244, 126, 75, 255)
	case core.EnemyNearDeath:
		return core.EnemyConditionLabel(condition), rl.NewColor(255, 78, 88, 255)
	default:
		return core.EnemyConditionLabel(condition), rl.NewColor(126, 231, 170, 255)
	}
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

	text := core.BattleEncounterTitle(g)
	subtitle := splashSubtitle(g)
	titleSize := float32(48)
	subSize := float32(20)
	spacing := float32(1.5)

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
	drawPanelOutline(int32(bgX), int32(bgY), int32(bgW), int32(bgH), rl.NewColor(borderEnemy.R, borderEnemy.G, borderEnemy.B, uint8(float32(borderEnemy.A)*overall)))

	titleX := cx - titleW/2
	titleY := bgY + padY
	// Splash uses fade-driven shadow alphas (titleAlpha/subAlpha track the
	// banner's overall opacity) so the shadow vanishes with the rest of the
	// banner; that's why this isn't drawTextWithShadow.
	rl.DrawTextEx(assets.hudFont, text, rl.NewVector2(titleX+3, titleY+3), titleSize*scale, spacing*scale, rl.NewColor(0, 0, 0, titleAlpha))
	rl.DrawTextEx(assets.hudFont, text, rl.NewVector2(titleX, titleY), titleSize*scale, spacing*scale, rl.NewColor(248, 232, 198, titleAlpha))

	if subtitle != "" {
		subX := cx - subMeasure.X/2
		subY := titleY + titleH + gap
		rl.DrawTextEx(assets.hudFont, subtitle, rl.NewVector2(subX+1, subY+1), subSize, 1, rl.NewColor(0, 0, 0, subAlpha))
		rl.DrawTextEx(assets.hudFont, subtitle, rl.NewVector2(subX, subY), subSize, 1, rl.NewColor(borderEnemy.R, borderEnemy.G, borderEnemy.B, subAlpha))
	}
}

func rosterHeader(g core.GameState) string {
	living := core.LivingBattleCount(&g)
	total := len(core.BattleMembers(&g))
	if total <= 1 {
		def := core.BattleEnemyInfo(g)
		return strings.ToUpper(def.SingularName)
	}
	return fmt.Sprintf("%s   %d / %d", strings.ToUpper(core.BattleEnemyGroupName(g)), living, total)
}

func splashSubtitle(g core.GameState) string {
	count := len(core.BattleMembers(&g))
	if count <= 1 {
		return "Hostile encounter"
	}
	def := core.BattleEnemyInfo(g)
	return fmt.Sprintf("%d %s closing in", count, def.PluralNoun)
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

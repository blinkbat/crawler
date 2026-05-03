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
// turn order) so they never compete for the same real estate.
func drawBattleHUD(g core.GameState, assets Resources) {
	drawEnemyRoster(g, assets)
	drawCombatLogPanel(g, assets)
	drawActionMenuPanel(g, assets)
}

// drawEnemyRoster shows the active enemy group at the top of the screen.
// Replaces the legacy floating target tooltip and the dense enemy info line
// that used to sit atop the bottom panel.
func drawEnemyRoster(g core.GameState, assets Resources) {
	if g.Battle.Phase == core.BattleWon || g.Battle.Phase == core.BattleLost {
		return
	}
	indices := visibleRosterIndices(g)
	if len(indices) == 0 {
		return
	}

	screenW := int32(rl.GetScreenWidth())
	rowH := int32(46)
	headerH := int32(56)
	padBottom := int32(16)
	w := int32(460)
	if len(indices) <= 1 {
		w = 360
	}
	h := headerH + int32(len(indices))*rowH + padBottom
	x := screenW/2 - w/2
	y := int32(34)

	drawCard(x, y, w, h, surfacePrimary, borderSoft, borderEnemy)

	header := rosterHeader(g)
	drawHeading(assets.hudFont, header, x+20, y+14, borderEnemy)

	targetable := g.Battle.ActionMode == core.ActionEnemyTarget && g.Battle.Phase == core.BattlePlayer

	for i, enemyIndex := range indices {
		enemy := g.Enemies[enemyIndex]
		rowY := y + headerH + int32(i)*rowH
		drawEnemyRosterRow(assets.hudFont, enemy, x+12, rowY, w-24, rowH-6, targetable && enemyIndex == g.Battle.EnemyIndex, !enemy.Alive)
	}
}

func visibleRosterIndices(g core.GameState) []int {
	out := make([]int, 0, len(g.Battle.EnemyGroup))
	for _, idx := range g.Battle.EnemyGroup {
		if idx < 0 || idx >= len(g.Enemies) {
			continue
		}
		enemy := g.Enemies[idx]
		if !enemy.Alive && enemy.DeathFade <= 0 {
			continue
		}
		out = append(out, idx)
	}
	return out
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
		bg = mixColor(bg, surfaceEnemyTint, 0.7)
		border = borderEnemy
	}
	drawSmallPanel(x, y, w, h, bg)
	drawSmallPanelOutline(x, y, w, h, border)

	leftPad := int32(18)
	if targeted {
		leftPad = 26
		bx := float32(x) + 6
		cy := float32(y) + float32(h)/2
		col := fadeColor(borderEnemy, 0.7+0.3*pulse(2.4))
		rl.DrawTriangle(
			rl.NewVector2(bx, cy-7),
			rl.NewVector2(bx+10, cy),
			rl.NewVector2(bx, cy+7),
			col,
		)
	}

	condition, condCol := enemyHealthStyle(enemy)

	nameX := float32(x + leftPad)
	displayName := core.EnemyDisplayName(enemy)
	drawTextWithShadow(font, displayName, nameX, float32(y+7), 19, nameCol)

	condSize := float32(13)
	condY := float32(y) + float32(h) - condSize - 7
	rl.DrawTextEx(font, condition, rl.NewVector2(nameX+1, condY+1), condSize, 1, rl.NewColor(0, 0, 0, 200))
	rl.DrawTextEx(font, condition, rl.NewVector2(nameX, condY), condSize, 1, condCol)

	// HP bar on the right, vertically centered.
	barW := float32(160)
	barH := float32(20)
	barX := float32(x+w) - barW - 14
	barY := float32(y) + (float32(h)-barH)/2
	drawBar(font, barX, barY, barW, barH, "HP", enemy.HP, enemy.MaxHP, barEnemyHP, fading)

	// Burn indicator immediately left of HP bar.
	if enemy.BurnTurns > 0 {
		burnW := float32(26)
		burnH := barH
		burnX := barX - burnW - 8
		burnY := barY
		flicker := 0.55 + 0.45*pulse(3.4)
		drawSmallPanel(int32(burnX), int32(burnY), int32(burnW), int32(burnH), fadeColor(barBurn, flicker))
		drawSmallPanelOutline(int32(burnX), int32(burnY), int32(burnW), int32(burnH), rl.NewColor(255, 200, 120, 220))
		drawTextCentered(font, fmt.Sprintf("%d", enemy.BurnTurns), burnX+burnW/2, burnY+1, 14, rl.RayWhite)
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

	screenW := int32(rl.GetScreenWidth())
	w := int32(340)
	h := int32(170)
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
		drawActionFootHint(assets.hudFont, x, y, w, h, "A/D target", "Z confirm", "X back")
	case core.ActionPartyTarget:
		targetName := "Ally"
		if g.Battle.PartyTarget >= 0 && g.Battle.PartyTarget < len(g.Party) {
			targetName = g.Party[g.Battle.PartyTarget].Name
		}
		drawTextWithShadow(assets.hudFont, fmt.Sprintf("%s -> %s", core.SkillName(g.Battle.PendingSkill), targetName), float32(contentX), float32(contentY), 23, textPrimary)
		drawTextWithShadow(assets.hudFont, "Choose an ally", float32(contentX), float32(contentY+34), 16, textLabel)
		drawActionFootHint(assets.hudFont, x, y, w, h, "A/D choose", "Z confirm", "X back")
	default:
		drawActionMenuOptions(g, assets, contentX, contentY, member)
		drawActionFootHint(assets.hudFont, x, y, w, h, "W/S choose", "Z confirm", "")
	}
}

func drawActionMenuOptions(g core.GameState, assets Resources, x, y int32, member core.PartyMember) {
	skill := core.PartySkill(member)
	skillName := core.SkillName(skill)
	skillCost := core.SkillCost(skill)

	rowSpacing := int32(40)

	drawActionRow(assets.hudFont, x, y, "Attack", "", g.Battle.MenuIndex == 0)

	costLabel := ""
	if skillCost > 0 {
		costLabel = fmt.Sprintf("%d MP", skillCost)
	}
	drawActionRow(assets.hudFont, x, y+rowSpacing, skillName, costLabel, g.Battle.MenuIndex == 1)
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

func drawActionFootHint(font rl.Font, x, y, w, h int32, hints ...string) {
	combined := ""
	for _, hint := range hints {
		if hint == "" {
			continue
		}
		if combined != "" {
			combined += "    "
		}
		combined += hint
	}
	if combined == "" {
		return
	}
	size := float32(13)
	measure := rl.MeasureTextEx(font, combined, size, 1)
	hx := float32(x+w) - measure.X - 16
	hy := float32(y+h) - measure.Y - 12
	rl.DrawTextEx(font, combined, rl.NewVector2(hx+1, hy+1), size, 1, rl.NewColor(0, 0, 0, 200))
	rl.DrawTextEx(font, combined, rl.NewVector2(hx, hy), size, 1, textHint)
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
	if g.Battle.Splash <= 0 || g.Battle.EnemyIndex < 0 || g.Battle.EnemyIndex >= len(g.Enemies) {
		return
	}
	const splashTotal = float32(1.15)
	progress := splashTotal - g.Battle.Splash
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

	cx := float32(rl.GetScreenWidth()) / 2
	cy := float32(rl.GetScreenHeight())*0.42 + (1-intro)*-26

	bgX := cx - bgW/2
	bgY := cy - bgH/2

	bgAlpha := uint8(220 * overall)
	titleAlpha := uint8(255 * overall)
	subAlpha := uint8(220 * overall)

	drawPanel(int32(bgX), int32(bgY), int32(bgW), int32(bgH), rl.NewColor(8, 10, 16, bgAlpha))
	drawPanelOutline(int32(bgX), int32(bgY), int32(bgW), int32(bgH), rl.NewColor(borderEnemy.R, borderEnemy.G, borderEnemy.B, uint8(float32(borderEnemy.A)*overall)))

	titleX := cx - titleW/2
	titleY := bgY + padY
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
	total := len(g.Battle.EnemyGroup)
	if total <= 1 {
		def := core.BattleEnemyInfo(g)
		return strings.ToUpper(def.SingularName)
	}
	return fmt.Sprintf("%s   %d / %d", strings.ToUpper(core.BattleEnemyGroupName(g)), living, total)
}

func splashSubtitle(g core.GameState) string {
	count := len(g.Battle.EnemyGroup)
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

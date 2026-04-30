package render

import (
	"crawler/internal/app/battle"
	"crawler/internal/app/core"
	"fmt"
	rl "github.com/gen2brain/raylib-go/raylib"
	"image/color"
)

func drawBattleOverlay(g core.GameState, assets Resources) {
	panelX := int32(12)
	panelH := int32(168)
	panelY := int32(rl.GetScreenHeight()) - panelH - 14
	panelW := int32(540)
	drawRoundedRect(panelX, panelY, panelW, panelH, 0.07, rl.NewColor(9, 10, 12, 190))
	drawRoundedRectLines(panelX, panelY, panelW, panelH, 0.07, rl.NewColor(210, 220, 230, 180))

	enemyText := "Rat"
	enemyHP := 0
	enemyMaxHP := core.RatMaxHP
	aliveCount := battle.LivingBattleCount(&g)
	if g.Battle.EnemyIndex >= 0 && g.Battle.EnemyIndex < len(g.Enemies) {
		e := g.Enemies[g.Battle.EnemyIndex]
		enemyText = e.Name
		enemyHP = e.HP
		enemyMaxHP = e.MaxHP
	}
	if len(g.Battle.EnemyGroup) > 1 {
		enemyText = fmt.Sprintf("Rat Pack  %d left  Target %d/%d HP:%d/%d", aliveCount, battle.BattleTargetOrdinal(g), aliveCount, enemyHP, enemyMaxHP)
	} else {
		enemyText = fmt.Sprintf("%s  HP:%d/%d", enemyText, enemyHP, enemyMaxHP)
	}
	drawHUDText(assets.hudFont, enemyText, panelX+18, panelY+14, 22)
	activeName := "Party"
	if battle.PartyMemberAlive(g.Party, g.Battle.CurrentParty) {
		activeName = g.Party[g.Battle.CurrentParty].Name
	}
	drawHUDText(assets.hudFont, fmt.Sprintf("Active: %s", activeName), panelX+18, panelY+42, 20)
	drawCombatLog(g, assets, panelX+18, panelY+72, 282, panelH-88)
	if g.Battle.Phase == core.BattlePlayer {
		drawBattleActionMenu(g, assets, panelX+318, panelY+18)
	}
}

func drawCombatLog(g core.GameState, assets Resources, x, y, w, h int32) {
	drawRoundedRect(x-6, y-5, w+12, h+10, 0.09, rl.NewColor(3, 5, 9, 105))
	drawRoundedRectLines(x-6, y-5, w+12, h+10, 0.09, rl.NewColor(210, 220, 230, 70))
	lines := g.Battle.Log
	if len(lines) == 0 && g.Battle.Message != "" {
		lines = []string{g.Battle.Message}
	}
	lineH := int32(18)
	maxLines := int(h / lineH)
	if maxLines < 1 {
		maxLines = 1
	}
	start := len(lines) - int(maxLines)
	if start < 0 {
		start = 0
	}
	visible := lines[start:]
	startY := y + h - int32(len(visible))*lineH
	for i, line := range visible {
		drawHUDText(assets.hudFont, line, x, startY+int32(i)*lineH, 16)
	}
}

func drawBattleActionMenu(g core.GameState, assets Resources, x, y int32) {
	switch g.Battle.ActionMode {
	case core.ActionEnemyTarget:
		action := "Attack"
		if g.Battle.PendingSkill != core.SkillNone {
			action = battle.SkillName(g.Battle.PendingSkill)
		}
		drawHUDText(assets.hudFont, action, x, y, 21)
		drawHUDText(assets.hudFont, "A/D target  Z/Space/Enter", x, y+30, 16)
		drawHUDText(assets.hudFont, "X/Esc back", x, y+52, 16)
	case core.ActionPartyTarget:
		drawHUDText(assets.hudFont, battle.SkillName(g.Battle.PendingSkill), x, y, 21)
		drawHUDText(assets.hudFont, "A/D ally  Z/Space/Enter", x, y+30, 16)
		drawHUDText(assets.hudFont, "X/Esc back", x, y+52, 16)
	default:
		if g.Battle.CurrentParty < 0 || g.Battle.CurrentParty >= len(g.Party) {
			return
		}
		skill := battle.PartySkill(g.Party[g.Battle.CurrentParty].Name)
		skillText := battle.SkillName(skill)
		if cost := battle.SkillCost(skill); cost > 0 {
			skillText = fmt.Sprintf("%s  %d MP", skillText, cost)
		}
		drawActionOption(assets.hudFont, "Attack", x, y, g.Battle.MenuIndex == 0)
		drawActionOption(assets.hudFont, skillText, x, y+28, g.Battle.MenuIndex == 1)
		drawHUDText(assets.hudFont, "W/S choose  Z/Space/Enter", x, y+58, 15)
	}
}

func drawActionOption(font rl.Font, text string, x, y int32, selected bool) {
	if selected {
		drawRoundedRect(x-14, y-3, 210, 25, 0.28, rl.NewColor(72, 76, 110, 145))
		rl.DrawTriangle(
			rl.NewVector2(float32(x-4), float32(y+9)),
			rl.NewVector2(float32(x-10), float32(y+3)),
			rl.NewVector2(float32(x-10), float32(y+15)),
			rl.NewColor(118, 235, 136, 255),
		)
	}
	drawHUDText(font, text, x+4, y, 18)
}

func drawTargetTooltip(g core.GameState, assets Resources) {
	if g.Battle.Phase == core.BattleWon || g.Battle.EnemyIndex < 0 || g.Battle.EnemyIndex >= len(g.Enemies) {
		return
	}
	enemy := g.Enemies[g.Battle.EnemyIndex]
	if !enemy.Alive {
		return
	}
	monsterType := enemy.MonsterType
	if monsterType == "" {
		monsterType = "Beast"
	}
	condition := enemyHealthText(enemy)
	screenW := int32(rl.GetScreenWidth())
	screenH := int32(rl.GetScreenHeight())
	panelW := int32(310)
	panelH := int32(86)
	panelX := screenW/2 - panelW/2
	panelY := int32(float32(screenH) * 0.35)
	if panelY < 170 {
		panelY = 170
	}
	if maxY := screenH/2 - 70; panelY > maxY {
		panelY = maxY
	}

	drawRoundedRect(panelX, panelY, panelW, panelH, 0.1, rl.NewColor(6, 10, 18, 176))
	drawRoundedRectLines(panelX, panelY, panelW, panelH, 0.1, rl.NewColor(255, 222, 94, 205))
	centerX := float32(panelX + panelW/2)
	drawTextCentered(assets.hudFont, enemy.Name, centerX, float32(panelY+9), 23, rl.RayWhite)
	drawTextCentered(assets.hudFont, monsterType, centerX, float32(panelY+38), 18, rl.NewColor(184, 215, 238, 255))
	drawTextCentered(assets.hudFont, condition, centerX, float32(panelY+60), 18, enemyHealthColor(enemy))
}

func enemyHealthText(enemy core.Enemy) string {
	if enemy.MaxHP <= 0 || enemy.HP >= enemy.MaxHP {
		return "Unharmed"
	}
	percent := float64(enemy.HP) / float64(enemy.MaxHP)
	switch {
	case percent > 0.75:
		return "Scuffed"
	case percent > 0.5:
		return "Injured"
	case percent > 0.25:
		return "Badly Wounded"
	default:
		return "Near Death"
	}
}

func enemyHealthColor(enemy core.Enemy) color.RGBA {
	if enemy.MaxHP <= 0 || enemy.HP >= enemy.MaxHP {
		return rl.NewColor(126, 231, 170, 255)
	}
	percent := float64(enemy.HP) / float64(enemy.MaxHP)
	switch {
	case percent > 0.75:
		return rl.NewColor(208, 226, 128, 255)
	case percent > 0.5:
		return rl.NewColor(246, 196, 91, 255)
	case percent > 0.25:
		return rl.NewColor(244, 126, 75, 255)
	default:
		return rl.NewColor(255, 78, 88, 255)
	}
}

func drawBattleSplash(g core.GameState, assets Resources) {
	if g.Battle.Splash <= 0 || g.Battle.EnemyIndex < 0 || g.Battle.EnemyIndex >= len(g.Enemies) {
		return
	}
	text := fmt.Sprintf("%s Encounter!", g.Enemies[g.Battle.EnemyIndex].Name)
	if len(g.Battle.EnemyGroup) > 1 {
		text = fmt.Sprintf("Rat Pack x%d!", len(g.Battle.EnemyGroup))
	}
	size := float32(42)
	spacing := float32(1.5)
	textSize := rl.MeasureTextEx(assets.hudFont, text, size, spacing)
	x := float32(rl.GetScreenWidth())/2 - textSize.X/2
	y := float32(rl.GetScreenHeight())*0.28 - textSize.Y/2
	padX := float32(28)
	padY := float32(14)
	alpha := uint8(235)
	if g.Battle.Splash < 0.25 {
		alpha = uint8(235 * (g.Battle.Splash / 0.25))
	}
	bgAlpha := uint8(float32(alpha) * 0.78)
	drawRoundedRect(
		int32(x-padX),
		int32(y-padY),
		int32(textSize.X+padX*2),
		int32(textSize.Y+padY*2),
		0.12,
		rl.NewColor(8, 10, 12, bgAlpha),
	)
	rl.DrawTextEx(assets.hudFont, text, rl.NewVector2(x+3, y+3), size, spacing, rl.NewColor(0, 0, 0, alpha))
	rl.DrawTextEx(assets.hudFont, text, rl.NewVector2(x, y), size, spacing, rl.NewColor(245, 248, 252, alpha))
}

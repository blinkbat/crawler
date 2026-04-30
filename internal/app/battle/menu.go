package battle

import (
	"crawler/internal/app/core"
	"crawler/internal/app/input"
	"fmt"
)

func updateActionMenu(g *core.GameState) {
	if input.UpPressed() {
		g.Battle.MenuIndex = core.WrapIndex(g.Battle.MenuIndex-1, 2)
	}
	if input.DownPressed() {
		g.Battle.MenuIndex = core.WrapIndex(g.Battle.MenuIndex+1, 2)
	}
	if input.BackPressed() {
		setBattleStatus(g, "Choose an action.")
		return
	}
	if !input.ConfirmPressed() {
		return
	}
	if g.Battle.MenuIndex == 0 {
		g.Battle.PendingSkill = core.SkillNone
		g.Battle.ActionMode = core.ActionEnemyTarget
		setBattleStatus(g, "Choose a target.")
		return
	}

	skill := core.PartySkill(g.Party[g.Battle.CurrentParty])
	cost := core.SkillCost(skill)
	if g.Party[g.Battle.CurrentParty].MP < cost {
		setBattleStatus(g, fmt.Sprintf("%s needs %d MP.", core.SkillName(skill), cost))
		return
	}
	g.Battle.PendingSkill = skill
	switch skill {
	case core.SkillSwipe:
		useSwipe(g)
	case core.SkillPrayer:
		g.Battle.ActionMode = core.ActionPartyTarget
		g.Battle.PartyTarget = g.Battle.CurrentParty
		setBattleStatus(g, "Choose who receives Prayer.")
	case core.SkillSteal, core.SkillFirebolt:
		g.Battle.ActionMode = core.ActionEnemyTarget
		setBattleStatus(g, fmt.Sprintf("Choose a target for %s.", core.SkillName(skill)))
	default:
		setBattleStatus(g, "No skill ready.")
	}
}

func updateEnemyTargeting(g *core.GameState) {
	if input.TargetNextPressed() {
		cycleBattleTarget(g, 1)
	}
	if input.TargetPreviousPressed() {
		cycleBattleTarget(g, -1)
	}
	if input.BackPressed() {
		g.Battle.ActionMode = core.ActionMenu
		g.Battle.PendingSkill = core.SkillNone
		setBattleStatus(g, "Choose an action.")
		return
	}
	if !input.ConfirmPressed() {
		return
	}
	switch g.Battle.PendingSkill {
	case core.SkillNone:
		useAttack(g)
	case core.SkillSteal:
		useSteal(g)
	case core.SkillFirebolt:
		useFirebolt(g)
	default:
		g.Battle.ActionMode = core.ActionMenu
		setBattleStatus(g, "That needs another target.")
	}
}

func updatePartyTargeting(g *core.GameState) {
	if input.TargetNextPressed() {
		cyclePartyTarget(g, 1)
	}
	if input.TargetPreviousPressed() {
		cyclePartyTarget(g, -1)
	}
	if input.BackPressed() {
		g.Battle.ActionMode = core.ActionMenu
		g.Battle.PendingSkill = core.SkillNone
		setBattleStatus(g, "Choose an action.")
		return
	}
	if !input.ConfirmPressed() {
		return
	}
	if g.Battle.PendingSkill == core.SkillPrayer {
		usePrayer(g)
		return
	}
	g.Battle.ActionMode = core.ActionMenu
	setBattleStatus(g, "That targets enemies.")
}

func cycleBattleTarget(g *core.GameState, delta int) {
	living := core.LivingBattleEnemyIndices(g)
	if len(living) == 0 {
		return
	}
	next := cycleTarget(g.Battle.EnemyIndex, living, delta)
	g.Battle.EnemyIndex = living[next]
	setBattleStatus(g, core.BattleEnemyTargetStatus(*g, next+1, len(living)))
}

func cyclePartyTarget(g *core.GameState, delta int) {
	living := core.LivingPartyTargets(g.Party)
	if len(living) == 0 {
		return
	}
	next := cycleTarget(g.Battle.PartyTarget, living, delta)
	g.Battle.PartyTarget = living[next]
	setBattleStatus(g, fmt.Sprintf("Targeting %s.", g.Party[g.Battle.PartyTarget].Name))
}

func cycleTarget(current int, targets []int, delta int) int {
	currentSlot := 0
	for i, target := range targets {
		if target == current {
			currentSlot = i
			break
		}
	}
	return core.WrapIndex(currentSlot+delta, len(targets))
}

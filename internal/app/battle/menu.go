package battle

import (
	"crawler/internal/app/core"
	"fmt"
	rl "github.com/gen2brain/raylib-go/raylib"
)

func updateActionMenu(g *core.GameState) {
	if upPressed() || downPressed() {
		g.Battle.MenuIndex = (g.Battle.MenuIndex + 1) % 2
	}
	if backPressed() {
		setBattleStatus(g, "Choose an action.")
		return
	}
	if !confirmPressed() {
		return
	}
	if g.Battle.MenuIndex == 0 {
		g.Battle.PendingSkill = core.SkillNone
		g.Battle.ActionMode = core.ActionEnemyTarget
		setBattleStatus(g, "Choose a target.")
		return
	}

	skill := partySkill(g.Party[g.Battle.CurrentParty].Name)
	cost := SkillCost(skill)
	if g.Party[g.Battle.CurrentParty].MP < cost {
		setBattleStatus(g, fmt.Sprintf("%s needs %d MP.", SkillName(skill), cost))
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
		setBattleStatus(g, fmt.Sprintf("Choose a target for %s.", SkillName(skill)))
	default:
		setBattleStatus(g, "No skill ready.")
	}
}

func updateEnemyTargeting(g *core.GameState) {
	if rl.IsKeyPressed(rl.KeyTab) || rl.IsKeyPressed(rl.KeyRight) || rl.IsKeyPressed(rl.KeyD) || rl.IsKeyPressed(rl.KeyDown) {
		cycleBattleTarget(g, 1)
	}
	if rl.IsKeyPressed(rl.KeyLeft) || rl.IsKeyPressed(rl.KeyA) || rl.IsKeyPressed(rl.KeyUp) {
		cycleBattleTarget(g, -1)
	}
	if backPressed() {
		g.Battle.ActionMode = core.ActionMenu
		g.Battle.PendingSkill = core.SkillNone
		setBattleStatus(g, "Choose an action.")
		return
	}
	if !confirmPressed() {
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
	if rl.IsKeyPressed(rl.KeyTab) || rl.IsKeyPressed(rl.KeyRight) || rl.IsKeyPressed(rl.KeyD) || rl.IsKeyPressed(rl.KeyDown) {
		cyclePartyTarget(g, 1)
	}
	if rl.IsKeyPressed(rl.KeyLeft) || rl.IsKeyPressed(rl.KeyA) || rl.IsKeyPressed(rl.KeyUp) {
		cyclePartyTarget(g, -1)
	}
	if backPressed() {
		g.Battle.ActionMode = core.ActionMenu
		g.Battle.PendingSkill = core.SkillNone
		setBattleStatus(g, "Choose an action.")
		return
	}
	if !confirmPressed() {
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
	living := make([]int, 0, len(g.Battle.EnemyGroup))
	for _, index := range g.Battle.EnemyGroup {
		if index >= 0 && index < len(g.Enemies) && g.Enemies[index].Alive {
			living = append(living, index)
		}
	}
	if len(living) == 0 {
		return
	}
	current := 0
	for i, index := range living {
		if index == g.Battle.EnemyIndex {
			current = i
			break
		}
	}
	next := (current + delta) % len(living)
	if next < 0 {
		next += len(living)
	}
	g.Battle.EnemyIndex = living[next]
	setBattleStatus(g, fmt.Sprintf("Targeting rat %d of %d.", next+1, len(living)))
}

func cyclePartyTarget(g *core.GameState, delta int) {
	if len(g.Party) == 0 {
		return
	}
	g.Battle.PartyTarget = (g.Battle.PartyTarget + delta) % len(g.Party)
	if g.Battle.PartyTarget < 0 {
		g.Battle.PartyTarget += len(g.Party)
	}
	setBattleStatus(g, fmt.Sprintf("Targeting %s.", g.Party[g.Battle.PartyTarget].Name))
}

func validLivingEnemy(g *core.GameState, index int) bool {
	return index >= 0 && index < len(g.Enemies) && g.Enemies[index].Alive
}

func partySkill(className string) int {
	switch className {
	case "Warrior":
		return core.SkillSwipe
	case "Cleric":
		return core.SkillPrayer
	case "Thief":
		return core.SkillSteal
	case "Wizard":
		return core.SkillFirebolt
	}
	return core.SkillNone
}

func PartySkill(className string) int {
	return partySkill(className)
}

func SkillName(skill int) string {
	switch skill {
	case core.SkillSwipe:
		return "Swipe"
	case core.SkillPrayer:
		return "Prayer"
	case core.SkillSteal:
		return "Steal"
	case core.SkillFirebolt:
		return "Firebolt"
	}
	return "Skill"
}

func SkillCost(skill int) int {
	switch skill {
	case core.SkillSwipe:
		return 3
	case core.SkillPrayer:
		return 5
	case core.SkillFirebolt:
		return 6
	default:
		return 0
	}
}

func confirmPressed() bool {
	return rl.IsKeyPressed(rl.KeyEnter) || rl.IsKeyPressed(rl.KeySpace) || rl.IsKeyPressed(rl.KeyZ)
}

func backPressed() bool {
	return rl.IsKeyPressed(rl.KeyEscape) || rl.IsKeyPressed(rl.KeyX)
}

func upPressed() bool {
	return rl.IsKeyPressed(rl.KeyUp) || rl.IsKeyPressed(rl.KeyW)
}

func downPressed() bool {
	return rl.IsKeyPressed(rl.KeyDown) || rl.IsKeyPressed(rl.KeyS)
}

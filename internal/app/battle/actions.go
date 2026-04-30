package battle

import (
	"crawler/internal/app/core"
	"fmt"
)

func useAttack(g *core.GameState) {
	if !core.EnemyAlive(g.Enemies, g.Battle.EnemyIndex) {
		setBattleStatus(g, "No target.")
		return
	}
	attacker := &g.Party[g.Battle.CurrentParty]
	damage := attacker.Attack
	attacker.AttackBump = core.BumpDuration
	defeated := damageEnemy(g, g.Battle.EnemyIndex, damage)
	if defeated {
		setBattleMessage(g, fmt.Sprintf("%s drops a rat.", attacker.Name))
	} else {
		setBattleMessage(g, fmt.Sprintf("%s hits for %d.", attacker.Name, damage))
	}
	finishPartyAction(g)
}

func useSwipe(g *core.GameState) {
	actor := &g.Party[g.Battle.CurrentParty]
	cost := core.SkillCost(core.SkillSwipe)
	if actor.MP < cost {
		setBattleStatus(g, "Swipe needs more MP.")
		return
	}
	actor.MP -= cost
	actor.AttackBump = core.BumpDuration
	hits := 0
	for _, index := range g.Battle.EnemyGroup {
		if !core.EnemyAlive(g.Enemies, index) {
			continue
		}
		damageEnemy(g, index, 3)
		hits++
	}
	if hits == 0 {
		setBattleMessage(g, "Swipe catches only air.")
	} else {
		setBattleMessage(g, fmt.Sprintf("%s swipes through %d foes.", actor.Name, hits))
	}
	finishPartyAction(g)
}

func usePrayer(g *core.GameState) {
	actor := &g.Party[g.Battle.CurrentParty]
	cost := core.SkillCost(core.SkillPrayer)
	if actor.MP < cost {
		setBattleStatus(g, "Prayer needs more MP.")
		return
	}
	if g.Battle.PartyTarget < 0 || g.Battle.PartyTarget >= len(g.Party) {
		setBattleStatus(g, "No ally selected.")
		return
	}
	if g.Party[g.Battle.PartyTarget].HP <= 0 {
		setBattleStatus(g, "Prayer cannot revive.")
		return
	}
	actor.MP -= cost
	target := &g.Party[g.Battle.PartyTarget]
	heal := 10
	target.HP += heal
	if target.HP > target.MaxHP {
		target.HP = target.MaxHP
	}
	actor.AttackBump = core.BumpDuration
	target.DamageFlash = core.FlashDuration
	setBattleMessage(g, fmt.Sprintf("%s prays over %s.", actor.Name, target.Name))
	finishPartyAction(g)
}

func useSteal(g *core.GameState) {
	if !core.EnemyAlive(g.Enemies, g.Battle.EnemyIndex) {
		setBattleStatus(g, "No target.")
		return
	}
	actor := &g.Party[g.Battle.CurrentParty]
	actor.AttackBump = core.BumpDuration
	enemy := &g.Enemies[g.Battle.EnemyIndex]
	if enemy.Item == "" {
		setBattleMessage(g, "There is nothing to steal.")
		finishPartyAction(g)
		return
	}
	if core.GameRNG.Float64() < 0.7 {
		item := enemy.Item
		enemy.Item = ""
		setBattleMessage(g, fmt.Sprintf("%s steals %s.", actor.Name, item))
	} else {
		setBattleMessage(g, fmt.Sprintf("%s fails to steal.", actor.Name))
	}
	finishPartyAction(g)
}

func useFirebolt(g *core.GameState) {
	if !core.EnemyAlive(g.Enemies, g.Battle.EnemyIndex) {
		setBattleStatus(g, "No target.")
		return
	}
	actor := &g.Party[g.Battle.CurrentParty]
	cost := core.SkillCost(core.SkillFirebolt)
	if actor.MP < cost {
		setBattleStatus(g, "Firebolt needs more MP.")
		return
	}
	actor.MP -= cost
	actor.AttackBump = core.BumpDuration
	damage := 6
	defeated := damageEnemy(g, g.Battle.EnemyIndex, damage)
	enemy := &g.Enemies[g.Battle.EnemyIndex]
	burned := false
	if !defeated && enemy.BurnTurns <= 0 && core.GameRNG.Float64() < 0.82 {
		enemy.BurnTurns = 3 + core.GameRNG.Intn(3)
		burned = true
	}
	if defeated {
		setBattleMessage(g, fmt.Sprintf("%s's Firebolt drops the rat.", actor.Name))
	} else if burned {
		setBattleMessage(g, fmt.Sprintf("%s scorches the rat for %d. Burning!", actor.Name, damage))
	} else if enemy.BurnTurns > 0 {
		setBattleMessage(g, fmt.Sprintf("%s hits for %d. Burn is already active.", actor.Name, damage))
	} else {
		setBattleMessage(g, fmt.Sprintf("%s hits for %d.", actor.Name, damage))
	}
	finishPartyAction(g)
}

func finishPartyAction(g *core.GameState) {
	if core.LivingBattleCount(g) == 0 {
		winBattle(g, "The last rat falls.")
		return
	}
	if next := core.NextLivingBattleEnemy(g); next >= 0 && !core.EnemyAlive(g.Enemies, g.Battle.EnemyIndex) {
		g.Battle.EnemyIndex = next
	}
	if next := core.NextLivingPartyMember(g.Party, g.Battle.CurrentParty+1); next >= 0 {
		beginPartyTurn(g, next)
		return
	}
	g.Battle.Phase = core.BattleEnemy
	g.Battle.Timer = 0.45
	g.Battle.ActionMode = core.ActionMenu
	g.Battle.PendingSkill = core.SkillNone
}

func damageEnemy(g *core.GameState, enemyIndex, damage int) bool {
	if !core.EnemyAlive(g.Enemies, enemyIndex) {
		return false
	}
	enemy := &g.Enemies[enemyIndex]
	enemy.DamageFlash = core.FlashDuration
	enemy.HP -= damage
	if enemy.HP > 0 {
		return false
	}
	enemy.HP = 0
	enemy.Alive = false
	enemy.BurnTurns = 0
	enemy.DeathFade = core.DeathFadeDuration
	return true
}

func resolveBurns(g *core.GameState) int {
	hits := 0
	for _, enemyIndex := range g.Battle.EnemyGroup {
		if !core.EnemyAlive(g.Enemies, enemyIndex) || g.Enemies[enemyIndex].BurnTurns <= 0 {
			continue
		}
		g.Enemies[enemyIndex].BurnTurns--
		damageEnemy(g, enemyIndex, 2)
		hits++
	}
	if next := core.NextLivingBattleEnemy(g); next >= 0 && !core.EnemyAlive(g.Enemies, g.Battle.EnemyIndex) {
		g.Battle.EnemyIndex = next
	}
	return hits
}

func resolveRatAttacks(g *core.GameState) int {
	hits := 0
	targetCursor := 0
	for _, enemyIndex := range g.Battle.EnemyGroup {
		if !core.EnemyAlive(g.Enemies, enemyIndex) {
			continue
		}
		target := core.NextLivingPartyMember(g.Party, targetCursor)
		if target < 0 {
			target = core.FirstLivingPartyMember(g.Party)
		}
		if target < 0 {
			break
		}
		g.Enemies[enemyIndex].AttackBump = core.BumpDuration
		g.Party[target].DamageFlash = core.FlashDuration
		g.Party[target].HP -= 2
		if g.Party[target].HP < 0 {
			g.Party[target].HP = 0
		}
		targetCursor = target + 1
		hits++
	}
	return hits
}

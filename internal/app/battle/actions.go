package battle

import (
	"crawler/internal/app/core"
	"fmt"
)

func useAttack(g *core.GameState) {
	if !validLivingEnemy(g, g.Battle.EnemyIndex) {
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
	cost := SkillCost(core.SkillSwipe)
	if actor.MP < cost {
		setBattleStatus(g, "Swipe needs more MP.")
		return
	}
	actor.MP -= cost
	actor.AttackBump = core.BumpDuration
	hits := 0
	for _, index := range g.Battle.EnemyGroup {
		if !validLivingEnemy(g, index) {
			continue
		}
		damageEnemy(g, index, 3)
		hits++
	}
	if hits == 0 {
		setBattleMessage(g, "Swipe catches only air.")
	} else {
		setBattleMessage(g, fmt.Sprintf("Warrior swipes through %d foes.", hits))
	}
	finishPartyAction(g)
}

func usePrayer(g *core.GameState) {
	actor := &g.Party[g.Battle.CurrentParty]
	cost := SkillCost(core.SkillPrayer)
	if actor.MP < cost {
		setBattleStatus(g, "Prayer needs more MP.")
		return
	}
	if g.Battle.PartyTarget < 0 || g.Battle.PartyTarget >= len(g.Party) {
		setBattleStatus(g, "No ally selected.")
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
	setBattleMessage(g, fmt.Sprintf("Cleric prays over %s.", target.Name))
	finishPartyAction(g)
}

func useSteal(g *core.GameState) {
	if !validLivingEnemy(g, g.Battle.EnemyIndex) {
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
		setBattleMessage(g, fmt.Sprintf("Thief steals %s.", item))
	} else {
		setBattleMessage(g, "Thief fails to steal.")
	}
	finishPartyAction(g)
}

func useFirebolt(g *core.GameState) {
	if !validLivingEnemy(g, g.Battle.EnemyIndex) {
		setBattleStatus(g, "No target.")
		return
	}
	actor := &g.Party[g.Battle.CurrentParty]
	cost := SkillCost(core.SkillFirebolt)
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
		setBattleMessage(g, "Wizard's Firebolt drops the rat.")
	} else if burned {
		setBattleMessage(g, fmt.Sprintf("Wizard scorches the rat for %d. Burning!", damage))
	} else if enemy.BurnTurns > 0 {
		setBattleMessage(g, fmt.Sprintf("Wizard hits for %d. Burn is already active.", damage))
	} else {
		setBattleMessage(g, fmt.Sprintf("Wizard hits for %d.", damage))
	}
	finishPartyAction(g)
}

func finishPartyAction(g *core.GameState) {
	if LivingBattleCount(g) == 0 {
		g.Battle.Phase = core.BattleWon
		g.Battle.Timer = core.VictoryDanceDuration
		setBattleMessage(g, "The last rat falls.")
		return
	}
	if next := nextLivingBattleEnemy(g); next >= 0 && !validLivingEnemy(g, g.Battle.EnemyIndex) {
		g.Battle.EnemyIndex = next
	}
	if next := nextLivingPartyMember(g.Party, g.Battle.CurrentParty+1); next >= 0 {
		beginPartyTurn(g, next)
		return
	}
	g.Battle.Phase = core.BattleEnemy
	g.Battle.Timer = 0.45
	g.Battle.ActionMode = core.ActionMenu
	g.Battle.PendingSkill = core.SkillNone
}

func damageEnemy(g *core.GameState, enemyIndex, damage int) bool {
	if !validLivingEnemy(g, enemyIndex) {
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
		if !validLivingEnemy(g, enemyIndex) || g.Enemies[enemyIndex].BurnTurns <= 0 {
			continue
		}
		g.Enemies[enemyIndex].BurnTurns--
		damageEnemy(g, enemyIndex, 2)
		hits++
	}
	if next := nextLivingBattleEnemy(g); next >= 0 && !validLivingEnemy(g, g.Battle.EnemyIndex) {
		g.Battle.EnemyIndex = next
	}
	return hits
}

func resolveRatAttacks(g *core.GameState) int {
	hits := 0
	targetCursor := 0
	for _, enemyIndex := range g.Battle.EnemyGroup {
		if enemyIndex < 0 || enemyIndex >= len(g.Enemies) || !g.Enemies[enemyIndex].Alive {
			continue
		}
		target := nextLivingPartyMember(g.Party, targetCursor)
		if target < 0 {
			target = firstLivingPartyMember(g.Party)
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

package battle

import (
	"crawler/internal/app/core"
	"crawler/internal/app/input"
)

func Start(g *core.GameState, enemyIndex int) {
	group := nearbyBattleGroup(g.Enemies, enemyIndex)
	g.Battle.EnemyIndex = enemyIndex
	g.Battle.EnemyGroup = group
	g.Battle.CurrentParty = core.FirstLivingPartyMember(g.Party)
	resetBattleAction(g)
	g.Battle.PartyTarget = core.FirstLivingPartyMember(g.Party)
	g.Battle.Phase = core.BattlePlayer
	g.Battle.Timer = 0
	g.Battle.Splash = 1.15
	g.Battle.Log = nil
	setBattleMessage(g, core.BattleEncounterMessage(*g))
}

func Update(g *core.GameState, dt float32) {
	updateBattleEffects(g, dt)
	if g.Battle.EnemyIndex < 0 || g.Battle.EnemyIndex >= len(g.Enemies) {
		g.Battle.Phase = core.BattleNone
		return
	}
	if core.LivingBattleCount(g) == 0 && g.Battle.Phase != core.BattleWon {
		g.Battle.Phase = core.BattleNone
		return
	}
	if g.Battle.Phase != core.BattleWon && g.Battle.Phase != core.BattleLost && core.LivingPartyCount(g.Party) == 0 {
		loseBattle(g, "The party is driven back. Press Enter to recover.")
		return
	}
	if g.Battle.Splash > 0 {
		g.Battle.Splash -= dt
		if g.Battle.Splash < 0 {
			g.Battle.Splash = 0
		}
	}

	switch g.Battle.Phase {
	case core.BattlePlayer:
		updatePlayerBattle(g)
	case core.BattleEnemy:
		g.Battle.Timer -= dt
		if g.Battle.Timer > 0 {
			return
		}
		burns := resolveBurns(g)
		if core.LivingBattleCount(g) == 0 {
			winBattle(g, "The fire finishes them.")
			return
		}
		hits := resolveEnemyAttacks(g)
		if core.LivingPartyCount(g.Party) == 0 {
			loseBattle(g, core.BattleLossMessage(*g))
			return
		}
		g.Battle.Phase = core.BattlePlayer
		beginPartyTurn(g, core.FirstLivingPartyMember(g.Party))
		setBattleMessage(g, core.BattleEnemyAttackMessage(*g, hits, burns))
	case core.BattleWon:
		g.Battle.Timer -= dt
		if g.Battle.Timer <= 0 && !battleDeathFadeActive(g) {
			leaveBattle(g, "The field is quiet.")
		}
	case core.BattleLost:
		if input.ConfirmPressed() {
			recoverFromLoss(g)
		}
	}
}

func winBattle(g *core.GameState, message string) {
	g.Battle.Phase = core.BattleWon
	g.Battle.Timer = core.VictoryDanceDuration
	resetBattleAction(g)
	setBattleMessage(g, message)
}

func loseBattle(g *core.GameState, message string) {
	g.Battle.Phase = core.BattleLost
	resetBattleAction(g)
	setBattleMessage(g, message)
}

func leaveBattle(g *core.GameState, message string) {
	g.Battle.EnemyIndex = -1
	g.Battle.EnemyGroup = nil
	resetBattleAction(g)
	g.Battle.Phase = core.BattleNone
	setBattleMessage(g, message)
}

func recoverFromLoss(g *core.GameState) {
	core.ResetGameState(g)
	setBattleMessage(g, "You catch your breath.")
}

func updatePlayerBattle(g *core.GameState) {
	if !core.PartyMemberAlive(g.Party, g.Battle.CurrentParty) {
		beginPartyTurn(g, core.FirstLivingPartyMember(g.Party))
		if g.Battle.CurrentParty < 0 {
			loseBattle(g, "The party is driven back. Press Enter to recover.")
			return
		}
	}
	if g.Battle.EnemyIndex < 0 || g.Battle.EnemyIndex >= len(g.Enemies) || !g.Enemies[g.Battle.EnemyIndex].Alive {
		if next := core.NextLivingBattleEnemy(g); next >= 0 {
			g.Battle.EnemyIndex = next
		}
	}

	switch g.Battle.ActionMode {
	case core.ActionEnemyTarget:
		updateEnemyTargeting(g)
	case core.ActionPartyTarget:
		updatePartyTargeting(g)
	default:
		updateActionMenu(g)
	}
}

func beginPartyTurn(g *core.GameState, partyIndex int) {
	g.Battle.CurrentParty = partyIndex
	resetBattleAction(g)
	if partyIndex >= 0 && partyIndex < len(g.Party) {
		g.Battle.PartyTarget = partyIndex
	} else {
		g.Battle.PartyTarget = core.FirstLivingPartyMember(g.Party)
	}
	if g.Battle.PartyTarget < 0 {
		g.Battle.PartyTarget = 0
	}
}

func resetBattleAction(g *core.GameState) {
	g.Battle.ActionMode = core.ActionMenu
	g.Battle.MenuIndex = 0
	g.Battle.PendingSkill = core.SkillNone
}

func setBattleStatus(g *core.GameState, message string) {
	g.Battle.Message = message
}

func setBattleMessage(g *core.GameState, message string) {
	g.Battle.Message = message
	if message == "" {
		return
	}
	if len(g.Battle.Log) > 0 && g.Battle.Log[len(g.Battle.Log)-1] == message {
		return
	}
	g.Battle.Log = append(g.Battle.Log, message)
	if len(g.Battle.Log) > 40 {
		g.Battle.Log = g.Battle.Log[len(g.Battle.Log)-40:]
	}
}

func updateBattleEffects(g *core.GameState, dt float32) {
	for i := range g.Party {
		g.Party[i].AttackBump = core.ApproachZero(g.Party[i].AttackBump, dt)
		g.Party[i].DamageFlash = core.ApproachZero(g.Party[i].DamageFlash, dt)
	}
	for i := range g.Enemies {
		g.Enemies[i].AttackBump = core.ApproachZero(g.Enemies[i].AttackBump, dt)
		g.Enemies[i].DamageFlash = core.ApproachZero(g.Enemies[i].DamageFlash, dt)
		g.Enemies[i].DeathFade = core.ApproachZero(g.Enemies[i].DeathFade, dt)
	}
}

func battleDeathFadeActive(g *core.GameState) bool {
	for _, index := range g.Battle.EnemyGroup {
		if index >= 0 && index < len(g.Enemies) && g.Enemies[index].DeathFade > 0 {
			return true
		}
	}
	return false
}

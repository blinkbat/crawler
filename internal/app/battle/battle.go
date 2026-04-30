package battle

import (
	"crawler/internal/app/core"
	"fmt"
)

func Start(g *core.GameState, enemyIndex int) {
	group := nearbyBattleGroup(g.Enemies, enemyIndex)
	g.Battle.EnemyIndex = enemyIndex
	g.Battle.EnemyGroup = group
	g.Battle.CurrentParty = firstLivingPartyMember(g.Party)
	g.Battle.ActionMode = core.ActionMenu
	g.Battle.MenuIndex = 0
	g.Battle.PendingSkill = core.SkillNone
	g.Battle.PartyTarget = firstLivingPartyMember(g.Party)
	g.Battle.Phase = core.BattlePlayer
	g.Battle.Timer = 0
	g.Battle.Splash = 1.15
	g.Battle.Log = nil
	if len(group) == 1 {
		setBattleMessage(g, "A rat blocks the way.")
	} else {
		setBattleMessage(g, fmt.Sprintf("%d rats close in.", len(group)))
	}
}

func Update(g *core.GameState, dt float32) {
	updateBattleEffects(g, dt)
	if g.Battle.EnemyIndex < 0 || g.Battle.EnemyIndex >= len(g.Enemies) {
		g.Battle.Phase = core.BattleNone
		return
	}
	if LivingBattleCount(g) == 0 && g.Battle.Phase != core.BattleWon {
		g.Battle.Phase = core.BattleNone
		return
	}
	if g.Battle.Phase != core.BattleWon && livingPartyCount(g.Party) == 0 {
		g.Battle.Phase = core.BattleLost
		setBattleMessage(g, "The party is driven back. Press Enter to recover.")
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
		if LivingBattleCount(g) == 0 {
			g.Battle.Phase = core.BattleWon
			g.Battle.Timer = core.VictoryDanceDuration
			setBattleMessage(g, "The fire finishes them.")
			return
		}
		hits := resolveRatAttacks(g)
		if livingPartyCount(g.Party) == 0 {
			g.Battle.Phase = core.BattleLost
			setBattleMessage(g, "The rats drive the party back. Press Enter to recover.")
			return
		}
		g.Battle.Phase = core.BattlePlayer
		beginPartyTurn(g, firstLivingPartyMember(g.Party))
		if burns > 0 && hits > 1 {
			setBattleMessage(g, fmt.Sprintf("Flames bite. %d rats snap at the party.", hits))
		} else if burns > 0 && hits == 1 {
			setBattleMessage(g, "Flames bite. A rat snaps at the party.")
		} else if hits == 1 {
			setBattleMessage(g, "A rat snaps at the party.")
		} else {
			setBattleMessage(g, fmt.Sprintf("%d rats snap at the party.", hits))
		}
	case core.BattleWon:
		g.Battle.Timer -= dt
		if g.Battle.Timer <= 0 && !battleDeathFadeActive(g) {
			g.Battle.EnemyIndex = -1
			g.Battle.EnemyGroup = nil
			g.Battle.PendingSkill = core.SkillNone
			g.Battle.Phase = core.BattleNone
			setBattleMessage(g, "The dungeon is quiet.")
		}
	case core.BattleLost:
		if confirmPressed() {
			recoverFromLoss(g)
		}
	}
}

func recoverFromLoss(g *core.GameState) {
	core.ResetGameState(g)
	setBattleMessage(g, "You catch your breath.")
}

func updatePlayerBattle(g *core.GameState) {
	if !PartyMemberAlive(g.Party, g.Battle.CurrentParty) {
		beginPartyTurn(g, firstLivingPartyMember(g.Party))
		if g.Battle.CurrentParty < 0 {
			g.Battle.Phase = core.BattleLost
			setBattleMessage(g, "The party is driven back. Press Enter to recover.")
			return
		}
	}
	if g.Battle.EnemyIndex < 0 || g.Battle.EnemyIndex >= len(g.Enemies) || !g.Enemies[g.Battle.EnemyIndex].Alive {
		if next := nextLivingBattleEnemy(g); next >= 0 {
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
	g.Battle.ActionMode = core.ActionMenu
	g.Battle.MenuIndex = 0
	g.Battle.PendingSkill = core.SkillNone
	if partyIndex >= 0 && partyIndex < len(g.Party) {
		g.Battle.PartyTarget = partyIndex
	} else {
		g.Battle.PartyTarget = firstLivingPartyMember(g.Party)
	}
	if g.Battle.PartyTarget < 0 {
		g.Battle.PartyTarget = 0
	}
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

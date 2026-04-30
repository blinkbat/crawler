package battle

import (
	"crawler/internal/app/core"
	"fmt"
)

func nearbyBattleGroup(enemies []core.Enemy, trigger int) []int {
	if trigger < 0 || trigger >= len(enemies) || !enemies[trigger].Alive {
		return nil
	}
	type candidate struct {
		index int
		dist  int
	}
	triggerEnemy := enemies[trigger]
	candidates := []candidate{{index: trigger, dist: -1}}
	for i, enemy := range enemies {
		if i == trigger || !enemy.Alive {
			continue
		}
		dist := core.AbsInt(enemy.TileX-triggerEnemy.TileX) + core.AbsInt(enemy.TileZ-triggerEnemy.TileZ)
		if dist <= 2 {
			candidates = append(candidates, candidate{index: i, dist: dist})
		}
	}
	for i := 1; i < len(candidates); i++ {
		for j := i; j > 0 && candidates[j].dist < candidates[j-1].dist; j-- {
			candidates[j], candidates[j-1] = candidates[j-1], candidates[j]
		}
	}
	limit := len(candidates)
	if limit > 3 {
		limit = 3
	}
	group := make([]int, 0, limit)
	for i := 0; i < limit; i++ {
		group = append(group, candidates[i].index)
	}
	return group
}

func LivingBattleCount(g *core.GameState) int {
	count := 0
	for _, index := range g.Battle.EnemyGroup {
		if index >= 0 && index < len(g.Enemies) && g.Enemies[index].Alive {
			count++
		}
	}
	return count
}

func nextLivingBattleEnemy(g *core.GameState) int {
	for _, index := range g.Battle.EnemyGroup {
		if index >= 0 && index < len(g.Enemies) && g.Enemies[index].Alive {
			return index
		}
	}
	return -1
}

func livingPartyCount(party []core.PartyMember) int {
	count := 0
	for _, member := range party {
		if member.HP > 0 {
			count++
		}
	}
	return count
}

func PartyMemberAlive(party []core.PartyMember, index int) bool {
	return index >= 0 && index < len(party) && party[index].HP > 0
}

func firstLivingPartyMember(party []core.PartyMember) int {
	return nextLivingPartyMember(party, 0)
}

func nextLivingPartyMember(party []core.PartyMember, start int) int {
	for i := start; i < len(party); i++ {
		if party[i].HP > 0 {
			return i
		}
	}
	return -1
}

func BattleTargetOrdinal(g core.GameState) int {
	ordinal := 1
	for _, index := range g.Battle.EnemyGroup {
		if index < 0 || index >= len(g.Enemies) || !g.Enemies[index].Alive {
			continue
		}
		if index == g.Battle.EnemyIndex {
			return ordinal
		}
		ordinal++
	}
	return 1
}

func TurnForecast(g core.GameState, limit int) []string {
	labels := make([]string, 0, limit)
	if g.Battle.Phase == core.BattleNone || limit <= 0 {
		return labels
	}

	startParty := g.Battle.CurrentParty
	enemyFirst := g.Battle.Phase == core.BattleEnemy
	for len(labels) < limit {
		if enemyFirst {
			appendEnemyTurn(&labels, g, limit)
			enemyFirst = false
			startParty = 0
		}
		for i := startParty; i < len(g.Party) && len(labels) < limit; i++ {
			if g.Party[i].HP > 0 {
				labels = append(labels, g.Party[i].Name)
			}
		}
		appendEnemyTurn(&labels, g, limit)
		startParty = 0
		if livingPartyCount(g.Party) == 0 || LivingBattleCount(&g) == 0 {
			break
		}
	}
	return labels
}

func appendEnemyTurn(labels *[]string, g core.GameState, limit int) {
	if len(*labels) >= limit {
		return
	}
	count := LivingBattleCount(&g)
	if count <= 0 {
		return
	}
	if count == 1 {
		*labels = append(*labels, "Rat")
		return
	}
	*labels = append(*labels, fmt.Sprintf("Rats x%d", count))
}

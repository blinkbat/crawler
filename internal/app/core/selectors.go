package core

import "fmt"

type TurnEntry struct {
	Label string
	Class PartyClass
	Enemy bool
}

func PartyHPTotals(party []PartyMember) (int, int) {
	hp := 0
	maxHP := 0
	for _, member := range party {
		hp += member.HP
		maxHP += member.MaxHP
	}
	return hp, maxHP
}

func LivingPartyCount(party []PartyMember) int {
	count := 0
	for _, member := range party {
		if member.HP > 0 {
			count++
		}
	}
	return count
}

func PartyMemberAlive(party []PartyMember, index int) bool {
	return index >= 0 && index < len(party) && party[index].HP > 0
}

func FirstLivingPartyMember(party []PartyMember) int {
	return NextLivingPartyMember(party, 0)
}

func NextLivingPartyMember(party []PartyMember, start int) int {
	if start < 0 {
		start = 0
	}
	for i := start; i < len(party); i++ {
		if party[i].HP > 0 {
			return i
		}
	}
	return -1
}

func LivingPartyTargets(party []PartyMember) []int {
	living := make([]int, 0, len(party))
	for i := range party {
		if party[i].HP > 0 {
			living = append(living, i)
		}
	}
	return living
}

func EnemyAlive(enemies []Enemy, index int) bool {
	return index >= 0 && index < len(enemies) && enemies[index].Alive
}

func BattleContainsEnemy(b Battle, index int) bool {
	for _, enemyIndex := range b.EnemyGroup {
		if enemyIndex == index {
			return true
		}
	}
	return false
}

func LivingBattleEnemyIndices(g *GameState) []int {
	living := make([]int, 0, len(g.Battle.EnemyGroup))
	for _, index := range g.Battle.EnemyGroup {
		if EnemyAlive(g.Enemies, index) {
			living = append(living, index)
		}
	}
	return living
}

func LivingBattleCount(g *GameState) int {
	return len(LivingBattleEnemyIndices(g))
}

func NextLivingBattleEnemy(g *GameState) int {
	for _, index := range g.Battle.EnemyGroup {
		if EnemyAlive(g.Enemies, index) {
			return index
		}
	}
	return -1
}

func BattleTargetOrdinal(g GameState) int {
	ordinal := 1
	for _, index := range g.Battle.EnemyGroup {
		if !EnemyAlive(g.Enemies, index) {
			continue
		}
		if index == g.Battle.EnemyIndex {
			return ordinal
		}
		ordinal++
	}
	return 1
}

func TurnForecast(g GameState, limit int) []TurnEntry {
	turns := make([]TurnEntry, 0, limit)
	if g.Battle.Phase == BattleNone || limit <= 0 {
		return turns
	}

	startParty := g.Battle.CurrentParty
	if startParty < 0 {
		startParty = 0
	}
	enemyFirst := g.Battle.Phase == BattleEnemy
	for len(turns) < limit {
		if enemyFirst {
			appendEnemyTurn(&turns, g, limit)
			enemyFirst = false
			startParty = 0
		}
		for i := startParty; i < len(g.Party) && len(turns) < limit; i++ {
			if g.Party[i].HP > 0 {
				turns = append(turns, TurnEntry{Label: g.Party[i].Name, Class: g.Party[i].Class})
			}
		}
		appendEnemyTurn(&turns, g, limit)
		startParty = 0
		if LivingPartyCount(g.Party) == 0 || LivingBattleCount(&g) == 0 {
			break
		}
	}
	return turns
}

func appendEnemyTurn(turns *[]TurnEntry, g GameState, limit int) {
	if len(*turns) >= limit {
		return
	}
	count := LivingBattleCount(&g)
	if count <= 0 {
		return
	}
	label := "Rat"
	if count > 1 {
		label = fmt.Sprintf("Rats x%d", count)
	}
	*turns = append(*turns, TurnEntry{Label: label, Enemy: true})
}

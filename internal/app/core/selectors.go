package core

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
	count := 0
	for _, index := range g.Battle.EnemyGroup {
		if EnemyAlive(g.Enemies, index) {
			count++
		}
	}
	return count
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

// TurnForecast walks the scheduled queue from the current cursor and emits a
// per-actor preview. Supports up to `limit` entries, looping into the next
// round if the current one runs short. Each entry is one actor (a single
// party member or a single enemy slot), since mixed initiative interleaves
// them — there's no longer a "block of enemies" to collapse.
func TurnForecast(g GameState, limit int) []TurnEntry {
	turns := make([]TurnEntry, 0, limit)
	phase := g.Battle.Phase
	active := phase == BattlePlayer || phase == BattleAttackTiming || phase == BattleEnemyTiming
	if !active || limit <= 0 || len(g.Battle.Queue) == 0 {
		return turns
	}

	cursor := g.Battle.QueueCursor
	if cursor < 0 {
		cursor = 0
	}
	// First pass: rest of this round.
	for cursor < len(g.Battle.Queue) && len(turns) < limit {
		if entry, ok := turnEntryFor(g, g.Battle.Queue[cursor]); ok {
			turns = append(turns, entry)
		}
		cursor++
	}
	// If we still have room, fall through to the cached "next round"
	// projection that was built when the current round started. Actors
	// that have since died are skipped at render time by turnEntryFor.
	for _, actor := range g.Battle.NextRoundQueue {
		if len(turns) >= limit {
			break
		}
		if entry, ok := turnEntryFor(g, actor); ok {
			turns = append(turns, entry)
		}
	}
	return turns
}

// turnEntryFor materializes one queue actor into a TurnEntry. Skips dead
// actors (returns false). Used by TurnForecast.
func turnEntryFor(g GameState, actor ActorRef) (TurnEntry, bool) {
	if actor.IsParty {
		if actor.Index < 0 || actor.Index >= len(g.Party) || g.Party[actor.Index].HP <= 0 {
			return TurnEntry{}, false
		}
		p := g.Party[actor.Index]
		return TurnEntry{Label: p.Name, Class: p.Class}, true
	}
	if actor.Index < 0 || actor.Index >= len(g.Battle.EnemyGroup) {
		return TurnEntry{}, false
	}
	enemyIdx := g.Battle.EnemyGroup[actor.Index]
	if !EnemyAlive(g.Enemies, enemyIdx) {
		return TurnEntry{}, false
	}
	return TurnEntry{Label: EnemyInfoFor(g.Enemies[enemyIdx]).SingularName, Enemy: true}, true
}


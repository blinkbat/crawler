package core

type TurnEntry struct {
	Label string
	Class PartyClass
	Enemy bool
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

// WrapNextLivingPartyMember walks the party forward from `start`, wrapping to
// index 0 once it falls off the end. Returns -1 only when no member is alive.
// Used by the enemy round-robin attack cursor so the wrap behavior lives in
// one helper instead of being implicit in callers.
func WrapNextLivingPartyMember(party []PartyMember, start int) int {
	if len(party) == 0 {
		return -1
	}
	start = WrapIndex(start, len(party))
	for offset := 0; offset < len(party); offset++ {
		i := WrapIndex(start+offset, len(party))
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

// EnemyAlive checks whether the slot in the given slice is in-bounds and
// holds a live enemy. Works for either the active pack's Members or any
// other []Enemy the caller hands in.
func EnemyAlive(enemies []Enemy, index int) bool {
	return index >= 0 && index < len(enemies) && enemies[index].Alive
}

// BattleMembers returns the active pack's member slice, or nil when no
// pack is engaged. Callers that need write access through the slice (HP,
// flags, etc.) can index it directly since &Members[i] is stable for the
// lifetime of the pack.
func BattleMembers(g *GameState) []Enemy {
	if g.Battle.ActivePack < 0 || g.Battle.ActivePack >= len(g.Packs) {
		return nil
	}
	return g.Packs[g.Battle.ActivePack].Members
}

// BattleMemberAt returns a write-through pointer to one member of the
// active pack, or nil if the slot is invalid.
func BattleMemberAt(g *GameState, slot int) *Enemy {
	if g.Battle.ActivePack < 0 || g.Battle.ActivePack >= len(g.Packs) {
		return nil
	}
	members := g.Packs[g.Battle.ActivePack].Members
	if slot < 0 || slot >= len(members) {
		return nil
	}
	return &members[slot]
}

// BattleEnemyAlive is the slot-aware alive check for the active pack.
func BattleEnemyAlive(g *GameState, slot int) bool {
	m := BattleMemberAt(g, slot)
	return m != nil && m.Alive
}

// LivingBattleEnemyIndices returns the slot indices of every alive member
// in the active pack — used by the player's target cycler.
func LivingBattleEnemyIndices(g *GameState) []int {
	members := BattleMembers(g)
	living := make([]int, 0, len(members))
	for i, m := range members {
		if m.Alive {
			living = append(living, i)
		}
	}
	return living
}

func LivingBattleCount(g *GameState) int {
	count := 0
	for _, m := range BattleMembers(g) {
		if m.Alive {
			count++
		}
	}
	return count
}

func NextLivingBattleEnemy(g *GameState) int {
	for i, m := range BattleMembers(g) {
		if m.Alive {
			return i
		}
	}
	return -1
}

// SelectedEnemySlot returns the active pack slot the player's enemy cursor
// is pointing at, or -1 when no battle is active or the slot is out of
// range. Render-side targeting markers should query this instead of
// reading g.Battle.EnemyIndex directly — keeps the "is the marker valid"
// guard in one place so a future enemy-list compaction can't surprise a
// renderer that didn't bounds-check.
func SelectedEnemySlot(g *GameState) int {
	members := BattleMembers(g)
	if len(members) == 0 {
		return -1
	}
	if g.Battle.EnemyIndex < 0 || g.Battle.EnemyIndex >= len(members) {
		return -1
	}
	return g.Battle.EnemyIndex
}

// HighlightedAllyIndex returns the party index the player's ally cursor is
// pointing at (heal-skill or item-target modes), or -1 when no ally is
// currently being targeted or the slot is out of range. Counterpart to
// SelectedEnemySlot for the friendly side of the marker logic.
func HighlightedAllyIndex(g *GameState) int {
	if g.Battle.PartyTarget < 0 || g.Battle.PartyTarget >= len(g.Party) {
		return -1
	}
	return g.Battle.PartyTarget
}

// ActiveActorIndex returns the party index whose turn it currently is, or
// -1 when no party member is acting (enemy turn, splash, post-battle).
// Lets renderers light up the "your turn" highlight without re-deriving
// the phase check at each call site.
func ActiveActorIndex(g *GameState) int {
	switch g.Battle.Phase {
	case BattlePlayer, BattleAttackTiming:
		if g.Battle.CurrentParty < 0 || g.Battle.CurrentParty >= len(g.Party) {
			return -1
		}
		return g.Battle.CurrentParty
	}
	return -1
}

// PackLeaderSlot returns the slot of the pack's leader: the highest-Tier
// member, ties broken by member order. Empty packs return 0 (callers
// should range-check before drawing).
func PackLeaderSlot(p Pack) int {
	bestSlot := 0
	bestTier := -1
	for i, m := range p.Members {
		t := EnemyInfo(m.Kind).Tier
		if t > bestTier {
			bestTier = t
			bestSlot = i
		}
	}
	return bestSlot
}

// PackLeader returns the highest-Tier member of the pack, or a zero
// Enemy when the pack is empty.
func PackLeader(p Pack) Enemy {
	if len(p.Members) == 0 {
		return Enemy{}
	}
	return p.Members[PackLeaderSlot(p)]
}

// PackLeaderKindSlot is the EnemyKind-only variant of PackLeaderSlot.
// Editor tooling works with []EnemyKind (pre-spawn pack specs) and can't
// build a full []Enemy just to ask "which member draws as the field
// icon?" — this keeps the highest-Tier-wins rule in one place.
func PackLeaderKindSlot(kinds []EnemyKind) int {
	bestSlot := 0
	bestTier := -1
	for i, k := range kinds {
		t := EnemyInfo(k).Tier
		if t > bestTier {
			bestTier = t
			bestSlot = i
		}
	}
	return bestSlot
}

// PackLeaderKind returns the highest-Tier EnemyKind in the slice, or
// EnemyRat when the slice is empty (matches the editor's fallback for an
// empty pack spec). Ties are broken by member order.
func PackLeaderKind(kinds []EnemyKind) EnemyKind {
	if len(kinds) == 0 {
		return EnemyRat
	}
	return kinds[PackLeaderKindSlot(kinds)]
}

// PackXPValue is the sum of XPValue across every member of a pack —
// the loot pool every living member earns when this pack falls. Per-
// character (not split), so a 3-rat pack pays 15 XP to each survivor.
func PackXPValue(p Pack) int {
	total := 0
	for _, m := range p.Members {
		total += EnemyInfo(m.Kind).XPValue
	}
	return total
}

// AwardBattleXP grants the active pack's PackXPValue to every living
// party member and processes level-ups. Returns the per-member XP
// amount and the indices of members who gained at least one level
// (for log messages). Called from winBattle right after the kill is
// confirmed but before the victory timer / level-up modal trigger.
func AwardBattleXP(g *GameState) (perMember int, leveledIndices []int) {
	if g.Battle.ActivePack < 0 || g.Battle.ActivePack >= len(g.Packs) {
		return 0, nil
	}
	perMember = PackXPValue(g.Packs[g.Battle.ActivePack])
	if perMember <= 0 {
		return 0, nil
	}
	for i := range g.Party {
		if g.Party[i].HP <= 0 {
			continue
		}
		if AddXP(&g.Party[i], perMember) > 0 {
			leveledIndices = append(leveledIndices, i)
		}
	}
	return perMember, leveledIndices
}

// PackAlive reports whether any member of the pack is still alive.
func PackAlive(p Pack) bool {
	for _, m := range p.Members {
		if m.Alive {
			return true
		}
	}
	return false
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
	members := BattleMembers(&g)
	if actor.Index < 0 || actor.Index >= len(members) {
		return TurnEntry{}, false
	}
	enemy := members[actor.Index]
	if !enemy.Alive {
		return TurnEntry{}, false
	}
	return TurnEntry{Label: EnemyInfoFor(enemy).SingularName, Enemy: true}, true
}

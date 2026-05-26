package core

type TurnEntry struct {
	Label string
	Class PartyClass
	Enemy bool
}

// --- Generic walking primitives ----------------------------------------
//
// Five small helpers fold the eight party walkers + three battle-enemy
// walkers that used to inline the same loop shape with different
// predicates. The public functions below (LivingPartyCount,
// FirstLivingPartyMember, WrapNextAvailablePartyMember, etc.) all
// resolve through these so a future "filter by class / status" needs
// one new predicate + one new wrapper, not another hand-rolled loop.

// indicesWhere returns the indices of every element matching `pred`.
func indicesWhere[T any](slice []T, pred func(T) bool) []int {
	out := make([]int, 0, len(slice))
	for i, v := range slice {
		if pred(v) {
			out = append(out, i)
		}
	}
	return out
}

// countWhere returns the number of elements matching `pred`.
func countWhere[T any](slice []T, pred func(T) bool) int {
	n := 0
	for _, v := range slice {
		if pred(v) {
			n++
		}
	}
	return n
}

// nextWhere returns the first index ≥ start where pred is true, or -1.
// Clamps start to 0 for negative inputs so a fresh "find the first
// matching slot" caller can pass -1 / 0 interchangeably.
func nextWhere[T any](slice []T, start int, pred func(T) bool) int {
	if start < 0 {
		start = 0
	}
	for i := start; i < len(slice); i++ {
		if pred(slice[i]) {
			return i
		}
	}
	return -1
}

// wrapNextWhere walks forward from `start`, wrapping to index 0 once
// it falls off the end, and returns the first matching index. Returns
// -1 only when no element matches.
func wrapNextWhere[T any](slice []T, start int, pred func(T) bool) int {
	if len(slice) == 0 {
		return -1
	}
	start = WrapIndex(start, len(slice))
	for offset := 0; offset < len(slice); offset++ {
		i := WrapIndex(start+offset, len(slice))
		if pred(slice[i]) {
			return i
		}
	}
	return -1
}

// partyAlive / partyAvailable / enemyAlive are the canonical predicates
// the walkers feed through indicesWhere / countWhere / nextWhere /
// wrapNextWhere. Lifting them out keeps the "alive AND not ingested"
// definition in one spot — earlier passes inlined the `HP > 0 &&
// !Ingested` check at every call site.
func partyAlive(m PartyMember) bool     { return m.HP > 0 }
func partyAvailable(m PartyMember) bool { return m.HP > 0 && !m.Ingested }
func enemyAlive(e Enemy) bool           { return e.Alive }

func LivingPartyCount(party []PartyMember) int {
	return countWhere(party, partyAlive)
}

func PartyMemberAlive(party []PartyMember, index int) bool {
	return index >= 0 && index < len(party) && party[index].HP > 0
}

// PartyMemberAvailable reports whether the member at index can act / be
// targeted this turn — alive AND not currently ingested by a mantrap.
// Ingested members are alive (their HP is preserved while inside the
// plant) but they're functionally out of the fight, so every "is this
// slot a valid actor / target?" predicate routes through this helper
// rather than the bare HP>0 check.
func PartyMemberAvailable(party []PartyMember, index int) bool {
	if !PartyMemberAlive(party, index) {
		return false
	}
	return !party[index].Ingested
}

// ActivePartyCount counts party members who can still act this turn —
// alive AND not ingested. Drives the loss check: when the count falls to
// zero, the encounter ends (a party that's all ingested has nobody to
// continue the fight, same effect as a wipe).
func ActivePartyCount(party []PartyMember) int {
	return countWhere(party, partyAvailable)
}

// WrapNextAvailablePartyMember walks the party forward from `start`,
// wrapping to index 0 once it falls off the end, and returns the first
// available (alive + not ingested) slot. Returns -1 when nobody is
// available. Used by the enemy attack cursor so a swallowed prey
// doesn't get bitten on top of the lockout.
func WrapNextAvailablePartyMember(party []PartyMember, start int) int {
	return wrapNextWhere(party, start, partyAvailable)
}

// AvailablePartyTargets returns the indices of every member who can be
// chosen as a heal/item target this turn. Mirrors LivingPartyTargets
// but excludes ingested members; both target cyclers (heal skill, item
// use) route through this so a Cleric can't waste a Prayer trying to
// reach the prey inside a mantrap.
func AvailablePartyTargets(party []PartyMember) []int {
	return indicesWhere(party, partyAvailable)
}

// FirstAvailablePartyMember is the alive-and-not-ingested companion to
// FirstLivingPartyMember. Used as the fallback when an enemy spell needs
// a usable target (Ingest, Sleep) but the picker's preferred index was
// already out of reach.
func FirstAvailablePartyMember(party []PartyMember) int {
	return nextWhere(party, 0, partyAvailable)
}

// MantrapHasPrey reports whether any party member is currently being
// digested by the active-pack member at `slot`. Used by the mantrap AI
// to gate SkillIngest — a plant with prey can't snatch another.
func MantrapHasPrey(party []PartyMember, slot int) bool {
	for _, m := range party {
		if m.Ingested && m.IngestedBy == slot {
			return true
		}
	}
	return false
}

// ReleaseIngestedBy frees every party member currently held by the
// active-pack slot. Called from damageEnemy on the killing blow and
// from clearBattleResidual on any battle exit. Returns the indices of
// the members that were freed so the caller can log "X breaks free."
func ReleaseIngestedBy(party []PartyMember, slot int) []int {
	var freed []int
	for i := range party {
		if party[i].Ingested && party[i].IngestedBy == slot {
			party[i].Ingested = false
			party[i].IngestedBy = -1
			freed = append(freed, i)
		}
	}
	return freed
}

// ReleaseAllIngested frees every ingested party member regardless of
// swallower. Called on battle exit so a desynced encounter (forced
// recovery, F5 playtest restart) doesn't leak the lockout into the
// next session.
func ReleaseAllIngested(party []PartyMember) {
	for i := range party {
		if party[i].Ingested {
			party[i].Ingested = false
			party[i].IngestedBy = -1
		}
	}
}

// ModalKind tags the active full-screen modal in the exploration
// scene. Listed in priority order — higher values "win" over lower
// when multiple flags are set, mirroring the explore.Update gate
// sequence. ActiveModal is the single source of truth for "what
// modal is on top right now?" so a future caller (editor return
// path, scripted-event modal, etc.) doesn't re-derive the order.
type ModalKind int

const (
	ModalNone ModalKind = iota
	ModalPauseMenu
	ModalDebugMenu
	ModalChest
	ModalPanels
	ModalLevelUp
)

// ActiveModal returns the highest-priority modal currently open in
// the exploration scene, or ModalNone if movement / battle should
// process input. Mirrors the priority ladder in explore.Update:
// level-up > panels > chest > pause > simulation. The battle phase
// is NOT modeled here — it's a separate scene-level mode, not a
// modal overlay.
func ActiveModal(g *GameState) ModalKind {
	if g == nil {
		return ModalNone
	}
	switch {
	case g.LevelUpOpen:
		return ModalLevelUp
	case g.PanelsOpen:
		return ModalPanels
	case g.ChestOpen >= 0:
		return ModalChest
	case g.DebugMenuOpen:
		return ModalDebugMenu
	case g.MenuOpen:
		return ModalPauseMenu
	}
	return ModalNone
}

func FirstLivingPartyMember(party []PartyMember) int {
	return nextWhere(party, 0, partyAlive)
}

func NextLivingPartyMember(party []PartyMember, start int) int {
	return nextWhere(party, start, partyAlive)
}

// WrapNextLivingPartyMember walks the party forward from `start`, wrapping to
// index 0 once it falls off the end. Returns -1 only when no member is alive.
// Used by the enemy round-robin attack cursor so the wrap behavior lives in
// one helper instead of being implicit in callers.
func WrapNextLivingPartyMember(party []PartyMember, start int) int {
	return wrapNextWhere(party, start, partyAlive)
}

func LivingPartyTargets(party []PartyMember) []int {
	return indicesWhere(party, partyAlive)
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
	return indicesWhere(BattleMembers(g), enemyAlive)
}

func LivingBattleCount(g *GameState) int {
	return countWhere(BattleMembers(g), enemyAlive)
}

func NextLivingBattleEnemy(g *GameState) int {
	return nextWhere(BattleMembers(g), 0, enemyAlive)
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
		t := EnemyInfoFor(m).Tier
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

// PackXPValue is the sum of XPValue across every member of a pack —
// the loot pool every living member earns when this pack falls. Per-
// character (not split), so a 3-rat pack pays 15 XP to each survivor.
func PackXPValue(p Pack) int {
	total := 0
	for _, m := range p.Members {
		total += EnemyInfoFor(m).XPValue
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
//
// Thin wrapper around TurnForecastInto for callers that don't have a
// reusable buffer (tests, ad-hoc inspection). Render-side callers that
// run every frame should pass a persistent buffer through Into.
func TurnForecast(g *GameState, limit int) []TurnEntry {
	return TurnForecastInto(g, nil, limit)
}

// TurnForecastInto is the buffer-reuse variant of TurnForecast. When
// `into` has enough cap, it's truncated to length 0 and re-used; the
// returned slice shares its backing array. Lets the per-frame render
// caller keep a 7-entry buffer alive across frames so the forecast
// doesn't allocate on every panel draw during battle.
func TurnForecastInto(g *GameState, into []TurnEntry, limit int) []TurnEntry {
	if cap(into) < limit {
		into = make([]TurnEntry, 0, limit)
	} else {
		into = into[:0]
	}
	phase := g.Battle.Phase
	active := phase == BattlePlayer || phase == BattleAttackTiming || phase == BattleEnemyTiming
	if !active || limit <= 0 || len(g.Battle.Queue) == 0 {
		return into
	}

	cursor := g.Battle.QueueCursor
	if cursor < 0 {
		cursor = 0
	}
	// First pass: rest of this round.
	for cursor < len(g.Battle.Queue) && len(into) < limit {
		if entry, ok := turnEntryFor(g, g.Battle.Queue[cursor]); ok {
			into = append(into, entry)
		}
		cursor++
	}
	// If we still have room, fall through to the cached "next round"
	// projection that was built when the current round started. Actors
	// that have since died are skipped at render time by turnEntryFor.
	for _, actor := range g.Battle.NextRoundQueue {
		if len(into) >= limit {
			break
		}
		if entry, ok := turnEntryFor(g, actor); ok {
			into = append(into, entry)
		}
	}
	return into
}

// turnEntryFor materializes one queue actor into a TurnEntry. Skips dead
// actors (returns false). Used by TurnForecast.
func turnEntryFor(g *GameState, actor ActorRef) (TurnEntry, bool) {
	if actor.IsParty {
		if !actor.ValidPartyIndex(g.Party) || g.Party[actor.Index].HP <= 0 {
			return TurnEntry{}, false
		}
		p := g.Party[actor.Index]
		return TurnEntry{Label: p.Name, Class: p.Class}, true
	}
	members := BattleMembers(g)
	if actor.Index < 0 || actor.Index >= len(members) {
		return TurnEntry{}, false
	}
	enemy := members[actor.Index]
	if !enemy.Alive {
		return TurnEntry{}, false
	}
	return TurnEntry{Label: EnemyInfoFor(enemy).SingularName, Enemy: true}, true
}

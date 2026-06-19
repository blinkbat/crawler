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

// indicesWhereInto is indicesWhere writing into a caller-owned buffer
// (re-sliced to length 0) — the allocation-free variant for per-frame
// callers. The returned slice aliases buf's backing array and is valid
// until the caller's next reuse of that buffer.
func indicesWhereInto[T any](buf []int, slice []T, pred func(T) bool) []int {
	buf = buf[:0]
	for i, v := range slice {
		if pred(v) {
			buf = append(buf, i)
		}
	}
	return buf
}

// filterInto copies every element of src matching `keep` into buf
// (re-sliced to length 0) and returns it — the value-preserving sibling of
// indicesWhereInto (which yields indices). The allocation-free filter shape
// behind LiveStacksInto / LiveConsumablesInto / SellableStacksInto (items.go,
// economy.go) and OutOfBattleHealsInto (party.go); pass nil to allocate. The
// returned slice aliases buf's backing array and is valid until the caller's
// next reuse of that buffer.
func filterInto[T any](buf []T, src []T, keep func(T) bool) []T {
	buf = buf[:0]
	for _, v := range src {
		if keep(v) {
			buf = append(buf, v)
		}
	}
	return buf
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

// LivingPartyIndices returns the seat indices of every member with HP > 0
// — the selectable set for the out-of-battle ally-target picker, where the
// Ingested status doesn't apply. (The in-battle target cyclers use
// AvailablePartyTargets, which additionally excludes ingested prey.)
func LivingPartyIndices(party []PartyMember) []int {
	return indicesWhere(party, partyAlive)
}

// LivingPartyIndicesInto is LivingPartyIndices into a caller-owned buffer —
// for the per-frame picker paths (use-target update + draw) that would
// otherwise allocate a fresh index slice every frame the picker is open.
func LivingPartyIndicesInto(buf []int, party []PartyMember) []int {
	return indicesWhereInto(buf, party, partyAlive)
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

// PeekNextEnemyTarget returns the party-member index the enemy side will
// attack next — the first available slot after EnemyAttackCursor — WITHOUT
// advancing the cursor. Returns -1 when nobody is available. The battle
// side's pickEnemyAttackTarget commits the cursor after calling this; the
// render side reads it to preview the incoming-hit marker non-mutatingly.
// Sharing this peek keeps the "who's next" rule from drifting between the
// two packages.
func PeekNextEnemyTarget(g *GameState) int {
	return WrapNextAvailablePartyMember(g.Party, g.Battle.EnemyAttackCursor+1)
}

// AvailablePartyTargets returns the indices of every member who can be
// chosen as a heal/item target this turn — living AND not ingested.
// Both target cyclers (heal skill, item use) route through this so a
// Cleric can't waste a Prayer trying to reach the prey inside a mantrap.
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

// curableStatusCounterCount is the number of turn-counter statuses that BOTH
// CureDebuffs and ClearPartyTransientStatuses clear — Sleep, Stun, Webbed,
// Confused. The init assert below pins transientStatusCounters to this length
// so a new status counter added to one clear-list (but forgotten in the other)
// trips at startup instead of leaving Cleanse and battle-exit silently out of
// sync on what a curable status is.
const curableStatusCounterCount = 4

// transientStatusCounters returns pointers to the shared curable status
// counters on a member — Sleep, Stun, Webbed, Confused. Both CureDebuffs (which
// also clears Poison) and ClearPartyTransientStatuses (which also clears the
// stance / buff / shield / regen fields) route their counter-clearing through
// this one list so the "what counts as a curable transient status" set can't
// drift between the two. NOT Poison: it lingers past a fight as a wound, so it
// rides CureDebuffs' own list, not this shared one.
func transientStatusCounters(m *PartyMember) []*int {
	return []*int{&m.SleepTurns, &m.StunTurns, &m.WebbedTurns, &m.ConfusedTurns}
}

func init() {
	// Pin the shared list's length so a new counter status added to the clear
	// path lands in transientStatusCounters (covering both callers) rather than
	// being inlined into just one of them.
	if got := len(transientStatusCounters(&PartyMember{})); got != curableStatusCounterCount {
		panic("core: transientStatusCounters length must equal curableStatusCounterCount — add the new status counter here so CureDebuffs and ClearPartyTransientStatuses stay in sync")
	}
}

// CureDebuffs clears the curable NEGATIVE combat statuses off a single member
// — Poison, Sleep, Stun, Webbed, Confused — and returns how many were active
// (so the caller can phrase "nothing to cure" vs "cured N"). It deliberately
// leaves the Defending stance and the positive Bless buff (BuffTurns) intact,
// and does NOT touch Ingested (a lockout, not a curable status — an ingested
// member is untargetable anyway). The Cleric's Cleanse is the caller; kept
// here beside ClearPartyTransientStatuses so the "what counts as a curable
// debuff" set lives in one place — the four shared counters come from
// transientStatusCounters, with Poison added on (a cure clears Poison; battle
// exit does not).
func CureDebuffs(m *PartyMember) int {
	if m == nil {
		return 0
	}
	cured := 0
	for _, c := range append(transientStatusCounters(m), &m.PoisonTurns) {
		if *c > 0 {
			cured++
			*c = 0
		}
	}
	return cured
}

// ClearPartyTransientStatuses wipes the combat-only status effects off
// every party member at battle exit — EXCEPT Poison, which lingers as a
// lasting wound. It never touches HP, so the dead stay dead. Per the
// current design call, only "dead" and "poisoned" survive a fight;
// Sleep / Stun / Webbed / Confused (the shared transientStatusCounters set),
// the Defending stance, and the positive Bless buff + Renewal regen all clear
// the moment the battle ends. (Ingest is released separately via
// ReleaseAllIngested, which also restores the swallowed member.)
func ClearPartyTransientStatuses(party []PartyMember) {
	for i := range party {
		m := &party[i]
		for _, c := range transientStatusCounters(m) {
			*c = 0
		}
		m.Defending = false
		m.Buffs = nil
		m.ShieldHP = 0
		m.IceArmorTurns = 0
		m.RegenTurns = 0
		m.RegenPerTurn = 0
	}
}

// clearPartyCombatTransients strips the combat-only state that must never
// outlive a battle: transient statuses, ingestion links, and per-member
// animation timers. The shared trio behind the save sanitizer, the load-path
// scrub, and field recovery — adding a new battle-only field to one of the
// underlying clearers covers all three callers at once. Note it deliberately
// preserves Poison; the full-restore caller (field recovery) zeroes that itself.
func clearPartyCombatTransients(party []PartyMember) {
	ClearPartyTransientStatuses(party) // Sleep / Stun / Webbed / Confused / Defending
	ReleaseAllIngested(party)          // Ingested / IngestedBy
	for i := range party {
		clearMemberAnimTimers(&party[i])
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
	ModalOptionsMenu
	ModalDebugMenu
	ModalRetroMenu
	ModalShop
	ModalDoorPrompt
	ModalChest
	ModalPanels
	ModalLevelUp
	// ModalDialog is the branching-conversation overlay. Highest priority —
	// a conversation in progress shouldn't be shadowed by any other overlay
	// (it's triggered into and blocks explore until it ends or is skipped).
	ModalDialog
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
	// ChestOpen / DoorPrompt are index-or-(-1) sentinels. Bound them
	// against their backing slice (not just `>= 0`) so a GameState built
	// by struct literal — where these zero-value to 0 — can't render a
	// phantom chest/door modal for an index that doesn't exist. With the
	// slice-length bound, a zero-value index only triggers when there's
	// actually an entry at that index, and an out-of-range sentinel is
	// inert. Production still goes through NewGameState (which sets -1).
	switch {
	case g.DialogOpen:
		return ModalDialog
	case g.LevelUpOpen:
		return ModalLevelUp
	case g.PanelsOpen:
		return ModalPanels
	case g.ShopOpen:
		// Opened from the pause menu (which clears MenuOpen), so in
		// practice nothing else is open alongside it; sits high in the
		// ladder so a stray lower flag can't shadow the shop's input.
		return ModalShop
	case g.ChestOpen >= 0 && g.ChestOpen < len(g.Chests):
		return ModalChest
	case g.DoorPrompt >= 0 && g.DoorPrompt < len(g.Doors):
		return ModalDoorPrompt
	case g.RetroMenuOpen:
		// Above the debug menu: it's the debug menu's child and replaces it
		// on screen (openRetroMenu clears DebugMenuOpen, same hand-off as
		// pause → debug), but the priority slot keeps a stray double-open
		// from letting the parent shadow the child's input.
		return ModalRetroMenu
	case g.DebugMenuOpen:
		return ModalDebugMenu
	case g.OptionsMenuOpen:
		return ModalOptionsMenu
	case g.MenuOpen:
		return ModalPauseMenu
	}
	return ModalNone
}

func FirstLivingPartyMember(party []PartyMember) int {
	return nextWhere(party, 0, partyAlive)
}

// EnemyAlive checks whether the slot in the given slice is in-bounds and
// holds a live enemy. Works for either the active pack's Members or any
// other []Enemy the caller hands in.
func EnemyAlive(enemies []Enemy, index int) bool {
	return index >= 0 && index < len(enemies) && enemies[index].Alive
}

// ActivePack returns a write-through pointer to the engaged pack, or nil
// when no battle is in progress / ActivePack is out of range. The single
// home for the "is there a live engaged pack?" bounds check — BattleMembers,
// BattleMemberAt, AwardBattleXP, AwardBattleLoot, and the enemy-summon path
// all read through it instead of each re-rolling
// `ActivePack < 0 || >= len(g.Packs)`.
func ActivePack(g *GameState) *Pack {
	if g.Battle.ActivePack < 0 || g.Battle.ActivePack >= len(g.Packs) {
		return nil
	}
	return &g.Packs[g.Battle.ActivePack]
}

// BattleMembers returns the active pack's member slice, or nil when no
// pack is engaged. Callers that need write access through the slice (HP,
// flags, etc.) can index it directly since &Members[i] is stable for the
// lifetime of the pack.
func BattleMembers(g *GameState) []Enemy {
	if p := ActivePack(g); p != nil {
		return p.Members
	}
	return nil
}

// BattleMemberAt returns a write-through pointer to one member of the
// active pack, or nil if the slot is invalid.
func BattleMemberAt(g *GameState, slot int) *Enemy {
	p := ActivePack(g)
	if p == nil {
		return nil
	}
	if slot < 0 || slot >= len(p.Members) {
		return nil
	}
	return &p.Members[slot]
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

// CountLivingEnemies counts the alive members in an already-resolved roster
// slice — the allocation-free variant of LivingBattleCount for callers (the
// per-frame battle Update) that have hoisted BattleMembers and want to avoid
// re-deriving the pack each call.
func CountLivingEnemies(members []Enemy) int {
	return countWhere(members, enemyAlive)
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

// leaderSlot returns the index of the highest-tier element among n items,
// ties broken by lowest index. Shared by PackLeaderSlot (runtime Pack) and
// PackSpawnLeaderSlot (authored PackSpawn) so the "leader = beefiest member"
// rule lives in one place.
func leaderSlot(n int, tierAt func(int) int) int {
	bestSlot := 0
	bestTier := -1
	for i := 0; i < n; i++ {
		if t := tierAt(i); t > bestTier {
			bestTier = t
			bestSlot = i
		}
	}
	return bestSlot
}

// averageOverLiving returns the mean of valueAt(i) across the [0,n) slots
// where aliveAt(i) is true, or 0 when none are alive. Sibling to leaderSlot's
// two-closure shape — shared by PartyAverageLevel and PackAverageLevel so the
// "average over the living, 0 when all down" rule (the flee-chance math) lives
// in one place.
func averageOverLiving(n int, aliveAt func(int) bool, valueAt func(int) int) float64 {
	sum, count := 0, 0
	for i := 0; i < n; i++ {
		if aliveAt(i) {
			sum += valueAt(i)
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return float64(sum) / float64(count)
}

// PartyAverageLevel returns the mean Level of LIVING party members (HP > 0),
// or 0 when the whole party is down. The party side of the flee-chance math.
func PartyAverageLevel(party []PartyMember) float64 {
	return averageOverLiving(len(party),
		func(i int) bool { return party[i].HP > 0 },
		func(i int) int { return party[i].Level })
}

// PackAverageLevel returns the mean EnemyLevel of LIVING pack members, or 0 for
// an empty / all-dead pack. The pack side of the flee-chance math — averaging
// (not max) so a lone weak straggler is easy to flee and a fresh full pack is
// hard.
func PackAverageLevel(p Pack) float64 {
	return averageOverLiving(len(p.Members),
		func(i int) bool { return p.Members[i].Alive },
		func(i int) int { return EnemyLevel(&p.Members[i]) })
}

// FleeChance returns the [FleeFloor, FleeCap] probability of a successful flee,
// driven by the party's average living level vs the pack's: even-level ≈
// BaseFleeChance, each level of party advantage shifts it by FleePerLevelStep.
// Never guaranteed, never impossible.
func FleeChance(partyAvgLevel, packAvgLevel float64) float64 {
	return Clamp(BaseFleeChance+(partyAvgLevel-packAvgLevel)*FleePerLevelStep, FleeFloor, FleeCap)
}

// PackLeaderSlot returns the slot of the pack's leader: the highest-Tier
// member, ties broken by member order. Empty packs return 0 (callers
// should range-check before drawing).
func PackLeaderSlot(p Pack) int {
	// Read .Tier through enemyGoverningDef (pointer) rather than EnemyInfoFor,
	// which copies the whole EnemyDefinition out per member — this runs once per
	// member per pack per frame via PackLeaderKind in render.drawFieldPacks.
	return leaderSlot(len(p.Members), func(i int) int { return enemyGoverningDef(&p.Members[i]).Tier })
}

// PackLeader returns the highest-Tier member of the pack, or a zero
// Enemy when the pack is empty.
func PackLeader(p Pack) Enemy {
	if len(p.Members) == 0 {
		return Enemy{}
	}
	return p.Members[PackLeaderSlot(p)]
}

// PackLeaderKind returns just the EnemyKind of the highest-Tier member (the
// zero kind for an empty pack). The per-frame field-billboard path
// (render.drawFieldPacks) needs only the kind, so this avoids PackLeader's
// full Enemy copy (which embeds the pointer-bearing DefinitionOverride) once
// per pack per frame.
func PackLeaderKind(p Pack) EnemyKind {
	if len(p.Members) == 0 {
		return Enemy{}.Kind
	}
	return p.Members[PackLeaderSlot(p)].Kind
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
	pack := ActivePack(g)
	if pack == nil {
		return 0, nil
	}
	perMember = PackXPValue(*pack)
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
	// Pointer, not a value copy: Enemy embeds a full DefinitionOverride, and
	// this runs per queue entry (up to target*ATBQueueSlotMultiplier) every
	// battle frame via the cached forecast. The dead-entry early-return then
	// costs no copy.
	enemy := &members[actor.Index]
	if !enemy.Alive {
		return TurnEntry{}, false
	}
	return TurnEntry{Label: EnemySingularName(enemy), Enemy: true}, true
}

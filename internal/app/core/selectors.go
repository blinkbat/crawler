package core

import (
	"reflect"
	"strings"
)

type TurnEntry struct {
	Label string
	Class PartyClass
	Enemy bool
	// Index is the actor's index in its own list (party slot, or BattleMembers
	// slot for an enemy) — lets the turn-order panel match a row to the targeted actor.
	Index int
}

// Generic walking primitives. The public party/enemy walkers below resolve
// through these so a new "filter by class/status" needs one predicate + one wrapper.

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

// indicesWhereInto is indicesWhere into a caller-owned buffer (re-sliced to 0) —
// allocation-free. Returned slice aliases buf until the caller's next reuse.
func indicesWhereInto[T any](buf []int, slice []T, pred func(T) bool) []int {
	buf = buf[:0]
	for i, v := range slice {
		if pred(v) {
			buf = append(buf, i)
		}
	}
	return buf
}

// filterInto copies elements of src matching `keep` into buf (re-sliced to 0) —
// the value-preserving sibling of indicesWhereInto. Pass nil to allocate;
// returned slice aliases buf until the caller's next reuse.
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
// Negative start clamps to 0.
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

// wrapNextWhere walks forward from `start`, wrapping past the end, returning the
// first matching index, or -1 if none match.
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

// partyAlive / partyAvailable / enemyAlive: the canonical predicates the walkers
// feed through, keeping "alive AND not ingested" in one spot.
func partyAlive(m PartyMember) bool     { return m.HP > 0 }
func partyAvailable(m PartyMember) bool { return MemberAvailable(m) }
func enemyAlive(e Enemy) bool           { return e.Alive }

// MemberAvailable reports whether a member can act / be targeted — alive AND not
// ingested. The one home for "ingested counts as unavailable"; call sites must use
// this rather than re-inlining `HP <= 0 || Ingested`.
func MemberAvailable(m PartyMember) bool { return m.HP > 0 && !m.Ingested }

func LivingPartyCount(party []PartyMember) int {
	return countWhere(party, partyAlive)
}

// LivingPartyIndices returns seat indices of every member with HP > 0 — the
// out-of-battle ally-target set (Ingested doesn't apply; in-battle uses AvailablePartyTargets).
func LivingPartyIndices(party []PartyMember) []int {
	return indicesWhere(party, partyAlive)
}

// LivingPartyIndicesInto is LivingPartyIndices into a caller-owned buffer (per-frame picker paths).
func LivingPartyIndicesInto(buf []int, party []PartyMember) []int {
	return indicesWhereInto(buf, party, partyAlive)
}

func PartyMemberAlive(party []PartyMember, index int) bool {
	return PartyIndexInRange(party, index) && partyAlive(party[index])
}

// FirstDownedPartyMember returns the seat index of the first downed (HP <= 0)
// member, or -1 if none are down — the Cleric's Resurrect auto-target.
func FirstDownedPartyMember(party []PartyMember) int {
	for i := range party {
		if party[i].HP <= 0 {
			return i
		}
	}
	return -1
}

// PartyMemberAvailable reports whether the member can act / be targeted this turn
// — alive AND not ingested. Ingested members are alive but out of the fight.
func PartyMemberAvailable(party []PartyMember, index int) bool {
	if !PartyMemberAlive(party, index) {
		return false
	}
	return !party[index].Ingested
}

// ActivePartyCount counts members who can still act (alive AND not ingested).
// Drives the loss check: zero ends the encounter (all-ingested = wipe).
func ActivePartyCount(party []PartyMember) int {
	return countWhere(party, partyAvailable)
}

// WrapNextAvailablePartyMember walks forward from `start` (wrapping) to the first
// available (alive + not ingested) slot, or -1. Used by the enemy attack cursor.
func WrapNextAvailablePartyMember(party []PartyMember, start int) int {
	return wrapNextWhere(party, start, partyAvailable)
}

// PeekNextEnemyTarget returns the next party-member index the enemy will attack
// (first available after EnemyAttackCursor) WITHOUT advancing the cursor, or -1.
// Shared so battle (commits) and render (previews) don't drift.
func PeekNextEnemyTarget(g *GameState) int {
	// Skip a Vanished member (untargetable) so a back-row caster can't strike it.
	return wrapNextWhere(g.Party, g.Battle.EnemyAttackCursor+1, func(m PartyMember) bool {
		return partyAvailable(m) && m.VanishTurns == 0
	})
}

// AvailablePartyTargets returns indices of members choosable as a heal/item target
// (living AND not ingested) — so a heal can't be wasted on ingested prey.
func AvailablePartyTargets(party []PartyMember) []int {
	return indicesWhere(party, partyAvailable)
}

// FirstAvailablePartyMember is the alive-and-not-ingested companion to
// FirstLivingPartyMember — the fallback when an enemy spell's preferred target is out of reach.
func FirstAvailablePartyMember(party []PartyMember) int {
	return nextWhere(party, 0, partyAvailable)
}

// MantrapHasPrey reports whether any member is being digested by the active-pack
// member at `slot`. Gates SkillIngest — a plant with prey can't snatch another.
func MantrapHasPrey(party []PartyMember, slot int) bool {
	for _, m := range party {
		if m.Ingested && m.IngestedBy == slot {
			return true
		}
	}
	return false
}

// ReleaseIngestedBy frees every member held by the active-pack slot (killing blow /
// battle exit) and returns their indices for logging.
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

// ReleaseAllIngested frees every ingested member regardless of swallower (battle
// exit), so a desynced encounter doesn't leak the lockout.
func ReleaseAllIngested(party []PartyMember) {
	for i := range party {
		if party[i].Ingested {
			party[i].Ingested = false
			party[i].IngestedBy = -1
		}
	}
}

// partyTurnsClass buckets every `*Turns int` field on PartyMember so a newly-added
// counter can't silently bypass the cure/clear plumbing.
type partyTurnsClass int

const (
	turnsCurableTransient partyTurnsClass = iota // cleared by CureDebuffs AND ClearPartyTransientStatuses; also drives transientStatusCounters
	turnsLingeringCurable                        // cured by CureDebuffs but survives a fight (Poison only)
	turnsBenign                                  // beneficial/neutral, never cured (Regen, Ice Armor)
)

// partyTurnsCounterClass classifies each PartyMember timed-status counter. The init
// reflection assert below trips at startup on an unclassified `*Turns` field (mirrors
// Enemy's clearedEnemyTurnsCounters guard) and pins transientStatusCounters to the
// turnsCurableTransient set so the two can't drift.
var partyTurnsCounterClass = map[string]partyTurnsClass{
	"SleepTurns":    turnsCurableTransient,
	"StunTurns":     turnsCurableTransient,
	"WebbedTurns":   turnsCurableTransient,
	"ConfusedTurns": turnsCurableTransient,
	"PoisonTurns":   turnsLingeringCurable,
	"RegenTurns":    turnsBenign,
	"IceArmorTurns": turnsBenign,
	"VanishTurns":   turnsBenign, // beneficial (untargetable); combat-only, cleared on exit
	"SpiritTurns":   turnsBenign, // Ancestral Spirit shade; combat-only, cleared on exit
}

// transientStatusCounters returns pointers to the shared curable counters
// (Sleep/Stun/Webbed/Confused). Both CureDebuffs and ClearPartyTransientStatuses
// route through this so the set can't drift. NOT Poison (it lingers past a fight).
func transientStatusCounters(m *PartyMember) []*int {
	return []*int{&m.SleepTurns, &m.StunTurns, &m.WebbedTurns, &m.ConfusedTurns}
}

func init() {
	// Every PartyMember `*Turns int` field must be classified, forcing a deliberate
	// cure/clear decision on any new counter instead of a silent lingering bug.
	curable := 0
	t := reflect.TypeOf(PartyMember{})
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Type.Kind() != reflect.Int || !strings.HasSuffix(f.Name, "Turns") {
			continue
		}
		class, ok := partyTurnsCounterClass[f.Name]
		if !ok {
			panic("core: PartyMember." + f.Name + " is an unclassified timed-status counter — add it to partyTurnsCounterClass (and transientStatusCounters if curable) so CureDebuffs/ClearPartyTransientStatuses can't skip it")
		}
		if class == turnsCurableTransient {
			curable++
		}
	}
	// Pin the pointer list to the map's curable-transient count so the two stay in sync.
	if got := len(transientStatusCounters(&PartyMember{})); got != curable {
		panic("core: transientStatusCounters must list exactly the turnsCurableTransient entries in partyTurnsCounterClass")
	}
}

// SkillCuresDebuffs reports whether skill's benefit is a status cure (rather than
// an HP heal). The single home for "is this a cure skill" — a cure's effect isn't a
// SkillEffect field, so it can't be inferred from def.Effect; both the out-of-battle
// benefit gate and the explore apply path key off this instead of `== SkillCleanse`.
func SkillCuresDebuffs(skill SkillID) bool {
	return skill == SkillCleanse
}

// CureDebuffs clears the curable NEGATIVE statuses (Poison + the four shared
// counters) off a member and returns how many were active. Leaves Defending and
// Bless intact; does NOT touch Ingested. Caller is the Cleric's Cleanse.
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

// HasCurableDebuff reports whether m carries any debuff CureDebuffs would clear, so
// the Cleanse paths can refuse a cure-nothing cast before spending MP.
func HasCurableDebuff(m *PartyMember) bool {
	if m == nil {
		return false
	}
	if m.PoisonTurns > 0 {
		return true
	}
	for _, c := range transientStatusCounters(m) {
		if *c > 0 {
			return true
		}
	}
	return false
}

// ClearPartyTransientStatuses wipes combat-only statuses off every member at
// battle exit EXCEPT Poison (a lingering wound). Never touches HP. Only dead and
// poisoned survive a fight. (Ingest is released separately via ReleaseAllIngested.)
func ClearPartyTransientStatuses(party []PartyMember) {
	for i := range party {
		ClearMemberTransientStatuses(&party[i])
	}
}

// ClearMemberTransientStatuses is the per-member body of ClearPartyTransientStatuses;
// also used when a member dies outside battle. Never touches HP or Poison.
func ClearMemberTransientStatuses(m *PartyMember) {
	for _, c := range transientStatusCounters(m) {
		*c = 0
	}
	m.Defending = false
	m.Guarding = false
	m.Guarded = false
	m.GuardedBy = 0
	m.LastStandUsed = false // re-arms next battle
	m.Buffs = nil
	m.ShieldHP = 0
	m.IceArmorTurns = 0
	m.RegenTurns = 0
	m.RegenPerTurn = 0
	m.VanishTurns = 0
	m.SpiritTurns = 0
}

// clearPartyCombatTransients strips state that must never outlive a battle:
// transient statuses, ingestion links, anim timers. Shared by the save sanitizer,
// load scrub, and field recovery. Preserves Poison (field recovery zeroes it itself).
func clearPartyCombatTransients(party []PartyMember) {
	ClearPartyTransientStatuses(party) // Sleep / Stun / Webbed / Confused / Defending
	ReleaseAllIngested(party)          // Ingested / IngestedBy
	for i := range party {
		clearMemberAnimTimers(&party[i])
	}
}

// ModalKind tags the active full-screen explore-scene modal. These are plain tags,
// NOT a priority ranking — the iota order does not encode precedence (e.g. ModalShop
// sorts below ModalChest here but resolves ABOVE it at runtime). Precedence lives
// solely in ActiveModal's switch order below; only the single resolved value is ever
// consumed, never compared. Add a kind anywhere and wire it into that switch.
type ModalKind int

const (
	ModalNone ModalKind = iota
	ModalPauseMenu
	ModalOptionsMenu
	ModalSoundMenu
	ModalDebugMenu
	ModalRetroMenu
	ModalCombatTune
	ModalWipeMenu
	ModalShop
	ModalDoorPrompt
	ModalChest
	ModalPanels
	ModalLevelUp
	// ModalDialog is the branching-conversation overlay; blocks explore until it ends.
	ModalDialog
	// ModalQuitConfirm: the quit prompt. Highest priority so nothing shadows a pending quit.
	ModalQuitConfirm

	// ModalCount bounds the modal-updater dispatch table in explore (init-asserted
	// complete). Runtime-only enum (never serialized), so appending here is safe.
	ModalCount
)

// ActiveModal returns the highest-priority open explore-scene modal, or ModalNone
// if movement/battle should process input. Battle phase is NOT a modal.
func ActiveModal(g *GameState) ModalKind {
	if g == nil {
		return ModalNone
	}
	// ChestOpen / DoorPrompt are index-or-(-1) sentinels, bounded against their
	// slice (not just >= 0) so a struct-literal zero-value index can't render a phantom modal.
	switch {
	case g.QuitConfirmOpen:
		return ModalQuitConfirm
	case g.DialogOpen:
		return ModalDialog
	case g.LevelUpOpen:
		return ModalLevelUp
	case g.PanelsOpen:
		return ModalPanels
	case g.ShopOpen:
		// High in the ladder so a stray lower flag can't shadow the shop's input.
		return ModalShop
	case g.ChestOpen >= 0 && g.ChestOpen < len(g.Chests):
		return ModalChest
	case g.DoorPrompt >= 0 && g.DoorPrompt < len(g.Doors):
		return ModalDoorPrompt
	case g.RetroMenuOpen:
		// Above the debug menu (its child) so a stray double-open can't let the
		// parent shadow the child's input.
		return ModalRetroMenu
	case g.CombatTuneOpen:
		// Child of the debug menu, same priority rationale as the retro submenu.
		return ModalCombatTune
	case g.WipeMenuOpen:
		return ModalWipeMenu
	case g.DebugMenuOpen:
		return ModalDebugMenu
	case g.SoundMenuOpen:
		// Child of the Options menu — above it so a stray double-open can't let the
		// parent shadow the child's input.
		return ModalSoundMenu
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

// EnemyAlive reports whether the slot is in-bounds and holds a live enemy.
func EnemyAlive(enemies []Enemy, index int) bool {
	return index >= 0 && index < len(enemies) && enemies[index].Alive
}

// ActivePack returns a write-through pointer to the engaged pack, or nil when no
// battle / out of range. The single home for the "is there a live engaged pack?" bounds check.
func ActivePack(g *GameState) *Pack {
	if g.Battle.ActivePack < 0 || g.Battle.ActivePack >= len(g.Packs) {
		return nil
	}
	return &g.Packs[g.Battle.ActivePack]
}

// BattleMembers returns the active pack's member slice (or nil). &Members[i] is
// stable for the pack's lifetime, so callers can index for write access.
func BattleMembers(g *GameState) []Enemy {
	if p := ActivePack(g); p != nil {
		return p.Members
	}
	return nil
}

// BattleMemberAt returns a write-through pointer to one active-pack member, or nil if invalid.
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

// LivingBattleEnemyIndices returns alive active-pack slots — the target set for
// RANGED/MAGIC attacks (any row).
func LivingBattleEnemyIndices(g *GameState) []int {
	return indicesWhere(BattleMembers(g), enemyAlive)
}

// MeleeReachableBattleEnemyIndices returns alive enemy slots a MELEE attack can hit
// — the effective front row only (back row protected until the front falls), Flying
// foes excluded (immune to melee).
func MeleeReachableBattleEnemyIndices(g *GameState) []int {
	members := BattleMembers(g)
	out := make([]int, 0, len(members))
	for i := range members {
		if members[i].Alive && EnemyMeleeReachable(members, i) {
			out = append(out, i)
		}
	}
	return out
}

// NoMeleeReachableEnemy reports whether a MELEE attack reaches NO foe because every
// living enemy is out of melee range (all Flying / wholly covered). The foe-side twin
// of BackRowMeleeBlocked's attacker-side gate: both grey the Attack/melee-skill rows,
// this one firing when the attacker stands up front yet has nothing to swing at.
// Allocation-free (unlike len(MeleeReachableBattleEnemyIndices)==0) for the render path.
func NoMeleeReachableEnemy(g *GameState) bool {
	members := BattleMembers(g)
	for i := range members {
		if members[i].Alive && EnemyMeleeReachable(members, i) {
			return false
		}
	}
	return true
}

// MeleeActionBlocked reports whether a melee action of class ac from party slot i can
// connect with nothing — i stuck in the protected back row, OR no foe is melee-reachable
// (all Flying). The single grey-out gate for the Attack row and melee skill rows; non-
// melee never blocks. The refusal path (battle.enterEnemyTargeting / updateSkillMenu)
// re-tests the two reasons apart to log the right line.
func MeleeActionBlocked(g *GameState, ac AttackClass, i int) bool {
	return BackRowMeleeBlocked(ac, g.Party, i) || (ac.IsMelee() && NoMeleeReachableEnemy(g))
}

// AoEReachesEnemy reports whether an all-enemy skill's sweep actually lands on the
// enemy at `slot`: a melee AoE (Swipe/Whirlwind) is front-gated and skips Flying
// foes; a ranged/magic AoE (Fireball/Arc Bolt) sweeps the whole pack. Shared by the
// battle apply (forEachTargetableEnemy) and the render AoE chevron preview so the
// markers match who's actually hit.
func AoEReachesEnemy(members []Enemy, skill SkillID, slot int) bool {
	if slot < 0 || slot >= len(members) || !members[slot].Alive {
		return false
	}
	if SkillAttackClassFor(skill).IsMelee() {
		return EnemyMeleeReachable(members, slot)
	}
	return true
}

// BattlePendingAttackIsMelee reports whether the targeted action is MELEE (basic
// attack off the equipped weapon, skill off its reach class). Shared body for the
// renderer (which can't import battle) and battle.battlePendingAttackMelee, which
// delegates here.
func BattlePendingAttackIsMelee(g *GameState) bool {
	if g.Battle.PendingSkill == SkillNone {
		i := g.Battle.CurrentParty
		if i < 0 || i >= len(g.Party) {
			return false
		}
		return BasicAttackClass(EquippedWeapon(g.Party[i])).IsMelee()
	}
	return SkillAttackClassFor(g.Battle.PendingSkill).IsMelee()
}

// BattleEnemyTargetReachable reports whether the slot can be hit by the targeted
// action: always for ranged/magic, for melee only when in the effective front row
// and not Flying (flyers are melee-immune). Shared by the cycler's confirm gate and
// the roster gray-out.
func BattleEnemyTargetReachable(g *GameState, slot int) bool {
	members := BattleMembers(g)
	if slot < 0 || slot >= len(members) {
		return false
	}
	if !BattlePendingAttackIsMelee(g) {
		return true
	}
	return members[slot].Alive && EnemyMeleeReachable(members, slot)
}

// PeekNextMeleeEnemyTarget is PeekNextEnemyTarget for a MELEE enemy attack: the
// first available member after the cursor in the effective front row, or -1.
// Does NOT advance the cursor.
func PeekNextMeleeEnemyTarget(g *GameState) int {
	// PartyInEffectiveFront's front check reduces to a per-member Row test once
	// PartyFrontHasLiving is hoisted (constant across the scan), letting the
	// value-only wrapNextWhere predicate carry it.
	frontHasLiving := PartyFrontHasLiving(g.Party)
	return wrapNextWhere(g.Party, g.Battle.EnemyAttackCursor+1, func(m PartyMember) bool {
		return partyAvailable(m) && m.VanishTurns == 0 && (m.Row == RowFront || !frontHasLiving)
	})
}

// tauntedAttackerTarget returns the slot a live Taunt forces the current enemy
// attacker (g.Battle.EnemyAttacker) onto — any row — or ok=false. A defeated or
// ingested taunter releases the lock.
func tauntedAttackerTarget(g *GameState) (int, bool) {
	enemy := BattleMemberAt(g, g.Battle.EnemyAttacker)
	if enemy == nil || enemy.TauntTurns <= 0 {
		return -1, false
	}
	t := enemy.TauntedBy
	if t < 0 || t >= len(g.Party) {
		return -1, false
	}
	if !MemberAvailable(g.Party[t]) || g.Party[t].VanishTurns > 0 {
		return -1, false
	}
	return t, true
}

// PeekEnemyAttackerTarget returns the party slot the current enemy attacker will
// hit, WITHOUT advancing the round-robin cursor. Precedence mirrors the commit
// path: a live Taunt overrides any row; else a basic attack (EnemyPendingSkill ==
// SkillNone) is melee and front-gated, while a pending skill casts at any row.
// Shared by the battle commit (pickEnemyAttackTarget) and the render forecast so
// the incoming-hit marker can't drift from who's actually struck. -1 = no target.
func PeekEnemyAttackerTarget(g *GameState) int {
	if forced, ok := tauntedAttackerTarget(g); ok {
		return forced
	}
	var target int
	if g.Battle.EnemyPendingSkill == SkillNone {
		target = PeekNextMeleeEnemyTarget(g)
	} else {
		target = PeekNextEnemyTarget(g)
	}
	// Guard redirect rides here (like Taunt above) so the render forecast and the
	// commit path agree on who's actually struck.
	return redirectToGuardian(g, target)
}

// redirectToGuardian returns the slot a hit aimed at `target` actually lands on:
// the guardian covering target (Warrior's Guard) instead of target, or target
// unchanged. A MELEE attacker only intercepts onto a front-row-reachable guardian
// (mirrors the front gate — a back-row guardian can't soak a melee swing it
// couldn't otherwise take); a casting attacker reaches any row. Lapses when the
// guardian is down/ingested or is the target itself.
func redirectToGuardian(g *GameState, target int) int {
	if target < 0 || target >= len(g.Party) {
		return target
	}
	ward := g.Party[target]
	if !ward.Guarded {
		return target
	}
	guardian := ward.GuardedBy
	if guardian < 0 || guardian >= len(g.Party) || guardian == target {
		return target
	}
	if !MemberAvailable(g.Party[guardian]) {
		return target
	}
	if g.Battle.EnemyPendingSkill == SkillNone &&
		!(g.Party[guardian].Row == RowFront || !PartyFrontHasLiving(g.Party)) {
		return target
	}
	return guardian
}

// SetGuard makes `guardian` cover `ward`: ward's incoming hits redirect to the
// guardian (redirectToGuardian) until the guardian's next turn clears it
// (ClearGuardBy in beginPartyTurn). A guardian covers ONE ward — re-guarding drops
// the prior cover. Self-guard is inert (clears, sets nothing). Bounds-checked.
func SetGuard(party []PartyMember, guardian, ward int) {
	if guardian < 0 || guardian >= len(party) || ward < 0 || ward >= len(party) {
		return
	}
	ClearGuardBy(party, guardian) // drop any prior ward this guardian covered
	if guardian == ward {
		return // nothing to cover
	}
	party[guardian].Guarding = true
	party[ward].Guarded = true
	party[ward].GuardedBy = guardian
}

// ClearGuardBy ends `guardian`'s cover: clears its Guarding flag and drops the
// Guarded link on any ward it was protecting. Called when the guardian acts again.
func ClearGuardBy(party []PartyMember, guardian int) {
	if guardian >= 0 && guardian < len(party) {
		party[guardian].Guarding = false
	}
	for i := range party {
		if party[i].Guarded && party[i].GuardedBy == guardian {
			party[i].Guarded = false
			party[i].GuardedBy = 0
		}
	}
}

func LivingBattleCount(g *GameState) int {
	return countWhere(BattleMembers(g), enemyAlive)
}

// CountLivingEnemies counts alive members in a resolved roster slice — the
// alloc-free LivingBattleCount for callers that have hoisted BattleMembers.
func CountLivingEnemies(members []Enemy) int {
	return countWhere(members, enemyAlive)
}

func NextLivingBattleEnemy(g *GameState) int {
	return nextWhere(BattleMembers(g), 0, enemyAlive)
}

// SelectedEnemySlot returns the active-pack slot the enemy cursor points at, or -1.
// Render markers should query this rather than reading g.Battle.EnemyIndex directly.
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

// HighlightedAllyIndex returns the party index the ally cursor points at
// (heal/item modes), or -1. The friendly counterpart to SelectedEnemySlot.
func HighlightedAllyIndex(g *GameState) int {
	if g.Battle.PartyTarget < 0 || g.Battle.PartyTarget >= len(g.Party) {
		return -1
	}
	return g.Battle.PartyTarget
}

// ActiveActorIndex returns the party index whose turn it is, or -1 (enemy turn,
// splash, post-battle). For the "your turn" highlight.
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

// leaderSlot returns the index of the highest-tier element (ties → lowest index).
// Shared by PackLeaderSlot and PackSpawnLeaderSlot ("leader = beefiest member").
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

// averageOverLiving returns the mean of valueAt(i) over the slots where aliveAt(i),
// or 0 when none are alive. Shared by PartyAverageLevel / PackAverageLevel (flee math).
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

// PartyAverageLevel returns the mean Level of living members, or 0 when all down.
func PartyAverageLevel(party []PartyMember) float64 {
	return averageOverLiving(len(party),
		func(i int) bool { return party[i].HP > 0 },
		func(i int) int { return party[i].Level })
}

// PackAverageLevel returns the mean EnemyLevel of living pack members, or 0 for an
// empty/dead pack. Averaging (not max) so a lone straggler is easy to flee.
func PackAverageLevel(p Pack) float64 {
	return averageOverLiving(len(p.Members),
		func(i int) bool { return p.Members[i].Alive },
		func(i int) int { return EnemyLevel(&p.Members[i]) })
}

// FleeChance returns the [FleeFloor, FleeCap] flee probability from party vs pack
// average level (even ≈ BaseFleeChance, each level shifts by FleePerLevelStep).
func FleeChance(partyAvgLevel, packAvgLevel float64) float64 {
	return Clamp(BaseFleeChance+(partyAvgLevel-packAvgLevel)*FleePerLevelStep, FleeFloor, FleeCap)
}

// PackLeaderSlot returns the highest-Tier member's slot (ties → member order).
// Empty packs return 0 (callers should range-check).
func PackLeaderSlot(p Pack) int {
	// .Tier via enemyGoverningDef (pointer), not EnemyInfoFor (copies the whole
	// definition) — this runs per member per pack per frame.
	return leaderSlot(len(p.Members), func(i int) int { return enemyGoverningDef(&p.Members[i]).Tier })
}

// PackLeader returns the highest-Tier member, or a zero Enemy when empty.
func PackLeader(p Pack) Enemy {
	if len(p.Members) == 0 {
		return Enemy{}
	}
	return p.Members[PackLeaderSlot(p)]
}

// PackLeaderKind returns just the highest-Tier member's EnemyKind (zero kind when
// empty), avoiding PackLeader's full Enemy copy on the per-frame billboard path.
func PackLeaderKind(p Pack) EnemyKind {
	if len(p.Members) == 0 {
		return Enemy{}.Kind
	}
	return p.Members[PackLeaderSlot(p)].Kind
}

// PackXPValue sums XPValue across the pack — the per-character (not split) award
// every living member earns when the pack falls.
func PackXPValue(p Pack) int {
	total := 0
	for _, m := range p.Members {
		total += EnemyInfoFor(m).XPValue
	}
	return total
}

// AwardBattleXP grants the active pack's PackXPValue to every living member and
// processes level-ups. Returns the per-member amount and the indices that leveled.
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

// TurnForecast walks the scheduled queue from the cursor, emitting up to `limit`
// per-actor preview entries, looping into the next round if short. Thin wrapper
// over TurnForecastInto for callers without a reusable buffer (tests, ad-hoc).
func TurnForecast(g *GameState, limit int) []TurnEntry {
	return TurnForecastInto(g, nil, limit)
}

// TurnForecastInto is TurnForecast into a reusable buffer (truncated to 0 when it
// has cap; returned slice shares it) — for per-frame render callers.
func TurnForecastInto(g *GameState, into []TurnEntry, limit int) []TurnEntry {
	if cap(into) < limit {
		into = make([]TurnEntry, 0, limit)
	} else {
		into = into[:0]
	}
	active := g.Battle.Phase.InCombat()
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
	// Fall through to the cached next-round projection; since-died actors are
	// skipped by turnEntryFor.
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

// turnEntryFor materializes one queue actor into a TurnEntry, skipping dead actors (false).
func turnEntryFor(g *GameState, actor ActorRef) (TurnEntry, bool) {
	if actor.IsParty {
		if !actor.ValidPartyIndex(g.Party) || g.Party[actor.Index].HP <= 0 {
			return TurnEntry{}, false
		}
		p := g.Party[actor.Index]
		return TurnEntry{Label: p.Name, Class: p.Class, Index: actor.Index}, true
	}
	members := BattleMembers(g)
	if actor.Index < 0 || actor.Index >= len(members) {
		return TurnEntry{}, false
	}
	// Pointer, not a copy: Enemy embeds a full DefinitionOverride and this runs
	// per queue entry every battle frame; the dead-entry early-return costs no copy.
	enemy := &members[actor.Index]
	if !enemy.Alive {
		return TurnEntry{}, false
	}
	return TurnEntry{Label: EnemySingularName(enemy), Enemy: true, Index: actor.Index}, true
}

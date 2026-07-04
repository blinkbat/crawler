package battle

import (
	"fmt"

	"crawler/internal/app/audio"
	"crawler/internal/app/core"
	"crawler/internal/app/input"
)

// Shared battle-prompt status strings — named so the wording lives in one place.
const (
	msgChooseAction   = "Choose an action."
	msgChooseTarget   = "Choose a target."
	msgChooseItem     = "Choose an item."
	msgChooseSkill    = "Choose a skill."
	msgNoTarget       = "No target."
	msgNoSkillReady   = "No skill ready."
	msgNoItems        = "No items."
	msgInvalidTarget  = "Invalid target."
	msgNoAllySelected = "No ally selected."
	msgNothingToSteal = "There is nothing to steal."
	// Greyed melee ACTION refused (Attack row or melee skill picked anyway): one pattern
	// via refuseMeleeAction — buzz + logged named-unit line, reason picked here. Back row =
	// actor stuck behind the live front; flying = actor up front but every foe airborne.
	msgMeleeBackRowFmt = "%s can't reach from the back row!"
	msgMeleeFlyingFmt  = "%s can't reach a flying foe!"
	// Greyed FOE target confirmed anyway (mixed pack — cursor on an unreachable cell):
	// same buzz + log, foe-side reason — a melee-immune flyer vs a foe covered by the front.
	msgFlyingMeleeTarget  = "Can't reach a flying foe with the current weapon."
	msgBackRowMeleeTarget = "Can't reach a foe covered behind the front rank."
)

// Start begins a battle against the pack at packIndex (the whole roster becomes
// the enemy list). fleeReturnX/Z is the pre-step tile the player retreats to on a
// successful Flee (so a pack ambush steps them back, not onto the pack's tile).
// packIndexInRange reports whether idx names a slot in g.Packs — the shared
// bounds rule for the pack-index domain (mirrors partyIndexValid for the party).
func packIndexInRange(g *core.GameState, idx int) bool {
	return idx >= 0 && idx < len(g.Packs)
}

// dropPackAt removes pack idx from g.Packs when in range (no-op otherwise). The
// single pack-removal seam shared by fleeBattle and clearBattleResidual.
func dropPackAt(g *core.GameState, idx int) {
	if packIndexInRange(g, idx) {
		g.Packs = append(g.Packs[:idx], g.Packs[idx+1:]...)
	}
}

// validLivePack reports whether packIndex names an in-range, still-alive pack —
// the entry guard shared by Start and DebugSkipWin.
func validLivePack(g *core.GameState, packIndex int) bool {
	return packIndexInRange(g, packIndex) && core.PackAlive(g.Packs[packIndex])
}

func Start(g *core.GameState, packIndex, fleeReturnX, fleeReturnZ int, engageSide core.EngageSide) {
	if !validLivePack(g, packIndex) {
		return
	}
	g.Battle.ActivePack = packIndex
	g.Battle.FleeReturnX = fleeReturnX
	g.Battle.FleeReturnZ = fleeReturnZ
	// Pack foes into the front row left-to-right at spawn (not only after a death),
	// so an authored back-heavy pack reads correctly from turn one.
	core.ShuntEnemyFormation(core.BattleMembers(g))
	// Seat the party's LIVE formation from its home slots: rotate by the engage side
	// (ambush), then pack the living forward so an already-downed frontliner doesn't
	// hold the line. Home slots are untouched, so the party reverts to its preferred
	// formation after the fight.
	core.SetBattleStartFormation(g.Party, engageSide)
	g.Battle.EnemyIndex = core.NextLivingBattleEnemy(g)
	g.Battle.PartyTarget = core.FirstLivingPartyMember(g.Party)
	g.Battle.Splash = core.BattleSplashDuration
	g.Battle.EngageSide = engageSide
	// ActionLog is a continuous in/out-of-combat buffer, so a fight no longer resets it.
	// resetBattleTransients clears every per-fight transient so a Start after an
	// unclean exit can't leak stale combat state. EnemyIndex / PartyTarget were set
	// above and are intentionally left untouched.
	resetBattleTransients(&g.Battle)
	resetBattleAction(g)
	setBattleMessage(g, core.BattleEncounterMessage(g))
	beginNewRound(g)
	// Short anticipatory buzz as the encounter slams in (after resetBattleTransients,
	// which zeroes the rumble timer).
	core.TriggerRumble(&g.Battle, core.RumbleBattleStart, core.RumbleBattleStartDur)
}

func Update(g *core.GameState, dt float32) {
	// dt is pre-clamped by explore.Update so one hitched frame can't fast-forward
	// the bar past its window; tickFlashHold relies on this (worst case it pushes
	// onResolve back one frame, never drops/duplicates it).
	//
	// Hit-stop freeze: while HitStop > 0 the world is paused, but DamageFlash holds
	// at peak (the freeze IS the impact). The phase handler still runs so HitStop
	// counts down; at zero the apply step fires and normal updates resume.
	if g.Battle.HitStop > 0 {
		switch g.Battle.Phase {
		case core.BattleAttackTiming:
			updateAttackTiming(g, dt)
		case core.BattleEnemyTiming:
			updateEnemyTiming(g, dt)
		default:
			// HitStop is only set during a timing phase; drain it for any other
			// phase rather than freeze the battle forever.
			g.Battle.HitStop = 0
		}
		return
	}
	// Resolve the engaged pack + roster ONCE per combat frame (the guards below all
	// re-derive it otherwise). updateBattleEffects only decays timers — never
	// changes roster length / Alive — so the hoisted slice stays valid past it.
	pack := core.ActivePack(g)
	var members []core.Enemy
	if pack != nil {
		members = pack.Members
	}
	updateBattleEffects(g, dt, members)
	tickQualityPopup(g, dt)
	// Wipeout-win check MUST precede the EnemyIndex-desync leave below: an
	// all-enemies-dead pack is a WIN regardless of cursor position, and routing it
	// to leaveBattle would forfeit XP/gold/loot. winBattle sets Phase=BattleWon so
	// this can't re-enter. Gated on an active combat phase, and on a real pack —
	// BattleMembers reads stale/-1 ActivePack as empty, which must NOT look like a wipe.
	inCombatPhase := g.Battle.Phase.InCombat()
	// Party wipe before enemy wipe (mirrors finishActorTurn): if both sides are
	// down the battle is a loss, never a zero-survivor victory.
	if g.Battle.Phase != core.BattleWon && g.Battle.Phase != core.BattleLost && core.ActivePartyCount(g.Party) == 0 {
		loseBattle(g, "The party is driven back. Press to recover.")
		return
	}
	if inCombatPhase && checkEnemyWipeoutFor(g, pack, members) {
		return
	}
	// Desynced EnemyIndex (e.g. a culled enemy): leave so residual queue/timing
	// state doesn't linger. Empty message so the quiet-area message isn't clobbered.
	// Gated on an active combat phase: a won/lost battle legitimately carries a stale
	// index (the killing blow's target is gone), and force-leaving there would skip
	// the spoils screen + foe-killed triggers.
	if inCombatPhase && (g.Battle.EnemyIndex < 0 || g.Battle.EnemyIndex >= len(members)) {
		leaveBattle(g, "")
		return
	}
	if g.Battle.Splash > 0 {
		g.Battle.Splash = core.ApproachZero(g.Battle.Splash, dt)
	}

	switch g.Battle.Phase {
	case core.BattlePlayer:
		updatePlayerBattle(g)
	case core.BattleAttackTiming:
		updateAttackTiming(g, dt)
	case core.BattleEnemyTiming:
		updateEnemyTiming(g, dt)
	case core.BattleWon:
		updateVictorySpoils(g, dt)
	case core.BattleLost:
		if input.ConfirmPressed() {
			recoverFromLoss(g)
		}
	}
}

// --- Mixed-initiative scheduler --------------------------------------------

// checkEnemyWipeout fires the standard "last enemy down" win if no enemies remain.
// Returns true when it transitioned the battle (caller should return). Flavor
// kills (e.g. burn) still call winBattle directly; this is the canonical case.
func checkEnemyWipeout(g *core.GameState) bool {
	return checkEnemyWipeoutFor(g, core.ActivePack(g), core.BattleMembers(g))
}

// checkEnemyWipeoutFor is checkEnemyWipeout against an already-resolved pack +
// roster (so Update's hoisted slice is reused). A nil pack never wins — empty
// members must NOT look like a wipe and re-award spoils against a gone pack.
func checkEnemyWipeoutFor(g *core.GameState, pack *core.Pack, members []core.Enemy) bool {
	return winIfEnemiesWiped(g, pack, members, core.LastBattleEnemyFallsMessage())
}

// winIfEnemiesWiped wins the battle with `message` iff a real pack has no living
// member left. Shared by the standard last-enemy-down path and flavor kills (e.g.
// burn) that supply their own message. A nil pack never wins — empty members must
// NOT look like a wipe and re-award spoils against a gone pack.
func winIfEnemiesWiped(g *core.GameState, pack *core.Pack, members []core.Enemy, message string) bool {
	if pack == nil || core.CountLivingEnemies(members) != 0 {
		return false
	}
	winBattle(g, message)
	return true
}

// checkPartyWipeout fires the loss path when no party member is available
// (alive AND not ingested), so a fully-swallowed party counts as a wipe. Returns
// true when it transitioned the battle (caller should return).
func checkPartyWipeout(g *core.GameState) bool {
	if core.ActivePartyCount(g.Party) == 0 {
		loseBattle(g, core.BattleLossMessage(g))
		return true
	}
	return false
}

// beginNewRound rebuilds the turn queue and starts the first actor. Burn ticks
// are NOT applied here — they fire per-actor at turn start (see startActorTurn).
func beginNewRound(g *core.GameState) {
	if checkEnemyWipeout(g) {
		return
	}
	// Ingested members are skipped from the queue, so their Poison ticks here at
	// the round boundary. Before the loss gate so a poison kill is honored.
	tickPoisonForIngestedParty(g)
	if checkPartyWipeout(g) {
		return
	}
	// Reset the enemy-attack round-robin cursor (PartyTarget is the player's
	// ally selection and stays untouched).
	g.Battle.EnemyAttackCursor = core.NoIndex
	g.Battle.Queue = buildTurnQueue(g)
	g.Battle.QueueCursor = 0
	// Pre-bake the next round's projection for TurnForecast. Non-persisting
	// variant so it doesn't consume the readiness the real next round needs.
	g.Battle.NextRoundQueue = projectNextRoundQueue(g)
	startActorTurn(g)
}

// buildTurnQueue assembles the next batch of actor turns via an ATB-style tick
// scheduler: each alive actor's readiness gauge accumulates by SPD per tick;
// whoever crosses ATBReadyThreshold acts, carrying the overflow forward, so the
// queue interleaves by who hits the gate (no rigid one-slot-per-actor round).
//
// Gauges SEED from g.Battle.Readiness (carried across rounds, not reset), so a
// faster actor's surplus accrues into EXTRA turns — SPD drives turn rate, not
// just order. The sim still stops once every alive actor has acted once (the
// "round" boundary downstream housekeeping hangs off). Ingested party members are
// skipped; they re-enter the queue the call AFTER their swallower dies.
func buildTurnQueue(g *core.GameState) []core.ActorRef {
	return simulateTurnQueue(g, true)
}

// projectNextRoundQueue previews the next round WITHOUT advancing persisted
// readiness, so the turn-forecast HUD stays accurate under the carry-over model.
func projectNextRoundQueue(g *core.GameState) []core.ActorRef {
	return simulateTurnQueue(g, false)
}

func simulateTurnQueue(g *core.GameState, persist bool) []core.ActorRef {
	members := core.BattleMembers(g)
	type tickActor struct {
		ref   core.ActorRef
		spd   int
		ready int
		acted bool
	}
	actors := make([]tickActor, 0, len(g.Party)+len(members))
	for i, p := range g.Party {
		if !core.MemberAvailable(p) {
			continue
		}
		ref := core.ActorRef{IsParty: true, Index: i}
		actors = append(actors, tickActor{ref: ref, spd: actorSpeed(g, ref), ready: g.Battle.Readiness[ref]})
	}
	for slot, m := range members {
		if !m.Alive {
			continue
		}
		ref := core.ActorRef{IsParty: false, Index: slot}
		actors = append(actors, tickActor{ref: ref, spd: actorSpeed(g, ref), ready: g.Battle.Readiness[ref]})
	}
	if len(actors) == 0 {
		return nil
	}
	// Only positive-SPD actors reach the ATB gate; counting SPD-0 actors toward
	// `target` would keep actedCount from reaching it (picker skips them below),
	// leaving the loop to escape only via maxSlots. SPD=0 opts an actor out.
	target := 0
	for _, a := range actors {
		if a.spd > 0 {
			target++
		}
	}
	if target == 0 {
		// Degenerate: every living actor has SPD <= 0 (only via hand-authored
		// stats). Returning nil would loop beginNewRound forever to a stack
		// overflow, so give each living actor one slot in declaration order.
		fallback := make([]core.ActorRef, 0, len(actors))
		for _, a := range actors {
			fallback = append(fallback, a.ref)
		}
		return fallback
	}
	queue := make([]core.ActorRef, 0, target*2)
	actedCount := 0
	threshold := core.ATBReadyThreshold
	// Hard cap so a pathological SPD spread can't blow past this multiple of participants.
	maxSlots := target * core.ATBQueueSlotMultiplier
	for actedCount < target {
		// maxSlots caps SURPLUS turns but must never starve a not-yet-acted actor.
		// Once capped, keep advancing the clock but only pick actors that haven't
		// acted — else an extreme spread lets a fast actor fill every slot and skip
		// the slow one's guaranteed turn (and the round-boundary housekeeping).
		capped := len(queue) >= maxSlots
		// Pick the actor reaching threshold in the fewest ticks; compared via
		// cross-multiplication to stay integer.
		bestIdx := -1
		for i := range actors {
			if actors[i].spd <= 0 {
				continue
			}
			if capped && actors[i].acted {
				continue
			}
			if bestIdx < 0 {
				bestIdx = i
				continue
			}
			// Floor each "need" at 0 first: persisted readiness can carry OVER
			// threshold (negative need), and without the floor the negative
			// extrapolation orders two already-ready actors by spd instead of by
			// readiness. Floored, both read as "0 ticks" and the tie-break picks
			// the larger surplus.
			ni := threshold - actors[i].ready
			if ni < 0 {
				ni = 0
			}
			nb := threshold - actors[bestIdx].ready
			if nb < 0 {
				nb = 0
			}
			li := ni * actors[bestIdx].spd
			lb := nb * actors[i].spd
			if li < lb || (li == lb && actors[i].ready > actors[bestIdx].ready) {
				bestIdx = i
			}
		}
		if bestIdx < 0 {
			break
		}
		need := threshold - actors[bestIdx].ready
		ticks := (need + actors[bestIdx].spd - 1) / actors[bestIdx].spd
		if ticks < 0 {
			ticks = 0
		}
		for i := range actors {
			actors[i].ready += ticks * actors[i].spd
		}
		queue = append(queue, actors[bestIdx].ref)
		actors[bestIdx].ready -= threshold
		if !actors[bestIdx].acted {
			actors[bestIdx].acted = true
			actedCount++
		}
	}
	if persist {
		// Carry each actor's leftover gauge into the next round, rebuilt fresh so
		// dead actors are pruned automatically.
		next := make(map[core.ActorRef]int, len(actors))
		for _, a := range actors {
			next[a.ref] = a.ready
		}
		g.Battle.Readiness = next
	}
	return queue
}

// actorSpeed returns the actor's SPD (both sides read Stats.SPD). Webbed party
// members have effective SPD halved (floor 1) — "you still act, but the world
// gets ahead of you." Enemy-side Webbed isn't inflicted today (party-only branch).
func actorSpeed(g *core.GameState, actor core.ActorRef) int {
	if actor.IsParty {
		if !actor.ValidPartyIndex(g.Party) {
			return 0
		}
		spd := core.EffectiveStatsPtr(&g.Party[actor.Index]).SPD
		// Fleet Footed (Thief) lifts turn rate before the Webbed halving.
		spd += core.PassiveRank(&g.Party[actor.Index], core.PassiveFleetFooted) * core.FleetFootedSPDPerRank
		if g.Party[actor.Index].WebbedTurns > 0 {
			spd /= core.WebbedSpeedDivisor
		}
		return floorSPD(spd)
	}
	m := core.BattleMemberAt(g, actor.Index)
	if m == nil {
		return 0
	}
	// Effective SPD folds any debuff (Cripple).
	return floorSPD(core.EffectiveEnemyStats(m).SPD)
}

// floorSPD clamps SPD to >=1: a 0-SPD actor never crosses the ATB threshold, so it
// would drop out of the queue (and never tick its own debuff down) — the floor opts
// it back in. Crippled foes still act, just rarely.
func floorSPD(spd int) int {
	return max(spd, 1)
}

// pushEnemyReadiness shoves an enemy's carry-over ATB gauge back by `amount`
// (floored at 0) — the Warrior's Sunder. A one-shot edit, distinct from the SPD
// debuff. No-ops on a non-positive push; returns true when the gauge moved.
func pushEnemyReadiness(g *core.GameState, slot, amount int) bool {
	if amount <= 0 {
		return false
	}
	ref := core.ActorRef{IsParty: false, Index: slot}
	if g.Battle.Readiness == nil {
		g.Battle.Readiness = map[core.ActorRef]int{}
	}
	cur := g.Battle.Readiness[ref]
	before := cur
	core.SubFloorZero(&cur, amount)
	g.Battle.Readiness[ref] = cur
	// Honest move report: a target with no banked readiness (the common case
	// right after it acts) is floored at 0 — the gauge didn't move, so the
	// caller's "turn is shoved back" line must not fire.
	return cur != before
}

// actorAppearsBefore reports whether `ref` occupies any queue slot strictly
// before `cursor`. Used to tick an ingested member's Poison only on their FIRST
// slot in the round (the ATB queue can hold the same fast actor several times).
func actorAppearsBefore(queue []core.ActorRef, cursor int, ref core.ActorRef) bool {
	if cursor > len(queue) {
		cursor = len(queue)
	}
	for i := 0; i < cursor; i++ {
		if queue[i] == ref {
			return true
		}
	}
	return false
}

// startActorTurn opens the turn of the actor at the queue cursor. Per turn:
// skip-if-dead → burn tick (may kill) → input action. Off the queue end starts a
// fresh round (which may end the battle).
func startActorTurn(g *core.GameState) {
	// Loop rather than tail-recurse: a single call can skip many consecutive slots
	// under ATB carry-over, and per-skip recursion would grow the stack with the
	// queue length. Only a real turn / round boundary / battle-end transition returns.
	for {
		for g.Battle.QueueCursor < len(g.Battle.Queue) {
			skipped := g.Battle.Queue[g.Battle.QueueCursor]
			if isActorAlive(g, skipped) {
				break
			}
			// A member ingested mid-round (after the queue was built) still owes
			// their end-of-turn Poison tick — "Poison survives the lockout". The
			// helper no-ops on dead/enemy actors.
			//
			// Tick ONLY on their FIRST queue slot: ATB carry-over can hold a fast
			// member in several slots, and an earlier slot already ticked — without
			// the guard a mid-round-ingested fast member double-drains in one round.
			if !actorAppearsBefore(g.Battle.Queue, g.Battle.QueueCursor, skipped) {
				tickPoisonAfterPartyTurn(g, skipped)
			}
			g.Battle.QueueCursor++
		}
		if g.Battle.QueueCursor >= len(g.Battle.Queue) {
			beginNewRound(g)
			return
		}
		actor := g.Battle.Queue[g.Battle.QueueCursor]

		// Burn ticks at the burning actor's turn start; a burn-kill skips their
		// action and checks win in case it took the last enemy.
		if killed := tickBurnAtTurnStart(g, actor); killed {
			if winIfEnemiesWiped(g, core.ActivePack(g), core.BattleMembers(g), "The fire finishes them.") {
				return
			}
			// This turn slot is still consumed, so advance any pending Meteor fuse (as
			// finishActorTurn/advanceSkippedTurn do) — else it freezes for the burn-death
			// turn. Meteor only hits enemies, so a fresh enemy-wipe check covers its kills.
			resolveMeteorIfDue(g)
			if winIfEnemiesWiped(g, core.ActivePack(g), core.BattleMembers(g), "The meteor finishes them.") {
				return
			}
			// The burning actor is skipped, not acting — zero the per-turn tallies so its
			// burn/Meteor kills don't leak into the next actor's Bloodthirst / Killing Spree.
			g.Battle.PhysDamageThisTurn = 0
			g.Battle.EnemyKillsThisTurn = 0
			g.Battle.QueueCursor++
			continue
		}

		// Sleep / Stun both cost the turn and tick at turn start. Sleep clears on
		// damage (in the damage helpers); Stun runs its full duration regardless.
		// They tick INDEPENDENTLY: a both-afflicted actor drains BOTH counters this
		// turn — ticking only one would silently extend the other. Sleep is evaluated
		// first and Stun's log line is suppressed when Sleep already spoke, so the
		// turn still reads as "asleep". advanceSkippedTurn returns true when its DoT
		// tick ended the battle.
		asleep := tickSleepAtTurnStart(g, actor)
		stunned := tickStunAtTurnStart(g, actor, asleep)
		if asleep || stunned {
			if advanceSkippedTurn(g, actor) {
				return
			}
			continue
		}

		if actor.IsParty {
			beginPartyTurn(g, actor.Index)
			return
		}
		if beginEnemyAttack(g, actor.Index) {
			return
		}
		// Covered back-row foe with no reaching attack — pass its turn (elapses
		// statuses/DoT + advances the queue, like a slept actor).
		if advanceSkippedTurn(g, actor) {
			return
		}
		continue
	}
}

// advanceSkippedTurn closes out a turn that Sleep/Stun cost the actor. It still
// ELAPSES per-turn effects as a normal turn would: clears the Defend brace, drains
// non-damaging statuses, AND ticks Poison (the tick must fire here too, else a
// repeatedly-slept poisoned actor's DoT freezes for the whole lockout). The tick
// can kill, so win is checked before advancing.
//
// Returns true when the tick ended the battle (caller must stop); false after
// advancing QueueCursor so the caller continues to the next slot.
func advanceSkippedTurn(g *core.GameState, actor core.ActorRef) bool {
	consumeDefendAndGuardOnSkip(g, actor)
	drainNonDamagingPartyStatuses(g, actor)
	drainNonDamagingEnemyStatuses(g, actor)
	// No first-slot guard here (unlike startActorTurn): runs once per real turn end.
	tickPoisonAfterPartyTurn(g, actor)
	tickEnemyEndOfTurnDoTs(g, actor)
	// A pending Meteor counts down on THIS turn slot too — same reasoning as the DoT
	// ticks above: a fuse that only advanced on un-skipped turns would freeze through a
	// Sleep/Stun lockout and land late. Its kills are caught by the wipeout check below.
	resolveMeteorIfDue(g)
	// Zero the per-turn tallies (symmetry with finishActorTurn) so no stale figure
	// leaks into the next actor's Bloodthirst / Killing Spree — a DoT/Meteor kill on
	// this skipped turn must not credit the next actor with a kill it never made.
	g.Battle.PhysDamageThisTurn = 0
	g.Battle.EnemyKillsThisTurn = 0
	// Party wipe FIRST (matches finishActorTurn/Update): if a skipped turn downs the
	// last member AND a DoT/Meteor kills the last enemy in the same tick, it's a loss,
	// never a zero-survivor victory — winBattle would set Phase=BattleWon and Update's
	// party-wipe gate could no longer correct it.
	if checkPartyWipeout(g) || checkEnemyWipeout(g) {
		return true
	}
	// A DoT tick above can kill the targeted enemy; move the cursor off the corpse.
	repointEnemyCursorIfDead(g)
	g.Battle.QueueCursor++
	return false
}

// consumeDefendAndGuardOnSkip expires the one-round braces — Defend and Warrior's
// Guard cover — on a Sleep/Stun-skipped (or covered-foe-passed) turn, mirroring
// beginPartyTurn's normal-turn clear. Without the Guard clear a slept/stunned
// guardian keeps redirecting allies' hits onto itself for extra rounds (it stays
// MemberAvailable, so redirectToGuardian still routes to it). No-ops on enemies
// (neither state).
func consumeDefendAndGuardOnSkip(g *core.GameState, actor core.ActorRef) {
	if actor.IsParty && actor.ValidPartyIndex(g.Party) {
		g.Party[actor.Index].Defending = false
		core.ClearGuardBy(g.Party, actor.Index)
	}
}

// drainNonDamagingPartyStatuses ticks the no-damage party counters (Webbed,
// Confused) for the actor whose turn just ended. Called from both finishActorTurn
// and the Sleep/Stun skip path, else those statuses outlast their duration on a
// skipped turn. No-ops on enemies and zeroed counters.
//
// NOTE: this is a hand-maintained tick list, NOT table-driven — a NEW timed `*Turns`
// counter must be added here (or to the enemy mirror) by hand to drain each turn. The
// death-clear classifier (partyDeathStatuses in actions.go, reflect-asserted complete)
// forces every `*Turns` counter to be acknowledged there; use that same edit as the
// reminder to wire a per-turn drain here.
func drainNonDamagingPartyStatuses(g *core.GameState, actor core.ActorRef) {
	tickWebbedAfterPartyTurn(g, actor)
	tickConfusedAfterPartyTurn(g, actor)
	tickBlessAfterPartyTurn(g, actor)
	tickIceArmorAfterPartyTurn(g, actor)
	tickRegenAfterPartyTurn(g, actor)
	tickVanishAfterPartyTurn(g, actor)
	applyOverchargeRegen(g, actor)
}

// tickVanishAfterPartyTurn drains the Thief's Vanish (untargetable) one turn at the
// end of their turn; clears silently. Re-targetable once it hits 0.
func tickVanishAfterPartyTurn(g *core.GameState, actor core.ActorRef) {
	tickPartyStatusCounter(g, actor, func(m *core.PartyMember) *int { return &m.VanishTurns }, "")
}

// drainNonDamagingEnemyStatuses ticks the enemy no-damage counters (buff/debuff
// turns) — the enemy mirror of drainNonDamagingPartyStatuses, called at the same
// seams so a debuff elapses even on a skipped turn. No-ops on party actors.
func drainNonDamagingEnemyStatuses(g *core.GameState, actor core.ActorRef) {
	tickEnemyBuffAfterTurn(g, actor)
	tickEnemyTauntAfterTurn(g, actor)
}

// tickSkipStatusAtTurnStart drains one tick of a skip-this-turn status (Sleep /
// Stun) at the afflicted actor's turn start, logs "<Name> <verb>." (unless quiet),
// and returns true if the actor must skip. Shared body for Sleep/Stun. quiet lets a
// second skip-status drain its counter without logging when an earlier one already
// spoke this turn. (Sleep's wake-on-damage path lives in the damage helpers,
// independent of this drain.)
func tickSkipStatusAtTurnStart(
	g *core.GameState, actor core.ActorRef, verb string, quiet bool,
	counterRefParty func(*core.PartyMember) *int,
	counterRefEnemy func(*core.Enemy) *int,
) bool {
	if actor.IsParty {
		if !actor.ValidPartyIndex(g.Party) {
			return false
		}
		m := &g.Party[actor.Index]
		if m.HP <= 0 {
			return false
		}
		c := counterRefParty(m)
		if *c <= 0 {
			return false
		}
		*c--
		if !quiet {
			setBattleMessage(g, fmt.Sprintf("%s %s.", m.Name, verb))
		}
		return true
	}
	enemy, ok := livingEnemyAt(g, actor.Index)
	if !ok {
		return false
	}
	c := counterRefEnemy(enemy)
	if *c <= 0 {
		return false
	}
	*c--
	if !quiet {
		setBattleMessage(g, fmt.Sprintf("%s %s.", core.EnemyDisplayName(enemy), verb))
	}
	return true
}

func tickSleepAtTurnStart(g *core.GameState, actor core.ActorRef) bool {
	return tickSkipStatusAtTurnStart(g, actor, "is asleep", false,
		func(m *core.PartyMember) *int { return &m.SleepTurns },
		func(e *core.Enemy) *int { return &e.SleepTurns })
}

// tickStunAtTurnStart drains a Stun tick and skips the actor's turn. quiet
// suppresses the log line (set when Sleep already announced the skip this turn) so
// the counter still drains. No skill currently inflicts party Stun, but m.StunTurns
// is a real classified counter (partyDeathStatuses) — the party branch ticks/honors
// it if anything ever sets it.
func tickStunAtTurnStart(g *core.GameState, actor core.ActorRef, quiet bool) bool {
	return tickSkipStatusAtTurnStart(g, actor, "is stunned", quiet,
		func(m *core.PartyMember) *int { return &m.StunTurns },
		func(e *core.Enemy) *int { return &e.StunTurns })
}

// isActorAlive reports whether a queue actor is still in the fight. For party
// slots this means alive AND not ingested (a swallowed member skips until the mantrap dies).
func isActorAlive(g *core.GameState, actor core.ActorRef) bool {
	if actor.IsParty {
		return core.PartyMemberAvailable(g.Party, actor.Index)
	}
	return core.BattleEnemyAlive(g, actor.Index)
}

// repointEnemyCursorIfDead moves EnemyIndex onto the next living enemy when the
// pointed one died (leaving it put if none remain) — the "no cursor on a corpse" rule.
func repointEnemyCursorIfDead(g *core.GameState) {
	if !core.BattleEnemyAlive(g, g.Battle.EnemyIndex) {
		if next := core.NextLivingBattleEnemy(g); next >= 0 {
			g.Battle.EnemyIndex = next
		}
	}
}

// finishActorTurn is the single hand-off every action's apply* path uses: checks
// win/lose, repoints EnemyIndex off a dead target, advances the cursor, starts the
// next turn. Party Poison ticks HERE (after the action, before win/lose) so a
// poison kill on the last member is honored.
func finishActorTurn(g *core.GameState) {
	g.Battle.ClearTiming()
	// Canonical clear of ChargeNeedsRelease — every finished action flows through
	// here, guaranteeing the next turn starts with a fresh release-gate state.
	g.Battle.ChargeNeedsRelease = false
	// Defensively clear HitStop so an early-exit apply path can't leak a stuck freeze.
	g.Battle.HitStop = 0
	if g.Battle.QueueCursor >= 0 && g.Battle.QueueCursor < len(g.Battle.Queue) {
		// End-of-turn DoTs / status drains (party poison, enemy poison+bleed, both
		// sides' non-damaging counters): run after the action so the actor acts
		// first. Each helper short-circuits on the wrong actor kind. No first-slot
		// guard (unlike startActorTurn) — runs once per real turn end.
		tickPoisonAfterPartyTurn(g, g.Battle.Queue[g.Battle.QueueCursor])
		tickEnemyEndOfTurnDoTs(g, g.Battle.Queue[g.Battle.QueueCursor])
		drainNonDamagingEnemyStatuses(g, g.Battle.Queue[g.Battle.QueueCursor])
		drainNonDamagingPartyStatuses(g, g.Battle.Queue[g.Battle.QueueCursor])
		// Bloodthirst converts the actor's phys output to lifesteal (no-op without the node).
		applyBloodthirst(g, g.Battle.Queue[g.Battle.QueueCursor])
		// Killing Spree grants an ATB burst if the turn scored a kill (no-op without the node).
		applyKillingSpree(g, g.Battle.Queue[g.Battle.QueueCursor])
		// The summoner's Ancestral Spirit shade strikes alongside them (no-op without one).
		tickAncestralSpirit(g, g.Battle.Queue[g.Battle.QueueCursor])
	}
	// A pending Meteor counts down each turn and lands when its fuse runs out. Before
	// the tally-zero so its kills don't leak into the next actor's Killing Spree.
	resolveMeteorIfDue(g)
	// Zero the per-turn tallies now they're banked, so reflect/counter damage can't
	// leak into the next member's Bloodthirst / Killing Spree.
	g.Battle.PhysDamageThisTurn = 0
	g.Battle.EnemyKillsThisTurn = 0
	// Party wipe FIRST: a killing blow whose own end-of-turn tick (poison,
	// Overcharge recoil) also fells the last member must read as a loss — winning
	// with zero living members would exit to explore in an unrecoverable state.
	if checkPartyWipeout(g) {
		return
	}
	if checkEnemyWipeout(g) {
		return
	}
	repointEnemyCursorIfDead(g)
	g.Battle.QueueCursor++
	startActorTurn(g)
}

// --- Party turn ------------------------------------------------------------

// beginPartyTurn opens the action menu for the member. Owns Phase=BattlePlayer
// plus Defending reset (defending lasts exactly one round-trip).
func beginPartyTurn(g *core.GameState, partyIndex int) {
	g.Battle.Phase = core.BattlePlayer
	g.Battle.ClearTiming()
	g.Battle.TimingIntro = 0
	g.Battle.EnemyAttacker = core.NoIndex
	g.Battle.CurrentParty = partyIndex
	resetBattleAction(g)
	if core.PartyIndexInRange(g.Party, partyIndex) {
		g.Party[partyIndex].Defending = false
		// Guard covers "this round" — it lapses when the guardian acts again.
		core.ClearGuardBy(g.Party, partyIndex)
		g.Battle.PartyTarget = partyIndex
	} else {
		// Out-of-bounds partyIndex (bogus queue actor): fall back to the first
		// living member, or route to BattleLost rather than open the menu for a corpse.
		g.Battle.PartyTarget = core.FirstLivingPartyMember(g.Party)
		if g.Battle.PartyTarget < 0 {
			loseBattle(g, core.BattleLossMessage(g))
			return
		}
	}
	repointEnemyCursorIfDead(g)
}

func updatePlayerBattle(g *core.GameState) {
	// Defensive net: if the current actor died between turns, skip ahead.
	if !core.PartyMemberAvailable(g.Party, g.Battle.CurrentParty) {
		g.Battle.QueueCursor++
		startActorTurn(g)
		return
	}

	switch g.Battle.ActionMode {
	case core.ActionMenu:
		updateActionMenu(g)
	case core.ActionEnemyTarget:
		updateEnemyTargeting(g)
	case core.ActionPartyTarget:
		updatePartyTargeting(g)
	case core.ActionItemMenu:
		updateItemMenu(g)
	case core.ActionItemTarget:
		updateItemTarget(g)
	case core.ActionSkillMenu:
		updateSkillMenu(g)
	case core.ActionFleeConfirm:
		updateFleeConfirm(g)
	case core.ActionSwapTarget:
		updateSwapTarget(g)
	default:
		// Fail loudly on an unhandled mode rather than silently routing to the menu —
		// parity with the modal/updateMenu switches' "new kind panics at startup" contract.
		panic(fmt.Sprintf("updatePlayerBattle: unhandled ActionMode %d", g.Battle.ActionMode))
	}
}

// tickFlashHold drains the post-press flash timer, then (high grades) the hit-stop
// freeze. Returns true while either is on screen; when both drain, fires onResolve
// and still returns true so the caller doesn't double-tick the bar this frame.
//
// Great/Excellent order: flash (cursor frozen) → HitStop (world freezes) →
// onResolve. Miss/Nice/Good skip HitStop (HitStopFor returns 0).
func tickFlashHold(g *core.GameState, dt float32, onResolve func()) bool {
	if g.Battle.HitStop > 0 {
		g.Battle.HitStop -= dt
		if g.Battle.HitStop > 0 {
			return true
		}
		g.Battle.HitStop = 0
		fireImpact(g, onResolve)
		return true
	}
	if g.Battle.TimingFlash <= 0 {
		return false
	}
	g.Battle.TimingFlash -= dt
	if g.Battle.TimingFlash > 0 {
		return true
	}
	g.Battle.TimingFlash = 0
	if stop := core.HitStopFor(g.Battle.Timing.Quality); stop > 0 {
		// Great/Excellent: freeze first, then fireImpact resolves + shakes once
		// the freeze releases (see fireImpact — shaking here read as pre-hit).
		g.Battle.HitStop = stop
		return true
	}
	fireImpact(g, onResolve)
	return true
}

// fireImpact runs the action's apply (onResolve) and THEN arms the grade shake, so
// the shake lands WITH the impact after the freeze, not during it. CombatShakeFor
// returns 0 for Miss/Nice and a graded peak for Good/Great/Excellent; a crit/AoE
// punch armed in onResolve STACKS on top of it via AddCombatShake.
func fireImpact(g *core.GameState, onResolve func()) {
	// Capture the grade BEFORE onResolve — every resolve path calls ClearTiming(),
	// which zeroes Timing.Quality to Miss, so reading it after would always give a
	// zero-peak (no-op) shake and the graded Good/Great/Excellent shake would never fire.
	basePeak, baseDur := core.CombatShakeFor(g.Battle.Timing.Quality)
	onResolve()
	core.AddCombatShake(&g.Battle, basePeak, baseDur)
}

// driveSequenceInput reads one directional tap into the active sequence/recall bar
// and fires the per-slot pulse on a correct land. Shared by the Sequence and Recall
// kinds (Recall gates the call on its reveal phase).
func driveSequenceInput(g *core.GameState) {
	prevCursor := g.Battle.Timing.SequenceCursor
	switch {
	case g.Battle.Timing.Resolved:
		// no input once resolved
	case input.ArrowUpPressed():
		g.Battle.Timing.SequenceInput(core.SeqDirUp)
	case input.ArrowRightPressed():
		g.Battle.Timing.SequenceInput(core.SeqDirRight)
	case input.ArrowDownPressed():
		g.Battle.Timing.SequenceInput(core.SeqDirDown)
	case input.ArrowLeftPressed():
		g.Battle.Timing.SequenceInput(core.SeqDirLeft)
	}
	// Pulse the just-landed slot only on Correct (a wrong tap draws red, no bounce).
	if g.Battle.Timing.SequenceCursor > prevCursor &&
		prevCursor < len(g.Battle.Timing.SequenceResults) &&
		g.Battle.Timing.SequenceResults[prevCursor] == core.SeqResultCorrect {
		g.Battle.SequencePulseTimer = core.SequencePulseDuration
		g.Battle.SequencePulseIndex = prevCursor
	}
}

func updateAttackTiming(g *core.GameState, dt float32) {
	if tickFlashHold(g, dt, func() { applyPendingAction(g, g.Battle.Timing.Quality) }) {
		return
	}
	// Charge-bar release gate: while the carried-over confirm key is still held,
	// suppress every engage path (intro-skip, Hold, Release) so the player can't
	// drive the bar from menu-confirm. Intro still ticks so the auto-arm isn't blocked.
	if g.Battle.ChargeNeedsRelease && !input.AttackTimingHeld() {
		g.Battle.ChargeNeedsRelease = false
	}
	engageReady := !g.Battle.ChargeNeedsRelease
	if g.Battle.TimingIntro > 0 {
		// Charge bars skip the intro only on a FRESH edge press after the release
		// gate clears — else the target-confirm Enter would bleed in and engage the
		// charge, then the natural key release would resolve it at quality=Miss.
		isCharge := g.Battle.Timing.IsChargeFamily()
		if isCharge && engageReady && input.AttackTimingPressed() {
			g.Battle.TimingIntro = 0
		} else {
			g.Battle.TimingIntro -= dt
			return
		}
	}

	// Each arm only DRIVES input for its kind; the shared advance-the-bar tick is
	// hoisted out below.
	switch g.Battle.Timing.Kind {
	case core.TimingKindCharge, core.TimingKindOvercharge:
		// Hold/Release gated by engageReady: still holding at auto-arm doesn't
		// engage (cursor ticks to Miss); must release-then-press. Overcharge shares
		// this flow, only its resolve differs.
		if engageReady && !g.Battle.Timing.Resolved && input.AttackTimingHeld() {
			g.Battle.Timing.Hold()
		}
		if engageReady && !g.Battle.Timing.Resolved && input.AttackTimingReleased() {
			g.Battle.Timing.Release()
		}
	case core.TimingKindSequence:
		driveSequenceInput(g)
	case core.TimingKindRecall:
		// Memory bar: taps ignored during the reveal phase; once hidden, drives the
		// same per-slot grading as the sequence bar.
		if g.Battle.Timing.RecallHidden() {
			driveSequenceInput(g)
		}
	case core.TimingKindReels:
		// Slot gamble: each press stops the next reel.
		if !g.Battle.Timing.Resolved && input.AttackTimingPressed() {
			g.Battle.Timing.StopNextReel()
		}
	default:
		if !g.Battle.Timing.Resolved && input.AttackTimingPressed() {
			g.Battle.Timing.Press()
		}
	}

	if !g.Battle.Timing.Resolved {
		g.Battle.Timing.Tick(dt)
	}

	resolveTimingBar(g, func() { applyPendingAction(g, g.Battle.Timing.Quality) })
}

// gradeSounds is the per-grade audio cue table — all bars dispatch off it so the
// same grade sounds the same.
// Two bands by design: a miss cue, the "landed" cue for the lower grades, and the
// "great" cue for the top grades. The shared value across each band is intentional
// — retuning a band means changing BOTH of its cells.
var gradeSounds = [...]audio.Sound{
	core.TimingQualityMiss: audio.SoundInputMiss,
	// Landed band.
	core.TimingQualityNice: audio.SoundInputHit,
	core.TimingQualityGood: audio.SoundInputHit,
	// Great band.
	core.TimingQualityGreat:     audio.SoundInputGreat,
	core.TimingQualityExcellent: audio.SoundInputGreat,
}

// init asserts gradeSounds covers every timing grade.
func init() {
	if len(gradeSounds) != int(core.TimingQualityCount) {
		panic("battle: gradeSounds length must match core.TimingQualityCount")
	}
}

// soundForGrade picks the input cue for a grade; out-of-range falls back to Miss.
func soundForGrade(q int) audio.Sound {
	if q < 0 || q >= len(gradeSounds) {
		return gradeSounds[core.TimingQualityMiss]
	}
	return gradeSounds[q]
}

// resolveTimingBar is the shared tail of the attack and defend bars: a registered
// press flashes + plays the grade cue and holds; a clean miss plays the miss cue
// and runs onResolve. Returns true when the caller should stop this tick.
func resolveTimingBar(g *core.GameState, onResolve func()) bool {
	if !g.Battle.Timing.Resolved {
		return true
	}
	if g.Battle.Timing.Pressed {
		g.Battle.TimingFlash = core.TimingFlashDuration
		audio.Play(soundForGrade(g.Battle.Timing.Quality))
		return true
	}
	audio.Play(audio.SoundInputMiss)
	onResolve()
	return false
}

// --- Enemy turn ------------------------------------------------------------

// enemyActionIsMelee reports whether this turn's action is a melee swing — the basic
// attack, or a melee-class skill (Stoneslam) — i.e. the reach-gated actions. SkillNone
// resolves via the enemy's basic-attack class (SkillAttackClassFor doesn't cover the
// implicit basic attack); magic/ranged skills are not gated (they cast through any row).
func enemyActionIsMelee(enemy *core.Enemy, skill core.SkillID) bool {
	if skill == core.SkillNone {
		return enemy != nil && core.EnemyBasicAttackClass(enemy.Kind).IsMelee()
	}
	return core.SkillAttackClassFor(skill).IsMelee()
}

// enemyAttackWhiffs rolls the foe's accuracy (EffectiveEnemyStats so Blind lowers
// it) and reports a clean miss. Nil-safe; the one place the attack accuracy roll
// lives so the skill-melee and plain-melee branches can't drift.
func enemyAttackWhiffs(g *core.GameState, enemy *core.Enemy) bool {
	return enemy != nil && !core.RollEnemyHit(g.Rand(), core.EffectiveEnemyStats(enemy))
}

// beginEnemyAttack arms the defend bar against the enemy at slot (index into the
// active pack's Members).
func beginEnemyAttack(g *core.GameState, slot int) bool {
	// Pick the turn's action first (casters may cast instead of melee; SkillNone =
	// plain melee), so the reach gate below can see whether this turn even needs reach.
	enemy := core.BattleMemberAt(g, slot)
	skill := core.SkillNone
	if enemy != nil {
		skill = enemyAIPickSkill(g, *enemy, slot)
	}
	// Melee reach gate — the mirror of the party's BackRowMeleeBlocked. A melee action
	// (the basic swing OR a melee-class skill like Stoneslam) from a foe NOT in the
	// effective front (a covered back-row foe) can't connect; magic/ranged skills cast
	// through any row. With no reaching action this turn the covered foe can't act.
	if enemyActionIsMelee(enemy, skill) && !core.EnemyInEffectiveFront(core.BattleMembers(g), slot) {
		return false
	}
	g.Battle.EnemyAttacker = slot
	g.Battle.TimingFlash = 0
	g.Battle.TimingIntro = core.EnemyTurnIntro
	g.Battle.Phase = core.BattleEnemyTiming
	g.Battle.EnemyPendingSkill = skill
	g.Battle.EnemyAttackMisses = false
	if skill != core.SkillNone {
		// A single-target melee skill (the mantrap's Ingest bite) rolls accuracy like
		// a basic swing — a physical lunge can whiff. On a miss, EnemyAttackMisses wins
		// the resolve switch over the pending skill, so the skill never lands. Magic/
		// ranged casts and AoE melee (Stoneslam, countered by Defend) still auto-connect.
		if enemyActionIsMelee(enemy, skill) && !core.SkillEffectFor(skill).AppliesAOEParty &&
			enemyAttackWhiffs(g, enemy) {
			g.Battle.EnemyAttackMisses = true
		}
		// Pre-resolve Timing so the defend bar never arms — the intro elapses and
		// resolveAndFinish routes through resolveEnemySpell (or the miss narration).
		g.Battle.Timing = core.TimingState{Resolved: true}
		return true
	}
	// Plain melee: roll accuracy NOW, before the defend bar. A clean whiff has
	// nothing to defend, so skip the minigame (same Resolved=true short-circuit) and
	// let the intro elapse into the miss. EffectiveEnemyStats so Blind lowers it.
	if enemyAttackWhiffs(g, enemy) {
		g.Battle.EnemyAttackMisses = true
		g.Battle.Timing = core.TimingState{Resolved: true}
		return true
	}
	g.Battle.Timing = core.NewTimingState(g.Rand(), core.DefendTimingDuration)
	return true
}

// enemyAIPickSkill picks a skill for the enemy's turn, or SkillNone (plain melee).
// Rolls against SkillCastChance (0 = never casts). `slot` filters per-instance
// dead-ends (Ingest by a mantrap with prey) BEFORE the roll so it bites instead.
func enemyAIPickSkill(g *core.GameState, enemy core.Enemy, slot int) core.SkillID {
	def := core.EnemyInfoFor(enemy)
	if len(def.Skills) == 0 || def.SkillCastChance <= 0 {
		return core.SkillNone
	}
	// Filter before roll: an empty usable list falls to melee WITHOUT consuming a
	// cast outcome; a non-empty list rolls normally. Reversing would have a
	// no-target mantrap "roll a cast" then have nothing to cast (AI flinch).
	usable := usableEnemySkills(g, def.Skills, slot)
	if len(usable) == 0 {
		return core.SkillNone
	}
	if g.Rand().Float64() >= def.SkillCastChance {
		return core.SkillNone
	}
	return usable[g.Rand().Intn(len(usable))]
}

// usableEnemySkills filters an enemy's Skills to those whose preconditions hold
// now, keyed off the casting slot for per-instance state (mantrap holding prey).
// Skills without per-instance gates always pass through.
func usableEnemySkills(g *core.GameState, skills []core.SkillID, slot int) []core.SkillID {
	enemy := core.BattleMemberAt(g, slot)
	// A covered back-row foe can't reach with a melee-class skill, so don't let the AI
	// pick one (it'd whiff its turn); it casts a magic/ranged skill or falls to a skip.
	canMelee := core.EnemyInEffectiveFront(core.BattleMembers(g), slot)
	out := make([]core.SkillID, 0, len(skills))
	for _, s := range skills {
		if !canMelee && core.SkillAttackClassFor(s).IsMelee() {
			continue
		}
		// Per-battle cast limit: drop the skill once used PerBattleCastLimit times.
		// A nil SkillCastCount reads as zero on lookup (no lazy-init for uncapped skills).
		if limit := core.SkillCastLimitFor(s); limit > 0 && enemy != nil {
			if enemy.SkillCastCount[s] >= limit {
				continue
			}
		}
		switch s {
		case core.SkillIngest:
			// Mantrap can't ingest while holding prey or with no available target.
			if core.MantrapHasPrey(g.Party, slot) {
				continue
			}
			if core.FirstAvailablePartyMember(g.Party) < 0 {
				continue
			}
		}
		out = append(out, s)
	}
	return out
}

// updateEnemyTiming drives the player's defend bar against the attacking enemy.
func updateEnemyTiming(g *core.GameState, dt float32) {
	if tickFlashHold(g, dt, func() { resolveAndFinishEnemyAttack(g) }) {
		return
	}
	if g.Battle.TimingIntro > 0 {
		g.Battle.TimingIntro -= dt
		return
	}
	// Spell-cast path: no defend bar (Timing pre-resolved). After the intro,
	// route to resolveAndFinishEnemyAttack where the spell applies.
	if g.Battle.EnemyPendingSkill != core.SkillNone {
		resolveAndFinishEnemyAttack(g)
		return
	}
	// Miss path: accuracy whiffed in beginEnemyAttack, no defend bar. Resolve
	// straight to the miss narration (skip the auto-miss SoundInputMiss tail — the
	// enemy fumbled its swing, the player didn't fumble a press).
	if g.Battle.EnemyAttackMisses {
		resolveAndFinishEnemyAttack(g)
		return
	}
	if !g.Battle.Timing.Resolved && input.DefendTimingPressed() {
		g.Battle.Timing.Press()
	}
	if !g.Battle.Timing.Resolved {
		g.Battle.Timing.Tick(dt)
	}
	resolveTimingBar(g, func() { resolveAndFinishEnemyAttack(g) })
}

// resolveAndFinishEnemyAttack applies the attacker's hit (scaled by defend
// quality) and advances the queue. Branches on miss / pending skill / plain melee.
func resolveAndFinishEnemyAttack(g *core.GameState) {
	switch {
	case g.Battle.EnemyAttackMisses:
		// A miss is always a melee action (basic swing or a melee skill like Ingest).
		// Clear the pending skill first so the miss-target peek uses the melee front-
		// gate, not the any-row cast peek — else the whiff can name a back-row member
		// the lunge could never have reached (and drift EnemyAttackCursor onto it).
		g.Battle.EnemyPendingSkill = core.SkillNone
		resolveEnemyMiss(g, g.Battle.EnemyAttacker)
	case g.Battle.EnemyPendingSkill != core.SkillNone:
		resolveEnemySpell(g, g.Battle.EnemyAttacker, g.Battle.EnemyPendingSkill)
	default:
		resolveEnemyAttacker(g, g.Battle.EnemyAttacker, g.Battle.Timing.Quality)
	}
	g.Battle.ClearTiming()
	g.Battle.EnemyAttacker = core.NoIndex
	g.Battle.EnemyPendingSkill = core.SkillNone
	g.Battle.EnemyAttackMisses = false
	finishActorTurn(g)
}

// resolveEnemySpell is the apply path for enemy-cast skills: resolves the context
// and dispatches to the enemySpellHandlers entry (the init guard in actions.go
// asserts every EnemyCastable skill has a handler and vice-versa).
func resolveEnemySpell(g *core.GameState, slot int, skill core.SkillID) {
	enemy, ok := livingEnemyAt(g, slot)
	if !ok {
		return
	}
	effect := core.SkillEffectFor(skill)
	// AoE phys (Stoneslam) and summons (Raise Bones) bypass the target gate — else
	// an all-ingested party would deadlock the necromancer's stalemate-breaking
	// summon. Single-target casts keep the gate (nothing to do without a target).
	target := -1
	if !effect.AppliesAOEParty && !effect.AppliesSummonSkeleton {
		// Single-target enemy casts are magic (any row), never front-gated —
		// PeekEnemyAttackerTarget keys off EnemyPendingSkill (non-None here).
		target = pickEnemyAttackTarget(g)
		if target < 0 {
			// No target (e.g. the last reachable ally was just swallowed). Surface
			// the no-op so the advancing forecast isn't mistaken for a frozen battle.
			setBattleMessage(g, fmt.Sprintf("%s hesitates.", core.EnemyDisplayName(enemy)))
			return
		}
	}
	def := core.EnemyInfoFor(*enemy)
	ctx := enemySpellCtx{
		g:         g,
		slot:      slot,
		target:    target,
		enemy:     enemy,
		def:       def,
		skillName: core.SkillName(skill),
		effect:    effect,
	}
	handler, ok := enemySpellHandlers[skill]
	if !ok {
		// Unreachable (init guard panics on missing handlers); defense-in-depth log.
		setBattleMessage(g, fmt.Sprintf("%s mutters something (unhandled skill %d).", core.TheEnemy(def), int(skill)))
		return
	}
	cast := handler(ctx)
	// Re-fetch the LIVE caster by slot AFTER dispatch: Raise Bones reallocates
	// pack.Members, dangling ctx.enemy, so the pre-dispatch pointer can't carry
	// post-cast writes. nil = the caster died mid-cast (e.g. a reflect).
	caster := core.BattleMemberAt(g, slot)
	if caster == nil {
		return
	}
	// AttackBump (the cast-lunge offset) on the live caster — every handler ends
	// with it applied exactly once.
	stampEnemyBump(caster)
	// Stamp the per-battle cast counter on the same live caster. Only a cast that
	// FIRED (cast==true) charges — a cancelled no-op mustn't burn a limited charge.
	if cast && core.SkillCastLimitFor(skill) > 0 {
		if caster.SkillCastCount == nil {
			caster.SkillCastCount = map[core.SkillID]int{}
		}
		caster.SkillCastCount[skill]++
	}
}

// --- Win / lose / boilerplate ----------------------------------------------

func tickQualityPopup(g *core.GameState, dt float32) {
	if g.Battle.LastQualityTimer <= 0 {
		return
	}
	g.Battle.LastQualityTimer -= dt
	if g.Battle.LastQualityTimer < 0 {
		g.Battle.LastQualityTimer = 0
	}
}

func winBattle(g *core.GameState, message string) {
	// Idempotency guard: the spoils tally below must fire once per win.
	if g.Battle.Phase == core.BattleWon {
		return
	}
	g.Battle.Phase = core.BattleWon
	g.Battle.Timer = core.VictoryDanceDuration
	g.Battle.ClearTiming()
	resetBattleAction(g)
	setBattleMessageCat(g, message, core.LogDeath) // the felling/victory banner reads as a foe death
	// Credit felled foes to the bestiary (kill counts drive the identify threshold).
	core.RecordBattleKills(g)
	// Snapshot pre-award level/XP for the spoils screen before AwardBattleXP mutates
	// the totals (dead members included, rendered greyed).
	spoils := core.VictorySpoils{Members: make([]core.MemberSpoils, len(g.Party))}
	for i := range g.Party {
		spoils.Members[i] = core.MemberSpoils{
			Slot:      i,
			BeforeLvl: g.Party[i].Level,
			BeforeXP:  g.Party[i].XP,
		}
	}
	// XP fires once: living members get the full pack value, dead get nothing.
	// Level-ups queue points onto PendingLevelUps / SkillPoints; allocation is
	// deferred to the Tome (nothing auto-opens here).
	perMember, leveled := core.AwardBattleXP(g)
	if perMember > 0 {
		// Don't re-embed `message` (the kill line already logged) — it'd print twice.
		setBattleMessage(g, fmt.Sprintf("Party gains %d XP each.", perMember))
		for _, idx := range leveled {
			setBattleMessage(g, fmt.Sprintf("%s reaches level %d!", g.Party[idx].Name, g.Party[idx].Level))
		}
	}
	for i := range g.Party {
		spoils.Members[i].AfterLvl = g.Party[i].Level
		spoils.Members[i].AfterXP = g.Party[i].XP
		if g.Party[i].HP > 0 {
			spoils.Members[i].GainedXP = perMember
		}
	}
	// Gold + drops off the same pack; logged separately so a zero-gold fight
	// doesn't print "finds 0 gold".
	gold, drops := core.AwardBattleLoot(g)
	if gold > 0 {
		setBattleMessage(g, fmt.Sprintf("Party finds %d gold.", gold))
	}
	for _, kind := range drops {
		setBattleMessage(g, fmt.Sprintf("Picked up %s.", core.ItemInfo(kind).Name))
	}
	// Arm the spoils screen and ring the fanfare. Active=true switches Update off
	// the timed auto-leave and onto updateVictorySpoils.
	spoils.Gold = gold
	spoils.Drops = aggregateDrops(drops)
	spoils.Active = true
	g.Battle.Spoils = spoils
	g.Battle.VictoryElapsed = 0
	g.Battle.VictoryLevelSfxCursor = 0
	g.Battle.VictoryLootSfxCursor = 0
	g.Battle.VictoryTickSfxCursor = 0
	audio.Play(audio.SoundVictory)
}

// aggregateDrops folds the flat ItemKind list into display stacks (kind→count),
// preserving first-seen order.
func aggregateDrops(drops []core.ItemKind) []core.ItemStack {
	if len(drops) == 0 {
		return nil
	}
	stacks := make([]core.ItemStack, 0, len(drops))
	at := make(map[core.ItemKind]int, len(drops))
	for _, k := range drops {
		if i, ok := at[k]; ok {
			stacks[i].Count++
			continue
		}
		at[k] = len(stacks)
		stacks = append(stacks, core.ItemStack{Kind: k, Count: 1})
	}
	return stacks
}

// updateVictorySpoils drives the BattleWon phase. No captured spoils (debug
// skip-win) keeps the timed auto-leave; otherwise it advances the spoils clock,
// rings cues per threshold, and Confirm fast-forwards then tears down.
func updateVictorySpoils(g *core.GameState, dt float32) {
	if !g.Battle.Spoils.Active {
		g.Battle.Timer -= dt
		if g.Battle.Timer <= 0 && !battleDeathFadeActive(g) {
			leaveBattle(g, g.Area.QuietMessage)
			// Foe-killed triggers fire after teardown so the dialog overlays
			// explore, not the battle scene.
			core.FireFoeKilledTriggers(g)
		}
		return
	}
	g.Battle.VictoryElapsed += dt
	// Eased fill fraction, shared by the level-up and XP-tick cue checks below.
	fill := core.VictoryFillProgress(g.Battle.VictoryElapsed)
	// Ring the level-up cue as each bar crosses a threshold (cursor tracks rung count).
	shownLevels := levelsShownAt(g, fill)
	for g.Battle.VictoryLevelSfxCursor < shownLevels {
		audio.Play(audio.SoundLevelUp)
		g.Battle.VictoryLevelSfxCursor++
	}
	// Pop the loot cue as each row cascades in (cursor mirrors the renderer's reveal).
	shownLoot := core.VictoryLootRevealed(g.Battle.VictoryElapsed, len(g.Battle.Spoils.Drops))
	for g.Battle.VictoryLootSfxCursor < shownLoot {
		audio.Play(audio.SoundItemGet)
		g.Battle.VictoryLootSfxCursor++
	}
	// Count-up blip per VictoryXPPerTick of shown XP, tied to the eased fill;
	// capped to one Play per frame so a huge haul can't machine-gun.
	if tickIdx := xpTicksAt(g, fill); tickIdx > g.Battle.VictoryTickSfxCursor {
		audio.Play(audio.SoundXPTick)
		g.Battle.VictoryTickSfxCursor = tickIdx
	}
	if input.ConfirmPressed() {
		if !core.VictorySpoilsAnimDone(g.Battle.VictoryElapsed) {
			// Skip: snap to finished and mark every cue rung so the skip doesn't
			// fire a burst of sounds next frame.
			g.Battle.VictoryElapsed = core.VictorySpoilsAnimEnd()
			g.Battle.VictoryLevelSfxCursor = levelsShownAt(g, 1)
			g.Battle.VictoryLootSfxCursor = len(g.Battle.Spoils.Drops)
			g.Battle.VictoryTickSfxCursor = xpTicksAt(g, 1)
			return
		}
		leaveBattle(g, g.Area.QuietMessage)
		core.FireFoeKilledTriggers(g)
	}
}

// xpShownAt totals the XP visible across all members at fill fraction p (the
// count-up tick cadence keys off it).
func xpShownAt(g *core.GameState, p float32) int {
	total := 0
	for _, ms := range g.Battle.Spoils.Members {
		total += int(ms.AddedAt(p))
	}
	return total
}

// xpTicksAt is how many XP count-up blips have sounded by fill fraction p — the shown
// XP divided into VictoryXPPerTick steps, with the divisor guard (0 → no ticks) that
// both the running cue and the skip-to-end snap need, folded into one place.
func xpTicksAt(g *core.GameState, p float32) int {
	if core.VictoryXPPerTick <= 0 {
		return 0
	}
	return xpShownAt(g, p) / core.VictoryXPPerTick
}

// levelsShownAt totals the level-ups visible at fill fraction p, via the same
// MemberSpoils.ProjectAt the bars draw with (cue counter stays in lockstep).
func levelsShownAt(g *core.GameState, p float32) int {
	total := 0
	for _, ms := range g.Battle.Spoils.Members {
		if _, _, gained := ms.ProjectAt(p); gained > 0 {
			total += gained
		}
	}
	return total
}

// DebugSkipWin auto-resolves a pack as a win WITHOUT the battle scene (the "Skip
// Battles" toggle): fells every member, then runs the normal winBattle + leaveBattle
// bookkeeping. No-ops on an invalid / already-dead pack.
func DebugSkipWin(g *core.GameState, packIndex int) {
	if !validLivePack(g, packIndex) {
		return
	}
	g.Battle.ActivePack = packIndex
	g.Battle.EnemyIndex = core.NoIndex
	// Fell the whole pack so the win bookkeeping tallies it as a fought win.
	for i := range g.Packs[packIndex].Members {
		g.Packs[packIndex].Members[i].HP = 0
		g.Packs[packIndex].Members[i].Alive = false
	}
	winBattle(g, "Debug: skipped the battle.")
	// clearBattleResidual drops the pack (Phase==BattleWon) and resets transients.
	leaveBattle(g, g.Area.QuietMessage)
	// Same foe-killed triggers as the fought-win path, so Skip behaves identically.
	core.FireFoeKilledTriggers(g)
}

func loseBattle(g *core.GameState, message string) {
	g.Battle.Phase = core.BattleLost
	g.Battle.ClearTiming()
	resetBattleAction(g)
	setBattleMessageCat(g, message, core.LogDamageParty)
}

// fleeBattle is the debug "Easy Battle Quit" exit: drop the engaged pack and
// return to explore. No XP, no win/loss. Gated on g.EasyBattleQuit at the call site.
func fleeBattle(g *core.GameState) {
	dropPackAt(g, g.Battle.ActivePack)
	// Clear ActivePack BEFORE leaveBattle so clearBattleResidual's pack-defeated
	// drop can't re-remove whatever pack shifted into the now-stale slot.
	g.Battle.ActivePack = core.NoIndex
	leaveBattle(g, g.Area.QuietMessage)
}

func leaveBattle(g *core.GameState, message string) {
	clearBattleResidual(g)
	g.Battle.Phase = core.BattleNone
	if message != "" {
		setBattleMessage(g, message)
	}
	// Pending level-ups are no longer force-opened post-battle; the party-card "+"
	// indicator surfaces them and the player spends via the Tome when ready.
}

// clearBattleResidual zeroes every transient battle field. Used by leaveBattle
// and Update's defensive early-exit so a desynced encounter leaves no residue.
// Defeated packs are dropped here so the cleared slot doesn't ghost-render.
func clearBattleResidual(g *core.GameState) {
	// Release ingested members so the lockout doesn't bleed into the next encounter
	// (damageEnemy releases on the killing blow; this catches every other exit).
	core.ReleaseAllIngested(g.Party)
	// Clear combat-only statuses so they don't linger into exploration. Poison and
	// death intentionally persist — see ClearPartyTransientStatuses.
	core.ClearPartyTransientStatuses(g.Party)
	// Drop queued VFX intents and reset the render particle pool: formation-relative
	// particles captured camera-relative positions that, after exit, map to random
	// world locations — without the reset the player sees a ~1s ghost burst.
	g.VFXQueue = g.VFXQueue[:0]
	core.RequestVFXReset(g)
	// Drop the active pack when defeated (BattleWon, or Update's early-exit on a
	// desynced kill). LivingBattleCount==0 means wiped; on loss/flee enemies survive
	// and the pack stays.
	packDefeated := g.Battle.Phase == core.BattleWon || core.LivingBattleCount(g) == 0
	if packDefeated {
		dropPackAt(g, g.Battle.ActivePack)
	}
	g.Battle.ActivePack = core.NoIndex
	g.Battle.EnemyIndex = core.NoIndex
	// resetBattleTransients drops the queue, timing/charge state, clocks, pending
	// cast, and spoils snapshot so the next fight starts clean.
	resetBattleTransients(&g.Battle)
	resetBattleAction(g)
	// Drop held-turn auto-repeat carry: it isn't ticked during a battle, so a turn
	// key held through the fight would otherwise start the first post-battle turn
	// mid-cooldown instead of firing instantly (mirrors explore.ResetTurnRepeat).
	g.TurnHeldLast = false
	g.TurnRepeatCooldown = 0
}

func recoverFromLoss(g *core.GameState) {
	// ResetGameState requests the VFX reset itself (so the in-menu Restart path
	// gets it too), dropping the lost fight's particles.
	core.ResetGameState(g)
	// Recovery toast is a transient status, not a log event — setBattleStatus keeps
	// the fresh-run Log empty while surfacing the message on the field HUD.
	setBattleStatus(g, "You catch your breath.")
}

func resetBattleAction(g *core.GameState) {
	g.Battle.ActionMode = core.ActionMenu
	g.Battle.MenuIndex = 0
	g.Battle.PendingSkill = core.SkillNone
	g.Battle.PendingItem = core.ItemNone
	g.Battle.ItemMenuIndex = 0
}

// resetBattleTransients zeroes every per-fight transient. Both Start and
// clearBattleResidual call it so a new transient can't be cleared on one path and
// leaked on the other. EnemyIndex / ActivePack / Splash / PartyTarget are NOT
// touched — each caller sets those itself.
func resetBattleTransients(b *core.Battle) {
	b.ClearTiming()
	b.TimingIntro = 0
	b.ChargeNeedsRelease = false
	b.HitStop = 0
	b.ShakeTimer = 0
	b.RumbleTimer = 0
	b.SequencePulseTimer = 0
	b.SequencePulseIndex = -1
	b.LastQualityTimer = 0
	b.EnemyAttacker = core.NoIndex
	b.EnemyAttackCursor = core.NoIndex
	b.EnemyPendingSkill = core.SkillNone
	b.EnemyAttackMisses = false
	b.PhysDamageThisTurn = 0
	b.EnemyKillsThisTurn = 0
	b.MeteorFuse = 0
	b.MeteorDamage = 0
	b.Queue = nil
	b.QueueCursor = 0
	b.NextRoundQueue = nil
	b.Readiness = nil
	b.Spoils = core.VictorySpoils{}
	b.VictoryElapsed = 0
	b.VictoryLevelSfxCursor = 0
	b.VictoryLootSfxCursor = 0
	b.VictoryTickSfxCursor = 0
}

// setBattleStatus writes to the transient prompt slot. The renderer separates
// status from log via Message != Log[-1], so the order contract is: setBattleStatus
// for prompts/validation (before commit), setBattleMessage when the action lands.
// Don't call setBattleStatus AFTER setBattleMessage in the same frame.
func setBattleStatus(g *core.GameState, message string) {
	g.SetStatusMessage(message)
}

// setBattleMessage writes the status line AND appends to the combat log (non-empty,
// non-immediate-repeat). Dedupe is one-step-behind only: drops "X then X" but NOT
// "X then Y then X" so alternating-actor sequences read correctly.
func setBattleMessage(g *core.GameState, message string) {
	// LogMessage dedupes consecutive repeats + caps the buffer. Neutral (LogInfo) tint.
	g.LogMessage(message)
}

// setBattleMessageCat is setBattleMessage with an explicit color category (see
// core.LogCategory): damage/heal/death lines color-code; everything else stays LogInfo.
func setBattleMessageCat(g *core.GameState, message string, cat core.LogCategory) {
	g.LogMessageCat(message, cat)
}

// logFoeHit logs a party→foe damage line: gold when the blow felled the foe, white otherwise.
func logFoeHit(g *core.GameState, message string, defeated bool) {
	cat := core.LogDamageFoe
	if defeated {
		cat = core.LogDeath
	}
	setBattleMessageCat(g, message, cat)
}

// tickHitTimers decays the three per-actor hit-reaction timers (bump, flash,
// knockback) toward zero. Shared by the party and enemy decay loops.
func tickHitTimers(bump, flash, knockback *float32, dt float32) {
	*bump = core.ApproachZero(*bump, dt)
	*flash = core.ApproachZero(*flash, dt)
	*knockback = core.ApproachZero(*knockback, dt)
}

// enemySlideSlots backs UpdateEnemySlides' per-frame slot resolve (reused buffer,
// single-threaded update path).
var enemySlideSlots []core.EnemySlot

// updateBattleEffects decays the per-actor hit-reaction timers each frame.
// `members` is the caller-hoisted active-pack roster.
func updateBattleEffects(g *core.GameState, dt float32, members []core.Enemy) {
	for i := range g.Party {
		tickHitTimers(&g.Party[i].AttackBump, &g.Party[i].DamageFlash, &g.Party[i].HitKnockback, dt)
		g.Party[i].DamagePopupTimer = core.ApproachZero(g.Party[i].DamagePopupTimer, dt)
		g.Party[i].SwapSlide = core.ApproachZero(g.Party[i].SwapSlide, dt)
	}
	for i := range members {
		tickHitTimers(&members[i].AttackBump, &members[i].DamageFlash, &members[i].HitKnockback, dt)
		members[i].DeathFade = core.ApproachZero(members[i].DeathFade, dt)
		members[i].DamagePopupTimer = core.ApproachZero(members[i].DamagePopupTimer, dt)
	}
	// Arm/decay the foe formation glide AFTER the DeathFade decay above, so a corpse
	// that finishes fading this tick drops from the rank and the survivors slide to
	// close the gap (rather than snapping). Slots resolved once, shared with the draw.
	enemySlideSlots = core.ResolveEnemySlots(members, enemySlideSlots)
	core.UpdateEnemySlides(members, enemySlideSlots, dt)
	g.Battle.SequencePulseTimer = core.ApproachZero(g.Battle.SequencePulseTimer, dt)
	if g.Battle.SequencePulseTimer <= 0 {
		g.Battle.SequencePulseIndex = -1
	}
	// Decay the screen shake. Paused during hit-stop (Update early-returns), so a
	// high-grade hit reads as freeze → settle.
	g.Battle.ShakeTimer = core.ApproachZero(g.Battle.ShakeTimer, dt)
}

func battleDeathFadeActive(g *core.GameState) bool {
	// Index, not value-range: Enemy is a large struct and we only read one field.
	members := core.BattleMembers(g)
	for i := range members {
		if members[i].DeathFade > 0 {
			return true
		}
	}
	return false
}

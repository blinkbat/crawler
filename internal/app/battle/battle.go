package battle

import (
	"fmt"

	"crawler/internal/app/audio"
	"crawler/internal/app/core"
	"crawler/internal/app/input"
)

// Start engages the pack at packIndex. The entire pack roster becomes the
// in-battle enemy list; no spatial clustering is involved.
func Start(g *core.GameState, packIndex int) {
	if packIndex < 0 || packIndex >= len(g.Packs) || !core.PackAlive(g.Packs[packIndex]) {
		return
	}
	g.Battle.ActivePack = packIndex
	g.Battle.EnemyIndex = core.NextLivingBattleEnemy(g)
	g.Battle.PartyTarget = core.FirstLivingPartyMember(g.Party)
	g.Battle.EnemyAttackCursor = -1
	g.Battle.Splash = core.BattleSplashDuration
	// Preallocate to the cap so the 15-30+ setBattleMessage appends per
	// fight don't trigger slice growth. setBattleMessage trims to
	// BattleLogMaxLines on overflow, so cap is the steady-state ceiling.
	g.Battle.Log = make([]string, 0, core.BattleLogMaxLines)
	g.Battle.ClearTiming()
	g.Battle.TimingIntro = 0
	g.Battle.HitStop = 0
	g.Battle.ShakeTimer = 0
	g.Battle.RumbleTimer = 0
	g.Battle.SequencePulseTimer = 0
	g.Battle.SequencePulseIndex = -1
	g.Battle.EnemyAttacker = -1
	// Belt-and-suspenders with the other transients above: clear any pending
	// enemy cast so a Start that somehow followed an unclean exit can't make
	// the first enemy turn skip its defend bar and fire a phantom spell. Both
	// real entry paths (leaveBattle's clearBattleResidual, ResetGameState)
	// already clear it, but every sibling transient is reset here too.
	g.Battle.EnemyPendingSkill = core.SkillNone
	g.Battle.LastQualityTimer = 0
	g.Battle.Queue = nil
	g.Battle.QueueCursor = 0
	g.Battle.NextRoundQueue = nil
	g.Battle.Readiness = nil // fresh ATB gauges each battle
	resetBattleAction(g)
	setBattleMessage(g, core.BattleEncounterMessage(g))
	beginNewRound(g)
}

func Update(g *core.GameState, dt float32) {
	// dt is already clamped by explore.Update so a single hitched frame
	// can't fast-forward the timing bar past its window in one tick. This
	// clamp is what makes tickFlashHold safe: a huge dt could otherwise
	// drain BOTH the flash timer AND the hit-stop in a single frame, but
	// the worst the helper does is push onResolve back one frame — never
	// drops or duplicates it. See tickFlashHold for the two-phase shape.
	//
	// Hit-stop freeze: while HitStop > 0, the world is paused (sprite bumps,
	// popups, death fades, splash banner all hold). DamageFlash specifically
	// stays at peak strength through the freeze — that's intentional, since
	// the freeze IS the impact moment and the enemy reading "flashed white"
	// reinforces it. The phase handler still runs so HitStop counts down via
	// tickFlashHold; once it hits zero the apply step fires and normal
	// updates resume next frame.
	if g.Battle.HitStop > 0 {
		switch g.Battle.Phase {
		case core.BattleAttackTiming:
			updateAttackTiming(g, dt)
		case core.BattleEnemyTiming:
			updateEnemyTiming(g, dt)
		default:
			// HitStop is only ever set during a timing phase; if a future
			// phase sets it without a handler here, drain it rather than
			// freeze the battle forever waiting for a tick that never comes.
			g.Battle.HitStop = 0
		}
		return
	}
	updateBattleEffects(g, dt)
	tickQualityPopup(g, dt)
	// Defensive early-exit: a desynced EnemyIndex (points past the active pack
	// — e.g. a culled enemy) routes through leaveBattle so residual queue /
	// timing / attacker state doesn't linger across frames. Empty message so we
	// don't overwrite the quiet-area message with a stale combat status.
	members := core.BattleMembers(g)
	if g.Battle.EnemyIndex < 0 || g.Battle.EnemyIndex >= len(members) {
		leaveBattle(g, "")
		return
	}
	// All enemies down but the per-action win paths didn't fire (a state
	// desync). Route through winBattle, NOT leaveBattle — an all-enemies-dead
	// pack is a WIN, and leaveBattle would silently forfeit the encounter's
	// XP / gold / loot. winBattle sets Phase = BattleWon, so this guard can't
	// re-enter. Mirrors checkEnemyWipeout.
	if core.LivingBattleCount(g) == 0 && g.Battle.Phase != core.BattleWon {
		winBattle(g, core.LastBattleEnemyFallsMessage(g))
		return
	}
	if g.Battle.Phase != core.BattleWon && g.Battle.Phase != core.BattleLost && core.ActivePartyCount(g.Party) == 0 {
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
	case core.BattleAttackTiming:
		updateAttackTiming(g, dt)
	case core.BattleEnemyTiming:
		updateEnemyTiming(g, dt)
	case core.BattleWon:
		g.Battle.Timer -= dt
		if g.Battle.Timer <= 0 && !battleDeathFadeActive(g) {
			leaveBattle(g, g.Area.QuietMessage)
		}
	case core.BattleLost:
		if input.ConfirmPressed() {
			recoverFromLoss(g)
		}
	}
}

// --- Mixed-initiative scheduler --------------------------------------------

// checkEnemyWipeout fires the standard "last enemy down" win path if no
// enemies remain in the active pack. Returns true when it transitioned
// the battle; callers should `return` immediately to short-circuit the
// rest of their frame. Pulls the LivingBattleCount + winBattle +
// LastBattleEnemyFallsMessage triple into one seam so a future "victory
// fanfare delay" or "XP-award timing" change lands once. Specific
// flavor messages (e.g. "The fire finishes them." on a burn-kill) still
// inline `winBattle` directly — this helper is for the canonical case.
func checkEnemyWipeout(g *core.GameState) bool {
	if core.LivingBattleCount(g) == 0 {
		winBattle(g, core.LastBattleEnemyFallsMessage(g))
		return true
	}
	return false
}

// checkPartyWipeout fires the standard "no available party member" loss
// path. Available = alive AND not ingested, so a fully-swallowed party
// counts as a wipe even though their HP is preserved. Returns true when
// it transitioned the battle; callers should `return` immediately.
func checkPartyWipeout(g *core.GameState) bool {
	if core.ActivePartyCount(g.Party) == 0 {
		loseBattle(g, core.BattleLossMessage(g))
		return true
	}
	return false
}

// beginNewRound rebuilds the SPD-sorted turn queue and starts the first
// actor's turn. Burn ticks are NOT applied here — they fire per-actor when
// the burning character's turn comes up (see startActorTurn).
func beginNewRound(g *core.GameState) {
	if checkEnemyWipeout(g) {
		return
	}
	// Poison on ingested members ticks here instead of at end-of-turn —
	// they're skipped from the queue (buildTurnQueue), so a per-round
	// drain at the round boundary is the closest analog to the normal
	// end-of-turn cadence. Fire BEFORE the loss gate so a poison kill on
	// the last available member still routes through ActivePartyCount.
	tickPoisonForIngestedParty(g)
	if checkPartyWipeout(g) {
		return
	}
	// Reset the per-round round-robin cursor for enemy attack targets so a
	// fresh round starts attacking from party slot 0. PartyTarget is the
	// player's heal/item ally selection and stays untouched here.
	g.Battle.EnemyAttackCursor = -1
	g.Battle.Queue = buildTurnQueue(g)
	g.Battle.QueueCursor = 0
	// Pre-bake the projection of the round AFTER this one so TurnForecast
	// doesn't have to re-sort every frame. Uses the non-persisting variant
	// so it doesn't consume the readiness the real next round needs.
	g.Battle.NextRoundQueue = projectNextRoundQueue(g)
	startActorTurn(g)
}

// buildTurnQueue assembles the next batch of actor turns using an
// ATB-style tick scheduler. Each alive actor has a readiness gauge
// that accumulates by their SPD per tick; whoever crosses
// ATBReadyThreshold first acts, then carries the overflow into the
// next tick. The queue is naturally interleaved by who hits the gate
// when — there's no rigid "round" of one slot per actor. A SPD 6
// Thief reaches readiness roughly twice as often as a SPD 3 Warrior,
// so the queue looks like [Thief, Goblin, Cleric, Thief, Wizard,
// Warrior, ...] instead of clustered SPD-descending slots.
//
// Each actor's gauge SEEDS from g.Battle.Readiness (carried across
// rounds) rather than resetting to 0, so a faster actor's leftover
// surplus accumulates into EXTRA turns over time — SPD drives turn rate,
// not merely order. The sim still stops once every alive actor has acted
// at least once (the "round" boundary downstream code — poison-on-
// ingested ticks, enemy-attack cursor reset — hangs housekeeping off);
// the persisted surplus is what lets a small/steady SPD lead eventually
// convert to a bonus turn instead of being discarded at the round edge.
// Ingested party members are skipped entirely; they re-enter the queue
// the call AFTER their swallower dies.
func buildTurnQueue(g *core.GameState) []core.ActorRef {
	return simulateTurnQueue(g, true)
}

// projectNextRoundQueue previews the round AFTER the current one WITHOUT
// advancing the persisted readiness — the real next round simulates from
// the same gauge state and yields this same queue, so the turn-forecast
// HUD stays accurate under the carry-over model.
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
		if p.HP <= 0 || p.Ingested {
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
	// Only actors with positive SPD ever reach the ATB gate; counting
	// SPD-0 actors toward `target` would prevent `actedCount` from
	// reaching it (the picker skips them via the spd<=0 branch below),
	// and the loop would only escape via the maxSlots fallback every
	// round — inflating the queue to 4× the participant count. Filter
	// them out of the target count up front. An author who sets SPD=0
	// in the editor is explicitly opting that actor out of the queue.
	target := 0
	for _, a := range actors {
		if a.spd > 0 {
			target++
		}
	}
	if target == 0 {
		// Degenerate encounter: every living actor has SPD <= 0 (only
		// reachable via hand-authored custom-enemy stats — built-ins are
		// all SPD > 0). Returning nil would leave startActorTurn ->
		// beginNewRound rebuilding an empty queue forever (neither side
		// is wiped), recursing to a stack overflow. Give each living
		// actor one slot in declaration order so the round still resolves.
		fallback := make([]core.ActorRef, 0, len(actors))
		for _, a := range actors {
			fallback = append(fallback, a.ref)
		}
		return fallback
	}
	queue := make([]core.ActorRef, 0, target*2)
	actedCount := 0
	threshold := core.ATBReadyThreshold
	// Hard cap: even pathological mixes (one tick-fast actor while
	// everyone else has SPD 1) shouldn't blow past the configured
	// multiple of participant count per round.
	maxSlots := target * core.ATBQueueSlotMultiplier
	for actedCount < target {
		// maxSlots caps SURPLUS turns (a fast actor acting many times before
		// the slow ones), but must never starve a not-yet-acted actor of its
		// guaranteed single turn. Once the queue hits the cap, keep advancing
		// the clock yet only let actors that haven't acted this round be
		// picked — otherwise an extreme SPD spread (e.g. 30 vs 1) lets the
		// fast actor fill every slot before the slow actor crosses the gate,
		// skipping its turn and the round-boundary housekeeping that hangs off
		// "every alive actor acted at least once."
		capped := len(queue) >= maxSlots
		// Pick the actor that reaches `threshold` in the fewest ticks.
		// ticksNeeded = ceil((threshold - ready) / spd), compared via
		// cross-multiplication to keep everything integer.
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
			li := (threshold - actors[i].ready) * actors[bestIdx].spd
			lb := (threshold - actors[bestIdx].ready) * actors[i].spd
			if li < lb {
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
		// Carry each actor's leftover gauge into the next round. Rebuilt
		// fresh from the current actor set, so entries for actors who died
		// this round are pruned automatically.
		next := make(map[core.ActorRef]int, len(actors))
		for _, a := range actors {
			next[a.ref] = a.ready
		}
		g.Battle.Readiness = next
	}
	return queue
}

// actorSpeed returns the SPD of the actor. Both sides read Stats.SPD
// now that enemies carry a full Stats block (legacy EnemyDefinition.Speed
// was retired in favour of symmetry). Webbed party members have their
// effective SPD halved (rounded down, floor 1) for the turn-queue
// sort — Webbed's signature is "you still act, but the world gets ahead
// of you." Enemy-side Webbed is not currently inflicted (no player
// skill applies it yet), so the webbed branch is party-only today.
func actorSpeed(g *core.GameState, actor core.ActorRef) int {
	if actor.IsParty {
		if !actor.ValidPartyIndex(g.Party) {
			return 0
		}
		spd := core.EffectiveStats(g.Party[actor.Index]).SPD
		if g.Party[actor.Index].WebbedTurns > 0 {
			spd /= 2
			if spd < 1 {
				spd = 1
			}
		}
		return spd
	}
	m := core.BattleMemberAt(g, actor.Index)
	if m == nil {
		return 0
	}
	// Effective SPD folds any active debuff (the Thief's Cripple). Floor at 1 so
	// a heavy SPD debuff can't drive the gauge to 0 — at 0 the enemy would never
	// cross the ATB threshold, never take a turn, and so never tick the debuff
	// down (it drains at end-of-turn): a permanent self-sustaining lockout. A
	// crippled foe still acts, just rarely.
	spd := core.EffectiveEnemyStats(*m).SPD
	if spd < 1 {
		spd = 1
	}
	return spd
}

// actorAppearsBefore reports whether `ref` occupies any queue slot strictly
// before `cursor`. ActorRef is a comparable {IsParty, Index} value, so this is
// an exact identity match. Used by the skip loop to tick an ingested member's
// Poison only on their FIRST slot in the round (the ATB queue can hold the
// same fast actor several times — see simulateTurnQueue).
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

// startActorTurn opens the turn of whatever actor sits at the queue cursor.
// Order each turn: skip-if-dead → burn tick (may kill) → input action.
// If the queue runs off the end, a fresh round starts (which may end the
// battle if everyone on a side has died).
func startActorTurn(g *core.GameState) {
	for g.Battle.QueueCursor < len(g.Battle.Queue) {
		skipped := g.Battle.Queue[g.Battle.QueueCursor]
		if isActorAlive(g, skipped) {
			break
		}
		// A party member queued this round but ingested before their turn
		// still owes their end-of-turn Poison tick — "Poison survives the
		// lockout". Without this the DoT pauses for the swallow round and only
		// resumes via tickPoisonForIngestedParty next round. The helper no-ops
		// on dead members (HP<=0) and enemies, so it fires only for an alive,
		// ingested, poisoned member. buildTurnQueue excludes members ingested
		// at round start, so the queue can only hold a member ingested
		// mid-round.
		//
		// Tick ONLY on the member's FIRST queue slot this round: under ATB
		// carry-over a high-SPD member can hold several slots, and any earlier
		// slot already accounted for one tick (a real turn before the ingest,
		// via finishActorTurn, or this same first-slot guard). Without the
		// guard a fast, mid-round-ingested member takes one poison tick per
		// slot — double damage and double duration drain in a single round.
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

	// Burn ticks at the start of the burning actor's own turn. If the burn
	// kills them, skip their input action (and check win conditions in case
	// the burn took out the last enemy).
	if killed := tickBurnAtTurnStart(g, actor); killed {
		if core.LivingBattleCount(g) == 0 {
			winBattle(g, "The fire finishes them.")
			return
		}
		g.Battle.QueueCursor++
		startActorTurn(g)
		return
	}

	// Sleep / Stun skip — both statuses cost the actor their turn,
	// both tick at the start of their own turn, both fall through the
	// same advance-and-restart path. Sleep clears on damage (handled
	// in the damage helpers); Stun doesn't — the lockout runs its
	// rolled duration regardless of damage in between. Sleep ticks
	// first so a target that's both sleeping and stunned reads as
	// "asleep" in the log.
	if asleep := tickSleepAtTurnStart(g, actor); asleep {
		advanceSkippedTurn(g, actor)
		return
	}
	if stunned := tickStunAtTurnStart(g, actor); stunned {
		advanceSkippedTurn(g, actor)
		return
	}

	if actor.IsParty {
		beginPartyTurn(g, actor.Index)
	} else {
		beginEnemyAttack(g, actor.Index)
	}
}

// advanceSkippedTurn closes out a turn that Sleep or Stun cost the actor.
// A skipped turn still ELAPSES the actor's per-turn effects, exactly as a
// normal turn (finishActorTurn) and the ingest lockout do: it clears the
// one-round Defend brace, drains the non-damaging party statuses
// (Webbed/Confused), AND ticks Poison. The poison tick is the bug fix — it
// was previously omitted here, so a poisoned actor who was repeatedly
// slept/stunned had its DoT frozen (zero damage, no duration decrement) for
// the whole lockout, unlike every other status. The tick can kill, so win
// conditions are checked before advancing — mirroring finishActorTurn and
// the burn-at-turn-start path.
func advanceSkippedTurn(g *core.GameState, actor core.ActorRef) {
	consumeDefendOnSkip(g, actor)
	drainNonDamagingPartyStatuses(g, actor)
	drainNonDamagingEnemyStatuses(g, actor)
	tickPoisonAfterPartyTurn(g, actor)
	tickPoisonAfterEnemyTurn(g, actor)
	if checkEnemyWipeout(g) || checkPartyWipeout(g) {
		return
	}
	g.Battle.QueueCursor++
	startActorTurn(g)
}

// consumeDefendOnSkip clears a party member's Defend brace when their
// turn is skipped by Sleep/Stun. beginPartyTurn clears it on a normal
// turn; without this a member who chose Defend and is then put to sleep
// keeps the damage reduction every round they're out, instead of the
// single round Defend is meant to last. Enemies don't use Defending.
func consumeDefendOnSkip(g *core.GameState, actor core.ActorRef) {
	if actor.IsParty && actor.Index >= 0 && actor.Index < len(g.Party) {
		g.Party[actor.Index].Defending = false
	}
}

// drainNonDamagingPartyStatuses ticks the party-side counters that carry
// no damage (Webbed, Confused) for the actor whose turn just ended.
// finishActorTurn calls it on a normal turn; the Sleep/Stun skip path in
// startActorTurn calls it too — without that, a member who is webbed/
// confused AND asleep/stunned never has those counters decrement (their
// turn is skipped, so finishActorTurn never runs), and the slow / retarget
// effect outlasts its rolled duration. No-ops on enemies and on members
// with the counter already at zero.
func drainNonDamagingPartyStatuses(g *core.GameState, actor core.ActorRef) {
	tickWebbedAfterPartyTurn(g, actor)
	tickConfusedAfterPartyTurn(g, actor)
	tickBlessAfterPartyTurn(g, actor)
	tickRegenAfterPartyTurn(g, actor)
}

// drainNonDamagingEnemyStatuses ticks the enemy-side non-damaging counters
// (buff/debuff turns) for the actor whose turn just ended — the enemy mirror of
// drainNonDamagingPartyStatuses. Called at the same end-of-turn seams (normal
// turn end + the Sleep/Stun skip path) so a debuff elapses even on a skipped
// turn. No-ops on party actors and on enemies with no active buff; a future
// enemy self-buff / regen plugs in here.
func drainNonDamagingEnemyStatuses(g *core.GameState, actor core.ActorRef) {
	tickEnemyBuffAfterTurn(g, actor)
}

// tickSkipStatusAtTurnStart drains one tick from a skip-this-turn status
// counter (Sleep / Stun) at the start of the afflicted actor's own
// turn, emitting a "(Name) <verb>." log line and returning true if the
// actor must skip their action. Shared body for tickSleepAtTurnStart /
// tickStunAtTurnStart — they used to be byte-for-byte copies that
// differed only in the field accessor and the verb.
//
// counterRefParty / counterRefEnemy return pointers into the affected
// member's counter so the helper can both read and decrement without a
// type-specific switch. (Sleep has a separate wake-on-damage path
// inside the damage helpers — counter draining here is independent of
// that.)
func tickSkipStatusAtTurnStart(
	g *core.GameState, actor core.ActorRef, verb string,
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
		setBattleMessage(g, fmt.Sprintf("%s %s.", m.Name, verb))
		return true
	}
	enemy := core.BattleMemberAt(g, actor.Index)
	if enemy == nil || !enemy.Alive {
		return false
	}
	c := counterRefEnemy(enemy)
	if *c <= 0 {
		return false
	}
	*c--
	setBattleMessage(g, fmt.Sprintf("%s %s.", core.TheEnemy(core.EnemyInfoFor(*enemy)), verb))
	return true
}

func tickSleepAtTurnStart(g *core.GameState, actor core.ActorRef) bool {
	return tickSkipStatusAtTurnStart(g, actor, "is asleep",
		func(m *core.PartyMember) *int { return &m.SleepTurns },
		func(e *core.Enemy) *int { return &e.SleepTurns })
}

// Players can't currently be stunned (no enemy skill inflicts it yet),
// but the helper's party branch keeps the symmetry future-proof.
func tickStunAtTurnStart(g *core.GameState, actor core.ActorRef) bool {
	return tickSkipStatusAtTurnStart(g, actor, "is stunned",
		func(m *core.PartyMember) *int { return &m.StunTurns },
		func(e *core.Enemy) *int { return &e.StunTurns })
}

// isActorAlive answers "is this queue actor still in the fight?" Used for
// skipping dead entries in the round queue. For party slots this means
// "alive AND not currently ingested" — a member swallowed mid-round
// must skip their queued turn until the mantrap dies.
func isActorAlive(g *core.GameState, actor core.ActorRef) bool {
	if actor.IsParty {
		return core.PartyMemberAvailable(g.Party, actor.Index)
	}
	return core.BattleEnemyAlive(g, actor.Index)
}

// repointEnemyCursorIfDead moves g.Battle.EnemyIndex onto the next living
// enemy when the currently-pointed one has died, leaving it put if none
// remain. Shared by finishActorTurn and beginPartyTurn so the "don't leave the
// target cursor on a corpse" rule lives in one place.
func repointEnemyCursorIfDead(g *core.GameState) {
	if !core.BattleEnemyAlive(g, g.Battle.EnemyIndex) {
		if next := core.NextLivingBattleEnemy(g); next >= 0 {
			g.Battle.EnemyIndex = next
		}
	}
}

// finishActorTurn is the single hand-off used by every action's apply* path.
// Checks win/lose, fixes up EnemyIndex if the target died, advances the
// queue cursor, and starts the next actor's turn.
//
// Poison tick runs HERE for party actors — right after their action lands,
// before win/lose checks so a poison kill is honored if it drops the last
// living member.
func finishActorTurn(g *core.GameState) {
	g.Battle.ClearTiming()
	// Canonical clear of ChargeNeedsRelease: any action that finished
	// (player or enemy) flows through here, so this is the seam that
	// guarantees the next turn starts with a fresh release-gate state.
	// clearBattleResidual also zeros the field as a belt-and-suspenders
	// catch for early-exit battle leaves; the inline clear inside
	// updateAttackTiming is the runtime "key was released" path.
	g.Battle.ChargeNeedsRelease = false
	// HitStop is normally drained to zero by tickFlashHold before
	// applyPendingAction fires, but clear it defensively here so an early-
	// exit apply path (e.g. "No target." after the target died between
	// confirm and apply) can't leak a stuck freeze across the next phase.
	g.Battle.HitStop = 0
	if g.Battle.QueueCursor >= 0 && g.Battle.QueueCursor < len(g.Battle.Queue) {
		// Party side ticks party poison; enemy side ticks enemy poison.
		// Both run at end-of-actor-turn so the actor still gets their
		// action in before the DoT lands. Each helper short-circuits
		// on the wrong actor kind so dispatching is fine here.
		tickPoisonAfterPartyTurn(g, g.Battle.Queue[g.Battle.QueueCursor])
		tickPoisonAfterEnemyTurn(g, g.Battle.Queue[g.Battle.QueueCursor])
		drainNonDamagingEnemyStatuses(g, g.Battle.Queue[g.Battle.QueueCursor])
		// Webbed + Confused tick alongside Poison — every party-side
		// status counter ticks at the END of the webbed/confused
		// member's own turn so they get one full action under the
		// status before the counter decrements. Mirrors the Poison
		// shape; same actor-kind dispatch. (The Sleep/Stun skip path
		// drains these too, so a skipped turn still elapses the duration.)
		drainNonDamagingPartyStatuses(g, g.Battle.Queue[g.Battle.QueueCursor])
	}
	if checkEnemyWipeout(g) {
		return
	}
	if checkPartyWipeout(g) {
		return
	}
	repointEnemyCursorIfDead(g)
	g.Battle.QueueCursor++
	startActorTurn(g)
}

// --- Party turn ------------------------------------------------------------

// beginPartyTurn opens the action menu for the given party member. Owns
// Phase=BattlePlayer plus Defending-flag reset (defending lasts exactly one
// round-trip — through the rest of the round and back).
func beginPartyTurn(g *core.GameState, partyIndex int) {
	g.Battle.Phase = core.BattlePlayer
	g.Battle.ClearTiming()
	g.Battle.TimingIntro = 0
	g.Battle.EnemyAttacker = -1
	g.Battle.CurrentParty = partyIndex
	resetBattleAction(g)
	if partyIndex >= 0 && partyIndex < len(g.Party) {
		g.Party[partyIndex].Defending = false
		g.Battle.PartyTarget = partyIndex
	} else {
		// Reached only on an out-of-bounds partyIndex — the queue has handed us
		// a bogus actor. Fall back to the first living member; if there's no
		// living member to fall back to, route to BattleLost rather than
		// silently clamping PartyTarget to 0 and opening the menu for a
		// corpse.
		g.Battle.PartyTarget = core.FirstLivingPartyMember(g.Party)
		if g.Battle.PartyTarget < 0 {
			loseBattle(g, core.BattleLossMessage(g))
			return
		}
	}
	repointEnemyCursorIfDead(g)
}

func updatePlayerBattle(g *core.GameState) {
	// If the current actor died between turns (e.g. an enemy went first this
	// round and killed them), the queue would normally advance past us — but
	// as a defensive net here we skip ahead.
	if !core.PartyMemberAvailable(g.Party, g.Battle.CurrentParty) {
		g.Battle.QueueCursor++
		startActorTurn(g)
		return
	}

	switch g.Battle.ActionMode {
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
	default:
		updateActionMenu(g)
	}
}

// tickFlashHold drains the post-press flash timer and then, on high grades,
// the hit-stop freeze that chains after it. Returns true while either
// timer is still on screen (caller bails this frame); when both have
// fully drained, fires onResolve and still returns true so the caller
// doesn't double-tick the underlying bar in the same frame.
//
// Phase order on a Great/Excellent press:
//
//  1. TimingFlash counts down (bar pulses with quality color, cursor frozen)
//  2. HitStop counts down (everything in the world freezes — see Update)
//  3. onResolve fires (damage / heal applies, popup spawns)
//
// On a Miss / Nice / Good, phase 2 is skipped — HitStopFor returns 0 and
// onResolve fires the moment the flash hits zero.
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
		// Great/Excellent: freeze first (anticipation), then fireImpact resolves
		// + shakes once the freeze releases — see fireImpact for why the shake
		// no longer arms here (it used to read as preceding the hit).
		g.Battle.HitStop = stop
		return true
	}
	fireImpact(g, onResolve)
	return true
}

// fireImpact runs the action's apply (onResolve: damage / heal / per-skill VFX /
// hit-glyph / popups) and THEN arms the base combat shake — so the shake lands
// WITH the impact (after the hit-stop freeze) instead of during it. It used to
// arm at flash-end, before both the freeze and onResolve, which made the screen
// shake DURING the anticipation freeze — reading as the shake preceding the
// attack. Now: flash → freeze (still) → fireImpact (damage + VFX + shake all at
// the connect). CombatShakeFor returns 0 for Miss/Nice/Good (no base shake), and
// a stronger crit/AoE shake armed inside onResolve survives the base arm here
// via TriggerCombatShake's keep-the-stronger rule.
func fireImpact(g *core.GameState, onResolve func()) {
	onResolve()
	basePeak, baseDur := core.CombatShakeFor(g.Battle.Timing.Quality)
	core.TriggerCombatShake(&g.Battle, basePeak, baseDur)
}

// updateAttackTiming drives the player's attack/skill timing bar. The bar
// resolves either when the player provides input (press for press-kind bars,
// release for charge-kind bars) or when the cursor runs off the end. An
// engaged resolution triggers a brief flash hold so the quality reads before
// the action lands; an auto-miss applies immediately so the turn doesn't
// dwell on a non-event.
// driveSequenceInput reads one directional tap into the active sequence/recall
// bar and fires the per-slot pulse on a correct land. Shared by the Sequence
// and Recall timing kinds (Recall gates the call on its reveal phase) so the
// four-arrow dispatch + pulse bookkeeping lives in one place.
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
	// Light up the pulse for the just-landed slot — only on Correct, so
	// a wrong tap still draws the red result but doesn't bounce.
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
	// Charge-bar release gate: while the confirm key carried over from
	// menu confirmation is still held, suppress every engage path
	// (intro-skip, Hold, Release). The intro counter still ticks so
	// the 3s auto-arm isn't blocked, but a player who's stuck holding
	// from menu-confirm can't accidentally drive the bar. Cleared the
	// frame after AttackTimingHeld goes false.
	if g.Battle.ChargeNeedsRelease && !input.AttackTimingHeld() {
		g.Battle.ChargeNeedsRelease = false
	}
	engageReady := !g.Battle.ChargeNeedsRelease
	if g.Battle.TimingIntro > 0 {
		// Charge bars skip the intro on a FRESH edge press AFTER the
		// release gate has cleared. Without the edge-only rule, the
		// same Enter the player used to confirm the target would bleed
		// into the next frame's input check and instantly engage the
		// charge — bar cursor would advance, player's natural release
		// of the confirm key would resolve the bar at quality=Miss.
		isCharge := g.Battle.Timing.Kind == core.TimingKindCharge || g.Battle.Timing.Kind == core.TimingKindOvercharge
		if isCharge && engageReady && input.AttackTimingPressed() {
			g.Battle.TimingIntro = 0
		} else {
			g.Battle.TimingIntro -= dt
			return
		}
	}

	// Each arm only DRIVES input for its kind; the shared "advance the bar if
	// the input didn't already resolve it" tick is hoisted out below so it
	// isn't copy-pasted into every case.
	switch g.Battle.Timing.Kind {
	case core.TimingKindCharge, core.TimingKindOvercharge:
		// Hold/Release also gated by engageReady. A player still
		// holding the confirm key at the 3s auto-arm doesn't engage
		// the bar — cursor ticks past Peak to Miss naturally. They
		// must release-then-press to land a quality. Overcharge shares
		// the exact flow; only its resolve (overload band) differs.
		if engageReady && !g.Battle.Timing.Resolved && input.AttackTimingHeld() {
			g.Battle.Timing.Hold()
		}
		if engageReady && !g.Battle.Timing.Resolved && input.AttackTimingReleased() {
			g.Battle.Timing.Release()
		}
	case core.TimingKindSequence:
		driveSequenceInput(g)
	case core.TimingKindRecall:
		// Memory bar: directional taps are ignored during the reveal phase
		// (the player is memorizing); once the pattern hides, input drives
		// the same per-slot grading as the sequence bar.
		if g.Battle.Timing.RecallHidden() {
			driveSequenceInput(g)
		}
	case core.TimingKindReels:
		// Slot gamble: each press stops the next spinning reel.
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

	if !g.Battle.Timing.Resolved {
		return
	}
	if g.Battle.Timing.Pressed {
		g.Battle.TimingFlash = core.TimingFlashDuration
		audio.Play(soundForGrade(g.Battle.Timing.Quality))
		return
	}
	audio.Play(audio.SoundInputMiss)
	applyPendingAction(g, g.Battle.Timing.Quality)
}

// gradeSounds is the per-grade audio cue table. Battle-side equivalent of
// render's qualityVisuals — kept in this package because audio doesn't
// import core. Press, charge, and sequence bars all dispatch off this one
// table so they sound the same on the same grade.
var gradeSounds = [...]audio.Sound{
	core.TimingQualityMiss:      audio.SoundInputMiss,
	core.TimingQualityNice:      audio.SoundInputHit,
	core.TimingQualityGood:      audio.SoundInputHit,
	core.TimingQualityGreat:     audio.SoundInputGreat,
	core.TimingQualityExcellent: audio.SoundInputGreat,
}

// init asserts gradeSounds covers every timing grade. Pairs with the
// equivalent inits on core's timingGrades and render's qualityVisuals
// so all three parallel tables track TimingQualityCount.
func init() {
	if len(gradeSounds) != int(core.TimingQualityCount) {
		panic("battle: gradeSounds length must match core.TimingQualityCount")
	}
}

// soundForGrade picks the input cue for a freshly resolved timing grade.
// Out-of-range qualities fall back to the Miss cue.
func soundForGrade(q int) audio.Sound {
	if q < 0 || q >= len(gradeSounds) {
		return gradeSounds[core.TimingQualityMiss]
	}
	return gradeSounds[q]
}

// --- Enemy turn ------------------------------------------------------------

// beginEnemyAttack arms the defend bar against the queue-slot enemy. The
// slot is an index into the active pack's Members.
func beginEnemyAttack(g *core.GameState, slot int) {
	g.Battle.EnemyAttacker = slot
	g.Battle.TimingFlash = 0
	g.Battle.TimingIntro = core.EnemyTurnIntro
	g.Battle.Phase = core.BattleEnemyTiming
	// Pick a skill for this turn — spellcasting enemies (goblin mage)
	// may cast Firebolt / Sleep instead of plain melee. enemyAIPickSkill
	// returns SkillNone for non-caster enemies; the field path runs the
	// existing defend-timing flow against that bite. Spell casts skip
	// the defend bar entirely (the player can't "block" Firebolt with
	// timing yet — keeping the UX scope tight).
	enemy := core.BattleMemberAt(g, slot)
	g.Battle.EnemyPendingSkill = core.SkillNone
	if enemy != nil {
		g.Battle.EnemyPendingSkill = enemyAIPickSkill(g, *enemy, slot)
	}
	if g.Battle.EnemyPendingSkill != core.SkillNone {
		// Mark the Timing as already resolved so the defend bar never
		// arms — the spell-cast intro elapses and resolveAndFinish
		// routes through resolveEnemySpell.
		g.Battle.Timing = core.TimingState{Resolved: true}
		return
	}
	g.Battle.Timing = core.NewTimingState(g.Rand(), core.DefendTimingDuration)
}

// enemyAIPickSkill picks a skill for the enemy's current turn — or
// SkillNone meaning "default to plain melee." Reads EnemyDefinition.
// Skills and rolls against SkillCastChance: with probability p the
// enemy picks uniformly from its skills, otherwise it melees. p == 0
// (default) means "never casts" — non-caster enemies leave the field
// blank and short-circuit out of this function.
//
// `slot` is the active-pack index of the casting enemy; used to filter
// out skills that can't fire from this specific instance (Ingest by a
// mantrap that already has prey, etc.). The filter happens BEFORE the
// SkillCastChance roll so a mantrap that can't ingest doesn't waste its
// cast slot on a dead-end skill — it bites instead.
func enemyAIPickSkill(g *core.GameState, enemy core.Enemy, slot int) core.SkillID {
	def := core.EnemyInfoFor(enemy)
	if len(def.Skills) == 0 || def.SkillCastChance <= 0 {
		return core.SkillNone
	}
	// Filter-before-roll order is intentional: an empty usable list
	// short-circuits to melee WITHOUT consuming a "cast" outcome, but
	// a non-empty usable list rolls SkillCastChance normally. A mantrap
	// that's already digesting prey filters Ingest out → usable is empty
	// → bites. A mantrap with prey AND a second skill in the list (future
	// caster expansion) would keep the second skill in play. Reversing
	// the order would mean a no-prey-target mantrap "rolls a cast" and
	// then has nothing to cast — wasting the chance check and feeling
	// like the AI flinched.
	usable := usableEnemySkills(g, def.Skills, slot)
	if len(usable) == 0 {
		return core.SkillNone
	}
	if g.Rand().Float64() >= def.SkillCastChance {
		return core.SkillNone
	}
	return usable[g.Rand().Intn(len(usable))]
}

// usableEnemySkills filters an enemy's authored Skills list down to the
// ones whose preconditions are satisfied right now — keyed off the
// casting slot so per-instance state (e.g. "this mantrap already holds
// prey") gates correctly. Skills without per-instance gates (Firebolt,
// Sleep) always pass through.
func usableEnemySkills(g *core.GameState, skills []core.SkillID, slot int) []core.SkillID {
	enemy := core.BattleMemberAt(g, slot)
	out := make([]core.SkillID, 0, len(skills))
	for _, s := range skills {
		// Per-battle cast limit gate: drop the skill once the caster
		// has used it PerBattleCastLimit times. Reads through the
		// registry rather than hardcoding the SkillID so future
		// capped skills (boss ultimates) plug in for free. A nil
		// SkillCastCount map reads as zero on lookup, so uncapped
		// callers don't pay the lazy-init cost.
		if limit := core.SkillCastLimitFor(s); limit > 0 && enemy != nil {
			if enemy.SkillCastCount[s] >= limit {
				continue
			}
		}
		switch s {
		case core.SkillIngest:
			// Mantrap can't ingest while already holding someone, and
			// it can't ingest if no party member is available to be
			// targeted (everyone is dead or already in another mantrap).
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

// updateEnemyTiming drives the player's defend bar against the currently
// attacking enemy. A press triggers a brief flash hold so the block reads;
// an auto-miss (no press) resolves immediately. After resolution, advance
// the round queue.
func updateEnemyTiming(g *core.GameState, dt float32) {
	if tickFlashHold(g, dt, func() { resolveAndFinishEnemyAttack(g) }) {
		return
	}
	if g.Battle.TimingIntro > 0 {
		g.Battle.TimingIntro -= dt
		return
	}
	// Spell-cast path: no defend bar arms because beginEnemyAttack
	// pre-resolved the Timing. Once the intro elapses, route directly
	// to resolveAndFinishEnemyAttack — the spell apply happens there.
	if g.Battle.EnemyPendingSkill != core.SkillNone {
		resolveAndFinishEnemyAttack(g)
		return
	}
	if !g.Battle.Timing.Resolved && input.DefendTimingPressed() {
		g.Battle.Timing.Press()
	}
	if !g.Battle.Timing.Resolved {
		g.Battle.Timing.Tick(dt)
	}
	if !g.Battle.Timing.Resolved {
		return
	}
	if g.Battle.Timing.Pressed {
		g.Battle.TimingFlash = core.TimingFlashDuration
		audio.Play(soundForGrade(g.Battle.Timing.Quality))
		return
	}
	audio.Play(audio.SoundInputMiss)
	resolveAndFinishEnemyAttack(g)
}

// resolveAndFinishEnemyAttack applies the current attacker's hit (scaled by
// the resolved defend quality) and advances the round queue. Branches on
// EnemyPendingSkill — SkillNone for plain melee, anything else routes
// through resolveEnemySpell with the picked cast.
func resolveAndFinishEnemyAttack(g *core.GameState) {
	if g.Battle.EnemyPendingSkill != core.SkillNone {
		resolveEnemySpell(g, g.Battle.EnemyAttacker, g.Battle.EnemyPendingSkill)
	} else {
		resolveEnemyAttacker(g, g.Battle.EnemyAttacker, g.Battle.Timing.Quality)
	}
	g.Battle.ClearTiming()
	g.Battle.EnemyAttacker = -1
	g.Battle.EnemyPendingSkill = core.SkillNone
	finishActorTurn(g)
}

// resolveEnemySpell is the apply path for enemy-cast skills (goblin mage
// Firebolt / Sleep, mantrap Ingest). Resolves the common context
// (caster, target, definition, skill name, effect) and dispatches to
// the per-skill handler registered in enemySpellHandlers. The init
// guard in actions.go asserts every EnemyCastable skill has a handler
// here and vice-versa, so the dispatch can't silently fizzle.
func resolveEnemySpell(g *core.GameState, slot int, skill core.SkillID) {
	enemy := core.BattleMemberAt(g, slot)
	if enemy == nil || !enemy.Alive {
		return
	}
	effect := core.SkillEffectFor(skill)
	// Skills that don't need a single party target (AoE phys like
	// Stoneslam, summons like Raise Bones) MUST bypass the
	// pickEnemyAttackTarget gate — otherwise an encounter where
	// every party member is ingested by mantraps would deadlock
	// (no living-and-available target → return → no cast → the
	// necromancer can never summon to break the stalemate). For
	// single-target casts (Firebolt, Sleep, Ingest, Web, Confuse)
	// the gate stands: those genuinely have nothing to do without
	// a target.
	target := -1
	if !effect.AppliesAOEParty && !effect.AppliesSummonSkeleton {
		target = pickEnemyAttackTarget(g)
		if target < 0 {
			// No living, non-ingested party target for a single-target cast
			// (e.g. the last reachable ally just got swallowed mid-round).
			// pickEnemyAttackTarget already advanced the round-robin cursor,
			// so the queue stays consistent — but don't let the enemy's whole
			// turn elapse with a blank combat log; surface the no-op so the
			// forecast advancing isn't mistaken for a frozen battle.
			setBattleMessage(g, fmt.Sprintf("%s hesitates.", core.TheEnemy(core.EnemyInfoFor(*enemy))))
			return
		}
	}
	enemy.AttackBump = core.BumpDuration
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
		// Unreachable: init guard panics on missing handlers. Keep
		// the log line as defense-in-depth so a future hot-reload /
		// runtime registration path doesn't go silent.
		setBattleMessage(g, fmt.Sprintf("%s mutters something (unhandled skill %d).", core.TheEnemy(def), int(skill)))
		return
	}
	handler(ctx)
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
	g.Battle.Phase = core.BattleWon
	g.Battle.Timer = core.VictoryDanceDuration
	g.Battle.ClearTiming()
	resetBattleAction(g)
	setBattleMessage(g, message)
	// Credit every felled foe to the bestiary (kill counts drive the
	// 5-kills-to-identify threshold). Reads alongside the XP / loot
	// awards below — all "the pack is dead, tally the spoils" bookkeeping.
	core.RecordBattleKills(g)
	// XP award fires once, right after the kill is confirmed. Living
	// members get the full pack value; dead members get nothing
	// (incentive to keep your tank up). Level-ups queue stat points
	// onto PendingLevelUps; the level-up modal opens on leaveBattle
	// when those points are still unspent.
	perMember, leveled := core.AwardBattleXP(g)
	if perMember > 0 {
		// The kill line was already logged above; don't re-embed `message`
		// here or it prints twice in the combat log.
		setBattleMessage(g, fmt.Sprintf("Party gains %d XP each.", perMember))
		for _, idx := range leveled {
			setBattleMessage(g, fmt.Sprintf("%s reaches level %d!", g.Party[idx].Name, g.Party[idx].Level))
		}
	}
	// Gold + item drops fire right after XP, off the same defeated pack.
	// Logged separately so a fight that paid out nothing (e.g. a zero-gold
	// Skeleton raise) doesn't print a misleading "finds 0 gold" line.
	gold, drops := core.AwardBattleLoot(g)
	if gold > 0 {
		setBattleMessage(g, fmt.Sprintf("Party finds %d gold.", gold))
	}
	for _, kind := range drops {
		setBattleMessage(g, fmt.Sprintf("Picked up %s.", core.ItemInfo(kind).Name))
	}
}

// DebugSkipWin auto-resolves a pack as a win WITHOUT entering the battle
// scene — the debug "Skip Battles" toggle's engagement path. It fells every
// pack member, then runs the exact win bookkeeping a fought battle would
// (winBattle: bestiary kills + XP + loot) and the normal teardown (leaveBattle
// → clearBattleResidual, which removes the defeated pack because Phase is
// BattleWon). Called from explore's engagement check instead of Start when
// g.DebugSkipBattles is on. No-ops on an invalid / already-dead pack.
func DebugSkipWin(g *core.GameState, packIndex int) {
	if packIndex < 0 || packIndex >= len(g.Packs) || !core.PackAlive(g.Packs[packIndex]) {
		return
	}
	g.Battle.ActivePack = packIndex
	g.Battle.EnemyIndex = -1
	// Preallocate the log so winBattle's setBattleMessage appends don't grow a
	// nil slice (Start does the same before its messages).
	g.Battle.Log = make([]string, 0, core.BattleLogMaxLines)
	// Fell the whole pack so RecordBattleKills / AwardBattleXP / AwardBattleLoot
	// tally it exactly as a fought win would.
	for i := range g.Packs[packIndex].Members {
		g.Packs[packIndex].Members[i].HP = 0
		g.Packs[packIndex].Members[i].Alive = false
	}
	winBattle(g, "Debug: skipped the battle.")
	// leaveBattle's clearBattleResidual drops the pack (Phase == BattleWon) and
	// resets all battle transients back to the explore-clean state.
	leaveBattle(g, g.Area.QuietMessage)
}

func loseBattle(g *core.GameState, message string) {
	g.Battle.Phase = core.BattleLost
	g.Battle.ClearTiming()
	resetBattleAction(g)
	setBattleMessage(g, message)
}

// fleeBattle is the debug "Easy Battle Quit" exit: drop the engaged pack
// from the field (so the player doesn't immediately re-trigger it) and
// return to explore. No XP, no win/loss — purely a way to bail out of a
// fight while testing. Gated on g.EasyBattleQuit at the call site.
func fleeBattle(g *core.GameState) {
	if g.Battle.ActivePack >= 0 && g.Battle.ActivePack < len(g.Packs) {
		g.Packs = append(g.Packs[:g.Battle.ActivePack], g.Packs[g.Battle.ActivePack+1:]...)
	}
	// The fled pack is already removed above; clear ActivePack BEFORE
	// leaveBattle so clearBattleResidual's "pack defeated" drop (which fires
	// on LivingBattleCount==0) can't re-remove whatever pack shifted into the
	// now-stale slot.
	g.Battle.ActivePack = -1
	leaveBattle(g, g.Area.QuietMessage)
}

func leaveBattle(g *core.GameState, message string) {
	clearBattleResidual(g)
	g.Battle.Phase = core.BattleNone
	if message != "" {
		setBattleMessage(g, message)
	}
	// Pending level-ups are no longer force-opened post-battle — the
	// auto-modal interrupted the explore flow and locked the player into
	// committing stats before they could even walk away. PendingLevelUps
	// + PendingSkillPoints now sit on the member; the party-card "+"
	// indicator surfaces that there's a level to allocate, and the
	// player spends them when ready via the Tome menu (Character tab
	// for stats, Skills tab for the skill tree).
}

// clearBattleResidual drops every transient battle field back to its zero
// value. Used by leaveBattle and by Update's defensive early-exit so a
// desynced encounter (e.g. EnemyIndex points at a culled enemy) leaves no
// queue / timing-bar residue lingering for the next frame to inherit.
// Defeated packs are dropped from the field here so the cleared slot
// doesn't keep ghost-rendering an empty marker.
func clearBattleResidual(g *core.GameState) {
	// Release any party members still ingested at battle exit so the
	// "out of action" lockout doesn't bleed into the next encounter.
	// damageEnemy already releases on the killing blow; this catches
	// every other exit (loss recovery, early-exit defensive path).
	core.ReleaseAllIngested(g.Party)
	// Clear the combat-only statuses (Sleep / Stun / Webbed / Confused /
	// Defending) so they don't linger into exploration. Poison and death
	// intentionally persist past the fight — see ClearPartyTransientStatuses.
	core.ClearPartyTransientStatuses(g.Party)
	// Drop any queued VFX intents — a stale "spawn ember on enemy 2"
	// would otherwise materialise in the wrong scene on the next
	// render. Also signal the render layer to clear its particle
	// pool, since formation-relative particles spawned during the
	// fight have world positions that were captured camera-relative
	// (in front of the battle camera at fixed offsets) — after
	// battle exit, the explore camera moves freely and those
	// positions are now at random world locations. Without the
	// reset, the player sees a ~1s "ghost burst" floating in mid-
	// air on every battle exit.
	g.VFXQueue = g.VFXQueue[:0]
	core.RequestVFXReset(g)
	// Drop the active pack when it's defeated — either the normal BattleWon
	// path OR the defensive early-exit in Update (all enemies dead but the
	// phase never reached BattleWon, e.g. a desynced kill). LivingBattleCount
	// counts living enemies, so ==0 means the pack is wiped; on a loss / flee
	// enemies survive (count > 0) and the pack correctly stays on the field.
	packDefeated := g.Battle.Phase == core.BattleWon || core.LivingBattleCount(g) == 0
	if packDefeated && g.Battle.ActivePack >= 0 && g.Battle.ActivePack < len(g.Packs) {
		g.Packs = append(g.Packs[:g.Battle.ActivePack], g.Packs[g.Battle.ActivePack+1:]...)
	}
	g.Battle.ActivePack = -1
	g.Battle.EnemyIndex = -1
	g.Battle.Queue = nil
	g.Battle.QueueCursor = 0
	g.Battle.NextRoundQueue = nil
	g.Battle.Readiness = nil
	g.Battle.ClearTiming()
	g.Battle.TimingIntro = 0
	g.Battle.ChargeNeedsRelease = false
	g.Battle.HitStop = 0
	g.Battle.SequencePulseTimer = 0
	g.Battle.SequencePulseIndex = -1
	g.Battle.EnemyAttacker = -1
	g.Battle.EnemyAttackCursor = -1
	g.Battle.EnemyPendingSkill = core.SkillNone
	resetBattleAction(g)
}

func recoverFromLoss(g *core.GameState) {
	// ResetGameState now requests the VFX reset itself (so the in-menu
	// Restart path gets it too), which drops the lost fight's formation-
	// relative particles that would otherwise ghost in the field.
	core.ResetGameState(g)
	// Recovery toast is a transient status, not a combat-log event — using
	// setBattleStatus keeps the fresh-run Log empty (the player isn't in
	// battle anymore) while still surfacing the message on the field HUD.
	// The Area's QuietMessage that ResetGameState seeded into Message is
	// briefly replaced, which is the intended behavior — the toast tells
	// the player what just happened, then any movement / encounter writes
	// over it normally.
	setBattleStatus(g, "You catch your breath.")
}

func resetBattleAction(g *core.GameState) {
	g.Battle.ActionMode = core.ActionMenu
	g.Battle.MenuIndex = 0
	g.Battle.PendingSkill = core.SkillNone
	g.Battle.PendingItem = core.ItemNone
	g.Battle.ItemMenuIndex = 0
}

// setBattleStatus writes to the transient prompt slot. The renderer's
// transientStatus heuristic separates "status" from "log" by checking
// Message != Log[-1] — so the order contract is: call setBattleStatus to
// surface a prompt or validation error (before any action commits), then
// call setBattleMessage when the action lands. Reversing the order works
// today because a real action's message lands in both Message AND Log, so
// the now-stale status from a prior setBattleStatus gets dedupe-suppressed
// — but callers shouldn't rely on setBattleStatus AFTER setBattleMessage
// in the same frame: the status would survive a frame instead of being
// instantly shadowed by the action's log line.
func setBattleStatus(g *core.GameState, message string) {
	g.SetStatusMessage(message)
}

// setBattleMessage writes to the transient status line AND appends to the
// rolling combat log when the message is non-empty and not an immediate
// repeat of the previous line.
//
// Dedupe is intentionally one-step-behind only: we drop "X then X" but NOT
// "X then Y then X" so the log preserves chronology. A future reader who's
// tempted to turn this into a set-dedupe should know that alternating-actor
// patterns ("Warrior hits / Rat bites / Warrior hits") reading the same
// twice is correct — the player needs to see the sequence.
func setBattleMessage(g *core.GameState, message string) {
	g.Battle.Message = message
	if message == "" {
		return
	}
	if len(g.Battle.Log) > 0 && g.Battle.Log[len(g.Battle.Log)-1] == message {
		return
	}
	g.Battle.Log = append(g.Battle.Log, message)
	if len(g.Battle.Log) > core.BattleLogMaxLines {
		g.Battle.Log = g.Battle.Log[len(g.Battle.Log)-core.BattleLogMaxLines:]
	}
}

// tickHitTimers decays the three per-actor hit-reaction timers (lunge
// bump, damage flash, recoil knockback) toward zero — the trio
// ApplyFlatDamage/ApplyHitRecoil arm. Shared by the party and enemy decay
// loops so they can't drift on which timers fade.
func tickHitTimers(bump, flash, knockback *float32, dt float32) {
	*bump = core.ApproachZero(*bump, dt)
	*flash = core.ApproachZero(*flash, dt)
	*knockback = core.ApproachZero(*knockback, dt)
}

func updateBattleEffects(g *core.GameState, dt float32) {
	for i := range g.Party {
		tickHitTimers(&g.Party[i].AttackBump, &g.Party[i].DamageFlash, &g.Party[i].HitKnockback, dt)
	}
	members := core.BattleMembers(g)
	for i := range members {
		tickHitTimers(&members[i].AttackBump, &members[i].DamageFlash, &members[i].HitKnockback, dt)
		members[i].DeathFade = core.ApproachZero(members[i].DeathFade, dt)
		members[i].DamagePopupTimer = core.ApproachZero(members[i].DamagePopupTimer, dt)
	}
	g.Battle.SequencePulseTimer = core.ApproachZero(g.Battle.SequencePulseTimer, dt)
	if g.Battle.SequencePulseTimer <= 0 {
		g.Battle.SequencePulseIndex = -1
	}
	// Decay the combat screen shake. Paused during hit-stop (Update
	// early-returns before this runs while HitStop > 0), so a Great/Excellent
	// hit reads as freeze → settle: the camera shakes hard through the freeze,
	// then eases back once the world unpauses.
	g.Battle.ShakeTimer = core.ApproachZero(g.Battle.ShakeTimer, dt)
}

func battleDeathFadeActive(g *core.GameState) bool {
	for _, m := range core.BattleMembers(g) {
		if m.DeathFade > 0 {
			return true
		}
	}
	return false
}

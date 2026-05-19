package battle

import (
	"fmt"
	"sort"

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
	g.Battle.Log = nil
	g.Battle.Timing = core.TimingState{}
	g.Battle.TimingFlash = 0
	g.Battle.TimingIntro = 0
	g.Battle.HitStop = 0
	g.Battle.SequencePulseTimer = 0
	g.Battle.SequencePulseIndex = -1
	g.Battle.EnemyAttacker = -1
	g.Battle.LastQualityTimer = 0
	g.Battle.Queue = nil
	g.Battle.QueueCursor = 0
	g.Battle.NextRoundQueue = nil
	resetBattleAction(g)
	setBattleMessage(g, core.BattleEncounterMessage(*g))
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
		}
		return
	}
	updateBattleEffects(g, dt)
	tickQualityPopup(g, dt)
	// Early-exit paths route through leaveBattle so residual queue / timing
	// / attacker state doesn't linger across frames. The message is empty so
	// we don't overwrite the quiet-area message with a stale combat status.
	members := core.BattleMembers(g)
	if g.Battle.EnemyIndex < 0 || g.Battle.EnemyIndex >= len(members) {
		leaveBattle(g, "")
		return
	}
	if core.LivingBattleCount(g) == 0 && g.Battle.Phase != core.BattleWon {
		leaveBattle(g, "")
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

// beginNewRound rebuilds the SPD-sorted turn queue and starts the first
// actor's turn. Burn ticks are NOT applied here — they fire per-actor when
// the burning character's turn comes up (see startActorTurn).
func beginNewRound(g *core.GameState) {
	if core.LivingBattleCount(g) == 0 {
		winBattle(g, core.LastBattleEnemyFallsMessage(*g))
		return
	}
	if core.ActivePartyCount(g.Party) == 0 {
		loseBattle(g, core.BattleLossMessage(*g))
		return
	}
	// Reset the per-round round-robin cursor for enemy attack targets so a
	// fresh round starts attacking from party slot 0. PartyTarget is the
	// player's heal/item ally selection and stays untouched here.
	g.Battle.EnemyAttackCursor = -1
	g.Battle.Queue = buildTurnQueue(g)
	g.Battle.QueueCursor = 0
	// Pre-bake the projection of the round AFTER this one so TurnForecast
	// doesn't have to re-sort every frame. The projection only matters when
	// the current round runs short for the forecast's row budget.
	g.Battle.NextRoundQueue = buildTurnQueue(g)
	startActorTurn(g)
}

// buildTurnQueue assembles all living actors into a single list, sorted by
// SPD descending. Stable sort means tied speeds keep their original order
// (party first, then enemies, in their slot order). Ingested party members
// are skipped — they re-enter the queue on the round AFTER their swallower
// dies (the release flips Ingested off, the next beginNewRound picks them
// up).
func buildTurnQueue(g *core.GameState) []core.ActorRef {
	members := core.BattleMembers(g)
	queue := make([]core.ActorRef, 0, len(g.Party)+len(members))
	for i, p := range g.Party {
		if p.HP <= 0 || p.Ingested {
			continue
		}
		queue = append(queue, core.ActorRef{IsParty: true, Index: i})
	}
	for slot, m := range members {
		if !m.Alive {
			continue
		}
		queue = append(queue, core.ActorRef{IsParty: false, Index: slot})
	}
	sort.SliceStable(queue, func(a, b int) bool {
		return actorSpeed(g, queue[a]) > actorSpeed(g, queue[b])
	})
	return queue
}

// actorSpeed returns the SPD of the actor. Party uses Stats.SPD; enemies use
// the implicit per-kind Speed from EnemyDefinition.
func actorSpeed(g *core.GameState, actor core.ActorRef) int {
	if actor.IsParty {
		if actor.Index < 0 || actor.Index >= len(g.Party) {
			return 0
		}
		return g.Party[actor.Index].Stats.SPD
	}
	m := core.BattleMemberAt(g, actor.Index)
	if m == nil {
		return 0
	}
	return core.EnemyInfoFor(*m).Speed
}

// startActorTurn opens the turn of whatever actor sits at the queue cursor.
// Order each turn: skip-if-dead → burn tick (may kill) → input action.
// If the queue runs off the end, a fresh round starts (which may end the
// battle if everyone on a side has died).
func startActorTurn(g *core.GameState) {
	for g.Battle.QueueCursor < len(g.Battle.Queue) {
		if isActorAlive(g, g.Battle.Queue[g.Battle.QueueCursor]) {
			break
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

	// Sleep skip — decrement the sleep counter at the start of the
	// sleeping actor's own turn (same rhythm as burn) and forfeit
	// their action this round. Damage during a future round wakes them
	// early; here we only handle the "still asleep at turn start" path.
	if asleep := tickSleepAtTurnStart(g, actor); asleep {
		g.Battle.QueueCursor++
		startActorTurn(g)
		return
	}

	if actor.IsParty {
		beginPartyTurn(g, actor.Index)
	} else {
		beginEnemyAttack(g, actor.Index)
	}
}

// tickSleepAtTurnStart decrements SleepTurns and emits a "(Name) sleeps."
// log line when the actor is still asleep at turn start. Returns true
// when the actor must skip their input action this turn. Mirrors
// tickBurnAtTurnStart's shape: counter ticks here, wake-on-damage lives
// in the damage helpers, both rules pinned to the actor's own turn.
func tickSleepAtTurnStart(g *core.GameState, actor core.ActorRef) bool {
	if actor.IsParty {
		if actor.Index < 0 || actor.Index >= len(g.Party) {
			return false
		}
		m := &g.Party[actor.Index]
		if m.HP <= 0 || m.SleepTurns <= 0 {
			return false
		}
		m.SleepTurns--
		setBattleMessage(g, fmt.Sprintf("%s is asleep.", m.Name))
		return true
	}
	enemy := core.BattleMemberAt(g, actor.Index)
	if enemy == nil || !enemy.Alive || enemy.SleepTurns <= 0 {
		return false
	}
	enemy.SleepTurns--
	setBattleMessage(g, fmt.Sprintf("The %s is asleep.", core.EnemyInfoFor(*enemy).SingularNoun))
	return true
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

// finishActorTurn is the single hand-off used by every action's apply* path.
// Replaces the old finishPartyAction (party→party / party→enemy phase flip).
// Checks win/lose, fixes up EnemyIndex if the target died, advances the
// queue cursor, and starts the next actor's turn.
//
// Poison tick runs HERE for party actors — right after their action lands,
// before win/lose checks so a poison kill is honored if it drops the last
// living member.
func finishActorTurn(g *core.GameState) {
	g.Battle.Timing = core.TimingState{}
	g.Battle.TimingFlash = 0
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
		tickPoisonAfterPartyTurn(g, g.Battle.Queue[g.Battle.QueueCursor])
	}
	if core.LivingBattleCount(g) == 0 {
		winBattle(g, core.LastBattleEnemyFallsMessage(*g))
		return
	}
	if core.ActivePartyCount(g.Party) == 0 {
		loseBattle(g, core.BattleLossMessage(*g))
		return
	}
	if !core.BattleEnemyAlive(g, g.Battle.EnemyIndex) {
		if next := core.NextLivingBattleEnemy(g); next >= 0 {
			g.Battle.EnemyIndex = next
		}
	}
	g.Battle.QueueCursor++
	startActorTurn(g)
}

// --- Party turn ------------------------------------------------------------

// beginPartyTurn opens the action menu for the given party member. Owns
// Phase=BattlePlayer plus Defending-flag reset (defending lasts exactly one
// round-trip — through the rest of the round and back).
func beginPartyTurn(g *core.GameState, partyIndex int) {
	g.Battle.Phase = core.BattlePlayer
	g.Battle.Timing = core.TimingState{}
	g.Battle.TimingFlash = 0
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
			loseBattle(g, core.BattleLossMessage(*g))
			return
		}
	}
	if !core.BattleEnemyAlive(g, g.Battle.EnemyIndex) {
		if next := core.NextLivingBattleEnemy(g); next >= 0 {
			g.Battle.EnemyIndex = next
		}
	}
}

func updatePlayerBattle(g *core.GameState) {
	// If the current actor died between turns (e.g. an enemy went first this
	// round and killed them), the queue would normally advance past us — but
	// as a defensive net here we skip ahead.
	if !core.PartyMemberAlive(g.Party, g.Battle.CurrentParty) {
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
		onResolve()
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
		g.Battle.HitStop = stop
		return true
	}
	onResolve()
	return true
}

// updateAttackTiming drives the player's attack/skill timing bar. The bar
// resolves either when the player provides input (press for press-kind bars,
// release for charge-kind bars) or when the cursor runs off the end. An
// engaged resolution triggers a brief flash hold so the quality reads before
// the action lands; an auto-miss applies immediately so the turn doesn't
// dwell on a non-event.
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
		if g.Battle.Timing.Kind == core.TimingKindCharge && engageReady && input.AttackTimingPressed() {
			g.Battle.TimingIntro = 0
		} else {
			g.Battle.TimingIntro -= dt
			return
		}
	}

	switch g.Battle.Timing.Kind {
	case core.TimingKindCharge:
		// Hold/Release also gated by engageReady. A player still
		// holding the confirm key at the 3s auto-arm doesn't engage
		// the bar — cursor ticks past Peak to Miss naturally. They
		// must release-then-press to land a quality.
		if engageReady && !g.Battle.Timing.Resolved && input.AttackTimingHeld() {
			g.Battle.Timing.Hold()
		}
		if engageReady && !g.Battle.Timing.Resolved && input.AttackTimingReleased() {
			g.Battle.Timing.Release()
		}
		if !g.Battle.Timing.Resolved {
			g.Battle.Timing.Tick(dt)
		}
	case core.TimingKindSequence:
		prevCursor := g.Battle.Timing.SequenceCursor
		if !g.Battle.Timing.Resolved && input.ArrowUpPressed() {
			g.Battle.Timing.SequenceInput(core.SeqDirUp)
		} else if !g.Battle.Timing.Resolved && input.ArrowRightPressed() {
			g.Battle.Timing.SequenceInput(core.SeqDirRight)
		} else if !g.Battle.Timing.Resolved && input.ArrowDownPressed() {
			g.Battle.Timing.SequenceInput(core.SeqDirDown)
		} else if !g.Battle.Timing.Resolved && input.ArrowLeftPressed() {
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
		if !g.Battle.Timing.Resolved {
			g.Battle.Timing.Tick(dt)
		}
	default:
		if !g.Battle.Timing.Resolved && input.AttackTimingPressed() {
			g.Battle.Timing.Press()
		}
		if !g.Battle.Timing.Resolved {
			g.Battle.Timing.Tick(dt)
		}
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
	out := make([]core.SkillID, 0, len(skills))
	for _, s := range skills {
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
	g.Battle.Timing = core.TimingState{}
	g.Battle.EnemyAttacker = -1
	g.Battle.EnemyPendingSkill = core.SkillNone

	if core.ActivePartyCount(g.Party) == 0 {
		loseBattle(g, core.BattleLossMessage(*g))
		return
	}
	g.Battle.QueueCursor++
	startActorTurn(g)
}

// resolveEnemySpell is the apply path for enemy-cast skills (goblin mage
// Firebolt / Sleep). Picks a random living party target, plays the cast
// sound, and dispatches on the skill: Firebolt deals magic damage (armor
// bypassed); Sleep inflicts SleepTurns on the target. Unknown skills
// fall through to a no-op log line so a future enemy author getting the
// skill name wrong doesn't crash mid-encounter.
func resolveEnemySpell(g *core.GameState, slot int, skill core.SkillID) {
	enemy := core.BattleMemberAt(g, slot)
	if enemy == nil || !enemy.Alive {
		return
	}
	target := pickEnemyAttackTarget(g)
	if target < 0 {
		return
	}
	enemy.AttackBump = core.BumpDuration
	def := core.EnemyInfoFor(*enemy)
	skillName := core.SkillName(skill)
	effect := core.SkillEffectFor(skill)
	switch skill {
	case core.SkillFirebolt:
		// Enemy spell-damage formula: SpellPower (per-kind magic
		// stat) + the skill's own Effect.Damage base. SpellPower
		// defaults to 0 so a non-caster enemy mis-routed here can't
		// silently deal huge damage; the goblin mage sets a
		// substantive value in its EnemyDefinition.
		rawDamage := def.SpellPower + effect.Damage
		killed := damagePartyMember(g, target, rawDamage, core.SkillTagMagic)
		if killed {
			setBattleMessage(g, fmt.Sprintf("The %s incinerates %s.", def.SingularNoun, g.Party[target].Name))
		} else {
			setBattleMessage(g, fmt.Sprintf("The %s casts %s — %s burns for %d.", def.SingularNoun, skillName, g.Party[target].Name, rawDamage))
		}
		audio.Play(audio.SoundInputGreat)
	case core.SkillIngest:
		// Defensive re-check: enemyAIPickSkill won't route here without an
		// available target, but the world can shift between turns (e.g. a
		// fast ally killed the only viable target). Cancel cleanly with a
		// log line so the combat log doesn't go silent on the cast.
		picked := target
		if !core.PartyMemberAvailable(g.Party, picked) {
			picked = core.FirstAvailablePartyMember(g.Party)
		}
		if picked < 0 {
			setBattleMessage(g, fmt.Sprintf("The %s lunges, but finds no one to seize.", def.SingularNoun))
			return
		}
		m := &g.Party[picked]
		m.Ingested = true
		m.IngestedBy = slot
		// Status fields cleared vs. preserved on ingestion is a
		// deliberate-feel choice, not housekeeping:
		//
		//   - SleepTurns: cleared. Getting swallowed is violent enough
		//     to wake the sleeper (same "violence breaks the spell"
		//     rule as the damage-wake path); also avoids the absurd
		//     state of "asleep inside a mantrap, also asleep otherwise."
		//   - Defending: cleared. The brace was for an incoming hit
		//     they'll never receive; the buff has nothing to apply to.
		//   - PoisonTurns: PRESERVED. Ingest shouldn't be a free escape
		//     from a status the player got hit with earlier. The counter
		//     pauses passively (the ingested member's turn is skipped,
		//     so tickPoisonAfterPartyTurn never fires for them) and
		//     resumes ticking on their first turn after release.
		m.SleepTurns = 0
		m.Defending = false
		setBattleMessage(g, fmt.Sprintf("The %s engulfs %s!", def.SingularNoun, m.Name))
		audio.Play(audio.SoundEnemyHit)
	case core.SkillSleep:
		m := &g.Party[target]
		// Defense-in-depth: pickEnemyAttackTarget only returns living
		// members today, but a future code path that lets a corpse
		// through would silently land sleep on a dead body. Guard
		// here so the rule "sleep needs a living target" lives at
		// the apply seam, not just at the picker.
		if m.HP <= 0 {
			return
		}
		if m.SleepTurns > 0 {
			setBattleMessage(g, fmt.Sprintf("The %s casts %s — %s is already asleep.", def.SingularNoun, skillName, m.Name))
			return
		}
		duration := effect.SleepDuration(g.Rand())
		if duration <= 0 {
			duration = core.SleepMinTurns
		}
		m.SleepTurns = duration
		setBattleMessage(g, fmt.Sprintf("The %s casts %s — %s falls asleep.", def.SingularNoun, skillName, m.Name))
		audio.Play(audio.SoundInputHit)
	default:
		// #12: log the skill id so a future author who registers a
		// new skill but forgets to add a case here at least sees the
		// numeric culprit in the combat log instead of a silent fizzle.
		setBattleMessage(g, fmt.Sprintf("The %s mutters something (unhandled skill %d).", def.SingularNoun, int(skill)))
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
	g.Battle.Phase = core.BattleWon
	g.Battle.Timer = core.VictoryDanceDuration
	g.Battle.Timing = core.TimingState{}
	g.Battle.TimingFlash = 0
	resetBattleAction(g)
	setBattleMessage(g, message)
	// XP award fires once, right after the kill is confirmed. Living
	// members get the full pack value; dead members get nothing
	// (incentive to keep your tank up). Level-ups queue stat points
	// onto PendingLevelUps; the level-up modal opens on leaveBattle
	// when those points are still unspent.
	perMember, leveled := core.AwardBattleXP(g)
	if perMember > 0 {
		setBattleMessage(g, fmt.Sprintf("%s Party gains %d XP each.", message, perMember))
		for _, idx := range leveled {
			setBattleMessage(g, fmt.Sprintf("%s reaches level %d!", g.Party[idx].Name, g.Party[idx].Level))
		}
	}
}

func loseBattle(g *core.GameState, message string) {
	g.Battle.Phase = core.BattleLost
	g.Battle.Timing = core.TimingState{}
	g.Battle.TimingFlash = 0
	resetBattleAction(g)
	setBattleMessage(g, message)
}

func leaveBattle(g *core.GameState, message string) {
	clearBattleResidual(g)
	g.Battle.Phase = core.BattleNone
	if message != "" {
		setBattleMessage(g, message)
	}
	// If any party member walked away with unspent level-up points, raise
	// the level-up modal so the player allocates them before drifting
	// back to exploration. explore.Update routes through this flag
	// above the pause/chest priorities.
	if idx := core.FirstPendingLevelUp(g.Party); idx >= 0 {
		g.LevelUpOpen = true
		g.LevelUpMember = idx
		g.LevelUpStat = core.StatSTR
	}
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
	if g.Battle.Phase == core.BattleWon && g.Battle.ActivePack >= 0 && g.Battle.ActivePack < len(g.Packs) {
		g.Packs = append(g.Packs[:g.Battle.ActivePack], g.Packs[g.Battle.ActivePack+1:]...)
	}
	g.Battle.ActivePack = -1
	g.Battle.EnemyIndex = -1
	g.Battle.Queue = nil
	g.Battle.QueueCursor = 0
	g.Battle.NextRoundQueue = nil
	g.Battle.Timing = core.TimingState{}
	g.Battle.TimingFlash = 0
	g.Battle.TimingIntro = 0
	g.Battle.ChargeNeedsRelease = false
	g.Battle.HitStop = 0
	g.Battle.SequencePulseTimer = 0
	g.Battle.SequencePulseIndex = -1
	g.Battle.EnemyAttacker = -1
	g.Battle.EnemyAttackCursor = -1
	resetBattleAction(g)
}

func recoverFromLoss(g *core.GameState) {
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
	g.Battle.Message = message
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

func updateBattleEffects(g *core.GameState, dt float32) {
	for i := range g.Party {
		g.Party[i].AttackBump = core.ApproachZero(g.Party[i].AttackBump, dt)
		g.Party[i].DamageFlash = core.ApproachZero(g.Party[i].DamageFlash, dt)
	}
	members := core.BattleMembers(g)
	for i := range members {
		members[i].AttackBump = core.ApproachZero(members[i].AttackBump, dt)
		members[i].DamageFlash = core.ApproachZero(members[i].DamageFlash, dt)
		members[i].DeathFade = core.ApproachZero(members[i].DeathFade, dt)
		members[i].DamagePopupTimer = core.ApproachZero(members[i].DamagePopupTimer, dt)
	}
	g.Battle.SequencePulseTimer = core.ApproachZero(g.Battle.SequencePulseTimer, dt)
	if g.Battle.SequencePulseTimer <= 0 {
		g.Battle.SequencePulseIndex = -1
	}
}

func battleDeathFadeActive(g *core.GameState) bool {
	for _, m := range core.BattleMembers(g) {
		if m.DeathFade > 0 {
			return true
		}
	}
	return false
}

package battle

import (
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
	if g.Battle.Phase != core.BattleWon && g.Battle.Phase != core.BattleLost && core.LivingPartyCount(g.Party) == 0 {
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
	if core.LivingPartyCount(g.Party) == 0 {
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
// (party first, then enemies, in their slot order).
func buildTurnQueue(g *core.GameState) []core.ActorRef {
	members := core.BattleMembers(g)
	queue := make([]core.ActorRef, 0, len(g.Party)+len(members))
	for i, p := range g.Party {
		if p.HP <= 0 {
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

	if actor.IsParty {
		beginPartyTurn(g, actor.Index)
	} else {
		beginEnemyAttack(g, actor.Index)
	}
}

// isActorAlive answers "is this queue actor still in the fight?" Used for
// skipping dead entries in the round queue.
func isActorAlive(g *core.GameState, actor core.ActorRef) bool {
	if actor.IsParty {
		return actor.Index >= 0 && actor.Index < len(g.Party) && g.Party[actor.Index].HP > 0
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
	if core.LivingPartyCount(g.Party) == 0 {
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
//	1. TimingFlash counts down (bar pulses with quality color, cursor frozen)
//	2. HitStop counts down (everything in the world freezes — see Update)
//	3. onResolve fires (damage / heal applies, popup spawns)
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
	if g.Battle.TimingIntro > 0 {
		// Charge bars skip the intro the moment the player engages the input
		// — that's the "starts if you hit the input" behavior. For other
		// kinds the intro is short anyway and runs out on its own.
		if g.Battle.Timing.Kind == core.TimingKindCharge && (input.AttackTimingHeld() || input.AttackTimingPressed()) {
			g.Battle.TimingIntro = 0
		} else {
			g.Battle.TimingIntro -= dt
			return
		}
	}

	switch g.Battle.Timing.Kind {
	case core.TimingKindCharge:
		if !g.Battle.Timing.Resolved && input.AttackTimingHeld() {
			g.Battle.Timing.Hold()
		}
		if !g.Battle.Timing.Resolved && input.AttackTimingReleased() {
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
	g.Battle.Timing = core.NewTimingState(g.Rand(), core.DefendTimingDuration)
	g.Battle.TimingFlash = 0
	g.Battle.TimingIntro = core.EnemyTurnIntro
	g.Battle.Phase = core.BattleEnemyTiming
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
// the resolved defend quality) and advances the round queue.
func resolveAndFinishEnemyAttack(g *core.GameState) {
	resolveEnemyAttacker(g, g.Battle.EnemyAttacker, g.Battle.Timing.Quality)
	g.Battle.Timing = core.TimingState{}
	g.Battle.EnemyAttacker = -1

	if core.LivingPartyCount(g.Party) == 0 {
		loseBattle(g, core.BattleLossMessage(*g))
		return
	}
	g.Battle.QueueCursor++
	startActorTurn(g)
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
}

// clearBattleResidual drops every transient battle field back to its zero
// value. Used by leaveBattle and by Update's defensive early-exit so a
// desynced encounter (e.g. EnemyIndex points at a culled enemy) leaves no
// queue / timing-bar residue lingering for the next frame to inherit.
// Defeated packs are dropped from the field here so the cleared slot
// doesn't keep ghost-rendering an empty marker.
func clearBattleResidual(g *core.GameState) {
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
	g.Battle.HitStop = 0
	g.Battle.SequencePulseTimer = 0
	g.Battle.SequencePulseIndex = -1
	g.Battle.EnemyAttacker = -1
	g.Battle.EnemyAttackCursor = -1
	resetBattleAction(g)
}

func recoverFromLoss(g *core.GameState) {
	core.ResetGameState(g)
	setBattleMessage(g, "You catch your breath.")
}

func resetBattleAction(g *core.GameState) {
	g.Battle.ActionMode = core.ActionMenu
	g.Battle.MenuIndex = 0
	g.Battle.PendingSkill = core.SkillNone
	g.Battle.PendingItem = core.ItemNone
	g.Battle.ItemMenuIndex = 0
}

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

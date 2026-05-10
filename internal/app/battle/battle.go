package battle

import (
	"sort"

	"crawler/internal/app/core"
	"crawler/internal/app/input"
)

func Start(g *core.GameState, enemyIndex int) {
	group := nearbyBattleGroup(g.Map, g.Enemies, enemyIndex)
	g.Battle.EnemyIndex = enemyIndex
	g.Battle.EnemyGroup = group
	g.Battle.PartyTarget = core.FirstLivingPartyMember(g.Party)
	g.Battle.Splash = core.BattleSplashDuration
	g.Battle.Log = nil
	g.Battle.Timing = core.TimingState{}
	g.Battle.TimingFlash = 0
	g.Battle.TimingIntro = 0
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
	// can't fast-forward the timing bar past its window in one tick.
	updateBattleEffects(g, dt)
	tickQualityPopup(g, dt)
	if g.Battle.EnemyIndex < 0 || g.Battle.EnemyIndex >= len(g.Enemies) {
		g.Battle.Phase = core.BattleNone
		return
	}
	if core.LivingBattleCount(g) == 0 && g.Battle.Phase != core.BattleWon {
		g.Battle.Phase = core.BattleNone
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
	// fresh round starts attacking from party slot 0.
	g.Battle.PartyTarget = -1
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
	queue := make([]core.ActorRef, 0, len(g.Party)+len(g.Battle.EnemyGroup))
	for i, p := range g.Party {
		if p.HP <= 0 {
			continue
		}
		queue = append(queue, core.ActorRef{IsParty: true, Index: i})
	}
	for slot, enemyIdx := range g.Battle.EnemyGroup {
		if !core.EnemyAlive(g.Enemies, enemyIdx) {
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
	if actor.Index < 0 || actor.Index >= len(g.Battle.EnemyGroup) {
		return 0
	}
	enemyIdx := g.Battle.EnemyGroup[actor.Index]
	if enemyIdx < 0 || enemyIdx >= len(g.Enemies) {
		return 0
	}
	return core.EnemyInfoFor(g.Enemies[enemyIdx]).Speed
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
	if actor.Index < 0 || actor.Index >= len(g.Battle.EnemyGroup) {
		return false
	}
	return core.EnemyAlive(g.Enemies, g.Battle.EnemyGroup[actor.Index])
}

// finishActorTurn is the single hand-off used by every action's apply* path.
// Replaces the old finishPartyAction (party→party / party→enemy phase flip).
// Checks win/lose, fixes up EnemyIndex if the target died, advances the
// queue cursor, and starts the next actor's turn.
func finishActorTurn(g *core.GameState) {
	g.Battle.Timing = core.TimingState{}
	g.Battle.TimingFlash = 0
	if core.LivingBattleCount(g) == 0 {
		winBattle(g, core.LastBattleEnemyFallsMessage(*g))
		return
	}
	if core.LivingPartyCount(g.Party) == 0 {
		loseBattle(g, core.BattleLossMessage(*g))
		return
	}
	if !core.EnemyAlive(g.Enemies, g.Battle.EnemyIndex) {
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
		g.Battle.PartyTarget = core.FirstLivingPartyMember(g.Party)
	}
	if g.Battle.PartyTarget < 0 {
		g.Battle.PartyTarget = 0
	}
	if !core.EnemyAlive(g.Enemies, g.Battle.EnemyIndex) {
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

// updateAttackTiming drives the player's attack/skill timing bar. The bar
// resolves either when the player provides input (press for press-kind bars,
// release for charge-kind bars) or when the cursor runs off the end. An
// engaged resolution triggers a brief flash hold so the quality reads before
// the action lands; an auto-miss applies immediately so the turn doesn't
// dwell on a non-event.
func updateAttackTiming(g *core.GameState, dt float32) {
	if g.Battle.TimingFlash > 0 {
		g.Battle.TimingFlash -= dt
		if g.Battle.TimingFlash > 0 {
			return
		}
		g.Battle.TimingFlash = 0
		applyPendingAction(g, g.Battle.Timing.Quality)
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
		if !g.Battle.Timing.Resolved && input.ArrowUpPressed() {
			g.Battle.Timing.SequenceInput(core.SeqDirUp)
		} else if !g.Battle.Timing.Resolved && input.ArrowRightPressed() {
			g.Battle.Timing.SequenceInput(core.SeqDirRight)
		} else if !g.Battle.Timing.Resolved && input.ArrowDownPressed() {
			g.Battle.Timing.SequenceInput(core.SeqDirDown)
		} else if !g.Battle.Timing.Resolved && input.ArrowLeftPressed() {
			g.Battle.Timing.SequenceInput(core.SeqDirLeft)
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
		return
	}
	applyPendingAction(g, g.Battle.Timing.Quality)
}

// --- Enemy turn ------------------------------------------------------------

// beginEnemyAttack arms the defend bar against the queue-slot enemy. The
// slot is an index into Battle.EnemyGroup (NOT g.Enemies); resolveEnemyAttacker
// indirects through the group to get the actual Enemy.
func beginEnemyAttack(g *core.GameState, slot int) {
	g.Battle.EnemyAttacker = slot
	g.Battle.Timing = core.NewTimingState(core.DefendTimingDuration)
	g.Battle.TimingFlash = 0
	g.Battle.TimingIntro = core.EnemyTurnIntro
	g.Battle.Phase = core.BattleEnemyTiming
}

// updateEnemyTiming drives the player's defend bar against the currently
// attacking enemy. A press triggers a brief flash hold so the block reads;
// an auto-miss (no press) resolves immediately. After resolution, advance
// the round queue.
func updateEnemyTiming(g *core.GameState, dt float32) {
	if g.Battle.TimingFlash > 0 {
		g.Battle.TimingFlash -= dt
		if g.Battle.TimingFlash > 0 {
			return
		}
		g.Battle.TimingFlash = 0
		resolveAndFinishEnemyAttack(g)
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
		return
	}
	resolveAndFinishEnemyAttack(g)
}

// resolveAndFinishEnemyAttack applies the current attacker's hit (scaled by
// the resolved defend quality) and advances the round queue.
func resolveAndFinishEnemyAttack(g *core.GameState) {
	enemyIndex := -1
	if g.Battle.EnemyAttacker >= 0 && g.Battle.EnemyAttacker < len(g.Battle.EnemyGroup) {
		enemyIndex = g.Battle.EnemyGroup[g.Battle.EnemyAttacker]
	}
	resolveEnemyAttacker(g, enemyIndex, g.Battle.Timing.Quality)
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
	g.Battle.EnemyIndex = -1
	g.Battle.EnemyGroup = nil
	g.Battle.Queue = nil
	g.Battle.QueueCursor = 0
	g.Battle.NextRoundQueue = nil
	g.Battle.Timing = core.TimingState{}
	g.Battle.TimingFlash = 0
	resetBattleAction(g)
	g.Battle.Phase = core.BattleNone
	setBattleMessage(g, message)
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

func setBattleMessage(g *core.GameState, message string) {
	g.Battle.Message = message
	if message == "" {
		return
	}
	if len(g.Battle.Log) > 0 && g.Battle.Log[len(g.Battle.Log)-1] == message {
		return
	}
	g.Battle.Log = append(g.Battle.Log, message)
	if len(g.Battle.Log) > 40 {
		g.Battle.Log = g.Battle.Log[len(g.Battle.Log)-40:]
	}
}

func updateBattleEffects(g *core.GameState, dt float32) {
	for i := range g.Party {
		g.Party[i].AttackBump = core.ApproachZero(g.Party[i].AttackBump, dt)
		g.Party[i].DamageFlash = core.ApproachZero(g.Party[i].DamageFlash, dt)
	}
	for i := range g.Enemies {
		g.Enemies[i].AttackBump = core.ApproachZero(g.Enemies[i].AttackBump, dt)
		g.Enemies[i].DamageFlash = core.ApproachZero(g.Enemies[i].DamageFlash, dt)
		g.Enemies[i].DeathFade = core.ApproachZero(g.Enemies[i].DeathFade, dt)
		g.Enemies[i].DamagePopupTimer = core.ApproachZero(g.Enemies[i].DamagePopupTimer, dt)
	}
}

func battleDeathFadeActive(g *core.GameState) bool {
	for _, index := range g.Battle.EnemyGroup {
		if index >= 0 && index < len(g.Enemies) && g.Enemies[index].DeathFade > 0 {
			return true
		}
	}
	return false
}

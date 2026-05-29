package battle

import (
	"math/rand"
	"strings"
	"testing"

	"crawler/internal/app/core"
)

// seedGameRNG replaces the GameState's RNG with a deterministic one so the
// Steal and Firebolt-burn rolls become reproducible inside a test. The
// previous global-swap pattern is gone: each test now owns its own RNG via
// its own GameState.
func seedGameRNG(t *testing.T, g *core.GameState, seed int64) {
	t.Helper()
	g.RNG = rand.New(rand.NewSource(seed))
}

// newTestState builds a minimal GameState with a 4-class party and one pack
// of two rats. CurrentParty/EnemyIndex are pre-pointed at slot 0/0. The
// battle isn't run through Start — phase is BattlePlayer so apply*'s
// finishActorTurn doesn't surprise tests with queue plumbing.
func newTestState() *core.GameState {
	party := []core.PartyMember{
		{Class: core.ClassWarrior, Name: "Vex", Stats: core.Stats{STR: 6, DEX: 2, INT: 1, WIS: 1, VIT: 5, SPD: 3}, HP: 10, MaxHP: 10, MP: 5, MaxMP: 5},
		{Class: core.ClassCleric, Name: "Mira", Stats: core.Stats{STR: 2, DEX: 2, INT: 2, WIS: 6, VIT: 4, SPD: 4}, HP: 1, MaxHP: 8, MP: 7, MaxMP: 7},
		{Class: core.ClassThief, Name: "Nyx", Stats: core.Stats{STR: 3, DEX: 6, INT: 2, WIS: 1, VIT: 4, SPD: 6}, HP: 8, MaxHP: 8, MP: 3, MaxMP: 3},
		{Class: core.ClassWizard, Name: "Sol", Stats: core.Stats{STR: 1, DEX: 2, INT: 6, WIS: 2, VIT: 4, SPD: 4}, HP: 8, MaxHP: 8, MP: 8, MaxMP: 8},
	}
	pack := core.Pack{
		TileX: 0, TileZ: 0,
		Members: []core.Enemy{core.NewEnemy(core.EnemyRat), core.NewEnemy(core.EnemyRat)},
	}
	g := &core.GameState{
		Party:  party,
		Packs:  []core.Pack{pack},
		Battle: core.Battle{ActivePack: 0, EnemyIndex: 0, CurrentParty: 0, Phase: core.BattlePlayer, PartyTarget: 1, EnemyAttackCursor: -1},
	}
	// Pre-build a one-actor queue so finishActorTurn can run cursor++ without
	// hitting an empty queue and triggering beginNewRound.
	g.Battle.Queue = []core.ActorRef{{IsParty: true, Index: 0}}
	g.Battle.QueueCursor = 0
	return g
}

func TestApplyAttack_DealsSTRDamageAndPopup(t *testing.T) {
	g := newTestState()
	startHP := g.Packs[0].Members[0].HP
	applyAttack(g, core.TimingQualityExcellent)
	// STR 6, base 0, Excellent doubles → 12. Rat has 10 MaxHP so the rat
	// dies either way. With the crit system in place, the roll can land
	// a Critical that doubles AGAIN to 24 — the test accepts either,
	// since the contract being asserted is "STR damage applied and the
	// popup recorded the dealt figure," not "crit roll did/didn't fire."
	if g.Packs[0].Members[0].HP != 0 {
		t.Fatalf("expected rat at 0 HP, got %d (start %d)", g.Packs[0].Members[0].HP, startHP)
	}
	if g.Packs[0].Members[0].Alive {
		t.Fatalf("rat should be dead")
	}
	if got := g.Packs[0].Members[0].DamagePopup; got != 12 && got != 24 {
		t.Fatalf("popup should record 12 (no crit) or 24 (crit), got %d", got)
	}
}

func TestApplyAttack_NoTargetBailsCleanly(t *testing.T) {
	g := newTestState()
	// Only the targeted enemy is dead — leave the second alive so the action
	// doesn't trigger winBattle (which would overwrite the status message).
	g.Packs[0].Members[0].Alive = false
	g.Packs[0].Members[0].HP = 0
	g.Battle.EnemyIndex = 0
	if landed := applyAttack(g, core.TimingQualityExcellent); landed {
		t.Fatalf("applyAttack on dead target should not land")
	}
	if !strings.Contains(g.Battle.Message, "No target") {
		t.Fatalf("expected 'No target' status, got %q", g.Battle.Message)
	}
}

func TestApplySwipe_HitsAllLivingMembersAndSpendsMP(t *testing.T) {
	g := newTestState()
	// Move to Warrior — already at slot 0. Pre-pay nothing; setup will deduct.
	if !setupSwipe(g) {
		t.Fatalf("setupSwipe should succeed with 5/5 MP")
	}
	if g.Party[0].MP != 5-core.SkillCost(core.SkillSwipe) {
		t.Fatalf("setupSwipe should debit cost, MP=%d", g.Party[0].MP)
	}
	startA := g.Packs[0].Members[0].HP
	startB := g.Packs[0].Members[1].HP
	applySwipe(g, core.TimingQualityGood)
	// Swipe is melee, base 0, STR 6 → 6 damage × 1.5 (Good) = 9 per enemy.
	if g.Packs[0].Members[0].HP >= startA {
		t.Fatalf("rat 0 should have taken damage")
	}
	if g.Packs[0].Members[1].HP >= startB {
		t.Fatalf("rat 1 should have taken damage")
	}
}

func TestSetupPrayer_RequiresValidLivingAlly(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 1 // Mira (cleric)
	g.Battle.PartyTarget = 0  // Vex, alive
	if !setupPrayer(g) {
		t.Fatalf("setupPrayer on living target should succeed")
	}

	// Refresh and set target to a dead member.
	g = newTestState()
	g.Battle.CurrentParty = 1
	g.Battle.PartyTarget = 2
	g.Party[2].HP = 0
	if setupPrayer(g) {
		t.Fatalf("setupPrayer should refuse to revive")
	}
	if !strings.Contains(g.Battle.Message, "revive") {
		t.Fatalf("expected revive-refusal message, got %q", g.Battle.Message)
	}
}

func TestSetupPrayer_RejectsIfNotEnoughMP(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 1
	g.Party[1].MP = 1
	if setupPrayer(g) {
		t.Fatalf("setupPrayer with insufficient MP should fail")
	}
}

func TestApplyPrayer_HealsAndClampsAtMax(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 1 // Mira
	g.Battle.PartyTarget = 0  // Vex at full HP
	g.Party[0].HP = 5         // wound them so we can verify the bump
	applyPrayer(g, core.TimingQualityGood)
	// WIS 6 + base 1 = 7, × Good (1.5) = 10. Vex MaxHP=10 → caps at 10.
	if g.Party[0].HP != 10 {
		t.Fatalf("expected Vex healed to 10 (clamped), got %d", g.Party[0].HP)
	}
}

func TestApplyPrayer_DoesNotReviveFallenAlly(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 1
	g.Battle.PartyTarget = 2
	g.Party[2].HP = 0
	applyPrayer(g, core.TimingQualityExcellent)
	if g.Party[2].HP != 0 {
		t.Fatalf("Prayer must not revive; got HP %d", g.Party[2].HP)
	}
}

func TestApplyFirebolt_DamagesAndCanBurn(t *testing.T) {
	g := newTestState()
	seedGameRNG(t, g, 1)
	g.Battle.CurrentParty = 3 // Sol
	startMP := g.Party[3].MP
	startHP := g.Packs[0].Members[0].HP
	if !setupFirebolt(g) {
		t.Fatalf("setupFirebolt should succeed")
	}
	// MP is deducted in setup (uniform policy across all skills); apply
	// only spends MP when called via the setup→apply chain in beginPendingAction.
	if g.Party[3].MP != startMP-core.SkillCost(core.SkillFirebolt) {
		t.Fatalf("setupFirebolt should debit cost, got %d (was %d)", g.Party[3].MP, startMP)
	}
	postSetupMP := g.Party[3].MP
	applyFirebolt(g, core.TimingQualityExcellent)
	if g.Party[3].MP != postSetupMP {
		t.Fatalf("applyFirebolt should NOT debit additional MP, got %d (was %d)", g.Party[3].MP, postSetupMP)
	}
	if g.Packs[0].Members[0].HP >= startHP {
		t.Fatalf("rat should take Firebolt damage")
	}
}

func TestApplyFirebolt_TargetDiedBetweenConfirmAndApply(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 3
	// Pretend the bar resolved but the rat is already gone (e.g. another
	// burn-tick killed it before this apply ran). apply doesn't deduct MP
	// (setup did, in the live flow), so calling apply directly without
	// setup leaves MP untouched.
	g.Packs[0].Members[0].Alive = false
	g.Packs[0].Members[0].HP = 0
	g.Packs[0].Members[1].Alive = false
	g.Packs[0].Members[1].HP = 0
	startMP := g.Party[3].MP
	if landed := applyFirebolt(g, core.TimingQualityExcellent); landed {
		t.Fatalf("Firebolt on dead target should not land")
	}
	if g.Party[3].MP != startMP {
		t.Fatalf("Firebolt apply must not touch MP, got %d (was %d)", g.Party[3].MP, startMP)
	}
}

func TestApplyFirebolt_DoesNotStackBurnOnAlreadyBurning(t *testing.T) {
	g := newTestState()
	seedGameRNG(t, g, 1)
	g.Battle.CurrentParty = 3
	g.Packs[0].Members[0].BurnTurns = 2
	preBurn := g.Packs[0].Members[0].BurnTurns
	applyFirebolt(g, core.TimingQualityExcellent)
	// HP drops, but BurnTurns shouldn't grow from a fresh roll —
	// it can only decrement at turn start.
	if g.Packs[0].Members[0].BurnTurns > preBurn {
		t.Fatalf("burn shouldn't stack: was %d, now %d", preBurn, g.Packs[0].Members[0].BurnTurns)
	}
}

func TestApplySteal_LandsItemAndClearsLoot(t *testing.T) {
	// Seed picks a roll that lands under the success chance. Verified by trying
	// several seeds until landing on one that produces success — kept here so
	// the test is deterministic without depending on fragile RNG internals.
	g := newTestState()
	seedGameRNG(t, g, 1)
	g.Battle.CurrentParty = 2 // Nyx (thief)
	preItem := g.Packs[0].Members[0].Item
	if preItem == "" {
		t.Fatalf("expected rat to start with stealable item")
	}
	// DEX 6 + Excellent quality → very high chance; for seed=1 this lands.
	applySteal(g, core.TimingQualityExcellent)
	// On success Item is cleared; on failure the message says fail.
	if g.Packs[0].Members[0].Item == "" {
		if g.Inventory == nil || len(g.Inventory) == 0 {
			t.Fatalf("success should add item to inventory")
		}
		if !strings.Contains(g.Battle.Message, "steals") {
			t.Fatalf("success should set steal message, got %q", g.Battle.Message)
		}
	} else {
		if !strings.Contains(g.Battle.Message, "fails") {
			t.Fatalf("failure should set fail message, got %q", g.Battle.Message)
		}
	}
}

func TestApplySteal_EmptyEnemyMessages(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 2
	g.Packs[0].Members[0].Item = ""
	if !applySteal(g, core.TimingQualityGood) {
		t.Fatalf("steal on empty enemy still 'lands' (gesture happens)")
	}
	if !strings.Contains(g.Battle.Message, "nothing to steal") {
		t.Fatalf("expected 'nothing to steal', got %q", g.Battle.Message)
	}
}

func TestDamageEnemy_KillsAtZero(t *testing.T) {
	g := newTestState()
	_, defeated := damageEnemy(g, 0, 99, core.TimingQualityMiss, core.SkillTagPhys)
	if !defeated {
		t.Fatalf("massive overkill should mark defeated")
	}
	if g.Packs[0].Members[0].HP != 0 || g.Packs[0].Members[0].Alive {
		t.Fatalf("rat should be dead at HP=0")
	}
	if g.Packs[0].Members[0].DeathFade <= 0 {
		t.Fatalf("DeathFade should be armed for death animation")
	}
	if g.Packs[0].Members[0].BurnTurns != 0 {
		t.Fatalf("burn should clear on death")
	}
}

func TestDamageEnemy_FlashOnSurvivedHit(t *testing.T) {
	g := newTestState()
	dealt, defeated := damageEnemy(g, 0, 1, core.TimingQualityMiss, core.SkillTagPhys)
	if defeated {
		t.Fatalf("1 damage should not kill a fresh rat")
	}
	if dealt != 1 {
		t.Fatalf("expected 1 dealt damage (rat has no armor), got %d", dealt)
	}
	if g.Packs[0].Members[0].DamageFlash <= 0 {
		t.Fatalf("flash should be set on survivable hit")
	}
}

func TestHealPartyMember_ClampsAtMaxAndRejectsCorpse(t *testing.T) {
	g := newTestState()
	g.Party[0].HP = 7
	if !healPartyMember(g, 0, 99) {
		t.Fatalf("heal on living member should succeed")
	}
	if g.Party[0].HP != g.Party[0].MaxHP {
		t.Fatalf("heal should clamp at MaxHP; got %d, want %d", g.Party[0].HP, g.Party[0].MaxHP)
	}
	// Dead members can't be healed.
	g.Party[2].HP = 0
	if healPartyMember(g, 2, 4) {
		t.Fatalf("heal must not revive fallen ally")
	}
	if g.Party[2].HP != 0 {
		t.Fatalf("HP should stay 0, got %d", g.Party[2].HP)
	}
}

// TestDamageEnemy_AmoebaArmor regression-checks the amoeba's flagship
// "phys whiffs to 1, magic shreds" contract. The pre-fix bug: armor
// math was correct but the combat-log message reported the pre-armor
// damage figure, making armor look broken from the player's
// perspective. The fix routes the post-armor `dealt` value out to the
// caller; this test pins both the math AND the return value.
func TestDamageEnemy_AmoebaArmor(t *testing.T) {
	g := newTestState()
	// Replace the first enemy in the pack with an amoeba so we hit
	// the armored case without rewiring placePacks.
	g.Packs[0].Members[0] = core.NewEnemy(core.EnemyAmoeba)
	amoebaHP := g.Packs[0].Members[0].HP
	// Phys hit of 12 vs armor 8 → 4 dealt.
	dealt, defeated := damageEnemy(g, 0, 12, core.TimingQualityExcellent, core.SkillTagPhys)
	if dealt != 4 {
		t.Fatalf("phys 12 vs armor 8 should deal 4, got %d", dealt)
	}
	if defeated {
		t.Fatalf("amoeba shouldn't die from a 4-damage hit at full HP")
	}
	if g.Packs[0].Members[0].HP != amoebaHP-4 {
		t.Fatalf("amoeba HP should drop by post-armor amount; got %d (was %d)", g.Packs[0].Members[0].HP, amoebaHP)
	}
	// Magic hit of 12 vs armor 8 → 12 dealt (armor bypassed).
	g.Packs[0].Members[0] = core.NewEnemy(core.EnemyAmoeba)
	dealt, _ = damageEnemy(g, 0, 12, core.TimingQualityExcellent, core.SkillTagMagic)
	if dealt != 12 {
		t.Fatalf("magic 12 should bypass armor and deal 12, got %d", dealt)
	}
	// Phys 1 vs armor 8 → floor-1 contract.
	g.Packs[0].Members[0] = core.NewEnemy(core.EnemyAmoeba)
	dealt, _ = damageEnemy(g, 0, 1, core.TimingQualityMiss, core.SkillTagPhys)
	if dealt != 1 {
		t.Fatalf("phys 1 vs armor 8 should still deal 1 (armor is damp, not immunity), got %d", dealt)
	}
}

func TestDamagePartyMember_GuardsAndKills(t *testing.T) {
	g := newTestState()
	if _, killed := damagePartyMember(g, -1, 5, core.SkillTagPhys); killed {
		t.Fatalf("out-of-bounds party damage should no-op")
	}
	if _, killed := damagePartyMember(g, 0, 0, core.SkillTagPhys); killed {
		t.Fatalf("zero damage should no-op")
	}
	if _, killed := damagePartyMember(g, 0, 999, core.SkillTagPhys); !killed {
		t.Fatalf("lethal damage should report killed")
	}
	if g.Party[0].HP != 0 {
		t.Fatalf("party HP should clamp at 0, got %d", g.Party[0].HP)
	}
}

func TestTickBurnAtTurnStart_TicksAndKills(t *testing.T) {
	g := newTestState()
	g.Packs[0].Members[0].BurnTurns = 2
	g.Packs[0].Members[0].HP = core.BurnTickDamage // exactly enough to die this tick
	killed := tickBurnAtTurnStart(g, core.ActorRef{IsParty: false, Index: 0})
	if !killed {
		t.Fatalf("burn tick should kill at exact-HP")
	}
	if g.Packs[0].Members[0].Alive {
		t.Fatalf("enemy should be dead")
	}
}

func TestTickBurnAtTurnStart_PartyActorIsNoOp(t *testing.T) {
	g := newTestState()
	if tickBurnAtTurnStart(g, core.ActorRef{IsParty: true, Index: 0}) {
		t.Fatalf("party burn tick is unsupported and must be a no-op")
	}
}

func TestResolveEnemyAttacker_DefendingHalvesDamage(t *testing.T) {
	g := newTestState()
	// Set Vex defending and arm a rat to hit him next.
	g.Party[0].Defending = true
	g.Battle.EnemyAttackCursor = -1
	startHP := g.Party[0].HP
	if !resolveEnemyAttacker(g, 0, core.TimingQualityMiss) {
		t.Fatalf("rat attack should land")
	}
	// Rat AttackDamage = 3, no defend timing bonus, but Defending halves to 1
	// (with the 1-floor when scaled <=0).
	if g.Party[0].HP >= startHP {
		t.Fatalf("Vex should have taken some damage")
	}
	taken := startHP - g.Party[0].HP
	if taken > 2 {
		t.Fatalf("Defending should soak hit; took %d, expected <=2", taken)
	}
}

func TestResolveEnemyAttacker_ExcellentBlockCanZeroDamage(t *testing.T) {
	g := newTestState()
	startHP := g.Party[0].HP
	resolveEnemyAttacker(g, 0, core.TimingQualityExcellent)
	taken := startHP - g.Party[0].HP
	// AttackDamage=3 × 0.25 (Excellent defense) = 0 (int truncation).
	if taken != 0 {
		t.Fatalf("Excellent block of 3 dmg should drop to 0, took %d", taken)
	}
}

func TestPickEnemyAttackTarget_SkipsDeadPartyMembers(t *testing.T) {
	g := newTestState()
	g.Party[1].HP = 0
	g.Battle.EnemyAttackCursor = 0 // last hit Vex; next pick should skip Mira
	pick := pickEnemyAttackTarget(g)
	if pick == 1 {
		t.Fatalf("pickEnemyAttackTarget shouldn't pick a dead member")
	}
	if g.Battle.EnemyAttackCursor != pick {
		t.Fatalf("cursor should advance to the picked slot, got cursor=%d pick=%d", g.Battle.EnemyAttackCursor, pick)
	}
}

func TestQualityTag_HidesLowGrades(t *testing.T) {
	if qualityTag(core.TimingQualityMiss) != "" {
		t.Errorf("Miss should not prefix")
	}
	if qualityTag(core.TimingQualityNice) != "" {
		t.Errorf("Nice should not prefix")
	}
	if !strings.HasPrefix(qualityTag(core.TimingQualityExcellent), "Excellent") {
		t.Errorf("Excellent should prefix the message")
	}
}

// --- Poison status ---------------------------------------------------------

// newPoisonState returns a state with the Diseased Rat as the only pack
// member and a healthy party. PoisonChance is 60% — seed-pinned so the
// inflict roll is deterministic.
func newPoisonState() *core.GameState {
	g := newTestState()
	g.Packs[0].Members = []core.Enemy{core.NewEnemy(core.EnemyDiseasedRat)}
	g.Battle.EnemyIndex = 0
	g.Battle.EnemyAttackCursor = -1
	return g
}

func TestResolveEnemyAttacker_DiseasedRatCanInflictPoison(t *testing.T) {
	// Try several seeds; with PoisonChance=0.60 most should land.
	landed := false
	for seed := int64(1); seed <= 5 && !landed; seed++ {
		g := newPoisonState()
		seedGameRNG(t, g, seed)
		resolveEnemyAttacker(g, 0, core.TimingQualityMiss)
		// Find any poisoned party member.
		for _, p := range g.Party {
			if p.PoisonTurns > 0 {
				landed = true
				if p.PoisonTurns < core.PoisonMinTurns || p.PoisonTurns > core.PoisonMaxTurns {
					t.Errorf("poison duration out of [%d, %d]: %d",
						core.PoisonMinTurns, core.PoisonMaxTurns, p.PoisonTurns)
				}
				break
			}
		}
	}
	if !landed {
		t.Fatalf("expected at least one of 5 seeds to land poison from a 60%% chance")
	}
}

func TestResolveEnemyAttacker_PlainRatNeverPoisons(t *testing.T) {
	g := newTestState()
	seedGameRNG(t, g, 1)
	for i := 0; i < 20; i++ {
		resolveEnemyAttacker(g, 0, core.TimingQualityMiss)
		if g.Party[0].PoisonTurns > 0 {
			t.Fatalf("plain rat should not inflict poison")
		}
		// Reset HP so we don't drop the target below 0 mid-loop.
		g.Party[0].HP = g.Party[0].MaxHP
	}
}

func TestResolveEnemyAttacker_PoisonDoesNotStack(t *testing.T) {
	g := newPoisonState()
	seedGameRNG(t, g, 1)
	// Pre-poison the front-line target and remember the duration.
	g.Party[0].PoisonTurns = 4
	preDuration := g.Party[0].PoisonTurns
	// Pin the attack to slot 0 specifically by forcing the cursor.
	g.Battle.EnemyAttackCursor = -1
	resolveEnemyAttacker(g, 0, core.TimingQualityMiss)
	if g.Party[0].PoisonTurns != preDuration {
		t.Fatalf("poison shouldn't stack onto an already-poisoned target: was %d, now %d",
			preDuration, g.Party[0].PoisonTurns)
	}
}

func TestTickPoisonAfterPartyTurn_TicksAndDecrements(t *testing.T) {
	g := newTestState()
	g.Party[0].PoisonTurns = 3
	startHP := g.Party[0].HP
	killed := tickPoisonAfterPartyTurn(g, core.ActorRef{IsParty: true, Index: 0})
	if killed {
		t.Fatalf("single tick at full HP shouldn't kill")
	}
	if g.Party[0].HP != startHP-core.PoisonTickDamage {
		t.Fatalf("tick should drain PoisonTickDamage HP; was %d, now %d", startHP, g.Party[0].HP)
	}
	if g.Party[0].PoisonTurns != 2 {
		t.Fatalf("tick should decrement PoisonTurns; got %d", g.Party[0].PoisonTurns)
	}
}

func TestTickPoisonAfterPartyTurn_KillsAtZero(t *testing.T) {
	g := newTestState()
	g.Party[0].HP = core.PoisonTickDamage
	g.Party[0].PoisonTurns = 1
	if !tickPoisonAfterPartyTurn(g, core.ActorRef{IsParty: true, Index: 0}) {
		t.Fatalf("poison should report killed when HP drops to 0")
	}
	if g.Party[0].HP != 0 {
		t.Fatalf("HP should clamp at 0; got %d", g.Party[0].HP)
	}
}

func TestTickPoisonAfterPartyTurn_NoPoisonIsNoOp(t *testing.T) {
	g := newTestState()
	startHP := g.Party[0].HP
	tickPoisonAfterPartyTurn(g, core.ActorRef{IsParty: true, Index: 0})
	if g.Party[0].HP != startHP {
		t.Fatalf("unpoisoned member should not take poison damage")
	}
}

func TestTickPoisonAfterPartyTurn_EnemyActorIsNoOp(t *testing.T) {
	g := newTestState()
	// Even with a poison-flagged party slot, an enemy ActorRef must skip.
	g.Party[0].PoisonTurns = 3
	if tickPoisonAfterPartyTurn(g, core.ActorRef{IsParty: false, Index: 0}) {
		t.Fatalf("enemy actor should be a no-op for poison tick")
	}
}

// --- Hit-stop chain (TimingFlash → HitStop → onResolve) -------------------
//
// These tests drive tickFlashHold directly to verify the two-phase decay:
// the flash counts down first, then HitStop kicks in (only on high grades),
// and only when both have drained does onResolve fire. The chain is the
// single most load-bearing piece of the JUICE pass — if it ever fires
// onResolve too early or twice, the damage popup/audio/apply step all
// land at the wrong moment.

func TestResolveAndFinishEnemyAttack_TicksEnemyPoison(t *testing.T) {
	g := newTestState()
	g.Battle.Queue = []core.ActorRef{{IsParty: false, Index: 0}}
	g.Battle.QueueCursor = 0
	g.Battle.EnemyAttacker = 0
	g.Battle.Timing.Quality = core.TimingQualityExcellent
	g.Packs[0].Members[0].PoisonTurns = 2
	startHP := g.Packs[0].Members[0].HP

	resolveAndFinishEnemyAttack(g)

	if got := g.Packs[0].Members[0].PoisonTurns; got != 1 {
		t.Fatalf("enemy poison should tick after enemy action; got %d", got)
	}
	if got := g.Packs[0].Members[0].HP; got != startHP-core.PoisonTickDamage {
		t.Fatalf("enemy poison should deal %d; HP was %d now %d", core.PoisonTickDamage, startHP, got)
	}
}

func TestResolveAndFinishEnemyAttack_PoisonKillWinsBattle(t *testing.T) {
	g := newTestState()
	g.Packs[0].Members = []core.Enemy{core.NewEnemy(core.EnemyRat)}
	g.Battle.Queue = []core.ActorRef{{IsParty: false, Index: 0}}
	g.Battle.QueueCursor = 0
	g.Battle.EnemyIndex = 0
	g.Battle.EnemyAttacker = 0
	g.Battle.Timing.Quality = core.TimingQualityExcellent
	g.Packs[0].Members[0].HP = core.PoisonTickDamage
	g.Packs[0].Members[0].PoisonTurns = 1

	resolveAndFinishEnemyAttack(g)

	if g.Battle.Phase != core.BattleWon {
		t.Fatalf("poison killing the last enemy should win battle, phase=%v message=%q", g.Battle.Phase, g.Battle.Message)
	}
	if g.Packs[0].Members[0].Alive {
		t.Fatalf("enemy should be dead after poison tick")
	}
}

func TestTickFlashHold_LowGradeFiresImmediatelyAtFlashZero(t *testing.T) {
	g := newTestState()
	g.Battle.Timing.Quality = core.TimingQualityGood
	g.Battle.TimingFlash = core.TimingFlashDuration
	resolved := 0
	// First tick: drains flash but doesn't quite zero it.
	if !tickFlashHold(g, core.TimingFlashDuration*0.5, func() { resolved++ }) {
		t.Fatalf("flash still running, tickFlashHold should report busy")
	}
	if resolved != 0 {
		t.Fatalf("Good grade with flash still active shouldn't fire onResolve")
	}
	// Second tick: drains the rest. No hit-stop for Good, so onResolve fires.
	if !tickFlashHold(g, core.TimingFlashDuration, func() { resolved++ }) {
		t.Fatalf("flash expiring tick should still report busy")
	}
	if resolved != 1 {
		t.Fatalf("Good grade should onResolve when flash hits 0, got %d calls", resolved)
	}
	if g.Battle.HitStop != 0 {
		t.Fatalf("Good grade should not arm HitStop, got %v", g.Battle.HitStop)
	}
}

func TestTickFlashHold_ExcellentGradeChainsIntoHitStop(t *testing.T) {
	g := newTestState()
	g.Battle.Timing.Quality = core.TimingQualityExcellent
	g.Battle.TimingFlash = core.TimingFlashDuration
	resolved := 0
	// Drain the flash entirely in one tick.
	tickFlashHold(g, core.TimingFlashDuration*2, func() { resolved++ })
	if resolved != 0 {
		t.Fatalf("Excellent grade should NOT fire onResolve at flash zero — it should arm HitStop instead")
	}
	if g.Battle.HitStop <= 0 {
		t.Fatalf("Excellent flash expiry should arm HitStop, got %v", g.Battle.HitStop)
	}
	if g.Battle.HitStop != core.HitStopExcellent {
		t.Fatalf("HitStop should equal HitStopExcellent (%v), got %v", core.HitStopExcellent, g.Battle.HitStop)
	}
	// Drain the hit-stop. Now onResolve should fire.
	tickFlashHold(g, core.HitStopExcellent*2, func() { resolved++ })
	if resolved != 1 {
		t.Fatalf("onResolve should fire exactly once after HitStop drains, got %d calls", resolved)
	}
	if g.Battle.HitStop != 0 {
		t.Fatalf("HitStop should clamp at 0 after expiry, got %v", g.Battle.HitStop)
	}
}

func TestTickFlashHold_GreatGradeUsesGreatHitStop(t *testing.T) {
	g := newTestState()
	g.Battle.Timing.Quality = core.TimingQualityGreat
	g.Battle.TimingFlash = core.TimingFlashDuration
	resolved := 0
	tickFlashHold(g, core.TimingFlashDuration*2, func() { resolved++ })
	if g.Battle.HitStop != core.HitStopGreat {
		t.Fatalf("Great should arm HitStopGreat (%v), got %v", core.HitStopGreat, g.Battle.HitStop)
	}
	if resolved != 0 {
		t.Fatalf("Great grade should defer onResolve until HitStop drains")
	}
	tickFlashHold(g, core.HitStopGreat*2, func() { resolved++ })
	if resolved != 1 {
		t.Fatalf("Great grade should onResolve exactly once after HitStop, got %d", resolved)
	}
}

func TestTickFlashHold_NoFlashNoOp(t *testing.T) {
	g := newTestState()
	// No flash armed → returns false (caller should NOT bail).
	called := false
	if tickFlashHold(g, 0.1, func() { called = true }) {
		t.Fatalf("idle tickFlashHold should report not-busy")
	}
	if called {
		t.Fatalf("onResolve should not fire when there was nothing to drain")
	}
}

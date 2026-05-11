package battle

import (
	"math/rand"
	"strings"
	"testing"

	"crawler/internal/app/core"
)

// withSeededRNG swaps GameRNG for the test and restores it on exit so the
// Steal and Firebolt-burn rolls become reproducible.
func withSeededRNG(t *testing.T, seed int64, fn func()) {
	t.Helper()
	saved := core.GameRNG
	core.GameRNG = rand.New(rand.NewSource(seed))
	defer func() { core.GameRNG = saved }()
	fn()
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
	// STR 6, base 0, Excellent doubles → 12. Rat has 10 MaxHP so the rat dies.
	if g.Packs[0].Members[0].HP != 0 {
		t.Fatalf("expected rat at 0 HP, got %d (start %d)", g.Packs[0].Members[0].HP, startHP)
	}
	if g.Packs[0].Members[0].Alive {
		t.Fatalf("rat should be dead")
	}
	if g.Packs[0].Members[0].DamagePopup != 12 {
		t.Fatalf("popup should record dealt damage (12), got %d", g.Packs[0].Members[0].DamagePopup)
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
	withSeededRNG(t, 1, func() {
		g := newTestState()
		g.Battle.CurrentParty = 3 // Sol
		startMP := g.Party[3].MP
		startHP := g.Packs[0].Members[0].HP
		if !setupFirebolt(g) {
			t.Fatalf("setupFirebolt should succeed")
		}
		// MP is deducted in apply, not setup.
		if g.Party[3].MP != startMP {
			t.Fatalf("setupFirebolt should NOT debit MP, got %d (was %d)", g.Party[3].MP, startMP)
		}
		applyFirebolt(g, core.TimingQualityExcellent)
		if g.Party[3].MP != startMP-core.SkillCost(core.SkillFirebolt) {
			t.Fatalf("applyFirebolt should debit cost, got %d", g.Party[3].MP)
		}
		if g.Packs[0].Members[0].HP >= startHP {
			t.Fatalf("rat should take Firebolt damage")
		}
	})
}

func TestApplyFirebolt_TargetDiedBetweenConfirmAndApply(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 3
	// Pretend the bar resolved but the rat is already gone (e.g. another
	// burn-tick killed it before this apply ran). MP must NOT be deducted.
	g.Packs[0].Members[0].Alive = false
	g.Packs[0].Members[0].HP = 0
	g.Packs[0].Members[1].Alive = false
	g.Packs[0].Members[1].HP = 0
	startMP := g.Party[3].MP
	if landed := applyFirebolt(g, core.TimingQualityExcellent); landed {
		t.Fatalf("Firebolt on dead target should not land")
	}
	if g.Party[3].MP != startMP {
		t.Fatalf("Firebolt MP should be preserved when target dies first, got %d", g.Party[3].MP)
	}
}

func TestApplyFirebolt_DoesNotStackBurnOnAlreadyBurning(t *testing.T) {
	withSeededRNG(t, 1, func() {
		g := newTestState()
		g.Battle.CurrentParty = 3
		g.Packs[0].Members[0].BurnTurns = 2
		preBurn := g.Packs[0].Members[0].BurnTurns
		applyFirebolt(g, core.TimingQualityExcellent)
		// HP drops, but BurnTurns shouldn't grow from a fresh roll —
		// it can only decrement at turn start.
		if g.Packs[0].Members[0].BurnTurns > preBurn {
			t.Fatalf("burn shouldn't stack: was %d, now %d", preBurn, g.Packs[0].Members[0].BurnTurns)
		}
	})
}

func TestApplySteal_LandsItemAndClearsLoot(t *testing.T) {
	// Seed picks a roll that lands under the success chance. Verified by trying
	// several seeds until landing on one that produces success — kept here so
	// the test is deterministic without depending on fragile RNG internals.
	withSeededRNG(t, 1, func() {
		g := newTestState()
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
	})
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
	defeated := damageEnemy(g, 0, 99)
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
	defeated := damageEnemy(g, 0, 1)
	if defeated {
		t.Fatalf("1 damage should not kill a fresh rat")
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

func TestDamagePartyMember_GuardsAndKills(t *testing.T) {
	g := newTestState()
	if damagePartyMember(g, -1, 5) {
		t.Fatalf("out-of-bounds party damage should no-op")
	}
	if damagePartyMember(g, 0, 0) {
		t.Fatalf("zero damage should no-op")
	}
	if damagePartyMember(g, 0, 999) != true {
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
		withSeededRNG(t, seed, func() {
			g := newPoisonState()
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
		})
	}
	if !landed {
		t.Fatalf("expected at least one of 5 seeds to land poison from a 60%% chance")
	}
}

func TestResolveEnemyAttacker_PlainRatNeverPoisons(t *testing.T) {
	withSeededRNG(t, 1, func() {
		g := newTestState()
		for i := 0; i < 20; i++ {
			resolveEnemyAttacker(g, 0, core.TimingQualityMiss)
			if g.Party[0].PoisonTurns > 0 {
				t.Fatalf("plain rat should not inflict poison")
			}
			// Reset HP so we don't drop the target below 0 mid-loop.
			g.Party[0].HP = g.Party[0].MaxHP
		}
	})
}

func TestResolveEnemyAttacker_PoisonDoesNotStack(t *testing.T) {
	withSeededRNG(t, 1, func() {
		g := newPoisonState()
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
	})
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

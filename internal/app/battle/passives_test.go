package battle

import (
	"testing"

	"crawler/internal/app/core"
)

// TestDamageEnemy_TalliesPhysicalOnly checks the Bloodthirst feed: physical
// hits accumulate into PhysDamageThisTurn, magic hits don't.
func TestDamageEnemy_TalliesPhysicalOnly(t *testing.T) {
	g := newTestState()
	g.Packs[0].Members[0].HP = 100
	g.Packs[0].Members[0].MaxHP = 100

	damageEnemy(g, 0, 7, core.TimingQualityGood, core.SkillTagPhys)
	if g.Battle.PhysDamageThisTurn != 7 {
		t.Fatalf("phys tally = %d, want 7", g.Battle.PhysDamageThisTurn)
	}
	damageEnemy(g, 0, 5, core.TimingQualityGood, core.SkillTagMagic)
	if g.Battle.PhysDamageThisTurn != 7 {
		t.Errorf("magic hit changed phys tally: got %d, want 7", g.Battle.PhysDamageThisTurn)
	}
}

// TestApplyBloodthirst_HealsShareOfTally verifies the Warrior heals
// BloodthirstHealPerRank of the turn's physical damage per rank, and that the
// passive no-ops without the node.
func TestApplyBloodthirst_HealsShareOfTally(t *testing.T) {
	g := newTestState()
	g.Party[0].TreeRanks = map[string]int{core.PassiveBloodthirst: 3}
	g.Party[0].HP = 1
	g.Party[0].MaxHP = 100
	g.Battle.PhysDamageThisTurn = 20

	applyBloodthirst(g, core.ActorRef{IsParty: true, Index: 0})
	want := 1 + int(20*3*core.BloodthirstHealPerRank) // 1 + 6
	if g.Party[0].HP != want {
		t.Errorf("Bloodthirst HP = %d, want %d", g.Party[0].HP, want)
	}

	// No node → no heal.
	g2 := newTestState()
	g2.Party[0].HP = 1
	g2.Party[0].MaxHP = 100
	g2.Battle.PhysDamageThisTurn = 20
	applyBloodthirst(g2, core.ActorRef{IsParty: true, Index: 0})
	if g2.Party[0].HP != 1 {
		t.Errorf("Bloodthirst healed without the node: HP = %d, want 1", g2.Party[0].HP)
	}
}

// TestApplyBloodthirst_EnemyActorNoOp confirms an enemy actor never lifesteals
// off the tally (the reflect/counter-discard guarantee).
func TestApplyBloodthirst_EnemyActorNoOp(t *testing.T) {
	g := newTestState()
	g.Party[0].TreeRanks = map[string]int{core.PassiveBloodthirst: 3}
	g.Party[0].HP = 1
	g.Party[0].MaxHP = 100
	g.Battle.PhysDamageThisTurn = 20
	applyBloodthirst(g, core.ActorRef{IsParty: false, Index: 0})
	if g.Party[0].HP != 1 {
		t.Errorf("enemy actor triggered Bloodthirst: HP = %d, want 1", g.Party[0].HP)
	}
}

// TestApplyShadowStep_BonusGatedOnInitiative checks the +damage only lands when
// the target hasn't acted yet this round, and only for a member with the node.
func TestApplyShadowStep_BonusGatedOnInitiative(t *testing.T) {
	g := newTestState()
	g.Party[2].TreeRanks = map[string]int{core.PassiveShadowStep: 2} // Nyx, the Thief
	g.Battle.EnemyIndex = 0

	// Default queue: [party0], cursor 0 — enemy 0 isn't before the cursor, so it
	// still acts this round → bonus applies.
	got := applyShadowStep(g, &g.Party[2], 100)
	want := 100 + int(100*2*core.ShadowStepBonusPerRank) // 100 + 30
	if got != want {
		t.Errorf("Shadow Step (target acts later) = %d, want %d", got, want)
	}

	// Enemy already acted this round (sits before the cursor) → no bonus.
	g.Battle.Queue = []core.ActorRef{{IsParty: false, Index: 0}, {IsParty: true, Index: 2}}
	g.Battle.QueueCursor = 1
	if got := applyShadowStep(g, &g.Party[2], 100); got != 100 {
		t.Errorf("Shadow Step fired after target acted: %d, want 100", got)
	}

	// No node → no bonus even with initiative.
	g.Battle.Queue = []core.ActorRef{{IsParty: true, Index: 0}}
	g.Battle.QueueCursor = 0
	if got := applyShadowStep(g, &g.Party[0], 100); got != 100 {
		t.Errorf("Shadow Step fired without the node: %d, want 100", got)
	}
}

// TestTryRiposte_CountersAttacker confirms the dodge counter deals damage to the
// attacker for a Warrior with the node, and nothing without it.
func TestTryRiposte_CountersAttacker(t *testing.T) {
	g := newTestState()
	g.Party[0].TreeRanks = map[string]int{core.PassiveRiposte: 1}
	g.Packs[0].Members[0].HP = 100
	g.Packs[0].Members[0].MaxHP = 100

	tryRiposte(g, 0, 0)
	if g.Packs[0].Members[0].HP >= 100 {
		t.Errorf("Riposte dealt no counter damage: enemy HP = %d", g.Packs[0].Members[0].HP)
	}

	g2 := newTestState()
	g2.Packs[0].Members[0].HP = 100
	g2.Packs[0].Members[0].MaxHP = 100
	tryRiposte(g2, 0, 0) // no node
	if g2.Packs[0].Members[0].HP != 100 {
		t.Errorf("Riposte fired without the node: enemy HP = %d, want 100", g2.Packs[0].Members[0].HP)
	}
}

// TestTryRiposte_FeedsBloodthirst is the fix-1 guard: a Warrior holding both
// Riposte and Bloodthirst lifesteals off the counter, even though it lands on
// the enemy's turn (so the end-of-turn tally never sees it).
func TestTryRiposte_FeedsBloodthirst(t *testing.T) {
	g := newTestState()
	g.Party[0].Stats.STR = 20 // ensure the counter deals enough to round a heal > 0
	g.Party[0].TreeRanks = map[string]int{core.PassiveRiposte: 1, core.PassiveBloodthirst: 3}
	g.Party[0].HP = 1
	g.Party[0].MaxHP = 100
	g.Packs[0].Members[0].HP = 100
	g.Packs[0].Members[0].MaxHP = 100

	tryRiposte(g, 0, 0)
	if g.Party[0].HP <= 1 {
		t.Errorf("Riposte counter didn't feed Bloodthirst: Warrior HP = %d, want > 1", g.Party[0].HP)
	}
}

// TestTryRetribution_NoReflectWhenDefenderDowned is the fix-2 guard: a hit that
// downed the warded member draws no dying-corpse reflection.
func TestTryRetribution_NoReflectWhenDefenderDowned(t *testing.T) {
	g := newTestState()
	g.Party[1].TreeRanks = map[string]int{core.PassiveRetribution: 3}
	g.Party[1].HP = 0 // the blow killed the Cleric
	g.Packs[0].Members[0].HP = 100
	g.Packs[0].Members[0].MaxHP = 100
	tryRetribution(g, 0, 1, 10)
	if g.Packs[0].Members[0].HP != 100 {
		t.Errorf("Retribution reflected from a downed defender: enemy HP = %d, want 100", g.Packs[0].Members[0].HP)
	}
}

// TestTryRetribution_ReflectsShare confirms the reflect scales with rank, no-ops
// without the node, and never touches a dead attacker.
func TestTryRetribution_ReflectsShare(t *testing.T) {
	dropFor := func(rank int) int {
		g := newTestState()
		if rank > 0 {
			g.Party[1].TreeRanks = map[string]int{core.PassiveRetribution: rank} // Mira, the Cleric
		}
		g.Packs[0].Members[0].HP = 100
		g.Packs[0].Members[0].MaxHP = 100
		tryRetribution(g, 0, 1, 10)
		return 100 - g.Packs[0].Members[0].HP
	}

	if d := dropFor(0); d != 0 {
		t.Errorf("Retribution reflected without the node: drop = %d, want 0", d)
	}
	d1, d3 := dropFor(1), dropFor(3)
	if d1 <= 0 {
		t.Errorf("Retribution rank 1 reflected nothing: drop = %d", d1)
	}
	if d3 <= d1 {
		t.Errorf("Retribution didn't scale with rank: rank1 drop %d, rank3 drop %d", d1, d3)
	}

	// Dead attacker: no reflect, no panic.
	g := newTestState()
	g.Party[1].TreeRanks = map[string]int{core.PassiveRetribution: 3}
	g.Packs[0].Members[0].HP = 0
	g.Packs[0].Members[0].Alive = false
	tryRetribution(g, 0, 1, 10)
}

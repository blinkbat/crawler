package battle

import (
	"testing"

	"crawler/internal/app/core"
)

// TestApplyFireball_HitsEveryLivingEnemy verifies the AoE-status path damages
// every living enemy in the pack (the per-target Burn roll is RNG-gated and
// covered indirectly by the tryProcStatus tests).
func TestApplyFireball_HitsEveryLivingEnemy(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 3 // Sol (wizard, INT 6)
	seedGameRNG(t, g, 1)
	startA := g.Packs[0].Members[0].HP
	startB := g.Packs[0].Members[1].HP

	applyFireball(g, core.TimingQualityExcellent)

	if g.Packs[0].Members[0].HP >= startA {
		t.Errorf("rat 0 took no Fireball damage (HP %d >= %d)", g.Packs[0].Members[0].HP, startA)
	}
	if g.Packs[0].Members[1].HP >= startB {
		t.Errorf("rat 1 took no Fireball damage (HP %d >= %d)", g.Packs[0].Members[1].HP, startB)
	}
}

// TestApplyArcBolt_T3BurnsSurvivors is the regression guard for the dead-delta
// fix: Arc Bolt's T3 "+15% Burn" upgrade used to be silently dropped (the old
// applyAoEDamage path rolled no status). Now that applyArcBolt routes through
// applyAoEStatusSkill, a maxed Arc Bolt must be able to Burn enemies that
// survive the hit. Enemies are given large HP so the AoE doesn't kill them
// first (a felled target is correctly skipped by the proc's `defeated` gate);
// seed 2 lands a burn on both rats.
func TestApplyArcBolt_T3BurnsSurvivors(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 3 // Sol (wizard)
	g.Party[3].SkillTiers = map[core.SkillID]int{core.SkillArcBolt: 3}
	for i := range g.Packs[0].Members {
		g.Packs[0].Members[i].HP = 500
		g.Packs[0].Members[i].MaxHP = 500
	}
	seedGameRNG(t, g, 2)

	applyArcBolt(g, core.TimingQualityExcellent)

	if g.Packs[0].Members[0].BurnTurns == 0 && g.Packs[0].Members[1].BurnTurns == 0 {
		t.Error("maxed Arc Bolt should burn a surviving target — the T3 burn delta is no longer dead")
	}
}

// TestApplyCleanse_CuresTargetButKeepsBuff cleanses an ally that is NOT the
// queue-cursor actor (slot 0), so finishActorTurn's end-of-turn drain can't
// confound the assertion. The target's debuffs clear; their Bless buff stays.
func TestApplyCleanse_CuresTargetButKeepsBuff(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 1 // Mira (cleric) casts
	g.Battle.PartyTarget = 3  // cleanse Sol (slot 3, not the drained queue actor)
	g.Party[3].PoisonTurns = 3
	g.Party[3].SleepTurns = 2
	core.StampPartyBuff(&g.Party[3], core.SkillBless, core.SkillEffect{BuffStats: core.Stats{STR: 1}, BuffTurns: 2})

	applyCleanse(g, core.TimingQualityGood)

	if g.Party[3].PoisonTurns != 0 || g.Party[3].SleepTurns != 0 {
		t.Errorf("Cleanse left debuffs on the target: %+v", g.Party[3])
	}
	if got := core.MaxStatusModTurns(g.Party[3].Buffs); got != 2 {
		t.Errorf("Cleanse wrongly stripped the target's Bless buff (turns=%d, want 2)", got)
	}
}

// TestSetupCleanse_RefusesFallenAlly mirrors setupPrayer's revive guard: a
// dead target is refused without spending MP.
func TestSetupCleanse_RefusesFallenAlly(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 1 // Mira
	g.Battle.PartyTarget = 0
	g.Party[0].HP = 0
	mpBefore := g.Party[1].MP

	if setupCleanse(g) {
		t.Error("setupCleanse should refuse a fallen target")
	}
	if g.Party[1].MP != mpBefore {
		t.Errorf("setupCleanse spent MP on a refused cast (MP %d -> %d)", mpBefore, g.Party[1].MP)
	}
}

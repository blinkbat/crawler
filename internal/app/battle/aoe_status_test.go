package battle

import (
	"testing"

	"crawler/internal/app/core"
)

// TestApplyFireball_HitsEveryLivingEnemy: the AoE path damages every living enemy (the per-target Burn roll is tested elsewhere).
func TestApplyFireball_HitsEveryLivingEnemy(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 3
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

// TestApplyArcBolt_T3BurnsSurvivors: Arc Bolt's T3 "+15% Burn" must reach survivors (the old path dropped it).
// Large HP so the AoE doesn't kill first (felled targets are skipped); seed 2 burns both rats.
func TestApplyArcBolt_T3BurnsSurvivors(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 3
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

// TestApplyCleanse_CuresTargetButKeepsBuff: debuffs clear, Bless buff stays. Slot 3 (not the queue actor) so the drain can't confound it.
func TestApplyCleanse_CuresTargetButKeepsBuff(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 1
	g.Battle.PartyTarget = 3
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

// TestSetupCleanse_RefusesFallenAlly: a dead target is refused without spending MP.
func TestSetupCleanse_RefusesFallenAlly(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 1
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

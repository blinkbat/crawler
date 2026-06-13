package battle

import (
	"testing"

	"crawler/internal/app/core"
)

// TestApplyConeOfCold_ChillsWholePack verifies the AoE chill: every living,
// surviving enemy takes damage AND gets the SPD chill stamped (the multi-target
// mirror of Frostbite), via the shared applyAoEStatusSkill body.
func TestApplyConeOfCold_ChillsWholePack(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 3 // wizard casts
	// Two tanky targets so the frost sweep can't kill them — we need survivors
	// to observe the chill on each.
	g.Packs[0].Members = []core.Enemy{core.NewEnemy(core.EnemyRat), core.NewEnemy(core.EnemyRat)}
	for i := range g.Packs[0].Members {
		g.Packs[0].Members[i].HP = 999
		g.Packs[0].Members[i].MaxHP = 999
	}

	if !applyConeOfCold(g, core.TimingQualityGreat) {
		t.Fatal("applyConeOfCold reported not-landed")
	}

	for i, e := range core.BattleMembers(g) {
		if e.HP >= 999 {
			t.Errorf("enemy %d took no damage: HP = %d", i, e.HP)
		}
		if got := core.MaxStatusModTurns(e.Debuffs); got != core.ConeOfColdChillTurns {
			t.Errorf("enemy %d chill turns = %d, want %d", i, got, core.ConeOfColdChillTurns)
		}
		if got := enemyDebuffStats(e).SPD; got != -core.ConeOfColdSPDReduction {
			t.Errorf("enemy %d chill SPD = %d, want %d", i, got, -core.ConeOfColdSPDReduction)
		}
	}
}

// TestApplyConeOfCold_NoChillOnKilledTarget confirms a target the sweep kills
// carries no dangling chill (mirrors the single-target Frostbite guard).
func TestApplyConeOfCold_NoChillOnKilledTarget(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 3
	g.Packs[0].Members = []core.Enemy{core.NewEnemy(core.EnemyRat)}
	g.Packs[0].Members[0].HP = 1 // sweep kills it

	if !applyConeOfCold(g, core.TimingQualityExcellent) {
		t.Fatal("applyConeOfCold reported not-landed")
	}
	e := core.BattleMembers(g)[0]
	if e.Alive {
		t.Fatal("target should be dead after Cone of Cold on 1 HP")
	}
	if len(e.Debuffs) != 0 {
		t.Errorf("killed target carries a chill: %+v, want none", e.Debuffs)
	}
}

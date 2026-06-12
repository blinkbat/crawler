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
		if e.BuffTurns != core.ConeOfColdChillTurns {
			t.Errorf("enemy %d chill BuffTurns = %d, want %d", i, e.BuffTurns, core.ConeOfColdChillTurns)
		}
		if e.BuffStats.SPD != -core.ConeOfColdSPDReduction {
			t.Errorf("enemy %d chill BuffStats.SPD = %d, want %d", i, e.BuffStats.SPD, -core.ConeOfColdSPDReduction)
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
	if e.BuffTurns != 0 {
		t.Errorf("killed target carries a chill: BuffTurns = %d, want 0", e.BuffTurns)
	}
}

package battle

import (
	"testing"

	"crawler/internal/app/core"
)

// TestApplyConeOfCold_ChillsWholePack: every surviving enemy takes damage AND gets the SPD chill (AoE mirror of Frostbite).
func TestApplyConeOfCold_ChillsWholePack(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 3 // wizard casts
	// Tanky so the sweep can't kill — need survivors to observe each chill.
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

// TestApplyConeOfCold_NoChillOnKilledTarget: a killed target carries no dangling chill.
func TestApplyConeOfCold_NoChillOnKilledTarget(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 3
	g.Packs[0].Members = []core.Enemy{core.NewEnemy(core.EnemyRat)}
	g.Packs[0].Members[0].HP = 1

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

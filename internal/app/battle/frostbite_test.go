package battle

import (
	"testing"

	"crawler/internal/app/core"
)

// TestApplyFrostbite_DamagesAndChillsSurvivor verifies the "damage + debuff"
// pattern: a surviving target takes damage AND always gets the SPD chill (the
// enemy BuffStats mirror), with EffectiveEnemyStats folding the slow in.
func TestApplyFrostbite_DamagesAndChillsSurvivor(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 3 // wizard casts
	g.Battle.EnemyIndex = 0
	// Tanky target so the frost hit can't kill it — we need a survivor to
	// observe the chill.
	g.Packs[0].Members[0].HP = 999
	g.Packs[0].Members[0].MaxHP = 999

	baseSPD := core.EnemyInfoFor(g.Packs[0].Members[0]).Stats.SPD

	if !applyFrostbite(g, core.TimingQualityGreat) {
		t.Fatal("applyFrostbite reported not-landed")
	}

	enemy := core.BattleMembers(g)[0]
	if enemy.HP >= 999 {
		t.Errorf("target took no damage: HP = %d, want < 999", enemy.HP)
	}
	if enemy.BuffTurns != core.FrostbiteChillTurns {
		t.Errorf("chill BuffTurns = %d, want %d", enemy.BuffTurns, core.FrostbiteChillTurns)
	}
	if enemy.BuffStats.SPD != -core.FrostbiteSPDReduction {
		t.Errorf("chill BuffStats.SPD = %d, want %d", enemy.BuffStats.SPD, -core.FrostbiteSPDReduction)
	}
	if got := core.EffectiveEnemyStats(enemy).SPD; got != baseSPD-core.FrostbiteSPDReduction {
		t.Errorf("EffectiveEnemyStats.SPD = %d, want %d", got, baseSPD-core.FrostbiteSPDReduction)
	}
}

// TestApplyFrostbite_NoChillOnKill confirms a killing hit stamps no debuff —
// the chill only lands on a surviving target (no dangling buff on a corpse).
func TestApplyFrostbite_NoChillOnKill(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 3
	g.Battle.EnemyIndex = 0
	g.Packs[0].Members[0].HP = 1 // any damage kills

	if !applyFrostbite(g, core.TimingQualityExcellent) {
		t.Fatal("applyFrostbite reported not-landed")
	}

	enemy := core.BattleMembers(g)[0]
	if enemy.Alive {
		t.Fatalf("target should be dead after Frostbite on 1 HP")
	}
	if enemy.BuffTurns != 0 {
		t.Errorf("killed target carries a chill: BuffTurns = %d, want 0", enemy.BuffTurns)
	}
}

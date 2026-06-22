package battle

import (
	"testing"

	"crawler/internal/app/core"
)

// TestApplyFrostbite_DamagesAndChillsSurvivor: a survivor takes damage AND gets the SPD chill folded into EffectiveEnemyStats.
func TestApplyFrostbite_DamagesAndChillsSurvivor(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 3
	g.Battle.EnemyIndex = 0
	// Tanky so the hit can't kill — need a survivor to observe the chill.
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
	if got := core.MaxStatusModTurns(enemy.Debuffs); got != core.FrostbiteChillTurns {
		t.Errorf("chill turns = %d, want %d", got, core.FrostbiteChillTurns)
	}
	if got := enemyDebuffStats(enemy).SPD; got != -core.FrostbiteSPDReduction {
		t.Errorf("chill SPD = %d, want %d", got, -core.FrostbiteSPDReduction)
	}
	if got := core.EffectiveEnemyStats(&enemy).SPD; got != baseSPD-core.FrostbiteSPDReduction {
		t.Errorf("EffectiveEnemyStats.SPD = %d, want %d", got, baseSPD-core.FrostbiteSPDReduction)
	}
}

// TestApplyFrostbite_NoChillOnKill: a killing hit stamps no debuff (no dangling chill on a corpse).
func TestApplyFrostbite_NoChillOnKill(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 3
	g.Battle.EnemyIndex = 0
	g.Packs[0].Members[0].HP = 1

	if !applyFrostbite(g, core.TimingQualityExcellent) {
		t.Fatal("applyFrostbite reported not-landed")
	}

	enemy := core.BattleMembers(g)[0]
	if enemy.Alive {
		t.Fatalf("target should be dead after Frostbite on 1 HP")
	}
	if len(enemy.Debuffs) != 0 {
		t.Errorf("killed target carries a chill: %+v, want none", enemy.Debuffs)
	}
}

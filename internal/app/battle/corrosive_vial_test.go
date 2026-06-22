package battle

import (
	"testing"

	"crawler/internal/app/core"
)

// TestApplyCorrosiveVial_StripsArmor: Armor drops by ArmorReduction (the field damageEnemy reads), floored at 0, no damage.
func TestApplyCorrosiveVial_StripsArmor(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 2
	g.Battle.EnemyIndex = 0
	g.Packs[0].Members[0].Armor = 10
	hpBefore := g.Packs[0].Members[0].HP

	if !applyCorrosiveVial(g, core.TimingQualityGreat) {
		t.Fatal("applyCorrosiveVial reported not-landed")
	}

	enemy := core.BattleMembers(g)[0]
	if enemy.Armor != 10-core.CorrosiveArmorReduction {
		t.Errorf("Armor = %d, want %d", enemy.Armor, 10-core.CorrosiveArmorReduction)
	}
	if enemy.HP != hpBefore {
		t.Errorf("Corrosive Vial dealt damage: HP %d -> %d (should be no-damage)", hpBefore, enemy.HP)
	}
}

// TestApplyCorrosiveVial_FloorsAtZero: a deep strip can't drive Armor negative (which would turn mitigation into a heal).
func TestApplyCorrosiveVial_FloorsAtZero(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 2
	g.Battle.EnemyIndex = 0
	g.Packs[0].Members[0].Armor = 1 // < CorrosiveArmorReduction

	if !applyCorrosiveVial(g, core.TimingQualityMiss) {
		t.Fatal("applyCorrosiveVial reported not-landed")
	}
	if got := core.BattleMembers(g)[0].Armor; got != 0 {
		t.Errorf("Armor = %d, want 0 (must floor, never go negative)", got)
	}
}

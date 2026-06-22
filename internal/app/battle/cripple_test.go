package battle

import (
	"testing"

	"crawler/internal/app/core"
)

// TestApplyCripple_LowersEnemySPD: the negative-SPD debuff lands and EffectiveEnemyStats folds it in while the counter runs.
func TestApplyCripple_LowersEnemySPD(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 2 // thief casts
	g.Battle.EnemyIndex = 0

	target := core.BattleMembers(g)[0]
	baseSPD := core.EnemyInfoFor(target).Stats.SPD

	if !applyCripple(g, core.TimingQualityGreat) {
		t.Fatal("applyCripple reported not-landed")
	}

	enemy := core.BattleMembers(g)[0]
	if got := core.MaxStatusModTurns(enemy.Debuffs); got != core.CrippleTurns {
		t.Errorf("debuff turns = %d, want %d", got, core.CrippleTurns)
	}
	if got := enemyDebuffStats(enemy).SPD; got != -core.CrippleSPDReduction {
		t.Errorf("debuff SPD = %d, want %d", got, -core.CrippleSPDReduction)
	}
	if got := core.EffectiveEnemyStats(&enemy).SPD; got != baseSPD-core.CrippleSPDReduction {
		t.Errorf("EffectiveEnemyStats.SPD = %d, want %d", got, baseSPD-core.CrippleSPDReduction)
	}
}

// TestTickEnemyBuffAfterTurn_DrainsDebuff: the debuff counts down one per enemy turn; SPD reduction stays live until expiry.
func TestTickEnemyBuffAfterTurn_DrainsDebuff(t *testing.T) {
	g := newTestState()
	core.StampEnemyDebuff(&g.Packs[0].Members[0], core.SkillCripple, core.SkillEffect{BuffStats: core.Stats{SPD: -2}, BuffTurns: 2})
	actor := core.ActorRef{IsParty: false, Index: 0}

	base := core.EnemyInfoFor(g.Packs[0].Members[0]).Stats.SPD

	tickEnemyBuffAfterTurn(g, actor)
	if got := core.MaxStatusModTurns(g.Packs[0].Members[0].Debuffs); got != 1 {
		t.Fatalf("after one tick debuff turns = %d, want 1", got)
	}
	if got := core.EffectiveEnemyStats(&g.Packs[0].Members[0]).SPD; got != base-2 {
		t.Errorf("debuff dropped early at 1 turn left: EffectiveEnemyStats.SPD = %d, want %d", got, base-2)
	}

	tickEnemyBuffAfterTurn(g, actor)
	if got := core.MaxStatusModTurns(g.Packs[0].Members[0].Debuffs); got != 0 {
		t.Errorf("after second tick debuff turns = %d, want 0", got)
	}
	if got := core.EffectiveEnemyStats(&g.Packs[0].Members[0]).SPD; got != base {
		t.Errorf("expired debuff still applying: EffectiveEnemyStats.SPD = %d, want base %d", got, base)
	}
}

// TestActorSpeed_CrippleNeverZero: a debuff deeper than base SPD still floors ATB speed at 1,
// so a crippled foe eventually acts (and ticks the debuff down) instead of locking out.
func TestActorSpeed_CrippleNeverZero(t *testing.T) {
	g := newTestState()
	base := core.EnemyInfoFor(g.Packs[0].Members[0]).Stats.SPD
	core.StampEnemyDebuff(&g.Packs[0].Members[0], core.SkillCripple, core.SkillEffect{BuffStats: core.Stats{SPD: -(base + 5)}, BuffTurns: 3})
	actor := core.ActorRef{IsParty: false, Index: 0}

	if got := actorSpeed(g, actor); got < 1 {
		t.Errorf("actorSpeed = %d under a heavy cripple, want floored at >= 1 (else permanent lockout)", got)
	}
}

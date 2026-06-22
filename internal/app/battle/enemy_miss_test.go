package battle

import (
	"math/rand"
	"testing"

	"crawler/internal/app/core"
)

// TestResolveEnemyMiss_DealsNoDamage: a whiffed enemy attack leaves all party HP untouched while resolving cleanly.
func TestResolveEnemyMiss_DealsNoDamage(t *testing.T) {
	g := newTestState()
	g.Battle.EnemyAttacker = 0

	before := make([]int, len(g.Party))
	for i := range g.Party {
		before[i] = g.Party[i].HP
	}

	resolveEnemyMiss(g, 0)

	for i := range g.Party {
		if g.Party[i].HP != before[i] {
			t.Errorf("party %d took damage on an enemy miss: HP %d -> %d", i, before[i], g.Party[i].HP)
		}
	}
}

// TestBeginEnemyAttack_MissSkipsDefendBar: a forced-miss enemy turn suppresses the defend
// bar (Timing pre-resolved, miss flag set) so the player never plays the input minigame.
func TestBeginEnemyAttack_MissSkipsDefendBar(t *testing.T) {
	g := newTestState()
	stats := core.EffectiveEnemyStats(&core.BattleMembers(g)[0])

	// Find a seed whose first draw whiffs; accuracy caps below 1 so one always exists.
	seed := int64(1)
	for ; seed < 10000; seed++ {
		if !core.RollEnemyHit(rand.New(rand.NewSource(seed)), stats) {
			break
		}
	}
	g.RNG = rand.New(rand.NewSource(seed))

	beginEnemyAttack(g, 0)

	if !g.Battle.EnemyAttackMisses {
		t.Fatal("expected EnemyAttackMisses set on a whiffed accuracy roll")
	}
	if !g.Battle.Timing.Resolved {
		t.Error("defend bar armed on a miss — Timing should be pre-resolved so the input game is skipped")
	}
}

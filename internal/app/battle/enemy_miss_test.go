package battle

import (
	"math/rand"
	"testing"

	"crawler/internal/app/core"
)

// TestResolveEnemyMiss_DealsNoDamage confirms a whiffed enemy attack leaves the
// whole party's HP untouched (no damage, no status) while still resolving the
// turn cleanly.
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

// TestBeginEnemyAttack_MissSkipsDefendBar drives a forced-miss enemy turn and
// asserts the defend bar is suppressed (Timing pre-resolved, the miss flag set)
// — the player never enters the input minigame for a swing that can't land. The
// miss is forced by re-seeding the battle RNG until the first RollEnemyHit fails
// for the test enemy's accuracy.
func TestBeginEnemyAttack_MissSkipsDefendBar(t *testing.T) {
	g := newTestState()
	stats := core.EffectiveEnemyStats(core.BattleMembers(g)[0])

	// Find a seed whose first Float64 draw whiffs (>= hit chance). Accuracy caps
	// below 1, so such a seed always exists; bound the search defensively.
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

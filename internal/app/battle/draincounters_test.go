package battle

import (
	"testing"

	"crawler/internal/app/core"
)

// TestTurnEndDrainCounters verifies that every partyDeathStatuses / enemyDeathStatuses
// counter classified drainTurnEnd actually ticks down via the end-of-turn drain path.
// The startup assert only forces a kept-on-death counter to DECLARE a drain path; this
// closes the loop for the drainTurnEnd class by proving the hand-maintained
// drainNonDamaging* tick lists really drain each one (so a counter can't be classified
// drainTurnEnd yet forgotten in the tick list).
func TestTurnEndDrainCounters(t *testing.T) {
	partyActor := core.ActorRef{IsParty: true, Index: 0}
	for _, s := range partyDeathStatuses {
		if s.drain != drainTurnEnd {
			continue
		}
		g := newTestState()
		g.Party[0].HP = g.Party[0].MaxHP
		*s.ptr(&g.Party[0]) = 2
		drainNonDamagingPartyStatuses(g, partyActor)
		if got := *s.ptr(&g.Party[0]); got != 1 {
			t.Errorf("party %s is classified drainTurnEnd but did not tick down at turn end (got %d, want 1) — wire it into drainNonDamagingPartyStatuses", s.field, got)
		}
	}

	enemyActor := core.ActorRef{IsParty: false, Index: 0}
	for _, s := range enemyDeathStatuses {
		if s.drain != drainTurnEnd {
			continue
		}
		g := newTestState()
		*s.ptr(&g.Packs[0].Members[0]) = 2
		drainNonDamagingEnemyStatuses(g, enemyActor)
		if got := *s.ptr(&g.Packs[0].Members[0]); got != 1 {
			t.Errorf("enemy %s is classified drainTurnEnd but did not tick down at turn end (got %d, want 1) — wire it into drainNonDamagingEnemyStatuses", s.field, got)
		}
	}
}

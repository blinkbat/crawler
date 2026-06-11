package battle

import (
	"testing"

	"crawler/internal/app/core"
)

// TestActorAppearsBefore covers the skip-loop dedup helper directly: exact
// {IsParty,Index} identity, only slots strictly before the cursor, and a
// cursor past the queue end clamps rather than panics.
func TestActorAppearsBefore(t *testing.T) {
	q := []core.ActorRef{
		{IsParty: true, Index: 2},
		{IsParty: false, Index: 0},
		{IsParty: true, Index: 2},
	}
	if !actorAppearsBefore(q, 2, core.ActorRef{IsParty: true, Index: 2}) {
		t.Error("party slot 2 appears at index 0, should be found before cursor 2")
	}
	if actorAppearsBefore(q, 1, core.ActorRef{IsParty: false, Index: 0}) {
		t.Error("enemy slot 0 is AT cursor 1, not before it — must not match")
	}
	if actorAppearsBefore(q, 0, core.ActorRef{IsParty: true, Index: 2}) {
		t.Error("nothing precedes cursor 0")
	}
	if !actorAppearsBefore(q, 99, core.ActorRef{IsParty: true, Index: 2}) {
		t.Error("an over-long cursor must clamp to len and still find the actor")
	}
}

// TestIngestedPoisonTicksOncePerRound is the regression guard for the skip-loop
// poison double-tick: a poisoned party member ingested mid-round can legally
// hold MULTIPLE queue slots in one round (ATB carry-over). Their poison must
// tick exactly ONCE that round, not once per skipped slot.
func TestIngestedPoisonTicksOncePerRound(t *testing.T) {
	g := newTestState()
	// Nyx (slot 2): poisoned, swallowed, and holding TWO slots this round.
	g.Party[2].PoisonTurns = 3
	g.Party[2].HP = 8
	g.Party[2].Ingested = true
	g.Party[2].IngestedBy = 0
	// Two Nyx slots, then a living actor (Vex, slot 0) to break the skip loop
	// before it runs off the end into beginNewRound.
	g.Battle.Queue = []core.ActorRef{
		{IsParty: true, Index: 2},
		{IsParty: true, Index: 2},
		{IsParty: true, Index: 0},
	}
	g.Battle.QueueCursor = 0

	startActorTurn(g)

	if g.Party[2].PoisonTurns != 2 {
		t.Errorf("ingested member poison drained to %d, want 2 (one tick per round, not one per queue slot)", g.Party[2].PoisonTurns)
	}
	// And the tick must actually deal damage. Regression guard: the poison tick
	// routed through damagePartyMember, whose ingested-lockout early-return
	// zeroed the damage — the counter drained while HP stayed put, making ingest
	// a free poison escape. Assert HP actually fell (poison ticks on ingested
	// prey by design).
	if g.Party[2].HP >= 8 {
		t.Errorf("ingested member HP stayed at %d after a poison tick — ingest must not zero the DoT (the counter drained but no damage landed)", g.Party[2].HP)
	}
}

package battle

import (
	"testing"

	"crawler/internal/app/core"
)

// TestApplyBless_BuffsLivingPartyAndSkipsDownedIngested verifies the party-
// wide apply: every living, non-ingested member gets the stat buff (the caster
// included), and downed / ingested members are skipped. Checks a member that
// is neither the caster nor the queue-cursor actor so finishActorTurn's
// end-of-turn drain doesn't muddy the asserted counter.
func TestApplyBless_BuffsLivingPartyAndSkipsDownedIngested(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 1 // Mira (cleric) casts
	g.Party[0].HP = 0         // Vex is down (also the queue-cursor actor)
	g.Party[2].Ingested = true

	solBaseINT := core.EffectiveStats(g.Party[3]).INT

	if !applyBless(g, core.TimingQualityGreat) {
		t.Fatal("applyBless reported not-landed")
	}

	// Sol (slot 3) is living and not the drained queue actor → full buff.
	sol := g.Party[3]
	if sol.BuffTurns != core.BlessBuffTurns {
		t.Errorf("Sol BuffTurns = %d, want %d", sol.BuffTurns, core.BlessBuffTurns)
	}
	if sol.BuffStats.INT != core.BlessBuffPerStat {
		t.Errorf("Sol BuffStats.INT = %d, want %d", sol.BuffStats.INT, core.BlessBuffPerStat)
	}
	if got := core.EffectiveStats(sol).INT; got != solBaseINT+core.BlessBuffPerStat {
		t.Errorf("Sol EffectiveStats.INT = %d, want %d", got, solBaseINT+core.BlessBuffPerStat)
	}
	// The caster (Mira, slot 1) is included in the party-wide buff — and gets one
	// EXTRA turn at apply time to absorb the end-of-turn self-drain that, in real
	// play, ticks the caster's copy down once immediately (the caster is the
	// current actor). This isolated test routes finishActorTurn's drain onto the
	// downed queue-cursor actor (Vex, slot 0) instead, so it observes the
	// PRE-drain value: BlessBuffTurns+1.
	if g.Party[1].BuffTurns != core.BlessBuffTurns+1 {
		t.Errorf("caster BuffTurns = %d, want %d (BlessBuffTurns+1, pre-self-drain)", g.Party[1].BuffTurns, core.BlessBuffTurns+1)
	}
	// And that extra turn nets out: one self-drain (what finishActorTurn does to
	// the caster on the cast turn in real play) leaves the caster at exactly
	// BlessBuffTurns — the same useful duration the allies got, not one fewer.
	tickBlessAfterPartyTurn(g, core.ActorRef{IsParty: true, Index: 1})
	if g.Party[1].BuffTurns != core.BlessBuffTurns {
		t.Errorf("caster BuffTurns after one self-drain = %d, want %d (must match allies, not be one short)", g.Party[1].BuffTurns, core.BlessBuffTurns)
	}
	// Downed and ingested members get nothing.
	if g.Party[0].BuffTurns != 0 {
		t.Errorf("downed Vex got BuffTurns %d, want 0", g.Party[0].BuffTurns)
	}
	if g.Party[2].BuffTurns != 0 {
		t.Errorf("ingested Nyx got BuffTurns %d, want 0", g.Party[2].BuffTurns)
	}
}

// TestTickBlessAfterPartyTurn_DrainsBuff confirms the buff counts down one per
// the member's turn and the magnitude stays live until the counter hits zero.
func TestTickBlessAfterPartyTurn_DrainsBuff(t *testing.T) {
	g := newTestState()
	g.Party[3].BuffTurns = 2
	g.Party[3].BuffStats = core.Stats{STR: 1, DEX: 1, INT: 1, WIS: 1}
	actor := core.ActorRef{IsParty: true, Index: 3}

	tickBlessAfterPartyTurn(g, actor)
	if g.Party[3].BuffTurns != 1 {
		t.Fatalf("after one tick BuffTurns = %d, want 1", g.Party[3].BuffTurns)
	}
	// Stats stay boosted while the counter still runs.
	if got := core.EffectiveStats(g.Party[3]).STR; got != g.Party[3].Stats.STR+1 {
		t.Errorf("buff dropped early at 1 turn left: EffectiveStats.STR = %d", got)
	}

	tickBlessAfterPartyTurn(g, actor)
	if g.Party[3].BuffTurns != 0 {
		t.Errorf("after second tick BuffTurns = %d, want 0", g.Party[3].BuffTurns)
	}
	// Counter expired → the boost no longer applies.
	if got := core.EffectiveStats(g.Party[3]).STR; got != g.Party[3].Stats.STR {
		t.Errorf("expired buff still boosting: EffectiveStats.STR = %d, want base %d", got, g.Party[3].Stats.STR)
	}
}

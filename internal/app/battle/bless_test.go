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
	if got := core.MaxStatusModTurns(sol.Buffs); got != core.BlessBuffTurns {
		t.Errorf("Sol buff turns = %d, want %d", got, core.BlessBuffTurns)
	}
	if got := partyBuffStats(sol).INT; got != core.BlessBuffPerStat {
		t.Errorf("Sol buff INT = %d, want %d", got, core.BlessBuffPerStat)
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
	if got := core.MaxStatusModTurns(g.Party[1].Buffs); got != core.BlessBuffTurns+1 {
		t.Errorf("caster buff turns = %d, want %d (BlessBuffTurns+1, pre-self-drain)", got, core.BlessBuffTurns+1)
	}
	// And that extra turn nets out: one self-drain (what finishActorTurn does to
	// the caster on the cast turn in real play) leaves the caster at exactly
	// BlessBuffTurns — the same useful duration the allies got, not one fewer.
	tickBlessAfterPartyTurn(g, core.ActorRef{IsParty: true, Index: 1})
	if got := core.MaxStatusModTurns(g.Party[1].Buffs); got != core.BlessBuffTurns {
		t.Errorf("caster buff turns after one self-drain = %d, want %d (must match allies, not be one short)", got, core.BlessBuffTurns)
	}
	// Downed and ingested members get nothing.
	if len(g.Party[0].Buffs) != 0 {
		t.Errorf("downed Vex got a buff: %+v, want none", g.Party[0].Buffs)
	}
	if len(g.Party[2].Buffs) != 0 {
		t.Errorf("ingested Nyx got a buff: %+v, want none", g.Party[2].Buffs)
	}
}

// TestTickBlessAfterPartyTurn_DrainsBuff confirms the buff counts down one per
// the member's turn and the magnitude stays live until the counter hits zero.
func TestTickBlessAfterPartyTurn_DrainsBuff(t *testing.T) {
	g := newTestState()
	core.StampPartyBuff(&g.Party[3], core.SkillBless, core.SkillEffect{BuffStats: core.Stats{STR: 1, DEX: 1, INT: 1, WIS: 1}, BuffTurns: 2})
	actor := core.ActorRef{IsParty: true, Index: 3}

	tickBlessAfterPartyTurn(g, actor)
	if got := core.MaxStatusModTurns(g.Party[3].Buffs); got != 1 {
		t.Fatalf("after one tick buff turns = %d, want 1", got)
	}
	// Stats stay boosted while the counter still runs.
	if got := core.EffectiveStats(g.Party[3]).STR; got != g.Party[3].Stats.STR+1 {
		t.Errorf("buff dropped early at 1 turn left: EffectiveStats.STR = %d", got)
	}

	tickBlessAfterPartyTurn(g, actor)
	if got := core.MaxStatusModTurns(g.Party[3].Buffs); got != 0 {
		t.Errorf("after second tick buff turns = %d, want 0", got)
	}
	// Counter expired → the boost no longer applies.
	if got := core.EffectiveStats(g.Party[3]).STR; got != g.Party[3].Stats.STR {
		t.Errorf("expired buff still boosting: EffectiveStats.STR = %d, want base %d", got, g.Party[3].Stats.STR)
	}
}

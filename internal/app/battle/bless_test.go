package battle

import (
	"testing"

	"crawler/internal/app/core"
)

// TestApplyBless_BuffsLivingPartyAndSkipsDownedIngested: every living, non-ingested member
// (caster included) gets the buff; downed/ingested are skipped. Asserts on slot 3 (not caster, not queue actor).
func TestApplyBless_BuffsLivingPartyAndSkipsDownedIngested(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 1
	g.Party[0].HP = 0 // down (also the queue-cursor actor)
	g.Party[2].Ingested = true

	solBaseINT := core.EffectiveStats(g.Party[3]).INT

	if !applyBless(g, core.TimingQualityGreat) {
		t.Fatal("applyBless reported not-landed")
	}

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
	// The caster (slot 1) gets one EXTRA turn to absorb the immediate self-drain; this test routes
	// the drain onto downed slot 0, so it sees the pre-drain value BlessBuffTurns+1.
	if got := core.MaxStatusModTurns(g.Party[1].Buffs); got != core.BlessBuffTurns+1 {
		t.Errorf("caster buff turns = %d, want %d (BlessBuffTurns+1, pre-self-drain)", got, core.BlessBuffTurns+1)
	}
	// One self-drain nets the caster back to BlessBuffTurns — same duration the allies got.
	tickBlessAfterPartyTurn(g, core.ActorRef{IsParty: true, Index: 1})
	if got := core.MaxStatusModTurns(g.Party[1].Buffs); got != core.BlessBuffTurns {
		t.Errorf("caster buff turns after one self-drain = %d, want %d (must match allies, not be one short)", got, core.BlessBuffTurns)
	}
	if len(g.Party[0].Buffs) != 0 {
		t.Errorf("downed Vex got a buff: %+v, want none", g.Party[0].Buffs)
	}
	if len(g.Party[2].Buffs) != 0 {
		t.Errorf("ingested Nyx got a buff: %+v, want none", g.Party[2].Buffs)
	}
}

// TestTickBlessAfterPartyTurn_DrainsBuff: the buff counts down one per turn; magnitude stays live until zero.
func TestTickBlessAfterPartyTurn_DrainsBuff(t *testing.T) {
	g := newTestState()
	core.StampPartyBuff(&g.Party[3], core.SkillBless, core.SkillEffect{BuffStats: core.Stats{STR: 1, DEX: 1, INT: 1, WIS: 1}, BuffTurns: 2})
	actor := core.ActorRef{IsParty: true, Index: 3}

	tickBlessAfterPartyTurn(g, actor)
	if got := core.MaxStatusModTurns(g.Party[3].Buffs); got != 1 {
		t.Fatalf("after one tick buff turns = %d, want 1", got)
	}
	if got := core.EffectiveStats(g.Party[3]).STR; got != g.Party[3].Stats.STR+1 {
		t.Errorf("buff dropped early at 1 turn left: EffectiveStats.STR = %d", got)
	}

	tickBlessAfterPartyTurn(g, actor)
	if got := core.MaxStatusModTurns(g.Party[3].Buffs); got != 0 {
		t.Errorf("after second tick buff turns = %d, want 0", got)
	}
	if got := core.EffectiveStats(g.Party[3]).STR; got != g.Party[3].Stats.STR {
		t.Errorf("expired buff still boosting: EffectiveStats.STR = %d, want base %d", got, g.Party[3].Stats.STR)
	}
}

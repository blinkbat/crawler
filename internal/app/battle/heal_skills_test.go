package battle

import (
	"testing"

	"crawler/internal/app/core"
)

// TestApplySecondWind_HealsCaster: the Warrior's flat self-heal restores caster HP (no target pick).
func TestApplySecondWind_HealsCaster(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 0
	g.Party[0].MaxHP = 20
	g.Party[0].HP = 3

	applySecondWind(g, core.TimingQualityGood)

	if g.Party[0].HP <= 3 {
		t.Errorf("Second Wind did not heal the caster (HP=%d)", g.Party[0].HP)
	}
}

// TestApplyRenewal_StampsRegenThenTicks: Renewal stamps a regen and the end-of-turn tick heals, decrements, and fades.
func TestApplyRenewal_StampsRegenThenTicks(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 1 // cleric casts
	g.Battle.PartyTarget = 3  // slot 3, not the drained queue actor
	g.Party[3].MaxHP = 40
	g.Party[3].HP = 10

	applyRenewal(g, core.TimingQualityGood)

	if g.Party[3].RegenTurns <= 0 {
		t.Fatalf("Renewal did not stamp RegenTurns (got %d)", g.Party[3].RegenTurns)
	}
	if g.Party[3].RegenPerTurn <= 0 {
		t.Fatalf("Renewal did not snapshot RegenPerTurn (got %d)", g.Party[3].RegenPerTurn)
	}

	turns := g.Party[3].RegenTurns
	hp := g.Party[3].HP
	actor := core.ActorRef{IsParty: true, Index: 3}
	tickRegenAfterPartyTurn(g, actor)
	if g.Party[3].RegenTurns != turns-1 {
		t.Errorf("regen counter = %d after one tick, want %d", g.Party[3].RegenTurns, turns-1)
	}
	if g.Party[3].HP <= hp {
		t.Errorf("regen tick did not heal (HP %d -> %d)", hp, g.Party[3].HP)
	}

	for g.Party[3].RegenTurns > 0 {
		tickRegenAfterPartyTurn(g, actor)
	}
	if g.Party[3].RegenTurns != 0 {
		t.Errorf("regen did not end at 0 (got %d)", g.Party[3].RegenTurns)
	}
	// A tick on expired regen is a no-op (no underflow).
	tickRegenAfterPartyTurn(g, actor)
	if g.Party[3].RegenTurns != 0 {
		t.Errorf("regen counter went negative: %d", g.Party[3].RegenTurns)
	}
}

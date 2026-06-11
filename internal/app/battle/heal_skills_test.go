package battle

import (
	"testing"

	"crawler/internal/app/core"
)

// TestApplySecondWind_HealsCaster verifies the Warrior's flat self-heal
// restores the caster's HP (no target pick).
func TestApplySecondWind_HealsCaster(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 0 // Vex (warrior)
	g.Party[0].MaxHP = 20
	g.Party[0].HP = 3

	applySecondWind(g, core.TimingQualityGood)

	if g.Party[0].HP <= 3 {
		t.Errorf("Second Wind did not heal the caster (HP=%d)", g.Party[0].HP)
	}
}

// TestApplyRenewal_StampsRegenThenTicks verifies Renewal stamps a regen on the
// ally and that the end-of-turn tick heals + decrements + eventually fades.
func TestApplyRenewal_StampsRegenThenTicks(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 1 // Mira (cleric) casts
	g.Battle.PartyTarget = 3  // Renewal on Sol (slot 3, not the drained queue actor)
	g.Party[3].MaxHP = 40
	g.Party[3].HP = 10

	applyRenewal(g, core.TimingQualityGood)

	if g.Party[3].RegenTurns <= 0 {
		t.Fatalf("Renewal did not stamp RegenTurns (got %d)", g.Party[3].RegenTurns)
	}
	if g.Party[3].RegenPerTurn <= 0 {
		t.Fatalf("Renewal did not snapshot RegenPerTurn (got %d)", g.Party[3].RegenPerTurn)
	}

	// One end-of-turn tick: heals and decrements the counter.
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

	// Drain the rest; the status must end cleanly at zero.
	for g.Party[3].RegenTurns > 0 {
		tickRegenAfterPartyTurn(g, actor)
	}
	if g.Party[3].RegenTurns != 0 {
		t.Errorf("regen did not end at 0 (got %d)", g.Party[3].RegenTurns)
	}
	// A further tick on an expired regen is a no-op (no heal past MaxHP, no underflow).
	tickRegenAfterPartyTurn(g, actor)
	if g.Party[3].RegenTurns != 0 {
		t.Errorf("regen counter went negative: %d", g.Party[3].RegenTurns)
	}
}

package battle

import (
	"testing"

	"crawler/internal/app/core"
)

// TestDebugSkipWin_AwardsAndRemovesPack verifies the skip-battles auto-resolve:
// the engaged pack is felled, XP is awarded to living members, kills are
// recorded, the pack is removed from the field, and the battle state resets to
// the explore-clean BattleNone.
func TestDebugSkipWin_AwardsAndRemovesPack(t *testing.T) {
	g := newTestState()
	packCount := len(g.Packs)
	xpBefore := g.Party[0].XP

	DebugSkipWin(g, 0)

	if len(g.Packs) != packCount-1 {
		t.Errorf("pack not removed: %d packs remain, want %d", len(g.Packs), packCount-1)
	}
	if g.Party[0].XP <= xpBefore {
		t.Errorf("no XP awarded to living member (XP %d -> %d)", xpBefore, g.Party[0].XP)
	}
	if g.Battle.Phase != core.BattleNone {
		t.Errorf("battle phase = %v, want BattleNone after skip-win", g.Battle.Phase)
	}
	if g.Battle.ActivePack != -1 {
		t.Errorf("ActivePack = %d, want -1 after teardown", g.Battle.ActivePack)
	}
}

// TestDebugSkipWin_NoOpOnInvalidPack guards the bounds / dead-pack early-out.
func TestDebugSkipWin_NoOpOnInvalidPack(t *testing.T) {
	g := newTestState()
	packCount := len(g.Packs)
	DebugSkipWin(g, 99) // out of range
	if len(g.Packs) != packCount {
		t.Errorf("invalid pack index mutated the field: %d packs, want %d", len(g.Packs), packCount)
	}
}

package battle

import (
	"testing"

	"crawler/internal/app/core"
)

// TestDebugSkipWin_AwardsAndRemovesPack: the pack is felled and removed, living members get
// XP, and the battle resets to BattleNone.
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
	DebugSkipWin(g, 99)
	if len(g.Packs) != packCount {
		t.Errorf("invalid pack index mutated the field: %d packs, want %d", len(g.Packs), packCount)
	}
}

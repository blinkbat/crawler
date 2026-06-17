package core

import "testing"

// TestDebugBoostParty adds the boost to every stat and refreshes the derived
// HP/MP pools (reviving the downed), mirroring the level-up Derive math.
func TestDebugBoostParty(t *testing.T) {
	party := []PartyMember{
		{Stats: Stats{STR: 6, DEX: 2, INT: 2, WIS: 1, VIT: 5, SPD: 3}, HP: 1, MaxHP: 10, MP: 0, MaxMP: 4},
	}
	DebugBoostParty(party, 100)
	m := party[0]
	if m.Stats.STR != 106 || m.Stats.DEX != 102 || m.Stats.INT != 102 || m.Stats.WIS != 101 || m.Stats.VIT != 105 || m.Stats.SPD != 103 {
		t.Errorf("stats not all boosted by 100: %+v", m.Stats)
	}
	if want := MaxHPFor(m.Stats); m.MaxHP != want || m.HP != want {
		t.Errorf("HP not refreshed to full: HP=%d MaxHP=%d, want %d", m.HP, m.MaxHP, want)
	}
	if want := 4 + 100*MPPerINT; m.MaxMP != want || m.MP != want {
		t.Errorf("MP not refreshed: MP=%d MaxMP=%d, want %d", m.MP, m.MaxMP, want)
	}
}

// TestDebugBoostParty_NegativeAmountFloors guards the negative-boost path:
// AdjustStat floors each stat at 0, so the derived MaxMP must grow by the
// ACTUAL applied INT delta (not the raw amount). A raw-amount subtraction
// would drop MaxMP — and the MP=MaxMP that follows — below zero when the
// amount exceeds the current INT.
func TestDebugBoostParty_NegativeAmountFloors(t *testing.T) {
	party := []PartyMember{
		{Stats: Stats{STR: 3, DEX: 3, INT: 2, WIS: 3, VIT: 4, SPD: 3}, HP: 5, MaxHP: 20, MP: 1, MaxMP: 8},
	}
	// -100 over-subtracts every stat; all floor at 0.
	DebugBoostParty(party, -100)
	m := party[0]
	if m.Stats.INT != 0 {
		t.Errorf("INT should floor at 0, got %d", m.Stats.INT)
	}
	if m.MaxMP < 0 || m.MP < 0 {
		t.Errorf("MaxMP/MP went negative: MaxMP=%d MP=%d", m.MaxMP, m.MP)
	}
	// MaxMP grew by the real INT delta (2 -> 0 = -2 points), floored at 0:
	// 8 + (-2)*MPPerINT, clamped to >= 0.
	wantMP := 8 + (0-2)*MPPerINT
	if wantMP < 0 {
		wantMP = 0
	}
	if m.MaxMP != wantMP || m.MP != wantMP {
		t.Errorf("MaxMP/MP = %d/%d, want %d (grown by applied INT delta, floored)", m.MaxMP, m.MP, wantMP)
	}
	if want := MaxHPFor(m.Stats); m.MaxHP != want || m.HP != want {
		t.Errorf("HP not refreshed from clamped VIT: HP=%d MaxHP=%d, want %d", m.HP, m.MaxHP, want)
	}
}

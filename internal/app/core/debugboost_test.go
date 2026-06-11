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

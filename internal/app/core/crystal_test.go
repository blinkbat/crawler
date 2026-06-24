package core

import "testing"

// TestTickCrystalRecharge_ReArmsAtCeiling: re-arms at CrystalRechargeSteps, never over-charges.
func TestTickCrystalRecharge_ReArmsAtCeiling(t *testing.T) {
	g := &GameState{Crystals: []Crystal{
		{Charge: CrystalRechargeSteps - 2, Charged: false},
		{Charge: CrystalRechargeSteps, Charged: true},
	}}

	TickCrystalRecharge(g)
	if g.Crystals[0].Charged {
		t.Fatalf("crystal re-armed one step early (charge %d)", g.Crystals[0].Charge)
	}
	if g.Crystals[0].Charge != CrystalRechargeSteps-1 {
		t.Fatalf("charge = %d, want %d", g.Crystals[0].Charge, CrystalRechargeSteps-1)
	}

	TickCrystalRecharge(g)
	if !g.Crystals[0].Charged || g.Crystals[0].Charge != CrystalRechargeSteps {
		t.Fatalf("crystal should re-arm at the ceiling, got charge=%d charged=%v", g.Crystals[0].Charge, g.Crystals[0].Charged)
	}
	if g.Crystals[1].Charge != CrystalRechargeSteps {
		t.Fatalf("an already-charged crystal was over-charged to %d", g.Crystals[1].Charge)
	}
}

// TestAdjacentChargedCrystalIndex: a charged crystal fires on it or a cardinal neighbor only.
func TestAdjacentChargedCrystalIndex(t *testing.T) {
	crystals := []Crystal{
		{TileX: 5, TileZ: 5, Charged: false},
		{TileX: 3, TileZ: 3, Charged: true},
	}
	cases := []struct {
		x, z, want int
		note       string
	}{
		{5, 5, -1, "dormant crystal under the player must not fire"},
		{3, 3, 1, "charged crystal under the player fires"},
		{3, 4, 1, "charged crystal one tile away fires"},
		{4, 4, -1, "charged crystal on a diagonal (dist 2) does not fire"},
		{0, 0, -1, "distant charged crystal does not fire"},
	}
	for _, c := range cases {
		if got := AdjacentChargedCrystalIndex(crystals, c.x, c.z); got != c.want {
			t.Errorf("AdjacentChargedCrystalIndex(%d,%d) = %d, want %d — %s", c.x, c.z, got, c.want, c.note)
		}
	}
}

// TestRestorePartyFully fully restores every member to max HP+MP, REVIVING the dead.
func TestRestorePartyFully(t *testing.T) {
	g := &GameState{Party: []PartyMember{
		{HP: 1, MaxHP: 10, MP: 0, MaxMP: 5},
		{HP: 0, MaxHP: 10, MP: 0, MaxMP: 5}, // downed → revived
	}}
	if n := RestorePartyFully(g); n != 2 {
		t.Fatalf("restored %d members, want 2 (all, including the downed)", n)
	}
	for i := range g.Party {
		m := g.Party[i]
		if m.HP != m.MaxHP || m.MP != m.MaxMP {
			t.Errorf("member %d not fully restored: HP=%d/%d MP=%d/%d", i, m.HP, m.MaxHP, m.MP, m.MaxMP)
		}
	}
}

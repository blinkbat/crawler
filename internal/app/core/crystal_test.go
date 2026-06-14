package core

import "testing"

// TestTickCrystalRecharge_ReArmsAtCeiling pins the per-step recharge: a dormant
// crystal climbs one charge per call and re-arms exactly at CrystalRechargeSteps,
// while an already-charged crystal is left untouched (not over-charged).
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

// TestAdjacentChargedCrystalIndex covers the trigger reach: a charged crystal
// fires when the player is on it or a cardinal neighbor; dormant or distant
// crystals never fire.
func TestAdjacentChargedCrystalIndex(t *testing.T) {
	crystals := []Crystal{
		{TileX: 5, TileZ: 5, Charged: false}, // dormant
		{TileX: 3, TileZ: 3, Charged: true},  // charged
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

// TestRestorePartyFully tops living members to full HP+MP and skips the
// dead / ingested (the healing crystal's effect).
func TestRestorePartyFully(t *testing.T) {
	g := &GameState{Party: []PartyMember{
		{HP: 1, MaxHP: 10, MP: 0, MaxMP: 5},
		{HP: 0, MaxHP: 10, MP: 0, MaxMP: 5},                // down — skipped
		{HP: 3, MaxHP: 8, MP: 2, MaxMP: 9, Ingested: true}, // ingested — skipped
	}}
	if n := RestorePartyFully(g); n != 1 {
		t.Fatalf("restored %d members, want 1 (living, non-ingested only)", n)
	}
	if g.Party[0].HP != 10 || g.Party[0].MP != 5 {
		t.Errorf("living member not fully restored: HP=%d MP=%d", g.Party[0].HP, g.Party[0].MP)
	}
	if g.Party[1].HP != 0 {
		t.Errorf("a downed member was revived (HP=%d)", g.Party[1].HP)
	}
	if g.Party[2].HP != 3 || g.Party[2].MP != 2 {
		t.Errorf("an ingested member was healed: HP=%d MP=%d", g.Party[2].HP, g.Party[2].MP)
	}
}

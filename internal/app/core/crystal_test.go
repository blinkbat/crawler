package core

import "testing"

// TestRestorePackFull_DropsSummonsRevivesAuthored: a flee reset drops mid-fight
// summons (so the pack reverts to its authored roster) and revives + full-heals the
// rest with statuses cleared.
func TestRestorePackFull_DropsSummonsRevivesAuthored(t *testing.T) {
	pack := &Pack{Members: []Enemy{
		{Kind: EnemyRat, HP: 1, MaxHP: 10, Alive: true, PoisonTurns: 2},
		{Kind: EnemySkeleton, HP: 0, MaxHP: 8, Alive: false, Summoned: true}, // raised mid-fight
		{Kind: EnemyRat, HP: 0, MaxHP: 10, Alive: false},                     // authored, downed
	}}
	RestorePackFull(pack)
	if len(pack.Members) != 2 {
		t.Fatalf("summoned member not dropped: %d members, want 2", len(pack.Members))
	}
	for i := range pack.Members {
		m := pack.Members[i]
		if m.Summoned {
			t.Errorf("member %d still marked summoned", i)
		}
		if !m.Alive || m.HP != m.MaxHP || m.PoisonTurns != 0 {
			t.Errorf("member %d not fully restored: alive=%v HP=%d/%d poison=%d", i, m.Alive, m.HP, m.MaxHP, m.PoisonTurns)
		}
	}
}

// TestRestorePackFull_ReSeatsFrontRowCap: reviving a pack after a flee must not
// leave more than EnemyFrontRowCap members in the front row. Combat promotes
// back→front on deaths but never demotes, so a downed front member (still RowFront)
// plus the back member promoted to cover it are ALL front-row after a naive revive.
// RestorePackFull resets everyone to back then re-shunts, capping the front row.
func TestRestorePackFull_ReSeatsFrontRowCap(t *testing.T) {
	// Authored 2 front + 1 back; mid-fight a front foe died and the back foe was
	// promoted, so at flee time three members carry RowFront (one dead).
	pack := &Pack{Members: []Enemy{
		{Kind: EnemyRat, MaxHP: 10, HP: 0, Alive: false, Row: RowFront}, // died in front
		{Kind: EnemyRat, MaxHP: 10, HP: 4, Alive: true, Row: RowFront},  // living front
		{Kind: EnemyRat, MaxHP: 10, HP: 6, Alive: true, Row: RowFront},  // promoted from back
	}}
	RestorePackFull(pack)
	front := 0
	for i := range pack.Members {
		if pack.Members[i].Alive && pack.Members[i].Row == RowFront {
			front++
		}
	}
	if front > EnemyFrontRowCap {
		t.Fatalf("front row over cap after restore: %d living front, cap %d", front, EnemyFrontRowCap)
	}
}

// TestDropCrystalsOnPacks: a crystal sharing a pack's tile is removed (both block).
func TestDropCrystalsOnPacks(t *testing.T) {
	packs := []Pack{{TileX: 3, TileZ: 4, Members: []Enemy{{Kind: EnemyRat, Alive: true}}}}
	crystals := []Crystal{
		{TileX: 1, TileZ: 1, Charged: true}, // clear
		{TileX: 3, TileZ: 4, Charged: true}, // on the pack → dropped
	}
	got := dropCrystalsOnPacks(crystals, packs, false)
	if len(got) != 1 || got[0].TileX != 1 || got[0].TileZ != 1 {
		t.Fatalf("expected only the (1,1) crystal to survive, got %+v", got)
	}
}

// TestDropCrystalsOnPacks_VoxelLevelAware: on a voxel map a crystal only drops when
// a pack shares its FLOOR — a crystal on a deck above a ground-level pack survives.
func TestDropCrystalsOnPacks_VoxelLevelAware(t *testing.T) {
	packs := []Pack{{TileX: 3, TileZ: 4, Level: 0, Members: []Enemy{{Kind: EnemyRat, Alive: true}}}}
	crystals := []Crystal{
		{TileX: 3, TileZ: 4, Level: 2, Charged: true}, // deck above the pack → kept
		{TileX: 3, TileZ: 4, Level: 0, Charged: true}, // same floor as the pack → dropped
	}
	got := dropCrystalsOnPacks(crystals, packs, true)
	if len(got) != 1 || got[0].Level != 2 {
		t.Fatalf("expected only the deck (level 2) crystal to survive, got %+v", got)
	}
}

// TestTickCrystalSpins_DecaysToZero: the touch-armed fast spin drains by dt each
// frame and floors at 0 (no negative countdown that would replay the burst).
func TestTickCrystalSpins_DecaysToZero(t *testing.T) {
	g := &GameState{Crystals: []Crystal{
		{SpinBurst: CrystalSpinBurstDuration},
		{SpinBurst: 0}, // idle crystal stays at 0
	}}
	TickCrystalSpins(g, 0.1)
	if g.Crystals[0].SpinBurst != CrystalSpinBurstDuration-0.1 {
		t.Fatalf("burst did not decay by dt: got %v, want %v", g.Crystals[0].SpinBurst, CrystalSpinBurstDuration-0.1)
	}
	if g.Crystals[1].SpinBurst != 0 {
		t.Errorf("idle crystal spin went non-zero: %v", g.Crystals[1].SpinBurst)
	}
	TickCrystalSpins(g, CrystalSpinBurstDuration) // overshoot
	if g.Crystals[0].SpinBurst != 0 {
		t.Errorf("burst floored below 0: %v", g.Crystals[0].SpinBurst)
	}
}

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

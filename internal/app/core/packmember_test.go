package core

import "testing"

// TestSkillOnDiskNameRoundTrip asserts SkillIDFromOnDiskName is the exact inverse of SkillOnDiskName
// for every SkillID, so a name with an unexpected glyph fires here, not by silently losing loadouts on load.
func TestSkillOnDiskNameRoundTrip(t *testing.T) {
	ids := AllSkillIDs()
	if len(ids) == 0 {
		t.Fatal("AllSkillIDs returned no entries; the registry can't have been wired up correctly")
	}
	for _, id := range ids {
		name := SkillOnDiskName(id)
		if name == "" {
			t.Errorf("SkillOnDiskName(%v) returned empty — every non-None skill must have a stable identifier", id)
			continue
		}
		got, ok := SkillIDFromOnDiskName(name)
		if !ok {
			t.Errorf("SkillIDFromOnDiskName(%q) returned ok=false for round-trip from %v", name, id)
			continue
		}
		if got != id {
			t.Errorf("round-trip mismatch: %v → %q → %v (want %v)", id, name, got, id)
		}
	}
}

// TestSkillOnDiskNameRejectsBlank: a blank/whitespace field must not slip past lookup as SkillNone.
func TestSkillOnDiskNameRejectsBlank(t *testing.T) {
	if _, ok := SkillIDFromOnDiskName(""); ok {
		t.Error("empty string must not round-trip; got ok=true")
	}
	if _, ok := SkillIDFromOnDiskName("   "); ok {
		t.Error("whitespace-only string must not round-trip; got ok=true")
	}
}

// TestClampMapDimension guards the shared editor clamp's [Min, Max] contract.
func TestClampMapDimension(t *testing.T) {
	cases := []struct {
		in, want int
	}{
		{-5, MinMapDimension},
		{0, MinMapDimension},
		{MinMapDimension - 1, MinMapDimension},
		{MinMapDimension, MinMapDimension},
		{MinMapDimension + 1, MinMapDimension + 1},
		{MaxMapDimension, MaxMapDimension},
		{MaxMapDimension + 1, MaxMapDimension},
		{10000, MaxMapDimension},
	}
	for _, tc := range cases {
		if got := ClampMapDimension(tc.in); got != tc.want {
			t.Errorf("ClampMapDimension(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestResetGameStatePreservesProgressionAndRecoversParty(t *testing.T) {
	area := AreaDefinition{
		Width:       3,
		Height:      3,
		Walls:       []string{"...", "...", "..."},
		Floor:       []string{"...", "...", "..."},
		Decor:       []string{"...", "...", "..."},
		Props:       []string{"...", "...", "..."},
		StartTileX:  1,
		StartTileZ:  1,
		StartFacing: East,
	}
	g := NewGameState(area)
	// Clear starter rations so the survives-reset assertion tests just the cheese stamped in.
	g.Inventory = nil
	g.Inventory = AddItem(g.Inventory, ItemCheese, 2)
	g.Party[0].Level = 4
	g.Party[0].XP = 123
	g.Party[0].PendingLevelUps = 3
	g.Party[0].HP = 0
	g.Party[0].MP = 0
	g.Party[0].PoisonTurns = 2
	g.Party[0].Ingested = true
	g.Party[0].IngestedBy = 1

	ResetGameState(&g)

	if g.Party[0].Level != 4 || g.Party[0].XP != 123 || g.Party[0].PendingLevelUps != 3 {
		t.Fatalf("party progression should survive reset: %+v", g.Party[0])
	}
	if g.Party[0].HP != g.Party[0].MaxHP || g.Party[0].MP != g.Party[0].MaxMP {
		t.Fatalf("party should recover HP/MP, got HP %d/%d MP %d/%d",
			g.Party[0].HP, g.Party[0].MaxHP, g.Party[0].MP, g.Party[0].MaxMP)
	}
	if g.Party[0].PoisonTurns != 0 || g.Party[0].Ingested || g.Party[0].IngestedBy != -1 {
		t.Fatalf("battle statuses should clear on reset: %+v", g.Party[0])
	}
	if len(g.Inventory) != 1 || g.Inventory[0].Kind != ItemCheese || g.Inventory[0].Count != 2 {
		t.Fatalf("inventory should survive reset, got %+v", g.Inventory)
	}
}

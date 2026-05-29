package core

import (
	"crawler/internal/app/core/mapfile"
	"testing"
)

// TestSkillOnDiskNameRoundTrip asserts SkillIDFromOnDiskName is the
// exact inverse of SkillOnDiskName for every registered SkillID.
// SkillOnDiskName derives from SkillName via lowercase + space→
// underscore — if a future skill name picks up punctuation or some
// other glyph the inverse map doesn't expect, this test fires before
// a player's mapfile starts silently losing skill loadouts on load.
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

// TestSkillOnDiskNameRejectsBlank guards the bare-empty input path so
// a corrupted mapfile field can't slip past lookup as SkillNone.
func TestSkillOnDiskNameRejectsBlank(t *testing.T) {
	if _, ok := SkillIDFromOnDiskName(""); ok {
		t.Error("empty string must not round-trip; got ok=true")
	}
	if _, ok := SkillIDFromOnDiskName("   "); ok {
		t.Error("whitespace-only string must not round-trip; got ok=true")
	}
}

// TestClampMapDimension guards the shared editor clamp against silent
// regressions — three call sites depend on the [Min, Max] contract.
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

func TestCustomEnemyPackRoundTripAndRuntimeStats(t *testing.T) {
	mf := mapfile.MapFile{
		Name:      "Custom Test",
		Materials: "dungeon",
		Width:     4,
		Height:    4,
		StartX:    1,
		StartZ:    1,
		StartFace: "east",
		Walls:     []string{"....", "....", "....", "...."},
		Floor:     []string{"....", "....", "....", "...."},
		Decor:     []string{"....", "....", "....", "...."},
		Props:     []string{"....", "....", "....", "...."},
		Packs: []mapfile.MapPack{
			{Members: []string{"venom_bat"}, X: 2, Z: 1},
		},
		CustomEnemies: []mapfile.MapCustomEnemy{
			{
				Name:            "venom_bat",
				BaseKind:        "bat",
				HP:              33,
				MP:              5,
				STR:             1,
				DEX:             1,
				INT:             1,
				WIS:             1,
				VIT:             1,
				SPD:             12,
				Armor:           4,
				XPValue:         77,
				Tier:            9,
				AttackDamage:    11,
				SkillCastChance: 1,
				SpellPower:      8,
				Skills:          []string{"sleep"},
			},
		},
	}
	area, err := AreaFromMapFile(mf, "")
	if err != nil {
		t.Fatalf("AreaFromMapFile: %v", err)
	}
	if got := PackMemberCustomName(area.PackSpawns[0], 0); got != "venom_bat" {
		t.Fatalf("pack should retain custom member name, got %q", got)
	}

	g := NewGameState(area)
	if len(g.Packs) != 1 || len(g.Packs[0].Members) != 1 {
		t.Fatalf("expected one spawned custom enemy, got %+v", g.Packs)
	}
	enemy := g.Packs[0].Members[0]
	if !enemy.HasDefinitionOverride {
		t.Fatalf("custom enemy should carry a definition override")
	}
	def := EnemyInfoFor(enemy)
	if def.SingularName != "venom bat" || def.MaxHP != 33 || def.Stats.SPD != 12 ||
		def.AttackDamage != 11 || def.XPValue != 77 || def.Tier != 9 ||
		def.SkillCastChance != 1 || def.SpellPower != 8 || def.Armor != 4 {
		t.Fatalf("custom definition not applied: %+v", def)
	}
	if area.CustomEnemies[0].MP != 5 {
		t.Fatalf("custom MP should round-trip into core def, got %d", area.CustomEnemies[0].MP)
	}
	if len(def.Skills) != 1 || def.Skills[0] != SkillSleep {
		t.Fatalf("custom skills not applied: %+v", def.Skills)
	}
	if xp := PackXPValue(g.Packs[0]); xp != 77 {
		t.Fatalf("custom XP should feed PackXPValue, got %d", xp)
	}

	encoded, err := MapFileFromArea(area)
	if err != nil {
		t.Fatalf("MapFileFromArea: %v", err)
	}
	if got := encoded.Packs[0].Members[0]; got != "venom_bat" {
		t.Fatalf("custom pack member should save by custom name, got %q", got)
	}
	if got := encoded.CustomEnemies[0].MP; got != 5 {
		t.Fatalf("custom MP should round-trip back to mapfile, got %d", got)
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
	// Clear starter equipment so the inventory-survives-reset
	// assertion below tests JUST the cheese stack the test stamps in.
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

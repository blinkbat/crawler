package core

import (
	"crawler/internal/app/core/mapfile"
	"reflect"
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
	// EnemyInfoFor overlays the per-instance MaxHP, which Instantiate scales by
	// the global difficulty dial (like NewEnemy) — so the authored 33 reads back
	// as ScaleEnemyDifficulty(33). The other fields are unscaled (damage scales
	// later, at EnemyBasicDamage read time, not in the def).
	if def.SingularName != "venom bat" || def.MaxHP != ScaleEnemyDifficulty(33) || def.Stats.SPD != 12 ||
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

// TestCustomEnemyDefFromMapRejectsBadFields locks the load-time validation
// that mirrors the static enemy registry's init guards: a hand-edited custom
// enemy row with an out-of-range cast chance or a negative mitigation / reward
// / damage field is refused at load rather than silently reaching combat math.
func TestCustomEnemyDefFromMapRejectsBadFields(t *testing.T) {
	base := mapfile.MapCustomEnemy{Name: "bad", BaseKind: "bat", HP: 10}
	// Sanity: the clean baseline loads (so the cases below isolate the field).
	if _, err := CustomEnemyDefFromMap(base); err != nil {
		t.Fatalf("clean custom enemy should load, got %v", err)
	}
	cases := []struct {
		name   string
		mutate func(*mapfile.MapCustomEnemy)
	}{
		{"cast chance > 1", func(c *mapfile.MapCustomEnemy) { c.SkillCastChance = 5 }},
		{"negative cast chance", func(c *mapfile.MapCustomEnemy) { c.SkillCastChance = -0.5 }},
		{"negative armor", func(c *mapfile.MapCustomEnemy) { c.Armor = -1 }},
		{"negative mdef", func(c *mapfile.MapCustomEnemy) { c.MDef = -1 }},
		{"negative attack", func(c *mapfile.MapCustomEnemy) { c.AttackDamage = -3 }},
		{"negative xp", func(c *mapfile.MapCustomEnemy) { c.XPValue = -10 }},
		{"non-positive HP", func(c *mapfile.MapCustomEnemy) { c.HP = 0 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ce := base
			tc.mutate(&ce)
			if _, err := CustomEnemyDefFromMap(ce); err == nil {
				t.Fatalf("expected an error for %s, got nil", tc.name)
			}
		})
	}
}

// TestCustomEnemyDefToRuntime guards the def->runtime half of the lockstep
// chain that TestCustomEnemyDefMapRoundTrip does NOT cover. That test pins
// the encode<->decode pair (sites 3 & 4); this one pins Definition() (site 5)
// and Instantiate() (site 6): a fully-populated def is pushed through
// MapCustomEnemyFromDef -> CustomEnemyDefFromMap (so the mapfile round trip
// is also re-asserted on the way in) and then through Definition() and
// Instantiate(), and every authored stat / override is checked on the
// materialized runtime EnemyDefinition. So if a new field is added to the
// struct + schema + encode/decode but forgotten in Definition()/Instantiate(),
// it drops here instead of silently never reaching combat math.
func TestCustomEnemyDefToRuntime(t *testing.T) {
	orig := CustomEnemyDef{
		Name:            "Runtime Test", // has whitespace -> sanitizes to Runtime_Test
		BaseKind:        EnemyGoblin,
		HP:              42, // must be > 0 (load rejects <= 0)
		MP:              7,
		Stats:           Stats{STR: 11, DEX: 12, INT: 13, WIS: 14, VIT: 15, SPD: 16},
		Armor:           4,
		MDef:            5,
		XPValue:         88,
		Tier:            6,
		AttackDamage:    9,
		SkillCastChance: 0.75,
		SpellPower:      8,
		Skills:          []SkillID{SkillFirebolt, SkillSleep},
	}

	// Belt-and-suspenders: confirm the fixture really sets every field to a
	// non-zero value, so an assertion below can't pass only because both
	// sides defaulted to the same zero (mirrors the round-trip test's guard).
	rv := reflect.ValueOf(orig)
	for i := 0; i < rv.NumField(); i++ {
		if rv.Field(i).IsZero() {
			t.Errorf("fixture leaves field %q at its zero value; set it to a distinct non-default value so the path actually covers it", rv.Type().Field(i).Name)
		}
	}

	// Round-trip through the mapfile shape on the way in, so the def the
	// runtime sees is the one that survives a save/load (not just the raw
	// in-memory fixture). Name picks up SanitizeCustomEnemyName here.
	row, err := MapCustomEnemyFromDef(orig)
	if err != nil {
		t.Fatalf("MapCustomEnemyFromDef: %v", err)
	}
	def, err := CustomEnemyDefFromMap(row)
	if err != nil {
		t.Fatalf("CustomEnemyDefFromMap: %v", err)
	}

	// Definition() (site 5): the authored stats/overrides must land on the
	// synthesized EnemyDefinition battle and selectors read.
	ed := def.Definition()
	if ed.MaxHP != orig.HP {
		t.Errorf("Definition MaxHP = %d, want authored %d", ed.MaxHP, orig.HP)
	}
	if ed.Stats != orig.Stats {
		t.Errorf("Definition Stats = %+v, want %+v", ed.Stats, orig.Stats)
	}
	if ed.Armor != orig.Armor || ed.MDef != orig.MDef {
		t.Errorf("Definition Armor/MDef = %d/%d, want %d/%d", ed.Armor, ed.MDef, orig.Armor, orig.MDef)
	}
	if ed.XPValue != orig.XPValue || ed.Tier != orig.Tier {
		t.Errorf("Definition XPValue/Tier = %d/%d, want %d/%d", ed.XPValue, ed.Tier, orig.XPValue, orig.Tier)
	}
	if ed.AttackDamage != orig.AttackDamage || ed.SpellPower != orig.SpellPower {
		t.Errorf("Definition AttackDamage/SpellPower = %d/%d, want %d/%d", ed.AttackDamage, ed.SpellPower, orig.AttackDamage, orig.SpellPower)
	}
	if ed.SkillCastChance != orig.SkillCastChance {
		t.Errorf("Definition SkillCastChance = %v, want %v", ed.SkillCastChance, orig.SkillCastChance)
	}
	if !reflect.DeepEqual(ed.Skills, orig.Skills) {
		t.Errorf("Definition Skills = %+v, want %+v", ed.Skills, orig.Skills)
	}
	// Display name derives from the (sanitized) authored name: underscores
	// become spaces (SanitizeCustomEnemyName folded the typed space to "_").
	if ed.SingularName != "Runtime Test" {
		t.Errorf("Definition SingularName = %q, want %q", ed.SingularName, "Runtime Test")
	}

	// Instantiate() (site 6): the materialized Enemy must keep the base kind
	// for renderer lookup, carry the override, and apply the authored HP
	// scaled by the global difficulty dial (matching NewEnemy / the existing
	// pack runtime test).
	enemy := def.Instantiate()
	if enemy.Kind != orig.BaseKind {
		t.Errorf("Instantiate Kind = %v, want base kind %v", enemy.Kind, orig.BaseKind)
	}
	if !enemy.HasDefinitionOverride {
		t.Fatalf("Instantiate should carry a definition override")
	}
	wantHP := ScaleEnemyDifficulty(orig.HP)
	if enemy.HP != wantHP || enemy.MaxHP != wantHP {
		t.Errorf("Instantiate HP/MaxHP = %d/%d, want %d", enemy.HP, enemy.MaxHP, wantHP)
	}
	if enemy.Armor != orig.Armor {
		t.Errorf("Instantiate Armor = %d, want %d", enemy.Armor, orig.Armor)
	}
	// EnemyInfoFor overlays the per-instance override, so reading it back
	// confirms the authored stats survive all the way to the combat reader.
	got := EnemyInfoFor(enemy)
	if got.AttackDamage != orig.AttackDamage || got.SpellPower != orig.SpellPower || got.Tier != orig.Tier {
		t.Errorf("EnemyInfoFor override lost authored stats: %+v", got)
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
	// Clear the starter rations so the inventory-survives-reset
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

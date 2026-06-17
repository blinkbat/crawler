package core

import (
	"bytes"
	"path/filepath"
	"testing"

	"crawler/internal/app/core/mapfile"
)

// repoRoot returns the project root from the test working dir (Go runs
// tests with cwd = package dir, so we walk up to the repo root where
// maps/ lives).
func repoRoot() string {
	return filepath.Join("..", "..", "..")
}

// TestBundledMapsLoad sanity-checks every .map file shipped under maps/.
// Strict per-map assertions about prop placement, materials, and enemy
// counts were removed once the editor became the source of truth — they were
// brittle against any hand edit (and against adding/removing maps). The test
// now DISCOVERS the maps on disk via glob rather than naming them, so renaming
// or deleting a map can't make this go stale. The loose checks below catch the
// failures we actually care about: malformed files, bad starts, unreadable
// headers, unreachable packs.
func TestBundledMapsLoad(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join(repoRoot(), "maps", "*"+mapfile.Ext))
	if err != nil {
		t.Fatalf("glob maps: %v", err)
	}
	if len(paths) == 0 {
		t.Skip("no bundled maps under maps/ to check")
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			area, err := LoadArea(path)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			if area.Name == "" {
				t.Errorf("name empty")
			}
			if area.Width == 0 || area.Height == 0 {
				t.Fatalf("empty layout")
			}
			if len(area.Walls) != area.Height || len(area.Floor) != area.Height ||
				len(area.Decor) != area.Height || len(area.Props) != area.Height {
				t.Fatalf("layer row counts disagree with height %d", area.Height)
			}
			if area.StartTileX < 0 || area.StartTileX >= area.Width ||
				area.StartTileZ < 0 || area.StartTileZ >= area.Height {
				t.Errorf("start (%d,%d) out of bounds for %dx%d", area.StartTileX, area.StartTileZ, area.Width, area.Height)
			}
			if isStartBlocked(area) {
				t.Errorf("start tile is a blocking tile — player would spawn inside geometry")
			}
			// Every pack should be reachable from the player start after the
			// runtime snap pass. A pack the player can't walk up to means the
			// encounter never fires; catching this in CI is cheaper than
			// noticing it in playtest.
			if blocked := unreachablePacks(area); blocked > 0 {
				t.Errorf("%d/%d packs unreachable from start after snap", blocked, len(area.PackSpawns))
			}
		})
	}
}

// unreachablePacks runs the same reachability shape as the editor's
// warning: BFS from the player start under BlockedAt, then count packs
// whose snapped runtime position falls outside the visited set. Lives in
// the test file so the runtime stays editor-agnostic.
func unreachablePacks(a AreaDefinition) int {
	if a.StartTileZ < 0 || a.StartTileZ >= a.Height ||
		a.StartTileX < 0 || a.StartTileX >= a.Width {
		return len(a.PackSpawns)
	}
	if a.BlockedAt(a.StartTileX, a.StartTileZ) {
		return len(a.PackSpawns)
	}
	w := a.Width
	h := a.Height
	visited := make([]bool, w*h)
	stack := [][2]int{{a.StartTileX, a.StartTileZ}}
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		x, z := p[0], p[1]
		if z < 0 || z >= h || x < 0 || x >= w {
			continue
		}
		idx := z*w + x
		if visited[idx] || a.BlockedAt(x, z) {
			continue
		}
		visited[idx] = true
		stack = append(stack, [2]int{x + 1, z}, [2]int{x - 1, z}, [2]int{x, z + 1}, [2]int{x, z - 1})
	}
	blocked := 0
	for _, snap := range SnappedSpawnPositions(a) {
		if !snap.Placed() {
			blocked++
			continue
		}
		x, z := snap.TileX, snap.TileZ
		if x < 0 || z < 0 || x >= w || z >= h {
			blocked++
			continue
		}
		if !visited[z*w+x] {
			blocked++
		}
	}
	return blocked
}

// TestSelfDoorSurvivesRename guards the self-portal round-trip. A door whose
// TargetMap is the SelfMapToken must keep that placeholder through
// load → (rename area path) → save; otherwise a same-map portal silently
// re-serializes as an explicit (now-wrong) cross-map target after a rename,
// because the runtime door still held the OLD expanded map id.
func TestSelfDoorSurvivesRename(t *testing.T) {
	mf := mapfile.MapFile{
		Name:      "Self Door",
		Materials: "dungeon",
		Width:     3,
		Height:    3,
		StartX:    1,
		StartZ:    1,
		StartFace: "east",
		Walls:     []string{"...", "...", "..."},
		Floor:     []string{"...", "...", "..."},
		Decor:     []string{"...", "...", "..."},
		Props:     []string{"...", "...", "..."},
		Doors: []mapfile.MapDoor{
			{Name: "loop", TargetMap: mapfile.SelfMapToken, TargetDoor: "loop", X: 0, Z: 1, Facing: "west"},
		},
	}
	area, err := AreaFromMapFile(mf, "maps/original.map")
	if err != nil {
		t.Fatalf("AreaFromMapFile: %v", err)
	}
	if got := area.DoorSpawns[0].TargetMap; got != mapfile.SelfMapToken {
		t.Fatalf("self door should keep the %q placeholder at load (not the expanded id), got %q", mapfile.SelfMapToken, got)
	}
	// Rename: the area is now saved under a different path than it loaded from.
	area.Path = "maps/renamed.map"
	encoded, err := MapFileFromArea(area)
	if err != nil {
		t.Fatalf("MapFileFromArea: %v", err)
	}
	if got := encoded.Doors[0].TargetMap; got != mapfile.SelfMapToken {
		t.Fatalf("self door should re-serialize as %q after rename, got %q", mapfile.SelfMapToken, got)
	}
}

// TestCrystalAuthoringAndValidation pins the editable-crystal contract end to
// end: authored crystals round-trip, an explicit empty set means "zero" (no
// fallback), a legacy map with no crystals section gets the default entrance
// crystal, and a hand-edited crystal on a blocked / duplicate tile is rejected
// at load.
func TestCrystalAuthoringAndValidation(t *testing.T) {
	base := func() mapfile.MapFile {
		return mapfile.MapFile{
			Name:      "Crystals",
			Materials: "dungeon",
			Width:     3,
			Height:    3,
			StartX:    1,
			StartZ:    1,
			StartFace: "east",
			Walls:     []string{"###", "#.#", "###"},
			Floor:     []string{"...", "...", "..."},
			Decor:     []string{"...", "...", "..."},
			Props:     []string{"...", "...", "..."},
		}
	}

	// Authored crystal round-trips and is honored verbatim.
	mf := base()
	mf.CrystalsDefined = true
	mf.Crystals = []mapfile.MapCrystal{{X: 1, Z: 1}}
	area, err := AreaFromMapFile(mf, "maps/c.map")
	if err != nil {
		t.Fatalf("AreaFromMapFile (authored): %v", err)
	}
	if !area.CrystalsAuthored || len(area.CrystalSpawns) != 1 || area.CrystalSpawns[0] != (CrystalSpawn{TileX: 1, TileZ: 1}) {
		t.Fatalf("authored crystal not carried through: %+v authored=%v", area.CrystalSpawns, area.CrystalsAuthored)
	}
	if got := placeCrystals(area); len(got) != 1 || !got[0].Charged {
		t.Fatalf("authored crystal should place one charged crystal, got %+v", got)
	}

	// Explicit empty set = deliberately zero crystals (no entrance fallback).
	empty := base()
	empty.CrystalsDefined = true
	areaEmpty, err := AreaFromMapFile(empty, "maps/c.map")
	if err != nil {
		t.Fatalf("AreaFromMapFile (empty authored): %v", err)
	}
	if got := placeCrystals(areaEmpty); len(got) != 0 {
		t.Fatalf("an authored empty crystal set must place zero crystals, got %+v", got)
	}

	// Legacy map (no crystals section) falls back to the default entrance crystal.
	legacy := base()
	areaLegacy, err := AreaFromMapFile(legacy, "maps/c.map")
	if err != nil {
		t.Fatalf("AreaFromMapFile (legacy): %v", err)
	}
	if areaLegacy.CrystalsAuthored {
		t.Fatal("a map with no crystals section must not read as authored")
	}
	if got := placeCrystals(areaLegacy); len(got) != 1 {
		t.Fatalf("a legacy map should fall back to one entrance crystal, got %+v", got)
	}

	// A crystal on a blocked tile is rejected at load. Walls are gone (the
	// faces layer no longer blocks), so block with deep water — the sole
	// blocking floor — at (0,0) and try to place a crystal there.
	onBlocked := base()
	onBlocked.Floor = []string{"W..", "...", "..."} // 'W' deep water blocks
	onBlocked.CrystalsDefined = true
	onBlocked.Crystals = []mapfile.MapCrystal{{X: 0, Z: 0}}
	if _, err := AreaFromMapFile(onBlocked, "maps/c.map"); err == nil {
		t.Fatal("expected error for crystal on a blocked tile, got nil")
	}

	// Duplicate crystal tiles are rejected at load.
	dup := base()
	dup.CrystalsDefined = true
	dup.Crystals = []mapfile.MapCrystal{{X: 1, Z: 1}, {X: 1, Z: 1}}
	if _, err := AreaFromMapFile(dup, "maps/c.map"); err == nil {
		t.Fatal("expected error for duplicate crystal tile, got nil")
	}
}

// TestDialogsAndTriggersDiskRoundTrip exercises the full authored dialog +
// trigger path through the on-disk format: Area → MapFile → encode bytes →
// parse → MapFile → Area, asserting a conditioned choice and an enter-tile
// trigger survive intact. Guards the areas.go conversion + the mapfile
// dialogs:/triggers: sections together (the seam the unit round-trips don't
// cover end-to-end).
func TestDialogsAndTriggersDiskRoundTrip(t *testing.T) {
	foe := EnemyKinds()[0].Kind
	area := AreaDefinition{
		Path:        "maps/m.map",
		Name:        "Round Trip",
		Materials:   MaterialDungeon,
		Width:       3,
		Height:      3,
		StartTileX:  1,
		StartTileZ:  1,
		StartFacing: East,
		Walls:       []string{"...", "...", "..."},
		Floor:       []string{"...", "...", "..."},
		Decor:       []string{"...", "...", "..."},
		Props:       []string{"...", "...", "..."},
		Dialogs: []DialogDefinition{{
			ID: "d1", StartNodeID: "s",
			Nodes: []DialogNode{{ID: "s", SpeakerID: SpeakerStranger, Text: "Hi", Choices: []DialogChoice{
				{ID: "c", Label: "Buy", Conditions: []DialogChoiceCondition{
					{Kind: DialogCondGold, Gold: 25},
					{Kind: DialogCondFoeKilled, FoeKind: foe, FoeKills: 3},
				}},
			}}},
		}},
		Triggers: []DialogTrigger{
			{ID: "t1", Kind: DialogTriggerEnterTile, DialogID: "d1", TileX: 2, TileZ: 0, Once: true},
			{ID: "t2", Kind: DialogTriggerFoeKilled, DialogID: "d1", FoeKind: foe, FoeKills: 2},
		},
	}

	mf, err := MapFileFromArea(area)
	if err != nil {
		t.Fatalf("MapFileFromArea: %v", err)
	}
	var buf bytes.Buffer
	if err := mf.Encode(&buf); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	parsed, err := mapfile.Parse(&buf)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got, err := AreaFromMapFile(parsed, "maps/m.map")
	if err != nil {
		t.Fatalf("AreaFromMapFile: %v", err)
	}

	if len(got.Dialogs) != 1 || len(got.Dialogs[0].Nodes) != 1 {
		t.Fatalf("dialog did not survive round-trip: %+v", got.Dialogs)
	}
	conds := got.Dialogs[0].Nodes[0].Choices[0].Conditions
	if len(conds) != 2 || conds[0].Kind != DialogCondGold || conds[0].Gold != 25 ||
		conds[1].Kind != DialogCondFoeKilled || conds[1].FoeKind != foe || conds[1].FoeKills != 3 {
		t.Fatalf("choice conditions mangled in round-trip: %+v", conds)
	}
	if !slicesEqualTriggers(area.Triggers, got.Triggers) {
		t.Fatalf("triggers mangled in round-trip:\n in=%+v\nout=%+v", area.Triggers, got.Triggers)
	}
}

// TestAreaFromMapFile_ValidatesOptionalLayerDimensions guards that a PRESENT
// ceiling/elevation layer is dimension-checked like the required layers — a
// truncated one must be rejected at load rather than silently reading as the
// default (e.g. a short ceiling reads "no roof", flipping AreaIsOutdoor →
// wrong weather/lighting). Absent and full-dimension layers still load clean.
func TestAreaFromMapFile_ValidatesOptionalLayerDimensions(t *testing.T) {
	base := func() mapfile.MapFile {
		return mapfile.MapFile{
			Name: "Optional", Materials: "dungeon", Width: 3, Height: 3,
			StartX: 1, StartZ: 1, StartFace: "east",
			Walls: []string{"...", "...", "..."},
			Floor: []string{"...", "...", "..."},
			Decor: []string{"...", "...", "..."},
			Props: []string{"...", "...", "..."},
		}
	}

	// Absent optional layers blank-fill — must load clean.
	if _, err := AreaFromMapFile(base(), "maps/m.map"); err != nil {
		t.Fatalf("absent optional layers should load: %v", err)
	}

	// Full-dimension ceiling + elevation — must load clean.
	full := base()
	full.Ceiling = []string{"...", "...", "..."}
	full.Elevation = []string{"000", "000", "000"}
	if _, err := AreaFromMapFile(full, "maps/m.map"); err != nil {
		t.Fatalf("full-dimension optional layers should load: %v", err)
	}

	// Short ceiling (2 rows, declared height 3) — must be rejected.
	short := base()
	short.Ceiling = []string{"...", "..."}
	if _, err := AreaFromMapFile(short, "maps/m.map"); err == nil {
		t.Error("a present-but-short ceiling layer must be rejected, not blank-substituted")
	}

	// Narrow elevation row (width 2 vs declared 3) — must be rejected.
	narrow := base()
	narrow.Elevation = []string{"000", "00", "000"}
	if _, err := AreaFromMapFile(narrow, "maps/m.map"); err == nil {
		t.Error("a present elevation layer with a short row must be rejected")
	}
}

// TestPackMember_CustomNameShadowsBuiltin guards the resolution-order fix: a
// custom enemy whose name collides with a built-in kind ("goblin") must resolve
// to the CUSTOM def (carrying its overrides), not be silently shadowed by the
// built-in. Pack-member resolution checks the map's CustomEnemies before the
// built-in registry.
func TestPackMember_CustomNameShadowsBuiltin(t *testing.T) {
	// Sanity: the name really does collide with a built-in kind.
	if _, ok := EnemyKindFromName("goblin"); !ok {
		t.Fatal("precondition: \"goblin\" should name a built-in kind")
	}

	row, err := MapCustomEnemyFromDef(CustomEnemyDef{
		Name:     "goblin",  // deliberately collides with the built-in
		BaseKind: EnemyRat,  // a DIFFERENT base, so a shadow would be visible
		HP:       99,
		Stats:    Stats{STR: 1, DEX: 1, INT: 1, WIS: 1, VIT: 1, SPD: 1},
		XPValue:  7,
		Tier:     2,
	})
	if err != nil {
		t.Fatalf("MapCustomEnemyFromDef: %v", err)
	}
	mf := mapfile.MapFile{
		Name: "Collide", Materials: "dungeon", Width: 3, Height: 3,
		StartX: 1, StartZ: 1, StartFace: "east",
		Walls: []string{"...", "...", "..."},
		Floor: []string{"...", "...", "..."},
		Decor: []string{"...", "...", "..."},
		Props: []string{"...", "...", "..."},
		CustomEnemies: []mapfile.MapCustomEnemy{row},
		Packs:         []mapfile.MapPack{{Members: []string{"goblin"}, X: 0, Z: 0}},
	}
	area, err := AreaFromMapFile(mf, "maps/collide.map")
	if err != nil {
		t.Fatalf("AreaFromMapFile: %v", err)
	}
	if len(area.PackSpawns) != 1 || len(area.PackSpawns[0].Members) != 1 {
		t.Fatalf("pack did not resolve: %+v", area.PackSpawns)
	}
	if got := area.PackSpawns[0].Members[0].CustomName; got != "goblin" {
		t.Errorf("colliding name resolved to the built-in (CustomName=%q), want the custom def %q", got, "goblin")
	}
}

// TestChestOnStartTileRejected guards the load-time validation fix: a chest
// authored on the player-start tile blocks movement onto the spawn, so the
// runtime silently dropped it (hiding the mistake). It must now be rejected at
// load, where the editor's placement rules already forbid it.
func TestChestOnStartTileRejected(t *testing.T) {
	mf := mapfile.MapFile{
		Name: "ChestOnStart", Materials: "dungeon", Width: 3, Height: 3,
		StartX: 1, StartZ: 1, StartFace: "east",
		Walls:  []string{"...", "...", "..."},
		Floor:  []string{"...", "...", "..."},
		Decor:  []string{"...", "...", "..."},
		Props:  []string{"...", "...", "..."},
		Chests: []mapfile.MapChest{{Items: []string{"Crust of Bread"}, X: 1, Z: 1}}, // on the start tile
	}
	if _, err := AreaFromMapFile(mf, "maps/chest.map"); err == nil {
		t.Fatal("expected a chest on the player-start tile to be rejected at load, got nil")
	}

	// A chest on a DIFFERENT tile still loads clean (the rule is start-tile-only).
	mf.Chests = []mapfile.MapChest{{Items: []string{"Crust of Bread"}, X: 0, Z: 0}}
	if _, err := AreaFromMapFile(mf, "maps/chest.map"); err != nil {
		t.Fatalf("a chest off the start tile should load: %v", err)
	}
}

func isStartBlocked(a AreaDefinition) bool {
	if a.StartTileZ < 0 || a.StartTileZ >= a.Height {
		return true
	}
	if a.StartTileX < 0 || a.StartTileX >= a.Width {
		return true
	}
	if a.Walls[a.StartTileZ][a.StartTileX] == TileRock {
		return true
	}
	switch a.Props[a.StartTileZ][a.StartTileX] {
	case TileTree, TileTreeXL, TileRockLarge, TileBushLarge:
		return true
	}
	return false
}

package core

import (
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

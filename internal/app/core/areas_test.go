package core

import (
	"path/filepath"
	"testing"
)

// repoRoot returns the project root from the test working dir (Go runs
// tests with cwd = package dir, so we walk up to the repo root where
// maps/ lives).
func repoRoot() string {
	return filepath.Join("..", "..", "..")
}

// TestBundledMapsLoad sanity-checks every .map file shipped under maps/.
// Strict per-map assertions about prop placement and enemy counts were
// removed once the editor became the source of truth — they were brittle
// against any hand edit. The loose checks below catch the failures we
// actually care about: malformed files, bad starts, unreadable headers.
func TestBundledMapsLoad(t *testing.T) {
	cases := []struct {
		id           string
		wantMaterial MaterialSet
	}{
		{"dungeon", MaterialDungeon},
		{"field", MaterialField},
		{"forgotten_plaza", MaterialField},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			area, err := LoadArea(filepath.Join(repoRoot(), "maps", tc.id+".map"))
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			if area.Name == "" {
				t.Errorf("name empty")
			}
			if area.Materials != tc.wantMaterial {
				t.Errorf("materials: got %v, want %v", area.Materials, tc.wantMaterial)
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

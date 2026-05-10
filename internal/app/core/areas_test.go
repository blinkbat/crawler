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
		})
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

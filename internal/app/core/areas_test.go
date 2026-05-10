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
			if len(area.Layout) == 0 || len(area.Layout[0]) == 0 {
				t.Fatalf("empty layout")
			}
			h := len(area.Layout)
			w := len(area.Layout[0])
			if area.StartTileX < 0 || area.StartTileX >= w ||
				area.StartTileZ < 0 || area.StartTileZ >= h {
				t.Errorf("start (%d,%d) out of bounds for %dx%d", area.StartTileX, area.StartTileZ, w, h)
			}
			if isStartBlocked(area) {
				t.Errorf("start tile is a blocking tile — player would spawn inside geometry")
			}
		})
	}
}

func isStartBlocked(a AreaDefinition) bool {
	if a.StartTileZ < 0 || a.StartTileZ >= len(a.Layout) {
		return true
	}
	row := a.Layout[a.StartTileZ]
	if a.StartTileX < 0 || a.StartTileX >= len(row) {
		return true
	}
	switch row[a.StartTileX] {
	case TileRock, TileTree, TileTreeXL, TileRockLarge, TileBushLarge:
		return true
	}
	return false
}

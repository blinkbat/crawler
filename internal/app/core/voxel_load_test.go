package core

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"crawler/internal/app/core/mapfile"
)

// TestGappedMapLoadsAndRoundTrips loads the shipped forest_path.map (its raised
// trail carved into a walk-under land bridge), confirms the gap survives into the
// area's Solids stack, that you can walk under it, and that the converter
// round-trip preserves it.
func TestGappedMapLoadsAndRoundTrips(t *testing.T) {
	path := filepath.Join("..", "..", "..", "maps", "forest_path.map")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("map not present: %v", err)
	}
	mf, err := mapfile.Parse(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	area, err := AreaFromMapFile(mf, "forest_path")
	if err != nil {
		t.Fatalf("AreaFromMapFile: %v", err)
	}
	if len(area.Solids) == 0 {
		t.Fatalf("gapped map produced no Solids stack")
	}
	if area.AllColumnsGapless() {
		t.Fatalf("land-bridge map should NOT be all-gapless")
	}
	// The bridge at column (48,10): ground standable under the deck, gap above,
	// deck on top.
	const base = ElevationBaseline
	const bx, bz = 48, 10
	if !area.Standable(bx, base, bz) {
		t.Errorf("ground under the bridge at (%d,%d,%d) should be standable", bx, base, bz)
	}
	if _, gap := area.SolidAt(bx, base+1, bz); gap {
		t.Errorf("(%d,%d,%d) should be the walk-under gap", bx, base+1, bz)
	}
	if !area.Standable(bx, base+2, bz) {
		t.Errorf("the deck at (%d,%d,%d) should be standable", bx, base+2, bz)
	}
	// You can walk UNDER the bridge east-west along the ground.
	if l, ok := area.ResolveStep(bx-1, base, bz, East); !ok || l != base {
		t.Errorf("walk under the bridge (west->east) = (%d,%v), want (%d,true)", l, ok, base)
	}

	// Round-trip through the converters and back.
	mf2, err := MapFileFromArea(area)
	if err != nil {
		t.Fatalf("MapFileFromArea: %v", err)
	}
	var buf bytes.Buffer
	if err := mf2.Encode(&buf); err != nil {
		t.Fatalf("encode: %v", err)
	}
	mf3, err := mapfile.Parse(&buf)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	area2, err := AreaFromMapFile(mf3, "forest_path")
	if err != nil {
		t.Fatalf("re-AreaFromMapFile: %v", err)
	}
	if !AreaContentEqual(area, area2) {
		t.Errorf("land-bridge map did not survive a converter round-trip")
	}
}

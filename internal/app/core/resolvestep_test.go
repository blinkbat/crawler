package core

import "testing"

// TestResolveStepEquivalentToStepElevationOK proves the voxel ResolveStep is a
// faithful generalization: on a heightfield area (Solids nil), for the column
// top it agrees with StepElevationOK on every tile/direction — so existing maps
// move identically. Reuses the ramp fixture from elevation_test.go.
func TestResolveStepEquivalentToStepElevationOK(t *testing.T) {
	a := elevTestArea()
	for z := 0; z < a.Height; z++ {
		for x := 0; x < a.Width; x++ {
			fromL := a.ElevationLevelAt(x, z) // the single standable surface
			for _, d := range CardinalDirs {
				_, got := a.ResolveStep(x, fromL, z, d)
				want := a.StepElevationOK(x, z, d)
				if got != want {
					t.Errorf("ResolveStep(%d,%d,%d,dir=%d)=%v, StepElevationOK=%v", x, fromL, z, d, got, want)
				}
			}
		}
	}
}

// TestResolveStepVoxelTruthTable pins the cube-model behaviors the user asked
// for, on the gapped bridge column.
func TestResolveStepVoxelTruthTable(t *testing.T) {
	// 3-wide, 1-deep corridor. Column 0 = plain ground. Column 1 = a bridge:
	// ground + gap + deck. Column 2 = plain ground. All ground at level 0 here
	// (a self-contained fixture; absolute baseline is irrelevant to the rule).
	a := AreaDefinition{Width: 3, Height: 1}
	// plane0: all solid ground; plane1: air; plane2: deck over column 1 only.
	a.Solids = [][]string{
		{"###"},
		{string([]byte{SolidAir, SolidAir, SolidAir})},
		{string([]byte{SolidAir, '#', SolidAir})},
	}
	// Walking from column 0 (ground L0) East toward the bridge: you land UNDER
	// the deck, on the bridge column's ground at L0 — not blocked, not the deck.
	if toL, ok := a.ResolveStep(0, 0, 0, East); !ok || toL != 0 {
		t.Errorf("walk-under: ResolveStep east from ground = (%d,%v), want (0,true)", toL, ok)
	}
	// Standing ON the deck (L2) at column 1, stepping East to plain ground
	// column 2 (only surface L0): a 2-high drop with no matching edge → blocked.
	if _, ok := a.ResolveStep(1, 2, 0, East); ok {
		t.Errorf("deck->lower-ground 2-high drop should be blocked")
	}
	// "ev1 on ev0 is a wall": a 1-high solid step blocks a flat walk. Column 0
	// ground L0 -> a neighbour solid at both L0 and L1 (top L1) is a cliff.
	wall := AreaDefinition{Width: 2, Height: 1}
	wall.Solids = [][]string{{"##"}, {string([]byte{SolidAir, '#'})}}
	// col1 is solid at L0 and L1 → its only standable surface is L1; from col0's
	// L0 the edges don't match (0 vs 1) → blocked (the wall).
	if _, ok := wall.ResolveStep(0, 0, 0, East); ok {
		t.Errorf("ev1-on-ev0 should read as a wall (blocked)")
	}
	// And the reverse: from the higher tile (L1) onto the lower (L0) is also a
	// cliff, not a step.
	if _, ok := wall.ResolveStep(1, 1, 0, West); ok {
		t.Errorf("stepping off a 1-high ledge should be blocked (cliff)")
	}
}

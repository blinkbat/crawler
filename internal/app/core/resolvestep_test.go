package core

import "testing"

// TestResolveStepEquivalentToStepElevationOK: on a heightfield area (Solids nil)
// ResolveStep agrees with StepElevationOK everywhere, so existing maps move identically.
func TestResolveStepEquivalentToStepElevationOK(t *testing.T) {
	a := elevTestArea()
	for z := 0; z < a.Height; z++ {
		for x := 0; x < a.Width; x++ {
			fromL := a.ElevationLevelAt(x, z)
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

// TestResolveStepVoxelTruthTable pins the cube-model behaviors on a gapped bridge column.
func TestResolveStepVoxelTruthTable(t *testing.T) {
	// 3-wide corridor: col 0/2 ground, col 1 a bridge (ground + gap + deck), all at L0.
	a := AreaDefinition{Width: 3, Height: 1}
	a.Solids = [][]string{
		{"###"},
		{string([]byte{SolidAir, SolidAir, SolidAir})},
		{string([]byte{SolidAir, '#', SolidAir})},
	}
	// Col 0 (L0) East toward the bridge: land UNDER the deck on col 1's ground (L0).
	if toL, ok := a.ResolveStep(0, 0, 0, East); !ok || toL != 0 {
		t.Errorf("walk-under: ResolveStep east from ground = (%d,%v), want (0,true)", toL, ok)
	}
	// On the deck (L2) at col 1, East to col 2 ground (L0): 2-high drop → blocked.
	if _, ok := a.ResolveStep(1, 2, 0, East); ok {
		t.Errorf("deck->lower-ground 2-high drop should be blocked")
	}
	// "ev1 on ev0 is a wall": a 1-high solid step blocks a flat walk.
	wall := AreaDefinition{Width: 2, Height: 1}
	wall.Solids = [][]string{{"##"}, {string([]byte{SolidAir, '#'})}}
	// col1's only surface is L1; from col0's L0 the edges don't match → blocked.
	if _, ok := wall.ResolveStep(0, 0, 0, East); ok {
		t.Errorf("ev1-on-ev0 should read as a wall (blocked)")
	}
	// Reverse: higher tile (L1) onto lower (L0) is also a cliff, not a step.
	if _, ok := wall.ResolveStep(1, 1, 0, West); ok {
		t.Errorf("stepping off a 1-high ledge should be blocked (cliff)")
	}
}

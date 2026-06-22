package core

import "testing"

// gappedArea: 3×3 map, column (1,1) solid at L0, AIR at L1, solid at L2 (a deck walked UNDER at L0, OVER at L2); other columns plain ground.
func gappedArea() AreaDefinition {
	air := "000"
	a := AreaDefinition{Width: 3, Height: 3}
	plane0 := []string{"###", "###", "###"}
	plane1 := []string{air, "0" + string(SolidAir) + "0", air}
	plane2 := []string{air, "0#0", air}
	a.Solids = [][]string{plane0, plane1, plane2}
	return a
}

func TestSolidAtAndStandable(t *testing.T) {
	a := gappedArea()
	if _, solid := a.SolidAt(1, 0, 1); !solid {
		t.Errorf("(1,0,1) should be solid ground")
	}
	if _, solid := a.SolidAt(1, 1, 1); solid {
		t.Errorf("(1,1,1) should be AIR (the gap)")
	}
	if _, solid := a.SolidAt(1, 2, 1); !solid {
		t.Errorf("(1,2,1) should be the floating cube")
	}
	if !a.Standable(1, 0, 1) {
		t.Errorf("ground under the box should be standable (air above)")
	}
	if a.Standable(1, 1, 1) {
		t.Errorf("the air gap is not standable")
	}
	if !a.Standable(1, 2, 1) {
		t.Errorf("the deck top should be standable")
	}
	if !a.Standable(0, 0, 0) || a.Standable(0, 1, 0) {
		t.Errorf("plain column should be standable only at L0")
	}
	if got := a.TopSolidLevel(1, 1); got != 2 {
		t.Errorf("TopSolidLevel(1,1) = %d, want 2", got)
	}
	if got := a.LowestStandableLevel(1, 1); got != 0 {
		t.Errorf("LowestStandableLevel(1,1) = %d, want 0 (under the box)", got)
	}
}

// TestHeightfieldStackIdentity: migration is lossless — stack-from-heightfield then read tops back is identity for a gapless map, which reports AllColumnsGapless.
func TestHeightfieldStackIdentity(t *testing.T) {
	a := AreaDefinition{
		Width: 3, Height: 3,
		Elevation: []string{"010", "000", "002"},
	}
	if !a.AllColumnsGapless() {
		t.Fatalf("heightfield (nil Solids) must be gapless")
	}
	stack := BuildSolidsFromElevation(&a)
	b := AreaDefinition{Width: 3, Height: 3, Solids: stack}
	if !b.AllColumnsGapless() {
		t.Errorf("a stack built from a heightfield must be gapless")
	}
	for z := 0; z < a.Height; z++ {
		for x := 0; x < a.Width; x++ {
			if got, want := b.TopSolidLevel(x, z), a.ElevationLevelAt(x, z); got != want {
				t.Errorf("top(%d,%d)=%d, want %d (heightfield identity)", x, z, got, want)
			}
		}
	}
	rows := ElevationRowsFromSolids(&b)
	for z := range rows {
		if rows[z] != a.Elevation[z] {
			t.Errorf("elevation row %d round-trip: %q, want %q", z, rows[z], a.Elevation[z])
		}
	}
}

// TestSolidsEqualAbsentVsMaterialized: dirty-check guard — a heightfield area (Solids nil) must compare EQUAL to the same area with its stack materialized, else every save reads dirty.
func TestSolidsEqualAbsentVsMaterialized(t *testing.T) {
	a := AreaDefinition{Width: 3, Height: 3, Elevation: []string{"010", "000", "000"}}
	b := a
	b.Solids = BuildSolidsFromElevation(&a)
	if !solidsEqual(a, b) {
		t.Errorf("heightfield vs materialized stack should be solids-equal")
	}
	if !AreaContentEqual(a, b) {
		t.Errorf("heightfield vs materialized stack should be content-equal (not dirty)")
	}
	// A genuine gap must NOT compare equal to the flat heightfield.
	c := gappedArea()
	flat := AreaDefinition{Width: 3, Height: 3}
	if solidsEqual(c, flat) {
		t.Errorf("a gapped area must not be solids-equal to a flat one")
	}
}

func TestGappedColumnNotGapless(t *testing.T) {
	a := gappedArea()
	if a.AllColumnsGapless() {
		t.Errorf("a column with a floating cube over air must report NOT gapless")
	}
}

// TestEditorAuthoringPrimitives: editor cube-placement helpers — floating cube over air materializes a gapped stack, clearing trims back, SetColumnTop sets a gapless height.
func TestEditorAuthoringPrimitives(t *testing.T) {
	a := AreaDefinition{Width: 3, Height: 3, Elevation: []string{"AAA", "AAA", "AAA"}}
	const base = ElevationBaseline // 10
	// Floating cube two levels above ground at (1,1): materializes the stack, creates a gap (air at base+1).
	a.SetCube(1, base+2, 1, TileRock)
	if len(a.Solids) == 0 {
		t.Fatalf("SetCube should materialize the stack")
	}
	if a.AllColumnsGapless() {
		t.Errorf("a floating cube over air should make the map gapped")
	}
	if !a.Standable(1, base, 1) {
		t.Errorf("ground under the new cube should remain standable")
	}
	if _, gap := a.SolidAt(1, base+1, 1); gap {
		t.Errorf("level base+1 should be the air gap")
	}
	if !a.Standable(1, base+2, 1) {
		t.Errorf("the placed cube top should be standable")
	}
	a.ClearCube(1, base+2, 1)
	if !a.AllColumnsGapless() {
		t.Errorf("clearing the floating cube should restore a gapless map")
	}
	a.SetColumnTop(0, 0, base+3)
	if got := a.TopSolidLevel(0, 0); got != base+3 {
		t.Errorf("SetColumnTop top = %d, want %d", got, base+3)
	}
	if !a.columnGapless(0, 0) {
		t.Errorf("SetColumnTop column must be gapless (solid 0..top)")
	}
}

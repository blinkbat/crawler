package core

import "testing"

func testRegionArea() AreaDefinition {
	return AreaDefinition{
		Width: 4, Height: 4,
		Walls:     []string{"....", "....", "....", "...."},
		Floor:     []string{"abcd", "efgh", "ijkl", "mnop"},
		Decor:     []string{"....", "....", "....", "...."},
		Props:     []string{"....", "....", "....", "...."},
		Ceiling:   []string{"....", "....", "....", "...."},
		Elevation: []string{"0000", "0000", "0000", "0000"},
	}
}

// TestCopyPasteRegion: a copied rectangle stamps at the paste anchor across
// layers, clips at edges, and never mutates the source snapshot.
func TestCopyPasteRegion(t *testing.T) {
	a := testRegionArea()
	r := CopyRegion(&a, 0, 0, 1, 1) // top-left 2x2: floor "ab"/"ef"
	if r.W != 2 || r.H != 2 {
		t.Fatalf("region size = %dx%d, want 2x2", r.W, r.H)
	}
	if len(r.Layers) != 6 {
		t.Fatalf("region has %d layers, want 6", len(r.Layers))
	}

	a.PasteRegion(r, 2, 2) // stamp at (2,2)
	if a.Floor[2] != "ijab" || a.Floor[3] != "mnef" {
		t.Fatalf("paste floor rows = %q / %q, want ijab / mnef", a.Floor[2], a.Floor[3])
	}
	// Rows above the paste untouched.
	if a.Floor[0] != "abcd" || a.Floor[1] != "efgh" {
		t.Fatalf("paste leaked above the anchor: %q / %q", a.Floor[0], a.Floor[1])
	}

	// Snapshot is independent of the source after the copy.
	if r.Layers[1][0] != "ab" {
		t.Fatalf("copied floor row = %q, want ab", r.Layers[1][0])
	}
}

// TestPasteRegion_ClipsAtEdge: a paste straddling the edge writes only in-bounds cells.
func TestPasteRegion_ClipsAtEdge(t *testing.T) {
	a := testRegionArea()
	r := CopyRegion(&a, 0, 0, 1, 1) // 2x2
	a.PasteRegion(r, 3, 3)          // only (3,3) is in bounds; the rest clips off
	if a.Floor[3] != "mnoa" {       // last col gets 'a', the region's (0,0)
		t.Fatalf("edge paste = %q, want mnoa", a.Floor[3])
	}
}

// TestCopyRegion_ClampsAndDegenerate covers out-of-range and empty cases.
func TestCopyRegion_ClampsAndDegenerate(t *testing.T) {
	a := testRegionArea()
	// Over-range corners clamp to the 4x4 area.
	if r := CopyRegion(&a, -5, -5, 99, 99); r.W != 4 || r.H != 4 {
		t.Fatalf("clamp size = %dx%d, want 4x4", r.W, r.H)
	}
	// Nil + empty region are no-ops.
	if r := CopyRegion(nil, 0, 0, 1, 1); !r.Empty() {
		t.Fatal("nil area should yield an empty region")
	}
	a.PasteRegion(TileRegion{}, 0, 0) // must not panic
}

// TestCopyPasteRegion_Voxel: on a materialized voxel map the cube stack travels
// with the region (Elevation alone is ignored by SolidAt there).
func TestCopyPasteRegion_Voxel(t *testing.T) {
	a := testRegionArea()
	EnsureSolids(&a)
	// Place a floating cube at (1,2,1) — solid at level 2, air below it.
	a.SetCube(1, 2, 1, TileRock)
	if _, solid := a.SolidAt(1, 2, 1); !solid {
		t.Fatal("setup: expected a cube at (1,2,1)")
	}

	r := CopyRegion(&a, 1, 1, 1, 1) // single tile carrying the floating cube
	if len(r.Solids) < 3 {
		t.Fatalf("voxel region captured %d planes, want >=3", len(r.Solids))
	}

	a.PasteRegion(r, 3, 3) // stamp the cube onto an empty column
	if _, solid := a.SolidAt(3, 2, 3); !solid {
		t.Fatal("voxel paste did not reproduce the floating cube at (3,2,3)")
	}
	// Below the floating cube stays air (the stack copied faithfully).
	if _, solid := a.SolidAt(3, 1, 3); solid {
		t.Fatal("voxel paste filled air below the floating cube")
	}
}

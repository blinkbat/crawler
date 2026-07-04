package core

import (
	"bytes"
	"testing"

	"crawler/internal/app/core/mapfile"
)

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

// TestPropYawRoundTrip: an authored prop facing survives Area → MapFile → encode →
// parse → Area, an all-auto grid is NOT serialized (byte-stable), and Set/clear work.
func TestPropYawRoundTrip(t *testing.T) {
	a := testRegionArea()
	a.Props = []string{"....", ".T..", "....", "...."}
	a.SetPropYawStep(1, 1, 3) // 3 * 30° = 90°
	if got := a.PropYawStepAt(1, 1); got != 3 {
		t.Fatalf("PropYawStepAt = %d, want 3", got)
	}
	if deg, ok := a.PropYawOverride(1, 1); !ok || deg != 90 {
		t.Fatalf("PropYawOverride = (%v,%v), want (90,true)", deg, ok)
	}

	mf, err := MapFileFromArea(a)
	if err != nil {
		t.Fatalf("MapFileFromArea: %v", err)
	}
	var buf bytes.Buffer
	if err := mf.Encode(&buf); err != nil {
		t.Fatalf("encode: %v", err)
	}
	parsed, err := mapfile.Parse(&buf)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got, err := AreaFromMapFile(parsed, "x")
	if err != nil {
		t.Fatalf("AreaFromMapFile: %v", err)
	}
	if step := got.PropYawStepAt(1, 1); step != 3 {
		t.Errorf("round-trip facing step = %d, want 3", step)
	}

	// An all-auto grid must not be written (pre-feature maps stay byte-identical).
	a.SetPropYawStep(1, 1, -1)
	mf2, _ := MapFileFromArea(a)
	if len(mf2.PropYaw) != 0 {
		t.Errorf("all-auto PropYaw serialized as %d rows, want 0", len(mf2.PropYaw))
	}
}

// TestPropYawHighSteps guards a validation bug: steps 10 and 11 (300°/330°) encode
// as lowercase 'a'/'b', which the level-grid validator (uppercase 'A'..'K' only) once
// rejected — so those two facings failed to save/reload. Full Encode→Parse→Area path.
func TestPropYawHighSteps(t *testing.T) {
	for _, step := range []int{10, 11} {
		a := testRegionArea()
		a.Props = []string{"....", ".T..", "....", "...."}
		a.SetPropYawStep(1, 1, step)

		mf, err := MapFileFromArea(a)
		if err != nil {
			t.Fatalf("step %d: MapFileFromArea: %v", step, err)
		}
		var buf bytes.Buffer
		if err := mf.Encode(&buf); err != nil {
			t.Fatalf("step %d: encode: %v", step, err)
		}
		parsed, err := mapfile.Parse(&buf) // Parse validates — the reproducer for the bug
		if err != nil {
			t.Fatalf("step %d: parse: %v", step, err)
		}
		got, err := AreaFromMapFile(parsed, "x")
		if err != nil {
			t.Fatalf("step %d: AreaFromMapFile: %v", step, err)
		}
		if s := got.PropYawStepAt(1, 1); s != step {
			t.Errorf("round-trip facing step = %d, want %d", s, step)
		}
	}
}

// TestClearRegion blanks the rectangle across grid layers (floor → auto '.') and
// leaves cells outside it untouched — the grid half of the editor's Cut/move.
func TestClearRegion(t *testing.T) {
	a := testRegionArea()
	a.ClearRegion(1, 1, 2, 2) // inner 2x2
	want := []string{"abcd", "e..h", "i..l", "mnop"}
	for z, w := range want {
		if a.Floor[z] != w {
			t.Errorf("ClearRegion floor row %d = %q, want %q", z, a.Floor[z], w)
		}
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

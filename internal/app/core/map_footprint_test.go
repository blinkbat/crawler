package core

import "testing"

// TestPropFootprint_RockFormation checks the 2×2 anchor footprint.
// Anchor at (0,0) → occupies (0,0), (1,0), (0,1), (1,1). Order doesn't
// matter for caller correctness but it does for this assertion — we
// freeze the slice contents to lock the contract.
func TestPropFootprint_RockFormation(t *testing.T) {
	got := PropFootprint(TileRockFormation)
	want := []MultiTileOffset{{0, 0}, {1, 0}, {0, 1}, {1, 1}}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("offset[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

// TestPropFootprint_SingleTileReturnsNil — every single-tile prop char
// reports a nil footprint so the editor's auto-fill path knows to fall
// through to the single-cell place. Iterates every known prop char; only
// the multi-tile anchors should return non-nil.
func TestPropFootprint_SingleTileReturnsNil(t *testing.T) {
	singleTile := []byte{
		TileTree, TileTreeXL, TileRockLarge, TileBushLarge,
		TileCrate, TileBarrel, TileUrn, TileStalagmite,
		TilePillar, TileBrokenPillar, TileStatue, TileObelisk, TileFountain,
		TileRockCairn, TileRockFormationTail, // tail isn't an anchor either
	}
	for _, c := range singleTile {
		if fp := PropFootprint(c); fp != nil {
			t.Errorf("PropFootprint(%q) = %v, want nil for single-tile prop", c, fp)
		}
	}
}

// TestPropFootprintTail_PairsWithAnchor — the tail char returned for
// each anchor must be a recognized prop char (so it blocks movement) and
// must NOT itself be an anchor (so it doesn't recurse the auto-fill).
func TestPropFootprintTail_PairsWithAnchor(t *testing.T) {
	for _, anchor := range []byte{TileRockFormation} {
		tail := PropFootprintTail(anchor)
		if tail == 0 {
			t.Errorf("PropFootprintTail(%q) returned 0; anchor needs a tail", anchor)
			continue
		}
		if !IsPropChar(tail) {
			t.Errorf("tail %q for anchor %q should be a blocking prop char", tail, anchor)
		}
		if fp := PropFootprint(tail); fp != nil {
			t.Errorf("tail %q for anchor %q is itself an anchor (footprint=%v); should be tail-only", tail, anchor, fp)
		}
	}
}

// TestDecorFootprint_Archway — the 1×2 archway anchor footprint.
func TestDecorFootprint_Archway(t *testing.T) {
	got := DecorFootprint(DecorArchway)
	want := []MultiTileOffset{{0, 0}, {1, 0}}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("offset[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

// TestDecorFootprintTail_PairsWithAnchor mirrors the props-side check —
// the tail must be a recognized decor char (so map.go's TileLabel and
// the renderer skip it correctly).
func TestDecorFootprintTail_PairsWithAnchor(t *testing.T) {
	tail := DecorFootprintTail(DecorArchway)
	if tail == 0 {
		t.Fatalf("DecorFootprintTail(DecorArchway) returned 0")
	}
	if tail != DecorArchwayTail {
		t.Errorf("DecorFootprintTail = %q, want %q", tail, DecorArchwayTail)
	}
	if TileLabel(TileLayerDecor, tail) == "?" {
		t.Errorf("tail %q has no TileLabel entry for decor layer", tail)
	}
}

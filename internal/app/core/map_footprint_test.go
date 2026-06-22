package core

import "testing"

// TestPropFootprint_RockFormation: 2×2 anchor footprint. Order is frozen here to lock the contract, though callers don't depend on it.
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

// TestPropFootprint_SingleTileReturnsNil — single-tile prop chars report nil so the editor's auto-fill falls through to a single-cell place.
func TestPropFootprint_SingleTileReturnsNil(t *testing.T) {
	singleTile := []byte{
		TileTree, TileTreeXL, TileRockLarge, TileBushLarge,
		TileCrate, TileBarrel, TileUrn, TileStalagmite,
		TilePillar, TileBrokenPillar, TileStatue, TileObelisk, TileFountain,
		TileRockCairn, TileRockFormationTail,
	}
	for _, c := range singleTile {
		if fp := PropFootprint(c); fp != nil {
			t.Errorf("PropFootprint(%q) = %v, want nil for single-tile prop", c, fp)
		}
	}
}

// TestPropFootprintTail_PairsWithAnchor — each anchor's tail must be a prop char (blocks movement) but not itself an anchor (else auto-fill recurses).
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

// TestDecorFootprint_Archway — 1×2 archway anchor footprint.
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

// TestDecorFootprintTail_PairsWithAnchor — tail must be a recognized decor char so map.go's TileLabel and the renderer skip it.
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

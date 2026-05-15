package mapfile

import (
	"bytes"
	"strings"
	"testing"
)

const sample = `name: Test Map
materials: dungeon
quiet: It is quiet.
size: 5x4
start: 1 1 east
walls:
#####
#...#
#...#
#####
floor:
.....
.c~n.
.wsi.
.....
decor:
.....
.,fv.
.o*!.
.....
props:
.....
..T..
.CRU.
.....
enemies:
rat 2 1
bat,rat 3 2
`

func TestParseSample(t *testing.T) {
	mf, err := Parse(strings.NewReader(sample))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if mf.Name != "Test Map" || mf.Materials != "dungeon" || mf.Quiet != "It is quiet." {
		t.Fatalf("metadata mismatch: %+v", mf)
	}
	if mf.Width != 5 || mf.Height != 4 {
		t.Fatalf("size mismatch: %dx%d", mf.Width, mf.Height)
	}
	if mf.StartX != 1 || mf.StartZ != 1 || mf.StartFace != "east" {
		t.Fatalf("start mismatch: %+v", mf)
	}
	for name, rows := range map[string][]string{
		"walls": mf.Walls,
		"floor": mf.Floor,
		"decor": mf.Decor,
		"props": mf.Props,
	} {
		if len(rows) != 4 {
			t.Fatalf("%s rows = %d, want 4", name, len(rows))
		}
	}
	if mf.Walls[0] != "#####" || mf.Walls[1] != "#...#" {
		t.Fatalf("walls mismatch: %v", mf.Walls)
	}
	if mf.Props[1] != "..T.." {
		t.Fatalf("props mismatch: %v", mf.Props)
	}
	if mf.Props[2] != ".CRU." {
		t.Fatalf("props row with new props mismatch: %v", mf.Props)
	}
	if mf.Floor[1] != ".c~n." {
		t.Fatalf("floor row with new variants mismatch: %v", mf.Floor)
	}
	if mf.Decor[2] != ".o*!." {
		t.Fatalf("decor row with new variants mismatch: %v", mf.Decor)
	}
	if len(mf.Packs) != 2 {
		t.Fatalf("pack count: %d, want 2 (%+v)", len(mf.Packs), mf.Packs)
	}
	if len(mf.Packs[0].Members) != 1 || mf.Packs[0].Members[0] != "rat" {
		t.Fatalf("pack 0 mismatch: %+v", mf.Packs[0])
	}
	if len(mf.Packs[1].Members) != 2 || mf.Packs[1].Members[0] != "bat" || mf.Packs[1].Members[1] != "rat" {
		t.Fatalf("pack 1 mismatch: %+v", mf.Packs[1])
	}
}

func TestRoundTrip(t *testing.T) {
	mf, err := Parse(strings.NewReader(sample))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var buf bytes.Buffer
	if err := mf.Encode(&buf); err != nil {
		t.Fatalf("encode: %v", err)
	}
	mf2, err := Parse(&buf)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	for i := 0; i < mf.Height; i++ {
		if mf.Walls[i] != mf2.Walls[i] {
			t.Errorf("walls row %d differs: %q vs %q", i, mf.Walls[i], mf2.Walls[i])
		}
		if mf.Floor[i] != mf2.Floor[i] {
			t.Errorf("floor row %d differs: %q vs %q", i, mf.Floor[i], mf2.Floor[i])
		}
		if mf.Decor[i] != mf2.Decor[i] {
			t.Errorf("decor row %d differs: %q vs %q", i, mf.Decor[i], mf2.Decor[i])
		}
		if mf.Props[i] != mf2.Props[i] {
			t.Errorf("props row %d differs: %q vs %q", i, mf.Props[i], mf2.Props[i])
		}
	}
	if len(mf.Packs) != len(mf2.Packs) {
		t.Fatalf("pack count: %d vs %d", len(mf.Packs), len(mf2.Packs))
	}
	for i := range mf.Packs {
		a, b := mf.Packs[i], mf2.Packs[i]
		if a.X != b.X || a.Z != b.Z || len(a.Members) != len(b.Members) {
			t.Fatalf("pack %d shape differs: %+v vs %+v", i, a, b)
		}
		for j := range a.Members {
			if a.Members[j] != b.Members[j] {
				t.Fatalf("pack %d member %d differs: %q vs %q", i, j, a.Members[j], b.Members[j])
			}
		}
	}
}

func TestRejectMismatchedLayerSize(t *testing.T) {
	bad := strings.Replace(sample, "size: 5x4", "size: 6x4", 1)
	if _, err := Parse(strings.NewReader(bad)); err == nil {
		t.Fatal("expected error for wrong width, got nil")
	}
}

func TestRejectMissingLayer(t *testing.T) {
	// Drop the props section — should fail validation since every layer
	// is mandatory and same-sized.
	withoutProps := strings.Replace(sample, "props:\n.....\n..T..\n.CRU.\n.....\n", "", 1)
	if _, err := Parse(strings.NewReader(withoutProps)); err == nil {
		t.Fatal("expected error for missing props layer, got nil")
	}
}

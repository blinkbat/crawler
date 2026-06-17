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

// TestCrystalsRoundTrip pins the optional crystals: section — it parses the
// position-only rows and re-encodes them byte-stably. Mirrors the doors /
// chests sections' backward-compat shape (absent section ⇒ no crystals).
func TestCrystalsRoundTrip(t *testing.T) {
	withCrystals := sample + "crystals:\n1 1\n3 2\n"
	mf, err := Parse(strings.NewReader(withCrystals))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(mf.Crystals) != 2 {
		t.Fatalf("crystal count: %d, want 2 (%+v)", len(mf.Crystals), mf.Crystals)
	}
	if mf.Crystals[0] != (MapCrystal{X: 1, Z: 1}) || mf.Crystals[1] != (MapCrystal{X: 3, Z: 2}) {
		t.Fatalf("crystal positions mismatch: %+v", mf.Crystals)
	}
	var buf bytes.Buffer
	if err := mf.Encode(&buf); err != nil {
		t.Fatalf("encode: %v", err)
	}
	mf2, err := Parse(&buf)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if len(mf2.Crystals) != len(mf.Crystals) {
		t.Fatalf("crystal count after round-trip: %d vs %d", len(mf2.Crystals), len(mf.Crystals))
	}
	for i := range mf.Crystals {
		if mf.Crystals[i] != mf2.Crystals[i] {
			t.Errorf("crystal %d differs: %+v vs %+v", i, mf.Crystals[i], mf2.Crystals[i])
		}
	}
	// A map with no crystals must not emit a crystals: section (byte-stable
	// with pre-crystal maps).
	plain, err := Parse(strings.NewReader(sample))
	if err != nil {
		t.Fatalf("parse plain: %v", err)
	}
	var pbuf bytes.Buffer
	if err := plain.Encode(&pbuf); err != nil {
		t.Fatalf("encode plain: %v", err)
	}
	if strings.Contains(pbuf.String(), "crystals:") {
		t.Fatal("a map with no crystals must not emit a crystals: section")
	}
}

// TestCrystalsEmptySectionRoundTrips pins that an explicit but empty
// crystals: section survives round-trip as "defined, zero rows" (the author
// deliberately wants no crystals) rather than collapsing to the absent-section
// legacy case.
func TestCrystalsEmptySectionRoundTrips(t *testing.T) {
	withEmpty := sample + "crystals:\n"
	mf, err := Parse(strings.NewReader(withEmpty))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !mf.CrystalsDefined {
		t.Fatal("an explicit crystals: section must set CrystalsDefined even with no rows")
	}
	if len(mf.Crystals) != 0 {
		t.Fatalf("expected zero crystal rows, got %d", len(mf.Crystals))
	}
	var buf bytes.Buffer
	if err := mf.Encode(&buf); err != nil {
		t.Fatalf("encode: %v", err)
	}
	if !strings.Contains(buf.String(), "crystals:") {
		t.Fatal("a defined-but-empty crystal set must still emit the crystals: section")
	}
	mf2, err := Parse(&buf)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if !mf2.CrystalsDefined || len(mf2.Crystals) != 0 {
		t.Fatalf("empty crystal section lost across round-trip: defined=%v rows=%d", mf2.CrystalsDefined, len(mf2.Crystals))
	}
}

// TestRejectCrystalOutOfBounds mirrors the pack/chest bounds guard: an
// out-of-range crystal row fails validation at parse rather than silently
// dropping at runtime.
func TestRejectCrystalOutOfBounds(t *testing.T) {
	bad := sample + "crystals:\n9 9\n"
	if _, err := Parse(strings.NewReader(bad)); err == nil {
		t.Fatal("expected error for out-of-bounds crystal, got nil")
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

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

// TestPerFloorSectionsRoundTrip pins the "@N" floor suffix on entity sections:
// levels are stamped on parse, survive a re-encode, and a multi-floor map groups
// entities back under one header per floor.
func TestPerFloorSectionsRoundTrip(t *testing.T) {
	perFloor := sample + // sample already has enemies at level 0 (rat 2 1, bat,rat 3 2)
		"enemies@3:\nbat 1 1\n" +
		"chests:\n(empty) 2 2\n" +
		"chests@2:\nrat 1 2\n" + // item token is free text; "rat" is fine here
		"doors@4:\ngate self gate2 2 1 north\n" +
		"crystals@5:\n3 1\n"
	mf, err := Parse(strings.NewReader(perFloor))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// Level 0 packs from sample, plus one at level 3.
	if len(mf.Packs) != 3 {
		t.Fatalf("pack count: %d, want 3", len(mf.Packs))
	}
	if mf.Packs[0].Level != 0 || mf.Packs[1].Level != 0 || mf.Packs[2].Level != 3 {
		t.Fatalf("pack levels: %d,%d,%d want 0,0,3", mf.Packs[0].Level, mf.Packs[1].Level, mf.Packs[2].Level)
	}
	if len(mf.Chests) != 2 || mf.Chests[0].Level != 0 || mf.Chests[1].Level != 2 {
		t.Fatalf("chest levels: %+v", mf.Chests)
	}
	if len(mf.Doors) != 1 || mf.Doors[0].Level != 4 {
		t.Fatalf("door level: %+v", mf.Doors)
	}
	if len(mf.Crystals) != 1 || mf.Crystals[0].Level != 5 {
		t.Fatalf("crystal level: %+v", mf.Crystals)
	}
	// Re-encode and re-parse: levels must survive.
	var buf bytes.Buffer
	if err := mf.Encode(&buf); err != nil {
		t.Fatalf("encode: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"enemies@3:", "chests@2:", "doors@4:", "crystals@5:"} {
		if !strings.Contains(out, want) {
			t.Errorf("encoded output missing %q section\n%s", want, out)
		}
	}
	mf2, err := Parse(&buf)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if mf2.Packs[2].Level != 3 || mf2.Chests[1].Level != 2 || mf2.Doors[0].Level != 4 || mf2.Crystals[0].Level != 5 {
		t.Fatalf("levels not preserved across round-trip: %+v", mf2)
	}
}

// TestSingleFloorOmitsFloorSuffix guards byte-stability: a map with only level-0
// entities must encode with NO "@N" headers (identical to the pre-feature form).
func TestSingleFloorOmitsFloorSuffix(t *testing.T) {
	mf, err := Parse(strings.NewReader(sample))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var buf bytes.Buffer
	if err := mf.Encode(&buf); err != nil {
		t.Fatalf("encode: %v", err)
	}
	if strings.Contains(buf.String(), "@") {
		t.Errorf("single-floor map emitted an @N section:\n%s", buf.String())
	}
}

// TestRejectFloorSuffixOnGrid rejects an "@N" suffix on a non-entity section.
func TestRejectFloorSuffixOnGrid(t *testing.T) {
	bad := strings.Replace(sample, "walls:", "walls@2:", 1)
	if _, err := Parse(strings.NewReader(bad)); err == nil {
		t.Fatalf("expected error for @N on a grid section")
	}
}

// TestCrystalsRoundTrip pins the optional crystals: section; absent ⇒ no crystals.
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
	// A map with no crystals must not emit a crystals: section.
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

// TestCrystalsEmptySectionRoundTrips: an explicit empty crystals: section survives as "defined, zero rows".
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

// TestPropStackRoundTrip pins the multi-plane propstack:/decorstack: sections:
// planes survive a re-encode, and a map without them emits nothing.
func TestPropStackRoundTrip(t *testing.T) {
	// 3 planes (level 0..2) of 4 rows × 5 cols each: a tree on floor 0 and a
	// boulder on floor 2 in the same column (1,1) — inexpressible as one grid.
	stacked := sample +
		"propstack:\n" +
		".....\n.T...\n.....\n.....\n" + // level 0
		".....\n.....\n.....\n.....\n" + // level 1
		".....\n.O...\n.....\n.....\n" // level 2
	mf, err := Parse(strings.NewReader(stacked))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(mf.PropStack) != 3 {
		t.Fatalf("propstack planes: %d, want 3", len(mf.PropStack))
	}
	if mf.PropStack[0][1] != ".T..." || mf.PropStack[2][1] != ".O..." {
		t.Fatalf("propstack content mismatch: %q / %q", mf.PropStack[0][1], mf.PropStack[2][1])
	}
	var buf bytes.Buffer
	if err := mf.Encode(&buf); err != nil {
		t.Fatalf("encode: %v", err)
	}
	if !strings.Contains(buf.String(), "propstack:") {
		t.Fatalf("encoded output missing propstack:\n%s", buf.String())
	}
	mf2, err := Parse(&buf)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if len(mf2.PropStack) != 3 || mf2.PropStack[2][1] != ".O..." {
		t.Fatalf("propstack lost across round-trip: %+v", mf2.PropStack)
	}
	// A plain map emits no propstack:/decorstack:.
	plain, _ := Parse(strings.NewReader(sample))
	var pbuf bytes.Buffer
	if err := plain.Encode(&pbuf); err != nil {
		t.Fatalf("encode plain: %v", err)
	}
	if strings.Contains(pbuf.String(), "propstack:") || strings.Contains(pbuf.String(), "decorstack:") {
		t.Fatalf("plain map emitted a scatter stack:\n%s", pbuf.String())
	}
}

// TestRejectCrystalOutOfBounds: an out-of-range crystal row fails parse.
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
	// Drop props — every layer is mandatory and same-sized.
	withoutProps := strings.Replace(sample, "props:\n.....\n..T..\n.CRU.\n.....\n", "", 1)
	if _, err := Parse(strings.NewReader(withoutProps)); err == nil {
		t.Fatal("expected error for missing props layer, got nil")
	}
}

// TestSolidsRoundTrip pins the optional solids: section: a floating cube parses N stacked Height-row planes, re-encodes byte-stably.
func TestSolidsRoundTrip(t *testing.T) {
	const gapped = sample + `solids:
00000
00000
00000
00000
00000
0####
0####
00000
00000
00#00
00000
00000
`
	mf, err := Parse(strings.NewReader(gapped))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(mf.Solids) != 3 {
		t.Fatalf("solids planes = %d, want 3", len(mf.Solids))
	}
	for L, plane := range mf.Solids {
		if len(plane) != mf.Height {
			t.Fatalf("plane %d has %d rows, want %d", L, len(plane), mf.Height)
		}
	}
	if mf.Solids[1][1] != "0####" {
		t.Fatalf("plane 1 row 1 = %q, want %q", mf.Solids[1][1], "0####")
	}
	if mf.Solids[2][1] != "00#00" {
		t.Fatalf("plane 2 row 1 = %q, want %q", mf.Solids[2][1], "00#00")
	}
	var buf bytes.Buffer
	if err := mf.Encode(&buf); err != nil {
		t.Fatalf("encode: %v", err)
	}
	mf2, err := Parse(&buf)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if len(mf2.Solids) != len(mf.Solids) {
		t.Fatalf("re-parsed plane count %d, want %d", len(mf2.Solids), len(mf.Solids))
	}
	for L := range mf.Solids {
		for i := range mf.Solids[L] {
			if mf.Solids[L][i] != mf2.Solids[L][i] {
				t.Errorf("plane %d row %d differs: %q vs %q", L, i, mf.Solids[L][i], mf2.Solids[L][i])
			}
		}
	}
}

// TestHeightfieldOmitsSolids confirms a map with no solids: round-trips without one.
func TestHeightfieldOmitsSolids(t *testing.T) {
	mf, err := Parse(strings.NewReader(sample))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(mf.Solids) != 0 {
		t.Fatalf("heightfield map parsed %d solids planes, want 0", len(mf.Solids))
	}
	var buf bytes.Buffer
	if err := mf.Encode(&buf); err != nil {
		t.Fatalf("encode: %v", err)
	}
	if strings.Contains(buf.String(), "solids:") {
		t.Fatalf("heightfield map encoded a solids: section:\n%s", buf.String())
	}
}

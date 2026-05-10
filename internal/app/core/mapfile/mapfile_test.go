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
layout:
#####
#...#
#.T.#
#####
enemies:
rat 2 1
bat 3 2
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
	if len(mf.Layout) != 4 || mf.Layout[2] != "#.T.#" {
		t.Fatalf("layout mismatch: %v", mf.Layout)
	}
	if len(mf.Enemies) != 2 || mf.Enemies[0].Kind != "rat" || mf.Enemies[1].Kind != "bat" {
		t.Fatalf("enemies mismatch: %+v", mf.Enemies)
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
	if mf2.Name != mf.Name || mf2.Width != mf.Width || mf2.Height != mf.Height ||
		mf2.StartX != mf.StartX || mf2.StartFace != mf.StartFace ||
		len(mf2.Layout) != len(mf.Layout) || len(mf2.Enemies) != len(mf.Enemies) {
		t.Fatalf("round-trip mismatch:\n  before: %+v\n  after:  %+v", mf, mf2)
	}
	for i := range mf.Layout {
		if mf.Layout[i] != mf2.Layout[i] {
			t.Fatalf("row %d differs: %q vs %q", i, mf.Layout[i], mf2.Layout[i])
		}
	}
}

func TestRejectMismatchedSize(t *testing.T) {
	bad := strings.Replace(sample, "size: 5x4", "size: 6x4", 1)
	if _, err := Parse(strings.NewReader(bad)); err == nil {
		t.Fatal("expected error for wrong width, got nil")
	}
}

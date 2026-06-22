package mapfile

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestRealForestMapStable confirms the shipped forest_path.map (a voxel-gap land
// bridge) is byte-stable through Parse->Encode, and emits no solids: when stripped.
func TestRealForestMapStable(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "maps", "forest_path.map")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("map not present: %v", err)
	}
	mf, err := Parse(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(mf.Solids) == 0 {
		t.Fatalf("forest map should carry a solids stack (the land bridge)")
	}
	var buf1 bytes.Buffer
	if err := mf.Encode(&buf1); err != nil {
		t.Fatalf("encode: %v", err)
	}
	enc1 := append([]byte(nil), buf1.Bytes()...) // snapshot: Parse below drains buf1
	if !bytes.Equal(raw, enc1) {
		t.Fatalf("re-encoding the shipped map changed its bytes (%d -> %d)", len(raw), len(enc1))
	}
	mf2, err := Parse(bytes.NewReader(enc1))
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	var buf2 bytes.Buffer
	if err := mf2.Encode(&buf2); err != nil {
		t.Fatalf("re-encode: %v", err)
	}
	if !bytes.Equal(enc1, buf2.Bytes()) {
		t.Fatalf("Parse->Encode not idempotent on a real map")
	}
	// Stripped of its stack the map is a heightfield: must encode with NO solids:.
	mf.Solids = nil
	var hf bytes.Buffer
	if err := mf.Encode(&hf); err != nil {
		t.Fatalf("encode heightfield: %v", err)
	}
	if bytes.Contains(hf.Bytes(), []byte("solids:")) {
		t.Fatalf("a gapless (heightfield) map encoded a spurious solids: section")
	}
}

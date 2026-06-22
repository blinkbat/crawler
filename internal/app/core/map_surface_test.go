package core

import "testing"

// TestMapSurfaceAtHeightfield: observer-relative map slice on a pure heightfield (Solids nil): same level → Floor, raised → Wall, lower → Below; ramp shows from both levels it joins.
func TestMapSurfaceAtHeightfield(t *testing.T) {
	a := elevTestArea()

	if s := a.MapSurfaceAt(0, 0, 0); s.Kind != MapSurfaceFloor {
		t.Errorf("flat ground at observer level: kind=%v, want Floor", s.Kind)
	}
	if s := a.MapSurfaceAt(1, 0, 0); s.Kind != MapSurfaceWall {
		t.Errorf("raised plateau from below: kind=%v, want Wall", s.Kind)
	}
	if s := a.MapSurfaceAt(1, 1, 0); s.Kind != MapSurfaceRamp {
		t.Errorf("ramp from its low level: kind=%v, want Ramp", s.Kind)
	}

	// Observer up on the plateau (level 1).
	if s := a.MapSurfaceAt(0, 0, 1); s.Kind != MapSurfaceBelow || s.Depth != 1 {
		t.Errorf("ground from the plateau: %+v, want Below depth 1", s)
	}
	if s := a.MapSurfaceAt(1, 0, 1); s.Kind != MapSurfaceFloor {
		t.Errorf("standing on the plateau: kind=%v, want Floor", s.Kind)
	}
	if s := a.MapSurfaceAt(1, 1, 1); s.Kind != MapSurfaceRamp {
		t.Errorf("ramp from the level above: kind=%v, want Ramp", s.Kind)
	}

	if s := a.MapSurfaceAt(9, 9, 0); s.Kind != MapSurfaceVoid {
		t.Errorf("out of bounds: kind=%v, want Void", s.Kind)
	}
}

// TestMapSurfaceAtVoxelOverhang — voxel-only cases: walk-under deck must NOT paint over the floor beneath, stacked column reads as wall, empty column is void.
func TestMapSurfaceAtVoxelOverhang(t *testing.T) {
	// Column: floor cube at 0, air at 1, deck cube at 2 — a walk-under deck.
	overhang := AreaDefinition{
		Width:  1,
		Height: 1,
		Floor:  []string{"."},
		Solids: [][]string{{"#"}, {"0"}, {"#"}},
	}
	if s := overhang.MapSurfaceAt(0, 0, 0); s.Kind != MapSurfaceFloor {
		t.Errorf("under an overhang: kind=%v, want Floor (the deck above must not show)", s.Kind)
	}

	// Two stacked cubes read as a wall from level 0.
	wall := AreaDefinition{
		Width:  1,
		Height: 1,
		Floor:  []string{"."},
		Solids: [][]string{{"#"}, {"#"}},
	}
	if s := wall.MapSurfaceAt(0, 0, 0); s.Kind != MapSurfaceWall {
		t.Errorf("two-cube wall: kind=%v, want Wall", s.Kind)
	}

	// All-air column: no surface at or below observer → void.
	empty := AreaDefinition{
		Width:  1,
		Height: 1,
		Floor:  []string{"."},
		Solids: [][]string{{"0"}},
	}
	if s := empty.MapSurfaceAt(0, 0, 0); s.Kind != MapSurfaceVoid {
		t.Errorf("empty column: kind=%v, want Void", s.Kind)
	}
}

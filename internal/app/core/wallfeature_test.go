package core

import "testing"

// TestWallSwitchOpensWallViaTrigger is the headline end-to-end: a wall switch sets a
// named switch; a Preserve trigger conditioned on that switch opens a wall tile. This
// is the StarEdit-style composition the whole feature is built for.
func TestWallSwitchOpensWallViaTrigger(t *testing.T) {
	g := &GameState{
		Area: AreaDefinition{
			Width: 5, Height: 5,
			Elevation: []string{"AAAAA", "AAAAA", "AAAAA", "AAAAA", "AAAAA"},
			WallFeatures: []WallFeature{
				{ID: "sw1", Kind: WallSwitch, X: 1, Z: 0, Dir: South, Switch: "gate"},
			},
			Triggers: []Trigger{{
				ID:         "openGate",
				Conditions: []Condition{{Kind: CondSwitch, Switch: "gate"}},
				Actions:    []Action{{Kind: ActionOpenWall, TileX: 3, TileZ: 0}},
				Preserve:   true,
			}},
		},
	}
	g.Player.TileX, g.Player.TileZ = 1, 1 // stand south of the switch, facing it (North)
	g.Player.Facing = North
	g.Area.Elevation[0] = "AAA" + string(ElevationChar(ElevationWallRingLevel)) + "A" // raise (3,0) into a wall

	// Nothing flipped yet: the gate wall stands.
	raised := g.Area.ElevationLevelAt(3, 0)
	if raised != ElevationWallRingLevel {
		t.Fatalf("precondition: (3,0) should be a raised wall (%d), got %d", ElevationWallRingLevel, raised)
	}

	idx := FacedWallFeature(g, true)
	if idx != 0 {
		t.Fatalf("party facing the switch should resolve feature 0, got %d", idx)
	}
	if !ActivateWallFeature(g, idx) {
		t.Fatal("activating the wall switch should succeed")
	}
	if !g.Switches["gate"] {
		t.Fatal("switch should have set the 'gate' flag")
	}
	// The Preserve trigger fired off the flag and opened (3,0) down to the party's level.
	if g.Area.ElevationLevelAt(3, 0) != g.Area.ElevationLevelAt(1, 1) {
		t.Fatalf("openWall should have lowered (3,0) to the party level %d, got %d",
			g.Area.ElevationLevelAt(1, 1), g.Area.ElevationLevelAt(3, 0))
	}
}

// TestWallSwitchToggles: a switch-kind fixture toggles its flag each activation; a
// bombable/secret sets it one-way.
func TestWallFeatureKindOps(t *testing.T) {
	g := &GameState{Area: AreaDefinition{Width: 3, Height: 3, WallFeatures: []WallFeature{
		{ID: "s", Kind: WallSwitch, X: 1, Z: 1, Switch: "s"},
		{ID: "b", Kind: WallBombable, X: 0, Z: 0, Switch: "b"},
	}}}
	ActivateWallFeature(g, 0)
	ActivateWallFeature(g, 0)
	if g.Switches["s"] {
		t.Fatal("a switch toggled twice should be back to false")
	}
	ActivateWallFeature(g, 1)
	ActivateWallFeature(g, 1)
	if !g.Switches["b"] {
		t.Fatal("a bombable wall sets its switch one-way (stays true)")
	}
}

// TestWallFeatureOnceStopsReactivation: a Once fixture won't re-activate.
func TestWallFeatureOnceStopsReactivation(t *testing.T) {
	g := &GameState{Area: AreaDefinition{Width: 3, Height: 3, WallFeatures: []WallFeature{
		{ID: "b", Kind: WallBombable, X: 0, Z: 0, Switch: "b", Once: true},
	}}}
	if !ActivateWallFeature(g, 0) {
		t.Fatal("first activation should succeed")
	}
	if ActivateWallFeature(g, 0) {
		t.Fatal("a Once fixture must not re-activate")
	}
}

func TestWallFeaturesJSONRoundTrip(t *testing.T) {
	in := []WallFeature{
		{ID: "wf1", Kind: WallSecret, X: 2, Z: 3, Dir: West, Switch: "hidden", Once: true},
		{ID: "wf2", Kind: WallSwitch, X: 0, Z: 0, Dir: North, Switch: "lever"},
	}
	lines, err := WallFeaturesToLines(in)
	if err != nil {
		t.Fatalf("WallFeaturesToLines: %v", err)
	}
	out, err := WallFeaturesFromLines(lines)
	if err != nil {
		t.Fatalf("WallFeaturesFromLines: %v", err)
	}
	if len(in) != len(out) {
		t.Fatalf("round-trip length mismatch: %d vs %d", len(in), len(out))
	}
	for i := range in {
		if in[i] != out[i] {
			t.Fatalf("round-trip mismatch at %d:\n in=%+v\nout=%+v", i, in[i], out[i])
		}
	}
}

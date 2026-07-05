package core

import (
	"reflect"
	"testing"
)

// atLocGame builds a GameState with one region + one atLocation-condition trigger
// (start dialog d1). preserve controls whether it re-fires.
func atLocGame(loc Location, preserve bool) *GameState {
	return &GameState{
		Area: AreaDefinition{
			Dialogs:   []DialogDefinition{sampleDialog()},
			Triggers:  []Trigger{{ID: "lt1", Conditions: []Condition{{Kind: CondAtLocation, LocationID: loc.ID}}, Actions: []Action{{Kind: ActionDialog, DialogID: "d1"}}, Preserve: preserve}},
			Locations: []Location{loc},
		},
	}
}

func TestAtLocationConditionFires(t *testing.T) {
	g := atLocGame(Location{ID: "loc1", X: 1, Z: 0, W: 2, H: 2}, false)
	g.Player.TileX, g.Player.TileZ = 5, 5
	EvaluateTriggers(g)
	if g.DialogOpen {
		t.Fatal("standing outside the region should not fire")
	}
	g.Player.TileX, g.Player.TileZ = 1, 0
	EvaluateTriggers(g)
	if !g.DialogOpen {
		t.Fatal("standing inside the region should open the dialog")
	}
	// fire-once: still inside, must not re-fire.
	CloseDialog(g)
	EvaluateTriggers(g)
	if g.DialogOpen {
		t.Fatal("a fire-once atLocation trigger should not re-fire while still inside")
	}
}

func TestAtLocationIsElevationSpecific(t *testing.T) {
	g := atLocGame(Location{ID: "loc1", X: 1, Z: 0, W: 2, H: 2, Level: 1}, false)
	g.Player.TileX, g.Player.TileZ, g.Player.Level = 1, 0, 0
	EvaluateTriggers(g)
	if g.DialogOpen {
		t.Fatal("a region on level 1 must not fire for a party standing on level 0")
	}
	g.Player.Level = 1
	EvaluateTriggers(g)
	if !g.DialogOpen {
		t.Fatal("standing in the region on its own level should fire")
	}
}

func TestLocationsJSONRoundTrip(t *testing.T) {
	locs := []Location{
		{ID: "camp", Name: "Bandit Camp", X: 1, Z: 2, W: 3, H: 4, Level: 2},
		{ID: "gate", X: 0, Z: 0, W: 1, H: 1},
	}
	lines, err := LocationsToLines(locs)
	if err != nil {
		t.Fatalf("LocationsToLines: %v", err)
	}
	got, err := LocationsFromLines(lines)
	if err != nil {
		t.Fatalf("LocationsFromLines: %v", err)
	}
	if !reflect.DeepEqual(locs, got) {
		t.Fatalf("round-trip mismatch:\n want %+v\n got  %+v", locs, got)
	}
}

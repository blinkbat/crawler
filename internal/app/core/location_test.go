package core

import (
	"reflect"
	"testing"
)

// enterLocGame builds a GameState with one region + one enterLocation trigger.
func enterLocGame(loc Location, once bool) *GameState {
	return &GameState{
		Area: AreaDefinition{
			Dialogs:   []DialogDefinition{sampleDialog()},
			Triggers:  []DialogTrigger{{ID: "lt1", Kind: DialogTriggerEnterLocation, DialogID: "d1", LocationID: loc.ID, Once: once}},
			Locations: []Location{loc},
		},
	}
}

func TestEnterLocationRisingEdge(t *testing.T) {
	g := enterLocGame(Location{ID: "loc1", X: 1, Z: 0, W: 2, H: 2}, false)
	if FireEnterLocationTriggers(g, 5, 5, 0) {
		t.Fatal("standing outside the region should not fire")
	}
	if !FireEnterLocationTriggers(g, 1, 0, 0) || !g.DialogOpen {
		t.Fatal("crossing into the region should open the dialog")
	}
	CloseDialog(g)
	// Still inside (moved within the region) — must not re-fire.
	if FireEnterLocationTriggers(g, 2, 1, 0) {
		t.Fatal("staying inside the region should not re-fire (rising edge only)")
	}
	// Leave, then re-enter: a non-Once trigger fires again on the new crossing.
	FireEnterLocationTriggers(g, 5, 5, 0)
	if !FireEnterLocationTriggers(g, 1, 0, 0) {
		t.Fatal("re-entering should fire a non-Once trigger again")
	}
}

func TestEnterLocationOnceDoesNotRepeat(t *testing.T) {
	g := enterLocGame(Location{ID: "loc1", X: 0, Z: 0, W: 1, H: 1}, true)
	if !FireEnterLocationTriggers(g, 0, 0, 0) || !g.DialogOpen {
		t.Fatal("first crossing should fire")
	}
	CloseDialog(g)
	FireEnterLocationTriggers(g, 5, 5, 0) // leave
	if FireEnterLocationTriggers(g, 0, 0, 0) {
		t.Fatal("a Once enter-location trigger must not fire on a second crossing")
	}
}

func TestEnterLocationIsElevationSpecific(t *testing.T) {
	g := enterLocGame(Location{ID: "loc1", X: 1, Z: 0, W: 2, H: 2, Level: 1}, false)
	// Same tile, wrong level: the region is on level 1, the player on the ground.
	if FireEnterLocationTriggers(g, 1, 0, 0) {
		t.Fatal("a region on level 1 must not fire for a player standing on level 0")
	}
	if !FireEnterLocationTriggers(g, 1, 0, 1) || !g.DialogOpen {
		t.Fatal("entering the region on its own level should fire")
	}
}

func TestEnterLocationDefersWhileDialogOpen(t *testing.T) {
	g := enterLocGame(Location{ID: "loc1", X: 0, Z: 0, W: 2, H: 2}, false)
	g.DialogOpen = true // a tile trigger opened a dialog on this same step
	if FireEnterLocationTriggers(g, 0, 0, 0) {
		t.Fatal("must not fire while a dialog is open")
	}
	if g.InsideLocations["loc1"] {
		t.Fatal("the crossing must not be recorded as inside while deferred")
	}
	// Dialog closes; the same crossing now fires (wasn't consumed).
	g.DialogOpen = false
	if !FireEnterLocationTriggers(g, 0, 0, 0) || !g.DialogOpen {
		t.Fatal("the deferred crossing should fire once the dialog closes")
	}
}

func TestEnterLocationOneDialogPerStep(t *testing.T) {
	// Two overlapping regions both entered on the same tile; only one fires this
	// step, the other is left unrecorded so it fires on the next step.
	g := &GameState{
		Area: AreaDefinition{
			Dialogs: []DialogDefinition{sampleDialog()},
			Triggers: []DialogTrigger{
				{ID: "a", Kind: DialogTriggerEnterLocation, DialogID: "d1", LocationID: "locA"},
				{ID: "b", Kind: DialogTriggerEnterLocation, DialogID: "d1", LocationID: "locB"},
			},
			Locations: []Location{
				{ID: "locA", X: 0, Z: 0, W: 2, H: 2},
				{ID: "locB", X: 0, Z: 0, W: 2, H: 2},
			},
		},
	}
	if !FireEnterLocationTriggers(g, 0, 0, 0) || !g.DialogOpen {
		t.Fatal("first region should fire on the crossing")
	}
	if g.InsideLocations["locB"] {
		t.Fatal("the second region's crossing must be deferred, not recorded")
	}
	CloseDialog(g)
	if !FireEnterLocationTriggers(g, 0, 0, 0) {
		t.Fatal("the deferred second region should fire on the next step")
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

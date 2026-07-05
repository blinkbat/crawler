package editor

import (
	"testing"

	"crawler/internal/app/core"
)

// TestLocationEditorWiring runs the editor package's init() (the modalHandlers /
// dropdownEntryBuilders / triggerKindLabel coverage asserts) and checks the
// Locations authoring surface is registered.
func TestLocationEditorWiring(t *testing.T) {
	if _, ok := modalHandlers[modalLocationEdit]; !ok {
		t.Error("modalLocationEdit has no modalHandlers entry")
	}
	if _, ok := dropdownEntryBuilders[ddDialogTriggerLocation]; !ok {
		t.Error("ddDialogTriggerLocation has no dropdownEntryBuilders entry")
	}
	if conditionKindLabel(core.CondAtLocation) == string(core.CondAtLocation) {
		t.Error("conditionKindLabel missing a case for atLocation")
	}
}

// TestCreateAndDeleteLocation exercises the right-click create path + the modal
// delete, including the auto-id and the editor selecting the new region.
func TestCreateAndDeleteLocation(t *testing.T) {
	area := core.AreaDefinition{Width: 10, Height: 10}
	s := freshState(area)
	createLocationAt(&s, 2, 3)
	if len(s.area.Locations) != 1 {
		t.Fatalf("createLocationAt: want 1 region, got %d", len(s.area.Locations))
	}
	loc := s.area.Locations[0]
	if loc.ID != "location_1" || loc.X != 2 || loc.Z != 3 {
		t.Fatalf("unexpected region: %+v", loc)
	}
	if s.modal != modalLocationEdit || s.modalLocationIdx != 0 {
		t.Fatalf("create should open the editor on the new region (modal=%d idx=%d)", s.modal, s.modalLocationIdx)
	}
	deleteCurrentLocation(&s)
	if len(s.area.Locations) != 0 {
		t.Fatalf("deleteCurrentLocation: want 0 regions, got %d", len(s.area.Locations))
	}
}

// TestLocationClampKeepsRegionOnMap verifies the stepper clamp can't push a region
// off the grid or below 1×1.
func TestLocationClampKeepsRegionOnMap(t *testing.T) {
	area := core.AreaDefinition{Width: 8, Height: 8}
	s := freshState(area)
	loc := &core.Location{X: 6, Z: 6, W: 5, H: 5, Level: 99}
	clampLocation(&s, loc)
	if loc.W > s.area.Width || loc.X+loc.W > s.area.Width || loc.Z+loc.H > s.area.Height {
		t.Fatalf("clamp left region off-map: %+v", *loc)
	}
	if loc.W < 1 || loc.H < 1 {
		t.Fatalf("clamp produced a degenerate region: %+v", *loc)
	}
	if loc.Level > maxEditLevel {
		t.Fatalf("clamp left level out of range: %d", loc.Level)
	}
}

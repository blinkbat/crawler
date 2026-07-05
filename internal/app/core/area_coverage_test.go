package core

import (
	"reflect"
	"strings"
	"testing"
)

// TestAreaDefinitionFieldsTracked is a tripwire over the two hand-maintained
// field rosters AreaContentEqual and CloneArea each walk independently. Adding an
// AreaDefinition field without updating BOTH silently breaks editor dirty
// detection / undo snapshots. When this fails: handle the field in
// AreaContentEqual and CloneArea, then add it to the set below.
func TestAreaDefinitionFieldsTracked(t *testing.T) {
	tracked := map[string]bool{
		// Ignored by AreaContentEqual (Path) / lazy cache (faceOverrideIdx), but
		// still must be a conscious choice — listed so a rename trips this test.
		"Path":            true,
		"faceOverrideIdx": true,

		"Name": true, "Width": true, "Height": true,
		"Walls": true, "Floor": true, "Decor": true, "Props": true,
		"Ceiling": true, "Elevation": true, "Solids": true,
		"PropLevels": true, "DecorLevels": true, "PropYaw": true, "FaceOverrides": true,
		"PropStack": true, "DecorStack": true,
		"Materials":   true,
		"WeatherMode": true,
		"StartTileX":  true, "StartTileZ": true, "StartFacing": true,
		"PackSpawns": true, "ChestSpawns": true, "DoorSpawns": true,
		"CrystalSpawns": true, "CrystalsAuthored": true,
		"QuietMessage": true, "Dialogs": true, "Triggers": true, "Locations": true,
		"WallFeatures": true,
	}
	tp := reflect.TypeOf(AreaDefinition{})
	for i := 0; i < tp.NumField(); i++ {
		if name := tp.Field(i).Name; !tracked[name] {
			t.Errorf("AreaDefinition.%s is untracked — handle it in AreaContentEqual and CloneArea, then add it here", name)
		}
	}
	if tp.NumField() != len(tracked) {
		t.Errorf("tracked set has %d names but AreaDefinition has %d fields — a tracked field was removed", len(tracked), tp.NumField())
	}
}

// TestSpawnSummaryProbesCoverAllSpawns ties spawnSummaryProbes to the *Spawns
// fields on AreaDefinition, so a new spawn list can't silently fail to surface in
// AreaTileSummary.
func TestSpawnSummaryProbesCoverAllSpawns(t *testing.T) {
	tp := reflect.TypeOf(AreaDefinition{})
	spawnFields := 0
	for i := 0; i < tp.NumField(); i++ {
		if strings.HasSuffix(tp.Field(i).Name, "Spawns") {
			spawnFields++
		}
	}
	if spawnFields != len(spawnSummaryProbes) {
		t.Errorf("AreaDefinition has %d *Spawns fields but spawnSummaryProbes has %d rows — add a probe for the new spawn type", spawnFields, len(spawnSummaryProbes))
	}
	seen := make(map[string]bool, len(spawnSummaryProbes))
	for _, p := range spawnSummaryProbes {
		if p.label == "" || p.present == nil {
			t.Error("spawnSummaryProbes has an incomplete row (empty label or nil probe)")
		}
		if seen[p.label] {
			t.Errorf("duplicate spawn probe label %q", p.label)
		}
		seen[p.label] = true
	}
}

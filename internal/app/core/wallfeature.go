package core

// Wall features are interactive fixtures mounted on a specific wall FACE — a tile
// (X,Z) plus a cardinal direction (like a FaceOverride). They are the spatial event
// sources of the trigger system: activating one sets its named Switch (StarEdit
// style), then re-evaluates triggers, so every downstream effect (openWall, spawnFoe,
// dialog, …) composes through the general engine rather than a bespoke wall path.
//
//   - WallSwitch:   a lever. Face it + Use → TOGGLES its Switch (reusable).
//   - WallBombable: a cracked wall. Face it + Use → SETS its Switch (a one-way blast).
//   - WallSecret:   looks solid. BUMP it (walk into it) → SETS its Switch (found).
//
// Author a trigger like {conditions: switch S set; actions: openWall (x,z); preserve}
// to turn the flipped switch into an opened passage that survives save/load.

// WallFeatureKind tags the fixture + how the party activates it.
type WallFeatureKind string

const (
	WallSwitch   WallFeatureKind = "switch"
	WallBombable WallFeatureKind = "bombable"
	WallSecret   WallFeatureKind = "secret"
)

// WallFeatureKinds returns the kinds in canonical (editor display) order.
func WallFeatureKinds() []WallFeatureKind {
	return []WallFeatureKind{WallSwitch, WallBombable, WallSecret}
}

// WallFeatureKindLabel is the editor/UI label for a kind.
func WallFeatureKindLabel(k WallFeatureKind) string {
	switch k {
	case WallBombable:
		return "Bombable Wall"
	case WallSecret:
		return "Secret Wall"
	default:
		return "Wall Switch"
	}
}

// WallFeature is one fixture on tile (X,Z)'s Dir face (0=N,1=E,2=S,3=W). Activating
// it sets/toggles Switch and re-evaluates triggers. Once = it fires only the first
// time (tracked in TriggersFired by its namespaced id).
type WallFeature struct {
	ID     string          `json:"id"`
	Kind   WallFeatureKind `json:"kind"`
	X      int             `json:"x"`
	Z      int             `json:"z"`
	Dir    int             `json:"dir,omitempty"` // face direction: 0=N,1=E,2=S,3=W
	Switch string          `json:"switch,omitempty"`
	Once   bool            `json:"once,omitempty"`
}

// UsesBump reports whether the feature activates by walking INTO it (secret walls)
// rather than by a Use press while facing it (switches / bombable).
func (f WallFeature) UsesBump() bool { return f.Kind == WallSecret }

// wallFeatureFiredKey namespaces a feature's id in the shared TriggersFired set so a
// Once feature and a trigger can't collide on the same id.
func wallFeatureFiredKey(id string) string { return "wall:" + id }

// WallFeaturesToLines marshals wall features for the .map wallfeatures: section.
func WallFeaturesToLines(fs []WallFeature) ([]string, error) {
	return jsonObjectsToLines(fs, "wallfeature", func(f WallFeature) string { return f.ID })
}

// WallFeaturesFromLines unmarshals the .map wallfeatures: section into wall features.
func WallFeaturesFromLines(lines []string) ([]WallFeature, error) {
	return jsonObjectsFromLines[WallFeature](lines, "wallfeature")
}

// WallFeatureIndexAt returns the index of a feature at (x,z) on the dir face, or -1.
func WallFeatureIndexAt(fs []WallFeature, x, z, dir int) int {
	for i, f := range fs {
		if f.X == x && f.Z == z && f.Dir == dir {
			return i
		}
	}
	return -1
}

// WallFeatureAnyAt returns the index of the FIRST feature on any face of (x,z), or
// -1 (used by the editor's "is there a fixture here?" checks and rendering).
func WallFeatureAnyAt(fs []WallFeature, x, z int) int {
	for i, f := range fs {
		if f.X == x && f.Z == z {
			return i
		}
	}
	return -1
}

// FacedWallFeature returns the wall feature the party is currently looking at (the
// fixture on the front tile's face pointing back at the party), or -1. useOnly limits
// the match to Use-activated features (switch/bombable); when false it matches any
// (used by the bump path for secret walls). The front tile is one step ahead in the
// party's facing; the fixture faces the OPPOSITE way (toward the party).
func FacedWallFeature(g *GameState, useOnly bool) int {
	if g == nil {
		return -1
	}
	dx, dz := FacingVector(g.Player.Facing)
	fx, fz := g.Player.TileX+dx, g.Player.TileZ+dz
	faceDir := OppositeFacing(g.Player.Facing) // the front tile's face toward the party
	idx := WallFeatureIndexAt(g.Area.WallFeatures, fx, fz, faceDir)
	if idx < 0 {
		return -1
	}
	if useOnly && g.Area.WallFeatures[idx].UsesBump() {
		return -1
	}
	return idx
}

// ActivateWallFeature fires the feature at index idx: sets/toggles its Switch and
// re-evaluates triggers. A Once feature that already fired is a no-op. Returns true
// if it activated (so the caller can play a cue / consume the input).
func ActivateWallFeature(g *GameState, idx int) bool {
	if g == nil || idx < 0 || idx >= len(g.Area.WallFeatures) {
		return false
	}
	f := g.Area.WallFeatures[idx]
	if f.Once && g.TriggersFired[wallFeatureFiredKey(f.ID)] {
		return false
	}
	if f.Switch != "" {
		if g.Switches == nil {
			g.Switches = map[string]bool{}
		}
		if f.Kind == WallSwitch {
			g.Switches[f.Switch] = !g.Switches[f.Switch] // a lever toggles
		} else {
			g.Switches[f.Switch] = true // bombable / secret set one-way
		}
	}
	if f.Once {
		markTriggerFired(g, wallFeatureFiredKey(f.ID))
	}
	EvaluateTriggers(g)
	return true
}

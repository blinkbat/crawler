package core

// Locations are named, elevation-specific rectangular regions of a map's tile
// grid (the spatial counterpart to a single enterTile). The CondAtLocation trigger
// condition holds while the party stands inside a region on its level; rising-edge
// crossings are tracked in GameState.InsideLocations so a location-gated trigger
// can fire once per crossing rather than every step inside.

// Location is one named region: a tile-AABB [X, X+W) × [Z, Z+H) on elevation Level.
type Location struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
	X    int    `json:"x"`
	Z    int    `json:"z"`
	W    int    `json:"w"`
	H    int    `json:"h"`
	// Level is the elevation level the region occupies (like prop/decor levels);
	// containment requires the unit to stand on this level, so stacked floors don't
	// share a region. 0 = ground (omitted from JSON when zero).
	Level int `json:"level,omitempty"`
}

// Contains reports whether tile (x,z) on level lvl falls inside the region.
func (l Location) Contains(x, z, lvl int) bool {
	return lvl == l.Level && x >= l.X && x < l.X+l.W && z >= l.Z && z < l.Z+l.H
}

// LocationByID returns the named location, or (zero, false). Linear — region lists
// are short and authored, so no index is built.
func LocationByID(locs []Location, id string) (Location, bool) {
	for _, l := range locs {
		if l.ID == id {
			return l, true
		}
	}
	return Location{}, false
}

// LocationIndexAt returns the first region containing (x,z) on level, or -1.
// The editor uses it to resolve a right-click to the region under the cursor.
func LocationIndexAt(locs []Location, x, z, level int) int {
	for i, l := range locs {
		if l.Contains(x, z, level) {
			return i
		}
	}
	return -1
}

// LocationsToLines marshals locations for the .map locations: section (one JSON per line).
func LocationsToLines(locs []Location) ([]string, error) {
	return jsonObjectsToLines(locs, "location", func(l Location) string { return l.ID })
}

// LocationsFromLines unmarshals the .map locations: section into locations.
func LocationsFromLines(lines []string) ([]Location, error) {
	return jsonObjectsFromLines[Location](lines, "location")
}

// SeedLocationPresence rebuilds the inside-region set from the player's current
// tile+level, so a freshly-entered area doesn't fire an enter trigger for a region
// the player simply spawned inside — only a later crossing fires. Call on every
// player (re)placement (new game, area transition, door reposition).
func SeedLocationPresence(g *GameState) {
	if g == nil {
		return
	}
	g.InsideLocations = make(map[string]bool, len(g.Area.Locations))
	for _, loc := range g.Area.Locations {
		g.InsideLocations[loc.ID] = loc.Contains(g.Player.TileX, g.Player.TileZ, g.Player.Level)
	}
}

// Region entry is handled by the general trigger engine: an `atLocation` condition
// (trigger.go) tests whether the party stands inside a named region, and
// EvaluateTriggers polls it on each step landing. InsideLocations / SeedLocationPresence
// are retained below only as a cheap presence cache for any UI that wants it.

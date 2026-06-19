package core

import (
	"slices"
	"strings"
)

// Voxel occupancy — the cube model that supersedes the single-height Elevation
// heightfield. A column (x,z) is a stack of cells, each SOLID (a cube, whose
// char is its material/skin) or AIR. This is what lets a map express a floating
// box / bridge / overhang: solid@0, AIR@1, solid@2 is a deck you walk UNDER at
// level 0 and OVER at level 2 — geometry the one-height-per-tile model could
// never hold.
//
// Migration discipline (see voxel staging in the plan): AreaDefinition.Solids
// stays NIL for a pure heightfield. Every accessor here then DERIVES occupancy
// from the legacy Elevation layer (a column is solid from level 0 up to its
// stored top), so a flat/legacy map answers exactly as it did before — and the
// editor's elevation brush, which writes Elevation, stays authoritative. Solids
// is non-nil only for a map that actually has a gap (loaded from a solids:
// section, or authored with the cube tool). This keeps the conversion staged:
// nothing changes for existing maps until a gap is introduced.

// SolidAir is the cell char meaning "no cube here" (empty air). It coincides
// with ElevationGround ('0') — a blank elevation cell and an air voxel share the
// sentinel — and is deliberately disjoint from the face-skin alphabet
// ('#','+','=','&','$', see FaceSkins) that marks a solid cube's material, so a
// solids plane never confuses a cube skin with empty space.
const SolidAir = ElevationGround

// IsVoxel reports whether the area carries a materialized voxel stack (a non-nil
// Solids), as opposed to a pure heightfield that derives occupancy from
// Elevation. The single home for the "is this a voxel map?" test that gates the
// ResolveStep / PackIndexAtTileLevel paths against their heightfield fallbacks —
// so changing how a voxel map is detected isn't a hunt across call sites.
func (a *AreaDefinition) IsVoxel() bool { return len(a.Solids) > 0 }

// SolidAt reports whether a solid cube occupies (x, level, z) and, if so, the
// cube's material/skin char. THE single read path into the voxel stack: when
// Solids is populated it indexes the plane directly; when Solids is nil (a pure
// heightfield) it derives occupancy from Elevation — solid from level 0 up to
// ElevationLevelAt inclusive, skinned by FaceSkinAt — so callers get identical
// answers whether or not the stack has been materialized. Out-of-bounds (x,z),
// a negative level, or a level above the tallest stored plane read as air.
func (a *AreaDefinition) SolidAt(x, level, z int) (skin byte, solid bool) {
	if level < 0 || !a.InBounds(x, z) {
		return 0, false
	}
	if len(a.Solids) > 0 {
		if level >= len(a.Solids) {
			return 0, false // above the tallest stored plane: implicit air
		}
		c, ok := a.layerByteAt(a.Solids[level], x, z)
		if !ok || c == SolidAir {
			return 0, false
		}
		return c, true
	}
	// Heightfield fallback: a column is solid from 0 up to its stored top. Ramp
	// tiles store their LOW level (the wedge above is drawn separately), so the
	// solid block ends at the low level — identical to today's render/movement.
	if level <= a.ElevationLevelAt(x, z) {
		return a.FaceSkinAt(x, z), true
	}
	return 0, false
}

// TopSolidLevel returns the highest level with a solid cube in column (x,z), or
// -1 for a wholly empty column (only reachable in explicit-Solids mode). In
// heightfield mode it is exactly ElevationLevelAt.
func (a *AreaDefinition) TopSolidLevel(x, z int) int {
	if !a.InBounds(x, z) {
		return -1
	}
	if len(a.Solids) > 0 {
		for L := len(a.Solids) - 1; L >= 0; L-- {
			if _, solid := a.SolidAt(x, L, z); solid {
				return L
			}
		}
		return -1
	}
	return a.ElevationLevelAt(x, z)
}

// maxElevationTop returns the tallest ElevationLevelAt across every column (0 if
// the area is flat/empty). Shared by SolidStackHeight and BuildSolidsFromElevation
// so the heightfield top scan lives in one place.
func (a *AreaDefinition) maxElevationTop() int {
	top := 0
	for z := 0; z < a.Height; z++ {
		for x := 0; x < a.Width; x++ {
			if t := a.ElevationLevelAt(x, z); t > top {
				top = t
			}
		}
	}
	return top
}

// SolidStackHeight is the number of levels worth iterating for column walks
// (rendering, reachability): the count of stored planes when Solids is set,
// else one past the tallest heightfield column. Always >= 1.
func (a *AreaDefinition) SolidStackHeight() int {
	if len(a.Solids) > 0 {
		return len(a.Solids)
	}
	return a.maxElevationTop() + 1
}

// Standable reports whether a unit can stand atop level L in column (x,z): the
// cube at L is solid AND L+1 is air (headroom for the body). The unit rests on
// cube L's top face and occupies L+1's air.
func (a *AreaDefinition) Standable(x, L, z int) bool {
	if _, solid := a.SolidAt(x, L, z); !solid {
		return false
	}
	_, above := a.SolidAt(x, L+1, z)
	return !above
}

// LowestStandableLevel returns the lowest level a unit can stand on in column
// (x,z), or -1 if none. Used to seed a unit's level when only its (x,z) is
// known (legacy save without a stored level, authored start tile).
func (a *AreaDefinition) LowestStandableLevel(x, z int) int {
	h := a.SolidStackHeight()
	for L := 0; L < h; L++ {
		if a.Standable(x, L, z) {
			return L
		}
	}
	return -1
}

// BuildSolidsFromElevation materializes the voxel stack for a heightfield area:
// column (x,z) becomes solid from level 0 up to ElevationLevelAt(x,z), each cube
// skinned by FaceSkinAt so cliff faces look identical. The inverse of reading
// tops back out (see ElevationRowsFromSolids). Returns planes 0..maxTop; the
// top plane always carries at least one cube, so there are no trailing all-air
// planes.
func BuildSolidsFromElevation(a *AreaDefinition) [][]string {
	maxTop := a.maxElevationTop()
	out := make([][]string, maxTop+1)
	for L := 0; L <= maxTop; L++ {
		rows := make([]string, a.Height)
		for z := 0; z < a.Height; z++ {
			buf := make([]byte, a.Width)
			for x := 0; x < a.Width; x++ {
				if a.ElevationLevelAt(x, z) >= L {
					buf[x] = a.FaceSkinAt(x, z)
				} else {
					buf[x] = SolidAir
				}
			}
			rows[z] = string(buf)
		}
		out[L] = rows
	}
	return out
}

// ElevationRowsFromSolids projects an explicit voxel stack down to the legacy
// single-height elevation layer (the TOP solid level per column, as a base-36
// char). Written alongside an authored solids: section as a graceful-downgrade
// elevation: section, so a build that ignores solids: still sees the column
// tops as a heightfield rather than a flat floor. An empty column writes ground.
func ElevationRowsFromSolids(a *AreaDefinition) []string {
	rows := make([]string, a.Height)
	for z := 0; z < a.Height; z++ {
		buf := make([]byte, a.Width)
		for x := 0; x < a.Width; x++ {
			top := a.TopSolidLevel(x, z)
			if top < 0 {
				top = 0
			}
			buf[x] = ElevationChar(top)
		}
		rows[z] = string(buf)
	}
	return rows
}

// surfaceEdgeLevel returns the elevation level a unit standing atop level L in
// column (x,z) presents across its edge toward `dir`, and ok=false when that
// edge isn't walkable (a ramp's sheer perpendicular side). A flat surface
// presents L on every edge; a ramp ground tile presents its high/low edges via
// the shared EdgeLevelOf rule (ramps stay ground-only, so L is the ramp's stored
// low level). The single rule ResolveStep uses on both the leaving and entering
// side, so the cliff/ramp math can't drift from the heightfield edgeLevel.
func (a *AreaDefinition) surfaceEdgeLevel(x, L, z, dir int) (int, bool) {
	if f, ok := a.RampAt(x, z); ok {
		return EdgeLevelOf(L, f, dir)
	}
	return L, true
}

// ResolveStep resolves a step from standing on level `fromL` in column
// (fromX,fromZ) heading cardinal `dir`: it returns the destination standing
// level in the neighbour column, or ok=false if the step is blocked (a cliff,
// the side of a cube = a wall, no reachable surface, or a sheer ramp side).
//
// The rule generalizes StepElevationOK to a column with MORE THAN ONE walkable
// surface: you land on the neighbour's standable surface whose entry edge level
// equals the edge level you leave from. Standable surfaces in a column are
// separated by an air gap (>= 2 levels apart), so at most one matches — which
// is what makes "ground under a bridge vs the bridge deck" unambiguous: from the
// ground you match the neighbour's ground (walk UNDER the deck); from the deck
// you match the deck. For a heightfield column (one surface = the top) this is
// provably identical to StepElevationOK.
func (a *AreaDefinition) ResolveStep(fromX, fromL, fromZ, dir int) (toL int, ok bool) {
	exitEdge, exitOK := a.surfaceEdgeLevel(fromX, fromL, fromZ, dir)
	if !exitOK {
		return 0, false
	}
	dx, dz := FacingVector(dir)
	nx, nz := fromX+dx, fromZ+dz
	if !a.InBounds(nx, nz) {
		return 0, false
	}
	fromDir := OppositeFacing(dir)
	h := a.SolidStackHeight()
	for L := 0; L < h; L++ {
		if !a.Standable(nx, L, nz) {
			continue
		}
		if entry, eok := a.surfaceEdgeLevel(nx, L, nz, fromDir); eok && entry == exitEdge {
			return L, true
		}
	}
	return 0, false
}

// GroundSpawnLevel is the exported wrapper over spawnLevel: the standing level a
// unit placed at (x,z) should occupy (lowest standable surface, else the column
// top). Used by out-of-package placement paths (door transitions) that build a
// player without a saved level — the heightfield case returns the column top, so
// it's a no-op there.
func (a *AreaDefinition) GroundSpawnLevel(x, z int) int {
	return spawnLevel(a, x, z)
}

// --- Editor authoring primitives -------------------------------------------

// EnsureSolids materializes the voxel stack from the heightfield Elevation layer
// if it isn't already explicit, so an editor cube edit has a stack to write
// into. No-op when Solids is already present. After this, ElevationLevelAt reads
// the stack, so the editor's elevation edits must route through the Solids-aware
// helpers (SetColumnTop / SetCube / ClearCube) rather than the Elevation layer.
func EnsureSolids(a *AreaDefinition) {
	if len(a.Solids) == 0 {
		a.Solids = BuildSolidsFromElevation(a)
	}
}

func blankSolidPlane(w, h int) []string {
	rows := make([]string, h)
	air := strings.Repeat(string(SolidAir), w)
	for i := range rows {
		rows[i] = air
	}
	return rows
}

// growSolidsTo pads the stack to at least n planes with all-air planes.
func (a *AreaDefinition) growSolidsTo(n int) {
	for len(a.Solids) < n {
		a.Solids = append(a.Solids, blankSolidPlane(a.Width, a.Height))
	}
}

// trimTopAir drops trailing all-air planes so the stack height tracks the
// tallest cube (keeps round-trips and SolidStackHeight tight). Never trims
// below one plane.
func (a *AreaDefinition) trimTopAir() {
	for len(a.Solids) > 1 && planeAllAir(a.Solids[len(a.Solids)-1]) {
		a.Solids = a.Solids[:len(a.Solids)-1]
	}
}

// setSolidCell writes char c at (x,level,z), materializing + growing the stack
// and normalizing the affected row to Width first.
func (a *AreaDefinition) setSolidCell(x, level, z int, c byte) {
	if level < 0 || !a.InBounds(x, z) {
		return
	}
	EnsureSolids(a)
	a.growSolidsTo(level + 1)
	row := []byte(a.Solids[level][z])
	for len(row) < a.Width {
		row = append(row, SolidAir)
	}
	row[x] = c
	a.Solids[level][z] = string(row)
}

// SetCube places a solid cube at (x,level,z), skinned by `skin` (an air char is
// coerced to plain rock so a placed cube is always solid). The editor's "place a
// floating cube" primitive — this is what authors a bridge/overhang.
func (a *AreaDefinition) SetCube(x, level, z int, skin byte) {
	if skin == SolidAir {
		skin = TileRock
	}
	a.setSolidCell(x, level, z, skin)
}

// ClearCube removes the cube at (x,level,z) (sets it to air), trimming any
// trailing all-air planes the removal exposed.
func (a *AreaDefinition) ClearCube(x, level, z int) {
	a.setSolidCell(x, level, z, SolidAir)
	a.trimTopAir()
}

// SetColumnTop sets column (x,z) solid from level 0 up to `top` and air above —
// the voxel-aware "Set Height" the editor uses once a map has a stack (so a
// height edit can't desync from the Elevation layer the renderer no longer
// reads). Any floating cube above `top` in this column is cleared, matching the
// heightfield "set the ground height" intent. The cube skin is FaceSkinAt.
func (a *AreaDefinition) SetColumnTop(x, z, top int) {
	if !a.InBounds(x, z) || top < 0 {
		return
	}
	EnsureSolids(a)
	a.growSolidsTo(top + 1)
	skin := a.FaceSkinAt(x, z)
	for L := 0; L < len(a.Solids); L++ {
		if L <= top {
			a.setSolidCell(x, L, z, skin)
		} else {
			a.setSolidCell(x, L, z, SolidAir)
		}
	}
	a.trimTopAir()
}

// CloneSolids deep-copies a voxel stack (nil-safe). string rows are immutable,
// so copying the plane slices is a full deep copy. Returns nil for an empty
// stack so a heightfield area keeps Solids==nil.
func CloneSolids(in [][]string) [][]string {
	if len(in) == 0 {
		return nil
	}
	out := make([][]string, len(in))
	for L := range in {
		out[L] = append([]string(nil), in[L]...)
	}
	return out
}

// columnGapless reports whether column (x,z) is a pure heightfield run — solid
// from 0 up to its top with no air gap (so it can be encoded by a single
// elevation char). An empty column is trivially gapless.
func (a *AreaDefinition) columnGapless(x, z int) bool {
	top := a.TopSolidLevel(x, z)
	if top < 0 {
		return true
	}
	for L := 0; L <= top; L++ {
		if _, solid := a.SolidAt(x, L, z); !solid {
			return false
		}
	}
	return true
}

// AllColumnsGapless reports whether every column is gapless — i.e. the whole
// area is expressible as a heightfield and can round-trip to disk as the legacy
// elevation: section (keeping existing maps byte-identical). A nil Solids is a
// heightfield by definition.
func (a *AreaDefinition) AllColumnsGapless() bool {
	if len(a.Solids) == 0 {
		return true
	}
	for z := 0; z < a.Height; z++ {
		for x := 0; x < a.Width; x++ {
			if !a.columnGapless(x, z) {
				return false
			}
		}
	}
	return true
}

// canonicalSolids returns the area's voxel stack normalized for comparison:
// the stored Solids when present, else the stack derived from Elevation, with
// trailing all-air planes trimmed and every row padded to Width. So a
// heightfield map (Solids nil) and the same map with its stack materialized
// compare equal rather than dirty — the absent==derived rule for AreaContentEqual.
func canonicalSolids(a AreaDefinition) [][]string {
	stack := a.Solids
	if len(stack) == 0 {
		stack = BuildSolidsFromElevation(&a)
	}
	// Trim trailing all-air planes.
	hi := len(stack)
	for hi > 0 && planeAllAir(stack[hi-1]) {
		hi--
	}
	stack = stack[:hi]
	out := make([][]string, len(stack))
	for L := range stack {
		out[L] = normalizeOptionalLayer(stack[L], a.Width, a.Height, SolidAir)
	}
	return out
}

func planeAllAir(rows []string) bool {
	for _, r := range rows {
		for i := 0; i < len(r); i++ {
			if r[i] != SolidAir {
				return false
			}
		}
	}
	return true
}

// solidsEqual compares two areas' voxel stacks with absent==derived semantics.
func solidsEqual(a, b AreaDefinition) bool {
	// Fast path: two pure heightfields are fully described by their Elevation
	// layer, which AreaContentEqual already compares in the gridLayers() walk —
	// so there's nothing extra to check and no need to materialize either stack.
	if len(a.Solids) == 0 && len(b.Solids) == 0 {
		return true
	}
	ca, cb := canonicalSolids(a), canonicalSolids(b)
	if len(ca) != len(cb) {
		return false
	}
	for L := range ca {
		if !slices.Equal(ca[L], cb[L]) {
			return false
		}
	}
	return true
}

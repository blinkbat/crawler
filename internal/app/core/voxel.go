package core

import (
	"crawler/internal/app/core/mapfile"
)

// Voxel occupancy — cube model superseding the single-height Elevation
// heightfield. A column (x,z) is a stack of cells, each SOLID (cube, char =
// material/skin) or AIR. Expresses floating decks/bridges/overhangs (solid@0,
// air@1, solid@2 = walk UNDER at 0, OVER at 2).
//
// Migration discipline: Solids stays NIL for a pure heightfield, and every
// accessor DERIVES occupancy from the Elevation layer (solid from 0 up to the
// stored top) so legacy maps answer unchanged and the editor's elevation brush
// stays authoritative. Solids is non-nil only once a map has a gap.

// SolidAir is the "no cube" (air) char. Coincides with ElevationGround ('0') and
// is disjoint from the face-skin alphabet (FaceSkins) marking a cube's material.
const SolidAir = ElevationGround

// IsVoxel reports whether the area has a materialized voxel stack (non-nil
// Solids) vs. a pure heightfield. Gates ResolveStep / PackIndexAtTileLevel.
func (a *AreaDefinition) IsVoxel() bool { return len(a.Solids) > 0 }

// SolidAt reports whether a cube occupies (x, level, z) and its skin char. THE
// single read path into the stack: indexes Solids when set, else derives from
// Elevation (solid 0..ElevationLevelAt, skinned by FaceSkinAt) for identical
// answers. OOB, negative level, or above the tallest stored plane read as air.
func (a *AreaDefinition) SolidAt(x, level, z int) (skin byte, solid bool) {
	if level < 0 || !a.InBounds(x, z) {
		return 0, false
	}
	if len(a.Solids) > 0 {
		if level >= len(a.Solids) {
			return 0, false // above tallest plane: implicit air
		}
		c, ok := a.layerByteAt(a.Solids[level], x, z)
		if !ok || c == SolidAir {
			return 0, false
		}
		return c, true
	}
	// Heightfield fallback: solid from 0 up to the stored top. Ramp tiles store
	// their LOW level, so the solid block ends there (wedge drawn separately).
	return a.heightfieldCellSkin(x, level, z)
}

// heightfieldCellSkin is the heightfield occupancy rule for one cell: solid
// (skinned by FaceSkinAt) from level 0 up to the column's stored top, else air.
// Single home for the rule SolidAt's fallback and BuildSolidsFromElevation share.
func (a *AreaDefinition) heightfieldCellSkin(x, level, z int) (skin byte, solid bool) {
	if level <= a.ElevationLevelAt(x, z) {
		return a.FaceSkinAt(x, z), true
	}
	return 0, false
}

// TopSolidLevel returns the highest solid level in column (x,z), or -1 for a
// wholly empty column (explicit-Solids only). Heightfield mode = ElevationLevelAt.
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

// maxElevationTopCache memoizes the tallest-column scan (a full W×H ElevationLevelAt
// pass) per area so repeated SolidStackHeight / LowestStandableLevel calls on a
// heightfield don't re-scan. Keyed by name + dims + an allocation-free fold of the
// Elevation layer (the scan's only input on the heightfield path) so a struct copy
// or an edited elevation can't serve a stale top. Package-level (not a struct field)
// to stay copy-safe, mirroring the render-layer per-area caches.
//
// INVARIANT: single-writer. The read-then-write in maxElevationTop is unsynchronized
// and assumes the single-threaded game loop. If geometry queries ever run off the main
// goroutine (e.g. an async autosave), guard this with a mutex or give it a per-caller cache.
var maxElevationTopCache struct {
	name          string
	width, height int
	hash          uint64
	top           int
	primed        bool
}

// elevationLayerHash folds the Elevation rows into an FNV-1a digest, allocation-free,
// with row separators so ragged splits can't collide. FNV primitives + FoldLayerRow
// live in hash.go (shared by render's layersHash and shop's inventory fingerprint).
func (a *AreaDefinition) elevationLayerHash() uint64 {
	h := FNVOffset64
	for _, row := range a.Elevation {
		h = FoldLayerRow(h, row)
	}
	return h
}

// forEachCell invokes fn for every (x,z) column in row-major order. Shared by the
// full-grid walks that fit a plain visit (no early exit / per-row assembly).
func (a *AreaDefinition) forEachCell(fn func(x, z int)) {
	for z := 0; z < a.Height; z++ {
		for x := 0; x < a.Width; x++ {
			fn(x, z)
		}
	}
}

// maxElevationTop returns the tallest ElevationLevelAt across all columns (0 if flat).
// Only reached on the heightfield (nil-Solids) path, so the Elevation-layer hash fully
// fingerprints its inputs.
func (a *AreaDefinition) maxElevationTop() int {
	hash := a.elevationLayerHash()
	c := &maxElevationTopCache
	if c.primed && c.name == a.Name && c.width == a.Width && c.height == a.Height && c.hash == hash {
		return c.top
	}
	top := 0
	a.forEachCell(func(x, z int) {
		if t := a.ElevationLevelAt(x, z); t > top {
			top = t
		}
	})
	c.name, c.width, c.height, c.hash, c.top, c.primed = a.Name, a.Width, a.Height, hash, top, true
	return top
}

// SolidStackHeight is the level count to iterate for column walks: stored plane
// count when Solids is set, else one past the tallest column. Always >= 1.
func (a *AreaDefinition) SolidStackHeight() int {
	if len(a.Solids) > 0 {
		return len(a.Solids)
	}
	return a.maxElevationTop() + 1
}

// Standable reports whether a unit can stand atop level L in column (x,z): L
// solid AND L+1 air (body headroom).
func (a *AreaDefinition) Standable(x, L, z int) bool {
	if _, solid := a.SolidAt(x, L, z); !solid {
		return false
	}
	_, above := a.SolidAt(x, L+1, z)
	return !above
}

// LowestStandableLevel returns the lowest standable level in column (x,z), or -1.
// Seeds a unit's level when only (x,z) is known (legacy save, authored start).
func (a *AreaDefinition) LowestStandableLevel(x, z int) int {
	h := a.SolidStackHeight()
	for L := 0; L < h; L++ {
		if a.Standable(x, L, z) {
			return L
		}
	}
	return -1
}

// MapSurfaceKind classifies what an observer at observerLevel sees at a column on
// the top-down map. Shared rule behind the in-game map's level slice (minimap + Map tab).
type MapSurfaceKind int

const (
	MapSurfaceVoid  MapSurfaceKind = iota // no solid at or below the observer
	MapSurfaceFloor                       // walkable floor at the observer's level
	MapSurfaceWall                        // solid rising into the observer's eyeline (cliff/wall)
	MapSurfaceBelow                       // floor below the observer (Depth >= 1 levels down)
	MapSurfaceRamp                        // ramp connecting to/from the observer's level
)

// MapSurface is the classified column; Depth (>=1) is levels below the observer
// for MapSurfaceBelow, 0 otherwise.
type MapSurface struct {
	Kind  MapSurfaceKind
	Depth int
}

// MapSurfaceAt classifies column (x,z) from observer level L. Voxel-aware,
// reducing to heightfield "raised = wall, lower = faded" on a nil-Solids map:
// ramp low edge at L or L-1 → Ramp; cube at L+1 (eyeline) → Wall; cube at L with
// air above → Floor; else highest solid below L → Below{Depth}; else Void.
// A deck above the observer is never reported (the floor beneath it is).
func (a *AreaDefinition) MapSurfaceAt(x, z, L int) MapSurface {
	if !a.InBounds(x, z) {
		return MapSurface{Kind: MapSurfaceVoid}
	}
	if _, ok := a.RampAt(x, z); ok {
		low := a.ElevationLevelAt(x, z) // ramps store their LOW level
		if low == L || low == L-1 {
			return MapSurface{Kind: MapSurfaceRamp}
		}
	}
	// Heightfield fast path: the column is solid 0..top, so classify by comparing L
	// against the top decoded ONCE — equivalent to the SolidAt loop below, which would
	// re-decode ElevationLevelAt for the same column on each of its 2..L+1 probes. L>=0
	// in every real call (observer = player level); a negative L falls through to the
	// generic loop which already handles sub-zero levels as air.
	if len(a.Solids) == 0 && L >= 0 {
		top := a.ElevationLevelAt(x, z)
		switch {
		case L+1 <= top:
			return MapSurface{Kind: MapSurfaceWall}
		case L <= top:
			return MapSurface{Kind: MapSurfaceFloor}
		default:
			return MapSurface{Kind: MapSurfaceBelow, Depth: L - top}
		}
	}
	if _, solid := a.SolidAt(x, L+1, z); solid {
		return MapSurface{Kind: MapSurfaceWall}
	}
	if _, solid := a.SolidAt(x, L, z); solid {
		return MapSurface{Kind: MapSurfaceFloor}
	}
	for d := 1; L-d >= 0; d++ {
		if _, solid := a.SolidAt(x, L-d, z); solid {
			return MapSurface{Kind: MapSurfaceBelow, Depth: d}
		}
	}
	return MapSurface{Kind: MapSurfaceVoid}
}

// BuildSolidsFromElevation materializes the voxel stack from a heightfield:
// each column solid 0..ElevationLevelAt, skinned by FaceSkinAt. Inverse of
// ElevationRowsFromSolids. Returns planes 0..maxTop, no trailing all-air planes.
func BuildSolidsFromElevation(a *AreaDefinition) [][]string {
	maxTop := a.maxElevationTop()
	out := make([][]string, maxTop+1)
	for L := 0; L <= maxTop; L++ {
		rows := make([]string, a.Height)
		for z := 0; z < a.Height; z++ {
			buf := make([]byte, a.Width)
			for x := 0; x < a.Width; x++ {
				if skin, solid := a.heightfieldCellSkin(x, L, z); solid {
					buf[x] = skin
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

// ElevationRowsFromSolids projects a voxel stack down to the legacy elevation
// layer (top solid level per column, base-36 char) as a graceful downgrade for
// readers that ignore solids:. Empty column writes ground.
func ElevationRowsFromSolids(a *AreaDefinition) []string {
	rows := make([]string, a.Height)
	for z := 0; z < a.Height; z++ {
		buf := make([]byte, a.Width)
		for x := 0; x < a.Width; x++ {
			top := MaxZero(a.TopSolidLevel(x, z))
			buf[x] = ElevationChar(top)
		}
		rows[z] = string(buf)
	}
	return rows
}

// surfaceEdgeLevel returns the level a unit atop L in column (x,z) presents at
// its `dir` edge; ok=false on a ramp's sheer perpendicular side. Flat presents L
// everywhere; a ramp uses EdgeLevelOf (L = its stored low level). ResolveStep
// uses this on both leave and enter sides.
func (a *AreaDefinition) surfaceEdgeLevel(x, L, z, dir int) (int, bool) {
	if f, ok := a.RampAt(x, z); ok {
		return EdgeLevelOf(L, f, dir)
	}
	return L, true
}

// ResolveStep resolves a step from level `fromL` in (fromX,fromZ) heading `dir`,
// returning the destination standing level, or ok=false if blocked (cliff, cube
// side, no surface, sheer ramp side). Generalizes StepElevationOK to multi-surface
// columns: you land on the neighbour surface whose entry edge equals your exit
// edge. Surfaces are >=2 levels apart so at most one matches (ground-under-bridge
// vs deck is unambiguous). Identical to StepElevationOK on a heightfield column.
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
	bestL, bestOK := 0, false
	for L := 0; L < h; L++ {
		if !a.Standable(nx, L, nz) {
			continue
		}
		// On a valid map (surfaces >=2 apart) at most one entry edge matches; if a
		// map ever violates that, prefer the match nearest fromL over the lowest.
		if entry, eok := a.surfaceEdgeLevel(nx, L, nz, fromDir); eok && entry == exitEdge {
			if !bestOK || AbsInt(L-fromL) < AbsInt(bestL-fromL) {
				bestL, bestOK = L, true
			}
		}
	}
	if bestOK {
		return bestL, true
	}
	return 0, false
}

// ResolveStepLanding resolves a one-tile cardinal step from (fromX,fromL,fromZ) heading
// dir: the surface level the party lands on and whether the step is reachable (not a
// cliff/blocked). Unifies the voxel (ResolveStep) and heightfield (StepElevationOK +
// ElevationLevelAt) paths so movement callers don't branch on IsVoxel themselves. On a
// heightfield the returned level is the destination column's elevation.
func (a *AreaDefinition) ResolveStepLanding(fromX, fromL, fromZ, dir int) (landLevel int, reachable bool) {
	if a.IsVoxel() {
		return a.ResolveStep(fromX, fromL, fromZ, dir)
	}
	dx, dz := FacingVector(dir)
	return a.ElevationLevelAt(fromX+dx, fromZ+dz), a.StepElevationOK(fromX, fromZ, dir)
}

// resolveEntityLevel returns the standing floor a placed entity should occupy at
// (x,z): its authored `stored` level when that surface is standable, else the
// column's ground spawn level. This covers legacy entities (stored 0, before the
// per-floor @N sections) on a raised-baseline map — a non-standable stored level
// falls back to the actual ground, so old maps render unchanged.
func (a *AreaDefinition) resolveEntityLevel(x, z, stored int) int {
	if a.Standable(x, stored, z) {
		return stored
	}
	return a.GroundSpawnLevel(x, z)
}

// GroundSpawnLevel is the standing level a unit placed at (x,z) should occupy
// (lowest standable surface, else column top). For out-of-package placement
// (door transitions) building a player without a saved level.
func (a *AreaDefinition) GroundSpawnLevel(x, z int) int {
	return spawnLevel(a, x, z)
}

// --- Editor authoring primitives -------------------------------------------

// EnsureSolids materializes the voxel stack from Elevation if not already
// explicit, giving an editor cube edit a stack to write into. No-op when Solids
// is present. After this, edits must route through SetColumnTop / SetCube /
// ClearCube, not the Elevation layer (which ElevationLevelAt no longer reads).
func EnsureSolids(a *AreaDefinition) {
	if len(a.Solids) == 0 {
		a.Solids = BuildSolidsFromElevation(a)
	}
}

func blankSolidPlane(w, h int) []string {
	return mapfile.BlankLayer(w, h, SolidAir)
}

// growSolidsTo pads the stack to at least n planes with all-air planes.
func (a *AreaDefinition) growSolidsTo(n int) {
	for len(a.Solids) < n {
		a.Solids = append(a.Solids, blankSolidPlane(a.Width, a.Height))
	}
}

// trimTopAir drops trailing all-air planes so height tracks the tallest cube.
// Never trims below one plane.
func (a *AreaDefinition) trimTopAir() {
	a.Solids = trimScatterTop(a.Solids, SolidAir)
}

// setSolidCell writes c at (x,level,z), materializing + growing the stack and
// padding the affected row to Width first.
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

// SetCube places a cube at (x,level,z) skinned by `skin` (air coerced to rock so
// a placed cube is always solid). The editor's floating-cube primitive (bridge/overhang).
func (a *AreaDefinition) SetCube(x, level, z int, skin byte) {
	if skin == SolidAir {
		skin = TileRock
	}
	a.setSolidCell(x, level, z, skin)
}

// ClearCube sets (x,level,z) to air, trimming any trailing all-air planes exposed.
func (a *AreaDefinition) ClearCube(x, level, z int) {
	a.setSolidCell(x, level, z, SolidAir)
	a.trimTopAir()
}

// SetColumnTop sets column (x,z) solid 0..top, air above — the voxel-aware "Set
// Height". Clears any floating cube above `top`. Cube skin is FaceSkinAt.
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

// cloneRows deep-copies a grid layer's rows (nil-safe; string rows immutable).
// Semantics of append([]string(nil), rows...): nil→nil, empty→empty.
func cloneRows(rows []string) []string {
	return append([]string(nil), rows...)
}

// CloneSolids deep-copies a voxel stack (nil-safe; string rows immutable).
// Returns nil for an empty stack so a heightfield area keeps Solids==nil.
func CloneSolids(in [][]string) [][]string {
	if len(in) == 0 {
		return nil
	}
	out := make([][]string, len(in))
	for L := range in {
		out[L] = cloneRows(in[L])
	}
	return out
}

// columnGapless reports whether column (x,z) is solid 0..top with no air gap
// (encodable as a single elevation char). Empty column is trivially gapless.
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

// AllColumnsGapless reports whether the area is expressible as a heightfield and
// can round-trip as the legacy elevation: section. Nil Solids is gapless by definition.
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

// canonicalSolids normalizes the voxel stack for comparison: stored Solids, else
// derived from Elevation, with trailing all-air planes trimmed and rows padded to
// Width — the absent==derived rule so a heightfield and its materialized form compare equal.
func canonicalSolids(a AreaDefinition) [][]string {
	stack := a.Solids
	if len(stack) == 0 {
		stack = BuildSolidsFromElevation(&a)
	}
	return canonicalizeStack(stack, a.Width, a.Height, SolidAir)
}

func planeAllAir(rows []string) bool { return planeAllChar(rows, SolidAir) }

// solidsEqual compares two areas' voxel stacks with absent==derived semantics.
func solidsEqual(a, b AreaDefinition) bool {
	// Fast path: two heightfields are fully described by Elevation, already
	// compared in AreaContentEqual's gridLayers() walk.
	if len(a.Solids) == 0 && len(b.Solids) == 0 {
		return true
	}
	return canonicalStacksEqual(canonicalSolids(a), canonicalSolids(b))
}

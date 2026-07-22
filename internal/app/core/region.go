package core

import (
	"fmt"
	"reflect"
)

// region.go: editor region copy/paste — a rectangle of the grid layers
// snapshotted and stamped elsewhere. Grid layers only (gridLayers()); entities
// (packs/chests/doors/crystals) live in spawn lists and are NOT copied.

// TileRegion is a copied rectangle of the grid layers, indexed in gridLayers()
// order; rows may be shorter than W if the source was ragged (paste tolerates it).
// On a voxel source (Solids != nil) the materialized cube stack is captured in
// Solids too, since ElevationLevelAt ignores the Elevation layer there — copying
// the grid layers alone would silently drop all 3D geometry.
type TileRegion struct {
	W, H   int
	Layers [][]string
	Solids [][]string // per-level rectangle rows; nil for a heightfield source
	// PropStack/DecorStack: per-floor scatter planes; nil for a single-grid source
	// (the Props/Decor grid in Layers carries it). PropLevels/DecorLevels: the legacy
	// per-tile level tags — NOT in gridLayers(), so captured here explicitly or a
	// legacy source's props/decor paste at floor auto. Faces: cliff-face overrides in
	// the rectangle, coords made region-local (remapped back on paste).
	PropStack   [][]string
	DecorStack  [][]string
	PropLevels  []string
	DecorLevels []string
	// PropYaw: per-tile prop-facing overrides in the rectangle ('.' = auto), captured
	// like the level grids so a copied prop keeps its authored orientation.
	PropYaw []string
	Faces   []FaceOverride
}

// Empty reports whether the region has nothing to paste.
func (r TileRegion) Empty() bool { return r.W <= 0 || r.H <= 0 || len(r.Layers) == 0 }

// regionPlaneFieldCount pins the number of per-tile plane fields on TileRegion (the
// slice members). CopyRegion captures them and FlipHorizontal/FlipVertical/Rotate90CW
// each transform them by HAND — nothing else keeps them in lockstep, so a new plane a
// transform forgets silently corrupts flip/rotate geometry (it keeps its original
// orientation). The init tripwire fires when the field set changes: update CopyRegion
// + all three transforms + the paste path, then bump this count.
const regionPlaneFieldCount = 8

func init() {
	n := 0
	t := reflect.TypeOf(TileRegion{})
	for i := 0; i < t.NumField(); i++ {
		if t.Field(i).Type.Kind() == reflect.Slice {
			n++
		}
	}
	if n != regionPlaneFieldCount {
		panic(fmt.Sprintf("core: TileRegion has %d plane fields, expected %d — a plane was added/removed; update CopyRegion + FlipHorizontal/FlipVertical/Rotate90CW + the paste path, then bump regionPlaneFieldCount", n, regionPlaneFieldCount))
	}
}

// clampSubstr returns s[lo:hi] with both bounds clamped to len(s) (lo<=hi
// assumed), tolerating ragged source rows shorter than the copied rectangle.
func clampSubstr(s string, lo, hi int) string {
	if lo > len(s) {
		lo = len(s)
	}
	if hi > len(s) {
		hi = len(s)
	}
	return s[lo:hi]
}

// normalizeRect corner-swaps (x0,z0)-(x1,z1) into min/max order and clamps it to the
// area bounds, reporting ok=false when the area is empty or the clamped rect is
// degenerate. The shared preamble behind CopyRegion / ClearRegion (both take any
// corner order); extracted so the swap/clamp/bail can't drift between the two.
func (a *AreaDefinition) normalizeRect(x0, z0, x1, z1 int) (nx0, nz0, nx1, nz1 int, ok bool) {
	if a == nil || a.Width <= 0 || a.Height <= 0 {
		return 0, 0, 0, 0, false
	}
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	if z0 > z1 {
		z0, z1 = z1, z0
	}
	x0, x1 = Clamp(x0, 0, a.Width-1), Clamp(x1, 0, a.Width-1)
	z0, z1 = Clamp(z0, 0, a.Height-1), Clamp(z1, 0, a.Height-1)
	if x1 < x0 || z1 < z0 {
		return 0, 0, 0, 0, false
	}
	return x0, z0, x1, z1, true
}

// CopyRegion snapshots the inclusive rectangle (x0,z0)-(x1,z1) (any corner
// order) across all grid layers; coords clamped to the area, degenerate yields
// empty. Returned strings are immutable, so the snapshot survives later source edits.
func CopyRegion(a *AreaDefinition, x0, z0, x1, z1 int) TileRegion {
	x0, z0, x1, z1, ok := a.normalizeRect(x0, z0, x1, z1)
	if !ok {
		return TileRegion{}
	}
	w, h := x1-x0+1, z1-z0+1
	layers := a.gridLayers()
	out := TileRegion{W: w, H: h, Layers: make([][]string, len(layers))}
	for li, lp := range layers {
		rows := make([]string, h)
		for i := 0; i < h; i++ {
			z := z0 + i
			if z >= len(*lp) {
				continue // ragged: leave row ""
			}
			rows[i] = clampSubstr((*lp)[z], x0, x1+1)
		}
		out.Layers[li] = rows
	}
	// Voxel source: also snapshot the cube stack (the Elevation layer is dead there).
	// The per-floor prop/decor stacks, legacy level tags, and face overrides all live
	// outside gridLayers(), so capture them explicitly or paste silently drops them.
	out.Solids = copyStackRect(a.Solids, x0, z0, x1, z1)
	out.PropStack = copyStackRect(a.PropStack, x0, z0, x1, z1)
	out.DecorStack = copyStackRect(a.DecorStack, x0, z0, x1, z1)
	out.PropLevels = copyLevelRect(a.PropLevels, x0, z0, x1, z1)
	out.DecorLevels = copyLevelRect(a.DecorLevels, x0, z0, x1, z1)
	out.PropYaw = copyLevelRect(a.PropYaw, x0, z0, x1, z1)
	out.Faces = copyFaceRect(a.FaceOverrides, x0, z0, x1, z1)
	return out
}

// copyStackRect snapshots a per-level scatter/solid stack's rectangle, one row per
// (level, z), tolerating ragged planes. Returns nil for an absent stack (a
// single-grid/heightfield source — the grid layer carries it).
func copyStackRect(stack [][]string, x0, z0, x1, z1 int) [][]string {
	if len(stack) == 0 {
		return nil
	}
	h := z1 - z0 + 1
	planes := make([][]string, len(stack))
	for L := range stack {
		rows := make([]string, h)
		for i := 0; i < h; i++ {
			z := z0 + i
			if z >= len(stack[L]) {
				continue
			}
			rows[i] = clampSubstr(stack[L][z], x0, x1+1)
		}
		planes[L] = rows
	}
	return planes
}

// copyLevelRect snapshots a level-tag grid's rectangle as full-width rows (auto
// where the source is absent/ragged), so a paste overwrites the destination's tags
// exactly — including clearing stale non-auto tags a legacy source left at auto.
func copyLevelRect(grid []string, x0, z0, x1, z1 int) []string {
	w, h := x1-x0+1, z1-z0+1
	rows := make([]string, h)
	for i := 0; i < h; i++ {
		buf := make([]byte, w)
		var srcRow string
		if z := z0 + i; z < len(grid) {
			srcRow = grid[z]
		}
		for j := 0; j < w; j++ {
			if x := x0 + j; x < len(srcRow) {
				buf[j] = srcRow[x]
			} else {
				buf[j] = PropLevelAuto
			}
		}
		rows[i] = string(buf)
	}
	return rows
}

// copyFaceRect collects the face overrides inside the rectangle, coords made
// region-local (remapped back on paste). nil when none fall in the rectangle.
func copyFaceRect(faces []FaceOverride, x0, z0, x1, z1 int) []FaceOverride {
	var out []FaceOverride
	for _, o := range faces {
		if o.X < x0 || o.X > x1 || o.Z < z0 || o.Z > z1 {
			continue
		}
		lo := o
		lo.X, lo.Z = o.X-x0, o.Z-z0
		out = append(out, lo)
	}
	return out
}

// PasteRegion stamps r's top-left at (atX,atZ), overwriting in-bounds cells and
// clipping the rest. No-op on an empty region.
func (a *AreaDefinition) PasteRegion(r TileRegion, atX, atZ int) {
	if a == nil || r.Empty() {
		return
	}
	layers := a.gridLayers()
	for li := 0; li < len(layers) && li < len(r.Layers); li++ {
		lp := layers[li]
		// Per-layer open char for padding a ragged destination row to width.
		blank := a.layerBlank(lp)
		for i, row := range r.Layers[li] {
			z := atZ + i
			if z < 0 || z >= len(*lp) {
				continue
			}
			dest := []byte((*lp)[z])
			// Pad a short row to width first, else in-bounds paste cells past its
			// end are dropped (hand-built fixtures may be ragged).
			for len(dest) < a.Width {
				dest = append(dest, blank)
			}
			for j := 0; j < len(row); j++ {
				x := atX + j
				if x < 0 || x >= len(dest) {
					continue
				}
				dest[x] = row[j]
			}
			(*lp)[z] = string(dest)
		}
	}
	a.pasteRegionSolids(r, atX, atZ)
	a.pasteRegionScatter(r, atX, atZ)
	a.pasteRegionFaces(r, atX, atZ)
}

// pasteRegionSolids stamps a voxel region's cube stack at (atX,atZ). Materializes
// the destination so the paste overwrites the full column (cells above the copied
// stack within the rectangle are cleared to air), then trims trailing all-air planes.
func (a *AreaDefinition) pasteRegionSolids(r TileRegion, atX, atZ int) {
	if len(r.Solids) == 0 {
		// Heightfield source (no cube stack) into a MATERIALIZED destination: the copied
		// heights landed only in the dead Elevation layer (ElevationLevelAt ignores it
		// under a Solids stack, and the next save overwrites it via ElevationRowsFromSolids),
		// so no cubes would be stamped. Synthesize the columns from the region's copied
		// Elevation + Walls rows, mirroring BuildSolidsFromElevation's heightfield rule.
		if a.IsVoxel() {
			a.stampHeightfieldRegionCubes(r, atX, atZ)
		}
		return
	}
	// Same full-column stamp as the prop/decor stacks (cells above the copied planes
	// clear to air); setSolidCell grows the stack as needed, then trim trailing air.
	EnsureSolids(a)
	a.pasteScatterStack(r.Solids, len(a.Solids), SolidAir, r.W, r.H, atX, atZ, a.setSolidCell)
	a.trimTopAir()
}

// stampHeightfieldRegionCubes builds cubes for a heightfield region pasted into a
// materialized voxel destination: each column solid 0..decoded-elevation (skinned
// by the region's wall char, else rock), cells above cleared to air within the
// rectangle so a shorter copied column reveals the void beneath. Full-column stamp
// mirrors pasteScatterStack. Reads the region's own rows (the destination's dead
// Elevation layer may be absent, so it can't be relied on).
func (a *AreaDefinition) stampHeightfieldRegionCubes(r TileRegion, atX, atZ int) {
	elevRows := r.layerRows(a, &a.Elevation)
	if elevRows == nil {
		return
	}
	wallRows := r.layerRows(a, &a.Walls)
	depth := len(a.Solids)
	for i := 0; i < r.H; i++ {
		z := atZ + i
		var erow string
		if i < len(elevRows) {
			erow = elevRows[i]
		}
		for j := 0; j < r.W; j++ {
			x := atX + j
			if !a.InBounds(x, z) {
				continue
			}
			top := 0
			if j < len(erow) {
				top = ElevationLevelFromChar(erow[j])
			}
			skin := byte(TileRock)
			if wallRows != nil && i < len(wallRows) && j < len(wallRows[i]) {
				if c := wallRows[i][j]; c != TileOpen {
					skin = c
				}
			}
			hi := top
			if depth-1 > hi {
				hi = depth - 1
			}
			for L := 0; L <= hi; L++ {
				if L <= top {
					a.setSolidCell(x, L, z, skin)
				} else {
					a.setSolidCell(x, L, z, SolidAir)
				}
			}
		}
	}
	a.trimTopAir()
}

// gridLayerIndex returns the position of layer pointer lp within gridLayers(), or
// -1. Lets a copied region read its Walls/Elevation/Props/Decor rows by canonical
// role without hardcoding the enumeration order.
func (a *AreaDefinition) gridLayerIndex(lp *[]string) int {
	for i, p := range a.gridLayers() {
		if p == lp {
			return i
		}
	}
	return -1
}

// layerRows returns the region's captured rows for the grid layer at pointer lp
// (nil if the region didn't capture it), keyed by gridLayers() role.
func (r TileRegion) layerRows(a *AreaDefinition, lp *[]string) []string {
	i := a.gridLayerIndex(lp)
	if i < 0 || i >= len(r.Layers) {
		return nil
	}
	return r.Layers[i]
}

// pasteRegionScatter stamps a region's per-floor prop/decor stacks and legacy level
// tags at (atX,atZ). Stacks overwrite the full column in the rectangle (dest cubes
// above the copied planes clear to blank), mirroring pasteRegionSolids; the level
// grids are stamped as an overlay so a legacy source's props land on the right floor.
func (a *AreaDefinition) pasteRegionScatter(r TileRegion, atX, atZ int) {
	// Level tags first: stampLegacyScatterRegion resolves a legacy region's props onto
	// the destination stack using them, and an auto tag must resolve against the pasted
	// grid (order among the three level grids is irrelevant).
	a.pasteLevelGrid(&a.PropLevels, r.PropLevels, atX, atZ)
	a.pasteLevelGrid(&a.DecorLevels, r.DecorLevels, atX, atZ)
	a.pasteLevelGrid(&a.PropYaw, r.PropYaw, atX, atZ)

	if len(r.PropStack) > 0 {
		a.EnsurePropStack()
		a.pasteScatterStack(r.PropStack, len(a.PropStack), TilePropEmpty, r.W, r.H, atX, atZ, a.SetProp)
	} else if len(a.PropStack) > 0 {
		// Legacy scatter source into a MATERIALIZED destination: the region's prop grid
		// landed in the frozen a.Props (unread once a stack exists), so it would never
		// render. Re-stamp it onto the stack at each column's resolved floor.
		a.stampLegacyScatterRegion(r, atX, atZ, len(a.PropStack), &a.Props, a.PropLevels, TilePropEmpty, a.SetProp)
	}
	if len(r.DecorStack) > 0 {
		a.EnsureDecorStack()
		a.pasteScatterStack(r.DecorStack, len(a.DecorStack), DecorEmpty, r.W, r.H, atX, atZ, a.SetDecor)
	} else if len(a.DecorStack) > 0 {
		a.stampLegacyScatterRegion(r, atX, atZ, len(a.DecorStack), &a.Decor, a.DecorLevels, DecorEmpty, a.SetDecor)
	}
}

// stampLegacyScatterRegion re-stamps a legacy (single-grid) scatter region onto a
// MATERIALIZED destination's per-floor stack. Full-column overwrite mirrors the
// materialized-region paste: each rectangle column is cleared across the stack's
// existing floors, then the region's single content char (read from its captured
// rows) is placed on its resolved floor (explicit level tag, else the destination
// column's ground). Without this the region's props vanish into the frozen grid.
func (a *AreaDefinition) stampLegacyScatterRegion(r TileRegion, atX, atZ, depth int, gridPtr *[]string, levelGrid []string, blank byte, set func(x, level, z int, c byte)) {
	rows := r.layerRows(a, gridPtr)
	for i := 0; i < r.H; i++ {
		z := atZ + i
		var row string
		if rows != nil && i < len(rows) {
			row = rows[i]
		}
		for j := 0; j < r.W; j++ {
			x := atX + j
			if !a.InBounds(x, z) {
				continue
			}
			for L := 0; L < depth; L++ {
				set(x, L, z, blank)
			}
			if j < len(row) {
				if c := row[j]; c != blank {
					set(x, MaxZero(a.levelGridAt(levelGrid, x, z)), z, c)
				}
			}
		}
	}
}

// pasteScatterStack writes a scatter region across levels 0..max(dest,region)-1 so
// dest content above the pasted stack clears (blank cells), while `set` (SetProp/
// SetDecor) materializes + trims. Off-map cells no-op via set's own guard.
func (a *AreaDefinition) pasteScatterStack(planes [][]string, destLevels int, blank byte, w, h, atX, atZ int, set func(x, level, z int, c byte)) {
	levels := destLevels
	if len(planes) > levels {
		levels = len(planes)
	}
	for L := 0; L < levels; L++ {
		for i := 0; i < h; i++ {
			z := atZ + i
			var row string
			if L < len(planes) && i < len(planes[L]) {
				row = planes[L][i]
			}
			for j := 0; j < w; j++ {
				c := blank
				if j < len(row) {
					c = row[j]
				}
				set(atX+j, L, z, c)
			}
		}
	}
}

// pasteLevelGrid stamps a captured level-tag rectangle into dest at (atX,atZ). Skips
// entirely when both source and dest are trivial (all-auto/absent), preserving the
// nil round-trip; otherwise materializes dest so the overwrite (auto included) lands.
func (a *AreaDefinition) pasteLevelGrid(dest *[]string, src []string, atX, atZ int) {
	if len(src) == 0 {
		return
	}
	if len(*dest) == 0 && planeAllChar(src, PropLevelAuto) {
		return
	}
	*dest = normalizeOptionalLayer(*dest, a.Width, a.Height, PropLevelAuto)
	for i, row := range src {
		z := atZ + i
		if z < 0 || z >= len(*dest) {
			continue
		}
		buf := []byte((*dest)[z])
		for j := 0; j < len(row); j++ {
			x := atX + j
			if x < 0 || x >= len(buf) {
				continue
			}
			buf[x] = row[j]
		}
		(*dest)[z] = string(buf)
	}
}

// ClearRegion resets the inclusive rectangle (any corner order) to blank across
// every grid layer, clears voxel cubes / per-floor scatter / legacy level tags /
// face overrides inside it, and leaves spawns to the caller. Inverse of PasteRegion;
// the editor's Cut / region-move snapshot first, then call this on the old bounds.
func (a *AreaDefinition) ClearRegion(x0, z0, x1, z1 int) {
	x0, z0, x1, z1, ok := a.normalizeRect(x0, z0, x1, z1)
	if !ok {
		return
	}
	for _, lp := range a.gridLayers() {
		blank := a.layerBlank(lp)
		if lp == &a.Elevation {
			// Clear to the editor's default ground (baseline), NOT level 0 — else a Cut
			// on a baseline heightfield map leaves a MaxElevationLevel-deep sheer pit at
			// the vacated rectangle. layerBlank stays ElevationGround for the loader's
			// absent-layer convention; only the cleared cells use baseline. On a
			// materialized voxel map the Elevation layer is dead (cubes cleared to void
			// below, then re-derived on save), so this only affects heightfield maps.
			blank = ElevationChar(ElevationBaseline)
		}
		for z := z0; z <= z1; z++ {
			if z >= len(*lp) {
				continue
			}
			dest := []byte((*lp)[z])
			for len(dest) < a.Width {
				dest = append(dest, a.layerBlank(lp))
			}
			for x := x0; x <= x1 && x < len(dest); x++ {
				dest[x] = blank
			}
			(*lp)[z] = string(dest)
		}
	}
	if len(a.Solids) > 0 {
		for L := range a.Solids {
			for z := z0; z <= z1; z++ {
				for x := x0; x <= x1; x++ {
					a.setSolidCell(x, L, z, SolidAir)
				}
			}
		}
		a.trimTopAir()
	}
	a.clearScatterStackRect(a.PropStack, TilePropEmpty, x0, z0, x1, z1, a.SetProp)
	a.clearScatterStackRect(a.DecorStack, DecorEmpty, x0, z0, x1, z1, a.SetDecor)
	a.clearLevelGridRect(a.PropLevels, x0, z0, x1, z1)
	a.clearLevelGridRect(a.DecorLevels, x0, z0, x1, z1)
	a.clearLevelGridRect(a.PropYaw, x0, z0, x1, z1)
	if len(a.FaceOverrides) > 0 {
		kept := a.FaceOverrides[:0:0]
		for _, o := range a.FaceOverrides {
			if o.X >= x0 && o.X <= x1 && o.Z >= z0 && o.Z <= z1 {
				continue
			}
			kept = append(kept, o)
		}
		a.FaceOverrides = kept
		a.faceOverrideIdx = nil
	}
}

// clearScatterStackRect blanks every level of a per-floor scatter stack inside the
// rectangle (no-op for an absent stack); set (SetProp/SetDecor) materializes + trims.
func (a *AreaDefinition) clearScatterStackRect(stack [][]string, blank byte, x0, z0, x1, z1 int, set func(x, level, z int, c byte)) {
	if len(stack) == 0 {
		return
	}
	for L := range stack {
		for z := z0; z <= z1; z++ {
			for x := x0; x <= x1; x++ {
				set(x, L, z, blank)
			}
		}
	}
}

// clearLevelGridRect resets a legacy per-tile level-tag grid's rectangle to auto
// (no-op for an absent grid — an all-auto grid stays nil).
func (a *AreaDefinition) clearLevelGridRect(grid []string, x0, z0, x1, z1 int) {
	for z := z0; z <= z1 && z < len(grid); z++ {
		buf := []byte(grid[z])
		for x := x0; x <= x1 && x < len(buf); x++ {
			buf[x] = PropLevelAuto
		}
		grid[z] = string(buf)
	}
}

// pasteRegionFaces stamps the region's face overrides at (atX,atZ) with overwrite
// semantics: any dest override inside the paste rectangle is dropped first (a source
// tile with no override means base skin), then the source's are remapped in-bounds.
func (a *AreaDefinition) pasteRegionFaces(r TileRegion, atX, atZ int) {
	if len(r.Faces) == 0 && len(a.FaceOverrides) == 0 {
		return
	}
	x1, z1 := atX+r.W-1, atZ+r.H-1
	kept := make([]FaceOverride, 0, len(a.FaceOverrides)+len(r.Faces))
	for _, o := range a.FaceOverrides {
		if o.X >= atX && o.X <= x1 && o.Z >= atZ && o.Z <= z1 {
			continue // inside the paste rectangle — replaced below
		}
		kept = append(kept, o)
	}
	for _, o := range r.Faces {
		dx, dz := o.X+atX, o.Z+atZ
		if !a.InBounds(dx, dz) {
			continue
		}
		o.X, o.Z = dx, dz
		kept = append(kept, o)
	}
	// Keep the (Z,X) ordering FaceOverrides relies on for deterministic encoding.
	sortByZX(kept, func(o FaceOverride) (int, int) { return o.X, o.Z })
	a.FaceOverrides = kept
	a.faceOverrideIdx = nil // invalidate the lazy (x,z)->index lookup
}

// --- Region transforms (editor flip / rotate) -----------------------------------
//
// FlipHorizontal / FlipVertical / Rotate90CW return a transformed copy of the region
// for the editor's clipboard flip/rotate. Every parallel plane/grid moves in lockstep
// and the direction-carrying data (Floor ramp arrows, prop-yaw steps, cliff-face
// overrides) is remapped to the new orientation. Regions produced by CopyRegion are
// rectangular (full-width rows), which these assume; ragged fixtures degrade gracefully.

// regionFloorLayer is Floor's index in gridLayers() order (Walls, Floor, Decor,
// Props, Ceiling, Elevation) — the only layer whose chars encode a direction (ramp
// arrows). Init-asserted against gridLayers() so a reorder can't silently misroute
// the ramp remap onto the wrong layer.
const regionFloorLayer = 1

func init() {
	var a AreaDefinition
	if a.gridLayerIndex(&a.Floor) != regionFloorLayer {
		panic("core: regionFloorLayer must equal Floor's index in gridLayers() — the region ramp remap would target the wrong layer")
	}
}

// FlipFacingH / FlipFacingV / RotateFacingCW map a cardinal facing under a left-right
// mirror (E↔W), a top-bottom mirror (N↔S), and a 90° clockwise turn (N→E→S→W). Shared
// by the region transforms and the editor's entity/door remap.
func FlipFacingH(f int) int {
	switch NormalizeFacing(f) {
	case East:
		return West
	case West:
		return East
	}
	return NormalizeFacing(f)
}

func FlipFacingV(f int) int {
	switch NormalizeFacing(f) {
	case North:
		return South
	case South:
		return North
	}
	return NormalizeFacing(f)
}

func RotateFacingCW(f int) int { return (NormalizeFacing(f) + 1) % FacingCount }

func reverseRow(s string) string {
	b := []byte(s)
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return string(b)
}

// reverseEachRow reverses every row's bytes (a horizontal mirror of a grid).
func reverseEachRow(rows []string) []string {
	if rows == nil {
		return nil
	}
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = reverseRow(r)
	}
	return out
}

// reverseRowOrder reverses the order of rows (a vertical mirror of a grid).
func reverseRowOrder(rows []string) []string {
	if rows == nil {
		return nil
	}
	out := make([]string, len(rows))
	for i := range rows {
		out[len(rows)-1-i] = rows[i]
	}
	return out
}

func mapPlanes(planes [][]string, fn func([]string) []string) [][]string {
	if planes == nil {
		return nil
	}
	out := make([][]string, len(planes))
	for L := range planes {
		out[L] = fn(planes[L])
	}
	return out
}

// remapRowChars rewrites every byte of every row through fn (in place on the slice).
func remapRowChars(rows []string, fn func(byte) byte) {
	for i, r := range rows {
		b := []byte(r)
		for j := range b {
			b[j] = fn(b[j])
		}
		rows[i] = string(b)
	}
}

// remapRampChar re-points a ramp arrow through facingMap; non-ramp chars pass through.
func remapRampChar(c byte, facingMap func(int) int) byte {
	if f, ok := RampAscentFacing(c); ok {
		return RampCharForFacing(facingMap(f))
	}
	return c
}

// remapYawChar re-steps an authored prop-yaw char through stepMap (mod PropYawSteps);
// auto/invalid chars pass through.
func remapYawChar(c byte, stepMap func(int) int) byte {
	if step, ok := propYawStepFromChar(c); ok {
		s := ((stepMap(step) % PropYawSteps) + PropYawSteps) % PropYawSteps
		return PropYawStepChar(s)
	}
	return c
}

// Yaw-step transforms for the region flip/rotate — expressed in PropYawSteps so
// they track the authored-orientation resolution instead of hardcoding 30° math.
// Horizontal mirror sends θ→180°-θ, vertical mirror θ→-θ, a 90° CW turn θ→θ+90°.
func flipYawStepH(s int) int    { return PropYawSteps/2 - s }
func flipYawStepV(s int) int    { return -s }
func rotateYawStepCW(s int) int { return s + PropYawSteps/4 }

// permuteSkins moves each face's skin to its transformed direction.
func permuteSkins(s [FacingCount]byte, dirMap func(int) int) [FacingCount]byte {
	var out [FacingCount]byte
	for d := 0; d < FacingCount; d++ {
		out[dirMap(d)] = s[d]
	}
	return out
}

// FlipHorizontal mirrors the region left↔right.
func (r TileRegion) FlipHorizontal() TileRegion {
	out := r
	out.Layers = mapPlanes(r.Layers, reverseEachRow)
	out.Solids = mapPlanes(r.Solids, reverseEachRow)
	out.PropStack = mapPlanes(r.PropStack, reverseEachRow)
	out.DecorStack = mapPlanes(r.DecorStack, reverseEachRow)
	out.PropLevels = reverseEachRow(r.PropLevels)
	out.DecorLevels = reverseEachRow(r.DecorLevels)
	out.PropYaw = reverseEachRow(r.PropYaw)
	if len(out.Layers) > regionFloorLayer {
		remapRowChars(out.Layers[regionFloorLayer], func(c byte) byte { return remapRampChar(c, FlipFacingH) })
	}
	remapRowChars(out.PropYaw, func(c byte) byte { return remapYawChar(c, flipYawStepH) })
	out.Faces = transformFaces(r.Faces, func(o FaceOverride) FaceOverride {
		o.X = r.W - 1 - o.X
		o.Skins = permuteSkins(o.Skins, FlipFacingH)
		return o
	})
	return out
}

// FlipVertical mirrors the region top↔bottom.
func (r TileRegion) FlipVertical() TileRegion {
	out := r
	out.Layers = mapPlanes(r.Layers, reverseRowOrder)
	out.Solids = mapPlanes(r.Solids, reverseRowOrder)
	out.PropStack = mapPlanes(r.PropStack, reverseRowOrder)
	out.DecorStack = mapPlanes(r.DecorStack, reverseRowOrder)
	out.PropLevels = reverseRowOrder(r.PropLevels)
	out.DecorLevels = reverseRowOrder(r.DecorLevels)
	out.PropYaw = reverseRowOrder(r.PropYaw)
	if len(out.Layers) > regionFloorLayer {
		remapRowChars(out.Layers[regionFloorLayer], func(c byte) byte { return remapRampChar(c, FlipFacingV) })
	}
	remapRowChars(out.PropYaw, func(c byte) byte { return remapYawChar(c, flipYawStepV) })
	out.Faces = transformFaces(r.Faces, func(o FaceOverride) FaceOverride {
		o.Z = r.H - 1 - o.Z
		o.Skins = permuteSkins(o.Skins, FlipFacingV)
		return o
	})
	return out
}

// Rotate90CW turns the region 90° clockwise (W and H swap).
func (r TileRegion) Rotate90CW() TileRegion {
	out := r
	out.W, out.H = r.H, r.W
	out.Layers = mapPlanes(r.Layers, func(rows []string) []string { return rotateGridCW(rows, r.W, r.H, TileOpen) })
	out.Solids = mapPlanes(r.Solids, func(rows []string) []string { return rotateGridCW(rows, r.W, r.H, SolidAir) })
	out.PropStack = mapPlanes(r.PropStack, func(rows []string) []string { return rotateGridCW(rows, r.W, r.H, TilePropEmpty) })
	out.DecorStack = mapPlanes(r.DecorStack, func(rows []string) []string { return rotateGridCW(rows, r.W, r.H, DecorEmpty) })
	out.PropLevels = rotateGridCW(r.PropLevels, r.W, r.H, PropLevelAuto)
	out.DecorLevels = rotateGridCW(r.DecorLevels, r.W, r.H, PropLevelAuto)
	out.PropYaw = rotateGridCW(r.PropYaw, r.W, r.H, PropYawAuto)
	if len(out.Layers) > regionFloorLayer {
		remapRowChars(out.Layers[regionFloorLayer], func(c byte) byte { return remapRampChar(c, RotateFacingCW) })
	}
	remapRowChars(out.PropYaw, func(c byte) byte { return remapYawChar(c, rotateYawStepCW) })
	out.Faces = transformFaces(r.Faces, func(o FaceOverride) FaceOverride {
		o.X, o.Z = r.H-1-o.Z, o.X
		o.Skins = permuteSkins(o.Skins, RotateFacingCW)
		return o
	})
	return out
}

// rotateGridCW rebuilds a w×h grid as an h-wide, w-tall grid rotated 90° clockwise:
// new[x][h-1-z] = old[z][x]. Missing cells (ragged) pad with pad.
func rotateGridCW(rows []string, w, h int, pad byte) []string {
	if rows == nil {
		return nil
	}
	out := make([]string, w)
	for x := 0; x < w; x++ {
		buf := make([]byte, h)
		for z := 0; z < h; z++ {
			c := pad
			if z < len(rows) && x < len(rows[z]) {
				c = rows[z][x]
			}
			buf[h-1-z] = c
		}
		out[x] = string(buf)
	}
	return out
}

// transformFaces applies fn to a copy of each face override (nil-safe).
func transformFaces(faces []FaceOverride, fn func(FaceOverride) FaceOverride) []FaceOverride {
	if faces == nil {
		return nil
	}
	out := make([]FaceOverride, len(faces))
	for i, o := range faces {
		out[i] = fn(o)
	}
	return out
}

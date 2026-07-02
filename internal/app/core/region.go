package core

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
	Faces       []FaceOverride
}

// Empty reports whether the region has nothing to paste.
func (r TileRegion) Empty() bool { return r.W <= 0 || r.H <= 0 || len(r.Layers) == 0 }

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

// CopyRegion snapshots the inclusive rectangle (x0,z0)-(x1,z1) (any corner
// order) across all grid layers; coords clamped to the area, degenerate yields
// empty. Returned strings are immutable, so the snapshot survives later source edits.
func CopyRegion(a *AreaDefinition, x0, z0, x1, z1 int) TileRegion {
	if a == nil || a.Width <= 0 || a.Height <= 0 {
		return TileRegion{}
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
		return
	}
	// Same full-column stamp as the prop/decor stacks (cells above the copied planes
	// clear to air); setSolidCell grows the stack as needed, then trim trailing air.
	EnsureSolids(a)
	a.pasteScatterStack(r.Solids, len(a.Solids), SolidAir, r.W, r.H, atX, atZ, a.setSolidCell)
	a.trimTopAir()
}

// pasteRegionScatter stamps a region's per-floor prop/decor stacks and legacy level
// tags at (atX,atZ). Stacks overwrite the full column in the rectangle (dest cubes
// above the copied planes clear to blank), mirroring pasteRegionSolids; the level
// grids are stamped as an overlay so a legacy source's props land on the right floor.
func (a *AreaDefinition) pasteRegionScatter(r TileRegion, atX, atZ int) {
	if len(r.PropStack) > 0 {
		a.EnsurePropStack()
		a.pasteScatterStack(r.PropStack, len(a.PropStack), TilePropEmpty, r.W, r.H, atX, atZ, a.SetProp)
	}
	if len(r.DecorStack) > 0 {
		a.EnsureDecorStack()
		a.pasteScatterStack(r.DecorStack, len(a.DecorStack), DecorEmpty, r.W, r.H, atX, atZ, a.SetDecor)
	}
	a.pasteLevelGrid(&a.PropLevels, r.PropLevels, atX, atZ)
	a.pasteLevelGrid(&a.DecorLevels, r.DecorLevels, atX, atZ)
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

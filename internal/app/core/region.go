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
}

// Empty reports whether the region has nothing to paste.
func (r TileRegion) Empty() bool { return r.W <= 0 || r.H <= 0 || len(r.Layers) == 0 }

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
			src := (*lp)[z]
			lo, hi := x0, x1+1
			if lo > len(src) {
				lo = len(src)
			}
			if hi > len(src) {
				hi = len(src)
			}
			rows[i] = src[lo:hi]
		}
		out.Layers[li] = rows
	}
	// Voxel source: also snapshot the cube stack for the rectangle, one row per
	// (level, z). Heightfield sources leave Solids nil (Elevation layer carries them).
	if len(a.Solids) > 0 {
		planes := make([][]string, len(a.Solids))
		for L := range a.Solids {
			rows := make([]string, h)
			for i := 0; i < h; i++ {
				z := z0 + i
				if z >= len(a.Solids[L]) {
					continue
				}
				src := a.Solids[L][z]
				lo, hi := x0, x1+1
				if lo > len(src) {
					lo = len(src)
				}
				if hi > len(src) {
					hi = len(src)
				}
				rows[i] = src[lo:hi]
			}
			planes[L] = rows
		}
		out.Solids = planes
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
}

// pasteRegionSolids stamps a voxel region's cube stack at (atX,atZ). Materializes
// the destination so the paste overwrites the full column (cells above the copied
// stack within the rectangle are cleared to air), then trims trailing all-air planes.
func (a *AreaDefinition) pasteRegionSolids(r TileRegion, atX, atZ int) {
	if len(r.Solids) == 0 {
		return
	}
	EnsureSolids(a)
	a.growSolidsTo(len(r.Solids))
	for L := 0; L < len(a.Solids); L++ {
		for i := 0; i < r.H; i++ {
			z := atZ + i
			if z < 0 || z >= a.Height {
				continue
			}
			var row string
			if L < len(r.Solids) && i < len(r.Solids[L]) {
				row = r.Solids[L][i]
			}
			for j := 0; j < r.W; j++ {
				x := atX + j
				if x < 0 || x >= a.Width {
					continue
				}
				c := byte(SolidAir)
				if j < len(row) {
					c = row[j]
				}
				a.setSolidCell(x, L, z, c)
			}
		}
	}
	a.trimTopAir()
}

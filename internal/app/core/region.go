package core

// region.go is the data transform behind the editor's region copy/paste: a
// rectangular block of the grid layers snapshotted from one place and
// stamped at another. Pure (raylib-free) so it's unit-tested in the core suite;
// the editor owns the marquee selection + undo around these calls.
//
// Scope: the grid layers only — exactly the set gridLayers() enumerates (walls,
// floor, decor, props, ceiling, elevation, …). Entities (packs/chests/doors/
// crystals) live in separate spawn lists and are NOT part of a region copy.

// TileRegion is a copied rectangle of the grid layers. Layers is indexed in
// gridLayers() order (walls, floor, decor, props, ceiling, elevation); each is up
// to H rows, each row up to W chars (rows may be shorter if the source was
// ragged — paste tolerates that).
type TileRegion struct {
	W, H   int
	Layers [][]string
}

// Empty reports whether the region has nothing to paste.
func (r TileRegion) Empty() bool { return r.W <= 0 || r.H <= 0 || len(r.Layers) == 0 }

// CopyRegion snapshots the inclusive rectangle (x0,z0)-(x1,z1) — given in any
// corner order — across all six grid layers. Coordinates are clamped to the
// area; a degenerate rect/area yields an empty region. The returned strings are
// fresh (Go string slicing shares backing bytes immutably), so the snapshot is
// safe to hold across later edits to the source.
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
				continue // ragged: leave row "" (paste skips missing cells)
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
	return out
}

// PasteRegion stamps r with its top-left at (atX,atZ), overwriting every grid
// layer cell that lands in bounds and skips the rest (so a paste near an edge
// clips cleanly). Because a region carries ALL grid layers, the stamped block is
// internally consistent — it reproduces exactly the source cells, so it can't
// create a wall+prop conflict the source didn't have. No-op on an empty region.
func (a *AreaDefinition) PasteRegion(r TileRegion, atX, atZ int) {
	if a == nil || r.Empty() {
		return
	}
	layers := a.gridLayers()
	for li := 0; li < len(layers) && li < len(r.Layers); li++ {
		lp := layers[li]
		// Per-layer blank for padding a short (ragged) destination row up to the
		// area width — the shared per-layer open char (reorder-safe vs gridLayers'
		// order), the same mapping AreaContentEqual uses.
		blank := a.layerBlank(lp)
		for i, row := range r.Layers[li] {
			z := atZ + i
			if z < 0 || z >= len(*lp) {
				continue
			}
			dest := []byte((*lp)[z])
			// A row shorter than the area width would silently drop in-bounds
			// paste cells past its end; pad it to width with the layer blank first
			// so the stamp lands fully (loaded maps are rectangular, but hand-built
			// fixtures may not be).
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
}

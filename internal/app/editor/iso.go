package editor

import (
	rl "github.com/gen2brain/raylib-go/raylib"
)

// Isometric preview — a read-only 3D-ish view of the map toggled from
// View ▸ Isometric View (or the `I` key, for Iso). The flat top-down grid
// can't show elevation; this projects every column as a lifted block so hills,
// pits, and land bridges read at a glance. Painting and hover are suppressed
// while it's on (cellAt early-returns in iso mode) — it's a look, not a second
// edit surface; you flip back to top-down to paint.
//
// Tab already cycles the active layer in the editor, so the toggle lives on `I`
// rather than Tab.

// Iso tunables: the per-level vertical rise (in halfW units) and the side-face
// shades that fake one-directional lighting so the block facets read as solid.
const (
	isoLiftFrac       = float32(0.55)
	isoLeftFaceShade  = float32(0.62)
	isoRightFaceShade = float32(0.45)
)

var (
	isoEdge     = rl.NewColor(20, 24, 30, 90)  // thin cell outline on the top cap
	isoEmptyTop = rl.NewColor(70, 76, 86, 255) // top color when the Floor layer is hidden
)

// toggleIsoView flips the isometric preview on/off (View menu + `I` hotkey).
func toggleIsoView(s *State) {
	s.isoView = !s.isoView
	if s.isoView {
		s.flash("Isometric view (read-only) — press I for top-down")
	} else {
		s.flash("Top-down view")
	}
}

// isoProj is the per-frame projection: where tile (0,0)'s top-face center lands
// on screen, the iso tile half-extents, the lift per level, and the panel clip
// bounds used to cull off-screen columns.
type isoProj struct {
	originX, originY               float32
	halfW, halfH, lift             float32
	clipX0, clipY0, clipX1, clipY1 float32
}

// topCenter projects a tile at normalized elevation lvl to the screen-space
// center of its top face (2:1 diamond, lifted up by lvl levels).
func (p isoProj) topCenter(x, z, lvl int) (float32, float32) {
	sx := p.originX + (float32(x)-float32(z))*p.halfW
	sy := p.originY + (float32(x)+float32(z))*p.halfH - float32(lvl)*p.lift
	return sx, sy
}

// drawGridIso renders the map in a 2:1 isometric projection: every column is a
// block lifted by its elevation above the map's lowest level, drawn back-to-
// front (painter's order) so nearer/taller columns occlude farther ones. It's
// deliberately minimal — floor-colored top faces with shaded side walls, no
// entities/props/decor — a height-reading preview rather than a second editor.
func drawGridIso(s *State, font rl.Font) {
	W, H := s.area.Width, s.area.Height
	if W == 0 || H == 0 {
		return
	}
	grid := s.rect.grid

	// Normalize elevation to the map's lowest column so a flat map reads flat
	// (stored levels are baseline-relative, and voxel vs heightfield maps
	// disagree on the absolute level; subtracting the min self-corrects both).
	// Track the span too, to scale the lift so a tall map still fits the panel.
	minLvl, maxLvl := 1<<30, -(1 << 30)
	for z := 0; z < H; z++ {
		for x := 0; x < W; x++ {
			l := s.area.ElevationLevelAt(x, z)
			if l < minLvl {
				minLvl = l
			}
			if l > maxLvl {
				maxLvl = l
			}
		}
	}
	span := maxLvl - minLvl // 0 on a flat map

	// Auto-fit the diamond to the grid panel, then apply the user's zoom. The
	// iso footprint is (W+H) tiles wide and (W+H) half-heights tall plus the
	// elevation lift, so it's sized independently of the top-down cellPx fit.
	availW := grid.Width - 2*gridMargin
	availH := grid.Height - 2*gridMargin
	denomW := float32(W + H)
	// Vertical room: base diamond is denomW*halfH (= denomW*0.5*halfW) plus the
	// lift adds span*isoLiftFrac*halfW, plus a tile of slack top & bottom — all
	// expressed in halfW units so one division yields halfW.
	denomH := denomW*0.5 + float32(span)*isoLiftFrac + 2
	halfW := availW / denomW
	if hh := availH / denomH; hh < halfW {
		halfW = hh
	}
	halfW *= s.zoom
	if halfW < 2 {
		halfW = 2
	}
	p := isoProj{
		halfW: halfW,
		halfH: halfW * 0.5,
		lift:  halfW * isoLiftFrac,
	}

	// Center the footprint in the panel (plus the user's pan). The mean tile
	// sits at ((W-1)-(H-1))/2 in (x-z) and ((W-1)+(H-1))/2 in (x+z); anchor that
	// to the panel center, biased up by half the lift span so extruded blocks
	// stay centered rather than sinking.
	cx := grid.X + grid.Width/2 + s.panX
	cy := grid.Y + grid.Height/2 + s.panY
	meanXmZ := float32((W-1)-(H-1)) / 2
	meanXpZ := float32((W-1)+(H-1)) / 2
	p.originX = cx - meanXmZ*p.halfW
	p.originY = cy - meanXpZ*p.halfH + float32(span)*p.lift/2
	p.clipX0, p.clipY0 = grid.X, grid.Y
	p.clipX1, p.clipY1 = grid.X+grid.Width, grid.Y+grid.Height

	// Clip to the panel so a zoomed-in scene doesn't spill over the palette /
	// metadata columns (which draw before the grid). Top-down relies on a
	// per-tile cull; iso scissors and additionally skips off-screen columns.
	rl.BeginScissorMode(int32(grid.X), int32(grid.Y), int32(grid.Width), int32(grid.Height))
	defer rl.EndScissorMode()

	floorHidden := s.layerHidden[LayerFloor]

	// Painter's order: sweep diagonals of constant (x+z), far (small sum) first.
	maxSum := (W - 1) + (H - 1)
	for sum := 0; sum <= maxSum; sum++ {
		xLo := 0
		if sum-(H-1) > xLo {
			xLo = sum - (H - 1)
		}
		xHi := sum
		if W-1 < xHi {
			xHi = W - 1
		}
		for x := xLo; x <= xHi; x++ {
			z := sum - x
			lvl := s.area.ElevationLevelAt(x, z) - minLvl
			drawIsoColumn(s, x, z, lvl, p, floorHidden)
		}
	}
}

// drawIsoColumn paints one map column as an iso block: shaded side walls
// extruded down to the base plane, then the floor-colored top cap with a thin
// cell outline. Columns whose projected footprint falls outside the panel are
// culled cheaply before any draw.
func drawIsoColumn(s *State, x, z, lvl int, p isoProj, floorHidden bool) {
	sx, sy := p.topCenter(x, z, lvl)
	w, h := p.halfW, p.halfH
	side := float32(lvl) * p.lift
	// Cull: the column spans [sx-w, sx+w] horizontally and [sy-h, sy+h+side]
	// vertically (cap top to base-plane bottom). Skip if it can't touch the panel.
	if sx+w < p.clipX0 || sx-w > p.clipX1 || sy-h > p.clipY1 || sy+h+side < p.clipY0 {
		return
	}

	top := rl.NewVector2(sx, sy-h)
	left := rl.NewVector2(sx-w, sy)
	bottom := rl.NewVector2(sx, sy+h)
	right := rl.NewVector2(sx+w, sy)

	base := floorColor(s.area.Floor[z][x]) // rl.Color (alias of color.RGBA)
	if floorHidden {
		base = isoEmptyTop
	}

	// Side walls first (the cap draws over their top edge). Skipped at level 0
	// (flat — no visible sides). Quad corners are wound CCW in screen-Y-down so
	// they survive backface culling (see render.drawTriangleCCW for the why).
	if lvl > 0 {
		lp := rl.NewVector2(left.X, left.Y+side)
		bp := rl.NewVector2(bottom.X, bottom.Y+side)
		rp := rl.NewVector2(right.X, right.Y+side)
		drawIsoQuad(lp, bp, bottom, left, isoShade(base, isoLeftFaceShade))   // left face
		drawIsoQuad(bp, rp, right, bottom, isoShade(base, isoRightFaceShade)) // right face
	}

	// Top cap + thin cell outline so abutting same-height tiles still read apart.
	drawIsoQuad(top, left, bottom, right, base)
	rl.DrawLineEx(top, left, 1, isoEdge)
	rl.DrawLineEx(left, bottom, 1, isoEdge)
	rl.DrawLineEx(bottom, right, 1, isoEdge)
	rl.DrawLineEx(right, top, 1, isoEdge)
}

// drawIsoQuad fills a convex quad given its four corners in CCW (screen-Y-down)
// order, as two triangles of the same winding. raylib's 2D pipeline culls
// CW-wound triangles on some drivers, so callers must pass CCW corners.
func drawIsoQuad(a, b, c, d rl.Vector2, col rl.Color) {
	rl.DrawTriangle(a, b, c, col)
	rl.DrawTriangle(a, c, d, col)
}

// isoShade scales a color's RGB by f (clamped 0..255), darkening side faces so
// the block facets read as lit from one side.
func isoShade(c rl.Color, f float32) rl.Color {
	return rl.NewColor(scaleChan(c.R, f), scaleChan(c.G, f), scaleChan(c.B, f), c.A)
}

func scaleChan(v uint8, f float32) uint8 {
	r := float32(v) * f
	if r > 255 {
		r = 255
	}
	if r < 0 {
		r = 0
	}
	return uint8(r)
}

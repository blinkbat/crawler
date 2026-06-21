package editor

import (
	"fmt"
	"math"

	"crawler/internal/app/core"
	"crawler/internal/app/render"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// 3D view — a rotatable, simplified solid-block view of the map toggled from
// View ▸ Isometric View (or `I`). Unlike the flat top-down grid it shows
// elevation directly: every column is extruded to its height so hills, pits, and
// land bridges read at a glance, and you can orbit around them. It's also a
// LIMITED edit surface — elevation only: left-click raises the hovered column,
// right-click lowers it, and (with the Ramp tool on) left-click drops a ramp /
// right-click clears one. The flat layers (floor/walls/decor/props/entities)
// stay on the top-down grid; press `I` to flip back for those.
//
// Kept deliberately simple (flat-shaded cubes, no props/decor/entities) so it's
// a fast height-editing surface, not a second full renderer. It renders into an
// off-screen RenderTexture sized to the grid panel and blits that in — the same
// trick the Foe/Object Visualizers use — so the 3D scene stays inside the canvas
// and picking maps cleanly to the panel rect.

// Iso scene tunables (world units inside the off-screen render).
const (
	isoTileW    = float32(1.0)  // one tile = one world unit
	isoLevelH   = float32(0.55) // vertical rise per elevation level
	isoFovy     = float32(46)
	isoPitchDeg = 38.0 // camera tilt above the horizon
	// isoPanSpeed scales middle-drag panning (screen px → world units).
	isoPanSpeed = float32(0.02)
)

var (
	isoEdge      = rl.NewColor(20, 24, 30, 130)   // cube wire edges
	isoEmptyTop  = rl.NewColor(70, 76, 86, 255)   // top color when the Floor layer is hidden
	isoHoverWire = rl.NewColor(255, 224, 130, 255) // hovered-column highlight
	isoRampMark  = rl.NewColor(198, 168, 104, 255) // ramp top marker (brass, matches the map)
	isoBG        = rl.NewColor(26, 28, 34, 255)    // off-screen clear behind the blocks
)

// toggleIsoView flips the 3D view on/off (View menu + `I` hotkey). Frees the
// off-screen target on the way out so the GPU handle isn't held in top-down mode.
func toggleIsoView(s *State) {
	s.isoView = !s.isoView
	if s.isoView {
		s.flash("3D view — Q/E orbit · wheel zoom · L-click raise / R-click lower · I for top-down")
		return
	}
	s.freeIsoRT()
	s.flash("Top-down view")
}

// Close releases the editor's GPU resources — currently the 3D-view render
// target. Call when leaving the editor scene so the handle isn't orphaned when
// the editor is exited while the 3D view is on (freeIsoRT otherwise only runs on
// the I-toggle / a panel resize). Idempotent.
func (s *State) Close() { s.freeIsoRT() }

// freeIsoRT releases the off-screen iso target if allocated (idempotent).
func (s *State) freeIsoRT() {
	if s.isoRT.ID != 0 {
		rl.UnloadRenderTexture(s.isoRT)
		s.isoRT = rl.RenderTexture2D{}
		s.isoRTW, s.isoRTH = 0, 0
	}
}

// ensureIsoRT (re)allocates the off-screen target to match the grid panel size,
// reusing it across frames and only reallocating when the panel resizes.
func (s *State) ensureIsoRT(w, h int32) {
	if s.isoRT.ID != 0 && s.isoRTW == w && s.isoRTH == h {
		return
	}
	s.freeIsoRT()
	s.isoRT = rl.LoadRenderTexture(w, h)
	s.isoRTW, s.isoRTH = w, h
}

// isoLevelSpan returns the lowest and highest elevation level across the map —
// columns are drawn relative to the lowest so a flat map reads flat and a tall
// one still fits the panel.
func isoLevelSpan(s *State) (minL, maxL int) {
	minL, maxL = 1<<30, -(1 << 30)
	for z := 0; z < s.area.Height; z++ {
		for x := 0; x < s.area.Width; x++ {
			l := s.area.ElevationLevelAt(x, z)
			if l < minL {
				minL = l
			}
			if l > maxL {
				maxL = l
			}
		}
	}
	if minL > maxL { // empty map guard
		minL, maxL = 0, 0
	}
	return minL, maxL
}

// isoColumnHeight is the world-space height of column (x,z): at least one level
// tall (so flat maps still show a pickable slab) plus its rise above the floor.
func isoColumnHeight(s *State, x, z, minL int) float32 {
	return float32(s.area.ElevationLevelAt(x, z)-minL+1) * isoLevelH
}

// isoCamera builds the orbit camera from the view state: one of four 45°-offset
// yaw angles, a fixed downward pitch, and a fit-to-map distance scaled by zoom,
// targeting the (panned) map center. The level span is passed in (computed once
// per frame by the caller) so the whole-map scan isn't repeated per camera build.
func (s *State) isoCamera(minL, maxL int) rl.Camera3D {
	W, H := s.area.Width, s.area.Height
	tall := float32(maxL-minL+1) * isoLevelH
	cx := float32(W-1) / 2
	cz := float32(H-1) / 2
	target := rl.NewVector3(cx+s.isoTargetX, tall*0.5, cz+s.isoTargetZ)

	yaw := float64(s.isoYaw)*(math.Pi/2) + math.Pi/4
	pitch := isoPitchDeg * math.Pi / 180
	dir := rl.NewVector3(
		float32(math.Cos(pitch)*math.Cos(yaw)),
		float32(math.Sin(pitch)),
		float32(math.Cos(pitch)*math.Sin(yaw)),
	)
	radius := 0.5*float32(math.Hypot(float64(W), float64(H))) + tall
	if radius < 1 {
		radius = 1
	}
	dist := radius / float32(math.Sin(float64(isoFovy)*math.Pi/360)) * 1.1
	if s.isoZoom > 0 {
		dist /= s.isoZoom
	}
	return rl.Camera3D{
		Position:   rl.Vector3Add(target, rl.Vector3Scale(dir, dist)),
		Target:     target,
		Up:         rl.NewVector3(0, 1, 0),
		Fovy:       isoFovy,
		Projection: rl.CameraPerspective,
	}
}

// isoColumnBox is the world-space bounding box of column (x,z), used for both
// drawing and ray-pick so the click target matches the rendered block exactly.
func (s *State) isoColumnBox(x, z, minL int) (center, size rl.Vector3) {
	ht := isoColumnHeight(s, x, z, minL)
	center = rl.NewVector3(float32(x), ht*0.5, float32(z))
	size = rl.NewVector3(isoTileW*0.98, ht, isoTileW*0.98)
	return center, size
}

// isoRayInRect builds the world-space pick ray for a mouse point over `rect`,
// for a camera whose scene was rendered into an RT of rect's size and blitted at
// rect. raylib's GetScreenToWorldRay assumes a full-window projection, so this
// replicates its unproject math with the rect's own dimensions instead.
func isoRayInRect(mp rl.Vector2, rect rl.Rectangle, cam rl.Camera3D) rl.Ray {
	ndcX := 2*(mp.X-rect.X)/rect.Width - 1
	ndcY := 1 - 2*(mp.Y-rect.Y)/rect.Height
	aspect := rect.Width / rect.Height
	proj := rl.MatrixPerspective(cam.Fovy*math.Pi/180, aspect, 0.01, 1000)
	view := rl.MatrixLookAt(cam.Position, cam.Target, cam.Up)
	near := rl.Vector3Unproject(rl.NewVector3(ndcX, ndcY, 0), proj, view)
	far := rl.Vector3Unproject(rl.NewVector3(ndcX, ndcY, 1), proj, view)
	return rl.NewRay(cam.Position, rl.Vector3Normalize(rl.Vector3Subtract(far, near)))
}

// isoPick returns the column under the mouse (nearest ray-hit), or (-1,-1) when
// the cursor is off the canvas or misses every block. minL is the precomputed
// level floor (shared with the caller's camera build) so the column-box math
// doesn't rescan the map.
func (s *State) isoPick(cam rl.Camera3D, mp rl.Vector2, minL int) (int, int) {
	if !pointIn(mp, s.rect.grid) {
		return -1, -1
	}
	ray := isoRayInRect(mp, s.rect.grid, cam)
	bestX, bestZ := -1, -1
	best := float32(math.MaxFloat32)
	for z := 0; z < s.area.Height; z++ {
		for x := 0; x < s.area.Width; x++ {
			center, size := s.isoColumnBox(x, z, minL)
			half := rl.Vector3Scale(size, 0.5)
			bb := rl.NewBoundingBox(rl.Vector3Subtract(center, half), rl.Vector3Add(center, half))
			if c := rl.GetRayCollisionBox(ray, bb); c.Hit && c.Distance < best {
				best, bestX, bestZ = c.Distance, x, z
			}
		}
	}
	return bestX, bestZ
}

// drawGridIso renders the map as extruded 3D blocks into the off-screen target
// and blits it into the grid panel, then overlays a hovered-column readout. The
// flat top-down overlays are skipped (drawGrid returns right after this).
func drawGridIso(s *State, font rl.Font) {
	grid := s.rect.grid
	w, h := int32(grid.Width), int32(grid.Height)
	if w <= 0 || h <= 0 || s.area.Width == 0 || s.area.Height == 0 {
		return
	}
	s.ensureIsoRT(w, h)
	if s.isoRT.ID == 0 {
		return
	}
	minL, maxL := isoLevelSpan(s)
	cam := s.isoCamera(minL, maxL)
	floorHidden := s.layerHidden[LayerFloor]

	rl.BeginTextureMode(s.isoRT)
	rl.ClearBackground(isoBG)
	rl.BeginMode3D(cam)
	for z := 0; z < s.area.Height; z++ {
		for x := 0; x < s.area.Width; x++ {
			s.drawIsoColumn(x, z, minL, floorHidden, x == s.isoHoverX && z == s.isoHoverZ)
		}
	}
	rl.EndMode3D()
	rl.EndTextureMode()

	// RenderTextures store rows bottom-up; negate source height to blit upright.
	rl.DrawTextureRec(s.isoRT.Texture,
		rl.NewRectangle(0, 0, float32(w), -float32(h)),
		rl.NewVector2(grid.X, grid.Y), rl.White)

	drawIsoReadout(s, font, grid)
}

// drawIsoColumn paints one map column: a darker body cube with a brighter
// floor-colored top cap, wire edges, an optional ramp marker, and a gold
// highlight when hovered.
func (s *State) drawIsoColumn(x, z, minL int, floorHidden, hovered bool) {
	center, size := s.isoColumnBox(x, z, minL)
	// Route the floor read through the bounds-safe cellAt (not raw Floor[z][x]):
	// a loaded/ragged area can have rows shorter than Width or fewer rows than
	// Height, and the raw index would panic on entering 3D view. Missing cells
	// fall back to the empty-top color.
	top := isoEmptyTop
	if fb, ok := cellAt(s.area.Floor, x, z); ok {
		top = rl.Color(floorColor(fb))
	}
	if floorHidden {
		top = isoEmptyTop
	}
	// Darker body so the bright top cap reads as a lit surface (fakes one-axis
	// lighting without a shader — flat DrawCube is otherwise unshaded).
	rl.DrawCubeV(center, size, render.ShadeColor(top, 0.7))
	capCenter := rl.NewVector3(center.X, center.Y+size.Y/2-0.01, center.Z)
	rl.DrawCubeV(capCenter, rl.NewVector3(size.X, 0.02, size.Z), top)
	rl.DrawCubeWiresV(center, size, isoEdge)

	if _, ok := s.area.RampAt(x, z); ok {
		// A small brass slab on the cap marks a ramp connector (direction detail
		// stays on the top-down view; this is just "there's a ramp here").
		mark := rl.NewVector3(center.X, center.Y+size.Y/2+0.02, center.Z)
		rl.DrawCubeV(mark, rl.NewVector3(size.X*0.5, 0.06, size.Z*0.5), isoRampMark)
	}
	if hovered {
		hi := rl.NewVector3(size.X+0.04, size.Y+0.04, size.Z+0.04)
		rl.DrawCubeWiresV(center, hi, isoHoverWire)
	}
}

// drawIsoReadout shows the hovered column's coordinates + elevation level (signed
// from ground) in the canvas corner, plus a one-line control hint.
func drawIsoReadout(s *State, font rl.Font, grid rl.Rectangle) {
	hint := "3D · Q/E orbit · wheel zoom · L raise / R lower · I top-down"
	rl.DrawTextEx(font, hint, rl.NewVector2(grid.X+8, grid.Y+8), editorFontHint, 1, rl.NewColor(210, 214, 222, 200))
	if s.isoHoverX >= 0 {
		lvl := s.area.ElevationLevelAt(s.isoHoverX, s.isoHoverZ) - core.ElevationBaseline
		txt := fmt.Sprintf("(%d, %d)  level %+d", s.isoHoverX, s.isoHoverZ, lvl)
		rl.DrawTextEx(font, txt, rl.NewVector2(grid.X+8, grid.Y+8+editorFontHint+4), editorFontHint, 1, rl.NewColor(255, 224, 130, 235))
	}
}

// updateIsoCanvas drives the 3D view's input: orbit (Q/E), zoom (wheel),
// pan (middle-drag), and elevation editing on the hovered column. Called from
// updateMouse while isoView is on; the top-down paint/pan path is skipped.
func updateIsoCanvas(s *State, mp rl.Vector2) {
	if rl.IsKeyPressed(rl.KeyE) {
		s.isoYaw = (s.isoYaw + 1) & 3
	}
	if rl.IsKeyPressed(rl.KeyQ) {
		s.isoYaw = (s.isoYaw + 3) & 3
	}

	minL, maxL := isoLevelSpan(s)
	cam := s.isoCamera(minL, maxL)
	s.isoHoverX, s.isoHoverZ = s.isoPick(cam, mp, minL)

	if !pointIn(mp, s.rect.grid) {
		return // orbit keys still applied above; no edit/zoom/pan off-canvas
	}

	if wheel := rl.GetMouseWheelMove(); wheel != 0 {
		// Guard a zero seed: a multiplicative zoom stuck at 0 would never recover
		// (0 × factor = 0) and the camera's dist/isoZoom would freeze. Clamp the
		// base into range before scaling so the wheel always responds.
		if s.isoZoom <= 0 {
			s.isoZoom = 1
		}
		s.isoZoom *= 1 + 0.12*wheel
		s.isoZoom = core.Clamp(s.isoZoom, float32(0.3), float32(6))
	}
	if rl.IsMouseButtonDown(rl.MouseMiddleButton) {
		d := rl.GetMouseDelta()
		yaw := float64(s.isoYaw)*(math.Pi/2) + math.Pi/4
		rx, rz := float32(math.Cos(yaw+math.Pi/2)), float32(math.Sin(yaw+math.Pi/2))
		fx, fz := float32(math.Cos(yaw)), float32(math.Sin(yaw))
		k := isoPanSpeed / s.isoZoom
		s.isoTargetX -= (rx*d.X + fx*d.Y) * k
		s.isoTargetZ -= (rz*d.X + fz*d.Y) * k
	}

	hx, hz := s.isoHoverX, s.isoHoverZ
	if hx < 0 {
		return
	}
	if s.rampMode {
		if rl.IsMouseButtonPressed(rl.MouseLeftButton) {
			placeRamp(s, hx, hz)
		}
		if rl.IsMouseButtonPressed(rl.MouseRightButton) {
			isoClearRamp(s, hx, hz)
		}
		return
	}
	if rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		isoSetColumnLevel(s, hx, hz, +1)
	}
	if rl.IsMouseButtonPressed(rl.MouseRightButton) {
		isoSetColumnLevel(s, hx, hz, -1)
	}
}

// isoSetColumnLevel raises (delta +1) or lowers (delta -1) the column's ground
// level, clamped to [0, maxEditLevel], routing through the shared ground-level
// setter (voxel SetColumnTop / heightfield Elevation char) and the undo hook.
func isoSetColumnLevel(s *State, x, z, delta int) {
	cur := s.area.ElevationLevelAt(x, z)
	next := cur + delta
	if next < 0 {
		next = 0
	}
	if next > maxEditLevel {
		next = maxEditLevel
	}
	if next == cur {
		return
	}
	pushUndo(s)
	setTileGroundLevel(s, x, z, next)
	s.dirty = true
}

// isoClearRamp removes a ramp connector from (x,z) (resets its floor to auto),
// the 3D-view counterpart of the top-down ramp-erase. No-op when there's no ramp.
func isoClearRamp(s *State, x, z int) {
	if _, ok := s.area.RampAt(x, z); !ok {
		return
	}
	pushUndo(s)
	setLayerCell(&s.area.Floor, x, z, core.FloorAuto)
	s.dirty = true
}

package editor

import (
	"fmt"
	"math"

	"crawler/internal/app/core"
	"crawler/internal/app/render"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// 3D view (View ▸ Isometric View / `I`): a FULL editing surface that renders the
// real lit world via render.DrawArea (not stand-in cubes) into an off-screen
// RenderTexture and blits it into the grid panel. The same tools/brushes/layers
// as the top-down canvas apply at the ray-picked tile, on the active floor
// (editLevel). Camera: right-drag orbits freely, Shift+right-drag pans, arrows
// pan, wheel zooms, Q/E snap the yaw by 90° (the mousewheel BUTTON is never bound).
// The chrome panels (palette, Levels, menus) stay live via handleChromeInput, so
// the active floor stays selectable while editing in 3D.

// Iso scene tunables (world units inside the off-screen render).
const (
	isoFovy     = float32(46)
	isoPitchDeg = 38.0           // default camera tilt above the horizon
	isoPanSpeed = float32(0.05)  // shift+middle-drag pan: screen px → world units (real scale)
	isoOrbitRate = float32(0.004) // right-drag tumble: radians per screen px (kept gentle — full-speed felt twitchy)
	isoMinPitch = float32(0.12)  // tumble pitch clamp (near-horizon)
	isoMaxPitch = float32(1.50)  // tumble pitch clamp (near top-down)
	isoMinZoom  = float32(0.3)   // 3D-view zoom clamp (parallels minZoom/maxZoom for the canvas)
	isoMaxZoom  = float32(6)

	// Default orbit, shared by freshState (initial camera) and resetView (Home) so
	// the two can't drift.
	isoDefaultYaw   = float32(math.Pi / 4)              // 45° default orbit
	isoDefaultPitch = float32(isoPitchDeg * math.Pi / 180) // tilt above horizon
)

var (
	isoHoverWire   = editorGold                     // hovered-cell / placeable preview
	isoBlockedWire = rl.NewColor(255, 96, 96, 255)  // footprint-won't-fit preview
	isoBG          = rl.NewColor(26, 28, 34, 255)   // off-screen clear
	isoActiveFloor = withAlpha(editorCyan, 55)      // faint slab marking the active editLevel
)

// setIsoView switches to (on=true) or away from the 3D view, a no-op if already
// there. Routes through toggleIsoView so the flash + RT teardown stay in one place.
func setIsoView(s *State, on bool) {
	if s.isoView != on {
		toggleIsoView(s)
	}
}

// toggleIsoView flips the 3D view on/off, freeing the off-screen target on exit.
func toggleIsoView(s *State) {
	s.isoView = !s.isoView
	if s.isoView {
		s.flash("3D edit — right-drag orbit · shift+right pan · wheel zoom · arrows pan · Q/E snap · I for top-down")
		return
	}
	s.freeIsoRT()
	s.flash("Top-down view")
}

// Close releases the editor's GPU resources: the 3D-view render target plus the
// visualizer/object-browser preview textures (their close is otherwise reached only
// via closeModal, so exiting to title with one open would leak it). Idempotent.
func (s *State) Close() {
	s.freeIsoRT()
	closeAllEditorPreviews()
}

// closeAllEditorPreviews frees every editor preview RT (Foe / Party / Object
// Visualizers). The roster of render closers lives here so a teardown path
// covering ALL previews — Close to title — can't drift from the list. (closeModal
// keeps its own per-modal switch: a normal modal close tears down only that
// modal's preview, not all three.) Idempotent.
func closeAllEditorPreviews() {
	render.CloseFoePreview()
	render.ClosePartyPreview()
	render.CloseObjectPreview()
}

// freeIsoRT releases the off-screen iso target if allocated (idempotent).
func (s *State) freeIsoRT() {
	if s.isoRT.ID != 0 {
		rl.UnloadRenderTexture(s.isoRT)
		s.isoRT = rl.RenderTexture2D{}
		s.isoRTW, s.isoRTH = 0, 0
	}
}

// ensureIsoRT (re)allocates the off-screen target to the grid panel size (resize only).
func (s *State) ensureIsoRT(w, h int32) {
	if s.isoRT.ID != 0 && s.isoRTW == w && s.isoRTH == h {
		return
	}
	s.freeIsoRT()
	s.isoRT = rl.LoadRenderTexture(w, h)
	s.isoRTW, s.isoRTH = w, h
}

// isoLevelSpan returns the lowest+highest elevation level (columns draw relative
// to the lowest).
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
	if minL > maxL { // empty map
		minL, maxL = 0, 0
	}
	return minL, maxL
}

// isoCamera builds the orbit camera in REAL world coordinates (core.TileSize /
// core.LevelStep), so render.DrawArea — the live game renderer — draws the editor
// scene identically. One of four 45° yaws, fixed pitch, fit-to-map distance ×
// zoom. Level span passed in to avoid rescanning.
func (s *State) isoCamera(minL, maxL int) rl.Camera3D {
	W, H := s.area.Width, s.area.Height
	cx := float32(W) * core.TileSize / 2
	cz := float32(H) * core.TileSize / 2
	yLow := core.ElevationWorldY(minL)
	yHigh := core.ElevationWorldY(maxL) + core.LevelStep
	target := rl.NewVector3(cx+s.isoTargetX, (yLow+yHigh)*0.5, cz+s.isoTargetZ)

	yaw := float64(s.isoYaw)
	pitch := float64(s.isoPitch)
	dir := rl.NewVector3(
		float32(math.Cos(pitch)*math.Cos(yaw)),
		float32(math.Sin(pitch)),
		float32(math.Cos(pitch)*math.Sin(yaw)),
	)
	// Fit-to-view distance: size the orbit radius so the map's bounding box fills
	// the panel AT THE CURRENT view angle + panel aspect. The old bounding-sphere
	// fit was angle/aspect-blind, so a non-square map at steep top-down shrank to a
	// thin off-centre strip (looked like a rendering fail). We project the 8 AABB
	// corners onto the camera basis and take the distance that keeps every corner
	// inside the frustum in both axes. dir points target→camera; the frustum opens
	// along -dir, so a corner's depth from the camera is (dist - rel·dir).
	spanX := float32(W) * core.TileSize
	spanZ := float32(H) * core.TileSize
	halfH := (yHigh - yLow) * 0.5
	right := rl.Vector3Normalize(rl.Vector3CrossProduct(dir, rl.NewVector3(0, 1, 0)))
	if rl.Vector3Length(right) < 1e-4 { // near-vertical view: dir ∥ up, pick a stable axis
		right = rl.NewVector3(1, 0, 0)
	}
	camUp := rl.Vector3CrossProduct(right, dir)
	tanV := float32(math.Tan(float64(isoFovy) * math.Pi / 360))
	aspect := float32(1)
	if s.rect.grid.Height > 0 && s.rect.grid.Width > 0 {
		aspect = s.rect.grid.Width / s.rect.grid.Height
	}
	tanH := tanV * aspect
	var dist float32
	for _, sx := range []float32{-0.5, 0.5} {
		for _, sy := range []float32{-1, 1} {
			for _, sz := range []float32{-0.5, 0.5} {
				rel := rl.NewVector3(sx*spanX, sy*halfH, sz*spanZ)
				fwd := rl.Vector3DotProduct(rel, dir) // + = toward camera (nearer)
				lat := float32(math.Abs(float64(rl.Vector3DotProduct(rel, right))))
				vert := float32(math.Abs(float64(rl.Vector3DotProduct(rel, camUp))))
				if d := lat/tanH + fwd; d > dist {
					dist = d
				}
				if d := vert/tanV + fwd; d > dist {
					dist = d
				}
			}
		}
	}
	dist *= 1.06 // small breathing margin so edge tiles aren't flush to the panel
	if dist < core.TileSize {
		dist = core.TileSize
	}
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

// isoColumnBox is the world-space bounding box of column (x,z) in real
// coordinates; shared by the hover highlight and ray-pick so the click target
// matches the rendered geometry. Spans from a bit below the lowest floor up to
// the column's top surface (at least one level tall so flat maps stay pickable).
func (s *State) isoColumnBox(x, z, minL int) (center, size rl.Vector3) {
	yBot := core.ElevationWorldY(minL) - core.LevelStep*0.5
	yTop := core.ElevationWorldY(s.area.ElevationLevelAt(x, z))
	if yTop <= yBot {
		yTop = yBot + core.LevelStep
	}
	center = rl.NewVector3(core.TileCenter(x), (yBot+yTop)*0.5, core.TileCenter(z))
	size = rl.NewVector3(core.TileSize*0.98, yTop-yBot, core.TileSize*0.98)
	return center, size
}

// isoWrongLevel reports whether painting at (x,z) would land on a floor that is
// NOT the visible top surface there — the active edit floor (editLevel) differs
// from the column's surface level. The Elevation layer is exempt: it's how you
// CHANGE a column's level, so gating it would make raising/lowering impossible.
// Drives the red "set the right floor first" hover + the blocked paint in the 3D
// view, where the floor-vs-surface mismatch is otherwise invisible.
func (s *State) isoWrongLevel(x, z int) bool {
	if s.layer == LayerElevation || !s.area.InBounds(x, z) {
		return false
	}
	return s.area.ElevationLevelAt(x, z) != s.editLevel
}

// drawIsoBrushPreview outlines the cells the active tool will paint at the hover,
// on the ACTIVE floor: a multi-tile footprint (gold placeable / red blocked), the
// brush-size block for grid layers, or a single cell otherwise. Turns red when the
// hover sits on a wrong-level column (isoWrongLevel) — painting is blocked there.
func drawIsoBrushPreview(s *State) {
	hx, hz := s.isoHoverX, s.isoHoverZ
	wrong := s.isoWrongLevel(hx, hz)
	if fp := activeBrushFootprint(s); fp != nil {
		col := isoHoverWire
		if wrong || !footprintPlaceable(s, hx, hz, fp) {
			col = isoBlockedWire
		}
		for _, off := range fp {
			s.drawIsoCellBox(hx+off.DX, hz+off.DZ, col)
		}
		return
	}
	col := isoHoverWire
	if wrong {
		col = isoBlockedWire
	}
	half := s.brushSize / 2
	if !isGridLayer(s.layer) || s.brushSize <= 1 {
		s.drawIsoCellBox(hx, hz, col)
		return
	}
	for dz := -half; dz <= half; dz++ {
		for dx := -half; dx <= half; dx++ {
			s.drawIsoCellBox(hx+dx, hz+dz, col)
		}
	}
}

// activeBrushFootprint returns the active Props/Decor brush's multi-tile footprint,
// or nil (single-tile brush / non-scatter layer).
func activeBrushFootprint(s *State) []core.MultiTileOffset {
	c := s.activeBrush().Char
	switch s.layer {
	case LayerProps:
		return core.PropFootprint(c)
	case LayerDecor:
		return core.DecorFootprint(c)
	}
	return nil
}

// drawIsoCellBox outlines cell (x,z) at the active floor's height (where a paint
// lands), skipping off-map cells.
func (s *State) drawIsoCellBox(x, z int, col rl.Color) {
	if !s.area.InBounds(x, z) {
		return
	}
	y := core.ElevationWorldY(s.editLevel)
	center := rl.NewVector3(core.TileCenter(x), y+core.LevelStep*0.5, core.TileCenter(z))
	size := rl.NewVector3(core.TileSize*0.9, core.LevelStep, core.TileSize*0.9)
	// Filled translucent slab so the hovered cell reads as a highlight, with the
	// crisp wire box on top keeping the exact edges legible.
	rl.DrawCubeV(center, size, withAlpha(col, 70))
	rl.DrawCubeWiresV(center, size, col)
}

// isoRayInRect builds the pick ray for a mouse point over `rect` (the off-screen
// grid panel). raylib's GetScreenToWorldRay assumes a full window, so we offset
// the mouse into the panel's local space and use the viewport-aware -Ex variant
// with the panel's dims — matching the projection the RT was rendered with. (A
// prior hand-rolled Unproject produced a Z-flipped ray under raylib-go's matrix
// conventions, so nothing ever picked; see iso_pick_debug_test.go.)
func isoRayInRect(mp rl.Vector2, rect rl.Rectangle, cam rl.Camera3D) rl.Ray {
	local := rl.NewVector2(mp.X-rect.X, mp.Y-rect.Y)
	return rl.GetScreenToWorldRayEx(local, cam, int32(rect.Width), int32(rect.Height))
}

// isoPick returns the tile under the mouse, or (-1,-1) when off-canvas / off-map.
// It first ray-tests the per-column boxes (so the visible surface wins), then
// falls back to the active floor's ground plane — the column boxes are inset with
// seams between them, so a grazing ray over flat/low terrain would otherwise slip
// through and painting would silently miss. minL is the precomputed level floor.
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
	if bestX >= 0 {
		return bestX, bestZ
	}
	// Fallback: intersect the active floor's horizontal plane so a brush lands
	// anywhere over the map footprint, not only on a column the ray happened to hit.
	if ray.Direction.Y != 0 {
		planeY := core.ElevationWorldY(s.editLevel)
		if t := (planeY - ray.Position.Y) / ray.Direction.Y; t > 0 {
			hit := rl.Vector3Add(ray.Position, rl.Vector3Scale(ray.Direction, t))
			tx := int(math.Floor(float64(hit.X / core.TileSize)))
			tz := int(math.Floor(float64(hit.Z / core.TileSize)))
			if s.area.InBounds(tx, tz) {
				return tx, tz
			}
		}
	}
	return -1, -1
}

// ensureIsoPreview returns a preview GameState (chests/doors/crystals/packs made
// live) for the 3D view's entity + foe draw, rebuilt only when the map content
// changes (contentEpoch). NewGameState is too heavy to run per frame; caching by
// epoch runs it once per committed edit.
func (s *State) ensureIsoPreview() *core.GameState {
	if s.isoPreview == nil || s.isoPreviewEpoch != s.contentEpoch {
		g := core.NewGameState(core.CloneArea(s.area))
		s.isoPreview = &g
		s.isoPreviewEpoch = s.contentEpoch
	}
	return s.isoPreview
}

// drawGridIso renders extruded 3D blocks off-screen, blits them, then overlays a readout.
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

	rl.BeginTextureMode(s.isoRT)
	rl.ClearBackground(isoBG)
	rl.BeginMode3D(cam)
	// Render through the REAL game renderer so the editor shows the actual lit,
	// textured world (floors/walls/props/decor), not stand-in cubes. Fixed daytime
	// lighting (stepCount 0). Clear-view cuts the atmospheric fog + gloom (an
	// authoring surface must SEE); freeze pins sway/flicker unless Object Animation
	// is on. Both scoped to this render so F5 playtests keep the real look.
	// defer the resets so a panic mid-draw (e.g. a driver fault in DrawModelEx)
	// can't leak the editor grade into the in-game render path.
	render.SetEditorClearView(true)
	defer render.SetEditorClearView(false)
	render.SetEditorFreezeAnim(!s.animateObjects)
	defer render.SetEditorFreezeAnim(false)
	render.DrawArea(cam, &s.area, frameAssets, 0, nil, s.levelVisibleInIso)
	// Entities + foes so the 3D view matches play: chests, doors, crystals, packs.
	// Built from a preview GameState cached per content edit (see ensureIsoPreview).
	if g := s.ensureIsoPreview(); g != nil {
		render.DrawChests(cam, g, frameAssets)
		render.DrawDoors(cam, g, frameAssets)
		render.DrawCrystals(cam, g, frameAssets)
		render.DrawEnemies(cam, g, frameAssets)
	}
	// Active-floor indicator: a faint translucent slab across the map at the
	// active editLevel, so the user can see WHICH floor a paint will land on (the
	// user's "edit one floor, others visible" cue). Drawn before the hover box.
	ey := core.ElevationWorldY(s.editLevel)
	mcx := float32(s.area.Width) * core.TileSize / 2
	mcz := float32(s.area.Height) * core.TileSize / 2
	rl.DrawCubeV(rl.NewVector3(mcx, ey+0.03, mcz),
		rl.NewVector3(float32(s.area.Width)*core.TileSize, 0.02, float32(s.area.Height)*core.TileSize),
		isoActiveFloor)
	// Brush/footprint preview at the active floor: the exact cells the next paint
	// will hit (gold), or red when a multi-tile footprint won't fit.
	if s.isoHoverX >= 0 {
		drawIsoBrushPreview(s)
	}
	rl.EndMode3D()
	rl.EndTextureMode()

	// RenderTextures are bottom-up; negate source height to blit upright.
	rl.DrawTextureRec(s.isoRT.Texture,
		rl.NewRectangle(0, 0, float32(w), -float32(h)),
		rl.NewVector2(grid.X, grid.Y), rl.White)

	drawIsoReadout(s, font, grid)
}

// drawIsoReadout shows the hovered column's coords + signed level, plus a hint.
func drawIsoReadout(s *State, font rl.Font, grid rl.Rectangle) {
	hint := "3D · right-drag orbit · shift+right pan · wheel zoom · arrows pan · L tool · R-click menu · Q/E snap · PgUp/Dn or Levels = floor · I top-down"
	rl.DrawTextEx(font, hint, rl.NewVector2(grid.X+8, grid.Y+8), editorFontHint, 1, rl.NewColor(210, 214, 222, 200))
	// Active floor + brush: the floor a paint lands on (matches the slab) and what
	// the active tool will stamp — so editing in 3D isn't blind to either.
	active := fmt.Sprintf("floor %s  ·  %s", signedLevelLabel(s.editLevel), s.activeBrush().Name)
	rl.DrawTextEx(font, active, rl.NewVector2(grid.X+8, grid.Y+8+editorFontHint+4), editorFontHint, 1, rl.NewColor(150, 210, 255, 235))
	if s.isoHoverX >= 0 {
		lvl := s.area.ElevationLevelAt(s.isoHoverX, s.isoHoverZ) - core.ElevationBaseline
		txt := fmt.Sprintf("%s  surface %+d", core.TileCoord(s.isoHoverX, s.isoHoverZ), lvl)
		rl.DrawTextEx(font, txt, rl.NewVector2(grid.X+8, grid.Y+8+2*(editorFontHint+4)), editorFontHint, 1, withAlpha(editorGold, 235))
	}
}

// isoPanTarget slides the 3D orbit target by a screen-space delta (dx = right,
// dy = down), the shared pan rule for Shift+right-drag and arrow-key panning so
// keyboard and mouse never disagree on direction.
func (s *State) isoPanTarget(dx, dy float32) {
	yaw := float64(s.isoYaw)
	rx, rz := float32(math.Cos(yaw+math.Pi/2)), float32(math.Sin(yaw+math.Pi/2))
	fx, fz := float32(math.Cos(yaw)), float32(math.Sin(yaw))
	k := isoPanSpeed / s.isoZoom
	s.isoTargetX -= (rx*dx + fx*dy) * k
	s.isoTargetZ -= (rz*dx + fz*dy) * k
}

// updateIsoCanvas drives the 3D editing surface: the SAME tool/brush/layer/
// editLevel model as the top-down canvas, fed the ray-picked tile — so every
// tool paints in 3D. Right-drag orbits (free tumble), Shift+right-drag pans, the
// wheel zooms, Q/E snap the yaw by 90°, left edits, a right-click (no drag)
// opens the context menu.
func updateIsoCanvas(s *State, mp rl.Vector2) {
	if rl.IsKeyPressed(rl.KeyE) {
		s.isoYaw += math.Pi / 2
	}
	if rl.IsKeyPressed(rl.KeyQ) {
		s.isoYaw -= math.Pi / 2
	}

	minL, maxL := isoLevelSpan(s)
	cam := s.isoCamera(minL, maxL)
	s.isoHoverX, s.isoHoverZ = s.isoPick(cam, mp, minL)
	// Mirror the pick onto hoverX/Z so the shared rect/line/select + finishDrag
	// code (which reads the hover tile) resolves the right cell in 3D too.
	s.hoverX, s.hoverZ = s.isoHoverX, s.isoHoverZ
	hx, hz := s.isoHoverX, s.isoHoverZ
	shift := rl.IsKeyDown(rl.KeyLeftShift) || rl.IsKeyDown(rl.KeyRightShift)
	ctrl := rl.IsKeyDown(rl.KeyLeftControl) || rl.IsKeyDown(rl.KeyRightControl)

	// Camera: right-drag orbits (tumble); Shift+right-drag pans the target. Before
	// the on-canvas gate so a drag leaving the panel keeps tracking. The mousewheel
	// BUTTON is intentionally unbound; a right-click without a drag opens the menu.
	s.updateRightDrag(mp)
	if rl.IsMouseButtonDown(rl.MouseRightButton) {
		d := rl.GetMouseDelta()
		if shift {
			s.isoPanTarget(d.X, d.Y) // Shift+right-drag pans
		} else {
			// Right-drag orbits freely — yaw with horizontal, pitch with vertical.
			s.isoYaw += d.X * isoOrbitRate
			s.isoPitch = core.Clamp(s.isoPitch-d.Y*isoOrbitRate, isoMinPitch, isoMaxPitch)
		}
	}
	// A paint/drag commits on release wherever the cursor ends, and an in-progress
	// stroke keeps painting across picked cells — both ungated by the panel rect.
	if rl.IsMouseButtonReleased(rl.MouseLeftButton) {
		finishDrag(s)
	}
	if rl.IsMouseButtonDown(rl.MouseLeftButton) && s.drag != dragNone && hx >= 0 && !s.isoWrongLevel(hx, hz) {
		continueDrag(s, hx, hz) // skip wrong-level cells mid-stroke (see isoWrongLevel)
	}

	if !pointIn(mp, s.rect.grid) {
		return // camera + release handled above; no new edit/zoom off-canvas
	}

	if wheel := rl.GetMouseWheelMove(); wheel != 0 {
		// Guard a zero seed: multiplicative zoom stuck at 0 never recovers.
		if s.isoZoom <= 0 {
			s.isoZoom = 1
		}
		s.isoZoom *= 1 + canvasZoomWheelRate*wheel
		s.isoZoom = core.Clamp(s.isoZoom, isoMinZoom, isoMaxZoom)
	}

	if hx < 0 {
		return
	}
	// Left press starts the active tool's interaction (startDrag dispatches paint /
	// rect / flood / ramp / entity exactly as top-down). Right opens the context
	// menu, or clears a ramp in ramp mode.
	if rl.IsMouseButtonPressed(rl.MouseLeftButton) && !s.isoWrongLevel(hx, hz) {
		startDrag(s, hx, hz, ctrl, shift) // block starting a paint/place on a wrong-level column
	}
	if rl.IsMouseButtonReleased(rl.MouseRightButton) && !s.rightDragMoved {
		if s.rampMode {
			isoClearRamp(s, hx, hz)
			return
		}
		openContextMenu(s, mp.X, mp.Y, hx, hz)
	}
}

// levelVisibleInIso reports whether floor L's props/decor should render in the 3D
// view — honoring the Levels-panel eye toggles so the user can isolate a floor
// while editing. Out-of-range levels are visible (never hide unexpected content).
func (s *State) levelVisibleInIso(L int) bool {
	if L < 0 || L >= len(s.levelHidden) {
		return true
	}
	return !s.levelHidden[L]
}

// isoClearRamp removes a ramp at (x,z) (floor → auto). No-op when there's none.
func isoClearRamp(s *State, x, z int) {
	if _, ok := s.area.RampAt(x, z); !ok {
		return
	}
	pushUndo(s)
	setLayerCell(&s.area.Floor, x, z, core.FloorAuto)
	s.dirty = true
}

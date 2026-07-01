package render

import (
	"math"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Foe Visualizer preview: DrawFoePreview shows the foe as a small combat diorama.
// 3D pass → cached off-screen texture → blit (raylib has no sub-rect 3D viewport).

// previewRT is a cached off-screen render target sized to a preview panel. Foe and
// Party visualizers each keep an instance; (re)alloc + teardown live here once.
type previewRT struct {
	rt   rl.RenderTexture2D
	w, h int32
	init bool
}

// ensure lazily (re)creates the texture on miss/resize, unloading the old handle
// first. false on alloc failure (caller skips drawing rather than bind an invalid target).
func (p *previewRT) ensure(w, h int32) bool {
	if p.init && p.w == w && p.h == h {
		return true
	}
	if p.init {
		rl.UnloadRenderTexture(p.rt)
		p.init = false
	}
	rt := rl.LoadRenderTexture(w, h)
	if rt.ID == 0 {
		return false
	}
	p.rt = rt
	rl.SetTextureFilter(p.rt.Texture, rl.FilterBilinear)
	p.w, p.h = w, h
	p.init = true
	return true
}

// ensureStable is ensure for FULL-SCREEN capture targets (menu fade + retro
// filter), whose requested size tracks the window and so changes every frame
// during a resize drag. It reuses the existing target unchanged while the window
// is mid-resize, deferring the Unload+Load realloc until the size settles:
// reallocating a GPU RenderTexture while the OS is still streaming resize events
// is the intermittent DrawModelEx crash. During the drag the stale-sized target is
// simply blitted to fit (a transient, drag-only cosmetic imperfection). Once there
// is no target yet, or the size has held for a frame, it falls through to ensure.
func (p *previewRT) ensureStable(w, h int32) bool {
	if p.init && windowResizing {
		return true
	}
	return p.ensure(w, h)
}

// close unloads the cached texture. Idempotent.
func (p *previewRT) close() {
	if !p.init {
		return
	}
	rl.UnloadRenderTexture(p.rt)
	p.rt = rl.RenderTexture2D{}
	p.w, p.h = 0, 0
	p.init = false
}

// blit draws the cached texture into rect, negating source height to undo the
// RenderTexture y-flip. The one place that flip detail lives.
func (p *previewRT) blit(rect rl.Rectangle) {
	p.blitTinted(rect, rl.White)
}

// blitTinted is blit with a caller-supplied tint (menu-fade + retro blits route here).
func (p *previewRT) blitTinted(rect rl.Rectangle, tint rl.Color) {
	rl.DrawTextureRec(p.rt.Texture,
		rl.NewRectangle(0, 0, float32(p.w), -float32(p.h)),
		rl.NewVector2(rect.X, rect.Y),
		tint)
}

// visualizerGroundSize: diorama floor extent + grid slice count (Foe + Party).
const visualizerGroundSize = float32(14)

// beginVisualizerScene opens the off-screen 3D pass: bind, clear, enter 3D, lay a
// groundSize×groundSize floor (and, when drawGrid, a 1-unit grid). Pair with
// EndMode3D/EndTextureMode. groundSize/drawGrid are parameterized so the object
// (prop) preview can reuse this with a tighter floor and no grid.
func (p *previewRT) beginVisualizerScene(cam rl.Camera3D, groundSize float32, drawGrid bool) {
	rl.BeginTextureMode(p.rt)
	rl.ClearBackground(foePreviewBG)
	rl.BeginMode3D(cam)
	rl.DrawPlane(rl.NewVector3(0, 0, 0), rl.NewVector2(groundSize, groundSize), foePreviewGround)
	if drawGrid {
		rl.DrawGrid(int32(groundSize), 1)
	}
}

var foePreviewRT previewRT

// foeAnchor is the preview's formation-center anchor (battleFormationCenterY, as
// battle billboards use) so the preview matches an encounter.
var foeAnchor = rl.NewVector3(0, battleFormationCenterY, 0)

// foePreviewCamera is the diorama camera, derived from the DEFAULT battle tuning so
// the preview reads at the SAME downward tilt + FOV combat does (back along +Z, the
// look pitched down by |CamPitch|). Forward points into −Z, so a positive depthOffset
// pushes the sprite away, matching the battle "back into the arena" read.
func foePreviewCamera() rl.Camera3D {
	tune := core.DefaultBattleTuning()
	pitch := float64(-tune.CamPitch) // battle tilt magnitude (looking down)
	const dist = float32(4.4)
	target := rl.NewVector3(0, 0.85, 0)
	return rl.Camera3D{
		Position:   rl.NewVector3(0, target.Y+dist*float32(math.Sin(pitch)), dist*float32(math.Cos(pitch))),
		Target:     target,
		Up:         worldUp,
		Fovy:       tune.CamFOV,
		Projection: rl.CameraPerspective,
	}
}

// FoePreviewZoomMin / FoePreviewZoomMax bound the preview zoom (1.0 = default
// framing, larger = closer).
const (
	FoePreviewZoomMin = 0.5
	FoePreviewZoomMax = 4.0
)

// zoomedPreviewCamera dollies the diorama camera toward its target (zoom 1 =
// default, >1 closer). Shared by both previews so they zoom identically.
func zoomedPreviewCamera(zoom float32) rl.Camera3D {
	cam := foePreviewCamera()
	if zoom <= 0 {
		zoom = 1
	}
	dir := rl.Vector3Subtract(cam.Position, cam.Target)
	cam.Position = rl.Vector3Add(cam.Target, rl.Vector3Scale(dir, 1.0/zoom))
	return cam
}

// LiveFoeOverride returns the currently-loaded visual for kind as an override
// (defaults + any visuals.json overlay). The editor seeds its working copy from
// this. ok=false if the kind has no visual.
func LiveFoeOverride(assets Resources, kind core.EnemyKind) (core.EnemyVisualOverride, bool) {
	v, ok := enemyVisualFor(assets, kind)
	if !ok {
		return core.EnemyVisualOverride{}, false
	}
	return enemyVisualOverride(v), true
}

// SetLiveFoeOverride applies ov onto the in-memory visual for kind so a save shows
// without a reload. enemyVisuals is shared by reference through by-value Resources,
// so the write reaches the editor + LiveFoeOverride. No-op if out of range; caller
// persists to visuals.json separately.
func SetLiveFoeOverride(assets Resources, kind core.EnemyKind, ov core.EnemyVisualOverride) {
	base, ok := visualAt(assets.enemyVisuals, int(kind))
	if !ok {
		return
	}
	v := applyEnemyVisualOverride(base, ov)
	// Re-bake FX into the live display texture (positional/tint apply live at draw
	// time; only texture-baked FX need the re-derive).
	v.texture = displayTextureForSlug(core.EnemySlug(kind), pristineOrTexture(v), ov)
	assets.enemyVisuals[kind] = v
}

// DrawFoePreview renders kind's billboard (with override ov applied) into rect:
// ground, shadow, sprite, chevron, using the same math drawBattlePack does so the
// preview is faithful. Safe per-frame; the texture is cached, reallocated on resize.
func DrawFoePreview(rect rl.Rectangle, assets Resources, kind core.EnemyKind, ov core.EnemyVisualOverride, zoom float32, showGizmos bool, previewTex rl.Texture2D) {
	base, ok := enemyVisualFor(assets, kind)
	if !ok {
		return
	}
	v := applyEnemyVisualOverride(base, ov)
	// Asset-tab live preview overrides the texture; zero ID falls back to the real one.
	if previewTex.ID != 0 {
		v.texture = previewTex
	}
	// Foot-anchor the sprite + cursor exactly like drawBattlePack (feet on FoeFloorY,
	// not floating at the formation center), so the preview matches battle grounding.
	// Gizmos stay at foeAnchor — the battle VFX origin (formation center).
	spriteAnchor := rl.NewVector3(foeAnchor.X, core.DefaultBattleTuning().FoeFloorY+v.size.Y/2, foeAnchor.Z)
	drawVisualizerPreview(rect, &foePreviewRT, v, zoom, spriteAnchor, showGizmos, drawTargetChevron)
}

// drawVisualizerPreview renders the resolved visual v into rect through rt with the
// shared diorama sequence (camera, ground, shadow, cursor, sprite, gizmos, blit).
// anchor positions the sprite + cursor; drawMarker paints the target cursor (the
// party/foe wrappers differ only in lookup, RT, anchor, and friendly-vs-hostile cursor).
func drawVisualizerPreview(rect rl.Rectangle, rt *previewRT, v enemyVisual, zoom float32, anchor rl.Vector3, showGizmos bool, drawMarker func(cam rl.Camera3D, at rl.Vector3, scale float32)) {
	w, h := int32(rect.Width), int32(rect.Height)
	if w <= 0 || h <= 0 {
		return
	}
	if !rt.ensure(w, h) {
		return
	}
	cam := zoomedPreviewCamera(zoom)
	rt.beginVisualizerScene(cam, visualizerGroundSize, true)
	place := resolveBillboardPlacement(cam, anchor, &v)
	if v.shadowRadius > 0 {
		drawGroundShadow(place.shadowX, place.shadowZ, v.shadowRadius)
	}
	drawMarker(cam, place.chevron, v.effectiveMarkerScale())
	drawTextureBillboard(cam, v.texture, place.sprite, v.size, v.resolveTint())
	// Authoring gizmos (Layout tab only; Asset tab hides them). drawPreviewGizmos
	// anchors at foeAnchor — the battle VFX origin — not place.base.
	if showGizmos {
		drawPreviewGizmos(cam, v)
	}
	rl.EndMode3D()
	rl.EndTextureMode()
	rt.blit(rect)
}

// drawPreviewGizmos paints the three authoring anchor gizmos (particle = orange,
// hit-glyph = cyan, damage-number = gold), each nudged + sized by the same
// override fields and helpers the live VFX path uses, anchored at foeAnchor, so
// the diorama reads like an encounter.
func drawPreviewGizmos(cam rl.Camera3D, v enemyVisual) {
	pAnchor := cameraRelativeOffset(cam, foeAnchor, v.particleXOffset, v.particleYOffset, v.particleZOffset)
	drawAnchorGizmo(pAnchor, 0.16*v.effectiveParticleScale(), gizmoParticleColor)
	gAnchor := cameraRelativeOffset(cam, foeAnchor, v.glyphXOffset, v.glyphYOffset, 0)
	gAnchor.Y += hitGlyphRise
	drawAnchorGizmo(gAnchor, 0.13*v.effectiveGlyphScale(), gizmoGlyphColor)
	nAnchor := cameraRelativeOffset(cam, foeAnchor, v.popupXOffset, v.popupYOffset, 0)
	nAnchor.Y += popupWorldRise
	drawAnchorGizmo(nAnchor, 0.10, gizmoNumberColor)
}

// drawAnchorGizmo paints a wireframe sphere at p, depth-independent so it's never
// hidden behind the opaque billboard. Radius tracks the effect's size slider.
func drawAnchorGizmo(p rl.Vector3, radius float32, col rl.Color) {
	if radius <= 0 {
		return
	}
	drawDepthIndependent(func() { rl.DrawSphereWires(p, radius, 6, 6, col) })
}

// CloseFoePreview unloads the cached texture when the Foe Visualizer closes. Idempotent.
func CloseFoePreview() {
	foePreviewRT.close()
}

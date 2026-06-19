package render

import (
	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Foe Visualizer preview support. The editor's Foe Visualizer modal
// (editor/foeview.go) calls DrawFoePreview every frame to show the foe it's
// tuning as a small combat-like diorama, redrawn live as the author drags the
// placement / shadow / cursor / tint sliders. The 3D pass is rendered to a
// cached off-screen texture sized to the panel and then blitted into it, so the
// world geometry lands inside the panel rect instead of the full window (raylib
// has no public sub-rect 3D viewport, so a RenderTexture is the clean route).

// previewRT is a cached off-screen render target sized to a visualizer
// preview panel. Both the Foe and Party visualizers keep their own instance
// (foePreviewRT / partyPreviewRT) — the diorama 3D pass renders into it and
// is then blitted into the panel rect. The (re)allocation + teardown logic is
// identical for both, so it lives on this struct instead of being duplicated
// per file.
type previewRT struct {
	rt   rl.RenderTexture2D
	w, h int32
	init bool
}

// ensure lazily (re)creates the cached off-screen texture when it's missing or
// the requested size changed. The old texture is unloaded before a resize so
// the GPU handle doesn't leak across window-size changes. Returns false when
// the allocation fails (e.g. GPU OOM) so the caller skips drawing and retries
// next frame, rather than driving BeginTextureMode/BeginMode3D against an
// invalid render target.
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

// close unloads the cached off-screen texture. Idempotent — safe to call when
// nothing is allocated.
func (p *previewRT) close() {
	if !p.init {
		return
	}
	rl.UnloadRenderTexture(p.rt)
	p.rt = rl.RenderTexture2D{}
	p.w, p.h = 0, 0
	p.init = false
}

var foePreviewRT previewRT

// foeAnchor is the preview's formation-center anchor — the same vertical center
// (battleFormationCenterY) battle billboards use, so a foe that sits right in
// the preview sits right in an actual encounter. The contact shadow lands on
// the ground plane at y≈0 and the billboard centers here, exactly as
// drawBattlePack arranges them.
var foeAnchor = rl.NewVector3(0, battleFormationCenterY, 0)

// foePreviewBG / foePreviewGround tint the diorama: a dark neutral void behind
// the foe and a muted floor so the contact shadow reads.
var (
	foePreviewBG     = rl.NewColor(26, 28, 34, 255)
	foePreviewGround = rl.NewColor(54, 58, 66, 255)
)

// Authoring-gizmo tints shared by both visualizer previews (foe + party): the
// orange particle-burst anchor, the cyan hit-glyph anchor, and the gold
// floating-damage-number anchor. Hoisted here so the two preview draws read the
// same three colors instead of re-typing the NewColor literals per file.
var (
	gizmoParticleColor = rl.NewColor(255, 168, 86, 210)
	gizmoGlyphColor    = rl.NewColor(176, 226, 255, 220)
	gizmoNumberColor   = rl.NewColor(255, 232, 120, 220)
)

// foePreviewCamera is the fixed three-quarter camera the diorama is viewed
// through: pulled back along +Z and slightly up, looking at the foe's mid
// height. Forward points into −Z, so a positive depthOffset pushes the sprite
// AWAY from the viewer — matching the battle "back into the arena" read.
func foePreviewCamera() rl.Camera3D {
	return rl.Camera3D{
		Position:   rl.NewVector3(0, 1.45, 4.4),
		Target:     rl.NewVector3(0, 0.85, 0),
		Up:         rl.NewVector3(0, 1, 0),
		Fovy:       46,
		Projection: rl.CameraPerspective,
	}
}

// FoePreviewZoomMin / FoePreviewZoomMax bound the visualizer preview zoom (the
// editor clamps the wheel-driven factor to this range). 1.0 = the default
// framing; larger = closer in.
const (
	FoePreviewZoomMin = 0.5
	FoePreviewZoomMax = 4.0
)

// zoomedPreviewCamera dollies the fixed diorama camera toward its target along
// the view ray: zoom 1 = the default framing, >1 moves the eye closer (the
// sprite fills more of the panel), <1 pulls back. Both visualizer previews share
// it so foe and friend zoom identically.
func zoomedPreviewCamera(zoom float32) rl.Camera3D {
	cam := foePreviewCamera()
	if zoom <= 0 {
		zoom = 1
	}
	dir := rl.Vector3Subtract(cam.Position, cam.Target)
	cam.Position = rl.Vector3Add(cam.Target, rl.Vector3Scale(dir, 1.0/zoom))
	return cam
}

// LiveFoeOverride returns the currently-LOADED visual for kind as an override
// — code defaults with any maps/sprites/visuals.json already overlaid at load
// time. The editor seeds its working copy from this so opening a foe shows
// exactly what the game draws right now. ok=false if the kind has no visual.
func LiveFoeOverride(assets Resources, kind core.EnemyKind) (core.EnemyVisualOverride, bool) {
	v, ok := enemyVisualFor(assets, kind)
	if !ok {
		return core.EnemyVisualOverride{}, false
	}
	return enemyVisualOverride(v), true
}

// SetLiveFoeOverride applies ov onto the in-memory visual for kind, so the
// editor's just-saved tuning takes effect immediately — without a reload. The
// enemyVisuals slice is shared by reference through the (by-value) Resources, so
// mutating an entry here updates the same slice the editor's render loop and
// LiveFoeOverride read; otherwise the editor would re-seed from the stale loaded
// value on the next foe-cycle and the save would look reverted. The texture is
// preserved (applyEnemyVisualOverride keeps it). No-op if the kind is out of
// range. Persisted separately to visuals.json by the caller; this is only the
// live in-memory mirror.
func SetLiveFoeOverride(assets Resources, kind core.EnemyKind, ov core.EnemyVisualOverride) {
	base, ok := visualAt(assets.enemyVisuals, int(kind))
	if !ok {
		return
	}
	assets.enemyVisuals[kind] = applyEnemyVisualOverride(base, ov)
}

// DrawFoePreview renders kind's billboard — with the in-progress override ov
// applied on top of its code-default texture — into rect: ground, contact
// shadow, the upright sprite, and the target chevron, arranged with the exact
// same math drawBattlePack uses so the preview is faithful. Safe to call every
// frame; the off-screen texture is cached and only reallocated when the panel
// size changes.
func DrawFoePreview(rect rl.Rectangle, assets Resources, kind core.EnemyKind, ov core.EnemyVisualOverride, zoom float32, showGizmos bool, previewTex rl.Texture2D) {
	base, ok := enemyVisualFor(assets, kind)
	if !ok {
		return
	}
	v := applyEnemyVisualOverride(base, ov)
	// Asset-tab live preview overrides the displayed texture (non-destructive
	// filter result); zero ID falls back to the kind's real texture.
	if previewTex.ID != 0 {
		v.texture = previewTex
	}
	w, h := int32(rect.Width), int32(rect.Height)
	if w <= 0 || h <= 0 {
		return
	}
	if !foePreviewRT.ensure(w, h) {
		return
	}
	cam := zoomedPreviewCamera(zoom)

	rl.BeginTextureMode(foePreviewRT.rt)
	rl.ClearBackground(foePreviewBG)
	rl.BeginMode3D(cam)
	rl.DrawPlane(rl.NewVector3(0, 0, 0), rl.NewVector2(14, 14), foePreviewGround)
	rl.DrawGrid(14, 1)

	// Same per-kind placement as the battle roster, via the shared helper so
	// the preview stays faithful to drawBattlePack's depth/shadow/chevron/
	// yOffset ordering by construction (not by a hand-synced comment).
	place := resolveBillboardPlacement(cam, foeAnchor, &v)
	if v.shadowRadius > 0 {
		drawGroundShadow(place.shadowX, place.shadowZ, v.shadowRadius)
	}
	drawTargetChevron(cam, place.chevron, v.effectiveMarkerScale())
	drawTextureBillboard(cam, v.texture, place.sprite, v.size, v.resolveTint())

	// Authoring gizmos: small wireframe spheres at the combat anchor of this
	// kind's particle burst (orange) and hit glyph (cyan), so the glyph/particle
	// anchor + size sliders have live feedback even though the real glyph/
	// particle systems don't run in this static diorama. Anchored at foeAnchor
	// (NOT place.base) to match the battle VFX origin — resolveAnchor uses the
	// pre-depthOffset formation center — nudged + sized by the same fields and
	// helpers (cameraRelativeOffset, hitGlyphRise, effective*Scale) the live path
	// uses, so what reads here reads in an encounter.
	// Gizmos are Layout-tab authoring aids; the Asset tab hides them so the bare
	// sprite (and its baked tint / pixelation) reads clean.
	if showGizmos {
		pAnchor := cameraRelativeOffset(cam, foeAnchor, v.particleXOffset, v.particleYOffset, v.particleZOffset)
		drawAnchorGizmo(pAnchor, 0.16*v.effectiveParticleScale(), gizmoParticleColor)
		gAnchor := cameraRelativeOffset(cam, foeAnchor, v.glyphXOffset, v.glyphYOffset, 0)
		gAnchor.Y += hitGlyphRise
		drawAnchorGizmo(gAnchor, 0.13*v.effectiveGlyphScale(), gizmoGlyphColor)
		// Gold gizmo = floating damage-NUMBER spawn (Num X/Y). popupWorldRise
		// matches the baked rise drawFloatingDamage adds, so the dot sits where
		// the number does.
		nAnchor := cameraRelativeOffset(cam, foeAnchor, v.popupXOffset, v.popupYOffset, 0)
		nAnchor.Y += popupWorldRise
		drawAnchorGizmo(nAnchor, 0.10, gizmoNumberColor)
	}

	rl.EndMode3D()
	rl.EndTextureMode()

	// Blit the off-screen render into the panel. RenderTextures are stored
	// flipped, so the source height is negated to draw it right-side up.
	rl.DrawTextureRec(foePreviewRT.rt.Texture,
		rl.NewRectangle(0, 0, float32(w), -float32(h)),
		rl.NewVector2(rect.X, rect.Y),
		rl.White)
}

// drawAnchorGizmo paints a small wireframe sphere at p, drawn depth-independent
// (like the selector pyramid in drawMarkerOnTop) so it's never hidden behind the
// opaque billboard. Foe Visualizer authoring aid only — radius tracks the
// effect's size slider so the gizmo doubles as a rough size readout. State is
// restored so the blit and any later draw are unaffected.
func drawAnchorGizmo(p rl.Vector3, radius float32, col rl.Color) {
	if radius <= 0 {
		return
	}
	rl.DrawRenderBatchActive() // flush prior depth-tested geometry
	rl.DisableDepthTest()
	rl.DisableDepthMask()
	rl.DrawSphereWires(p, radius, 6, 6, col)
	rl.DrawRenderBatchActive() // flush the gizmo with depth off
	rl.EnableDepthMask()
	rl.EnableDepthTest()
}

// CloseFoePreview unloads the cached off-screen texture. The editor calls this
// when the Foe Visualizer modal closes so the GPU handle isn't held for the
// rest of the session. Idempotent — safe to call when nothing is allocated.
func CloseFoePreview() {
	foePreviewRT.close()
}

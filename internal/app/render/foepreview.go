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

var (
	foePreviewRT     rl.RenderTexture2D
	foePreviewRTW    int32
	foePreviewRTH    int32
	foePreviewRTInit bool
)

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
	foePreviewGrid   = rl.NewColor(72, 78, 88, 255)
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

// DrawFoePreview renders kind's billboard — with the in-progress override ov
// applied on top of its code-default texture — into rect: ground, contact
// shadow, the upright sprite, and the target chevron, arranged with the exact
// same math drawBattlePack uses so the preview is faithful. Safe to call every
// frame; the off-screen texture is cached and only reallocated when the panel
// size changes.
func DrawFoePreview(rect rl.Rectangle, assets Resources, kind core.EnemyKind, ov core.EnemyVisualOverride) {
	base, ok := enemyVisualFor(assets, kind)
	if !ok {
		return
	}
	v := applyEnemyVisualOverride(base, ov)
	w, h := int32(rect.Width), int32(rect.Height)
	if w <= 0 || h <= 0 {
		return
	}
	if !ensureFoePreviewRT(w, h) {
		return
	}
	cam := foePreviewCamera()

	rl.BeginTextureMode(foePreviewRT)
	rl.ClearBackground(foePreviewBG)
	rl.BeginMode3D(cam)
	rl.DrawPlane(rl.NewVector3(0, 0, 0), rl.NewVector2(14, 14), foePreviewGround)
	rl.DrawGrid(14, 1)
	_ = foePreviewGrid

	// Same per-kind placement as the battle roster, via the shared helper so
	// the preview stays faithful to drawBattlePack's depth/shadow/chevron/
	// yOffset ordering by construction (not by a hand-synced comment).
	place := resolveBillboardPlacement(cam, foeAnchor, v)
	if v.shadowRadius > 0 {
		drawGroundShadow(place.shadowX, place.shadowZ, v.shadowRadius)
	}
	drawTargetChevron(cam, place.chevron)
	drawTextureBillboard(cam, v.texture, place.sprite, v.size, v.resolveTint())

	rl.EndMode3D()
	rl.EndTextureMode()

	// Blit the off-screen render into the panel. RenderTextures are stored
	// flipped, so the source height is negated to draw it right-side up.
	rl.DrawTextureRec(foePreviewRT.Texture,
		rl.NewRectangle(0, 0, float32(w), -float32(h)),
		rl.NewVector2(rect.X, rect.Y),
		rl.White)
}

// ensureFoePreviewRT lazily (re)creates the cached off-screen texture when it's
// missing or the requested size changed. The old texture is unloaded before a
// resize so the GPU handle doesn't leak across window-size changes.
func ensureFoePreviewRT(w, h int32) bool {
	if foePreviewRTInit && foePreviewRTW == w && foePreviewRTH == h {
		return true
	}
	if foePreviewRTInit {
		rl.UnloadRenderTexture(foePreviewRT)
		foePreviewRTInit = false
	}
	rt := rl.LoadRenderTexture(w, h)
	if rt.ID == 0 {
		// Allocation failed (e.g. GPU OOM). Stay uninitialized so the caller
		// skips drawing and we retry next frame, rather than driving
		// BeginTextureMode/BeginMode3D against an invalid render target.
		return false
	}
	foePreviewRT = rt
	rl.SetTextureFilter(foePreviewRT.Texture, rl.FilterBilinear)
	foePreviewRTW, foePreviewRTH = w, h
	foePreviewRTInit = true
	return true
}

// CloseFoePreview unloads the cached off-screen texture. The editor calls this
// when the Foe Visualizer modal closes so the GPU handle isn't held for the
// rest of the session. Idempotent — safe to call when nothing is allocated.
func CloseFoePreview() {
	if !foePreviewRTInit {
		return
	}
	rl.UnloadRenderTexture(foePreviewRT)
	foePreviewRT = rl.RenderTexture2D{}
	foePreviewRTW, foePreviewRTH = 0, 0
	foePreviewRTInit = false
}

package render

import (
	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Party Visualizer preview support — the party-side twin of foepreview.go. The
// editor's Party Visualizer modal (editor/partyview.go) calls DrawPartyPreview
// every frame to show the class it's tuning as a small combat-like diorama,
// redrawn live as the author drags the placement / shadow / cursor / tint
// sliders. It shares the foe preview's diorama camera, anchor, void/floor tints,
// and authoring-gizmo helper (foePreviewCamera / foeAnchor / foePreviewBG /
// foePreviewGround / drawAnchorGizmo) since both render the exact same billboard
// pipeline; only the lookup (party class vs enemy kind) and the cached off-screen
// render texture differ.

// partyPreviewRT is the party visualizer's cached off-screen render target —
// the party-side twin of foePreviewRT. See foepreview.go for the previewRT
// struct and its ensure/close methods.
var partyPreviewRT previewRT

// LivePartyOverride returns the currently-LOADED visual for class as an override
// — code defaults with any maps/sprites/partyvisuals.json already overlaid at
// load time. The editor seeds its working copy from this so opening a class
// shows exactly what the game draws right now. ok=false if the class has no
// visual (defensive — every class is populated at load).
func LivePartyOverride(assets Resources, class core.PartyClass) (core.PartyVisualOverride, bool) {
	v, ok := partyVisualFor(assets, class)
	if !ok {
		return core.PartyVisualOverride{}, false
	}
	return enemyVisualOverride(v), true
}

// SetLivePartyOverride applies ov onto the in-memory visual for class so the
// editor's just-saved tuning takes effect immediately without a reload. The
// partyVisuals slice is shared by reference through the (by-value) Resources, so
// mutating an entry here updates the same slice the editor's render loop and
// LivePartyOverride read — otherwise a class-cycle would re-seed from the stale
// loaded value and the save would look reverted. The texture is preserved.
// No-op if the class is out of range. Persisted separately to partyvisuals.json
// by the caller; this is only the live in-memory mirror.
func SetLivePartyOverride(assets Resources, class core.PartyClass, ov core.PartyVisualOverride) {
	base, ok := visualAt(assets.partyVisuals, int(class))
	if !ok {
		return
	}
	v := applyEnemyVisualOverride(base, ov)
	// Re-bake the non-destructive FX onto the pristine base into the live display
	// texture (same as the foe side) so a Save applies in-session, not on restart.
	v.texture = displayTextureForSlug(core.PartyClassSlug(class), pristineOrTexture(v), ov)
	assets.partyVisuals[class] = v
}

// DrawPartyPreview renders class's billboard — with the in-progress override ov
// applied on top of its code-default texture — into rect: ground, contact
// shadow, the upright sprite, and the target chevron, arranged with the exact
// same math drawBattlePack / DrawPartySprites use so the preview is faithful.
// Safe to call every frame; the off-screen texture is cached and only
// reallocated when the panel size changes.
func DrawPartyPreview(rect rl.Rectangle, assets Resources, class core.PartyClass, ov core.PartyVisualOverride, zoom float32, showGizmos bool, previewTex rl.Texture2D) {
	base, ok := partyVisualFor(assets, class)
	if !ok {
		return
	}
	v := applyEnemyVisualOverride(base, ov)
	// Asset-tab live preview overrides the displayed texture (non-destructive
	// filter result); zero ID falls back to the class's real texture.
	if previewTex.ID != 0 {
		v.texture = previewTex
	}
	w, h := int32(rect.Width), int32(rect.Height)
	if w <= 0 || h <= 0 {
		return
	}
	if !partyPreviewRT.ensure(w, h) {
		return
	}
	cam := zoomedPreviewCamera(zoom)

	partyPreviewRT.beginVisualizerScene(cam)

	place := resolveBillboardPlacement(cam, foeAnchor, &v)
	if v.shadowRadius > 0 {
		drawGroundShadow(place.shadowX, place.shadowZ, v.shadowRadius)
	}
	drawFriendlyTargetMarker(cam, place.chevron, v.effectiveMarkerScale())
	drawTextureBillboard(cam, v.texture, place.sprite, v.size, v.resolveTint())

	// Authoring gizmos for the glyph/particle anchor sliders, identical to the
	// foe preview (orange = particle burst origin, cyan = hit-glyph anchor) so
	// enemy→party hit FX can be aligned to a member's sprite the same way.
	// Gizmos are Layout-tab authoring aids; the Asset tab hides them so the bare
	// sprite (and its baked tint / pixelation) reads clean.
	if showGizmos {
		drawPreviewGizmos(cam, v)
	}

	rl.EndMode3D()
	rl.EndTextureMode()

	partyPreviewRT.blit(rect)
}

// ClosePartyPreview unloads the cached off-screen texture when the Party
// Visualizer modal closes. Idempotent.
func ClosePartyPreview() {
	partyPreviewRT.close()
}

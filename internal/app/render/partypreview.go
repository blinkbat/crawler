package render

import (
	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Party Visualizer preview support — the party-side twin of foepreview.go, sharing its diorama
// camera/anchor/tints and gizmo helpers. Only the lookup (class vs kind) and cached RT differ.

// partyPreviewRT is the party visualizer's cached off-screen render target (twin of foePreviewRT).
var partyPreviewRT previewRT

// LivePartyOverride returns the currently-loaded visual for class as an override (defaults with
// partyvisuals.json overlaid). ok=false if the class has no visual (defensive).
func LivePartyOverride(assets Resources, class core.PartyClass) (core.PartyVisualOverride, bool) {
	v, ok := partyVisualFor(assets, class)
	if !ok {
		return core.PartyVisualOverride{}, false
	}
	return enemyVisualOverride(v), true
}

// SetLivePartyOverride applies ov onto the in-memory visual for class so a save takes effect without
// reload. partyVisuals is shared by reference through the by-value Resources, so this mutates the
// same slice LivePartyOverride reads (else a class-cycle re-seeds from the stale loaded value).
// No-op if the class is out of range. Persistence to partyvisuals.json is the caller's job.
func SetLivePartyOverride(assets Resources, class core.PartyClass, ov core.PartyVisualOverride) {
	base, ok := visualAt(assets.partyVisuals, int(class))
	if !ok {
		return
	}
	v := applyEnemyVisualOverride(base, ov)
	// Re-bake non-destructive FX onto the pristine base so a Save applies in-session.
	v.texture = displayTextureForSlug(core.PartyClassSlug(class), pristineOrTexture(v), ov)
	assets.partyVisuals[class] = v
}

// DrawPartyPreview renders class's billboard (with override ov applied) into rect using the same
// math as drawBattlePack/DrawPartySprites. Safe per-frame; the off-screen texture is cached and
// reallocated only on panel resize.
func DrawPartyPreview(rect rl.Rectangle, assets Resources, class core.PartyClass, ov core.PartyVisualOverride, zoom float32, showGizmos bool, previewTex rl.Texture2D) {
	base, ok := partyVisualFor(assets, class)
	if !ok {
		return
	}
	v := applyEnemyVisualOverride(base, ov)
	// Asset-tab live preview overrides the displayed texture; zero ID falls back to the real texture.
	if previewTex.ID != 0 {
		v.texture = previewTex
	}
	drawVisualizerPreview(rect, &partyPreviewRT, v, zoom, foeAnchor, showGizmos, drawFriendlyTargetMarker)
}

// ClosePartyPreview unloads the cached off-screen texture when the Party
// Visualizer modal closes. Idempotent.
func ClosePartyPreview() {
	partyPreviewRT.close()
}

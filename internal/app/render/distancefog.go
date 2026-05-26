package render

import (
	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Distance fog for billboards. The world lighting shader fogs lit
// mesh geometry (walls / floors / props) via the standard
// `1 - exp(-density · dist)` curve clamped to 0.85. Billboards
// bypass that shader — raylib's DrawBillboardRec is a fixed-
// function sprite blit, multiplicative tint only — so a CPU
// alpha-mix approach was tried first and DIDN'T work: multiplicative
// tint can only darken or color-filter, it can't lerp the texture
// toward fog color.
//
// The fix is a dedicated billboard-fog fragment shader (see
// lighting.go's billboardFogVertexShader / billboardFogFragmentShader
// pair). Each billboard draw path (drawFieldPacks / drawBattlePack
// / DrawPartySprites) wraps its loop in BeginShaderMode(billboardFog)
// — same `mix(base, fogColor, fog)` math the world shader runs,
// but on the GPU per fragment so the silhouette mask is respected.
//
// This file owns the per-frame lighting-profile cache that both
// the world shader and the billboard shader read from, so the
// "what's the current FogColor / FogDensity" answer lives in one
// spot.

// cachedLightingProfile holds the per-frame resolved profile so
// billboard draws don't each re-run applyTimeOfDay+lightingFor.
// DrawWorld populates it as the first draw call inside the 3D
// pass; every subsequent billboard draw reads it. First-frame
// (before DrawWorld has ever run) reads as the zero value — fog
// density 0, fog color black — which produces no wash, the
// graceful default.
var cachedLightingProfile lightingProfile

// fogCeiling caps the fog mix so distant objects don't vanish
// entirely into fog color — even at maximum distance, 15% of the
// original tint survives. Single source of truth: both shaders
// (lightingFragmentShader + billboardFogFragmentShader) carry
// `{{FOG_CEILING}}` placeholders that resolveShaderFogCeiling
// substitutes with this value at LoadShaderFromMemory time.
const fogCeiling = float32(0.85)

// cacheLightingProfile is the setter DrawWorld calls once per
// frame with the freshly-computed profile. Keeping this on the
// render side (instead of stashing on GameState) avoids leaking
// render types into core.
func cacheLightingProfile(p lightingProfile) { cachedLightingProfile = p }

// resolvedLightingProfile returns the cached per-frame lighting
// profile. Callers must come AFTER DrawWorld in the draw order;
// see drawAdventureScene in run.go — billboard draws always
// follow the world draw inside the same 3D pass.
func resolvedLightingProfile(g core.GameState) lightingProfile {
	_ = g
	return cachedLightingProfile
}

// beginBillboardFogPass uploads the fog uniforms for the current
// frame's profile + camera, switches raylib into the billboard fog
// shader, and returns the end-function the caller should defer.
// Every billboard call site (drawFieldPacks, drawBattlePack,
// DrawPartySprites) previously open-coded the same four lines —
// `defer beginBillboardFogPass(camera, g, assets)()` collapses
// them to one. Centralising the begin/end pair also means a
// future fourth billboard surface (NPCs, item drops) joins the
// same seam without duplicating the uniform-upload boilerplate.
func beginBillboardFogPass(camera rl.Camera3D, g core.GameState, assets Resources) func() {
	profile := resolvedLightingProfile(g)
	assets.billboardFog.applyUniforms(camera, profile)
	rl.BeginShaderMode(assets.billboardFog.shader)
	return rl.EndShaderMode
}

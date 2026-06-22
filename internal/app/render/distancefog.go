package render

import (
	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Distance fog for billboards. Billboards bypass the world lighting shader
// (DrawBillboardRec is a fixed-function blit, multiplicative tint only — can't
// lerp toward fog color), so each billboard draw path wraps its loop in a
// dedicated billboardFog shader (see lighting.go) running the same mix() math
// per fragment. This file owns the per-frame lighting-profile cache both shaders read.

// cachedLightingProfile holds the per-frame profile so billboard draws don't
// re-run applyTimeOfDay+lightingFor. DrawWorld populates it first in the 3D pass;
// first-frame zero value (density 0) produces no wash — the graceful default.
var cachedLightingProfile lightingProfile

// fogCeiling caps the fog mix so distant objects keep 15% of their tint. Single
// source of truth: both shaders carry a {{FOG_CEILING}} placeholder substituted
// at LoadShaderFromMemory time.
const fogCeiling = float32(0.85)

// cacheLightingProfile is the setter DrawWorld calls once per frame. Kept render-
// side to avoid leaking render types into core.
func cacheLightingProfile(p lightingProfile) { cachedLightingProfile = p }

// resolvedLightingProfile returns the cached profile. Callers must come AFTER
// DrawWorld in the draw order (see drawAdventureScene in run.go).
func resolvedLightingProfile(g *core.GameState) lightingProfile {
	_ = g
	return cachedLightingProfile
}

// beginBillboardFogPass uploads fog uniforms, switches into the billboardFog
// shader, and returns the end-func to defer.
func beginBillboardFogPass(camera rl.Camera3D, g *core.GameState, assets Resources) func() {
	profile := resolvedLightingProfile(g)
	assets.billboardFog.applyUniforms(camera, profile)
	rl.BeginShaderMode(assets.billboardFog.shader)
	return rl.EndShaderMode
}

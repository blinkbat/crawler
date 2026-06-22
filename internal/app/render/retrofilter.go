package render

import (
	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Retro post-process filters (Debug ▸ Retro Filters). The 3D pass renders into an
// off-screen RenderTexture, then blits through ONE combined shader; each filter
// is a 0..1 intensity uniform in a fixed pipeline order, so any subset layers in
// one pass. HUD/popups/menus draw after and stay crisp. All lazy.

var (
	retroRT      previewRT // capture texture; shares the foe-preview RT lifecycle
	retroShader  rl.Shader
	retroLoaded  bool
	retroFailed  bool // shader failed to compile — disable the pass for the session
	retroLocRes  int32
	retroLocTime int32
	retroLocs    [core.RetroFilterCount]int32
)

// Reused uniform-upload scratch so EndRetroCapture doesn't allocate per
// SetShaderValue. SetShaderValue copies through to GL synchronously, so reusing
// one buffer across the per-filter loop is safe. Render is single-threaded.
var (
	retroResBuf  [2]float32
	retroTimeBuf [1]float32
	retroValBuf  [1]float32
)

// retroUniformNames maps each filter kind to its shader uniform, length-locked to
// the enum (a new filter without a uniform compile-errors here / panics below).
var retroUniformNames = [core.RetroFilterCount]string{
	core.RetroFilterPixelate:  "fPixelate",
	core.RetroFilterChroma:    "fChroma",
	core.RetroFilterPosterize: "fPosterize",
	core.RetroFilterDither:    "fDither",
	core.RetroFilterGameBoy:   "fGameBoy",
	core.RetroFilterScanlines: "fScanlines",
	core.RetroFilterPalette:   "fPalette",
}

func init() {
	for k, name := range retroUniformNames {
		if name == "" {
			panic("render: retroUniformNames missing an entry for RetroFilterKind " + core.RetroFilterName(core.RetroFilterKind(k)))
		}
	}
}

// retroFilterFragmentShader is the combined filter pipeline. Order is deliberate:
// sampling effects first (pixelate UV, chroma fetch), then palette work
// (posterize → dither → Game Boy → Palette), then scanlines last on top.
const retroFilterFragmentShader = `
#version 330
in vec2 fragTexCoord;
in vec4 fragColor;
uniform sampler2D texture0;
uniform vec2 resolution;
uniform float time;
uniform float fPixelate;
uniform float fChroma;
uniform float fPosterize;
uniform float fDither;
uniform float fGameBoy;
uniform float fScanlines;
uniform float fPalette;
out vec4 finalColor;

// 4x4 Bayer matrix, normalized to (0,1) thresholds at +0.5/16 centers.
const float bayer[16] = float[16](
     0.0,  8.0,  2.0, 10.0,
    12.0,  4.0, 14.0,  6.0,
     3.0, 11.0,  1.0,  9.0,
    15.0,  7.0, 13.0,  5.0);

// Classic 4-shade green LCD ramp, dark to light.
const vec3 gbRamp[4] = vec3[4](
    vec3(0.055, 0.149, 0.055),
    vec3(0.188, 0.384, 0.188),
    vec3(0.545, 0.675, 0.059),
    vec3(0.741, 0.890, 0.420));

// DawnBringer 16 — a balanced general-purpose 16-color pixel-art palette.
// The Palette filter snaps each pixel to the nearest of these (RGB distance).
const vec3 db16[16] = vec3[16](
    vec3(0.078, 0.047, 0.110), vec3(0.267, 0.141, 0.204),
    vec3(0.188, 0.204, 0.427), vec3(0.306, 0.290, 0.306),
    vec3(0.522, 0.298, 0.188), vec3(0.204, 0.396, 0.141),
    vec3(0.816, 0.275, 0.282), vec3(0.459, 0.443, 0.380),
    vec3(0.349, 0.490, 0.808), vec3(0.824, 0.490, 0.173),
    vec3(0.522, 0.584, 0.631), vec3(0.427, 0.667, 0.173),
    vec3(0.824, 0.667, 0.600), vec3(0.427, 0.761, 0.792),
    vec3(0.855, 0.831, 0.369), vec3(0.871, 0.933, 0.839));

void main()
{
    vec2 uv = fragTexCoord;

    // Pixelate: quantize the UV onto a coarse grid. Intensity drives the
    // block size (1px = off ... ~14px = full chunk).
    if (fPixelate > 0.0) {
        float px = mix(1.0, 14.0, fPixelate);
        vec2 grid = max(resolution / px, vec2(1.0));
        uv = (floor(uv*grid) + 0.5) / grid;
    }

    // Chroma fringe: fetch R and B slightly off-axis (worn composite cable).
    // The center fetch's ALPHA is carried through to the output — when the
    // skybox is exempted from the pass, the capture is cleared transparent
    // and the filtered environment alpha-blits over the crisp sky.
    vec4 baseTex = texture(texture0, uv);
    vec3 col;
    if (fChroma > 0.0) {
        float off = fChroma * 0.0045;
        col.r = texture(texture0, uv + vec2(off, 0.0)).r;
        col.g = baseTex.g;
        col.b = texture(texture0, uv - vec2(off, 0.0)).b;
    } else {
        col = baseTex.rgb;
    }

    // Posterize: crush the color depth. Intensity drives how few levels
    // survive (48 ≈ subtle banding ... 4 = poster).
    if (fPosterize > 0.0) {
        float levels = mix(48.0, 4.0, fPosterize);
        col = floor(col*levels + 0.5) / levels;
    }

    // Ordered dither: Bayer-threshold the color toward a 6-level quantize.
    // Blended by intensity so it can sit under a posterize without fighting.
    if (fDither > 0.0) {
        int bx = int(mod(gl_FragCoord.x, 4.0));
        int by = int(mod(gl_FragCoord.y, 4.0));
        float t = (bayer[by*4 + bx] + 0.5) / 16.0 - 0.5;
        float levels = 6.0;
        vec3 q = floor((col + t*(1.5/levels))*levels + 0.5) / levels;
        col = mix(col, q, fDither);
    }

    // Game Boy: luminance onto the 4-shade green LCD ramp.
    if (fGameBoy > 0.0) {
        float l = dot(col, vec3(0.299, 0.587, 0.114));
        int step = int(clamp(floor(l*4.0), 0.0, 3.0));
        col = mix(col, gbRamp[step], fGameBoy);
    }

    // Palette: snap to the nearest color in a fixed 16-color palette
    // (nearest-neighbor in RGB space), then blend by intensity. This is the
    // hard "limited palette" pixel-art look — far fewer, hand-picked colors
    // than posterize's per-channel banding. Layered after Game Boy so an
    // explicit palette choice wins when both are on.
    if (fPalette > 0.0) {
        vec3 best = db16[0];
        float bestD = dot(col - db16[0], col - db16[0]);
        for (int i = 1; i < 16; i++) {
            vec3 d = col - db16[i];
            float dist = dot(d, d);
            if (dist < bestD) { bestD = dist; best = db16[i]; }
        }
        col = mix(col, best, fPalette);
    }

    // Scanlines: soft CRT line darkening on alternating screen rows.
    if (fScanlines > 0.0) {
        float s = 0.5 + 0.5*sin(gl_FragCoord.y*3.14159265);
        col *= 1.0 - fScanlines*0.45*s;
    }

    finalColor = vec4(col, baseTex.a);
}
`

// ensureRetroShader lazily compiles the filter shader and caches its uniform
// locations; a failed compile disables the pass for the session (logged once).
func ensureRetroShader() bool {
	if retroLoaded {
		return true
	}
	if retroFailed {
		return false
	}
	sh := rl.LoadShaderFromMemory("", retroFilterFragmentShader)
	if sh.ID == 0 {
		retroFailed = true
		LogRenderError("retro filter shader failed to compile — filters disabled this session")
		return false
	}
	retroShader = sh
	retroLocRes = rl.GetShaderLocation(sh, "resolution")
	retroLocTime = rl.GetShaderLocation(sh, "time")
	// Log the resolved locations so a dropped/unwired uniform (-1) is observable
	// without a false-positive panic (GL also reports -1 for an unused uniform).
	LogRenderInit("retro filter shader %d locs: resolution=%d time=%d", sh.ID, retroLocRes, retroLocTime)
	for k := range retroLocs {
		retroLocs[k] = rl.GetShaderLocation(sh, retroUniformNames[k])
		LogRenderInit("retro filter loc %s (%s)=%d", core.RetroFilterName(core.RetroFilterKind(k)), retroUniformNames[k], retroLocs[k])
	}
	retroLoaded = true
	return true
}

// ensureRetroRT lazily (re)creates the capture texture at the screen size.
func ensureRetroRT(w, h int32) bool {
	if w <= 0 || h <= 0 {
		return false
	}
	return retroRT.ensure(w, h)
}

// BeginRetroCapture redirects drawing into the capture texture when a filter is
// active. true when capturing — the caller MUST then call EndRetroCapture after
// its 3D pass. false (renders straight to backbuffer) when filters are off or the
// shader/RT can't be built.
func BeginRetroCapture(g *core.GameState) bool {
	if g == nil || !core.AnyRetroFilterActive(&g.RetroFilters) {
		return false
	}
	if !ensureRetroShader() {
		return false
	}
	sw, sh := screenSize()
	if !ensureRetroRT(sw, sh) {
		return false
	}
	rl.BeginTextureMode(retroRT.rt)
	return true
}

// EndRetroCapture stops capturing and blits the scene to the backbuffer through
// the filter shader. Pair with a true BeginRetroCapture. skyOnBackbuffer means
// the caller already drew the crisp skybox there and the capture (cleared
// transparent) alpha-composites over it; otherwise the backbuffer is wiped first.
func EndRetroCapture(g *core.GameState, skyOnBackbuffer bool) {
	if g == nil {
		// Defensive: BeginRetroCapture returns false on nil g, so reaching here
		// with nil means the contract was ignored — bail before the deref.
		return
	}
	rl.EndTextureMode()
	if !skyOnBackbuffer {
		rl.ClearBackground(rl.Black)
	}
	retroResBuf[0], retroResBuf[1] = float32(retroRT.w), float32(retroRT.h)
	rl.SetShaderValue(retroShader, retroLocRes, retroResBuf[:], rl.ShaderUniformVec2)
	retroTimeBuf[0] = float32(rl.GetTime())
	rl.SetShaderValue(retroShader, retroLocTime, retroTimeBuf[:], rl.ShaderUniformFloat)
	for k := range retroLocs {
		retroValBuf[0] = float32(g.RetroFilters[k])
		rl.SetShaderValue(retroShader, retroLocs[k], retroValBuf[:], rl.ShaderUniformFloat)
	}
	rl.BeginShaderMode(retroShader)
	// blit flips the bottom-up texture upright; the shader applies the filter as it composites.
	retroRT.blit(rl.NewRectangle(0, 0, 0, 0))
	rl.EndShaderMode()
}

// DrawCrispSpritePass draws the billboards (and VFX) UNFILTERED over the already-
// filtered environment (the "Filter Sprites: Off" path). It re-opens the capture
// RT (whose depth still holds the environment), wipes only COLOR to transparent
// while keeping depth, draws the sprites (depth-testing for correct wall
// occlusion), then blits just the sprite layer crisp on top.
//
// Caller contract: invoke ONLY right after a true EndRetroCapture in the
// sprite-exempt arm, with the SAME camera. No-op if the RT was never built.
func DrawCrispSpritePass(camera rl.Camera3D, g *core.GameState, assets Resources) {
	if !retroRT.init {
		return
	}
	rl.BeginTextureMode(retroRT.rt)
	// Wipe COLOR to transparent but keep DEPTH: disable blending (so the blank
	// writes through, not a no-op alpha-composite) and the depth mask (so the
	// environment depth survives). Flush the batch while blending is still off, or
	// the clear quad replays later with blending back on.
	rl.DisableColorBlend()
	rl.DisableDepthMask()
	rl.DisableDepthTest()
	rl.DrawRectangle(0, 0, retroRT.w, retroRT.h, rl.Blank)
	rl.DrawRenderBatchActive()
	rl.EnableColorBlend()
	rl.EnableDepthMask()
	rl.EnableDepthTest()
	// Sprites render against the retained depth — those behind walls fail the
	// test, so the crisp blit carries only visible sprites.
	rl.BeginMode3D(camera)
	DrawEnemies(camera, g, assets)
	DrawPartySprites(camera, g, assets)
	TickAndDrawVFX(camera, g, assets)
	rl.EndMode3D()
	rl.EndTextureMode()
	// Crisp blit (no filter shader): the transparent background lets sprites
	// alpha-composite over the filtered environment. blit flips upright.
	retroRT.blit(rl.NewRectangle(0, 0, 0, 0))
}

// UnloadRetroFilter frees the capture texture and shader (Resources.Unload at
// shutdown). Idempotent.
func UnloadRetroFilter() {
	retroRT.close()
	if retroLoaded {
		rl.UnloadShader(retroShader)
		retroShader = rl.Shader{}
		retroLoaded = false
	}
}

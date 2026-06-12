package render

import (
	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Retro post-process filters (Debug ▸ Retro Filters). The adventure scene's
// 3D pass (sky + world + billboards + particles) renders into an off-screen
// RenderTexture, which is then blitted to the backbuffer through ONE combined
// fragment shader. Each filter is a 0..1 intensity uniform read from
// core.GameState.RetroFilters, applied in a fixed pipeline order inside the
// shader — so any subset of filters layers in a single pass, no ping-pong
// chain. HUD, popups, weather, and menus draw AFTER the blit and stay crisp.
//
// Everything is lazy: with all intensities at zero (the default), the scene
// renders straight to the backbuffer and none of this allocates or runs.
// Package-level singleton lifecycle, same pattern as the foe-preview RT.

var (
	retroRT      rl.RenderTexture2D
	retroRTW     int32
	retroRTH     int32
	retroRTInit  bool
	retroShader  rl.Shader
	retroLoaded  bool
	retroFailed  bool // shader failed to compile — disable the pass for the session
	retroLocRes  int32
	retroLocTime int32
	retroLocs    [core.RetroFilterCount]int32
)

// Reused uniform-upload scratch so EndRetroCapture doesn't allocate a fresh
// []float32 per SetShaderValue every frame the pass runs (mirrors lighting.go's
// uniformVec3Buf / uniformFloatBuf). SetShaderValue copies the value through to
// GL synchronously, so reusing one buffer across the per-filter loop is safe.
// Render is single-threaded; one shared scratch each is fine.
var (
	retroResBuf  [2]float32
	retroTimeBuf [1]float32
	retroValBuf  [1]float32
)

// retroUniformNames maps each filter kind to its shader uniform, length-locked
// to the enum by the array size (a new filter without a uniform is a compile
// error here and a startup panic below if left empty).
var retroUniformNames = [core.RetroFilterCount]string{
	core.RetroFilterPixelate:  "fPixelate",
	core.RetroFilterChroma:    "fChroma",
	core.RetroFilterPosterize: "fPosterize",
	core.RetroFilterDither:    "fDither",
	core.RetroFilterGameBoy:   "fGameBoy",
	core.RetroFilterScanlines: "fScanlines",
}

func init() {
	for k, name := range retroUniformNames {
		if name == "" {
			panic("render: retroUniformNames missing an entry for RetroFilterKind " + core.RetroFilterName(core.RetroFilterKind(k)))
		}
	}
}

// retroFilterFragmentShader is the combined filter pipeline. Order matters
// and is deliberate: sampling effects first (pixelate quantizes the UV,
// chroma fringes the fetch), then palette work on the fetched color
// (posterize → dither → Game Boy), then screen-space scanlines last so the
// CRT lines ride on top of whatever palette the pipeline produced.
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

    // Scanlines: soft CRT line darkening on alternating screen rows.
    if (fScanlines > 0.0) {
        float s = 0.5 + 0.5*sin(gl_FragCoord.y*3.14159265);
        col *= 1.0 - fScanlines*0.45*s;
    }

    finalColor = vec4(col, baseTex.a);
}
`

// ensureRetroShader lazily compiles the filter shader and caches its uniform
// locations. A failed compile marks the pass disabled for the session (logged
// once) rather than retrying every frame or crashing the scene.
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
	// Stamp the resolved uniform locations to the render log so the shader half
	// of the RetroFilterKind contract is observable: a filter whose uniform was
	// dropped/renamed in the shader source (or never wired into the pipeline)
	// resolves to -1 here, visible in the log without a false-positive panic
	// (GL legitimately reports -1 for a declared-but-unused uniform too).
	LogRenderInit("retro filter shader %d locs: resolution=%d time=%d", sh.ID, retroLocRes, retroLocTime)
	for k := range retroLocs {
		retroLocs[k] = rl.GetShaderLocation(sh, retroUniformNames[k])
		LogRenderInit("retro filter loc %s (%s)=%d", core.RetroFilterName(core.RetroFilterKind(k)), retroUniformNames[k], retroLocs[k])
	}
	retroLoaded = true
	return true
}

// ensureRetroRT lazily (re)creates the capture texture at the current screen
// size — same resize discipline as the foe-preview RT.
func ensureRetroRT(w, h int32) bool {
	if retroRTInit && retroRTW == w && retroRTH == h {
		return true
	}
	if retroRTInit {
		rl.UnloadRenderTexture(retroRT)
		retroRTInit = false
	}
	if w <= 0 || h <= 0 {
		return false
	}
	rt := rl.LoadRenderTexture(w, h)
	if rt.ID == 0 {
		return false
	}
	retroRT = rt
	retroRTW, retroRTH = w, h
	retroRTInit = true
	return true
}

// BeginRetroCapture redirects subsequent drawing into the capture texture
// when at least one retro filter is active. Returns true when capturing —
// the caller MUST then call EndRetroCapture after its 3D pass. Returns false
// (and draws nothing) when filters are off or the shader/RT can't be built,
// in which case the scene renders directly to the backbuffer as always.
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
	rl.BeginTextureMode(retroRT)
	return true
}

// EndRetroCapture stops capturing and blits the captured environment to the
// backbuffer through the filter shader. Pair with a true return from
// BeginRetroCapture. skyOnBackbuffer reports that the caller already drew
// the (crisp, filter-exempt) skybox to the backbuffer before the capture —
// the capture was cleared TRANSPARENT, so the blit alpha-composites the
// filtered environment over that sky.
//
// When the sky is NOT on the backbuffer, the backbuffer is
// ClearBackground'd first (before the blit): the frame's load-bearing depth
// wipe happened inside the capture texture, so without this the backbuffer
// would still carry LAST frame's depth — and the crisp-sprite pass that
// follows (which re-renders the environment depth-only, then draws
// billboards depth-tested against it) would z-fight history. The clear's
// color is irrelevant; the blit overwrites every pixel. (In the
// sky-on-backbuffer arm, run.go's own pre-sky ClearBackground performed
// that same wipe.)
func EndRetroCapture(g *core.GameState, skyOnBackbuffer bool) {
	rl.EndTextureMode()
	if !skyOnBackbuffer {
		rl.ClearBackground(rl.Black)
	}
	retroResBuf[0], retroResBuf[1] = float32(retroRTW), float32(retroRTH)
	rl.SetShaderValue(retroShader, retroLocRes, retroResBuf[:], rl.ShaderUniformVec2)
	retroTimeBuf[0] = float32(rl.GetTime())
	rl.SetShaderValue(retroShader, retroLocTime, retroTimeBuf[:], rl.ShaderUniformFloat)
	for k := range retroLocs {
		retroValBuf[0] = float32(g.RetroFilters[k])
		rl.SetShaderValue(retroShader, retroLocs[k], retroValBuf[:], rl.ShaderUniformFloat)
	}
	rl.BeginShaderMode(retroShader)
	// RenderTextures store rows bottom-up; the negative source height flips
	// the blit right-side up (same idiom as the foe-preview blit).
	rl.DrawTextureRec(retroRT.Texture,
		rl.NewRectangle(0, 0, float32(retroRTW), -float32(retroRTH)),
		rl.NewVector2(0, 0), rl.White)
	rl.EndShaderMode()
}

// GL blend factors / equation for the depth-only re-render below. Named
// locally — raylib doesn't export GL_* and these three are stable core-GL
// values (glBlendFunc(GL_ZERO, GL_ONE) + glBlendEquation(GL_FUNC_ADD)).
const (
	glZero    = 0
	glOne     = 1
	glFuncAdd = 0x8006
)

// RetroDepthPrepass re-renders the environment with a ZERO/ONE blend —
// every fragment's color is discarded (dst keeps the filtered blit) but
// DEPTH writes proceed normally. This rebuilds the backbuffer's depth so the
// crisp sprite pass that follows is properly occluded by walls, trees, and
// props even though the environment's COLOR came from the filtered texture.
// The classic color-mask depth prepass, done with blend factors because the
// raylib-go bindings don't expose rlColorMask. Only runs while filters are
// active, so the environment double-draw costs nothing in normal play.
//
// The world geometry goes through drawWorld's depthOnly path: the full
// DrawWorld that fed the capture already set this frame's lighting/torch/fog
// uniforms (same shader, same camera, same time-of-day), so the prepass skips
// re-collecting torches and re-uploading those uniforms — it only needs the
// geometry re-rasterized to rebuild depth. (DrawChests / DrawDoors carry no
// such per-frame lighting setup, so they re-run as-is.)
func RetroDepthPrepass(camera rl.Camera3D, g core.GameState, assets Resources) {
	rl.SetBlendFactors(glZero, glOne, glFuncAdd)
	rl.BeginBlendMode(rl.BlendCustom)
	drawWorld(camera, g, assets, true)
	DrawChests(camera, g, assets)
	DrawDoors(camera, g, assets)
	rl.EndBlendMode()
}

// UnloadRetroFilter frees the capture texture and shader. Called from
// Resources.Unload at shutdown; idempotent and safe when nothing loaded.
func UnloadRetroFilter() {
	if retroRTInit {
		rl.UnloadRenderTexture(retroRT)
		retroRT = rl.RenderTexture2D{}
		retroRTInit = false
		retroRTW, retroRTH = 0, 0
	}
	if retroLoaded {
		rl.UnloadShader(retroShader)
		retroShader = rl.Shader{}
		retroLoaded = false
	}
}

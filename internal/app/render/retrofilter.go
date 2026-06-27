package render

import (
	"fmt"
	"strings"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Retro post-process filters: 3D pass → off-screen RT → ONE combined shader; each
// filter is a 0..1 intensity uniform in fixed pipeline order, so any subset layers
// in one pass. HUD draws after, crisp. All lazy.

var (
	retroRT      previewRT // capture texture; shares the foe-preview RT lifecycle
	retroShader  rl.Shader
	retroLoaded  bool
	retroFailed  bool // compile failed — pass disabled for the session
	retroLocRes  int32
	retroLocTime int32
	retroLocs    [core.RetroFilterCount]int32
)

// Reused uniform-upload scratch (SetShaderValue copies to GL synchronously, so one
// buffer across the loop is safe). Render is single-threaded.
var (
	retroResBuf  [2]float32
	retroTimeBuf [1]float32
	retroValBuf  [1]float32
)

// retroUniformNames maps each filter kind to its shader uniform; length-locked to
// the enum (missing entry panics in init).
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

// init asserts the retro shader's bayer[]/gbRamp[] GLSL constants still carry the
// exact values of their Go twins in spriteedit.go (bayer4x4 / gbGreenRamp). GLSL
// arrays can't be shared, so the CPU palette filter mirrors them by hand; editing
// one side without the other now panics at startup instead of drifting silently.
func init() {
	src := strings.NewReplacer(" ", "", "\n", "", "\t", "").Replace(retroFilterFragmentShader)

	var bayer strings.Builder
	bayer.WriteString("float[16](")
	for i, v := range bayer4x4 {
		if i > 0 {
			bayer.WriteByte(',')
		}
		fmt.Fprintf(&bayer, "%.1f", v)
	}
	bayer.WriteByte(')')
	if !strings.Contains(src, bayer.String()) {
		panic("render: retro shader bayer[] disagrees with bayer4x4 (spriteedit.go) — mirror the change to both")
	}

	var ramp strings.Builder
	ramp.WriteString("vec3[4](")
	for i, c := range gbGreenRamp {
		if i > 0 {
			ramp.WriteByte(',')
		}
		fmt.Fprintf(&ramp, "vec3(%.3f,%.3f,%.3f)", c[0], c[1], c[2])
	}
	ramp.WriteByte(')')
	if !strings.Contains(src, ramp.String()) {
		panic("render: retro shader gbRamp[] disagrees with gbGreenRamp (spriteedit.go) — mirror the change to both")
	}

	// The shader's dither quantize level count mirrors spriteedit.go's ditherQuantLevels;
	// GLSL can't share the const, so string-match it like bayer[]/gbRamp[] above.
	if !strings.Contains(src, fmt.Sprintf("floatlevels=%.1f;", ditherQuantLevels)) {
		panic("render: retro shader dither levels disagrees with ditherQuantLevels (spriteedit.go) — mirror the change to both")
	}
}

// retroFilterFragmentShader is the combined pipeline. Order matters: sampling
// (pixelate UV, chroma) → palette (posterize→dither→GameBoy→Palette) → scanlines.
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

// ensureRetroShader lazily compiles the shader + caches uniform locations; a
// failed compile disables the pass for the session.
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
	// Log locations; GL reports -1 for unused uniforms, so don't panic on -1.
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
// active. true ⇒ caller MUST call EndRetroCapture after its 3D pass; false ⇒
// renders straight to backbuffer (filters off or shader/RT unavailable).
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

// EndRetroCapture blits the scene to the backbuffer through the filter shader.
// Pair with a true BeginRetroCapture. skyOnBackbuffer ⇒ caller already drew the
// crisp sky and the capture alpha-composites over it; else the backbuffer is wiped.
func EndRetroCapture(g *core.GameState, skyOnBackbuffer bool) {
	// Contract: BeginRetroCapture returns false on nil g, so this shouldn't happen.
	if g == nil {
		return
	}
	rl.EndTextureMode()
	if !skyOnBackbuffer {
		rl.ClearBackground(rl.Black)
	}
	blitRetroRT(g, true)
}

// blitRetroRT composites the capture RT onto the backbuffer. withShader ⇒ upload the
// filter uniforms and wrap the blit in the retro shader (the filtered environment / FX
// paths); else a plain crisp blit (the sprite pass). The blit flips upright (negative
// height lives in previewRT.blit). Upload MUST precede BeginShaderMode.
func blitRetroRT(g *core.GameState, withShader bool) {
	if withShader {
		uploadRetroUniforms(g)
		rl.BeginShaderMode(retroShader)
		retroRT.blit(rl.NewRectangle(0, 0, 0, 0))
		rl.EndShaderMode()
		return
	}
	retroRT.blit(rl.NewRectangle(0, 0, 0, 0))
}

// uploadRetroUniforms pushes the resolution / time / per-filter intensities to the
// shader. Shared by EndRetroCapture and DrawFilteredCombatFX so the second blit uses
// the identical filter settings.
func uploadRetroUniforms(g *core.GameState) {
	retroResBuf[0], retroResBuf[1] = float32(retroRT.w), float32(retroRT.h)
	rl.SetShaderValue(retroShader, retroLocRes, retroResBuf[:], rl.ShaderUniformVec2)
	retroTimeBuf[0] = float32(rl.GetTime())
	rl.SetShaderValue(retroShader, retroLocTime, retroTimeBuf[:], rl.ShaderUniformFloat)
	for k := range retroLocs {
		retroValBuf[0] = float32(g.RetroFilters[k])
		rl.SetShaderValue(retroShader, retroLocs[k], retroValBuf[:], rl.ShaderUniformFloat)
	}
}

// DrawFilteredCombatFX renders the combat "juice" — VFX particles + hit-glyphs —
// into the retro RT and blits it through the retro shader, so the action glyphs and
// sparks crunch with the rest of the scene instead of popping crisp on top. Owns the
// once-per-frame VFX tick for this path; the caller must skip the inline/crisp VFX
// draw AND the unfiltered hit-glyph draw. Call only in battle when a filter is active
// (RT + shader are live from EndRetroCapture this frame); falls back to a crisp draw
// if the RT/shader somehow aren't ready, so the FX never silently vanishes.
func DrawFilteredCombatFX(camera rl.Camera3D, g *core.GameState, assets Resources) {
	if !retroRT.init || !retroLoaded {
		rl.BeginMode3D(camera)
		rl.DisableDepthTest()
		TickAndDrawVFX(camera, g, assets)
		rl.EnableDepthTest()
		rl.EndMode3D()
		DrawHitGlyphs(camera, g, assets)
		return
	}
	rl.BeginTextureMode(retroRT.rt)
	rl.ClearBackground(rl.Blank) // transparent FX layer (alpha-composites on blit)
	// Particles (3D, depth off so battle FX paint over) then hit-glyphs (2D screen
	// space) — both land in the RT and the one -height blit flips them upright.
	rl.BeginMode3D(camera)
	rl.DisableDepthTest()
	TickAndDrawVFX(camera, g, assets)
	rl.EnableDepthTest()
	rl.EndMode3D()
	DrawHitGlyphs(camera, g, assets)
	rl.EndTextureMode()
	blitRetroRT(g, true)
}

// DrawCrispSpritePass draws billboards (+VFX, unless withVFX is false) UNFILTERED
// over the already-filtered environment ("Filter Sprites: Off"): re-opens the
// capture RT (depth still holds the environment), wipes COLOR but keeps depth, draws
// sprites (depth-tested for wall occlusion), blits the sprite layer crisp on top.
// withVFX=false leaves the once-per-frame VFX tick to a later filtered FX pass
// (battle + filter active — see DrawFilteredCombatFX).
// Contract: call ONLY right after a true EndRetroCapture, SAME camera. No-op if no RT.
func DrawCrispSpritePass(camera rl.Camera3D, g *core.GameState, assets Resources, withVFX bool) {
	if !retroRT.init {
		return
	}
	rl.BeginTextureMode(retroRT.rt)
	// Wipe COLOR, keep DEPTH: blending off (so the blank writes through) + depth
	// mask off (environment depth survives). Flush the batch while blending is off,
	// else the clear quad replays later with blending back on.
	rl.DisableColorBlend()
	rl.DisableDepthMask()
	rl.DisableDepthTest()
	rl.DrawRectangle(0, 0, retroRT.w, retroRT.h, rl.Blank)
	rl.DrawRenderBatchActive()
	rl.EnableColorBlend()
	rl.EnableDepthMask()
	rl.EnableDepthTest()
	// Sprites depth-test against retained depth (only visible ones blit) — except in
	// battle, where they paint over the environment like the non-filtered path.
	rl.BeginMode3D(camera)
	inBattle := g.Battle.Active()
	if inBattle {
		rl.DisableDepthTest()
	}
	DrawEnemies(camera, g, assets)
	DrawPartySprites(camera, g, assets)
	if withVFX {
		TickAndDrawVFX(camera, g, assets)
	}
	if inBattle {
		rl.EnableDepthTest()
	}
	rl.EndMode3D()
	rl.EndTextureMode()
	// Crisp blit (no shader): transparent bg alpha-composites sprites over the
	// filtered environment.
	blitRetroRT(g, false)
}

// UnloadRetroFilter frees the capture texture + shader. Idempotent.
func UnloadRetroFilter() {
	retroRT.close()
	if retroLoaded {
		rl.UnloadShader(retroShader)
		retroShader = rl.Shader{}
		retroLoaded = false
	}
}

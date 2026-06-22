package render

import (
	"fmt"
	"log"
	"math"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// fogCeilingToken: placeholder both shaders carry for the fog clamp ceiling;
// resolveShaderTokens substitutes fogCeiling (distancefog.go) at load time.
const fogCeilingToken = "{{FOG_CEILING}}"

// painterlyGradeToken: placeholder both shaders carry for the shared color grade;
// resolveShaderTokens substitutes painterlyGradeGLSL at load time.
const painterlyGradeToken = "{{PAINTERLY_GRADE}}"

// painterlyGradeGLSL is the shared day→night grade injected into BOTH shaders so
// they can't drift. Operates on vec3 `lit` via `nightMood` (0 day, 1 night);
// temp locals blocked off to avoid scope collision. The four KNOBS live here.
const painterlyGradeGLSL = `
    // (1) Soft highlight shoulder — roll bright pastels off instead of clipping.
    lit = lit / (1.0 + 0.16 * max(max(lit.r, lit.g), lit.b));
    {
        // (2) Shadow lift: airy/papery by day, near-black gloom at night.
        float lift = mix(0.055, 0.004, nightMood);
        lit = lift + lit * (1.0 - lift);
        // (3) Saturation: clean pastel by day, drained and eerie at night.
        float gradeLuma = dot(lit, vec3(0.299, 0.587, 0.114));
        lit = mix(lit, vec3(gradeLuma), mix(0.16, 0.45, nightMood));
        // (4) Temperature: warm paper-cream by day, cold moonlit indigo at night.
        lit *= mix(vec3(1.03, 1.00, 0.95), vec3(0.82, 0.88, 1.08), nightMood);
    }
`

// resolveShaderTokens substitutes the fog ceiling + painterly grade into a
// shader's source at load time.
func resolveShaderTokens(src string) string {
	src = strings.ReplaceAll(src, fogCeilingToken, fmt.Sprintf("%.4f", fogCeiling))
	src = strings.ReplaceAll(src, painterlyGradeToken, painterlyGradeGLSL)
	return src
}

const lightingVertexShader = `#version 330

in vec3 vertexPosition;
in vec2 vertexTexCoord;
in vec3 vertexNormal;
in vec4 vertexColor;

uniform mat4 mvp;
uniform mat4 matModel;
uniform mat4 matNormal;

out vec2 fragTexCoord;
out vec4 fragColor;
out vec3 fragNormal;
out vec3 fragPosition;

void main() {
    fragTexCoord = vertexTexCoord;
    fragColor = vertexColor;
    fragNormal = normalize(vec3(matNormal * vec4(vertexNormal, 0.0)));
    fragPosition = vec3(matModel * vec4(vertexPosition, 1.0));
    gl_Position = mvp * vec4(vertexPosition, 1.0);
}
`

// billboardFogVertexShader: the fogged billboard vertex shader. No normal pipe —
// raylib's billboard draw supplies none, which would feed garbage into lighting.
const billboardFogVertexShader = `#version 330

in vec3 vertexPosition;
in vec2 vertexTexCoord;
in vec4 vertexColor;

uniform mat4 mvp;
uniform mat4 matModel;

out vec2 fragTexCoord;
out vec4 fragColor;
out vec3 fragPosition;

void main() {
    fragTexCoord = vertexTexCoord;
    fragColor = vertexColor;
    fragPosition = vec3(matModel * vec4(vertexPosition, 1.0));
    gl_Position = mvp * vec4(vertexPosition, 1.0);
}
`

// billboardFogFragmentShader: sample, mix toward fogColor by the world's fog
// curve, output. No directional lighting (billboards face the camera).
const billboardFogFragmentShader = `#version 330

in vec2 fragTexCoord;
in vec4 fragColor;
in vec3 fragPosition;

uniform sampler2D texture0;
uniform vec4 colDiffuse;

uniform vec3 viewPos;
uniform vec3 fogColor;
uniform float fogDensity;
uniform float nightMood; // 0 = serene day, 1 = spooky night — drives the grade

out vec4 finalColor;

void main() {
    vec4 texel = texture(texture0, fragTexCoord);
    // Alpha-test cutout: drop near-transparent sprite fragments so the
    // billboard's transparent quad corners don't WRITE DEPTH and clip the
    // particles / geometry drawn behind them (the "sprites cut up by an
    // invisible bounding box" bug — particles draw last, depth-tested, in
    // the same 3D pass). Tested on the texture's own alpha only, not the
    // runtime tint/fade alpha, so the cutout silhouette stays stable while
    // a defeated enemy fades out. The threshold is far lower than the world
    // shader's 0.5 (hard foliage cutouts) on purpose: it kills only the
    // fully-transparent quad while leaving the soft, alpha-blended sprite
    // edge intact, which a 0.5 cutout would harden.
    if (texel.a < 0.08) discard;
    vec3 base = texel.rgb * fragColor.rgb * colDiffuse.rgb;
    float dist = length(viewPos - fragPosition);
    float fog = 1.0 - exp(-fogDensity * dist);
    fog = clamp(fog, 0.0, {{FOG_CEILING}});
    vec3 lit = mix(base, fogColor, fog);

    // Shared painterly grade — injected from painterlyGradeGLSL (one source for
    // both shaders).
    {{PAINTERLY_GRADE}}

    finalColor = vec4(lit, texel.a * fragColor.a * colDiffuse.a);
}
`

type billboardFogShaderPipe struct {
	shader        rl.Shader
	locViewPos    int32
	locFogColor   int32
	locFogDensity int32
	locNightMood  int32
}

func loadBillboardFogShader() billboardFogShaderPipe {
	shader := rl.LoadShaderFromMemory(billboardFogVertexShader, resolveShaderTokens(billboardFogFragmentShader))
	if shader.ID == 0 {
		log.Println("render: billboard fog shader failed to compile; billboards will not fog out at distance")
		LogRenderError("billboard fog shader compile FAILED (shader.ID==0); raylib falls back to default shader, billboards will not fog")
	} else {
		LogRenderInit("billboard fog shader compiled OK (shader.ID=%d)", shader.ID)
	}
	pipe := billboardFogShaderPipe{
		shader:        shader,
		locViewPos:    rl.GetShaderLocation(shader, "viewPos"),
		locFogColor:   rl.GetShaderLocation(shader, "fogColor"),
		locFogDensity: rl.GetShaderLocation(shader, "fogDensity"),
		locNightMood:  rl.GetShaderLocation(shader, "nightMood"),
	}
	LogRenderInit("billboard fog locs: viewPos=%d fogColor=%d fogDensity=%d nightMood=%d", pipe.locViewPos, pipe.locFogColor, pipe.locFogDensity, pipe.locNightMood)
	// Re-prime the uniform-upload memo (an ID match isn't sufficient — see
	// loadLightingShader).
	billboardFogPrimed = false
	return pipe
}

func (s billboardFogShaderPipe) unload() {
	if s.shader.ID != 0 {
		rl.UnloadShader(s.shader)
	}
}

// uniformVec3Buf / uniformFloatBuf are reused across every uniform upload so the
// per-frame paths don't allocate fresh slice literals. Renderer is single-threaded.
var (
	uniformVec3Buf  [3]float32
	uniformFloatBuf [1]float32
)

// billboardFog* memoize the last fog-uniform upload to skip redundant ones (the
// uniforms only shift on a phase crossing). Keyed on shader ID so a reload re-primes.
var (
	billboardFogViewPos  rl.Vector3
	billboardFogProfile  lightingProfile
	billboardFogShaderID uint32
	billboardFogPrimed   bool
)

func (s billboardFogShaderPipe) applyUniforms(camera rl.Camera3D, profile lightingProfile) {
	if s.shader.ID == 0 {
		return
	}
	if billboardFogPrimed && billboardFogShaderID == s.shader.ID &&
		camera.Position == billboardFogViewPos && profile == billboardFogProfile {
		return
	}
	billboardFogPrimed = true
	billboardFogShaderID = s.shader.ID
	billboardFogViewPos = camera.Position
	billboardFogProfile = profile
	uniformVec3Buf[0], uniformVec3Buf[1], uniformVec3Buf[2] = camera.Position.X, camera.Position.Y, camera.Position.Z
	rl.SetShaderValue(s.shader, s.locViewPos, uniformVec3Buf[:], rl.ShaderUniformVec3)
	uniformVec3Buf[0], uniformVec3Buf[1], uniformVec3Buf[2] = profile.FogColor.X, profile.FogColor.Y, profile.FogColor.Z
	rl.SetShaderValue(s.shader, s.locFogColor, uniformVec3Buf[:], rl.ShaderUniformVec3)
	uniformFloatBuf[0] = profile.FogDensity
	rl.SetShaderValue(s.shader, s.locFogDensity, uniformFloatBuf[:], rl.ShaderUniformFloat)
	uniformFloatBuf[0] = profile.Mood
	rl.SetShaderValue(s.shader, s.locNightMood, uniformFloatBuf[:], rl.ShaderUniformFloat)
}

// Lighting is a single forward pass: sun + hemisphere ambient + pseudo-AO + fog.
// Cast shadows were removed (the multi-pass depth/PCF setup was fragile against
// raylib's implicit texture binding — DrawMesh ignores SetShaderValueTexture).
const lightingFragmentShader = `#version 330

in vec2 fragTexCoord;
in vec4 fragColor;
in vec3 fragNormal;
in vec3 fragPosition;

uniform sampler2D texture0;
uniform vec4 colDiffuse;

uniform vec3 sunDirection;
uniform vec3 sunColor;
uniform vec3 ambientColor;
uniform vec3 viewPos;
uniform vec3 fogColor;
uniform float fogDensity;
uniform float specularStrength;
uniform float shadowStrength;
uniform float nightMood; // 0 = serene day, 1 = spooky night — drives the grade

// Torch point lights. Unused slots carry a zero color so the loop
// always runs MAX_TORCHES iterations (a fixed bound 330 likes) with
// disabled torches contributing nothing — no int-count uniform
// needed. torchRange is the shared world-unit reach of every torch.
#define MAX_TORCHES 12
uniform vec3 torchPos[MAX_TORCHES];
uniform vec3 torchColor[MAX_TORCHES];
uniform float torchRange;

out vec4 finalColor;

void main() {
    vec4 texel = texture(texture0, fragTexCoord);
    // Alpha-test cutout: fully-transparent fragments don't write to
    // color OR depth. Without this, leaf/bush/etc. textures with
    // 0-alpha pixels stamp their cutout shape into the depth buffer,
    // which blocks anything behind them — trees behind other trees
    // disappear, and the hidden bits shift as the camera moves
    // because draw order changes the depth-buffer "holes." Threshold
    // 0.5 is the standard alpha-cutout gate.
    if (texel.a * colDiffuse.a < 0.5) discard;
    vec3 base = texel.rgb * fragColor.rgb * colDiffuse.rgb;

    vec3 N = normalize(fragNormal);
    vec3 L = normalize(-sunDirection);
    vec3 V = normalize(viewPos - fragPosition);
    vec3 H = normalize(L + V);

    float NdotL = max(dot(N, L), 0.0);

    // Hemisphere ambient: sky tint above, ground tint below
    float upDot = N.y * 0.5 + 0.5;
    vec3 hemi = mix(ambientColor * 0.65, ambientColor, upDot);

    // Painted cel shading — a soft three-tone ramp (shadow / mid / light)
    // with smoothstepped terminators so the banding reads as gouache
    // brushwork, not hard plastic toon. The shadow band floors at 0.45 so
    // unlit faces stay luminous and airy (Wind-Waker-ish serenity) rather
    // than sinking to black; night gloom comes from the grade + dim night
    // sun, not from crushed diffuse. KNOBS: band centers/softness below,
    // and the two mix() weights (how "cel" vs smooth the surface reads).
    float wrap = clamp((dot(N, L) + 0.30) / 1.30, 0.0, 1.0);
    float band = 0.45
               + smoothstep(0.34, 0.46, wrap) * 0.27
               + smoothstep(0.60, 0.74, wrap) * 0.28;
    float shade = mix(wrap, band, 0.55);
    vec3 diffuse = sunColor * mix(NdotL, shade, 0.55);

    float spec = 0.0;
    if (specularStrength > 0.001 && NdotL > 0.0) {
        spec = pow(max(dot(N, H), 0.0), 26.0) * specularStrength;
    }

    // Rim light — kept as a hint of the painted-edge feel but
    // pulled WAY back from the prior pass. At 0.16 it reads as a
    // gentle painted edge on silhouettes rather than a hot halo.
    float rim = pow(1.0 - max(dot(N, V), 0.0), 2.6);
    rim *= smoothstep(-0.1, 0.5, dot(N, L));
    vec3 rimLight = sunColor * rim * 0.16;

    // Pseudo-AO: darken slightly where surface points away from sun. With
    // cast shadows gone this is the only thing that gives shaded areas
    // visual depth, so shadowStrength tunes "how dark do shadowed-feeling
    // surfaces get."
    float ao = mix(1.0 - shadowStrength, 1.0, smoothstep(-0.1, 0.6, dot(N, L)));

    vec3 lit = base * (hemi + diffuse) * ao + sunColor * spec + rimLight;

    // Torch point lights — warm pools of light in the dark dungeon.
    // Each torch is a range-limited point light with a quadratic
    // soft-edge falloff; a wrapped N·L term lets floors and walls
    // near the torch base catch light even at grazing angles. The
    // accumulated torch contribution multiplies the surface base
    // colour (so a torch lights the texture, it doesn't just add a
    // flat orange wash) and is added on top of the ambient/sun lit
    // value computed above.
    vec3 torchAccum = vec3(0.0);
    for (int i = 0; i < MAX_TORCHES; i++) {
        vec3 toL = torchPos[i] - fragPosition;
        float d = length(toL);
        vec3 Ld = toL / max(d, 0.001);
        float ndl = max(dot(N, Ld), 0.0);
        float wrapNdl = ndl * 0.65 + 0.35;
        float atten = clamp(1.0 - d / torchRange, 0.0, 1.0);
        atten *= atten;
        torchAccum += torchColor[i] * wrapNdl * atten;
    }
    lit += base * torchAccum;

    // Exponential height-aware fog. The ceiling preserves 15% of
    // the lit tint at maximum distance so silhouettes don't fade
    // to invisibility. {{FOG_CEILING}} is substituted at shader-
    // load time from the Go fogCeiling constant — see
    // resolveShaderTokens above. ONE source of truth across Go
    // + both shaders.
    float dist = length(viewPos - fragPosition);
    float fog = 1.0 - exp(-fogDensity * dist);
    fog = clamp(fog, 0.0, {{FOG_CEILING}});
    lit = mix(lit, fogColor, fog);

    // Shared painterly grade — injected from painterlyGradeGLSL (one source for
    // both shaders).
    {{PAINTERLY_GRADE}}

    finalColor = vec4(lit, texel.a * fragColor.a * colDiffuse.a);
}
`

type lightingShader struct {
	shader            rl.Shader
	locViewPos        int32
	locSunDirection   int32
	locSunColor       int32
	locAmbientColor   int32
	locFogColor       int32
	locFogDensity     int32
	locSpecStrength   int32
	locShadowStrength int32
	locNightMood      int32
	locTorchPos       int32
	locTorchColor     int32
	locTorchRange     int32
}

// maxTorches mirrors MAX_TORCHES in the shader. The Go side picks the closest N
// and zeroes the rest.
const maxTorches = 12

// torchRangeWorld is every torch's reach (torchRange uniform) — ~9 units, so a
// torch lights its room without bleeding down a corridor.
const torchRangeWorld = float32(9.0)

// torchLight is one active torch's world position + (already flickered) color.
type torchLight struct {
	Pos   rl.Vector3
	Color rl.Vector3
}

// Reused flat upload buffers so the per-frame torch upload doesn't allocate.
// Indexed as [i*3 + channel].
var (
	torchPosBuf   [maxTorches * 3]float32
	torchColorBuf [maxTorches * 3]float32
)

// Torch upload memo. torchRange is constant → uploaded once; torchSlotsZeroed
// lets a torchless area skip the per-frame uploads. Keyed on shader ID.
var (
	torchRangePrimed   bool
	torchRangeShaderID uint32
	torchSlotsZeroed   bool
	torchSlotsShaderID uint32
)

func loadLightingShader() lightingShader {
	shader := rl.LoadShaderFromMemory(lightingVertexShader, resolveShaderTokens(lightingFragmentShader))
	if shader.ID == 0 {
		// ID==0 (compile/link fail) makes BeginShaderMode silently no-op and the
		// world draws unlit; log so we don't lose the signal.
		log.Println("render: lighting shader failed to compile; rendering will fall back to raylib's default shader")
		LogRenderError("lighting shader compile FAILED (shader.ID==0); the world will draw with raylib's default shader, NO sun/ambient/fog/torch lighting")
	} else {
		LogRenderInit("lighting shader compiled OK (shader.ID=%d)", shader.ID)
	}
	s := lightingShader{
		shader:            shader,
		locViewPos:        rl.GetShaderLocation(shader, "viewPos"),
		locSunDirection:   rl.GetShaderLocation(shader, "sunDirection"),
		locSunColor:       rl.GetShaderLocation(shader, "sunColor"),
		locAmbientColor:   rl.GetShaderLocation(shader, "ambientColor"),
		locFogColor:       rl.GetShaderLocation(shader, "fogColor"),
		locFogDensity:     rl.GetShaderLocation(shader, "fogDensity"),
		locSpecStrength:   rl.GetShaderLocation(shader, "specularStrength"),
		locShadowStrength: rl.GetShaderLocation(shader, "shadowStrength"),
		locNightMood:      rl.GetShaderLocation(shader, "nightMood"),
		locTorchPos:       rl.GetShaderLocation(shader, "torchPos"),
		locTorchColor:     rl.GetShaderLocation(shader, "torchColor"),
		locTorchRange:     rl.GetShaderLocation(shader, "torchRange"),
	}
	LogRenderInit("lighting locs: viewPos=%d sunDir=%d sunCol=%d amb=%d fogCol=%d fogDens=%d spec=%d shadow=%d night=%d torchPos=%d torchCol=%d torchRange=%d",
		s.locViewPos, s.locSunDirection, s.locSunColor, s.locAmbientColor, s.locFogColor, s.locFogDensity, s.locSpecStrength, s.locShadowStrength, s.locNightMood, s.locTorchPos, s.locTorchColor, s.locTorchRange)
	// Re-prime the uniform memos: GL can reuse a program ID after a reload, so an
	// ID match alone isn't proof the uniforms still hold our last values.
	lightingUniformPrimed = false
	torchRangePrimed = false
	torchSlotsZeroed = false
	return s
}

// uploadTorches pushes the active torch point lights to the shader (remaining
// slots zeroed). Call once per frame before drawing torch-lit geometry. A
// torchless area skips the slot upload once its slots are already zeroed.
func (l lightingShader) uploadTorches(torches []torchLight) {
	if l.shader.ID == 0 {
		return
	}
	// torchRange never changes — upload once (re-primes if the shader reloads).
	if !torchRangePrimed || torchRangeShaderID != l.shader.ID {
		uniformFloatBuf[0] = torchRangeWorld
		rl.SetShaderValue(l.shader, l.locTorchRange, uniformFloatBuf[:], rl.ShaderUniformFloat)
		torchRangePrimed = true
		torchRangeShaderID = l.shader.ID
		torchSlotsZeroed = false // force a fresh slot upload after a reload
	}
	// Torchless area: once the slots are zeroed, leave them (a lit area flickers
	// and re-uploads each frame, but an empty area has nothing to refresh).
	if len(torches) == 0 && torchSlotsZeroed && torchSlotsShaderID == l.shader.ID {
		return
	}
	for i := 0; i < maxTorches; i++ {
		if i < len(torches) {
			torchPosBuf[i*3] = torches[i].Pos.X
			torchPosBuf[i*3+1] = torches[i].Pos.Y
			torchPosBuf[i*3+2] = torches[i].Pos.Z
			torchColorBuf[i*3] = torches[i].Color.X
			torchColorBuf[i*3+1] = torches[i].Color.Y
			torchColorBuf[i*3+2] = torches[i].Color.Z
		} else {
			torchPosBuf[i*3], torchPosBuf[i*3+1], torchPosBuf[i*3+2] = 0, 0, 0
			torchColorBuf[i*3], torchColorBuf[i*3+1], torchColorBuf[i*3+2] = 0, 0, 0
		}
	}
	rl.SetShaderValueV(l.shader, l.locTorchPos, torchPosBuf[:], rl.ShaderUniformVec3, maxTorches)
	rl.SetShaderValueV(l.shader, l.locTorchColor, torchColorBuf[:], rl.ShaderUniformVec3, maxTorches)
	torchSlotsZeroed = len(torches) == 0
	torchSlotsShaderID = l.shader.ID
}

func (l lightingShader) unload() {
	if l.shader.ID != 0 {
		rl.UnloadShader(l.shader)
	}
}

// sunDir is the pre-normalized world-space direction the sun shines toward (the
// lighting shader's directional vector).
var sunDir = normalizeVec3(rl.NewVector3(0.42, -0.78, 0.32))

func normalizeVec3(v rl.Vector3) rl.Vector3 {
	length := float32(math.Sqrt(float64(v.X*v.X + v.Y*v.Y + v.Z*v.Z)))
	if length == 0 {
		return rl.NewVector3(0, -1, 0)
	}
	return rl.NewVector3(v.X/length, v.Y/length, v.Z/length)
}

// lightingUniform* memoize the last profile-derived upload. viewPos always
// uploads; the ~7 profile-derived uniforms only shift on a phase crossing. Keyed
// on shader ID so a reload re-primes.
var (
	lightingUniformProfile  lightingProfile
	lightingUniformShaderID uint32
	lightingUniformPrimed   bool
)

func (l lightingShader) applyUniforms(camera rl.Camera3D, ambient lightingProfile) {
	if l.shader.ID == 0 {
		return
	}
	uniformVec3Buf[0], uniformVec3Buf[1], uniformVec3Buf[2] = camera.Position.X, camera.Position.Y, camera.Position.Z
	rl.SetShaderValue(l.shader, l.locViewPos, uniformVec3Buf[:], rl.ShaderUniformVec3)
	// Remaining uniforms are constant or profile-derived; skip when profile + shader unchanged.
	if lightingUniformPrimed && lightingUniformShaderID == l.shader.ID && ambient == lightingUniformProfile {
		return
	}
	lightingUniformProfile = ambient
	lightingUniformShaderID = l.shader.ID
	lightingUniformPrimed = true
	uniformVec3Buf[0], uniformVec3Buf[1], uniformVec3Buf[2] = sunDir.X, sunDir.Y, sunDir.Z
	rl.SetShaderValue(l.shader, l.locSunDirection, uniformVec3Buf[:], rl.ShaderUniformVec3)
	uniformVec3Buf[0], uniformVec3Buf[1], uniformVec3Buf[2] = ambient.SunColor.X, ambient.SunColor.Y, ambient.SunColor.Z
	rl.SetShaderValue(l.shader, l.locSunColor, uniformVec3Buf[:], rl.ShaderUniformVec3)
	uniformVec3Buf[0], uniformVec3Buf[1], uniformVec3Buf[2] = ambient.AmbientColor.X, ambient.AmbientColor.Y, ambient.AmbientColor.Z
	rl.SetShaderValue(l.shader, l.locAmbientColor, uniformVec3Buf[:], rl.ShaderUniformVec3)
	uniformVec3Buf[0], uniformVec3Buf[1], uniformVec3Buf[2] = ambient.FogColor.X, ambient.FogColor.Y, ambient.FogColor.Z
	rl.SetShaderValue(l.shader, l.locFogColor, uniformVec3Buf[:], rl.ShaderUniformVec3)
	uniformFloatBuf[0] = ambient.FogDensity
	rl.SetShaderValue(l.shader, l.locFogDensity, uniformFloatBuf[:], rl.ShaderUniformFloat)
	uniformFloatBuf[0] = ambient.SpecularStrength
	rl.SetShaderValue(l.shader, l.locSpecStrength, uniformFloatBuf[:], rl.ShaderUniformFloat)
	uniformFloatBuf[0] = ambient.ShadowStrength
	rl.SetShaderValue(l.shader, l.locShadowStrength, uniformFloatBuf[:], rl.ShaderUniformFloat)
	uniformFloatBuf[0] = ambient.Mood
	rl.SetShaderValue(l.shader, l.locNightMood, uniformFloatBuf[:], rl.ShaderUniformFloat)
}

type lightingProfile struct {
	SunColor         rl.Vector3
	AmbientColor     rl.Vector3
	FogColor         rl.Vector3
	FogDensity       float32
	SpecularStrength float32
	ShadowStrength   float32
	// Mood is the day→night dial driving the painterly grade (0 = day, 1 = night),
	// set per frame by applyTimeOfDay (outdoors tracks star alpha; enclosed
	// dungeons pinned high). Read by the shaders as the `nightMood` uniform.
	Mood float32
}

// Per-area lighting profiles (reused, not rebuilt per frame). indoorFogThreshold
// straddles the two FogDensity values below; a fogger profile in a ceilinged area
// gets the spooky-dungeon override (daycycle.go). Co-located to keep the verdict aligned.
const indoorFogThreshold = 0.06

var (
	dungeonLighting = lightingProfile{
		// Most fields overridden by applyTimeOfDay; only FogDensity + Specular
		// survive (specular dimmed so dungeon stone doesn't catch hot highlights).
		SunColor:         rl.NewVector3(0.95, 0.86, 0.72),
		AmbientColor:     rl.NewVector3(0.22, 0.24, 0.30),
		FogColor:         rl.NewVector3(0.06, 0.07, 0.09),
		FogDensity:       0.085,
		SpecularStrength: 0.12,
		ShadowStrength:   0.45,
	}
	fieldLighting = lightingProfile{
		// Fog at 0.026 so distant trees/walls fade into storybook haze; specular
		// dropped so leaves/grass don't shimmer.
		SunColor:         rl.NewVector3(1.05, 0.99, 0.86),
		AmbientColor:     rl.NewVector3(0.46, 0.52, 0.62),
		FogColor:         rl.NewVector3(0.74, 0.86, 0.96),
		FogDensity:       0.026,
		SpecularStrength: 0.05,
		ShadowStrength:   0.30,
	}
)

// attachShader binds the lighting shader to every material on the model.
// GetMaterials aliases the model's underlying material memory, so mutating the
// slice elements writes back through the C pointer.
func attachShader(model *rl.Model, shader rl.Shader) {
	if model == nil || shader.ID == 0 {
		return
	}
	materials := model.GetMaterials()
	for i := range materials {
		materials[i].Shader = shader
	}
}

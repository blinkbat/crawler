package render

import (
	"fmt"
	"log"
	"math"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// fogCeilingToken is the placeholder string both fragment shaders
// carry where they would otherwise inline the `0.85` clamp ceiling.
// resolveShaderFogCeiling substitutes the Go fogCeiling constant in
// before LoadShaderFromMemory, so the GLSL source has the literal
// value once it reaches the compiler. Keeps the ceiling tuned from
// one place — see fogCeiling in distancefog.go.
const fogCeilingToken = "{{FOG_CEILING}}"

func resolveShaderFogCeiling(src string) string {
	return strings.ReplaceAll(src, fogCeilingToken, fmt.Sprintf("%.4f", fogCeiling))
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

// billboardFogVertexShader is the shared vertex shader for the
// distance-fogged billboard pass. Same shape as the lighting
// vertex shader, but without the normal pipe — raylib's billboard
// draw doesn't supply vertex normals so the lighting shader's
// `normalize(matNormal * vertexNormal)` would feed garbage into
// the fragment's lighting math. We only need fragPosition (to
// compute distance to camera) and fragTexCoord (to sample the
// sprite atlas).
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

// billboardFogFragmentShader is the minimal billboard pass: sample
// the texture, mix toward fogColor by the same exponential fog
// curve the world shader uses, output. No directional lighting —
// billboards face the camera so a single-direction sun would just
// flatten them anyway; the world shader does its lighting compute
// for mesh geometry, and this leaves billboards as colored sprite
// silhouettes that nevertheless fade into the fog like everything
// else around them.
//
// {{FOG_CEILING}} is substituted at shader-load time from the Go
// `fogCeiling` constant via resolveShaderFogCeiling, so the
// ceiling lives in exactly one place across Go + both shaders.
const billboardFogFragmentShader = `#version 330

in vec2 fragTexCoord;
in vec4 fragColor;
in vec3 fragPosition;

uniform sampler2D texture0;
uniform vec4 colDiffuse;

uniform vec3 viewPos;
uniform vec3 fogColor;
uniform float fogDensity;

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
    finalColor = vec4(lit, texel.a * fragColor.a * colDiffuse.a);
}
`

type billboardFogShaderPipe struct {
	shader        rl.Shader
	locViewPos    int32
	locFogColor   int32
	locFogDensity int32
}

func loadBillboardFogShader() billboardFogShaderPipe {
	shader := rl.LoadShaderFromMemory(billboardFogVertexShader, resolveShaderFogCeiling(billboardFogFragmentShader))
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
	}
	LogRenderInit("billboard fog locs: viewPos=%d fogColor=%d fogDensity=%d", pipe.locViewPos, pipe.locFogColor, pipe.locFogDensity)
	return pipe
}

func (s billboardFogShaderPipe) unload() {
	if s.shader.ID != 0 {
		rl.UnloadShader(s.shader)
	}
}

// uniformVec3Buf / uniformFloatBuf are reused across every shader-
// uniform upload so the per-frame applyUniforms paths don't allocate
// fresh []float32{...} slice literals for each Vec3 / Float. Renderer
// is single-threaded; one shared scratch per shape is safe.
var (
	uniformVec3Buf  [3]float32
	uniformFloatBuf [1]float32
)

func (s billboardFogShaderPipe) applyUniforms(camera rl.Camera3D, profile lightingProfile) {
	if s.shader.ID == 0 {
		return
	}
	uniformVec3Buf[0], uniformVec3Buf[1], uniformVec3Buf[2] = camera.Position.X, camera.Position.Y, camera.Position.Z
	rl.SetShaderValue(s.shader, s.locViewPos, uniformVec3Buf[:], rl.ShaderUniformVec3)
	uniformVec3Buf[0], uniformVec3Buf[1], uniformVec3Buf[2] = profile.FogColor.X, profile.FogColor.Y, profile.FogColor.Z
	rl.SetShaderValue(s.shader, s.locFogColor, uniformVec3Buf[:], rl.ShaderUniformVec3)
	uniformFloatBuf[0] = profile.FogDensity
	rl.SetShaderValue(s.shader, s.locFogDensity, uniformFloatBuf[:], rl.ShaderUniformFloat)
}

// Cast shadows used to live here as a separate depth pass + PCF lookup. They
// were removed because the multi-pass setup was fragile against raylib's
// implicit texture-binding behavior (DrawMesh ignores SetShaderValueTexture
// registrations, so the shadow sampler kept reading garbage). Lighting is
// now a single forward pass: directional sun + hemisphere ambient + a
// pseudo-AO term + exponential fog. Good enough for the chunky look the
// game's going for, and a lot easier to maintain.
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

    // Gentle painted shading — soft smoothstep banding that
    // still suggests the storybook cel feel, but at a much
    // lower mix so the surface stays close to the original
    // continuous wrap-diffuse. Anything stronger pushes the
    // brightness range too wide for comfortable viewing.
    float wrap = clamp((dot(N, L) + 0.25) / 1.25, 0.0, 1.0);
    float toon = smoothstep(0.18, 0.55, wrap) * 0.50
               + smoothstep(0.55, 0.85, wrap) * 0.50;
    float shade = mix(wrap, toon, 0.35);
    vec3 diffuse = sunColor * mix(NdotL, shade, 0.35);

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
    // resolveShaderFogCeiling above. ONE source of truth across Go
    // + both shaders.
    float dist = length(viewPos - fragPosition);
    float fog = 1.0 - exp(-fogDensity * dist);
    fog = clamp(fog, 0.0, {{FOG_CEILING}});
    lit = mix(lit, fogColor, fog);

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
	locTorchPos       int32
	locTorchColor     int32
	locTorchRange     int32
}

// maxTorches mirrors MAX_TORCHES in the fragment shader — the fixed
// upper bound on simultaneously-lit torches. The Go side picks the
// closest N braziers to the camera and disables the rest by zeroing
// their colour.
const maxTorches = 12

// torchRangeWorld is the world-unit reach of every torch's light
// pool, shared by all torches via the torchRange uniform. ~7 units ≈
// 3.5 tiles, so a torch lights its own room without bleeding far
// down a corridor — keeps the dungeon dark between torches.
const torchRangeWorld = float32(9.0)

// torchLight is one active torch's world position + (already
// flickered) RGB colour, handed to uploadTorches.
type torchLight struct {
	Pos   rl.Vector3
	Color rl.Vector3
}

// Reused flat upload buffers so the per-frame torch upload doesn't
// allocate. Indexed as [i*3 + channel].
var (
	torchPosBuf   [maxTorches * 3]float32
	torchColorBuf [maxTorches * 3]float32
)

func loadLightingShader() lightingShader {
	shader := rl.LoadShaderFromMemory(lightingVertexShader, resolveShaderFogCeiling(lightingFragmentShader))
	if shader.ID == 0 {
		// Compile/link failure leaves shader.ID == 0; raylib's BeginShaderMode
		// will silently no-op past that point, so the world draws unlit with
		// no other warning. One-line startup log so we don't lose the signal.
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
		locTorchPos:       rl.GetShaderLocation(shader, "torchPos"),
		locTorchColor:     rl.GetShaderLocation(shader, "torchColor"),
		locTorchRange:     rl.GetShaderLocation(shader, "torchRange"),
	}
	LogRenderInit("lighting locs: viewPos=%d sunDir=%d sunCol=%d amb=%d fogCol=%d fogDens=%d spec=%d shadow=%d torchPos=%d torchCol=%d torchRange=%d",
		s.locViewPos, s.locSunDirection, s.locSunColor, s.locAmbientColor, s.locFogColor, s.locFogDensity, s.locSpecStrength, s.locShadowStrength, s.locTorchPos, s.locTorchColor, s.locTorchRange)
	return s
}

// uploadTorches pushes the active torch point lights to the shader.
// Every slot is written each frame: active torches get their world
// position + flickered colour, the remaining slots are zeroed so
// their loop iteration contributes nothing. Call once per frame
// before drawing the world geometry that should be torch-lit.
func (l lightingShader) uploadTorches(torches []torchLight) {
	if l.shader.ID == 0 {
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
	uniformFloatBuf[0] = torchRangeWorld
	rl.SetShaderValue(l.shader, l.locTorchRange, uniformFloatBuf[:], rl.ShaderUniformFloat)
}

func (l lightingShader) unload() {
	if l.shader.ID != 0 {
		rl.UnloadShader(l.shader)
	}
}

// sunDir is the world-space direction the sun shines toward, pre-normalized
// at init time. Used as the directional light vector for the lighting shader.
var sunDir = normalizeVec3(rl.NewVector3(0.42, -0.78, 0.32))

func normalizeVec3(v rl.Vector3) rl.Vector3 {
	length := float32(math.Sqrt(float64(v.X*v.X + v.Y*v.Y + v.Z*v.Z)))
	if length == 0 {
		return rl.NewVector3(0, -1, 0)
	}
	return rl.NewVector3(v.X/length, v.Y/length, v.Z/length)
}

func (l lightingShader) applyUniforms(camera rl.Camera3D, ambient lightingProfile) {
	if l.shader.ID == 0 {
		return
	}
	uniformVec3Buf[0], uniformVec3Buf[1], uniformVec3Buf[2] = camera.Position.X, camera.Position.Y, camera.Position.Z
	rl.SetShaderValue(l.shader, l.locViewPos, uniformVec3Buf[:], rl.ShaderUniformVec3)
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
}

type lightingProfile struct {
	SunColor         rl.Vector3
	AmbientColor     rl.Vector3
	FogColor         rl.Vector3
	FogDensity       float32
	SpecularStrength float32
	ShadowStrength   float32
}

// Per-area lighting profiles, hoisted to package vars so we don't rebuild a
// fresh struct every frame. Both areas reuse the same shader.
var (
	dungeonLighting = lightingProfile{
		// Most fields here are overridden by applyTimeOfDay at
		// render time; only FogDensity and SpecularStrength
		// survive. Specular dimmed so dungeon stone doesn't
		// catch hot highlights.
		SunColor:         rl.NewVector3(0.95, 0.86, 0.72),
		AmbientColor:     rl.NewVector3(0.22, 0.24, 0.30),
		FogColor:         rl.NewVector3(0.06, 0.07, 0.09),
		FogDensity:       0.085,
		SpecularStrength: 0.12,
		ShadowStrength:   0.45,
	}
	fieldLighting = lightingProfile{
		// Field fog density bumped from 0.018 → 0.026 so distant
		// trees / walls fade into atmospheric haze the way they
		// do in painted storybook spreads, instead of all sitting
		// in sharp focus. Specular dropped so leaves and grass
		// don't shimmer.
		SunColor:         rl.NewVector3(1.05, 0.99, 0.86),
		AmbientColor:     rl.NewVector3(0.46, 0.52, 0.62),
		FogColor:         rl.NewVector3(0.74, 0.86, 0.96),
		FogDensity:       0.026,
		SpecularStrength: 0.05,
		ShadowStrength:   0.30,
	}
)

// attachShader binds the lighting shader to every material on the model so the
// model is rendered through it (instead of raylib's default flat shader).
// GetMaterials returns a slice that aliases the model's underlying material
// memory, so mutating the slice elements writes back through the C pointer.
func attachShader(model *rl.Model, shader rl.Shader) {
	if model == nil || shader.ID == 0 {
		return
	}
	materials := model.GetMaterials()
	for i := range materials {
		materials[i].Shader = shader
	}
}

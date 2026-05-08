package render

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
)

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

out vec4 finalColor;

void main() {
    vec4 texel = texture(texture0, fragTexCoord);
    vec3 base = texel.rgb * fragColor.rgb * colDiffuse.rgb;

    vec3 N = normalize(fragNormal);
    vec3 L = normalize(-sunDirection);
    vec3 V = normalize(viewPos - fragPosition);
    vec3 H = normalize(L + V);

    float NdotL = max(dot(N, L), 0.0);

    // Hemisphere ambient: sky tint above, ground tint below
    float upDot = N.y * 0.5 + 0.5;
    vec3 hemi = mix(ambientColor * 0.65, ambientColor, upDot);

    // Wrap-around for slightly softer shading
    float wrap = clamp((dot(N, L) + 0.25) / 1.25, 0.0, 1.0);
    vec3 diffuse = sunColor * mix(NdotL, wrap, 0.35);

    float spec = 0.0;
    if (specularStrength > 0.001 && NdotL > 0.0) {
        spec = pow(max(dot(N, H), 0.0), 26.0) * specularStrength;
    }

    // Pseudo-AO: darken slightly where surface points away from sun. With
    // cast shadows gone this is the only thing that gives shaded areas
    // visual depth, so shadowStrength tunes "how dark do shadowed-feeling
    // surfaces get."
    float ao = mix(1.0 - shadowStrength, 1.0, smoothstep(-0.1, 0.6, dot(N, L)));

    vec3 lit = base * (hemi + diffuse) * ao + sunColor * spec;

    // Exponential height-aware fog
    float dist = length(viewPos - fragPosition);
    float fog = 1.0 - exp(-fogDensity * dist);
    fog = clamp(fog, 0.0, 0.85);
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
}

func loadLightingShader() lightingShader {
	shader := rl.LoadShaderFromMemory(lightingVertexShader, lightingFragmentShader)
	return lightingShader{
		shader:            shader,
		locViewPos:        rl.GetShaderLocation(shader, "viewPos"),
		locSunDirection:   rl.GetShaderLocation(shader, "sunDirection"),
		locSunColor:       rl.GetShaderLocation(shader, "sunColor"),
		locAmbientColor:   rl.GetShaderLocation(shader, "ambientColor"),
		locFogColor:       rl.GetShaderLocation(shader, "fogColor"),
		locFogDensity:     rl.GetShaderLocation(shader, "fogDensity"),
		locSpecStrength:   rl.GetShaderLocation(shader, "specularStrength"),
		locShadowStrength: rl.GetShaderLocation(shader, "shadowStrength"),
	}
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
	rl.SetShaderValue(l.shader, l.locViewPos, []float32{camera.Position.X, camera.Position.Y, camera.Position.Z}, rl.ShaderUniformVec3)
	rl.SetShaderValue(l.shader, l.locSunDirection, []float32{sunDir.X, sunDir.Y, sunDir.Z}, rl.ShaderUniformVec3)
	rl.SetShaderValue(l.shader, l.locSunColor, []float32{ambient.SunColor.X, ambient.SunColor.Y, ambient.SunColor.Z}, rl.ShaderUniformVec3)
	rl.SetShaderValue(l.shader, l.locAmbientColor, []float32{ambient.AmbientColor.X, ambient.AmbientColor.Y, ambient.AmbientColor.Z}, rl.ShaderUniformVec3)
	rl.SetShaderValue(l.shader, l.locFogColor, []float32{ambient.FogColor.X, ambient.FogColor.Y, ambient.FogColor.Z}, rl.ShaderUniformVec3)
	rl.SetShaderValue(l.shader, l.locFogDensity, []float32{ambient.FogDensity}, rl.ShaderUniformFloat)
	rl.SetShaderValue(l.shader, l.locSpecStrength, []float32{ambient.SpecularStrength}, rl.ShaderUniformFloat)
	rl.SetShaderValue(l.shader, l.locShadowStrength, []float32{ambient.ShadowStrength}, rl.ShaderUniformFloat)
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
		SunColor:         rl.NewVector3(0.95, 0.86, 0.72),
		AmbientColor:     rl.NewVector3(0.22, 0.24, 0.30),
		FogColor:         rl.NewVector3(0.06, 0.07, 0.09),
		FogDensity:       0.085,
		SpecularStrength: 0.22,
		ShadowStrength:   0.45,
	}
	fieldLighting = lightingProfile{
		SunColor:         rl.NewVector3(1.05, 0.99, 0.86),
		AmbientColor:     rl.NewVector3(0.46, 0.52, 0.62),
		FogColor:         rl.NewVector3(0.74, 0.86, 0.96),
		FogDensity:       0.018,
		SpecularStrength: 0.10,
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

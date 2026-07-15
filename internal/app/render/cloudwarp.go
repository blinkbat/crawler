package render

import (
	rl "github.com/gen2brain/raylib-go/raylib"
)

// Cloud domain-warp: the two cumulus planes (DrawSkyBackground) are sampled
// through a slowly churning screen-space flow field so the clouds gently billow
// and change shape as they drift, instead of sliding as rigid paintings. The
// field is a cheap two-octave sinusoid plasma — a few sin() per fragment over
// two full-screen quads, so the cost is negligible. Screen-space (not sample-UV
// space) on purpose: the warp is independent of the scrolling source rect, so it
// never pops when the cloud UV wraps every few minutes. Subtle by design — small
// amplitude, low frequencies, so the field reshapes over tens of seconds.
const cloudWarpFragmentShader = `
#version 330
in vec2 fragTexCoord;
in vec4 fragColor;
uniform sampler2D texture0;
uniform vec4 colDiffuse;
uniform float uTime;       // seconds (rl.GetTime)
uniform vec2  uResolution; // screen px, to normalize gl_FragCoord
uniform float uWarpAmp;    // UV-space displacement (small = subtle)
uniform float uWarpScale;  // flow-field spatial frequency (cells across screen)
uniform float uPhase;      // decorrelates the two cloud planes
out vec4 finalColor;

void main() {
    // Screen-space flow coordinate — independent of the scrolling sample UV, so
    // the warp field never jumps when the cloud source rect wraps.
    vec2 s = (gl_FragCoord.xy / uResolution) * uWarpScale + uPhase;
    float t = uTime;
    // Two rotated sinusoid octaves → an organic, non-obviously-repeating swirl.
    // Frequencies kept low (t*0.05..0.11 rad/s) so the whole field reshapes over
    // ~a minute; the cross terms (s.x/s.y coupling) bend it into slow curls.
    vec2 flow;
    flow.x = sin(s.y * 1.7 + t * 0.110) + 0.5 * sin(s.y * 3.6 - t * 0.067 + s.x * 1.3);
    flow.y = sin(s.x * 1.9 - t * 0.090) + 0.5 * sin(s.x * 3.2 + t * 0.053 + s.y * 1.3);
    vec4 texel = texture(texture0, fragTexCoord + uWarpAmp * flow);
    finalColor = texel * fragColor * colDiffuse;
}
`

type cloudWarpShaderPipe struct {
	shader        rl.Shader
	locTime       int32
	locResolution int32
	locWarpAmp    int32
	locWarpScale  int32
	locPhase      int32
}

// cloudWarpLayer is one plane's warp tuning. The near plane (big crisp cumulus)
// churns a touch harder and finer than the hazy far plane; the phase offset
// decorrelates the two so they don't billow in lockstep. TUNABLES — raise amp
// for a stronger trip, raise scale for more localized bulging (vs broad wobble).
type cloudWarpLayer struct {
	amp, scale, phase float32
}

var (
	cloudWarpFar  = cloudWarpLayer{amp: 0.005, scale: 2.8, phase: 0.0}
	cloudWarpNear = cloudWarpLayer{amp: 0.008, scale: 4.2, phase: 17.3}
)

// loadCloudWarpShader compiles the warp shader (default vertex shader + custom
// fragment). A failed compile returns a zero pipe; DrawSkyBackground checks
// shader.ID and falls back to an undistorted cloud draw.
func loadCloudWarpShader() cloudWarpShaderPipe {
	shader := rl.LoadShaderFromMemory("", cloudWarpFragmentShader)
	if shader.ID == 0 {
		LogRenderError("cloud warp shader compile FAILED (shader.ID==0); clouds will draw undistorted")
		return cloudWarpShaderPipe{}
	}
	LogRenderInit("cloud warp shader compiled OK (shader.ID=%d)", shader.ID)
	return cloudWarpShaderPipe{
		shader:        shader,
		locTime:       rl.GetShaderLocation(shader, "uTime"),
		locResolution: rl.GetShaderLocation(shader, "uResolution"),
		locWarpAmp:    rl.GetShaderLocation(shader, "uWarpAmp"),
		locWarpScale:  rl.GetShaderLocation(shader, "uWarpScale"),
		locPhase:      rl.GetShaderLocation(shader, "uPhase"),
	}
}

func (s cloudWarpShaderPipe) unload() {
	if s.shader.ID != 0 {
		rl.UnloadShader(s.shader)
	}
}

// begin binds the warp shader and uploads the per-frame time + resolution. Pair
// with rl.EndShaderMode(); set each plane via layer() before its draw.
func (s cloudWarpShaderPipe) begin(screenW, screenH, now float32) {
	rl.BeginShaderMode(s.shader)
	setShaderFloat(s.shader, s.locTime, now)
	setShaderVec2(s.shader, s.locResolution, screenW, screenH)
}

// layer uploads one plane's warp params. The caller MUST flush the batch
// (rl.DrawRenderBatchActive) between the two planes' draws — else this second
// upload retroactively applies to the first plane's still-batched vertices.
func (s cloudWarpShaderPipe) layer(l cloudWarpLayer) {
	setShaderFloat(s.shader, s.locWarpAmp, l.amp)
	setShaderFloat(s.shader, s.locWarpScale, l.scale)
	setShaderFloat(s.shader, s.locPhase, l.phase)
}

package render

import (
	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// timeProfile is the time-of-day half of the lighting equation. The area
// profile (dungeonLighting / fieldLighting) supplies fog density and
// specular strength — those don't move with the sun — and the time
// profile overrides sun, ambient, and fog colors plus shadow strength.
// Six profiles are sampled at the phase boundaries; runtime interpolates
// between adjacent ones using PhaseAtStep's progress value so the
// transition is smooth instead of snapping every 25 steps.
type timeProfile struct {
	SunColor       rl.Vector3
	AmbientColor   rl.Vector3
	FogColor       rl.Vector3
	ShadowStrength float32
	// SkyTint multiplies the sky texture. Components above 1 brighten;
	// below 1 darken. Used by DrawSkyBackground so the backdrop tracks
	// the same arc as in-world lighting.
	SkyTint rl.Vector3
	// StarAlpha is the 0..1 visibility of the star overlay drawn on
	// top of the sky. 0 hides the layer (daylight phases); 1 paints
	// it at full transparency-mapped strength (midnight). Interpolated
	// across the phase boundary like the other fields so the field
	// fades in / out smoothly instead of popping per-step.
	StarAlpha float32
}

// timeProfiles indexed by core.TimeOfDay. Sized via core.TimeOfDayCount so
// the compiler enforces the parallel-array contract with the enum — adding
// a phase to core.TimeOfDay forces a row added here too instead of a
// silent OOB at runtime.
var timeProfiles = [core.TimeOfDayCount]timeProfile{
	// Dawn — cool low light with a warm horizon. Sun is rising, so the
	// directional component is dim and warm-tinted; ambient still carries
	// the cool of night. Early dawn is intentionally a notch darker than
	// the previous tuning so the transition out of midnight still reads
	// as "barely-lit pre-sunrise" before brightening into morning.
	core.Dawn: {
		SunColor:     rl.NewVector3(0.92, 0.52, 0.38),
		AmbientColor: rl.NewVector3(0.32, 0.28, 0.36),
		FogColor:     rl.NewVector3(0.78, 0.52, 0.46),
		// Sunrise tint pushed hard pink-orange so the horizon reads
		// as a sunrise instead of "muddy pre-morning." Interpolation
		// toward Morning's white tint still gives a smooth crossover;
		// the dramatic boundary is what sells the time-of-day arc.
		ShadowStrength: 0.48,
		SkyTint:        rl.NewVector3(1.18, 0.72, 0.58),
		// Stars linger faintly at dawn — fading through to 0 as
		// timeProfileAt blends toward Morning, then stay invisible
		// across the daylight phases. Bumped from 0.18 so the last
		// constellations are still visible at the brightest dawn
		// frame, not pinned just under threshold.
		StarAlpha: 0.32,
	},
	// Morning — bright, slightly warm. Closest to a "default" daylight
	// look; the area's base profile (which was tuned originally) shows
	// through best here.
	core.Morning: {
		SunColor:       rl.NewVector3(1.05, 0.99, 0.86),
		AmbientColor:   rl.NewVector3(0.46, 0.52, 0.62),
		FogColor:       rl.NewVector3(0.74, 0.86, 0.96),
		ShadowStrength: 0.30,
		SkyTint:        rl.NewVector3(1.00, 1.00, 1.00),
		StarAlpha:      0,
	},
	// Afternoon — peak brightness, slightly cooler than morning to give
	// the day an arc instead of a flat plateau.
	core.Afternoon: {
		SunColor:       rl.NewVector3(1.10, 1.04, 0.92),
		AmbientColor:   rl.NewVector3(0.52, 0.58, 0.68),
		FogColor:       rl.NewVector3(0.80, 0.90, 0.98),
		ShadowStrength: 0.28,
		SkyTint:        rl.NewVector3(1.05, 1.08, 1.10),
		StarAlpha:      0,
	},
	// Dusk — sun low, deep gold sky, long warm shadows. This is the
	// most stylized phase; intentionally exaggerated so the player
	// notices the transition.
	core.Dusk: {
		// Sunset: pushed deep gold + a touch of magenta-red so the
		// horizon reads as a proper sunset, not a faded afternoon.
		// Components above 1 brighten R; G stays warm but lower; B
		// kept low to keep the sky from going neutral. Strong shadow
		// strength accentuates the long-shadow feel.
		SunColor:       rl.NewVector3(1.32, 0.68, 0.34),
		AmbientColor:   rl.NewVector3(0.42, 0.32, 0.32),
		FogColor:       rl.NewVector3(0.78, 0.46, 0.36),
		ShadowStrength: 0.50,
		SkyTint:        rl.NewVector3(1.28, 0.58, 0.34),
		// Stars start to peek toward late dusk — bumped from 0.04 so
		// the layer is actually perceptible at the Dusk→Evening
		// crossover, where the sky is already darkening enough for
		// the pinpoints to read.
		StarAlpha: 0.12,
	},
	// Evening — indigo twilight. Sun has set; the only color is what
	// the sky and lingering atmosphere bounce around.
	core.Evening: {
		SunColor:       rl.NewVector3(0.45, 0.46, 0.66),
		AmbientColor:   rl.NewVector3(0.18, 0.22, 0.34),
		FogColor:       rl.NewVector3(0.18, 0.22, 0.32),
		ShadowStrength: 0.55,
		SkyTint:        rl.NewVector3(0.36, 0.40, 0.62),
		StarAlpha:      0.55,
	},
	// Midnight — moonlit blue. Very dark, but enough light to read by;
	// shadowStrength is highest here so unlit surfaces really sink.
	core.Midnight: {
		SunColor:       rl.NewVector3(0.20, 0.24, 0.46),
		AmbientColor:   rl.NewVector3(0.10, 0.13, 0.24),
		FogColor:       rl.NewVector3(0.06, 0.08, 0.16),
		ShadowStrength: 0.65,
		SkyTint:        rl.NewVector3(0.14, 0.18, 0.34),
		StarAlpha:      1.0,
	},
}

// timeProfileAt samples the cycle at the given step count and returns the
// blended profile between the current phase and the next. Wraps at the
// midnight→dawn boundary so the loop is seamless.
func timeProfileAt(steps int) timeProfile {
	phase, p := core.PhaseAtStep(steps)
	cur := timeProfiles[phase]
	next := timeProfiles[(int(phase)+1)%len(timeProfiles)]
	return timeProfile{
		SunColor:       lerpVec3(cur.SunColor, next.SunColor, p),
		AmbientColor:   lerpVec3(cur.AmbientColor, next.AmbientColor, p),
		FogColor:       lerpVec3(cur.FogColor, next.FogColor, p),
		ShadowStrength: cur.ShadowStrength + (next.ShadowStrength-cur.ShadowStrength)*p,
		SkyTint:        lerpVec3(cur.SkyTint, next.SkyTint, p),
		StarAlpha:      cur.StarAlpha + (next.StarAlpha-cur.StarAlpha)*p,
	}
}

// skyColor converts a SkyTint vector to a clamped 0–255 RGBA color usable
// by rl.DrawTexturePro. Mirrors how the lighting shader clamps inside the
// fragment program.
func skyColor(tint rl.Vector3) rl.Color {
	clamp := func(v float32) uint8 {
		v *= 255
		if v < 0 {
			return 0
		}
		if v > 255 {
			return 255
		}
		return uint8(v)
	}
	return rl.NewColor(clamp(tint.X), clamp(tint.Y), clamp(tint.Z), 255)
}

// applyTimeOfDay overlays the time-of-day overrides onto a base area
// profile, returning the runtime profile passed to the lighting shader.
// Fog density and specular strength come from the area; everything else
// comes from the time of day.
func applyTimeOfDay(base lightingProfile, t timeProfile) lightingProfile {
	return lightingProfile{
		SunColor:         t.SunColor,
		AmbientColor:     t.AmbientColor,
		FogColor:         t.FogColor,
		FogDensity:       base.FogDensity,
		SpecularStrength: base.SpecularStrength,
		ShadowStrength:   t.ShadowStrength,
	}
}

func lerpVec3(a, b rl.Vector3, t float32) rl.Vector3 {
	return rl.NewVector3(
		a.X+(b.X-a.X)*t,
		a.Y+(b.Y-a.Y)*t,
		a.Z+(b.Z-a.Z)*t,
	)
}

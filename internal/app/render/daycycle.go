package render

import (
	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// timeProfile is the time-of-day half of the lighting equation: it overrides
// sun/ambient/fog colors + shadow strength, while the area profile supplies fog
// density + specular. Six profiles sampled at phase boundaries; runtime
// interpolates between adjacent ones (PhaseAtStep progress) for a smooth transition.
type timeProfile struct {
	SunColor       rl.Vector3
	AmbientColor   rl.Vector3
	FogColor       rl.Vector3
	ShadowStrength float32
	// SkyTint multiplies the sky texture (>1 brighten, <1 darken). Used by
	// DrawSkyBackground so the backdrop tracks in-world lighting.
	SkyTint rl.Vector3
	// StarAlpha is the 0..1 star-overlay visibility (0 = day, 1 = midnight),
	// interpolated across the boundary so it fades in/out smoothly.
	StarAlpha float32
}

// timeProfiles indexed by core.TimeOfDay, sized via core.TimeOfDayCount so a new
// phase forces a row here instead of a silent runtime OOB.
var timeProfiles = [core.TimeOfDayCount]timeProfile{
	// Dawn — cool low light, warm horizon; a notch darker than morning so the
	// exit from midnight reads as barely-lit pre-sunrise.
	core.Dawn: {
		SunColor:       rl.NewVector3(0.78, 0.50, 0.40),
		AmbientColor:   rl.NewVector3(0.30, 0.28, 0.34),
		FogColor:       rl.NewVector3(0.68, 0.50, 0.46),
		ShadowStrength: 0.48,
		SkyTint:        rl.NewVector3(0.96, 0.72, 0.62),
		StarAlpha:      0.32, // last constellations at brightest dawn
	},
	// Morning — relaxing pastel daylight. Sun+ambient kept near 1.0 so the
	// directional light SHADES the already-pastel base rather than searing it.
	core.Morning: {
		SunColor:       rl.NewVector3(0.60, 0.58, 0.52),
		AmbientColor:   rl.NewVector3(0.36, 0.40, 0.47),
		FogColor:       rl.NewVector3(0.84, 0.88, 0.92),
		ShadowStrength: 0.26,
		SkyTint:        rl.NewVector3(0.84, 0.86, 0.90),
		StarAlpha:      0,
	},
	// Afternoon — peak day: a touch brighter/cooler than morning for an arc, but
	// sun+ambient still near 1.0 so the pastel field doesn't blow out.
	core.Afternoon: {
		SunColor:       rl.NewVector3(0.63, 0.61, 0.54),
		AmbientColor:   rl.NewVector3(0.38, 0.41, 0.48),
		FogColor:       rl.NewVector3(0.86, 0.90, 0.94),
		ShadowStrength: 0.24,
		SkyTint:        rl.NewVector3(0.88, 0.90, 0.94),
		StarAlpha:      0,
	},
	// Dusk — sun low, deep gold sky, long warm shadows. The most stylized phase,
	// exaggerated so the player notices the transition.
	core.Dusk: {
		SunColor:       rl.NewVector3(0.94, 0.62, 0.38),
		AmbientColor:   rl.NewVector3(0.38, 0.30, 0.32),
		FogColor:       rl.NewVector3(0.70, 0.46, 0.38),
		ShadowStrength: 0.50,
		SkyTint:        rl.NewVector3(0.94, 0.56, 0.36),
		StarAlpha:      0.14, // stars start to peek
	},
	// Evening — spooky indigo twilight: light drops hard and cool so the pastel
	// warmth turns uneasy; high shadow strength sinks unlit surfaces into blue.
	core.Evening: {
		SunColor:       rl.NewVector3(0.32, 0.34, 0.52),
		AmbientColor:   rl.NewVector3(0.14, 0.18, 0.28),
		FogColor:       rl.NewVector3(0.14, 0.18, 0.28),
		ShadowStrength: 0.68,
		SkyTint:        rl.NewVector3(0.28, 0.32, 0.52),
		StarAlpha:      0.62,
	},
	// Midnight — deep moonlit gloom: cold dim moonlight, near-black ambient,
	// smothering indigo fog so far props vanish into the night.
	core.Midnight: {
		SunColor:       rl.NewVector3(0.16, 0.22, 0.40),
		AmbientColor:   rl.NewVector3(0.06, 0.10, 0.18),
		FogColor:       rl.NewVector3(0.04, 0.06, 0.12),
		ShadowStrength: 0.78,
		SkyTint:        rl.NewVector3(0.10, 0.14, 0.26),
		StarAlpha:      1.0,
	},
}

// timeProfileCache memoizes the last (steps → profile) result. The profile is a
// pure function of step count, but DrawSkyBackground and DrawWorld each sample it
// per frame, so the cache halves the lerp blends. Keyed on steps; order-independent.
var timeProfileCache struct {
	steps  int
	primed bool
	prof   timeProfile
}

// timeProfileAt returns the profile blended between the current phase and next,
// wrapping at midnight→dawn. Memoized per step count.
func timeProfileAt(steps int) timeProfile {
	if timeProfileCache.primed && timeProfileCache.steps == steps {
		return timeProfileCache.prof
	}
	phase, p := core.PhaseAtStep(steps)
	cur := timeProfiles[phase]
	next := timeProfiles[(int(phase)+1)%len(timeProfiles)]
	prof := timeProfile{
		SunColor:       lerpVec3(cur.SunColor, next.SunColor, p),
		AmbientColor:   lerpVec3(cur.AmbientColor, next.AmbientColor, p),
		FogColor:       lerpVec3(cur.FogColor, next.FogColor, p),
		ShadowStrength: core.Lerp(cur.ShadowStrength, next.ShadowStrength, p),
		SkyTint:        lerpVec3(cur.SkyTint, next.SkyTint, p),
		StarAlpha:      core.Lerp(cur.StarAlpha, next.StarAlpha, p),
	}
	timeProfileCache.steps = steps
	timeProfileCache.prof = prof
	timeProfileCache.primed = true
	return prof
}

// skyColor converts a SkyTint vector to a clamped RGBA color via the shared
// toByte, so the float→byte conversion matches every other color path.
func skyColor(tint rl.Vector3) rl.Color {
	return rl.NewColor(toByte(tint.X), toByte(tint.Y), toByte(tint.Z), 255)
}

// applyTimeOfDay overlays the time-of-day overrides onto a base area profile,
// returning the runtime profile for the lighting shader (fog density + specular
// from the area, everything else from the time). Enclosed dungeons get a
// permanent gloomy override (below); outdoor fields pass through.
func applyTimeOfDay(base lightingProfile, t timeProfile, enclosed bool) lightingProfile {
	sun := t.SunColor
	ambient := t.AmbientColor
	fog := t.FogColor
	shadow := t.ShadowStrength
	// Mood drives the painterly grade; outdoors it tracks StarAlpha (0 day → 1
	// midnight) so grade and stars rise together. Enclosed dungeons pin it below.
	mood := t.StarAlpha
	// Spooky-dungeon override only for actually enclosed areas (dense fog AND a
	// ceiling) — a stone-walled forest with open sky is NOT enclosed and keeps the
	// day cycle (the "forest dawn darker than field dawn" fix).
	if base.FogDensity > indoorFogThreshold && enclosed {
		// Dark but navigable: low cool fill + dim (not pitch-black) ambient so a
		// torchless dungeon is still playable; brazier pools add warm light on top.
		sun = rl.NewVector3(0.13, 0.14, 0.18)
		ambient = rl.NewVector3(0.12, 0.13, 0.17)
		fog = rl.NewVector3(0.03, 0.04, 0.06)
		shadow = 0.80
		// Floor mood uneasy but not fully maxed, so warm torch pools still glow
		// against the cold grade. Take the higher of night-outside and this floor.
		if mood < 0.7 {
			mood = 0.7
		}
	}
	return lightingProfile{
		SunColor:         sun,
		AmbientColor:     ambient,
		FogColor:         fog,
		FogDensity:       base.FogDensity,
		SpecularStrength: base.SpecularStrength,
		ShadowStrength:   shadow,
		Mood:             mood,
	}
}

func lerpVec3(a, b rl.Vector3, t float32) rl.Vector3 {
	return rl.NewVector3(
		core.Lerp(a.X, b.X, t),
		core.Lerp(a.Y, b.Y, t),
		core.Lerp(a.Z, b.Z, t),
	)
}

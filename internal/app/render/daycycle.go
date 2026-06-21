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
		// Sunrise — softer than the prior "hot pink-orange" pass.
		// The sky tint now stays in 0..1 so it tints the texture
		// rather than over-brightening it.
		SunColor:       rl.NewVector3(0.78, 0.50, 0.40),
		AmbientColor:   rl.NewVector3(0.30, 0.28, 0.34),
		FogColor:       rl.NewVector3(0.68, 0.50, 0.46),
		ShadowStrength: 0.48,
		SkyTint:        rl.NewVector3(0.96, 0.72, 0.62),
		// Last constellations still visible at brightest dawn.
		StarAlpha: 0.32,
	},
	// Morning — gentle warm daylight. Sun multipliers pulled below
	// 1.0 so the directional light no longer brightens textures past
	// their base — the world reads "painted dawn-after" rather than
	// "high-noon-stage-light." Ambient kept warm enough to lift
	// shadows without going neutral grey.
	// Morning — relaxing pastel daylight. The textures are already
	// pastel-bright, so the sun + ambient sum is kept near 1.0:
	// the directional light SHADES the pastel base rather than
	// pushing it past white. Any higher and the meadow sears.
	core.Morning: {
		SunColor:       rl.NewVector3(0.60, 0.58, 0.52),
		AmbientColor:   rl.NewVector3(0.36, 0.40, 0.47),
		FogColor:       rl.NewVector3(0.84, 0.88, 0.92),
		ShadowStrength: 0.26,
		SkyTint:        rl.NewVector3(0.84, 0.86, 0.90),
		StarAlpha:      0,
	},
	// Afternoon — peak relaxing-day. A touch brighter and cooler
	// than morning so the day has an arc, but the sun+ambient sum
	// still lands close to 1.0 to keep the pastel field from
	// blowing out at the brightest point.
	core.Afternoon: {
		SunColor:       rl.NewVector3(0.63, 0.61, 0.54),
		AmbientColor:   rl.NewVector3(0.38, 0.41, 0.48),
		FogColor:       rl.NewVector3(0.86, 0.90, 0.94),
		ShadowStrength: 0.24,
		SkyTint:        rl.NewVector3(0.88, 0.90, 0.94),
		StarAlpha:      0,
	},
	// Dusk — sun low, deep gold sky, long warm shadows. This is the
	// most stylized phase; intentionally exaggerated so the player
	// notices the transition.
	core.Dusk: {
		// Sunset — gold + warm red, but no longer pushed past 1.0.
		// The sky still reads "proper sunset" via the colour
		// ratios (warm R, lower G, dim B) without over-brightening
		// any single channel.
		SunColor:       rl.NewVector3(0.94, 0.62, 0.38),
		AmbientColor:   rl.NewVector3(0.38, 0.30, 0.32),
		FogColor:       rl.NewVector3(0.70, 0.46, 0.38),
		ShadowStrength: 0.50,
		SkyTint:        rl.NewVector3(0.94, 0.56, 0.36),
		// Stars start to peek toward late dusk.
		StarAlpha: 0.14,
	},
	// Evening — properly spooky indigo twilight. Light drops
	// hard and goes cool-blue so the meadow's pastel warmth
	// becomes uneasy and shadowed. Shadow strength is high so
	// unlit surfaces sink into deep blue dusk.
	core.Evening: {
		SunColor:       rl.NewVector3(0.32, 0.34, 0.52),
		AmbientColor:   rl.NewVector3(0.14, 0.18, 0.28),
		FogColor:       rl.NewVector3(0.14, 0.18, 0.28),
		ShadowStrength: 0.68,
		SkyTint:        rl.NewVector3(0.28, 0.32, 0.52),
		StarAlpha:      0.62,
	},
	// Midnight — deep moonlit gloom. Cold cyan moonlight at low
	// intensity, near-black ambient, fog drops to a smothering
	// indigo so far props vanish into the night. The pastel
	// daytime palette of the textures recedes; the player feels
	// like they shouldn't be out here.
	core.Midnight: {
		SunColor:       rl.NewVector3(0.16, 0.22, 0.40),
		AmbientColor:   rl.NewVector3(0.06, 0.10, 0.18),
		FogColor:       rl.NewVector3(0.04, 0.06, 0.12),
		ShadowStrength: 0.78,
		SkyTint:        rl.NewVector3(0.10, 0.14, 0.26),
		StarAlpha:      1.0,
	},
}

// timeProfileCache memoizes the last (steps → profile) result. The
// profile is a pure function of the step count, which is constant within
// a frame, but DrawSkyBackground and DrawWorld each sample it once per
// frame — so without the cache the six lerp blends run twice every
// frame. Keyed on steps so it stays correct across the player taking a
// step (the only thing that changes the profile); order-independent, so
// it works whether the sky or the world draws first.
var timeProfileCache struct {
	steps  int
	primed bool
	prof   timeProfile
}

// timeProfileAt samples the cycle at the given step count and returns the
// blended profile between the current phase and the next. Wraps at the
// midnight→dawn boundary so the loop is seamless. Memoized per step count
// (see timeProfileCache).
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

// skyColor converts a SkyTint vector to a clamped 0–255 RGBA color usable
// by rl.DrawTexturePro. Mirrors how the lighting shader clamps inside the
// fragment program. Uses the shared toByte (rounded, clamped 0..1→byte) so the
// float→byte conversion matches every other color path in the package.
func skyColor(tint rl.Vector3) rl.Color {
	return rl.NewColor(toByte(tint.X), toByte(tint.Y), toByte(tint.Z), 255)
}

// applyTimeOfDay overlays the time-of-day overrides onto a base area
// profile, returning the runtime profile passed to the lighting shader.
// Fog density and specular strength come from the area; everything else
// comes from the time of day.
//
// Indoor / dungeon areas (identified by their dense base fog) get
// their daytime lighting crushed into a permanent gloomy palette —
// the dungeon should feel spooky whether it's morning or midnight
// outside. We pull SunColor and AmbientColor toward a cool dim
// torchlight family that reads as "underground, no sky," and lean
// ShadowStrength heavier so unlit surfaces sink into the dark.
// Outdoor field areas (light fog) pass through unchanged so the
// pastel meadow + day/night arc still works above ground.
func applyTimeOfDay(base lightingProfile, t timeProfile, enclosed bool) lightingProfile {
	sun := t.SunColor
	ambient := t.AmbientColor
	fog := t.FogColor
	shadow := t.ShadowStrength
	// Mood drives the painterly grade (serene by day → spooky at night).
	// Outdoors it tracks StarAlpha, which already eases 0 (bright day) → 1
	// (midnight) across the cycle, so the grade and the star field rise
	// together. Enclosed dungeons get pinned uneasy below — set there.
	mood := t.StarAlpha
	// Spooky-dungeon override applies ONLY to actually enclosed
	// areas — dungeon material set (dense fog) AND a real ceiling
	// overhead. A forest authored on the dungeon palette (stone
	// walls, but open sky / no ceiling) is NOT enclosed, so it
	// keeps the normal day-cycle lighting instead of being crushed
	// to torchlit gloom. This is the fix for "forest dawn way
	// darker than field dawn."
	if base.FogDensity > indoorFogThreshold && enclosed {
		// Dark, but navigable. The directional sun is a low cool
		// fill so walls read as silhouettes; ambient is dim but
		// not pitch-black, so a dungeon with no torches is still
		// playable. Brazier torch point lights add warm pools on
		// top. Fog stays deep so anything past a torch's reach
		// recedes into the dark.
		sun = rl.NewVector3(0.13, 0.14, 0.18)
		ambient = rl.NewVector3(0.12, 0.13, 0.17)
		fog = rl.NewVector3(0.03, 0.04, 0.06)
		shadow = 0.80
		// Underground reads spooky regardless of the surface clock — but
		// not fully maxed, so the warm brazier torch pools still glow against
		// the cold grade rather than being desaturated away. Take the higher
		// of "it's also night outside" and this floor.
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

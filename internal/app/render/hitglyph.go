package render

import (
	"math"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Hit glyphs: short-lived 2D VECTOR shapes drawn over a struck target during an
// attack — a clarity cue (what kind of blow landed) layered on top of the 3D
// particle burst. Unlike the particles (3D billboards, depth-sorted in the
// world pass), a glyph is crisp screen-space line/arc/ring art drawn in the HUD
// pass, projected from the target's world position.
//
// Glyphs are presentation-only (like the particle pool) — they live here in the
// render package, NOT on GameState. They're spawned from the SAME VFX requests
// the apply handlers already emit (spawnFromRequest → spawnHitGlyph), with the
// glyph style derived from the VFX kind, so every existing damaging hit
// (player→foe and enemy→party) lights one up with zero apply-handler wiring.
//
// NOTE: a basic attack's glyph now keys off the weapon class — edged weapons
// emit VFXSlash (slash glyph), while unarmed fists / blunt club+hammer / ranged
// strikes emit VFXImpact (impact glyph), via core.WeaponHitVFX at the apply
// site. Enemy basic melee (claw/bite/slam) emits VFXImpact on the struck party
// member, and the Stone Golem's slam uses VFXStoneslam — all three share the
// impact glyph. The bladed melee SKILLS (Swipe/Backstab/Whirlwind/Crushing
// Blow) still emit VFXSlash via vfxKindFor.

type hitGlyphKind uint8

const (
	glyphNone hitGlyphKind = iota
	glyphSlash
	glyphImpact
	glyphFrost
	glyphSpark
	glyphFire
	glyphHoly
	glyphVenom
)

const (
	hitGlyphCap      = 32
	hitGlyphDuration = float32(0.42)
	hitGlyphRise     = float32(0.5) // world-unit lift from the anchor to torso height
	hitGlyphRadius   = float32(26)  // base screen radius in px
	// partyGlyphExtraRise lifts a party-anchored glyph above the base torso rise.
	// Party billboards sit low and near the camera (partySpritePosition Y≈0.62),
	// so they project into the lower screen where the battle HUD ribbon sits;
	// the extra lift floats the glyph above the member's head, clear of the HUD,
	// so an incoming-hit cue on the party is actually visible (enemy glyphs sit
	// high already and need no extra lift).
	partyGlyphExtraRise = float32(0.42)
)

// hitGlyphForVFX maps an impact VFX kind to its clarity glyph. Heal / status /
// utility VFX (Heal, Steal, Scan, Sleep, Web, Confuse, Ingest, Death) return
// glyphNone — they aren't damaging hits, so they get no glyph. Single source
// for the kind→glyph mapping; a new attack VFX lights a glyph by adding a row
// here plus a drawHitGlyph case.
func hitGlyphForVFX(k core.VFXKind) hitGlyphKind {
	switch k {
	case core.VFXSlash:
		return glyphSlash
	case core.VFXImpact, core.VFXStoneslam:
		return glyphImpact
	case core.VFXFrost:
		return glyphFrost
	case core.VFXArc:
		return glyphSpark
	case core.VFXEmber:
		return glyphFire
	case core.VFXSmite:
		return glyphHoly
	case core.VFXVenom:
		return glyphVenom
	}
	return glyphNone
}

// hitGlyph is one live overlay: a kind + the captured target world position
// (the battle camera is static during a hit, so a fixed anchor projects to a
// stable screen spot for the glyph's ~0.4s life) + a per-kind on-screen size
// scale + its age. X/Y/Z is the target's anchor WITHOUT the vertical lift; Rise
// is the world-Y lift applied in SCREEN space at draw time (see DrawHitGlyphs)
// so an off-center foe's glyph stays directly above it under the pitched battle
// camera instead of drifting sideways toward the vertical vanishing point.
type hitGlyph struct {
	Kind     hitGlyphKind
	X, Y, Z  float32
	Rise     float32
	Scale    float32
	Elapsed  float32
	Duration float32
}

var hitGlyphs = make([]hitGlyph, 0, hitGlyphCap)

// resetHitGlyphs drops every live glyph — called alongside ResetParticles on a
// VFX reset (battle exit, area transition) so a glyph can't linger into the
// next scene.
func resetHitGlyphs() { hitGlyphs = hitGlyphs[:0] }

// spawnHitGlyph queues a glyph at the struck target's world position o (already
// nudged by the kind's HORIZONTAL glyph anchor offset at the call site — the
// vertical offsets arrive via extraRise). The base torso lift (hitGlyphRise)
// plus extraRise are stored as a screen-space Rise applied at draw, not baked
// into the world Y, so the glyph doesn't drift sideways for off-center foes
// under the pitched battle camera. Sized by scale (per-kind glyphScale; <=0 →
// 1). No-op for glyphNone or when the pool is at its cap.
func spawnHitGlyph(kind hitGlyphKind, o rl.Vector3, extraRise, scale float32) {
	if kind == glyphNone || len(hitGlyphs) >= hitGlyphCap {
		return
	}
	if scale <= 0 {
		scale = 1
	}
	hitGlyphs = append(hitGlyphs, hitGlyph{Kind: kind, X: o.X, Y: o.Y, Z: o.Z, Rise: hitGlyphRise + extraRise, Scale: scale, Duration: hitGlyphDuration})
}

// DrawHitGlyphs projects each live glyph's world anchor to screen and draws its
// animated shape, then ages + culls the pool. Call in the HUD pass (after
// EndMode3D) so the crisp 2D art layers over the world — but BEFORE the damage
// popups so the number reads on top. Mirrors the particle sweep's
// draw-before-advance so a freshly-spawned glyph always gets one visible frame.
func DrawHitGlyphs(camera rl.Camera3D) {
	if len(hitGlyphs) == 0 {
		return
	}
	dt := clampFrameDelta(rl.GetFrameTime())
	sw, _ := screenSizeF()
	write := 0
	for read := range hitGlyphs {
		gph := &hitGlyphs[read]
		anchor := rl.NewVector3(gph.X, gph.Y, gph.Z)
		screen := rl.GetWorldToScreen(anchor, camera)
		// Apply the vertical lift in SCREEN space, not world space: take the
		// glyph's X from the un-lifted anchor (so it stays directly above the
		// target) but its Y from the lifted anchor's projection. Under the pitched
		// battle camera a world-Y rise projects diagonally, so baking it into the
		// anchor made off-center foes' glyphs slide sideways toward the vanishing
		// column — locking X to the base projection removes that drift.
		if gph.Rise != 0 {
			lifted := rl.GetWorldToScreen(rl.NewVector3(gph.X, gph.Y+gph.Rise, gph.Z), camera)
			screen.Y = lifted.Y
		}
		// GetWorldToScreen mirrors points behind the camera to the wrong side of
		// the screen, so skip a behind-camera anchor (it would draw a ghost glyph).
		// Use the strict <=0 gate (behindCamera), not behindCull's negative slack
		// — the slack is for keeping world tiles abreast of the camera, and would
		// let a glyph just behind the camera plane ghost through.
		if !behindCamera(camera, anchor) && !popupOffScreenX(screen.X, sw) {
			drawHitGlyph(gph.Kind, screen.X, screen.Y, gph.Elapsed/gph.Duration, gph.Scale)
		}
		gph.Elapsed += dt
		if gph.Elapsed < gph.Duration {
			if write != read {
				hitGlyphs[write] = *gph
			}
			write++
		}
	}
	hitGlyphs = hitGlyphs[:write]
}

// glyphFade maps life fraction t∈[0,1] to an alpha that holds bright early then
// eases out — so the shape pops in clearly and fades, never lingering flat.
func glyphFade(t float32) uint8 {
	if t <= 0 {
		return 255
	}
	if t >= 1 {
		return 0
	}
	return uint8(255 * (1 - t*t))
}

// glyphGrow is an ease-out 0→1 used by the expanding glyphs (impact ring, fire,
// holy) so they punch outward fast then settle.
func glyphGrow(t float32) float32 {
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	return 1 - (1-t)*(1-t)
}

// glyphPopR is a quick pop-in radius: scales to full over the first ~30% of
// life then holds — for the "drawn instantly, then fades" glyphs (frost, venom).
// baseR is the already-scaled base radius (hitGlyphRadius × per-kind glyphScale).
func glyphPopR(t, baseR float32) float32 {
	s := t / 0.3
	if s > 1 {
		s = 1
	}
	return baseR * (0.45 + 0.55*s)
}

func drawHitGlyph(kind hitGlyphKind, cx, cy, t, scale float32) {
	// Fold the per-kind size scale into the base screen radius once, then hand
	// the scaled radius to each painter (which used to read the hitGlyphRadius
	// const directly). scale<=0 collapses to 1× so an unset value draws full
	// size, matching effectiveGlyphScale.
	if scale <= 0 {
		scale = 1
	}
	r := hitGlyphRadius * scale
	switch kind {
	case glyphSlash:
		drawGlyphSlash(cx, cy, t, r)
	case glyphImpact:
		drawGlyphImpact(cx, cy, t, r)
	case glyphFrost:
		drawGlyphFrost(cx, cy, t, r)
	case glyphSpark:
		drawGlyphSpark(cx, cy, t, r)
	case glyphFire:
		drawGlyphFire(cx, cy, t, r)
	case glyphHoly:
		drawGlyphHoly(cx, cy, t, r)
	case glyphVenom:
		drawGlyphVenom(cx, cy, t, r)
	default:
		// glyphNone (and any future unmapped kind) draws nothing — a hit with
		// no clarity glyph. No-op rather than a panic: this is draw-time
		// dispatch off a per-frame value, not a startup coverage assert, so a
		// stray kind should silently skip its frame rather than crash the HUD.
	}
}

// drawGlyphSlash — a diagonal blade stroke that sweeps top-right → bottom-left,
// with a thinner trailing edge, then fades. Reads as a quick cut.
func drawGlyphSlash(cx, cy, t, baseR float32) {
	col := colorWithAlpha(glyphSlashColor, glyphFade(t))
	r := baseR * 1.2
	ext := t / 0.4 // stroke extends over the first 40% of life
	if ext > 1 {
		ext = 1
	}
	x1, y1 := cx+r*0.8, cy-r*0.8
	x2, y2 := x1-2*r*0.8*ext, y1+2*r*0.8*ext
	rl.DrawLineEx(rl.NewVector2(x1, y1), rl.NewVector2(x2, y2), 4, col)
	off := r * 0.3
	rl.DrawLineEx(rl.NewVector2(x1-off, y1-off*0.25), rl.NewVector2(x2-off, y2-off*0.25), 2, fadeColor(col, 0.6))
}

// spokeBurst draws n evenly-spaced radial spokes from radius `inner` out to
// `outer`, each `thick` px wide, in `col`. The shared body of the impact + holy
// glyph bursts (both 8 evenly-spaced spokes differing only in radii/thickness/
// color) so the two can't drift. The frost/fire bursts keep their own loops —
// frost's spokes carry tip branches and fire's angle sweeps with t, so neither
// reduces to a plain even fan.
func spokeBurst(cx, cy float32, n int, inner, outer, thick float32, col rl.Color) {
	for i := 0; i < n; i++ {
		ang := float64(i) * tau / float64(n)
		dx, dy := float32(math.Cos(ang)), float32(math.Sin(ang))
		rl.DrawLineEx(rl.NewVector2(cx+dx*inner, cy+dy*inner), rl.NewVector2(cx+dx*outer, cy+dy*outer), thick, col)
	}
}

// drawGlyphImpact — a blunt "POW": 8 radial spikes punching outward.
func drawGlyphImpact(cx, cy, t, baseR float32) {
	col := colorWithAlpha(glyphImpactColor, glyphFade(t))
	r := baseR
	spike := r * (0.5 + 0.45*glyphGrow(t))
	spokeBurst(cx, cy, 8, r*0.25, spike, 3, col)
}

// drawGlyphFrost — a 6-armed snowflake (each arm a spoke + two tip branches),
// popping in then fading. Pale blue.
func drawGlyphFrost(cx, cy, t, baseR float32) {
	col := colorWithAlpha(glyphFrostColor, glyphFade(t))
	r := glyphPopR(t, baseR)
	for i := 0; i < 6; i++ {
		ang := float64(i) * tau / 6
		dx, dy := float32(math.Cos(ang)), float32(math.Sin(ang))
		tipX, tipY := cx+dx*r, cy+dy*r
		rl.DrawLineEx(rl.NewVector2(cx, cy), rl.NewVector2(tipX, tipY), 2, col)
		// two branches angled back from a point partway out the arm
		bx, by := cx+dx*r*0.62, cy+dy*r*0.62
		px, py := -dy, dx // perpendicular
		bl := r * 0.26
		rl.DrawLineEx(rl.NewVector2(bx, by), rl.NewVector2(bx+(dx*0.5+px*0.7)*bl, by+(dy*0.5+py*0.7)*bl), 2, col)
		rl.DrawLineEx(rl.NewVector2(bx, by), rl.NewVector2(bx+(dx*0.5-px*0.7)*bl, by+(dy*0.5-py*0.7)*bl), 2, col)
	}
	// Center hub scaled to baseR (≈2.5px at the default radius) so it tracks the
	// glyph's size instead of sitting at a fixed pixel radius like the arms do.
	rl.DrawCircleV(rl.NewVector2(cx, cy), baseR*0.096, col)
}

// drawGlyphSpark — 3 jagged lightning tendrils radiating from a bright core,
// twitching frame-to-frame like a live electric arc.
func drawGlyphSpark(cx, cy, t, baseR float32) {
	a := glyphFade(t)
	bolt := colorWithAlpha(glyphSparkBolt, a)
	r := baseR * 1.25
	// Step the wall clock into ~18 discrete frames/sec so the bolts SNAP to new
	// jagged positions rather than sliding smoothly — that staccato is what
	// reads as lightning twitch. Each tendril jitters off its own seed.
	step := math.Floor(rl.GetTime() * 18)
	for i, ang := range []float64{-1.4, 0.35, 2.05} {
		drawLightningTendril(cx, cy, ang, r, bolt, step*3+float64(i))
	}
	// Bright core scaled to baseR (≈3px at the default radius) so it tracks the
	// glyph's size rather than sitting at a fixed pixel radius.
	rl.DrawCircleV(rl.NewVector2(cx, cy), baseR*0.115, colorWithAlpha(glyphSparkCore, a))
}

// drawLightningTendril draws a 3-segment zigzag outward along `ang`. seed feeds
// the per-segment twitch so each redraw frame jags a little differently.
func drawLightningTendril(cx, cy float32, ang float64, r float32, col rl.Color, seed float64) {
	dx, dy := float32(math.Cos(ang)), float32(math.Sin(ang))
	px, py := -dy, dx // perpendicular jitter axis
	steps := [...]struct{ along, perp float32 }{{0.38, 0.2}, {0.72, -0.16}, {1.0, 0.04}}
	prevX, prevY := cx, cy
	for si, s := range steps {
		// Twitch the perpendicular offset by a stepped pseudo-random nudge so
		// the bolt jitters like a live arc; kept small ("a bit") so it still
		// aims along ang.
		perp := s.perp + glyphJitter(seed+float64(si)*7.13)*0.16
		nx := cx + dx*r*s.along + px*r*perp
		ny := cy + dy*r*s.along + py*r*perp
		rl.DrawLineEx(rl.NewVector2(prevX, prevY), rl.NewVector2(nx, ny), 2, col)
		prevX, prevY = nx, ny
	}
}

// glyphJitter is a cheap deterministic pseudo-random in [-1, 1) from a seed —
// the fract(sin·k) hash — used to twitch the spark glyph's lightning per frame
// step without pulling a real RNG into the render hot path.
func glyphJitter(seed float64) float32 {
	v := math.Sin(seed*12.9898) * 43758.5453
	return float32((v-math.Floor(v))*2 - 1)
}

// drawGlyphFire — a flame burst: a slowly-rotating glow hex, radial spikes, and
// a bright core, all expanding outward.
func drawGlyphFire(cx, cy, t, baseR float32) {
	a := glyphFade(t)
	outer := colorWithAlpha(glyphFireOuter, a)
	inner := colorWithAlpha(glyphFireInner, a)
	r := baseR * (0.7 + 0.5*glyphGrow(t))
	rl.DrawPoly(rl.NewVector2(cx, cy), 6, r*0.7, t*90, fadeColor(outer, 0.4))
	for i := 0; i < 6; i++ {
		ang := float64(i)*tau/6 + float64(t)*1.4
		dx, dy := float32(math.Cos(ang)), float32(math.Sin(ang))
		rl.DrawLineEx(rl.NewVector2(cx, cy), rl.NewVector2(cx+dx*r, cy+dy*r), 3, outer)
	}
	rl.DrawCircleV(rl.NewVector2(cx, cy), r*0.34, inner)
}

// drawGlyphHoly — radiant: 8 rays from a ring, expanding, gold.
func drawGlyphHoly(cx, cy, t, baseR float32) {
	col := colorWithAlpha(glyphHolyColor, glyphFade(t))
	r := baseR * (0.6 + 0.6*glyphGrow(t))
	spokeBurst(cx, cy, 8, r*0.32, r, 2, col)
	rl.DrawRing(rl.NewVector2(cx, cy), r*0.42, r*0.52, 0, 360, 28, col)
}

// drawGlyphVenom — a toxic mark: a ring with a couple of bubbles and a drip that
// grows downward as it fades. Green.
func drawGlyphVenom(cx, cy, t, baseR float32) {
	a := glyphFade(t)
	col := colorWithAlpha(glyphVenomColor, a)
	r := glyphPopR(t, baseR)
	rl.DrawCircleLines(int32(cx), int32(cy), r*0.5, col)
	rl.DrawCircleV(rl.NewVector2(cx-r*0.3, cy-r*0.16), r*0.18, fadeColor(col, 0.85))
	rl.DrawCircleV(rl.NewVector2(cx+r*0.28, cy-r*0.05), r*0.13, fadeColor(col, 0.7))
	drip := r * (0.35 + 0.85*t)
	dripTopY := cy + r*0.3
	rl.DrawLineEx(rl.NewVector2(cx, dripTopY), rl.NewVector2(cx, dripTopY+drip), 2, col)
	rl.DrawCircleV(rl.NewVector2(cx, dripTopY+drip), 3, col)
}

// --- Editor gallery export -------------------------------------------------
// The hit glyphs flash for ~0.4s mid-attack, so the author never gets a good
// look. The editor's Hit Glyphs viewer (editor/hitglyphs.go) plays each one on a
// loop; these two symbols are the only window it needs into this otherwise
// render-private art.

// EditorHitGlyphNames lists the clarity-glyph styles for that viewer, in the
// order EditorDrawHitGlyph indexes them — parallel to the glyphSlash..glyphVenom
// enum (gallery index i → kind i+1, skipping glyphNone). The init below asserts
// the count matches the enum so a new glyph can't silently drop out of the gallery.
var EditorHitGlyphNames = []string{"Slash", "Impact", "Frost", "Spark", "Fire", "Holy", "Venom"}

func init() {
	if len(EditorHitGlyphNames) != int(glyphVenom) {
		panic("render: EditorHitGlyphNames out of sync with the hitGlyphKind enum")
	}
}

// EditorDrawHitGlyph draws gallery glyph i (0-based, parallel to
// EditorHitGlyphNames) at screen (cx,cy) with life fraction t∈[0,1] and size
// scale — the editor loops t to animate the preview. Out-of-range i is a no-op.
func EditorDrawHitGlyph(i int, cx, cy, t, scale float32) {
	if i < 0 || i >= len(EditorHitGlyphNames) {
		return
	}
	drawHitGlyph(hitGlyphKind(i+1), cx, cy, t, scale) // +1 skips glyphNone
}

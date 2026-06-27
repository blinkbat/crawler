package render

import (
	"math"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Hit glyphs: short-lived 2D vector shapes drawn over a struck target — a clarity
// cue for what blow landed, projected crisp in the HUD pass.
//
// Presentation-only (like the particle pool), NOT on GameState; spawned from the
// VFX requests the apply handlers emit, style derived from the VFX kind.

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
	hitGlyphRise     = float32(0.5) // world-unit lift to torso height
	hitGlyphRadius   = float32(26)  // base screen radius, px
	// partyGlyphExtraRise floats a party glyph above the member's head, clear of the
	// HUD ribbon (party billboards sit low; enemy glyphs already sit high).
	partyGlyphExtraRise = float32(0.42)
)

// hitGlyphForVFX returns the clarity glyph for a VFX kind (glyphNone for non-damaging).
// The kind→glyph mapping lives in vfx.go's merged vfxKinds table (alongside the spawn
// pattern) so a new VFX kind forces both entries; out-of-range falls back to glyphNone.
func hitGlyphForVFX(k core.VFXKind) hitGlyphKind {
	if k < 0 || k >= core.VFXKindCount {
		return glyphNone
	}
	return vfxKinds[k].glyph
}

// hitGlyph is one live overlay: kind + the target's ANCHOR IDENTITY (re-resolved
// each frame, not a frozen XYZ, since the target moves in depth during its ~0.4s
// life). GlyphXOffset nudges camera-right; Rise is a world-Y lift applied in SCREEN
// space (see DrawHitGlyphs); DepthOffset matches the billboard's depthOffset so the
// glyph sits at the right depth.
type hitGlyph struct {
	Kind         hitGlyphKind
	Anchor       core.VFXAnchor
	SlotIdx      int
	TileX, TileZ int
	GlyphXOffset float32
	DepthOffset  float32
	Rise         float32
	Scale        float32
	Elapsed      float32
	Duration     float32
	// lastX/Y/Z: last resolved world anchor (post offsets) so a glyph finishes in
	// place if the anchor stops resolving mid-life. haveLast guards never-resolved.
	lastX, lastY, lastZ float32
	haveLast            bool
}

var hitGlyphs = make([]hitGlyph, 0, hitGlyphCap)

// resetHitGlyphs drops every live glyph (with ResetParticles on a VFX reset).
func resetHitGlyphs() { hitGlyphs = hitGlyphs[:0] }

// spawnHitGlyph queues a glyph bound to the target's ANCHOR for DrawHitGlyphs to
// track. Torso lift + extraRise stored as screen-space Rise. scale <=0 → 1. No-op
// for glyphNone or a full pool.
func spawnHitGlyph(kind hitGlyphKind, req core.VFXRequest, glyphXOffset, depthOffset, extraRise, scale float32) {
	if kind == glyphNone || len(hitGlyphs) >= hitGlyphCap {
		return
	}
	if scale <= 0 {
		scale = 1
	}
	hitGlyphs = append(hitGlyphs, hitGlyph{
		Kind:         kind,
		Anchor:       req.Anchor,
		SlotIdx:      req.SlotIdx,
		TileX:        req.TileX,
		TileZ:        req.TileZ,
		GlyphXOffset: glyphXOffset,
		DepthOffset:  depthOffset,
		Rise:         hitGlyphRise + extraRise,
		Scale:        scale,
		Duration:     hitGlyphDuration,
	})
}

// DrawHitGlyphs re-resolves each anchor, projects, draws, then ages + culls. Call
// in the HUD pass (after EndMode3D) BEFORE damage popups. Draw-before-advance so a
// fresh glyph gets one visible frame.
func DrawHitGlyphs(camera rl.Camera3D, g *core.GameState, assets Resources) {
	if len(hitGlyphs) == 0 {
		return
	}
	dt := clampFrameDelta(rl.GetFrameTime())
	sw, _ := screenSizeF()
	write := 0
	for read := range hitGlyphs {
		gph := &hitGlyphs[read]
		// Re-resolve from the anchor identity each frame so the glyph tracks the
		// recoiling/lunging sprite; fall back to the last good anchor if it stops
		// resolving mid-life, or skip drawing if never resolved.
		var anchor rl.Vector3
		draw := true
		if origin, ok := resolveAnchor(camera, g, core.VFXRequest{Anchor: gph.Anchor, SlotIdx: gph.SlotIdx, TileX: gph.TileX, TileZ: gph.TileZ}); ok {
			// Re-apply X nudge + depth push-back so the anchor sits at the billboard's
			// depth (else a pushed-back off-center sprite projects its glyph sideways).
			anchor = cameraRelativeOffset(camera, origin, gph.GlyphXOffset, 0, gph.DepthOffset)
			gph.lastX, gph.lastY, gph.lastZ, gph.haveLast = anchor.X, anchor.Y, anchor.Z, true
		} else if gph.haveLast {
			anchor = rl.NewVector3(gph.lastX, gph.lastY, gph.lastZ)
		} else {
			draw = false
		}
		if draw {
			screen := rl.GetWorldToScreen(anchor, camera)
			// Lift in SCREEN space (X un-lifted, Y lifted): a world-Y rise projects
			// diagonally under the pitched camera, sliding off-center glyphs sideways.
			if gph.Rise != 0 {
				lifted := rl.GetWorldToScreen(rl.NewVector3(anchor.X, anchor.Y+gph.Rise, anchor.Z), camera)
				screen.Y = lifted.Y
			}
			// GetWorldToScreen mirrors behind-camera points, so skip them with the
			// strict behindCamera gate (not behindCull's slack).
			if !behindCamera(camera, anchor) && !popupOffScreenX(screen.X, sw) {
				drawHitGlyph(gph.Kind, screen.X, screen.Y, gph.Elapsed/gph.Duration, gph.Scale)
			}
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

// glyphFade maps life fraction t∈[0,1] to alpha (bright early, eases out).
func glyphFade(t float32) uint8 {
	if t <= 0 {
		return 255
	}
	if t >= 1 {
		return 0
	}
	return uint8(255 * (1 - t*t))
}

// glyphGrow is an ease-out 0→1 for the expanding glyphs (impact, fire, holy).
func glyphGrow(t float32) float32 {
	return core.EaseOutQuad(t)
}

// glyphPopR is a quick pop-in radius (full by ~30% of life) for frost/venom.
// baseR is already scaled.
func glyphPopR(t, baseR float32) float32 {
	s := t / 0.3
	if s > 1 {
		s = 1
	}
	return baseR * (0.45 + 0.55*s)
}

func drawHitGlyph(kind hitGlyphKind, cx, cy, t, scale float32) {
	// Fold per-kind scale into base radius once. scale<=0 → 1×.
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
		// glyphNone / unmapped: no-op (per-frame dispatch, don't crash the HUD).
	}
}

// drawGlyphSlash — diagonal blade stroke, top-right → bottom-left, thin trailing edge.
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

// radialSpokes invokes fn for n evenly-spaced directions around a circle, with an
// optional starting-angle offset, passing each spoke's unit (dx,dy). Centralizes
// the `ang := i·τ/n; cos/sin` boilerplate shared by the burst/snowflake/flame
// glyphs and the ice-armor status mark.
func radialSpokes(n int, offset float64, fn func(i int, dx, dy float32)) {
	for i := 0; i < n; i++ {
		ang := float64(i)*tau/float64(n) + offset
		fn(i, float32(math.Cos(ang)), float32(math.Sin(ang)))
	}
}

// spokeBurst draws n evenly-spaced radial spokes from inner to outer, thick px
// wide. Shared by impact + holy.
func spokeBurst(cx, cy float32, n int, inner, outer, thick float32, col rl.Color) {
	radialSpokes(n, 0, func(_ int, dx, dy float32) {
		rl.DrawLineEx(rl.NewVector2(cx+dx*inner, cy+dy*inner), rl.NewVector2(cx+dx*outer, cy+dy*outer), thick, col)
	})
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
	radialSpokes(6, 0, func(_ int, dx, dy float32) {
		tipX, tipY := cx+dx*r, cy+dy*r
		rl.DrawLineEx(rl.NewVector2(cx, cy), rl.NewVector2(tipX, tipY), 2, col)
		// two branches angled back from a point partway out the arm
		bx, by := cx+dx*r*0.62, cy+dy*r*0.62
		px, py := -dy, dx // perpendicular
		bl := r * 0.26
		rl.DrawLineEx(rl.NewVector2(bx, by), rl.NewVector2(bx+(dx*0.5+px*0.7)*bl, by+(dy*0.5+py*0.7)*bl), 2, col)
		rl.DrawLineEx(rl.NewVector2(bx, by), rl.NewVector2(bx+(dx*0.5-px*0.7)*bl, by+(dy*0.5-py*0.7)*bl), 2, col)
	})
	// Center hub scaled to baseR so it tracks the glyph's size.
	rl.DrawCircleV(rl.NewVector2(cx, cy), baseR*0.096, col)
}

// drawGlyphSpark — 3 jagged lightning tendrils radiating from a bright core,
// twitching frame-to-frame like a live electric arc.
func drawGlyphSpark(cx, cy, t, baseR float32) {
	a := glyphFade(t)
	bolt := colorWithAlpha(glyphSparkBolt, a)
	r := baseR * 1.25
	// Step the clock into ~18 frames/sec so the bolts SNAP rather than slide —
	// that staccato reads as lightning twitch.
	step := math.Floor(frameTime() * 18)
	for i, ang := range []float64{-1.4, 0.35, 2.05} {
		drawLightningTendril(cx, cy, ang, r, bolt, step*3+float64(i))
	}
	// Bright core scaled to baseR so it tracks the glyph's size.
	rl.DrawCircleV(rl.NewVector2(cx, cy), baseR*0.115, colorWithAlpha(glyphSparkCore, a))
}

// drawLightningTendril draws a 3-segment zigzag along `ang`; seed feeds the
// per-segment twitch so each frame jags differently.
func drawLightningTendril(cx, cy float32, ang float64, r float32, col rl.Color, seed float64) {
	dx, dy := float32(math.Cos(ang)), float32(math.Sin(ang))
	px, py := -dy, dx // perpendicular jitter axis
	steps := [...]struct{ along, perp float32 }{{0.38, 0.2}, {0.72, -0.16}, {1.0, 0.04}}
	prevX, prevY := cx, cy
	for si, s := range steps {
		// Stepped pseudo-random perp nudge so the bolt jitters; kept small so it
		// still aims along ang.
		perp := s.perp + glyphJitter(seed+float64(si)*7.13)*0.16
		nx := cx + dx*r*s.along + px*r*perp
		ny := cy + dy*r*s.along + py*r*perp
		rl.DrawLineEx(rl.NewVector2(prevX, prevY), rl.NewVector2(nx, ny), 2, col)
		prevX, prevY = nx, ny
	}
}

// glyphJitter is a cheap deterministic pseudo-random in [-1, 1) (the fract(sin·k)
// hash, constants in theme.go) — twitches the spark glyph without a real RNG.
func glyphJitter(seed float64) float32 {
	v := math.Sin(seed*fractSinHashA) * fractSinHashB
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
	radialSpokes(6, float64(t)*1.4, func(_ int, dx, dy float32) {
		rl.DrawLineEx(rl.NewVector2(cx, cy), rl.NewVector2(cx+dx*r, cy+dy*r), 3, outer)
	})
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
// The editor's Hit Glyphs viewer (editor/hitglyphs.go) loops each glyph (they
// flash for only ~0.4s mid-attack); these symbols are its only window in.

// editorHitGlyphs is the single ordered gallery table: name + kind paired so the
// names list and the draw-by-index path can't drift (the old parallel slice + the
// "+1 skips glyphNone" index arithmetic both derive from this now). glyphNone is
// simply omitted rather than skipped by an offset.
var editorHitGlyphs = []struct {
	name string
	kind hitGlyphKind
}{
	{"Slash", glyphSlash},
	{"Impact", glyphImpact},
	{"Frost", glyphFrost},
	{"Spark", glyphSpark},
	{"Fire", glyphFire},
	{"Holy", glyphHoly},
	{"Venom", glyphVenom},
}

// EditorHitGlyphNames lists the glyph style names for the viewer, derived from the
// ordered editorHitGlyphs table (built once; returned slice is read-only).
var EditorHitGlyphNames = func() []string {
	names := make([]string, len(editorHitGlyphs))
	for i, e := range editorHitGlyphs {
		names[i] = e.name
	}
	return names
}()

// EditorDrawHitGlyph draws gallery glyph i (0-based, parallel to EditorHitGlyphNames)
// at (cx,cy), life fraction t, size scale. Out-of-range i is a no-op.
func EditorDrawHitGlyph(i int, cx, cy, t, scale float32) {
	if i < 0 || i >= len(editorHitGlyphs) {
		return
	}
	drawHitGlyph(editorHitGlyphs[i].kind, cx, cy, t, scale)
}

package render

import (
	"image/color"
	"math"
	"math/rand"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// VFX particle pool, owned by render (positions resolve from the camera; draw uses billboards).
// Battle/explore emit intents via core.EnqueueXxxVFX; TickAndDrawVFX consumes them per frame.
// particleCapacity is the pre-reserve; particleHardCap (4×) is the spawn drop ceiling that
// bounds a runaway spawner.
const (
	particleCapacity = 512
	particleHardCap  = 2048
)

// init asserts capacity <= hardCap, else a fresh pool would already be "full" and drop every spawn.
func init() {
	if particleCapacity > particleHardCap {
		panic("render: particleCapacity must be <= particleHardCap")
	}
}

var particles = make([]particle, 0, particleCapacity)

// ResetParticles drops every live particle (on a scene change) so stale formation-relative
// positions don't drift into the new scene.
func ResetParticles() {
	particles = particles[:0]
}

// pushParticle is the spawn choke point; drops the particle at the hard cap to bound a runaway spawner.
func pushParticle(p particle) {
	if len(particles) >= particleHardCap {
		return
	}
	particles = append(particles, p)
}

// vfxRNG drives particle jitter. Independent of GameState.RNG: VFX are presentation-only, and
// drawing from the game RNG would bias gameplay rolls by particle count.
var vfxRNG = rand.New(rand.NewSource(0x5EED_F1AC))

// particleShape names the rendered silhouette family (a switch row in drawParticle, no texture asset).
type particleShape uint8

const (
	shapeSpark particleShape = iota
	shapeMote
	shapeShard
	shapeRing
	shapeDust
	shapeStrand
)

// particle is one live FX dot: position drifts under velocity + gravity·t; size/color animate over [0, Duration].
type particle struct {
	X, Y, Z    float32
	VX, VY, VZ float32
	GX, GY, GZ float32 // constant accel (gravity, drag substitute)
	Elapsed    float32
	Duration   float32
	SizeStart  float32
	SizeEnd    float32
	ColorStart color.RGBA
	ColorEnd   color.RGBA
	Rotation   float32
	Spin       float32
	Shape      particleShape
}

// alive reports whether the particle still has lifetime left.
func (p *particle) alive() bool { return p.Elapsed < p.Duration }

// TickAndDrawVFX is the per-frame render entry point: drain the VFX queue, advance every live
// particle by clamped dt, draw each. Must run inside rl.BeginMode3D/EndMode3D for depth sorting.
func TickAndDrawVFX(camera rl.Camera3D, g *core.GameState, assets Resources) {
	// Reset before draining so the new frame's spawns land in a clean pool, not beside stale anchors.
	if core.TakeVFXResetRequest(g) {
		ResetParticles()
		resetHitGlyphs()
		resetBarGhosts()
	}
	for _, req := range core.DrainVFXQueue(g) {
		spawnFromRequest(camera, g, req, assets)
	}
	// Clamp dt so a long stall doesn't fast-forward every particle past its lifetime in one frame.
	dt := clampFrameDelta(rl.GetFrameTime())
	// Two-pointer sweep: advance + compact in place (live stay at the front; no per-frame alloc).
	write := 0
	for read := range particles {
		p := &particles[read]
		// Draw BEFORE advancing so a particle gets at least one visible frame even if this dt
		// would age it past Duration (advancing first could cull a sub-dt particle unseen).
		drawParticle(p)
		p.Elapsed += dt
		if !p.alive() {
			continue
		}
		p.VX += p.GX * dt
		p.VY += p.GY * dt
		p.VZ += p.GZ * dt
		p.X += p.VX * dt
		p.Y += p.VY * dt
		p.Z += p.VZ * dt
		p.Rotation += p.Spin * dt
		if write != read {
			particles[write] = *p
		}
		write++
	}
	particles = particles[:write]
}

// resolveAnchor returns a VFX request's world-space spawn origin, reusing the camera-aware
// formation math the billboards use so VFX land at the actor's visible spot, not its logical tile.
func resolveAnchor(camera rl.Camera3D, g *core.GameState, req core.VFXRequest) (rl.Vector3, bool) {
	switch req.Anchor {
	case core.VFXAnchorEnemy:
		members := core.BattleMembers(g)
		if req.SlotIdx < 0 || req.SlotIdx >= len(members) {
			return rl.Vector3{}, false
		}
		return enemyDrawPosition(camera, g, req.SlotIdx, &members[req.SlotIdx]), true
	case core.VFXAnchorParty:
		if req.SlotIdx < 0 || req.SlotIdx >= len(g.Party) {
			return rl.Vector3{}, false
		}
		m := &g.Party[req.SlotIdx]
		return partySpritePosition(camera, g.Party, req.SlotIdx, m.AttackBump, 0, m.HitKnockback), true
	case core.VFXAnchorTile:
		return tileWorldPos(req.TileX, req.TileZ, 0.05), true
	default:
		// Mirror spawnFromRequest's logged default so a newly added VFXAnchor surfaces
		// here too (this returns false first, so the spawn-side log never fires alone).
		if !loggedUnknownAnchor[req.Anchor] {
			loggedUnknownAnchor[req.Anchor] = true
			LogRenderError("resolveAnchor: unhandled VFXAnchor %d — VFX skipped", int(req.Anchor))
		}
		return rl.Vector3{}, false
	}
}

// spawnFromRequest dispatches a queued intent to its per-kind spawn pattern. Keep particle
// counts small — draw cost is dominated by billboard transforms.
func spawnFromRequest(camera rl.Camera3D, g *core.GameState, req core.VFXRequest, assets Resources) {
	origin, ok := resolveAnchor(camera, g, req)
	if !ok {
		return
	}
	// Per-enemy/class GFX tuning (Foe/Party Visualizer) can nudge + resize the burst and glyph;
	// only enemy/party anchors are keyed, tile uses the raw origin at 1×. origin is rebound to the
	// particle anchor (the glyph keeps its own). glyphRise carries the VERTICAL offset, applied in
	// SCREEN space at draw so the glyph stays above off-center foes under the pitched camera; the
	// glyph re-resolves its anchor each frame so it tracks recoil/lunge.
	glyphXOffset, glyphScale := float32(0), float32(1)
	glyphRise := float32(0)
	glyphDepth := float32(0)
	particleScale := float32(1)
	switch req.Anchor {
	case core.VFXAnchorEnemy:
		if v, ok := enemyVisualForVFX(g, assets, req.SlotIdx); ok {
			glyphXOffset = v.glyphXOffset
			glyphRise = v.glyphYOffset
			glyphDepth = v.depthOffset
			glyphScale = v.effectiveGlyphScale()
			origin = cameraRelativeOffset(camera, origin, v.particleXOffset, v.particleYOffset, v.particleZOffset)
			particleScale = v.effectiveParticleScale()
		}
	case core.VFXAnchorParty:
		// Party sprites sit low; lift the glyph above the member's head so the cue is visible.
		glyphRise = partyGlyphExtraRise
		// Per-class GFX tuning (Party Visualizer); without this the saved offsets never reached live FX.
		if req.SlotIdx >= 0 && req.SlotIdx < len(g.Party) {
			if v, ok := partyVisualFor(assets, g.Party[req.SlotIdx].Class); ok {
				glyphXOffset = v.glyphXOffset
				glyphRise += v.glyphYOffset
				glyphDepth = v.depthOffset
				glyphScale = v.effectiveGlyphScale()
				origin = cameraRelativeOffset(camera, origin, v.particleXOffset, v.particleYOffset, v.particleZOffset)
				particleScale = v.effectiveParticleScale()
			}
		}
	case core.VFXAnchorTile:
		// Raw origin at 1×, no tuning. Explicit so a new VFXAnchor can't silently inherit it.
	default:
		if !loggedUnknownAnchor[req.Anchor] {
			loggedUnknownAnchor[req.Anchor] = true
			LogRenderError("spawnFromRequest: unhandled VFXAnchor %d — using raw origin at 1×", int(req.Anchor))
		}
	}
	// Snapshot the pool length so scaleBurst transforms only this request's particles.
	from := len(particles)
	if req.Kind >= 0 && req.Kind < core.VFXKindCount && vfxKinds[req.Kind].spawn != nil {
		vfxKinds[req.Kind].spawn(origin)
	} else {
		// A VFXKind with no spawn pattern: surface once per kind (a wiring gap, not a no-op).
		if !loggedUnknownVFX[req.Kind] {
			loggedUnknownVFX[req.Kind] = true
			LogRenderError("spawnFromRequest: no spawn pattern for VFX kind %d — effect dropped", int(req.Kind))
		}
	}
	// Scale the just-spawned burst by particleScale around its anchor (1× is a no-op).
	scaleBurst(origin, from, particleScale)
	// Clarity glyph over the struck target, keyed by VFX kind (glyphNone is a no-op, so only
	// damaging hits get one). Drawn in the HUD pass by DrawHitGlyphs.
	spawnHitGlyph(hitGlyphForVFX(req.Kind), req, glyphXOffset, glyphDepth, glyphRise, glyphScale)
}

// vfxKind pairs a kind's spawn pattern with its clarity glyph so a new core.VFXKind
// forces BOTH entries in one place (spawn nil = no particles, glyphNone = no glyph).
type vfxKind struct {
	spawn func(rl.Vector3)
	glyph hitGlyphKind
}

// vfxKinds is the single per-VFXKind table (replaces the old spawn switch + the
// parallel vfxGlyphs map). Indexed by core.VFXKind; init asserts every kind in
// [0, VFXKindCount) is present (a zero-value entry — nil spawn + glyphNone — is
// only valid for genuinely particle-less, glyph-less kinds like VFXNone).
var vfxKinds = [core.VFXKindCount]vfxKind{
	core.VFXNone:      {nil, glyphNone},
	core.VFXSlash:     {spawnSlash, glyphSlash},
	core.VFXImpact:    {spawnImpact, glyphImpact},
	core.VFXEmber:     {spawnEmber, glyphFire},
	core.VFXHeal:      {spawnHeal, glyphNone},
	core.VFXSmite:     {spawnSmite, glyphHoly},
	core.VFXVenom:     {spawnVenom, glyphVenom},
	core.VFXFrost:     {spawnFrost, glyphFrost},
	core.VFXArc:       {spawnArc, glyphSpark},
	core.VFXSteal:     {spawnSteal, glyphNone},
	core.VFXDeath:     {spawnDeath, glyphNone},
	core.VFXStoneslam: {spawnStoneslam, glyphImpact},
	core.VFXSleep:     {spawnSleep, glyphNone},
	core.VFXWeb:       {spawnWeb, glyphNone},
	core.VFXConfuse:   {spawnConfuse, glyphNone},
	core.VFXIngest:    {spawnIngest, glyphNone},
	core.VFXScan:      {spawnScan, glyphNone},
}

// vfxKindsSpawnlessOK flags the kinds that intentionally carry NO spawn pattern,
// so init can tell a deliberate particle-less kind from a forgotten wiring gap.
var vfxKindsSpawnlessOK = map[core.VFXKind]bool{
	core.VFXNone: true, // sentinel; never spawned
}

// init asserts the merged vfxKinds table covers every VFXKind with a spawn pattern
// (except the deliberately particle-less ones), mirroring the old switch + the
// vfxGlyphs length-assert so a new kind forces both a spawn and a glyph choice.
// vfxAnchorHandled lists the anchors resolveAnchor + spawnFromRequest explicitly switch
// on. Both have a logged-once runtime default; this turns that into a startup panic so a
// new anchor can't ship silently degraded.
var vfxAnchorHandled = map[core.VFXAnchor]bool{
	core.VFXAnchorEnemy: true,
	core.VFXAnchorParty: true,
	core.VFXAnchorTile:  true,
}

func init() {
	for k := core.VFXKind(0); k < core.VFXKindCount; k++ {
		if vfxKinds[k].spawn == nil && !vfxKindsSpawnlessOK[k] {
			panic("render/vfx: vfxKinds missing a spawn pattern for a VFX kind — add its spawn func + clarity glyph (or mark it spawnless)")
		}
	}
	for a := core.VFXAnchor(0); a < core.VFXAnchorCount; a++ {
		if !vfxAnchorHandled[a] {
			panic("render/vfx: VFXAnchor not handled — add it to resolveAnchor AND spawnFromRequest (and vfxAnchorHandled)")
		}
	}
}

// enemyVisualForVFX resolves the enemyVisual for the enemy in a battle slot (bounds-checked).
// ok=false when the slot is empty or the kind has no visual.
func enemyVisualForVFX(g *core.GameState, assets Resources, slot int) (enemyVisual, bool) {
	members := core.BattleMembers(g)
	if slot < 0 || slot >= len(members) {
		return enemyVisual{}, false
	}
	return enemyVisualFor(assets, members[slot].Kind)
}

// scaleBurst resizes particles[from:] around anchor o, scaling XZ displacement, velocity,
// accel, and size. Velocity+gravity scale together so timing is preserved while the envelope
// grows. Y is left untouched (ground rings seed at an absolute floor Y). scale<=0 or ==1 is a no-op.
func scaleBurst(o rl.Vector3, from int, scale float32) {
	if scale <= 0 || scale == 1 || from >= len(particles) {
		return
	}
	for i := from; i < len(particles); i++ {
		p := &particles[i]
		p.X = o.X + (p.X-o.X)*scale
		p.Z = o.Z + (p.Z-o.Z)*scale
		p.VX *= scale
		p.VY *= scale
		p.VZ *= scale
		p.GX *= scale
		p.GY *= scale
		p.GZ *= scale
		p.SizeStart *= scale
		p.SizeEnd *= scale
	}
}

// loggedUnknownVFX / loggedUnknownAnchor dedupe the unknown-kind/anchor warnings to once each.
var loggedUnknownVFX = map[core.VFXKind]bool{}

var loggedUnknownAnchor = map[core.VFXAnchor]bool{}

// --- Per-kind spawn patterns -----------------------------------------------
// Each pushes particles into the pool via shared randomization helpers (vfxJitter, randAngle, …).

func vfxJitter(scale float32) float32 {
	return (vfxRNG.Float32()*2 - 1) * scale
}

func randAngle() float32 {
	return vfxRNG.Float32() * tau
}

// radialBurst parameterizes the "fling N particles outward on a random heading" skeleton most
// impact/cast effects share. speed/VY/duration are uniform [min,max] ranges.
type radialBurst struct {
	count              int
	speedMin, speedMax float32 // horizontal speed range (radial mode)
	horizScale         float32 // VX/VZ multiplier (treated as 1 when 0)
	// vyOnly: no radial fling — VX/VZ become ± velJitter, for rise/fall-in-place emitters.
	vyOnly             bool
	velJitter          float32 // ± jitter on VX and VZ (vyOnly mode)
	posJitter          float32 // ± jitter on X and Z
	yBase, yJitter     float32 // Y = base + jitter (relative to origin unless groundY)
	groundY            bool    // Y is absolute (ground-anchored) instead of o.Y-relative
	vyMin, vyMax       float32 // vertical velocity range
	gravityY           float32
	durMin, durMax     float32
	sizeStart, sizeEnd float32
	spin               float32 // ± spin amplitude (0 = no spin)
	colorStart         rl.Color
	colorEnd           rl.Color
	shape              particleShape
}

// uniform draws a value in [lo, hi] from vfxRNG.
func uniform(lo, hi float32) float32 {
	return lo + vfxRNG.Float32()*(hi-lo)
}

func spawnRadialBurst(o rl.Vector3, b radialBurst) {
	horiz := b.horizScale
	if horiz == 0 {
		horiz = 1
	}
	for i := 0; i < b.count; i++ {
		var vx, vz float32
		if b.vyOnly {
			vx = vfxJitter(b.velJitter)
			vz = vfxJitter(b.velJitter)
		} else {
			ang := randAngle()
			speed := uniform(b.speedMin, b.speedMax)
			vx = float32(math.Cos(float64(ang))) * speed * horiz
			vz = float32(math.Sin(float64(ang))) * speed * horiz
		}
		y := b.yBase + vfxJitter(b.yJitter)
		if !b.groundY {
			y += o.Y
		}
		p := particle{
			X:          o.X + vfxJitter(b.posJitter),
			Y:          y,
			Z:          o.Z + vfxJitter(b.posJitter),
			VX:         vx,
			VY:         uniform(b.vyMin, b.vyMax),
			VZ:         vz,
			GY:         b.gravityY,
			Duration:   uniform(b.durMin, b.durMax),
			SizeStart:  b.sizeStart,
			SizeEnd:    b.sizeEnd,
			ColorStart: b.colorStart,
			ColorEnd:   b.colorEnd,
			Shape:      b.shape,
		}
		if b.spin != 0 {
			p.Spin = (vfxRNG.Float32()*2 - 1) * b.spin
		}
		pushParticle(p)
	}
}

func spawnSlash(o rl.Vector3) {
	// Bright crescent of fast outward sparks + a quick ground ring.
	spawnRadialBurst(o, radialBurst{
		count: 14, speedMin: 2.4, speedMax: 4.0,
		yBase: 0.05, yJitter: 0.18,
		vyMin: -0.2, vyMax: 1.0, gravityY: -3.2,
		durMin: 0.32, durMax: 0.44, sizeStart: 0.10, sizeEnd: 0.02,
		colorStart: rl.NewColor(255, 248, 200, 255),
		colorEnd:   rl.NewColor(248, 196, 110, 0),
		shape:      shapeSpark,
	})
	pushRing(o, rl.NewColor(255, 232, 168, 220), 0.18, 1.1, 0.32)
}

func spawnImpact(o rl.Vector3) {
	// Blunt hit: tight fast pop of pale sparks + a small ring — a "thud," not a "cut."
	spawnRadialBurst(o, radialBurst{
		count: 12, speedMin: 1.8, speedMax: 3.2,
		yBase: 0.05, yJitter: 0.14,
		vyMin: -0.1, vyMax: 0.9, gravityY: -3.0,
		durMin: 0.22, durMax: 0.32, sizeStart: 0.13, sizeEnd: 0.02,
		colorStart: rl.NewColor(255, 246, 214, 255),
		colorEnd:   rl.NewColor(220, 178, 120, 0),
		shape:      shapeSpark,
	})
	pushRing(o, rl.NewColor(255, 230, 178, 220), 0.14, 0.8, 0.26)
}

func spawnEmber(o rl.Vector3) {
	// Drifting orange embers + a hot core flash; longer-lived so the burn lingers.
	spawnRadialBurst(o, radialBurst{
		count: 22, speedMin: 0.9, speedMax: 2.5, horizScale: 0.6,
		posJitter: 0.08, yJitter: 0.18,
		vyMin: 0.9, vyMax: 2.1, gravityY: -1.4,
		durMin: 0.55, durMax: 0.85, sizeStart: 0.16, sizeEnd: 0.04,
		colorStart: rl.NewColor(255, 196, 96, 255),
		colorEnd:   rl.NewColor(180, 50, 30, 0),
		shape:      shapeMote,
	})
	pushRing(o, rl.NewColor(255, 152, 80, 220), 0.20, 1.4, 0.38)
}

func spawnHeal(o rl.Vector3) {
	// Slow-rising green-gold motes — gentle, not explosive.
	spawnRadialBurst(o, radialBurst{
		count: 16, vyOnly: true, velJitter: 0.25,
		posJitter: 0.22, yBase: -0.2,
		vyMin: 0.6, vyMax: 1.1,
		durMin: 0.9, durMax: 1.3, sizeStart: 0.09, sizeEnd: 0.14,
		colorStart: rl.NewColor(212, 248, 196, 255),
		colorEnd:   rl.NewColor(132, 200, 96, 0),
		shape:      shapeMote,
	})
}

func spawnSmite(o rl.Vector3) {
	// Vertical pillar of light shards descending then bursting out.
	spawnRadialBurst(o, radialBurst{
		count: 10, vyOnly: true,
		posJitter: 0.10, yBase: 0.9, yJitter: 0.20,
		vyMin: -3.0, vyMax: -3.0,
		durMin: 0.28, durMax: 0.38, sizeStart: 0.18, sizeEnd: 0.05,
		colorStart: rl.NewColor(255, 248, 196, 255),
		colorEnd:   rl.NewColor(232, 196, 112, 0),
		shape:      shapeShard,
	})
	spawnRadialBurst(o, radialBurst{
		count: 10, speedMin: 1.8, speedMax: 3.0,
		yBase: 0.05, vyMin: 0.4, vyMax: 0.4, gravityY: -2.4,
		durMin: 0.36, durMax: 0.36, sizeStart: 0.10, sizeEnd: 0.02,
		colorStart: rl.NewColor(255, 240, 168, 255),
		colorEnd:   rl.NewColor(212, 152, 64, 0),
		shape:      shapeSpark,
	})
	pushRing(o, rl.NewColor(255, 248, 168, 230), 0.22, 1.4, 0.42)
}

func spawnVenom(o rl.Vector3) {
	// Lazy green mist; lingers longer than the others.
	spawnRadialBurst(o, radialBurst{
		count: 18, speedMin: 0.4, speedMax: 1.0,
		posJitter: 0.10, yJitter: 0.20,
		vyMin: 0.15, vyMax: 0.45,
		durMin: 0.7, durMax: 1.1, sizeStart: 0.14, sizeEnd: 0.22,
		colorStart: rl.NewColor(176, 232, 132, 220),
		colorEnd:   rl.NewColor(96, 140, 64, 0),
		shape:      shapeMote,
	})
}

func spawnFrost(o rl.Vector3) {
	// Sharp expanding diamond shards.
	spawnRadialBurst(o, radialBurst{
		count: 18, speedMin: 2.0, speedMax: 3.2,
		yJitter: 0.20, vyMin: -0.2, vyMax: 0.6, gravityY: -1.6,
		durMin: 0.4, durMax: 0.58, sizeStart: 0.08, sizeEnd: 0.18, spin: 6,
		colorStart: rl.NewColor(220, 240, 255, 255),
		colorEnd:   rl.NewColor(104, 168, 224, 0),
		shape:      shapeShard,
	})
	pushRing(o, rl.NewColor(180, 220, 255, 220), 0.20, 1.3, 0.32)
}

func spawnArc(o rl.Vector3) {
	// Short harsh blue flashes — mimics a chain-zap without drawing a polyline per arc.
	spawnRadialBurst(o, radialBurst{
		count: 14, speedMin: 2.8, speedMax: 3.8,
		yJitter: 0.30, vyMin: -0.8, vyMax: 0.8,
		durMin: 0.18, durMax: 0.18, sizeStart: 0.10, sizeEnd: 0.02,
		colorStart: rl.NewColor(196, 224, 255, 255),
		colorEnd:   rl.NewColor(96, 132, 232, 0),
		shape:      shapeSpark,
	})
}

func spawnSteal(o rl.Vector3) {
	// Quick yellow star pop — reads as "pluck," not "explosion."
	spawnRadialBurst(o, radialBurst{
		count: 8, speedMin: 1.6, speedMax: 2.2,
		yBase: 0.1, vyMin: 0.8, vyMax: 1.2, gravityY: -2.4,
		durMin: 0.30, durMax: 0.30, sizeStart: 0.12, sizeEnd: 0.02,
		colorStart: rl.NewColor(255, 240, 168, 255),
		colorEnd:   rl.NewColor(232, 196, 96, 0),
		shape:      shapeSpark,
	})
}

func spawnScan(o rl.Vector3) {
	// Pale-cyan reveal pulse — slow, soft motes drifting up; reads as "studied," not "struck."
	spawnRadialBurst(o, radialBurst{
		count: 12, speedMin: 1.0, speedMax: 1.6,
		yBase: 0.1, vyMin: 0.5, vyMax: 0.9, gravityY: -0.6,
		durMin: 0.45, durMax: 0.6, sizeStart: 0.09, sizeEnd: 0.02,
		colorStart: rl.NewColor(168, 240, 255, 255),
		colorEnd:   rl.NewColor(96, 168, 220, 0),
		shape:      shapeSpark,
	})
}

func spawnDeath(o rl.Vector3) {
	// Gray dispersing dust.
	spawnRadialBurst(o, radialBurst{
		count: 16, speedMin: 0.8, speedMax: 1.4,
		yJitter: 0.20, vyMin: 0.0, vyMax: 0.4, gravityY: -1.0,
		durMin: 0.7, durMax: 1.1, sizeStart: 0.14, sizeEnd: 0.24,
		colorStart: rl.NewColor(184, 168, 152, 220),
		colorEnd:   rl.NewColor(96, 88, 84, 0),
		shape:      shapeDust,
	})
}

func spawnStoneslam(o rl.Vector3) {
	// Big heavy dust cloud at the ground + outward shockwave.
	spawnRadialBurst(o, radialBurst{
		count: 28, speedMin: 1.4, speedMax: 2.6,
		posJitter: 0.10, yBase: 0.05, yJitter: 0.05, groundY: true,
		vyMin: 0.8, vyMax: 1.4, gravityY: -1.6,
		durMin: 0.7, durMax: 1.1, sizeStart: 0.22, sizeEnd: 0.36,
		colorStart: rl.NewColor(168, 152, 128, 220),
		colorEnd:   rl.NewColor(86, 76, 64, 0),
		shape:      shapeDust,
	})
	pushRing(rl.NewVector3(o.X, 0.05, o.Z), rl.NewColor(196, 184, 152, 240), 0.32, 2.2, 0.5)
}

func spawnSleep(o rl.Vector3) {
	// Drifting indigo blobs — sells "Zzz."
	spawnRadialBurst(o, radialBurst{
		count: 8, vyOnly: true, velJitter: 0.20,
		posJitter: 0.18, yBase: 0.1,
		vyMin: 0.35, vyMax: 0.6,
		durMin: 1.0, durMax: 1.4, sizeStart: 0.10, sizeEnd: 0.18,
		colorStart: rl.NewColor(168, 200, 240, 220),
		colorEnd:   rl.NewColor(96, 132, 192, 0),
		shape:      shapeMote,
	})
}

func spawnWeb(o rl.Vector3) {
	// Dropping strands — short vertical streaks that fall and fade.
	spawnRadialBurst(o, radialBurst{
		count: 12, vyOnly: true,
		posJitter: 0.20, yBase: 0.5, yJitter: 0.10,
		vyMin: -1.8, vyMax: -1.2,
		durMin: 0.5, durMax: 0.7, sizeStart: 0.10, sizeEnd: 0.14,
		colorStart: rl.NewColor(220, 200, 240, 220),
		colorEnd:   rl.NewColor(150, 110, 200, 0),
		shape:      shapeStrand,
	})
}

func spawnConfuse(o rl.Vector3) {
	// Swirling motes orbiting the target, in two color groups (gold + violet).
	for i := 0; i < 16; i++ {
		ang := randAngle()
		radius := 0.20 + vfxRNG.Float32()*0.18
		pushParticle(particle{
			X:          o.X + float32(math.Cos(float64(ang)))*radius,
			Y:          o.Y + 0.2 + vfxJitter(0.4),
			Z:          o.Z + float32(math.Sin(float64(ang)))*radius,
			VX:         -float32(math.Sin(float64(ang))) * 1.4,
			VY:         vfxJitter(0.4),
			VZ:         float32(math.Cos(float64(ang))) * 1.4,
			Duration:   0.7 + vfxRNG.Float32()*0.3,
			SizeStart:  0.08,
			SizeEnd:    0.04,
			ColorStart: pickConfuseTone(i),
			ColorEnd:   rl.NewColor(80, 64, 110, 0),
			Shape:      shapeMote,
		})
	}
}

func spawnIngest(o rl.Vector3) {
	// Inward green motes — the "being swallowed" feel.
	for i := 0; i < 14; i++ {
		ang := randAngle()
		radius := 0.6 + vfxRNG.Float32()*0.4
		startX := o.X + float32(math.Cos(float64(ang)))*radius
		startZ := o.Z + float32(math.Sin(float64(ang)))*radius
		pushParticle(particle{
			X: startX, Y: o.Y + 0.5 + vfxJitter(0.3), Z: startZ,
			VX:         (o.X - startX) * 2.0,
			VY:         -0.6,
			VZ:         (o.Z - startZ) * 2.0,
			Duration:   0.4 + vfxRNG.Float32()*0.2,
			SizeStart:  0.12,
			SizeEnd:    0.02,
			ColorStart: rl.NewColor(168, 220, 120, 230),
			ColorEnd:   rl.NewColor(64, 92, 48, 0),
			Shape:      shapeMote,
		})
	}
}

// confuseViolet is the odd-index tone of the Confuse swirl, paired with giltBright.
var confuseViolet = rl.NewColor(196, 132, 220, 255)

// pickConfuseTone alternates two tones for the Confuse swirl by even/odd index (no RNG roll).
func pickConfuseTone(i int) color.RGBA {
	if i&1 == 0 {
		return giltBright
	}
	return confuseViolet
}

// pushRing seeds one expanding ground-aligned ring — the shockwave companion to impact patterns.
func pushRing(o rl.Vector3, col color.RGBA, sizeStart, sizeEnd, duration float32) {
	pushParticle(particle{
		X: o.X, Y: 0.06, Z: o.Z,
		Duration:   duration,
		SizeStart:  sizeStart,
		SizeEnd:    sizeEnd,
		ColorStart: col,
		ColorEnd:   colorWithAlpha(col, 0),
		Shape:      shapeRing,
	})
}

// --- Drawing ----------------------------------------------------------------

// drawParticle renders one live particle: picks a primitive by shape, interpolates color+size by lifetime.
func drawParticle(p *particle) {
	if p.Duration <= 0 {
		// Guard the lifetime divide (drawParticle runs before the alive() cull).
		return
	}
	t := core.Clamp(p.Elapsed/p.Duration, 0, 1)
	size := p.SizeStart + (p.SizeEnd-p.SizeStart)*t
	col := core.MixColor(p.ColorStart, p.ColorEnd, float64(t))
	pos := rl.NewVector3(p.X, p.Y, p.Z)
	switch p.Shape {
	case shapeRing:
		// Ground-aligned ring; rotate around X so the circle lies flat (else it draws vertically).
		rl.DrawCircle3D(pos, size, rl.NewVector3(1, 0, 0), 90, col)
	case shapeStrand:
		// Tall thin sliver for the dripping-web look; spun about its vertical axis (p.Rotation).
		drawSpunCube(pos, rl.NewVector3(size*0.2, size, size*0.2), p.Rotation, col)
	case shapeShard:
		// Pointy diamond — a stretched cube, cheap icicle stand-in; spun about its vertical axis.
		drawSpunCube(pos, rl.NewVector3(size*0.55, size, size*0.55), p.Rotation, col)
	case shapeDust:
		// Soft chunky sphere.
		rl.DrawSphere(pos, size*0.5, col)
	case shapeSpark, shapeMote:
		fallthrough
	default:
		// Tight sphere — the default dot.
		rl.DrawSphere(pos, size*0.5, col)
	}
}

// drawSpunCube draws a centered cube rotated about its vertical axis so a particle's
// accumulated p.Rotation (driven by p.Spin) reads visually instead of being dead state.
func drawSpunCube(pos, dim rl.Vector3, rotation float32, col color.RGBA) {
	rl.PushMatrix()
	rl.Translatef(pos.X, pos.Y, pos.Z)
	rl.Rotatef(rotation, 0, 1, 0)
	rl.DrawCubeV(rl.Vector3{}, dim, col)
	rl.PopMatrix()
}

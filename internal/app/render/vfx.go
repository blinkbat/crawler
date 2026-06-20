package render

import (
	"image/color"
	"math"
	"math/rand"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// VFX particle pool. Owned by the render package because positions
// resolve from the camera each spawn — and the actual draw uses raylib
// billboards. Battle / explore code can't see this slice; they emit
// intents via core.EnqueueXxxVFX which TickAndDrawVFX consumes once
// per frame (drain queue → resolve anchor → seed particles → tick →
// draw).
//
// particleCapacity is the pre-reserved slice cap — sized to fit a
// Stoneslam AoE (~4 hits × 28 ground particles + lingering dust)
// plus a couple of follow-on impacts without reallocating.
// particleHardCap is the slice-length ceiling enforced by spawn
// helpers: when the pool reaches this size, new spawns are dropped
// rather than allowed to grow without bound. Picked at 4× capacity
// to swallow a worst-case "every party member casts Stoneslam-tier
// AoE on the same frame" scenario; anything past that is a sign of
// a stuck spawn loop and dropping is the right behaviour.
const (
	particleCapacity = 512
	particleHardCap  = 2048
)

// Sanity assert: the pre-reserve capacity must fit under the drop
// ceiling, otherwise a freshly-allocated pool would already be
// "full" by pushParticle's check and every spawn would silently
// drop. Catches a future tuning pass that inverts the constants by
// accident.
func init() {
	if particleCapacity > particleHardCap {
		panic("render: particleCapacity must be <= particleHardCap")
	}
}

var particles = make([]particle, 0, particleCapacity)

// ResetParticles drops every live particle. Called by the renderer
// in response to a core.RequestVFXReset signal from battle or
// explore (battle exit, area transition) so formation-relative
// particles from the previous context don't drift into the new
// scene at stale world positions.
func ResetParticles() {
	particles = particles[:0]
}

// pushParticle is the choke point every per-kind spawn pattern
// routes through. Drops the particle when the pool is at the hard
// cap so a runaway spawner can't unbounded-grow memory or starve
// frame time. Pool growth past particleCapacity reallocates once
// per growth ladder step; the hard cap prevents that from running
// forever.
func pushParticle(p particle) {
	if len(particles) >= particleHardCap {
		return
	}
	particles = append(particles, p)
}

// vfxRNG seeds particle direction / lifetime vfxJitter so two particle
// bursts on consecutive frames don't fan out identically. Independent
// of GameState.RNG because VFX are presentation-only — they don't
// need to be deterministic with gameplay rolls, and seeding off the
// game RNG would visibly bias damage rolls toward "more particles
// emitted" frames.
var vfxRNG = rand.New(rand.NewSource(0x5EED_F1AC))

// particleShape names the rendered silhouette family. Each shape is
// drawn with a small handful of raylib primitives (DrawBillboard or
// DrawCircle3D) so adding a new look is a switch row in drawParticle,
// not a new texture asset.
type particleShape uint8

const (
	shapeSpark particleShape = iota
	shapeMote
	shapeShard
	shapeRing
	shapeDust
	shapeStrand
)

// particle is one live FX dot. Position drifts under (velocity +
// gravity·t); size and color animate over [0, Duration]. Fields are
// stored as float32 so the slice packs tight — 4-byte alignment for
// the dominant data.
type particle struct {
	X, Y, Z       float32
	VX, VY, VZ    float32
	GX, GY, GZ    float32 // constant accel (gravity, drag substitute)
	Elapsed       float32
	Duration      float32
	SizeStart     float32
	SizeEnd       float32
	ColorStart    color.RGBA
	ColorEnd      color.RGBA
	Rotation      float32
	Spin          float32
	Shape         particleShape
	GroundAligned bool // ring/disc shapes drawn flat on the floor
}

// alive reports whether the particle still has lifetime left.
func (p *particle) alive() bool { return p.Elapsed < p.Duration }

// TickAndDrawVFX is the per-frame render entry point. Drains the
// game's VFX intent queue (spawning new particles), advances every
// live particle by raylib's frame dt, then issues billboard / disc
// draws for each. Must be called inside an rl.BeginMode3D / EndMode3D
// pair so particles depth-sort with the world.
func TickAndDrawVFX(camera rl.Camera3D, g *core.GameState, assets Resources) {
	// Reset request fires when battle / explore signals "drop every
	// live particle now" (battle exit + area transition + scene
	// return). Honored BEFORE draining the queue so the new frame's
	// spawns land into a clean pool rather than alongside stale
	// particles whose anchors no longer mean anything.
	if core.TakeVFXResetRequest(g) {
		ResetParticles()
		resetHitGlyphs()
		// Trailing HP-bar ghosts are presentation residue like the pools
		// above — drop them on the same scene-shape signal so a mid-drain
		// trail can't ghost into the next scene's first frame.
		resetBarGhosts()
	}
	for _, req := range core.DrainVFXQueue(g) {
		spawnFromRequest(camera, g, req, assets)
	}
	// Clamp dt to the same upper bound the gameplay layers use so a
	// long stall (debugger pause) doesn't fast-forward every particle
	// past its lifetime in one frame.
	dt := clampFrameDelta(rl.GetFrameTime())
	// Two-pointer sweep: advance + compact in place. Live particles
	// stay at the front of the slice; expired ones get overwritten
	// by the next live one. Keeps order stable enough for visual
	// purposes and avoids per-frame allocations.
	write := 0
	for read := range particles {
		p := &particles[read]
		// Draw at the current state BEFORE advancing, so each live particle
		// renders this frame at its current t (a freshly-spawned one at t=0)
		// and is guaranteed at least one visible frame even if this frame's
		// clamped dt would age it past its Duration. Advancing first (the old
		// order) could tick a sub-dt-lifetime particle straight past alive()
		// and cull it before it ever drew. Latent today — every effect's
		// Duration exceeds the dt clamp — but keeps a future short effect
		// honest. The one-frame physics lag this introduces is imperceptible.
		drawParticle(camera, p)
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

// resolveAnchor returns the world-space "spawn origin" for a VFX
// request. For enemy/party anchors this reuses the same camera-aware
// formation math the billboard draws use, so VFX land at the actor's
// visible spot rather than at the actor's logical tile (which would
// be wrong during battle, where formations float in front of the
// camera and don't correspond to a single tile).
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
	}
	return rl.Vector3{}, false
}

// spawnFromRequest dispatches a queued intent to the per-kind spawn
// pattern. Each pattern composes 1-30 particles. Keep particle counts
// small per spawn — many cheap dots read better than a few expensive
// ones, and our draw cost is dominated by billboard transforms.
func spawnFromRequest(camera rl.Camera3D, g *core.GameState, req core.VFXRequest, assets Resources) {
	origin, ok := resolveAnchor(camera, g, req)
	if !ok {
		return
	}
	// Per-enemy GFX tuning: a struck enemy KIND can nudge + resize its impact
	// particle burst and clarity glyph (authored in the Foe Visualizer). Only
	// enemy-anchored VFX are keyed by kind; party- and tile-anchored effects use
	// the raw resolved origin at 1×. The glyph keeps its own anchor (raw origin +
	// glyph offsets); `origin` is then rebound to the particle anchor so the
	// per-kind dispatch switch below stays untouched and scaleBurst scales the
	// burst around its own center.
	// glyphXOffset keeps only the HORIZONTAL glyph offset (X along camera-right);
	// every VERTICAL offset (per-kind glyphYOffset, party head-lift) is folded
	// into glyphRise, a world-Y amount the glyph applies in SCREEN space at draw.
	// Doing the lift in screen space keeps the glyph directly above an off-center
	// foe under the pitched battle camera (see spawnHitGlyph / DrawHitGlyphs).
	// Neither is baked into a world position here: the glyph stores the anchor
	// identity + these offsets and re-resolves its live position each frame, so it
	// tracks the target as it recoils/lunges (a frozen anchor drifted off
	// off-center foes — see hitGlyph).
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
		// Party sprites sit low and near the camera, projecting into the bottom
		// HUD band; lift the glyph above the member's head so the incoming-hit
		// cue is visible. (No per-kind tuning for party — that's enemy-only.)
		glyphRise = partyGlyphExtraRise
		// Per-CLASS GFX tuning, mirroring the enemy branch: a struck party
		// member's class can nudge + resize its hit glyph and impact burst
		// (authored in the Party Visualizer -> partyvisuals.json). Without
		// this the saved Glyph/Ptcl offsets loaded fine but never reached the
		// live FX, so tuning them "didn't seem to save."
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
		// Tile-anchored effects use the raw resolved origin at 1× — no per-kind
		// glyph/particle tuning, same as party. Explicit so a new VFXAnchor
		// can't silently inherit this no-op.
	default:
		if !loggedUnknownAnchor[req.Anchor] {
			loggedUnknownAnchor[req.Anchor] = true
			LogRenderError("spawnFromRequest: unhandled VFXAnchor %d — using raw origin at 1×", int(req.Anchor))
		}
	}
	// Snapshot the pool length so scaleBurst transforms only the particles THIS
	// request appends (every spawnXxx pushes onto the shared pool).
	from := len(particles)
	switch req.Kind {
	case core.VFXSlash:
		spawnSlash(origin)
	case core.VFXImpact:
		spawnImpact(origin)
	case core.VFXEmber:
		spawnEmber(origin)
	case core.VFXHeal:
		spawnHeal(origin)
	case core.VFXSmite:
		spawnSmite(origin)
	case core.VFXVenom:
		spawnVenom(origin)
	case core.VFXFrost:
		spawnFrost(origin)
	case core.VFXArc:
		spawnArc(origin)
	case core.VFXSteal:
		spawnSteal(origin)
	case core.VFXDeath:
		spawnDeath(origin)
	case core.VFXStoneslam:
		spawnStoneslam(origin)
	case core.VFXSleep:
		spawnSleep(origin)
	case core.VFXWeb:
		spawnWeb(origin)
	case core.VFXConfuse:
		spawnConfuse(origin)
	case core.VFXIngest:
		spawnIngest(origin)
	case core.VFXScan:
		spawnScan(origin)
	default:
		// A core.VFXKind with no spawn pattern here silently dropped its
		// effect before. Surface it once per unknown kind (the log stash
		// is bounded, so this can't spam) — a new kind that reaches the
		// queue without a case is a wiring gap, not a no-op.
		if !loggedUnknownVFX[req.Kind] {
			loggedUnknownVFX[req.Kind] = true
			LogRenderError("spawnFromRequest: no spawn pattern for VFX kind %d — effect dropped", int(req.Kind))
		}
	}
	// Uniformly scale the just-spawned burst (spread + dot size) by the kind's
	// particleScale around its anchor. A 1× scale is a no-op fast-path inside.
	scaleBurst(origin, from, particleScale)
	// Clarity glyph: a crisp 2D vector shape over the struck target, keyed by
	// the VFX kind (slash / impact / frost / spark / fire / holy / venom). Drawn
	// in the HUD pass by DrawHitGlyphs; glyphNone (heal / status / utility VFX)
	// is a no-op, so only damaging hits get one — and every existing impact VFX
	// gets it for free (player→foe and enemy→party alike). Anchored + sized by
	// the kind's glyph offsets/scale.
	spawnHitGlyph(hitGlyphForVFX(req.Kind), req, glyphXOffset, glyphDepth, glyphRise, glyphScale)
}

// enemyVisualForVFX resolves the per-kind enemyVisual for the enemy occupying
// battle slot, mirroring resolveAnchor's slot bounds check so a stale/out-of-
// range request can't panic. ok=false when the slot is empty or the kind has no
// visual — callers then fall back to an un-nudged, 1× burst/glyph.
func enemyVisualForVFX(g *core.GameState, assets Resources, slot int) (enemyVisual, bool) {
	members := core.BattleMembers(g)
	if slot < 0 || slot >= len(members) {
		return enemyVisual{}, false
	}
	return enemyVisualFor(assets, members[slot].Kind)
}

// scaleBurst uniformly resizes the particles in particles[from:] around anchor
// o: it scales each particle's XZ displacement from o, its velocity, its
// constant accel, and its start/end size by `scale`. Because velocity AND
// gravity scale together, the trajectory's TIMING is preserved while its
// spatial envelope (peak height, spread) scales linearly — so a 1.5× burst
// looks like the same burst, just bigger. Y POSITION is deliberately left
// untouched: ground rings/dust seed at an absolute floor Y, and scaling that
// around the torso-height anchor would punch them through the floor; scaling
// velocity + size already carries the size read. scale<=0 or ==1 is a no-op.
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

// loggedUnknownVFX dedupes the unknown-kind error so a missing spawn
// pattern logs once, not every frame the effect is requested. Touched
// only from the single-threaded render path.
var loggedUnknownVFX = map[core.VFXKind]bool{}

// loggedUnknownAnchor dedupes the unhandled-anchor warning (a new core.VFXAnchor
// reaching spawnFromRequest without a switch case) the same way, so it logs
// once per kind rather than every frame the effect is requested.
var loggedUnknownAnchor = map[core.VFXAnchor]bool{}

// --- Per-kind spawn patterns -----------------------------------------------
//
// Each pattern is a free function that pushes one or more particles
// into the pool. They share a small set of randomization helpers
// (vfxJitter, randAngle, etc.) so the noise feel is consistent across
// effects — and a single "make the bursts feel calmer" pass can tune
// every kind by turning the helpers' constants down.

func vfxJitter(scale float32) float32 {
	return (vfxRNG.Float32()*2 - 1) * scale
}

func randAngle() float32 {
	return vfxRNG.Float32() * tau
}

// radialBurst parameterizes the "fling N particles outward on a random
// horizontal heading" skeleton that nearly every impact/cast effect
// shares (slash, ember, smite spray, venom, frost, arc, steal, death,
// stoneslam). Each field is a literal the old hand-written loops baked
// in; speed / VY / duration are uniform [min,max] ranges. The struct +
// spawnRadialBurst collapse ~9 near-identical loop bodies so "calm every
// burst down" is a one-function tune, as the section comment promised.
type radialBurst struct {
	count              int
	speedMin, speedMax float32 // horizontal speed range (radial mode)
	horizScale         float32 // VX/VZ multiplier (treated as 1 when 0)
	// vyOnly switches off the angular horizontal fling: VX/VZ become a small
	// ± velJitter instead of speed-along-a-heading, so this skeleton also
	// covers the plain "rise / fall in place" emitters (heal, smite pillar,
	// sleep, web) that have no radial spread.
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

// uniform draws a value in [lo, hi] from vfxRNG. lo==hi returns the
// constant (still advances the stream, which is harmless — vfxRNG is
// presentation-only and never feeds gameplay rolls).
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
	// Bright crescent of fast outward sparks plus a quick ground
	// ring. The ring sells "impact happened HERE" while the sparks
	// sell direction and speed.
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
	// Blunt/percussive hit: a tight, fast pop of pale sparks + a small bright
	// ring right at the contact point. Punchier and more compact than the slash
	// crescent (fewer, shorter-lived sparks; a smaller ring) so it reads as a
	// "thud," not a "cut" — the unarmed/club/hammer and claw/bite/slam look.
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
	// Drifting orange embers + a hot core flash. Lives a bit longer
	// than slash so the player sees the burn linger.
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
	// Slow-rising green-gold motes. Stays light so the heal reads as
	// gentle, not explosive.
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
	// Short, harsh blue flashes — mimics a chain-zap without an
	// actual line-of-zap (too expensive to draw a polyline per arc).
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
	// Quick yellow star pop — small and fast so it reads as "pluck"
	// not "explosion."
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
	// Pale-cyan "reveal" pulse — a calm ring of motes drifting upward as
	// the foe is identified. Slow and soft (low speed, gentle rise, longer
	// life) so it reads as "studied," not "struck."
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
	// Drifting indigo blobs that wobble side to side — sells "Zzz."
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
	// Swirling motes orbiting the target. Two color groups (gold +
	// violet) so the swirl reads as the wisp's pair of tones.
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
	// Inward green motes — opposite direction of the others so the
	// "being swallowed" feel reads.
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

// pickConfuseTone returns one of two alternating color tones for the
// Confuse swirl. Even/odd indexing keeps the pattern visually mixed
// without needing a per-particle RNG roll.
func pickConfuseTone(i int) color.RGBA {
	if i&1 == 0 {
		return rl.NewColor(232, 196, 112, 255)
	}
	return rl.NewColor(196, 132, 220, 255)
}

// pushRing seeds one expanding ground-aligned ring. Used as the
// "shockwave" companion to several impact patterns — the ring tells
// the player WHERE the impact landed, the directional sparks tell
// them HOW HARD.
func pushRing(o rl.Vector3, col color.RGBA, sizeStart, sizeEnd, duration float32) {
	pushParticle(particle{
		X: o.X, Y: 0.06, Z: o.Z,
		Duration:      duration,
		SizeStart:     sizeStart,
		SizeEnd:       sizeEnd,
		ColorStart:    col,
		ColorEnd:      colorWithAlpha(col, 0),
		Shape:         shapeRing,
		GroundAligned: true,
	})
}

// --- Drawing ----------------------------------------------------------------

// drawParticle renders one live particle. Position has already been
// advanced by TickAndDrawVFX; this function just picks a primitive
// based on shape and interpolates color + size by lifetime.
func drawParticle(camera rl.Camera3D, p *particle) {
	if p.Duration <= 0 {
		// Guard the lifetime divide: drawParticle runs before the alive()
		// cull, so a zero-Duration particle would otherwise yield NaN/Inf t
		// and feed garbage into size/color/position.
		return
	}
	t := p.Elapsed / p.Duration
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	size := p.SizeStart + (p.SizeEnd-p.SizeStart)*t
	col := core.MixColor(p.ColorStart, p.ColorEnd, float64(t))
	pos := rl.NewVector3(p.X, p.Y, p.Z)
	switch p.Shape {
	case shapeRing:
		// Ground-aligned ring — drawn as a thick circle outline so
		// the shockwave reads even when the camera tilts away from
		// horizontal. raylib's DrawCircle3D takes radius + rotation
		// axis; rotate around the X axis so the circle lies flat
		// on the floor (otherwise it draws vertically).
		rl.DrawCircle3D(pos, size, rl.NewVector3(1, 0, 0), 90, col)
	case shapeStrand:
		// Tall, thin billboard for the "dripping web" look. Width
		// is a tiny fraction of size so the strand reads as a line
		// rather than a square.
		rl.DrawCubeV(pos, rl.NewVector3(size*0.2, size, size*0.2), col)
	case shapeShard:
		// Pointy diamond — cube scaled with one axis stretched.
		// Cheap stand-in for an icicle shape without authoring a
		// mesh.
		rl.DrawCubeV(pos, rl.NewVector3(size*0.55, size, size*0.55), col)
	case shapeDust:
		// Soft chunky sphere; dust clouds.
		rl.DrawSphere(pos, size*0.5, col)
	case shapeSpark, shapeMote:
		fallthrough
	default:
		// Tight sphere with a slight stretch on Y so it reads as a
		// dot from any camera angle. The default for everything
		// without a custom shape.
		rl.DrawSphere(pos, size*0.5, col)
	}
	_ = camera
}

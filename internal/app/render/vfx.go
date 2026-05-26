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
func TickAndDrawVFX(camera rl.Camera3D, g *core.GameState) {
	// Reset request fires when battle / explore signals "drop every
	// live particle now" (battle exit + area transition + scene
	// return). Honored BEFORE draining the queue so the new frame's
	// spawns land into a clean pool rather than alongside stale
	// particles whose anchors no longer mean anything.
	if core.TakeVFXResetRequest(g) {
		ResetParticles()
	}
	for _, req := range core.DrainVFXQueue(g) {
		spawnFromRequest(camera, g, req)
	}
	dt := rl.GetFrameTime()
	// Clamp dt to the same upper bound the gameplay layers use so a
	// long stall (debugger pause) doesn't fast-forward every particle
	// past its lifetime in one frame.
	if dt > 1.0/15.0 {
		dt = 1.0 / 15.0
	}
	// Two-pointer sweep: advance + compact in place. Live particles
	// stay at the front of the slice; expired ones get overwritten
	// by the next live one. Keeps order stable enough for visual
	// purposes and avoids per-frame allocations.
	write := 0
	for read := range particles {
		p := &particles[read]
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
	for i := range particles {
		drawParticle(camera, &particles[i])
	}
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
		return enemyDrawPosition(camera, *g, req.SlotIdx, members[req.SlotIdx]), true
	case core.VFXAnchorParty:
		if req.SlotIdx < 0 || req.SlotIdx >= len(g.Party) {
			return rl.Vector3{}, false
		}
		m := g.Party[req.SlotIdx]
		return partySpritePosition(camera, req.SlotIdx, m.Class, m.AttackBump, 0, m.HitKnockback), true
	case core.VFXAnchorTile:
		return tileWorldPos(req.TileX, req.TileZ, 0.05), true
	}
	return rl.Vector3{}, false
}

// spawnFromRequest dispatches a queued intent to the per-kind spawn
// pattern. Each pattern composes 1-30 particles. Keep particle counts
// small per spawn — many cheap dots read better than a few expensive
// ones, and our draw cost is dominated by billboard transforms.
func spawnFromRequest(camera rl.Camera3D, g *core.GameState, req core.VFXRequest) {
	origin, ok := resolveAnchor(camera, g, req)
	if !ok {
		return
	}
	switch req.Kind {
	case core.VFXSlash:
		spawnSlash(origin)
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
	}
}

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
	return vfxRNG.Float32() * 2 * math.Pi
}

func spawnSlash(o rl.Vector3) {
	// Bright crescent of fast outward sparks plus a quick ground
	// ring. The ring sells "impact happened HERE" while the sparks
	// sell direction and speed.
	for i := 0; i < 14; i++ {
		ang := randAngle()
		speed := 2.4 + vfxRNG.Float32()*1.6
		pushParticle(particle{
			X: o.X, Y: o.Y + 0.05 + vfxJitter(0.18), Z: o.Z,
			VX: float32(math.Cos(float64(ang))) * speed,
			VY: 0.4 + vfxJitter(0.6),
			VZ: float32(math.Sin(float64(ang))) * speed,
			GY:         -3.2,
			Duration:   0.32 + vfxRNG.Float32()*0.12,
			SizeStart:  0.10,
			SizeEnd:    0.02,
			ColorStart: rl.NewColor(255, 248, 200, 255),
			ColorEnd:   rl.NewColor(248, 196, 110, 0),
			Shape:      shapeSpark,
		})
	}
	pushRing(o, rl.NewColor(255, 232, 168, 220), 0.18, 1.1, 0.32)
}

func spawnEmber(o rl.Vector3) {
	// Drifting orange embers + a hot core flash. Lives a bit longer
	// than slash so the player sees the burn linger.
	for i := 0; i < 22; i++ {
		ang := randAngle()
		speed := 0.9 + vfxRNG.Float32()*1.6
		pushParticle(particle{
			X: o.X + vfxJitter(0.08), Y: o.Y + vfxJitter(0.18), Z: o.Z + vfxJitter(0.08),
			VX: float32(math.Cos(float64(ang))) * speed * 0.6,
			VY: 0.9 + vfxRNG.Float32()*1.2,
			VZ: float32(math.Sin(float64(ang))) * speed * 0.6,
			GY:         -1.4,
			Duration:   0.55 + vfxRNG.Float32()*0.30,
			SizeStart:  0.16,
			SizeEnd:    0.04,
			ColorStart: rl.NewColor(255, 196, 96, 255),
			ColorEnd:   rl.NewColor(180, 50, 30, 0),
			Shape:      shapeMote,
		})
	}
	pushRing(o, rl.NewColor(255, 152, 80, 220), 0.20, 1.4, 0.38)
}

func spawnHeal(o rl.Vector3) {
	// Slow-rising green-gold motes. Stays light so the heal reads as
	// gentle, not explosive.
	for i := 0; i < 16; i++ {
		pushParticle(particle{
			X: o.X + vfxJitter(0.22), Y: o.Y - 0.2, Z: o.Z + vfxJitter(0.22),
			VX:         vfxJitter(0.25),
			VY:         0.6 + vfxRNG.Float32()*0.5,
			VZ:         vfxJitter(0.25),
			Duration:   0.9 + vfxRNG.Float32()*0.4,
			SizeStart:  0.09,
			SizeEnd:    0.14,
			ColorStart: rl.NewColor(212, 248, 196, 255),
			ColorEnd:   rl.NewColor(132, 200, 96, 0),
			Shape:      shapeMote,
		})
	}
}

func spawnSmite(o rl.Vector3) {
	// Vertical pillar of light shards descending then bursting out.
	for i := 0; i < 10; i++ {
		pushParticle(particle{
			X: o.X + vfxJitter(0.10), Y: o.Y + 0.9 + vfxJitter(0.20), Z: o.Z + vfxJitter(0.10),
			VY:         -3.0,
			Duration:   0.28 + vfxRNG.Float32()*0.10,
			SizeStart:  0.18,
			SizeEnd:    0.05,
			ColorStart: rl.NewColor(255, 248, 196, 255),
			ColorEnd:   rl.NewColor(232, 196, 112, 0),
			Shape:      shapeShard,
		})
	}
	for i := 0; i < 10; i++ {
		ang := randAngle()
		speed := 1.8 + vfxRNG.Float32()*1.2
		pushParticle(particle{
			X: o.X, Y: o.Y + 0.05, Z: o.Z,
			VX: float32(math.Cos(float64(ang))) * speed,
			VY: 0.4,
			VZ: float32(math.Sin(float64(ang))) * speed,
			GY:         -2.4,
			Duration:   0.36,
			SizeStart:  0.10,
			SizeEnd:    0.02,
			ColorStart: rl.NewColor(255, 240, 168, 255),
			ColorEnd:   rl.NewColor(212, 152, 64, 0),
			Shape:      shapeSpark,
		})
	}
	pushRing(o, rl.NewColor(255, 248, 168, 230), 0.22, 1.4, 0.42)
}

func spawnVenom(o rl.Vector3) {
	// Lazy green mist; lingers longer than the others.
	for i := 0; i < 18; i++ {
		ang := randAngle()
		speed := 0.4 + vfxRNG.Float32()*0.6
		pushParticle(particle{
			X: o.X + vfxJitter(0.10), Y: o.Y + vfxJitter(0.20), Z: o.Z + vfxJitter(0.10),
			VX: float32(math.Cos(float64(ang))) * speed,
			VY: 0.3 + vfxJitter(0.15),
			VZ: float32(math.Sin(float64(ang))) * speed,
			Duration:   0.7 + vfxRNG.Float32()*0.4,
			SizeStart:  0.14,
			SizeEnd:    0.22,
			ColorStart: rl.NewColor(176, 232, 132, 220),
			ColorEnd:   rl.NewColor(96, 140, 64, 0),
			Shape:      shapeMote,
		})
	}
}

func spawnFrost(o rl.Vector3) {
	// Sharp expanding diamond shards.
	for i := 0; i < 18; i++ {
		ang := randAngle()
		speed := 2.0 + vfxRNG.Float32()*1.2
		pushParticle(particle{
			X: o.X, Y: o.Y + vfxJitter(0.20), Z: o.Z,
			VX: float32(math.Cos(float64(ang))) * speed,
			VY: 0.2 + vfxJitter(0.4),
			VZ: float32(math.Sin(float64(ang))) * speed,
			GY:         -1.6,
			Duration:   0.4 + vfxRNG.Float32()*0.18,
			SizeStart:  0.08,
			SizeEnd:    0.18,
			Spin:       (vfxRNG.Float32()*2 - 1) * 6,
			ColorStart: rl.NewColor(220, 240, 255, 255),
			ColorEnd:   rl.NewColor(104, 168, 224, 0),
			Shape:      shapeShard,
		})
	}
	pushRing(o, rl.NewColor(180, 220, 255, 220), 0.20, 1.3, 0.32)
}

func spawnArc(o rl.Vector3) {
	// Short, harsh blue flashes — mimics a chain-zap without an
	// actual line-of-zap (too expensive to draw a polyline per arc).
	for i := 0; i < 14; i++ {
		ang := randAngle()
		speed := 2.8 + vfxRNG.Float32()*1.0
		pushParticle(particle{
			X: o.X, Y: o.Y + vfxJitter(0.30), Z: o.Z,
			VX: float32(math.Cos(float64(ang))) * speed,
			VY: vfxJitter(0.8),
			VZ: float32(math.Sin(float64(ang))) * speed,
			Duration:   0.18,
			SizeStart:  0.10,
			SizeEnd:    0.02,
			ColorStart: rl.NewColor(196, 224, 255, 255),
			ColorEnd:   rl.NewColor(96, 132, 232, 0),
			Shape:      shapeSpark,
		})
	}
}

func spawnSteal(o rl.Vector3) {
	// Quick yellow star pop — small and fast so it reads as "pluck"
	// not "explosion."
	for i := 0; i < 8; i++ {
		ang := randAngle()
		speed := 1.6 + vfxRNG.Float32()*0.6
		pushParticle(particle{
			X: o.X, Y: o.Y + 0.1, Z: o.Z,
			VX: float32(math.Cos(float64(ang))) * speed,
			VY: 0.8 + vfxRNG.Float32()*0.4,
			VZ: float32(math.Sin(float64(ang))) * speed,
			GY:         -2.4,
			Duration:   0.30,
			SizeStart:  0.12,
			SizeEnd:    0.02,
			ColorStart: rl.NewColor(255, 240, 168, 255),
			ColorEnd:   rl.NewColor(232, 196, 96, 0),
			Shape:      shapeSpark,
		})
	}
}

func spawnDeath(o rl.Vector3) {
	// Gray dispersing dust.
	for i := 0; i < 16; i++ {
		ang := randAngle()
		speed := 0.8 + vfxRNG.Float32()*0.6
		pushParticle(particle{
			X: o.X, Y: o.Y + vfxJitter(0.20), Z: o.Z,
			VX: float32(math.Cos(float64(ang))) * speed,
			VY: 0.2 + vfxJitter(0.2),
			VZ: float32(math.Sin(float64(ang))) * speed,
			GY:         -1.0,
			Duration:   0.7 + vfxRNG.Float32()*0.4,
			SizeStart:  0.14,
			SizeEnd:    0.24,
			ColorStart: rl.NewColor(184, 168, 152, 220),
			ColorEnd:   rl.NewColor(96, 88, 84, 0),
			Shape:      shapeDust,
		})
	}
}

func spawnStoneslam(o rl.Vector3) {
	// Big heavy dust cloud at the ground + outward shockwave.
	for i := 0; i < 28; i++ {
		ang := randAngle()
		speed := 1.4 + vfxRNG.Float32()*1.2
		pushParticle(particle{
			X: o.X + vfxJitter(0.10), Y: 0.05 + vfxJitter(0.05), Z: o.Z + vfxJitter(0.10),
			VX: float32(math.Cos(float64(ang))) * speed,
			VY: 0.8 + vfxRNG.Float32()*0.6,
			VZ: float32(math.Sin(float64(ang))) * speed,
			GY:         -1.6,
			Duration:   0.7 + vfxRNG.Float32()*0.4,
			SizeStart:  0.22,
			SizeEnd:    0.36,
			ColorStart: rl.NewColor(168, 152, 128, 220),
			ColorEnd:   rl.NewColor(86, 76, 64, 0),
			Shape:      shapeDust,
		})
	}
	pushRing(rl.NewVector3(o.X, 0.05, o.Z), rl.NewColor(196, 184, 152, 240), 0.32, 2.2, 0.5)
}

func spawnSleep(o rl.Vector3) {
	// Drifting indigo blobs that wobble side to side — sells "Zzz."
	for i := 0; i < 8; i++ {
		pushParticle(particle{
			X: o.X + vfxJitter(0.18), Y: o.Y + 0.1, Z: o.Z + vfxJitter(0.18),
			VX:         vfxJitter(0.20),
			VY:         0.35 + vfxRNG.Float32()*0.25,
			VZ:         vfxJitter(0.20),
			Duration:   1.0 + vfxRNG.Float32()*0.4,
			SizeStart:  0.10,
			SizeEnd:    0.18,
			ColorStart: rl.NewColor(168, 200, 240, 220),
			ColorEnd:   rl.NewColor(96, 132, 192, 0),
			Shape:      shapeMote,
		})
	}
}

func spawnWeb(o rl.Vector3) {
	// Dropping strands — short vertical streaks that fall and fade.
	for i := 0; i < 12; i++ {
		pushParticle(particle{
			X: o.X + vfxJitter(0.20), Y: o.Y + 0.5 + vfxJitter(0.10), Z: o.Z + vfxJitter(0.20),
			VY:         -1.2 - vfxRNG.Float32()*0.6,
			Duration:   0.5 + vfxRNG.Float32()*0.2,
			SizeStart:  0.10,
			SizeEnd:    0.14,
			ColorStart: rl.NewColor(220, 200, 240, 220),
			ColorEnd:   rl.NewColor(150, 110, 200, 0),
			Shape:      shapeStrand,
		})
	}
}

func spawnConfuse(o rl.Vector3) {
	// Swirling motes orbiting the target. Two color groups (gold +
	// violet) so the swirl reads as the wisp's pair of tones.
	for i := 0; i < 16; i++ {
		ang := randAngle()
		radius := 0.20 + vfxRNG.Float32()*0.18
		pushParticle(particle{
			X: o.X + float32(math.Cos(float64(ang)))*radius,
			Y: o.Y + 0.2 + vfxJitter(0.4),
			Z: o.Z + float32(math.Sin(float64(ang)))*radius,
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

// (lerpColor removed — callers route through core.MixColor with a
// float64(t) cast. The render package no longer carries its own
// per-channel-lerp duplicate.)

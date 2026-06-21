package core

// VFX (visual-effect) intent layer. Battle and explore code don't get
// to instantiate particles themselves — they push VFXRequest values
// onto GameState.VFXQueue, and the render side drains the queue each
// frame, resolves camera-relative world positions, and emits actual
// particles into a render-package pool.
//
// Why a queue instead of "battle calls render directly":
//   - Battle has no access to the rl.Camera3D (raylib is render-side
//     only by package layering convention here).
//   - Many VFX targets are formation slots whose world position is
//     re-computed every frame from the camera; capturing a stale XYZ
//     at spawn time would drift if the player rotates mid-frame.
//   - Tests can assert "Firebolt enqueued a VFX request" without
//     pulling raylib into the test binary's load path.
//
// The queue is drained completely each render frame; ordering is
// preserved so chained effects (impact spark → ring shockwave) read
// in the order the apply function emitted them.

// VFXKind enumerates the per-skill / per-event visual styles. Each
// kind binds to a spawn function on the render side that decides the
// particle pattern (burst, drifting motes, expanding ring, etc.).
// Adding a new kind: append a row here, add the dispatch case in
// render/vfx.go's spawnFromRequest, and the new effect lights up
// wherever a caller emits it.
type VFXKind int

const (
	VFXNone VFXKind = iota
	// Bladed melee — a quick crescent slash stroke + spark cluster. Used by
	// edged basic attacks (sword/axe/dagger/spear) and the bladed melee skills
	// (Swipe, Backstab, Whirlwind, Crushing Blow).
	VFXSlash
	// Blunt/percussive melee — a compact "thud" pop + impact ring. Used by
	// unarmed fists, blunt weapons (club/hammer), ranged projectile strikes, and
	// enemy claw/bite/slam basic attacks. Maps to the impact clarity glyph.
	VFXImpact
	// Firebolt impact — orange/red embers drifting upward + a
	// short-lived shockwave ring at ground.
	VFXEmber
	// Prayer / Mass Mend — green-gold motes rising over the target.
	VFXHeal
	// Smite — golden vertical pillar of light shards.
	VFXSmite
	// Venom Strike — green wispy mist that puffs out.
	VFXVenom
	// Frost Lance — pale-blue diamond shards expanding outward.
	VFXFrost
	// Arc Bolt — quick electric-blue arcs.
	VFXArc
	// Steal — small yellow star pop.
	VFXSteal
	// Enemy death — gray dispersing dust.
	VFXDeath
	// Stone slam — heavy gray-brown dust kicked up at ground.
	VFXStoneslam
	// Sleep apply — drifting indigo Z-blobs.
	VFXSleep
	// Web apply — purple sticky strands.
	VFXWeb
	// Confuse apply — yellow-purple swirl.
	VFXConfuse
	// Ingest apply — green throat motion (dragged toward enemy).
	VFXIngest
	// Scan — a quick pale-cyan "reveal" ring rising off the target as the
	// Thief identifies it. Information cue, no impact feel.
	VFXScan
	// VFXKindCount is the number of VFX kinds (including VFXNone). Render-side
	// tables that must cover every kind (the spawn dispatch, the hit-glyph map)
	// length-check against this so a newly appended kind that's missed in one of
	// them fails loudly instead of silently dropping its effect/glyph.
	VFXKindCount
)

// VFXAnchor names what target's world position the renderer should
// resolve when materialising the request into particles.
type VFXAnchor int

const (
	// VFXAnchorEnemy: SlotIdx indexes the active pack's Members.
	VFXAnchorEnemy VFXAnchor = iota
	// VFXAnchorParty: SlotIdx indexes g.Party.
	VFXAnchorParty
	// VFXAnchorTile: TileX/TileZ identify a world tile (used for
	// out-of-battle effects, future puzzle FX, ground-anchored
	// AoE shockwaves).
	VFXAnchorTile
)

// VFXRequest is one queued spawn intent. Battle / explore code build
// these and append to GameState.VFXQueue; the render layer drains
// the queue each frame.
type VFXRequest struct {
	Kind    VFXKind
	Anchor  VFXAnchor
	SlotIdx int
	TileX   int
	TileZ   int
}

// EnqueueEnemyVFX appends a VFX request anchored to an active-pack
// enemy slot. The slot is NOT range-checked here — the render-side
// drain drops requests whose slot is out of range, so callers can
// chain after a damageEnemy() without re-checking the kill status.
func EnqueueEnemyVFX(g *GameState, kind VFXKind, slot int) {
	if g == nil || kind == VFXNone {
		return
	}
	g.VFXQueue = append(g.VFXQueue, VFXRequest{Kind: kind, Anchor: VFXAnchorEnemy, SlotIdx: slot})
}

// EnqueuePartyVFX appends a VFX request anchored to a party slot. The
// slot is NOT range-checked here — the render-side drain drops
// out-of-range requests, so heal helpers can call without first
// validating.
func EnqueuePartyVFX(g *GameState, kind VFXKind, slot int) {
	if g == nil || kind == VFXNone {
		return
	}
	g.VFXQueue = append(g.VFXQueue, VFXRequest{Kind: kind, Anchor: VFXAnchorParty, SlotIdx: slot})
}

// EnqueueTileVFX appends a VFX request anchored to a world tile.
// Intended for out-of-battle effects (future puzzle plates / door
// activations) and ground-anchored AoE shockwaves where the floor is
// the visual centre.
//
// DEFERRED: nothing enqueues a tile-anchored request yet. The render
// side already materialises VFXAnchorTile (render/vfx.go), so this is
// the producer half waiting on its first caller — kept wired so the
// future feature plugs in without re-deriving the anchor plumbing. Not
// a live code path today.
func EnqueueTileVFX(g *GameState, kind VFXKind, tileX, tileZ int) {
	if g == nil || kind == VFXNone {
		return
	}
	g.VFXQueue = append(g.VFXQueue, VFXRequest{Kind: kind, Anchor: VFXAnchorTile, TileX: tileX, TileZ: tileZ})
}

// DrainVFXQueue returns the pending requests and clears the queue.
// Render calls this once per frame; the caller is responsible for
// materialising each request into particles before the queue refills
// next frame.
//
// The live queue is swapped with a spare back-buffer rather than re-sliced
// in place, so the returned slice is NOT aliased by g.VFXQueue: a spawn
// handler that enqueues a follow-on VFX during the drain appends to the
// fresh (empty) buffer, not the slice the caller is mid-iteration over.
// The two buffers ping-pong, so capacity is still reused frame to frame and
// battle-heavy frames don't keep reallocating.
func DrainVFXQueue(g *GameState) []VFXRequest {
	if g == nil || len(g.VFXQueue) == 0 {
		return nil
	}
	out := g.VFXQueue
	g.VFXQueue = g.vfxQueueSpare[:0]
	g.vfxQueueSpare = out
	return out
}

// RequestVFXReset signals the render layer that every live particle
// should be dropped before the next frame. Battle and explore call
// this on scene-shape changes (battle exit, area transition) so
// formation-relative particles from the previous context don't
// drift into the new one. Render reads + clears the flag in
// TickAndDrawVFX.
func RequestVFXReset(g *GameState) {
	if g == nil {
		return
	}
	g.VFXResetRequested = true
}

// TakeVFXResetRequest reads the reset flag and clears it. Render
// calls this once per frame; a true return means "drop the
// particle pool before processing this frame's queue."
func TakeVFXResetRequest(g *GameState) bool {
	if g == nil || !g.VFXResetRequested {
		return false
	}
	g.VFXResetRequested = false
	return true
}

package core

// VFX (visual-effect) intent layer. Battle/explore push VFXRequest values onto
// GameState.VFXQueue; render drains the queue each frame, resolves
// camera-relative world positions, and emits particles into a render pool.
//
// Why a queue (not "battle calls render directly"): battle has no rl.Camera3D
// (raylib is render-side only); many targets are formation slots whose world
// position is recomputed per frame, so a captured XYZ would drift; and tests
// can assert "X enqueued a VFX request" without pulling raylib into the binary.
//
// Drained completely each frame, order preserved so chained effects read in
// emit order.

// VFXKind enumerates the per-skill / per-event visual styles, each bound to a
// render-side spawn function. Adding a kind: append a row here + add the
// dispatch case in render/vfx.go's spawnFromRequest.
type VFXKind int

const (
	VFXNone VFXKind = iota
	// Bladed melee — crescent slash + spark cluster (edged attacks, bladed skills).
	VFXSlash
	// Blunt melee — "thud" pop + impact ring (fists, blunt weapons, ranged, enemy claw/bite/slam).
	VFXImpact
	// Firebolt impact — embers drifting up + short-lived ground shockwave ring.
	VFXEmber
	// Prayer / Mass Mend — green-gold motes rising over the target.
	VFXHeal
	// Smite — golden vertical pillar of light shards.
	VFXSmite
	// Venom Strike — green wispy mist puff.
	VFXVenom
	// Frost Lance — pale-blue diamond shards expanding outward.
	VFXFrost
	// Arc Bolt — quick electric-blue arcs.
	VFXArc
	// Steal — small yellow star pop.
	VFXSteal
	// Enemy death — gray dispersing dust.
	VFXDeath
	// Stone slam — heavy gray-brown dust at ground.
	VFXStoneslam
	// Sleep apply — drifting indigo Z-blobs.
	VFXSleep
	// Web apply — purple sticky strands.
	VFXWeb
	// Confuse apply — yellow-purple swirl.
	VFXConfuse
	// Ingest apply — green throat motion (dragged toward enemy).
	VFXIngest
	// Scan — pale-cyan reveal ring rising off the target (info cue, no impact).
	VFXScan
	// VFXKindCount is the kind count (incl. VFXNone). Render tables that must
	// cover every kind length-check against this so a missed kind fails loudly.
	VFXKindCount
)

// VFXAnchor names whose world position render resolves when materialising the
// request into particles.
type VFXAnchor int

const (
	// VFXAnchorEnemy: SlotIdx indexes the active pack's Members.
	VFXAnchorEnemy VFXAnchor = iota
	// VFXAnchorParty: SlotIdx indexes g.Party.
	VFXAnchorParty
	// VFXAnchorTile: TileX/TileZ identify a world tile (out-of-battle effects,
	// ground-anchored AoE shockwaves).
	VFXAnchorTile
)

// VFXRequest is one queued spawn intent appended to GameState.VFXQueue.
type VFXRequest struct {
	Kind    VFXKind
	Anchor  VFXAnchor
	SlotIdx int
	TileX   int
	TileZ   int
}

// EnqueueEnemyVFX appends a request anchored to an active-pack enemy slot. The
// slot is NOT range-checked — the render drain drops out-of-range requests, so
// callers can chain after damageEnemy() without re-checking kill status.
func EnqueueEnemyVFX(g *GameState, kind VFXKind, slot int) {
	if g == nil || kind == VFXNone {
		return
	}
	g.VFXQueue = append(g.VFXQueue, VFXRequest{Kind: kind, Anchor: VFXAnchorEnemy, SlotIdx: slot})
}

// EnqueuePartyVFX appends a request anchored to a party slot. The slot is NOT
// range-checked — the render drain drops out-of-range requests.
func EnqueuePartyVFX(g *GameState, kind VFXKind, slot int) {
	if g == nil || kind == VFXNone {
		return
	}
	g.VFXQueue = append(g.VFXQueue, VFXRequest{Kind: kind, Anchor: VFXAnchorParty, SlotIdx: slot})
}

// EnqueueTileVFX appends a request anchored to a world tile.
//
// DEFERRED: nothing enqueues a tile-anchored request yet (render already
// materialises VFXAnchorTile). Kept wired as the producer half awaiting its
// first caller. Not a live code path today.
func EnqueueTileVFX(g *GameState, kind VFXKind, tileX, tileZ int) {
	if g == nil || kind == VFXNone {
		return
	}
	g.VFXQueue = append(g.VFXQueue, VFXRequest{Kind: kind, Anchor: VFXAnchorTile, TileX: tileX, TileZ: tileZ})
}

// DrainVFXQueue returns the pending requests and clears the queue (called once
// per frame).
//
// Swaps the live queue with a spare back-buffer rather than re-slicing in
// place, so the returned slice is NOT aliased by g.VFXQueue: a follow-on VFX
// enqueued during the drain lands in the fresh buffer, not the slice being
// iterated. The two ping-pong, reusing capacity frame to frame.
func DrainVFXQueue(g *GameState) []VFXRequest {
	if g == nil || len(g.VFXQueue) == 0 {
		return nil
	}
	out := g.VFXQueue
	g.VFXQueue = g.vfxQueueSpare[:0]
	g.vfxQueueSpare = out
	return out
}

// RequestVFXReset signals render to drop every live particle before the next
// frame. Called on scene-shape changes (battle exit, area transition) so
// formation-relative particles don't drift into the new context.
func RequestVFXReset(g *GameState) {
	if g == nil {
		return
	}
	g.VFXResetRequested = true
}

// TakeVFXResetRequest reads and clears the reset flag (once per frame). A true
// return means "drop the particle pool before processing this frame's queue."
func TakeVFXResetRequest(g *GameState) bool {
	if g == nil || !g.VFXResetRequested {
		return false
	}
	g.VFXResetRequested = false
	return true
}

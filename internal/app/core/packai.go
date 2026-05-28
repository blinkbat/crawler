package core

import "slices"

// Junkyard-dog pack AI. Packs roam within a small leash around their
// spawn tile, occasionally taking a step when the player does. They
// step toward the player when the player is close, otherwise drift to
// a random open neighbor that keeps them inside the leash. They don't
// chase outside the leash and they don't move every step — the goal
// is "this dog noticed you walk by and decided to look up," not "you
// have a hostile escort following you across the map."
//
// All randomness comes from GameState.RNG so the seed travels with the
// run state (tests / future save-load) instead of leaking onto a
// package-level singleton.

// PackStepIntoPlayer reports whether a pack at (tx, tz) and the player
// at (px, pz) share a tile — the collision condition that should
// trigger a battle. Lives here (not in explore) so the headless tests
// covering pack-AI can assert it without dragging raylib in.
func PackStepIntoPlayer(tx, tz, px, pz int) bool {
	return tx == px && tz == pz
}

// cardinalSteps returns the four cardinal-direction (dx, dz) pairs in
// FacingVector order (North, East, South, West). Builds from
// FacingVector so the AI and the player-step code can't disagree on
// what "south" means — a future facing-convention change is one switch
// edit, not a manual table reshuffle.
func cardinalSteps() [FacingCount][2]int {
	var out [FacingCount][2]int
	for i := 0; i < FacingCount; i++ {
		dx, dz := FacingVector(i)
		out[i] = [2]int{dx, dz}
	}
	return out
}

// packAIStep is one pack's per-step move plan. PackEngagePlayer is the
// post-move state: if a pack stepped onto the player's tile, the
// caller initiates a battle with this pack index instead of applying
// the move (the pack visually "stays" at the engagement tile since
// battle Start uses Pack.TileX/TileZ for the splash).
type packAIStep struct {
	PackIdx      int
	NextX        int
	NextZ        int
	EngagePlayer bool
	Moved        bool
}

// PlanPackSteps walks every alive pack and returns the moves they
// chose this player-step. Pure planner — no mutation, no battle init,
// no animation. The caller (explore.movement) applies the moves
// sequentially and triggers a battle when a pack walks onto the
// player.
//
// Rules:
//   - Roll PackStepChance per pack; skip if it fails.
//   - If the player is within PackChaseRadius (Chebyshev), step the
//     pack one tile closer along the axis with the largest delta
//     (ties: prefer X). Skip if that tile is blocked or occupied.
//   - Otherwise wander: pick a random cardinal neighbor that's open,
//     unoccupied, and still inside the leash. If none qualifies,
//     don't move.
//
// Moves are checked against the area's static blockers (BlockedAt:
// walls, props, deep water), runtime chests, doors, and other packs'
// CURRENT tiles. The player's tile is allowed as a destination only
// when the pack is doing the chase branch — that's the engagement
// condition; the wander branch refuses to step onto the player so a
// passive dog wandering past doesn't accidentally start a fight.
func PlanPackSteps(g *GameState) []packAIStep {
	if g == nil {
		return nil
	}
	plans := make([]packAIStep, 0, len(g.Packs))
	occupied := buildPackOccupancy(g.Packs, -1)
	for i := range g.Packs {
		if !PackAlive(g.Packs[i]) {
			continue
		}
		plan, ok := planOnePack(g, i, occupied)
		if !ok {
			continue
		}
		// Reserve the destination so a later pack doesn't plan into
		// the same square this frame.
		delete(occupied, [2]int{g.Packs[i].TileX, g.Packs[i].TileZ})
		occupied[[2]int{plan.NextX, plan.NextZ}] = true
		plans = append(plans, plan)
	}
	return plans
}

func planOnePack(g *GameState, idx int, occupied map[[2]int]bool) (packAIStep, bool) {
	if g.RNG.Float32() >= PackStepChance {
		return packAIStep{}, false
	}
	p := g.Packs[idx]
	px, pz := g.Player.TileX, g.Player.TileZ

	// Chase branch: player inside the chase radius. Pick the cardinal
	// step that closes the larger of (dx, dz) — feels deliberate, not
	// random.
	if ChebyshevDistance(p.TileX, p.TileZ, px, pz) <= PackChaseRadius {
		nx, nz, ok := chaseStep(g, p, occupied, px, pz)
		if !ok {
			return packAIStep{}, false
		}
		engage := PackStepIntoPlayer(nx, nz, px, pz)
		return packAIStep{PackIdx: idx, NextX: nx, NextZ: nz, EngagePlayer: engage, Moved: true}, true
	}

	// Wander branch: pick one of the open cardinal neighbors that
	// stays inside the leash and isn't the player's tile.
	nx, nz, ok := wanderStep(g, p, occupied, px, pz)
	if !ok {
		return packAIStep{}, false
	}
	return packAIStep{PackIdx: idx, NextX: nx, NextZ: nz, Moved: true}, true
}

func chaseStep(g *GameState, p Pack, occupied map[[2]int]bool, px, pz int) (int, int, bool) {
	dx := px - p.TileX
	dz := pz - p.TileZ
	// Prefer the longer axis. If both are equal, X first — arbitrary
	// but deterministic so the chase doesn't visually dither.
	steps := [4][2]int{}
	n := 0
	if AbsInt(dx) >= AbsInt(dz) {
		if dx != 0 {
			steps[n] = [2]int{Sign(dx), 0}
			n++
		}
		if dz != 0 {
			steps[n] = [2]int{0, Sign(dz)}
			n++
		}
	} else {
		if dz != 0 {
			steps[n] = [2]int{0, Sign(dz)}
			n++
		}
		if dx != 0 {
			steps[n] = [2]int{Sign(dx), 0}
			n++
		}
	}
	for i := 0; i < n; i++ {
		tx, tz := p.TileX+steps[i][0], p.TileZ+steps[i][1]
		if !packCanMoveTo(g, p, occupied, tx, tz, true /* allow player tile */, px, pz) {
			continue
		}
		return tx, tz, true
	}
	return 0, 0, false
}

func wanderStep(g *GameState, p Pack, occupied map[[2]int]bool, px, pz int) (int, int, bool) {
	cardinals := cardinalSteps()
	// Shuffle in place via Fisher-Yates against the shared RNG so the
	// wander direction is independent of FacingVector's array order.
	for i := 3; i > 0; i-- {
		j := g.RNG.Intn(i + 1)
		cardinals[i], cardinals[j] = cardinals[j], cardinals[i]
	}
	for _, c := range cardinals {
		tx, tz := p.TileX+c[0], p.TileZ+c[1]
		if ChebyshevDistance(tx, tz, p.HomeX, p.HomeZ) > PackLeashRadius {
			continue
		}
		if !packCanMoveTo(g, p, occupied, tx, tz, false /* refuse player tile */, px, pz) {
			continue
		}
		return tx, tz, true
	}
	return 0, 0, false
}

// packCanMoveTo is the pack-flavored entry of CanEnterTile: packs are
// never allowed onto doors (area transitions are player-only), they
// avoid other packs, and they only step onto the player tile when
// allowPlayer is set (the chase branch passes true to encode an
// engagement; the wander branch passes false so a passive dog doesn't
// accidentally start a fight). px/pz let the caller declare the
// player's tile without making the global player position a hidden
// dependency.
func packCanMoveTo(g *GameState, p Pack, occupied map[[2]int]bool, tx, tz int, allowPlayer bool, px, pz int) bool {
	_ = p
	return CanEnterTile(g, tx, tz, EnterOpts{
		AllowDoorTile:   false,
		AllowPlayerTile: allowPlayer,
		PlayerTileX:     px,
		PlayerTileZ:     pz,
		OccupiedPacks:   occupied,
	})
}

// buildPackOccupancy returns the set of tiles currently held by alive
// packs, optionally excluding a specific pack index (for the
// "where am I allowed to move to" check that shouldn't see the
// moving pack's own tile as blocked).
func buildPackOccupancy(packs []Pack, exclude int) map[[2]int]bool {
	occ := make(map[[2]int]bool, len(packs))
	for i, p := range packs {
		if i == exclude {
			continue
		}
		if !PackAlive(p) {
			continue
		}
		occ[[2]int{p.TileX, p.TileZ}] = true
	}
	return occ
}

// TickPackAnimations advances every alive pack's step animation by
// dt seconds. When the animation completes, the pack's visible X/Z
// snap to the tile-center destination and Anim is cleared. Called
// once per explore frame from the explore loop so the visual
// transitions are smooth no matter how big dt happens to be (the
// caller's dt clamp ensures a single hitch can't overshoot the
// duration).
//
// Mirrors the player's animation tick in shape — same Animation
// struct, same eased lerp via Smoothstep — so a pack's tile-to-tile
// step reads as the same beat the player's step does.
func TickPackAnimations(g *GameState, dt float32) {
	if g == nil {
		return
	}
	for i := range g.Packs {
		anim := &g.Packs[i].Anim
		if anim.Kind != AnimStep {
			continue
		}
		anim.Elapsed += dt
		t := anim.Elapsed / anim.Duration
		if t > 1 {
			t = 1
		}
		eased := Smoothstep(t)
		g.Packs[i].X = Lerp(anim.FromX, anim.ToX, eased)
		g.Packs[i].Z = Lerp(anim.FromZ, anim.ToZ, eased)
		if anim.Elapsed >= anim.Duration {
			g.Packs[i].X = anim.ToX
			g.Packs[i].Z = anim.ToZ
			*anim = Animation{}
		}
	}
}

// StartPackStep arms a pack's animation toward (toTileX, toTileZ).
// The tile coordinate update is the caller's job (packs jump their
// logical tile immediately so collision and AI planning see the
// new position); this just fills in the visual interpolation
// state. Duration matches the player's StepDuration so the field
// reads as a single tempo whether the player or a pack moved.
func StartPackStep(p *Pack, toTileX, toTileZ int) {
	p.Anim = Animation{
		Kind:     AnimStep,
		Duration: StepDuration,
		FromX:    p.X,
		FromZ:    p.Z,
		ToX:      TileCenter(toTileX),
		ToZ:      TileCenter(toTileZ),
	}
}

// SnapPackToTile zeroes any in-flight animation and locks the visible
// position to the tile center. Used on battle engagement so the
// pack doesn't visually drift mid-splash, and as a safety net for
// any code path that wants "stop wherever you are and be at your
// tile now."
func SnapPackToTile(p *Pack) {
	p.X = TileCenter(p.TileX)
	p.Z = TileCenter(p.TileZ)
	p.Anim = Animation{}
}

// RevealRadius marks every in-bounds tile in a square of side
// (2*radius+1) centered on (cx, cz) as visited. radius=0 marks just
// the center tile (legacy single-tile reveal); radius=1 paints the
// 3×3 fog-of-war window the field uses today. Defensive: tolerates
// nil or short Visited grids so the editor's preview path can call
// it without crashing.
func RevealRadius(g *GameState, cx, cz, radius int) {
	if g == nil || g.Visited == nil {
		return
	}
	if radius < 0 {
		radius = 0
	}
	for dz := -radius; dz <= radius; dz++ {
		z := cz + dz
		if z < 0 || z >= len(g.Visited) {
			continue
		}
		row := g.Visited[z]
		for dx := -radius; dx <= radius; dx++ {
			x := cx + dx
			if !g.Area.InBounds(x, z) || x < 0 || x >= len(row) {
				continue
			}
			row[x] = true
		}
	}
}

// PackIndexAtTile returns the index of the alive pack standing on
// (x, z), or -1. Used by the player-step path to detect "stepped
// into a pack" → battle init. Skips dead packs so a defeated pack
// resting at a tile doesn't block movement onto it.
func PackIndexAtTile(packs []Pack, x, z int) int {
	return slices.IndexFunc(packs, func(p Pack) bool {
		return PackAlive(p) && p.TileX == x && p.TileZ == z
	})
}


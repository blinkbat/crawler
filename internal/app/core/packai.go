package core

import "slices"

// Pack AI dispatch. Each pack carries a PackAI mode (default PackAINone);
// PlanPackSteps hands each alive pack to its registered planner. A new style is
// one packAIPlanners row + one planner.
//
// Modes:
//   - PackAINone: stationary, never plans a step.
//   - PackAIJunkyardDog: wanders a small leash, occasionally steps, chases when
//     the player is close — but never beyond the leash.
//   - PackAIPatrol: paces the X axis out to PatrolRadius (bouncing at ends/walls),
//     ignoring the player but engaging if it paces onto their tile. Tracks
//     Pack.PatrolDir.
//   - PackAISkittish: flees within SkittishFleeRadius, else wanders; never engages.
//
// All randomness comes from GameState.RNG so the seed travels with run state.

// packPlanner plans one pack's per-step move. May roll g.RNG and read the
// player's tile; MUST NOT mutate g or occupied (the dispatcher reserves the
// destination afterward).
type packPlanner func(g *GameState, idx int, occupied map[[2]int]bool) (packAIStep, bool)

// packAIPlanners is the per-mode dispatch table; init asserts every mode has a row.
var packAIPlanners = [PackAICount]packPlanner{
	PackAINone:        planStationaryPack,
	PackAIJunkyardDog: planJunkyardDogPack,
	PackAIPatrol:      planPatrolPack,
	PackAISkittish:    planSkittishPack,
}

func init() {
	for i, p := range packAIPlanners {
		if p == nil {
			panic("core: packAIPlanners missing a planner for PackAI " + PackAIName(PackAI(i)))
		}
	}
}

// planStationaryPack is the PackAINone planner — never steps.
func planStationaryPack(*GameState, int, map[[2]int]bool) (packAIStep, bool) {
	return packAIStep{}, false
}

// PackStepIntoPlayer reports whether a pack at (tx,tz) and player at (px,pz)
// share a tile. Lives here (not explore) so headless tests need no raylib.
func PackStepIntoPlayer(tx, tz, px, pz int) bool {
	return tx == px && tz == pz
}

// PackEngagesPlayerAt reports whether pack p stepping onto (nx,nz) collides with
// the player and should start a battle: same tile, and on a voxel map the same
// standing surface (so a pack on a bridge deck doesn't engage below it). The
// heightfield level check is skipped (Level isn't tracked per-surface there).
func PackEngagesPlayerAt(g *GameState, p Pack, nx, nz int) bool {
	if !PackStepIntoPlayer(nx, nz, g.Player.TileX, g.Player.TileZ) {
		return false
	}
	if len(g.Area.Solids) == 0 {
		return true
	}
	land := p.Level
	if dir, ok := FacingFromDelta(nx-p.TileX, nz-p.TileZ); ok {
		if l, stepOK := g.Area.ResolveStep(p.TileX, p.Level, p.TileZ, dir); stepOK {
			land = l
		}
	}
	return land == g.Player.Level
}

// cardinalStepsBase is the fixed [dx,dz] table for the four facings, derived
// once from FacingVector (so AI and player-step code agree on directions).
// cardinalSteps returns a value-array copy each call, so wanderStep can shuffle
// it in place without recomputing.
var cardinalStepsBase = func() [FacingCount][2]int {
	var out [FacingCount][2]int
	for i := 0; i < FacingCount; i++ {
		dx, dz := FacingVector(i)
		out[i] = [2]int{dx, dz}
	}
	return out
}()

func cardinalSteps() [FacingCount][2]int {
	return cardinalStepsBase
}

// packAIStep is one pack's per-step move plan. EngagePlayer set means the pack
// stepped onto the player's tile: the caller starts a battle with this pack
// instead of applying the move.
type packAIStep struct {
	PackIdx      int
	NextX        int
	NextZ        int
	EngagePlayer bool
	Moved        bool
	// PatrolDir is the pace direction a patrol planner chose (possibly flipped);
	// the applier writes it back to Pack.PatrolDir for patrol packs only.
	PatrolDir int
}

// PlanPackSteps returns the moves alive packs chose this player-step. Pure
// planner — no mutation/battle/animation; the caller applies them and triggers
// battle when a pack walks onto the player. Per-mode rules live in each planner.
// Destinations are checked against static blockers, chests, doors, and other
// packs' current tiles.
func PlanPackSteps(g *GameState) []packAIStep {
	if g == nil {
		return nil
	}
	// Reuse package-level scratch across steps (single-threaded loop, result
	// consumed before the next call — mirrors render's torchCandidateBuf reuse).
	// Fast path: skip the occupancy map + loop when no alive pack is mobile (the
	// common all-PackAINone map).
	if !anyMobilePack(g.Packs) {
		packPlanBuf = packPlanBuf[:0]
		return packPlanBuf
	}
	plans := packPlanBuf[:0]
	occupied := buildPackOccupancy(g.Packs, -1)
	engagePlanned := false
	for i := range g.Packs {
		if !PackAlive(g.Packs[i]) {
			continue
		}
		mode := g.Packs[i].AI
		if int(mode) < 0 || int(mode) >= len(packAIPlanners) {
			continue // corrupt mode (future map) — treat as stationary
		}
		plan, ok := packAIPlanners[mode](g, i, occupied)
		if !ok {
			continue
		}
		// Only ONE engagement resolves per tick (ApplyPackSteps holds the
		// second). Skip the held plan so we DON'T vacate its tile in occupied —
		// else a later pack would plan onto it and overlap.
		if plan.EngagePlayer && engagePlanned {
			continue
		}
		// Reserve the destination and free the vacated tile.
		delete(occupied, [2]int{g.Packs[i].TileX, g.Packs[i].TileZ})
		occupied[[2]int{plan.NextX, plan.NextZ}] = true
		if plan.EngagePlayer {
			engagePlanned = true
		}
		plans = append(plans, plan)
	}
	packPlanBuf = plans
	return plans
}

// anyMobilePack reports whether at least one alive pack is non-stationary.
func anyMobilePack(packs []Pack) bool {
	for i := range packs {
		if PackAlive(packs[i]) && packs[i].AI != PackAINone {
			return true
		}
	}
	return false
}

// packPlanBuf / packOccupancyBuf are reused across player steps to avoid
// per-step allocation (single-threaded, consumed before the next call).
var (
	packPlanBuf      []packAIStep
	packOccupancyBuf map[[2]int]bool
)

// ApplyPackSteps applies PlanPackSteps' moves (tile + animation advance, patrol
// dir persisted) and returns the index of the ONE pack that engaged the player
// (-1 if none). Pure core (no raylib) so the engagement contract is testable.
// A second engager is HELD in place (its move not applied) so it doesn't end up
// overlapping the player after the first battle. Animation is armed before the
// tile update so StartPackStep captures the current X/Z as "from".
func ApplyPackSteps(g *GameState, plans []packAIStep) int {
	if g == nil {
		return -1
	}
	engaged := -1
	for _, plan := range plans {
		if !plan.Moved {
			continue
		}
		if plan.PackIdx < 0 || plan.PackIdx >= len(g.Packs) {
			continue
		}
		if plan.EngagePlayer && engaged >= 0 {
			continue
		}
		p := &g.Packs[plan.PackIdx]
		StartPackStep(p, plan.NextX, plan.NextZ)
		// On a voxel map, resolve the landing surface so the pack tracks
		// under/over a bridge.
		if g.Area.IsVoxel() {
			if dir, ok := FacingFromDelta(plan.NextX-p.TileX, plan.NextZ-p.TileZ); ok {
				if toL, stepOK := g.Area.ResolveStep(p.TileX, p.Level, p.TileZ, dir); stepOK {
					p.Level = toL
				}
			}
		}
		p.TileX = plan.NextX
		p.TileZ = plan.NextZ
		// Only patrol packs read PatrolDir; other modes leave it zero.
		if p.AI == PackAIPatrol {
			p.PatrolDir = plan.PatrolDir
		}
		if plan.EngagePlayer {
			engaged = plan.PackIdx
			SnapPackToTile(p)
		}
	}
	return engaged
}

// planJunkyardDogPack plans one junkyard-dog pack's step (chase within
// PackChaseRadius, else wander the leash).
func planJunkyardDogPack(g *GameState, idx int, occupied map[[2]int]bool) (packAIStep, bool) {
	if g.Rand().Float32() >= PackStepChance {
		return packAIStep{}, false
	}
	p := g.Packs[idx]
	px, pz := g.Player.TileX, g.Player.TileZ

	if ChebyshevDistance(p.TileX, p.TileZ, px, pz) <= PackChaseRadius {
		nx, nz, ok := chaseStep(g, p, occupied, px, pz)
		if !ok {
			return packAIStep{}, false
		}
		engage := PackEngagesPlayerAt(g, p, nx, nz)
		return packAIStep{PackIdx: idx, NextX: nx, NextZ: nz, EngagePlayer: engage, Moved: true}, true
	}

	nx, nz, ok := wanderStep(g, p, occupied, px, pz)
	if !ok {
		return packAIStep{}, false
	}
	return packAIStep{PackIdx: idx, NextX: nx, NextZ: nz, Moved: true}, true
}

// withinLeash reports whether (tx,tz) is within PackLeashRadius of the pack's
// home. Shared leash gate for chaseStep / fleeStep / wanderStep.
func withinLeash(p Pack, tx, tz int) bool {
	return ChebyshevDistance(tx, tz, p.HomeX, p.HomeZ) <= PackLeashRadius
}

// axisPrioritySteps orders the X-axis step xStep and the Z-axis step zStep into a
// fixed-priority pair: X-first by default, Z-first when zFirst. The shared
// tie-break order behind chaseStep (prefer the longer axis) and fleeStep (prefer
// the nearer axis) — each passes its own steps + preference.
func axisPrioritySteps(xStep, zStep [2]int, zFirst bool) [2][2]int {
	if zFirst {
		return [2][2]int{zStep, xStep}
	}
	return [2][2]int{xStep, zStep}
}

func chaseStep(g *GameState, p Pack, occupied map[[2]int]bool, px, pz int) (int, int, bool) {
	dx := px - p.TileX
	dz := pz - p.TileZ
	// Prefer the longer axis; ties go X-first (deterministic, no dither). A
	// zero-delta axis contributes no step (toward an already-aligned axis).
	ordered := axisPrioritySteps([2]int{Sign(dx), 0}, [2]int{0, Sign(dz)}, AbsInt(dz) > AbsInt(dx))
	steps := [FacingCount][2]int{}
	n := 0
	for _, s := range ordered {
		if s[0] == 0 && s[1] == 0 {
			continue
		}
		steps[n] = s
		n++
	}
	for i := 0; i < n; i++ {
		tx, tz := p.TileX+steps[i][0], p.TileZ+steps[i][1]
		// Stay on the leash even while chasing — a dog at its edge isn't dragged
		// further from home.
		if !withinLeash(p, tx, tz) {
			continue
		}
		if !packCanMoveTo(g, p, occupied, tx, tz, true /* allow player tile */, px, pz) {
			continue
		}
		return tx, tz, true
	}
	return 0, 0, false
}

// planPatrolPack plans one patrolling pack's step: paces the X axis out to
// PatrolRadius (bouncing at ends/blockers), ignoring the player but engaging if
// it paces onto them. The chosen pace dir rides back on PatrolDir.
func planPatrolPack(g *GameState, idx int, occupied map[[2]int]bool) (packAIStep, bool) {
	if g.Rand().Float32() >= PatrolStepChance {
		return packAIStep{}, false
	}
	p := g.Packs[idx]
	px, pz := g.Player.TileX, g.Player.TileZ
	dir := p.PatrolDir
	if dir == 0 {
		dir = 1
	}
	// Try the current pace dir; if past the span or blocked, flip. Boxed in
	// both ways = hold this step.
	for _, d := range [2]int{dir, -dir} {
		nx := p.TileX + d
		nz := p.TileZ
		if AbsInt(nx-p.HomeX) > PatrolRadius {
			continue
		}
		if !packCanMoveTo(g, p, occupied, nx, nz, true /* allow player tile = engage */, px, pz) {
			continue
		}
		return packAIStep{
			PackIdx:      idx,
			NextX:        nx,
			NextZ:        nz,
			EngagePlayer: PackEngagesPlayerAt(g, p, nx, nz),
			Moved:        true,
			PatrolDir:    d,
		}, true
	}
	return packAIStep{}, false
}

// planSkittishPack plans one skittish pack's step: flee within
// SkittishFleeRadius, else wander. Never engages (fleeStep refuses the player
// tile), so it must be cornered to catch.
func planSkittishPack(g *GameState, idx int, occupied map[[2]int]bool) (packAIStep, bool) {
	if g.Rand().Float32() >= PackStepChance {
		return packAIStep{}, false
	}
	p := g.Packs[idx]
	px, pz := g.Player.TileX, g.Player.TileZ
	if ChebyshevDistance(p.TileX, p.TileZ, px, pz) <= SkittishFleeRadius {
		if nx, nz, ok := fleeStep(g, p, occupied, px, pz); ok {
			return packAIStep{PackIdx: idx, NextX: nx, NextZ: nz, Moved: true}, true
		}
		return packAIStep{}, false
	}
	nx, nz, ok := wanderStep(g, p, occupied, px, pz)
	if !ok {
		return packAIStep{}, false
	}
	return packAIStep{PackIdx: idx, NextX: nx, NextZ: nz, Moved: true}, true
}

// fleeStep picks the cardinal step that increases distance from the player,
// staying on the leash and refusing the player tile. Prefers the nearer axis
// (smaller |delta|) to break line-of-approach. False when cornered.
func fleeStep(g *GameState, p Pack, occupied map[[2]int]bool, px, pz int) (int, int, bool) {
	dx := p.TileX - px // sign points AWAY from the player along X
	dz := p.TileZ - pz
	// Away direction per axis; on a tie (delta 0) default positive so the pack
	// still has a way to peel off.
	awayX := Sign(dx)
	if awayX == 0 {
		awayX = 1
	}
	awayZ := Sign(dz)
	if awayZ == 0 {
		awayZ = 1
	}
	// Prefer the nearer axis (smaller |delta|) to break line-of-approach; ties
	// keep X-first. Both axes always step (zero-delta defaulted to +1 above).
	steps := axisPrioritySteps([2]int{awayX, 0}, [2]int{0, awayZ}, AbsInt(dz) < AbsInt(dx))
	for _, s := range steps {
		nx, nz := p.TileX+s[0], p.TileZ+s[1]
		if !withinLeash(p, nx, nz) {
			continue
		}
		if !packCanMoveTo(g, p, occupied, nx, nz, false /* never onto the player */, px, pz) {
			continue
		}
		return nx, nz, true
	}
	return 0, 0, false
}

func wanderStep(g *GameState, p Pack, occupied map[[2]int]bool, px, pz int) (int, int, bool) {
	cardinals := cardinalSteps()
	// Fisher-Yates shuffle so wander direction is independent of array order.
	for i := len(cardinals) - 1; i > 0; i-- {
		j := g.Rand().Intn(i + 1)
		cardinals[i], cardinals[j] = cardinals[j], cardinals[i]
	}
	for _, c := range cardinals {
		tx, tz := p.TileX+c[0], p.TileZ+c[1]
		if !withinLeash(p, tx, tz) {
			continue
		}
		if !packCanMoveTo(g, p, occupied, tx, tz, false /* refuse player tile */, px, pz) {
			continue
		}
		return tx, tz, true
	}
	return 0, 0, false
}

// packCanMoveTo is the pack-flavored CanEnterTile: packs never enter doors,
// avoid other packs, and step onto the player tile only when allowPlayer is set
// (chase=true to encode engagement, wander=false). px/pz pass the player tile
// explicitly rather than as a hidden global.
func packCanMoveTo(g *GameState, p Pack, occupied map[[2]int]bool, tx, tz int, allowPlayer bool, px, pz int) bool {
	// Honor the player's cliff/ramp rule (StepElevationOK) so packs don't climb
	// sheer cliffs. On a voxel map, resolve the landing surface so the entry
	// check is level-aware; landLevel stays p.Level on a flat map.
	landLevel := p.Level
	voxel := g.Area.IsVoxel()
	if dir, ok := FacingFromDelta(tx-p.TileX, tz-p.TileZ); ok {
		if voxel {
			l, stepOK := g.Area.ResolveStep(p.TileX, p.Level, p.TileZ, dir)
			if !stepOK {
				return false
			}
			landLevel = l
		} else if !g.Area.StepElevationOK(p.TileX, p.TileZ, dir) {
			return false
		}
	}
	opts := EnterOpts{
		AllowDoorTile:   false,
		AllowPlayerTile: allowPlayer,
		PlayerTileX:     px,
		PlayerTileZ:     pz,
		OccupiedPacks:   occupied,
	}
	if voxel {
		return CanEnterTileAtLevel(g, tx, tz, landLevel, opts)
	}
	return CanEnterTile(g, tx, tz, opts)
}

// buildPackOccupancy returns the tiles held by alive packs, optionally
// excluding one index (so a moving pack doesn't see its own tile as blocked).
func buildPackOccupancy(packs []Pack, exclude int) map[[2]int]bool {
	// Reuse packOccupancyBuf (cleared, not re-made) — see PlanPackSteps.
	occ := packOccupancyBuf
	if occ == nil {
		occ = make(map[[2]int]bool, len(packs))
		packOccupancyBuf = occ
	} else {
		clear(occ)
	}
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

// TickPackAnimations advances every alive pack's step animation by dt, snapping
// to the tile center and clearing Anim on completion. Mirrors the player's tick
// (same Smoothstep ease) so steps read at the same beat.
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

// StartPackStep arms a pack's visual animation toward (toTileX, toTileZ); the
// tile-coordinate update is the caller's job. Duration matches StepDuration.
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

// SnapPackToTile zeroes any in-flight animation and locks the visible position
// to the tile center (used on battle engagement so the pack doesn't drift).
func SnapPackToTile(p *Pack) {
	p.X = TileCenter(p.TileX)
	p.Z = TileCenter(p.TileZ)
	p.Anim = Animation{}
}

// RevealRadius marks the (2*radius+1)-square centered on (cx,cz) as visited
// (radius=0 = just the center, radius=1 = the 3×3 fog window). Tolerates nil/
// short Visited grids.
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

// PackIndexAtTile returns the alive pack on (x,z), or -1 (skips dead packs so
// they don't block movement). Used by the player-step path to detect battle init.
func PackIndexAtTile(packs []Pack, x, z int) int {
	return slices.IndexFunc(packs, func(p Pack) bool {
		return PackAlive(p) && p.TileX == x && p.TileZ == z
	})
}

// PackIndexAtTileLevel is the level-aware PackIndexAtTile: the alive pack on
// (x,z) at `level`, or -1. Used on voxel maps so a step under a deck engages
// only a pack on that ground surface, not one on the deck above.
func PackIndexAtTileLevel(packs []Pack, x, z, level int) int {
	return slices.IndexFunc(packs, func(p Pack) bool {
		return PackAlive(p) && p.TileX == x && p.TileZ == z && p.Level == level
	})
}

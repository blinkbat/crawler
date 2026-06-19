package core

import "slices"

// Pack AI dispatch. Each pack carries a PackAI mode authored on its
// PackSpawn (default PackAINone — stationary, no per-step planning).
// PlanPackSteps walks alive packs and, for each, hands control to the
// planner registered for that pack's mode. New movement styles land as
// one entry in packAIPlanners plus the per-mode planOneXxxPack body.
//
// Today's modes:
//   - PackAINone: stationary. Never plans a step. The editor's "no AI"
//     default and what every old map without an AI column resolves to.
//   - PackAIJunkyardDog: wanders inside a small leash around the spawn
//     tile, occasionally taking a step when the player does, and steps
//     toward the player when they're close. Doesn't chase outside the
//     leash and doesn't move every step — the goal is "this dog noticed
//     you walk by and decided to look up," not "you have a hostile
//     escort following you across the map."
//   - PackAIPatrol: paces a fixed line along the X axis around the spawn
//     tile (out to PatrolRadius, bouncing at the ends / walls), ignoring
//     the player but engaging if it paces onto their tile — a sentry on a
//     beat. Tracks its pace direction in Pack.PatrolDir.
//   - PackAISkittish: flees directly away when the player is within
//     SkittishFleeRadius, otherwise wanders the leash. Never engages —
//     it only runs, so the player has to corner it to catch it.
//
// All randomness comes from GameState.RNG so the seed travels with the
// run state (tests / future save-load) instead of leaking onto a
// package-level singleton.

// packPlanner plans one pack's per-step move. (state, packIndex,
// occupied) → (step, planned). Implementations may roll dice off
// g.RNG and read the player's tile; they must NOT mutate g or the
// occupied map (the dispatcher reserves the destination after the
// plan returns).
type packPlanner func(g *GameState, idx int, occupied map[[2]int]bool) (packAIStep, bool)

// packAIPlanners is the per-mode dispatch table. Indexed by PackAI so a
// new mode is one row + one planner. Init below asserts every mode in
// the enum has a row.
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

// planStationaryPack is the PackAINone planner — no step, ever. The
// pack sits at its spawn tile until the player walks into it.
func planStationaryPack(*GameState, int, map[[2]int]bool) (packAIStep, bool) {
	return packAIStep{}, false
}

// PackStepIntoPlayer reports whether a pack at (tx, tz) and the player
// at (px, pz) share a tile — the collision condition that should
// trigger a battle. Lives here (not in explore) so the headless tests
// covering pack-AI can assert it without dragging raylib in.
func PackStepIntoPlayer(tx, tz, px, pz int) bool {
	return tx == px && tz == pz
}

// PackEngagesPlayerAt reports whether pack p stepping onto (nx,nz) collides with
// the player and should start a battle: the same tile, and — on a voxel map —
// the same standing surface, so a pack crossing a bridge deck doesn't engage a
// player on the ground beneath it (or vice versa). On a heightfield the level
// check is skipped: a pack's Level isn't tracked per-surface there (it can drift
// across a ramp), and engagement was always purely tile-based, so this stays
// byte-identical. The voxel branch resolves the pack's landing surface the same
// way ApplyPackSteps will, then requires it to match the player's standing level.
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

// cardinalSteps returns the four cardinal-direction (dx, dz) pairs in
// FacingVector order (North, East, South, West). Builds from
// FacingVector so the AI and the player-step code can't disagree on
// what "south" means — a future facing-convention change is one switch
// edit, not a manual table reshuffle.
// cardinalStepsBase is the fixed [dx,dz] table for the four facings, derived
// once from FacingVector. cardinalSteps returns a copy each call (it's a value
// array), so wanderStep can shuffle it in place without recomputing the table
// per wandering pack per step.
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
	// PatrolDir carries the pace direction a PackAIPatrol planner chose
	// this step (it may have flipped at a wall / leash end). The step
	// applier (explore's tickPackAI) writes it back onto Pack.PatrolDir
	// for patrol packs only; the other modes leave it at its zero value
	// and the applier ignores it for them.
	PatrolDir int
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
	// Reuse package-level scratch across steps rather than allocating a fresh
	// plans slice + occupancy map on every landed player step. PlanPackSteps
	// runs only on the single-threaded game loop and its result is consumed by
	// ApplyPackSteps before the next call, so sharing the buffers is safe
	// (mirrors render's torchCandidateBuf reuse).
	// Fast path: if no alive pack has a mobile AI, there's nothing to plan — skip
	// building the occupancy map and the per-pack loop entirely. The common
	// all-PackAINone map (every map authored without an AI column, and any with
	// only stationary packs) hits this every landed step instead of churning the
	// occupancy map for moves that can never happen.
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
			// Corrupt mode (out-of-range value from a future map). Treat
			// as stationary — safer than crashing or planning random
			// moves the author didn't intend.
			continue
		}
		plan, ok := packAIPlanners[mode](g, i, occupied)
		if !ok {
			continue
		}
		// Only ONE engagement resolves per tick — ApplyPackSteps holds a
		// second engager in place (its move is not applied). Mirror that rule
		// here: skip the held plan entirely so we DON'T vacate its current
		// tile in `occupied`. Otherwise a later pack would see the held pack's
		// still-occupied tile as free and plan onto it, overlapping for a tick.
		if plan.EngagePlayer && engagePlanned {
			continue
		}
		// Reserve the destination so a later pack doesn't plan into
		// the same square this frame, and free the tile this pack vacates.
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

// anyMobilePack reports whether at least one alive pack has a non-stationary
// AI mode. A cheap O(packs) scan with no allocation — lets PlanPackSteps
// short-circuit on all-stationary maps before it builds the occupancy map.
func anyMobilePack(packs []Pack) bool {
	for i := range packs {
		if PackAlive(packs[i]) && packs[i].AI != PackAINone {
			return true
		}
	}
	return false
}

// packPlanBuf / packOccupancyBuf are reused across player steps so the
// per-step pack planning doesn't allocate a fresh slice + map each time.
// Single-threaded (game loop) and consumed before the next call — see
// PlanPackSteps.
var (
	packPlanBuf      []packAIStep
	packOccupancyBuf map[[2]int]bool
)

// ApplyPackSteps applies the moves PlanPackSteps produced: each chosen pack's
// tile AND visual animation advance, patrol packs persist their pace direction,
// and the function returns the index of the ONE pack that engaged the player
// this tick (or -1 if none). Pure core (no raylib) so the engagement contract
// is headlessly testable; explore's tickPackAI is a thin wrapper around it.
//
// Only a SINGLE engagement resolves per tick. A second pack whose plan also
// lands on the player's tile is held in place — its move is NOT applied —
// because the engaged pack starts the battle and the loser would otherwise be
// left overlapping the player with no battle of its own once that battle ends.
//
// The animation is armed BEFORE the tile update so StartPackStep captures the
// pack's current X/Z as the "from"; the tile then jumps so subsequent packs'
// planning this tick sees the new occupancy (PlanPackSteps already reserved it).
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
		// On a voxel map, resolve which surface the pack lands on so it tracks
		// under/over a bridge; on a heightfield ResolveStep returns the column
		// top, leaving Level == ElevationLevelAt as before.
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

// planJunkyardDogPack plans one junkyard-dog pack's step. Same chase /
// wander rules previously hard-coded as the only AI mode — now opt-in
// per pack via PackAIJunkyardDog.
func planJunkyardDogPack(g *GameState, idx int, occupied map[[2]int]bool) (packAIStep, bool) {
	if g.Rand().Float32() >= PackStepChance {
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
		engage := PackEngagesPlayerAt(g, p, nx, nz)
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
	steps := [FacingCount][2]int{}
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
		// Stay on the leash even while chasing. A dog already at its leash
		// edge must not be dragged further from home by a passing player —
		// that's the "hostile escort across the map" behavior the mode
		// docstring promises to avoid. Mirrors the leash gate wanderStep /
		// fleeStep apply (PackChaseRadius < PackLeashRadius, so a player can
		// still be chased while inside the leash, just not beyond it).
		if ChebyshevDistance(tx, tz, p.HomeX, p.HomeZ) > PackLeashRadius {
			continue
		}
		if !packCanMoveTo(g, p, occupied, tx, tz, true /* allow player tile */, px, pz) {
			continue
		}
		return tx, tz, true
	}
	return 0, 0, false
}

// planPatrolPack plans one patrolling pack's step. The pack paces along
// the X axis around its home tile out to PatrolRadius, bouncing at the
// ends and at any blocker. It ignores the player's position entirely (no
// chase) but DOES engage if its pace walks it onto the player's tile — a
// sentry you can blunder into. The chosen pace direction (possibly flipped
// this step) rides back on the step's PatrolDir for the applier to persist.
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
	// Try the current pace direction; if the next tile is past the patrol
	// span or blocked, flip and try the other way. A fully-boxed-in patrol
	// (both sides blocked) just holds its tile this step.
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

// planSkittishPack plans one skittish pack's step: flee directly away when
// the player is within SkittishFleeRadius, otherwise wander the leash like
// the junkyard dog's idle branch. A fleeing pack never steps onto the
// player (allowPlayer=false in fleeStep), so it can't engage — it only ever
// runs, and the player has to corner it against a wall / leash edge to
// catch it.
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
// staying inside the leash and refusing the player's own tile (so a fleeing
// pack can't accidentally engage). Prefers the axis with the smaller current
// separation so the pack breaks line-of-approach on the side the player is
// closing from. Returns false when no away-step is open (cornered).
func fleeStep(g *GameState, p Pack, occupied map[[2]int]bool, px, pz int) (int, int, bool) {
	dx := p.TileX - px // sign points AWAY from the player along X
	dz := p.TileZ - pz
	// Away direction per axis; when level on an axis (delta 0), default to
	// the positive step so the pack still has a way to peel off.
	awayX := Sign(dx)
	if awayX == 0 {
		awayX = 1
	}
	awayZ := Sign(dz)
	if awayZ == 0 {
		awayZ = 1
	}
	// Try the axis where the player is nearest first (smaller |delta|) so
	// the pack opens the tighter gap.
	steps := [2][2]int{{awayX, 0}, {0, awayZ}}
	if AbsInt(dz) < AbsInt(dx) {
		steps = [2][2]int{{0, awayZ}, {awayX, 0}}
	}
	for _, s := range steps {
		nx, nz := p.TileX+s[0], p.TileZ+s[1]
		if ChebyshevDistance(nx, nz, p.HomeX, p.HomeZ) > PackLeashRadius {
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
	// Shuffle in place via Fisher-Yates against the shared RNG so the
	// wander direction is independent of FacingVector's array order.
	for i := len(cardinals) - 1; i > 0; i-- {
		j := g.Rand().Intn(i + 1)
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
	// Honor the same cliff/ramp rule the player obeys (StepElevationOK): a pack
	// may only step between tiles whose shared edge sits at one elevation, or
	// across a ramp. Without this, packs climb sheer cliffs the player can't —
	// and now that raised tiles draw no support column, a chasing pack visibly
	// floats up onto a plateau. Pack steps are always one cardinal tile.
	// On a voxel map, resolve the surface the pack would land on so the entry
	// check can be level-aware (a tree blocks only its own levels). landLevel
	// stays p.Level on a flat map, where CanEnterTile is used as before.
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

// buildPackOccupancy returns the set of tiles currently held by alive
// packs, optionally excluding a specific pack index (for the
// "where am I allowed to move to" check that shouldn't see the
// moving pack's own tile as blocked).
func buildPackOccupancy(packs []Pack, exclude int) map[[2]int]bool {
	// Reuse packOccupancyBuf (cleared, not re-made) across steps — see the
	// buffer-reuse note on PlanPackSteps, the sole caller.
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

// PackIndexAtTileLevel is the level-aware PackIndexAtTile: the alive pack on
// (x,z) standing at `level`, or -1. The player-step engage path uses it on a
// voxel map so a step that lands UNDER a deck engages only a pack sharing that
// ground surface, not one standing on the deck above (whose Level differs). On a
// heightfield, PackIndexAtTile is used instead (Level isn't tracked per-surface).
func PackIndexAtTileLevel(packs []Pack, x, z, level int) int {
	return slices.IndexFunc(packs, func(p Pack) bool {
		return PackAlive(p) && p.TileX == x && p.TileZ == z && p.Level == level
	})
}

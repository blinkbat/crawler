package core

import (
	"fmt"
	"math"
	"slices"
)

// TileCoord formats a (x, z) tile coordinate as a human-readable string
// for UI labels and editor diagnostics ("(3, 7)"). Centralized so the
// spacing convention stays consistent across modal headers, debug
// overlays, and status messages. Error-message formatters keep their
// own compact form inline — they're a different audience.
func TileCoord(x, z int) string {
	return fmt.Sprintf("(%d, %d)", x, z)
}

// Tile characters. Grouped by the layer that owns them. The .map on-disk
// format uses these literal bytes; the editor's brush palettes paint them.

// Walls layer. TileOpen marks an open cell; TileRock is a wall blocker.
// Both exported so callers don't have to open-code '.' / '#' against the
// walls grid.
const (
	TileOpen = '.' // open cell (walkable)
	TileRock = '#' // wall blocker
)

// Ceiling layer. Parallel grid to walls; chars share the wall convention
// since both layers describe the same "solid block?" yes/no question (a
// wall is a solid floor-to-ceiling column; a ceiling is a solid slab
// covering the cell from above). A cell with no ceiling (TileCeilingOpen)
// shows the skybox above; one with TileCeilingSolid renders an opaque
// quad at wall height, turning that tile into roofed interior space —
// the visual cue for "you are inside a dungeon room."
const (
	TileCeilingOpen  = '.' // no ceiling — sky shows through
	TileCeilingSolid = '#' // solid ceiling slab at wall height
)

// Floor layer. Walkable surfaces — material-keyed variants (grass / dirt /
// dark grass / stone) read against the material's own floor pixels;
// universal variants (path / planks / water / sand / snow) load their own
// textures and apply in any material. FloorDeepWater is the sole blocking
// floor tile: it renders flat (you can see across it) but BlockedAt
// reports it as impassable, modeling water too deep to wade through.
const (
	FloorAuto      = '.' // pick a variant by per-tile hash (back-compat default)
	FloorGrass     = 'g'
	FloorDirt      = 'd'
	FloorDarkGrass = 'k'
	FloorStone     = 's'
	// Universal floor variants — render the same in any material set so an
	// author can lay a stone path through a forest or a wooden plank floor
	// across a cave. None of these block movement EXCEPT FloorDeepWater.
	FloorCobble    = 'c' // mortared cobblestone path
	FloorPlank     = 'w' // wooden planks
	FloorWater     = '~' // shallow water — walkable, just a different look
	FloorDeepWater = 'W' // deep water — blocks movement, vision passes over
	FloorSand      = 'n' // pale sand
	FloorSnow      = 'i' // packed snow
)

// Decor layer. '.' means "let the renderer's auto-scatter decide"; '_'
// suppresses any auto decor; explicit chars force a specific small prop.
// Decor is purely cosmetic — it never blocks movement.
const (
	DecorAuto     = '.'
	DecorEmpty    = '_'
	DecorBush     = 'b'
	DecorMushroom = 'm'
	DecorPebble   = 'p'
	// Soft-ground details: visual breakup for fields and plazas.
	DecorTallGrass = ',' // upright blades of grass
	DecorFlowers   = 'f' // mixed wildflowers
	DecorClover    = 'v' // low clover patch
	DecorReeds     = 'r' // tall reed cluster
	// Atmospheric markers: tell a story about what happened on this tile.
	DecorBones  = 'o' // skull + scattered bones
	DecorScorch = 'x' // black scorch ring
	DecorBlood  = '!' // dried bloodstain
	DecorCobweb = '*' // corner cobweb
	// Forest leftovers: dead wood that breaks up grass without blocking.
	DecorStump    = 't' // weathered tree stump
	DecorLog      = 'l' // mossy fallen log
	DecorLeafPile = 'L' // pile of fallen leaves
	// Multi-tile structures (walkable — decor doesn't block). DecorArchway
	// is the anchor at the left/west tile of a 2-tile (1×2 along +X)
	// archway; DecorArchwayTail must be placed at the matching east tile
	// or the renderer falls back to drawing the anchor solo. The arch
	// passes overhead — both tiles stay walkable.
	DecorArchway     = 'A' // archway anchor (left tile)
	DecorArchwayTail = 'a' // archway tail   (right tile)
	// DecorLilypad is the swamp dressing for water tiles — a flat floating
	// pad with a small bloom. Pure decor (does not block); intended to be
	// painted on FloorWater / FloorDeepWater for the swamp aesthetic but
	// works on any floor.
	DecorLilypad = 'y'
	// Atmospheric / interior dressings — none block. Authors can layer
	// these on any walkable tile to add story / texture.
	DecorRug         = 'u' // woven floor rug, warm tones
	DecorCandle      = 'c' // stubby candle on a small puddle of wax
	DecorBootprints  = 'i' // pair of stamped boot prints
	DecorAshHeap     = 'h' // small heap of grey ash (smaller / cooler than scorch)
	DecorPuddle      = 'q' // shallow water puddle
	DecorRootCluster = 'k' // gnarled root stubs poking from the floor
)

// Props layer. TilePropEmpty marks an open cell; every other char listed
// here is a blocker. Mirrors FloorAuto / DecorAuto on the other layers so
// callers don't open-code '.' for "no prop here."
const (
	TilePropEmpty = '.' // open cell, no prop
	TileTree      = 'T' // regular tree, blocks
	TileTreeXL    = 'X' // extra-large tree, blocks
	// Tree shape variants — all 1-tile, all blocking. Designed to
	// break up the visual monotony of long forest stretches without
	// adding new content rules (same blocking, same minimap color
	// family). The renderer reuses assets.tree at different scales
	// and offsets via drawPropTreeTall / drawPropTreeTwin /
	// drawPropTreeYoung in render/world.go.
	TileTreeTall  = '|' // tall narrow pine, slimmer + taller than Tree
	TileTreeTwin  = '@' // two trees crammed into one tile, offset
	TileTreeYoung = '/' // young / smaller tree, scrubby thicket
	TileRockLarge = 'O' // boulder, blocks
	TileBushLarge = 'B' // dense bush, blocks
	// Inhabited / ruined props: read as "someone lived here."
	TileCrate        = 'C' // wooden crate
	TileBarrel       = 'R' // banded barrel
	TileUrn          = 'U' // belly-shouldered urn
	TileStalagmite   = 'S' // cave stalagmite spire
	TilePillar       = 'P' // intact stone pillar with capital
	TileBrokenPillar = 'I' // toppled / chest-high pillar stub
	TileStatue       = 'M' // weathered humanoid statue
	TileObelisk      = 'Q' // tall four-sided pyramid-capped obelisk
	TileFountain     = 'F' // low fountain with a central plume
	// Larger rock formations. TileRockCairn is a 1-tile prop — a taller
	// stacked-stone column distinct from the squat TileRockLarge boulder.
	// TileRockFormation occupies a 2×2 footprint: place the anchor at the
	// top-left tile of the square and TileRockFormationTail at the other
	// three tiles. The renderer skips drawing tails; the anchor's mesh
	// covers the whole footprint. All four tiles block movement.
	TileRockCairn         = 'K' // tall stacked-stone cairn (1 tile, blocks)
	TileRockFormation     = 'J' // 2×2 rock formation anchor (top-left)
	TileRockFormationTail = 'j' // 2×2 rock formation footprint shadow
	// Outdoor / field tileset — stoneworked + agricultural props. All
	// single-tile blockers; the editor's brush palette and the
	// renderer's propModels map pick them up via the canonical list +
	// init-time coverage asserts.
	TileWell       = 'W' // stone-ringed well (cross-layer with FloorDeepWater; layers dispatch independently)
	TileGravestone = 'G' // weathered tombstone
	TileSignPost   = 'N' // wooden sign on a post
	TileHayBale    = 'H' // round bound straw bale
	TileScarecrow  = 'Y' // cross + sackcloth scarecrow
	// Indoor / dungeon tileset — furniture and crypt props. All
	// single-tile blockers, same registry pattern.
	TileBookshelf   = 'V' // tall wooden shelf with books
	TileTable       = 'E' // wooden table with legs
	TileBed         = 'D' // wood-frame bed with bedding
	TileBrazier     = 'Z' // metal brazier on a tripod with flame (floor, BLOCKS, bright light)
	TileSarcophagus = 'A' // stone sarcophagus with lid (cross-layer with DecorArchway)
	// TileTorch is a wall-mounted torch — NON-blocking (sits on the
	// wall, leaves the floor clear) so it can line dungeon corridors
	// freely. The renderer auto-orients it to face away from the
	// adjacent wall and gives it an animated particle flame + a
	// (dimmer-than-brazier) point light. Place it on a floor tile
	// next to a wall.
	TileTorch = 'z' // wall torch (non-blocking, animated flame, dim light)
)

// Doors are modeled as entities (like chests), not as a tile char on
// any layer. The renderer reads g.Doors and draws a doorframe billboard
// at each door's tile; movement checks "is there a door under my new
// position?" on step landing. The tile underneath stays a regular
// walkable floor — no grid-layer character is needed.

// SpawnSnapReason tags why a SnappedSpawnPositions entry exists in the
// shape it does. Replaces the older "{-1, -1}" sentinel which conflated
// two distinct outcomes ("the author left this pack empty" and "the
// snap search couldn't find an open tile near the authored position").
// Callers that want to surface a useful diagnostic (the editor's
// reachability warning) can now distinguish them.
type SpawnSnapReason int

const (
	// SpawnSnapPlaced means the pack will be rendered at TileX/TileZ.
	SpawnSnapPlaced SpawnSnapReason = iota
	// SpawnSnapEmptyMembers means the authored spawn has no enemies; the
	// runtime drops it, no field representative gets drawn.
	SpawnSnapEmptyMembers
	// SpawnSnapNoOpenTile means the nearest-open-tile search came up
	// empty (every cell on the map is blocked or already taken).
	SpawnSnapNoOpenTile
)

// SpawnSnap is the result of one PackSpawn's runtime placement pass.
// Reason == SpawnSnapPlaced is the success case; everything else means
// TileX/TileZ should be treated as undefined.
type SpawnSnap struct {
	TileX  int
	TileZ  int
	Reason SpawnSnapReason
}

// Placed reports whether this snap successfully positioned the pack —
// callers that only care about the success case should branch on this.
func (s SpawnSnap) Placed() bool { return s.Reason == SpawnSnapPlaced }

// placePacks converts the area's pack-spawn placeholders into runtime
// Packs, snapping each pack's tile to the nearest open square so the
// author doesn't have to perfect placement against geometry. The player's
// start tile is seeded as occupied so the snapping never lands on it.
// Empty pack rosters are skipped — a pack with zero members has no field
// representative to render or engage.
func placePacks(a AreaDefinition) []Pack {
	packs := make([]Pack, 0, len(a.PackSpawns))
	for i, snap := range SnappedSpawnPositions(a) {
		if !snap.Placed() {
			continue
		}
		spawn := a.PackSpawns[i]
		members := make([]Enemy, 0, len(spawn.Members))
		for _, member := range spawn.Members {
			if name := member.CustomName; name != "" {
				if def, ok := CustomEnemyByName(a.CustomEnemies, name); ok {
					members = append(members, def.Instantiate())
					continue
				}
			}
			members = append(members, NewEnemy(member.Kind))
		}
		packs = append(packs, Pack{
			TileX:   snap.TileX,
			TileZ:   snap.TileZ,
			HomeX:   snap.TileX,
			HomeZ:   snap.TileZ,
			X:       TileCenter(snap.TileX),
			Z:       TileCenter(snap.TileZ),
			Members: members,
		})
	}
	return packs
}

// SnappedSpawnPositions returns, for each PackSpawn on `a`, the runtime
// tile coords the pack will actually occupy after placePacks' snap pass.
// Index in the output matches index in a.PackSpawns. A spawn that gets
// dropped reports its Reason so callers (e.g. the editor's reachability
// warning) can describe WHY a pack was dropped instead of treating
// every drop as the same case.
//
// Exposed so the editor's reachability check can use the *snapped*
// positions — otherwise an author who places a pack on a wall would see a
// false "unreachable" warning even though the game will silently relocate
// the pack at runtime, and vice versa.
func SnappedSpawnPositions(a AreaDefinition) []SpawnSnap {
	out := make([]SpawnSnap, 0, len(a.PackSpawns))
	occupied := map[[2]int]bool{{a.StartTileX, a.StartTileZ}: true}
	for _, spawn := range a.PackSpawns {
		if len(spawn.Members) == 0 {
			out = append(out, SpawnSnap{Reason: SpawnSnapEmptyMembers})
			continue
		}
		x, z := nearestOpenTile(a, spawn.TileX, spawn.TileZ, occupied)
		if x < 0 {
			out = append(out, SpawnSnap{Reason: SpawnSnapNoOpenTile})
			continue
		}
		occupied[[2]int{x, z}] = true
		out = append(out, SpawnSnap{TileX: x, TileZ: z, Reason: SpawnSnapPlaced})
	}
	return out
}

func nearestOpenTile(a AreaDefinition, wantX, wantZ int, occupied map[[2]int]bool) (int, int) {
	if a.FloorAt(wantX, wantZ) && !occupied[[2]int{wantX, wantZ}] {
		return wantX, wantZ
	}
	bestX, bestZ := -1, -1
	bestDist := math.MaxInt
	for z := 0; z < a.Height; z++ {
		for x := 0; x < a.Width; x++ {
			if !a.FloorAt(x, z) || occupied[[2]int{x, z}] {
				continue
			}
			dist := AbsInt(x-wantX) + AbsInt(z-wantZ)
			if dist < bestDist {
				bestDist = dist
				bestX, bestZ = x, z
			}
		}
	}
	return bestX, bestZ
}

// WallAt returns true if the cell holds a wall in the walls layer. Out-of-
// bounds reads as a wall so callers don't have to range-check first.
func (a AreaDefinition) WallAt(x, z int) bool {
	if !a.InBounds(x, z) {
		return true
	}
	return a.Walls[z][x] == TileRock
}

// CeilingAt reports whether the cell has a solid ceiling slab. Out-of-
// bounds reads as no-ceiling so the renderer doesn't paint slabs past
// the map edge. Maps loaded from older .map files without a ceiling:
// section get a blank ceiling layer at parse time, so this always
// returns false for them.
func (a AreaDefinition) CeilingAt(x, z int) bool {
	if !a.InBounds(x, z) {
		return false
	}
	if len(a.Ceiling) != a.Height {
		return false
	}
	row := a.Ceiling[z]
	if x >= len(row) {
		return false
	}
	return row[x] == TileCeilingSolid
}

// TileAt returns a "compositing" character for code that just wants to
// know what's most-significantly at a cell — walls win over props win
// over deep water win over open. Used by the minimap and any callers
// that haven't switched to explicit per-layer queries yet.
func (a AreaDefinition) TileAt(x, z int) byte {
	if !a.InBounds(x, z) {
		return TileRock
	}
	if a.Walls[z][x] == TileRock {
		return TileRock
	}
	if p := a.Props[z][x]; IsPropChar(p) {
		return p
	}
	if a.Floor[z][x] == FloorDeepWater {
		return FloorDeepWater
	}
	return TileOpen
}

// BlockedAt reports whether movement into this cell is impossible.
// Either the walls layer has a wall, or the props layer holds a blocker.
// Out-of-bounds reads as blocked (matches WallAt's convention) so callers
// don't have to range-check first — note this means FloorAt(OOB) is false,
// not "the cell is open but past the map." A caller that needs to
// distinguish "off-map" from "blocked-on-map" should InBounds() first.
//
// Runtime blockers like chests sit on a separate slice (GameState.Chests)
// and are NOT considered here — explore.go layers a ChestAt check on
// top of BlockedAt to avoid coupling the area definition to runtime
// state, and so the editor (which only ever holds an AreaDefinition)
// can still call BlockedAt without seeing a phantom block.
func (a AreaDefinition) BlockedAt(x, z int) bool {
	if !a.InBounds(x, z) {
		return true
	}
	if a.Walls[z][x] == TileRock {
		return true
	}
	if IsPropChar(a.Props[z][x]) && !PropIsNonBlocking(a.Props[z][x]) {
		return true
	}
	return a.Floor[z][x] == FloorDeepWater
}

// PropIsNonBlocking reports whether a prop char, despite being a valid
// prop (IsPropChar true), should NOT block movement. Wall-mounted
// torches sit on the wall and leave the floor clear, so the player
// (and packs) can walk through their tile — letting torches line a
// corridor without sealing it. Every other prop blocks.
func PropIsNonBlocking(c byte) bool {
	return c == TileTorch
}

// EnterOpts parameterizes CanEnterTile. The zero value forbids door
// stepping, the player tile, and any pack-occupied tile — i.e. the
// strictest set of runtime blockers a pack faces during wandering.
// Callers flip the booleans for their context:
//   - Player step: AllowDoorTile=true (steps onto doors trigger area
//     transitions), AllowPlayerTile is meaningless (the player isn't
//     on the destination tile yet anyway), OccupiedPacks=nil (the
//     pack-collision rule is owned by the caller, which has the
//     separate "step into pack → engage" branch).
//   - Pack chase: AllowPlayerTile=true (the chase tile IS the player —
//     that's the engagement signal), AllowDoorTile=false, supply
//     OccupiedPacks to skip squares held by other packs.
//   - Pack wander: AllowPlayerTile=false (a passive wander shouldn't
//     accidentally engage), AllowDoorTile=false, OccupiedPacks set.
//
// PlayerTileX/Z is only consulted when OccupiedPacks is non-nil OR
// AllowPlayerTile is set — saves callers from having to fish out the
// player's tile when their context doesn't care.
type EnterOpts struct {
	AllowDoorTile   bool
	AllowPlayerTile bool
	PlayerTileX     int
	PlayerTileZ     int
	OccupiedPacks   map[[2]int]bool
}

// CanEnterTile is the single-source-of-truth predicate for "can an
// actor legally step onto (tx, tz) right now." Composes the area's
// static BlockedAt (walls, props, deep water) with the runtime
// blockers (chests, doors, packs, player). Centralizes a rule that
// used to live in both packai.go and explore/movement.go's startStep —
// future balance tweaks (packs-block-player, packs-walk-through-doors,
// chests-don't-block-packs) are a one-line flip of EnterOpts at the
// call site instead of forking the predicate.
func CanEnterTile(g *GameState, tx, tz int, opts EnterOpts) bool {
	if g == nil || !g.Area.InBounds(tx, tz) {
		return false
	}
	if g.Area.BlockedAt(tx, tz) {
		return false
	}
	if ChestIndexAt(g.Chests, tx, tz) >= 0 {
		return false
	}
	if !opts.AllowDoorTile && DoorIndexAt(g.Doors, tx, tz) >= 0 {
		return false
	}
	// PlayerTileX/Z is only meaningful when the caller actually declared
	// a player position — either by setting AllowPlayerTile (pack-chase
	// "the player IS the destination" case) or by passing OccupiedPacks
	// (any pack-AI path that needs to avoid stepping onto the player).
	// Without this gate the zero-default PlayerTileX/Z=(0,0) would falsely
	// block every caller's step toward tile (0, 0).
	if opts.AllowPlayerTile || opts.OccupiedPacks != nil {
		if tx == opts.PlayerTileX && tz == opts.PlayerTileZ {
			return opts.AllowPlayerTile
		}
	}
	if opts.OccupiedPacks != nil && opts.OccupiedPacks[[2]int{tx, tz}] {
		return false
	}
	return true
}

// ChestTakeAllRow is the synthetic row index that sits one past the
// last live-stack row in the chest-open modal — selecting it drains
// every remaining stack. Both the input loop (explore/chest.go) and
// the renderer (render/chest.go) pivot on this index, so the convention
// lives in core where neither package "owns" it. `stackCount` is
// len(LiveStacks(chest.Items)).
func ChestTakeAllRow(stackCount int) int { return stackCount }

// MarkChestLootedIfEmpty flips the chest's Looted flag (and clears any
// zero-count stack residue) when every stack has been drained. Single
// source of truth for "is this chest empty enough to render with the
// lid open?" — replaces the small empty-check duplicated between the
// chest-modal's Take-All path and its close path.
func MarkChestLootedIfEmpty(c *Chest) {
	if c == nil {
		return
	}
	if len(LiveStacks(c.Items)) == 0 {
		c.Items = nil
		c.Looted = true
	}
}

// ChestIndexAt returns the index of the chest on the given tile, or -1
// when no chest is there. Linear scan; chest counts per map are tiny
// (<10 typical) so a map keyed by [2]int isn't worth the allocation.
// DoorIndexAt and PackIndexAtTile follow the same pattern via
// slices.IndexFunc — three linear "find spawn at tile" lookups sharing
// the stdlib idiom.
func ChestIndexAt(chests []Chest, x, z int) int {
	return slices.IndexFunc(chests, func(c Chest) bool { return c.TileX == x && c.TileZ == z })
}

// AdjacentChestIndex returns the index of a chest the player can reach
// from (x, z) via a one-tile step in any cardinal direction, or -1 when
// none. Used by movement and rendering paths that need ANY adjacent
// chest (including looted ones — the lid model still blocks the tile).
// Diagonals don't count — the player can't step diagonally either.
func AdjacentChestIndex(chests []Chest, x, z int) int {
	for i, c := range chests {
		if AbsInt(c.TileX-x)+AbsInt(c.TileZ-z) == 1 {
			return i
		}
	}
	return -1
}

// AdjacentInteractableChestIndex is the openable-only variant: skips
// chests that have already been looted, since their dialog can't be
// re-opened. Both the Confirm-key interaction (explore) and the
// adjacent-chest prompt (render) want this filtered form, so the
// "is there an openable chest next to me?" rule lives in one place.
func AdjacentInteractableChestIndex(chests []Chest, x, z int) int {
	idx := AdjacentChestIndex(chests, x, z)
	if idx < 0 || chests[idx].Looted {
		return -1
	}
	return idx
}

// PackSpawnIndexAt returns the index of the pack spawn at the given
// tile, or -1 when none. Authored-list mirror of the in-pack ChestIndexAt
// helper; the editor's hover summary uses it and a future "tile
// inspector" anywhere else can reuse it.
func PackSpawnIndexAt(spawns []PackSpawn, x, z int) int {
	for i, sp := range spawns {
		if sp.TileX == x && sp.TileZ == z {
			return i
		}
	}
	return -1
}

// ChestSpawnIndexAt returns the index of the chest spawn at the given
// tile, or -1 when none. Authored-list counterpart to runtime
// ChestIndexAt.
func ChestSpawnIndexAt(spawns []ChestSpawn, x, z int) int {
	for i, sp := range spawns {
		if sp.TileX == x && sp.TileZ == z {
			return i
		}
	}
	return -1
}

// DoorSpawnIndexAt returns the index of the door spawn at the given
// tile, or -1 when none. Authored-list counterpart to runtime
// DoorIndexAt.
func DoorSpawnIndexAt(spawns []DoorSpawn, x, z int) int {
	for i, sp := range spawns {
		if sp.TileX == x && sp.TileZ == z {
			return i
		}
	}
	return -1
}

// AreaTileSummary returns a compact human-readable description of
// what's painted on the (x, z) tile across every layer + every
// entity list. Empty layers are omitted so a clean grass cell reads
// as just "Grass" rather than "Open / Grass / — / —". Returns
// "(empty)" only when the cell holds nothing. Returns "" for an
// out-of-bounds tile.
//
// Used by the editor's hover-tile readout and reusable by any future
// in-game tile inspector / debug overlay. Moved out of the editor
// package so the per-layer label + entity walk lives in one place.
func AreaTileSummary(a AreaDefinition, x, z int) string {
	if !a.InBounds(x, z) {
		return ""
	}
	parts := make([]string, 0, 8)
	if lbl := TileLabel(TileLayerWalls, a.Walls[z][x]); lbl != "" {
		parts = append(parts, lbl)
	}
	if lbl := TileLabel(TileLayerFloor, a.Floor[z][x]); lbl != "" {
		parts = append(parts, lbl)
	}
	if lbl := TileLabel(TileLayerDecor, a.Decor[z][x]); lbl != "" {
		parts = append(parts, lbl)
	}
	if lbl := TileLabel(TileLayerProps, a.Props[z][x]); lbl != "" {
		parts = append(parts, lbl)
	}
	if a.StartTileX == x && a.StartTileZ == z {
		parts = append(parts, "Start")
	}
	if PackSpawnIndexAt(a.PackSpawns, x, z) >= 0 {
		parts = append(parts, "Pack")
	}
	if ChestSpawnIndexAt(a.ChestSpawns, x, z) >= 0 {
		parts = append(parts, "Chest")
	}
	if DoorSpawnIndexAt(a.DoorSpawns, x, z) >= 0 {
		parts = append(parts, "Door")
	}
	if len(parts) == 0 {
		return "(empty)"
	}
	return joinSummary(parts)
}

// joinSummary concatenates AreaTileSummary parts with " / " — pulled
// out so core/map.go doesn't pull in "strings" just for one call.
func joinSummary(parts []string) string {
	out := parts[0]
	for _, p := range parts[1:] {
		out += " / " + p
	}
	return out
}

// FloorAt is the inverse of BlockedAt — true when the cell is walkable.
func (a AreaDefinition) FloorAt(x, z int) bool {
	return !a.BlockedAt(x, z)
}

// InBounds reports whether the (x, z) coordinate sits inside the area's
// declared dimensions.
func (a AreaDefinition) InBounds(x, z int) bool {
	return z >= 0 && z < a.Height && x >= 0 && x < a.Width
}

// MultiTileOffset is one cell of a multi-tile prop's footprint, expressed
// as a (dx, dz) offset from the anchor tile.
type MultiTileOffset struct {
	DX, DZ int
}

// FootprintCentroid returns the (dx, dz) offset from the anchor tile's
// center to the geometric center of the whole footprint, expressed in
// tile-space units (one unit = TileSize). For a 2×2 footprint anchored
// at the top-left the centroid sits at (+0.5, +0.5); for a 1×2 anchored
// on the left it sits at (+0.5, 0). Returns (0, 0) for a single-tile
// footprint or an empty slice — caller can multiply by TileSize for
// world-space offsets.
//
// Used by the renderer's multi-tile prop / decor dispatch so the
// per-anchor "+TileSize*0.5" literals stay out of the call site.
func FootprintCentroid(offsets []MultiTileOffset) (dx, dz float32) {
	if len(offsets) <= 1 {
		return 0, 0
	}
	var sx, sz float32
	for _, o := range offsets {
		sx += float32(o.DX)
		sz += float32(o.DZ)
	}
	n := float32(len(offsets))
	return sx / n, sz / n
}

// FootprintWorldOffset returns the world-space (X, Z) offset from the
// anchor tile's center to the centroid of the footprint — i.e.
// FootprintCentroid scaled by TileSize. Drops the per-call-site
// `cdx*core.TileSize, cdz*core.TileSize` arithmetic the renderer was
// repeating once for props and once for decor.
func FootprintWorldOffset(offsets []MultiTileOffset) (x, z float32) {
	cdx, cdz := FootprintCentroid(offsets)
	return cdx * TileSize, cdz * TileSize
}

// PropFootprint returns the cells occupied by the prop anchored at the
// given anchor char, including the anchor itself at (0,0). Single-tile
// props return a single-element slice. Returns nil for non-anchor or
// unknown chars — callers should treat that as "single-tile". The
// returned slice is fresh; modifying it is harmless.
//
// Used by the editor to auto-fill footprint shadow chars when painting a
// multi-tile anchor, and to validate the footprint fits in-bounds before
// the click commits.
func PropFootprint(anchor byte) []MultiTileOffset {
	switch anchor {
	case TileRockFormation:
		return []MultiTileOffset{{0, 0}, {1, 0}, {0, 1}, {1, 1}}
	}
	return nil
}

// PropFootprintTail returns the char that should be written to the tail
// cells of a multi-tile prop's footprint (every cell except the anchor).
// Returns 0 if the anchor is single-tile or unknown.
func PropFootprintTail(anchor byte) byte {
	switch anchor {
	case TileRockFormation:
		return TileRockFormationTail
	}
	return 0
}

// DecorFootprint mirrors PropFootprint for the decor layer. Decor doesn't
// block, but multi-tile decor (archways) still needs to mark its
// footprint so the renderer's anchor draws the spanning mesh and the
// tail draws nothing.
func DecorFootprint(anchor byte) []MultiTileOffset {
	switch anchor {
	case DecorArchway:
		return []MultiTileOffset{{0, 0}, {1, 0}}
	}
	return nil
}

// DecorFootprintTail returns the tail char for a multi-tile decor anchor.
func DecorFootprintTail(anchor byte) byte {
	switch anchor {
	case DecorArchway:
		return DecorArchwayTail
	}
	return 0
}

// propTileCharList is the canonical list of every prop-layer char that
// blocks movement. IsPropChar dispatches against it via a set; the
// renderer's minimap walks PropTileChars() to assert its color map
// covers every entry. Kept package-private so external callers can't
// mutate the slice — use the PropTileChars() accessor for read-only
// access, which returns a defensive copy.
var propTileCharList = []byte{
	TileTree, TileTreeXL, TileTreeTall, TileTreeTwin, TileTreeYoung,
	TileRockLarge, TileBushLarge,
	TileCrate, TileBarrel, TileUrn, TileStalagmite,
	TilePillar, TileBrokenPillar, TileStatue, TileObelisk, TileFountain,
	TileRockCairn, TileRockFormation, TileRockFormationTail,
	// Outdoor batch.
	TileWell, TileGravestone, TileSignPost, TileHayBale, TileScarecrow,
	// Dungeon-interior batch.
	TileBookshelf, TileTable, TileBed, TileBrazier, TileSarcophagus,
	// Wall torch — a prop char for rendering/editor/registry, but
	// exempted from blocking in BlockedAt (it mounts on the wall).
	TileTorch,
}

// propTileCharSet is the O(1) lookup for IsPropChar, built once at
// init from propTileCharList so neither list can drift from the other.
var propTileCharSet = buildPropTileCharSet()

func buildPropTileCharSet() map[byte]struct{} {
	m := make(map[byte]struct{}, len(propTileCharList))
	for _, c := range propTileCharList {
		m[c] = struct{}{}
	}
	return m
}

// PropTileChars returns the list of every blocking prop-layer char as
// a defensive copy. Read-only seam — the underlying list lives package-
// private so callers can't mutate it and desync propTileCharSet. Used
// by the renderer's minimap init to assert color coverage.
func PropTileChars() []byte {
	out := make([]byte, len(propTileCharList))
	copy(out, propTileCharList)
	return out
}

// IsPropChar returns true if c names a known blocking prop. Open-prop
// cells use '.'; future props (chests, doors) get added to propTileCharList.
func IsPropChar(c byte) bool {
	_, ok := propTileCharSet[c]
	return ok
}

// decorTileCharList is the canonical list of every explicit decor-layer
// char that has a renderable model. '.' (auto-scatter) and '_' (force-
// empty) are deliberately excluded — they're sentinels handled by the
// renderer's dispatch, not entries in decorModels. The renderer asserts
// at init that every entry here has a registered model; the editor
// derives its brush palette from layerBrushes which mirrors this set.
var decorTileCharList = []byte{
	DecorBush, DecorMushroom, DecorPebble,
	DecorTallGrass, DecorFlowers, DecorClover, DecorReeds,
	DecorBones, DecorScorch, DecorBlood, DecorCobweb,
	DecorStump, DecorLog, DecorLeafPile,
	DecorArchway, DecorArchwayTail,
	DecorLilypad,
	// Atmospheric batch — interior dressings, weather, foliage.
	DecorRug, DecorCandle, DecorBootprints,
	DecorAshHeap, DecorPuddle, DecorRootCluster,
}

// DecorTileChars returns the list of every renderable decor-layer char
// as a defensive copy. Used by the renderer's init to assert coverage
// of decorModels — adding a DecorXxx const without a loader panics at
// startup instead of silently no-op'ing in drawDecor.
func DecorTileChars() []byte {
	out := make([]byte, len(decorTileCharList))
	copy(out, decorTileCharList)
	return out
}

// BlockingFloorChars returns every floor-layer char that BlockedAt
// reports true for. Today that's just FloorDeepWater; the slice exists
// so callers (minimap color coverage, debug overlays) don't have to
// open-code the set and a future blocker (lava, void) is one append.
func BlockingFloorChars() []byte {
	return []byte{FloorDeepWater}
}

// floorTileCharList enumerates every floor-layer char with a defined
// visual (universal variants + material-keyed variants). Excludes the
// sentinel FloorAuto. Adding a new floor char here automatically
// extends TileLabel coverage (via the label table below) and feeds
// any future "list all floor types" UI.
var floorTileCharList = []byte{
	FloorGrass, FloorDirt, FloorDarkGrass, FloorStone,
	FloorCobble, FloorPlank, FloorWater, FloorDeepWater,
	FloorSand, FloorSnow,
}

// FloorTileChars returns the canonical list of named floor-layer
// chars as a defensive copy. Mirrors PropTileChars / DecorTileChars.
func FloorTileChars() []byte {
	out := make([]byte, len(floorTileCharList))
	copy(out, floorTileCharList)
	return out
}

// TileLayer enumerates the four authored grid layers that carry a
// tile char (walls / floor / decor / props). The editor's `Layer`
// enum adds Ceiling and Entities on top: Ceiling has its own grid
// but reuses '.' / '#' from the walls registry rather than declaring
// new chars (so TileLabel's per-layer dispatch doesn't add a row for
// it), and Entities aren't tile chars at all — they live in the
// spawn slices. If a future surface needs to walk all SIX of the
// editor's layers, it should keep that asymmetry in mind.
type TileLayer int

const (
	TileLayerWalls TileLayer = iota
	TileLayerFloor
	TileLayerDecor
	TileLayerProps
)

// tileLabelTable is the per-(layer, char) human label registry that
// powers TileLabel. Sentinels (empty/auto chars) map to "" so the
// debug overlay can skip them without an extra branch at the call
// site. The init() block below asserts that every char in the
// canonical FloorTileChars / DecorTileChars / PropTileChars / walls /
// ceiling sets has an entry — adding a new tile const without a
// label now panics at startup instead of returning "?" silently.
var tileLabelTable = map[TileLayer]map[byte]string{
	TileLayerWalls: {
		TileOpen: "",
		TileRock: "Wall",
	},
	TileLayerFloor: {
		FloorAuto:      "",
		FloorGrass:     "Grass",
		FloorDirt:      "Dirt",
		FloorDarkGrass: "Dark Grass",
		FloorStone:     "Stone",
		FloorCobble:    "Cobble",
		FloorPlank:     "Planks",
		FloorWater:     "Water",
		FloorDeepWater: "Deep Water",
		FloorSand:      "Sand",
		FloorSnow:      "Snow",
	},
	TileLayerDecor: {
		DecorAuto:        "",
		DecorEmpty:       "",
		DecorBush:        "Bush",
		DecorMushroom:    "Mushroom",
		DecorPebble:      "Pebble",
		DecorTallGrass:   "Tall Grass",
		DecorFlowers:     "Flowers",
		DecorClover:      "Clover",
		DecorReeds:       "Reeds",
		DecorBones:       "Bones",
		DecorScorch:      "Scorch",
		DecorBlood:       "Blood",
		DecorCobweb:      "Cobweb",
		DecorStump:       "Stump",
		DecorLog:         "Log",
		DecorLeafPile:    "Leaf Pile",
		DecorArchway:     "Arch (left)",
		DecorArchwayTail: "Arch (right)",
		DecorLilypad:     "Lilypad",
		DecorRug:         "Rug",
		DecorCandle:      "Candle",
		DecorBootprints:  "Boot Prints",
		DecorAshHeap:     "Ash Heap",
		DecorPuddle:      "Puddle",
		DecorRootCluster: "Roots",
	},
	TileLayerProps: {
		TilePropEmpty:         "",
		TileTree:              "Tree",
		TileTreeXL:            "Tree XL",
		TileTreeTall:          "Tall Tree",
		TileTreeTwin:          "Twin Trees",
		TileTreeYoung:         "Young Tree",
		TileRockLarge:         "Boulder",
		TileBushLarge:         "Large Bush",
		TileCrate:             "Crate",
		TileBarrel:            "Barrel",
		TileUrn:               "Urn",
		TileStalagmite:        "Stalagmite",
		TilePillar:            "Pillar",
		TileBrokenPillar:      "Broken Pillar",
		TileStatue:            "Statue",
		TileObelisk:           "Obelisk",
		TileFountain:          "Fountain",
		TileRockCairn:         "Rock Cairn",
		TileRockFormation:     "Rock Formation (anchor)",
		TileRockFormationTail: "Rock Formation (tail)",
		TileWell:              "Well",
		TileGravestone:        "Gravestone",
		TileSignPost:          "Sign Post",
		TileHayBale:           "Hay Bale",
		TileScarecrow:         "Scarecrow",
		TileBookshelf:         "Bookshelf",
		TileTable:             "Table",
		TileBed:               "Bed",
		TileBrazier:           "Brazier",
		TileSarcophagus:       "Sarcophagus",
		TileTorch:             "Wall Torch",
	},
}

// init asserts every authored tile char has a TileLabel entry. The
// walks below mirror the existing minimap-color and entityBrushColors
// inits — adding a new floor/decor/prop const without a label panics
// at startup instead of returning "?" from a debug overlay later.
//
// Additionally, asserts that any byte appearing on multiple grid
// layers is registered as an INTENDED cross-layer overlap in
// crossLayerCharOverlaps. The per-layer dispatch in
// world.go / map.go / TileLabel makes overlaps safe today, but the
// editor's hoverTileSummary and any future "what's at this tile?"
// surface would render two different labels for the same char. An
// unregistered collision panics at startup so the author has to
// confirm the overlap is deliberate.
func init() {
	floorLabels := tileLabelTable[TileLayerFloor]
	for _, c := range floorTileCharList {
		if _, ok := floorLabels[c]; !ok {
			panic("core: floor char '" + string(c) + "' missing from tileLabelTable[TileLayerFloor]")
		}
	}
	decorLabels := tileLabelTable[TileLayerDecor]
	for _, c := range decorTileCharList {
		if _, ok := decorLabels[c]; !ok {
			panic("core: decor char '" + string(c) + "' missing from tileLabelTable[TileLayerDecor]")
		}
	}
	propLabels := tileLabelTable[TileLayerProps]
	for _, c := range propTileCharList {
		if _, ok := propLabels[c]; !ok {
			panic("core: prop char '" + string(c) + "' missing from tileLabelTable[TileLayerProps]")
		}
	}
	assertNoUnregisteredCrossLayerOverlaps()
}

// crossLayerOverlap pairs the two layers that share a byte AND the
// label each layer assigns to it. Both are recorded so a future
// reader can see WHY the overlap is intentional ("water on floor and
// a well prop happen to spell the same char").
type crossLayerOverlap struct {
	A, B   TileLayer
	Char   byte
	NameA  string
	NameB  string
	Reason string
}

// crossLayerCharOverlaps is the registry of deliberately-shared byte
// values across grid layers. Each pair was vetted at the time of
// authoring: per-layer dispatch keeps the overlap safe AND no surface
// reads "what's at this tile?" without specifying the layer (the
// editor's hoverTileSummary walks every layer separately and labels
// them with the layer name, so two labels for the same char are fine).
// A new overlap must register here or the init assert below panics.
var crossLayerCharOverlaps = []crossLayerOverlap{
	{
		A: TileLayerFloor, B: TileLayerProps, Char: 'W',
		NameA: "FloorDeepWater", NameB: "TileWell",
		Reason: "deep-water floor tile and the village well prop happen to share the 'W' mnemonic; layers are dispatched independently.",
	},
	{
		A: TileLayerDecor, B: TileLayerProps, Char: 'A',
		NameA: "DecorArchway", NameB: "TileSarcophagus",
		Reason: "archway-anchor decor and stone sarcophagus prop share the 'A' mnemonic.",
	},
	{
		A: TileLayerDecor, B: TileLayerProps, Char: 'L',
		NameA: "DecorLeafPile", NameB: "TileTable",
		Reason: "leaf-pile decor and dungeon table prop share the 'L' mnemonic (L for Leaf and L for table-Leg).",
	},
	{
		A: TileLayerDecor, B: TileLayerFloor, Char: 'c',
		NameA: "DecorCandle", NameB: "FloorCobble",
		Reason: "candle decor and cobblestone floor tile share the 'c' mnemonic.",
	},
	{
		A: TileLayerDecor, B: TileLayerFloor, Char: 'i',
		NameA: "DecorBootprints", NameB: "FloorSnow",
		Reason: "bootprint decor and snow floor tile share the 'i' mnemonic — bootprints on snow happens to be a natural pairing.",
	},
	{
		A: TileLayerDecor, B: TileLayerFloor, Char: 'k',
		NameA: "DecorRootCluster", NameB: "FloorDarkGrass",
		Reason: "root-cluster decor and dark-grass floor share the 'k' mnemonic.",
	},
}

// assertNoUnregisteredCrossLayerOverlaps walks every (layer, char)
// pair in tileLabelTable and pairs them up by char. Any char appearing
// on >1 layer that isn't covered by crossLayerCharOverlaps panics.
// Sentinel chars ('.' / '_' / '#') are skipped — those are shared by
// design across layers (e.g. '.' means "open" everywhere).
func assertNoUnregisteredCrossLayerOverlaps() {
	sentinels := map[byte]struct{}{
		'.': {}, '_': {}, '#': {},
	}
	// chars[c] -> list of layers where it's a registered tile.
	chars := make(map[byte][]TileLayer)
	for layer, labels := range tileLabelTable {
		for c := range labels {
			if _, sentinel := sentinels[c]; sentinel {
				continue
			}
			chars[c] = append(chars[c], layer)
		}
	}
	registered := make(map[byte]struct{}, len(crossLayerCharOverlaps))
	for _, o := range crossLayerCharOverlaps {
		registered[o.Char] = struct{}{}
	}
	for c, layers := range chars {
		if len(layers) < 2 {
			continue
		}
		if _, ok := registered[c]; ok {
			continue
		}
		panic("core: cross-layer char '" + string(c) + "' is shared by multiple layers but not registered in crossLayerCharOverlaps — add an entry documenting why this is deliberate")
	}
}

// TileLabel returns a short human-readable name for a tile char on the
// given layer. Empty cells and "auto" sentinels return the empty string
// so the debug overlay can skip them without an extra check at the call
// site. Unknown chars return "?". Table-driven from tileLabelTable so
// adding a new tile is one row, not three switches.
func TileLabel(layer TileLayer, c byte) string {
	if labels, ok := tileLabelTable[layer]; ok {
		if label, ok := labels[c]; ok {
			return label
		}
	}
	return "?"
}

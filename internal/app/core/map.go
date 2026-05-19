package core

import (
	"fmt"
	"math"
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
)

// Props layer. TilePropEmpty marks an open cell; every other char listed
// here is a blocker. Mirrors FloorAuto / DecorAuto on the other layers so
// callers don't open-code '.' for "no prop here."
const (
	TilePropEmpty = '.' // open cell, no prop
	TileTree      = 'T' // regular tree, blocks
	TileTreeXL    = 'X' // extra-large tree, blocks
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
)

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
		for _, kind := range spawn.Members {
			members = append(members, NewEnemy(kind))
		}
		packs = append(packs, Pack{TileX: snap.TileX, TileZ: snap.TileZ, Members: members})
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
	if IsPropChar(a.Props[z][x]) {
		return true
	}
	return a.Floor[z][x] == FloorDeepWater
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
func ChestIndexAt(chests []Chest, x, z int) int {
	for i, c := range chests {
		if c.TileX == x && c.TileZ == z {
			return i
		}
	}
	return -1
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
	TileTree, TileTreeXL, TileRockLarge, TileBushLarge,
	TileCrate, TileBarrel, TileUrn, TileStalagmite,
	TilePillar, TileBrokenPillar, TileStatue, TileObelisk, TileFountain,
	TileRockCairn, TileRockFormation, TileRockFormationTail,
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

// TileLayer enumerates the four authored grid layers. Used as a typed
// parameter to TileLabel so callers can't pass a typo'd layer string
// silently and get "?" back.
type TileLayer int

const (
	TileLayerWalls TileLayer = iota
	TileLayerFloor
	TileLayerDecor
	TileLayerProps
)

// TileLabel returns a short human-readable name for a tile char on the
// given layer. Empty cells and "auto" sentinels return the empty string
// so the debug overlay can skip them without an extra check at the call
// site. Unknown chars return "?".
func TileLabel(layer TileLayer, c byte) string {
	switch layer {
	case TileLayerWalls:
		switch c {
		case TileRock:
			return "Wall"
		case TileOpen:
			return ""
		}
	case TileLayerFloor:
		switch c {
		case FloorAuto:
			return ""
		case FloorGrass:
			return "Grass"
		case FloorDirt:
			return "Dirt"
		case FloorDarkGrass:
			return "Dark Grass"
		case FloorStone:
			return "Stone"
		case FloorCobble:
			return "Cobble"
		case FloorPlank:
			return "Planks"
		case FloorWater:
			return "Water"
		case FloorDeepWater:
			return "Deep Water"
		case FloorSand:
			return "Sand"
		case FloorSnow:
			return "Snow"
		}
	case TileLayerDecor:
		switch c {
		case DecorAuto, DecorEmpty:
			return ""
		case DecorBush:
			return "Bush"
		case DecorMushroom:
			return "Mushroom"
		case DecorPebble:
			return "Pebble"
		case DecorTallGrass:
			return "Tall Grass"
		case DecorFlowers:
			return "Flowers"
		case DecorClover:
			return "Clover"
		case DecorReeds:
			return "Reeds"
		case DecorBones:
			return "Bones"
		case DecorScorch:
			return "Scorch"
		case DecorBlood:
			return "Blood"
		case DecorCobweb:
			return "Cobweb"
		case DecorStump:
			return "Stump"
		case DecorLog:
			return "Log"
		case DecorLeafPile:
			return "Leaf Pile"
		case DecorArchway:
			return "Arch (left)"
		case DecorArchwayTail:
			return "Arch (right)"
		case DecorLilypad:
			return "Lilypad"
		}
	case TileLayerProps:
		switch c {
		case TilePropEmpty:
			return ""
		case TileTree:
			return "Tree"
		case TileTreeXL:
			return "Tree XL"
		case TileRockLarge:
			return "Boulder"
		case TileBushLarge:
			return "Large Bush"
		case TileCrate:
			return "Crate"
		case TileBarrel:
			return "Barrel"
		case TileUrn:
			return "Urn"
		case TileStalagmite:
			return "Stalagmite"
		case TilePillar:
			return "Pillar"
		case TileBrokenPillar:
			return "Broken Pillar"
		case TileStatue:
			return "Statue"
		case TileObelisk:
			return "Obelisk"
		case TileFountain:
			return "Fountain"
		case TileRockCairn:
			return "Rock Cairn"
		case TileRockFormation:
			return "Rock Formation (anchor)"
		case TileRockFormationTail:
			return "Rock Formation (tail)"
		}
	}
	return "?"
}

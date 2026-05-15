package core

import (
	"math"
)

// Tile characters. Grouped by the layer that owns them. The .map on-disk
// format uses these literal bytes; the editor's brush palettes paint them.

// Walls layer. TileOpen marks an open cell; TileRock is a wall blocker.
// Both exported so callers don't have to open-code '.' / '#' against the
// walls grid.
const (
	TileOpen = '.' // open cell (walkable)
	TileRock = '#' // wall blocker
)

// Floor layer. Walkable surfaces — never block. Material-keyed variants
// (grass / dirt / dark grass / stone) read against the material's own
// floor pixels; universal variants (path / planks / water / sand / snow)
// load their own textures and apply in any material.
const (
	FloorAuto      = '.' // pick a variant by per-tile hash (back-compat default)
	FloorGrass     = 'g'
	FloorDirt      = 'd'
	FloorDarkGrass = 'k'
	FloorStone     = 's'
	// Universal floor variants — render the same in any material set so an
	// author can lay a stone path through a forest or a wooden plank floor
	// across a cave. None of these block movement.
	FloorCobble = 'c' // mortared cobblestone path
	FloorPlank  = 'w' // wooden planks
	FloorWater  = '~' // shallow water — walkable, just a different look
	FloorSand   = 'n' // pale sand
	FloorSnow   = 'i' // packed snow
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
	DecorBones   = 'o' // skull + scattered bones
	DecorScorch  = 'x' // black scorch ring
	DecorBlood   = '!' // dried bloodstain
	DecorCobweb  = '*' // corner cobweb
	// Forest leftovers: dead wood that breaks up grass without blocking.
	DecorStump    = 't' // weathered tree stump
	DecorLog      = 'l' // mossy fallen log
	DecorLeafPile = 'L' // pile of fallen leaves
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
	TileCrate         = 'C' // wooden crate
	TileBarrel        = 'R' // banded barrel
	TileUrn           = 'U' // belly-shouldered urn
	TileStalagmite    = 'S' // cave stalagmite spire
	TilePillar        = 'P' // intact stone pillar with capital
	TileBrokenPillar  = 'I' // toppled / chest-high pillar stub
	TileStatue        = 'M' // weathered humanoid statue
	TileObelisk       = 'Q' // tall four-sided pyramid-capped obelisk
	TileFountain      = 'F' // low fountain with a central plume
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

// TileAt returns a "compositing" character for code that just wants to
// know what's most-significantly at a cell — walls win over props win
// over open. Used by the minimap and any callers that haven't switched
// to explicit per-layer queries yet.
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
	return TileOpen
}

// BlockedAt reports whether movement into this cell is impossible.
// Either the walls layer has a wall, or the props layer holds a blocker.
// Out-of-bounds reads as blocked (matches WallAt's convention) so callers
// don't have to range-check first — note this means FloorAt(OOB) is false,
// not "the cell is open but past the map." A caller that needs to
// distinguish "off-map" from "blocked-on-map" should InBounds() first.
func (a AreaDefinition) BlockedAt(x, z int) bool {
	if !a.InBounds(x, z) {
		return true
	}
	if a.Walls[z][x] == TileRock {
		return true
	}
	return IsPropChar(a.Props[z][x])
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

// IsPropChar returns true if c names a known blocking prop. Open-prop
// cells use '.'; future props (chests, doors) get added here.
func IsPropChar(c byte) bool {
	switch c {
	case TileTree, TileTreeXL, TileRockLarge, TileBushLarge,
		TileCrate, TileBarrel, TileUrn, TileStalagmite,
		TilePillar, TileBrokenPillar, TileStatue, TileObelisk, TileFountain:
		return true
	}
	return false
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
		}
	}
	return "?"
}

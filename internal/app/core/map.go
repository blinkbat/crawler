package core

import (
	"math"
)

// Tile characters. Grouped by the layer that owns them. The .map on-disk
// format uses these literal bytes; the editor's brush palettes paint them.

// Walls layer.
const (
	tileFloor = '.' // package-private: documents "open" without leaking an unused export
	TileRock  = '#' // wall blocker
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

// Props layer. Empty cell is '.'. Every char listed here blocks movement.
const (
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

// placePacks converts the area's pack-spawn placeholders into runtime
// Packs, snapping each pack's tile to the nearest open square so the
// author doesn't have to perfect placement against geometry. The player's
// start tile is seeded as occupied so the snapping never lands on it.
// Empty pack rosters are skipped — a pack with zero members has no field
// representative to render or engage.
func placePacks(a AreaDefinition) []Pack {
	packs := make([]Pack, 0, len(a.PackSpawns))
	occupied := map[[2]int]bool{{a.StartTileX, a.StartTileZ}: true}
	for _, spawn := range a.PackSpawns {
		if len(spawn.Members) == 0 {
			continue
		}
		x, z := nearestOpenTile(a, spawn.TileX, spawn.TileZ, occupied)
		if x < 0 {
			continue
		}
		occupied[[2]int{x, z}] = true
		members := make([]Enemy, 0, len(spawn.Members))
		for _, kind := range spawn.Members {
			members = append(members, NewEnemy(kind))
		}
		packs = append(packs, Pack{TileX: x, TileZ: z, Members: members})
	}
	return packs
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
	return tileFloor
}

// BlockedAt reports whether movement into this cell is impossible.
// Either the walls layer has a wall, or the props layer holds a blocker.
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

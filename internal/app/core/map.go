package core

import (
	"math"
)

// Tile characters. Grouped by the layer that owns them. The .map on-disk
// format uses these literal bytes; the editor's brush palettes paint them.

// Walls layer.
const (
	TileFloor = '.' // historical name; in the layered format this means "open"
	TileRock  = '#' // wall blocker
)

// Floor layer.
const (
	FloorAuto      = '.' // pick a variant by per-tile hash (back-compat default)
	FloorGrass     = 'g'
	FloorDirt      = 'd'
	FloorDarkGrass = 'k'
	FloorStone     = 's'
)

// Decor layer. '.' means "let the renderer's auto-scatter decide"; '_'
// suppresses any auto decor; explicit chars force a specific small prop.
const (
	DecorAuto     = '.'
	DecorEmpty    = '_'
	DecorBush     = 'b'
	DecorMushroom = 'm'
	DecorPebble   = 'p'
)

// Props layer. Empty cell is '.'.
const (
	TileTree      = 'T' // regular tree, blocks
	TileTreeXL    = 'X' // extra-large tree, blocks
	TileRockLarge = 'O' // boulder, blocks
	TileBushLarge = 'B' // dense bush, blocks
)

// NewGameMap composes a layered runtime map from four parallel grids.
// Width/height are derived from the layers; callers should validate they
// match before calling. Materials selects the texture/lighting set.
func NewGameMap(walls, floor, decor, props []string, materials MaterialSet) GameMap {
	height := len(walls)
	width := 0
	if height > 0 {
		width = len(walls[0])
	}
	return GameMap{
		Width:     width,
		Height:    height,
		Walls:     walls,
		Floor:     floor,
		Decor:     decor,
		Props:     props,
		Materials: materials,
	}
}

func placeEnemies(m GameMap, spawns []EnemySpawn, startX, startZ int) []Enemy {
	enemies := make([]Enemy, 0, len(spawns))
	occupied := map[[2]int]bool{{startX, startZ}: true}
	for _, spawn := range spawns {
		x, z := nearestOpenTile(m, spawn.TileX, spawn.TileZ, occupied)
		if x < 0 {
			continue
		}
		occupied[[2]int{x, z}] = true
		enemies = append(enemies, NewEnemy(spawn.Kind, x, z))
	}
	return enemies
}

func nearestOpenTile(m GameMap, wantX, wantZ int, occupied map[[2]int]bool) (int, int) {
	if m.FloorAt(wantX, wantZ) && !occupied[[2]int{wantX, wantZ}] {
		return wantX, wantZ
	}
	bestX, bestZ := -1, -1
	bestDist := math.MaxInt
	for z := 0; z < m.Height; z++ {
		for x := 0; x < m.Width; x++ {
			if !m.FloorAt(x, z) || occupied[[2]int{x, z}] {
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

// WallAt returns true if the cell holds a wall in the walls layer.
func (m GameMap) WallAt(x, z int) bool {
	if !m.inBounds(x, z) {
		return true
	}
	return m.Walls[z][x] == TileRock
}

// PropAt returns the prop character at the cell ('.' for empty).
func (m GameMap) PropAt(x, z int) byte {
	if !m.inBounds(x, z) {
		return '.'
	}
	return m.Props[z][x]
}

// FloorVariantAt returns the floor character (auto / variant). Wall cells
// still have a floor underneath conceptually — it's just not drawn.
func (m GameMap) FloorVariantAt(x, z int) byte {
	if !m.inBounds(x, z) {
		return FloorAuto
	}
	return m.Floor[z][x]
}

// DecorAt returns the decor character (auto / explicit / force-empty).
func (m GameMap) DecorAt(x, z int) byte {
	if !m.inBounds(x, z) {
		return DecorAuto
	}
	return m.Decor[z][x]
}

// TileAt returns a "compositing" character for code that just wants to
// know what's most-significantly at a cell — walls win over props win
// over open. Used by the minimap and any callers that haven't switched
// to explicit per-layer queries yet.
func (m GameMap) TileAt(x, z int) byte {
	if !m.inBounds(x, z) {
		return TileRock
	}
	if m.Walls[z][x] == TileRock {
		return TileRock
	}
	if p := m.Props[z][x]; isPropChar(p) {
		return p
	}
	return TileFloor
}

// BlockedAt reports whether movement into this cell is impossible.
// Either the walls layer has a wall, or the props layer holds a blocker.
func (m GameMap) BlockedAt(x, z int) bool {
	if !m.inBounds(x, z) {
		return true
	}
	if m.Walls[z][x] == TileRock {
		return true
	}
	return isPropChar(m.Props[z][x])
}

// FloorAt is the inverse of BlockedAt — true when the cell is walkable.
func (m GameMap) FloorAt(x, z int) bool {
	return !m.BlockedAt(x, z)
}

func (m GameMap) inBounds(x, z int) bool {
	return z >= 0 && z < m.Height && x >= 0 && x < m.Width
}

// isPropChar returns true if c names a known blocking prop. Open-prop
// cells use '.'; future props (chests, doors) get added here.
func isPropChar(c byte) bool {
	switch c {
	case TileTree, TileTreeXL, TileRockLarge, TileBushLarge:
		return true
	}
	return false
}

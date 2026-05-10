package core

import (
	"math"
)

const (
	TileFloor     = '.'
	TileRock      = '#' // wall — no floor under, blocks movement
	TileTree      = 'T' // regular tree, blocks
	TileTreeXL    = 'X' // extra-large tree, blocks
	TileRockLarge = 'O' // boulder on a floor tile, blocks
	TileBushLarge = 'B' // dense bush on a floor tile, blocks
)

func NewGameMap(rows []string, materials MaterialSet) GameMap {
	height := len(rows)
	width := 0
	for _, row := range rows {
		if len(row) > width {
			width = len(row)
		}
	}
	return GameMap{Width: width, Height: height, Rows: rows, Materials: materials}
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

func (m GameMap) WallAt(x, z int) bool {
	return m.BlockedAt(x, z)
}

func (m GameMap) TileAt(x, z int) byte {
	if z < 0 || z >= m.Height || x < 0 || x >= len(m.Rows[z]) {
		return TileRock
	}
	return m.Rows[z][x]
}

func (m GameMap) BlockedAt(x, z int) bool {
	switch m.TileAt(x, z) {
	case TileRock, TileTree, TileTreeXL, TileRockLarge, TileBushLarge:
		return true
	default:
		return false
	}
}

func (m GameMap) FloorAt(x, z int) bool {
	return !m.BlockedAt(x, z)
}

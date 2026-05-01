package core

import (
	"math"
)

const (
	TileFloor = '.'
	TileRock  = '#'
	TileTree  = 'T'
)

var DungeonLayout = []string{
	"################",
	"#..............#",
	"#.####.#####.#.#",
	"#.#..#.#...#.#.#",
	"#.#..#.#.#.#.#.#",
	"#.#....#.#.#.#.#",
	"#.######.#.#.#.#",
	"#........#.#...#",
	"#.########.###.#",
	"#..............#",
	"#.####.######..#",
	"#.#......#.....#",
	"#.#.####.#.###.#",
	"#.#.#......#...#",
	"#...############",
	"################",
}

var FieldLayout = buildFieldLayout(30, 22, [][2]int{
	{5, 3}, {13, 3}, {22, 3},
	{8, 6}, {18, 6}, {25, 6},
	{4, 9}, {12, 10}, {21, 10},
	{7, 14}, {16, 14}, {24, 15},
	{11, 18}, {20, 18},
})

func buildFieldLayout(width, height int, trees [][2]int) []string {
	rows := make([][]byte, height)
	for z := 0; z < height; z++ {
		rows[z] = make([]byte, width)
		for x := 0; x < width; x++ {
			tile := byte(TileFloor)
			if x == 0 || z == 0 || x == width-1 || z == height-1 {
				tile = TileRock
			}
			rows[z][x] = tile
		}
	}
	for _, tree := range trees {
		x, z := tree[0], tree[1]
		if x > 0 && x < width-1 && z > 0 && z < height-1 {
			rows[z][x] = TileTree
		}
	}
	layout := make([]string, height)
	for z := range rows {
		layout[z] = string(rows[z])
	}
	return layout
}

func NewGameMap(rows []string) GameMap {
	height := len(rows)
	width := 0
	for _, row := range rows {
		if len(row) > width {
			width = len(row)
		}
	}
	return GameMap{Width: width, Height: height, Rows: rows}
}

func placeRats(m GameMap, desired [][2]int) []Enemy {
	enemies := make([]Enemy, 0, len(desired))
	occupied := map[[2]int]bool{{StartTileX, StartTileZ}: true}
	for _, pos := range desired {
		x, z := nearestOpenTile(m, pos[0], pos[1], occupied)
		if x < 0 {
			continue
		}
		occupied[[2]int{x, z}] = true
		enemies = append(enemies, NewRat(x, z))
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
	case TileRock, TileTree:
		return true
	default:
		return false
	}
}

func (m GameMap) FloorAt(x, z int) bool {
	return !m.BlockedAt(x, z)
}

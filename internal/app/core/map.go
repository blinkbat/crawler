package core

import (
	"math"
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

func NewGameMap(rows []string) GameMap {
	height := len(rows)
	width := 0
	if height > 0 {
		width = len(rows[0])
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
	if z < 0 || z >= m.Height || x < 0 || x >= m.Width {
		return true
	}
	return m.Rows[z][x] == '#'
}

func (m GameMap) FloorAt(x, z int) bool {
	return !m.WallAt(x, z)
}

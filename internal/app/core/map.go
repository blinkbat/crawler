package core

import (
	"fmt"
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

// FieldProp describes a hand-placed blocker on the field map. Multiple kinds
// share the placement system so the variety (regular tree, XL tree, large
// rock, large bush) can be sprinkled without grouping by type.
type FieldProp struct {
	X, Z int
	Tile byte
}

var FieldLayout = buildFieldLayout(30, 22, []FieldProp{
	// Regular trees scattered in clusters.
	{5, 3, TileTree}, {13, 3, TileTree}, {22, 3, TileTree},
	{18, 6, TileTree}, {25, 6, TileTree},
	{4, 9, TileTree}, {21, 10, TileTree},
	{7, 14, TileTree}, {24, 15, TileTree},
	{20, 18, TileTree},
	// A few XL trees punctuating the canopy.
	{8, 6, TileTreeXL},
	{12, 10, TileTreeXL},
	{16, 14, TileTreeXL},
	{11, 18, TileTreeXL},
	// Large boulders.
	{15, 5, TileRockLarge},
	{3, 12, TileRockLarge},
	{26, 11, TileRockLarge},
	{19, 16, TileRockLarge},
	// Large bushes.
	{9, 8, TileBushLarge},
	{17, 9, TileBushLarge},
	{6, 16, TileBushLarge},
	{23, 18, TileBushLarge},
	{14, 17, TileBushLarge},
})

func buildFieldLayout(width, height int, props []FieldProp) []string {
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
	for _, p := range props {
		// Props must land strictly inside the wall ring. A typo'd coordinate
		// at width-1 / 0 / height-1 used to be silently dropped, which made
		// missing props in the field a head-scratcher. Panic loudly at init
		// instead — this data is built once at startup, never user input.
		if p.X <= 0 || p.X >= width-1 || p.Z <= 0 || p.Z >= height-1 {
			panic(fmt.Sprintf("field prop at (%d,%d) is outside the playable interior (%dx%d)", p.X, p.Z, width, height))
		}
		rows[p.Z][p.X] = p.Tile
	}
	layout := make([]string, height)
	for z := range rows {
		layout[z] = string(rows[z])
	}
	return layout
}

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

package core

const (
	AreaDungeon AreaID = iota
	AreaField
)

const (
	MaterialDungeon MaterialSet = iota
	MaterialField
)

var areaDefinitions = []AreaDefinition{
	{
		ID:          AreaDungeon,
		Name:        "Stone Labyrinth",
		Layout:      DungeonLayout,
		Materials:   MaterialDungeon,
		StartTileX:  1,
		StartTileZ:  1,
		StartFacing: East,
		EnemySpawns: []EnemySpawn{
			// Mixed encounters: rat-and-bat packs read more interesting than
			// flat rat hordes, and the speed/HP differences force the player
			// to think about target priority.
			{Kind: EnemyRat, TileX: 6, TileZ: 1},
			{Kind: EnemyBat, TileX: 7, TileZ: 1},
			{Kind: EnemyRat, TileX: 9, TileZ: 7},
			{Kind: EnemyBat, TileX: 10, TileZ: 7},
			{Kind: EnemyRat, TileX: 3, TileZ: 13},
			{Kind: EnemyBat, TileX: 4, TileZ: 13},
			{Kind: EnemyRat, TileX: 5, TileZ: 13},
		},
		QuietMessage: "The dungeon is quiet.",
	},
	{
		ID:          AreaField,
		Name:        "Green Field",
		Layout:      FieldLayout,
		Materials:   MaterialField,
		StartTileX:  14,
		StartTileZ:  11,
		StartFacing: East,
		EnemySpawns: []EnemySpawn{
			{Kind: EnemyRat, TileX: 6, TileZ: 1},
			{Kind: EnemyRat, TileX: 7, TileZ: 1},
			{Kind: EnemyBat, TileX: 9, TileZ: 7},
			{Kind: EnemyRat, TileX: 10, TileZ: 7},
			{Kind: EnemyBat, TileX: 3, TileZ: 13},
			{Kind: EnemyRat, TileX: 4, TileZ: 13},
			{Kind: EnemyBat, TileX: 5, TileZ: 13},
		},
		QuietMessage: "The field is quiet.",
	},
}

func DefaultArea() AreaDefinition {
	return AreaByID(AreaField)
}

func AreaByID(id AreaID) AreaDefinition {
	for _, area := range areaDefinitions {
		if area.ID == id {
			return area
		}
	}
	return areaDefinitions[len(areaDefinitions)-1]
}

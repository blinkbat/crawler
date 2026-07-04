package core

import "testing"

func TestCloneAreaDeepCopiesAuthorableSlices(t *testing.T) {
	area := AreaDefinition{
		Path:         "maps/original.map",
		Name:         "Original",
		Width:        3,
		Height:       3,
		Walls:        []string{"...", ".#.", "..."},
		Floor:        []string{"ggg", "g.g", "ggg"},
		Decor:        []string{"...", ".b.", "..."},
		Props:        []string{"...", ".T.", "..."},
		Ceiling:      []string{"...", ".#.", "..."},
		Materials:    MaterialField,
		StartTileX:   1,
		StartTileZ:   1,
		StartFacing:  East,
		QuietMessage: "quiet",
		PackSpawns: []PackSpawn{
			{
				TileX: 2,
				TileZ: 1,
				Members: []PackMemberRef{
					BuiltinPackMember(EnemyRat),
					BuiltinPackMember(EnemyBat),
				},
			},
		},
		ChestSpawns: []ChestSpawn{
			{TileX: 0, TileZ: 2, Items: []ItemKind{ItemCheese, ItemBatJerky}},
		},
		DoorSpawns: []DoorSpawn{
			{TileX: 0, TileZ: 1, Name: "out", TargetMap: "field", TargetDoor: "in", Facing: West},
		},
	}

	clone := CloneArea(area)
	if !AreaContentEqual(area, clone) {
		t.Fatal("clone should start content-equal to source")
	}

	clone.Walls[0] = "###"
	clone.PackSpawns[0].Members[1].Kind = EnemyRat
	clone.ChestSpawns[0].Items[0] = ItemBatJerky
	clone.DoorSpawns[0].Name = "changed"

	if area.Walls[0] == clone.Walls[0] {
		t.Fatal("wall rows should be deep-copied")
	}
	if area.PackSpawns[0].Members[1].Kind == clone.PackSpawns[0].Members[1].Kind {
		t.Fatal("pack members should be deep-copied")
	}
	if area.ChestSpawns[0].Items[0] == clone.ChestSpawns[0].Items[0] {
		t.Fatal("chest items should be deep-copied")
	}
	if area.DoorSpawns[0].Name == clone.DoorSpawns[0].Name {
		t.Fatal("door spawns should be deep-copied")
	}
	if AreaContentEqual(area, clone) {
		t.Fatal("mutated clone should no longer match source content")
	}

	renamedPath := CloneArea(area)
	renamedPath.Path = "maps/copy.map"
	if !AreaContentEqual(area, renamedPath) {
		t.Fatal("path-only changes should not affect content equality")
	}
}

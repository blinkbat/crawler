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
					{Kind: EnemyBat, CustomName: "venom_bat"},
				},
			},
		},
		ChestSpawns: []ChestSpawn{
			{TileX: 0, TileZ: 2, Items: []ItemKind{ItemCheese, ItemBatJerky}},
		},
		DoorSpawns: []DoorSpawn{
			{TileX: 0, TileZ: 1, Name: "out", TargetMap: "field", TargetDoor: "in", Facing: West},
		},
		CustomEnemies: []CustomEnemyDef{
			{Name: "venom_bat", BaseKind: EnemyBat, HP: 12, MP: 3, Stats: Stats{SPD: 7}, Skills: []SkillID{SkillSleep}},
		},
	}

	clone := CloneArea(area)
	if !AreaContentEqual(area, clone) {
		t.Fatal("clone should start content-equal to source")
	}

	clone.Walls[0] = "###"
	clone.PackSpawns[0].Members[1].CustomName = "renamed"
	clone.ChestSpawns[0].Items[0] = ItemBatJerky
	clone.DoorSpawns[0].Name = "changed"
	clone.CustomEnemies[0].Skills[0] = SkillFirebolt

	if area.Walls[0] == clone.Walls[0] {
		t.Fatal("wall rows should be deep-copied")
	}
	if area.PackSpawns[0].Members[1].CustomName == clone.PackSpawns[0].Members[1].CustomName {
		t.Fatal("pack members should be deep-copied")
	}
	if area.ChestSpawns[0].Items[0] == clone.ChestSpawns[0].Items[0] {
		t.Fatal("chest items should be deep-copied")
	}
	if area.DoorSpawns[0].Name == clone.DoorSpawns[0].Name {
		t.Fatal("door spawns should be deep-copied")
	}
	if area.CustomEnemies[0].Skills[0] == clone.CustomEnemies[0].Skills[0] {
		t.Fatal("custom enemy skills should be deep-copied")
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

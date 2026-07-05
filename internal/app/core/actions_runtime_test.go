package core

import "testing"

func spawnTestArea() AreaDefinition {
	return AreaDefinition{
		Width: 5, Height: 5,
		Elevation: []string{"AAAAA", "AAAAA", "AAAAA", "AAAAA", "AAAAA"}, // all wall-height (level 10)
	}
}

func TestSpawnFoeAt(t *testing.T) {
	g := &GameState{Area: spawnTestArea()}
	g.Player.TileX, g.Player.TileZ = 0, 0
	kind := EnemyKinds()[0].Kind
	SpawnFoeAt(g, kind, 2, 3, 0)
	if len(g.Packs) != 1 || g.Packs[0].TileX != 2 || g.Packs[0].TileZ != 3 {
		t.Fatalf("SpawnFoeAt: want a pack at (2,3), got %+v", g.Packs)
	}
	if len(g.Packs[0].Members) != 1 || g.Packs[0].Members[0].Kind != kind {
		t.Fatalf("spawned pack has wrong members: %+v", g.Packs[0].Members)
	}
	// On the party's tile: refused.
	SpawnFoeAt(g, kind, 0, 0, 0)
	// Already-occupied tile: refused (still 1 pack).
	SpawnFoeAt(g, kind, 2, 3, 0)
	if len(g.Packs) != 1 {
		t.Fatalf("SpawnFoeAt should refuse the party tile and an occupied tile, got %d packs", len(g.Packs))
	}
}

func TestSpawnChestAt(t *testing.T) {
	g := &GameState{Area: spawnTestArea()}
	items := []ItemKind{AllItems()[0].Kind}
	SpawnChestAt(g, 1, 1, 0, items)
	if len(g.Chests) != 1 || g.Chests[0].TileX != 1 || g.Chests[0].Looted {
		t.Fatalf("SpawnChestAt: want a stocked chest at (1,1), got %+v", g.Chests)
	}
	// Duplicate on the same tile refused.
	SpawnChestAt(g, 1, 1, 0, items)
	if len(g.Chests) != 1 {
		t.Fatalf("SpawnChestAt should refuse an occupied tile, got %d chests", len(g.Chests))
	}
}

func TestOpenWallAtHeightfield(t *testing.T) {
	g := &GameState{Area: spawnTestArea()}
	g.Player.TileX, g.Player.TileZ = 0, 0
	// Party stands on level-10 ground; opening (1,0) should lower it to the party's level.
	g.Area.Elevation[0] = "A" + string(ElevationChar(ElevationWallRingLevel)) + "AAA" // raise (1,0) to a wall
	before := g.Area.ElevationLevelAt(1, 0)
	OpenWallAt(g, 1, 0, 0)
	after := g.Area.ElevationLevelAt(1, 0)
	if after == before || after != g.Area.ElevationLevelAt(0, 0) {
		t.Fatalf("OpenWallAt should lower (1,0) to the party's elevation %d, got %d (was %d)", g.Area.ElevationLevelAt(0, 0), after, before)
	}
}

func TestTeleportParty(t *testing.T) {
	g := &GameState{Area: spawnTestArea()}
	TeleportParty(g, 4, 2, 0)
	if g.Player.TileX != 4 || g.Player.TileZ != 2 {
		t.Fatalf("TeleportParty: want (4,2), got (%d,%d)", g.Player.TileX, g.Player.TileZ)
	}
}

func TestGiveGoldActionFloorsAtZero(t *testing.T) {
	g := &GameState{Gold: 10}
	runAction(g, Action{Kind: ActionGiveGold, Count: -50})
	if g.Gold != 0 {
		t.Fatalf("giveGold must floor at 0, got %d", g.Gold)
	}
	runAction(g, Action{Kind: ActionGiveGold, Count: 25})
	if g.Gold != 25 {
		t.Fatalf("giveGold +25 from 0 = 25, got %d", g.Gold)
	}
}

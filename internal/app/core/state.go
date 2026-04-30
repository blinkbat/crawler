package core

func NewPlayer(tileX, tileZ, facing int) Player {
	return Player{
		TileX:  tileX,
		TileZ:  tileZ,
		Facing: NormalizeFacing(facing),
		X:      TileCenter(tileX),
		Z:      TileCenter(tileZ),
		Yaw:    FacingYaw(facing),
	}
}

func NewGameState(m GameMap) GameState {
	return GameState{
		Map:     m,
		Player:  NewPlayer(StartTileX, StartTileZ, StartFacing),
		Party:   NewParty(),
		Enemies: placeRats(m, [][2]int{{6, 1}, {7, 1}, {9, 7}, {10, 7}, {3, 13}, {4, 13}, {5, 13}}),
		Battle: Battle{
			EnemyIndex:   -1,
			EnemyGroup:   nil,
			CurrentParty: 0,
			ActionMode:   ActionMenu,
			PendingSkill: SkillNone,
			PartyTarget:  0,
			Phase:        BattleNone,
			Message:      "The dungeon is quiet.",
		},
	}
}

func ResetGameState(g *GameState) {
	m := g.Map
	if len(m.Rows) == 0 {
		m = NewGameMap(DungeonLayout)
	}
	*g = NewGameState(m)
}

func NewParty() []PartyMember {
	return []PartyMember{
		{Name: "Warrior", HP: 28, MaxHP: 28, MP: 4, MaxMP: 4, Attack: 4},
		{Name: "Cleric", HP: 24, MaxHP: 24, MP: 18, MaxMP: 18, Attack: 5},
		{Name: "Thief", HP: 22, MaxHP: 22, MP: 8, MaxMP: 8, Attack: 3},
		{Name: "Wizard", HP: 26, MaxHP: 26, MP: 24, MaxMP: 24, Attack: 4},
	}
}

func NewRat(tileX, tileZ int) Enemy {
	return Enemy{TileX: tileX, TileZ: tileZ, HP: RatMaxHP, MaxHP: RatMaxHP, Alive: true, Name: "Feral Rat", MonsterType: "Beast", Item: "Morsel of Cheese"}
}

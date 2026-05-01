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
	g := GameState{
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
			Message:      "The field is quiet.",
		},
	}
	return g
}

func ResetGameState(g *GameState) {
	m := g.Map
	if len(m.Rows) == 0 {
		m = NewGameMap(FieldLayout)
	}
	*g = NewGameState(m)
}

func NewParty() []PartyMember {
	party := make([]PartyMember, 0, len(partyClassDefinitions))
	for _, def := range partyClassDefinitions {
		party = append(party, PartyMember{
			Class:  def.Class,
			Name:   def.Name,
			HP:     def.MaxHP,
			MaxHP:  def.MaxHP,
			MP:     def.MaxMP,
			MaxMP:  def.MaxMP,
			Attack: def.Atk,
		})
	}
	return party
}

func NewRat(tileX, tileZ int) Enemy {
	return NewEnemy(EnemyRat, tileX, tileZ)
}

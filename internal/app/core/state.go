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

func NewGameState(area AreaDefinition) GameState {
	m := NewGameMap(area.Layout, area.Materials)
	g := GameState{
		Map:     m,
		AreaID:  area.ID,
		Player:  NewPlayer(area.StartTileX, area.StartTileZ, area.StartFacing),
		Party:   NewParty(),
		Enemies: placeEnemies(m, area.EnemySpawns, area.StartTileX, area.StartTileZ),
		Battle: Battle{
			EnemyIndex:   -1,
			EnemyGroup:   nil,
			CurrentParty: 0,
			ActionMode:   ActionMenu,
			PendingSkill: SkillNone,
			PartyTarget:  0,
			Phase:        BattleNone,
			Message:      area.QuietMessage,
		},
	}
	return g
}

func ResetGameState(g *GameState) {
	*g = NewGameState(AreaByID(g.AreaID))
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

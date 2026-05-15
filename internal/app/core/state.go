package core

import (
	"math/rand"
	"time"
)

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
	g := GameState{
		Area:   area,
		Player: NewPlayer(area.StartTileX, area.StartTileZ, area.StartFacing),
		Party:  NewParty(),
		Packs:  placePacks(area),
		Battle: Battle{
			ActivePack:        -1,
			EnemyIndex:        -1,
			CurrentParty:      0,
			ActionMode:        ActionMenu,
			PendingSkill:      SkillNone,
			PartyTarget:       0,
			EnemyAttackCursor: -1,
			Phase:             BattleNone,
			Message:           area.QuietMessage,
		},
		// Wall-clock seed so consecutive playthroughs differ. A future
		// "Restart with seed N" command would assign a deterministic
		// Rand here instead.
		RNG: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	return g
}

// ResetGameState rebuilds the world for the same area — used on loss recovery
// (Press Enter after a wipe) and on the in-menu Restart action. Inventory is
// preserved across the reset so stolen loot survives a recoverable wipe;
// only the field/battle state is rewound. Use NewGameState for a full reset.
func ResetGameState(g *GameState) {
	saved := g.Inventory
	*g = NewGameState(g.Area)
	g.Inventory = saved
}

func NewParty() []PartyMember {
	party := make([]PartyMember, 0, len(partyClassDefinitions))
	for _, def := range partyClassDefinitions {
		maxHP := MaxHPFor(def.Stats)
		party = append(party, PartyMember{
			Class: def.Class,
			Name:  def.Name,
			Stats: def.Stats,
			HP:    maxHP,
			MaxHP: maxHP,
			MP:    def.MaxMP,
			MaxMP: def.MaxMP,
		})
	}
	return party
}

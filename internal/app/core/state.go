package core

import (
	"math/rand"
	"time"
)

// clampStartCoord snaps a start tile index to [0, dim-1]. dim must be > 0;
// callers (NewGameState) hand in area.Width / area.Height which the editor
// keeps positive, so the dim==0 fallback to 0 is a defensive corner that
// keeps later indexing in-bounds rather than asserting.
func clampStartCoord(v, dim int) int {
	if dim <= 0 {
		return 0
	}
	if v < 0 {
		return 0
	}
	if v >= dim {
		return dim - 1
	}
	return v
}

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
	// Defensive coord clamp. AreaFromMapFile already validates disk loads,
	// but the editor's F5 path can build a GameState directly from in-memory
	// edits — if a future editor bug leaves StartTileX/Z out of range, snap
	// to the nearest valid cell so downstream Walls[Z][X] reads don't panic.
	startX := clampStartCoord(area.StartTileX, area.Width)
	startZ := clampStartCoord(area.StartTileZ, area.Height)
	g := GameState{
		Area:      area,
		Player:    NewPlayer(startX, startZ, area.StartFacing),
		Party:     NewParty(),
		Packs:     placePacks(area),
		Chests:    placeChests(area),
		ChestOpen: -1,
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

// placeChests converts the area's authored chest list into runtime
// Chests. Items are folded into stacks at construction time so the
// modal can show "2x Cheese" instead of two separate rows for the same
// kind. Out-of-bounds and start-tile chests are dropped — would leave a
// chest the player could never approach.
func placeChests(a AreaDefinition) []Chest {
	if len(a.ChestSpawns) == 0 {
		return nil
	}
	out := make([]Chest, 0, len(a.ChestSpawns))
	for _, sp := range a.ChestSpawns {
		if !a.InBounds(sp.TileX, sp.TileZ) {
			continue
		}
		if sp.TileX == a.StartTileX && sp.TileZ == a.StartTileZ {
			continue
		}
		var stacks []ItemStack
		for _, kind := range sp.Items {
			stacks = AddItem(stacks, kind, 1)
		}
		out = append(out, Chest{
			TileX:  sp.TileX,
			TileZ:  sp.TileZ,
			Items:  stacks,
			Looted: len(stacks) == 0,
		})
	}
	return out
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
			// Every fresh PartyMember starts at BaseLevel with no XP
			// banked and no pending point-allocations. XPForLevel(1)
			// is the threshold to reach level 2.
			Level: BaseLevel,
		})
	}
	return party
}

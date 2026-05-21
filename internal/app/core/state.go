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
	visited := make([][]bool, area.Height)
	for z := range visited {
		visited[z] = make([]bool, area.Width)
	}
	if area.InBounds(startX, startZ) {
		visited[startZ][startX] = true
	}
	g := GameState{
		Area:      area,
		Player:    NewPlayer(startX, startZ, area.StartFacing),
		Party:     NewParty(),
		Packs:     placePacks(area),
		Chests:    placeChests(area),
		Doors:     placeDoors(area),
		Visited:   visited,
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

// placeDoors converts the area's authored door list into runtime
// Doors. Out-of-bounds doors are dropped defensively (the mapfile
// validator should have caught them on load) and doors with no name
// are skipped — runtime resolution by name would be ambiguous.
// Same-tile doors are allowed; the player only triggers the first
// match found, but authoring two doors on one tile is a pattern the
// editor should refuse (warned but not enforced here so the runtime
// stays robust to legacy maps).
func placeDoors(a AreaDefinition) []Door {
	if len(a.DoorSpawns) == 0 {
		return nil
	}
	out := make([]Door, 0, len(a.DoorSpawns))
	for _, sp := range a.DoorSpawns {
		if !a.InBounds(sp.TileX, sp.TileZ) || sp.Name == "" {
			continue
		}
		out = append(out, Door{
			TileX:      sp.TileX,
			TileZ:      sp.TileZ,
			Name:       sp.Name,
			TargetMap:  sp.TargetMap,
			TargetDoor: sp.TargetDoor,
			Facing:     sp.Facing,
		})
	}
	return out
}

// DoorIndexAt returns the index of the door at the given tile, or -1
// when no door is there. Mirrors ChestIndexAt. Used by the explore
// movement loop on every step-land to detect "stepped onto a door."
func DoorIndexAt(doors []Door, x, z int) int {
	for i, d := range doors {
		if d.TileX == x && d.TileZ == z {
			return i
		}
	}
	return -1
}

// DoorByName returns the door with the given name from the slice, or
// nil if not found. Used at transition resolution: the destination
// area's door list is searched for the named match so the player
// respawns at the right tile.
func DoorByName(doors []Door, name string) *Door {
	for i := range doors {
		if doors[i].Name == name {
			return &doors[i]
		}
	}
	return nil
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

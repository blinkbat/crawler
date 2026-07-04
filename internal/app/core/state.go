package core

import (
	"math/rand"
	"time"
)

// clampStartCoord snaps a start tile index to [0, dim-1]; dim<=0 falls back to
// 0 defensively so later indexing stays in-bounds.
func clampStartCoord(v, dim int) int {
	if dim <= 0 {
		return 0
	}
	return Clamp(v, 0, dim-1)
}

// ClampedStart returns the start tile snapped into [0,Width-1]×[0,Height-1].
func (a AreaDefinition) ClampedStart() (x, z int) {
	return clampStartCoord(a.StartTileX, a.Width), clampStartCoord(a.StartTileZ, a.Height)
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

// SnapPlayerToTile pins the player's visual X/Z to its tile center, ending any
// in-flight step interpolation. Player-side mirror of SnapPackToTile.
func SnapPlayerToTile(p *Player) {
	p.X = TileCenter(p.TileX)
	p.Z = TileCenter(p.TileZ)
}

// CarryProgressionFrom copies party-not-world run-state from prev onto g (party,
// bag, gold, quests, bestiary, step count, weather, RNG, run toggles). An area
// transition rebuilds a fresh GameState for the destination then calls this so
// these fields don't snap back to the new-state seed.
func (g *GameState) CarryProgressionFrom(prev *GameState) {
	g.Party = prev.Party
	copyRunProgression(g, prev)
	g.StepCount = prev.StepCount
	g.Weather = prev.Weather
	g.RNG = prev.RNG
	// ActionLog is one continuous in/out-of-combat buffer (party-run state, like
	// Quests) — carry it so a door step doesn't wipe the rolling log. StatusMessage
	// is transient and left to the new area's QuietMessage. NOT in copyRunProgression:
	// a Restart wants a fresh log, an area transition does not.
	g.ActionLog = prev.ActionLog
	copyRunPreferences(g, prev)
}

// copyRunProgression copies the party-not-world fields every "rebuild state, keep
// the run" path shares: bag, gold, quests, and foe knowledge. The caller copies
// Party itself (a transition keeps live HP; a Restart resets it). nil Bestiary
// stays nil. One home so a new run-progression field lands in both paths.
func copyRunProgression(dst, src *GameState) {
	dst.Inventory = src.Inventory
	dst.Gold = src.Gold
	dst.Quests = src.Quests
	// Foe knowledge travels with the party; without this each door step wipes
	// kill counts + Scanned flags.
	if src.Bestiary != nil {
		dst.Bestiary = src.Bestiary
	}
}

// copyRunPreferences copies the runtime preferences that travel with a run (not
// world state): presentation prefs (vibration, retro filters, battle tuning + wipe)
// and dev/debug toggles. One home shared by CarryProgressionFrom and ResetGameState
// so a new pref persists across BOTH an area transition and a Restart.
func copyRunPreferences(dst, src *GameState) {
	dst.RumbleEnabled = src.RumbleEnabled
	dst.RetroFilters = src.RetroFilters
	dst.RetroFilterSky = src.RetroFilterSky
	dst.RetroFilterSprites = src.RetroFilterSprites
	dst.BattleTuning = src.BattleTuning
	dst.BattleWipe = src.BattleWipe
	dst.DebugOverlay = src.DebugOverlay
	dst.EnemiesDisabled = src.EnemiesDisabled
	dst.EasyBattleQuit = src.EasyBattleQuit
	dst.RenderLogEnabled = src.RenderLogEnabled
	dst.DebugAllSkills = src.DebugAllSkills
	dst.DebugSkipBattles = src.DebugSkipBattles
}

func NewGameState(area AreaDefinition) GameState {
	// Defensive coord clamp: the editor's F5 path builds a GameState from
	// in-memory edits, so snap an out-of-range start so Walls[Z][X] can't panic.
	startX, startZ := area.ClampedStart()
	visited := make([][]bool, area.Height)
	for z := range visited {
		visited[z] = make([]bool, area.Width)
	}
	// Mark the start tile now; RevealRadius (needs a *GameState) reveals the
	// surrounding fog window after the struct is assembled.
	if area.InBounds(startX, startZ) {
		visited[startZ][startX] = true
	}
	packs := placePacks(&area)
	// Crystals block their tile, so drop any that coincide with a pack's (possibly
	// runtime-snapped) tile — two blockers on one cell would trap the pack and render
	// it embedded in the crystal.
	crystals := placeCrystals(area)
	crystals = dropCrystalsOnPacks(crystals, packs, area.IsVoxel())
	g := GameState{
		Area:       area,
		Player:     NewPlayer(startX, startZ, area.StartFacing),
		Party:      NewParty(),
		Packs:      packs,
		Chests:     placeChests(area),
		Doors:      placeDoors(area),
		Crystals:   crystals,
		Visited:    visited,
		ChestOpen:  NoIndex,
		DoorPrompt: NoIndex,
		// Runtime preference (not in SaveData); mutable in the Options menu.
		RumbleEnabled: true,
		// Runtime preferences (not in SaveData), preserved across Restart.
		RetroFilters:       DefaultRetroFilters(),
		RetroFilterSky:     DefaultRetroFilterSky,
		RetroFilterSprites: DefaultRetroFilterSprites,
		BattleTuning:       DefaultBattleTuning(),
		BattleWipe:         WipeNone,
		Inventory:          starterInventory(),
		Quests:             StarterQuests(),
		// Empty foe knowledge; fills via RecordBattleKills / MarkScanned. A save
		// overlays its persisted Bestiary in GameStateFromSave.
		Bestiary: make(Bestiary),
		Battle: Battle{
			ActivePack:        -1,
			EnemyIndex:        -1,
			CurrentParty:      0,
			ActionMode:        ActionMenu,
			PendingSkill:      SkillNone,
			PartyTarget:       0,
			EnemyAttackCursor: -1,
			Phase:             BattleNone,
		},
		// Ambient status line from the area's quiet message, until the first
		// action logs over it. Not appended to ActionLog (per-area flavor).
		StatusMessage: area.QuietMessage,
		// Wall-clock seed so consecutive playthroughs differ.
		RNG: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	// Standing level = spawn tile's lowest standable surface (ground), so a
	// player spawned under a bridge starts on the ground, not the deck.
	g.Player.Level = spawnLevel(&area, startX, startZ)
	// Seed region presence from the spawn tile so loading inside a region doesn't
	// fire its enter trigger — only a later crossing does (see location.go).
	SeedLocationPresence(&g)
	RevealRadius(&g, startX, startZ, SightRadius)
	return g
}

// CloseTransitionOverlays dismisses every transient explore overlay so none
// survives an area change. A cross-map transition rebuilds GameState fresh
// (NewGameState) and gets this for free; a same-map (in-place) teleport keeps the
// struct, so it must call this to match. ONE home for the "closed" set: add a new
// overlay gate here and both transition paths stay consistent.
func CloseTransitionOverlays(g *GameState) {
	g.MenuOpen = false
	g.OptionsMenuOpen = false
	g.SoundMenuOpen = false
	g.DebugMenuOpen = false
	g.RetroMenuOpen = false
	g.CombatTuneOpen = false
	g.WipeMenuOpen = false
	g.QuitConfirmOpen = false
	g.LevelUpOpen = false
	g.PanelsOpen = false
	g.SkillTreeOpen = false
	g.DialogOpen = false
	CloseEquipPicker(g)
	g.ChestOpen = NoIndex
	g.DoorPrompt = NoIndex
}

// spawnLevel picks the standing level for a unit at (x,z): the lowest standable
// surface, falling back to the column top so the value is never -1.
func spawnLevel(a *AreaDefinition, x, z int) int {
	if lo := a.LowestStandableLevel(x, z); lo >= 0 {
		return lo
	}
	return a.ElevationLevelAt(x, z)
}

// starterInventory returns the spawn inventory: rations only, no equipment
// (gear is earned through shops / chests / drops).
func starterInventory() []ItemStack {
	return []ItemStack{
		{Kind: ItemCrustOfBread, Count: 3},
		{Kind: ItemMagicPhial, Count: 2},
	}
}

// placeDoors converts the authored door list into runtime Doors. Out-of-bounds
// and nameless doors are dropped (name resolution would be ambiguous). Same-tile
// doors are allowed; only the first match triggers.
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
			Level:      a.resolveEntityLevel(sp.TileX, sp.TileZ, sp.Level),
			Name:       sp.Name,
			TargetMap:  sp.TargetMap,
			TargetDoor: sp.TargetDoor,
			Facing:     sp.Facing,
			Style:      sp.Style,
		})
	}
	return out
}

// DoorIndexAt returns the index of the door at (x,z), or -1. Mirrors ChestIndexAt.
func DoorIndexAt(doors []Door, x, z int) int {
	return SpawnIndexAt(doors, x, z)
}

// DoorIndexOn is the level-aware door lookup: on a voxel map only a door on
// `level` matches (so a door on another floor of the same column doesn't fire /
// block through the floor), else tile-only. Mirrors PackIndexAtLanding.
func DoorIndexOn(doors []Door, x, z, level int, isVoxel bool) int {
	if !isVoxel {
		return SpawnIndexAt(doors, x, z)
	}
	for i, d := range doors {
		if d.TileX == x && d.TileZ == z && d.Level == level {
			return i
		}
	}
	return -1
}

// doorIndexOn is DoorIndexOn's blocker-tail spelling (levelAware flag matches the
// chest/crystal helpers so canEnterRuntimeBlockersAt reads uniformly).
func doorIndexOn(doors []Door, x, z, level int, levelAware bool) int {
	return DoorIndexOn(doors, x, z, level, levelAware)
}

// DoorByName returns the named door, or nil. Used at transition resolution to
// find the destination door the player respawns at.
func DoorByName(doors []Door, name string) *Door {
	for i := range doors {
		if doors[i].Name == name {
			return &doors[i]
		}
	}
	return nil
}

// placeChests converts the authored chest list into runtime Chests. Items are
// folded into stacks. Out-of-bounds and start-tile chests are dropped.
func placeChests(a AreaDefinition) []Chest {
	if len(a.ChestSpawns) == 0 {
		return nil
	}
	// Compare against the CLAMPED start (the real spawn tile), not raw
	// StartTileX/Z, so a chest can't land on the spawn when the start was clamped.
	sx, sz := a.ClampedStart()
	out := make([]Chest, 0, len(a.ChestSpawns))
	for _, sp := range a.ChestSpawns {
		if !a.InBounds(sp.TileX, sp.TileZ) {
			continue
		}
		if sp.TileX == sx && sp.TileZ == sz {
			continue
		}
		var stacks []ItemStack
		for _, kind := range sp.Items {
			stacks = AddItem(stacks, kind, 1)
		}
		out = append(out, Chest{
			TileX:  sp.TileX,
			TileZ:  sp.TileZ,
			Level:  a.resolveEntityLevel(sp.TileX, sp.TileZ, sp.Level),
			Items:  stacks,
			Looted: len(stacks) == 0,
		})
	}
	return out
}

// placeCrystals drops the area's healing crystals, each seeded CHARGED. If
// CrystalsAuthored, use exactly a.CrystalSpawns (an empty list = deliberately
// zero crystals, no fallback); otherwise (legacy map) synthesize the default
// entrance crystal. Authored tiles are validated at load, so trusted here.
func placeCrystals(a AreaDefinition) []Crystal {
	spawns := a.CrystalSpawns
	if !a.CrystalsAuthored && len(spawns) == 0 {
		spawns = DefaultEntranceCrystalSpawns(a)
	}
	out := make([]Crystal, 0, len(spawns))
	for _, c := range spawns {
		out = append(out, Crystal{TileX: c.TileX, TileZ: c.TileZ, Level: a.resolveEntityLevel(c.TileX, c.TileZ, c.Level), Charge: CrystalRechargeSteps, Charged: true})
	}
	return out
}

// dropCrystalsOnPacks removes crystals sharing a tile with a pack (crystals block,
// so an overlap is two blockers on one cell). Level-aware on a voxel map: a crystal
// on a deck above (or ground below) a pack on a different floor of the same column
// doesn't actually collide, so only a same-floor pack drops it — mirroring the
// level-aware PackIndexAtLanding runtime blocker. Filters in place; order preserved.
func dropCrystalsOnPacks(crystals []Crystal, packs []Pack, isVoxel bool) []Crystal {
	kept := crystals[:0]
	for _, c := range crystals {
		if PackIndexAtLanding(packs, c.TileX, c.TileZ, c.Level, isVoxel) < 0 {
			kept = append(kept, c)
		}
	}
	return kept
}

// DefaultEntranceCrystalSpawns returns the auto-placed entrance crystal position
// (one-element slice, or nil when nowhere is clear). Crystals BLOCK their tile, so
// the entrance crystal must sit BESIDE the spawn, never on it: prefer a standable
// non-door cardinal neighbor, then any standable neighbor, else no crystal. Shared
// by placeCrystals (legacy fallback) and the editor.
func DefaultEntranceCrystalSpawns(a AreaDefinition) []CrystalSpawn {
	sx, sz := a.ClampedStart()
	isDoorTile := func(x, z int) bool {
		return DoorSpawnIndexAt(a.DoorSpawns, x, z) >= 0
	}
	// Cardinal offsets from FacingVector (canonical deltas). Scan ORDER is
	// deliberately N,S,W,E (not N/E/S/W) — it sets which neighbor wins when
	// several are clear, preserving the historical pick.
	step := func(facing int) [2]int {
		dx, dz := FacingVector(facing)
		return [2]int{dx, dz}
	}
	neighbors := [...][2]int{step(North), step(South), step(West), step(East)}
	for _, d := range neighbors {
		nx, nz := sx+d[0], sz+d[1]
		if a.InBounds(nx, nz) && !a.BlockedAt(nx, nz) && !isDoorTile(nx, nz) {
			return []CrystalSpawn{{TileX: nx, TileZ: nz}}
		}
	}
	// No clear non-door neighbor — accept a door-adjacent standable one rather than
	// the start tile (which a blocking crystal can't occupy); else skip entirely.
	for _, d := range neighbors {
		nx, nz := sx+d[0], sz+d[1]
		if a.InBounds(nx, nz) && !a.BlockedAt(nx, nz) {
			return []CrystalSpawn{{TileX: nx, TileZ: nz}}
		}
	}
	return nil
}

// CrystalIndexAt returns the index of the crystal on (x,z), or -1.
func CrystalIndexAt(crystals []Crystal, x, z int) int {
	return SpawnIndexAt(crystals, x, z)
}

// crystalIndexOn is the level-aware crystal lookup for the blocker tail: when
// levelAware, only a crystal on `level` blocks; else tile-only.
func crystalIndexOn(crystals []Crystal, x, z, level int, levelAware bool) int {
	for i, c := range crystals {
		if c.TileX == x && c.TileZ == z && (!levelAware || c.Level == level) {
			return i
		}
	}
	return -1
}

// AdjacentChargedCrystalIndex returns a CHARGED crystal within Manhattan
// distance 1 of (x,z) (on or beside the player), or -1. Dormant ones ignored.
func AdjacentChargedCrystalIndex(crystals []Crystal, x, z int) int {
	return adjacentChargedCrystalIndex(crystals, x, z, 0, false)
}

// AdjacentChargedCrystalIndexOn is the level-aware variant: on a voxel map a
// crystal only rests when it shares the player's floor (`level`), so one on
// another deck at the same/adjacent (x,z) isn't triggered through the floor.
func AdjacentChargedCrystalIndexOn(crystals []Crystal, x, z, level int, isVoxel bool) int {
	return adjacentChargedCrystalIndex(crystals, x, z, level, isVoxel)
}

func adjacentChargedCrystalIndex(crystals []Crystal, x, z, level int, levelAware bool) int {
	for i := range crystals {
		c := crystals[i]
		if !c.Charged {
			continue
		}
		if levelAware && c.Level != level {
			continue
		}
		if ManhattanDistance(c.TileX, c.TileZ, x, z) <= 1 {
			return i
		}
	}
	return -1
}

// ResetGameState rebuilds the world for the same area (loss recovery / in-menu
// Restart). Inventory and party progression are preserved; battle statuses and
// HP/MP reset so recovery can't strand a defeated party. NewGameState is the
// full reset.
func ResetGameState(g *GameState) {
	// Snapshot run-carried state before NewGameState reseeds it. Party is reset for
	// field recovery (HP/MP full); the rest of the run carries through unchanged.
	prev := *g
	savedParty := resetPartyForFieldRecovery(g.Party)
	*g = NewGameState(g.Area)
	g.Party = savedParty
	copyRunProgression(g, &prev) // bag, gold, quests, bestiary — survive a restart
	copyRunPreferences(g, &prev) // presentation prefs + debug toggles
	// Drop lingering particles: Restart can fire mid-battle, so formation-relative
	// battle particles would otherwise ghost into the fresh field.
	RequestVFXReset(g)
}

func resetPartyForFieldRecovery(party []PartyMember) []PartyMember {
	out := make([]PartyMember, len(party))
	copy(out, party)
	clearPartyCombatTransients(out)
	for i := range out {
		out[i].HP = out[i].MaxHP
		out[i].MP = out[i].MaxMP
		// Full restore clears Poison too (the shared clearer preserves it).
		out[i].PoisonTurns = 0
	}
	return out
}

func NewParty() []PartyMember {
	party := make([]PartyMember, 0, len(partyClassDefinitions))
	// Pack columns within each row into a 2×2: first member left, second right.
	frontCount, backCount := 0, 0
	for _, def := range partyClassDefinitions {
		maxHP := MaxHPFor(def.Stats)
		row := DefaultPartyRow(def.Class)
		col := defaultSlotCol(row, &frontCount, &backCount)
		party = append(party, PartyMember{
			Class: def.Class,
			Name:  def.Name,
			Stats: def.Stats,
			HP:    maxHP,
			MaxHP: maxHP,
			MP:    def.MaxMP,
			MaxMP: def.MaxMP,
			// Row is the live row, seeded to the home row; battle start rotates it.
			Row:     row,
			HomeRow: row,
			HomeCol: col,
			Level:   BaseLevel,
			// One starting SkillPoint, no skills learned: the player spends this
			// first point in the Tome to choose their opening skill.
			SkillPoints: 1,
		})
	}
	return party
}

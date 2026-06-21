package core

import (
	"math/rand"
	"slices"
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
	return Clamp(v, 0, dim-1)
}

// ClampedStart returns the area's start tile snapped into [0,Width-1]×[0,Height-1]
// — the single home for the "derive the spawn tile from StartTileX/Z" clamp that
// NewGameState, placeChests, and DefaultEntranceCrystalSpawns all share.
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

// SnapPlayerToTile pins the player's visual X/Z to the center of its current
// tile, ending any in-flight step interpolation. The player-side mirror of
// SnapPackToTile — used wherever the player is placed without an animated step
// (the engage snap, the step-anim finish, the flee retreat).
func SnapPlayerToTile(p *Player) {
	p.X = TileCenter(p.TileX)
	p.Z = TileCenter(p.TileZ)
}

// CarryProgressionFrom copies the run-state that belongs to the PARTY, not the
// world, from prev onto g: party + bag + gold + quest journal + bestiary +
// step count + weather + RNG + the debug/render runtime toggles. An area
// transition rebuilds
// a fresh GameState for the destination map (so packs/chests/fog reset like a
// save-point) and then calls this so those carried fields don't snap back to
// the new-state seed (0 gold, starter quests, cleared weather, …). Lives here,
// beside the GameState struct, so adding a new "travels with the party" field
// is a one-line edit next to the field instead of a remembered copy buried in
// the door-transition call site.
func (g *GameState) CarryProgressionFrom(prev *GameState) {
	g.Party = prev.Party
	g.Inventory = prev.Inventory
	g.Gold = prev.Gold
	g.Quests = prev.Quests
	// Foe knowledge travels with the party like the quest journal — without
	// this, every door step wipes kill counts + Scanned flags (NewGameState
	// seeded a fresh empty map). nil stays nil; mirrors ResetGameState's carry.
	if prev.Bestiary != nil {
		g.Bestiary = prev.Bestiary
	}
	g.StepCount = prev.StepCount
	g.Weather = prev.Weather
	g.RNG = prev.RNG
	copyRunToggles(g, prev)
}

// copyRunToggles copies the runtime dev/debug preferences that travel with a run
// (not world state) from src onto dst. Shared by CarryProgressionFrom (area
// transitions) and ResetGameState (restart / loss recovery) so the toggle list
// has ONE home: a new Debug-submenu toggle added here persists across both paths
// instead of silently resetting on whichever call site forgot to list it.
func copyRunToggles(dst, src *GameState) {
	dst.DebugOverlay = src.DebugOverlay
	dst.EnemiesDisabled = src.EnemiesDisabled
	dst.EasyBattleQuit = src.EasyBattleQuit
	dst.RenderLogEnabled = src.RenderLogEnabled
	dst.DebugAllSkills = src.DebugAllSkills
	dst.DebugSkipBattles = src.DebugSkipBattles
}

func NewGameState(area AreaDefinition) GameState {
	// Defensive coord clamp. AreaFromMapFile already validates disk loads,
	// but the editor's F5 path can build a GameState directly from in-memory
	// edits — if a future editor bug leaves StartTileX/Z out of range, snap
	// to the nearest valid cell so downstream Walls[Z][X] reads don't panic.
	startX, startZ := area.ClampedStart()
	visited := make([][]bool, area.Height)
	for z := range visited {
		visited[z] = make([]bool, area.Width)
	}
	// Seed the start tile's 3×3 fog-of-war window so the player
	// doesn't spawn standing in a single-tile pinhole — the map
	// should reflect what they'd "see" from their starting position.
	// RevealRadius needs a *GameState so we build the grid first,
	// reveal after the struct's assembled.
	if area.InBounds(startX, startZ) {
		visited[startZ][startX] = true
	}
	g := GameState{
		Area:       area,
		Player:     NewPlayer(startX, startZ, area.StartFacing),
		Party:      NewParty(),
		Packs:      placePacks(&area),
		Chests:     placeChests(area),
		Doors:      placeDoors(area),
		Crystals:   placeCrystals(area),
		Visited:    visited,
		ChestOpen:  -1,
		DoorPrompt: -1,
		// Controller vibration on by default; the player can mute it in the
		// Options menu. Runtime preference (not persisted in SaveData).
		RumbleEnabled: true,
		// Out-of-the-box retro post-process mix (Debug ▸ Retro Filters) —
		// same preference class as RumbleEnabled: runtime, not in SaveData,
		// preserved across Restart. Sky stays crisp by default.
		RetroFilters:       DefaultRetroFilters(),
		RetroFilterSky:     DefaultRetroFilterSky,
		RetroFilterSprites: DefaultRetroFilterSprites,
		// Starting bag: rations only, no equipment — see starterInventory.
		Inventory: starterInventory(),
		Quests:    StarterQuests(),
		// Empty foe knowledge at the start of a run; fills as the party
		// fights (RecordBattleKills) and scans (MarkScanned). A save
		// overlays its persisted Bestiary on top in GameStateFromSave.
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
		// Seed the ambient status line from the area's quiet message (shown under
		// the HUD until the first real action logs over it). Not appended to the
		// ActionLog — it's per-area flavor, not an action.
		StatusMessage: area.QuietMessage,
		// Wall-clock seed so consecutive playthroughs differ. A future
		// "Restart with seed N" command would assign a deterministic
		// Rand here instead.
		RNG: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	// Seed the party's standing level to the spawn tile's lowest standable
	// surface (the ground). On a heightfield that's the column top, matching the
	// pre-voxel single-surface assumption; on a voxel map a player spawned under
	// a bridge starts on the ground, not the deck.
	g.Player.Level = spawnLevel(&area, startX, startZ)
	RevealRadius(&g, startX, startZ, SightRadius)
	return g
}

// spawnLevel picks the standing level a unit placed at (x,z) should occupy: the
// lowest standable surface (the ground), falling back to the column top when no
// surface is standable (a fully sealed / degenerate column) so the value is
// always a sane level rather than -1.
func spawnLevel(a *AreaDefinition, x, z int) int {
	if lo := a.LowestStandableLevel(x, z); lo >= 0 {
		return lo
	}
	return a.ElevationLevelAt(x, z)
}

// starterInventory returns the inventory the party spawns with. The
// party starts with NO equipment — every member's Equipped slots are
// empty (NewParty leaves them zero) and the bag carries no gear, just a
// few rations: 3 crusts of bread (a small HP heal) and 2 magic phials (a
// small MP restore, so the casters aren't dry on the first fight).
// Equipment is earned through shops / chests / drops, not at creation.
func starterInventory() []ItemStack {
	return []ItemStack{
		{Kind: ItemCrustOfBread, Count: 3},
		{Kind: ItemMagicPhial, Count: 2},
	}
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
			Style:      sp.Style,
		})
	}
	return out
}

// DoorIndexAt returns the index of the door at the given tile, or -1
// when no door is there. Mirrors ChestIndexAt. Used by the explore
// movement loop on every step-land to detect "stepped onto a door."
func DoorIndexAt(doors []Door, x, z int) int {
	return slices.IndexFunc(doors, func(d Door) bool { return d.TileX == x && d.TileZ == z })
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
	// Compare against the CLAMPED start (the tile the player actually spawns on —
	// NewGameState snaps an out-of-range authored start), not the raw StartTileX/Z,
	// so a chest can't end up on the real spawn when the authored start was clamped.
	// Same clamp placeCrystals / DefaultEntranceCrystalSpawns use.
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
			Items:  stacks,
			Looted: len(stacks) == 0,
		})
	}
	return out
}

// placeCrystals drops the area's healing crystals, each seeded CHARGED so the
// player can bank a save+heal on contact. The source of positions depends on
// whether the map authored its crystals:
//   - CrystalsAuthored (the map carried an explicit crystals: section, which
//     the editor always writes) ⇒ use exactly a.CrystalSpawns. An authored but
//     EMPTY list means the author deliberately wants zero crystals, and is
//     honored as such — no fallback.
//   - otherwise (a legacy map predating editable crystals) ⇒ synthesize the
//     default entrance crystal via DefaultEntranceCrystalSpawns so those maps
//     keep their Grimrock-style save point unchanged.
//
// Charge state persists per-tile via SaveData. Authored crystal tiles are
// validated standable + duplicate-free at load (AreaFromMapFile) and guarded by
// the editor's placement rules, so this conversion can trust them directly.
func placeCrystals(a AreaDefinition) []Crystal {
	spawns := a.CrystalSpawns
	if !a.CrystalsAuthored && len(spawns) == 0 {
		spawns = DefaultEntranceCrystalSpawns(a)
	}
	out := make([]Crystal, 0, len(spawns))
	for _, c := range spawns {
		out = append(out, Crystal{TileX: c.TileX, TileZ: c.TileZ, Charge: CrystalRechargeSteps, Charged: true})
	}
	return out
}

// DefaultEntranceCrystalSpawns returns the auto-placed entrance crystal's
// position (a one-element slice, or nil when there's nowhere clear) — a
// Grimrock-style save point next to the player start. It scans the four
// cardinal neighbors for a standable, non-door tile and lands on the start tile
// itself only if no neighbor is clear; if the start IS a door it prefers any
// standable neighbor over planting a crystal on the door, giving up (nil) only
// when nothing is clear. Shared by placeCrystals (the legacy-map fallback) and
// the editor, which seeds this as a real, editable CrystalSpawn when an
// unauthored map is opened so the entrance crystal can be moved or removed.
func DefaultEntranceCrystalSpawns(a AreaDefinition) []CrystalSpawn {
	sx, sz := a.ClampedStart()
	isDoorTile := func(x, z int) bool {
		return DoorSpawnIndexAt(a.DoorSpawns, x, z) >= 0
	}
	// Cardinal neighbour offsets, derived from FacingVector so the step
	// deltas can't drift from the canonical facing convention (the same
	// source cardinalStepsBase in packai.go builds from). The scan ORDER is
	// deliberately North, South, West, East — NOT FacingVector's N/E/S/W —
	// because it determines which neighbour an entrance crystal prefers when
	// several are clear, and this order preserves the historical pick. Build
	// from FacingVector per direction (rather than reusing the N/E/S/W-ordered
	// cardinalStepsBase) so the deltas stay canonical without disturbing that
	// long-standing preference order.
	step := func(facing int) [2]int {
		dx, dz := FacingVector(facing)
		return [2]int{dx, dz}
	}
	neighbors := [...][2]int{step(North), step(South), step(West), step(East)}
	cx, cz, found := sx, sz, false
	for _, d := range neighbors {
		nx, nz := sx+d[0], sz+d[1]
		if a.InBounds(nx, nz) && !a.BlockedAt(nx, nz) && !isDoorTile(nx, nz) {
			cx, cz, found = nx, nz, true
			break
		}
	}
	// Fallback is the start tile itself — fine UNLESS the start IS a door (a
	// crystal billboard on a door looks wrong and the door transition could fire
	// before the player uses it). In that rare case prefer any standable neighbor
	// (even an unrelated one) over the door; if there's genuinely nowhere clear,
	// skip the entrance crystal rather than plant it on the door.
	if !found && isDoorTile(sx, sz) {
		for _, d := range neighbors {
			nx, nz := sx+d[0], sz+d[1]
			if a.InBounds(nx, nz) && !a.BlockedAt(nx, nz) {
				return []CrystalSpawn{{TileX: nx, TileZ: nz}}
			}
		}
		return nil
	}
	return []CrystalSpawn{{TileX: cx, TileZ: cz}}
}

// CrystalIndexAt returns the index of the crystal exactly on the tile, or -1.
// Mirrors ChestIndexAt / DoorIndexAt.
func CrystalIndexAt(crystals []Crystal, x, z int) int {
	return slices.IndexFunc(crystals, func(c Crystal) bool { return c.TileX == x && c.TileZ == z })
}

// AdjacentChargedCrystalIndex returns the index of a CHARGED crystal the player
// at (x,z) can use — on their tile (distance 0) or a cardinal neighbor (distance
// 1), so a Grimrock crystal triggers whether you step onto it or up beside it.
// Returns -1 when no charged crystal is in reach. Dormant crystals are ignored.
func AdjacentChargedCrystalIndex(crystals []Crystal, x, z int) int {
	for i := range crystals {
		c := crystals[i]
		if !c.Charged {
			continue
		}
		if ManhattanDistance(c.TileX, c.TileZ, x, z) <= 1 {
			return i
		}
	}
	return -1
}

// ResetGameState rebuilds the world for the same area — used on loss recovery
// (Press Enter after a wipe) and on the in-menu Restart action. Inventory and
// party progression are preserved, while battle statuses and HP/MP are reset
// so recovery cannot strand the player with an already-defeated party. Use
// NewGameState for a full reset.
func ResetGameState(g *GameState) {
	// Snapshot the run-carried debug/render toggles before NewGameState reseeds
	// them, so copyRunToggles can restore them below (same set, one home — see
	// CarryProgressionFrom). The struct copy is cheap; only the toggles are read.
	prev := *g
	savedInventory := g.Inventory
	savedParty := resetPartyForFieldRecovery(g.Party)
	savedGold := g.Gold
	savedQuests := g.Quests
	savedBestiary := g.Bestiary
	savedRumble := g.RumbleEnabled
	savedRetroFilters := g.RetroFilters
	savedRetroSky := g.RetroFilterSky
	savedRetroSprites := g.RetroFilterSprites
	*g = NewGameState(g.Area)
	g.Inventory = savedInventory
	g.Party = savedParty
	// Gold + the quest journal + the bestiary are run progression, not world
	// state — they survive a loss-recovery / restart the same way inventory and
	// party levels do. Without restoring the bestiary, a wipe-and-recover or
	// pause-menu Restart would silently wipe every kill count + Scanned flag,
	// even though those are persisted to disk and carried across area
	// transitions. NewGameState already seeded a fresh map; only overwrite it
	// when the prior run actually had one (nil stays nil — harmless).
	g.Gold = savedGold
	g.Quests = savedQuests
	if savedBestiary != nil {
		g.Bestiary = savedBestiary
	}
	// Preserve the player's vibration preference across a restart (an
	// accessibility setting, not world state) — and the retro-filter
	// intensities, the same class of presentation preference.
	g.RumbleEnabled = savedRumble
	g.RetroFilters = savedRetroFilters
	g.RetroFilterSky = savedRetroSky
	g.RetroFilterSprites = savedRetroSprites
	// Debug toggles are runtime dev preferences, not world state — preserve them
	// across a restart/loss-recovery the same way area transitions carry them.
	copyRunToggles(g, &prev)
	// Signal the render layer to drop any lingering particles. Restart can
	// fire mid-battle (the pause menu's Restart row is reachable outside the
	// two timing phases), so formation-relative battle particles would
	// otherwise ghost into the fresh field at wrong camera-relative spots —
	// the same reason clearBattleResidual / area transitions request it.
	RequestVFXReset(g)
}

func resetPartyForFieldRecovery(party []PartyMember) []PartyMember {
	out := make([]PartyMember, len(party))
	copy(out, party)
	// Delegate the "what's a combat-transient" list (statuses, ingestion, anim
	// timers) to the shared clearer rather than re-listing it.
	clearPartyCombatTransients(out)
	for i := range out {
		out[i].HP = out[i].MaxHP
		out[i].MP = out[i].MaxMP
		// Field recovery is a FULL restore, so it also clears Poison — which
		// the shared clearer deliberately preserves, hence the explicit line
		// here on top of it.
		out[i].PoisonTurns = 0
	}
	return out
}

func NewParty() []PartyMember {
	party := make([]PartyMember, 0, len(partyClassDefinitions))
	// Pack columns within each row so the default reads as a proper 2×2: the
	// first member of a row takes the left column, the second the right.
	frontCount, backCount := 0, 0
	for _, def := range partyClassDefinitions {
		maxHP := MaxHPFor(def.Stats)
		row := DefaultPartyRow(def.Class)
		col := ColLeft
		if row == RowFront {
			if frontCount%2 == 1 {
				col = ColRight
			}
			frontCount++
		} else {
			if backCount%2 == 1 {
				col = ColRight
			}
			backCount++
		}
		party = append(party, PartyMember{
			Class: def.Class,
			Name:  def.Name,
			Stats: def.Stats,
			HP:    maxHP,
			MaxHP: maxHP,
			MP:    def.MaxMP,
			MaxMP: def.MaxMP,
			// Default formation: front-line classes up front, casters in back.
			// Row is the live row (seeded to the home row; battle start rotates it).
			Row:     row,
			HomeRow: row,
			HomeCol: col,
			// Every fresh PartyMember starts at BaseLevel with no XP
			// banked and no pending point-allocations. XPForLevel(1)
			// is the threshold to reach level 2.
			Level: BaseLevel,
			// One starting SkillPoint, no skills learned: every member
			// begins UNLEARNED and the player spends this first point in
			// the Tome to choose their opening skill (true first choice).
			// LearnedSkills is empty until that purchase, so the battle
			// Skill menu reads "(no skills)" until they invest.
			SkillPoints: 1,
		})
	}
	return party
}

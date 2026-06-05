package core

import (
	"crawler/internal/app/core/mapfile"
	"math/rand"
)

type MaterialSet int

// PackMemberRef is one authored pack member. Built-ins carry only Kind;
// custom enemies carry their base Kind for visual fallback plus CustomName
// for lookup in AreaDefinition.CustomEnemies.
type PackMemberRef struct {
	Kind       EnemyKind
	CustomName string
}

// PackSpawn is one authored pack on the map: a tile position and the roster
// of enemy references that make up the pack. Single-member packs are the
// common case and read back from the on-disk format the same as the old
// single-enemy spawn line. The field renders one figure per pack (the
// highest-tier member); the rest are revealed when the battle starts.
type PackSpawn struct {
	TileX   int
	TileZ   int
	Members []PackMemberRef
	// AI selects the per-pack movement behavior. Zero value PackAINone
	// is the default — the pack sits at its spawn tile until the player
	// walks into it, the behavior every pack used to have implicitly.
	// The author opts a pack into an active mode via the pack-edit
	// modal in the editor.
	AI PackAI
}

// PackAI is the movement style applied to one pack. Authored on the
// PackSpawn so different packs on the same map can use different modes.
type PackAI int

const (
	// PackAINone is the default — the pack is stationary, waiting for
	// the player to step into it. No per-step planning runs.
	PackAINone PackAI = iota
	// PackAIJunkyardDog is the wander-near-spawn / chase-when-close
	// behavior described in packai.go. Was the only mode in the
	// codebase before per-pack AI landed; now opt-in per pack.
	PackAIJunkyardDog
	// PackAIPatrol paces a fixed line along the X axis around the spawn
	// tile (out to PatrolRadius, bouncing at the ends and at walls),
	// ignoring the player until it paces into their tile — a sentry on a
	// beat, not a hunter. Tracks its current pace direction in
	// Pack.PatrolDir.
	PackAIPatrol
	// PackAISkittish flees: when the player is within SkittishFleeRadius
	// it steps directly away (never onto the player, so it can't engage);
	// otherwise it wanders its leash like the junkyard dog's idle branch.
	// Prey that runs rather than fights.
	PackAISkittish
	// PackAICount sizes name / label tables. Bump by adding a mode
	// above this line; init guards (areas.go's packAINameTable,
	// editor's modal row) catch the missing wiring at startup.
	PackAICount = int(PackAISkittish) + 1
)

// ChestSpawn is one authored chest on the map: a tile position and the
// loot the chest holds. The runtime form (Chest) carries an additional
// Looted flag so re-opening an already-emptied chest is a cheap no-op
// instead of regenerating the items.
type ChestSpawn struct {
	TileX int
	TileZ int
	Items []ItemKind
}

// DoorSpawn is one authored door on the map: a tile position, the
// door's identifier (unique within this map), the destination map id
// + door name to step into, and the post-transition facing the player
// should adopt. Same shape as the on-disk MapDoor — mapfile->core
// resolves SelfMapToken to the local map name at load time so the
// runtime never sees the placeholder.
// DoorStyle picks the visual fixture a door renders as. The destination /
// transition behavior is identical across styles — this is purely how the
// doorway looks so an author can match a door to its surroundings (a
// timber frame in a town, a stone arch in a cave, a freestanding gateway
// in the open field).
type DoorStyle int

const (
	DoorStyleBuilding DoorStyle = iota // timber-framed door (the original)
	DoorStyleCave                      // rough stone archway
	DoorStyleField                     // open gateway / trail arch
	// DoorStyleCount is the wrap modulus for cycling styles in the editor
	// and the size of the per-style model table in the renderer.
	DoorStyleCount
)

type DoorSpawn struct {
	TileX      int
	TileZ      int
	Name       string
	TargetMap  string
	TargetDoor string
	Facing     int
	Style      DoorStyle
}

// HasTarget reports whether this door names a destination it can
// actually resolve. An empty TargetMap or TargetDoor means the door
// was authored but never finished — every "should this door fire?"
// predicate (runtime trigger, editor validator, parse-time check)
// goes through here so the rule lives in one place.
func (d DoorSpawn) HasTarget() bool {
	return mapfile.DoorTargetComplete(d.TargetMap, d.TargetDoor)
}

// TileXZ is implemented by the authored spawn types (PackSpawn /
// ChestSpawn / DoorSpawn), all of which carry a TileX/TileZ position.
// Generic "find / remove the spawn on this tile" helpers in core and the
// editor range over it instead of re-typing the coordinate read per
// spawn slice.
type TileXZ interface {
	Tile() (int, int)
}

func (s PackSpawn) Tile() (int, int)  { return s.TileX, s.TileZ }
func (s ChestSpawn) Tile() (int, int) { return s.TileX, s.TileZ }
func (s DoorSpawn) Tile() (int, int)  { return s.TileX, s.TileZ }

// Door is one runtime door on the field. Built from AreaDefinition.
// DoorSpawns by NewGameState (placeDoors). Doors block neither movement
// nor vision: stepping onto a door's tile fires the area transition,
// resolved by the explore loop against the door's TargetMap + TargetDoor.
type Door struct {
	TileX int
	TileZ int
	Name  string
	// TargetMap names the destination map id (the bare name, e.g.
	// "dungeon" for dungeon.map). The literal "self" placeholder is
	// resolved to the local map id by AreaFromMapFile.
	TargetMap string
	// TargetDoor is the destination door's Name. Resolution lookup
	// is DoorByName(destination.Doors, TargetDoor).
	TargetDoor string
	// Facing has two related uses, both honored by doorExitTile in
	// run.go:
	//   1. The player's post-transition look direction — the camera
	//      yaw is set from this when they emerge.
	//   2. The offset vector for the exit tile: the player is placed
	//      one step in this direction from the door tile, so the
	//      destination door sits behind them and they don't
	//      immediately re-trigger the transition.
	// Both readings collapse to the same value: "which way is the
	// player facing when they walk out of this doorway." Authors
	// should set it accordingly; the engine doesn't infer either
	// reading separately.
	Facing int
	// Style is the visual fixture (timber / cave / field). Render-only;
	// the transition behavior is identical across styles.
	Style DoorStyle
}

// HasTarget reports whether this runtime door names a resolvable
// destination. Sibling of DoorSpawn.HasTarget — same rule applied to
// the runtime list.
func (d Door) HasTarget() bool {
	return mapfile.DoorTargetComplete(d.TargetMap, d.TargetDoor)
}

// AreaTransition is the queued "swap to this area next frame" request
// the explore movement loop hands off to the run loop when the player
// steps onto a door. Empty TargetMap is the zero / "no transition"
// marker — the run loop only swaps when TargetMap != "".
type AreaTransition struct {
	TargetMap  string
	TargetDoor string
}

// PanelTab indexes the game-panels overlay tabs. Order is the on-screen
// left-to-right tab order; switching tabs cycles through this enum with
// the L1/R1 shoulders or arrow keys. Adding a new tab is one enum row
// + one row in render/panels.go's drawer table.
type PanelTab int

const (
	PanelTabStats PanelTab = iota
	PanelTabEquipment
	PanelTabItems
	PanelTabSkills
	PanelTabQuests
	PanelTabMap
	PanelTabCount
)

// PanelTabCharacter is the new spelling of the former PanelTabStats —
// the tab covers level, XP, stats, armor, skill points, and the
// allocate-level-up entry point, so "Character" reads truer than
// "Stats." Aliased for backwards compat with any save / replay
// state that already had the old name.
const PanelTabCharacter = PanelTabStats

// PanelTabLabel returns the short human label for a tab — used for the
// tab strip header in the overlay.
func PanelTabLabel(t PanelTab) string {
	switch t {
	case PanelTabStats:
		return "Character"
	case PanelTabEquipment:
		return "Equipment"
	case PanelTabItems:
		return "Items"
	case PanelTabSkills:
		return "Skills"
	case PanelTabQuests:
		return "Quests"
	case PanelTabMap:
		return "Map"
	default:
		// Parallel to the PanelTabCount-locked render.panelTabDrawers
		// array: a new tab that forgets a label here fails loudly instead
		// of showing "?" in the tab strip.
		panic("core: PanelTabLabel missing case for PanelTab")
	}
}

// Chest is one runtime chest on the field. Items is the stack-counted
// loot the player can withdraw via the chest-open modal; Looted goes
// true once every stack is drained (or Take All fires), at which point
// the chest renders open and ignores further interactions. Chests block
// movement onto their tile — the player walks up to an adjacent square
// and opens with the Confirm key.
type Chest struct {
	TileX  int
	TileZ  int
	Items  []ItemStack
	Looted bool
}

// AreaDefinition is the runtime form of a map. Built from a mapfile.MapFile
// via AreaFromMapFile (see areas.go). Path is the source disk location and
// is empty for unsaved maps the editor is still working on.
//
// Geometry is layered: Walls / Floor / Decor / Props are four parallel
// ASCII grids of the same dimensions. Width and Height are stored
// explicitly so blank layers can be reconstructed without inferring
// from any single grid (an empty .map gets all four blank).
type AreaDefinition struct {
	Path   string
	Name   string
	Width  int
	Height int
	Walls  []string
	Floor  []string
	Decor  []string
	Props  []string
	// Ceiling is the fifth geometry layer: same dimensions as Walls, but
	// only TileCeilingSolid cells get an overhead slab rendered. Empty
	// rows here are normalized into a blank layer at load time so older
	// .map files (pre-ceiling) keep working without a re-save.
	Ceiling []string
	// Elevation is the sixth geometry layer: per-tile ground LEVEL ('0'..'9'),
	// same dimensions as Walls. A ramp floor tile stores its LOW level here.
	// Older .map files without an elevation: section load as an all-'0' (flat)
	// layer, so ElevationLevelAt reads 0 everywhere for them.
	Elevation   []string
	Materials   MaterialSet
	StartTileX  int
	StartTileZ  int
	StartFacing int
	PackSpawns  []PackSpawn
	// ChestSpawns is the authored chest list. Converted to runtime
	// Chests in NewGameState; the field-render and interact paths read
	// the runtime list, not this one.
	ChestSpawns []ChestSpawn
	// DoorSpawns is the authored door list. Converted to runtime
	// g.Doors in NewGameState; the explore movement loop reads the
	// runtime list to detect "stepped onto a door tile" transitions.
	DoorSpawns []DoorSpawn
	// CustomEnemies are author-defined enemy templates scoped to this
	// area. The editor's modalCustomEnemies CRUDs them; pack spawns
	// reference them by Name; battle instantiates an Enemy via
	// CustomEnemyDef.Instantiate. Empty for built-in-only maps.
	CustomEnemies []CustomEnemyDef
	QuietMessage  string
}

type Player struct {
	TileX     int
	TileZ     int
	Facing    int
	X         float32
	Z         float32
	Yaw       float32
	LookYaw   float32
	LookPitch float32
	// GroundY is the world-space height of the ground under the player,
	// eased during a step animation (see updateAnimation). Only meaningful
	// while Anim.Kind == AnimStep; at rest the camera derives the ground
	// height from the tile via AreaDefinition.StandGroundY, so this needs no
	// initialization on spawn / load / transition.
	GroundY float32
	Anim    Animation
}

type Animation struct {
	Kind     AnimKind
	Elapsed  float32
	Duration float32
	FromX    float32
	FromZ    float32
	ToX      float32
	ToZ      float32
	FromYaw  float32
	ToYaw    float32
	// FromY / ToY ease the player's ground height across a step when the
	// from-tile and to-tile sit at different elevation levels (walking a
	// ramp). Both are AreaDefinition.StandGroundY values; zero for flat
	// steps, so a level map never moves the camera vertically.
	FromY float32
	ToY   float32
}

type GameState struct {
	Area AreaDefinition
	// StepCount is the total number of player tile-steps taken in this
	// session. Drives the day/night cycle: every StepsPerCycle steps is
	// one full loop through the six time-of-day phases. Incremented by the
	// explore package when a step actually lands (not when blocked).
	StepCount int
	// Weather is the ambient-rain state (outdoor-only, purely atmospheric).
	// Advances on landed steps + eases per frame; see core/weather.go. Zero
	// value is WeatherClear with no cooldown, so a fresh session starts dry
	// and the per-step roll governs the first storm.
	Weather WeatherState
	Player  Player
	Party   []PartyMember
	// Packs is the field roster — each pack occupies one tile and holds
	// the enemies revealed if it's engaged. Only the highest-tier member
	// of a pack is rendered on the field; the rest appear at battle start.
	Packs     []Pack
	Battle    Battle
	MenuOpen  bool
	MenuIndex int
	// DebugOverlay shows in-world tile labels and a player-coord readout.
	// Toggled from the pause menu. Off by default so a fresh session looks
	// clean. It also doubles as the "debug mode" master gate: the Debug
	// submenu (and its toggles below) is only reachable while this is on.
	DebugOverlay bool
	// DebugMenuOpen is true while the debug submenu is showing. Opened
	// from the pause menu's Debug row (always reachable now — the master
	// "Debug Mode" on/off toggle lives inside the submenu). DebugMenuIndex
	// is its row cursor.
	DebugMenuOpen  bool
	DebugMenuIndex int
	// OptionsMenuOpen is true while the Options submenu is showing (opened
	// from the pause menu's Options row). OptionsMenuIndex is its cursor.
	// Mutually exclusive with MenuOpen / DebugMenuOpen — opening a submenu
	// drops the pause menu, mirroring the Debug submenu flow.
	OptionsMenuOpen  bool
	OptionsMenuIndex int
	// Out-of-battle "use" target picker, shared by the panels overlay's
	// Items tab (use a consumable on an ally) and Skills tab (cast a
	// single-target heal on an ally). UseTargetOpen gates the ally-picker
	// sub-modal; UseTargetCursor indexes the LIVING-member list it shows.
	// Exactly one of UsePendingItem / UsePendingSkill is set (the other is
	// its None sentinel) to say what the chosen ally receives; the skill
	// path also remembers UsePendingCaster (the member paying the MP).
	UseTargetOpen    bool
	UseTargetCursor  int
	UsePendingItem   ItemKind
	UsePendingSkill  SkillID
	UsePendingCaster int
	// Out-of-battle heal-skill chooser (Skills tab Use). Raised only when the
	// cursored member has MORE THAN ONE out-of-battle heal (today just the
	// Cleric: Prayer + Mass Mend) — a single heal casts directly, none refuses.
	// HealPickCaster is the member; HealPickCursor indexes core.OutOfBattleHeals
	// for that member. On confirm it routes into the same cast path (ally picker
	// for a single-target heal, immediate party-wide apply for Mass Mend).
	HealPickOpen   bool
	HealPickCaster int
	HealPickCursor int
	// EnemiesDisabled (debug) removes field packs from play: they stop
	// rendering and neither the step-into nor the wander AI can start a
	// battle. Lets the player walk a map freely to inspect it.
	EnemiesDisabled bool
	// EasyBattleQuit (debug) lets the player abandon an active battle
	// instantly from the action menu (drops the engaged pack and returns
	// to explore). Off by default so normal play can't trivially flee.
	EasyBattleQuit bool
	// RenderLogEnabled (debug) turns on per-frame diagnostics dumped to
	// crawler-render.log. Used to inspect the world-draw path
	// (camera state, lighting profile, tile/prop counts, shader IDs)
	// when chasing flicker / invisibility bugs that don't reproduce
	// in the editor.
	RenderLogEnabled bool
	// Inventory is shared across the party — single global stack list.
	// Stocked by Steal pickups and consumed by the in-battle Item action.
	Inventory []ItemStack
	// Gold is the party's shared currency. Earned from battle loot
	// (AwardBattleLoot) and spent at the pause-menu shop. Persisted in the
	// save file. Single pool, like Inventory.
	Gold int
	// Shop overlay. Opened IN-UNIVERSE (by a merchant / shop tile in the
	// world — entry point not yet wired; never a menu row). ShopOpen gates it;
	// ShopTab picks the Buy / Sell column; ShopCursor is the highlighted row
	// within the active tab's list. All three reset on open (openShop).
	// Mutually exclusive with the other overlays via core.ActiveModal's ladder.
	ShopOpen   bool
	ShopTab    ShopTab
	ShopCursor int
	// Quests is the journal — a save-persisted list of objectives the
	// player reads from the char menu's Quests tab. Carried across area
	// transitions / restarts like Party / Inventory / Gold. Empty for now
	// (no seed quests); the Quests tab uses PanelsRowCursor for its row.
	Quests []Quest
	// Chests is the runtime list of on-field chests. Built from
	// AreaDefinition.ChestSpawns by NewGameState. Looted chests stay in
	// the slice (so their open-lid sprite keeps rendering); the explore
	// loop just refuses interaction on them.
	Chests []Chest
	// Doors is the runtime list of area-transition doors on the field.
	// Built from AreaDefinition.DoorSpawns by NewGameState. The explore
	// movement loop checks this slice on every step-land to detect
	// "stepped onto a door" and fire the transition.
	Doors []Door
	// DoorPrompt is the index into Doors of the door the player just
	// stepped onto and is being asked to confirm, or -1 when no prompt is
	// showing. Stepping onto a door opens this confirm modal instead of
	// transitioning immediately; confirming sets PendingTransition, and
	// cancelling clears it (the player stays on the tile and can walk off).
	DoorPrompt int
	// PendingTransition is set when the player confirms the door prompt.
	// The run loop consumes this on the frame after movement settles to
	// swap in the new GameState. Empty TargetMap means "no transition
	// queued."
	PendingTransition AreaTransition
	// ChestOpen is the index into Chests of the currently-open chest, or
	// -1 when no chest UI is showing. ChestMenuIndex is the cursor row
	// inside the open chest (which item is highlighted). Both live in
	// GameState (not a transient overlay slice) so the pause menu and
	// the renderer can branch on "is a chest modal showing right now?"
	// without reaching into explore.
	ChestOpen      int
	ChestMenuIndex int
	// LevelUpOpen is true while the stat-allocation modal is showing.
	// The modal is NO LONGER auto-opened post-battle — battle wins now
	// accrue PendingLevelUps + SkillPoints on the member, and the
	// player chooses when to allocate by opening the modal from the
	// Tome panels overlay's Character / Stats tab. LevelUpMember is
	// the index into Party of the member currently allocating stat
	// points; the modal walks pending members in slice order and
	// closes when no member has unspent points left. core.ActiveModal
	// surfaces LevelUp above the panels / chest / pause priorities so
	// the dispatch ladder lives in one helper. The row cursor lives
	// on LevelUpRowCursor below; the retired LevelUpStat field was
	// redundant with it (cursor < StatCount already names the stat).
	LevelUpOpen   bool
	LevelUpMember int
	// LevelUpPending is the per-stat staged increment count. The
	// player accumulates picks inside the modal; nothing commits to
	// the underlying member until they confirm the Apply row. Lets
	// them see the resulting stat block before locking it in.
	LevelUpPending [StatCount]int
	// LevelUpRowCursor is the modal's row cursor. Covers stat rows
	// (0..StatCount-1) and the Apply button (StatCount). Skill points
	// no longer ride this modal — they're spent from the Skills
	// panel's tree UI.
	LevelUpRowCursor int
	// PanelsOpen is true while the game panels overlay is up (Stats /
	// Equipment / Items / Skills / Map tabs). Triggered by the "big
	// start" button — gamepad middle / keyboard I — and gated above
	// the pause-menu / battle priorities in explore.Update so the
	// overlay can't co-exist with combat input. PanelsTab is the
	// currently-shown tab; PanelsRowCursor is the per-tab vertical
	// cursor — its semantic is "selected row within the active tab,"
	// so it indexes a party member on Stats/Equipment/Skills, an
	// inventory stack on Items, and is unused on Map (the Map tab
	// uses PanelsMapZoom instead). Resets to 0 on every tab switch
	// so each tab opens at the top.
	PanelsOpen      bool
	PanelsTab       PanelTab
	PanelsRowCursor int
	// Skill-tree modal: a Diablo-2-style sub-dialog raised from the Skills
	// tab (Confirm on a member) showing that member's three trees. While
	// SkillTreeOpen the modal owns panel input — SkillTreeCol picks the
	// tree column (Left/Right), SkillTreeRow picks the node within it
	// (Up/Down), Confirm invests a SkillPoint, Back closes just the modal.
	// SkillTreeMember is the member the trees belong to (the cursored party
	// column at open time). All reset when the modal opens; SkillTreeOpen
	// is forced false on overlay close / tab switch so it can't strand.
	SkillTreeOpen   bool
	SkillTreeMember int
	SkillTreeCol    int
	SkillTreeRow    int
	// Equipment tab (no drag-and-drop — it works like the Items menu).
	// EquipSlotCursor is the focused equip slot row (0..EquipSlotCount-1)
	// on the cursored member (PanelsRowCursor picks the member column).
	// Confirm/click on a slot opens the item picker: a smaller sub-modal
	// listing the inventory items eligible for that slot. EquipPickerOpen
	// gates that sub-modal and EquipPickerCursor is the row inside it.
	// All three reset on overlay open / tab switch (ResetEquipPanels).
	EquipSlotCursor   int
	EquipPickerOpen   bool
	EquipPickerCursor int
	// PanelsMapZoom is the cells-on-screen value for the Map tab. Saved
	// separately from PanelsScroll so cycling between tabs preserves
	// each tab's cursor state. Initialized lazily on first Map view.
	PanelsMapZoom int
	// Visited tracks which tiles the player has stepped on for the
	// fog-of-war reveal in the Map panel. Width matches Area.Width;
	// indexed as Visited[z][x]. Built by NewGameState (start tile pre-
	// marked) and updated on every successful step in explore.
	Visited [][]bool
	Quit    bool
	// VFXQueue holds visual-effect spawn intents emitted by the battle
	// and explore layers. The render layer drains it each frame and
	// materialises particles in its private pool — keeping VFX data
	// in core (rather than in render) means battle code can emit FX
	// without taking a raylib import. See core/vfx.go for the request
	// type and enqueue helpers; render/vfx.go owns the pool + tick +
	// draw. Cleared on area transition + on battle exit so stale
	// particles don't render in the new scene.
	VFXQueue []VFXRequest
	// VFXResetRequested is a one-shot signal to the render layer:
	// when true, drop every live particle BEFORE processing the next
	// frame's VFXQueue. The bool is the only seam battle code can
	// use to invalidate the render-side pool without importing
	// render (which would create a cycle). The renderer consumes the
	// flag — reads, acts, clears — once per frame. Battle's
	// clearBattleResidual sets it on every battle exit so the
	// formation-relative particles from the fight don't drift into
	// the explore camera's view at random world positions.
	VFXResetRequested bool
	// RNG is the per-state random source for all gameplay rolls (accuracy,
	// steal, burn duration, press-window placement, etc.). Per-state means
	// two GameStates (e.g. a fresh playtest after an editor change) don't
	// share a stream — and a future "Restart from seed N" can drop a
	// deterministic Rand in here instead of seeding from the wall clock.
	// Always non-nil after NewGameState; Rand() lazily initializes if a
	// caller built a GameState by struct literal (tests).
	RNG *rand.Rand
}

// Rand returns the GameState's RNG, lazily initializing it with a
// wall-clock seed if a caller built the GameState by struct literal
// (mostly tests). Production paths go through NewGameState which seeds
// the RNG eagerly; this helper is the safety net for direct
// construction.
func (g *GameState) Rand() *rand.Rand {
	if g.RNG == nil {
		g.RNG = rand.New(rand.NewSource(rand.Int63()))
	}
	return g.RNG
}

// RandRangeF returns a uniform float32 in [lo, hi) from the GameState RNG
// (nil-safe via Rand). Single home for the "lo + rand*(hi-lo)" range roll
// so the expression can't drift between call sites (weather lightning, etc.).
func (g *GameState) RandRangeF(lo, hi float32) float32 {
	return lo + g.Rand().Float32()*(hi-lo)
}

// SetStatusMessage writes the transient status / quiet-message line shown
// under the HUD. Battle's setBattleStatus and the exploration code (e.g. a
// failed door transition) share this slot — it doubles as the ambient
// "quiet message" out of combat — so writes route through here rather than
// poking g.Battle.Message directly at the call site.
func (g *GameState) SetStatusMessage(msg string) {
	g.Battle.Message = msg
}

// Pack is one runtime enemy pack on the field. Members carries the per-
// instance battle state for every enemy in the pack; their TileX/TileZ
// fields aren't used while the pack is whole (the pack's tile is the
// authority). When a battle is active and this is the engaged pack, the
// renderer reshuffles members into battle formation by slot.
//
// Three (X, Z) pairs are now in play across the pack lifecycle and
// each names a different concept — kept distinct on purpose so a
// future per-tile pack lookup doesn't have to guess which one to key
// on:
//
//   - PackSpawn.TileX/TileZ (authoring layer, in AreaDefinition):
//     the tile the author dropped the pack on in the editor. Read by
//     the editor's reachability / placement check and the snapper.
//   - Pack.TileX/TileZ (runtime, this struct): the tile the pack is
//     standing on right NOW. Updated by the wander/chase AI; this is
//     the field-render position and the engagement target.
//   - Pack.HomeX/HomeZ (runtime, this struct): the leash anchor for
//     the junkyard-dog AI — the pack will roam, but never further
//     than PackLeashRadius (Chebyshev) from this point. Seeded from
//     the snapped spawn tile in placePacks so authored placement is
//     the leash center. Never reassigned — packs leash to their
//     birth tile for the run.
type Pack struct {
	TileX int
	TileZ int
	HomeX int
	HomeZ int
	// X, Z are the visible (interpolated) world coords used by the
	// field renderer. They track the tile center when the pack is at
	// rest and ease toward the destination tile while Anim is active.
	// Seeded in placePacks to the snapped tile center and snapped to
	// the target on Anim completion or battle engagement.
	X float32
	Z float32
	// Anim is the pack's step animation state. While Anim.Kind is
	// AnimStep the renderer lerps X/Z between FromX/Z and ToX/Z;
	// TickPackAnimations clears it when Elapsed >= Duration.
	Anim    Animation
	Members []Enemy
	// AI mirrors the authoring PackSpawn.AI for this pack's runtime
	// behavior — propagated by placePacks so the per-step planner can
	// dispatch on the per-pack mode without crossing back to the area.
	AI PackAI
	// PatrolDir is the PackAIPatrol pace direction along the X axis
	// (+1 east / -1 west). Runtime-only (not authored or saved — packs
	// rebuild fresh): placePacks seeds it to +1; the patrol planner flips
	// it at the leash boundary / a wall, and the explore step-applier
	// writes the chosen direction back. Unused by the other AI modes.
	PatrolDir int
}

// Stats is the per-actor attribute block. Drives derived values (HP from VIT)
// and damage formulas (STR for melee, INT for magic, WIS for heal, DEX for
// thief precision, SPD for turn order).
type Stats struct {
	STR int
	DEX int
	INT int
	WIS int
	VIT int
	SPD int
}

type PartyMember struct {
	Class PartyClass
	Name  string
	Stats Stats
	HP    int
	MaxHP int // derived from Stats.VIT
	MP    int
	MaxMP int
	// Armor lives outside Stats so it isn't a spendable level-up stat —
	// it's defensive scaffolding for future equipment. Defaults to 0 for
	// every party member at character creation; enemies set non-zero
	// values in their EnemyDefinition (amoeba is the headline tanky
	// foe). Clipped against phys-tagged incoming damage in ApplyArmor.
	// EffectiveArmor (party.go) sums this with bonuses from Equipped.
	Armor int

	// Equipped is the per-slot equipment, indexed by EquipSlotIndex
	// (RightHand / LeftHand / Armor / Accessory1 / Accessory2).
	// ItemNone means the slot is empty. Equipment stat bonuses /
	// armor bonuses are read through EffectiveStats / EffectiveArmor
	// / EffectiveMDef rather than baked into Stats directly, so a
	// future "swap your sword mid-battle" path doesn't need to
	// recompute base stats — the readers always pull from Equipped.
	Equipped [EquipSlotCount]ItemKind

	AttackBump  float32
	DamageFlash float32
	// HitKnockback is the reaction timer for taking a hit — the
	// renderer pushes the sprite AWAY from the attacker (toward
	// the camera, for the party) for the timer's duration, peaking
	// mid-curve via BumpOffset. Mirrors AttackBump in shape but
	// represents the receiver's recoil rather than the attacker's
	// lunge. Set in damagePartyMember whenever real damage lands.
	HitKnockback float32

	// Defending is set when the member chose the Defend action on their last
	// turn. It cuts incoming damage. The flag is cleared at the start of the
	// member's *next* turn, so its lifetime is "every queue position between
	// this member's defend and their next action." A slow defender soaks more
	// enemy turns than a fast one — by design: tanks tend to be slow, and the
	// SPD interaction makes Defend more valuable in their hands.
	Defending bool

	// PoisonTurns counts down each time the member's own turn resolves
	// (immediately AFTER their action), dealing PoisonTickDamage per tick.
	// Inflicted by the Diseased Rat's bite; cannot stack onto an already-
	// poisoned member (mirrors the Burn rule for enemies).
	PoisonTurns int

	// SleepTurns counts down at the start of the member's own turn (like
	// Burn). While > 0, the turn skips entirely. Any incoming damage > 0
	// wakes the sleeper and zeroes the counter. Doesn't stack onto an
	// already-sleeping target. Inflicted by SkillSleep (goblin mage).
	SleepTurns int

	// StunTurns mirrors the enemy-side field — counts down at the
	// start of the stunned member's own turn, target skips their
	// action until it hits zero. No wake-on-damage (Sleep's job).
	// No party-facing skill inflicts Stun yet, but the field exists
	// so the helper in battle.go has a symmetric path against
	// PartyMember and Enemy.
	StunTurns int

	// Ingested is true while a Venus Mantrap has the member swallowed.
	// While set: the member is removed from the turn queue, can't be
	// targeted by friend or foe, and can't be damaged (the prey is
	// effectively invulnerable inside the plant — see actions.go's
	// damagePartyMember / healPartyMember short-circuits). IngestedBy
	// is the active-pack slot of the mantrap currently holding them;
	// the prey is released when that slot's enemy dies or the battle
	// ends. Inflicted by SkillIngest.
	Ingested   bool
	IngestedBy int

	// WebbedTurns counts the Cave Spider's web. While > 0 the member's
	// effective SPD is halved (in battle.actorSpeed) and Ingest refuses
	// to land on them. Ticks at the END of the webbed member's own
	// turn (like Poison). Inflicted by SkillWeb; expires when the
	// counter ticks to 0. Does not stack — re-applying replaces the
	// counter only if the incoming duration is higher.
	WebbedTurns int

	// ConfusedTurns counts the Will-o'-Wisp's mind-twist. While > 0,
	// the member's chosen action has WispConfuseRetargetRoll chance
	// to retarget randomly (any living friend OR foe) at apply
	// time. Ticks at the END of the confused member's own turn.
	// WIS-resistible at apply roll; does not stack.
	ConfusedTurns int

	// Level and XP track per-character progression. XP is the running
	// total toward the next level; XPForLevel(Level) is the threshold.
	// PendingLevelUps queues completed level-ups whose stat points the
	// player hasn't spent yet — populated by ApplyXP, drained by the
	// level-up modal. Per-character (not pooled) so each member has
	// their own pace; living members get the full encounter XP, dead
	// members get nothing.
	Level int
	XP    int
	// PendingLevelUps is the unspent stat-point counter. Each level
	// grants LevelStatPoints (default 3) which the level-up modal
	// walks down via SpendStatPoint.
	PendingLevelUps int
	// SkillPoints is the spendable pool used to purchase tier upgrades
	// in the Skills panel's tree UI. Earned at level-up (one per level
	// via LevelSkillPoints), saved across levels — the player decides
	// when and where to invest. Replaces the legacy PendingSkillPoints
	// flow that auto-spent into MaxMP at the level-up modal.
	SkillPoints int
	// SkillTiers tracks the purchased upgrade tier per skill (0 = base
	// skill, 1..MaxSkillTier = progressively-upgraded). nil-safe: an
	// unspent skill returns 0 via SkillTierOf. Per-member so a Wizard's
	// Firebolt and a Cleric's same skill (if a future class shared one)
	// can level independently.
	SkillTiers map[SkillID]int
	// TreeRanks tracks ranks invested per Diablo-2-style skill-tree node
	// (see core/skilltrees.go), keyed by node ID. nil-safe: an
	// un-invested node reads 0 via TreeNodeRank. Spent from SkillPoints
	// in the Skills-tab tree modal. UI-only for now — ranks fill pips and
	// gate prerequisites but don't yet alter combat (the "skill impl"
	// pass wires effects to these ranks later). Serializes with the rest
	// of PartyMember in the save file; old saves load it as nil.
	TreeRanks map[string]int
	// SkillCursor is the index into the class's Skills array that the
	// action menu's "Skill" row casts. In-battle Tab cycles it; the
	// renderer reads it via PartySkill so the row label matches what
	// Enter will actually fire. 0 = signature skill (default); 1+ =
	// the class's two thematic skills (see PartyClassDefinition.Skills).
	SkillCursor int
}

// HPPerVIT is the MaxHP granted per point of VIT. Kept small so the
// numbers stay readable on the party-card bars. Single source of truth
// for both MaxHPFor and the VIT stat description shown in the level-up UI.
const HPPerVIT = 2

// MaxHPFor returns the derived MaxHP from a Stats block.
func MaxHPFor(s Stats) int {
	return s.VIT * HPPerVIT
}

// MeleeDamage = STR + skill base. Used for Attack, Swipe, etc.
func MeleeDamage(s Stats, base int) int {
	return s.STR + base
}

// MagicDamage = INT + skill base. Used for Firebolt and other casts.
func MagicDamage(s Stats, base int) int {
	return s.INT + base
}

// HealAmount = WIS + skill base. Used for Prayer.
func HealAmount(s Stats, base int) int {
	return s.WIS + base
}

// accuracyFrom is the shared hit-chance curve [0, 1]: a stat-driven
// baseline plus the timing grade's bonus, clamped. The governing stat
// differs by attack type — MeleeAccuracy passes STR, RangedAccuracy
// passes DEX — but the curve shape is identical so the two can't drift.
// An Excellent press pushes the result past 1.0 before the clamp, so a
// perfectly-timed hit always lands regardless of stat.
func accuracyFrom(stat int, quality int) float64 {
	base := AccuracyBaseline + AccuracyPerStat*float64(stat)
	bonus := 0.0
	if quality >= 0 && quality < len(timingGrades) {
		bonus = timingGrades[quality].AccuracyBonus
	}
	return Clamp(base+bonus, 0, 1)
}

// MeleeAccuracy is the per-swing hit chance of a MELEE attack — STR is
// the governing stat (a strong fighter lands their swings; a frail caster
// flails). The basic attack is melee, so it rolls this. Skills aren't
// accuracy-gated (they pay MP, shouldn't be double-jeopardied).
func MeleeAccuracy(s Stats, quality int) float64 {
	return accuracyFrom(s.STR, quality)
}

// RangedAccuracy is the per-shot hit chance of a RANGED attack — DEX is
// the governing stat. The seam for ranged attacks; no current attack is
// flagged ranged, so DEX's live offensive roles stay dodge + crit until
// a ranged attack rolls this.
func RangedAccuracy(s Stats, quality int) float64 {
	return accuracyFrom(s.DEX, quality)
}

// StealChance clamps the (tier-augmented) base steal chance to [0, 1].
// Steal no longer scales with DEX — it's a flat base chance, modified
// only by timing quality at the call site (applySteal). Kept as a
// function so the clamp + the "base can exceed 1 via Steal tier bonuses"
// contract live in one place.
func StealChance(base float64) float64 {
	return Clamp(base, 0, 1)
}

type Enemy struct {
	Kind  EnemyKind
	HP    int
	MaxHP int
	Alive bool
	// CustomName is non-empty for enemies instantiated from an area's
	// CustomEnemies list. Kind remains the base kind so renderers can reuse
	// the built-in sprite, while DefinitionOverride holds the authored combat
	// stats and display text.
	CustomName            string
	DefinitionOverride    EnemyDefinition
	HasDefinitionOverride bool
	// Item is the steal loot kind. Seeded from EnemyDefinition.Item at
	// spawn time and reset to ItemNone once stolen, so the same enemy
	// can't be looted twice in one battle. ItemKind (not a name string)
	// so the steal path adds it to inventory without a name→kind lookup.
	// Per-enemy overrides aren't currently authored anywhere — if/when
	// the editor grows per-spawn loot, the Name + custom MaxHP fields can
	// follow in the same pass.
	Item ItemKind

	// Armor is the per-instance damage damp seeded from
	// EnemyDefinition.Armor at NewEnemy time. Phys-tagged damage clips
	// by this amount (floor 1); magic / heal / buff bypass entirely.
	// Stored per-instance so a future "amoeba splits, halving its
	// armor" mechanic can mutate it without changing the definition.
	Armor int

	AttackBump  float32
	DamageFlash float32
	// HitKnockback is the reaction recoil timer. The renderer pushes
	// the enemy sprite AWAY from the camera (deeper into the arena)
	// for the timer's duration. Mirrors AttackBump in shape but
	// represents the receiver's flinch rather than the attacker's
	// lunge. Set in damageEnemy whenever real damage lands.
	HitKnockback float32
	DeathFade    float32
	BurnTurns    int
	// SleepTurns counts down at the start of the enemy's own turn (same
	// shape as BurnTurns). Currently the goblin mage only inflicts
	// sleep on the party, but the field exists so a future "Lullaby"
	// party skill against enemies plugs into the same machinery.
	SleepTurns int
	// StunTurns is the skip-next-turn counter applied by quality-
	// conditional procs (Crushing Blow on Great+, Frost Lance on
	// Great+). Unlike SleepTurns, damage does NOT clear it — the
	// enemy stays locked out until the counter decrements to zero
	// on its own turn-start tick.
	StunTurns int
	// PoisonTurns mirrors the party-side field — the Thief's Venom
	// Strike applies it. Decrements at the end of the enemy's turn
	// (tickPoisonAfterEnemyTurn) for symmetry with the party-side
	// tick. Per-instance because it's inflicted in combat, not part
	// of the enemy's static definition.
	PoisonTurns int

	// SkillCastCount tracks per-battle uses of any skill whose
	// definition carries a non-zero PerBattleCastLimit. Read by
	// usableEnemySkills to filter the AI's pick list — once a
	// capped skill hits its limit, it's dropped from the cast set
	// for the rest of the encounter. Lazy-init on first cast so
	// the common (uncapped) case stays a single nil-map allocation.
	// The Necromancer's RaiseBones is the headline user; future
	// "boss has 3 ultimates" patterns plug in for free.
	SkillCastCount map[SkillID]int

	// Floating damage popup state. Value is the number to show, Quality is
	// the timing grade (drives color + the trailing "!" on Excellent), Timer
	// counts down to zero from QualityResultDuration.
	DamagePopup        int
	DamagePopupQuality int
	DamagePopupTimer   float32
}

// ActorRef points at one slot in the turn queue. IsParty=true means Index is
// an index into Party; IsParty=false means Index is a slot into the active
// pack's Members (g.Packs[Battle.ActivePack].Members).
type ActorRef struct {
	IsParty bool
	Index   int
}

// ValidPartyIndex reports whether this actor references a real party
// slot — IsParty=true AND Index is in [0, len(party)). Used by every
// party-side tick/lookup that used to inline the "Index < 0 || Index
// >= len(g.Party)" guard; the helper keeps the bounds rule in one
// place so a future "ghost party slot" or "joinable NPC" gate lands
// once. Returns false for enemy-actor refs without needing a caller
// short-circuit.
func (a ActorRef) ValidPartyIndex(party []PartyMember) bool {
	return a.IsParty && a.Index >= 0 && a.Index < len(party)
}

// Battle owns every transient piece of state for an in-progress encounter.
// The struct has grown a lot — the lifetimes of its fields are deliberate
// and don't all reset on the same boundary, so it's worth knowing which
// bucket each field falls into:
//
//   - Battle-lifetime (set by Start, cleared by leaveBattle): ActivePack,
//     EnemyIndex, Log, Splash, Queue, NextRoundQueue.
//   - Round-lifetime (rebuilt by beginNewRound): Queue, NextRoundQueue,
//     EnemyAttackCursor, plus the QueueCursor that walks Queue.
//   - Turn-lifetime (set per actor's turn): CurrentParty, ActionMode,
//     MenuIndex, PendingSkill, PendingItem, ItemMenuIndex, PartyTarget,
//     EnemyAttacker, plus all Timing* fields.
//   - Animation-lifetime (visible-feedback timers — counted down in
//     updateBattleEffects): TimingFlash, TimingIntro, LastQualityTimer,
//     SequencePulseTimer.
//   - Hit-stop (counted down in tickFlashHold, since it pauses every
//     other animation ticker including updateBattleEffects): HitStop.
//
// finishActorTurn and leaveBattle are the two hand-offs; clearBattleResidual
// is the canonical "reset everything transient" centerpiece.
type Battle struct {
	// ActivePack is the index into g.Packs of the engaged pack. -1 = no
	// battle in progress. The pack's Members slice is the in-battle enemy
	// roster; EnemyIndex addresses members in that slice.
	ActivePack   int
	EnemyIndex   int
	CurrentParty int
	ActionMode   ActionMode
	MenuIndex    int
	PendingSkill SkillID
	// PartyTarget is the player's currently-highlighted ally (heal skills,
	// item targets). Independent of the enemy attack cursor — cycling the
	// player's ally selection no longer shifts who enemies hit next.
	PartyTarget int
	// EnemyAttackCursor is the round-robin pointer enemies advance through
	// when picking who to swing at. Lives separately from PartyTarget so the
	// player's ally cycling doesn't perturb enemy targeting.
	EnemyAttackCursor int
	Phase             BattlePhase
	Timer             float32
	Splash            float32
	Message           string
	Log               []string

	// Mixed-initiative turn queue. Built once at the start of each round
	// (sorted by SPD descending, ties broken by side then index), consumed
	// front-to-back. Cursor exhausted → start a new round (resolve burns,
	// rebuild queue). Dead actors are skipped on advance.
	Queue       []ActorRef
	QueueCursor int
	// NextRoundQueue is the projected SPD-sorted queue for the round after
	// the current one — built alongside Queue at round start so the turn-
	// forecast HUD doesn't have to re-sort every frame. May go stale during
	// the round if actors die; render-time death-skip handles that gracefully.
	NextRoundQueue []ActorRef
	// Readiness carries each actor's leftover ATB gauge ACROSS rounds,
	// keyed by ActorRef. buildTurnQueue seeds from it and writes back the
	// remainder each round, so SPD increases an actor's turn RATE (a
	// faster actor's surplus accumulates into extra turns over time), not
	// just turn order within a round. Reset at battle Start; new actors
	// (raised skeletons) default to 0.
	Readiness map[ActorRef]int

	// Timed-hit minigame state. Timing drives the bar; TimingFlash holds the
	// bar visible for a beat after a press; TimingIntro is a pre-bar pause so
	// the prompt reads. EnemyAttacker tracks the queue slot of an attacking
	// enemy. LastQuality* drives the floating popup over the actor.
	Timing      TimingState
	TimingFlash float32
	TimingIntro float32
	// ChargeNeedsRelease is set when a charge bar arms to gate the
	// engage check until the player has RELEASED the confirm key from
	// menu confirmation. Without it, the same Enter that confirmed the
	// target would bleed into the bar's held-state check and engage
	// the charge immediately. Cleared on first frame where
	// AttackTimingHeld() is false after the bar arms; thereafter
	// fresh presses engage the charge normally.
	ChargeNeedsRelease bool
	EnemyAttacker      int
	LastQuality        int
	LastQualityTimer   float32
	LastQualityIndex   int
	LastQualityIsBlock bool

	// EnemyPendingSkill is the skill the currently-attacking enemy is
	// casting this turn (goblin mage Firebolt / Sleep). SkillNone means
	// "plain melee" — the existing defend-timing path runs. When set,
	// updateEnemyTiming skips the defend bar and routes to
	// resolveEnemySpell instead of resolveEnemyAttacker. Cleared on
	// turn end by resolveAndFinishEnemyAttack.
	EnemyPendingSkill SkillID

	// HitStop is the post-flash freeze on Great/Excellent grades — see
	// HitStopFor in core/timing.go. While >0, battle Update returns early
	// and the bar's apply step is deferred. Pauses every transient ticker
	// (popup, sprite bumps, damage flashes) so the moment punctuates.
	HitStop float32

	// ShakeTimer / ShakePeak / ShakeDur drive the combat screen-shake.
	// ShakeTimer is the countdown (decayed in updateBattleEffects);
	// ShakePeak is the peak camera offset in world units; ShakeDur is the
	// duration it was armed with (the normalizer for the ease-out). The
	// render camera offsets by ShakePeak·(ShakeTimer/ShakeDur). All three
	// are armed together by core.TriggerCombatShake — a small base on a
	// well-timed press, a bigger one on crits / AoE casts (which override
	// the base). The wall-clock-driven oscillation means the screen shakes
	// even while HitStop freezes the sim. Reset at Start / clearBattleResidual.
	ShakeTimer float32
	ShakePeak  float32
	ShakeDur   float32

	// SequencePulseTimer + Index drive the brief scale-up animation on the
	// arrow that just landed correctly during the pickpocket sequence. The
	// renderer reads these to single out one slot per tap. Index is -1
	// when no pulse is in flight.
	SequencePulseTimer float32
	SequencePulseIndex int

	// PendingItem is set when the player picks an item out of the inventory
	// menu and is choosing the ally to use it on. Reset to ItemNone after
	// the use applies (or the player backs out). The picker state itself —
	// which inventory row is highlighted — is ItemMenuIndex.
	PendingItem   ItemKind
	ItemMenuIndex int
	// SkillMenuIndex is the cursor inside the Skill submenu (opens
	// when the player picks the "Skill" action row). Indexes into the
	// current member's PartyClassDefinition.Skills. Persisted on the
	// member as SkillCursor on confirm so the next turn's submenu
	// opens on whichever skill they last picked.
	SkillMenuIndex int
}

// Active reports whether a battle is currently in progress (any phase
// other than BattleNone). Single source for the "are we in combat?"
// predicate so explore / render / HUD gates don't drift on what counts
// as active — a future "BattleWon shouldn't count for input gating"
// tweak is one method, not a grep.
func (b Battle) Active() bool {
	return b.Phase != BattleNone
}

// ClearTiming drops the timing-bar minigame state and its flash hold
// back to zero. Used by every seam that ends a turn / battle so the
// next phase opens with a clean bar. Promoted to a method so the
// "drop Timing and TimingFlash together" rule lives in one spot.
func (b *Battle) ClearTiming() {
	b.Timing = TimingState{}
	b.TimingFlash = 0
}

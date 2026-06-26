package core

import (
	"crawler/internal/app/core/mapfile"
	"math/rand"
)

type MaterialSet int

// PackMemberRef is one authored pack member. Built-ins carry only Kind; custom
// enemies carry their base Kind (visual fallback) plus CustomName for lookup.
type PackMemberRef struct {
	Kind       EnemyKind
	CustomName string
	// Row is the authored formation rank. Zero value RowFront, so pre-rows
	// packs read as all-front.
	Row Row
}

// PackSpawn is one authored pack: a tile position + its enemy roster. The field
// renders one figure (highest-tier member); the rest reveal at battle start.
type PackSpawn struct {
	TileX   int
	TileZ   int
	Members []PackMemberRef
	// AI selects per-pack movement. Zero value PackAINone = stationary until
	// stepped into.
	AI PackAI
}

// PackAI is the movement style for one pack, authored on the PackSpawn.
type PackAI int

const (
	// PackAINone: stationary, waits for the player. No per-step planning.
	PackAINone PackAI = iota
	// PackAIJunkyardDog: wander-near-spawn / chase-when-close (see packai.go).
	PackAIJunkyardDog
	// PackAIPatrol paces a fixed X-axis line around the spawn (out to
	// PatrolRadius, bouncing at ends/walls), engaging only by pacing into the
	// player. Pace direction in Pack.PatrolDir.
	PackAIPatrol
	// PackAISkittish flees: steps directly away within SkittishFleeRadius (never
	// onto the player), else wanders its leash.
	PackAISkittish
	// PackAICount sizes name/label tables. Bump by adding a mode above; init
	// guards catch missing wiring at startup.
	PackAICount = int(PackAISkittish) + 1
)

// ChestSpawn is one authored chest: a tile position + its loot. (Runtime Chest
// adds a Looted flag.)
type ChestSpawn struct {
	TileX int
	TileZ int
	Items []ItemKind
}

// DoorStyle picks the visual fixture a door renders as. Transition behavior is
// identical across styles — purely cosmetic so an author can match surroundings.
// (A same-map portal keeps the "self" placeholder end-to-end so a map rename
// can't strand a self-referencing door; resolved at use time.)
type DoorStyle int

const (
	DoorStyleBuilding DoorStyle = iota // timber-framed door (the original)
	DoorStyleCave                      // rough stone archway
	DoorStyleField                     // open gateway / trail arch
	// DoorStyleCount is the style-cycle wrap modulus + render model-table size.
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

// HasTarget reports whether this door names a resolvable destination (empty
// TargetMap/TargetDoor = authored but unfinished). Single home for the rule.
func (d DoorSpawn) HasTarget() bool {
	return mapfile.DoorTargetComplete(d.TargetMap, d.TargetDoor)
}

// CrystalSpawn is one authored healing crystal (Grimrock-style save/heal point):
// just a tile position. (Runtime Crystal adds live Charge/Charged state.)
type CrystalSpawn struct {
	TileX int
	TileZ int
}

// TileXZ is implemented by the authored spawn types (PackSpawn / ChestSpawn /
// DoorSpawn / CrystalSpawn) AND their runtime counterparts (Chest / Door /
// Crystal) so the generic "thing on this tile" scan (SpawnIndexAt) ranges over it.
type TileXZ interface {
	Tile() (int, int)
}

func (s PackSpawn) Tile() (int, int)    { return s.TileX, s.TileZ }
func (s ChestSpawn) Tile() (int, int)   { return s.TileX, s.TileZ }
func (s DoorSpawn) Tile() (int, int)    { return s.TileX, s.TileZ }
func (s CrystalSpawn) Tile() (int, int) { return s.TileX, s.TileZ }
func (c Chest) Tile() (int, int)        { return c.TileX, c.TileZ }
func (d Door) Tile() (int, int)         { return d.TileX, d.TileZ }
func (c Crystal) Tile() (int, int)      { return c.TileX, c.TileZ }

// Door is one runtime door (from DoorSpawns via placeDoors). Blocks neither
// movement nor vision: stepping onto its tile fires the area transition.
type Door struct {
	TileX int
	TileZ int
	Name  string
	// TargetMap is the destination map id (bare name); "self" is resolved to the
	// local map id by AreaFromMapFile.
	TargetMap string
	// TargetDoor is the destination door's Name (DoorByName lookup).
	TargetDoor string
	// Facing is "which way the player faces walking out": sets post-transition
	// camera yaw AND offsets the exit tile one step that way (so they don't
	// re-trigger the transition). See doorExitTile in run.go.
	Facing int
	// Style is the visual fixture (render-only).
	Style DoorStyle
}

// HasTarget reports whether this runtime door names a resolvable destination.
func (d Door) HasTarget() bool {
	return mapfile.DoorTargetComplete(d.TargetMap, d.TargetDoor)
}

// AreaTransition is the queued "swap area next frame" request from the explore
// loop to the run loop. Empty TargetMap = no transition.
type AreaTransition struct {
	TargetMap  string
	TargetDoor string
}

// PanelTab indexes the game-panels overlay tabs, in left-to-right order. Adding
// a tab = one enum row + one row in render/panels.go's drawer table.
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

// PanelTabCharacter aliases PanelTabStats (the tab now reads "Character").
const PanelTabCharacter = PanelTabStats

// PanelTabLabel returns the short label for a tab (the tab-strip header).
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
		// Hosts Quests + Bestiary sub-tabs, so the top-level tab reads "Journal".
		return "Journal"
	case PanelTabMap:
		return "Map"
	default:
		// Fail loudly if a new tab forgets its label.
		panic("core: PanelTabLabel missing case for PanelTab")
	}
}

// JournalSubtab selects the Journal tab's view (quest log or bestiary), toggled
// Left/Right. Active view is GameState.JournalTab.
type JournalSubtab int

const (
	JournalQuests JournalSubtab = iota
	JournalBestiary
	JournalSubtabCount
)

// JournalSubtabLabel returns the short label for a journal sub-tab.
func JournalSubtabLabel(s JournalSubtab) string {
	switch s {
	case JournalQuests:
		return "Quests"
	case JournalBestiary:
		return "Bestiary"
	default:
		panic("core: JournalSubtabLabel missing case for JournalSubtab")
	}
}

// Chest is one runtime chest. Looted goes true once every stack is drained, at
// which point it renders open and ignores interaction. Blocks movement onto its
// tile — opened from an adjacent square.
type Chest struct {
	TileX  int
	TileZ  int
	Items  []ItemStack
	Looted bool
}

// Crystal is a Grimrock-style healing crystal. BLOCKS its tile (a solid object);
// while Charged, standing BESIDE it and pressing Confirm fully restores HP+MP AND
// autosaves, then it goes dormant. Recharges +1 per landed step up to
// CrystalRechargeSteps (re-arms). Charge state persists in SaveData.
type Crystal struct {
	TileX   int
	TileZ   int
	Charge  int
	Charged bool
	// SpinBurst is the touch-armed fast-spin countdown (from CrystalSpinBurstDuration),
	// decayed each frame by TickCrystalSpins. Render-only transient; 0 = idle spin only.
	// Not persisted (CrystalSave omits it) — a transient animation, not save state.
	SpinBurst float32
}

// AreaDefinition is the runtime form of a map (from AreaFromMapFile). Path is the
// disk source, empty for unsaved maps. Geometry is parallel ASCII grids of equal
// dimensions: required Walls/Floor/Decor/Props plus optional layers below. Width
// and Height are stored explicitly so blank layers reconstruct.
type AreaDefinition struct {
	Path   string
	Name   string
	Width  int
	Height int
	Walls  []string
	Floor  []string
	Decor  []string
	Props  []string
	// Ceiling: same dims as Walls; only TileCeilingSolid cells render an overhead
	// slab. Empty rows normalize to a blank layer at load so pre-ceiling maps work.
	Ceiling []string
	// Elevation: per-tile ground LEVEL ('0'..'9'); a ramp stores its LOW level.
	// Maps without an elevation: section load all-'0' (flat).
	Elevation []string
	// Solids is the VOXEL occupancy stack superseding Elevation: Solids[level][z]
	// is a width-wide row, one char per x; non-SolidAir = a solid cube at
	// (x,level,z) (char = material/skin). nil/empty = pure heightfield from
	// Elevation (materialized on demand, round-trips as the legacy elevation:
	// section). Only a column with a GAP forces the solids: section. See voxel.go.
	Solids [][]string
	// PropLevels: optional per-tile prop LEVEL (one base-36 char, or
	// PropLevelAuto='.' = lowest standable surface), so a prop on a deck renders +
	// blocks at the deck. nil = all auto (pre-voxel). See PropLevelAt.
	PropLevels []string
	// DecorLevels: decor analogue of PropLevels, '.' = auto. nil = all auto. See
	// DecorLevelAt.
	DecorLevels []string
	// FaceOverrides holds per-tile, per-direction cliff-face skin overrides (the
	// top-down editor can't paint a vertical face, so faces are a tile property).
	// No entry = base FaceSkinAt skin on every face. Sorted by (Z,X) for
	// deterministic encoding. See FaceSkinForDir.
	FaceOverrides []FaceOverride
	// faceOverrideIdx is a lazily-built (x,z)->index map so faceOverrideAt is O(1)
	// instead of scanning FaceOverrides per cube-face per frame. Built on first
	// lookup, invalidated (nil) by SetFaceDir / CloneArea. Unexported, so excluded
	// from encoding + equality — only FaceOverrides is authoritative.
	faceOverrideIdx map[[2]int]int
	Materials       MaterialSet
	StartTileX      int
	StartTileZ      int
	StartFacing     int
	PackSpawns      []PackSpawn
	// ChestSpawns is the authored chest list → runtime Chests in NewGameState.
	ChestSpawns []ChestSpawn
	// DoorSpawns is the authored door list → runtime g.Doors in NewGameState.
	DoorSpawns []DoorSpawn
	// CrystalSpawns is the authored crystal list → runtime Crystals (placeCrystals).
	CrystalSpawns []CrystalSpawn
	// CrystalsAuthored is true when the .map carried a `crystals:` section, so an
	// EMPTY CrystalSpawns means "deliberately none" — placeCrystals only
	// synthesizes the default entrance crystal when this is false (legacy maps).
	CrystalsAuthored bool
	// CustomEnemies are area-scoped author-defined enemy templates; pack spawns
	// reference them by Name, instantiated via CustomEnemyDef.Instantiate.
	CustomEnemies []CustomEnemyDef
	QuietMessage  string
	// Dialogs are the area's authored branching conversations (see dialog.go),
	// started by StartDialog (by id).
	Dialogs []DialogDefinition
	// Triggers auto-start a dialog on a world event (step / foe killed / region
	// entered) — see dialogtrigger.go.
	Triggers []DialogTrigger
	// Locations are named, elevation-specific rectangular regions a
	// DialogTriggerEnterLocation can fire on — see location.go.
	Locations []Location
}

type Player struct {
	TileX int
	TileZ int
	// Level is the voxel level of the cube-top the party stands on (ground under a
	// bridge vs deck over it). On a heightfield it just tracks the column top;
	// load-bearing only on a voxel map. Updated by ResolveStep each step.
	Level     int
	Facing    int
	X         float32
	Z         float32
	Yaw       float32
	LookYaw   float32
	LookPitch float32
	// GroundY is the eased world-space ground height under the player, only
	// meaningful while Anim.Kind == AnimStep; at rest the camera uses
	// AreaDefinition.StandGroundY, so this needs no init on spawn/load.
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
	// FromY / ToY ease ground height across a step between tiles at different
	// levels (a ramp). Both StandGroundY values; zero for flat steps.
	FromY float32
	ToY   float32
}

type GameState struct {
	Area AreaDefinition
	// StepCount is total player tile-steps this session (landed, not blocked).
	// Drives the day/night cycle: StepsPerCycle steps = one full phase loop.
	StepCount int
	// Weather is the ambient-rain state (outdoor-only, atmospheric); see
	// weather.go. Zero value WeatherClear, so a fresh session starts dry.
	Weather WeatherState
	Player  Player
	Party   []PartyMember
	// Packs is the field roster — one tile each, holding the enemies revealed
	// on engage. Only the highest-tier member renders on the field.
	Packs     []Pack
	Battle    Battle
	MenuOpen  bool
	MenuIndex int
	// DebugOverlay shows in-world tile labels + a coord readout. Off by default.
	// Doubles as the master debug gate: the Debug submenu is only reachable while on.
	DebugOverlay bool
	// DebugMenuOpen: debug submenu showing (opened from the pause menu's Debug
	// row). DebugMenuIndex is its row cursor.
	DebugMenuOpen  bool
	DebugMenuIndex int
	// SoundMenuOpen: Sound sub-submenu (music + SFX volume sliders) showing, opened
	// from the Options menu's Sound row; SoundMenuIndex its cursor. The volumes
	// themselves live in the audio package (persisted globally), not here.
	SoundMenuOpen  bool
	SoundMenuIndex int
	// RetroMenuOpen: Retro Filters sub-submenu showing; RetroMenuIndex its cursor.
	// RetroFilters holds per-filter 0..1 intensities the post-process pass reads;
	// all zeros = pass skipped. Runtime preference (not in SaveData), kept across
	// Restart.
	RetroMenuOpen  bool
	RetroMenuIndex int
	RetroFilters   [RetroFilterCount]float64
	// CombatTuneOpen: Debug ▸ Combat Tuning sub-submenu showing; CombatTuneIndex its
	// row cursor. BattleTuning holds the live combat-scene geometry the render layer
	// reads (camera/foe/party placement). Runtime debug preference (not in SaveData),
	// kept across Restart like RetroFilters.
	CombatTuneOpen  bool
	CombatTuneIndex int
	BattleTuning    BattleTuning
	// WipeMenuOpen: Debug ▸ Screen Wipe FX sub-submenu showing; WipeMenuIndex its
	// cursor. BattleWipe = the selected entry transition (played on battle start);
	// BattleWipePreview is a countdown that plays the wipe over the field for the
	// debug preview. Runtime debug prefs (not in SaveData), kept across Restart.
	WipeMenuOpen      bool
	WipeMenuIndex     int
	BattleWipe        BattleWipeKind
	BattleWipePreview float32
	// RetroFilterSky: true (default) captures the skybox inside the filter pass;
	// false draws it crisp with the filtered environment blitted over.
	RetroFilterSky bool
	// RetroFilterSprites: true captures billboards inside the filter pass; false
	// (default) draws them crisp over the filtered environment so per-asset
	// visuals.json FX show through. Render layer keeps occlusion correct.
	RetroFilterSprites bool
	// OptionsMenuOpen: Options submenu showing; OptionsMenuIndex its cursor.
	// Mutually exclusive with MenuOpen / DebugMenuOpen.
	OptionsMenuOpen  bool
	OptionsMenuIndex int
	// QuitConfirmOpen gates the "Quit — unsaved progress lost?" modal. Confirm
	// sets Quit; cancel reopens the pause menu. Transient (not in SaveData).
	QuitConfirmOpen bool
	// Out-of-battle "use" target picker, shared by the Items tab (consumable on
	// ally) and Skills tab (single-target heal). UseTargetCursor indexes the
	// LIVING-member list. Exactly one of UsePendingItem / UsePendingSkill is set;
	// the skill path also remembers UsePendingCaster (pays the MP).
	UseTargetOpen    bool
	UseTargetCursor  int
	UsePendingItem   ItemKind
	UsePendingSkill  SkillID
	UsePendingCaster int
	// Out-of-battle support-skill chooser (Skills tab), raised only when the member
	// has >1 castable support skill (heals + cures, no buffs); a single skill casts
	// directly. HealPickCursor indexes core.OutOfBattleSupportSkills for the member.
	HealPickOpen   bool
	HealPickCaster int
	HealPickCursor int
	// EnemiesDisabled (debug) removes field packs entirely (no render, no battle).
	EnemiesDisabled bool
	// EasyBattleQuit (debug) lets the player abandon an active battle from the
	// action menu. Off by default.
	EasyBattleQuit bool
	// RenderLogEnabled (debug) dumps per-frame world-draw diagnostics to
	// crawler-render.log for chasing flicker / invisibility bugs.
	RenderLogEnabled bool
	// DebugAllSkills (debug) lists EVERY player-castable skill in the battle menu
	// and makes casts free (skips MP deduction + affordability gate).
	DebugAllSkills bool
	// DebugSkipBattles (debug) auto-resolves an engaged pack as a win (kills + XP
	// + loot via the normal path) WITHOUT entering the scene. Unlike
	// EnemiesDisabled it still pays out. See battle.DebugSkipWin.
	DebugSkipBattles bool
	// RumbleEnabled is the controller-vibration setting (Options → "Vibration",
	// default true); false forces the motor to 0. Runtime preference (not in SaveData).
	RumbleEnabled bool
	// Inventory is the party's single shared stack list. Stocked by Steal,
	// consumed by the Item action.
	Inventory []ItemStack
	// Gold is the party's shared currency (earned via AwardBattleLoot). Persisted.
	Gold int
	// Shop overlay, opened IN-UNIVERSE (entry point not yet wired). ShopTab picks
	// Buy/Sell; ShopCursor the row. All reset on open. Mutually exclusive via
	// core.ActiveModal.
	ShopOpen   bool
	ShopTab    ShopTab
	ShopCursor int
	// Quests is the save-persisted journal, carried across transitions/restarts.
	// Empty for now; the Quests tab uses PanelsRowCursor for its row.
	Quests []Quest
	// Bestiary is accumulated foe knowledge (kill counts + scanned flags),
	// save-persisted and carried like Quests. A kind becomes "known" (HP revealed)
	// after BestiaryIDKills defeats or one Scan. See bestiary.go.
	Bestiary Bestiary
	// Chests is the runtime chest list (from ChestSpawns). Looted chests stay
	// (open-lid sprite); the explore loop refuses interaction on them.
	Chests []Chest
	// Doors is the runtime transition-door list (from DoorSpawns); checked on
	// every step-land to fire transitions.
	Doors []Door
	// Crystals is the runtime healing-crystal list (placeCrystals); recharged per
	// step, fires heal+autosave on/beside a charged one. Charge persists via SaveData.
	Crystals []Crystal
	// ActionLog is the rolling log of notable actions in AND out of combat (one
	// continuous buffer, capped at ActionLogMaxLines; written via LogMessage[Cat],
	// consecutive dupes coalesced). StatusMessage is the latest single line shown
	// under the HUD, also the freshest ActionLog entry.
	ActionLog     []LogLine
	StatusMessage string
	// DoorPrompt is the Doors index of the door being confirmed, or -1. Stepping
	// onto a door opens this modal; confirm sets PendingTransition, cancel clears it.
	DoorPrompt int
	// PendingTransition is set on door-prompt confirm; the run loop consumes it
	// the frame after movement settles. Empty TargetMap = none queued.
	PendingTransition AreaTransition
	// ChestOpen is the Chests index of the open chest, or -1; ChestMenuIndex is
	// the highlighted row. On GameState (not a transient slice) so pause/render
	// can branch on "chest modal showing?" without reaching into explore.
	ChestOpen      int
	ChestMenuIndex int
	// LevelUpOpen gates the stat-allocation modal (no longer auto-opened
	// post-battle — wins accrue PendingLevelUps + SkillPoints, the player opens
	// it from the Character tab). LevelUpMember is the Party index allocating; the
	// modal walks pending members in slice order. core.ActiveModal surfaces it
	// above panels/chest/pause.
	LevelUpOpen   bool
	LevelUpMember int
	// LevelUpPending is the per-stat staged increment count; nothing commits to
	// the member until they confirm Apply.
	LevelUpPending [StatCount]int
	// LevelUpRowCursor covers stat rows (0..StatCount-1) and the Apply button
	// (StatCount). Skill points are spent from the Skills tree, not here.
	LevelUpRowCursor int
	// PanelsOpen gates the panels overlay (Stats/Equipment/Items/Skills/Map),
	// gated above pause/battle so it can't coexist with combat input. PanelsTab is
	// the shown tab; PanelsRowCursor is the per-tab row (member on
	// Stats/Equipment/Skills, inventory stack on Items, unused on Map). Resets to
	// 0 on tab switch.
	PanelsOpen      bool
	PanelsTab       PanelTab
	PanelsRowCursor int
	// PanelSwapSource is the Character tab's formation-swap "held" member, or -1.
	// A second pick swaps slots; re-picking the held one cancels. Reset to -1 on
	// every open/close/tab switch (resetPanelSubmodals). UI cursor, not save state.
	PanelSwapSource int
	// JournalTab selects the Journal view (JournalQuests default / bestiary),
	// toggled Left/Right; PanelsRowCursor scrolls the active sub-view. Not persisted.
	JournalTab JournalSubtab
	// Skill-tree modal (Confirm on a Skills-tab member): SkillTreeCol picks the
	// tree (Left/Right), SkillTreeRow the node (Up/Down), Confirm invests a point.
	// SkillTreeMember is the member; all reset on open, SkillTreeOpen forced false
	// on overlay close / tab switch so it can't strand.
	SkillTreeOpen   bool
	SkillTreeMember int
	SkillTreeCol    int
	SkillTreeRow    int
	// Equipment tab (works like the Items menu). EquipSlotCursor is the focused
	// slot row on the cursored member; Confirm opens the item picker
	// (EquipPickerOpen gates it, EquipPickerCursor its row). All reset on
	// open/tab switch (ResetEquipPanels).
	EquipSlotCursor   int
	EquipPickerOpen   bool
	EquipPickerCursor int
	// PanelsMapZoom is the Map tab's cells-on-screen value, kept separate so tabs
	// preserve their own cursor state. Lazy-init on first Map view.
	PanelsMapZoom int
	// PanelsMapPanX/Z offset the Map view center (in tiles) from the player for
	// scrolling. Reset to 0 (re-centered) on overlay open / Map (re)entry.
	PanelsMapPanX int
	PanelsMapPanZ int
	// PanelsMapPanAccumX/Z + PanelsMapZoomAccum are the analog stick/wheel
	// accumulators that drain into the integer pan/zoom above. Transient input state
	// — live here (not package globals) so they reset with the GameState lifetime.
	PanelsMapPanAccumX float32
	PanelsMapPanAccumZ float32
	PanelsMapZoomAccum float32
	// TurnHeldLast / TurnRepeatCooldown drive explore's held-turn auto-repeat.
	// Transient input state, co-located for the same reason.
	TurnHeldLast       bool
	TurnRepeatCooldown float32
	// Visited tracks stepped-on tiles for the Map fog-of-war reveal; indexed
	// Visited[z][x]. Start tile pre-marked; updated on every successful step.
	Visited [][]bool
	// DialogOpen gates the conversation overlay (see dialog.go); Dialog holds the
	// live conversation. Highest-priority explore modal, opened by StartDialog,
	// dismissed by CloseDialog. Transient (not in SaveData).
	DialogOpen bool
	Dialog     DialogState
	// TriggersFired records fired Once dialog triggers (keyed by DialogTrigger.ID)
	// so they don't repeat. Resets per area visit, but IS persisted
	// (SaveData.TriggersFired) so a saved-past Once cutscene doesn't replay on reload.
	TriggersFired map[string]bool
	// InsideLocations tracks which regions the player currently stands in, for
	// rising-edge enter-location detection (see location.go). Transient: reseeded
	// from the spawn tile on every area entry, never saved.
	InsideLocations map[string]bool
	Quit            bool
	// VFXQueue holds VFX spawn intents from battle/explore; the render layer
	// drains it each frame into its private pool. Keeping the data in core lets
	// battle emit FX without a raylib import. See vfx.go. Cleared on transition + battle exit.
	VFXQueue []VFXRequest
	// vfxQueueSpare is the back-buffer DrainVFXQueue swaps in so a handler
	// re-enqueuing mid-drain appends to the fresh buffer, not the one being
	// iterated. The two ping-pong, reusing capacity.
	vfxQueueSpare []VFXRequest
	// VFXResetRequested is a one-shot signal: drop every live particle before the
	// next frame's VFXQueue. The only seam battle can use to invalidate the
	// render pool without importing render. Renderer reads/acts/clears once per
	// frame. Set on every battle exit so formation-relative particles don't drift.
	VFXResetRequested bool
	// RNG is the per-state random source for all gameplay rolls. Per-state so two
	// GameStates don't share a stream (and a future seeded Restart can drop a
	// deterministic Rand here). Always non-nil after NewGameState; Rand()
	// lazily inits for struct-literal construction (tests).
	RNG *rand.Rand
}

// Rand returns the GameState's RNG, lazily wall-clock-seeding it for
// struct-literal construction (tests); NewGameState seeds it eagerly.
func (g *GameState) Rand() *rand.Rand {
	if g.RNG == nil {
		g.RNG = rand.New(rand.NewSource(rand.Int63()))
	}
	return g.RNG
}

// RandRangeF returns a uniform float32 in [lo, hi) from the GameState RNG
// (nil-safe via Rand).
func (g *GameState) RandRangeF(lo, hi float32) float32 {
	return lo + g.Rand().Float32()*(hi-lo)
}

// SetStatusMessage writes the transient HUD status line WITHOUT logging it —
// for UI prompts ("Choose a target") that shouldn't pollute the ActionLog. Use
// LogMessage for real actions/results.
func (g *GameState) SetStatusMessage(msg string) {
	g.StatusMessage = msg
}

// LogCategory tags an action-log line for color-coding (render's logCategoryColor
// maps each to a tint). LogInfo is the neutral default.
type LogCategory uint8

const (
	LogInfo        LogCategory = iota // neutral flavor / prompts / spoils — pale blue
	LogDamageFoe                      // damage the party deals to a foe — white
	LogDamageParty                    // damage the party takes — pale red
	LogHeal                           // HP restored to the party — pale green
	LogDeath                          // a foe is felled — gold
	// LogCategoryCount bounds the parallel tint table in render (logCategoryColors);
	// a new category trips its init-time coverage assert until a tint is added.
	LogCategoryCount
)

// LogLine is one action-log entry: its text plus the category that colors it.
type LogLine struct {
	Text string
	Cat  LogCategory
}

// LogMessage records msg in the ActionLog (and status line) with the neutral
// LogInfo category. Empty msg clears the status line without logging. For
// actions/results; UI prompts use SetStatusMessage.
func (g *GameState) LogMessage(msg string) {
	g.LogMessageCat(msg, LogInfo)
}

// LogMessageCat is LogMessage with an explicit color category (consecutive dupes
// coalesced by text, capped at ActionLogMaxLines).
func (g *GameState) LogMessageCat(msg string, cat LogCategory) {
	g.StatusMessage = msg
	if msg == "" {
		return
	}
	if n := len(g.ActionLog); n > 0 && g.ActionLog[n-1].Text == msg {
		return
	}
	g.ActionLog = append(g.ActionLog, LogLine{Text: msg, Cat: cat})
	if len(g.ActionLog) > ActionLogMaxLines {
		g.ActionLog = g.ActionLog[len(g.ActionLog)-ActionLogMaxLines:]
	}
}

// Pack is one runtime enemy pack on the field. Members carries per-instance
// battle state; their TileX/TileZ are unused while whole (the pack's tile is the
// authority). On engage, the renderer reshuffles members into battle formation.
//
// Three (X,Z) pairs name distinct concepts: PackSpawn.TileX/TileZ (authored
// tile), Pack.TileX/TileZ (current tile, AI-updated, render+engage position),
// Pack.HomeX/HomeZ (junkyard-dog leash anchor — never roams >PackLeashRadius
// Chebyshev from it; seeded from the spawn tile, never reassigned).
type Pack struct {
	TileX int
	TileZ int
	// Level is the pack's standing voxel level (analogue of Player.Level). On a
	// heightfield it tracks the column top; on a voxel map it keeps a roaming
	// pack on the right surface. Updated by the AI via ResolveStep.
	Level int
	HomeX int
	HomeZ int
	// X, Z are the interpolated world coords for the field renderer: tile center
	// at rest, easing toward the destination while Anim is active.
	X float32
	Z float32
	// Anim is the step animation; while AnimStep the renderer lerps X/Z,
	// TickPackAnimations clears it at Elapsed >= Duration.
	Anim    Animation
	Members []Enemy
	// AI mirrors PackSpawn.AI (propagated by placePacks) so the planner dispatches
	// without crossing back to the area.
	AI PackAI
	// PatrolDir is the PackAIPatrol X-axis pace direction (+1 east / -1 west).
	// Runtime-only: seeded +1, flipped at the leash boundary / a wall. Unused by
	// other modes.
	PatrolDir int
}

// Stats is the per-actor attribute block: HP from VIT; STR melee, INT magic, WIS
// heal, DEX thief precision, SPD turn order.
type Stats struct {
	STR int
	DEX int
	INT int
	WIS int
	VIT int
	SPD int
}

// Negated returns the stat block with every field sign-flipped — turns a buff
// delta into the matching debuff (see NegStatDebuff).
func (s Stats) Negated() Stats {
	return Stats{STR: -s.STR, DEX: -s.DEX, INT: -s.INT, WIS: -s.WIS, VIT: -s.VIT, SPD: -s.SPD}
}

// StatusMod is one active timed stat/defense modifier — the unit of the
// STACKABLE buff/debuff system. Mods coexist and SUM; keyed by Source, so
// re-casting the SAME skill REFRESHES (no double-stack), different skills add
// independent entries. Turns ticks down at the bearer's end-of-turn, dropped at
// 0. Stats deltas additive (negative = debuff); Armor/MDef flat defensive
// grants. Combat-only.
type StatusMod struct {
	Source SkillID
	Stats  Stats
	Armor  int
	MDef   int
	Turns  int
}

type PartyMember struct {
	Class PartyClass
	Name  string
	Stats Stats
	HP    int
	MaxHP int // derived from Stats.VIT
	MP    int
	MaxMP int

	// Hunger is the per-member satiety meter (see hunger.go): 0 = Full, climbing
	// +HungerPerStep each landed step to SatietyMax = Starving. ONLY food lowers it
	// (FeedMember) — crystals and heals can't. Stored inverted so a zero value (new
	// member, or a save predating hunger) reads as Full. Drives SatietyStage, the
	// Starving status, the starving stat penalty, and the no-heal-while-starving gate.
	Hunger int
	// Armor lives outside Stats so it isn't a spendable level-up stat. Defaults 0
	// for party; enemies set it in EnemyDefinition. Clipped against phys damage in
	// ApplyArmor; EffectiveArmor sums this with Equipped bonuses.
	Armor int

	// Equipped is per-slot equipment (EquipSlotIndex), ItemNone = empty. Bonuses
	// are read through EffectiveStats/Armor/MDef, not baked into Stats, so a
	// mid-battle swap needs no base-stat recompute.
	Equipped [EquipSlotCount]ItemKind

	// Row/Col are the LIVE combat slot — what melee reach AND in-battle sprite
	// placement read. Recomputed from the home slot at battle start via
	// AmbushLive{Row,Col} (engage-side rotation), then packed front-ward as members
	// fall (downed sink to the back, living fill the front). Drifts during a fight;
	// reverts because the home slot below is never touched. Zero value (RowFront,
	// ColLeft).
	Row Row
	Col Col
	// HomeRow/HomeCol are the STANDING 2×2 slot — the player's PREFERRED formation,
	// persisted across fights and never mutated by combat. Battle start rotates these
	// into the live Row/Col; restoring the formation after a fight is just "use Home."
	HomeRow Row
	HomeCol Col

	AttackBump float32
	// HitAnim bundles the shared take-a-hit reaction (flash / knockback / damage
	// popup) with Enemy. Embedded — fields promote (m.DamageFlash, m.DamagePopup, …)
	// and JSON-flatten so the save keys are unchanged. Party-received hits pass
	// TimingQualityMiss → no "!"; set in applyPartyDamage.
	HitAnim

	// SwapSlide animates the formation Swap: a countdown (from SwapSlideDuration)
	// during which the sprite eases from its SwapFrom{Row,Col} slot to its current
	// live slot. Render-only transient; 0 = resting in the live slot.
	SwapSlide   float32
	SwapFromRow Row
	SwapFromCol Col

	// Defending (set by the Defend action) cuts incoming damage, cleared at the
	// start of the member's NEXT turn — so a slow defender soaks more enemy turns
	// (by design: tanks are slow, making Defend more valuable for them).
	Defending bool

	// PoisonTurns ticks AFTER the member's own action, dealing PoisonTickDamage.
	// Inflicted by the Diseased Rat; does not stack.
	PoisonTurns int

	// SleepTurns ticks at the start of the member's turn (skips it entirely);
	// incoming damage >0 wakes them. Does not stack. Inflicted by SkillSleep.
	SleepTurns int

	// StunTurns mirrors the enemy field — ticks at turn-start, skips the action,
	// no wake-on-damage. No party skill inflicts it yet; field exists for the
	// symmetric battle.go helper.
	StunTurns int

	// Ingested: a Venus Mantrap has swallowed the member — removed from the queue,
	// untargetable + undamageable (see damagePartyMember/healPartyMember). IngestedBy
	// is the holding mantrap's pack slot; released when it dies or battle ends.
	// Inflicted by SkillIngest.
	Ingested   bool
	IngestedBy int

	// WebbedTurns (Cave Spider): halves effective SPD and blocks Ingest. Ticks at
	// END of turn (like Poison). Does not stack (re-apply only raises the counter).
	WebbedTurns int

	// ConfusedTurns (Will-o'-Wisp): WispConfuseRetargetRoll chance to retarget the
	// action randomly (any friend OR foe). Ticks at END of turn. WIS-resistible;
	// does not stack.
	ConfusedTurns int

	// Buffs holds the member's STACKABLE buffs (Bless, War Banner, Stone Skin,
	// Smoke Bomb) as keyed StatusMod entries. Summed Stats/Armor/MDef fold into
	// the Effective* readers while live; each ticks at END of turn
	// (drainNonDamagingPartyStatuses), dropped at 0. Combat-only. Any active buff
	// paints PartyStatusBlessed.
	Buffs []StatusMod

	// ShieldHP (Cleric's Aegis) is a pool spent BEFORE HP — only overflow reaches
	// HP. Not turn-counted; cleared on battle exit. Coexists with Bless.
	ShieldHP int

	// IceArmorTurns (Wizard's Ice Armor): grants IceArmorMDef AND chills any enemy
	// landing a basic attack. Ticks at END of turn; SEPARATE counter so it coexists
	// with Bless / Stone Skin. Cleared on battle exit.
	IceArmorTurns int

	// RegenTurns / RegenPerTurn (Cleric's Renewal HoT): heals RegenPerTurn at END
	// of turn, then decrements. RegenPerTurn is snapshotted at cast so a later WIS
	// change can't retune it. Combat-only; surfaces as PartyStatusRegen. Does not
	// stack (re-apply replaces both).
	RegenTurns   int
	RegenPerTurn int

	// Level/XP track per-character progression; XPForLevel(Level) is the next
	// threshold. PendingLevelUps queues unspent level-ups (AddXP fills, modal
	// drains). Per-character; dead members earn no encounter XP.
	Level int
	XP    int
	// PendingLevelUps is the unspent stat-point counter; each level grants
	// LevelStatPoints, spent via SpendStatPoint.
	PendingLevelUps int
	// SkillPoints is the spendable pool for tier upgrades in the Skills tree,
	// earned one per level (LevelSkillPoints) and saved across levels.
	SkillPoints int
	// SkillTiers tracks the purchased tier per skill (0 = base). nil-safe via
	// SkillTierOf. Per-member so shared skills level independently.
	SkillTiers map[SkillID]int
	// TreeRanks tracks ranks per skill-tree node (see skilltrees.go), keyed by
	// node ID. nil-safe via TreeNodeRank. First rank learns the node's GrantSkill;
	// further ranks advance SkillTiers (EffectiveSkillEffect applies the upgrade).
	// Serialized; old saves load nil.
	TreeRanks map[string]int
	// SkillCursor indexes the member's LEARNED skills for the action menu's
	// "Skill" row (Tab cycles it). Clamps to 0 when out of range.
	SkillCursor int
}

// HPPerVIT is the MaxHP granted per point of VIT — source of truth for MaxHPFor
// and the VIT level-up description.
const HPPerVIT = 2

// MaxHPFor returns the derived MaxHP from a Stats block.
func MaxHPFor(s Stats) int {
	return s.VIT * HPPerVIT
}

// MPForINTDelta returns the MaxMP change for an INT delta (MPPerINT each) —
// single home for the formula so preview + god-mode boost can't drift.
func MPForINTDelta(delta int) int {
	return delta * MPPerINT
}

// MaxMPFor returns a class's derived MaxMP for the given stats: the class base
// plus MPForINTDelta for INT gained since creation. Mirrors MaxHPFor so the load
// path can re-derive instead of trusting a persisted value. Floors at 0. Returns
// ok=false for an unknown class (no proto to anchor the base).
func MaxMPFor(class PartyClass, stats Stats) (int, bool) {
	proto, ok := partyClassByID[class]
	if !ok {
		return 0, false
	}
	mp := proto.MaxMP + MPForINTDelta(stats.INT-proto.Stats.INT)
	if mp < 0 {
		mp = 0
	}
	return mp, true
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

// accuracyFrom is the shared hit-chance curve [0, 1]: stat-driven baseline +
// timing-grade bonus, clamped. Governing stat varies by caller (STR/DEX). An
// Excellent press exceeds 1.0 pre-clamp, so a perfect hit always lands.
func accuracyFrom(stat int, quality int) float64 {
	base := AccuracyBaseline + AccuracyPerStat*float64(stat)
	bonus := timingGrades[timingGradeAt(quality)].AccuracyBonus
	return Clamp(base+bonus, 0, 1)
}

// MeleeAccuracy is the per-swing melee hit chance (governing stat STR). The
// basic attack rolls this; skills aren't accuracy-gated (they pay MP).
func MeleeAccuracy(s Stats, quality int) float64 {
	return accuracyFrom(s.STR, quality)
}

// StealChance clamps the (tier-augmented) base steal chance to [0, 1]. Steal is
// a flat base (no DEX scaling), modified only by timing at applySteal.
func StealChance(base float64) float64 {
	return Clamp(base, 0, 1)
}

// HitAnim is the take-a-hit visual reaction shared by PartyMember and Enemy: the
// damage flash, the recoil knockback timer, and the floating damage popup. Embedded
// in both so ApplyDamageWithPopup/HitTarget bridge ONE field set. Fields are promoted
// (e.DamageFlash, m.DamagePopup, …) and JSON-flatten — the on-disk PartyMember save
// keys are unchanged.
type HitAnim struct {
	DamageFlash float32
	// HitKnockback is the take-a-hit recoil timer; the renderer pushes the sprite
	// over its duration via BumpOffset. Set on real damage.
	HitKnockback float32
	// Floating damage popup. Quality drives color + trailing "!"; Timer counts
	// down from QualityResultDuration.
	DamagePopup        int
	DamagePopupQuality int
	DamagePopupTimer   float32
}

type Enemy struct {
	Kind  EnemyKind
	HP    int
	MaxHP int
	Alive bool
	// CustomName is non-empty for area CustomEnemies; Kind stays the base kind
	// (reuses the sprite) while DefinitionOverride holds the authored stats/text.
	CustomName            string
	DefinitionOverride    EnemyDefinition
	HasDefinitionOverride bool
	// Item is the steal loot kind (from EnemyDefinition.Item), reset to ItemNone
	// once stolen so an enemy can't be looted twice.
	Item ItemKind

	// Armor is the per-instance phys-damage damp (from EnemyDefinition.Armor);
	// clips phys damage (floor 1), bypassed by magic/heal/buff. Per-instance so a
	// future split-and-halve mechanic can mutate it.
	Armor int

	// Row is the enemy's combat rank, seeded front-first at spawn and updated by
	// the per-death shunt that keeps the front packed. Gates melee reach (see
	// formation.go). Zero value RowFront.
	Row Row

	AttackBump float32
	// HitAnim bundles the shared take-a-hit reaction quintet (flash / knockback /
	// floating damage popup). Embedded — fields stay accessed as e.DamageFlash etc.,
	// and (for PartyMember) JSON-flatten to the same save keys.
	HitAnim
	DeathFade float32

	// SlotSlide animates a formation reshuffle (foe dies → front re-packs, or a back
	// foe is shunted up): a countdown (from SlotSlideDuration) during which the sprite
	// eases from its SlideFrom{Row,Slot,Count} placement to its current live placement.
	// Render-only transient; 0 = resting in the live slot. Mirrors PartyMember.SwapSlide.
	SlotSlide      float32
	SlideFromRow   Row
	SlideFromSlot  int
	SlideFromCount int
	// placed{Row,Slot,Count} cache the last-resolved formation placement so the tick
	// (UpdateEnemySlides) can detect a reshuffle and arm SlotSlide from the prior slot;
	// placedValid guards the battle-start seat (first placement snaps, doesn't slide).
	placedRow   Row
	placedSlot  int
	placedCount int
	placedValid bool

	BurnTurns int
	// SleepTurns ticks at turn-start (like BurnTurns). No party skill inflicts it
	// on enemies yet; field exists for a future Lullaby.
	SleepTurns int
	// StunTurns (skip-next-turn) from quality-conditional procs (Crushing Blow /
	// Frost Lance on Great+). Unlike Sleep, damage does NOT clear it.
	StunTurns int
	// PoisonTurns (Thief's Venom Strike) ticks at END of the enemy's turn
	// (tickPoisonAfterEnemyTurn). Per-instance (inflicted in combat).
	PoisonTurns int

	// BleedTurns (Warrior Rend / Thief Lacerate) is a SEPARATE counter from
	// PoisonTurns so Bleed STACKS with Poison. Ticks at END of turn
	// (tickBleedAfterEnemyTurn) dealing BleedTickDamage; cleared on death.
	BleedTurns int

	// Debuffs holds the enemy's STACKABLE debuffs (Cripple, Blind, Smoke Bomb,
	// chill) as keyed StatusMod entries — enemy mirror of PartyMember.Buffs. Summed
	// Stats fold into EffectiveEnemyStats (NEGATIVE deltas) while live; each ticks
	// at END of turn (tickEnemyBuffAfterTurn), dropped at 0, cleared on death.
	Debuffs []StatusMod

	// TauntTurns / TauntedBy force this enemy's basic-attack target (Warrior's
	// Taunt) to TauntedBy's party slot while that ally is a living target. Drains
	// at END of turn (tickEnemyTauntAfterTurn), cleared on death; a lapsed taunt
	// falls back to normal targeting.
	TauntTurns int
	TauntedBy  int

	// SkillCastCount tracks per-battle uses of any skill with a non-zero
	// PerBattleCastLimit; usableEnemySkills drops a capped skill once it hits the
	// limit. Lazy-init on first cast (Necromancer's RaiseBones is the headline user).
	SkillCastCount map[SkillID]int

	// Summoned marks a mid-fight addition (Necromancer's Raise Bones) so a Flee can
	// drop it — the abandoned pack reverts to its authored roster, not authored+summons.
	Summoned bool
}

// ActorRef points at one turn-queue slot: IsParty=true → Index into Party, else
// into the active pack's Members.
type ActorRef struct {
	IsParty bool
	Index   int
}

// ValidPartyIndex reports whether this actor is a real party slot (IsParty AND
// Index in range). Returns false for enemy refs.
func (a ActorRef) ValidPartyIndex(party []PartyMember) bool {
	return a.IsParty && PartyIndexInRange(party, a.Index)
}

// PartyIndexInRange is the one home for the `idx >= 0 && idx < len(party)` rule.
// Answers only "is the index a real seat"; callers layer HP/Ingested checks on top.
func PartyIndexInRange(party []PartyMember, idx int) bool {
	return idx >= 0 && idx < len(party)
}

// Battle owns all transient encounter state. Field lifetimes reset on different
// boundaries:
//
//   - Battle-lifetime (Start → leaveBattle): ActivePack, EnemyIndex, Splash,
//     Queue, NextRoundQueue.
//   - Round-lifetime (beginNewRound): Queue, NextRoundQueue, EnemyAttackCursor,
//     QueueCursor.
//   - Turn-lifetime (per actor): CurrentParty, ActionMode, MenuIndex,
//     PendingSkill, PendingItem, ItemMenuIndex, PartyTarget, EnemyAttacker, Timing*.
//   - Animation-lifetime (updateBattleEffects): TimingFlash, TimingIntro,
//     LastQualityTimer, SequencePulseTimer.
//   - Hit-stop (tickFlashHold, since it pauses other tickers): HitStop.
//
// clearBattleResidual is the canonical "reset everything transient".
type Battle struct {
	// ActivePack is the engaged pack's g.Packs index; -1 = no battle. Its Members
	// slice is the enemy roster, addressed by EnemyIndex.
	ActivePack   int
	EnemyIndex   int
	CurrentParty int
	ActionMode   ActionMode
	MenuIndex    int
	PendingSkill SkillID
	// PartyTarget is the player's highlighted ally (heal/item targets),
	// independent of EnemyAttackCursor.
	PartyTarget int
	// EnemyAttackCursor is the enemies' round-robin target pointer, separate from
	// PartyTarget so ally cycling doesn't perturb enemy targeting.
	EnemyAttackCursor int
	Phase             BattlePhase
	Timer             float32
	Splash            float32
	// EngageSide is how the fight was entered — front walk-in vs side/back ambush.
	// Drives the splash title ("Battle!" vs "Ambushed!"). Battle-lifetime.
	EngageSide EngageSide

	// Mixed-initiative turn queue, built per round (SPD desc, ties by side then
	// index), consumed front-to-back; cursor exhausted → new round. Dead actors
	// skipped on advance.
	Queue       []ActorRef
	QueueCursor int
	// NextRoundQueue is the projected next-round queue, built alongside Queue so
	// the forecast HUD doesn't re-sort per frame. May go stale on deaths;
	// render-time death-skip handles it.
	NextRoundQueue []ActorRef
	// Readiness carries each actor's leftover ATB gauge ACROSS rounds, so SPD
	// raises turn RATE (surplus accumulates into extra turns), not just order.
	// Reset at Start; new actors default 0.
	Readiness map[ActorRef]int

	// PhysDamageThisTurn tallies phys damage the cursor actor dealt this turn;
	// finishActorTurn converts it to Warrior Bloodthirst lifesteal then zeroes it.
	// Only SkillTagPhys hits feed it. Turn-lifetime despite sitting here.
	PhysDamageThisTurn int

	// Timed-hit minigame: Timing drives the bar; TimingFlash holds it visible a
	// beat after a press; TimingIntro is a pre-bar pause. LastQuality* drives the
	// floating popup.
	Timing      TimingState
	TimingFlash float32
	TimingIntro float32
	// ChargeNeedsRelease gates the charge-engage check until the player RELEASES
	// the confirm key, so the same Enter that confirmed the target doesn't engage
	// the charge immediately. Cleared on the first not-held frame after the bar arms.
	ChargeNeedsRelease bool
	EnemyAttacker      int
	LastQuality        int
	LastQualityTimer   float32
	LastQualityIndex   int
	LastQualityIsBlock bool

	// EnemyPendingSkill is the attacking enemy's skill this turn (mage Firebolt /
	// Sleep). SkillNone = plain melee. When set, updateEnemyTiming skips the
	// defend bar and routes to resolveEnemySpell. Cleared on turn end.
	EnemyPendingSkill SkillID

	// EnemyAttackMisses is set when a melee enemy turn rolled a clean miss — a basic
	// swing or a single-target melee skill (Ingest). Suppresses the defend bar and
	// wins the resolve switch over EnemyPendingSkill (the skill never lands). Cleared on turn end.
	EnemyAttackMisses bool

	// FleeReturnX/Z are the tile the player retreats to on a successful Flee — the
	// pre-engage-step square (so a pack-ambush flee steps off the pack's tile).
	FleeReturnX int
	FleeReturnZ int

	// HitStop is the post-flash freeze on Great/Excellent (see HitStopFor). While
	// >0, battle Update returns early and the apply step is deferred, pausing every
	// transient ticker so the moment punctuates.
	HitStop float32

	// ShakeTimer / ShakePeak / ShakeDur drive the combat screen-shake (camera
	// offset = ShakePeak·(ShakeTimer/ShakeDur)). Armed together by
	// TriggerCombatShake. Wall-clock oscillation, so it shakes even during
	// HitStop. Reset at Start / clearBattleResidual.
	ShakeTimer float32
	ShakePeak  float32
	ShakeDur   float32

	// RumbleStrength / RumbleTimer / RumbleDur drive controller vibration, armed
	// alongside the shake. Strength is the [0,1] motor peak. core.TickRumble
	// decays them per frame and returns the level for input.ApplyRumble (keeping
	// the raylib call out of core). Decays to 0 so it can't stick on.
	RumbleStrength float32
	RumbleTimer    float32
	RumbleDur      float32

	// SequencePulseTimer/Index drive the scale-up on the pickpocket arrow that
	// just landed. Index is -1 when no pulse is in flight.
	SequencePulseTimer float32
	SequencePulseIndex int

	// PendingItem is the item picked and awaiting an ally target, reset to
	// ItemNone after use/back-out. ItemMenuIndex is the highlighted picker row.
	PendingItem   ItemKind
	ItemMenuIndex int
	// SkillMenuIndex is the Skill submenu cursor (into the member's learned
	// skills), persisted to the member's SkillCursor on confirm.
	SkillMenuIndex int
	// SkillMenuList / ItemMenuList are per-turn scratch buffers backing the skill
	// and item submenus, refilled each open frame (refreshSkillMenuBuf /
	// refreshItemMenuBuf) so the renderer reads them straight. Reused, never
	// aliased; valid only for the fill frame. Not serialized.
	SkillMenuList []SkillID
	ItemMenuList  []ItemStack

	// Spoils + timers drive the victory spoils screen (render/victory.go). Spoils
	// is the before/after snapshot winBattle captures on award; VictoryElapsed
	// counts up from BattleWon (card eases in after VictoryDanceBeat, bars fill
	// over VictoryBarFillDuration); the *SfxCursor fields track how many level-up
	// / loot / XP-tick cues have rung so the loop fires each as its bar crosses a
	// threshold. All reset in clearBattleResidual. Spoils.Active false (debug
	// skip-win) falls back to the timed auto-leave.
	Spoils                VictorySpoils
	VictoryElapsed        float32
	VictoryLevelSfxCursor int
	VictoryLootSfxCursor  int
	VictoryTickSfxCursor  int
}

// MemberSpoils is one member's before→after XP snapshot for the spoils screen,
// recorded by winBattle around the award so the bar can animate across level
// thresholds. Dead members carry GainedXP == 0 and render greyed.
type MemberSpoils struct {
	Slot      int
	BeforeLvl int
	BeforeXP  int // within-level remainder at BeforeLvl
	AfterLvl  int
	AfterXP   int
	GainedXP  int
}

// VictorySpoils is a won battle's full payout, a read-only mirror captured by
// winBattle for the spoils screen. Drops is the per-defeat items folded into
// kind→count stacks. Active gates the screen (false → timed auto-leave).
type VictorySpoils struct {
	Members []MemberSpoils
	Gold    int
	Drops   []ItemStack
	Active  bool
}

// Active reports whether a battle is in progress (any phase but BattleNone) —
// single source for the "in combat?" predicate.
func (b Battle) Active() bool {
	return b.Phase != BattleNone
}

// ClearTiming zeroes the timing-bar state + flash hold together, so every
// turn/battle-end seam opens the next phase with a clean bar.
func (b *Battle) ClearTiming() {
	b.Timing = TimingState{}
	b.TimingFlash = 0
}

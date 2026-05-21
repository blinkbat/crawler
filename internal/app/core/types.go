package core

import "math/rand"

type MaterialSet int

// PackSpawn is one authored pack on the map: a tile position and the roster
// of enemy kinds that make up the pack. Single-member packs are the common
// case and read back from the on-disk format the same as the old single-
// enemy spawn line. The field renders one figure per pack (the highest-tier
// member); the rest are revealed when the battle starts.
type PackSpawn struct {
	TileX   int
	TileZ   int
	Members []EnemyKind
}

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
type DoorSpawn struct {
	TileX      int
	TileZ      int
	Name       string
	TargetMap  string
	TargetDoor string
	Facing     int
}

// HasTarget reports whether this door names a destination it can
// actually resolve. An empty TargetMap or TargetDoor means the door
// was authored but never finished — every "should this door fire?"
// predicate (runtime trigger, editor validator, parse-time check)
// goes through here so the rule lives in one place.
func (d DoorSpawn) HasTarget() bool {
	return d.TargetMap != "" && d.TargetDoor != ""
}

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
}

// HasTarget reports whether this runtime door names a resolvable
// destination. Sibling of DoorSpawn.HasTarget — same rule applied to
// the runtime list.
func (d Door) HasTarget() bool {
	return d.TargetMap != "" && d.TargetDoor != ""
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
	PanelTabMap
	PanelTabCount
)

// PanelTabLabel returns the short human label for a tab — used for the
// tab strip header in the overlay.
func PanelTabLabel(t PanelTab) string {
	switch t {
	case PanelTabStats:
		return "Stats"
	case PanelTabEquipment:
		return "Equipment"
	case PanelTabItems:
		return "Items"
	case PanelTabSkills:
		return "Skills"
	case PanelTabMap:
		return "Map"
	}
	return "?"
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
	Ceiling     []string
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
	DoorSpawns   []DoorSpawn
	QuietMessage string
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
	Anim      Animation
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
}

type GameState struct {
	Area AreaDefinition
	// StepCount is the total number of player tile-steps taken in this
	// session. Drives the day/night cycle: every 150 steps is one full
	// loop through the six time-of-day phases. Incremented by the
	// explore package when a step actually lands (not when blocked).
	StepCount int
	Player    Player
	Party     []PartyMember
	// Packs is the field roster — each pack occupies one tile and holds
	// the enemies revealed if it's engaged. Only the highest-tier member
	// of a pack is rendered on the field; the rest appear at battle start.
	Packs     []Pack
	Battle    Battle
	MenuOpen  bool
	MenuIndex int
	// DebugOverlay shows in-world tile labels and a player-coord readout.
	// Toggled from the pause menu. Off by default so a fresh session looks
	// clean.
	DebugOverlay bool
	// Inventory is shared across the party — single global stack list.
	// Stocked by Steal pickups and consumed by the in-battle Item action.
	Inventory []ItemStack
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
	// PendingTransition is set by the explore movement loop when the
	// player steps onto a door tile. The run loop consumes this on the
	// frame after movement settles to swap in the new GameState. Empty
	// TargetMap means "no transition queued."
	PendingTransition AreaTransition
	// ChestOpen is the index into Chests of the currently-open chest, or
	// -1 when no chest UI is showing. ChestMenuIndex is the cursor row
	// inside the open chest (which item is highlighted). Both live in
	// GameState (not a transient overlay slice) so the pause menu and
	// the renderer can branch on "is a chest modal showing right now?"
	// without reaching into explore.
	ChestOpen      int
	ChestMenuIndex int
	// LevelUpOpen is true while the post-battle level-up modal is up.
	// LevelUpMember is the index into Party of the member currently
	// allocating stat points; the modal walks members in slice order
	// and closes when no member has PendingLevelUps left. LevelUpStat
	// is the row cursor inside the modal (a core.Stat). Same shape as
	// the chest modal — explore.Update gates on LevelUpOpen above the
	// pause/battle priorities so the player can't accidentally drift.
	LevelUpOpen   bool
	LevelUpMember int
	LevelUpStat   Stat
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
	// PanelsMapZoom is the cells-on-screen value for the Map tab. Saved
	// separately from PanelsScroll so cycling between tabs preserves
	// each tab's cursor state. Initialized lazily on first Map view.
	PanelsMapZoom int
	// Visited tracks which tiles the player has stepped on for the
	// fog-of-war reveal in the Map panel. Width matches Area.Width;
	// indexed as Visited[z][x]. Built by NewGameState (start tile pre-
	// marked) and updated on every successful step in explore.
	Visited [][]bool
	Quit            bool
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

// Pack is one runtime enemy pack on the field. Members carries the per-
// instance battle state for every enemy in the pack; their TileX/TileZ
// fields aren't used while the pack is whole (the pack's tile is the
// authority). When a battle is active and this is the engaged pack, the
// renderer reshuffles members into battle formation by slot.
type Pack struct {
	TileX   int
	TileZ   int
	Members []Enemy
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
	Armor int

	AttackBump  float32
	DamageFlash float32

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

	// Level and XP track per-character progression. XP is the running
	// total toward the next level; XPForLevel(Level) is the threshold.
	// PendingLevelUps queues completed level-ups whose stat points the
	// player hasn't spent yet — populated by ApplyXP, drained by the
	// level-up modal. Per-character (not pooled) so each member has
	// their own pace; living members get the full encounter XP, dead
	// members get nothing.
	Level           int
	XP              int
	PendingLevelUps int
}

// MaxHPFor returns the derived MaxHP from a Stats block. Two HP per VIT keeps
// the numbers small and readable on the party-card bars.
func MaxHPFor(s Stats) int {
	return s.VIT * 2
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

// AttackAccuracy returns the hit chance [0, 1] of a basic attack given the
// attacker's stats and the timing grade they scored. DEX is the primary
// driver: a deft thief connects almost every swing, a stumpy warrior whiffs
// more often when their timing slips. Timing quality stacks on top — a Miss
// timing leaves accuracy at the DEX baseline, an Excellent press functionally
// guarantees the hit by pushing accuracy past 1.0 before the clamp.
//
// Tuning is keyed off the actual party DEX spread (Warrior/Cleric/Wizard at
// DEX 2, Thief at DEX 6):
//
//	Warrior (DEX 2), Miss timing:      0.63 → ~37% whiff
//	Warrior (DEX 2), Good timing:      0.83 → ~17% whiff
//	Warrior (DEX 2), Excellent timing: 1.08 → always hit
//	Thief   (DEX 6), Miss timing:      0.79 → ~21% whiff
//	Thief   (DEX 6), Excellent timing: 1.24 → always hit
//
// Basic attacks are the only action gated by this; skills already pay MP
// and shouldn't be doubly punished by a whiff.
func AttackAccuracy(s Stats, quality int) float64 {
	base := AccuracyBaseline + AccuracyPerDEX*float64(s.DEX)
	bonus := 0.0
	if quality >= 0 && quality < len(timingGrades) {
		bonus = timingGrades[quality].AccuracyBonus
	}
	acc := base + bonus
	if acc > 1.0 {
		acc = 1.0
	}
	if acc < 0.0 {
		acc = 0.0
	}
	return acc
}

// AttackHits rolls an accuracy check using AttackAccuracy against the given
// RNG. Returns true when the swing lands. Callers should use this only for
// basic attacks — skills already pay their own resource costs and rolling
// miss on top would feel punishing. `rng` is the GameState's per-state
// RNG (g.Rand()); tests pass their own seeded source for determinism.
func AttackHits(rng *rand.Rand, s Stats, quality int) bool {
	return rng.Float64() < AttackAccuracy(s, quality)
}

// StealChance scales the base steal chance by DEX: chance = base × (1 + DEX/20).
// Capped at 1.0 so a high-DEX rogue can't ever exceed certainty.
func StealChance(s Stats, base float64) float64 {
	chance := base * (1 + float64(s.DEX)/20)
	if chance > 1 {
		chance = 1
	}
	if chance < 0 {
		chance = 0
	}
	return chance
}

type Enemy struct {
	Kind  EnemyKind
	HP    int
	MaxHP int
	Alive bool
	// Item is the steal loot. Seeded from EnemyDefinition.Item at spawn time
	// and cleared once stolen, so the same enemy can't be looted twice in
	// one battle. Per-enemy overrides aren't currently authored anywhere —
	// if/when the editor grows per-spawn loot, the Name + custom MaxHP
	// fields can follow in the same pass.
	Item string

	// Armor is the per-instance damage damp seeded from
	// EnemyDefinition.Armor at NewEnemy time. Phys-tagged damage clips
	// by this amount (floor 1); magic / heal / buff bypass entirely.
	// Stored per-instance so a future "amoeba splits, halving its
	// armor" mechanic can mutate it without changing the definition.
	Armor int

	AttackBump  float32
	DamageFlash float32
	DeathFade   float32
	BurnTurns   int
	// SleepTurns counts down at the start of the enemy's own turn (same
	// shape as BurnTurns). Currently the goblin mage only inflicts
	// sleep on the party, but the field exists so a future "Lullaby"
	// party skill against enemies plugs into the same machinery.
	SleepTurns int

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

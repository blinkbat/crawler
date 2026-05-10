package core

// GameMap is the runtime form of an area's geometry, stored as four
// parallel ASCII grids (the editor's layers). All four are the same
// dimensions; the per-cell character meanings live in core/map.go's
// constant blocks.
type GameMap struct {
	Width     int
	Height    int
	Walls     []string
	Floor     []string
	Decor     []string
	Props     []string
	Materials MaterialSet
}

type MaterialSet int

type EnemySpawn struct {
	Kind  EnemyKind
	TileX int
	TileZ int
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
	Path         string
	Name         string
	Width        int
	Height       int
	Walls        []string
	Floor        []string
	Decor        []string
	Props        []string
	Materials    MaterialSet
	StartTileX   int
	StartTileZ   int
	StartFacing  int
	EnemySpawns  []EnemySpawn
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
	Kind     int
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
	Map       GameMap
	Area      AreaDefinition
	// StepCount is the total number of player tile-steps taken in this
	// session. Drives the day/night cycle: every 150 steps is one full
	// loop through the six time-of-day phases. Incremented by the
	// explore package when a step actually lands (not when blocked).
	StepCount int
	Player    Player
	Party     []PartyMember
	Enemies   []Enemy
	Battle    Battle
	MenuOpen  bool
	MenuIndex int
	// Inventory is shared across the party — single global stack list.
	// Stocked by Steal pickups and consumed by the in-battle Item action.
	Inventory []ItemStack
	Quit      bool
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

	AttackBump  float32
	DamageFlash float32

	// Defending is set when the member chose the Defend action on their last
	// turn. It cuts incoming damage. The flag is cleared at the start of the
	// member's *next* turn, so its lifetime is "every queue position between
	// this member's defend and their next action." A slow defender soaks more
	// enemy turns than a fast one — by design: tanks tend to be slow, and the
	// SPD interaction makes Defend more valuable in their hands.
	Defending bool
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
	Kind        EnemyKind
	TileX       int
	TileZ       int
	HP          int
	MaxHP       int
	Alive       bool
	Name        string
	MonsterType string
	Item        string

	AttackBump  float32
	DamageFlash float32
	DeathFade   float32
	BurnTurns   int

	// Floating damage popup state. Value is the number to show, Quality is
	// the timing grade (drives color + the trailing "!" on Excellent), Timer
	// counts down to zero from QualityResultDuration.
	DamagePopup        int
	DamagePopupQuality int
	DamagePopupTimer   float32
}

// ActorRef points at one slot in the turn queue. IsParty=true means Index is
// an index into Party; IsParty=false means Index is a slot into Battle.EnemyGroup
// (NOT into g.Enemies — the slot is the queue's position; you indirect through
// EnemyGroup[Index] to get the actual Enemy).
type ActorRef struct {
	IsParty bool
	Index   int
}

type Battle struct {
	EnemyIndex   int
	EnemyGroup   []int
	CurrentParty int
	ActionMode   int
	MenuIndex    int
	PendingSkill int
	PartyTarget  int
	Phase        int
	Timer        float32
	Splash       float32
	Message      string
	Log          []string

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
	Timing             TimingState
	TimingFlash        float32
	TimingIntro        float32
	EnemyAttacker      int
	LastQuality        int
	LastQualityTimer   float32
	LastQualityIndex   int
	LastQualityIsBlock bool

	// PendingItem is set when the player picks an item out of the inventory
	// menu and is choosing the ally to use it on. Reset to ItemNone after
	// the use applies (or the player backs out). The picker state itself —
	// which inventory row is highlighted — is ItemMenuIndex.
	PendingItem   ItemKind
	ItemMenuIndex int
}

package core

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
	PackSpawns   []PackSpawn
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
	// Inventory is shared across the party — single global stack list.
	// Stocked by Steal pickups and consumed by the in-battle Item action.
	Inventory []ItemStack
	Quit      bool
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
	Kind EnemyKind
	HP   int
	MaxHP int
	Alive bool
	// Item is the steal loot. Seeded from EnemyDefinition.Item at spawn time
	// and cleared once stolen, so the same enemy can't be looted twice in
	// one battle. Per-enemy overrides aren't currently authored anywhere —
	// if/when the editor grows per-spawn loot, the Name + custom MaxHP
	// fields can follow in the same pass.
	Item string

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
// an index into Party; IsParty=false means Index is a slot into the active
// pack's Members (g.Packs[Battle.ActivePack].Members).
type ActorRef struct {
	IsParty bool
	Index   int
}

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

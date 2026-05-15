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
	// DebugOverlay shows in-world tile labels and a player-coord readout.
	// Toggled from the pause menu. Off by default so a fresh session looks
	// clean.
	DebugOverlay bool
	// Inventory is shared across the party — single global stack list.
	// Stocked by Steal pickups and consumed by the in-battle Item action.
	Inventory []ItemStack
	Quit      bool
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
	Timing             TimingState
	TimingFlash        float32
	TimingIntro        float32
	EnemyAttacker      int
	LastQuality        int
	LastQualityTimer   float32
	LastQualityIndex   int
	LastQualityIsBlock bool

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

package core

const (
	// InitialWindowWidth/Height seed rl.InitWindow at startup. The window is
	// immediately resized to the monitor by render.SetDisplayMode(DisplayFullscreen),
	// so these are NOT the runtime screen dimensions — never use them for
	// HUD layout (render code reads rl.GetScreenWidth/Height via screenSize()
	// in the render package). They also double as the default size the
	// Windowed display mode snaps to on the first Fullscreen→Windowed toggle.
	InitialWindowWidth  = 1180
	InitialWindowHeight = 820

	TileSize             = 2.05
	WallHeight           = 2.25
	EyeHeight            = 1.32
	StepDuration         = 0.18
	TurnDuration         = 0.14
	BumpDuration         = 0.18
	FlashDuration        = 0.16
	DeathFadeDuration    = 0.55
	VictoryDanceDuration = 3.0
	MouseSense           = 0.0024
	MaxLookYaw           = 0.78
	MaxLookPitch         = 0.62
	FreeLookReturnSpeed  = 3.4
)

// AnimKind tags Player.Anim so the renderer/updater can dispatch by kind
// instead of poking at duration/elapsed alone.
type AnimKind int

const (
	AnimNone AnimKind = iota
	AnimStep
	AnimTurn
)

// BattlePhase is the top-level state of the battle FSM. Values are
// internal-only (no save file format depends on the integers), so the
// historical gaps are gone and the enum walks via iota.
type BattlePhase int

const (
	BattleNone BattlePhase = iota
	BattlePlayer
	BattleWon
	BattleLost
	BattleAttackTiming
	BattleEnemyTiming
)

// ActionMode is the sub-state of BattlePlayer — which input mode the action
// menu is currently in (action picker, target picker, item picker, etc.).
type ActionMode int

const (
	ActionMenu ActionMode = iota
	ActionEnemyTarget
	ActionPartyTarget
	ActionItemMenu
	ActionItemTarget
)

// ActionRow enumerates the in-battle action menu rows. The integer values
// double as g.Battle.MenuIndex cursor positions; reordering this enum
// reorders the menu. Typed so a free `int` can't accidentally stand in
// where a row label is expected.
type ActionRow int

const (
	ActionRowAttack ActionRow = iota
	ActionRowSkill
	ActionRowDefend
	ActionRowItem
)

// ActionRowCount is the wrap modulus for the action-menu cursor.
const ActionRowCount = int(ActionRowItem) + 1

const (
	// BattleSplashDuration is how long the encounter banner sits on screen at
	// the start of a battle. The battle code seeds Battle.Splash with this and
	// the renderer uses it for ease-in/ease-out math, so they stay in sync.
	BattleSplashDuration = float32(1.15)

	AttackTimingDuration = float32(1.4)
	DefendTimingDuration = float32(1.3)

	TimingFlashDuration   = float32(0.32)
	QualityResultDuration = float32(0.70)

	// Hit-stop is the brief world-pause inserted between the timing flash and
	// the action's apply step on Great/Excellent grades. The bar's already
	// frozen at the press; this freezes EVERYTHING else (sprite bumps, popup
	// floats, enemy bars) so the moment punctuates. Tuned short — under 200ms
	// — to feel like a satisfying punch, not a stutter.
	HitStopGreat     = float32(0.10)
	HitStopExcellent = float32(0.16)

	// Sequence arrow pulse: how long an arrow scales up after landing a
	// correct tap. Slightly less than the flash duration so the pulse decays
	// before the bar fades, keeping each tap visually punctuated.
	SequencePulseDuration = float32(0.22)
	EnemyTurnIntro        = float32(0.85)
	// Charge bars get a longer pre-arm pause so the player has time to read
	// the prompt. The bar arms early if the player presses/holds the input,
	// see updateAttackTiming for the skip logic.
	AttackTimingIntro = float32(0.35)
	// ChargeTimingIntro is the auto-arm timeout for charge bars: the
	// bar waits this long for the player to engage their input before
	// arming on its own. Pressing/holding during the wait skips
	// straight into the bar (see updateAttackTiming). 3s is the
	// "wait for the player, but don't softlock if they're afk" sweet
	// spot — long enough to read the prompt and breathe, short enough
	// that a missed cue doesn't burn the whole turn.
	ChargeTimingIntro = float32(3.0)
	// BlockBumpDuration is intentionally LONGER than BumpDuration (0.22 vs
	// 0.18). The hit landing on an enemy gets its own DamageFlash + popup
	// to read the impact; a successful block has none of that — the defender
	// just doesn't lose much HP. The longer recoil sells the block visually
	// when the damage number is "1" or "0."
	BlockBumpDuration = float32(0.22)

	// Charge minigame: hold the input through three ticks, then release at
	// the peak. Tick markers land at 30% / 56% / 78% of the bar; the peak
	// window runs 78%..86% (an 8% sweet spot — tighter than the previous
	// 12%). Past 86% the bar runs into a brief decay zone before timing out
	// at 100%. Charge grading now snaps to "fully filled tick count" — see
	// resolveCharge for the discrete grade dispatch.
	ChargeTimingDuration = float32(1.8)
	ChargeTick1Pct       = float32(0.30)
	ChargeTick2Pct       = float32(0.56)
	ChargeTick3Pct       = float32(0.78)
	ChargePeakStart      = float32(0.78)
	ChargePeakEnd        = float32(0.86)

	// Pickpocket sequence minigame: tap a randomized run of N directions in
	// order before time runs out. Each correct tap holds the grade; each
	// miss (wrong key OR a slot that timed out unfilled) drops it one notch.
	// Finishing all-correct under StealFastThreshold bumps grade by one
	// (capped at Excellent).
	StealTimingDuration = float32(1.8)
	StealSequenceLength = 5
	StealFastThreshold  = float32(1.0)

	// DefendingDamageMult scales incoming damage when the target picked the
	// Defend action on their previous turn. Stacks multiplicatively with the
	// defend timing quality, so a perfectly-blocked hit on a defending member
	// is reduced even further.
	DefendingDamageMult = float32(0.5)

	// Basic-attack accuracy curve. AttackAccuracy in types.go computes the
	// per-swing hit chance as:
	//     AccuracyBaseline + AccuracyPerDEX*DEX + timingBonus
	// then clamps to [0, 1]. timingBonus comes from timingGrades.AccuracyBonus
	// below; this pair sets the DEX-driven floor (Warrior at DEX 2 hits
	// 0.63 on a Miss timing, Thief at DEX 6 hits 0.79).
	AccuracyBaseline = 0.55
	AccuracyPerDEX   = 0.04

	// BurnTickDamage is the per-turn damage applied to a burning actor at
	// the start of their own turn. Flat so the strategic value of burn
	// stays predictable across enemy HP scales.
	BurnTickDamage = 2

	// PoisonTickDamage is the per-turn damage applied to a poisoned party
	// member, ticked immediately AFTER their action resolves (vs. burn,
	// which ticks at the start of the actor's turn). The "after the act"
	// timing means the player still gets their action in, but bleeds out
	// faster than they can heal it back without dedicated cleansing.
	PoisonTickDamage = 1

	// PoisonMinTurns / PoisonMaxTurns bound the duration rolled when a
	// Diseased Rat's bite inflicts Poison. Range inclusive at both ends.
	PoisonMinTurns = 3
	PoisonMaxTurns = 5

	// Skill / enemy proc chances. Lifted out of the per-entry registry
	// literals (party.go skillDefinitions, enemies.go enemyDefinitions) so
	// a balance pass touches one file. The registry still owns the
	// per-entry binding; these constants are the values it cites.
	StealBaseChance         = 0.40 // Thief: Steal base success before DEX/quality scaling.
	FireboltBurnChance      = 0.45 // Wizard: Firebolt burn inflict before quality scaling.
	DiseasedRatPoisonChance = 0.60 // Diseased Rat: per-bite poison inflict.

	// Day/night cycle tuning. Six phases of StepsPerPhase player tile-steps
	// make up one full loop (StepsPerCycle). Only landed exploration steps
	// advance the cycle (battles don't tick it), so combat preserves the
	// phase the player walked into.
	StepsPerPhase = 25
	StepsPerCycle = StepsPerPhase * TimeOfDayCount

	// BattleLogMaxLines caps the rolling combat log buffer so a long fight
	// doesn't grow it unbounded. The renderer reads len(Log) to draw the
	// last-N visible lines; this cap is the ceiling for any scrollback
	// feature that might land later.
	BattleLogMaxLines = 40

	// MaxMapDimension caps editor map width/height. Used by both the typed
	// numeric input and the +/- resize buttons so they share one ceiling.
	MaxMapDimension = 200

	// MinMapDimension is the smallest playable map. Border walls take 2
	// cells in each axis; 4×4 leaves a 2×2 interior — the tightest you
	// can put a player start and one pack on without overlap.
	MinMapDimension = 4
)

// PressWindow groups the press-bar window geometry. Values are fractions
// of the bar's duration. At construction time the window's position is
// randomized within [Min, Max] so two consecutive bars don't land in the
// same place — but it never starts before Min so the player can't get hit
// with a window that opens immediately. Width is fixed; MaxEnd clamps the
// tail so a window can't run into the last sliver of the bar (slides back
// to fit, see NewTimingState).
var PressWindow = struct {
	MinStart float32
	MaxStart float32
	Width    float32
	MaxEnd   float32
}{
	MinStart: 0.38,
	MaxStart: 0.62,
	Width:    0.18,
	MaxEnd:   0.96,
}

// timingGrades is the single per-grade attribute table for the timed-hit
// minigame. Every core-side function that varies by TimingQuality reads
// from this slice — Label, attack/defense multipliers, and the
// accuracy-roll bonus all live in one row per grade so a balance pass
// touches one place instead of four parallel switches.
//
// Render-side (color, throb intensity) and battle-side (audio cue) attrs
// live in their own per-package tables (qualityVisuals in render/timing.go,
// gradeSounds in battle/battle.go) — package layering keeps audio out of
// core and rl.Color out of core, so we get one table per layer.
//
// Defense multipliers are <1 (lower incoming damage); attack multipliers
// are >=1 (higher outgoing damage); the accuracy bonus is added to the
// DEX-driven baseline and clamped at 1.0 (see AttackAccuracy).
var timingGrades = []struct {
	Label         string
	Atk           float32
	Def           float32
	AccuracyBonus float64
}{
	TimingQualityMiss:      {Label: "Miss...", Atk: 1.0, Def: 1.0, AccuracyBonus: 0.0},
	TimingQualityNice:      {Label: "Nice!", Atk: 1.25, Def: 0.75, AccuracyBonus: 0.10},
	TimingQualityGood:      {Label: "Good!", Atk: 1.5, Def: 0.5, AccuracyBonus: 0.20},
	TimingQualityGreat:     {Label: "Great!", Atk: 1.75, Def: 0.35, AccuracyBonus: 0.30},
	TimingQualityExcellent: {Label: "Excellent!", Atk: 2.0, Def: 0.25, AccuracyBonus: 0.45},
}

// init asserts timingGrades covers every TimingQualityXxx grade. The
// other two parallel tables (qualityVisuals in render, gradeSounds in
// battle) carry their own length-check inits against TimingQualityCount.
// Adding a new grade is now: extend the iota in timing.go, add a row
// here, add a row in qualityVisuals, add a row in gradeSounds — any
// one missing panics at startup.
func init() {
	if len(timingGrades) != int(TimingQualityCount) {
		panic("core: timingGrades length must match TimingQualityCount — add a row when extending the grade enum")
	}
}

// SkillID identifies a learned skill. Stored on Battle.PendingSkill and
// used as the map key for action handlers.
type SkillID int

const (
	SkillNone SkillID = iota
	SkillSwipe
	SkillPrayer
	SkillSteal
	SkillFirebolt
	// SkillSleep is the goblin-mage's status-inflict cast — single
	// target, puts a party member to sleep for SleepMin..SleepMaxTurns.
	// Wakes on any incoming damage. Tagged Magic so it bypasses armor.
	SkillSleep
	// SkillIngest is the Venus Mantrap's signature swallow — single
	// target, removes the party member from combat (skipped in the turn
	// queue, untargetable by friend or foe) until the mantrap that
	// swallowed them is defeated. Each mantrap holds at most one
	// prisoner; a mantrap with prey can still bite-attack but won't
	// ingest a second target. Tagged Magic so the cast itself bypasses
	// armor (it doesn't deal damage anyway).
	SkillIngest
)

// SkillTag classifies a skill for damage-type interactions (armor,
// future elemental resists) and for HUD color-coding. Phys damage
// clips against the target's Armor; Magic / Heal / Buff bypass it.
// Independent of SkillKind: Kind controls the stat-scaling formula
// (STR vs INT vs WIS), Tag controls the defensive interaction.
type SkillTag int

const (
	SkillTagNone SkillTag = iota
	SkillTagPhys
	SkillTagMagic
	SkillTagHeal
	SkillTagBuff
)

// Sleep status duration bounds. Same shape as BurnMin/MaxTurns and
// PoisonMin/MaxTurns — the SleepEffect roller picks uniform-random
// value in [Min, Max] when the cast lands. Any incoming damage > 0
// wakes the sleeper and clears their SleepTurns counter.
const (
	SleepMinTurns = 2
	SleepMaxTurns = 5
)

// XP / level constants. Per-character XP and levels (one pool + one
// counter per PartyMember) with a geometric per-level cost: each level
// costs LevelXPBase × LevelXPRatio^(level-1) — 100, 200, 400, 800.
// LevelStatPoints is the number of points the player allocates on each
// level-up — small enough that one level is a noticeable bump, not a
// respec. BaseLevel is the level every member starts at (1, not 0, so
// the cost formula works out).
const (
	LevelXPBase     = 100
	LevelXPRatio    = 2.0
	LevelStatPoints = 3
	BaseLevel       = 1
)

const (
	North = 0
	East  = 1
	South = 2
	West  = 3
)

// PauseMenuItem enumerates the rows in the pause menu. The integer values
// double as menu cursor positions (g.MenuIndex), so reordering this enum
// reorders the menu. Single source of truth shared by explore (cursor
// dispatch) and render (row drawing) — neither side reinvents the count.
type PauseMenuItem int

const (
	PauseMenuRestart PauseMenuItem = iota
	PauseMenuStats
	PauseMenuDebug
	PauseMenuDisplay
	PauseMenuJukebox
	PauseMenuQuit
)

// PauseMenuCount is the wrap modulus for the pause menu cursor. Bump by
// adding a PauseMenuItem enum constant above this line — neither caller
// hard-codes "3" anywhere.
const PauseMenuCount = int(PauseMenuQuit) + 1

package core

import (
	"math/rand"
	"time"
)

const (
	ScreenWidth  = 1180
	ScreenHeight = 820

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

// BattlePhase is the top-level state of the battle FSM. Gaps in the value
// space are historical — leave them so any persisted save (none today,
// future-proofing) doesn't shift meaning.
type BattlePhase int

const (
	BattleNone         BattlePhase = 0
	BattlePlayer       BattlePhase = 1
	BattleWon          BattlePhase = 3
	BattleLost         BattlePhase = 4
	BattleAttackTiming BattlePhase = 5
	BattleEnemyTiming  BattlePhase = 6
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

// Action menu row indices. Owned here so both the battle input layer and the
// renderer reference one source of truth; reordering the rows is a one-place
// edit. ActionRowCount is the menu's wrap modulus.
const (
	ActionRowAttack = 0
	ActionRowSkill  = 1
	ActionRowDefend = 2
	ActionRowItem   = 3
	ActionRowCount  = 4
)

const (
	// BattleSplashDuration is how long the encounter banner sits on screen at
	// the start of a battle. The battle code seeds Battle.Splash with this and
	// the renderer uses it for ease-in/ease-out math, so they stay in sync.
	BattleSplashDuration = float32(1.15)

	AttackTimingDuration = float32(1.4)
	DefendTimingDuration = float32(1.3)
	// Press-window geometry, expressed as fractions of the bar's duration.
	// At construction time the window's *position* is randomized within
	// PressWindowMinStart..PressWindowMaxStart so two consecutive bars don't
	// land in the same place — but it never starts before
	// PressWindowMinStart, so the player can't get hit with a window that
	// opens immediately. Width is fixed; the sweet spot sits in its center.
	PressWindowMinStart   = float32(0.38)
	PressWindowMaxStart   = float32(0.62)
	PressWindowWidth      = float32(0.18)
	TimingFlashDuration   = float32(0.32)
	QualityResultDuration = float32(0.70)
	EnemyTurnIntro        = float32(0.85)
	// Charge bars get a longer pre-arm pause so the player has time to read
	// the prompt. The bar arms early if the player presses/holds the input,
	// see updateAttackTiming for the skip logic.
	AttackTimingIntro = float32(0.35)
	ChargeTimingIntro = float32(0.85)
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

	// Day/night cycle tuning. Six phases of StepsPerPhase player tile-steps
	// make up one full loop (StepsPerCycle). Only landed exploration steps
	// advance the cycle (battles don't tick it), so combat preserves the
	// phase the player walked into.
	StepsPerPhase = 25
	StepsPerCycle = StepsPerPhase * 6
)

// Damage / heal / defense multipliers for each timing-quality grade. Used
// by TimingBonusMult / TimingDefenseMult in timing.go. Pulled out here so
// balance tuning lives in one place; the timing module just dispatches.
const (
	TimingMultMiss      = float32(1.0)
	TimingMultNice      = float32(1.25)
	TimingMultGood      = float32(1.5)
	TimingMultGreat     = float32(1.75)
	TimingMultExcellent = float32(2.0)

	// Defense multipliers are <1 (lower incoming damage); Excellent quarters
	// the hit, Miss takes the full thing.
	TimingDefMiss      = float32(1.0)
	TimingDefNice      = float32(0.75)
	TimingDefGood      = float32(0.5)
	TimingDefGreat     = float32(0.35)
	TimingDefExcellent = float32(0.25)
)

// SkillID identifies a learned skill. Stored on Battle.PendingSkill and
// used as the map key for action handlers.
type SkillID int

const (
	SkillNone SkillID = iota
	SkillSwipe
	SkillPrayer
	SkillSteal
	SkillFirebolt
)

const (
	North = 0
	East  = 1
	South = 2
	West  = 3
)

var GameRNG = rand.New(rand.NewSource(time.Now().UnixNano()))

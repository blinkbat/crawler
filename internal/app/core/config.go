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
	AnimNone             = 0
	AnimStep             = 1
	AnimTurn             = 2
	BattleNone         = 0
	BattlePlayer       = 1
	BattleWon          = 3
	BattleLost         = 4
	BattleAttackTiming = 5
	BattleEnemyTiming  = 6
	ActionMenu           = 0
	ActionEnemyTarget    = 1
	ActionPartyTarget    = 2
	// Item picker: list of inventory entries to choose from.
	ActionItemMenu       = 3
	// Item target: pick which ally the chosen item is used on.
	ActionItemTarget     = 4

	// BattleSplashDuration is how long the encounter banner sits on screen at
	// the start of a battle. The battle code seeds Battle.Splash with this and
	// the renderer uses it for ease-in/ease-out math, so they stay in sync.
	BattleSplashDuration = float32(1.15)

	AttackTimingDuration  = float32(1.4)
	DefendTimingDuration  = float32(1.3)
	// Press-window geometry, expressed as fractions of the bar's duration.
	// At construction time the window's *position* is randomized within
	// PressWindowMinStart..PressWindowMaxStart so two consecutive bars don't
	// land in the same place — but it never starts before
	// PressWindowMinStart, so the player can't get hit with a window that
	// opens immediately. Width is fixed; the sweet spot sits in its center.
	PressWindowMinStart = float32(0.38)
	PressWindowMaxStart = float32(0.62)
	PressWindowWidth    = float32(0.18)
	TimingFlashDuration   = float32(0.32)
	QualityResultDuration = float32(0.70)
	EnemyTurnIntro        = float32(0.85)
	// Charge bars get a longer pre-arm pause so the player has time to read
	// the prompt. The bar arms early if the player presses/holds the input,
	// see updateAttackTiming for the skip logic.
	AttackTimingIntro     = float32(0.35)
	ChargeTimingIntro     = float32(0.85)
	BlockBumpDuration     = float32(0.22)

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
	SkillNone            = 0
	SkillSwipe           = 1
	SkillPrayer          = 2
	SkillSteal           = 3
	SkillFirebolt        = 4
	North                = 0
	East                 = 1
	South                = 2
	West                 = 3
)

var GameRNG = rand.New(rand.NewSource(time.Now().UnixNano()))

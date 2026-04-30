package core

import (
	"math/rand"
	"time"
)

const (
	ScreenWidth  = 1180
	ScreenHeight = 820

	TileSize          = 2.05
	WallHeight        = 2.25
	EyeHeight         = 1.32
	StepDuration      = 0.18
	TurnDuration      = 0.14
	BumpDuration      = 0.18
	FlashDuration     = 0.16
	DeathFadeDuration = 0.55
	MouseSense        = 0.0024
	MaxLookYaw        = 0.78
	MaxLookPitch      = 0.62
	StartTileX        = 1
	StartTileZ        = 1
	StartFacing       = East
	AnimNone          = 0
	AnimStep          = 1
	AnimTurn          = 2
	BattleNone        = 0
	BattlePlayer      = 1
	BattleEnemy       = 2
	BattleWon         = 3
	BattleLost        = 4
	ActionMenu        = 0
	ActionEnemyTarget = 1
	ActionPartyTarget = 2
	ActorParty        = 0
	ActorEnemy        = 1
	SkillNone         = 0
	SkillSwipe        = 1
	SkillPrayer       = 2
	SkillSteal        = 3
	SkillFirebolt     = 4
	RatMaxHP          = 10
	North             = 0
	East              = 1
	South             = 2
	West              = 3
)

var GameRNG = rand.New(rand.NewSource(time.Now().UnixNano()))

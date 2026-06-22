package core

import "fmt"

type EnemyCondition int

const (
	EnemyUnharmed EnemyCondition = iota
	EnemyScuffed
	EnemyInjured
	EnemyBadlyWounded
	EnemyNearDeath
	// EnemyConditionCount sizes the render-layer color table (enemyConditionColors).
	EnemyConditionCount = int(EnemyNearDeath) + 1
)

// woundBands: lower-bound HP fraction (strictly greater than) + label, ordered
// healthiest to most-wounded. EnemyConditionFor picks the first row HP% clears.
var woundBands = [...]struct {
	MinPercent float64
	Condition  EnemyCondition
	Label      string
}{
	{0.75, EnemyScuffed, "Scuffed"},
	{0.50, EnemyInjured, "Injured"},
	{0.25, EnemyBadlyWounded, "Badly Wounded"},
	{0.0, EnemyNearDeath, "Near Death"}, // fallthrough: anything > 0
}

// EnemyConditionFor takes *Enemy (not by value) to avoid copying the embedded
// EnemyDefinition per roster row per frame. Only reads HP/MaxHP.
func EnemyConditionFor(enemy *Enemy) EnemyCondition {
	if enemy.MaxHP <= 0 || enemy.HP >= enemy.MaxHP {
		return EnemyUnharmed
	}
	percent := float64(enemy.HP) / float64(enemy.MaxHP)
	for _, band := range woundBands {
		if percent > band.MinPercent {
			return band.Condition
		}
	}
	return EnemyNearDeath
}

func EnemyConditionLabel(condition EnemyCondition) string {
	for _, band := range woundBands {
		if band.Condition == condition {
			return band.Label
		}
	}
	return "Unharmed"
}

func init() {
	// woundBands must cover every condition except EnemyUnharmed (the HP==Max
	// early-return). Assert it grew with EnemyConditionCount, else a new
	// condition silently lacks a threshold and can never be returned.
	if len(woundBands) != EnemyConditionCount-1 {
		panic(fmt.Sprintf("core: woundBands has %d rows, expected EnemyConditionCount-1 (%d)", len(woundBands), EnemyConditionCount-1))
	}
}

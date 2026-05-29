package core

type EnemyCondition int

const (
	EnemyUnharmed EnemyCondition = iota
	EnemyScuffed
	EnemyInjured
	EnemyBadlyWounded
	EnemyNearDeath
	// EnemyConditionCount sizes the parallel color table in the render
	// layer (enemyConditionColors). Bump by adding a condition above.
	EnemyConditionCount = int(EnemyNearDeath) + 1
)

// woundBands is the per-condition descriptor: the lower-bound HP
// fraction that triggers the band (strictly greater than) plus the
// human-readable label. Bands are ordered top-down from healthiest to
// most-wounded — EnemyConditionFor walks the table and picks the first
// row whose threshold the actor's HP percent clears. Replaces the two
// parallel switches that hand-mirrored these thresholds and labels.
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

func EnemyConditionFor(enemy Enemy) EnemyCondition {
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

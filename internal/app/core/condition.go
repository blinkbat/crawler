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

func EnemyConditionFor(enemy Enemy) EnemyCondition {
	if enemy.MaxHP <= 0 || enemy.HP >= enemy.MaxHP {
		return EnemyUnharmed
	}
	percent := float64(enemy.HP) / float64(enemy.MaxHP)
	switch {
	case percent > 0.75:
		return EnemyScuffed
	case percent > 0.5:
		return EnemyInjured
	case percent > 0.25:
		return EnemyBadlyWounded
	default:
		return EnemyNearDeath
	}
}

func EnemyConditionLabel(condition EnemyCondition) string {
	switch condition {
	case EnemyScuffed:
		return "Scuffed"
	case EnemyInjured:
		return "Injured"
	case EnemyBadlyWounded:
		return "Badly Wounded"
	case EnemyNearDeath:
		return "Near Death"
	default:
		return "Unharmed"
	}
}

package core

import "testing"

// TestFleeChance_LevelDifferenceAndClamp pins the flee curve: even level = base,
// party advantage raises it, and the result clamps to [Floor, Cap] so escape is
// never guaranteed or impossible.
func TestFleeChance_LevelDifferenceAndClamp(t *testing.T) {
	if got := FleeChance(3, 3); got != BaseFleeChance {
		t.Errorf("FleeChance(even) = %v, want base %v", got, BaseFleeChance)
	}
	if FleeChance(6, 3) <= FleeChance(3, 3) {
		t.Errorf("party advantage did not raise flee chance: %v <= %v", FleeChance(6, 3), FleeChance(3, 3))
	}
	if FleeChance(3, 6) >= FleeChance(3, 3) {
		t.Errorf("party disadvantage did not lower flee chance: %v >= %v", FleeChance(3, 6), FleeChance(3, 3))
	}
	if got := FleeChance(100, 1); got != FleeCap {
		t.Errorf("FleeChance(huge advantage) = %v, want cap %v", got, FleeCap)
	}
	if got := FleeChance(1, 100); got != FleeFloor {
		t.Errorf("FleeChance(huge disadvantage) = %v, want floor %v", got, FleeFloor)
	}
}

// TestAverageLevels_LivingOnly confirms the level averages skip the dead so a
// near-wiped party / a pack with one straggler reads its survivors' levels.
func TestAverageLevels_LivingOnly(t *testing.T) {
	party := []PartyMember{
		{Level: 4, HP: 10},
		{Level: 2, HP: 0}, // dead — excluded
		{Level: 6, HP: 5},
	}
	if got := PartyAverageLevel(party); got != 5 { // (4+6)/2
		t.Errorf("PartyAverageLevel = %v, want 5", got)
	}
	pack := Pack{Members: []Enemy{
		{Alive: true},  // unauthored level → DefaultEnemyLevel
		{Alive: false}, // dead — excluded
	}}
	if got := PackAverageLevel(pack); got != float64(DefaultEnemyLevel) {
		t.Errorf("PackAverageLevel = %v, want %v", got, float64(DefaultEnemyLevel))
	}
}

// TestEnemyLevel_DefaultsWhenUnauthored guards the "default level, no per-row
// wiring" contract — an unauthored definition reads DefaultEnemyLevel.
func TestEnemyLevel_DefaultsWhenUnauthored(t *testing.T) {
	if got := EnemyLevel(NewEnemy(EnemyRat)); got != DefaultEnemyLevel {
		t.Errorf("EnemyLevel(rat) = %d, want default %d", got, DefaultEnemyLevel)
	}
}

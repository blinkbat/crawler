package core

import "testing"

// TestFleeChance_LevelDifferenceAndClamp: even level = base, advantage raises it, clamped to [Floor, Cap].
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

// TestAverageLevels_LivingOnly confirms the averages skip the dead.
func TestAverageLevels_LivingOnly(t *testing.T) {
	party := []PartyMember{
		{Level: 4, HP: 10},
		{Level: 2, HP: 0},
		{Level: 6, HP: 5},
	}
	if got := PartyAverageLevel(party); got != 5 { // (4+6)/2
		t.Errorf("PartyAverageLevel = %v, want 5", got)
	}
	rat := NewEnemy(EnemyRat) // Tier 1 == DefaultEnemyLevel
	pack := Pack{Members: []Enemy{
		rat,
		{Alive: false},
	}}
	if got := PackAverageLevel(pack); got != float64(EnemyLevel(&rat)) {
		t.Errorf("PackAverageLevel = %v, want %v", got, float64(EnemyLevel(&rat)))
	}
}

// TestEnemyLevel_ReadsTier: with no authored Level, EnemyLevel falls back to the
// foe's Tier so flee odds scale with pack threat (a higher-tier pack is harder to
// flee), not a uniform level 1.
func TestEnemyLevel_ReadsTier(t *testing.T) {
	bat := NewEnemy(EnemyBat) // Tier 2, no authored Level
	rat := NewEnemy(EnemyRat) // Tier 1
	if got, want := EnemyLevel(&bat), enemyGoverningDef(&bat).Tier; got != want {
		t.Errorf("EnemyLevel(bat) = %d, want its Tier %d", got, want)
	}
	if EnemyLevel(&bat) <= EnemyLevel(&rat) {
		t.Errorf("higher-tier Bat should out-level Tier-1 Rat for flee math")
	}
}

package core

import "testing"

// TestEnemyHitChance_DEXScalesAndClamps: DEX raises hit chance linearly,
// clamped to [Floor, Cap].
func TestEnemyHitChance_DEXScalesAndClamps(t *testing.T) {
	mid := EnemyHitChance(Stats{DEX: 3})
	if want := EnemyAccuracyBaseline + EnemyAccuracyPerDEX*float64(3); mid < want-1e-9 || mid > want+1e-9 {
		t.Errorf("EnemyHitChance(DEX 3) = %v, want ~%v", mid, want)
	}
	if EnemyHitChance(Stats{DEX: 6}) <= mid {
		t.Errorf("hit chance did not rise with DEX: DEX6 %v <= DEX3 %v", EnemyHitChance(Stats{DEX: 6}), mid)
	}
	if got := EnemyHitChance(Stats{DEX: 999}); got != EnemyAccuracyCap {
		t.Errorf("EnemyHitChance(DEX 999) = %v, want cap %v", got, EnemyAccuracyCap)
	}
	if got := EnemyHitChance(Stats{DEX: -50}); got != EnemyAccuracyFloor {
		t.Errorf("EnemyHitChance(DEX -50) = %v, want floor %v", got, EnemyAccuracyFloor)
	}
}

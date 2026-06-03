package core

import "testing"

// TestFlyingMeleePenalty verifies the ranged/flying scaffold: a melee swing
// at a flyer loses accuracy, a ranged weapon shrugs the penalty, and a
// ground target is unaffected either way.
func TestFlyingMeleePenalty(t *testing.T) {
	m := NewParty()[0]
	quality := int(TimingQualityGood)

	// Melee weapon: flyer accuracy is strictly below ground accuracy, and
	// ground accuracy is unchanged from the bare curve.
	m.Equipped[EquipRightHand] = ItemIronSword
	base := memberAttackAccuracy(m, quality)
	if vsGround := memberAttackAccuracyVs(m, false, quality); vsGround != base {
		t.Errorf("ground accuracy changed by the flying path: base %.3f vs %.3f", base, vsGround)
	}
	vsFlyer := memberAttackAccuracyVs(m, true, quality)
	if vsFlyer >= base {
		t.Errorf("melee accuracy vs flyer (%.3f) should be below ground (%.3f)", vsFlyer, base)
	}

	// Ranged weapon: flyer accuracy equals the bare curve (no penalty).
	m.Equipped[EquipRightHand] = ItemShortBow
	if got, want := memberAttackAccuracyVs(m, true, quality), memberAttackAccuracy(m, quality); got != want {
		t.Errorf("ranged accuracy vs flyer (%.3f) should equal base (%.3f)", got, want)
	}

	// The reach predicate the battle layer reads.
	if !CanReachFlying(WeaponBow) || CanReachFlying(WeaponSword) {
		t.Error("CanReachFlying: bow should reach a flyer, sword should not")
	}
}

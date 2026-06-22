package core

import "testing"

// TestFlyingMeleePenalty: melee vs a flyer loses accuracy, ranged shrugs it, ground is unaffected.
func TestFlyingMeleePenalty(t *testing.T) {
	m := NewParty()[0]
	quality := int(TimingQualityGood)

	// Melee: flyer accuracy is below ground; ground is unchanged from the bare curve.
	m.Equipped[EquipRightHand] = ItemIronSword
	base := memberAttackAccuracy(m, quality)
	if vsGround := memberAttackAccuracyVs(m, false, quality); vsGround != base {
		t.Errorf("ground accuracy changed by the flying path: base %.3f vs %.3f", base, vsGround)
	}
	vsFlyer := memberAttackAccuracyVs(m, true, quality)
	if vsFlyer >= base {
		t.Errorf("melee accuracy vs flyer (%.3f) should be below ground (%.3f)", vsFlyer, base)
	}

	// Ranged: flyer accuracy equals the bare curve (no penalty).
	m.Equipped[EquipRightHand] = ItemShortBow
	if got, want := memberAttackAccuracyVs(m, true, quality), memberAttackAccuracy(m, quality); got != want {
		t.Errorf("ranged accuracy vs flyer (%.3f) should equal base (%.3f)", got, want)
	}

	if !CanReachFlying(WeaponBow) || CanReachFlying(WeaponSword) {
		t.Error("CanReachFlying: bow should reach a flyer, sword should not")
	}
}

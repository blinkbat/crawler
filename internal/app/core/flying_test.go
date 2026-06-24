package core

import "testing"

// TestFlyingMeleeImmunity: a flyer is hard-unreachable by melee (no damage), reachable
// by a ranged weapon; ground foes are always melee-reachable.
func TestFlyingMeleeImmunity(t *testing.T) {
	if !CanReachFlying(WeaponBow) || CanReachFlying(WeaponSword) {
		t.Error("CanReachFlying: bow should reach a flyer, sword should not")
	}

	m := NewParty()[0]
	m.Equipped[EquipRightHand] = ItemIronSword
	if MemberMeleeReachesFlyer(m) {
		t.Error("a melee weapon must not reach a flyer")
	}
	m.Equipped[EquipRightHand] = ItemShortBow
	if !MemberMeleeReachesFlyer(m) {
		t.Error("a ranged weapon should reach a flyer")
	}

	// EnemyMeleeReachable: a front-row flyer is melee-immune; a front-row ground foe is reachable.
	members := []Enemy{
		{Alive: true, Row: RowFront, Kind: EnemyBat}, // Flying
		{Alive: true, Row: RowFront, Kind: EnemyRat}, // grounded
	}
	if !EnemyInfo(EnemyBat).Flying {
		t.Fatal("test assumes EnemyBat is Flying")
	}
	if EnemyMeleeReachable(members, 0) {
		t.Error("a Flying foe must not be melee-reachable even in the front row")
	}
	if !EnemyMeleeReachable(members, 1) {
		t.Error("a grounded front-row foe should be melee-reachable")
	}
}

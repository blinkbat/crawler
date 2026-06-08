package core

import "testing"

// TestEquippedWeaponHandPreference pins the right-then-left hand fallback in
// EquippedWeapon: a weapon in the right hand wins, a weapon in the left hand
// is used when the right hand holds no real weapon, and both hands empty
// (or holding non-weapons) stays unarmed (WeaponNone). Guards against the
// regression where EquippedWeapon read only the right hand and silently
// ignored a left-equipped weapon that CanEquipInSlot allows.
func TestEquippedWeaponHandPreference(t *testing.T) {
	memberWith := func(right, left ItemKind) PartyMember {
		var m PartyMember
		m.Equipped[EquipRightHand] = right
		m.Equipped[EquipLeftHand] = left
		return m
	}

	// ItemIronSword -> WeaponSword, ItemDagger -> WeaponDagger,
	// ItemWoodenShield -> WeaponNone (a non-weapon hand item).
	cases := []struct {
		name        string
		right, left ItemKind
		want        WeaponType
	}{
		{"right-hand weapon wins", ItemIronSword, ItemDagger, WeaponSword},
		{"right wins over empty left", ItemIronSword, ItemNone, WeaponSword},
		{"left used when right empty", ItemNone, ItemDagger, WeaponDagger},
		{"left used when right is a non-weapon shield", ItemWoodenShield, ItemDagger, WeaponDagger},
		{"unarmed when both empty", ItemNone, ItemNone, WeaponNone},
		{"unarmed when both non-weapons", ItemWoodenShield, ItemWoodenShield, WeaponNone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := EquippedWeapon(memberWith(tc.right, tc.left)); got != tc.want {
				t.Errorf("EquippedWeapon(right=%v,left=%v) = %v, want %v", tc.right, tc.left, got, tc.want)
			}
		})
	}
}

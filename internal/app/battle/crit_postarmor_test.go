package battle

import (
	"testing"

	"crawler/internal/app/core"
)

// TestDamageEnemyCrit_PostArmor: crit multiplies the MITIGATED damage (so a hit
// floored to 1 by armor still doubles to 2), and sets the popup crit flag. A
// non-crit hit takes the same armor floor without the multiply.
func TestDamageEnemyCrit_PostArmor(t *testing.T) {
	g := newTestState()
	e := &g.Packs[0].Members[0]
	e.Armor = 8
	e.MaxHP, e.HP = 100, 100

	// raw 3 vs armor 8 → mitigated floor of 1; crit ×CritMultiplier post-armor.
	dealt, _ := damageEnemyCrit(g, 0, 3, core.TimingQualityMiss, core.SkillTagPhys, true, false, false)
	if want := 1 * core.CritMultiplier; dealt != want {
		t.Fatalf("post-armor crit dealt %d, want %d (1 mitigated × %d)", dealt, want, core.CritMultiplier)
	}
	if !e.DamagePopupCrit {
		t.Error("crit popup flag not set on the struck enemy")
	}

	// Non-crit baseline: same armor floor, no multiply, no crit flag.
	e.HP = 100
	dealt, _ = damageEnemyCrit(g, 0, 3, core.TimingQualityMiss, core.SkillTagPhys, false, false, false)
	if dealt != 1 {
		t.Fatalf("non-crit floored hit dealt %d, want 1", dealt)
	}
	if e.DamagePopupCrit {
		t.Error("non-crit hit wrongly set the crit popup flag")
	}
}

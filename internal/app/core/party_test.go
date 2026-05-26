package core

import (
	"math/rand"
	"testing"
)

func TestMaxHPFor_TwoPerVIT(t *testing.T) {
	cases := []struct {
		vit  int
		want int
	}{
		{0, 0},
		{1, 2},
		{4, 8},
		{6, 12},
	}
	for _, tc := range cases {
		got := MaxHPFor(Stats{VIT: tc.vit})
		if got != tc.want {
			t.Errorf("MaxHPFor(VIT=%d) = %d, want %d", tc.vit, got, tc.want)
		}
	}
}

func TestMeleeDamage_AddsSTR(t *testing.T) {
	if got := MeleeDamage(Stats{STR: 6}, 0); got != 6 {
		t.Errorf("MeleeDamage(STR=6, 0) = %d, want 6", got)
	}
	if got := MeleeDamage(Stats{STR: 3}, 2); got != 5 {
		t.Errorf("MeleeDamage(STR=3, 2) = %d, want 5", got)
	}
}

func TestMagicDamage_AddsINT(t *testing.T) {
	if got := MagicDamage(Stats{INT: 6}, 1); got != 7 {
		t.Errorf("MagicDamage(INT=6, 1) = %d, want 7", got)
	}
}

func TestHealAmount_AddsWIS(t *testing.T) {
	if got := HealAmount(Stats{WIS: 6}, 1); got != 7 {
		t.Errorf("HealAmount(WIS=6, 1) = %d, want 7", got)
	}
}

func TestStealChance_ScalesByDEX(t *testing.T) {
	// base 0.40 × (1 + 6/20) = 0.40 × 1.30 = 0.52
	got := StealChance(Stats{DEX: 6}, 0.40)
	want := 0.52
	if absFloat(got-want) > 1e-9 {
		t.Errorf("StealChance(DEX=6, 0.40) = %v, want %v", got, want)
	}
}

func TestStealChance_CapsAtOne(t *testing.T) {
	// base 0.9, DEX 20 → 0.9 × 2.0 = 1.8, should clamp to 1.
	got := StealChance(Stats{DEX: 20}, 0.9)
	if got != 1 {
		t.Errorf("StealChance over 1 should clamp, got %v", got)
	}
}

func TestStealChance_ClampsNegativeToZero(t *testing.T) {
	// Pathological negative base — guard returns 0, not a negative chance.
	got := StealChance(Stats{DEX: 4}, -0.5)
	if got != 0 {
		t.Errorf("StealChance with negative base should clamp to 0, got %v", got)
	}
}

func TestSkillDamage_DispatchesByKind(t *testing.T) {
	warrior := Stats{STR: 6, INT: 1}
	wizard := Stats{STR: 1, INT: 6}
	// Swipe is melee (base 0) — warrior's STR=6 → 6 dmg, wizard's STR=1 → 1 dmg.
	if got := SkillDamage(warrior, SkillSwipe); got != 6 {
		t.Errorf("SkillDamage(warrior, Swipe) = %d, want 6", got)
	}
	if got := SkillDamage(wizard, SkillSwipe); got != 1 {
		t.Errorf("SkillDamage(wizard, Swipe) = %d, want 1", got)
	}
	// Firebolt is magic (base 1) — wizard's INT=6 → 7 dmg.
	if got := SkillDamage(wizard, SkillFirebolt); got != 7 {
		t.Errorf("SkillDamage(wizard, Firebolt) = %d, want 7", got)
	}
	// Steal is utility — no stat scaling.
	if got := SkillDamage(wizard, SkillSteal); got != 0 {
		t.Errorf("SkillDamage(*, Steal) = %d, want 0 (utility)", got)
	}
}

func TestSkillDamage_UnknownSkillReturnsZero(t *testing.T) {
	if got := SkillDamage(Stats{STR: 9}, SkillNone); got != 0 {
		t.Errorf("SkillDamage with SkillNone should return 0, got %d", got)
	}
}

func TestSkillHeal_OnlyHealKindAddsWIS(t *testing.T) {
	cleric := Stats{WIS: 6}
	// Prayer is heal kind, base 1 → 6 + 1 = 7.
	if got := SkillHeal(cleric, SkillPrayer); got != 7 {
		t.Errorf("SkillHeal(cleric, Prayer) = %d, want 7", got)
	}
	// Non-heal skills return their flat heal base (typically 0).
	if got := SkillHeal(cleric, SkillSwipe); got != 0 {
		t.Errorf("SkillHeal(*, Swipe) = %d, want 0", got)
	}
}

func TestSkillCost_MatchesRegistry(t *testing.T) {
	cases := map[SkillID]int{
		SkillSwipe:    2,
		SkillPrayer:   4,
		SkillSteal:    0,
		SkillFirebolt: 5,
	}
	for skill, want := range cases {
		if got := SkillCost(skill); got != want {
			t.Errorf("SkillCost(%v) = %d, want %d", skill, got, want)
		}
	}
}

func TestSkillName_KnownSkills(t *testing.T) {
	cases := map[SkillID]string{
		SkillSwipe:    "Swipe",
		SkillPrayer:   "Prayer",
		SkillSteal:    "Steal",
		SkillFirebolt: "Firebolt",
	}
	for skill, want := range cases {
		if got := SkillName(skill); got != want {
			t.Errorf("SkillName(%v) = %q, want %q", skill, got, want)
		}
	}
	if got := SkillName(SkillNone); got != "Skill" {
		t.Errorf("SkillName(SkillNone) fallback = %q, want %q", got, "Skill")
	}
}

func TestSkillTargetMode_MatchesRegistry(t *testing.T) {
	cases := map[SkillID]ActionMode{
		SkillSwipe:    ActionMenu,
		SkillPrayer:   ActionPartyTarget,
		SkillSteal:    ActionEnemyTarget,
		SkillFirebolt: ActionEnemyTarget,
	}
	for skill, want := range cases {
		if got := SkillTargetMode(skill); got != want {
			t.Errorf("SkillTargetMode(%v) = %v, want %v", skill, got, want)
		}
	}
}

func TestPartySkill_MatchesClass(t *testing.T) {
	cases := map[PartyClass]SkillID{
		ClassWarrior: SkillSwipe,
		ClassCleric:  SkillPrayer,
		ClassThief:   SkillSteal,
		ClassWizard:  SkillFirebolt,
	}
	for class, want := range cases {
		got := PartySkill(PartyMember{Class: class})
		if got != want {
			t.Errorf("PartySkill(class=%v) = %v, want %v", class, got, want)
		}
	}
}

func TestBurnDuration_WithinRange(t *testing.T) {
	rng := rand.New(rand.NewSource(13))
	effect := SkillEffect{BurnMinTurns: 2, BurnMaxTurns: 3}
	for i := 0; i < 50; i++ {
		d := effect.BurnDuration(rng)
		if d < 2 || d > 3 {
			t.Fatalf("BurnDuration out of range [2,3]: got %d", d)
		}
	}
}

func TestBurnDuration_InvertedReturnsZero(t *testing.T) {
	// Degenerate case: max < min returns 0 (no burn). This matches the
	// shared rollDuration semantics used by every other duration helper
	// (Sleep / Stun / Bind / Confuse / Poison) — the contract is "fail
	// open as no status" so a non-burn skill that picks up the
	// SkillEffect by accident can't roll a phantom DoT. Earlier the
	// test asserted "return min" on the inverted path, which made
	// BurnDuration the only helper with that behaviour; consolidating
	// onto rollDuration aligned the contract.
	e := SkillEffect{BurnMinTurns: 4, BurnMaxTurns: 2}
	if got := e.BurnDuration(nil); got != 0 {
		t.Errorf("BurnDuration on inverted range = %d, want 0", got)
	}
}

func TestBurnDuration_DegenerateMinZero(t *testing.T) {
	// min <= 0 also returns 0. Matches the shared rollDuration rule
	// — a non-burn skill picking up the effect won't accidentally
	// roll a status from a zero-base.
	e := SkillEffect{BurnMinTurns: 0, BurnMaxTurns: 0}
	if got := e.BurnDuration(nil); got != 0 {
		t.Errorf("BurnDuration on zero range = %d, want 0", got)
	}
}

func TestPartyClasses_DefensiveCopy(t *testing.T) {
	a := PartyClasses()
	b := PartyClasses()
	if len(a) != len(b) || len(a) == 0 {
		t.Fatalf("PartyClasses should return matching non-empty slices")
	}
	a[0].Name = "MUTATED"
	if b[0].Name == "MUTATED" {
		t.Errorf("PartyClasses returned aliased slice; mutating one leaked into another")
	}
}

func absFloat(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

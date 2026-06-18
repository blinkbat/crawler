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

func TestStealChance_PassesBaseThrough(t *testing.T) {
	// Steal no longer scales with DEX — the base passes through unchanged.
	got := StealChance(0.40)
	want := 0.40
	if absFloat(got-want) > 1e-9 {
		t.Errorf("StealChance(0.40) = %v, want %v", got, want)
	}
}

func TestStealChance_CapsAtOne(t *testing.T) {
	// A tier-augmented base over 1 should clamp to 1.
	got := StealChance(1.8)
	if got != 1 {
		t.Errorf("StealChance over 1 should clamp, got %v", got)
	}
}

func TestStealChance_ClampsNegativeToZero(t *testing.T) {
	// Pathological negative base — guard returns 0, not a negative chance.
	got := StealChance(-0.5)
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

func TestPartySkill_UnlearnedThenLearned(t *testing.T) {
	// New model: every member starts UNLEARNED — PartySkill returns
	// SkillNone until a granting node is bought, then resolves to the
	// learned skill at SkillCursor. (root node id → skill it grants.)
	cases := map[PartyClass]struct {
		node string
		want SkillID
	}{
		ClassWarrior: {"shield-bash", SkillCrushingBlow},
		ClassCleric:  {"smite", SkillSmite},
		ClassThief:   {"steal", SkillSteal},
		ClassWizard:  {"firebolt", SkillFirebolt},
	}
	for class, c := range cases {
		m := PartyMember{Class: class}
		if got := PartySkill(&m); got != SkillNone {
			t.Errorf("PartySkill(unlearned %v) = %v, want SkillNone", class, got)
		}
		m.SkillPoints = 1
		if !BuySkillNode(&m, c.node) {
			t.Fatalf("BuySkillNode(%v, %q) failed", class, c.node)
		}
		if got := PartySkill(&m); got != c.want {
			t.Errorf("PartySkill(learned %v) = %v, want %v", class, got, c.want)
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

// TestProjectXP_Cases pins the pure level-projection used by both AddXP and
// the victory spoils animation. Curve is geometric: XPForLevel(1)=100,
// (2)=200, (3)=400 (LevelXPBase=100, LevelXPRatio=2).
func TestProjectXP_Cases(t *testing.T) {
	cases := []struct {
		startLvl, startXP, added    int
		wantLvl, wantXP, wantGained int
		note                        string
	}{
		{1, 0, 50, 1, 50, 0, "no crossing"},
		{1, 0, 100, 2, 0, 1, "exact single crossing"},
		{1, 0, 150, 2, 50, 1, "single crossing with remainder"},
		{1, 0, 350, 3, 50, 2, "multi-level crossing (100+200, +50)"},
		{2, 150, 100, 3, 50, 1, "mid-level start crosses one"},
		{1, 50, 0, 1, 50, 0, "zero added is a no-op"},
		{1, 0, -10, 1, 0, 0, "negative added clamps to zero"},
		{0, 0, 0, 1, 0, 0, "sub-base start normalizes to BaseLevel"},
	}
	for _, tc := range cases {
		lvl, xp, gained := ProjectXP(tc.startLvl, tc.startXP, tc.added)
		if lvl != tc.wantLvl || xp != tc.wantXP || gained != tc.wantGained {
			t.Errorf("ProjectXP(%d,%d,%d) = (lvl %d, xp %d, gained %d); want (%d,%d,%d) — %s",
				tc.startLvl, tc.startXP, tc.added, lvl, xp, gained,
				tc.wantLvl, tc.wantXP, tc.wantGained, tc.note)
		}
	}
}

// TestAddXP_MatchesProjectXP locks AddXP's mutation to ProjectXP's pure
// result (so the spoils screen's animated preview and the real award can't
// drift) AND that each crossed level grants the right point payouts.
func TestAddXP_MatchesProjectXP(t *testing.T) {
	cases := []struct{ startLvl, startXP, amount int }{
		{1, 0, 50}, {1, 0, 150}, {1, 0, 350}, {2, 150, 100}, {1, 90, 1000},
	}
	for _, tc := range cases {
		wantLvl, wantXP, wantGained := ProjectXP(tc.startLvl, tc.startXP, tc.amount)
		m := PartyMember{Level: tc.startLvl, XP: tc.startXP, HP: 10}
		got := AddXP(&m, tc.amount)
		if got != wantGained {
			t.Errorf("AddXP(%d,%d,+%d) returned %d, want %d", tc.startLvl, tc.startXP, tc.amount, got, wantGained)
		}
		if m.Level != wantLvl || m.XP != wantXP {
			t.Errorf("AddXP(%d,%d,+%d) left (lvl %d, xp %d), want (%d,%d)",
				tc.startLvl, tc.startXP, tc.amount, m.Level, m.XP, wantLvl, wantXP)
		}
		if m.PendingLevelUps != wantGained*LevelStatPoints {
			t.Errorf("AddXP gained %d levels → PendingLevelUps %d, want %d", wantGained, m.PendingLevelUps, wantGained*LevelStatPoints)
		}
		if m.SkillPoints != wantGained*LevelSkillPoints {
			t.Errorf("AddXP gained %d levels → SkillPoints %d, want %d", wantGained, m.SkillPoints, wantGained*LevelSkillPoints)
		}
	}
}

// TestAddXP_DeadMemberNoop confirms a downed member earns nothing (the
// "living members get XP" rule the spoils snapshot relies on for GainedXP).
func TestAddXP_DeadMemberNoop(t *testing.T) {
	m := PartyMember{Level: 2, XP: 80, HP: 0}
	if got := AddXP(&m, 500); got != 0 || m.Level != 2 || m.XP != 80 || m.PendingLevelUps != 0 {
		t.Errorf("AddXP on downed member mutated state: gained %d, lvl %d, xp %d, pending %d", got, m.Level, m.XP, m.PendingLevelUps)
	}
}

// TestRestoreMP_ClampsAndReportsDelta locks the Magic Phial's MP-restore
// helper: it tops up toward MaxMP, returns the actual amount restored, and
// no-ops on a downed member (mirroring HealMember).
func TestRestoreMP_ClampsAndReportsDelta(t *testing.T) {
	m := &PartyMember{MP: 2, MaxMP: 10, HP: 5, MaxHP: 5}
	if got := RestoreMP(m, 5); got != 5 || m.MP != 7 {
		t.Fatalf("RestoreMP(5): delta %d, MP %d; want 5, 7", got, m.MP)
	}
	// Over-fill clamps at MaxMP and reports only what fit.
	if got := RestoreMP(m, 99); got != 3 || m.MP != 10 {
		t.Fatalf("RestoreMP overfill: delta %d, MP %d; want 3, 10", got, m.MP)
	}
	// Already full → no-op, zero delta.
	if got := RestoreMP(m, 5); got != 0 {
		t.Fatalf("RestoreMP at full should restore 0, got %d", got)
	}
	// A downed member can't drink.
	dead := &PartyMember{MP: 0, MaxMP: 10, HP: 0}
	if got := RestoreMP(dead, 5); got != 0 || dead.MP != 0 {
		t.Fatalf("downed RestoreMP should no-op, got delta %d MP %d", got, dead.MP)
	}
}

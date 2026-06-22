package core

import (
	"slices"
	"testing"
)

// TestNewHealSkillNodesGrant: Second Wind (Warrior root) + Renewal/Mass Mend (Cleric Mercy) learn.
func TestNewHealSkillNodesGrant(t *testing.T) {
	w := PartyMember{Class: ClassWarrior, SkillPoints: 1}
	if !BuySkillNode(&w, "second-wind") {
		t.Fatal("buy second-wind failed")
	}
	if !slices.Contains(LearnedSkills(&w), SkillSecondWind) {
		t.Error("second-wind node did not grant Second Wind")
	}

	// Mercy is a linear chain: prayer → cleanse → renewal → mass-mend.
	m := PartyMember{Class: ClassCleric, SkillPoints: 4}
	for _, id := range []string{"prayer", "cleanse", "renewal", "mass-mend"} {
		if !BuySkillNode(&m, id) {
			t.Fatalf("buy %s failed (prereq chain)", id)
		}
	}
	learned := LearnedSkills(&m)
	for _, sk := range []SkillID{SkillRenewal, SkillMassMend} {
		if !slices.Contains(learned, sk) {
			t.Errorf("Mercy chain did not grant %s (learned: %v)", SkillName(sk), learned)
		}
	}
}

// TestRenewalTierFolding checks the ladder: base, +1 turn (T1), +1 heal (T2), +1 turn (T3).
func TestRenewalTierFolding(t *testing.T) {
	m := PartyMember{Class: ClassCleric, SkillPoints: 6}
	BuySkillNode(&m, "prayer")
	BuySkillNode(&m, "cleanse")
	BuySkillNode(&m, "renewal")
	if base := EffectiveSkillEffect(&m, SkillRenewal); base.RegenTurns != RenewalRegenTurns || base.Heal != RenewalRegenBase {
		t.Fatalf("base effect = turns %d / heal %d, want %d / %d", base.RegenTurns, base.Heal, RenewalRegenTurns, RenewalRegenBase)
	}
	BuySkillNode(&m, "renewal") // tier 1: +1 turn
	if up := EffectiveSkillEffect(&m, SkillRenewal); up.RegenTurns != RenewalRegenTurns+1 {
		t.Errorf("tier-1 RegenTurns = %d, want %d", up.RegenTurns, RenewalRegenTurns+1)
	}
	BuySkillNode(&m, "renewal") // tier 2: +1 per-turn heal
	BuySkillNode(&m, "renewal") // tier 3: +1 turn
	maxed := EffectiveSkillEffect(&m, SkillRenewal)
	if maxed.RegenTurns != RenewalRegenTurns+2 {
		t.Errorf("maxed RegenTurns = %d, want %d", maxed.RegenTurns, RenewalRegenTurns+2)
	}
	if maxed.Heal != RenewalRegenBase+1 {
		t.Errorf("maxed per-turn Heal = %d, want %d", maxed.Heal, RenewalRegenBase+1)
	}
}

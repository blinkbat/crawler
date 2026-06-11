package core

import (
	"slices"
	"testing"
)

// TestNewHealSkillNodesGrant verifies the three newly-wired support nodes learn
// their skills: Second Wind (Warrior root), and Renewal + Mass Mend (Cleric
// Mercy nodes reached after Prayer).
func TestNewHealSkillNodesGrant(t *testing.T) {
	w := PartyMember{Class: ClassWarrior, SkillPoints: 1}
	if !BuySkillNode(&w, "second-wind") {
		t.Fatal("buy second-wind failed")
	}
	if !slices.Contains(LearnedSkills(&w), SkillSecondWind) {
		t.Error("second-wind node did not grant Second Wind")
	}

	// The Cleric Mercy tree is a linear chain: prayer → cleanse → renewal →
	// mass-mend, so reaching renewal/mass-mend means ranking the nodes before
	// them. Walk the whole chain, then assert both heal skills are learned.
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

// TestRenewalTierFolding checks the Renewal ladder folds correctly: base
// duration, +1 turn at T1, +1 per-turn heal at T2, +1 turn at T3.
func TestRenewalTierFolding(t *testing.T) {
	m := PartyMember{Class: ClassCleric, SkillPoints: 6}
	BuySkillNode(&m, "prayer")  // Mercy root
	BuySkillNode(&m, "cleanse") // renewal's prerequisite in the linear chain
	BuySkillNode(&m, "renewal") // rank 1 = tier 0 (base)
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

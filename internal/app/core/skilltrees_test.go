package core

import "testing"

// TestLearnedSkills_FreshMemberEmpty: a fresh member (no ranks) has learned nothing.
func TestLearnedSkills_FreshMemberEmpty(t *testing.T) {
	for _, c := range []PartyClass{ClassWarrior, ClassCleric, ClassThief, ClassWizard} {
		m := PartyMember{Class: c}
		if got := LearnedSkills(&m); len(got) != 0 {
			t.Errorf("LearnedSkills(fresh %v) = %v, want empty", c, got)
		}
	}
}

// TestNewParty_SeedsOneSkillPointUnlearned: each member starts with one SkillPoint, zero learned.
func TestNewParty_SeedsOneSkillPointUnlearned(t *testing.T) {
	for _, m := range NewParty() {
		if m.SkillPoints != 1 {
			t.Errorf("%s starts with %d SkillPoints, want 1", m.Name, m.SkillPoints)
		}
		if got := LearnedSkills(&m); len(got) != 0 {
			t.Errorf("%s starts knowing %v, want nothing learned", m.Name, got)
		}
	}
}

// TestBuySkillNode_LearnsThenUpgradesLadder walks a granting root up its ladder: rank 1 learns at
// tier 0, each further rank advances SkillTiers, and a maxed node lands on MaxSkillTier and refuses more.
func TestBuySkillNode_LearnsThenUpgradesLadder(t *testing.T) {
	m := PartyMember{Class: ClassWizard, SkillPoints: MaxSkillTier + 1}

	if !BuySkillNode(&m, "firebolt") {
		t.Fatal("buy firebolt rank 1 failed")
	}
	if got := LearnedSkills(&m); len(got) != 1 || got[0] != SkillFirebolt {
		t.Fatalf("after rank 1 LearnedSkills = %v, want [Firebolt]", got)
	}
	if got := SkillTierOf(&m, SkillFirebolt); got != 0 {
		t.Errorf("tier after rank 1 = %d, want 0 (base)", got)
	}
	base := EffectiveSkillEffect(&m, SkillFirebolt)

	if !BuySkillNode(&m, "firebolt") {
		t.Fatal("buy firebolt rank 2 failed")
	}
	if got := SkillTierOf(&m, SkillFirebolt); got != 1 {
		t.Errorf("tier after rank 2 = %d, want 1", got)
	}
	if up := EffectiveSkillEffect(&m, SkillFirebolt); up.Damage != base.Damage+2 {
		t.Errorf("Firebolt tier-1 damage = %d, want base+2 (%d)", up.Damage, base.Damage+2)
	}

	BuySkillNode(&m, "firebolt")
	BuySkillNode(&m, "firebolt")
	if got := SkillTierOf(&m, SkillFirebolt); got != MaxSkillTier {
		t.Errorf("tier at max rank = %d, want MaxSkillTier (%d)", got, MaxSkillTier)
	}
	if got := TreeNodeRank(&m, "firebolt"); got != MaxSkillTier+1 {
		t.Errorf("node rank at max = %d, want %d", got, MaxSkillTier+1)
	}
	if m.SkillPoints != 0 {
		t.Errorf("SkillPoints after maxing = %d, want 0", m.SkillPoints)
	}
	if BuySkillNode(&m, "firebolt") {
		t.Error("BuySkillNode succeeded on a maxed node")
	}
}

// TestBuySkillNode_PassiveNodeGrantsNoSkill: a GrantSkill==SkillNone node records its rank but must
// not change LearnedSkills or the SkillTiers map. Uses passive `bloodthirst` after granting
// `cleave`→`rend` (Warrior Fury) — searing-light is now an active granting node.
func TestBuySkillNode_PassiveNodeGrantsNoSkill(t *testing.T) {
	m := PartyMember{Class: ClassWarrior, SkillPoints: 3}
	if !BuySkillNode(&m, "cleave") { // grants Swipe (root)
		t.Fatal("buy cleave failed")
	}
	if !BuySkillNode(&m, "rend") { // grants Rend (requires cleave)
		t.Fatal("buy rend failed")
	}
	learnedBefore := len(LearnedSkills(&m))
	tiersBefore := len(m.SkillTiers)

	if !BuySkillNode(&m, "bloodthirst") { // passive, grants nothing (requires rend)
		t.Fatal("buy bloodthirst failed")
	}
	if got := len(LearnedSkills(&m)); got != learnedBefore {
		t.Errorf("passive node changed LearnedSkills count %d -> %d", learnedBefore, got)
	}
	if len(m.SkillTiers) != tiersBefore {
		t.Errorf("passive node wrote a SkillTiers entry: %v", m.SkillTiers)
	}
	if got := TreeNodeRank(&m, "bloodthirst"); got != 1 {
		t.Errorf("bloodthirst rank = %d, want 1", got)
	}
}

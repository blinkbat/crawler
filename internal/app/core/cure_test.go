package core

import (
	"slices"
	"testing"
)

// TestCureDebuffs_ClearsDebuffsKeepsBuffAndDefend: removes the five debuffs, keeps Bless + Defending.
func TestCureDebuffs_ClearsDebuffsKeepsBuffAndDefend(t *testing.T) {
	m := PartyMember{
		HP: 5, PoisonTurns: 2, SleepTurns: 1, StunTurns: 1, WebbedTurns: 3, ConfusedTurns: 2,
		Defending: true,
	}
	StampPartyBuff(&m, SkillBless, SkillEffect{BuffStats: Stats{STR: 1}, BuffTurns: 3})
	if cured := CureDebuffs(&m); cured != 5 {
		t.Errorf("cured %d, want 5", cured)
	}
	if m.PoisonTurns != 0 || m.SleepTurns != 0 || m.StunTurns != 0 || m.WebbedTurns != 0 || m.ConfusedTurns != 0 {
		t.Errorf("debuffs not all cleared: %+v", m)
	}
	if !m.Defending {
		t.Error("Cleanse wrongly stripped the Defending stance")
	}
	if len(m.Buffs) != 1 || m.Buffs[0].Source != SkillBless {
		t.Errorf("Cleanse wrongly stripped the Bless buff: %+v", m.Buffs)
	}

	clean := PartyMember{HP: 5}
	if got := CureDebuffs(&clean); got != 0 {
		t.Errorf("clean member cured = %d, want 0", got)
	}
	if got := CureDebuffs(nil); got != 0 { // nil-safe
		t.Errorf("nil member cured = %d, want 0", got)
	}
}

// TestNewSkillTreeNodesGrant: granting nodes learn their skill once root + node are ranked.
func TestNewSkillTreeNodesGrant(t *testing.T) {
	cases := []struct {
		class      PartyClass
		root, node string
		skill      SkillID
	}{
		{ClassWizard, "firebolt", "fireball", SkillFireball},
		{ClassThief, "venom-strike", "poison-cloud", SkillPoisonCloud},
		{ClassCleric, "prayer", "cleanse", SkillCleanse},
	}
	for _, c := range cases {
		m := PartyMember{Class: c.class, SkillPoints: 4}
		if !BuySkillNode(&m, c.root) {
			t.Fatalf("%v: buying root %q failed", c.class, c.root)
		}
		if !BuySkillNode(&m, c.node) {
			t.Fatalf("%v: buying node %q failed (prereq not satisfied?)", c.class, c.node)
		}
		if !slices.Contains(LearnedSkills(&m), c.skill) {
			t.Errorf("%v: after ranking %q, LearnedSkills lacks %s", c.class, c.node, SkillName(c.skill))
		}
	}
}

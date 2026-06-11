package core

import "testing"

// TestBlessRegistryShape pins the Cleric buff's registry contract: it must be
// player-castable, Buff-tagged (the first SkillTagBuff user), flagged
// AppliesAOEPartyBuff, and carry the base magnitude/duration the tier ladder
// builds on — buffing the four offensive/support stats while leaving VIT and
// SPD alone (VIT would desync MaxHP; SPD would perturb the ATB turn order).
func TestBlessRegistryShape(t *testing.T) {
	if !SkillPlayerCastable(SkillBless) {
		t.Error("Bless must be PlayerCastable")
	}
	if got := SkillTagFor(SkillBless); got != SkillTagBuff {
		t.Errorf("Bless tag = %v, want SkillTagBuff", got)
	}
	eff := SkillEffectFor(SkillBless)
	if !eff.AppliesAOEPartyBuff {
		t.Error("Bless must set AppliesAOEPartyBuff (whole-party apply)")
	}
	if eff.BuffTurns != BlessBuffTurns {
		t.Errorf("Bless base BuffTurns = %d, want %d", eff.BuffTurns, BlessBuffTurns)
	}
	for name, got := range map[string]int{"STR": eff.BuffStats.STR, "DEX": eff.BuffStats.DEX, "INT": eff.BuffStats.INT, "WIS": eff.BuffStats.WIS} {
		if got != BlessBuffPerStat {
			t.Errorf("Bless base %s buff = %d, want %d", name, got, BlessBuffPerStat)
		}
	}
	if eff.BuffStats.VIT != 0 || eff.BuffStats.SPD != 0 {
		t.Errorf("Bless must not buff VIT/SPD, got VIT=%d SPD=%d", eff.BuffStats.VIT, eff.BuffStats.SPD)
	}
}

// TestEffectiveStats_FoldsActiveBuff verifies the buff is folded into the
// effective stat sheet only while the counter runs, per-stat, and never
// touches the stats Bless deliberately omits.
func TestEffectiveStats_FoldsActiveBuff(t *testing.T) {
	m := PartyMember{Stats: Stats{STR: 3, DEX: 2, INT: 1, WIS: 4, VIT: 5, SPD: 3}}
	m.BuffStats = Stats{STR: 2, DEX: 2, INT: 2, WIS: 2}

	// Counter at 0: the buff is inert, EffectiveStats == base.
	m.BuffTurns = 0
	if got := EffectiveStats(m); got != m.Stats {
		t.Errorf("inactive buff leaked: EffectiveStats = %+v, want base %+v", got, m.Stats)
	}

	// Counter running: each declared stat lifts, the rest stay put.
	m.BuffTurns = 2
	got := EffectiveStats(m)
	if got.STR != 5 || got.DEX != 4 || got.INT != 3 || got.WIS != 6 {
		t.Errorf("active buff fold = %+v, want STR5 DEX4 INT3 WIS6", got)
	}
	if got.VIT != 5 || got.SPD != 3 {
		t.Errorf("buff perturbed VIT/SPD: %+v", got)
	}
}

// TestClearPartyTransientStatuses_ClearsBuff guards that the buff is combat-
// only — both the counter and the magnitude are wiped on battle exit.
func TestClearPartyTransientStatuses_ClearsBuff(t *testing.T) {
	party := []PartyMember{{HP: 5, BuffTurns: 3, BuffStats: Stats{STR: 2, WIS: 1}}}
	ClearPartyTransientStatuses(party)
	if party[0].BuffTurns != 0 || party[0].BuffStats != (Stats{}) {
		t.Errorf("buff survived battle exit: turns=%d stats=%+v", party[0].BuffTurns, party[0].BuffStats)
	}
}

// TestPartyStatus_BlessedPrecedence checks the buff surfaces as the lone
// positive counted status, but any real threat (here Poison) outranks it so
// the player still sees the danger.
func TestPartyStatus_BlessedPrecedence(t *testing.T) {
	if kind, turns := PartyStatus(PartyMember{HP: 5, BuffTurns: 3}); kind != PartyStatusBlessed || turns != 3 {
		t.Errorf("blessed member status = (%v,%d), want (Blessed,3)", kind, turns)
	}
	if kind, _ := PartyStatus(PartyMember{HP: 5, BuffTurns: 3, PoisonTurns: 2}); kind != PartyStatusPoisoned {
		t.Errorf("poison+buff status = %v, want Poisoned (threat outranks buff)", kind)
	}
	if got := PartyStatusLabel(PartyStatusBlessed); got != "BLESSED" {
		t.Errorf("Blessed label = %q, want BLESSED", got)
	}
}

// TestBlessingNode_GrantsAndUpgradesBless walks the Conviction tree's blessing
// root: rank 1 learns Bless at its base effect, and further ranks climb the
// tier ladder (+1 turn, then +1/+1 to every blessed stat).
func TestBlessingNode_GrantsAndUpgradesBless(t *testing.T) {
	m := PartyMember{Class: ClassCleric, SkillPoints: MaxSkillTier + 1}

	if !BuySkillNode(&m, "blessing") {
		t.Fatal("buy blessing rank 1 failed")
	}
	if learned := LearnedSkills(&m); len(learned) != 1 || learned[0] != SkillBless {
		t.Fatalf("after rank 1 LearnedSkills = %v, want [Bless]", learned)
	}
	if base := EffectiveSkillEffect(&m, SkillBless); base.BuffTurns != BlessBuffTurns || base.BuffStats.STR != BlessBuffPerStat {
		t.Fatalf("rank-1 base effect = %+v, want turns %d / per-stat %d", base, BlessBuffTurns, BlessBuffPerStat)
	}

	// Rank 2 = tier 1 (+1 turn, magnitude unchanged).
	BuySkillNode(&m, "blessing")
	if up := EffectiveSkillEffect(&m, SkillBless); up.BuffTurns != BlessBuffTurns+1 || up.BuffStats.STR != BlessBuffPerStat {
		t.Errorf("tier-1 effect = %+v, want turns %d / per-stat %d", up, BlessBuffTurns+1, BlessBuffPerStat)
	}

	// Ranks 3 & 4 = tiers 2 & 3 (+1 each stat apiece); maxed caps at MaxSkillTier.
	BuySkillNode(&m, "blessing")
	BuySkillNode(&m, "blessing")
	maxed := EffectiveSkillEffect(&m, SkillBless)
	if maxed.BuffStats.STR != BlessBuffPerStat+2 || maxed.BuffStats.WIS != BlessBuffPerStat+2 {
		t.Errorf("maxed per-stat = STR%d/WIS%d, want %d", maxed.BuffStats.STR, maxed.BuffStats.WIS, BlessBuffPerStat+2)
	}
	if maxed.BuffTurns != BlessBuffTurns+1 {
		t.Errorf("maxed BuffTurns = %d, want %d", maxed.BuffTurns, BlessBuffTurns+1)
	}
}

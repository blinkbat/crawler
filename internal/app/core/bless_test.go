package core

import "testing"

// TestBlessRegistryShape pins the registry contract; buffs STR/DEX/INT/WIS but not VIT/SPD (VIT desyncs MaxHP, SPD perturbs ATB).
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

// TestEffectiveStats_FoldsActiveBuff: the buff folds in only while the counter runs, per-stat.
func TestEffectiveStats_FoldsActiveBuff(t *testing.T) {
	m := PartyMember{Stats: Stats{STR: 3, DEX: 2, INT: 1, WIS: 4, VIT: 5, SPD: 3}}

	if got := EffectiveStats(m); got != m.Stats { // no buff: == base
		t.Errorf("inactive buff leaked: EffectiveStats = %+v, want base %+v", got, m.Stats)
	}

	StampPartyBuff(&m, SkillBless, SkillEffect{BuffStats: Stats{STR: 2, DEX: 2, INT: 2, WIS: 2}, BuffTurns: 2})
	got := EffectiveStats(m)
	if got.STR != 5 || got.DEX != 4 || got.INT != 3 || got.WIS != 6 {
		t.Errorf("active buff fold = %+v, want STR5 DEX4 INT3 WIS6", got)
	}
	if got.VIT != 5 || got.SPD != 3 {
		t.Errorf("buff perturbed VIT/SPD: %+v", got)
	}
}

// TestEffectiveStats_StacksMultipleBuffs: different skills' buffs SUM; re-casting the same one refreshes.
func TestEffectiveStats_StacksMultipleBuffs(t *testing.T) {
	m := PartyMember{Stats: Stats{STR: 3, DEX: 2}}
	StampPartyBuff(&m, SkillBless, SkillEffect{BuffStats: Stats{STR: 1, DEX: 1}, BuffTurns: 3})
	StampPartyBuff(&m, SkillWarBanner, SkillEffect{BuffStats: Stats{STR: 2}, BuffArmor: 2, BuffTurns: 4})

	if got := EffectiveStats(m); got.STR != 6 || got.DEX != 3 { // STR 3+1+2, DEX 2+1
		t.Errorf("stacked stats = STR%d/DEX%d, want STR6/DEX3", got.STR, got.DEX)
	}
	if a := EffectiveArmor(m); a != 2 {
		t.Errorf("War Banner armor not folded alongside Bless: EffectiveArmor = %d, want 2", a)
	}
	StampPartyBuff(&m, SkillBless, SkillEffect{BuffStats: Stats{STR: 1, DEX: 1}, BuffTurns: 3}) // re-cast: refresh
	if got := EffectiveStats(m); got.STR != 6 {
		t.Errorf("re-cast double-stacked: STR = %d, want 6", got.STR)
	}
}

// TestEffectiveMDef_FoldsBuffWIS pins that MDef reads EFFECTIVE WIS (not raw m.Stats); MDefBonus (Stone Skin) stacks on top.
func TestEffectiveMDef_FoldsBuffWIS(t *testing.T) {
	m := PartyMember{Stats: Stats{WIS: 4}}
	if got := EffectiveMDef(m); got != 4 { // MagicDefense == WIS
		t.Fatalf("base EffectiveMDef = %d, want 4 (raw WIS)", got)
	}

	StampPartyBuff(&m, SkillBless, SkillEffect{BuffStats: Stats{WIS: 3}, BuffTurns: 3})
	if got := EffectiveMDef(m); got != 7 {
		t.Errorf("buffed EffectiveMDef = %d, want 7 (effective WIS 4+3)", got)
	}

	StampPartyBuff(&m, SkillStoneSkin, SkillEffect{BuffMDef: 2, BuffTurns: 3})
	if got := EffectiveMDef(m); got != 9 {
		t.Errorf("EffectiveMDef with buff MDef = %d, want 9 (7 + 2 MDefBonus)", got)
	}

	if _, mdef := EffectiveDefenses(m); mdef != 9 { // parity with EffectiveMDef
		t.Errorf("EffectiveDefenses mdef = %d, want 9 (parity with EffectiveMDef)", mdef)
	}
}

// TestClearPartyTransientStatuses_ClearsBuff: buffs are combat-only, wiped on battle exit.
func TestClearPartyTransientStatuses_ClearsBuff(t *testing.T) {
	party := []PartyMember{{HP: 5}}
	StampPartyBuff(&party[0], SkillBless, SkillEffect{BuffStats: Stats{STR: 2, WIS: 1}, BuffTurns: 3})
	ClearPartyTransientStatuses(party)
	if len(party[0].Buffs) != 0 {
		t.Errorf("buff survived battle exit: %+v", party[0].Buffs)
	}
}

// TestPartyStatus_BlessedPrecedence: a buff surfaces as positive, but a threat (Poison) outranks it.
func TestPartyStatus_BlessedPrecedence(t *testing.T) {
	blessed := PartyMember{HP: 5}
	StampPartyBuff(&blessed, SkillBless, SkillEffect{BuffStats: Stats{STR: 1}, BuffTurns: 3})
	if kind, turns := PartyStatus(&blessed); kind != PartyStatusBlessed || turns != 3 {
		t.Errorf("blessed member status = (%v,%d), want (Blessed,3)", kind, turns)
	}
	blessed.PoisonTurns = 2
	if kind, _ := PartyStatus(&blessed); kind != PartyStatusPoisoned {
		t.Errorf("poison+buff status = %v, want Poisoned (threat outranks buff)", kind)
	}
	if got := PartyStatusLabel(PartyStatusBlessed); got != "BLESSED" {
		t.Errorf("Blessed label = %q, want BLESSED", got)
	}
}

// TestBlessingNode_GrantsAndUpgradesBless: rank 1 learns Bless at base, further ranks climb the tier ladder.
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

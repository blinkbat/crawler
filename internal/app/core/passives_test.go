package core

import (
	"math"
	"testing"
)

// TestMemberCritChance_LuckyStrikeAddsPerRank locks the Lucky Strike passive
// onto the crit curve: each rank adds LuckyStrikeCritPerRank on top of the
// DEX/timing baseline, and the result still respects CritCap.
func TestMemberCritChance_LuckyStrikeAddsPerRank(t *testing.T) {
	thief := PartyMember{Class: ClassThief, Stats: Stats{DEX: 6}}
	base := MemberCritChance(thief, TimingQualityMiss)

	thief.TreeRanks = map[string]int{PassiveLuckyStrike: 3}
	withRanks := MemberCritChance(thief, TimingQualityMiss)

	wantDelta := 3 * LuckyStrikeCritPerRank
	if got := withRanks - base; math.Abs(got-wantDelta) > 1e-9 {
		t.Errorf("Lucky Strike delta = %.4f, want %.4f", got, wantDelta)
	}
	if withRanks > CritCap {
		t.Errorf("MemberCritChance %.4f exceeds CritCap %.4f", withRanks, CritCap)
	}
}

// TestMemberCritChance_RespectsCap guards the re-clamp: a high-DEX member on the
// top grade whose base already sits near the cap can't be pushed past CritCap by
// stacking Lucky Strike ranks.
func TestMemberCritChance_RespectsCap(t *testing.T) {
	m := PartyMember{Class: ClassThief, Stats: Stats{DEX: 99}, TreeRanks: map[string]int{PassiveLuckyStrike: 3}}
	if got := MemberCritChance(m, TimingQualityMiss); got > CritCap {
		t.Errorf("MemberCritChance = %.4f, want <= CritCap %.4f", got, CritCap)
	}
}

// TestMemberCritChance_ReadsEffectiveDEX pins that the member-aware crit curve
// reads EFFECTIVE DEX (base + gear + buffs), not raw m.Stats — so a +DEX buff
// lifts crit the same way gear does. Guards against a regression that swaps
// EffectiveStats(m) back to m.Stats inside MemberCritChance (which the bare-stat
// tests above can't catch).
func TestMemberCritChance_ReadsEffectiveDEX(t *testing.T) {
	thief := PartyMember{Class: ClassThief, Stats: Stats{DEX: 6}}
	base := MemberCritChance(thief, TimingQualityMiss)

	const buffDEX = 5
	StampPartyBuff(&thief, SkillBless, SkillEffect{BuffStats: Stats{DEX: buffDEX}, BuffTurns: 3})
	withBuff := MemberCritChance(thief, TimingQualityMiss)

	wantDelta := buffDEX * CritPerDEX
	if got := withBuff - base; math.Abs(got-wantDelta) > 1e-9 {
		t.Errorf("buff-DEX crit delta = %.4f, want %.4f (MemberCritChance must read effective DEX)", got, wantDelta)
	}
}

// TestPassiveRank_UnlearnedReadsZero confirms the nil-safe / wrong-class read:
// a member who never bought the node (or carries no TreeRanks at all) reports 0,
// so the battle hooks no-op without a class guard.
func TestPassiveRank_UnlearnedReadsZero(t *testing.T) {
	var fresh PartyMember
	if got := PassiveRank(&fresh, PassiveBloodthirst); got != 0 {
		t.Errorf("fresh member rank = %d, want 0", got)
	}
	cleric := PartyMember{Class: ClassCleric, TreeRanks: map[string]int{PassiveRetribution: 2}}
	if got := PassiveRank(&cleric, PassiveRiposte); got != 0 {
		t.Errorf("cleric riposte rank = %d, want 0 (warrior node)", got)
	}
	if got := PassiveRank(&cleric, PassiveRetribution); got != 2 {
		t.Errorf("cleric retribution rank = %d, want 2", got)
	}
}

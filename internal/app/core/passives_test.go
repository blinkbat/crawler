package core

import (
	"math"
	"testing"
)

// TestMemberCritChance_LuckyStrikeAddsPerRank: each rank adds LuckyStrikeCritPerRank, capped at CritCap.
func TestMemberCritChance_LuckyStrikeAddsPerRank(t *testing.T) {
	thief := PartyMember{Class: ClassThief, Stats: Stats{DEX: 6}}
	base := MemberCritChance(&thief, TimingQualityMiss)

	thief.TreeRanks = map[string]int{PassiveLuckyStrike: 3}
	withRanks := MemberCritChance(&thief, TimingQualityMiss)

	wantDelta := 3 * LuckyStrikeCritPerRank
	if got := withRanks - base; math.Abs(got-wantDelta) > 1e-9 {
		t.Errorf("Lucky Strike delta = %.4f, want %.4f", got, wantDelta)
	}
	if withRanks > CritCap {
		t.Errorf("MemberCritChance %.4f exceeds CritCap %.4f", withRanks, CritCap)
	}
}

// TestMemberCritChance_RespectsCap: Lucky Strike ranks can't push past CritCap.
func TestMemberCritChance_RespectsCap(t *testing.T) {
	m := PartyMember{Class: ClassThief, Stats: Stats{DEX: 99}, TreeRanks: map[string]int{PassiveLuckyStrike: 3}}
	if got := MemberCritChance(&m, TimingQualityMiss); got > CritCap {
		t.Errorf("MemberCritChance = %.4f, want <= CritCap %.4f", got, CritCap)
	}
}

// TestMemberCritChance_ReadsEffectiveDEX: the crit curve reads EFFECTIVE DEX, not raw m.Stats.
func TestMemberCritChance_ReadsEffectiveDEX(t *testing.T) {
	thief := PartyMember{Class: ClassThief, Stats: Stats{DEX: 6}}
	base := MemberCritChance(&thief, TimingQualityMiss)

	const buffDEX = 5
	StampPartyBuff(&thief, SkillBless, SkillEffect{BuffStats: Stats{DEX: buffDEX}, BuffTurns: 3})
	withBuff := MemberCritChance(&thief, TimingQualityMiss)

	wantDelta := buffDEX * CritPerDEX
	if got := withBuff - base; math.Abs(got-wantDelta) > 1e-9 {
		t.Errorf("buff-DEX crit delta = %.4f, want %.4f (MemberCritChance must read effective DEX)", got, wantDelta)
	}
}

// TestPassiveRank_UnlearnedReadsZero: nil-safe / wrong-class reads report 0.
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

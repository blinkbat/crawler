package core

import "math/rand"

// Passive skill-tree nodes carry no GrantSkill — the battle pipeline reads the
// invested RANK and folds a per-rank effect into combat (init guard below catches
// drift from skilltrees.go). Hooks: Riposte=counter a dodged strike,
// Bloodthirst=heal a share of phys damage, Retribution=reflect damage taken,
// Shadow Step=bonus damage acting first, Lucky Strike=added crit.
const (
	PassiveRiposte     = "riposte"
	PassiveBloodthirst = "bloodthirst"
	PassiveRetribution = "retribution"
	PassiveShadowStep  = "shadow-step"
	PassiveLuckyStrike = "lucky-strike"
)

// passiveNodeIDs is the canonical list the init guard walks.
var passiveNodeIDs = []string{
	PassiveRiposte,
	PassiveBloodthirst,
	PassiveRetribution,
	PassiveShadowStep,
	PassiveLuckyStrike,
}

// init asserts every passive id resolves to a real tree node granting NO castable
// skill (a typo would read rank 0 forever; a GrantSkill node would double-fire).
func init() {
	for _, id := range passiveNodeIDs {
		found := false
		for _, c := range AllPartyClasses() {
			n, ok := findTreeNode(c, id)
			if !ok {
				continue
			}
			if n.GrantSkill != SkillNone {
				panic("core: passive node '" + id + "' must not grant a castable skill — its effect is wired through the battle passive hooks, not the skill registry")
			}
			found = true
		}
		if !found {
			panic("core: passive node id '" + id + "' resolves to no skill-tree node — the battle passive hooks would silently no-op")
		}
	}
}

// PassiveRank returns how many ranks `m` invested in the node (0 if unlearned /
// wrong class, nil-safe). A passive scales by raw node rank, NOT the SkillTiers ladder.
func PassiveRank(m *PartyMember, nodeID string) int {
	return TreeNodeRank(m, nodeID)
}

// MemberCritChance is CritChance for `m` plus the Lucky Strike bonus
// (LuckyStrikeCritPerRank per rank), re-clamped at CritCap. Member-aware sibling
// of CritChance.
func MemberCritChance(m *PartyMember, quality int) float64 {
	// Nil-safe: EffectiveStatsPtr dereferences m.Equipped.
	if m == nil {
		return 0
	}
	chance := CritChance(EffectiveStatsPtr(m), quality)
	chance += float64(PassiveRank(m, PassiveLuckyStrike)) * LuckyStrikeCritPerRank
	return Clamp(chance, 0, CritCap)
}

// MemberRollCrit rolls a crit for a member, folding in Lucky Strike via
// MemberCritChance. Member-aware sibling of RollCrit.
func MemberRollCrit(rng *rand.Rand, m *PartyMember, quality int) bool {
	return RollChance(rng, MemberCritChance(m, quality))
}

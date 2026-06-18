package core

import "math/rand"

// Passive skill-tree nodes carry no GrantSkill — instead of learning a castable
// skill, the battle pipeline reads the member's invested RANK at the node and
// folds a per-rank effect into combat. These consts name the nodes the pipeline
// queries so the hot-path lookups don't hardcode the authoring strings that the
// tree tables in skilltrees.go own (a drift between the two would silently make
// the passive a no-op — the init guard below catches it at process start).
//
// Each maps to one combat hook:
//   - Riposte (Warrior, Battle Sense): counter an enemy whose strike you dodge.
//   - Bloodthirst (Warrior, Fury): heal a share of the turn's physical damage.
//   - Retribution (Cleric, Conviction): reflect a share of damage taken.
//   - Shadow Step (Thief, Shadow Arts): bonus damage when acting before target.
//   - Lucky Strike (Thief, Cutpurse): added crit chance.
const (
	PassiveRiposte     = "riposte"
	PassiveBloodthirst = "bloodthirst"
	PassiveRetribution = "retribution"
	PassiveShadowStep  = "shadow-step"
	PassiveLuckyStrike = "lucky-strike"
)

// passiveNodeIDs is the canonical list the init guard walks. Append a new
// passive's id here when its effect is wired so the guard keeps the const in
// lockstep with a real, non-granting tree node.
var passiveNodeIDs = []string{
	PassiveRiposte,
	PassiveBloodthirst,
	PassiveRetribution,
	PassiveShadowStep,
	PassiveLuckyStrike,
}

// init asserts the passive-node contract, mirroring the registry-invariant
// guards used across the codebase (skills / tiles / skill trees): every passive
// id must resolve to a real tree node that grants NO castable skill. A typo'd id
// would make the battle hook read rank 0 forever (a silent no-op); a node that
// also carries a GrantSkill would mean the effect fires from two systems at
// once. Either is a panic at process start instead of a quiet mis-balance found
// at playtest.
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

// PassiveRank returns how many ranks `m` has invested in the named passive node
// (0 if unlearned, nil-safe). A thin, intent-documenting alias over TreeNodeRank
// used at the battle hook sites: a passive's strength scales by raw node rank,
// NOT by the SkillTiers upgrade ladder that GRANTING nodes drive. Members of a
// class that doesn't own the node read 0 (their TreeRanks never hold its id), so
// callers don't need a class check.
func PassiveRank(m *PartyMember, nodeID string) int {
	return TreeNodeRank(m, nodeID)
}

// MemberCritChance is CritChance for `m` plus the additive Lucky Strike passive
// bonus (LuckyStrikeCritPerRank per rank), re-clamped at CritCap so the passive
// can't push past the ceiling the base DEX/timing curve already respects. The
// member-aware sibling of CritChance (which takes bare Stats for enemies and
// the equipment-preview readouts).
func MemberCritChance(m PartyMember, quality int) float64 {
	chance := CritChance(EffectiveStats(m), quality)
	chance += float64(PassiveRank(&m, PassiveLuckyStrike)) * LuckyStrikeCritPerRank
	return Clamp(chance, 0, CritCap)
}

// MemberRollCrit rolls a crit for a party member, folding in Lucky Strike via
// MemberCritChance. The member-aware sibling of RollCrit (which enemies and
// stat-only call sites still use).
func MemberRollCrit(rng *rand.Rand, m PartyMember, quality int) bool {
	return RollChance(rng, MemberCritChance(m, quality))
}

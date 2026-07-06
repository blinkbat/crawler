package core

import (
	"fmt"
	"strings"
)

// packmember.go holds authored-pack helpers plus the small shared utilities that
// used to live beside the (now-removed) custom-enemy system: the enemy-stat-bounds
// validator the registry init shares, the mapfile skill on-disk-name layer, and the
// map-dimension clamp.

// BuiltinPackMember returns a pack member resolving to a built-in enemy kind.
func BuiltinPackMember(kind EnemyKind) PackMemberRef {
	return PackMemberRef{Kind: kind}
}

// PackMemberIndexInRange is the one home for the `idx >= 0 && idx < len(sp.Members)`
// rule — the pack-member cousin of PartyIndexInRange.
func PackMemberIndexInRange(sp PackSpawn, idx int) bool {
	return idx >= 0 && idx < len(sp.Members)
}

// AppendBuiltinPackMember appends a built-in enemy kind.
func AppendBuiltinPackMember(sp *PackSpawn, kind EnemyKind) {
	sp.Members = append(sp.Members, BuiltinPackMember(kind))
}

// RemovePackMember removes one member slot.
func RemovePackMember(sp *PackSpawn, idx int) {
	if !PackMemberIndexInRange(*sp, idx) {
		return
	}
	sp.Members = append(sp.Members[:idx], sp.Members[idx+1:]...)
}

// SwapPackMembers swaps two member slots.
func SwapPackMembers(sp *PackSpawn, i, j int) {
	if !PackMemberIndexInRange(*sp, i) || !PackMemberIndexInRange(*sp, j) {
		return
	}
	sp.Members[i], sp.Members[j] = sp.Members[j], sp.Members[i]
}

// defaultEnemyKind is the fallback kind returned when a pack slot or battle target
// can't be resolved (empty pack, out-of-range index). Named so the "which enemy do we
// fall back to" decision lives in one place instead of EnemyRat hardcoded at each site.
const defaultEnemyKind = EnemyRat

// PackMemberDefinition returns a pack member's effective definition (member kinds
// resolve straight to the registry).
func PackMemberDefinition(sp PackSpawn, idx int) EnemyDefinition {
	if !PackMemberIndexInRange(sp, idx) {
		return EnemyInfo(defaultEnemyKind)
	}
	return EnemyInfo(sp.Members[idx].Kind)
}

// PackMemberDisplayName is the editor-facing name for an authored pack slot.
func PackMemberDisplayName(sp PackSpawn, idx int) string {
	return PackMemberDefinition(sp, idx).SingularName
}

// PackMemberVisualKind returns the base kind whose sprite/color represents a pack slot.
func PackMemberVisualKind(sp PackSpawn, idx int) EnemyKind {
	if !PackMemberIndexInRange(sp, idx) {
		return defaultEnemyKind
	}
	return sp.Members[idx].Kind
}

// PackSpawnLeaderSlot returns the highest-tier member slot.
func PackSpawnLeaderSlot(sp PackSpawn) int {
	return leaderSlot(len(sp.Members), func(i int) int { return PackMemberDefinition(sp, i).Tier })
}

// PackSpawnLeaderKind returns the visual base kind for the pack's highest-tier member.
func PackSpawnLeaderKind(sp PackSpawn) EnemyKind {
	if len(sp.Members) == 0 {
		return defaultEnemyKind
	}
	return PackMemberVisualKind(sp, PackSpawnLeaderSlot(sp))
}

// enemyStatBounds names the combat-stat fields validateEnemyStatBounds checks, so
// callers can't transpose armor/mdef/attack/xp/spellpower/tier into the wrong slot
// (they're all bare ints).
type enemyStatBounds struct {
	Name             string
	SkillCastChance  float64
	PoisonChance     float64
	LifestealPercent float64
	Armor            int
	MDef             int
	AttackDamage     int
	XPValue          int
	SpellPower       int
	Tier             int
}

// validateEnemyStatBounds checks the static enemy registry's combat-stat fields at
// init: probability fields (SkillCastChance, PoisonChance, LifestealPercent) must be
// in [0,1]; mitigation, reward, damage, and tier must be non-negative.
func validateEnemyStatBounds(b enemyStatBounds) error {
	if !ValidChance(b.SkillCastChance) {
		return fmt.Errorf("enemy %q has SkillCastChance %v outside [0, 1]", b.Name, b.SkillCastChance)
	}
	if !ValidChance(b.PoisonChance) {
		return fmt.Errorf("enemy %q has PoisonChance %v outside [0, 1]", b.Name, b.PoisonChance)
	}
	if !ValidChance(b.LifestealPercent) {
		return fmt.Errorf("enemy %q has LifestealPercent %v outside [0, 1]", b.Name, b.LifestealPercent)
	}
	if b.Armor < 0 || b.MDef < 0 || b.AttackDamage < 0 || b.XPValue < 0 || b.SpellPower < 0 || b.Tier < 0 {
		return fmt.Errorf("enemy %q has a negative stat field (armor/mdef/attack/xp/spellpower/tier)", b.Name)
	}
	return nil
}

// SkillOnDiskName is the canonical lower-snake-case identifier for mapfile skill
// lists. A skill may pin a frozen slug via skillDefinition.OnDiskName so renaming
// its display Name doesn't change its persisted identity; otherwise it derives
// from the display name.
func SkillOnDiskName(s SkillID) string {
	if s == SkillNone {
		return ""
	}
	if def, ok := skillInfo(s); ok && def.OnDiskName != "" {
		return strings.ToLower(def.OnDiskName)
	}
	return strings.ToLower(strings.ReplaceAll(SkillName(s), " ", "_"))
}

// skillByOnDiskName is the O(1) reverse lookup for SkillIDFromOnDiskName, built once at init.
var skillByOnDiskName = buildSkillByOnDiskName()

func buildSkillByOnDiskName() map[string]SkillID {
	m := make(map[string]SkillID, len(skillDefinitions))
	for _, def := range skillDefinitions {
		if name := SkillOnDiskName(def.Skill); name != "" {
			// Collision assert: two skills whose names collapse to the same
			// on-disk slug would last-write-wins and mis-route SkillIDFromOnDiskName.
			if existing, dup := m[name]; dup && existing != def.Skill {
				panic("core: skill on-disk name collision on " + name)
			}
			m[name] = def.Skill
		}
	}
	return m
}

// SkillIDFromOnDiskName is the inverse of SkillOnDiskName.
func SkillIDFromOnDiskName(name string) (SkillID, bool) {
	if name == "" {
		return SkillNone, false
	}
	id, ok := skillByOnDiskName[strings.ToLower(strings.TrimSpace(name))]
	return id, ok
}

// AllSkillIDs returns every SkillID registered in declaration order.
func AllSkillIDs() []SkillID {
	return skillIDsWhereInto(make([]SkillID, 0, len(skillDefinitions)), nil)
}

// EnemyCastableSkills returns every skill the enemy AI is allowed to cast.
func EnemyCastableSkills() []SkillID {
	return skillIDsWhereInto(make([]SkillID, 0, len(skillDefinitions)), func(d skillDefinition) bool { return d.EnemyCastable })
}

// IsEnemyCastable reports whether a skill's registry entry has the EnemyCastable flag set.
func IsEnemyCastable(s SkillID) bool {
	def, ok := skillInfo(s)
	return ok && def.EnemyCastable
}

// ClampMapDimension clips v to the editor's playable range.
func ClampMapDimension(v int) int {
	if v < MinMapDimension {
		return MinMapDimension
	}
	if v > MaxMapDimension {
		return MaxMapDimension
	}
	return v
}

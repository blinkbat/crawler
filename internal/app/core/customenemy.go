package core

import (
	"crawler/internal/app/core/mapfile"
	"fmt"
	"strings"
)

// CustomEnemyDef is an author-defined enemy template stored alongside a map.
// BaseKind supplies the built-in sprite and attack verbs; the other fields
// override combat stats, reward, display name, and AI loadout.
type CustomEnemyDef struct {
	Name     string
	BaseKind EnemyKind
	HP       int
	MP       int
	Stats    Stats

	Armor           int
	XPValue         int
	Tier            int
	AttackDamage    int
	SkillCastChance float64
	SpellPower      int
	Skills          []SkillID
}

// DefaultCustomEnemy returns a working clone of a built-in enemy for the
// editor's "New custom enemy" flow.
func DefaultCustomEnemy(name string, base EnemyKind) CustomEnemyDef {
	def := EnemyInfo(base)
	return CustomEnemyDef{
		Name:            name,
		BaseKind:        base,
		HP:              def.MaxHP,
		MP:              0,
		Stats:           Stats{STR: 1, DEX: 1, INT: 1, WIS: 1, VIT: 1, SPD: def.Speed},
		Armor:           def.Armor,
		XPValue:         def.XPValue,
		Tier:            def.Tier,
		AttackDamage:    def.AttackDamage,
		SkillCastChance: def.SkillCastChance,
		SpellPower:      def.SpellPower,
		Skills:          append([]SkillID(nil), def.Skills...),
	}
}

// CustomEnemyDefFromMap converts one on-disk custom enemy row into the core
// definition used by editor/runtime code.
func CustomEnemyDefFromMap(ce mapfile.MapCustomEnemy) (CustomEnemyDef, error) {
	base, ok := EnemyKindFromName(ce.BaseKind)
	if !ok {
		return CustomEnemyDef{}, fmt.Errorf("custom enemy %q references unknown base kind %q", ce.Name, ce.BaseKind)
	}
	skills := make([]SkillID, 0, len(ce.Skills))
	for _, name := range ce.Skills {
		id, ok := SkillIDFromOnDiskName(name)
		if !ok {
			return CustomEnemyDef{}, fmt.Errorf("custom enemy %q references unknown skill %q", ce.Name, name)
		}
		skills = append(skills, id)
	}
	return CustomEnemyDef{
		Name:            ce.Name,
		BaseKind:        base,
		HP:              ce.HP,
		MP:              ce.MP,
		Stats:           Stats{STR: ce.STR, DEX: ce.DEX, INT: ce.INT, WIS: ce.WIS, VIT: ce.VIT, SPD: ce.SPD},
		Armor:           ce.Armor,
		XPValue:         ce.XPValue,
		Tier:            ce.Tier,
		AttackDamage:    ce.AttackDamage,
		SkillCastChance: ce.SkillCastChance,
		SpellPower:      ce.SpellPower,
		Skills:          skills,
	}, nil
}

// MapCustomEnemyFromDef converts a core custom enemy definition back to the
// mapfile row shape, validating every registry-backed field on the way out.
func MapCustomEnemyFromDef(ce CustomEnemyDef) (mapfile.MapCustomEnemy, error) {
	baseName, ok := EnemyKindName(ce.BaseKind)
	if !ok {
		return mapfile.MapCustomEnemy{}, fmt.Errorf("custom enemy %q has unknown base kind %d", ce.Name, int(ce.BaseKind))
	}
	skillNames := make([]string, 0, len(ce.Skills))
	for _, id := range ce.Skills {
		name := SkillOnDiskName(id)
		if name == "" {
			return mapfile.MapCustomEnemy{}, fmt.Errorf("custom enemy %q has unknown skill id %d", ce.Name, int(id))
		}
		skillNames = append(skillNames, name)
	}
	// Defense-in-depth sanitize: the editor already collapses whitespace
	// when the author types, but routing through the helper here means any
	// future writer (importer, script) can't land a name on disk that the
	// strings.Fields-based loader would later misparse.
	safeName := SanitizeCustomEnemyName(ce.Name)
	if safeName == "" {
		return mapfile.MapCustomEnemy{}, fmt.Errorf("custom enemy has empty name after sanitize")
	}
	return mapfile.MapCustomEnemy{
		Name:            safeName,
		BaseKind:        baseName,
		HP:              ce.HP,
		MP:              ce.MP,
		STR:             ce.Stats.STR,
		DEX:             ce.Stats.DEX,
		INT:             ce.Stats.INT,
		WIS:             ce.Stats.WIS,
		VIT:             ce.Stats.VIT,
		SPD:             ce.Stats.SPD,
		Armor:           ce.Armor,
		XPValue:         ce.XPValue,
		Tier:            ce.Tier,
		AttackDamage:    ce.AttackDamage,
		SkillCastChance: ce.SkillCastChance,
		SpellPower:      ce.SpellPower,
		Skills:          skillNames,
	}, nil
}

// Definition synthesizes the effective EnemyDefinition used by battle and
// selectors for a custom enemy.
func (d CustomEnemyDef) Definition() EnemyDefinition {
	base := EnemyInfo(d.BaseKind)
	display := strings.ReplaceAll(strings.TrimSpace(d.Name), "_", " ")
	if display == "" {
		display = base.SingularName
	}
	noun := strings.ToLower(display)
	def := base
	def.Name = display
	def.SingularName = display
	def.PluralName = display + "s"
	def.SingularNoun = noun
	def.PluralNoun = noun + "s"
	def.GroupName = display
	def.MaxHP = d.HP
	def.AttackDamage = d.AttackDamage
	def.Speed = d.Stats.SPD
	def.Tier = d.Tier
	def.Armor = d.Armor
	def.XPValue = d.XPValue
	def.Skills = append([]SkillID(nil), d.Skills...)
	def.SkillCastChance = d.SkillCastChance
	def.SpellPower = d.SpellPower
	return def
}

// Instantiate materializes a runtime Enemy from this def. Kind stays the base
// kind for renderer lookup; DefinitionOverride carries the authored stats and
// loadout for EnemyInfoFor readers.
func (d CustomEnemyDef) Instantiate() Enemy {
	def := d.Definition()
	return Enemy{
		Kind:                  d.BaseKind,
		HP:                    def.MaxHP,
		MaxHP:                 def.MaxHP,
		Armor:                 def.Armor,
		Alive:                 true,
		Item:                  def.Item,
		CustomName:            d.Name,
		DefinitionOverride:    def,
		HasDefinitionOverride: true,
	}
}

// LookupEnemyStats returns the effective definition for an Enemy instance.
// The area argument is retained for older call sites; custom enemies now carry
// their override on the Enemy itself.
func LookupEnemyStats(area AreaDefinition, enemy Enemy) EnemyDefinition {
	_ = area
	return EnemyInfoFor(enemy)
}

// CustomEnemyByName looks up a def by exact or sanitized name. Supporting the
// sanitized form keeps mapfile pack references stable even if an editor path
// normalized whitespace before writing.
func CustomEnemyByName(defs []CustomEnemyDef, name string) (CustomEnemyDef, bool) {
	for _, d := range defs {
		if d.Name == name || SanitizeCustomEnemyName(d.Name) == name {
			return d, true
		}
	}
	return CustomEnemyDef{}, false
}

// BuiltinPackMember returns an authored pack member that resolves directly to
// a built-in enemy kind.
func BuiltinPackMember(kind EnemyKind) PackMemberRef {
	return PackMemberRef{Kind: kind}
}

// CustomPackMember returns an authored pack member that resolves through the
// map's CustomEnemies registry while retaining the base kind for visuals.
func CustomPackMember(def CustomEnemyDef) PackMemberRef {
	return PackMemberRef{Kind: def.BaseKind, CustomName: def.Name}
}

// PackMemberCustomName returns the custom enemy name stored for a pack member
// slot, or "" for a built-in member.
func PackMemberCustomName(sp PackSpawn, idx int) string {
	if idx < 0 || idx >= len(sp.Members) {
		return ""
	}
	return sp.Members[idx].CustomName
}

// AppendBuiltinPackMember appends a built-in enemy kind.
func AppendBuiltinPackMember(sp *PackSpawn, kind EnemyKind) {
	sp.Members = append(sp.Members, BuiltinPackMember(kind))
}

// AppendCustomPackMember appends a custom enemy reference to a pack.
func AppendCustomPackMember(sp *PackSpawn, def CustomEnemyDef) {
	sp.Members = append(sp.Members, CustomPackMember(def))
}

// RemovePackMember removes one member slot.
func RemovePackMember(sp *PackSpawn, idx int) {
	if idx < 0 || idx >= len(sp.Members) {
		return
	}
	sp.Members = append(sp.Members[:idx], sp.Members[idx+1:]...)
}

// SwapPackMembers swaps two member slots.
func SwapPackMembers(sp *PackSpawn, i, j int) {
	if i < 0 || i >= len(sp.Members) || j < 0 || j >= len(sp.Members) {
		return
	}
	sp.Members[i], sp.Members[j] = sp.Members[j], sp.Members[i]
}

// PackMemberDefinition returns the effective definition for an authored pack
// member, resolving custom names through the containing area.
func PackMemberDefinition(a AreaDefinition, sp PackSpawn, idx int) EnemyDefinition {
	if name := PackMemberCustomName(sp, idx); name != "" {
		if def, ok := CustomEnemyByName(a.CustomEnemies, name); ok {
			return def.Definition()
		}
	}
	if idx < 0 || idx >= len(sp.Members) {
		return EnemyInfo(EnemyRat)
	}
	return EnemyInfo(sp.Members[idx].Kind)
}

// PackMemberDisplayName is the editor-facing name for an authored pack slot.
func PackMemberDisplayName(a AreaDefinition, sp PackSpawn, idx int) string {
	return PackMemberDefinition(a, sp, idx).SingularName
}

// PackMemberVisualKind returns the base kind whose sprite/color should
// represent an authored pack slot.
func PackMemberVisualKind(a AreaDefinition, sp PackSpawn, idx int) EnemyKind {
	if name := PackMemberCustomName(sp, idx); name != "" {
		if def, ok := CustomEnemyByName(a.CustomEnemies, name); ok {
			return def.BaseKind
		}
	}
	if idx < 0 || idx >= len(sp.Members) {
		return EnemyRat
	}
	return sp.Members[idx].Kind
}

// PackSpawnLeaderSlot returns the highest-tier member slot for an authored
// pack, resolving custom enemy tiers as well as built-in tiers.
func PackSpawnLeaderSlot(a AreaDefinition, sp PackSpawn) int {
	bestSlot := 0
	bestTier := -1
	for i := range sp.Members {
		t := PackMemberDefinition(a, sp, i).Tier
		if t > bestTier {
			bestTier = t
			bestSlot = i
		}
	}
	return bestSlot
}

// PackSpawnLeaderKind returns the visual base kind for the authored pack's
// highest-tier member.
func PackSpawnLeaderKind(a AreaDefinition, sp PackSpawn) EnemyKind {
	if len(sp.Members) == 0 {
		return EnemyRat
	}
	return PackMemberVisualKind(a, sp, PackSpawnLeaderSlot(a, sp))
}

// SanitizeCustomEnemyName collapses whitespace into underscores and trims
// edges so mapfile rows remain field-splittable.
func SanitizeCustomEnemyName(name string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(name)), "_")
}

// SkillOnDiskName is the canonical lower-snake-case identifier used in
// custom-enemy mapfile skill lists.
func SkillOnDiskName(s SkillID) string {
	if s == SkillNone {
		return ""
	}
	return strings.ToLower(strings.ReplaceAll(SkillName(s), " ", "_"))
}

// SkillIDFromOnDiskName is the inverse of SkillOnDiskName.
func SkillIDFromOnDiskName(name string) (SkillID, bool) {
	if name == "" {
		return SkillNone, false
	}
	want := strings.ToLower(strings.TrimSpace(name))
	for _, def := range skillDefinitions {
		if SkillOnDiskName(def.Skill) == want {
			return def.Skill, true
		}
	}
	return SkillNone, false
}

// AllSkillIDs returns every SkillID registered in declaration order.
func AllSkillIDs() []SkillID {
	out := make([]SkillID, 0, len(skillDefinitions))
	for _, def := range skillDefinitions {
		out = append(out, def.Skill)
	}
	return out
}

// EnemyCastableSkills returns every skill the enemy AI is allowed to cast.
func EnemyCastableSkills() []SkillID {
	out := make([]SkillID, 0, len(skillDefinitions))
	for _, def := range skillDefinitions {
		if def.EnemyCastable {
			out = append(out, def.Skill)
		}
	}
	return out
}

// IsEnemyCastable reports whether a skill's registry entry has the
// EnemyCastable flag set. Sibling of EnemyCastableSkills — that's the
// "give me the list" form, this is the "is THIS one in?" form. Used
// by the battle package's init guard to walk its handler map and
// catch handlers that linger after the flag is cleared.
func IsEnemyCastable(s SkillID) bool {
	for _, def := range skillDefinitions {
		if def.Skill == s {
			return def.EnemyCastable
		}
	}
	return false
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

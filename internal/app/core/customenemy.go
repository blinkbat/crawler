package core

import (
	"crawler/internal/app/core/mapfile"
	"fmt"
	"strings"
)

// CustomEnemyDef is an author-defined enemy template stored alongside a map. BaseKind supplies the
// built-in sprite and attack verbs; other fields override combat stats, reward, name, and AI loadout.
type CustomEnemyDef struct {
	Name     string
	BaseKind EnemyKind
	HP       int
	// MP is RESERVED: persisted but unused — the runtime Enemy has no MP pool (casts gate on
	// SkillCastChance). Kept for forward-compatibility if enemy MP mechanics ship.
	MP    int
	Stats Stats

	Armor           int
	MDef            int
	XPValue         int
	Tier            int
	AttackDamage    int
	SkillCastChance float64
	SpellPower      int
	Skills          []SkillID
}

// DefaultCustomEnemy returns a working clone of a built-in enemy for the editor's "New" flow.
func DefaultCustomEnemy(name string, base EnemyKind) CustomEnemyDef {
	def := EnemyInfo(base)
	return CustomEnemyDef{
		Name:            name,
		BaseKind:        base,
		HP:              def.MaxHP,
		MP:              0,
		Stats:           def.Stats,
		Armor:           def.Armor,
		MDef:            def.MDef,
		XPValue:         def.XPValue,
		Tier:            def.Tier,
		AttackDamage:    def.AttackDamage,
		SkillCastChance: def.SkillCastChance,
		SpellPower:      def.SpellPower,
		Skills:          append([]SkillID(nil), def.Skills...),
	}
}

// validateEnemyStatBounds checks combat-stat fields shared by the static registry and custom
// enemies: proc chances (SkillCastChance, PoisonChance) must be in [0,1]; mitigation/reward/damage
// must be non-negative. Shared so registry init (panic) and the map loader (author error) agree.
// CustomEnemyDef has no PoisonChance, so the custom path passes 0.
func validateEnemyStatBounds(name string, skillCastChance, poisonChance float64, armor, mdef, attackDamage, xpValue, spellPower, tier int) error {
	if !ValidChance(skillCastChance) {
		return fmt.Errorf("enemy %q has SkillCastChance %v outside [0, 1]", name, skillCastChance)
	}
	if !ValidChance(poisonChance) {
		return fmt.Errorf("enemy %q has PoisonChance %v outside [0, 1]", name, poisonChance)
	}
	// Tier included so the save path agrees with the loader (parseCustomEnemyLine rejects negatives).
	if armor < 0 || mdef < 0 || attackDamage < 0 || xpValue < 0 || spellPower < 0 || tier < 0 {
		return fmt.Errorf("enemy %q has a negative stat field (armor/mdef/attack/xp/spellpower/tier)", name)
	}
	return nil
}

// validateCustomEnemyExtras guards HP/MP, which validateEnemyStatBounds doesn't cover. HP must be
// positive: an HP<=0 row Instantiates to an "alive corpse" (enemyAlive keys on Alive, not HP) the
// encounter can never beat. MP must be non-negative. Shared by loader and writer so they can't drift.
func validateCustomEnemyExtras(name string, hp, mp int) error {
	if hp <= 0 {
		return fmt.Errorf("custom enemy %q has non-positive HP (%d)", name, hp)
	}
	if mp < 0 {
		return fmt.Errorf("custom enemy %q has negative MP (%d)", name, mp)
	}
	return nil
}

// CustomEnemyDefFromMap converts one on-disk custom enemy row into the core definition.
func CustomEnemyDefFromMap(ce mapfile.MapCustomEnemy) (CustomEnemyDef, error) {
	base, ok := EnemyKindFromName(ce.BaseKind)
	if !ok {
		return CustomEnemyDef{}, fmt.Errorf("custom enemy %q references unknown base kind %q", ce.Name, ce.BaseKind)
	}
	// Refuse bad rows at load (shared with the writer so the two can't drift).
	if err := validateCustomEnemyExtras(ce.Name, ce.HP, ce.MP); err != nil {
		return CustomEnemyDef{}, err
	}
	if err := validateEnemyStatBounds(ce.Name, ce.SkillCastChance, 0, ce.Armor, ce.MDef, ce.AttackDamage, ce.XPValue, ce.SpellPower, ce.Tier); err != nil {
		return CustomEnemyDef{}, err
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
		MDef:            ce.MDef,
		XPValue:         ce.XPValue,
		Tier:            ce.Tier,
		AttackDamage:    ce.AttackDamage,
		SkillCastChance: ce.SkillCastChance,
		SpellPower:      ce.SpellPower,
		Skills:          skills,
	}, nil
}

// MapCustomEnemyFromDef converts a core definition back to the mapfile row, validating on the way out.
func MapCustomEnemyFromDef(ce CustomEnemyDef) (mapfile.MapCustomEnemy, error) {
	baseName, ok := EnemyKindName(ce.BaseKind)
	if !ok {
		return mapfile.MapCustomEnemy{}, fmt.Errorf("custom enemy %q has unknown base kind %d", ce.Name, int(ce.BaseKind))
	}
	// Validate on the way OUT too: a non-editor writer (importer/script) could otherwise persist a
	// field the loader would refuse, yielding an unloadable map. Same lockstep for HP/MP below.
	if err := validateEnemyStatBounds(ce.Name, ce.SkillCastChance, 0, ce.Armor, ce.MDef, ce.AttackDamage, ce.XPValue, ce.SpellPower, ce.Tier); err != nil {
		return mapfile.MapCustomEnemy{}, err
	}
	if err := validateCustomEnemyExtras(ce.Name, ce.HP, ce.MP); err != nil {
		return mapfile.MapCustomEnemy{}, err
	}
	skillNames := make([]string, 0, len(ce.Skills))
	for _, id := range ce.Skills {
		name := SkillOnDiskName(id)
		if name == "" {
			return mapfile.MapCustomEnemy{}, fmt.Errorf("custom enemy %q has unknown skill id %d", ce.Name, int(id))
		}
		skillNames = append(skillNames, name)
	}
	// Sanitize so any future writer can't land a name the strings.Fields loader would misparse.
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
		MDef:            ce.MDef,
		XPValue:         ce.XPValue,
		Tier:            ce.Tier,
		AttackDamage:    ce.AttackDamage,
		SkillCastChance: ce.SkillCastChance,
		SpellPower:      ce.SpellPower,
		Skills:          skillNames,
	}, nil
}

// Definition synthesizes the runtime EnemyDefinition battle/selectors read for a custom enemy.
//
// LOCKSTEP SITES — a new authored field must be added in all of:
//  1. CustomEnemyDef (above),
//  2. mapfile.MapCustomEnemy + its encode format/field-count,
//  3. MapCustomEnemyFromDef (def -> row),
//  4. CustomEnemyDefFromMap (row -> def),
//  5. this Definition() (def -> runtime), and
//  6. Instantiate() if the field affects the materialized Enemy.
//
// 3&4 guarded by TestCustomEnemyDefMapRoundTrip; 5&6 by TestCustomEnemyDefToRuntime.
func (d CustomEnemyDef) Definition() EnemyDefinition {
	base := EnemyInfo(d.BaseKind)
	display := CustomEnemyDisplayName(d.Name)
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
	def.Stats = d.Stats
	def.Tier = d.Tier
	def.Armor = d.Armor
	def.MDef = d.MDef
	def.XPValue = d.XPValue
	def.Skills = append([]SkillID(nil), d.Skills...)
	def.SkillCastChance = d.SkillCastChance
	def.SpellPower = d.SpellPower
	return def
}

// Instantiate materializes a runtime Enemy. Kind stays the base kind for renderer lookup;
// DefinitionOverride carries the authored stats/loadout for EnemyInfoFor readers.
func (d CustomEnemyDef) Instantiate() Enemy {
	def := d.Definition()
	// Scale spawn HP by the difficulty dial like NewEnemy, else custom foes get scaled damage but baseline HP.
	maxHP := ScaleEnemyDifficulty(def.MaxHP)
	return Enemy{
		Kind:                  d.BaseKind,
		HP:                    maxHP,
		MaxHP:                 maxHP,
		Armor:                 def.Armor,
		Alive:                 true,
		Item:                  def.Item,
		CustomName:            d.Name,
		DefinitionOverride:    def,
		HasDefinitionOverride: true,
	}
}

// CustomEnemyByName looks up a def by exact or sanitized name (so pack refs survive whitespace normalization).
func CustomEnemyByName(defs []CustomEnemyDef, name string) (CustomEnemyDef, bool) {
	for _, d := range defs {
		if d.Name == name || SanitizeCustomEnemyName(d.Name) == name {
			return d, true
		}
	}
	return CustomEnemyDef{}, false
}

// BuiltinPackMember returns a pack member resolving to a built-in enemy kind.
func BuiltinPackMember(kind EnemyKind) PackMemberRef {
	return PackMemberRef{Kind: kind}
}

// CustomPackMember returns a pack member resolving through CustomEnemies, keeping base kind for visuals.
func CustomPackMember(def CustomEnemyDef) PackMemberRef {
	return PackMemberRef{Kind: def.BaseKind, CustomName: def.Name}
}

// PackMemberCustomName returns a slot's custom enemy name, or "" for a built-in member.
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

// packMemberCustom resolves the custom enemy a pack slot names via the area's roster. ok=false means
// a plain built-in kind (read sp.Members[idx].Kind). Shared by PackMemberDefinition/PackMemberVisualKind.
func packMemberCustom(a AreaDefinition, sp PackSpawn, idx int) (CustomEnemyDef, bool) {
	if name := PackMemberCustomName(sp, idx); name != "" {
		return CustomEnemyByName(a.CustomEnemies, name)
	}
	return CustomEnemyDef{}, false
}

// PackMemberDefinition returns a pack member's effective definition, resolving custom names via the area.
func PackMemberDefinition(a AreaDefinition, sp PackSpawn, idx int) EnemyDefinition {
	if def, ok := packMemberCustom(a, sp, idx); ok {
		return def.Definition()
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

// PackMemberVisualKind returns the base kind whose sprite/color represents a pack slot.
func PackMemberVisualKind(a AreaDefinition, sp PackSpawn, idx int) EnemyKind {
	if def, ok := packMemberCustom(a, sp, idx); ok {
		return def.BaseKind
	}
	if idx < 0 || idx >= len(sp.Members) {
		return EnemyRat
	}
	return sp.Members[idx].Kind
}

// PackSpawnLeaderSlot returns the highest-tier member slot, resolving custom and built-in tiers.
func PackSpawnLeaderSlot(a AreaDefinition, sp PackSpawn) int {
	return leaderSlot(len(sp.Members), func(i int) int { return PackMemberDefinition(a, sp, i).Tier })
}

// PackSpawnLeaderKind returns the visual base kind for the pack's highest-tier member.
func PackSpawnLeaderKind(a AreaDefinition, sp PackSpawn) EnemyKind {
	if len(sp.Members) == 0 {
		return EnemyRat
	}
	return PackMemberVisualKind(a, sp, PackSpawnLeaderSlot(a, sp))
}

// SanitizeCustomEnemyName folds whitespace runs to single underscores and trims edges so the name
// survives the loader's strings.Fields split. PRESERVES case and other punctuation. NOT slugify or
// SanitizeFilename — each owns a different on-disk format; don't swap them.
func SanitizeCustomEnemyName(name string) string {
	// Commas/semicolons are the pack-member-list separators on disk, so a name carrying
	// one would re-split into phantom members on reload. Fold them to spaces first so
	// they collapse into the single-underscore separator below, like any whitespace.
	name = strings.NewReplacer(",", " ", ";", " ").Replace(name)
	return strings.Join(strings.Fields(strings.TrimSpace(name)), "_")
}

// CustomEnemyDisplayName is the inverse of SanitizeCustomEnemyName ("_" back to spaces).
func CustomEnemyDisplayName(name string) string {
	return strings.ReplaceAll(strings.TrimSpace(name), "_", " ")
}

// SkillOnDiskName is the canonical lower-snake-case identifier for mapfile skill lists.
func SkillOnDiskName(s SkillID) string {
	if s == SkillNone {
		return ""
	}
	return strings.ToLower(strings.ReplaceAll(SkillName(s), " ", "_"))
}

// skillByOnDiskName is the O(1) reverse lookup for SkillIDFromOnDiskName, built once at init.
var skillByOnDiskName = buildSkillByOnDiskName()

func buildSkillByOnDiskName() map[string]SkillID {
	m := make(map[string]SkillID, len(skillDefinitions))
	for _, def := range skillDefinitions {
		if name := SkillOnDiskName(def.Skill); name != "" {
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

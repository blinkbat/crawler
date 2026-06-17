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
	// MP is RESERVED: authored and persisted through the mapfile, but the
	// runtime Enemy has no MP pool (enemy casts gate on SkillCastChance, not
	// resource), so Definition()/Instantiate() do not apply it. Kept so the
	// schema is forward-compatible if enemy MP mechanics ship later.
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

// DefaultCustomEnemy returns a working clone of a built-in enemy for the
// editor's "New custom enemy" flow.
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

// validateEnemyStatBounds checks the scalar combat-stat fields common to
// the static registry (EnemyDefinition) and authored custom enemies
// (CustomEnemyDef): SkillCastChance rides a [0,1] contract (a stray 5
// would cast every turn) and the mitigation / reward / damage fields must
// be non-negative. Returns a descriptive error (nil when clean) so the
// registry init can panic on it while the map loader surfaces it to the
// author — one set of bounds, two failure modes, no drift.
func validateEnemyStatBounds(name string, skillCastChance float64, armor, mdef, attackDamage, xpValue, spellPower, tier int) error {
	if skillCastChance < 0 || skillCastChance > 1 {
		return fmt.Errorf("enemy %q has SkillCastChance %v outside [0, 1]", name, skillCastChance)
	}
	// Tier is included so the editor save path (MapCustomEnemyFromDef →
	// validateEnemyStatBounds) agrees with the map LOADER (parseCustomEnemyLine,
	// which rejects every negative numeric field): without it, a negative tier
	// authored in the editor saves fine but yields an unloadable map.
	if armor < 0 || mdef < 0 || attackDamage < 0 || xpValue < 0 || spellPower < 0 || tier < 0 {
		return fmt.Errorf("enemy %q has a negative stat field (armor/mdef/attack/xp/spellpower/tier)", name)
	}
	return nil
}

// CustomEnemyDefFromMap converts one on-disk custom enemy row into the core
// definition used by editor/runtime code.
func CustomEnemyDefFromMap(ce mapfile.MapCustomEnemy) (CustomEnemyDef, error) {
	base, ok := EnemyKindFromName(ce.BaseKind)
	if !ok {
		return CustomEnemyDef{}, fmt.Errorf("custom enemy %q references unknown base kind %q", ce.Name, ce.BaseKind)
	}
	// HP must be positive. A hand-edited row with hp <= 0 would Instantiate
	// to an Enemy{HP:0, MaxHP:0, Alive:true} — an "alive corpse" that
	// enemyAlive() (keyed on Alive, not HP) counts as a live combatant, so
	// the encounter can never be won until the player manually whittles it
	// negative. Refuse it at load instead, the same way AreaFromMapFile
	// rejects bad dimensions.
	if ce.HP <= 0 {
		return CustomEnemyDef{}, fmt.Errorf("custom enemy %q has non-positive HP (%d)", ce.Name, ce.HP)
	}
	// Symmetric with MapCustomEnemyFromDef's encode-side guard: reject negative
	// MP at load too, so a hand-edited row can't load yet be unsaveable.
	if ce.MP < 0 {
		return CustomEnemyDef{}, fmt.Errorf("custom enemy %q has negative MP (%d)", ce.Name, ce.MP)
	}
	// Mirror the static-registry init guards (enemies.go) for hand-edited
	// rows via the shared validateEnemyStatBounds so the two paths can't
	// drift on bounds. Refuse bad data at load rather than letting it reach
	// combat math.
	if err := validateEnemyStatBounds(ce.Name, ce.SkillCastChance, ce.Armor, ce.MDef, ce.AttackDamage, ce.XPValue, ce.SpellPower, ce.Tier); err != nil {
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

// MapCustomEnemyFromDef converts a core custom enemy definition back to the
// mapfile row shape, validating every registry-backed field on the way out.
func MapCustomEnemyFromDef(ce CustomEnemyDef) (mapfile.MapCustomEnemy, error) {
	baseName, ok := EnemyKindName(ce.BaseKind)
	if !ok {
		return mapfile.MapCustomEnemy{}, fmt.Errorf("custom enemy %q has unknown base kind %d", ce.Name, int(ce.BaseKind))
	}
	// Validate the numeric bounds on the way OUT, matching the map LOADER
	// (parseCustomEnemyLine rejects negatives; CustomEnemyDefFromMap re-checks via
	// validateEnemyStatBounds). The editor clamps stats at 0, but a non-editor
	// writer (importer/script — see the lockstep note below) could otherwise
	// persist a negative field that the loader would then refuse, yielding an
	// unloadable map. MP isn't in the shared validator (the static registry has
	// no MP pool), so it's checked here alongside.
	if err := validateEnemyStatBounds(ce.Name, ce.SkillCastChance, ce.Armor, ce.MDef, ce.AttackDamage, ce.XPValue, ce.SpellPower, ce.Tier); err != nil {
		return mapfile.MapCustomEnemy{}, err
	}
	if ce.MP < 0 {
		return mapfile.MapCustomEnemy{}, fmt.Errorf("custom enemy %q has negative MP %d", ce.Name, ce.MP)
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
		MDef:            ce.MDef,
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
//
// LOCKSTEP SITES — a new authored custom-enemy field must be added in all of:
//  1. the CustomEnemyDef struct (above),
//  2. the mapfile.MapCustomEnemy struct (mapfile/mapfile.go) + its encode
//     format/field-count,
//  3. MapCustomEnemyFromDef (def -> mapfile row),
//  4. CustomEnemyDefFromMap (mapfile row -> def),
//  5. this Definition() (def -> runtime EnemyDefinition), and
//  6. Instantiate() if the field affects the materialized Enemy.
//
// The encode<->decode pair (3 & 4) is guarded by
// TestCustomEnemyDefMapRoundTrip; the def->runtime pair (5 & 6) is guarded by
// TestCustomEnemyDefToRuntime — so a dropped field on either path fails loudly.
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

// Instantiate materializes a runtime Enemy from this def. Kind stays the base
// kind for renderer lookup; DefinitionOverride carries the authored stats and
// loadout for EnemyInfoFor readers.
func (d CustomEnemyDef) Instantiate() Enemy {
	def := d.Definition()
	// Scale spawn HP by the global difficulty dial, exactly as NewEnemy does for
	// base kinds — otherwise custom foes would get scaled damage (via
	// EnemyBasicDamage / enemySpellDamage, which read the override) but baseline
	// HP, leaving them inconsistently squishy as EnemyDifficultyMul rises.
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

// packMemberCustom resolves the custom enemy a pack slot names, if any: it reads
// the slot's custom-name tag and looks it up in the area's custom roster.
// ok=false means the slot is a plain built-in kind (read sp.Members[idx].Kind).
// Shared by PackMemberDefinition and PackMemberVisualKind so the name→roster
// resolution prologue lives in exactly one place.
func packMemberCustom(a AreaDefinition, sp PackSpawn, idx int) (CustomEnemyDef, bool) {
	if name := PackMemberCustomName(sp, idx); name != "" {
		return CustomEnemyByName(a.CustomEnemies, name)
	}
	return CustomEnemyDef{}, false
}

// PackMemberDefinition returns the effective definition for an authored pack
// member, resolving custom names through the containing area.
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

// PackMemberVisualKind returns the base kind whose sprite/color should
// represent an authored pack slot.
func PackMemberVisualKind(a AreaDefinition, sp PackSpawn, idx int) EnemyKind {
	if def, ok := packMemberCustom(a, sp, idx); ok {
		return def.BaseKind
	}
	if idx < 0 || idx >= len(sp.Members) {
		return EnemyRat
	}
	return sp.Members[idx].Kind
}

// PackSpawnLeaderSlot returns the highest-tier member slot for an authored
// pack, resolving custom enemy tiers as well as built-in tiers.
func PackSpawnLeaderSlot(a AreaDefinition, sp PackSpawn) int {
	return leaderSlot(len(sp.Members), func(i int) int { return PackMemberDefinition(a, sp, i).Tier })
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
//
// On-disk contract: a custom-enemy NAME token inside a whitespace-delimited
// .map row. PRESERVES case and punctuation — it only folds runs of
// whitespace to a single underscore (the loader's strings.Fields split is
// the only thing it must survive). Intentionally NOT the same as slugify
// (enemyvisual.go — lowercases + strips punctuation) or SanitizeFilename
// (areas.go — lowercases, restricts to [a-z0-9_-]). Don't swap one for
// another; each owns a different on-disk format.
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

package core

import (
	"fmt"
	"reflect"
)

type EnemyKind int

const (
	EnemyRat EnemyKind = iota
	EnemyBat
	EnemyDiseasedRat
	EnemyGoblin
	EnemyGoblinMage
	EnemyAmoeba
	EnemyVenusMantrap
	// CaveSpider/VampireBat/Wisp/StoneGolem/Necromancer each add a new mechanic
	// (Webbed, lifesteal, Confused, AoE phys, summons). Skeleton is the
	// Necromancer's raise-only summon target.
	EnemyCaveSpider
	EnemyVampireBat
	EnemyWisp
	EnemyStoneGolem
	EnemyNecromancer
	EnemySkeleton

	enemyKindCount // sentinel: EnemyKind cardinality (assertAppendOnly coverage)
)

// init pins every EnemyKind's serialized int value (the Bestiary persists as
// map[EnemyKind]BestiaryEntry, JSON-keyed by int). A mid-enum insert renumbers
// later kinds and silently mis-attributes saved entries — these literals trip a
// startup panic instead. Only APPENDING at the end is safe. Mirrors items.go.
func init() {
	assertAppendOnly("EnemyKind (renumbers saved bestiary entries)", int(enemyKindCount),
		EnemyRat, EnemyBat, EnemyDiseasedRat,
		EnemyGoblin, EnemyGoblinMage, EnemyAmoeba,
		EnemyVenusMantrap, EnemyCaveSpider, EnemyVampireBat,
		EnemyWisp, EnemyStoneGolem, EnemyNecromancer,
		EnemySkeleton,
	)
}

type EnemyDefinition struct {
	Kind EnemyKind
	// MapToken is the .map serialization token (+ MapAliases for legacy/no-underscore
	// forms). Deliberately distinct from the display Name strings, but kept on the
	// definition so the kind→token mapping has a single source (areas.go derives the
	// name lookups from here). MapToken must be non-empty; init asserts coverage.
	MapToken     string
	MapAliases   []string
	Name         string
	SingularName string
	PluralName   string
	SingularNoun string
	PluralNoun   string
	GroupName    string
	// Item is the steal loot (Thief's Steal), ItemNone if none. Typed as
	// ItemKind so the steal path equips it with no name→kind round-trip.
	Item         ItemKind
	MaxHP        int
	AttackDamage int
	// Stats is the full STR/DEX/INT/WIS/VIT/SPD block. SPD drives turn order +
	// frequency; DEX drives crit + dodge on basics; WIS shortens incoming
	// status. INT/STR aren't yet read by damage formulas (authored for a future
	// "derive damage from Stats" pass).
	Stats Stats
	// MDef damps incoming magic: ApplyMagicDefense clips magic-tagged damage by
	// MDef (floor 1); phys/heal/buff bypass.
	MDef int
	// Tier orders kinds by threat. The highest-Tier pack member is the field
	// figure (rest hidden until reveal); ties break by member order.
	Tier int
	// Level is the foe's power level, read only by the flee-chance math. 0 =
	// unauthored → EnemyLevel resolves to DefaultEnemyLevel.
	Level              int
	AttackVerbSingular string
	AttackVerbPlural   string
	// PoisonChance is the per-hit probability (0..1) a landed attack inflicts
	// Poison. Non-zero only on poison foes (diseased rat).
	PoisonChance float64
	// Armor is the flat phys damp: phys attacks resolve as max(damage-armor, 1);
	// magic/heal/buff bypass. The amoeba is the headline tanky foe.
	Armor int
	// XPValue is the per-member XP each living member earns on this kill (not
	// pooled). Roughly Tier*5.
	XPValue int
	// Skills lists the castable non-attack skills; the AI (enemyAIPickSkill)
	// rolls per-turn over them, empty falls through to melee.
	Skills []SkillID
	// SkillCastChance is the per-turn probability (0..1) of casting a Skill
	// instead of melee. 0 = never casts.
	SkillCastChance float64
	// SpellPower is the magic-attack stat for enemy spell damage (Firebolt:
	// SpellPower + Effect.Damage). 0 for non-casters.
	SpellPower int
	// LifestealPercent (0..1) of post-armor phys damage the enemy heals on its
	// OWN basic attack. Vampire Bat is the headline user. Caps at MaxHP.
	LifestealPercent float64
	// GoldMin/GoldMax bound the per-member gold roll on defeat, summed across
	// the pack in AwardBattleLoot. Both zero = no gold. Roughly Tier*3..Tier*6.
	GoldMin int
	GoldMax int
	// Drops is the per-defeat drop table (each Chance rolled independently into
	// shared inventory). Separate from Item (the steal loot).
	Drops []ItemDrop
	// Flying: immune to MELEE — a melee/unarmed basic or melee skill can't reach a
	// flyer at all (hard gate, no damage); only a ranged weapon or magic touches it.
	// Enemy attacks against the party are unaffected. See EnemyMeleeReachable.
	Flying bool
}

var enemyDefinitions = []EnemyDefinition{
	{
		Kind:               EnemyRat,
		MapToken:           "rat",
		Name:               "Feral Rat",
		SingularName:       "Rat",
		PluralName:         "Rats",
		SingularNoun:       "rat",
		PluralNoun:         "rats",
		GroupName:          "Rat Pack",
		Item:               ItemCheese,
		MaxHP:              10,
		AttackDamage:       3,
		Stats:              Stats{STR: 2, DEX: 3, INT: 0, WIS: 1, VIT: 2, SPD: 4},
		Tier:               1,
		XPValue:            5,
		GoldMin:            1,
		GoldMax:            3,
		AttackVerbSingular: "snaps",
		AttackVerbPlural:   "snap",
	},
	{
		Kind:         EnemyBat,
		MapToken:     "bat",
		Name:         "Cave Bat",
		SingularName: "Bat",
		PluralName:   "Bats",
		SingularNoun: "bat",
		PluralNoun:   "bats",
		GroupName:    "Bat Swarm",
		// Bat jerky heals more than rat cheese (see ItemDefinition.HealAmount).
		Item:               ItemBatJerky,
		MaxHP:              7,
		AttackDamage:       2,
		Stats:              Stats{STR: 1, DEX: 5, INT: 0, WIS: 1, VIT: 1, SPD: 7},
		Tier:               2,
		XPValue:            8,
		GoldMin:            2,
		GoldMax:            4,
		Flying:             true,
		AttackVerbSingular: "bites",
		AttackVerbPlural:   "bite",
	},
	{
		Kind:         EnemyDiseasedRat,
		MapToken:     "diseased_rat",
		MapAliases:   []string{"diseasedrat"},
		Name:         "Diseased Rat",
		SingularName: "Diseased Rat",
		PluralName:   "Diseased Rats",
		SingularNoun: "diseased rat",
		PluralNoun:   "diseased rats",
		GroupName:    "Diseased Pack",
		// No loot — its meat is inedible.
		Item:               ItemNone,
		MaxHP:              12,
		AttackDamage:       3,
		Stats:              Stats{STR: 2, DEX: 3, INT: 0, WIS: 2, VIT: 3, SPD: 5},
		Tier:               3,
		XPValue:            12,
		GoldMin:            2,
		GoldMax:            5,
		AttackVerbSingular: "bites",
		AttackVerbPlural:   "bite",
		PoisonChance:       DiseasedRatPoisonChance, // per bite; tuned in config.go
	},
	{
		Kind:               EnemyGoblin,
		MapToken:           "goblin",
		Name:               "Goblin",
		SingularName:       "Goblin",
		PluralName:         "Goblins",
		SingularNoun:       "goblin",
		PluralNoun:         "goblins",
		GroupName:          "Goblin Band",
		Item:               ItemCheese,
		MaxHP:              14,
		AttackDamage:       4,
		Stats:              Stats{STR: 3, DEX: 2, INT: 0, WIS: 1, VIT: 3, SPD: 5},
		Tier:               3,
		XPValue:            14,
		GoldMin:            3,
		GoldMax:            6,
		Drops:              []ItemDrop{{Kind: ItemCheese, Chance: 0.30}},
		AttackVerbSingular: "stabs",
		AttackVerbPlural:   "stab",
	},
	{
		Kind:         EnemyGoblinMage,
		MapToken:     "goblin_mage",
		MapAliases:   []string{"goblinmage"},
		Name:         "Goblin Mage",
		SingularName: "Goblin Mage",
		PluralName:   "Goblin Mages",
		SingularNoun: "goblin mage",
		PluralNoun:   "goblin mages",
		GroupName:    "Mage Coven",
		Item:         ItemBatJerky,
		// Squishy spellcaster — the danger is the spells, not the wand-whack.
		MaxHP:              9,
		AttackDamage:       2,
		Stats:              Stats{STR: 1, DEX: 2, INT: 6, WIS: 4, VIT: 2, SPD: 4},
		MDef:               3,
		Tier:               4,
		XPValue:            20,
		AttackVerbSingular: "swings a wand at",
		AttackVerbPlural:   "swing wands at",
		// Firebolt (damage) + Sleep (disabler); enemyAIPickSkill picks uniformly.
		Skills:          []SkillID{SkillFirebolt, SkillSleep},
		SkillCastChance: GoblinMageCastChance,
		SpellPower:      6,
		GoldMin:         5,
		GoldMax:         10,
		Drops:           []ItemDrop{{Kind: ItemBatJerky, Chance: 0.25}},
	},
	{
		Kind:         EnemyAmoeba,
		MapToken:     "amoeba",
		Name:         "Stone Amoeba",
		SingularName: "Amoeba",
		PluralName:   "Amoebae",
		SingularNoun: "amoeba",
		PluralNoun:   "amoebae",
		GroupName:    "Amoeba Cluster",
		Item:         ItemNone,
		// Low HP, very high armor: phys whiffs to 1, magic shreds. High WIS shrugs off
		// most status-spam (DoTs/CC) — a crit (post-armor) is the reliable answer.
		MaxHP:              8,
		AttackDamage:       2,
		Stats:              Stats{STR: 1, DEX: 0, INT: 0, WIS: 12, VIT: 2, SPD: 2},
		Tier:               3,
		XPValue:            16,
		Armor:              8,
		GoldMin:            2,
		GoldMax:            5,
		AttackVerbSingular: "engulfs",
		AttackVerbPlural:   "engulf",
	},
	{
		Kind:         EnemyVenusMantrap,
		MapToken:     "venus_mantrap",
		MapAliases:   []string{"mantrap", "venusmantrap"},
		Name:         "Venus Mantrap",
		SingularName: "Mantrap",
		PluralName:   "Mantraps",
		SingularNoun: "mantrap",
		PluralNoun:   "mantraps",
		GroupName:    "Mantrap Grove",
		Item:         ItemNone,
		// Slow tanky lurker: a heavy bite plus the signature Ingest that pulls a
		// member out of the fight until the plant dies.
		MaxHP:              22,
		AttackDamage:       5,
		Stats:              Stats{STR: 4, DEX: 1, INT: 0, WIS: 2, VIT: 5, SPD: 3},
		MDef:               1,
		Tier:               4,
		XPValue:            22,
		AttackVerbSingular: "snaps at",
		AttackVerbPlural:   "snap at",
		Skills:             []SkillID{SkillIngest},
		// Always Ingest when holding no prey (threat identity). Once it has
		// someone, usableEnemySkills strips Ingest (one prey per plant) so it
		// falls through to melee: round 1 ingest, later rounds bite.
		SkillCastChance: MantrapIngestCastChance,
		GoldMin:         6,
		GoldMax:         12,
	},
	{
		Kind:               EnemyCaveSpider,
		MapToken:           "cave_spider",
		MapAliases:         []string{"spider", "cavespider"},
		Name:               "Cave Spider",
		SingularName:       "Cave Spider",
		PluralName:         "Cave Spiders",
		SingularNoun:       "cave spider",
		PluralNoun:         "cave spiders",
		GroupName:          "Spider Nest",
		Item:               ItemNone,
		MaxHP:              11,
		AttackDamage:       3,
		Stats:              Stats{STR: 2, DEX: 4, INT: 0, WIS: 2, VIT: 3, SPD: 5},
		Tier:               3,
		XPValue:            13,
		AttackVerbSingular: "bites",
		AttackVerbPlural:   "bite",
		// Web applies Webbed (half-SPD, ingest-immune) for SpiderWebbedTurns.
		Skills:          []SkillID{SkillWeb},
		SkillCastChance: SpiderWebCastChance,
		GoldMin:         2,
		GoldMax:         5,
	},
	{
		Kind:               EnemyVampireBat,
		MapToken:           "vampire_bat",
		MapAliases:         []string{"vampirebat"},
		Name:               "Vampire Bat",
		SingularName:       "Vampire Bat",
		PluralName:         "Vampire Bats",
		SingularNoun:       "vampire bat",
		PluralNoun:         "vampire bats",
		GroupName:          "Vampire Swarm",
		Item:               ItemBatJerky,
		MaxHP:              13,
		AttackDamage:       4,
		Stats:              Stats{STR: 3, DEX: 5, INT: 0, WIS: 1, VIT: 3, SPD: 6},
		Tier:               4,
		XPValue:            18,
		AttackVerbSingular: "drains",
		AttackVerbPlural:   "drain",
		// Bite heals VampireBatLifesteal × post-armor damage; rides plain melee,
		// so no Skills listed.
		LifestealPercent: VampireBatLifesteal,
		GoldMin:          4,
		GoldMax:          8,
		Flying:           true,
	},
	{
		Kind:         EnemyWisp,
		MapToken:     "wisp",
		MapAliases:   []string{"will_o_wisp", "willowisp"},
		Name:         "Will-o'-Wisp",
		SingularName: "Wisp",
		PluralName:   "Wisps",
		SingularNoun: "wisp",
		PluralNoun:   "wisps",
		GroupName:    "Wisp Cluster",
		Item:         ItemNone,
		// Fragile but fast; the Confuse cast is the real threat, not the bite.
		MaxHP:              6,
		AttackDamage:       1,
		Stats:              Stats{STR: 0, DEX: 6, INT: 4, WIS: 6, VIT: 1, SPD: 7},
		MDef:               4,
		Tier:               3,
		XPValue:            16,
		AttackVerbSingular: "flickers at",
		AttackVerbPlural:   "flicker at",
		Skills:             []SkillID{SkillConfuse},
		SkillCastChance:    WispConfuseCastChance,
		GoldMin:            3,
		GoldMax:            7,
		Flying:             true,
	},
	{
		Kind:         EnemyStoneGolem,
		MapToken:     "stone_golem",
		MapAliases:   []string{"golem", "stonegolem"},
		Name:         "Stone Golem",
		SingularName: "Golem",
		PluralName:   "Golems",
		SingularNoun: "golem",
		PluralNoun:   "golems",
		GroupName:    "Golem Pair",
		Item:         ItemNone,
		// Tier-5 elite: beefy HP, very high armor, slow. Stoneslam is an AoE phys
		// cast on every living slot (player armor clips it).
		MaxHP:              80,
		AttackDamage:       7,
		Stats:              Stats{STR: 6, DEX: 0, INT: 2, WIS: 3, VIT: 8, SPD: 1},
		MDef:               6,
		Tier:               5,
		Armor:              14,
		XPValue:            40,
		AttackVerbSingular: "smashes",
		AttackVerbPlural:   "smash",
		Skills:             []SkillID{SkillStoneslam},
		SkillCastChance:    StoneGolemSlamCastChance,
		SpellPower:         4,
		GoldMin:            15,
		GoldMax:            30,
		Drops:              []ItemDrop{{Kind: ItemWoodenShield, Chance: 0.20}},
	},
	{
		Kind:         EnemyNecromancer,
		MapToken:     "necromancer",
		MapAliases:   []string{"necro"},
		Name:         "Necromancer",
		SingularName: "Necromancer",
		PluralName:   "Necromancers",
		SingularNoun: "necromancer",
		PluralNoun:   "necromancers",
		GroupName:    "Crypt Coven",
		Item:         ItemNone,
		// Tier-5 boss, mid HP: focusing it ends the summoning loop. Firebolt for
		// chip + RaiseBones (capped via the skill's PerBattleCastLimit).
		MaxHP:              26,
		AttackDamage:       3,
		Stats:              Stats{STR: 1, DEX: 2, INT: 6, WIS: 5, VIT: 4, SPD: 4},
		MDef:               5,
		Tier:               5,
		XPValue:            36,
		AttackVerbSingular: "incants at",
		AttackVerbPlural:   "incant at",
		Skills:             []SkillID{SkillRaiseBones, SkillFirebolt},
		SkillCastChance:    NecromancerCastChance,
		SpellPower:         5,
		GoldMin:            12,
		GoldMax:            24,
		Drops:              []ItemDrop{{Kind: ItemBrassAmulet, Chance: 0.25}},
	},
	{
		Kind:         EnemySkeleton,
		MapToken:     "skeleton",
		Name:         "Skeleton",
		SingularName: "Skeleton",
		PluralName:   "Skeletons",
		SingularNoun: "skeleton",
		PluralNoun:   "skeletons",
		GroupName:    "Bone Mob",
		Item:         ItemNone,
		// Tier-2 grunt, normally a Necromancer raise. Lean — cleared in a hit or
		// two but burns the player's actions. No skills.
		MaxHP:              10,
		AttackDamage:       3,
		Stats:              Stats{STR: 2, DEX: 2, INT: 0, WIS: 0, VIT: 2, SPD: 4},
		Tier:               2,
		XPValue:            6,
		AttackVerbSingular: "rakes",
		AttackVerbPlural:   "rake",
	},
}

// enemyByKind is the O(1) lookup for enemyDefinitions, built once at init.
// Hand-built (not via BuildRegistry) because it stores POINTERS into the
// backing array, not ~200-byte value copies — so the narrow field accessors
// (enemyGoverningDef etc.) read one field through a stable pointer per frame.
var enemyByKind = func() map[EnemyKind]*EnemyDefinition {
	m := make(map[EnemyKind]*EnemyDefinition, len(enemyDefinitions))
	for i := range enemyDefinitions {
		m[enemyDefinitions[i].Kind] = &enemyDefinitions[i]
	}
	return m
}()

// Validate the static registry at init: probability fields ride [0,1], gold
// bounds are non-negative + ordered, drops name real items. Bad data here is a
// programmer error, so panic.
func init() {
	// Duplicate Kind: enemyByKind last-write-wins, so EnemyInfo would silently
	// return the second row while EnemyKinds() lists both — panic instead.
	seenKind := make(map[EnemyKind]struct{}, len(enemyDefinitions))
	for _, def := range enemyDefinitions {
		if _, dup := seenKind[def.Kind]; dup {
			panic(fmt.Sprintf("core/enemies: duplicate EnemyKind %d (%q) in enemyDefinitions", def.Kind, def.Name))
		}
		seenKind[def.Kind] = struct{}{}
	}
	for _, def := range enemyDefinitions {
		// Shared with the custom-enemy loader (customenemy.go).
		if err := validateEnemyStatBounds(enemyStatBounds{
			Name:            def.Name,
			SkillCastChance: def.SkillCastChance,
			PoisonChance:    def.PoisonChance,
			Armor:           def.Armor,
			MDef:            def.MDef,
			AttackDamage:    def.AttackDamage,
			XPValue:         def.XPValue,
			SpellPower:      def.SpellPower,
			Tier:            def.Tier,
		}); err != nil {
			panic("core/enemies: " + err.Error())
		}
		if def.GoldMin < 0 || def.GoldMax < 0 {
			panic("core/enemies: " + def.Name + " GoldMin/GoldMax must be non-negative")
		}
		if def.GoldMax < def.GoldMin {
			panic("core/enemies: " + def.Name + " GoldMax must be >= GoldMin")
		}
		for _, d := range def.Drops {
			if !ValidChance(d.Chance) {
				panic("core/enemies: " + def.Name + " drop Chance must be in [0, 1]")
			}
			if _, ok := ItemInfoOk(d.Kind); !ok {
				panic("core/enemies: " + def.Name + " drops an unregistered item kind")
			}
		}
	}
}

// EnemyKinds returns a defensive copy of the registry in declaration order.
func EnemyKinds() []EnemyDefinition {
	out := make([]EnemyDefinition, len(enemyDefinitions))
	copy(out, enemyDefinitions)
	return out
}

// EnemyKindCount returns the registered kind count without EnemyKinds()' copy.
func EnemyKindCount() int {
	return len(enemyDefinitions)
}

// TheEnemy returns the article-prefixed singular ("The rat"). Centralised so a
// future per-enemy article override is one method.
func TheEnemy(def EnemyDefinition) string {
	return theNoun(def.SingularNoun)
}

// theNoun binds the definite article to a bare enemy noun (one place for the article).
func theNoun(noun string) string {
	return "The " + noun
}

// EnemyDisplayName returns the "The <noun>" display string for a live instance.
func EnemyDisplayName(e *Enemy) string {
	return TheEnemy(EnemyInfoFor(*e))
}

// TheFoe is the lowercase object-position form ("the rat") for a live instance —
// the single source for the mid-sentence "the <noun>" that combat log lines spell
// by hand. Use TheEnemy/EnemyDisplayName at sentence start (capitalized).
func TheFoe(e *Enemy) string {
	return "the " + EnemySingularNoun(e)
}

// EnemyInfoOk is the validating sibling of EnemyInfo: (def, true) for a
// registered kind, (zero, false) otherwise.
func EnemyInfoOk(kind EnemyKind) (EnemyDefinition, bool) {
	if def, ok := enemyByKind[kind]; ok {
		return *def, true
	}
	return EnemyDefinition{}, false
}

func EnemyInfo(kind EnemyKind) EnemyDefinition {
	if def, ok := enemyByKind[kind]; ok {
		return *def
	}
	// Unreachable for valid data — an in-memory corrupt Kind. Panic rather than
	// silently ship a 1-HP placeholder into combat.
	panic(fmt.Sprintf("core: EnemyInfo called with unregistered kind %d — add it to enemyDefinitions", int(kind)))
}

// EnemyInfoFor returns the kind's definition with the instance's live MaxHP
// (if set) and Armor applied — for renderer/log text about a live enemy.
func EnemyInfoFor(enemy Enemy) EnemyDefinition {
	def := *enemyGoverningDef(&enemy)
	if enemy.HasDefinitionOverride {
		def.Kind = enemy.Kind
	}
	if enemy.MaxHP > 0 {
		def.MaxHP = enemy.MaxHP
	}
	// Armor 0 is meaningful (stripped, or authored at 0), so always copy the
	// live field through unlike MaxHP.
	def.Armor = enemy.Armor
	return def
}

// enemyGoverningDef returns a POINTER to the governing definition (embedded
// DefinitionOverride for a custom enemy, else the registry row) without copying
// the ~200-byte struct — for the per-frame narrow accessors. Does NOT apply live
// MaxHP/Armor (use EnemyInfoFor). Panics on an unregistered kind.
func enemyGoverningDef(e *Enemy) *EnemyDefinition {
	if e.HasDefinitionOverride {
		return &e.DefinitionOverride
	}
	if def, ok := enemyByKind[e.Kind]; ok {
		return def
	}
	panic(fmt.Sprintf("core: enemy carries unregistered kind %d — add it to enemyDefinitions", int(e.Kind)))
}

// EnemyName / EnemySingularName read one field via enemyGoverningDef without
// materializing the whole struct — the cheap per-frame roster/turn-queue accessors.
func EnemyName(e *Enemy) string         { return enemyGoverningDef(e).Name }
func EnemySingularName(e *Enemy) string { return enemyGoverningDef(e).SingularName }

// EnemyFlying reads the flying flag through the governing-def POINTER — the
// per-frame melee-reach predicates (EnemyMeleeReachable, target picker) call this
// per enemy cell, so it must not copy the whole EnemyDefinition like EnemyInfoFor.
func EnemyFlying(e *Enemy) bool { return enemyGoverningDef(e).Flying }

// EffectiveEnemyStats returns combat stats with active debuffs folded in (the
// enemy-side mirror of EffectiveStats): summed debuff deltas added per-stat,
// each floored at 0. Fast-path when no debuff is active.
func EffectiveEnemyStats(e *Enemy) Stats {
	// Base Stats through the governing-def pointer (per combat roll, only the
	// 24-byte block needed) — no full-struct copy.
	out := enemyGoverningDef(e).Stats
	if len(e.Debuffs) == 0 {
		return out
	}
	delta, _, _ := SumStatusMods(e.Debuffs)
	return addStatsFloored(out, delta)
}

// EffectiveEnemyDefenses returns effective Armor (from per-instance e.Armor) and
// MDef (from the definition) with their StatusMod deltas added and floored at 0 —
// the enemy-side mirror of EffectiveDefenses. Runs per mitigated hit.
func EffectiveEnemyDefenses(e *Enemy) (armor, mdef int) {
	baseMDef := enemyGoverningDef(e).MDef
	if len(e.Debuffs) == 0 {
		return MaxZero(e.Armor), MaxZero(baseMDef)
	}
	_, armorDelta, mdefDelta := SumStatusMods(e.Debuffs)
	return MaxZero(e.Armor + armorDelta), MaxZero(baseMDef + mdefDelta)
}

// StampEnemyDebuff adds or refreshes a skill's stat debuff on the enemy for
// effect.BuffTurns turns. Returns false (no-op) for a nil target or a skill with
// no debuff (BuffTurns == 0); callers use the bool to drive an "afflicted" log.
// Different skills' debuffs SUM (in EffectiveEnemyStats); the same skill refreshes.
// Gate damage-then-stamp callers on !defeated so a kill leaves no dangling debuff.
func StampEnemyDebuff(e *Enemy, source SkillID, effect SkillEffect) bool {
	if e == nil || effect.BuffTurns <= 0 {
		return false
	}
	e.Debuffs = applyStatusMod(e.Debuffs, StatusMod{Source: source, Stats: effect.BuffStats, Armor: effect.BuffArmor, MDef: effect.BuffMDef, Turns: effect.BuffTurns})
	return true
}

// DispelEnemyBuffs strips every NET-BENEFICIAL StatusMod from the enemy (the
// Wizard's Dispel) and returns how many were removed. Negative debuffs (our
// Cripple/Blind/etc.) are kept — we don't want to cleanse the foe. Forward-
// compatible: enemies carry no self-buffs today, so it usually finds nothing.
func DispelEnemyBuffs(e *Enemy) int {
	if e == nil || len(e.Debuffs) == 0 {
		return 0
	}
	kept := e.Debuffs[:0]
	removed := 0
	for _, mod := range e.Debuffs {
		if StatusModsNetBeneficial([]StatusMod{mod}) {
			removed++
			continue
		}
		kept = append(kept, mod)
	}
	e.Debuffs = kept
	return removed
}

// EnemyLevel is the foe's level for the flee-chance math: the authored Level if
// set, else the authored Tier (which already encodes relative threat, so flee odds
// scale with how dangerous the pack actually is instead of a uniform level 1), else
// DefaultEnemyLevel.
func EnemyLevel(e *Enemy) int {
	// Through the governing-def pointer (Level/Tier aren't override-adjusted).
	def := enemyGoverningDef(e)
	if def.Level > 0 {
		return def.Level
	}
	if def.Tier > 0 {
		return def.Tier
	}
	return DefaultEnemyLevel
}

// ScaleEnemyDifficulty applies the global EnemyDifficulty multiplier
// (EnemyDifficultyNum/Den) to a base stat: round-to-nearest, floored at 1 for any
// positive input. The single seam behind "make foes harder" (HP, basic + spell damage).
func ScaleEnemyDifficulty(n int) int {
	// int64 so a large custom-enemy stat can't overflow the intermediate multiply.
	scaled := int((int64(n)*int64(EnemyDifficultyNum) + int64(EnemyDifficultyDen)/2) / int64(EnemyDifficultyDen))
	if n > 0 && scaled < 1 {
		scaled = 1
	}
	return scaled
}

// EnemyBasicDamage is the enemy's basic-attack damage (difficulty-scaled). The
// seam for a future "derive from Stats.STR" pass.
func EnemyBasicDamage(e *Enemy) int {
	return ScaleEnemyDifficulty(enemyGoverningDef(e).AttackDamage)
}

func NewEnemy(kind EnemyKind) Enemy {
	return newEnemyFromDef(kind, EnemyInfo(kind))
}

// newEnemyFromDef builds the base spawn Enemy from a definition: difficulty-scaled
// HP, alive, with the def's drop item + armor. The single spawn-construction site
// shared by NewEnemy and CustomEnemyDef.Instantiate (which augments the override
// fields). SkillCastCount stays nil — nil-map reads return 0, so the lookup is
// safe before any cast; handleEnemyRaiseBones lazily allocates.
func newEnemyFromDef(kind EnemyKind, def EnemyDefinition) Enemy {
	maxHP := ScaleEnemyDifficulty(def.MaxHP)
	return Enemy{
		Kind:  kind,
		HP:    maxHP,
		MaxHP: maxHP,
		Alive: true,
		Item:  def.Item,
		Armor: def.Armor,
	}
}

// clearEnemyAnimTimers zeros an enemy's transient animation state (lunge / the
// shared take-a-hit HitAnim quintet / death-fade) — the enemy twin of
// clearMemberAnimTimers. DeathFade is the enemy-only field (no party equivalent).
func clearEnemyAnimTimers(e *Enemy) {
	e.AttackBump = 0
	e.HitAnim = HitAnim{}
	e.DeathFade = 0
	// Formation-slide transients: drop any in-flight glide AND the cached placement, so
	// a re-engaged pack (Flee → fight again) re-seats its slots fresh without sliding in.
	e.SlotSlide = 0
	e.SlideFromRow, e.SlideFromSlot, e.SlideFromCount = RowFront, 0, 0
	e.placedRow, e.placedSlot, e.placedCount, e.placedValid = RowFront, 0, 0, false
}

// ClearEnemyCombatTransients clears an enemy's timed combat statuses (every *Turns
// counter + Debuffs) and its taunt back-reference — the enemy mirror of
// ClearMemberTransientStatuses. Used on a Flee (the revived foe must be pristine);
// distinct from battle.clearEnemyStatusesOnDeath, which KEEPS TauntTurns (moot on a
// corpse). TauntedBy is only meaningful while TauntTurns>0, so clearing the counter
// makes it moot; it stays 0 (no -1 sentinel exists for it). The init assert below
// pins this to the reflected *Turns field set so a new counter can't survive a Flee.
func ClearEnemyCombatTransients(e *Enemy) {
	e.BurnTurns = 0
	e.SleepTurns = 0
	e.StunTurns = 0
	e.PoisonTurns = 0
	e.BleedTurns = 0
	e.TauntTurns = 0
	e.TauntedBy = 0
	e.Debuffs = nil
}

// clearedEnemyTurnsCounters names every *Turns field ClearEnemyCombatTransients
// zeros; the init assert below trips at startup if Enemy gains a *Turns int counter
// not listed here (so a new status can't silently survive a Flee).
var clearedEnemyTurnsCounters = map[string]bool{
	"BurnTurns":   true,
	"SleepTurns":  true,
	"StunTurns":   true,
	"PoisonTurns": true,
	"BleedTurns":  true,
	"TauntTurns":  true,
}

func init() {
	forEachTurnsField(reflect.TypeOf(Enemy{}), func(name string) {
		if !clearedEnemyTurnsCounters[name] {
			panic("core: Enemy." + name + " is an unlisted timed-status counter — add it to ClearEnemyCombatTransients/clearedEnemyTurnsCounters so it can't survive a Flee")
		}
	})
}

// RestorePackFull resets a pack to its spawn condition: drops mid-fight summons
// (Raise Bones) so the roster reverts to what was authored, then for the survivors
// restores full HP, revives the downed, restores steal loot + Armor from the
// definition, and clears all combat statuses / hit-animations. Re-shunts the
// formation so the revived ranks pack front-first. Used on a successful Flee —
// abandoning a fight forfeits ALL progress against the pack (escape, not attrition).
func RestorePackFull(pack *Pack) {
	if pack == nil {
		return
	}
	// Drop summoned members in place (preserve authored order).
	kept := pack.Members[:0]
	for _, m := range pack.Members {
		if !m.Summoned {
			kept = append(kept, m)
		}
	}
	pack.Members = kept
	for i := range pack.Members {
		m := &pack.Members[i]
		def := enemyGoverningDef(m)
		m.HP = m.MaxHP
		m.Alive = true
		m.Item = def.Item
		// Source Armor from the governing def, NOT EnemyInfoFor — the latter copies
		// the live (possibly Corrosive-Vial-stripped) value through, so it would be a
		// no-op here and leave a debuffed foe stripped after a "full" restore.
		m.Armor = def.Armor
		ClearEnemyCombatTransients(m)
		m.SkillCastCount = nil
		clearEnemyAnimTimers(m)
	}
	ShuntEnemyFormation(pack.Members)
}

// EnemySingularNoun reads the bare singular noun ("rat") via enemyGoverningDef
// without materializing the whole struct — matches its EnemyName/EnemyDisplayName
// siblings (all *Enemy) so the log path never copies the ~200-byte Enemy.
func EnemySingularNoun(e *Enemy) string {
	return enemyGoverningDef(e).SingularNoun
}

// These battle-string builders take *GameState to avoid copying the whole
// struct on every per-frame HUD-string call.
func BattleEnemyInfo(g *GameState) EnemyDefinition {
	members := BattleMembers(g)
	if g.Battle.EnemyIndex >= 0 && g.Battle.EnemyIndex < len(members) {
		return EnemyInfoFor(members[g.Battle.EnemyIndex])
	}
	if len(members) > 0 {
		return EnemyInfoFor(members[0])
	}
	return EnemyInfo(EnemyRat)
}

func BattleEnemyGroupName(g *GameState) string {
	return BattleEnemyInfo(g).GroupName
}

func BattleEnemyTargetStatus(g *GameState, ordinal, total int) string {
	def := BattleEnemyInfo(g)
	return fmt.Sprintf("Targeting %s %d of %d.", def.SingularNoun, ordinal, total)
}

// BattleEncounterMessage is the action-log line when a fight opens — generic ("foe"),
// matching the generic splash title; the spelled count title-cases for sentence start.
func BattleEncounterMessage(g *GameState) string {
	count := LivingBattleCount(g)
	if count <= 1 {
		return "A foe blocks the way."
	}
	return fmt.Sprintf("%s foes close in.", countWordTitle(count))
}

// countWordTitle spells a small count as a title-cased word (sentence-start friendly),
// digits fallback past the table. Packs cap at EnemyPackCap, so 2–5 cover real use.
func countWordTitle(n int) string {
	words := [...]string{"Zero", "One", "Two", "Three", "Four", "Five", "Six", "Seven", "Eight", "Nine", "Ten"}
	if n >= 0 && n < len(words) {
		return words[n]
	}
	return fmt.Sprintf("%d", n)
}

// BattleEncounterTitle is the splash headline: a plain "Battle!", or "Ambushed!" when
// the fight was entered from the side/back (any non-front engage).
func BattleEncounterTitle(g *GameState) string {
	if g.Battle.EngageSide != EngageFront {
		return "Ambushed!"
	}
	return "Battle!"
}

func LastBattleEnemyFallsMessage() string {
	return "The last foe falls."
}

func BattleLossMessage(g *GameState) string {
	if LivingBattleCount(g) <= 1 {
		return "The foe drives the party back. Press to recover."
	}
	return "The foes drive the party back. Press to recover."
}

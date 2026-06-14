package core

import "fmt"

type EnemyKind int

const (
	EnemyRat EnemyKind = iota
	EnemyBat
	EnemyDiseasedRat
	EnemyGoblin
	EnemyGoblinMage
	EnemyAmoeba
	EnemyVenusMantrap
	// Roster expansion: six new kinds bringing the bestiary to 13.
	// CaveSpider / VampireBat / Wisp / StoneGolem / Necromancer
	// each introduce a mechanic the existing roster doesn't cover
	// (Webbed status, lifesteal, Confused status, AoE phys casts, and
	// mid-fight summons respectively). Skeleton is the Necromancer's
	// summon target — a tier-2 grunt that exists only via raises.
	EnemyCaveSpider
	EnemyVampireBat
	EnemyWisp
	EnemyStoneGolem
	EnemyNecromancer
	EnemySkeleton
)

type EnemyDefinition struct {
	Kind         EnemyKind
	Name         string
	SingularName string
	PluralName   string
	SingularNoun string
	PluralNoun   string
	GroupName    string
	// Item is the kind this enemy carries as steal loot (Thief's Steal),
	// ItemNone for enemies that carry nothing. Typed as ItemKind (not a
	// name string) so the steal path equips it directly with no
	// name→kind round-trip.
	Item         ItemKind
	MaxHP        int
	AttackDamage int
	// Stats is the full STR/DEX/INT/WIS/VIT/SPD block — symmetric
	// with the party side. SPD drives turn-order AND turn frequency
	// (replaces the legacy standalone Speed field). DEX drives crit
	// on basic attacks and dodge against incoming basic attacks. WIS
	// shortens incoming status durations via ShortenStatusDuration.
	// INT and STR are not yet read by damage formulas (AttackDamage
	// / SpellPower still drive raw damage) but are authored so a
	// future "derive damage from Stats" pass can retire those scalar
	// fields without an authoring migration.
	Stats Stats
	// MDef is the magic-defense damp on incoming spells. Symmetric
	// with Armor on the phys side: ApplyMagicDefense clips magic-
	// tagged damage by MDef (floor 1); phys/heal/buff bypass.
	MDef int
	// Tier orders enemy kinds by threat. The highest-Tier member of a pack
	// is the figure shown on the field (everyone else is hidden until the
	// battle reveals them). Ties break by member order within the pack.
	Tier int
	// Level is the foe's power level, read only by the flee-chance math today
	// (party avg level vs pack avg level). 0 means "unauthored" — EnemyLevel
	// resolves it to DefaultEnemyLevel, so every kind has a sane level without
	// per-row authoring. No XP/scaling wiring reads it yet.
	Level              int
	AttackVerbSingular string
	AttackVerbPlural   string
	// PoisonChance is the per-hit probability (0..1) that this enemy's
	// landed attack will inflict the Poison status on the target. Zero for
	// most kinds; non-zero only on poison-themed foes like the diseased rat.
	PoisonChance float64
	// Armor is the flat phys-damage damp this enemy carries. Phys-tagged
	// attacks resolve as max(damage - armor, 1); magic / heal / buff
	// bypass. Defaults to 0 for most kinds. The amoeba is the headline
	// tanky foe (high armor, low HP — magic shreds it, melee chips).
	Armor int
	// XPValue is the experience reward each living party member earns
	// when this enemy is defeated. Per-character (not pooled): every
	// member who's alive at the moment of the kill gets the full XP.
	// Roughly Tier * 5, scaled by perceived difficulty.
	XPValue int
	// Skills lists the non-attack skills the enemy can cast. The AI
	// rolls a per-turn behaviour table (enemyAIPickSkill) over this
	// slice; an empty/nil slice falls through to plain melee. The
	// goblin mage is the first user — Firebolt + Sleep.
	Skills []SkillID
	// SkillCastChance is the per-turn probability (0..1) that this
	// enemy casts one of its Skills instead of plain melee. Mirrors
	// PoisonChance's shape so a per-kind probability lives next to
	// the loadout it modifies. 0 = never casts (any non-caster enemy
	// can leave this blank). Goblin mage sets a meaningful value.
	SkillCastChance float64
	// SpellPower is the "magic attack" stat used by enemy spell-damage
	// formulas (resolveEnemySpell's Firebolt path: damage = SpellPower
	// + Effect.Damage). Defaults to 0 — non-casters never read it. A
	// future enemy with INT-equivalent magic damage sets this so the
	// physical AttackDamage stays separate from the spell scaling.
	SpellPower int
	// LifestealPercent is the 0..1 fraction of phys-damage-dealt that
	// the enemy heals on its OWN basic attack. The Vampire Bat is the
	// headline user; other kinds leave this at zero. Applied on the
	// post-armor damage value so the heal scales with what actually
	// landed, not the rolled raw — armor-shrugged 1-damage hits still
	// pass the floor through to the heal in proportion. Heal caps at
	// MaxHP so a sustained drain doesn't overcap.
	LifestealPercent float64
	// GoldMin / GoldMax bound the gold this enemy pays out on defeat —
	// a uniform roll in [GoldMin, GoldMax] per member, summed across the
	// pack in AwardBattleLoot. Both zero = no gold (Skeleton raises stay
	// near-worthless so the necromancer summon isn't a gold faucet).
	// Roughly Tier * 3 .. Tier * 6, scaled by perceived threat.
	GoldMin int
	GoldMax int
	// Drops is the per-defeat item drop table — each entry rolls its
	// Chance independently and lands in the shared inventory on victory.
	// Separate from Item (the mid-fight steal loot, one per enemy). Empty
	// for most kinds; the tougher foes seed a small chance at gear.
	Drops []ItemDrop
	// Flying marks an airborne enemy. A melee basic attack against a
	// flyer takes a steep accuracy penalty (FlyingMeleeAccuracyPenalty)
	// unless the wielder's weapon strikes at range (WeaponIsRanged) —
	// the scaffold for "bring a bow to fight bats." Skills and enemy
	// attacks are unaffected. See MemberAttackHitsTarget.
	Flying bool
}

var enemyDefinitions = []EnemyDefinition{
	{
		Kind:               EnemyRat,
		Name:               "Feral Rat",
		SingularName:       "Rat",
		PluralName:         "Rats",
		SingularNoun:       "rat",
		PluralNoun:         "rats",
		GroupName:          "Rat Pack",
		Item:               ItemCheese,
		MaxHP:              10,
		AttackDamage:       3,
		Stats:              Stats{STR: 2, DEX: 3, INT: 0, WIS: 1, VIT: 2, SPD: 6},
		Tier:               1,
		XPValue:            5,
		GoldMin:            1,
		GoldMax:            3,
		AttackVerbSingular: "snaps",
		AttackVerbPlural:   "snap",
	},
	{
		Kind:         EnemyBat,
		Name:         "Cave Bat",
		SingularName: "Bat",
		PluralName:   "Bats",
		SingularNoun: "bat",
		PluralNoun:   "bats",
		GroupName:    "Bat Swarm",
		// Bat jerky heals more than rat cheese — see ItemDefinition.HealAmount;
		// the bat is also faster and harder to land hits on, so the loot is the
		// payoff for fighting (or robbing) the trickier enemy.
		Item:               ItemBatJerky,
		MaxHP:              7,
		AttackDamage:       2,
		Stats:              Stats{STR: 1, DEX: 5, INT: 0, WIS: 1, VIT: 1, SPD: 9},
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
		Name:         "Diseased Rat",
		SingularName: "Diseased Rat",
		PluralName:   "Diseased Rats",
		SingularNoun: "diseased rat",
		PluralNoun:   "diseased rats",
		GroupName:    "Diseased Pack",
		// No carried loot — its meat is no good for eating.
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
		// DiseasedRatPoisonChance per bite. Pairs with the rat's higher
		// HP and damage to make a diseased pack the threat upgrade over a
		// plain rat pack. Tuning value lives in config.go.
		PoisonChance: DiseasedRatPoisonChance,
	},
	{
		Kind:               EnemyGoblin,
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
		Name:         "Goblin Mage",
		SingularName: "Goblin Mage",
		PluralName:   "Goblin Mages",
		SingularNoun: "goblin mage",
		PluralNoun:   "goblin mages",
		GroupName:    "Mage Coven",
		Item:         ItemBatJerky,
		// Lower HP than a regular goblin — squishy spellcaster. The
		// danger is in the spells, not the wand-whack.
		MaxHP:              9,
		AttackDamage:       2,
		Stats:              Stats{STR: 1, DEX: 2, INT: 6, WIS: 4, VIT: 2, SPD: 4},
		MDef:               3,
		Tier:               4,
		XPValue:            20,
		AttackVerbSingular: "swings a wand at",
		AttackVerbPlural:   "swing wands at",
		// Per-turn AI rolls over these: Firebolt is the damage option,
		// Sleep is the disabler. Order in the slice is irrelevant —
		// the battle package's enemyAIPickSkill picks uniformly.
		// Plain melee remains the SkillCastChance miss-roll fallback.
		Skills:          []SkillID{SkillFirebolt, SkillSleep},
		SkillCastChance: GoblinMageCastChance,
		SpellPower:      6,
		GoldMin:         5,
		GoldMax:         10,
		Drops:           []ItemDrop{{Kind: ItemBatJerky, Chance: 0.25}},
	},
	{
		Kind:         EnemyAmoeba,
		Name:         "Stone Amoeba",
		SingularName: "Amoeba",
		PluralName:   "Amoebae",
		SingularNoun: "amoeba",
		PluralNoun:   "amoebae",
		GroupName:    "Amoeba Cluster",
		Item:         ItemNone,
		// Low HP × very high armor: phys swings whiff to 1, magic
		// shreds. Sets up the "switch your party to magic when armor
		// shows up" lesson.
		MaxHP:              8,
		AttackDamage:       2,
		Stats:              Stats{STR: 1, DEX: 0, INT: 0, WIS: 0, VIT: 2, SPD: 2},
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
		Name:         "Venus Mantrap",
		SingularName: "Mantrap",
		PluralName:   "Mantraps",
		SingularNoun: "mantrap",
		PluralNoun:   "mantraps",
		GroupName:    "Mantrap Grove",
		Item:         ItemNone,
		// Slow, tanky, two-trick lurker: a heavy bite when the party gets
		// close, plus the signature Ingest that pulls a member out of the
		// fight until the plant is killed. The bite alone is mid-tier;
		// Ingest is what makes the encounter scary — losing your healer
		// for the rest of the battle changes the whole math.
		MaxHP:              22,
		AttackDamage:       5,
		Stats:              Stats{STR: 4, DEX: 1, INT: 0, WIS: 2, VIT: 5, SPD: 3},
		MDef:               1,
		Tier:               4,
		XPValue:            22,
		AttackVerbSingular: "snaps at",
		AttackVerbPlural:   "snap at",
		Skills:             []SkillID{SkillIngest},
		// Always reach for prey when no prey is held — that's the
		// mantrap's threat identity. Once it has someone, the
		// usableEnemySkills filter strips Ingest from the cast list
		// (each plant holds at most one prey) so the usable set is
		// empty and the mantrap falls through to plain melee bites
		// against the rest of the party. Result: round 1 → ingest,
		// later rounds → bite, kill the plant to free the prisoner.
		// An earlier 0.35 roll-to-cast made the mantrap bite ~2/3 of
		// the time even when starving, which read as "doesn't
		// prioritize ingest."
		SkillCastChance: 1.0,
		GoldMin:         6,
		GoldMax:         12,
	},
	{
		Kind:               EnemyCaveSpider,
		Name:               "Cave Spider",
		SingularName:       "Cave Spider",
		PluralName:         "Cave Spiders",
		SingularNoun:       "cave spider",
		PluralNoun:         "cave spiders",
		GroupName:          "Spider Nest",
		Item:               ItemNone,
		MaxHP:              11,
		AttackDamage:       3,
		Stats:              Stats{STR: 2, DEX: 4, INT: 0, WIS: 2, VIT: 3, SPD: 7},
		Tier:               3,
		XPValue:            13,
		AttackVerbSingular: "bites",
		AttackVerbPlural:   "bite",
		// SkillWeb is the tempo-control skill — applies Webbed (half-SPD,
		// can't be ingested while webbed) for SpiderWebbedTurns turns.
		// SkillCastChance is meaningful so the spider doesn't always web;
		// plain bites pad the rounds in between to keep cast pressure
		// from feeling spammy.
		Skills:          []SkillID{SkillWeb},
		SkillCastChance: SpiderWebCastChance,
		GoldMin:         2,
		GoldMax:         5,
	},
	{
		Kind:               EnemyVampireBat,
		Name:               "Vampire Bat",
		SingularName:       "Vampire Bat",
		PluralName:         "Vampire Bats",
		SingularNoun:       "vampire bat",
		PluralNoun:         "vampire bats",
		GroupName:          "Vampire Swarm",
		Item:               ItemBatJerky,
		MaxHP:              13,
		AttackDamage:       4,
		Stats:              Stats{STR: 3, DEX: 5, INT: 0, WIS: 1, VIT: 3, SPD: 8},
		Tier:               4,
		XPValue:            18,
		AttackVerbSingular: "drains",
		AttackVerbPlural:   "drain",
		// Lifesteal is the bat's defining trick — every successful bite
		// heals the bat for VampireBatLifesteal × damage-after-armor.
		// Reused tag SkillTagPhys (the bite is physical); no Skills are
		// listed because the lifesteal rides on plain melee, not a cast.
		LifestealPercent: VampireBatLifesteal,
		GoldMin:          4,
		GoldMax:          8,
		Flying:           true,
	},
	{
		Kind:         EnemyWisp,
		Name:         "Will-o'-Wisp",
		SingularName: "Wisp",
		PluralName:   "Wisps",
		SingularNoun: "wisp",
		PluralNoun:   "wisps",
		GroupName:    "Wisp Cluster",
		Item:         ItemNone,
		// Fragile but fast — high SPD lets it go before the party can
		// reliably burst it down, and its bite is just a placeholder
		// melee (the SkillConfuse cast is the real threat).
		MaxHP:              6,
		AttackDamage:       1,
		Stats:              Stats{STR: 0, DEX: 6, INT: 4, WIS: 6, VIT: 1, SPD: 9},
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
		Name:         "Stone Golem",
		SingularName: "Golem",
		PluralName:   "Golems",
		SingularNoun: "golem",
		PluralNoun:   "golems",
		GroupName:    "Golem Pair",
		Item:         ItemNone,
		// Tier-5 elite. Beefy HP, very high armor, slow as the amoeba.
		// Stoneslam is an AoE phys cast hitting every living party slot —
		// armor on the player side clips it, so a Defending Warrior eats
		// it well but the Wizard takes the full slap. Identifies as the
		// "active aggressor armor wall" complement to the Amoeba's
		// passive shrug-everything stance.
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
		Name:         "Necromancer",
		SingularName: "Necromancer",
		PluralName:   "Necromancers",
		SingularNoun: "necromancer",
		PluralNoun:   "necromancers",
		GroupName:    "Crypt Coven",
		Item:         ItemNone,
		// Tier-5 boss. Mid HP — squishy enough that focusing it ends the
		// summoning loop, but enough armor of position (always at the
		// back of a pack) that "kill the necro first" is a real choice.
		// Two skills: Firebolt for chip damage and RaiseBones for the
		// signature add-summon. The cap on raises lives on the skill
		// definition (PerBattleCastLimit) so future capped casters
		// reuse the field without an enemy-specific counter.
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
		Name:         "Skeleton",
		SingularName: "Skeleton",
		PluralName:   "Skeletons",
		SingularNoun: "skeleton",
		PluralNoun:   "skeletons",
		GroupName:    "Bone Mob",
		Item:         ItemNone,
		// Tier-2 grunt that exists only as a Necromancer raise (no
		// PackSpawn authoring path drops them directly today, though
		// authors can if they want a free skeleton). Stats are lean —
		// they're meant to be cleared in one or two hits but burn the
		// player's actions doing it. No skills.
		MaxHP:              10,
		AttackDamage:       3,
		Stats:              Stats{STR: 2, DEX: 2, INT: 0, WIS: 0, VIT: 2, SPD: 4},
		Tier:               2,
		XPValue:            6,
		AttackVerbSingular: "rakes",
		AttackVerbPlural:   "rake",
	},
}

// enemyByKind is the O(1) lookup map for enemyDefinitions, built once at
// init. EnemyInfo is called per-frame from the renderer (roster, popups),
// so the map matches the partyClassByID / skillByID pattern in party.go.
//
// Values are POINTERS into the enemyDefinitions backing array (stable — a
// package-level slice never reallocates) rather than copies, so the narrow
// field accessors (enemyGoverningDef → EnemyName / EnemySingularName /
// enemyBaseStats) read a single field through the pointer instead of copying
// the ~200-byte EnemyDefinition out of the map on every per-frame call.
var enemyByKind = func() map[EnemyKind]*EnemyDefinition {
	m := make(map[EnemyKind]*EnemyDefinition, len(enemyDefinitions))
	for i := range enemyDefinitions {
		m[enemyDefinitions[i].Kind] = &enemyDefinitions[i]
	}
	return m
}()

// Probability fields ride a [0, 1] contract — values past 1 roll
// "always" which is usually a typo (a designer meant 0.5 and wrote 5).
// Negative values would invert the gate silently. Panic at init so the
// bad data never reaches the gameplay loop. Used to be inline in the
// registry builder; lifted to a sibling init() block when the builder
// folded into the generic BuildRegistry helper.
func init() {
	// Guard against a duplicate Kind in the registry. BuildRegistry above
	// last-write-wins on a repeated key, so a copy-paste that duplicated a
	// Kind would make EnemyInfo silently return the SECOND row while
	// EnemyKinds() still lists both — a confusing data desync, not a crash.
	// Panic at init like the name-table collision guard (buildEnemyKindByName).
	seenKind := make(map[EnemyKind]struct{}, len(enemyDefinitions))
	for _, def := range enemyDefinitions {
		if _, dup := seenKind[def.Kind]; dup {
			panic(fmt.Sprintf("core/enemies: duplicate EnemyKind %d (%q) in enemyDefinitions", def.Kind, def.Name))
		}
		seenKind[def.Kind] = struct{}{}
	}
	for _, def := range enemyDefinitions {
		// Shared with the custom-enemy loader (customenemy.go): SkillCastChance
		// [0,1] + non-negative mitigation/reward/damage fields. Panic here
		// since a bad static-registry row is a programmer error, not data.
		if err := validateEnemyStatBounds(def.Name, def.SkillCastChance, def.Armor, def.MDef, def.AttackDamage, def.XPValue, def.SpellPower, def.Tier); err != nil {
			panic("core/enemies: " + err.Error())
		}
		if def.PoisonChance < 0 || def.PoisonChance > 1 {
			panic("core/enemies: " + def.Name + " PoisonChance must be in [0, 1]")
		}
		// Gold bounds must be non-negative and ordered — AwardBattleLoot
		// tolerates a slip at runtime, but a negative/inverted authored
		// value is almost always a typo, so reject it where it's written.
		if def.GoldMin < 0 || def.GoldMax < 0 {
			panic("core/enemies: " + def.Name + " GoldMin/GoldMax must be non-negative")
		}
		if def.GoldMax < def.GoldMin {
			panic("core/enemies: " + def.Name + " GoldMax must be >= GoldMin")
		}
		// Drop chances ride the same [0, 1] contract as the proc fields,
		// and a drop must name a real item kind.
		for _, d := range def.Drops {
			if d.Chance < 0 || d.Chance > 1 {
				panic("core/enemies: " + def.Name + " drop Chance must be in [0, 1]")
			}
			if _, ok := ItemInfoOk(d.Kind); !ok {
				panic("core/enemies: " + def.Name + " drops an unregistered item kind")
			}
		}
	}
}

// EnemyKinds returns the enemy registry in declaration order. Used by
// the editor to build its entity-brush palette and packAddRules table
// at init — adding a new enemy kind to enemyDefinitions automatically
// surfaces in the brush list and the pack-edit modal's add-shortcuts.
// Returns a defensive copy so callers can't mutate the registry.
func EnemyKinds() []EnemyDefinition {
	out := make([]EnemyDefinition, len(enemyDefinitions))
	copy(out, enemyDefinitions)
	return out
}

// EnemyKindCount returns the number of registered enemy kinds WITHOUT the
// defensive whole-slice copy EnemyKinds() makes — for per-frame callers (e.g.
// the bestiary tally header) that only need the count, not the definitions.
func EnemyKindCount() int {
	return len(enemyDefinitions)
}

// TheEnemy returns the article-prefixed singular form of an enemy ("The
// rat", "The goblin mage"). Combat log lines repeated "The " + def.
// SingularNoun a dozen times — centralising here means a future enemy
// that wants a different article ("An Amoeba" / a proper-named boss)
// is one method, not a grep across battle.go and actions.go.
func TheEnemy(def EnemyDefinition) string {
	return "The " + def.SingularNoun
}

// EnemyInfoOk is the validating sibling of EnemyInfo (mirrors ItemInfoOk):
// returns (definition, true) for a registered kind, (zero, false) otherwise —
// for callers that want to handle an unknown kind rather than take EnemyInfo's
// panic (e.g. a tool validating externally-sourced kinds before use).
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
	// Unreachable for valid data: every declared kind has an enemyDefinitions
	// row (enemyByKind is built from it), and the only externally-sourced
	// kinds — custom enemies' BaseKind — are validated by EnemyKindFromName
	// at load. Reaching here means an in-memory corrupt Kind or a declared
	// enum value with no definition; surface it loudly rather than shipping a
	// 1-HP "Enemy" placeholder silently into combat and the renderer.
	panic(fmt.Sprintf("core: EnemyInfo called with unregistered kind %d — add it to enemyDefinitions", int(kind)))
}

// EnemyInfoFor is the read accessor for per-instance enemy display data.
// Returns the kind's static definition with the instance's authored MaxHP
// applied if set — used wherever the renderer / log text needs descriptive
// strings about the live enemy.
func EnemyInfoFor(enemy Enemy) EnemyDefinition {
	def := *enemyGoverningDef(&enemy)
	if enemy.HasDefinitionOverride {
		def.Kind = enemy.Kind
	}
	if enemy.MaxHP > 0 {
		def.MaxHP = enemy.MaxHP
	}
	// Armor: 0 is a meaningful live value (armor-stripped, or a custom
	// enemy authored at 0), so unlike MaxHP we always copy through —
	// the live runtime field is the source of truth.
	def.Armor = enemy.Armor
	return def
}

// enemyGoverningDef returns a POINTER to the definition that governs this
// enemy's display/combat fields WITHOUT copying the ~200-byte EnemyDefinition:
// the embedded DefinitionOverride for a custom enemy, else the shared registry
// row. The narrow single-field accessors (EnemyName / EnemySingularName /
// enemyBaseStats) read through it on the per-frame battle roster + turn-queue
// paths, where EnemyInfoFor's full-struct value return is wasteful. Live MaxHP /
// Armor overrides are NOT applied here — callers needing those go through
// EnemyInfoFor. Panics on an unregistered kind, matching EnemyInfo's contract.
func enemyGoverningDef(e *Enemy) *EnemyDefinition {
	if e.HasDefinitionOverride {
		return &e.DefinitionOverride
	}
	if def, ok := enemyByKind[e.Kind]; ok {
		return def
	}
	panic(fmt.Sprintf("core: enemy carries unregistered kind %d — add it to enemyDefinitions", int(e.Kind)))
}

// EnemyName / EnemySingularName read a single descriptive field of an enemy's
// governing definition without materializing the whole struct — the cheap
// accessors for the per-frame battle roster and turn-queue labels (vs
// EnemyInfoFor(...).Name, which copies ~200 bytes per call). Take *Enemy so the
// caller doesn't copy the Enemy (which embeds the ~250-byte DefinitionOverride)
// either.
func EnemyName(e *Enemy) string         { return enemyGoverningDef(e).Name }
func EnemySingularName(e *Enemy) string { return enemyGoverningDef(e).SingularName }

// EffectiveEnemyStats returns the enemy's combat stats with every active debuff
// folded in — the enemy-side mirror of EffectiveStats. The summed Debuffs stat
// deltas are added per-stat (player debuffs stamp NEGATIVE deltas — Cripple SPD,
// Blind DEX, and so on — and DIFFERENT skills' debuffs SUM), each stat floored at
// 0 like the party reader so a debuff can't drive a stat negative into the combat
// math. The base stats stay clean on the definition; combat reads (dodge / crit /
// turn-rate / status-resist) route through here so every kind of stat delta lands.
// Cheap fast-path when no debuff is active (the common case).
func EffectiveEnemyStats(e Enemy) Stats {
	// Read base Stats through the governing-def pointer rather than
	// EnemyInfoFor's full-struct value copy — this runs per combat roll
	// (dodge / crit / resist / SPD) and only needs the 24-byte Stats block.
	out := enemyGoverningDef(&e).Stats
	if len(e.Debuffs) == 0 {
		return out
	}
	delta, _, _ := SumStatusMods(e.Debuffs)
	return addStatsFloored(out, delta)
}

// StampEnemyDebuff adds or refreshes a skill's stat debuff (its negative
// BuffStats) on the enemy for effect.BuffTurns of the enemy's turns — the single
// home for the enemy-side stamp that Cripple, Blind, Frostbite, Cone of Cold,
// Smoke Bomb, and the Ice Armor chill all share. Returns false (and no-ops) when
// the target is nil or the skill carries no debuff (BuffTurns == 0), so a damage
// skill with no chill component skips it cleanly and callers can use the bool to
// drive their "chilled / afflicted" log. STACKS with OTHER skills' debuffs (they
// sum in EffectiveEnemyStats); re-stamping the SAME skill refreshes its entry.
// Callers that deal damage first should gate on `!defeated` so a killed target
// keeps no dangling debuff (the kill check is a battle concept this can't see).
func StampEnemyDebuff(e *Enemy, source SkillID, effect SkillEffect) bool {
	if e == nil || effect.BuffTurns <= 0 {
		return false
	}
	e.Debuffs = applyStatusMod(e.Debuffs, StatusMod{Source: source, Stats: effect.BuffStats, Armor: effect.BuffArmor, MDef: effect.BuffMDef, Turns: effect.BuffTurns})
	return true
}

// EnemyLevel is the foe's level, defaulting an unauthored (0) definition to
// DefaultEnemyLevel so every kind has a sane level without per-row authoring.
// Read by the flee-chance math (party avg level vs pack avg level); no other
// system reads enemy level yet.
func EnemyLevel(e Enemy) int {
	if l := EnemyInfoFor(e).Level; l > 0 {
		return l
	}
	return DefaultEnemyLevel
}

// EnemyBasicDamage is the enemy's basic-attack damage, read through the
// definition overlay. The single seam for "how hard does this enemy's basic
// swing hit" — symmetric with the party side's MemberAttackDamage — so the
// anticipated "derive from Stats.STR" pass edits this one accessor instead of
// an inline EnemyInfoFor(...).AttackDamage field read at the call site.
// ScaleEnemyDifficulty applies the global EnemyDifficulty multiplier
// (EnemyDifficultyNum/Den) to a base stat — round-to-nearest, integer-exact, and
// floored at 1 for any positive input so a scaled damage/HP value never collapses
// to 0. The single seam behind "make foes harder": spawn HP, basic-attack damage,
// and enemy spell damage all route through it (the last from the battle package).
func ScaleEnemyDifficulty(n int) int {
	scaled := (n*EnemyDifficultyNum + EnemyDifficultyDen/2) / EnemyDifficultyDen
	if n > 0 && scaled < 1 {
		scaled = 1
	}
	return scaled
}

func EnemyBasicDamage(e Enemy) int {
	return ScaleEnemyDifficulty(EnemyInfoFor(e).AttackDamage)
}

func NewEnemy(kind EnemyKind) Enemy {
	def := EnemyInfo(kind)
	maxHP := ScaleEnemyDifficulty(def.MaxHP)
	return Enemy{
		Kind:  kind,
		HP:    maxHP,
		MaxHP: maxHP,
		Alive: true,
		Item:  def.Item,
		Armor: def.Armor,
		// SkillCastCount stays nil here — Go's nil-map read returns
		// zero, so usableEnemySkills's `enemy.SkillCastCount[s]`
		// lookup is safe before any cast. The first cast that needs
		// to record a count lazily allocates via the nil-guard in
		// handleEnemyRaiseBones. Matches the zero-value convention
		// every other status counter on this struct uses (BurnTurns,
		// SleepTurns, etc.) — eager allocation would single out one
		// field without buying behaviour.
	}
}

func EnemyDisplayName(enemy Enemy) string {
	return enemyGoverningDef(&enemy).Name
}

func EnemySingularNoun(enemy Enemy) string {
	return enemyGoverningDef(&enemy).SingularNoun
}

// These battle-string builders take *GameState like the rest of the
// selectors in this package — taking GameState by value copied the whole
// struct (slices, maps, party) on every per-frame HUD-string call.
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

func BattleEncounterMessage(g *GameState) string {
	count := LivingBattleCount(g)
	def := BattleEnemyInfo(g)
	if count <= 1 {
		return fmt.Sprintf("A %s blocks the way.", def.SingularNoun)
	}
	return fmt.Sprintf("%d %s close in.", count, def.PluralNoun)
}

func BattleEncounterTitle(g *GameState) string {
	count := len(BattleMembers(g))
	def := BattleEnemyInfo(g)
	if count <= 1 {
		return fmt.Sprintf("%s Encounter!", def.Name)
	}
	return fmt.Sprintf("%s x%d!", def.GroupName, count)
}

func LastBattleEnemyFallsMessage(g *GameState) string {
	return fmt.Sprintf("The last %s falls.", BattleEnemyInfo(g).SingularNoun)
}

func BattleLossMessage(g *GameState) string {
	count := LivingBattleCount(g)
	def := BattleEnemyInfo(g)
	if count <= 1 {
		return fmt.Sprintf("The %s drives the party back. Press to recover.", def.SingularNoun)
	}
	return fmt.Sprintf("The %s drive the party back. Press to recover.", def.PluralNoun)
}

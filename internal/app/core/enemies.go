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
)

type EnemyDefinition struct {
	Kind         EnemyKind
	Name         string
	SingularName string
	PluralName   string
	SingularNoun string
	PluralNoun   string
	GroupName    string
	Item         string
	MaxHP        int
	AttackDamage int
	// Speed is the implicit initiative value used to slot enemies into the
	// mixed-initiative turn queue. Higher = acts earlier. Players have full
	// stat blocks; enemies only need this one number for now.
	Speed int
	// Tier orders enemy kinds by threat. The highest-Tier member of a pack
	// is the figure shown on the field (everyone else is hidden until the
	// battle reveals them). Ties break by member order within the pack.
	Tier               int
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
		Item:               "Morsel of Cheese",
		MaxHP:              10,
		AttackDamage:       3,
		Speed:              6,
		Tier:               1,
		XPValue:            5,
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
		// Bat jerky heals more than rat cheese — see ItemHealAmount; the bat
		// is also faster and harder to land hits on, so the loot is the
		// payoff for fighting (or robbing) the trickier enemy.
		Item:               "Bat Jerky",
		MaxHP:              7,
		AttackDamage:       2,
		Speed:              9,
		Tier:               2,
		XPValue:            8,
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
		Item:               "",
		MaxHP:              12,
		AttackDamage:       3,
		Speed:              5,
		Tier:               3,
		XPValue:            12,
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
		Item:               "Morsel of Cheese",
		MaxHP:              14,
		AttackDamage:       4,
		Speed:              5,
		Tier:               3,
		XPValue:            14,
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
		Item:         "Bat Jerky",
		// Lower HP than a regular goblin — squishy spellcaster. The
		// danger is in the spells, not the wand-whack.
		MaxHP:              9,
		AttackDamage:       2,
		Speed:              4,
		Tier:               4,
		XPValue:            20,
		AttackVerbSingular: "swings a wand at",
		AttackVerbPlural:   "swing wands at",
		// Per-turn AI rolls over these: Firebolt is the damage option,
		// Sleep is the disabler. Order in the slice is irrelevant —
		// the battle package's enemyAIPickSkill picks uniformly.
		// Plain melee remains the SkillCastChance miss-roll fallback.
		Skills:          []SkillID{SkillFirebolt, SkillSleep},
		SkillCastChance: 0.5,
		SpellPower:      6,
	},
	{
		Kind:         EnemyAmoeba,
		Name:         "Stone Amoeba",
		SingularName: "Amoeba",
		PluralName:   "Amoebae",
		SingularNoun: "amoeba",
		PluralNoun:   "amoebae",
		GroupName:    "Amoeba Cluster",
		Item:         "",
		// Low HP × very high armor: phys swings whiff to 1, magic
		// shreds. Sets up the "switch your party to magic when armor
		// shows up" lesson.
		MaxHP:              8,
		AttackDamage:       2,
		Speed:              2,
		Tier:               3,
		XPValue:            16,
		Armor:              8,
		AttackVerbSingular: "engulfs",
		AttackVerbPlural:   "engulf",
	},
}

// enemyByKind is the O(1) lookup map for enemyDefinitions, built once at
// init. EnemyInfo is called per-frame from the renderer (roster, popups),
// so the map matches the partyClassByID / skillByID pattern in party.go.
var enemyByKind = buildEnemyByKind()

func buildEnemyByKind() map[EnemyKind]EnemyDefinition {
	m := make(map[EnemyKind]EnemyDefinition, len(enemyDefinitions))
	for _, def := range enemyDefinitions {
		// Probability fields ride a [0, 1] contract — values past 1
		// roll "always" which is usually a typo (a designer meant
		// 0.5 and wrote 5). Negative values would invert the gate
		// silently. Panic at init so the bad data never reaches the
		// gameplay loop.
		if def.SkillCastChance < 0 || def.SkillCastChance > 1 {
			panic("core/enemies: " + def.Name + " SkillCastChance must be in [0, 1]")
		}
		if def.PoisonChance < 0 || def.PoisonChance > 1 {
			panic("core/enemies: " + def.Name + " PoisonChance must be in [0, 1]")
		}
		m[def.Kind] = def
	}
	return m
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

func EnemyInfo(kind EnemyKind) EnemyDefinition {
	if def, ok := enemyByKind[kind]; ok {
		return def
	}
	return EnemyDefinition{
		Kind:               kind,
		Name:               "Enemy",
		SingularName:       "Enemy",
		PluralName:         "Enemies",
		SingularNoun:       "enemy",
		PluralNoun:         "enemies",
		GroupName:          "Enemy Group",
		MaxHP:              1,
		AttackDamage:       1,
		Speed:              5,
		AttackVerbSingular: "strikes",
		AttackVerbPlural:   "strike",
	}
}

// EnemyInfoFor is the read accessor for per-instance enemy display data.
// Returns the kind's static definition with the instance's authored MaxHP
// applied if set — used wherever the renderer / log text needs descriptive
// strings about the live enemy.
func EnemyInfoFor(enemy Enemy) EnemyDefinition {
	def := EnemyInfo(enemy.Kind)
	if enemy.MaxHP > 0 {
		def.MaxHP = enemy.MaxHP
	}
	return def
}

func NewEnemy(kind EnemyKind) Enemy {
	def := EnemyInfo(kind)
	return Enemy{
		Kind:  kind,
		HP:    def.MaxHP,
		MaxHP: def.MaxHP,
		Alive: true,
		Item:  def.Item,
		Armor: def.Armor,
	}
}

func EnemyDisplayName(enemy Enemy) string {
	return EnemyInfoFor(enemy).Name
}

func EnemySingularNoun(enemy Enemy) string {
	return EnemyInfoFor(enemy).SingularNoun
}

func BattleEnemyInfo(g GameState) EnemyDefinition {
	members := BattleMembers(&g)
	if g.Battle.EnemyIndex >= 0 && g.Battle.EnemyIndex < len(members) {
		return EnemyInfoFor(members[g.Battle.EnemyIndex])
	}
	if len(members) > 0 {
		return EnemyInfoFor(members[0])
	}
	return EnemyInfo(EnemyRat)
}

func BattleEnemyGroupName(g GameState) string {
	return BattleEnemyInfo(g).GroupName
}

func BattleEnemyTargetStatus(g GameState, ordinal, total int) string {
	def := BattleEnemyInfo(g)
	return fmt.Sprintf("Targeting %s %d of %d.", def.SingularNoun, ordinal, total)
}

func BattleEncounterMessage(g GameState) string {
	count := LivingBattleCount(&g)
	def := BattleEnemyInfo(g)
	if count <= 1 {
		return fmt.Sprintf("A %s blocks the way.", def.SingularNoun)
	}
	return fmt.Sprintf("%d %s close in.", count, def.PluralNoun)
}

func BattleEncounterTitle(g GameState) string {
	count := len(BattleMembers(&g))
	def := BattleEnemyInfo(g)
	if count <= 1 {
		return fmt.Sprintf("%s Encounter!", def.Name)
	}
	return fmt.Sprintf("%s x%d!", def.GroupName, count)
}

func LastBattleEnemyFallsMessage(g GameState) string {
	return fmt.Sprintf("The last %s falls.", BattleEnemyInfo(g).SingularNoun)
}

func BattleLossMessage(g GameState) string {
	count := LivingBattleCount(&g)
	def := BattleEnemyInfo(g)
	if count <= 1 {
		return fmt.Sprintf("The %s drives the party back. Press Enter to recover.", def.SingularNoun)
	}
	return fmt.Sprintf("The %s drive the party back. Press Enter to recover.", def.PluralNoun)
}

package core

import "fmt"

type EnemyKind int

const (
	EnemyRat EnemyKind = iota
	EnemyBat
	EnemyDiseasedRat
)

type EnemyDefinition struct {
	Kind               EnemyKind
	Name               string
	SingularName       string
	PluralName         string
	SingularNoun       string
	PluralNoun         string
	GroupName          string
	Item               string
	MaxHP              int
	AttackDamage       int
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
		AttackVerbSingular: "snaps",
		AttackVerbPlural:   "snap",
	},
	{
		Kind:               EnemyBat,
		Name:               "Cave Bat",
		SingularName:       "Bat",
		PluralName:         "Bats",
		SingularNoun:       "bat",
		PluralNoun:         "bats",
		GroupName:          "Bat Swarm",
		// Bat jerky heals more than rat cheese — see ItemHealAmount; the bat
		// is also faster and harder to land hits on, so the loot is the
		// payoff for fighting (or robbing) the trickier enemy.
		Item:               "Bat Jerky",
		MaxHP:              7,
		AttackDamage:       2,
		Speed:              9,
		Tier:               2,
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
		AttackVerbSingular: "bites",
		AttackVerbPlural:   "bite",
		// DiseasedRatPoisonChance per bite. Pairs with the rat's higher
		// HP and damage to make a diseased pack the threat upgrade over a
		// plain rat pack. Tuning value lives in config.go.
		PoisonChance: DiseasedRatPoisonChance,
	},
}

// enemyByKind is the O(1) lookup map for enemyDefinitions, built once at
// init. EnemyInfo is called per-frame from the renderer (roster, popups),
// so the map matches the partyClassByID / skillByID pattern in party.go.
var enemyByKind = buildEnemyByKind()

func buildEnemyByKind() map[EnemyKind]EnemyDefinition {
	m := make(map[EnemyKind]EnemyDefinition, len(enemyDefinitions))
	for _, def := range enemyDefinitions {
		m[def.Kind] = def
	}
	return m
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


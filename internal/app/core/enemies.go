package core

import "fmt"

type EnemyKind int

const (
	EnemyRat EnemyKind = iota
	EnemyBat
)

type EnemyDefinition struct {
	Kind               EnemyKind
	Name               string
	MonsterType        string
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
	Speed              int
	AttackVerbSingular string
	AttackVerbPlural   string
}

var enemyDefinitions = []EnemyDefinition{
	{
		Kind:               EnemyRat,
		Name:               "Feral Rat",
		MonsterType:        "Beast",
		SingularName:       "Rat",
		PluralName:         "Rats",
		SingularNoun:       "rat",
		PluralNoun:         "rats",
		GroupName:          "Rat Pack",
		Item:               "Morsel of Cheese",
		MaxHP:              10,
		AttackDamage:       3,
		Speed:              6,
		AttackVerbSingular: "snaps",
		AttackVerbPlural:   "snap",
	},
	{
		Kind:               EnemyBat,
		Name:               "Cave Bat",
		MonsterType:        "Beast",
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
		AttackVerbSingular: "bites",
		AttackVerbPlural:   "bite",
	},
}

func EnemyInfo(kind EnemyKind) EnemyDefinition {
	for _, def := range enemyDefinitions {
		if def.Kind == kind {
			return def
		}
	}
	return EnemyDefinition{
		Kind:               kind,
		Name:               "Enemy",
		MonsterType:        "Beast",
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

func EnemyInfoFor(enemy Enemy) EnemyDefinition {
	def := EnemyInfo(enemy.Kind)
	if enemy.Name != "" {
		def.Name = enemy.Name
	}
	if enemy.MonsterType != "" {
		def.MonsterType = enemy.MonsterType
	}
	if enemy.MaxHP > 0 {
		def.MaxHP = enemy.MaxHP
	}
	return def
}

func NewEnemy(kind EnemyKind, tileX, tileZ int) Enemy {
	def := EnemyInfo(kind)
	return Enemy{
		Kind:        kind,
		TileX:       tileX,
		TileZ:       tileZ,
		HP:          def.MaxHP,
		MaxHP:       def.MaxHP,
		Alive:       true,
		Name:        def.Name,
		MonsterType: def.MonsterType,
		Item:        def.Item,
	}
}

func EnemyDisplayName(enemy Enemy) string {
	return EnemyInfoFor(enemy).Name
}

func EnemyMonsterType(enemy Enemy) string {
	return EnemyInfoFor(enemy).MonsterType
}

func EnemySingularNoun(enemy Enemy) string {
	return EnemyInfoFor(enemy).SingularNoun
}

func BattleEnemyInfo(g GameState) EnemyDefinition {
	if g.Battle.EnemyIndex >= 0 && g.Battle.EnemyIndex < len(g.Enemies) {
		return EnemyInfoFor(g.Enemies[g.Battle.EnemyIndex])
	}
	for _, index := range g.Battle.EnemyGroup {
		if index >= 0 && index < len(g.Enemies) {
			return EnemyInfoFor(g.Enemies[index])
		}
	}
	return EnemyInfo(EnemyRat)
}

func BattleEnemyGroupName(g GameState) string {
	return BattleEnemyInfo(g).GroupName
}

func BattleEnemyTurnLabel(g GameState) string {
	count := LivingBattleCount(&g)
	def := BattleEnemyInfo(g)
	if count <= 1 {
		return def.SingularName
	}
	return fmt.Sprintf("%s x%d", def.PluralName, count)
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
	count := len(g.Battle.EnemyGroup)
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

func BattleEnemyAttackMessage(g GameState, hits, burns int) string {
	def := BattleEnemyInfo(g)
	switch {
	case burns > 0 && hits > 1:
		return fmt.Sprintf("Flames bite. %d %s %s at the party.", hits, def.PluralNoun, def.AttackVerbPlural)
	case burns > 0 && hits == 1:
		return fmt.Sprintf("Flames bite. A %s %s at the party.", def.SingularNoun, def.AttackVerbSingular)
	case hits == 1:
		return fmt.Sprintf("A %s %s at the party.", def.SingularNoun, def.AttackVerbSingular)
	default:
		return fmt.Sprintf("%d %s %s at the party.", hits, def.PluralNoun, def.AttackVerbPlural)
	}
}

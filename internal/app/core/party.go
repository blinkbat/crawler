package core

type PartyClass int

const (
	ClassWarrior PartyClass = iota
	ClassCleric
	ClassThief
	ClassWizard
)

type PartyClassDefinition struct {
	Class PartyClass
	Name  string
	MaxHP int
	MaxMP int
	Atk   int
	Skill int
}

type skillDefinition struct {
	Skill      int
	Name       string
	Cost       int
	TargetMode int
	Effect     SkillEffect
}

type SkillEffect struct {
	Damage       int
	Heal         int
	StealChance  float64
	BurnChance   float64
	BurnMinTurns int
	BurnMaxTurns int
}

var partyClassDefinitions = []PartyClassDefinition{
	{Class: ClassWarrior, Name: "Warrior", MaxHP: 28, MaxMP: 4, Atk: 4, Skill: SkillSwipe},
	{Class: ClassCleric, Name: "Cleric", MaxHP: 24, MaxMP: 18, Atk: 5, Skill: SkillPrayer},
	{Class: ClassThief, Name: "Thief", MaxHP: 22, MaxMP: 8, Atk: 3, Skill: SkillSteal},
	{Class: ClassWizard, Name: "Wizard", MaxHP: 26, MaxMP: 24, Atk: 4, Skill: SkillFirebolt},
}

var skillDefinitions = []skillDefinition{
	{Skill: SkillSwipe, Name: "Swipe", Cost: 3, TargetMode: ActionMenu, Effect: SkillEffect{Damage: 3}},
	{Skill: SkillPrayer, Name: "Prayer", Cost: 5, TargetMode: ActionPartyTarget, Effect: SkillEffect{Heal: 10}},
	{Skill: SkillSteal, Name: "Steal", Cost: 0, TargetMode: ActionEnemyTarget, Effect: SkillEffect{StealChance: 0.7}},
	{Skill: SkillFirebolt, Name: "Firebolt", Cost: 6, TargetMode: ActionEnemyTarget, Effect: SkillEffect{Damage: 6, BurnChance: 0.82, BurnMinTurns: 3, BurnMaxTurns: 5}},
}

func PartyClasses() []PartyClassDefinition {
	defs := make([]PartyClassDefinition, len(partyClassDefinitions))
	copy(defs, partyClassDefinitions)
	return defs
}

func partyClassInfo(class PartyClass) (PartyClassDefinition, bool) {
	for _, def := range partyClassDefinitions {
		if def.Class == class {
			return def, true
		}
	}
	return PartyClassDefinition{}, false
}

func PartySkill(member PartyMember) int {
	if def, ok := partyClassInfo(member.Class); ok {
		return def.Skill
	}
	return SkillNone
}

func skillInfo(skill int) (skillDefinition, bool) {
	for _, def := range skillDefinitions {
		if def.Skill == skill {
			return def, true
		}
	}
	return skillDefinition{}, false
}

func SkillName(skill int) string {
	if def, ok := skillInfo(skill); ok {
		return def.Name
	}
	return "Skill"
}

func SkillCost(skill int) int {
	if def, ok := skillInfo(skill); ok {
		return def.Cost
	}
	return 0
}

func SkillTargetMode(skill int) int {
	if def, ok := skillInfo(skill); ok {
		return def.TargetMode
	}
	return ActionMenu
}

func SkillEffectFor(skill int) SkillEffect {
	if def, ok := skillInfo(skill); ok {
		return def.Effect
	}
	return SkillEffect{}
}

func (effect SkillEffect) BurnDuration() int {
	if effect.BurnMaxTurns <= effect.BurnMinTurns {
		return effect.BurnMinTurns
	}
	return effect.BurnMinTurns + GameRNG.Intn(effect.BurnMaxTurns-effect.BurnMinTurns+1)
}

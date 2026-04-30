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
	Skill int
	Name  string
	Cost  int
}

var partyClassDefinitions = []PartyClassDefinition{
	{Class: ClassWarrior, Name: "Warrior", MaxHP: 28, MaxMP: 4, Atk: 4, Skill: SkillSwipe},
	{Class: ClassCleric, Name: "Cleric", MaxHP: 24, MaxMP: 18, Atk: 5, Skill: SkillPrayer},
	{Class: ClassThief, Name: "Thief", MaxHP: 22, MaxMP: 8, Atk: 3, Skill: SkillSteal},
	{Class: ClassWizard, Name: "Wizard", MaxHP: 26, MaxMP: 24, Atk: 4, Skill: SkillFirebolt},
}

var skillDefinitions = []skillDefinition{
	{Skill: SkillSwipe, Name: "Swipe", Cost: 3},
	{Skill: SkillPrayer, Name: "Prayer", Cost: 5},
	{Skill: SkillSteal, Name: "Steal", Cost: 0},
	{Skill: SkillFirebolt, Name: "Firebolt", Cost: 6},
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

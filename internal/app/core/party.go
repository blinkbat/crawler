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
	Stats Stats
	MaxMP int
	Skill int
}

// SkillKind tags how a skill scales off the actor's stats.
//   Melee:   damage = STR + base
//   Magic:   damage = INT + base
//   Heal:    heal   = WIS + base
//   Utility: no stat-scaled damage (Steal, Defend, etc.)
const (
	SkillKindNone    = 0
	SkillKindMelee   = 1
	SkillKindMagic   = 2
	SkillKindHeal    = 3
	SkillKindUtility = 4
)

type skillDefinition struct {
	Skill      int
	Name       string
	Cost       int
	TargetMode int
	Kind       int
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

// Party stats post-difficulty pass. Numbers are deliberately tighter than
// the old "8 in your specialty" baseline: top stat is 6, supporting stats
// hover at 1-3, VIT trims HP to a level where careless play actually loses
// fights. MP pools shrunk so casters can't spam-firebolt encounters away.
var partyClassDefinitions = []PartyClassDefinition{
	{Class: ClassWarrior, Name: "Warrior", Stats: Stats{STR: 6, DEX: 2, INT: 1, WIS: 1, VIT: 5, SPD: 3}, MaxMP: 2, Skill: SkillSwipe},
	{Class: ClassCleric, Name: "Cleric", Stats: Stats{STR: 2, DEX: 2, INT: 2, WIS: 6, VIT: 4, SPD: 4}, MaxMP: 7, Skill: SkillPrayer},
	{Class: ClassThief, Name: "Thief", Stats: Stats{STR: 3, DEX: 6, INT: 2, WIS: 1, VIT: 4, SPD: 6}, MaxMP: 3, Skill: SkillSteal},
	{Class: ClassWizard, Name: "Wizard", Stats: Stats{STR: 1, DEX: 2, INT: 6, WIS: 2, VIT: 4, SPD: 4}, MaxMP: 8, Skill: SkillFirebolt},
}

// Skill registry. Effect.Damage / Effect.Heal are flat baselines that the
// stat-scaled formulas add on top (see types.go's MeleeDamage etc.). Tuned
// so that a focused class with their stat at 8 lands roughly the same total
// damage as the pre-stats values: e.g. Wizard (INT 8) Firebolt = 8 + 2 = 10
// pre-quality, scaling further with timing.
// Skill registry. Difficulty pass: bases trimmed and Firebolt's burn-chance
// pulled down so a single Excellent doesn't auto-burn every cast.
var skillDefinitions = []skillDefinition{
	{Skill: SkillSwipe, Name: "Swipe", Cost: 2, TargetMode: ActionMenu, Kind: SkillKindMelee, Effect: SkillEffect{Damage: 0}},
	{Skill: SkillPrayer, Name: "Prayer", Cost: 4, TargetMode: ActionPartyTarget, Kind: SkillKindHeal, Effect: SkillEffect{Heal: 1}},
	{Skill: SkillSteal, Name: "Steal", Cost: 0, TargetMode: ActionEnemyTarget, Kind: SkillKindUtility, Effect: SkillEffect{StealChance: 0.40}},
	{Skill: SkillFirebolt, Name: "Firebolt", Cost: 5, TargetMode: ActionEnemyTarget, Kind: SkillKindMagic, Effect: SkillEffect{Damage: 1, BurnChance: 0.45, BurnMinTurns: 2, BurnMaxTurns: 3}},
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

// SkillKindOf returns the skill's Kind tag (Melee / Magic / Heal / Utility),
// used by apply* functions to pick the right stat for damage/heal scaling.
func SkillKindOf(skill int) int {
	if def, ok := skillInfo(skill); ok {
		return def.Kind
	}
	return SkillKindNone
}

// SkillDamage computes a skill's pre-quality damage from the actor's stats,
// dispatching on the skill's Kind. Melee adds STR, Magic adds INT, anything
// else returns just the skill base. Quality scaling (ScaleDamage) applies on
// top at the call site.
func SkillDamage(stats Stats, skill int) int {
	def, ok := skillInfo(skill)
	if !ok {
		return 0
	}
	switch def.Kind {
	case SkillKindMelee:
		return MeleeDamage(stats, def.Effect.Damage)
	case SkillKindMagic:
		return MagicDamage(stats, def.Effect.Damage)
	default:
		return def.Effect.Damage
	}
}

// SkillHeal computes a skill's pre-quality heal from the actor's stats,
// dispatching on Kind. Heal kind adds WIS; anything else returns just the
// skill base. Quality scaling (ScaleHeal) applies on top at the call site.
func SkillHeal(stats Stats, skill int) int {
	def, ok := skillInfo(skill)
	if !ok {
		return 0
	}
	switch def.Kind {
	case SkillKindHeal:
		return HealAmount(stats, def.Effect.Heal)
	default:
		return def.Effect.Heal
	}
}

func (effect SkillEffect) BurnDuration() int {
	if effect.BurnMaxTurns <= effect.BurnMinTurns {
		return effect.BurnMinTurns
	}
	return effect.BurnMinTurns + GameRNG.Intn(effect.BurnMaxTurns-effect.BurnMinTurns+1)
}

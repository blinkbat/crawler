package core

import "math/rand"

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
	Skill SkillID
}

// SkillKind tags how a skill scales off the actor's stats.
//   Melee:   damage = STR + base
//   Magic:   damage = INT + base
//   Heal:    heal   = WIS + base
//   Utility: no stat-scaled damage (Steal, Defend, etc.)
type SkillKind int

const (
	SkillKindMelee SkillKind = iota + 1
	SkillKindMagic
	SkillKindHeal
	SkillKindUtility
)

// SkillMinigame picks which timing minigame arms when the skill confirms.
// Press is the default (a single-press window); Charge is hold-and-release
// with three ticks; Sequence is the directional pickpocket rhythm.
type SkillMinigame int

const (
	MinigamePress SkillMinigame = iota
	MinigameCharge
	MinigameSequence
)

type skillDefinition struct {
	Skill      SkillID
	Name       string
	Cost       int
	TargetMode ActionMode
	Kind       SkillKind
	Minigame   SkillMinigame
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
//
// SEAT ORDER CONTRACT: the slice order is also the in-battle seating order
// and the SPD-tie-breaker order in buildTurnQueue. Editor save format and
// `render` formation positioning both index by class slot. Reordering this
// slice silently reshuffles party formation and tie-broken initiative; if
// you need to add a class, append rather than insert.
var partyClassDefinitions = []PartyClassDefinition{
	{Class: ClassWarrior, Name: "Warrior", Stats: Stats{STR: 6, DEX: 2, INT: 1, WIS: 1, VIT: 5, SPD: 3}, MaxMP: 2, Skill: SkillSwipe},
	{Class: ClassCleric, Name: "Cleric", Stats: Stats{STR: 2, DEX: 2, INT: 2, WIS: 6, VIT: 4, SPD: 4}, MaxMP: 7, Skill: SkillPrayer},
	{Class: ClassThief, Name: "Thief", Stats: Stats{STR: 3, DEX: 6, INT: 2, WIS: 1, VIT: 4, SPD: 6}, MaxMP: 3, Skill: SkillSteal},
	{Class: ClassWizard, Name: "Wizard", Stats: Stats{STR: 1, DEX: 2, INT: 6, WIS: 2, VIT: 4, SPD: 4}, MaxMP: 8, Skill: SkillFirebolt},
}

// partyClassByID is the O(1) lookup for partyClassDefinitions. Built once
// at init for the same reason as skillByID — partyClassInfo is called per
// frame from selectors and per-party-member render code.
var partyClassByID = buildPartyClassByID()

func buildPartyClassByID() map[PartyClass]PartyClassDefinition {
	m := make(map[PartyClass]PartyClassDefinition, len(partyClassDefinitions))
	for _, def := range partyClassDefinitions {
		m[def.Class] = def
	}
	return m
}

// Skill registry. Effect.Damage / Effect.Heal are flat baselines that the
// stat-scaled formulas add on top (see types.go's MeleeDamage etc.). Tuned
// so that a focused class with their stat at 8 lands roughly the same total
// damage as the pre-stats values: e.g. Wizard (INT 8) Firebolt = 8 + 2 = 10
// pre-quality, scaling further with timing. Difficulty pass: bases trimmed
// and Firebolt's burn-chance pulled down so a single Excellent doesn't
// auto-burn every cast.
var skillDefinitions = []skillDefinition{
	{Skill: SkillSwipe, Name: "Swipe", Cost: 2, TargetMode: ActionMenu, Kind: SkillKindMelee, Minigame: MinigamePress, Effect: SkillEffect{Damage: 0}},
	{Skill: SkillPrayer, Name: "Prayer", Cost: 4, TargetMode: ActionPartyTarget, Kind: SkillKindHeal, Minigame: MinigameCharge, Effect: SkillEffect{Heal: 1}},
	{Skill: SkillSteal, Name: "Steal", Cost: 0, TargetMode: ActionEnemyTarget, Kind: SkillKindUtility, Minigame: MinigameSequence, Effect: SkillEffect{StealChance: StealBaseChance}},
	{Skill: SkillFirebolt, Name: "Firebolt", Cost: 5, TargetMode: ActionEnemyTarget, Kind: SkillKindMagic, Minigame: MinigameCharge, Effect: SkillEffect{Damage: 1, BurnChance: FireboltBurnChance, BurnMinTurns: 2, BurnMaxTurns: 3}},
}

// skillByID is the O(1) lookup table for skillDefinitions. Built once at
// init so per-frame skillInfo calls don't linear-walk the registry. The
// registry slice is still the source of truth (controls iteration order
// when the editor lists skills, etc.); the map is just a read cache.
var skillByID = buildSkillByID()

func buildSkillByID() map[SkillID]skillDefinition {
	m := make(map[SkillID]skillDefinition, len(skillDefinitions))
	for _, def := range skillDefinitions {
		m[def.Skill] = def
	}
	return m
}

func PartyClasses() []PartyClassDefinition {
	defs := make([]PartyClassDefinition, len(partyClassDefinitions))
	copy(defs, partyClassDefinitions)
	return defs
}

func partyClassInfo(class PartyClass) (PartyClassDefinition, bool) {
	def, ok := partyClassByID[class]
	return def, ok
}

func PartySkill(member PartyMember) SkillID {
	if def, ok := partyClassInfo(member.Class); ok {
		return def.Skill
	}
	return SkillNone
}

func skillInfo(skill SkillID) (skillDefinition, bool) {
	def, ok := skillByID[skill]
	return def, ok
}

// SkillMinigameFor returns the minigame kind used by the skill (Press by
// default for unknown IDs). Exposed for the battle layer to dispatch off
// the registry instead of hand-maintained predicates.
func SkillMinigameFor(skill SkillID) SkillMinigame {
	if def, ok := skillInfo(skill); ok {
		return def.Minigame
	}
	return MinigamePress
}

func SkillName(skill SkillID) string {
	if def, ok := skillInfo(skill); ok {
		return def.Name
	}
	return "Skill"
}

func SkillCost(skill SkillID) int {
	if def, ok := skillInfo(skill); ok {
		return def.Cost
	}
	return 0
}

func SkillTargetMode(skill SkillID) ActionMode {
	if def, ok := skillInfo(skill); ok {
		return def.TargetMode
	}
	return ActionMenu
}

func SkillEffectFor(skill SkillID) SkillEffect {
	if def, ok := skillInfo(skill); ok {
		return def.Effect
	}
	return SkillEffect{}
}

// SkillDamage computes a skill's pre-quality damage from the actor's stats,
// dispatching on the skill's Kind. Melee adds STR, Magic adds INT, anything
// else returns just the skill base. Quality scaling (ScaleDamage) applies on
// top at the call site.
func SkillDamage(stats Stats, skill SkillID) int {
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
func SkillHeal(stats Stats, skill SkillID) int {
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

func (effect SkillEffect) BurnDuration(rng *rand.Rand) int {
	if effect.BurnMaxTurns <= effect.BurnMinTurns {
		return effect.BurnMinTurns
	}
	return effect.BurnMinTurns + rng.Intn(effect.BurnMaxTurns-effect.BurnMinTurns+1)
}

// PoisonEffect describes the parameters for inflicting / ticking Poison.
// Mirrors SkillEffect.Burn* — single shape for both DoT statuses so battle
// code calls the same RollDuration method regardless of source. The
// package-level DefaultPoisonEffect bakes in the config constants; a future
// poison source (trap, alchemist item) can build its own with different
// numbers without recompiling battle.
type PoisonEffect struct {
	MinTurns   int
	MaxTurns   int
	TickDamage int
}

// DefaultPoisonEffect is the canonical Diseased Rat poison: bounded by
// PoisonMin/MaxTurns and ticking PoisonTickDamage per turn. Lives here
// rather than on EnemyDefinition so a future poison-not-from-an-enemy
// source can reuse the same shape.
var DefaultPoisonEffect = PoisonEffect{
	MinTurns:   PoisonMinTurns,
	MaxTurns:   PoisonMaxTurns,
	TickDamage: PoisonTickDamage,
}

// RollDuration picks a random duration in [MinTurns, MaxTurns] inclusive.
// Returns MinTurns when the range is degenerate, matching BurnDuration's
// behavior so both DoT rollers fail open instead of panicking on bad data.
func (p PoisonEffect) RollDuration(rng *rand.Rand) int {
	span := p.MaxTurns - p.MinTurns
	if span <= 0 {
		return p.MinTurns
	}
	return p.MinTurns + rng.Intn(span+1)
}

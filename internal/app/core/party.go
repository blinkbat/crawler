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
//
//	Melee:   damage = STR + base
//	Magic:   damage = INT + base
//	Heal:    heal   = WIS + base
//	Utility: no stat-scaled damage (Steal, Defend, etc.)
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
	// Tag classifies the skill for armor / future resist math and for
	// HUD color-coding. See SkillTag. Phys hits target.Armor; Magic /
	// Heal / Buff bypass.
	Tag      SkillTag
	Minigame SkillMinigame
	Effect   SkillEffect
}

type SkillEffect struct {
	Damage       int
	Heal         int
	StealChance  float64
	BurnChance   float64
	BurnMinTurns int
	BurnMaxTurns int
	// SleepMinTurns/MaxTurns gate the SleepEffect on this skill. Zero
	// means "this skill doesn't cause sleep" — the apply path can
	// short-circuit without touching the RNG.
	SleepMinTurns int
	SleepMaxTurns int
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
	{Skill: SkillSwipe, Name: "Swipe", Cost: 2, TargetMode: ActionMenu, Kind: SkillKindMelee, Tag: SkillTagPhys, Minigame: MinigamePress, Effect: SkillEffect{Damage: 0}},
	{Skill: SkillPrayer, Name: "Prayer", Cost: 4, TargetMode: ActionPartyTarget, Kind: SkillKindHeal, Tag: SkillTagHeal, Minigame: MinigameCharge, Effect: SkillEffect{Heal: 1}},
	{Skill: SkillSteal, Name: "Steal", Cost: 0, TargetMode: ActionEnemyTarget, Kind: SkillKindUtility, Tag: SkillTagNone, Minigame: MinigameSequence, Effect: SkillEffect{StealChance: StealBaseChance}},
	{Skill: SkillFirebolt, Name: "Firebolt", Cost: 5, TargetMode: ActionEnemyTarget, Kind: SkillKindMagic, Tag: SkillTagMagic, Minigame: MinigameCharge, Effect: SkillEffect{Damage: 1, BurnChance: FireboltBurnChance, BurnMinTurns: 2, BurnMaxTurns: 3}},
	// Sleep is the goblin-mage's signature. Magic-tagged so armor doesn't
	// gate the proc; press-minigame so the cast resolves quickly. Damage
	// is 0 — the only effect is the status. The mage doesn't pay MP
	// (enemies don't have an MP pool); a future caster class learning
	// Sleep can set the Cost field.
	{Skill: SkillSleep, Name: "Sleep", Cost: 0, TargetMode: ActionPartyTarget, Kind: SkillKindUtility, Tag: SkillTagMagic, Minigame: MinigamePress, Effect: SkillEffect{SleepMinTurns: SleepMinTurns, SleepMaxTurns: SleepMaxTurns}},
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

// SleepDuration rolls a uniform sleep duration in [Min, Max] inclusive.
// Returns 0 (no sleep) when the bounds are degenerate so a non-sleep
// skill that picks up the SkillEffect by accident doesn't randomly
// inflict a one-turn coma.
func (effect SkillEffect) SleepDuration(rng *rand.Rand) int {
	if effect.SleepMinTurns <= 0 || effect.SleepMaxTurns < effect.SleepMinTurns {
		return 0
	}
	span := effect.SleepMaxTurns - effect.SleepMinTurns
	if span <= 0 {
		return effect.SleepMinTurns
	}
	return effect.SleepMinTurns + rng.Intn(span+1)
}

// SkillTagFor returns the SkillTag of a skill — used by the damage path
// to decide whether to clip against the target's Armor (phys only).
// Unknown skills return SkillTagNone, which the armor-clip branch treats
// as "don't apply armor" (safer: a future skill author forgets to tag
// and we still produce damage instead of silently armoring the hit).
func SkillTagFor(skill SkillID) SkillTag {
	if def, ok := skillInfo(skill); ok {
		return def.Tag
	}
	return SkillTagNone
}

// ApplyArmor clamps physical damage down by the target's armor, never
// below 1 (every connection deals at least 1 — armor is a damp, not an
// immunity). Magic / Heal / Buff tagged actions bypass armor entirely:
// they call the skill's flat damage formula and skip this helper. A
// SkillTagNone caller goes through unaffected too so untagged legacy
// callers keep their old behaviour.
func ApplyArmor(dmg int, tag SkillTag, armor int) int {
	if tag != SkillTagPhys || armor <= 0 || dmg <= 0 {
		return dmg
	}
	reduced := dmg - armor
	if reduced < 1 {
		return 1
	}
	return reduced
}

// XPForLevel returns the XP cost to advance FROM level `level` to the
// next. Geometric curve: LevelXPBase × LevelXPRatio^(level-1) — so
// level 1→2 costs 100, level 2→3 costs 200, etc. Returns a positive
// integer; caller compares actor's accumulated XP against it.
func XPForLevel(level int) int {
	if level < BaseLevel {
		level = BaseLevel
	}
	cost := float64(LevelXPBase)
	for i := BaseLevel; i < level; i++ {
		cost *= LevelXPRatio
	}
	return int(cost)
}

// AddXP banks `amount` of experience onto a party member, processing
// multiple level-ups if the running total crosses several thresholds.
// Each level-up increments Level + queues a PendingLevelUps point-spend
// (the level-up modal drains the queue). Returns the number of levels
// the member just gained — callers use it for the "Warrior reaches
// level 3!" log line. No-op for amount<=0 or a dead member (HP <= 0,
// matching the "living members get XP" rule in AwardBattleXP).
func AddXP(m *PartyMember, amount int) int {
	if amount <= 0 || m == nil || m.HP <= 0 {
		return 0
	}
	if m.Level < BaseLevel {
		m.Level = BaseLevel
	}
	m.XP += amount
	levels := 0
	for {
		need := XPForLevel(m.Level)
		if need <= 0 || m.XP < need {
			break
		}
		m.XP -= need
		m.Level++
		m.PendingLevelUps++
		levels++
	}
	return levels
}

// FirstPendingLevelUp returns the index of the first party member with
// unspent level-up points, or -1 when nobody has any. The modal walks
// members in slice order — closing the modal on member N's last point
// advances to N+1 by calling this again.
func FirstPendingLevelUp(party []PartyMember) int {
	for i := range party {
		if party[i].PendingLevelUps > 0 {
			return i
		}
	}
	return -1
}

// Stat enumerates the six spendable level-up stats in display order.
// Used by the level-up modal as the row cursor and as the index into
// statTable below — adding a new stat is one row in that table plus
// one enum constant here, NOT three parallel switches.
type Stat int

const (
	StatSTR Stat = iota
	StatDEX
	StatINT
	StatWIS
	StatVIT
	StatSPD
	StatCount
)

// statSpec is one row in statTable: the label, a read accessor against
// a Stats value, and an in-place increment against a *Stats. Keyed
// implicitly by slice index (Stat's iota value), so the slice and the
// enum stay 1:1 — a runtime length-check in init() asserts that
// invariant the moment a future author forgets to update both.
type statSpec struct {
	Label string
	Get   func(Stats) int
	Add   func(*Stats)
}

var statTable = []statSpec{
	StatSTR: {Label: "STR", Get: func(s Stats) int { return s.STR }, Add: func(s *Stats) { s.STR++ }},
	StatDEX: {Label: "DEX", Get: func(s Stats) int { return s.DEX }, Add: func(s *Stats) { s.DEX++ }},
	StatINT: {Label: "INT", Get: func(s Stats) int { return s.INT }, Add: func(s *Stats) { s.INT++ }},
	StatWIS: {Label: "WIS", Get: func(s Stats) int { return s.WIS }, Add: func(s *Stats) { s.WIS++ }},
	StatVIT: {Label: "VIT", Get: func(s Stats) int { return s.VIT }, Add: func(s *Stats) { s.VIT++ }},
	StatSPD: {Label: "SPD", Get: func(s Stats) int { return s.SPD }, Add: func(s *Stats) { s.SPD++ }},
}

func init() {
	if len(statTable) != int(StatCount) {
		panic("core: statTable length must match StatCount — add a row when adding a Stat enum value")
	}
}

// StatLabel returns the 3-letter display label for a stat. Stable
// across the level-up modal and the Party Stats screen.
func StatLabel(s Stat) string {
	if s < 0 || int(s) >= len(statTable) {
		return "?"
	}
	return statTable[s].Label
}

// StatValue reads the named stat from a Stats block. Pairs with
// StatLabel for the modal's row rendering.
func StatValue(s Stats, st Stat) int {
	if st < 0 || int(st) >= len(statTable) {
		return 0
	}
	return statTable[st].Get(s)
}

// SpendStatPoint moves one PendingLevelUps point into the named stat.
// Returns true on success; false when there are no points left or stat
// is unknown. VIT spends auto-bump MaxHP via MaxHPFor and heal the
// member by the delta so the level-up feels rewarding instead of
// "your HP cap went up but you're still hurt."
func SpendStatPoint(m *PartyMember, stat Stat) bool {
	if m == nil || m.PendingLevelUps <= 0 {
		return false
	}
	if stat < 0 || int(stat) >= len(statTable) {
		return false
	}
	oldMaxHP := MaxHPFor(m.Stats)
	statTable[stat].Add(&m.Stats)
	if stat == StatVIT {
		newMaxHP := MaxHPFor(m.Stats)
		delta := newMaxHP - oldMaxHP
		m.MaxHP = newMaxHP
		m.HP += delta
		if m.HP > m.MaxHP {
			m.HP = m.MaxHP
		}
	}
	m.PendingLevelUps--
	return true
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

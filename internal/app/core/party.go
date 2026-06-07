package core

import (
	"fmt"
	"math/rand"
)

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
	// Skills is the per-class learned list. Index 0 is the class's
	// signature skill — shown in the action menu's Skill row by
	// default. Indices 1+ are the class's two thematic skills (e.g.
	// Warrior: Crushing Blow / Whirlwind; Wizard: Frost Lance / Arc
	// Bolt — see partyClassDefinitions for the per-class roster); the
	// in-battle Tab key cycles which one the Skill row casts. Always
	// length SkillsPerClass; init asserts the shape stays consistent.
	Skills [SkillsPerClass]SkillID
}

// SkillsPerClass is the slot count each party class learns. Fixed at
// 3 for now ("3 skills per char, we will refine later" per the
// design call) — bumping this requires extending every class def's
// Skills array and isn't a behavior the menu copes with dynamically
// yet, so it's a const rather than a configurable.
const SkillsPerClass = 3

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
// with three ticks; Sequence is the directional tap rhythm (Venom Strike);
// Reels is
// the slot-machine gamble; Recall is the show-then-hide memory pattern;
// Overcharge is a charge bar with a risky post-peak overload band.
type SkillMinigame int

const (
	MinigamePress SkillMinigame = iota
	MinigameCharge
	MinigameSequence
	MinigameReels
	MinigameRecall
	MinigameOvercharge
)

type skillDefinition struct {
	Skill SkillID
	Name  string
	// Description is the one-line UX blurb shown in the panels overlay
	// Skills tab. Lives on the registry so adding a new skill is one
	// row instead of "registry + a switch case in panels.go". Empty
	// for enemy-only skills (Sleep / Ingest) — the panels tab only
	// renders player-castable skills.
	Description string
	Cost        int
	TargetMode  ActionMode
	Kind        SkillKind
	// Tag classifies the skill for armor / future resist math and for
	// HUD color-coding. See SkillTag. Phys hits target.Armor; Magic /
	// Heal / Buff bypass.
	Tag      SkillTag
	Minigame SkillMinigame
	Effect   SkillEffect
	// PlayerCastable is true when this skill is a valid choice from a
	// party member's action menu. Enemy-only skills (Sleep, Ingest)
	// set this false so the action menu can't accidentally surface them
	// and an init-time assert can verify every player class's Skill
	// field points at a castable entry. SkillFor / PartySkill route
	// through this so a future "player learns Sleep" feature is one
	// field flip plus a handler registration, not a "find every
	// gatekeep switch" hunt.
	PlayerCastable bool
	// EnemyCastable flags skills the enemy AI's resolveEnemySpell
	// dispatcher knows how to fire. The editor's custom-enemy modal
	// reads this via EnemyCastableSkills to filter the skill picker
	// so authors don't accidentally assign a player-only skill to an
	// enemy that would silently never fire it. Adding a new enemy
	// skill is: flip this flag + add a case to resolveEnemySpell.
	EnemyCastable bool
	// PerBattleCastLimit caps how many times any one caster can fire
	// this skill in a single battle. 0 (default) means "uncapped" —
	// every existing skill leaves it zero so the behaviour is
	// opt-in. The Necromancer's SkillRaiseBones is the headline user
	// (capped at NecromancerRaiseLimit). The cap is checked against
	// Enemy.SkillCastCount[skill] in usableEnemySkills before the AI
	// even considers the skill; once spent, the skill drops out of
	// the AI's pick list for the rest of the encounter.
	PerBattleCastLimit int
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
	// AppliesIngest is true when a successful apply pulls the target out
	// of combat (Ingested status) until the caster dies. The mantrap's
	// signature. Carried as a registry flag so the apply path doesn't
	// have to branch on the SkillID itself — same shape as the Sleep /
	// Burn min/max fields above.
	AppliesIngest bool
	// StunChance is the per-apply probability that the skill inflicts
	// Stun on the target (skip-next-turn, doesn't break on damage like
	// Sleep does). Zero means "this skill doesn't stun" — apply path
	// can short-circuit without touching the RNG. StunMin/Max bound
	// the rolled duration in turns.
	StunChance   float64
	StunMinTurns int
	StunMaxTurns int
	// PoisonChance / Min / Max are the party-side equivalent. Lets a
	// player-cast skill (Thief's Venom Strike) inflict the Poison DoT
	// on an enemy target — symmetric with the existing Burn /
	// Sleep / Stun fields. Zero chance short-circuits the apply.
	PoisonChance   float64
	PoisonMinTurns int
	PoisonMaxTurns int
	// BindChance / Min / Max gate Webbed apply on the target. Webbed
	// halves SPD and refuses Ingest until cleared; the Cave Spider's
	// SkillWeb is the headline applier. Same fail-open shape as the
	// other status fields — zero chance short-circuits.
	BindChance   float64
	BindMinTurns int
	BindMaxTurns int
	// ConfuseChance / Min / Max gate the Confused apply. Confused
	// rolls per-action retarget (random friend or foe) when the
	// member acts. WIS-resistible at apply time so a high-WIS
	// Cleric is harder to confuse than a low-WIS Warrior.
	ConfuseChance   float64
	ConfuseMinTurns int
	ConfuseMaxTurns int
	// AppliesAOEParty flags a skill whose damage hits EVERY living
	// party member instead of a single target. Stone Golem's
	// Stoneslam is the first user; the apply path reads this flag
	// and loops damage across living slots with per-target armor
	// applied. Damage = Effect.Damage + actor.SpellPower.
	AppliesAOEParty bool
	// AppliesSummonSkeleton flags a skill that inserts a fresh
	// Skeleton into the caster's pack mid-battle. The Necromancer
	// is the headline user; the apply path constructs an Enemy via
	// NewEnemy(EnemySkeleton) and queue-inserts it. Carrier flag
	// rather than a hard-coded SkillID branch so future raises
	// (Raise Zombie, Raise Ghoul) can reuse the apply.
	AppliesSummonSkeleton bool
	// AppliesAOEEnemies declares that a player skill hits EVERY living
	// enemy in the pack (Swipe / Whirlwind / Arc Bolt) rather than a
	// single target — the player-side mirror of AppliesAOEParty. The
	// render-side targeting preview (SkillTargetsAllEnemies) reads it to
	// fan the chevron across the whole enemy line; the battle apply path
	// implements the actual multi-hit in each skill's applyAoEDamage
	// handler. Set it on exactly the skills whose handler loops the pack
	// so the preview and the hit can't disagree (a future single-target
	// ActionMenu skill won't read as AoE the way the old shape-heuristic
	// risked).
	AppliesAOEEnemies bool
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
	{Class: ClassWarrior, Name: "Warrior", Stats: Stats{STR: 6, DEX: 2, INT: 1, WIS: 1, VIT: 5, SPD: 3}, MaxMP: 4,
		Skills: [SkillsPerClass]SkillID{SkillSwipe, SkillCrushingBlow, SkillWhirlwind}},
	{Class: ClassCleric, Name: "Cleric", Stats: Stats{STR: 2, DEX: 2, INT: 2, WIS: 6, VIT: 4, SPD: 4}, MaxMP: 9,
		Skills: [SkillsPerClass]SkillID{SkillPrayer, SkillMassMend, SkillSmite}},
	{Class: ClassThief, Name: "Thief", Stats: Stats{STR: 3, DEX: 6, INT: 2, WIS: 1, VIT: 4, SPD: 6}, MaxMP: 5,
		Skills: [SkillsPerClass]SkillID{SkillSteal, SkillBackstab, SkillVenomStrike}},
	{Class: ClassWizard, Name: "Wizard", Stats: Stats{STR: 1, DEX: 2, INT: 6, WIS: 2, VIT: 4, SPD: 4}, MaxMP: 10,
		Skills: [SkillsPerClass]SkillID{SkillFirebolt, SkillFrostLance, SkillArcBolt}},
}

// partyClassByID is the O(1) lookup for partyClassDefinitions. Built once
// at init for the same reason as skillByID — partyClassInfo is called per
// frame from selectors and per-party-member render code.
var partyClassByID = BuildRegistry(partyClassDefinitions, func(d PartyClassDefinition) PartyClass { return d.Class })

// Skill registry. Effect.Damage / Effect.Heal are flat baselines that the
// stat-scaled formulas add on top (see types.go's MeleeDamage etc.). Tuned
// so that a focused class with their stat at 8 lands roughly the same total
// damage as the pre-stats values: e.g. Wizard (INT 8) Firebolt = 8 + 2 = 10
// pre-quality, scaling further with timing. Difficulty pass: bases trimmed
// and Firebolt's burn-chance pulled down so a single Excellent doesn't
// auto-burn every cast.
var skillDefinitions = []skillDefinition{
	{Skill: SkillSwipe, Name: "Swipe", Description: "STR-scaled cleave through every living enemy in the pack.", Cost: 2, TargetMode: ActionMenu, Kind: SkillKindMelee, Tag: SkillTagPhys, Minigame: MinigamePress, Effect: SkillEffect{Damage: 0, AppliesAOEEnemies: true}, PlayerCastable: true},
	{Skill: SkillPrayer, Name: "Prayer", Description: "WIS-scaled single-ally heal. Charge bar — release at peak.", Cost: 4, TargetMode: ActionPartyTarget, Kind: SkillKindHeal, Tag: SkillTagHeal, Minigame: MinigameCharge, Effect: SkillEffect{Heal: 1}, PlayerCastable: true},
	{Skill: SkillSteal, Name: "Steal", Description: "Pickpocket the target. Stop the reels — matches drive the chance.", Cost: 0, TargetMode: ActionEnemyTarget, Kind: SkillKindUtility, Tag: SkillTagNone, Minigame: MinigameReels, Effect: SkillEffect{StealChance: StealBaseChance}, PlayerCastable: true},
	{Skill: SkillFirebolt, Name: "Firebolt", Description: "INT-scaled magic damage. Charge; release past the peak to Overcharge (bonus damage, burns you). Chance to inflict Burn.", Cost: 5, TargetMode: ActionEnemyTarget, Kind: SkillKindMagic, Tag: SkillTagMagic, Minigame: MinigameOvercharge, Effect: SkillEffect{Damage: 1, BurnChance: FireboltBurnChance, BurnMinTurns: 2, BurnMaxTurns: 3}, PlayerCastable: true, EnemyCastable: true},
	// Crushing Blow (Warrior): charge-up single-target physical hit.
	// +4 base on top of STR, 3 MP. On Great/Excellent timing rolls
	// CrushingBlowStunChance for the Stun proc — heavy-hit fantasy
	// that pays off in lockout when the player nails the charge.
	{Skill: SkillCrushingBlow, Name: "Crushing Blow", Description: "STR-scaled heavy hit. Charge. Stun proc on Great+.", Cost: 3, TargetMode: ActionEnemyTarget, Kind: SkillKindMelee, Tag: SkillTagPhys, Minigame: MinigameCharge, Effect: SkillEffect{Damage: 4, StunChance: CrushingBlowStunChance, StunMinTurns: StunMinTurns, StunMaxTurns: StunMaxTurns}, PlayerCastable: true},
	// Whirlwind (Warrior): charge-up AoE physical. +2 base per target,
	// 4 MP. Damage scales hard with quality — a Miss whiffs across
	// every enemy for chip damage, an Excellent reaps everyone. The
	// cost is the charge wind-up risk: spinning up in a tight room
	// can leave the warrior exposed if an enemy beats them on SPD.
	{Skill: SkillWhirlwind, Name: "Whirlwind", Description: "STR-scaled AoE cleave. Charge — quality scales hard.", Cost: 4, TargetMode: ActionMenu, Kind: SkillKindMelee, Tag: SkillTagPhys, Minigame: MinigameCharge, Effect: SkillEffect{Damage: 2, AppliesAOEEnemies: true}, PlayerCastable: true},
	// Mass Mend (Cleric): charge-up AoE heal. +1 base + WIS per ally,
	// 6 MP. Smaller per-target than Prayer but covers the whole alive
	// party — the cleric's answer to a Whirlwind / multi-poison turn
	// that left everyone wounded at once.
	{Skill: SkillMassMend, Name: "Mass Mend", Description: "WIS-scaled heal across the whole alive party. Charge.", Cost: 6, TargetMode: ActionMenu, Kind: SkillKindHeal, Tag: SkillTagHeal, Minigame: MinigameCharge, Effect: SkillEffect{Heal: 1}, PlayerCastable: true},
	// Smite (Cleric): press-tap single-target magic damage. +2 base +
	// WIS, 3 MP. Lets the cleric chip enemies when nobody needs a
	// heal — press minigame keeps it fast so it doesn't compete with
	// Prayer's charge for screen time.
	{Skill: SkillSmite, Name: "Smite", Description: "WIS-scaled magic damage. Press tap, no burn.", Cost: 3, TargetMode: ActionEnemyTarget, Kind: SkillKindMagic, Tag: SkillTagMagic, Minigame: MinigamePress, Effect: SkillEffect{Damage: 2}, PlayerCastable: true},
	// Backstab (Thief): charge-up single-target physical hit. +2 base
	// + STR, 3 MP. Damage DOUBLES on Excellent timing (timing-gated
	// crit) — the thief's signature "land the hit perfectly or it's
	// just a poke." The crit multiplier lives in applyBackstab since
	// it's quality-conditional and SkillEffect doesn't carry a
	// generic crit field.
	{Skill: SkillBackstab, Name: "Backstab", Description: "STR-scaled phys hit. Damage doubles on Excellent.", Cost: 3, TargetMode: ActionEnemyTarget, Kind: SkillKindMelee, Tag: SkillTagPhys, Minigame: MinigameCharge, Effect: SkillEffect{Damage: 2}, PlayerCastable: true},
	// Venom Strike (Thief): sequence-minigame single-target physical
	// + Poison apply. +1 base + STR per hit, 3 MP. Pairs with Steal's
	// sequence rhythm — the thief's two skills share a "rhythm-game"
	// identity. Poison-apply chance scales with quality so a clean
	// sequence reliably lands the DoT.
	{Skill: SkillVenomStrike, Name: "Venom Strike", Description: "STR-scaled phys hit. Sequence — applies Poison.", Cost: 3, TargetMode: ActionEnemyTarget, Kind: SkillKindMelee, Tag: SkillTagPhys, Minigame: MinigameSequence, Effect: SkillEffect{Damage: 1, PoisonChance: VenomStrikePoisonChance, PoisonMinTurns: PoisonMinTurns, PoisonMaxTurns: PoisonMaxTurns}, PlayerCastable: true},
	// Frost Lance (Wizard): charge-up single-target magic. +2 base +
	// INT, 5 MP. On Great/Excellent timing applies a 1-turn Stun —
	// lower base damage than Firebolt but reliable lockout instead
	// of the burn-chance lottery. Different tactical role.
	{Skill: SkillFrostLance, Name: "Frost Lance", Description: "INT-scaled magic damage. Reliable Stun on Great+.", Cost: 5, TargetMode: ActionEnemyTarget, Kind: SkillKindMagic, Tag: SkillTagMagic, Minigame: MinigameCharge, Effect: SkillEffect{Damage: 2, StunChance: FrostLanceStunChance, StunMinTurns: FrostLanceStunTurns, StunMaxTurns: FrostLanceStunTurns}, PlayerCastable: true},
	// Arc Bolt (Wizard): sequence-minigame AoE magic. +1 base + INT
	// per target, 6 MP. Each correct tap in the sequence reads as a
	// new bolt arcing to the next enemy; on apply, all living enemies
	// take quality-scaled damage. Magic-tagged so amoebas don't
	// shrug it off. Pricier than Firebolt because it hits everyone.
	{Skill: SkillArcBolt, Name: "Arc Bolt", Description: "INT-scaled magic AoE. Memorize the glyph pattern, then recall it — arcs to every enemy.", Cost: 6, TargetMode: ActionMenu, Kind: SkillKindMagic, Tag: SkillTagMagic, Minigame: MinigameRecall, Effect: SkillEffect{Damage: 1, AppliesAOEEnemies: true}, PlayerCastable: true},
	// Sleep is the goblin-mage's signature. Magic-tagged so armor doesn't
	// gate the proc; press-minigame so the cast resolves quickly. Damage
	// is 0 — the only effect is the status. The mage doesn't pay MP
	// (enemies don't have an MP pool); a future caster class learning
	// Sleep can set the Cost field AND flip PlayerCastable.
	{Skill: SkillSleep, Name: "Sleep", Cost: 0, TargetMode: ActionPartyTarget, Kind: SkillKindUtility, Tag: SkillTagMagic, Minigame: MinigamePress, Effect: SkillEffect{SleepMinTurns: SleepMinTurns, SleepMaxTurns: SleepMaxTurns}, EnemyCastable: true},
	// Ingest mirrors Sleep's shape: enemy-only, Magic-tagged, single
	// party target. AppliesIngest carries the "removed from combat until
	// the caster dies" behaviour so the apply path can stay registry-
	// driven instead of branching on the SkillID itself.
	{Skill: SkillIngest, Name: "Ingest", Cost: 0, TargetMode: ActionPartyTarget, Kind: SkillKindUtility, Tag: SkillTagMagic, Minigame: MinigamePress, Effect: SkillEffect{AppliesIngest: true}, EnemyCastable: true},
	// Web (Cave Spider): enemy-only, Magic-tagged, single party target.
	// Applies the Webbed status — halves the member's effective SPD and
	// blocks Ingest until cleared. Duration carried in BindMin/Max
	// (3-turn fixed today); a future enchant or party skill can reuse
	// the field without touching the apply path.
	{Skill: SkillWeb, Name: "Web", Cost: 0, TargetMode: ActionPartyTarget, Kind: SkillKindUtility, Tag: SkillTagMagic, Minigame: MinigamePress, Effect: SkillEffect{BindChance: 1.0, BindMinTurns: SpiderWebbedMinTurns, BindMaxTurns: SpiderWebbedMaxTurns}, EnemyCastable: true},
	// Confuse (Will-o'-Wisp): enemy-only, Magic-tagged, single party
	// target. Applies the Confused status — per-action retarget roll
	// when the afflicted member acts. WIS resists on the apply roll
	// so a high-WIS Cleric is sturdier against it; duration is fixed
	// in ConfuseMin/Max.
	{Skill: SkillConfuse, Name: "Confuse", Cost: 0, TargetMode: ActionPartyTarget, Kind: SkillKindUtility, Tag: SkillTagMagic, Minigame: MinigamePress, Effect: SkillEffect{ConfuseChance: 1.0, ConfuseMinTurns: WispConfuseMinTurns, ConfuseMaxTurns: WispConfuseMaxTurns}, EnemyCastable: true},
	// Stoneslam (Stone Golem): enemy-only, Phys-tagged, AoE. Damage
	// hits every living party member; per-target armor applies
	// because the tag is Phys. The slap value comes from caster
	// SpellPower + Effect.Damage scaled by quality at apply time.
	{Skill: SkillStoneslam, Name: "Stoneslam", Cost: 0, TargetMode: ActionMenu, Kind: SkillKindMelee, Tag: SkillTagPhys, Minigame: MinigamePress, Effect: SkillEffect{Damage: 2, AppliesAOEParty: true}, EnemyCastable: true},
	// Raise Bones (Necromancer): enemy-only, Magic-tagged, no target
	// (the summon lands in the caster's own pack). Capped at
	// NecromancerRaiseLimit casts per battle via PerBattleCastLimit
	// — the AI's usableEnemySkills check drops the skill from the
	// pick list once the cap is hit, so no apply-time gate is
	// needed.
	{Skill: SkillRaiseBones, Name: "Raise Bones", Cost: 0, TargetMode: ActionMenu, Kind: SkillKindUtility, Tag: SkillTagMagic, Minigame: MinigamePress, Effect: SkillEffect{AppliesSummonSkeleton: true}, EnemyCastable: true, PerBattleCastLimit: NecromancerRaiseLimit},
}

// SkillPlayerCastable reports whether the skill can appear in a party
// member's action menu. Enemy-only skills (Sleep, Ingest) return false.
// Unknown skill IDs return false too — a future-typo'd registration is
// kept OUT of the menu, not silently surfaced.
func SkillPlayerCastable(s SkillID) bool {
	def, ok := skillInfo(s)
	return ok && def.PlayerCastable
}

// PlayerCastableSkills returns every skill flagged PlayerCastable, in
// registry declaration order. Used by battle's init() to assert each
// one has a handler registered, and reusable for any future "what
// skills can I learn?" UI that needs the canonical list.
func PlayerCastableSkills() []SkillID {
	out := make([]SkillID, 0, len(skillDefinitions))
	for _, def := range skillDefinitions {
		if def.PlayerCastable {
			out = append(out, def.Skill)
		}
	}
	return out
}

// skillByID is the O(1) lookup table for skillDefinitions. Built once at
// init so per-frame skillInfo calls don't linear-walk the registry. The
// registry slice is still the source of truth (controls iteration order
// when the editor lists skills, etc.); the map is just a read cache.
var skillByID = BuildRegistry(skillDefinitions, func(d skillDefinition) SkillID { return d.Skill })

func PartyClasses() []PartyClassDefinition {
	defs := make([]PartyClassDefinition, len(partyClassDefinitions))
	copy(defs, partyClassDefinitions)
	return defs
}

func partyClassInfo(class PartyClass) (PartyClassDefinition, bool) {
	def, ok := partyClassByID[class]
	return def, ok
}

// PartySkill returns the skill the action menu's Skill row currently
// casts for this member. With multiple skills per class (SkillsPerClass)
// the result depends on member.SkillCursor; the in-battle Tab key
// cycles that index. Out-of-range cursors clamp to 0 so a corrupted
// save can't crash the action menu.
func PartySkill(member PartyMember) SkillID {
	def, ok := partyClassInfo(member.Class)
	if !ok {
		return SkillNone
	}
	idx := member.SkillCursor
	if idx < 0 || idx >= len(def.Skills) {
		idx = 0
	}
	return def.Skills[idx]
}

// PartySkills returns the full learned skill array for a member.
// Used by the panels overlay's Skills tab, the battle skill submenu,
// and the cycle logic. Returns the fixed-size array (not a slice) so
// per-frame callers don't allocate — previously this minted a fresh
// slice every frame. Callers iterate with `for i, s := range arr`.
// Unknown class returns a zero array; range yields SkillsPerClass
// SkillNone values which callers already filter.
func PartySkills(member PartyMember) [SkillsPerClass]SkillID {
	if def, ok := partyClassInfo(member.Class); ok {
		return def.Skills
	}
	return [SkillsPerClass]SkillID{}
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

// SkillDescription returns the panels-overlay blurb for a skill. Pure
// UX text — empty for skills without an authored description (enemy-
// only skills, future skills missing a row). Sourced from the skill
// registry's Description field so adding a skill is one row.
func SkillDescription(skill SkillID) string {
	if def, ok := skillInfo(skill); ok {
		return def.Description
	}
	return ""
}

func SkillCost(skill SkillID) int {
	if def, ok := skillInfo(skill); ok {
		return def.Cost
	}
	return 0
}

func CanAffordSkill(m PartyMember, skill SkillID) bool {
	return m.MP >= SkillCost(skill)
}

// SpendSkillMP checks affordability and, if the member can pay, deducts the
// skill's MP cost — returning whether the cast may proceed. The single seam for
// "pay for a skill": battle's chargeMP wraps it (adding the battle-status
// message on failure) and the out-of-battle cast path calls it directly, so a
// future "refund on cancel" / "VIT raises the MP pool" rule is one edit, not
// several inlined `MP -= SkillCost` sites.
func SpendSkillMP(m *PartyMember, skill SkillID) bool {
	if m == nil || !CanAffordSkill(*m, skill) {
		return false
	}
	m.MP -= SkillCost(skill)
	return true
}

// SkillCastLimitFor returns the registry's PerBattleCastLimit for a
// skill — 0 means "uncapped." The battle AI's usableEnemySkills
// filter reads this to drop a skill from the cast set once an enemy
// has spent it PerBattleCastLimit times in the encounter (tracked on
// Enemy.SkillCastCount). Wraps the lookup so callers don't have to
// import skillInfo and so the "unknown skill → 0" fail-open lives
// in one place.
func SkillCastLimitFor(skill SkillID) int {
	if def, ok := skillInfo(skill); ok {
		return def.PerBattleCastLimit
	}
	return 0
}

func SkillTargetMode(skill SkillID) ActionMode {
	if def, ok := skillInfo(skill); ok {
		return def.TargetMode
	}
	return ActionMenu
}

// SkillTargetsAllEnemies reports whether a player skill hits every
// living enemy in the pack (Swipe / Whirlwind / Arc Bolt). Reads the
// declarative SkillEffect.AppliesAOEEnemies flag rather than inferring
// from skill shape, so the render-side targeting preview (which fans
// the chevron across the whole enemy line) stays pinned to the same
// marker the AoE skills carry. Stoneslam hits the PARTY (AppliesAOEParty)
// and is EnemyCastable-only, so it's excluded.
func SkillTargetsAllEnemies(skill SkillID) bool {
	def, ok := skillInfo(skill)
	return ok && def.PlayerCastable && def.Effect.AppliesAOEEnemies
}

// SkillHealableOutOfBattle reports whether a player skill can be cast
// from the panels overlay's Skills tab while exploring — currently the
// Heal-tagged skills (Prayer, Mass Mend). Damage / utility skills stay
// battle-only. Used by the Skills-tab "Use" action to decide which rows
// are castable out of combat.
func SkillHealableOutOfBattle(skill SkillID) bool {
	def, ok := skillInfo(skill)
	return ok && def.PlayerCastable && def.Tag == SkillTagHeal
}

// OutOfBattleHeals returns the member's skills castable as a heal outside
// battle (SkillHealableOutOfBattle), in class-skill order — e.g. the Cleric's
// {Prayer, Mass Mend}. The Skills-tab "Use" flow reads this to decide whether
// to cast directly (one heal), pop a chooser (multiple), or refuse (none).
// Allocates a small slice; called on a button press / while the chooser modal
// is open, never on a per-frame combat path.
func OutOfBattleHeals(m PartyMember) []SkillID {
	var out []SkillID
	for _, s := range PartySkills(m) {
		if s != SkillNone && SkillHealableOutOfBattle(s) {
			out = append(out, s)
		}
	}
	return out
}

// HealMember restores up to `amount` HP to a LIVING, non-ingested member,
// clamped at MaxHP. It never revives (a downed member at HP <= 0 is
// untouched — reviving is not a heal), ignores non-positive amounts, and
// skips a member ingested by a mantrap (out of reach, untargetable —
// matching applyMassMend's in-battle skip). Item / skill use routes
// through this so the clamp + no-revive + ingest rules live in one place.
func HealMember(m *PartyMember, amount int) {
	if m == nil || amount <= 0 || m.HP <= 0 || m.Ingested {
		return
	}
	m.HP += amount
	if m.HP > m.MaxHP {
		m.HP = m.MaxHP
	}
}

// RestoreMP tops up a member's MP by amount, clamped at MaxMP, and returns the
// actual amount restored (0 if already full / not eligible). Mirrors
// HealMember on the MP axis: a downed (HP<=0) or ingested member can't drink.
// Used by the Magic Phial's use paths.
func RestoreMP(m *PartyMember, amount int) int {
	if m == nil || amount <= 0 || m.HP <= 0 || m.Ingested {
		return 0
	}
	before := m.MP
	m.MP += amount
	if m.MP > m.MaxMP {
		m.MP = m.MaxMP
	}
	return m.MP - before
}

func SkillEffectFor(skill SkillID) SkillEffect {
	if def, ok := skillInfo(skill); ok {
		return def.Effect
	}
	return SkillEffect{}
}

// scaleDamageByKind applies the per-kind stat-scaling rule to a skill's
// base damage: Melee adds STR, Magic adds INT, anything else passes the
// base through. Shared by SkillDamage (base effect) and SkillDamageFor
// (tier-augmented effect) so the dispatch lives in one place.
func scaleDamageByKind(kind SkillKind, stats Stats, base int) int {
	switch kind {
	case SkillKindMelee:
		return MeleeDamage(stats, base)
	case SkillKindMagic:
		return MagicDamage(stats, base)
	default:
		return base
	}
}

// scaleHealByKind applies the per-kind stat-scaling rule to a skill's base
// heal: Heal kind adds WIS, anything else passes the base through. Shared
// by SkillHeal and SkillHealFor.
func scaleHealByKind(kind SkillKind, stats Stats, base int) int {
	if kind == SkillKindHeal {
		return HealAmount(stats, base)
	}
	return base
}

// SkillDamage computes a skill's pre-quality damage from the actor's stats,
// dispatching on the skill's Kind. Quality scaling (ScaleDamage) applies on
// top at the call site.
func SkillDamage(stats Stats, skill SkillID) int {
	def, ok := skillInfo(skill)
	if !ok {
		return 0
	}
	return scaleDamageByKind(def.Kind, stats, def.Effect.Damage)
}

// SkillHeal computes a skill's pre-quality heal from the actor's stats,
// dispatching on Kind. Quality scaling (ScaleHeal) applies on top at the
// call site.
func SkillHeal(stats Stats, skill SkillID) int {
	def, ok := skillInfo(skill)
	if !ok {
		return 0
	}
	return scaleHealByKind(def.Kind, stats, def.Effect.Heal)
}

// SumStatPending totals the staged per-stat allocations the level-up
// modal carries. Both the explore-side input handler (used to gate
// "is there budget for another stat point?") and the render-side
// drawing (used to display "staged N / N") used to inline the same
// 5-line sum. One helper means a future "skip slots flagged as
// locked" rule lands in one place.
func SumStatPending(p [StatCount]int) int {
	n := 0
	for _, v := range p {
		n += v
	}
	return n
}

// rollDuration is the shared "uniform draw on [min, max] with
// fail-open" generator behind every status-duration helper on
// SkillEffect (Burn / Sleep / Stun / Poison). Degenerate bounds —
// min <= 0 or max < min — return 0 so a non-status skill that picks
// up the SkillEffect by accident doesn't roll a phantom DoT. Single
// helper means four near-identical 8-liners collapse to one body;
// each public method is now a thin wrapper that names its fields.
func rollDuration(rng *rand.Rand, min, max int) int {
	if min <= 0 || max < min {
		return 0
	}
	return RandRangeI(rng, min, max)
}

// BurnDuration rolls a uniform burn duration in [Min, Max]. Routes
// through rollDuration so every status-duration helper on SkillEffect
// shares one body — earlier this method open-coded the rng math with
// a slightly different degenerate-bounds rule (`<=` instead of `<`),
// which let an inverted Min/Max return Min where the other helpers
// returned 0. Now consistent with Sleep/Stun/Bind/Confuse/Poison.
func (effect SkillEffect) BurnDuration(rng *rand.Rand) int {
	return rollDuration(rng, effect.BurnMinTurns, effect.BurnMaxTurns)
}

// SleepDuration rolls a uniform sleep duration in [Min, Max] inclusive.
// Returns 0 (no sleep) when the bounds are degenerate so a non-sleep
// skill that picks up the SkillEffect by accident doesn't randomly
// inflict a one-turn coma.
func (effect SkillEffect) SleepDuration(rng *rand.Rand) int {
	return rollDuration(rng, effect.SleepMinTurns, effect.SleepMaxTurns)
}

// StunDuration rolls a uniform stun duration in [Min, Max] inclusive.
// Same fail-open shape as SleepDuration — degenerate bounds return 0
// so a non-stun skill picking up the SkillEffect by accident doesn't
// stun anyone.
func (effect SkillEffect) StunDuration(rng *rand.Rand) int {
	return rollDuration(rng, effect.StunMinTurns, effect.StunMaxTurns)
}

// BindDuration rolls a uniform bind duration in [Min, Max]. Same
// fail-open shape as SleepDuration — degenerate bounds return 0.
// Used by handleEnemyWeb to land the Cave Spider's Bound status.
func (effect SkillEffect) BindDuration(rng *rand.Rand) int {
	return rollDuration(rng, effect.BindMinTurns, effect.BindMaxTurns)
}

// ConfuseDuration rolls a uniform confuse duration in [Min, Max].
// Same fail-open shape as the rest — degenerate bounds return 0.
// Used by handleEnemyConfuse for the Will-o'-Wisp's Confused status.
func (effect SkillEffect) ConfuseDuration(rng *rand.Rand) int {
	return rollDuration(rng, effect.ConfuseMinTurns, effect.ConfuseMaxTurns)
}

// PoisonDuration rolls a uniform poison duration in [Min, Max].
// Mirrors BurnDuration / SleepDuration / StunDuration — degenerate
// bounds return 0 so a non-poison skill picking up the effect by
// accident doesn't poison anyone. Used by applyVenomStrike.
func (effect SkillEffect) PoisonDuration(rng *rand.Rand) int {
	return rollDuration(rng, effect.PoisonMinTurns, effect.PoisonMaxTurns)
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
	if tag != SkillTagPhys {
		return dmg
	}
	return mitigate(dmg, armor)
}

// mitigate subtracts `soak` from `dmg` with a floor of 1 (any
// connection deals at least 1 — mitigation is a damp, not immunity).
// Damage that's already 0 or a non-positive soak pass through. Shared
// by ApplyArmor (phys) and ApplyMagicDefense (magic) so the floor-1
// rule lives in one place.
func mitigate(dmg, soak int) int {
	if soak <= 0 || dmg <= 0 {
		return dmg
	}
	if reduced := dmg - soak; reduced > 1 {
		return reduced
	}
	return 1
}

// MagicDefense returns the magic mitigation value derived from a Stats
// block. Mirrors ApplyArmor's role on the phys side — currently WIS in
// flat 1:1 (every point of WIS soaks 1 damage off a magic hit), but a
// future caster-equipment pass can layer +MDef gear into this seam.
func MagicDefense(s Stats) int {
	return s.WIS
}

// ApplyMagicDefense clamps magic-tagged damage by the target's MDef,
// floor 1. Symmetrical to ApplyArmor: phys hits clip against Armor,
// magic hits clip against MDef. Heal/Buff/Phys/None tagged actions
// bypass — phys hits already went through ApplyArmor and shouldn't be
// double-soaked. Keeps the no-stack rule with armor explicit.
func ApplyMagicDefense(dmg int, tag SkillTag, mdef int) int {
	if tag != SkillTagMagic {
		return dmg
	}
	return mitigate(dmg, mdef)
}

// ApplyFlatDamage applies a pre-resolved (already-mitigated) flat
// `amount` to an actor: stamps the standard damage flash and floors HP
// at 0. Returns true if this hit dropped the actor to 0. Pointer-based
// because Enemy and PartyMember are distinct structs that carry the
// same HP / DamageFlash fields. The shared HP-floor + flash contract
// behind the in-battle damage helpers AND the out-of-battle poison
// tick — so "Poison ticks for VIT% instead of flat" or a flash-timing
// change lands in one place. Sleep-wake, recoil, mitigation, popups,
// audio, and death-status cleanup stay with each caller because they
// legitimately differ by context.
func ApplyFlatDamage(hp *int, flash *float32, amount int) (died bool) {
	*flash = FlashDuration
	*hp -= amount
	if *hp <= 0 {
		*hp = 0
		return true
	}
	return false
}

// ApplyHitRecoil arms the "this actor visibly took a real hit" reaction
// shared by the enemy and party damage paths: on positive `damage` the
// knockback recoil timer arms and any active Sleep breaks ("violence
// breaks the spell"). A zero/soaked hit leaves both untouched. Pointer-
// based for the same Enemy/PartyMember reason as ApplyFlatDamage. The
// out-of-battle poison tick deliberately does NOT call this — it has no
// recoil and wakes only on a lethal tick.
func ApplyHitRecoil(knockback *float32, sleep *int, damage int) {
	if damage <= 0 {
		return
	}
	*knockback = HitKnockbackDuration
	if *sleep > 0 {
		*sleep = 0
	}
}

// DodgeChance returns the [0, 1] probability a defender sidesteps an
// incoming basic attack. See config.go's DodgePerDEX / DodgeCap for
// tuning notes. Skill-applied damage is not currently dodgeable —
// mirrors MeleeAccuracy, which only gates basic attacks.
func DodgeChance(s Stats) float64 {
	return Clamp(DodgePerDEX*float64(s.DEX), 0, DodgeCap)
}

// RollChance returns true with probability `p` against `rng`. The
// single "did this probabilistic check land?" idiom shared by
// RollDodge, RollCrit, MemberAttackHits, and any future status-proc /
// steal / lifesteal roll — keeps the dice-edge contract (`<`, not
// `<=`) and the clamp behavior consistent. p outside [0, 1] is
// allowed: p<=0 always returns false, p>=1 always returns true.
func RollChance(rng *rand.Rand, p float64) bool {
	if p <= 0 {
		return false
	}
	if p >= 1 {
		return true
	}
	return rng.Float64() < p
}

// RollDodge rolls a dodge attempt for the defender against `rng`. True
// means the incoming basic attack misses entirely.
func RollDodge(rng *rand.Rand, s Stats) bool {
	return RollChance(rng, DodgeChance(s))
}

// CritChance returns the [0, 1] probability that a connecting damage
// roll crits. DEX is the linear driver; timing grade adds a per-grade
// bonus from timingGrades.CritBonus. Returned chance is capped at
// CritCap so even a max-DEX hero on Excellent timing leaves some swing
// in the dice. Quality outside the table contributes no bonus (defensive
// against a future grade enum extension that forgets the CritBonus
// column — init asserts the table length, so this branch is dead today).
func CritChance(s Stats, quality int) float64 {
	base := CritBaseline + CritPerDEX*float64(s.DEX)
	if quality >= 0 && quality < len(timingGrades) {
		base += timingGrades[quality].CritBonus
	}
	return Clamp(base, 0, CritCap)
}

// RollCrit rolls a crit attempt against `rng`. True means the caller
// should multiply the post-armor damage by CritMultiplier and surface
// "Critical!" in the combat log.
func RollCrit(rng *rand.Rand, s Stats, quality int) bool {
	return RollChance(rng, CritChance(s, quality))
}

// ShortenStatusDuration returns the rolled status duration after
// subtracting wis/StatusShortenDivisor turns (floor 1). Applied when an
// enemy lands Sleep / Poison / Bound / Confuse on a party member — a
// high-WIS Cleric shrugs statuses off faster than a low-WIS Warrior.
// Burn isn't called here because the party doesn't get burned today
// (Firebolt is player → enemy only); when an enemy gains a burn skill,
// the symmetric path (enemy WIS shortens player-applied burn) wires
// through this same helper.
func ShortenStatusDuration(duration, wis int) int {
	if duration <= 0 {
		return duration
	}
	if wis <= 0 || StatusShortenDivisor <= 0 {
		return duration
	}
	shaved := duration - wis/StatusShortenDivisor
	if shaved < 1 {
		return 1
	}
	return shaved
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
		// Saturate before the curve can overflow float64→int at absurd
		// levels; keeps the documented "positive integer" contract.
		if cost >= MaxLevelXPCost {
			return MaxLevelXPCost
		}
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
		// Each level grants LevelStatPoints stat points (default 3)
		// to spend in the level-up modal PLUS LevelSkillPoints skill
		// points (default 1) that drop into the member's SkillPoints
		// pool. Skill points are spent later from the Skills panel's
		// tree UI; the level-up modal handles stat points only.
		m.PendingLevelUps += LevelStatPoints
		m.SkillPoints += LevelSkillPoints
		levels++
	}
	return levels
}

// FirstPendingLevelUp returns the index of the first party member
// with unspent stat points, or -1 when nobody has any. The level-up
// modal walks members in slice order — closing the modal on member
// N's last point advances to N+1 via another call. SkillPoints live
// outside this gate (they're spent at the player's leisure from the
// Skills panel, so the modal doesn't block on them).
func FirstPendingLevelUp(party []PartyMember) int {
	for i := range party {
		if party[i].PendingLevelUps > 0 {
			return i
		}
	}
	return -1
}

// HasUnspentPoints reports whether a party member has any allocation
// debt: unspent stat points (PendingLevelUps) OR unspent skill
// points (SkillPoints). The party card's "+" badge, the Tome's
// Character tab "Spend N" hint, and any future "press X to allocate"
// prompt all gate on this single predicate so the UI signals stay
// consistent. Keep ANY two-counter contract changes here (e.g. if
// a future "free respec" path zeroes stats without touching skill
// points, the badge logic doesn't need to be hunted down).
func HasUnspentPoints(m PartyMember) bool {
	return m.PendingLevelUps > 0 || m.SkillPoints > 0
}

// PartyStatusKind tags the single highest-priority status a party
// member is currently afflicted by. Two render surfaces (the party
// card status label in render/party.go and the Tome's Stats badge
// in render/panels.go) used to walk the priority ladder inline,
// each with its own switch — and they drifted (the party card
// missed Sleep/Stun/Bound/Confused entirely; the Tome badge missed
// Stun/Bound/Confused). One enum + one resolver here is now the
// single source of truth.
type PartyStatusKind int

const (
	PartyStatusNone PartyStatusKind = iota
	PartyStatusDown
	PartyStatusIngested
	PartyStatusWebbed
	PartyStatusConfused
	PartyStatusStunned
	PartyStatusAsleep
	PartyStatusPoisoned
	PartyStatusDefending
	// PartyStatusCount is the length-assert sentinel for any render-side
	// table indexed by PartyStatusKind. New status kinds slot in above
	// this row; the assert at the registry's init catches a missed table
	// row at startup rather than silently rendering with a zero fill.
	PartyStatusCount
)

// PartyStatus picks the single highest-priority active status for a
// member. Priority ordering is what surfaces to the player when
// multiple statuses stack — Down beats everything (the rest don't
// matter when the member is at 0 HP), Ingested beats the rest (it's
// the most disruptive — the member can't act and can't be hit by
// friends or foes), then the disabling DoT/lockout statuses
// (Bound/Confused/Stunned/Asleep) in descending "how much it hurts
// the player's plan" order, then Poisoned (DoT but actionable),
// then Defending (lowest priority — it's a positive status the
// player chose).
//
// Returns PartyStatusNone if the member has no surfaced status.
// The `turns` return is the remaining counter for any counted
// status, or 0 for boolean statuses (Down / Ingested / Defending).
func PartyStatus(m PartyMember) (kind PartyStatusKind, turns int) {
	switch {
	case m.HP <= 0:
		return PartyStatusDown, 0
	case m.Ingested:
		return PartyStatusIngested, 0
	case m.WebbedTurns > 0:
		return PartyStatusWebbed, m.WebbedTurns
	case m.ConfusedTurns > 0:
		return PartyStatusConfused, m.ConfusedTurns
	case m.StunTurns > 0:
		return PartyStatusStunned, m.StunTurns
	case m.SleepTurns > 0:
		return PartyStatusAsleep, m.SleepTurns
	case m.PoisonTurns > 0:
		return PartyStatusPoisoned, m.PoisonTurns
	case m.Defending:
		return PartyStatusDefending, 0
	}
	return PartyStatusNone, 0
}

// PartyStatusLabel returns the short uppercase label rendered for a
// given status kind. Pair with PartyStatus(m) to drive a render
// surface — never branch on the kind enum yourself at the call
// site; that's how the two surfaces drifted before this helper.
func PartyStatusLabel(kind PartyStatusKind) string {
	switch kind {
	case PartyStatusDown:
		return "DOWN"
	case PartyStatusIngested:
		return "INGESTED"
	case PartyStatusWebbed:
		return "WEBBED"
	case PartyStatusConfused:
		return "CONFUSED"
	case PartyStatusStunned:
		return "STUNNED"
	case PartyStatusAsleep:
		return "ASLEEP"
	case PartyStatusPoisoned:
		return "POISONED"
	case PartyStatusDefending:
		return "DEFENDING"
	}
	return ""
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
	// Derive runs the level-up side effect a point in this stat triggers
	// on the WHOLE member (not just Stats) — e.g. VIT growing MaxHP, INT
	// growing the MP pool. Called by SpendStatPoint AFTER Add, so it reads
	// the already-incremented stat. nil for stats with no pool side effect.
	// Living in the row (rather than an if-chain in SpendStatPoint) means
	// the init length-assert covers it and a new derived stat can't be
	// silently forgotten.
	Derive func(*PartyMember)
}

var statTable = []statSpec{
	StatSTR: {Label: "STR", Get: func(s Stats) int { return s.STR }, Add: func(s *Stats) { s.STR++ }},
	StatDEX: {Label: "DEX", Get: func(s Stats) int { return s.DEX }, Add: func(s *Stats) { s.DEX++ }},
	StatINT: {Label: "INT", Get: func(s Stats) int { return s.INT }, Add: func(s *Stats) { s.INT++ },
		// INT feeds the MP pool: grow MaxMP and top off current MP by the
		// same delta so the point is immediately usable.
		Derive: func(m *PartyMember) { m.MaxMP += MPPerINT; GainUpTo(&m.MP, m.MaxMP, MPPerINT) }},
	StatWIS: {Label: "WIS", Get: func(s Stats) int { return s.WIS }, Add: func(s *Stats) { s.WIS++ }},
	StatVIT: {Label: "VIT", Get: func(s Stats) int { return s.VIT }, Add: func(s *Stats) { s.VIT++ },
		// VIT recomputes MaxHP authoritatively (= VIT·HPPerVIT) and heals
		// by the per-point delta so the level-up feels rewarding.
		Derive: func(m *PartyMember) { m.MaxHP = MaxHPFor(m.Stats); GainUpTo(&m.HP, m.MaxHP, HPPerVIT) }},
	StatSPD: {Label: "SPD", Get: func(s Stats) int { return s.SPD }, Add: func(s *Stats) { s.SPD++ }},
}

// statSetters is the absolute-write half of statTable. Kept separate
// (rather than fattening statSpec) because every existing reader uses
// only Get/Add and adding Set to statSpec would touch every entry
// without need.
var statSetters = []func(*Stats, int){
	StatSTR: func(s *Stats, v int) { s.STR = v },
	StatDEX: func(s *Stats, v int) { s.DEX = v },
	StatINT: func(s *Stats, v int) { s.INT = v },
	StatWIS: func(s *Stats, v int) { s.WIS = v },
	StatVIT: func(s *Stats, v int) { s.VIT = v },
	StatSPD: func(s *Stats, v int) { s.SPD = v },
}

// AdjustStat applies delta to the named stat, clamping at zero. Used
// by the custom-enemy editor's +/- buttons. Single seam so a future
// per-stat range cap or "you can't drop VIT below 1 or the HP math
// explodes" guard lands in one place.
func AdjustStat(s *Stats, st Stat, delta int) {
	if st < 0 || int(st) >= len(statTable) || s == nil {
		return
	}
	v := statTable[st].Get(*s) + delta
	if v < 0 {
		v = 0
	}
	statSetters[st](s, v)
}

// statDescriptions is the per-stat one-liner the level-up modal
// renders. Lives next to statTable so a new Stat enum value's
// description lands in the same place as its label / Get / Add.
var statDescriptions = []string{
	StatSTR: "Melee damage & hit chance",
	StatDEX: "Dodge, Crit, Ranged hit",
	StatINT: fmt.Sprintf("Magic damage & MP (+%d MP per point)", MPPerINT),
	StatWIS: "Heal, Magic defense, Status resist",
	StatVIT: fmt.Sprintf("Max HP (+%d per point)", HPPerVIT),
	StatSPD: "Turn frequency (act more often)",
}

// StatDescription returns the level-up modal blurb for the named
// stat. Out-of-range values return "" so a future enum addition
// missing a description doesn't panic the modal renderer.
func StatDescription(s Stat) string {
	if s < 0 || int(s) >= len(statDescriptions) {
		return ""
	}
	return statDescriptions[s]
}

// StatPreviewLine returns the per-row "what this spend buys" string
// the level-up modal shows IN PLACE of the static description when
// the player has staged at least one point in the row. Computes the
// projected post-spend derived values (damage, hit %, dodge %, crit
// %, heal, mdef, max HP, etc.) by applying `pending` clones of the
// stat's Add to a working copy. Returns "" when pending <= 0 or stat
// is out-of-range so the renderer can fall through to StatDescription.
func StatPreviewLine(stat Stat, current Stats, pending int) string {
	if pending <= 0 || stat < 0 || int(stat) >= len(statTable) {
		return ""
	}
	after := current
	for i := 0; i < pending; i++ {
		statTable[stat].Add(&after)
	}
	switch stat {
	case StatSTR:
		// STR drives both melee damage and melee hit chance.
		h0 := MeleeAccuracy(current, TimingQualityMiss) * 100
		h1 := MeleeAccuracy(after, TimingQualityMiss) * 100
		return fmt.Sprintf("Melee %d→%d  Hit %.0f→%.0f%%", MeleeDamage(current, 0), MeleeDamage(after, 0), h0, h1)
	case StatDEX:
		// DEX's active effects are dodge + crit (ranged hit is dormant
		// until a ranged attack ships, so it's left off the preview).
		d0 := DodgeChance(current) * 100
		d1 := DodgeChance(after) * 100
		c0 := CritChance(current, TimingQualityMiss) * 100
		c1 := CritChance(after, TimingQualityMiss) * 100
		return fmt.Sprintf("Dodge %.0f→%.0f%%  Crit %.0f→%.0f%%", d0, d1, c0, c1)
	case StatINT:
		// INT drives magic damage and the MP pool (MPPerINT per point).
		return fmt.Sprintf("Magic %d→%d  MaxMP +%d", MagicDamage(current, 0), MagicDamage(after, 0), (after.INT-current.INT)*MPPerINT)
	case StatWIS:
		return fmt.Sprintf("Heal %d→%d  MDef %d→%d", HealAmount(current, 0), HealAmount(after, 0), MagicDefense(current), MagicDefense(after))
	case StatVIT:
		return fmt.Sprintf("MaxHP %d → %d", MaxHPFor(current), MaxHPFor(after))
	case StatSPD:
		return fmt.Sprintf("SPD %d → %d (more turns)", current.SPD, after.SPD)
	default:
		// statTable's init assert guarantees a row per Stat; this switch
		// is the parallel one it doesn't cover. The package init() calls
		// StatPreviewLine for every Stat, so a missing case trips this
		// panic at STARTUP rather than when a player stages that stat.
		panic("core: StatPreviewLine missing case for stat")
	}
}

// CommitLevelUp applies the staged stat-point spend on a member by
// calling SpendStatPoint the right number of times. Returns true if
// at least one point landed so the caller can decide whether to
// advance to the next pending member. Skill points are NOT spent
// here — they accrue on the member at level-up time and are spent
// from the Skills panel's tree UI via BuySkillNode.
func CommitLevelUp(m *PartyMember, pendingStats [StatCount]int) bool {
	if m == nil {
		return false
	}
	any := false
	for i := 0; i < int(StatCount); i++ {
		for k := 0; k < pendingStats[i]; k++ {
			if !SpendStatPoint(m, Stat(i)) {
				break
			}
			any = true
		}
	}
	return any
}

func init() {
	if len(statTable) != int(StatCount) {
		panic("core: statTable length must match StatCount — add a row when adding a Stat enum value")
	}
	if len(statDescriptions) != int(StatCount) {
		panic("core: statDescriptions length must match StatCount — add a row when adding a Stat enum value")
	}
	if len(statSetters) != int(StatCount) {
		panic("core: statSetters length must match StatCount — add a row when adding a Stat enum value")
	}
	// StatPreviewLine's per-stat switch is the one parallel table the
	// length-asserts above can't cover (each case formats different
	// derived values, so it can't collapse into a []string). Force its
	// coverage here: calling it for every Stat with a staged point makes
	// a missing case panic at STARTUP (the switch's default) instead of
	// when a player happens to stage that stat mid-game.
	var probe Stats
	for i := Stat(0); i < StatCount; i++ {
		if StatPreviewLine(i, probe, 1) == "" {
			panic(fmt.Sprintf("core: StatPreviewLine returned empty for stat index %d — add a preview case", int(i)))
		}
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
	statTable[stat].Add(&m.Stats)
	// Run the stat's pool side effect (VIT→MaxHP, INT→MaxMP), if any —
	// table-driven so a new derived stat lands in its statTable row, not
	// a hardcoded branch here.
	if derive := statTable[stat].Derive; derive != nil {
		derive(m)
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
// Routes through rollDuration so it shares the one degenerate-bounds
// contract with the SkillEffect DoT rollers (Burn/Sleep/Stun/Bind/Confuse)
// — previously it open-coded the math with the OLD `span <= 0 → MinTurns`
// rule, which diverged from rollDuration's `min <= 0 || max < min → 0`.
func (p PoisonEffect) RollDuration(rng *rand.Rand) int {
	return rollDuration(rng, p.MinTurns, p.MaxTurns)
}

// TickPoisonStep applies one tick of poison damage to every poisoned,
// alive party member and decrements their counter. Called after each
// successful exploration step so a fight-inflicted poison doesn't stick
// indefinitely while the player walks around.
//
// There are three poison-tick code paths total — each fires in a
// distinct context, and the divergence is intentional:
//
//   - TickPoisonStep (this function) — out-of-battle, fires on every
//     player tile-step. No combat log; no battle context to write to.
//   - battle.tickPoisonAfterPartyTurn — in-battle, fires at the end
//     of a poisoned member's own turn (the "user still gets to act
//     before bleeding" beat). Emits combat-log lines.
//   - battle.tickPoisonForIngestedParty — in-battle, fires at the
//     start of each new round for poisoned members who are ingested
//     by a mantrap (i.e. skipped from the per-turn queue). Without
//     this third path, ingest would silently pause the poison DoT.
//
// All three drain PoisonTurns, deal PoisonTickDamage, and bypass
// armor (poison is magical decay). The HP-floor + damage-flash step is
// now shared via ApplyFlatDamage, so a "Poison ticks for VIT% instead
// of flat" change lands in one helper; what still legitimately differs
// per path is the surrounding context (combat-log lines, the wake rule,
// the round-vs-step cadence).
//
// Returns the number of members hit this step — callers can use it to
// emit a HUD nudge ("Poison stings!") without re-walking the party.
func TickPoisonStep(g *GameState) int {
	ticks := 0
	for i := range g.Party {
		m := &g.Party[i]
		if m.HP <= 0 || m.PoisonTurns <= 0 {
			continue
		}
		m.PoisonTurns--
		// Poison is magical decay — bypass armor (matches the in-battle
		// tick path which passes SkillTagMagic). Shares the HP-floor +
		// flash contract with the battle damage path via ApplyFlatDamage;
		// the wake rule below is poison-specific (only a lethal tick wakes
		// a sleeper out of battle, unlike the in-battle wake-on-any-hit).
		if ApplyFlatDamage(&m.HP, &m.DamageFlash, PoisonTickDamage) {
			m.SleepTurns = 0
		}
		ticks++
	}
	return ticks
}

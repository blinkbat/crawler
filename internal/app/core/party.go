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

// PartyClassCount is the number of PartyClass values (ClassWarrior..ClassWizard),
// so render can size a per-class array as [core.PartyClassCount] without the bare
// 4. Equals PartyMemberCount today (one member per class); init() asserts the two
// stay in lockstep.
const PartyClassCount = 4

type PartyClassDefinition struct {
	Class PartyClass
	Name  string
	Stats Stats
	MaxMP int
}

// PartyMemberCount is the fixed party size = number of class definitions.
// The slice order is the seating / tie-break contract (see partyClassDefinitions),
// and render formation + save format index by class slot, so the count is a
// real contract, not incidental. init() asserts len(partyClassDefinitions)
// matches — adding a class is an append plus bumping this, which then trips
// any downstream that assumed four until it's updated.
const PartyMemberCount = 4

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
	// NoUpgrades marks a player-castable skill that has NO tier-upgrade
	// ladder — a single-rank utility whose effect doesn't scale (Scan
	// reveals a foe's HP; there's nothing to improve numerically). It's
	// granted by an `actOnce` tree node (MaxRank 1, see skilltrees.go) and
	// is EXEMPT from skilltree.go's "every PlayerCastable skill has exactly
	// MaxSkillTier rows in skillTierTable" invariant: that guard skips a
	// NoUpgrades skill (and asserts it carries no stray tier rows). Every
	// other player skill leaves this false and keeps its standard 3-tier
	// damage/proc ladder.
	NoUpgrades bool
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
	// BleedChance / Min / Max gate the Bleed DoT (Warrior Rend / Thief
	// Lacerate) on an enemy target — same shape as Poison, but a SEPARATE
	// counter (Enemy.BleedTurns) so Bleed stacks alongside Poison. Zero chance
	// short-circuits the apply.
	BleedChance   float64
	BleedMinTurns int
	BleedMaxTurns int
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
	// BuffStats / BuffTurns declare a stat buff a skill grants its target(s)
	// for BuffTurns of their turns (Cleric's Bless is the first user). The
	// apply path stamps both onto the recipient's BuffStats / BuffTurns, where
	// EffectiveStats folds them in while the counter runs. Zero BuffTurns =
	// the skill grants no buff, so a non-buff skill that picks up the
	// SkillEffect by accident applies nothing. Pairs with SkillTagBuff +
	// AppliesAOEPartyBuff for the party-wide case.
	BuffStats Stats
	BuffTurns int
	// AppliesAOEPartyBuff flags a buff skill whose BuffStats/BuffTurns land on
	// EVERY living party member instead of a single ally — the buff-side
	// mirror of AppliesAOEParty (damage) and AppliesAOEEnemies. Bless sets it;
	// the apply handler loops the living party stamping the buff on each.
	AppliesAOEPartyBuff bool
	// RegenTurns declares a heal-over-time a skill grants its ally target
	// (Cleric's Renewal — the game's first HoT). The apply path stamps it onto
	// the recipient's RegenTurns and snapshots the per-turn heal (the skill's
	// quality/WIS-scaled Heal) onto RegenPerTurn; the regen then ticks at the
	// END of the member's own turn (like Poison, inverted) until it runs out.
	// Zero = no HoT. The positive-status mirror of the Burn/Poison min/max
	// fields; fixed duration (not rolled) like BuffTurns.
	RegenTurns int
	// ArmorReduction declares how much a skill strips from the target enemy's
	// per-instance Armor (the Thief's Corrosive Vial). Applied directly to
	// Enemy.Armor (floored at 0) for the rest of the battle — distinct from the
	// turn-counted BuffStats debuff: a permanent armor break, not a status, that
	// the damageEnemy mitigation chain reads immediately. Zero = no strip.
	ArmorReduction int
	// ATBPush is how much a skill shoves the target enemy's ATB readiness gauge
	// backwards on a landed hit (the Warrior's Sunder), delaying its next turn.
	// A one-shot subtraction from g.Battle.Readiness, distinct from the SPD
	// debuff — it doesn't persist, it just resets the clock once. Zero = no push.
	ATBPush int
	// BuffArmor / BuffMDef ride the party buff bundle alongside BuffStats: a buff
	// skill (the Warrior's Stone Skin) can grant flat Armor / MDef for BuffTurns,
	// folded by EffectiveArmor / EffectiveMDef while the shared counter runs.
	// Zero = the buff grants no defensive bonus (Bless leaves these 0). Stamped
	// together with BuffStats / BuffTurns via StampPartyBuff.
	BuffArmor int
	BuffMDef  int
	// ShieldHP declares a damage-absorbing shield a skill grants its ally target
	// (the Cleric's Aegis). Stamped onto PartyMember.ShieldHP, which the party
	// damage path spends before HP until depleted or the battle ends. Zero = no
	// shield. Not turn-counted — it lasts until the absorb pool is used up.
	ShieldHP int
	// IceArmorTurns declares the duration of a reactive frost ward a skill grants
	// its caster (the Wizard's Ice Armor): while PartyMember.IceArmorTurns runs,
	// the caster gains MDef and chills any enemy that lands a basic attack on
	// them. Zero = the skill grants no ward. Fixed duration (not rolled), ticked
	// at the caster's end-of-turn like the other buffs.
	IceArmorTurns int
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
	{Class: ClassWarrior, Name: "Warrior", Stats: Stats{STR: 6, DEX: 2, INT: 1, WIS: 1, VIT: 5, SPD: 3}, MaxMP: 4},
	{Class: ClassCleric, Name: "Cleric", Stats: Stats{STR: 2, DEX: 2, INT: 2, WIS: 6, VIT: 4, SPD: 4}, MaxMP: 9},
	{Class: ClassThief, Name: "Thief", Stats: Stats{STR: 3, DEX: 6, INT: 2, WIS: 1, VIT: 4, SPD: 6}, MaxMP: 5},
	{Class: ClassWizard, Name: "Wizard", Stats: Stats{STR: 1, DEX: 2, INT: 6, WIS: 2, VIT: 4, SPD: 4}, MaxMP: 10},
}

// partyClassByID is the O(1) lookup for partyClassDefinitions. Built once
// at init for the same reason as skillByID — partyClassInfo is called per
// frame from selectors and per-party-member render code.
var partyClassByID = BuildRegistry(partyClassDefinitions, func(d PartyClassDefinition) PartyClass { return d.Class })

// Skill registry. Effect.Damage / Effect.Heal are flat baselines that the
// stat-scaled formulas add on top (see types.go's MeleeDamage etc.). Tuned
// so that a focused class (top stat 6 post-difficulty-pass) lands a sensible
// total: e.g. Wizard (INT 6) Firebolt = 6 + 1 base = 7 pre-quality, scaling
// further with timing. Difficulty pass: bases trimmed
// and Firebolt's burn-chance pulled down so a single Excellent doesn't
// auto-burn every cast.
var skillDefinitions = []skillDefinition{
	{Skill: SkillSwipe, Name: "Swipe", Description: "STR-scaled cleave through every living enemy in the pack.", Cost: 2, TargetMode: ActionMenu, Kind: SkillKindMelee, Tag: SkillTagPhys, Minigame: MinigamePress, Effect: SkillEffect{Damage: 0, AppliesAOEEnemies: true}, PlayerCastable: true},
	{Skill: SkillPrayer, Name: "Prayer", Description: "WIS-scaled single-ally heal. Charge bar — release at peak.", Cost: 4, TargetMode: ActionPartyTarget, Kind: SkillKindHeal, Tag: SkillTagHeal, Minigame: MinigameCharge, Effect: SkillEffect{Heal: 1}, PlayerCastable: true},
	{Skill: SkillSteal, Name: "Steal", Description: "Pickpocket the target. Stop the reels — matches drive the chance.", Cost: 0, TargetMode: ActionEnemyTarget, Kind: SkillKindUtility, Tag: SkillTagNone, Minigame: MinigameReels, Effect: SkillEffect{StealChance: StealBaseChance}, PlayerCastable: true},
	{Skill: SkillFirebolt, Name: "Firebolt", Description: "INT-scaled magic damage. Charge; release past the peak to Overcharge (bonus damage, burns you). Chance to inflict Burn.", Cost: 5, TargetMode: ActionEnemyTarget, Kind: SkillKindMagic, Tag: SkillTagMagic, Minigame: MinigameOvercharge, Effect: SkillEffect{Damage: 1, BurnChance: FireboltBurnChance, BurnMinTurns: FireBurnMinTurns, BurnMaxTurns: FireBurnMaxTurns}, PlayerCastable: true, EnemyCastable: true},
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
	// Cripple (Thief): single-target SPD debuff, the first enemy-side debuff.
	// No damage — like Bless the timing grade is cosmetic and the effect always
	// lands. Stamps a negative SPD onto the target's BuffStats/BuffTurns (the
	// enemy-side mirror of the party buff fields), folded by EffectiveEnemyStats
	// so the foe's ATB turn-rate drops. SkillTagNone (no mitigation interaction,
	// like Steal/Scan).
	{Skill: SkillCripple, Name: "Cripple", Description: "Sap an enemy's SPD for a few turns, slowing how often it acts. No damage.", Cost: 3, TargetMode: ActionEnemyTarget, Kind: SkillKindUtility, Tag: SkillTagNone, Minigame: MinigamePress, Effect: SkillEffect{BuffStats: Stats{SPD: -CrippleSPDReduction}, BuffTurns: CrippleTurns}, PlayerCastable: true},
	// Corrosive Vial (Thief): single-target armor break. No damage — strips the
	// target's per-instance Armor (floored at 0) for the rest of the battle so
	// phys hits land harder. A permanent break, not a turn-counted status (it
	// mutates Enemy.Armor directly). SkillTagNone, like Cripple / Steal.
	{Skill: SkillCorrosiveVial, Name: "Corrosive Vial", Description: "Hurl acid that eats an enemy's Armor for the rest of the fight, so every hit lands harder. No damage.", Cost: 3, TargetMode: ActionEnemyTarget, Kind: SkillKindUtility, Tag: SkillTagNone, Minigame: MinigamePress, Effect: SkillEffect{ArmorReduction: CorrosiveArmorReduction}, PlayerCastable: true},
	// Frost Lance (Wizard): charge-up single-target magic. +2 base +
	// INT, 5 MP. On Great/Excellent timing applies a 1-turn Stun —
	// lower base damage than Firebolt but reliable lockout instead
	// of the burn-chance lottery. Different tactical role.
	{Skill: SkillFrostLance, Name: "Frost Lance", Description: "INT-scaled magic damage. Reliable Stun on Great+.", Cost: 5, TargetMode: ActionEnemyTarget, Kind: SkillKindMagic, Tag: SkillTagMagic, Minigame: MinigameCharge, Effect: SkillEffect{Damage: 2, StunChance: FrostLanceStunChance, StunMinTurns: FrostLanceStunTurns, StunMaxTurns: FrostLanceStunTurns}, PlayerCastable: true},
	// Frostbite (Wizard): charge magic that always chills. INT-scaled damage
	// plus a guaranteed SPD debuff (the enemy BuffStats mirror) on a surviving
	// target — the damaging counterpart to Cripple's pure-utility slow. The
	// chill always lands (no proc roll), so timing only scales the damage.
	{Skill: SkillFrostbite, Name: "Frostbite", Description: "INT-scaled frost magic that always chills — lowers the target's SPD for a few turns.", Cost: 4, TargetMode: ActionEnemyTarget, Kind: SkillKindMagic, Tag: SkillTagMagic, Minigame: MinigameCharge, Effect: SkillEffect{Damage: FrostbiteDamageBase, BuffStats: Stats{SPD: -FrostbiteSPDReduction}, BuffTurns: FrostbiteChillTurns}, PlayerCastable: true},
	// Cone of Cold (Wizard): AoE chill — INT-scaled frost damage to every living
	// enemy plus a guaranteed per-target SPD chill. The pack-wide Frostbite,
	// routed through applyAoEStatusSkill (AppliesAOEEnemies). Lower per-target
	// damage / shorter chill than the single bolt.
	{Skill: SkillConeOfCold, Name: "Cone of Cold", Description: "INT-scaled frost across the whole pack. Charge — chills every enemy, lowering their SPD.", Cost: 7, TargetMode: ActionMenu, Kind: SkillKindMagic, Tag: SkillTagMagic, Minigame: MinigameCharge, Effect: SkillEffect{Damage: ConeOfColdDamageBase, AppliesAOEEnemies: true, BuffStats: Stats{SPD: -ConeOfColdSPDReduction}, BuffTurns: ConeOfColdChillTurns}, PlayerCastable: true},
	// Sunder (Warrior, Battle Sense tree): STR-scaled phys hit that also shoves
	// the target's ATB readiness back (ATBPush) so its next turn lands later — a
	// damaging tempo swing, the offensive counterpart to the Thief's Cripple.
	// Charge minigame; the push lands whenever the hit connects.
	{Skill: SkillSunder, Name: "Sunder", Description: "STR-scaled phys hit that shoves the target's turn later. Charge.", Cost: 4, TargetMode: ActionEnemyTarget, Kind: SkillKindMelee, Tag: SkillTagPhys, Minigame: MinigameCharge, Effect: SkillEffect{Damage: SunderDamageBase, ATBPush: SunderATBPush}, PlayerCastable: true},
	// Taunt (Warrior, Battle Sense tree): forces the target enemy to attack the
	// casting Warrior on its next turn (no damage). Single-rank utility, like Scan
	// / Cleanse — NoUpgrades. Press minigame; the pull always lands.
	{Skill: SkillTaunt, Name: "Taunt", Description: "Force the target enemy to attack you next turn. No damage.", Cost: 2, TargetMode: ActionEnemyTarget, Kind: SkillKindUtility, Tag: SkillTagNone, Minigame: MinigamePress, Effect: SkillEffect{}, PlayerCastable: true, NoUpgrades: true},
	// War Banner (Warrior, Ancestral Call tree): party-wide STR + Armor rally —
	// the martial mirror of Bless, using the shared party buff bundle. No target
	// (ActionMenu); Buff tag; press minigame, grade cosmetic. Armor (not VIT) is
	// the defensive half — a VIT buff would be inert since MaxHP isn't re-derived.
	{Skill: SkillWarBanner, Name: "War Banner", Description: "Plant a banner — raises the whole party's STR and Armor for several turns. No damage.", Cost: 5, TargetMode: ActionMenu, Kind: SkillKindUtility, Tag: SkillTagBuff, Minigame: MinigamePress, Effect: SkillEffect{BuffStats: Stats{STR: WarBannerPerStat}, BuffArmor: WarBannerArmor, BuffTurns: WarBannerTurns, AppliesAOEPartyBuff: true}, PlayerCastable: true},
	// Stone Skin (Warrior, Ancestral Call tree): single-ally Armor + MDef ward via
	// the party buff bundle's BuffArmor / BuffMDef fields. No damage; ally target;
	// press minigame, grade cosmetic.
	{Skill: SkillStoneSkin, Name: "Stone Skin", Description: "Ward an ally with temporary Armor and MDef. No damage.", Cost: 4, TargetMode: ActionPartyTarget, Kind: SkillKindUtility, Tag: SkillTagBuff, Minigame: MinigamePress, Effect: SkillEffect{BuffArmor: StoneSkinArmor, BuffMDef: StoneSkinMDef, BuffTurns: StoneSkinTurns}, PlayerCastable: true},
	// Blind (Cleric, Radiance tree): saps the target enemy's DEX (the accuracy
	// stat) so it whiffs more — the DEX-flavored sibling of Cripple, via the enemy
	// BuffStats debuff mirror. No damage; press minigame, grade cosmetic.
	{Skill: SkillBlind, Name: "Blind", Description: "Sear an enemy's eyes — lowers its accuracy for several turns. No damage.", Cost: 3, TargetMode: ActionEnemyTarget, Kind: SkillKindUtility, Tag: SkillTagNone, Minigame: MinigamePress, Effect: SkillEffect{BuffStats: Stats{DEX: -BlindDEXReduction}, BuffTurns: BlindTurns}, PlayerCastable: true},
	// Aegis (Cleric, Conviction tree): grants an ally a damage-absorbing shield
	// (ShieldHP) that soaks incoming hits before HP until depleted. No damage;
	// ally target; press minigame, grade cosmetic.
	{Skill: SkillAegis, Name: "Aegis", Description: "Shield an ally — absorbs incoming damage before it reaches their HP. No damage.", Cost: 4, TargetMode: ActionPartyTarget, Kind: SkillKindUtility, Tag: SkillTagBuff, Minigame: MinigamePress, Effect: SkillEffect{ShieldHP: AegisShieldBase}, PlayerCastable: true},
	// Smoke Bomb (Thief, Shadow Arts tree): one DEX magnitude buffs the whole
	// party's evasion AND saps every enemy's accuracy. No target (ActionMenu); the
	// party-buff side rides BuffStats/BuffTurns, the enemy-debuff side mirrors it
	// in the handler. Press minigame, grade cosmetic.
	{Skill: SkillSmokeBomb, Name: "Smoke Bomb", Description: "Drop a smoke screen — the party gains evasion while every enemy loses accuracy. No damage.", Cost: 4, TargetMode: ActionMenu, Kind: SkillKindUtility, Tag: SkillTagNone, Minigame: MinigamePress, Effect: SkillEffect{BuffStats: Stats{DEX: SmokeBombDEX}, BuffTurns: SmokeBombTurns, AppliesAOEPartyBuff: true}, PlayerCastable: true},
	// Ice Armor (Wizard, Cryomancy tree): self-buff — while it stands the caster
	// gains MDef and chills any enemy that lands a basic attack on them. No target
	// (ActionMenu, self); Buff tag; charge minigame to give the ward some weight.
	{Skill: SkillIceArmor, Name: "Ice Armor", Description: "Sheathe yourself in frost — gain MDef and chill any foe that strikes you. Charge.", Cost: 5, TargetMode: ActionMenu, Kind: SkillKindUtility, Tag: SkillTagBuff, Minigame: MinigameCharge, Effect: SkillEffect{IceArmorTurns: IceArmorTurnsBase}, PlayerCastable: true},
	// Rend (Warrior, Fury tree): STR-scaled phys hit that applies Bleed — the
	// game's third DoT, on its own Enemy.BleedTurns counter so it stacks with
	// Poison. Charge minigame (a wound-up cleaving cut); the bleed is a
	// quality-scaled proc like Venom Strike's Poison.
	{Skill: SkillRend, Name: "Rend", Description: "STR-scaled phys hit that opens a Bleed wound — damage over time. Charge.", Cost: 4, TargetMode: ActionEnemyTarget, Kind: SkillKindMelee, Tag: SkillTagPhys, Minigame: MinigameCharge, Effect: SkillEffect{Damage: RendDamageBase, BleedChance: RendBleedChance, BleedMinTurns: BleedMinTurns, BleedMaxTurns: BleedMaxTurns}, PlayerCastable: true},
	// Lacerate (Thief, Venomancy tree): the same Bleed DoT as Rend, lighter base
	// hit — its draw is stacking the bleed alongside the tree's Poison (separate
	// counters). Sequence minigame, matching the Thief's Venom Strike.
	{Skill: SkillLacerate, Name: "Lacerate", Description: "STR-scaled phys cut that opens a Bleed — stacks alongside Poison. Sequence.", Cost: 4, TargetMode: ActionEnemyTarget, Kind: SkillKindMelee, Tag: SkillTagPhys, Minigame: MinigameSequence, Effect: SkillEffect{Damage: LacerateDamageBase, BleedChance: LacerateBleedChance, BleedMinTurns: BleedMinTurns, BleedMaxTurns: BleedMaxTurns}, PlayerCastable: true},
	// Arc Bolt (Wizard): sequence-minigame AoE magic. +1 base + INT
	// per target, 6 MP. Each correct tap in the sequence reads as a
	// new bolt arcing to the next enemy; on apply, all living enemies
	// take quality-scaled damage. Magic-tagged so amoebas don't
	// shrug it off. Pricier than Firebolt because it hits everyone.
	{Skill: SkillArcBolt, Name: "Arc Bolt", Description: "INT-scaled magic AoE. Memorize the glyph pattern, then recall it — arcs to every enemy.", Cost: 6, TargetMode: ActionMenu, Kind: SkillKindMagic, Tag: SkillTagMagic, Minigame: MinigameRecall, Effect: SkillEffect{Damage: 1, AppliesAOEEnemies: true}, PlayerCastable: true},
	// Scan (Thief, via the Shadow Arts tree): single-target inspect that
	// deals no damage and applies no status. A landed cast IDENTIFIES the
	// target's KIND in the bestiary (Bestiary.MarkScanned) — the shortcut
	// to the normal 5-kills-to-identify threshold — after which that
	// kind's exact HP shows in the battle roster. Costs 2 MP so it's a
	// real resource decision, not a free pre-cast every fight; a
	// simple Press bar keeps it in the standard action flow, but the ID
	// lands at any timing grade (it's information, not a chance roll).
	// Tag None — never touches armor / resist math.
	{Skill: SkillScan, Name: "Scan", Description: "Identify the target's kind — reveals its HP (here and in the bestiary). No damage.", Cost: 2, TargetMode: ActionEnemyTarget, Kind: SkillKindUtility, Tag: SkillTagNone, Minigame: MinigamePress, Effect: SkillEffect{}, PlayerCastable: true, NoUpgrades: true},
	// Bless (Cleric, Conviction tree): party-wide stat buff. TargetMode
	// ActionMenu (no target pick — it always hits the whole party, like Mass
	// Mend / Whirlwind), Utility kind (the buff doesn't stat-scale), Buff tag
	// (the first SkillTagBuff user; bypasses armor/MDef, though it deals no
	// damage anyway). Press minigame keeps it in the standard flow; the grade
	// is cosmetic — the buff always lands. AppliesAOEPartyBuff drives the
	// loop-the-party apply. Tier upgrades (skilltree.go) stack magnitude +
	// duration onto the base BuffStats / BuffTurns below.
	{Skill: SkillBless, Name: "Bless", Description: "Bless the whole party — raises STR, DEX, INT and WIS for a few turns. No damage.", Cost: 4, TargetMode: ActionMenu, Kind: SkillKindUtility, Tag: SkillTagBuff, Minigame: MinigamePress, Effect: SkillEffect{BuffStats: Stats{STR: BlessBuffPerStat, DEX: BlessBuffPerStat, INT: BlessBuffPerStat, WIS: BlessBuffPerStat}, BuffTurns: BlessBuffTurns, AppliesAOEPartyBuff: true}, PlayerCastable: true},
	// Fireball (Wizard, Pyromancy tree): the AoE counterpart to Firebolt.
	// INT-scaled magic damage to every living enemy (AppliesAOEEnemies) plus a
	// per-target Burn roll. Charge minigame. Pricier than Arc Bolt (6) because
	// it also burns. Applied via applyAoEStatusSkill in battle/actions.go.
	{Skill: SkillFireball, Name: "Fireball", Description: "INT-scaled magic fire across the whole pack. Charge — per-target Burn chance.", Cost: 7, TargetMode: ActionMenu, Kind: SkillKindMagic, Tag: SkillTagMagic, Minigame: MinigameCharge, Effect: SkillEffect{Damage: 1, AppliesAOEEnemies: true, BurnChance: FireballBurnChance, BurnMinTurns: FireBurnMinTurns, BurnMaxTurns: FireBurnMaxTurns}, PlayerCastable: true},
	// Poison Cloud (Thief, Venomancy tree): the AoE counterpart to Venom Strike.
	// Light STR-scaled damage to every living enemy plus a per-target Poison
	// roll. Phys-tagged + Melee-kind to mirror Venom Strike (the direct damage
	// is minor — the whole-pack DoT is the point; poison ticks bypass armor
	// regardless). Sequence minigame, same rhythm identity as Venom Strike.
	{Skill: SkillPoisonCloud, Name: "Poison Cloud", Description: "STR-scaled toxin across the whole pack. Sequence — per-target Poison chance.", Cost: 6, TargetMode: ActionMenu, Kind: SkillKindMelee, Tag: SkillTagPhys, Minigame: MinigameSequence, Effect: SkillEffect{Damage: 1, AppliesAOEEnemies: true, PoisonChance: PoisonCloudPoisonChance, PoisonMinTurns: PoisonMinTurns, PoisonMaxTurns: PoisonMaxTurns}, PlayerCastable: true},
	// Cleanse (Cleric, Mercy tree): single-ally status cure — no damage, no
	// scaling. Clears the curable combat debuffs via core.CureDebuffs (leaves
	// the Bless buff + Defending intact). NoUpgrades: a cure has no damage/proc
	// ladder to climb, same as Scan. Press minigame; the cure always lands
	// (grade cosmetic).
	{Skill: SkillCleanse, Name: "Cleanse", Description: "Cure an ally's Poison, Sleep, Stun, Web and Confusion. No damage.", Cost: 3, TargetMode: ActionPartyTarget, Kind: SkillKindUtility, Tag: SkillTagNone, Minigame: MinigamePress, Effect: SkillEffect{}, PlayerCastable: true, NoUpgrades: true},
	// Second Wind (Warrior, Ancestral Call tree): a flat self-heal. ActionMenu
	// (no target — heals the caster), Utility kind so the Warrior's low WIS
	// doesn't gate it (the base is flat; quality still scales the cast). Tag
	// None (battle-only; a heal has no mitigation/armor interaction). Charge
	// minigame to give the breather some weight. Tier ladder adds +heal.
	{Skill: SkillSecondWind, Name: "Second Wind", Description: "Catch your breath — a flat self-heal. Charge.", Cost: 3, TargetMode: ActionMenu, Kind: SkillKindUtility, Tag: SkillTagNone, Minigame: MinigameCharge, Effect: SkillEffect{Heal: SecondWindHealBase}, PlayerCastable: true},
	// Renewal (Cleric, Mercy tree): heal-over-time on one ally — the game's
	// first HoT. Heal kind so the per-turn amount snapshots the caster's
	// WIS-scaled value at cast; Tag None (battle-only — regen ticks need turns,
	// so it's not an out-of-battle heal). Effect.Heal is the base per-turn
	// amount, RegenTurns the base duration; the apply stamps them onto the
	// target's RegenPerTurn / RegenTurns. Tier ladder adds +turns / +per-turn.
	{Skill: SkillRenewal, Name: "Renewal", Description: "Heal-over-time on an ally — restores HP at the end of their turns. Charge.", Cost: 4, TargetMode: ActionPartyTarget, Kind: SkillKindHeal, Tag: SkillTagNone, Minigame: MinigameCharge, Effect: SkillEffect{Heal: RenewalRegenBase, RegenTurns: RenewalRegenTurns}, PlayerCastable: true},
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
	{Skill: SkillWeb, Name: "Web", Cost: 0, TargetMode: ActionPartyTarget, Kind: SkillKindUtility, Tag: SkillTagMagic, Minigame: MinigamePress, Effect: SkillEffect{BindChance: WebBindChance, BindMinTurns: SpiderWebbedMinTurns, BindMaxTurns: SpiderWebbedMaxTurns}, EnemyCastable: true},
	// Confuse (Will-o'-Wisp): enemy-only, Magic-tagged, single party
	// target. Applies the Confused status — per-action retarget roll
	// when the afflicted member acts. WIS resists on the apply roll
	// so a high-WIS Cleric is sturdier against it; duration is fixed
	// in ConfuseMin/Max.
	{Skill: SkillConfuse, Name: "Confuse", Cost: 0, TargetMode: ActionPartyTarget, Kind: SkillKindUtility, Tag: SkillTagMagic, Minigame: MinigamePress, Effect: SkillEffect{ConfuseChance: WispConfuseApplyChance, ConfuseMinTurns: WispConfuseMinTurns, ConfuseMaxTurns: WispConfuseMaxTurns}, EnemyCastable: true},
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

// SkillHasNoUpgrades reports whether a skill opts out of the tier-upgrade
// ladder — a single-rank utility (Scan) whose effect doesn't scale. The
// skilltree.go tier-table invariant skips these, and they're granted by an
// actOnce tree node (MaxRank 1). Unknown skill IDs return false.
func SkillHasNoUpgrades(s SkillID) bool {
	def, ok := skillInfo(s)
	return ok && def.NoUpgrades
}

// PlayerCastableSkills returns every skill flagged PlayerCastable, in
// registry declaration order. Used by battle's init() to assert each
// one has a handler registered, and reusable for any future "what
// skills can I learn?" UI that needs the canonical list.
func PlayerCastableSkills() []SkillID {
	return PlayerCastableSkillsInto(make([]SkillID, 0, len(skillDefinitions)))
}

// PlayerCastableSkillsInto is the buffer-reusing form of PlayerCastableSkills
// (re-sliced to length 0) for the per-frame caller: the battle skill menu's
// debug "all skills" mode rebuilds this list every frame the submenu is open.
// Pass nil to allocate. The set is static (registry-derived), so order is the
// skillDefinitions order.
func PlayerCastableSkillsInto(buf []SkillID) []SkillID {
	return skillIDsWhereInto(buf, func(d skillDefinition) bool { return d.PlayerCastable })
}

// skillIDsWhereInto appends every SkillID whose registry entry satisfies pred
// (nil pred = all) to buf re-sliced to length 0, in declaration order — the
// shared shape behind PlayerCastableSkillsInto / AllSkillIDs / EnemyCastableSkills.
// Pass nil buf to allocate; per-frame callers pass a scratch slice to reuse.
func skillIDsWhereInto(buf []SkillID, pred func(skillDefinition) bool) []SkillID {
	buf = buf[:0]
	for _, def := range skillDefinitions {
		if pred == nil || pred(def) {
			buf = append(buf, def.Skill)
		}
	}
	return buf
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

// AllPartyClasses returns the bare PartyClass keys in canonical definition
// order (Warrior, Cleric, Thief, Wizard), derived from partyClassDefinitions
// rather than hand-listed so a new class only has to be appended in one place.
// The single home for "iterate every class" loops (passives / skill-tree
// init invariants, etc.).
func AllPartyClasses() []PartyClass {
	out := make([]PartyClass, len(partyClassDefinitions))
	for i, d := range partyClassDefinitions {
		out[i] = d.Class
	}
	return out
}

func partyClassInfo(class PartyClass) (PartyClassDefinition, bool) {
	def, ok := partyClassByID[class]
	return def, ok
}

// PartyClassName returns the display name for a party class ("Warrior"),
// falling back to the class slug if the class is somehow unregistered. Mirrors
// EnemyInfo(kind).Name on the foe side; used by the Party Visualizer for the
// header label and save-flash text.
func PartyClassName(class PartyClass) string {
	if def, ok := partyClassInfo(class); ok {
		return def.Name
	}
	return PartyClassSlug(class)
}

// PartySkill returns the skill the action menu's Skill row currently
// casts for this member: the entry at member.SkillCursor within the
// member's LEARNED skills (see LearnedSkills / PartySkills). The
// in-battle Tab key cycles that index. Out-of-range cursors — a
// corrupted save, or a skill un-learned since the cursor was last set —
// clamp to 0; a member who has learned nothing yet returns SkillNone.
func PartySkill(member *PartyMember) SkillID {
	skills := PartySkills(member)
	if len(skills) == 0 {
		return SkillNone
	}
	idx := member.SkillCursor
	if idx < 0 || idx >= len(skills) {
		idx = 0
	}
	return skills[idx]
}

// PartySkills returns the member's learned castable skills — the set
// they have invested at least one rank into through the skill trees, in
// a stable tree/node order (see LearnedSkills). Used by the panels
// overlay's Skills tab, the battle skill submenu, and the cycle logic.
// A member who has learned nothing yet returns an empty slice; every
// caller already handles len == 0. Replaces the old fixed class loadout:
// progression now flows entirely through the Tome's tree purchases.
func PartySkills(member *PartyMember) []SkillID {
	return LearnedSkills(member)
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

func CanAffordSkill(m *PartyMember, skill SkillID) bool {
	return m.MP >= SkillCost(skill)
}

// SpendSkillMP checks affordability and, if the member can pay, deducts the
// skill's MP cost — returning whether the cast may proceed. The single seam for
// "pay for a skill": battle's chargeMP wraps it (adding the battle-status
// message on failure) and the out-of-battle cast path calls it directly, so a
// future "refund on cancel" / "VIT raises the MP pool" rule is one edit, not
// several inlined `MP -= SkillCost` sites.
func SpendSkillMP(m *PartyMember, skill SkillID) bool {
	if m == nil || !CanAffordSkill(m, skill) {
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
// Allocates a small slice; per-frame callers (the chooser's update + draw
// loops) use OutOfBattleHealsInto with a reusable buffer instead.
func OutOfBattleHeals(m *PartyMember) []SkillID {
	return OutOfBattleHealsInto(nil, m)
}

// OutOfBattleHealsInto is OutOfBattleHeals into a caller-owned buffer
// (re-sliced to length 0) — the allocation-free variant for the heal
// chooser's per-frame update/draw paths. The returned slice aliases buf's
// backing array and is valid until the caller's next reuse of it.
func OutOfBattleHealsInto(buf []SkillID, m *PartyMember) []SkillID {
	return filterInto(buf, PartySkills(m), func(s SkillID) bool {
		return s != SkillNone && SkillHealableOutOfBattle(s)
	})
}

// HealMember restores up to `amount` HP to a LIVING, non-ingested member,
// clamped at MaxHP. It never revives (a downed member at HP <= 0 is
// untouched — reviving is not a heal), ignores non-positive amounts, and
// skips a member ingested by a mantrap (out of reach, untargetable —
// matching applyMassMend's in-battle skip). Item / skill use routes
// through this so the clamp + no-revive + ingest rules live in one place.
// Returns true when the member was a valid heal target (guards passed) so
// callers can gate a flash / sound / "it worked" message on the heal landing
// rather than re-checking the same HP / ingest rules; party-wide callers that
// don't care simply ignore the result.
func HealMember(m *PartyMember, amount int) bool {
	if m == nil || amount <= 0 || !partyAvailable(*m) {
		return false
	}
	GainUpTo(&m.HP, m.MaxHP, amount)
	return true
}

// HealWholeParty applies HealMember(amount) to every LIVING (HP > 0) party
// member. The shared party-wide heal loop behind both the in-battle and the
// out-of-battle Mass Mend, so the "who does a party heal touch?" rule lives
// in one place. HealMember's own guards (no revive, clamp at MaxHP, ingest
// skip) still apply per member; the HP > 0 check here keeps the loop from
// even calling HealMember on the dead.
func HealWholeParty(g *GameState, amount int) {
	if g == nil || amount <= 0 {
		return
	}
	for i := range g.Party {
		if g.Party[i].HP > 0 {
			HealMember(&g.Party[i], amount)
		}
	}
}

// RestorePartyFully tops every LIVING member to full HP AND full MP — the
// healing crystal's effect (and a clean home for any future "full rest" source).
// Per-member MaxHP/MaxMP are the caps; HealMember / RestoreMP clamp and skip the
// dead / ingested. Returns the number of members restored.
func RestorePartyFully(g *GameState) int {
	if g == nil {
		return 0
	}
	restored := 0
	for i := range g.Party {
		m := &g.Party[i]
		if !partyAvailable(*m) {
			continue
		}
		HealMember(m, m.MaxHP)
		RestoreMP(m, m.MaxMP)
		restored++
	}
	return restored
}

// TickCrystalRecharge advances every DORMANT crystal's charge by one step,
// re-arming it (Charged=true) once it reaches CrystalRechargeSteps. Charged
// crystals are left alone. Call once per landed exploration step, alongside the
// poison / weather / day-cycle ticks.
func TickCrystalRecharge(g *GameState) {
	if g == nil {
		return
	}
	for i := range g.Crystals {
		c := &g.Crystals[i]
		if c.Charged {
			continue
		}
		if c.Charge++; c.Charge >= CrystalRechargeSteps {
			c.Charge = CrystalRechargeSteps
			c.Charged = true
		}
	}
}

// clearMemberAnimTimers zeros a member's transient ANIMATION timers (lunge /
// damage-flash / knockback) — not gameplay state. Shared by the field-recovery
// reset and the save sanitizer so "what's an animation timer" lives in one
// place (statuses go through ClearPartyTransientStatuses instead).
func clearMemberAnimTimers(m *PartyMember) {
	m.AttackBump = 0
	m.DamageFlash = 0
	m.HitKnockback = 0
}

// RestoreMP tops up a member's MP by amount, clamped at MaxMP, and returns the
// actual amount restored (0 if already full / not eligible). Mirrors
// HealMember on the MP axis: a downed (HP<=0) or ingested member can't drink.
// Used by the Magic Phial's use paths.
func RestoreMP(m *PartyMember, amount int) int {
	if m == nil || amount <= 0 || !partyAvailable(*m) {
		return 0
	}
	before := m.MP
	GainUpTo(&m.MP, m.MaxMP, amount)
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
//
// Degenerate-bounds policy: RETURN 0 on min <= 0 || max < min (fail open to
// "no status effect"). This intentionally differs from its two siblings —
// RandRangeI (util.go) returns lo on hi <= lo, and rollGold (economy.go)
// swaps inverted bounds then clamps to >= 0.
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

// BleedDuration rolls a uniform bleed duration in [Min, Max] — mirrors
// PoisonDuration; degenerate bounds return 0 so a non-bleed skill that picks up
// the effect by accident bleeds no one. Used by the Rend / Lacerate apply.
func (effect SkillEffect) BleedDuration(rng *rand.Rand) int {
	return rollDuration(rng, effect.BleedMinTurns, effect.BleedMaxTurns)
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

// EnemyHitChance is the [0,1] probability an enemy's BASIC physical attack
// connects, rolled BEFORE the defend bar so a whiff can skip the input minigame
// entirely (nothing to defend). DEX-driven (read through EffectiveEnemyStats, so
// a future accuracy debuff like Blind lowers it) and clamped to
// [EnemyAccuracyFloor, EnemyAccuracyCap] — even a sharp foe can whiff and a
// debuffed one still lands sometimes. Enemy SKILLS are NOT accuracy-gated,
// mirroring the player side where only basic attacks roll MeleeAccuracy.
func EnemyHitChance(s Stats) float64 {
	return Clamp(EnemyAccuracyBaseline+EnemyAccuracyPerDEX*float64(s.DEX), EnemyAccuracyFloor, EnemyAccuracyCap)
}

// RollEnemyHit rolls whether an enemy's basic attack connects. False = a clean
// miss; the caller skips the defend bar and narrates the whiff.
func RollEnemyHit(rng *rand.Rand, s Stats) bool {
	return RollChance(rng, EnemyHitChance(s))
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

// ProjectXP walks a starting (level, within-level remainder) forward by
// `added` XP and returns the resulting level, remainder, and the number of
// level thresholds crossed — WITHOUT mutating anything. It's the pure core
// of AddXP, shared with the victory spoils screen, which replays it at a
// partial `added` each frame to animate an XP bar filling across level-ups.
// startLvl is normalized to BaseLevel; negative `added` is treated as 0 so
// a caller can't accidentally drain XP.
func ProjectXP(startLvl, startXP, added int) (lvl, xp, gained int) {
	lvl = startLvl
	if lvl < BaseLevel {
		lvl = BaseLevel
	}
	xp = startXP
	if added > 0 {
		xp += added
	}
	for {
		need := XPForLevel(lvl)
		if need <= 0 || xp < need {
			break
		}
		xp -= need
		lvl++
		gained++
	}
	return lvl, xp, gained
}

// AddXP banks `amount` of experience onto a party member, processing
// multiple level-ups if the running total crosses several thresholds.
// Each level-up increments Level + queues a PendingLevelUps point-spend
// (the level-up modal drains the queue). Returns the number of levels
// the member just gained — callers use it for the "Warrior reaches
// level 3!" log line. No-op for amount<=0 or a dead member (HP <= 0,
// matching the "living members get XP" rule in AwardBattleXP).
//
// The level math is delegated to ProjectXP so the real award and the
// victory screen's animated preview can never disagree on where a given
// XP total lands. Each crossed level grants LevelStatPoints stat points
// (default 3) for the level-up modal PLUS LevelSkillPoints skill points
// (default 1) into the member's SkillPoints pool, spent later from the
// Skills panel's tree UI.
func AddXP(m *PartyMember, amount int) int {
	if amount <= 0 || m == nil || m.HP <= 0 {
		return 0
	}
	lvl, xp, gained := ProjectXP(m.Level, m.XP, amount)
	m.Level = lvl
	m.XP = xp
	m.PendingLevelUps += gained * LevelStatPoints
	m.SkillPoints += gained * LevelSkillPoints
	return gained
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
// Takes *PartyMember to avoid copying the (large) member struct on the
// per-card-per-frame draw path; read-only.
func HasUnspentPoints(m *PartyMember) bool {
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
	// PartyStatusBlessed / PartyStatusRegen are POSITIVE counted statuses
	// (Cleric's Bless / Renewal). They sit just above Defending in priority —
	// below every threat so a poisoned-and-blessed member still surfaces the
	// poison — and, like Defending, don't flicker (good news shouldn't alarm).
	PartyStatusBlessed
	PartyStatusRegen
	// PartyStatusShielded / PartyStatusIceArmor are the POSITIVE defensive wards
	// (Aegis ShieldHP / Ice Armor). They sit with Bless/Regen below every threat
	// and above Defending, and don't flicker. Without these rows a member whose
	// ONLY active status is a shield or ice ward showed no pill at all.
	PartyStatusShielded
	PartyStatusIceArmor
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
// Takes *PartyMember to avoid copying the member struct on the per-card-per-
// frame draw path; read-only.
//
// Both the priority resolution and the label read the single
// partyStatusBands table below — they used to be two parallel switches that
// could silently drift (a new kind added to the label switch but not the
// resolver would never surface), the same hazard woundBands fixed for enemy
// conditions.
func PartyStatus(m *PartyMember) (kind PartyStatusKind, turns int) {
	for _, band := range partyStatusBands {
		if active, t := band.Active(m); active {
			return band.Kind, t
		}
	}
	return PartyStatusNone, 0
}

// PartyStatusLabel returns the short uppercase label rendered for a
// given status kind. Pair with PartyStatus(m) to drive a render
// surface — never branch on the kind enum yourself at the call
// site; that's how the two surfaces drifted before this helper.
func PartyStatusLabel(kind PartyStatusKind) string {
	for _, band := range partyStatusBands {
		if band.Kind == kind {
			return band.Label
		}
	}
	return ""
}

// partyStatusBands is the priority-ordered source for both PartyStatus
// (resolver) and PartyStatusLabel — walked top to bottom, the first band whose
// Active predicate fires wins. Order is what surfaces when statuses stack:
// Down beats everything (nothing else matters at 0 HP), Ingested next (most
// disruptive — can't act, can't be hit), then the disabling lockout/DoT
// statuses in descending "how much it wrecks the plan" order, then the
// positive statuses, then Defending (lowest — a positive the player chose).
// Adding a PartyStatusKind is ONE row here, asserted complete in init().
var partyStatusBands = []struct {
	Kind   PartyStatusKind
	Label  string
	Active func(m *PartyMember) (active bool, turns int)
}{
	{PartyStatusDown, "DOWN", func(m *PartyMember) (bool, int) { return m.HP <= 0, 0 }},
	{PartyStatusIngested, "INGESTED", func(m *PartyMember) (bool, int) { return m.Ingested, 0 }},
	{PartyStatusWebbed, "WEBBED", func(m *PartyMember) (bool, int) { return m.WebbedTurns > 0, m.WebbedTurns }},
	{PartyStatusConfused, "CONFUSED", func(m *PartyMember) (bool, int) { return m.ConfusedTurns > 0, m.ConfusedTurns }},
	{PartyStatusStunned, "STUNNED", func(m *PartyMember) (bool, int) { return m.StunTurns > 0, m.StunTurns }},
	{PartyStatusAsleep, "ASLEEP", func(m *PartyMember) (bool, int) { return m.SleepTurns > 0, m.SleepTurns }},
	{PartyStatusPoisoned, "POISONED", func(m *PartyMember) (bool, int) { return m.PoisonTurns > 0, m.PoisonTurns }},
	{PartyStatusBlessed, "BLESSED", func(m *PartyMember) (bool, int) {
		return len(m.Buffs) > 0 && StatusModsNetBeneficial(m.Buffs), MaxStatusModTurns(m.Buffs)
	}},
	{PartyStatusRegen, "REGEN", func(m *PartyMember) (bool, int) { return m.RegenTurns > 0, m.RegenTurns }},
	{PartyStatusShielded, "SHIELD", func(m *PartyMember) (bool, int) { return m.ShieldHP > 0, m.ShieldHP }},
	{PartyStatusIceArmor, "ICE ARMOR", func(m *PartyMember) (bool, int) { return m.IceArmorTurns > 0, m.IceArmorTurns }},
	{PartyStatusDefending, "DEFENDING", func(m *PartyMember) (bool, int) { return m.Defending, 0 }},
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

// SumStats returns the per-field sum of two stat blocks. Used to fold a
// skill's per-tier BuffStats delta into its base effect (EffectiveSkillEffect)
// — no clamping, since buff deltas are authored non-negative; the floor-at-0
// guard lives at the EffectiveStats fold site where a hypothetical debuff
// would matter.
func SumStats(a, b Stats) Stats {
	// Hand-unrolled per-field add rather than the statTable/statSetters enum
	// loop: this is on the hottest combat path (folded per actor per action via
	// EffectiveStats / EffectiveEnemyStats / EffectiveSkillEffect), where the
	// slice-of-func-pointer indirection can't inline and dominates the cost of
	// six integer adds. A new Stat field needs a line here — the tradeoff is
	// deliberate for this fold (the editor/UI listing paths still use statTable).
	return Stats{
		STR: a.STR + b.STR,
		DEX: a.DEX + b.DEX,
		INT: a.INT + b.INT,
		WIS: a.WIS + b.WIS,
		VIT: a.VIT + b.VIT,
		SPD: a.SPD + b.SPD,
	}
}

// applyStatusMod inserts or refreshes a timed modifier in mods, keyed by Source:
// an existing entry from the same skill is REPLACED (a re-cast refreshes, never
// double-stacks), a new source is appended. A non-positive Turns is a no-op
// (returns mods unchanged) so a zero-duration effect leaves no inert entry. The
// shared insert path behind StampPartyBuff / StampEnemyDebuff — assign the
// returned (possibly grown) slice back.
func applyStatusMod(mods []StatusMod, mod StatusMod) []StatusMod {
	if mod.Turns <= 0 {
		return mods
	}
	for i := range mods {
		if mods[i].Source == mod.Source {
			mods[i] = mod
			return mods
		}
	}
	return append(mods, mod)
}

// SumStatusMods folds every active mod into one additive (stat, armor, mdef)
// bundle. The shared core read behind EffectiveStats / EffectiveArmor /
// EffectiveMDef (party) and EffectiveEnemyStats (enemy), so one stacking rule
// governs both sides.
func SumStatusMods(mods []StatusMod) (stats Stats, armor, mdef int) {
	for _, m := range mods {
		stats = SumStats(stats, m.Stats)
		armor += m.Armor
		mdef += m.MDef
	}
	return stats, armor, mdef
}

// addStatsFloored returns base + delta summed per-stat, each result floored at 0
// so a negative delta (a debuff, or a future cursed item) can't drive a stat
// below zero into the combat math (MaxHPFor / damage / accuracy). The single
// fold behind EffectiveStats' buff layer and EffectiveEnemyStats — one floor
// rule for both the party and enemy sides. A zero delta is a cheap no-op loop.
func addStatsFloored(base, delta Stats) Stats {
	// Hand-unrolled per-field add+floor (see SumStats) — same hot-path rationale.
	return Stats{
		STR: floorInt(base.STR + delta.STR),
		DEX: floorInt(base.DEX + delta.DEX),
		INT: floorInt(base.INT + delta.INT),
		WIS: floorInt(base.WIS + delta.WIS),
		VIT: floorInt(base.VIT + delta.VIT),
		SPD: floorInt(base.SPD + delta.SPD),
	}
}

// floorInt clamps a stat fold result up to 0 so a negative delta (a debuff, or a
// future cursed item) can't drive an effective stat below zero into the combat
// math (MaxHPFor / damage / accuracy).
func floorInt(v int) int {
	if v < 0 {
		return 0
	}
	return v
}

// TickStatusMods decrements every mod's Turns by one and drops the expired ones,
// returning the surviving slice (in place) plus the Sources that just expired
// (for "X fades" narration). Drained at the bearer's end-of-turn.
func TickStatusMods(mods []StatusMod) (remaining []StatusMod, expired []SkillID) {
	remaining = mods[:0]
	for _, m := range mods {
		m.Turns--
		if m.Turns > 0 {
			remaining = append(remaining, m)
		} else {
			expired = append(expired, m.Source)
		}
	}
	return remaining, expired
}

// MaxStatusModTurns returns the longest remaining duration across mods (0 when
// none) — what the single positive-buff pill shows for a multiply-buffed member.
func MaxStatusModTurns(mods []StatusMod) int {
	longest := 0
	for _, m := range mods {
		if m.Turns > longest {
			longest = m.Turns
		}
	}
	return longest
}

// StatusModsNetBeneficial reports whether the summed numeric effect of a
// StatusMod slice is net-positive (stat points + Armor + MDef). PartyMember.Buffs
// is the positive-buff channel, but StampPartyBuff stores ANY mod with Turns>0 —
// gating the "Blessed" pill on this keeps a future net-negative self-mod (a debuff
// authored with negative BuffStats) from mislabeling as a buff on the party card.
func StatusModsNetBeneficial(mods []StatusMod) bool {
	// Fold every mod into one (stats, armor, mdef) bundle through the shared
	// SumStatusMods accumulator (which itself folds per-stat via SumStats), then
	// total the aggregate's six stat points + armor + mdef once — instead of
	// re-inlining the six-field stat sum per mod, which had to be edited in
	// lockstep with SumStats whenever a Stat field was added.
	stats, armor, mdef := SumStatusMods(mods)
	sum := stats.STR + stats.DEX + stats.INT + stats.WIS + stats.VIT + stats.SPD + armor + mdef
	return sum > 0
}

// StampPartyBuff adds or refreshes one skill's buff on a member: a StatusMod
// keyed by source carrying the effect's BuffStats / BuffArmor / BuffMDef for
// BuffTurns. STACKS with OTHER skills' buffs (they sum in EffectiveStats /
// EffectiveArmor / EffectiveMDef); re-casting the SAME skill refreshes its entry
// rather than double-stacking. The single home for Bless / War Banner / Stone
// Skin / Smoke Bomb's per-member stamp, mirroring the enemy-side StampEnemyDebuff.
// Returns false (and no-ops) for a nil member or a non-buff effect (BuffTurns 0).
func StampPartyBuff(m *PartyMember, source SkillID, e SkillEffect) bool {
	if m == nil || e.BuffTurns <= 0 {
		return false
	}
	m.Buffs = applyStatusMod(m.Buffs, StatusMod{Source: source, Stats: e.BuffStats, Armor: e.BuffArmor, MDef: e.BuffMDef, Turns: e.BuffTurns})
	return true
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

// DebugBoostParty adds `amount` to every base stat of every party member and
// refreshes their derived pools the same way the per-point level-up Derive
// hooks do — MaxHP recomputed authoritatively from the new VIT, MaxMP grown by
// MPPerINT per added INT point — then tops HP/MP off (reviving the downed,
// since this is a god-mode test button). Combat re-reads stats through
// EffectiveStats, so a mid-battle boost takes effect immediately. Wired to the
// Debug submenu's "Boost Stats" row.
func DebugBoostParty(party []PartyMember, amount int) {
	for i := range party {
		m := &party[i]
		// Grow MaxMP by the ACTUAL applied INT delta, not the raw amount:
		// AdjustStat floors each stat at 0, so a negative boost can change INT
		// by less than `amount`. Using the raw amount would drop MaxMP (and the
		// MP=MaxMP that follows) below the floored-INT-justified value, even
		// negative. Mirror MaxHP, which already recomputes authoritatively.
		beforeINT := m.Stats.INT
		for s := Stat(0); s < StatCount; s++ {
			AdjustStat(&m.Stats, s, amount)
		}
		m.MaxHP = MaxHPFor(m.Stats)
		m.MaxMP += MPForINTDelta(m.Stats.INT - beforeINT)
		if m.MaxMP < 0 {
			m.MaxMP = 0
		}
		m.HP = m.MaxHP
		m.MP = m.MaxMP
	}
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
func StatPreviewLine(stat Stat, current Stats, pending int, accuracyStat Stat) string {
	if pending <= 0 || stat < 0 || int(stat) >= len(statTable) {
		return ""
	}
	after := current
	for i := 0; i < pending; i++ {
		statTable[stat].Add(&after)
	}
	switch stat {
	case StatSTR:
		// STR always drives melee DAMAGE. It only drives to-hit when STR is the
		// member's weapon accuracy stat (unarmed / heavy weapon); a light or
		// ranged weapon hits off DEX, so a STR spend wouldn't move its hit % —
		// don't preview a Hit gain that won't materialize.
		line := fmt.Sprintf("Melee %d→%d", MeleeDamage(current, 0), MeleeDamage(after, 0))
		if accuracyStat == StatSTR {
			h0 := MeleeAccuracy(current, TimingQualityMiss) * 100
			h1 := MeleeAccuracy(after, TimingQualityMiss) * 100
			line += fmt.Sprintf("  Hit %.0f→%.0f%%", h0, h1)
		}
		return line
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
		return fmt.Sprintf("Magic %d→%d  MaxMP +%d", MagicDamage(current, 0), MagicDamage(after, 0), MPForINTDelta(after.INT-current.INT))
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
	if len(partyClassDefinitions) != PartyMemberCount {
		panic("core: partyClassDefinitions length must match PartyMemberCount — append a class and bump PartyMemberCount together")
	}
	if PartyClassCount != PartyMemberCount {
		panic("core: PartyClassCount must match PartyMemberCount — one party member per class; bump both together")
	}
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
		if StatPreviewLine(i, probe, 1, StatSTR) == "" {
			panic(fmt.Sprintf("core: StatPreviewLine returned empty for stat index %d — add a preview case", int(i)))
		}
	}
	// partyStatusBands drives BOTH PartyStatus (resolver) and PartyStatusLabel,
	// so asserting every kind has a label transitively guarantees the resolver
	// can surface it too. Force coverage so a new PartyStatusKind added to the
	// enum but missed in partyStatusBands panics at STARTUP instead of silently
	// never surfacing. PartyStatusNone is the one kind with no row (the absence
	// of a status), so skip it.
	for k := PartyStatusNone + 1; k < PartyStatusCount; k++ {
		if PartyStatusLabel(k) == "" {
			panic(fmt.Sprintf("core: partyStatusBands missing kind %d — add a row", int(k)))
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
// All three drain PoisonTurns, deal DefaultPoisonEffect.TickDamage, and bypass
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
		// Per-tick damage reads through DefaultPoisonEffect.TickDamage so the
		// PoisonEffect struct is the single source for poison numbers — a
		// future trap/alchemist poison with a different TickDamage flows here
		// without re-touching this loop.
		if ApplyFlatDamage(&m.HP, &m.DamageFlash, DefaultPoisonEffect.TickDamage) {
			m.SleepTurns = 0
		}
		ticks++
	}
	return ticks
}

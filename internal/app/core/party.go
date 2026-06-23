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

// PartyClassCount is the number of PartyClass values. Equals PartyMemberCount
// (one member per class); init() asserts the two stay in lockstep.
const PartyClassCount = 4

type PartyClassDefinition struct {
	Class PartyClass
	Name  string
	Stats Stats
	MaxMP int
}

// PartyMemberCount is the fixed party size = number of class definitions. The
// slice order is the seating / tie-break contract; render formation + save
// format index by class slot. init() asserts len(partyClassDefinitions) matches.
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
// Press: single-press window (default); Charge: hold-and-release; Sequence:
// directional tap rhythm; Reels: slot-machine gamble; Recall: memory pattern;
// Overcharge: charge bar with a risky post-peak overload band.
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
	// Description is the panels-overlay Skills-tab blurb. Empty for enemy-only
	// skills (the tab only renders player-castable skills).
	Description string
	Cost        int
	TargetMode  ActionMode
	Kind        SkillKind
	// Tag drives armor/resist math + HUD color. Phys hits target.Armor; Magic /
	// Heal / Buff bypass.
	Tag      SkillTag
	Minigame SkillMinigame
	Effect   SkillEffect
	// PlayerCastable: valid choice from a member's action menu. Enemy-only
	// skills (Sleep, Ingest) set false so the menu can't surface them.
	PlayerCastable bool
	// EnemyCastable: the enemy AI's resolveEnemySpell can fire it; the editor's
	// custom-enemy skill picker filters on it. New enemy skill = flip + add a
	// resolveEnemySpell case.
	EnemyCastable bool
	// PerBattleCastLimit caps casts per caster per battle; 0 (default) = uncapped.
	// Checked against Enemy.SkillCastCount[skill] in usableEnemySkills (Necromancer's
	// SkillRaiseBones is the headline user).
	PerBattleCastLimit int
	// NoUpgrades marks a player-castable skill with NO tier-upgrade ladder (Scan):
	// granted by an actOnce node (MaxRank 1) and EXEMPT from skilltree.go's "every
	// PlayerCastable skill has MaxSkillTier rows" invariant.
	NoUpgrades bool
	// OnDiskName pins the mapfile skill identifier independent of the display Name.
	// Empty = derive from Name (lowercase, spaces→underscores). Freeze a slug here
	// BEFORE renaming a skill's display Name, else maps referencing the old slug
	// fail to load that skill. See SkillOnDiskName.
	OnDiskName string
}

type SkillEffect struct {
	Damage       int
	Heal         int
	StealChance  float64
	BurnChance   float64
	BurnMinTurns int
	BurnMaxTurns int
	// SleepMin/MaxTurns gate Sleep apply; zero = no sleep (apply short-circuits, no RNG).
	SleepMinTurns int
	SleepMaxTurns int
	// AppliesIngest pulls the target out of combat (Ingested) until the caster
	// dies — the mantrap's signature. Registry flag so apply needn't branch on SkillID.
	AppliesIngest bool
	// StunChance: per-apply Stun probability (skip-next-turn, doesn't break on
	// damage like Sleep). Zero = no stun. StunMin/Max bound the rolled duration.
	StunChance   float64
	StunMinTurns int
	StunMaxTurns int
	// PoisonChance/Min/Max: player-side Poison DoT apply (Thief's Venom Strike). Zero short-circuits.
	PoisonChance   float64
	PoisonMinTurns int
	PoisonMaxTurns int
	// BleedChance/Min/Max gate the Bleed DoT (Rend / Lacerate) — same shape as
	// Poison but a SEPARATE counter (Enemy.BleedTurns) so it stacks. Zero short-circuits.
	BleedChance   float64
	BleedMinTurns int
	BleedMaxTurns int
	// BindChance/Min/Max gate Webbed apply (halves SPD, refuses Ingest; Cave
	// Spider's SkillWeb). Zero short-circuits.
	BindChance   float64
	BindMinTurns int
	BindMaxTurns int
	// ConfuseChance/Min/Max gate Confused apply (per-action retarget roll).
	// WIS-resistible at apply time.
	ConfuseChance   float64
	ConfuseMinTurns int
	ConfuseMaxTurns int
	// AppliesAOEParty: damage hits EVERY living party member (Stoneslam). Per-target
	// armor applies. Damage = Effect.Damage + actor.SpellPower.
	AppliesAOEParty bool
	// AppliesSummonSkeleton inserts a fresh Skeleton into the caster's pack
	// (Necromancer); carrier flag so future raises reuse the apply.
	AppliesSummonSkeleton bool
	// AppliesAOEEnemies: player skill hits EVERY living enemy (Swipe/Whirlwind/Arc
	// Bolt). Set on exactly the skills whose handler loops the pack so the
	// SkillTargetsAllEnemies preview and the hit can't disagree.
	AppliesAOEEnemies bool
	// BuffStats / BuffTurns: a stat buff for BuffTurns turns (Bless), folded by
	// EffectiveStats while the counter runs. Zero BuffTurns = no buff. Pairs with
	// SkillTagBuff + AppliesAOEPartyBuff for the party-wide case.
	BuffStats Stats
	BuffTurns int
	// AppliesAOEPartyBuff: buff lands on EVERY living party member (Bless).
	AppliesAOEPartyBuff bool
	// RegenTurns: heal-over-time on an ally (Renewal). Apply stamps RegenTurns +
	// snapshots the WIS-scaled per-turn heal onto RegenPerTurn; ticks at end of
	// the member's turn. Zero = no HoT. Fixed duration like BuffTurns.
	RegenTurns int
	// ArmorReduction strips the target enemy's Armor (floored 0) for the rest of
	// the battle (Corrosive Vial) — permanent break, not a status. Zero = no strip.
	ArmorReduction int
	// ATBPush shoves the target enemy's ATB readiness back once on a landed hit
	// (Sunder) — one-shot, doesn't persist. Zero = no push.
	ATBPush int
	// BuffArmor / BuffMDef ride the buff bundle with BuffStats (Stone Skin):
	// flat Armor/MDef for BuffTurns, folded by EffectiveArmor/EffectiveMDef. Zero = none.
	BuffArmor int
	BuffMDef  int
	// ShieldHP: a damage-absorbing shield on an ally (Aegis), spent before HP until
	// depleted. Not turn-counted. Zero = no shield.
	ShieldHP int
	// IceArmorTurns: reactive frost ward on the caster (Ice Armor) — gains MDef
	// and chills attackers while it runs. Fixed duration, end-of-turn tick. Zero = none.
	IceArmorTurns int
}

// SEAT ORDER CONTRACT: the slice order is the in-battle seating order and the
// SPD-tie-breaker order in buildTurnQueue; save format and render formation
// index by class slot. Reordering reshuffles formation + tie-broken initiative
// — append a class, never insert.
var partyClassDefinitions = []PartyClassDefinition{
	{Class: ClassWarrior, Name: "Warrior", Stats: Stats{STR: 6, DEX: 2, INT: 1, WIS: 1, VIT: 5, SPD: 3}, MaxMP: 4},
	{Class: ClassCleric, Name: "Cleric", Stats: Stats{STR: 2, DEX: 2, INT: 2, WIS: 6, VIT: 4, SPD: 4}, MaxMP: 9},
	{Class: ClassThief, Name: "Thief", Stats: Stats{STR: 3, DEX: 6, INT: 2, WIS: 1, VIT: 4, SPD: 6}, MaxMP: 5},
	{Class: ClassWizard, Name: "Wizard", Stats: Stats{STR: 1, DEX: 2, INT: 6, WIS: 2, VIT: 4, SPD: 4}, MaxMP: 10},
}

// partyClassByID is the O(1) lookup for partyClassDefinitions (partyClassInfo is per-frame).
var partyClassByID = BuildRegistry(partyClassDefinitions, func(d PartyClassDefinition) PartyClass { return d.Class })

// Skill registry. Effect.Damage / Effect.Heal are flat baselines the
// stat-scaled formulas add on top (see MeleeDamage etc.).
var skillDefinitions = []skillDefinition{
	{Skill: SkillSwipe, Name: "Swipe", Description: "STR-scaled cleave through every living enemy in the pack.", Cost: 2, TargetMode: ActionMenu, Kind: SkillKindMelee, Tag: SkillTagPhys, Minigame: MinigamePress, Effect: SkillEffect{Damage: 0, AppliesAOEEnemies: true}, PlayerCastable: true},
	{Skill: SkillPrayer, Name: "Prayer", Description: "WIS-scaled single-ally heal. Charge bar — release at peak.", Cost: 4, TargetMode: ActionPartyTarget, Kind: SkillKindHeal, Tag: SkillTagHeal, Minigame: MinigameCharge, Effect: SkillEffect{Heal: 1}, PlayerCastable: true},
	{Skill: SkillSteal, Name: "Steal", Description: "Pickpocket the target. Stop the reels — matches drive the chance.", Cost: 0, TargetMode: ActionEnemyTarget, Kind: SkillKindUtility, Tag: SkillTagNone, Minigame: MinigameReels, Effect: SkillEffect{StealChance: StealBaseChance}, PlayerCastable: true},
	{Skill: SkillFirebolt, Name: "Firebolt", Description: "INT-scaled magic damage. Charge; release past the peak to Overcharge (bonus damage, burns you). Chance to inflict Burn.", Cost: 5, TargetMode: ActionEnemyTarget, Kind: SkillKindMagic, Tag: SkillTagMagic, Minigame: MinigameOvercharge, Effect: SkillEffect{Damage: 1, BurnChance: FireboltBurnChance, BurnMinTurns: FireBurnMinTurns, BurnMaxTurns: FireBurnMaxTurns}, PlayerCastable: true, EnemyCastable: true},
	// Crushing Blow (Warrior): charge single-target phys; Stun proc on Great/Excellent.
	{Skill: SkillCrushingBlow, Name: "Crushing Blow", Description: "STR-scaled heavy hit. Charge. Stun proc on Great+.", Cost: 3, TargetMode: ActionEnemyTarget, Kind: SkillKindMelee, Tag: SkillTagPhys, Minigame: MinigameCharge, Effect: SkillEffect{Damage: 4, StunChance: CrushingBlowStunChance, StunMinTurns: StunMinTurns, StunMaxTurns: StunMaxTurns}, PlayerCastable: true},
	// Whirlwind (Warrior): charge AoE phys; quality scales damage hard.
	{Skill: SkillWhirlwind, Name: "Whirlwind", Description: "STR-scaled AoE cleave. Charge — quality scales hard.", Cost: 4, TargetMode: ActionMenu, Kind: SkillKindMelee, Tag: SkillTagPhys, Minigame: MinigameCharge, Effect: SkillEffect{Damage: 2, AppliesAOEEnemies: true}, PlayerCastable: true},
	// Mass Mend (Cleric): charge AoE heal across the alive party.
	{Skill: SkillMassMend, Name: "Mass Mend", Description: "WIS-scaled heal across the whole alive party. Charge.", Cost: 6, TargetMode: ActionMenu, Kind: SkillKindHeal, Tag: SkillTagHeal, Minigame: MinigameCharge, Effect: SkillEffect{Heal: 1}, PlayerCastable: true},
	// Smite (Cleric): press-tap single-target WIS-scaled magic.
	{Skill: SkillSmite, Name: "Smite", Description: "WIS-scaled magic damage. Press tap, no burn.", Cost: 3, TargetMode: ActionEnemyTarget, Kind: SkillKindMagic, Tag: SkillTagMagic, Minigame: MinigamePress, Effect: SkillEffect{Damage: 2}, PlayerCastable: true},
	// Backstab (Thief): charge single-target phys; damage DOUBLES on Excellent
	// (the crit multiplier lives in applyBackstab — SkillEffect has no crit field).
	{Skill: SkillBackstab, Name: "Backstab", Description: "STR-scaled phys hit. Damage doubles on Excellent.", Cost: 3, TargetMode: ActionEnemyTarget, Kind: SkillKindMelee, Tag: SkillTagPhys, Minigame: MinigameCharge, Effect: SkillEffect{Damage: 2}, PlayerCastable: true},
	// Venom Strike (Thief): sequence single-target phys + Poison apply (quality scales the proc).
	{Skill: SkillVenomStrike, Name: "Venom Strike", Description: "STR-scaled phys hit. Sequence — applies Poison.", Cost: 3, TargetMode: ActionEnemyTarget, Kind: SkillKindMelee, Tag: SkillTagPhys, Minigame: MinigameSequence, Effect: SkillEffect{Damage: 1, PoisonChance: VenomStrikePoisonChance, PoisonMinTurns: PoisonMinTurns, PoisonMaxTurns: PoisonMaxTurns}, PlayerCastable: true},
	// Cripple (Thief): single-target SPD debuff, no damage. Stamps negative SPD
	// via the enemy BuffStats mirror, dropping the foe's ATB turn-rate.
	{Skill: SkillCripple, Name: "Cripple", Description: "Sap an enemy's SPD for a few turns, slowing how often it acts. No damage.", Cost: 3, TargetMode: ActionEnemyTarget, Kind: SkillKindUtility, Tag: SkillTagNone, Minigame: MinigamePress, Effect: SkillEffect{BuffStats: Stats{SPD: -CrippleSPDReduction}, BuffTurns: CrippleTurns}, PlayerCastable: true},
	// Corrosive Vial (Thief): no damage; permanently strips target Armor (floored
	// 0, mutates Enemy.Armor) for the rest of the battle.
	{Skill: SkillCorrosiveVial, Name: "Corrosive Vial", Description: "Hurl acid that eats an enemy's Armor for the rest of the fight, so every hit lands harder. No damage.", Cost: 3, TargetMode: ActionEnemyTarget, Kind: SkillKindUtility, Tag: SkillTagNone, Minigame: MinigamePress, Effect: SkillEffect{ArmorReduction: CorrosiveArmorReduction}, PlayerCastable: true},
	// Frost Lance (Wizard): charge single-target magic; reliable Stun on Great/Excellent.
	{Skill: SkillFrostLance, Name: "Frost Lance", Description: "INT-scaled magic damage. Reliable Stun on Great+.", Cost: 5, TargetMode: ActionEnemyTarget, Kind: SkillKindMagic, Tag: SkillTagMagic, Minigame: MinigameCharge, Effect: SkillEffect{Damage: 2, StunChance: FrostLanceStunChance, StunMinTurns: FrostLanceStunTurns, StunMaxTurns: FrostLanceStunTurns}, PlayerCastable: true},
	// Frostbite (Wizard): charge INT-scaled magic + guaranteed SPD chill (no proc roll).
	{Skill: SkillFrostbite, Name: "Frostbite", Description: "INT-scaled frost magic that always chills — lowers the target's SPD for a few turns.", Cost: 4, TargetMode: ActionEnemyTarget, Kind: SkillKindMagic, Tag: SkillTagMagic, Minigame: MinigameCharge, Effect: SkillEffect{Damage: FrostbiteDamageBase, BuffStats: Stats{SPD: -FrostbiteSPDReduction}, BuffTurns: FrostbiteChillTurns}, PlayerCastable: true},
	// Cone of Cold (Wizard): pack-wide Frostbite — INT-scaled frost + per-target
	// SPD chill via applyAoEStatusSkill.
	{Skill: SkillConeOfCold, Name: "Cone of Cold", Description: "INT-scaled frost across the whole pack. Charge — chills every enemy, lowering their SPD.", Cost: 7, TargetMode: ActionMenu, Kind: SkillKindMagic, Tag: SkillTagMagic, Minigame: MinigameCharge, Effect: SkillEffect{Damage: ConeOfColdDamageBase, AppliesAOEEnemies: true, BuffStats: Stats{SPD: -ConeOfColdSPDReduction}, BuffTurns: ConeOfColdChillTurns}, PlayerCastable: true},
	// Sunder (Warrior): charge STR-scaled phys + ATBPush (shoves the target's next turn later on a hit).
	{Skill: SkillSunder, Name: "Sunder", Description: "STR-scaled phys hit that shoves the target's turn later. Charge.", Cost: 4, TargetMode: ActionEnemyTarget, Kind: SkillKindMelee, Tag: SkillTagPhys, Minigame: MinigameCharge, Effect: SkillEffect{Damage: SunderDamageBase, ATBPush: SunderATBPush}, PlayerCastable: true},
	// Taunt (Warrior): forces the target to attack the caster next turn; no damage. NoUpgrades.
	{Skill: SkillTaunt, Name: "Taunt", Description: "Force the target enemy to attack you next turn. No damage.", Cost: 2, TargetMode: ActionEnemyTarget, Kind: SkillKindUtility, Tag: SkillTagNone, Minigame: MinigamePress, Effect: SkillEffect{}, PlayerCastable: true, NoUpgrades: true},
	// War Banner (Warrior): party-wide STR + Armor rally. Armor (not VIT) is the
	// defensive half — a VIT buff would be inert since MaxHP isn't re-derived.
	{Skill: SkillWarBanner, Name: "War Banner", Description: "Plant a banner — raises the whole party's STR and Armor for several turns. No damage.", Cost: 5, TargetMode: ActionMenu, Kind: SkillKindUtility, Tag: SkillTagBuff, Minigame: MinigamePress, Effect: SkillEffect{BuffStats: Stats{STR: WarBannerPerStat}, BuffArmor: WarBannerArmor, BuffTurns: WarBannerTurns, AppliesAOEPartyBuff: true}, PlayerCastable: true},
	// Stone Skin (Warrior): single-ally Armor + MDef ward (BuffArmor/BuffMDef); no damage.
	{Skill: SkillStoneSkin, Name: "Stone Skin", Description: "Ward an ally with temporary Armor and MDef. No damage.", Cost: 4, TargetMode: ActionPartyTarget, Kind: SkillKindUtility, Tag: SkillTagBuff, Minigame: MinigamePress, Effect: SkillEffect{BuffArmor: StoneSkinArmor, BuffMDef: StoneSkinMDef, BuffTurns: StoneSkinTurns}, PlayerCastable: true},
	// Blind (Cleric): saps the target's DEX (accuracy) via the enemy BuffStats mirror; no damage.
	{Skill: SkillBlind, Name: "Blind", Description: "Sear an enemy's eyes — lowers its accuracy for several turns. No damage.", Cost: 3, TargetMode: ActionEnemyTarget, Kind: SkillKindUtility, Tag: SkillTagNone, Minigame: MinigamePress, Effect: SkillEffect{BuffStats: Stats{DEX: -BlindDEXReduction}, BuffTurns: BlindTurns}, PlayerCastable: true},
	// Aegis (Cleric): grants an ally a damage-absorbing shield (ShieldHP); no damage.
	{Skill: SkillAegis, Name: "Aegis", Description: "Shield an ally — absorbs incoming damage before it reaches their HP. No damage.", Cost: 4, TargetMode: ActionPartyTarget, Kind: SkillKindUtility, Tag: SkillTagBuff, Minigame: MinigamePress, Effect: SkillEffect{ShieldHP: AegisShieldBase}, PlayerCastable: true},
	// Smoke Bomb (Thief): one DEX magnitude buffs party evasion AND saps every
	// enemy's accuracy (handler mirrors the buff onto enemies); no damage.
	{Skill: SkillSmokeBomb, Name: "Smoke Bomb", Description: "Drop a smoke screen — the party gains evasion while every enemy loses accuracy. No damage.", Cost: 4, TargetMode: ActionMenu, Kind: SkillKindUtility, Tag: SkillTagNone, Minigame: MinigamePress, Effect: SkillEffect{BuffStats: Stats{DEX: SmokeBombDEX}, BuffTurns: SmokeBombTurns, AppliesAOEPartyBuff: true}, PlayerCastable: true},
	// Ice Armor (Wizard): self-buff — gains MDef and chills any enemy that basic-attacks the caster.
	{Skill: SkillIceArmor, Name: "Ice Armor", Description: "Sheathe yourself in frost — gain MDef and chill any foe that strikes you. Charge.", Cost: 5, TargetMode: ActionMenu, Kind: SkillKindUtility, Tag: SkillTagBuff, Minigame: MinigameCharge, Effect: SkillEffect{IceArmorTurns: IceArmorTurnsBase}, PlayerCastable: true},
	// Rend (Warrior): charge STR-scaled phys + Bleed DoT (own Enemy.BleedTurns counter, stacks with Poison).
	{Skill: SkillRend, Name: "Rend", Description: "STR-scaled phys hit that opens a Bleed wound — damage over time. Charge.", Cost: 4, TargetMode: ActionEnemyTarget, Kind: SkillKindMelee, Tag: SkillTagPhys, Minigame: MinigameCharge, Effect: SkillEffect{Damage: RendDamageBase, BleedChance: RendBleedChance, BleedMinTurns: BleedMinTurns, BleedMaxTurns: BleedMaxTurns}, PlayerCastable: true},
	// Lacerate (Thief): sequence Bleed (same as Rend), lighter hit — stacks alongside Poison (separate counters).
	{Skill: SkillLacerate, Name: "Lacerate", Description: "STR-scaled phys cut that opens a Bleed — stacks alongside Poison. Sequence.", Cost: 4, TargetMode: ActionEnemyTarget, Kind: SkillKindMelee, Tag: SkillTagPhys, Minigame: MinigameSequence, Effect: SkillEffect{Damage: LacerateDamageBase, BleedChance: LacerateBleedChance, BleedMinTurns: BleedMinTurns, BleedMaxTurns: BleedMaxTurns}, PlayerCastable: true},
	// Arc Bolt (Wizard): recall AoE magic — quality-scaled damage to all living enemies.
	{Skill: SkillArcBolt, Name: "Arc Bolt", Description: "INT-scaled magic AoE. Memorize the glyph pattern, then recall it — arcs to every enemy.", Cost: 6, TargetMode: ActionMenu, Kind: SkillKindMagic, Tag: SkillTagMagic, Minigame: MinigameRecall, Effect: SkillEffect{Damage: 1, AppliesAOEEnemies: true}, PlayerCastable: true},
	// Scan (Thief): no damage; IDs the target's kind (Bestiary.MarkScanned,
	// shortcut past the 5-kills threshold) revealing its HP. Lands at any grade. NoUpgrades.
	{Skill: SkillScan, Name: "Scan", Description: "Identify the target's kind — reveals its HP (here and in the bestiary). No damage.", Cost: 2, TargetMode: ActionEnemyTarget, Kind: SkillKindUtility, Tag: SkillTagNone, Minigame: MinigamePress, Effect: SkillEffect{}, PlayerCastable: true, NoUpgrades: true},
	// Bless (Cleric): party-wide stat buff, no damage. AppliesAOEPartyBuff drives
	// the loop-the-party apply; tier upgrades stack magnitude + duration.
	{Skill: SkillBless, Name: "Bless", Description: "Bless the whole party — raises STR, DEX, INT and WIS for a few turns. No damage.", Cost: 4, TargetMode: ActionMenu, Kind: SkillKindUtility, Tag: SkillTagBuff, Minigame: MinigamePress, Effect: SkillEffect{BuffStats: Stats{STR: BlessBuffPerStat, DEX: BlessBuffPerStat, INT: BlessBuffPerStat, WIS: BlessBuffPerStat}, BuffTurns: BlessBuffTurns, AppliesAOEPartyBuff: true}, PlayerCastable: true},
	// Fireball (Wizard): AoE Firebolt — INT-scaled magic to all living enemies +
	// per-target Burn roll, via applyAoEStatusSkill.
	{Skill: SkillFireball, Name: "Fireball", Description: "INT-scaled magic fire across the whole pack. Charge — per-target Burn chance.", Cost: 7, TargetMode: ActionMenu, Kind: SkillKindMagic, Tag: SkillTagMagic, Minigame: MinigameCharge, Effect: SkillEffect{Damage: 1, AppliesAOEEnemies: true, BurnChance: FireballBurnChance, BurnMinTurns: FireBurnMinTurns, BurnMaxTurns: FireBurnMaxTurns}, PlayerCastable: true},
	// Poison Cloud (Thief): AoE Venom Strike — light STR-scaled damage to all
	// living enemies + per-target Poison roll (the whole-pack DoT is the point).
	{Skill: SkillPoisonCloud, Name: "Poison Cloud", Description: "STR-scaled toxin across the whole pack. Sequence — per-target Poison chance.", Cost: 6, TargetMode: ActionMenu, Kind: SkillKindMelee, Tag: SkillTagPhys, Minigame: MinigameSequence, Effect: SkillEffect{Damage: 1, AppliesAOEEnemies: true, PoisonChance: PoisonCloudPoisonChance, PoisonMinTurns: PoisonMinTurns, PoisonMaxTurns: PoisonMaxTurns}, PlayerCastable: true},
	// Cleanse (Cleric): single-ally cure via CureDebuffs (leaves Bless + Defending
	// intact); no damage. NoUpgrades.
	{Skill: SkillCleanse, Name: "Cleanse", Description: "Cure an ally's Poison, Sleep, Stun, Web and Confusion. No damage.", Cost: 3, TargetMode: ActionPartyTarget, Kind: SkillKindUtility, Tag: SkillTagNone, Minigame: MinigamePress, Effect: SkillEffect{}, PlayerCastable: true, NoUpgrades: true},
	// Second Wind (Warrior): flat self-heal. Utility kind so low WIS doesn't gate
	// it (base is flat; quality still scales). Tier ladder adds +heal.
	{Skill: SkillSecondWind, Name: "Second Wind", Description: "Catch your breath — a flat self-heal. Charge.", Cost: 3, TargetMode: ActionMenu, Kind: SkillKindUtility, Tag: SkillTagNone, Minigame: MinigameCharge, Effect: SkillEffect{Heal: SecondWindHealBase}, PlayerCastable: true},
	// Renewal (Cleric): HoT on one ally. Effect.Heal = base per-turn (snapshots
	// the WIS-scaled value at cast), RegenTurns = base duration. Tier adds +turns/+per-turn.
	{Skill: SkillRenewal, Name: "Renewal", Description: "Heal-over-time on an ally — restores HP at the end of their turns. Charge.", Cost: 4, TargetMode: ActionPartyTarget, Kind: SkillKindHeal, Tag: SkillTagNone, Minigame: MinigameCharge, Effect: SkillEffect{Heal: RenewalRegenBase, RegenTurns: RenewalRegenTurns}, PlayerCastable: true},
	// Sleep (goblin-mage): Magic-tagged so armor doesn't gate the proc; damage 0,
	// status only. A future player caster sets Cost + flips PlayerCastable.
	{Skill: SkillSleep, Name: "Sleep", Cost: 0, TargetMode: ActionPartyTarget, Kind: SkillKindUtility, Tag: SkillTagMagic, Minigame: MinigamePress, Effect: SkillEffect{SleepMinTurns: SleepMinTurns, SleepMaxTurns: SleepMaxTurns}, EnemyCastable: true},
	// Ingest (enemy-only): AppliesIngest carries "removed from combat until the caster dies."
	{Skill: SkillIngest, Name: "Ingest", Cost: 0, TargetMode: ActionPartyTarget, Kind: SkillKindUtility, Tag: SkillTagMagic, Minigame: MinigamePress, Effect: SkillEffect{AppliesIngest: true}, EnemyCastable: true},
	// Web (Cave Spider, enemy-only): applies Webbed (halves SPD, blocks Ingest); duration in BindMin/Max.
	{Skill: SkillWeb, Name: "Web", Cost: 0, TargetMode: ActionPartyTarget, Kind: SkillKindUtility, Tag: SkillTagMagic, Minigame: MinigamePress, Effect: SkillEffect{BindChance: WebBindChance, BindMinTurns: SpiderWebbedMinTurns, BindMaxTurns: SpiderWebbedMaxTurns}, EnemyCastable: true},
	// Confuse (Will-o'-Wisp, enemy-only): applies Confused (per-action retarget); WIS resists on apply.
	{Skill: SkillConfuse, Name: "Confuse", Cost: 0, TargetMode: ActionPartyTarget, Kind: SkillKindUtility, Tag: SkillTagMagic, Minigame: MinigamePress, Effect: SkillEffect{ConfuseChance: WispConfuseApplyChance, ConfuseMinTurns: WispConfuseMinTurns, ConfuseMaxTurns: WispConfuseMaxTurns}, EnemyCastable: true},
	// Stoneslam (Stone Golem, enemy-only): Phys AoE hitting every living member;
	// per-target armor applies. Value = caster SpellPower + Effect.Damage, quality-scaled.
	{Skill: SkillStoneslam, Name: "Stoneslam", Cost: 0, TargetMode: ActionMenu, Kind: SkillKindMelee, Tag: SkillTagPhys, Minigame: MinigamePress, Effect: SkillEffect{Damage: 2, AppliesAOEParty: true}, EnemyCastable: true},
	// Raise Bones (Necromancer, enemy-only): summons into the caster's pack.
	// PerBattleCastLimit drops it from the AI pick list once spent (no apply-time gate).
	{Skill: SkillRaiseBones, Name: "Raise Bones", Cost: 0, TargetMode: ActionMenu, Kind: SkillKindUtility, Tag: SkillTagMagic, Minigame: MinigamePress, Effect: SkillEffect{AppliesSummonSkeleton: true}, EnemyCastable: true, PerBattleCastLimit: NecromancerRaiseLimit},
}

// SkillPlayerCastable reports whether the skill can appear in a member's action
// menu. Enemy-only and unknown skill IDs return false (kept out of the menu).
func SkillPlayerCastable(s SkillID) bool {
	def, ok := skillInfo(s)
	return ok && def.PlayerCastable
}

// SkillHasNoUpgrades reports whether a skill opts out of the tier-upgrade
// ladder (Scan); the skilltree.go tier-table invariant skips these. Unknown = false.
func SkillHasNoUpgrades(s SkillID) bool {
	def, ok := skillInfo(s)
	return ok && def.NoUpgrades
}

// PlayerCastableSkills returns every PlayerCastable skill in registry order.
func PlayerCastableSkills() []SkillID {
	return PlayerCastableSkillsInto(make([]SkillID, 0, len(skillDefinitions)))
}

// PlayerCastableSkillsInto is the buffer-reusing form of PlayerCastableSkills
// for per-frame callers (buf re-sliced to length 0; pass nil to allocate).
func PlayerCastableSkillsInto(buf []SkillID) []SkillID {
	return skillIDsWhereInto(buf, func(d skillDefinition) bool { return d.PlayerCastable })
}

// skillIDsWhereInto appends every SkillID whose entry satisfies pred (nil = all)
// to buf (re-sliced to 0), in declaration order. Shared by PlayerCastableSkillsInto
// / AllSkillIDs / EnemyCastableSkills.
func skillIDsWhereInto(buf []SkillID, pred func(skillDefinition) bool) []SkillID {
	buf = buf[:0]
	for _, def := range skillDefinitions {
		if pred == nil || pred(def) {
			buf = append(buf, def.Skill)
		}
	}
	return buf
}

// skillByID is the O(1) read cache for skillDefinitions (the slice stays the
// source of truth + iteration order).
var skillByID = BuildRegistry(skillDefinitions, func(d skillDefinition) SkillID { return d.Skill })

func PartyClasses() []PartyClassDefinition {
	defs := make([]PartyClassDefinition, len(partyClassDefinitions))
	copy(defs, partyClassDefinitions)
	return defs
}

// AllPartyClasses returns the PartyClass keys in definition order — the single
// home for "iterate every class" loops.
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

// PartyClassName returns the display name for a class ("Warrior"), falling back
// to the slug if unregistered.
func PartyClassName(class PartyClass) string {
	if def, ok := partyClassInfo(class); ok {
		return def.Name
	}
	return PartyClassSlug(class)
}

// PartySkill returns the skill at member.SkillCursor within their learned
// skills (Tab cycles the index). Out-of-range cursors clamp to 0; nothing
// learned returns SkillNone.
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

// PartySkills returns the member's learned castable skills (≥1 invested rank),
// in stable tree/node order. Empty when nothing learned.
func PartySkills(member *PartyMember) []SkillID {
	return LearnedSkills(member)
}

func skillInfo(skill SkillID) (skillDefinition, bool) {
	def, ok := skillByID[skill]
	return def, ok
}

// SkillMinigameFor returns the skill's minigame kind (Press for unknown IDs).
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

// SkillDescription returns the panels-overlay blurb (empty when none authored).
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

// SpendSkillMP deducts the skill's MP cost if affordable, returning whether the
// cast may proceed. The single seam for "pay for a skill."
func SpendSkillMP(m *PartyMember, skill SkillID) bool {
	if m == nil || !CanAffordSkill(m, skill) {
		return false
	}
	m.MP -= SkillCost(skill)
	return true
}

// SkillCastLimitFor returns the registry's PerBattleCastLimit (0 = uncapped;
// unknown skill = 0).
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

// SkillTargetsAllEnemies reports whether a player skill hits every living enemy
// (reads AppliesAOEEnemies; Stoneslam hits the party and is enemy-only, excluded).
func SkillTargetsAllEnemies(skill SkillID) bool {
	def, ok := skillInfo(skill)
	return ok && def.PlayerCastable && def.Effect.AppliesAOEEnemies
}

// SkillUsableOutOfBattle reports whether a player skill can be cast while exploring:
// a friendly-targeted skill whose whole effect is an immediate, lasting benefit — an
// HP heal or a status cure. Buffs, wards and heal-over-time (Bless, Stone Skin, Aegis,
// War Banner, Smoke Bomb, Ice Armor, Renewal) are battle-only — their turn counters
// can't tick outside combat, so they're excluded.
func SkillUsableOutOfBattle(skill SkillID) bool {
	def, ok := skillInfo(skill)
	if !ok || !def.PlayerCastable {
		return false
	}
	// Friendly only — never an enemy-target or any AoE-enemy/party-damage skill.
	if def.TargetMode == ActionEnemyTarget || def.Effect.AppliesAOEEnemies || def.Effect.AppliesAOEParty {
		return false
	}
	// Reject anything turn-scoped or buff/ward-shaped (it would do nothing out of battle).
	e := def.Effect
	if e.BuffTurns != 0 || e.RegenTurns != 0 || e.ShieldHP != 0 || e.IceArmorTurns != 0 ||
		e.BuffArmor != 0 || e.BuffMDef != 0 || e.AppliesAOEPartyBuff || e.BuffStats != (Stats{}) {
		return false
	}
	// Beneficial: restores HP, or cures status (Cleanse — its cure isn't an Effect field).
	return e.Heal > 0 || skill == SkillCleanse
}

// OutOfBattleSkillScope classifies how an out-of-battle support skill applies, so
// the explore-side caster can route it the way battle does (pick / self / party).
type OutOfBattleSkillScope int

const (
	SkillScopeAlly  OutOfBattleSkillScope = iota // pick one living ally (Prayer, Cleanse)
	SkillScopeSelf                               // the caster only (Second Wind)
	SkillScopeParty                              // every living member (Mass Mend)
)

// OutOfBattleSkillScopeFor classifies skill's apply scope: a single-ally target picks
// a recipient; an untargeted Heal-kind skill blankets the party; anything else is self.
func OutOfBattleSkillScopeFor(skill SkillID) OutOfBattleSkillScope {
	def, ok := skillInfo(skill)
	if !ok {
		return SkillScopeSelf
	}
	switch {
	case def.TargetMode == ActionPartyTarget:
		return SkillScopeAlly
	case def.Kind == SkillKindHeal:
		return SkillScopeParty
	default:
		return SkillScopeSelf
	}
}

// OutOfBattleSupportSkills returns the member's out-of-battle-castable support skills
// (heals + cures), in skill order. Per-frame callers use OutOfBattleSupportSkillsInto.
func OutOfBattleSupportSkills(m *PartyMember) []SkillID {
	return OutOfBattleSupportSkillsInto(nil, m)
}

// OutOfBattleSupportSkillsInto is OutOfBattleSupportSkills into a caller-owned buffer
// (re-sliced to 0; result aliases buf until next reuse).
func OutOfBattleSupportSkillsInto(buf []SkillID, m *PartyMember) []SkillID {
	return filterInto(buf, PartySkills(m), func(s SkillID) bool {
		return s != SkillNone && SkillUsableOutOfBattle(s)
	})
}

// HealMember restores up to `amount` HP to a LIVING, non-ingested member,
// clamped at MaxHP. Never revives, ignores non-positive amounts, skips
// ingested. Returns true when the member was a valid heal target.
func HealMember(m *PartyMember, amount int) bool {
	if m == nil || amount <= 0 || !partyAvailable(*m) {
		return false
	}
	GainUpTo(&m.HP, m.MaxHP, amount)
	return true
}

// HealWholeParty applies HealMember(amount) to every LIVING (HP > 0) member —
// the shared party-wide heal loop behind both Mass Mend paths.
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

// RestorePartyFully tops every LIVING member to full HP AND MP (the healing
// crystal's effect). Returns the number restored.
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

// TickCrystalRecharge advances every dormant crystal's charge one step, re-arming
// it at CrystalRechargeSteps. Call once per landed exploration step.
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

// clearMemberAnimTimers zeros a member's transient animation timers (lunge /
// damage-flash / knockback), not gameplay state (statuses go through
// ClearPartyTransientStatuses).
func clearMemberAnimTimers(m *PartyMember) {
	m.AttackBump = 0
	m.DamageFlash = 0
	m.HitKnockback = 0
}

// RestoreMP tops up MP by amount (clamped at MaxMP), returning the actual amount
// restored. Like HealMember: a downed or ingested member can't drink.
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

// scaleDamageByKind: Melee adds STR, Magic adds INT, else passes base through.
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

// scaleHealByKind: Heal kind adds WIS, else passes base through.
func scaleHealByKind(kind SkillKind, stats Stats, base int) int {
	if kind == SkillKindHeal {
		return HealAmount(stats, base)
	}
	return base
}

// SkillDamage computes a skill's pre-quality damage from the actor's stats
// (quality scaling applies on top at the call site).
func SkillDamage(stats Stats, skill SkillID) int {
	def, ok := skillInfo(skill)
	if !ok {
		return 0
	}
	return scaleDamageByKind(def.Kind, stats, def.Effect.Damage)
}

// SkillHeal computes a skill's pre-quality heal from the actor's stats.
func SkillHeal(stats Stats, skill SkillID) int {
	def, ok := skillInfo(skill)
	if !ok {
		return 0
	}
	return scaleHealByKind(def.Kind, stats, def.Effect.Heal)
}

// SumStatPending totals the level-up modal's staged per-stat allocations.
func SumStatPending(p [StatCount]int) int {
	n := 0
	for _, v := range p {
		n += v
	}
	return n
}

// rollDuration is the shared uniform [min, max] draw behind every SkillEffect
// status-duration helper. Degenerate bounds (min <= 0 || max < min) return 0
// (fail open to "no status") — note this differs from RandRangeI (returns lo)
// and rollGold (swaps then clamps).
func rollDuration(rng *rand.Rand, min, max int) int {
	if min <= 0 || max < min {
		return 0
	}
	return RandRangeI(rng, min, max)
}

// BurnDuration rolls a uniform burn duration in [Min, Max] (via rollDuration).
func (effect SkillEffect) BurnDuration(rng *rand.Rand) int {
	return rollDuration(rng, effect.BurnMinTurns, effect.BurnMaxTurns)
}

// SleepDuration rolls a uniform sleep duration in [Min, Max]; degenerate bounds → 0.
func (effect SkillEffect) SleepDuration(rng *rand.Rand) int {
	return rollDuration(rng, effect.SleepMinTurns, effect.SleepMaxTurns)
}

// StunDuration rolls a uniform stun duration in [Min, Max]; degenerate bounds → 0.
func (effect SkillEffect) StunDuration(rng *rand.Rand) int {
	return rollDuration(rng, effect.StunMinTurns, effect.StunMaxTurns)
}

// BindDuration rolls a uniform bind duration in [Min, Max]; degenerate bounds → 0.
func (effect SkillEffect) BindDuration(rng *rand.Rand) int {
	return rollDuration(rng, effect.BindMinTurns, effect.BindMaxTurns)
}

// ConfuseDuration rolls a uniform confuse duration in [Min, Max]; degenerate bounds → 0.
func (effect SkillEffect) ConfuseDuration(rng *rand.Rand) int {
	return rollDuration(rng, effect.ConfuseMinTurns, effect.ConfuseMaxTurns)
}

// PoisonDuration rolls a uniform poison duration in [Min, Max]; degenerate bounds → 0.
func (effect SkillEffect) PoisonDuration(rng *rand.Rand) int {
	return rollDuration(rng, effect.PoisonMinTurns, effect.PoisonMaxTurns)
}

// BleedDuration rolls a uniform bleed duration in [Min, Max]; degenerate bounds → 0.
func (effect SkillEffect) BleedDuration(rng *rand.Rand) int {
	return rollDuration(rng, effect.BleedMinTurns, effect.BleedMaxTurns)
}

// SkillTagFor returns a skill's SkillTag (drives the armor clip; phys only).
// Unknown = SkillTagNone (treated as "don't apply armor").
func SkillTagFor(skill SkillID) SkillTag {
	if def, ok := skillInfo(skill); ok {
		return def.Tag
	}
	return SkillTagNone
}

// SkillAttackClassFor returns a skill's reach class for the row rules (only melee
// is front-gated), derived from its Kind. Unknown = ranged (any-row).
func SkillAttackClassFor(skill SkillID) AttackClass {
	if def, ok := skillInfo(skill); ok {
		return SkillAttackClass(def.Kind)
	}
	return AttackRanged
}

// ApplyArmor clamps physical damage by the target's armor (floor 1; armor is a
// damp, not immunity). Only SkillTagPhys is affected; everything else passes through.
func ApplyArmor(dmg int, tag SkillTag, armor int) int {
	if tag != SkillTagPhys {
		return dmg
	}
	return mitigate(dmg, armor)
}

// mitigate subtracts soak from dmg with a floor of 1. Zero dmg or non-positive
// soak pass through. Shared by ApplyArmor + ApplyMagicDefense.
func mitigate(dmg, soak int) int {
	if soak <= 0 || dmg <= 0 {
		return dmg
	}
	if reduced := dmg - soak; reduced > 1 {
		return reduced
	}
	return 1
}

// MagicDefense returns magic mitigation from a Stats block — currently WIS 1:1.
func MagicDefense(s Stats) int {
	return s.WIS
}

// ApplyMagicDefense clamps SkillTagMagic damage by the target's MDef (floor 1).
// Everything else (incl. phys, already armored) passes through — no double-soak.
func ApplyMagicDefense(dmg int, tag SkillTag, mdef int) int {
	if tag != SkillTagMagic {
		return dmg
	}
	return mitigate(dmg, mdef)
}

// ApplyFlatDamage applies a pre-mitigated flat amount to an actor: stamps the
// damage flash (only on positive amount) and floors HP at 0. Returns true if it
// dropped the actor to 0. Pointer-based so Enemy and PartyMember share it; the
// shared HP-floor + flash contract behind battle damage AND the poison tick.
func ApplyFlatDamage(hp *int, flash *float32, amount int) (died bool) {
	if amount > 0 {
		*flash = FlashDuration
	}
	*hp -= amount
	if *hp <= 0 {
		*hp = 0
		return true
	}
	return false
}

// ApplyHitRecoil arms the "took a real hit" reaction: on positive damage the
// knockback timer arms and any active Sleep breaks. Zero/soaked hits no-op. The
// out-of-battle poison tick deliberately skips this (wakes only on a lethal tick).
func ApplyHitRecoil(knockback *float32, sleep *int, damage int) {
	if damage <= 0 {
		return
	}
	*knockback = HitKnockbackDuration
	if *sleep > 0 {
		*sleep = 0
	}
}

// DodgeChance is the [0,1] probability a defender sidesteps an incoming basic
// attack (DodgePerDEX/DodgeCap). Skill damage isn't dodgeable.
func DodgeChance(s Stats) float64 {
	return Clamp(DodgePerDEX*float64(s.DEX), 0, DodgeCap)
}

// RollChance returns true with probability p — the shared probabilistic-check
// idiom (dice edge is `<`, not `<=`). p<=0 always false, p>=1 always true.
func RollChance(rng *rand.Rand, p float64) bool {
	if p <= 0 {
		return false
	}
	if p >= 1 {
		return true
	}
	return rng.Float64() < p
}

// RollDodge rolls a dodge for the defender; true = the basic attack misses.
func RollDodge(rng *rand.Rand, s Stats) bool {
	return RollChance(rng, DodgeChance(s))
}

// CritChance is the [0,1] crit probability: DEX-linear + timingGrades.CritBonus,
// capped at CritCap. Quality outside the table contributes no bonus.
func CritChance(s Stats, quality int) float64 {
	base := CritBaseline + CritPerDEX*float64(s.DEX)
	base += timingGrades[timingGradeAt(quality)].CritBonus
	return Clamp(base, 0, CritCap)
}

// RollCrit rolls a crit; true = multiply post-armor damage by CritMultiplier.
func RollCrit(rng *rand.Rand, s Stats, quality int) bool {
	return RollChance(rng, CritChance(s, quality))
}

// EnemyHitChance is the [0,1] chance an enemy's BASIC attack connects, rolled
// BEFORE the defend bar (a whiff skips the minigame). DEX-driven, clamped to
// [EnemyAccuracyFloor, EnemyAccuracyCap]. Enemy SKILLS are not accuracy-gated.
func EnemyHitChance(s Stats) float64 {
	return Clamp(EnemyAccuracyBaseline+EnemyAccuracyPerDEX*float64(s.DEX), EnemyAccuracyFloor, EnemyAccuracyCap)
}

// RollEnemyHit rolls whether an enemy's basic attack connects; false = a clean miss.
func RollEnemyHit(rng *rand.Rand, s Stats) bool {
	return RollChance(rng, EnemyHitChance(s))
}

// ShortenStatusDuration shaves wis/StatusShortenDivisor turns off the rolled
// duration (floor 1) — a high-WIS member shrugs enemy statuses off faster.
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

// XPForLevel returns the XP cost to advance FROM level to the next. Geometric:
// LevelXPBase × LevelXPRatio^(level-1). Returns a positive integer.
func XPForLevel(level int) int {
	if level < BaseLevel {
		level = BaseLevel
	}
	cost := float64(LevelXPBase)
	for i := BaseLevel; i < level; i++ {
		cost *= LevelXPRatio
		// Saturate before the curve overflows float64→int at absurd levels.
		if cost >= MaxLevelXPCost {
			return MaxLevelXPCost
		}
	}
	return int(cost)
}

// ProjectXP walks (level, remainder) forward by `added` XP, returning the
// resulting level, remainder, and thresholds crossed — WITHOUT mutating. The
// pure core of AddXP (the spoils screen replays it for the animated bar).
// startLvl normalizes to BaseLevel; negative `added` is treated as 0.
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

// AddXP banks `amount` XP onto a member, processing multiple level-ups, and
// returns the levels gained. No-op for amount<=0 or a dead member (HP<=0). Math
// is delegated to ProjectXP. Each level grants LevelStatPoints (PendingLevelUps)
// + LevelSkillPoints (SkillPoints).
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

// FirstPendingLevelUp returns the index of the first member with unspent stat
// points, or -1. SkillPoints live outside this gate.
func FirstPendingLevelUp(party []PartyMember) int {
	for i := range party {
		if party[i].PendingLevelUps > 0 {
			return i
		}
	}
	return -1
}

// HasUnspentPoints reports any allocation debt: PendingLevelUps OR SkillPoints.
// The single predicate behind the "+" badge etc. so UI signals stay consistent.
func HasUnspentPoints(m *PartyMember) bool {
	return m.PendingLevelUps > 0 || m.SkillPoints > 0
}

// PartyStatusKind tags the single highest-priority status a member is afflicted
// by — the single source of truth for the party card and Tome badge surfaces.
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
	// Positive statuses (Bless / Renewal) — below every threat, above Defending.
	PartyStatusBlessed
	PartyStatusRegen
	// Positive defensive wards (Aegis ShieldHP / Ice Armor) — with Bless/Regen.
	PartyStatusShielded
	PartyStatusIceArmor
	PartyStatusDefending
	// PartyStatusCount is the length-assert sentinel; new kinds slot in above it.
	PartyStatusCount
)

// PartyStatus picks the single highest-priority active status (priority is the
// partyStatusBands order). Returns PartyStatusNone if none; `turns` is the
// remaining counter (0 for boolean statuses).
func PartyStatus(m *PartyMember) (kind PartyStatusKind, turns int) {
	for _, band := range partyStatusBands {
		if active, t := band.Active(m); active {
			return band.Kind, t
		}
	}
	return PartyStatusNone, 0
}

// PartyStatusLabel returns the short uppercase label for a status kind. Pair
// with PartyStatus(m) — never branch on the kind enum at the call site.
func PartyStatusLabel(kind PartyStatusKind) string {
	for _, band := range partyStatusBands {
		if band.Kind == kind {
			return band.Label
		}
	}
	return ""
}

// partyStatusBands is the priority-ordered source for PartyStatus + PartyStatusLabel:
// the first band whose Active predicate fires wins. Adding a PartyStatusKind is
// ONE row here, asserted complete in init().
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

// Stat enumerates the six spendable level-up stats in display order; the index
// into statTable. Adding a stat = one enum constant + one statTable row.
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

// statSpec is one statTable row: label, read accessor, and in-place increment.
// Keyed by slice index (Stat's iota); init() length-asserts the 1:1 invariant.
type statSpec struct {
	Label string
	Get   func(Stats) int
	Add   func(*Stats)
	// Derive runs the level-up side effect on the WHOLE member (VIT→MaxHP,
	// INT→MP pool), called by SpendStatPoint AFTER Add. nil = no pool effect.
	Derive func(*PartyMember)
}

var statTable = []statSpec{
	StatSTR: {Label: "STR", Get: func(s Stats) int { return s.STR }, Add: func(s *Stats) { s.STR++ }},
	StatDEX: {Label: "DEX", Get: func(s Stats) int { return s.DEX }, Add: func(s *Stats) { s.DEX++ }},
	StatINT: {Label: "INT", Get: func(s Stats) int { return s.INT }, Add: func(s *Stats) { s.INT++ },
		// INT grows MaxMP and tops off MP by the same delta.
		Derive: func(m *PartyMember) { m.MaxMP += MPPerINT; GainUpTo(&m.MP, m.MaxMP, MPPerINT) }},
	StatWIS: {Label: "WIS", Get: func(s Stats) int { return s.WIS }, Add: func(s *Stats) { s.WIS++ }},
	StatVIT: {Label: "VIT", Get: func(s Stats) int { return s.VIT }, Add: func(s *Stats) { s.VIT++ },
		// VIT recomputes MaxHP authoritatively and heals by the per-point delta.
		Derive: func(m *PartyMember) { m.MaxHP = MaxHPFor(m.Stats); GainUpTo(&m.HP, m.MaxHP, HPPerVIT) }},
	StatSPD: {Label: "SPD", Get: func(s Stats) int { return s.SPD }, Add: func(s *Stats) { s.SPD++ }},
}

// statSetters is the absolute-write half of statTable (kept separate so readers
// using only Get/Add aren't touched).
var statSetters = []func(*Stats, int){
	StatSTR: func(s *Stats, v int) { s.STR = v },
	StatDEX: func(s *Stats, v int) { s.DEX = v },
	StatINT: func(s *Stats, v int) { s.INT = v },
	StatWIS: func(s *Stats, v int) { s.WIS = v },
	StatVIT: func(s *Stats, v int) { s.VIT = v },
	StatSPD: func(s *Stats, v int) { s.SPD = v },
}

// SumStats returns the per-field sum of two stat blocks (no clamping; the
// floor-at-0 guard lives at the EffectiveStats fold site).
func SumStats(a, b Stats) Stats {
	// Hand-unrolled rather than the statTable enum loop: hottest combat path,
	// where func-pointer indirection can't inline. A new Stat field needs a line here.
	return Stats{
		STR: a.STR + b.STR,
		DEX: a.DEX + b.DEX,
		INT: a.INT + b.INT,
		WIS: a.WIS + b.WIS,
		VIT: a.VIT + b.VIT,
		SPD: a.SPD + b.SPD,
	}
}

// Total returns the sum of the six stat fields — the single home for the
// "add up every stat point" fold.
func (s Stats) Total() int {
	return s.STR + s.DEX + s.INT + s.WIS + s.VIT + s.SPD
}

// applyStatusMod inserts or refreshes a mod keyed by Source (same skill REPLACES,
// never double-stacks; new source appends). Non-positive Turns is a no-op. Shared
// by StampPartyBuff / StampEnemyDebuff — assign the returned slice back.
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
// bundle — the shared read behind both the party and enemy Effective* helpers.
func SumStatusMods(mods []StatusMod) (stats Stats, armor, mdef int) {
	for _, m := range mods {
		stats = SumStats(stats, m.Stats)
		armor += m.Armor
		mdef += m.MDef
	}
	return stats, armor, mdef
}

// addStatsFloored returns base + delta per-stat, each floored at 0 so a negative
// delta (debuff) can't drive a stat below zero into the combat math. The single
// fold behind both the party and enemy Effective* buff layers.
func addStatsFloored(base, delta Stats) Stats {
	// Hand-unrolled (see SumStats) — same hot-path rationale.
	return Stats{
		STR: floorInt(base.STR + delta.STR),
		DEX: floorInt(base.DEX + delta.DEX),
		INT: floorInt(base.INT + delta.INT),
		WIS: floorInt(base.WIS + delta.WIS),
		VIT: floorInt(base.VIT + delta.VIT),
		SPD: floorInt(base.SPD + delta.SPD),
	}
}

// floorInt clamps a stat fold result up to 0 (a debuff can't drive it negative).
func floorInt(v int) int {
	if v < 0 {
		return 0
	}
	return v
}

// TickStatusMods decrements every mod's Turns and drops expired ones, returning
// the survivors (a fresh slice) plus the just-expired Sources (for "X fades").
// Does NOT alias `mods`, so a caller that retains the original sees it unchanged.
func TickStatusMods(mods []StatusMod) (remaining []StatusMod, expired []SkillID) {
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

// StatusModsNetBeneficial reports whether a StatusMod slice's summed numeric
// effect is net-positive — gates the "Blessed" pill so a net-negative self-mod
// doesn't mislabel as a buff.
func StatusModsNetBeneficial(mods []StatusMod) bool {
	stats, armor, mdef := SumStatusMods(mods)
	sum := stats.Total() + armor + mdef
	return sum > 0
}

// StampPartyBuff adds/refreshes one skill's buff (BuffStats/Armor/MDef for
// BuffTurns) on a member. STACKS with other skills; re-casting the SAME skill
// refreshes. No-ops (false) for nil member or non-buff effect (BuffTurns 0).
func StampPartyBuff(m *PartyMember, source SkillID, e SkillEffect) bool {
	if m == nil || e.BuffTurns <= 0 {
		return false
	}
	m.Buffs = applyStatusMod(m.Buffs, StatusMod{Source: source, Stats: e.BuffStats, Armor: e.BuffArmor, MDef: e.BuffMDef, Turns: e.BuffTurns})
	return true
}

// AdjustStat applies delta to the named stat, clamping at zero (custom-enemy
// editor's +/- buttons).
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

// DebugBoostParty adds `amount` to every base stat of every member, refreshes
// derived pools (MaxHP/MaxMP) like the level-up Derive hooks, and tops HP/MP off
// (reviving the downed — god-mode test button).
func DebugBoostParty(party []PartyMember, amount int) {
	for i := range party {
		m := &party[i]
		// Grow MaxMP by the ACTUAL applied INT delta, not raw amount: AdjustStat
		// floors at 0, so a negative boost can change INT by less than `amount`.
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

// statDescriptions is the per-stat level-up modal one-liner (kept next to statTable).
var statDescriptions = []string{
	StatSTR: "Melee damage & hit chance",
	StatDEX: "Dodge, Crit, Ranged hit",
	StatINT: fmt.Sprintf("Magic damage & MP (+%d MP per point)", MPPerINT),
	StatWIS: "Heal, Magic defense, Status resist",
	StatVIT: fmt.Sprintf("Max HP (+%d per point)", HPPerVIT),
	StatSPD: "Turn frequency (act more often)",
}

// StatDescription returns the level-up modal blurb for a stat (out-of-range = "").
func StatDescription(s Stat) string {
	if s < 0 || int(s) >= len(statDescriptions) {
		return ""
	}
	return statDescriptions[s]
}

// StatPreviewLine returns the "what this spend buys" string shown in place of
// StatDescription when ≥1 point is staged: projected post-spend derived values.
// Returns "" when pending<=0 or stat out-of-range (renderer falls through).
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
		// STR always drives melee DAMAGE; it drives to-hit only when STR is the
		// weapon accuracy stat — don't preview a Hit gain that won't materialize.
		line := fmt.Sprintf("Melee %d→%d", MeleeDamage(current, 0), MeleeDamage(after, 0))
		if accuracyStat == StatSTR {
			h0 := MeleeAccuracy(current, TimingQualityMiss) * 100
			h1 := MeleeAccuracy(after, TimingQualityMiss) * 100
			line += fmt.Sprintf("  Hit %.0f→%.0f%%", h0, h1)
		}
		return line
	case StatDEX:
		// DEX's active effects: dodge + crit (ranged hit is dormant, left off).
		d0 := DodgeChance(current) * 100
		d1 := DodgeChance(after) * 100
		c0 := CritChance(current, TimingQualityMiss) * 100
		c1 := CritChance(after, TimingQualityMiss) * 100
		return fmt.Sprintf("Dodge %.0f→%.0f%%  Crit %.0f→%.0f%%", d0, d1, c0, c1)
	case StatINT:
		return fmt.Sprintf("Magic %d→%d  MaxMP +%d", MagicDamage(current, 0), MagicDamage(after, 0), MPForINTDelta(after.INT-current.INT))
	case StatWIS:
		return fmt.Sprintf("Heal %d→%d  MDef %d→%d", HealAmount(current, 0), HealAmount(after, 0), MagicDefense(current), MagicDefense(after))
	case StatVIT:
		return fmt.Sprintf("MaxHP %d → %d", MaxHPFor(current), MaxHPFor(after))
	case StatSPD:
		return fmt.Sprintf("SPD %d → %d (more turns)", current.SPD, after.SPD)
	default:
		// init() calls this for every Stat, so a missing case panics at STARTUP.
		panic("core: StatPreviewLine missing case for stat")
	}
}

// CommitLevelUp applies the staged stat-point spend via SpendStatPoint, returning
// true if any point landed. Skill points are NOT spent here.
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
	// StatPreviewLine's per-stat switch is the one parallel table the asserts
	// above can't cover — force coverage so a missing case panics at STARTUP.
	var probe Stats
	for i := Stat(0); i < StatCount; i++ {
		if StatPreviewLine(i, probe, 1, StatSTR) == "" {
			panic(fmt.Sprintf("core: StatPreviewLine returned empty for stat index %d — add a preview case", int(i)))
		}
	}
	// Assert every PartyStatusKind has a partyStatusBands row (skipping
	// PartyStatusNone), so a missed kind panics at STARTUP instead of never surfacing.
	for k := PartyStatusNone + 1; k < PartyStatusCount; k++ {
		if PartyStatusLabel(k) == "" {
			panic(fmt.Sprintf("core: partyStatusBands missing kind %d — add a row", int(k)))
		}
	}
}

// StatLabel returns the 3-letter display label for a stat.
func StatLabel(s Stat) string {
	if s < 0 || int(s) >= len(statTable) {
		return "?"
	}
	return statTable[s].Label
}

// StatValue reads the named stat from a Stats block.
func StatValue(s Stats, st Stat) int {
	if st < 0 || int(st) >= len(statTable) {
		return 0
	}
	return statTable[st].Get(s)
}

// SpendStatPoint moves one PendingLevelUps point into the named stat. False when
// no points left or stat unknown. Runs the stat's Derive pool side effect.
func SpendStatPoint(m *PartyMember, stat Stat) bool {
	if m == nil || m.PendingLevelUps <= 0 {
		return false
	}
	if stat < 0 || int(stat) >= len(statTable) {
		return false
	}
	statTable[stat].Add(&m.Stats)
	if derive := statTable[stat].Derive; derive != nil {
		derive(m)
	}
	m.PendingLevelUps--
	return true
}

// PoisonEffect describes parameters for inflicting / ticking Poison. A future
// poison source (trap, item) can build its own with different numbers.
type PoisonEffect struct {
	MinTurns   int
	MaxTurns   int
	TickDamage int
}

// DefaultPoisonEffect is the canonical Diseased Rat poison (PoisonMin/MaxTurns,
// PoisonTickDamage per turn).
var DefaultPoisonEffect = PoisonEffect{
	MinTurns:   PoisonMinTurns,
	MaxTurns:   PoisonMaxTurns,
	TickDamage: PoisonTickDamage,
}

// RollDuration picks a uniform duration in [MinTurns, MaxTurns] (via rollDuration).
func (p PoisonEffect) RollDuration(rng *rand.Rand) int {
	return rollDuration(rng, p.MinTurns, p.MaxTurns)
}

// TickPoisonStep applies one poison tick to every poisoned, alive member and
// decrements the counter. Called after each exploration step. Returns the number
// of members hit.
//
// Three poison-tick paths exist, deliberately diverging in context: this one
// (out-of-battle, per tile-step, no combat log); battle.tickPoisonAfterPartyTurn
// (end of the member's turn, emits log lines); battle.tickPoisonForIngestedParty
// (start of round for ingested members — without it, ingest pauses the DoT). All
// drain PoisonTurns, deal DefaultPoisonEffect.TickDamage, and bypass armor.
func TickPoisonStep(g *GameState) int {
	ticks := 0
	for i := range g.Party {
		m := &g.Party[i]
		if m.HP <= 0 || m.PoisonTurns <= 0 {
			continue
		}
		m.PoisonTurns--
		// Bypasses armor (poison is magical decay). Out-of-battle wake rule is
		// poison-specific: only a lethal tick wakes a sleeper (unlike in-battle).
		if ApplyFlatDamage(&m.HP, &m.DamageFlash, DefaultPoisonEffect.TickDamage) {
			// Lethal tick: scrub every combat-only status off the corpse.
			ClearMemberTransientStatuses(m)
		}
		ticks++
	}
	return ticks
}

package core

import "reflect"

// Per-skill upgrade ladder; folds tier modifiers into the SkillEffect battle code applies.
// Driven by skilltrees.go: BuySkillNode writes PartyMember.SkillTiers (rank 1 = tier 0/base,
// each further rank = next tier); EffectiveSkillEffect / SkillDamageFor / SkillHealFor /
// SkillTierMod fold the deltas into casts. Spent only through the tree modal.
// Each tier is ONE numeric/bool delta on an existing SkillEffect field or a tier-only field below.

// MaxSkillTier is the maximum purchasable tier per skill. Every system reads this constant.
const MaxSkillTier = 3

// SkillTierUpgradeCost is the SkillPoint price of every tier-upgrade rung (flat per tier).
const SkillTierUpgradeCost = 1

// SkillTierUpgrade is one purchasable rung of a skill's tree. Cost is in SkillPoints;
// Effect is the additive delta applied to the base SkillEffect on purchase.
type SkillTierUpgrade struct {
	Tier        int
	Label       string
	Description string
	Cost        int
	Effect      SkillEffectDelta
}

// SkillEffectDelta is the additive modifier applied per purchased tier. Mirrors SkillEffect's
// shape — fields are added (bool flags OR'd) into the base via addSkillEffectDelta. Not embedded
// because both are built with keyed composite literals in ~80 sites (promoted fields can't be set
// in an outer keyed literal). The init-time reflection assert below pins the two structs + the
// fold against drift.
type SkillEffectDelta struct {
	Damage         int
	Heal           int
	StealChance    float64
	BurnChance     float64
	BurnMinTurns   int
	BurnMaxTurns   int
	PoisonChance   float64
	PoisonMinTurns int
	PoisonMaxTurns int
	BleedChance    float64
	BleedMinTurns  int
	BleedMaxTurns  int
	StunChance     float64
	StunMinTurns   int
	StunMaxTurns   int
	SleepMinTurns  int
	SleepMaxTurns  int
	// StealBonusDamage: STR-multiplier damage on a successful steal (Thief Steal T3). Tier-only.
	StealBonusDamage int
	// CritDoubleOnExcellent: turns an Excellent timing hit into a double-damage crit. Tier-only.
	CritDoubleOnExcellent bool
	// BuffStats / BuffTurns: buff-skill magnitude/duration deltas (Bless).
	BuffStats      Stats
	BuffTurns      int
	RegenTurns     int // heal-over-time duration delta (Renewal)
	ArmorReduction int // Corrosive Vial armor-strip delta
	ATBPush        int // Sunder readiness-shove delta
	// BuffArmor / BuffMDef: Stone Skin ward deltas.
	BuffArmor        int
	BuffMDef         int
	ShieldHP         int     // Aegis absorb-pool delta
	IceArmorTurns    int     // Ice Armor duration delta
	PercentCurrentHP float64 // Static Field %-current-HP delta
}

// tier builds one SkillTierUpgrade row at the flat SkillTierUpgradeCost.
func tier(t int, label, description string, effect SkillEffectDelta) SkillTierUpgrade {
	return SkillTierUpgrade{Tier: t, Label: label, Description: description, Cost: SkillTierUpgradeCost, Effect: effect}
}

// skillTierTable is the source of truth for every player-castable skill's upgrade ladder
// (MaxSkillTier rows each, in tier order). The init guard below asserts the row count.
var skillTierTable = map[SkillID][]SkillTierUpgrade{
	// ── Warrior ──────────────────────────────────────────────
	SkillSwipe: {
		tier(1, "+2 damage", "+2 base damage to every hit in the cleave.", SkillEffectDelta{Damage: 2}),
		tier(2, "+2 damage", "+2 more base damage to the whole cleave.", SkillEffectDelta{Damage: 2}),
		tier(3, "+2 damage", "Another +2 base damage. Whole pack feels it.", SkillEffectDelta{Damage: 2}),
	},
	SkillCrushingBlow: {
		tier(1, "+3 damage", "+3 base damage on the heavy hit.", SkillEffectDelta{Damage: 3}),
		tier(2, "+15% stun", "Stun roll gets +15% chance on a landed Great/Excellent.", SkillEffectDelta{StunChance: CrushingBlowTierStunBump}),
		tier(3, "Excellent crits", "An Excellent timing hit deals double damage.", SkillEffectDelta{CritDoubleOnExcellent: true}),
	},
	SkillWhirlwind: {
		tier(1, "+2 damage", "+2 base damage per target on the spin.", SkillEffectDelta{Damage: 2}),
		tier(2, "+2 damage", "+2 more base damage per target on the spin.", SkillEffectDelta{Damage: 2}),
		tier(3, "+2 damage", "Another +2 base damage. Excellent timing eviscerates the pack.", SkillEffectDelta{Damage: 2}),
	},
	SkillSecondWind: {
		tier(1, "+3 heal", "+3 to the flat self-heal.", SkillEffectDelta{Heal: 3}),
		tier(2, "+3 heal", "Another +3 to the breather.", SkillEffectDelta{Heal: 3}),
		tier(3, "+3 heal", "A third +3 — a maxed Second Wind is a real comeback.", SkillEffectDelta{Heal: 3}),
	},
	// ── Cleric ───────────────────────────────────────────────
	SkillPrayer: {
		tier(1, "+3 heal", "+3 base heal on the target.", SkillEffectDelta{Heal: 3}),
		tier(2, "+3 heal", "Another +3 heal. Tank-grade recovery in one cast.", SkillEffectDelta{Heal: 3}),
		tier(3, "+3 heal", "A third +3 heal — Prayer alone can top off a tank.", SkillEffectDelta{Heal: 3}),
	},
	SkillMassMend: {
		tier(1, "+2 heal", "+2 base heal across every alive party member.", SkillEffectDelta{Heal: 2}),
		tier(2, "+2 heal", "Another +2 heal across the whole party.", SkillEffectDelta{Heal: 2}),
		tier(3, "+2 heal", "A third +2 heal — full-party sustain in one cast.", SkillEffectDelta{Heal: 2}),
	},
	SkillSmite: {
		tier(1, "+2 damage", "+2 base damage on the press tap.", SkillEffectDelta{Damage: 2}),
		tier(2, "+2 damage", "Another +2 base damage.", SkillEffectDelta{Damage: 2}),
		tier(3, "+25% stun", "Lands a Stun roll with 25% chance on Great/Excellent timing.", SkillEffectDelta{StunChance: SmiteTierStunChance, StunMinTurns: StatusTurnStep, StunMaxTurns: StatusTurnStep}),
	},
	SkillBless: {
		tier(1, "+1 turn", "The blessing lingers one turn longer on the whole party.", SkillEffectDelta{BuffTurns: 1}),
		tier(2, "+1 to blessed stats", "+1 more to STR, DEX, INT and WIS for every blessed ally.", SkillEffectDelta{BuffStats: Stats{STR: 1, DEX: 1, INT: 1, WIS: 1}}),
		tier(3, "+1 to blessed stats", "Another +1 to all four blessed stats — a maxed blessing is a sweeping party buff.", SkillEffectDelta{BuffStats: Stats{STR: 1, DEX: 1, INT: 1, WIS: 1}}),
	},
	SkillRenewal: {
		tier(1, "+1 turn", "The regen ticks one more turn.", SkillEffectDelta{RegenTurns: 1}),
		tier(2, "+1 heal/turn", "+1 to the per-turn heal (before WIS scaling).", SkillEffectDelta{Heal: 1}),
		tier(3, "+1 turn", "Another turn of regen — a maxed Renewal sustains an ally for the whole fight.", SkillEffectDelta{RegenTurns: 1}),
	},
	// ── Thief ────────────────────────────────────────────────
	SkillSteal: {
		tier(1, "+15% chance", "Steal succeeds 15% more often.", SkillEffectDelta{StealChance: StealTierChanceBump}),
		tier(2, "+15% chance", "Another +15% steal chance.", SkillEffectDelta{StealChance: StealTierChanceBump}),
		tier(3, "Cuts on lift", "A successful steal also deals STR damage.", SkillEffectDelta{StealBonusDamage: 1}),
	},
	SkillBackstab: {
		tier(1, "+2 damage", "+2 base damage on the dagger thrust.", SkillEffectDelta{Damage: 2}),
		tier(2, "Excellent crits harder", "Excellent timing's existing double-damage stacks an additional damage tier.", SkillEffectDelta{CritDoubleOnExcellent: true}),
		tier(3, "+3 damage", "Another +3 base damage. Backstab carries.", SkillEffectDelta{Damage: 3}),
	},
	SkillVenomStrike: {
		tier(1, "+15% Poison", "Poison-apply chance bumped by 15%.", SkillEffectDelta{PoisonChance: VenomStrikeTierPoisonBump}),
		tier(2, "+1 Poison turn", "Poison's max-roll duration extends by one turn.", SkillEffectDelta{PoisonMaxTurns: 1}),
		tier(3, "+2 damage", "+2 base damage on the strike itself.", SkillEffectDelta{Damage: 2}),
	},
	SkillCripple: {
		tier(1, "+1 turn", "The cripple lingers one of the target's turns longer.", SkillEffectDelta{BuffTurns: 1}),
		tier(2, "-1 more SPD", "Saps another point of SPD while it lasts.", SkillEffectDelta{BuffStats: Stats{SPD: -1}}),
		tier(3, "-1 more SPD", "Another point of SPD — a maxed Cripple drags a slow foe to a crawl.", SkillEffectDelta{BuffStats: Stats{SPD: -1}}),
	},
	SkillCorrosiveVial: {
		tier(1, "+2 Armor break", "Strips 2 more Armor per vial.", SkillEffectDelta{ArmorReduction: 2}),
		tier(2, "+2 Armor break", "Another +2 Armor stripped.", SkillEffectDelta{ArmorReduction: 2}),
		tier(3, "+3 Armor break", "A maxed vial melts even a heavy carapace.", SkillEffectDelta{ArmorReduction: 3}),
	},
	SkillPoisonCloud: {
		tier(1, "+15% Poison", "Every enemy in the cloud rolls a 15% higher Poison chance.", SkillEffectDelta{PoisonChance: PoisonCloudTierPoisonBump}),
		tier(2, "+1 Poison turn", "The cloud's Poison lingers one extra turn on its max roll.", SkillEffectDelta{PoisonMaxTurns: 1}),
		tier(3, "+1 damage", "+1 base damage to every enemy caught in the cloud.", SkillEffectDelta{Damage: 1}),
	},
	// ── Wizard ───────────────────────────────────────────────
	SkillFirebolt: {
		tier(1, "+2 damage", "+2 base damage on the bolt.", SkillEffectDelta{Damage: 2}),
		tier(2, "+20% Burn", "Burn-apply chance bumped by 20%.", SkillEffectDelta{BurnChance: FireTierBurnChanceBump}),
		tier(3, "+1 Burn turn", "Burn lasts one turn longer on min and max rolls.", SkillEffectDelta{BurnMinTurns: StatusTurnStep, BurnMaxTurns: StatusTurnStep}),
	},
	SkillFrostLance: {
		tier(1, "+2 damage", "+2 base damage on the lance.", SkillEffectDelta{Damage: 2}),
		tier(2, "+15% Stun", "Stun roll gets +15% chance on Great/Excellent.", SkillEffectDelta{StunChance: FrostLanceTierStunChance}),
		tier(3, "+1 Stun turn", "Stun lasts an extra turn when it lands.", SkillEffectDelta{StunMinTurns: StatusTurnStep, StunMaxTurns: StatusTurnStep}),
	},
	SkillFrostbite: {
		tier(1, "+2 damage", "+2 base frost damage.", SkillEffectDelta{Damage: 2}),
		tier(2, "+1 chill turn", "The chill lingers one of the target's turns longer.", SkillEffectDelta{BuffTurns: 1}),
		tier(3, "-1 more SPD", "Saps another point of SPD — a maxed Frostbite nearly freezes its mark.", SkillEffectDelta{BuffStats: Stats{SPD: -1}}),
	},
	SkillConeOfCold: {
		tier(1, "+1 damage", "+1 base frost damage to every enemy in the cone.", SkillEffectDelta{Damage: 1}),
		tier(2, "+1 chill turn", "The pack-wide chill lasts one extra turn.", SkillEffectDelta{BuffTurns: 1}),
		tier(3, "-1 more SPD", "Saps another point of SPD from every chilled foe.", SkillEffectDelta{BuffStats: Stats{SPD: -1}}),
	},
	SkillArcBolt: {
		tier(1, "+1 damage", "+1 base damage per arc target.", SkillEffectDelta{Damage: 1}),
		tier(2, "+1 damage", "Another +1 damage per target.", SkillEffectDelta{Damage: 1}),
		tier(3, "+15% Burn", "Every arc target rolls a 15% burn chance.", SkillEffectDelta{BurnChance: ArcBoltBurnChance, BurnMinTurns: ArcBoltBurnMinTurns, BurnMaxTurns: ArcBoltBurnMaxTurns}),
	},
	SkillFireball: {
		tier(1, "+2 damage", "+2 base magic damage to every enemy in the blast.", SkillEffectDelta{Damage: 2}),
		tier(2, "+20% Burn", "Per-target Burn-apply chance bumped by 20%.", SkillEffectDelta{BurnChance: FireTierBurnChanceBump}),
		tier(3, "+1 Burn turn", "Burn lasts one turn longer on min and max rolls.", SkillEffectDelta{BurnMinTurns: StatusTurnStep, BurnMaxTurns: StatusTurnStep}),
	},
	// ── Warrior (tree-node skills) ───────────────────────────
	SkillSunder: {
		tier(1, "+2 damage", "+2 base damage on the sundering blow.", SkillEffectDelta{Damage: 2}),
		tier(2, "+2 damage", "Another +2 base damage.", SkillEffectDelta{Damage: 2}),
		tier(3, "Harder shove", "Knocks the target's turn even further back.", SkillEffectDelta{ATBPush: SunderATBPushPerTier}),
	},
	SkillWarBanner: {
		tier(1, "+1 turn", "The banner stands one turn longer.", SkillEffectDelta{BuffTurns: 1}),
		tier(2, "+1 STR/Armor", "+1 more to the rallied STR and Armor for every ally.", SkillEffectDelta{BuffStats: Stats{STR: 1}, BuffArmor: 1}),
		tier(3, "+1 STR/Armor", "Another +1 to both — a maxed banner is a sweeping party buff.", SkillEffectDelta{BuffStats: Stats{STR: 1}, BuffArmor: 1}),
	},
	SkillStoneSkin: {
		tier(1, "+2 Armor", "+2 more Armor on the ward.", SkillEffectDelta{BuffArmor: 2}),
		tier(2, "+2 MDef", "+2 more MDef on the ward.", SkillEffectDelta{BuffMDef: 2}),
		tier(3, "+1 turn", "The ward lasts one of the ally's turns longer.", SkillEffectDelta{BuffTurns: 1}),
	},
	// ── Cleric (tree-node skills) ────────────────────────────
	SkillBlind: {
		tier(1, "+1 turn", "The blindness lingers one of the target's turns longer.", SkillEffectDelta{BuffTurns: 1}),
		tier(2, "-1 more DEX", "Saps another point of accuracy while it lasts.", SkillEffectDelta{BuffStats: Stats{DEX: -1}}),
		tier(3, "-1 more DEX", "Another point of accuracy — a maxed Blind nearly closes a foe's eyes.", SkillEffectDelta{BuffStats: Stats{DEX: -1}}),
	},
	SkillAegis: {
		tier(1, "+4 shield", "+4 to the absorb pool.", SkillEffectDelta{ShieldHP: 4}),
		tier(2, "+4 shield", "Another +4 absorb.", SkillEffectDelta{ShieldHP: 4}),
		tier(3, "+6 shield", "A maxed Aegis turns aside a heavy blow outright.", SkillEffectDelta{ShieldHP: 6}),
	},
	// ── Thief (tree-node skills) ─────────────────────────────
	SkillSmokeBomb: {
		tier(1, "+1 turn", "The smoke hangs one turn longer.", SkillEffectDelta{BuffTurns: 1}),
		tier(2, "+1 DEX swing", "Another point of party evasion and enemy accuracy loss.", SkillEffectDelta{BuffStats: Stats{DEX: 1}}),
		tier(3, "+1 DEX swing", "A maxed Smoke Bomb blinds the room and slips the party clear.", SkillEffectDelta{BuffStats: Stats{DEX: 1}}),
	},
	// ── Wizard (tree-node skills) ────────────────────────────
	SkillIceArmor: {
		tier(1, "+1 turn", "The frost ward stands one turn longer.", SkillEffectDelta{IceArmorTurns: 1}),
		tier(2, "+1 turn", "Another turn of ward.", SkillEffectDelta{IceArmorTurns: 1}),
		tier(3, "+2 turns", "A maxed Ice Armor sheathes the caster for most of a fight.", SkillEffectDelta{IceArmorTurns: 2}),
	},
	// ── Bleed strikes (Warrior Rend / Thief Lacerate) ────────
	SkillRend: {
		tier(1, "+2 damage", "+2 base damage on the opening cut.", SkillEffectDelta{Damage: 2}),
		tier(2, "+1 Bleed turn", "The wound bleeds one turn longer on min and max rolls.", SkillEffectDelta{BleedMinTurns: StatusTurnStep, BleedMaxTurns: StatusTurnStep}),
		tier(3, "+15% Bleed", "Bleed-apply chance bumped by 15% — a maxed Rend almost always draws blood.", SkillEffectDelta{BleedChance: BleedStrikeTierChanceBump}),
	},
	SkillLacerate: {
		tier(1, "+1 damage", "+1 base damage on the cut.", SkillEffectDelta{Damage: 1}),
		tier(2, "+1 Bleed turn", "The wound bleeds one turn longer on min and max rolls.", SkillEffectDelta{BleedMinTurns: StatusTurnStep, BleedMaxTurns: StatusTurnStep}),
		tier(3, "+15% Bleed", "Bleed-apply chance bumped by 15%.", SkillEffectDelta{BleedChance: BleedStrikeTierChanceBump}),
	},
	// ── Radiance / Pyromancy / Storm / Cutpurse (tree-node skills) ───
	SkillSearingLight: {
		tier(1, "+2 damage", "+2 base radiant damage on the sear.", SkillEffectDelta{Damage: 2}),
		tier(2, "+1 Burn turn", "The radiant Burn lingers one turn longer on min and max rolls.", SkillEffectDelta{BurnMinTurns: StatusTurnStep, BurnMaxTurns: StatusTurnStep}),
		tier(3, "+15% Burn", "Burn-apply chance bumped by 15% — a maxed Searing Light almost always ignites.", SkillEffectDelta{BurnChance: SearingLightTierBurnBump}),
	},
	SkillImmolate: {
		tier(1, "+2 damage", "+2 base fire damage to every enemy in the zone.", SkillEffectDelta{Damage: 2}),
		tier(2, "+1 Burn turn", "Every enemy's Burn lasts one turn longer.", SkillEffectDelta{BurnMinTurns: StatusTurnStep, BurnMaxTurns: StatusTurnStep}),
		tier(3, "+1 Burn turn", "Another turn of Burn — a maxed Immolate roasts the whole pack for the fight.", SkillEffectDelta{BurnMinTurns: StatusTurnStep, BurnMaxTurns: StatusTurnStep}),
	},
	SkillMug: {
		tier(1, "+2 damage", "+2 base damage on the strike.", SkillEffectDelta{Damage: 2}),
		tier(2, "+15% lift", "Mug's steal succeeds 15% more often.", SkillEffectDelta{StealChance: StealTierChanceBump}),
		tier(3, "+2 damage", "Another +2 base damage. A maxed Mug hits and lifts.", SkillEffectDelta{Damage: 2}),
	},
	SkillChainLightning: {
		tier(1, "+1 damage", "+1 base damage per arc target.", SkillEffectDelta{Damage: 1}),
		tier(2, "+1 damage", "Another +1 damage per target.", SkillEffectDelta{Damage: 1}),
		tier(3, "+15% Stun", "Every arc target rolls a 15% higher Stun chance.", SkillEffectDelta{StunChance: ChainLightningTierStunBump}),
	},
	SkillStaticField: {
		tier(1, "+6% HP", "Saps another 6% of the target's current HP.", SkillEffectDelta{PercentCurrentHP: StaticFieldPercentPerTier}),
		tier(2, "+6% HP", "Another +6% of current HP.", SkillEffectDelta{PercentCurrentHP: StaticFieldPercentPerTier}),
		tier(3, "+6% HP", "A maxed Static Field tears away a third of a healthy foe's HP.", SkillEffectDelta{PercentCurrentHP: StaticFieldPercentPerTier}),
	},
	SkillConsecrate: {
		tier(1, "+2 damage", "+2 base radiant damage to every enemy.", SkillEffectDelta{Damage: 2}),
		tier(2, "+2 damage", "Another +2 base damage across the pack.", SkillEffectDelta{Damage: 2}),
		tier(3, "+2 damage", "A third +2 — maxed Consecrate scours the whole field.", SkillEffectDelta{Damage: 2}),
	},
	SkillRecklessSwing: {
		tier(1, "+3 damage", "+3 base damage on the wild swing.", SkillEffectDelta{Damage: 3}),
		tier(2, "+3 damage", "Another +3 base damage.", SkillEffectDelta{Damage: 3}),
		tier(3, "+3 damage", "A third +3 — a maxed Reckless Swing hits like a truck (mind the open guard).", SkillEffectDelta{Damage: 3}),
	},
	SkillCombust: {
		tier(1, "+2 base", "+2 base magic damage on the detonation.", SkillEffectDelta{Damage: 2}),
		tier(2, "+2 base", "Another +2 base damage.", SkillEffectDelta{Damage: 2}),
		tier(3, "+3 base", "A maxed Combust turns a long Burn into a devastating blast.", SkillEffectDelta{Damage: 3}),
	},
}

// init asserts every player-castable skill has exactly MaxSkillTier rows in
// skillTierTable with tier indices 1..MaxSkillTier in order. Drift panics at start.
func init() {
	for _, s := range PlayerCastableSkills() {
		// NoUpgrades skill (e.g. Scan): no ladder, must carry no stray rows; exempt from the count check.
		if SkillHasNoUpgrades(s) {
			if _, ok := skillTierTable[s]; ok {
				panic("core: skill " + SkillName(s) + " is NoUpgrades but has skillTierTable rows — drop the rows or the flag")
			}
			continue
		}
		rows, ok := skillTierTable[s]
		if !ok {
			panic("core: PlayerCastable skill " + SkillName(s) + " has no skillTierTable entry — add MaxSkillTier upgrade rows")
		}
		if len(rows) != MaxSkillTier {
			panic("core: skillTierTable for " + SkillName(s) + " must have exactly MaxSkillTier rows")
		}
		for i, row := range rows {
			if row.Tier != i+1 {
				panic("core: skillTierTable for " + SkillName(s) + " has out-of-order Tier values")
			}
		}
	}
	for s := range skillTierTable {
		if !SkillPlayerCastable(s) {
			panic("core: skillTierTable has entry for " + SkillName(s) + " which is not PlayerCastable — drop the row or flip the flag")
		}
	}
}

// SkillTierOf returns the member's purchased tier for the skill (0..MaxSkillTier). Nil-safe.
func SkillTierOf(m *PartyMember, s SkillID) int {
	if m == nil || m.SkillTiers == nil {
		return 0
	}
	return Clamp(m.SkillTiers[s], 0, MaxSkillTier)
}

// skillTierUpgradeFor returns the upgrade for a skill's tier (1..MaxSkillTier); ok=false if out of range.
func skillTierUpgradeFor(s SkillID, tier int) (SkillTierUpgrade, bool) {
	if tier < 1 || tier > MaxSkillTier {
		return SkillTierUpgrade{}, false
	}
	rows, ok := skillTierTable[s]
	if !ok || tier-1 >= len(rows) {
		return SkillTierUpgrade{}, false
	}
	return rows[tier-1], true
}

// EffectiveSkillEffect returns the base SkillEffect for s with every purchased tier's delta stacked
// in (numeric added, bool OR'd). The source of truth for combat math; SkillEffectFor gives the
// un-modified base for editor/validation use. Nil member returns the base unchanged (tier 0).
func EffectiveSkillEffect(m *PartyMember, s SkillID) SkillEffect {
	eff := SkillEffectFor(s)
	if m == nil {
		return eff
	}
	forEachPurchasedTier(m, s, func(up SkillTierUpgrade) {
		addSkillEffectDelta(&eff, up.Effect)
	})
	return eff
}

// forEachPurchasedTier invokes fn with each purchased tier upgrade for skill s, walking tiers
// 1..SkillTierOf and stopping at the first missing row. Shared by EffectiveSkillEffect/SkillTierMod.
func forEachPurchasedTier(m *PartyMember, s SkillID, fn func(SkillTierUpgrade)) {
	tier := SkillTierOf(m, s)
	for i := 1; i <= tier; i++ {
		up, ok := skillTierUpgradeFor(s, i)
		if !ok {
			break
		}
		fn(up)
	}
}

// addSkillEffectDelta folds one tier's delta into the accumulating effect, field by field.
func addSkillEffectDelta(eff *SkillEffect, d SkillEffectDelta) {
	eff.Damage += d.Damage
	eff.Heal += d.Heal
	eff.StealChance += d.StealChance
	eff.BurnChance += d.BurnChance
	eff.BurnMinTurns += d.BurnMinTurns
	eff.BurnMaxTurns += d.BurnMaxTurns
	eff.PoisonChance += d.PoisonChance
	eff.PoisonMinTurns += d.PoisonMinTurns
	eff.PoisonMaxTurns += d.PoisonMaxTurns
	eff.BleedChance += d.BleedChance
	eff.BleedMinTurns += d.BleedMinTurns
	eff.BleedMaxTurns += d.BleedMaxTurns
	eff.StunChance += d.StunChance
	eff.StunMinTurns += d.StunMinTurns
	eff.StunMaxTurns += d.StunMaxTurns
	eff.SleepMinTurns += d.SleepMinTurns
	eff.SleepMaxTurns += d.SleepMaxTurns
	eff.BuffStats = SumStats(eff.BuffStats, d.BuffStats)
	eff.BuffTurns += d.BuffTurns
	eff.RegenTurns += d.RegenTurns
	eff.ArmorReduction += d.ArmorReduction
	eff.ATBPush += d.ATBPush
	eff.BuffArmor += d.BuffArmor
	eff.BuffMDef += d.BuffMDef
	eff.ShieldHP += d.ShieldHP
	eff.IceArmorTurns += d.IceArmorTurns
	eff.PercentCurrentHP += d.PercentCurrentHP
}

// deltaTierOnlyFields are SkillEffectDelta fields with NO matching SkillEffect field; they ride
// SkillTierMod, not addSkillEffectDelta, so the init drift-guard skips them. Sync with SkillTierMod.
var deltaTierOnlyFields = map[string]bool{
	"StealBonusDamage":      true,
	"CritDoubleOnExcellent": true,
}

// init pins SkillEffect, SkillEffectDelta and addSkillEffectDelta against drift: for every
// non-tier-only delta field, asserts SkillEffect has a same-name/type field AND addSkillEffectDelta
// folds it (sets a sentinel, folds into a zero SkillEffect, checks the field moved). Panics at start.
// Hand-unrolled fold stays off the combat path; this guard pays reflection once at startup.
func init() {
	deltaType := reflect.TypeOf(SkillEffectDelta{})
	effType := reflect.TypeOf(SkillEffect{})
	for i := 0; i < deltaType.NumField(); i++ {
		f := deltaType.Field(i)
		if deltaTierOnlyFields[f.Name] {
			continue
		}
		ef, ok := effType.FieldByName(f.Name)
		if !ok {
			panic("core: SkillEffectDelta." + f.Name + " has no matching SkillEffect field — add it to SkillEffect or list it in deltaTierOnlyFields")
		}
		if ef.Type != f.Type {
			panic("core: SkillEffectDelta." + f.Name + " type differs from SkillEffect." + f.Name + " — the fields must mirror")
		}
		// Delta with only this field set to a sentinel.
		var delta SkillEffectDelta
		setDeltaSentinel(reflect.ValueOf(&delta).Elem().Field(i))
		var eff SkillEffect
		addSkillEffectDelta(&eff, delta)
		if reflect.ValueOf(eff).FieldByName(f.Name).IsZero() {
			panic("core: addSkillEffectDelta does not fold SkillEffectDelta." + f.Name + " — add an `eff." + f.Name + " += d." + f.Name + "` step")
		}
	}
	// Guard the tier-only set itself: each listed name must be a real
	// SkillEffectDelta field with NO SkillEffect twin (a twin means it should be
	// folded by addSkillEffectDelta, not ride SkillTierMod). Catches a typo/rename
	// or an accidental promotion that would silently mis-handle the field.
	for name := range deltaTierOnlyFields {
		if _, ok := deltaType.FieldByName(name); !ok {
			panic("core: deltaTierOnlyFields lists " + name + " which is not a SkillEffectDelta field — fix the name")
		}
		if _, ok := effType.FieldByName(name); ok {
			panic("core: deltaTierOnlyFields lists " + name + " but SkillEffect has a same-name field — drop it from deltaTierOnlyFields so addSkillEffectDelta folds it")
		}
	}
}

// setDeltaSentinel writes a distinct non-zero value into a SkillEffectDelta field so folding it
// provably moves the matching SkillEffect field. Struct fields seed their first member (SumStats
// folds every member uniformly).
func setDeltaSentinel(v reflect.Value) {
	switch v.Kind() {
	case reflect.Int:
		v.SetInt(7)
	case reflect.Float64:
		v.SetFloat(0.5)
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Struct:
		if v.NumField() == 0 {
			panic("core: setDeltaSentinel hit an empty struct field — extend it")
		}
		setDeltaSentinel(v.Field(0))
	default:
		panic("core: setDeltaSentinel unhandled SkillEffectDelta field kind " + v.Kind().String())
	}
}

// SkillDamageFor is the tier-aware counterpart of SkillDamage: pre-quality damage with every
// purchased +Damage delta stacked in, then stat-scaled (Melee+STR, Magic+INT, Utility passthrough).
func SkillDamageFor(m *PartyMember, s SkillID) int {
	if m == nil {
		return SkillDamage(Stats{}, s)
	}
	def, ok := skillInfo(s)
	if !ok {
		return 0
	}
	effect := EffectiveSkillEffect(m, s)
	// EffectiveStats so equipped items' StatBonus (Iron Sword +STR, tome +INT) bumps damage.
	return scaleDamageByKind(def.Kind, EffectiveStats(*m), effect.Damage)
}

// SkillHealFor mirrors SkillDamageFor for heals: Heal kind adds WIS to the tier-augmented base
// (via EffectiveStats so a WIS accessory lifts output), else returns the base Heal unchanged.
func SkillHealFor(m *PartyMember, s SkillID) int {
	if m == nil {
		return SkillHeal(Stats{}, s)
	}
	def, ok := skillInfo(s)
	if !ok {
		return 0
	}
	effect := EffectiveSkillEffect(m, s)
	return scaleHealByKind(def.Kind, EffectiveStats(*m), effect.Heal)
}

// SkillTierMod returns the combined delta of every purchased tier for the tier-only extension
// fields (StealBonusDamage, CritDoubleOnExcellent) the apply path reads alongside the SkillEffect.
func SkillTierMod(m *PartyMember, s SkillID) SkillEffectDelta {
	var mod SkillEffectDelta
	if m == nil {
		return mod
	}
	forEachPurchasedTier(m, s, func(up SkillTierUpgrade) {
		d := up.Effect
		mod.StealBonusDamage += d.StealBonusDamage
		if d.CritDoubleOnExcellent {
			mod.CritDoubleOnExcellent = true
		}
	})
	return mod
}

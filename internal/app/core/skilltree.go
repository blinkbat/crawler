package core

// Per-skill upgrade ladder + helpers that fold the tier modifiers into the
// SkillEffect the battle code applies. Each player-castable skill has
// MaxSkillTier possible upgrade slots. This is now LIVE: the Diablo-2-style
// skill trees in skilltrees.go drive it — BuySkillNode writes
// PartyMember.SkillTiers as the player ranks up a granting node (rank 1 =
// tier 0 / base, each further rank = the next tier), and EffectiveSkillEffect
// / SkillDamageFor / SkillHealFor / SkillTierMod (called from
// battle/actions.go) fold the purchased deltas into every cast. Points are
// spent only through the tree modal (BuySkillNode) — there is no separate
// per-skill buyer.
//
// The base skill (tier 0) is available the moment a tree node learns it;
// tiers 1..MaxSkillTier stack additive deltas onto the base effect as the
// node's rank climbs.
//
// Design contract: each tier is ONE numeric/bool delta applied to a
// SkillEffect field that already exists (Damage, Heal, BurnChance,
// etc.) or a tier-only field below (StealBonusDamage). Keeping the
// delta surface small means the apply
// path in battle/actions.go is:
//
//   effect := core.EffectiveSkillEffect(m, skill)
//
// instead of "look up base + branch on tier per skill." Tuning a
// number is one row in skillTierTable; adding a new delta is one
// field below + one apply site in EffectiveSkillEffect.

// MaxSkillTier is the maximum purchasable tier per skill. Three tiers
// per skill is the design target — bumping this means authoring more
// rows in skillTierTable, but the rest of the system (UI cursor math,
// SP cost, registry lookups) reads from this constant so no other
// site needs editing.
const MaxSkillTier = 3

// SkillTierUpgrade is one purchasable rung of a skill's tree.
// Description is the player-facing tooltip; Cost is in SkillPoints
// (1 by default — a future "expensive elite tier" would bump this).
// The Effect field carries the additive delta applied to the base
// SkillEffect when this tier is purchased.
type SkillTierUpgrade struct {
	Tier        int
	Label       string
	Description string
	Cost        int
	Effect      SkillEffectDelta
}

// SkillEffectDelta is the additive modifier applied per purchased
// tier. Mirrors SkillEffect's shape — fields are added (or OR'd, for
// bool flags) into the base. New tier abilities that don't fit an
// existing field add a new row here + an apply step in
// EffectiveSkillEffect; the UI and table-walker code don't need to
// change.
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
	// StealBonusDamage is the STR-multiplier damage dealt on a
	// successful steal (Thief Steal T3). 0 = the steal stays a
	// pure utility cast.
	StealBonusDamage int
	// CritDoubleOnExcellent is a bool flag that the per-skill
	// apply path checks when scoring an Excellent timing roll —
	// turns the hit into a double-damage crit. Used by Crushing
	// Blow T3 and Backstab T2.
	CritDoubleOnExcellent bool
	// BuffStats / BuffTurns are the buff-skill deltas — per-stat magnitude and
	// duration added onto the base SkillEffect's BuffStats / BuffTurns as the
	// granting node ranks up. Bless's tiers use these (T2/T3 add magnitude, T1
	// adds a turn); EffectiveSkillEffect folds them in alongside the other
	// numeric fields.
	BuffStats Stats
	BuffTurns int
	// RegenTurns is the heal-over-time duration delta (Renewal's tiers add
	// turns; the per-turn amount rides the existing Heal delta). Folded into
	// the base SkillEffect.RegenTurns by EffectiveSkillEffect.
	RegenTurns int
	// ArmorReduction is the Corrosive Vial armor-strip delta — tiers deepen the
	// break. Folded into the base SkillEffect.ArmorReduction by EffectiveSkillEffect.
	ArmorReduction int
	// ATBPush is the Sunder readiness-shove delta — tiers shove the target's ATB
	// gauge harder. Folded into the base SkillEffect.ATBPush.
	ATBPush int
	// BuffArmor / BuffMDef are the Stone Skin ward deltas — tiers raise the flat
	// Armor / MDef granted. Folded into the base SkillEffect.BuffArmor / BuffMDef.
	BuffArmor int
	BuffMDef  int
	// ShieldHP is the Aegis absorb-pool delta — tiers grow the shield. Folded
	// into the base SkillEffect.ShieldHP.
	ShieldHP int
	// IceArmorTurns is the Ice Armor duration delta — tiers extend how long the
	// frost ward stands. Folded into the base SkillEffect.IceArmorTurns.
	IceArmorTurns int
}

// skillTierTable is the source of truth for every player-castable
// skill's upgrade ladder. Three rows per skill, in tier order. The
// init guard below asserts every PlayerCastable skill has exactly
// MaxSkillTier rows — drift between this table and the registry
// panics at startup instead of producing silently empty trees.
var skillTierTable = map[SkillID][]SkillTierUpgrade{
	// ── Warrior ──────────────────────────────────────────────
	SkillSwipe: {
		{Tier: 1, Label: "+2 damage", Description: "+2 base damage to every hit in the cleave.", Cost: 1, Effect: SkillEffectDelta{Damage: 2}},
		{Tier: 2, Label: "+2 damage", Description: "+2 more base damage to the whole cleave.", Cost: 1, Effect: SkillEffectDelta{Damage: 2}},
		{Tier: 3, Label: "+2 damage", Description: "Another +2 base damage. Whole pack feels it.", Cost: 1, Effect: SkillEffectDelta{Damage: 2}},
	},
	SkillCrushingBlow: {
		{Tier: 1, Label: "+3 damage", Description: "+3 base damage on the heavy hit.", Cost: 1, Effect: SkillEffectDelta{Damage: 3}},
		{Tier: 2, Label: "+15% stun", Description: "Stun roll gets +15% chance on a landed Great/Excellent.", Cost: 1, Effect: SkillEffectDelta{StunChance: 0.15}},
		{Tier: 3, Label: "Excellent crits", Description: "An Excellent timing hit deals double damage.", Cost: 1, Effect: SkillEffectDelta{CritDoubleOnExcellent: true}},
	},
	SkillWhirlwind: {
		{Tier: 1, Label: "+2 damage", Description: "+2 base damage per target on the spin.", Cost: 1, Effect: SkillEffectDelta{Damage: 2}},
		{Tier: 2, Label: "+2 damage", Description: "+2 more base damage per target on the spin.", Cost: 1, Effect: SkillEffectDelta{Damage: 2}},
		{Tier: 3, Label: "+2 damage", Description: "Another +2 base damage. Excellent timing eviscerates the pack.", Cost: 1, Effect: SkillEffectDelta{Damage: 2}},
	},
	SkillSecondWind: {
		{Tier: 1, Label: "+3 heal", Description: "+3 to the flat self-heal.", Cost: 1, Effect: SkillEffectDelta{Heal: 3}},
		{Tier: 2, Label: "+3 heal", Description: "Another +3 to the breather.", Cost: 1, Effect: SkillEffectDelta{Heal: 3}},
		{Tier: 3, Label: "+3 heal", Description: "A third +3 — a maxed Second Wind is a real comeback.", Cost: 1, Effect: SkillEffectDelta{Heal: 3}},
	},
	// ── Cleric ───────────────────────────────────────────────
	SkillPrayer: {
		{Tier: 1, Label: "+3 heal", Description: "+3 base heal on the target.", Cost: 1, Effect: SkillEffectDelta{Heal: 3}},
		{Tier: 2, Label: "+3 heal", Description: "Another +3 heal. Tank-grade recovery in one cast.", Cost: 1, Effect: SkillEffectDelta{Heal: 3}},
		{Tier: 3, Label: "+3 heal", Description: "A third +3 heal — Prayer alone can top off a tank.", Cost: 1, Effect: SkillEffectDelta{Heal: 3}},
	},
	SkillMassMend: {
		{Tier: 1, Label: "+2 heal", Description: "+2 base heal across every alive party member.", Cost: 1, Effect: SkillEffectDelta{Heal: 2}},
		{Tier: 2, Label: "+2 heal", Description: "Another +2 heal across the whole party.", Cost: 1, Effect: SkillEffectDelta{Heal: 2}},
		{Tier: 3, Label: "+2 heal", Description: "A third +2 heal — full-party sustain in one cast.", Cost: 1, Effect: SkillEffectDelta{Heal: 2}},
	},
	SkillSmite: {
		{Tier: 1, Label: "+2 damage", Description: "+2 base damage on the press tap.", Cost: 1, Effect: SkillEffectDelta{Damage: 2}},
		{Tier: 2, Label: "+2 damage", Description: "Another +2 base damage.", Cost: 1, Effect: SkillEffectDelta{Damage: 2}},
		{Tier: 3, Label: "+25% stun", Description: "Lands a Stun roll with 25% chance on Great/Excellent timing.", Cost: 1, Effect: SkillEffectDelta{StunChance: 0.25, StunMinTurns: 1, StunMaxTurns: 1}},
	},
	SkillBless: {
		{Tier: 1, Label: "+1 turn", Description: "The blessing lingers one turn longer on the whole party.", Cost: 1, Effect: SkillEffectDelta{BuffTurns: 1}},
		{Tier: 2, Label: "+1 to blessed stats", Description: "+1 more to STR, DEX, INT and WIS for every blessed ally.", Cost: 1, Effect: SkillEffectDelta{BuffStats: Stats{STR: 1, DEX: 1, INT: 1, WIS: 1}}},
		{Tier: 3, Label: "+1 to blessed stats", Description: "Another +1 to all four blessed stats — a maxed blessing is a sweeping party buff.", Cost: 1, Effect: SkillEffectDelta{BuffStats: Stats{STR: 1, DEX: 1, INT: 1, WIS: 1}}},
	},
	SkillRenewal: {
		{Tier: 1, Label: "+1 turn", Description: "The regen ticks one more turn.", Cost: 1, Effect: SkillEffectDelta{RegenTurns: 1}},
		{Tier: 2, Label: "+1 heal/turn", Description: "+1 to the per-turn heal (before WIS scaling).", Cost: 1, Effect: SkillEffectDelta{Heal: 1}},
		{Tier: 3, Label: "+1 turn", Description: "Another turn of regen — a maxed Renewal sustains an ally for the whole fight.", Cost: 1, Effect: SkillEffectDelta{RegenTurns: 1}},
	},
	// ── Thief ────────────────────────────────────────────────
	SkillSteal: {
		{Tier: 1, Label: "+15% chance", Description: "Steal succeeds 15% more often.", Cost: 1, Effect: SkillEffectDelta{StealChance: 0.15}},
		{Tier: 2, Label: "+15% chance", Description: "Another +15% steal chance.", Cost: 1, Effect: SkillEffectDelta{StealChance: 0.15}},
		{Tier: 3, Label: "Cuts on lift", Description: "A successful steal also deals STR damage.", Cost: 1, Effect: SkillEffectDelta{StealBonusDamage: 1}},
	},
	SkillBackstab: {
		{Tier: 1, Label: "+2 damage", Description: "+2 base damage on the dagger thrust.", Cost: 1, Effect: SkillEffectDelta{Damage: 2}},
		{Tier: 2, Label: "Excellent crits harder", Description: "Excellent timing's existing double-damage stacks an additional damage tier.", Cost: 1, Effect: SkillEffectDelta{CritDoubleOnExcellent: true}},
		{Tier: 3, Label: "+3 damage", Description: "Another +3 base damage. Backstab carries.", Cost: 1, Effect: SkillEffectDelta{Damage: 3}},
	},
	SkillVenomStrike: {
		{Tier: 1, Label: "+15% Poison", Description: "Poison-apply chance bumped by 15%.", Cost: 1, Effect: SkillEffectDelta{PoisonChance: 0.15}},
		{Tier: 2, Label: "+1 Poison turn", Description: "Poison's max-roll duration extends by one turn.", Cost: 1, Effect: SkillEffectDelta{PoisonMaxTurns: 1}},
		{Tier: 3, Label: "+2 damage", Description: "+2 base damage on the strike itself.", Cost: 1, Effect: SkillEffectDelta{Damage: 2}},
	},
	SkillCripple: {
		{Tier: 1, Label: "+1 turn", Description: "The cripple lingers one of the target's turns longer.", Cost: 1, Effect: SkillEffectDelta{BuffTurns: 1}},
		{Tier: 2, Label: "-1 more SPD", Description: "Saps another point of SPD while it lasts.", Cost: 1, Effect: SkillEffectDelta{BuffStats: Stats{SPD: -1}}},
		{Tier: 3, Label: "-1 more SPD", Description: "Another point of SPD — a maxed Cripple drags a slow foe to a crawl.", Cost: 1, Effect: SkillEffectDelta{BuffStats: Stats{SPD: -1}}},
	},
	SkillCorrosiveVial: {
		{Tier: 1, Label: "+2 Armor break", Description: "Strips 2 more Armor per vial.", Cost: 1, Effect: SkillEffectDelta{ArmorReduction: 2}},
		{Tier: 2, Label: "+2 Armor break", Description: "Another +2 Armor stripped.", Cost: 1, Effect: SkillEffectDelta{ArmorReduction: 2}},
		{Tier: 3, Label: "+3 Armor break", Description: "A maxed vial melts even a heavy carapace.", Cost: 1, Effect: SkillEffectDelta{ArmorReduction: 3}},
	},
	SkillPoisonCloud: {
		{Tier: 1, Label: "+15% Poison", Description: "Every enemy in the cloud rolls a 15% higher Poison chance.", Cost: 1, Effect: SkillEffectDelta{PoisonChance: 0.15}},
		{Tier: 2, Label: "+1 Poison turn", Description: "The cloud's Poison lingers one extra turn on its max roll.", Cost: 1, Effect: SkillEffectDelta{PoisonMaxTurns: 1}},
		{Tier: 3, Label: "+1 damage", Description: "+1 base damage to every enemy caught in the cloud.", Cost: 1, Effect: SkillEffectDelta{Damage: 1}},
	},
	// ── Wizard ───────────────────────────────────────────────
	SkillFirebolt: {
		{Tier: 1, Label: "+2 damage", Description: "+2 base damage on the bolt.", Cost: 1, Effect: SkillEffectDelta{Damage: 2}},
		{Tier: 2, Label: "+20% Burn", Description: "Burn-apply chance bumped by 20%.", Cost: 1, Effect: SkillEffectDelta{BurnChance: 0.20}},
		{Tier: 3, Label: "+1 Burn turn", Description: "Burn lasts one turn longer on min and max rolls.", Cost: 1, Effect: SkillEffectDelta{BurnMinTurns: 1, BurnMaxTurns: 1}},
	},
	SkillFrostLance: {
		{Tier: 1, Label: "+2 damage", Description: "+2 base damage on the lance.", Cost: 1, Effect: SkillEffectDelta{Damage: 2}},
		{Tier: 2, Label: "+15% Stun", Description: "Stun roll gets +15% chance on Great/Excellent.", Cost: 1, Effect: SkillEffectDelta{StunChance: 0.15}},
		{Tier: 3, Label: "+1 Stun turn", Description: "Stun lasts an extra turn when it lands.", Cost: 1, Effect: SkillEffectDelta{StunMinTurns: 1, StunMaxTurns: 1}},
	},
	SkillFrostbite: {
		{Tier: 1, Label: "+2 damage", Description: "+2 base frost damage.", Cost: 1, Effect: SkillEffectDelta{Damage: 2}},
		{Tier: 2, Label: "+1 chill turn", Description: "The chill lingers one of the target's turns longer.", Cost: 1, Effect: SkillEffectDelta{BuffTurns: 1}},
		{Tier: 3, Label: "-1 more SPD", Description: "Saps another point of SPD — a maxed Frostbite nearly freezes its mark.", Cost: 1, Effect: SkillEffectDelta{BuffStats: Stats{SPD: -1}}},
	},
	SkillConeOfCold: {
		{Tier: 1, Label: "+1 damage", Description: "+1 base frost damage to every enemy in the cone.", Cost: 1, Effect: SkillEffectDelta{Damage: 1}},
		{Tier: 2, Label: "+1 chill turn", Description: "The pack-wide chill lasts one extra turn.", Cost: 1, Effect: SkillEffectDelta{BuffTurns: 1}},
		{Tier: 3, Label: "-1 more SPD", Description: "Saps another point of SPD from every chilled foe.", Cost: 1, Effect: SkillEffectDelta{BuffStats: Stats{SPD: -1}}},
	},
	SkillArcBolt: {
		{Tier: 1, Label: "+1 damage", Description: "+1 base damage per arc target.", Cost: 1, Effect: SkillEffectDelta{Damage: 1}},
		{Tier: 2, Label: "+1 damage", Description: "Another +1 damage per target.", Cost: 1, Effect: SkillEffectDelta{Damage: 1}},
		{Tier: 3, Label: "+15% Burn", Description: "Every arc target rolls a 15% burn chance.", Cost: 1, Effect: SkillEffectDelta{BurnChance: 0.15, BurnMinTurns: 1, BurnMaxTurns: 2}},
	},
	SkillFireball: {
		{Tier: 1, Label: "+2 damage", Description: "+2 base magic damage to every enemy in the blast.", Cost: 1, Effect: SkillEffectDelta{Damage: 2}},
		{Tier: 2, Label: "+20% Burn", Description: "Per-target Burn-apply chance bumped by 20%.", Cost: 1, Effect: SkillEffectDelta{BurnChance: 0.20}},
		{Tier: 3, Label: "+1 Burn turn", Description: "Burn lasts one turn longer on min and max rolls.", Cost: 1, Effect: SkillEffectDelta{BurnMinTurns: 1, BurnMaxTurns: 1}},
	},
	// ── Warrior (tree-node skills) ───────────────────────────
	SkillSunder: {
		{Tier: 1, Label: "+2 damage", Description: "+2 base damage on the sundering blow.", Cost: 1, Effect: SkillEffectDelta{Damage: 2}},
		{Tier: 2, Label: "+2 damage", Description: "Another +2 base damage.", Cost: 1, Effect: SkillEffectDelta{Damage: 2}},
		{Tier: 3, Label: "Harder shove", Description: "Knocks the target's turn even further back.", Cost: 1, Effect: SkillEffectDelta{ATBPush: 25}},
	},
	SkillWarBanner: {
		{Tier: 1, Label: "+1 turn", Description: "The banner stands one turn longer.", Cost: 1, Effect: SkillEffectDelta{BuffTurns: 1}},
		{Tier: 2, Label: "+1 STR/Armor", Description: "+1 more to the rallied STR and Armor for every ally.", Cost: 1, Effect: SkillEffectDelta{BuffStats: Stats{STR: 1}, BuffArmor: 1}},
		{Tier: 3, Label: "+1 STR/Armor", Description: "Another +1 to both — a maxed banner is a sweeping party buff.", Cost: 1, Effect: SkillEffectDelta{BuffStats: Stats{STR: 1}, BuffArmor: 1}},
	},
	SkillStoneSkin: {
		{Tier: 1, Label: "+2 Armor", Description: "+2 more Armor on the ward.", Cost: 1, Effect: SkillEffectDelta{BuffArmor: 2}},
		{Tier: 2, Label: "+2 MDef", Description: "+2 more MDef on the ward.", Cost: 1, Effect: SkillEffectDelta{BuffMDef: 2}},
		{Tier: 3, Label: "+1 turn", Description: "The ward lasts one of the ally's turns longer.", Cost: 1, Effect: SkillEffectDelta{BuffTurns: 1}},
	},
	// ── Cleric (tree-node skills) ────────────────────────────
	SkillBlind: {
		{Tier: 1, Label: "+1 turn", Description: "The blindness lingers one of the target's turns longer.", Cost: 1, Effect: SkillEffectDelta{BuffTurns: 1}},
		{Tier: 2, Label: "-1 more DEX", Description: "Saps another point of accuracy while it lasts.", Cost: 1, Effect: SkillEffectDelta{BuffStats: Stats{DEX: -1}}},
		{Tier: 3, Label: "-1 more DEX", Description: "Another point of accuracy — a maxed Blind nearly closes a foe's eyes.", Cost: 1, Effect: SkillEffectDelta{BuffStats: Stats{DEX: -1}}},
	},
	SkillAegis: {
		{Tier: 1, Label: "+4 shield", Description: "+4 to the absorb pool.", Cost: 1, Effect: SkillEffectDelta{ShieldHP: 4}},
		{Tier: 2, Label: "+4 shield", Description: "Another +4 absorb.", Cost: 1, Effect: SkillEffectDelta{ShieldHP: 4}},
		{Tier: 3, Label: "+6 shield", Description: "A maxed Aegis turns aside a heavy blow outright.", Cost: 1, Effect: SkillEffectDelta{ShieldHP: 6}},
	},
	// ── Thief (tree-node skills) ─────────────────────────────
	SkillSmokeBomb: {
		{Tier: 1, Label: "+1 turn", Description: "The smoke hangs one turn longer.", Cost: 1, Effect: SkillEffectDelta{BuffTurns: 1}},
		{Tier: 2, Label: "+1 DEX swing", Description: "Another point of party evasion and enemy accuracy loss.", Cost: 1, Effect: SkillEffectDelta{BuffStats: Stats{DEX: 1}}},
		{Tier: 3, Label: "+1 DEX swing", Description: "A maxed Smoke Bomb blinds the room and slips the party clear.", Cost: 1, Effect: SkillEffectDelta{BuffStats: Stats{DEX: 1}}},
	},
	// ── Wizard (tree-node skills) ────────────────────────────
	SkillIceArmor: {
		{Tier: 1, Label: "+1 turn", Description: "The frost ward stands one turn longer.", Cost: 1, Effect: SkillEffectDelta{IceArmorTurns: 1}},
		{Tier: 2, Label: "+1 turn", Description: "Another turn of ward.", Cost: 1, Effect: SkillEffectDelta{IceArmorTurns: 1}},
		{Tier: 3, Label: "+2 turns", Description: "A maxed Ice Armor sheathes the caster for most of a fight.", Cost: 1, Effect: SkillEffectDelta{IceArmorTurns: 2}},
	},
	// ── Bleed strikes (Warrior Rend / Thief Lacerate) ────────
	SkillRend: {
		{Tier: 1, Label: "+2 damage", Description: "+2 base damage on the opening cut.", Cost: 1, Effect: SkillEffectDelta{Damage: 2}},
		{Tier: 2, Label: "+1 Bleed turn", Description: "The wound bleeds one turn longer on min and max rolls.", Cost: 1, Effect: SkillEffectDelta{BleedMinTurns: 1, BleedMaxTurns: 1}},
		{Tier: 3, Label: "+15% Bleed", Description: "Bleed-apply chance bumped by 15% — a maxed Rend almost always draws blood.", Cost: 1, Effect: SkillEffectDelta{BleedChance: 0.15}},
	},
	SkillLacerate: {
		{Tier: 1, Label: "+1 damage", Description: "+1 base damage on the cut.", Cost: 1, Effect: SkillEffectDelta{Damage: 1}},
		{Tier: 2, Label: "+1 Bleed turn", Description: "The wound bleeds one turn longer on min and max rolls.", Cost: 1, Effect: SkillEffectDelta{BleedMinTurns: 1, BleedMaxTurns: 1}},
		{Tier: 3, Label: "+15% Bleed", Description: "Bleed-apply chance bumped by 15%.", Cost: 1, Effect: SkillEffectDelta{BleedChance: 0.15}},
	},
}

// init asserts every player-castable skill has exactly MaxSkillTier
// rows in skillTierTable, and that tier indices are 1..MaxSkillTier
// in order. Drift panics at process start, mirroring the AGENTS.md
// invariant pattern for skill / tile / prop registries.
func init() {
	for _, s := range PlayerCastableSkills() {
		// A NoUpgrades skill (single-rank utility like Scan) legitimately
		// has no tier ladder; it must carry NO stray tier rows, but it's
		// exempt from the "exactly MaxSkillTier rows" requirement below.
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

// SkillTierOf returns the member's currently-purchased tier for the
// given skill (0..MaxSkillTier). Nil-safe and zero-value-safe: a
// freshly-created party member with no SkillTiers map returns 0 for
// every skill. The Skills panel reads through this to render the
// tree's filled / empty / next-buyable node states.
func SkillTierOf(m *PartyMember, s SkillID) int {
	if m == nil || m.SkillTiers == nil {
		return 0
	}
	t := m.SkillTiers[s]
	if t < 0 {
		return 0
	}
	if t > MaxSkillTier {
		return MaxSkillTier
	}
	return t
}

// skillTierUpgradeFor returns the upgrade definition for a skill's
// tier (1..MaxSkillTier). Returns ok=false when the tier index is out
// of range; the UI uses this to grey out the "next purchase" line
// when the tree is fully invested.
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

// EffectiveSkillEffect returns the base SkillEffect for `s` with
// every purchased tier's delta stacked in. Numeric fields are added;
// bool flags are OR'd. This is the single source of truth the battle
// apply path reads — `core.SkillEffectFor(s)` returns the BASE row
// from the registry and should only be used by code that explicitly
// wants the un-modified shape (editor preview, save validation,
// etc.). For combat math, always go through EffectiveSkillEffect.
//
// A nil member (encounter previews, tests that don't model
// characters) returns the base effect unchanged, equivalent to
// "every skill at tier 0."
func EffectiveSkillEffect(m *PartyMember, s SkillID) SkillEffect {
	eff := SkillEffectFor(s)
	if m == nil {
		return eff
	}
	tier := SkillTierOf(m, s)
	for i := 1; i <= tier; i++ {
		up, ok := skillTierUpgradeFor(s, i)
		if !ok {
			break
		}
		d := up.Effect
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
	}
	return eff
}

// SkillDamageFor is the tier-aware counterpart of SkillDamage. Returns
// the actor's pre-quality damage for the skill with every purchased
// tier's +Damage delta stacked in, then dispatched through the same
// stat-scaling rule (Melee adds STR, Magic adds INT, Utility passes
// through). Battle apply handlers should call this instead of
// SkillDamage so tier bumps land.
func SkillDamageFor(m *PartyMember, s SkillID) int {
	if m == nil {
		return SkillDamage(Stats{}, s)
	}
	def, ok := skillInfo(s)
	if !ok {
		return 0
	}
	effect := EffectiveSkillEffect(m, s)
	// Read through EffectiveStats so equipped items' StatBonus picks
	// up: an Iron Sword (+2 STR) bumps a Melee skill's pre-quality
	// damage, a tome (+2 INT) bumps a Magic skill, etc. Base m.Stats
	// stays clean — level-up spends still edit the base.
	return scaleDamageByKind(def.Kind, EffectiveStats(*m), effect.Damage)
}

// SkillHealFor mirrors SkillDamageFor for healing skills — Heal kind
// adds WIS to the tier-augmented base, anything else returns the base
// effect's Heal field unchanged. Reads through EffectiveStats so a
// WIS-boosting accessory lifts heal output.
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

// SkillTierMod returns the combined delta of every purchased tier
// for the bool/integer "extension" fields that don't live in the
// base SkillEffect — StealBonusDamage, CritDoubleOnExcellent. The
// apply path reads these alongside the SkillEffect to decide
// tier-only behaviors (steal cuts, crit doubles).
func SkillTierMod(m *PartyMember, s SkillID) SkillEffectDelta {
	var mod SkillEffectDelta
	if m == nil {
		return mod
	}
	tier := SkillTierOf(m, s)
	for i := 1; i <= tier; i++ {
		up, ok := skillTierUpgradeFor(s, i)
		if !ok {
			break
		}
		d := up.Effect
		mod.StealBonusDamage += d.StealBonusDamage
		if d.CritDoubleOnExcellent {
			mod.CritDoubleOnExcellent = true
		}
	}
	return mod
}

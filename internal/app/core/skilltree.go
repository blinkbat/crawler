package core

import "reflect"

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
//
// This struct hand-mirrors ~20 additive fields of SkillEffect (party.go)
// and addSkillEffectDelta folds them one by one. Embedding a shared
// SkillEffectAdditive struct into BOTH would be the tidier dedupe, but
// SkillEffect / SkillEffectDelta are built with keyed composite literals
// in ~80 sites across the registry, tests and battle code — promoted
// (embedded) fields can't be set in an outer keyed literal, so embedding
// would force a rewrite of every one of them. Instead the drift between
// the three (the two structs + the fold) is pinned by an init-time
// reflection assert below: a new additive field added to one but missed
// in another panics at startup, so the lockstep edit can't be silently
// half-done.
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

// tier builds one SkillTierUpgrade row with Cost defaulting to the standard
// 1 SkillPoint — the only cost any current rung uses. Centralizing the default
// here means a future "expensive elite tier" reprices in one place (add a
// tierCost variant) instead of editing ~90 literal `Cost: 1` fields. Mirrors
// the nd()/act() row constructors skilltrees.go uses for the same reason.
func tier(t int, label, description string, effect SkillEffectDelta) SkillTierUpgrade {
	return SkillTierUpgrade{Tier: t, Label: label, Description: description, Cost: 1, Effect: effect}
}

// skillTierTable is the source of truth for every player-castable
// skill's upgrade ladder. Three rows per skill, in tier order. The
// init guard below asserts every PlayerCastable skill has exactly
// MaxSkillTier rows — drift between this table and the registry
// panics at startup instead of producing silently empty trees.
var skillTierTable = map[SkillID][]SkillTierUpgrade{
	// ── Warrior ──────────────────────────────────────────────
	SkillSwipe: {
		tier(1, "+2 damage", "+2 base damage to every hit in the cleave.", SkillEffectDelta{Damage: 2}),
		tier(2, "+2 damage", "+2 more base damage to the whole cleave.", SkillEffectDelta{Damage: 2}),
		tier(3, "+2 damage", "Another +2 base damage. Whole pack feels it.", SkillEffectDelta{Damage: 2}),
	},
	SkillCrushingBlow: {
		tier(1, "+3 damage", "+3 base damage on the heavy hit.", SkillEffectDelta{Damage: 3}),
		tier(2, "+15% stun", "Stun roll gets +15% chance on a landed Great/Excellent.", SkillEffectDelta{StunChance: 0.15}),
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
		tier(3, "+25% stun", "Lands a Stun roll with 25% chance on Great/Excellent timing.", SkillEffectDelta{StunChance: 0.25, StunMinTurns: StunTurnStep, StunMaxTurns: StunTurnStep}),
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
		tier(1, "+15% chance", "Steal succeeds 15% more often.", SkillEffectDelta{StealChance: 0.15}),
		tier(2, "+15% chance", "Another +15% steal chance.", SkillEffectDelta{StealChance: 0.15}),
		tier(3, "Cuts on lift", "A successful steal also deals STR damage.", SkillEffectDelta{StealBonusDamage: 1}),
	},
	SkillBackstab: {
		tier(1, "+2 damage", "+2 base damage on the dagger thrust.", SkillEffectDelta{Damage: 2}),
		tier(2, "Excellent crits harder", "Excellent timing's existing double-damage stacks an additional damage tier.", SkillEffectDelta{CritDoubleOnExcellent: true}),
		tier(3, "+3 damage", "Another +3 base damage. Backstab carries.", SkillEffectDelta{Damage: 3}),
	},
	SkillVenomStrike: {
		tier(1, "+15% Poison", "Poison-apply chance bumped by 15%.", SkillEffectDelta{PoisonChance: 0.15}),
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
		tier(1, "+15% Poison", "Every enemy in the cloud rolls a 15% higher Poison chance.", SkillEffectDelta{PoisonChance: 0.15}),
		tier(2, "+1 Poison turn", "The cloud's Poison lingers one extra turn on its max roll.", SkillEffectDelta{PoisonMaxTurns: 1}),
		tier(3, "+1 damage", "+1 base damage to every enemy caught in the cloud.", SkillEffectDelta{Damage: 1}),
	},
	// ── Wizard ───────────────────────────────────────────────
	SkillFirebolt: {
		tier(1, "+2 damage", "+2 base damage on the bolt.", SkillEffectDelta{Damage: 2}),
		tier(2, "+20% Burn", "Burn-apply chance bumped by 20%.", SkillEffectDelta{BurnChance: 0.20}),
		tier(3, "+1 Burn turn", "Burn lasts one turn longer on min and max rolls.", SkillEffectDelta{BurnMinTurns: 1, BurnMaxTurns: 1}),
	},
	SkillFrostLance: {
		tier(1, "+2 damage", "+2 base damage on the lance.", SkillEffectDelta{Damage: 2}),
		tier(2, "+15% Stun", "Stun roll gets +15% chance on Great/Excellent.", SkillEffectDelta{StunChance: 0.15}),
		tier(3, "+1 Stun turn", "Stun lasts an extra turn when it lands.", SkillEffectDelta{StunMinTurns: StunTurnStep, StunMaxTurns: StunTurnStep}),
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
		tier(3, "+15% Burn", "Every arc target rolls a 15% burn chance.", SkillEffectDelta{BurnChance: 0.15, BurnMinTurns: 1, BurnMaxTurns: 2}),
	},
	SkillFireball: {
		tier(1, "+2 damage", "+2 base magic damage to every enemy in the blast.", SkillEffectDelta{Damage: 2}),
		tier(2, "+20% Burn", "Per-target Burn-apply chance bumped by 20%.", SkillEffectDelta{BurnChance: 0.20}),
		tier(3, "+1 Burn turn", "Burn lasts one turn longer on min and max rolls.", SkillEffectDelta{BurnMinTurns: 1, BurnMaxTurns: 1}),
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
		tier(2, "+1 Bleed turn", "The wound bleeds one turn longer on min and max rolls.", SkillEffectDelta{BleedMinTurns: 1, BleedMaxTurns: 1}),
		tier(3, "+15% Bleed", "Bleed-apply chance bumped by 15% — a maxed Rend almost always draws blood.", SkillEffectDelta{BleedChance: 0.15}),
	},
	SkillLacerate: {
		tier(1, "+1 damage", "+1 base damage on the cut.", SkillEffectDelta{Damage: 1}),
		tier(2, "+1 Bleed turn", "The wound bleeds one turn longer on min and max rolls.", SkillEffectDelta{BleedMinTurns: 1, BleedMaxTurns: 1}),
		tier(3, "+15% Bleed", "Bleed-apply chance bumped by 15%.", SkillEffectDelta{BleedChance: 0.15}),
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
	return Clamp(m.SkillTiers[s], 0, MaxSkillTier)
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
	forEachPurchasedTier(m, s, func(up SkillTierUpgrade) {
		addSkillEffectDelta(&eff, up.Effect)
	})
	return eff
}

// forEachPurchasedTier invokes fn with each tier upgrade the member has
// purchased for skill s, walking tiers 1..SkillTierOf and stopping at the
// first tier with no upgrade row. The shared walk behind EffectiveSkillEffect
// and SkillTierMod so the per-tier accumulation loop lives in one place.
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

// addSkillEffectDelta folds one purchased tier's delta into the accumulating
// effect, field by field. Every summable SkillEffect field lives here so
// EffectiveSkillEffect's per-tier loop stays a single call — add a new tier-able
// field in one place, not at every fold site.
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
}

// deltaTierOnlyFields are the SkillEffectDelta fields that have NO matching
// SkillEffect field and ride SkillTierMod instead of addSkillEffectDelta —
// the init drift-guard below skips them. Keep in sync with SkillTierMod's
// fold and the SkillEffectDelta comment.
var deltaTierOnlyFields = map[string]bool{
	"StealBonusDamage":      true,
	"CritDoubleOnExcellent": true,
}

// init pins SkillEffect, SkillEffectDelta and addSkillEffectDelta together so
// the three can't drift. For every non-tier-only field of SkillEffectDelta it
// asserts (1) SkillEffect carries a field of the same name and type, and (2)
// addSkillEffectDelta actually folds it — by setting a lone sentinel on the
// delta field, folding into a zero SkillEffect, and checking the matching
// SkillEffect field moved. A new additive field added to one struct but missed
// in the other (or in the fold) panics at process start, the same loud-on-drift
// contract the skillTierTable coverage init uses. (The fold is hand-unrolled
// rather than reflection-driven because EffectiveSkillEffect runs on the combat
// path — see SumStats' hot-path note; this guard pays the reflection cost once
// at startup instead.)
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
		// Build a delta with only this field set to a non-zero sentinel.
		var delta SkillEffectDelta
		setDeltaSentinel(reflect.ValueOf(&delta).Elem().Field(i))
		var eff SkillEffect
		addSkillEffectDelta(&eff, delta)
		if reflect.ValueOf(eff).FieldByName(f.Name).IsZero() {
			panic("core: addSkillEffectDelta does not fold SkillEffectDelta." + f.Name + " — add an `eff." + f.Name + " += d." + f.Name + "` step")
		}
	}
}

// setDeltaSentinel writes a distinct non-zero value into a SkillEffectDelta
// field so folding it provably moves the matching SkillEffect field. Mirrors
// the test helper of the same shape; struct fields (BuffStats) seed their first
// member, which is enough since SumStats folds every member uniformly.
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
	forEachPurchasedTier(m, s, func(up SkillTierUpgrade) {
		d := up.Effect
		mod.StealBonusDamage += d.StealBonusDamage
		if d.CritDoubleOnExcellent {
			mod.CritDoubleOnExcellent = true
		}
	})
	return mod
}

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
// SkillEffect field that already exists (Damage, BurnChance, etc.) or
// a tier-only field below (HealBonus, StealBonusDamage). Keeping the
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
	HealBonus      int // extra heal applied to the apply path's heal amount
	StealChance    float64
	BurnChance     float64
	BurnMinTurns   int
	BurnMaxTurns   int
	PoisonChance   float64
	PoisonMinTurns int
	PoisonMaxTurns int
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
	// ── Cleric ───────────────────────────────────────────────
	SkillPrayer: {
		{Tier: 1, Label: "+3 heal", Description: "+3 base heal on the target.", Cost: 1, Effect: SkillEffectDelta{HealBonus: 3}},
		{Tier: 2, Label: "+3 heal", Description: "Another +3 heal. Tank-grade recovery in one cast.", Cost: 1, Effect: SkillEffectDelta{HealBonus: 3}},
		{Tier: 3, Label: "+3 heal", Description: "A third +3 heal — Prayer alone can top off a tank.", Cost: 1, Effect: SkillEffectDelta{HealBonus: 3}},
	},
	SkillMassMend: {
		{Tier: 1, Label: "+2 heal", Description: "+2 base heal across every alive party member.", Cost: 1, Effect: SkillEffectDelta{HealBonus: 2}},
		{Tier: 2, Label: "+2 heal", Description: "Another +2 heal across the whole party.", Cost: 1, Effect: SkillEffectDelta{HealBonus: 2}},
		{Tier: 3, Label: "+2 heal", Description: "A third +2 heal — full-party sustain in one cast.", Cost: 1, Effect: SkillEffectDelta{HealBonus: 2}},
	},
	SkillSmite: {
		{Tier: 1, Label: "+2 damage", Description: "+2 base damage on the press tap.", Cost: 1, Effect: SkillEffectDelta{Damage: 2}},
		{Tier: 2, Label: "+2 damage", Description: "Another +2 base damage.", Cost: 1, Effect: SkillEffectDelta{Damage: 2}},
		{Tier: 3, Label: "+25% stun", Description: "Lands a Stun roll with 25% chance on Great/Excellent timing.", Cost: 1, Effect: SkillEffectDelta{StunChance: 0.25, StunMinTurns: 1, StunMaxTurns: 1}},
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
	SkillArcBolt: {
		{Tier: 1, Label: "+1 damage", Description: "+1 base damage per arc target.", Cost: 1, Effect: SkillEffectDelta{Damage: 1}},
		{Tier: 2, Label: "+1 damage", Description: "Another +1 damage per target.", Cost: 1, Effect: SkillEffectDelta{Damage: 1}},
		{Tier: 3, Label: "+15% Burn", Description: "Every arc target rolls a 15% burn chance.", Cost: 1, Effect: SkillEffectDelta{BurnChance: 0.15, BurnMinTurns: 1, BurnMaxTurns: 2}},
	},
}

// init asserts every player-castable skill has exactly MaxSkillTier
// rows in skillTierTable, and that tier indices are 1..MaxSkillTier
// in order. Drift panics at process start, mirroring the AGENTS.md
// invariant pattern for skill / tile / prop registries.
func init() {
	for _, s := range PlayerCastableSkills() {
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
		eff.Heal += d.Heal + d.HealBonus
		eff.StealChance += d.StealChance
		eff.BurnChance += d.BurnChance
		eff.BurnMinTurns += d.BurnMinTurns
		eff.BurnMaxTurns += d.BurnMaxTurns
		eff.PoisonChance += d.PoisonChance
		eff.PoisonMinTurns += d.PoisonMinTurns
		eff.PoisonMaxTurns += d.PoisonMaxTurns
		eff.StunChance += d.StunChance
		eff.StunMinTurns += d.StunMinTurns
		eff.StunMaxTurns += d.StunMaxTurns
		eff.SleepMinTurns += d.SleepMinTurns
		eff.SleepMaxTurns += d.SleepMaxTurns
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

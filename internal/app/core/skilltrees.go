package core

import "slices"

// Diablo-2-style skill trees: each class has skillTreesPerClass thematic ladders of nodes.
// DATA + navigation layer AND the purchase->combat bridge: buying a GrantSkill node's first rank
// LEARNS that skill (LearnedSkills -> battle Skill menu); further ranks advance the per-skill
// ladder in skilltree.go (SkillTiers). Non-granting nodes (passives/capstones) only fill pips for now.
// The init guard below panics on drift (wrong tree count, dup node id, out-of-tree prerequisite).

// skillTreesPerClass is the fixed number of trees every class exposes (one column each in the modal).
const skillTreesPerClass = 3

// SkillTreeNode is one purchasable node in a class tree. Rank is tracked per member in
// PartyMember.TreeRanks keyed by ID. Tier is the row within its tree (0 = root, set by the builder).
// Requires names the node that must hold rank >= 1 before this unlocks; "" = always available (root).
type SkillTreeNode struct {
	ID       string
	Name     string
	Desc     string
	MaxRank  int
	Cost     int // SkillPoints per rank
	Tier     int
	Requires string
	// GrantSkill: castable skill this node LEARNS at rank >= 1; further ranks feed the per-skill
	// ladder (SkillTiers) via BuySkillNode. SkillNone = passive/novel node, no castable skill.
	GrantSkill SkillID
}

// SkillTreeDef is one of a class's trees: display name, theme blurb, and ordered node ladder.
type SkillTreeDef struct {
	Name  string
	Theme string
	Nodes []SkillTreeNode
}

// nd is the terse node constructor; Cost fixed at 1 SkillPoint/rank.
func nd(id, name, desc string, maxRank int) SkillTreeNode {
	return SkillTreeNode{ID: id, Name: name, Desc: desc, MaxRank: maxRank, Cost: 1}
}

// act builds an ACTIVE node that LEARNS a skill at rank 1, with ranks 2..MaxSkillTier+1 feeding the
// per-skill ladder. MaxRank is fixed at MaxSkillTier+1 here so grant sites can't drift from MaxSkillTier.
func act(id, name, desc string, grant SkillID) SkillTreeNode {
	n := nd(id, name, desc, MaxSkillTier+1)
	n.GrantSkill = grant
	return n
}

// actOnce is a single-rank granting node (MaxRank 1) for utility skills with no upgrade ladder
// (e.g. Scan), where act()'s extra ranks would be dead pips.
func actOnce(id, name, desc string, grant SkillID) SkillTreeNode {
	n := nd(id, name, desc, 1)
	n.GrantSkill = grant
	return n
}

// linearTree wires nodes into a single chain: Tier = index, each past root Requires the one above.
func linearTree(name, theme string, nodes []SkillTreeNode) SkillTreeDef {
	for i := range nodes {
		nodes[i].Tier = i
		if i > 0 {
			nodes[i].Requires = nodes[i-1].ID
		}
	}
	return SkillTreeDef{Name: name, Theme: theme, Nodes: nodes}
}

// classSkillTrees is the source of truth for every class's trees. Linear for now; the data model
// supports arbitrary Requires for future branching.
var classSkillTrees = map[PartyClass][]SkillTreeDef{
	// ── Warrior ──────────────────────────────────────────────────────
	ClassWarrior: {
		linearTree("Battle Sense", "Disable, hinder, intercept, react", []SkillTreeNode{
			act("shield-bash", "Shield Bash", "Phys hit with a chance to Stun on good timing.", SkillCrushingBlow),
			actOnce("taunt", "Taunt", "Force the target enemy to attack the Warrior next turn.", SkillTaunt),
			nd("guard", "Guard", "Cover an ally this round; their incoming hits redirect to you.", 3),
			act("sunder", "Sunder", "Phys hit that pushes the target's turn later.", SkillSunder),
			nd("riposte", "Riposte", "Passive: counter-attack an enemy whose strike you dodge.", 1),
		}),
		linearTree("Ancestral Call", "Utility, light heal, protection, summons", []SkillTreeNode{
			act("second-wind", "Second Wind", "A flat self-heal — catch your breath mid-fight.", SkillSecondWind),
			act("war-banner", "War Banner", "Plant a banner: party gains stats while it stands.", SkillWarBanner),
			act("stone-skin", "Stone Skin", "Grant an ally temporary Armor and MDef.", SkillStoneSkin),
			nd("ancestral-spirit", "Ancestral Spirit", "Summon a warrior shade to fight beside the party.", 1),
			nd("last-stand", "Last Stand", "Capstone: once per battle, survive a lethal blow at 1 HP.", 1),
		}),
		linearTree("Fury", "Lifesteal, bleed, AoE, self-harm", []SkillTreeNode{
			act("cleave", "Cleave", "Multi-hit AoE swing across the enemy pack.", SkillSwipe),
			act("rend", "Rend", "Phys hit that applies a Bleed damage-over-time.", SkillRend),
			nd("bloodthirst", "Bloodthirst", "Passive: heal for a share of all physical damage dealt.", 3),
			nd("reckless-swing", "Reckless Swing", "A heavy hit that lowers your own Armor for a turn.", 3),
			nd("crimson-rampage", "Crimson Rampage", "Capstone: deal more damage the lower your HP.", 1),
		}),
	},
	// ── Cleric ───────────────────────────────────────────────────────
	ClassCleric: {
		linearTree("Radiance", "Holy offense, smite, anti-status", []SkillTreeNode{
			act("smite", "Smite", "Magic burst; bonus damage to undead.", SkillSmite),
			nd("searing-light", "Searing Light", "A radiant damage-over-time.", 3),
			act("blind", "Blind", "Lower an enemy's accuracy for several turns.", SkillBlind),
			nd("consecrate", "Consecrate", "AoE radiant damage across the enemy pack.", 3),
			nd("judgment", "Judgment", "Capstone: execute low-HP enemies for massive damage.", 1),
		}),
		linearTree("Mercy", "Restoration, regen, cleanse, revive", []SkillTreeNode{
			act("prayer", "Prayer", "Single-target heal on an ally.", SkillPrayer),
			actOnce("cleanse", "Cleanse", "Cure an ally's Poison, Sleep, Stun, Web and Confusion.", SkillCleanse),
			act("renewal", "Renewal", "Heal-over-time regeneration on an ally.", SkillRenewal),
			act("mass-mend", "Mass Mend", "Heal the entire living party at once.", SkillMassMend),
			nd("resurrect", "Resurrect", "Capstone: revive a downed party member.", 1),
		}),
		linearTree("Conviction", "Buffs, wards, retribution", []SkillTreeNode{
			act("blessing", "Blessing", "Buff the whole party's STR, DEX, INT and WIS for a few turns.", SkillBless),
			act("aegis", "Aegis", "Shield an ally against the next hit.", SkillAegis),
			nd("retribution", "Retribution", "Passive: attackers take reflected damage.", 3),
			nd("martyrs-bond", "Martyr's Bond", "Redirect an ally's incoming damage to the Cleric.", 1),
			nd("bulwark-of-faith", "Bulwark of Faith", "Capstone: party-wide Armor and MDef aura.", 1),
		}),
	},
	// ── Thief ────────────────────────────────────────────────────────
	ClassThief: {
		linearTree("Shadow Arts", "Stealth, evasion, control", []SkillTreeNode{
			act("backstab", "Backstab", "High-crit opener; damage doubles on Excellent timing.", SkillBackstab),
			actOnce("scan", "Scan", "Inspect a foe: reveal its exact HP for the rest of the battle.", SkillScan),
			act("cripple", "Cripple", "Sap an enemy's SPD, slowing how often it acts.", SkillCripple),
			act("smoke-bomb", "Smoke Bomb", "Party gains evasion; enemies lose accuracy for a turn.", SkillSmokeBomb),
			nd("vanish", "Vanish", "Become untargetable for one turn and drop aggro.", 1),
			nd("shadow-step", "Shadow Step", "Passive: bonus damage when acting before the target.", 3),
		}),
		linearTree("Venomancy", "Toxins, DoT, armor break", []SkillTreeNode{
			act("venom-strike", "Venom Strike", "Phys hit that applies Poison.", SkillVenomStrike),
			act("poison-cloud", "Poison Cloud", "AoE Poison across the enemy pack.", SkillPoisonCloud),
			act("corrosive-vial", "Corrosive Vial", "Eat away an enemy's Armor for the fight, so all hits land harder.", SkillCorrosiveVial),
			act("lacerate", "Lacerate", "A Bleed that stacks alongside Poison.", SkillLacerate),
			nd("plague", "Plague", "Capstone: poison spreads when a poisoned enemy dies.", 1),
		}),
		linearTree("Cutpurse", "Larceny, tempo, passive masteries", []SkillTreeNode{
			act("steal", "Steal", "Pickpocket the target; timing drives the chance.", SkillSteal),
			nd("mug", "Mug", "Deal damage and steal in a single hit.", 3),
			nd("lucky-strike", "Lucky Strike", "Passive: increased critical-hit chance.", 3),
			nd("fleet-footed", "Fleet Footed", "Passive: increased dodge and SPD.", 3),
			nd("killing-spree", "Killing Spree", "Capstone: a kill grants a burst of turn speed.", 1),
		}),
	},
	// ── Wizard ───────────────────────────────────────────────────────
	ClassWizard: {
		linearTree("Pyromancy", "Fire, burn, AoE detonation", []SkillTreeNode{
			act("firebolt", "Firebolt", "Single-target fire; chance to apply Burn.", SkillFirebolt),
			act("fireball", "Fireball", "AoE fire across the enemy pack; per-target Burn.", SkillFireball),
			nd("immolate", "Immolate", "A sustained Burn damage zone.", 3),
			nd("combust", "Combust", "Detonate a target's Burn stacks for a damage spike.", 3),
			nd("meteor", "Meteor", "Capstone: a massive delayed AoE.", 1),
		}),
		linearTree("Cryomancy", "Frost, slow, freeze, control", []SkillTreeNode{
			act("frost-lance", "Frost Lance", "Magic hit with a Stun proc on good timing.", SkillFrostLance),
			act("frostbite", "Frostbite", "Frost magic that always chills, reducing the target's SPD.", SkillFrostbite),
			act("cone-of-cold", "Cone of Cold", "Frost across the whole pack that chills every enemy, lowering SPD.", SkillConeOfCold),
			act("ice-armor", "Ice Armor", "Buff: attackers are chilled; gain MDef.", SkillIceArmor),
			nd("shatter", "Shatter", "Capstone: bonus damage against frozen or stunned foes.", 1),
		}),
		linearTree("Storm", "Lightning, chain, arcane utility", []SkillTreeNode{
			act("arc-bolt", "Arc Bolt", "An arc of lightning that strikes the whole pack.", SkillArcBolt),
			nd("chain-lightning", "Chain Lightning", "Lightning that bounces across the pack.", 3),
			nd("static-field", "Static Field", "Deal damage as a share of the enemy's current HP.", 3),
			nd("dispel", "Dispel", "Strip a buff or status from an enemy.", 3),
			nd("overcharge", "Overcharge", "Capstone: regenerate MP; spells may cost nothing.", 1),
		}),
	},
}

func init() {
	// Node IDs must be globally unique: findTreeNode/passives.go search per-class
	// and take the first hit, so a cross-class duplicate would mis-resolve.
	seen := map[string]bool{}
	for _, c := range AllPartyClasses() {
		trees, ok := classSkillTrees[c]
		if !ok || len(trees) != skillTreesPerClass {
			panic("core: classSkillTrees must define exactly skillTreesPerClass trees per class")
		}
		// granted: SkillIDs learned by a node in THIS class. At most one node per class may grant a
		// skill — BuySkillNode writes SkillTiers[grant]=rank-1, so a second would desync the tier.
		// Per-class, not global; a different class granting the same skill is fine.
		granted := map[SkillID]bool{}
		for _, tr := range trees {
			if tr.Name == "" {
				panic("core: skill tree with empty Name")
			}
			if len(tr.Nodes) == 0 {
				panic("core: skill tree '" + tr.Name + "' has no nodes")
			}
			inTree := map[string]bool{}
			for _, n := range tr.Nodes {
				inTree[n.ID] = true
			}
			for _, n := range tr.Nodes {
				if n.ID == "" || n.MaxRank < 1 || n.Cost < 1 {
					panic("core: malformed skill tree node in " + tr.Name)
				}
				if seen[n.ID] {
					panic("core: duplicate skill tree node id '" + n.ID + "'")
				}
				seen[n.ID] = true
				if n.Requires != "" && !inTree[n.Requires] {
					panic("core: skill tree node '" + n.ID + "' requires '" + n.Requires + "' outside its tree")
				}
				// Granting node: check the skill is player-castable and not granted twice in the class
				// (act() already fixes MaxRank at MaxSkillTier+1).
				if n.GrantSkill != SkillNone {
					if !SkillPlayerCastable(n.GrantSkill) {
						panic("core: skill tree node '" + n.ID + "' grants non-player-castable skill " + SkillName(n.GrantSkill))
					}
					if granted[n.GrantSkill] {
						panic("core: skill " + SkillName(n.GrantSkill) + " is granted by more than one node in the same class — BuySkillNode would desync its tier")
					}
					// NoUpgrades skill needs a single-rank node (actOnce): extra ranks have no ladder rows.
					if n.MaxRank > 1 && SkillHasNoUpgrades(n.GrantSkill) {
						panic("core: skill tree node '" + n.ID + "' grants NoUpgrades skill " + SkillName(n.GrantSkill) + " with MaxRank>1 — extra ranks would waste SkillPoints; use a single-rank node")
					}
					// Converse: an upgradeable skill needs a multi-rank node (act), else its tiers strand.
					if n.MaxRank <= 1 && !SkillHasNoUpgrades(n.GrantSkill) {
						panic("core: skill tree node '" + n.ID + "' grants upgradeable skill " + SkillName(n.GrantSkill) + " with MaxRank<=1 — its upgrade tiers would be unreachable; use a multi-rank node")
					}
					granted[n.GrantSkill] = true
				}
			}
		}
	}
}

// SkillTreesFor returns a class's trees. Shared authoring data — read, never mutate.
func SkillTreesFor(c PartyClass) []SkillTreeDef {
	return classSkillTrees[c]
}

// findTreeNode locates a node by id within a class's trees.
func findTreeNode(c PartyClass, id string) (SkillTreeNode, bool) {
	for _, tr := range classSkillTrees[c] {
		for _, n := range tr.Nodes {
			if n.ID == id {
				return n, true
			}
		}
	}
	return SkillTreeNode{}, false
}

// SkillTreeNodeName resolves a node id to its display name within a class.
func SkillTreeNodeName(c PartyClass, id string) (string, bool) {
	n, ok := findTreeNode(c, id)
	if !ok {
		return "", false
	}
	return n.Name, true
}

// TreeNodeRank returns the ranks invested in a node (0..MaxRank). Nil-safe.
func TreeNodeRank(m *PartyMember, id string) int {
	if m == nil || m.TreeRanks == nil {
		return 0
	}
	return m.TreeRanks[id]
}

// SkillNodeUnlocked reports whether the prerequisite holds: root always unlocked, else Requires rank >= 1.
func SkillNodeUnlocked(m *PartyMember, n SkillTreeNode) bool {
	if n.Requires == "" {
		return true
	}
	return TreeNodeRank(m, n.Requires) >= 1
}

// SkillNodeMaxed reports whether the member has bought every rank of a node.
func SkillNodeMaxed(m *PartyMember, n SkillTreeNode) bool {
	return TreeNodeRank(m, n.ID) >= n.MaxRank
}

// SkillNodeBuyable reports whether the next rank can be bought now: unlocked, not maxed, affordable.
func SkillNodeBuyable(m *PartyMember, n SkillTreeNode) bool {
	if m == nil {
		return false
	}
	return SkillNodeUnlocked(m, n) && !SkillNodeMaxed(m, n) && m.SkillPoints >= n.Cost
}

// BuySkillNode purchases one rank of node id, spending its Cost; recorded in TreeRanks. Returns
// false (no change) when unknown, locked, maxed, or unaffordable. For a GrantSkill node, rank 1
// makes the skill castable (tier 0) and each further rank advances SkillTiers[grant] one rung.
func BuySkillNode(m *PartyMember, id string) bool {
	if m == nil {
		return false
	}
	n, ok := findTreeNode(m.Class, id)
	if !ok {
		return false
	}
	if !SkillNodeBuyable(m, n) {
		return false
	}
	if m.TreeRanks == nil {
		m.TreeRanks = make(map[string]int, 8)
	}
	newRank := TreeNodeRank(m, id) + 1
	m.TreeRanks[id] = newRank
	m.SkillPoints -= n.Cost
	if n.GrantSkill != SkillNone {
		if m.SkillTiers == nil {
			m.SkillTiers = make(map[SkillID]int, 4)
		}
		// rank r -> tier r-1, capped at MaxSkillTier (rank 1 = tier 0).
		tier := newRank - 1
		if tier > MaxSkillTier {
			tier = MaxSkillTier
		}
		m.SkillTiers[n.GrantSkill] = tier
	}
	return true
}

// LearnedSkills returns the castable skills m has learned via their trees, in authoring order,
// deduped. Learned once any granting node holds rank >= 1. Source of truth for the battle Skill
// menu; a fresh member returns empty. Nil-safe.
func LearnedSkills(m *PartyMember) []SkillID {
	if m == nil {
		return nil
	}
	return LearnedSkillsInto(m, nil)
}

// LearnedSkillsInto is the buffer-reusing form of LearnedSkills for per-frame callers. Pass nil to
// allocate. Dedup is a linear slices.Contains over the tiny result, avoiding a per-call map. Nil-safe.
func LearnedSkillsInto(m *PartyMember, buf []SkillID) []SkillID {
	buf = buf[:0]
	if m == nil {
		return buf
	}
	for _, tr := range classSkillTrees[m.Class] {
		for _, n := range tr.Nodes {
			if n.GrantSkill == SkillNone || slices.Contains(buf, n.GrantSkill) {
				continue
			}
			if TreeNodeRank(m, n.ID) >= 1 {
				buf = append(buf, n.GrantSkill)
			}
		}
	}
	return buf
}

// TreeInvestedRanks sums the ranks bought across a tree (numerator of the "3 / 15" summary).
func TreeInvestedRanks(m *PartyMember, tr SkillTreeDef) int {
	total := 0
	for _, n := range tr.Nodes {
		total += TreeNodeRank(m, n.ID)
	}
	return total
}

// TreeMaxRanks sums a tree's MaxRank across every node (denominator of the summary).
func TreeMaxRanks(tr SkillTreeDef) int {
	total := 0
	for _, n := range tr.Nodes {
		total += n.MaxRank
	}
	return total
}

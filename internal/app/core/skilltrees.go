package core

// Diablo-2-style skill trees: each party class has THREE thematic trees,
// each a vertical ladder of skill nodes the player invests SkillPoints
// into. This file is the DATA + navigation layer for the trees AND the
// bridge that turns purchases into combat: a node carries name /
// description / cost / rank / prerequisite plus an optional GrantSkill.
// Buying a granting node's first rank LEARNS that skill (LearnedSkills →
// the battle Skill menu); each further rank advances the legacy per-skill
// upgrade ladder in skilltree.go (PartyMember.SkillTiers) so
// EffectiveSkillEffect folds the upgrade into casts. Nodes with no
// GrantSkill (passives, capstones, novel mechanics) still only fill pips
// for now — their effects are deferred to later slices.
//
// Design contract, mirroring the registry-invariant pattern used across
// the codebase (tiles / props / skills): the init guard below panics on
// drift — wrong tree count, duplicate node id, or a prerequisite that
// points outside its own tree — so a malformed authoring edit fails at
// process start instead of producing a silently-broken tree.

// skillTreesPerClass is the fixed number of trees every class exposes.
// Three is the design target (one offense-y, one support-y, one
// risk/identity tree per class); the modal UI lays out exactly this many
// columns.
const skillTreesPerClass = 3

// SkillTreeNode is one purchasable node in a class tree. Rank is tracked
// per party member in PartyMember.TreeRanks keyed by ID. Tier is the
// node's row within its tree (0 = root, filled in by the builder).
// Requires names the node that must hold at least one rank before this
// one unlocks (D2-style gating); "" means "always available" (the root).
type SkillTreeNode struct {
	ID       string
	Name     string
	Desc     string
	MaxRank  int
	Cost     int // SkillPoints per rank
	Tier     int
	Requires string
	// GrantSkill is the castable skill this node LEARNS at rank ≥ 1 (it then
	// appears in the battle Skill menu). Further ranks feed the legacy
	// per-skill upgrade ladder (SkillTiers) via BuySkillNode, so
	// EffectiveSkillEffect applies +damage/+proc upgrades. SkillNone = the
	// node carries no castable skill yet (a passive/novel node not wired).
	GrantSkill SkillID
}

// SkillTreeDef is one of a class's three trees: a display name, a short
// theme blurb (the kind of skills it holds), and its ordered node ladder.
type SkillTreeDef struct {
	Name  string
	Theme string
	Nodes []SkillTreeNode
}

// nd is the terse node constructor used by the tree tables — every node
// currently costs one SkillPoint per rank, so Cost is fixed here and the
// authoring rows only carry id / name / blurb / max-rank.
func nd(id, name, desc string, maxRank int) SkillTreeNode {
	return SkillTreeNode{ID: id, Name: name, Desc: desc, MaxRank: maxRank, Cost: 1}
}

// act is the node constructor for an ACTIVE node — one that LEARNS a castable
// skill (grant) at rank 1 and upgrades it with further ranks. A granting
// node's rank ladder is fixed: rank 1 learns the skill, and ranks
// 2..MaxSkillTier+1 feed the per-skill upgrade ladder (see BuySkillNode), so
// MaxRank is ALWAYS MaxSkillTier+1 (learn + every upgrade). Setting it here —
// rather than passing it per call — keeps the 10 root authoring sites from
// drifting away from MaxSkillTier. (Passive / novel nodes stay on nd, which
// takes an explicit maxRank, until their effect is wired.)
func act(id, name, desc string, grant SkillID) SkillTreeNode {
	n := nd(id, name, desc, MaxSkillTier+1)
	n.GrantSkill = grant
	return n
}

// actOnce is a single-rank granting node: it LEARNS its skill at rank 1 and
// has NO upgrade ranks (MaxRank 1). For utility skills with no damage / proc
// ladder to climb — Scan reveals HP, there's nothing for further ranks to
// improve — where act()'s MaxSkillTier+1 ranks would just be dead pips. The
// init guard treats it like any grant node (the granted skill must be
// player-castable and unique within the class).
func actOnce(id, name, desc string, grant SkillID) SkillTreeNode {
	n := nd(id, name, desc, 1)
	n.GrantSkill = grant
	return n
}

// linearTree wires a slice of nodes into a single-chain tree: each node's
// Tier becomes its index and each (past the root) requires the node above
// it. Keeps the authoring tables free of hand-numbered tiers / repeated
// Requires strings — branching trees would set those explicitly instead.
func linearTree(name, theme string, nodes []SkillTreeNode) SkillTreeDef {
	for i := range nodes {
		nodes[i].Tier = i
		if i > 0 {
			nodes[i].Requires = nodes[i-1].ID
		}
	}
	return SkillTreeDef{Name: name, Theme: theme, Nodes: nodes}
}

// classSkillTrees is the source of truth for every class's three trees.
// Authored as linear ladders for this first UI pass; the data model
// already supports arbitrary Requires for future branching.
var classSkillTrees = map[PartyClass][]SkillTreeDef{
	// ── Warrior ──────────────────────────────────────────────────────
	ClassWarrior: {
		linearTree("Battle Sense", "Disable, hinder, intercept, react", []SkillTreeNode{
			act("shield-bash", "Shield Bash", "Phys hit with a chance to Stun on good timing.", SkillCrushingBlow),
			nd("taunt", "Taunt", "Force the target enemy to attack the Warrior next turn.", 1),
			nd("guard", "Guard", "Cover an ally this round; their incoming hits redirect to you.", 3),
			nd("sunder", "Sunder", "Phys hit that pushes the target's turn later.", 3),
			nd("riposte", "Riposte", "Passive: counter when you dodge or a Guarded ally is struck.", 1),
		}),
		linearTree("Ancestral Call", "Utility, light heal, protection, summons", []SkillTreeNode{
			nd("second-wind", "Second Wind", "A small self or party heal.", 3),
			nd("war-banner", "War Banner", "Plant a banner: party gains stats while it stands.", 3),
			nd("stone-skin", "Stone Skin", "Grant an ally temporary Armor and MDef.", 3),
			nd("ancestral-spirit", "Ancestral Spirit", "Summon a warrior shade to fight beside the party.", 1),
			nd("last-stand", "Last Stand", "Capstone: once per battle, survive a lethal blow at 1 HP.", 1),
		}),
		linearTree("Fury", "Lifesteal, bleed, AoE, self-harm", []SkillTreeNode{
			act("cleave", "Cleave", "Multi-hit AoE swing across the enemy pack.", SkillSwipe),
			nd("rend", "Rend", "Phys hit that applies a Bleed damage-over-time.", 3),
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
			nd("blind", "Blind", "Lower an enemy's accuracy for several turns.", 3),
			nd("consecrate", "Consecrate", "AoE radiant damage across the enemy pack.", 3),
			nd("judgment", "Judgment", "Capstone: execute low-HP enemies for massive damage.", 1),
		}),
		linearTree("Mercy", "Restoration, regen, cleanse, revive", []SkillTreeNode{
			act("prayer", "Prayer", "Single-target heal on an ally.", SkillPrayer),
			nd("cleanse", "Cleanse", "Cure status effects on one ally.", 3),
			nd("renewal", "Renewal", "Heal-over-time regeneration on an ally.", 3),
			nd("mass-mend", "Mass Mend", "Heal the entire living party at once.", 3),
			nd("resurrect", "Resurrect", "Capstone: revive a downed party member.", 1),
		}),
		linearTree("Conviction", "Buffs, wards, retribution", []SkillTreeNode{
			nd("blessing", "Blessing", "Buff the party's stats and accuracy.", 3),
			nd("aegis", "Aegis", "Shield an ally against the next hit.", 3),
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
			nd("cripple", "Cripple", "Lower an enemy's SPD.", 3),
			nd("smoke-bomb", "Smoke Bomb", "Party gains evasion; enemies lose accuracy for a turn.", 3),
			nd("vanish", "Vanish", "Become untargetable for one turn and drop aggro.", 1),
			nd("shadow-step", "Shadow Step", "Passive: bonus damage when acting before the target.", 3),
		}),
		linearTree("Venomancy", "Toxins, DoT, armor break", []SkillTreeNode{
			act("venom-strike", "Venom Strike", "Phys hit that applies Poison.", SkillVenomStrike),
			nd("corrosive-vial", "Corrosive Vial", "Break an enemy's Armor so all hits land harder.", 3),
			nd("poison-cloud", "Poison Cloud", "AoE Poison across the enemy pack.", 3),
			nd("lacerate", "Lacerate", "A Bleed that stacks alongside Poison.", 3),
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
			nd("fireball", "Fireball", "AoE fire across the enemy pack.", 3),
			nd("immolate", "Immolate", "A sustained Burn damage zone.", 3),
			nd("combust", "Combust", "Detonate a target's Burn stacks for a damage spike.", 3),
			nd("meteor", "Meteor", "Capstone: a massive delayed AoE.", 1),
		}),
		linearTree("Cryomancy", "Frost, slow, freeze, control", []SkillTreeNode{
			act("frost-lance", "Frost Lance", "Magic hit with a Stun proc on good timing.", SkillFrostLance),
			nd("frostbite", "Frostbite", "Chill a target, reducing its SPD.", 3),
			nd("cone-of-cold", "Cone of Cold", "AoE slow across the enemy pack.", 3),
			nd("ice-armor", "Ice Armor", "Buff: attackers are chilled; gain MDef.", 3),
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
	for _, c := range []PartyClass{ClassWarrior, ClassCleric, ClassThief, ClassWizard} {
		trees, ok := classSkillTrees[c]
		if !ok || len(trees) != skillTreesPerClass {
			panic("core: classSkillTrees must define exactly skillTreesPerClass trees per class")
		}
		seen := map[string]bool{}
		// granted tracks which SkillIDs are learned by a node in THIS class's
		// trees. A skill must be granted by at most one node per class:
		// BuySkillNode writes SkillTiers[grant] = rank-1 absolutely, so a
		// second node granting the same skill would silently desync the combat
		// tier from the node's displayed rank (buying the second node at rank 1
		// would reset a skill the first node had maxed). Per-class, not global —
		// a different class learning the same skill via its own node is fine.
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
				// A node that grants a castable skill bridges to the legacy
				// per-skill upgrade ladder: rank 1 learns the skill (tier 0),
				// each further rank stacks one SkillTiers upgrade (see
				// BuySkillNode). The act() constructor fixes MaxRank at
				// MaxSkillTier+1, so the only checks left to make are that the
				// granted skill is actually player-castable and that no two
				// nodes in the class grant the same skill.
				if n.GrantSkill != SkillNone {
					if !SkillPlayerCastable(n.GrantSkill) {
						panic("core: skill tree node '" + n.ID + "' grants non-player-castable skill " + SkillName(n.GrantSkill))
					}
					if granted[n.GrantSkill] {
						panic("core: skill " + SkillName(n.GrantSkill) + " is granted by more than one node in the same class — BuySkillNode would desync its tier")
					}
					granted[n.GrantSkill] = true
				}
			}
		}
	}
}

// SkillTreesFor returns the three trees a class learns. The returned
// slice is the shared authoring data — callers read it, never mutate it.
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

// SkillTreeNodeName resolves a node id to its display name within a
// class — used by the UI to spell out a locked node's prerequisite.
func SkillTreeNodeName(c PartyClass, id string) (string, bool) {
	n, ok := findTreeNode(c, id)
	if !ok {
		return "", false
	}
	return n.Name, true
}

// TreeNodeRank returns how many ranks the member has invested in a node
// (0..MaxRank). Nil-safe and clamp-safe so a fresh member with no
// TreeRanks map reads 0 for everything.
func TreeNodeRank(m *PartyMember, id string) int {
	if m == nil || m.TreeRanks == nil {
		return 0
	}
	return m.TreeRanks[id]
}

// SkillNodeUnlocked reports whether the node's prerequisite is satisfied:
// a root node (no Requires) is always unlocked; otherwise the required
// node must hold at least one rank.
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

// SkillNodeBuyable reports whether the member can purchase the node's next
// rank right now: unlocked, not maxed, and enough SkillPoints.
func SkillNodeBuyable(m *PartyMember, n SkillTreeNode) bool {
	if m == nil {
		return false
	}
	return SkillNodeUnlocked(m, n) && !SkillNodeMaxed(m, n) && m.SkillPoints >= n.Cost
}

// BuySkillNode purchases one rank of the node `id` for member `m`,
// spending its Cost in SkillPoints. Returns false and changes nothing
// when the node is unknown, locked, maxed, or unaffordable. The rank is
// recorded in TreeRanks.
//
// For an active (GrantSkill) node the purchase also drives combat: the
// first rank makes the skill castable (it appears in the battle Skill
// menu via LearnedSkills) at tier 0 = its base effect; each further rank
// advances SkillTiers[grant] one rung up the legacy upgrade ladder so
// EffectiveSkillEffect folds the next +damage/+proc delta into casts.
// Passive / novel nodes (GrantSkill == SkillNone) record their rank only.
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
		// rank 1 → tier 0 (learn / base); rank r → tier r-1, capped at the
		// top of the ladder. The init guard fixes MaxRank == MaxSkillTier+1,
		// so a maxed node lands exactly on MaxSkillTier.
		tier := newRank - 1
		if tier > MaxSkillTier {
			tier = MaxSkillTier
		}
		m.SkillTiers[n.GrantSkill] = tier
	}
	return true
}

// LearnedSkills returns the castable skills `m` has learned through their
// skill trees, in tree/node authoring order with duplicates collapsed. A
// skill counts as learned the moment any node that grants it holds at least
// one rank. This is the source of truth for the battle Skill menu — it
// REPLACES the old fixed class loadout, so a freshly-created member (no
// ranks bought) returns an empty list and learns their first skill by
// spending their starting SkillPoint in the Tome. Nil-safe.
func LearnedSkills(m *PartyMember) []SkillID {
	if m == nil {
		return nil
	}
	var out []SkillID
	seen := map[SkillID]bool{}
	for _, tr := range classSkillTrees[m.Class] {
		for _, n := range tr.Nodes {
			if n.GrantSkill == SkillNone || seen[n.GrantSkill] {
				continue
			}
			if TreeNodeRank(m, n.ID) >= 1 {
				out = append(out, n.GrantSkill)
				seen[n.GrantSkill] = true
			}
		}
	}
	return out
}

// TreeInvestedRanks sums the ranks the member has bought across a whole
// tree — the numerator of the Skills-tab summary's "3 / 15" read.
func TreeInvestedRanks(m *PartyMember, tr SkillTreeDef) int {
	total := 0
	for _, n := range tr.Nodes {
		total += TreeNodeRank(m, n.ID)
	}
	return total
}

// TreeMaxRanks sums a tree's MaxRank across every node — the denominator
// of the "invested / total" summary read.
func TreeMaxRanks(tr SkillTreeDef) int {
	total := 0
	for _, n := range tr.Nodes {
		total += n.MaxRank
	}
	return total
}

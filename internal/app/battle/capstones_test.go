package battle

import (
	"testing"

	"crawler/internal/app/core"
)

// grantNode gives a member rank in a tree node (passive or otherwise) for testing.
func grantNode(m *core.PartyMember, id string, rank int) {
	if m.TreeRanks == nil {
		m.TreeRanks = map[string]int{}
	}
	m.TreeRanks[id] = rank
}

// TestLastStand_SurvivesThenFalls: the first lethal blow leaves the Warrior at 1 HP;
// the second (charge spent) downs them.
func TestLastStand_SurvivesThenFalls(t *testing.T) {
	g := newTestState()
	grantNode(&g.Party[0], core.PassiveLastStand, 1)
	g.Party[0].HP = 5

	damagePartyMember(g, 0, 999, core.SkillTagNone)
	if g.Party[0].HP != 1 || !g.Party[0].LastStandUsed {
		t.Fatalf("Last Stand: HP=%d used=%v, want HP=1 used=true", g.Party[0].HP, g.Party[0].LastStandUsed)
	}
	damagePartyMember(g, 0, 999, core.SkillTagNone)
	if g.Party[0].HP > 0 {
		t.Errorf("second lethal hit left HP=%d, want <=0 (charge spent)", g.Party[0].HP)
	}
}

// TestCrimsonRampage_ScalesWithMissingHP: a near-dead Warrior with the node hits harder.
func TestCrimsonRampage_ScalesWithMissingHP(t *testing.T) {
	m := &core.PartyMember{HP: 1, MaxHP: 10}
	grantNode(m, core.PassiveCrimsonRampage, 1)
	if got := applyCrimsonRampage(m, 100); got <= 100 {
		t.Errorf("Crimson Rampage at 1/10 HP gave %d, want > 100", got)
	}
	full := &core.PartyMember{HP: 10, MaxHP: 10}
	grantNode(full, core.PassiveCrimsonRampage, 1)
	if got := applyCrimsonRampage(full, 100); got != 100 {
		t.Errorf("Crimson Rampage at full HP gave %d, want 100 (no bonus)", got)
	}
}

// TestShatter_BonusVsStunned: +damage only against a stunned/frozen foe.
func TestShatter_BonusVsStunned(t *testing.T) {
	m := &core.PartyMember{}
	grantNode(m, core.PassiveShatter, 1)
	stunned := &core.Enemy{StunTurns: 1}
	if got := applyShatter(m, stunned, 100); got <= 100 {
		t.Errorf("Shatter vs stunned gave %d, want > 100", got)
	}
	awake := &core.Enemy{}
	if got := applyShatter(m, awake, 100); got != 100 {
		t.Errorf("Shatter vs un-stunned gave %d, want 100 (no bonus)", got)
	}
}

// TestJudgment_ExecutesLowHP: a foe at/under the execute fraction dies outright,
// bypassing heavy MDef; a healthy foe merely takes damage.
func TestJudgment_ExecutesLowHP(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 1 // Cleric
	g.Battle.EnemyIndex = 0
	e := &g.Packs[0].Members[0]
	e.HP, e.MaxHP, e.Armor = 3, 100, 50 // 3% HP, heavy Armor (execute bypasses defenses)

	if !applyJudgment(g, core.TimingQualityGood) {
		t.Fatal("applyJudgment reported not-landed")
	}
	if core.BattleMembers(g)[0].Alive {
		t.Errorf("Judgment did not execute a 3/100-HP foe")
	}
}

// TestCombust_DetonatesBurn: spends the target's Burn for a per-turn spike.
func TestCombust_DetonatesBurn(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 3 // Wizard
	g.Battle.EnemyIndex = 0
	e := &g.Packs[0].Members[0]
	e.HP, e.MaxHP, e.BurnTurns = 200, 200, 3

	if !applyCombust(g, core.TimingQualityGood) {
		t.Fatal("applyCombust reported not-landed")
	}
	after := core.BattleMembers(g)[0]
	if after.BurnTurns != 0 {
		t.Errorf("Combust left BurnTurns=%d, want 0 (consumed)", after.BurnTurns)
	}
	if dealt := 200 - after.HP; dealt < 3*core.CombustDamagePerBurnTurn {
		t.Errorf("Combust dealt %d, want >= %d (3 burn turns × per-turn)", dealt, 3*core.CombustDamagePerBurnTurn)
	}
}

// TestResurrect_RevivesDownedAlly: brings a downed member back at part of MaxHP.
func TestResurrect_RevivesDownedAlly(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 1 // Cleric
	g.Party[0].HP = 0         // downed Warrior

	if !applyResurrect(g, core.TimingQualityGood) {
		t.Fatal("applyResurrect reported not-landed")
	}
	if g.Party[0].HP <= 0 {
		t.Errorf("Resurrect left HP=%d, want > 0", g.Party[0].HP)
	}
}

// TestMeteor_FuseLandsAoE: when the fuse runs out, every living enemy takes the cast damage.
func TestMeteor_FuseLandsAoE(t *testing.T) {
	g := newTestState()
	for i := range g.Packs[0].Members {
		g.Packs[0].Members[i].HP, g.Packs[0].Members[i].MaxHP = 50, 50
	}
	g.Battle.MeteorFuse = 1
	g.Battle.MeteorDamage = 10

	resolveMeteorIfDue(g)
	if g.Battle.MeteorFuse != 0 {
		t.Errorf("fuse = %d, want 0 after landing", g.Battle.MeteorFuse)
	}
	for i, e := range core.BattleMembers(g) {
		if e.HP >= 50 {
			t.Errorf("enemy %d took no meteor damage (HP %d)", i, e.HP)
		}
	}
}

// TestPlague_SpreadsOnPoisonedDeath: a poisoned foe's death poisons the rest of the pack.
func TestPlague_SpreadsOnPoisonedDeath(t *testing.T) {
	g := newTestState()
	grantNode(&g.Party[2], core.PassivePlague, 1) // Thief holds Plague
	g.Packs[0].Members[0].PoisonTurns = 2         // the dying foe is poisoned
	g.Packs[0].Members[1].HP = 50                 // the other foe survives

	damageEnemy(g, 0, 999, core.TimingQualityGood, core.SkillTagNone) // kill the poisoned foe
	if core.BattleMembers(g)[1].PoisonTurns <= 0 {
		t.Error("Plague did not spread poison to the surviving foe")
	}
}

// TestVanish_UntargetableSkipped: a vanished member is skipped by enemy targeting.
func TestVanish_UntargetableSkipped(t *testing.T) {
	g := newTestState()
	g.Party[2].VanishTurns = 2 // Thief vanished
	g.Battle.EnemyAttacker = 0
	g.Battle.EnemyPendingSkill = core.SkillNone
	g.Battle.EnemyAttackCursor = 1 // scan starts at slot 2 (the vanished Thief)

	if got := core.PeekEnemyAttackerTarget(g); got == 2 {
		t.Errorf("enemy targeted the vanished member (slot 2)")
	}
	g.Party[2].VanishTurns = 0
	if got := core.PeekEnemyAttackerTarget(g); got != 2 {
		t.Errorf("after Vanish ended, target = %d, want 2 (natural)", got)
	}
}

// TestDispel_StripsBeneficialOnly: Dispel removes net-beneficial enemy mods, keeps debuffs.
func TestDispel_StripsBeneficialOnly(t *testing.T) {
	e := &core.Enemy{Debuffs: []core.StatusMod{
		{Source: core.SkillBless, Stats: core.Stats{STR: 3}, Turns: 2}, // beneficial
		{Source: core.SkillCripple, Stats: core.Stats{SPD: -3}, Turns: 2}, // our debuff
	}}
	if removed := core.DispelEnemyBuffs(e); removed != 1 {
		t.Errorf("DispelEnemyBuffs removed %d, want 1 (the beneficial mod)", removed)
	}
	if len(e.Debuffs) != 1 || e.Debuffs[0].Source != core.SkillCripple {
		t.Errorf("Dispel should keep the negative debuff, got %+v", e.Debuffs)
	}
}

// TestKillingSpree_GrantsReadinessOnKill: a kill this turn gives the Thief an ATB burst.
func TestKillingSpree_GrantsReadinessOnKill(t *testing.T) {
	g := newTestState()
	grantNode(&g.Party[2], core.PassiveKillingSpree, 1)
	g.Battle.EnemyKillsThisTurn = 1

	applyKillingSpree(g, core.ActorRef{IsParty: true, Index: 2})
	if g.Battle.Readiness[core.ActorRef{IsParty: true, Index: 2}] <= 0 {
		t.Error("Killing Spree granted no readiness after a kill")
	}
}

// TestBulwark_BuffsWholeParty: Bulwark of Faith stamps an Armor/MDef buff on the party.
func TestBulwark_BuffsWholeParty(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 1 // Cleric
	if !applyBulwark(g, core.TimingQualityGood) {
		t.Fatal("applyBulwark reported not-landed")
	}
	if len(g.Party[0].Buffs) == 0 {
		t.Error("Bulwark stamped no buff on the party")
	}
}

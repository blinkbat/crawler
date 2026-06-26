package battle

import (
	"testing"

	"crawler/internal/app/core"
)

// TestStaticField_BypassesArmorPercentHP: Static Field deals a SHARE of current HP
// and is SkillTagNone, so heavy Armor that would zero a flat hit must NOT reduce it.
func TestStaticField_BypassesArmorPercentHP(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 3 // Wizard
	g.Battle.EnemyIndex = 0
	e := &g.Packs[0].Members[0]
	e.HP, e.MaxHP, e.Armor = 100, 100, 50 // Armor 50 would zero an ~18 flat hit

	if !applyStaticField(g, core.TimingQualityExcellent) {
		t.Fatal("applyStaticField reported not-landed")
	}
	dealt := 100 - core.BattleMembers(g)[0].HP
	// 0.18 × 100 = 18 base, quality-scaled up; Armor must be bypassed entirely.
	if dealt < 18 {
		t.Errorf("Static Field dealt %d; want >= 18 (%% of current HP, armor bypassed)", dealt)
	}
}

// TestStaticField_FloorsAtOne: a tiny current-HP share rounds to 0, but the field
// must still chip at least 1 (never a free no-op hit).
func TestStaticField_FloorsAtOne(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 3
	g.Battle.EnemyIndex = 0
	e := &g.Packs[0].Members[0]
	e.HP, e.MaxHP = 1, 8 // 0.18 × 1 = 0 → must floor to 1

	if !applyStaticField(g, core.TimingQualityMiss) {
		t.Fatal("applyStaticField reported not-landed")
	}
	if got := core.BattleMembers(g)[0].HP; got != 0 {
		t.Errorf("low-HP target HP = %d, want 0 (floored 1 damage finishes it)", got)
	}
}

// TestImmolate_BurnsWholePack: Immolate's BurnChance is 1.0, so every SURVIVING
// enemy must come out burning (the guaranteed-DoT contract that distinguishes it
// from Fireball's chance roll).
func TestImmolate_BurnsWholePack(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 3 // Wizard
	for i := range g.Packs[0].Members {
		g.Packs[0].Members[i].HP = 50 // survive the blast so the burn can land
		g.Packs[0].Members[i].MaxHP = 50
	}

	if !applyImmolate(g, core.TimingQualityGood) {
		t.Fatal("applyImmolate reported not-landed")
	}
	for i, e := range core.BattleMembers(g) {
		if e.Alive && e.BurnTurns <= 0 {
			t.Errorf("enemy %d survived Immolate but isn't burning (BurnTurns %d)", i, e.BurnTurns)
		}
	}
}

// TestMug_DealsDamage: the strike half of Mug lands phys damage (the steal half is
// the proven Steal path, gated on a loot roll).
func TestMug_DealsDamage(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 2 // Thief
	g.Battle.EnemyIndex = 0
	e := &g.Packs[0].Members[0]
	e.HP, e.MaxHP = 50, 50

	if !applyMug(g, core.TimingQualityGreat) {
		t.Fatal("applyMug reported not-landed")
	}
	if dealt := 50 - core.BattleMembers(g)[0].HP; dealt <= 0 {
		t.Errorf("Mug dealt %d damage, want > 0", dealt)
	}
}

// TestGuard_RedirectsHitsToGuardian: a hit aimed at a guarded ally lands on the
// guardian instead; the cover clears when the guardian acts again.
func TestGuard_RedirectsHitsToGuardian(t *testing.T) {
	g := newTestState()
	// Warrior (0) covers Cleric (1).
	core.SetGuard(g.Party, 0, 1)
	if !g.Party[0].Guarding || !g.Party[1].Guarded || g.Party[1].GuardedBy != 0 {
		t.Fatalf("SetGuard didn't link cover: guardian=%+v ward=%+v", g.Party[0], g.Party[1])
	}

	// A melee enemy whose natural target is the ward (cursor 0 → scan starts at slot 1).
	g.Battle.EnemyAttacker = 0
	g.Battle.EnemyPendingSkill = core.SkillNone
	g.Battle.EnemyAttackCursor = 0
	if got := core.PeekEnemyAttackerTarget(g); got != 0 {
		t.Errorf("hit on guarded ally not redirected: target = %d, want 0 (guardian)", got)
	}

	// The guardian acting again drops the cover.
	core.ClearGuardBy(g.Party, 0)
	if g.Party[0].Guarding || g.Party[1].Guarded {
		t.Errorf("ClearGuardBy left flags set: guardian=%+v ward=%+v", g.Party[0], g.Party[1])
	}
	if got := core.PeekEnemyAttackerTarget(g); got != 1 {
		t.Errorf("after clear, target = %d, want 1 (natural ward, no redirect)", got)
	}
}

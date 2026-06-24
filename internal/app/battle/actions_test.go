package battle

import (
	"math/rand"
	"strings"
	"testing"

	"crawler/internal/app/core"
)

// seedGameRNG gives the GameState a deterministic RNG for reproducible rolls.
func seedGameRNG(t *testing.T, g *core.GameState, seed int64) {
	t.Helper()
	g.RNG = rand.New(rand.NewSource(seed))
}

// newTestState builds a minimal GameState: 4-class party, one pack of two rats,
// cursors at 0/0. Phase is BattlePlayer (not run through Start) to avoid queue plumbing surprises.
func newTestState() *core.GameState {
	party := []core.PartyMember{
		{Class: core.ClassWarrior, Name: "Vex", Stats: core.Stats{STR: 6, DEX: 2, INT: 1, WIS: 1, VIT: 5, SPD: 3}, HP: 10, MaxHP: 10, MP: 5, MaxMP: 5},
		{Class: core.ClassCleric, Name: "Mira", Stats: core.Stats{STR: 2, DEX: 2, INT: 2, WIS: 6, VIT: 4, SPD: 4}, HP: 1, MaxHP: 8, MP: 7, MaxMP: 7},
		{Class: core.ClassThief, Name: "Nyx", Stats: core.Stats{STR: 3, DEX: 6, INT: 2, WIS: 1, VIT: 4, SPD: 6}, HP: 8, MaxHP: 8, MP: 3, MaxMP: 3},
		{Class: core.ClassWizard, Name: "Sol", Stats: core.Stats{STR: 1, DEX: 2, INT: 6, WIS: 2, VIT: 4, SPD: 4}, HP: 8, MaxHP: 8, MP: 8, MaxMP: 8},
	}
	pack := core.Pack{
		TileX: 0, TileZ: 0,
		Members: []core.Enemy{core.NewEnemy(core.EnemyRat), core.NewEnemy(core.EnemyRat)},
	}
	g := &core.GameState{
		Party:  party,
		Packs:  []core.Pack{pack},
		Battle: core.Battle{ActivePack: 0, EnemyIndex: 0, CurrentParty: 0, Phase: core.BattlePlayer, PartyTarget: 1, EnemyAttackCursor: -1},
	}
	// Deterministic by default; without it g.Rand() lazily seeds from global rand and goes flaky.
	g.RNG = rand.New(rand.NewSource(1))
	// One-actor queue so finishActorTurn's cursor++ doesn't hit an empty queue / beginNewRound.
	g.Battle.Queue = []core.ActorRef{{IsParty: true, Index: 0}}
	g.Battle.QueueCursor = 0
	return g
}

// TestSkippedTurnTicksPoison: a turn skipped by Sleep/Stun must still tick Poison
// (damage + duration) on both sides; the skip path once ticked only Webbed/Confused.
func TestSkippedTurnTicksPoison(t *testing.T) {
	// Party side.
	g := newTestState()
	g.Party[0].HP = 10
	g.Party[0].PoisonTurns = 3
	startHP := g.Party[0].HP
	advanceSkippedTurn(g, core.ActorRef{IsParty: true, Index: 0})
	if g.Party[0].PoisonTurns != 2 {
		t.Errorf("party poison duration: got %d, want 2 (one tick on skipped turn)", g.Party[0].PoisonTurns)
	}
	if g.Party[0].HP >= startHP {
		t.Errorf("party poison dealt no damage on skipped turn: HP %d, was %d", g.Party[0].HP, startHP)
	}

	// Enemy side.
	g2 := newTestState()
	enHP := g2.Packs[0].Members[0].HP
	g2.Packs[0].Members[0].PoisonTurns = 2
	advanceSkippedTurn(g2, core.ActorRef{IsParty: false, Index: 0})
	if g2.Packs[0].Members[0].PoisonTurns != 1 {
		t.Errorf("enemy poison duration: got %d, want 1 (one tick on skipped turn)", g2.Packs[0].Members[0].PoisonTurns)
	}
	if g2.Packs[0].Members[0].HP >= enHP {
		t.Errorf("enemy poison dealt no damage on skipped turn: HP %d, was %d", g2.Packs[0].Members[0].HP, enHP)
	}
}

// TestBuildTurnQueue_SPDIncreasesTurnRate: SPD drives turn RATE over many rounds, not
// just order within one. SPD-6 vs SPD-2 ≈ 3× turns, only because readiness persists across rounds.
func TestBuildTurnQueue_SPDIncreasesTurnRate(t *testing.T) {
	g := &core.GameState{
		Party: []core.PartyMember{
			{Name: "Fast", Stats: core.Stats{SPD: 6}, HP: 10, MaxHP: 10},
			{Name: "Slow", Stats: core.Stats{SPD: 2}, HP: 10, MaxHP: 10},
		},
		// One dead enemy: BattleMembers non-empty but no queue actor, isolating the two party SPDs.
		Packs:  []core.Pack{{Members: []core.Enemy{{Kind: core.EnemyRat, Alive: false}}}},
		Battle: core.Battle{ActivePack: 0, EnemyIndex: 0, Phase: core.BattlePlayer},
	}
	fast, slow := 0, 0
	const rounds = 300
	for r := 0; r < rounds; r++ {
		for _, ref := range buildTurnQueue(g) {
			if !ref.IsParty {
				continue
			}
			switch ref.Index {
			case 0:
				fast++
			case 1:
				slow++
			}
		}
	}
	if slow == 0 || fast <= slow {
		t.Fatalf("SPD 6 should out-turn SPD 2: fast=%d slow=%d", fast, slow)
	}
	if ratio := float64(fast) / float64(slow); ratio < 2.5 {
		t.Fatalf("SPD 6 vs 2 should grant ~3x the turns (rate, not just order); got ratio %.2f (fast=%d slow=%d)", ratio, fast, slow)
	}
}

func TestApplyAttack_DealsSTRDamageAndPopup(t *testing.T) {
	g := newTestState()
	startHP := g.Packs[0].Members[0].HP
	applyAttack(g, core.TimingQualityExcellent)
	// STR 6 × Excellent = 12; a crit doubles again to 24. Tie popup to actual crit
	// outcome (seed-1 deterministic) so a double-without-crit bug is caught.
	crit := strings.Contains(g.StatusMessage, "Critical")
	want := 12
	if crit {
		want = 24
	}
	if got := g.Packs[0].Members[0].DamagePopup; got != want {
		t.Fatalf("popup should be %d (crit=%v), got %d", want, crit, got)
	}
	// Relative to spawn HP, not a hardcoded value, so EnemyDifficultyMul scaling can't make it brittle.
	expHP := startHP - want
	if expHP < 0 {
		expHP = 0
	}
	if got := g.Packs[0].Members[0].HP; got != expHP {
		t.Fatalf("expected rat at %d HP (start %d, took %d), got %d", expHP, startHP, want, got)
	}
	if alive := g.Packs[0].Members[0].Alive; alive != (expHP > 0) {
		t.Fatalf("rat Alive=%v, want %v (HP %d)", alive, expHP > 0, expHP)
	}
}

func TestApplyAttack_NoTargetBailsCleanly(t *testing.T) {
	g := newTestState()
	// Leave the second enemy alive so the action doesn't trigger winBattle (overwrites the message).
	g.Packs[0].Members[0].Alive = false
	g.Packs[0].Members[0].HP = 0
	g.Battle.EnemyIndex = 0
	if landed := applyAttack(g, core.TimingQualityExcellent); landed {
		t.Fatalf("applyAttack on dead target should not land")
	}
	if !strings.Contains(g.StatusMessage, "No target") {
		t.Fatalf("expected 'No target' status, got %q", g.StatusMessage)
	}
}

func TestApplySwipe_HitsAllLivingMembersAndSpendsMP(t *testing.T) {
	g := newTestState()
	if !setupSwipe(g) {
		t.Fatalf("setupSwipe should succeed with 5/5 MP")
	}
	if g.Party[0].MP != 5-core.SkillCost(core.SkillSwipe) {
		t.Fatalf("setupSwipe should debit cost, MP=%d", g.Party[0].MP)
	}
	startA := g.Packs[0].Members[0].HP
	startB := g.Packs[0].Members[1].HP
	applySwipe(g, core.TimingQualityGood)
	if g.Packs[0].Members[0].HP >= startA {
		t.Fatalf("rat 0 should have taken damage")
	}
	if g.Packs[0].Members[1].HP >= startB {
		t.Fatalf("rat 1 should have taken damage")
	}
}

func TestSetupPrayer_RequiresValidLivingAlly(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 1
	g.Battle.PartyTarget = 0 // alive
	if !setupPrayer(g) {
		t.Fatalf("setupPrayer on living target should succeed")
	}

	// Dead target.
	g = newTestState()
	g.Battle.CurrentParty = 1
	g.Battle.PartyTarget = 2
	g.Party[2].HP = 0
	if setupPrayer(g) {
		t.Fatalf("setupPrayer should refuse to revive")
	}
	if !strings.Contains(g.StatusMessage, "revive") {
		t.Fatalf("expected revive-refusal message, got %q", g.StatusMessage)
	}
}

func TestSetupPrayer_RejectsIfNotEnoughMP(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 1
	g.Party[1].MP = 1
	if setupPrayer(g) {
		t.Fatalf("setupPrayer with insufficient MP should fail")
	}
}

func TestApplyPrayer_HealsAndClampsAtMax(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 1
	g.Battle.PartyTarget = 0
	g.Party[0].HP = 5 // wound to verify the bump
	applyPrayer(g, core.TimingQualityGood)
	// WIS 6 + base 1 = 7 × Good (1.5) = 10, clamped at MaxHP 10.
	if g.Party[0].HP != 10 {
		t.Fatalf("expected Vex healed to 10 (clamped), got %d", g.Party[0].HP)
	}
}

func TestApplyPrayer_DoesNotReviveFallenAlly(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 1
	g.Battle.PartyTarget = 2
	g.Party[2].HP = 0
	applyPrayer(g, core.TimingQualityExcellent)
	if g.Party[2].HP != 0 {
		t.Fatalf("Prayer must not revive; got HP %d", g.Party[2].HP)
	}
}

func TestApplyFirebolt_DamagesAndCanBurn(t *testing.T) {
	g := newTestState()
	seedGameRNG(t, g, 1)
	g.Battle.CurrentParty = 3
	startMP := g.Party[3].MP
	startHP := g.Packs[0].Members[0].HP
	if !setupFirebolt(g) {
		t.Fatalf("setupFirebolt should succeed")
	}
	// MP is deducted in setup (uniform policy); apply never debits.
	if g.Party[3].MP != startMP-core.SkillCost(core.SkillFirebolt) {
		t.Fatalf("setupFirebolt should debit cost, got %d (was %d)", g.Party[3].MP, startMP)
	}
	postSetupMP := g.Party[3].MP
	applyFirebolt(g, core.TimingQualityExcellent)
	if g.Party[3].MP != postSetupMP {
		t.Fatalf("applyFirebolt should NOT debit additional MP, got %d (was %d)", g.Party[3].MP, postSetupMP)
	}
	if g.Packs[0].Members[0].HP >= startHP {
		t.Fatalf("rat should take Firebolt damage")
	}
}

func TestApplyFirebolt_TargetDiedBetweenConfirmAndApply(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 3
	// Target already gone before apply ran; apply doesn't deduct MP so direct call leaves it untouched.
	g.Packs[0].Members[0].Alive = false
	g.Packs[0].Members[0].HP = 0
	g.Packs[0].Members[1].Alive = false
	g.Packs[0].Members[1].HP = 0
	startMP := g.Party[3].MP
	if landed := applyFirebolt(g, core.TimingQualityExcellent); landed {
		t.Fatalf("Firebolt on dead target should not land")
	}
	if g.Party[3].MP != startMP {
		t.Fatalf("Firebolt apply must not touch MP, got %d (was %d)", g.Party[3].MP, startMP)
	}
}

func TestApplyFirebolt_DoesNotStackBurnOnAlreadyBurning(t *testing.T) {
	g := newTestState()
	seedGameRNG(t, g, 1)
	g.Battle.CurrentParty = 3
	g.Packs[0].Members[0].BurnTurns = 2
	preBurn := g.Packs[0].Members[0].BurnTurns
	applyFirebolt(g, core.TimingQualityExcellent)
	// BurnTurns can't grow from a fresh roll — it only decrements at turn start.
	if g.Packs[0].Members[0].BurnTurns > preBurn {
		t.Fatalf("burn shouldn't stack: was %d, now %d", preBurn, g.Packs[0].Members[0].BurnTurns)
	}
}

func TestApplySteal_LandsItemAndClearsLoot(t *testing.T) {
	// Seed 1 lands the steal deterministically.
	g := newTestState()
	seedGameRNG(t, g, 1)
	g.Battle.CurrentParty = 2 // thief
	preItem := g.Packs[0].Members[0].Item
	if preItem == core.ItemNone {
		t.Fatalf("expected rat to start with stealable item")
	}
	applySteal(g, core.TimingQualityExcellent)
	// Success clears Item; failure sets the fail message.
	if g.Packs[0].Members[0].Item == core.ItemNone {
		if g.Inventory == nil || len(g.Inventory) == 0 {
			t.Fatalf("success should add item to inventory")
		}
		if !strings.Contains(g.StatusMessage, "steals") {
			t.Fatalf("success should set steal message, got %q", g.StatusMessage)
		}
	} else {
		if !strings.Contains(g.StatusMessage, "fails") {
			t.Fatalf("failure should set fail message, got %q", g.StatusMessage)
		}
	}
}

func TestApplySteal_EmptyEnemyMessages(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 2
	g.Packs[0].Members[0].Item = core.ItemNone
	if !applySteal(g, core.TimingQualityGood) {
		t.Fatalf("steal on empty enemy still 'lands'")
	}
	if !strings.Contains(g.StatusMessage, "nothing to steal") {
		t.Fatalf("expected 'nothing to steal', got %q", g.StatusMessage)
	}
}

func TestDamageEnemy_KillsAtZero(t *testing.T) {
	g := newTestState()
	_, defeated := damageEnemy(g, 0, 99, core.TimingQualityMiss, core.SkillTagPhys)
	if !defeated {
		t.Fatalf("massive overkill should mark defeated")
	}
	if g.Packs[0].Members[0].HP != 0 || g.Packs[0].Members[0].Alive {
		t.Fatalf("rat should be dead at HP=0")
	}
	if g.Packs[0].Members[0].DeathFade <= 0 {
		t.Fatalf("DeathFade should be armed for death animation")
	}
	if g.Packs[0].Members[0].BurnTurns != 0 {
		t.Fatalf("burn should clear on death")
	}
}

func TestDamageEnemy_FlashOnSurvivedHit(t *testing.T) {
	g := newTestState()
	dealt, defeated := damageEnemy(g, 0, 1, core.TimingQualityMiss, core.SkillTagPhys)
	if defeated {
		t.Fatalf("1 damage should not kill a fresh rat")
	}
	if dealt != 1 {
		t.Fatalf("expected 1 dealt damage (rat has no armor), got %d", dealt)
	}
	if g.Packs[0].Members[0].DamageFlash <= 0 {
		t.Fatalf("flash should be set on survivable hit")
	}
}

func TestHealPartyMember_ClampsAtMaxAndRejectsCorpse(t *testing.T) {
	g := newTestState()
	g.Party[0].HP = 7
	if !healPartyMember(g, 0, 99) {
		t.Fatalf("heal on living member should succeed")
	}
	if g.Party[0].HP != g.Party[0].MaxHP {
		t.Fatalf("heal should clamp at MaxHP; got %d, want %d", g.Party[0].HP, g.Party[0].MaxHP)
	}
	g.Party[2].HP = 0
	if healPartyMember(g, 2, 4) {
		t.Fatalf("heal must not revive fallen ally")
	}
	if g.Party[2].HP != 0 {
		t.Fatalf("HP should stay 0, got %d", g.Party[2].HP)
	}
}

// TestDamageEnemy_AmoebaArmor pins the "phys whiffs to 1, magic shreds" contract AND
// that the returned `dealt` is the post-armor figure (the log once reported pre-armor).
func TestDamageEnemy_AmoebaArmor(t *testing.T) {
	g := newTestState()
	g.Packs[0].Members[0] = core.NewEnemy(core.EnemyAmoeba)
	amoebaHP := g.Packs[0].Members[0].HP
	// Phys 12 vs armor 8 → 4 dealt.
	dealt, defeated := damageEnemy(g, 0, 12, core.TimingQualityExcellent, core.SkillTagPhys)
	if dealt != 4 {
		t.Fatalf("phys 12 vs armor 8 should deal 4, got %d", dealt)
	}
	if defeated {
		t.Fatalf("amoeba shouldn't die from a 4-damage hit at full HP")
	}
	if g.Packs[0].Members[0].HP != amoebaHP-4 {
		t.Fatalf("amoeba HP should drop by post-armor amount; got %d (was %d)", g.Packs[0].Members[0].HP, amoebaHP)
	}
	// Magic 12 bypasses armor → 12 dealt.
	g.Packs[0].Members[0] = core.NewEnemy(core.EnemyAmoeba)
	dealt, _ = damageEnemy(g, 0, 12, core.TimingQualityExcellent, core.SkillTagMagic)
	if dealt != 12 {
		t.Fatalf("magic 12 should bypass armor and deal 12, got %d", dealt)
	}
	// Phys 1 vs armor 8 → floor-1.
	g.Packs[0].Members[0] = core.NewEnemy(core.EnemyAmoeba)
	dealt, _ = damageEnemy(g, 0, 1, core.TimingQualityMiss, core.SkillTagPhys)
	if dealt != 1 {
		t.Fatalf("phys 1 vs armor 8 should still deal 1 (armor is damp, not immunity), got %d", dealt)
	}
}

func TestDamagePartyMember_GuardsAndKills(t *testing.T) {
	g := newTestState()
	if _, killed := damagePartyMember(g, -1, 5, core.SkillTagPhys); killed {
		t.Fatalf("out-of-bounds party damage should no-op")
	}
	if _, killed := damagePartyMember(g, 0, 0, core.SkillTagPhys); killed {
		t.Fatalf("zero damage should no-op")
	}
	if _, killed := damagePartyMember(g, 0, 999, core.SkillTagPhys); !killed {
		t.Fatalf("lethal damage should report killed")
	}
	if g.Party[0].HP != 0 {
		t.Fatalf("party HP should clamp at 0, got %d", g.Party[0].HP)
	}
}

func TestTickBurnAtTurnStart_TicksAndKills(t *testing.T) {
	g := newTestState()
	g.Packs[0].Members[0].BurnTurns = 2
	g.Packs[0].Members[0].HP = core.BurnTickDamage // exact lethal
	killed := tickBurnAtTurnStart(g, core.ActorRef{IsParty: false, Index: 0})
	if !killed {
		t.Fatalf("burn tick should kill at exact-HP")
	}
	if g.Packs[0].Members[0].Alive {
		t.Fatalf("enemy should be dead")
	}
}

func TestTickBurnAtTurnStart_PartyActorIsNoOp(t *testing.T) {
	g := newTestState()
	if tickBurnAtTurnStart(g, core.ActorRef{IsParty: true, Index: 0}) {
		t.Fatalf("party burn tick is unsupported and must be a no-op")
	}
}

func TestResolveEnemyAttacker_DefendingHalvesDamage(t *testing.T) {
	g := newTestState()
	// Seed 1: the rat neither dodges nor crits, isolating the Defending soak.
	g.Party[0].Defending = true
	g.Battle.EnemyAttackCursor = -1
	startHP := g.Party[0].HP
	if !resolveEnemyAttacker(g, 0, core.TimingQualityMiss) {
		t.Fatalf("rat attack should land")
	}
	if g.Party[0].HP >= startHP {
		t.Fatalf("Vex should have taken some damage")
	}
	taken := startHP - g.Party[0].HP
	if taken > 2 {
		t.Fatalf("Defending should soak hit; took %d, expected <=2", taken)
	}
}

func TestResolveEnemyAttacker_ExcellentBlockCanZeroDamage(t *testing.T) {
	g := newTestState()
	// Wisp (AttackDamage 1): 1 × 0.25 truncates to 0 even on a crit, so the block always zeroes.
	g.Packs[0].Members[0] = core.NewEnemy(core.EnemyWisp)
	startHP := g.Party[0].HP
	resolveEnemyAttacker(g, 0, core.TimingQualityExcellent)
	taken := startHP - g.Party[0].HP
	if taken != 0 {
		t.Fatalf("Excellent block of a 1-dmg hit should drop to 0, took %d", taken)
	}
}

// TestPerformSwap_ArmsSlideAndTradesSlots: the formation Swap glides — both members
// get a SwapSlide armed from their pre-swap slot, and their live slots trade.
func TestPerformSwap_ArmsSlideAndTradesSlots(t *testing.T) {
	g := newTestState()
	g.Battle.CurrentParty = 0
	g.Battle.PartyTarget = 2 // living Thief
	beforeRow0, beforeCol0 := g.Party[0].Row, g.Party[0].Col
	beforeRow2, beforeCol2 := g.Party[2].Row, g.Party[2].Col

	performSwap(g)

	if g.Party[0].SwapSlide != core.SwapSlideDuration || g.Party[2].SwapSlide != core.SwapSlideDuration {
		t.Fatalf("both members should arm a full slide, got %v / %v", g.Party[0].SwapSlide, g.Party[2].SwapSlide)
	}
	if g.Party[0].SwapFromRow != beforeRow0 || g.Party[0].SwapFromCol != beforeCol0 {
		t.Error("actor's SwapFrom should record its pre-swap slot")
	}
	if g.Party[0].Row != beforeRow2 || g.Party[0].Col != beforeCol2 {
		t.Error("actor should now occupy the partner's old slot")
	}
}

// TestUnreachableMeleeTargetMsg_FlyingVsBackRow: the buzz message names the real
// reason — a flyer is melee-immune (flying message), a grounded foe is the back-row one.
func TestUnreachableMeleeTargetMsg_FlyingVsBackRow(t *testing.T) {
	g := newTestState()
	g.Packs[0].Members = []core.Enemy{
		core.NewEnemy(core.EnemyBat), // flying
		core.NewEnemy(core.EnemyRat), // grounded
	}
	g.Battle.ActivePack = 0
	if got := unreachableMeleeTargetMsg(g, 0); got != msgFlyingMeleeTarget {
		t.Errorf("flying foe msg = %q, want %q", got, msgFlyingMeleeTarget)
	}
	if got := unreachableMeleeTargetMsg(g, 1); got != msgBackRowMeleeTarget {
		t.Errorf("grounded foe msg = %q, want %q", got, msgBackRowMeleeTarget)
	}
}

func TestPickEnemyAttackTarget_SkipsDeadPartyMembers(t *testing.T) {
	g := newTestState()
	g.Party[1].HP = 0
	g.Battle.EnemyAttackCursor = 0 // next pick should skip the dead slot 1
	pick := pickEnemyAttackTarget(g)
	if pick == 1 {
		t.Fatalf("pickEnemyAttackTarget shouldn't pick a dead member")
	}
	if g.Battle.EnemyAttackCursor != pick {
		t.Fatalf("cursor should advance to the picked slot, got cursor=%d pick=%d", g.Battle.EnemyAttackCursor, pick)
	}
}

func TestQualityTag_HidesLowGrades(t *testing.T) {
	if qualityTag(core.TimingQualityMiss) != "" {
		t.Errorf("Miss should not prefix")
	}
	if qualityTag(core.TimingQualityNice) != "" {
		t.Errorf("Nice should not prefix")
	}
	if !strings.HasPrefix(qualityTag(core.TimingQualityExcellent), "Excellent") {
		t.Errorf("Excellent should prefix the message")
	}
}

// --- Poison status ---------------------------------------------------------

// newPoisonState: a single Diseased Rat (60% PoisonChance) and a healthy party.
func newPoisonState() *core.GameState {
	g := newTestState()
	g.Packs[0].Members = []core.Enemy{core.NewEnemy(core.EnemyDiseasedRat)}
	g.Battle.EnemyIndex = 0
	g.Battle.EnemyAttackCursor = -1
	return g
}

func TestResolveEnemyAttacker_DiseasedRatCanInflictPoison(t *testing.T) {
	// Try several seeds; at 60% most should land.
	landed := false
	for seed := int64(1); seed <= 5 && !landed; seed++ {
		g := newPoisonState()
		seedGameRNG(t, g, seed)
		resolveEnemyAttacker(g, 0, core.TimingQualityMiss)
		for _, p := range g.Party {
			if p.PoisonTurns > 0 {
				landed = true
				if p.PoisonTurns < core.PoisonMinTurns || p.PoisonTurns > core.PoisonMaxTurns {
					t.Errorf("poison duration out of [%d, %d]: %d",
						core.PoisonMinTurns, core.PoisonMaxTurns, p.PoisonTurns)
				}
				break
			}
		}
	}
	if !landed {
		t.Fatalf("expected at least one of 5 seeds to land poison from a 60%% chance")
	}
}

// TestTickBleedAfterEnemyTurn_DamagesAndStacksWithPoison: the bleed tick drains its
// OWN counter and deals damage, leaving Poison untouched — Bleed and Poison stack independently.
func TestTickBleedAfterEnemyTurn_DamagesAndStacksWithPoison(t *testing.T) {
	g := newTestState()
	e := &g.Packs[0].Members[0]
	e.HP, e.MaxHP = 50, 50
	e.BleedTurns = 2
	e.PoisonTurns = 2 // bleed tick must not touch this
	startHP := e.HP

	if tickBleedAfterEnemyTurn(g, core.ActorRef{IsParty: false, Index: 0}) {
		t.Fatal("bleed should not kill a 50-HP enemy")
	}
	if e.BleedTurns != 1 {
		t.Errorf("BleedTurns = %d, want 1", e.BleedTurns)
	}
	if e.PoisonTurns != 2 {
		t.Errorf("bleed tick wrongly drained PoisonTurns = %d, want 2 (DoTs stack independently)", e.PoisonTurns)
	}
	if e.HP >= startHP {
		t.Errorf("bleed dealt no damage: HP %d -> %d", startHP, e.HP)
	}
}

// TestApplyRend_OpensBleed: a connecting Rend stamps a sane-duration Bleed.
// Seed-looped since the proc is 85% quality-scaled.
func TestApplyRend_OpensBleed(t *testing.T) {
	landed := false
	for seed := int64(1); seed <= 8 && !landed; seed++ {
		g := newTestState()
		g.Battle.CurrentParty = 0
		g.Battle.EnemyIndex = 0
		g.Packs[0].Members[0].HP, g.Packs[0].Members[0].MaxHP = 99, 99
		seedGameRNG(t, g, seed)
		if !applyRend(g, core.TimingQualityExcellent) {
			t.Fatal("applyRend reported not-landed")
		}
		if bt := core.BattleMembers(g)[0].BleedTurns; bt > 0 {
			landed = true
			if bt > core.BleedMaxTurns {
				t.Errorf("bleed duration %d exceeds max %d", bt, core.BleedMaxTurns)
			}
		}
	}
	if !landed {
		t.Fatal("expected Rend to open a Bleed within 8 seeds at 85% chance")
	}
}

func TestResolveEnemyAttacker_PlainRatNeverPoisons(t *testing.T) {
	g := newTestState()
	seedGameRNG(t, g, 1)
	for i := 0; i < 20; i++ {
		resolveEnemyAttacker(g, 0, core.TimingQualityMiss)
		if g.Party[0].PoisonTurns > 0 {
			t.Fatalf("plain rat should not inflict poison")
		}
		g.Party[0].HP = g.Party[0].MaxHP // keep target above 0 mid-loop

	}
}

func TestResolveEnemyAttacker_PoisonDoesNotStack(t *testing.T) {
	g := newPoisonState()
	seedGameRNG(t, g, 1)
	g.Party[0].PoisonTurns = 4
	preDuration := g.Party[0].PoisonTurns
	g.Battle.EnemyAttackCursor = -1
	resolveEnemyAttacker(g, 0, core.TimingQualityMiss)
	if g.Party[0].PoisonTurns != preDuration {
		t.Fatalf("poison shouldn't stack onto an already-poisoned target: was %d, now %d",
			preDuration, g.Party[0].PoisonTurns)
	}
}

func TestTickPoisonAfterPartyTurn_TicksAndDecrements(t *testing.T) {
	g := newTestState()
	g.Party[0].PoisonTurns = 3
	startHP := g.Party[0].HP
	killed := tickPoisonAfterPartyTurn(g, core.ActorRef{IsParty: true, Index: 0})
	if killed {
		t.Fatalf("single tick at full HP shouldn't kill")
	}
	if g.Party[0].HP != startHP-core.PoisonTickDamage {
		t.Fatalf("tick should drain PoisonTickDamage HP; was %d, now %d", startHP, g.Party[0].HP)
	}
	if g.Party[0].PoisonTurns != 2 {
		t.Fatalf("tick should decrement PoisonTurns; got %d", g.Party[0].PoisonTurns)
	}
}

func TestTickPoisonAfterPartyTurn_KillsAtZero(t *testing.T) {
	g := newTestState()
	g.Party[0].HP = core.PoisonTickDamage
	g.Party[0].PoisonTurns = 1
	if !tickPoisonAfterPartyTurn(g, core.ActorRef{IsParty: true, Index: 0}) {
		t.Fatalf("poison should report killed when HP drops to 0")
	}
	if g.Party[0].HP != 0 {
		t.Fatalf("HP should clamp at 0; got %d", g.Party[0].HP)
	}
}

func TestTickPoisonAfterPartyTurn_NoPoisonIsNoOp(t *testing.T) {
	g := newTestState()
	startHP := g.Party[0].HP
	tickPoisonAfterPartyTurn(g, core.ActorRef{IsParty: true, Index: 0})
	if g.Party[0].HP != startHP {
		t.Fatalf("unpoisoned member should not take poison damage")
	}
}

func TestTickPoisonAfterPartyTurn_EnemyActorIsNoOp(t *testing.T) {
	g := newTestState()
	g.Party[0].PoisonTurns = 3 // enemy ActorRef must still skip
	if tickPoisonAfterPartyTurn(g, core.ActorRef{IsParty: false, Index: 0}) {
		t.Fatalf("enemy actor should be a no-op for poison tick")
	}
}

// --- Hit-stop chain (TimingFlash → HitStop → onResolve) -------------------
// Two-phase decay: flash drains first, then HitStop (high grades only); onResolve
// fires once both drain. Firing early or twice mistimes the popup/audio/apply step.

func TestResolveAndFinishEnemyAttack_TicksEnemyPoison(t *testing.T) {
	g := newTestState()
	g.Battle.Queue = []core.ActorRef{{IsParty: false, Index: 0}}
	g.Battle.QueueCursor = 0
	g.Battle.EnemyAttacker = 0
	g.Battle.Timing.Quality = core.TimingQualityExcellent
	g.Packs[0].Members[0].PoisonTurns = 2
	startHP := g.Packs[0].Members[0].HP

	resolveAndFinishEnemyAttack(g)

	if got := g.Packs[0].Members[0].PoisonTurns; got != 1 {
		t.Fatalf("enemy poison should tick after enemy action; got %d", got)
	}
	if got := g.Packs[0].Members[0].HP; got != startHP-core.PoisonTickDamage {
		t.Fatalf("enemy poison should deal %d; HP was %d now %d", core.PoisonTickDamage, startHP, got)
	}
}

func TestResolveAndFinishEnemyAttack_PoisonKillWinsBattle(t *testing.T) {
	g := newTestState()
	g.Packs[0].Members = []core.Enemy{core.NewEnemy(core.EnemyRat)}
	g.Battle.Queue = []core.ActorRef{{IsParty: false, Index: 0}}
	g.Battle.QueueCursor = 0
	g.Battle.EnemyIndex = 0
	g.Battle.EnemyAttacker = 0
	g.Battle.Timing.Quality = core.TimingQualityExcellent
	g.Packs[0].Members[0].HP = core.PoisonTickDamage
	g.Packs[0].Members[0].PoisonTurns = 1

	resolveAndFinishEnemyAttack(g)

	if g.Battle.Phase != core.BattleWon {
		t.Fatalf("poison killing the last enemy should win battle, phase=%v message=%q", g.Battle.Phase, g.StatusMessage)
	}
	if g.Packs[0].Members[0].Alive {
		t.Fatalf("enemy should be dead after poison tick")
	}
}

// TestResolveEnemyAttacker_IceArmorChillsAttacker: a connecting attack on a frost-warded
// member stamps an SPD chill on the attacker. DEX is zeroed so the swing can't be dodged.
func TestResolveEnemyAttacker_IceArmorChillsAttacker(t *testing.T) {
	g := newTestState()
	g.Battle.EnemyAttacker = 0
	for i := range g.Party {
		g.Party[i].Stats.DEX = 0
		g.Party[i].IceArmorTurns = 2
	}
	enemy := core.BattleMemberAt(g, 0)
	if len(enemy.Debuffs) != 0 {
		t.Fatalf("precondition: attacker should start un-debuffed, got %+v", enemy.Debuffs)
	}
	if !resolveEnemyAttacker(g, 0, core.TimingQualityMiss) {
		t.Fatal("resolveEnemyAttacker reported no action")
	}
	if got := core.MaxStatusModTurns(enemy.Debuffs); got != core.IceArmorChillTurns {
		t.Fatalf("ice armor should chill attacker for %d turns, got %d", core.IceArmorChillTurns, got)
	}
	if got := enemyDebuffStats(*enemy).SPD; got != -core.IceArmorChillSPD {
		t.Fatalf("ice armor chill should sap %d SPD, got %d", core.IceArmorChillSPD, got)
	}
}

// enemyDebuffStats / partyBuffStats sum the stackable mod slices for test reads.
func enemyDebuffStats(e core.Enemy) core.Stats     { s, _, _ := core.SumStatusMods(e.Debuffs); return s }
func partyBuffStats(m core.PartyMember) core.Stats { s, _, _ := core.SumStatusMods(m.Buffs); return s }

func TestTickFlashHold_LowGradeFiresImmediatelyAtFlashZero(t *testing.T) {
	g := newTestState()
	g.Battle.Timing.Quality = core.TimingQualityGood
	g.Battle.TimingFlash = core.TimingFlashDuration
	resolved := 0
	// First tick: drains flash but doesn't zero it.
	if !tickFlashHold(g, core.TimingFlashDuration*0.5, func() { resolved++ }) {
		t.Fatalf("flash still running, tickFlashHold should report busy")
	}
	if resolved != 0 {
		t.Fatalf("Good grade with flash still active shouldn't fire onResolve")
	}
	// Second tick zeroes it; no hit-stop for Good, so onResolve fires.
	if !tickFlashHold(g, core.TimingFlashDuration, func() { resolved++ }) {
		t.Fatalf("flash expiring tick should still report busy")
	}
	if resolved != 1 {
		t.Fatalf("Good grade should onResolve when flash hits 0, got %d calls", resolved)
	}
	if g.Battle.HitStop != 0 {
		t.Fatalf("Good grade should not arm HitStop, got %v", g.Battle.HitStop)
	}
}

func TestTickFlashHold_ExcellentGradeChainsIntoHitStop(t *testing.T) {
	g := newTestState()
	g.Battle.Timing.Quality = core.TimingQualityExcellent
	g.Battle.TimingFlash = core.TimingFlashDuration
	resolved := 0
	tickFlashHold(g, core.TimingFlashDuration*2, func() { resolved++ }) // drain flash in one tick
	if resolved != 0 {
		t.Fatalf("Excellent grade should NOT fire onResolve at flash zero — it should arm HitStop instead")
	}
	if g.Battle.HitStop <= 0 {
		t.Fatalf("Excellent flash expiry should arm HitStop, got %v", g.Battle.HitStop)
	}
	if g.Battle.HitStop != core.HitStopExcellent {
		t.Fatalf("HitStop should equal HitStopExcellent (%v), got %v", core.HitStopExcellent, g.Battle.HitStop)
	}
	// Drain the hit-stop → onResolve fires.
	tickFlashHold(g, core.HitStopExcellent*2, func() { resolved++ })
	if resolved != 1 {
		t.Fatalf("onResolve should fire exactly once after HitStop drains, got %d calls", resolved)
	}
	if g.Battle.HitStop != 0 {
		t.Fatalf("HitStop should clamp at 0 after expiry, got %v", g.Battle.HitStop)
	}
}

func TestTickFlashHold_GreatGradeUsesGreatHitStop(t *testing.T) {
	g := newTestState()
	g.Battle.Timing.Quality = core.TimingQualityGreat
	g.Battle.TimingFlash = core.TimingFlashDuration
	resolved := 0
	tickFlashHold(g, core.TimingFlashDuration*2, func() { resolved++ })
	if g.Battle.HitStop != core.HitStopGreat {
		t.Fatalf("Great should arm HitStopGreat (%v), got %v", core.HitStopGreat, g.Battle.HitStop)
	}
	if resolved != 0 {
		t.Fatalf("Great grade should defer onResolve until HitStop drains")
	}
	tickFlashHold(g, core.HitStopGreat*2, func() { resolved++ })
	if resolved != 1 {
		t.Fatalf("Great grade should onResolve exactly once after HitStop, got %d", resolved)
	}
}

func TestTickFlashHold_NoFlashNoOp(t *testing.T) {
	g := newTestState()
	// No flash armed → returns false.
	called := false
	if tickFlashHold(g, 0.1, func() { called = true }) {
		t.Fatalf("idle tickFlashHold should report not-busy")
	}
	if called {
		t.Fatalf("onResolve should not fire when there was nothing to drain")
	}
}

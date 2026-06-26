package core

import "testing"

func TestStageForHunger_Bands(t *testing.T) {
	cases := []struct {
		hunger int
		want   SatietyStage
	}{
		{-10, SatietyFull},
		{0, SatietyFull},
		{satietyStageSpan - 1, SatietyFull},
		{satietyStageSpan, SatietySated},
		{satietyStageSpan * 2, SatietyHungry},
		{satietyStageSpan * 3, SatietyFamished},
		{satietyStageSpan * 4, SatietyStarving},
		{SatietyMax, SatietyStarving},
		{SatietyMax + 100, SatietyStarving}, // clamps past the top band
	}
	for _, c := range cases {
		if got := StageForHunger(c.hunger); got != c.want {
			t.Errorf("StageForHunger(%d) = %d, want %d", c.hunger, got, c.want)
		}
	}
}

func TestMemberStarving(t *testing.T) {
	if MemberStarving(PartyMember{Hunger: 0}) {
		t.Error("a full member should not be starving")
	}
	if !MemberStarving(PartyMember{Hunger: SatietyMax}) {
		t.Error("a member at SatietyMax should be starving")
	}
}

func TestTickHungerStep_OnlyConscious(t *testing.T) {
	g := &GameState{Party: []PartyMember{
		{HP: 10, MaxHP: 10},                     // conscious → climbs
		{HP: 0, MaxHP: 10},                      // downed → frozen
		{HP: 10, MaxHP: 10, Ingested: true},     // ingested → frozen
		{HP: 10, MaxHP: 10, Hunger: SatietyMax}, // already empty → clamps
	}}
	TickHungerStep(g)
	if g.Party[0].Hunger != HungerPerStep {
		t.Errorf("conscious member Hunger = %d, want %d", g.Party[0].Hunger, HungerPerStep)
	}
	if g.Party[1].Hunger != 0 {
		t.Errorf("downed member burned food: Hunger = %d, want 0", g.Party[1].Hunger)
	}
	if g.Party[2].Hunger != 0 {
		t.Errorf("ingested member burned food: Hunger = %d, want 0", g.Party[2].Hunger)
	}
	if g.Party[3].Hunger != SatietyMax {
		t.Errorf("Hunger climbed past SatietyMax: %d", g.Party[3].Hunger)
	}
}

func TestFeedMember_ClampsAndReports(t *testing.T) {
	m := PartyMember{Hunger: 100}
	if got := FeedMember(&m, 30); got != 30 || m.Hunger != 70 {
		t.Errorf("FeedMember(30): restored=%d Hunger=%d, want 30/70", got, m.Hunger)
	}
	if got := FeedMember(&m, 1000); got != 70 || m.Hunger != 0 {
		t.Errorf("FeedMember(overfeed): restored=%d Hunger=%d, want 70/0", got, m.Hunger)
	}
	if got := FeedMember(&m, 0); got != 0 {
		t.Errorf("FeedMember(0) restored %d, want 0", got)
	}
}

func TestHealMember_BlockedWhileStarving(t *testing.T) {
	starving := &PartyMember{HP: 1, MaxHP: 10, Hunger: SatietyMax}
	if HealMember(starving, 5) {
		t.Error("HealMember should refuse a starving member")
	}
	if starving.HP != 1 {
		t.Errorf("starving member healed anyway: HP = %d, want 1", starving.HP)
	}
	fed := &PartyMember{HP: 1, MaxHP: 10, Hunger: 0}
	if !HealMember(fed, 5) || fed.HP != 6 {
		t.Errorf("fed member should heal: ok? HP = %d, want 6", fed.HP)
	}
}

func TestRestorePartyFully_SkipsStarving(t *testing.T) {
	g := &GameState{Party: []PartyMember{
		{HP: 1, MaxHP: 10, MP: 0, MaxMP: 5},                     // restored
		{HP: 1, MaxHP: 10, MP: 0, MaxMP: 5, Hunger: SatietyMax}, // starving → skipped
	}}
	if n := RestorePartyFully(g); n != 1 {
		t.Fatalf("restored %d, want 1 (the starving member is skipped)", n)
	}
	if g.Party[0].HP != 10 {
		t.Errorf("non-starving member not restored: HP = %d", g.Party[0].HP)
	}
	if g.Party[1].HP != 1 {
		t.Errorf("starving member restored by crystal: HP = %d, want 1", g.Party[1].HP)
	}
}

func TestHealGates_RefuseStarving(t *testing.T) {
	wounded := PartyMember{HP: 1, MaxHP: 10, Hunger: SatietyMax} // wounded AND starving
	if MemberCanBeHealed(wounded) {
		t.Error("MemberCanBeHealed should refuse a starving member (HP heal can't land)")
	}
	potion := ItemDefinition{HealAmount: 8} // pure HP heal, no food
	if ItemHelpsTarget(potion, wounded) {
		t.Error("a pure-heal item should not 'help' a starving member")
	}
	// Food still helps a starving member (it feeds, and lifts Starving so a heal lands).
	food := ItemDefinition{HealAmount: 4, SatietyGain: 60}
	if !ItemHelpsTarget(food, wounded) {
		t.Error("food should still help a starving member")
	}
	// Once fed, a pure-heal item helps again.
	fed := PartyMember{HP: 1, MaxHP: 10, Hunger: 0}
	if !MemberCanBeHealed(fed) || !ItemHelpsTarget(potion, fed) {
		t.Error("a fed, wounded member should be healable")
	}
}

func TestEffectiveStats_StarvingPenalty(t *testing.T) {
	m := PartyMember{Stats: Stats{STR: 10, DEX: 2, INT: 10, WIS: 10, VIT: 10, SPD: 10}, Hunger: SatietyMax}
	eff := EffectiveStats(m)
	if eff.STR != 10-StarvingStatPenalty {
		t.Errorf("starving STR = %d, want %d", eff.STR, 10-StarvingStatPenalty)
	}
	if eff.DEX != 0 { // 2 - 3, floored at 0
		t.Errorf("starving DEX = %d, want 0 (floored)", eff.DEX)
	}
	fed := EffectiveStats(PartyMember{Stats: Stats{STR: 10}, Hunger: 0})
	if fed.STR != 10 {
		t.Errorf("fed STR = %d, want 10 (no penalty)", fed.STR)
	}
}

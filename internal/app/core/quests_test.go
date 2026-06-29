package core

import "testing"

// TestCompleteQuest_GrantsRewardOnce: completing a quest pays its authored gold/XP
// exactly once; a second call (already complete) grants nothing.
func TestCompleteQuest_GrantsRewardOnce(t *testing.T) {
	g := &GameState{
		Gold:  10,
		Party: NewParty(),
		Quests: []Quest{
			{ID: "bounty", Title: "Bounty", Status: QuestActive, RewardGold: 25, RewardXP: 40},
		},
	}
	xpBefore := g.Party[0].XP

	if !CompleteQuest(g, "bounty") {
		t.Fatal("CompleteQuest did not transition an active quest")
	}
	if g.Gold != 35 {
		t.Errorf("gold after reward = %d, want 35 (10 + 25)", g.Gold)
	}
	if g.Party[0].XP == xpBefore {
		t.Errorf("party XP unchanged (%d) — reward XP not granted", g.Party[0].XP)
	}

	goldAfter, xpAfter := g.Gold, g.Party[0].XP
	if CompleteQuest(g, "bounty") {
		t.Error("CompleteQuest re-transitioned an already-complete quest")
	}
	if g.Gold != goldAfter || g.Party[0].XP != xpAfter {
		t.Errorf("reward granted twice: gold %d->%d, xp %d->%d", goldAfter, g.Gold, xpAfter, g.Party[0].XP)
	}
}

// TestCompleteQuest_ZeroRewardNoOp: a quest with no authored reward (the default)
// still completes but touches neither gold nor XP.
func TestCompleteQuest_ZeroRewardNoOp(t *testing.T) {
	g := &GameState{
		Gold:   7,
		Party:  NewParty(),
		Quests: []Quest{{ID: "errand", Status: QuestActive}},
	}
	xpBefore := g.Party[0].XP
	if !CompleteQuest(g, "errand") {
		t.Fatal("CompleteQuest did not transition")
	}
	if g.Gold != 7 || g.Party[0].XP != xpBefore {
		t.Errorf("zero-reward quest changed state: gold=%d xp=%d", g.Gold, g.Party[0].XP)
	}
}

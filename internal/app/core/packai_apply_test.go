package core

import "testing"

// TestApplyPackSteps_OneEngagementPerTick pins the single-engagement rule: when
// two packs' plans both land on the player's tile in one tick, only the first
// engages and the second is HELD on its current tile — never moved onto the
// player (which would leave it overlapping the player once the first battle
// resolves).
func TestApplyPackSteps_OneEngagementPerTick(t *testing.T) {
	g := &GameState{
		Player: Player{TileX: 4, TileZ: 4},
		Packs: []Pack{
			{TileX: 5, TileZ: 4},
			{TileX: 3, TileZ: 4},
		},
	}
	plans := []packAIStep{
		{PackIdx: 0, NextX: 4, NextZ: 4, EngagePlayer: true, Moved: true},
		{PackIdx: 1, NextX: 4, NextZ: 4, EngagePlayer: true, Moved: true},
	}
	if engaged := ApplyPackSteps(g, plans); engaged != 0 {
		t.Fatalf("engaged = %d, want 0 (first engager wins)", engaged)
	}
	if g.Packs[0].TileX != 4 || g.Packs[0].TileZ != 4 {
		t.Errorf("engaged pack at (%d,%d), want (4,4)", g.Packs[0].TileX, g.Packs[0].TileZ)
	}
	if g.Packs[1].TileX != 3 || g.Packs[1].TileZ != 4 {
		t.Errorf("second engager moved onto player at (%d,%d), want held at (3,4)", g.Packs[1].TileX, g.Packs[1].TileZ)
	}
}

// TestApplyPackSteps_NonEngagerStillMovesAfterEngagement confirms the guard only
// blocks a competing ENGAGER — a pack that merely wanders (EngagePlayer false)
// after an engagement is claimed still applies its move and persists its patrol
// pace direction.
func TestApplyPackSteps_NonEngagerStillMovesAfterEngagement(t *testing.T) {
	g := &GameState{
		Player: Player{TileX: 4, TileZ: 4},
		Packs: []Pack{
			{TileX: 5, TileZ: 4},
			{TileX: 1, TileZ: 1, AI: PackAIPatrol},
		},
	}
	plans := []packAIStep{
		{PackIdx: 0, NextX: 4, NextZ: 4, EngagePlayer: true, Moved: true},
		{PackIdx: 1, NextX: 2, NextZ: 1, Moved: true, PatrolDir: 1},
	}
	if engaged := ApplyPackSteps(g, plans); engaged != 0 {
		t.Fatalf("engaged = %d, want 0", engaged)
	}
	if g.Packs[1].TileX != 2 || g.Packs[1].TileZ != 1 {
		t.Errorf("non-engaging mover held at (%d,%d), want moved to (2,1)", g.Packs[1].TileX, g.Packs[1].TileZ)
	}
	if g.Packs[1].PatrolDir != 1 {
		t.Errorf("patrol dir not persisted: got %d, want 1", g.Packs[1].PatrolDir)
	}
}

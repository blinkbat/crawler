package core

import (
	"math/rand"
	"testing"
)

// TestPlanPackSteps_HeldEngagerKeepsItsTile: only one engagement resolves per
// tick, so PlanPackSteps must NOT vacate the held engager's tile (two patrols
// pace onto the player; exactly one engage plan expected).
func TestPlanPackSteps_HeldEngagerKeepsItsTile(t *testing.T) {
	openRows := func(n int) []string {
		rows := make([]string, n)
		for i := range rows {
			rows[i] = "....."
		}
		return rows
	}
	g := &GameState{
		Area: AreaDefinition{
			Width: 5, Height: 3,
			Walls: openRows(3), Floor: openRows(3), Decor: openRows(3), Props: openRows(3),
		},
		Player: Player{TileX: 2, TileZ: 1},
		Packs: []Pack{
			// Paces +1 from x=1 onto the player at x=2.
			{TileX: 1, TileZ: 1, HomeX: 1, HomeZ: 1, AI: PackAIPatrol, PatrolDir: 1, Members: []Enemy{{Alive: true}}},
			// Paces -1 from x=3 onto the player — the second engager.
			{TileX: 3, TileZ: 1, HomeX: 3, HomeZ: 1, AI: PackAIPatrol, PatrolDir: -1, Members: []Enemy{{Alive: true}}},
		},
	}
	// Seed 2's first two Float32 draws (0.167, 0.265) clear both patrol gates.
	g.RNG = rand.New(rand.NewSource(2))

	plans := PlanPackSteps(g)
	engagers := 0
	for _, p := range plans {
		if p.EngagePlayer {
			engagers++
		}
	}
	if engagers != 1 {
		t.Fatalf("PlanPackSteps emitted %d engage plans, want exactly 1 (the second engager must be held, not re-planned): %+v", engagers, plans)
	}

	// Applying must leave the second pack on its own tile.
	ApplyPackSteps(g, plans)
	if g.Packs[1].TileX != 3 || g.Packs[1].TileZ != 1 {
		t.Errorf("held second engager moved to (%d,%d), want held at (3,1)", g.Packs[1].TileX, g.Packs[1].TileZ)
	}
}

// TestApplyPackSteps_OneEngagementPerTick: when two plans land on the player's
// tile, only the first engages and the second is HELD on its current tile.
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

// TestApplyPackSteps_NonEngagerStillMovesAfterEngagement: the guard blocks only
// a competing ENGAGER — a wanderer still moves and persists its patrol dir.
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

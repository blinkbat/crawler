package core

import (
	"math/rand"
	"testing"
)

// TestPlanPackSteps_HeldEngagerKeepsItsTile guards the planning-occupancy fix:
// only ONE engagement resolves per tick (ApplyPackSteps holds a second
// engager), so PlanPackSteps must NOT vacate the held pack's tile in its
// occupancy map. Two patrol packs pace onto the player from opposite sides;
// the planner must emit exactly one engage plan (the first), leaving the
// second held in place — otherwise a later pack could plan onto the freed-but-
// still-occupied tile.
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
			// Paces +1 (default dir) from x=1 onto the player at x=2.
			{TileX: 1, TileZ: 1, HomeX: 1, HomeZ: 1, AI: PackAIPatrol, PatrolDir: 1, Members: []Enemy{{Alive: true}}},
			// Paces -1 from x=3 onto the player at x=2 — the second engager.
			{TileX: 3, TileZ: 1, HomeX: 3, HomeZ: 1, AI: PackAIPatrol, PatrolDir: -1, Members: []Enemy{{Alive: true}}},
		},
	}
	// Seed so both patrol gates (PatrolStepChance=0.6) pass this tick (seed 2's
	// first two Float32 draws are 0.167, 0.265); the patrol step itself is
	// deterministic once the gate clears.
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

	// Applying the plan must leave the second pack on its own tile — never
	// moved onto the player or onto a tile a freed reservation would expose.
	ApplyPackSteps(g, plans)
	if g.Packs[1].TileX != 3 || g.Packs[1].TileZ != 1 {
		t.Errorf("held second engager moved to (%d,%d), want held at (3,1)", g.Packs[1].TileX, g.Packs[1].TileZ)
	}
}

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

package battle

import (
	"math/rand"
	"testing"

	"crawler/internal/app/core"
)

// forceFleeSeed returns an RNG whose first flee roll (RollChance vs `chance`) lands on `success`.
func forceFleeSeed(chance float64, success bool) *rand.Rand {
	for seed := int64(1); seed < 100000; seed++ {
		if (rand.New(rand.NewSource(seed)).Float64() < chance) == success {
			return rand.New(rand.NewSource(seed))
		}
	}
	return rand.New(rand.NewSource(1))
}

// TestPerformFlee_SuccessRetreatsAndKeepsPack: a successful flee ends the battle, snaps the party to the pre-combat tile, leaves the pack.
func TestPerformFlee_SuccessRetreatsAndKeepsPack(t *testing.T) {
	g := newTestState()
	for i := range g.Party {
		g.Party[i].Level = 5 // high flee chance vs level-1 rats
	}
	g.Player.TileX, g.Player.TileZ = 2, 2
	g.Battle.FleeReturnX, g.Battle.FleeReturnZ = 5, 6

	chance := core.FleeChance(core.PartyAverageLevel(g.Party), core.PackAverageLevel(*core.ActivePack(g)))
	g.RNG = forceFleeSeed(chance, true)

	performFlee(g)

	if g.Battle.Phase != core.BattleNone {
		t.Errorf("Phase = %v after a successful flee, want BattleNone (left combat)", g.Battle.Phase)
	}
	if g.Player.TileX != 5 || g.Player.TileZ != 6 {
		t.Errorf("party not retreated: tile (%d,%d), want (5,6)", g.Player.TileX, g.Player.TileZ)
	}
	if len(g.Packs) != 1 {
		t.Errorf("pack count = %d after flee, want 1 (the fled pack stays on the field)", len(g.Packs))
	}
}

// TestPerformFlee_SuccessSkipsRepositionWhenTileOccupied: if a pack occupies the retreat tile, the flee succeeds but escapes in place.
func TestPerformFlee_SuccessSkipsRepositionWhenTileOccupied(t *testing.T) {
	g := newTestState()
	for i := range g.Party {
		g.Party[i].Level = 5
	}
	g.Player.TileX, g.Player.TileZ = 2, 2
	g.Battle.FleeReturnX, g.Battle.FleeReturnZ = 5, 6
	// Second pack occupies the retreat tile.
	g.Packs = append(g.Packs, core.Pack{TileX: 5, TileZ: 6, Members: []core.Enemy{core.NewEnemy(core.EnemyRat)}})

	chance := core.FleeChance(core.PartyAverageLevel(g.Party), core.PackAverageLevel(*core.ActivePack(g)))
	g.RNG = forceFleeSeed(chance, true)

	performFlee(g)

	if g.Battle.Phase != core.BattleNone {
		t.Errorf("Phase = %v, want BattleNone (flee still succeeds)", g.Battle.Phase)
	}
	if g.Player.TileX == 5 && g.Player.TileZ == 6 {
		t.Error("party teleported onto the pack-occupied retreat tile — should escape in place instead")
	}
	if g.Player.TileX != 2 || g.Player.TileZ != 2 {
		t.Errorf("party moved unexpectedly: tile (%d,%d), want held at (2,2)", g.Player.TileX, g.Player.TileZ)
	}
}

// TestPerformFlee_SuccessRestoresPack: a successful flee fully heals + revives the
// abandoned pack so re-engaging starts fresh (fleeing forfeits all progress).
func TestPerformFlee_SuccessRestoresPack(t *testing.T) {
	g := newTestState()
	for i := range g.Party {
		g.Party[i].Level = 5 // high flee chance vs level-1 rats
	}
	g.Player.TileX, g.Player.TileZ = 2, 2
	g.Battle.FleeReturnX, g.Battle.FleeReturnZ = 5, 6

	pack := core.ActivePack(g)
	// Wound one rat and down the other, plus stick a status on the survivor.
	pack.Members[0].HP = 1
	pack.Members[0].PoisonTurns = 3
	pack.Members[1].HP = 0
	pack.Members[1].Alive = false

	chance := core.FleeChance(core.PartyAverageLevel(g.Party), core.PackAverageLevel(*pack))
	g.RNG = forceFleeSeed(chance, true)

	performFlee(g)

	for i := range pack.Members {
		m := pack.Members[i]
		if !m.Alive {
			t.Errorf("member %d still downed after flee — should be revived", i)
		}
		if m.HP != m.MaxHP {
			t.Errorf("member %d HP = %d after flee, want full %d", i, m.HP, m.MaxHP)
		}
		if m.PoisonTurns != 0 {
			t.Errorf("member %d kept PoisonTurns=%d after flee, want 0", i, m.PoisonTurns)
		}
	}
}

// TestPerformFlee_FailureKeepsBattleAndPosition: a failed flee burns the turn — battle continues, party unmoved.
func TestPerformFlee_FailureKeepsBattleAndPosition(t *testing.T) {
	g := newTestState()
	for i := range g.Party {
		g.Party[i].Level = 1
	}
	g.Player.TileX, g.Player.TileZ = 2, 2
	g.Battle.FleeReturnX, g.Battle.FleeReturnZ = 5, 6

	chance := core.FleeChance(core.PartyAverageLevel(g.Party), core.PackAverageLevel(*core.ActivePack(g)))
	g.RNG = forceFleeSeed(chance, false)

	performFlee(g)

	if g.Battle.ActivePack < 0 {
		t.Error("battle ended on a FAILED flee — it should continue (turn burned)")
	}
	if g.Player.TileX != 2 || g.Player.TileZ != 2 {
		t.Errorf("party moved on a failed flee: tile (%d,%d), want (2,2)", g.Player.TileX, g.Player.TileZ)
	}
}

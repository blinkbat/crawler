package core

import "testing"

// TestVictoryFillProgress_Curve pins the spoils XP-bar fill easing: flat 0
// through the dance beat, eased to exactly 1 by the animation end, monotonic
// in between. Expressed against the timing consts so a retune can't silently
// break the contract.
func TestVictoryFillProgress_Curve(t *testing.T) {
	if p := VictoryFillProgress(0); p != 0 {
		t.Errorf("fill at 0 = %v, want 0", p)
	}
	if p := VictoryFillProgress(VictoryDanceBeat); p != 0 {
		t.Errorf("fill at dance beat = %v, want 0 (no fill until the pose ends)", p)
	}
	if p := VictoryFillProgress(VictorySpoilsAnimEnd()); p != 1 {
		t.Errorf("fill at anim end = %v, want 1", p)
	}
	if p := VictoryFillProgress(VictorySpoilsAnimEnd() + 5); p != 1 {
		t.Errorf("fill past anim end = %v, want clamped 1", p)
	}
	// Midpoint: ease-out quad 1-(1-0.5)^2 = 0.75 (above linear — XP rushes in
	// early, then decelerates).
	mid := VictoryDanceBeat + VictoryBarFillDuration*0.5
	if p := VictoryFillProgress(mid); absFloat(float64(p)-0.75) > 1e-6 {
		t.Errorf("fill at midpoint = %v, want 0.75 (ease-out quad)", p)
	}
	// Monotonic non-decreasing across the window.
	prev := float32(-1)
	for e := float32(0); e <= VictorySpoilsAnimEnd()+0.2; e += 0.05 {
		p := VictoryFillProgress(e)
		if p < prev {
			t.Fatalf("fill not monotonic: at %v got %v < previous %v", e, p, prev)
		}
		prev = p
	}
}

// TestVictorySpoilsAnimDone_Boundary locks the done-gate (footer swap +
// Confirm-skip target) to the fill end.
func TestVictorySpoilsAnimDone_Boundary(t *testing.T) {
	end := VictorySpoilsAnimEnd()
	if VictorySpoilsAnimDone(end - 0.01) {
		t.Error("anim reported done just before the end")
	}
	if !VictorySpoilsAnimDone(end) {
		t.Error("anim not done at the end")
	}
	if !VictorySpoilsAnimDone(end + 1) {
		t.Error("anim not done past the end")
	}
}

// TestVictoryLootRevealed_Cascade checks loot rows cascade one per stagger
// starting at the dance beat, clamp to n, and stay at 0 beforehand. Sample
// points sit safely off the exact stagger boundaries so a sub-frame float
// rounding can't flake the assertion (an off-by-one exactly on a boundary is
// invisible in the animation anyway).
func TestVictoryLootRevealed_Cascade(t *testing.T) {
	const n = 3
	if got := VictoryLootRevealed(VictoryDanceBeat-0.1, n); got != 0 {
		t.Errorf("revealed before dance beat = %d, want 0", got)
	}
	if got := VictoryLootRevealed(0, n); got != 0 {
		t.Errorf("revealed at 0 = %d, want 0", got)
	}
	if got := VictoryLootRevealed(VictoryDanceBeat+0.01, n); got != 1 {
		t.Errorf("first row not revealed just after the beat: got %d, want 1", got)
	}
	if got := VictoryLootRevealed(VictoryDanceBeat+1.5*VictoryLootStagger, n); got != 2 {
		t.Errorf("second row not revealed: got %d, want 2", got)
	}
	if got := VictoryLootRevealed(VictoryDanceBeat+10*VictoryLootStagger, n); got != n {
		t.Errorf("reveal did not clamp to n: got %d, want %d", got, n)
	}
	if got := VictoryLootRevealed(VictoryDanceBeat+1, 0); got != 0 {
		t.Errorf("revealed with no drops = %d, want 0", got)
	}
}

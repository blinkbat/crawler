package core

import (
	"math/rand"
	"testing"
)

// TestStatusResistChance_WISLinearCapped: chance scales per WIS and clamps to the cap.
func TestStatusResistChance_WISLinearCapped(t *testing.T) {
	if got := StatusResistChance(0); got != 0 {
		t.Errorf("WIS 0 should never resist, got %v", got)
	}
	if got := StatusResistChance(4); got < 0.199 || got > 0.201 { // 4 * 0.05
		t.Errorf("WIS 4 resist = %v, want ~0.20", got)
	}
	if got := StatusResistChance(1000); got != StatusResistCap {
		t.Errorf("huge WIS should clamp to cap %v, got %v", StatusResistCap, got)
	}
}

// TestResistStatusDuration_NegatesOrShortens: a resisted roll returns 0; otherwise the
// duration is WIS-shortened (never the full unshortened value at high WIS).
func TestResistStatusDuration_NegatesOrShortens(t *testing.T) {
	// WIS 0 never resists and never shortens → full duration always.
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 50; i++ {
		if got := ResistStatusDuration(rng, 5, 0); got != 5 {
			t.Fatalf("WIS 0 should pass the full duration, got %v", got)
		}
	}
	// High WIS (near the cap): over many rolls we must see at least one full resist (0)
	// and the landed ones must be shortened below 5.
	rng = rand.New(rand.NewSource(1))
	sawResist, sawLanded := false, false
	for i := 0; i < 200; i++ {
		got := ResistStatusDuration(rng, 5, 12) // 60% resist, shorten by 12/3=4 → 1
		switch {
		case got == 0:
			sawResist = true
		case got >= 5:
			t.Fatalf("landed status not shortened: got %v, want <5", got)
		default:
			sawLanded = true
		}
	}
	if !sawResist {
		t.Error("high WIS never fully resisted over 200 rolls")
	}
	if !sawLanded {
		t.Error("high WIS resisted everything — expected some shortened lands")
	}
}

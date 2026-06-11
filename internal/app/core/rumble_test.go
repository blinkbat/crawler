package core

import "testing"

// TestTriggerRumble_KeepStronger pins the arm rules: a stronger buzz overrides
// a running one, a weaker one doesn't stomp it, strength clamps to 1, and a
// non-positive strength/dur is a no-op.
func TestTriggerRumble_KeepStronger(t *testing.T) {
	var b Battle

	TriggerRumble(&b, 0.3, 1.0)
	if b.RumbleStrength != 0.3 || b.RumbleTimer != 1.0 {
		t.Fatalf("first arm: strength=%v timer=%v, want 0.3/1.0", b.RumbleStrength, b.RumbleTimer)
	}
	// Stronger overrides.
	TriggerRumble(&b, 0.8, 1.0)
	if b.RumbleStrength != 0.8 {
		t.Errorf("stronger arm should override: strength=%v, want 0.8", b.RumbleStrength)
	}
	// Weaker, while one is still running, must NOT stomp it.
	TriggerRumble(&b, 0.2, 1.0)
	if b.RumbleStrength != 0.8 {
		t.Errorf("weaker arm stomped a running stronger rumble: strength=%v, want 0.8", b.RumbleStrength)
	}
	// Strength clamps to 1.
	var c Battle
	TriggerRumble(&c, 2.0, 0.5)
	if c.RumbleStrength != 1 {
		t.Errorf("strength not clamped: %v, want 1", c.RumbleStrength)
	}
	// No-ops.
	var d Battle
	TriggerRumble(&d, 0, 1.0)
	TriggerRumble(&d, 0.5, 0)
	if d.RumbleTimer != 0 {
		t.Errorf("non-positive strength/dur should no-op: timer=%v", d.RumbleTimer)
	}
	TriggerRumble(nil, 0.5, 1.0) // must not panic
}

// TestTickRumble_DecaysToZero checks the envelope decays linearly and clamps
// the level + timer at zero.
func TestTickRumble_DecaysToZero(t *testing.T) {
	var b Battle
	TriggerRumble(&b, 0.8, 1.0)

	if lvl := TickRumble(&b, 0.5); lvl < 0.39 || lvl > 0.41 { // 0.8 * 0.5/1.0
		t.Errorf("mid-decay level = %v, want ~0.4", lvl)
	}
	if b.RumbleTimer < 0.49 || b.RumbleTimer > 0.51 {
		t.Errorf("timer after 0.5s = %v, want ~0.5", b.RumbleTimer)
	}
	// Tick past the end: clamps to 0 (both level and timer), no negative.
	if lvl := TickRumble(&b, 1.0); lvl != 0 {
		t.Errorf("over-decay level = %v, want 0", lvl)
	}
	if b.RumbleTimer != 0 {
		t.Errorf("timer went negative: %v", b.RumbleTimer)
	}
	// Inactive battle ticks to 0 without panic.
	if lvl := TickRumble(&b, 0.1); lvl != 0 {
		t.Errorf("inactive tick = %v, want 0", lvl)
	}
	if TickRumble(nil, 0.1) != 0 {
		t.Error("nil battle should tick to 0")
	}
}

// TestTriggerCombatShake_AlsoArmsRumble verifies the coupling: arming the
// camera shake also arms a proportional rumble (the haptic half of impact
// feedback), graded by the shake peak.
func TestTriggerCombatShake_AlsoArmsRumble(t *testing.T) {
	var b Battle
	TriggerCombatShake(&b, CombatShakeBigPeak, CombatShakeBigDur)
	if b.ShakeTimer <= 0 {
		t.Fatal("shake not armed")
	}
	want := CombatShakeBigPeak * RumblePerShakePeak // 0.055 * 15 = 0.825
	if b.RumbleStrength < want-0.001 || b.RumbleStrength > want+0.001 {
		t.Errorf("crit/AoE shake armed rumble strength=%v, want ~%v", b.RumbleStrength, want)
	}
	if b.RumbleTimer != CombatShakeBigDur {
		t.Errorf("rumble dur = %v, want %v (shake dur)", b.RumbleTimer, CombatShakeBigDur)
	}
	// A subtle Great press buzzes lightly, not at the big level.
	var g Battle
	peak, dur := CombatShakeFor(TimingQualityGreat)
	TriggerCombatShake(&g, peak, dur)
	if g.RumbleStrength <= 0 || g.RumbleStrength >= b.RumbleStrength {
		t.Errorf("Great-press rumble = %v, want positive but weaker than the crit/AoE %v", g.RumbleStrength, b.RumbleStrength)
	}
}

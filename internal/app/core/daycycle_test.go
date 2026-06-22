package core

import "testing"

func TestPhaseAtStepBoundaries(t *testing.T) {
	cases := []struct {
		step      int
		wantPhase TimeOfDay
		wantProg  float32
	}{
		{0, Dawn, 0},
		{12, Dawn, 12.0 / 25.0},
		{24, Dawn, 24.0 / 25.0},
		{25, Morning, 0},
		{50, Afternoon, 0},
		{74, Afternoon, 24.0 / 25.0},
		{75, Dusk, 0},
		{100, Evening, 0},
		{125, Midnight, 0},
		{149, Midnight, 24.0 / 25.0},
		// Wrap: step 150 lands on Dawn again.
		{150, Dawn, 0},
		{151, Dawn, 1.0 / 25.0},
		{300, Dawn, 0},
	}
	for _, tc := range cases {
		gotPhase, gotProg := PhaseAtStep(tc.step)
		if gotPhase != tc.wantPhase {
			t.Errorf("step %d: phase = %v, want %v", tc.step, gotPhase, tc.wantPhase)
		}
		if abs(gotProg-tc.wantProg) > 1e-5 {
			t.Errorf("step %d: progress = %f, want %f", tc.step, gotProg, tc.wantProg)
		}
	}
}

func TestPhaseAtStepNegativeClamps(t *testing.T) {
	// Negative step counts shouldn't blow up — clamp to 0 (Dawn).
	phase, prog := PhaseAtStep(-1)
	if phase != Dawn || prog != 0 {
		t.Errorf("got (%v, %f), want (Dawn, 0)", phase, prog)
	}
}

func abs(f float32) float32 {
	if f < 0 {
		return -f
	}
	return f
}

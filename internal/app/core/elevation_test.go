package core

import "testing"

// elevTestArea is a 3×3 map with a ramp ascending North at (1,1):
//
//	elevation     floor
//	  0 1 0       . . .
//	  0 0 0       . ^ .
//	  0 0 0       . . .
//
// So (1,0) is a level-1 plateau, the ramp at (1,1) (low level 0) bridges the
// level-0 ground at (1,2) up to it, and (1,1)'s east/west sides are sheer.
func elevTestArea() AreaDefinition {
	return AreaDefinition{
		Width:  3,
		Height: 3,
		Floor: []string{
			"...",
			".^.",
			"...",
		},
		Elevation: []string{
			"010",
			"000",
			"000",
		},
	}
}

func TestElevationReads(t *testing.T) {
	a := elevTestArea()
	if got := a.ElevationLevelAt(1, 0); got != 1 {
		t.Errorf("ElevationLevelAt(1,0) = %d, want 1", got)
	}
	if got := a.ElevationLevelAt(0, 0); got != 0 {
		t.Errorf("ElevationLevelAt(0,0) = %d, want 0", got)
	}
	if got := a.ElevationLevelAt(9, 9); got != 0 {
		t.Errorf("ElevationLevelAt(out of bounds) = %d, want 0", got)
	}
	if f, ok := a.RampAt(1, 1); !ok || f != North {
		t.Errorf("RampAt(1,1) = (%d,%v), want (North,true)", f, ok)
	}
	if _, ok := a.RampAt(0, 0); ok {
		t.Errorf("RampAt(0,0) ok=true, want false (flat floor)")
	}
}

func TestStandGroundY(t *testing.T) {
	a := elevTestArea()
	if got, want := a.StandGroundY(0, 0), float32(0); got != want {
		t.Errorf("StandGroundY flat L0 = %v, want %v", got, want)
	}
	if got, want := a.StandGroundY(1, 0), 1*LevelStep; got != want {
		t.Errorf("StandGroundY flat L1 = %v, want %v", got, want)
	}
	if got, want := a.StandGroundY(1, 1), 0.5*LevelStep; got != want {
		t.Errorf("StandGroundY ramp(low 0) = %v, want %v (mid-slope)", got, want)
	}
}

func TestStepElevation(t *testing.T) {
	a := elevTestArea()
	cases := []struct {
		name        string
		fx, fz, dir int
		want        bool
	}{
		{"flat low onto ramp's low side (walk up)", 1, 2, North, true},
		{"ramp off the top onto high flat", 1, 1, North, true},
		{"high flat onto ramp's high side (walk down)", 1, 0, South, true},
		{"flat to a higher flat is a cliff", 0, 0, East, false},
		{"mounting a ramp from the side is blocked", 0, 1, East, false},
		{"flat to flat at the same level", 0, 2, East, true},
	}
	for _, c := range cases {
		if got := a.StepElevationOK(c.fx, c.fz, c.dir); got != c.want {
			t.Errorf("%s: StepElevationOK(%d,%d,%d)=%v, want %v", c.name, c.fx, c.fz, c.dir, got, c.want)
		}
	}
}

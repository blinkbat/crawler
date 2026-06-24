package core

import (
	"math"
	"testing"
)

// TestWrapAngle folds angles into [-π, π] so a yaw difference eases the short way.
func TestWrapAngle(t *testing.T) {
	const eps = 1e-4
	cases := []struct {
		in, want float32
	}{
		{0, 0},
		{math.Pi, math.Pi},
		{-math.Pi, -math.Pi},
		{1.5 * math.Pi, -0.5 * math.Pi}, // 270° → -90°, the short way
		{-1.5 * math.Pi, 0.5 * math.Pi}, // -270° → +90°
		{3 * math.Pi, math.Pi},          // multiple wraps
		{0.5 * math.Pi, 0.5 * math.Pi},  // already short
	}
	for _, c := range cases {
		if got := WrapAngle(c.in); math.Abs(float64(got-c.want)) > eps {
			t.Errorf("WrapAngle(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

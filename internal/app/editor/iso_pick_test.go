package editor

import (
	"math"
	"testing"

	"crawler/internal/app/core"
	rl "github.com/gen2brain/raylib-go/raylib"
)

// TestIsoPickCentreHits guards the 3D-view ray pick: with the default orbit
// camera, a mouse at the panel centre must resolve an in-bounds tile. Regression
// for the Z-flipped hand-rolled unprojection that made isoPick return (-1,-1)
// everywhere (nothing was paintable in 3D). isoPick is CPU-only math, so this
// runs headless.
func TestIsoPickCentreHits(t *testing.T) {
	s := freshState(blankArea(8, 8, core.FloorAuto))
	s.rect.grid = rl.NewRectangle(300, 80, 640, 640)

	minL, maxL := isoLevelSpan(&s)
	cam := s.isoCamera(minL, maxL)
	mp := rl.NewVector2(s.rect.grid.X+s.rect.grid.Width/2, s.rect.grid.Y+s.rect.grid.Height/2)

	x, z := s.isoPick(cam, mp, minL)
	if x < 0 || z < 0 || !s.area.InBounds(x, z) {
		t.Fatalf("panel-centre pick returned out-of-bounds tile (%d,%d); expected an in-bounds cell", x, z)
	}
}

// TestRayAABBHit pins the pure-Go slab test that replaced the CGo
// rl.GetRayCollisionBox in the per-column pick loop: entry distance on a hit, miss
// when pointing away or parallel-and-outside, and distance 0 when the origin is
// inside the box.
func TestRayAABBHit(t *testing.T) {
	lo, hi := rl.NewVector3(-1, -1, -1), rl.NewVector3(1, 1, 1)
	cases := []struct {
		name     string
		pos, dir rl.Vector3
		wantHit  bool
		wantDist float32
	}{
		{"down onto top face", rl.NewVector3(0, 5, 0), rl.NewVector3(0, -1, 0), true, 4},
		{"pointing away", rl.NewVector3(0, 5, 0), rl.NewVector3(0, 1, 0), false, 0},
		{"origin inside", rl.NewVector3(0, 0, 0), rl.NewVector3(0, -1, 0), true, 0},
		{"parallel above the box", rl.NewVector3(0, 5, 0), rl.NewVector3(1, 0, 0), false, 0},
	}
	for _, c := range cases {
		dist, hit := rayAABBHit(rl.Ray{Position: c.pos, Direction: c.dir}, lo, hi)
		if hit != c.wantHit {
			t.Errorf("%s: hit = %v, want %v", c.name, hit, c.wantHit)
			continue
		}
		if hit && math.Abs(float64(dist-c.wantDist)) > 1e-4 {
			t.Errorf("%s: dist = %v, want %v", c.name, dist, c.wantDist)
		}
	}
}

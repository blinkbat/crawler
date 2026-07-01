package editor

import (
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

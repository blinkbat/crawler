package editor

import (
	"testing"

	"crawler/internal/app/core"
	rl "github.com/gen2brain/raylib-go/raylib"
)

func TestIsoCamTopDownFraming(t *testing.T) {
	area, err := core.LoadArea("../../../maps/forest_path.map")
	if err != nil {
		t.Skipf("load: %v", err)
	}
	s := freshState(area)
	surfaceAreaLevels(&s)
	w, h := int32(640), int32(640)
	minL, maxL := isoLevelSpan(&s)
	cx := float32(s.area.Width) * core.TileSize / 2
	cz := float32(s.area.Height) * core.TileSize / 2
	midY := (core.ElevationWorldY(minL) + core.ElevationWorldY(maxL)) / 2
	t.Logf("map %dx%d levels [%d..%d]", s.area.Width, s.area.Height, minL, maxL)
	for _, p := range []float32{0.66, 1.2, 1.45, 1.50} {
		s.isoPitch = p
		cam := s.isoCamera(minL, maxL)
		ctr := rl.GetWorldToScreenEx(rl.NewVector3(cx, midY, cz), cam, w, h)
		// project the four map corners too
		c00 := rl.GetWorldToScreenEx(rl.NewVector3(0, midY, 0), cam, w, h)
		c11 := rl.GetWorldToScreenEx(rl.NewVector3(float32(s.area.Width)*core.TileSize, midY, float32(s.area.Height)*core.TileSize), cam, w, h)
		t.Logf("pitch=%.2f camPos=%v centre=%v corner00=%v corner11=%v", p, cam.Position, ctr, c00, c11)
	}
}

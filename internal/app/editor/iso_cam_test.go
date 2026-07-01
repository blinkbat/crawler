package editor

import (
	"testing"

	"crawler/internal/app/core"
	rl "github.com/gen2brain/raylib-go/raylib"
)

// TestIsoCamFramingFitsPanel guards the 3D-view fit: across the pitch range
// (including extreme top-down), a real non-square map must stay centred and keep
// all 8 bounding-box corners inside the panel. Regression for the angle/aspect-
// blind bounding-sphere fit that shrank steep top-down to a tiny off-centre strip.
func TestIsoCamFramingFitsPanel(t *testing.T) {
	area, err := core.LoadArea("../../../maps/forest_path.map")
	if err != nil {
		t.Skipf("load: %v", err)
	}
	s := freshState(area)
	surfaceAreaLevels(&s)
	const w, h = int32(640), int32(640)
	s.rect.grid = rl.NewRectangle(0, 0, float32(w), float32(h))
	minL, maxL := isoLevelSpan(&s)
	cx := float32(s.area.Width) * core.TileSize / 2
	cz := float32(s.area.Height) * core.TileSize / 2
	yLo, yHi := core.ElevationWorldY(minL), core.ElevationWorldY(maxL)+core.LevelStep

	for _, p := range []float32{0.66, 1.2, 1.45, 1.50} {
		s.isoPitch = p
		cam := s.isoCamera(minL, maxL)
		ctr := rl.GetWorldToScreenEx(rl.NewVector3(cx, (yLo+yHi)/2, cz), cam, w, h)
		if dx := ctr.X - float32(w)/2; dx < -2 || dx > 2 {
			t.Errorf("pitch %.2f: map centre off-centre horizontally: x=%.1f (want ~%d)", p, ctr.X, w/2)
		}
		for _, X := range []float32{0, float32(s.area.Width) * core.TileSize} {
			for _, Y := range []float32{yLo, yHi} {
				for _, Z := range []float32{0, float32(s.area.Height) * core.TileSize} {
					sp := rl.GetWorldToScreenEx(rl.NewVector3(X, Y, Z), cam, w, h)
					if sp.X < 0 || sp.X > float32(w) || sp.Y < 0 || sp.Y > float32(h) {
						t.Errorf("pitch %.2f: corner (%.0f,%.0f,%.0f) projects off-panel: %v", p, X, Y, Z, sp)
					}
				}
			}
		}
	}
}

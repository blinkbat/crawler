package render

import (
	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Wall-feature fixture tints (unlit markers, deliberately readable against stone).
var (
	wallSwitchPlate   = rl.NewColor(72, 60, 44, 255) // dark iron backing plate
	wallSwitchKnob    = brassStud                    // brass lever knob (shared with door studs)
	wallBombablePanel = rl.NewColor(96, 78, 66, 255) // cracked masonry patch
	wallBombableSeam  = rl.NewColor(30, 24, 20, 255) // dark fracture line
)

// DrawWallFeatures paints a small fixture on each authored wall feature's face: a
// brass lever (switch) or a cracked masonry patch (bombable). Secret walls draw
// NOTHING in-game — they must look like ordinary walls until found by bumping. Must
// run inside the same BeginMode3D pass as DrawWorld (see run.go).
func DrawWallFeatures(camera rl.Camera3D, g *core.GameState, assets Resources) {
	vc := newViewCull(camera)
	for _, f := range g.Area.WallFeatures {
		if f.Kind == core.WallSecret {
			continue // hidden by design
		}
		dx, dz := core.FacingVector(f.Dir)
		groundY := g.Area.StandGroundYAt(f.X, g.Area.ElevationLevelAt(f.X, f.Z), f.Z)
		// Sit on the tile's face: center pushed ~half a tile toward the face, mid-height.
		cx := core.TileCenter(f.X) + float32(dx)*0.48
		cz := core.TileCenter(f.Z) + float32(dz)*0.48
		cy := groundY + 0.55
		pos := rl.NewVector3(cx, cy, cz)
		if vc.cull(pos) {
			continue
		}
		// Face-aligned thin dimension: N/S faces are thin in Z, E/W faces thin in X.
		thin := dz != 0 // true when the face points along Z (N/S): keep it flat against that wall
		switch f.Kind {
		case core.WallSwitch:
			if thin {
				rl.DrawCube(pos, 0.16, 0.30, 0.06, wallSwitchPlate)
			} else {
				rl.DrawCube(pos, 0.06, 0.30, 0.16, wallSwitchPlate)
			}
			rl.DrawSphere(rl.NewVector3(cx, cy+0.09, cz), 0.055, wallSwitchKnob)
		case core.WallBombable:
			if thin {
				rl.DrawCube(pos, 0.40, 0.40, 0.06, wallBombablePanel)
				rl.DrawCube(rl.NewVector3(cx, cy, cz), 0.06, 0.42, 0.07, wallBombableSeam)
			} else {
				rl.DrawCube(pos, 0.06, 0.40, 0.40, wallBombablePanel)
				rl.DrawCube(rl.NewVector3(cx, cy, cz), 0.07, 0.42, 0.06, wallBombableSeam)
			}
		default:
			// WallSecret is handled by the continue above (draws nothing in-game); any
			// OTHER kind is a new core.WallFeatureKind added without a fixture here —
			// fail loud (matches the package's unhandled-enum discipline) rather than
			// silently drawing an invisible feature.
			panic("render: DrawWallFeatures has no fixture for WallFeatureKind " + string(f.Kind))
		}
	}
}

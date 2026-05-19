package render

import (
	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// DrawDoors paints every authored door in the current area as a wooden
// doorframe at its tile center, rotated to face the player's
// post-transition direction. The lighting shader is bound by DrawWorld
// before this call so the door picks up the same per-area profile;
// callers must invoke DrawDoors inside the same rl.BeginMode3D pass.
//
// Doors don't have a "looted" or "open" state today — they render
// identically before and after the player steps through. A future
// "locked" state can add a closed-panel variant or recolor the brass
// keystone.
func DrawDoors(camera rl.Camera3D, g core.GameState, assets Resources) {
	for _, d := range g.Doors {
		center := tileWorldPos(d.TileX, d.TileZ, 0)
		yaw := doorYawDeg(d.Facing)
		assets.doorProp.draw(center, 1.0, yaw)
	}
}

// doorYawDeg maps a core.Facing direction to the degree rotation the
// doorframe's mesh needs so the opening points in that direction. The
// mesh is authored facing +Z (south), so North needs a 180° flip,
// East/West get 90°/270°.
func doorYawDeg(facing int) float32 {
	switch core.NormalizeFacing(facing) {
	case core.North:
		return 180
	case core.East:
		return 90
	case core.South:
		return 0
	case core.West:
		return 270
	}
	return 0
}

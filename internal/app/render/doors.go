package render

import (
	"strings"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// DrawDoors paints every authored door as a doorframe at its tile center, rotated to face the
// player's post-transition direction. Must be called inside the same rl.BeginMode3D pass as DrawWorld
// (shares its bound lighting shader).
func DrawDoors(camera rl.Camera3D, g *core.GameState, assets Resources) {
	vc := newViewCull(camera)
	for _, d := range g.Doors {
		center := tileWorldPos(d.TileX, d.TileZ, g.Area.StandGroundYAt(d.TileX, d.Level, d.TileZ))
		if vc.cull(center) {
			continue
		}
		yaw := southFacingYaw(d.Facing)
		// Out-of-range style falls back to Building (index 0, always present).
		style := clampTableIndex(d.Style, len(assets.doorProps), core.DoorStyleBuilding)
		assets.doorProps[style].draw(center, 1.0, yaw)
	}
}

// DrawDoorPrompt paints the "enter this doorway?" confirm modal (active when g.DoorPrompt >= 0).
// 2D overlay drawn after the world pass; no-op when no prompt is active.
func DrawDoorPrompt(g *core.GameState, assets Resources) {
	if g.DoorPrompt < 0 || g.DoorPrompt >= len(g.Doors) {
		return
	}
	target := g.Doors[g.DoorPrompt].TargetMap
	if target == core.SelfMapToken {
		// Same-map portal: show the current area's own name, not "Self".
		target = core.MapIDFromPath(g.Area.Path)
	}
	dest := humanizeMapID(target)
	drawConfirmModal(assets.hudFont, "DOORWAY", "Travel to "+dest+"?", []HintSeg{
		Hint("Enter", GlyphA),
		Hint("Stay", GlyphB),
	})
}

// humanizeMapID turns a bare map id ("forgotten_plaza") into a display label ("Forgotten Plaza");
// empty id falls back to "the next area". Single-entry memo skips the FieldsFunc/Join allocs on
// the per-frame held-prompt calls.
var (
	humanizeCacheIn  string
	humanizeCacheOut string
)

func humanizeMapID(id string) string {
	if id == "" {
		return "the next area"
	}
	if id == humanizeCacheIn {
		return humanizeCacheOut
	}
	words := strings.FieldsFunc(id, func(r rune) bool { return r == '_' || r == '-' })
	for i, w := range words {
		if w == "" {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	out := strings.Join(words, " ")
	if out == "" {
		// id was all separators — FieldsFunc yielded no words; reuse the empty-id fallback.
		out = "the next area"
	}
	humanizeCacheIn, humanizeCacheOut = id, out
	return out
}

//go:build debug

package render

import (
	"fmt"
	"sort"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// DebugBuildEnabled is true in debug builds; debug_release.go sets it to
// false. The pause menu reads it to gate the Debug Overlay label.
const DebugBuildEnabled = true

// This file is compiled only with `go build -tags debug`. The release
// build picks up debug_release.go's no-op DrawDebugOverlay stub instead,
// so the tile-label sort/walk and the per-frame label-allocation buffer
// never reach the shipped binary. The pause menu's "Debug overlay"
// toggle still flips g.DebugOverlay either way — in release builds it's
// just an inert flag.

// debugLabelRange is how many tiles around the player get labelled. 4 covers
// the immediate "what am I standing next to" question without flooding the
// screen — the labels overlap badly past ~25 tiles even sorted by depth.
const debugLabelRange = 4

// debugLabelsBuf reuses the per-tile label slice across frames so the
// overlay's hot loop doesn't allocate a fresh slice every draw when
// enabled. Renderer is single-threaded, so re-slicing is safe.
var debugLabelsBuf = make([]labelStack, 0, (2*debugLabelRange+1)*(2*debugLabelRange+1))

type labelStack struct {
	screen rl.Vector2
	lines  []string
	dist   float32
}

// DrawDebugOverlay paints the in-world tile labels and a coord readout when
// g.DebugOverlay is on. Each visible tile within debugLabelRange of the
// player gets a screen-space label stack identifying its floor / decor /
// prop and (x,z) coords so the author can verify what they're looking at.
// The top-left corner shows the player's own (x,z), facing, step count,
// and map name.
//
// Off by default; toggled from the pause menu. No-op when DebugOverlay is
// false OR when the pause menu is open (labels would just clutter the
// pause UI), so calling unconditionally from the scene draw is free.
func DrawDebugOverlay(camera rl.Camera3D, g core.GameState, assets Resources) {
	if !g.DebugOverlay || g.MenuOpen {
		return
	}
	m := g.Area

	facing, _ := core.FacingName(g.Player.Facing)
	header := []string{
		fmt.Sprintf("X=%d  Z=%d  %s", g.Player.TileX, g.Player.TileZ, facing),
		fmt.Sprintf("step %d  %s", g.StepCount, m.Name),
	}
	debugHeadingCol := rl.NewColor(186, 240, 186, 245)
	for i, line := range header {
		x, y := float32(14), float32(12+i*22)
		rl.DrawTextEx(assets.hudFont, line, rl.NewVector2(x+1, y+1), 22, 1.2, shadowHeavy)
		rl.DrawTextEx(assets.hudFont, line, rl.NewVector2(x, y), 22, 1.2, debugHeadingCol)
	}

	// Pre-compute forward so we can cheaply drop tiles behind the camera.
	forward := horizontalForward(camera)
	camPos := camera.Position
	pX := g.Player.TileX
	pZ := g.Player.TileZ
	screenW, screenH := screenSizeF()

	labels := debugLabelsBuf[:0]

	for dz := -debugLabelRange; dz <= debugLabelRange; dz++ {
		for dx := -debugLabelRange; dx <= debugLabelRange; dx++ {
			x := pX + dx
			z := pZ + dz
			if !m.InBounds(x, z) {
				continue
			}
			cx := core.TileCenter(x)
			cz := core.TileCenter(z)
			wdx := cx - camPos.X
			wdz := cz - camPos.Z
			// Drop tiles behind the camera. GetWorldToScreen mis-projects
			// behind-camera points into the visible range, so the dot
			// product check is the reliable filter.
			if wdx*forward.X+wdz*forward.Z < -1.5 {
				continue
			}
			lines := tileLabelLines(m, x, z, x == pX && z == pZ)
			if len(lines) == 0 {
				continue
			}
			lines = append(lines, fmt.Sprintf("(%d,%d)", x, z))
			// Float the label ~1.6 units above the floor so it sits above
			// the tile's prop instead of inside it.
			screen := rl.GetWorldToScreen(rl.NewVector3(cx, 1.6, cz), camera)
			if screen.X < -120 || screen.X > screenW+120 || screen.Y < -80 || screen.Y > screenH+80 {
				continue
			}
			labels = append(labels, labelStack{
				screen: screen,
				lines:  lines,
				dist:   wdx*wdx + wdz*wdz,
			})
		}
	}

	// Paint far labels first so near labels overdraw — without a depth
	// buffer for 2D text, this is the only way to keep nearby tiles
	// readable when many labels stack.
	sort.Slice(labels, func(i, j int) bool {
		return labels[i].dist > labels[j].dist
	})

	for _, lbl := range labels {
		y := lbl.screen.Y
		for _, line := range lbl.lines {
			drawDebugLabel(assets.hudFont, line, lbl.screen.X, y)
			y += 19
		}
	}
	debugLabelsBuf = labels
}

// tileLabelLines returns the readable layer names for the tile at (x, z).
// Walls short-circuit floor/decor/prop labels — there's no surface under a
// solid wall to label separately. youHere prepends a "YOU" marker for the
// tile the player is standing on.
func tileLabelLines(m core.AreaDefinition, x, z int, youHere bool) []string {
	var lines []string
	if youHere {
		lines = append(lines, "YOU")
	}
	if m.Walls[z][x] == core.TileRock {
		return append(lines, "Wall")
	}
	if f := core.TileLabel(core.TileLayerFloor, m.Floor[z][x]); f != "" {
		lines = append(lines, f)
	}
	if d := core.TileLabel(core.TileLayerDecor, m.Decor[z][x]); d != "" {
		lines = append(lines, d)
	}
	if p := core.TileLabel(core.TileLayerProps, m.Props[z][x]); p != "" {
		lines = append(lines, p)
	}
	return lines
}

// drawDebugLabel paints one line of a debug label stack — black-translucent
// pill behind, two-pass text (shadow + foreground) on top so the label
// reads cleanly over any 3D background.
func drawDebugLabel(font rl.Font, text string, x, y float32) {
	if text == "" {
		return
	}
	const size = float32(15)
	const spacing = float32(1)
	measure := rl.MeasureTextEx(font, text, size, spacing)
	rx := x - measure.X/2
	ry := y - measure.Y/2
	const pad = float32(4)
	rl.DrawRectangle(int32(rx-pad), int32(ry-pad/2), int32(measure.X+pad*2), int32(measure.Y+pad), rl.NewColor(0, 0, 0, 185))
	rl.DrawTextEx(font, text, rl.NewVector2(rx+1, ry+1), size, spacing, shadowHeavy)
	rl.DrawTextEx(font, text, rl.NewVector2(rx, ry), size, spacing, rl.NewColor(220, 240, 220, 245))
}

package render

import (
	"fmt"
	"sort"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Overlay is always-available (not build-tag gated): g.DebugOverlay is false by default and every
// line is behind an early-return guard, so release overhead with it off is zero.

// debugLabelRange: tiles around the player to label. Past ~25 labels overlap badly even depth-sorted.
const debugLabelRange = 4

// debugLabelSlack{X,Y}: pixels past each screen edge a projected tile label may
// drift before it's culled (the debug-overlay sibling of offscreenPopupSlack).
const (
	debugLabelSlackX = float32(120)
	debugLabelSlackY = float32(80)
)

// debugLabelsBuf reuses the per-tile label slice across frames (renderer is single-threaded).
var debugLabelsBuf = make([]labelStack, 0, (2*debugLabelRange+1)*(2*debugLabelRange+1))

type labelStack struct {
	screen rl.Vector2
	lines  []string
	dist   float32
}

// DrawDebugOverlay paints in-world tile labels (floor/decor/prop + coords within debugLabelRange)
// and a top-left player readout (x,z / facing / step / map). No-op when DebugOverlay is off or the
// pause menu is open, so calling it unconditionally is free.
func DrawDebugOverlay(camera rl.Camera3D, g *core.GameState, assets Resources) {
	if !g.DebugOverlay || g.MenuOpen {
		return
	}
	m := g.Area

	facing, _ := core.FacingName(g.Player.Facing)
	header := []string{
		fmt.Sprintf("X=%d  Z=%d  %s", g.Player.TileX, g.Player.TileZ, facing),
		fmt.Sprintf("step %d  %s", g.StepCount, m.Name),
	}
	for i, line := range header {
		x, y := float32(14), float32(12+i*22)
		drawTextWithShadowStyle(assets.hudFont, line, x, y, FontBody, FontSpacingBody, debugHeadingColor, shadowHeavy, 1, 1)
	}

	// Pre-compute forward to drop tiles behind the camera.
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
			// Drop tiles behind the camera; GetWorldToScreen mis-projects them into the visible range, so dot-product is the reliable filter.
			if wdx*forward.X+wdz*forward.Z < -1.5 {
				continue
			}
			lines := tileLabelLines(m, x, z, x == pX && z == pZ)
			if len(lines) == 0 {
				continue
			}
			lines = append(lines, fmt.Sprintf("(%d,%d)", x, z))
			// Float the label ~1.6 units above the floor so it sits above the tile's prop.
			screen := rl.GetWorldToScreen(rl.NewVector3(cx, 1.6, cz), camera)
			if screen.X < -debugLabelSlackX || screen.X > screenW+debugLabelSlackX || screen.Y < -debugLabelSlackY || screen.Y > screenH+debugLabelSlackY {
				continue
			}
			labels = append(labels, labelStack{
				screen: screen,
				lines:  lines,
				dist:   wdx*wdx + wdz*wdz,
			})
		}
	}

	// Paint far labels first so near labels overdraw (no depth buffer for 2D text).
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

// tileLabelLines returns the readable layer names for the tile at (x, z); youHere prepends "YOU".
func tileLabelLines(m core.AreaDefinition, x, z int, youHere bool) []string {
	var lines []string
	if youHere {
		lines = append(lines, "YOU")
	}
	// Use bounds-safe accessors, not m.Floor[z][x]: ragged/short layer rows would panic a raw index.
	if c, ok := m.FloorCharAt(x, z); ok {
		if f := core.TileLabel(core.TileLayerFloor, c); f != "" {
			lines = append(lines, f)
		}
	}
	if c, ok := m.DecorCharAt(x, z); ok {
		if d := core.TileLabel(core.TileLayerDecor, c); d != "" {
			lines = append(lines, d)
		}
	}
	if c, ok := m.PropCharAt(x, z); ok {
		if p := core.TileLabel(core.TileLayerProps, c); p != "" {
			lines = append(lines, p)
		}
	}
	return lines
}

// drawDebugLabel paints one debug-label line: translucent pill behind, two-pass text on top.
func drawDebugLabel(font rl.Font, text string, x, y float32) {
	if text == "" {
		return
	}
	const size = FontSmall
	const spacing = float32(1)
	measure := rl.MeasureTextEx(font, text, size, spacing)
	rx := x - measure.X/2
	ry := y - measure.Y/2
	const pad = float32(4)
	rl.DrawRectangle(int32(rx-pad), int32(ry-pad/2), int32(measure.X+pad*2), int32(measure.Y+pad), shadowMid)
	drawTextWithShadowStyle(font, text, rx, ry, size, spacing, debugLabelColor, shadowHeavy, 1, 1)
}

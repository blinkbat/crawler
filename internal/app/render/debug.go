package render

import (
	"fmt"
	"sort"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// The debug overlay used to be gated behind a `debug` build tag so its
// tile-label sort/walk and per-frame label buffer wouldn't ship in release
// — but `g.DebugOverlay` is false by default, and every line below is
// inside an early-return guard against that flag, so the overhead in
// release-with-overlay-off is zero. Making the overlay always-available
// means the pause-menu toggle now does what the player expects in any
// build, without forcing a `go build -tags debug` rebuild.

// debugLabelRange is how many tiles around the player get labelled. 4 covers
// the immediate "what am I standing next to" question without flooding the
// screen — the labels overlap badly past ~25 tiles even sorted by depth.
const debugLabelRange = 4

// Debug-overlay text tints — named so the two near-identical greens
// (the coord heading vs the in-world tile labels) aren't inline literals.
var (
	debugHeadingColor = rl.NewColor(186, 240, 186, 245)
	debugLabelColor   = rl.NewColor(220, 240, 220, 245)
)

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
// youHere prepends a "YOU" marker for the tile the player is standing on. Walls
// are gone (a tile is always a walkable surface, possibly raised), so every
// tile shows its floor/decor/prop labels — no short-circuit.
func tileLabelLines(m core.AreaDefinition, x, z int, youHere bool) []string {
	var lines []string
	if youHere {
		lines = append(lines, "YOU")
	}
	// Read through the bounds-safe accessors (NOT direct m.Floor[z][x] indexing):
	// the caller only guarantees InBounds against Width/Height, but a struct-built
	// or mid-edit area can have ragged/short layer rows, which a raw index panics on.
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

// drawDebugLabel paints one line of a debug label stack — black-translucent
// pill behind, two-pass text (shadow + foreground) on top so the label
// reads cleanly over any 3D background.
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

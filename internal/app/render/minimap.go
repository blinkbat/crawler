package render

import (
	"fmt"
	"image/color"

	"crawler/internal/app/core"
	rl "github.com/gen2brain/raylib-go/raylib"
)

func drawMinimap(m core.AreaDefinition, g core.GameState, assets Resources) {
	const (
		cell      = int32(12)
		viewCells = int32(13)
		pad       = int32(20)
		header    = int32(34)
		footer    = int32(28) // time-of-day strip beneath the grid
	)
	p := g.Player
	half := int(viewCells / 2)
	startX := p.TileX - half
	startZ := p.TileZ - half
	gridSize := viewCells * cell
	panelW := gridSize + 16
	panelH := gridSize + 16 + header + footer

	drawCard(pad, pad, panelW, panelH, surfacePrimary, borderSoft, borderStrong)
	areaName := g.Area.Name
	drawHeading(assets.hudFont, "AREA", pad+14, pad+10, borderStrong)
	if areaName != "" {
		drawTextWithShadow(assets.hudFont, areaName, float32(pad+74), float32(pad+10), 14, textMuted)
	}

	gridX := pad + 8
	gridY := pad + 8 + header
	footerY := gridY + gridSize + 6
	drawMinimapTimeOfDay(assets.hudFont, g.StepCount, pad+14, footerY, panelW-28)

	for localZ := int32(0); localZ < viewCells; localZ++ {
		for localX := int32(0); localX < viewCells; localX++ {
			mapX := startX + int(localX)
			mapZ := startZ + int(localZ)
			col := rl.NewColor(8, 10, 14, 235)
			if mapX >= 0 && mapX < m.Width && mapZ >= 0 && mapZ < m.Height {
				col = minimapTileColor(m.Materials, m.TileAt(mapX, mapZ))
			}
			rl.DrawRectangle(gridX+localX*cell, gridY+localZ*cell, cell-1, cell-1, col)
		}
	}

	for _, pack := range g.Packs {
		if !core.PackAlive(pack) {
			continue
		}
		localX := pack.TileX - startX
		localZ := pack.TileZ - startZ
		if localX < 0 || localZ < 0 || localX >= int(viewCells) || localZ >= int(viewCells) {
			continue
		}
		x := gridX + int32(localX)*cell + cell/2
		z := gridY + int32(localZ)*cell + cell/2
		rl.DrawCircle(x, z, 4, rl.NewColor(220, 76, 70, 255))
		rl.DrawCircleLines(x, z, 5, rl.NewColor(255, 200, 200, 220))
	}

	drawMinimapArrow(
		rl.NewVector2(float32(gridX+gridSize/2), float32(gridY+gridSize/2)),
		p.Facing,
	)
}

// minimapPropColors keys every prop-layer char to its minimap swatch.
// Map-driven instead of a switch so a missing entry stands out at code
// review and the unknown-tile fallback path stays a single branch.
// Must enumerate the same set as core.IsPropChar's switch in
// internal/app/core/map.go — a future prop char goes in both lists.
// Colors are deliberately distinct from the editor's tileColorByChar:
// the minimap tone palette is tuned for the small-rendered case, not
// for matching the editor swatch exactly.
// init asserts every prop-tile char registered in core.PropTileChars
// has a minimap color, AND that every blocking-floor char (currently
// FloorDeepWater; future lava etc.) has a dedicated case in
// minimapTileColor's switch. Without these guards, a new blocker tile
// silently fell back to the "unknown tile" tone on the minimap —
// catching it at startup is far cheaper than noticing in playtest.
func init() {
	for _, c := range core.PropTileChars() {
		if _, ok := minimapPropColors[c]; !ok {
			panic("render/minimap: missing color for prop tile " + string(c))
		}
	}
	for _, c := range core.BlockingFloorChars() {
		col := minimapTileColor(core.MaterialField, c)
		fallback := minimapTileColor(core.MaterialField, core.TileOpen)
		if col == fallback {
			panic("render/minimap: blocking floor tile '" + string(c) + "' falls through to the open-tile color — add an explicit case in minimapTileColor")
		}
	}
}

var minimapPropColors = map[byte]rl.Color{
	core.TileTree:              rl.NewColor(42, 132, 56, 240),
	core.TileTreeXL:            rl.NewColor(28, 102, 44, 240),
	core.TileRockLarge:         rl.NewColor(120, 116, 108, 240),
	core.TileBushLarge:         rl.NewColor(110, 168, 92, 240),
	core.TileCrate:             rl.NewColor(168, 122, 72, 240),
	core.TileBarrel:            rl.NewColor(148, 100, 60, 240),
	core.TileUrn:               rl.NewColor(186, 112, 72, 240),
	core.TileStalagmite:        rl.NewColor(196, 188, 174, 240),
	core.TilePillar:            rl.NewColor(214, 206, 188, 240),
	core.TileBrokenPillar:      rl.NewColor(180, 172, 156, 240),
	core.TileStatue:            rl.NewColor(228, 220, 204, 240),
	core.TileObelisk:           rl.NewColor(86, 90, 104, 240),
	core.TileFountain:          rl.NewColor(96, 158, 208, 240),
	core.TileRockCairn:         rl.NewColor(150, 138, 116, 240),
	core.TileRockFormation:     rl.NewColor(118, 102, 86, 240),
	core.TileRockFormationTail: rl.NewColor(118, 102, 86, 240),
}

func minimapTileColor(material core.MaterialSet, tile byte) color.RGBA {
	if tile == core.TileRock {
		if material == core.MaterialDungeon {
			return rl.NewColor(132, 132, 126, 235)
		}
		return rl.NewColor(112, 112, 106, 235)
	}
	if tile == core.FloorDeepWater {
		return rl.NewColor(28, 60, 104, 235)
	}
	if col, ok := minimapPropColors[tile]; ok {
		return col
	}
	if material == core.MaterialDungeon {
		return rl.NewColor(82, 84, 88, 235)
	}
	return rl.NewColor(60, 121, 54, 235)
}

// drawMinimapTimeOfDay paints the day/night cycle indicator under the
// minimap grid: phase name on the left, raw step counter on the right,
// and a thin progress bar showing how far through the current phase the
// player is. The cycle is 150 steps total (6 × 25), so the bar wraps
// every full day.
func drawMinimapTimeOfDay(font rl.Font, stepCount int, x, y, width int32) {
	phase, progress := core.PhaseAtStep(stepCount)
	name := core.PhaseName(phase)
	// Line 1: "DAWN  step 12 / 150" (left-aligned phase, right-aligned counter).
	drawTextWithShadow(font, name, float32(x), float32(y), 14, textPrimary)
	counter := fmt.Sprintf("step %d / %d", stepCount%core.StepsPerCycle, core.StepsPerCycle)
	measure := rl.MeasureTextEx(font, counter, 12, 1)
	drawTextWithShadow(font, counter, float32(x)+float32(width)-measure.X, float32(y)+1, 12, textHint)
	// Line 2: thin track, with the phase highlighted as a 1/N segment that
	// fills as the player walks through it.
	trackY := y + 18
	trackH := int32(4)
	trackW := width
	trackCol := rl.NewColor(8, 12, 22, 200)
	rl.DrawRectangle(x, trackY, trackW, trackH, trackCol)
	segW := trackW / int32(core.TimeOfDayCount)
	// Past phases: solid color. Current phase: filled by progress.
	for i := 0; i < int(phase); i++ {
		rl.DrawRectangle(x+int32(i)*segW, trackY, segW-1, trackH, phaseColors[i])
	}
	curW := int32(float32(segW) * progress)
	if curW > 0 {
		rl.DrawRectangle(x+int32(phase)*segW, trackY, curW, trackH, phaseColors[phase])
	}
}

// phaseColors mirrors the rough sky tint of each lighting phase so the HUD
// strip itself reads as a tiny day at a glance. Indexed by TimeOfDay; size
// is asserted at compile time against core.TimeOfDayCount via _phaseColorsLen.
var phaseColors = [core.TimeOfDayCount]rl.Color{
	core.Dawn:      rl.NewColor(232, 168, 152, 255), // dawn — rose
	core.Morning:   rl.NewColor(220, 224, 200, 255), // morning — pale gold
	core.Afternoon: rl.NewColor(190, 220, 244, 255), // afternoon — sky
	core.Dusk:      rl.NewColor(232, 152, 96, 255),  // dusk — orange
	core.Evening:   rl.NewColor(96, 110, 180, 255),  // evening — indigo
	core.Midnight:  rl.NewColor(40, 56, 110, 255),   // midnight — deep blue
}

func drawMinimapArrow(center rl.Vector2, facing int) {
	const arrowSize = float32(7)
	var tip, left, right rl.Vector2
	switch core.NormalizeFacing(facing) {
	case core.North:
		tip = rl.NewVector2(center.X, center.Y-arrowSize)
		left = rl.NewVector2(center.X-arrowSize, center.Y+arrowSize)
		right = rl.NewVector2(center.X+arrowSize, center.Y+arrowSize)
	case core.East:
		tip = rl.NewVector2(center.X+arrowSize, center.Y)
		left = rl.NewVector2(center.X-arrowSize, center.Y-arrowSize)
		right = rl.NewVector2(center.X-arrowSize, center.Y+arrowSize)
	case core.South:
		tip = rl.NewVector2(center.X, center.Y+arrowSize)
		left = rl.NewVector2(center.X+arrowSize, center.Y-arrowSize)
		right = rl.NewVector2(center.X-arrowSize, center.Y-arrowSize)
	case core.West:
		tip = rl.NewVector2(center.X-arrowSize, center.Y)
		left = rl.NewVector2(center.X+arrowSize, center.Y+arrowSize)
		right = rl.NewVector2(center.X+arrowSize, center.Y-arrowSize)
	}
	drawTriangleCCW(tip, left, right, rl.NewColor(132, 240, 148, 255))
}

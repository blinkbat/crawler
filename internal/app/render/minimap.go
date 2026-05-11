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

func minimapTileColor(material core.MaterialSet, tile byte) color.RGBA {
	switch tile {
	case core.TileRock:
		if material == core.MaterialDungeon {
			return rl.NewColor(132, 132, 126, 235)
		}
		return rl.NewColor(112, 112, 106, 235)
	case core.TileTree:
		return rl.NewColor(42, 132, 56, 240)
	case core.TileTreeXL:
		// Slightly darker / saturated than TileTree so the XL footprints stand
		// out as the canopy "tent poles" the player navigates around.
		return rl.NewColor(28, 102, 44, 240)
	case core.TileRockLarge:
		// Boulder tone matches the field's wall-rock palette so it reads as
		// "this is hard cover, not foliage."
		return rl.NewColor(120, 116, 108, 240)
	case core.TileBushLarge:
		// Lighter, yellower green than the trees so bushes don't get confused
		// with canopy on the map.
		return rl.NewColor(110, 168, 92, 240)
	default:
		if material == core.MaterialDungeon {
			return rl.NewColor(82, 84, 88, 235)
		}
		return rl.NewColor(60, 121, 54, 235)
	}
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
	// Line 2: thin track, with the phase highlighted as a 1/6 segment that
	// fills as the player walks through it.
	trackY := y + 18
	trackH := int32(4)
	trackW := width
	trackCol := rl.NewColor(8, 12, 22, 200)
	rl.DrawRectangle(x, trackY, trackW, trackH, trackCol)
	segW := trackW / int32(len(phaseColors))
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
// strip itself reads as a tiny day at a glance.
var phaseColors = [6]rl.Color{
	rl.NewColor(232, 168, 152, 255), // dawn — rose
	rl.NewColor(220, 224, 200, 255), // morning — pale gold
	rl.NewColor(190, 220, 244, 255), // afternoon — sky
	rl.NewColor(232, 152, 96, 255),  // dusk — orange
	rl.NewColor(96, 110, 180, 255),  // evening — indigo
	rl.NewColor(40, 56, 110, 255),   // midnight — deep blue
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
	rl.DrawTriangle(tip, left, right, rl.NewColor(132, 240, 148, 255))
}

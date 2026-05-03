package render

import (
	"image/color"

	"crawler/internal/app/core"
	rl "github.com/gen2brain/raylib-go/raylib"
)

func drawMinimap(m core.GameMap, g core.GameState, assets Resources) {
	const (
		cell      = int32(12)
		viewCells = int32(13)
		pad       = int32(20)
		header    = int32(34)
	)
	p := g.Player
	half := int(viewCells / 2)
	startX := p.TileX - half
	startZ := p.TileZ - half
	gridSize := viewCells * cell
	panelW := gridSize + 16
	panelH := gridSize + 16 + header

	drawCard(pad, pad, panelW, panelH, surfacePrimary, borderSoft, borderStrong)
	areaName := core.AreaByID(g.AreaID).Name
	drawHeading(assets.hudFont, "AREA", pad+14, pad+10, borderStrong)
	if areaName != "" {
		drawTextWithShadow(assets.hudFont, areaName, float32(pad+74), float32(pad+10), 14, textMuted)
	}

	gridX := pad + 8
	gridY := pad + 8 + header

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

	for _, enemy := range g.Enemies {
		if !enemy.Alive {
			continue
		}
		localX := enemy.TileX - startX
		localZ := enemy.TileZ - startZ
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
	default:
		if material == core.MaterialDungeon {
			return rl.NewColor(82, 84, 88, 235)
		}
		return rl.NewColor(60, 121, 54, 235)
	}
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

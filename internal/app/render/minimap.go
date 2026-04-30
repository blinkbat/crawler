package render

import (
	"crawler/internal/app/core"
	rl "github.com/gen2brain/raylib-go/raylib"
)

func drawMinimap(m core.GameMap, g core.GameState) {
	const (
		cell      = int32(12)
		viewCells = int32(13)
		pad       = int32(12)
	)
	p := g.Player
	half := int(viewCells / 2)
	startX := p.TileX - half
	startZ := p.TileZ - half
	panelSize := viewCells * cell

	drawRoundedRect(pad-3, pad-3, panelSize+6, panelSize+6, 0.06, rl.NewColor(10, 10, 12, 165))
	for localZ := int32(0); localZ < viewCells; localZ++ {
		for localX := int32(0); localX < viewCells; localX++ {
			mapX := startX + int(localX)
			mapZ := startZ + int(localZ)
			col := rl.NewColor(18, 20, 24, 228)
			if mapX >= 0 && mapX < m.Width && mapZ >= 0 && mapZ < m.Height {
				col = rl.NewColor(38, 42, 48, 228)
			}
			if mapX >= 0 && mapX < m.Width && mapZ >= 0 && mapZ < m.Height && m.WallAt(mapX, mapZ) {
				col = rl.NewColor(178, 92, 96, 228)
			}
			rl.DrawRectangle(pad+localX*cell, pad+localZ*cell, cell-1, cell-1, col)
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
		x := pad + int32(localX)*cell + cell/2
		z := pad + int32(localZ)*cell + cell/2
		rl.DrawCircle(x, z, 4, rl.NewColor(210, 66, 60, 255))
	}

	drawMinimapArrow(
		rl.NewVector2(float32(pad+viewCells*cell/2), float32(pad+viewCells*cell/2)),
		p.Facing,
	)
}

func drawMinimapArrow(center rl.Vector2, facing int) {
	const arrowSize = float32(6.6)
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
	rl.DrawTriangle(tip, left, right, rl.NewColor(118, 235, 136, 255))
}

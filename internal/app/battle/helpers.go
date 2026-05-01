package battle

import (
	"crawler/internal/app/core"
)

func nearbyBattleGroup(m core.GameMap, enemies []core.Enemy, trigger int) []int {
	if trigger < 0 || trigger >= len(enemies) || !enemies[trigger].Alive {
		return nil
	}
	type candidate struct {
		index int
		dist  int
	}
	triggerEnemy := enemies[trigger]
	candidates := []candidate{{index: trigger, dist: -1}}
	for i, enemy := range enemies {
		if i == trigger || !enemy.Alive {
			continue
		}
		dist := core.AbsInt(enemy.TileX-triggerEnemy.TileX) + core.AbsInt(enemy.TileZ-triggerEnemy.TileZ)
		if dist <= 2 && battlePathClear(m, triggerEnemy.TileX, triggerEnemy.TileZ, enemy.TileX, enemy.TileZ) {
			candidates = append(candidates, candidate{index: i, dist: dist})
		}
	}
	for i := 1; i < len(candidates); i++ {
		for j := i; j > 0 && candidates[j].dist < candidates[j-1].dist; j-- {
			candidates[j], candidates[j-1] = candidates[j-1], candidates[j]
		}
	}
	limit := len(candidates)
	if limit > 3 {
		limit = 3
	}
	group := make([]int, 0, limit)
	for i := 0; i < limit; i++ {
		group = append(group, candidates[i].index)
	}
	return group
}

func battlePathClear(m core.GameMap, fromX, fromZ, toX, toZ int) bool {
	dx := toX - fromX
	dz := toZ - fromZ
	adx := core.AbsInt(dx)
	adz := core.AbsInt(dz)
	if adx+adz > 2 {
		return false
	}
	if adx == 0 || adz == 0 {
		return straightBattlePathClear(m, fromX, fromZ, toX, toZ)
	}
	return !m.BlockedAt(toX, fromZ) || !m.BlockedAt(fromX, toZ)
}

func straightBattlePathClear(m core.GameMap, fromX, fromZ, toX, toZ int) bool {
	stepX := 0
	if toX > fromX {
		stepX = 1
	} else if toX < fromX {
		stepX = -1
	}
	stepZ := 0
	if toZ > fromZ {
		stepZ = 1
	} else if toZ < fromZ {
		stepZ = -1
	}
	for x, z := fromX+stepX, fromZ+stepZ; x != toX || z != toZ; x, z = x+stepX, z+stepZ {
		if m.BlockedAt(x, z) {
			return false
		}
	}
	return true
}

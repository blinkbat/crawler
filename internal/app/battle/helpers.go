package battle

import (
	"crawler/internal/app/core"
)

func nearbyBattleGroup(enemies []core.Enemy, trigger int) []int {
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
		if dist <= 2 {
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

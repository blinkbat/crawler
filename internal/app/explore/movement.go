package explore

import (
	"crawler/internal/app/battle"
	"crawler/internal/app/core"
	rl "github.com/gen2brain/raylib-go/raylib"
	"math"
)

func Update(g *core.GameState, m core.GameMap) {
	dt := rl.GetFrameTime()
	if g.Battle.Phase != core.BattleNone {
		battle.Update(g, dt)
		return
	}

	if rl.IsKeyPressed(rl.KeyEscape) {
		g.MenuOpen = !g.MenuOpen
		g.Player.LookYaw = 0
		g.Player.LookPitch = 0
		return
	}
	if g.MenuOpen {
		updateMenu(g, m)
		return
	}

	updateFreeLook(&g.Player)
	if g.Player.Anim.Kind == core.AnimNone && StartAdjacent(g) {
		return
	}
	updatePlayer(g, m)
	if g.Battle.Phase == core.BattleNone && g.Player.Anim.Kind == core.AnimNone {
		StartAdjacent(g)
	}
}

func updateMenu(g *core.GameState, m core.GameMap) {
	if rl.IsKeyPressed(rl.KeyUp) || rl.IsKeyPressed(rl.KeyW) {
		g.MenuIndex = (g.MenuIndex + 1) % 2
	}
	if rl.IsKeyPressed(rl.KeyDown) || rl.IsKeyPressed(rl.KeyS) {
		g.MenuIndex = (g.MenuIndex + 1) % 2
	}
	if rl.IsKeyPressed(rl.KeyR) {
		restartGame(g, m)
		return
	}
	if rl.IsKeyPressed(rl.KeyQ) {
		g.Quit = true
		return
	}
	if rl.IsKeyPressed(rl.KeyEnter) || rl.IsKeyPressed(rl.KeySpace) {
		switch g.MenuIndex {
		case 0:
			restartGame(g, m)
		case 1:
			g.Quit = true
		}
	}
}

func restartGame(g *core.GameState, m core.GameMap) {
	*g = core.NewGameState(m)
}

func updateFreeLook(p *core.Player) {
	if rl.IsMouseButtonDown(rl.MouseRightButton) {
		mouse := rl.GetMouseDelta()
		p.LookYaw = core.Clamp(p.LookYaw+mouse.X*core.MouseSense, -core.MaxLookYaw, core.MaxLookYaw)
		p.LookPitch = core.Clamp(p.LookPitch-mouse.Y*core.MouseSense, -core.MaxLookPitch, core.MaxLookPitch)
		return
	}
	p.LookYaw = 0
	p.LookPitch = 0
}

func updatePlayer(g *core.GameState, m core.GameMap) {
	dt := rl.GetFrameTime()
	p := &g.Player

	if p.Anim.Kind != core.AnimNone {
		updateAnimation(p, dt)
		return
	}

	switch {
	case rl.IsKeyPressed(rl.KeyLeft) || rl.IsKeyPressed(rl.KeyQ):
		startTurn(p, -1)
	case rl.IsKeyPressed(rl.KeyRight) || rl.IsKeyPressed(rl.KeyE):
		startTurn(p, 1)
	case rl.IsKeyPressed(rl.KeyW) || rl.IsKeyPressed(rl.KeyUp):
		startStep(p, g, m, 0, 1)
	case rl.IsKeyPressed(rl.KeyS) || rl.IsKeyPressed(rl.KeyDown):
		startStep(p, g, m, 0, -1)
	case rl.IsKeyPressed(rl.KeyA):
		startStep(p, g, m, -1, 0)
	case rl.IsKeyPressed(rl.KeyD):
		startStep(p, g, m, 1, 0)
	}
}

func startStep(p *core.Player, g *core.GameState, m core.GameMap, strafe, forward int) {
	dx, dz := core.FacingVector(p.Facing)
	rx, rz := core.FacingVector(core.NormalizeFacing(p.Facing + 1))
	targetX := p.TileX + dx*forward + rx*strafe
	targetZ := p.TileZ + dz*forward + rz*strafe
	if m.WallAt(targetX, targetZ) {
		return
	}
	if enemyIndex := liveEnemyAt(g.Enemies, targetX, targetZ); enemyIndex >= 0 {
		battle.Start(g, enemyIndex)
		return
	}

	p.TileX = targetX
	p.TileZ = targetZ
	p.Anim = core.Animation{
		Kind:     core.AnimStep,
		Duration: core.StepDuration,
		FromX:    p.X,
		FromZ:    p.Z,
		ToX:      core.TileCenter(targetX),
		ToZ:      core.TileCenter(targetZ),
	}
}

func startTurn(p *core.Player, delta int) {
	nextFacing := core.NormalizeFacing(p.Facing + delta)
	p.Facing = nextFacing
	p.Anim = core.Animation{
		Kind:     core.AnimTurn,
		Duration: core.TurnDuration,
		FromYaw:  p.Yaw,
		ToYaw:    p.Yaw + float32(delta)*math.Pi/2,
	}
}

func updateAnimation(p *core.Player, dt float32) {
	p.Anim.Elapsed += dt
	t := p.Anim.Elapsed / p.Anim.Duration
	if t >= 1 {
		t = 1
	}
	eased := core.Smoothstep(t)

	switch p.Anim.Kind {
	case core.AnimStep:
		p.X = core.Lerp(p.Anim.FromX, p.Anim.ToX, eased)
		p.Z = core.Lerp(p.Anim.FromZ, p.Anim.ToZ, eased)
	case core.AnimTurn:
		p.Yaw = core.LerpAngle(p.Anim.FromYaw, p.Anim.ToYaw, eased)
	}

	if p.Anim.Elapsed < p.Anim.Duration {
		return
	}
	if p.Anim.Kind == core.AnimStep {
		p.X = core.TileCenter(p.TileX)
		p.Z = core.TileCenter(p.TileZ)
	}
	p.Yaw = core.FacingYaw(p.Facing)
	p.Anim = core.Animation{}
}

func liveEnemyAt(enemies []core.Enemy, tileX, tileZ int) int {
	for i, e := range enemies {
		if e.Alive && e.TileX == tileX && e.TileZ == tileZ {
			return i
		}
	}
	return -1
}

func adjacentEnemyIndex(enemies []core.Enemy, tileX, tileZ int) int {
	for i, e := range enemies {
		if !e.Alive {
			continue
		}
		if core.AbsInt(e.TileX-tileX)+core.AbsInt(e.TileZ-tileZ) == 1 {
			return i
		}
	}
	return -1
}

func StartAdjacent(g *core.GameState) bool {
	enemyIndex := adjacentEnemyIndex(g.Enemies, g.Player.TileX, g.Player.TileZ)
	if enemyIndex < 0 {
		return false
	}
	battle.Start(g, enemyIndex)
	return true
}

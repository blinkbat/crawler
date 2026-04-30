package explore

import (
	"crawler/internal/app/battle"
	"crawler/internal/app/core"
	"crawler/internal/app/input"
	rl "github.com/gen2brain/raylib-go/raylib"
	"math"
)

func Update(g *core.GameState) {
	dt := rl.GetFrameTime()
	if g.Battle.Phase != core.BattleNone {
		battle.Update(g, dt)
		return
	}

	if input.PausePressed() {
		g.MenuOpen = !g.MenuOpen
		g.Player.LookYaw = 0
		g.Player.LookPitch = 0
		return
	}
	if g.MenuOpen {
		updateMenu(g)
		return
	}

	updateFreeLook(&g.Player)
	if g.Player.Anim.Kind == core.AnimNone && StartAdjacent(g) {
		return
	}
	updatePlayer(g)
	if g.Battle.Phase == core.BattleNone && g.Player.Anim.Kind == core.AnimNone {
		StartAdjacent(g)
	}
}

func updateMenu(g *core.GameState) {
	if input.BackPressed() {
		g.MenuOpen = false
		return
	}
	if input.UpPressed() {
		g.MenuIndex = core.WrapIndex(g.MenuIndex-1, 2)
	}
	if input.DownPressed() {
		g.MenuIndex = core.WrapIndex(g.MenuIndex+1, 2)
	}
	if input.RestartPressed() {
		restartGame(g)
		return
	}
	if input.QuitPressed() {
		g.Quit = true
		return
	}
	if input.ConfirmPressed() {
		switch g.MenuIndex {
		case 0:
			restartGame(g)
		case 1:
			g.Quit = true
		}
	}
}

func restartGame(g *core.GameState) {
	core.ResetGameState(g)
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

func updatePlayer(g *core.GameState) {
	dt := rl.GetFrameTime()
	p := &g.Player

	if p.Anim.Kind != core.AnimNone {
		updateAnimation(p, dt)
		return
	}

	switch {
	case input.TurnLeftPressed():
		startTurn(p, -1)
	case input.TurnRightPressed():
		startTurn(p, 1)
	case input.StepForwardPressed():
		startStep(p, g, 0, 1)
	case input.StepBackPressed():
		startStep(p, g, 0, -1)
	case input.StrafeLeftPressed():
		startStep(p, g, -1, 0)
	case input.StrafeRightPressed():
		startStep(p, g, 1, 0)
	}
}

func startStep(p *core.Player, g *core.GameState, strafe, forward int) {
	dx, dz := core.FacingVector(p.Facing)
	rx, rz := core.FacingVector(core.NormalizeFacing(p.Facing + 1))
	targetX := p.TileX + dx*forward + rx*strafe
	targetZ := p.TileZ + dz*forward + rz*strafe
	if g.Map.WallAt(targetX, targetZ) {
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
	duration := core.TurnDuration * float32(core.AbsInt(delta))
	if duration <= 0 {
		duration = core.TurnDuration
	}
	p.Anim = core.Animation{
		Kind:     core.AnimTurn,
		Duration: duration,
		FromYaw:  p.Yaw,
		ToYaw:    p.Yaw + float32(delta)*math.Pi/2,
	}
}

func startTurnToTile(p *core.Player, tileX, tileZ int) bool {
	targetFacing, ok := facingForTile(p, tileX, tileZ)
	if !ok {
		return false
	}
	diff := core.NormalizeFacing(targetFacing - p.Facing)
	switch diff {
	case 0:
		return false
	case 1:
		startTurn(p, 1)
	case 2:
		startTurn(p, 2)
	case 3:
		startTurn(p, -1)
	}
	return true
}

func facingForTile(p *core.Player, tileX, tileZ int) (int, bool) {
	dx := tileX - p.TileX
	dz := tileZ - p.TileZ
	switch {
	case dx > 0:
		return core.East, true
	case dx < 0:
		return core.West, true
	case dz > 0:
		return core.South, true
	case dz < 0:
		return core.North, true
	}
	return p.Facing, false
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
	if startTurnToTile(&g.Player, g.Enemies[enemyIndex].TileX, g.Enemies[enemyIndex].TileZ) {
		return true
	}
	battle.Start(g, enemyIndex)
	return true
}

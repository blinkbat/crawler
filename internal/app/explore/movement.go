package explore

import (
	"crawler/internal/app/battle"
	"crawler/internal/app/core"
	"crawler/internal/app/input"
	"crawler/internal/app/render"
	rl "github.com/gen2brain/raylib-go/raylib"
	"math"
)

func Update(g *core.GameState) {
	dt := rl.GetFrameTime()
	// Cap dt so a frame stall (window drag, debugger pause, slow load) can't
	// fast-forward animations or overshoot tile-step targets in one tick.
	// Matches the cap in battle.Update so the whole game has consistent
	// minimum-effective tick rate.
	if dt > 1.0/15.0 {
		dt = 1.0 / 15.0
	}

	// The level-up modal sits above every other overlay — the player
	// MUST allocate the points they just earned before drifting back
	// into exploration (or a chest, or the pause menu). No Esc-out:
	// PendingLevelUps reads as a debt the player owes the system.
	if g.LevelUpOpen {
		updateLevelUpModal(g)
		return
	}

	// Read-only Party Stats overlay opens from the pause menu and
	// closes with Esc/Back. No state mutation here — the screen is
	// purely informational; we just gate input so movement / pause
	// don't leak through.
	if g.StatsScreenOpen {
		if input.BackPressed() {
			g.StatsScreenOpen = false
		}
		return
	}

	// The chest modal sits above movement but below the pause menu — Esc
	// from inside a chest closes the chest, not the game. Handled first so
	// the player can't drift around (or open the pause menu) while a chest
	// dialog is showing on screen.
	if g.ChestOpen >= 0 {
		updateChestModal(g)
		return
	}

	// The pause menu sits above the simulation: when it's open we route input
	// through the menu instead of advancing battle / explore. Pause-key edges
	// toggle the menu from either state, but during battle the toggle is
	// gated on a non-timing phase so the player can't pause through a timing
	// bar (which would skip the input window).
	if g.MenuOpen {
		updateMenu(g)
		return
	}
	pause := input.PausePressed(g.Battle.Phase != core.BattleNone)
	if pause && pauseAllowed(g) {
		g.MenuOpen = true
		g.Player.LookYaw = 0
		g.Player.LookPitch = 0
		return
	}
	if g.Battle.Phase != core.BattleNone {
		battle.Update(g, dt)
		return
	}

	updateFreeLook(&g.Player, dt)
	if g.Player.Anim.Kind == core.AnimNone && startAdjacent(g) {
		return
	}
	// Confirm key opens an adjacent chest. Checked before movement so a
	// "step forward + Enter" muscle-memory press doesn't double as a step
	// in the chest's direction.
	if g.Player.Anim.Kind == core.AnimNone && tryOpenAdjacentChest(g) {
		return
	}
	updatePlayer(g)
	if g.Battle.Phase == core.BattleNone && g.Player.Anim.Kind == core.AnimNone {
		startAdjacent(g)
	}
}

// tryOpenAdjacentChest is the Confirm-key interaction for chests. If
// the player is one tile away from a non-looted chest, open its modal
// and return true so the rest of the explore tick (free-look, movement)
// skips this frame.
func tryOpenAdjacentChest(g *core.GameState) bool {
	if !input.ConfirmPressed() {
		return false
	}
	idx := core.AdjacentInteractableChestIndex(g.Chests, g.Player.TileX, g.Player.TileZ)
	if idx < 0 {
		return false
	}
	g.ChestOpen = idx
	g.ChestMenuIndex = 0
	return true
}

// pauseAllowed reports whether the global pause menu can be opened right now.
// In the field it's always allowed; during battle we forbid pausing while a
// timing bar is active so the player can't sidestep the input window.
func pauseAllowed(g *core.GameState) bool {
	switch g.Battle.Phase {
	case core.BattleAttackTiming, core.BattleEnemyTiming:
		return false
	}
	return true
}

func updateMenu(g *core.GameState) {
	if input.BackPressed() {
		g.MenuOpen = false
		return
	}
	if input.UpPressed() {
		g.MenuIndex = core.WrapIndex(g.MenuIndex-1, core.PauseMenuCount)
	}
	if input.DownPressed() {
		g.MenuIndex = core.WrapIndex(g.MenuIndex+1, core.PauseMenuCount)
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
		switch core.PauseMenuItem(g.MenuIndex) {
		case core.PauseMenuRestart:
			restartGame(g)
		case core.PauseMenuStats:
			// Drop the pause menu and raise the read-only stats overlay.
			// Esc inside the stats screen closes it (handled at the
			// top of explore.Update so the overlay shadows pause input
			// the same way a chest does).
			g.MenuOpen = false
			g.StatsScreenOpen = true
		case core.PauseMenuDebug:
			g.DebugOverlay = !g.DebugOverlay
		case core.PauseMenuDisplay:
			render.ToggleDisplayMode()
		case core.PauseMenuJukebox:
			render.PlayJukebox()
		case core.PauseMenuQuit:
			g.Quit = true
		}
	}
}

func restartGame(g *core.GameState) {
	core.ResetGameState(g)
}

func updateFreeLook(p *core.Player, dt float32) {
	if rl.IsMouseButtonDown(rl.MouseRightButton) {
		mouse := rl.GetMouseDelta()
		p.LookYaw = core.Clamp(p.LookYaw+mouse.X*core.MouseSense, -core.MaxLookYaw, core.MaxLookYaw)
		p.LookPitch = core.Clamp(p.LookPitch-mouse.Y*core.MouseSense, -core.MaxLookPitch, core.MaxLookPitch)
		return
	}
	p.LookYaw = core.Approach(p.LookYaw, 0, core.FreeLookReturnSpeed*dt)
	p.LookPitch = core.Approach(p.LookPitch, 0, core.FreeLookReturnSpeed*dt)
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
	case input.UpPressed():
		startStep(p, g, 0, 1)
	case input.DownPressed():
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
	if g.Area.BlockedAt(targetX, targetZ) {
		return
	}
	// Chests block the tile they sit on — even after being looted — so
	// the open lid keeps its position and the player can't walk through
	// the model. Handled here rather than in core.BlockedAt because the
	// chest list is runtime state, not part of AreaDefinition.
	if core.ChestIndexAt(g.Chests, targetX, targetZ) >= 0 {
		return
	}

	p.TileX = targetX
	p.TileZ = targetZ
	g.StepCount++
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
		p.Yaw = core.Lerp(p.Anim.FromYaw, p.Anim.ToYaw, eased)
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

func adjacentPackIndex(packs []core.Pack, tileX, tileZ int) int {
	for i, p := range packs {
		if !core.PackAlive(p) {
			continue
		}
		if core.AbsInt(p.TileX-tileX)+core.AbsInt(p.TileZ-tileZ) == 1 {
			return i
		}
	}
	return -1
}

func startAdjacent(g *core.GameState) bool {
	packIndex := adjacentPackIndex(g.Packs, g.Player.TileX, g.Player.TileZ)
	if packIndex < 0 {
		return false
	}
	pack := g.Packs[packIndex]
	if startTurnToTile(&g.Player, pack.TileX, pack.TileZ) {
		return true
	}
	battle.Start(g, packIndex)
	return true
}

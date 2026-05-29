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
	// Cap dt so a frame stall (window drag, debugger pause, slow load) can't
	// fast-forward animations or overshoot tile-step targets in one tick.
	// explore.Update is the single owner of this clamp — battle.Update
	// trusts the dt it's handed here is already clamped (see core.MaxFrameStep
	// / ClampFrameTime), so the whole game shares one minimum tick rate.
	dt := core.ClampFrameTime(rl.GetFrameTime())

	// Modal priority — single source of truth lives in core.ActiveModal
	// so any future scene that needs the same "what overlay is on top?"
	// answer reads from one helper instead of replicating the if-chain.
	// Each modal has its own updater here; the order of update calls
	// is determined entirely by ActiveModal's enum ladder.
	//
	// Notes per modal:
	//   - LevelUp: no Esc-out (PendingLevelUps reads as a debt the
	//     player owes the system).
	//   - Panels: out-of-battle only; the open path further down
	//     refuses to open during combat to keep the timing-bar window
	//     honest.
	//   - Chest: Esc inside a chest closes the chest, not the game.
	//   - PauseMenu: routes input through the menu; pause-key edges
	//     toggle the menu from either state.
	switch core.ActiveModal(g) {
	case core.ModalLevelUp:
		updateLevelUpModal(g)
		return
	case core.ModalPanels:
		updatePanels(g)
		return
	case core.ModalChest:
		updateChestModal(g)
		return
	case core.ModalDoorPrompt:
		updateDoorPrompt(g)
		return
	case core.ModalDebugMenu:
		updateDebugMenu(g)
		return
	case core.ModalPauseMenu:
		updateMenu(g)
		return
	}

	// Panels-open shortcut. Sits between the modal dispatch and the
	// pause-key check so the player can press I / middle-button to
	// jump into the overlay without first toggling pause off.
	if !g.Battle.Active() {
		if input.PanelsTogglePressed() {
			openPanels(g)
			return
		}
		if tab, ok := input.PanelTabShortcutPressed(); ok {
			openPanels(g)
			g.PanelsTab = tab
			return
		}
	}
	pause := input.PausePressed(g.Battle.Active())
	if pause && pauseAllowed(g) {
		g.MenuOpen = true
		g.Player.LookYaw = 0
		g.Player.LookPitch = 0
		return
	}
	if g.Battle.Active() {
		battle.Update(g, dt)
		return
	}

	updateFreeLook(&g.Player, dt)
	// Tick pack animations every explore frame so an in-flight step
	// (armed when the AI applied a move) eases tile-to-tile alongside
	// the player's own animation. Independent of whether the player
	// is mid-step, so a wandering pack still moves while the player
	// stands still.
	core.TickPackAnimations(g, dt)
	// Confirm key opens an adjacent chest. Checked before movement so a
	// "step forward + Enter" muscle-memory press doesn't double as a step
	// in the chest's direction.
	if g.Player.Anim.Kind == core.AnimNone && tryOpenAdjacentChest(g) {
		return
	}
	updatePlayer(g, dt)
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
			// Drop the pause menu and open the panels overlay on the
			// Stats tab — that surface is the richer multi-tab
			// dashboard (Stats / Equipment / Items / Skills / Map),
			// of which the legacy compact view was a subset.
			g.MenuOpen = false
			openPanels(g)
			g.PanelsTab = core.PanelTabStats
		case core.PauseMenuDebug:
			// First confirm enables debug mode; once on, the row opens the
			// debug submenu (which holds the enemy / time / easy-quit
			// toggles and the off-switch). "Tools only when debug is on."
			if g.DebugOverlay {
				openDebugMenu(g)
			} else {
				g.DebugOverlay = true
			}
		case core.PauseMenuDisplay:
			render.ToggleDisplayMode()
		case core.PauseMenuJukebox:
			render.PlayJukebox()
		case core.PauseMenuQuit:
			g.Quit = true
		}
	}
}

// openDebugMenu swaps the pause menu for the debug submenu. Reachable
// only while debug mode (DebugOverlay) is on — the pause-menu Debug row
// gates the call.
func openDebugMenu(g *core.GameState) {
	g.MenuOpen = false
	g.DebugMenuOpen = true
	g.DebugMenuIndex = 0
}

// updateDebugMenu drives the debug submenu: enemy on/off, advance the
// time-of-day phase, easy-battle-quit toggle, and the debug-mode
// off-switch. Back closes straight to explore (not back to the pause
// menu) — the debug menu is a leaf, not a pause sub-page.
func updateDebugMenu(g *core.GameState) {
	if input.BackPressed() {
		g.DebugMenuOpen = false
		return
	}
	g.DebugMenuIndex = input.CursorUpDown(g.DebugMenuIndex, core.DebugMenuCount)
	if !input.ConfirmPressed() {
		return
	}
	switch core.DebugMenuItem(g.DebugMenuIndex) {
	case core.DebugMenuEnemies:
		g.EnemiesDisabled = !g.EnemiesDisabled
	case core.DebugMenuAdvanceTime:
		// One phase forward. StepCount drives the day/night cycle, so
		// bumping it by a full phase advances the lighting without
		// teleporting the player or disturbing encounter pacing.
		g.StepCount += core.StepsPerPhase
	case core.DebugMenuEasyQuit:
		g.EasyBattleQuit = !g.EasyBattleQuit
	case core.DebugMenuRenderLog:
		g.RenderLogEnabled = !g.RenderLogEnabled
		if g.RenderLogEnabled {
			render.OpenRenderLog()
		} else {
			render.CloseRenderLog()
		}
	case core.DebugMenuDisable:
		g.DebugOverlay = false
		g.DebugMenuOpen = false
	case core.DebugMenuClose:
		g.DebugMenuOpen = false
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

func updatePlayer(g *core.GameState, dt float32) {
	p := &g.Player

	if p.Anim.Kind != core.AnimNone {
		updateAnimation(g, dt)
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
	// Step-into-pack starts a battle WITHOUT applying the move: the
	// player stays on the current tile and the battle splash takes
	// over. Checked BEFORE CanEnterTile so the engagement signal isn't
	// swallowed by the generic blocker rule. Pack-AI rolls happen on
	// a successful step, so we run them only when this branch DOESN'T
	// fire.
	if idx := core.PackIndexAtTile(g.Packs, targetX, targetZ); idx >= 0 && !g.EnemiesDisabled {
		if startTurnToTile(p, targetX, targetZ) {
			return
		}
		// Snap the engaging pack to its tile so the battle splash
		// doesn't show it mid-step. Mirrors the AI-side engagement
		// snap inside tickPackAI.
		core.SnapPackToTile(&g.Packs[idx])
		battle.Start(g, idx)
		return
	}
	// Everything else (walls, props, deep water, chests, other packs)
	// goes through CanEnterTile so the rule lives in one place.
	// AllowDoorTile=true because the player stepping onto a door fires
	// a transition; the engagement branch above already consumed the
	// pack-tile case, so we don't need OccupiedPacks here.
	if !core.CanEnterTile(g, targetX, targetZ, core.EnterOpts{AllowDoorTile: true}) {
		return
	}

	p.TileX = targetX
	p.TileZ = targetZ
	g.StepCount++
	// Out-of-battle poison tick: a fight-inflicted poison kept ticking
	// counters down on every party turn during battle but had no hook in
	// exploration, so the status would stick forever after a fight ended.
	// Hooking the tick here lines it up with the player's most natural
	// "unit of time outside combat" — one tile traversed.
	core.TickPoisonStep(g)
	// Fog-of-war reveal: every step paints the 3×3 window centered on
	// the player onto Visited, so the map shows the immediate vicinity
	// even when the player only ever brushed past a tile. Radius is a
	// fixed game-design constant (one tile of sight) — when we ever
	// want torchlight / vision modifiers, they pipe a different radius
	// into RevealRadius here instead of a second reveal pass.
	core.RevealRadius(g, targetX, targetZ, core.SightRadius)
	// Junkyard-dog AI tick: each alive pack rolls (independently) on
	// every successful player step. If a pack lands on the player,
	// initiate the battle and snap the player's VISUAL coords to the
	// new tile center so the splash doesn't show them mid-animation
	// frozen at the previous tile. The step animation is skipped on
	// engagement — battle takes the camera over immediately anyway.
	if engagedPack := tickPackAI(g); engagedPack >= 0 {
		p.X = core.TileCenter(targetX)
		p.Z = core.TileCenter(targetZ)
		battle.Start(g, engagedPack)
		return
	}
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

func updateAnimation(g *core.GameState, dt float32) {
	p := &g.Player
	p.Anim.Elapsed += dt
	t := p.Anim.Elapsed / p.Anim.Duration
	if t >= 1 {
		t = 1
	}
	eased := core.Smoothstep(t)

	finishedKind := p.Anim.Kind

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

	// Door trigger: stepping onto a door tile queues an area
	// transition for the run loop to consume on the next frame. We
	// only fire on AnimStep completion (not turns) so spinning in
	// place on top of a door doesn't loop-transition. The run loop
	// checks g.PendingTransition.TargetMap != "" and swaps.
	if finishedKind == core.AnimStep {
		tryQueueDoorTransition(g)
	}
}

// tryQueueDoorTransition checks whether the player just stepped onto a
// door tile and, if so, opens the confirm prompt (g.DoorPrompt) rather
// than transitioning immediately — the player chooses to enter or cancel.
// Doors with an empty TargetMap (defensive — the validator rejects these
// on load, but a hand-built editor state could slip one through) are
// ignored.
func tryQueueDoorTransition(g *core.GameState) {
	idx := core.DoorIndexAt(g.Doors, g.Player.TileX, g.Player.TileZ)
	if idx < 0 {
		return
	}
	if !g.Doors[idx].HasTarget() {
		return
	}
	g.DoorPrompt = idx
}

// updateDoorPrompt drives the "Enter <area>? / Cancel" confirm modal that
// opens when the player steps onto a door. Confirm queues the transition
// for the run loop; Back/cancel just closes the prompt and leaves the
// player standing on the door tile (they can walk off — the prompt only
// re-opens on a fresh step onto a door, not while standing still).
func updateDoorPrompt(g *core.GameState) {
	if g.DoorPrompt < 0 || g.DoorPrompt >= len(g.Doors) {
		g.DoorPrompt = -1
		return
	}
	if input.BackPressed() {
		g.DoorPrompt = -1
		return
	}
	if input.ConfirmPressed() {
		door := g.Doors[g.DoorPrompt]
		g.DoorPrompt = -1
		g.PendingTransition = core.AreaTransition{
			TargetMap:  door.TargetMap,
			TargetDoor: door.TargetDoor,
		}
	}
}

// tickPackAI advances every alive pack one step under the
// junkyard-dog rules in core.PlanPackSteps. Returns the index of a
// pack that walked onto the player's tile (battle should start with
// that pack), or -1 if no engagement happened this tick. Packs that
// chose to move have their tile AND animation updated; non-movers
// leave g.Packs alone so a paused pack just stays put.
//
// Engagement-on-AI-step is a hard win for the pack — there's no
// "turn to face" pre-amble like the player-side step-into path,
// because the pack chose the contact, not the player. The visual
// pack X/Z is snapped to the engagement tile so the battle splash
// doesn't show the pack mid-step.
func tickPackAI(g *core.GameState) int {
	// Debug enemies-off: packs neither wander nor engage. Bail before
	// planning so a disabled pack can't chase or step onto the player.
	if g.EnemiesDisabled {
		return -1
	}
	plans := core.PlanPackSteps(g)
	engaged := -1
	for _, plan := range plans {
		if !plan.Moved {
			continue
		}
		if plan.PackIdx < 0 || plan.PackIdx >= len(g.Packs) {
			continue
		}
		p := &g.Packs[plan.PackIdx]
		// Arm the visual animation BEFORE updating TileX/TileZ so
		// StartPackStep captures the current pack.X/Z as the "from"
		// (they still match the previous tile's center). The tile
		// update below jumps the logical position so collision /
		// AI planning on subsequent packs in this same tick see the
		// new occupancy via PlanPackSteps' own reservation.
		core.StartPackStep(p, plan.NextX, plan.NextZ)
		p.TileX = plan.NextX
		p.TileZ = plan.NextZ
		if plan.EngagePlayer && engaged < 0 {
			engaged = plan.PackIdx
			core.SnapPackToTile(p)
		}
	}
	return engaged
}

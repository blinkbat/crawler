package explore

import (
	"crawler/internal/app/audio"
	"crawler/internal/app/battle"
	"crawler/internal/app/core"
	"crawler/internal/app/input"
	"crawler/internal/app/render"
	"fmt"
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

	// Ambient-rain tint follows its target smoothly every adventure frame,
	// before the modal/battle dispatch below can early-return — so the
	// bluegray wash keeps easing in / out even while a panel is open or a
	// battle is running (the storm's step-driven state only changes during
	// free exploration; here it's purely the visual catch-up).
	core.TickWeather(g, dt)

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
	case core.ModalDialog:
		updateDialogModal(g)
		return
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
	case core.ModalShop:
		updateShop(g)
		return
	case core.ModalRetroMenu:
		updateRetroMenu(g)
		return
	case core.ModalDebugMenu:
		updateDebugMenu(g)
		return
	case core.ModalOptionsMenu:
		updateOptionsMenu(g)
		return
	case core.ModalPauseMenu:
		updateMenu(g)
		return
	case core.ModalNone:
		// No overlay is open — fall through to the panels-open shortcut,
		// pause check, and movement below.
	default:
		// ActiveModal is a hand-maintained enum ladder; a new modal value
		// without an arm here would silently fall through to movement
		// (eating input the overlay should own). Fail loudly instead,
		// matching updatePanels' missing-case panic.
		panic(fmt.Sprintf("explore: Update missing dispatch case for modal %d", core.ActiveModal(g)))
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
	// Confirm key also fires an adjacent charged healing crystal. Same
	// gate as the chest: a settled player pressing Confirm. Checked after
	// chests so the two interactions can't both fire on one press (a
	// crystal and a chest are never on the same tile anyway).
	if g.Player.Anim.Kind == core.AnimNone && tryUseAdjacentCrystal(g) {
		return
	}
	updatePlayer(g, dt)
}

// tryAdjacentInteraction is the shared Confirm-key adjacent-interaction gate
// behind tryOpenAdjacentChest and tryUseAdjacentCrystal — the two were
// byte-for-byte parallel (Confirm edge → find an in-reach target index → bail
// if none → act). `find` resolves the adjacent target index for this
// interaction (returns <0 when nothing is in reach); `act` performs the
// interaction on the resolved index. Returns true when the interaction fired,
// so the caller skips the rest of the explore tick (free-look, movement) this
// frame; false (no-op) when Confirm wasn't pressed or no target is in reach, so
// the press falls through to movement as usual. The chest/crystal finders take
// different slice types, so each caller closes over its own slice and exposes
// the same parameterless func() int shape here.
func tryAdjacentInteraction(find func() int, act func(idx int)) bool {
	if !input.ConfirmPressed() {
		return false
	}
	idx := find()
	if idx < 0 {
		return false
	}
	act(idx)
	return true
}

// tryOpenAdjacentChest is the Confirm-key interaction for chests. If
// the player is one tile away from a non-looted chest, open its modal
// and return true so the rest of the explore tick (free-look, movement)
// skips this frame.
func tryOpenAdjacentChest(g *core.GameState) bool {
	return tryAdjacentInteraction(
		func() int {
			return core.AdjacentInteractableChestIndex(g.Chests, g.Player.TileX, g.Player.TileZ)
		},
		func(idx int) {
			g.ChestOpen = idx
			g.ChestMenuIndex = 0
		},
	)
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
	g.MenuIndex = input.CursorUpDown(g.MenuIndex, core.PauseMenuCount)
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
		case core.PauseMenuOptions:
			openOptionsMenu(g)
		case core.PauseMenuDebug:
			openDebugMenu(g)
		case core.PauseMenuQuit:
			g.Quit = true
		}
	}
}

// updateLeafMenu runs the shared input loop for a leaf submenu (Options /
// Debug): Back clears the submenu's open flag, Up/Down moves its cursor,
// Confirm fires onConfirm with the selected row index. The pause root
// (updateMenu) doesn't use this — it carries extra Restart/Quit hotkeys —
// but the two leaf submenus had byte-identical skeletons, so this is the
// input-side companion to the render side's shared drawTitledMenuCard.
func updateLeafMenu(open *bool, index *int, count int, onConfirm func(item int)) {
	if input.BackPressed() {
		*open = false
		return
	}
	*index = input.CursorUpDown(*index, count)
	if input.ConfirmPressed() {
		onConfirm(*index)
	}
}

// openOptionsMenu swaps the pause menu for the Options submenu.
func openOptionsMenu(g *core.GameState) {
	g.MenuOpen = false
	g.OptionsMenuOpen = true
	g.OptionsMenuIndex = 0
}

// updateOptionsMenu drives the Options submenu: display-mode toggle, a
// jump to the party-stats dashboard, and a run restart. Back closes
// straight to explore (a leaf submenu, not a pause sub-page — same shape
// as the Debug submenu).
func updateOptionsMenu(g *core.GameState) {
	updateLeafMenu(&g.OptionsMenuOpen, &g.OptionsMenuIndex, core.OptionsMenuCount, func(item int) {
		switch core.OptionsMenuItem(item) {
		case core.OptionsMenuDisplay:
			render.ToggleDisplayMode()
		case core.OptionsMenuVibration:
			g.RumbleEnabled = !g.RumbleEnabled
		case core.OptionsMenuSave:
			saveGame(g)
		case core.OptionsMenuRestart:
			restartGame(g)
		case core.OptionsMenuClose:
			g.OptionsMenuOpen = false
		}
	})
}

// openDebugMenu swaps the pause menu for the Debug submenu. Always
// reachable now — the master "Debug Mode" on/off toggle lives inside the
// submenu (DebugMenuToggle) rather than gating access to it.
func openDebugMenu(g *core.GameState) {
	g.MenuOpen = false
	g.DebugMenuOpen = true
	g.DebugMenuIndex = 0
}

// updateDebugMenu drives the debug submenu: the debug-mode toggle, enemy
// on/off, advance the time-of-day phase, easy-battle-quit, render-log,
// and the audio sound-tester. Back closes straight to explore (a leaf,
// not a pause sub-page).
func updateDebugMenu(g *core.GameState) {
	updateLeafMenu(&g.DebugMenuOpen, &g.DebugMenuIndex, core.DebugMenuCount, func(item int) {
		switch core.DebugMenuItem(item) {
		case core.DebugMenuToggle:
			g.DebugOverlay = !g.DebugOverlay
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
		case core.DebugMenuJukebox:
			render.PlayJukebox()
		case core.DebugMenuAllSkills:
			g.DebugAllSkills = !g.DebugAllSkills
		case core.DebugMenuBoostStats:
			// One-shot action (not a toggle): each confirm stacks another boost.
			core.DebugBoostParty(g.Party, core.DebugStatBoost)
		case core.DebugMenuSkipBattles:
			g.DebugSkipBattles = !g.DebugSkipBattles
		case core.DebugMenuTestRumble:
			// Arm a pulse on g.Battle; the main loop's TickRumble/ApplyRumble
			// drives it every frame regardless of scene, so it fires even with
			// the pause menu open and out of combat.
			core.TriggerRumble(&g.Battle, core.RumbleTestStrength, core.RumbleTestDur)
		case core.DebugMenuRetro:
			openRetroMenu(g)
		case core.DebugMenuStartDialog:
			startFirstAreaDialog(g)
		case core.DebugMenuClose:
			g.DebugMenuOpen = false
		}
	})
}

// openRetroMenu swaps the Debug submenu for the Retro Filters sub-submenu —
// the same hand-off shape as pause → debug.
func openRetroMenu(g *core.GameState) {
	g.DebugMenuOpen = false
	g.RetroMenuOpen = true
	g.RetroMenuIndex = 0
}

// updateRetroMenu drives the Retro Filters submenu. The first
// RetroFilterCount rows are intensity sliders (cursor position == filter
// kind): Left/Right nudges the intensity by RetroFilterStep, Confirm toggles
// between off and the default intensity. Filters LAYER — any number can be
// non-zero at once; the render pass applies them all in one shader. The last
// two rows are Reset All and Close. Back returns to explore (leaf-menu rule).
func updateRetroMenu(g *core.GameState) {
	updateLeafMenu(&g.RetroMenuOpen, &g.RetroMenuIndex, core.RetroMenuCount, func(item int) {
		switch {
		case item < int(core.RetroFilterCount):
			core.ToggleRetroFilter(&g.RetroFilters, core.RetroFilterKind(item))
		case item == core.RetroMenuSkyToggle:
			g.RetroFilterSky = !g.RetroFilterSky
		case item == core.RetroMenuResetAll:
			g.RetroFilters = core.DefaultRetroFilters()
			g.RetroFilterSky = core.DefaultRetroFilterSky
		case item == core.RetroMenuAllOff:
			g.RetroFilters = [core.RetroFilterCount]float64{}
		case item == core.RetroMenuClose:
			g.RetroMenuOpen = false
		}
	})
	// Fine intensity adjust on the cursored slider row. Gated on the menu
	// still being open (updateLeafMenu may have just closed it via Back).
	if g.RetroMenuOpen && g.RetroMenuIndex < int(core.RetroFilterCount) {
		if delta := input.CursorLeftRight(); delta != 0 {
			core.AdjustRetroFilter(&g.RetroFilters[g.RetroMenuIndex], delta)
		}
	}
}

func restartGame(g *core.GameState) {
	core.ResetGameState(g)
}

// saveGame writes the run to disk from the Options submenu. Closes the
// submenu so the resulting status message (success or the error reason) is
// visible under the HUD, and plays a confirm / refusal ping. A failed write
// (read-only disk, permissions) surfaces its reason rather than silently
// no-op'ing.
func saveGame(g *core.GameState) {
	g.OptionsMenuOpen = false
	// Saving is a field action — refuse mid-battle. A save snapshots only
	// the persistent run (no battle state), so reloading would drop the
	// fight; and although NewSaveData defensively strips combat-transient
	// statuses, the cleaner contract is "you can't save during a fight."
	if g.Battle.Active() {
		g.SetStatusMessage("Can't save during a battle.")
		audio.Play(audio.SoundInputMiss)
		return
	}
	if err := core.SaveGame(g); err != nil {
		g.LogMessage("Save failed: " + err.Error())
		audio.Play(audio.SoundInputMiss)
		return
	}
	g.LogMessage("Game saved.")
	audio.Play(audio.SoundInputGreat)
}

// tryUseAdjacentCrystal is the Confirm-key interaction for healing crystals
// (mirrors tryOpenAdjacentChest). When the player presses Confirm one tile away
// from — or on top of — a CHARGED crystal, fire the Grimrock-style rest and
// return true so the rest of the explore tick (free-look, movement) skips this
// frame. No-op (returns false) when no charged crystal is in reach or Confirm
// wasn't pressed, so the press falls through to movement as usual.
func tryUseAdjacentCrystal(g *core.GameState) bool {
	return tryAdjacentInteraction(
		func() int {
			return core.AdjacentChargedCrystalIndex(g.Crystals, g.Player.TileX, g.Player.TileZ)
		},
		func(idx int) {
			fireHealingCrystal(g, idx)
		},
	)
}

// fireHealingCrystal restores the whole party to full HP+MP, puts the crystal
// dormant, and AUTOSAVEs (the codebase's first autosave). The heal+discharge
// always lands; the save is best-effort — it refuses on an unsaved editor-
// playtest map (Area.Path == ""), in which case the party is still restored and
// the message says so.
func fireHealingCrystal(g *core.GameState, idx int) {
	core.RestorePartyFully(g)
	g.Crystals[idx].Charged = false
	g.Crystals[idx].Charge = 0
	if err := core.SaveGame(g); err != nil {
		g.LogMessage("The crystal restores the party. (Autosave failed: " + err.Error() + ")")
		audio.Play(audio.SoundInputGreat)
		return
	}
	g.LogMessage("The crystal restores the party and saves your progress.")
	audio.Play(audio.SoundInputGreat)
}

func updateFreeLook(p *core.Player, dt float32) {
	// Mouse right-drag takes priority while held — relative motion, not
	// dt-scaled (one mouse delta = one yaw nudge).
	if input.LookDragActive() {
		mouse := input.LookMouseDelta()
		p.LookYaw = core.Clamp(p.LookYaw+mouse.X*core.MouseSense, -core.MaxLookYaw, core.MaxLookYaw)
		p.LookPitch = core.Clamp(p.LookPitch-mouse.Y*core.MouseSense, -core.MaxLookPitch, core.MaxLookPitch)
		return
	}
	// Right analog stick free-look — an analog hold, so dt-scaled by
	// StickLookSense. Mirrors the mouse axes (right = +yaw, up = +pitch)
	// and shares the same clamps + recenter-on-release so the two feel
	// identical.
	if sx, sy := input.LookStick(); sx != 0 || sy != 0 {
		p.LookYaw = core.Clamp(p.LookYaw+sx*core.StickLookSense*dt, -core.MaxLookYaw, core.MaxLookYaw)
		p.LookPitch = core.Clamp(p.LookPitch-sy*core.StickLookSense*dt, -core.MaxLookPitch, core.MaxLookPitch)
		return
	}
	p.LookYaw = core.Approach(p.LookYaw, 0, core.FreeLookReturnSpeed*dt)
	p.LookPitch = core.Approach(p.LookPitch, 0, core.FreeLookReturnSpeed*dt)
}

func updatePlayer(g *core.GameState, dt float32) {
	p := &g.Player

	// Mid-animation: advance it. When it COMPLETES with time to spare and a
	// movement key is still held, fall through to arm the next step and spend
	// the leftover on it this same frame. Without that carry, held movement
	// loses up to a frame of motion at every tile boundary (the finishing frame
	// snaps to center, the next frame only re-arms) — a periodic hitch that
	// reads as "not smooth." updateAnimation returns 0 while still running, or
	// when landing opened an overlay that should halt continued walking.
	if p.Anim.Kind != core.AnimNone {
		leftover := updateAnimation(g, dt)
		if leftover <= 0 || p.Anim.Kind != core.AnimNone {
			return
		}
		dt = leftover
	}

	// Held (level) reads, not edge: the player completes one step/turn per
	// animation, and holding the key re-fires the next as soon as this one
	// lands — continuous movement paced by the animation. A tap still steps
	// exactly once.
	switch {
	case input.TurnLeftHeld():
		startTurn(p, -1)
	case input.TurnRightHeld():
		startTurn(p, 1)
	case input.StepForwardHeld():
		startStep(p, g, 0, 1)
	case input.StepBackHeld():
		startStep(p, g, 0, -1)
	case input.StrafeLeftHeld():
		startStep(p, g, -1, 0)
	case input.StrafeRightHeld():
		startStep(p, g, 1, 0)
	}

	// Advance a freshly-armed step/turn by dt on the SAME frame it starts:
	// the full frame dt for a fresh press, or the carried-over remainder when
	// continuing held movement across a tile boundary — so motion flows instead
	// of resting a frame at FromX. startStep may start a battle instead (no Anim
	// armed) or the move may be blocked (no Anim); the guard skips both.
	if p.Anim.Kind != core.AnimNone {
		updateAnimation(g, dt)
	}
}

func startStep(p *core.Player, g *core.GameState, strafe, forward int) {
	// The tile the player stands on entering this step — the square a
	// successful Flee retreats to. For a step-into-pack engage the player
	// never moves (this stays the current tile); for a pack-ambush engage
	// (the player completes a step, then a pack lands on them) this is the
	// pre-step tile, so fleeing steps back OFF the pack rather than onto it.
	fleeFromX, fleeFromZ := p.TileX, p.TileZ
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
	// A pack on a cliff (different elevation level with no connecting ramp)
	// can't be reached, so it can't be engaged either — fall through to the
	// normal blocked-move handling. No-op on flat maps (StepElevationOK is
	// always true at equal levels).
	engageDir, engageDirOK := facingForTile(p, targetX, targetZ)
	engageReachable := true
	if engageDirOK {
		if len(g.Area.Solids) > 0 {
			_, engageReachable = g.Area.ResolveStep(p.TileX, p.Level, p.TileZ, engageDir)
		} else {
			engageReachable = g.Area.StepElevationOK(p.TileX, p.TileZ, engageDir)
		}
	}
	if idx := core.PackIndexAtTile(g.Packs, targetX, targetZ); idx >= 0 && !g.EnemiesDisabled && engageReachable {
		if startTurnToTile(p, targetX, targetZ) {
			return
		}
		// Snap the engaging pack to its tile so the battle splash
		// doesn't show it mid-step. Mirrors the AI-side engagement
		// snap inside tickPackAI.
		core.SnapPackToTile(&g.Packs[idx])
		// Debug "Skip Battles": auto-resolve the pack as a win (kills + XP +
		// loot) without entering the battle scene, then stay in explore.
		if g.DebugSkipBattles {
			battle.DebugSkipWin(g, idx)
			return
		}
		battle.Start(g, idx, fleeFromX, fleeFromZ)
		return
	}
	// Elevation/voxel gate FIRST: resolve WHICH surface the party would land on
	// (the ground under a floating deck vs the deck itself) and reject a cliff.
	// Steps are orthogonal, so the tile delta resolves to a single cardinal dir.
	// The entry check below then needs that level for level-aware prop blocking.
	landLevel := p.Level
	if dir, ok := facingForTile(p, targetX, targetZ); ok {
		if len(g.Area.Solids) > 0 {
			l, stepOK := g.Area.ResolveStep(p.TileX, p.Level, p.TileZ, dir)
			if !stepOK {
				return
			}
			landLevel = l
		} else {
			if !g.Area.StepElevationOK(p.TileX, p.TileZ, dir) {
				return
			}
			landLevel = g.Area.ElevationLevelAt(targetX, targetZ)
		}
	} else {
		// Non-cardinal step (shouldn't happen for orthogonal movement): keep the
		// old no-gate behavior, landing on the destination column's surface.
		if lo := g.Area.LowestStandableLevel(targetX, targetZ); lo >= 0 {
			landLevel = lo
		} else {
			landLevel = g.Area.ElevationLevelAt(targetX, targetZ)
		}
	}
	// Everything else (walls, props, deep water, chests, other packs) goes through
	// CanEnterTile so the rule lives in one place. AllowDoorTile=true because the
	// player stepping onto a door fires a transition; the engagement branch above
	// already consumed the pack-tile case, so we don't need OccupiedPacks here. On
	// a voxel map the level-aware variant lets a prop block only its own levels —
	// so you can walk UNDER a deck past a ground-rooted tree.
	if len(g.Area.Solids) > 0 {
		if !core.CanEnterTileAtLevel(g, targetX, targetZ, landLevel, core.EnterOpts{AllowDoorTile: true}) {
			return
		}
	} else if !core.CanEnterTile(g, targetX, targetZ, core.EnterOpts{AllowDoorTile: true}) {
		return
	}
	// Ground height the player leaves from — captured before TileX/TileZ/Level
	// advance so a ramp / level change can ease the camera between heights.
	fromGroundY := g.Area.StandGroundYAt(p.TileX, p.Level, p.TileZ)

	p.TileX = targetX
	p.TileZ = targetZ
	p.Level = landLevel
	g.StepCount++
	// Out-of-battle poison tick: a fight-inflicted poison kept ticking
	// counters down on every party turn during battle but had no hook in
	// exploration, so the status would stick forever after a fight ended.
	// Hooking the tick here lines it up with the player's most natural
	// "unit of time outside combat" — one tile traversed.
	core.TickPoisonStep(g)
	// Ambient weather advances on the same per-step beat: outdoors this
	// rolls to start / counts down a rain storm; indoors it recedes any
	// active storm. The smooth tint follow runs per frame in Update.
	core.TickWeatherStep(g)
	// Fog-of-war reveal: every step paints the 3×3 window centered on
	// the player onto Visited, so the map shows the immediate vicinity
	// even when the player only ever brushed past a tile. Radius is a
	// fixed game-design constant (one tile of sight) — when we ever
	// want torchlight / vision modifiers, they pipe a different radius
	// into RevealRadius here instead of a second reveal pass.
	core.RevealRadius(g, targetX, targetZ, core.SightRadius)
	// Healing crystals: recharge every dormant crystal one step. The heal
	// itself is no longer automatic — the player presses Confirm beside a
	// charged crystal to use it (tryUseAdjacentCrystal), like opening a chest.
	core.TickCrystalRecharge(g)
	// Pack-AI tick (per-pack mode dispatched in core.PlanPackSteps): each
	// alive pack plans (independently) on every successful player step. If a pack lands on the player,
	// initiate the battle and snap the player's VISUAL coords to the
	// new tile center so the splash doesn't show them mid-animation
	// frozen at the previous tile. The step animation is skipped on
	// engagement — battle takes the camera over immediately anyway.
	if engagedPack := tickPackAI(g); engagedPack >= 0 {
		core.SnapPlayerToTile(p) // p.TileX/Z were advanced to targetX/Z above
		battle.Start(g, engagedPack, fleeFromX, fleeFromZ)
		return
	}
	p.Anim = core.Animation{
		Kind:     core.AnimStep,
		Duration: core.StepDuration,
		FromX:    p.X,
		FromZ:    p.Z,
		ToX:      core.TileCenter(targetX),
		ToZ:      core.TileCenter(targetZ),
		FromY:    fromGroundY,
		ToY:      g.Area.StandGroundYAt(targetX, landLevel, targetZ),
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

// updateAnimation advances the active step/turn by dt. It returns the leftover
// time (Elapsed - Duration) once the animation COMPLETES so the caller can
// spend that remainder arming the next step in the same frame — the carry that
// keeps held movement continuous across tile boundaries. Returns 0 while the
// animation is still running, and 0 when landing opened an overlay (door
// prompt / enter-tile dialog) that should halt continued walking.
func updateAnimation(g *core.GameState, dt float32) float32 {
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
		p.GroundY = core.Lerp(p.Anim.FromY, p.Anim.ToY, eased)
	case core.AnimTurn:
		p.Yaw = core.Lerp(p.Anim.FromYaw, p.Anim.ToYaw, eased)
	}

	if p.Anim.Elapsed < p.Anim.Duration {
		return 0
	}
	leftover := p.Anim.Elapsed - p.Anim.Duration
	if p.Anim.Kind == core.AnimStep {
		core.SnapPlayerToTile(p)
		p.GroundY = p.Anim.ToY
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
		// Enter-tile dialog triggers fire on the same step-land beat as door
		// transitions. Skip when a door prompt opened on this tile (a portal
		// takes precedence) so the player isn't handed two overlays at once;
		// FireEnterTileTriggers itself no-ops if a dialog is already open.
		if g.DoorPrompt < 0 {
			core.FireEnterTileTriggers(g, g.Player.TileX, g.Player.TileZ)
		}
	}
	// If landing opened an overlay (door prompt or an enter-tile dialog), the
	// player shouldn't keep walking into it — swallow the remainder so the
	// caller doesn't arm another step this frame.
	if core.ActiveModal(g) != core.ModalNone {
		return 0
	}
	return leftover
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

// tickPackAI advances every alive pack one step under its authored AI mode
// (None / JunkyardDog / Patrol / Skittish, dispatched in core.PlanPackSteps).
// Returns the index of a
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
	// Plan then apply. The apply loop (single-engagement-per-tick rule, tile +
	// animation advance, patrol-dir persist) lives in core.ApplyPackSteps so the
	// engagement contract is unit-tested headlessly.
	return core.ApplyPackSteps(g, core.PlanPackSteps(g))
}

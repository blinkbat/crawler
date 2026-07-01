package explore

import (
	"crawler/internal/app/audio"
	"crawler/internal/app/battle"
	"crawler/internal/app/core"
	"crawler/internal/app/input"
	"crawler/internal/app/render"
	"fmt"
	"math"
)

// modalUpdaters dispatches the open explore-scene modal to its per-frame updater,
// keyed by core.ModalKind. dt is passed to every row (most ignore it; updatePanels
// animates with it). ModalNone has no row — Update handles it as fall-through. init
// asserts every non-None kind is filled so a new modal can't silently no-op.
var modalUpdaters = [core.ModalCount]func(*core.GameState, float32){
	core.ModalQuitConfirm: func(g *core.GameState, _ float32) { updateQuitConfirm(g) },
	core.ModalDialog:      func(g *core.GameState, _ float32) { updateDialogModal(g) },
	core.ModalLevelUp:     func(g *core.GameState, _ float32) { updateLevelUpModal(g) },
	core.ModalPanels:      func(g *core.GameState, dt float32) { updatePanels(g, dt) },
	core.ModalChest:       func(g *core.GameState, _ float32) { updateChestModal(g) },
	core.ModalDoorPrompt:  func(g *core.GameState, _ float32) { updateDoorPrompt(g) },
	core.ModalShop:        func(g *core.GameState, _ float32) { updateShop(g) },
	core.ModalRetroMenu:   func(g *core.GameState, _ float32) { updateRetroMenu(g) },
	core.ModalCombatTune:  func(g *core.GameState, _ float32) { updateCombatTuneMenu(g) },
	core.ModalWipeMenu:    func(g *core.GameState, _ float32) { updateWipeMenu(g) },
	core.ModalDebugMenu:   func(g *core.GameState, _ float32) { updateDebugMenu(g) },
	core.ModalOptionsMenu: func(g *core.GameState, _ float32) { updateOptionsMenu(g) },
	core.ModalSoundMenu:   func(g *core.GameState, _ float32) { updateSoundMenu(g) },
	core.ModalPauseMenu:   func(g *core.GameState, _ float32) { updateMenu(g) },
}

func init() {
	for m := core.ModalKind(0); m < core.ModalCount; m++ {
		if m == core.ModalNone {
			continue
		}
		if modalUpdaters[m] == nil {
			panic(fmt.Sprintf("explore: modalUpdaters missing a row for modal kind %d", m))
		}
	}
}

func Update(g *core.GameState) {
	// Clamp dt: a frame stall must not fast-forward animations or overshoot tile
	// targets. Single owner — battle.Update trusts the dt here is already clamped.
	dt := core.ClampFrameTime(input.FrameTime())

	// Weather tint eases every frame, before the early-returns below, so the wash
	// keeps catching up while a panel/battle is up (pure visual catch-up).
	core.TickWeather(g, dt)
	// Crystal touch-spin eases out here too, so the burst doesn't freeze if a modal
	// (e.g. the autosave-driven panel) opens mid-spin.
	core.TickCrystalSpins(g, dt)
	// Screen Wipe FX preview countdown — ticks even with the debug submenu open so the
	// previewed effect plays over the field.
	if g.BattleWipePreview > 0 {
		g.BattleWipePreview = core.ApproachZero(g.BattleWipePreview, dt)
	}

	// Modal dispatch: ActiveModal picks the open overlay (priority ladder lives there);
	// modalUpdaters owns it for this frame. Any non-None modal MUST have a row (init
	// asserts) — a gap would silently fall through to movement and eat the overlay's input.
	if m := core.ActiveModal(g); m != core.ModalNone {
		modalUpdaters[m](g, dt)
		return
	}

	// Panels-open shortcut: before the pause check so I / middle-button jumps in directly.
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
	pause := input.PausePressed()
	if pause && pauseAllowed(g) {
		g.MenuOpen = true
		resetLook(&g.Player)
		return
	}
	if g.Battle.Active() {
		battle.Update(g, dt)
		return
	}

	updateFreeLook(&g.Player, dt)
	// Tick pack animations every frame so a wandering pack moves while the player stands still.
	core.TickPackAnimations(g, dt)
	// Confirm opens an adjacent chest. Before movement so "step + Enter" doesn't double as a step.
	if g.Player.Anim.Kind == core.AnimNone && tryOpenAdjacentChest(g) {
		return
	}
	// Confirm also fires an adjacent charged crystal. After chests so one press can't fire both.
	if g.Player.Anim.Kind == core.AnimNone && tryUseAdjacentCrystal(g) {
		return
	}
	updatePlayer(g, dt)
}

// tryAdjacentInteraction is the shared Confirm-key gate: find an in-reach target
// (find returns <0 for none), act on it. Returns true when it fired so the
// caller skips the rest of the tick; false falls through to movement.
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

// tryOpenAdjacentChest opens an adjacent non-looted chest's modal on Confirm.
func tryOpenAdjacentChest(g *core.GameState) bool {
	return tryAdjacentInteraction(
		func() int {
			return core.AdjacentInteractableChestIndexOn(g.Chests, g.Player.TileX, g.Player.TileZ, g.Player.Level, g.Area.IsVoxel())
		},
		func(idx int) {
			g.ChestOpen = idx
			g.ChestMenuIndex = 0
		},
	)
}

// pauseAllowed forbids opening the pause menu while a battle timing bar is
// active, so the player can't sidestep the input window.
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
		openQuitConfirm(g)
		return
	}
	if input.ConfirmPressed() {
		switch core.PauseMenuItem(g.MenuIndex) {
		case core.PauseMenuOptions:
			openOptionsMenu(g)
		case core.PauseMenuDebug:
			openDebugMenu(g)
		case core.PauseMenuQuit:
			openQuitConfirm(g)
		default:
			// MenuIndex is clamped to PauseMenuCount above, so a new PauseMenuItem
			// row without a case here lands loudly instead of silently no-opping.
			panic(fmt.Sprintf("updateMenu: no handler for PauseMenuItem %d", g.MenuIndex))
		}
	}
}

// openQuitConfirm swaps the pause menu for the quit-confirmation prompt.
func openQuitConfirm(g *core.GameState) {
	g.MenuOpen = false
	g.QuitConfirmOpen = true
}

// updateQuitConfirm: Confirm sets g.Quit (run loop exits), Back returns to the
// pause menu. Gate between every quit trigger and the actual exit.
func updateQuitConfirm(g *core.GameState) {
	if input.BackPressed() {
		g.QuitConfirmOpen = false
		g.MenuOpen = true
		return
	}
	if input.ConfirmPressed() {
		g.QuitConfirmOpen = false
		g.Quit = true
	}
}

// updateLeafMenu is the shared input loop for a leaf submenu (Options / Debug):
// Back clears open, Up/Down moves index, Confirm fires onConfirm. The pause root
// doesn't use it (extra Restart/Quit hotkeys).
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

// updateListPicker is the shared 1-D sub-modal loop for the list pickers that own
// an int cursor + a slice (heal-skill, use-target, …): Back or an empty list calls
// close; CursorUpDown walks [0,count); Confirm fires confirm(index). Pickers with
// extra axes/inputs (skill tree's columns, chest's Take-All / dynamic clamp) keep
// their own loop. The caller resolves count/guards first, then hands the tail here.
func updateListPicker(index *int, count int, close func(), confirm func(item int)) {
	if input.BackPressed() || count <= 0 {
		close()
		return
	}
	*index = input.CursorUpDown(*index, count)
	if input.ConfirmPressed() && *index >= 0 && *index < count {
		confirm(*index)
	}
}

// openSubmenu is the shared open-a-submenu preamble: close the parent, open the
// child, reset the child cursor.
func openSubmenu(parentOpen, childOpen *bool, childIndex *int) {
	*parentOpen = false
	*childOpen = true
	*childIndex = 0
}

// resetLook neutralizes free-look so a half-rotated camera doesn't bleed into an
// overlay/menu.
func resetLook(p *core.Player) {
	p.LookYaw = 0
	p.LookPitch = 0
}

// openOptionsMenu swaps the pause menu for the Options submenu.
func openOptionsMenu(g *core.GameState) {
	openSubmenu(&g.MenuOpen, &g.OptionsMenuOpen, &g.OptionsMenuIndex)
}

// updateOptionsMenu drives the Options submenu. Back closes straight to explore
// (a leaf submenu).
func updateOptionsMenu(g *core.GameState) {
	updateLeafMenu(&g.OptionsMenuOpen, &g.OptionsMenuIndex, core.OptionsMenuCount, func(item int) {
		switch core.OptionsMenuItem(item) {
		case core.OptionsMenuDisplay:
			render.ToggleDisplayMode()
		case core.OptionsMenuVibration:
			g.RumbleEnabled = !g.RumbleEnabled
		case core.OptionsMenuSound:
			openSoundMenu(g)
		case core.OptionsMenuSave:
			saveGame(g)
		case core.OptionsMenuRestart:
			restartGame(g)
		case core.OptionsMenuClose:
			g.OptionsMenuOpen = false
		default:
			panic(fmt.Sprintf("updateOptionsMenu: no handler for OptionsMenuItem %d", item))
		}
	})
}

// openSoundMenu swaps the Options menu for the Sound submenu (music + SFX sliders).
func openSoundMenu(g *core.GameState) {
	openSubmenu(&g.OptionsMenuOpen, &g.SoundMenuOpen, &g.SoundMenuIndex)
}

// updateSoundMenu drives the Sound submenu: two Left/Right volume sliders (Music,
// SFX) then Close. Each nudge applies live (audible at once via the audio package);
// the volumes persist once on CLOSE (Back or the Close row), not per-nudge, so a
// held adjust isn't a disk thrash.
func updateSoundMenu(g *core.GameState) {
	wasOpen := g.SoundMenuOpen
	updateSliderLeafMenu(&g.SoundMenuOpen, &g.SoundMenuIndex, core.SoundMenuCount, core.SoundMenuSliderCount,
		func(item int) {
			switch core.SoundMenuItem(item) {
			case core.SoundMenuMusic, core.SoundMenuSFX:
				// Slider rows: Left/Right adjusts (below); Confirm is intentionally a no-op.
			case core.SoundMenuMute:
				audio.ToggleMute()
			case core.SoundMenuClose:
				g.SoundMenuOpen = false
			default:
				// All current rows are handled above; a new SoundMenuItem without a case
				// lands loudly instead of silently eating the confirm.
				panic(fmt.Sprintf("updateSoundMenu: no handler for SoundMenuItem %d", item))
			}
		},
		func(row, delta int) {
			step := float32(delta) * core.VolumeAdjustStep
			switch core.SoundMenuItem(row) {
			case core.SoundMenuMusic:
				audio.SetMusicVolume(audio.MusicVolume() + step)
			case core.SoundMenuSFX:
				audio.SetSFXVolume(audio.SFXVolume() + step)
			}
		})
	if wasOpen && !g.SoundMenuOpen {
		audio.SaveVolumeSettings() // persist on close (Back-out or Close row)
	}
}

// openDebugMenu swaps the pause menu for the Debug submenu. The master "Debug
// Mode" toggle lives inside the submenu, not gating access to it.
func openDebugMenu(g *core.GameState) {
	openSubmenu(&g.MenuOpen, &g.DebugMenuOpen, &g.DebugMenuIndex)
}

// updateDebugMenu drives the debug submenu. Back closes straight to explore.
func updateDebugMenu(g *core.GameState) {
	updateLeafMenu(&g.DebugMenuOpen, &g.DebugMenuIndex, core.DebugMenuCount, func(item int) {
		switch core.DebugMenuItem(item) {
		case core.DebugMenuToggle:
			g.DebugOverlay = !g.DebugOverlay
		case core.DebugMenuEnemies:
			g.EnemiesDisabled = !g.EnemiesDisabled
		case core.DebugMenuAdvanceTime:
			// StepCount drives the day/night cycle; bump a full phase to advance
			// lighting without moving the player.
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
			// Not a toggle: each confirm stacks another boost.
			core.DebugBoostParty(g.Party, core.DebugStatBoost)
		case core.DebugMenuSkipBattles:
			g.DebugSkipBattles = !g.DebugSkipBattles
		case core.DebugMenuTestRumble:
			// Arm a pulse on g.Battle; the main loop drives it every frame
			// regardless of scene, so it fires even with the pause menu open.
			core.TriggerRumble(&g.Battle, core.RumbleTestStrength, core.RumbleTestDur)
		case core.DebugMenuRetro:
			openRetroMenu(g)
		case core.DebugMenuCombatTune:
			openCombatTuneMenu(g)
		case core.DebugMenuWipe:
			openWipeMenu(g)
		case core.DebugMenuStartDialog:
			startFirstAreaDialog(g)
		case core.DebugMenuClose:
			g.DebugMenuOpen = false
		default:
			panic(fmt.Sprintf("updateDebugMenu: no handler for DebugMenuItem %d", item))
		}
	})
}

// openRetroMenu swaps the Debug submenu for the Retro Filters sub-submenu.
func openRetroMenu(g *core.GameState) {
	openSubmenu(&g.DebugMenuOpen, &g.RetroMenuOpen, &g.RetroMenuIndex)
}

// updateSliderLeafMenu drives a leaf submenu whose first sliderCount rows are
// Left/Right-adjustable sliders (the trailing rows are actions). It snapshots the
// cursored row BEFORE updateLeafMenu (which may move the cursor via Up/Down this same
// frame) so the Left/Right adjust acts on the row drawn highlighted this frame, not the
// one nav just moved to; the adjust is gated on still-open (Back may have closed it).
func updateSliderLeafMenu(open *bool, index *int, count, sliderCount int, onConfirm func(item int), adjust func(row, delta int)) {
	adjustRow := *index
	updateLeafMenu(open, index, count, onConfirm)
	if *open && adjustRow < sliderCount {
		if delta := input.CursorLeftRight(); delta != 0 {
			adjust(adjustRow, delta)
		}
	}
}

// updateRetroMenu drives the Retro Filters submenu. First RetroFilterCount rows
// are intensity sliders (cursor == filter kind); filters LAYER (all applied in
// one shader). Last rows are Reset All / Close. Back returns to explore.
func updateRetroMenu(g *core.GameState) {
	updateSliderLeafMenu(&g.RetroMenuOpen, &g.RetroMenuIndex, core.RetroMenuCount, int(core.RetroFilterCount),
		func(item int) {
			switch {
			case item < int(core.RetroFilterCount):
				core.ToggleRetroFilter(&g.RetroFilters, core.RetroFilterKind(item))
			case item == core.RetroMenuSkyToggle:
				g.RetroFilterSky = !g.RetroFilterSky
			case item == core.RetroMenuSpriteToggle:
				g.RetroFilterSprites = !g.RetroFilterSprites
			case item == core.RetroMenuResetAll:
				g.RetroFilters = core.DefaultRetroFilters()
				g.RetroFilterSky = core.DefaultRetroFilterSky
				g.RetroFilterSprites = core.DefaultRetroFilterSprites
			case item == core.RetroMenuAllOff:
				g.RetroFilters = [core.RetroFilterCount]float64{}
			case item == core.RetroMenuClose:
				g.RetroMenuOpen = false
			}
		},
		func(row, delta int) { core.AdjustRetroFilter(&g.RetroFilters[row], delta) })
}

// openWipeMenu swaps the Debug submenu for the Screen Wipe FX sub-submenu.
func openWipeMenu(g *core.GameState) {
	openSubmenu(&g.DebugMenuOpen, &g.WipeMenuOpen, &g.WipeMenuIndex)
}

// updateWipeMenu drives the Screen Wipe FX submenu: each row is a wipe kind (Confirm
// selects it AND plays a preview over the field), then Close. Back returns to explore.
func updateWipeMenu(g *core.GameState) {
	updateLeafMenu(&g.WipeMenuOpen, &g.WipeMenuIndex, core.BattleWipeMenuCount(), func(item int) {
		if item == core.BattleWipeCloseRow() {
			g.WipeMenuOpen = false
			return
		}
		g.BattleWipe = core.BattleWipeKind(item)
		g.BattleWipePreview = core.BattleWipePreviewSeconds // play it now over the field
	})
}

// openCombatTuneMenu swaps the Debug submenu for the Combat Tuning sub-submenu (live
// camera/foe/party placement sliders — best driven from inside a battle so the scene
// updates behind the panel).
func openCombatTuneMenu(g *core.GameState) {
	openSubmenu(&g.DebugMenuOpen, &g.CombatTuneOpen, &g.CombatTuneIndex)
}

// updateCombatTuneMenu drives the Combat Tuning submenu: the first slider rows take
// Left/Right to adjust the cursored value; the trailing rows are Reset / Dump /
// Close. Mirrors updateRetroMenu (snapshot the cursored row before nav so the adjust
// acts on the row drawn highlighted this frame).
func updateCombatTuneMenu(g *core.GameState) {
	updateSliderLeafMenu(&g.CombatTuneOpen, &g.CombatTuneIndex, core.BattleTuneMenuCount(), core.BattleTuneSliderCount(),
		func(item int) {
			switch {
			case item < core.BattleTuneSliderCount():
				// Slider row: Confirm is a no-op; Left/Right (below) does the adjust.
			case item == core.BattleTuneResetRow():
				g.BattleTuning = core.DefaultBattleTuning()
			case item == core.BattleTuneDumpRow():
				if path, err := core.DumpBattleTuning(&g.BattleTuning); err == nil {
					g.SetStatusMessage("Combat tuning written to " + path)
				} else {
					g.SetStatusMessage("Tuning dump failed: " + err.Error())
				}
			case item == core.BattleTuneCloseRow():
				g.CombatTuneOpen = false
			}
		},
		func(row, delta int) { core.AdjustBattleTuneSlider(&g.BattleTuning, row, delta) })
}

func restartGame(g *core.GameState) {
	core.ResetGameState(g)
	ResetTurnRepeat(g)
}

// saveGame writes the run to disk from the Options submenu. Closes the submenu
// so the status message is visible, and pings confirm / refusal.
func saveGame(g *core.GameState) {
	g.OptionsMenuOpen = false
	// Refuse mid-battle: a save snapshots only the persistent run (no battle
	// state), so reloading would drop the fight.
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

// tryUseAdjacentCrystal fires a charged healing crystal on Confirm when the
// player is adjacent to or on top of one (mirrors tryOpenAdjacentChest).
func tryUseAdjacentCrystal(g *core.GameState) bool {
	return tryAdjacentInteraction(
		func() int {
			return core.AdjacentChargedCrystalIndexOn(g.Crystals, g.Player.TileX, g.Player.TileZ, g.Player.Level, g.Area.IsVoxel())
		},
		func(idx int) {
			fireHealingCrystal(g, idx)
		},
	)
}

// fireHealingCrystal fully restores the party (HP+MP, REVIVING the dead), makes
// the crystal dormant, and AUTOSAVEs. Restore+discharge always land; the save is
// best-effort (refused on an unsaved editor-playtest map, Area.Path == "").
func fireHealingCrystal(g *core.GameState, idx int) {
	core.RestorePartyFully(g)
	// Arm the one-shot fast spin + play the dedicated sparkly cue on the touch (before
	// the gem goes dormant) — the crystal visibly expends its charge.
	g.Crystals[idx].SpinBurst = core.CrystalSpinBurstDuration
	g.Crystals[idx].Charged = false
	g.Crystals[idx].Charge = 0
	audio.Play(audio.SoundCrystal)
	// Soft shimmer in the hands as the crystal discharges (TickRumble runs every
	// frame on g.Battle, so this drives even outside combat).
	core.TriggerRumble(&g.Battle, core.RumbleCrystal, core.RumbleCrystalDur)
	if err := core.SaveGame(g); err != nil {
		g.LogMessageCat("The crystal restores the party. (Autosave failed: "+err.Error()+")", core.LogHeal)
		return
	}
	g.LogMessageCat("The crystal restores the party and saves your progress.", core.LogHeal)
}

// applyLook adds yaw/pitch deltas and clamps to the look bounds; the mouse and
// stick branches share it so their clamping can't diverge.
func applyLook(p *core.Player, dYaw, dPitch float32) {
	p.LookYaw = core.Clamp(p.LookYaw+dYaw, -core.MaxLookYaw, core.MaxLookYaw)
	p.LookPitch = core.Clamp(p.LookPitch+dPitch, -core.MaxLookPitch, core.MaxLookPitch)
}

func updateFreeLook(p *core.Player, dt float32) {
	// Mouse right-drag wins while held — relative motion, not dt-scaled.
	if input.LookDragActive() {
		mouse := input.LookMouseDelta()
		applyLook(p, mouse.X*core.MouseSense, -mouse.Y*core.MouseSense)
		return
	}
	// Right-stick free-look: analog hold, dt-scaled. Mirrors the mouse axes and
	// clamps so the two feel identical.
	if sx, sy := input.LookStick(); sx != 0 || sy != 0 {
		applyLook(p, sx*core.StickLookSense*dt, -sy*core.StickLookSense*dt)
		return
	}
	p.LookYaw = core.Approach(p.LookYaw, 0, core.FreeLookReturnSpeed*dt)
	p.LookPitch = core.Approach(p.LookPitch, 0, core.FreeLookReturnSpeed*dt)
}

// Turn auto-repeat pacing lives on GameState (g.TurnHeldLast / g.TurnRepeatCooldown):
// a fresh press turns once immediately; a held key waits core.TurnRepeatDelay after
// each turn before re-firing (without this a held key spins continuously the instant
// the turn animation finishes). TurnHeldLast distinguishes tap from hold.

// ResetTurnRepeat clears held-turn state. updatePlayer doesn't run during a
// battle or area swap, so a turn key held uninterrupted across one would carry a
// stale cooldown/held-edge into the next area. Called on transition + restart.
func ResetTurnRepeat(g *core.GameState) {
	g.TurnHeldLast = false
	g.TurnRepeatCooldown = 0
}

func updatePlayer(g *core.GameState, dt float32) {
	p := &g.Player

	// Detect a fresh turn press before any early return so the held-edge stays
	// tracked even during a turn animation. A new press or a release clears the
	// cooldown (self-clearing — no stale value survives a release / scene change).
	turnHeld := input.TurnLeftHeld() || input.TurnRightHeld()
	if !turnHeld || !g.TurnHeldLast {
		g.TurnRepeatCooldown = 0
	}
	g.TurnHeldLast = turnHeld

	// Mid-animation: advance it. On completion with leftover time and a key still
	// held, carry the leftover into arming the next step this same frame — without
	// it, held movement loses a frame of motion at every tile boundary (a hitch).
	// updateAnimation returns 0 while running, or when landing opened an overlay.
	if p.Anim.Kind != core.AnimNone {
		leftover := updateAnimation(g, dt)
		if leftover <= 0 || p.Anim.Kind != core.AnimNone {
			return
		}
		dt = leftover
	}

	// Held (level) reads, not edge: holding re-fires the next step as this one
	// lands (a tap still steps once). Turning is the exception — it waits out
	// turnRepeatCooldown. The cooldown counts down HERE (past the mid-animation
	// return) so it ticks only at rest between turns, making the gap exactly
	// core.TurnRepeatDelay; a fresh press zeroed it above, so taps are instant.
	if g.TurnRepeatCooldown > 0 {
		g.TurnRepeatCooldown -= dt
	}
	canTurn := g.TurnRepeatCooldown <= 0
	switch {
	case canTurn && input.TurnLeftHeld():
		startTurn(p, -1)
		g.TurnRepeatCooldown = core.TurnRepeatDelay
	case canTurn && input.TurnRightHeld():
		startTurn(p, 1)
		g.TurnRepeatCooldown = core.TurnRepeatDelay
	case input.StepForwardHeld():
		startStep(p, g, 1)
	case input.StepBackHeld():
		startStep(p, g, -1)
	}

	// Advance a freshly-armed step/turn by dt on the SAME frame it starts so
	// motion flows instead of resting a frame at FromX. startStep may start a
	// battle or be blocked (no Anim either way); the guard skips both.
	if p.Anim.Kind != core.AnimNone {
		updateAnimation(g, dt)
	}
}

func startStep(p *core.Player, g *core.GameState, forward int) {
	// The tile the player stands on entering this step — where a successful Flee
	// retreats to. For a step-into-pack engage this stays the current tile; for a
	// pack-ambush engage it's the pre-step tile, so fleeing steps off the pack.
	fleeFromX, fleeFromZ := p.TileX, p.TileZ
	dx, dz := core.FacingVector(p.Facing)
	targetX := p.TileX + dx*forward
	targetZ := p.TileZ + dz*forward
	// Step-into-pack starts a battle WITHOUT moving. Checked BEFORE CanEnterTile
	// so the engagement isn't swallowed by the generic blocker rule; pack-AI rolls
	// only run when this branch doesn't fire. A pack on an unreachable cliff falls
	// through to blocked-move handling (no-op on flat maps).
	engageDir, engageDirOK := facingForTile(p, targetX, targetZ)
	engageReachable := true
	engageLevel := p.Level
	if engageDirOK {
		// On a heightfield engageLevel is unused (packHit below is tile-only), but the
		// shared resolver keeps the voxel/heightfield split in core, not here.
		engageLevel, engageReachable = g.Area.ResolveStepLanding(p.TileX, p.Level, p.TileZ, engageDir)
	}
	// On a voxel map, only engage a pack on the surface the step lands on
	// (engageLevel) so walking UNDER a deck isn't ambushed by a pack on it. On a
	// heightfield, fall back to tile-only (pack Level isn't tracked per-surface).
	packHit := core.PackIndexAtTile(g.Packs, targetX, targetZ)
	if g.Area.IsVoxel() {
		// engageLevel is only meaningful for a resolved cardinal step; without one
		// (degenerate same-tile target) it defaults to p.Level and could match a pack
		// on the player's own surface, so gate the level-aware lookup on engageDirOK.
		packHit = -1
		if engageDirOK {
			packHit = core.PackIndexAtLanding(g.Packs, targetX, targetZ, engageLevel, true)
		}
	}
	if packHit >= 0 && !g.EnemiesDisabled && engageReachable {
		if startTurnToTile(p, targetX, targetZ) {
			return
		}
		// Snap the pack to its tile so the splash doesn't show it mid-step.
		core.SnapPackToTile(&g.Packs[packHit])
		// Debug "Skip Battles": auto-resolve as a win without the battle scene.
		if g.DebugSkipBattles {
			battle.DebugSkipWin(g, packHit)
			return
		}
		// Stepped INTO the pack — head-on engage, home formation stands.
		battle.Start(g, packHit, fleeFromX, fleeFromZ, core.EngageFront)
		return
	}
	// Elevation/voxel gate FIRST: resolve which surface the party lands on (ground
	// under a deck vs the deck) and reject a cliff. The entry check below needs
	// that level for level-aware prop blocking.
	landLevel := p.Level
	if dir, ok := facingForTile(p, targetX, targetZ); ok {
		l, stepOK := g.Area.ResolveStepLanding(p.TileX, p.Level, p.TileZ, dir)
		if !stepOK {
			return
		}
		landLevel = l
	} else {
		// Non-cardinal step (shouldn't happen for orthogonal movement): no gate,
		// land on the destination column's surface.
		if lo := g.Area.LowestStandableLevel(targetX, targetZ); lo >= 0 {
			landLevel = lo
		} else {
			landLevel = g.Area.ElevationLevelAt(targetX, targetZ)
		}
	}
	// Everything else (walls, props, deep water, chests, packs) goes through
	// CanEnterTile. AllowDoorTile=true (stepping onto a door fires a transition);
	// the engagement branch already consumed the pack-tile case. The voxel
	// level-aware variant lets a prop block only its own levels (walk under a deck).
	if !core.CanEnterLanding(g, targetX, targetZ, landLevel, core.EnterOpts{AllowDoorTile: true}, g.Area.IsVoxel()) {
		return
	}
	// Ground height left from — captured before the TileX/Z/Level advance so a
	// ramp / level change can ease the camera between heights.
	fromGroundY := g.Area.StandGroundYAt(p.TileX, p.Level, p.TileZ)

	p.TileX = targetX
	p.TileZ = targetZ
	p.Level = landLevel
	g.StepCount++
	// Out-of-battle poison tick, hooked here (one tile = the unit of time outside
	// combat) so a fight-inflicted poison doesn't stick forever.
	core.TickPoisonStep(g)
	// Satiety burns one step per tile crawled (battles don't advance it, mirroring
	// the day cycle); conscious members starve at SatietyMax.
	core.TickHungerStep(g)
	// Weather per step: outdoors rolls/counts down a storm, indoors recedes one.
	core.TickWeatherStep(g)
	// Fog-of-war: paint the 3×3 window onto Visited (radius = one tile of sight).
	core.RevealRadius(g, targetX, targetZ, core.SightRadius)
	// Recharge every dormant crystal one step (using one is manual, not automatic).
	core.TickCrystalRecharge(g)
	// Pack-AI tick: each alive pack plans on every successful step. If one lands
	// on the player, start the battle and snap the player's visual coords to the
	// tile center (the step animation is skipped — battle takes the camera).
	if engagedPack := tickPackAI(g); engagedPack >= 0 {
		core.SnapPlayerToTile(p) // p.TileX/Z were advanced to targetX/Z above
		// Pack stepped onto the player — ambush from behind; back rank shoved to
		// the exposed front until they reposition.
		battle.Start(g, engagedPack, fleeFromX, fleeFromZ, core.EngageBack)
		return
	}
	// A clean walking step (no battle/ambush consumed it above) — play a footstep.
	audio.PlayFootstep()
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
	default:
		// NormalizeFacing constrains diff to 0..3; out of range means its
		// contract changed. Fail loudly rather than no-op the turn.
		panic(fmt.Sprintf("explore: startTurnToTile got out-of-range facing diff %d", diff))
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

// updateAnimation advances the active step/turn by dt and returns the leftover
// time (Elapsed - Duration) on completion, so the caller can arm the next step
// the same frame (the carry that keeps held movement continuous). Returns 0
// while still running, and 0 when landing opened an overlay.
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

	// Door trigger: stepping onto a door queues an area transition. Only on
	// AnimStep completion (not turns) so spinning on a door doesn't loop-
	// transition.
	if finishedKind == core.AnimStep {
		tryQueueDoorTransition(g)
		// Enter-tile + enter-location triggers fire on the same step-land beat. Skip
		// when a door prompt opened here (portal takes precedence). Location detection
		// runs even if an enter-tile dialog opened, so its inside-set stays current
		// (the firing itself no-ops while a dialog is up).
		if g.DoorPrompt < 0 {
			core.FireEnterTileTriggers(g, g.Player.TileX, g.Player.TileZ)
			core.FireEnterLocationTriggers(g, g.Player.TileX, g.Player.TileZ, g.Player.Level)
		}
	}
	// If landing opened an overlay, swallow the remainder so the caller doesn't
	// arm another step into it this frame.
	if core.ActiveModal(g) != core.ModalNone {
		return 0
	}
	return leftover
}

// tryQueueDoorTransition opens the confirm prompt (g.DoorPrompt) when the player
// stepped onto a door, rather than transitioning immediately. Doors with an
// empty TargetMap are ignored (defensive — the validator rejects these on load).
func tryQueueDoorTransition(g *core.GameState) {
	idx := core.DoorIndexOn(g.Doors, g.Player.TileX, g.Player.TileZ, g.Player.Level, g.Area.IsVoxel())
	if idx < 0 {
		return
	}
	if !g.Doors[idx].HasTarget() {
		return
	}
	g.DoorPrompt = idx
}

// updateDoorPrompt drives the "Enter <area>? / Cancel" modal. Confirm queues the
// transition; Back closes the prompt, leaving the player on the door tile (it
// only re-opens on a fresh step onto a door, not while standing still).
func updateDoorPrompt(g *core.GameState) {
	if g.DoorPrompt < 0 || g.DoorPrompt >= len(g.Doors) {
		g.DoorPrompt = core.NoIndex
		return
	}
	if input.BackPressed() {
		g.DoorPrompt = core.NoIndex
		return
	}
	if input.ConfirmPressed() {
		door := g.Doors[g.DoorPrompt]
		g.DoorPrompt = core.NoIndex
		g.PendingTransition = core.AreaTransition{
			TargetMap:  door.TargetMap,
			TargetDoor: door.TargetDoor,
		}
	}
}

// tickPackAI advances every alive pack one step under its AI mode (dispatched in
// core.PlanPackSteps). Returns the index of a pack that walked onto the player's
// tile (start a battle), or -1. Engagement-on-AI-step is a hard win for the pack
// (no turn-to-face pre-amble like the player-side path).
func tickPackAI(g *core.GameState) int {
	// Debug enemies-off: bail before planning so a disabled pack can't chase.
	if g.EnemiesDisabled {
		return -1
	}
	// Plan then apply. The apply loop lives in core.ApplyPackSteps so the
	// engagement contract is unit-tested headlessly.
	return core.ApplyPackSteps(g, core.PlanPackSteps(g))
}

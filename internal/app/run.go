package app

import (
	"crawler/internal/app/audio"
	"crawler/internal/app/core"
	"crawler/internal/app/core/mapfile"
	"crawler/internal/app/editor"
	"crawler/internal/app/explore"
	"crawler/internal/app/input"
	"crawler/internal/app/render"
	"crawler/internal/app/title"

	rl "github.com/gen2brain/raylib-go/raylib"
)

type scene int

const (
	sceneTitle scene = iota
	sceneAdventure
	sceneEditor
)

type appState struct {
	scene  scene
	title  title.State
	game   core.GameState
	editor editor.State
	// testFromEditor flips on when the editor's F5 path drops us into
	// adventure with the in-memory area. Quitting from the in-game menu
	// then returns to the editor instead of the title screen.
	testFromEditor bool
	quit           bool
}

func Run() {
	// Note: FlagWindowHighdpi was tried here to keep HUD layout math aligned
	// on fractional-DPI displays, but in practice it caused a right/down
	// drift on Windows 1.5× scaling because raylib's GetScreenWidth returns
	// logical points while the framebuffer is scaled up — HUD coords landed
	// in the wrong pixel band. The flag stays off until the layout code is
	// audited to consistently use one of GetScreenWidth (logical) or
	// GetRenderWidth (physical) end-to-end.
	rl.SetConfigFlags(rl.FlagVsyncHint | rl.FlagWindowResizable)
	rl.InitWindow(core.InitialWindowWidth, core.InitialWindowHeight, "Crawler")
	defer rl.CloseWindow()

	rl.SetExitKey(rl.KeyNull)
	render.SetDisplayMode(render.DisplayFullscreen)
	rl.SetTargetFPS(core.TargetFPS)

	// Procedural sound bank — short input/hit/heal/death cues, generated in
	// code from sine sweeps and bell envelopes. Safe to call on systems with
	// no audio device (Init becomes a no-op; Play stays silent).
	audio.Init()
	defer audio.Close()

	assets := render.LoadResources()
	defer assets.Unload()
	// Flush + close the debug render log on exit if it was left enabled
	// (the Debug-menu toggle is the only other close path). Idempotent.
	defer render.CloseRenderLog()
	// Cut controller vibration on exit so the pad doesn't keep buzzing if the
	// process lingers after the window closes.
	defer input.StopRumble()

	state := appState{scene: sceneTitle, title: title.New()}

	for !rl.WindowShouldClose() && !state.quit {
		dt := rl.GetFrameTime()
		// Sample the analog stick once per frame so the directional edge
		// predicates (UpPressed / CursorUpDown / …) are idempotent within
		// the frame — see input.NewFrame.
		input.NewFrame()

		switch state.scene {
		case sceneTitle:
			updateTitleScene(&state)
		case sceneAdventure:
			updateAdventureScene(&state)
		case sceneEditor:
			updateEditorScene(&state, dt)
		default:
			panic("run: unhandled scene in update dispatch — add it to both scene switches")
		}

		// Drive controller rumble every frame, scene-independently: TickRumble
		// decays the combat-rumble envelope (and returns 0 outside battle / in
		// the title+editor scenes, where Battle is zero-valued), so a rumble
		// armed just before a battle ends still eases off instead of sticking.
		input.ApplyRumble(core.TickRumble(&state.game.Battle, dt), state.game.RumbleEnabled)

		rl.BeginDrawing()
		switch state.scene {
		case sceneTitle:
			title.Draw(state.title, assets)
		case sceneAdventure:
			drawAdventureScene(&state.game, assets)
		case sceneEditor:
			editor.Draw(&state.editor, assets)
		default:
			panic("run: unhandled scene in draw dispatch — add it to both scene switches")
		}
		rl.EndDrawing()
	}
}

func updateTitleScene(state *appState) {
	switch title.Update(&state.title) {
	case title.ActionStartAdventure:
		path := state.title.ChosenMapPath()
		area, err := core.LoadArea(path)
		if err != nil {
			state.title.SetLoadError("Failed: " + err.Error())
			return
		}
		state.game = core.NewGameState(area)
		state.scene = sceneAdventure
	case title.ActionContinue:
		data, err := core.LoadSave()
		if err != nil {
			state.title.SetLoadError("Load failed: " + err.Error())
			return
		}
		game, err := core.GameStateFromSave(data)
		if err != nil {
			state.title.SetLoadError("Load failed: " + err.Error())
			return
		}
		state.game = game
		state.scene = sceneAdventure
	case title.ActionOpenEditor:
		state.editor = editor.New()
		state.scene = sceneEditor
	case title.ActionQuit:
		state.quit = true
	}
}

func updateAdventureScene(state *appState) {
	explore.Update(&state.game)
	if state.game.PendingTransition.TargetMap != "" {
		if err := applyAreaTransition(&state.game); err != nil {
			// Transition resolution failed — clear the request, surface
			// the error in the quiet message slot so the player sees a
			// clue, and otherwise drop the player off where they were.
			// Common cause: target map file missing, or the named door
			// doesn't exist in the destination.
			state.game.SetStatusMessage("Door failed: " + err.Error())
		}
		state.game.PendingTransition = core.AreaTransition{}
	}
	if state.game.Quit {
		state.game.Quit = false
		if state.testFromEditor {
			state.testFromEditor = false
			state.scene = sceneEditor
			// Drop lingering render-side particles, same as the return-to-
			// title path: quitting a playtest mid-battle would otherwise
			// freeze formation-relative particles in the pool and thaw them
			// onto the next F5 playtest at stale positions.
			render.ResetParticles()
			return
		}
		returnToTitleScene(state)
	}
}

// returnToTitleScene resets the active scene to the title menu and
// rebuilds a fresh title.State. Both the adventure-quit path and the
// editor's ExitToTitle path land here so the "back to title" rule
// (scene flag + title re-init) lives in one place rather than being
// duplicated across two scene updaters.
func returnToTitleScene(state *appState) {
	state.scene = sceneTitle
	state.title = title.New()
	// Drop any lingering render-side particles — the title scene
	// doesn't draw the adventure pipeline, so the pool would freeze
	// mid-animation and thaw onto the next adventure entry at stale
	// world positions.
	render.ResetParticles()
}

// applyAreaTransition loads the destination map and rebuilds the
// GameState so the player exits through the matching door. Inventory,
// party (HP/MP/levels/status), gold, the quest journal, and StepCount are
// preserved across the transition — only the world (area, packs, chests,
// doors) is swapped. Battle / chest / modal state is dropped, since any of
// those flags would dangle pointers into the old area.
//
// Fog-of-war contract: the destination's Visited grid is allocated
// fresh by NewGameState (with only the destination's start tile pre-
// marked) and the SOURCE map's Visited is discarded. Returning to a
// previously-explored map re-fogs it. Intentional per-area-is-its-
// own-discovery feel; if cross-area persistence is wanted later, swap
// to a session-level `map[mapID][][]bool` and copy in/out here.
func applyAreaTransition(g *core.GameState) error {
	target := g.PendingTransition.TargetMap
	doorName := g.PendingTransition.TargetDoor
	if target == "" || doorName == "" {
		// Same shape as Door.HasTarget — the predicate is identical, but
		// the runtime queues the transition before resolving to a Door,
		// so we read the raw PendingTransition fields here. Empty either
		// = no transition queued.
		return nil
	}
	// Same-map portals don't reload from disk — the current area
	// already holds the destination door. Skipping the load avoids
	// resetting Packs / Chests for an in-map teleport.
	currentID := core.MapIDFromPath(g.Area.Path)
	if target == currentID || target == mapfile.SelfMapToken {
		dest := core.DoorByName(g.Doors, doorName)
		if dest == nil {
			return errDoorNotFound(target, doorName)
		}
		x, z := doorExitTile(g.Area, g.Doors, *dest)
		g.Player = core.NewPlayer(x, z, dest.Facing)
		// Symmetry with the cross-map branch below (which rebuilds the
		// whole GameState): close any open equipment picker and clear the
		// particle pool so an in-map teleport can't leave the sub-modal
		// stranded or leave stale VFX anchored at the old position.
		// (Latent today — only the door prompt queues a transition and the
		// panels overlay is closed during movement — but keeps a future
		// "teleport from an open panel" honest.)
		core.CloseEquipPicker(g)
		core.RequestVFXReset(g)
		return nil
	}
	area, err := core.LoadArea(core.MapPath(target))
	if err != nil {
		return err
	}
	next := core.NewGameState(area)
	// Carry forward the things that belong to the party, not the world.
	next.Party = g.Party
	next.Inventory = g.Inventory
	// Gold + the quest journal travel with the party, not the map — without
	// this they'd reset to the fresh-state seed (0 gold, starter quests)
	// every time the player walked through a door.
	next.Gold = g.Gold
	next.Quests = g.Quests
	next.StepCount = g.StepCount
	// Carry the storm across the threshold like the day/night phase: an
	// outdoor->outdoor door keeps the rain rolling; stepping into a roofed
	// area lets the next TickWeatherStep recede it.
	next.Weather = g.Weather
	next.RNG = g.RNG
	next.DebugOverlay = g.DebugOverlay
	next.EnemiesDisabled = g.EnemiesDisabled
	next.EasyBattleQuit = g.EasyBattleQuit
	// The render-log FILE stays open across the transition (it's not closed
	// here), and the per-frame logger gates on the file-open state, so the
	// flag must carry too — otherwise the Debug submenu reads "Render Log:
	// Off" while the log keeps writing, desyncing the toggle.
	next.RenderLogEnabled = g.RenderLogEnabled
	dest := core.DoorByName(next.Doors, doorName)
	if dest == nil {
		return errDoorNotFound(target, doorName)
	}
	x, z := doorExitTile(next.Area, next.Doors, *dest)
	next.Player = core.NewPlayer(x, z, dest.Facing)
	*g = next
	// Signal the render layer to drop any lingering particles —
	// formation-relative VFX from a fight that ended just before
	// the door step would otherwise drift through the new area's
	// camera view.
	core.RequestVFXReset(g)
	return nil
}

// doorExitTile picks the tile the player materializes onto when
// stepping through a door. Prefer the tile one step in the door's
// facing direction (so the destination door is behind them and they
// don't immediately re-trigger). Fall back to the door tile itself
// when the preferred exit is blocked or holds another door — better
// to risk a same-frame re-trigger than to drop the player onto a
// wall.
func doorExitTile(area core.AreaDefinition, doors []core.Door, dest core.Door) (int, int) {
	dx, dz := core.FacingVector(dest.Facing)
	fx, fz := dest.TileX+dx, dest.TileZ+dz
	if area.InBounds(fx, fz) && !area.BlockedAt(fx, fz) && core.DoorIndexAt(doors, fx, fz) < 0 {
		return fx, fz
	}
	return dest.TileX, dest.TileZ
}

type errDoorMiss struct {
	mapID    string
	doorName string
}

func (e errDoorMiss) Error() string {
	return "no door named '" + e.doorName + "' in map '" + e.mapID + "'"
}

func errDoorNotFound(mapID, doorName string) error {
	return errDoorMiss{mapID: mapID, doorName: doorName}
}

func updateEditorScene(state *appState, dt float32) {
	switch editor.Update(&state.editor, dt) {
	case editor.ActionExitToTitle:
		returnToTitleScene(state)
	case editor.ActionTest:
		// Build a runtime GameState from the in-memory area without
		// touching disk. The editor's State stays intact so we land back
		// on the same map (and the same dirty-marker) when the player
		// quits the playtest. StepCount is seeded from the editor's
		// previewed phase so the playtest opens in that lighting.
		area := state.editor.Area()
		// Ctrl+F5 test-from-cursor: temporarily override StartTile to
		// the editor's grid cursor for this run only. The authored
		// area on disk and in the editor's State keeps its original
		// StartTile; we just point the runtime player there.
		if x, z, ok := state.editor.TestStartOverride(); ok {
			area.StartTileX = x
			area.StartTileZ = z
			state.editor.ClearTestStartOverride()
		}
		state.game = core.NewGameState(area)
		state.game.StepCount = state.editor.PreviewStepCount()
		state.scene = sceneAdventure
		state.testFromEditor = true
	}
}

func drawAdventureScene(game *core.GameState, assets render.Resources) {
	camera := render.Camera(*game)
	// Explicit clear is REQUIRED — bisect confirmed that without it
	// the world geometry (props in particular) flickers and trees
	// hide behind invisible depth-buffer holes that shift as the
	// camera moves. The expected raylib contract is that
	// BeginDrawing's implicit rlClearScreenBuffers covers depth,
	// but on this build the depth buffer carries stale fragments
	// from the previous frame without the explicit ClearBackground
	// here. DrawSkyBackground paints the full viewport in 2D so the
	// sky-blue color set by the clear is overdrawn immediately —
	// the color value is irrelevant, the call is for the depth wipe
	// that comes with it.
	rl.ClearBackground(rl.NewColor(87, 172, 244, 255))
	render.DrawSkyBackground(assets, *game)
	rl.BeginMode3D(camera)
	render.DrawWorld(camera, *game, assets)
	render.DrawChests(camera, *game, assets)
	render.DrawDoors(camera, *game, assets)
	render.DrawEnemies(camera, *game, assets)
	render.DrawPartySprites(camera, *game, assets)
	// VFX inside the 3D pass so billboard particles depth-sort with
	// the rest of the scene. TickAndDrawVFX drains GameState.VFXQueue
	// (mutating g), advances the render-side pool by raylib's frame
	// dt, and emits draws for every live particle. Kept after the
	// party draw so impact sparks paint over the sprite, not under.
	render.TickAndDrawVFX(camera, game, assets)
	rl.EndMode3D()
	// Ambient rain sits above the 3D world (darkening it) but below the
	// world-space popups and HUD, so combat numbers, prompts, and menus
	// stay readable through the storm. No-op when the weather is clear.
	render.DrawWeather(*game)
	// Danger vignette — claret edges breathing while an enemy is mid-swing
	// (defend timing). Same layer slot as the weather wash: over the world,
	// under every popup and HUD pane. No-op outside that phase.
	render.DrawBattleDangerVignette(*game)
	render.DrawChestPrompt(camera, *game, assets)
	// Hit-glyph clarity shapes over struck targets — HUD pass (crisp 2D), but
	// before the damage popups so the number floats on top of the glyph.
	render.DrawHitGlyphs(camera)
	render.DrawDamagePopups(camera, *game, assets)
	render.DrawQualityPopup(camera, *game, assets)
	render.DrawDebugOverlay(camera, *game, assets)
	render.DrawOverlay(*game, assets)
	render.DrawChestModal(*game, assets)
	render.DrawLevelUpModal(*game, assets)
	render.DrawPanelsOverlay(*game, assets)
	render.DrawDoorPrompt(*game, assets)
}

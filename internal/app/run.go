package app

import (
	"fmt"

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
	sceneCount = int(sceneEditor) + 1 // sentinel: number of scenes (sizes sceneHandlers)
)

// sceneHandlers pairs each scene's per-frame update and draw so the two can't desync
// (previously two hand-maintained switch ladders). A scene added without a row here
// fails the init assert below. dt is ignored by scenes that don't need it.
var sceneHandlers = [sceneCount]struct {
	update func(*appState, float32)
	draw   func(*appState, render.Resources)
}{
	sceneTitle:     {func(s *appState, _ float32) { updateTitleScene(s) }, func(s *appState, a render.Resources) { title.Draw(s.title, a) }},
	sceneAdventure: {func(s *appState, _ float32) { updateAdventureScene(s) }, func(s *appState, a render.Resources) { drawAdventureScene(&s.game, a) }},
	sceneEditor:    {updateEditorScene, func(s *appState, a render.Resources) { editor.Draw(&s.editor, a) }},
}

func init() {
	for s, h := range sceneHandlers {
		if h.update == nil || h.draw == nil {
			panic(fmt.Sprintf("run: scene %d has no sceneHandlers row (update/draw)", s))
		}
	}
}

type appState struct {
	scene  scene
	title  title.State
	game   core.GameState
	editor editor.State
	// testFromEditor: set when the editor's F5 path enters adventure with the
	// in-memory area, so quitting returns to the editor, not the title.
	testFromEditor bool
	quit           bool
}

func Run() {
	// FlagWindowHighdpi stays off: on Windows 1.5× it drifts HUD coords (logical
	// GetScreenWidth vs scaled framebuffer). Re-enable only once layout uses one
	// of GetScreenWidth/GetRenderWidth consistently.
	rl.SetConfigFlags(rl.FlagVsyncHint | rl.FlagWindowResizable)
	rl.InitWindow(core.InitialWindowWidth, core.InitialWindowHeight, "Crawler")
	defer rl.CloseWindow()

	rl.SetExitKey(rl.KeyNull)
	render.SetDisplayMode(render.DisplayFullscreen)
	rl.SetTargetFPS(core.TargetFPS)

	// Procedural sound bank. Safe with no audio device (Init no-ops, Play silent).
	audio.Init()
	defer audio.Close()

	assets := render.LoadResources()
	defer assets.Unload()
	// Flush + close the debug render log if it was left enabled. Idempotent.
	defer render.CloseRenderLog()
	// Cut rumble on exit so the pad doesn't keep buzzing if the process lingers.
	defer input.StopRumble()

	state := appState{scene: sceneTitle, title: title.New()}

	for !rl.WindowShouldClose() && !state.quit {
		dt := rl.GetFrameTime()
		// Sample the stick once per frame so directional edge predicates are
		// idempotent within the frame — see input.NewFrame.
		input.NewFrame()
		// Sample window size + wall-clock once per frame so the many per-frame HUD
		// helpers read cached values instead of each making cgo round-trips.
		render.BeginFrame()

		// Global Alt+Enter display toggle, scene-independent and BEFORE the
		// scene update. Enter-based confirms ignore Enter-with-Alt so this
		// press never doubles as a commit.
		if input.DisplayTogglePressed() {
			render.ToggleDisplayMode()
		}

		handler := sceneHandlers[state.scene]
		handler.update(&state, dt)

		// Drive rumble every frame: TickRumble decays the combat envelope (0
		// outside battle) so a rumble armed just before a battle ends eases off.
		input.ApplyRumble(core.TickRumble(&state.game.Battle, dt), state.game.RumbleEnabled)

		// Feed + crossfade the BGM every frame. Music plays in the adventure scene
		// (not title/editor); inBattle swaps the explore theme for the battle theme, so
		// entering/exiting combat crossfades between them. No-ops without a device.
		inAdventure := state.scene == sceneAdventure
		inBattle := inAdventure && state.game.Battle.Active()
		audio.UpdateMusic(dt, inAdventure, inBattle)
		// Release the second footfall of a walk-step cluster once its gap elapses.
		audio.UpdateFootsteps(dt)

		rl.BeginDrawing()
		// Re-read state.scene: update may have changed it this frame (the draw should
		// follow the scene we're transitioning into, as the switch did).
		sceneHandlers[state.scene].draw(&state, assets)
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
		state.editor = editor.NewDefault()
		state.scene = sceneEditor
	case title.ActionQuit:
		state.quit = true
	}
}

func updateAdventureScene(state *appState) {
	explore.Update(&state.game)
	if state.game.PendingTransition.TargetMap != "" {
		if err := applyAreaTransition(&state.game); err != nil {
			// Resolution failed (missing map file or door) — surface a clue and
			// leave the player where they were.
			state.game.LogMessage("Door failed: " + err.Error())
		} else {
			// Drop held-turn auto-repeat carry so a key held through the door
			// doesn't start the next area mid-cooldown.
			explore.ResetTurnRepeat(&state.game)
		}
		state.game.PendingTransition = core.AreaTransition{}
	}
	if state.game.Quit {
		state.game.Quit = false
		if state.testFromEditor {
			state.testFromEditor = false
			state.scene = sceneEditor
			// Drop lingering transient VFX: quitting a playtest mid-battle would
			// otherwise thaw stale particles/glyphs/bar-ghosts onto the next F5.
			render.ResetTransientVFX()
			return
		}
		returnToTitleScene(state)
	}
}

// returnToTitleScene resets to the title menu and rebuilds a fresh title.State.
// Both the adventure-quit and editor ExitToTitle paths land here.
func returnToTitleScene(state *appState) {
	state.scene = sceneTitle
	state.title = title.New()
	// Drop lingering transient VFX — the title doesn't draw the adventure
	// pipeline, so particles/glyphs/bar-ghosts would thaw onto the next
	// adventure entry at stale positions.
	render.ResetTransientVFX()
}

// applyAreaTransition loads the destination map and rebuilds the GameState so
// the player exits through the matching door. Party progression carries; only
// the world swaps. Battle/chest/modal state drops (would dangle into the old area).
//
// Fog-of-war: the destination's Visited is fresh and the source's discarded, so
// returning re-fogs (intentional per-area discovery).
func applyAreaTransition(g *core.GameState) error {
	target := g.PendingTransition.TargetMap
	doorName := g.PendingTransition.TargetDoor
	if target == "" && doorName == "" {
		// Both empty = nothing queued (mirrors Door.HasTarget on the raw fields).
		return nil
	}
	if target == "" || doorName == "" {
		// Exactly one field set is a malformed/partial transition. The caller only
		// reaches here with TargetMap set, so this is a door missing its target door
		// name — surface it instead of silently no-op'ing and stranding the player.
		return fmt.Errorf("incomplete transition (map=%q door=%q)", target, doorName)
	}
	// Same-map portals don't reload from disk (the area already holds the door),
	// avoiding a Packs/Chests reset for an in-map teleport.
	currentID := core.MapIDFromPath(g.Area.Path)
	if target == currentID || target == mapfile.SelfMapToken {
		dest := core.DoorByName(g.Doors, doorName)
		if dest == nil {
			return errDoorNotFound(target, doorName)
		}
		x, z := doorExitTile(g.Area, g.Doors, *dest)
		g.Player = core.NewPlayer(x, z, dest.Facing)
		// Seat the standing level on the exit tile so a door onto a voxel map
		// lands on the ground, not level 0 (no-op on a heightfield).
		g.Player.Level = g.Area.GroundSpawnLevel(x, z)
		// Re-seed region presence at the door exit (NewGameState seeded the area's
		// start tile, not where this portal landed) so a door INTO a region fires its
		// enter trigger only on the next crossing, not on arrival.
		core.SeedLocationPresence(g)
		// Same-map keeps the GameState struct (no NewGameState rebuild), so every
		// transient overlay must be closed explicitly to match the cross-map branch's
		// fresh-state reset — otherwise a panel/menu/chest open at teleport time
		// would persist into the destination. CloseTransitionOverlays owns that set.
		core.CloseTransitionOverlays(g)
		core.RequestVFXReset(g)
		return nil
	}
	area, err := core.LoadArea(core.MapPath(target))
	if err != nil {
		return err
	}
	next := core.NewGameState(area)
	// Carry what belongs to the party, not the world (party, bag, gold, journal,
	// step count, weather, RNG, debug toggles). The set lives on
	// CarryProgressionFrom so this path and the struct can't drift.
	next.CarryProgressionFrom(g)
	dest := core.DoorByName(next.Doors, doorName)
	if dest == nil {
		return errDoorNotFound(target, doorName)
	}
	x, z := doorExitTile(next.Area, next.Doors, *dest)
	next.Player = core.NewPlayer(x, z, dest.Facing)
	next.Player.Level = next.Area.GroundSpawnLevel(x, z)
	*g = next
	// Re-seed at the door exit (NewGameState seeded next's start tile).
	core.SeedLocationPresence(g)
	// Drop lingering particles — VFX from a fight ending just before the door
	// step would otherwise drift through the new area's camera view.
	core.RequestVFXReset(g)
	return nil
}

// doorExitTile picks the tile the player lands on through a door: one step in
// the door's facing (so they don't re-trigger), else the door tile if blocked.
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
		state.editor.Close() // free the 3D-view render target if it was open
		returnToTitleScene(state)
	case editor.ActionTest:
		// Build a runtime GameState from the in-memory area (no disk). The
		// editor's State stays intact so quitting lands on the same map.
		// StepCount seeds from the previewed phase for that lighting.
		area := state.editor.Area()
		// Ctrl+F5 test-from-cursor: override StartTile to the grid cursor for
		// this run only; the authored area keeps its original StartTile.
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
	camera := render.Camera(game)
	// Retro-filter capture (Debug ▸ Retro Filters): when a filter is active the
	// environment renders into an off-screen texture EndRetroCapture blits
	// through the shader, so the WORLD crunches while sprites/HUD/weather stay
	// crisp. Skybox exemption ("Filter Skybox"): the sky is drawn CRISP up front
	// and the capture cleared TRANSPARENT, so the environment composites over it.
	// drawSky pairs the load-bearing backbuffer depth wipe (ClearBackground) with
	// the sky draw; the two arms below must not let them diverge (see AGENTS.md).
	drawSky := func() {
		rl.ClearBackground(render.SkyClearColor)
		render.DrawSkyBackground(assets, game)
	}
	skyCrisp := core.AnyRetroFilterActive(&game.RetroFilters) && !game.RetroFilterSky
	if skyCrisp {
		drawSky()
	}
	filtered := render.BeginRetroCapture(game)
	// Explicit clear is REQUIRED (bisect-confirmed): without it geometry flickers
	// behind stale depth fragments this build carries across frames. Color is
	// irrelevant (overdrawn) — the call is for the depth wipe. The crisp-sky arm
	// clears to TRANSPARENT so the sky survives the blit.
	if filtered && skyCrisp {
		rl.ClearBackground(rl.Blank)
	} else if !skyCrisp {
		drawSky()
	}
	// Sprite exemption ("Filter Sprites"): when exempt, billboards draw crisp on
	// top (DrawCrispSpritePass) so their baked FX shows through; else inline.
	exemptSprites := filtered && !game.RetroFilterSprites
	// In battle with a filter active, the FX layer (particles + hit-glyphs) routes
	// through the retro shader (DrawFilteredCombatFX) so the combat juice crunches
	// with the world instead of popping crisp — independent of the sprite exemption.
	battleFiltered := filtered && game.Battle.Active()
	// 3D SCENE pass — sky, geometry, chests, doors, and (unless exempt) sprites.
	rl.BeginMode3D(camera)
	render.DrawWorld(camera, game, assets)
	render.DrawChests(camera, game, assets)
	render.DrawDoors(camera, game, assets)
	render.DrawCrystals(camera, game, assets)
	if !exemptSprites {
		// In battle, paint the billboards OVER the environment (no clipping into walls
		// or trees): depth test off, and the draw funcs order each group back-to-front
		// so they still layer correctly among themselves. Exploration keeps depth test
		// so field packs are still occluded by geometry.
		inBattle := game.Battle.Active()
		if inBattle {
			rl.DisableDepthTest()
		}
		render.DrawEnemies(camera, game, assets)
		render.DrawPartySprites(camera, game, assets)
		// VFX in the 3D pass so particles depth-sort; after the party draw so
		// sparks paint over the sprite. TickAndDrawVFX drains VFXQueue (mutates g).
		// In battle+filtered the FX layer is deferred to DrawFilteredCombatFX below.
		if !battleFiltered {
			render.TickAndDrawVFX(camera, game, assets)
		}
		if inBattle {
			rl.EnableDepthTest()
		}
	}
	rl.EndMode3D()
	// Blit the FILTERED environment to the backbuffer — opaque normally,
	// alpha-composited over the crisp sky in the skybox-exempt arm.
	if filtered {
		render.EndRetroCapture(game, skyCrisp)
		// Crisp sprites over it, reusing the capture's depth so they still occlude.
		// The state-mutating VFX tick runs once/frame: here when exempt (and NOT
		// battle-filtered, where the FX pass owns it), inline above otherwise.
		if exemptSprites {
			render.DrawCrispSpritePass(camera, game, assets, !battleFiltered)
		}
		// Filtered combat FX (particles + hit-glyphs) over the sprites.
		if battleFiltered {
			render.DrawFilteredCombatFX(camera, game, assets)
		}
	}
	// Ambient rain — above the world, below popups/HUD. No-op when clear.
	render.DrawWeather(game)
	// Danger vignette — claret edges while an enemy is mid-swing. Weather-wash
	// layer; no-op outside that phase.
	render.DrawBattleDangerVignette(game)
	render.DrawChestPrompt(camera, game, assets)
	render.DrawCrystalPrompt(camera, game, assets)
	// Hit-glyph shapes over struck targets — before the damage popups so the
	// number floats on top of the glyph. In battle+filtered they're drawn (filtered)
	// inside DrawFilteredCombatFX above, so skip the crisp draw here.
	if !battleFiltered {
		render.DrawHitGlyphs(camera, game, assets)
	}
	render.DrawDamagePopups(camera, game, assets)
	render.DrawQualityPopup(camera, game, assets)
	render.DrawDebugOverlay(camera, game, assets)
	// DrawOverlay paints the HUD AND any open top-level menu, cross-fading
	// between them (the panels overlay rides this fade path, not a separate call).
	render.DrawOverlay(game, assets)
	render.DrawChestModal(game, assets)
	render.DrawLevelUpModal(game, assets)
	render.DrawDoorPrompt(game, assets)
	// Dialog over every other explore modal (highest-priority — see ModalDialog).
	render.DrawDialogModal(game, assets)
	// Quit confirm (ModalQuitConfirm) over everything, even a dialog.
	render.DrawQuitConfirm(game, assets)
}

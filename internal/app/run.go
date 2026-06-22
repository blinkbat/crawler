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
	// testFromEditor: set when the editor's F5 path enters adventure with the
	// in-memory area, so quitting returns to the editor, not the title.
	testFromEditor bool
	quit           bool
}

func Run() {
	// FlagWindowHighdpi stays off: on Windows 1.5× it drifts HUD coords because
	// GetScreenWidth is logical while the framebuffer is scaled. Re-enable only
	// after the layout code consistently uses one of GetScreenWidth/GetRenderWidth.
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

		// Global Alt+Enter display toggle, scene-independent and BEFORE the
		// scene update. Enter-based confirms ignore Enter-with-Alt so this
		// press never doubles as a commit.
		if input.DisplayTogglePressed() {
			render.ToggleDisplayMode()
		}

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

		// Drive rumble every frame: TickRumble decays the combat envelope (0
		// outside battle) so a rumble armed just before a battle ends eases off.
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
			// Resolution failed (missing map file or door) — surface a clue and
			// leave the player where they were.
			state.game.LogMessage("Door failed: " + err.Error())
		} else {
			// Drop held-turn auto-repeat carry so a key held through the door
			// doesn't start the next area mid-cooldown.
			explore.ResetTurnRepeat()
		}
		state.game.PendingTransition = core.AreaTransition{}
	}
	if state.game.Quit {
		state.game.Quit = false
		if state.testFromEditor {
			state.testFromEditor = false
			state.scene = sceneEditor
			// Drop lingering particles: quitting a playtest mid-battle would
			// otherwise thaw stale formation-relative particles onto the next F5.
			render.ResetParticles()
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
	// Drop lingering particles — the title doesn't draw the adventure pipeline,
	// so the pool would thaw onto the next adventure entry at stale positions.
	render.ResetParticles()
}

// applyAreaTransition loads the destination map and rebuilds the GameState so
// the player exits through the matching door. Party progression carries; only
// the world (area, packs, chests, doors) swaps. Battle/chest/modal state is
// dropped (those flags would dangle pointers into the old area).
//
// Fog-of-war: the destination's Visited is allocated fresh and the source's is
// discarded, so returning to a map re-fogs it (intentional per-area discovery).
func applyAreaTransition(g *core.GameState) error {
	target := g.PendingTransition.TargetMap
	doorName := g.PendingTransition.TargetDoor
	if target == "" || doorName == "" {
		// Mirrors Door.HasTarget on the raw PendingTransition fields (the
		// transition is queued before resolving to a Door). Empty = none queued.
		return nil
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
		// Symmetry with the cross-map branch: close any equip picker and clear
		// VFX so a (latent today) teleport from an open panel stays honest.
		core.CloseEquipPicker(g)
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
	// Drop lingering particles — VFX from a fight ending just before the door
	// step would otherwise drift through the new area's camera view.
	core.RequestVFXReset(g)
	return nil
}

// doorExitTile picks the tile the player lands on through a door. Prefer one
// step in the door's facing (so they don't immediately re-trigger); fall back
// to the door tile if that's blocked or holds another door.
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
	// environment pass renders into an off-screen texture that EndRetroCapture
	// blits through the filter shader, so the WORLD crunches while sprites/HUD/
	// popups/weather stay crisp. Filters off = a single bool check, drawn direct.
	//
	// Skybox exemption ("Filter Skybox"): when exempt, the sky is drawn CRISP to
	// the backbuffer up front and the capture is cleared TRANSPARENT, so the
	// filtered environment alpha-composites over the clean sky at blit time.
	skyCrisp := core.AnyRetroFilterActive(&game.RetroFilters) && !game.RetroFilterSky
	if skyCrisp {
		// Doubles as the backbuffer depth wipe for the crisp-sky arm.
		rl.ClearBackground(render.SkyClearColor)
		render.DrawSkyBackground(assets, game)
	}
	filtered := render.BeginRetroCapture(game)
	// Explicit clear is REQUIRED (bisect-confirmed): without it world geometry
	// flickers behind stale depth-buffer fragments this build carries across
	// frames despite BeginDrawing's implicit clear. The color is irrelevant
	// (DrawSkyBackground overdraws it) — the call is for the depth wipe.
	// Inside a capture it lands on the capture's own buffers; the crisp-sky arm
	// clears to TRANSPARENT so the sky survives the blit.
	if filtered && skyCrisp {
		rl.ClearBackground(rl.Blank)
	} else if !skyCrisp {
		rl.ClearBackground(render.SkyClearColor)
		render.DrawSkyBackground(assets, game)
	}
	// Sprite exemption (the menu's "Filter Sprites" toggle): when filters are
	// active and sprites are exempt, the enemy/party billboards are held OUT of
	// the captured environment pass and drawn crisp on top afterward (see
	// DrawCrispSpritePass) so the per-asset visuals.json FX baked into each
	// sprite shows through without the screen filter stacking over it. When
	// sprites are NOT exempt (or no filter is active), they draw inline here and
	// crunch with the world.
	exemptSprites := filtered && !game.RetroFilterSprites
	// 3D SCENE pass — sky, world geometry, chests, doors, and (unless exempt) the
	// sprites in one pass. Drawing the sprites inside the capture means a retro
	// filter crunches the WHOLE world uniformly; exempting them keeps them crisp.
	// Per-foe look comes from the editor's visuals.json FX baked into each sprite
	// texture either way. HUD, popups, weather, and menus draw later in screen
	// space and stay crisp.
	rl.BeginMode3D(camera)
	render.DrawWorld(camera, game, assets)
	render.DrawChests(camera, game, assets)
	render.DrawDoors(camera, game, assets)
	render.DrawCrystals(camera, game, assets)
	if !exemptSprites {
		render.DrawEnemies(camera, game, assets)
		render.DrawPartySprites(camera, game, assets)
		// VFX inside the 3D pass so billboard particles depth-sort with the rest of
		// the scene. TickAndDrawVFX drains GameState.VFXQueue (mutating g), advances
		// the render-side pool by raylib's frame dt, and emits draws for every live
		// particle. Kept after the party draw so impact sparks paint over the sprite,
		// not under.
		render.TickAndDrawVFX(camera, game, assets)
	}
	rl.EndMode3D()
	// Close the retro capture and blit the FILTERED environment to the backbuffer
	// — opaquely in the normal arm, alpha-composited over the crisp sky in the
	// skybox-exempt arm.
	if filtered {
		render.EndRetroCapture(game, skyCrisp)
		// Then lay the crisp, unfiltered sprites over the filtered environment,
		// re-using the capture's depth so they still occlude behind walls. The
		// VFX tick (state-mutating) runs exactly once per frame — here in the
		// exempt arm, inline above otherwise.
		if exemptSprites {
			render.DrawCrispSpritePass(camera, game, assets)
		}
	}
	// Ambient rain sits above the 3D world (darkening it) but below the
	// world-space popups and HUD, so combat numbers, prompts, and menus
	// stay readable through the storm. No-op when the weather is clear.
	render.DrawWeather(game)
	// Danger vignette — claret edges breathing while an enemy is mid-swing
	// (defend timing). Same layer slot as the weather wash: over the world,
	// under every popup and HUD pane. No-op outside that phase.
	render.DrawBattleDangerVignette(game)
	render.DrawChestPrompt(camera, game, assets)
	render.DrawCrystalPrompt(camera, game, assets)
	// Hit-glyph clarity shapes over struck targets — HUD pass (crisp 2D), but
	// before the damage popups so the number floats on top of the glyph.
	render.DrawHitGlyphs(camera, game, assets)
	render.DrawDamagePopups(camera, game, assets)
	render.DrawQualityPopup(camera, game, assets)
	render.DrawDebugOverlay(camera, game, assets)
	// DrawOverlay paints the HUD AND any open top-level menu (incl. the Tome /
	// character panels), cross-fading between them — so the panels overlay is no
	// longer a separate call here; it's drawn through DrawOverlay's fade path.
	render.DrawOverlay(game, assets)
	render.DrawChestModal(game, assets)
	render.DrawLevelUpModal(game, assets)
	render.DrawDoorPrompt(game, assets)
	// Dialog sits last so the conversation overlay paints on top of every
	// other explore modal (it's the highest-priority modal — see
	// core.ActiveModal / ModalDialog).
	render.DrawDialogModal(game, assets)
	// Quit confirm is the top-priority modal (ModalQuitConfirm) — paint it over
	// everything, including a dialog, so a pending quit decision is never hidden.
	render.DrawQuitConfirm(game, assets)
}

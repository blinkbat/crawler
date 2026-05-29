package app

import (
	"crawler/internal/app/audio"
	"crawler/internal/app/core"
	"crawler/internal/app/core/mapfile"
	"crawler/internal/app/editor"
	"crawler/internal/app/explore"
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

	state := appState{scene: sceneTitle, title: title.New()}

	for !rl.WindowShouldClose() && !state.quit {
		dt := rl.GetFrameTime()

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
			state.game.Battle.Message = "Door failed: " + err.Error()
		}
		state.game.PendingTransition = core.AreaTransition{}
	}
	if state.game.Quit {
		state.game.Quit = false
		if state.testFromEditor {
			state.testFromEditor = false
			state.scene = sceneEditor
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
// party (HP/MP/levels/status), and StepCount are preserved across the
// transition — only the world (area, packs, chests, doors) is
// swapped. Battle / chest / modal state is dropped, since any of
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
	next.StepCount = g.StepCount
	next.RNG = g.RNG
	next.DebugOverlay = g.DebugOverlay
	next.EnemiesDisabled = g.EnemiesDisabled
	next.EasyBattleQuit = g.EasyBattleQuit
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
	// No explicit clear — DrawSkyBackground paints the full viewport
	// every frame, so any clear here would be immediately overdrawn.
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
	render.TickAndDrawVFX(camera, game)
	rl.EndMode3D()
	render.DrawChestPrompt(camera, *game, assets)
	render.DrawDamagePopups(camera, *game, assets)
	render.DrawQualityPopup(camera, *game, assets)
	render.DrawDebugOverlay(camera, *game, assets)
	render.DrawOverlay(*game, assets)
	render.DrawChestModal(*game, assets)
	render.DrawLevelUpModal(*game, assets)
	render.DrawPanelsOverlay(*game, assets)
	render.DrawDoorPrompt(*game, assets)
}

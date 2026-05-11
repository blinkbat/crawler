package app

import (
	"crawler/internal/app/core"
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
	rl.SetConfigFlags(rl.FlagVsyncHint | rl.FlagWindowResizable)
	rl.InitWindow(core.ScreenWidth, core.ScreenHeight, "Crawler")
	defer rl.CloseWindow()

	rl.SetExitKey(rl.KeyNull)
	applyWindowedFullscreen()
	rl.SetTargetFPS(120)

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
		}

		rl.BeginDrawing()
		switch state.scene {
		case sceneTitle:
			title.Draw(state.title, assets)
		case sceneAdventure:
			drawAdventureScene(state.game, assets)
		case sceneEditor:
			editor.Draw(&state.editor, assets)
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
	if state.game.Quit {
		state.game.Quit = false
		if state.testFromEditor {
			state.testFromEditor = false
			state.scene = sceneEditor
			return
		}
		state.scene = sceneTitle
		state.title = title.New()
	}
}

func updateEditorScene(state *appState, dt float32) {
	switch editor.Update(&state.editor, dt) {
	case editor.ActionExitToTitle:
		state.scene = sceneTitle
		state.title = title.New()
	case editor.ActionTest:
		// Build a runtime GameState from the in-memory area without
		// touching disk. The editor's State stays intact so we land back
		// on the same map (and the same dirty-marker) when the player
		// quits the playtest. StepCount is seeded from the editor's
		// previewed phase so the playtest opens in that lighting.
		area := state.editor.Area()
		state.game = core.NewGameState(area)
		state.game.StepCount = state.editor.PreviewStepCount()
		state.scene = sceneAdventure
		state.testFromEditor = true
	}
}

func drawAdventureScene(game core.GameState, assets render.Resources) {
	camera := render.Camera(game.Player)
	rl.ClearBackground(rl.NewColor(87, 172, 244, 255))
	render.DrawSkyBackground(assets, game)
	rl.BeginMode3D(camera)
	render.DrawWorld(camera, game, assets)
	render.DrawEnemies(camera, game, assets)
	render.DrawPartySprites(camera, game, assets)
	rl.EndMode3D()
	render.DrawDamagePopups(camera, game, assets)
	render.DrawQualityPopup(camera, game, assets)
	render.DrawOverlay(game, assets)
}

func applyWindowedFullscreen() {
	monitor := rl.GetCurrentMonitor()
	position := rl.GetMonitorPosition(monitor)
	width := rl.GetMonitorWidth(monitor)
	height := rl.GetMonitorHeight(monitor)
	if width <= 0 || height <= 0 {
		return
	}
	rl.SetWindowSize(width, height)
	rl.SetWindowPosition(int(position.X), int(position.Y))
	rl.SetWindowState(rl.FlagBorderlessWindowedMode)
}

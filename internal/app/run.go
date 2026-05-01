package app

import (
	"crawler/internal/app/core"
	"crawler/internal/app/explore"
	"crawler/internal/app/render"

	rl "github.com/gen2brain/raylib-go/raylib"
)

func Run() {
	rl.SetConfigFlags(rl.FlagVsyncHint | rl.FlagWindowResizable)
	rl.InitWindow(core.ScreenWidth, core.ScreenHeight, "Crawler")
	defer rl.CloseWindow()

	rl.SetExitKey(rl.KeyNull)
	applyWindowedFullscreen()
	rl.SetTargetFPS(120)

	state := core.NewGameState(core.DefaultArea())
	assets := render.LoadResources()
	defer assets.Unload()

	for !rl.WindowShouldClose() && !state.Quit {
		explore.Update(&state)
		camera := render.Camera(state.Player)

		rl.BeginDrawing()
		rl.ClearBackground(rl.NewColor(87, 172, 244, 255))
		render.DrawSkyBackground(assets)
		rl.BeginMode3D(camera)
		render.DrawWorld(state.Map, assets)
		render.DrawEnemies(camera, state, assets)
		render.DrawPartySprites(camera, state, assets)
		rl.EndMode3D()
		render.DrawOverlay(state, assets)
		render.DrawBattlePartyLabels(camera, state, assets)
		rl.EndDrawing()
	}
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

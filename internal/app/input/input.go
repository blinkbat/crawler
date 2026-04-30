package input

import rl "github.com/gen2brain/raylib-go/raylib"

func ConfirmPressed() bool {
	return rl.IsKeyPressed(rl.KeyEnter) || rl.IsKeyPressed(rl.KeySpace) || rl.IsKeyPressed(rl.KeyZ)
}

func BackPressed() bool {
	return rl.IsKeyPressed(rl.KeyEscape) || rl.IsKeyPressed(rl.KeyX)
}

func UpPressed() bool {
	return rl.IsKeyPressed(rl.KeyUp) || rl.IsKeyPressed(rl.KeyW)
}

func DownPressed() bool {
	return rl.IsKeyPressed(rl.KeyDown) || rl.IsKeyPressed(rl.KeyS)
}

func TargetNextPressed() bool {
	return rl.IsKeyPressed(rl.KeyTab) || rl.IsKeyPressed(rl.KeyRight) || rl.IsKeyPressed(rl.KeyD) || rl.IsKeyPressed(rl.KeyDown)
}

func TargetPreviousPressed() bool {
	return rl.IsKeyPressed(rl.KeyLeft) || rl.IsKeyPressed(rl.KeyA) || rl.IsKeyPressed(rl.KeyUp)
}

func PausePressed() bool {
	return rl.IsKeyPressed(rl.KeyEscape)
}

func RestartPressed() bool {
	return rl.IsKeyPressed(rl.KeyR)
}

func QuitPressed() bool {
	return rl.IsKeyPressed(rl.KeyQ)
}

func TurnLeftPressed() bool {
	return rl.IsKeyPressed(rl.KeyLeft) || rl.IsKeyPressed(rl.KeyQ)
}

func TurnRightPressed() bool {
	return rl.IsKeyPressed(rl.KeyRight) || rl.IsKeyPressed(rl.KeyE)
}

func StepForwardPressed() bool {
	return rl.IsKeyPressed(rl.KeyW) || rl.IsKeyPressed(rl.KeyUp)
}

func StepBackPressed() bool {
	return rl.IsKeyPressed(rl.KeyS) || rl.IsKeyPressed(rl.KeyDown)
}

func StrafeLeftPressed() bool {
	return rl.IsKeyPressed(rl.KeyA)
}

func StrafeRightPressed() bool {
	return rl.IsKeyPressed(rl.KeyD)
}

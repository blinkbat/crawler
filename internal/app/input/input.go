package input

import rl "github.com/gen2brain/raylib-go/raylib"

// gamepadID is the controller slot we read. Raylib supports up to 4; we only
// care about player one for now.
const gamepadID = int32(0)

// Stick edge-detection threshold. The stick has to push past this magnitude
// to register a "press"; it must return below it (centered) before the next
// press can fire. 0.55 is conservative enough that small drift / dead-zone
// noise from worn sticks doesn't generate phantom presses.
const stickEdgeThreshold = float32(0.55)

// Per-direction "was past threshold last sample?" memory for analog-stick
// edge detection. Updated by the stick* helpers each time an Arrow* / target-
// cycle / nav input is queried.
var (
	prevStickUp    bool
	prevStickDown  bool
	prevStickLeft  bool
	prevStickRight bool
)

// gamepadConnected reports whether a gamepad is plugged in. All controller
// reads short-circuit to false when there's no pad — keyboard-only play
// still works, and we don't pay the cost of probing axes that aren't there.
func gamepadConnected() bool {
	return rl.IsGamepadAvailable(gamepadID)
}

// padPressed is a thin alias for "edge press of a gamepad button if a pad
// is connected." Saves the boilerplate at every call site.
func padPressed(button int32) bool {
	return gamepadConnected() && rl.IsGamepadButtonPressed(gamepadID, button)
}

// padDown reports whether a gamepad button is held this frame.
func padDown(button int32) bool {
	return gamepadConnected() && rl.IsGamepadButtonDown(gamepadID, button)
}

// padReleased reports whether a gamepad button was just released.
func padReleased(button int32) bool {
	return gamepadConnected() && rl.IsGamepadButtonReleased(gamepadID, button)
}

// stickEdgeY returns true on the frame the left stick crosses past the
// up/down threshold. dir = -1 for up, +1 for down. Updates prev state so the
// stick must return below threshold before the next edge can fire.
func stickEdgeY(dir int) bool {
	if !gamepadConnected() {
		return false
	}
	v := rl.GetGamepadAxisMovement(gamepadID, rl.GamepadAxisLeftY)
	if dir < 0 {
		nowUp := v <= -stickEdgeThreshold
		edge := nowUp && !prevStickUp
		prevStickUp = nowUp
		return edge
	}
	nowDown := v >= stickEdgeThreshold
	edge := nowDown && !prevStickDown
	prevStickDown = nowDown
	return edge
}

// stickEdgeX is the horizontal counterpart. dir = -1 for left, +1 for right.
func stickEdgeX(dir int) bool {
	if !gamepadConnected() {
		return false
	}
	v := rl.GetGamepadAxisMovement(gamepadID, rl.GamepadAxisLeftX)
	if dir < 0 {
		nowLeft := v <= -stickEdgeThreshold
		edge := nowLeft && !prevStickLeft
		prevStickLeft = nowLeft
		return edge
	}
	nowRight := v >= stickEdgeThreshold
	edge := nowRight && !prevStickRight
	prevStickRight = nowRight
	return edge
}

// --- High-level semantic actions ---------------------------------------------
// Every action accepts both keyboard and controller. Keep call sites using
// these names so future remapping touches one file.

func ConfirmPressed() bool {
	return rl.IsKeyPressed(rl.KeyEnter) || rl.IsKeyPressed(rl.KeySpace) || rl.IsKeyPressed(rl.KeyZ) ||
		padPressed(rl.GamepadButtonRightFaceDown) // A on Xbox / Cross on PS
}

func BackPressed() bool {
	return rl.IsKeyPressed(rl.KeyEscape) || rl.IsKeyPressed(rl.KeyX) ||
		padPressed(rl.GamepadButtonRightFaceRight) // B / Circle
}

func UpPressed() bool {
	return rl.IsKeyPressed(rl.KeyUp) || rl.IsKeyPressed(rl.KeyW) ||
		padPressed(rl.GamepadButtonLeftFaceUp) || stickEdgeY(-1)
}

func DownPressed() bool {
	return rl.IsKeyPressed(rl.KeyDown) || rl.IsKeyPressed(rl.KeyS) ||
		padPressed(rl.GamepadButtonLeftFaceDown) || stickEdgeY(1)
}

func TargetNextPressed() bool {
	return rl.IsKeyPressed(rl.KeyTab) || rl.IsKeyPressed(rl.KeyRight) || rl.IsKeyPressed(rl.KeyD) || rl.IsKeyPressed(rl.KeyDown) ||
		padPressed(rl.GamepadButtonLeftFaceRight) || padPressed(rl.GamepadButtonRightTrigger1) || stickEdgeX(1)
}

func TargetPreviousPressed() bool {
	return rl.IsKeyPressed(rl.KeyLeft) || rl.IsKeyPressed(rl.KeyA) || rl.IsKeyPressed(rl.KeyUp) ||
		padPressed(rl.GamepadButtonLeftFaceLeft) || padPressed(rl.GamepadButtonLeftTrigger1) || stickEdgeX(-1)
}

// PausePressed opens / closes the pause menu. Deliberately separate from
// BackPressed (which is also Esc): if both shared Esc, then in a battle the
// universal "back / cancel target" press would silently get eaten as Pause
// before the battle code saw it. P is the pause key here; the gamepad Start
// button works on a controller.
func PausePressed() bool {
	return rl.IsKeyPressed(rl.KeyP) || padPressed(rl.GamepadButtonMiddleRight) // Start
}

func RestartPressed() bool {
	return rl.IsKeyPressed(rl.KeyR) || padPressed(rl.GamepadButtonRightFaceUp) // Y / Triangle
}

func QuitPressed() bool {
	return rl.IsKeyPressed(rl.KeyQ) || padPressed(rl.GamepadButtonMiddleLeft) // Select / Share
}

func TurnLeftPressed() bool {
	return rl.IsKeyPressed(rl.KeyLeft) || rl.IsKeyPressed(rl.KeyQ) ||
		padPressed(rl.GamepadButtonLeftTrigger1) // LB
}

func TurnRightPressed() bool {
	return rl.IsKeyPressed(rl.KeyRight) || rl.IsKeyPressed(rl.KeyE) ||
		padPressed(rl.GamepadButtonRightTrigger1) // RB
}

func StepForwardPressed() bool {
	return rl.IsKeyPressed(rl.KeyW) || rl.IsKeyPressed(rl.KeyUp) ||
		padPressed(rl.GamepadButtonLeftFaceUp) || stickEdgeY(-1)
}

func StepBackPressed() bool {
	return rl.IsKeyPressed(rl.KeyS) || rl.IsKeyPressed(rl.KeyDown) ||
		padPressed(rl.GamepadButtonLeftFaceDown) || stickEdgeY(1)
}

func StrafeLeftPressed() bool {
	return rl.IsKeyPressed(rl.KeyA) || stickEdgeX(-1)
}

func StrafeRightPressed() bool {
	return rl.IsKeyPressed(rl.KeyD) || stickEdgeX(1)
}

// ConfirmDown reports whether any "confirm" key is currently held this frame.
// Counterpart to ConfirmPressed (which is the down-edge); used by hold-mode
// minigames where we need to know the button stays pressed.
func ConfirmDown() bool {
	return rl.IsKeyDown(rl.KeyEnter) || rl.IsKeyDown(rl.KeySpace) || rl.IsKeyDown(rl.KeyZ) ||
		padDown(rl.GamepadButtonRightFaceDown)
}

// ConfirmReleased reports whether a "confirm" key was just released this
// frame (up-edge). Used by hold-mode minigames to detect "release now."
func ConfirmReleased() bool {
	return rl.IsKeyReleased(rl.KeyEnter) || rl.IsKeyReleased(rl.KeySpace) || rl.IsKeyReleased(rl.KeyZ) ||
		padReleased(rl.GamepadButtonRightFaceDown)
}

// AttackTimingPressed reports whether the player hit the "attack" button for a
// timed-hit minigame.
func AttackTimingPressed() bool {
	return ConfirmPressed()
}

// AttackTimingHeld is the held-state counterpart used by charge minigames.
func AttackTimingHeld() bool {
	return ConfirmDown()
}

// AttackTimingReleased is the up-edge counterpart used by charge minigames.
func AttackTimingReleased() bool {
	return ConfirmReleased()
}

// DefendTimingPressed reports whether the player hit the "defend" button for
// a blocked-hit minigame.
func DefendTimingPressed() bool {
	return BackPressed()
}

// Directional one-frame edges for the pickpocket sequence minigame. Limited
// to inputs that visually map to arrow symbols: arrow keys, controller
// D-pad, and the left analog stick. WASD is intentionally NOT accepted —
// the prompt shows literal arrows so the input set should match.
func ArrowUpPressed() bool {
	return rl.IsKeyPressed(rl.KeyUp) ||
		padPressed(rl.GamepadButtonLeftFaceUp) || stickEdgeY(-1)
}

func ArrowDownPressed() bool {
	return rl.IsKeyPressed(rl.KeyDown) ||
		padPressed(rl.GamepadButtonLeftFaceDown) || stickEdgeY(1)
}

func ArrowLeftPressed() bool {
	return rl.IsKeyPressed(rl.KeyLeft) ||
		padPressed(rl.GamepadButtonLeftFaceLeft) || stickEdgeX(-1)
}

func ArrowRightPressed() bool {
	return rl.IsKeyPressed(rl.KeyRight) ||
		padPressed(rl.GamepadButtonLeftFaceRight) || stickEdgeX(1)
}

// ResetStickEdges seeds the analog stick edge memory from the *current*
// stick state. Call this when entering a new input context (e.g. arming the
// sequence minigame): if the stick happens to be tilted past threshold at
// that moment, we record it as already-active so the next stickEdge* call
// won't fire a phantom press on frame 1. The player has to center the stick
// and re-tilt to register a fresh edge.
func ResetStickEdges() {
	if !gamepadConnected() {
		prevStickUp = false
		prevStickDown = false
		prevStickLeft = false
		prevStickRight = false
		return
	}
	yv := rl.GetGamepadAxisMovement(gamepadID, rl.GamepadAxisLeftY)
	xv := rl.GetGamepadAxisMovement(gamepadID, rl.GamepadAxisLeftX)
	prevStickUp = yv <= -stickEdgeThreshold
	prevStickDown = yv >= stickEdgeThreshold
	prevStickLeft = xv <= -stickEdgeThreshold
	prevStickRight = xv >= stickEdgeThreshold
}

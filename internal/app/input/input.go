package input

import (
	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// gamepadID is the controller slot we read. Raylib supports up to 4; we only
// care about player one for now.
const gamepadID = int32(0)

// Stick edge-detection threshold. The stick has to push past this magnitude
// to register a "press"; it must return below it (centered) before the next
// press can fire. 0.55 is conservative enough that small drift / dead-zone
// noise from worn sticks doesn't generate phantom presses.
const stickEdgeThreshold = float32(0.55)

// stickEdgeKey identifies one of the four analog-stick directions for
// the edge-detection memory.
type stickEdgeKey int

const (
	stickEdgeUp stickEdgeKey = iota
	stickEdgeDown
	stickEdgeLeft
	stickEdgeRight
)

// prevStickEdges holds the "was past threshold last sample?" bit for
// each direction. Replaces the four parallel globals (prevStickUp /
// Down / Left / Right) that earlier passes carried — same shape, one
// table. stickEdgeY / stickEdgeX index into this by direction so a
// future fifth axis (right stick? trigger?) lands as one new key
// instead of two parallel globals + a new helper.
var prevStickEdges [4]bool

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

// stickAxisEdge returns true on the frame the analog axis crosses past
// the threshold in `dir` (`-1` = negative direction, `+1` = positive).
// Updates prev state so the stick must return below threshold before
// the next edge can fire. Generic over the per-direction memory slot
// so stickEdgeY (left stick Y) and stickEdgeX (left stick X) share
// one body instead of inlining four near-identical branches.
func stickAxisEdge(axis int32, dir int, negKey, posKey stickEdgeKey) bool {
	if !gamepadConnected() {
		return false
	}
	v := rl.GetGamepadAxisMovement(gamepadID, axis)
	if dir < 0 {
		now := v <= -stickEdgeThreshold
		edge := now && !prevStickEdges[negKey]
		prevStickEdges[negKey] = now
		return edge
	}
	now := v >= stickEdgeThreshold
	edge := now && !prevStickEdges[posKey]
	prevStickEdges[posKey] = now
	return edge
}

// stickEdgeY / stickEdgeX are thin wrappers naming the left-stick axes.
// dir = -1 for up/left, +1 for down/right.
func stickEdgeY(dir int) bool {
	return stickAxisEdge(rl.GamepadAxisLeftY, dir, stickEdgeUp, stickEdgeDown)
}

func stickEdgeX(dir int) bool {
	return stickAxisEdge(rl.GamepadAxisLeftX, dir, stickEdgeLeft, stickEdgeRight)
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

// CursorUpDown applies UpPressed / DownPressed to a wrap-around cursor
// inside [0, count). One-call replacement for the
//
//	if UpPressed()   { cursor = WrapIndex(cursor-1, n) }
//	if DownPressed() { cursor = WrapIndex(cursor+1, n) }
//
// pattern that was repeated in title menus, pause-menu pickers, sound
// modal columns, and editor pack/open modals. Safe for count <= 0
// (returns cursor unchanged so callers don't need to guard).
func CursorUpDown(cursor, count int) int {
	if count <= 0 {
		return cursor
	}
	if UpPressed() {
		cursor = core.WrapIndex(cursor-1, count)
	}
	if DownPressed() {
		cursor = core.WrapIndex(cursor+1, count)
	}
	return cursor
}

// ModalClosePressed is the edge-detect predicate for closing an editor
// modal. Esc and Enter both dismiss — Esc is "cancel", Enter is "I'm
// done editing" — and the two key handlers had drifted apart across
// modal updaters until this consolidation. Mouse-only close paths still
// go through their own button hit-tests; this is the keyboard rule.
func ModalClosePressed() bool {
	return rl.IsKeyPressed(rl.KeyEscape) || rl.IsKeyPressed(rl.KeyEnter)
}

// CursorLeftRight returns -1 on a Left edge, +1 on a Right edge, 0
// otherwise. Mirrors CursorUpDown's shape so sliders and cue-cycle
// pickers can share one Left/Right dispatch instead of inlining the
// keyboard probe twice. Edge-only (IsKeyPressed) so a held key doesn't
// stream values; callers do the value-adjust math.
func CursorLeftRight() int {
	left := rl.IsKeyPressed(rl.KeyLeft) || rl.IsKeyPressed(rl.KeyA) ||
		padPressed(rl.GamepadButtonLeftFaceLeft) || stickEdgeX(-1)
	right := rl.IsKeyPressed(rl.KeyRight) || rl.IsKeyPressed(rl.KeyD) ||
		padPressed(rl.GamepadButtonLeftFaceRight) || stickEdgeX(1)
	switch {
	case right && !left:
		return 1
	case left && !right:
		return -1
	}
	return 0
}

func TargetNextPressed() bool {
	return rl.IsKeyPressed(rl.KeyTab) || rl.IsKeyPressed(rl.KeyRight) || rl.IsKeyPressed(rl.KeyD) || rl.IsKeyPressed(rl.KeyDown) ||
		padPressed(rl.GamepadButtonLeftFaceRight) || padPressed(rl.GamepadButtonRightTrigger1) || stickEdgeX(1)
}

// NextTabPressed reports a pure Tab-key edge — used by the action
// menu's "cycle the Skill row's active skill" handler. Kept distinct
// from TargetNextPressed so the cycle key doesn't double up with the
// arrow-key + analog-stick navigation that's already wired to the
// up/down row cursor.
func NextTabPressed() bool {
	return rl.IsKeyPressed(rl.KeyTab) || padPressed(rl.GamepadButtonRightFaceRight)
}

func TargetPreviousPressed() bool {
	return rl.IsKeyPressed(rl.KeyLeft) || rl.IsKeyPressed(rl.KeyA) || rl.IsKeyPressed(rl.KeyUp) ||
		padPressed(rl.GamepadButtonLeftFaceLeft) || padPressed(rl.GamepadButtonLeftTrigger1) || stickEdgeX(-1)
}

// PausePressed reports the pause-menu edge. P and gamepad Start always
// open the menu; Esc also opens it but only when inBattle is false — in
// battle Esc is the "back / cancel target" edge and we must not eat it
// here. The caller threads its own context (battle or no battle) so this
// stays one function instead of two slightly-different probes.
//
// Controller mapping note: GamepadButtonMiddleRight is the "small
// start" button — Options on PS5 / Start on Xbox / Menu on Switch.
// The "big start" middle button (PS5 touchpad click / Xbox guide /
// closest raylib exposes is GamepadButtonMiddle) is bound by
// PanelsTogglePressed below for the game panels overlay so the two
// reads don't fight over the same input.
func PausePressed(inBattle bool) bool {
	if rl.IsKeyPressed(rl.KeyP) || padPressed(rl.GamepadButtonMiddleRight) { // Start
		return true
	}
	if !inBattle && rl.IsKeyPressed(rl.KeyEscape) {
		return true
	}
	return false
}

// PanelsTogglePressed is the edge to open or close the game panels
// overlay (stats / equipment / items / skills / zoomable map). Bound
// to the gamepad's Triangle / Y button (the JRPG-standard "menu"
// face button) plus the middle button — on a DualSense the latter
// is the PS button, on Xbox it's the guide button, the closest
// raylib exposes to the PS5 touchpad click. Triangle is the headline
// binding; the middle-button binding is retained for muscle memory.
// The "small start" Options button stays mapped to PausePressed; the
// two overlays are mutually exclusive in the explore-loop gate.
//
// Both opening and closing go through this same edge — pressing the
// same button toggles the overlay off, mirroring how phone status
// bars work. BackPressed (Esc / B / Circle) also closes the overlay
// from inside it.
//
// Keyboard does NOT toggle this way — the per-tab shortcuts
// (PanelTabShortcutPressed) own E/C/K/M/I, so the keyboard player
// jumps directly to a named tab rather than opening "wherever I
// was last." Pressing the same shortcut key again closes the
// overlay (handled in explore/panels.go), preserving the toggle
// feel without burning a sixth key on "open to last tab."
func PanelsTogglePressed() bool {
	return padPressed(rl.GamepadButtonMiddle) ||
		padPressed(rl.GamepadButtonRightFaceUp) // Triangle / Y
}

// PanelTabShortcutPressed reports a per-tab keyboard shortcut for the
// game panels overlay. Returns the target tab and true on a fresh
// edge, or zero+false when no shortcut fired this frame. The letters
// match the on-screen mnemonics:
//
//	C → Stats     (Character)
//	E → Equipment
//	I → Items     (Inventory)
//	K → Skills    (note the K in "skills")
//	M → Map
//
// Used by both the explore-loop "open panels to this tab" path and
// the in-panels "switch to this tab (or close if already on it)"
// path, so the same key behaves consistently across open / closed
// states.
func PanelTabShortcutPressed() (core.PanelTab, bool) {
	switch {
	case rl.IsKeyPressed(rl.KeyC):
		return core.PanelTabStats, true
	case rl.IsKeyPressed(rl.KeyE):
		return core.PanelTabEquipment, true
	case rl.IsKeyPressed(rl.KeyI):
		return core.PanelTabItems, true
	case rl.IsKeyPressed(rl.KeyK):
		return core.PanelTabSkills, true
	case rl.IsKeyPressed(rl.KeyM):
		return core.PanelTabMap, true
	}
	return 0, false
}

// RestartPressed reports the pause-menu "restart run" edge. Triangle/Y
// used to be bound here too, but Triangle was reassigned to the game
// panels overlay (see PanelsTogglePressed) so the menu-restart key is
// now keyboard R only — the pause menu still exposes Restart as a
// confirm-able row for the controller path.
func RestartPressed() bool {
	return rl.IsKeyPressed(rl.KeyR)
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
		prevStickEdges = [4]bool{}
		return
	}
	yv := rl.GetGamepadAxisMovement(gamepadID, rl.GamepadAxisLeftY)
	xv := rl.GetGamepadAxisMovement(gamepadID, rl.GamepadAxisLeftX)
	prevStickEdges[stickEdgeUp] = yv <= -stickEdgeThreshold
	prevStickEdges[stickEdgeDown] = yv >= stickEdgeThreshold
	prevStickEdges[stickEdgeLeft] = xv <= -stickEdgeThreshold
	prevStickEdges[stickEdgeRight] = xv >= stickEdgeThreshold
}

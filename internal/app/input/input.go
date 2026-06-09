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

// lookStickDeadzone is the right-stick magnitude below which free-look
// treats the stick as centered. Larger than zero so resting drift
// doesn't pan the camera; smaller than stickEdgeThreshold because
// free-look is analog (proportional), not an edge press.
const lookStickDeadzone = float32(0.15)

// stickEdgeKey identifies one of the four analog-stick directions for
// the edge-detection memory.
type stickEdgeKey int

const (
	stickEdgeUp stickEdgeKey = iota
	stickEdgeDown
	stickEdgeLeft
	stickEdgeRight
)

// stickNow holds "is this direction past threshold THIS frame," sampled
// once per frame by NewFrame; stickPrev holds the previous frame's
// sample. A directional edge is stickNow && !stickPrev. Sampling once
// per frame (rather than on every read) makes the directional
// predicates IDEMPOTENT within a frame: UpPressed / CursorUpDown / etc.
// can be called any number of times without the first call consuming
// the edge out from under the second. Indexed by stickEdgeKey.
var (
	stickNow  [4]bool
	stickPrev [4]bool
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

// NewFrame samples the analog stick once at the start of each frame and
// rolls the directional edge state forward (stickPrev <- stickNow, then
// re-sample stickNow). Call it exactly once per frame, before any input
// is read — run.go's main loop does this ahead of scene dispatch.
// Without it the directional edges never advance (stick navigation goes
// dead); with it, the per-direction predicates are pure reads that stay
// consistent no matter how many times they're called in a frame.
func NewFrame() {
	stickPrev = stickNow
	if !gamepadConnected() {
		stickNow = [4]bool{}
		return
	}
	yv := rl.GetGamepadAxisMovement(gamepadID, rl.GamepadAxisLeftY)
	xv := rl.GetGamepadAxisMovement(gamepadID, rl.GamepadAxisLeftX)
	stickNow[stickEdgeUp] = yv <= -stickEdgeThreshold
	stickNow[stickEdgeDown] = yv >= stickEdgeThreshold
	stickNow[stickEdgeLeft] = xv <= -stickEdgeThreshold
	stickNow[stickEdgeRight] = xv >= stickEdgeThreshold
}

// stickEdge reports the one-frame press edge for a direction: past
// threshold this frame, not last. Pure read (no mutation), so repeated
// same-frame calls all agree.
func stickEdge(key stickEdgeKey) bool {
	return stickNow[key] && !stickPrev[key]
}

// stickEdgeY / stickEdgeX are thin wrappers naming the left-stick axes.
// dir = -1 for up/left, +1 for down/right.
func stickEdgeY(dir int) bool {
	if dir < 0 {
		return stickEdge(stickEdgeUp)
	}
	return stickEdge(stickEdgeDown)
}

func stickEdgeX(dir int) bool {
	if dir < 0 {
		return stickEdge(stickEdgeLeft)
	}
	return stickEdge(stickEdgeRight)
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
	// Re-clamp even on a no-press frame: a list can shrink between
	// frames (a stack consumed, an entry removed) and leave the stored
	// cursor past the end until the next nav press.
	cursor = core.Clamp(cursor, 0, count-1)
	if UpPressed() {
		cursor = core.WrapIndex(cursor-1, count)
	}
	if DownPressed() {
		cursor = core.WrapIndex(cursor+1, count)
	}
	return cursor
}

// UpPressedArrows / DownPressedArrows are text-entry-safe vertical nav:
// the arrow keys and pad/stick, but NOT the W/S letter keys. A cursor
// row that doubles as a text field (the sound modal's Name row) can move
// off the row with these without the typed W/S letters also scrolling it.
func UpPressedArrows() bool {
	return rl.IsKeyPressed(rl.KeyUp) || padPressed(rl.GamepadButtonLeftFaceUp) || stickEdgeY(-1)
}

func DownPressedArrows() bool {
	return rl.IsKeyPressed(rl.KeyDown) || padPressed(rl.GamepadButtonLeftFaceDown) || stickEdgeY(1)
}

// CursorUpDownTextSafe is CursorUpDown without the W/S letter keys, for a
// row list that contains an active text field (so typing those letters
// edits the field instead of moving the cursor). Pad/stick/arrows still
// navigate, so a keyboard user is never trapped on the text row.
func CursorUpDownTextSafe(cursor, count int) int {
	if count <= 0 {
		return cursor
	}
	// Same no-press re-clamp as CursorUpDown — see there.
	cursor = core.Clamp(cursor, 0, count-1)
	if UpPressedArrows() {
		cursor = core.WrapIndex(cursor-1, count)
	}
	if DownPressedArrows() {
		cursor = core.WrapIndex(cursor+1, count)
	}
	return cursor
}

// --- Editor bindings ---------------------------------------------------------
// The map editor is keyboard+mouse-first but must still be operable with a
// controller (AGENTS.md). These predicates live here with the rest so the
// editor's bindings aren't raw rl reads scattered across editor/input.go.
// Note the editor deliberately uses Enter ALONE for commit (not the
// ConfirmPressed chord's Z / Space, which would type into a Name field) —
// so it gets its own confirm rather than reusing ConfirmPressed.

// EditorConfirmPressed is the editor's modal/commit edge: Enter (keyboard,
// chord-free so it can't collide with text entry) plus the pad A face
// button.
func EditorConfirmPressed() bool {
	return rl.IsKeyPressed(rl.KeyEnter) || rl.IsKeyPressed(rl.KeyKpEnter) ||
		padPressed(rl.GamepadButtonRightFaceDown) // A / Cross
}

// EditorCancelPressed is the editor's cancel/back edge: Esc plus pad B.
func EditorCancelPressed() bool {
	return rl.IsKeyPressed(rl.KeyEscape) ||
		padPressed(rl.GamepadButtonRightFaceRight) // B / Circle
}

// EditorTabPressed cycles text-field focus inside an editor modal. Tab is
// inherently a keyboard affordance (field-to-field focus); the pad drives
// modal row nav through CursorUpDown instead.
func EditorTabPressed() bool {
	return rl.IsKeyPressed(rl.KeyTab)
}

// EditorPaintPressed / EditorErasePressed are the grid-cursor paint / erase
// edges. Keyboard keeps Space / Backspace (so they don't collide with the
// modal Enter-commit). On the pad, A / Cross paints; erase uses Square / X
// rather than B / Circle, because B / Circle is the global editor cancel edge
// (it opens the Esc menu): sharing it would make a single "back" press both
// erase the cursor tile AND open the menu in the same frame. The in-game Use
// action also lives on Square / X, but the editor never reads that predicate,
// so the two never overlap.
func EditorPaintPressed() bool {
	return rl.IsKeyPressed(rl.KeySpace) || padPressed(rl.GamepadButtonRightFaceDown) // A / Cross
}

func EditorErasePressed() bool {
	return rl.IsKeyPressed(rl.KeyBackspace) || padPressed(rl.GamepadButtonRightFaceLeft) // Square / X
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

// CursorLeftRightWrap applies CursorLeftRight to a wrap-around cursor in
// [0, count). Horizontal mirror of CursorUpDown — the game-panels
// overlay uses it to move the selected party-member COLUMN (Stats /
// Equipment / Skills) now that L1/L2/R1/R2 own tab paging and the
// d-pad / stick drive the in-tab cursor. Safe for count <= 0.
func CursorLeftRightWrap(cursor, count int) int {
	if count <= 0 {
		return cursor
	}
	// Same no-press re-clamp as CursorUpDown — see there.
	cursor = core.Clamp(cursor, 0, count-1)
	switch CursorLeftRight() {
	case 1:
		return core.WrapIndex(cursor+1, count)
	case -1:
		return core.WrapIndex(cursor-1, count)
	}
	return cursor
}

func TargetNextPressed() bool {
	return rl.IsKeyPressed(rl.KeyTab) || rl.IsKeyPressed(rl.KeyRight) || rl.IsKeyPressed(rl.KeyD) || rl.IsKeyPressed(rl.KeyDown) ||
		padPressed(rl.GamepadButtonLeftFaceRight) || padPressed(rl.GamepadButtonRightTrigger1) || stickEdgeX(1)
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
// (PanelTabShortcutPressed) own C/E/I/K/J/M, so the keyboard player
// jumps directly to a named tab rather than opening "wherever I
// was last." Pressing the same shortcut key again closes the
// overlay (handled in explore/panels.go), preserving the toggle
// feel without a separate "open to last tab" key.
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
//	J → Quests    (Journal — Q is the turn-left key, so J stands in)
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
	case rl.IsKeyPressed(rl.KeyJ):
		return core.PanelTabQuests, true
	case rl.IsKeyPressed(rl.KeyM):
		return core.PanelTabMap, true
	}
	return 0, false
}

// MenuTabPrevPressed / MenuTabNextPressed page tabs INSIDE the
// game-panels overlay. Both shoulders and both triggers cycle — L1/L2
// page back, R1/R2 page forward — so the d-pad / left stick is free to
// drive the in-tab 2-D cursor (member column ↔ slot row) instead of
// doubling as tab navigation. Keyboard pages with Tab / Shift+Tab; the
// per-tab letter shortcuts (PanelTabShortcutPressed) still jump
// straight to a named tab.
func MenuTabPrevPressed() bool {
	if rl.IsKeyPressed(rl.KeyTab) && (rl.IsKeyDown(rl.KeyLeftShift) || rl.IsKeyDown(rl.KeyRightShift)) {
		return true
	}
	return padPressed(rl.GamepadButtonLeftTrigger1) || padPressed(rl.GamepadButtonLeftTrigger2)
}

func MenuTabNextPressed() bool {
	if rl.IsKeyPressed(rl.KeyTab) && !rl.IsKeyDown(rl.KeyLeftShift) && !rl.IsKeyDown(rl.KeyRightShift) {
		return true
	}
	return padPressed(rl.GamepadButtonRightTrigger1) || padPressed(rl.GamepadButtonRightTrigger2)
}

// PagedTab applies the L1/R1 (+ Tab / Shift+Tab) tab-paging edges to a
// wrap-around tab enum, returning the new value and whether it changed —
// so the caller can reset a per-tab cursor on a switch. Generic over the
// tab enum so the panels overlay and the shop share one Next/Prev →
// WrapEnum branch instead of hand-rolling it each.
func PagedTab[T ~int](cur T, count int) (T, bool) {
	switch {
	case MenuTabNextPressed():
		return core.WrapEnum(cur, 1, count), true
	case MenuTabPrevPressed():
		return core.WrapEnum(cur, -1, count), true
	}
	return cur, false
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

// UsePressed is the "use / cast" edge for the panels overlay's
// out-of-battle actions: using a consumable on the Items tab and casting
// a heal skill on the Skills tab (where Confirm is already spent on
// buying tier upgrades, so the use action needs its own button). Bound
// to keyboard F and the gamepad Square/X face button — the one AGENTS.md
// flags as intentionally unbound, so claiming it here invents no combo.
func UsePressed() bool {
	return rl.IsKeyPressed(rl.KeyF) || padPressed(rl.GamepadButtonRightFaceLeft) // Square / X
}

// DebugFleePressed is the edge that abandons an active battle when the
// debug "Easy Battle Quit" toggle is on. Bound to Backspace plus the
// gamepad Select/Share button — kept off the action-menu's confirm /
// back / arrow keys so it can't fire by accident during normal play
// (and it's gated on the toggle at the call site regardless).
func DebugFleePressed() bool {
	return rl.IsKeyPressed(rl.KeyBackspace) || padPressed(rl.GamepadButtonMiddleLeft) // Select / Share
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
		stickNow = [4]bool{}
		stickPrev = [4]bool{}
		return
	}
	yv := rl.GetGamepadAxisMovement(gamepadID, rl.GamepadAxisLeftY)
	xv := rl.GetGamepadAxisMovement(gamepadID, rl.GamepadAxisLeftX)
	stickNow[stickEdgeUp] = yv <= -stickEdgeThreshold
	stickNow[stickEdgeDown] = yv >= stickEdgeThreshold
	stickNow[stickEdgeLeft] = xv <= -stickEdgeThreshold
	stickNow[stickEdgeRight] = xv >= stickEdgeThreshold
	// Both equal -> no edge fires until the player centers and re-tilts.
	stickPrev = stickNow
}

// LookStick returns the right analog stick offset for explore free-look
// as (x, y) in roughly [-1, 1], with a centered deadzone so a resting
// stick reads as (0, 0). Returns (0, 0) when no pad is connected.
// Analog (not edge-detected): the free-look path scales these by
// core.StickLookSense·dt, mirroring the right-mouse-drag axes so mouse
// and controller share one look model.
func LookStick() (float32, float32) {
	if !gamepadConnected() {
		return 0, 0
	}
	x := rl.GetGamepadAxisMovement(gamepadID, rl.GamepadAxisRightX)
	y := rl.GetGamepadAxisMovement(gamepadID, rl.GamepadAxisRightY)
	if x > -lookStickDeadzone && x < lookStickDeadzone {
		x = 0
	}
	if y > -lookStickDeadzone && y < lookStickDeadzone {
		y = 0
	}
	return x, y
}

// --- Mouse / pointer (secondary input) ---------------------------------------
// Gamepad-first: the mouse drives only the Equipment-tab slot-picker clicks
// and right-drag free-look today, but those reads still funnel through here so
// no call site touches raylib directly and "is the mouse driving?" has one answer.

// PointerPos is the current mouse position in screen space.
func PointerPos() rl.Vector2 { return rl.GetMousePosition() }

// PointerMoved reports whether the mouse moved at all this frame — used to
// hand panel focus from the keyboard/controller cursor back to the mouse.
func PointerMoved() bool {
	d := rl.GetMouseDelta()
	return d.X != 0 || d.Y != 0
}

// ClickPressed reports a fresh left-mouse click. The Equipment-tab slot
// picker uses it to register a click on a slot or picker row; routing it
// through the input package keeps the raylib mouse read in one place.
func ClickPressed() bool { return rl.IsMouseButtonPressed(rl.MouseLeftButton) }

// LookDragActive reports the right-mouse free-look hold; LookMouseDelta is its
// per-frame motion. The mouse counterpart of LookStick.
func LookDragActive() bool       { return rl.IsMouseButtonDown(rl.MouseRightButton) }
func LookMouseDelta() rl.Vector2 { return rl.GetMouseDelta() }

package input

import (
	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// gamepadID is the controller slot read (player one).
const gamepadID = int32(0)

// stickEdgeThreshold: the stick must push past this to register a press and
// return below it before the next can fire. 0.55 ignores worn-stick drift.
const stickEdgeThreshold = float32(0.55)

// lookStickDeadzone is the right-stick centered band for free-look. >0 so
// resting drift doesn't pan; < stickEdgeThreshold because free-look is analog.
const lookStickDeadzone = float32(0.15)

// stickEdgeKey identifies one of the four stick directions for the edge memory.
type stickEdgeKey int

const (
	stickEdgeUp stickEdgeKey = iota
	stickEdgeDown
	stickEdgeLeft
	stickEdgeRight
)

// stickNow/stickPrev: "direction past threshold this/last frame" (sampled once
// per frame by NewFrame). An edge is stickNow && !stickPrev — sampling once
// makes the directional predicates idempotent within a frame. Indexed by stickEdgeKey.
var (
	stickNow  [4]bool
	stickPrev [4]bool
)

// padAvailable caches rl.IsGamepadAvailable once per frame (NewFrame); collapses
// the ~10-15 redundant cgo probes a frame's predicates used to issue into one.
var padAvailable bool

// gamepadConnected reports whether a pad is connected (cached per frame). All
// controller reads short-circuit to false when there's none.
func gamepadConnected() bool {
	return padAvailable
}

// padPressed/padDown/padReleased: button edge/held/release, gated on a connected pad.
func padPressed(button int32) bool {
	return gamepadConnected() && rl.IsGamepadButtonPressed(gamepadID, button)
}

func padDown(button int32) bool {
	return gamepadConnected() && rl.IsGamepadButtonDown(gamepadID, button)
}

func padReleased(button int32) bool {
	return gamepadConnected() && rl.IsGamepadButtonReleased(gamepadID, button)
}

// lastRumbleLevel is the last level handed to the driver, so an idle 0 doesn't
// re-issue every frame (driver runs only on a change, plus once to stop at 0).
var lastRumbleLevel float32 = -1

// ApplyRumble drives the motors to `level` (0..1), forcing 0 when disabled or no
// pad. Non-zero re-issues each frame (tracks falloff); idle 0 no-ops after one
// stop. Output goes through setGamepadRumble — raylib's SetGamepadVibration is a
// no-op on this GLFW backend, so it's NOT called.
func ApplyRumble(level float32, enabled bool) {
	if !enabled || !gamepadConnected() {
		level = 0
	}
	level = core.Clamp(level, 0, 1)
	if level == 0 && lastRumbleLevel == 0 {
		return // motor already off — don't re-issue every idle frame
	}
	lastRumbleLevel = level
	setGamepadRumble(level)
}

// StopRumble cuts the motors immediately. Called on exit — XInput vibration
// persists until changed, so an explicit stop matters.
func StopRumble() {
	if lastRumbleLevel == 0 {
		return
	}
	lastRumbleLevel = 0
	setGamepadRumble(0)
}

// NewFrame rolls the edge state forward (stickPrev <- stickNow, then re-sample).
// Call exactly once per frame before any input is read — else the directional
// edges never advance (stick nav goes dead).
func NewFrame() {
	// Sample pad presence once; predicates read cached padAvailable this frame.
	padAvailable = rl.IsGamepadAvailable(gamepadID)
	stickPrev = stickNow
	if !gamepadConnected() {
		stickNow = [4]bool{}
		return
	}
	sampleStickNow()
}

// sampleStickNow fills stickNow from the left-stick axes (active past
// stickEdgeThreshold). Assumes a pad is connected (no-pad zeroing in callers).
func sampleStickNow() {
	yv := rl.GetGamepadAxisMovement(gamepadID, rl.GamepadAxisLeftY)
	xv := rl.GetGamepadAxisMovement(gamepadID, rl.GamepadAxisLeftX)
	stickNow[stickEdgeUp] = yv <= -stickEdgeThreshold
	stickNow[stickEdgeDown] = yv >= stickEdgeThreshold
	stickNow[stickEdgeLeft] = xv <= -stickEdgeThreshold
	stickNow[stickEdgeRight] = xv >= stickEdgeThreshold
}

// stickEdge reports the one-frame press edge: past threshold this frame, not last.
func stickEdge(key stickEdgeKey) bool {
	return stickNow[key] && !stickPrev[key]
}

// stickEdgeY / stickEdgeX name the left-stick axes. dir = -1 up/left, +1 down/right.
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

// stickHeldY / stickHeldX are the level (held-past-threshold) reads powering
// hold-to-move. dir = -1 up/left, +1 down/right.
func stickHeldY(dir int) bool {
	if dir < 0 {
		return stickNow[stickEdgeUp]
	}
	return stickNow[stickEdgeDown]
}

func stickHeldX(dir int) bool {
	if dir < 0 {
		return stickNow[stickEdgeLeft]
	}
	return stickNow[stickEdgeRight]
}

// --- High-level semantic actions ---------------------------------------------
// Every action accepts keyboard and controller. Keep call sites on these names
// so remapping touches one file.

// altDown reports either Alt held — the modifier that turns Enter into the
// display-mode toggle instead of a confirm.
func altDown() bool {
	return rl.IsKeyDown(rl.KeyLeftAlt) || rl.IsKeyDown(rl.KeyRightAlt)
}

// DisplayTogglePressed is the global Alt+Enter fullscreen/windowed edge. The
// confirm predicates exclude Enter-with-Alt so it can't also confirm a row.
func DisplayTogglePressed() bool {
	return altDown() && (rl.IsKeyPressed(rl.KeyEnter) || rl.IsKeyPressed(rl.KeyKpEnter))
}

// plainEnterPressed is "bare main-Enter edge, NOT Alt+Enter" (the display
// toggle), shared by both confirm predicates. Main Enter only.
func plainEnterPressed() bool {
	return rl.IsKeyPressed(rl.KeyEnter) && !altDown()
}

// confirmChord is the confirm button set — Enter / Space / Z plus pad A/Cross.
// The edge/level/release variants differ only in which readers they pass.
func confirmChord(keyState, padState func(int32) bool) bool {
	return keyState(rl.KeyEnter) || keyState(rl.KeySpace) || keyState(rl.KeyZ) ||
		padState(rl.GamepadButtonRightFaceDown) // A on Xbox / Cross on PS
}

func ConfirmPressed() bool {
	// Enter arm ignores Alt+Enter (the fullscreen chord); rest is the shared set.
	return confirmChord(func(k int32) bool {
		if k == rl.KeyEnter {
			return plainEnterPressed()
		}
		return rl.IsKeyPressed(k)
	}, padPressed)
}

// backChord is the shared "back / cancel" chord — Escape plus pad B / Circle.
// EditorErasePressed deliberately does NOT use it (it's Square/X) so a back press
// can't both erase a tile and open the editor's Esc menu.
func backChord() bool {
	return rl.IsKeyPressed(rl.KeyEscape) ||
		padPressed(rl.GamepadButtonRightFaceRight) // B / Circle
}

func BackPressed() bool {
	return backChord() || rl.IsKeyPressed(rl.KeyX)
}

// padDirUp/Down/Left/Right: the controller half of a directional press (D-pad OR
// stick edge), shared across families so remapping the pad/stick is one edit.
func padDirUp() bool    { return padPressed(rl.GamepadButtonLeftFaceUp) || stickEdgeY(-1) }
func padDirDown() bool  { return padPressed(rl.GamepadButtonLeftFaceDown) || stickEdgeY(1) }
func padDirLeft() bool  { return padPressed(rl.GamepadButtonLeftFaceLeft) || stickEdgeX(-1) }
func padDirRight() bool { return padPressed(rl.GamepadButtonLeftFaceRight) || stickEdgeX(1) }

func UpPressed() bool {
	return rl.IsKeyPressed(rl.KeyUp) || rl.IsKeyPressed(rl.KeyW) || padDirUp()
}

func DownPressed() bool {
	return rl.IsKeyPressed(rl.KeyDown) || rl.IsKeyPressed(rl.KeyS) || padDirDown()
}

// CursorUpDown applies UpPressed / DownPressed to a wrap-around cursor in
// [0, count). Safe for count <= 0 (returns cursor unchanged).
func CursorUpDown(cursor, count int) int {
	if count <= 0 {
		return cursor
	}
	// Re-clamp even on a no-press frame: a list can shrink between frames and
	// leave the stored cursor past the end until the next nav press.
	cursor = core.Clamp(cursor, 0, count-1)
	if UpPressed() {
		cursor = core.WrapIndex(cursor-1, count)
	}
	if DownPressed() {
		cursor = core.WrapIndex(cursor+1, count)
	}
	return cursor
}

// UpPressedArrows / DownPressedArrows are text-entry-safe vertical nav: arrows +
// pad/stick but NOT W/S, so a row doubling as a text field doesn't scroll when
// W/S are typed.
func UpPressedArrows() bool {
	return rl.IsKeyPressed(rl.KeyUp) || padDirUp()
}

func DownPressedArrows() bool {
	return rl.IsKeyPressed(rl.KeyDown) || padDirDown()
}

// CursorUpDownTextSafe is CursorUpDown without W/S, for a list with an active
// text field. Pad/stick/arrows still navigate.
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
// Keyboard+mouse-first but controller-operable (AGENTS.md). Commit is Enter ALONE
// (not the ConfirmPressed chord's Z/Space, which would type into a Name field).

// EditorConfirmPressed is the editor's modal/commit edge: chord-free Enter plus pad A.
func EditorConfirmPressed() bool {
	// Same Alt+Enter exclusion as ConfirmPressed; also accepts keypad Enter.
	return plainEnterPressed() || (rl.IsKeyPressed(rl.KeyKpEnter) && !altDown()) ||
		padPressed(rl.GamepadButtonRightFaceDown) // A / Cross
}

// EditorCancelPressed is the editor's cancel/back edge: Esc plus pad B.
func EditorCancelPressed() bool {
	return backChord()
}

// EditorTabPressed cycles text-field focus in an editor modal (keyboard-only;
// the pad uses CursorUpDown for row nav).
func EditorTabPressed() bool {
	return rl.IsKeyPressed(rl.KeyTab)
}

// EditorPaintPressed / EditorErasePressed: grid-cursor paint/erase. Keyboard
// Space / Backspace; pad A/Cross paints, erase uses Square/X (not B/Circle, the
// cancel edge) so one "back" press can't both erase and open the Esc menu.
func EditorPaintPressed() bool {
	return rl.IsKeyPressed(rl.KeySpace) || padPressed(rl.GamepadButtonRightFaceDown) // A / Cross
}

func EditorErasePressed() bool {
	return rl.IsKeyPressed(rl.KeyBackspace) || padPressed(rl.GamepadButtonRightFaceLeft) // Square / X
}

// CursorLeftRight returns -1 on a Left edge, +1 on a Right edge, 0 otherwise.
// Edge-only so a held key doesn't stream values; callers do the adjust math.
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
// [0, count) — the panels overlay moves the member column with it. Safe for
// count <= 0.
func CursorLeftRightWrap(cursor, count int) int {
	if count <= 0 {
		return cursor
	}
	// Same no-press re-clamp as CursorUpDown.
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

// PausePressed reports the pause-menu edge: P / Esc / pad Start (MiddleRight, the
// "small start" Options/Menu button — the big Middle button is PanelsTogglePressed's
// so the two reads don't fight). Esc opens the menu the same way in AND out of
// battle (the pause check runs before battle.Update and returns, so Esc can't also
// fall through to the battle Back edge); pauseAllowed still blocks it during a
// timing bar so the input window can't be sidestepped.
func PausePressed() bool {
	return rl.IsKeyPressed(rl.KeyP) ||
		rl.IsKeyPressed(rl.KeyEscape) ||
		padPressed(rl.GamepadButtonMiddleRight) // Start
}

// PanelsTogglePressed opens/closes the panels overlay (same edge toggles off).
// Bound to pad Triangle/Y plus the middle button (PS/guide). Keyboard does NOT
// toggle here — the per-tab shortcuts own C/E/I/K/J/M (re-press closes).
func PanelsTogglePressed() bool {
	return padPressed(rl.GamepadButtonMiddle) ||
		padPressed(rl.GamepadButtonRightFaceUp) // Triangle / Y
}

// PanelTabShortcutPressed reports a per-tab keyboard shortcut, returning the tab
// and true on a fresh edge. Letters match the on-screen mnemonics:
//
//	C → Stats (Character), E → Equipment, I → Items, K → Skills,
//	J → Quests (Journal — Q is turn-left), M → Map
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

// MenuTabPrevPressed / MenuTabNextPressed page panels-overlay tabs: L1/L2 back,
// R1/R2 forward (freeing the d-pad/stick for the cursor); keyboard Tab/Shift+Tab.
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

// PagedTab applies the tab-paging edges to a wrap-around tab enum, returning the
// new value and whether it changed. Generic so the panels overlay and shop share it.
func PagedTab[T ~int](cur T, count int) (T, bool) {
	switch {
	case MenuTabNextPressed():
		return core.WrapEnum(cur, 1, count), true
	case MenuTabPrevPressed():
		return core.WrapEnum(cur, -1, count), true
	}
	return cur, false
}

// RestartPressed is the "restart run" edge — keyboard R only (the controller path
// uses the pause menu's Restart row; Triangle/Y went to the panels overlay).
func RestartPressed() bool {
	return rl.IsKeyPressed(rl.KeyR)
}

func QuitPressed() bool {
	return rl.IsKeyPressed(rl.KeyQ) || padPressed(rl.GamepadButtonMiddleLeft) // Select / Share
}

// UsePressed is the panels "use / cast" edge (Items consumable / Skills heal),
// separate from Confirm. F + pad Square/X (AGENTS.md flags Square/X as unbound).
func UsePressed() bool {
	return rl.IsKeyPressed(rl.KeyF) || padPressed(rl.GamepadButtonRightFaceLeft) // Square / X
}

// DebugFleePressed abandons a battle when "Easy Battle Quit" is on. Backspace +
// pad Select/Share — off the action menu's keys so it can't fire by accident.
func DebugFleePressed() bool {
	return rl.IsKeyPressed(rl.KeyBackspace) || padPressed(rl.GamepadButtonMiddleLeft) // Select / Share
}

// Held movement predicates: level reads of the four steps and two turns. The
// explore update re-fires the next step as the prior lands (no repeat timer); the
// edge *Pressed variants stay for menu nav, which must NOT auto-repeat.
func StepForwardHeld() bool {
	return rl.IsKeyDown(rl.KeyUp) || rl.IsKeyDown(rl.KeyW) ||
		padDown(rl.GamepadButtonLeftFaceUp) || stickHeldY(-1)
}

func StepBackHeld() bool {
	return rl.IsKeyDown(rl.KeyDown) || rl.IsKeyDown(rl.KeyS) ||
		padDown(rl.GamepadButtonLeftFaceDown) || stickHeldY(1)
}

func StrafeLeftHeld() bool {
	return rl.IsKeyDown(rl.KeyA) || stickHeldX(-1)
}

func StrafeRightHeld() bool {
	return rl.IsKeyDown(rl.KeyD) || stickHeldX(1)
}

func TurnLeftHeld() bool {
	return rl.IsKeyDown(rl.KeyLeft) || rl.IsKeyDown(rl.KeyQ) ||
		padDown(rl.GamepadButtonLeftTrigger1)
}

func TurnRightHeld() bool {
	return rl.IsKeyDown(rl.KeyRight) || rl.IsKeyDown(rl.KeyE) ||
		padDown(rl.GamepadButtonRightTrigger1)
}

// ConfirmDown / ConfirmReleased are the held / up-edge counterparts to
// ConfirmPressed, for hold-mode minigames.
func ConfirmDown() bool {
	return confirmChord(rl.IsKeyDown, padDown)
}

func ConfirmReleased() bool {
	return confirmChord(rl.IsKeyReleased, padReleased)
}

// AttackTiming{Pressed,Held,Released} map the timed/charge attack minigame to the
// Confirm edge/held/release; DefendTimingPressed maps the block to Back.
func AttackTimingPressed() bool {
	return ConfirmPressed()
}

func AttackTimingHeld() bool {
	return ConfirmDown()
}

func AttackTimingReleased() bool {
	return ConfirmReleased()
}

func DefendTimingPressed() bool {
	return BackPressed()
}

// Directional edges for the pickpocket sequence minigame. Arrows / D-pad / left
// stick only — WASD is NOT accepted since the prompt shows literal arrows.
func ArrowUpPressed() bool {
	return rl.IsKeyPressed(rl.KeyUp) || padDirUp()
}

func ArrowDownPressed() bool {
	return rl.IsKeyPressed(rl.KeyDown) || padDirDown()
}

func ArrowLeftPressed() bool {
	return rl.IsKeyPressed(rl.KeyLeft) || padDirLeft()
}

func ArrowRightPressed() bool {
	return rl.IsKeyPressed(rl.KeyRight) || padDirRight()
}

// ResetStickEdges seeds the edge memory from the current stick state on entering
// a new input context, so an already-tilted stick fires no phantom frame-1 edge.
func ResetStickEdges() {
	if !gamepadConnected() {
		stickNow = [4]bool{}
		stickPrev = [4]bool{}
		return
	}
	sampleStickNow()
	stickPrev = stickNow // equal -> no edge until centered and re-tilted
}

// applyDeadzone zeroes an axis value below dz (centered dead band).
func applyDeadzone(v, dz float32) float32 {
	if v > -dz && v < dz {
		return 0
	}
	return v
}

// LookStick returns the right-stick free-look offset (x, y) in ~[-1, 1] with a
// centered deadzone; (0, 0) when no pad. Analog, mirroring the right-mouse-drag axes.
func LookStick() (float32, float32) {
	if !gamepadConnected() {
		return 0, 0
	}
	x := applyDeadzone(rl.GetGamepadAxisMovement(gamepadID, rl.GamepadAxisRightX), lookStickDeadzone)
	y := applyDeadzone(rl.GetGamepadAxisMovement(gamepadID, rl.GamepadAxisRightY), lookStickDeadzone)
	return x, y
}

// --- Mouse / pointer (secondary input) ---------------------------------------
// Drives only Equipment slot-picker clicks and right-drag free-look, funneled
// through here so no call site touches raylib directly.

// PointerPos is the current mouse position in screen space.
func PointerPos() rl.Vector2 { return rl.GetMousePosition() }

// PointerMoved reports any mouse motion — hands panel focus back to the mouse.
func PointerMoved() bool {
	d := rl.GetMouseDelta()
	return d.X != 0 || d.Y != 0
}

// ClickPressed reports a fresh left-mouse click (Equipment slot/row picks).
func ClickPressed() bool { return rl.IsMouseButtonPressed(rl.MouseLeftButton) }

// LookDragActive is the right-mouse free-look hold; LookMouseDelta its motion.
func LookDragActive() bool       { return rl.IsMouseButtonDown(rl.MouseRightButton) }
func LookMouseDelta() rl.Vector2 { return rl.GetMouseDelta() }

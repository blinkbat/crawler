// Package editor is the in-game map authoring tool. Paints tiles and walls,
// drops the player start and enemy spawns, edits per-map metadata, and reads
// / writes .map files via core/mapfile.
package editor

import (
	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Action is what the editor wants the run loop to do next.
type Action int

const (
	ActionNone Action = iota
	ActionExitToTitle
	// ActionTest asks the run loop to drop into the adventure scene with
	// the editor's current in-memory area, returning here on quit.
	ActionTest
)

// Tool is the currently selected painting / placing mode. Tile brushes paint
// a byte into the layout; placement tools (PlayerStart, SpawnRat, SpawnBat)
// drop or move an entity.
type Tool int

const (
	ToolFloor Tool = iota
	ToolWall
	ToolTree
	ToolTreeXL
	ToolBoulder
	ToolBush
	ToolPlayerStart
	ToolSpawnRat
	ToolSpawnBat
)

type toolEntry struct {
	tool     Tool
	label    string
	hotkey   int32
	tileByte byte
	color    rl.Color
}

var toolEntries = []toolEntry{
	{ToolFloor, "Floor (.)", rl.KeyOne, core.TileFloor, rl.NewColor(180, 168, 140, 255)},
	{ToolWall, "Wall (#)", rl.KeyTwo, core.TileRock, rl.NewColor(96, 96, 110, 255)},
	{ToolTree, "Tree (T)", rl.KeyThree, core.TileTree, rl.NewColor(64, 140, 80, 255)},
	{ToolTreeXL, "Tree XL (X)", rl.KeyFour, core.TileTreeXL, rl.NewColor(36, 96, 56, 255)},
	{ToolBoulder, "Boulder (O)", rl.KeyFive, core.TileRockLarge, rl.NewColor(132, 110, 90, 255)},
	{ToolBush, "Bush (B)", rl.KeySix, core.TileBushLarge, rl.NewColor(112, 142, 70, 255)},
	{ToolPlayerStart, "Player Start", rl.KeySeven, 0, rl.NewColor(255, 220, 124, 255)},
	{ToolSpawnRat, "Spawn Rat", rl.KeyEight, 0, rl.NewColor(220, 156, 96, 255)},
	{ToolSpawnBat, "Spawn Bat", rl.KeyNine, 0, rl.NewColor(160, 130, 220, 255)},
}

type focusField int

const (
	focusNone focusField = iota
	focusName
	focusQuiet
	focusFilename
	focusWidth
	focusHeight
)

type modalKind int

const (
	modalNone modalKind = iota
	modalOpen
	modalSaveAs
	modalConfirmDirty
)

// pendingAction names the action that the confirm-dirty prompt is gating on.
// Lets the same modal cover Esc-to-title, New, and Open.
type pendingAction int

const (
	pendingNone pendingAction = iota
	pendingExitToTitle
	pendingNew
	pendingOpen
)

// dragKind tracks what a left-button drag is doing on the grid. Set on
// mouse-down based on modifiers + tool + cell contents; consumed each
// frame the button is held; cleared on release.
type dragKind int

const (
	dragNone dragKind = iota
	dragPaint
	dragRect
	dragStart
	dragEnemy
)

// statusEntry is one line in the rolling status log shown over the grid.
type statusEntry struct {
	msg   string
	timer float32
}

const undoLimit = 50

// State is the editor's mutable state across frames. Layout rectangles are
// recomputed in layout() each frame from the current window size.
type State struct {
	area core.AreaDefinition
	tool Tool

	focus focusField
	// numericBuf is the pending typed-digits buffer for focusWidth /
	// focusHeight. Held separately from area dimensions so partial typing
	// (e.g. "3" en route to "30") doesn't immediately resize.
	numericBuf string

	modal         modalKind
	modalPaths    []string
	modalCursor   int
	modalFilename string
	// modalRenaming is non-empty while the user is typing a new name for
	// the highlighted entry in the Open modal (R key). Holds the buffer
	// until they press Enter.
	modalRenaming string
	modalConfirmDelete bool
	// pending is the destructive action the confirm-dirty modal will run
	// when the user picks Save / Discard. Used to gate New, Open, and
	// Esc-to-title behind a "save first?" prompt when dirty.
	pending pendingAction

	undo  []core.AreaDefinition
	redo  []core.AreaDefinition
	dirty bool

	// statusLog stacks the most recent flashes; oldest expire first via
	// per-entry timers. Replaces the old single-line statusMsg.
	statusLog []statusEntry

	// brushSize is the side length of the square painted by tile brushes
	// (1, 3, or 5). Centered on the hovered cell.
	brushSize int

	// drag tracks what a left-button stroke is doing this frame.
	drag             dragKind
	dragSnapshotDone bool
	lastPaintX       int
	lastPaintZ       int
	rectAnchorX      int
	rectAnchorZ      int
	// dragSpawnIdx is the index into area.EnemySpawns being moved while
	// drag == dragEnemy.
	dragSpawnIdx int

	// gridCursorX/Z is a logical keyboard cursor over the grid. -1 means
	// the cursor is hidden (mouse-driven mode); set by arrow keys.
	gridCursorX int
	gridCursorZ int

	// hoverX/Z is the mouse-hovered cell, or (-1,-1) when off-grid.
	hoverX int
	hoverZ int

	// zoom multiplies the auto-fit cell size; pan offsets the grid plot.
	// Reset by R when no tool requires it (R is otherwise the rotate-start
	// hotkey, which only fires under ToolPlayerStart).
	zoom    float32
	panX    float32
	panY    float32
	panning bool

	exitRequested bool
	testRequested bool
	// awaitingOverwrite is set when Save As detected the typed filename
	// already exists on disk. The modal shifts into a Y/N prompt before
	// clobbering — Y proceeds, N/Esc returns to typing.
	awaitingOverwrite bool

	rect layoutRect
}

type layoutRect struct {
	topbar   rl.Rectangle
	palette  rl.Rectangle
	metadata rl.Rectangle
	grid     rl.Rectangle
	cellPx   float32
	gridX    float32
	gridY    float32
	gridW    float32
	gridH    float32
}

// Area returns a copy of the area currently being edited. Used by the run
// loop's F5 playtest path to spin up a GameState from in-memory edits.
func (s State) Area() core.AreaDefinition {
	return cloneArea(s.area)
}

// New starts the editor with a blank 16x16 map.
func New() State {
	return freshState(blankArea(16, 16))
}

// NewFromArea opens the editor on an already-loaded area.
func NewFromArea(a core.AreaDefinition) State {
	return freshState(a)
}

func freshState(a core.AreaDefinition) State {
	return State{
		area:         a,
		tool:         ToolWall,
		brushSize:    1,
		zoom:         1,
		gridCursorX:  -1,
		gridCursorZ:  -1,
		hoverX:       -1,
		hoverZ:       -1,
		dragSpawnIdx: -1,
	}
}

func blankArea(w, h int) core.AreaDefinition {
	rows := make([]string, h)
	for z := 0; z < h; z++ {
		b := make([]byte, w)
		for x := 0; x < w; x++ {
			if x == 0 || z == 0 || x == w-1 || z == h-1 {
				b[x] = core.TileRock
			} else {
				b[x] = core.TileFloor
			}
		}
		rows[z] = string(b)
	}
	return core.AreaDefinition{
		Name:         "Untitled",
		Layout:       rows,
		Materials:    core.MaterialDungeon,
		StartTileX:   1,
		StartTileZ:   1,
		StartFacing:  core.East,
		QuietMessage: "",
	}
}

// Update advances the editor one frame. Returns the next action for the
// run loop (ActionNone keeps editing; ActionExitToTitle / ActionTest pop).
func Update(s *State, dt float32) Action {
	s.layout()

	tickStatusLog(s, dt)

	if s.modal != modalNone {
		return updateModal(s)
	}

	if s.focus != focusNone {
		updateTextInput(s)
		if rl.IsMouseButtonPressed(rl.MouseLeftButton) && !pointIn(rl.GetMousePosition(), s.activeFieldRect()) {
			s.focus = focusNone
		}
		return ActionNone
	}

	updateHotkeys(s)
	updateMouse(s)

	if s.testRequested {
		s.testRequested = false
		if warnings := reachabilityWarnings(s.area); len(warnings) > 0 {
			for _, w := range warnings {
				s.flash("Test: " + w)
			}
			// Don't drop into a guaranteed-broken playtest. The user can
			// dismiss the warnings and press F5 again on a fixable issue,
			// but the start-on-wall case will keep flagging.
			if !canPlaytest(s.area) {
				return ActionNone
			}
		}
		return ActionTest
	}

	exit := s.exitRequested || rl.IsKeyPressed(rl.KeyEscape)
	s.exitRequested = false
	if exit {
		if s.dirty {
			s.pending = pendingExitToTitle
			s.modal = modalConfirmDirty
			return ActionNone
		}
		return ActionExitToTitle
	}

	return ActionNone
}

// canPlaytest is the strict subset of reachability checks that MUST pass
// before we'll drop into adventure mode — anything that would crash or
// soft-lock the player on entry. Less stringent than reachabilityWarnings.
func canPlaytest(a core.AreaDefinition) bool {
	if a.StartTileZ < 0 || a.StartTileZ >= len(a.Layout) {
		return false
	}
	if a.StartTileX < 0 || a.StartTileX >= len(a.Layout[0]) {
		return false
	}
	if isBlockingByte(a.Layout[a.StartTileZ][a.StartTileX]) {
		return false
	}
	return true
}

// flash pushes a transient message onto the rolling status log. Old
// messages expire on their own timer; the log is capped at 4 entries.
func (s *State) flash(msg string) {
	s.statusLog = append(s.statusLog, statusEntry{msg: msg, timer: 2.5})
	if len(s.statusLog) > 4 {
		s.statusLog = s.statusLog[len(s.statusLog)-4:]
	}
}

func tickStatusLog(s *State, dt float32) {
	if len(s.statusLog) == 0 {
		return
	}
	out := s.statusLog[:0]
	for _, e := range s.statusLog {
		e.timer -= dt
		if e.timer > 0 {
			out = append(out, e)
		}
	}
	s.statusLog = out
}

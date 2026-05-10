// Package editor is the in-game map authoring tool. Maps are stored as
// four parallel ASCII layers (walls / floor / decor / props) plus a list
// of entities (player start + enemy spawns). The editor lets the user
// select an active layer and paint into it with layer-specific brushes.
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

// Layer names which of the area's four grids (or the entity list) the
// editor is currently authoring. Active layer drives the palette, the
// click action, and the visual emphasis on the grid.
type Layer int

const (
	LayerWalls Layer = iota
	LayerFloor
	LayerDecor
	LayerProps
	LayerEntities

	layerCount = 5
)

// Brush is one entry in a layer's palette. For grid layers, char is the
// byte written into that layer at the painted cell. For LayerEntities,
// char is 0 and entity names which placement tool fires on click.
type Brush struct {
	Name   string
	Char   byte
	Entity entityKind
	Hotkey int32
	Color  rl.Color
}

type entityKind int

const (
	entityNone entityKind = iota
	entityPlayerStart
	entitySpawnRat
	entitySpawnBat
)

// layerBrushes is the per-layer palette table. Index into the active
// layer's slice with State.brushIdx[layer]. Hotkeys 1–9 map directly to
// indices 0–8 within the active layer.
var layerBrushes = [layerCount][]Brush{
	LayerWalls: {
		{Name: "Wall (#)", Char: core.TileRock, Hotkey: rl.KeyOne, Color: rl.NewColor(96, 96, 110, 255)},
		{Name: "Open (.)", Char: '.', Hotkey: rl.KeyTwo, Color: rl.NewColor(180, 168, 140, 255)},
	},
	LayerFloor: {
		{Name: "Auto", Char: core.FloorAuto, Hotkey: rl.KeyOne, Color: rl.NewColor(160, 168, 140, 255)},
		{Name: "Grass (g)", Char: core.FloorGrass, Hotkey: rl.KeyTwo, Color: rl.NewColor(120, 184, 110, 255)},
		{Name: "Dirt (d)", Char: core.FloorDirt, Hotkey: rl.KeyThree, Color: rl.NewColor(168, 132, 92, 255)},
		{Name: "Dark grass (k)", Char: core.FloorDarkGrass, Hotkey: rl.KeyFour, Color: rl.NewColor(72, 116, 70, 255)},
		{Name: "Stone (s)", Char: core.FloorStone, Hotkey: rl.KeyFive, Color: rl.NewColor(150, 148, 142, 255)},
	},
	LayerDecor: {
		{Name: "Auto", Char: core.DecorAuto, Hotkey: rl.KeyOne, Color: rl.NewColor(220, 224, 200, 255)},
		{Name: "Force empty (_)", Char: core.DecorEmpty, Hotkey: rl.KeyTwo, Color: rl.NewColor(60, 64, 70, 255)},
		{Name: "Bush (b)", Char: core.DecorBush, Hotkey: rl.KeyThree, Color: rl.NewColor(112, 142, 70, 255)},
		{Name: "Mushroom (m)", Char: core.DecorMushroom, Hotkey: rl.KeyFour, Color: rl.NewColor(220, 100, 110, 255)},
		{Name: "Pebble (p)", Char: core.DecorPebble, Hotkey: rl.KeyFive, Color: rl.NewColor(200, 192, 178, 255)},
	},
	LayerProps: {
		{Name: "None (erase)", Char: '.', Hotkey: rl.KeyOne, Color: rl.NewColor(60, 64, 70, 255)},
		{Name: "Tree (T)", Char: core.TileTree, Hotkey: rl.KeyTwo, Color: rl.NewColor(64, 140, 80, 255)},
		{Name: "Tree XL (X)", Char: core.TileTreeXL, Hotkey: rl.KeyThree, Color: rl.NewColor(36, 96, 56, 255)},
		{Name: "Boulder (O)", Char: core.TileRockLarge, Hotkey: rl.KeyFour, Color: rl.NewColor(132, 110, 90, 255)},
		{Name: "Bush large (B)", Char: core.TileBushLarge, Hotkey: rl.KeyFive, Color: rl.NewColor(112, 142, 70, 255)},
	},
	LayerEntities: {
		{Name: "Player Start", Entity: entityPlayerStart, Hotkey: rl.KeyOne, Color: rl.NewColor(255, 220, 124, 255)},
		{Name: "Spawn Rat", Entity: entitySpawnRat, Hotkey: rl.KeyTwo, Color: rl.NewColor(220, 156, 96, 255)},
		{Name: "Spawn Bat", Entity: entitySpawnBat, Hotkey: rl.KeyThree, Color: rl.NewColor(160, 130, 220, 255)},
	},
}

func layerName(l Layer) string {
	switch l {
	case LayerWalls:
		return "Walls"
	case LayerFloor:
		return "Floor"
	case LayerDecor:
		return "Decor"
	case LayerProps:
		return "Props"
	case LayerEntities:
		return "Entities"
	}
	return "?"
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

type pendingAction int

const (
	pendingNone pendingAction = iota
	pendingExitToTitle
	pendingNew
	pendingOpen
)

type dragKind int

const (
	dragNone dragKind = iota
	dragPaint
	dragRect
	dragStart
	dragEnemy
)

type statusEntry struct {
	msg   string
	timer float32
}

const undoLimit = 50

// State is the editor's mutable state across frames.
type State struct {
	area core.AreaDefinition

	layer    Layer
	brushIdx [layerCount]int

	focus      focusField
	numericBuf string

	modal              modalKind
	modalPaths         []string
	modalCursor        int
	modalFilename      string
	modalRenaming      string
	modalConfirmDelete bool
	pending            pendingAction

	undo  []core.AreaDefinition
	redo  []core.AreaDefinition
	dirty bool

	statusLog []statusEntry

	brushSize int

	drag             dragKind
	dragSnapshotDone bool
	lastPaintX       int
	lastPaintZ       int
	rectAnchorX      int
	rectAnchorZ      int
	dragSpawnIdx     int

	gridCursorX int
	gridCursorZ int
	hoverX      int
	hoverZ      int

	zoom    float32
	panX    float32
	panY    float32
	panning bool

	exitRequested     bool
	testRequested     bool
	awaitingOverwrite bool

	rect layoutRect
}

type layoutRect struct {
	topbar     rl.Rectangle
	layerTabs  rl.Rectangle
	palette    rl.Rectangle
	metadata   rl.Rectangle
	grid       rl.Rectangle
	cellPx     float32
	gridX      float32
	gridY      float32
	gridW      float32
	gridH      float32
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
		layer:        LayerWalls,
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
	walls := make([]string, h)
	floor := make([]string, h)
	decor := make([]string, h)
	props := make([]string, h)
	for z := 0; z < h; z++ {
		wb := make([]byte, w)
		for x := 0; x < w; x++ {
			if x == 0 || z == 0 || x == w-1 || z == h-1 {
				wb[x] = core.TileRock
			} else {
				wb[x] = '.'
			}
		}
		walls[z] = string(wb)
		floor[z] = blankRow(w, core.FloorAuto)
		decor[z] = blankRow(w, core.DecorAuto)
		props[z] = blankRow(w, '.')
	}
	return core.AreaDefinition{
		Name:         "Untitled",
		Width:        w,
		Height:       h,
		Walls:        walls,
		Floor:        floor,
		Decor:        decor,
		Props:        props,
		Materials:    core.MaterialDungeon,
		StartTileX:   1,
		StartTileZ:   1,
		StartFacing:  core.East,
		QuietMessage: "",
	}
}

func blankRow(width int, c byte) string {
	b := make([]byte, width)
	for i := range b {
		b[i] = c
	}
	return string(b)
}

// activeBrush returns the currently selected brush in the active layer.
func (s *State) activeBrush() Brush {
	palette := layerBrushes[s.layer]
	idx := s.brushIdx[s.layer]
	if idx < 0 || idx >= len(palette) {
		idx = 0
	}
	return palette[idx]
}

// Update advances the editor one frame. Returns the next action for the
// run loop.
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
// soft-lock the player on entry.
func canPlaytest(a core.AreaDefinition) bool {
	if a.StartTileZ < 0 || a.StartTileZ >= a.Height {
		return false
	}
	if a.StartTileX < 0 || a.StartTileX >= a.Width {
		return false
	}
	if a.Walls[a.StartTileZ][a.StartTileX] == core.TileRock {
		return false
	}
	if isPropChar(a.Props[a.StartTileZ][a.StartTileX]) {
		return false
	}
	return true
}

// flash pushes a transient message onto the rolling status log.
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

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
	// entityAddRat/Bat/DiseasedRat append a member to the pack at the clicked
	// tile, creating a fresh pack if none is there yet. Placing a Rat brush
	// over an existing Rat-only pack makes it a 2-rat pack; mixing kinds
	// builds mixed packs. Right-click clears the entire pack.
	entityAddRat
	entityAddBat
	entityAddDiseasedRat
)

// layerBrushes is the per-layer palette table. Index into the active
// layer's slice with State.brushIdx[layer]. Hotkeys 1–9 map directly to
// indices 0–8 within the active layer; brushes past index 8 keep
// Hotkey: 0 (no keyboard binding — mouse-only) since we ran out of
// 1–9 keys on the palette.
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
		{Name: "Cobble (c)", Char: core.FloorCobble, Hotkey: rl.KeySix, Color: rl.NewColor(168, 156, 130, 255)},
		{Name: "Planks (w)", Char: core.FloorPlank, Hotkey: rl.KeySeven, Color: rl.NewColor(150, 102, 60, 255)},
		{Name: "Water (~)", Char: core.FloorWater, Hotkey: rl.KeyEight, Color: rl.NewColor(86, 142, 196, 255)},
		{Name: "Sand (n)", Char: core.FloorSand, Hotkey: rl.KeyNine, Color: rl.NewColor(228, 206, 158, 255)},
		{Name: "Snow (i)", Char: core.FloorSnow, Color: rl.NewColor(232, 240, 248, 255)},
	},
	LayerDecor: {
		{Name: "Auto", Char: core.DecorAuto, Hotkey: rl.KeyOne, Color: rl.NewColor(220, 224, 200, 255)},
		{Name: "Force empty (_)", Char: core.DecorEmpty, Hotkey: rl.KeyTwo, Color: rl.NewColor(60, 64, 70, 255)},
		{Name: "Bush (b)", Char: core.DecorBush, Hotkey: rl.KeyThree, Color: rl.NewColor(112, 142, 70, 255)},
		{Name: "Mushroom (m)", Char: core.DecorMushroom, Hotkey: rl.KeyFour, Color: rl.NewColor(220, 100, 110, 255)},
		{Name: "Pebble (p)", Char: core.DecorPebble, Hotkey: rl.KeyFive, Color: rl.NewColor(200, 192, 178, 255)},
		{Name: "Tall grass (,)", Char: core.DecorTallGrass, Hotkey: rl.KeySix, Color: rl.NewColor(150, 196, 110, 255)},
		{Name: "Flowers (f)", Char: core.DecorFlowers, Hotkey: rl.KeySeven, Color: rl.NewColor(236, 168, 196, 255)},
		{Name: "Clover (v)", Char: core.DecorClover, Hotkey: rl.KeyEight, Color: rl.NewColor(124, 186, 102, 255)},
		{Name: "Reeds (r)", Char: core.DecorReeds, Hotkey: rl.KeyNine, Color: rl.NewColor(110, 132, 90, 255)},
		{Name: "Bones (o)", Char: core.DecorBones, Color: rl.NewColor(228, 220, 198, 255)},
		{Name: "Scorch (x)", Char: core.DecorScorch, Color: rl.NewColor(44, 38, 36, 255)},
		{Name: "Blood (!)", Char: core.DecorBlood, Color: rl.NewColor(124, 38, 36, 255)},
		{Name: "Cobweb (*)", Char: core.DecorCobweb, Color: rl.NewColor(220, 222, 226, 255)},
		{Name: "Stump (t)", Char: core.DecorStump, Color: rl.NewColor(132, 92, 56, 255)},
		{Name: "Log (l)", Char: core.DecorLog, Color: rl.NewColor(118, 84, 52, 255)},
		{Name: "Leaf pile (L)", Char: core.DecorLeafPile, Color: rl.NewColor(196, 142, 80, 255)},
	},
	LayerProps: {
		{Name: "None (erase)", Char: core.TilePropEmpty, Hotkey: rl.KeyOne, Color: rl.NewColor(60, 64, 70, 255)},
		{Name: "Tree (T)", Char: core.TileTree, Hotkey: rl.KeyTwo, Color: rl.NewColor(64, 140, 80, 255)},
		{Name: "Tree XL (X)", Char: core.TileTreeXL, Hotkey: rl.KeyThree, Color: rl.NewColor(36, 96, 56, 255)},
		{Name: "Boulder (O)", Char: core.TileRockLarge, Hotkey: rl.KeyFour, Color: rl.NewColor(132, 110, 90, 255)},
		{Name: "Bush large (B)", Char: core.TileBushLarge, Hotkey: rl.KeyFive, Color: rl.NewColor(112, 142, 70, 255)},
		{Name: "Crate (C)", Char: core.TileCrate, Hotkey: rl.KeySix, Color: rl.NewColor(178, 130, 78, 255)},
		{Name: "Barrel (R)", Char: core.TileBarrel, Hotkey: rl.KeySeven, Color: rl.NewColor(166, 116, 70, 255)},
		{Name: "Urn (U)", Char: core.TileUrn, Hotkey: rl.KeyEight, Color: rl.NewColor(196, 122, 80, 255)},
		{Name: "Stalagmite (S)", Char: core.TileStalagmite, Hotkey: rl.KeyNine, Color: rl.NewColor(216, 210, 196, 255)},
		{Name: "Pillar (P)", Char: core.TilePillar, Color: rl.NewColor(220, 214, 198, 255)},
		{Name: "Broken pillar (I)", Char: core.TileBrokenPillar, Color: rl.NewColor(196, 188, 170, 255)},
		{Name: "Statue (M)", Char: core.TileStatue, Color: rl.NewColor(228, 222, 206, 255)},
		{Name: "Obelisk (Q)", Char: core.TileObelisk, Color: rl.NewColor(92, 96, 110, 255)},
		{Name: "Fountain (F)", Char: core.TileFountain, Color: rl.NewColor(100, 168, 222, 255)},
	},
	LayerEntities: {
		{Name: "Player Start", Entity: entityPlayerStart, Hotkey: rl.KeyOne, Color: rl.NewColor(255, 220, 124, 255)},
		{Name: "Add Rat", Entity: entityAddRat, Hotkey: rl.KeyTwo, Color: rl.NewColor(220, 156, 96, 255)},
		{Name: "Add Bat", Entity: entityAddBat, Hotkey: rl.KeyThree, Color: rl.NewColor(160, 130, 220, 255)},
		{Name: "Add Diseased Rat", Entity: entityAddDiseasedRat, Hotkey: rl.KeyFour, Color: rl.NewColor(140, 200, 90, 255)},
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
	dragPack
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
	// baseline is a snapshot of the area as last loaded from / saved to disk.
	// Used by undo/redo to detect "the working state now matches what's on
	// disk" so the dirty marker can clear instead of latching forever.
	baseline core.AreaDefinition

	statusLog []statusEntry

	brushSize int

	drag             dragKind
	dragSnapshotDone bool
	lastPaintX       int
	lastPaintZ       int
	rectAnchorX      int
	rectAnchorZ      int
	dragPackIdx      int

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

	// previewPhase is the day/night phase the author wants to drop into
	// when playtesting. Cycled with T; consumed by PreviewStepCount() to
	// seed g.StepCount on F5 so the editor can author tile palettes that
	// only read correctly at e.g. Dusk without playing a whole loop in.
	previewPhase core.TimeOfDay

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

// PreviewStepCount returns the StepCount value that places the player at
// the start of the editor's currently-selected preview phase. Used by the
// run loop on F5 so the playtest opens in the same lighting the author
// was previewing.
func (s State) PreviewStepCount() int {
	return int(s.previewPhase) * core.StepsPerPhase
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
		baseline:     cloneArea(a),
		layer:        LayerWalls,
		brushSize:    1,
		zoom:         1,
		gridCursorX:  -1,
		gridCursorZ:  -1,
		hoverX:       -1,
		hoverZ:       -1,
		dragPackIdx: -1,
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
				wb[x] = core.TileOpen
			}
		}
		walls[z] = string(wb)
		floor[z] = blankRow(w, core.FloorAuto)
		decor[z] = blankRow(w, core.DecorAuto)
		props[z] = blankRow(w, core.TilePropEmpty)
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
	if core.IsPropChar(a.Props[a.StartTileZ][a.StartTileX]) {
		return false
	}
	return true
}

// statusLogLifetime is the seed duration for a flash() entry's timer.
// drawStatus normalizes alpha against this same value so a fresh entry
// renders at alpha=1 and decays linearly to 0 across the lifetime.
const statusLogLifetime = float32(2.5)

// statusLogMaxEntries caps how many transient messages can stack at once.
const statusLogMaxEntries = 4

// flash pushes a transient message onto the rolling status log.
func (s *State) flash(msg string) {
	s.statusLog = append(s.statusLog, statusEntry{msg: msg, timer: statusLogLifetime})
	if len(s.statusLog) > statusLogMaxEntries {
		s.statusLog = s.statusLog[len(s.statusLog)-statusLogMaxEntries:]
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

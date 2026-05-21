// Package editor is the in-game map authoring tool. Maps are stored as
// five parallel ASCII layers (walls / floor / decor / props / ceiling)
// plus a list of entities (player start, enemy spawns, chests). The
// editor lets the user select an active layer and paint into it with
// layer-specific brushes.
package editor

import (
	"crawler/internal/app/core"
	"crawler/internal/app/render"

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
	LayerCeiling
	LayerEntities

	layerCount = 6
)

// Brush is one entry in a layer's palette. For grid layers, char is the
// byte written into that layer at the painted cell. For LayerEntities,
// Entity names which placement tool fires on click; EnemyKind carries
// the kind to add when Entity == entityAddEnemy.
type Brush struct {
	Name      string
	Char      byte
	Entity    entityKind
	EnemyKind core.EnemyKind // only meaningful when Entity == entityAddEnemy
	Hotkey    int32
	Color     rl.Color
}

type entityKind int

const (
	entityNone entityKind = iota
	entityPlayerStart
	// entityAddEnemy appends a member to the pack at the clicked tile,
	// creating a fresh pack if none is there yet. The specific kind to
	// add lives on Brush.EnemyKind so a single entityKind value handles
	// every enemy in core.EnemyKinds() — adding a new enemy is one row
	// in core/enemies.go's enemyDefinitions and the brush list +
	// packAddRules pick it up automatically. Right-click clears the
	// entire pack.
	entityAddEnemy
	// entityPlaceChest drops a chest at the clicked tile with the default
	// starter loot (one of every defined item kind). Right-click clears
	// the chest. Per-chest loot authoring is reserved for a future modal;
	// the brush gets the chest on the map and that's enough to test the
	// in-game open flow.
	entityPlaceChest
	// entityPlaceDoor drops an area-transition door at the clicked tile
	// with a placeholder name and "self" target. The clicked tile must
	// have an open walls/props cell — doors live on walkable floor.
	// Per-door authoring (rename, set target_map / target_door / facing)
	// happens in the modalDoorEdit modal opened by clicking an existing
	// door. Right-click clears the door.
	entityPlaceDoor
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
		{Name: "Auto", Char: core.FloorAuto, Hotkey: rl.KeyOne, Color: floorAutoColor},
		{Name: "Grass (g)", Char: core.FloorGrass, Hotkey: rl.KeyTwo, Color: rl.NewColor(120, 184, 110, 255)},
		{Name: "Dirt (d)", Char: core.FloorDirt, Hotkey: rl.KeyThree, Color: rl.NewColor(168, 132, 92, 255)},
		{Name: "Dark grass (k)", Char: core.FloorDarkGrass, Hotkey: rl.KeyFour, Color: rl.NewColor(72, 116, 70, 255)},
		{Name: "Stone (s)", Char: core.FloorStone, Hotkey: rl.KeyFive, Color: rl.NewColor(150, 148, 142, 255)},
		{Name: "Cobble (c)", Char: core.FloorCobble, Hotkey: rl.KeySix, Color: rl.NewColor(168, 156, 130, 255)},
		{Name: "Planks (w)", Char: core.FloorPlank, Hotkey: rl.KeySeven, Color: rl.NewColor(150, 102, 60, 255)},
		{Name: "Water (~)", Char: core.FloorWater, Hotkey: rl.KeyEight, Color: rl.NewColor(150, 204, 232, 255)},
		{Name: "Deep water (W)", Char: core.FloorDeepWater, Color: rl.NewColor(30, 60, 102, 255)},
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
		{Name: "Arch left (A)", Char: core.DecorArchway, Color: rl.NewColor(204, 196, 174, 255)},
		{Name: "Arch right (a)", Char: core.DecorArchwayTail, Color: rl.NewColor(184, 176, 154, 255)},
		{Name: "Lilypad (y)", Char: core.DecorLilypad, Color: rl.NewColor(96, 168, 100, 255)},
		{Name: "Rug (u)", Char: core.DecorRug, Color: rl.NewColor(176, 84, 68, 255)},
		{Name: "Candle (c)", Char: core.DecorCandle, Color: rl.NewColor(244, 220, 156, 255)},
		{Name: "Boot prints (i)", Char: core.DecorBootprints, Color: rl.NewColor(90, 68, 44, 255)},
		{Name: "Ash heap (h)", Char: core.DecorAshHeap, Color: rl.NewColor(132, 124, 116, 255)},
		{Name: "Puddle (q)", Char: core.DecorPuddle, Color: rl.NewColor(108, 154, 188, 255)},
		{Name: "Roots (k)", Char: core.DecorRootCluster, Color: rl.NewColor(92, 68, 44, 255)},
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
		{Name: "Rock cairn (K)", Char: core.TileRockCairn, Color: rl.NewColor(150, 138, 116, 255)},
		{Name: "Rock formation (J)", Char: core.TileRockFormation, Color: rl.NewColor(118, 102, 86, 255)},
		{Name: "Formation tail (j)", Char: core.TileRockFormationTail, Color: rl.NewColor(96, 84, 72, 255)},
		// Outdoor batch.
		{Name: "Well (W)", Char: core.TileWell, Color: rl.NewColor(132, 138, 142, 255)},
		{Name: "Gravestone (G)", Char: core.TileGravestone, Color: rl.NewColor(168, 162, 152, 255)},
		{Name: "Sign post (N)", Char: core.TileSignPost, Color: rl.NewColor(160, 110, 64, 255)},
		{Name: "Hay bale (H)", Char: core.TileHayBale, Color: rl.NewColor(216, 184, 110, 255)},
		{Name: "Scarecrow (Y)", Char: core.TileScarecrow, Color: rl.NewColor(196, 162, 96, 255)},
		// Dungeon interior batch.
		{Name: "Bookshelf (V)", Char: core.TileBookshelf, Color: rl.NewColor(132, 90, 56, 255)},
		{Name: "Table (E)", Char: core.TileTable, Color: rl.NewColor(160, 116, 72, 255)},
		{Name: "Bed (D)", Char: core.TileBed, Color: rl.NewColor(176, 90, 96, 255)},
		{Name: "Brazier (Z)", Char: core.TileBrazier, Color: rl.NewColor(220, 132, 64, 255)},
		{Name: "Sarcophagus (A)", Char: core.TileSarcophagus, Color: rl.NewColor(200, 192, 174, 255)},
	},
	LayerCeiling: {
		{Name: "Solid (#)", Char: core.TileCeilingSolid, Hotkey: rl.KeyOne, Color: rl.NewColor(110, 96, 80, 255)},
		{Name: "Open (.)", Char: core.TileCeilingOpen, Hotkey: rl.KeyTwo, Color: rl.NewColor(86, 142, 196, 255)},
	},
	LayerEntities: buildEntityBrushes(),
}

// entityBrushHotkeys is the positional hotkey pool for enemy brushes on
// LayerEntities. Slot 0 is reserved for Player Start (key 1); enemies
// take slots 1..len-1 (keys 2..N). Past pool length, brushes get no
// hotkey (mouse-only) — matching the convention on other layers.
var entityBrushHotkeys = []int32{rl.KeyTwo, rl.KeyThree, rl.KeyFour, rl.KeyFive, rl.KeySix, rl.KeySeven, rl.KeyEight}

// entityBrushColors is the per-enemy swatch tint. Falls back to a
// neutral grey if a future kind isn't in the map — the swatch still
// renders, just unstyled. Hand-tuned to keep adjacent foes visually
// distinct on the grid.
var entityBrushColors = map[core.EnemyKind]rl.Color{
	core.EnemyRat:         rl.NewColor(220, 156, 96, 255),
	core.EnemyBat:         rl.NewColor(160, 130, 220, 255),
	core.EnemyDiseasedRat: rl.NewColor(140, 200, 90, 255),
	core.EnemyGoblin:      rl.NewColor(132, 196, 110, 255),
	core.EnemyGoblinMage:  rl.NewColor(220, 168, 244, 255),
	core.EnemyAmoeba:      rl.NewColor(180, 200, 220, 255),
	core.EnemyVenusMantrap: rl.NewColor(220, 124, 158, 255),
}

// init asserts entityBrushColors covers every enemy kind — without
// this guard, a new EnemyKind added to core silently renders with the
// neutral-grey fallback swatch and the author can't tell at a glance
// which enemy the brush represents. Same pattern as the minimap
// coverage check in render/minimap.go.
func init() {
	for _, def := range core.EnemyKinds() {
		if _, ok := entityBrushColors[def.Kind]; !ok {
			panic("editor: missing entityBrushColors entry for " + def.Name)
		}
	}
}

// buildEntityBrushes assembles the LayerEntities palette: Player Start,
// one brush per registered EnemyKind, then Place Chest. Driven by
// core.EnemyKinds() so adding a new enemy is one row in core's
// enemyDefinitions — the editor brush picks it up automatically.
// Hotkey 1 is Player Start; enemies take 2..N from entityBrushHotkeys;
// Place Chest takes the next free hotkey.
func buildEntityBrushes() []Brush {
	brushes := []Brush{
		{Name: "Player Start", Entity: entityPlayerStart, Hotkey: rl.KeyOne, Color: render.MarkerStart},
	}
	defs := core.EnemyKinds()
	for i, def := range defs {
		hk := int32(0)
		if i < len(entityBrushHotkeys) {
			hk = entityBrushHotkeys[i]
		}
		col, ok := entityBrushColors[def.Kind]
		if !ok {
			col = rl.NewColor(180, 180, 180, 255)
		}
		brushes = append(brushes, Brush{
			Name:      "Add " + def.SingularName,
			Entity:    entityAddEnemy,
			EnemyKind: def.Kind,
			Hotkey:    hk,
			Color:     col,
		})
	}
	chestHK := int32(0)
	if slot := len(defs) + 1; slot-1 < len(entityBrushHotkeys) {
		chestHK = entityBrushHotkeys[slot-1]
	}
	brushes = append(brushes, Brush{
		Name:   "Place Chest",
		Entity: entityPlaceChest,
		Hotkey: chestHK,
		Color:  render.MarkerChest,
	})
	doorHK := int32(0)
	if slot := len(defs) + 2; slot-1 < len(entityBrushHotkeys) {
		doorHK = entityBrushHotkeys[slot-1]
	}
	brushes = append(brushes, Brush{
		Name:   "Place Door",
		Entity: entityPlaceDoor,
		Hotkey: doorHK,
		Color:  render.MarkerDoor,
	})
	return brushes
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
	case LayerCeiling:
		return "Ceiling"
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
	// Door-edit text-field foci. Switch with Tab inside modalDoorEdit;
	// updateTextInput dispatches the keystrokes onto the right field of
	// the active DoorSpawn via activeTextTarget.
	focusDoorName
	focusDoorTargetMap
	focusDoorTargetDoor
)

type modalKind int

const (
	modalNone modalKind = iota
	modalOpen
	modalSaveAs
	modalConfirmDirty
	// modalPackEdit displays the inline pack editor for a clicked pack
	// on the Entities layer: list members with × to remove, ▲/▼ to
	// reorder, and a + row to add new members. Anchored over the pack's
	// tile. modalPackIdx holds the index into area.PackSpawns; if the
	// pack gets dropped while the modal is open, the modal closes.
	modalPackEdit
	// modalChestEdit is the inline chest editor — analogous to
	// modalPackEdit but for chests. Lists the chest's authored items
	// with X to remove, ▲/▼ to reorder, and one-key shortcuts to append
	// a new item kind from chestAddRules. Anchored over the chest's
	// tile. modalChestIdx holds the area.ChestSpawns index; the modal
	// closes if the chest gets dropped while open.
	modalChestEdit
	// modalSounds is the in-editor sound creator. Lets the author
	// synthesize a cue from sliders (sweep params), preview it, save it
	// to maps/sounds/<name>.wav, delete a saved cue, and assign a saved
	// cue to any of the six built-in audio.Sound entries.
	modalSounds
	// modalDoorEdit is the inline door editor opened by clicking an
	// existing door on the Entities layer. Lets the author rename the
	// door, set its target_map / target_door (cross-map links), pick
	// the post-transition facing, and delete it. Without this modal,
	// doors are effectively unusable for cross-map travel — placement
	// only stamps a "door_N" placeholder with target=self.
	modalDoorEdit
	// modalValidate is the cross-map door / reachability report modal
	// opened from the topbar Validate button. Read-only summary of
	// every reachabilityWarning and crossMapDoorWarning the current
	// area carries — same data the metadata badge surfaces, but
	// uncapped so the author can see the full list at once.
	modalValidate
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
	// paletteScroll is the per-layer vertical scroll offset (in pixels)
	// applied to the brush entries. Adjusted by mouse-wheel when the
	// pointer is over the palette. Clamped in drawPalette so the bottom
	// entry never overshoots the visible area. Stored per-layer so
	// switching tabs doesn't reset what was off-screen.
	paletteScroll [layerCount]float32

	focus      focusField
	numericBuf string

	modal              modalKind
	modalPaths         []string
	modalCursor        int
	modalFilename      string
	modalRenaming      string
	modalConfirmDelete bool
	// modalPackIdx is the area.PackSpawns index currently being edited
	// when modal == modalPackEdit. -1 outside the pack-edit flow.
	modalPackIdx int
	// modalChestIdx is the area.ChestSpawns index currently being
	// edited when modal == modalChestEdit. -1 outside the chest-edit
	// flow. Mirror of modalPackIdx.
	modalChestIdx int
	// modalDoorIdx is the area.DoorSpawns index being edited when
	// modal == modalDoorEdit. -1 outside the flow.
	modalDoorIdx int
	// modalValidateRows is the snapshot of warnings shown in
	// modalValidate. Captured at open time so the read-only display
	// doesn't reflow while the user is reading it.
	modalValidateRows []string
	// Sound modal state. Lives on State so the slider positions and
	// last-edited name survive open/close of the modal — closing
	// shouldn't reset the user's tuning work.
	soundParams    soundParamSet
	soundName      string
	soundCursor    int // row cursor inside the sound modal
	soundLeftPanel int // 0 = synth params, 1 = saved-sound list, 2 = cue assignments
	// soundSavedCache holds the result of audio.ListUserSounds() for
	// the current Update→Draw frame so we don't ReadDir twice per frame
	// while the modal is open. Refreshed by updateSoundsModal at the
	// top of each frame; drawSoundsModal reads it back.
	soundSavedCache []string
	pending         pendingAction

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

	// Ctrl+F5 "test from cursor" override: when testStartOverride is true,
	// the run loop consumes testStartOverrideX/Z as the playtest's
	// starting tile instead of area.StartTileX/Z. Reset by the run loop
	// after it builds the GameState so the next test reverts to the
	// authored start.
	testStartOverride  bool
	testStartOverrideX int
	testStartOverrideZ int

	// previewPhase is the day/night phase the author wants to drop into
	// when playtesting. Cycled with T; consumed by PreviewStepCount() to
	// seed g.StepCount on F5 so the editor can author tile palettes that
	// only read correctly at e.g. Dusk without playing a whole loop in.
	previewPhase core.TimeOfDay

	rect layoutRect
}

type layoutRect struct {
	topbar    rl.Rectangle
	layerTabs rl.Rectangle
	palette   rl.Rectangle
	metadata  rl.Rectangle
	grid      rl.Rectangle
	cellPx    float32
	gridX     float32
	gridY     float32
	gridW     float32
	gridH     float32
}

// Area returns a copy of the area currently being edited. Used by the run
// loop's F5 playtest path to spin up a GameState from in-memory edits.
func (s State) Area() core.AreaDefinition {
	return cloneArea(s.area)
}

// ReachabilityWarnings runs reachabilityWarnings against the current
// area state and returns the result. The check is a single flood-fill
// (a few hundred cells on a typical authored map) so we can afford to
// compute it per-frame rather than caching across edits — the metadata
// panel calls this once per draw to render the warnings badge.
func (s *State) ReachabilityWarnings() []string {
	return reachabilityWarnings(s.area)
}

// PreviewStepCount returns the StepCount value that places the player at
// the start of the editor's currently-selected preview phase. Used by the
// run loop on F5 so the playtest opens in the same lighting the author
// was previewing.
func (s State) PreviewStepCount() int {
	return int(s.previewPhase) * core.StepsPerPhase
}

// TestStartOverride returns (x, z, true) when the editor's last
// ActionTest came from a Ctrl+F5 "test-from-cursor" press, telling the
// run loop to override the playtest's starting tile. Returns (_, _,
// false) otherwise so the authored area.StartTile is honored. Consumed
// by the run loop and reset by ClearTestStartOverride below.
func (s State) TestStartOverride() (int, int, bool) {
	return s.testStartOverrideX, s.testStartOverrideZ, s.testStartOverride
}

// ClearTestStartOverride drops the one-shot test-from-cursor override
// after the run loop has consumed it. Subsequent F5 presses revert to
// the authored start.
func (s *State) ClearTestStartOverride() {
	s.testStartOverride = false
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
		area:          a,
		baseline:      cloneArea(a),
		layer:         LayerWalls,
		brushSize:     1,
		zoom:          1,
		gridCursorX:   -1,
		gridCursorZ:   -1,
		hoverX:        -1,
		hoverZ:        -1,
		dragPackIdx:   -1,
		modalPackIdx:  -1,
		modalChestIdx: -1,
		modalDoorIdx:  -1,
	}
}

func blankArea(w, h int) core.AreaDefinition {
	walls := make([]string, h)
	floor := make([]string, h)
	decor := make([]string, h)
	props := make([]string, h)
	ceiling := make([]string, h)
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
		ceiling[z] = blankRow(w, core.TileCeilingOpen)
	}
	return core.AreaDefinition{
		Name:         "Untitled",
		Width:        w,
		Height:       h,
		Walls:        walls,
		Floor:        floor,
		Decor:        decor,
		Props:        props,
		Ceiling:      ceiling,
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
		// Cross-map door validation runs once at the F5 gate (loads
		// every referenced .map from disk, so per-frame would be
		// expensive). Surfaces dangling target_map / target_door
		// references that the per-frame check can't catch. Doesn't
		// block the playtest — the runtime tolerates broken doors
		// (the transition fails with a quiet error) — just informs
		// the author so they can fix before shipping.
		for _, w := range crossMapDoorWarnings(s.area) {
			s.flash("Doors: " + w)
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
	// BlockedAt covers walls, blocking props, AND deep water — any of
	// those at the start tile would soft-lock the player on spawn.
	// Routes through one helper so a future blocker (e.g. lava) lands
	// the playtest check automatically.
	if a.BlockedAt(a.StartTileX, a.StartTileZ) {
		return false
	}
	// A chest at the start tile gets silently dropped by core.placeChests
	// at runtime — refuse the playtest so the author sees the data-loss
	// problem instead of wondering where the chest went. Editor's
	// placeChestAt already refuses this configuration; this catches
	// legacy / hand-edited .map files.
	if core.ChestSpawnIndexAt(a.ChestSpawns, a.StartTileX, a.StartTileZ) >= 0 {
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

// flash pushes a transient message onto the rolling status log. If the
// same message is already on the log (e.g. typing in the numeric resize
// input below MinMapDimension fires a "too small" flash on every digit),
// refresh its timer in place instead of stacking duplicate rows. Keeps
// the log readable when the same validation error fires repeatedly.
func (s *State) flash(msg string) {
	for i, e := range s.statusLog {
		if e.msg == msg {
			s.statusLog[i].timer = statusLogLifetime
			return
		}
	}
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

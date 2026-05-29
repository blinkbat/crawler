// Package editor is the in-game map authoring tool. Maps are stored as
// five parallel ASCII layers (walls / floor / decor / props / ceiling)
// plus a list of entities (player start, enemy spawns, chests). The
// editor lets the user select an active layer and paint into it with
// layer-specific brushes.
package editor

import (
	"crawler/internal/app/core"
	"crawler/internal/app/render"
	"fmt"

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

// numberRowKeys is the top-row 1..9 key codes. Package-level so the
// brush-select hotkeys (1..9 / Shift+1..9, all nine) and the Alt+1..6
// layer jump (first layerCount) don't rebuild the slice every input
// frame. The first six double as the per-layer jump keys.
var numberRowKeys = [9]int32{
	rl.KeyOne, rl.KeyTwo, rl.KeyThree, rl.KeyFour, rl.KeyFive,
	rl.KeySix, rl.KeySeven, rl.KeyEight, rl.KeyNine,
}

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
	// entityClear erases any pack member, chest, or door at the clicked
	// tile (the player start is anchored and not cleared by this brush).
	// Mirrors right-click on the Entities layer but exposes the action
	// as a first-class brush so authors can stay in left-click mode
	// when they're doing a series of removals.
	entityClear
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
// paletteLabels mirrors layerBrushes with the pre-formatted display
// label ("1 Wall (#)", "2 Open (.)", ...) for each entry. Built once
// at package init so the palette draw loop doesn't fmt.Sprintf the
// same N strings every frame — palette content is static between
// layer-table edits, so caching them here is safe.
var paletteLabels [layerCount][]string

func init() {
	for layer, brushes := range layerBrushes {
		labels := make([]string, len(brushes))
		for i, b := range brushes {
			labels[i] = fmt.Sprintf("%d %s", i+1, b.Name)
		}
		paletteLabels[layer] = labels
	}
}

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
		{Name: "Force empty (_)", Char: core.DecorEmpty, Hotkey: rl.KeyTwo, Color: clearBrushColor},
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
		{Name: "None (erase)", Char: core.TilePropEmpty, Hotkey: rl.KeyOne, Color: clearBrushColor},
		{Name: "Tree (T)", Char: core.TileTree, Hotkey: rl.KeyTwo, Color: rl.NewColor(64, 140, 80, 255)},
		{Name: "Tree XL (X)", Char: core.TileTreeXL, Hotkey: rl.KeyThree, Color: rl.NewColor(36, 96, 56, 255)},
		{Name: "Tall Tree (|)", Char: core.TileTreeTall, Color: rl.NewColor(52, 118, 64, 255)},
		{Name: "Twin Trees (@)", Char: core.TileTreeTwin, Color: rl.NewColor(58, 130, 72, 255)},
		{Name: "Young Tree (/)", Char: core.TileTreeYoung, Color: rl.NewColor(96, 168, 102, 255)},
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
		{Name: "Wall Torch (z)", Char: core.TileTorch, Color: rl.NewColor(240, 168, 96, 255)},
		{Name: "Sarcophagus (A)", Char: core.TileSarcophagus, Color: rl.NewColor(200, 192, 174, 255)},
	},
	LayerCeiling: {
		{Name: "Solid (#)", Char: core.TileCeilingSolid, Hotkey: rl.KeyOne, Color: rl.NewColor(110, 96, 80, 255)},
		{Name: "Open (.)", Char: core.TileCeilingOpen, Hotkey: rl.KeyTwo, Color: rl.NewColor(86, 142, 196, 255)},
	},
	LayerEntities: buildEntityBrushes(),
}

// entityBrushHotkeys is the positional hotkey pool for enemy brushes on
// LayerEntities. Keys 1 and 2 are reserved for the Clear and Player
// Start brushes respectively; enemies take Key3..KeyN. Past pool
// length, brushes get no hotkey (mouse-only) — matching the convention
// on other layers.
var entityBrushHotkeys = []int32{rl.KeyThree, rl.KeyFour, rl.KeyFive, rl.KeySix, rl.KeySeven, rl.KeyEight, rl.KeyNine}

// entityBrushColors is the per-enemy swatch tint. Falls back to a
// neutral grey if a future kind isn't in the map — the swatch still
// renders, just unstyled. Hand-tuned to keep adjacent foes visually
// distinct on the grid.
var entityBrushColors = map[core.EnemyKind]rl.Color{
	core.EnemyRat:          rl.NewColor(220, 156, 96, 255),
	core.EnemyBat:          rl.NewColor(160, 130, 220, 255),
	core.EnemyDiseasedRat:  rl.NewColor(140, 200, 90, 255),
	core.EnemyGoblin:       rl.NewColor(132, 196, 110, 255),
	core.EnemyGoblinMage:   rl.NewColor(220, 168, 244, 255),
	core.EnemyAmoeba:       rl.NewColor(180, 200, 220, 255),
	core.EnemyVenusMantrap: rl.NewColor(220, 124, 158, 255),
	// Roster expansion. Distinct hues so the editor swatch doesn't
	// blur with the existing set at a glance.
	core.EnemyCaveSpider:  rl.NewColor(96, 60, 110, 255),   // deep purple — webby/venomous
	core.EnemyVampireBat:  rl.NewColor(200, 70, 80, 255),   // crimson — blood drain identity
	core.EnemyWisp:        rl.NewColor(180, 220, 255, 255), // cold ghostly blue
	core.EnemyStoneGolem:  rl.NewColor(120, 116, 108, 255), // weathered stone
	core.EnemyNecromancer: rl.NewColor(76, 84, 130, 255),   // robed shadow indigo
	core.EnemySkeleton:    rl.NewColor(230, 226, 198, 255), // pale bone
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
		{Name: "Clear", Entity: entityClear, Hotkey: rl.KeyOne, Color: clearBrushColor},
		{Name: "Player Start", Entity: entityPlayerStart, Hotkey: rl.KeyTwo, Color: render.MarkerStart},
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

// layerDisplayNames is the Layer→label table powering layerName.
// Indexed by Layer so a new layer added to the enum must extend this
// array, and the init below catches a length mismatch at startup —
// same lockstep guard the sibling switches in applyTool / eraseAt /
// activeGrid already panic on at a missing case.
var layerDisplayNames = [layerCount]string{
	LayerWalls:    "Walls",
	LayerFloor:    "Floor",
	LayerDecor:    "Decor",
	LayerProps:    "Props",
	LayerCeiling:  "Ceiling",
	LayerEntities: "Entities",
}

func layerName(l Layer) string {
	if int(l) < 0 || int(l) >= len(layerDisplayNames) {
		panic("editor: layerName called with unhandled Layer — add it to layerDisplayNames")
	}
	return layerDisplayNames[l]
}

type focusField int

const (
	focusNone focusField = iota
	focusName
	focusQuiet
	focusFilename
	focusWidth
	focusHeight
	// New-map dialog text fields. Switch with Tab inside modalNew; the
	// committed width / height live on State.modalNewWidth/Height (not
	// the area, since the area hasn't been replaced yet at edit time).
	focusNewWidth
	focusNewHeight
	// focusCustomEnemyName is the rename text field on the custom-
	// enemy edit form (modalCustomEnemies). Routes through the same
	// pumpPrintableASCII helper the door-name and area-name fields use.
	focusCustomEnemyName
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
	// modalNew is the "New map" setup dialog. Lets the author pick a
	// starting width / height and the default floor tile that fills the
	// blank interior before the area is replaced. Opens from Ctrl+N /
	// the topbar New button (after a confirmDirty bounce if the current
	// area has unsaved edits).
	modalNew
	// modalCustomEnemies is the per-map custom-enemy authoring modal.
	// CRUD for AreaDefinition.CustomEnemies: pick a base sprite, set stats,
	// toggle skills, then add them to authored packs from the pack modal.
	modalCustomEnemies
	// modalEscMenu is the pause-style menu opened by pressing Esc on
	// the editor canvas. Bridges the gap between "Esc = exit to title"
	// (jarring on fullscreen, especially when the author wants to
	// drop to Windowed without quitting) and the runtime's pause menu
	// in adventure mode. Offers Display toggle, Continue, and Exit
	// (which still bounces through modalConfirmDirty if the area has
	// unsaved edits).
	modalEscMenu
	// modalCount is the count sentinel for the modalKind enum — used by
	// the modalHandlers init assert in draw.go to walk every legal
	// value and confirm the dispatch table is complete. Keep this row
	// at the END of the enum so iota arithmetic is "count = last + 1."
	// Not a dispatchable modal — modalHandlers must NOT have a row
	// for it.
	modalCount
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
	// metadataScroll is the vertical scroll offset (in pixels) applied
	// to the right-hand MAP panel so its full layout (name, materials,
	// quiet message, dimensions, start, facing, path, reachability) can
	// be reached on shorter windows. Adjusted by mouse-wheel when the
	// pointer is over the metadata panel and clamped by ScrollMetadata.
	metadataScroll float32

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
	// New-map dialog state. modalNewWidth / modalNewHeight hold the
	// in-progress dimensions (text-input commits write here, not the
	// area); modalNewFloor is the chosen default floor char that
	// blankArea will fill the interior with on confirm.
	modalNewWidth  int
	modalNewHeight int
	modalNewFloor  byte
	// modalCustomIdx is the AreaDefinition.CustomEnemies index of the
	// currently selected entry in the custom-enemies modal. -1 means
	// no selection (the form shows a "pick one" placeholder).
	modalCustomIdx int
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

	// hideTileGlyphs toggles the per-tile char overlay in the grid.
	// Default false → overlay is ON, since most authors want to see
	// what's where. ALT (tapped on release with no other key pressed
	// during the hold) flips it so the author can hide the glyphs to
	// inspect raw cell color or take a clean screenshot. altChordUsed
	// tracks whether ALT+something was pressed during the current Alt
	// hold; if so we suppress the toggle on release so Alt+1..6 (layer
	// jump) doesn't double-trigger the overlay flip.
	hideTileGlyphs bool
	altChordUsed   bool

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

	// contextMenu holds the in-flight right-click menu state. When open,
	// updateContextMenu absorbs the frame's clicks until the user picks
	// a row, clicks outside, or presses Esc — see context.go for the
	// row-build / dispatcher pair.
	contextMenu contextMenuState

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

// tileCorner returns the screen-space top-left corner of grid tile
// (tx, tz) under the current zoom/pan. The single source of truth for
// the "tile coord → screen" math: any future change to centering,
// zoom anchoring, or panel layout updates this one helper instead of
// rewriting 20+ open-coded `s.rect.gridX + float32(tx)*cell` sites.
func (r layoutRect) tileCorner(tx, tz int) (float32, float32) {
	return r.gridX + float32(tx)*r.cellPx, r.gridY + float32(tz)*r.cellPx
}

// tileCenter is tileCorner shifted by half a cell — gives the
// pixel-space center of a tile, which is what marker glyphs / arrows /
// pack badges anchor on.
func (r layoutRect) tileCenter(tx, tz int) (float32, float32) {
	return r.gridX + (float32(tx)+0.5)*r.cellPx, r.gridY + (float32(tz)+0.5)*r.cellPx
}

// tileRect returns a screen-space rl.Rectangle covering a single
// tile. Convenience wrapper around tileCorner for the common
// "DrawRectangleRec" pattern.
func (r layoutRect) tileRect(tx, tz int) rl.Rectangle {
	x, y := r.tileCorner(tx, tz)
	return rl.NewRectangle(x, y, r.cellPx, r.cellPx)
}

// Area returns a copy of the area currently being edited. Used by the run
// loop's F5 playtest path to spin up a GameState from in-memory edits.
func (s State) Area() core.AreaDefinition {
	return core.CloneArea(s.area)
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

// New starts the editor with a blank default-sized map filled with
// FloorAuto. Used by the run loop's "open the editor from the title
// screen" path; the in-editor New flow goes through modalNew so the
// author can pick the size and default floor up-front. Size routes
// through core.DefaultNewMapDimension so both entry points stay in
// lockstep on what a fresh map looks like.
func New() State {
	return freshState(blankArea(core.DefaultNewMapDimension, core.DefaultNewMapDimension, core.FloorAuto))
}

// NewFromArea opens the editor on an already-loaded area.
func NewFromArea(a core.AreaDefinition) State {
	return freshState(a)
}

func freshState(a core.AreaDefinition) State {
	return State{
		area:           a,
		baseline:       core.CloneArea(a),
		layer:          LayerWalls,
		brushSize:      1,
		zoom:           1,
		gridCursorX:    -1,
		gridCursorZ:    -1,
		hoverX:         -1,
		hoverZ:         -1,
		dragPackIdx:    -1,
		modalPackIdx:   -1,
		modalChestIdx:  -1,
		modalDoorIdx:   -1,
		modalCustomIdx: -1,
	}
}

func blankArea(w, h int, floorChar byte) core.AreaDefinition {
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
		floor[z] = blankRow(w, floorChar)
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
			// Click-outside-defocus needs to run the same finalization
			// the Enter/Tab paths do — the custom-enemy name field
			// can't drop focus on a duplicate without resolving the
			// name uniqueness, otherwise two defs share a mapfile
			// row key and the saver silently keeps only one.
			finalizeFocusedField(s)
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

	// Esc no longer exits the editor directly — it opens the editor
	// pause menu (modalEscMenu) where the author can choose to flip
	// display mode, continue, or actually exit. The exitRequested
	// path (set by topbar button or other in-code triggers) still
	// fires the dirty-bounce-then-exit flow so external callers
	// don't have to know about the menu detour.
	if editorCancelPressed() {
		openModal(s, modalEscMenu)
		return ActionNone
	}
	if s.exitRequested {
		s.exitRequested = false
		if s.dirty {
			openConfirmDirtyModal(s, pendingExitToTitle)
			return ActionNone
		}
		return ActionExitToTitle
	}

	return ActionNone
}

// canPlaytest is the strict subset of reachability checks that MUST pass
// before we'll drop into adventure mode — anything that would crash or
// soft-lock the player on entry. Delegates to startTileBlocker so the
// playtest gate and the reachability warnings stay perfectly in sync;
// a future blocker (e.g. lava) added to BlockedAt lands both paths via
// one edit.
func canPlaytest(a core.AreaDefinition) bool {
	return startTileBlocker(a) == ""
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

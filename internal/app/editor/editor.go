// Package editor is the in-game map authoring tool. Maps are stored as
// six paintable grid layers (walls / floor / decor / props / ceiling /
// elevation) plus a list of entities (player start, enemy spawns, chests). The
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

// Layer names which of the area's six grid layers (walls / floor / decor /
// props / ceiling / elevation) — or the entity list — the editor is currently authoring.
// Active layer drives the palette, the click action, and the visual emphasis
// on the grid.
type Layer int

const (
	LayerWalls Layer = iota
	LayerFloor
	LayerDecor
	LayerProps
	LayerCeiling
	LayerElevation
	LayerEntities
)

// maxEditLevel is the highest elevation level the height selector reaches.
// Levels serialize as one base-36 char per cell ('0'..'9' then 'A'..'Z'), so
// the editor can stack up to core.MaxElevationLevel+1 floors.
const maxEditLevel = core.MaxElevationLevel

// layerCount is the number of editor layers (six grid layers + the
// entity list), derived from the enum's last value so adding a
// layer doesn't need a separate magic-number bump.
const layerCount = int(LayerEntities) + 1

// numberRowKeys is the top-row 1..9 key codes. Package-level so the
// brush-select hotkeys (1..9 / Shift+1..9, all nine) and the Alt+1..N
// layer jump (first layerCount = 7 keys) don't rebuild the slice every
// input frame. The first layerCount entries double as the per-layer jump
// keys — this assumes layerCount <= 9 (the number of numberRowKeys entries);
// a 10th layer would silently lose its Alt-jump key without growing this row.
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
	// in core/enemies.go's enemyDefinitions and both the brush list and
	// the pack editor's add-member dropdown pick it up automatically.
	// Right-click clears the entire pack.
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
	// entityPlaceCrystal drops a healing crystal (Grimrock-style save/heal
	// point) at the clicked tile. Crystals carry no per-instance authoring
	// (charge state is runtime), so there's no edit modal — placement is the
	// whole story. Right-click (or the Clear brush) removes it.
	entityPlaceCrystal
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
		// All walls stay in one muted grey FAMILY (built off wallSwatch) so the
		// editor canvas reads them as uniform structure — but each variant takes
		// a subtle tint so they're still tellable apart at a glance, not a
		// rainbow. The palette names carry the precise identity.
		{Name: "Wall (#)", Char: core.TileRock, Hotkey: rl.KeyOne, Color: wallSwatch},
		{Name: "Open (.)", Char: '.', Hotkey: rl.KeyTwo, Color: rl.NewColor(180, 168, 140, 255)},
		{Name: "Rock+Light Ivy (+)", Char: core.TileWallRockIvyLight, Hotkey: rl.KeyThree, Color: tintSwatch(wallSwatch, -8, 6, -10)},
		{Name: "Rock+Heavy Ivy (=)", Char: core.TileWallRockIvyHeavy, Hotkey: rl.KeyFour, Color: tintSwatch(wallSwatch, -20, 4, -22)},
		{Name: "Rock Cracked (&)", Char: core.TileWallRockCracked, Hotkey: rl.KeyFive, Color: tintSwatch(wallSwatch, -10, -10, -8)},
		{Name: "Rock Crumbling ($)", Char: core.TileWallRockCrumbling, Hotkey: rl.KeySix, Color: tintSwatch(wallSwatch, -2, -12, -22)},
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
		// Non-blocking decorative plants.
		{Name: "Exotic Flower (e)", Char: core.TilePropExoticFlower, Color: rl.NewColor(206, 110, 170, 255)},
		{Name: "Tall Fern (()", Char: core.TilePropTallFern, Color: rl.NewColor(90, 146, 86, 255)},
		{Name: "Tall Grass ())", Char: core.TilePropGrassTuft, Color: rl.NewColor(140, 178, 108, 255)},
	},
	LayerCeiling: {
		{Name: "Solid (#)", Char: core.TileCeilingSolid, Hotkey: rl.KeyOne, Color: rl.NewColor(110, 96, 80, 255)},
		{Name: "Open (.)", Char: core.TileCeilingOpen, Hotkey: rl.KeyTwo, Color: rl.NewColor(86, 142, 196, 255)},
	},
	// Elevation: a single "Set Height" brush stamps the height selector's
	// current level (activeBrush rewrites its Char to '0'+editLevel each
	// frame, so flood-fill + the brush preview pick the level up for free).
	// Ramp placement is the toolbar's Ramp tool-mode, not a brush.
	LayerElevation: {
		{Name: "Set Height", Char: 0, Hotkey: rl.KeyOne, Color: rl.NewColor(150, 140, 120, 255)},
	},
	LayerEntities: buildEntityBrushes(),
}

// entityBrushHotkeys is the positional hotkey pool for enemy brushes on
// LayerEntities. Keys 1 and 2 are reserved for the Clear and Player
// Start brushes respectively; enemies take Key3..KeyN. Past pool
// length, brushes get no hotkey (mouse-only) — matching the convention
// on other layers.
//
// Positional-by-palette-order is intentional here (the palette IS an ordered
// list, like every other layer's): these 3..9 keys select a brush WITHIN the
// active layer, the same generic number-row scheme every layer uses — not a
// per-kind mnemonic. (The pack-edit MODAL no longer has per-kind add-keys at
// all; it opens a dropdown — see dropdown.go.) enemyDefinitions is kept in
// EnemyKind enum order (asserted-adjacent in core), so the i-th palette enemy
// is deterministic; the keys are distinct constants that never collide with
// the reserved Key1/Key2.
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
			col = entityFallbackColor
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
	crystalHK := int32(0)
	if slot := len(defs) + 3; slot-1 < len(entityBrushHotkeys) {
		crystalHK = entityBrushHotkeys[slot-1]
	}
	brushes = append(brushes, Brush{
		Name:   "Place Crystal",
		Entity: entityPlaceCrystal,
		Hotkey: crystalHK,
		Color:  render.MarkerCrystal,
	})
	return brushes
}

// layerDisplayNames is the Layer→label table powering layerName.
// Indexed by Layer so a new layer added to the enum must extend this
// array, and the init below catches a length mismatch at startup —
// same lockstep guard the sibling switches in applyTool / eraseAt /
// activeGrid already panic on at a missing case.
var layerDisplayNames = [layerCount]string{
	LayerWalls:     "Walls",
	LayerFloor:     "Floor",
	LayerElevation: "Elevation",
	LayerDecor:     "Decor",
	LayerProps:     "Props",
	LayerCeiling:   "Ceiling",
	LayerEntities:  "Entities",
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
	// Door-edit text-field foci. Switch with Tab inside modalDoorEdit;
	// updateTextInput dispatches the keystrokes onto the right field of
	// the active DoorSpawn via activeTextTarget.
	focusDoorName
	focusDoorTargetMap
	focusDoorTargetDoor
	// Dialog-editor text-field foci. The node editor (modalDialogNodeEdit)
	// and choice editor (modalDialogChoiceEdit) pump these in their own
	// update loops via pumpFocusField, mirroring the door modal. Each maps
	// to a field of the active node/choice through dialogTextTarget.
	focusDialogNodeText
	focusDialogNodeNext
	focusDialogNodeContinue
	focusDialogChoiceLabel
	focusDialogChoiceNext
	// focusDialogActionID edits the quest-id / event-id of the action editor
	// (modalDialogActionEdit), routed to the right field by the action's kind
	// (see dialogActionIDTarget). Shared by the node- and choice-action editors.
	focusDialogActionID
	// Condition-editor foci (modalDialogCondEdit). String fields edit the
	// active condition in place via currentDialogCond; numeric fields route
	// through the shared dialogNumBuf (see dialogNumericTarget).
	focusDialogCondQuestID
	focusDialogCondMessage
	focusDialogCondGold
	focusDialogCondFoeKills
	focusDialogCondTileX
	focusDialogCondTileZ
	// Trigger-editor numeric foci (modalDialogTriggerEdit) — share dialogNumBuf
	// with the condition numerics (only one editor is open at a time).
	focusDialogTrigTileX
	focusDialogTrigTileZ
	focusDialogTrigFoeKills
)

type modalKind int

const (
	modalNone modalKind = iota
	modalOpen
	modalSaveAs
	modalConfirmDirty
	// modalPackEdit displays the inline pack editor for a clicked pack
	// on the Entities layer: list members with X to remove, K/J to
	// reorder, and an "+ Add member" dropdown (Enter) to add new members.
	// Anchored over the pack's tile. modalPackIdx holds the index into
	// area.PackSpawns; if the pack gets dropped while the modal is open,
	// the modal closes.
	modalPackEdit
	// modalChestEdit is the inline chest editor — analogous to
	// modalPackEdit but for chests. Lists the chest's authored items
	// with X to remove and an "+ Add item" dropdown (Enter) to append a
	// new item kind. Anchored over the chest's tile. modalChestIdx holds
	// the area.ChestSpawns index; the modal closes if the chest gets
	// dropped while open.
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
	// modalEscMenu is the pause-style menu opened by pressing Esc on
	// the editor canvas. Bridges the gap between "Esc = exit to title"
	// (jarring on fullscreen, especially when the author wants to
	// drop to Windowed without quitting) and the runtime's pause menu
	// in adventure mode. Offers Display toggle, Continue, and Exit
	// (which still bounces through modalConfirmDirty if the area has
	// unsaved edits).
	modalEscMenu
	// modalFoeView is the Foe Visualizer: a live combat-preview pane for any
	// enemy KIND plus sliders for its billboard placement, contact shadow,
	// target cursor, and tint. Save writes the tuning to maps/sprites/
	// visuals.json (core.EnemyVisualOverride), which the game overlays on its
	// code-default visuals at load — so the author tunes a foe and saves it
	// straight into the game. Map-independent (it edits global per-kind look,
	// not anything on the current area).
	modalFoeView
	// modalPartyView is the Party Visualizer: the party-side twin of
	// modalFoeView. A live combat-preview pane for any party CLASS plus the same
	// billboard placement / shadow / cursor / tint sliders + sprite-PNG strip,
	// saving to maps/sprites/partyvisuals.json (core.PartyVisualOverride). Reuses
	// the foe modal's geometry, field table, and sprite-edit engine. Also
	// map-independent (per-class global look).
	modalPartyView
	// modalEntityList is the "Objects" index: a scrollable list of every
	// pack / chest / door (plus the player start) in the map. Clicking a row
	// recenters the view on that entity and opens its editor — so the author
	// can manage placements without hunting tiles on a big map.
	modalEntityList
	// modalDialogList is the top of the dialog authoring flow: an entity-list
	// of the area's conversations (modalDialogIdx selects one). Add creates a
	// new dialog (auto-id'd) with one starter node; Edit opens its node list;
	// Delete drops it.
	modalDialogList
	// modalDialogNodes lists the selected dialog's nodes (modalDialogNodeIdx
	// selects one). Add / Delete / Set-Start manage the node set; Edit opens
	// the node editor. Esc returns to modalDialogList.
	modalDialogNodes
	// modalDialogNodeEdit is the per-node editor: speaker (dropdown), text /
	// next / continue-label / quest-complete (text fields), an Is-Menu toggle,
	// and the node's choice list (Add / Edit / Delete). Esc returns to
	// modalDialogNodes.
	modalDialogNodeEdit
	// modalDialogChoiceEdit edits one choice of the active node: its label,
	// next-node target, an end-action (opens modalDialogActionEdit), and the
	// choice's condition list (Add / Edit / Delete). Esc → modalDialogNodeEdit.
	modalDialogChoiceEdit
	// modalDialogActionEdit edits the end-action fired by the active NODE or
	// CHOICE (kind dropdown: none / start-quest / complete-quest / event, plus
	// the quest-id or event-id). modalDialogActionOnChoice selects which holder
	// it targets; Esc returns to whichever editor opened it.
	modalDialogActionEdit
	// modalDialogCondEdit edits one selectability condition of the active
	// choice: kind (dropdown) + the kind's params (gold / quest / foe-killed /
	// tile-visited) + an optional disabled message. Esc returns to
	// modalDialogChoiceEdit.
	modalDialogCondEdit
	// modalDialogTriggerList lists the area's dialog triggers (auto-start a
	// conversation on enter-tile / foe-killed). Opened from the dialog list.
	modalDialogTriggerList
	// modalDialogTriggerEdit edits one trigger: kind + target dialog (both
	// dropdowns) + a Once toggle + the kind's params. Esc returns to the list.
	modalDialogTriggerEdit
	// modalHitGlyphs is the read-only Hit Glyphs viewer: a looping gallery of the
	// combat clarity glyphs (slash / impact / frost / spark / fire / holy / venom)
	// drawn over a struck target. Pure preview so the author can see the symbols
	// that normally flash for a fraction of a second mid-attack. See hitglyphs.go.
	modalHitGlyphs
	// modalObjectView is the read-only Object Browser: a paged 3D gallery of every
	// placeable decor + prop, each rendered as a live thumbnail (lit, ground-
	// shadowed, animated) so the author can spot-check the whole object set at a
	// glance without stamping them onto a map. Pure preview. See objectview.go.
	modalObjectView
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
	dragLine
	dragStart
	dragPack
	dragChest
	dragDoor
	dragSelect // marquee region drag (toolSelect): sets the copy/paste selection on release
)

// toolMode is the active grid-paint tool, chosen from the toolbar's tool
// group. It makes the previously modifier-only actions (rectangle, flood,
// eyedropper) discoverable and clickable, and adds Line + Box (hollow rect).
// Modifiers still override while Brush is the active tool — Ctrl = Flood,
// Shift = Rect, Alt = Pick — so existing muscle memory keeps working; an
// explicitly chosen tool is honored without modifiers.
type toolMode int

const (
	toolBrush  toolMode = iota // freehand paint stroke (default)
	toolLine                   // straight line, click anchor → release endpoint
	toolRect                   // filled rectangle
	toolBox                    // hollow rectangle outline (handy for room walls)
	toolFlood                  // flood-fill the connected region
	toolPick                   // eyedropper: sample the cell's char into the brush
	toolSelect                 // marquee: drag a region to copy (Ctrl+C) / paste (Ctrl+V)
)

// toolModeLabels are the toolbar button captions, indexed by toolMode. Kept
// next to the enum so a new tool adds its label here and the toolbar picks it
// up via the toolButtons builder.
var toolModeLabels = [...]string{
	toolBrush:  "Brush",
	toolLine:   "Line",
	toolRect:   "Rect",
	toolBox:    "Box",
	toolFlood:  "Flood",
	toolPick:   "Pick",
	toolSelect: "Select",
}

// toolModeHelp is the hover-tooltip text per tool — the terse labels above don't
// say what Box / Flood / Pick / Select do, so the toolbar shows this on hover.
// Indexed by toolMode like toolModeLabels.
var toolModeHelp = [...]string{
	toolBrush:  "Paint freehand with the selected brush.",
	toolLine:   "Drag a straight line of tiles.",
	toolRect:   "Drag a filled rectangle.",
	toolBox:    "Drag a hollow rectangle — handy for room walls.",
	toolFlood:  "Flood-fill the connected same-tile region.",
	toolPick:   "Eyedropper — sample the clicked tile into the brush.",
	toolSelect: "Marquee — drag a region, then Ctrl+C to copy, Ctrl+V to paste.",
}

// brushRef identifies a palette brush as (layer, index within that layer's
// palette). Used by the recent-brushes quick-pick row.
type brushRef struct {
	layer Layer
	idx   int
}

// maxRecentBrushes caps the recent-brush swatch row.
const maxRecentBrushes = 8

type statusEntry struct {
	msg   string
	timer float32
	// warn tints the row danger-colored so warnings (e.g. post-save
	// reachability problems) read distinctly from neutral confirmations
	// like "Saved" instead of blending into the same status style.
	warn bool
}

const undoLimit = 50

// State is the editor's mutable state across frames.
type State struct {
	area core.AreaDefinition

	// reachWarnings caches the last reachabilityWarnings(s.area) result so the
	// metadata panel's per-frame badge draw doesn't re-run a full-grid BFS
	// (plus two w*h allocations) every frame. reachValid=false means "stale,
	// recompute on next read"; it's flipped false by every area mutation —
	// pushUndo (the universal pre-mutation hook), undo/redo, and performNewMap.
	// Zero value (false) is correct: the first read computes it.
	reachWarnings []string
	reachValid    bool
	// contentEpoch bumps on every area mutation (the same chokepoints that
	// flip reachValid: commitUndoSnapshot, undo/redo, performNewMap). Per-frame
	// draws that cache a derived-from-content string (the topbar info readout,
	// the hover tooltip) key on it so they rebuild only when the map actually
	// changed, not every frame.
	contentEpoch uint64

	layer    Layer
	brushIdx [layerCount]int
	// recentBrushes is the most-recently-selected (layer, brush) pairs,
	// newest first, deduped and capped at maxRecentBrushes. Drawn as a quick
	// swatch row in the grid's bottom-left corner; clicking one jumps to that
	// layer + brush. Recorded by recordRecentBrush at every brush-select site.
	recentBrushes []brushRef
	// layerHidden hides a layer from the grid draw (the layer-tab eye
	// toggles). Hidden layers still exist on disk and stay editable; this is
	// a pure view filter so the author can isolate what they're working on.
	// Alt-clicking an eye solos that layer (hides all others). Entities count
	// as a layer here too, so entity markers can be hidden.
	layerHidden [layerCount]bool
	// editLevel is the height selector's current level (0..maxEditLevel): the
	// value the Elevation "Set Height" brush stamps and the focus level for
	// the slice-view tinting. rampMode toggles the smart ramp tool — while on,
	// a left-click places a connective ramp (right-click clears one) instead
	// of normal painting.
	editLevel int
	rampMode  bool
	// Verticality is now a Photoshop-style LEVELS model (always on — no lens
	// toggle). editLevel is the ACTIVE level: every content paint lifts the
	// painted tile to it (so picking a level and drawing builds that floor),
	// and it's the level the Levels panel highlights. levelHidden[L] hides
	// every tile on level L from the grid draw (the panel's per-level eye),
	// EXCEPT a ramp connecting to the active level, which always shows so
	// connections stay visible across hidden floors. topLevel is the highest
	// level the panel exposes (grown via the panel's +; auto-covers any level
	// present in the map or the active level). The on-disk model stays one
	// level per tile — this is a pure authoring view.
	levelHidden [maxEditLevel + 1]bool
	topLevel    int
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
	// Dialog-editor indices. modalDialogIdx selects the area.Dialogs entry
	// the node list / node editor operate on; modalDialogNodeIdx selects the
	// node within that dialog the node / choice editors operate on;
	// modalDialogChoiceIdx selects the choice within that node the choice
	// editor edits. All -1 outside their flows (reset by closeModal).
	modalDialogIdx       int
	modalDialogNodeIdx   int
	modalDialogChoiceIdx int
	// modalDialogCondIdx selects the condition within the active choice the
	// condition editor edits; modalDialogTriggerIdx selects the area.Triggers
	// entry the trigger editor edits. Both -1 outside their flows.
	modalDialogCondIdx    int
	modalDialogTriggerIdx int
	// modalDialogActionOnChoice selects whose end-action the action editor
	// (modalDialogActionEdit) targets: the current CHOICE's EndAction when true,
	// the current NODE's when false. See currentDialogActionHolder.
	modalDialogActionOnChoice bool
	// dialogNumBuf is the shared edit buffer for whichever numeric dialog
	// condition/trigger field is focused (gold / kills / tile X / tile Z).
	// Seeded from the field when focus enters it (focusDialogNumeric) and
	// parsed back on each keystroke (see dialogNumericTarget / pumpDialogNumeric)
	// — one buffer suffices since only one numeric field is ever focused.
	dialogNumBuf string
	// New-map dialog state. modalNewWidth / modalNewHeight hold the
	// in-progress dimensions (text-input commits write here, not the
	// area); modalNewFloor is the chosen default floor char that
	// blankArea will fill the interior with on confirm.
	modalNewWidth  int
	modalNewHeight int
	modalNewFloor  byte
	// modalValidateRows is the snapshot of warnings shown in
	// modalValidate. Captured at open time so the read-only display
	// doesn't reflow while the user is reading it.
	modalValidateRows []string
	// Sound modal state. Lives on State so the slider positions and
	// last-edited name survive open/close of the modal — closing
	// shouldn't reset the user's tuning work.
	soundParams    soundParamSet
	soundName      string
	soundCursor    int        // row cursor inside the sound modal
	soundLeftPanel soundPanel // which column of the sound modal has focus
	// soundParamScroll is the vertical scroll offset (pixels) of the params
	// column's slider body — the column now holds more sliders (grouped into
	// Oscillator / Envelope / FX sections) than fit at once, so the body
	// scrolls under a fixed sub-header and a fixed name/actions footer.
	soundParamScroll float32
	// deleteArmed is the shared two-press delete guard token: the first Delete
	// click for a token (e.g. "sound:<name>", "custom:<name>", "door") arms it
	// with a flash, the second for the SAME token confirms. One field + the
	// armOrConfirmDelete helper back the sound / custom-enemy / door deletes so
	// the "click again to confirm" rule lives once. Cleared on modal close.
	deleteArmed string
	// soundSavedCache holds the result of audio.ListUserSounds() for
	// the current Update→Draw frame so we don't ReadDir twice per frame
	// while the modal is open. Refreshed by updateSoundsModal at the
	// top of each frame; drawSoundsModal reads it back.
	soundSavedCache []string
	// soundAssignCache is the cue-slug → user-sound assignment map, cached
	// alongside soundSavedCache. Both are refreshed on modal-open and after
	// each save/delete/assign mutation (refreshSoundCaches) — NOT per frame —
	// so the assign column's draw can index this map instead of re-reading +
	// re-parsing assignments.txt once per cue every frame.
	soundAssignCache map[string]string

	// Foe Visualizer (modalFoeView) state. foeKind is the enemy kind being
	// tuned; foeVisual is the working override the sliders edit + the preview
	// draws; foeBaseline snapshots the values at open so Reset reverts this
	// session's edits. foeCursor is the focused slider row. Persist across
	// open/close so reopening returns to the last foe.
	foeKind     core.EnemyKind
	foeVisual   core.EnemyVisualOverride
	foeBaseline core.EnemyVisualOverride
	foeCursor   int
	foeInit     bool

	// Party Visualizer (modalPartyView) state — the party-class twin of the foe
	// fields above. partyClass is the class being tuned; partyVisual is the
	// working override (= EnemyVisualOverride alias); partyBaseline snapshots for
	// Reset; partyCursor is the focused slider row; partyInit gates first-open
	// seeding so reopening returns to the last class.
	partyClass    core.PartyClass
	partyVisual   core.PartyVisualOverride
	partyBaseline core.PartyVisualOverride
	partyCursor   int
	partyInit     bool

	// foeViewTab selects which pane the Foe/Party Visualizer shows: 0 = Layout
	// (the placement/shadow/cursor/glyph/tint slider stack), 1 = Asset (the
	// sprite-PNG bake/import strip). Shared by both modals since only one is open
	// at a time. See foeViewTabLabels.
	foeViewTab int
	// foeViewZoom is the visualizer preview's dolly factor (mouse wheel over the
	// preview pane). 1 = default framing; clamped to render.FoePreviewZoom{Min,Max}.
	// Shared by both visualizer modals; reset to 1 on open.
	foeViewZoom float32
	// Asset-tab state, shared by both visualizer modals. The non-destructive
	// adjustment VALUES (Pixelate/Brightness/Contrast) live in the visual override
	// (foeVisual/partyVisual), not here. assetCursor is the focused asset-slider
	// row; assetPreviewStale requests a live-preview rebuild next frame (set on a
	// slider change / Revert / foe cycle / open). See foeview.go's asset* helpers.
	assetCursor       int
	assetPreviewStale bool

	// objectViewPage is the current page of the Object Browser gallery
	// (modalObjectView) — a paged grid of every placeable decor/prop thumbnail.
	// Reset to 0 on open; the wheel / arrow keys page through. See objectview.go.
	objectViewPage int

	pending pendingAction

	undo  []core.AreaDefinition
	redo  []core.AreaDefinition
	dirty bool
	// baseline is a snapshot of the area as last loaded from / saved to disk.
	// Used by undo/redo to detect "the working state now matches what's on
	// disk" so the dirty marker can clear instead of latching forever.
	baseline core.AreaDefinition

	statusLog []statusEntry

	brushSize int

	// tool is the active grid-paint tool (toolbar tool group). Default
	// toolBrush (zero value) preserves the original freehand-paint behavior.
	tool toolMode
	drag dragKind

	// Region copy/paste (toolSelect). selActive marks a committed marquee; its
	// inclusive, normalized tile bounds are sel{X0,Z0,X1,Z1}. clipboard holds the
	// last Ctrl+C snapshot (across the six grid layers) for Ctrl+V paste at the
	// cursor. Tiles only — entities aren't part of a region copy.
	selActive                  bool
	selX0, selZ0, selX1, selZ1 int
	clipboard                  core.TileRegion
	// dragSnapshotDone reports whether the current paint stroke has already
	// banked its single undo snapshot. dragUndoBefore holds the pre-stroke
	// area, captured at stroke start and committed to the undo stack LAZILY by
	// strokePaint — only once the stroke actually mutates a cell. A stroke that
	// changes nothing (every cell refused by a brush guard, or painting a cell
	// its current value) banks no undo step and leaves the redo stack intact.
	dragSnapshotDone bool
	dragUndoBefore   core.AreaDefinition
	lastPaintX       int
	lastPaintZ       int
	rectAnchorX      int
	rectAnchorZ      int
	// rectHollow is set when a rectangle drag was started with the Box tool,
	// so finishDrag paints only the outline instead of a filled block.
	rectHollow  bool
	dragPackIdx int
	// dragChestIdx / dragDoorIdx mirror dragPackIdx for the chest / door
	// drag-move flows: the spawn index grabbed at press, relocated (or, on a
	// release-in-place, opened in its edit modal) at release. -1 when idle.
	dragChestIdx int
	dragDoorIdx  int

	gridCursorX int
	gridCursorZ int
	hoverX      int
	hoverZ      int

	zoom    float32
	panX    float32
	panY    float32
	panning bool
	// scrollDrag is the cross-frame memory for an in-flight scrollbar thumb
	// drag (which bar, and where inside the thumb the grab landed). Zero value
	// (id == scrollNone) means no bar is being dragged. See scrollbar.go.
	scrollDrag scrollDragState

	exitRequested     bool
	testRequested     bool
	awaitingOverwrite bool

	// showTileGlyphs toggles the per-tile char overlay in the grid.
	// Default false → overlay OFF (drawing every layer's char at once is
	// too noisy to leave on). ALT (tapped on release with no other key
	// pressed during the hold) flips it ON; when on, each cell shows the
	// char of the ACTIVE layer only (see currentLayerGlyph) so it stays
	// legible. altChordUsed tracks whether ALT+something was pressed
	// during the current Alt hold; if so we suppress the toggle on
	// release so Alt+1..6 (layer jump) doesn't double-trigger the flip.
	showTileGlyphs bool
	altChordUsed   bool
	// showDoorLinks toggles the door-link diagnostic overlay (the "Links"
	// toolbar button): connectors from each door to its same-map target plus
	// warning rings on doors whose target_door doesn't resolve.
	showDoorLinks bool

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

	// dropdown is the single open-dropdown slot — the reusable "pick one of
	// N" selector the pack/chest editors open instead of memorizing a long
	// list of per-kind add-keys. Zero value (owner == ddNone) means closed.
	// See dropdown.go.
	dropdown dropdownState

	rect layoutRect
}

type layoutRect struct {
	topbar    rl.Rectangle
	toolbar   rl.Rectangle // action button row beneath the topbar menu bar
	layerTabs rl.Rectangle
	levels    rl.Rectangle // Photoshop-style elevation-level panel (eye toggles + active select)
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

// ReachabilityWarnings returns the cached reachabilityWarnings(s.area)
// result, recomputing only when the area changed since the last call. The
// metadata panel calls this once per draw (every frame the editor is open),
// and the check is a full-grid flood-fill that allocates two w*h bool slices
// — on a large map that's wasteful GC pressure 60×/sec for a value that only
// changes on an edit. reachValid is invalidated by every area mutation
// (pushUndo / undo / redo / performNewMap), so the cache can't go stale.
func (s *State) ReachabilityWarnings() []string {
	if !s.reachValid {
		s.reachWarnings = reachabilityWarnings(s.area)
		s.reachValid = true
	}
	return s.reachWarnings
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

// materializeEntranceCrystal makes the entrance crystal an editable entity
// rather than an invisible runtime ghost. A map that hasn't authored its
// crystals (legacy maps, freshly-blanked new maps, and maps opened from disk
// without a crystals: section) gets the same default entrance crystal the
// runtime would synthesize — but as a real, movable, deletable CrystalSpawn —
// and is marked authored so saving persists the author's exact set (including
// zero, if they delete it). Already-authored maps are left untouched so an
// intentional empty set survives a round-trip. Shared by every editor entry
// point that swaps in a new area (freshState / performNewMap / openSelectedMap)
// so none of them can drift back into the unauthored-ghost state.
func materializeEntranceCrystal(a core.AreaDefinition) core.AreaDefinition {
	if !a.CrystalsAuthored {
		a.CrystalSpawns = core.DefaultEntranceCrystalSpawns(a)
		a.CrystalsAuthored = true
	}
	return a
}

func freshState(a core.AreaDefinition) State {
	a = materializeEntranceCrystal(a)
	return State{
		area:                  a,
		baseline:              core.CloneArea(a),
		layer:                 LayerWalls,
		brushSize:             1,
		zoom:                  1,
		gridCursorX:           -1,
		gridCursorZ:           -1,
		hoverX:                -1,
		hoverZ:                -1,
		dragPackIdx:           -1,
		dragChestIdx:          -1,
		dragDoorIdx:           -1,
		modalPackIdx:          -1,
		modalChestIdx:         -1,
		modalDoorIdx:          -1,
		modalDialogIdx:        -1,
		modalDialogNodeIdx:    -1,
		modalDialogChoiceIdx:  -1,
		modalDialogCondIdx:    -1,
		modalDialogTriggerIdx: -1,
	}
}

func blankArea(w, h int, floorChar byte) core.AreaDefinition {
	walls := make([]string, h)
	floor := make([]string, h)
	decor := make([]string, h)
	props := make([]string, h)
	ceiling := make([]string, h)
	elevation := make([]string, h)
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
		elevation[z] = blankRow(w, core.ElevationGround)
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
		Elevation:    elevation,
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

// activeBrush returns the currently selected brush in the active layer. On the
// Elevation layer the brush char is the height selector's current level
// ('0'+editLevel) rather than a static palette char, so paint / flood-fill /
// the brush preview all stamp the selected height through the normal path.
func (s *State) activeBrush() Brush {
	palette := layerBrushes[s.layer]
	idx := s.brushIdx[s.layer]
	if idx < 0 || idx >= len(palette) {
		idx = 0
	}
	b := palette[idx]
	if s.layer == LayerElevation {
		b.Char = core.ElevationChar(s.editLevel)
	}
	return b
}

// clampLevel bounds a level into [0, maxEditLevel].
func clampLevel(l int) int {
	return core.Clamp(l, 0, maxEditLevel)
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

	// An open menu-bar pull-down (or any top-level dropdown) owns input — handle
	// it here so the canvas + hotkeys stay inert behind it. No-op when closed.
	if s.dropdownOpen() {
		updateDropdown(s)
		return ActionNone
	}

	// An open right-click context menu owns ALL input until it closes. Its own
	// handler (updateContextMenu, inside updateMouse) absorbs the mouse and the
	// Esc/click-outside close — but the keyboard hotkeys run BEFORE updateMouse,
	// so gate them here too. Otherwise number keys, Ctrl+Z/Y, Alt+1..7, and
	// Space/Backspace would mutate the map behind the open menu.
	if !s.contextMenu.open {
		updateHotkeys(s)
	}
	updateMouse(s)

	if s.testRequested {
		s.testRequested = false
		// Validate the spawn the playtest will ACTUALLY use. On a Ctrl+F5
		// "test from cursor" the run loop spawns at testStartOverride*, so the
		// gate must check those coords — otherwise a bad authored start could
		// block a perfectly valid cursor test (and the override cell would skip
		// the full start-blocker, having only been BlockedAt-checked at arm).
		checkArea := s.area
		if s.testStartOverride {
			checkArea.StartTileX = s.testStartOverrideX
			checkArea.StartTileZ = s.testStartOverrideZ
		}
		// Physical-only gate: block the playtest ONLY when the player would
		// spawn somewhere impossible / confusing (start out of bounds, inside
		// geometry, or sharing a chest tile). Reachability ("can you actually
		// walk to the packs / chests?") is deliberately NOT a gate and is no
		// longer auto-flashed here — a map may be hard, sealed, or unconventional
		// by design, and the editor shouldn't presume how it's meant to be
		// played. Reachability lives in the at-will Validate modal instead.
		if !canPlaytest(checkArea) {
			s.flash("Test: " + startTileBlocker(checkArea))
			return ActionNone
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
func (s *State) flash(msg string) { s.pushStatus(msg, false) }

// flashWarn pushes a warning-tinted status row (drawn danger-colored)
// so it stands out from neutral confirmations like "Saved".
func (s *State) flashWarn(msg string) { s.pushStatus(msg, true) }

func (s *State) pushStatus(msg string, warn bool) {
	for i, e := range s.statusLog {
		if e.msg == msg {
			s.statusLog[i].timer = statusLogLifetime
			s.statusLog[i].warn = warn
			return
		}
	}
	s.statusLog = append(s.statusLog, statusEntry{msg: msg, timer: statusLogLifetime, warn: warn})
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

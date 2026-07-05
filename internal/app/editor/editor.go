// Package editor is the in-game map authoring tool: five paintable grid layers
// (floor / decor / props / ceiling / elevation) plus per-tile faces (set from the
// right-click context menu, not a paint layer) and entities.
package editor

import (
	"crawler/internal/app/core"
	"crawler/internal/app/render"
	"fmt"
	"strconv"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Action is what the editor wants the run loop to do next.
type Action int

const (
	ActionNone Action = iota
	ActionExitToTitle
	// ActionTest drops the run loop into the adventure scene with the editor's
	// in-memory area, returning here on quit.
	ActionTest
)

// Layer is the active grid layer (or the entity list) being authored. Drives the
// palette, click action, and grid emphasis.
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
// Levels serialize as one base-36 char per cell ('0'..'9' then 'A'..'Z').
const maxEditLevel = core.MaxElevationLevel

// layerCount is the number of editor layers (six grid + entities), from the
// enum's last value so adding a layer needs no magic-number bump.
const layerCount = int(LayerEntities) + 1

// numberRowKeys is the top-row 1..9 key codes, used for brush-select hotkeys and
// Alt+N layer jump. The first len(selectableLayers) entries double as jump keys;
// this assumes len(selectableLayers) <= 9 (asserted in init).
var numberRowKeys = [9]int32{
	rl.KeyOne, rl.KeyTwo, rl.KeyThree, rl.KeyFour, rl.KeyFive,
	rl.KeySix, rl.KeySeven, rl.KeyEight, rl.KeyNine,
}

func init() {
	// A selectable layer past the 9th would lose its Alt+N jump key — trip loudly.
	if len(selectableLayers) > len(numberRowKeys) {
		panic("editor: more selectable layers than numberRowKeys — grow numberRowKeys to keep Alt+N layer jumps")
	}
}

// Brush is one entry in a layer's palette. Grid layers write Char at the painted
// cell; LayerEntities fires the Entity placement tool on click.
type Brush struct {
	Name      string
	Char      byte
	Entity    entityKind
	EnemyKind core.EnemyKind // only meaningful when Entity == entityAddEnemy
	Hotkey    int32
	Color     rl.Color
	// Erase runs the active layer's eraseAt reset instead of stamping Char.
	Erase bool
}

type entityKind int

const (
	// entityNone is the iota anchor and the zero value of Brush.Entity — a grid
	// (non-entity) brush carries it implicitly. Never matched directly; the entity
	// brushes below are the live cases.
	entityNone entityKind = iota
	// entityClear erases the pack/chest/door at the clicked tile (the anchored
	// player start is exempt). First-class brush mirror of right-click clear.
	entityClear
	entityPlayerStart
	// entityAddEnemy appends a member (Brush.EnemyKind) to the pack at the clicked
	// tile, creating one if absent. Right-click clears the pack.
	entityAddEnemy
	// entityPlaceChest drops a chest with default starter loot. Right-click clears.
	entityPlaceChest
	// entityPlaceDoor drops an area-transition door with a placeholder name and
	// "self" target; authored via modalDoorEdit. Right-click clears.
	entityPlaceDoor
	// entityPlaceCrystal drops a healing crystal (no per-instance authoring).
	// Right-click (or Clear brush) removes it.
	entityPlaceCrystal
)

// paletteLabels mirrors layerBrushes with pre-formatted display labels ("1 Wall
// (#)", ...), built once at init since palette content is static between edits.
var paletteLabels [layerCount][]string

func init() {
	// Build the Faces palette from core.FaceSkins so adding a skin is one row in
	// core. Label is "Name (c)"; swatch + hotkey from the editor-local tables.
	faces := make([]Brush, 0, len(core.FaceSkins))
	for i, sk := range core.FaceSkins {
		col, ok := faceSkinSwatch[sk.Char]
		if !ok {
			col = wallSwatch
		}
		var hk int32
		if i < len(numberRowKeys) {
			hk = numberRowKeys[i]
		}
		faces = append(faces, Brush{
			Name:   fmt.Sprintf("%s (%c)", sk.Name, sk.Char),
			Char:   sk.Char,
			Hotkey: hk,
			Color:  col,
		})
	}
	layerBrushes[LayerWalls] = faces

	// Append an Erase brush to every grid layer (Entities has entityClear instead).
	for l := range layerBrushes {
		if Layer(l) == LayerEntities {
			continue
		}
		layerBrushes[l] = append(layerBrushes[l], Brush{Name: "Erase", Erase: true, Color: clearBrushColor})
	}
	for layer, brushes := range layerBrushes {
		labels := make([]string, len(brushes))
		for i, b := range brushes {
			if prefix := brushHotkeyPrefix(i); prefix != "" {
				labels[i] = prefix + " " + b.Name
			} else {
				labels[i] = b.Name // mouse-only (past the two number rows) — no key shown
			}
		}
		paletteLabels[layer] = labels
	}
}

// brushHotkeyPrefix returns the real select-key label for palette index i — "1".."9"
// for the number row, "Sh+1".."Sh+9" for the Shift row — or "" past index 17, where
// the brush is mouse/scroll-only (updateHotkeys maps positions, not brush.Hotkey).
// Numbering only the reachable brushes keeps the invisible keyboard ceiling honest.
func brushHotkeyPrefix(i int) string {
	switch {
	case i < 0 || i >= 2*len(numberRowKeys):
		return ""
	case i < len(numberRowKeys):
		return strconv.Itoa(i + 1)
	default:
		return "Sh+" + strconv.Itoa(i-len(numberRowKeys)+1)
	}
}

// faceSkinSwatch maps a cliff-face skin char to its palette swatch (one muted
// grey family with per-variant tints). Unmapped skins fall back to wallSwatch.
var faceSkinSwatch = map[byte]rl.Color{
	core.TileWallRockIvyLight:  tintSwatch(wallSwatch, -8, 6, -10),
	core.TileWallRockIvyHeavy:  tintSwatch(wallSwatch, -20, 4, -22),
	core.TileWallRockCracked:   tintSwatch(wallSwatch, -10, -10, -8),
	core.TileWallRockCrumbling: tintSwatch(wallSwatch, -2, -12, -22),
}

// elevationPlaceholderChar is a sentinel: activeBrush overwrites the LayerElevation
// brush's Char with core.ElevationChar(editLevel) each frame, so it's never stamped.
const elevationPlaceholderChar = 0

// layerBrushes holds each layer's palette. LayerWalls (Faces) is built in init
// from core.FaceSkins; those brushes only pick a tile's exposed cliff-face skin.
var layerBrushes = [layerCount][]Brush{
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
	// Elevation: one "Set Height" brush stamps the selector's level (activeBrush
	// rewrites its Char each frame). Ramps are the toolbar's Ramp mode, not a brush.
	LayerElevation: {
		{Name: "Set Height", Char: elevationPlaceholderChar, Hotkey: rl.KeyOne, Color: rl.NewColor(150, 140, 120, 255)},
	},
	LayerEntities: buildEntityBrushes(),
}

// entityBrushHotkeys is the positional hotkey pool for enemy brushes on
// LayerEntities. Keys 1/2 are reserved for Clear / Player Start; enemies take
// 3..N. Slices numberRowKeys so it can't drift from the number-row binding.
// Past pool length, brushes get no hotkey (mouse-only).
var entityBrushHotkeys = numberRowKeys[2:]

// entityBrushColors is the per-enemy swatch tint (init asserts full coverage).
var entityBrushColors = map[core.EnemyKind]rl.Color{
	core.EnemyRat:          rl.NewColor(220, 156, 96, 255),
	core.EnemyBat:          rl.NewColor(160, 130, 220, 255),
	core.EnemyDiseasedRat:  rl.NewColor(140, 200, 90, 255),
	core.EnemyGoblin:       rl.NewColor(132, 196, 110, 255),
	core.EnemyGoblinMage:   rl.NewColor(220, 168, 244, 255),
	core.EnemyAmoeba:       rl.NewColor(180, 200, 220, 255),
	core.EnemyVenusMantrap: rl.NewColor(220, 124, 158, 255),
	core.EnemyCaveSpider:   rl.NewColor(96, 60, 110, 255),
	core.EnemyVampireBat:   rl.NewColor(200, 70, 80, 255),
	core.EnemyWisp:         rl.NewColor(180, 220, 255, 255),
	core.EnemyStoneGolem:   rl.NewColor(120, 116, 108, 255),
	core.EnemyNecromancer:  rl.NewColor(76, 84, 130, 255),
	core.EnemySkeleton:     rl.NewColor(230, 226, 198, 255),
}

// entityBrushColor returns kind's swatch tint, or the grey fallback for an unmapped
// kind. Single home for the lookup+fallback (the palette build and the pack marker).
func entityBrushColor(kind core.EnemyKind) rl.Color {
	if col, ok := entityBrushColors[kind]; ok {
		return col
	}
	return entityFallbackColor
}

// init asserts entityBrushColors covers every enemy kind, and that the Prop/Decor
// palettes carry a brush for every core prop/decor char — add a prop/decor in core
// and forget the palette here and it silently never appears in the editor.
func init() {
	for _, def := range core.EnemyKinds() {
		if _, ok := entityBrushColors[def.Kind]; !ok {
			panic("editor: missing entityBrushColors entry for " + def.Name)
		}
	}
	assertPaletteCovers(LayerProps, core.PropTileChars(), "PropTileChars")
	assertPaletteCovers(LayerDecor, core.DecorTileChars(), "DecorTileChars")
}

// assertPaletteCovers panics if any char in want lacks a layerBrushes[layer] entry.
func assertPaletteCovers(layer Layer, want []byte, source string) {
	have := make(map[byte]struct{}, len(layerBrushes[layer]))
	for _, b := range layerBrushes[layer] {
		have[b.Char] = struct{}{}
	}
	for _, c := range want {
		if _, ok := have[c]; !ok {
			panic("editor: layerBrushes is missing a brush for core." + source + " char '" + string(c) + "' — add it to the palette")
		}
	}
}

// buildEntityBrushes assembles the LayerEntities palette: Clear, Player Start,
// one brush per core.EnemyKinds(), then Chest/Door/Crystal.
func buildEntityBrushes() []Brush {
	brushes := []Brush{
		{Name: "Clear", Entity: entityClear, Hotkey: rl.KeyOne, Color: clearBrushColor},
		{Name: "Player Start", Entity: entityPlayerStart, Hotkey: rl.KeyTwo, Color: render.MarkerStart},
	}
	defs := core.EnemyKinds()
	// slot runs across entityBrushHotkeys: enemies first, then each trailing brush.
	slot := 0
	for _, def := range defs {
		brushes = append(brushes, Brush{
			Name:      "Add " + def.SingularName,
			Entity:    entityAddEnemy,
			EnemyKind: def.Kind,
			Hotkey:    entityHotkeyAt(slot),
			Color:     entityBrushColor(def.Kind),
		})
		slot++
	}
	brushes = append(brushes, Brush{
		Name:   "Place Chest",
		Entity: entityPlaceChest,
		Hotkey: entityHotkeyAt(slot),
		Color:  render.MarkerChest,
	})
	slot++
	brushes = append(brushes, Brush{
		Name:   "Place Door",
		Entity: entityPlaceDoor,
		Hotkey: entityHotkeyAt(slot),
		Color:  render.MarkerDoor,
	})
	slot++
	brushes = append(brushes, Brush{
		Name:   "Place Crystal",
		Entity: entityPlaceCrystal,
		Hotkey: entityHotkeyAt(slot),
		Color:  render.MarkerCrystal,
	})
	return brushes
}

// entityHotkeyAt returns the entity-brush hotkey for slot i, or 0 past the pool.
func entityHotkeyAt(i int) int32 {
	if i >= 0 && i < len(entityBrushHotkeys) {
		return entityBrushHotkeys[i]
	}
	return 0
}

// layerName / layerAccent read the display label + color-code from layerDefs
// (layerdef.go) — the accents echo each layer's palette family (floor=grass,
// decor=teal, props=wood, ceiling=sky, elevation=stone-tan, entities=amber).
func layerName(l Layer) string {
	if int(l) < 0 || int(l) >= layerCount {
		panic("editor: layerName called with unhandled Layer")
	}
	return layerDefs[l].name
}

// layerAccent returns layer l's color-code (falls back to the active-border tint).
func layerAccent(l Layer) rl.Color {
	if int(l) < 0 || int(l) >= layerCount {
		return editorBorderActive
	}
	return layerDefs[l].accent
}

// selectableLayers are the layers the author can make active (Tab-cycle, Alt+N,
// layer picker). LayerWalls is absent: faces are now a per-tile property set from
// the right-click context menu, not a paintable layer.
var selectableLayers = []Layer{LayerFloor, LayerDecor, LayerProps, LayerCeiling, LayerElevation, LayerEntities}

// cycleSelectableLayer returns the next (+1) / previous (-1) selectable layer,
// wrapping. Skips the unselectable LayerWalls (absent from selectableLayers).
func cycleSelectableLayer(cur Layer, dir int) Layer {
	return cycleByIndex(selectableLayers, cur, dir)
}

// setIfChanged writes v to *dst only when it differs, banking one undo snapshot and
// marking the map dirty — the single home for the "re-picking the current value must
// not churn undo/dirty" field-setter idiom. Callers guard nil containers first.
func setIfChanged[T comparable](s *State, dst *T, v T) {
	if *dst == v {
		return
	}
	pushUndo(s)
	*dst = v
	s.dirty = true
}

type focusField int

const (
	focusNone focusField = iota
	focusName
	focusQuiet
	focusFilename
	focusWidth
	focusHeight
	// New-map dialog text fields (modalNew); committed to State.modalNewWidth/Height.
	focusNewWidth
	focusNewHeight
	// Location-edit text focus (modalLocationEdit): the region's Name field.
	focusLocationName
	// Door-edit text foci (modalDoorEdit), routed via activeTextTarget.
	focusDoorName
	focusDoorTargetMap
	focusDoorTargetDoor
	// Dialog-editor text foci (node + choice editors), routed via dialogTextTarget.
	focusDialogNodeText
	focusDialogNodeNext
	focusDialogNodeContinue
	focusDialogChoiceLabel
	focusDialogChoiceNext
	// focusDialogActionID edits the action editor's quest-id / event-id
	// (modalDialogActionEdit), routed by action kind via dialogActionIDTarget.
	focusDialogActionID
	// Condition-editor foci (modalDialogCondEdit); numerics route through dialogNumBuf.
	focusDialogCondQuestID
	focusDialogCondMessage
	focusDialogCondGold
	focusDialogCondFoeKills
	focusDialogCondTileX
	focusDialogCondTileZ
	// Trigger-editor CONDITION numeric foci (modalDialogTriggerEdit) — share dialogNumBuf.
	focusDialogTrigTileX
	focusDialogTrigTileZ
	focusDialogTrigFoeKills
	// Trigger-editor ACTION numeric foci.
	focusTrigActTileX
	focusTrigActTileZ
	focusTrigActCount
	// Trigger-editor text foci (routed via dialogTrigTextTarget, pumped with pumpFocusField):
	// the condition's switch/counter/quest-id name, and the action's switch/counter/text/quest-id.
	focusTrigCondText
	focusTrigActText
	// focusWallFeatureSwitch edits a wall fixture's target switch name.
	focusWallFeatureSwitch
)

type modalKind int

const (
	modalNone modalKind = iota
	modalOpen
	modalSaveAs
	modalConfirmDirty
	// modalPackEdit is the inline pack editor (X remove, K/J reorder, Enter
	// add-member dropdown). modalPackIdx indexes area.PackSpawns; closes if dropped.
	modalPackEdit
	// modalChestEdit is the inline chest editor (X remove, Enter add-item dropdown).
	// modalChestIdx indexes area.ChestSpawns; closes if dropped.
	modalChestEdit
	// modalSounds is the in-editor sound creator: synthesize/preview/save a cue to
	// maps/sounds/<name>.wav, delete, or assign to a built-in audio.Sound.
	modalSounds
	// modalDoorEdit is the inline door editor: rename, set target_map / target_door,
	// pick facing, delete.
	modalDoorEdit
	// modalValidate is the read-only reachability + cross-map-door report (uncapped).
	modalValidate
	// modalNew is the "New map" setup dialog (width / height / default floor).
	modalNew
	// modalEscMenu is the Esc pause menu: Display toggle, Continue, Exit.
	modalEscMenu
	// modalFoeView is the Foe Visualizer: live combat preview + placement/shadow/
	// cursor/tint sliders, saving per-kind look to maps/sprites/visuals.json.
	// Map-independent.
	modalFoeView
	// modalPartyView is the Party Visualizer: the party-class twin of modalFoeView,
	// saving to maps/sprites/partyvisuals.json. Reuses the foe modal's engine.
	modalPartyView
	// modalEntityList is the "Objects" index of every pack/chest/door + start;
	// clicking a row recenters and opens its editor.
	modalEntityList
	// modalDialogList lists the area's conversations (modalDialogIdx selects).
	modalDialogList
	// modalDialogNodes lists the selected dialog's nodes (modalDialogNodeIdx selects).
	modalDialogNodes
	// modalDialogNodeEdit is the per-node editor (speaker / text / next / choices).
	modalDialogNodeEdit
	// modalDialogChoiceEdit edits one choice (label / next / end-action / conditions).
	modalDialogChoiceEdit
	// modalDialogActionEdit edits the end-action of the active node or choice
	// (modalDialogActionOnChoice picks which holder).
	modalDialogActionEdit
	// modalDialogCondEdit edits one choice selectability condition.
	modalDialogCondEdit
	// modalDialogTriggerList lists the area's triggers (StarEdit-style condition→action).
	modalDialogTriggerList
	// modalDialogTriggerEdit edits one trigger (primary condition + action + preserve).
	modalDialogTriggerEdit
	// modalWallFeatureEdit edits one wall fixture (switch/bombable/secret): kind, face
	// direction, target switch, once, delete. modalWallFeatureIdx indexes area.WallFeatures.
	modalWallFeatureEdit
	// modalLocationEdit edits one named region (name / bounds / level / delete).
	// modalLocationIdx indexes area.Locations; closes if dropped.
	modalLocationEdit
	// modalHitGlyphs is the read-only Hit Glyphs gallery preview. See hitglyphs.go.
	modalHitGlyphs
	// modalObjectView is the read-only paged 3D Object Browser. See objectview.go.
	modalObjectView
	// modalWallFaces is the per-tile cliff-face skin editor (base + N/E/S/W),
	// targeting wallFaceX/wallFaceZ. See wallfaces.go.
	modalWallFaces
	// modalHelp is the read-only keyboard-shortcut reference (? key). See help.go.
	modalHelp
	// modalCrystalEdit is the per-crystal editor: floor (Level) stepper + delete.
	// modalCrystalIdx indexes area.CrystalSpawns; closes if dropped.
	modalCrystalEdit
	// modalGoto is the jump-to-tile dialog: type an X,Z to recenter, or pick/save a
	// view bookmark. See gotoedit.go.
	modalGoto
	// modalStats is the read-only Map Stats report (tile mix, counts, encounter
	// budget). See statsview.go.
	modalStats
	// modalPrefabs is the persistent stamp library (save/load/delete). See prefab.go.
	modalPrefabs
	// modalCount is the enum count sentinel (modalHandlers coverage assert in
	// draw.go). Must stay last; not a dispatchable modal.
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
	dragSelect     // marquee region drag (toolSelect): sets the copy/paste selection on release
	dragSelectMove // drag INSIDE a committed marquee (toolSelect): moves its contents on release
	dragMeasure    // ruler drag (toolMeasure): live tile-span readout, commits nothing
)

// toolMode is the active grid-paint tool. While Brush is active, modifiers
// override: Ctrl = Flood, Shift = Rect, Alt = Pick.
type toolMode int

const (
	toolBrush   toolMode = iota // freehand paint stroke (default)
	toolLine                    // straight line, click anchor → release endpoint
	toolRect                    // filled rectangle
	toolBox                     // hollow rectangle outline (handy for room walls)
	toolFlood                   // flood-fill the connected region
	toolPick                    // eyedropper: sample the cell's char into the brush
	toolSelect                  // marquee: drag a region to copy (Ctrl+C) / paste (Ctrl+V)
	toolMeasure                 // ruler: drag to read the spanned tile W×H + distance (no edit)
	// toolModeCount sizes the label/help tables (init-asserted below); keep last.
	toolModeCount
)

// toolModeLabels are the toolbar button captions, indexed by toolMode.
var toolModeLabels = [toolModeCount]string{
	toolBrush:   "Brush",
	toolLine:    "Line",
	toolRect:    "Rect",
	toolBox:     "Box",
	toolFlood:   "Flood",
	toolPick:    "Pick",
	toolSelect:  "Select",
	toolMeasure: "Measure",
}

// toolModeHelp is the hover-tooltip text per tool, indexed by toolMode.
var toolModeHelp = [toolModeCount]string{
	toolBrush:   "Paint freehand with the selected brush.",
	toolLine:    "Drag a straight line of tiles.",
	toolRect:    "Drag a filled rectangle.",
	toolBox:     "Drag a hollow rectangle — handy for room walls.",
	toolFlood:   "Flood-fill the connected same-tile region.",
	toolPick:    "Eyedropper — sample the clicked tile into the brush.",
	toolSelect:  "Marquee — drag a region, then Ctrl+C to copy, Ctrl+V to paste.",
	toolMeasure: "Ruler — drag to read the spanned tile size + distance (doesn't edit).",
}

// init trips at startup if a toolMode lacks a toolbar caption or hover help — the
// [toolModeCount] arrays above would otherwise silently leave a new tool blank.
func init() {
	for m := toolMode(0); m < toolModeCount; m++ {
		if toolModeLabels[m] == "" || toolModeHelp[m] == "" {
			panic("editor: toolMode " + strconv.Itoa(int(m)) + " missing a toolModeLabels/toolModeHelp entry")
		}
	}
}

// brushRef identifies a palette brush as (layer, index). Used by recent-brushes.
type brushRef struct {
	layer Layer
	idx   int
}

// maxRecentBrushes caps the recent-brush swatch row.
const maxRecentBrushes = 8

type statusEntry struct {
	msg   string
	timer float32
	warn  bool // tints the row danger-colored so warnings read distinctly
}

const undoLimit = 50

// State is the editor's mutable state across frames.
type State struct {
	area core.AreaDefinition

	// reachWarnings caches reachabilityWarnings(s.area) so the per-frame badge
	// doesn't re-run a full-grid BFS. reachValid=false means recompute on next
	// read; flipped false by every area mutation.
	reachWarnings []string
	reachValid    bool
	// contentEpoch bumps on every area mutation. Per-frame draws caching a
	// derived-from-content string key on it to rebuild only when the map changed.
	contentEpoch uint64

	layer    Layer
	brushIdx [layerCount]int
	// recentBrushes is the recent (layer, brush) picks, newest first, deduped and
	// capped at maxRecentBrushes; drawn as a swatch quick-pick row.
	recentBrushes []brushRef
	// layerHidden is a pure view filter hiding a layer from the grid draw (eye
	// toggles; Alt-click solos). Entities count as a layer here.
	layerHidden [layerCount]bool
	// editLevel is the active level (0..maxEditLevel): the Elevation brush stamps
	// it and every content paint lifts the tile to it. rampMode toggles the smart
	// ramp tool (left-click places, right-click clears).
	editLevel int
	rampMode  bool
	// sculptMode (Elevation layer): left-drag raises each brushed column +1 relative to
	// its current top, right-click lowers −1 — the terracing gesture, vs the "Set Height"
	// brush's absolute stamp. Mutually exclusive with rampMode.
	sculptMode bool
	// levelHidden[L] hides every tile on level L (per-level eye), except a ramp
	// connecting to the active level. Always-on Photoshop-style levels model.
	levelHidden [maxEditLevel + 1]bool
	// topLevel / bottomLevel bound the level rows the panel exposes; start equal
	// (ground only) and grow outward as floors come into play. Stored 0..maxEditLevel
	// (ground = core.ElevationBaseline) but displayed signed (stored − baseline).
	topLevel    int
	bottomLevel int
	// paletteScroll is the per-layer vertical scroll offset (px) of the brush list.
	paletteScroll [layerCount]float32
	// metadataScroll is the vertical scroll offset (px) of the right-hand MAP panel.
	metadataScroll float32

	focus      focusField
	numericBuf string

	modal         modalKind
	modalPaths    []string
	modalCursor   int
	modalFilename string
	// openFilter is the Open-modal's live type-to-filter query (case-insensitive map-id
	// substring); empty = show all. modalCursor indexes the FILTERED view, not modalPaths.
	openFilter    string
	modalRenaming string
	// modalRenamingActive is the Open-modal rename sub-mode flag. Separate from the
	// modalRenaming text so backspacing the field to empty doesn't silently exit rename
	// (the text can't double as the mode sentinel).
	modalRenamingActive bool
	modalConfirmDelete  bool
	// modalPackIdx / modalChestIdx / modalDoorIdx index the spawn being edited in
	// the corresponding modal; -1 outside the flow.
	modalPackIdx  int
	modalChestIdx int
	modalDoorIdx  int
	// doorPickMaps / doorPickDoors cache the door editor's target-map / target-door
	// picker rows, built from disk when each dropdown opens (not per frame). Cleared
	// when the door modal closes.
	doorPickMaps  []string
	doorPickDoors []string
	// Dialog-editor indices select the active Dialogs entry / node / choice; all
	// -1 outside their flows (reset by closeModal).
	modalDialogIdx       int
	modalDialogNodeIdx   int
	modalDialogChoiceIdx int
	// modalDialogCondIdx / modalDialogTriggerIdx select the active condition /
	// Triggers entry; -1 outside their flows.
	modalDialogCondIdx    int
	modalDialogTriggerIdx int
	// modalWallFeatureIdx indexes the WallFeatures entry being edited; -1 outside the flow.
	modalWallFeatureIdx int
	// modalLocationIdx indexes the Locations entry being edited; -1 outside the flow.
	modalLocationIdx int
	// modalCrystalIdx indexes the CrystalSpawns entry being edited; -1 outside the flow.
	modalCrystalIdx int
	// Jump-to-tile modal (modalGoto): gotoX/gotoZ are the edit buffers, gotoField picks
	// the focused one (0=X,1=Z). bookmarks are session view marks (click to recenter).
	gotoX, gotoZ   string
	gotoField      int
	bookmarks      []gotoBookmark
	bookmarkCursor int // scroll anchor for the bookmark list (mouse-wheel scrolled)
	// Prefab library modal (modalPrefabs): prefabName is the save-name buffer,
	// prefabNameFocus routes typing to it, prefabPaths caches the on-disk listing.
	prefabName      string
	prefabNameFocus bool
	prefabPaths     []string
	prefabCursor    int
	// modalDialogActionOnChoice: action editor targets the choice's EndAction when
	// true, else the node's. See currentDialogActionHolder.
	modalDialogActionOnChoice bool
	// dialogNumBuf is the shared edit buffer for the focused numeric dialog field
	// (only one numeric field is ever focused).
	dialogNumBuf string
	// dialogNumUndoBefore / dialogNumSnapDone give a focused numeric dialog field
	// the same lazy single-step undo as a paint stroke: snapshot the area on focus,
	// bank ONE step on the first real edit (focusDialogNumeric / pumpDialogNumeric).
	dialogNumUndoBefore core.AreaDefinition
	dialogNumSnapDone   bool
	// textUndoBefore / textUndoFocus / textUndoSnapped give focused PROSE text fields
	// (map Name/Quiet, door + location names/targets, dialog text/labels) the same
	// lazy single-step undo: armTextUndo snapshots the area when the field gains focus,
	// onFocusedTextEdit banks ONE step on the first keystroke. textUndoFocus is the
	// focus the snapshot belongs to (focusNone = disarmed).
	textUndoBefore  core.AreaDefinition
	textUndoFocus   focusField
	textUndoSnapped bool
	// New-map dialog state (in-progress dims + default floor char), committed by
	// blankArea on confirm — not the area until then.
	modalNewWidth  int
	modalNewHeight int
	modalNewFloor  byte
	// modalValidateRows snapshots the warnings shown in modalValidate (frozen at
	// open so the read-only display doesn't reflow while read).
	modalValidateRows []string
	// Sound modal state; on State so tuning survives open/close.
	soundParams    soundParamSet
	soundName      string
	soundCursor    int        // row cursor inside the sound modal
	soundLeftPanel soundPanel // which column of the sound modal has focus
	// soundParamScroll is the vertical scroll offset (px) of the params slider body.
	soundParamScroll float32
	// deleteArmed is the shared two-press delete guard token (first click arms,
	// second for the SAME token confirms). Backs sound/door deletes.
	deleteArmed string
	// soundSavedCache / soundAssignCache cache audio.ListUserSounds() and the
	// cue→user-sound assignment map; refreshed on open + each mutation, not per frame.
	soundSavedCache  []string
	soundAssignCache map[string]string

	// Foe Visualizer (modalFoeView) state. foeVisual is the working override the
	// sliders edit; foeBaseline snapshots at open for Reset. Persist across open/close.
	foeKind     core.EnemyKind
	foeVisual   core.EnemyVisualOverride
	foeBaseline core.EnemyVisualOverride
	foeCursor   int
	foeInit     bool

	// Party Visualizer (modalPartyView) state — the party-class twin of the foe fields.
	partyClass    core.PartyClass
	partyVisual   core.PartyVisualOverride
	partyBaseline core.PartyVisualOverride
	partyCursor   int
	partyInit     bool

	// foeViewTab selects the Foe/Party Visualizer pane: 0 = Layout, 1 = Asset.
	foeViewTab int
	// foeViewZoom is the preview dolly factor (clamped to render.FoePreviewZoom
	// {Min,Max}); reset to 1 on open.
	foeViewZoom float32
	// Asset-tab state. Adjustment VALUES live in the visual override, not here.
	// assetPreviewStale requests a live-preview rebuild next frame.
	assetCursor       int
	assetPreviewStale bool

	// objectViewPage is the current Object Browser page (modalObjectView).
	objectViewPage int
	// Object Browser per-item view: drag-rotate (yaw/pitch) + wheel-zoom, keyed by
	// object index so each thumbnail keeps its own pose. objViewDrag is the index
	// being drag-rotated (-1 = none).
	objViewView map[int]objPreview
	objViewDrag int

	pending pendingAction

	undo  []core.AreaDefinition
	redo  []core.AreaDefinition
	dirty bool
	// baseline snapshots the area as last loaded/saved, so undo/redo can clear
	// dirty when the working state matches disk again.
	baseline core.AreaDefinition

	// autosaveTimer accumulates edited-but-unsaved seconds; at autosaveInterval it
	// writes a crash-recovery snapshot (tickAutosave). Reset on save / clean state.
	autosaveTimer float32

	statusLog []statusEntry
	// statusHistory is the rolling recall buffer (newest last, capped) so a message
	// that auto-expired can still be read; showStatusLog toggles its panel (L key).
	statusHistory []string
	showStatusLog bool

	brushSize int
	// scatterDensity (Decor/Props layers, brush size > 1): 0 = off, else the per-cell
	// stamp probability so a big brush lays organic foliage fields instead of a solid
	// block. Cycled by the toolbar Scatter button.
	scatterDensity float32
	// brushYaw is the pending prop facing new Props paints inherit: -1 = auto
	// (procedural yaw), else a step in [0, core.PropYawSteps). Cycled with R on the
	// Props layer; R over an existing prop rotates that tile instead.
	brushYaw int

	// mirrorX / mirrorZ mirror every grid-layer stamp across the map's vertical /
	// horizontal center axis (live symmetry paint). mirroring guards applyTool against
	// re-entering itself while stamping the mirrored partners.
	mirrorX, mirrorZ bool
	mirroring        bool

	// tool is the active grid-paint tool. Zero value toolBrush = freehand paint.
	tool toolMode
	drag dragKind

	// Region copy/paste (toolSelect). selActive marks a committed marquee with
	// inclusive normalized bounds sel{X0,Z0,X1,Z1}; clipboard holds the last
	// Ctrl+C tile snapshot, clipEntities the spawns that sat on it.
	selActive                  bool
	selX0, selZ0, selX1, selZ1 int
	// cancelHandled: set within updateHotkeys when Esc was consumed this frame
	// (e.g. clearing a selection) so the same-frame pause-menu open is suppressed.
	cancelHandled bool
	clipboard     core.TileRegion
	// clipEntities are the spawn clones captured with clipboard (region-local coords),
	// so a paste reproduces the packs/chests/doors/crystals that sat on the region too.
	clipEntities regionEntities
	// dragSnapshotDone reports whether the stroke banked its undo snapshot.
	// dragUndoBefore is the pre-stroke area, committed LAZILY by strokePaint only
	// once the stroke mutates a cell (a no-op stroke banks nothing).
	dragSnapshotDone bool
	dragUndoBefore   core.AreaDefinition
	lastPaintX       int
	lastPaintZ       int
	rectAnchorX      int
	rectAnchorZ      int
	// rectHollow: a Box-tool rect drag, so finishDrag paints outline only.
	rectHollow  bool
	dragPackIdx int
	// dragChestIdx / dragDoorIdx mirror dragPackIdx for chest/door drag-move; -1 idle.
	dragChestIdx int
	dragDoorIdx  int

	hoverX int
	hoverZ int

	zoom    float32
	panX    float32
	panY    float32
	panning bool
	// minimapDragging: a left-press that began on the minimap; while held it drags
	// the view (continuous recenter), even once the cursor leaves the minimap rect.
	minimapDragging bool
	// rightDragStart / rightDragMoved disambiguate a right-button PAN drag from a
	// right-CLICK (context menu): the press records the start; movement past
	// panDragThreshold marks it a drag so the release opens no menu. The mousewheel
	// BUTTON is never bound — right-drag replaced the old middle-drag pan.
	rightDragStart rl.Vector2
	rightDragMoved bool
	// scrollDrag is cross-frame memory for an in-flight scrollbar thumb drag.
	// Zero value (scrollNone) = no drag. See scrollbar.go.
	scrollDrag scrollDragState

	exitRequested     bool
	testRequested     bool
	awaitingOverwrite bool

	// showTileGlyphs toggles the per-tile char overlay (active layer only).
	// Flipped by ALT tapped alone; altChordUsed suppresses the flip when Alt was
	// chorded (e.g. Alt+1..6 layer jump).
	showTileGlyphs bool
	altChordUsed   bool
	// showDoorLinks toggles the door-link diagnostic overlay (connectors +
	// warning rings on unresolved targets).
	showDoorLinks bool
	// showHeatmap toggles the coverage heatmap (top-down): tiles tinted by walking
	// distance from the start, with unreachable pockets flagged. Reveals dead
	// stretches + pacing a binary reachability check can't.
	showHeatmap bool

	// isoView swaps to a rotatable 3D block view; suppresses the top-down
	// screen→tile path (cellAt early-returns). See iso.go.
	isoView bool
	// animateObjects toggles foliage sway / torch flicker in the 3D view (View
	// menu). Default off: a still scene is calmer to author in and cheaper.
	animateObjects bool
	// 3D-view camera + pick state (iso.go). isoHoverX/Z is the ray-picked column
	// (-1 when off-canvas); isoRT is the off-screen render target, isoRTW/H its
	// size for lazy reallocation. isoYaw/isoPitch are the continuous orbit angles
	// in radians (right-drag tumbles; Q/E snap yaw by 90°).
	isoYaw, isoPitch       float32
	isoZoom                float32
	isoTargetX, isoTargetZ float32
	isoHoverX, isoHoverZ   int
	isoRT                  rl.RenderTexture2D
	isoRTW, isoRTH         int32
	// isoReqW/H is the size ensureIsoRT was asked for LAST frame. While the request
	// keeps changing (window drag-resize), the realloc is deferred and the existing
	// RT reused — reallocating the GPU FBO mid-resize then rendering into it is the
	// intermittent DrawModelEx crash. Realloc only fires once the size settles.
	isoReqW, isoReqH int32
	// isoRTSettle counts consecutive frames the requested size has held steady.
	// A native window-border drag doesn't reliably report the mouse button as down,
	// so we can't lean on that: the realloc waits until the size is stable for
	// isoRTSettleFrames frames, riding out the brief pauses within a resize drag.
	isoRTSettle int
	// isoPreview caches a NewGameState(area) powering the 3D view's entity/foe
	// draw (chests/doors/crystals/packs); rebuilt when contentEpoch changes so
	// the heavy build runs once per edit, not per frame. See ensureIsoPreview.
	isoPreview      *core.GameState
	isoPreviewEpoch uint64
	// isoSpan{Min,Max} caches the 3D view's level span (isoLevelSpan) on
	// contentEpoch so Update + Draw don't each rescan the whole grid per frame.
	isoSpanMin, isoSpanMax int
	isoSpanEpoch           uint64
	isoSpanReady           bool

	// Ctrl+F5 "test from cursor": when set, the run loop uses testStartOverrideX/Z
	// as the playtest start, then resets the flag.
	testStartOverride  bool
	testStartOverrideX int
	testStartOverrideZ int

	// previewPhase is the day/night phase to playtest in (cycled with T), seeding
	// g.StepCount on F5 via PreviewStepCount().
	previewPhase core.TimeOfDay

	// dropdown is the single open-dropdown slot. Zero value (ddNone) = closed.
	// See dropdown.go. The grid right-click menu is the ddContext owner.
	dropdown dropdownState
	// recentSnapshot caches the recent-maps list (from disk) as of when a menu was
	// opened, so building the File menu's rows every frame doesn't re-read prefs.
	recentSnapshot []string
	// ctxTile{X,Z} remember the tile the open ddContext right-click menu acts on.
	ctxTileX, ctxTileZ int
	// faceTarget* remember the tile + direction the open ddFaceSkin dropdown edits.
	// faceTargetDir is a core direction (0=N..3=W), or -1 for all faces (base skin).
	faceTargetX, faceTargetZ, faceTargetDir int
	// wallFace{X,Z} is the tile the open modalWallFaces edits. See wallfaces.go.
	wallFaceX, wallFaceZ int

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

// tileCorner returns the screen-space top-left corner of tile (tx,tz) under the
// current zoom/pan. Single source of truth for tile coord → screen.
func (r layoutRect) tileCorner(tx, tz int) (float32, float32) {
	return r.gridX + float32(tx)*r.cellPx, r.gridY + float32(tz)*r.cellPx
}

// tileCenter is tileCorner shifted by half a cell (marker/arrow/badge anchor).
func (r layoutRect) tileCenter(tx, tz int) (float32, float32) {
	return r.gridX + (float32(tx)+0.5)*r.cellPx, r.gridY + (float32(tz)+0.5)*r.cellPx
}

// tileRect returns a screen-space rectangle covering one tile.
func (r layoutRect) tileRect(tx, tz int) rl.Rectangle {
	x, y := r.tileCorner(tx, tz)
	return rl.NewRectangle(x, y, r.cellPx, r.cellPx)
}

// Area returns a copy of the area being edited (for the F5 playtest path).
func (s State) Area() core.AreaDefinition {
	return core.CloneArea(s.area)
}

// ReachabilityWarnings returns the cached reachabilityWarnings(s.area),
// recomputing only when the area changed (the check is a full-grid BFS run once
// per draw). reachValid is invalidated by every area mutation.
func (s *State) ReachabilityWarnings() []string {
	if !s.reachValid {
		s.reachWarnings = reachabilityWarnings(s.area)
		s.reachValid = true
	}
	return s.reachWarnings
}

// PreviewStepCount returns the StepCount that lands the player at the start of
// the selected preview phase, seeding the F5 playtest's lighting.
func (s State) PreviewStepCount() int {
	return int(s.previewPhase) * core.StepsPerPhase
}

// TestStartOverride returns (x, z, true) when the last ActionTest was a Ctrl+F5
// test-from-cursor, else (_, _, false). Reset by ClearTestStartOverride.
func (s State) TestStartOverride() (int, int, bool) {
	return s.testStartOverrideX, s.testStartOverrideZ, s.testStartOverride
}

// ClearTestStartOverride drops the one-shot test-from-cursor override.
func (s *State) ClearTestStartOverride() {
	s.testStartOverride = false
}

// New starts the editor with a blank default-sized FloorAuto map (the in-editor
// New flow goes through modalNew).
func New() State {
	return freshState(blankArea(core.DefaultNewMapDimension, core.DefaultNewMapDimension, core.FloorAuto))
}

// NewDefault is the title-screen entry: reopen the last map worked on (per
// editorprefs), falling back to a blank map when there's no valid last map.
func NewDefault() State {
	// A leftover recovery snapshot (crash / abrupt quit before the last save) takes
	// priority — reopen it with dirty set so the edits aren't lost.
	if area, ok := loadRecovery(); ok {
		s := NewFromArea(area)
		diskBaseline := false
		if area.Path != "" {
			if disk, err := core.LoadArea(area.Path); err == nil {
				s.baseline = core.CloneArea(disk) // revert/dirty compare against real disk state
				diskBaseline = true
			}
		}
		surfaceAreaLevels(&s)
		// Only a real disk baseline can be "stale" (snapshot == saved file). A
		// never-saved map (Path == "") or an unreadable/deleted baseline has NO other
		// copy: freshState seeded baseline = clone(area), so AreaContentEqual is
		// vacuously true — dropping it here would silently lose the recovered work.
		// Keep it dirty in that case.
		if diskBaseline && core.AreaContentEqual(s.area, s.baseline) {
			clearRecovery() // stale snapshot equals disk — drop it and open normally
		} else {
			s.dirty = true
			s.flash("Recovered unsaved changes — Save to keep them")
			return s
		}
	}
	if path := LastMapPath(); path != "" {
		if area, err := core.LoadArea(path); err == nil {
			s := NewFromArea(area)
			surfaceAreaLevels(&s) // open with the map's full level range + start floor
			return s
		}
	}
	return New()
}

// NewFromArea opens the editor on an already-loaded area.
func NewFromArea(a core.AreaDefinition) State {
	return freshState(a)
}

// materializeEntranceCrystal turns the runtime's default entrance crystal into a
// real editable CrystalSpawn for any map that hasn't authored crystals, marking
// it authored so saving persists the exact set (including an intentional empty
// one). Already-authored maps are untouched. Shared by every area-swap entry point.
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
		layer:                 LayerFloor, // Walls/"Faces" is no longer a selectable paint layer (see selectableLayers)
		editLevel:             core.ElevationBaseline,
		topLevel:              core.ElevationBaseline,
		bottomLevel:           core.ElevationBaseline,
		brushSize:             1,
		brushYaw:              -1, // props default to procedural (auto) facing
		zoom:                  1,
		hoverX:                -1,
		hoverZ:                -1,
		isoView:               true, // 3D is the default authoring view
		isoZoom:               1,
		isoYaw:                isoDefaultYaw,
		isoPitch:              isoDefaultPitch,
		isoHoverX:             -1,
		isoHoverZ:             -1,
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
		modalWallFeatureIdx:   -1,
		modalLocationIdx:      -1,
		modalCrystalIdx:       -1,
	}
}

func blankArea(w, h int, floorChar byte) core.AreaDefinition {
	walls := make([]string, h)
	floor := make([]string, h)
	decor := make([]string, h)
	props := make([]string, h)
	ceiling := make([]string, h)
	elevation := make([]string, h)
	// Interior at baseline; outer ring raised one level to form an enclosing wall.
	// Ring face skins are explicit rock; the interior carries the default skin.
	baseChar := core.ElevationChar(core.ElevationBaseline)
	wallChar := core.ElevationChar(core.ElevationWallRingLevel)
	for z := 0; z < h; z++ {
		wb := make([]byte, w)
		eb := make([]byte, w)
		for x := 0; x < w; x++ {
			if x == 0 || z == 0 || x == w-1 || z == h-1 {
				wb[x] = core.TileRock
				eb[x] = wallChar
			} else {
				wb[x] = core.TileOpen
				eb[x] = baseChar
			}
		}
		walls[z] = string(wb)
		elevation[z] = string(eb)
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

// activeBrush returns the selected brush in the active layer. On LayerElevation
// the Char is overwritten with the height selector's current level.
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

// clampLevel bounds a STORED level into [0, maxEditLevel].
func clampLevel(l int) int {
	return core.Clamp(l, 0, maxEditLevel)
}

// signedLevelLabel renders a stored level signed relative to ground: "0", "+3", "-2".
func signedLevelLabel(stored int) string {
	d := stored - core.ElevationBaseline
	if d > 0 {
		return "+" + strconv.Itoa(d)
	}
	return strconv.Itoa(d) // strconv prints the leading '-' for negatives
}

// Update advances the editor one frame, returning the next action.
func Update(s *State, dt float32) Action {
	s.layout()

	tickStatusLog(s, dt)
	tickAutosave(s, dt)

	// Disarm the prose-text-field undo session once its field loses focus, so
	// re-entering the SAME field later arms a fresh snapshot (armTextUndo keys on
	// focus identity). Runs before input so a same-frame refocus re-arms cleanly.
	if s.focus == focusNone && s.textUndoFocus != focusNone {
		s.textUndoFocus = focusNone
	}

	if s.modal != modalNone {
		return updateModal(s)
	}

	if s.focus != focusNone {
		updateTextInput(s)
		if rl.IsMouseButtonPressed(rl.MouseLeftButton) && !pointIn(rl.GetMousePosition(), s.activeFieldRect()) {
			// Click-outside must run the same finalization as Enter/Tab so the
			// focused width/height field commits its typed value (a half-typed
			// dimension would otherwise be silently dropped).
			finalizeFocusedField(s)
			s.focus = focusNone
		}
		return ActionNone
	}

	// An open dropdown owns input; canvas + hotkeys stay inert behind it.
	if s.dropdownOpen() {
		updateDropdown(s)
		return ActionNone
	}

	// (The right-click menu is a dropdown now, so the s.dropdownOpen() gate above
	// already absorbs input while it's open — hotkeys and the mouse stay inert.)
	s.cancelHandled = false
	updateHotkeys(s)
	updateMouse(s)

	if s.testRequested {
		s.testRequested = false
		// Validate the spawn the playtest will ACTUALLY use: a Ctrl+F5 cursor test
		// spawns at testStartOverride*, so gate those coords.
		checkArea := s.area
		if s.testStartOverride {
			checkArea.StartTileX = s.testStartOverrideX
			checkArea.StartTileZ = s.testStartOverrideZ
		}
		// Physical-only gate (out of bounds / inside geometry / on a chest).
		// Reachability is NOT a gate — it lives in the at-will Validate modal.
		if !canPlaytest(checkArea) {
			s.flash("Test: " + startTileBlocker(checkArea))
			return ActionNone
		}
		// Cross-map door validation runs once here (disk I/O, too costly per frame).
		// Non-blocking — the runtime tolerates broken doors; this just informs.
		for _, w := range crossMapDoorWarnings(s.area) {
			s.flash("Doors: " + w)
		}
		return ActionTest
	}

	// Esc opens the pause menu (modalEscMenu), not a direct exit. The
	// exitRequested path still runs the dirty-bounce-then-exit flow.
	if editorCancelPressed() && !s.cancelHandled {
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

// canPlaytest is the strict subset of start-tile checks that must pass before
// dropping into adventure mode. Delegates to startTileBlocker so the gate and the
// reachability warnings stay in sync.
func canPlaytest(a core.AreaDefinition) bool {
	return startTileBlocker(a) == ""
}

// statusLogLifetime seeds a flash() entry's timer; drawStatus decays alpha against it.
const statusLogLifetime = float32(2.5)

// statusLogMaxEntries caps stacked transient messages.
const statusLogMaxEntries = 4

// flash pushes a transient status message, refreshing in place if already present.
func (s *State) flash(msg string) { s.pushStatus(msg, false) }

// flashWarn pushes a warning-tinted (danger-colored) status row.
func (s *State) flashWarn(msg string) { s.pushStatus(msg, true) }

// statusHistoryMax caps the recall buffer (L-toggle panel).
const statusHistoryMax = 30

func (s *State) pushStatus(msg string, warn bool) {
	// Rolling history so an expired toast can still be recalled (deduped vs. the last
	// entry so a repeated flash doesn't spam it). Warnings get a leading marker.
	rec := msg
	if warn {
		rec = "! " + msg
	}
	if n := len(s.statusHistory); n == 0 || s.statusHistory[n-1] != rec {
		s.statusHistory = append(s.statusHistory, rec)
		if len(s.statusHistory) > statusHistoryMax {
			s.statusHistory = s.statusHistory[len(s.statusHistory)-statusHistoryMax:]
		}
	}
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

// tickAutosave writes a crash-recovery snapshot once the map has been dirty for
// autosaveInterval seconds, then re-arms. A clean map clears the timer (a manual
// save already dropped the recovery file via writeAreaTo).
func tickAutosave(s *State, dt float32) {
	if !s.dirty {
		s.autosaveTimer = 0
		return
	}
	s.autosaveTimer += dt
	if s.autosaveTimer >= autosaveInterval {
		s.autosaveTimer = 0
		writeRecovery(s.area)
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

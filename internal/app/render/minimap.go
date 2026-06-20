package render

import (
	"image/color"
	"math"

	"crawler/internal/app/core"
	rl "github.com/gen2brain/raylib-go/raylib"
)

// Minimap geometry. Width / height pre-computed so MinimapWidth /
// MinimapBottomY (below) can be called by sibling HUD panels — the
// turn-order column docks directly under the minimap on the left
// edge using these values instead of duplicating the literals.
//
// minimapHeader is now the area-name strip (no "AREA" tick label —
// just the name in soft type) plus a touch of top breathing room.
// Footer is the time-of-day phase bar beneath the grid.
const (
	minimapCell      = int32(12)
	minimapViewCells = int32(17) // odd so the player sits dead-center; wider window shows more surrounding area
	minimapHeader    = int32(26)
	minimapFooter    = int32(52) // time-of-day strip beneath the grid (label line + gap + bar + cursor pips)
	minimapPanelW    = minimapViewCells*minimapCell + 16
	minimapPanelH    = minimapViewCells*minimapCell + 16 + minimapHeader + minimapFooter
)

// MinimapWidth is the on-screen width of the corner minimap card.
// Used by the turn-order / action-log panels (which sit beneath the
// minimap on the same left edge) so they can match its width
// without baking in a literal.
func MinimapWidth() int32 { return minimapPanelW }

// MinimapBottomY returns the Y screen coordinate of the bottom edge
// of the minimap card. The turn-order panel anchors here plus a
// hudColumnGap so the two read as a single stacked column.
func MinimapBottomY() int32 { return hudEdgePad + minimapPanelH }

func drawMinimap(m *core.AreaDefinition, g *core.GameState, assets Resources) {
	cell := minimapCell
	viewCells := minimapViewCells
	pad := hudEdgePad
	header := minimapHeader
	p := g.Player
	half := int(viewCells / 2)
	startX := p.TileX - half
	startZ := p.TileZ - half
	gridSize := viewCells * cell
	panelW := minimapPanelW
	panelH := minimapPanelH

	drawPanelCard(pad, pad, panelW, panelH)
	areaName := g.Area.Name
	if areaName != "" {
		// Area name reads as the panel's natural label — no "AREA"
		// tick. Centered above the grid so it doesn't crowd the
		// upper-left wood mitre.
		nameMeasure := minimapMeasureCache.measure(assets.hudFont, areaName, FontSmall, 1)
		nameX := float32(pad) + (float32(panelW)-nameMeasure.X)/2
		drawTextWithShadow(assets.hudFont, areaName, nameX, float32(pad+6), FontSmall, textMuted)
	}

	gridX := pad + 8
	gridY := pad + 8 + header
	footerY := gridY + gridSize + 12
	drawMinimapGridBacking(gridX, gridY, gridSize)
	drawMinimapTimeOfDay(assets.hudFont, g.StepCount, pad+14, footerY, panelW-28)

	// One MaterialIsIndoor lookup for the whole grid (a per-area constant),
	// passed into each cell rather than recomputed per cell.
	indoor := core.MaterialIsIndoor(m.Materials)
	// Single mapSliceCell pass over the window PLUS a one-cell border, recorded
	// into the slice/seenWall grids. The fill draws the window cells; the outline
	// below reads the grids (including the border ring) for O(1) neighbour tests —
	// so MapSurfaceAt runs once per window+border cell instead of once for the fill
	// and again ~5× per cell for the outline's self+neighbour lookups.
	vc := int(viewCells)
	gw := vc + 2
	slice := make([]bool, gw*gw)
	seen := make([]bool, gw*gw)
	ramp := make([]int8, gw*gw)
	for localZ := -1; localZ <= vc; localZ++ {
		for localX := -1; localX <= vc; localX++ {
			col, onSlice, seenWall, rampDir := mapSliceCell(m, g, indoor, startX+localX, startZ+localZ)
			i := (localZ+1)*gw + (localX + 1)
			slice[i], seen[i], ramp[i] = onSlice, seenWall, rampDir
			if localX >= 0 && localX < vc && localZ >= 0 && localZ < vc {
				rl.DrawRectangle(gridX+int32(localX)*cell, gridY+int32(localZ)*cell, cell-1, cell-1, col)
			}
		}
	}
	// Editor-style silhouette: a border ONLY where explored current-level floor
	// abuts a seen wall — drawn over the cell fills.
	drawMapLevelOutline(slice, seen, gw, vc, vc, float32(gridX), float32(gridY), float32(cell), float32(cell))
	// Up/down stair glyphs on ramp cells, drawn over the fill + outline.
	drawMapStairIcons(ramp, gw, vc, vc, float32(gridX), float32(gridY), float32(cell), float32(cell))

	// Pack markers intentionally omitted from the corner minimap too:
	// matches the Map panel's fog-of-war rule (terrain only, enemies
	// stay off the map). The world view is the only place to see who's
	// around.

	drawMinimapArrow(
		rl.NewVector2(float32(gridX+gridSize/2), float32(gridY+gridSize/2)),
		p.Facing,
	)
	drawMinimapCartographerFrame(assets.hudFont, gridX, gridY, gridSize)
}

func drawMinimapGridBacking(x, y, size int32) {
	drawGlassPane(x-3, y-3, size+5, size+5, fadeColor(glassWarm, 0.55))
	rl.DrawRectangle(x-1, y-1, size+1, size+1, fadeColor(woodInlay, 0.65))
}

func drawMinimapCartographerFrame(font rl.Font, x, y, size int32) {
	frame := rl.NewRectangle(float32(x-3), float32(y-3), float32(size+5), float32(size+5))
	rl.DrawRectangleLinesEx(frame, 2, woodAccentFrame)
	inner := rl.NewRectangle(float32(x), float32(y), float32(size-1), float32(size-1))
	rl.DrawRectangleLinesEx(inner, 1, fadeColor(giltDim, 0.55))

	// Subtle crosshair and rule marks: enough to read as a cartographer's
	// plate without covering the actual dungeon cells.
	mid := size / 2
	ruleCol := fadeColor(giltDim, 0.22)
	rl.DrawRectangle(x+mid, y+2, 1, size-5, ruleCol)
	rl.DrawRectangle(x+2, y+mid, size-5, 1, ruleCol)
	for i := int32(4); i < size-4; i += minimapCell * 4 {
		rl.DrawRectangle(x+i, y-2, 1, 4, fadeColor(giltDim, 0.38))
		rl.DrawRectangle(x+i, y+size-2, 1, 4, fadeColor(giltDim, 0.28))
		rl.DrawRectangle(x-2, y+i, 4, 1, fadeColor(giltDim, 0.38))
		rl.DrawRectangle(x+size-2, y+i, 4, 1, fadeColor(giltDim, 0.28))
	}

	pipCol := fadeColor(giltBright, 0.72)
	drawDiamondPip(float32(x-3), float32(y-3), 2.6, pipCol)
	drawDiamondPip(float32(x+size+2), float32(y-3), 2.6, pipCol)
	drawDiamondPip(float32(x-3), float32(y+size+2), 2.6, pipCol)
	drawDiamondPip(float32(x+size+2), float32(y+size+2), 2.6, pipCol)

	// Compass initials sit JUST INSIDE the grid edges now (N/S used to float
	// above/below the plate, reading as detached). Each gets a strong dark halo
	// (drawCompassLabel) so it stays legible over both the dark walkable floor
	// and the light blocker cells it may overlap.
	nCol := fadeColor(inkAccent, 0.92)
	nm := minimapMeasureCache.measure(font, "N", FontTiny, 1)
	sm := minimapMeasureCache.measure(font, "S", FontTiny, 1)
	wm := minimapMeasureCache.measure(font, "W", FontTiny, 1)
	em := minimapMeasureCache.measure(font, "E", FontTiny, 1)
	drawCompassLabel(font, "N", float32(x)+float32(size)/2-nm.X/2, float32(y)+3, nCol)
	drawCompassLabel(font, "S", float32(x)+float32(size)/2-sm.X/2, float32(y+size)-sm.Y-3, nCol)
	drawCompassLabel(font, "W", float32(x)+4, float32(y)+float32(size)/2-wm.Y/2, nCol)
	drawCompassLabel(font, "E", float32(x+size)-em.X-4, float32(y)+float32(size)/2-em.Y/2, nCol)
}

// drawCompassLabel paints a minimap compass initial with a strong dark halo —
// a 6-way black offset under the colored glyph — so the letter reads over both
// the dark floor and the bright blocker cells now that N/S sit inside the grid.
// Heavier than drawTextWithShadow's single drop, which washed out over light cells.
func drawCompassLabel(font rl.Font, s string, x, y float32, col rl.Color) {
	halo := fadeColor(rl.Black, 0.85)
	for _, d := range compassHaloOffsets {
		rl.DrawTextEx(font, s, rl.NewVector2(x+d[0], y+d[1]), FontTiny, 1, halo)
	}
	rl.DrawTextEx(font, s, rl.NewVector2(x, y), FontTiny, 1, col)
}

var compassHaloOffsets = [6][2]float32{{-1, 0}, {1, 0}, {0, -1}, {0, 1}, {1, 1}, {-1, -1}}

// minimapPropColors keys every prop-layer char to its minimap swatch.
// Map-driven instead of a switch so a missing entry stands out at code
// review and the unknown-tile fallback path stays a single branch.
// Must enumerate the same set as core.IsPropChar's switch in
// internal/app/core/map.go — a future prop char goes in both lists.
// Colors are deliberately distinct from the editor's tileColorByChar:
// the minimap tone palette is tuned for the small-rendered case, not
// for matching the editor swatch exactly.
// init asserts every prop-tile char registered in core.PropTileChars
// has a minimap color, AND that every blocking-floor char (currently
// FloorDeepWater; future lava etc.) has a dedicated case in
// minimapTileColor's switch. Without these guards, a new blocker tile
// silently fell back to the "unknown tile" tone on the minimap —
// catching it at startup is far cheaper than noticing in playtest.
func init() {
	for _, c := range core.PropTileChars() {
		if _, ok := minimapPropColors[c]; !ok {
			panic("render/minimap: missing color for prop tile " + string(c))
		}
	}
	// Reverse coverage: a color keyed to a char core no longer (or never)
	// recognises as a prop is dead data that the forward loop can't catch —
	// it means minimapPropColors and core's prop set have drifted (a rename,
	// a removed tile). Fail at startup so the two lists stay in lockstep both
	// ways, not just "every core prop has a color."
	for c := range minimapPropColors {
		if !core.IsPropChar(c) {
			panic("render/minimap: minimapPropColors has color for '" + string(c) + "' which is not a core prop tile — remove it or add it to core.PropTileChars")
		}
	}
	fieldIndoor := core.MaterialIsIndoor(core.MaterialField)
	for _, c := range core.BlockingFloorChars() {
		col := minimapTileColor(fieldIndoor, c)
		fallback := minimapTileColor(fieldIndoor, core.TileOpen)
		if col == fallback {
			panic("render/minimap: blocking floor tile '" + string(c) + "' falls through to the open-tile color — add an explicit case in minimapTileColor")
		}
	}
}

// minimapPropAlpha is the opacity every prop swatch shares; minimapStructAlpha
// is the slightly-less-opaque alpha the structural wall/water/floor tones use.
// Named so the repeated byte isn't hand-keyed across each NewColor entry.
const (
	minimapPropAlpha   uint8 = 240
	minimapStructAlpha uint8 = 235
)

// minimapProp builds a prop swatch at the shared minimapPropAlpha so each table
// entry carries only its hue, not a hand-keyed fourth byte that could drift.
func minimapProp(r, g, b uint8) rl.Color { return rl.NewColor(r, g, b, minimapPropAlpha) }

var minimapPropColors = map[byte]rl.Color{
	core.TileTree:              minimapProp(42, 132, 56),
	core.TileTreeXL:            minimapProp(28, 102, 44),
	core.TileTreeTall:          minimapProp(36, 118, 50),
	core.TileTreeTwin:          minimapProp(40, 124, 58),
	core.TileTreeYoung:         minimapProp(96, 168, 88),
	core.TileRockLarge:         minimapProp(120, 116, 108),
	core.TileBushLarge:         minimapProp(110, 168, 92),
	core.TileCrate:             minimapProp(168, 122, 72),
	core.TileBarrel:            minimapProp(148, 100, 60),
	core.TileUrn:               minimapProp(186, 112, 72),
	core.TileStalagmite:        minimapProp(196, 188, 174),
	core.TilePillar:            minimapProp(214, 206, 188),
	core.TileBrokenPillar:      minimapProp(180, 172, 156),
	core.TileStatue:            minimapProp(228, 220, 204),
	core.TileObelisk:           minimapProp(86, 90, 104),
	core.TileFountain:          minimapProp(96, 158, 208),
	core.TileRockCairn:         minimapProp(150, 138, 116),
	core.TileRockFormation:     minimapProp(118, 102, 86),
	core.TileRockFormationTail: minimapProp(118, 102, 86),
	core.TileWell:              minimapProp(132, 138, 142),
	core.TileGravestone:        minimapProp(168, 162, 152),
	core.TileSignPost:          minimapProp(160, 110, 64),
	core.TileHayBale:           minimapProp(216, 184, 110),
	core.TileScarecrow:         minimapProp(196, 162, 96),
	core.TileBookshelf:         minimapProp(132, 90, 56),
	core.TileTable:             minimapProp(160, 116, 72),
	core.TileBed:               minimapProp(176, 90, 96),
	core.TileBrazier:           minimapProp(220, 132, 64),
	core.TileTorch:             minimapProp(240, 168, 96),
	core.TileSarcophagus:       minimapProp(200, 192, 174),
	core.TilePropExoticFlower:  minimapProp(206, 110, 170),
	core.TilePropTallFern:      minimapProp(90, 146, 86),
	core.TilePropGrassTuft:     minimapProp(140, 178, 108),
}

// minimapPropColorTable / minimapPropColorPresent mirror minimapPropColors into
// a [256] array indexed by tile char, built once at init. The corner minimap
// recolors viewCells×viewCells cells every frame (and the Map tab the whole
// grid), so the per-cell lookup runs hundreds of times a frame — array indexing
// skips the map hash, matching the inlinePropTable / faceVariantTable pattern
// used elsewhere in the renderer. minimapPropColors stays the authored source of
// truth (the init coverage checks read it); this is a generated read cache.
var minimapPropColorTable, minimapPropColorPresent = buildMinimapPropColorTable()

func buildMinimapPropColorTable() ([256]rl.Color, [256]bool) {
	var t [256]rl.Color
	var p [256]bool
	for c, col := range minimapPropColors {
		// Normalize toward a common light "blocker" tone as the cache is built —
		// the authored hues stay the source of truth, but the rendered map reads
		// as dark=walkable / light=blocked rather than a rainbow of props.
		t[c], p[c] = normalizeMapProp(col), true
	}
	return t, p
}

// mapPropBase is the common light tone every prop swatch is pulled toward so the
// map reads as a clean walkable/blocked contrast; mapPropTint is how much of the
// authored hue survives over it. Tuned for "slightly distinguishable, not a
// rainbow" — enough tint to tell foliage from masonry, not enough to fight the
// floor/wall legibility convention. One knob each, in one place.
var mapPropBase = rl.NewColor(150, 146, 136, minimapPropAlpha)

const mapPropTint = 0.30 // fraction of the authored hue retained over the base

// normalizeMapProp blends an authored prop swatch toward mapPropBase, keeping
// mapPropTint of its original hue so props vary slightly without returning to a
// per-prop rainbow.
func normalizeMapProp(c rl.Color) rl.Color {
	out := core.MixColor(mapPropBase, c, mapPropTint)
	out.A = minimapPropAlpha
	return out
}

// minimapTileColor maps a composed tile char (from AreaDefinition.TileAt)
// to its 2D-map swatch, following a strict legibility convention:
// WALKABLE tiles are DARK and BLOCKING tiles (walls, deep water, props)
// are LIGHT. The dark floor reads as the negative space you move through
// and every solid thing pops brighter against it, so "where can I walk"
// is obvious at a glance. Tones stay above the near-black fog
// (mapTileFogColor) so explored-but-walkable never reads as unrevealed.
// Shared by the corner minimap and the panels Map tab via mapSliceCell.
// minimap floor/wall/water tones. Hoisted out of minimapTileColor (like the
// prop hues above and the time-of-day / player-arrow tints below) so the
// structural-tile palette lives in one named block rather than as inline
// NewColor literals mid-switch. Walls read LIGHT (the brightest blocker, framing
// the dark walkable space); deep water reads as a bright impassable blue (it
// blocks movement); walkable floor stays deliberately dark with a faint biome
// tint so indoor vs outdoor still distinguish without lifting into blocker
// brightness.
var (
	minimapWallIndoor   = rl.NewColor(178, 176, 168, minimapStructAlpha)
	minimapWallOutdoor  = rl.NewColor(160, 158, 150, minimapStructAlpha)
	minimapDeepWater    = rl.NewColor(122, 172, 216, minimapStructAlpha)
	minimapFloorIndoor  = rl.NewColor(44, 46, 52, minimapStructAlpha)
	minimapFloorOutdoor = rl.NewColor(38, 56, 40, minimapStructAlpha)
)

// mapNonBlockingPropTint is how much of a non-blocking prop's hue survives over
// the floor tone on the map. Small on purpose: a torch / flower / fern / grass
// tuft is walk-through, so it should read ALMOST as floor — only props that
// actually block your path get their full bright swatch.
const mapNonBlockingPropTint = 0.22

// minimapTileColor maps a tile char to its swatch. `indoor` is the area's
// MaterialIsIndoor verdict, hoisted to the caller so the per-cell loop computes
// it once per draw rather than re-running findMaterialDef for every cell.
func minimapTileColor(indoor bool, tile byte) color.RGBA {
	floor := minimapFloorOutdoor
	if indoor {
		floor = minimapFloorIndoor
	}
	switch {
	case tile == core.TileRock:
		if indoor {
			return minimapWallIndoor
		}
		return minimapWallOutdoor
	case tile == core.FloorDeepWater:
		return minimapDeepWater
	default:
		if minimapPropColorPresent[tile] {
			if core.PropIsNonBlocking(tile) {
				// Walk-through props (torch, flower, fern, grass) read ALMOST as
				// floor — just a faint hint of their hue — so they don't masquerade
				// as obstacles. Only blocking props keep their full bright swatch.
				out := core.MixColor(floor, minimapPropColorTable[tile], mapNonBlockingPropTint)
				out.A = minimapStructAlpha
				return out
			}
			// Blocking props sit lighter than the dark walkable floor below, so
			// they read as "stuff in the way."
			return minimapPropColorTable[tile]
		}
		return floor
	}
}

// mapRampColor is the swatch for a ramp connecting to/from the current level —
// a warm brass that reads as "the way up/down," distinct from the dark walkable
// floor and the light wall/cliff tones so connections between levels stand out.
var mapRampColor = rl.NewColor(198, 168, 104, minimapStructAlpha)

// Level-slice fade: a floor below the observer recedes geometrically toward the
// fog so it reads as "down there" context without competing with the current
// level — the in-game mirror of the editor canvas's levelDistanceFade. Clamped
// so even the deepest visible floor keeps a faint read. The map recolors many
// cells per frame, so the falloff is a precomputed table (no per-cell math.Pow).
const (
	mapLevelFadeFalloff = 0.55
	mapLevelFadeMin     = float32(0.18)
)

var mapLevelFadeTable = func() [core.MaxElevationLevel + 1]float32 {
	var t [core.MaxElevationLevel + 1]float32
	for d := range t {
		f := float32(math.Pow(mapLevelFadeFalloff, float64(d)))
		if f < mapLevelFadeMin {
			f = mapLevelFadeMin
		}
		t[d] = f
	}
	return t
}()

func mapLevelFadeFor(d int) float32 {
	if d < 0 {
		d = -d
	}
	if d >= len(mapLevelFadeTable) {
		return mapLevelFadeMin
	}
	return mapLevelFadeTable[d]
}

// mapSurfaceColor turns an observer-relative column classification (core's level
// slice) into its map swatch: the current level at full strength, walls/cliffs
// light, ramps brass, and floors below faded toward the fog by their depth. Void
// reads as fog (same as unrevealed) so an open shaft doesn't paint a phantom
// floor. Shared by the corner minimap and the Map tab via mapSliceCell.
// (The Map tab no longer fills MapSurfaceWall — see mapSliceCell — but this still
// resolves every other kind for both surfaces.)
func mapSurfaceColor(m *core.AreaDefinition, indoor bool, x, z int, surf core.MapSurface) rl.Color {
	switch surf.Kind {
	case core.MapSurfaceWall:
		if indoor {
			return minimapWallIndoor
		}
		return minimapWallOutdoor
	case core.MapSurfaceRamp:
		return mapRampColor
	case core.MapSurfaceBelow:
		base := minimapTileColor(indoor, m.TileAt(x, z))
		return core.MixColor(base, mapTileFogColor, float64(1-mapLevelFadeFor(surf.Depth)))
	case core.MapSurfaceFloor:
		return minimapTileColor(indoor, m.TileAt(x, z))
	default: // MapSurfaceVoid
		return mapTileFogColor
	}
}

// minimapLevelOutlineColor / Underlay are the SEEN-WALL border: a muted brass
// line over a faint dark halo. Deliberately dim (low alpha) so it reads as a
// quiet pen line tracing the walls you've actually seen, not a bright highlight.
// Shared by the corner minimap and the panels Map tab via drawMapLevelOutline.
var (
	minimapLevelOutlineColor    = rl.NewColor(176, 146, 86, 150)
	minimapLevelOutlineUnderlay = rl.NewColor(12, 12, 18, 130)
)

// minimapOutlineCoreThick / UnderThick are the seen-boundary border stroke
// widths (px): a brass core over a slightly wider dark halo. Tune both to make
// the wall/cliff outline heavier or lighter.
const (
	minimapOutlineCoreThick  = float32(2.5)
	minimapOutlineUnderThick = float32(4)
)

// mapSliceCell classifies and colors one cell for the level-sliced in-game maps
// (corner minimap + panels Map tab). Returns the fog-of-war fill with the
// next-elevation wall suppressed to fog (the outline marks seen walls instead of
// filling them as whole tiles), plus whether the cell is on the current-level
// walkable slice (floor/ramp) and whether it is a SEEN BOUNDARY — a wall (a
// next-elevation cube) OR a cliff edge (a drop to a revealed lower level). Both
// bound the current level, so both earn an outline. Unrevealed / out-of-bounds
// cells are fog with both flags false. Exactly one MapSurfaceAt per call —
// callers cache the flags in a window+border grid.
// rampDir reports a ramp cell's travel direction relative to the player's level:
// +1 = stairs UP, -1 = stairs DOWN, 0 = not a ramp. drawMapStairIcons keys the
// up/down stair glyph off it.
func mapSliceCell(m *core.AreaDefinition, g *core.GameState, indoor bool, x, z int) (col rl.Color, onSlice, seenWall bool, rampDir int8) {
	col = mapTileFogColor
	if !m.InBounds(x, z) || !visitedAt(g, x, z) {
		return col, false, false, 0
	}
	surf := m.MapSurfaceAt(x, z, g.Player.Level)
	switch surf.Kind {
	case core.MapSurfaceFloor, core.MapSurfaceRamp:
		onSlice = true
		if surf.Kind == core.MapSurfaceRamp {
			// A ramp stores its LOW level; MapSurfaceAt only returns Ramp when that
			// low is the player's level (L → ascends to L+1, stairs UP) or one
			// below it (L-1 → its high end is L, stairs DOWN from here).
			if m.ElevationLevelAt(x, z) >= g.Player.Level {
				rampDir = 1
			} else {
				rampDir = -1
			}
		}
	case core.MapSurfaceWall, core.MapSurfaceBelow:
		// Wall = a next-elevation cube (a cliff/wall face rising into eyeline);
		// Below = a drop to a lower level (a cliff EDGE you'd fall off). Both
		// bound the floor you're on, so both get the border. (Below still fills
		// its faded lower-level swatch — only Wall is suppressed to fog.)
		seenWall = true
	}
	if surf.Kind != core.MapSurfaceWall {
		col = mapSurfaceColor(m, indoor, x, z, surf)
	}
	return col, onSlice, seenWall, rampDir
}

// drawMapLevelOutline strokes a border ONLY where an explored current-level cell
// (floor/ramp) abuts a SEEN BOUNDARY — a revealed wall (next-elevation cube) or a
// cliff edge (a drop to a revealed lower level). It deliberately does NOT outline
// the fog frontier (an explored floor next to unexplored space): that terrain
// might still be walkable, so a border there would imply a boundary that hasn't
// been seen. slice/seenWall are window+border grids (gw == cols+2, indexed with a
// +1 border offset), so neighbour tests one step outside the window are O(1)
// reads. Geometry is float so the integer-celled corner minimap and the
// fractional-celled Map tab share it. The grids must be sized gw*(rows+2).
func drawMapLevelOutline(slice, seenWall []bool, gw, cols, rows int, originX, originY, cellW, cellH float32) {
	in := func(lx, lz int) bool { return lx >= -1 && lz >= -1 && lx <= cols && lz <= rows }
	isSlice := func(lx, lz int) bool { return in(lx, lz) && slice[(lz+1)*gw+(lx+1)] }
	isWall := func(lx, lz int) bool { return in(lx, lz) && seenWall[(lz+1)*gw+(lx+1)] }
	// Double-stroke (faint dark halo under a muted brass core) so the line stays
	// legible over both the dark walkable floor and the fog.
	edge := func(ax, ay, bx, by float32) {
		rl.DrawLineEx(rl.NewVector2(ax, ay), rl.NewVector2(bx, by), minimapOutlineUnderThick, minimapLevelOutlineUnderlay)
		rl.DrawLineEx(rl.NewVector2(ax, ay), rl.NewVector2(bx, by), minimapOutlineCoreThick, minimapLevelOutlineColor)
	}
	for lz := 0; lz < rows; lz++ {
		for lx := 0; lx < cols; lx++ {
			if !isSlice(lx, lz) {
				continue
			}
			px := originX + float32(lx)*cellW
			py := originY + float32(lz)*cellH
			if isWall(lx-1, lz) { // left edge abuts a seen wall
				edge(px, py, px, py+cellH)
			}
			if isWall(lx+1, lz) { // right
				edge(px+cellW, py, px+cellW, py+cellH)
			}
			if isWall(lx, lz-1) { // top
				edge(px, py, px+cellW, py)
			}
			if isWall(lx, lz+1) { // bottom
				edge(px, py+cellH, px+cellW, py+cellH)
			}
		}
	}
}

// stairIconColor is the dark ink the stair silhouette is drawn in — dark so the
// steps read crisply over the brass ramp fill on both maps.
var stairIconColor = rl.NewColor(34, 24, 12, 240)

// drawMapStairIcons stamps a staircase glyph on every ramp cell in the window:
// rampDir > 0 → stairs UP, < 0 → stairs DOWN, 0 → no ramp. rampDir is the same
// window+border grid the fill pass built (gw == cols+2, +1 border offset); only
// the in-window cells are iterated. Geometry matches drawMapLevelOutline so the
// corner minimap and the Map tab share it.
func drawMapStairIcons(rampDir []int8, gw, cols, rows int, originX, originY, cellW, cellH float32) {
	r := min(cellW, cellH) * 0.34
	if r < 2 {
		return // cells too small to read a stair glyph — skip rather than draw mush
	}
	for lz := 0; lz < rows; lz++ {
		for lx := 0; lx < cols; lx++ {
			d := rampDir[(lz+1)*gw+(lx+1)]
			if d == 0 {
				continue
			}
			cx := originX + (float32(lx)+0.5)*cellW
			cy := originY + (float32(lz)+0.5)*cellH
			drawStairIcon(cx, cy, r, d > 0)
		}
	}
}

// drawStairIcon draws a staircase silhouette centered at (cx,cy) with half-extent
// r: three steps whose tops ASCEND left→right for up-stairs and DESCEND for
// down-stairs, so a ramp reads as "stairs up" vs "stairs down". The stepped top
// edge is the "stairs" cue; the slope direction is the up/down cue.
func drawStairIcon(cx, cy, r float32, up bool) {
	const steps = 3
	w, h := r*2, r*2
	left, bottom := cx-r, cy+r
	stepW := w / steps
	unit := h / steps
	for i := 0; i < steps; i++ {
		n := i + 1 // ascending step count from the left
		if !up {
			n = steps - i // descending for down-stairs
		}
		bh := unit * float32(n)
		rl.DrawRectangleRec(rl.NewRectangle(left+float32(i)*stepW, bottom-bh, stepW, bh), stairIconColor)
	}
}

// drawMinimapTimeOfDay paints the day/night cycle indicator under the
// minimap grid: phase name on the left, raw step counter on the right,
// and a thin progress bar showing how far through the current phase the
// player is. The cycle is 150 steps total (6 × 25), so the bar wraps
// every full day.
//
// minimapMeasureCache memoizes rl.MeasureTextEx for the two strings the
// always-visible minimap shapes every frame — the centered area name and the
// right-aligned "step N / M" counter. Both change only on area transition /
// landed step, not at 60 Hz, so the raw per-frame CGO measure was pure waste.
// The shared cache keys on (text, size), so the two sizes coexist.
var minimapMeasureCache measureCache

func drawMinimapTimeOfDay(font rl.Font, stepCount int, x, y, width int32) {
	phase, progress := core.PhaseAtStep(stepCount)
	name := core.PhaseName(phase)
	// Phase name only — the raw "step N / 150" counter was removed (it read as
	// debug chrome); the bar + cursor below carry the through-the-day progress.
	drawTextWithShadow(font, name, float32(x), float32(y), FontSmall, textPrimary)
	// Line 2: thin track, with the phase highlighted as a 1/N segment that
	// fills as the player walks through it.
	trackY := y + 30
	trackH := int32(4)
	trackW := width
	rl.DrawRectangle(x-2, trackY-2, trackW+4, trackH+4, fadeColor(woodDark, 0.72))
	rl.DrawRectangle(x, trackY, trackW, trackH, barTrack)
	// Segment edges via truncated-difference (x + trackW*i/count) so the
	// rounding remainder is spread across segments instead of pooling at the
	// right end — the final phase now reaches the right edge exactly, with no
	// unfilled sliver.
	count := int32(core.TimeOfDayCount)
	segEdge := func(i int32) int32 { return x + trackW*i/count }
	ph := int32(phase)
	// Past phases: warm wood fill. Current phase: bright gilt by progress. The
	// fills are wood/brass tones (woodenPhaseColor mutes each phase's sky tint
	// heavily toward woodAccent) so the strip reads as brass-on-wood cabinetry
	// in line with the rest of the HUD, not a saturated rainbow.
	for i := int32(0); i < ph; i++ {
		e0, e1 := segEdge(i), segEdge(i+1)
		rl.DrawRectangle(e0, trackY, e1-e0-1, trackH, woodenPhaseColor(phaseColors[i]))
	}
	segL, segR := segEdge(ph), segEdge(ph+1)
	curW := int32(float32(segR-segL) * progress)
	if curW > 0 {
		rl.DrawRectangle(segL, trackY, curW, trackH, woodenPhaseColor(phaseColors[phase]))
	}
	// Tiny gilt cursor riding the current phase — a 3-px diamond
	// pip parked above the bar at the exact progress position, like
	// a sextant's needle on a brass scale. Reads as "the time of day
	// is here" at a glance, more legible than the colour wash alone.
	cursorX := float32(segL) + float32(segR-segL)*progress
	cursorY := float32(trackY) + float32(trackH)/2 - 5
	for i := int32(0); i <= count; i++ {
		rl.DrawRectangle(segEdge(i), trackY-3, 1, trackH+6, fadeColor(giltDim, 0.42))
	}
	drawDiamondPip(cursorX, cursorY, 3, giltBright)
	drawDiamondPip(cursorX, float32(trackY+trackH)+5, 2, fadeColor(giltDim, 0.70))
}

// phaseColors mirrors the rough sky tint of each lighting phase so the HUD
// strip itself reads as a tiny day at a glance. Indexed by TimeOfDay; the
// [core.TimeOfDayCount] array size is itself the compile-time length guard.
// woodenPhaseColor blends a phase's bright sky tint heavily toward the HUD's
// wood accent so the day/night strip reads as warm brass-on-wood cabinetry
// rather than a saturated rainbow, while keeping a faint per-phase hue cue.
func woodenPhaseColor(c rl.Color) rl.Color {
	const k = 0.64                         // pull toward wood
	out := core.MixColor(c, woodAccent, k) // per-channel lerp lives in core, not a local re-roll
	out.A = 255                            // strip always paints fully opaque regardless of input alphas
	return out
}

var phaseColors = [core.TimeOfDayCount]rl.Color{
	core.Dawn:      rl.NewColor(232, 168, 152, 255), // dawn — rose
	core.Morning:   rl.NewColor(220, 224, 200, 255), // morning — pale gold
	core.Afternoon: rl.NewColor(190, 220, 244, 255), // afternoon — sky
	core.Dusk:      rl.NewColor(232, 152, 96, 255),  // dusk — orange
	core.Evening:   rl.NewColor(96, 110, 180, 255),  // evening — indigo
	core.Midnight:  rl.NewColor(40, 56, 110, 255),   // midnight — deep blue
}

// playerArrowColor is the shared green tint used wherever the player's
// facing is drawn on a 2D map surface (corner minimap + panels Map
// tab). Aliases the theme marker palette's markerPlayer so it sits with
// the sibling chest/door/pack markers instead of as a lone literal here.
var playerArrowColor = markerPlayer

// 2D map surface marker palette — shared by the corner minimap and
// the panels Map tab so a future tuning pass adjusts both surfaces in
// one edit. Pack markers are red (danger), chest markers are gold
// (loot), door markers are warm wood (passage). The Looted chest
// variant dims the gold so a played-through dungeon reads at a
// glance.
// mapChestMarkerColor / mapDoorMarkerColor / mapChestLootedColor now
// alias the theme's marker palette so the minimap and the editor canvas
// can never drift on entity tone. (Pack markers are intentionally
// omitted from both the minimap and the panels Map tab, so no pack-
// marker aliases live here.) Unvisited tiles fall back to the same
// flat fog as out-of-bounds — the fog/mix variables were retired when
// the partial-haze look proved to leak too much layout through to the
// player.
var (
	mapChestMarkerColor = markerChest
	mapChestLootedColor = markerChestDim
	mapDoorMarkerColor  = markerDoor
)

// drawFacingArrow paints a triangle at `center` pointing in `facing`.
// `forward` is the half-depth (tip distance from center along facing)
// AND the half-depth behind center for the base; `sideways` is the
// base's half-width perpendicular to facing. Equilateral triangle when
// forward == sideways; longer / thinner spear when sideways < forward.
// Shared by the minimap's player arrow and the panels Map-tab arrow so
// a future palette / shape tweak applies to both.
func drawFacingArrow(center rl.Vector2, forward, sideways float32, facing int, col rl.Color) {
	var tip, left, right rl.Vector2
	switch core.NormalizeFacing(facing) {
	case core.North:
		tip = rl.NewVector2(center.X, center.Y-forward)
		left = rl.NewVector2(center.X-sideways, center.Y+forward)
		right = rl.NewVector2(center.X+sideways, center.Y+forward)
	case core.East:
		tip = rl.NewVector2(center.X+forward, center.Y)
		left = rl.NewVector2(center.X-forward, center.Y-sideways)
		right = rl.NewVector2(center.X-forward, center.Y+sideways)
	case core.South:
		tip = rl.NewVector2(center.X, center.Y+forward)
		left = rl.NewVector2(center.X+sideways, center.Y-forward)
		right = rl.NewVector2(center.X-sideways, center.Y-forward)
	case core.West:
		tip = rl.NewVector2(center.X-forward, center.Y)
		left = rl.NewVector2(center.X+forward, center.Y+sideways)
		right = rl.NewVector2(center.X+forward, center.Y-sideways)
	}
	drawTriangleCCW(tip, left, right, col)
}

func drawMinimapArrow(center rl.Vector2, facing int) {
	// Equilateral so the corner minimap reads as a chunky compass.
	drawFacingArrow(center, 7, 7, facing, playerArrowColor)
}

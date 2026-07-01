package render

import (
	"image/color"
	"math"

	"crawler/internal/app/core"
	rl "github.com/gen2brain/raylib-go/raylib"
)

// Minimap geometry, exposed via MinimapWidth/MinimapBottomY so sibling HUD panels dock to it.
// Header is the area-name strip; footer is the time-of-day phase bar.
const (
	minimapCell      = int32(12)
	minimapViewCells = int32(17) // odd so the player sits dead-center; wider window shows more surrounding area
	minimapHeader    = int32(26)
	minimapFooter    = int32(52) // time-of-day strip beneath the grid (label line + gap + bar + cursor pips)
	// minimapGridInset is the gutter between the card edge and the grid; the panel reserves it on
	// both sides. minimapFooterGap is the breath above the time-of-day strip; minimapFooterInsetX
	// is the strip's own side inset (its width is panelW - 2*minimapFooterInsetX).
	minimapGridInset    = int32(8)
	minimapFooterGap    = int32(12)
	minimapFooterInsetX = int32(14)
	minimapPanelW       = minimapViewCells*minimapCell + 2*minimapGridInset
	minimapPanelH       = minimapViewCells*minimapCell + 2*minimapGridInset + minimapHeader + minimapFooter
)

// minimapSliceBuf / minimapSeenBuf / minimapRampBuf / minimapColBuf are reused per-cell
// classifier grids for drawMinimap (single-threaded). Their contents are cached across
// frames keyed on minimapClassCache — see the classify loop in drawMinimap.
var (
	minimapSliceBuf []bool
	minimapSeenBuf  []bool
	minimapRampBuf  []int8
	minimapColBuf   []rl.Color
)

// minimapClassKey fingerprints the inputs the minimap cell classification depends on:
// area (Path), player tile, and level. NOT facing (only rotates the arrow) or time
// (footer only), and fog reveals happen on the same step that moves the player — so a
// stationary player reuses the cached grids instead of reclassifying ~vc² cells/frame.
type minimapClassKey struct {
	path                string
	tileX, tileZ, level int
	revealGen           int // fog reveal counter; busts the cache on a non-moving reveal
	valid               bool
}

var minimapClassCache minimapClassKey

// borderIdx flattens a window-cell coord into the +1-border scratch grids (gw == cols+2),
// so the "+1 each axis" convention lives in one place across the classify/draw/outline/stair sites.
func borderIdx(gw, lx, lz int) int { return (lz+1)*gw + (lx + 1) }

// MinimapWidth is the corner minimap card's on-screen width (panels beneath it match it).
func MinimapWidth() int32 { return minimapPanelW }

// MinimapBottomY is the minimap card's bottom-edge Y (the turn-order panel anchors here).
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

	// drawCard with noAccent (not drawPanelCard): the minimap skips the left accent
	// rail so there's no bright vertical edge-line down the panel's left side.
	drawCard(pad, pad, panelW, panelH, surfacePrimary, borderSoft, noAccent)
	areaName := g.Area.Name
	if areaName != "" {
		// Area name centered above the grid as the panel's natural label.
		nameMeasure := minimapMeasureCache.measure(assets.hudFont, areaName, FontSmall, 1)
		nameX := float32(pad) + (float32(panelW)-nameMeasure.X)/2
		drawTextWithShadow(assets.hudFont, areaName, nameX, float32(pad+6), FontSmall, textMuted)
	}

	gridX := pad + minimapGridInset
	gridY := pad + minimapGridInset + header
	footerY := gridY + gridSize + minimapFooterGap
	drawMinimapGridBacking(gridX, gridY, gridSize)
	drawMinimapTimeOfDay(assets.hudFont, g.StepCount, pad+minimapFooterInsetX, footerY, panelW-2*minimapFooterInsetX)

	// One MaterialIsIndoor lookup for the whole grid (per-area constant), passed per cell.
	indoor := core.MaterialIsIndoor(m.Materials)
	// Single mapSliceCell pass over the window + one-cell border into the slice/seenWall grids,
	// so the outline's neighbour tests are O(1) reads (one MapSurfaceAt per cell, not ~6×).
	vc := int(viewCells)
	gw := vc + 2
	// Reused scratch grids (cap-grow like memberColumnBuf); the loop overwrites every index.
	n := gw * gw
	if cap(minimapSliceBuf) < n {
		minimapSliceBuf = make([]bool, n)
		minimapSeenBuf = make([]bool, n)
		minimapRampBuf = make([]int8, n)
		minimapColBuf = make([]rl.Color, n)
		minimapClassCache.valid = false // fresh grids — force a reclassify
	}
	slice := minimapSliceBuf[:n]
	seen := minimapSeenBuf[:n]
	ramp := minimapRampBuf[:n]
	col := minimapColBuf[:n]
	// Reclassify only when the fingerprint changes (player moved / changed level /
	// area changed); otherwise the cached grids from last frame still hold. The draw
	// calls below always run — only the ~vc² MapSurfaceAt classifications are skipped.
	key := minimapClassKey{path: m.Path, tileX: p.TileX, tileZ: p.TileZ, level: p.Level, revealGen: g.RevealGen, valid: true}
	if key != minimapClassCache {
		for localZ := -1; localZ <= vc; localZ++ {
			for localX := -1; localX <= vc; localX++ {
				c, onSlice, seenWall, rampDir := mapSliceCell(m, g, indoor, startX+localX, startZ+localZ)
				i := borderIdx(gw, localX, localZ)
				slice[i], seen[i], ramp[i], col[i] = onSlice, seenWall, rampDir, c
			}
		}
		minimapClassCache = key
	}
	for localZ := 0; localZ < vc; localZ++ {
		for localX := 0; localX < vc; localX++ {
			i := borderIdx(gw, localX, localZ)
			rl.DrawRectangle(gridX+int32(localX)*cell, gridY+int32(localZ)*cell, cell-1, cell-1, col[i])
		}
	}
	// Border only where explored current-level floor abuts a seen wall, over the fills.
	drawMapLevelOutline(slice, seen, gw, vc, vc, float32(gridX), float32(gridY), float32(cell), float32(cell))
	// Up/down stair glyphs on ramp cells.
	drawMapStairIcons(ramp, gw, vc, vc, float32(gridX), float32(gridY), float32(cell), float32(cell))

	// Pack markers omitted (terrain only; enemies stay off the map, per the Map-panel rule).

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

	// Subtle crosshair + rule marks (cartographer's plate, without covering the cells).
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

	// Compass initials sit just inside the grid edges, each with a strong dark halo
	// (drawCompassLabel) for legibility over both dark floor and light blocker cells.
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

// drawCompassLabel paints a compass initial with a 6-way black halo (heavier than
// drawTextWithShadow's single drop, which washed out over light cells).
func drawCompassLabel(font rl.Font, s string, x, y float32, col rl.Color) {
	halo := fadeColor(rl.Black, 0.85)
	for _, d := range compassHaloOffsets {
		rl.DrawTextEx(font, s, rl.NewVector2(x+d[0], y+d[1]), FontTiny, 1, halo)
	}
	rl.DrawTextEx(font, s, rl.NewVector2(x, y), FontTiny, 1, col)
}

var compassHaloOffsets = [6][2]float32{{-1, 0}, {1, 0}, {0, -1}, {0, 1}, {1, 1}, {-1, -1}}

// init asserts minimapPropColors covers exactly core.PropTileChars (both directions), and that
// every core.BlockingFloorChars tile has a dedicated minimapTileColor case (no fall-through to fog).
func init() {
	for _, c := range core.PropTileChars() {
		if _, ok := minimapPropColors[c]; !ok {
			panic("render/minimap: missing color for prop tile " + string(c))
		}
	}
	// Reverse coverage: a color for a non-prop char means the two lists drifted.
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

// Shared alphas: prop swatches vs the slightly-less-opaque structural wall/water/floor tones.
const (
	minimapPropAlpha   uint8 = 240
	minimapStructAlpha uint8 = 235
)

// minimapProp builds a prop swatch at the shared minimapPropAlpha (entries carry only their hue).
func minimapProp(r, g, b uint8) rl.Color { return rl.NewColor(r, g, b, minimapPropAlpha) }

// minimapPropColors keys each prop char to its swatch (map-driven so a missing entry is caught
// at init); tones are tuned for the small-rendered case, distinct from the editor's palette.
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

// minimapPropColorTable / minimapPropColorPresent mirror minimapPropColors into a [256] array
// (built once at init) so the per-cell lookup skips the map hash. minimapPropColors stays authoritative.
var minimapPropColorTable, minimapPropColorPresent = buildMinimapPropColorTable()

func buildMinimapPropColorTable() ([256]rl.Color, [256]bool) {
	var t [256]rl.Color
	var p [256]bool
	for c, col := range minimapPropColors {
		// Normalize toward a common light blocker tone so the map reads dark=walkable/light=blocked.
		t[c], p[c] = normalizeMapProp(col), true
	}
	return t, p
}

// mapPropBase is the common light tone props are pulled toward; mapPropTint is how much
// authored hue survives (enough to tell foliage from masonry, not a rainbow).
var mapPropBase = rl.NewColor(150, 146, 136, minimapPropAlpha)

const mapPropTint = 0.30 // fraction of the authored hue retained over the base

// normalizeMapProp blends an authored swatch toward mapPropBase, keeping mapPropTint of its hue.
func normalizeMapProp(c rl.Color) rl.Color {
	out := core.MixColor(mapPropBase, c, mapPropTint)
	out.A = minimapPropAlpha
	return out
}

// Minimap structural tones (walls/water/floor). Legibility convention: walkable = DARK,
// blocking = LIGHT, all kept above the near-black fog so explored never reads as unrevealed.
var (
	minimapWallIndoor   = rl.NewColor(178, 176, 168, minimapStructAlpha)
	minimapWallOutdoor  = rl.NewColor(160, 158, 150, minimapStructAlpha)
	minimapDeepWater    = rl.NewColor(122, 172, 216, minimapStructAlpha)
	minimapFloorIndoor  = rl.NewColor(44, 46, 52, minimapStructAlpha)
	minimapFloorOutdoor = rl.NewColor(38, 56, 40, minimapStructAlpha)
)

// mapNonBlockingPropTint is how much of a walk-through prop's hue survives over the floor
// (small, so torches/flowers/ferns read almost as floor — only blockers get a full swatch).
const mapNonBlockingPropTint = 0.22

// minimapTileColor maps a tile char to its swatch. indoor is hoisted to the caller so the
// per-cell loop computes it once per draw.
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
				// Walk-through props read almost as floor (faint hue hint).
				out := core.MixColor(floor, minimapPropColorTable[tile], mapNonBlockingPropTint)
				out.A = minimapStructAlpha
				return out
			}
			// Blocking props read lighter than the floor — "stuff in the way."
			return minimapPropColorTable[tile]
		}
		return floor
	}
}

// mapRampColor is the warm brass for a ramp to/from the current level (the way up/down).
var mapRampColor = rl.NewColor(198, 168, 104, minimapStructAlpha)

// Level-slice fade: floors below the observer recede geometrically toward the fog (clamped to a
// faint floor). Precomputed table since the map recolors many cells per frame.
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
	return core.DistanceFade(d, mapLevelFadeTable[:], mapLevelFadeMin)
}

// mapSurfaceColor turns a column classification into its swatch: current level full strength,
// walls/cliffs light, ramps brass, floors-below faded by depth, void = fog (no phantom floor).
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

// minimapLevelOutlineColor / Underlay are the seen-wall border: a dim brass line over a dark halo.
var (
	minimapLevelOutlineColor    = rl.NewColor(176, 146, 86, 150)
	minimapLevelOutlineUnderlay = rl.NewColor(12, 12, 18, 130)
)

// Seen-boundary border stroke widths (px): brass core over a wider dark halo.
const (
	minimapOutlineCoreThick  = float32(2.5)
	minimapOutlineUnderThick = float32(4)
)

// mapSliceCell classifies/colors one cell for the level-sliced maps. Returns the fog-of-war
// fill (next-elevation walls suppressed to fog — the outline marks them instead), whether the
// cell is on the current walkable slice (floor/ramp), and whether it's a SEEN BOUNDARY (wall or
// cliff edge). Unrevealed/out-of-bounds = fog, both flags false. One MapSurfaceAt per call.
// rampDir: +1 stairs UP, -1 DOWN, 0 not a ramp (keys drawMapStairIcons).
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
			// A ramp stores its LOW level: low==player's level → UP; low one below → DOWN.
			if m.ElevationLevelAt(x, z) >= g.Player.Level {
				rampDir = 1
			} else {
				rampDir = -1
			}
		}
	case core.MapSurfaceWall, core.MapSurfaceBelow:
		// Wall = next-elevation cube; Below = a drop to a lower level. Both bound the floor,
		// so both get the border. (Below still fills its faded swatch; only Wall is suppressed to fog.)
		seenWall = true
	}
	if surf.Kind != core.MapSurfaceWall {
		col = mapSurfaceColor(m, indoor, x, z, surf)
	}
	return col, onSlice, seenWall, rampDir
}

// drawMapLevelOutline strokes a border only where an explored floor/ramp cell abuts a SEEN
// BOUNDARY (wall or cliff edge) — never the fog frontier (that floor might still be walkable).
// slice/seenWall are window+border grids (gw == cols+2, +1 offset) so neighbour tests are O(1).
// Float geometry, so the integer-cell minimap and fractional-cell Map tab share it.
func drawMapLevelOutline(slice, seenWall []bool, gw, cols, rows int, originX, originY, cellW, cellH float32) {
	in := func(lx, lz int) bool { return lx >= -1 && lz >= -1 && lx <= cols && lz <= rows }
	isSlice := func(lx, lz int) bool { return in(lx, lz) && slice[borderIdx(gw, lx, lz)] }
	isWall := func(lx, lz int) bool { return in(lx, lz) && seenWall[borderIdx(gw, lx, lz)] }
	// Double-stroke (dark halo under a brass core) so it stays legible over floor and fog.
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

// stairIconColor is the dark ink for the stair silhouette (crisp over the brass ramp fill).
var stairIconColor = rl.NewColor(34, 24, 12, 240)

// drawMapStairIcons stamps a staircase glyph on each ramp cell (rampDir >0 UP, <0 DOWN, 0 none).
// Shares the window+border grid + geometry with drawMapLevelOutline.
func drawMapStairIcons(rampDir []int8, gw, cols, rows int, originX, originY, cellW, cellH float32) {
	r := min(cellW, cellH) * 0.34
	if r < 2 {
		return // cells too small to read a stair glyph
	}
	for lz := 0; lz < rows; lz++ {
		for lx := 0; lx < cols; lx++ {
			d := rampDir[borderIdx(gw, lx, lz)]
			if d == 0 {
				continue
			}
			cx := originX + (float32(lx)+0.5)*cellW
			cy := originY + (float32(lz)+0.5)*cellH
			drawStairIcon(cx, cy, r, d > 0)
		}
	}
}

// drawStairIcon draws a 3-step staircase at (cx,cy), half-extent r: tops ascend left→right for
// up-stairs, descend for down. Stepped edge = "stairs"; slope direction = up/down.
func drawStairIcon(cx, cy, r float32, up bool) {
	const steps = 3
	w, h := r*2, r*2
	left, bottom := cx-r, cy+r
	stepW := w / steps
	unit := h / steps
	for i := 0; i < steps; i++ {
		n := i + 1 // ascending from the left
		if !up {
			n = steps - i // descending
		}
		bh := unit * float32(n)
		rl.DrawRectangleRec(rl.NewRectangle(left+float32(i)*stepW, bottom-bh, stepW, bh), stairIconColor)
	}
}

// minimapMeasureCache memoizes MeasureTextEx for the always-visible minimap strings
// (area name + counter), which change only on area/step, not 60 Hz.
var minimapMeasureCache measureCache

// drawMinimapTimeOfDay paints the day/night indicator under the grid: phase name + a progress
// bar with a 1/N segment that fills through the current phase.
func drawMinimapTimeOfDay(font rl.Font, stepCount int, x, y, width int32) {
	phase, progress := core.PhaseAtStep(stepCount)
	name := core.PhaseName(phase)
	drawTextWithShadow(font, name, float32(x), float32(y), FontSmall, textPrimary)
	// Sharp-cornered phase strip (NOT the rounded drawSmallPanel/drawGaugeWell gauge —
	// this is a multi-segment brass-on-wood track with its own tints, ticks, and needle).
	const (
		phaseTrackDY = int32(30) // strip top below the phase name's text top
		phaseTrackH  = int32(4)  // strip height
	)
	trackY := y + phaseTrackDY
	trackH := phaseTrackH
	trackW := width
	rl.DrawRectangle(x-2, trackY-2, trackW+4, trackH+4, fadeColor(woodDark, 0.72))
	rl.DrawRectangle(x, trackY, trackW, trackH, barTrack)
	// Segment edges via truncated-difference so the remainder spreads across segments
	// (the final phase reaches the right edge exactly, no unfilled sliver).
	count := int32(core.TimeOfDayCount)
	segEdge := func(i int32) int32 { return x + trackW*i/count }
	ph := int32(phase)
	// Past phases warm wood fill, current phase gilt by progress (woodenPhaseColor mutes the
	// sky tint toward wood so the strip reads as brass-on-wood, not a rainbow).
	for i := int32(0); i < ph; i++ {
		e0, e1 := segEdge(i), segEdge(i+1)
		// -1 trims a hairline seam; guard >0 so a <=1px segment can't pass raylib a
		// negative width (renders as a wrong/oversized fill) — matches the curW guard below.
		if w := e1 - e0 - 1; w > 0 {
			rl.DrawRectangle(e0, trackY, w, trackH, woodenPhaseColor(phaseColors[i]))
		}
	}
	segL, segR := segEdge(ph), segEdge(ph+1)
	curW := int32(float32(segR-segL) * progress)
	if curW > 0 {
		rl.DrawRectangle(segL, trackY, curW, trackH, woodenPhaseColor(phaseColors[phase]))
	}
	// Gilt cursor pip riding the current phase at the progress position (a sextant needle).
	cursorX := float32(segL) + float32(segR-segL)*progress
	cursorY := float32(trackY) + float32(trackH)/2 - 5
	for i := int32(0); i <= count; i++ {
		rl.DrawRectangle(segEdge(i), trackY-3, 1, trackH+6, fadeColor(giltDim, 0.42))
	}
	drawDiamondPip(cursorX, cursorY, 3, giltBright)
	drawDiamondPip(cursorX, float32(trackY+trackH)+5, 2, fadeColor(giltDim, 0.70))
}

// woodenPhaseColor blends a phase's sky tint heavily toward wood accent (brass-on-wood, not a rainbow).
func woodenPhaseColor(c rl.Color) rl.Color {
	const k = 0.64 // pull toward wood
	out := core.MixColor(c, woodAccent, k)
	out.A = 255 // strip always fully opaque
	return out
}

// phaseColors is the sextant strip's per-phase accent, indexed by TimeOfDay.
// Hand-tuned for legibility on the wood strip (brighter/more saturated than the
// raw timeProfiles SkyTint, then muted 64% toward wood by woodenPhaseColor) — it
// PARALLELS the lighting phases by feel, it is NOT derived from SkyTint, so a sky
// retint does not require (or auto-propagate to) a change here.
var phaseColors = [core.TimeOfDayCount]rl.Color{
	core.Dawn:      rl.NewColor(232, 168, 152, 255), // dawn — rose
	core.Morning:   rl.NewColor(220, 224, 200, 255), // morning — pale gold
	core.Afternoon: rl.NewColor(190, 220, 244, 255), // afternoon — sky
	core.Dusk:      rl.NewColor(232, 152, 96, 255),  // dusk — orange
	core.Evening:   rl.NewColor(96, 110, 180, 255),  // evening — indigo
	core.Midnight:  rl.NewColor(40, 56, 110, 255),   // midnight — deep blue
}

// playerArrowColor is the player-facing tint on 2D map surfaces; aliases markerPlayer.
var playerArrowColor = markerPlayer

// 2D map marker palette, aliasing the theme markers so minimap + editor can't drift.
// (Pack markers are intentionally omitted from both map surfaces.)
var (
	mapChestMarkerColor = markerChest
	mapChestLootedColor = markerChestDim
	mapDoorMarkerColor  = markerDoor
)

// drawFacingArrow paints a triangle at center pointing in facing: forward = half-depth
// (tip + base), sideways = base half-width. Equilateral when equal, spear when sideways < forward.
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
	// Equilateral — a chunky compass.
	drawFacingArrow(center, 7, 7, facing, playerArrowColor)
}

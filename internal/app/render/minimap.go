package render

import (
	"image/color"
	"strconv"

	"crawler/internal/app/core"
	rl "github.com/gen2brain/raylib-go/raylib"
)

// stepCounterCache memoizes the formatted "step N / Total" string so
// the HUD doesn't rebuild it every frame when nothing's changed. The
// counter only advances when the player takes a tile-step (rare —
// once every few hundred frames), but the minimap painted it through
// fmt.Sprintf 60 Hz. Stash the last step and result.
var stepCounterCache struct {
	step    int
	text    string
	primed  bool
	maxStep int
}

func stepCounterText(modStep int) string {
	if stepCounterCache.primed &&
		stepCounterCache.step == modStep &&
		stepCounterCache.maxStep == core.StepsPerCycle {
		return stepCounterCache.text
	}
	stepCounterCache.text = "step " + strconv.Itoa(modStep) + " / " + strconv.Itoa(core.StepsPerCycle)
	stepCounterCache.step = modStep
	stepCounterCache.maxStep = core.StepsPerCycle
	stepCounterCache.primed = true
	return stepCounterCache.text
}

// minimapOutOfBoundsColor is the dim background tone drawn for the
// 13x13 minimap window's cells that fall outside the area's bounds.
// Hoisted from inside the per-tile loop so the value is named once
// and the per-tile path is a plain copy.
var minimapOutOfBoundsColor = rl.NewColor(8, 10, 14, 235)

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
	minimapViewCells = int32(13)
	minimapHeader    = int32(26)
	minimapFooter    = int32(28) // time-of-day strip beneath the grid
	minimapPanelW    = minimapViewCells*minimapCell + 16
	minimapPanelH    = minimapViewCells*minimapCell + 16 + minimapHeader + minimapFooter
)

// MinimapWidth is the on-screen width of the corner minimap card.
// Used by the turn-order / combat-log panels (which sit beneath the
// minimap on the same left edge) so they can match its width
// without baking in a literal.
func MinimapWidth() int32 { return minimapPanelW }

// MinimapBottomY returns the Y screen coordinate of the bottom edge
// of the minimap card. The turn-order panel anchors here plus a
// hudColumnGap so the two read as a single stacked column.
func MinimapBottomY() int32 { return hudEdgePad + minimapPanelH }

func drawMinimap(m core.AreaDefinition, g core.GameState, assets Resources) {
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

	drawCard(pad, pad, panelW, panelH, surfacePrimary, borderSoft, borderSoft)
	areaName := g.Area.Name
	if areaName != "" {
		// Area name reads as the panel's natural label — no "AREA"
		// tick. Centered above the grid so it doesn't crowd the
		// upper-left wood mitre.
		nameMeasure := rl.MeasureTextEx(assets.hudFont, areaName, FontSmall, 1)
		nameX := float32(pad) + (float32(panelW)-nameMeasure.X)/2
		drawTextWithShadow(assets.hudFont, areaName, nameX, float32(pad+6), FontSmall, textMuted)
	}

	gridX := pad + 8
	gridY := pad + 8 + header
	footerY := gridY + gridSize + 6
	drawMinimapTimeOfDay(assets.hudFont, g.StepCount, pad+14, footerY, panelW-28)

	for localZ := int32(0); localZ < viewCells; localZ++ {
		for localX := int32(0); localX < viewCells; localX++ {
			mapX := startX + int(localX)
			mapZ := startZ + int(localZ)
			// Unrevealed tiles paint the same flat fog as out-of-bounds
			// so the player can't read the layout through the haze —
			// the minimap is strictly "what you've walked on" until a
			// step lands the tile inside the reveal radius.
			col := minimapOutOfBoundsColor
			if m.InBounds(mapX, mapZ) && visitedAt(g, mapX, mapZ) {
				col = minimapTileColor(m.Materials, m.TileAt(mapX, mapZ))
			}
			rl.DrawRectangle(gridX+localX*cell, gridY+localZ*cell, cell-1, cell-1, col)
		}
	}

	// Pack markers intentionally omitted from the corner minimap too:
	// matches the Map panel's fog-of-war rule (terrain only, enemies
	// stay off the map). The world view is the only place to see who's
	// around.

	drawMinimapArrow(
		rl.NewVector2(float32(gridX+gridSize/2), float32(gridY+gridSize/2)),
		p.Facing,
	)
}

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
	for _, c := range core.BlockingFloorChars() {
		col := minimapTileColor(core.MaterialField, c)
		fallback := minimapTileColor(core.MaterialField, core.TileOpen)
		if col == fallback {
			panic("render/minimap: blocking floor tile '" + string(c) + "' falls through to the open-tile color — add an explicit case in minimapTileColor")
		}
	}
}

var minimapPropColors = map[byte]rl.Color{
	core.TileTree:              rl.NewColor(42, 132, 56, 240),
	core.TileTreeXL:            rl.NewColor(28, 102, 44, 240),
	core.TileTreeTall:          rl.NewColor(36, 118, 50, 240),
	core.TileTreeTwin:          rl.NewColor(40, 124, 58, 240),
	core.TileTreeYoung:         rl.NewColor(96, 168, 88, 240),
	core.TileRockLarge:         rl.NewColor(120, 116, 108, 240),
	core.TileBushLarge:         rl.NewColor(110, 168, 92, 240),
	core.TileCrate:             rl.NewColor(168, 122, 72, 240),
	core.TileBarrel:            rl.NewColor(148, 100, 60, 240),
	core.TileUrn:               rl.NewColor(186, 112, 72, 240),
	core.TileStalagmite:        rl.NewColor(196, 188, 174, 240),
	core.TilePillar:            rl.NewColor(214, 206, 188, 240),
	core.TileBrokenPillar:      rl.NewColor(180, 172, 156, 240),
	core.TileStatue:            rl.NewColor(228, 220, 204, 240),
	core.TileObelisk:           rl.NewColor(86, 90, 104, 240),
	core.TileFountain:          rl.NewColor(96, 158, 208, 240),
	core.TileRockCairn:         rl.NewColor(150, 138, 116, 240),
	core.TileRockFormation:     rl.NewColor(118, 102, 86, 240),
	core.TileRockFormationTail: rl.NewColor(118, 102, 86, 240),
	core.TileWell:              rl.NewColor(132, 138, 142, 240),
	core.TileGravestone:        rl.NewColor(168, 162, 152, 240),
	core.TileSignPost:          rl.NewColor(160, 110, 64, 240),
	core.TileHayBale:           rl.NewColor(216, 184, 110, 240),
	core.TileScarecrow:         rl.NewColor(196, 162, 96, 240),
	core.TileBookshelf:         rl.NewColor(132, 90, 56, 240),
	core.TileTable:             rl.NewColor(160, 116, 72, 240),
	core.TileBed:               rl.NewColor(176, 90, 96, 240),
	core.TileBrazier:           rl.NewColor(220, 132, 64, 240),
	core.TileTorch:             rl.NewColor(240, 168, 96, 240),
	core.TileSarcophagus:       rl.NewColor(200, 192, 174, 240),
}

func minimapTileColor(material core.MaterialSet, tile byte) color.RGBA {
	indoor := core.MaterialIsIndoor(material)
	if tile == core.TileRock {
		if indoor {
			return rl.NewColor(132, 132, 126, 235)
		}
		return rl.NewColor(112, 112, 106, 235)
	}
	if tile == core.FloorDeepWater {
		return rl.NewColor(28, 60, 104, 235)
	}
	if col, ok := minimapPropColors[tile]; ok {
		return col
	}
	if indoor {
		return rl.NewColor(82, 84, 88, 235)
	}
	return rl.NewColor(60, 121, 54, 235)
}

// drawMinimapTimeOfDay paints the day/night cycle indicator under the
// minimap grid: phase name on the left, raw step counter on the right,
// and a thin progress bar showing how far through the current phase the
// player is. The cycle is 150 steps total (6 × 25), so the bar wraps
// every full day.
func drawMinimapTimeOfDay(font rl.Font, stepCount int, x, y, width int32) {
	phase, progress := core.PhaseAtStep(stepCount)
	name := core.PhaseName(phase)
	// Line 1: "DAWN  step 12 / 150" (left-aligned phase, right-aligned counter).
	drawTextWithShadow(font, name, float32(x), float32(y), FontSmall, textPrimary)
	counter := stepCounterText(stepCount % core.StepsPerCycle)
	measure := rl.MeasureTextEx(font, counter, FontTiny, 1)
	drawTextWithShadow(font, counter, float32(x)+float32(width)-measure.X, float32(y)+1, FontTiny, textHint)
	// Line 2: thin track, with the phase highlighted as a 1/N segment that
	// fills as the player walks through it.
	trackY := y + 18
	trackH := int32(4)
	trackW := width
	rl.DrawRectangle(x, trackY, trackW, trackH, barTrackColor)
	segW := trackW / int32(core.TimeOfDayCount)
	// Past phases: solid color. Current phase: filled by progress.
	for i := 0; i < int(phase); i++ {
		rl.DrawRectangle(x+int32(i)*segW, trackY, segW-1, trackH, phaseColors[i])
	}
	curW := int32(float32(segW) * progress)
	if curW > 0 {
		rl.DrawRectangle(x+int32(phase)*segW, trackY, curW, trackH, phaseColors[phase])
	}
	// Tiny gilt cursor riding the current phase — a 3-px diamond
	// pip parked above the bar at the exact progress position, like
	// a sextant's needle on a brass scale. Reads as "the time of day
	// is here" at a glance, more legible than the colour wash alone.
	cursorX := float32(x+int32(phase)*segW) + float32(segW)*progress
	cursorY := float32(trackY) + float32(trackH)/2 - 5
	drawDiamondPip(cursorX, cursorY, 3, giltBright)
}

// phaseColors mirrors the rough sky tint of each lighting phase so the HUD
// strip itself reads as a tiny day at a glance. Indexed by TimeOfDay; size
// is asserted at compile time against core.TimeOfDayCount via _phaseColorsLen.
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
// tab). Named so palette changes touch one literal.
var playerArrowColor = rl.NewColor(132, 240, 148, 255)

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

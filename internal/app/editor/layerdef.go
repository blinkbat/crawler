package editor

import (
	"fmt"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// layerdef.go — the single per-layer descriptor table. Each editor Layer is ONE row
// here; the former hand-maintained per-layer switches (applyTool, eraseAt, activeGrid,
// activeLayerCharAt, activeFootprint, eraseSentinel, layerStampsActiveLevel,
// isSentinelBrush, brushPreviewColor, currentLayerGlyph) plus the name/accent tables
// now all read from it, so adding a layer is one entry, not a dozen edits. The init
// below asserts every row is fully populated — a missing field fails loud at startup.

type layerDef struct {
	name         string                                     // selector label (layerName)
	accent       rl.Color                                   // selector chip / dropdown swatch (layerAccent)
	grid         func(s *State) *[]string                   // edited slice; nil = gridless (Entities)
	stampsLevel  bool                                       // a content paint lifts the tile to editLevel
	sentinel     byte                                       // erase "empty" char (grid layers)
	hasSentinel  bool                                       // false for Entities (no per-tile char)
	footprint    func(c byte) []core.MultiTileOffset        // multi-tile brush footprint; nil = single tile
	apply        func(s *State, x, z int, b Brush)          // paint the active brush (owns its dirty/stamp)
	erase        func(s *State, x, z int) bool              // clear the cell; returns whether it changed
	charAt       func(s *State, x, z int) (byte, bool)      // raw active-grid char; ok=false gridless
	isSentinel   func(c byte) bool                          // char is a semantic no-paint brush (Auto/empty)
	previewColor func(s *State, b Brush) rl.Color           // rect-drag preview tint
	glyph        func(s *State, x, z, lvl int) (byte, bool) // overlay char for the cell; ok=false = none
}

// layerDefs is populated in init() (not as a var initializer) to sidestep a false
// initialization cycle: several rows' funcs read layerDefs back at call time
// (apply → paintContentCell → stampActiveLevel → layerStampsActiveLevel; eraseToSentinel),
// which Go's var-init cycle checker rejects even though those reads only run later.
var layerDefs [layerCount]layerDef

func init() {
	layerDefs = [layerCount]layerDef{
		LayerWalls: {
			name: "Faces", accent: rl.NewColor(150, 148, 142, 255),
			grid:     func(s *State) *[]string { return &s.area.Walls },
			sentinel: core.TileOpen, hasSentinel: true,
			// Walls don't stamp a level; stampActiveLevel no-ops, so paintContentCell just
			// sets the face + dirty (matches the old fall-through).
			apply: func(s *State, x, z int, b Brush) {
				paintContentCell(s, x, z, func() bool { applyFaceBrush(s, x, z, b.Char); return true })
			},
			erase:      eraseToSentinel,
			charAt:     func(s *State, x, z int) (byte, bool) { return cellAt(s.area.Walls, x, z) },
			isSentinel: func(c byte) bool { return c == core.TileOpen },
			previewColor: func(s *State, b Brush) rl.Color {
				if b.Char == core.TileRock {
					return wallColor()
				}
				return floorColor(core.FloorAuto)
			},
			glyph: func(s *State, x, z, lvl int) (byte, bool) {
				if w, ok := cellAt(s.area.Walls, x, z); ok && core.IsFaceSkinChar(w) {
					return w, true
				}
				return 0, false
			},
		},
		LayerFloor: {
			name: "Floor", accent: rl.NewColor(120, 184, 110, 255),
			grid:        func(s *State) *[]string { return &s.area.Floor },
			stampsLevel: true,
			sentinel:    core.FloorAuto, hasSentinel: true,
			apply: func(s *State, x, z int, b Brush) {
				paintContentCell(s, x, z, func() bool { setLayerCell(&s.area.Floor, x, z, b.Char); return true })
			},
			erase:        eraseToSentinel,
			charAt:       func(s *State, x, z int) (byte, bool) { return cellAt(s.area.Floor, x, z) },
			isSentinel:   func(c byte) bool { return c == core.FloorAuto },
			previewColor: func(s *State, b Brush) rl.Color { return floorColor(b.Char) },
			glyph: func(s *State, x, z, lvl int) (byte, bool) {
				if f, ok := cellAt(s.area.Floor, x, z); ok && f != core.FloorAuto && f != 0 {
					return f, true
				}
				return 0, false
			},
		},
		LayerDecor: {
			name: "Decor", accent: rl.NewColor(110, 186, 170, 255),
			grid:        func(s *State) *[]string { return &s.area.Decor },
			stampsLevel: true,
			sentinel:    core.DecorEmpty, hasSentinel: true,
			footprint: core.DecorFootprint,
			apply: func(s *State, x, z int, b Brush) {
				paintContentCell(s, x, z, func() bool { return applyDecorBrush(s, x, z, b.Char) })
			},
			// Erase suppresses scatter (DecorEmpty), NOT auto-scatter (DecorAuto).
			erase:      func(s *State, x, z int) bool { setDecorFloor(s, x, z, core.DecorEmpty); return true },
			charAt:     func(s *State, x, z int) (byte, bool) { return s.area.DecorForDisplay(x, z, s.editLevel), true },
			isSentinel: func(c byte) bool { return c == core.DecorAuto || c == core.DecorEmpty },
			previewColor: func(s *State, b Brush) rl.Color {
				if b.Char == core.DecorAuto {
					return b.Color // auto has no single render color; mirror its swatch tint
				}
				return decorColor(b.Char)
			},
			glyph: func(s *State, x, z, lvl int) (byte, bool) {
				if d := s.area.DecorForDisplay(x, z, s.editLevel); d != core.DecorAuto && d != core.DecorEmpty {
					return d, true
				}
				return 0, false
			},
		},
		LayerProps: {
			name: "Props", accent: rl.NewColor(200, 140, 82, 255),
			grid:        func(s *State) *[]string { return &s.area.Props },
			stampsLevel: true,
			sentinel:    core.TilePropEmpty, hasSentinel: true,
			footprint: core.PropFootprint,
			apply: func(s *State, x, z int, b Brush) {
				paintContentCell(s, x, z, func() bool { return applyPropBrush(s, x, z, b.Char) })
			},
			erase:      func(s *State, x, z int) bool { clearPropCell(s, x, z); return true },
			charAt:     func(s *State, x, z int) (byte, bool) { return s.area.PropForDisplay(x, z, s.editLevel), true },
			isSentinel: func(c byte) bool { return c == core.TilePropEmpty },
			previewColor: func(s *State, b Brush) rl.Color {
				if core.IsPropChar(b.Char) {
					return propColor(b.Char)
				}
				return floorColor(core.FloorAuto)
			},
			glyph: func(s *State, x, z, lvl int) (byte, bool) {
				if p := s.area.PropForDisplay(x, z, s.editLevel); core.IsPropChar(p) {
					return p, true
				}
				return 0, false
			},
		},
		LayerCeiling: {
			name: "Ceiling", accent: rl.NewColor(96, 150, 208, 255),
			grid:        func(s *State) *[]string { return &s.area.Ceiling },
			stampsLevel: true,
			sentinel:    core.TileCeilingOpen, hasSentinel: true,
			apply: func(s *State, x, z int, b Brush) {
				paintContentCell(s, x, z, func() bool { setLayerCell(&s.area.Ceiling, x, z, b.Char); return true })
			},
			erase:        eraseToSentinel,
			charAt:       func(s *State, x, z int) (byte, bool) { return cellAt(s.area.Ceiling, x, z) },
			isSentinel:   func(c byte) bool { return c == core.TileCeilingOpen }, // '.' = no slab (sky), a sentinel
			previewColor: func(s *State, b Brush) rl.Color { return ceilingColor() },
			glyph: func(s *State, x, z, lvl int) (byte, bool) {
				if c, ok := cellAt(s.area.Ceiling, x, z); ok && s.area.CeilingAt(x, z) {
					return c, true
				}
				return 0, false
			},
		},
		LayerElevation: {
			name: "Elevation", accent: rl.NewColor(198, 168, 120, 255),
			grid:     func(s *State) *[]string { return &s.area.Elevation },
			sentinel: core.ElevationChar(core.ElevationBaseline), hasSentinel: true,
			apply: func(s *State, x, z int, b Brush) {
				// Voxel paint: place ONE cube at (x, editLevel, z); a gap between stacked
				// tiles makes a walk-under bridge. Elevation doesn't stamp (stampActiveLevel no-ops).
				paintContentCell(s, x, z, func() bool { s.area.SetCube(x, s.editLevel, z, s.area.FaceSkinAt(x, z)); return true })
			},
			erase: func(s *State, x, z int) bool {
				// Remove the tile at (x, editLevel, z) — voxel inverse of a paint. Clear any ramp too.
				s.area.ClearCube(x, s.editLevel, z)
				if _, ok := s.area.RampAt(x, z); ok {
					setLayerCell(&s.area.Floor, x, z, core.FloorAuto)
				}
				return true
			},
			charAt:       func(s *State, x, z int) (byte, bool) { return cellAt(s.area.Elevation, x, z) },
			isSentinel:   func(c byte) bool { return c == core.ElevationGround }, // ground is the flat default
			previewColor: func(s *State, b Brush) rl.Color { return elevationLevelColor(s.editLevel) },
			glyph: func(s *State, x, z, lvl int) (byte, bool) {
				if lvl != core.ElevationBaseline { // off-ground tiles show their level char
					return core.ElevationChar(lvl), true
				}
				return 0, false
			},
		},
		LayerEntities: {
			name: "Entities", accent: rl.NewColor(214, 176, 96, 255),
			// Gridless: no grid/sentinel/footprint/level tags. applyEntityBrush sets its own
			// dirty only when a placement lands; clearEntitiesAt reports whether it removed one.
			apply:        func(s *State, x, z int, b Brush) { applyEntityBrush(s, x, z, b.Entity) },
			erase:        func(s *State, x, z int) bool { return clearEntitiesAt(s, x, z) },
			charAt:       func(s *State, x, z int) (byte, bool) { return 0, false },
			isSentinel:   func(c byte) bool { return false },
			previewColor: func(s *State, b Brush) rl.Color { return editorFallbackColor },
			glyph:        func(s *State, x, z, lvl int) (byte, bool) { return 0, false },
		},
	}
}

// paintContentCell stamps a cell via set (which reports whether a paint actually
// landed) then, ONLY on success, lifts the tile to the active level (a no-op on
// layers whose stampsLevel is false) and marks dirty — the shared tail every
// non-entity paint uses. A refused placement (set returns false: wall / player-start /
// footprint won't fit) must NOT rewrite the column's elevation or dirty the map. The
// dirty is optimistic; strokePaint repairs it if nothing changed.
func paintContentCell(s *State, x, z int, set func() bool) {
	if !set() {
		return
	}
	stampActiveLevel(s, x, z)
	s.dirty = true
}

// eraseToSentinel resets the active grid cell to the layer's sentinel — the shared
// erase for plain-grid layers (walls/floor/ceiling).
func eraseToSentinel(s *State, x, z int) bool {
	setLayerCell(activeGrid(s), x, z, layerDefs[s.layer].sentinel)
	return true
}

func init() {
	for l := 0; l < layerCount; l++ {
		d := &layerDefs[l]
		if d.name == "" || d.apply == nil || d.erase == nil || d.charAt == nil ||
			d.isSentinel == nil || d.previewColor == nil || d.glyph == nil {
			panic(fmt.Sprintf("editor: layerDefs[%d] incomplete — a new layer must fill every behavior field", l))
		}
	}
}

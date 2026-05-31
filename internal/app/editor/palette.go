package editor

import (
	"crawler/internal/app/render"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Editor UI chrome palette. The map-content colors (tile brushes, swatches)
// already live in layerBrushes (editor.go); these are the colors of the
// editor's own buttons, panels, borders, and overlays so they're not
// duplicated as raw rl.NewColor literals across draw.go.
var (
	// Panels and backgrounds — ascend in lightness from window backdrop to
	// hover-highlighted entries. bgFieldInset is the deepest tone, used as
	// the recessed background for editable text fields and the grid pane.
	bgFieldInset  = rl.NewColor(14, 16, 22, 255)
	bgWindow      = rl.NewColor(20, 22, 30, 255)
	bgPaletteCol  = rl.NewColor(24, 28, 38, 255)
	bgPanel       = rl.NewColor(28, 32, 44, 255)
	bgEntry       = rl.NewColor(36, 40, 52, 255)
	bgEntryHover  = rl.NewColor(40, 46, 58, 255)
	bgButton      = rl.NewColor(48, 54, 70, 255)
	bgButtonHover = rl.NewColor(48, 56, 72, 255)
	bgRowHover    = rl.NewColor(60, 70, 90, 255)
	bgActive      = rl.NewColor(72, 88, 130, 255)

	// Borders. dim is the resting state, active is the selection-outline
	// color; outlineHard is the dark seam between panels. Prefixed with
	// "editor" so they don't shadow theme.BorderDim / theme.BorderActive
	// at call sites that consume both. editorBorderInactive sits between
	// outlineHard and editorBorderDim — used for readonly-value frames.
	editorBorderInactive = rl.NewColor(50, 58, 76, 255)
	editorBorderDim      = rl.NewColor(70, 80, 100, 255)
	editorBorderMid      = rl.NewColor(96, 108, 132, 255)
	editorBorderActive   = rl.NewColor(180, 220, 244, 255)
	outlineHard          = rl.NewColor(8, 10, 14, 255)

	// Text colors specific to the editor; the shared HUD theme covers most
	// strings, these handle the brighter entry text and the swatch outline.
	textBright    = rl.NewColor(220, 230, 245, 255)
	textEntry     = rl.NewColor(230, 234, 244, 255)
	textReadonly  = rl.NewColor(200, 210, 230, 255)
	swatchEdge    = rl.NewColor(0, 0, 0, 200)
	gridLineCol   = rl.NewColor(0, 0, 0, 80)
	gridLineMajor = rl.NewColor(0, 0, 0, 160)
	// entityMarkerOutline is the dark ring drawn around pack / chest /
	// start markers on the editor canvas. Aliases render.MarkerOutline
	// so the editor canvas and the minimap can never drift on the
	// silhouette tone — both share one constant via the render theme.
	entityMarkerOutline = render.MarkerOutline

	// Tile-color fallback palette. floorAutoColor is the warm tan used
	// for unrecognized floor and decor tiles (and also the FloorAuto
	// brush swatch in editor.go); editorFallbackColor is the neutral
	// grey for unrecognized props and the "unknown char" last-resort;
	// ceilingFallbackColor is the muted brown that mirrors the indoor
	// ceiling slab tone for any non-palette ceiling char (corrupted
	// save, future char dropped from the brush list).
	floorAutoColor       = rl.NewColor(160, 168, 140, 255)
	editorFallbackColor  = rl.NewColor(200, 200, 200, 255)
	ceilingFallbackColor = rl.NewColor(110, 96, 80, 255)
	// entityFallbackColor is the neutral swatch for an enemy kind with
	// no entityBrushColors entry — shared by the brush builder and the
	// in-grid pack marker so the two can't drift (was an inline
	// rl.NewColor(180,180,180,255) literal at both sites).
	entityFallbackColor = rl.NewColor(180, 180, 180, 255)
	// clearBrushColor is the muted dark-grey swatch used by every
	// "this brush sets the cell to its empty sentinel" entry (decor
	// Force-empty, props None/erase, entities Clear). Three brushes
	// used to declare rl.NewColor(60, 64, 70, 255) inline; centralized
	// here so a tweak is one edit.
	clearBrushColor = rl.NewColor(60, 64, 70, 255)

	// Label tint for drawLabel — the muted blue-grey field captions. Sole
	// label color; named so it lives with the rest of the editor chrome
	// instead of inline in draw.go.
	editorLabelColor = rl.NewColor(138, 160, 188, 220)

	// Semantic ok/warn pairs. The reachability metadata badge uses the
	// muted pair; the placement-footprint ghost uses the brighter pair
	// (it's a transient overlay that must pop against the map). Two pairs,
	// not one, because the two surfaces are tuned to different saturations
	// on purpose — naming them keeps the green/red intent explicit.
	editorReachOK   = rl.NewColor(70, 130, 100, 255)
	editorReachWarn = rl.NewColor(180, 80, 80, 255)
	editorPlaceOK   = rl.NewColor(120, 240, 140, 255)
	editorPlaceWarn = rl.NewColor(240, 110, 110, 255)
)

// withAlpha returns c with its alpha overridden — lets a base palette
// color be reused at different opacities (e.g. the placement ghost's
// bright outline vs. its faint fill) without a second NewColor literal.
func withAlpha(c rl.Color, a uint8) rl.Color {
	c.A = a
	return c
}

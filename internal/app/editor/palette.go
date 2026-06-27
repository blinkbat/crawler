package editor

import (
	"crawler/internal/app/core"
	"crawler/internal/app/render"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Editor chrome font ladder — its OWN smaller scale, not the render.Font*
// tokens (which are too large for the editor's tight buttons). sounds.go keeps
// its own soundFont* trio.
const (
	editorFontTopbar = float32(20) // topbar map-id label
	editorFontBody   = float32(18) // buttons + primary panel text
	editorFontLabel  = float32(16) // topbar info line / field labels
	editorFontAccent = float32(15) // dense list rows / palette hints
	editorFontHint   = float32(14) // compact modal/footer hints
	editorFontTiny   = float32(13) // sub-hint captions
	editorFontTick   = float32(12) // grid axis tick labels
)

// tooltipLineH is the line pitch for drawTooltipCard rows, shared by every tooltip
// caller so the row stride isn't conflated with (or smuggled in as) the font size.
const tooltipLineH = float32(14)

// bodyLineH is the body-font line height used to vertically center editorFontBody
// text in a row (r.Y + (r.Height-bodyLineH)/2).
const bodyLineH = float32(16)

// Editor UI chrome palette: the editor's own buttons, panels, borders, and
// overlays (map-content colors live in layerBrushes, editor.go).
var (
	// Panels/backgrounds, ascending in lightness. bgFieldInset (deepest) backs
	// editable text fields and the grid pane.
	bgFieldInset = rl.NewColor(14, 16, 22, 255)
	bgWindow     = rl.NewColor(20, 22, 30, 255)
	bgPaletteCol = rl.NewColor(24, 28, 38, 255)
	bgPanel      = rl.NewColor(28, 32, 44, 255)
	bgEntry      = rl.NewColor(36, 40, 52, 255)
	bgEntryHover = rl.NewColor(40, 46, 58, 255)
	bgButton     = rl.NewColor(48, 54, 70, 255)
	bgRowHover   = rl.NewColor(60, 70, 90, 255)
	bgActive     = rl.NewColor(72, 88, 130, 255)
	// panelBackingColor is the near-black backing behind floating canvas overlays
	// (minimap, brush-recents); each site applies its own alpha via withAlpha.
	panelBackingColor = rl.NewColor(12, 14, 20, 255)
	// Minimap pixel tones: flat floor base, and the lighter wall tone.
	minimapFloorCol = rl.NewColor(58, 56, 50, 255)
	minimapWallCol  = rl.NewColor(150, 152, 160, 255)

	// Borders. outlineHard is the dark panel seam. "editor" prefix avoids
	// shadowing theme.BorderDim/Active. editorBorderInactive frames readonly values.
	editorBorderInactive = rl.NewColor(50, 58, 76, 255)
	editorBorderDim      = rl.NewColor(70, 80, 100, 255)
	editorBorderMid      = rl.NewColor(96, 108, 132, 255)
	editorBorderActive   = rl.NewColor(180, 220, 244, 255)
	outlineHard          = rl.NewColor(8, 10, 14, 255)

	// Editor-specific text: brighter entry text + swatch outline (shared HUD
	// theme covers most strings).
	textBright   = rl.NewColor(220, 230, 245, 255)
	textEntry    = rl.NewColor(230, 234, 244, 255)
	textReadonly = rl.NewColor(200, 210, 230, 255)
	// Black scrims at varying alpha for the canvas grid + swatch rims. Local on
	// purpose (canvas chrome, not HUD shadows; render doesn't export those).
	swatchEdge    = rl.NewColor(0, 0, 0, 200)
	gridLineCol   = rl.NewColor(0, 0, 0, 80)
	gridLineMajor = rl.NewColor(0, 0, 0, 160)
	// glyphShadow is the drop-shadow behind canvas tile-glyphs + elevation digits.
	glyphShadow = withAlpha(outlineHard, 200)
	// selectionOutline is the white ring around brush-ghost / rect-drag / pack-drag previews.
	selectionOutline = rl.NewColor(255, 255, 255, 220)
	// marqueeOutline/Fill style the Select-tool copy/paste marquee. Amber to read
	// distinct from the white selectionOutline ghost on the same canvas.
	marqueeOutline = rl.NewColor(255, 206, 84, 235)
	marqueeFill    = rl.NewColor(255, 206, 84, 40)
	// entityMarkerOutline rings pack/chest/start markers. Aliases render.MarkerOutline
	// so canvas and minimap can't drift on the silhouette tone.
	entityMarkerOutline = render.MarkerOutline

	// Tile-color fallbacks. floorAutoColor: warm tan for unrecognized floor/decor
	// (also the FloorAuto swatch). editorFallbackColor: neutral grey for unknown
	// props. ceilingFallbackColor: muted brown for non-palette ceiling chars.
	floorAutoColor = rl.NewColor(160, 168, 140, 255)
	// wallSwatch is the base grey for the wall brush FAMILY; variants derive via
	// tintSwatch so walls read as one muted family.
	wallSwatch           = rl.NewColor(128, 128, 142, 255)
	editorFallbackColor  = rl.NewColor(200, 200, 200, 255)
	ceilingFallbackColor = rl.NewColor(110, 96, 80, 255)
	// entityFallbackColor: neutral swatch for an enemy kind with no entityBrushColors
	// entry; shared by the brush builder and the in-grid pack marker.
	entityFallbackColor = rl.NewColor(180, 180, 180, 255)
	// clearBrushColor: swatch for every "set cell to empty sentinel" brush
	// (decor Force-empty, props None/erase, entities Clear).
	clearBrushColor = rl.NewColor(60, 64, 70, 255)

	// editorLabelColor tints drawLabel field captions.
	editorLabelColor = rl.NewColor(138, 160, 188, 220)

	// sentinelLabelColor tints a sentinel brush's row label (e.g. erase/clear),
	// cooler than textEntry so it reads as a non-paint action.
	sentinelLabelColor = rl.NewColor(190, 200, 220, 255)

	// hiddenTabTextColor dims a hidden layer/elevation tab's label.
	hiddenTabTextColor = rl.NewColor(112, 116, 126, 255)

	// Visibility-eye glyph tints (drawLayerEye): resting, hidden-dim, hover.
	layerEyeNormal = rl.NewColor(176, 182, 196, 255)
	layerEyeDim    = rl.NewColor(98, 102, 112, 255)
	layerEyeHover  = rl.NewColor(232, 236, 246, 255)

	// editorActiveLevelText: warm tan for the "Active level" readout, shared by
	// the toolbar and sidebar/levels readouts.
	editorActiveLevelText = rl.NewColor(220, 210, 180, 255)

	// Hover-tooltip chrome: dark backing, light body, gilt heading.
	tooltipBG   = rl.NewColor(18, 22, 30, 230)
	tooltipText = rl.NewColor(220, 224, 234, 255)
	// Shares render.MarkerStart's gilt so a retune can't strand the heading.
	tooltipHeading = render.MarkerStart

	// Semantic ok/warn pairs. Muted pair = reachability badge; brighter pair =
	// placement ghost (must pop as a transient overlay).
	editorReachOK   = rl.NewColor(70, 130, 100, 255)
	editorReachWarn = rl.NewColor(180, 80, 80, 255)
	editorPlaceOK   = rl.NewColor(120, 240, 140, 255)
	editorPlaceWarn = rl.NewColor(240, 110, 110, 255)

	// Reachability badge FILL text (brighter green/red, legible inside the dark
	// outlined badges). editorReachWarnText also paints "(+N more)" via withAlpha.
	editorReachOKText   = rl.NewColor(150, 220, 180, 255)
	editorReachWarnText = rl.NewColor(240, 180, 180, 255)
)

// tintSwatch nudges a base swatch by per-channel deltas (clamped [0,255]),
// keeping alpha. Derives a family of related brush colors (e.g. wall variants).
func tintSwatch(base rl.Color, dr, dg, db int) rl.Color {
	return rl.NewColor(
		core.ClampByte(int(base.R)+dr),
		core.ClampByte(int(base.G)+dg),
		core.ClampByte(int(base.B)+db),
		base.A)
}

// withAlpha returns c with its alpha overridden. Thin alias over
// render.ColorWithAlpha.
func withAlpha(c rl.Color, a uint8) rl.Color {
	return render.ColorWithAlpha(c, a)
}

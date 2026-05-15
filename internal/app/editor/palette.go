package editor

import (
	rl "github.com/gen2brain/raylib-go/raylib"
)

// Editor UI chrome palette. The map-content colors (tile brushes, swatches)
// already live in layerBrushes (editor.go); these are the colors of the
// editor's own buttons, panels, borders, and overlays so they're not
// duplicated as raw rl.NewColor literals across draw.go.
var (
	// Panels and backgrounds — ascend in lightness from window backdrop to
	// hover-highlighted entries.
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
	// at call sites that consume both.
	editorBorderDim    = rl.NewColor(70, 80, 100, 255)
	editorBorderMid    = rl.NewColor(96, 108, 132, 255)
	editorBorderActive = rl.NewColor(180, 220, 244, 255)
	outlineHard        = rl.NewColor(8, 10, 14, 255)

	// Text colors specific to the editor; the shared HUD theme covers most
	// strings, these handle the brighter entry text and the swatch outline.
	textBright   = rl.NewColor(220, 230, 245, 255)
	textEntry    = rl.NewColor(230, 234, 244, 255)
	swatchEdge   = rl.NewColor(0, 0, 0, 200)
	gridLineCol  = rl.NewColor(0, 0, 0, 80)
	overlayShade = rl.NewColor(0, 0, 0, 185)
)

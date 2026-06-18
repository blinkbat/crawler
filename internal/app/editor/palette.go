package editor

import (
	"crawler/internal/app/render"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Editor type scale for the chrome the topbar / toolbar / buttons draw.
// The map editor runs its OWN smaller scale rather than the render.Font*
// tokens (whose FontSmall is already 16 and FontBody 20 — too large for
// this chrome, and large enough to overflow the editor's tight buttons).
// editorFontAccent/Tiny cover the dense list-row and sub-hint sizes;
// sounds.go keeps its own soundFont* trio for the sound modal.
const (
	// Bumped one notch up (was 18/16/14/13/12/11/10) alongside the game's
	// canonical sizes for the Della Respira face.
	editorFontTopbar = float32(20) // topbar map-id label
	editorFontBody   = float32(18) // buttons + primary panel text
	editorFontLabel  = float32(16) // topbar info line / field labels
	editorFontAccent = float32(15) // dense list rows / palette hints
	editorFontHint   = float32(14) // compact modal/footer hints
	editorFontTiny   = float32(13) // sub-hint captions
	editorFontTick   = float32(12) // grid axis tick labels
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
	textBright   = rl.NewColor(220, 230, 245, 255)
	textEntry    = rl.NewColor(230, 234, 244, 255)
	textReadonly = rl.NewColor(200, 210, 230, 255)
	// Plain black scrims at varying alpha for the canvas grid + swatch
	// rims. These happen to share RGBA with render's shadowStrong (200) /
	// shadowLight (160), but they're a different purpose (canvas chrome,
	// not HUD text shadows) and render doesn't export those tokens — kept
	// local on purpose. Promote to a shared "black scrim" token only if
	// more sites accrue.
	swatchEdge    = rl.NewColor(0, 0, 0, 200)
	gridLineCol   = rl.NewColor(0, 0, 0, 80)
	gridLineMajor = rl.NewColor(0, 0, 0, 160)
	// glyphShadow is the dark drop-shadow behind canvas tile-glyphs and
	// elevation digits — outlineHard at 200 alpha (was an inline
	// rl.NewColor(8,10,14,200) at the char-overlay and elevation-slice draws).
	glyphShadow = withAlpha(outlineHard, 200)
	// selectionOutline is the white ring around the brush-ghost,
	// rectangle-drag, and pack-drag previews on the canvas (was a 200/220
	// alpha split across the three sites that read as unintentional).
	selectionOutline = rl.NewColor(255, 255, 255, 220)
	// marqueeOutline / marqueeFill style the region copy/paste marquee (the
	// Select tool). Amber so the committed region reads distinct from the
	// white brush / rectangle-drag ghost (selectionOutline) on the same canvas.
	marqueeOutline = rl.NewColor(255, 206, 84, 235)
	marqueeFill    = rl.NewColor(255, 206, 84, 40)
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
	floorAutoColor = rl.NewColor(160, 168, 140, 255)
	// wallSwatch is the base grey for the wall brush FAMILY (plain rock +
	// ivy/cracked/crumbling variants). Variants derive from it via tintSwatch
	// so the editor canvas reads walls as one muted family — light enough to
	// stay legible against the dark grid and distinct from the warm floor tan.
	wallSwatch           = rl.NewColor(128, 128, 142, 255)
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

	// hiddenTabTextColor dims a layer/elevation tab's label when that layer
	// is hidden, so the hidden state reads across the whole tab (not just the
	// eye). Two tab draws used to declare this inline.
	hiddenTabTextColor = rl.NewColor(112, 116, 126, 255)

	// Hover-tooltip chrome: a near-opaque dark backing, light body text, and
	// a gilt heading tint for the first line. Named here with the rest of the
	// editor chrome rather than inline in draw.go's tooltip draw.
	tooltipBG   = rl.NewColor(18, 22, 30, 230)
	tooltipText = rl.NewColor(220, 224, 234, 255)
	// Same gilt the render theme uses for entity-marker starts (already
	// consumed here as render.MarkerStart in draw.go) — share the token so a
	// gilt retune can't leave the tooltip heading behind.
	tooltipHeading = render.MarkerStart

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

// tintSwatch nudges a base swatch by per-channel deltas (clamped to [0,255]),
// keeping alpha. Used to derive a family of closely-related brush colors — e.g.
// the wall variants shift off wallSwatch toward green-grey (ivy) or browner
// grey (cracked/crumbling) so they read as one family yet stay distinguishable.
func tintSwatch(base rl.Color, dr, dg, db int) rl.Color {
	clamp := func(v, d int) uint8 {
		v += d
		if v < 0 {
			v = 0
		} else if v > 255 {
			v = 255
		}
		return uint8(v)
	}
	return rl.NewColor(clamp(int(base.R), dr), clamp(int(base.G), dg), clamp(int(base.B), db), base.A)
}

// withAlpha returns c with its alpha overridden — lets a base palette
// color be reused at different opacities (e.g. the placement ghost's
// bright outline vs. its faint fill) without a second NewColor literal.
// Thin alias over render.ColorWithAlpha so the set-alpha logic lives once
// in render/theme.go (rl.Color is a color.RGBA alias, so it passes through).
func withAlpha(c rl.Color, a uint8) rl.Color {
	return render.ColorWithAlpha(c, a)
}

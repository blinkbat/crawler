package render

import (
	"image/color"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Theme is the shared HUD palette. Returned from (Resources).Theme() so
// scenes outside the render package (title, editor) draw with the same
// surface/border/text colors as the in-game HUD.
type Theme struct {
	SurfacePrimary    rl.Color
	SurfaceLog        rl.Color
	SurfaceVeil       rl.Color
	SurfaceActiveTint rl.Color // selection-row tint, paired with BorderActive
	BorderDim         rl.Color
	BorderSoft        rl.Color
	BorderStrong      rl.Color
	BorderActive      rl.Color
	BorderDanger      rl.Color
	TextPrimary       rl.Color
	TextMuted         rl.Color
	TextLabel         rl.Color
	TextDim           rl.Color
	TextHint          rl.Color
	// Entity-marker colors shared by the editor canvas and the in-game
	// minimap. Both surfaces draw the same notion of "this tile holds
	// a chest / door / start / pack" so the colors live in the theme
	// rather than as private literals on each side.
	MarkerStart    rl.Color
	MarkerChest    rl.Color
	MarkerChestDim rl.Color
	MarkerDoor     rl.Color
	MarkerPack     rl.Color
	MarkerPackEdge rl.Color
	MarkerOutline  rl.Color
}

// Theme returns the HUD's color palette. The values mirror the package-
// internal vars in theme.go; this is the single seam external scenes use.
func (r Resources) Theme() Theme {
	return Theme{
		SurfacePrimary:    surfacePrimary,
		SurfaceLog:        surfaceLog,
		SurfaceVeil:       surfaceVeil,
		SurfaceActiveTint: surfaceActiveTint,
		BorderDim:         borderDim,
		BorderSoft:        borderSoft,
		BorderStrong:      borderStrong,
		BorderActive:      borderActive,
		BorderDanger:      borderDanger,
		TextPrimary:       textPrimary,
		TextMuted:         textMuted,
		TextLabel:         textLabel,
		TextDim:           textDim,
		TextHint:          textHint,
		MarkerStart:       markerStart,
		MarkerChest:       markerChest,
		MarkerChestDim:    markerChestDim,
		MarkerDoor:        markerDoor,
		MarkerPack:        markerPack,
		MarkerPackEdge:    markerPackEdge,
		MarkerOutline:     markerOutline,
	}
}

// Exported entity-marker color vars. Mirror the package-private theme
// vars so non-Resources callers (the editor's init-time brush palette)
// can read them without first constructing a Resources. Initialized in
// theme.go's var block, so editor init (which runs after render init,
// since editor imports render) sees the populated values.
var (
	MarkerStart    = markerStart
	MarkerChest    = markerChest
	MarkerChestDim = markerChestDim
	MarkerDoor     = markerDoor
	MarkerPack     = markerPack
	MarkerPackEdge = markerPackEdge
	MarkerOutline  = markerOutline
)

// DrawCard fills + outlines a rounded panel with the standard corner radius
// and adds the left accent stripe. Public alias for drawCard.
func DrawCard(x, y, w, h int32, fill, outline, accent color.RGBA) {
	drawCard(x, y, w, h, fill, outline, accent)
}

// DrawHeading writes a small uppercase header with a colored underline tick.
func DrawHeading(font rl.Font, text string, x, y int32, accent color.RGBA) {
	drawHeading(font, text, x, y, accent)
}

// DrawSubHeading writes the second-tier heading style used inside modal
// columns ("Synth params", "Saved sounds", etc.) — a drop-shadowed label
// at 18px in the accent color. Lighter than DrawHeading, no underline.
// Centralized here so modal authors don't reach for DrawTextWithShadow at
// an ad-hoc size and drift the visual tier.
func DrawSubHeading(font rl.Font, text string, x, y float32, accent color.RGBA) {
	drawTextWithShadow(font, text, x, y, FontBody, accent)
}

// DrawTextWithShadow draws text with a dark drop shadow.
func DrawTextWithShadow(font rl.Font, text string, x, y, size float32, col color.RGBA) {
	drawTextWithShadow(font, text, x, y, size, col)
}

// DrawSelectedRow paints the standard "cursor is on this row"
// highlight per UI_STANDARDS.md "Row > Selected": warm glass fill,
// gilt left spine (3 px), and a thin gilt-dim underline along the
// bottom edge. Replaces the older blue-purple "active-tint" fill
// so every list-style surface speaks the library aesthetic.
//
// Used by modal pickers (chest, level-up, sound editor) and any
// non-`drawListRow` cursor highlight. New code should prefer
// drawListRow directly for the full per-state ladder.
func DrawSelectedRow(r rl.Rectangle) {
	rl.DrawRectangleRec(r, glassWarm)
	// Gilt left spine — 3 px, vertically inset 5 px top/bottom so it
	// reads as a marker rather than the entire left edge. Termini
	// pips at each end of the spine give it the "illuminated
	// manuscript marker bead" feel rather than a bare strip.
	spineX := int32(r.X) + 4
	spineTop := int32(r.Y) + 5
	spineH := int32(r.Height) - 10
	rl.DrawRectangle(spineX, spineTop, 3, spineH, giltBright)
	pipX := float32(spineX) + 1
	drawDiamondPip(pipX, float32(spineTop)-1, 2.5, giltBright)
	drawDiamondPip(pipX, float32(spineTop+spineH)+1, 2.5, giltBright)
	// Underline along the bottom edge, capped by tiny pips so the
	// underline doesn't read as a bare line.
	underY := int32(r.Y+r.Height) - 3
	underX := int32(r.X) + 12
	underW := int32(r.Width) - 24
	rl.DrawRectangle(underX, underY, underW, 1, giltDim)
	drawDiamondPip(float32(underX), float32(underY), 1.5, giltDim)
	drawDiamondPip(float32(underX+underW), float32(underY), 1.5, giltDim)
}

// DrawSelectedRowI is the int32-coords variant of DrawSelectedRow for
// callers that already work in pixel-snapped int32 layouts (pause menu,
// battle action row). Same visual contract — the two helpers exist
// only because raylib's rect-fill takes a float Rectangle while its
// rect-fill-i takes int32 directly, and converting at every call site
// added noise without changing the surface/border combo we want shared.
func DrawSelectedRowI(x, y, w, h int32) {
	drawSmallPanel(x, y, w, h, glassWarm)
	spineX := x + 4
	spineTop := y + 5
	spineH := h - 10
	rl.DrawRectangle(spineX, spineTop, 3, spineH, giltBright)
	pipX := float32(spineX) + 1
	drawDiamondPip(pipX, float32(spineTop)-1, 2.5, giltBright)
	drawDiamondPip(pipX, float32(spineTop+spineH)+1, 2.5, giltBright)
	underY := y + h - 3
	underX := x + 12
	underW := w - 24
	rl.DrawRectangle(underX, underY, underW, 1, giltDim)
	drawDiamondPip(float32(underX), float32(underY), 1.5, giltDim)
	drawDiamondPip(float32(underX+underW), float32(underY), 1.5, giltDim)
}

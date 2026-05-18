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
	}
}

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
	drawTextWithShadow(font, text, x, y, 18, accent)
}

// DrawTextWithShadow draws text with a dark drop shadow.
func DrawTextWithShadow(font rl.Font, text string, x, y, size float32, col color.RGBA) {
	drawTextWithShadow(font, text, x, y, size, col)
}

// DrawSelectedRow paints the standard "cursor is on this row" highlight:
// the active-tint fill plus the active-color outline. Used by modal
// row lists (sound editor, future pickers) so the selection style stays
// consistent with the pause menu without duplicating the two-call
// fill + outline block at every call site.
func DrawSelectedRow(r rl.Rectangle) {
	rl.DrawRectangleRec(r, surfaceActiveTint)
	rl.DrawRectangleLinesEx(r, 1, borderActive)
}

// DrawSelectedRowI is the int32-coords variant of DrawSelectedRow for
// callers that already work in pixel-snapped int32 layouts (pause menu,
// battle action row). Same visual contract — the two helpers exist
// only because raylib's rect-fill takes a float Rectangle while its
// rect-fill-i takes int32 directly, and converting at every call site
// added noise without changing the surface/border combo we want shared.
func DrawSelectedRowI(x, y, w, h int32) {
	drawSmallPanel(x, y, w, h, surfaceActiveTint)
	drawSmallPanelOutline(x, y, w, h, borderActive)
}

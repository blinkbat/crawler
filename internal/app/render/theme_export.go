package render

import (
	"image/color"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Theme is the shared HUD palette. Returned from (Resources).Theme() so
// scenes outside the render package (title, editor) draw with the same
// surface/border/text colors as the in-game HUD.
type Theme struct {
	SurfacePrimary rl.Color
	SurfaceLog     rl.Color
	SurfaceVeil    rl.Color
	BorderDim      rl.Color
	BorderSoft     rl.Color
	BorderStrong   rl.Color
	BorderActive   rl.Color
	BorderDanger   rl.Color
	TextPrimary    rl.Color
	TextMuted      rl.Color
	TextLabel      rl.Color
	TextDim        rl.Color
	TextHint       rl.Color
}

// Theme returns the HUD's color palette. The values mirror the package-
// internal vars in theme.go; this is the single seam external scenes use.
func (r Resources) Theme() Theme {
	return Theme{
		SurfacePrimary: surfacePrimary,
		SurfaceLog:     surfaceLog,
		SurfaceVeil:    surfaceVeil,
		BorderDim:      borderDim,
		BorderSoft:     borderSoft,
		BorderStrong:   borderStrong,
		BorderActive:   borderActive,
		BorderDanger:   borderDanger,
		TextPrimary:    textPrimary,
		TextMuted:      textMuted,
		TextLabel:      textLabel,
		TextDim:        textDim,
		TextHint:       textHint,
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

// DrawTextWithShadow draws text with a dark drop shadow.
func DrawTextWithShadow(font rl.Font, text string, x, y, size float32, col color.RGBA) {
	drawTextWithShadow(font, text, x, y, size, col)
}

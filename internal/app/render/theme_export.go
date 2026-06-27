package render

import (
	"image/color"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Theme is the shared HUD palette, returned from (Resources).Theme() so scenes
// outside the render package draw with the same surface/border/text colors.
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
	// Entity-marker colors shared by the editor canvas and the in-game minimap.
	MarkerStart    rl.Color
	MarkerChest    rl.Color
	MarkerChestDim rl.Color
	MarkerDoor     rl.Color
	MarkerCrystal  rl.Color
	MarkerPack     rl.Color
	MarkerOutline  rl.Color
}

// Theme returns the HUD's color palette, mirroring the package-internal vars in theme.go.
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
		TextHint:          textDim,
		// Marker colors derive from the exported Marker* vars (single source of truth).
		MarkerStart:    MarkerStart,
		MarkerChest:    MarkerChest,
		MarkerChestDim: MarkerChestDim,
		MarkerDoor:     MarkerDoor,
		MarkerCrystal:  MarkerCrystal,
		MarkerPack:     MarkerPack,
		MarkerOutline:  MarkerOutline,
	}
}

// Exported entity-marker color vars, mirroring the package-private theme vars so
// non-Resources callers (editor brush palette) can read them without a Resources.
var (
	MarkerStart    = markerStart
	MarkerChest    = markerChest
	MarkerChestDim = markerChestDim
	MarkerDoor     = markerDoor
	MarkerCrystal  = markerCrystal
	MarkerPack     = markerPack
	MarkerOutline  = markerOutline
)

// FadeColor scales col's alpha by the 0..1 multiplier (clamped). Alias for fadeColor.
func FadeColor(col color.RGBA, alpha float32) color.RGBA {
	return fadeColor(col, alpha)
}

// ColorWithAlpha replaces col's alpha with byteAlpha (set, don't scale). Alias for colorWithAlpha.
func ColorWithAlpha(col color.RGBA, byteAlpha uint8) color.RGBA {
	return colorWithAlpha(col, byteAlpha)
}

// DrawCard fills + outlines a rounded panel with a left accent stripe. Alias for drawCard.
func DrawCard(x, y, w, h int32, fill, outline, accent color.RGBA) {
	drawCard(x, y, w, h, fill, outline, accent)
}

// DrawHeading writes a small uppercase header with a colored underline tick.
func DrawHeading(font rl.Font, text string, x, y int32, accent color.RGBA) {
	drawHeading(font, text, x, y, accent)
}

// DrawSubHeading writes the second-tier modal-column heading: a drop-shadowed
// FontBody label in the accent color, lighter than DrawHeading, no underline.
func DrawSubHeading(font rl.Font, text string, x, y float32, accent color.RGBA) {
	drawTextWithShadow(font, text, x, y, FontBody, accent)
}

// DrawTextWithShadow draws text with a dark drop shadow.
func DrawTextWithShadow(font rl.Font, text string, x, y, size float32, col color.RGBA) {
	drawTextWithShadow(font, text, x, y, size, col)
}

// DrawEngravedText is the exported top-lit gradient lettering (see drawEngravedText).
// Heading-tier and up only.
func DrawEngravedText(font rl.Font, text string, x, y, size float32, col color.RGBA) {
	drawEngravedText(font, text, x, y, size, col)
}

// selectionPlateShrinkY is how much shorter than its row a selection plate draws
// so the gilt highlight floats inside the row band. Shared by level-up + journal.
const selectionPlateShrinkY = int32(6)

// Selection-plate bleed: how far SelectionRowRect grows a row past its text origin.
const (
	selectionPlateInsetX = int32(6)
	selectionPlateInsetY = int32(4)
)

// SelectionRowRect insets a row's (x, y, w) by the canonical DrawSelectedRow
// padding; height passes through unchanged.
func SelectionRowRect(x, y, w, h int32) rl.Rectangle {
	return rl.NewRectangle(float32(x-selectionPlateInsetX), float32(y-selectionPlateInsetY), float32(w+2*selectionPlateInsetX), float32(h))
}

// drawModalListRow paints the gilt selection plate when focused, then runs fn to
// draw the row content. Shared by the chest and dialog-choice modals.
func drawModalListRow(x, y, w, h int32, focused bool, fn func()) {
	if focused {
		DrawSelectedRow(SelectionRowRect(x, y, w, h))
	}
	fn()
}

// drawModalTextRow is the common case of drawModalListRow: optional cursor
// highlight plus one left-aligned body label at the row origin. Shared by the
// chest and dialog-choice modals so the highlight+label pairing lives in one place.
func drawModalTextRow(font rl.Font, x, y, w, h int32, focused bool, label string, col rl.Color) {
	drawModalListRow(x, y, w, h, focused, func() {
		drawTextWithShadow(font, label, float32(x), float32(y), FontBody, col)
	})
}

// modalTextRow is one entry for drawModalTextRowList: its label, whether the cursor
// rests on it, whether it's disabled (greyed + non-focusable), and the disabled tint.
type modalTextRow struct {
	label       string
	focused     bool
	disabled    bool
	disabledCol rl.Color
}

// drawModalTextRowList paints rows top-down from y at (rowX, rowW, rowH), each tinted
// via rowTextColor and highlighted only when focused && !disabled; returns the y past
// the last row. Shared by the dialog + chest modals so the per-row tint/draw idiom
// lives in one place.
func drawModalTextRowList(font rl.Font, rowX, y, rowW, rowH int32, rows []modalTextRow) int32 {
	for _, r := range rows {
		col := rowTextColor(r.focused, r.disabled, r.disabledCol)
		drawModalTextRow(font, rowX, y, rowW, rowH, r.focused && !r.disabled, r.label, col)
		y += rowH
	}
	return y
}

// DrawSelectedRow paints the standard cursor-on-row highlight per UI_STANDARDS.md
// "Row > Selected": warm glass fill, 3px gilt left spine, thin gilt-dim underline.
func DrawSelectedRow(r rl.Rectangle) {
	flick := candleFlicker()
	drawShadowedGlassPane(r, glassWarm)
	rl.DrawRectangleLinesEx(r, 1, fadeColor(giltDim, 0.75*flick))
	if r.Width > 24 && r.Height > 10 {
		rl.DrawRectangleGradientV(
			int32(r.X+8), int32(r.Y+3), int32(r.Width-16), int32(r.Height/2),
			fadeColor(giltBright, 0.20*flick),
			fadeColor(giltBright, 0.02),
		)
	}
	drawRowSheen(r, flick)
	// Gilt left spine — 3px, inset 5px top/bottom, with terminus pips.
	spineX := int32(r.X) + 4
	spineTop := int32(r.Y) + 5
	spineH := int32(r.Height) - 10
	rl.DrawRectangle(spineX, spineTop, 3, spineH, giltBright)
	pipX := float32(spineX) + 1
	drawDiamondPip(pipX, float32(spineTop)-1, 2.5, giltBright)
	drawDiamondPip(pipX, float32(spineTop+spineH)+1, 2.5, giltBright)
	if r.Width >= 96 && r.Height >= 28 {
		drawFleuron(r.X+r.Width-16, r.Y+r.Height/2, 2.4, fadeColor(giltDim, 0.55))
	}
	// Underline along the bottom edge, capped by tiny pips.
	underY := int32(r.Y+r.Height) - 3
	underX := int32(r.X) + 12
	underW := int32(r.Width) - 24
	rl.DrawRectangle(underX, underY, underW, 1, giltDim)
	drawDiamondPip(float32(underX), float32(underY), 1.5, giltDim)
	drawDiamondPip(float32(underX+underW), float32(underY), 1.5, giltDim)
}

// DrawFleuron is the exported wrapper around drawFleuron — the four-direction
// gilt sigil used as ornamental punctuation.
func DrawFleuron(cx, cy, r float32, col rl.Color) {
	drawFleuron(cx, cy, r, col)
}

// DrawTitleRule paints a heraldic banner divider — a gilt rule with a centred
// fleuron and flanking end fleurons. Width w spans end-fleuron centre to centre.
func DrawTitleRule(x, y, w float32) {
	endFlR := float32(5)
	midFlR := float32(4)
	cx := x + w/2
	// Inset the line from the end fleurons so they read as caps.
	leftStart := x + endFlR + 4
	rightEnd := x + w - endFlR - 4
	drawSplitRule(leftStart, rightEnd, cx, y, midFlR+6, giltDim)
	drawFleuron(x+endFlR, y, endFlR, giltDim)
	drawFleuron(x+w-endFlR, y, endFlR, giltDim)
	drawFleuron(cx, y, midFlR, giltBright)
}

// DrawSelectedRowI is the int32-coords variant of DrawSelectedRow.
func DrawSelectedRowI(x, y, w, h int32) {
	DrawSelectedRow(rl.NewRectangle(float32(x), float32(y), float32(w), float32(h)))
}

package render

import (
	"fmt"
	"image/color"
	"math"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Shared HUD palette. Surfaces ascend in opacity/depth:
// veil < log < primary; tints overlay primary for state.
var (
	surfacePrimary    = rl.NewColor(12, 16, 28, 222)
	surfaceLog        = rl.NewColor(4, 6, 12, 148)
	surfaceVeil       = rl.NewColor(0, 0, 0, 130)
	surfaceActiveTint = rl.NewColor(58, 52, 96, 215)
	surfaceTargetTint = rl.NewColor(30, 64, 70, 215)
	surfaceDownTint   = rl.NewColor(28, 22, 28, 165)
	surfaceEnemyTint  = rl.NewColor(54, 28, 32, 205)

	borderDim    = rl.NewColor(98, 124, 158, 95)
	borderSoft   = rl.NewColor(122, 158, 196, 160)
	borderStrong = rl.NewColor(170, 220, 244, 220)
	borderActive = rl.NewColor(255, 220, 124, 235)
	borderTarget = rl.NewColor(118, 235, 136, 235)
	borderEnemy  = rl.NewColor(255, 144, 96, 230)
	borderDanger = rl.NewColor(244, 90, 90, 235)

	textPrimary = rl.NewColor(244, 248, 252, 255)
	textMuted   = rl.NewColor(190, 204, 224, 240)
	textLabel   = rl.NewColor(146, 174, 204, 235)
	textDim     = rl.NewColor(118, 134, 158, 220)
	textHint    = rl.NewColor(138, 160, 188, 220)

	barHPHigh  = rl.NewColor(108, 220, 132, 255)
	barHPMid   = rl.NewColor(232, 188, 88, 255)
	barHPLow   = rl.NewColor(236, 90, 90, 255)
	barMP      = rl.NewColor(96, 162, 232, 255)
	barEnemyHP = rl.NewColor(216, 80, 76, 255)
	barBurn    = rl.NewColor(248, 132, 64, 255)

	// Billboard tints for the in-world combatant markers — the warm
	// off-white the player's target reads as, and the slightly redder
	// pulse the currently-attacking enemy reads as. Pulled out here so
	// future palette passes don't have to chase NewColor literals across
	// world.go's draw loop.
	tintEnemyTargeted = rl.NewColor(255, 228, 190, 255)
	tintEnemyAttacker = rl.NewColor(255, 196, 156, 255)

	// Shadow tints for drop-shadowed text and overlay scrims. Pre-named so
	// callers don't open-code rl.NewColor(0,0,0,…) with a drifting alpha.
	// Strength runs Light (background hints) → Mid (HUD body) → Strong
	// (large titles / debug pills) → Heavy (top-of-stack labels).
	shadowLight  = rl.NewColor(0, 0, 0, 160)
	shadowMid    = rl.NewColor(0, 0, 0, 180)
	shadowStrong = rl.NewColor(0, 0, 0, 200)
	shadowHeavy  = rl.NewColor(0, 0, 0, 220)
)

const (
	cornerRadius      = float32(10)
	smallCornerRadius = float32(6)
	stripeWidth       = int32(3)

	// Heading tick markers (drawHeading underline) have a minimum width so
	// short headings still read as labelled. Bar value text inset is the
	// constant pad on the right edge of drawBar.
	headingTickMinWidth = int32(28)
	barValuePadRight    = float32(10)
	barLabelPadLeft     = float32(8)

	// World-popup horizontal slack: how many pixels past the screen edges a
	// 3D-to-2D projected popup can drift before we cull it. Larger than zero
	// so a popup whose anchor moves slightly off-screen still fades cleanly
	// instead of snapping to invisible mid-animation.
	offscreenPopupSlack = float32(200)
)

// drawPanel fills a rounded rect at a fixed pixel corner radius.
func drawPanel(x, y, w, h int32, fill color.RGBA) {
	if w <= 0 || h <= 0 {
		return
	}
	rect := rl.NewRectangle(float32(x), float32(y), float32(w), float32(h))
	rl.DrawRectangleRounded(rect, fixedRoundnessFor(w, h, cornerRadius), 8, fill)
}

func drawPanelOutline(x, y, w, h int32, col color.RGBA) {
	if w <= 0 || h <= 0 {
		return
	}
	rect := rl.NewRectangle(float32(x), float32(y), float32(w), float32(h))
	rl.DrawRectangleRoundedLinesEx(rect, fixedRoundnessFor(w, h, cornerRadius), 8, 1, col)
}

func drawSmallPanel(x, y, w, h int32, fill color.RGBA) {
	if w <= 0 || h <= 0 {
		return
	}
	rect := rl.NewRectangle(float32(x), float32(y), float32(w), float32(h))
	rl.DrawRectangleRounded(rect, fixedRoundnessFor(w, h, smallCornerRadius), 6, fill)
}

func drawSmallPanelOutline(x, y, w, h int32, col color.RGBA) {
	if w <= 0 || h <= 0 {
		return
	}
	rect := rl.NewRectangle(float32(x), float32(y), float32(w), float32(h))
	rl.DrawRectangleRoundedLinesEx(rect, fixedRoundnessFor(w, h, smallCornerRadius), 6, 1, col)
}

func fixedRoundnessFor(w, h int32, target float32) float32 {
	minDim := float32(core.MinInt(int(w), int(h)))
	if minDim <= 0 {
		return 0
	}
	r := 2 * target / minDim
	if r > 1 {
		r = 1
	}
	return r
}

// drawAccentStripe paints a thin colored bar inside a panel's left edge,
// inset slightly so it reads as part of the card rather than its border.
func drawAccentStripe(panelX, panelY, panelH int32, col color.RGBA) {
	if panelH < 16 {
		return
	}
	rl.DrawRectangle(panelX+5, panelY+8, stripeWidth, panelH-16, col)
}

// drawCard fills + outlines a panel and adds the left accent stripe.
func drawCard(x, y, w, h int32, fill, outline, accent color.RGBA) {
	drawPanel(x, y, w, h, fill)
	drawPanelOutline(x, y, w, h, outline)
	if accent.A > 0 {
		drawAccentStripe(x, y, h, accent)
	}
}

// pulse oscillates 0..1 at the given frequency in Hz.
func pulse(speed float64) float32 {
	return 0.5 + 0.5*float32(math.Sin(rl.GetTime()*speed*math.Pi*2))
}

// fadeColor returns col scaled by alpha multiplier in 0..1.
func fadeColor(col color.RGBA, alpha float32) color.RGBA {
	if alpha < 0 {
		alpha = 0
	}
	if alpha > 1 {
		alpha = 1
	}
	col.A = uint8(float32(col.A) * alpha)
	return col
}

// hpFillColor selects a tier color based on remaining HP percent.
func hpFillColor(value, maxValue int) color.RGBA {
	if maxValue <= 0 {
		return barHPLow
	}
	p := float32(value) / float32(maxValue)
	switch {
	case p > 0.6:
		return barHPHigh
	case p > 0.3:
		return barHPMid
	default:
		return barHPLow
	}
}

// drawBar renders a track + filled portion + thin outline, all rounded.
// label is drawn as a small uppercase tag at the bar's left, value text on right.
func drawBar(font rl.Font, x, y, width, height float32, label string, value, maxValue int, fill color.RGBA, muted bool) {
	if maxValue <= 0 {
		maxValue = 1
	}
	pct := float32(value) / float32(maxValue)
	if pct < 0 {
		pct = 0
	}
	if pct > 1 {
		pct = 1
	}
	track := rl.NewColor(8, 12, 22, 200)
	outline := borderDim
	if muted {
		fill = rl.NewColor(96, 84, 92, 230)
	}
	ix, iy, iw, ih := int32(x), int32(y), int32(width), int32(height)
	drawSmallPanel(ix, iy, iw, ih, track)
	if pct > 0 {
		fillW := int32(float32(iw-2) * pct)
		if fillW > 0 {
			drawSmallPanel(ix+1, iy+1, fillW, ih-2, fill)
		}
	}
	drawSmallPanelOutline(ix, iy, iw, ih, outline)

	labelSize := float32(13)
	if height < 18 {
		labelSize = 12
	}
	labelMeasure := rl.MeasureTextEx(font, label, labelSize, 1)
	labelY := y + (float32(ih)-labelMeasure.Y)/2 - 1
	rl.DrawTextEx(font, label, rl.NewVector2(x+barLabelPadLeft, labelY+1), labelSize, 1, shadowMid)
	rl.DrawTextEx(font, label, rl.NewVector2(x+barLabelPadLeft, labelY), labelSize, 1, fadeColor(textLabel, 1))

	valText := ""
	if maxValue > 0 {
		valText = formatBarValue(value, maxValue)
	}
	if valText != "" {
		// Value scales with bar height — taller bars get a bigger, more
		// readable number. Bright by default; faded only when the member is
		// muted (down). A double-offset drop shadow gives clean contrast
		// against any fill color.
		valSize := labelSize
		if height > 20 {
			valSize = labelSize + (height-20)*0.55
		}
		valColor := textPrimary
		if muted {
			valColor = textDim
		}
		valMeasure := rl.MeasureTextEx(font, valText, valSize, 1)
		valY := y + (float32(ih)-valMeasure.Y)/2 - 1
		valX := x + width - valMeasure.X - barValuePadRight
		rl.DrawTextEx(font, valText, rl.NewVector2(valX+2, valY+2), valSize, 1, shadowHeavy)
		rl.DrawTextEx(font, valText, rl.NewVector2(valX+1, valY+1), valSize, 1, shadowHeavy)
		rl.DrawTextEx(font, valText, rl.NewVector2(valX, valY), valSize, 1, valColor)
	}
}

func formatBarValue(value, maxValue int) string {
	return fmt.Sprintf("%d/%d", value, maxValue)
}

// drawArrowMarker paints a small triangle chevron. The base sits at `center`
// perpendicular to the direction; the apex is `center + (tipDx, tipDy)`.
// Base width is 2*halfWidth. Used by HUD selection / target / active-actor
// indicators where a tiny arrow reads better than a label — saves party,
// battle, and item-target panels from each computing their own three
// rl.Vector2 corners by hand.
func drawArrowMarker(center rl.Vector2, tipDx, tipDy, halfWidth float32, col color.RGBA) {
	tipLen := float32(math.Sqrt(float64(tipDx*tipDx + tipDy*tipDy)))
	if tipLen == 0 {
		return
	}
	px := -tipDy / tipLen * halfWidth
	py := tipDx / tipLen * halfWidth
	rl.DrawTriangle(
		rl.NewVector2(center.X+tipDx, center.Y+tipDy),
		rl.NewVector2(center.X-px, center.Y-py),
		rl.NewVector2(center.X+px, center.Y+py),
		col,
	)
}

// drawTextWithShadow paints text twice: once offset by (1,1) at shadowStrong,
// once at the requested color. The single +1 offset reads as a clean drop
// shadow under most HUD sizes; callers that want a heavier shadow for large
// titles (menu rows, debug pills) go through drawTextWithShadowStyle. Lives
// here alongside the shadowLight/Mid/Strong/Heavy palette it consumes.
func drawTextWithShadow(font rl.Font, text string, x, y, size float32, col color.RGBA) {
	drawTextWithShadowStyle(font, text, x, y, size, col, shadowStrong, 1, 1)
}

// drawTextWithShadowStyle is the parametric form of drawTextWithShadow. shadowCol
// picks the drop color (shadowLight/Mid/Strong/Heavy above); offX/offY pick the
// drop offset in pixels. Use this when an ad-hoc shadow alpha or offset is
// actually load-bearing (splash titles, menu rows); prefer the non-styled
// drawTextWithShadow for everything else so HUD shadows stay consistent.
func drawTextWithShadowStyle(font rl.Font, text string, x, y, size float32, col, shadowCol color.RGBA, offX, offY float32) {
	rl.DrawTextEx(font, text, rl.NewVector2(x+offX, y+offY), size, 1, shadowCol)
	rl.DrawTextEx(font, text, rl.NewVector2(x, y), size, 1, col)
}

// drawHeading writes a small uppercase header inside a panel, with a colored
// underline tick to give it weight.
func drawHeading(font rl.Font, text string, x, y int32, accent color.RGBA) {
	size := float32(20)
	spacing := float32(1.8)
	pos := rl.NewVector2(float32(x), float32(y))
	rl.DrawTextEx(font, text, rl.NewVector2(pos.X+1, pos.Y+1), size, spacing, shadowStrong)
	rl.DrawTextEx(font, text, pos, size, spacing, textLabel)
	measure := rl.MeasureTextEx(font, text, size, spacing)
	tickW := int32(measure.X)
	if tickW < headingTickMinWidth {
		tickW = headingTickMinWidth
	}
	rl.DrawRectangle(x, y+int32(measure.Y)+5, tickW, 3, accent)
}

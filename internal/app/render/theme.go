package render

import (
	"fmt"
	"image/color"
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Shared HUD palette. Surfaces ascend in opacity/depth:
// veil < log < primary; tints overlay primary for state.
var (
	surfacePrimary    = rl.NewColor(12, 16, 28, 222)
	surfaceInner      = rl.NewColor(22, 28, 44, 200)
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
)

const (
	cornerRadius      = float32(10)
	smallCornerRadius = float32(6)
	stripeWidth       = int32(3)
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
	minDim := float32(w)
	if float32(h) < minDim {
		minDim = float32(h)
	}
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
	textCol := textMuted
	if muted {
		fill = rl.NewColor(96, 84, 92, 230)
		textCol = textDim
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
	rl.DrawTextEx(font, label, rl.NewVector2(x+8, labelY+1), labelSize, 1, rl.NewColor(0, 0, 0, 180))
	rl.DrawTextEx(font, label, rl.NewVector2(x+8, labelY), labelSize, 1, fadeColor(textLabel, 1))

	valText := ""
	if maxValue > 0 {
		valText = formatBarValue(value, maxValue)
	}
	if valText != "" {
		valSize := labelSize
		valMeasure := rl.MeasureTextEx(font, valText, valSize, 1)
		valY := y + (float32(ih)-valMeasure.Y)/2 - 1
		valX := x + width - valMeasure.X - 8
		rl.DrawTextEx(font, valText, rl.NewVector2(valX+1, valY+1), valSize, 1, rl.NewColor(0, 0, 0, 180))
		rl.DrawTextEx(font, valText, rl.NewVector2(valX, valY), valSize, 1, textCol)
	}
}

func formatBarValue(value, maxValue int) string {
	return fmt.Sprintf("%d/%d", value, maxValue)
}

// drawHeading writes a small uppercase header inside a panel, with a colored
// underline tick to give it weight.
func drawHeading(font rl.Font, text string, x, y int32, accent color.RGBA) {
	size := float32(15)
	spacing := float32(1.6)
	pos := rl.NewVector2(float32(x), float32(y))
	rl.DrawTextEx(font, text, rl.NewVector2(pos.X+1, pos.Y+1), size, spacing, rl.NewColor(0, 0, 0, 200))
	rl.DrawTextEx(font, text, pos, size, spacing, textLabel)
	measure := rl.MeasureTextEx(font, text, size, spacing)
	tickW := int32(measure.X)
	if tickW < 22 {
		tickW = 22
	}
	rl.DrawRectangle(x, y+int32(measure.Y)+4, tickW, 2, accent)
}

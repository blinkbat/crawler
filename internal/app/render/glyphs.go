package render

import (
	"image/color"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// InputGlyph — controller button icons that replace spelled-out keys (gamepad-first). Procedural raylib primitives, no PNG assets.
type InputGlyph int

const (
	GlyphA          InputGlyph = iota // confirm        (A / Cross)
	GlyphB                            // back / cancel   (B / Circle)
	GlyphX                            // use            (Square / X face)
	GlyphY                            // panels / Tome   (Y / Triangle)
	GlyphLB                           // page tab back   (L1 / LB)
	GlyphRB                           // page tab fwd    (R1 / RB)
	GlyphStart                        // pause / apply   (Start / Options)
	GlyphSelect                       // quit / share    (Select / View)
	GlyphUp                           // d-pad up
	GlyphDown                         // d-pad down
	GlyphLeft                         // d-pad left
	GlyphRight                        // d-pad right
	GlyphUpDown                       // d-pad vertical pair
	GlyphLeftRight                    // d-pad horizontal pair
	GlyphLeftStick                    // left analog stick (Map pan)
	GlyphRightStick                   // right analog stick (Map zoom / free-look)
)

// HintSeg is one control affordance: button glyph(s) plus an action word. No glyphs = plain text; no label = icon-only.
type HintSeg struct {
	Glyphs []InputGlyph
	Label  string
}

// Hint constructs a HintSeg: Hint("Confirm", GlyphA).
func Hint(label string, glyphs ...InputGlyph) HintSeg {
	return HintSeg{Glyphs: glyphs, Label: label}
}

// confirmBackHints is the ubiquitous "Confirm / Back" footer, built once so the
// many steady-state surfaces that show it don't reallocate a hint bar per frame.
var confirmBackHints = []HintSeg{
	Hint("Confirm", GlyphA),
	Hint("Back", GlyphB),
}

// Hint-bar layout: glyphs in a seg sit nearly flush; label trails by glyphLabelGap; segs spaced by hintSegGap.
const (
	glyphInnerGap = float32(3)
	glyphLabelGap = float32(6)
	hintSegGap    = float32(18)
)

// Glyph aspect factors, shared by the measure path (glyphWidth) and the draw path
// (drawShoulderButton / drawStartSelect) so the two can't drift: bumper + system
// (start/select) advance widths as multiples of glyph height, and the pill height.
const (
	glyphBumperWFrac = float32(1.55)
	glyphSystemWFrac = float32(1.3)
	glyphPillHFrac   = float32(0.82)
)

// glyphBoxH is a glyph's drawn height, slightly taller than the text.
func glyphBoxH(size float32) float32 { return size * 1.4 }

// glyphWidth reports a glyph's advance width. Face/d-pad square; bumpers + start/select wider.
func glyphWidth(g InputGlyph, size float32) float32 {
	gh := glyphBoxH(size)
	switch g {
	case GlyphLB, GlyphRB:
		return gh * glyphBumperWFrac
	case GlyphStart, GlyphSelect:
		return gh * glyphSystemWFrac
	default:
		return gh
	}
}

func measureHintSeg(font rl.Font, seg HintSeg, size float32) float32 {
	w := float32(0)
	for i, g := range seg.Glyphs {
		if i > 0 {
			w += glyphInnerGap
		}
		w += glyphWidth(g, size)
	}
	if seg.Label != "" {
		if len(seg.Glyphs) > 0 {
			w += glyphLabelGap
		}
		w += hintLabelMeasureCache.measure(font, seg.Label, size, canonicalSpacing(size)).X
	}
	return w
}

// hintLabelMeasureCache memoizes hint-bar label widths; a centered DrawHintBar measures the bar twice/frame, so this avoids 2× cgo reshapes per label.
var hintLabelMeasureCache measureCache

func measureHintBar(font rl.Font, segs []HintSeg, size float32) float32 {
	w := float32(0)
	for i, s := range segs {
		if i > 0 {
			w += hintSegGap
		}
		w += measureHintSeg(font, s, size)
	}
	return w
}

// drawHintSegs paints segments left-anchored at x, returns the end x. Labels ride labelCol; whole run fades by alpha.
func drawHintSegs(font rl.Font, segs []HintSeg, x, y, size float32, labelCol color.RGBA, alpha float32) float32 {
	cur := x
	for i, s := range segs {
		if i > 0 {
			cur += hintSegGap
		}
		for j, g := range s.Glyphs {
			if j > 0 {
				cur += glyphInnerGap
			}
			cur += drawInputGlyph(font, g, cur, y, size, alpha)
		}
		if s.Label != "" {
			if len(s.Glyphs) > 0 {
				cur += glyphLabelGap
			}
			drawTextWithShadow(font, s.Label, cur, y, size, fadeColor(labelCol, alpha))
			cur += hintLabelMeasureCache.measure(font, s.Label, size, canonicalSpacing(size)).X
		}
	}
	return cur
}

// DrawHintBar centers a hint strip at cx with diamond termini. For centered modal footers and HUD hints.
func DrawHintBar(font rl.Font, segs []HintSeg, cx, y, size float32) {
	w := measureHintBar(font, segs, size)
	x := cx - w/2
	drawHintSegs(font, segs, x, y, size, textDim, 1)
	pipCol := fadeColor(textDim, 0.65)
	pipY := y + size/2
	drawDiamondPip(x-14, pipY, 1.8, pipCol)
	drawDiamondPip(x+w+14, pipY, 1.8, pipCol)
}

// DrawHintBarLeft paints the strip left-anchored at x (no termini).
func DrawHintBarLeft(font rl.Font, segs []HintSeg, x, y, size float32) {
	drawHintSegs(font, segs, x, y, size, textDim, 1)
}

// drawModalFooterGlyphs / ...Left paint a modal's footer hint bar, centred or left-anchored on the card.
func drawModalFooterGlyphs(font rl.Font, card rl.Rectangle, segs []HintSeg) {
	drawModalFooterGlyphsSized(font, card, segs, FontTiny)
}

// drawModalFooterGlyphsSized is the centred footer at a caller-chosen font size (the
// shop runs its footer at FontSmall); the baseline still lands via the shared
// footerBaselineY/uiFooterMargin math, not a hand-tuned per-surface offset.
func drawModalFooterGlyphsSized(font rl.Font, card rl.Rectangle, segs []HintSeg, size float32) {
	y := float32(footerBaselineY(int32(card.Y+card.Height), size))
	DrawHintBar(font, segs, card.X+card.Width/2, y, size)
}

func drawModalFooterGlyphsLeft(font rl.Font, card rl.Rectangle, x float32, segs []HintSeg) {
	y := float32(footerBaselineY(int32(card.Y+card.Height), FontSmall))
	DrawHintBarLeft(font, segs, x, y, FontSmall)
}

// glyphPromptScratch backs drawGlyphPrompt's single seg so the per-frame in-world
// prompt (drawn while adjacent to a chest/crystal/door) allocates nothing. Safe to
// reuse: each call fully measures + draws before returning, never retaining it.
var (
	glyphPromptSegScratch   = make([]HintSeg, 1)
	glyphPromptGlyphScratch = make([]InputGlyph, 1)
)

// drawGlyphPrompt centers a single "[glyph] Verb" in-world cue; brighter than a footer (borderActive, no termini).
func drawGlyphPrompt(font rl.Font, glyph InputGlyph, label string, cx, y, size float32) {
	glyphPromptGlyphScratch[0] = glyph
	glyphPromptSegScratch[0] = HintSeg{Glyphs: glyphPromptGlyphScratch, Label: label}
	w := measureHintBar(font, glyphPromptSegScratch, size)
	drawHintSegs(font, glyphPromptSegScratch, cx-w/2, y, size, borderActive, 1)
}

// drawInputGlyph paints one button icon (top-left at x,y) centered on the text line and returns its advance width.
func drawInputGlyph(font rl.Font, g InputGlyph, x, y, size, alpha float32) float32 {
	gh := glyphBoxH(size)
	cy := y + size/2
	switch g {
	case GlyphA:
		return drawFaceButton(font, "A", glyphAColor, x, cy, gh, alpha)
	case GlyphB:
		return drawFaceButton(font, "B", glyphBColor, x, cy, gh, alpha)
	case GlyphX:
		return drawFaceButton(font, "X", glyphXColor, x, cy, gh, alpha)
	case GlyphY:
		return drawFaceButton(font, "Y", glyphYColor, x, cy, gh, alpha)
	case GlyphLB:
		return drawShoulderButton(font, "LB", x, cy, gh, alpha)
	case GlyphRB:
		return drawShoulderButton(font, "RB", x, cy, gh, alpha)
	case GlyphStart:
		return drawStartSelect(true, x, cy, gh, alpha)
	case GlyphSelect:
		return drawStartSelect(false, x, cy, gh, alpha)
	case GlyphLeftStick:
		return drawStickGlyph(font, "L", x, cy, gh, alpha)
	case GlyphRightStick:
		return drawStickGlyph(font, "R", x, cy, gh, alpha)
	default:
		return drawDpadGlyph(g, x, cy, gh, alpha)
	}
}

// inputGlyphCoverage tags each glyph as explicitly handled (true) or routed through the d-pad default (false). The init() guard asserts full iota coverage so a new glyph can't silently fall through. Ledger only; no runtime effect.
var inputGlyphCoverage = map[InputGlyph]bool{
	GlyphA:          true,
	GlyphB:          true,
	GlyphX:          true,
	GlyphY:          true,
	GlyphLB:         true,
	GlyphRB:         true,
	GlyphStart:      true,
	GlyphSelect:     true,
	GlyphUp:         false, // d-pad default
	GlyphDown:       false, // d-pad default
	GlyphLeft:       false, // d-pad default
	GlyphRight:      false, // d-pad default
	GlyphUpDown:     false, // d-pad default
	GlyphLeftRight:  false, // d-pad default
	GlyphLeftStick:  true,
	GlyphRightStick: true,
}

// glyphCount is one past the last InputGlyph const, for the init coverage guard.
const glyphCount = GlyphRightStick + 1

func init() {
	if len(inputGlyphCoverage) != int(glyphCount) {
		panic("render: inputGlyphCoverage size does not match the InputGlyph const run — a glyph is unregistered")
	}
	for g := InputGlyph(0); g < glyphCount; g++ {
		if _, ok := inputGlyphCoverage[g]; !ok {
			panic("render: InputGlyph value missing from inputGlyphCoverage — register it (explicit case or d-pad default)")
		}
	}
}

// drawFaceButton: dark disc + colored ring + colored letter.
func drawFaceButton(font rl.Font, letter string, col color.RGBA, x, cy, gh, alpha float32) float32 {
	cx := x + gh/2
	r := gh/2 - 1
	rl.DrawCircle(int32(cx), int32(cy), r, fadeColor(glyphBody, alpha))
	rl.DrawCircleLines(int32(cx), int32(cy), r, fadeColor(col, alpha))
	rl.DrawCircleLines(int32(cx), int32(cy), r-1, fadeColor(col, alpha*0.65))
	drawGlyphLetter(font, letter, cx, cy, gh*0.62, fadeColor(col, alpha))
	return gh
}

// Glyph-pill roundness: glyphPillRoundness = softer bumper/system radius; glyphPadRoundness = tighter d-pad cap.
const (
	glyphPillRoundness = float32(0.6)
	glyphPadRoundness  = float32(0.35)
)

// drawGlyphPill paints the shared rounded body fill + 1px rim for shoulder/start-select/d-pad glyphs.
func drawGlyphPill(rect rl.Rectangle, roundness, alpha float32) {
	rl.DrawRectangleRounded(rect, roundness, 6, fadeColor(glyphBody, alpha))
	rl.DrawRectangleRoundedLinesEx(rect, roundness, 6, 1, fadeColor(glyphRim, alpha))
}

// drawShoulderButton is a bumper pill (wider than tall) with its LB/RB label.
func drawShoulderButton(font rl.Font, label string, x, cy, gh, alpha float32) float32 {
	w := gh * glyphBumperWFrac
	h := gh * glyphPillHFrac
	drawGlyphPill(rl.NewRectangle(x, cy-h/2, w, h), glyphPillRoundness, alpha)
	drawGlyphLetter(font, label, x+w/2, cy, gh*0.5, fadeColor(glyphInk, alpha))
	return w
}

// drawStartSelect: Start = three-line menu icon; Select = two overlapping view panes.
func drawStartSelect(start bool, x, cy, gh, alpha float32) float32 {
	w := gh * glyphSystemWFrac
	h := gh * glyphPillHFrac
	drawGlyphPill(rl.NewRectangle(x, cy-h/2, w, h), glyphPillRoundness, alpha)
	ink := fadeColor(glyphInk, alpha)
	cx := x + w/2
	if start {
		// Hamburger menu.
		lw := w * 0.42
		for i := -1; i <= 1; i++ {
			ly := cy + float32(i)*h*0.22
			rl.DrawLineEx(rl.NewVector2(cx-lw/2, ly), rl.NewVector2(cx+lw/2, ly), 1.5, ink)
		}
	} else {
		// Two overlapping panes.
		s := h * 0.30
		rl.DrawRectangleLinesEx(rl.NewRectangle(cx-s*0.9, cy-s*0.7, s*1.2, s*1.2), 1.4, ink)
		rl.DrawRectangleLinesEx(rl.NewRectangle(cx-s*0.2, cy-s*0.1, s*1.2, s*1.2), 1.4, ink)
	}
	return w
}

// drawStickGlyph draws an analog-stick icon: a rounded tile, a housing ring, a
// thumb disc, and the L/R label — distinguishing the two sticks in Map hints.
func drawStickGlyph(font rl.Font, label string, x, cy, gh, alpha float32) float32 {
	drawGlyphPill(rl.NewRectangle(x+1, cy-gh/2+1, gh-2, gh-2), glyphPadRoundness, alpha)
	cx := x + gh/2
	r := gh * 0.28
	rl.DrawCircleLines(int32(cx), int32(cy), r+2, fadeColor(glyphRim, alpha*0.8))
	rl.DrawCircle(int32(cx), int32(cy), r, fadeColor(glyphBody, alpha))
	rl.DrawCircleLines(int32(cx), int32(cy), r, fadeColor(giltBright, alpha))
	drawGlyphLetter(font, label, cx, cy, gh*0.42, fadeColor(giltBright, alpha))
	return gh
}

// drawDpadGlyph draws a d-pad tile with chevrons; requested direction(s) bright, the rest dim.
func drawDpadGlyph(g InputGlyph, x, cy, gh, alpha float32) float32 {
	cx := x + gh/2
	drawGlyphPill(rl.NewRectangle(x+1, cy-gh/2+1, gh-2, gh-2), glyphPadRoundness, alpha)

	up := g == GlyphUp || g == GlyphUpDown
	down := g == GlyphDown || g == GlyphUpDown
	left := g == GlyphLeft || g == GlyphLeftRight
	right := g == GlyphRight || g == GlyphLeftRight

	bright := fadeColor(giltBright, alpha)
	dim := fadeColor(glyphRim, alpha*0.7)
	cw := gh * 0.16 // chevron half-width
	off := gh * 0.30
	tip := gh * 0.40
	chevCol := func(on bool) color.RGBA {
		if on {
			return bright
		}
		return dim
	}
	// Up
	rl.DrawLineEx(rl.NewVector2(cx-cw, cy-off), rl.NewVector2(cx, cy-tip), 1.6, chevCol(up))
	rl.DrawLineEx(rl.NewVector2(cx, cy-tip), rl.NewVector2(cx+cw, cy-off), 1.6, chevCol(up))
	// Down
	rl.DrawLineEx(rl.NewVector2(cx-cw, cy+off), rl.NewVector2(cx, cy+tip), 1.6, chevCol(down))
	rl.DrawLineEx(rl.NewVector2(cx, cy+tip), rl.NewVector2(cx+cw, cy+off), 1.6, chevCol(down))
	// Left
	rl.DrawLineEx(rl.NewVector2(cx-off, cy-cw), rl.NewVector2(cx-tip, cy), 1.6, chevCol(left))
	rl.DrawLineEx(rl.NewVector2(cx-tip, cy), rl.NewVector2(cx-off, cy+cw), 1.6, chevCol(left))
	// Right
	rl.DrawLineEx(rl.NewVector2(cx+off, cy-cw), rl.NewVector2(cx+tip, cy), 1.6, chevCol(right))
	rl.DrawLineEx(rl.NewVector2(cx+tip, cy), rl.NewVector2(cx+off, cy+cw), 1.6, chevCol(right))
	return gh
}

// glyphLetterMeasureCache memoizes face-button letter widths to avoid a per-glyph per-frame cgo MeasureTextEx.
var glyphLetterMeasureCache measureCache

// drawGlyphLetter centers a glyph's letter/label on (cx, cy).
func drawGlyphLetter(font rl.Font, s string, cx, cy, size float32, col color.RGBA) {
	m := glyphLetterMeasureCache.measure(font, s, size, canonicalSpacing(size))
	drawTextWithShadow(font, s, cx-m.X/2, cy-m.Y/2, size, col)
}

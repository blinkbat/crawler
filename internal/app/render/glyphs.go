package render

import (
	"image/color"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Controller-input glyphs — the on-screen button ICONS that replace
// spelled-out key names everywhere in the UI. The game is gamepad-first
// (AGENTS.md / UI_STANDARDS.md), so every hint reads as a controller prompt:
// a colored face button, a bumper pill, a start/select pictogram, or a d-pad
// direction. We draw the CONTROLLER set only for now; a later pass can switch
// to keyboard glyphs by device.
//
// All hint surfaces build a []HintSeg (a glyph or two plus an action word) and
// hand it to DrawHintBar / DrawHintBarLeft (or the modal-footer wrappers). The
// glyphs are procedural raylib primitives in the library palette — see the
// glyph* tokens in theme.go — so there are no PNG assets to load or free.
type InputGlyph int

const (
	GlyphA         InputGlyph = iota // confirm        (A / Cross)
	GlyphB                           // back / cancel   (B / Circle)
	GlyphX                           // use            (Square / X face)
	GlyphY                           // panels / Tome   (Y / Triangle)
	GlyphLB                          // page tab back   (L1 / LB)
	GlyphRB                          // page tab fwd    (R1 / RB)
	GlyphStart                       // pause / apply   (Start / Options)
	GlyphSelect                      // quit / share    (Select / View)
	GlyphUp                          // d-pad up
	GlyphDown                        // d-pad down
	GlyphLeft                        // d-pad left
	GlyphRight                       // d-pad right
	GlyphUpDown                      // d-pad vertical pair
	GlyphLeftRight                   // d-pad horizontal pair
)

// HintSeg is one control affordance: the button glyph(s) plus the action word
// they perform (e.g. {GlyphA} + "Confirm", or {GlyphLB, GlyphRB} + "Tabs").
// A seg with no glyphs is plain text (used for the Map footer's area/zoom
// preamble); a seg with no label is icon-only.
type HintSeg struct {
	Glyphs []InputGlyph
	Label  string
}

// Hint is the terse constructor used at every call site: Hint("Confirm", GlyphA).
func Hint(label string, glyphs ...InputGlyph) HintSeg {
	return HintSeg{Glyphs: glyphs, Label: label}
}

// Hint-bar layout tunables. Glyphs in one seg ([LB][RB]) sit nearly flush; the
// label trails its glyph(s) by glyphLabelGap; segments are spaced hintSegGap.
const (
	glyphInnerGap = float32(3)
	glyphLabelGap = float32(6)
	hintSegGap    = float32(18)
)

// glyphBoxH is a glyph's drawn height for a given hint font size — a touch
// taller than the text so the icon reads as a raised button beside the word.
func glyphBoxH(size float32) float32 { return size * 1.4 }

// glyphWidth reports a single glyph's advance width. Face buttons + d-pad are
// square; bumpers and start/select are wider to fit their pictograms.
func glyphWidth(g InputGlyph, size float32) float32 {
	gh := glyphBoxH(size)
	switch g {
	case GlyphLB, GlyphRB:
		return gh * 1.55
	case GlyphStart, GlyphSelect:
		return gh * 1.3
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

// hintLabelMeasureCache memoizes the hint-bar label widths. The label set is
// a tiny fixed vocabulary ("Confirm"/"Back"/"Continue"/…) re-measured every
// frame a footer is up — and a CENTERED DrawHintBar measures the whole bar
// twice (once to center, once while drawing), so without the cache each footer
// label is reshaped 2× per frame via cgo.
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

// drawHintSegs paints the segments left-anchored at x and returns the end x.
// Glyphs keep their full button color (the icon is the point); the label rides
// labelCol, and the whole run fades by alpha so callers can dim/animate it.
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

// DrawHintBar centers a controller-hint strip at screen-x cx, with the same
// diamond termini DrawFooterHint stitches so the glyph footers read as part of
// the panel-ornament language. Use for centered modal footers and HUD hints.
func DrawHintBar(font rl.Font, segs []HintSeg, cx, y, size float32) {
	w := measureHintBar(font, segs, size)
	x := cx - w/2
	drawHintSegs(font, segs, x, y, size, textHint, 1)
	pipCol := fadeColor(textHint, 0.65)
	pipY := y + size/2
	drawDiamondPip(x-14, pipY, 1.8, pipCol)
	drawDiamondPip(x+w+14, pipY, 1.8, pipCol)
}

// DrawHintBarLeft paints the strip left-anchored at x (no termini) — the
// left-aligned mirror used by the picker sub-modals and the skill-tree footer.
func DrawHintBarLeft(font rl.Font, segs []HintSeg, x, y, size float32) {
	drawHintSegs(font, segs, x, y, size, textHint, 1)
}

// drawModalFooterGlyphs / ...Left are the glyph mirrors of drawModalFooter /
// drawModalFooterLeft, so a modal swaps its hint string for a []HintSeg with a
// one-line change and the card-geometry math stays in one place.
func drawModalFooterGlyphs(font rl.Font, card rl.Rectangle, segs []HintSeg) {
	DrawHintBar(font, segs, card.X+card.Width/2, card.Y+card.Height-modalFooterTextOffset, FontTiny)
}

func drawModalFooterGlyphsLeft(font rl.Font, card rl.Rectangle, x float32, segs []HintSeg) {
	DrawHintBarLeft(font, segs, x, card.Y+card.Height-pickerFooterTextOffset, FontSmall)
}

// drawGlyphPrompt centers a single "[glyph] Verb" cue (the in-world chest /
// crystal / door affordances). Brighter than a footer (borderActive label, no
// termini) so it reads as an actionable call-to-press over the 3D scene.
func drawGlyphPrompt(font rl.Font, glyph InputGlyph, label string, cx, y, size float32) {
	segs := []HintSeg{Hint(label, glyph)}
	w := measureHintBar(font, segs, size)
	drawHintSegs(font, segs, cx-w/2, y, size, borderActive, 1)
}

// drawInputGlyph paints one button icon with its top-left at (x, y), vertically
// centered on the text line (height ~size), and returns its advance width.
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
	default:
		return drawDpadGlyph(g, x, cy, gh, alpha)
	}
}

// inputGlyphCoverage is the maintained set of every InputGlyph value, tagging
// each as either explicitly handled by drawInputGlyph's switch (true) or
// intentionally routed through its d-pad default (false). The init() guard
// below asserts this set covers the whole iota run [GlyphA, glyphCount), so a
// newly-added InputGlyph can't silently fall into the d-pad default — adding a
// const without registering it here panics at startup. This is a coverage
// ledger only; it does not affect runtime drawing.
var inputGlyphCoverage = map[InputGlyph]bool{
	GlyphA:         true,
	GlyphB:         true,
	GlyphX:         true,
	GlyphY:         true,
	GlyphLB:        true,
	GlyphRB:        true,
	GlyphStart:     true,
	GlyphSelect:    true,
	GlyphUp:        false, // d-pad default
	GlyphDown:      false, // d-pad default
	GlyphLeft:      false, // d-pad default
	GlyphRight:     false, // d-pad default
	GlyphUpDown:    false, // d-pad default
	GlyphLeftRight: false, // d-pad default
}

// glyphCount is one past the last InputGlyph const; kept adjacent to the const
// block's tail (GlyphLeftRight) so the init guard can walk the full iota run.
const glyphCount = GlyphLeftRight + 1

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

// drawFaceButton is the dark-disc / colored-letter style: a raised dark circle,
// a colored ring, and the letter in the button's signature hue.
func drawFaceButton(font rl.Font, letter string, col color.RGBA, x, cy, gh, alpha float32) float32 {
	cx := x + gh/2
	r := gh/2 - 1
	rl.DrawCircle(int32(cx), int32(cy), r, fadeColor(glyphBody, alpha))
	rl.DrawCircleLines(int32(cx), int32(cy), r, fadeColor(col, alpha))
	rl.DrawCircleLines(int32(cx), int32(cy), r-1, fadeColor(col, alpha*0.65))
	drawGlyphLetter(font, letter, cx, cy, gh*0.62, fadeColor(col, alpha))
	return gh
}

// Glyph-pill roundness tokens — the only thing that differs between the
// non-face glyph buttons that share drawGlyphPill. glyphPillRoundness is the
// softer bumper/system-button radius (shoulders, start/select); glyphPadRoundness
// is the tighter d-pad cap. Named so the two callers of each can't drift.
const (
	glyphPillRoundness = float32(0.6)
	glyphPadRoundness  = float32(0.35)
)

// drawGlyphPill paints the shared controller-glyph backing: a rounded body fill
// + a 1px rim, both faded by alpha. The shoulder, start/select, and d-pad
// glyphs all sit on this same body/rim pairing (only the roundness differs), so
// the token pair lives here instead of being re-typed per glyph.
func drawGlyphPill(rect rl.Rectangle, roundness, alpha float32) {
	rl.DrawRectangleRounded(rect, roundness, 6, fadeColor(glyphBody, alpha))
	rl.DrawRectangleRoundedLinesEx(rect, roundness, 6, 1, fadeColor(glyphRim, alpha))
}

// drawShoulderButton is a bumper pill (wider than tall) with its LB/RB label.
func drawShoulderButton(font rl.Font, label string, x, cy, gh, alpha float32) float32 {
	w := gh * 1.55
	h := gh * 0.82
	drawGlyphPill(rl.NewRectangle(x, cy-h/2, w, h), glyphPillRoundness, alpha)
	drawGlyphLetter(font, label, x+w/2, cy, gh*0.5, fadeColor(glyphInk, alpha))
	return w
}

// drawStartSelect draws the two system-button pictograms inside a rounded pill:
// Start = the three-line "menu" icon; Select = two overlapping "view" panes.
func drawStartSelect(start bool, x, cy, gh, alpha float32) float32 {
	w := gh * 1.3
	h := gh * 0.82
	drawGlyphPill(rl.NewRectangle(x, cy-h/2, w, h), glyphPillRoundness, alpha)
	ink := fadeColor(glyphInk, alpha)
	cx := x + w/2
	if start {
		// Three stacked horizontal lines (hamburger / "menu").
		lw := w * 0.42
		for i := -1; i <= 1; i++ {
			ly := cy + float32(i)*h*0.22
			rl.DrawLineEx(rl.NewVector2(cx-lw/2, ly), rl.NewVector2(cx+lw/2, ly), 1.5, ink)
		}
	} else {
		// Two overlapping panes ("view" / share).
		s := h * 0.30
		rl.DrawRectangleLinesEx(rl.NewRectangle(cx-s*0.9, cy-s*0.7, s*1.2, s*1.2), 1.4, ink)
		rl.DrawRectangleLinesEx(rl.NewRectangle(cx-s*0.2, cy-s*0.1, s*1.2, s*1.2), 1.4, ink)
	}
	return w
}

// drawDpadGlyph draws a rounded d-pad tile with chevrons on each arm — the
// requested direction(s) bright (giltBright), the rest dim so the cross still
// reads as a d-pad.
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
	// Up chevron (apex toward top).
	rl.DrawLineEx(rl.NewVector2(cx-cw, cy-off), rl.NewVector2(cx, cy-tip), 1.6, chevCol(up))
	rl.DrawLineEx(rl.NewVector2(cx, cy-tip), rl.NewVector2(cx+cw, cy-off), 1.6, chevCol(up))
	// Down chevron.
	rl.DrawLineEx(rl.NewVector2(cx-cw, cy+off), rl.NewVector2(cx, cy+tip), 1.6, chevCol(down))
	rl.DrawLineEx(rl.NewVector2(cx, cy+tip), rl.NewVector2(cx+cw, cy+off), 1.6, chevCol(down))
	// Left chevron.
	rl.DrawLineEx(rl.NewVector2(cx-off, cy-cw), rl.NewVector2(cx-tip, cy), 1.6, chevCol(left))
	rl.DrawLineEx(rl.NewVector2(cx-tip, cy), rl.NewVector2(cx-off, cy+cw), 1.6, chevCol(left))
	// Right chevron.
	rl.DrawLineEx(rl.NewVector2(cx+off, cy-cw), rl.NewVector2(cx+tip, cy), 1.6, chevCol(right))
	rl.DrawLineEx(rl.NewVector2(cx+tip, cy), rl.NewVector2(cx+off, cy+cw), 1.6, chevCol(right))
	return gh
}

// glyphLetterMeasureCache memoizes the face-button letter widths ("A"/"B"/
// "LB"/"RB"/…). drawGlyphLetter centers each letter per glyph per hint-bar
// draw — the battle action footer draws every battle frame, in-world prompts
// every explore frame near a chest/door — so the raw MeasureTextEx was a cgo
// round-trip per glyph per frame. The letter set is tiny and constant.
var glyphLetterMeasureCache measureCache

// drawGlyphLetter centers a glyph's letter/label on (cx, cy).
func drawGlyphLetter(font rl.Font, s string, cx, cy, size float32, col color.RGBA) {
	m := glyphLetterMeasureCache.measure(font, s, size, canonicalSpacing(size))
	drawTextWithShadow(font, s, cx-m.X/2, cy-m.Y/2, size, col)
}

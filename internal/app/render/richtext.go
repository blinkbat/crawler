package render

import (
	"image/color"
	"unicode/utf8"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Procedural symbol glyphs. The UI font (Della Respira) lacks the geometric/
// arrow/relational symbols the HUD prints (▶ → ↑ ↓ ● ★ ✓ ≤ ≥) and a few marks it
// drops (… ’ ≈), so we draw them as vector shapes here rather than bundle a
// second font. drawRichText / measureRichText splice these into text runs, so
// every label that mixes text + symbol renders with no per-call-site change.
// Extending coverage is one map row + one drawer.

// symGlyph is one procedurally-drawn symbol: adv is its advance width as a
// fraction of font size; draw paints it within the box glyphBoxFor computes.
type symGlyph struct {
	adv  float32
	draw func(b glyphBox, col color.RGBA)
}

// glyphBox is the inline drawing region for one symbol, sized to the text.
// top/bot bracket it vertically around the text's visual center; left/right
// horizontally within its advance; t is a size-scaled stroke thickness (floored).
type glyphBox struct {
	left, right, top, bot, cx, cy, t float32
}

func glyphBoxFor(x, y, size, adv float32) glyphBox {
	w := adv * size
	padX := w * 0.12
	left := x + padX
	right := x + w - padX
	// Center on the text's visual middle (~0.50·size below DrawTextEx's top-left origin).
	cy := y + size*0.50
	hh := size * 0.30
	// Stroke matches text weight: heavier under faux-bold, floored at tiny sizes.
	strokeFrac := float32(0.085)
	if fauxBoldEnabled {
		strokeFrac = 0.105
	}
	t := size * strokeFrac
	if t < 1.6 {
		t = 1.6
	}
	return glyphBox{left: left, right: right, top: cy - hh, bot: cy + hh, cx: (left + right) / 2, cy: cy, t: t}
}

// triRadius is the in-box triangle radius — the smaller half-extent, so a tight
// advance (▸) reads smaller than a wide one (▶) without separate tuning.
func (b glyphBox) triRadius() float32 {
	rx := (b.right - b.left) / 2
	ry := (b.bot - b.top) / 2
	if rx < ry {
		return rx
	}
	return ry
}

// symGlyphs maps each font-absent rune to its advance + drawer. Rotations are in
// raylib's y-down convention: 0°=right, 90°=down, 180°=left, -90°=up.
var symGlyphs = map[rune]symGlyph{
	'→': {1.00, glyphArrowRight},
	'←': {1.00, glyphArrowLeft},
	'↑': {0.70, glyphArrowUp},
	'↓': {0.70, glyphArrowDown},
	'↔': {1.10, glyphArrowLeftRight},
	'▶': {0.66, glyphTriRight},
	'▸': {0.52, glyphTriRight},
	'◂': {0.52, glyphTriLeft},
	'▲': {0.74, glyphTriUp},
	'▼': {0.74, glyphTriDown},
	'●': {0.62, glyphBullet},
	'★': {0.88, glyphStar},
	'✓': {0.74, glyphCheck},
	'∈': {0.66, glyphElementOf},
	'≤': {0.82, glyphLessEqual},
	'≥': {0.82, glyphGreaterEqual},
	'…': {1.10, glyphEllipsis},
	'’': {0.30, glyphApostrophe},
	'≈': {0.74, glyphApprox},
}

// fauxBoldEnabled gates the synthetic semibold (drawBoldText). Off by default;
// the font has no bold file, so bold is faked by over-stamping. Callers flip it
// via SetFauxBold. Package-level is fine — UI text draws are single-threaded.
var fauxBoldEnabled = false

// SetFauxBold toggles the synthetic semibold applied to all UI text.
func SetFauxBold(on bool) { fauxBoldEnabled = on }

// FauxBoldEnabled reports the current faux-bold setting.
func FauxBoldEnabled() bool { return fauxBoldEnabled }

// boldStampOffset is the rightward over-stamp that fakes semibold. Scales with
// size (floored so small text still thickens).
func boldStampOffset(size float32) float32 {
	if o := size * 0.03; o > 0.5 {
		return o
	}
	return 0.5
}

// drawBoldText draws a text run, faking semibold (a second stamp nudged right)
// only when fauxBoldEnabled. The advance is unchanged, so layout stays exact.
// All drawRichText font runs route here, so this one toggle governs UI weight.
func drawBoldText(font rl.Font, text string, x, y, size, spacing float32, col color.RGBA) {
	rl.DrawTextEx(font, text, rl.NewVector2(x, y), size, spacing, col)
	if fauxBoldEnabled {
		rl.DrawTextEx(font, text, rl.NewVector2(x+boldStampOffset(size), y), size, spacing, col)
	}
}

// containsSymGlyph reports whether text has any procedural symbol. Runs on every
// label draw, so the common case is cheap: every symGlyph rune is non-ASCII, so a
// byte scan rejects the ASCII majority; only a high byte pays the rune+map check.
func containsSymGlyph(text string) bool {
	for i := 0; i < len(text); i++ {
		if text[i] >= 0x80 {
			return hasSymGlyphRune(text[i:])
		}
	}
	return false
}

func hasSymGlyphRune(text string) bool {
	for _, r := range text {
		if _, ok := symGlyphs[r]; ok {
			return true
		}
	}
	return false
}

// DrawRichText is the exported, symbol-aware drop-in for rl.DrawTextEx, for
// packages outside render (the editor, which otherwise renders symbols as
// missing-glyph boxes). Same signature so the swap is mechanical.
func DrawRichText(font rl.Font, text string, position rl.Vector2, fontSize, spacing float32, tint color.RGBA) {
	drawRichText(font, text, position.X, position.Y, fontSize, spacing, tint)
}

// MeasureRichText is the exported, symbol-aware drop-in for rl.MeasureTextEx,
// pairing with DrawRichText so editor layout measures symbol-bearing labels to
// their real drawn width.
func MeasureRichText(font rl.Font, text string, fontSize, spacing float32) rl.Vector2 {
	return measureRichText(font, text, fontSize, spacing)
}

// walkRichText splits text into ordered segments (font-glyph runs + the symbols
// between them), invoking onRun/onSym in order. The single splice model behind
// drawRichText and measureRichText so they can't disagree on the boundaries.
func walkRichText(text string, onRun func(seg string), onSym func(g symGlyph)) {
	runStart := 0
	emit := func(end int) {
		if end > runStart {
			onRun(text[runStart:end])
		}
	}
	for i := 0; i < len(text); {
		r, w := utf8.DecodeRuneInString(text[i:])
		if g, ok := symGlyphs[r]; ok {
			emit(i)
			onSym(g)
			i += w
			runStart = i
			continue
		}
		i += w
	}
	emit(len(text))
}

// drawRichText draws text interleaving font glyphs and symbols, laying segments
// left-to-right with `spacing` between. Symbol-free strings take the fast path.
func drawRichText(font rl.Font, text string, x, y, size, spacing float32, col color.RGBA) {
	drawRichTextKnown(font, text, containsSymGlyph(text), x, y, size, spacing, col)
}

// richRunMeasureCache memoizes the per-run width the symbol path uses to advance
// the cursor, so a symbol-bearing label doesn't re-shape its runs every frame.
var richRunMeasureCache measureCache

// drawRichTextKnown is drawRichText with the has-symbol bit decided, so a caller
// drawing the SAME string several times (shadow + main pass) pays containsSymGlyph once.
func drawRichTextKnown(font rl.Font, text string, hasSym bool, x, y, size, spacing float32, col color.RGBA) {
	if !hasSym {
		drawBoldText(font, text, x, y, size, spacing, col)
		return
	}
	cx := x
	first := true
	gap := func() {
		if !first {
			cx += spacing
		}
		first = false
	}
	walkRichText(text,
		func(seg string) {
			gap()
			drawBoldText(font, seg, cx, y, size, spacing, col)
			cx += richRunMeasureCache.measure(font, seg, size, spacing).X
		},
		func(g symGlyph) {
			gap()
			g.draw(glyphBoxFor(cx, y, size, g.adv), col)
			cx += g.adv * size
		})
}

// measureRichText is drawRichText's measure twin (same segment+gap model).
func measureRichText(font rl.Font, text string, size, spacing float32) rl.Vector2 {
	if !containsSymGlyph(text) {
		return rl.MeasureTextEx(font, text, size, spacing)
	}
	width := float32(0)
	first := true
	gap := func() {
		if !first {
			width += spacing
		}
		first = false
	}
	walkRichText(text,
		func(seg string) {
			gap()
			width += richRunMeasureCache.measure(font, seg, size, spacing).X
		},
		func(g symGlyph) {
			gap()
			width += g.adv * size
		})
	// Height is the font's reported single-line height (not raw `size`), matching
	// the fast path so vertical-centering callers don't shift symbol-bearing labels.
	return rl.NewVector2(width, rl.MeasureTextEx(font, "X", size, spacing).Y)
}

// --- the drawers ----------------------------------------------------------

func glyphTriRight(b glyphBox, col color.RGBA) {
	rl.DrawPoly(rl.NewVector2(b.cx, b.cy), 3, b.triRadius(), 0, col)
}
func glyphTriLeft(b glyphBox, col color.RGBA) {
	rl.DrawPoly(rl.NewVector2(b.cx, b.cy), 3, b.triRadius(), 180, col)
}
func glyphTriUp(b glyphBox, col color.RGBA) {
	rl.DrawPoly(rl.NewVector2(b.cx, b.cy), 3, b.triRadius(), -90, col)
}
func glyphTriDown(b glyphBox, col color.RGBA) {
	rl.DrawPoly(rl.NewVector2(b.cx, b.cy), 3, b.triRadius(), 90, col)
}

func glyphBullet(b glyphBox, col color.RGBA) {
	rl.DrawCircleV(rl.NewVector2(b.cx, b.cy), b.triRadius()*0.92, col)
}

// Arrow shaft/head placement as fractions of the head radius, shared by every
// line-arrow drawer: shaft stops at arrowShaftStopFrac, head centered at arrowHeadCenterFrac.
const (
	arrowShaftStopFrac  = float32(0.7)
	arrowHeadCenterFrac = float32(0.55)
)

// arrowHead draws a filled triangle at (hx,hy) pointing along rotation; shared
// by the line arrows.
func arrowHead(hx, hy, r, rotation float32, col color.RGBA) {
	rl.DrawPoly(rl.NewVector2(hx, hy), 3, r, rotation, col)
}

func glyphArrowRight(b glyphBox, col color.RGBA) {
	hr := (b.bot - b.top) * 0.5
	rl.DrawLineEx(rl.NewVector2(b.left, b.cy), rl.NewVector2(b.right-hr*arrowShaftStopFrac, b.cy), b.t, col)
	arrowHead(b.right-hr*arrowHeadCenterFrac, b.cy, hr, 0, col)
}
func glyphArrowLeft(b glyphBox, col color.RGBA) {
	hr := (b.bot - b.top) * 0.5
	rl.DrawLineEx(rl.NewVector2(b.right, b.cy), rl.NewVector2(b.left+hr*arrowShaftStopFrac, b.cy), b.t, col)
	arrowHead(b.left+hr*arrowHeadCenterFrac, b.cy, hr, 180, col)
}
func glyphArrowUp(b glyphBox, col color.RGBA) {
	hr := (b.right - b.left) * 0.5
	rl.DrawLineEx(rl.NewVector2(b.cx, b.bot), rl.NewVector2(b.cx, b.top+hr*arrowShaftStopFrac), b.t, col)
	arrowHead(b.cx, b.top+hr*arrowHeadCenterFrac, hr, -90, col)
}
func glyphArrowDown(b glyphBox, col color.RGBA) {
	hr := (b.right - b.left) * 0.5
	rl.DrawLineEx(rl.NewVector2(b.cx, b.top), rl.NewVector2(b.cx, b.bot-hr*arrowShaftStopFrac), b.t, col)
	arrowHead(b.cx, b.bot-hr*arrowHeadCenterFrac, hr, 90, col)
}
func glyphArrowLeftRight(b glyphBox, col color.RGBA) {
	hr := (b.bot - b.top) * 0.5
	rl.DrawLineEx(rl.NewVector2(b.left+hr*arrowHeadCenterFrac, b.cy), rl.NewVector2(b.right-hr*arrowHeadCenterFrac, b.cy), b.t, col)
	arrowHead(b.right-hr*arrowHeadCenterFrac, b.cy, hr, 0, col)
	arrowHead(b.left+hr*arrowHeadCenterFrac, b.cy, hr, 180, col)
}

func glyphCheck(b glyphBox, col color.RGBA) {
	// Short down-stroke into a long up-stroke.
	knee := rl.NewVector2(b.left+(b.right-b.left)*0.36, b.bot)
	rl.DrawLineEx(rl.NewVector2(b.left, b.cy+(b.bot-b.cy)*0.1), knee, b.t, col)
	rl.DrawLineEx(knee, rl.NewVector2(b.right, b.top), b.t, col)
}

// glyphStarFanBuf backs glyphStar's triangle-fan vertices (centre + 10 points +
// closing repeat). Reused so the ★ glyph doesn't allocate per draw.
var glyphStarFanBuf = make([]rl.Vector2, 0, 12)

func glyphStar(b glyphBox, col color.RGBA) {
	// Five-point star as a triangle fan over alternating radii (shared starVerts).
	cx, cy := b.cx, b.cy
	outer := b.triRadius()
	verts := starVerts(cx, cy, outer, outer*0.42, 5)
	pts := glyphStarFanBuf[:0]
	pts = append(pts, rl.NewVector2(cx, cy))
	pts = append(pts, verts...)
	pts = append(pts, verts[0]) // close the fan back to the first point
	glyphStarFanBuf = pts
	rl.DrawTriangleFan(pts, col)
}

func glyphElementOf(b glyphBox, col color.RGBA) {
	// Epsilon-ish: left vertical bar with top / middle / bottom arms.
	rl.DrawLineEx(rl.NewVector2(b.left, b.top), rl.NewVector2(b.left, b.bot), b.t, col)
	rl.DrawLineEx(rl.NewVector2(b.left, b.top), rl.NewVector2(b.right, b.top), b.t, col)
	rl.DrawLineEx(rl.NewVector2(b.left, b.cy), rl.NewVector2(b.right-(b.right-b.left)*0.25, b.cy), b.t, col)
	rl.DrawLineEx(rl.NewVector2(b.left, b.bot), rl.NewVector2(b.right, b.bot), b.t, col)
}

func glyphLessEqual(b glyphBox, col color.RGBA) {
	midY := b.cy - (b.cy-b.top)*0.35
	rl.DrawLineEx(rl.NewVector2(b.right, b.top), rl.NewVector2(b.left, midY), b.t, col)
	rl.DrawLineEx(rl.NewVector2(b.left, midY), rl.NewVector2(b.right, b.cy+(b.bot-b.cy)*0.15), b.t, col)
	rl.DrawLineEx(rl.NewVector2(b.left, b.bot), rl.NewVector2(b.right, b.bot), b.t, col)
}
func glyphGreaterEqual(b glyphBox, col color.RGBA) {
	midY := b.cy - (b.cy-b.top)*0.35
	rl.DrawLineEx(rl.NewVector2(b.left, b.top), rl.NewVector2(b.right, midY), b.t, col)
	rl.DrawLineEx(rl.NewVector2(b.right, midY), rl.NewVector2(b.left, b.cy+(b.bot-b.cy)*0.15), b.t, col)
	rl.DrawLineEx(rl.NewVector2(b.left, b.bot), rl.NewVector2(b.right, b.bot), b.t, col)
}

// glyphEllipsis draws three baseline dots (…), which Della Respira drops.
func glyphEllipsis(b glyphBox, col color.RGBA) {
	r := b.t * 0.7
	// Cap r at width/6 so three dots + gaps always fit (no overlap/spill); floor
	// so they never vanish at tiny sizes.
	if maxR := (b.right - b.left) / 6; r > maxR {
		r = maxR
	}
	if r < 1.2 {
		r = 1.2
	}
	dy := b.bot - r // sit low, like a baseline ellipsis
	xL, xR := b.left+r, b.right-r
	rl.DrawCircleV(rl.NewVector2(xL, dy), r, col)
	rl.DrawCircleV(rl.NewVector2((xL+xR)/2, dy), r, col)
	rl.DrawCircleV(rl.NewVector2(xR, dy), r, col)
}

// glyphApostrophe draws the right single quote (’) the font omits.
func glyphApostrophe(b glyphBox, col color.RGBA) {
	x := (b.left + b.right) / 2
	h := (b.bot - b.top) * 0.34
	rl.DrawLineEx(rl.NewVector2(x+b.t*0.25, b.top), rl.NewVector2(x-b.t*0.15, b.top+h), b.t, col)
}

// glyphApprox draws the two strokes of ≈, which the font also drops.
func glyphApprox(b glyphBox, col color.RGBA) {
	dy := (b.bot - b.top) * 0.16
	rl.DrawLineEx(rl.NewVector2(b.left, b.cy-dy), rl.NewVector2(b.right, b.cy-dy), b.t, col)
	rl.DrawLineEx(rl.NewVector2(b.left, b.cy+dy), rl.NewVector2(b.right, b.cy+dy), b.t, col)
}

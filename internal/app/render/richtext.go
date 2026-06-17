package render

import (
	"image/color"
	"math"
	"unicode/utf8"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Procedural symbol glyphs. The UI font (embedded Della Respira, see
// resources.go) carries Latin + most punctuation but NOT the geometric/
// arrow/relational symbols the HUD prints (▶ → ↑ ↓ ● ★ ✓ ≤ ≥) nor a few
// typographic marks it happens to drop (… ’ ≈).
// Rather than bundle a second "symbol" font, we DRAW those glyphs as crisp
// vector shapes here, so the game ships exactly one font and we keep full
// control over how each symbol looks (matching the gamepad-glyph approach in
// glyphs.go). drawRichText / measureRichText splice these into normal text
// runs; the central text helpers (drawTextWithShadowStyle, the measure
// caches) route through them, so every HUD/battle/menu label that mixes text
// and a symbol renders correctly with no per-call-site change.
//
// Extending coverage is one map row + one drawer — the path to "draw every
// symbol procedurally." Editor-only surfaces draw via rl.DrawTextEx directly
// and don't pass through here yet; routing them is a later step.

// symGlyph is one procedurally-drawn symbol: adv is its advance width as a
// fraction of the font size (so it scales with the text it sits in); draw
// paints it within the box glyphBoxFor computes.
type symGlyph struct {
	adv  float32
	draw func(b glyphBox, col color.RGBA)
}

// glyphBox is the inline drawing region for one symbol, sized to the
// surrounding text. top/bot bracket the glyph vertically around the text's
// visual center; left/right bracket it horizontally within its advance; t is
// a stroke thickness scaled to the size (floored so it never vanishes).
type glyphBox struct {
	left, right, top, bot, cx, cy, t float32
}

func glyphBoxFor(x, y, size, adv float32) glyphBox {
	w := adv * size
	padX := w * 0.12
	left := x + padX
	right := x + w - padX
	// Center on the text's visual middle (~0.50·size below the top-left
	// origin rl.DrawTextEx draws from), with a cap-ish half-height.
	cy := y + size*0.50
	hh := size * 0.30
	// Stroke thickness matches the text weight: heavier when the optional
	// faux-bold is on (so symbols don't read thin beside emboldened text),
	// normal otherwise. Floored so it never vanishes at tiny sizes.
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

// triRadius is the in-box radius for the equilateral-triangle indicators —
// the smaller of the box's half-extents so a tight advance (▸) reads smaller
// than a wide one (▶) without separate tuning.
func (b glyphBox) triRadius() float32 {
	rx := (b.right - b.left) / 2
	ry := (b.bot - b.top) / 2
	if rx < ry {
		return rx
	}
	return ry
}

// symGlyphs maps each font-absent rune to its advance + drawer. DrawPoly /
// DrawLineEx / DrawCircleV are used throughout so winding never matters and
// the shapes stay crisp (vector) at any size. Rotations are in raylib's
// y-down convention: 0°=right, 90°=down, 180°=left, 270°/-90°=up.
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

// fauxBoldEnabled gates the synthetic semibold (drawBoldText). OFF by
// default — the UI renders Della Respira at its true regular weight. The
// font has no bold file, so "bold" can only be faked by over-stamping;
// callers flip it on via SetFauxBold (e.g. wired to a display option).
// Package-level is fine: UI text draws are single-threaded on the raylib loop.
var fauxBoldEnabled = false

// SetFauxBold toggles the synthetic semibold weight applied to ALL UI text.
// Off by default. Exposed so an Options toggle / persisted setting can drive
// it without the render internals leaking out.
func SetFauxBold(on bool) { fauxBoldEnabled = on }

// FauxBoldEnabled reports the current faux-bold setting.
func FauxBoldEnabled() bool { return fauxBoldEnabled }

// boldStampOffset is the sub-pixel rightward over-stamp that fakes a semibold
// weight from the regular-only UI font. Scales with size — floored so small
// text still thickens — so body copy and titles read equally heavier.
func boldStampOffset(size float32) float32 {
	if o := size * 0.03; o > 0.5 {
		return o
	}
	return 0.5
}

// drawBoldText draws a text run, optionally faking semibold by stamping it a
// second time nudged boldStampOffset to the right — but ONLY when
// fauxBoldEnabled. The glyph ADVANCE is unchanged either way, so
// measurement/layout stay exact; the extra stamp just thickens stems. All
// font runs in drawRichText route through here, so this one toggle governs
// the whole UI's weight.
func drawBoldText(font rl.Font, text string, x, y, size, spacing float32, col color.RGBA) {
	rl.DrawTextEx(font, text, rl.NewVector2(x, y), size, spacing, col)
	if fauxBoldEnabled {
		rl.DrawTextEx(font, text, rl.NewVector2(x+boldStampOffset(size), y), size, spacing, col)
	}
}

// containsSymGlyph reports whether text has any procedurally-drawn symbol.
// This runs on EVERY label draw (the drawRichText fast-path gate), so the
// symbol-free common case must be cheap: every symGlyph rune is non-ASCII, so
// a string with no high byte can't contain one — a tight byte scan (no UTF-8
// decode, no map probe) rejects the ASCII majority. Only a string that
// actually has a non-ASCII byte pays the precise rune+map check, and only from
// that byte onward.
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

// DrawRichText is the exported, procedural-symbol-aware drop-in for
// rl.DrawTextEx, for packages outside render that draw the shared UI font
// directly — the map editor, which otherwise renders symbols (▼ ▲ → ≥ ★ ✓ …)
// as missing-glyph boxes since Della Respira lacks them. Same signature as
// rl.DrawTextEx so the swap is mechanical; every editor label then matches
// the game's symbol drawing (and the faux-bold weight, when that's enabled).
func DrawRichText(font rl.Font, text string, position rl.Vector2, fontSize, spacing float32, tint color.RGBA) {
	drawRichText(font, text, position.X, position.Y, fontSize, spacing, tint)
}

// MeasureRichText is the exported, procedural-symbol-aware drop-in for
// rl.MeasureTextEx — pairs with DrawRichText so editor layout (button
// auto-width, tooltip / dropdown sizing) measures symbol-bearing labels to
// their real drawn width instead of the font's missing-glyph advance.
func MeasureRichText(font rl.Font, text string, fontSize, spacing float32) rl.Vector2 {
	return measureRichText(font, text, fontSize, spacing)
}

// walkRichText splits text into ordered segments — maximal runs of font
// glyphs and the procedural symbols between them — invoking onRun(seg) /
// onSym(g) for each in order. THE single splice model behind drawRichText and
// measureRichText, so the two can't disagree on what's a run vs a symbol or
// where the boundaries fall.
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

// drawRichText draws text that may interleave font glyphs and procedural
// symbols, laying segments left-to-right with `spacing` between them so the
// mix tracks like one line. Symbol-free strings take the direct fast path.
func drawRichText(font rl.Font, text string, x, y, size, spacing float32, col color.RGBA) {
	drawRichTextKnown(font, text, containsSymGlyph(text), x, y, size, spacing, col)
}

// richRunMeasureCache memoizes the per-run width the symbol-path draw uses to
// advance the cursor, so an on-screen symbol-bearing label (pause-menu ▸ rows,
// a log line with →) doesn't re-shape its text runs via cgo every frame.
var richRunMeasureCache measureCache

// drawRichTextKnown is drawRichText with the has-symbol bit already decided, so
// a caller drawing the SAME string several times (drop shadow + main pass, the
// engraved gradient bands) pays containsSymGlyph ONCE rather than per pass.
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

// measureRichText is drawRichText's measure twin — same segment+gap model so
// centered / right-aligned callers place mixed strings correctly.
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
	// Height is the true single-line height the font reports (NOT the raw
	// `size`), matching the symbol-free fast path's rl.MeasureTextEx so
	// callers that vertically center on measure.Y don't shift symbol-bearing
	// labels by the font's ascent/descent slack.
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
// line-arrow drawer so they stay mutually aligned: the shaft stops at
// arrowShaftStopFrac (tucked just under the head) and the head triangle is
// centered at arrowHeadCenterFrac.
const (
	arrowShaftStopFrac  = float32(0.7)
	arrowHeadCenterFrac = float32(0.55)
)

// arrowHead draws a small filled triangle pointing along rotation (raylib
// y-down degrees) centered at (hx,hy); shared by the line arrows.
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

func glyphStar(b glyphBox, col color.RGBA) {
	// Five-point star as a triangle fan over alternating outer/inner radii.
	cx, cy := b.cx, b.cy
	outer := b.triRadius()
	inner := outer * 0.42
	pts := make([]rl.Vector2, 0, 12)
	pts = append(pts, rl.NewVector2(cx, cy))
	for i := 0; i <= 10; i++ {
		ang := -90.0 + float64(i)*36.0 // start pointing up
		rad := outer
		if i%2 == 1 {
			rad = inner
		}
		ra := float32(ang) * (3.14159265 / 180)
		pts = append(pts, rl.NewVector2(cx+rad*cosf(ra), cy+rad*sinf(ra)))
	}
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

// glyphEllipsis draws three baseline dots (…) — Della Respira drops the
// precomposed ellipsis, so we stamp it from the same dots a font would.
func glyphEllipsis(b glyphBox, col color.RGBA) {
	r := b.t * 0.7
	// Cap so three dots + gaps always fit the box width (centers at left+r,
	// mid, right-r need >=2r spacing → r <= width/6); guards against overlap /
	// spill past the advance if the glyph's advance is ever tightened or the
	// stroke widened. Floor so the dots never vanish at tiny sizes.
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

// glyphApostrophe draws a short, slightly back-leaning tick high in the box —
// the typographic right single quote (’) the font omits.
func glyphApostrophe(b glyphBox, col color.RGBA) {
	x := (b.left + b.right) / 2
	h := (b.bot - b.top) * 0.34
	rl.DrawLineEx(rl.NewVector2(x+b.t*0.25, b.top), rl.NewVector2(x-b.t*0.15, b.top+h), b.t, col)
}

// glyphApprox draws the two stacked strokes of ≈ (approximately-equal), which
// Della Respira also drops.
func glyphApprox(b glyphBox, col color.RGBA) {
	dy := (b.bot - b.top) * 0.16
	rl.DrawLineEx(rl.NewVector2(b.left, b.cy-dy), rl.NewVector2(b.right, b.cy-dy), b.t, col)
	rl.DrawLineEx(rl.NewVector2(b.left, b.cy+dy), rl.NewVector2(b.right, b.cy+dy), b.t, col)
}

// cosf/sinf are thin float32 wrappers over math.Cos/Sin for glyphStar's
// vertex ring.
func cosf(r float32) float32 { return float32(math.Cos(float64(r))) }
func sinf(r float32) float32 { return float32(math.Sin(float64(r))) }

package render

import (
	"image/color"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Per-frame samples taken once in BeginFrame so the dozens of screenSize/pulse
// callers per frame don't each make a cgo round-trip for values that are constant
// within a frame (window size only changes on resize; the clock is frame-fixed).
var (
	cachedScreenW int32
	cachedScreenH int32
	uiFrameTime   float64
	// windowResizing is true on any frame whose window size differs from the prior
	// frame's — i.e. the window is mid-resize. Full-screen capture targets read it
	// to DEFER their Unload+Load realloc until the size settles (see ensureStable).
	windowResizing bool
)

// BeginFrame samples the window size and wall-clock once at the top of each frame.
// Called from the main loop before any scene draw; the getters below read these
// caches instead of re-querying raylib per call site. A zero cachedScreenW means
// BeginFrame hasn't run yet (tests, pre-first-frame) — the getters fall back to a
// live query so they stay correct.
func BeginFrame() {
	w := int32(rl.GetScreenWidth())
	h := int32(rl.GetScreenHeight())
	// A size change vs the prior frame means the window is being dragged-resized.
	// Skip the very first frame (cachedScreenW==0) so a cold start isn't flagged.
	windowResizing = cachedScreenW != 0 && (w != cachedScreenW || h != cachedScreenH)
	cachedScreenW, cachedScreenH = w, h
	uiFrameTime = rl.GetTime()
}

// frameTime returns the wall-clock sampled in BeginFrame (falls back to a live
// read before the first BeginFrame). Frame-constant home for UI animation curves.
func frameTime() float64 {
	if cachedScreenW == 0 {
		return rl.GetTime()
	}
	return uiFrameTime
}

// screenSize returns the window's width and height as int32.
func screenSize() (w, h int32) {
	if cachedScreenW == 0 {
		return int32(rl.GetScreenWidth()), int32(rl.GetScreenHeight())
	}
	return cachedScreenW, cachedScreenH
}

// clampFrameDelta clips a per-frame delta to the shared simulation floor so a
// long stall can't fast-forward an animation past its lifetime in one frame.
// Shares core.ClampFrameTime so the 1/15s ceiling can't drift from gameplay.
func clampFrameDelta(dt float32) float32 {
	return core.ClampFrameTime(dt)
}

// screenSizeF is the float32 variant for fractional-position callers.
func screenSizeF() (w, h float32) {
	sw, sh := screenSize()
	return float32(sw), float32(sh)
}

// fillScreen paints col over the whole screen — the full-screen overlay stamp
// shared by the tint-wipe cases (caller pre-fades col).
func fillScreen(col color.RGBA) {
	sw, sh := screenSize()
	rl.DrawRectangle(0, 0, sw, sh, col)
}

// centerX returns the left X to horizontally center a panel of width w.
func centerX(w int32) int32 {
	sw, _ := screenSize()
	return sw/2 - w/2
}

// centerXF is the float32 form for sub-pixel centering.
func centerXF(w float32) float32 {
	sw, _ := screenSizeF()
	return (sw - w) / 2
}

// CenterXF is the exported alias of centerXF for packages outside render.
func CenterXF(w float32) float32 { return centerXF(w) }

// ScreenSize / ScreenSizeF are the exported counterparts of screenSize.
func ScreenSize() (w, h int32)    { return screenSize() }
func ScreenSizeF() (w, h float32) { return screenSizeF() }

// drawTextureBillboard renders the full texture as a camera-facing billboard.
// Source rect is always the full texture — no atlas slicing.
func drawTextureBillboard(camera rl.Camera3D, tex rl.Texture2D, pos rl.Vector3, size rl.Vector2, tint rl.Color) {
	source := rl.NewRectangle(0, 0, float32(tex.Width), float32(tex.Height))
	rl.DrawBillboardRec(camera, tex, source, pos, size, tint)
}

// drawTextureBillboardRotated draws a camera-facing billboard spun `deg` degrees in
// its own plane (around center) — used to topple a downed party member flat.
func drawTextureBillboardRotated(camera rl.Camera3D, tex rl.Texture2D, pos rl.Vector3, size rl.Vector2, deg float32, tint rl.Color) {
	source := rl.NewRectangle(0, 0, float32(tex.Width), float32(tex.Height))
	rl.DrawBillboardPro(camera, tex, source, pos, worldUp, size, rl.NewVector2(0, 0), deg, tint)
}

// tileWorldPos returns the world-space center of tile (x, z) at vertical offset y.
func tileWorldPos(x, z int, y float32) rl.Vector3 {
	return rl.NewVector3(core.TileCenter(x), y, core.TileCenter(z))
}

// behindCamera reports whether p sits behind the camera's horizontal forward dir.
// raylib's GetWorldToScreen mirrors points behind the camera to positive XY, which
// would draw in-world prompts on the HUD; callers skip projection when this is true.
// Pitch is ignored — the test is along the floor, since prompts hover above it.
func behindCamera(camera rl.Camera3D, p rl.Vector3) bool {
	forward := horizontalForward(camera)
	dx := p.X - camera.Position.X
	dz := p.Z - camera.Position.Z
	return dx*forward.X+dz*forward.Z <= 0
}

// wrapTextLines greedily packs words into lines no wider than maxW. Words wider
// than maxW on their own are character-broken so no line overflows the panel.
func wrapTextLines(font rl.Font, text string, size, maxW float32) []string {
	if text == "" {
		return nil
	}
	words := splitWords(text)
	if len(words) == 0 {
		return []string{text}
	}
	var out []string
	cur := ""
	flushCur := func() {
		if cur != "" {
			out = append(out, cur)
			cur = ""
		}
	}
	// Measure at canonical tracking so wrapped lines can't overflow when drawn
	// through drawTextWithShadow (which tracks heading-size text wider than 1).
	spacing := canonicalSpacing(size)
	for _, w := range words {
		candidate := w
		if cur != "" {
			candidate = cur + " " + w
		}
		m := rl.MeasureTextEx(font, candidate, size, spacing)
		if m.X <= maxW {
			cur = candidate
			continue
		}
		// Candidate doesn't fit; flush so the new word starts its own row.
		flushCur()
		// Word fits alone → new line. Else it's wider than the panel → char-break.
		wMeasure := rl.MeasureTextEx(font, w, size, spacing)
		if wMeasure.X <= maxW {
			cur = w
			continue
		}
		pieces := breakWideWord(font, w, size, maxW)
		// All but the last become standalone lines; the last becomes cur.
		for i, piece := range pieces {
			if i == len(pieces)-1 {
				cur = piece
			} else {
				out = append(out, piece)
			}
		}
	}
	flushCur()
	return out
}

// breakWideWord splits a word into character-aligned chunks each no wider than
// maxW. If even a single rune overflows, runes are emitted on their own lines.
func breakWideWord(font rl.Font, word string, size, maxW float32) []string {
	if word == "" {
		return nil
	}
	var out []string
	cur := make([]rune, 0, len(word))
	spacing := canonicalSpacing(size)
	for _, r := range word {
		candidate := string(append(cur, r))
		m := rl.MeasureTextEx(font, candidate, size, spacing)
		if m.X <= maxW || len(cur) == 0 {
			cur = append(cur, r)
			continue
		}
		out = append(out, string(cur))
		cur = cur[:0]
		cur = append(cur, r)
	}
	if len(cur) > 0 {
		out = append(out, string(cur))
	}
	return out
}

// splitWords is a strings.Fields equivalent without the import.
func splitWords(s string) []string {
	var out []string
	cur := make([]byte, 0, len(s))
	flush := func() {
		if len(cur) > 0 {
			out = append(out, string(cur))
			cur = cur[:0]
		}
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			flush()
			continue
		}
		cur = append(cur, c)
	}
	flush()
	return out
}

// bodyBelowHeading returns the Y where body content begins beneath a heading
// whose text top sits at headingTop. See UI_STANDARDS.md "Spacing".
func bodyBelowHeading(headingTop int32, fontSize float32) int32 {
	return headingTop + int32(fontSize) + uiGapAfterTitle
}

// footerBaselineY returns the text-top Y for a footer hint uiFooterMargin above
// cardBottom. Subtracting line height keeps the same visual gap at any fontSize.
// See UI_STANDARDS.md "Spacing".
func footerBaselineY(cardBottom int32, fontSize float32) int32 {
	return cardBottom - int32(fontSize) - uiFooterMargin
}

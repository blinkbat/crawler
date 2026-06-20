package render

import (
	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// screenSize returns the current window's width and height as int32. Every
// HUD panel calls this once at the top of its draw function so layout math
// stays consistent within a frame.
func screenSize() (w, h int32) {
	return int32(rl.GetScreenWidth()), int32(rl.GetScreenHeight())
}

// clampFrameDelta clips a per-frame delta to the shared simulation floor
// (core.MaxFrameStep) so a long stall (alt-tab, breakpoint, GC pause) can't
// fast-forward an animation past its lifetime in one frame. The render-side
// animation pools (particles, hit glyphs, bar ghosts) share this so the 1/15s
// ceiling can't drift from the gameplay layers' core.ClampFrameTime.
func clampFrameDelta(dt float32) float32 {
	return core.ClampFrameTime(dt)
}

// screenSizeF is the float32 variant for callers that center / scale by
// fractional positions (popup anchors, splash math). Same underlying
// raylib reads — just typed at the call site.
func screenSizeF() (w, h float32) {
	return float32(rl.GetScreenWidth()), float32(rl.GetScreenHeight())
}

// centerX returns the left X for a panel of width w that should be
// horizontally centered on screen. Caller can post-clamp if needed.
func centerX(w int32) int32 {
	sw, _ := screenSize()
	return sw/2 - w/2
}

// centerXF is the float32 form for sub-pixel centering (splash banner,
// timing-bar layout).
func centerXF(w float32) float32 {
	sw, _ := screenSizeF()
	return (sw - w) / 2
}

// CenterXF is the exported alias of centerXF for packages outside
// render (title screen, top-level menus) that need the same centering
// rule so fullscreen-resize behaviour stays consistent everywhere.
// (Only the float form is exported — render-internal callers that work
// in pixel-snapped int32 layouts use the package-private centerX.)
func CenterXF(w float32) float32 { return centerXF(w) }

// ScreenSize / ScreenSizeF are the exported counterparts of the
// package-private screenSize helpers. External scenes (editor,
// title) used to inline `float32(rl.GetScreenWidth())` at every
// frame-layout site; routing through these keeps one window-size
// reader and matches the centerXF / CenterXF split.
func ScreenSize() (w, h int32)    { return screenSize() }
func ScreenSizeF() (w, h float32) { return screenSizeF() }

// drawTextureBillboard renders the full texture as a camera-facing
// billboard at `pos` with the given world-space `size` and tint. Wraps
// the `rl.DrawBillboardRec(camera, tex, NewRectangle(0, 0, tex.W,
// tex.H), pos, size, tint)` pattern that all three enemy / party /
// pack billboard sites repeat. Source rect is always the full texture
// — no atlas slicing — so abstracting the source-rect derivation here
// removes a per-call texture-dimension read.
func drawTextureBillboard(camera rl.Camera3D, tex rl.Texture2D, pos rl.Vector3, size rl.Vector2, tint rl.Color) {
	source := rl.NewRectangle(0, 0, float32(tex.Width), float32(tex.Height))
	rl.DrawBillboardRec(camera, tex, source, pos, size, tint)
}

// tileWorldPos returns the world-space center of a tile (x, z) at the
// given vertical offset y. Wraps the `core.TileCenter(x), y, core.TileCenter(z)`
// → `rl.NewVector3` pattern that the chest, enemy, party-billboard, and
// pack renderers all repeat. Keeps tile→world projection math in one
// place — if `core.TileCenter` ever changes (e.g. odd-coord offset),
// every billboard tracks the new convention automatically.
func tileWorldPos(x, z int, y float32) rl.Vector3 {
	return rl.NewVector3(core.TileCenter(x), y, core.TileCenter(z))
}

// behindCamera reports whether `p` sits behind the camera's horizontal
// forward direction. raylib's GetWorldToScreen returns a "mirrored"
// positive XY for points behind the camera, which would otherwise draw
// in-world prompts (chest hints, future NPC labels) on the player's
// HUD as they walk past. Callers should skip the projection when this
// returns true. Vertical (pitch) component is intentionally ignored —
// the test is "is it in front of me" along the floor, not along the
// look ray, since prompts hover above the floor by design.
func behindCamera(camera rl.Camera3D, p rl.Vector3) bool {
	forward := horizontalForward(camera)
	dx := p.X - camera.Position.X
	dz := p.Z - camera.Position.Z
	return dx*forward.X+dz*forward.Z <= 0
}

// wrapTextLines greedily packs words from `text` into lines no
// wider than `maxW` at the given font + size. Words that exceed
// `maxW` on their own (long IDs, paths, numeric readouts) are
// character-broken into multiple pieces so the panel never has
// to render text past its right edge. Used by drawActionLogPanel
// and anywhere else a fixed-width text surface needs to soft-
// break sentences.
func wrapTextLines(font rl.Font, text string, size, maxW float32) []string {
	if text == "" {
		return nil
	}
	// strings.Fields collapses runs of whitespace and trims leading /
	// trailing space — action-log lines build via fmt.Sprintf so they
	// don't carry stray indentation, but the trim guards against a
	// future emitter that does.
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
	// Measure at the size's canonical tracking so wrapped lines can't
	// overflow when drawn through drawTextWithShadow (which tracks
	// heading-size text wider than the default 1).
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
		// Candidate doesn't fit. Flush the current line first so the
		// new word starts on its own row.
		flushCur()
		// If the word itself is narrower than maxW, just start a new
		// line with it. Otherwise the word ALONE is wider than the
		// panel — break it character-wise so no line overflows.
		wMeasure := rl.MeasureTextEx(font, w, size, spacing)
		if wMeasure.X <= maxW {
			cur = w
			continue
		}
		pieces := breakWideWord(font, w, size, maxW)
		// All but the last piece become standalone lines; the last
		// becomes the new "cur" so a following short word can append.
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

// breakWideWord splits a single word into character-aligned chunks
// each no wider than maxW. Used as the fallback when wrapTextLines
// gets a single token that's wider than the wrap target. Greedy
// from the left: builds runes one at a time, measuring after each,
// and snaps a chunk when the next rune would push past the limit.
// Always returns at least one piece; if maxW is so tight that even
// a single rune overflows, individual runes are emitted on their
// own lines (still better than letting the original word overflow).
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

// splitWords is a strings.Fields equivalent without the import — keeps
// this file dependency-light. Treats runs of space / tab / newline as
// a single separator.
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

// bodyBelowHeading returns the Y where body content should begin beneath a
// heading whose TEXT TOP sits at headingTop, rendered at fontSize. It adds the
// heading's own line height (≈ the font's pixel size on this bake) plus the
// shared uiGapAfterTitle, so every "content under a header" surface keeps one
// rhythm regardless of the heading's font size. See UI_STANDARDS.md "Spacing".
func bodyBelowHeading(headingTop int32, fontSize float32) int32 {
	return headingTop + int32(fontSize) + uiGapAfterTitle
}

// footerBaselineY returns the Y (glyph/text TOP) for a footer hint that sits
// uiFooterMargin above the bottom edge of a card at cardBottom, rendered at
// fontSize. Because it subtracts the font's line height, every footer keeps
// the SAME visual gap off the bottom edge whether it draws at FontTiny
// (centered modal footer) or FontSmall (left-aligned picker / action-menu
// footer) — the per-surface `-30 / -28 / -26` offsets that drifted before all
// collapse into this one rule. See UI_STANDARDS.md "Spacing".
func footerBaselineY(cardBottom int32, fontSize float32) int32 {
	return cardBottom - int32(fontSize) - uiFooterMargin
}

package core

// Scrollbar / pannable-viewport geometry. Pure float math (no raylib) shared
// by the map editor's scrollbars and canvas pan, kept here so it is unit-
// testable and reusable by any future scrollable surface (a minimap pan, the
// panels Map zoom, …) without dragging the render layer into the math.
//
// Everything is in pixels along ONE axis; callers run the helpers once per
// axis. "offset" is how far the content has been scrolled (0 = showing the
// start). "track" is the scrollbar gutter's length; "view" is the visible
// content length; "content" is the full content length.

// ScrollMaxOffset is the largest valid scroll offset for content of length
// contentLen shown through a viewport of length viewLen. Zero when the content
// fits — there is nothing to scroll.
func ScrollMaxOffset(viewLen, contentLen float32) float32 {
	if contentLen <= viewLen {
		return 0
	}
	return contentLen - viewLen
}

// ScrollThumbExtent returns the length of a scrollbar thumb on a track of
// trackLen showing viewLen of contentLen. It is proportional to the visible
// fraction (viewLen/contentLen), floored at minThumb so it stays grabbable on
// very long content and capped at the track length. Content that fits (or
// degenerate inputs) yields a full-length thumb.
func ScrollThumbExtent(trackLen, viewLen, contentLen, minThumb float32) float32 {
	if contentLen <= viewLen || contentLen <= 0 || trackLen <= 0 {
		return trackLen
	}
	ext := trackLen * viewLen / contentLen
	if ext < minThumb {
		ext = minThumb
	}
	if ext > trackLen {
		ext = trackLen
	}
	return ext
}

// ScrollThumbPos maps a scroll offset to the thumb's leading-edge position
// within the track (0 .. trackLen-thumbExtent). The offset is clamped to its
// valid range first, so callers may pass a raw value.
func ScrollThumbPos(trackLen, viewLen, contentLen, minThumb, offset float32) float32 {
	maxOff := ScrollMaxOffset(viewLen, contentLen)
	if maxOff <= 0 {
		return 0
	}
	travel := trackLen - ScrollThumbExtent(trackLen, viewLen, contentLen, minThumb)
	if travel <= 0 {
		return 0
	}
	return travel * Clamp(offset, 0, maxOff) / maxOff
}

// ScrollOffsetForThumbPos is the inverse of ScrollThumbPos: given the thumb's
// leading-edge position within the track, return the scroll offset it
// represents (clamped to the valid offset range). Turns a thumb drag into a
// scroll value.
func ScrollOffsetForThumbPos(trackLen, viewLen, contentLen, minThumb, thumbPos float32) float32 {
	maxOff := ScrollMaxOffset(viewLen, contentLen)
	if maxOff <= 0 {
		return 0
	}
	travel := trackLen - ScrollThumbExtent(trackLen, viewLen, contentLen, minThumb)
	if travel <= 0 {
		return 0
	}
	return Clamp(maxOff*Clamp(thumbPos, 0, travel)/travel, 0, maxOff)
}

// ClampPanAxis bounds a pan offset on one axis so panned content can't be
// dragged out of its viewport. base is the content's screen position at
// pan==0 (e.g. a centered origin); viewStart/viewLen describe the viewport on
// that axis; contentLen is the content's pixel extent. When the content fits
// inside the viewport it is kept fully inside (pan may nudge within the slack
// but never push content partly off). When it overflows, panning can bring
// either edge to the viewport edge plus `overscroll` slack so boundary content
// is not jammed flush against the panel. Returns the clamped pan.
func ClampPanAxis(pan, base, viewStart, viewLen, contentLen, overscroll float32) float32 {
	pos := base + pan
	var lo, hi float32
	if contentLen <= viewLen {
		lo = viewStart
		hi = viewStart + viewLen - contentLen
	} else {
		lo = viewStart + viewLen - contentLen - overscroll
		hi = viewStart + overscroll
	}
	return Clamp(pos, lo, hi) - base
}

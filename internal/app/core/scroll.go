package core

// Scrollbar / pannable-viewport geometry. Pure float math (no raylib), pixels
// along ONE axis; callers run the helpers once per axis. "offset" = how far
// scrolled (0 = start); "track" = gutter length; "view" = visible; "content" = full.

// ScrollMaxOffset is the largest valid scroll offset; zero when content fits.
func ScrollMaxOffset(viewLen, contentLen float32) float32 {
	if contentLen <= viewLen {
		return 0
	}
	return contentLen - viewLen
}

// ScrollThumbExtent returns the thumb length, proportional to the visible
// fraction (viewLen/contentLen), floored at minThumb and capped at trackLen.
// Content that fits (or degenerate inputs) yields a full-length thumb.
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
// (0 .. trackLen-thumbExtent). Offset is clamped first, so callers may pass raw.
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

// ScrollOffsetForThumbPos is the inverse of ScrollThumbPos: thumb position →
// scroll offset (clamped). Turns a thumb drag into a scroll value.
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

// ClampPanAxis bounds a pan offset on one axis so content can't be dragged out
// of its viewport. base is the content's screen position at pan==0. Content that
// fits is kept fully inside; content that overflows can pan to the viewport edge
// plus `overscroll` slack. Returns the clamped pan.
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

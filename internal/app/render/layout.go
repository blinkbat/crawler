package render

import (
	rl "github.com/gen2brain/raylib-go/raylib"
)

// screenSize returns the current window's width and height as int32. Every
// HUD panel calls this once at the top of its draw function so layout math
// stays consistent within a frame.
func screenSize() (w, h int32) {
	return int32(rl.GetScreenWidth()), int32(rl.GetScreenHeight())
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

package render

import (
	"image/color"
	"math"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Screen-wipe FX: the battle-entry transition (and the Debug ▸ Screen Wipe FX
// preview). Kept CHEAP and retro-safe — the scene renders normally (retro filtering
// intact); these effects ride either the camera (Zoom/Spin/Wobble) or a full-screen
// overlay (Tint/Flash/Vignette). No render-target capture, so retro never blinks off.

const battleWipeDuration = core.BattleWipePreviewSeconds

// Screen-wipe FX tuning magnitudes (were inline magics across the camera + overlay
// cases).
const (
	wipeZoomFovDrop    = float32(32)   // WipeZoom: degrees of FOV narrowing at entry
	wipeZoomFovFloor   = float32(12)   // WipeZoom: FOV floor
	wipeWobbleFreq     = float32(38)   // WipeWobble: oscillation frequency
	wipeWobbleRoll     = float32(0.16) // WipeWobble: roll amplitude (radians)
	wipeTintMaxAlpha   = float32(0.6)  // WipeTint: peak overlay opacity at entry
	wipeVignetteAlpha  = float32(0.92) // WipeVignette: dark-iris ring opacity
	wipeWobbleFovDip   = float32(6)    // WipeWobble: FOV breathing coupled to the roll oscillation
	wipeFlashHalfLife  = float32(0.5)  // WipeFlash: fraction of t over which the flash fully decays
	wipeFlashPeakAlpha = float32(0.9)  // WipeFlash: peak white opacity at entry
)

// wipeTintColor is the warm overlay tone the WipeTint case washes the frame with.
var wipeTintColor = rl.NewColor(255, 170, 80, 255)

// battleWipeProgress returns (t in 0..1, active). The debug preview timer wins;
// otherwise the early window of Battle.Splash drives it. t goes 0 (full FX) → 1
// (settled). Inactive for WipeNone.
func battleWipeProgress(g *core.GameState) (float32, bool) {
	if g.BattleWipe == core.WipeNone {
		return 0, false
	}
	if g.BattleWipePreview > 0 {
		return core.Clamp(1-g.BattleWipePreview/battleWipeDuration, 0, 1), true
	}
	if g.Battle.Splash > 0 {
		if elapsed := core.BattleSplashDuration - g.Battle.Splash; elapsed >= 0 && elapsed < battleWipeDuration {
			return elapsed / battleWipeDuration, true
		}
	}
	return 0, false
}

func wipeEase(t float32) float32 { return core.Smoothstep(t) }

// battleWipeCamera returns the camera up-vector + FOV after applying any camera-based
// wipe FX (Zoom/Spin/Wobble) for this frame; identity (worldUp, fov) otherwise. dir is
// the camera's normalized look direction (for the roll axis).
func battleWipeCamera(g *core.GameState, dir rl.Vector3, fov float32) (rl.Vector3, float32) {
	up := worldUp
	t, active := battleWipeProgress(g)
	if !active {
		return up, fov
	}
	r := 1 - wipeEase(t) // 1 at entry → 0 settled
	switch g.BattleWipe {
	case core.WipeZoom:
		if fov -= wipeZoomFovDrop * r; fov < wipeZoomFovFloor {
			fov = wipeZoomFovFloor
		}
	case core.WipeSpin:
		up = wipeRollUp(dir, r*0.6)
	case core.WipeWobble:
		osc := float32(math.Sin(float64(t*wipeWobbleFreq))) * r
		up = wipeRollUp(dir, osc*wipeWobbleRoll)
		fov -= osc * wipeWobbleFovDip
	}
	return up, fov
}

// wipeRollUp rolls the camera up-vector by `roll` radians around the look axis.
func wipeRollUp(dir rl.Vector3, roll float32) rl.Vector3 {
	right := rl.Vector3Normalize(rl.Vector3CrossProduct(dir, worldUp))
	camUp := rl.Vector3Normalize(rl.Vector3CrossProduct(right, dir))
	c, s := float32(math.Cos(float64(roll))), float32(math.Sin(float64(roll)))
	return rl.Vector3Normalize(rl.Vector3Add(rl.Vector3Scale(camUp, c), rl.Vector3Scale(right, s)))
}

// DrawBattleWipeOverlay paints the overlay-based wipe FX (Tint/Flash/Vignette) over
// the scene + HUD during the transition. Camera-based kinds are no-ops here.
func DrawBattleWipeOverlay(g *core.GameState, _ Resources) {
	t, active := battleWipeProgress(g)
	// Pixelate caches a one-time scene snapshot; (re)build or release it to match state.
	syncWipeGrid(g, active)
	if !active {
		return
	}
	w, h := screenSizeF()
	switch g.BattleWipe {
	case core.WipeTint:
		fillScreen(fadeColor(wipeTintColor, (1-t)*wipeTintMaxAlpha))
	case core.WipeFlash:
		if a := 1 - t/wipeFlashHalfLife; a > 0 { // gone by half
			fillScreen(fadeColor(rl.White, a*wipeFlashPeakAlpha))
		}
	case core.WipeVignette:
		// Dark iris opening from the center: a ring from the growing hole radius out
		// to past the corner. inner=0 (full dark) → diag (revealed).
		diag := float32(math.Hypot(float64(w), float64(h))) / 2
		rl.DrawRing(rl.NewVector2(w/2, h/2), wipeEase(t)*diag, diag+4, 0, 360, 64, fadeColor(rl.Black, wipeVignetteAlpha))
	case core.WipePixelate:
		drawWipePixelate(t)
	}
}

// --- Pixelate ("Pixel Blur") -------------------------------------------------
//
// Costs a one-time GPU→CPU screen readback (the user opted in). To keep the PER-FRAME
// cost cheap we sample the snapshot ONCE into a small cached color grid, then each
// frame just draw the grid as blocks that merge chunky→fine as the wipe resolves and
// fade out to reveal the live scene. No per-frame readback or image sampling.

const wipePixelGridCols = 128 // snapshot grid width in cells (height derives from aspect)

var (
	wipeGridColors []color.RGBA
	wipeGridCols   int32
	wipeGridRows   int32
	wipeBlockW     int32 // px per cell on screen
	wipeGridW      int32 // screen size the snapshot was captured at (resize → recapture)
	wipeGridH      int32
	wipeGridValid  bool
)

// syncWipeGrid (re)captures the pixelate snapshot when the effect needs it and frees
// it otherwise. Capture is the one-time readback; cells are sampled once here. A
// mid-effect window resize re-captures so the cached grid never reads at a stale size.
func syncWipeGrid(g *core.GameState, active bool) {
	want := active && g.BattleWipe == core.WipePixelate
	if !want {
		if wipeGridValid {
			wipeGridColors = wipeGridColors[:0]
			wipeGridValid = false
		}
		return
	}
	sw, sh := screenSize()
	if sw <= 0 || sh <= 0 {
		return
	}
	if wipeGridValid && sw == wipeGridW && sh == wipeGridH {
		return // already captured at the current size
	}
	// Invalidate before mutating grid dims/recapturing: a failed readback below must not
	// leave drawWipePixelate indexing the new (larger) cols/rows against the stale slice.
	wipeGridValid = false
	wipeBlockW = sw / wipePixelGridCols
	if wipeBlockW < 1 {
		wipeBlockW = 1
	}
	wipeGridCols = sw / wipeBlockW
	wipeGridRows = sh / wipeBlockW
	img := rl.LoadImageFromScreen() // one readback of the composited (retro-filtered) frame
	if img == nil {
		return
	}
	n := int(wipeGridCols * wipeGridRows)
	if cap(wipeGridColors) < n {
		wipeGridColors = make([]color.RGBA, n)
	}
	wipeGridColors = wipeGridColors[:n]
	for gy := int32(0); gy < wipeGridRows; gy++ {
		for gx := int32(0); gx < wipeGridCols; gx++ {
			px := gx*wipeBlockW + wipeBlockW/2
			py := gy*wipeBlockW + wipeBlockW/2
			if px >= sw {
				px = sw - 1
			}
			if py >= sh {
				py = sh - 1
			}
			wipeGridColors[gy*wipeGridCols+gx] = rl.GetImageColor(*img, px, py)
		}
	}
	rl.UnloadImage(img)
	wipeGridW, wipeGridH = sw, sh
	wipeGridValid = true
}

// drawWipePixelate draws the cached snapshot grid as blocks that merge chunky→fine
// (stride shrinks 6→1) and fade out, resolving into the live scene behind.
func drawWipePixelate(t float32) {
	if !wipeGridValid {
		return
	}
	e := wipeEase(t)
	stride := int32((1-e)*5) + 1                    // 6 cells/block → 1 (fine)
	alpha := uint8(255 * core.Clamp(2*(1-e), 0, 1)) // opaque first half, fades second
	for gy := int32(0); gy < wipeGridRows; gy += stride {
		bh := stride * wipeBlockW
		for gx := int32(0); gx < wipeGridCols; gx += stride {
			c := colorWithAlpha(wipeGridColors[gy*wipeGridCols+gx], alpha)
			rl.DrawRectangle(gx*wipeBlockW, gy*wipeBlockW, stride*wipeBlockW, bh, c)
		}
	}
}

package render

import (
	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Menu fade — a quick cross-fade between the world HUD and a top-level menu.
// Opening a top-level menu (the pause MENU family or the character Tome) fades
// the world HUD OUT while the menu fades IN; closing reverses. Submenu
// transitions (Tome tab → skill-tree picker, battle action-menu → skill list)
// do NOT re-trigger it: they don't flip a top-level open flag, so the predicate
// below stays true across the sub-navigation and the fade holds at fully-shown.
//
// Presentation-only state, render-side — same convention as the particle /
// bar-ghost / hit-glyph pools (no GameState field; ticked once per frame from
// DrawOverlay via rl.GetFrameTime, clamped through clampFrameDelta).
const menuFadeDur = float32(0.08) // seconds for a full HUD↔menu cross-fade

var fadeRT previewRT

// inFadeCapture guards against re-entering the fade render-target capture.
// raylib's BeginTextureMode/EndTextureMode is not a real stack on this build —
// a nested EndTextureMode pops to the backbuffer rather than the outer target.
// Current callers never nest, but the guard makes a future nested call degrade
// to a plain draw instead of silently breaking the outer capture.
var inFadeCapture bool

var menuFade struct {
	progress float32                          // 0 = world HUD shown, 1 = menu shown
	drawer   func(*core.GameState, Resources) // the open (or fading-out) top-level menu's drawer
}

// menuFadeDrawer returns the drawer for the currently-open top-level menu (the
// pause-menu family or the character Tome), or nil when none is open. Priority
// mirrors DrawOverlay's former early-return ladder. The Tome routes through the
// gate-less drawPanelsBody so it can keep drawing through a close-fade after
// g.PanelsOpen has already flipped false (closePanels preserves tab/cursor
// state, so the fading-out frame renders the same page).
func menuFadeDrawer(g *core.GameState) func(*core.GameState, Resources) {
	switch {
	case g.MenuOpen:
		return drawMenuOverlay
	case g.OptionsMenuOpen:
		return drawOptionsMenuOverlay
	case g.ShopOpen:
		return drawShopOverlay
	case g.RetroMenuOpen:
		return drawRetroMenuOverlay
	case g.DebugMenuOpen:
		return drawDebugMenuOverlay
	case g.PanelsOpen:
		return drawPanelsBody
	}
	return nil
}

// tickMenuFade advances the cross-fade toward "menu shown" (1) while a
// top-level menu is open, else back toward "world shown" (0). Called once per
// frame at the top of DrawOverlay.
func tickMenuFade(g *core.GameState) {
	target := float32(0)
	if d := menuFadeDrawer(g); d != nil {
		menuFade.drawer = d
		target = 1
	}
	step := clampFrameDelta(rl.GetFrameTime()) / menuFadeDur
	switch {
	case menuFade.progress < target:
		menuFade.progress += step
		if menuFade.progress > target {
			menuFade.progress = target
		}
	case menuFade.progress > target:
		menuFade.progress -= step
		if menuFade.progress < target {
			menuFade.progress = target
		}
	}
	// Fully back to the world: drop the cached drawer so a stale menu can't
	// redraw, and so the next open re-resolves the current one.
	if menuFade.progress <= 0 {
		menuFade.progress = 0
		menuFade.drawer = nil
	}
}

// withFadeAlpha draws `draw` at the given 0..1 opacity. At full opacity it
// draws straight to the backbuffer (no render-target cost — the common,
// not-fading path); at zero it skips entirely; in between it captures `draw`
// into the fade RT and blits it back with a modulated alpha, fading a whole
// HUD/menu layer as one unit without threading an alpha through every call.
func withFadeAlpha(alpha float32, draw func()) {
	if alpha >= 0.999 {
		draw()
		return
	}
	if alpha <= 0.001 {
		return
	}
	if inFadeCapture {
		// Already inside a fade capture — nesting would break the outer target's
		// Begin/EndTextureMode pairing. Draw straight through instead.
		draw()
		return
	}
	sw, sh := screenSize()
	if !fadeRT.ensure(sw, sh) {
		draw() // allocation failed — a full-opacity draw beats a blank frame
		return
	}
	inFadeCapture = true
	rl.BeginTextureMode(fadeRT.rt)
	rl.ClearBackground(rl.Blank) // transparent, so the blit composites over the world
	draw()
	rl.EndTextureMode()
	inFadeCapture = false
	// Premultiplied-alpha blit. Content drawn into the RT over a transparent
	// (Blank) clear with standard alpha blending is stored PREMULTIPLIED (color
	// already scaled by coverage). Blitting that back with normal alpha blend
	// would multiply the translucent pixels by their alpha a SECOND time, so the
	// see-through card body / veil darkened and vanished faster than the opaque
	// frame strokes — the "bg fades before the border" artifact. Premultiplied
	// blend (out = src.rgb + dst·(1−src.a)) composites both proportionally, so the
	// whole card (body AND border) fades as one. Only the transition frames hit
	// this path (full opacity draws straight to the backbuffer), so the
	// steady-state look is unchanged.
	//
	// The fade tint must scale ALL FOUR channels by `alpha`: premultiplied rgb is
	// folded into the color, so a (255,255,255,alpha) tint would leave rgb at full
	// and an opaque pixel would resolve to menu + dst·(1−alpha·a) — i.e. ADD the
	// menu over the world instead of fading it out. A uniform gray (alpha in every
	// channel) scales the premultiplied rgb and the coverage together.
	g := uint8(alpha * 255)
	tint := rl.NewColor(g, g, g, g)
	rl.BeginBlendMode(rl.BlendAlphaPremultiply)
	// blitTinted flips the bottom-up RenderTexture upright and applies the
	// premultiplied gray fade tint.
	fadeRT.blitTinted(rl.NewRectangle(0, 0, 0, 0), tint)
	rl.EndBlendMode()
}

// closeFadeRT frees the fade capture texture. Wired into Resources.Unload.
func closeFadeRT() { fadeRT.close() }

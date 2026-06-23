package render

import (
	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Menu fade — cross-fade between the world HUD and a top-level menu. Submenu
// transitions don't re-trigger it (they don't flip a top-level open flag), so
// the fade holds fully-shown across sub-navigation.
//
// Presentation-only render-side state; ticked once per frame from DrawOverlay.
const menuFadeDur = float32(0.08)

var fadeRT previewRT

// inFadeCapture guards re-entry: raylib's Begin/EndTextureMode isn't a real
// stack here — a nested End pops to the backbuffer, breaking the outer capture.
var inFadeCapture bool

var menuFade struct {
	progress float32                          // 0 = world HUD, 1 = menu
	drawer   func(*core.GameState, Resources) // open (or fading-out) menu's drawer
}

// menuFadeDrawer returns the open top-level menu's drawer, or nil. The Tome
// routes through gate-less drawPanelsBody so it keeps drawing through a close-
// fade after g.PanelsOpen flips false (closePanels preserves tab/cursor state).
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
	case g.CombatTuneOpen:
		return drawCombatTuneOverlay
	case g.WipeMenuOpen:
		return drawWipeMenuOverlay
	case g.DebugMenuOpen:
		return drawDebugMenuOverlay
	case g.PanelsOpen:
		return drawPanelsBody
	}
	return nil
}

// tickMenuFade advances the cross-fade toward 1 while a menu is open, else 0.
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
	// Fully back to the world: drop the cached drawer so a stale menu can't redraw.
	if menuFade.progress <= 0 {
		menuFade.progress = 0
		menuFade.drawer = nil
	}
}

// withFadeAlpha draws `draw` at 0..1 opacity. Full opacity draws straight to the
// backbuffer; zero skips; in between it captures into the fade RT and blits back
// with modulated alpha, fading a whole layer as one unit.
func withFadeAlpha(alpha float32, draw func()) {
	if alpha >= 0.999 {
		draw()
		return
	}
	if alpha <= 0.001 {
		return
	}
	if inFadeCapture {
		// Nesting would break the outer Begin/EndTextureMode pairing — draw through.
		draw()
		return
	}
	sw, sh := screenSize()
	if !fadeRT.ensure(sw, sh) {
		draw() // allocation failed — full-opacity draw beats a blank frame
		return
	}
	inFadeCapture = true
	rl.BeginTextureMode(fadeRT.rt)
	rl.ClearBackground(rl.Blank) // transparent, so the blit composites over the world
	draw()
	rl.EndTextureMode()
	inFadeCapture = false
	// Premultiplied-alpha blit: content over a Blank clear is stored premultiplied,
	// so a normal-alpha blit would double-multiply translucent pixels (body fades
	// before border). Premultiplied blend fades body AND border as one. The tint
	// must scale ALL FOUR channels by alpha (uniform gray) — a (255,255,255,alpha)
	// tint would ADD the menu over the world instead of fading it out.
	g := uint8(alpha * 255)
	tint := rl.NewColor(g, g, g, g)
	rl.BeginBlendMode(rl.BlendAlphaPremultiply)
	// blitTinted flips the bottom-up RenderTexture upright + applies the tint.
	fadeRT.blitTinted(rl.NewRectangle(0, 0, 0, 0), tint)
	rl.EndBlendMode()
}

// closeFadeRT frees the fade capture texture. Wired into Resources.Unload.
func closeFadeRT() { fadeRT.close() }

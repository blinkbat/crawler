package render

import (
	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// DisplayMode names the user-selectable window presentation. The app boots
// in Fullscreen (borderless windowed covering the monitor — see Run's call
// to render.SetDisplayMode(DisplayFullscreen)); Windowed gives the user a
// resizable bordered window so they can drag, snap, or share-screen.
type DisplayMode int

const (
	DisplayFullscreen DisplayMode = iota
	DisplayWindowed
)

// displayModeNames indexes DisplayMode → menu label. A single table (rather
// than an if/return pair) means a future third mode added to the iota block
// surfaces a "?" label here instead of being silently folded into "Windowed",
// matching how other label tables in the codebase derive their text from a
// parallel slice indexed by the enum.
var displayModeNames = []string{
	DisplayFullscreen: "Fullscreen",
	DisplayWindowed:   "Windowed",
}

// lastWindowedW/H snapshot the window's size at the moment we transition
// AWAY from Windowed. Restored on the next transition back. Initialized to
// core.InitialWindowWidth/Height so a fresh process that never sees a user
// resize still gets a sensible bordered window matching the launch size —
// the same constants that seed the initial rl.InitWindow call.
var (
	lastWindowedW = int32(core.InitialWindowWidth)
	lastWindowedH = int32(core.InitialWindowHeight)
)

// CurrentDisplayMode reports which mode the window is in. Requires BOTH
// that the framebuffer match the monitor size AND that the
// FlagBorderlessWindowedMode bit is set — a user who drags + snaps a
// bordered window to monitor size would match the size check but not the
// flag check, so the menu label still correctly reads "Windowed" for them.
func CurrentDisplayMode() DisplayMode {
	if !rl.IsWindowState(rl.FlagBorderlessWindowedMode) {
		return DisplayWindowed
	}
	mw, mh, ok := currentMonitorSize()
	if !ok {
		return DisplayWindowed
	}
	if rl.GetScreenWidth() >= mw && rl.GetScreenHeight() >= mh {
		return DisplayFullscreen
	}
	return DisplayWindowed
}

// DisplayModeLabel returns the menu-row label for the given mode. An
// out-of-range mode returns a visible "?" sentinel rather than masquerading
// as one of the real modes, so a missing displayModeNames entry reads as a
// bug at the UI rather than a silent mislabel.
func DisplayModeLabel(m DisplayMode) string {
	if int(m) < 0 || int(m) >= len(displayModeNames) {
		return "Display ?"
	}
	return displayModeNames[m]
}

// currentMonitorSize reports the active monitor's pixel dimensions, with ok
// false when raylib can't report them (the <=0 guard the size-dependent
// transitions all need). Centralizes the GetCurrentMonitor + width + height +
// guard triple so the three call sites can't drift on the bounds check.
func currentMonitorSize() (mw, mh int, ok bool) {
	monitor := rl.GetCurrentMonitor()
	mw = rl.GetMonitorWidth(monitor)
	mh = rl.GetMonitorHeight(monitor)
	if mw <= 0 || mh <= 0 {
		return 0, 0, false
	}
	return mw, mh, true
}

// DisplayMenuRowLabel returns the full "Display: Fullscreen" / "Display:
// Windowed" string used by both the pause menu and the title screen. Pulls
// the current mode at call time so the label refreshes per-frame; centralized
// so a future label change (e.g. localization) is one edit.
func DisplayMenuRowLabel() string {
	return "Display: " + DisplayModeLabel(CurrentDisplayMode())
}

// SetDisplayMode switches the window to the requested mode. Idempotent —
// calling SetDisplayMode(Fullscreen) when the window is already fullscreen
// is a no-op beyond a defensive re-position. When leaving Windowed we
// snapshot its dimensions so the next return to Windowed restores the
// user's chosen size.
func SetDisplayMode(m DisplayMode) {
	current := CurrentDisplayMode()
	if m == DisplayFullscreen {
		// Remember the user's bordered window size before snapping to monitor.
		if current == DisplayWindowed {
			if w := int32(rl.GetScreenWidth()); w > 0 {
				lastWindowedW = w
			}
			if h := int32(rl.GetScreenHeight()); h > 0 {
				lastWindowedH = h
			}
		}
		w, h, ok := currentMonitorSize()
		if !ok {
			return
		}
		position := rl.GetMonitorPosition(rl.GetCurrentMonitor())
		// Set the borderless flag BEFORE the size/position so the OS doesn't
		// transiently flash a title-bar windowed state at monitor size. Then
		// SetWindowSize and SetWindowPosition land in the already-borderless
		// window.
		rl.SetWindowState(rl.FlagBorderlessWindowedMode)
		rl.SetWindowSize(w, h)
		rl.SetWindowPosition(int(position.X), int(position.Y))
		return
	}
	// Windowed: drop borderless, restore the last user-chosen size (or
	// default if never resized), and re-center on the active monitor.
	rl.ClearWindowState(rl.FlagBorderlessWindowedMode)
	rl.SetWindowSize(int(lastWindowedW), int(lastWindowedH))
	position := rl.GetMonitorPosition(rl.GetCurrentMonitor())
	if mw, mh, ok := currentMonitorSize(); ok {
		cx := int(position.X) + (mw-int(lastWindowedW))/2
		cy := int(position.Y) + (mh-int(lastWindowedH))/2
		rl.SetWindowPosition(cx, cy)
	} else {
		// Monitor info not available — at minimum park the bordered window
		// at the screen origin so it isn't stuck at the previous borderless
		// offset (which could be off-screen on a second monitor that just
		// disconnected).
		rl.SetWindowPosition(0, 0)
	}
}

// ToggleDisplayMode flips between Fullscreen and Windowed. Used by the
// pause-menu and title-screen Display rows on confirm.
func ToggleDisplayMode() {
	if CurrentDisplayMode() == DisplayFullscreen {
		SetDisplayMode(DisplayWindowed)
		return
	}
	SetDisplayMode(DisplayFullscreen)
}

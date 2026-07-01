package render

import (
	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// DisplayMode names the window presentation. Fullscreen is borderless windowed
// covering the monitor; Windowed is a resizable bordered window. Boots Fullscreen.
type DisplayMode int

const (
	DisplayFullscreen DisplayMode = iota
	DisplayWindowed
	displayModeCount
)

// displayModeNames indexes DisplayMode → menu label. Fixed-size array: a missing
// entry is a compile error (matching the package's [PanelTabCount]/[StatCount] tables).
var displayModeNames = [displayModeCount]string{
	DisplayFullscreen: "Fullscreen",
	DisplayWindowed:   "Windowed",
}

// lastWindowedW/H snapshot the window size when leaving Windowed, restored on
// the way back. Seeded to core.InitialWindowWidth/Height for a fresh process.
var (
	lastWindowedW = int32(core.InitialWindowWidth)
	lastWindowedH = int32(core.InitialWindowHeight)
)

// CurrentDisplayMode reports the window's mode, keyed purely on the
// FlagBorderlessWindowedMode bit — SetDisplayMode sets that flag ONLY on the
// fullscreen path, so it's the authoritative signal. (The old size>=monitor
// cross-check misread a genuine fullscreen window as Windowed on DPI-scaled
// monitors that report the borderless framebuffer a pixel under monitor size.)
func CurrentDisplayMode() DisplayMode {
	if rl.IsWindowState(rl.FlagBorderlessWindowedMode) {
		return DisplayFullscreen
	}
	return DisplayWindowed
}

// DisplayModeLabel returns the menu-row label; out-of-range yields a "?" sentinel.
func DisplayModeLabel(m DisplayMode) string {
	if int(m) < 0 || int(m) >= len(displayModeNames) {
		return "Display ?"
	}
	return displayModeNames[m]
}

// currentMonitorSize reports the active monitor's pixel size; ok false when
// raylib can't report them (the <=0 guard the size-dependent transitions need).
func currentMonitorSize() (mw, mh int, ok bool) {
	monitor := rl.GetCurrentMonitor()
	mw = rl.GetMonitorWidth(monitor)
	mh = rl.GetMonitorHeight(monitor)
	if mw <= 0 || mh <= 0 {
		return 0, 0, false
	}
	return mw, mh, true
}

// DisplayMenuRowLabel returns the "Display: <mode>" row string, resolved at call time.
func DisplayMenuRowLabel() string {
	return "Display: " + DisplayModeLabel(CurrentDisplayMode())
}

// SetDisplayMode switches the window to mode m. Idempotent. Leaving Windowed
// snapshots its size so the next return restores it.
func SetDisplayMode(m DisplayMode) {
	current := CurrentDisplayMode()
	if m == DisplayFullscreen {
		// Remember the bordered window size before snapping to monitor.
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
		// Borderless flag BEFORE size/position so the OS doesn't flash a title-bar
		// windowed state at monitor size.
		rl.SetWindowState(rl.FlagBorderlessWindowedMode)
		rl.SetWindowSize(w, h)
		rl.SetWindowPosition(int(position.X), int(position.Y))
		return
	}
	// Windowed: drop borderless, restore the last size, re-center.
	rl.ClearWindowState(rl.FlagBorderlessWindowedMode)
	rl.SetWindowSize(int(lastWindowedW), int(lastWindowedH))
	position := rl.GetMonitorPosition(rl.GetCurrentMonitor())
	if mw, mh, ok := currentMonitorSize(); ok {
		cx := int(position.X) + (mw-int(lastWindowedW))/2
		cy := int(position.Y) + (mh-int(lastWindowedH))/2
		rl.SetWindowPosition(cx, cy)
	} else {
		// No monitor info — park at origin so the window isn't stuck at a stale
		// borderless offset (possibly off-screen on a just-disconnected monitor).
		rl.SetWindowPosition(0, 0)
	}
}

// ToggleDisplayMode flips between Fullscreen and Windowed.
func ToggleDisplayMode() {
	if CurrentDisplayMode() == DisplayFullscreen {
		SetDisplayMode(DisplayWindowed)
		return
	}
	SetDisplayMode(DisplayFullscreen)
}

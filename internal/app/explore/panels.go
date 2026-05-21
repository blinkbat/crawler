package explore

import (
	"crawler/internal/app/core"
	"crawler/internal/app/input"
)

// openPanels raises the game-panels overlay and resets the per-tab
// cursor so a re-open starts on a fresh row. The tab itself is
// preserved across opens — players who left off on the Items tab
// re-enter on Items. PanelsMapZoom is preserved separately so map
// pans don't reset either.
func openPanels(g *core.GameState) {
	g.PanelsOpen = true
	g.PanelsRowCursor = 0
	if g.PanelsMapZoom <= 0 {
		g.PanelsMapZoom = panelsMapZoomDefault
	}
	// Snap the look yaw/pitch back to neutral so a half-rotated
	// free-look doesn't bleed into the overlay's screen-space rendering.
	g.Player.LookYaw = 0
	g.Player.LookPitch = 0
}

// closePanels takes the overlay down. Tab + zoom + cursor state stays
// on the GameState so reopening lands the player back where they were.
func closePanels(g *core.GameState) {
	g.PanelsOpen = false
}

// panelsMapZoomDefault is the initial cells-on-screen value for the Map
// tab. Tuned so a typical 20×14 map fits comfortably with room for the
// player arrow.
const panelsMapZoomDefault = 14

// panelsMapZoomMin / Max bound the zoomable-map's cells-on-screen
// extents. Below the minimum the map crowds the player marker out;
// above the maximum even a tiny map shows whole-map plus padding.
const (
	panelsMapZoomMin = 6
	panelsMapZoomMax = 48
)

// updatePanels routes one frame of input to whatever tab is up.
// Always-on bindings (close / tab cycle) live here; per-tab cursors
// dispatch through panelTabHandler.
func updatePanels(g *core.GameState) {
	// Both Back and the toggle close — same edge handling so the
	// big-start button is a true toggle.
	if input.BackPressed() || input.PanelsTogglePressed() {
		closePanels(g)
		return
	}
	// L1 / R1 + Tab + Left/Right cycle tabs. Use TargetPrevious /
	// TargetNext rather than ArrowLeft / ArrowRight because the
	// shoulders read more naturally as "page back / forward" on a
	// pad than the d-pad does.
	if input.TargetNextPressed() {
		setPanelTab(g, panelTabAdvance(g.PanelsTab, 1))
		return
	}
	if input.TargetPreviousPressed() {
		setPanelTab(g, panelTabAdvance(g.PanelsTab, -1))
		return
	}
	switch g.PanelsTab {
	case core.PanelTabStats, core.PanelTabEquipment, core.PanelTabSkills:
		// These tabs are read-only displays. Up/Down scroll the
		// party member cursor where it's meaningful; Confirm /
		// Back are handled above.
		g.PanelsRowCursor = input.CursorUpDown(g.PanelsRowCursor, len(g.Party))
	case core.PanelTabItems:
		count := len(core.LiveStacks(g.Inventory))
		g.PanelsRowCursor = input.CursorUpDown(g.PanelsRowCursor, count)
	case core.PanelTabMap:
		// Up/Down zooms the map (matching the wheel-zoom in the
		// editor). One press is one zoom step; the bounds are
		// soft-clamped so holding the key doesn't run off the rails.
		if input.UpPressed() {
			g.PanelsMapZoom -= 2
		}
		if input.DownPressed() {
			g.PanelsMapZoom += 2
		}
		if g.PanelsMapZoom < panelsMapZoomMin {
			g.PanelsMapZoom = panelsMapZoomMin
		}
		if g.PanelsMapZoom > panelsMapZoomMax {
			g.PanelsMapZoom = panelsMapZoomMax
		}
	}
}

// panelTabAdvance shifts the tab cursor by delta with wrap. Centralized
// so the L1/R1 paths don't drift apart on the wrap math.
func panelTabAdvance(t core.PanelTab, delta int) core.PanelTab {
	next := int(t) + delta
	count := int(core.PanelTabCount)
	if count <= 0 {
		return t
	}
	return core.PanelTab(core.WrapIndex(next, count))
}

// setPanelTab switches the active tab and resets the per-tab cursor.
// Map zoom is preserved (handled separately on GameState).
func setPanelTab(g *core.GameState, t core.PanelTab) {
	if t == g.PanelsTab {
		return
	}
	g.PanelsTab = t
	g.PanelsRowCursor = 0
}

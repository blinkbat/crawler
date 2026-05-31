package explore

import (
	"crawler/internal/app/core"
	"crawler/internal/app/input"
	"crawler/internal/app/render"
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
		g.PanelsMapZoom = core.PanelMapZoomDefault
	}
	// Snap the look yaw/pitch back to neutral so a half-rotated
	// free-look doesn't bleed into the overlay's screen-space rendering.
	g.Player.LookYaw = 0
	g.Player.LookPitch = 0
	core.ResetEquipCursor(g)
}

// closePanels takes the overlay down. Tab + zoom + cursor state stays
// on the GameState so reopening lands the player back where they were.
// Any in-progress equipment drag is cancelled and the render-side
// hit-rect layout is zeroed so reopening doesn't resume with a held
// item or route a frame-one click against stale rects from the prior
// session.
func closePanels(g *core.GameState) {
	g.PanelsOpen = false
	core.ClearEquipDrag(g)
	render.ResetEquipPanelLayout()
}

// updatePanels routes one frame of input to whatever tab is up.
// Always-on bindings (close / tab paging) live here; per-tab cursors
// dispatch through the switch below.
//
// Navigation model: the shoulders + triggers page TABS (so the d-pad /
// left stick is free to be an in-tab cursor), and the d-pad / stick
// drives a 2-D cursor inside a tab — Left/Right picks the party-member
// column, Up/Down moves the slot row (Equipment) or zooms (Map).
func updatePanels(g *core.GameState) {
	// Back closes the overlay — EXCEPT on the Equipment tab while an
	// item is held by the keyboard/controller cursor, where Back drops
	// the held item back instead (so a mis-lift doesn't kick the player
	// all the way out). The toggle button always closes outright.
	if input.BackPressed() {
		if g.PanelsTab == core.PanelTabEquipment && g.EquipDrag.Source != core.EquipDragSourceNone {
			core.ClearEquipDrag(g)
			return
		}
		closePanels(g)
		return
	}
	if input.PanelsTogglePressed() {
		closePanels(g)
		return
	}
	// Tab paging: L1/L2 back, R1/R2 forward on a pad; Tab / Shift+Tab on
	// the keyboard. The arrows are deliberately NOT wired here anymore —
	// they drive the in-tab cursor below.
	if input.MenuTabNextPressed() {
		setPanelTab(g, panelTabAdvance(g.PanelsTab, 1))
		return
	}
	if input.MenuTabPrevPressed() {
		setPanelTab(g, panelTabAdvance(g.PanelsTab, -1))
		return
	}
	// Direct-tab keyboard shortcuts (C / E / I / K / M). Pressing the
	// shortcut for the tab you're already on closes the overlay so
	// the same key gesture that opens "Items" also clears "Items"
	// — keeps the mnemonic keys feeling like real toggles.
	if tab, ok := input.PanelTabShortcutPressed(); ok {
		if tab == g.PanelsTab {
			closePanels(g)
		} else {
			setPanelTab(g, tab)
		}
		return
	}
	switch g.PanelsTab {
	case core.PanelTabCharacter:
		// Left/Right moves the party-member column; Confirm on a member
		// with unspent stat points opens the level-up modal targeting
		// that member. The modal closes the panels overlay automatically
		// (its own input loop takes over). The "+" badge on the
		// party-card name is the player's hint that allocation is
		// available.
		g.PanelsRowCursor = input.CursorLeftRightWrap(g.PanelsRowCursor, len(g.Party))
		if input.ConfirmPressed() && g.PanelsRowCursor >= 0 && g.PanelsRowCursor < len(g.Party) {
			m := g.Party[g.PanelsRowCursor]
			if m.PendingLevelUps > 0 {
				closePanels(g)
				openLevelUpFor(g, g.PanelsRowCursor)
			}
		}
	case core.PanelTabEquipment:
		// Equipment tab supports BOTH mouse drag-and-drop AND a
		// keyboard/controller lift/place cursor — see updateEquipmentTab.
		updateEquipmentTab(g)
	case core.PanelTabSkills:
		// View-only tab: Left/Right moves the party-member column.
		g.PanelsRowCursor = input.CursorLeftRightWrap(g.PanelsRowCursor, len(g.Party))
	case core.PanelTabItems:
		// Vertical inventory list: Up/Down walk the stacks.
		count := core.LiveStackCount(g.Inventory)
		g.PanelsRowCursor = input.CursorUpDown(g.PanelsRowCursor, count)
	case core.PanelTabMap:
		// Up/Down zooms the map by one cells-on-screen step per press;
		// the bounds (core.PanelMapZoomMin/Max) are soft-clamped so holding
		// the key doesn't run off the rails. (This is its own zoom model
		// in cells-on-screen, distinct from the editor's float scale.)
		if input.UpPressed() {
			g.PanelsMapZoom -= core.PanelMapZoomStep
		}
		if input.DownPressed() {
			g.PanelsMapZoom += core.PanelMapZoomStep
		}
		g.PanelsMapZoom = core.Clamp(g.PanelsMapZoom, core.PanelMapZoomMin, core.PanelMapZoomMax)
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
// Map zoom is preserved (handled separately on GameState). Any
// in-flight Equipment drag is cancelled — leaving it active across
// tab switches would let an LMB-release on a non-Equipment tab go
// unobserved (updateEquipmentDrag only runs on the Equipment tab),
// stranding the held item to be landed by the next stray click when
// the user returns. Resetting the stored render-side hit-rect
// layout on the same edge keeps the first frame after a switch INTO
// Equipment from reading a stale layout from a previous visit.
func setPanelTab(g *core.GameState, t core.PanelTab) {
	if t == g.PanelsTab {
		return
	}
	core.ClearEquipDrag(g)
	core.ResetEquipCursor(g)
	render.ResetEquipPanelLayout()
	g.PanelsTab = t
	g.PanelsRowCursor = 0
}

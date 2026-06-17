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
	// Re-center the Map tab on the player each open (pan is a transient inspect
	// offset, not persistent state).
	g.PanelsMapPanX = 0
	g.PanelsMapPanZ = 0
	// Snap the look yaw/pitch back to neutral so a half-rotated
	// free-look doesn't bleed into the overlay's screen-space rendering.
	g.Player.LookYaw = 0
	g.Player.LookPitch = 0
	resetPanelSubmodals(g)
}

// resetPanelSubmodals closes every panel sub-dialog (equipment slot picker,
// use-target picker, skill-tree modal, heal chooser) in one place. The three
// overlay lifecycle edges (open / close / tab-switch) all call it, so a new
// sub-modal added to the overlay can't be forgotten on one path and leak
// across it. The render-side hit-rect reset (ResetEquipPanelLayout) stays at
// the close / tab-switch sites since open doesn't need it.
func resetPanelSubmodals(g *core.GameState) {
	core.ResetEquipPanels(g)
	closeUseTarget(g)
	closeSkillTree(g)
	closeHealPick(g)
}

// closePanels takes the overlay down. Tab + zoom + cursor state stays
// on the GameState so reopening lands the player back where they were.
// Any in-progress equipment drag is cancelled and the render-side
// hit-rect layout is zeroed so reopening doesn't resume with a held
// item or route a frame-one click against stale rects from the prior
// session.
func closePanels(g *core.GameState) {
	g.PanelsOpen = false
	resetPanelSubmodals(g)
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
	// The out-of-battle "use" ally-target picker (Items / Skills tabs)
	// and the Equipment-tab slot picker are modal sub-dialogs: while one
	// is open it owns every panel input this frame (Back closes just the
	// picker, not the whole overlay; tab paging / shortcuts are
	// suppressed). Handle them first and return so nothing below sees the
	// same edge.
	if g.SkillTreeOpen {
		updateSkillTreeModal(g)
		return
	}
	if g.HealPickOpen {
		updateHealPicker(g)
		return
	}
	if g.UseTargetOpen {
		updateUseTargetPicker(g)
		return
	}
	if g.PanelsTab == core.PanelTabEquipment && g.EquipPickerOpen {
		updateEquipPicker(g)
		return
	}
	// Back closes the overlay. The toggle button always closes outright.
	if input.BackPressed() {
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
	if next, changed := input.PagedTab(g.PanelsTab, int(core.PanelTabCount)); changed {
		setPanelTab(g, next)
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
		// Equipment tab: a 2-D cursor over members × slots; Confirm or a
		// mouse click on a slot opens that slot's item picker — see
		// updateEquipmentTab.
		updateEquipmentTab(g)
	case core.PanelTabSkills:
		// Summary view: Left/Right picks the party-member column; Confirm
		// opens that member's skill-tree modal (the Diablo-2-style three-
		// tree sub-dialog where SkillPoints are actually spent). Use (F / □)
		// still casts a Heal-tag skill out of battle — a separate button so
		// opening the trees and casting a heal never collide.
		g.PanelsRowCursor = input.CursorLeftRightWrap(g.PanelsRowCursor, len(g.Party))
		if input.ConfirmPressed() && g.PanelsRowCursor >= 0 && g.PanelsRowCursor < len(g.Party) {
			openSkillTreeFor(g, g.PanelsRowCursor)
		}
		if input.UsePressed() {
			tryUseSkill(g)
		}
	case core.PanelTabItems:
		// Vertical inventory list: Up/Down walk the stacks. Confirm or
		// Use (F / □) uses the cursored consumable on a chosen ally.
		count := core.LiveStackCount(g.Inventory)
		g.PanelsRowCursor = input.CursorUpDown(g.PanelsRowCursor, count)
		if input.ConfirmPressed() || input.UsePressed() {
			tryUseItem(g)
		}
	case core.PanelTabQuests:
		// Journal hosts two sub-views: Left/Right toggles Quests ↔ Bestiary
		// (resetting the row cursor so each opens at the top), Up/Down scroll
		// the active list. Read-only — quests advance through gameplay hooks,
		// the bestiary fills through kills / scans, neither acts on Confirm.
		if sub := core.JournalSubtab(input.CursorLeftRightWrap(int(g.JournalTab), int(core.JournalSubtabCount))); sub != g.JournalTab {
			g.JournalTab = sub
			g.PanelsRowCursor = 0
		}
		rows := len(g.Quests)
		if g.JournalTab == core.JournalBestiary {
			rows = g.Bestiary.SeenCount()
		}
		g.PanelsRowCursor = input.CursorUpDown(g.PanelsRowCursor, rows)
	case core.PanelTabMap:
		// D-pad/stick PANS the map (the natural map-scroll control); Confirm (A)
		// CYCLES zoom one step tighter, wrapping back to the most-zoomed-out at
		// the floor — this frees all four directions for panning. Back (B) closes
		// (global). The pan step scales with zoom so a tap scrolls a meaningful
		// chunk at any scale.
		step := g.PanelsMapZoom / core.PanelMapPanDivisor
		if step < core.PanelMapPanStepMin {
			step = core.PanelMapPanStepMin
		}
		if dx := input.CursorLeftRight(); dx != 0 {
			g.PanelsMapPanX += dx * step
		}
		if input.UpPressed() {
			g.PanelsMapPanZ -= step
		}
		if input.DownPressed() {
			g.PanelsMapPanZ += step
		}
		if input.ConfirmPressed() {
			g.PanelsMapZoom -= core.PanelMapZoomStep
			if g.PanelsMapZoom < core.PanelMapZoomMin {
				g.PanelsMapZoom = core.PanelMapZoomMax
			}
		}
		g.PanelsMapZoom = core.Clamp(g.PanelsMapZoom, core.PanelMapZoomMin, core.PanelMapZoomMax)
		// Clamp the pan so the view center can't wander more than a map's span
		// off the player — generous enough to inspect any explored corner, tight
		// enough that you can't scroll into an endless void.
		g.PanelsMapPanX = core.Clamp(g.PanelsMapPanX, -g.Area.Width, g.Area.Width)
		g.PanelsMapPanZ = core.Clamp(g.PanelsMapPanZ, -g.Area.Height, g.Area.Height)
	default:
		// The render side is a compile-locked panelTabDrawers array; this
		// input-side dispatch is hand-maintained, so a new tab without an
		// input case would silently accept no input. Fail loudly instead.
		panic("explore: updatePanels missing input case for PanelTab")
	}
}

// setPanelTab switches the active tab and resets the per-tab cursor.
// Map zoom is preserved (handled separately on GameState). The
// Equipment-tab state (slot cursor + any open slot picker) is reset so
// a tab switch can't leave the picker stranded, and the stored
// render-side hit-rect layout is zeroed on the same edge so the first
// frame after a switch INTO Equipment can't read a stale layout from a
// previous visit.
func setPanelTab(g *core.GameState, t core.PanelTab) {
	if t == g.PanelsTab {
		return
	}
	resetPanelSubmodals(g)
	render.ResetEquipPanelLayout()
	g.PanelsTab = t
	g.PanelsRowCursor = 0
	// Re-center the Map view whenever it's (re)entered — pan is a transient
	// inspect offset, so a previous visit's scroll shouldn't linger.
	g.PanelsMapPanX = 0
	g.PanelsMapPanZ = 0
}

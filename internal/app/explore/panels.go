package explore

import (
	"crawler/internal/app/core"
	"crawler/internal/app/input"
	"crawler/internal/app/render"
)

// openPanels raises the overlay and resets the per-tab cursor. The tab and
// PanelsMapZoom are preserved across opens.
func openPanels(g *core.GameState) {
	g.PanelsOpen = true
	g.PanelsRowCursor = 0
	if g.PanelsMapZoom <= 0 {
		g.PanelsMapZoom = core.PanelMapZoomDefault
	}
	// Re-center the Map tab each open (pan is a transient inspect offset).
	recenterPanelMap(g)
	// Neutral look so a half-rotated free-look doesn't bleed into the overlay.
	resetLook(&g.Player)
	resetPanelSubmodals(g)
}

// resetPanelSubmodals closes every panel sub-dialog in one place, so a new one
// can't be forgotten on one of the open/close/tab-switch edges. The render-side
// hit-rect reset stays at the close/tab-switch sites (open doesn't need it).
func resetPanelSubmodals(g *core.GameState) {
	core.ResetEquipPanels(g)
	closeUseTarget(g)
	closeSkillTree(g)
	closeHealPick(g)
	g.PanelSwapSource = -1 // clear any half-started Character-tab formation swap
}

// closePanels takes the overlay down (tab/zoom/cursor persist so reopening lands
// where you were). Zeroes the render-side hit-rect layout so a frame-one click
// can't route against stale rects.
func closePanels(g *core.GameState) {
	g.PanelsOpen = false
	resetPanelSubmodals(g)
	render.ResetEquipPanelLayout()
}

// updatePanels routes one frame of input to the active tab. Always-on bindings
// (close / tab paging) live here; per-tab cursors dispatch through the switch.
// Nav model: shoulders + triggers page TABS, freeing the d-pad / stick to be an
// in-tab cursor (Left/Right = member column, Up/Down = slot row or zoom).
func updatePanels(g *core.GameState, dt float32) {
	// The use-target picker (Items/Skills) and the Equipment slot picker are modal
	// sub-dialogs: while one is open it owns every panel input (Back closes just
	// the picker, paging/shortcuts suppressed). Handle them first and return.
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
	// Back closes the overlay — UNLESS a Character-tab swap is half-started, where
	// it just cancels the pickup. The toggle button always closes outright (below).
	if input.BackPressed() {
		if g.PanelsTab == core.PanelTabCharacter && g.PanelSwapSource >= 0 {
			g.PanelSwapSource = -1
			return
		}
		closePanels(g)
		return
	}
	if input.PanelsTogglePressed() {
		closePanels(g)
		return
	}
	// Tab paging: L1/L2 back, R1/R2 forward; Tab / Shift+Tab on keyboard. Arrows
	// are NOT wired here — they drive the in-tab cursor below.
	if next, changed := input.PagedTab(g.PanelsTab, int(core.PanelTabCount)); changed {
		setPanelTab(g, next)
		return
	}
	// Direct-tab shortcuts (C/E/I/K/J/M). The shortcut for the current tab closes
	// the overlay, so the mnemonic keys feel like real toggles.
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
		// 2-D cursor over the formation grid: a D-pad/arrow press moves to the
		// orthogonal neighbour's member (by home slot), mirroring the on-screen 2×2.
		// Confirm on a member with unspent stat points opens the level-up modal.
		// Self-heal an out-of-range cursor so nav can't get stuck (all moves below
		// are gated on the cursor being in range).
		if len(g.Party) > 0 && !core.PartyIndexInRange(g.Party, g.PanelsRowCursor) {
			g.PanelsRowCursor = 0
		}
		switch dx := input.CursorLeftRight(); {
		case input.UpPressed():
			moveFormationCursor(g, -1, 0)
		case input.DownPressed():
			moveFormationCursor(g, 1, 0)
		case dx != 0:
			moveFormationCursor(g, 0, dx)
		}
		if input.ConfirmPressed() {
			if m, ok := validMember(g, g.PanelsRowCursor); ok && m.PendingLevelUps > 0 {
				closePanels(g)
				openLevelUpFor(g, g.PanelsRowCursor)
			}
		}
		// Use (□ / F) is the free formation SWAP: first press picks up the cursored
		// member, a second on a DIFFERENT member trades slots (a clean 2×2 swap),
		// re-pressing the held member cancels. Persists into the next fight.
		if input.UsePressed() && core.PartyIndexInRange(g.Party, g.PanelsRowCursor) {
			switch {
			case !core.PartyIndexInRange(g.Party, g.PanelSwapSource):
				g.PanelSwapSource = g.PanelsRowCursor
				g.SetStatusMessage("Swap " + g.Party[g.PanelsRowCursor].Name + " with whom?")
			case g.PanelSwapSource == g.PanelsRowCursor:
				g.PanelSwapSource = -1 // re-picked held member → cancel
			default:
				a, b := g.PanelSwapSource, g.PanelsRowCursor
				nameA, nameB := g.Party[a].Name, g.Party[b].Name
				core.SwapFormationSlots(g.Party, a, b)
				g.PanelSwapSource = -1
				g.SetStatusMessage(core.SwapPlacesMessage(nameA, nameB))
			}
		}
	case core.PanelTabEquipment:
		// 2-D cursor over members × slots; Confirm / click opens the slot picker.
		updateEquipmentTab(g)
	case core.PanelTabSkills:
		// Left/Right picks the member column, Confirm opens the skill-tree modal.
		// Use (F / □) casts a Heal skill out of battle (a separate button).
		g.PanelsRowCursor = input.CursorLeftRightWrap(g.PanelsRowCursor, len(g.Party))
		if input.ConfirmPressed() && core.PartyIndexInRange(g.Party, g.PanelsRowCursor) {
			openSkillTreeFor(g, g.PanelsRowCursor)
		}
		if input.UsePressed() {
			tryUseSkill(g)
		}
	case core.PanelTabItems:
		// Up/Down walk stacks; Confirm / Use applies the cursored consumable to an ally.
		count := core.LiveStackCount(g.Inventory)
		g.PanelsRowCursor = input.CursorUpDown(g.PanelsRowCursor, count)
		if input.ConfirmPressed() || input.UsePressed() {
			tryUseItem(g)
			// Consuming the last unit shrinks the live list; re-clamp now (mirrors the
			// shop/chest paths) so the highlight doesn't point past the new last stack.
			g.PanelsRowCursor = core.ClampIndex(g.PanelsRowCursor, core.LiveStackCount(g.Inventory))
		}
	case core.PanelTabQuests:
		// Two sub-views: Left/Right toggles Quests ↔ Bestiary (resetting the row
		// cursor), Up/Down scroll the active list. Read-only (no Confirm action).
		if sub := core.JournalSubtab(input.CursorLeftRightWrap(int(g.JournalTab), int(core.JournalSubtabCount))); sub != g.JournalTab {
			g.JournalTab = sub
			g.PanelsRowCursor = 0
		}
		g.PanelsRowCursor = input.CursorUpDown(g.PanelsRowCursor, g.JournalRowCount())
	case core.PanelTabMap:
		// Pan: left stick / d-pad / arrows / WASD (analog, both axes). Zoom: right
		// stick / mouse wheel. Analog input accumulates into the integer tile-pan +
		// stepped zoom. Pan speed scales with zoom so a flick covers a similar fraction
		// of the view at any scale.
		px, pz := input.MapPanInput()
		panRate := float32(g.PanelsMapZoom) * core.PanelMapPanRateFrac
		g.PanelsMapPanAccumX += px * panRate * dt
		g.PanelsMapPanAccumZ += pz * panRate * dt
		g.PanelsMapPanX += drainSteps(&g.PanelsMapPanAccumX)
		g.PanelsMapPanZ += drainSteps(&g.PanelsMapPanAccumZ)

		// Right stick up (negative) zooms in; wheel up (positive) zooms in. Both feed
		// one accumulator; each whole unit steps the zoom by PanelMapZoomStep (positive
		// accum = zoom in = PanelsMapZoom decreases).
		g.PanelsMapZoomAccum += -input.MapZoomAxis()*core.PanelMapZoomStickRate*dt + input.MapZoomWheel()
		g.PanelsMapZoom -= drainSteps(&g.PanelsMapZoomAccum) * core.PanelMapZoomStep
		g.PanelsMapZoom = core.Clamp(g.PanelsMapZoom, core.PanelMapZoomMin, core.PanelMapZoomMax)
		// Clamp the pan to a map's span off the player — enough to inspect any
		// corner, not into an endless void.
		g.PanelsMapPanX = core.Clamp(g.PanelsMapPanX, -g.Area.Width, g.Area.Width)
		g.PanelsMapPanZ = core.Clamp(g.PanelsMapPanZ, -g.Area.Height, g.Area.Height)
	default:
		// Hand-maintained dispatch (render side is a compile-locked array); a new
		// tab without a case would silently accept no input. Fail loudly.
		panic("explore: updatePanels missing input case for PanelTab")
	}
}

// homeSlotMember returns the party index whose preferred (home) slot is (row,col),
// or false if none — drives the Character tab's 2×2 formation-grid navigation.
func homeSlotMember(g *core.GameState, row core.Row, col core.Col) (int, bool) {
	for i := range g.Party {
		if g.Party[i].HomeRow == row && g.Party[i].HomeCol == col {
			return i, true
		}
	}
	return 0, false
}

// setPanelTab switches the active tab and resets the per-tab cursor (map zoom is
// preserved). Resets the Equipment slot picker and zeroes the render-side
// hit-rect layout so a switch INTO Equipment can't read a stale layout.
func setPanelTab(g *core.GameState, t core.PanelTab) {
	if t == g.PanelsTab {
		return
	}
	resetPanelSubmodals(g)
	render.ResetEquipPanelLayout()
	g.PanelsTab = t
	g.PanelsRowCursor = 0
	// Re-center the Map view on (re)entry — pan is a transient inspect offset.
	recenterPanelMap(g)
}

// moveFormationCursor walks the Character-tab cursor one step over the 2×2 home
// grid (dRow/dCol in {-1,0,1}); a move that would leave the grid is a no-op. Folds
// the four orthogonal arms into one rule: up from back / down from front flips the
// row; left from right / right from left flips the column.
func moveFormationCursor(g *core.GameState, dRow, dCol int) {
	if !core.PartyIndexInRange(g.Party, g.PanelsRowCursor) {
		return
	}
	row, col := g.Party[g.PanelsRowCursor].HomeRow, g.Party[g.PanelsRowCursor].HomeCol
	switch {
	case dRow < 0 && row == core.RowBack, dRow > 0 && row == core.RowFront:
		if idx, ok := homeSlotMember(g, core.FlipRow(row), col); ok {
			g.PanelsRowCursor = idx
		}
	case dCol < 0 && col == core.ColRight, dCol > 0 && col == core.ColLeft:
		if idx, ok := homeSlotMember(g, row, core.FlipCol(col)); ok {
			g.PanelsRowCursor = idx
		}
	}
}

// drainSteps drains whole units from an analog accumulator, returning the signed
// whole-unit count and leaving the sub-unit remainder for the next frame (truncates
// toward 0, so it handles both directions). Used for the Map tab's 1:1 tile pan and
// the stepped zoom alike.
func drainSteps(acc *float32) int {
	n := int(*acc)
	*acc -= float32(n)
	return n
}

// recenterPanelMap clears the Map tab's pan offset (a transient inspect offset) so
// the view re-centers on the player, and zeroes the analog accumulators.
func recenterPanelMap(g *core.GameState) {
	g.PanelsMapPanX = 0
	g.PanelsMapPanZ = 0
	g.PanelsMapPanAccumX, g.PanelsMapPanAccumZ, g.PanelsMapZoomAccum = 0, 0, 0
}

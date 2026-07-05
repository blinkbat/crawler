package explore

import (
	"fmt"

	"crawler/internal/app/audio"
	"crawler/internal/app/core"
	"crawler/internal/app/input"
)

// updateLevelUpModal drives the stat-spend dialog. Flow is "stage, then commit":
// Confirm on a stat row stages a point (if budget allows); Back on a staged row
// decrements it, Back on an empty row closes and reverts ALL staged points;
// Confirm on the Apply row commits via core.CommitLevelUp then advances.
// Staged (not immediate-spend) so the player can preview and back out.
func updateLevelUpModal(g *core.GameState) {
	if !g.LevelUpOpen {
		return
	}
	// The modal is scoped to a single member (opened from the Character tab for the
	// cursored member). If it somehow opens on a member with nothing to spend, close
	// rather than hopping to another member — allocation never jumps chars on its own.
	if !core.PartyIndexInRange(g.Party, g.LevelUpMember) ||
		g.Party[g.LevelUpMember].PendingLevelUps <= 0 {
		closeLevelUp(g)
		return
	}

	g.LevelUpRowCursor = input.CursorUpDown(g.LevelUpRowCursor, core.LevelUpRowCount)

	m := &g.Party[g.LevelUpMember]
	if input.ConfirmPressed() {
		switch {
		case core.IsLevelUpStatRow(g.LevelUpRowCursor):
			// Stage one more point if budget allows.
			if core.SumStatPending(g.LevelUpPending) < m.PendingLevelUps {
				g.LevelUpPending[g.LevelUpRowCursor]++
				audio.Play(audio.SoundInputHit)
			}
		case g.LevelUpRowCursor == core.LevelUpApplyRowIndex:
			// Commit staged picks, then close back to the Character tab. Allocation is
			// per-char: we never auto-jump to the next member with unspent points — any
			// remaining points stay banked and glow on that member's card.
			core.CommitLevelUp(m, g.LevelUpPending)
			closeLevelUp(g)
		default:
			// CursorUpDown clamps to LevelUpRowCount, so every row is a stat row or the
			// apply row — fail loud if the row layout ever grows a third kind (matches the
			// package's other menu switches).
			panic(fmt.Sprintf("updateLevelUpModal: LevelUpRowCursor %d is neither a stat row nor the apply row", g.LevelUpRowCursor))
		}
	}
	// Back on a staged stat row decrements it; on an empty focused row it closes
	// the modal (staged points on OTHER rows revert).
	if input.BackPressed() {
		if core.IsLevelUpStatRow(g.LevelUpRowCursor) && g.LevelUpPending[g.LevelUpRowCursor] > 0 {
			g.LevelUpPending[g.LevelUpRowCursor]--
		} else {
			closeLevelUp(g)
		}
	}
}

// openLevelUpFor opens (or re-focuses) the modal on a member, clearing staged
// allocations. Single seam for the four LevelUp* field writes.
func openLevelUpFor(g *core.GameState, member int) {
	g.LevelUpOpen = true
	g.LevelUpMember = member
	clearLevelUpStaging(g)
}

// closeLevelUp dismisses the modal and clears its staged state.
func closeLevelUp(g *core.GameState) {
	g.LevelUpOpen = false
	clearLevelUpStaging(g)
}

// clearLevelUpStaging zeroes the staged stat allocations and resets the row cursor.
func clearLevelUpStaging(g *core.GameState) {
	g.LevelUpPending = [core.StatCount]int{}
	g.LevelUpRowCursor = 0
}

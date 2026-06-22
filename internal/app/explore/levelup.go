package explore

import (
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
	if g.LevelUpMember < 0 || g.LevelUpMember >= len(g.Party) ||
		g.Party[g.LevelUpMember].PendingLevelUps <= 0 {
		advanceLevelUpMember(g)
	}
	if !g.LevelUpOpen {
		return
	}

	g.LevelUpRowCursor = input.CursorUpDown(g.LevelUpRowCursor, core.LevelUpRowCount)

	m := &g.Party[g.LevelUpMember]
	if input.ConfirmPressed() {
		switch {
		case isStatRow(g.LevelUpRowCursor):
			// Stage one more point if budget allows.
			if core.SumStatPending(g.LevelUpPending) < m.PendingLevelUps {
				g.LevelUpPending[g.LevelUpRowCursor]++
				audio.Play(audio.SoundInputHit)
			}
		case g.LevelUpRowCursor == core.LevelUpApplyRowIndex:
			// Commit staged picks; advanceLevelUpMember resets the pendings.
			core.CommitLevelUp(m, g.LevelUpPending)
			advanceLevelUpMember(g)
		}
	}
	// Back on a staged stat row decrements it; on an empty focused row it closes
	// the modal (staged points on OTHER rows revert).
	if input.BackPressed() {
		if isStatRow(g.LevelUpRowCursor) && g.LevelUpPending[g.LevelUpRowCursor] > 0 {
			g.LevelUpPending[g.LevelUpRowCursor]--
		} else {
			closeLevelUp(g)
		}
	}
}

// isStatRow reports whether the cursor sits on a stat row vs the Apply row.
func isStatRow(cursor int) bool {
	return cursor < int(core.StatCount)
}

// openLevelUpFor opens (or re-focuses) the modal on a member, clearing staged
// allocations. Single seam for the four LevelUp* field writes.
func openLevelUpFor(g *core.GameState, member int) {
	g.LevelUpOpen = true
	g.LevelUpMember = member
	g.LevelUpRowCursor = 0
	g.LevelUpPending = [core.StatCount]int{}
}

// closeLevelUp dismisses the modal and clears its staged state.
func closeLevelUp(g *core.GameState) {
	g.LevelUpOpen = false
	g.LevelUpRowCursor = 0
	g.LevelUpPending = [core.StatCount]int{}
}

// advanceLevelUpMember moves to the next member with unspent points, or closes
// when none remain. Each transition clears the staged-pending state.
func advanceLevelUpMember(g *core.GameState) {
	next := core.FirstPendingLevelUp(g.Party)
	if next < 0 {
		closeLevelUp(g)
		return
	}
	openLevelUpFor(g, next)
}

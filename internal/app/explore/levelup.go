package explore

import (
	"crawler/internal/app/audio"
	"crawler/internal/app/core"
	"crawler/internal/app/input"
)

// updateLevelUpModal drives the post-battle stat-spend dialog.
//
// The flow is "stage, then commit": Confirm (Z / A) on a stat row
// increments that stat's pending counter (if the player still has
// budget). Back (X / B / Esc) on a staged stat row decrements that
// row by one; Back on an empty row closes the modal and reverts ALL
// staged points. Confirm on the Apply row commits every staged
// change to the underlying member via core.CommitLevelUp, then
// advances to the next pending member or closes the modal.
//
// Why staged instead of immediate-spend: lets the player see the
// resulting stat block (current -> +pending = new) before locking
// in, and gives them a chance to back out of a misclick without a
// "save draft" feature.
//
// The modal is no longer auto-opened post-battle (the player chooses
// when via the Tome menu's Character tab); these handlers only run
// when something explicitly sets g.LevelUpOpen = true. Esc-out IS
// now allowed via the Back key, matching the rest of the HUD's
// Z/X conventions — the old "must spend before leaving" lock is
// gone.
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
			// Stat row: stage one more point if the budget allows.
			if core.SumStatPending(g.LevelUpPending) < m.PendingLevelUps {
				g.LevelUpPending[g.LevelUpRowCursor]++
				audio.Play(audio.SoundInputHit)
			}
		case g.LevelUpRowCursor == core.LevelUpApplyRowIndex:
			// Apply commits staged picks. advanceLevelUpMember resets the
			// staged pendings (so a re-open starts fresh) whether it moves
			// to the next member or closes.
			core.CommitLevelUp(m, g.LevelUpPending)
			advanceLevelUpMember(g)
		}
	}
	// Back (X / B / Esc) on a staged stat row decrements that row's
	// pending count. If nothing is staged on the focused row, Back
	// closes the modal entirely — staged points on OTHER rows revert.
	// Routes through input.BackPressed so the gamepad's Circle / B
	// button works alongside keyboard X / Esc without inventing any
	// new keys (replaces the old Backspace handler that didn't match
	// the rest of the HUD's Z/X conventions).
	if input.BackPressed() {
		if isStatRow(g.LevelUpRowCursor) && g.LevelUpPending[g.LevelUpRowCursor] > 0 {
			g.LevelUpPending[g.LevelUpRowCursor]--
		} else {
			closeLevelUp(g)
		}
	}
}

// isStatRow reports whether the level-up cursor sits on a stat row, as
// opposed to the Apply row one past the last stat (LevelUpApplyRowIndex).
func isStatRow(cursor int) bool {
	return cursor < int(core.StatCount)
}

// openLevelUpFor opens (or re-focuses) the stat-spend modal on a specific
// party member, clearing any staged allocations. Single seam for the
// four LevelUp* field writes the panels overlay and advanceLevelUpMember
// used to inline.
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

// advanceLevelUpMember moves the modal to the next party member with
// unspent stat points, or closes the modal when no member has any. Each
// transition clears the staged-pending state so the new member (or a
// future re-open) starts with a clean ledger.
func advanceLevelUpMember(g *core.GameState) {
	next := core.FirstPendingLevelUp(g.Party)
	if next < 0 {
		closeLevelUp(g)
		return
	}
	openLevelUpFor(g, next)
}

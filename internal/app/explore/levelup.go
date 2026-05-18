package explore

import (
	"crawler/internal/app/audio"
	"crawler/internal/app/core"
	"crawler/internal/app/input"
)

// updateLevelUpModal handles the post-battle stat-spend dialog. Up/Down
// pick a stat row, Confirm spends one point into the highlighted stat.
// Spent points decrement PendingLevelUps; when the current member runs
// out, advance to the next member with pending points; when nobody has
// points left, close the modal. Esc isn't honored — the player MUST
// allocate every point. A future "save draft" feature could relax that.
func updateLevelUpModal(g *core.GameState) {
	if !g.LevelUpOpen {
		return
	}
	// Defensive: if the current member ran out of points without the
	// "advance" path firing, find the next pending member or close.
	if g.LevelUpMember < 0 || g.LevelUpMember >= len(g.Party) ||
		g.Party[g.LevelUpMember].PendingLevelUps <= 0 {
		advanceLevelUpMember(g)
	}
	if !g.LevelUpOpen {
		return
	}

	g.LevelUpStat = core.Stat(input.CursorUpDown(int(g.LevelUpStat), int(core.StatCount)))

	if input.ConfirmPressed() {
		m := &g.Party[g.LevelUpMember]
		if core.SpendStatPoint(m, g.LevelUpStat) {
			audio.Play(audio.SoundInputHit)
		}
		// If this member just spent their last point, advance to the
		// next pending member, or close if none left. advanceLevelUpMember
		// reads PendingLevelUps after the spend so the just-finished
		// member doesn't get re-selected.
		if m.PendingLevelUps <= 0 {
			advanceLevelUpMember(g)
		}
	}
}

// advanceLevelUpMember moves the modal to the next party member with
// unspent points, or closes the modal when no member has any. Resets
// the stat-row cursor to StatSTR so each member starts at the top of
// the list. Single seam for the three previously-duplicated branches
// that did "find next pending member, else close."
func advanceLevelUpMember(g *core.GameState) {
	next := core.FirstPendingLevelUp(g.Party)
	if next < 0 {
		g.LevelUpOpen = false
		g.LevelUpStat = core.StatSTR
		return
	}
	g.LevelUpMember = next
	g.LevelUpStat = core.StatSTR
}

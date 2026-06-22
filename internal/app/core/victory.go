package core

// Victory spoils-screen timing math. Pure helpers turning seconds since
// BattleWon began (Battle.VictoryElapsed) into the screen's animated values.
// Shared by the battle update loop (rings audio cues) and the renderer (draws
// bars/rows) so the two can't drift. Pacing consts live in config.go.

// VictoryFillProgress is the eased XP-bar fill fraction at `elapsed` seconds:
// 0 until the pose (VictoryDanceBeat) ends, then EASE-OUT to 1 across
// VictoryBarFillDuration. Gold ticker + XP tick-sound ride the same curve.
// Ease-out QUAD (not cubic) over a shorter window so the tail doesn't crawl.
func VictoryFillProgress(elapsed float32) float32 {
	t := (elapsed - VictoryDanceBeat) / VictoryBarFillDuration
	if t <= 0 {
		return 0
	}
	if t >= 1 {
		return 1
	}
	// Ease-out quad: 1 - (1-t)^2.
	inv := 1 - t
	return 1 - inv*inv
}

// VictorySpoilsAnimEnd is the elapsed time the fill animation finishes — the
// Confirm-skip target, after which the Continue prompt shows.
func VictorySpoilsAnimEnd() float32 {
	return VictoryDanceBeat + VictoryBarFillDuration
}

// VictorySpoilsAnimDone reports whether the fill animation has finished.
func VictorySpoilsAnimDone(elapsed float32) bool {
	return elapsed >= VictorySpoilsAnimEnd()
}

// VictoryLootRevealed returns how many of `n` loot rows have slid in by
// `elapsed` seconds — one per VictoryLootStagger starting at the dance beat's
// end. Render reveals row i once this exceeds i; the update loop rings SoundItemGet.
func VictoryLootRevealed(elapsed float32, n int) int {
	if n <= 0 {
		return 0
	}
	since := elapsed - VictoryDanceBeat
	if since < 0 {
		return 0
	}
	shown := int(since/VictoryLootStagger) + 1
	if shown > n {
		shown = n
	}
	return shown
}

// AddedAt is the continuous XP this member has gained by fill fraction p
// (0..1). Single source of "XP shown right now" for the bar, tick counters,
// and level-up cue, so they can't disagree.
func (ms MemberSpoils) AddedAt(p float32) float32 {
	return float32(ms.GainedXP) * p
}

// ProjectAt returns the member's animated level / within-level remainder /
// levels gained at fill fraction p (AddedAt through ProjectXP). Shared by the
// XP-bar draw and the level-up SFX counter so they stay locked together.
func (ms MemberSpoils) ProjectAt(p float32) (lvl, xp, gained int) {
	return ProjectXP(ms.BeforeLvl, ms.BeforeXP, int(ms.AddedAt(p)))
}

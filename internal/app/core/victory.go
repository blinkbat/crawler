package core

// Victory spoils-screen timing math. Pure helpers turning seconds since
// BattleWon (Battle.VictoryElapsed) into animated values, shared by the battle
// loop + renderer so they can't drift. Pacing consts live in config.go.

// VictoryFillProgress is the XP-bar fill fraction: 0 until the pose
// (VictoryDanceBeat) ends, then EASE-OUT QUAD to 1 across VictoryBarFillDuration.
func VictoryFillProgress(elapsed float32) float32 {
	t := (elapsed - VictoryDanceBeat) / VictoryBarFillDuration
	if t <= 0 {
		return 0
	}
	if t >= 1 {
		return 1
	}
	return EaseOutQuad(t)
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

// VictoryLootRevealed returns how many of `n` loot rows have slid in — one per
// VictoryLootStagger from the dance beat's end. Render reveals row i once this exceeds i.
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

// AddedAt is the continuous XP gained by fill fraction p (0..1) — the single
// source of "XP shown right now" for the bar, tick counters, and level-up cue.
func (ms MemberSpoils) AddedAt(p float32) float32 {
	return float32(ms.GainedXP) * p
}

// ProjectAt returns animated level / within-level remainder / levels gained at
// fill fraction p (AddedAt through ProjectXP). Shared by the bar draw + SFX counter.
func (ms MemberSpoils) ProjectAt(p float32) (lvl, xp, gained int) {
	return ProjectXP(ms.BeforeLvl, ms.BeforeXP, int(ms.AddedAt(p)))
}

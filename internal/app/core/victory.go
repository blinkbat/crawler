package core

// Victory spoils-screen timing math. These pure helpers turn the seconds
// elapsed since the BattleWon phase began (Battle.VictoryElapsed) into the
// values the screen animates with: the 0..1 XP-bar fill fraction, the
// "is the animation finished?" gate, and how many loot rows have revealed.
// Shared by the battle update loop (battle.updateVictorySpoils, which rings
// the audio cues) and the renderer (render.DrawVictorySpoils, which draws
// the bars/rows) so the two can't drift on pacing. Pacing is set by the
// VictoryDanceBeat / VictoryBarFillDuration / VictoryLootStagger constants
// in config.go.

// VictoryFillProgress is the eased XP-bar fill fraction at `elapsed`
// seconds: 0 until the victory pose (VictoryDanceBeat) ends, then EASE-OUT
// to 1 across VictoryBarFillDuration — a quick rush of XP that decelerates
// into its final resting value (the satisfying count-up feel). The gold
// ticker and XP tick-sound cadence ride the same curve, so they cluster
// early and settle together. Uses ease-out QUAD (not cubic) over a shorter
// window so the tail doesn't crawl — the final tick lands promptly instead
// of creeping in for the last few XP.
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

// VictorySpoilsAnimEnd is the elapsed time at which the fill animation is
// fully done — the point a Confirm "skip" snaps to and after which the
// Continue prompt shows.
func VictorySpoilsAnimEnd() float32 {
	return VictoryDanceBeat + VictoryBarFillDuration
}

// VictorySpoilsAnimDone reports whether the fill animation has finished.
func VictorySpoilsAnimDone(elapsed float32) bool {
	return elapsed >= VictorySpoilsAnimEnd()
}

// VictoryLootRevealed returns how many of `n` loot rows have slid in by
// `elapsed` seconds — rows cascade one per VictoryLootStagger starting at
// the end of the dance beat. The renderer reveals row i once this count
// exceeds i; the update loop rings SoundItemGet as the count climbs.
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

// AddedAt is the (continuous) XP this member has gained by fill fraction p
// (0..1). The single source for "how much XP is shown right now" that the
// spoils screen's bar, the gold/XP tick counters, and the level-up cue all
// read, so they can't disagree on the in-flight amount.
func (ms MemberSpoils) AddedAt(p float32) float32 {
	return float32(ms.GainedXP) * p
}

// ProjectAt returns the member's animated level / within-level remainder /
// levels gained at fill fraction p — AddedAt run through ProjectXP. Shared by
// the XP-bar draw (render) and the level-up SFX counter (battle) so the bar
// and the cue stay locked to the same projection.
func (ms MemberSpoils) ProjectAt(p float32) (lvl, xp, gained int) {
	return ProjectXP(ms.BeforeLvl, ms.BeforeXP, int(ms.AddedAt(p)))
}

package render

import "crawler/internal/app/audio"

// Jukebox is the pause menu's sound tester. The current entry is what the
// label advertises and what Play emits on the next confirm; after Play
// fires, the index advances so successive confirms walk the whole bank.
//
// State is package-level because the jukebox is purely UI — it doesn't
// belong on GameState (which would entangle save/load with HUD scratch
// state) and doesn't need per-game-instance scoping either. raylib's
// draw loop is single-threaded, so the bare int doesn't need a mutex.
var jukeboxIndex int

// JukeboxRowLabel is the "Jukebox: SoundName" string shown on the pause
// menu's jukebox row. Names come from the audio package so adding a new
// cue surfaces here automatically (provided audio.soundNames also gains
// an entry — checked at compile time by the array size).
// currentJukeboxIndex normalizes jukeboxIndex against the live sound count,
// wrapping a stale/out-of-range cursor back to 0, and reports ok=false when
// the bank is empty. Both the label and the play path read the cursor through
// here so the clamp can't drift between them.
func currentJukeboxIndex() (idx, count int, ok bool) {
	count = audio.SoundCount()
	if count <= 0 {
		return 0, 0, false
	}
	if jukeboxIndex < 0 || jukeboxIndex >= count {
		jukeboxIndex = 0
	}
	return jukeboxIndex, count, true
}

func JukeboxRowLabel() string {
	idx, _, ok := currentJukeboxIndex()
	if !ok {
		return "Jukebox: (empty)"
	}
	return "Jukebox: " + audio.SoundName(audio.Sound(idx))
}

// PlayJukebox plays the currently-advertised sound and advances the
// jukebox cursor to the next entry, wrapping at the end of the bank. The
// label re-reads jukeboxIndex on the next frame so the player sees what
// they'll get on the next press.
func PlayJukebox() {
	idx, count, ok := currentJukeboxIndex()
	if !ok {
		return
	}
	audio.Play(audio.Sound(idx))
	jukeboxIndex = (idx + 1) % count
}

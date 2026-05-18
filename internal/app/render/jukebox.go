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
func JukeboxRowLabel() string {
	count := audio.SoundCount()
	if count <= 0 {
		return "Jukebox: (empty)"
	}
	if jukeboxIndex < 0 || jukeboxIndex >= count {
		jukeboxIndex = 0
	}
	return "Jukebox: " + audio.SoundName(audio.Sound(jukeboxIndex))
}

// PlayJukebox plays the currently-advertised sound and advances the
// jukebox cursor to the next entry, wrapping at the end of the bank. The
// label re-reads jukeboxIndex on the next frame so the player sees what
// they'll get on the next press.
func PlayJukebox() {
	count := audio.SoundCount()
	if count <= 0 {
		return
	}
	if jukeboxIndex < 0 || jukeboxIndex >= count {
		jukeboxIndex = 0
	}
	audio.Play(audio.Sound(jukeboxIndex))
	jukeboxIndex = (jukeboxIndex + 1) % count
}

package render

import "crawler/internal/app/audio"

// jukeboxIndex is the pause-menu sound tester's cursor. Package-level: purely UI,
// doesn't belong on GameState. raylib's draw loop is single-threaded, so no mutex.
var jukeboxIndex int

// currentJukeboxIndex normalizes jukeboxIndex against the live sound count
// (wrapping stale/out-of-range to 0), ok=false when the bank is empty. Both label
// and play path read through here so the clamp can't drift.
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

// JukeboxRowLabel is the "Jukebox: SoundName" string for the pause menu's jukebox row.
func JukeboxRowLabel() string {
	idx, _, ok := currentJukeboxIndex()
	if !ok {
		return "Jukebox: (empty)"
	}
	return "Jukebox: " + audio.SoundName(audio.Sound(idx))
}

// PlayJukebox plays the current sound and advances the cursor (wrapping at the bank end).
func PlayJukebox() {
	idx, count, ok := currentJukeboxIndex()
	if !ok {
		return
	}
	audio.Play(audio.Sound(idx))
	jukeboxIndex = (idx + 1) % count
}

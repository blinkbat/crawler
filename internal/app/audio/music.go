package audio

import (
	"crawler/internal/app/audio/userconfig"
	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// The two looping BGM tracks, streamed from maps/music/ (transcoded to OGG +
// loudness-matched — raylib's music streaming doesn't read the authored .m4a, and
// both are normalized to ~-16 LUFS so neither is louder across the crossfade). A
// missing file just means that track is silent (loadTrack no-ops).
const (
	bgExploreFile = "bg_explore.ogg" // non-battle exploration theme
	bgBattleFile  = "bg_battle.ogg"  // battle theme
)

// Music fade rates (per second). The explore theme SWELLS in slowly the first time
// you start exploring (from silence); battle enter/exit is a quicker equal-rate
// CROSSFADE between the two themes so combat music arrives promptly. UpdateMusic
// picks swell-vs-crossfade per direction.
const (
	musicSwellInPerSec   = float32(0.133) // 0→1 over ~7.5s — gentle first-explore intro
	musicCrossfadePerSec = float32(0.367) // ~2.7s explore↔battle crossfade
	// musicSilentGainEps: below this gain the battle track counts as silent, so the
	// explore theme rising from it gets the slow swell rather than the crossfade rate.
	musicSilentGainEps = float32(0.05)
)

// volumeChannel is one 0..1 slider level plus the shared effective/get/set logic.
// `effective` returns 0 while the master mute is on (the slider value is preserved
// so un-muting restores it). The SFX re-push to the live bank is NOT a field here
// (it would read sfx back, an init cycle) — SetSFXVolume/SetMuted call applySFXVolume.
type volumeChannel struct{ value float32 }

func (c volumeChannel) effective() float32 {
	if muted {
		return 0
	}
	return c.value
}

func (c *volumeChannel) set(v float32) { c.value = core.Clamp(v, 0, 1) }

// Volume settings (0..1), loaded from disk at Init and persisted on change. music
// scales the streamed track; sfx scales every bank Sound; muted is a master kill
// that forces BOTH to silence. Same single-goroutine contract as the bank (no lock).
// Values are seeded by loadVolumeSettings() in Init before any audio plays — no
// struct-literal defaults (they'd be dead).
var (
	music volumeChannel
	sfx   volumeChannel
	muted bool
)

// musicTrack is one looping BGM stream + its eased crossfade gain (0..1). The live
// stream volume is gain*music.effective(); the two tracks crossfade by easing toward
// opposite targets at the same rate.
type musicTrack struct {
	stream rl.Music
	loaded bool
	gain   float32
}

// exploreTrack (bg_explore) plays while exploring; battleTrack (bg_battle) plays in
// combat. Both stream + loop continuously once loaded; UpdateMusic crossfades their
// gains so entering/exiting battle swaps which is audible.
var (
	exploreTrack musicTrack
	battleTrack  musicTrack
)

// loadVolumeSettings pulls the persisted volumes + mute (or defaults) into the
// globals. Call once at Init before applySFXVolume / the first UpdateMusic.
func loadVolumeSettings() {
	music.value, sfx.value, muted = userconfig.LoadVolumes()
}

// applySFXVolume pushes the effective SFX volume (0 while muted) onto every loaded
// bank Sound. Call after loadBank and on any SFX-volume/mute change; zero-buffer
// (unloaded) slots are skipped.
func applySFXVolume() {
	vol := sfx.effective()
	for i := range bank {
		if soundLoaded(bank[i]) {
			rl.SetSoundVolume(bank[i], vol)
		}
	}
}

// loadTrack loads + begins one looping track, silent (UpdateMusic fades it in). No-op
// if the file is missing / fails to decode, so a missing track degrades to silence.
func loadTrack(t *musicTrack, file string) {
	stream := rl.LoadMusicStream(userconfig.MusicTrackPath(file))
	if !musicLoaded(stream) {
		return // missing/unsupported file — leave t.loaded false
	}
	stream.Looping = true
	t.stream = stream
	t.loaded = true
	t.gain = 0
	rl.SetMusicVolume(stream, 0)
	rl.PlayMusicStream(stream)
}

// startMusic loads both looping BGM tracks (silent; UpdateMusic fades them in/out).
func startMusic() {
	loadTrack(&exploreTrack, bgExploreFile)
	loadTrack(&battleTrack, bgBattleFile)
}

// stopMusic unloads both BGM streams (Close path). Safe if never loaded.
func stopMusic() {
	for _, t := range []*musicTrack{&exploreTrack, &battleTrack} {
		if t.loaded {
			rl.UnloadMusicStream(t.stream)
		}
		*t = musicTrack{}
	}
}

// updateTrack feeds one track's buffer and eases its gain toward target at rate,
// then sets the live stream volume (gain * the music master volume). No-op if unloaded.
func updateTrack(t *musicTrack, target, rate, dt float32) {
	if !t.loaded {
		return
	}
	rl.UpdateMusicStream(t.stream)
	t.gain = core.Approach(t.gain, target, rate*dt)
	rl.SetMusicVolume(t.stream, t.gain*music.effective())
}

// UpdateMusic MUST run every frame: it feeds both stream buffers and crossfades the
// two themes. wantMusic is true while exploring (incl. battle) — false on the title
// and in the editor, so both fade to silence there. inBattle picks WHICH theme is
// audible: entering/exiting battle eases the explore + battle gains toward opposite
// targets at the crossfade rate (a swap), while the FIRST explore entry from silence
// swells the explore theme in slowly. No-op when the device failed.
func UpdateMusic(dt float32, wantMusic, inBattle bool) {
	if !ready {
		return
	}
	exploreTarget, battleTarget := float32(0), float32(0)
	if wantMusic {
		if inBattle {
			battleTarget = 1
		} else {
			exploreTarget = 1
		}
	}
	// Explore theme rising from silence (title→explore, battle track idle) gets the
	// slow swell; every other move — including the battle crossfades — uses the quicker
	// equal crossfade rate so the two themes trade evenly.
	exploreRate := musicCrossfadePerSec
	if exploreTarget > exploreTrack.gain && battleTrack.gain < musicSilentGainEps {
		exploreRate = musicSwellInPerSec
	}
	updateTrack(&exploreTrack, exploreTarget, exploreRate, dt)
	updateTrack(&battleTrack, battleTarget, musicCrossfadePerSec, dt)
}

// MusicVolume / SFXVolume report the current settings (0..1) for the Sound menu's
// gauge fill + label.
func MusicVolume() float32 { return music.value }
func SFXVolume() float32   { return sfx.value }

// SetMusicVolume sets the music level (clamped 0..1). The live stream picks it up on
// the next UpdateMusic (scaled by the current fade). Does NOT persist — call
// SaveVolumeSettings when the player closes the menu.
func SetMusicVolume(v float32) { music.set(v) }

// SetSFXVolume sets the SFX level (clamped 0..1) and applies it to the live bank at
// once, so a slider nudge is audible on the next cue. Does NOT persist.
func SetSFXVolume(v float32) {
	sfx.set(v)
	applySFXVolume()
}

// Muted reports the master mute state (for the Sound menu's On/Off label).
func Muted() bool { return muted }

// SetMuted flips the master mute and applies it live: SFX is re-pushed to the bank at
// once (0 while muted), and music picks up the effective volume on the next
// UpdateMusic. The slider values are untouched, so un-muting restores them. Does NOT
// persist — SaveVolumeSettings (menu close) writes it.
func SetMuted(m bool) {
	muted = m
	applySFXVolume()
}

// ToggleMute flips mute (the Sound menu's Mute row).
func ToggleMute() { SetMuted(!muted) }

// SaveVolumeSettings persists the current music + SFX volumes + mute (call when the
// Sound menu closes). Best-effort: a write failure just means they reset next launch.
func SaveVolumeSettings() { _ = userconfig.SaveVolumes(music.value, sfx.value, muted) }

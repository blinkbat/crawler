// Package audio is the project's sound bank. The bank's defaults are
// procedural — a handful of short cues (input hit / input miss / heal /
// enemy hit / enemy death) generated at startup from sine waves and
// envelopes via the wavsynth subpackage. The editor's Sounds modal can
// save user-recorded / user-tuned .wav files under maps/sounds/ and
// rebind any built-in cue to one of those files via maps/sounds/
// assignments.txt (see the userconfig subpackage). On Init the bank
// synthesizes its procedural defaults, then overlays the persistent
// user assignments — so the repo ships asset-free but per-user
// customizations land on disk and survive restarts.
//
// The pure synthesis (sine sweeps, chord sums, WAV header building)
// lives in the wavsynth subpackage; the pure filesystem / parser logic
// lives in the userconfig subpackage. Both are split out so they can be
// unit-tested without pulling raylib into the test binary's load path.
// This file owns the raylib-side lifecycle: device init, sound bank
// assembly, playback, teardown. user.go owns the raylib-side preview
// ring and bank-overlay reload.
//
// Usage:
//
//	audio.Init()    // call once during app startup (after rl.InitWindow)
//	defer audio.Close()
//
//	audio.Play(audio.SoundInputHit)
//
// All Play calls are no-ops when the audio device failed to initialize, so
// callers don't need to guard. Sounds use a single shared playback channel
// per Sound — rapid re-presses cut the previous instance short, which is
// the right behavior for short input clicks (no buildup).
package audio

import (
	"crawler/internal/app/audio/wavsynth"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Sound identifies one entry in the procedural bank. Add a new enum value,
// load it inside loadBank, and any caller can Play it.
type Sound int

const (
	SoundInputHit   Sound = iota // Nice / Good landed
	SoundInputGreat              // Great / Excellent landed — brighter
	SoundInputMiss               // timing window missed
	SoundHeal                    // Prayer landed a heal
	SoundEnemyHit                // any damage application on an enemy
	SoundEnemyDeath              // enemy HP hit zero
	soundCount
)

// soundMeta is the single per-Sound metadata table — display label
// (shown in UI like the pause-menu jukebox) and canonical slug (used as
// the key in maps/sounds/assignments.txt). Both forms derive from this
// one table so adding a new Sound enum entry requires editing one row
// instead of keeping two parallel tables in lockstep.
var soundMeta = [soundCount]struct {
	Display   string
	Canonical string
}{
	SoundInputHit:   {"Input Hit", "input_hit"},
	SoundInputGreat: {"Input Great", "input_great"},
	SoundInputMiss:  {"Input Miss", "input_miss"},
	SoundHeal:       {"Heal", "heal"},
	SoundEnemyHit:   {"Enemy Hit", "enemy_hit"},
	SoundEnemyDeath: {"Enemy Death", "enemy_death"},
}

// SoundName returns the display label for a sound. Out-of-range values
// fall back to "Unknown" so a future enum addition that's missed in
// soundMeta still renders without a panic.
func SoundName(s Sound) string {
	if s < 0 || s >= soundCount {
		return "Unknown"
	}
	return soundMeta[s].Display
}

// SoundCount is the number of cues in the bank. Used by callers that
// cycle through every entry (jukebox) without hardcoding the iota tail.
func SoundCount() int {
	return int(soundCount)
}

var (
	bank  [soundCount]rl.Sound
	ready bool
)

// Init brings up the raylib audio device and synthesizes every sound in the
// bank. Safe to call more than once — subsequent calls no-op. Audio device
// failures leave ready=false so Play becomes a silent fallback rather than
// crashing on systems without a working audio stack.
//
// loadBank runs inside a panic-recover so a partial bank build (e.g. a
// future cue that loads from disk and fails mid-way) doesn't leak the
// entries that already loaded — the recovery path unloads what's there
// and shuts the device down, leaving ready=false. Today's pure-synth
// cues can't fail, but the guard is cheap and keeps the contract honest.
func Init() {
	if ready {
		return
	}
	rl.InitAudioDevice()
	if !rl.IsAudioDeviceReady() {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			unloadBank()
			rl.CloseAudioDevice()
			ready = false
			panic(r)
		}
	}()
	loadBank()
	ready = true
	// Overlay any user-saved assignments (maps/sounds/assignments.txt)
	// on top of the procedural bank. Missing files / malformed lines are
	// skipped silently at startup — built-in cues stay as the procedural
	// defaults. Failed cue slugs come back via ReloadUserAssignments's
	// first return but we don't surface them at boot since there's no
	// HUD yet to show a warning; the editor's Sounds modal will pick
	// them up via its explicit reload on Assign.
	_, _ = ReloadUserAssignments()
}

// Close unloads the bank and shuts the audio device down. Mirrors Init —
// safe to call when audio never came up.
func Close() {
	if !ready {
		return
	}
	unloadBank()
	unloadPreviewRing()
	rl.CloseAudioDevice()
	ready = false
}

// unloadBank releases every loaded entry and zeros the bank array. Guards
// against zero-value entries (nil Stream.Buffer) so calling on a partial
// load — the Init recover path's case — doesn't segfault inside raylib's
// C-side UnloadSound. Shared by Init's recover branch and Close so the
// "what does cleanup look like" answer lives in one place.
func unloadBank() {
	for _, s := range bank {
		if s.Stream.Buffer == nil {
			continue
		}
		rl.UnloadSound(s)
	}
	bank = [soundCount]rl.Sound{}
}

// Play fires the named sound. No-ops if audio isn't ready or the enum is
// out of range — callers can fire-and-forget.
func Play(id Sound) {
	if !ready || id < 0 || id >= soundCount {
		return
	}
	rl.PlaySound(bank[id])
}

// loadBank synthesizes every sound. Each entry has its own envelope and
// frequency shape so the cues read distinctly without overlapping.
func loadBank() {
	// Input hit: short ascending blip — 540 Hz up to 760 Hz over 70 ms,
	// soft attack, fast release. Subtle, "click" feel.
	bank[SoundInputHit] = pcmToSound(wavsynth.SynthSweep(0.07, 540, 760, 0.20, 0.005, 0.04))

	// Input great: brighter and a touch longer — sweeps up to 1.3 kHz with
	// a bell envelope so it rings. Higher volume so Excellents announce
	// themselves a hair louder than a regular Nice.
	bank[SoundInputGreat] = pcmToSound(wavsynth.SynthChord(0.14, []float64{660, 990, 1320}, 0.22))

	// Input miss: low, downward sweep — 200 Hz dropping to 130 Hz over
	// 130 ms. Reads as "deflated" without being annoying.
	bank[SoundInputMiss] = pcmToSound(wavsynth.SynthSweep(0.13, 200, 130, 0.18, 0.005, 0.07))

	// Heal: two-note chime, 520 Hz then 780 Hz, total ~180 ms. Each note
	// gets its own bell envelope so the seam between them is audible.
	bank[SoundHeal] = pcmToSound(wavsynth.SynthChime(0.09, 520, 780, 0.18))

	// Enemy hit: tiny thud — 220 Hz, 50 ms, sharp attack. Pure punctuation.
	bank[SoundEnemyHit] = pcmToSound(wavsynth.SynthSweep(0.05, 220, 180, 0.22, 0.001, 0.02))

	// Enemy death: descending sweep — 380 Hz down to 90 Hz over 280 ms,
	// long release tail. Reads as "fading out."
	bank[SoundEnemyDeath] = pcmToSound(wavsynth.SynthSweep(0.28, 380, 90, 0.20, 0.004, 0.14))
}

// pcmToSound builds a 16-bit mono PCM buffer into an rl.Sound via the
// shared bytesToSound path. Used by loadBank to populate the procedural
// defaults; user.go's preview ring takes the same path so we have one
// rl.Sound builder, not two.
func pcmToSound(pcm []int16) rl.Sound {
	if !rl.IsAudioDeviceReady() {
		return rl.Sound{}
	}
	return bytesToSound(wavsynth.BuildWAV(pcm, wavsynth.SampleRate))
}

// bytesToSound hands a pre-encoded WAV byte slice to raylib via
// LoadWaveFromMemory → LoadSoundFromWave. The Wave is unloaded
// immediately — the Sound owns its own copy of the data. Used by both
// pcmToSound (synth path) and playThroughRing (file/preview path) so
// the WAV-bytes-to-rl.Sound conversion lives in one place.
func bytesToSound(wav []byte) rl.Sound {
	wave := rl.LoadWaveFromMemory(".wav", wav, int32(len(wav)))
	snd := rl.LoadSoundFromWave(wave)
	rl.UnloadWave(wave)
	return snd
}

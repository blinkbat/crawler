// Package audio is the project's procedural sound bank. Tiny library by
// design: a handful of short cues — input hit, input miss, heal land,
// enemy hit, enemy death — generated at startup from sine waves and
// envelopes. No audio files on disk; the bank lives entirely in code so
// every sound is editable as numbers, and the repo stays asset-free.
//
// The pure synthesis (sine sweeps, chord sums, WAV header building) lives
// in the wavsynth subpackage so it can be unit-tested without pulling
// raylib into the test binary's load path. This file owns the raylib-side
// lifecycle: device init, sound bank assembly, playback, teardown.
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
	SoundInputHit     Sound = iota // Nice / Good landed
	SoundInputGreat                // Great / Excellent landed — brighter
	SoundInputMiss                 // timing window missed
	SoundHeal                      // Prayer landed a heal
	SoundEnemyHit                  // any damage application on an enemy
	SoundEnemyDeath                // enemy HP hit zero
	soundCount
)

var (
	bank  [soundCount]rl.Sound
	ready bool
)

// Init brings up the raylib audio device and synthesizes every sound in the
// bank. Safe to call more than once — subsequent calls no-op. Audio device
// failures leave ready=false so Play becomes a silent fallback rather than
// crashing on systems without a working audio stack.
func Init() {
	if ready {
		return
	}
	rl.InitAudioDevice()
	if !rl.IsAudioDeviceReady() {
		return
	}
	loadBank()
	ready = true
}

// Close unloads the bank and shuts the audio device down. Mirrors Init —
// safe to call when audio never came up.
func Close() {
	if !ready {
		return
	}
	for _, s := range bank {
		rl.UnloadSound(s)
	}
	rl.CloseAudioDevice()
	ready = false
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

// pcmToSound wraps a slice of 16-bit mono PCM samples in a minimal WAV
// container and hands it to raylib via LoadWaveFromMemory →
// LoadSoundFromWave. The Wave is unloaded immediately — the Sound owns its
// own copy of the data. Returns a zero-value Sound when the audio device
// isn't ready (which Play silently no-ops on).
func pcmToSound(pcm []int16) rl.Sound {
	if !rl.IsAudioDeviceReady() {
		return rl.Sound{}
	}
	buf := wavsynth.BuildWAV(pcm, wavsynth.SampleRate)
	wave := rl.LoadWaveFromMemory(".wav", buf, int32(len(buf)))
	sound := rl.LoadSoundFromWave(wave)
	rl.UnloadWave(wave)
	return sound
}

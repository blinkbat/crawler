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
	"crawler/internal/app/audio/userconfig"
	"crawler/internal/app/audio/wavsynth"
	"fmt"
	"os"

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

// soundCue is one row in the bank: display label (used in UI), the
// canonical slug (used as BOTH the key in maps/sounds/assignments.txt
// AND the on-disk filename stem for the default .wav), and the
// procedural-PCM seeder. One table, three uses — adding a new cue is
// one row and the bank, UI, file seeder, and action map all pick it up.
//
// Why a single table: previous shape kept `soundMeta` (Display +
// Canonical) and `defaultBankPCM()` (Sound → PCM) parallel, with the
// "same-canonical-name → default file" link implicit. Pulling the
// PCM onto the same row makes the link explicit and removes the
// "which file plays for which cue?" guesswork — see loadBank.
type soundCue struct {
	Display   string
	Canonical string
	PCM       func() []int16
}

// PCM-synth arg order (these are positional, so a transposed column is a
// silent timbre change — see wavsynth for the canonical contract):
//   SynthClick(duration, pitchHz, pitchDrop, noise, volume)
//   SynthChord(duration, freqs, volume)
//   SynthChime(noteDuration, firstHz, secondHz, volume)
//   SynthSweep(duration, startHz, endHz, volume, attack, release)
var soundCues = [soundCount]soundCue{
	// Bright UI tick — high pitch, large noise mix, very short.
	SoundInputHit: {Display: "Input Hit", Canonical: "input_hit",
		PCM: func() []int16 { return wavsynth.SynthClick(0.025, 1800, 0.5, 0.6, 0.22) }},
	// Rare reward — keeps tonality so Excellent stands apart from a
	// regular Nice. Stacked harmonics under a quick bell envelope.
	SoundInputGreat: {Display: "Input Great", Canonical: "input_great",
		PCM: func() []int16 { return wavsynth.SynthChord(0.10, []float64{660, 990, 1320}, 0.20) }},
	// Low dull thud — failure registers as weight, not absence.
	SoundInputMiss: {Display: "Input Miss", Canonical: "input_miss",
		PCM: func() []int16 { return wavsynth.SynthClick(0.045, 220, 0.7, 0.25, 0.20) }},
	// Tonal two-note chime — melodic shape IS the cue's identity, so
	// it breaks the percussive theme on purpose.
	SoundHeal: {Display: "Heal", Canonical: "heal",
		PCM: func() []int16 { return wavsynth.SynthChime(0.09, 520, 780, 0.18) }},
	// Tight mid-pitch thwack — moderate noise for impact crunch.
	SoundEnemyHit: {Display: "Enemy Hit", Canonical: "enemy_hit",
		PCM: func() []int16 { return wavsynth.SynthClick(0.035, 380, 0.6, 0.4, 0.24) }},
	// Heavier deep kick-drum thud with longer tail.
	SoundEnemyDeath: {Display: "Enemy Death", Canonical: "enemy_death",
		PCM: func() []int16 { return wavsynth.SynthClick(0.12, 140, 0.85, 0.2, 0.22) }},
}

// soundCues is an array of length soundCount, so a missing row reads
// as the zero value (empty Display / Canonical / nil PCM) instead of
// failing to compile. Verify every row is populated at init —
// otherwise a future Sound enum addition without a corresponding
// soundCues entry would silently ship as a no-op cue. Mirrors the
// convention AGENTS.md notes for timingGrades / statTable /
// partyStatusVisuals.
func init() {
	if len(soundCues) != int(soundCount) {
		panic(fmt.Sprintf("audio: soundCues length %d != soundCount %d", len(soundCues), soundCount))
	}
	for i, row := range soundCues {
		if row.Canonical == "" {
			panic(fmt.Sprintf("audio: soundCues[%d] has empty Canonical — add a row for the new Sound enum value", i))
		}
		if row.PCM == nil {
			panic(fmt.Sprintf("audio: soundCues[%d] (%s) has nil PCM — procedural fallback won't render", i, row.Canonical))
		}
	}
}

// SoundName returns the display label for a sound. Out-of-range values
// fall back to "Unknown" so a future enum addition that's missed in
// soundCues still renders without a panic.
func SoundName(s Sound) string {
	if s < 0 || s >= soundCount {
		return "Unknown"
	}
	return soundCues[s].Display
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
		// The device backend was opened by InitAudioDevice even though it
		// didn't come up usable. ready stays false so Close() (guarded on
		// ready) will never run, so shut the half-open device down here or
		// it leaks for the process lifetime.
		rl.CloseAudioDevice()
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
	// loadBank already reads assignments.txt and loads each cue from its
	// assigned .wav (resolveAssignedFile + loadCueFromDisk), so the user
	// overlay is applied here — no separate ReloadUserAssignments pass at
	// boot (that re-decoded every cue a second time). The editor's Sounds
	// modal still calls ReloadUserAssignments explicitly when an assignment
	// changes mid-session.
	loadBank()
	ready = true
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
	unloadSounds(bank[:])
	bank = [soundCount]rl.Sound{}
}

// unloadSounds releases every non-zero rl.Sound in the slice. Shared by
// unloadBank and unloadPreviewRing, which used to inline the same
// "skip nil-buffer, UnloadSound, zero the slot" loop. Mutates the slice
// in place (slot zeroing) so the buffer pointers can't be read again.
func unloadSounds(slots []rl.Sound) {
	for i := range slots {
		if slots[i].Stream.Buffer == nil {
			continue
		}
		rl.UnloadSound(slots[i])
		slots[i] = rl.Sound{}
	}
}

// forEachCue walks every (non-empty-canonical) row of soundCues, the
// shared scaffold loadBank / ensureBankOnDisk / ReloadUserAssignments
// each used to open-code as `for cue := Sound(0); cue < soundCount; cue++`
// then a `continue` on empty Canonical.
func forEachCue(fn func(cue Sound, row soundCue)) {
	for cue := Sound(0); cue < soundCount; cue++ {
		row := soundCues[cue]
		if row.Canonical == "" {
			continue
		}
		fn(cue, row)
	}
}

// resolveAssignedFile returns the filename a cue should load from given
// the assignments map. Empty assignment falls back to the canonical
// slug; assigned reports whether the player explicitly set an override
// (so callers can flag "assigned file missing" diagnostics).
func resolveAssignedFile(assigns map[string]string, canonical string) (name string, assigned bool) {
	name = assigns[canonical]
	assigned = name != ""
	if name == "" {
		name = canonical
	}
	return name, assigned
}

// Play fires the named sound. No-ops if audio isn't ready or the enum is
// out of range — callers can fire-and-forget.
func Play(id Sound) {
	if !ready || id < 0 || id >= soundCount {
		return
	}
	rl.PlaySound(bank[id])
}

// loadBank is the one path that builds the bank from the on-disk
// state: files + the action map (assignments.txt). Steps:
//
//  1. Ensure every cue's default .wav exists in maps/sounds/. Missing
//     files get written from soundCues[cue].PCM(). Existing files are
//     never clobbered — user edits via the editor's Sounds modal
//     persist across launches.
//  2. Ensure assignments.txt has an entry for every cue. Missing
//     entries default to the cue's canonical name (so cue input_hit
//     maps to input_hit.wav by default). The action map is then
//     authoritative: every bank slot reads from assignments[slug].
//  3. Load each cue's rl.Sound from its assigned file.
//
// Why this shape: the previous arrangement kept the procedural PCM,
// the disk seeder, and the assignments overlay as three separate
// concepts with implicit "same-canonical-name" fallbacks weaving
// between them. Now there are exactly two pieces of state — files
// and the action map — and both are on disk and user-editable.
//
// In-memory fallback: if a cue's assigned file can't be read at
// runtime (race, permissions, manual mid-process delete), the cue
// falls back to its freshly-synthesized PCM so Play(cue) never
// silently breaks even when the filesystem misbehaves.
func loadBank() {
	ensureBankOnDisk()
	assigns := userconfig.LoadAssignments()
	forEachCue(func(cue Sound, row soundCue) {
		fileName, _ := resolveAssignedFile(assigns, row.Canonical)
		bank[cue] = loadCueFromDisk(fileName, row.PCM)
	})
}

// ensureBankOnDisk writes any missing default .wav files and adds an
// entry to assignments.txt for any cue whose slug isn't already
// listed. After this call, every cue has both a file on disk and a
// row in the action map — no implicit "default" anywhere. Errors
// (read-only filesystem, no permission) are swallowed because the
// in-memory PCM fallback in loadCueFromDisk still covers playback.
func ensureBankOnDisk() {
	assigns := userconfig.LoadAssignments()
	assignsChanged := false
	forEachCue(func(_ Sound, row soundCue) {
		if row.PCM == nil {
			return
		}
		// File: write the default if no .wav lives at the canonical
		// name yet. Existing user-edited content is preserved.
		path := UserSoundPath(row.Canonical)
		if _, err := os.Stat(path); err != nil {
			_, _ = userconfig.WriteWAV(row.Canonical, row.PCM())
		}
		// Action map: backfill the entry if assignments.txt is missing
		// this cue. Same-name default means cue's slug → cue's slug.
		if _, ok := assigns[row.Canonical]; !ok {
			assigns[row.Canonical] = row.Canonical
			assignsChanged = true
		}
	})
	if assignsChanged {
		_ = userconfig.SaveAssignments(assigns)
	}
}

// readOrSynthSound resolves one cue's rl.Sound: it reads the named .wav
// from disk and, failing that (deleted mid-run, permission revoked),
// rebuilds from the supplied PCM closure so Play(cue) never goes silent
// on transient filesystem errors. fromFile reports which branch ran so
// callers that need to flag "assigned file missing" (ReloadUserAssignments)
// can. pcmFn nil-checks because a future malformed soundCues entry
// shouldn't crash the bank. Single source for the disk-or-synth rule
// shared by loadBank (via loadCueFromDisk) and the assignments reload.
func readOrSynthSound(name string, pcmFn func() []int16) (snd rl.Sound, fromFile bool) {
	if data, err := os.ReadFile(UserSoundPath(name)); err == nil {
		return bytesToSound(data), true
	}
	if pcmFn == nil {
		return rl.Sound{}, false
	}
	return pcmToSound(pcmFn()), false
}

// loadCueFromDisk is the discard-the-flag form used by loadBank.
func loadCueFromDisk(name string, pcmFn func() []int16) rl.Sound {
	snd, _ := readOrSynthSound(name, pcmFn)
	return snd
}

// replaceSound swaps the sound at *slot for next, unloading the old
// buffer first (guarded against the zero-value Sound) so a reassignment
// can't leak raylib's C-side audio buffer. Shared by the bank reload and
// the preview ring, which each open-coded the guard-then-unload dance.
func replaceSound(slot *rl.Sound, next rl.Sound) {
	if slot.Stream.Buffer != nil {
		rl.UnloadSound(*slot)
	}
	*slot = next
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

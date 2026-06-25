// Package audio is the project's sound bank. Defaults are procedural cues
// synthesized at startup (wavsynth); the editor's Sounds modal can save
// .wav files under maps/sounds/ and rebind cues via assignments.txt
// (userconfig). Init synthesizes defaults then overlays user assignments.
//
// wavsynth (pure synthesis) and userconfig (pure filesystem/parser) are
// split out so they unit-test without raylib on the load path. This file
// owns the raylib lifecycle (device init, bank assembly, playback,
// teardown); user.go owns the preview ring and bank-overlay reload.
//
//	audio.Init()    // once at startup, after rl.InitWindow
//	defer audio.Close()
//	audio.Play(audio.SoundInputHit)
//
// Play is a no-op when the device failed to init. One shared channel per
// Sound — rapid re-presses cut the previous instance (right for clicks).
package audio

import (
	"crawler/internal/app/audio/userconfig"
	"crawler/internal/app/audio/wavsynth"
	"fmt"
	"os"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Sound identifies one entry in the procedural bank.
type Sound int

const (
	SoundInputHit   Sound = iota // Nice / Good landed
	SoundInputGreat              // Great / Excellent landed — brighter
	SoundInputMiss               // timing window missed
	SoundHeal                    // Prayer landed a heal
	SoundEnemyHit                // any damage application on an enemy
	SoundEnemyDeath              // enemy HP hit zero
	SoundVictory                 // battle won — triumphant fanfare
	SoundLevelUp                 // a member crossed a level on the spoils screen
	SoundItemGet                 // a loot row revealed on the spoils screen
	SoundXPTick                  // subtle blip as the spoils XP bars count up
	soundCount
)

// soundCue is one bank row: Display label (UI), Canonical slug (BOTH the
// assignments.txt key AND the default .wav filename stem), and the PCM
// seeder. One row feeds bank, UI, file seeder, and action map.
type soundCue struct {
	Display   string
	Canonical string
	PCM       func() []int16
}

// Synth args are positional — a transposed column is a silent timbre
// change (see wavsynth for the contract):
//
//	SynthClick(duration, pitchHz, pitchDrop, noise, volume)
//	SynthChord(duration, freqs, volume)
//	SynthChime(noteDuration, firstHz, secondHz, volume)
//	SynthSweep(duration, startHz, endHz, volume, attack, release)
//	SynthWhistleTrill(duration, startHz, endHz, volume)
var soundCues = [soundCount]soundCue{
	// Bright UI tick.
	SoundInputHit: {Display: "Input Hit", Canonical: "input_hit",
		PCM: func() []int16 { return wavsynth.SynthClick(0.025, 1800, 0.5, 0.6, 0.22) }},
	// SMRPG-style trill whistle — a "tweet!" that stands apart from a Nice.
	SoundInputGreat: {Display: "Input Great", Canonical: "input_great",
		PCM: func() []int16 { return wavsynth.SynthWhistleTrill(0.16, 1200, 1800, 0.2) }},
	// Low dull thud.
	SoundInputMiss: {Display: "Input Miss", Canonical: "input_miss",
		PCM: func() []int16 { return wavsynth.SynthClick(0.045, 220, 0.7, 0.25, 0.20) }},
	// Two-note chime — melodic shape is the cue's identity; breaks the percussive theme.
	SoundHeal: {Display: "Heal", Canonical: "heal",
		PCM: func() []int16 { return wavsynth.SynthChime(0.09, 520, 780, 0.18) }},
	// Tight mid-pitch thwack.
	SoundEnemyHit: {Display: "Enemy Hit", Canonical: "enemy_hit",
		PCM: func() []int16 { return wavsynth.SynthClick(0.035, 380, 0.6, 0.4, 0.24) }},
	// Deep kick-drum thud, longer tail.
	SoundEnemyDeath: {Display: "Enemy Death", Canonical: "enemy_death",
		PCM: func() []int16 { return wavsynth.SynthClick(0.12, 140, 0.85, 0.2, 0.22) }},
	// Win fanfare — C-major chord rung longer than any combat cue.
	SoundVictory: {Display: "Victory", Canonical: "victory",
		PCM: func() []int16 { return wavsynth.SynthChord(0.45, []float64{523, 659, 784, 1047}, 0.22) }},
	// Level-up flourish — rising sweep, distinct from the win chord.
	SoundLevelUp: {Display: "Level Up", Canonical: "level_up",
		PCM: func() []int16 { return wavsynth.SynthSweep(0.30, 440, 880, 0.20, 0.02, 0.10) }},
	// Loot pickup — short bright pop.
	SoundItemGet: {Display: "Item Get", Canonical: "item_get",
		PCM: func() []int16 { return wavsynth.SynthClick(0.05, 1200, 0.4, 0.3, 0.20) }},
	// Count-up blip — short, quiet, high; reads as a tick not a tone.
	SoundXPTick: {Display: "XP Tick", Canonical: "xp_tick",
		PCM: func() []int16 { return wavsynth.SynthClick(0.014, 2600, 0.4, 0.1, 0.08) }},
}

// A missing soundCues row is the zero value (nil PCM), not a compile
// error — assert every row is populated so a new Sound enum value can't
// ship as a silent no-op. Mirrors timingGrades / statTable (AGENTS.md).
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

// SoundName returns the display label; out-of-range falls back to "Unknown".
func SoundName(s Sound) string {
	if s < 0 || s >= soundCount {
		return "Unknown"
	}
	return soundCues[s].Display
}

// SoundCount is the number of cues in the bank.
func SoundCount() int {
	return int(soundCount)
}

// CONTRACT: bank, ready, and the preview ring (user.go) are unsynchronized
// globals — every accessor MUST run on the single game goroutine. No lock, so
// a reload's UnloadSound racing a PlaySound on the same slot is a use-after-free
// in raylib's C side. Add a mutex before driving audio off-thread.
var (
	bank  [soundCount]rl.Sound
	ready bool
)

// Init brings up the audio device and synthesizes the bank. Safe to call
// repeatedly (subsequent calls no-op). Device failures leave ready=false so
// Play degrades to silence instead of crashing.
//
// loadBank runs under panic-recover so a partial build doesn't leak loaded
// entries; the recover path unloads, shuts the device down, and leaves
// ready=false. The panic is SWALLOWED (not re-raised) — Run calls Init with
// no recover, and re-panicking would violate the silent-fallback contract.
func Init() {
	if ready {
		return
	}
	rl.InitAudioDevice()
	if !rl.IsAudioDeviceReady() {
		// Backend opened but unusable; ready stays false so Close never runs.
		// Shut the half-open device down here or it leaks for the process life.
		rl.CloseAudioDevice()
		return
	}
	defer func() {
		if r := recover(); r != nil {
			// Clean up and return ready=false. Do NOT re-panic (see doc above).
			unloadBank()
			rl.CloseAudioDevice()
			ready = false
		}
	}()
	// loadBank applies the user overlay (reads assignments.txt, loads each
	// assigned .wav) — no separate ReloadUserAssignments pass at boot. The
	// editor still calls ReloadUserAssignments on a mid-session change.
	loadBank()
	ready = true
}

// Close unloads the bank and shuts the device down. Safe when audio never came up.
func Close() {
	if !ready {
		return
	}
	unloadBank()
	unloadPreviewRing()
	rl.CloseAudioDevice()
	ready = false
}

// unloadBank releases every loaded entry and zeros the bank. Guards zero-value
// entries so a partial load (Init's recover path) doesn't segfault in raylib.
func unloadBank() {
	unloadSounds(bank[:])
	bank = [soundCount]rl.Sound{}
}

// unloadSounds releases every non-zero rl.Sound in the slice, zeroing each slot
// in place so the buffer pointers can't be read again.
func unloadSounds(slots []rl.Sound) {
	for i := range slots {
		if slots[i].Stream.Buffer == nil {
			continue
		}
		rl.UnloadSound(slots[i])
		slots[i] = rl.Sound{}
	}
}

// forEachCue walks every soundCues row with a non-empty Canonical.
func forEachCue(fn func(cue Sound, row soundCue)) {
	for cue := Sound(0); cue < soundCount; cue++ {
		row := soundCues[cue]
		if row.Canonical == "" {
			continue
		}
		fn(cue, row)
	}
}

// resolveAssignedFile returns the filename a cue loads from. Empty assignment
// falls back to the canonical slug; assigned reports an explicit override (so
// callers can flag "assigned file missing").
func resolveAssignedFile(assigns map[string]string, canonical string) (name string, assigned bool) {
	name = assigns[canonical]
	assigned = name != ""
	if name == "" {
		name = canonical
	}
	return name, assigned
}

// Play fires the named sound; no-op if not ready or out of range (fire-and-forget).
func Play(id Sound) {
	if !ready || id < 0 || id >= soundCount {
		return
	}
	rl.PlaySound(bank[id])
}

// PlayResult fires SoundInputGreat on ok, the SoundInputMiss buzz otherwise.
func PlayResult(ok bool) {
	if ok {
		Play(SoundInputGreat)
	} else {
		Play(SoundInputMiss)
	}
}

// loadBank builds the bank from on-disk state (files + assignments.txt):
//  1. Write any missing default .wav from PCM(); never clobber existing files.
//  2. Backfill assignments.txt so every cue has a row; the map is then
//     authoritative (each bank slot reads from assignments[slug]).
//  3. Load each cue's rl.Sound from its assigned file.
//
// If an assigned file can't be read at runtime, the cue falls back to its
// freshly-synthesized PCM so Play(cue) never goes silent.
func loadBank() {
	assigns := ensureBankOnDisk()
	forEachCue(func(cue Sound, row soundCue) {
		fileName, _ := resolveAssignedFile(assigns, row.Canonical)
		bank[cue] = loadCueFromDisk(fileName, row.PCM)
	})
}

// ensureBankOnDisk writes missing default .wav files and backfills
// assignments.txt so every cue has a file and a map row. Errors are swallowed
// (loadCueFromDisk's PCM fallback covers playback). Returns the loaded map so
// the caller needn't re-read assignments.txt.
func ensureBankOnDisk() map[string]string {
	assigns := userconfig.LoadAssignments()
	assignsChanged := false
	forEachCue(func(_ Sound, row soundCue) {
		if row.PCM == nil {
			return
		}
		// Write the default only if no .wav exists yet (preserve user edits).
		path := UserSoundPath(row.Canonical)
		if _, err := os.Stat(path); err != nil {
			_, _ = userconfig.WriteWAV(row.Canonical, row.PCM())
		}
		// Backfill the map entry; same-name default = slug → slug.
		if _, ok := assigns[row.Canonical]; !ok {
			assigns[row.Canonical] = row.Canonical
			assignsChanged = true
		}
	})
	if assignsChanged {
		_ = userconfig.SaveAssignments(assigns)
	}
	return assigns
}

// readOrSynthSound resolves one cue's rl.Sound: read the named .wav, else
// rebuild from pcmFn so Play(cue) never goes silent on a transient FS error.
// fromFile reports which branch ran (for "assigned file missing" flags). pcmFn
// is nil-checked so a malformed soundCues entry can't crash the bank.
func readOrSynthSound(name string, pcmFn func() []int16) (snd rl.Sound, fromFile bool) {
	if data, err := os.ReadFile(UserSoundPath(name)); err == nil {
		// A corrupt/non-WAV payload decodes to a zero Sound; only honor the disk
		// branch on a playable decode, else fall through to synth.
		if snd := bytesToSound(data); snd.Stream.Buffer != nil {
			return snd, true
		}
	}
	if pcmFn == nil {
		return rl.Sound{}, false
	}
	return pcmToSound(pcmFn()), false
}

// loadCueFromDisk is readOrSynthSound discarding the fromFile flag.
func loadCueFromDisk(name string, pcmFn func() []int16) rl.Sound {
	snd, _ := readOrSynthSound(name, pcmFn)
	return snd
}

// replaceSound swaps *slot for next, unloading the old buffer first (guarded
// against the zero Sound) so a reassignment can't leak raylib's C-side buffer.
func replaceSound(slot *rl.Sound, next rl.Sound) {
	if slot.Stream.Buffer != nil {
		rl.UnloadSound(*slot)
	}
	*slot = next
}

// pcmToSound builds a 16-bit mono PCM buffer into an rl.Sound via bytesToSound.
func pcmToSound(pcm []int16) rl.Sound {
	if !rl.IsAudioDeviceReady() {
		return rl.Sound{}
	}
	return bytesToSound(wavsynth.BuildWAV(pcm))
}

// bytesToSound decodes a pre-encoded WAV byte slice to an rl.Sound via
// LoadWaveFromMemory → LoadSoundFromWave. The Wave is unloaded immediately —
// the Sound owns its own copy.
func bytesToSound(wav []byte) rl.Sound {
	// Never hand a Wave to a dead device, even though callers gate upstream.
	if !rl.IsAudioDeviceReady() {
		return rl.Sound{}
	}
	wave := rl.LoadWaveFromMemory(".wav", wav, int32(len(wav)))
	snd := rl.LoadSoundFromWave(wave)
	rl.UnloadWave(wave)
	return snd
}

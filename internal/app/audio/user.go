package audio

import (
	"crawler/internal/app/audio/userconfig"
	"crawler/internal/app/audio/wavsynth"
	"fmt"
	"os"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// User-sound subsystem. The pure filesystem / parsing helpers (path
// resolution, sanitize, sound list, assignment file I/O, WAV write)
// live in the audio/userconfig subpackage so they can be unit-tested
// without raylib's DLL load — this file is the raylib-bound side of
// the same subsystem: preview ring, bank reload, cue ID lookup, and
// the thin wrappers callers historically used.
//
// Backwards-compat: the previous (raylib-bound) API names —
// ListUserSounds, UserSoundPath, SaveUserSound, DeleteUserSound — stay
// as forwarding wrappers so the editor and other callers don't have
// to know about the userconfig split.

// UserSoundPath, ListUserSounds, SaveUserSound, DeleteUserSound are
// kept as the audio-package surface for the editor's sound-modal
// callers, which already import this package for raylib-bound APIs
// (Play, PreviewPCM, etc.) and would otherwise need a second import
// just for the path/list/save/delete helpers. They forward to
// userconfig's equivalents — pure pass-through, no extra logic.
func UserSoundPath(name string) string { return userconfig.SoundPath(name) }
func ListUserSounds() []string         { return userconfig.ListSounds() }

// WavExt is the canonical file extension for user-sound files; alias of
// userconfig.WavExt so the editor's flash messages don't need a second
// import.
const WavExt = userconfig.WavExt

func SaveUserSound(name string, pcm []int16) (string, error) {
	return userconfig.WriteWAV(name, pcm)
}
func DeleteUserSound(name string) error { return userconfig.DeleteSound(name) }

// soundIDByName maps the canonical slug ("input_hit") to its Sound enum
// value. Derived from soundCues at init so adding a new Sound enum
// entry automatically picks up here too — no parallel table to keep
// in lockstep.
var soundIDByName = buildSoundIDByName()

func buildSoundIDByName() map[string]Sound {
	m := make(map[string]Sound, len(soundCues))
	for i, cue := range soundCues {
		if cue.Canonical == "" {
			continue
		}
		m[cue.Canonical] = Sound(i)
	}
	return m
}

// SoundCanonicalName returns the assignments-file key for a built-in
// cue. Reads directly from soundCues — the canonical slug for
// SoundInputHit is "input_hit", etc. Out-of-range values return "".
func SoundCanonicalName(s Sound) string {
	if s < 0 || s >= soundCount {
		return ""
	}
	return soundCues[s].Canonical
}

// AssignUserSound points the built-in `cue` at the named user .wav so
// every Play(cue) thereafter emits the user's recording. Persists to
// maps/sounds/assignments.txt; reloads the bank slot immediately when
// the audio device is ready. Pass userName="" to revert to the
// procedural built-in.
//
// Returns any cue slugs whose reload failed (via the wrapped
// ReloadUserAssignments call) so the editor can flash a warning when
// the assignment was saved to disk but the in-memory bank didn't pick
// it up.
func AssignUserSound(cue Sound, userName string) (failed []string, err error) {
	assigns := userconfig.LoadAssignments()
	cueName := SoundCanonicalName(cue)
	if cueName == "" {
		return nil, fmt.Errorf("unknown cue %d", int(cue))
	}
	if userName == "" {
		delete(assigns, cueName)
	} else {
		if _, statErr := os.Stat(UserSoundPath(userName)); statErr != nil {
			return nil, fmt.Errorf("user sound %q not found", userName)
		}
		assigns[cueName] = userName
	}
	if saveErr := userconfig.SaveAssignments(assigns); saveErr != nil {
		return nil, saveErr
	}
	// Only the cue we just (re)assigned changed — reload its single bank slot
	// rather than re-reading + re-decoding every cue's .wav via the full
	// ReloadUserAssignments sweep.
	return reloadOneCue(cue), nil
}

// reloadOneCue rebuilds just `cue`'s bank slot from the current assignments
// file — the targeted form of ReloadUserAssignments for when one assignment
// changes. Returns the cue's canonical slug in `failed` if it had an explicit
// assignment that couldn't load. No-op (nil) when the device isn't ready or
// cue is out of range.
func reloadOneCue(cue Sound) (failed []string) {
	if !ready || cue < 0 || cue >= soundCount {
		return nil
	}
	assigns := userconfig.LoadAssignments()
	row := soundCues[cue]
	fileName, assigned := resolveAssignedFile(assigns, row.Canonical)
	newSound, fromFile := readOrSynthSound(fileName, row.PCM)
	if !fromFile && assigned {
		failed = append(failed, row.Canonical)
	}
	replaceSound(&bank[cue], newSound)
	return failed
}

// CurrentAssignment returns the user-sound name currently assigned to a
// cue, or "" if the cue uses the procedural default. Caller's UI reads
// this to render "Cue X → my_sound.wav" or "Cue X → (default)".
//
// This re-reads + re-parses assignments.txt on every call. A caller that
// reads several cues at once (the editor's sound modal, per frame) should
// load the whole map ONCE via AllAssignments and index it with
// SoundCanonicalName instead of calling this per cue.
func CurrentAssignment(cue Sound) string {
	return AllAssignments()[SoundCanonicalName(cue)]
}

// AllAssignments returns the full cue-slug → user-sound-name map parsed from
// assignments.txt (the single source CurrentAssignment indexes). Callers that
// need several cues' assignments in one frame load this once and index it with
// SoundCanonicalName, collapsing N file reads+parses into one.
func AllAssignments() map[string]string {
	return userconfig.LoadAssignments()
}

// ReloadUserAssignments re-reads assignments.txt and overlays the bank
// — every cue with an assignment gets its slot replaced by the user's
// .wav; every cue without one keeps the procedural default. Safe to
// call repeatedly; rebuilds the bank in place (unloading the prior
// slot's raylib.Sound so we don't leak GPU/audio handles).
//
// Returns a list of canonical cue slugs whose assignment FAILED to load
// — caller can surface these so the editor knows which assignments
// fall back to their procedural defaults. Empty slice = all
// assignments succeeded.
//
// If the audio device isn't ready (Init never ran or failed), this
// silently no-ops — the assignments file still persists for the next
// successful Init.
func ReloadUserAssignments() (failed []string, err error) {
	if !ready {
		return nil, nil
	}
	assigns := userconfig.LoadAssignments()
	forEachCue(func(cue Sound, row soundCue) {
		fileName, assigned := resolveAssignedFile(assigns, row.Canonical)
		newSound, fromFile := readOrSynthSound(fileName, row.PCM)
		if !fromFile && assigned {
			// An explicitly-assigned file failed to load (the synth
			// fallback covered playback, but the player should know).
			failed = append(failed, row.Canonical)
		}
		replaceSound(&bank[cue], newSound)
	})
	for cueName := range assigns {
		if _, ok := soundIDByName[cueName]; !ok {
			failed = append(failed, cueName)
		}
	}
	return failed, nil
}

// SynthSweep wraps wavsynth's sweep helper so the editor can build cues
// without importing wavsynth directly. Pure passthrough.
func SynthSweep(duration, startHz, endHz, volume, attack, release float64) []int16 {
	return wavsynth.SynthSweep(duration, startHz, endHz, volume, attack, release)
}

// WaveShape and the four WaveX constants are re-exported so the
// sound editor doesn't have to import wavsynth alongside audio just
// to name a value type. SynthShape is the rich-knob variant of
// SynthSweep — wave shape, noise mix, vibrato — fed by the
// editor's expanded slider list.
type WaveShape = wavsynth.WaveShape

const (
	WaveSine     = wavsynth.WaveSine
	WaveSquare   = wavsynth.WaveSquare
	WaveTriangle = wavsynth.WaveTriangle
	WaveSaw      = wavsynth.WaveSaw
)

// WaveShapeCount re-exports the WaveShape enum size so the editor's
// wave-picker slider bounds itself off the enum, not a literal.
const WaveShapeCount = wavsynth.WaveShapeCount

func WaveShapeName(w WaveShape) string { return wavsynth.WaveShapeName(w) }

// SynthShape exposes the rich procedural sweep primitive to non-
// wavsynth callers. Forwards verbatim.
func SynthShape(duration, startHz, endHz, volume, attack, release float64,
	wave WaveShape, noiseMix, vibHz, vibDepth float64) []int16 {
	return wavsynth.SynthShape(duration, startHz, endHz, volume, attack, release,
		wave, noiseMix, vibHz, vibDepth)
}

// previewRingSize bounds the in-flight preview clips. Sized at 4 because
// previews are short (<1s) and the UI naturally caps how fast a user can
// fire them; we just need enough slots that consecutive previews don't
// cut each other off mid-play.
const previewRingSize = 4

// previewRing is a small ring buffer of rl.Sound handles used by
// PreviewPCM and PreviewFile. Each new preview overwrites the oldest
// slot, unloading the prior buffer first — without this the editor's
// sound-creator would leak an rl.Sound on every Preview press.
var (
	previewRing   [previewRingSize]rl.Sound
	previewCursor int
)

// PreviewPCM plays freshly-synthesized PCM through the preview ring
// without touching disk. Built for the editor's sound modal so tweaking
// a slider and hitting Preview doesn't pollute maps/sounds/ with a
// preview.wav file. Silent no-op when the audio device isn't ready.
func PreviewPCM(pcm []int16) {
	if !ready || len(pcm) == 0 {
		return
	}
	wav := wavsynth.BuildWAV(pcm, wavsynth.SampleRate)
	playThroughRing(wav)
}

// PreviewFile loads a saved .wav by name and plays it through the
// preview ring. Used by the editor's Saved Sounds list audition path.
// Doesn't touch the assignments table — purely a one-shot play.
func PreviewFile(name string) {
	if !ready {
		return
	}
	data, err := os.ReadFile(UserSoundPath(name))
	if err != nil {
		return
	}
	playThroughRing(data)
}

func playThroughRing(wavBytes []byte) {
	snd := bytesToSound(wavBytes)
	// replaceSound unloads the slot we're about to overwrite — its raylib
	// buffer was allocated by a prior preview and would leak otherwise.
	replaceSound(&previewRing[previewCursor], snd)
	rl.PlaySound(snd)
	previewCursor = (previewCursor + 1) % previewRingSize
}

// unloadPreviewRing releases every preview slot. Called by Close so the
// ring's buffers get reclaimed when the audio device shuts down.
func unloadPreviewRing() {
	unloadSounds(previewRing[:])
	previewCursor = 0
}

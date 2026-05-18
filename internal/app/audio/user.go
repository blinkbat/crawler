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

// SoundsDir is the legacy alias for userconfig.SoundsDir, kept for
// callers that already imported audio. New callers should prefer the
// userconfig path directly.
func SoundsDir() string { return userconfig.SoundsDir() }

// UserSoundPath, ListUserSounds, SaveUserSound, DeleteUserSound are
// kept as the audio-package surface for backwards compatibility — they
// forward to userconfig's equivalents.
func UserSoundPath(name string) string { return userconfig.SoundPath(name) }
func ListUserSounds() []string         { return userconfig.ListSounds() }
func SaveUserSound(name string, pcm []int16) (string, error) {
	return userconfig.WriteWAV(name, pcm)
}
func DeleteUserSound(name string) error { return userconfig.DeleteSound(name) }

// soundIDByName maps the canonical slug ("input_hit") to its Sound enum
// value. Derived from soundMeta at init so adding a new Sound enum entry
// (with its row in soundMeta) automatically picks up here too — no
// parallel table to keep in lockstep.
var soundIDByName = buildSoundIDByName()

func buildSoundIDByName() map[string]Sound {
	m := make(map[string]Sound, len(soundMeta))
	for i, meta := range soundMeta {
		if meta.Canonical == "" {
			continue
		}
		m[meta.Canonical] = Sound(i)
	}
	return m
}

// SoundCanonicalName returns the assignments-file key for a built-in
// cue. Reads directly from soundMeta — the canonical slug for
// SoundInputHit is "input_hit", etc. Out-of-range values return "".
func SoundCanonicalName(s Sound) string {
	if s < 0 || s >= soundCount {
		return ""
	}
	return soundMeta[s].Canonical
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
	return ReloadUserAssignments()
}

// CurrentAssignment returns the user-sound name currently assigned to a
// cue, or "" if the cue uses the procedural default. Caller's UI reads
// this to render "Cue X → my_sound.wav" or "Cue X → (default)".
func CurrentAssignment(cue Sound) string {
	assigns := userconfig.LoadAssignments()
	cueName := SoundCanonicalName(cue)
	if cueName == "" {
		return ""
	}
	return assigns[cueName]
}

// ReloadUserAssignments re-reads assignments.txt and overlays the bank
// — every cue with an assignment gets its slot replaced by the user's
// .wav; every cue without one keeps the procedural default. Safe to
// call repeatedly; rebuilds the bank in place (unloading the prior
// slot's raylib.Sound so we don't leak GPU/audio handles).
//
// Returns a list of canonical cue slugs whose assignment FAILED to load
// — caller can surface these so the editor knows which assignments
// silently fell back to the previous bank entry. Empty slice = all
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
	for cueName, userName := range assigns {
		cue, ok := soundIDByName[cueName]
		if !ok {
			failed = append(failed, cueName)
			continue
		}
		path := UserSoundPath(userName)
		data, ierr := os.ReadFile(path)
		if ierr != nil {
			// Track the failure so the caller can flash a warning; the
			// cue keeps its current bank entry (procedural default or
			// the previous successful user assignment).
			failed = append(failed, cueName)
			continue
		}
		// Replace the slot. UnloadSound on the old entry first so its
		// raylib-side buffer doesn't leak; bytesToSound creates a fresh
		// handle from the file bytes.
		newSound := bytesToSound(data)
		if bank[cue].Stream.Buffer != nil {
			rl.UnloadSound(bank[cue])
		}
		bank[cue] = newSound
	}
	return failed, nil
}

// SynthSweep wraps wavsynth's sweep helper so the editor can build cues
// without importing wavsynth directly. Pure passthrough.
func SynthSweep(duration, startHz, endHz, volume, attack, release float64) []int16 {
	return wavsynth.SynthSweep(duration, startHz, endHz, volume, attack, release)
}

// previewRing is a small ring buffer of rl.Sound handles used by
// PreviewPCM and PreviewFile. Each new preview overwrites the oldest
// slot, unloading the prior buffer first — without this the editor's
// sound-creator would leak an rl.Sound on every Preview press. Sized at
// 4 because previews are short (<1s) and the UI naturally caps how
// fast a user can fire them; we just need enough slots that consecutive
// previews don't cut each other off mid-play.
var (
	previewRing   [4]rl.Sound
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
	// Unload the slot we're about to overwrite — its raylib buffer was
	// allocated by a prior preview and would leak otherwise.
	if previewRing[previewCursor].Stream.Buffer != nil {
		rl.UnloadSound(previewRing[previewCursor])
	}
	previewRing[previewCursor] = snd
	rl.PlaySound(snd)
	previewCursor = (previewCursor + 1) % len(previewRing)
}

// unloadPreviewRing releases every preview slot. Called by Close so the
// ring's buffers get reclaimed when the audio device shuts down.
func unloadPreviewRing() {
	for i, s := range previewRing {
		if s.Stream.Buffer != nil {
			rl.UnloadSound(s)
			previewRing[i] = rl.Sound{}
		}
	}
	previewCursor = 0
}

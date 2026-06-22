package audio

import (
	"crawler/internal/app/audio/userconfig"
	"crawler/internal/app/audio/wavsynth"
	"fmt"
	"os"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// User-sound subsystem, raylib-bound side: preview ring, bank reload, cue ID
// lookup, and forwarding wrappers. Pure FS/parsing helpers live in userconfig
// (so they unit-test without raylib's DLL load).
//
// UserSoundPath / ListUserSounds / SaveUserSound / DeleteUserSound forward to
// userconfig so the editor's sound-modal callers don't need a second import.
func UserSoundPath(name string) string { return userconfig.SoundPath(name) }
func ListUserSounds() []string         { return userconfig.ListSounds() }

// WavExt is the canonical user-sound file extension; alias of userconfig.WavExt.
const WavExt = userconfig.WavExt

func SaveUserSound(name string, pcm []int16) (string, error) {
	return userconfig.WriteWAV(name, pcm)
}
func DeleteUserSound(name string) error { return userconfig.DeleteSound(name) }

// ShapeParams is the full synth-knob set the sound editor edits (aliased from
// wavsynth). SaveUserSoundParams writes the .wav + editing sidecar;
// LoadUserSoundParams reads the sidecar (ok=false when a sound has none).
type ShapeParams = wavsynth.ShapeParams

func SaveUserSoundParams(name string, p ShapeParams) (string, error) {
	return userconfig.WriteSound(name, p)
}
func LoadUserSoundParams(name string) (ShapeParams, bool) {
	return userconfig.LoadParams(name)
}

// Musical-note helpers re-exported for the editor's note pickers.
const NoteCount = wavsynth.NoteCount

func NoteHz(i int) float64            { return wavsynth.NoteHz(i) }
func NoteName(i int) string           { return wavsynth.NoteName(i) }
func NearestNoteIndex(hz float64) int { return wavsynth.NearestNoteIndex(hz) }

// SoundCanonicalName returns the assignments-file key for a cue (e.g.
// SoundInputHit → "input_hit"); out-of-range returns "".
func SoundCanonicalName(s Sound) string {
	if s < 0 || s >= soundCount {
		return ""
	}
	return soundCues[s].Canonical
}

// AssignUserSound points cue at the named user .wav, persists to
// assignments.txt, and reloads the bank slot when the device is ready. Pass
// userName="" to revert to the procedural built-in. Returns cue slugs whose
// reload failed so the editor can warn (saved to disk but bank didn't pick up).
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
	// Reload just this slot, not the full ReloadUserAssignments sweep.
	return reloadOneCue(cue), nil
}

// reloadOneCue rebuilds just cue's bank slot from assignments.txt. Returns the
// cue slug in failed if its explicit assignment couldn't load. Nil when the
// device isn't ready or cue is out of range.
func reloadOneCue(cue Sound) (failed []string) {
	if !ready || cue < 0 || cue >= soundCount {
		return nil
	}
	assigns := userconfig.LoadAssignments()
	row := soundCues[cue]
	if reloadCueSlot(cue, assigns) {
		failed = append(failed, row.Canonical)
	}
	return failed
}

// reloadCueSlot rebuilds one cue's bank slot from assigns (resolve → read or
// synth → swap into bank[cue]). Returns true when an explicit assignment's file
// failed to load (synth covered playback, but the caller should surface it).
func reloadCueSlot(cue Sound, assigns map[string]string) (failed bool) {
	row := soundCues[cue]
	fileName, assigned := resolveAssignedFile(assigns, row.Canonical)
	newSound, fromFile := readOrSynthSound(fileName, row.PCM)
	replaceSound(&bank[cue], newSound)
	return !fromFile && assigned
}

// CurrentAssignment returns the user-sound name assigned to a cue, or "" for
// the procedural default. Re-reads assignments.txt per call — a caller needing
// several cues per frame should use AllAssignments once instead.
func CurrentAssignment(cue Sound) string {
	return AllAssignments()[SoundCanonicalName(cue)]
}

// AllAssignments returns the full cue-slug → user-sound-name map from
// assignments.txt. Load once and index with SoundCanonicalName for many cues.
func AllAssignments() map[string]string {
	return userconfig.LoadAssignments()
}

// ReloadUserAssignments re-reads assignments.txt and overlays the bank in place
// (assigned cues get the user's .wav, others keep the default; old slots are
// unloaded so handles don't leak). Returns canonical slugs whose assignment
// FAILED to load. No-op when the device isn't ready (the file still persists).
func ReloadUserAssignments() (failed []string, err error) {
	if !ready {
		return nil, nil
	}
	assigns := userconfig.LoadAssignments()
	forEachCue(func(cue Sound, row soundCue) {
		if reloadCueSlot(cue, assigns) {
			failed = append(failed, row.Canonical)
		}
	})
	// failed = assigned-FILE load failures only. An unknown cue key is an inert
	// orphan (forEachCue reads only known cues), skipped silently like any
	// malformed LoadAssignments line — not a false warning.
	return failed, nil
}

// SynthSweep forwards to wavsynth.SynthSweep.
func SynthSweep(duration, startHz, endHz, volume, attack, release float64) []int16 {
	return wavsynth.SynthSweep(duration, startHz, endHz, volume, attack, release)
}

// WaveShape and the WaveX constants are re-exported for the sound editor.
type WaveShape = wavsynth.WaveShape

const (
	WaveSine     = wavsynth.WaveSine
	WaveSquare   = wavsynth.WaveSquare
	WaveTriangle = wavsynth.WaveTriangle
	WaveSaw      = wavsynth.WaveSaw
)

// WaveShapeCount re-exports the WaveShape enum size for the editor's wave picker.
const WaveShapeCount = wavsynth.WaveShapeCount

func WaveShapeName(w WaveShape) string { return wavsynth.WaveShapeName(w) }

// SynthShape forwards to wavsynth.SynthShape.
func SynthShape(duration, startHz, endHz, volume, attack, release float64,
	wave WaveShape, noiseMix, vibHz, vibDepth float64) []int16 {
	return wavsynth.SynthShape(duration, startHz, endHz, volume, attack, release,
		wave, noiseMix, vibHz, vibDepth)
}

// SynthShapeParams forwards to wavsynth.SynthShapeParams.
func SynthShapeParams(p ShapeParams) []int16 { return wavsynth.SynthShapeParams(p) }

// previewRingSize bounds the in-flight preview clips — enough that consecutive
// short (<1s) previews don't cut each other off.
const previewRingSize = 4

// previewRing buffers preview rl.Sound handles for PreviewPCM/PreviewFile. Each
// new preview unloads the prior buffer in its slot so a Preview press can't leak.
var (
	previewRing   [previewRingSize]rl.Sound
	previewCursor int
)

// PreviewPCM plays freshly-synthesized PCM through the preview ring without
// touching disk. No-op when the device isn't ready.
func PreviewPCM(pcm []int16) {
	if !ready || len(pcm) == 0 {
		return
	}
	wav := wavsynth.BuildWAV(pcm, wavsynth.SampleRate)
	playThroughRing(wav)
}

// PreviewFile plays a saved .wav by name through the preview ring (one-shot;
// doesn't touch the assignments table).
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
	if snd.Stream.Buffer == nil {
		// Zero Sound (dead device or malformed bytes) — don't store or play.
		return
	}
	// Reuse a finished slot: replaceSound frees the slot's buffer, and freeing
	// one still streaming is a use-after-free. Scan from the cursor for the first
	// free/empty slot; only when all are in flight fall back to the oldest.
	slot := previewCursor
	found := false
	for i := 0; i < previewRingSize; i++ {
		idx := (previewCursor + i) % previewRingSize
		if s := previewRing[idx]; s.Stream.Buffer == nil || !rl.IsSoundPlaying(s) {
			slot = idx
			found = true
			break
		}
	}
	if !found {
		// All slots in flight — stop the oldest first so the mixer releases its
		// buffer before replaceSound frees it (otherwise a use-after-free).
		rl.StopSound(previewRing[slot])
	}
	replaceSound(&previewRing[slot], snd)
	rl.PlaySound(snd)
	previewCursor = (slot + 1) % previewRingSize
}

// unloadPreviewRing releases every preview slot. Called by Close.
func unloadPreviewRing() {
	unloadSounds(previewRing[:])
	previewCursor = 0
}

// Package userconfig holds the pure (non-raylib) logic for the audio
// package's user-sound subsystem: filesystem paths, name sanitization,
// the saved-sounds list, the WAV writer that hands its bytes off to
// raylib via the parent audio package, and the assignments-file parser.
//
// Split out of internal/app/audio for the same reason wavsynth was —
// the parent audio package imports raylib via purego, which fails to
// init under `go test` (raylib.dll isn't on the test binary's load
// path). Keeping these pure helpers raylib-free lets us cover them
// with unit tests; the audio package wraps them with the raylib-side
// preview ring + bank reload.
package userconfig

import (
	"crawler/internal/app/audio/wavsynth"
	"crawler/internal/app/core"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SoundsDirName is the on-disk folder where user .wav files live. Mirrors
// core.MapsDir's "maps" convention; both sit under the working directory.
const SoundsDirName = "maps/sounds"

// AssignmentFile holds "cue=filename" lines: one per built-in cue, the
// filename is relative to maps/sounds/. Sound enum entries not listed
// fall through to the procedural cue.
const AssignmentFile = "assignments.txt"

// SoundsDir resolves the on-disk sounds folder via core.ResolveAssetDir
// — the same machinery core.MapsDir uses for the maps folder.
func SoundsDir() string {
	return core.ResolveAssetDir(SoundsDirName)
}

// SanitizeName normalizes a user-typed sound name into a safe filename
// stem. Thin wrapper over core.SanitizeFilename with an empty fallback
// — sound saves refuse rather than producing a synthetic "untitled" file.
func SanitizeName(name string) string {
	return core.SanitizeFilename(name, "")
}

// SoundPath returns the .wav path for a named user sound (no existence
// check — caller's responsibility).
func SoundPath(name string) string {
	return filepath.Join(SoundsDir(), name+".wav")
}

// ListSounds returns the names (without .wav) of every .wav file in the
// sounds directory, sorted. Returns empty on missing dir or read errors
// so the editor's new-sound flow can still surface "no user sounds yet"
// without an error.
func ListSounds() []string {
	dir := SoundsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".wav") {
			continue
		}
		out = append(out, strings.TrimSuffix(name, filepath.Ext(name)))
	}
	sort.Strings(out)
	return out
}

// WriteWAV writes a PCM cue's WAV-encoded bytes to maps/sounds/<name>.wav
// after sanitizing the name. Returns the final on-disk filename
// (sanitized) so callers can report "Saved as X" in the UI. Overwrites
// any existing file at the same name — the caller's UI should confirm
// before calling. Returns an error if the sanitized name is empty.
func WriteWAV(name string, pcm []int16) (string, error) {
	clean := SanitizeName(name)
	if clean == "" {
		return "", fmt.Errorf("sound name required")
	}
	dir := SoundsDir()
	if err := os.MkdirAll(dir, core.AssetDirMode); err != nil {
		return clean, err
	}
	wav := wavsynth.BuildWAV(pcm, wavsynth.SampleRate)
	path := filepath.Join(dir, clean+".wav")
	if err := os.WriteFile(path, wav, core.AssetFileMode); err != nil {
		return clean, err
	}
	return clean, nil
}

// DeleteSound removes a named .wav from maps/sounds/. After delete,
// also strips any assignment that pointed at the file so the bank
// doesn't try to reload a missing cue on next reload. Returns the
// underlying os.Remove error verbatim so the caller can decide whether
// to flash.
func DeleteSound(name string) error {
	if err := os.Remove(SoundPath(name)); err != nil {
		return err
	}
	assigns := LoadAssignments()
	changed := false
	for cue, file := range assigns {
		if file == name {
			delete(assigns, cue)
			changed = true
		}
	}
	if changed {
		_ = SaveAssignments(assigns)
	}
	return nil
}

// LoadAssignments reads the assignments file. Lines are "cue=name" with
// '#' line comments. Missing file returns an empty map. Malformed lines
// are skipped silently so a partial corruption doesn't lose the rest.
func LoadAssignments() map[string]string {
	out := make(map[string]string)
	data, err := os.ReadFile(filepath.Join(SoundsDir(), AssignmentFile))
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		cue := strings.TrimSpace(line[:eq])
		name := strings.TrimSpace(line[eq+1:])
		if cue == "" || name == "" {
			continue
		}
		out[cue] = name
	}
	return out
}

// SaveAssignments writes the cue=name map back to disk in sorted key
// order so the file diffs cleanly across edits. Creates the sounds
// directory if missing.
func SaveAssignments(assigns map[string]string) error {
	dir := SoundsDir()
	if err := os.MkdirAll(dir, core.AssetDirMode); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("# Cue assignments: <cue_name>=<user_sound_name>\n")
	b.WriteString("# Removing a line reverts the cue to its procedural default.\n")
	cues := make([]string, 0, len(assigns))
	for cue := range assigns {
		cues = append(cues, cue)
	}
	sort.Strings(cues)
	for _, cue := range cues {
		fmt.Fprintf(&b, "%s=%s\n", cue, assigns[cue])
	}
	return os.WriteFile(filepath.Join(dir, AssignmentFile), []byte(b.String()), core.AssetFileMode)
}

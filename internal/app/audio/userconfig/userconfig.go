// Package userconfig holds the pure (non-raylib) audio user-sound logic: paths,
// name sanitization, saved-sounds list, WAV writer, assignments parser. Split
// out (like wavsynth) so it unit-tests without raylib's purego DLL load.
package userconfig

import (
	"crawler/internal/app/audio/wavsynth"
	"crawler/internal/app/core"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// SoundsDirName is the folder where user .wav files live, under the working dir.
const SoundsDirName = "maps/sounds"

// AssignmentFile holds "cue=filename" lines (filename relative to maps/sounds/).
// Unlisted cues fall through to the procedural default.
const AssignmentFile = "assignments.txt"

// SoundsDir resolves the on-disk sounds folder via core.ResolveAssetDir.
func SoundsDir() string {
	return core.ResolveAssetDir(SoundsDirName)
}

// MusicDirName is the folder streamed-music tracks live in (e.g. the exploration
// BGM); MusicDir resolves it the same cwd-then-exe way as SoundsDir.
const MusicDirName = "maps/music"

func MusicDir() string {
	return core.ResolveAssetDir(MusicDirName)
}

// MusicTrackPath returns the on-disk path for a named music file (filename incl.
// extension, e.g. "bg_explore.ogg") — no existence check.
func MusicTrackPath(filename string) string {
	return filepath.Join(MusicDir(), filename)
}

// VolumesFile persists the player's music + SFX volume (0..1) so they survive a
// restart — global audio settings, NOT per-save. Two "key=value" lines; a missing
// or unparsable file falls back to the defaults below.
const VolumesFile = "volumes.txt"

// Default music/SFX volumes for a first run (no volumes.txt yet). Music sits well
// below SFX — the BGM is a bed, not a foreground element.
const (
	DefaultMusicVolume = float32(0.4)
	DefaultSFXVolume   = float32(0.8)
)

// LoadVolumes reads maps/sounds/volumes.txt, returning the saved music + SFX volumes
// (clamped to [0,1]) and the master mute flag. Any missing key keeps its default, so a
// partial or absent file is safe (mute defaults off).
func LoadVolumes() (music, sfx float32, muted bool) {
	music, sfx = DefaultMusicVolume, DefaultSFXVolume
	data, err := os.ReadFile(filepath.Join(SoundsDir(), VolumesFile))
	if err != nil {
		return music, sfx, false
	}
	for _, line := range strings.Split(string(data), "\n") {
		key, val, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		key, val = strings.TrimSpace(key), strings.TrimSpace(val)
		if key == "mute" {
			muted, _ = strconv.ParseBool(val) // unparsable → false
			continue
		}
		f, err := strconv.ParseFloat(val, 32)
		if err != nil {
			continue
		}
		v := core.Clamp(float32(f), 0, 1)
		switch key {
		case "music":
			music = v
		case "sfx":
			sfx = v
		}
	}
	return music, sfx, muted
}

// SaveVolumes writes music + SFX volume + mute to maps/sounds/volumes.txt (creating
// the dir if needed). Volumes clamped to [0,1] so a stray value can't persist out of range.
func SaveVolumes(music, sfx float32, muted bool) error {
	dir := SoundsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	body := fmt.Sprintf("music=%.3f\nsfx=%.3f\nmute=%t\n", core.Clamp(music, 0, 1), core.Clamp(sfx, 0, 1), muted)
	return os.WriteFile(filepath.Join(dir, VolumesFile), []byte(body), 0o644)
}

// SanitizeName normalizes a name into a safe filename stem; empty stays empty
// (saves refuse rather than produce an "untitled" file).
func SanitizeName(name string) string {
	return core.SanitizeFilename(name, "")
}

// WavExt is the canonical user-sound file extension.
const WavExt = ".wav"

// ParamsExt is the JSON sidecar (synth knobs) beside a .wav, for reopening.
// A hand-dropped .wav has none.
const ParamsExt = ".snd"

// SoundPath returns the .wav path for a named user sound (no existence check).
func SoundPath(name string) string {
	return filepath.Join(SoundsDir(), name+WavExt)
}

// ParamsPath returns the .snd sidecar path for a named user sound.
func ParamsPath(name string) string {
	return filepath.Join(SoundsDir(), name+ParamsExt)
}

// ListSounds returns the names (sans .wav) of every .wav in the sounds dir,
// sorted. Empty on missing dir or read error.
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
		if !strings.HasSuffix(strings.ToLower(name), WavExt) {
			continue
		}
		// Strip only the trailing .wav, not the last dot-segment: a dotted stem like
		// "a.b.wav" must list as "a.b" so SoundPath round-trips it (filepath.Ext would
		// drop ".b", yielding a name that resolves to a different/missing file).
		out = append(out, name[:len(name)-len(WavExt)])
	}
	sort.Strings(out)
	return out
}

// WriteWAV writes a PCM cue's WAV bytes to maps/sounds/<name>.wav (name
// sanitized). Returns the sanitized filename. Overwrites; errors on empty name.
func WriteWAV(name string, pcm []int16) (string, error) {
	clean := SanitizeName(name)
	if clean == "" {
		return "", fmt.Errorf("sound name required")
	}
	dir := SoundsDir()
	if err := os.MkdirAll(dir, core.AssetDirMode); err != nil {
		return clean, err
	}
	wav := wavsynth.BuildWAV(pcm)
	path := filepath.Join(dir, clean+WavExt)
	if err := os.WriteFile(path, wav, core.AssetFileMode); err != nil {
		return clean, err
	}
	return clean, nil
}

// WriteSound writes <name>.wav plus a <name>.snd sidecar (synth knobs). Returns
// the sanitized stem. Sidecar write is best-effort — failing it doesn't fail the save.
func WriteSound(name string, p wavsynth.ShapeParams) (string, error) {
	clean := SanitizeName(name)
	if clean == "" {
		return "", fmt.Errorf("sound name required")
	}
	dir := SoundsDir()
	if err := os.MkdirAll(dir, core.AssetDirMode); err != nil {
		return clean, err
	}
	wav := wavsynth.BuildWAV(wavsynth.SynthShapeParams(p))
	if err := os.WriteFile(filepath.Join(dir, clean+WavExt), wav, core.AssetFileMode); err != nil {
		return clean, err
	}
	if data, err := json.MarshalIndent(p, "", "  "); err == nil {
		_ = os.WriteFile(filepath.Join(dir, clean+ParamsExt), data, core.AssetFileMode)
	}
	return clean, nil
}

// LoadParams reads the <name>.snd sidecar. ok=false when missing or unparseable
// (the editor grays out "Edit" for sounds it can't reconstruct).
func LoadParams(name string) (wavsynth.ShapeParams, bool) {
	var p wavsynth.ShapeParams
	clean := SanitizeName(name)
	if clean == "" {
		return p, false
	}
	data, err := os.ReadFile(ParamsPath(clean))
	if err != nil {
		return p, false
	}
	if json.Unmarshal(data, &p) != nil {
		return p, false
	}
	return p, true
}

// DeleteSound removes a .wav and strips any assignment pointing at it. Returns
// os.Remove's error verbatim.
func DeleteSound(name string) error {
	// Sanitize first so a crafted name can't path-traverse via os.Remove.
	clean := SanitizeName(name)
	if clean == "" {
		return fmt.Errorf("invalid sound name")
	}
	if err := os.Remove(SoundPath(clean)); err != nil {
		return err
	}
	// Best-effort: drop the sidecar too (may not exist) so it doesn't orphan.
	_ = os.Remove(ParamsPath(clean))
	assigns := LoadAssignments()
	changed := false
	for cue, file := range assigns {
		if file == clean {
			delete(assigns, cue)
			changed = true
		}
	}
	if changed {
		_ = SaveAssignments(assigns)
	}
	return nil
}

// LoadAssignments reads the assignments file ("cue=name" lines, '#' comments).
// Missing file returns an empty map; malformed lines are skipped silently.
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
		// Sanitize at parse time against path traversal in the SoundPath join.
		name := SanitizeName(strings.TrimSpace(line[eq+1:]))
		if cue == "" || name == "" {
			continue
		}
		out[cue] = name
	}
	return out
}

// SaveAssignments writes the cue=name map in sorted key order (clean diffs).
// Creates the sounds directory if missing.
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
		// Sanitized stem (idempotent, round-trips with LoadAssignments). Drop an
		// empty-sanitizing value rather than emit "cue=".
		name := SanitizeName(assigns[cue])
		if name == "" {
			continue
		}
		fmt.Fprintf(&b, "%s=%s\n", cue, name)
	}
	return os.WriteFile(filepath.Join(dir, AssignmentFile), []byte(b.String()), core.AssetFileMode)
}

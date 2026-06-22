package userconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"crawler/internal/app/audio/wavsynth"
)

// withWorkingDir runs fn with cwd = dir so SoundsDir() resolves to a temp dir.
func withWorkingDir(t *testing.T, dir string, fn func()) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	defer func() { _ = os.Chdir(old) }()
	fn()
}

// TestSanitizeName freezes the filename contract (lowercase ASCII/digits/_/-).
func TestSanitizeName(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"input_hit", "input_hit"},
		{"Input Hit", "input_hit"},
		{"  My Cue!  ", "my_cue"},
		{"weird/../path", "weirdpath"},
		{"héllo", "hllo"}, // multi-byte stripped
		{"123_test", "123_test"},
		{"-dash-name-", "-dash-name-"},
		{"", ""},
		{"!@#$%^&*()", ""},
	}
	for _, c := range cases {
		got := SanitizeName(c.in)
		if got != c.want {
			t.Errorf("SanitizeName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestWriteWAV_RoundTrip verifies a write lands at the sanitized path and in ListSounds.
func TestWriteWAV_RoundTrip(t *testing.T) {
	tmp := t.TempDir()
	withWorkingDir(t, tmp, func() {
		pcm := []int16{0, 0, 0, 0}
		saved, err := WriteWAV("Test Cue!", pcm)
		if err != nil {
			t.Fatalf("WriteWAV: %v", err)
		}
		if saved != "test_cue" {
			t.Errorf("saved name = %q, want %q", saved, "test_cue")
		}
		path := filepath.Join(SoundsDir(), saved+".wav")
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected file at %s: %v", path, err)
		}
		names := ListSounds()
		found := false
		for _, n := range names {
			if n == saved {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ListSounds() = %v, missing %q", names, saved)
		}
	})
}

// TestWriteSound_ParamsRoundTrip: params save lands .wav + .snd, LoadParams
// reconstructs the knobs, delete removes both.
func TestWriteSound_ParamsRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	withWorkingDir(t, tmp, func() {
		p := wavsynth.ShapeParams{
			Duration: 0.12, StartHz: 330, EndHz: 990, Volume: 0.3,
			Attack: 0.01, Decay: 0.05, Sustain: 0.6, Release: 0.08,
			Wave: wavsynth.WaveSquare, PulseWidth: 0.3, NoiseMix: 0.2,
			VibHz: 6, VibDepth: 0.1, TremoloHz: 4, TremoloDepth: 0.5,
			Cutoff: 0.7, Drive: 0.4, Crush: 0.25,
		}
		saved, err := WriteSound("Edit Me!", p)
		if err != nil {
			t.Fatalf("WriteSound: %v", err)
		}
		if saved != "edit_me" {
			t.Errorf("saved name = %q, want %q", saved, "edit_me")
		}
		if _, err := os.Stat(SoundPath(saved)); err != nil {
			t.Errorf("expected .wav at %s: %v", SoundPath(saved), err)
		}
		if _, err := os.Stat(ParamsPath(saved)); err != nil {
			t.Errorf("expected .snd sidecar at %s: %v", ParamsPath(saved), err)
		}
		got, ok := LoadParams(saved)
		if !ok {
			t.Fatal("LoadParams ok=false, want true")
		}
		if got != p {
			t.Errorf("LoadParams round-trip mismatch:\n got  %+v\n want %+v", got, p)
		}
		// .snd is metadata — it must not show up as a sound.
		for _, n := range ListSounds() {
			if strings.HasSuffix(n, ParamsExt) {
				t.Errorf("ListSounds leaked sidecar entry %q", n)
			}
		}
		if err := DeleteSound(saved); err != nil {
			t.Fatalf("DeleteSound: %v", err)
		}
		if _, err := os.Stat(ParamsPath(saved)); !os.IsNotExist(err) {
			t.Errorf("expected sidecar removed on delete, stat err = %v", err)
		}
	})
}

// TestLoadParams_MissingSidecar — a sound with no .snd reports ok=false.
func TestLoadParams_MissingSidecar(t *testing.T) {
	tmp := t.TempDir()
	withWorkingDir(t, tmp, func() {
		if _, err := WriteWAV("legacy", []int16{0, 0}); err != nil {
			t.Fatalf("WriteWAV: %v", err)
		}
		if _, ok := LoadParams("legacy"); ok {
			t.Error("LoadParams ok=true for a sound with no sidecar, want false")
		}
	})
}

// TestWriteWAV_RefusesEmpty — a name that sanitizes to "" must error.
func TestWriteWAV_RefusesEmpty(t *testing.T) {
	tmp := t.TempDir()
	withWorkingDir(t, tmp, func() {
		_, err := WriteWAV("!!!", []int16{0})
		if err == nil {
			t.Error("WriteWAV with all-strippable name should error")
		}
	})
}

// TestLoadAssignments_RoundTrip: only valid entries survive mixed/commented/malformed lines.
func TestLoadAssignments_RoundTrip(t *testing.T) {
	tmp := t.TempDir()
	withWorkingDir(t, tmp, func() {
		_ = os.MkdirAll(SoundsDir(), 0o755)
		content := strings.Join([]string{
			"# header comment",
			"input_hit=my_blip",
			"",
			"malformed_no_equals",
			"heal=heal_chord",
			"=empty_key",
			"unknown_cue=ignored_at_reload",
		}, "\n")
		err := os.WriteFile(filepath.Join(SoundsDir(), AssignmentFile), []byte(content), 0o644)
		if err != nil {
			t.Fatalf("write assignments: %v", err)
		}
		got := LoadAssignments()
		want := map[string]string{
			"input_hit":   "my_blip",
			"heal":        "heal_chord",
			"unknown_cue": "ignored_at_reload", // loader keeps; reload filters
		}
		if len(got) != len(want) {
			t.Errorf("got %d entries, want %d (%v)", len(got), len(want), got)
		}
		for k, v := range want {
			if got[k] != v {
				t.Errorf("got[%q] = %q, want %q", k, got[k], v)
			}
		}
	})
}

// TestSaveAssignments_DeterministicOrder verifies sorted key order on disk.
func TestSaveAssignments_DeterministicOrder(t *testing.T) {
	tmp := t.TempDir()
	withWorkingDir(t, tmp, func() {
		assigns := map[string]string{
			"zebra":     "z_sound",
			"input_hit": "a_sound",
			"heal":      "h_sound",
		}
		if err := SaveAssignments(assigns); err != nil {
			t.Fatalf("SaveAssignments: %v", err)
		}
		data, err := os.ReadFile(filepath.Join(SoundsDir(), AssignmentFile))
		if err != nil {
			t.Fatalf("read back: %v", err)
		}
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		// Skip header comments.
		var entries []string
		for _, line := range lines {
			if strings.HasPrefix(line, "#") || line == "" {
				continue
			}
			entries = append(entries, line)
		}
		want := []string{
			"heal=h_sound",
			"input_hit=a_sound",
			"zebra=z_sound",
		}
		if len(entries) != len(want) {
			t.Fatalf("got %d entries, want %d (%v)", len(entries), len(want), entries)
		}
		for i := range want {
			if entries[i] != want[i] {
				t.Errorf("entries[%d] = %q, want %q", i, entries[i], want[i])
			}
		}
	})
}

// TestDeleteSound_StripsOrphanAssignments — deleting a sound also removes any
// assignment that pointed at it.
func TestDeleteSound_StripsOrphanAssignments(t *testing.T) {
	tmp := t.TempDir()
	withWorkingDir(t, tmp, func() {
		if _, err := WriteWAV("victim", []int16{0, 0}); err != nil {
			t.Fatalf("WriteWAV: %v", err)
		}
		err := SaveAssignments(map[string]string{
			"input_hit": "victim",
			"heal":      "survivor",
		})
		if err != nil {
			t.Fatalf("SaveAssignments: %v", err)
		}
		if err := DeleteSound("victim"); err != nil {
			t.Fatalf("DeleteSound: %v", err)
		}
		// File gone.
		if _, err := os.Stat(SoundPath("victim")); !os.IsNotExist(err) {
			t.Errorf("expected file deleted, stat err = %v", err)
		}
		// Stale stripped, unrelated preserved.
		got := LoadAssignments()
		if _, exists := got["input_hit"]; exists {
			t.Errorf("expected input_hit assignment to be stripped, got %v", got)
		}
		if got["heal"] != "survivor" {
			t.Errorf("expected heal=survivor, got %v", got)
		}
	})
}

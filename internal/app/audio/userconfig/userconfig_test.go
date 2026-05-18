package userconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withWorkingDir runs fn with the process cwd switched to dir, then
// restores. Used by tests so SoundsDir() resolves to a per-test temp
// directory instead of the project's maps/sounds/.
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

// TestSanitizeName freezes the on-disk filename contract for sound
// saves: lowercase ASCII, digits, underscore, hyphen only; spaces fold
// to underscore; multi-byte / punctuation strips; empty stays empty.
func TestSanitizeName(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"input_hit", "input_hit"},
		{"Input Hit", "input_hit"},
		{"  My Cue!  ", "my_cue"},
		{"weird/../path", "weirdpath"},
		{"héllo", "hllo"}, // multi-byte stripped (per-byte filter)
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

// TestWriteWAV_RoundTrip writes a PCM buffer under a name that requires
// sanitization, then verifies the file ends up at the sanitized path
// and shows up in ListSounds. Uses a per-test temp dir as cwd so the
// real project's maps/sounds/ isn't touched.
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

// TestWriteWAV_RefusesEmpty — sanitization that yields "" must error
// instead of writing a "wav" file with no stem.
func TestWriteWAV_RefusesEmpty(t *testing.T) {
	tmp := t.TempDir()
	withWorkingDir(t, tmp, func() {
		_, err := WriteWAV("!!!", []int16{0})
		if err == nil {
			t.Error("WriteWAV with all-strippable name should error")
		}
	})
}

// TestLoadAssignments_RoundTrip writes an assignments file with mixed
// valid + commented + malformed lines, then confirms only the valid
// entries land in the parsed map.
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
			"unknown_cue": "ignored_at_reload", // loader keeps it; reload filters
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

// TestSaveAssignments_DeterministicOrder verifies the on-disk file uses
// sorted key order so diffs stay clean.
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
		// Skip header comment lines.
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

// TestDeleteSound_StripsOrphanAssignments — deleting a user sound
// should also remove any assignment that pointed at it, so a stale
// assignment doesn't survive the file's removal.
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
		// Stale assignment stripped, unrelated assignment preserved.
		got := LoadAssignments()
		if _, exists := got["input_hit"]; exists {
			t.Errorf("expected input_hit assignment to be stripped, got %v", got)
		}
		if got["heal"] != "survivor" {
			t.Errorf("expected heal=survivor, got %v", got)
		}
	})
}

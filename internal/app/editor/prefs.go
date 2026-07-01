package editor

import (
	"os"
	"path/filepath"
	"strings"

	"crawler/internal/app/core"
)

// prefs.go is the editor's tiny cross-session persistence: currently just the
// last-opened map path, so the editor reopens where you left off (NewDefault).
// One "key=value" line per pref; lives beside the maps it points at. Every read/
// write is best-effort — a prefs failure must never break editing.

const (
	editorPrefsFile = "editorprefs.txt"
	lastMapPrefKey  = "lastMap="
)

func editorPrefsPath() string {
	return filepath.Join(core.MapsDir(), editorPrefsFile)
}

// LastMapPath returns the stored last-opened map path, or "" if none / unreadable.
func LastMapPath() string {
	data, err := os.ReadFile(editorPrefsPath())
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, lastMapPrefKey) {
			return strings.TrimSpace(strings.TrimPrefix(line, lastMapPrefKey))
		}
	}
	return ""
}

// rememberLastMap records path as the last-opened map. Best-effort: a write
// failure is swallowed so it can't interrupt an open/save.
func rememberLastMap(path string) {
	if path == "" {
		return
	}
	_ = os.WriteFile(editorPrefsPath(), []byte(lastMapPrefKey+path+"\n"), core.AssetFileMode)
}

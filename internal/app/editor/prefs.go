package editor

import (
	"os"
	"path/filepath"
	"strings"

	"crawler/internal/app/core"
)

// prefs.go is the editor's tiny cross-session persistence: the last-opened map
// (so the editor reopens where you left off) and a short recent-maps list (File
// menu). One "key=value" line per pref; lives beside the maps it points at. Every
// read/write is best-effort — a prefs failure must never break editing.

const (
	editorPrefsFile = "editorprefs.txt"
	lastMapPrefKey  = "lastMap="
	recentPrefKey   = "recent="
	// recentMapsMax caps the File-menu recent list.
	recentMapsMax = 8
)

func editorPrefsPath() string {
	return filepath.Join(core.MapsDir(), editorPrefsFile)
}

// readPrefs parses the prefs file into (lastMap, recent). Missing/unreadable → zero.
func readPrefs() (lastMap string, recent []string) {
	data, err := os.ReadFile(editorPrefsPath())
	if err != nil {
		return "", nil
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, lastMapPrefKey):
			lastMap = strings.TrimSpace(strings.TrimPrefix(line, lastMapPrefKey))
		case strings.HasPrefix(line, recentPrefKey):
			if p := strings.TrimSpace(strings.TrimPrefix(line, recentPrefKey)); p != "" {
				recent = append(recent, p)
			}
		}
	}
	return lastMap, recent
}

// writePrefs persists lastMap + recent (best-effort).
func writePrefs(lastMap string, recent []string) {
	var b strings.Builder
	if lastMap != "" {
		b.WriteString(lastMapPrefKey + lastMap + "\n")
	}
	for _, p := range recent {
		b.WriteString(recentPrefKey + p + "\n")
	}
	_ = os.WriteFile(editorPrefsPath(), []byte(b.String()), core.AssetFileMode)
}

// LastMapPath returns the stored last-opened map path, or "" if none / unreadable.
func LastMapPath() string {
	last, _ := readPrefs()
	return last
}

// RecentMaps returns the recent-opened map paths, newest first.
func RecentMaps() []string {
	_, recent := readPrefs()
	return recent
}

// rememberLastMap records path as the last-opened map and pushes it to the front
// of the recent list (deduped, capped). Best-effort: a write failure is swallowed
// so it can't interrupt an open/save.
func rememberLastMap(path string) {
	if path == "" {
		return
	}
	_, recent := readPrefs()
	out := make([]string, 0, recentMapsMax)
	out = append(out, path)
	for _, p := range recent {
		if p == path {
			continue // dedupe: path is now at the front
		}
		out = append(out, p)
		if len(out) >= recentMapsMax {
			break
		}
	}
	writePrefs(path, out)
}

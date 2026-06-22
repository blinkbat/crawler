package core

import (
	"fmt"
	"path/filepath"
)

// PartyVisualOverride is the on-disk, raylib-free billboard description for a
// party CLASS. Field-for-field identical to EnemyVisualOverride, so it's a type
// alias — one JSON shape, one set of render readers. Authored into
// partyvisuals.json; render overlays it on code defaults. Absent file/class ⇒ default.
type PartyVisualOverride = EnemyVisualOverride

// PartyVisualsFileName is the party override file's basename in the sprites asset dir.
const PartyVisualsFileName = "partyvisuals.json"

// PartyVisualsPath resolves the party override file's on-disk path.
func PartyVisualsPath() string {
	return filepath.Join(ResolveAssetDir(SpritesDirName), PartyVisualsFileName)
}

// PartyClassSlug is the filesystem-safe key for a class (slugified display name,
// "Warrior" → "warrior"). Doubles as the PNG basename and override-map key.
func PartyClassSlug(class PartyClass) string {
	if def, ok := partyClassInfo(class); ok {
		return slugify(def.Name)
	}
	return slugify(fmt.Sprintf("class_%d", int(class)))
}

// LoadPartyVisualOverrides reads the override map keyed by class slug. Missing
// file = empty map (no error); a malformed file IS an error.
func LoadPartyVisualOverrides() (map[string]PartyVisualOverride, error) {
	return loadVisualOverrides(PartyVisualsPath())
}

// SavePartyVisualOverride writes/replaces a single class's override, preserving
// the others (read-modify-write, with the corrupt-file .bak safety net).
func SavePartyVisualOverride(slug string, ov PartyVisualOverride) error {
	return saveVisualOverride(PartyVisualsPath(), slug, ov)
}

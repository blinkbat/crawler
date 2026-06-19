package core

import (
	"fmt"
	"path/filepath"
)

// PartyVisualOverride is the on-disk, raylib-free description of how a single
// party CLASS is drawn as a battle billboard — size, vertical/back nudges,
// contact-shadow disc, target-cursor placement, hit-glyph + particle anchors,
// and a base tint. It is field-for-field identical to EnemyVisualOverride (the
// party and foe billboards share the same alignment knobs and the same render
// apply/convert helpers), so it's a type alias rather than a parallel struct:
// one set of fields, one JSON shape, one set of render readers. The editor's
// Party Visualizer authors these into maps/sprites/partyvisuals.json; render's
// loadPartyVisuals overlays them on the hardcoded code defaults at load time.
// Absent file or absent class ⇒ the code default stands.
type PartyVisualOverride = EnemyVisualOverride

// partyVisualsFileName is the basename of the party override file inside the
// sprites asset dir, beside the foe visuals.json and the authored PNGs.
const partyVisualsFileName = "partyvisuals.json"

// PartyVisualsPath resolves the party override file's on-disk path via the same
// ResolveAssetDir machinery the sprite PNGs and foe visuals use.
func PartyVisualsPath() string {
	return filepath.Join(ResolveAssetDir(SpritesDirName), partyVisualsFileName)
}

// PartyClassSlug is the stable, filesystem-safe key for a party class: a
// slugified form of its display name ("Warrior" → "warrior"). It doubles as
// the authored-PNG basename (warrior.png) and the override-map key, mirroring
// EnemySlug on the foe side.
func PartyClassSlug(class PartyClass) string {
	if def, ok := partyClassInfo(class); ok {
		return slugify(def.Name)
	}
	return slugify(fmt.Sprintf("class_%d", int(class)))
}

// LoadPartyVisualOverrides reads the party override map keyed by class slug. A
// missing file is NOT an error — it returns an empty map so render's overlay
// step is a clean no-op on a fresh checkout. A malformed file IS an error so
// the caller can surface it rather than silently reverting saved tuning.
func LoadPartyVisualOverrides() (map[string]PartyVisualOverride, error) {
	return loadVisualOverrides(PartyVisualsPath())
}

// SavePartyVisualOverride writes (or replaces) a single class's override,
// preserving every other class already in the file. Read-modify-write so the
// editor saving one class never clobbers another's tuning. Mirrors
// SaveEnemyVisualOverride, including the corrupt-file .bak safety net.
func SavePartyVisualOverride(slug string, ov PartyVisualOverride) error {
	return saveVisualOverride(PartyVisualsPath(), slug, ov)
}

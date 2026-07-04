package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// EnemyVisualOverride is the on-disk, raylib-free billboard description for one
// enemy KIND (size, nudges, shadow, cursor, hit-glyph/particle anchors, tint).
// Mirrors render's enemyVisual field-for-field (minus the GPU texture). The Foe
// Visualizer writes these to maps/sprites/visuals.json; render overlays them on
// the code defaults at load. Absent file/kind ⇒ code default stands.
//
// Distances are world units (tile = 1.0); tint channels 0..255 with TintA==0 =
// untinted. *Scale fields multiply a default; ZERO = unset/use 1.0 (so a file
// from before a field existed keeps full-size art, not an invisible 0).
type EnemyVisualOverride struct {
	SizeX         float32 `json:"sizeX"`
	SizeY         float32 `json:"sizeY"`
	YOffset       float32 `json:"yOffset"`
	DepthOffset   float32 `json:"depthOffset"`
	ShadowRadius  float32 `json:"shadowRadius"`
	ShadowOffsetX float32 `json:"shadowOffsetX"`
	ShadowOffsetZ float32 `json:"shadowOffsetZ"`
	MarkerYOffset float32 `json:"markerYOffset"`
	MarkerXOffset float32 `json:"markerXOffset"`
	// MarkerScale multiplies the target chevron size (1 = default).
	MarkerScale float32 `json:"markerScale"`
	// Hit-glyph anchor nudge (camera-right+, world-up+) + size multiplier.
	GlyphXOffset float32 `json:"glyphX"`
	GlyphYOffset float32 `json:"glyphY"`
	GlyphScale   float32 `json:"glyphScale"`
	// Particle-burst anchor nudge (camera-right+, world-up+, camera-forward+) +
	// scale (spread + dot size).
	ParticleXOffset float32 `json:"particleX"`
	ParticleYOffset float32 `json:"particleY"`
	ParticleZOffset float32 `json:"particleZ"`
	ParticleScale   float32 `json:"particleScale"`
	// Damage-NUMBER anchor nudge (camera-right+, world-up+), ADDITIVE on the
	// default rise. Distinct from the hit-GLYPH anchor above.
	PopupXOffset float32 `json:"popupX"`
	PopupYOffset float32 `json:"popupY"`
	TintR        uint8   `json:"tintR"`
	TintG        uint8   `json:"tintG"`
	TintB        uint8   `json:"tintB"`
	TintA        uint8   `json:"tintA"`
	// Non-destructive image adjustments applied at texture-build time (not baked
	// into the PNG). Pixelate 0..1 (point-sampled mosaic); Brightness/Contrast
	// -1..1. Zero on all = untouched.
	Pixelate   float32 `json:"pixelate"`
	Brightness float32 `json:"brightness"`
	Contrast   float32 `json:"contrast"`
	// Palette / retro FX, non-destructive, applied AFTER the tonal adjustments;
	// mirror render/retrofilter.go. Posterize 0..1 crushes color depth;
	// Saturation -1..1 (−1 gray, +1 double); Dither 0..1 (4×4 Bayer); GameBoy
	// 0..1 (4-shade green ramp). Zero on all = untouched.
	Posterize  float32 `json:"posterize"`
	Saturation float32 `json:"saturation"`
	Dither     float32 `json:"dither"`
	GameBoy    float32 `json:"gameBoy"`
	// MaxColors caps the palette to N distinct colors via median-cut on the
	// sprite itself (vs Posterize's fixed grid). Rounded; 0/<2 = no cap. float32
	// to ride the same override + slider plumbing.
	MaxColors float32 `json:"maxColors"`
}

// ColorCap returns the rounded palette-cap count and whether a cap applies
// (MaxColors >= 2; 0/<2 = no cap). One home for the round + threshold rule so the
// render filter doesn't re-encode it.
func (o EnemyVisualOverride) ColorCap() (int, bool) {
	if o.MaxColors < 2 {
		return 0, false
	}
	return RoundToInt(o.MaxColors), true
}

// EnemyVisualsFileName is the override-file basename in the sprites asset dir
// (beside the enemy PNGs).
const EnemyVisualsFileName = "visuals.json"

// EnemyVisualsPath resolves the override file's path via ResolveAssetDir.
func EnemyVisualsPath() string { return spritesFilePath(EnemyVisualsFileName) }

// SpritesDirName is the sprites asset folder (PNGs + visuals.json).
const SpritesDirName = "maps/sprites"

// spritesDir resolves the sprites asset folder; spritesFilePath joins a basename
// onto it. Single home for the ResolveAssetDir(SpritesDirName) construction shared
// by the enemy/party override path + save helpers.
func spritesDir() string                 { return ResolveAssetDir(SpritesDirName) }
func spritesFilePath(name string) string { return filepath.Join(spritesDir(), name) }

// EnemySlug is the filesystem-safe key for an enemy kind: slugify(Name)
// ("Feral Rat" → "feral_rat"). Doubles as the PNG basename + override-map key.
func EnemySlug(kind EnemyKind) string {
	return slugify(EnemyInfo(kind).Name)
}

// slugify lowercases s, collapses non-alphanumeric runs to one underscore, and
// trims edge underscores. On-disk contract for the sprite-asset key (visuals.json
// key + <slug>.png). Distinct from SanitizeFilename / SkillOnDiskName (which only
// lowercase + map spaces→underscores, keeping other punctuation) — don't swap;
// each owns a different on-disk format.
func slugify(s string) string {
	var b strings.Builder
	prevUnderscore := false
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevUnderscore = false
			continue
		}
		if !prevUnderscore {
			b.WriteByte('_')
			prevUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}

// LoadEnemyVisualOverrides reads the override map keyed by enemy slug. Missing
// file = empty map (not an error); malformed file IS an error.
func LoadEnemyVisualOverrides() (map[string]EnemyVisualOverride, error) {
	return loadVisualOverrides(EnemyVisualsPath())
}

// loadVisualOverrides reads a visual-override map from `path` (missing = empty
// map, malformed = error). Shared body behind Load{Enemy,Party}VisualOverrides.
func loadVisualOverrides(path string) (map[string]EnemyVisualOverride, error) {
	blob, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]EnemyVisualOverride{}, nil
		}
		return nil, err
	}
	out := map[string]EnemyVisualOverride{}
	if err := json.Unmarshal(blob, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// SaveEnemyVisualOverride writes (or replaces) one kind's override, preserving
// every other kind in the file (read-modify-write).
func SaveEnemyVisualOverride(slug string, ov EnemyVisualOverride) error {
	return saveVisualOverride(EnemyVisualsPath(), slug, ov)
}

// saveVisualOverride writes (or replaces) one slug's override in `path`,
// preserving every other entry. Shared body behind Save{Enemy,Party}VisualOverride.
func saveVisualOverride(path, slug string, ov EnemyVisualOverride) error {
	all := map[string]EnemyVisualOverride{}
	blob, err := os.ReadFile(path)
	switch {
	case err == nil:
		// Corrupt file: back up the original bytes to .bak (best-effort) so the
		// other foes' tuning is recoverable, then start fresh rather than strand
		// this save.
		if uerr := json.Unmarshal(blob, &all); uerr != nil {
			_ = os.WriteFile(path+".bak", blob, AssetFileMode)
			all = map[string]EnemyVisualOverride{}
		}
	case os.IsNotExist(err):
		// First author — empty map is correct.
	default:
		return err // transient I/O error — refuse rather than clobber
	}
	all[slug] = ov
	return saveVisualOverrides(path, all)
}

// saveVisualOverrides writes the whole map as indented JSON to `path`, creating
// the sprites dir if needed. Shared body behind Save{Enemy,Party}VisualOverrides.
func saveVisualOverrides(path string, all map[string]EnemyVisualOverride) error {
	blob, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(spritesDir(), AssetDirMode); err != nil {
		return err
	}
	return atomicWriteFile(path, blob)
}

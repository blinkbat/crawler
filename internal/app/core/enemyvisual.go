package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// EnemyVisualOverride is the on-disk, raylib-free description of how a single
// enemy KIND is drawn as a battle/field billboard: its size, vertical/back
// nudges, contact-shadow disc, target-cursor placement + size, the anchor +
// size of the in-combat hit-glyph and particle burst, and a base tint. It
// mirrors render's internal enemyVisual struct field-for-field (minus the GPU
// texture, which always comes from the sprite PNG / procedural art — never the
// save file). The editor's Foe Visualizer authors these and writes them to
// maps/sprites/visuals.json; render's loadEnemyVisuals overlays them on top of
// the hardcoded code defaults at load time. Absent file or absent kind ⇒ the
// code default stands, so a fresh checkout renders exactly as before.
//
// All distances are world units (a tile is 1.0 across); tint channels are
// 0..255 with TintA==0 meaning "untinted" (matches render's resolveTint). The
// three *Scale fields multiply a default size — a ZERO value means "unset, use
// the 1.0 default" (matches render's effective*Scale accessors), so a visuals
// file authored before these fields existed keeps full-size glyphs/particles/
// cursor rather than reading the missing field as an invisible 0.
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
	// MarkerScale multiplies the target chevron's silhouette size (1 = default).
	MarkerScale float32 `json:"markerScale"`
	// Hit-glyph anchor nudge + size. GlyphXOffset/GlyphYOffset shift the clarity
	// glyph from the struck sprite's center along camera-right(+ = screen right)
	// and world-up(+); GlyphScale multiplies its on-screen radius (1 = default).
	GlyphXOffset float32 `json:"glyphX"`
	GlyphYOffset float32 `json:"glyphY"`
	GlyphScale   float32 `json:"glyphScale"`
	// Particle-burst anchor nudge + size. ParticleX/Y/ZOffset shift the burst
	// origin along camera-right(+ = screen right), world-up(+), and camera-
	// forward(+ = into the arena); ParticleScale uniformly scales the burst's
	// spread + dot size (1 = default).
	ParticleXOffset float32 `json:"particleX"`
	ParticleYOffset float32 `json:"particleY"`
	ParticleZOffset float32 `json:"particleZ"`
	ParticleScale   float32 `json:"particleScale"`
	// Floating damage-NUMBER anchor nudge. PopupXOffset/PopupYOffset shift where
	// the combat damage popup spawns above the sprite, along camera-right(+ =
	// screen right) and world-up(+). This is ADDITIVE on top of the baked-in
	// default rise, so zero = the historical spot (no migration needed for files
	// written before these fields existed). Distinct from the hit-GLYPH anchor
	// (GlyphX/Y, the clarity symbol) — these move the number itself.
	PopupXOffset float32 `json:"popupX"`
	PopupYOffset float32 `json:"popupY"`
	TintR        uint8   `json:"tintR"`
	TintG        uint8   `json:"tintG"`
	TintB        uint8   `json:"tintB"`
	TintA        uint8   `json:"tintA"`
	// Non-destructive image adjustments applied to the sprite at texture-BUILD
	// time (read from here at load), NOT baked into the PNG — so they persist in
	// visuals.json, reload for further editing, and revert to none by zeroing.
	// Pixelate is a 0..1 mosaic intensity (rendered point-sampled, so the blocks
	// stay crisp); Brightness/Contrast are -1..1 (mapped to the engine's ±255 /
	// ±100 ranges). Zero on all three = the untouched sprite (no migration needed
	// for files written before these fields existed).
	Pixelate   float32 `json:"pixelate"`
	Brightness float32 `json:"brightness"`
	Contrast   float32 `json:"contrast"`
}

// EnemyVisualsFileName is the basename of the override file inside the sprites
// asset dir. Lives beside the authored enemy PNGs (maps/sprites/<slug>.png) so
// "where a foe's look is authored" is one folder. Exported so the editor can
// reference it instead of hardcoding the string.
const EnemyVisualsFileName = "visuals.json"

// EnemyVisualsPath resolves the override file's on-disk path via the same
// ResolveAssetDir machinery the sprite PNGs and sound overrides use (cwd-
// relative for `go run`, next-to-exe for a portable copy).
func EnemyVisualsPath() string {
	return filepath.Join(ResolveAssetDir(SpritesDirName), EnemyVisualsFileName)
}

// SpritesDirName is the sprites asset folder (PNGs + visuals.json).
const SpritesDirName = "maps/sprites"

// EnemySlug is the stable, filesystem-safe key for an enemy kind: a slugified
// form of its display Name ("Feral Rat" → "feral_rat", "Will-o'-Wisp" →
// "will_o_wisp"). It doubles as the authored-PNG basename (feral_rat.png) and
// the override-map key, so one derivation governs both — rename a foe and its
// asset key moves with it (re-author the file under the new slug).
func EnemySlug(kind EnemyKind) string {
	return slugify(EnemyInfo(kind).Name)
}

// slugify lowercases s and collapses every run of non-alphanumeric characters
// into a single underscore, trimming leading/trailing underscores. Stable for
// a given input so it's safe as a persisted key.
//
// On-disk contract: the enemy-visual / sprite-asset KEY (visuals.json key +
// <slug>.png basename). Alphanumeric + single underscore only; punctuation
// and apostrophes vanish ("Will-o'-Wisp" -> "will_o_wisp"). Intentionally NOT
// the same as SanitizeFilename (areas.go — keeps hyphens, strips other
// punctuation, has a fallback) or SanitizeCustomEnemyName (customenemy.go —
// preserves case + punctuation, only folds whitespace to underscore). Don't
// swap one for another; each owns a different on-disk format.
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

// LoadEnemyVisualOverrides reads the override map keyed by enemy slug. A
// missing file is NOT an error — it returns an empty map so render's overlay
// step is a clean no-op on a fresh checkout. A malformed file IS an error so
// the caller can surface "your visuals.json is broken" rather than silently
// reverting the author's saved tuning.
func LoadEnemyVisualOverrides() (map[string]EnemyVisualOverride, error) {
	return loadVisualOverrides(EnemyVisualsPath())
}

// loadVisualOverrides reads a visual-override map (enemy or party — the structs
// alias) from `path`. A missing file is NOT an error (empty map); a malformed
// one IS. Shared body behind Load{Enemy,Party}VisualOverrides.
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

// SaveEnemyVisualOverride writes (or replaces) a single kind's override,
// preserving every other kind already in the file. Read-modify-write so the
// editor saving one foe never clobbers another foe's saved tuning. Creates the
// sprites dir if it doesn't exist yet (first-author case).
func SaveEnemyVisualOverride(slug string, ov EnemyVisualOverride) error {
	return saveVisualOverride(EnemyVisualsPath(), slug, ov)
}

// saveVisualOverride writes (or replaces) a single slug's override in the file
// at `path`, preserving every other entry. Shared body behind
// Save{Enemy,Party}VisualOverride, including the corrupt-file .bak safety net.
func saveVisualOverride(path, slug string, ov EnemyVisualOverride) error {
	all := map[string]EnemyVisualOverride{}
	blob, err := os.ReadFile(path)
	switch {
	case err == nil:
		// Merge into the existing file. A corrupt (unparseable) file shouldn't
		// strand the author from saving THIS foe — but overwriting it would
		// silently destroy every OTHER foe's tuning that the bad bytes still
		// hold (one stray character is enough to fail the parse). Preserve the
		// original bytes in a sibling .bak first (best-effort) so the author
		// can recover the rest, THEN start fresh. A genuine read error (locked
		// file, bad permissions) is surfaced below instead of swallowed.
		if uerr := json.Unmarshal(blob, &all); uerr != nil {
			_ = os.WriteFile(path+".bak", blob, AssetFileMode)
			all = map[string]EnemyVisualOverride{}
		}
	case os.IsNotExist(err):
		// First author — the empty map is correct.
	default:
		// Transient I/O error: refuse rather than clobber the whole file with
		// just this one slug.
		return err
	}
	all[slug] = ov
	return saveVisualOverrides(path, all)
}

// saveVisualOverrides writes the whole override map as indented JSON to `path`,
// creating the sprites asset dir if needed. Shared body behind
// Save{Enemy,Party}VisualOverrides.
func saveVisualOverrides(path string, all map[string]EnemyVisualOverride) error {
	blob, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(ResolveAssetDir(SpritesDirName), AssetDirMode); err != nil {
		return err
	}
	return os.WriteFile(path, blob, AssetFileMode)
}

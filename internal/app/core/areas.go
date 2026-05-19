package core

import (
	"crawler/internal/app/core/mapfile"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	MaterialDungeon MaterialSet = iota
	MaterialField
)

// mapsDirName is the on-disk folder name where .map files live. Single
// source so renames are a one-line edit; MapsDir resolves it to an
// absolute or cwd-relative path at runtime.
const mapsDirName = "maps"

// AssetDirMode is the os.MkdirAll mode used for every auto-created asset
// directory (maps/, maps/sounds/, etc.). Centralized so a project-wide
// permissions change is one edit instead of grep-and-replace.
const AssetDirMode = 0o755

// MapsDir returns the directory where .map files live. Thin wrapper
// over ResolveAssetDir so the asset-folder lookup story is consistent
// with maps/sounds/ and any future asset directory.
func MapsDir() string {
	return ResolveAssetDir(mapsDirName)
}

// ResolveAssetDir resolves a relative asset folder name (e.g. "maps" or
// "maps/sounds") to a usable on-disk path. Resolution order: prefer the
// cwd-relative form (so `go run` from the repo root works) → fall back
// to the same path next to the running executable (so a portable copy
// of the binary works from any cwd) → fall back to the cwd-relative
// form for the first-run case so the caller's first write creates it
// where the user is.
//
// Used by core.MapsDir, audio.SoundsDir, and any future asset-folder
// helper so the resolution machinery isn't duplicated per asset type.
func ResolveAssetDir(rel string) string {
	if DirExists(rel) {
		return rel
	}
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), rel)
		if DirExists(candidate) {
			return candidate
		}
	}
	return rel
}

// DirExists is the canonical "is `path` an existing directory?" check.
// Shared by the asset-dir resolvers so they don't each carry their own
// private dirExists helper.
func DirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// LoadArea reads a .map file from disk and converts it into the in-memory
// AreaDefinition the game state expects.
func LoadArea(path string) (AreaDefinition, error) {
	mf, err := mapfile.Load(path)
	if err != nil {
		return AreaDefinition{}, err
	}
	return AreaFromMapFile(mf, path)
}

// AreaFromMapFile is the converter half of LoadArea, exposed so the editor
// can build a runnable area from in-memory edits without touching disk.
func AreaFromMapFile(mf mapfile.MapFile, path string) (AreaDefinition, error) {
	mat, ok := materialFromName(mf.Materials)
	if !ok {
		return AreaDefinition{}, fmt.Errorf("unknown material set %q", mf.Materials)
	}
	face, ok := facingFromName(mf.StartFace)
	if !ok {
		return AreaDefinition{}, fmt.Errorf("unknown facing %q", mf.StartFace)
	}
	// Dimensions and start position must be in-bounds before the rest of
	// the runtime trusts them. Without this, a corrupt .map file (zero
	// width, OOB start) reaches the renderer / WallAt / movement and
	// panics on the first index. The editor's reachability check guards
	// the F5 path; this guards Load-from-disk.
	if mf.Width <= 0 || mf.Height <= 0 {
		return AreaDefinition{}, fmt.Errorf("map dimensions must be positive (got %dx%d)", mf.Width, mf.Height)
	}
	layers := []struct {
		name string
		rows []string
	}{
		{"walls", mf.Walls},
		{"floor", mf.Floor},
		{"decor", mf.Decor},
		{"props", mf.Props},
	}
	for _, layer := range layers {
		if len(layer.rows) != mf.Height {
			return AreaDefinition{}, fmt.Errorf("%s layer has %d rows, declared height %d", layer.name, len(layer.rows), mf.Height)
		}
		for i, row := range layer.rows {
			if len(row) != mf.Width {
				return AreaDefinition{}, fmt.Errorf("%s layer row %d has width %d, want %d", layer.name, i, len(row), mf.Width)
			}
		}
	}
	if mf.StartX < 0 || mf.StartX >= mf.Width || mf.StartZ < 0 || mf.StartZ >= mf.Height {
		return AreaDefinition{}, fmt.Errorf("start position (%d,%d) is out of bounds for %dx%d", mf.StartX, mf.StartZ, mf.Width, mf.Height)
	}
	spawns := make([]PackSpawn, 0, len(mf.Packs))
	for _, p := range mf.Packs {
		if len(p.Members) == 0 {
			return AreaDefinition{}, fmt.Errorf("pack at (%d,%d) has no members", p.X, p.Z)
		}
		members := make([]EnemyKind, 0, len(p.Members))
		for _, name := range p.Members {
			kind, ok := EnemyKindFromName(name)
			if !ok {
				return AreaDefinition{}, fmt.Errorf("unknown enemy kind %q", name)
			}
			members = append(members, kind)
		}
		spawns = append(spawns, PackSpawn{TileX: p.X, TileZ: p.Z, Members: members})
	}
	chests := make([]ChestSpawn, 0, len(mf.Chests))
	for _, c := range mf.Chests {
		kinds := make([]ItemKind, 0, len(c.Items))
		for _, name := range c.Items {
			kind := ItemKindByName(name)
			if kind == ItemNone {
				return AreaDefinition{}, fmt.Errorf("unknown chest item %q at (%d,%d)", name, c.X, c.Z)
			}
			kinds = append(kinds, kind)
		}
		chests = append(chests, ChestSpawn{TileX: c.X, TileZ: c.Z, Items: kinds})
	}
	ceiling := mf.Ceiling
	if len(ceiling) == 0 {
		ceiling = mapfile.BlankLayer(mf.Width, mf.Height, TileCeilingOpen)
	}
	return AreaDefinition{
		Path:         path,
		Name:         mf.Name,
		Width:        mf.Width,
		Height:       mf.Height,
		Walls:        append([]string(nil), mf.Walls...),
		Floor:        append([]string(nil), mf.Floor...),
		Decor:        append([]string(nil), mf.Decor...),
		Props:        append([]string(nil), mf.Props...),
		Ceiling:      append([]string(nil), ceiling...),
		Materials:    mat,
		StartTileX:   mf.StartX,
		StartTileZ:   mf.StartZ,
		StartFacing:  face,
		PackSpawns:   spawns,
		ChestSpawns:  chests,
		QuietMessage: mf.Quiet,
	}, nil
}

// MapFileFromArea is the reverse converter — used by the editor to write the
// current in-memory area back to disk. Returns an error rather than silently
// substituting a default name when an enum value falls outside the registry:
// a corrupted material / facing / enemy kind in memory is far more useful as
// a refused save than as a silent rewrite to "dungeon" / "east" / "rat".
func MapFileFromArea(a AreaDefinition) (mapfile.MapFile, error) {
	matName, ok := MaterialName(a.Materials)
	if !ok {
		return mapfile.MapFile{}, fmt.Errorf("unknown material set %d", int(a.Materials))
	}
	faceName, ok := FacingName(a.StartFacing)
	if !ok {
		return mapfile.MapFile{}, fmt.Errorf("unknown start facing %d", a.StartFacing)
	}
	packs := make([]mapfile.MapPack, 0, len(a.PackSpawns))
	for _, s := range a.PackSpawns {
		names := make([]string, 0, len(s.Members))
		for _, kind := range s.Members {
			name, ok := EnemyKindName(kind)
			if !ok {
				return mapfile.MapFile{}, fmt.Errorf("unknown enemy kind %d in pack at (%d,%d)", int(kind), s.TileX, s.TileZ)
			}
			names = append(names, name)
		}
		packs = append(packs, mapfile.MapPack{
			Members: names,
			X:       s.TileX,
			Z:       s.TileZ,
		})
	}
	chests := make([]mapfile.MapChest, 0, len(a.ChestSpawns))
	for _, c := range a.ChestSpawns {
		names := make([]string, 0, len(c.Items))
		for _, kind := range c.Items {
			info := ItemInfo(kind)
			if info.Name == "" || info.Name == "Unknown Item" {
				return mapfile.MapFile{}, fmt.Errorf("unknown chest item kind %d at (%d,%d)", int(kind), c.TileX, c.TileZ)
			}
			names = append(names, info.Name)
		}
		chests = append(chests, mapfile.MapChest{Items: names, X: c.TileX, Z: c.TileZ})
	}
	ceiling := a.Ceiling
	if len(ceiling) == 0 {
		ceiling = mapfile.BlankLayer(a.Width, a.Height, TileCeilingOpen)
	}
	return mapfile.MapFile{
		Name:      a.Name,
		Materials: matName,
		Quiet:     a.QuietMessage,
		Width:     a.Width,
		Height:    a.Height,
		StartX:    a.StartTileX,
		StartZ:    a.StartTileZ,
		StartFace: faceName,
		Walls:     append([]string(nil), a.Walls...),
		Floor:     append([]string(nil), a.Floor...),
		Decor:     append([]string(nil), a.Decor...),
		Props:     append([]string(nil), a.Props...),
		Ceiling:   append([]string(nil), ceiling...),
		Packs:     packs,
		Chests:    chests,
	}, nil
}

// --- Table-driven name <-> enum lookups ---
//
// Each registry is a single slice of (enum, primary-name, alias-names...)
// tuples. The Name function looks up by enum; the FromName function looks
// up by case-folded name (matching any primary or alias). Both directions
// share the same source of truth, so adding a new enum value is a one-line
// edit instead of a "find the three switch statements" hunt — and an
// unknown value returns ok=false instead of silently coercing to the first
// option (which used to rewrite save data on the save path).

type materialNameEntry struct {
	value MaterialSet
	name  string
}

var materialNameTable = []materialNameEntry{
	{MaterialDungeon, "dungeon"},
	{MaterialField, "field"},
}

// MaterialName returns the canonical on-disk name for the material set,
// plus ok=false when the value is out of range. Callers that write to
// .map files should propagate the failure rather than silently committing
// a wrong material name.
func MaterialName(m MaterialSet) (string, bool) {
	for _, e := range materialNameTable {
		if e.value == m {
			return e.name, true
		}
	}
	return "", false
}

func materialFromName(s string) (MaterialSet, bool) {
	low := strings.ToLower(s)
	for _, e := range materialNameTable {
		if e.name == low {
			return e.value, true
		}
	}
	return 0, false
}

// MaterialOptions is the editor's dropdown order. Stable so palette colors
// stay associated with the right material in the UI.
var MaterialOptions = []MaterialSet{MaterialDungeon, MaterialField}

type facingNameEntry struct {
	value int
	name  string
}

var facingNameTable = []facingNameEntry{
	{North, "north"},
	{East, "east"},
	{South, "south"},
	{West, "west"},
}

// FacingName returns the canonical on-disk name for a facing. ok=false
// only when normalization produces a value out of range, which can't
// happen for the four legitimate enum values.
func FacingName(f int) (string, bool) {
	n := NormalizeFacing(f)
	for _, e := range facingNameTable {
		if e.value == n {
			return e.name, true
		}
	}
	return "", false
}

func facingFromName(s string) (int, bool) {
	low := strings.ToLower(s)
	for _, e := range facingNameTable {
		if e.name == low {
			return e.value, true
		}
	}
	return 0, false
}

type enemyKindNameEntry struct {
	value   EnemyKind
	name    string
	aliases []string
}

var enemyKindNameTable = []enemyKindNameEntry{
	{EnemyRat, "rat", nil},
	{EnemyBat, "bat", nil},
	// "diseasedrat" is accepted as a legacy alias for files saved before
	// the underscore-separated form became canonical.
	{EnemyDiseasedRat, "diseased_rat", []string{"diseasedrat"}},
	{EnemyGoblin, "goblin", nil},
	{EnemyGoblinMage, "goblin_mage", []string{"goblinmage"}},
	{EnemyAmoeba, "amoeba", nil},
	{EnemyVenusMantrap, "venus_mantrap", []string{"mantrap", "venusmantrap"}},
}

// enemyKindByName flattens enemyKindNameTable into a single primary+alias
// lookup so EnemyKindFromName doesn't double-loop. Keys are pre-lowercased
// since callers always normalize.
var enemyKindByName = buildEnemyKindByName()

func buildEnemyKindByName() map[string]EnemyKind {
	m := make(map[string]EnemyKind, len(enemyKindNameTable))
	for _, e := range enemyKindNameTable {
		m[e.name] = e.value
		for _, alias := range e.aliases {
			m[alias] = e.value
		}
	}
	return m
}

// EnemyKindName returns the canonical on-disk name for the enemy kind,
// plus ok=false on unknown values. MapFileFromArea propagates the failure
// to caller — better to refuse a save than silently rewrite enemy types.
func EnemyKindName(k EnemyKind) (string, bool) {
	for _, e := range enemyKindNameTable {
		if e.value == k {
			return e.name, true
		}
	}
	return "", false
}

func EnemyKindFromName(s string) (EnemyKind, bool) {
	kind, ok := enemyKindByName[strings.ToLower(s)]
	return kind, ok
}

// SanitizeFilename normalizes a user-provided string into a safe on-disk
// filename stem — lowercase ASCII letters, digits, underscore, hyphen
// only. Spaces fold to underscore; everything else strips. If `fallback`
// is non-empty it's returned when the input has nothing usable left;
// otherwise the empty string comes back so the caller can refuse the
// save. Shared by the editor's map-save flow and audio's user-sound
// save so both apply the same character-class contract.
func SanitizeFilename(name, fallback string) string {
	out := strings.ToLower(strings.TrimSpace(name))
	out = strings.ReplaceAll(out, " ", "_")
	cleaned := make([]byte, 0, len(out))
	for i := 0; i < len(out); i++ {
		c := out[i]
		switch {
		case c >= 'a' && c <= 'z':
			cleaned = append(cleaned, c)
		case c >= '0' && c <= '9':
			cleaned = append(cleaned, c)
		case c == '_' || c == '-':
			cleaned = append(cleaned, c)
		}
	}
	if len(cleaned) == 0 {
		return fallback
	}
	return string(cleaned)
}

// MapPath returns the canonical save path for a map ID under MapsDir.
// Strips a trailing ".map" (case-insensitive) from the id before appending
// the canonical ".map" extension — so a user typing "test.map" in the
// Save As field writes to maps/test.map, not maps/test.map.map.
func MapPath(id string) string {
	if len(id) >= 4 && strings.EqualFold(id[len(id)-4:], ".map") {
		id = id[:len(id)-4]
	}
	return filepath.Join(MapsDir(), id+".map")
}

// MapIDFromPath strips the directory and .map extension off a map path.
func MapIDFromPath(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

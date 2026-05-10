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

// MapsDir returns the directory where .map files live. It prefers a cwd-
// relative `maps/` (so `go run` from the repo root works), then falls back
// to a `maps/` directory next to the running executable (so a portable
// copy of the binary works from any cwd). When neither exists yet (first
// run), returns "maps" so the editor's first save creates it cwd-relative.
func MapsDir() string {
	if dirExists("maps") {
		return "maps"
	}
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), "maps")
		if dirExists(candidate) {
			return candidate
		}
	}
	return "maps"
}

func dirExists(path string) bool {
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
	spawns := make([]EnemySpawn, 0, len(mf.Enemies))
	for _, e := range mf.Enemies {
		kind, ok := EnemyKindFromName(e.Kind)
		if !ok {
			return AreaDefinition{}, fmt.Errorf("unknown enemy kind %q", e.Kind)
		}
		spawns = append(spawns, EnemySpawn{Kind: kind, TileX: e.X, TileZ: e.Z})
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
		Materials:    mat,
		StartTileX:   mf.StartX,
		StartTileZ:   mf.StartZ,
		StartFacing:  face,
		EnemySpawns:  spawns,
		QuietMessage: mf.Quiet,
	}, nil
}

// MapFileFromArea is the reverse converter — used by the editor to write the
// current in-memory area back to disk.
func MapFileFromArea(a AreaDefinition) mapfile.MapFile {
	enemies := make([]mapfile.MapEnemy, 0, len(a.EnemySpawns))
	for _, s := range a.EnemySpawns {
		enemies = append(enemies, mapfile.MapEnemy{
			Kind: EnemyKindName(s.Kind),
			X:    s.TileX,
			Z:    s.TileZ,
		})
	}
	return mapfile.MapFile{
		Name:      a.Name,
		Materials: MaterialName(a.Materials),
		Quiet:     a.QuietMessage,
		Width:     a.Width,
		Height:    a.Height,
		StartX:    a.StartTileX,
		StartZ:    a.StartTileZ,
		StartFace: FacingName(a.StartFacing),
		Walls:     append([]string(nil), a.Walls...),
		Floor:     append([]string(nil), a.Floor...),
		Decor:     append([]string(nil), a.Decor...),
		Props:     append([]string(nil), a.Props...),
		Enemies:   enemies,
	}
}

func MaterialName(m MaterialSet) string {
	switch m {
	case MaterialDungeon:
		return "dungeon"
	case MaterialField:
		return "field"
	}
	return "dungeon"
}

func materialFromName(s string) (MaterialSet, bool) {
	switch strings.ToLower(s) {
	case "dungeon":
		return MaterialDungeon, true
	case "field":
		return MaterialField, true
	}
	return 0, false
}

// MaterialOptions is the editor's dropdown order. Stable so palette colors
// stay associated with the right material in the UI.
var MaterialOptions = []MaterialSet{MaterialDungeon, MaterialField}

func FacingName(f int) string {
	switch NormalizeFacing(f) {
	case North:
		return "north"
	case East:
		return "east"
	case South:
		return "south"
	case West:
		return "west"
	}
	return "east"
}

func facingFromName(s string) (int, bool) {
	switch strings.ToLower(s) {
	case "north":
		return North, true
	case "east":
		return East, true
	case "south":
		return South, true
	case "west":
		return West, true
	}
	return 0, false
}

func EnemyKindName(k EnemyKind) string {
	switch k {
	case EnemyRat:
		return "rat"
	case EnemyBat:
		return "bat"
	}
	return "rat"
}

func EnemyKindFromName(s string) (EnemyKind, bool) {
	switch strings.ToLower(s) {
	case "rat":
		return EnemyRat, true
	case "bat":
		return EnemyBat, true
	}
	return 0, false
}

// MapPath returns the canonical save path for a map ID under MapsDir.
func MapPath(id string) string {
	return filepath.Join(MapsDir(), id+".map")
}

// MapIDFromPath strips the directory and .map extension off a map path.
func MapIDFromPath(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

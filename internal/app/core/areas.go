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
		PackSpawns:   spawns,
		QuietMessage: mf.Quiet,
	}, nil
}

// MapFileFromArea is the reverse converter — used by the editor to write the
// current in-memory area back to disk.
func MapFileFromArea(a AreaDefinition) mapfile.MapFile {
	packs := make([]mapfile.MapPack, 0, len(a.PackSpawns))
	for _, s := range a.PackSpawns {
		names := make([]string, 0, len(s.Members))
		for _, kind := range s.Members {
			names = append(names, EnemyKindName(kind))
		}
		packs = append(packs, mapfile.MapPack{
			Members: names,
			X:       s.TileX,
			Z:       s.TileZ,
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
		Packs:     packs,
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
	case EnemyDiseasedRat:
		return "diseased_rat"
	}
	return "rat"
}

func EnemyKindFromName(s string) (EnemyKind, bool) {
	switch strings.ToLower(s) {
	case "rat":
		return EnemyRat, true
	case "bat":
		return EnemyBat, true
	case "diseased_rat", "diseasedrat":
		return EnemyDiseasedRat, true
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

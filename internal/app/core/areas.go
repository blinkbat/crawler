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
	// MaterialCount is the number of material sets. The init guard in
	// this file asserts materialDefs covers every value below it, and
	// render's NewResources asserts a worldMaterial exists for each.
	MaterialCount
)

// mapsDirName is the on-disk folder name where .map files live. Single
// source so renames are a one-line edit; MapsDir resolves it to an
// absolute or cwd-relative path at runtime.
const mapsDirName = "maps"

// AssetDirMode / AssetFileMode are the os mode bits for every auto-created
// asset directory (maps/, maps/sounds/, …) and asset file write (.map
// files, user .wav sounds, assignments.txt). They alias the canonical
// definitions in the leaf mapfile package so the I/O layer can use them
// without importing core; a project-wide permissions change is one edit.
const (
	AssetDirMode  = mapfile.AssetDirMode
	AssetFileMode = mapfile.AssetFileMode
)

// SelfMapToken is the door TargetMap placeholder meaning "this same map".
// Re-exported from mapfile so runtime/render callers can recognize a
// self-portal without importing the leaf I/O package.
const SelfMapToken = mapfile.SelfMapToken

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
// Used by core.MapsDir, audio/userconfig.SoundsDir, and any future
// asset-folder helper so the resolution machinery isn't duplicated
// per asset type.
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
	customs := make([]CustomEnemyDef, 0, len(mf.CustomEnemies))
	for _, ce := range mf.CustomEnemies {
		def, err := CustomEnemyDefFromMap(ce)
		if err != nil {
			return AreaDefinition{}, err
		}
		customs = append(customs, def)
	}
	spawns := make([]PackSpawn, 0, len(mf.Packs))
	for _, p := range mf.Packs {
		if len(p.Members) == 0 {
			return AreaDefinition{}, fmt.Errorf("pack at (%d,%d) has no members", p.X, p.Z)
		}
		members := make([]PackMemberRef, 0, len(p.Members))
		for _, name := range p.Members {
			if kind, ok := EnemyKindFromName(name); ok {
				members = append(members, BuiltinPackMember(kind))
				continue
			}
			def, ok := CustomEnemyByName(customs, name)
			if !ok {
				return AreaDefinition{}, fmt.Errorf("unknown enemy kind or custom enemy %q", name)
			}
			members = append(members, CustomPackMember(def))
		}
		spawns = append(spawns, PackSpawn{TileX: p.X, TileZ: p.Z, Members: members, AI: PackAIFromName(p.AI)})
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
	// Doors round-trip. A same-map portal keeps SelfMapToken in the runtime
	// rather than being expanded to the concrete map id here — the transition
	// resolver (run.applyAreaTransition) and the editor validator both
	// understand the placeholder, and keeping it means the self-link survives
	// a map rename. Expanding it used to leave a stale id behind after a
	// rename, which then re-serialized as an explicit (now-wrong) cross-map
	// target. Display sites (the door prompt) resolve "self" to the current
	// area name.
	doors := make([]DoorSpawn, 0, len(mf.Doors))
	for _, d := range mf.Doors {
		facing, ok := facingFromName(d.Facing)
		if !ok {
			return AreaDefinition{}, fmt.Errorf("door %q has bad facing %q", d.Name, d.Facing)
		}
		doors = append(doors, DoorSpawn{
			TileX:      d.X,
			TileZ:      d.Z,
			Name:       d.Name,
			TargetMap:  d.TargetMap,
			TargetDoor: d.TargetDoor,
			Facing:     facing,
			Style:      doorStyleFromName(d.Style),
		})
	}
	ceiling := mf.Ceiling
	if len(ceiling) == 0 {
		ceiling = mapfile.BlankLayer(mf.Width, mf.Height, TileCeilingOpen)
	}
	elevation := mf.Elevation
	if len(elevation) == 0 {
		elevation = mapfile.BlankLayer(mf.Width, mf.Height, ElevationGround)
	}
	return AreaDefinition{
		Path:          path,
		Name:          mf.Name,
		Width:         mf.Width,
		Height:        mf.Height,
		Walls:         append([]string(nil), mf.Walls...),
		Floor:         append([]string(nil), mf.Floor...),
		Decor:         append([]string(nil), mf.Decor...),
		Props:         append([]string(nil), mf.Props...),
		Ceiling:       append([]string(nil), ceiling...),
		Elevation:     append([]string(nil), elevation...),
		Materials:     mat,
		StartTileX:    mf.StartX,
		StartTileZ:    mf.StartZ,
		StartFacing:   face,
		PackSpawns:    spawns,
		ChestSpawns:   chests,
		DoorSpawns:    doors,
		CustomEnemies: customs,
		QuietMessage:  mf.Quiet,
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
		for _, member := range s.Members {
			if customName := member.CustomName; customName != "" {
				safeName := SanitizeCustomEnemyName(customName)
				if safeName == "" {
					return mapfile.MapFile{}, fmt.Errorf("custom enemy member at (%d,%d) has empty name after sanitize", s.TileX, s.TileZ)
				}
				if _, ok := CustomEnemyByName(a.CustomEnemies, safeName); !ok {
					return mapfile.MapFile{}, fmt.Errorf("unknown custom enemy %q in pack at (%d,%d)", customName, s.TileX, s.TileZ)
				}
				names = append(names, safeName)
				continue
			}
			name, ok := EnemyKindName(member.Kind)
			if !ok {
				return mapfile.MapFile{}, fmt.Errorf("unknown enemy kind %d in pack at (%d,%d)", int(member.Kind), s.TileX, s.TileZ)
			}
			names = append(names, name)
		}
		packs = append(packs, mapfile.MapPack{
			Members: names,
			X:       s.TileX,
			Z:       s.TileZ,
			AI:      PackAIName(s.AI),
		})
	}
	chests := make([]mapfile.MapChest, 0, len(a.ChestSpawns))
	for _, c := range a.ChestSpawns {
		names := make([]string, 0, len(c.Items))
		for _, kind := range c.Items {
			info, ok := ItemInfoOk(kind)
			if !ok || info.Name == "" {
				return mapfile.MapFile{}, fmt.Errorf("unknown chest item kind %d at (%d,%d)", int(kind), c.TileX, c.TileZ)
			}
			names = append(names, info.Name)
		}
		chests = append(chests, mapfile.MapChest{Items: names, X: c.TileX, Z: c.TileZ})
	}
	localMapID := ""
	if a.Path != "" {
		localMapID = MapIDFromPath(a.Path)
	}
	doors := make([]mapfile.MapDoor, 0, len(a.DoorSpawns))
	for _, d := range a.DoorSpawns {
		faceName, ok := FacingName(d.Facing)
		if !ok {
			return mapfile.MapFile{}, fmt.Errorf("door %q has bad facing %d", d.Name, d.Facing)
		}
		// Encode same-map portals back to SelfMapToken so a moved/renamed
		// map keeps its internal links intact. Cross-map targets stay as
		// their explicit map id.
		target := d.TargetMap
		if localMapID != "" && target == localMapID {
			target = mapfile.SelfMapToken
		}
		doors = append(doors, mapfile.MapDoor{
			Name:       d.Name,
			TargetMap:  target,
			TargetDoor: d.TargetDoor,
			X:          d.TileX,
			Z:          d.TileZ,
			Facing:     faceName,
			Style:      DoorStyleName(d.Style),
		})
	}
	ceiling := a.Ceiling
	if len(ceiling) == 0 {
		ceiling = mapfile.BlankLayer(a.Width, a.Height, TileCeilingOpen)
	}
	elevation := a.Elevation
	if len(elevation) == 0 {
		elevation = mapfile.BlankLayer(a.Width, a.Height, ElevationGround)
	}
	customs := make([]mapfile.MapCustomEnemy, 0, len(a.CustomEnemies))
	for _, ce := range a.CustomEnemies {
		mapCE, err := MapCustomEnemyFromDef(ce)
		if err != nil {
			return mapfile.MapFile{}, err
		}
		customs = append(customs, mapCE)
	}
	return mapfile.MapFile{
		Name:          a.Name,
		Materials:     matName,
		Quiet:         a.QuietMessage,
		Width:         a.Width,
		Height:        a.Height,
		StartX:        a.StartTileX,
		StartZ:        a.StartTileZ,
		StartFace:     faceName,
		Walls:         append([]string(nil), a.Walls...),
		Floor:         append([]string(nil), a.Floor...),
		Decor:         append([]string(nil), a.Decor...),
		Props:         append([]string(nil), a.Props...),
		Ceiling:       append([]string(nil), ceiling...),
		Elevation:     append([]string(nil), elevation...),
		Packs:         packs,
		Chests:        chests,
		Doors:         doors,
		CustomEnemies: customs,
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

// namedEnum is one row of a value↔canonical-name table. Shared by the
// material and facing registries so their forward (value→name) scan lives
// once in lookupName. (The enemy-kind table carries an extra aliases
// field and keeps its own entry type + scan.)
type namedEnum[V comparable] struct {
	value V
	name  string
}

// lookupName scans a namedEnum table for `target`, returning its
// canonical name (ok=false on no match). The single forward-scan shared
// by MaterialName and FacingName.
func lookupName[V comparable](table []namedEnum[V], target V) (string, bool) {
	for _, e := range table {
		if e.value == target {
			return e.name, true
		}
	}
	return "", false
}

// materialDef is one row of the material registry: the canonical on-disk
// name plus per-material traits. indoor marks an enclosed interior (stone
// walls, ceiling slabs by default) vs. an outdoor biome — minimap tone,
// the world/wall pass, lighting profiles, and resource fallbacks branch on
// it, so a future cave/crypt material is one row here, not a grep for
// `== MaterialDungeon`. Single source for name lookup, editor dropdown
// order, and the indoor predicate so they can't drift.
type materialDef struct {
	value  MaterialSet
	name   string
	indoor bool
}

var materialDefs = []materialDef{
	{MaterialDungeon, "dungeon", true},
	{MaterialField, "field", false},
}

func init() {
	if len(materialDefs) != int(MaterialCount) {
		panic("core: materialDefs must have one row per MaterialSet — add a row when adding a material")
	}
	seen := make([]bool, int(MaterialCount))
	for _, d := range materialDefs {
		if int(d.value) < 0 || int(d.value) >= int(MaterialCount) {
			panic("core: materialDefs row has an out-of-range MaterialSet value")
		}
		if seen[d.value] {
			panic("core: materialDefs has a duplicate MaterialSet row")
		}
		seen[d.value] = true
	}
}

// findMaterialDef is the single forward scan over the material registry,
// shared by MaterialName / MaterialIsIndoor so they can't drift on lookup
// logic (materialFromName scans by name instead). Returns ok=false when
// the value is out of range.
func findMaterialDef(m MaterialSet) (materialDef, bool) {
	for _, d := range materialDefs {
		if d.value == m {
			return d, true
		}
	}
	return materialDef{}, false
}

// MaterialName returns the canonical on-disk name for the material set,
// plus ok=false when the value is out of range. Callers that write to
// .map files should propagate the failure rather than silently committing
// a wrong material name.
func MaterialName(m MaterialSet) (string, bool) {
	d, ok := findMaterialDef(m)
	return d.name, ok
}

func materialFromName(s string) (MaterialSet, bool) {
	low := strings.ToLower(s)
	for _, d := range materialDefs {
		if d.name == low {
			return d.value, true
		}
	}
	return 0, false
}

// MaterialOptions is the editor's dropdown order, derived from
// materialDefs so the two can't drift — a material added to the registry
// shows up in the editor dropdown automatically, in the same (stable)
// order so palette colors stay associated with the right material.
var MaterialOptions = buildMaterialOptions()

func buildMaterialOptions() []MaterialSet {
	opts := make([]MaterialSet, len(materialDefs))
	for i, d := range materialDefs {
		opts[i] = d.value
	}
	return opts
}

// MaterialIsIndoor reports whether the material set represents an
// enclosed interior (stone walls, ceiling slabs by default) vs. an
// outdoor biome, reading the trait off the material registry row.
func MaterialIsIndoor(m MaterialSet) bool {
	d, _ := findMaterialDef(m)
	return d.indoor
}

var facingNameTable = []namedEnum[int]{
	{North, mapfile.FacingNorthName},
	{East, mapfile.FacingEastName},
	{South, mapfile.FacingSouthName},
	{West, mapfile.FacingWestName},
}

// FacingShortLabels returns the single-letter UI labels for the four
// facings, indexed by core.North/East/South/West. Centralized so the
// editor's metadata panel and door-edit modal don't each carry their
// own []string{"N", "E", "S", "W"} literal — a future renaming
// (localisation, glyph swap) is one edit instead of a grep.
var FacingShortLabels = [FacingCount]string{
	North: "N",
	East:  "E",
	South: "S",
	West:  "W",
}

// FacingName returns the canonical on-disk name for a facing. ok=false
// only when normalization produces a value out of range, which can't
// happen for the four legitimate enum values.
func FacingName(f int) (string, bool) {
	return lookupName(facingNameTable, NormalizeFacing(f))
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

// FacingAwayFromAdjacentWall scans the four cardinal neighbours of
// (x, z) in N→E→S→W order and returns the facing pointing AWAY from the
// first wall found — the direction something mounted on that wall (a wall
// torch, a door set into it) should face into the room. found=false when
// the cell has no adjacent wall, so each caller picks its own fallback.
// Single source for the rule shared by the renderer's wall-torch
// orientation and the editor's door auto-facing.
func FacingAwayFromAdjacentWall(m AreaDefinition, x, z int) (facing int, found bool) {
	switch {
	case m.WallAt(x, z-1):
		return South, true // wall north → face south, into the room
	case m.WallAt(x+1, z):
		return West, true
	case m.WallAt(x, z+1):
		return North, true
	case m.WallAt(x-1, z):
		return East, true
	}
	return 0, false
}

// DoorStyleLabels are the editor button labels for the door styles,
// indexed by DoorStyle. Centralized so the door-edit modal doesn't carry
// its own literal list.
var DoorStyleLabels = [DoorStyleCount]string{
	DoorStyleBuilding: "Building",
	DoorStyleCave:     "Cave",
	DoorStyleField:    "Field",
}

// doorStyleNameTable is the canonical DoorStyle ↔ on-disk-name map.
// Indexed by DoorStyle so DoorStyleName is an O(1) lookup; the name
// strings match mapfile.DoorStyleNames row-for-row (init asserts the
// length stays aligned). Both directions go through this table instead
// of the two hand-mirrored switches that used to drift.
var doorStyleNameTable = [DoorStyleCount]string{
	DoorStyleBuilding: mapfile.DoorStyleBuildingName,
	DoorStyleCave:     mapfile.DoorStyleCaveName,
	DoorStyleField:    mapfile.DoorStyleFieldName,
}

// DoorStyleName maps a DoorStyle to its canonical on-disk string. An
// out-of-range style (shouldn't happen) falls back to building so a
// corrupt value still round-trips to a valid row.
func DoorStyleName(s DoorStyle) string {
	if s < 0 || int(s) >= len(doorStyleNameTable) {
		return mapfile.DoorStyleBuildingName
	}
	return doorStyleNameTable[s]
}

// doorStyleFromName maps an on-disk style string to a DoorStyle. Empty or
// unrecognized resolves to building (the parser already validates names,
// so this is the load-time default for a missing column).
func doorStyleFromName(s string) DoorStyle {
	want := strings.ToLower(s)
	for i, name := range doorStyleNameTable {
		if name == want {
			return DoorStyle(i)
		}
	}
	return DoorStyleBuilding
}

func init() {
	if len(doorStyleNameTable) != len(mapfile.DoorStyleNames) {
		panic("core: doorStyleNameTable length must match mapfile.DoorStyleNames — add a row when extending DoorStyle")
	}
	for i, name := range doorStyleNameTable {
		if name != mapfile.DoorStyleNames[i] {
			panic("core: doorStyleNameTable[" + name + "] disagrees with mapfile.DoorStyleNames — keep them in sync")
		}
	}
}

// packAINameTable is the canonical PackAI ↔ on-disk-name map. Indexed
// by PackAI so PackAIName is an O(1) lookup; the strings match
// mapfile.PackAINames row-for-row (init below asserts the length stays
// aligned). Mirrors doorStyleNameTable's shape.
var packAINameTable = [PackAICount]string{
	PackAINone:        mapfile.PackAINoneName,
	PackAIJunkyardDog: mapfile.PackAIJunkyardDogName,
	PackAIPatrol:      mapfile.PackAIPatrolName,
	PackAISkittish:    mapfile.PackAISkittishName,
}

// packAILabels is the editor-facing display name per mode — what the
// pack-edit modal cycles through. Kept next to packAINameTable so the
// on-disk slug and the player-facing label share one row position.
var packAILabels = [PackAICount]string{
	PackAINone:        "None (stationary)",
	PackAIJunkyardDog: "Junkyard Dog",
	PackAIPatrol:      "Patrol (paces)",
	PackAISkittish:    "Skittish (flees)",
}

// PackAIName returns the canonical on-disk string for a PackAI. Empty
// or out-of-range falls back to the no-op mode so a corrupt value still
// round-trips to a valid row.
func PackAIName(ai PackAI) string {
	if ai < 0 || int(ai) >= len(packAINameTable) {
		return mapfile.PackAINoneName
	}
	return packAINameTable[ai]
}

// PackAIFromName maps an on-disk name (case-insensitive) to a PackAI.
// Empty or unrecognized resolves to PackAINone (the editor's default
// for a freshly-placed pack), so the loader doesn't need a separate
// "missing column" branch.
func PackAIFromName(s string) PackAI {
	want := strings.ToLower(s)
	for i, name := range packAINameTable {
		if name == want {
			return PackAI(i)
		}
	}
	return PackAINone
}

// PackAILabel returns the editor-facing display name for a PackAI.
func PackAILabel(ai PackAI) string {
	if ai < 0 || int(ai) >= len(packAILabels) {
		return packAILabels[PackAINone]
	}
	return packAILabels[ai]
}

func init() {
	if len(packAINameTable) != len(mapfile.PackAINames) {
		panic("core: packAINameTable length must match mapfile.PackAINames — add a row when extending PackAI")
	}
	for i, name := range packAINameTable {
		if name != mapfile.PackAINames[i] {
			panic("core: packAINameTable[" + name + "] disagrees with mapfile.PackAINames — keep them in sync")
		}
	}
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
	// Roster expansion. Canonical names use the snake_case convention
	// the parser already enforces; aliases cover the no-underscore form
	// for hand-edited maps.
	{EnemyCaveSpider, "cave_spider", []string{"spider", "cavespider"}},
	{EnemyVampireBat, "vampire_bat", []string{"vampirebat"}},
	{EnemyWisp, "wisp", []string{"will_o_wisp", "willowisp"}},
	{EnemyStoneGolem, "stone_golem", []string{"golem", "stonegolem"}},
	{EnemyNecromancer, "necromancer", []string{"necro"}},
	{EnemySkeleton, "skeleton", nil},
}

// enemyKindByName flattens enemyKindNameTable into a single primary+alias
// lookup so EnemyKindFromName doesn't double-loop. Keys are pre-lowercased
// since callers always normalize.
var enemyKindByName = buildEnemyKindByName()

func buildEnemyKindByName() map[string]EnemyKind {
	m := make(map[string]EnemyKind, len(enemyKindNameTable))
	add := func(key string, v EnemyKind) {
		// Collision assert: two DIFFERENT kinds claiming the same name/alias
		// would silently last-write-wins and mis-route EnemyKindFromName. The
		// missing-row assert in init() can't catch this; guard it here at
		// build time. (A kind repeating its own alias is harmless — same value.)
		if existing, dup := m[key]; dup && existing != v {
			panic("core: enemyKindNameTable name/alias collision on " + key)
		}
		m[key] = v
	}
	for _, e := range enemyKindNameTable {
		add(e.name, e.value)
		for _, alias := range e.aliases {
			add(alias, e.value)
		}
	}
	return m
}

// Coverage assert: every registered EnemyKind must have a name row in
// enemyKindNameTable. Without this a new kind silently fails EnemyKindName
// (returning ok=false), which only surfaces when a map placing it is saved —
// not at startup. Mirrors the materialDefs / packAINameTable / doorStyleNameTable
// asserts above so the enemy table can't drift out of sync with the enum.
func init() {
	for _, def := range EnemyKinds() {
		if _, ok := EnemyKindName(def.Kind); !ok {
			panic("core: enemyKindNameTable is missing a name row for enemy kind " + def.Name)
		}
	}
}

// EnemyKindName returns the canonical on-disk name for the enemy kind,
// plus ok=false on unknown values. MapFileFromArea propagates the failure
// to caller — better to refuse a save than silently rewrite enemy types.
// Keeps its own scan (rather than the shared lookupName) because the enemy
// table's rows carry an extra `aliases` field, so it isn't a namedEnum.
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
// Strips a trailing extension (case-insensitive) from the id before
// reappending mapfile.Ext — so a user typing "test.map" in the Save
// As field writes to maps/test.map, not maps/test.map.map.
func MapPath(id string) string {
	if n := len(mapfile.Ext); len(id) >= n && strings.EqualFold(id[len(id)-n:], mapfile.Ext) {
		id = id[:len(id)-n]
	}
	return filepath.Join(MapsDir(), id+mapfile.Ext)
}

// MapIDFromPath strips the directory and .map extension off a map path.
func MapIDFromPath(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

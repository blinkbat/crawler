package core

import (
	"crawler/internal/app/core/mapfile"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	MaterialDungeon MaterialSet = iota
	MaterialField
	// MaterialCount is the number of material sets; init asserts materialDefs covers each.
	MaterialCount
)

// mapsDirName is the on-disk folder name where .map files live.
const mapsDirName = "maps"

// AssetDirMode / AssetFileMode are the os mode bits for auto-created asset dirs
// and file writes. Alias mapfile's defs so the I/O layer needn't import core.
const (
	AssetDirMode  = mapfile.AssetDirMode
	AssetFileMode = mapfile.AssetFileMode
)

// SelfMapToken is the door TargetMap placeholder for "this same map".
const SelfMapToken = mapfile.SelfMapToken

// MapsDir returns the directory where .map files live.
func MapsDir() string {
	return ResolveAssetDir(mapsDirName)
}

// ResolveAssetDir resolves a relative asset folder name to a usable path:
// cwd-relative (repo-root `go run`) → next to the executable (portable binary)
// → cwd-relative again (first-run case, where the caller's first write creates it).
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

// DirExists reports whether `path` is an existing directory.
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

// inBoundsWH reports whether tile (x,z) lies inside a w×h grid (used before the
// AreaDefinition — and its InBounds — exists). Delegates to the one definition.
func inBoundsWH(x, z, w, h int) bool { return mapfile.InBoundsWH(x, z, w, h) }

// oobErr is the shared "<what> is out of bounds" spawn-validation error.
func oobErr(what string, x, z, w, h int) error {
	return fmt.Errorf("%s at (%d,%d) is out of bounds for %dx%d", what, x, z, w, h)
}

// validateLayerDims checks a layer's row count and per-row width against w×h,
// returning a descriptive error on the first mismatch.
func validateLayerDims(name string, rows []string, w, h int) error {
	if len(rows) != h {
		return fmt.Errorf("%s layer has %d rows, declared height %d", name, len(rows), h)
	}
	for i, row := range rows {
		if len(row) != w {
			return fmt.Errorf("%s layer row %d has width %d, want %d", name, i, len(row), w)
		}
	}
	return nil
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
	// Dimensions must be positive before the runtime trusts them, else a corrupt
	// .map (zero width, OOB start) panics on first index in renderer/movement.
	if mf.Width <= 0 || mf.Height <= 0 {
		return AreaDefinition{}, fmt.Errorf("map dimensions must be positive (got %dx%d)", mf.Width, mf.Height)
	}
	for _, layer := range mf.RequiredLayers() {
		if err := validateLayerDims(layer.Name, layer.Rows, mf.Width, mf.Height); err != nil {
			return AreaDefinition{}, err
		}
	}
	// Ceiling/elevation are OPTIONAL (absent → blank-filled below), but a PRESENT
	// one must match dimensions: OptionalLayerOrBlank only substitutes for an
	// entirely-empty section, so a truncated layer would else default its missing
	// cells (a short ceiling reads as "no roof", flipping AreaIsOutdoor).
	optional := []struct {
		name string
		rows []string
	}{
		{mapfile.SectionCeiling, mf.Ceiling},
		{mapfile.SectionElevation, mf.Elevation},
		// PropLevels/DecorLevels: optional per-tile level grids. Guarded here for
		// parity with the editor's direct-AreaFromMapFile path, which skips validate().
		{mapfile.SectionPropLevels, mf.PropLevels},
		{mapfile.SectionDecorLevels, mf.DecorLevels},
	}
	for _, layer := range optional {
		if len(layer.rows) == 0 {
			continue // absent — defaulted by its reader
		}
		if err := validateLayerDims(layer.name, layer.rows, mf.Width, mf.Height); err != nil {
			return AreaDefinition{}, err
		}
	}
	// Solids: optional voxel stack. Same editor-path parity guard so a ragged
	// plane can't reach SolidAt / the renderer.
	for L, plane := range mf.Solids {
		if err := validateLayerDims(fmt.Sprintf("%s plane %d", mapfile.SectionSolids, L), plane, mf.Width, mf.Height); err != nil {
			return AreaDefinition{}, err
		}
	}
	if !inBoundsWH(mf.StartX, mf.StartZ, mf.Width, mf.Height) {
		return AreaDefinition{}, oobErr("start position", mf.StartX, mf.StartZ, mf.Width, mf.Height)
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
		if !inBoundsWH(p.X, p.Z, mf.Width, mf.Height) {
			return AreaDefinition{}, oobErr("pack", p.X, p.Z, mf.Width, mf.Height)
		}
		if len(p.Members) == 0 {
			return AreaDefinition{}, fmt.Errorf("pack at (%d,%d) has no members", p.X, p.Z)
		}
		members := make([]PackMemberRef, 0, len(p.Members))
		for _, name := range p.Members {
			// Custom enemies win a name collision with a built-in kind, else the
			// author's overrides (stats/skills/rewards) would be silently dropped.
			if def, ok := CustomEnemyByName(customs, name); ok {
				members = append(members, CustomPackMember(def))
				continue
			}
			kind, ok := EnemyKindFromName(name)
			if !ok {
				return AreaDefinition{}, fmt.Errorf("unknown enemy kind or custom enemy %q", name)
			}
			members = append(members, BuiltinPackMember(kind))
		}
		// Members decode front-first; stamp rows from trailing back count.
		ApplyMemberRows(members, p.BackCount)
		spawns = append(spawns, PackSpawn{TileX: p.X, TileZ: p.Z, Members: members, AI: PackAIFromName(p.AI)})
	}
	chests := make([]ChestSpawn, 0, len(mf.Chests))
	for _, c := range mf.Chests {
		if !inBoundsWH(c.X, c.Z, mf.Width, mf.Height) {
			return AreaDefinition{}, oobErr("chest", c.X, c.Z, mf.Width, mf.Height)
		}
		// A chest on the player start would soft-lock the spawn (blocks the tile).
		// Reject loudly here; placeChests silently drops it at runtime.
		if c.X == mf.StartX && c.Z == mf.StartZ {
			return AreaDefinition{}, fmt.Errorf("chest at (%d,%d) sits on the player start", c.X, c.Z)
		}
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
	// A same-map portal keeps SelfMapToken (not the concrete map id) so the
	// self-link survives a rename; expanding it would re-serialize a stale,
	// now-wrong cross-map target. Display sites resolve "self" to the area name.
	doors := make([]DoorSpawn, 0, len(mf.Doors))
	for _, d := range mf.Doors {
		if !inBoundsWH(d.X, d.Z, mf.Width, mf.Height) {
			return AreaDefinition{}, oobErr(fmt.Sprintf("door %q", d.Name), d.X, d.Z, mf.Width, mf.Height)
		}
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
	crystals := make([]CrystalSpawn, 0, len(mf.Crystals))
	for _, c := range mf.Crystals {
		crystals = append(crystals, CrystalSpawn{TileX: c.X, TileZ: c.Z})
	}
	dialogs, err := DialogsFromLines(mf.Dialogs)
	if err != nil {
		return AreaDefinition{}, err
	}
	triggers, err := TriggersFromLines(mf.Triggers)
	if err != nil {
		return AreaDefinition{}, err
	}
	locations, err := LocationsFromLines(mf.Locations)
	if err != nil {
		return AreaDefinition{}, err
	}
	ceiling := mapfile.OptionalLayerOrBlank(mf.Ceiling, mf.Width, mf.Height, TileCeilingOpen)
	elevation := mapfile.OptionalLayerOrBlank(mf.Elevation, mf.Width, mf.Height, ElevationGround)
	area := AreaDefinition{
		Path:             path,
		Name:             mf.Name,
		Width:            mf.Width,
		Height:           mf.Height,
		Walls:            cloneRows(mf.Walls),
		Floor:            cloneRows(mf.Floor),
		Decor:            cloneRows(mf.Decor),
		Props:            cloneRows(mf.Props),
		Ceiling:          cloneRows(ceiling),
		Elevation:        cloneRows(elevation),
		Solids:           CloneSolids(mf.Solids),
		PropLevels:       cloneRows(mf.PropLevels),
		DecorLevels:      cloneRows(mf.DecorLevels),
		FaceOverrides:    faceOverridesFromMap(mf.Faces),
		Materials:        mat,
		StartTileX:       mf.StartX,
		StartTileZ:       mf.StartZ,
		StartFacing:      face,
		PackSpawns:       spawns,
		ChestSpawns:      chests,
		DoorSpawns:       doors,
		CrystalSpawns:    crystals,
		CrystalsAuthored: mf.CrystalsDefined,
		CustomEnemies:    customs,
		QuietMessage:     mf.Quiet,
		Dialogs:          dialogs,
		Triggers:         triggers,
		Locations:        locations,
	}
	// Validate crystals now the area's BlockedAt geometry is built: reject a
	// hand-edited crystal on a blocked tile (renders embedded) or a duplicate
	// (double-counts). Only fires on hand-edited maps.
	seenCrystal := make(map[[2]int]bool, len(area.CrystalSpawns))
	for _, c := range area.CrystalSpawns {
		if area.BlockedAt(c.TileX, c.TileZ) {
			return AreaDefinition{}, fmt.Errorf("crystal at (%d,%d) sits on a blocked tile (wall/prop/deep water)", c.TileX, c.TileZ)
		}
		key := [2]int{c.TileX, c.TileZ}
		if seenCrystal[key] {
			return AreaDefinition{}, fmt.Errorf("duplicate crystal at (%d,%d)", c.TileX, c.TileZ)
		}
		seenCrystal[key] = true
	}
	return area, nil
}

// MapFileFromArea is the reverse converter (editor save-to-disk). Returns an
// error rather than silently substituting a default when an enum value is out
// of registry — a refused save beats a silent rewrite to "dungeon"/"east"/"rat".
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
		// Reorder front-first so the on-disk list reflects each member's row;
		// the trailing BackCount are back.
		ordered, backCount := PartitionMembersByRow(s.Members)
		names := make([]string, 0, len(ordered))
		for _, member := range ordered {
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
			kindName, ok := EnemyKindName(member.Kind)
			if !ok {
				return mapfile.MapFile{}, fmt.Errorf("unknown enemy kind %d in pack at (%d,%d)", int(member.Kind), s.TileX, s.TileZ)
			}
			names = append(names, kindName)
		}
		packs = append(packs, mapfile.MapPack{
			Members:   names,
			BackCount: backCount,
			X:         s.TileX,
			Z:         s.TileZ,
			AI:        PackAIName(s.AI),
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
	doors := make([]mapfile.MapDoor, 0, len(a.DoorSpawns))
	for _, d := range a.DoorSpawns {
		faceName, ok := FacingName(d.Facing)
		if !ok {
			return mapfile.MapFile{}, fmt.Errorf("door %q has bad facing %d", d.Name, d.Facing)
		}
		// Encode same-map portals back to SelfMapToken so a renamed map keeps its
		// internal links. Cross-map targets stay as their explicit map id.
		target := d.TargetMap
		if IsSelfPortal(a, target) {
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
	crystals := make([]mapfile.MapCrystal, 0, len(a.CrystalSpawns))
	for _, c := range a.CrystalSpawns {
		crystals = append(crystals, mapfile.MapCrystal{X: c.TileX, Z: c.TileZ})
	}
	ceiling := mapfile.OptionalLayerOrBlank(a.Ceiling, a.Width, a.Height, TileCeilingOpen)
	// Elevation/solids encoding: a gapless area writes ONLY elevation: (byte-
	// identical to legacy). A gapped area writes solids: AND projects column tops
	// into elevation: as a graceful downgrade for readers that ignore solids:.
	var elevation, solids = a.Elevation, [][]string(nil)
	if len(a.Solids) > 0 {
		// A materialized stack is authoritative: project tops into elevation:,
		// emit solids: only when a gap makes it inexpressible as a heightfield.
		elevation = ElevationRowsFromSolids(&a)
		if !a.AllColumnsGapless() {
			solids = CloneSolids(a.Solids)
		}
	}
	elevation = mapfile.OptionalLayerOrBlank(elevation, a.Width, a.Height, ElevationGround)
	customs := make([]mapfile.MapCustomEnemy, 0, len(a.CustomEnemies))
	for _, ce := range a.CustomEnemies {
		mapCE, err := MapCustomEnemyFromDef(ce)
		if err != nil {
			return mapfile.MapFile{}, err
		}
		customs = append(customs, mapCE)
	}
	dialogLines, err := DialogsToLines(a.Dialogs)
	if err != nil {
		return mapfile.MapFile{}, err
	}
	triggerLines, err := TriggersToLines(a.Triggers)
	if err != nil {
		return mapfile.MapFile{}, err
	}
	locationLines, err := LocationsToLines(a.Locations)
	if err != nil {
		return mapfile.MapFile{}, err
	}
	return mapfile.MapFile{
		Name:            a.Name,
		Materials:       matName,
		Quiet:           a.QuietMessage,
		Width:           a.Width,
		Height:          a.Height,
		StartX:          a.StartTileX,
		StartZ:          a.StartTileZ,
		StartFace:       faceName,
		Walls:           cloneRows(a.Walls),
		Floor:           cloneRows(a.Floor),
		Decor:           cloneRows(a.Decor),
		Props:           cloneRows(a.Props),
		Ceiling:         cloneRows(ceiling),
		Elevation:       cloneRows(elevation),
		Solids:          solids,
		PropLevels:      levelsForEncode(a.PropLevels, a.Width, a.Height),
		DecorLevels:     levelsForEncode(a.DecorLevels, a.Width, a.Height),
		Faces:           mapFacesFromArea(a),
		Packs:           packs,
		Chests:          chests,
		Doors:           doors,
		Crystals:        crystals,
		CrystalsDefined: a.CrystalsAuthored,
		CustomEnemies:   customs,
		Dialogs:         dialogLines,
		Triggers:        triggerLines,
		Locations:       locationLines,
	}, nil
}

// levelsForEncode returns a per-tile level grid to write, or nil when every cell
// is auto (so the map omits the section and stays byte-identical pre-feature).
func levelsForEncode(layer []string, w, h int) []string {
	explicit := false
	for z := 0; z < h && z < len(layer) && !explicit; z++ {
		row := layer[z]
		for x := 0; x < w && x < len(row); x++ {
			if row[x] != PropLevelAuto {
				explicit = true
				break
			}
		}
	}
	if !explicit {
		return nil
	}
	return mapfile.OptionalLayerOrBlank(layer, w, h, PropLevelAuto)
}

// faceOverridesFromMap converts on-disk per-tile face skins into FaceOverrides
// (nil when none).
func faceOverridesFromMap(faces []mapfile.MapFace) []FaceOverride {
	if len(faces) == 0 {
		return nil
	}
	out := make([]FaceOverride, len(faces))
	for i, f := range faces {
		out[i] = FaceOverride{X: f.X, Z: f.Z, Skins: f.Skins}
	}
	return out
}

// mapFacesFromArea projects FaceOverrides to on-disk form: unset faces → auto
// sentinel, sorted by (Z,X) so a re-save is deterministic/byte-identical.
func mapFacesFromArea(a AreaDefinition) []mapfile.MapFace {
	if len(a.FaceOverrides) == 0 {
		return nil
	}
	out := make([]mapfile.MapFace, 0, len(a.FaceOverrides))
	for _, o := range a.FaceOverrides {
		sk := o.Skins
		for d := range sk {
			if sk[d] == 0 {
				sk[d] = PropLevelAuto
			}
		}
		out = append(out, mapfile.MapFace{X: o.X, Z: o.Z, Skins: sk})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Z != out[j].Z {
			return out[i].Z < out[j].Z
		}
		return out[i].X < out[j].X
	})
	return out
}

// --- Table-driven name <-> enum lookups ---
// Each registry is a slice of (enum, primary-name, aliases...) tuples; Name
// looks up by enum, FromName by case-folded name. Unknown → ok=false, never a
// silent coerce to the first option (which used to rewrite save data).

// indexByName returns the index of the first row whose name case-insensitively
// equals s (ok=false on no match). Shared by the material/facing/style/AI decoders.
func indexByName[T any](table []T, s string, name func(T) string) (int, bool) {
	want := strings.ToLower(s)
	for i, row := range table {
		if name(row) == want {
			return i, true
		}
	}
	return 0, false
}

// decodeByName resolves a case-folded name to an enum value: indexByName, then
// convert the matched index via toEnum; (zero, false) on no match. The shared body
// behind the material/facing/style/AI name decoders (single-return callers drop ok).
func decodeByName[R any, T any](table []R, s string, name func(R) string, toEnum func(int) T) (T, bool) {
	if i, ok := indexByName(table, s, name); ok {
		return toEnum(i), true
	}
	var zero T
	return zero, false
}

// findByValue is the forward (value→row) mirror of indexByName: the first row
// whose value(row) equals want, ok=false on no match. Shared by the enum→row
// decoders whose registry carries an explicit value field (material/facing).
func findByValue[T any, V comparable](table []T, want V, value func(T) V) (T, bool) {
	for _, row := range table {
		if value(row) == want {
			return row, true
		}
	}
	var zero T
	return zero, false
}

// materialDef is one material registry row: on-disk name + traits. `indoor`
// (enclosed interior vs. outdoor biome) drives minimap tone, lighting, and
// resource fallbacks — a new material is one row, not a `== MaterialDungeon` grep.
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
	// Keep gridLayers() (drives Clone/Equal/region) in lockstep with the
	// converters, which hand-list the same layer fields; a layer added on one
	// side but not the other silently fails to round-trip.
	if got := len((&AreaDefinition{}).gridLayers()); got != mapfile.GridLayerCount {
		panic(fmt.Sprintf("core: gridLayers() has %d layers but mapfile has %d grid layers — update the Area↔MapFile converters (AreaFromMapFile / MapFileFromArea) too", got, mapfile.GridLayerCount))
	}
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

// findMaterialDef is the forward (enum→row) scan, shared by MaterialName /
// MaterialIsIndoor. ok=false when out of range.
func findMaterialDef(m MaterialSet) (materialDef, bool) {
	return findByValue(materialDefs, m, func(d materialDef) MaterialSet { return d.value })
}

// MaterialName returns the on-disk name for the material set; ok=false when out
// of range (save-path callers must propagate the failure).
func MaterialName(m MaterialSet) (string, bool) {
	d, ok := findMaterialDef(m)
	return d.name, ok
}

func materialFromName(s string) (MaterialSet, bool) {
	return decodeByName(materialDefs, s, func(d materialDef) string { return d.name }, func(i int) MaterialSet { return materialDefs[i].value })
}

// MaterialOptions is the editor's dropdown order, derived from materialDefs (so
// a new material shows up automatically, in a stable order for palette colors).
var MaterialOptions = buildMaterialOptions()

func buildMaterialOptions() []MaterialSet {
	opts := make([]MaterialSet, len(materialDefs))
	for i, d := range materialDefs {
		opts[i] = d.value
	}
	return opts
}

// MaterialIsIndoor reports whether the material set is an enclosed interior.
func MaterialIsIndoor(m MaterialSet) bool {
	d, _ := findMaterialDef(m)
	return d.indoor
}

// facingDef is one facing registry row: enum value, on-disk name, and the
// single-letter UI label (FacingShortLabels derives from it).
type facingDef struct {
	value int
	name  string // canonical on-disk name (mapfile.Facing*Name)
	short string // single-letter UI label
}

var facingDefs = []facingDef{
	{North, mapfile.FacingNorthName, "N"},
	{East, mapfile.FacingEastName, "E"},
	{South, mapfile.FacingSouthName, "S"},
	{West, mapfile.FacingWestName, "W"},
}

// FacingShortLabels are the single-letter UI labels indexed by North/East/South/
// West, derived from facingDefs.
var FacingShortLabels = func() [FacingCount]string {
	var out [FacingCount]string
	for _, d := range facingDefs {
		out[d.value] = d.short
	}
	return out
}()

// FacingName returns the on-disk name for a facing (ok=false only on an
// out-of-range value, impossible for the four legit enum values).
func FacingName(f int) (string, bool) {
	d, ok := findByValue(facingDefs, NormalizeFacing(f), func(d facingDef) int { return d.value })
	return d.name, ok
}

func facingFromName(s string) (int, bool) {
	return decodeByName(facingDefs, s, func(d facingDef) string { return d.name }, func(i int) int { return facingDefs[i].value })
}

// init asserts facingDefs covers every facing once, in mapfile.FacingNames order,
// with a non-empty short label — so a missing/blank row panics at startup.
func init() {
	if len(facingDefs) != FacingCount || len(mapfile.FacingNames) != FacingCount {
		panic("core: facingDefs length must match FacingCount and mapfile.FacingNames — add a row when extending the facing enum")
	}
	for i, d := range facingDefs {
		if d.value != i {
			panic("core: facingDefs row order must match the North/East/South/West enum")
		}
		if d.name != mapfile.FacingNames[i] {
			panic("core: facingDefs[" + d.name + "] disagrees with mapfile.FacingNames — keep them in sync")
		}
		if d.short == "" {
			panic("core: facingDefs[" + d.name + "] has an empty short label")
		}
	}
}

// FacingAwayFromAdjacentWall returns the facing AWAY from the first wall found
// scanning (x,z)'s cardinal neighbours N→E→S→W — the way a wall-mounted thing
// (torch, door) faces into the room. found=false when no adjacent wall.
// Takes *AreaDefinition (read-only): the struct is large (many slice/map headers)
// and this is called per wall-torch per frame in the draw loop — a value copy there
// is pure waste.
func FacingAwayFromAdjacentWall(m *AreaDefinition, x, z int) (facing int, found bool) {
	// A neighbour to back against is a solid obstruction OR a higher tile (walls
	// are elevation now). Off-map counts via WallAt.
	here := m.ElevationLevelAt(x, z)
	wall := func(nx, nz int) bool {
		return m.WallAt(nx, nz) || m.ElevationLevelAt(nx, nz) > here
	}
	switch {
	case wall(x, z-1):
		return South, true // wall north → face south
	case wall(x+1, z):
		return West, true
	case wall(x, z+1):
		return North, true
	case wall(x-1, z):
		return East, true
	}
	return 0, false
}

// doorStyleDef bundles a DoorStyle's on-disk slug and editor label in one row,
// indexed by DoorStyle; name matches mapfile.DoorStyleNames row-for-row (init asserts).
type doorStyleDef struct {
	name  string // canonical on-disk slug
	label string // editor button label
}

var doorStyleDefs = [DoorStyleCount]doorStyleDef{
	DoorStyleBuilding: {mapfile.DoorStyleBuildingName, "Building"},
	DoorStyleCave:     {mapfile.DoorStyleCaveName, "Cave"},
	DoorStyleField:    {mapfile.DoorStyleFieldName, "Field"},
}

// DoorStyleName maps a DoorStyle to its on-disk string; out-of-range falls back to building.
func DoorStyleName(s DoorStyle) string {
	if s < 0 || int(s) >= len(doorStyleDefs) {
		return mapfile.DoorStyleBuildingName
	}
	return doorStyleDefs[s].name
}

// DoorStyleLabel returns the editor button label; out-of-range falls back to building.
func DoorStyleLabel(s DoorStyle) string {
	if s < 0 || int(s) >= len(doorStyleDefs) {
		return doorStyleDefs[DoorStyleBuilding].label
	}
	return doorStyleDefs[s].label
}

// doorStyleFromName maps an on-disk style string to a DoorStyle; empty/unknown → building.
func doorStyleFromName(s string) DoorStyle {
	if style, ok := decodeByName(doorStyleDefs[:], s, func(d doorStyleDef) string { return d.name }, func(i int) DoorStyle { return DoorStyle(i) }); ok {
		return style
	}
	return DoorStyleBuilding
}

func init() {
	if len(doorStyleDefs) != len(mapfile.DoorStyleNames) {
		panic("core: doorStyleDefs length must match mapfile.DoorStyleNames — add a row when extending DoorStyle")
	}
	for i, d := range doorStyleDefs {
		if d.name != mapfile.DoorStyleNames[i] {
			panic("core: doorStyleDefs[" + d.name + "] disagrees with mapfile.DoorStyleNames — keep them in sync")
		}
	}
}

// packAIDef bundles a PackAI's on-disk slug and editor label in one row, indexed
// by PackAI; name matches mapfile.PackAINames row-for-row (init asserts).
type packAIDef struct {
	name  string // canonical on-disk slug
	label string // editor pack-edit modal label
}

var packAIDefs = [PackAICount]packAIDef{
	PackAINone:        {mapfile.PackAINoneName, "None (stationary)"},
	PackAIJunkyardDog: {mapfile.PackAIJunkyardDogName, "Junkyard Dog"},
	PackAIPatrol:      {mapfile.PackAIPatrolName, "Patrol (paces)"},
	PackAISkittish:    {mapfile.PackAISkittishName, "Skittish (flees)"},
}

// validPackAI reports whether ai indexes packAIDefs.
func validPackAI(ai PackAI) bool {
	return ai >= 0 && int(ai) < len(packAIDefs)
}

// PackAIName returns the on-disk string for a PackAI; out-of-range falls back to none.
func PackAIName(ai PackAI) string {
	if !validPackAI(ai) {
		return mapfile.PackAINoneName
	}
	return packAIDefs[ai].name
}

// PackAIFromName maps an on-disk name to a PackAI; empty/unknown → PackAINone.
func PackAIFromName(s string) PackAI {
	if ai, ok := decodeByName(packAIDefs[:], s, func(d packAIDef) string { return d.name }, func(i int) PackAI { return PackAI(i) }); ok {
		return ai
	}
	return PackAINone
}

// PackAILabel returns the editor-facing display name for a PackAI.
func PackAILabel(ai PackAI) string {
	if !validPackAI(ai) {
		return packAIDefs[PackAINone].label
	}
	return packAIDefs[ai].label
}

func init() {
	if len(packAIDefs) != len(mapfile.PackAINames) {
		panic("core: packAIDefs length must match mapfile.PackAINames — add a row when extending PackAI")
	}
	for i, d := range packAIDefs {
		if d.name != mapfile.PackAINames[i] {
			panic("core: packAIDefs[" + d.name + "] disagrees with mapfile.PackAINames — keep them in sync")
		}
	}
}

// enemyKindByName flattens the registry's MapToken+MapAliases into one
// primary+alias lookup. Keys are pre-lowercased.
var enemyKindByName = buildEnemyKindByName()

func buildEnemyKindByName() map[string]EnemyKind {
	m := make(map[string]EnemyKind, len(enemyDefinitions))
	add := func(key string, v EnemyKind) {
		// Collision assert: two DIFFERENT kinds claiming the same name/alias
		// would last-write-wins and mis-route EnemyKindFromName.
		if existing, dup := m[key]; dup && existing != v {
			panic("core: enemy MapToken/MapAliases collision on " + key)
		}
		m[key] = v
	}
	for _, def := range enemyDefinitions {
		add(def.MapToken, def.Kind)
		for _, alias := range def.MapAliases {
			add(alias, def.Kind)
		}
	}
	return m
}

// Coverage assert: every registered EnemyKind must declare a MapToken, else a new
// kind silently fails EnemyKindName until a map placing it is saved.
func init() {
	for _, def := range enemyDefinitions {
		if def.MapToken == "" {
			panic("core: enemy kind " + def.Name + " has no MapToken — set it in enemies.go")
		}
	}
}

// EnemyKindName returns the on-disk name for an enemy kind; ok=false on unknown
// (MapFileFromArea refuses the save rather than rewriting enemy types).
func EnemyKindName(k EnemyKind) (string, bool) {
	if def, ok := EnemyInfoOk(k); ok {
		return def.MapToken, true
	}
	return "", false
}

func EnemyKindFromName(s string) (EnemyKind, bool) {
	kind, ok := enemyKindByName[strings.ToLower(s)]
	return kind, ok
}

// SanitizeFilename normalizes a string into a safe filename stem: lowercase
// ASCII letters/digits/underscore/hyphen only, spaces → underscore, rest stripped.
// Returns `fallback` (or "" so the caller can refuse) when nothing usable remains.
// KEEPS hyphens — distinct from slugify (folds hyphens, no fallback) and
// SanitizeCustomEnemyName (preserves case+punctuation); don't substitute.
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

// MapPath returns the save path for a map ID under MapsDir, stripping a trailing
// .map first so "test.map" writes to maps/test.map, not maps/test.map.map.
func MapPath(id string) string {
	// Case-insensitive ext strip via TrimSuffix on the actual-case suffix, so
	// "test.MAP" trims too (matches the old EqualFold compare).
	if n := len(mapfile.Ext); len(id) >= n && strings.EqualFold(id[len(id)-n:], mapfile.Ext) {
		id = strings.TrimSuffix(id, id[len(id)-n:])
	}
	return filepath.Join(MapsDir(), id+mapfile.Ext)
}

// MapIDFromPath strips the directory and .map extension off a map path.
func MapIDFromPath(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// IsSelfPortal reports whether a door's TargetMap refers to area `a` itself
// (SelfMapToken or a's own map id from Path). An unsaved area (empty Path) has
// no id, so only the token counts.
func IsSelfPortal(a AreaDefinition, target string) bool {
	if target == SelfMapToken {
		return true
	}
	return a.Path != "" && target == MapIDFromPath(a.Path)
}

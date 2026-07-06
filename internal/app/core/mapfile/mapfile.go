// Package mapfile is the on-disk representation of an explorable area: plain
// text, multi-section — header lines, one ASCII grid per layer (walls / floor /
// decor / props / ceiling / elevation), then enemy / chest / door spawn
// sections. Diffs cleanly in git; editor layers map 1:1 to sections.
//
// Layer character conventions:
//
//	walls  : per-tile CLIFF-FACE SKIN (legacy name; no longer blocks — the
//	         rendered vertical face of an elevation step, shown only where
//	         elevation exposes a face). '.' plain rock (default), '#' rock,
//	         '+' light ivy, '=' heavy ivy, '&' cracked, '$' crumbling.
//	floor  : '.' auto-variant (per-tile hash), 'g' grass, 'd' dirt,
//	         'k' dark grass, 's' stone, 'c' cobblestone path, 'w' planks,
//	         '~' shallow water, 'W' deep water (blocks), 'n' sand, 'i' snow,
//	         '^'/'>'/'v'/'<' ramp ascending N/E/S/W (walkable; bridges the tile's
//	         elevation to one level higher in the arrow's direction)
//	decor  : '.' auto-scatter, '_' force-empty, 'b' bush, 'm' mushroom,
//	         'p' pebble cluster, ',' tall grass, 'f' wildflowers,
//	         'v' clover, 'r' reeds, 'o' bones, 'x' scorch, '!' blood,
//	         '*' cobweb, 't' stump, 'l' fallen log, 'L' leaf pile,
//	         'A' archway anchor (left), 'a' archway tail (right) — arch spans
//	         2 tiles along +X; 'y' lilypad, 'u' rug, 'c' candle,
//	         'i' bootprints, 'h' ash heap, 'q' puddle, 'k' roots.
//	         All never block.
//	props  : '.' empty, 'T' tree, 'X' tree XL, '|' tall tree,
//	         '@' twin trees, '/' young tree, 'O' boulder,
//	         'B' bush (large), 'C' crate, 'R' barrel, 'U' urn,
//	         'S' stalagmite, 'P' pillar, 'I' broken pillar,
//	         'M' statue, 'Q' obelisk, 'F' fountain,
//	         'K' rock cairn (1 tile), 'J' rock formation anchor (top-left of a
//	         2×2 footprint), 'j' formation tail (other 3 tiles),
//	         'W' well, 'G' gravestone, 'N' sign post, 'H' hay bale,
//	         'Y' scarecrow, 'V' bookshelf, 'E' table, 'D' bed,
//	         'Z' brazier, 'A' sarcophagus, 'z' wall torch,
//	         'e' exotic flower, '(' tall fern, ')' grass tuft.
//	         Blocking except the wall torch ('z') and the decorative
//	         plants ('e','(',')'); the rock-formation anchor's mesh covers
//	         the footprint, tails render nothing.
//	ceiling  : '.' open (sky shows through), '#' solid overhead slab.
//	elevation: per-tile ground LEVEL — '0'..'9' then 'A'..'K' for 10..20
//	         (base-36, one char/cell; blank/absent ⇒ '0', flat). The world is
//	         built entirely from this: walkable baseline at level 10, cliffs
//	         above, pits below. A ramp tile stores its LOW level; any unramped
//	         level change between adjacent tiles is an impassable cliff (renders
//	         a face). Optional.
package mapfile

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// MapFile round-trips losslessly with Encode/Parse.
type MapFile struct {
	Name      string
	Materials string
	Quiet     string
	// Weather is the optional per-area ambient-weather override: "" / "auto" (roof-
	// gated default), "clear" (never rains), "rain" (always storming). Absent on
	// pre-weather maps → auto; written only when non-auto so old maps stay byte-stable.
	Weather   string
	Width     int
	Height    int
	StartX    int
	StartZ    int
	StartFace string
	Walls     []string
	Floor     []string
	Decor     []string
	Props     []string
	// Ceiling is the optional fifth grid; empty on load for pre-ceiling maps,
	// which the loader fills with a blank "no ceiling" layer.
	Ceiling []string
	// Elevation is the optional sixth grid (per-tile ground LEVEL); empty on
	// load for pre-elevation maps, which the loader fills all-'0' (flat).
	Elevation []string
	// Solids is the optional voxel stack: Solids[level] is a Height×Width grid of
	// cube/air chars ('0' = air, else a cube's material char), planes lowest-first.
	// Written ONLY for a gap (floating cube over air) the single-height elevation
	// layer can't express; a heightfield omits it, byte-identical. When present,
	// elevation: is still written (column tops) for readers that ignore solids:.
	Solids [][]string
	// PropLevels is the optional per-tile prop-LEVEL grid: base-36 voxel level, or
	// '.' = auto. Written ONLY when some prop sits above its auto surface.
	PropLevels []string
	// DecorLevels is the decor analogue of PropLevels.
	DecorLevels []string
	// PropYaw is the optional per-tile prop-facing grid: '.' = auto (procedural yaw),
	// else a step digit. Written only when a prop carries an authored facing.
	PropYaw []string
	// PropStack / DecorStack are the optional per-FLOOR scatter stacks (the cube
	// analogue of Solids for placed content): Stack[level] is a Height×Width grid
	// of prop/decor chars, planes lowest-first. Written ONLY when a column carries
	// content on more than one floor (inexpressible as the single grid + level
	// tag); a single-floor map omits them, byte-identical. See core/floors.go.
	PropStack  [][]string
	DecorStack [][]string
	// Faces holds per-tile cliff-face skin overrides (N/E/S/W), one line per
	// overridden tile; empty for maps using only base/whole-tile skins.
	Faces []MapFace
	Packs []MapPack
	// Chests is the authored chest list. Items is a comma-separated list of
	// ItemDefinition.Name strings; empty = empty chest (renders open).
	Chests []MapChest
	// Doors is the authored door list: each names itself, its destination map +
	// matching-door name (or "self"), and the tile + post-transition facing.
	// Resolved at step-on time. Bidirectional pairs are author-authored — the
	// engine doesn't infer pairs.
	Doors []MapDoor
	// Crystals is the authored healing-crystal list (one tile position each);
	// optional.
	Crystals []MapCrystal
	// CrystalsDefined distinguishes "crystals: present but empty" (author wants
	// zero) from "absent" (legacy map; runtime fills a default entrance crystal).
	// Encode writes the section whenever set, so a zero-crystal map stays zero.
	CrystalsDefined bool
	// Dialogs is the authored conversation list — one OPAQUE JSON object per line.
	// This leaf package stays JSON-agnostic (verbatim); core marshals DialogDefinition.
	Dialogs []string
	// Triggers is the authored trigger list — opaque JSON per line, same
	// verbatim handling as Dialogs (core marshals Trigger).
	Triggers []string
	// Locations is the authored named-region list — opaque JSON per line, same
	// verbatim handling as Dialogs (core marshals Location).
	Locations []string
	// WallFeatures is the authored wall-fixture list (switches / bombable / secret
	// walls) — opaque JSON per line, same verbatim handling as Dialogs (core marshals
	// WallFeature).
	WallFeatures []string
}

// MapPack is one authored pack at a tile. Members is a non-empty enemy-kind
// list ordered FRONT row first then BACK; on-disk "kind[,kind...] X Z [ai]"
// (3 fields = legacy, AI defaults "none"). A ';' in the member field splits
// front from back ("f,f;b,b"); no ';' = all front.
type MapPack struct {
	Members []string
	// BackCount is how many of Members (front-first) are BACK row — the last
	// BackCount entries. Zero = all front, so row-less packs round-trip stably.
	BackCount int
	X         int
	Z         int
	// Level is the voxel floor this pack belongs to (the section's @N suffix; 0 =
	// the plain `enemies:` section / a single-floor map). See sectionFor.
	Level int
	// AI is the on-disk movement style (see PackAINames); empty ⇒ PackAINone
	// (stationary).
	AI string
}

// Pack AI names — canonical on-disk strings for each core.PackAI value.
const (
	PackAINoneName        = "none"
	PackAIJunkyardDogName = "junkyard_dog"
	PackAIPatrolName      = "patrol"
	PackAISkittishName    = "skittish"
)

// PackAINames is the canonical on-disk order, matching core.PackAI by index
// (index 0 = none, so an absent AI column resolves to no-op). Order MUST match
// core's PackAI iota (core/areas.go's packAIDefs init asserts alignment).
var PackAINames = [...]string{
	PackAINoneName,
	PackAIJunkyardDogName,
	PackAIPatrolName,
	PackAISkittishName,
}

// InBoundsWH reports whether (x,z) lies inside a w×h map. Exported so core can
// share the one definition (core.AreaDefinition.InBounds and the areas.go load
// validators call it directly).
func InBoundsWH(x, z, w, h int) bool {
	return x >= 0 && x < w && z >= 0 && z < h
}

// nameInList reports whether s case-insensitively matches one of names.
func nameInList(s string, names []string) bool {
	low := strings.ToLower(s)
	for _, name := range names {
		if name == low {
			return true
		}
	}
	return false
}

// IsPackAIName reports whether s names a canonical pack-AI mode (case-insensitive).
func IsPackAIName(s string) bool {
	return nameInList(s, PackAINames[:])
}

// splitPackMembers parses a pack's member field: comma-separated members, an
// optional ';' splitting FRONT (before) from BACK (after). Returns the flat
// front-first list and the back-row count.
func splitPackMembers(field string) (members []string, backCount int, err error) {
	frontStr, backStr := field, ""
	if i := strings.IndexByte(field, ';'); i >= 0 {
		frontStr, backStr = field[:i], field[i+1:]
	}
	front, err := parsePackGroup(frontStr)
	if err != nil {
		return nil, 0, err
	}
	back, err := parsePackGroup(backStr)
	if err != nil {
		return nil, 0, err
	}
	return append(front, back...), len(back), nil
}

// parsePackGroup splits one comma-separated member group, rejecting empty tokens;
// an empty/whitespace group yields no members.
func parsePackGroup(s string) ([]string, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	parts := strings.Split(s, ",")
	for i, m := range parts {
		m = strings.TrimSpace(m)
		if m == "" {
			return nil, fmt.Errorf("empty pack member")
		}
		parts[i] = m
	}
	return parts, nil
}

// encodePackMembers is the inverse of splitPackMembers. backCount<=0 writes the
// plain comma list (legacy shape); otherwise "front;back" (front may be empty).
func encodePackMembers(members []string, backCount int) string {
	if backCount <= 0 {
		return strings.Join(members, ",")
	}
	if backCount > len(members) {
		backCount = len(members)
	}
	split := len(members) - backCount
	return strings.Join(members[:split], ",") + ";" + strings.Join(members[split:], ",")
}

// MapChest is one authored chest at a tile. On-disk "item[,item...] X Z" (mirrors
// packs); an empty chest writes "(empty) X Z" to keep the row 3 fields.
type MapChest struct {
	Items []string
	X     int
	Z     int
	// Level is the voxel floor this chest belongs to (section @N suffix; 0 = plain
	// `chests:`). See sectionFor.
	Level int
}

// EmptyChestToken is the placeholder for a chest with no items (kept out of the
// item registry so it can't shadow a real ItemDefinition.Name).
const EmptyChestToken = "(empty)"

// MapDoor is one authored door at a tile. On-disk format:
//
//	<name> <target_map> <target_door> <X> <Z> <facing> [style]
//
// Name is unique within the map; TargetMap is the destination map id (bare name)
// or "self" for same-map portals; TargetDoor is the matching door's Name there;
// Facing is the post-transition direction (north/east/south/west). Style is the
// optional visual fixture (building/cave/field), defaulting to building so older
// 6-field rows load unchanged.
type MapDoor struct {
	Name       string
	TargetMap  string
	TargetDoor string
	X          int
	Z          int
	// Level is the voxel floor this door belongs to (section @N suffix; 0 = plain
	// `doors:`). See sectionFor.
	Level  int
	Facing string
	Style  string
}

// MapCrystal is one authored healing crystal at a tile. On-disk "X Z" only
// (charge state is runtime-only).
type MapCrystal struct {
	X int
	Z int
	// Level is the voxel floor this crystal belongs to (section @N suffix; 0 =
	// plain `crystals:`). See sectionFor.
	Level int
}

// MapFace is one tile's per-direction cliff-face skin override. Skins is indexed
// 0=N,1=E,2=S,3=W; '.' = use the tile's base skin. On disk "x z NESW", one per
// overridden tile.
type MapFace struct {
	X, Z  int
	Skins [4]byte
}

// DoorTargetComplete reports whether a door names both halves of a destination.
// Core door types route through this so parse-time and runtime checks can't drift.
func DoorTargetComplete(targetMap, targetDoor string) bool {
	return targetMap != "" && targetDoor != ""
}

// HasTarget reports whether this door names a complete destination.
func (d MapDoor) HasTarget() bool {
	return DoorTargetComplete(d.TargetMap, d.TargetDoor)
}

// SelfMapToken is the placeholder TargetMap for same-map portals (keeps the row
// 6 fields). Kept verbatim end-to-end — survives parse and re-encode so a renamed
// map keeps its self-loop (see TestSelfDoorSurvivesRename); resolved to the
// concrete map id only at transition/display time, never at load.
const SelfMapToken = "self"

const (
	FacingNorthName = "north"
	FacingEastName  = "east"
	FacingSouthName = "south"
	FacingWestName  = "west"
)

// FacingNames is the canonical on-disk order for facing strings.
var FacingNames = [...]string{
	FacingNorthName,
	FacingEastName,
	FacingSouthName,
	FacingWestName,
}

const (
	DoorStyleBuildingName = "building"
	DoorStyleCaveName     = "cave"
	DoorStyleFieldName    = "field"
)

// DoorStyleNames is the canonical on-disk order, matching core.DoorStyle* (index
// 0 = building, so an absent style column resolves to it).
var DoorStyleNames = [...]string{
	DoorStyleBuildingName,
	DoorStyleCaveName,
	DoorStyleFieldName,
}

// IsDoorStyleName reports whether s names a canonical door style (case-insensitive).
func IsDoorStyleName(s string) bool {
	return nameInList(s, DoorStyleNames[:])
}

// Ext is the canonical map-file extension.
const Ext = ".map"

// Ceiling-layer sentinels: open ('.') = sky shows through, solid ('#') = slab.
const (
	CeilingOpenChar  = '.'
	CeilingSolidChar = '#'
)

// ElevationGroundChar is the sentinel for the lowest ground level (0); blank/
// absent elevation seeds to it.
const ElevationGroundChar = '0'

// MaxLevelChar is the top of the elevation/level alphabet ('0'..'9' then 'A'..
// MaxLevelChar = 0..20). Single source within this package; core/map.go init
// pins ElevationChar(MaxElevationLevel) == MaxLevelChar so the two can't drift.
const MaxLevelChar = 'K'

// AssetDirMode / AssetFileMode are os mode bits for auto-created asset dirs/files.
const (
	AssetDirMode  = 0o755
	AssetFileMode = 0o644
)

// layerSlot is which grid/section the upcoming rows go into.
type layerSlot int

const (
	slotNone layerSlot = iota
	slotWalls
	slotFloor
	slotDecor
	slotProps
	slotCeiling
	slotElevation
	slotEnemies
	slotChests
	slotDoors
	slotCrystals
	slotDialogs
	slotTriggers
	slotLocations
	slotSolids
	slotPropLevels
	slotDecorLevels
	slotFaces
	slotPropStack
	slotDecorStack
	slotPropYaw
	slotWallFeatures
	// slotCount is the sentinel one past the last real slot, so the init coverage
	// assertion covers every appended slot automatically (a new slot before it is
	// checked without editing the loop bound).
	slotCount
)

// Section header names — on-disk labels (header line is name+colon); sectionFor
// and Encode share these so reader and writer can't drift.
const (
	SectionWalls       = "walls"
	SectionFloor       = "floor"
	SectionDecor       = "decor"
	SectionProps       = "props"
	SectionCeiling     = "ceiling"
	SectionElevation   = "elevation"
	SectionEnemies     = "enemies"
	SectionChests      = "chests"
	SectionDoors       = "doors"
	SectionCrystals    = "crystals"
	SectionDialogs     = "dialogs"
	SectionTriggers    = "triggers"
	SectionLocations   = "locations"
	SectionSolids      = "solids"
	SectionPropLevels  = "prop_levels"
	SectionDecorLevels = "decor_levels"
	SectionFaces       = "faces"
	SectionPropStack    = "propstack"
	SectionDecorStack   = "decorstack"
	SectionPropYaw      = "prop_yaw"
	SectionWallFeatures = "wallfeatures"
)

// Header-line keys — the preamble's "key: value" lines. parseHeaderLine reads,
// Encode writes (one spelling, both sides).
const (
	headerName      = "name"
	headerMaterials = "materials"
	headerQuiet     = "quiet"
	headerWeather   = "weather"
	headerSize      = "size"
	headerStart     = "start"
)

// layerSection describes one section: on-disk name, parse slot, and (for the six
// grid layers) a field accessor into MapFile. Entity sections carry a nil field.
// Single source for sectionFor (name→slot) and layerSlice (slot→*[]string).
type layerSection struct {
	name  string
	slot  layerSlot
	field func(*MapFile) *[]string
}

var layerSections = []layerSection{
	{SectionWalls, slotWalls, func(mf *MapFile) *[]string { return &mf.Walls }},
	{SectionFloor, slotFloor, func(mf *MapFile) *[]string { return &mf.Floor }},
	{SectionDecor, slotDecor, func(mf *MapFile) *[]string { return &mf.Decor }},
	{SectionProps, slotProps, func(mf *MapFile) *[]string { return &mf.Props }},
	{SectionCeiling, slotCeiling, func(mf *MapFile) *[]string { return &mf.Ceiling }},
	{SectionElevation, slotElevation, func(mf *MapFile) *[]string { return &mf.Elevation }},
	{SectionEnemies, slotEnemies, nil},
	{SectionChests, slotChests, nil},
	{SectionDoors, slotDoors, nil},
	{SectionCrystals, slotCrystals, nil},
	{SectionDialogs, slotDialogs, nil},
	{SectionTriggers, slotTriggers, nil},
	{SectionLocations, slotLocations, nil},
	// solids: is a multi-plane voxel stack, not a single grid — nil field +
	// bespoke code, excluded from GridLayerCount.
	{SectionSolids, slotSolids, nil},
	// prop_levels: OPTIONAL single grid written only when a prop sits above its
	// auto surface; nil field + bespoke encode keeps it off byte-stable maps.
	{SectionPropLevels, slotPropLevels, nil},
	// decor_levels: prop_levels for the decor layer.
	{SectionDecorLevels, slotDecorLevels, nil},
	// faces: sparse entity-style section (one line per overridden tile).
	{SectionFaces, slotFaces, nil},
	// propstack: / decorstack: multi-plane scatter stacks (per-floor props/decor),
	// nil field + bespoke code like solids:; excluded from GridLayerCount.
	{SectionPropStack, slotPropStack, nil},
	{SectionDecorStack, slotDecorStack, nil},
	// prop_yaw: OPTIONAL single grid of per-tile prop facings; nil field + bespoke
	// encode keeps it off byte-stable maps (like prop_levels:).
	{SectionPropYaw, slotPropYaw, nil},
	// wallfeatures: sparse opaque-JSON section (one line per wall fixture), same
	// verbatim handling as triggers:/dialogs: (core marshals WallFeature).
	{SectionWallFeatures, slotWallFeatures, nil},
}

// GridLayerCount is the number of grid layers (layerSections rows with a field
// accessor), computed in init; exported so core asserts its gridLayers() stays in
// lockstep (a 7th layer on either side panics at startup).
var GridLayerCount int

// init asserts layerSections covers every slot (slotWalls..slotPropYaw) exactly once
// (a missing slot panics) and tallies GridLayerCount.
func init() {
	seen := make(map[layerSlot]bool, len(layerSections))
	for _, s := range layerSections {
		if seen[s.slot] {
			panic(fmt.Sprintf("mapfile: duplicate layerSections slot %d", s.slot))
		}
		seen[s.slot] = true
		if s.field != nil {
			GridLayerCount++
		}
	}
	for slot := slotWalls; slot < slotCount; slot++ {
		if !seen[slot] {
			panic(fmt.Sprintf("mapfile: layerSections missing slot %d — add a row", slot))
		}
	}
}

// namedLayer pairs a grid layer's section name with its rows. Shared by validate
// (dimension checks) and Encode (header + row emit).
type namedLayer struct {
	name string
	rows []string
}

// NamedLayer is the exported view of a section→rows pairing, for cross-package
// callers (core.AreaFromMapFile) that iterate the required-layer roster.
type NamedLayer struct {
	Name string
	Rows []string
}

// RequiredLayers is the four mandatory grid layers in canonical order — the one
// source for the required section→field roster. Ceiling/elevation are excluded
// (optional, validated/encoded separately).
func (mf MapFile) RequiredLayers() []NamedLayer {
	return []NamedLayer{
		{SectionWalls, mf.Walls},
		{SectionFloor, mf.Floor},
		{SectionDecor, mf.Decor},
		{SectionProps, mf.Props},
	}
}

// requiredLayers is the internal namedLayer view of RequiredLayers, kept so
// validate/Encode reuse their existing field names.
func (mf MapFile) requiredLayers() []namedLayer {
	src := mf.RequiredLayers()
	out := make([]namedLayer, len(src))
	for i, l := range src {
		out[i] = namedLayer{l.Name, l.Rows}
	}
	return out
}

// Parse reads a .map file from r. Errors pinpoint the first malformed line.
func Parse(r io.Reader) (MapFile, error) {
	mf := MapFile{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	state := slotNone
	stateLevel := 0 // floor (@N) of the current entity section; 0 for grids/legacy
	lineNo := 0
	for sc.Scan() {
		lineNo++
		raw := strings.TrimRight(sc.Text(), "\r")

		// Section headers can appear anywhere and switch state — check first.
		if next, level, ok := sectionFor(raw); ok {
			if level != 0 && !slotSupportsLevel(next) {
				return mf, fmt.Errorf("line %d: section %q does not take an @floor suffix", lineNo, strings.TrimSpace(raw))
			}
			state, stateLevel = next, level
			// An empty crystals: section means "zero crystals", not
			// "unspecified" (see MapFile.CrystalsDefined).
			if next == slotCrystals {
				mf.CrystalsDefined = true
			}
			continue
		}

		// Blank lines are skipped globally. NOT `#`-prefixed lines — the wall
		// glyph IS `#`, so a comment prefix would collide with content.
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}

		if state == slotNone {
			if err := parseHeaderLine(&mf, line, lineNo); err != nil {
				return mf, err
			}
			continue
		}

		if state == slotEnemies {
			fields := strings.Fields(line)
			// 3 fields = legacy (AI defaults "none"); 4 = AI column present.
			if len(fields) != packFieldsLegacy && len(fields) != packFields {
				return mf, fmt.Errorf("line %d: expected '<kind[,kind...]> <x> <z> [ai]', got %q", lineNo, raw)
			}
			members, backCount, err := splitPackMembers(fields[0])
			if err != nil {
				return mf, fmt.Errorf("line %d: %v", lineNo, err)
			}
			if len(members) == 0 {
				return mf, fmt.Errorf("line %d: pack has no members", lineNo)
			}
			x, err := parseIntField(fields[1], "pack x", lineNo)
			if err != nil {
				return mf, err
			}
			z, err := parseIntField(fields[2], "pack z", lineNo)
			if err != nil {
				return mf, err
			}
			ai := ""
			if len(fields) == packFields {
				ai = strings.ToLower(fields[3])
				if !IsPackAIName(ai) {
					return mf, fmt.Errorf("line %d: unknown pack AI %q (expected one of %v)", lineNo, fields[3], PackAINames)
				}
			}
			mf.Packs = append(mf.Packs, MapPack{Members: members, BackCount: backCount, X: x, Z: z, Level: stateLevel, AI: ai})
			continue
		}

		if state == slotDoors {
			fields := strings.Fields(line)
			// 6 fields = legacy (style defaults building); 7 = style column present.
			if len(fields) != doorFieldsLegacy && len(fields) != doorFields {
				return mf, fmt.Errorf("line %d: expected '<name> <target_map> <target_door> <x> <z> <facing> [style]', got %q", lineNo, raw)
			}
			x, err := parseIntField(fields[3], "door x", lineNo)
			if err != nil {
				return mf, err
			}
			z, err := parseIntField(fields[4], "door z", lineNo)
			if err != nil {
				return mf, err
			}
			face := strings.ToLower(fields[5])
			if !IsFacingName(face) {
				return mf, fmt.Errorf("line %d: door facing must be north/east/south/west, got %q", lineNo, fields[5])
			}
			style := DoorStyleBuildingName
			if len(fields) == doorFields {
				style = strings.ToLower(fields[6])
				if !IsDoorStyleName(style) {
					return mf, fmt.Errorf("line %d: door style must be building/cave/field, got %q", lineNo, fields[6])
				}
			}
			mf.Doors = append(mf.Doors, MapDoor{
				Name:       fields[0],
				TargetMap:  fields[1],
				TargetDoor: fields[2],
				X:          x,
				Z:          z,
				Level:      stateLevel,
				Facing:     face,
				Style:      style,
			})
			continue
		}

		if state == slotChests {
			// Chest row: "item[,item...] X Z" or "(empty) X Z" (canonical ItemDefinition.Name).
			fields := strings.Fields(line)
			if len(fields) < chestFieldsMin {
				return mf, fmt.Errorf("line %d: expected '<item[,item...]> <x> <z>' or '(empty) <x> <z>', got %q", lineNo, raw)
			}
			// Item names may contain whitespace, so take the LAST two fields as X/Z.
			xField := fields[len(fields)-2]
			zField := fields[len(fields)-1]
			itemsToken := strings.Join(fields[:len(fields)-2], " ")
			x, err := parseIntField(xField, "chest x", lineNo)
			if err != nil {
				return mf, err
			}
			z, err := parseIntField(zField, "chest z", lineNo)
			if err != nil {
				return mf, err
			}
			var items []string
			if itemsToken != EmptyChestToken {
				for _, name := range strings.Split(itemsToken, ",") {
					name = strings.TrimSpace(name)
					if name == "" {
						return mf, fmt.Errorf("line %d: empty chest item entry", lineNo)
					}
					items = append(items, name)
				}
			}
			mf.Chests = append(mf.Chests, MapChest{Items: items, X: x, Z: z, Level: stateLevel})
			continue
		}

		if state == slotCrystals {
			// Crystal row: "X Z" — position only.
			fields := strings.Fields(line)
			if len(fields) != crystalFields {
				return mf, fmt.Errorf("line %d: expected '<x> <z>', got %q", lineNo, raw)
			}
			x, err := parseIntField(fields[0], "crystal x", lineNo)
			if err != nil {
				return mf, err
			}
			z, err := parseIntField(fields[1], "crystal z", lineNo)
			if err != nil {
				return mf, err
			}
			mf.Crystals = append(mf.Crystals, MapCrystal{X: x, Z: z, Level: stateLevel})
			continue
		}

		if state == slotDialogs {
			// One opaque JSON object per line, stored verbatim (core parses it). It
			// ends with '}', so it can't look like a section header (ends with ':').
			mf.Dialogs = append(mf.Dialogs, line)
			continue
		}

		if state == slotTriggers {
			// Opaque JSON-per-line, same handling as dialogs.
			mf.Triggers = append(mf.Triggers, line)
			continue
		}

		if state == slotLocations {
			// Opaque JSON-per-line, same handling as dialogs (core marshals Location).
			mf.Locations = append(mf.Locations, line)
			continue
		}

		if state == slotWallFeatures {
			// Opaque JSON-per-line, same handling as dialogs (core marshals WallFeature).
			mf.WallFeatures = append(mf.WallFeatures, line)
			continue
		}

		if stack := planeStackFor(&mf, state); stack != nil {
			// N planes of Height rows each, lowest-first; rows arrive contiguously
			// (blanks already skipped), so start a new plane when the current fills
			// to Height. Height is 0 only if size: is misordered after a grid —
			// fail pointedly rather than splitting every row into its own plane.
			if mf.Height == 0 {
				return mf, fmt.Errorf("line %d: a multi-plane section appears before size: (grid dimensions unknown)", lineNo)
			}
			appendPlaneRow(stack, raw, mf.Height)
			continue
		}

		if state == slotPropLevels {
			// Single Height-row grid of per-tile prop levels (base-36, '.' = auto).
			// appendGridRow tolerates an editor-inserted trailing blank but pinpoints a
			// non-blank overflow here rather than as a downstream size mismatch.
			if err := appendGridRow(&mf.PropLevels, raw, mf.Height, lineNo, "prop_levels: "); err != nil {
				return mf, err
			}
			continue
		}

		if state == slotDecorLevels {
			if err := appendGridRow(&mf.DecorLevels, raw, mf.Height, lineNo, "decor_levels: "); err != nil {
				return mf, err
			}
			continue
		}

		if state == slotPropYaw {
			// Single Height-row grid of per-tile prop facings ('.' = auto, else a step digit).
			if err := appendGridRow(&mf.PropYaw, raw, mf.Height, lineNo, "prop_yaw: "); err != nil {
				return mf, err
			}
			continue
		}

		if state == slotFaces {
			// One overridden tile per line: "x z NESW" ('.' = use base skin). A
			// malformed line is a loud error, not a silent drop.
			fields := strings.Fields(raw)
			if len(fields) != facesFieldCount || len(fields[2]) != faceSkinCount {
				return mf, fmt.Errorf("line %d: expected '<x> <z> <NESW>' (%d face skin chars), got %q", lineNo, faceSkinCount, raw)
			}
			fx, err := parseIntField(fields[0], "face x", lineNo)
			if err != nil {
				return mf, err
			}
			fz, err := parseIntField(fields[1], "face z", lineNo)
			if err != nil {
				return mf, err
			}
			var sk [faceSkinCount]byte
			copy(sk[:], fields[2])
			mf.Faces = append(mf.Faces, MapFace{X: fx, Z: fz, Skins: sk})
			continue
		}

		// Layer grid line. Past Height rows, blanks are tolerated (editors auto-insert
		// one before the next header) but a non-blank overflow is a structural error.
		// size: precedes every grid, so Height is always set.
		target := layerSlice(&mf, state)
		if target == nil {
			// A slot with neither a grid field nor a bespoke arm above — a new
			// layerSlot added without a parse handler. Unreachable for valid maps.
			panic(fmt.Sprintf("mapfile: section slot %d has no parse handler — add a bespoke arm or a grid field accessor", state))
		}
		if err := appendGridRow(target, raw, mf.Height, lineNo, ""); err != nil {
			return mf, err
		}
	}
	if err := sc.Err(); err != nil {
		return mf, err
	}
	if err := mf.validate(); err != nil {
		return mf, err
	}
	return mf, nil
}

// appendGridRow appends a grid row to target, tolerating an editor-inserted blank
// past the declared height (skipped) but rejecting a non-blank overflow as a
// structural error. section is the "<name>: " prefix for the error (empty for a
// generic layer grid). Shared by the prop_levels / decor_levels / layer arms.
func appendGridRow(target *[]string, raw string, height, lineNo int, section string) error {
	if len(*target) >= height {
		if strings.TrimSpace(raw) == "" {
			return nil
		}
		return fmt.Errorf("line %d: %sextra row past declared height %d", lineNo, section, height)
	}
	*target = append(*target, raw)
	return nil
}

// sectionFor maps a section-header line ("walls:", "enemies@3:", …) to its slot
// and voxel level. The trailing colon is required (a bare "walls" is not a
// header). An optional "@N" before the colon names the floor (level N) for a
// level-aware entity section; absent / "@0" = level 0 (the plain section). A
// malformed "@<non-int>" is treated as not-a-header (falls through to content).
func sectionFor(raw string) (slot layerSlot, level int, ok bool) {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasSuffix(trimmed, ":") {
		return slotNone, 0, false
	}
	name := trimmed[:len(trimmed)-1]
	if at := strings.IndexByte(name, '@'); at >= 0 {
		lv, err := strconv.Atoi(name[at+1:])
		if err != nil || lv < 0 {
			return slotNone, 0, false
		}
		level = lv
		name = name[:at]
	}
	for _, s := range layerSections {
		if s.name == name {
			return s.slot, level, true
		}
	}
	return slotNone, 0, false
}

// slotSupportsLevel reports whether a section may carry an "@N" floor suffix —
// the positional entity sections (one tile per row). Grid layers are still a
// single per-map plane in this schema, so a "@N" on one is an authoring error.
func slotSupportsLevel(slot layerSlot) bool {
	switch slot {
	case slotEnemies, slotChests, slotDoors, slotCrystals:
		return true
	}
	return false
}

// planeStackFor returns the MapFile multi-plane stack field for a stack slot
// (solids / propstack / decorstack), or nil for any other slot. The single
// name→field map behind the shared multi-plane parse arm.
func planeStackFor(mf *MapFile, slot layerSlot) *[][]string {
	switch slot {
	case slotSolids:
		return &mf.Solids
	case slotPropStack:
		return &mf.PropStack
	case slotDecorStack:
		return &mf.DecorStack
	}
	return nil
}

// appendPlaneRow appends raw to a multi-plane stack, starting a new plane when
// the current one has filled to height. Shared by the solids/propstack/decorstack arms.
func appendPlaneRow(stack *[][]string, raw string, height int) {
	if len(*stack) == 0 || len((*stack)[len(*stack)-1]) >= height {
		*stack = append(*stack, []string{})
	}
	last := len(*stack) - 1
	(*stack)[last] = append((*stack)[last], raw)
}

// layerSlice returns the MapFile grid field for a grid-layer slot, or nil for
// entity sections / unknown slots.
func layerSlice(mf *MapFile, slot layerSlot) *[]string {
	for _, s := range layerSections {
		if s.slot == slot && s.field != nil {
			return s.field(mf)
		}
	}
	return nil
}

func parseHeaderLine(mf *MapFile, line string, lineNo int) error {
	key, val, ok := splitKV(line)
	if !ok {
		return fmt.Errorf("line %d: expected 'key: value' or section header, got %q", lineNo, line)
	}
	switch key {
	case headerName:
		mf.Name = val
	case headerMaterials:
		mf.Materials = val
	case headerQuiet:
		mf.Quiet = val
	case headerWeather:
		mf.Weather = val
	case headerSize:
		w, h, err := parseSize(val)
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNo, err)
		}
		mf.Width, mf.Height = w, h
	case headerStart:
		x, z, face, err := parseStart(val)
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNo, err)
		}
		mf.StartX, mf.StartZ, mf.StartFace = x, z, face
	default:
		return fmt.Errorf("line %d: unknown header key %q", lineNo, key)
	}
	return nil
}

// validateOptionalGrid dimension-checks an optional single-grid layer (prop_levels
// / decor_levels): absent is fine, but a present grid must be exactly Height×Width.
func (mf *MapFile) validateOptionalGrid(name string, rows []string) error {
	if len(rows) == 0 {
		return nil
	}
	if err := mf.validateGridDims(name, rows); err != nil {
		return err
	}
	for i, row := range rows {
		// Each cell is the '.' auto sentinel or a level char ('0'..'9' then 'A'..'K'
		// for 10..20, same encoding as elevation). Anything else reads as level 0
		// downstream, silently flattening the prop/decor instead of failing here.
		for c := 0; c < len(row); c++ {
			if b := row[c]; b != '.' && !isLevelChar(b) {
				return fmt.Errorf("%s row %d col %d has bad level char %q (expected '.', %s)", name, i, c, string(row[c]), levelCharRangeHint())
			}
		}
	}
	return nil
}

// PropYawStepCount mirrors core.PropYawSteps (the authored-facing resolution); a
// core init assert keeps the two in lockstep. mapfile is the lower package and
// can't import core, so the count lives here and the validator derives its alphabet
// from it — no more accepting chars the core decoder silently drops to auto.
const PropYawStepCount = 12

// validatePropYawGrid dimension-checks the optional prop-facing grid and rejects
// stray chars. Its alphabet differs from the level grids: '.' auto plus the
// base-36-ish step digits '0'..'9' and lowercase 'a'.. that core.PropYawStepChar
// emits — but ONLY up to PropYawStepCount steps (10..11 → 'a'/'b' at 12 steps).
// Chars past that ('c'..'z') decode to "auto" in core, so reject them here instead
// of accepting a facing that silently does nothing and round-trips forever.
// Uppercase 'A'..'K' are LEVEL chars, not facings, so this must not reuse
// validateOptionalGrid's isLevelChar check.
func (mf *MapFile) validatePropYawGrid(rows []string) error {
	if len(rows) == 0 {
		return nil
	}
	if err := mf.validateGridDims(SectionPropYaw, rows); err != nil {
		return err
	}
	maxLetter := byte('a' + PropYawStepCount - 1 - 10) // last decodable step letter
	for i, row := range rows {
		for c := 0; c < len(row); c++ {
			b := row[c]
			if b == '.' || (b >= '0' && b <= '9') {
				continue
			}
			if PropYawStepCount > 10 && b >= 'a' && b <= maxLetter {
				continue
			}
			return fmt.Errorf("%s row %d col %d has bad facing char %q (expected '.', '0'..'9', or 'a'..%q)", SectionPropYaw, i, c, string(row[c]), string(maxLetter))
		}
	}
	return nil
}

// validateGridDims checks that rows is exactly Height rows of Width cols, tagging
// errors with name ("floor layer", "ceiling layer", "solids plane 2", "prop_levels"
// …). The single dimension check shared by every required + optional grid below.
func (mf *MapFile) validateGridDims(name string, rows []string) error {
	return ValidateGridDims(name, rows, mf.Width, mf.Height)
}

// ValidateGridDims checks that rows is exactly h rows of w cols, tagging errors with
// name. Exported free-function form so core's editor-path dimension check (which
// bypasses MapFile.validate) shares this one implementation instead of copying it.
func ValidateGridDims(name string, rows []string, w, h int) error {
	if len(rows) != h {
		return fmt.Errorf("%s has %d rows, size declares %d", name, len(rows), h)
	}
	for i, row := range rows {
		if len(row) != w {
			return fmt.Errorf("%s row %d has %d cols, size declares %d", name, i, len(row), w)
		}
	}
	return nil
}

// oobErr is the shared out-of-bounds error for a placed entity (pack/chest/crystal/
// door) at (x,z); what is the entity label (e.g. "pack" or `door "gate"`).
func (mf *MapFile) oobErr(what string, x, z int) error {
	return fmt.Errorf("%s at (%d,%d) outside map %dx%d", what, x, z, mf.Width, mf.Height)
}

// ValidateEntityBounds checks every placed entity (pack/chest/crystal/door) sits
// in-bounds. The single home for that check: validate() calls it on the load path,
// and core.AreaFromMapFile — the editor's direct build path, which skips validate() —
// calls it too, so neither path re-codes the bounds test (mirrors ValidateGridDims).
func (mf *MapFile) ValidateEntityBounds() error {
	inb := func(what string, x, z int) error {
		if !InBoundsWH(x, z, mf.Width, mf.Height) {
			return mf.oobErr(what, x, z)
		}
		return nil
	}
	for _, p := range mf.Packs {
		if err := inb("pack", p.X, p.Z); err != nil {
			return err
		}
	}
	for _, c := range mf.Chests {
		if err := inb("chest", c.X, c.Z); err != nil {
			return err
		}
	}
	for _, c := range mf.Crystals {
		if err := inb("crystal", c.X, c.Z); err != nil {
			return err
		}
	}
	for _, d := range mf.Doors {
		if err := inb(fmt.Sprintf("door %q", d.Name), d.X, d.Z); err != nil {
			return err
		}
	}
	return nil
}

// MaxFloorLevel is the highest floor an entity's @N section may name — the level
// MaxLevelChar ('K') encodes (0..20), the same alphabet as elevation/prop levels.
const MaxFloorLevel = 20

// validateFloor bounds-checks an entity's @N floor; what is the entity label.
func (mf *MapFile) validateFloor(what string, level, x, z int) error {
	if level < 0 || level > MaxFloorLevel {
		return fmt.Errorf("%s at (%d,%d) has floor %d outside 0..%d", what, x, z, level, MaxFloorLevel)
	}
	return nil
}

// isLevelChar reports whether b is a level char: '0'..'9' then 'A'..'K' (10..20).
// The 'K' upper bound == core's MaxElevationLevel (core/map.go init pins this), so
// the elevation and prop/decor-level grids share one alphabet definition.
func isLevelChar(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'A' && b <= MaxLevelChar)
}

// levelCharRangeHint describes the valid level alphabet for error messages,
// derived from MaxLevelChar so the hint can't go stale if the alphabet grows.
func levelCharRangeHint() string {
	return fmt.Sprintf("'0'..'9' or 'A'..'%c'", MaxLevelChar)
}

// ValidateGrids runs EVERY grid check — dimensions AND cell-char alphabet — over the
// required layers and each PRESENT optional grid (ceiling, elevation, solids/prop/
// decor stacks, prop_levels/prop_yaw/decor_levels). Absent optional grids are skipped
// (their readers default them). Exported so core's editor-path AreaFromMapFile shares
// the SAME checks as disk Load, rather than the weaker dims-only subset it used to run
// (which silently accepted bad level/yaw/elevation chars a disk load rejects).
func (mf *MapFile) ValidateGrids() error {
	for _, layer := range mf.RequiredLayers() {
		if err := mf.validateGridDims(layer.Name+" layer", layer.Rows); err != nil {
			return err
		}
	}
	// Ceiling: dims only (no cell alphabet). Absent → skipped (reader blanks it).
	if len(mf.Ceiling) > 0 {
		if err := mf.validateGridDims("ceiling layer", mf.Ceiling); err != nil {
			return err
		}
	}
	// Elevation: dims + level-char alphabet. Each cell must be a level char '0'..'9'
	// then 'A'..'K' for 10..20 (upper bound 'K' = core's MaxElevationLevel; core's
	// map.go init pins ElevationChar(MaxElevationLevel)=='K' so this can't drift).
	// Anything else reads as ground 0, silently flattening the geometry.
	if len(mf.Elevation) > 0 {
		if err := mf.validateGridDims("elevation layer", mf.Elevation); err != nil {
			return err
		}
		for i, row := range mf.Elevation {
			for c := 0; c < len(row); c++ {
				if b := row[c]; !isLevelChar(b) {
					return fmt.Errorf("elevation layer row %d col %d has bad level char %q (expected %s)", i, c, string(row[c]), levelCharRangeHint())
				}
			}
		}
	}
	// solids: optional voxel stack, each plane a full Height×Width grid. Only
	// dimensions checked (cell-char alphabet is core's); guards against a ragged plane.
	for L, plane := range mf.Solids {
		if err := mf.validateGridDims(fmt.Sprintf("solids plane %d", L), plane); err != nil {
			return err
		}
	}
	// propstack / decorstack: same per-plane dimension check as solids.
	for L, plane := range mf.PropStack {
		if err := mf.validateGridDims(fmt.Sprintf("%s plane %d", SectionPropStack, L), plane); err != nil {
			return err
		}
	}
	for L, plane := range mf.DecorStack {
		if err := mf.validateGridDims(fmt.Sprintf("%s plane %d", SectionDecorStack, L), plane); err != nil {
			return err
		}
	}
	// prop_levels / decor_levels: dims + level-char alphabet; prop_yaw its own facing alphabet.
	if err := mf.validateOptionalGrid(SectionPropLevels, mf.PropLevels); err != nil {
		return err
	}
	if err := mf.validatePropYawGrid(mf.PropYaw); err != nil {
		return err
	}
	if err := mf.validateOptionalGrid(SectionDecorLevels, mf.DecorLevels); err != nil {
		return err
	}
	return nil
}

func (mf *MapFile) validate() error {
	if mf.Width <= 0 || mf.Height <= 0 {
		return fmt.Errorf("size must be >0x0; got %dx%d", mf.Width, mf.Height)
	}
	// Ceiling/Elevation optional: missing → blank layer so downstream can index them
	// like the others (ceiling = "no roof", elevation = flat all-'0'). Fill BEFORE
	// ValidateGrids so a partial (present-but-ragged) one still fails as malformed.
	if len(mf.Ceiling) == 0 {
		mf.Ceiling = BlankLayer(mf.Width, mf.Height, CeilingOpenChar)
	}
	if len(mf.Elevation) == 0 {
		mf.Elevation = BlankLayer(mf.Width, mf.Height, ElevationGroundChar)
	}
	if err := mf.ValidateGrids(); err != nil {
		return err
	}
	// faces: bounds-check each so a stray line can't feed an off-map index.
	for _, f := range mf.Faces {
		if !InBoundsWH(f.X, f.Z, mf.Width, mf.Height) {
			return fmt.Errorf("faces entry (%d,%d) outside map", f.X, f.Z)
		}
	}
	if !InBoundsWH(mf.StartX, mf.StartZ, mf.Width, mf.Height) {
		return fmt.Errorf("start (%d,%d) outside map", mf.StartX, mf.StartZ)
	}
	// StartFace must be a canonical facing — else a MapFile built bypassing Parse
	// would Save a "start: X Z <bad>" line that Parse rejects on reload.
	if !IsFacingName(mf.StartFace) {
		return fmt.Errorf("start facing %q invalid", mf.StartFace)
	}
	// Entity bounds checked here (not in the parser) so a typo surfaces at load
	// rather than as a silently-skipped entry at runtime. Shared with the editor's
	// direct-AreaFromMapFile path via ValidateEntityBounds.
	if err := mf.ValidateEntityBounds(); err != nil {
		return err
	}
	for _, p := range mf.Packs {
		if err := mf.validateFloor("pack", p.Level, p.X, p.Z); err != nil {
			return err
		}
		// A zero-member pack encodes as " X Z" (no kind column), which the parser rejects
		// on field count — reject here so Save can't write a .map Load refuses.
		if len(p.Members) == 0 {
			return fmt.Errorf("pack at (%d,%d) has no members", p.X, p.Z)
		}
		// Members encode comma/semicolon-joined (',' within a row, ';' splits the
		// front/back rows) and re-split on those, so a member name containing either —
		// or whitespace, which the row decoder also splits on — would re-parse as
		// phantom members. An empty member encodes "a,,b", which the parser rejects.
		// Reject at the data-model boundary (mirrors the chest-item and door-name
		// guards) so it fails loudly at save.
		for _, m := range p.Members {
			if m == "" {
				return fmt.Errorf("pack at (%d,%d) has an empty member", p.X, p.Z)
			}
			if strings.ContainsAny(m, ",; \t") {
				return fmt.Errorf("pack at (%d,%d) member %q must not contain a comma, semicolon, or whitespace", p.X, p.Z, m)
			}
		}
	}
	for _, c := range mf.Chests {
		if err := mf.validateFloor("chest", c.Level, c.X, c.Z); err != nil {
			return err
		}
		// Items encode comma-joined and re-split on ',', so an item name containing a
		// comma would silently re-parse as two items, and an empty item is rejected by
		// the parser. Reject at the data-model boundary (mirrors the door-name whitespace
		// guard) so it fails loudly at save.
		for _, item := range c.Items {
			if item == "" {
				return fmt.Errorf("chest at (%d,%d) has an empty item entry", c.X, c.Z)
			}
			if strings.ContainsRune(item, ',') {
				return fmt.Errorf("chest at (%d,%d) item %q must not contain a comma", c.X, c.Z, item)
			}
		}
	}
	for _, c := range mf.Crystals {
		if err := mf.validateFloor("crystal", c.Level, c.X, c.Z); err != nil {
			return err
		}
	}
	// Doors: bounds, non-empty name + target, no duplicate names (runtime resolves
	// by name, so duplicates would be ambiguous).
	seenNames := make(map[string]struct{}, len(mf.Doors))
	for _, d := range mf.Doors {
		if err := mf.validateFloor(fmt.Sprintf("door %q", d.Name), d.Level, d.X, d.Z); err != nil {
			return err
		}
		if d.Name == "" {
			return fmt.Errorf("door at (%d,%d) has empty name", d.X, d.Z)
		}
		// The 3 leading door fields are space-delimited and variable-width, so
		// whitespace in any is unrecoverable on re-parse — reject at the data-model
		// boundary so it fails loudly at save, not as a .map the parser later rejects.
		if strings.ContainsAny(d.Name, " \t") {
			return fmt.Errorf("door name %q must not contain whitespace", d.Name)
		}
		if !d.HasTarget() {
			return fmt.Errorf("door %q at (%d,%d) missing target_map/target_door", d.Name, d.X, d.Z)
		}
		if strings.ContainsAny(d.TargetMap, " \t") {
			return fmt.Errorf("door %q target_map %q must not contain whitespace", d.Name, d.TargetMap)
		}
		if strings.ContainsAny(d.TargetDoor, " \t") {
			return fmt.Errorf("door %q target_door %q must not contain whitespace", d.Name, d.TargetDoor)
		}
		// Facing/style must match what the parser accepts on reload (an empty Style
		// is legal — the encoder fills it building).
		if !IsFacingName(d.Facing) {
			return fmt.Errorf("door %q facing %q must be north/east/south/west", d.Name, d.Facing)
		}
		if d.Style != "" && !IsDoorStyleName(d.Style) {
			return fmt.Errorf("door %q style %q must be building/cave/field", d.Name, d.Style)
		}
		if _, dup := seenNames[d.Name]; dup {
			return fmt.Errorf("duplicate door name %q in map", d.Name)
		}
		seenNames[d.Name] = struct{}{}
	}
	return nil
}

func splitKV(line string) (string, string, bool) {
	idx := strings.IndexByte(line, ':')
	if idx < 0 {
		return "", "", false
	}
	return strings.TrimSpace(line[:idx]), strings.TrimSpace(line[idx+1:]), true
}

func parseSize(val string) (int, int, error) {
	parts := strings.SplitN(val, "x", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("size must be WxH, got %q", val)
	}
	w, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	h, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil {
		return 0, 0, fmt.Errorf("size must be WxH integers, got %q", val)
	}
	return w, h, nil
}

func parseStart(val string) (int, int, string, error) {
	fields := strings.Fields(val)
	if len(fields) != startFields {
		return 0, 0, "", fmt.Errorf("start must be 'X Z facing', got %q", val)
	}
	x, err1 := strconv.Atoi(fields[0])
	z, err2 := strconv.Atoi(fields[1])
	if err1 != nil || err2 != nil {
		return 0, 0, "", fmt.Errorf("start coordinates must be integers, got %q", val)
	}
	face := strings.ToLower(fields[2])
	if !IsFacingName(face) {
		return 0, 0, "", fmt.Errorf("start facing must be north/east/south/west, got %q", fields[2])
	}
	return x, z, face, nil
}

// IsFacingName reports whether s is a canonical facing (case-insensitive,
// matching facingFromName so disk validation and conversion agree).
func IsFacingName(s string) bool {
	return nameInList(s, FacingNames[:])
}

// parseIntField parses a numeric field with the shared "line N: bad <name> %q" wrap.
func parseIntField(s, name string, lineNo int) (int, error) {
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("line %d: bad %s %q", lineNo, name, s)
	}
	return v, nil
}

// Positional field counts for the non-custom-enemy entity sections, so the parse
// width check and encode-format verb count cite one source. Sections with an
// optional trailing column carry both a Legacy and current width.
const (
	packFieldsLegacy = 3 // "kind[,kind...] X Z" (AI defaults to none)
	packFields       = 4 // + trailing AI column
	doorFieldsLegacy = 6 // "name target_map target_door X Z facing"
	doorFields       = 7 // + trailing style column
	chestFieldsMin   = 3 // "item[,item...] X Z" — item token may span fields, so >= not ==
	crystalFields    = 2 // "X Z"
	facesFieldCount  = 3 // "X Z NESW" (the NESW token is one field of faceSkinCount chars)
	startFields      = 3 // header "start: X Z facing"
)

// faceSkinCount is the per-direction skin char count in a faces row's NESW token,
// = len(MapFace.Skins). Named so the parse guard cites the array size, not 4.
const faceSkinCount = len(MapFace{}.Skins)

// Per-section encode format strings, broken out so init() can assert their
// `%`-verb counts match the field-count constants above (a mismatch panics at
// startup instead of writing a row the parser rejects). pack/doorEncodeFormat
// cover the current widths; legacy shorter rows use their own formats below.
const (
	// packFieldsLegacy / packFields verbs.
	packEncodeFormatLegacy = "%s %d %d\n"
	packEncodeFormat       = "%s %d %d %s\n"
	// chestFieldsMin verbs.
	chestEncodeFormat = "%s %d %d\n"
	// doorFields verbs; style always written, so the legacy 6-field row is never
	// emitted — older maps pick up the column on re-save.
	doorEncodeFormat = "%s %s %s %d %d %s %s\n"
	// crystalFields verbs.
	crystalEncodeFormat = "%d %d\n"
	// facesFieldCount verbs; NESW is one %s field, so 3 verbs not 3+faceSkinCount.
	facesEncodeFormat = "%d %d %s\n"
)

// fprintfVerbCount counts `%`-verbs in a format string (literal `%%` skipped).
func fprintfVerbCount(format string) int {
	verbs := 0
	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			continue
		}
		if i+1 < len(format) && format[i+1] == '%' {
			i++
			continue
		}
		verbs++
	}
	return verbs
}

// init asserts each per-section encode format's `%`-verb count matches its
// parser field-count constant (a mismatch panics at startup). chestEncodeFormat
// checks against chestFieldsMin since a chest's item token may contain spaces.
func init() {
	formatChecks := []struct {
		name   string
		format string
		fields int
	}{
		{"packEncodeFormatLegacy", packEncodeFormatLegacy, packFieldsLegacy},
		{"packEncodeFormat", packEncodeFormat, packFields},
		{"chestEncodeFormat", chestEncodeFormat, chestFieldsMin},
		{"doorEncodeFormat", doorEncodeFormat, doorFields},
		{"crystalEncodeFormat", crystalEncodeFormat, crystalFields},
		{"facesEncodeFormat", facesEncodeFormat, facesFieldCount},
	}
	for _, fc := range formatChecks {
		if verbs := fprintfVerbCount(fc.format); verbs != fc.fields {
			panic(fmt.Sprintf("mapfile: %s has %d verbs, expected %d to match its field-count constant", fc.name, verbs, fc.fields))
		}
	}
}

// leveledSectionName returns a section's on-disk header for a given floor: the
// plain name at level 0 (byte-identical to a single-floor map), else "name@N".
// Inverse of the @N parse in sectionFor.
func leveledSectionName(name string, level int) string {
	if level == 0 {
		return name
	}
	return name + "@" + strconv.Itoa(level)
}

// entityLevels returns the distinct floor levels present across count entities
// (levelAt(i) is entity i's level), sorted ascending. includeZero forces level 0
// into the set so an "always emit the base section" rule (enemies/chests, or a
// defined-but-empty crystals:) still writes its header when no entity sits there.
func entityLevels(count int, levelAt func(i int) int, includeZero bool) []int {
	seen := make(map[int]bool)
	if includeZero {
		seen[0] = true
	}
	for i := 0; i < count; i++ {
		seen[levelAt(i)] = true
	}
	out := make([]int, 0, len(seen))
	for l := range seen {
		out = append(out, l)
	}
	sort.Ints(out)
	return out
}

// writePlaneStack emits a multi-plane section ("name:" + every plane's rows,
// lowest-first, contiguous), but ONLY when the stack is non-empty (byte-identical
// otherwise). Shared by solids/propstack/decorstack. The parser re-splits by Height.
func writePlaneStack(bw *bufio.Writer, name string, stack [][]string) {
	if len(stack) == 0 {
		return
	}
	fmt.Fprintln(bw, name+":")
	for _, plane := range stack {
		for _, row := range plane {
			fmt.Fprintln(bw, row)
		}
	}
}

// writeVerbatimSection emits "name:" + each row verbatim, but ONLY when rows is
// non-empty (byte-identical otherwise). Shared by prop_levels/decor_levels/dialogs/triggers.
func writeVerbatimSection(bw *bufio.Writer, name string, rows []string) {
	if len(rows) == 0 {
		return
	}
	fmt.Fprintln(bw, name+":")
	for _, row := range rows {
		fmt.Fprintln(bw, row)
	}
}

// writeLeveledSection emits items grouped into per-floor "section@N" blocks
// (ascending; base level-0 block forced when forceBase). Single home for the
// pack/chest/door/crystal emit scaffold — emit writes one row and may error
// (the door target guard is the only caller that does).
func writeLeveledSection[T any](bw *bufio.Writer, section string, items []T, levelOf func(T) int, forceBase bool, emit func(*bufio.Writer, T) error) error {
	for _, lvl := range entityLevels(len(items), func(i int) int { return levelOf(items[i]) }, forceBase) {
		fmt.Fprintln(bw, leveledSectionName(section, lvl)+":")
		for _, e := range items {
			if levelOf(e) != lvl {
				continue
			}
			if err := emit(bw, e); err != nil {
				return err
			}
		}
	}
	return nil
}

// Encode writes mf in the canonical .map format; fixed layer order so maps diff cleanly.
func (mf MapFile) Encode(w io.Writer) error {
	bw := bufio.NewWriter(w)
	// Free-text header values are written trimmed: the parser TrimSpaces every content
	// line globally, so emitting surrounding whitespace would not survive a reload
	// (Save→Load→Encode must be byte-stable).
	fmt.Fprintf(bw, headerName+": %s\n", strings.TrimSpace(mf.Name))
	fmt.Fprintf(bw, headerMaterials+": %s\n", strings.TrimSpace(mf.Materials))
	fmt.Fprintf(bw, headerQuiet+": %s\n", strings.TrimSpace(mf.Quiet))
	// Weather is optional: written only when non-auto so pre-weather maps stay byte-stable.
	if wthr := strings.TrimSpace(mf.Weather); wthr != "" {
		fmt.Fprintf(bw, headerWeather+": %s\n", wthr)
	}
	fmt.Fprintf(bw, headerSize+": %dx%d\n", mf.Width, mf.Height)
	fmt.Fprintf(bw, headerStart+": %d %d %s\n", mf.StartX, mf.StartZ, mf.StartFace)
	ceiling := OptionalLayerOrBlank(mf.Ceiling, mf.Width, mf.Height, CeilingOpenChar)
	elevation := OptionalLayerOrBlank(mf.Elevation, mf.Width, mf.Height, ElevationGroundChar)
	for _, layer := range append(mf.requiredLayers(), namedLayer{SectionCeiling, ceiling}, namedLayer{SectionElevation, elevation}) {
		fmt.Fprintf(bw, "%s:\n", layer.name)
		for _, row := range layer.rows {
			fmt.Fprintln(bw, row)
		}
	}
	// solids: appended only for a gapped map (heightfield omits it, byte-identical).
	// Planes emit lowest-first as contiguous Height-row blocks; parser re-splits by Height.
	writePlaneStack(bw, SectionSolids, mf.Solids)
	// propstack / decorstack: appended only for a multi-floor map (a single-floor
	// map projects content into props:/decor: + *_levels: and omits these, staying
	// byte-identical).
	writePlaneStack(bw, SectionPropStack, mf.PropStack)
	writePlaneStack(bw, SectionDecorStack, mf.DecorStack)
	// prop_levels / decor_levels: appended only when some entity sits above its
	// auto surface (handled in writeVerbatimSection).
	writeVerbatimSection(bw, SectionPropLevels, mf.PropLevels)
	writeVerbatimSection(bw, SectionPropYaw, mf.PropYaw)
	writeVerbatimSection(bw, SectionDecorLevels, mf.DecorLevels)
	// faces: one line per overridden tile; omitted when none (byte-identical).
	if len(mf.Faces) > 0 {
		fmt.Fprintln(bw, SectionFaces+":")
		for _, f := range mf.Faces {
			fmt.Fprintf(bw, facesEncodeFormat, f.X, f.Z, string(f.Skins[:]))
		}
	}
	// enemies/chests: base section ALWAYS emitted (level 0, possibly empty); each
	// higher floor with packs/chests appends an "@N" section. Single-floor maps
	// have only level-0 entities, so output is byte-identical to the legacy form.
	if err := writeLeveledSection(bw, SectionEnemies, mf.Packs, func(p MapPack) int { return p.Level }, true,
		func(bw *bufio.Writer, p MapPack) error {
			// Single-member packs encode as the legacy "kind X Z" line; the AI column
			// is appended only when non-default, so stationary packs stay 3 fields.
			members := encodePackMembers(p.Members, p.BackCount)
			ai := strings.ToLower(strings.TrimSpace(p.AI))
			if ai == "" || ai == PackAINoneName {
				fmt.Fprintf(bw, packEncodeFormatLegacy, members, p.X, p.Z)
			} else {
				fmt.Fprintf(bw, packEncodeFormat, members, p.X, p.Z, ai)
			}
			return nil
		}); err != nil {
		return err
	}
	if err := writeLeveledSection(bw, SectionChests, mf.Chests, func(c MapChest) int { return c.Level }, true,
		func(bw *bufio.Writer, c MapChest) error {
			token := EmptyChestToken
			if len(c.Items) > 0 {
				token = strings.Join(c.Items, ",")
			}
			fmt.Fprintf(bw, chestEncodeFormat, token, c.X, c.Z)
			return nil
		}); err != nil {
		return err
	}
	// doors: appended only for floors that have a door (missing ⇒ zero-doors,
	// byte-identical). Level 0 is NOT forced — a door-free map writes nothing.
	if err := writeLeveledSection(bw, SectionDoors, mf.Doors, func(d MapDoor) int { return d.Level }, false,
		func(bw *bufio.Writer, d MapDoor) error {
			// Refuse a door without a complete destination — it would emit a row the
			// parser collapses to too few fields and rejects on reload. validate()
			// already enforces HasTarget(); mirror it here so a direct Encode (which
			// skips validate) can't produce an unparseable byte stream.
			if !d.HasTarget() {
				return fmt.Errorf("door %q has incomplete target (map=%q, door=%q); both must be set", d.Name, d.TargetMap, d.TargetDoor)
			}
			style := d.Style
			if style == "" {
				style = DoorStyleBuildingName
			}
			fmt.Fprintf(bw, doorEncodeFormat, d.Name, d.TargetMap, d.TargetDoor, d.X, d.Z, d.Facing, style)
			return nil
		}); err != nil {
		return err
	}
	// crystals: base section emits when defined at all (CrystalsDefined OR a row),
	// so a legacy map stays byte-identical; higher floors append "@N". Rows are
	// position-only; charge state lives in SaveData.
	if err := writeLeveledSection(bw, SectionCrystals, mf.Crystals, func(c MapCrystal) int { return c.Level }, mf.CrystalsDefined,
		func(bw *bufio.Writer, c MapCrystal) error {
			fmt.Fprintf(bw, crystalEncodeFormat, c.X, c.Z)
			return nil
		}); err != nil {
		return err
	}
	// dialogs / triggers: emit only when present (byte-identical otherwise); each
	// entry is a pre-encoded JSON object written verbatim.
	writeVerbatimSection(bw, SectionDialogs, mf.Dialogs)
	writeVerbatimSection(bw, SectionTriggers, mf.Triggers)
	writeVerbatimSection(bw, SectionLocations, mf.Locations)
	writeVerbatimSection(bw, SectionWallFeatures, mf.WallFeatures)
	return bw.Flush()
}

func Load(path string) (MapFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return MapFile{}, err
	}
	defer f.Close()
	return Parse(f)
}

// AtomicWrite stages the bytes produced by write into a sibling ".tmp" then renames
// it over path. os.Rename is atomic on a single volume, so a crash mid-write leaves
// the PRIOR file intact instead of a truncated one. The close/flush error is captured
// (a deferred Close would swallow it), preferring write's error if both fire; the temp
// is cleaned up best-effort on failure. The parent dir must already exist. Single home
// for the temp-then-rename pattern — the map encoder and core's save-blob writer share it.
func AtomicWrite(path string, write func(io.Writer) error) error {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	err = write(f)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

func Save(path string, mf MapFile) error {
	// Validate BEFORE touching disk — same check Parse runs, so a saved map reloads.
	if err := mf.validate(); err != nil {
		return fmt.Errorf("refusing to save invalid map %q: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), AssetDirMode); err != nil {
		return err
	}
	// I/O failure mid-encode (disk full, network drive) must not destroy the prior good map.
	return AtomicWrite(path, mf.Encode)
}

// mapDirEntries returns dir's non-dir .map entries (case-insensitive) — the shared
// read+filter behind List/ListByModTime. A missing dir is NOT an error (nil, nil).
func mapDirEntries(dir string) ([]os.DirEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]os.DirEntry, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(e.Name()), Ext) {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

// List returns the .map files in dir, sorted alphabetically.
func List(dir string) ([]string, error) {
	entries, err := mapDirEntries(dir)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, filepath.Join(dir, e.Name()))
	}
	sort.Strings(out)
	return out, nil
}

// ListByModTime returns the .map files in dir, newest-modified first (editor's
// Open modal). Stat failures drop the individual entry, not the whole list.
func ListByModTime(dir string) ([]string, error) {
	entries, err := mapDirEntries(dir)
	if err != nil {
		return nil, err
	}
	type entry struct {
		path string
		mod  int64
	}
	rows := make([]entry, 0, len(entries))
	for _, e := range entries {
		info, ierr := e.Info()
		if ierr != nil {
			continue
		}
		rows = append(rows, entry{path: filepath.Join(dir, e.Name()), mod: info.ModTime().UnixNano()})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].mod > rows[j].mod })
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.path
	}
	return out, nil
}

// BlankLayer returns a width × height grid filled with c (seeds fresh/resized layers).
func BlankLayer(width, height int, c byte) []string {
	rows := make([]string, height)
	row := strings.Repeat(string(c), width)
	for i := range rows {
		rows[i] = row
	}
	return rows
}

// OptionalLayerOrBlank returns rows when present, else a blank all-`c` layer.
// The "absent optional layer ⇒ blank" rule shared by Encode and core/areas.go.
// (validate keeps its own switch because it ALSO width-validates a present layer.)
func OptionalLayerOrBlank(rows []string, width, height int, c byte) []string {
	if len(rows) == 0 {
		return BlankLayer(width, height, c)
	}
	return rows
}

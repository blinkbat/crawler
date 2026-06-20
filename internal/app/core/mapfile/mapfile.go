// Package mapfile is the on-disk representation of an explorable area. The
// format is plain text, multi-section: header lines, then one ASCII grid per
// editing layer (walls / floor / decor / props / ceiling / elevation), then
// enemy / chest / door spawn sections. Chosen so a map diffs cleanly in git,
// can be glanced at in any editor without parsing, and gives the editor's
// layer system a 1:1 mapping with on-disk structure.
//
// Layer character conventions:
//
//	walls  : per-tile CLIFF-FACE SKIN (legacy section name). These no longer
//	         block — a wall is the rendered vertical face of an elevation step.
//	         '.' default (plain rock) skin, '#' explicit rock, '+' rock+light
//	         ivy, '=' rock+heavy ivy, '&' rock cracked, '$' rock crumbling. The
//	         skin only shows where the tile's elevation exposes a face.
//	floor  : '.' auto-variant (per-tile hash), 'g' grass, 'd' dirt,
//	         'k' dark grass, 's' stone, 'c' cobblestone path, 'w' planks,
//	         '~' shallow water, 'W' deep water (blocks), 'n' sand, 'i' snow,
//	         '^'/'>'/'v'/'<' ramp ascending N/E/S/W (walkable; bridges the
//	         tile's elevation level to one higher in the arrow's direction)
//	decor  : '.' auto-scatter, '_' force-empty, 'b' bush, 'm' mushroom,
//	         'p' pebble cluster, ',' tall grass, 'f' wildflowers,
//	         'v' clover, 'r' reeds, 'o' bones, 'x' scorch, '!' blood,
//	         '*' cobweb, 't' stump, 'l' fallen log, 'L' leaf pile,
//	         'A' archway anchor (left), 'a' archway tail (right) — both
//	         walkable; the arch spans 2 tiles along +X, 'y' lilypad
//	         (swamp dressing — pure decor, never blocks).
//	props  : '.' empty, 'T' tree, 'X' tree XL, '|' tall tree,
//	         '@' twin trees, '/' young tree, 'O' boulder,
//	         'B' bush (large), 'C' crate, 'R' barrel, 'U' urn,
//	         'S' stalagmite, 'P' pillar, 'I' broken pillar,
//	         'M' statue, 'Q' obelisk, 'F' fountain,
//	         'K' rock cairn (1 tile), 'J' rock formation anchor (top-left
//	         of a 2×2 footprint), 'j' formation tail (the other 3 tiles
//	         of the 2×2). All blocking; the anchor's mesh covers the
//	         whole footprint and tails render nothing.
//	ceiling  : '.' open (sky shows through), '#' solid overhead slab.
//	elevation: per-tile ground LEVEL — '0'..'9' for 0..9 then 'A'..'K' for
//	         10..20 (base-36, one char per cell; blank/absent ⇒ '0'). The world
//	         is built entirely from elevation: the walkable baseline sits at
//	         level 10, walls/cliffs rise above it and pits drop below. A ramp
//	         floor tile stores its LOW level here; it rises one level toward
//	         its arrow. Any unramped level change between adjacent tiles is an
//	         impassable cliff (and renders a face). Optional layer — pre-
//	         elevation maps load as all-'0' (flat).
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
	Width     int
	Height    int
	StartX    int
	StartZ    int
	StartFace string
	Walls     []string
	Floor     []string
	Decor     []string
	Props     []string
	// Ceiling is the optional fifth grid. .map files written before the
	// ceiling: section existed parse with Ceiling left empty, which the
	// loader fills with a blank "no ceiling" layer so older maps stay
	// compatible with no manual edit.
	Ceiling []string
	// Elevation is the optional sixth grid: per-tile ground LEVEL ('0'..'9').
	// Like Ceiling, .map files written before this section existed parse with
	// Elevation empty, which the loader fills with a blank all-'0' (flat)
	// layer so older maps stay compatible with no manual edit.
	Elevation []string
	// Solids is the optional voxel-stack section: Solids[level] is a full
	// Height×Width grid of cube/air chars ('0' = air, anything else = a solid
	// cube's material char), planes stacked lowest-level-first. Written ONLY for
	// a map that has a gap (a floating cube over air) that the single-height
	// elevation: layer can't express; a pure heightfield omits it and stays
	// byte-identical, exactly like the doors / crystals optional sections. When
	// present, an elevation: section is still written (column tops) as a
	// graceful downgrade for readers that ignore solids:.
	Solids [][]string
	// PropLevels is the optional per-tile prop-LEVEL grid: one base-36 char per
	// (x,z) giving the voxel level the prop on that tile sits on, or '.' for
	// "auto" (rest on the column's lowest standable surface). Written ONLY when
	// some prop sits above its auto surface (a tree on a bridge deck); a map whose
	// props all sit on the ground omits it and stays byte-identical, like solids:.
	PropLevels []string
	// DecorLevels is the decor analogue of PropLevels (per-tile decor level, '.' =
	// auto). Optional / written only when some decor sits above its auto surface.
	DecorLevels []string
	// Faces holds per-tile cliff-face skin overrides (N/E/S/W); optional, one line
	// per overridden tile. Empty for any map that uses only base/whole-tile skins.
	Faces []MapFace
	Packs []MapPack
	// Chests is the authored chest list. Each entry's Items field is a
	// comma-separated list of item names ("Morsel of Cheese,Bat Jerky")
	// matching ItemDefinition.Name. Empty list = an empty chest (renders
	// open by default).
	Chests []MapChest
	// CustomEnemies is the author-defined enemy template list. Optional;
	// older .map files without a `custom_enemies:` section parse with an
	// empty slice and round-trip back to disk without one. New maps that
	// define custom enemies emit the section after `doors:`.
	CustomEnemies []MapCustomEnemy
	// Doors is the authored door list. Each door names itself, names
	// its destination map + matching-door name (or "self" for same-map
	// portals), and records the tile + post-transition facing. The
	// runtime resolves doors at step-on time: load destination map,
	// look up the named door, spawn the player at its tile with its
	// facing. Bidirectional pairs are author-authored (both ends
	// reference each other); the engine doesn't infer pairs.
	Doors []MapDoor
	// Crystals is the authored healing-crystal list — one tile position per
	// entry. Optional; older .map files without a `crystals:` section parse
	// with an empty slice and round-trip back to disk without one, exactly
	// like the doors / custom_enemies sections.
	Crystals []MapCrystal
	// CrystalsDefined records whether the parsed file carried a `crystals:`
	// section header at all — distinguishing "section present but empty"
	// (the author deliberately wants zero crystals) from "section absent"
	// (a legacy map predating editable crystals, which the runtime fills with
	// a default entrance crystal). Encode writes the section whenever this is
	// set, so a zero-crystal map round-trips as zero rather than reverting to
	// the legacy fallback.
	CrystalsDefined bool
	// Dialogs is the authored conversation list, stored as one OPAQUE
	// JSON object per line in the optional `dialogs:` section. This leaf
	// I/O package stays JSON-agnostic — it reads each line verbatim and
	// writes it back unchanged; core (which already owns encoding/json)
	// marshals DialogDefinition values to/from these strings. Older .map
	// files without the section parse with an empty slice and round-trip
	// without one, exactly like the custom_enemies / doors sections.
	Dialogs []string
	// Triggers is the authored dialog-trigger list, stored as one OPAQUE JSON
	// object per line in the optional `triggers:` section — same verbatim,
	// JSON-agnostic handling as Dialogs (core marshals DialogTrigger values).
	Triggers []string
}

// MapPack is one authored pack at a tile. Members is a non-empty list of
// enemy-kind names, ordered FRONT row first then BACK row; on-disk format is
// "kind[,kind...] X Z [ai]" — three fields stay the legacy form (AI defaults to
// "none"), an optional fourth AI field names a non-default movement style. A
// ';' inside the member field splits the front group from the back group
// ("f,f;b,b"); no ';' = all front (the legacy shape).
type MapPack struct {
	Members []string
	// BackCount is how many of Members (which are ordered front-first) stand in
	// the BACK row — the last BackCount entries. Zero (the default / zero value)
	// means every member is front row, so legacy packs and any MapPack built
	// without rows read as all-front and round-trip byte-identically.
	BackCount int
	X         int
	Z         int
	// AI is the on-disk name of the pack's movement style (see
	// PackAINames). Empty means "use the default" — the loader maps
	// that to PackAINone, the stationary mode.
	AI string
}

// Pack AI names — canonical on-disk strings for each core.PackAI value.
// Defined in mapfile (the I/O package) so the leaf format never imports
// core; core's PackAIName / PackAIFromName alias these via a table.
const (
	PackAINoneName        = "none"
	PackAIJunkyardDogName = "junkyard_dog"
	PackAIPatrolName      = "patrol"
	PackAISkittishName    = "skittish"
)

// PackAINames is the canonical on-disk order for pack-AI strings,
// matching the core.PackAI enum by index. PackAINoneName at index 0
// means an absent / empty AI column resolves to the no-op behavior —
// the default per the editor's "new packs are stationary" rule. Order
// here MUST match core's PackAI iota (core/areas.go's packAIDefs
// init asserts the two stay aligned row-for-row).
var PackAINames = [...]string{
	PackAINoneName,
	PackAIJunkyardDogName,
	PackAIPatrolName,
	PackAISkittishName,
}

// IsPackAIName reports whether s names one of the canonical pack-AI
// modes (case-insensitive).
func IsPackAIName(s string) bool {
	low := strings.ToLower(s)
	for _, name := range PackAINames {
		if name == low {
			return true
		}
	}
	return false
}

// splitPackMembers parses a pack's member field. Members are comma-separated; an
// optional single ';' splits the FRONT row (before) from the BACK row (after).
// Returns the flat member list ordered front-first and how many trailing entries
// are back row. No ';' = all front (the legacy shape), so BackCount is 0.
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

// parsePackGroup splits one comma-separated member group, trimming each token
// and rejecting an empty one. An empty/whitespace group yields no members (a
// pack with no ';' has an empty back group; an all-back pack has an empty front).
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

// encodePackMembers is the inverse of splitPackMembers: members ordered
// front-first with the last backCount in the back row. backCount<=0 writes the
// plain comma list (legacy shape, byte-identical for row-less packs); otherwise
// it emits "front;back" (front may be empty for an all-back pack).
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

// MapChest is one authored chest at a tile. On-disk format mirrors
// packs: "item[,item...] X Z" so the parser can share splitting logic.
// An empty-loot chest writes as "(empty) X Z" — using the literal name
// "(empty)" keeps the row well-formed (always 3 whitespace-separated
// fields) without inventing a separate "no items" syntax.
type MapChest struct {
	Items []string
	X     int
	Z     int
}

// EmptyChestToken is the single-field placeholder for a chest authored
// with no items. Kept out of the item-name registry so it can never
// shadow a real ItemDefinition.Name; the parser maps it back to an
// empty Items slice and the encoder emits it when Items is empty.
// Exported so core's reserved-item-name guard cites this one source
// instead of re-hardcoding the literal.
const EmptyChestToken = "(empty)"

// MapDoor is one authored door at a tile. On-disk format:
//
//	<name> <target_map> <target_door> <X> <Z> <facing> [style]
//
// Name is this door's identifier (must be unique within the map);
// TargetMap is the destination map id (the bare name, e.g. "dungeon"
// for dungeon.map) or the literal "self" for same-map portals;
// TargetDoor is the matching door's Name in the destination; Facing
// is the post-transition direction the player faces and is one of
// north/east/south/west. Style is the optional visual fixture
// (building/cave/field); when absent it defaults to building so
// older 6-field door rows keep loading unchanged.
type MapDoor struct {
	Name       string
	TargetMap  string
	TargetDoor string
	X          int
	Z          int
	Facing     string
	Style      string
}

// MapCrystal is one authored healing crystal at a tile. On-disk format is
// just the two coordinates, "X Z" — a crystal carries no per-instance data
// (its charge state is runtime-only), so the row needs nothing more.
type MapCrystal struct {
	X int
	Z int
}

// MapFace is one tile's per-direction cliff-face skin override. Skins is indexed
// 0=N,1=E,2=S,3=W; a '.' entry means "use the tile's base skin for that face."
// On disk a face line is "x z NESW" (the 4 skin chars), one per overridden tile.
type MapFace struct {
	X, Z  int
	Skins [4]byte
}

// DoorTargetComplete reports whether a door names both halves of a
// destination. Core door types route through this too so parse-time and
// runtime checks cannot drift.
func DoorTargetComplete(targetMap, targetDoor string) bool {
	return targetMap != "" && targetDoor != ""
}

// HasTarget reports whether this authored door names a complete destination.
func (d MapDoor) HasTarget() bool {
	return DoorTargetComplete(d.TargetMap, d.TargetDoor)
}

// SelfMapToken is the placeholder TargetMap value for same-map
// portals — keeps the row well-formed (always 6 whitespace-separated
// fields) without leaving an ambiguous empty column. The parser
// rewrites it to the map's own name at load time, so runtime door
// resolution doesn't need a special case.
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

// DoorStyleNames is the canonical on-disk order for door-style strings,
// matching core.DoorStyleBuilding/Cave/Field. DoorStyleBuildingName is
// index 0 so an absent style column resolves to it.
var DoorStyleNames = [...]string{
	DoorStyleBuildingName,
	DoorStyleCaveName,
	DoorStyleFieldName,
}

// IsDoorStyleName reports whether s names one of the canonical door
// styles (case-insensitive).
func IsDoorStyleName(s string) bool {
	low := strings.ToLower(s)
	for _, name := range DoorStyleNames {
		if name == low {
			return true
		}
	}
	return false
}

// Ext is the canonical on-disk extension for map files. Lives in
// mapfile (the I/O package) so core can't import it as a string
// literal anywhere — `core.MapPath`, the editor's Save As preview,
// and List/ListByModTime all reference this constant so a future
// rename to ".mapv2" is a one-line change.
const Ext = ".map"

// Ceiling-layer sentinel chars in the on-disk format. CeilingOpenChar
// ('.') means "no ceiling — sky shows through"; CeilingSolidChar ('#')
// is a solid slab. Defined here (the I/O package) so the blank-ceiling
// seeding for older files spells "open" once; core.TileCeilingOpen /
// TileCeilingSolid alias these so the convention can't drift across the
// package boundary.
const (
	CeilingOpenChar  = '.'
	CeilingSolidChar = '#'
)

// ElevationGroundChar is the on-disk sentinel for the lowest ground level
// (level 0) in the optional elevation layer. Blank / absent elevation rows
// seed to this so flat maps read as all level 0. core.ElevationGround aliases
// it so the "flat" convention doesn't drift across the package boundary.
const ElevationGroundChar = '0'

// AssetDirMode / AssetFileMode are the os mode bits for auto-created
// asset directories and files. Defined in this leaf I/O package so Save
// (and other writers) can use them without importing core; core's
// AssetDirMode / AssetFileMode alias these (core imports mapfile, not
// the reverse), so a permissions change is one edit.
const (
	AssetDirMode  = 0o755
	AssetFileMode = 0o644
)

// MapCustomEnemy is one author-defined enemy template in the on-disk
// format. Fields are positional whitespace-separated on a single line:
//
//	<name> <base_kind> <hp> <mp> <str> <dex> <int> <wis> <vit> <spd> <armor> <mdef> <xp> <tier> <dmg> <sklch> <spwr> <skills>
//
// Skills is `-` for none or a comma-separated list of skill names
// ("firebolt,sleep"). BaseKind is the on-disk enemy name ("rat",
// "goblin_mage") whose sprite + flavor strings the custom enemy
// reuses. Loader resolves both name registries via the core package.
type MapCustomEnemy struct {
	Name            string
	BaseKind        string
	HP              int
	MP              int
	STR             int
	DEX             int
	INT             int
	WIS             int
	VIT             int
	SPD             int
	Armor           int
	MDef            int
	XPValue         int
	Tier            int
	AttackDamage    int
	SkillCastChance float64
	SpellPower      int
	Skills          []string
}

// customEnemyNoSkillsToken is the single-field placeholder for a
// custom enemy with no skills. Mirrors EmptyChestToken — keeps the
// row well-formed at customEnemyFieldCount (18) whitespace-separated
// fields without inventing an empty-column syntax.
const customEnemyNoSkillsToken = "-"

// layerSlot is the parser's notion of "which grid is the upcoming N rows
// going into." Lets the section dispatch share one collection loop.
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
	slotCustomEnemies
	slotDialogs
	slotTriggers
	slotSolids
	slotPropLevels
	slotDecorLevels
	slotFaces
)

// Section header names — the on-disk labels that introduce each part of
// a .map file (the header line on disk is the name plus a colon, e.g.
// "walls:"). Referenced by both sectionFor (parse) and Encode (write) so
// the parser and encoder can't drift on a section spelling. Exported so
// core's Area↔MapFile converter (areas.go) cites these constants for its
// own grid-layer dimension diagnostics instead of re-hardcoding the bare
// "walls"/"floor"/… string literals.
const (
	SectionWalls         = "walls"
	SectionFloor         = "floor"
	SectionDecor         = "decor"
	SectionProps         = "props"
	SectionCeiling       = "ceiling"
	SectionElevation     = "elevation"
	SectionEnemies       = "enemies"
	SectionChests        = "chests"
	SectionDoors         = "doors"
	SectionCrystals      = "crystals"
	SectionCustomEnemies = "custom_enemies"
	SectionDialogs       = "dialogs"
	SectionTriggers      = "triggers"
	SectionSolids        = "solids"
	SectionPropLevels    = "prop_levels"
	SectionDecorLevels   = "decor_levels"
	SectionFaces         = "faces"
)

// Header-line keys. The file preamble is "key: value" lines (not "key:" section
// headers), so these live apart from the Section* constants but follow the same
// "one spelling, both sides" rule: parseHeaderLine matches on them and Encode
// writes them, so a rename can't drift the reader and writer out of step.
const (
	headerName      = "name"
	headerMaterials = "materials"
	headerQuiet     = "quiet"
	headerSize      = "size"
	headerStart     = "start"
)

// layerSection describes one .map section: its on-disk name, the parse
// slot it maps to, and (for the six grid layers) a field accessor into
// MapFile. Entity sections (enemies/chests/doors/custom_enemies) carry a
// nil field — they parse into spawn lists, not a grid. This is the single
// source for sectionFor (name→slot) and layerSlice (slot→*[]string) so
// the two can't drift on which sections exist or which own a grid.
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
	{SectionCustomEnemies, slotCustomEnemies, nil},
	{SectionDialogs, slotDialogs, nil},
	{SectionTriggers, slotTriggers, nil},
	// solids: carries a multi-plane voxel stack, not a single grid, so it has
	// a nil field (parsed/encoded by bespoke code like the entity sections) and
	// is excluded from GridLayerCount.
	{SectionSolids, slotSolids, nil},
	// prop_levels: is an OPTIONAL single grid (per-tile prop level) written only
	// when a prop sits above its auto surface; nil field + bespoke encode keeps it
	// from being emitted for every map (which would break byte-stable round-trips).
	{SectionPropLevels, slotPropLevels, nil},
	// decor_levels: same as prop_levels for the decor layer.
	{SectionDecorLevels, slotDecorLevels, nil},
	// faces: is a sparse entity-style section (one line per overridden tile), not
	// a grid, so nil field + bespoke parse/encode.
	{SectionFaces, slotFaces, nil},
}

// GridLayerCount is the number of grid (string-row) layers a .map carries —
// the layerSections rows with a field accessor (walls/floor/decor/props/
// ceiling/elevation), as opposed to the spawn-list sections. Computed in init
// from the table so it can't drift from it. Exported so core can assert its
// own gridLayers() enumeration (and, by extension, the Area↔MapFile converters
// that hand-list these fields) stays in lockstep — a 7th grid layer added on
// either side trips a startup panic instead of silently failing to round-trip.
var GridLayerCount int

// init asserts layerSections covers every real slot (slotWalls..
// slotFaces) exactly once, so a new layerSlot enum value added
// without a table row panics at startup instead of silently parsing as
// slotNone / encoding nothing. It also tallies GridLayerCount.
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
	for slot := slotWalls; slot <= slotFaces; slot++ {
		if !seen[slot] {
			panic(fmt.Sprintf("mapfile: layerSections missing slot %d — add a row", slot))
		}
	}
}

// namedLayer pairs a grid layer's on-disk section name with its rows.
// Shared by validate (dimension checks) and Encode (header + row emit)
// so the layer list lives in one place instead of two near-identical
// struct-slice literals.
type namedLayer struct {
	name string
	rows []string
}

// requiredLayers is the four mandatory grid layers in canonical on-disk
// order. Ceiling is intentionally excluded — it's optional (legacy maps
// omit it) and validated / encoded separately: Encode appends ceiling to
// this list, while validate iterates it directly for the height/width
// checks and handles ceiling on its own afterward.
func (mf MapFile) requiredLayers() []namedLayer {
	return []namedLayer{
		{SectionWalls, mf.Walls},
		{SectionFloor, mf.Floor},
		{SectionDecor, mf.Decor},
		{SectionProps, mf.Props},
	}
}

// Parse reads a .map file from r. Errors pinpoint the first malformed line.
func Parse(r io.Reader) (MapFile, error) {
	mf := MapFile{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	state := slotNone
	lineNo := 0
	for sc.Scan() {
		lineNo++
		raw := strings.TrimRight(sc.Text(), "\r")

		// Section headers can appear at any point and switch state. Always
		// check before treating a line as content.
		if next, ok := sectionFor(raw); ok {
			state = next
			// Remember that a crystals: section was present even if it turns
			// out to hold no rows — an empty section means "zero crystals,"
			// not "unspecified" (see MapFile.CrystalsDefined).
			if next == slotCrystals {
				mf.CrystalsDefined = true
			}
			continue
		}

		// Blank lines are universally skipped — every section parser
		// used to open with the same trim+blank dance. Lifted here so
		// "did I remember to skip blanks?" is no longer a per-section
		// concern. NOT skipping `#`-prefixed lines because the wall
		// glyph IS `#` — a comment convention would collide with
		// content. If a comment syntax becomes needed later, pick a
		// prefix that can't appear at column 0 of any layer row.
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
			// 3 fields = legacy (AI defaults to "none"); 4 = AI column
			// present. Same backward-compat shape doors use for the
			// style column.
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
			mf.Packs = append(mf.Packs, MapPack{Members: members, BackCount: backCount, X: x, Z: z, AI: ai})
			continue
		}

		if state == slotDoors {
			fields := strings.Fields(line)
			// 6 fields = legacy row (style defaults to building); 7 = style
			// column present. Keeping both shapes valid means older maps load
			// untouched and only pick up a style column when re-saved.
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
				Facing:     face,
				Style:      style,
			})
			continue
		}

		if state == slotChests {
			// Chest row: "itemname[,itemname...] X Z" or "(empty) X Z" for
			// a no-loot chest. Item names use the canonical
			// ItemDefinition.Name strings (e.g. "Morsel of Cheese") so the
			// .map file is human-editable without an item-id lookup table
			// in the head.
			fields := strings.Fields(line)
			if len(fields) < chestFieldsMin {
				return mf, fmt.Errorf("line %d: expected '<item[,item...]> <x> <z>' or '(empty) <x> <z>', got %q", lineNo, raw)
			}
			// Item list can contain whitespace inside individual names
			// ("Morsel of Cheese"), so reassemble by taking the LAST two
			// fields as X/Z and the rest as a single item-list token.
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
			mf.Chests = append(mf.Chests, MapChest{Items: items, X: x, Z: z})
			continue
		}

		if state == slotCrystals {
			// Crystal row: "X Z" — position only (charge state is runtime).
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
			mf.Crystals = append(mf.Crystals, MapCrystal{X: x, Z: z})
			continue
		}

		if state == slotCustomEnemies {
			ce, err := parseCustomEnemyLine(line, lineNo)
			if err != nil {
				return mf, err
			}
			mf.CustomEnemies = append(mf.CustomEnemies, ce)
			continue
		}

		if state == slotDialogs {
			// One opaque JSON object per line — stored verbatim. mapfile does
			// not parse the JSON (core does on the way to DialogDefinition);
			// it only preserves the line so the section round-trips. A JSON
			// object ends with '}', so it can't be mistaken for a section
			// header (which must end with ':').
			mf.Dialogs = append(mf.Dialogs, line)
			continue
		}

		if state == slotTriggers {
			// Opaque JSON-per-line, identical handling to dialogs above (core
			// marshals DialogTrigger values; mapfile only preserves the lines).
			mf.Triggers = append(mf.Triggers, line)
			continue
		}

		if state == slotSolids {
			// The voxel stack is N planes of Height rows each, lowest level
			// first. Blank separator lines were already skipped above, so rows
			// arrive contiguously — start a new plane whenever the current one
			// has filled to Height. Height comes from size:, which the format
			// requires before any grid; if it's still 0 here the header is
			// misordered — fail with a pointed message rather than splitting
			// every row into its own plane and erroring obscurely in validate().
			if mf.Height == 0 {
				return mf, fmt.Errorf("line %d: solids: section appears before size: (grid dimensions unknown)", lineNo)
			}
			if len(mf.Solids) == 0 || len(mf.Solids[len(mf.Solids)-1]) >= mf.Height {
				mf.Solids = append(mf.Solids, []string{})
			}
			last := len(mf.Solids) - 1
			mf.Solids[last] = append(mf.Solids[last], raw)
			continue
		}

		if state == slotPropLevels {
			// A single Height-row grid of per-tile prop levels (base-36 char, or
			// '.' = auto). Same row-collection shape as the other grids.
			mf.PropLevels = append(mf.PropLevels, raw)
			continue
		}

		if state == slotDecorLevels {
			mf.DecorLevels = append(mf.DecorLevels, raw)
			continue
		}

		if state == slotFaces {
			// One overridden tile per line: "x z NESW" (the 4 face skin chars,
			// '.' = use the tile's base skin for that face). A malformed line is a
			// loud error (like every other entity section) rather than a silent
			// drop — a hand-edited typo here must not vanish without a diagnostic.
			fields := strings.Fields(raw)
			if len(fields) < facesFieldCount || len(fields[2]) != faceSkinCount {
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
			var sk [4]byte
			copy(sk[:], fields[2])
			mf.Faces = append(mf.Faces, MapFace{X: fx, Z: fz, Skins: sk})
			continue
		}

		// Layer grid line. Once Height rows are collected, blank lines are
		// tolerated (some editors auto-insert one before the next section
		// header) but a non-blank overflow row is a structural error — the
		// validator would catch it later, but reporting it on the offending
		// line gives a better diagnostic. (The format is headers-first:
		// `size:` precedes every grid, so Height is always set here. A
		// hand-edited file that puts `size:` after a grid is malformed — the
		// size line is then read as a grid row and validate() rejects the
		// 0x0 dims, which is correct.)
		target := layerSlice(&mf, state)
		if target == nil {
			// Reaching here means `state` is a section slot that is neither a grid
			// layer (field != nil) nor handled by a bespoke arm above — i.e. a new
			// layerSlot was added to layerSections (passing the init coverage
			// assert) without a parse handler. Fail loudly rather than silently
			// dropping every line of that section. Unreachable for any valid map.
			panic(fmt.Sprintf("mapfile: section slot %d has no parse handler — add a bespoke arm or a grid field accessor", state))
		}
		if len(*target) >= mf.Height {
			if strings.TrimSpace(raw) == "" {
				continue
			}
			return mf, fmt.Errorf("line %d: extra row past declared height %d", lineNo, mf.Height)
		}
		*target = append(*target, raw)
	}
	if err := sc.Err(); err != nil {
		return mf, err
	}
	if err := mf.validate(); err != nil {
		return mf, err
	}
	return mf, nil
}

// sectionFor maps a section-header line ("walls:", "doors:", …) to its
// slot. The trailing colon is required (a bare "walls" is not a header),
// matching the original switch's exact-match behavior.
func sectionFor(raw string) (layerSlot, bool) {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasSuffix(trimmed, ":") {
		return slotNone, false
	}
	name := trimmed[:len(trimmed)-1]
	for _, s := range layerSections {
		if s.name == name {
			return s.slot, true
		}
	}
	return slotNone, false
}

// layerSlice returns the MapFile grid field for a grid-layer slot, or nil
// for entity sections / unknown slots (matching the original switch).
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

// validateOptionalGrid dimension-checks an optional single-grid layer
// (prop_levels / decor_levels): absent (0 rows) is fine, but a present grid must
// be exactly Height×Width so a ragged plane can't reach the renderer/collision.
// Shared by the two near-identical optional-level-grid checks.
func (mf *MapFile) validateOptionalGrid(name string, rows []string) error {
	n := len(rows)
	if n == 0 {
		return nil
	}
	if n != mf.Height {
		return fmt.Errorf("%s has %d rows, size declares %d", name, n, mf.Height)
	}
	for i, row := range rows {
		if len(row) != mf.Width {
			return fmt.Errorf("%s row %d has %d cols, size declares %d", name, i, len(row), mf.Width)
		}
	}
	return nil
}

func (mf *MapFile) validate() error {
	if mf.Width <= 0 || mf.Height <= 0 {
		return fmt.Errorf("size must be >0x0; got %dx%d", mf.Width, mf.Height)
	}
	for _, layer := range mf.requiredLayers() {
		if len(layer.rows) != mf.Height {
			return fmt.Errorf("%s layer has %d rows, size declares %d", layer.name, len(layer.rows), mf.Height)
		}
		for i, row := range layer.rows {
			if len(row) != mf.Width {
				return fmt.Errorf("%s layer row %d has %d cols, size declares %d", layer.name, i, len(row), mf.Width)
			}
		}
	}
	// Ceiling is optional for legacy .map files. Missing → fill with a
	// blank "no ceiling" layer so downstream code (renderer, editor) can
	// always index it like the other four. A partial ceiling layer (some
	// rows missing) is treated as malformed because it almost certainly
	// indicates an authoring mistake, not an older format.
	switch len(mf.Ceiling) {
	case 0:
		mf.Ceiling = BlankLayer(mf.Width, mf.Height, CeilingOpenChar)
	case mf.Height:
		for i, row := range mf.Ceiling {
			if len(row) != mf.Width {
				return fmt.Errorf("ceiling layer row %d has %d cols, size declares %d", i, len(row), mf.Width)
			}
		}
	default:
		return fmt.Errorf("ceiling layer has %d rows, size declares %d", len(mf.Ceiling), mf.Height)
	}
	// Elevation is optional too (same legacy rule as ceiling): missing → blank
	// all-'0' (flat) layer; full height → validate widths; partial → malformed.
	switch len(mf.Elevation) {
	case 0:
		mf.Elevation = BlankLayer(mf.Width, mf.Height, ElevationGroundChar)
	case mf.Height:
		for i, row := range mf.Elevation {
			if len(row) != mf.Width {
				return fmt.Errorf("elevation layer row %d has %d cols, size declares %d", i, len(row), mf.Width)
			}
			// Every elevation cell must be a level char — '0'..'9' for levels
			// 0..9, then 'A'..'K' for 10..20 (base-36, one char per cell). The
			// upper bound is 'K' because core caps the encodable level at 20
			// (MaxElevationLevel); chars past 'K' are unreachable through the
			// encoder. ElevationLevelAt reads anything else as ground level 0,
			// so without this a stray char (hand-edit, wrong layer pasted in)
			// loads as flat ground and silently desyncs the intended cliff /
			// ramp geometry.
			for c := 0; c < len(row); c++ {
				if b := row[c]; !((b >= '0' && b <= '9') || (b >= 'A' && b <= 'K')) {
					return fmt.Errorf("elevation layer row %d col %d has bad level char %q (expected '0'..'9' or 'A'..'K')", i, c, string(row[c]))
				}
			}
		}
	default:
		return fmt.Errorf("elevation layer has %d rows, size declares %d", len(mf.Elevation), mf.Height)
	}
	// solids: is the optional voxel stack. Each plane is a full Height×Width
	// grid; planes stack lowest-level-first. Cell chars aren't constrained here
	// (the walls/face-skin layer isn't char-validated either — that alphabet
	// lives in core); only dimensions are checked, which is what protects the
	// renderer/movement from a ragged plane.
	for L, plane := range mf.Solids {
		if len(plane) != mf.Height {
			return fmt.Errorf("solids plane %d has %d rows, size declares %d", L, len(plane), mf.Height)
		}
		for i, row := range plane {
			if len(row) != mf.Width {
				return fmt.Errorf("solids plane %d row %d has %d cols, size declares %d", L, i, len(row), mf.Width)
			}
		}
	}
	// prop_levels / decor_levels: optional per-tile level grids; dimension-check
	// only (the char alphabet — base-36 level or '.' auto — is core's concern),
	// same as the other grids, so a ragged plane can't reach the renderer/collision.
	if err := mf.validateOptionalGrid(SectionPropLevels, mf.PropLevels); err != nil {
		return err
	}
	if err := mf.validateOptionalGrid(SectionDecorLevels, mf.DecorLevels); err != nil {
		return err
	}
	// faces: sparse per-tile overrides — bounds-check each so a stray line can't
	// feed an off-map index to the renderer.
	for _, f := range mf.Faces {
		if f.X < 0 || f.X >= mf.Width || f.Z < 0 || f.Z >= mf.Height {
			return fmt.Errorf("faces entry (%d,%d) outside map", f.X, f.Z)
		}
	}
	if mf.StartX < 0 || mf.StartX >= mf.Width || mf.StartZ < 0 || mf.StartZ >= mf.Height {
		return fmt.Errorf("start (%d,%d) outside map", mf.StartX, mf.StartZ)
	}
	// StartFace must be a canonical facing — mirrors the per-door guard below.
	// Without this, a MapFile built directly (bypassing Parse, which validates
	// the token) with an empty/garbage StartFace passes validate(), and Save
	// then writes a "start: X Z <bad>" line that Parse rejects on reload — a
	// silent Save/Parse asymmetry.
	if !IsFacingName(mf.StartFace) {
		return fmt.Errorf("start facing %q invalid", mf.StartFace)
	}
	// Pack and chest spawns are validated against bounds here rather
	// than in the parser so a single-row malformed entry surfaces with
	// the same "outside map" diagnostic as a malformed start. Without
	// this guard, an authoring typo silently survives parse and the
	// runtime placePacks / placeChests just skips the entry — a
	// frustrating "where did my pack go?" debug.
	for _, p := range mf.Packs {
		if p.X < 0 || p.X >= mf.Width || p.Z < 0 || p.Z >= mf.Height {
			return fmt.Errorf("pack at (%d,%d) outside map %dx%d", p.X, p.Z, mf.Width, mf.Height)
		}
	}
	for _, c := range mf.Chests {
		if c.X < 0 || c.X >= mf.Width || c.Z < 0 || c.Z >= mf.Height {
			return fmt.Errorf("chest at (%d,%d) outside map %dx%d", c.X, c.Z, mf.Width, mf.Height)
		}
	}
	// Crystals: same bounds guard as packs / chests so an out-of-range
	// hand-edit surfaces here at load rather than as a silently dropped
	// crystal at runtime.
	for _, c := range mf.Crystals {
		if c.X < 0 || c.X >= mf.Width || c.Z < 0 || c.Z >= mf.Height {
			return fmt.Errorf("crystal at (%d,%d) outside map %dx%d", c.X, c.Z, mf.Width, mf.Height)
		}
	}
	// Doors: validate bounds, non-empty name, non-empty target. Same
	// philosophy as packs / chests — a hand-edit typo surfaces here
	// instead of producing a silent "step on door, nothing happens"
	// runtime mystery. Duplicate names within the same map are also
	// rejected since runtime resolution by name would be ambiguous.
	seenNames := make(map[string]struct{}, len(mf.Doors))
	for _, d := range mf.Doors {
		if d.X < 0 || d.X >= mf.Width || d.Z < 0 || d.Z >= mf.Height {
			return fmt.Errorf("door %q at (%d,%d) outside map %dx%d", d.Name, d.X, d.Z, mf.Width, mf.Height)
		}
		if d.Name == "" {
			return fmt.Errorf("door at (%d,%d) has empty name", d.X, d.Z)
		}
		// The door row is space-delimited with three variable-width
		// leading fields (name, target_map, target_door), so whitespace
		// in any of them is unrecoverable on re-parse — Fields() would
		// split it into the wrong column count. Reject at the data-model
		// boundary so a spaced name fails loudly at save (see Save) rather
		// than silently producing a .map the parser later rejects.
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
		// Facing / style must match what the parser accepts on reload — Encode
		// writes Facing verbatim (and defaults an empty Style to building), so
		// an out-of-vocabulary value would Save fine and then fail Parse. Reject
		// it here at the data-model boundary, same philosophy as the asymmetric-
		// target guard in Encode (an empty Style is legal — the encoder fills it).
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

// IsFacingName reports whether s is one of the four canonical facing
// strings.
func IsFacingName(s string) bool {
	for _, name := range FacingNames {
		if s == name {
			return true
		}
	}
	return false
}

// parseIntField parses a numeric field with the canonical "line N:
// bad <name> %q" error wrap that the entity-row decoders (packs, doors,
// chests, crystals, custom enemy) inline. Several near-identical
// `strconv.Atoi` + `fmt.Errorf("line %d: bad %s %q", ...)` blocks
// collapse into one helper. The header rows (parseSize / parseStart)
// deliberately keep their own validation — they check shape (WxH) and a
// facing name, not just an integer, and their callers add the line wrap.
func parseIntField(s, name string, lineNo int) (int, error) {
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("line %d: bad %s %q", lineNo, name, s)
	}
	return v, nil
}

// Positional field counts for the non-custom-enemy entity sections.
// Named (rather than inlined as bare integer literals in the parse
// guards) so the parse-time width check and the matching encode-format
// verb count cite one source — mirrors the customEnemyFieldCount /
// customEnemyFieldCountLegacy pattern below. Sections with a backward-
// compatible optional trailing column carry both a Legacy width (column
// absent) and a current width (column present); the parser accepts both.
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

// faceSkinCount is the number of per-direction skin chars in a faces row's NESW
// token (N,E,S,W), which equals len(MapFace.Skins). Named so the faces parse
// width guard cites the array size rather than a bare 4.
const faceSkinCount = len(MapFace{}.Skins)

// Per-section encode format strings. Each is the fmt.Fprintf format the
// encoder writes one row per; broken out as named constants so init()
// can count their `%`-verbs and assert they stay in lockstep with the
// field-count constants above — same guard the customEnemyEncodeFormat
// assert provides, so a verb added/removed without bumping the count (or
// vice versa) panics at startup instead of writing a row the parser then
// rejects on the next load. packEncodeFormat / doorEncodeFormat cover the
// current (trailing-column-present) widths; the legacy shorter rows are
// emitted via their own narrower formats below.
const (
	// packFieldsLegacy verbs (default-AI packs) / packFields verbs (non-default AI).
	packEncodeFormatLegacy = "%s %d %d\n"
	packEncodeFormat       = "%s %d %d %s\n"
	// chestFieldsMin verbs.
	chestEncodeFormat = "%s %d %d\n"
	// doorFields verbs (style is always written, so the legacy 6-field row
	// is never emitted — older maps pick up the style column on re-save).
	doorEncodeFormat = "%s %s %s %d %d %s %s\n"
	// crystalFields verbs.
	crystalEncodeFormat = "%d %d\n"
	// facesFieldCount verbs ("X Z NESW"). The NESW token is one %s field of
	// faceSkinCount chars, so this is 3 verbs, not 3+faceSkinCount.
	facesEncodeFormat = "%d %d %s\n"
)

// customEnemyFieldCount is the positional column count for a current-
// schema custom-enemy row (MDef column included). Older maps written
// before MDef shipped use customEnemyFieldCountLegacy below — the
// parser accepts both widths and defaults MDef to 0 on the legacy
// path so existing maps load unchanged. Bumping the schema again is a
// matching parse/encode pair plus a legacy-width fallback if needed.
const (
	customEnemyFieldCount       = 18
	customEnemyFieldCountLegacy = 17
)

// customEnemySchema maps the post-stats positional columns of a custom-
// enemy row to their field index. Two layouts exist: the current schema
// (MDef column present) and the legacy pre-MDef schema (MDef absent, so
// mdef = -1 and every later column shifts left by one). Holding the
// layout in one struct keeps the legacy/current split in a single place
// instead of eight parallel index reassignments inside the decoder.
type customEnemySchema struct {
	armor  int
	mdef   int // -1 when the row predates the MDef column
	xp     int
	tier   int
	dmg    int
	skch   int
	spwr   int
	skills int
}

var (
	customEnemyCurrentSchema = customEnemySchema{armor: 10, mdef: 11, xp: 12, tier: 13, dmg: 14, skch: 15, spwr: 16, skills: 17}
	customEnemyLegacySchema  = customEnemySchema{armor: 10, mdef: -1, xp: 11, tier: 12, dmg: 13, skch: 14, spwr: 15, skills: 16}
)

// customEnemyEncodeFormat is the fmt.Fprintf format string the
// encoder writes one row per. Kept as a named constant so init() can
// count its `%`-verbs and assert the encoder and customEnemyFieldCount
// stay in lockstep — a future schema bump that touches the format
// string without bumping the count (or vice versa) panics at startup
// instead of producing a row the decoder rejects on the next load.
const customEnemyEncodeFormat = "%s %s %d %d %d %d %d %d %d %d %d %d %d %d %d %g %d %s\n"

// fprintfVerbCount counts the `%`-verbs in a fmt format string (each
// consumes one argument). A literal `%%` is skipped, not counted. Shared
// by every encode-format ↔ field-count assert below so the "does this
// format emit the right number of columns" check lives in one place.
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

func init() {
	if verbs := fprintfVerbCount(customEnemyEncodeFormat); verbs != customEnemyFieldCount {
		panic(fmt.Sprintf("mapfile: customEnemyEncodeFormat has %d verbs, customEnemyFieldCount is %d — they must match", verbs, customEnemyFieldCount))
	}
	// `skills` is the final positional column, so its index must be the
	// last slot of the row width. The verb-count check above only guards
	// the encoder; this guards the decoder's index table against drifting
	// from the field count (a schema index bumped without the count, or
	// vice versa, would silently mis-slice a parse).
	if customEnemyCurrentSchema.skills != customEnemyFieldCount-1 {
		panic(fmt.Sprintf("mapfile: customEnemyCurrentSchema.skills is %d, expected customEnemyFieldCount-1 (%d)", customEnemyCurrentSchema.skills, customEnemyFieldCount-1))
	}
	if customEnemyLegacySchema.skills != customEnemyFieldCountLegacy-1 {
		panic(fmt.Sprintf("mapfile: customEnemyLegacySchema.skills is %d, expected customEnemyFieldCountLegacy-1 (%d)", customEnemyLegacySchema.skills, customEnemyFieldCountLegacy-1))
	}
}

// init asserts every per-section encode format's `%`-verb count matches the
// field-count constant the parser guards that section against. Same lockstep
// guarantee customEnemyEncodeFormat gets: a verb added/removed without bumping
// the matching field-count constant (or vice versa) panics at startup instead
// of writing a row the parser rejects on the next load. (chestEncodeFormat is
// checked against chestFieldsMin since a chest's item token may itself contain
// spaces — the format always emits exactly chestFieldsMin whitespace groups.)
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

// parseCustomEnemyLine decodes a single positional row from the
// `custom_enemies:` section. Field order documented on MapCustomEnemy.
// Returns the wrap-style "line N: bad <field> %q" error every row
// decoder uses so the error report stays uniform. Accepts the legacy
// 17-field width too (pre-MDef) so older maps round-trip.
func parseCustomEnemyLine(line string, lineNo int) (MapCustomEnemy, error) {
	fields := strings.Fields(line)
	legacy := len(fields) == customEnemyFieldCountLegacy
	if !legacy && len(fields) != customEnemyFieldCount {
		return MapCustomEnemy{}, fmt.Errorf("line %d: custom enemy expects %d fields, got %d", lineNo, customEnemyFieldCount, len(fields))
	}
	ce := MapCustomEnemy{
		Name:     fields[0],
		BaseKind: fields[1],
	}
	// Column layout depends on the legacy/current schema split. MDef is
	// inserted between Armor and XPValue in the current schema; the
	// legacy layout omits it (mdef = -1) and shifts everything past it
	// left by one.
	schema := customEnemyCurrentSchema
	if legacy {
		schema = customEnemyLegacySchema
	}
	intFields := []struct {
		dst  *int
		raw  string
		name string
	}{
		{&ce.HP, fields[2], "hp"},
		{&ce.MP, fields[3], "mp"},
		{&ce.STR, fields[4], "str"},
		{&ce.DEX, fields[5], "dex"},
		{&ce.INT, fields[6], "int"},
		{&ce.WIS, fields[7], "wis"},
		{&ce.VIT, fields[8], "vit"},
		{&ce.SPD, fields[9], "spd"},
		{&ce.Armor, fields[schema.armor], "armor"},
		{&ce.XPValue, fields[schema.xp], "xp"},
		{&ce.Tier, fields[schema.tier], "tier"},
		{&ce.AttackDamage, fields[schema.dmg], "dmg"},
		{&ce.SpellPower, fields[schema.spwr], "spwr"},
	}
	if schema.mdef >= 0 {
		intFields = append(intFields, struct {
			dst  *int
			raw  string
			name string
		}{&ce.MDef, fields[schema.mdef], "mdef"})
	}
	for _, f := range intFields {
		v, err := parseIntField(f.raw, "custom enemy "+f.name, lineNo)
		if err != nil {
			return MapCustomEnemy{}, err
		}
		// Every numeric custom-enemy field is logically non-negative (HP /
		// stats / armor / xp / tier / damage / spell power / mdef). Rejecting
		// a negative is correct on its own AND surfaces the most common
		// symptom of a hand-edited wrong-width row mis-sliced under the
		// legacy/current column split (a value landing in the wrong column),
		// turning silent stat corruption into a load error.
		if v < 0 {
			return MapCustomEnemy{}, fmt.Errorf("line %d: custom enemy %s cannot be negative (%d) — check the column count", lineNo, f.name, v)
		}
		*f.dst = v
	}
	chance, err := strconv.ParseFloat(fields[schema.skch], 64)
	if err != nil {
		return MapCustomEnemy{}, fmt.Errorf("line %d: bad custom enemy sklch %q", lineNo, fields[schema.skch])
	}
	// Skill-cast chance is a probability in [0,1]. Bounding both ends (not
	// just negatives) also hardens the legacy/current width split: a row that
	// drops one field parses under the other schema and shifts an integer stat
	// (HP / damage / armor — almost always >1) into this column, so the >1
	// reject turns that silent mis-slice into a load error.
	if chance < 0 || chance > 1 {
		return MapCustomEnemy{}, fmt.Errorf("line %d: custom enemy skill-cast chance must be within [0,1] (%g) — check the column count", lineNo, chance)
	}
	ce.SkillCastChance = chance
	if fields[schema.skills] != customEnemyNoSkillsToken {
		for _, name := range strings.Split(fields[schema.skills], ",") {
			name = strings.TrimSpace(name)
			if name == "" {
				return MapCustomEnemy{}, fmt.Errorf("line %d: empty custom enemy skill entry", lineNo)
			}
			ce.Skills = append(ce.Skills, name)
		}
	}
	return ce, nil
}

// writeVerbatimSection emits an optional section as a "name:" header followed by
// each row written verbatim — but ONLY when rows is non-empty, so a map without
// the section stays byte-identical (the same backward-compat rule solids/doors/
// crystals follow). Shared by the flat string-row sections (prop_levels,
// decor_levels, dialogs, triggers); solids keeps its own nested plane loop.
func writeVerbatimSection(bw *bufio.Writer, name string, rows []string) {
	if len(rows) == 0 {
		return
	}
	fmt.Fprintln(bw, name+":")
	for _, row := range rows {
		fmt.Fprintln(bw, row)
	}
}

// Encode writes mf in the canonical .map format. Layers are emitted in a
// fixed order so encoded maps diff cleanly across edits.
func (mf MapFile) Encode(w io.Writer) error {
	bw := bufio.NewWriter(w)
	fmt.Fprintf(bw, headerName+": %s\n", mf.Name)
	fmt.Fprintf(bw, headerMaterials+": %s\n", mf.Materials)
	fmt.Fprintf(bw, headerQuiet+": %s\n", mf.Quiet)
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
	// solids: appended only for a gapped map (a pure heightfield omits it and
	// stays byte-identical, like doors / crystals). Planes emit lowest-level
	// first as contiguous Height-row blocks; the parser re-splits by Height, so
	// no separator is needed.
	if len(mf.Solids) > 0 {
		fmt.Fprintln(bw, SectionSolids+":")
		for _, plane := range mf.Solids {
			for _, row := range plane {
				fmt.Fprintln(bw, row)
			}
		}
	}
	// prop_levels / decor_levels: appended only when some prop/decor sits above
	// its auto surface (a decked entity); a map whose entities all rest on the
	// ground omits them and stays byte-identical, like solids:.
	writeVerbatimSection(bw, SectionPropLevels, mf.PropLevels)
	writeVerbatimSection(bw, SectionDecorLevels, mf.DecorLevels)
	// faces: one line per overridden tile ("x z NESW"); omitted entirely when no
	// tile overrides a face, so base-skin maps stay byte-identical.
	if len(mf.Faces) > 0 {
		fmt.Fprintln(bw, SectionFaces+":")
		for _, f := range mf.Faces {
			fmt.Fprintf(bw, facesEncodeFormat, f.X, f.Z, string(f.Skins[:]))
		}
	}
	fmt.Fprintln(bw, SectionEnemies+":")
	for _, p := range mf.Packs {
		// Single-member packs encode the same as the legacy "kind X Z" line
		// so maps without grouped packs stay byte-identical across the
		// format change. The AI column is appended only when non-default
		// (anything other than "none" / empty) so default-stationary
		// packs round-trip to the same 3-field shape.
		members := encodePackMembers(p.Members, p.BackCount)
		ai := strings.ToLower(strings.TrimSpace(p.AI))
		if ai == "" || ai == PackAINoneName {
			fmt.Fprintf(bw, packEncodeFormatLegacy, members, p.X, p.Z)
		} else {
			fmt.Fprintf(bw, packEncodeFormat, members, p.X, p.Z, ai)
		}
	}
	fmt.Fprintln(bw, SectionChests+":")
	for _, c := range mf.Chests {
		token := EmptyChestToken
		if len(c.Items) > 0 {
			token = strings.Join(c.Items, ",")
		}
		fmt.Fprintf(bw, chestEncodeFormat, token, c.X, c.Z)
	}
	// doors: section is appended only when present. Older .map files
	// without any doors stay byte-identical across the format change —
	// the parser treats a missing section as zero-doors. Same rule as
	// the pre-ceiling-section backwards compatibility above.
	if len(mf.Doors) > 0 {
		fmt.Fprintln(bw, SectionDoors+":")
		for _, d := range mf.Doors {
			// Refuse to write a half-populated door (one of
			// TargetMap/TargetDoor set, the other empty). The
			// encoder would emit a 5-field row that the parser
			// rejects on the next load — fail here at the save
			// boundary with a useful message so the editor can
			// surface the broken authoring before it's committed
			// to disk. MapDoor.HasTarget encodes the "both set"
			// rule, and an unauthored door (both empty) is also
			// legal — only the asymmetric case is rejected.
			haveMap := d.TargetMap != ""
			haveDoor := d.TargetDoor != ""
			if haveMap != haveDoor {
				return fmt.Errorf("door %q has asymmetric target (map=%q, door=%q); both must be set or both empty", d.Name, d.TargetMap, d.TargetDoor)
			}
			style := d.Style
			if style == "" {
				style = DoorStyleBuildingName
			}
			fmt.Fprintf(bw, doorEncodeFormat, d.Name, d.TargetMap, d.TargetDoor, d.X, d.Z, d.Facing, style)
		}
	}
	// crystals: emits when the map defines crystals at all — either it has
	// rows OR it was explicitly marked defined (an authored zero-crystal map).
	// A legacy map that never carried the section (CrystalsDefined false, no
	// rows) stays byte-identical, same backward-compat rule as doors. Rows are
	// position-only; charge state lives in SaveData, not the map.
	if mf.CrystalsDefined || len(mf.Crystals) > 0 {
		fmt.Fprintln(bw, SectionCrystals+":")
		for _, c := range mf.Crystals {
			fmt.Fprintf(bw, crystalEncodeFormat, c.X, c.Z)
		}
	}
	// custom_enemies: emits only when present so older maps stay
	// byte-identical. Order documented on MapCustomEnemy and matches
	// parseCustomEnemyLine's positional decode. The format string
	// is broken out as customEnemyEncodeFormat so init() can assert
	// its `%`-verb count matches customEnemyFieldCount — keeps the
	// encoder and decoder honest about how many columns a row has.
	if len(mf.CustomEnemies) > 0 {
		fmt.Fprintln(bw, SectionCustomEnemies+":")
		for _, ce := range mf.CustomEnemies {
			skills := customEnemyNoSkillsToken
			if len(ce.Skills) > 0 {
				skills = strings.Join(ce.Skills, ",")
			}
			fmt.Fprintf(bw, customEnemyEncodeFormat,
				ce.Name, ce.BaseKind,
				ce.HP, ce.MP,
				ce.STR, ce.DEX, ce.INT, ce.WIS, ce.VIT, ce.SPD,
				ce.Armor, ce.MDef, ce.XPValue, ce.Tier, ce.AttackDamage,
				ce.SkillCastChance, ce.SpellPower,
				skills,
			)
		}
	}
	// dialogs: emits only when present so older maps stay byte-identical. Each
	// entry is a pre-encoded JSON object written verbatim (core owns the
	// marshalling), one per line. triggers: same byte-stable rule.
	writeVerbatimSection(bw, SectionDialogs, mf.Dialogs)
	writeVerbatimSection(bw, SectionTriggers, mf.Triggers)
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

func Save(path string, mf MapFile) error {
	// Validate BEFORE touching disk. os.Create truncates, so a structurally
	// invalid map (out-of-bounds entity, empty/duplicate/whitespace door
	// name, …) must be rejected here — otherwise we'd both write a .map the
	// parser later refuses to load AND truncate the prior good file on the
	// way to that failure. This is the same check Parse runs on load, so a
	// saved map is always re-loadable.
	if err := mf.validate(); err != nil {
		return fmt.Errorf("refusing to save invalid map %q: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), AssetDirMode); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	// Capture the close error too — a deferred f.Close() on the return path
	// would swallow flush failures (network drive, quota, cross-device) and
	// the editor's "Saved successfully" toast would lie. Prefer the encode
	// error if both fire, since that's the root cause.
	err = mf.Encode(f)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// List returns the .map files in dir, sorted alphabetically. Missing dir
// returns an empty slice (first-run convenience).
func List(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(e.Name()), Ext) {
			continue
		}
		out = append(out, filepath.Join(dir, e.Name()))
	}
	sort.Strings(out)
	return out, nil
}

// ListByModTime returns the .map files in dir, newest-modified first.
// Used by the editor's Open modal so the file the author was just working
// on lands at the top of the list. Stat failures on individual entries
// drop those entries from the result rather than killing the whole list.
func ListByModTime(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	type entry struct {
		path string
		mod  int64
	}
	rows := make([]entry, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(e.Name()), Ext) {
			continue
		}
		path := filepath.Join(dir, e.Name())
		info, ierr := e.Info()
		if ierr != nil {
			continue
		}
		rows = append(rows, entry{path: path, mod: info.ModTime().UnixNano()})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].mod > rows[j].mod })
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.path
	}
	return out, nil
}

// BlankLayer returns a Width × Height grid filled with `c`. Editor uses it
// to seed fresh layers when creating a new map or resizing an existing one.
func BlankLayer(width, height int, c byte) []string {
	rows := make([]string, height)
	row := strings.Repeat(string(c), width)
	for i := range rows {
		rows[i] = row
	}
	return rows
}

// OptionalLayerOrBlank returns rows unchanged when present, else a blank
// (width × height, all-`c`) layer. The single "absent optional layer ⇒ blank"
// rule shared by Encode and the Area↔MapFile converters in core/areas.go, so
// the ceiling/elevation default isn't open-coded at each. (validate keeps its
// own switch because it ALSO width-validates a present layer.)
func OptionalLayerOrBlank(rows []string, width, height int, c byte) []string {
	if len(rows) == 0 {
		return BlankLayer(width, height, c)
	}
	return rows
}

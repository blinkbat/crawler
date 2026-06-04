// Package mapfile is the on-disk representation of an explorable area. The
// format is plain text, multi-section: header lines, then one ASCII grid
// per editing layer (walls / floor / decor / props), then a list of enemy
// spawns. Chosen so a map diffs cleanly in git, can be glanced at in any
// editor without parsing, and gives the editor's layer system a 1:1
// mapping with on-disk structure.
//
// Layer character conventions:
//
//	walls  : '.' open, '#' wall
//	floor  : '.' auto-variant (per-tile hash), 'g' grass, 'd' dirt,
//	         'k' dark grass, 's' stone, 'c' cobblestone path, 'w' planks,
//	         '~' shallow water, 'W' deep water (blocks), 'n' sand, 'i' snow
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
	Packs   []MapPack
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
}

// MapPack is one authored pack at a tile. Members is a non-empty list of
// enemy-kind names; on-disk format is "kind[,kind...] X Z [ai]" — three
// fields stay the legacy form (AI defaults to "none"), an optional fourth
// AI field names a non-default movement style.
type MapPack struct {
	Members []string
	X       int
	Z       int
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
// here MUST match core's PackAI iota (core/areas.go's packAINameTable
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

// emptyChestToken is the single-field placeholder for a chest authored
// with no items. Kept out of the item-name registry so it can never
// shadow a real ItemDefinition.Name; the parser maps it back to an
// empty Items slice and the encoder emits it when Items is empty.
const emptyChestToken = "(empty)"

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
//	<name> <base_kind> <hp> <mp> <str> <dex> <int> <wis> <vit> <spd> <armor> <xp> <tier> <dmg> <sklch> <spwr> <skills>
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
// custom enemy with no skills. Mirrors emptyChestToken — keeps the
// row well-formed at 17 whitespace-separated fields without inventing
// an empty-column syntax.
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
	slotEnemies
	slotChests
	slotDoors
	slotCustomEnemies
)

// Section header names — the on-disk labels that introduce each part of
// a .map file (the header line on disk is the name plus a colon, e.g.
// "walls:"). Referenced by both sectionFor (parse) and Encode (write) so
// the parser and encoder can't drift on a section spelling.
const (
	sectionWalls         = "walls"
	sectionFloor         = "floor"
	sectionDecor         = "decor"
	sectionProps         = "props"
	sectionCeiling       = "ceiling"
	sectionEnemies       = "enemies"
	sectionChests        = "chests"
	sectionDoors         = "doors"
	sectionCustomEnemies = "custom_enemies"
)

// layerSection describes one .map section: its on-disk name, the parse
// slot it maps to, and (for the five grid layers) a field accessor into
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
	{sectionWalls, slotWalls, func(mf *MapFile) *[]string { return &mf.Walls }},
	{sectionFloor, slotFloor, func(mf *MapFile) *[]string { return &mf.Floor }},
	{sectionDecor, slotDecor, func(mf *MapFile) *[]string { return &mf.Decor }},
	{sectionProps, slotProps, func(mf *MapFile) *[]string { return &mf.Props }},
	{sectionCeiling, slotCeiling, func(mf *MapFile) *[]string { return &mf.Ceiling }},
	{sectionEnemies, slotEnemies, nil},
	{sectionChests, slotChests, nil},
	{sectionDoors, slotDoors, nil},
	{sectionCustomEnemies, slotCustomEnemies, nil},
}

// init asserts layerSections covers every real slot (slotWalls..
// slotCustomEnemies) exactly once, so a new layerSlot enum value added
// without a table row panics at startup instead of silently parsing as
// slotNone / encoding nothing.
func init() {
	seen := make(map[layerSlot]bool, len(layerSections))
	for _, s := range layerSections {
		if seen[s.slot] {
			panic(fmt.Sprintf("mapfile: duplicate layerSections slot %d", s.slot))
		}
		seen[s.slot] = true
	}
	for slot := slotWalls; slot <= slotCustomEnemies; slot++ {
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
		{sectionWalls, mf.Walls},
		{sectionFloor, mf.Floor},
		{sectionDecor, mf.Decor},
		{sectionProps, mf.Props},
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
			if len(fields) != 3 && len(fields) != 4 {
				return mf, fmt.Errorf("line %d: expected '<kind[,kind...]> <x> <z> [ai]', got %q", lineNo, raw)
			}
			members := strings.Split(fields[0], ",")
			for i, m := range members {
				m = strings.TrimSpace(m)
				if m == "" {
					return mf, fmt.Errorf("line %d: empty pack member at position %d", lineNo, i)
				}
				members[i] = m
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
			if len(fields) == 4 {
				ai = strings.ToLower(fields[3])
				if !IsPackAIName(ai) {
					return mf, fmt.Errorf("line %d: unknown pack AI %q (expected one of %v)", lineNo, fields[3], PackAINames)
				}
			}
			mf.Packs = append(mf.Packs, MapPack{Members: members, X: x, Z: z, AI: ai})
			continue
		}

		if state == slotDoors {
			fields := strings.Fields(line)
			// 6 fields = legacy row (style defaults to building); 7 = style
			// column present. Keeping both shapes valid means older maps load
			// untouched and only pick up a style column when re-saved.
			if len(fields) != 6 && len(fields) != 7 {
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
			if len(fields) == 7 {
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
			if len(fields) < 3 {
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
			if itemsToken != emptyChestToken {
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

		if state == slotCustomEnemies {
			ce, err := parseCustomEnemyLine(line, lineNo)
			if err != nil {
				return mf, err
			}
			mf.CustomEnemies = append(mf.CustomEnemies, ce)
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
			continue
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
	case "name":
		mf.Name = val
	case "materials":
		mf.Materials = val
	case "quiet":
		mf.Quiet = val
	case "size":
		w, h, err := parseSize(val)
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNo, err)
		}
		mf.Width, mf.Height = w, h
	case "start":
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
	if mf.StartX < 0 || mf.StartX >= mf.Width || mf.StartZ < 0 || mf.StartZ >= mf.Height {
		return fmt.Errorf("start (%d,%d) outside map", mf.StartX, mf.StartZ)
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
		if !d.HasTarget() {
			return fmt.Errorf("door %q at (%d,%d) missing target_map/target_door", d.Name, d.X, d.Z)
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
	if len(fields) != 3 {
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
// bad <name> %q" error wrap that every row decoder (packs, doors,
// chests, dimensions, start coords) used to inline. Six near-
// identical `strconv.Atoi` + `fmt.Errorf("line %d: bad %s %q", ...)`
// blocks collapse into one helper.
func parseIntField(s, name string, lineNo int) (int, error) {
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("line %d: bad %s %q", lineNo, name, s)
	}
	return v, nil
}

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

func init() {
	// Count the `%`-verbs (each consumes one argument). `%%` is a
	// literal percent sign, not a verb — none today, but the loop is
	// defensive so a future format with literal % doesn't double-count.
	verbs := 0
	for i := 0; i < len(customEnemyEncodeFormat); i++ {
		if customEnemyEncodeFormat[i] != '%' {
			continue
		}
		if i+1 < len(customEnemyEncodeFormat) && customEnemyEncodeFormat[i+1] == '%' {
			i++
			continue
		}
		verbs++
	}
	if verbs != customEnemyFieldCount {
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
		*f.dst = v
	}
	chance, err := strconv.ParseFloat(fields[schema.skch], 64)
	if err != nil {
		return MapCustomEnemy{}, fmt.Errorf("line %d: bad custom enemy sklch %q", lineNo, fields[schema.skch])
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

// Encode writes mf in the canonical .map format. Layers are emitted in a
// fixed order so encoded maps diff cleanly across edits.
func (mf MapFile) Encode(w io.Writer) error {
	bw := bufio.NewWriter(w)
	fmt.Fprintf(bw, "name: %s\n", mf.Name)
	fmt.Fprintf(bw, "materials: %s\n", mf.Materials)
	fmt.Fprintf(bw, "quiet: %s\n", mf.Quiet)
	fmt.Fprintf(bw, "size: %dx%d\n", mf.Width, mf.Height)
	fmt.Fprintf(bw, "start: %d %d %s\n", mf.StartX, mf.StartZ, mf.StartFace)
	ceiling := mf.Ceiling
	if len(ceiling) == 0 {
		ceiling = BlankLayer(mf.Width, mf.Height, CeilingOpenChar)
	}
	for _, layer := range append(mf.requiredLayers(), namedLayer{sectionCeiling, ceiling}) {
		fmt.Fprintf(bw, "%s:\n", layer.name)
		for _, row := range layer.rows {
			fmt.Fprintln(bw, row)
		}
	}
	fmt.Fprintln(bw, sectionEnemies+":")
	for _, p := range mf.Packs {
		// Single-member packs encode the same as the legacy "kind X Z" line
		// so maps without grouped packs stay byte-identical across the
		// format change. The AI column is appended only when non-default
		// (anything other than "none" / empty) so default-stationary
		// packs round-trip to the same 3-field shape.
		members := strings.Join(p.Members, ",")
		ai := strings.ToLower(strings.TrimSpace(p.AI))
		if ai == "" || ai == PackAINoneName {
			fmt.Fprintf(bw, "%s %d %d\n", members, p.X, p.Z)
		} else {
			fmt.Fprintf(bw, "%s %d %d %s\n", members, p.X, p.Z, ai)
		}
	}
	fmt.Fprintln(bw, sectionChests+":")
	for _, c := range mf.Chests {
		token := emptyChestToken
		if len(c.Items) > 0 {
			token = strings.Join(c.Items, ",")
		}
		fmt.Fprintf(bw, "%s %d %d\n", token, c.X, c.Z)
	}
	// doors: section is appended only when present. Older .map files
	// without any doors stay byte-identical across the format change —
	// the parser treats a missing section as zero-doors. Same rule as
	// the pre-ceiling-section backwards compatibility above.
	if len(mf.Doors) > 0 {
		fmt.Fprintln(bw, sectionDoors+":")
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
			fmt.Fprintf(bw, "%s %s %s %d %d %s %s\n", d.Name, d.TargetMap, d.TargetDoor, d.X, d.Z, d.Facing, style)
		}
	}
	// custom_enemies: emits only when present so older maps stay
	// byte-identical. Order documented on MapCustomEnemy and matches
	// parseCustomEnemyLine's positional decode. The format string
	// is broken out as customEnemyEncodeFormat so init() can assert
	// its `%`-verb count matches customEnemyFieldCount — keeps the
	// encoder and decoder honest about how many columns a row has.
	if len(mf.CustomEnemies) > 0 {
		fmt.Fprintln(bw, sectionCustomEnemies+":")
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

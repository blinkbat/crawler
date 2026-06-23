package core

import (
	"crawler/internal/app/core/mapfile"
	"fmt"
	"math"
	"slices"
)

// TileCoord formats (x, z) as "(3, 7)" for UI labels and editor diagnostics.
func TileCoord(x, z int) string {
	return fmt.Sprintf("(%d, %d)", x, z)
}

// Tile characters, grouped by owning layer. The .map format uses these bytes.

// Face-skin layer (on-disk name "walls"). These chars NO LONGER block movement —
// a wall is the rendered vertical FACE of an elevation step (StepElevationOK +
// renderer cliff faces). This layer only selects which texture skins a tile's
// exposed cliff faces. Authored from the editor's Faces palette.
const (
	TileOpen = '.' // blank — default (plain rock) cliff-face skin
	TileRock = '#' // explicit plain-rock skin
	// Face-skin VARIANTS — cosmetic; IsFaceSkinChar treats each as a known skin.
	TileWallRockIvyLight  = '+' // rock face with sparse green ivy
	TileWallRockIvyHeavy  = '=' // rock face blanketed in ivy
	TileWallRockCracked   = '&' // rock face fractured with cracks
	TileWallRockCrumbling = '$' // rock face in heavy disrepair
)

// FaceSkin pairs a cliff-face skin char with its display name. FaceSkins is THE
// canonical roster every face-skin consumer derives from; adding a skin is one
// row here, with init asserts in map.go/resources.go catching a missing label/model.
type FaceSkin struct {
	Char byte
	Name string
}

var FaceSkins = []FaceSkin{
	{TileRock, "Rock"},
	{TileWallRockIvyLight, "Light Ivy"},
	{TileWallRockIvyHeavy, "Heavy Ivy"},
	{TileWallRockCracked, "Cracked"},
	{TileWallRockCrumbling, "Crumbling"},
}

var faceSkinCharSet = func() (set [256]bool) {
	for _, s := range FaceSkins {
		set[s.Char] = true
	}
	return
}()

// IsFaceSkinChar reports whether a byte is an explicit cliff-face skin. Blank
// TileOpen is NOT a member — it reads as default rock via FaceSkinAt.
func IsFaceSkinChar(c byte) bool { return faceSkinCharSet[c] }

// FaceSkinName returns the display name for a skin char; blank/unrecognized → "Rock".
func FaceSkinName(c byte) string {
	for _, s := range FaceSkins {
		if s.Char == c {
			return s.Name
		}
	}
	return "Rock"
}

// Ceiling layer. TileCeilingOpen shows the skybox; TileCeilingSolid renders an
// opaque slab at wall height (roofed interior — "you are inside a dungeon room").
const (
	TileCeilingOpen  = mapfile.CeilingOpenChar  // '.' no ceiling — sky shows through
	TileCeilingSolid = mapfile.CeilingSolidChar // '#' solid ceiling slab at wall height
)

// Floor layer. Walkable surfaces — material-keyed variants read the material's
// floor pixels; universal variants load their own textures and apply anywhere.
// FloorDeepWater is the sole blocking floor tile (renders flat, BlockedAt impassable).
const (
	FloorAuto      = '.' // pick a variant by per-tile hash (back-compat default)
	FloorGrass     = 'g'
	FloorDirt      = 'd'
	FloorDarkGrass = 'k'
	FloorStone     = 's'
	// Universal floor variants — render the same in any material. None block
	// movement EXCEPT FloorDeepWater.
	FloorCobble    = 'c' // mortared cobblestone path
	FloorPlank     = 'w' // wooden planks
	FloorWater     = '~' // shallow water — walkable, just a different look
	FloorDeepWater = 'W' // deep water — blocks movement, vision passes over
	FloorSand      = 'n' // pale sand
	FloorSnow      = 'i' // packed snow
	// Ramp floor tiles — walkable slopes bridging the tile's stored level to one
	// higher in the arrow's UPHILL direction (^ north, > east, v south, < west).
	// 'v' overlaps the clover decor char (registered in crossLayerCharOverlaps).
	FloorRampNorth = '^'
	FloorRampEast  = '>'
	FloorRampSouth = 'v'
	FloorRampWest  = '<'
)

// ElevationGround is the elevation-layer char for the lowest level (0); blank/
// legacy maps seed to it so they read as flat.
const ElevationGround = mapfile.ElevationGroundChar

// Decor layer (cosmetic, never blocks). '.' = auto-scatter, '_' = suppress,
// explicit chars force a specific small prop.
const (
	DecorAuto      = '.'
	DecorEmpty     = '_'
	DecorBush      = 'b'
	DecorMushroom  = 'm'
	DecorPebble    = 'p'
	DecorTallGrass = ',' // upright blades of grass
	DecorFlowers   = 'f' // mixed wildflowers
	DecorClover    = 'v' // low clover patch
	DecorReeds     = 'r' // tall reed cluster
	DecorBones     = 'o' // skull + scattered bones
	DecorScorch    = 'x' // black scorch ring
	DecorBlood     = '!' // dried bloodstain
	DecorCobweb    = '*' // corner cobweb
	DecorStump     = 't' // weathered tree stump
	DecorLog       = 'l' // mossy fallen log
	DecorLeafPile  = 'L' // pile of fallen leaves
	// 2-tile archway (1×2 along +X, walkable): anchor at left tile, DecorArchwayTail
	// at the matching east tile or the renderer draws the anchor solo.
	DecorArchway     = 'A' // archway anchor (left tile)
	DecorArchwayTail = 'a' // archway tail   (right tile)
	DecorLilypad     = 'y' // floating pad+bloom; intended for water tiles, works on any
	DecorRug         = 'u' // woven floor rug, warm tones
	DecorCandle      = 'c' // stubby candle on a small puddle of wax
	DecorBootprints  = 'i' // pair of stamped boot prints
	DecorAshHeap     = 'h' // small heap of grey ash (smaller / cooler than scorch)
	DecorPuddle      = 'q' // shallow water puddle
	DecorRootCluster = 'k' // gnarled root stubs poking from the floor
)

// Props layer. TilePropEmpty marks an open cell; every other char is a blocker.
const (
	TilePropEmpty = '.' // open cell, no prop
	TileTree      = 'T' // regular tree, blocks
	TileTreeXL    = 'X' // extra-large tree, blocks
	// Tree shape variants — all 1-tile, all blocking; visual variance only.
	TileTreeTall     = '|' // tall narrow pine, slimmer + taller than Tree
	TileTreeTwin     = '@' // two trees crammed into one tile, offset
	TileTreeYoung    = '/' // young / smaller tree, scrubby thicket
	TileRockLarge    = 'O' // boulder, blocks
	TileBushLarge    = 'B' // dense bush, blocks
	TileCrate        = 'C' // wooden crate
	TileBarrel       = 'R' // banded barrel
	TileUrn          = 'U' // belly-shouldered urn
	TileStalagmite   = 'S' // cave stalagmite spire
	TilePillar       = 'P' // intact stone pillar with capital
	TileBrokenPillar = 'I' // toppled / chest-high pillar stub
	TileStatue       = 'M' // weathered humanoid statue
	TileObelisk      = 'Q' // tall four-sided pyramid-capped obelisk
	TileFountain     = 'F' // low fountain with a central plume
	// TileRockFormation is a 2×2 footprint: anchor at top-left, TileRockFormationTail
	// at the other three (renderer skips tails; anchor mesh covers all). All block.
	TileRockCairn         = 'K' // tall stacked-stone cairn (1 tile, blocks)
	TileRockFormation     = 'J' // 2×2 rock formation anchor (top-left)
	TileRockFormationTail = 'j' // 2×2 rock formation footprint shadow
	// Outdoor / field tileset — single-tile blockers.
	TileWell       = 'W' // stone-ringed well (cross-layer with FloorDeepWater)
	TileGravestone = 'G' // weathered tombstone
	TileSignPost   = 'N' // wooden sign on a post
	TileHayBale    = 'H' // round bound straw bale
	TileScarecrow  = 'Y' // cross + sackcloth scarecrow
	// Indoor / dungeon tileset — single-tile blockers.
	TileBookshelf   = 'V' // tall wooden shelf with books
	TileTable       = 'E' // wooden table with legs
	TileBed         = 'D' // wood-frame bed with bedding
	TileBrazier     = 'Z' // metal brazier on a tripod with flame (floor, BLOCKS, bright light)
	TileSarcophagus = 'A' // stone sarcophagus with lid (cross-layer with DecorArchway)
	// TileTorch is a wall-mounted torch — NON-blocking (leaves the floor clear, so
	// it can line corridors). Renderer auto-orients it away from the adjacent wall.
	TileTorch = 'z' // wall torch (non-blocking, animated flame, dim light)
	// Non-blocking decorative plants — player/packs walk through (PropIsNonBlocking).
	TilePropExoticFlower = 'e' // large funky bloom on a tall stalk
	TilePropTallFern     = '(' // cluster of tall arching fronds
	TilePropGrassTuft    = ')' // tall grass tuft (visual variance)
)

// Doors are entities (g.Doors), not a tile char; the tile underneath stays
// walkable floor. Movement checks for a door on step landing.

// SpawnSnapReason tags why a SnappedSpawnPositions entry has its shape, so
// callers (the editor's reachability warning) can distinguish drop causes.
type SpawnSnapReason int

const (
	// SpawnSnapPlaced means the pack will be rendered at TileX/TileZ.
	SpawnSnapPlaced SpawnSnapReason = iota
	// SpawnSnapEmptyMembers means the authored spawn has no enemies; runtime drops it.
	SpawnSnapEmptyMembers
	// SpawnSnapNoOpenTile means the nearest-open-tile search came up empty.
	SpawnSnapNoOpenTile
)

// SpawnSnap is one PackSpawn's placement result; TileX/TileZ are undefined
// unless Reason == SpawnSnapPlaced.
type SpawnSnap struct {
	TileX  int
	TileZ  int
	Reason SpawnSnapReason
}

// Placed reports whether this snap successfully positioned the pack.
func (s SpawnSnap) Placed() bool { return s.Reason == SpawnSnapPlaced }

// placePacks converts pack-spawn placeholders into runtime Packs, snapping each
// to the nearest open square (start tile seeded occupied). Empty rosters skipped.
func placePacks(a *AreaDefinition) []Pack {
	packs := make([]Pack, 0, len(a.PackSpawns))
	for i, snap := range SnappedSpawnPositions(a) {
		if !snap.Placed() {
			continue
		}
		spawn := a.PackSpawns[i]
		members := make([]Enemy, 0, len(spawn.Members))
		for _, member := range spawn.Members {
			e := NewEnemy(member.Kind)
			if name := member.CustomName; name != "" {
				if def, ok := CustomEnemyByName(a.CustomEnemies, name); ok {
					e = def.Instantiate()
				}
			}
			e.Row = member.Row // carry authored formation rank (front/back)
			members = append(members, e)
		}
		packs = append(packs, Pack{
			TileX:     snap.TileX,
			TileZ:     snap.TileZ,
			Level:     spawnLevel(a, snap.TileX, snap.TileZ),
			HomeX:     snap.TileX,
			HomeZ:     snap.TileZ,
			X:         TileCenter(snap.TileX),
			Z:         TileCenter(snap.TileZ),
			Members:   members,
			AI:        spawn.AI,
			PatrolDir: 1, // east; only PackAIPatrol reads/flips it
		})
	}
	return packs
}

// SnappedSpawnPositions returns the runtime tile each PackSpawn occupies after
// placePacks' snap (output index matches a.PackSpawns), with a Reason per drop.
// Exposed so the editor's reachability check uses the *snapped* positions and
// doesn't false-warn on a pack the game would silently relocate at runtime.
func SnappedSpawnPositions(a *AreaDefinition) []SpawnSnap {
	out := make([]SpawnSnap, 0, len(a.PackSpawns))
	// Seed with the CLAMPED start (the real spawn tile), matching placeChests/
	// placeCrystals — a raw out-of-bounds start would otherwise let a pack snap
	// onto the actual spawn tile.
	sx, sz := a.ClampedStart()
	occupied := map[[2]int]bool{{sx, sz}: true}
	for _, spawn := range a.PackSpawns {
		if len(spawn.Members) == 0 {
			out = append(out, SpawnSnap{Reason: SpawnSnapEmptyMembers})
			continue
		}
		x, z := nearestOpenTile(a, spawn.TileX, spawn.TileZ, occupied)
		if x < 0 {
			out = append(out, SpawnSnap{Reason: SpawnSnapNoOpenTile})
			continue
		}
		occupied[[2]int{x, z}] = true
		out = append(out, SpawnSnap{TileX: x, TileZ: z, Reason: SpawnSnapPlaced})
	}
	return out
}

func nearestOpenTile(a *AreaDefinition, wantX, wantZ int, occupied map[[2]int]bool) (int, int) {
	if a.FloorAt(wantX, wantZ) && !occupied[[2]int{wantX, wantZ}] {
		return wantX, wantZ
	}
	bestX, bestZ := -1, -1
	bestDist := math.MaxInt
	for z := 0; z < a.Height; z++ {
		for x := 0; x < a.Width; x++ {
			if !a.FloorAt(x, z) || occupied[[2]int{x, z}] {
				continue
			}
			dist := ManhattanDistance(x, z, wantX, wantZ)
			if dist < bestDist {
				bestDist = dist
				bestX, bestZ = x, z
			}
		}
	}
	return bestX, bestZ
}

// layerByteAt safely reads layer[z][x], ok=false when ragged/short. Lets the
// base-layer readers tolerate a non-rectangular hand-built AreaDefinition
// (test fixtures) instead of panicking on index-out-of-range.
func (a *AreaDefinition) layerByteAt(layer []string, x, z int) (byte, bool) {
	if z < 0 || z >= len(layer) {
		return 0, false
	}
	row := layer[z]
	if x < 0 || x >= len(row) {
		return 0, false
	}
	return row[x], true
}

// FloorCharAt / DecorCharAt / PropCharAt return the raw authored char at (x, z),
// bounds-safe (ok=false out-of-range or ragged), for callers wanting the char
// itself rather than a derived predicate.
func (a *AreaDefinition) FloorCharAt(x, z int) (byte, bool) { return a.layerByteAt(a.Floor, x, z) }
func (a *AreaDefinition) DecorCharAt(x, z int) (byte, bool) { return a.layerByteAt(a.Decor, x, z) }
func (a *AreaDefinition) PropCharAt(x, z int) (byte, bool)  { return a.layerByteAt(a.Props, x, z) }

// WallAt reports whether the cell is a SOLID obstruction (off-map, blocking
// prop, or deep water). Does NOT consider elevation steps (StepElevationOK).
// Out-of-bounds reads as solid.
func (a *AreaDefinition) WallAt(x, z int) bool {
	if !a.InBounds(x, z) {
		return true
	}
	if p, ok := a.layerByteAt(a.Props, x, z); ok && IsPropChar(p) && !PropIsNonBlocking(p) {
		return true
	}
	if f, ok := a.layerByteAt(a.Floor, x, z); ok && IsBlockingFloor(f) {
		return true
	}
	return false
}

// FaceSkinAt returns the cliff-face skin char for tile (x,z); blank/TileOpen/OOB
// default to TileRock.
func (a *AreaDefinition) FaceSkinAt(x, z int) byte {
	c, ok := a.layerByteAt(a.Walls, x, z)
	if !ok || c == TileOpen {
		return TileRock
	}
	return c
}

// FaceOverride is a per-tile cliff-face skin override: Skins[dir] (0=N,1=E,2=S,
// 3=W) names that face's skin, or 0/PropLevelAuto to fall back to FaceSkinAt.
type FaceOverride struct {
	X, Z  int
	Skins [FacingCount]byte
}

// defaultFaceSkins returns a per-face array all set to PropLevelAuto (not 0), so
// a new override matches its on-disk form ('.' per face) and won't false-dirty.
func defaultFaceSkins() [FacingCount]byte {
	var s [FacingCount]byte
	for i := range s {
		s[i] = PropLevelAuto
	}
	return s
}

func (a *AreaDefinition) faceOverrideAt(x, z int) (FaceOverride, bool) {
	if len(a.FaceOverrides) == 0 {
		return FaceOverride{}, false
	}
	// Lazy (x,z)->index map for O(1) render-path lookup; nil'd by any mutation
	// (SetFaceDir / CloneArea).
	if a.faceOverrideIdx == nil {
		a.faceOverrideIdx = make(map[[2]int]int, len(a.FaceOverrides))
		for i, o := range a.FaceOverrides {
			a.faceOverrideIdx[[2]int{o.X, o.Z}] = i
		}
	}
	if i, ok := a.faceOverrideIdx[[2]int{x, z}]; ok {
		return a.FaceOverrides[i], true
	}
	return FaceOverride{}, false
}

// FaceSkinForDir returns the skin for tile (x,z)'s `dir` face: the override when
// set, else the base FaceSkinAt skin.
func (a *AreaDefinition) FaceSkinForDir(x, z, dir int) byte {
	if len(a.FaceOverrides) > 0 && dir >= 0 && dir < FacingCount {
		if o, ok := a.faceOverrideAt(x, z); ok {
			if sc := o.Skins[dir]; sc != 0 && sc != PropLevelAuto {
				return sc
			}
		}
	}
	return a.FaceSkinAt(x, z)
}

// SetFaceDir sets tile (x,z)'s `dir` face skin (0=N,1=E,2=S,3=W). A skin equal
// to the base (or 0/PropLevelAuto) clears that face; an all-default entry is dropped.
func (a *AreaDefinition) SetFaceDir(x, z, dir int, skin byte) {
	if !a.InBounds(x, z) || dir < 0 || dir >= FacingCount {
		return
	}
	a.faceOverrideIdx = nil // invalidate lazy lookup (editor-only path)
	if skin == a.FaceSkinAt(x, z) || skin == 0 {
		skin = PropLevelAuto
	}
	idx := -1
	for i, o := range a.FaceOverrides {
		if o.X == x && o.Z == z {
			idx = i
			break
		}
	}
	if idx < 0 {
		if skin == PropLevelAuto {
			return
		}
		// Unset faces init to the auto sentinel so a new override won't false-dirty.
		nf := FaceOverride{X: x, Z: z, Skins: defaultFaceSkins()}
		a.FaceOverrides = append(a.FaceOverrides, nf)
		idx = len(a.FaceOverrides) - 1
	}
	a.FaceOverrides[idx].Skins[dir] = skin
	allDefault := true
	for _, s := range a.FaceOverrides[idx].Skins {
		if s != 0 && s != PropLevelAuto {
			allDefault = false
			break
		}
	}
	if allDefault {
		a.FaceOverrides = append(a.FaceOverrides[:idx], a.FaceOverrides[idx+1:]...)
	}
}

// CeilingAt reports whether the cell has a solid ceiling slab. OOB reads as
// no-ceiling; maps without a ceiling: section get a blank layer (always false).
func (a *AreaDefinition) CeilingAt(x, z int) bool {
	c, ok := a.layerByteAt(a.Ceiling, x, z)
	return ok && c == TileCeilingSolid
}

// ElevationLevelAt returns the STORED ground level (0..MaxElevationLevel) of
// tile (x,z). Ramp tiles store their LOW level. OOB / missing layer / non-digit
// read as 0 (a map without an elevation layer reads as flat at the bottom).
func (a *AreaDefinition) ElevationLevelAt(x, z int) int {
	// With an explicit stack, this is the column's top solid level (for a gapped
	// column, the floating cube's top, not the standable ground).
	if len(a.Solids) > 0 {
		if t := a.TopSolidLevel(x, z); t >= 0 {
			return t
		}
		return 0
	}
	c, ok := a.layerByteAt(a.Elevation, x, z)
	if !ok {
		return 0
	}
	return ElevationLevelFromChar(c)
}

// Elevation range. ElevationBaseline is where the default walkable floor sits,
// giving headroom in both directions (raise for walls/cliffs, lower for pits).
// Levels 0..9 use '0'..'9', 10..20 extend into 'A'.. (base-36, one char/cell —
// see ElevationChar), keeping every grid op at 1 char = 1 cell.
const (
	ElevationBaseline = 10
	MaxElevationLevel = 20
	// elevDigitSpan is the base-36 digit/letter pivot ('0'..'9' = 0..9, then 'A'..).
	elevDigitSpan = 10
	// ElevationWallRingLevel is the default enclosing-wall level: one step above
	// the baseline so it reads as a 1-high cliff. Shared by blankArea + sealWallBorder.
	ElevationWallRingLevel = ElevationBaseline + 1
)

// ElevationLevelFromChar decodes a cell byte to a STORED level ('0'..'9'→0..9,
// 'A'..ElevationChar(MaxElevationLevel)→10..MaxElevationLevel); anything outside
// the legal alphabet reads as 0, agreeing with mapfile's validator (which rejects
// chars above the same upper bound rather than letting them clamp to the max).
func ElevationLevelFromChar(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return Clamp(int(c-'0'), 0, MaxElevationLevel)
	case c >= 'A' && c <= ElevationChar(MaxElevationLevel):
		return Clamp(int(c-'A')+elevDigitSpan, 0, MaxElevationLevel)
	}
	return 0
}

// ElevationChar encodes a level to its grid byte (inverse of ElevationLevelFromChar),
// clamped to [0, MaxElevationLevel].
func ElevationChar(level int) byte {
	level = Clamp(level, 0, MaxElevationLevel)
	if level < elevDigitSpan {
		return byte('0' + level)
	}
	return byte('A' + (level - elevDigitSpan))
}

// IsRampChar reports whether a floor char is one of the four ramp tiles.
func IsRampChar(c byte) bool {
	switch c {
	case FloorRampNorth, FloorRampEast, FloorRampSouth, FloorRampWest:
		return true
	}
	return false
}

// RampAscentFacing maps a ramp char to the cardinal facing it rises toward;
// ok=false for non-ramp chars.
func RampAscentFacing(c byte) (int, bool) {
	switch c {
	case FloorRampNorth:
		return North, true
	case FloorRampEast:
		return East, true
	case FloorRampSouth:
		return South, true
	case FloorRampWest:
		return West, true
	}
	return 0, false
}

// RampCharForFacing returns the ramp char ascending toward `facing` (inverse of
// RampAscentFacing).
func RampCharForFacing(facing int) byte {
	switch NormalizeFacing(facing) {
	case North:
		return FloorRampNorth
	case East:
		return FloorRampEast
	case South:
		return FloorRampSouth
	case West:
		return FloorRampWest
	}
	return FloorRampNorth
}

// RampAt reports whether tile (x,z)'s floor is a ramp and its ascent facing;
// ok=false OOB or on flat floor.
func (a *AreaDefinition) RampAt(x, z int) (facing int, ok bool) {
	c, present := a.layerByteAt(a.Floor, x, z)
	if !present {
		return 0, false
	}
	return RampAscentFacing(c)
}

// ElevationWorldY is the world-space Y of a floor at the given stored level.
// ElevationBaseline renders at y=0 (flat map sits at the origin, where combat
// and y~0 assumptions expect ground); the one home for stored-level→world-Y.
func ElevationWorldY(level int) float32 {
	return float32(level-ElevationBaseline) * LevelStep
}

// StandGroundY returns the world-space Y a unit rests at on tile (x,z). Flat =
// ElevationWorldY(level); a ramp reads its MID-slope height (low+0.5 levels).
func (a *AreaDefinition) StandGroundY(x, z int) float32 {
	return a.StandGroundYAt(x, a.ElevationLevelAt(x, z), z)
}

// StandGroundYAt is StandGroundY for an EXPLICIT standing level, so a unit on a
// bridge deck and one on the ground beneath it sit at their own heights. A ramp
// still reads +0.5 level. StandGroundY is the level==column-top special case.
func (a *AreaDefinition) StandGroundYAt(x, level, z int) float32 {
	y := float32(level - ElevationBaseline)
	if _, ok := a.RampAt(x, z); ok {
		y += 0.5
	}
	return y * LevelStep
}

// NoRamp is the sentinel ramp facing for a flat tile, passed to EdgeLevelOf.
const NoRamp = -1

// EdgeLevelOf is the pure edge-level rule for a tile's stored `level` and
// `rampFacing` (NoRamp = flat). Flat presents `level` on all edges; a ramp
// presents HIGH (low+1) on the edge it ascends toward, LOW on the opposite, and
// no walkable edge (ok=false) on its sheer perpendicular sides. Shared by
// edgeLevel and the renderer's cliff-face pass so movement and render can't drift.
func EdgeLevelOf(level, rampFacing, dir int) (int, bool) {
	if rampFacing == NoRamp {
		return level, true
	}
	switch NormalizeFacing(dir) {
	case NormalizeFacing(rampFacing):
		return level + 1, true
	case OppositeFacing(rampFacing):
		return level, true
	default:
		return 0, false
	}
}

// edgeLevel returns the level at tile (x,z)'s `dir` edge (ok=false if no walkable
// edge), delegating the rule to EdgeLevelOf.
func (a *AreaDefinition) edgeLevel(x, z, dir int) (int, bool) {
	if !a.InBounds(x, z) {
		return 0, false
	}
	ramp := NoRamp
	if f, ok := a.RampAt(x, z); ok {
		ramp = f
	}
	return EdgeLevelOf(a.ElevationLevelAt(x, z), ramp, dir)
}

// OppositeFacing returns the reverse cardinal of `f` (the wrap-safe half-turn).
func OppositeFacing(f int) int { return NormalizeFacing(f + FacingCount/2) }

// CardinalDirs is the canonical N→E→S→W neighbour order, shared by the
// face/exposure walks (TileExposesFace, render.drawCliffFaces).
var CardinalDirs = [FacingCount]int{North, East, South, West}

// NeighbourEdgeLevel returns the level the neighbour at (nx,nz) presents across
// the edge entered from `fromDir`: ramp-aware edgeLevel, else flat level, else
// the baseline off-map (so a raised border shows a clean 1-high lip).
func (a *AreaDefinition) NeighbourEdgeLevel(nx, nz, fromDir int) int {
	if !a.InBounds(nx, nz) {
		return ElevationBaseline
	}
	if l, ok := a.edgeLevel(nx, nz, fromDir); ok {
		return l
	}
	return a.ElevationLevelAt(nx, nz)
}

// TileExposesFace reports whether tile (x,z) renders >=1 cliff face — not a ramp
// and above some cardinal neighbour (or the baseline at the edge). The editor's
// "Set face" menu gates on this; renderer.drawCliffFaces mirrors it per-edge.
func TileExposesFace(a *AreaDefinition, x, z int) bool {
	if _, isRamp := a.RampAt(x, z); isRamp {
		return false
	}
	my := a.ElevationLevelAt(x, z)
	for _, d := range CardinalDirs {
		dx, dz := FacingVector(d)
		if my > a.NeighbourEdgeLevel(x+dx, z+dz, OppositeFacing(d)) {
			return true
		}
	}
	return false
}

// StepElevationOK reports whether a step from (fromX,fromZ) toward `dir` connects
// without a cliff — leave-edge and enter-edge levels match (ramp-aware). A
// mismatch or a perpendicular ramp mount/dismount is blocked.
func (a *AreaDefinition) StepElevationOK(fromX, fromZ, dir int) bool {
	dx, dz := FacingVector(dir)
	from, ok1 := a.edgeLevel(fromX, fromZ, dir)
	to, ok2 := a.edgeLevel(fromX+dx, fromZ+dz, OppositeFacing(dir))
	return ok1 && ok2 && from == to
}

// TileAt returns a composite char for "what's most-significantly here" — props
// > deep water > open. Used by the minimap; elevation is read separately.
func (a *AreaDefinition) TileAt(x, z int) byte {
	if !a.InBounds(x, z) {
		return TileOpen
	}
	if p, ok := a.layerByteAt(a.Props, x, z); ok && IsPropChar(p) {
		return p
	}
	if f, ok := a.layerByteAt(a.Floor, x, z); ok && f == FloorDeepWater {
		return FloorDeepWater
	}
	return TileOpen
}

// BlockedAt reports whether a tile is impossible to STAND on (blocking prop or
// deep water). Does NOT consider elevation (the lateral cliff rule is in
// StepElevationOK); OOB reads as blocked. Runtime blockers (chests) are layered
// on by explore.go, not here, so the editor can call this without runtime state.
func (a *AreaDefinition) BlockedAt(x, z int) bool {
	if !a.InBounds(x, z) {
		return true
	}
	if p, ok := a.layerByteAt(a.Props, x, z); ok && IsPropChar(p) && !PropIsNonBlocking(p) {
		return true
	}
	if f, ok := a.layerByteAt(a.Floor, x, z); ok {
		return IsBlockingFloor(f)
	}
	return false
}

// PropIsNonBlocking reports whether a valid prop char should NOT block movement
// (wall torches, decorative plants — walkable). Every other prop blocks.
func PropIsNonBlocking(c byte) bool {
	switch c {
	case TileTorch, TilePropExoticFlower, TilePropTallFern, TilePropGrassTuft:
		return true
	}
	return false
}

// PropBlockHeight is how many voxel levels a blocking prop occupies upward (its
// "tallness" for level-aware collision); 0 if non-blocking/empty. Squat props
// block 1, full trees/cairns/formations block 2 — so a ground tree blocks the
// walk-under path while leaving the deck above walkable.
func PropBlockHeight(c byte) int {
	switch c {
	case TileTreeYoung, TileRockLarge, TileBushLarge:
		return 1
	case TileTree, TileTreeTwin, TileRockCairn, TileRockFormation, TileRockFormationTail,
		TileTreeTall, TileTreeXL:
		return 2
	}
	return 0
}

// PropLevelAuto is the "no explicit level — rest on the column's lowest
// standable surface" sentinel, disjoint from the base-36 level chars. Absent
// PropLevels grid reads as all-auto.
const PropLevelAuto = '.'

// levelGridAt reads a per-tile level grid (PropLevels / DecorLevels): the
// authored char when set, else the column's lowest standable surface.
func (a *AreaDefinition) levelGridAt(layer []string, x, z int) int {
	if c, ok := a.layerByteAt(layer, x, z); ok && c != PropLevelAuto {
		return ElevationLevelFromChar(c)
	}
	// Heightfield auto = column top, read directly: LowestStandableLevel scans the
	// whole map, and this runs per-tile per-frame in drawWorld. Only gapped columns walk.
	if len(a.Solids) == 0 {
		return a.ElevationLevelAt(x, z)
	}
	if lo := a.LowestStandableLevel(x, z); lo >= 0 {
		return lo
	}
	return a.ElevationLevelAt(x, z)
}

// PropLevelAt / DecorLevelAt are the level the prop/decor on tile (x,z) sits on.
func (a *AreaDefinition) PropLevelAt(x, z int) int  { return a.levelGridAt(a.PropLevels, x, z) }
func (a *AreaDefinition) DecorLevelAt(x, z int) int { return a.levelGridAt(a.DecorLevels, x, z) }

// PropBlocksStanding reports whether the prop on tile (x,z) occupies standing
// `level` — it roots at PropLevelAt and rises PropBlockHeight. On a gapped column
// it blocks only those levels, so you can walk under a deck past a ground tree.
func (a *AreaDefinition) PropBlocksStanding(x, level, z int) bool {
	c, ok := a.layerByteAt(a.Props, x, z)
	if !ok || !IsPropChar(c) || PropIsNonBlocking(c) {
		return false
	}
	base := a.PropLevelAt(x, z)
	h := PropBlockHeight(c)
	return level >= base && level < base+h
}

// EnterOpts parameterizes CanEnterTile. Zero value = strictest (forbid doors,
// the player tile, pack-occupied tiles), the pack-wander set. Callers flip:
//   - Player step: AllowDoorTile=true, OccupiedPacks=nil (caller owns the
//     step-into-pack→engage branch).
//   - Pack chase: AllowPlayerTile=true (the chase tile IS the player), OccupiedPacks set.
//   - Pack wander: all false, OccupiedPacks set.
//
// PlayerTileX/Z is consulted only when OccupiedPacks!=nil OR AllowPlayerTile.
type EnterOpts struct {
	AllowDoorTile   bool
	AllowPlayerTile bool
	PlayerTileX     int
	PlayerTileZ     int
	OccupiedPacks   map[[2]int]bool
}

// CanEnterTile is the single predicate for "can an actor legally step onto
// (tx,tz) now": static BlockedAt + runtime blockers (chests, doors, packs, player).
func CanEnterTile(g *GameState, tx, tz int, opts EnterOpts) bool {
	if g == nil || !g.Area.InBounds(tx, tz) {
		return false
	}
	if g.Area.BlockedAt(tx, tz) {
		return false
	}
	return canEnterRuntimeBlockers(g, tx, tz, opts)
}

// CanEnterTileAtLevel is CanEnterTile with LEVEL-AWARE prop blocking
// (PropBlocksStanding), so a unit can step UNDER a deck past a ground tree. Used
// by voxel movement / pack-AI; deep water still blocks regardless of level.
func CanEnterTileAtLevel(g *GameState, tx, tz, level int, opts EnterOpts) bool {
	if g == nil || !g.Area.InBounds(tx, tz) {
		return false
	}
	if f, ok := g.Area.layerByteAt(g.Area.Floor, tx, tz); ok && IsBlockingFloor(f) {
		return false
	}
	if g.Area.PropBlocksStanding(tx, level, tz) {
		return false
	}
	return canEnterRuntimeBlockers(g, tx, tz, opts)
}

// canEnterRuntimeBlockers is the shared runtime-blocker tail (chests, doors,
// player/pack occupancy); static terrain is checked by the caller first.
// PlayerTileX/Z is gated on AllowPlayerTile/OccupiedPacks, else the zero-default
// (0,0) would falsely block every step toward tile (0,0).
func canEnterRuntimeBlockers(g *GameState, tx, tz int, opts EnterOpts) bool {
	if ChestIndexAt(g.Chests, tx, tz) >= 0 {
		return false
	}
	if !opts.AllowDoorTile && DoorIndexAt(g.Doors, tx, tz) >= 0 {
		return false
	}
	if opts.AllowPlayerTile || opts.OccupiedPacks != nil {
		if tx == opts.PlayerTileX && tz == opts.PlayerTileZ {
			return opts.AllowPlayerTile
		}
	}
	if opts.OccupiedPacks != nil && opts.OccupiedPacks[[2]int{tx, tz}] {
		return false
	}
	return true
}

// ChestTakeAllRow is the synthetic row one past the last live-stack row in the
// chest modal (selecting it drains all). `stackCount` is len(LiveStacks(chest.Items)).
func ChestTakeAllRow(stackCount int) int { return stackCount }

// MarkChestLootedIfEmpty flips Looted (and clears zero-count residue) once every
// stack is drained.
func MarkChestLootedIfEmpty(c *Chest) {
	if c == nil {
		return
	}
	if len(LiveStacks(c.Items)) == 0 {
		c.Items = nil
		c.Looted = true
	}
}

// ChestIndexAt returns the index of the chest on (x,z), or -1. Linear scan
// (chest counts per map are tiny); DoorIndexAt / PackIndexAtTile share the idiom.
func ChestIndexAt(chests []Chest, x, z int) int {
	return slices.IndexFunc(chests, func(c Chest) bool { return c.TileX == x && c.TileZ == z })
}

// AdjacentChestIndex returns the index of a chest one cardinal step from (x,z),
// or -1. Includes looted chests (lid still blocks the tile); no diagonals.
func AdjacentChestIndex(chests []Chest, x, z int) int {
	for i, c := range chests {
		if ManhattanDistance(c.TileX, c.TileZ, x, z) == 1 {
			return i
		}
	}
	return -1
}

// AdjacentInteractableChestIndex is the openable-only variant (skips looted chests).
func AdjacentInteractableChestIndex(chests []Chest, x, z int) int {
	idx := AdjacentChestIndex(chests, x, z)
	if idx < 0 || chests[idx].Looted {
		return -1
	}
	return idx
}

// SpawnIndexAt is the shared "find the authored spawn at (x,z)" scan, generic
// over any TileXZ spawn type.
func SpawnIndexAt[T TileXZ](spawns []T, x, z int) int {
	return slices.IndexFunc(spawns, func(s T) bool {
		tx, tz := s.Tile()
		return tx == x && tz == z
	})
}

// PackSpawnIndexAt returns the index of the pack spawn at (x,z), or -1.
func PackSpawnIndexAt(spawns []PackSpawn, x, z int) int {
	return SpawnIndexAt(spawns, x, z)
}

// ChestSpawnIndexAt returns the index of the chest spawn at (x,z), or -1.
func ChestSpawnIndexAt(spawns []ChestSpawn, x, z int) int {
	return SpawnIndexAt(spawns, x, z)
}

// DoorSpawnIndexAt returns the index of the door spawn at (x,z), or -1.
func DoorSpawnIndexAt(spawns []DoorSpawn, x, z int) int {
	return SpawnIndexAt(spawns, x, z)
}

// CrystalSpawnIndexAt returns the index of the crystal spawn at (x,z), or -1.
func CrystalSpawnIndexAt(spawns []CrystalSpawn, x, z int) int {
	return SpawnIndexAt(spawns, x, z)
}

// AreaTileSummary describes what's painted on (x,z) across every layer + entity
// list, omitting empty layers. "(empty)" when nothing, "" when OOB.
func AreaTileSummary(a *AreaDefinition, x, z int) string {
	if !a.InBounds(x, z) {
		return ""
	}
	parts := make([]string, 0, 8)
	if w, ok := a.layerByteAt(a.Walls, x, z); ok {
		if lbl := TileLabel(TileLayerWalls, w); lbl != "" {
			parts = append(parts, lbl)
		}
	}
	if f, ok := a.layerByteAt(a.Floor, x, z); ok {
		if lbl := TileLabel(TileLayerFloor, f); lbl != "" {
			parts = append(parts, lbl)
		}
	}
	if d, ok := a.layerByteAt(a.Decor, x, z); ok {
		if lbl := TileLabel(TileLayerDecor, d); lbl != "" {
			parts = append(parts, lbl)
		}
	}
	if p, ok := a.layerByteAt(a.Props, x, z); ok {
		if lbl := TileLabel(TileLayerProps, p); lbl != "" {
			parts = append(parts, lbl)
		}
	}
	if a.StartTileX == x && a.StartTileZ == z {
		parts = append(parts, "Start")
	}
	if PackSpawnIndexAt(a.PackSpawns, x, z) >= 0 {
		parts = append(parts, "Pack")
	}
	if ChestSpawnIndexAt(a.ChestSpawns, x, z) >= 0 {
		parts = append(parts, "Chest")
	}
	if DoorSpawnIndexAt(a.DoorSpawns, x, z) >= 0 {
		parts = append(parts, "Door")
	}
	if CrystalSpawnIndexAt(a.CrystalSpawns, x, z) >= 0 {
		parts = append(parts, "Crystal")
	}
	if len(parts) == 0 {
		return "(empty)"
	}
	return joinSummary(parts)
}

// joinSummary concatenates parts with " / " into one pre-sized buffer (called
// every frame by the editor hover readout; avoids per-part allocations).
func joinSummary(parts []string) string {
	if len(parts) == 1 {
		return parts[0]
	}
	const sep = " / "
	n := len(sep) * (len(parts) - 1)
	for _, p := range parts {
		n += len(p)
	}
	buf := make([]byte, 0, n)
	buf = append(buf, parts[0]...)
	for _, p := range parts[1:] {
		buf = append(buf, sep...)
		buf = append(buf, p...)
	}
	return string(buf)
}

// FloorAt is the inverse of BlockedAt — true when the cell is walkable.
func (a *AreaDefinition) FloorAt(x, z int) bool {
	return !a.BlockedAt(x, z)
}

// InBounds reports whether (x, z) is inside the area's dimensions.
func (a *AreaDefinition) InBounds(x, z int) bool {
	return z >= 0 && z < a.Height && x >= 0 && x < a.Width
}

// MultiTileOffset is one footprint cell as a (dx, dz) offset from the anchor.
type MultiTileOffset struct {
	DX, DZ int
}

// FootprintCentroid returns the (dx, dz) offset (tile-space units) from the
// anchor center to the footprint's geometric center; (0,0) for single-tile/empty.
func FootprintCentroid(offsets []MultiTileOffset) (dx, dz float32) {
	if len(offsets) <= 1 {
		return 0, 0
	}
	var sx, sz float32
	for _, o := range offsets {
		sx += float32(o.DX)
		sz += float32(o.DZ)
	}
	n := float32(len(offsets))
	return sx / n, sz / n
}

// FootprintWorldOffset is FootprintCentroid scaled by TileSize (world-space).
func FootprintWorldOffset(offsets []MultiTileOffset) (x, z float32) {
	cdx, cdz := FootprintCentroid(offsets)
	return cdx * TileSize, cdz * TileSize
}

// PropFootprint returns the cells (incl. anchor at (0,0)) occupied by the prop
// at `anchor`; nil for non-anchor/unknown (treat as single-tile). The returned
// slice is SHARED + read-only — must not be mutated (avoids per-frame allocation).
func PropFootprint(anchor byte) []MultiTileOffset {
	return propFootprints[anchor].offsets
}

// footprintDef bundles a multi-tile anchor's offsets with the tail char stamped
// on every non-anchor cell, so adding an anchor is one table entry.
type footprintDef struct {
	offsets []MultiTileOffset
	tail    byte
}

// Shared, read-only footprint offset slices (package-level to avoid per-frame allocation).
var (
	rockFormationFootprint = []MultiTileOffset{{0, 0}, {1, 0}, {0, 1}, {1, 1}}
	archwayFootprint       = []MultiTileOffset{{0, 0}, {1, 0}}
)

// propFootprints / decorFootprints map an anchor char to its footprint + tail;
// a miss returns the zero footprintDef ("single-tile or unknown anchor").
var (
	propFootprints = map[byte]footprintDef{
		TileRockFormation: {offsets: rockFormationFootprint, tail: TileRockFormationTail},
	}
	decorFootprints = map[byte]footprintDef{
		DecorArchway: {offsets: archwayFootprint, tail: DecorArchwayTail},
	}
)

// PropFootprintTail returns the tail char for a multi-tile prop's non-anchor
// cells; 0 if single-tile/unknown.
func PropFootprintTail(anchor byte) byte {
	return propFootprints[anchor].tail
}

// DecorFootprint mirrors PropFootprint for the decor layer.
func DecorFootprint(anchor byte) []MultiTileOffset {
	return decorFootprints[anchor].offsets
}

// DecorFootprintTail returns the tail char for a multi-tile decor anchor.
func DecorFootprintTail(anchor byte) byte {
	return decorFootprints[anchor].tail
}

// propTileCharList is the canonical list of every blocking prop-layer char
// (IsPropChar dispatches via a set; PropTileChars() exposes a defensive copy).
var propTileCharList = []byte{
	TileTree, TileTreeXL, TileTreeTall, TileTreeTwin, TileTreeYoung,
	TileRockLarge, TileBushLarge,
	TileCrate, TileBarrel, TileUrn, TileStalagmite,
	TilePillar, TileBrokenPillar, TileStatue, TileObelisk, TileFountain,
	TileRockCairn, TileRockFormation, TileRockFormationTail,
	TileWell, TileGravestone, TileSignPost, TileHayBale, TileScarecrow,
	TileBookshelf, TileTable, TileBed, TileBrazier, TileSarcophagus,
	TileTorch,                                                 // prop char, but non-blocking in BlockedAt (mounts on wall)
	TilePropExoticFlower, TilePropTallFern, TilePropGrassTuft, // non-blocking plants
}

// propTileCharSet is the O(1) lookup for IsPropChar, derived from propTileCharList.
var propTileCharSet = buildPropTileCharSet()

func buildPropTileCharSet() map[byte]struct{} {
	m := make(map[byte]struct{}, len(propTileCharList))
	for _, c := range propTileCharList {
		m[c] = struct{}{}
	}
	return m
}

// PropTileChars returns a defensive copy of every blocking prop-layer char.
func PropTileChars() []byte {
	out := make([]byte, len(propTileCharList))
	copy(out, propTileCharList)
	return out
}

// IsPropChar reports whether c names a known blocking prop.
func IsPropChar(c byte) bool {
	_, ok := propTileCharSet[c]
	return ok
}

// decorTileCharList is the canonical list of every explicit decor char with a
// renderable model. '.' (auto) and '_' (force-empty) are excluded sentinels.
// The renderer asserts each entry has a model.
var decorTileCharList = []byte{
	DecorBush, DecorMushroom, DecorPebble,
	DecorTallGrass, DecorFlowers, DecorClover, DecorReeds,
	DecorBones, DecorScorch, DecorBlood, DecorCobweb,
	DecorStump, DecorLog, DecorLeafPile,
	DecorArchway, DecorArchwayTail,
	DecorLilypad,
	DecorRug, DecorCandle, DecorBootprints,
	DecorAshHeap, DecorPuddle, DecorRootCluster,
}

// DecorTileChars returns a defensive copy of every renderable decor char.
func DecorTileChars() []byte {
	out := make([]byte, len(decorTileCharList))
	copy(out, decorTileCharList)
	return out
}

// blockingFloorChars is the single source of truth for blocking floor chars; a
// future blocker (lava, void) is one append here.
var blockingFloorChars = []byte{FloorDeepWater}

// blockingFloorCharSet is the O(1) lookup mirror (BlockedAt runs on every
// movement / pack-AI / nearest-open-tile query, so set membership beats a scan).
var blockingFloorCharSet = func() map[byte]struct{} {
	m := make(map[byte]struct{}, len(blockingFloorChars))
	for _, b := range blockingFloorChars {
		m[b] = struct{}{}
	}
	return m
}()

// IsBlockingFloor reports whether a floor char blocks movement.
func IsBlockingFloor(c byte) bool {
	_, ok := blockingFloorCharSet[c]
	return ok
}

// BlockingFloorChars returns a fresh copy of every blocking floor char.
func BlockingFloorChars() []byte {
	return append([]byte(nil), blockingFloorChars...)
}

// floorTileCharList enumerates every floor char with a defined visual (excludes
// the FloorAuto sentinel); feeds TileLabel coverage.
var floorTileCharList = []byte{
	FloorGrass, FloorDirt, FloorDarkGrass, FloorStone,
	FloorCobble, FloorPlank, FloorWater, FloorDeepWater,
	FloorSand, FloorSnow,
	FloorRampNorth, FloorRampEast, FloorRampSouth, FloorRampWest,
}

// TileLayer enumerates the four char-carrying grid layers (walls/floor/decor/
// props). The editor's Layer enum adds Ceiling (reuses '.'/'#' from walls, no
// own row) and Entities (spawn slices, not tile chars).
type TileLayer int

const (
	TileLayerWalls TileLayer = iota
	TileLayerFloor
	TileLayerDecor
	TileLayerProps
)

// tileLabelTable is the per-(layer, char) label registry powering TileLabel.
// Sentinels map to "". init() below asserts every canonical char has an entry,
// so a new tile const without a label panics at startup instead of returning "?".
var tileLabelTable = map[TileLayer]map[byte]string{
	// Face-skin labels are populated from FaceSkins in init; only the blank default is here.
	TileLayerWalls: {
		TileOpen: "", // default rock skin
	},
	TileLayerFloor: {
		FloorAuto:      "",
		FloorGrass:     "Grass",
		FloorDirt:      "Dirt",
		FloorDarkGrass: "Dark Grass",
		FloorStone:     "Stone",
		FloorCobble:    "Cobble",
		FloorPlank:     "Planks",
		FloorWater:     "Water",
		FloorDeepWater: "Deep Water",
		FloorSand:      "Sand",
		FloorSnow:      "Snow",
		FloorRampNorth: "Ramp ↑N",
		FloorRampEast:  "Ramp →E",
		FloorRampSouth: "Ramp ↓S",
		FloorRampWest:  "Ramp ←W",
	},
	TileLayerDecor: {
		DecorAuto:        "",
		DecorEmpty:       "",
		DecorBush:        "Bush",
		DecorMushroom:    "Mushroom",
		DecorPebble:      "Pebble",
		DecorTallGrass:   "Tall Grass",
		DecorFlowers:     "Flowers",
		DecorClover:      "Clover",
		DecorReeds:       "Reeds",
		DecorBones:       "Bones",
		DecorScorch:      "Scorch",
		DecorBlood:       "Blood",
		DecorCobweb:      "Cobweb",
		DecorStump:       "Stump",
		DecorLog:         "Log",
		DecorLeafPile:    "Leaf Pile",
		DecorArchway:     "Arch (left)",
		DecorArchwayTail: "Arch (right)",
		DecorLilypad:     "Lilypad",
		DecorRug:         "Rug",
		DecorCandle:      "Candle",
		DecorBootprints:  "Boot Prints",
		DecorAshHeap:     "Ash Heap",
		DecorPuddle:      "Puddle",
		DecorRootCluster: "Roots",
	},
	TileLayerProps: {
		TilePropEmpty:         "",
		TileTree:              "Tree",
		TileTreeXL:            "Tree XL",
		TileTreeTall:          "Tall Tree",
		TileTreeTwin:          "Twin Trees",
		TileTreeYoung:         "Young Tree",
		TileRockLarge:         "Boulder",
		TileBushLarge:         "Large Bush",
		TileCrate:             "Crate",
		TileBarrel:            "Barrel",
		TileUrn:               "Urn",
		TileStalagmite:        "Stalagmite",
		TilePillar:            "Pillar",
		TileBrokenPillar:      "Broken Pillar",
		TileStatue:            "Statue",
		TileObelisk:           "Obelisk",
		TileFountain:          "Fountain",
		TileRockCairn:         "Rock Cairn",
		TileRockFormation:     "Rock Formation (anchor)",
		TileRockFormationTail: "Rock Formation (tail)",
		TileWell:              "Well",
		TileGravestone:        "Gravestone",
		TileSignPost:          "Sign Post",
		TileHayBale:           "Hay Bale",
		TileScarecrow:         "Scarecrow",
		TileBookshelf:         "Bookshelf",
		TileTable:             "Table",
		TileBed:               "Bed",
		TileBrazier:           "Brazier",
		TileSarcophagus:       "Sarcophagus",
		TileTorch:             "Wall Torch",
		TilePropExoticFlower:  "Exotic Flower",
		TilePropTallFern:      "Tall Fern",
		TilePropGrassTuft:     "Grass Tuft",
	},
}

// init asserts every authored tile char has a TileLabel entry, and that any byte
// shared across layers is registered as an INTENDED overlap in
// crossLayerCharOverlaps (per-layer dispatch makes it safe, but a "what's here?"
// surface would render two labels). An unregistered collision panics at startup.
func init() {
	// Derive Faces-layer labels from FaceSkins so a new skin is one row there.
	faceLabels := tileLabelTable[TileLayerWalls]
	for _, s := range FaceSkins {
		faceLabels[s.Char] = s.Name + " Face"
	}
	floorLabels := tileLabelTable[TileLayerFloor]
	for _, c := range floorTileCharList {
		if _, ok := floorLabels[c]; !ok {
			panic("core: floor char '" + string(c) + "' missing from tileLabelTable[TileLayerFloor]")
		}
	}
	decorLabels := tileLabelTable[TileLayerDecor]
	for _, c := range decorTileCharList {
		if _, ok := decorLabels[c]; !ok {
			panic("core: decor char '" + string(c) + "' missing from tileLabelTable[TileLayerDecor]")
		}
	}
	propLabels := tileLabelTable[TileLayerProps]
	for _, c := range propTileCharList {
		if _, ok := propLabels[c]; !ok {
			panic("core: prop char '" + string(c) + "' missing from tileLabelTable[TileLayerProps]")
		}
	}
	assertNoUnregisteredCrossLayerOverlaps()
	// core/mapfile's elevation parser hardcodes the top-level char 'K' for its
	// validation range (it can't import core). Pin it here so bumping
	// MaxElevationLevel can't silently desync the two — if this fires, update the
	// 'A'..'K' bound in core/mapfile/mapfile.go to match.
	if c := ElevationChar(MaxElevationLevel); c != 'K' {
		panic(fmt.Sprintf("core: ElevationChar(MaxElevationLevel)=%q but core/mapfile hardcodes 'K' — update mapfile.go's elevation bound", c))
	}
}

// crossLayerOverlap records the two layers sharing a byte, each layer's label,
// and why the overlap is intentional.
type crossLayerOverlap struct {
	A, B   TileLayer
	Char   byte
	NameA  string
	NameB  string
	Reason string
}

// crossLayerCharOverlaps is the registry of deliberately-shared bytes across
// layers; a new overlap must register here or the init assert panics.
var crossLayerCharOverlaps = []crossLayerOverlap{
	{
		A: TileLayerFloor, B: TileLayerProps, Char: 'W',
		NameA: "FloorDeepWater", NameB: "TileWell",
		Reason: "deep-water floor tile and the village well prop happen to share the 'W' mnemonic; layers are dispatched independently.",
	},
	{
		A: TileLayerDecor, B: TileLayerProps, Char: 'A',
		NameA: "DecorArchway", NameB: "TileSarcophagus",
		Reason: "archway-anchor decor and stone sarcophagus prop share the 'A' mnemonic.",
	},
	{
		A: TileLayerDecor, B: TileLayerProps, Char: 'L',
		NameA: "DecorLeafPile", NameB: "TileTable",
		Reason: "leaf-pile decor and dungeon table prop share the 'L' mnemonic (L for Leaf and L for table-Leg).",
	},
	{
		A: TileLayerDecor, B: TileLayerFloor, Char: 'c',
		NameA: "DecorCandle", NameB: "FloorCobble",
		Reason: "candle decor and cobblestone floor tile share the 'c' mnemonic.",
	},
	{
		A: TileLayerDecor, B: TileLayerFloor, Char: 'i',
		NameA: "DecorBootprints", NameB: "FloorSnow",
		Reason: "bootprint decor and snow floor tile share the 'i' mnemonic — bootprints on snow happens to be a natural pairing.",
	},
	{
		A: TileLayerDecor, B: TileLayerFloor, Char: 'k',
		NameA: "DecorRootCluster", NameB: "FloorDarkGrass",
		Reason: "root-cluster decor and dark-grass floor share the 'k' mnemonic.",
	},
	{
		A: TileLayerDecor, B: TileLayerFloor, Char: 'v',
		NameA: "DecorClover", NameB: "FloorRampSouth",
		Reason: "clover decor and the south-ascending ramp floor tile share 'v' (v reads as a downward chevron); layers dispatch independently.",
	},
}

// assertNoUnregisteredCrossLayerOverlaps panics if any char on >1 layer isn't in
// crossLayerCharOverlaps. Sentinels ('.'/'_'/'#') are skipped (shared by design).
func assertNoUnregisteredCrossLayerOverlaps() {
	// Built from the named sentinel consts so a rename can't desync the skip-set.
	sentinels := map[byte]struct{}{
		TileOpen: {}, DecorEmpty: {}, TileRock: {},
	}
	// chars[c] -> layers where it's a registered tile.
	chars := make(map[byte][]TileLayer)
	for layer, labels := range tileLabelTable {
		for c := range labels {
			if _, sentinel := sentinels[c]; sentinel {
				continue
			}
			chars[c] = append(chars[c], layer)
		}
	}
	registered := make(map[byte]struct{}, len(crossLayerCharOverlaps))
	for _, o := range crossLayerCharOverlaps {
		registered[o.Char] = struct{}{}
	}
	for c, layers := range chars {
		if len(layers) < 2 {
			continue
		}
		if _, ok := registered[c]; ok {
			continue
		}
		panic("core: cross-layer char '" + string(c) + "' is shared by multiple layers but not registered in crossLayerCharOverlaps — add an entry documenting why this is deliberate")
	}
}

// TileLabel returns a short name for a tile char on a layer; sentinels → "",
// unknown → "?". Table-driven from tileLabelTable.
func TileLabel(layer TileLayer, c byte) string {
	if labels, ok := tileLabelTable[layer]; ok {
		if label, ok := labels[c]; ok {
			return label
		}
	}
	return "?"
}

package editor

import (
	"crawler/internal/app/core"
	"crawler/internal/app/core/mapfile"
	"fmt"
	"os"
	"slices"
	"strconv"
)

// activeFootprint returns the multi-tile footprint of the active brush,
// or nil if the current brush is single-tile. Props anchors check
// core.PropFootprint; decor anchors check core.DecorFootprint. Used by
// the hover preview and apply paths so authors see the footprint shape
// before placement and don't have to drop tail tiles by hand.
func activeFootprint(s *State) []core.MultiTileOffset {
	b := s.activeBrush()
	switch s.layer {
	case LayerProps:
		return core.PropFootprint(b.Char)
	case LayerDecor:
		return core.DecorFootprint(b.Char)
	}
	return nil
}

// footprintPlaceable reports whether the active brush's footprint fits
// at the (anchor) cell — every footprint cell must be in-bounds, not a
// wall, not occupied by another prop (for the props layer), and not the
// player start. The hover preview tints red when this is false so the
// author sees the click will be refused.
func footprintPlaceable(s *State, x, z int, footprint []core.MultiTileOffset) bool {
	for _, off := range footprint {
		fx, fz := x+off.DX, z+off.DZ
		if !s.area.InBounds(fx, fz) {
			return false
		}
		if s.area.WallAt(fx, fz) {
			return false
		}
		if s.area.StartTileX == fx && s.area.StartTileZ == fz {
			return false
		}
		if s.layer == LayerDecor {
			if ch, ok := cellAt(s.area.Props, fx, fz); ok && core.IsPropChar(ch) {
				return false
			}
		}
	}
	return true
}

// applyTool runs the active layer's selected brush at (x,z). Behavior is
// per-layer: grid layers set the layer's char; entity layer fires the
// chosen placement tool. Painting a wall on an entity-occupied cell auto-
// clears the entity; painting a prop on a wall is refused.
func applyTool(s *State, x, z int) {
	if !s.area.InBounds(x, z) {
		return
	}
	// Painting an invisible layer reads as a dead tool — if the layer being
	// edited is hidden (its eye toggled off, or another layer soloed), reveal
	// it so the stroke is actually visible. Idempotent; cheap per cell.
	s.layerHidden[s.layer] = false
	brush := s.activeBrush()
	switch s.layer {
	case LayerWalls:
		applyWallBrush(s, x, z, brush.Char)
	case LayerFloor:
		setLayerCell(&s.area.Floor, x, z, brush.Char)
	case LayerDecor:
		applyDecorBrush(s, x, z, brush.Char)
	case LayerProps:
		applyPropBrush(s, x, z, brush.Char)
	case LayerCeiling:
		setLayerCell(&s.area.Ceiling, x, z, brush.Char)
	case LayerElevation:
		// brush.Char is '0'+editLevel (activeBrush rewrites it), so this
		// stamps the height selector's current level onto the cell.
		setLayerCell(&s.area.Elevation, x, z, brush.Char)
	case LayerEntities:
		applyEntityBrush(s, x, z, brush.Entity)
		return // entity branch sets dirty itself when it lands
	default:
		panic("editor: applyTool missing case for layer — add it here, in eraseAt, and in activeGrid")
	}
	// Floors mode ("treat every level as its own floor"): a content paint also
	// lifts the tile to the active level, so picking a level and painting
	// builds that floor without hand-stamping the Elevation digit. Shared with
	// the flood-fill and fill-all paths via stampActiveLevel so all three
	// content-paint entry points honor the lens identically.
	stampActiveLevel(s, x, z)
	s.dirty = true
}

// layerStampsActiveLevel reports whether painting on `layer` should lift the
// tile to the active edit level. Floor / decor / props / ceiling sit on (or
// over) a floor, so painting them defines that floor's level. WALLS do NOT:
// a wall is a vertical structure, and re-stamping its tile's level on paint
// silently moved tiles between levels — under a fat brush it dropped a raised
// neighbour to the active level, which read as "painting a wall erased the
// walls on another level." Elevation sets the level itself; Entities have none.
func layerStampsActiveLevel(layer Layer) bool {
	switch layer {
	case LayerElevation, LayerEntities, LayerWalls:
		return false
	}
	return true
}

// stampActiveLevel lifts tile (x,z) to the active level — the single "a content
// paint builds the active floor" step shared by applyTool, floodFill, and
// fillEntireLayer so the three paths can't drift. The levels model is now
// ALWAYS on (Photoshop-style), so every floor-defining content paint targets
// the active level. Gated by layerStampsActiveLevel (walls/elevation/entities
// are exempt).
func stampActiveLevel(s *State, x, z int) {
	if !layerStampsActiveLevel(s.layer) {
		return
	}
	if !s.area.InBounds(x, z) {
		return
	}
	setLayerCell(&s.area.Elevation, x, z, core.ElevationChar(s.editLevel))
}

func applyWallBrush(s *State, x, z int, c byte) {
	turningWall := c == core.TileRock
	if turningWall && s.area.StartTileX == x && s.area.StartTileZ == z {
		s.flash("Move the player start before walling its tile")
		return
	}
	setLayerCell(&s.area.Walls, x, z, c)
	if turningWall {
		// Walls and props/decor/entities can't co-exist — wall wins.
		setLayerCell(&s.area.Props, x, z, core.TilePropEmpty)
		setLayerCell(&s.area.Decor, x, z, core.DecorAuto)
		removeAllEntitiesAt(&s.area, x, z)
	}
}

func applyDecorBrush(s *State, x, z int, c byte) {
	// Multi-tile decor anchor: validate the whole footprint fits and is
	// authorable before committing any cell, then auto-paint the tail
	// chars so the author doesn't have to drop them by hand.
	if footprint := core.DecorFootprint(c); footprint != nil {
		tail := core.DecorFootprintTail(c)
		for _, off := range footprint {
			fx, fz := x+off.DX, z+off.DZ
			if !s.area.InBounds(fx, fz) {
				s.flash("Footprint extends off the map")
				return
			}
			if s.area.WallAt(fx, fz) {
				s.flash("Footprint cell is a wall")
				return
			}
			if ch, _ := cellAt(s.area.Props, fx, fz); core.IsPropChar(ch) {
				s.flash("Footprint cell holds a prop")
				return
			}
			if s.area.StartTileX == fx && s.area.StartTileZ == fz {
				s.flash("Footprint cell holds the player start")
				return
			}
		}
		for _, off := range footprint {
			fx, fz := x+off.DX, z+off.DZ
			ch := tail
			if off.DX == 0 && off.DZ == 0 {
				ch = c
			}
			setLayerCell(&s.area.Decor, fx, fz, ch)
		}
		return
	}
	if s.area.WallAt(x, z) {
		s.flash("Decor needs an open cell")
		return
	}
	if ch, _ := cellAt(s.area.Props, x, z); core.IsPropChar(ch) {
		s.flash("Decor cell is occupied by a prop")
		return
	}
	if s.area.StartTileX == x && s.area.StartTileZ == z {
		s.flash("Cell holds the player start")
		return
	}
	setLayerCell(&s.area.Decor, x, z, c)
}

// clearPropCell clears the prop at (x,z). If that cell holds a multi-tile prop
// ANCHOR, the whole footprint — including the auto-painted tail cells — is
// cleared, so erasing a multi-tile prop by its anchor doesn't strand orphaned
// tail glyphs on the neighbouring cells. A single-tile prop (or already-empty
// cell) just clears the one cell.
func clearPropCell(a *core.AreaDefinition, x, z int) {
	if !a.InBounds(x, z) {
		return
	}
	propCh, _ := cellAt(a.Props, x, z)
	if footprint := core.PropFootprint(propCh); footprint != nil {
		for _, off := range footprint {
			fx, fz := x+off.DX, z+off.DZ
			if a.InBounds(fx, fz) {
				setLayerCell(&a.Props, fx, fz, core.TilePropEmpty)
			}
		}
		return
	}
	setLayerCell(&a.Props, x, z, core.TilePropEmpty)
}

func applyPropBrush(s *State, x, z int, c byte) {
	if c == core.TilePropEmpty {
		clearPropCell(&s.area, x, z)
		return
	}
	// Multi-tile prop anchor: validate the whole footprint fits and is
	// free, then auto-paint the tail char into the other footprint cells
	// so the author doesn't have to place each tile manually. If any
	// footprint cell is blocked, refuse the placement entirely (no
	// partial commits).
	if footprint := core.PropFootprint(c); footprint != nil {
		tail := core.PropFootprintTail(c)
		for _, off := range footprint {
			fx, fz := x+off.DX, z+off.DZ
			if !s.area.InBounds(fx, fz) {
				s.flash("Footprint extends off the map")
				return
			}
			if s.area.WallAt(fx, fz) {
				s.flash("Footprint cell is a wall")
				return
			}
			if s.area.StartTileX == fx && s.area.StartTileZ == fz {
				s.flash("Footprint cell holds the player start")
				return
			}
		}
		for _, off := range footprint {
			fx, fz := x+off.DX, z+off.DZ
			ch := tail
			if off.DX == 0 && off.DZ == 0 {
				ch = c
			}
			setLayerCell(&s.area.Props, fx, fz, ch)
			setLayerCell(&s.area.Decor, fx, fz, core.DecorAuto)
			removeAllEntitiesAt(&s.area, fx, fz)
		}
		return
	}
	if s.area.WallAt(x, z) {
		s.flash("Props need an open cell (remove the wall first)")
		return
	}
	if s.area.StartTileX == x && s.area.StartTileZ == z {
		s.flash("Cell holds the player start")
		return
	}
	setLayerCell(&s.area.Props, x, z, c)
	// A prop occupies the floor square; auto-clear any decor on it
	// and any pack / chest / door that would now be inside the prop.
	setLayerCell(&s.area.Decor, x, z, core.DecorAuto)
	removeAllEntitiesAt(&s.area, x, z)
}

func applyEntityBrush(s *State, x, z int, kind entityKind) {
	if !s.area.InBounds(x, z) {
		return
	}
	// Clear runs before the wall / prop guards so the author can erase a
	// stranded pack from a tile that's since become un-placeable (e.g. a
	// wall was painted over it). Without this, "Clear" would refuse on
	// the very tile most likely to need cleaning.
	if kind == entityClear {
		clearEntitiesAt(s, x, z)
		return
	}
	// Player start carries its own COMPLETE rule set (no wall / prop / deep
	// water — anything that would soft-lock the player on spawn), routed
	// through the shared startBlockers so it matches the right-click "Move
	// start here" path exactly. Handled before the generic entity guard so
	// its wall/prop wording isn't pre-empted by the looser "Entities need an
	// open cell" message.
	if kind == entityPlayerStart {
		if msg := firstBlocker(startBlockers(&s.area, x, z)...); msg != "" {
			s.flash(msg)
			return
		}
		s.area.StartTileX = x
		s.area.StartTileZ = z
		s.dirty = true
		return
	}
	if s.area.WallAt(x, z) {
		s.flash("Entities need an open cell")
		return
	}
	if ch, _ := cellAt(s.area.Props, x, z); core.IsPropChar(ch) {
		s.flash("Cell is occupied by a prop")
		return
	}
	brush := s.activeBrush()
	switch kind {
	case entityAddEnemy:
		addPackMember(s, x, z, brush.EnemyKind)
	case entityPlaceChest:
		placeChestAt(s, x, z)
	case entityPlaceDoor:
		placeDoorAt(s, x, z)
	case entityPlaceCrystal:
		placeCrystalAt(s, x, z)
	}
}

// blockerCheck is one named "is this tile illegal for the entity being
// placed?" predicate. blocker helpers below build these so the three
// placement paths (door, chest, pack) read as a flat list of rules
// rather than inlined if-flash-return ladders.
type blockerCheck struct {
	fail bool
	msg  string
}

// firstBlocker returns the first failing check's message, or "" when
// the tile is fine. Order matters — list start / wall / prop / floor /
// other-entity in the order the player would naturally encounter them
// so the flash always names the most obvious obstacle.
func firstBlocker(checks ...blockerCheck) string {
	for _, c := range checks {
		if c.fail {
			return c.msg
		}
	}
	return ""
}

// Named tile-blocker predicates. Each captures one rule + its user-
// facing message so placement helpers don't re-spell either. `noun`
// is "Door" / "Chest" / "Pack" — surfaces in messages where the
// entity word makes the cause obvious to the player.
func blkStart(a *core.AreaDefinition, x, z int) blockerCheck {
	return blockerCheck{a.StartTileX == x && a.StartTileZ == z, "Cell holds the player start"}
}
func blkWall(a *core.AreaDefinition, x, z int, noun string) blockerCheck {
	return blockerCheck{a.WallAt(x, z), noun + " needs an open cell (remove the wall first)"}
}
func blkProp(a *core.AreaDefinition, x, z int) blockerCheck {
	ch, _ := cellAt(a.Props, x, z)
	return blockerCheck{core.IsPropChar(ch), "Cell already holds a prop — clear it first"}
}
func blkDeepWater(a *core.AreaDefinition, x, z int, noun string) blockerCheck {
	ch, _ := cellAt(a.Floor, x, z)
	return blockerCheck{core.IsBlockingFloor(ch), noun + " can't sit on deep water"}
}
func blkPackHere(a *core.AreaDefinition, x, z int) blockerCheck {
	return blockerCheck{core.PackSpawnIndexAt(a.PackSpawns, x, z) >= 0, "Cell already holds a pack — clear it first"}
}
func blkChestHere(a *core.AreaDefinition, x, z int, clear bool) blockerCheck {
	msg := "Cell already holds a chest"
	if clear {
		msg += " — clear it first"
	}
	return blockerCheck{core.ChestSpawnIndexAt(a.ChestSpawns, x, z) >= 0, msg}
}
func blkDoorHere(a *core.AreaDefinition, x, z int) blockerCheck {
	return blockerCheck{core.DoorSpawnIndexAt(a.DoorSpawns, x, z) >= 0, "Cell already holds a door"}
}
func blkCrystalHere(a *core.AreaDefinition, x, z int) blockerCheck {
	return blockerCheck{core.CrystalSpawnIndexAt(a.CrystalSpawns, x, z) >= 0, "Cell already holds a crystal"}
}

// startBlockers is the player-start placement rule: no wall, no prop, no
// deep water (anything that would soft-lock the player on spawn). Shared by
// the entity-brush start tool (applyEntityBrush) AND the right-click "Move
// start here" context action so the two paths can't drift on which tiles are
// legal or on the flash wording. Pass through firstBlocker(startBlockers(...)...).
func startBlockers(a *core.AreaDefinition, x, z int) []blockerCheck {
	return []blockerCheck{
		blkWall(a, x, z, "Player start"),
		blkProp(a, x, z),
		blkDeepWater(a, x, z, "Player start"),
		// Chests and doors block movement onto their tile at runtime, so a
		// start sharing one would soft-lock the spawn / race the door
		// transition. The drag-move-start path already refuses these; keep the
		// entity-brush placement path in lockstep here (one rule, both paths).
		blkChestHere(a, x, z, false),
		blkDoorHere(a, x, z),
	}
}

// placeDoorAt drops a door at (x,z) with a placeholder name and a
// "self" target. The author renames it / sets the target in the
// modalDoorEdit modal opened by clicking the door. Like chests, doors
// can't share a tile with a pack (the runtime would race the
// transition trigger and the encounter start).
// doorPlaceBlockers / chestPlaceBlockers are the shared legality rule sets for
// dropping (or drag-relocating) a door / chest at (x,z). Both the initial
// placement brushes (placeDoorAt / placeChestAt) and the drag-move release path
// run through these so "where can this entity sit?" and its flash wording live
// in one place. When checking a relocation, the dragged entity is still at its
// OLD tile, so blkChestHere/blkDoorHere on the destination correctly flag only a
// DIFFERENT entity already sitting there.
func doorPlaceBlockers(a *core.AreaDefinition, x, z int) []blockerCheck {
	return []blockerCheck{
		blkStart(a, x, z),
		blkWall(a, x, z, "Door"),
		blkProp(a, x, z),
		blkDeepWater(a, x, z, "Door"),
		blkPackHere(a, x, z),
		blkChestHere(a, x, z, true),
		blkDoorHere(a, x, z),
		blkCrystalHere(a, x, z),
	}
}

func chestPlaceBlockers(a *core.AreaDefinition, x, z int) []blockerCheck {
	return []blockerCheck{
		blkStart(a, x, z),
		blkWall(a, x, z, "Chest"),
		blkProp(a, x, z),
		blkDeepWater(a, x, z, "Chest"),
		blkPackHere(a, x, z),
		blkChestHere(a, x, z, false),
		blkCrystalHere(a, x, z),
	}
}

// packPlaceBlockers is the shared legality rule for dropping (or drag-relocating)
// a pack at (x,z): no wall / prop / deep water / start / chest / door / crystal.
// Deliberately omits blkPackHere — the brush path MERGES into an existing pack on
// the tile and the drag path REPLACES it, so a pack already there isn't a blocker.
// Both addPackMember and the dragPack release route through this so the place and
// relocate paths can't drift (they previously did: the brush path skipped deep
// water, the drag path open-coded its own slightly different set).
func packPlaceBlockers(a *core.AreaDefinition, x, z int) []blockerCheck {
	return []blockerCheck{
		blkStart(a, x, z),
		blkWall(a, x, z, "Pack"),
		blkProp(a, x, z),
		blkDeepWater(a, x, z, "Pack"),
		blkChestHere(a, x, z, true),
		blkDoorHere(a, x, z),
		blkCrystalHere(a, x, z),
	}
}

// crystalPlaceBlockers is the legality rule set for dropping a crystal at
// (x,z). Mirrors chestPlaceBlockers — one entity per tile keeps the markers
// legible and lets removeAllEntitiesAt / clearEntitiesAt treat the lists
// uniformly. Crystals are non-blocking in play, but the editor still refuses
// walls / props / deep water so the billboard always has a standable tile (or
// at least a clear one) under it.
func crystalPlaceBlockers(a *core.AreaDefinition, x, z int) []blockerCheck {
	return []blockerCheck{
		blkStart(a, x, z),
		blkWall(a, x, z, "Crystal"),
		blkProp(a, x, z),
		blkDeepWater(a, x, z, "Crystal"),
		blkPackHere(a, x, z),
		blkChestHere(a, x, z, true),
		blkDoorHere(a, x, z),
		blkCrystalHere(a, x, z),
	}
}

func placeDoorAt(s *State, x, z int) {
	a := &s.area
	if msg := firstBlocker(doorPlaceBlockers(a, x, z)...); msg != "" {
		s.flash(msg)
		return
	}
	name := nextDoorName(s.area.DoorSpawns)
	s.area.DoorSpawns = append(s.area.DoorSpawns, core.DoorSpawn{
		TileX:      x,
		TileZ:      z,
		Name:       name,
		TargetMap:  core.SelfMapToken,
		TargetDoor: name,
		// Default the facing to point away from an adjacent wall (the
		// door "affixes" to that wall, opening into the room). Falls back
		// to the map's start facing when the cell has no neighbouring
		// wall. The author can still override it in the door modal.
		Facing: doorFacingForCell(&s.area, x, z),
		Style:  core.DoorStyleBuilding,
	})
	s.dirty = true
}

// doorFacingForCell picks a sensible default facing for a door placed at
// (x, z): away from the first adjacent wall found, so the door reads as
// set into that wall. Falls back to the map's StartFacing when the cell
// has no neighbouring wall. The wall-scan rule itself lives in
// core.FacingAwayFromAdjacentWall (shared with wall-torch orientation).
func doorFacingForCell(a *core.AreaDefinition, x, z int) int {
	if f, ok := core.FacingAwayFromAdjacentWall(*a, x, z); ok {
		return f
	}
	return a.StartFacing
}

// firstUnusedName returns the first `format`-with-N (N from 1 up) whose
// rendered string isn't already in `taken`. Used by the door placeholder
// namer (nextDoorName) to auto-pick a free slot name; the taken-set build
// stays in the caller.
func firstUnusedName(taken map[string]bool, format string) string {
	for i := 1; ; i++ {
		name := fmt.Sprintf(format, i)
		if !taken[name] {
			return name
		}
	}
}

// nextDoorName picks an unused placeholder name for a freshly-placed
// door. "door_1", "door_2", … — the author renames in the modal. The
// name needs to be unique within the map so runtime resolution by
// name is unambiguous.
func nextDoorName(spawns []core.DoorSpawn) string {
	taken := make(map[string]bool, len(spawns))
	for _, sp := range spawns {
		taken[sp.Name] = true
	}
	return firstUnusedName(taken, "door_%d")
}

// removeSpawnsAt drops every spawn sitting on (x, z), generic over the
// pack / chest / door spawn types via core.TileXZ. removeDoorAt /
// removeChestSpawnAt / removePackAt are thin wrappers so a future fourth
// spawn category doesn't need another hand-typed DeleteFunc closure.
func removeSpawnsAt[T core.TileXZ](spawns []T, x, z int) []T {
	return slices.DeleteFunc(spawns, func(sp T) bool {
		tx, tz := sp.Tile()
		return tx == x && tz == z
	})
}

// removeSpawnsWhere drops every spawn whose tile satisfies pred — the
// shape the "outside new bounds after a shrink" and "now sitting on a
// blocked tile after a fill" cleanups share.
func removeSpawnsWhere[T core.TileXZ](spawns []T, pred func(x, z int) bool) []T {
	return slices.DeleteFunc(spawns, func(sp T) bool {
		return pred(sp.Tile())
	})
}

// removeDoorAt drops the door at (x, z) from spawns (if any).
func removeDoorAt(spawns []core.DoorSpawn, x, z int) []core.DoorSpawn {
	return removeSpawnsAt(spawns, x, z)
}

// placeChestAt drops a chest with the default starter loot at (x,z). If
// a chest is already there, refuse (the author can right-click to clear
// and replace). Chests can't share a tile with a pack — the in-game
// interact prompt would race the pack's start-battle trigger.
func placeChestAt(s *State, x, z int) {
	a := &s.area
	// Deep water blocks movement onto the tile, so a chest there would
	// render floating with no way to step onto it (the player can still
	// interact from an adjacent walkable tile, but the visual reads as
	// a bug). Refuse rather than ship the surprise.
	if msg := firstBlocker(chestPlaceBlockers(a, x, z)...); msg != "" {
		s.flash(msg)
		return
	}
	s.area.ChestSpawns = append(s.area.ChestSpawns, core.ChestSpawn{
		TileX: x,
		TileZ: z,
		Items: defaultChestItems(),
	})
	s.dirty = true
}

// defaultChestItems is the seed loot for a freshly-placed chest. Keeps
// the brush a single-click placement — per-chest loot editing is a
// future feature. Returns a fresh slice so the spawn entry doesn't
// alias a package-level template.
func defaultChestItems() []core.ItemKind {
	return []core.ItemKind{core.ItemCheese, core.ItemBatJerky}
}

// removeChestSpawnAt drops the chest at (x, z) from spawns (if any).
func removeChestSpawnAt(spawns []core.ChestSpawn, x, z int) []core.ChestSpawn {
	return removeSpawnsAt(spawns, x, z)
}

// placeCrystalAt drops a healing crystal at (x,z). Refuses (with a flash) when
// the tile is illegal — see crystalPlaceBlockers. Crystals carry no per-tile
// data, so there's nothing more to author after placement.
func placeCrystalAt(s *State, x, z int) {
	a := &s.area
	if msg := firstBlocker(crystalPlaceBlockers(a, x, z)...); msg != "" {
		s.flash(msg)
		return
	}
	s.area.CrystalSpawns = append(s.area.CrystalSpawns, core.CrystalSpawn{TileX: x, TileZ: z})
	s.dirty = true
}

// removeCrystalSpawnAt drops the crystal at (x, z) from spawns (if any).
func removeCrystalSpawnAt(spawns []core.CrystalSpawn, x, z int) []core.CrystalSpawn {
	return removeSpawnsAt(spawns, x, z)
}

// eraseAt is the right-click action. Behavior is per-layer:
//   - Walls / Props : reset cell to '.'
//   - Floor         : reset to FloorAuto
//   - Decor         : reset to DecorAuto
//   - Entities      : remove the pack at this cell, or refuse on the start
func eraseAt(s *State, x, z int) {
	if !s.area.InBounds(x, z) {
		return
	}
	// Reveal the active layer so an erase on a hidden layer isn't an invisible
	// no-op (mirrors applyTool).
	s.layerHidden[s.layer] = false
	switch s.layer {
	case LayerWalls:
		setLayerCell(&s.area.Walls, x, z, core.TileOpen)
	case LayerFloor:
		setLayerCell(&s.area.Floor, x, z, core.FloorAuto)
	case LayerDecor:
		// Right-click on decor means "no scatter here" (DecorEmpty), not
		// "auto-scatter" (DecorAuto). The Auto brush is the explicit "let
		// the renderer pick" affordance; erase should suppress rather
		// than reset-to-random.
		setLayerCell(&s.area.Decor, x, z, core.DecorEmpty)
	case LayerProps:
		clearPropCell(&s.area, x, z)
	case LayerCeiling:
		setLayerCell(&s.area.Ceiling, x, z, core.TileCeilingOpen)
	case LayerElevation:
		// Reset the cell to ground level; if it carried a ramp, clear that
		// too (a ramp with no step is meaningless).
		setLayerCell(&s.area.Elevation, x, z, core.ElevationGround)
		if _, ok := s.area.RampAt(x, z); ok {
			setLayerCell(&s.area.Floor, x, z, core.FloorAuto)
		}
	case LayerEntities:
		if !clearEntitiesAt(s, x, z) {
			return
		}
	default:
		panic("editor: eraseAt missing case for layer — add it here, in applyTool, and in activeGrid")
	}
	s.dirty = true
}

// placeRamp is the smart ramp tool (toolbar Ramp mode). It inspects the
// clicked tile's cardinal neighbors, finds the single axis whose two opposite
// sides differ by exactly one level, and stamps the correct ramp arrow + its
// low level — so the author never has to pick a direction or hand-set the
// digit (the class of bug that the manual approach kept producing). Refuses
// (with a flash) when no neighbor pair forms a clean ±1 step. Snapshots undo
// only on a successful placement.
func placeRamp(s *State, x, z int) {
	if !s.area.InBounds(x, z) {
		return
	}
	// A ramp lives on the FLOOR layer, but a tile with a wall renders the wall
	// and never draws its floor — so a ramp stamped under a wall would be an
	// invisible, non-functional connector. Refuse with feedback.
	if s.area.WallAt(x, z) {
		s.flash("Can't place a ramp on a wall tile — clear the wall first")
		return
	}
	for _, pair := range [2][2]int{{core.North, core.South}, {core.East, core.West}} {
		af, bf := pair[0], pair[1]
		adx, adz := core.FacingVector(af)
		bdx, bdz := core.FacingVector(bf)
		if !s.area.InBounds(x+adx, z+adz) || !s.area.InBounds(x+bdx, z+bdz) {
			continue
		}
		aLvl := s.area.ElevationLevelAt(x+adx, z+adz)
		bLvl := s.area.ElevationLevelAt(x+bdx, z+bdz)
		var ascend, low int
		switch {
		case aLvl == bLvl+1:
			ascend, low = af, bLvl // higher side is toward af
		case bLvl == aLvl+1:
			ascend, low = bf, aLvl
		default:
			continue
		}
		pushUndo(s)
		setLayerCell(&s.area.Floor, x, z, core.RampCharForFacing(ascend))
		setLayerCell(&s.area.Elevation, x, z, core.ElevationChar(low))
		s.dirty = true
		return
	}
	s.flash("Ramp needs one neighbor a level higher on a single axis (set heights first)")
}

// clearEntitiesAt removes the pack, chest, and door at (x,z). Returns
// true when at least one entity was removed (the caller marks dirty
// on true; nothing changed on false). Refuses to clear the player
// start tile — the start is anchored and has to be moved by placing
// a new one elsewhere. Shared by right-click erase and the entityClear
// brush so both paths agree on what "clear" means.
func clearEntitiesAt(s *State, x, z int) bool {
	if s.area.StartTileX == x && s.area.StartTileZ == z {
		s.flash("Player start can't be erased; place it elsewhere instead")
		return false
	}
	before := len(s.area.PackSpawns) + len(s.area.ChestSpawns) + len(s.area.DoorSpawns) + len(s.area.CrystalSpawns)
	removeAllEntitiesAt(&s.area, x, z)
	if len(s.area.PackSpawns)+len(s.area.ChestSpawns)+len(s.area.DoorSpawns)+len(s.area.CrystalSpawns) == before {
		return false
	}
	s.dirty = true
	return true
}

// addPackMember appends a member of `kind` to the pack at (x,z). If no pack
// exists at the tile, creates a fresh pack with the single member.
func addPackMember(s *State, x, z int, kind core.EnemyKind) {
	a := &s.area
	// Shared place/relocate legality (also forbids deep water, which this path
	// used to allow while the drag path refused it).
	if msg := firstBlocker(packPlaceBlockers(a, x, z)...); msg != "" {
		s.flash(msg)
		return
	}
	if idx := core.PackSpawnIndexAt(s.area.PackSpawns, x, z); idx >= 0 {
		core.AppendBuiltinPackMember(&s.area.PackSpawns[idx], kind)
		s.dirty = true
		return
	}
	s.area.PackSpawns = append(s.area.PackSpawns, core.PackSpawn{
		TileX:   x,
		TileZ:   z,
		Members: []core.PackMemberRef{core.BuiltinPackMember(kind)},
	})
	s.dirty = true
}

// removeAllEntitiesAt strips every pack / chest / door spawn whose
// tile equals (x,z). The triplet appears in applyWallBrush,
// applyPropBrush (footprint + single-tile branches), and the runtime
// resize path — five call sites that used to open-code the three
// `removeXAt` calls. Centralized so a future fourth spawn category
// is one edit, not five. Mutates the slices on the passed-in area.
func removeAllEntitiesAt(a *core.AreaDefinition, x, z int) {
	a.PackSpawns = removePackAt(a.PackSpawns, x, z)
	a.ChestSpawns = removeChestSpawnAt(a.ChestSpawns, x, z)
	a.DoorSpawns = removeDoorAt(a.DoorSpawns, x, z)
	a.CrystalSpawns = removeCrystalSpawnAt(a.CrystalSpawns, x, z)
}

func removePackAt(packs []core.PackSpawn, x, z int) []core.PackSpawn {
	return removeSpawnsAt(packs, x, z)
}

// packSpawnLeaderKind picks the kind to render as a pack's field icon —
// the highest-Tier member, ties broken by member order. Delegates to
// core.PackLeaderKind so editor and runtime share one leader rule.
func packSpawnLeaderKind(a core.AreaDefinition, sp core.PackSpawn) core.EnemyKind {
	return core.PackSpawnLeaderKind(a, sp)
}

// setLayerCell mutates the byte at (x,z) inside one of the area's layer
// grids. Layer slices are addressed by pointer so we can write through
// without each caller threading a reference. Callers also flag
// reachability dirty after a layer mutation so the badge refreshes —
// the per-cell write itself doesn't know the State, so the flag flip is
// at every applyTool/eraseAt/floodFill/paintRect site that follows.
func setLayerCell(layer *[]string, x, z int, b byte) {
	row := []byte((*layer)[z])
	row[x] = b
	(*layer)[z] = string(row)
}

// copySelection snapshots the active marquee region (all six grid layers) into
// the clipboard for paste. The transform lives in core.CopyRegion; this is the
// editor glue (selection state + a flash).
func copySelection(s *State) {
	if !s.selActive {
		s.flash("Select a region first (Select tool), then Ctrl+C")
		return
	}
	s.clipboard = core.CopyRegion(&s.area, s.selX0, s.selZ0, s.selX1, s.selZ1)
	if s.clipboard.Empty() {
		s.flash("Nothing to copy")
		return
	}
	s.flash(fmt.Sprintf("Copied %d×%d region — Ctrl+V to paste at the cursor", s.clipboard.W, s.clipboard.H))
}

// pasteSelection stamps the clipboard with its top-left at (atX,atZ) under one
// undo step (pushUndo also flags reachability + bumps the content epoch). No-op
// off-map or with an empty clipboard.
func pasteSelection(s *State, atX, atZ int) {
	if s.clipboard.Empty() {
		s.flash("Clipboard empty — select a region and Ctrl+C first")
		return
	}
	if !s.area.InBounds(atX, atZ) {
		s.flash("Hover over the map, then Ctrl+V to paste at the cursor")
		return
	}
	pushUndo(s)
	s.area.PasteRegion(s.clipboard, atX, atZ)
	s.dirty = true
	s.flash(fmt.Sprintf("Pasted %d×%d region", s.clipboard.W, s.clipboard.H))
}

// clearSelection drops the active marquee selection. It deliberately leaves the
// clipboard intact (so a cross-map copy→paste still works) — call it when the
// area is replaced (New / Open) or resized, where the selection's tile bounds
// would otherwise point at coordinates that no longer match the new map (and
// could render an outline off the smaller/blank grid). A same-map paint undo
// keeps its selection, so this is NOT called from undoOne / redoOne.
func clearSelection(s *State) {
	s.selActive = false
}

// pushUndo snapshots the current area before a mutation. Any new mutation
// invalidates the redo stack — standard text-editor undo semantics.
// Capped at undoLimit to bound memory.
//
// On trim we shift the window down IN PLACE (copy + reslice) and nil out the
// vacated tail slots so their AreaDefinitions (string rows + spawns) are
// released for GC. Reslicing alone would advance the header but leave the
// trimmed-off head snapshots pinned by the backing array; an explicit nil of
// the freed slots releases them without allocating a fresh array every stroke
// (which the old make+copy did, churning ~17KB per stroke once at the cap).
func pushUndo(s *State) {
	commitUndoSnapshot(s, core.CloneArea(s.area))
}

// commitUndoSnapshot banks `before` (the pre-mutation area) onto the undo
// stack, invalidates the cached reachability warnings, and clears the redo
// stack. pushUndo is the snapshot-the-current-state-now wrapper used by the
// one-shot mutations; strokePaint hands in a snapshot it captured at stroke
// start so a multi-cell drag banks ONE undo step covering the whole stroke —
// and only when the stroke actually changed something.
//
// Every forward mutation routes through here, so it's the one place to
// invalidate the reachability cache (see State.ReachabilityWarnings).
func commitUndoSnapshot(s *State, before core.AreaDefinition) {
	s.reachValid = false
	s.contentEpoch++
	s.undo = append(s.undo, before)
	if excess := len(s.undo) - undoLimit; excess > 0 {
		// Shift the kept window to the front in place, then release the now-
		// duplicated tail slots so the trimmed-off snapshots can be collected.
		copy(s.undo, s.undo[excess:])
		for i := undoLimit; i < len(s.undo); i++ {
			s.undo[i] = core.AreaDefinition{}
		}
		s.undo = s.undo[:undoLimit]
	}
	s.redo = nil
}

func undoOne(s *State) {
	if len(s.undo) == 0 {
		s.flash("Nothing to undo")
		return
	}
	last := s.undo[len(s.undo)-1]
	s.undo = s.undo[:len(s.undo)-1]
	s.redo = append(s.redo, core.CloneArea(s.area))
	s.area = last
	s.reachValid = false // area replaced — recompute reachability lazily
	s.contentEpoch++
	// Stepping back to a snapshot that matches the on-disk baseline should
	// drop the dirty marker — don't pester the user with the unsaved-changes
	// modal if their working state is identical to what's on disk.
	s.dirty = !core.AreaContentEqual(s.area, s.baseline)
}

func redoOne(s *State) {
	if len(s.redo) == 0 {
		s.flash("Nothing to redo")
		return
	}
	last := s.redo[len(s.redo)-1]
	s.redo = s.redo[:len(s.redo)-1]
	s.undo = append(s.undo, core.CloneArea(s.area))
	s.area = last
	s.reachValid = false // area replaced — recompute reachability lazily
	s.contentEpoch++
	s.dirty = !core.AreaContentEqual(s.area, s.baseline)
}

// resize grows or shrinks every layer to (w,h). New cells default to the
// layer's blank value (walls border-only, others auto). Player start and
// enemy spawns outside the new bounds are clamped (start) or removed
// (spawns).
func resize(s *State, w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	// Floor at MinMapDimension so the border walls can't consume the
	// whole playable interior, and ceiling at MaxMapDimension so a
	// runaway +-spam can't grow the area past what the renderer was
	// tested on. Both edges share core.ClampMapDimension with the
	// metadata text input and the new-map dialog so the rules can't
	// drift per call site.
	if w < core.MinMapDimension {
		s.flash("Map width too small (min " + strconv.Itoa(core.MinMapDimension) + ")")
	}
	if h < core.MinMapDimension {
		s.flash("Map height too small (min " + strconv.Itoa(core.MinMapDimension) + ")")
	}
	w = core.ClampMapDimension(w)
	h = core.ClampMapDimension(h)
	if w == s.area.Width && h == s.area.Height {
		return
	}
	pushUndo(s)
	s.area.Walls = resizeLayer(s.area.Walls, s.area.Width, s.area.Height, w, h, core.TileOpen)
	s.area.Floor = resizeLayer(s.area.Floor, s.area.Width, s.area.Height, w, h, core.FloorAuto)
	s.area.Decor = resizeLayer(s.area.Decor, s.area.Width, s.area.Height, w, h, core.DecorAuto)
	s.area.Props = resizeLayer(s.area.Props, s.area.Width, s.area.Height, w, h, core.TilePropEmpty)
	s.area.Ceiling = resizeLayer(s.area.Ceiling, s.area.Width, s.area.Height, w, h, core.TileCeilingOpen)
	s.area.Elevation = resizeLayer(s.area.Elevation, s.area.Width, s.area.Height, w, h, core.ElevationGround)
	s.area.Width = w
	s.area.Height = h
	clearSelection(s) // bounds changed — a stale marquee could outline off-grid
	// resizeLayer fills new wall cells with plain TileOpen, so a grow leaves
	// the new outer rows/cols walkable to the edge (the doc's "walls
	// border-only" promise was unmet). Re-seal the perimeter ring with
	// TileRock so a resized map always has a complete outer wall, matching
	// blankArea.
	sealWallBorder(&s.area)
	// sealWallBorder just stamped the perimeter ring with rock, so clamp a
	// now-out-of-range start to the last INTERIOR cell (w-2 / h-2), not the
	// border column/row (w-1 / h-1) which is guaranteed wall — landing the
	// start on a blocked tile. Floor at 1 for a degenerate tiny map.
	// (reachabilityWarnings still flags an interior cell the author walled off.)
	if s.area.StartTileX >= w-1 {
		s.area.StartTileX = w - 2
	}
	if s.area.StartTileX < 1 {
		s.area.StartTileX = 1
	}
	if s.area.StartTileZ >= h-1 {
		s.area.StartTileZ = h - 2
	}
	if s.area.StartTileZ < 1 {
		s.area.StartTileZ = 1
	}
	packsBefore, chestsBefore, doorsBefore := len(s.area.PackSpawns), len(s.area.ChestSpawns), len(s.area.DoorSpawns)
	crystalsBefore := len(s.area.CrystalSpawns)
	s.area.PackSpawns = removePackSpawnsOutside(s.area.PackSpawns, w, h)
	s.area.ChestSpawns = removeChestSpawnsOutside(s.area.ChestSpawns, w, h)
	s.area.DoorSpawns = removeDoorSpawnsOutside(s.area.DoorSpawns, w, h)
	s.area.CrystalSpawns = removeCrystalSpawnsOutside(s.area.CrystalSpawns, w, h)
	// removeXOutside only drops spawns PAST the new bounds. A shrink can also
	// leave a spawn on the tile that just BECAME the border ring (in-bounds, so
	// kept above) — sealWallBorder then stamps a wall over it, burying a chest/
	// door in a wall where it's unreachable and (unlike a pack) doesn't
	// self-relocate at runtime. pruneBlockedSpawns drops any spawn now on a
	// blocked tile (the sealed border included), closing that gap; no-op when
	// nothing's buried.
	pruneBlockedSpawns(&s.area)
	// A shrink silently drops spawns past the new bounds or walled by them
	// (undoable, but the author should know). Flash a count of what fell off so
	// it's not a quiet data loss.
	dropped := (packsBefore - len(s.area.PackSpawns)) + (chestsBefore - len(s.area.ChestSpawns)) + (doorsBefore - len(s.area.DoorSpawns)) + (crystalsBefore - len(s.area.CrystalSpawns))
	if dropped > 0 {
		s.flash(fmt.Sprintf("Resize dropped %d spawn(s) outside or walled by the new bounds", dropped))
	}
	s.dirty = true
}

// removeDoorSpawnsOutside drops door entries whose tile sits past the
// new bounds after a shrink. Mirrors removeChestSpawnsOutside.
func removeDoorSpawnsOutside(spawns []core.DoorSpawn, w, h int) []core.DoorSpawn {
	return removeSpawnsWhere(spawns, func(x, z int) bool { return x >= w || z >= h })
}

// removeCrystalSpawnsOutside drops crystal entries whose tile sits past the
// new bounds after a shrink. Mirrors removeDoorSpawnsOutside.
func removeCrystalSpawnsOutside(spawns []core.CrystalSpawn, w, h int) []core.CrystalSpawn {
	return removeSpawnsWhere(spawns, func(x, z int) bool { return x >= w || z >= h })
}

// removePackSpawnsOutside drops pack entries whose tile sits past the new
// bounds after a shrink. Mirrors removeChestSpawnsOutside / removeDoorSpawnsOutside
// so all three resize-prune paths share the generic removeSpawnsWhere helper.
func removePackSpawnsOutside(spawns []core.PackSpawn, w, h int) []core.PackSpawn {
	return removeSpawnsWhere(spawns, func(x, z int) bool { return x >= w || z >= h })
}

// removeChestSpawnsOutside drops chest entries whose tile sits past
// the new bounds.
func removeChestSpawnsOutside(spawns []core.ChestSpawn, w, h int) []core.ChestSpawn {
	return removeSpawnsWhere(spawns, func(x, z int) bool { return x >= w || z >= h })
}

// sealWallBorder stamps TileRock around the outer ring of the walls layer,
// leaving the interior untouched. Called after resize so a grown/shrunk map
// always carries a complete outer wall ring — resizeLayer fills new wall
// cells with plain TileOpen, which would otherwise leave the new edge
// walkable to the boundary. Matches blankArea's perimeter rule.
func sealWallBorder(a *core.AreaDefinition) {
	for z := 0; z < a.Height && z < len(a.Walls); z++ {
		row := []byte(a.Walls[z])
		for x := 0; x < a.Width && x < len(row); x++ {
			if x == 0 || z == 0 || x == a.Width-1 || z == a.Height-1 {
				row[x] = core.TileRock
			}
		}
		a.Walls[z] = string(row)
	}
}

// resizeLayer copies an old WxH grid into a new W'xH' grid, padding the
// extra cells with `fill`. Old cells outside the new bounds are dropped.
func resizeLayer(old []string, oldW, oldH, newW, newH int, fill byte) []string {
	rows := make([]string, newH)
	for z := 0; z < newH; z++ {
		buf := make([]byte, newW)
		for x := 0; x < newW; x++ {
			if z < oldH && z < len(old) && x < oldW && x < len(old[z]) {
				buf[x] = old[z][x]
			} else {
				buf[x] = fill
			}
		}
		rows[z] = string(buf)
	}
	return rows
}

// saveCurrent writes to the area's existing path. If the area has never been
// saved (Path == ""), open the Save As modal so the user can name it.
func saveCurrent(s *State) {
	if s.area.Path == "" {
		openSaveAsModal(s)
		return
	}
	mf, err := core.MapFileFromArea(s.area)
	if err != nil {
		s.flash("Save failed: " + err.Error())
		return
	}
	if err := mapfile.Save(s.area.Path, mf); err != nil {
		s.flash("Save failed: " + err.Error())
		return
	}
	s.baseline = core.CloneArea(s.area)
	s.dirty = false
	// Saving no longer auto-flashes reachability warnings — reachability is an
	// at-will check now (the Validate modal), not something forced on every
	// save, since "can't reach X" is a design judgment, not a save error.
	s.flash("Saved " + core.MapIDFromPath(s.area.Path))
}

// renameMapFile renames a .map file on disk. Used by the Open modal's R key.
func renameMapFile(oldPath, newID string) (string, error) {
	newID = sanitizeFilename(newID)
	if newID == "" {
		return "", fmt.Errorf("filename required")
	}
	newPath := core.MapPath(newID)
	if newPath == oldPath {
		return oldPath, nil
	}
	if _, err := os.Stat(newPath); err == nil {
		return "", fmt.Errorf("%s already exists", newPath)
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		return "", err
	}
	return newPath, nil
}

// duplicateMapFile copies a .map file under a new name (suffixed _copy,
// _copy2, ...). Used by the Open modal's C key.
func duplicateMapFile(srcPath string) (string, error) {
	id := core.MapIDFromPath(srcPath)
	candidate := id + "_copy"
	for i := 2; ; i++ {
		path := core.MapPath(candidate)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			data, readErr := os.ReadFile(srcPath)
			if readErr != nil {
				return "", readErr
			}
			if err := os.WriteFile(path, data, core.AssetFileMode); err != nil {
				return "", err
			}
			return path, nil
		}
		candidate = fmt.Sprintf("%s_copy%d", id, i)
		if i > 99 {
			return "", fmt.Errorf("too many copies of %s", id)
		}
	}
}

func openModal(s *State, m modalKind) {
	s.modal = m
	s.modalCursor = 0
	switch m {
	case modalOpen:
		// Newest-first ordering: the file the author was last touching
		// (whether they just saved it or pulled it down) is the most
		// likely target of an Open, so it lands at the top of the list.
		paths, _ := mapfile.ListByModTime(core.MapsDir())
		s.modalPaths = paths
	}
}

// openConfirmDirtyModal raises the unsaved-changes prompt, stashing the
// action to run once the user resolves it (Save / Discard / Cancel).
// Every "this would discard edits" entry point — New, Open, Exit to
// title — routes through here so the pending-action + modal pairing
// can't drift across call sites.
func openConfirmDirtyModal(s *State, pending pendingAction) {
	s.pending = pending
	s.modal = modalConfirmDirty
}

// newMap is the user-facing entry: prompts about unsaved changes if the
// current map is dirty, otherwise opens the new-map setup modal so the
// author picks size + default floor before the area is replaced.
func newMap(s *State) {
	if s.dirty {
		openConfirmDirtyModal(s, pendingNew)
		return
	}
	openNewMapModal(s)
}

// openNewMapModal switches into the new-map setup dialog with sensible
// defaults (core.DefaultNewMapDimension square, FloorAuto). The dialog
// commits to performNewMap on confirm.
func openNewMapModal(s *State) {
	s.modal = modalNew
	s.modalNewWidth = core.DefaultNewMapDimension
	s.modalNewHeight = core.DefaultNewMapDimension
	s.modalNewFloor = core.FloorAuto
	s.focus = focusNewWidth
	s.numericBuf = ""
}

func requestOpen(s *State) {
	if s.dirty {
		openConfirmDirtyModal(s, pendingOpen)
		return
	}
	openModal(s, modalOpen)
}

// floodFill replaces the connected region of like-cells around (x,z) with
// b, on the active layer's grid only. 4-connected. No-op if (x,z) already
// holds b. For LayerEntities the operation is a no-op since entities
// aren't grid-stored.
func floodFill(s *State, x, z int, b byte) {
	layer := activeGrid(s)
	if layer == nil {
		return
	}
	if !s.area.InBounds(x, z) {
		return
	}
	// InBounds checks the area's declared Width/Height, which can exceed the
	// actual layer slice lengths for a ragged/partially-built area. Guard the
	// seed read against the real row/col extents — the rest of the codebase
	// reads grids through the bounds-safe layerByteAt; this seed is the one
	// raw index, and an out-of-range sample would panic.
	if z >= len(*layer) || x >= len((*layer)[z]) {
		return
	}
	target := (*layer)[z][x]
	if target == b {
		return // no-op fill (cell already the brush color) — snapshot nothing
	}
	// Snapshot only now that the fill is known to change cells, so a no-op
	// Ctrl+click doesn't push a useless undo step (and clobber the redo stack).
	pushUndo(s)
	var filled [][2]int
	rewriteLayerRows(layer, func(rows [][]byte) {
		stack := [][2]int{{x, z}}
		for len(stack) > 0 {
			p := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			px, pz := p[0], p[1]
			if pz < 0 || pz >= len(rows) || px < 0 || px >= len(rows[pz]) {
				continue
			}
			if rows[pz][px] != target {
				continue
			}
			rows[pz][px] = b
			filled = append(filled, [2]int{px, pz})
			stack = append(stack, [2]int{px + 1, pz}, [2]int{px - 1, pz}, [2]int{px, pz + 1}, [2]int{px, pz - 1})
		}
		// Never wall over the player start tile — the same exemption
		// applyWallBrush (per-cell) and fillEntireLayer enforce. The flood
		// may have filled it (it's inside the connected region); restore it
		// to the region's original value so the player can't spawn in rock.
		if s.layer == LayerWalls && b == core.TileRock {
			sx, sz := s.area.StartTileX, s.area.StartTileZ
			if sz >= 0 && sz < len(rows) && sx >= 0 && sx < len(rows[sz]) && rows[sz][sx] == b {
				rows[sz][sx] = target
			}
		}
	})
	// The flooded region joins the active floor — mirror of the per-cell stamp
	// in applyTool, so Ctrl+click builds a floor the same way a stroke does
	// (levels model is always on now). Walls are exempt (see stampActiveLevel).
	if layerStampsActiveLevel(s.layer) {
		// Batch the elevation lift of the flooded region into one row-set
		// rewrite rather than per-cell stampActiveLevel string rebuilds.
		ch := core.ElevationChar(s.editLevel)
		rewriteLayerRows(&s.area.Elevation, func(rows [][]byte) {
			for _, c := range filled {
				x, z := c[0], c[1]
				if z >= 0 && z < len(rows) && x >= 0 && x < len(rows[z]) {
					rows[z][x] = ch
				}
			}
		})
	}
	// Wall flood that turns cells into '#' nukes any pack/chest/door that
	// fell inside — same cleanup applyWallBrush does per-cell and
	// fillEntireLayer does for a full fill, routed through the shared
	// removeSpawnsWhere over core.TileXZ. (Previously only packs were pruned,
	// leaving chests/doors embedded in the new wall.)
	if s.layer == LayerWalls && b == core.TileRock {
		pruneBlockedSpawns(&s.area)
	}
	s.dirty = true
}

// pruneBlockedSpawns drops any pack / chest / door spawn that now sits on a
// blocked tile — the cleanup a wall flood/fill (and applyWallBrush per-cell)
// owes after turning cells into '#'. Routed through the shared
// removeSpawnsWhere over core.TileXZ so all three entity lists are pruned by
// the same rule.
func pruneBlockedSpawns(a *core.AreaDefinition) {
	blocked := func(x, z int) bool { return a.BlockedAt(x, z) }
	a.PackSpawns = removeSpawnsWhere(a.PackSpawns, blocked)
	a.ChestSpawns = removeSpawnsWhere(a.ChestSpawns, blocked)
	a.DoorSpawns = removeSpawnsWhere(a.DoorSpawns, blocked)
	a.CrystalSpawns = removeSpawnsWhere(a.CrystalSpawns, blocked)
}

// rewriteLayerRows clones the layer's row strings into a mutable
// [][]byte, calls visit on every (x, z) cell, and writes the result
// back. Shared by floodFill / paintRect / fillEntireLayer / anything
// else that needs the "build fresh byte rows then commit" idiom — the
// rows allocation is one alloc per call instead of one per layer
// helper.
func rewriteLayerRows(layer *[]string, visit func(rows [][]byte)) {
	rows := make([][]byte, len(*layer))
	for i, r := range *layer {
		rows[i] = []byte(r)
	}
	visit(rows)
	for i, r := range rows {
		(*layer)[i] = string(r)
	}
}

// fillEntireLayer overwrites every cell on the active grid layer with
// the active brush's character. Entity layer is skipped (would have no
// meaningful "fill"). Player start stays in place even when walls are
// being painted across the whole map — same exemption as paintRect's
// per-cell rule. Pushes a single undo so the action reverts atomically.
func fillEntireLayer(s *State) {
	if s.layer == LayerEntities {
		s.flash("Fill all not supported on Entities layer")
		return
	}
	layer := activeGrid(s)
	if layer == nil {
		return
	}
	brush := s.activeBrush()
	pushUndo(s)
	rewriteLayerRows(layer, func(rows [][]byte) {
		for z := 0; z < s.area.Height && z < len(rows); z++ {
			for x := 0; x < s.area.Width && x < len(rows[z]); x++ {
				if brush.Char == core.TileRock && s.area.StartTileX == x && s.area.StartTileZ == z {
					continue
				}
				rows[z][x] = brush.Char
			}
		}
	})
	// Painting walls everywhere takes packs/chests/doors that fell inside
	// out of play. Same cleanup applyWallBrush does per-cell, routed
	// through the shared removeSpawnsWhere over core.TileXZ.
	if s.layer == LayerWalls && brush.Char == core.TileRock {
		pruneBlockedSpawns(&s.area)
	}
	// A full content fill lifts the whole map to the active floor, consistent
	// with the per-cell / flood-fill stamp. (At level 0 — the default and the
	// common base-laying case — this is a no-op, so flat maps stay flat.)
	// Walls are exempt (see stampActiveLevel) so a wall fill can't flatten the
	// whole height map.
	if layerStampsActiveLevel(s.layer) {
		// Batch the whole-map elevation lift into one row-set rewrite instead
		// of a per-cell stampActiveLevel (each of which rebuilt a full row
		// string) — same write as stampActiveLevel: ElevationChar(editLevel)
		// into every in-bounds cell.
		ch := core.ElevationChar(s.editLevel)
		rewriteLayerRows(&s.area.Elevation, func(rows [][]byte) {
			for z := 0; z < s.area.Height && z < len(rows); z++ {
				for x := 0; x < s.area.Width && x < len(rows[z]); x++ {
					rows[z][x] = ch
				}
			}
		})
	}
	s.dirty = true
	s.flash("Filled " + layerName(s.layer))
}

// centerViewOnTile recenters the editor view so (tx, tz) sits in the
// middle of the grid pane. Used by G "center on start" — handy when a
// pan has drifted the view away from the player spawn on a large map.
// Zoom is left untouched.
func centerViewOnTile(s *State, tx, tz int) {
	if s.rect.cellPx <= 0 {
		return
	}
	// Target world-pixel coord of the centred tile, in the same frame
	// the layout uses (s.rect.gridX/Y already include panX/panY).
	want := s.rect.grid.X + s.rect.grid.Width/2
	wantY := s.rect.grid.Y + s.rect.grid.Height/2
	have, haveY := s.rect.tileCenter(tx, tz)
	s.panX += want - have
	s.panY += wantY - haveY
	s.flash("Centered on " + core.TileCoord(tx, tz))
}

// paintRect paints the active brush's cell value across the rectangle
// bounded by (x0,z0) and (x1,z1) on the active layer. Player start at the
// rect intersection is left in place.
func paintRect(s *State, x0, z0, x1, z1 int) {
	if s.layer == LayerEntities {
		return
	}
	brush := s.activeBrush()
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	if z0 > z1 {
		z0, z1 = z1, z0
	}
	if brushHasMultiTileFootprint(s) {
		// A multi-tile-footprint prop/decor brush can't tile across a
		// rectangle: every cell would re-validate the whole footprint against
		// the neighbours just stamped and refuse, spraying refusal flashes and
		// leaving a mostly-empty rect. Collapse to a single anchor stamp, the
		// same way applyToolBrushed does for the square brush.
		if s.area.InBounds(x0, z0) {
			applyTool(s, x0, z0)
		}
		return
	}
	for z := z0; z <= z1; z++ {
		for x := x0; x <= x1; x++ {
			if !s.area.InBounds(x, z) {
				continue
			}
			if brush.Char == core.TileRock && s.area.StartTileX == x && s.area.StartTileZ == z {
				continue
			}
			applyTool(s, x, z)
		}
	}
}

// paintRectOutline paints only the border cells of the rectangle bounded by
// (x0,z0)-(x1,z1) — the Box tool, for laying room walls without a fill-then-
// hollow. Mirrors paintRect's footprint-collapse + player-start protection.
func paintRectOutline(s *State, x0, z0, x1, z1 int) {
	if s.layer == LayerEntities {
		return
	}
	brush := s.activeBrush()
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	if z0 > z1 {
		z0, z1 = z1, z0
	}
	if brushHasMultiTileFootprint(s) {
		if s.area.InBounds(x0, z0) {
			applyTool(s, x0, z0)
		}
		return
	}
	for z := z0; z <= z1; z++ {
		for x := x0; x <= x1; x++ {
			if x != x0 && x != x1 && z != z0 && z != z1 {
				continue // interior cell — outline only
			}
			if !s.area.InBounds(x, z) {
				continue
			}
			if brush.Char == core.TileRock && s.area.StartTileX == x && s.area.StartTileZ == z {
				continue
			}
			applyTool(s, x, z)
		}
	}
}

// walkLine invokes fn for every grid cell along the Bresenham line from
// (x0,z0) to (x1,z1), inclusive of both endpoints. The one place the stepping
// math lives — shared by the Line tool (paintLine) and the freehand stroke
// interpolation (paintLineBetween, input.go) so they can't drift.
func walkLine(x0, z0, x1, z1 int, fn func(x, z int)) {
	dx := x1 - x0
	if dx < 0 {
		dx = -dx
	}
	dz := z1 - z0
	if dz < 0 {
		dz = -dz
	}
	sx, sz := 1, 1
	if x0 > x1 {
		sx = -1
	}
	if z0 > z1 {
		sz = -1
	}
	err := dx - dz
	cx, cz := x0, z0
	for {
		fn(cx, cz)
		if cx == x1 && cz == z1 {
			return
		}
		e2 := 2 * err
		if e2 > -dz {
			err -= dz
			cx += sx
		}
		if e2 < dx {
			err += dx
			cz += sz
		}
	}
}

// paintLine paints the active brush along the grid line from (x0,z0) to
// (x1,z1) — the Line tool. Mirrors paintRect's footprint-collapse + player-
// start protection. Distinct from input.go's paintLineBetween, which stamps a
// freehand stroke through the per-stroke lazy-undo machinery; this is a
// one-shot committed op (finishDrag pushes the single undo).
func paintLine(s *State, x0, z0, x1, z1 int) {
	if s.layer == LayerEntities {
		return
	}
	brush := s.activeBrush()
	if brushHasMultiTileFootprint(s) {
		if s.area.InBounds(x0, z0) {
			applyTool(s, x0, z0)
		}
		return
	}
	walkLine(x0, z0, x1, z1, func(cx, cz int) {
		if s.area.InBounds(cx, cz) &&
			!(brush.Char == core.TileRock && s.area.StartTileX == cx && s.area.StartTileZ == cz) {
			applyTool(s, cx, cz)
		}
	})
}

// activeGrid returns a pointer to the layer slice the user is editing, or
// nil for layers that don't have a grid (entities). Must stay in sync
// with the layer switches in applyTool and eraseAt — Ctrl+Click flood
// fill reads through this helper and silently no-ops when a grid layer
// is missed, which is exactly how LayerCeiling regressed before this
// case was added.
func activeGrid(s *State) *[]string {
	switch s.layer {
	case LayerWalls:
		return &s.area.Walls
	case LayerFloor:
		return &s.area.Floor
	case LayerDecor:
		return &s.area.Decor
	case LayerProps:
		return &s.area.Props
	case LayerCeiling:
		return &s.area.Ceiling
	case LayerElevation:
		return &s.area.Elevation
	case LayerEntities:
		// Entities have no grid slice — nil is the legitimate "not a grid
		// layer" answer flood-fill checks for. Distinguished from an
		// unhandled NEW layer (the default panic) so the regression that
		// silently dropped LayerCeiling here can't recur.
		return nil
	default:
		panic("editor: activeGrid missing case for layer — add it here, in applyTool, and in eraseAt")
	}
}

// startTileBlocker checks the three "the player can't even spawn"
// conditions and returns the user-facing warning string for the first
// one that fails. Empty string = start tile is fine.
//
// Single source of truth for the playtest gate and the reachability
// warning's start-tile preamble — they used to inline the same three
// checks, and a future blocker (e.g. lava) added to BlockedAt now
// lands both paths automatically.
func startTileBlocker(a core.AreaDefinition) string {
	if a.StartTileZ < 0 || a.StartTileZ >= a.Height ||
		a.StartTileX < 0 || a.StartTileX >= a.Width {
		return "start position is out of bounds"
	}
	if a.BlockedAt(a.StartTileX, a.StartTileZ) {
		return "start tile is blocked (player will spawn inside geometry)"
	}
	if core.ChestSpawnIndexAt(a.ChestSpawns, a.StartTileX, a.StartTileZ) >= 0 {
		return "start tile holds a chest (the chest will be dropped at runtime)"
	}
	return ""
}

// reachabilityWarnings reports playability problems for the area. Empty
// slice means no warnings. Used as a non-blocking check on save.
//
// The BFS treats both wall/prop blockers AND chest tiles as impassable,
// matching the runtime's movement rules — chests block the tile they
// sit on at runtime (explore.startStep refuses to step into a chest),
// so a chest dropped in a chokepoint can sever the map. Without this
// the editor's "all reachable" verdict could ship a map where the
// player physically can't reach packs / chests beyond the chest.
func reachabilityWarnings(a core.AreaDefinition) []string {
	var out []string
	if msg := startTileBlocker(a); msg != "" {
		return []string{msg}
	}
	h := a.Height
	w := a.Width
	// Pre-mark chest tiles as blocked so the BFS treats them like
	// walls. The start tile is exempt above (we already refuse that
	// configuration), so no risk of the BFS having nowhere to begin.
	chestBlock := make([]bool, w*h)
	for _, c := range a.ChestSpawns {
		if c.TileX < 0 || c.TileX >= w || c.TileZ < 0 || c.TileZ >= h {
			continue
		}
		chestBlock[c.TileZ*w+c.TileX] = true
	}
	visited := make([]bool, w*h)
	stack := [][2]int{{a.StartTileX, a.StartTileZ}}
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		px, pz := p[0], p[1]
		if pz < 0 || pz >= h || px < 0 || px >= w {
			continue
		}
		idx := pz*w + px
		if visited[idx] {
			continue
		}
		if a.BlockedAt(px, pz) || chestBlock[idx] {
			continue
		}
		visited[idx] = true
		// Expand only to elevation-CONNECTED neighbors: a cliff (level
		// mismatch with no ramp) blocks, a ramp bridges. On a flat map every
		// StepElevationOK is true, so this is identical to the old 4-way push.
		if a.StepElevationOK(px, pz, core.East) {
			stack = append(stack, [2]int{px + 1, pz})
		}
		if a.StepElevationOK(px, pz, core.West) {
			stack = append(stack, [2]int{px - 1, pz})
		}
		if a.StepElevationOK(px, pz, core.South) {
			stack = append(stack, [2]int{px, pz + 1})
		}
		if a.StepElevationOK(px, pz, core.North) {
			stack = append(stack, [2]int{px, pz - 1})
		}
	}
	// Check reachability against the SNAPPED pack positions, not the
	// authored ones — placePacks relocates pack tiles to the nearest open
	// square at runtime, so a pack authored on a wall lands somewhere else
	// in the actual game. Using snapped coords here means the warning
	// matches what the player will encounter.
	//
	// Drops are now classified: a pack with zero members is the author's
	// own omission ("empty"); a pack with members but no open tile is the
	// map being too crowded ("no open tile"). Surfacing both separately
	// gives the author a faster fix.
	var unreachable, emptyRoster, noOpenTile int
	for _, snap := range core.SnappedSpawnPositions(a) {
		switch snap.Reason {
		case core.SpawnSnapEmptyMembers:
			emptyRoster++
		case core.SpawnSnapNoOpenTile:
			noOpenTile++
		case core.SpawnSnapPlaced:
			x, z := snap.TileX, snap.TileZ
			if x < 0 || z < 0 || x >= w || z >= h {
				unreachable++
				continue
			}
			if !visited[z*w+x] {
				unreachable++
			}
		}
	}
	// Chests are reachable when AT LEAST ONE neighbour is in `visited` —
	// the chest tile itself is marked blocked (#3), so we check the
	// four-neighbour ring for an open approach instead. An "unreachable"
	// chest is one the player can never walk up to and open.
	var unreachableChests int
	for _, c := range a.ChestSpawns {
		if c.TileX < 0 || c.TileX >= w || c.TileZ < 0 || c.TileZ >= h {
			unreachableChests++
			continue
		}
		hasNeighbour := false
		for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			nx, nz := c.TileX+d[0], c.TileZ+d[1]
			if nx < 0 || nx >= w || nz < 0 || nz >= h {
				continue
			}
			if visited[nz*w+nx] {
				hasNeighbour = true
				break
			}
		}
		if !hasNeighbour {
			unreachableChests++
		}
	}
	if unreachable > 0 {
		out = append(out, fmt.Sprintf("%d/%d packs unreachable from start", unreachable, len(a.PackSpawns)))
	}
	if emptyRoster > 0 {
		out = append(out, fmt.Sprintf("%d/%d packs have no members", emptyRoster, len(a.PackSpawns)))
	}
	if noOpenTile > 0 {
		out = append(out, fmt.Sprintf("%d/%d packs can't fit on the map", noOpenTile, len(a.PackSpawns)))
	}
	if unreachableChests > 0 {
		out = append(out, fmt.Sprintf("%d/%d chests unreachable from start", unreachableChests, len(a.ChestSpawns)))
	}
	// Door checks (cheap / local — pairing across maps lives in
	// crossMapDoorWarnings since loading other .map files every frame
	// would burn CPU on a 60 Hz metadata draw). Local issues caught
	// here: door with no destination, door unreachable from start.
	var doorsNoTarget, doorsUnreachable int
	for _, d := range a.DoorSpawns {
		if !d.HasTarget() {
			doorsNoTarget++
		}
		if d.TileX < 0 || d.TileX >= w || d.TileZ < 0 || d.TileZ >= h {
			doorsUnreachable++
			continue
		}
		if !visited[d.TileZ*w+d.TileX] {
			doorsUnreachable++
		}
	}
	if doorsNoTarget > 0 {
		out = append(out, fmt.Sprintf("%d/%d doors missing target (target_map / target_door blank)", doorsNoTarget, len(a.DoorSpawns)))
	}
	if doorsUnreachable > 0 {
		out = append(out, fmt.Sprintf("%d/%d doors unreachable from start", doorsUnreachable, len(a.DoorSpawns)))
	}
	return out
}

// crossMapDoorWarnings validates each door's TargetMap / TargetDoor by
// loading the referenced map file from disk and looking up the named
// door. Expensive (file I/O per unique target map) so it's NOT called
// from the per-frame ReachabilityWarnings — invoked only at playtest
// gating (canPlaytest path) and a future "Validate Doors" topbar
// button. Returns one warning per dangling reference; same-map
// portals (TargetMap == "self" or the local map id) are checked
// against the in-memory area directly.
func crossMapDoorWarnings(a core.AreaDefinition) []string {
	if len(a.DoorSpawns) == 0 {
		return nil
	}
	// Cache loaded destination maps by id so multiple doors pointing
	// at the same target each only trigger one disk read.
	loaded := make(map[string]core.AreaDefinition)
	var out []string
	for _, d := range a.DoorSpawns {
		if !d.HasTarget() {
			continue // already flagged by reachabilityWarnings
		}
		// Same-map portal: just verify the named door exists locally.
		if core.IsSelfPortal(a, d.TargetMap) {
			if !mapHasDoor(a.DoorSpawns, d.TargetDoor) {
				out = append(out, fmt.Sprintf("door %q targets self/%s — no matching door in this map", d.Name, d.TargetDoor))
			}
			continue
		}
		dest, ok := loaded[d.TargetMap]
		if !ok {
			loadedArea, err := core.LoadArea(core.MapPath(d.TargetMap))
			if err != nil {
				out = append(out, fmt.Sprintf("door %q target map %q can't be loaded: %v", d.Name, d.TargetMap, err))
				loaded[d.TargetMap] = core.AreaDefinition{} // negative cache
				continue
			}
			loaded[d.TargetMap] = loadedArea
			dest = loadedArea
		}
		if dest.Width == 0 {
			continue // failed-load negative cache hit
		}
		if !mapHasDoor(dest.DoorSpawns, d.TargetDoor) {
			out = append(out, fmt.Sprintf("door %q targets %s/%s — destination map has no door by that name", d.Name, d.TargetMap, d.TargetDoor))
		}
	}
	return out
}

// dialogWarnings reports authoring problems in the area's dialogs + triggers
// that would silently no-op or dead-end at runtime: broken node / dialog
// references, out-of-bounds tiles, and unregistered foe kinds. Empty slice =
// clean. Surfaced in the Map ▸ Validate report alongside the reachability and
// cross-map-door checks so a conversation that can never resolve is caught at
// author time, not mid-playtest.
func dialogWarnings(a core.AreaDefinition) []string {
	var out []string
	for _, d := range a.Dialogs {
		if len(d.Nodes) == 0 {
			out = append(out, fmt.Sprintf("dialog %q has no nodes", d.ID))
			continue
		}
		if _, ok := d.NodeByID(d.StartNodeID); !ok {
			out = append(out, fmt.Sprintf("dialog %q start node %q not found (runtime falls back to the first node)", d.ID, d.StartNodeID))
		}
		for _, n := range d.Nodes {
			if n.NextNodeID != "" {
				if _, ok := d.NodeByID(n.NextNodeID); !ok {
					out = append(out, fmt.Sprintf("dialog %q node %q → next %q not found", d.ID, n.ID, n.NextNodeID))
				}
			}
			for _, c := range n.Choices {
				if c.NextNodeID != "" {
					if _, ok := d.NodeByID(c.NextNodeID); !ok {
						out = append(out, fmt.Sprintf("dialog %q node %q choice %q → next %q not found", d.ID, n.ID, c.ID, c.NextNodeID))
					}
				}
				for _, cond := range c.Conditions {
					if msg := dialogCondWarning(a, cond); msg != "" {
						out = append(out, fmt.Sprintf("dialog %q node %q choice %q: %s", d.ID, n.ID, c.ID, msg))
					}
				}
			}
		}
	}
	for _, t := range a.Triggers {
		if t.DialogID == "" {
			out = append(out, fmt.Sprintf("trigger %q has no target dialog", t.ID))
		} else if _, ok := core.DialogDefByID(a, t.DialogID); !ok {
			out = append(out, fmt.Sprintf("trigger %q → dialog %q not found", t.ID, t.DialogID))
		}
		switch t.Kind {
		case core.DialogTriggerEnterTile:
			if !a.InBounds(t.TileX, t.TileZ) {
				out = append(out, fmt.Sprintf("trigger %q enter-tile (%d,%d) is out of bounds", t.ID, t.TileX, t.TileZ))
			}
		case core.DialogTriggerFoeKilled:
			if _, ok := core.EnemyInfoOk(t.FoeKind); !ok {
				out = append(out, fmt.Sprintf("trigger %q references an unregistered foe kind", t.ID))
			}
		}
	}
	return out
}

// dialogCondWarning returns a one-line problem with a single choice condition,
// or "" when it's well-formed. Only the world-referencing kinds can be checked
// statically — gold/quest gates reference runtime state the editor can't see.
func dialogCondWarning(a core.AreaDefinition, cond core.DialogChoiceCondition) string {
	switch cond.Kind {
	case core.DialogCondFoeKilled:
		if _, ok := core.EnemyInfoOk(cond.FoeKind); !ok {
			return "foe-killed condition references an unregistered foe kind"
		}
	case core.DialogCondTileVisited:
		if !a.InBounds(cond.TileX, cond.TileZ) {
			return fmt.Sprintf("tile-visited condition (%d,%d) is out of bounds", cond.TileX, cond.TileZ)
		}
	}
	return ""
}

// mapHasDoor reports whether the given spawn list contains a door
// named `name`. Linear scan (door counts are tiny, ~10/map) so a
// map keyed by name isn't worth the allocation per check.
func mapHasDoor(spawns []core.DoorSpawn, name string) bool {
	for _, d := range spawns {
		if d.Name == name {
			return true
		}
	}
	return false
}

// performNewMap replaces the current area with a freshly-blank one of
// the chosen dimensions and default floor tile. Inputs are clamped to
// the same Min/Max ceiling that the resize affordance uses so the new
// map can't be born outside the playable range. Called by modalNew on
// commit and by runPendingAction's pendingNew path after the confirm
// dirty flow.
func performNewMap(s *State, w, h int, floor byte) {
	w = core.ClampMapDimension(w)
	h = core.ClampMapDimension(h)
	s.area = materializeEntranceCrystal(blankArea(w, h, floor))
	s.baseline = core.CloneArea(s.area)
	s.undo = nil
	s.redo = nil
	s.reachValid = false // fresh area — recompute reachability lazily
	s.contentEpoch++
	s.dirty = false
	clearSelection(s) // new map — old selection coords no longer apply
	s.zoom = 1
	s.panX, s.panY = 0, 0
	s.flash("New map")
}

// sanitizeFilename is a thin wrapper over core.SanitizeFilename with
// the editor's "untitled" fallback for all-strippable input — keeps the
// editor's call sites short while the actual character-class contract
// lives in core (shared with audio's user-sound saves).
func sanitizeFilename(name string) string {
	return core.SanitizeFilename(name, "untitled")
}

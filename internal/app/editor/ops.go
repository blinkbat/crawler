package editor

import (
	"crawler/internal/app/core"
	"crawler/internal/app/core/mapfile"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
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
		if s.layer == LayerDecor && core.IsPropChar(s.area.Props[fz][fx]) {
			return false
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
	case LayerEntities:
		applyEntityBrush(s, x, z, brush.Entity)
		return // entity branch sets dirty itself when it lands
	default:
		panic("editor: applyTool missing case for layer — add it here, in eraseAt, and in activeGrid")
	}
	s.dirty = true
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
			if core.IsPropChar(s.area.Props[fz][fx]) {
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
	if core.IsPropChar(s.area.Props[z][x]) {
		s.flash("Decor cell is occupied by a prop")
		return
	}
	if s.area.StartTileX == x && s.area.StartTileZ == z {
		s.flash("Cell holds the player start")
		return
	}
	setLayerCell(&s.area.Decor, x, z, c)
}

func applyPropBrush(s *State, x, z int, c byte) {
	if c == core.TilePropEmpty {
		setLayerCell(&s.area.Props, x, z, core.TilePropEmpty)
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
	if s.area.WallAt(x, z) {
		s.flash("Entities need an open cell")
		return
	}
	if core.IsPropChar(s.area.Props[z][x]) {
		s.flash("Cell is occupied by a prop")
		return
	}
	brush := s.activeBrush()
	switch kind {
	case entityPlayerStart:
		// Player start has the strictest tile rule: no walls, no props,
		// no deep water — anything that would soft-lock the player on
		// spawn. Packs and chests are tolerant of water (packs snap to
		// the nearest walkable tile, chests interact from adjacent), so
		// the floor-blocker rejection lives only on this branch.
		if core.IsBlockingFloor(s.area.Floor[z][x]) {
			s.flash("Player start can't sit on deep water")
			return
		}
		s.area.StartTileX = x
		s.area.StartTileZ = z
		s.dirty = true
	case entityAddEnemy:
		addPackMember(s, x, z, brush.EnemyKind)
	case entityPlaceChest:
		placeChestAt(s, x, z)
	case entityPlaceDoor:
		placeDoorAt(s, x, z)
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
	return blockerCheck{core.IsPropChar(a.Props[z][x]), "Cell already holds a prop — clear it first"}
}
func blkDeepWater(a *core.AreaDefinition, x, z int, noun string) blockerCheck {
	return blockerCheck{core.IsBlockingFloor(a.Floor[z][x]), noun + " can't sit on deep water"}
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

// placeDoorAt drops a door at (x,z) with a placeholder name and a
// "self" target. The author renames it / sets the target in the
// modalDoorEdit modal opened by clicking the door. Like chests, doors
// can't share a tile with a pack (the runtime would race the
// transition trigger and the encounter start).
func placeDoorAt(s *State, x, z int) {
	a := &s.area
	if msg := firstBlocker(
		blkStart(a, x, z),
		blkWall(a, x, z, "Door"),
		blkProp(a, x, z),
		blkDeepWater(a, x, z, "Door"),
		blkPackHere(a, x, z),
		blkChestHere(a, x, z, true),
		blkDoorHere(a, x, z),
	); msg != "" {
		s.flash(msg)
		return
	}
	name := nextDoorName(s.area.DoorSpawns)
	s.area.DoorSpawns = append(s.area.DoorSpawns, core.DoorSpawn{
		TileX:      x,
		TileZ:      z,
		Name:       name,
		TargetMap:  "self",
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

// nextDoorName picks an unused placeholder name for a freshly-placed
// door. "door_1", "door_2", … — the author renames in the modal. The
// name needs to be unique within the map so runtime resolution by
// name is unambiguous.
func nextDoorName(spawns []core.DoorSpawn) string {
	taken := make(map[string]struct{}, len(spawns))
	for _, sp := range spawns {
		taken[sp.Name] = struct{}{}
	}
	for i := 1; ; i++ {
		name := fmt.Sprintf("door_%d", i)
		if _, dup := taken[name]; !dup {
			return name
		}
	}
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
	if msg := firstBlocker(
		blkStart(a, x, z),
		blkWall(a, x, z, "Chest"),
		blkProp(a, x, z),
		blkDeepWater(a, x, z, "Chest"),
		blkPackHere(a, x, z),
		blkChestHere(a, x, z, false),
	); msg != "" {
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

// eraseAt is the right-click action. Behavior is per-layer:
//   - Walls / Props : reset cell to '.'
//   - Floor         : reset to FloorAuto
//   - Decor         : reset to DecorAuto
//   - Entities      : remove the pack at this cell, or refuse on the start
func eraseAt(s *State, x, z int) {
	if !s.area.InBounds(x, z) {
		return
	}
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
		setLayerCell(&s.area.Props, x, z, core.TilePropEmpty)
	case LayerCeiling:
		setLayerCell(&s.area.Ceiling, x, z, core.TileCeilingOpen)
	case LayerEntities:
		if !clearEntitiesAt(s, x, z) {
			return
		}
	default:
		panic("editor: eraseAt missing case for layer — add it here, in applyTool, and in activeGrid")
	}
	s.dirty = true
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
	before := len(s.area.PackSpawns) + len(s.area.ChestSpawns) + len(s.area.DoorSpawns)
	removeAllEntitiesAt(&s.area, x, z)
	if len(s.area.PackSpawns)+len(s.area.ChestSpawns)+len(s.area.DoorSpawns) == before {
		return false
	}
	s.dirty = true
	return true
}

// addPackMember appends a member of `kind` to the pack at (x,z). If no pack
// exists at the tile, creates a fresh pack with the single member.
func addPackMember(s *State, x, z int, kind core.EnemyKind) {
	a := &s.area
	if msg := firstBlocker(
		blkStart(a, x, z),
		blkChestHere(a, x, z, true),
	); msg != "" {
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

// pushUndo snapshots the current area before a mutation. Any new mutation
// invalidates the redo stack — standard text-editor undo semantics.
// Capped at undoLimit to bound memory.
//
// On trim we copy into a fresh backing array rather than reslicing the
// tail. Reslicing would advance the slice header but the original array
// still pins the trimmed-off head snapshots in memory — each snapshot
// holds multiple string-row slices plus PackSpawns, so over a long
// editing session you can accumulate dozens of MB of unreachable-but-
// uncollectable AreaDefinitions. The copy frees them for GC.
func pushUndo(s *State) {
	snap := core.CloneArea(s.area)
	s.undo = append(s.undo, snap)
	if len(s.undo) > undoLimit {
		trimmed := make([]core.AreaDefinition, undoLimit)
		copy(trimmed, s.undo[len(s.undo)-undoLimit:])
		s.undo = trimmed
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
	s.area.Width = w
	s.area.Height = h
	if s.area.StartTileX >= w {
		s.area.StartTileX = w - 1
	}
	if s.area.StartTileZ >= h {
		s.area.StartTileZ = h - 1
	}
	packsBefore, chestsBefore, doorsBefore := len(s.area.PackSpawns), len(s.area.ChestSpawns), len(s.area.DoorSpawns)
	s.area.PackSpawns = slices.DeleteFunc(s.area.PackSpawns, func(sp core.PackSpawn) bool {
		return sp.TileX >= w || sp.TileZ >= h
	})
	s.area.ChestSpawns = removeChestSpawnsOutside(s.area.ChestSpawns, w, h)
	s.area.DoorSpawns = removeDoorSpawnsOutside(s.area.DoorSpawns, w, h)
	// A shrink silently drops spawns past the new bounds (undoable, but the
	// author should know). Flash a count of what fell off so it's not a
	// quiet data loss.
	dropped := (packsBefore - len(s.area.PackSpawns)) + (chestsBefore - len(s.area.ChestSpawns)) + (doorsBefore - len(s.area.DoorSpawns))
	if dropped > 0 {
		s.flash(fmt.Sprintf("Resize dropped %d spawn(s) outside the new bounds", dropped))
	}
	s.dirty = true
}

// removeDoorSpawnsOutside drops door entries whose tile sits past the
// new bounds after a shrink. Mirrors removeChestSpawnsOutside.
func removeDoorSpawnsOutside(spawns []core.DoorSpawn, w, h int) []core.DoorSpawn {
	return removeSpawnsWhere(spawns, func(x, z int) bool { return x >= w || z >= h })
}

// removeChestSpawnsOutside drops chest entries whose tile sits past
// the new bounds.
func removeChestSpawnsOutside(spawns []core.ChestSpawn, w, h int) []core.ChestSpawn {
	return removeSpawnsWhere(spawns, func(x, z int) bool { return x >= w || z >= h })
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
	// Flash reachability warnings FIRST (danger-tinted), then the "Saved"
	// confirmation, so the confirmation is the newest entry and survives
	// the status-log trim even when several warnings fire — the author
	// always sees both "it saved" AND that the map has problems.
	for _, w := range reachabilityWarnings(s.area) {
		s.flashWarn("Warning: " + w)
	}
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
	target := (*layer)[z][x]
	if target == b {
		return // no-op fill (cell already the brush color) — snapshot nothing
	}
	// Snapshot only now that the fill is known to change cells, so a no-op
	// Ctrl+click doesn't push a useless undo step (and clobber the redo stack).
	pushUndo(s)
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
			stack = append(stack, [2]int{px + 1, pz}, [2]int{px - 1, pz}, [2]int{px, pz + 1}, [2]int{px, pz - 1})
		}
	})
	// Wall flood that turns cells into '#' nukes any packs that fell inside.
	if s.layer == LayerWalls && b == core.TileRock {
		s.area.PackSpawns = removeSpawnsWhere(s.area.PackSpawns, func(x, z int) bool { return s.area.BlockedAt(x, z) })
	}
	s.dirty = true
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
		blocked := func(x, z int) bool { return s.area.BlockedAt(x, z) }
		s.area.PackSpawns = removeSpawnsWhere(s.area.PackSpawns, blocked)
		s.area.ChestSpawns = removeSpawnsWhere(s.area.ChestSpawns, blocked)
		s.area.DoorSpawns = removeSpawnsWhere(s.area.DoorSpawns, blocked)
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
		stack = append(stack, [2]int{px + 1, pz}, [2]int{px - 1, pz}, [2]int{px, pz + 1}, [2]int{px, pz - 1})
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
	localMapID := ""
	if a.Path != "" {
		localMapID = core.MapIDFromPath(a.Path)
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
		if d.TargetMap == "self" || d.TargetMap == localMapID {
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
	s.area = blankArea(w, h, floor)
	s.baseline = core.CloneArea(s.area)
	s.undo = nil
	s.redo = nil
	s.dirty = false
	s.zoom = 1
	s.panX, s.panY = 0, 0
	s.flash("New map")
}

func mapStem(path string) string {
	if path == "" {
		return ""
	}
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// sanitizeFilename is a thin wrapper over core.SanitizeFilename with
// the editor's "untitled" fallback for all-strippable input — keeps the
// editor's call sites short while the actual character-class contract
// lives in core (shared with audio's user-sound saves).
func sanitizeFilename(name string) string {
	return core.SanitizeFilename(name, "untitled")
}

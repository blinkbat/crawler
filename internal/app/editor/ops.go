package editor

import (
	"crawler/internal/app/core"
	"crawler/internal/app/core/mapfile"
	"fmt"
	"os"
	"slices"
	"strconv"
)

// activeFootprint returns the active brush's multi-tile footprint (props/decor carry
// one; other layers are single-tile → nil). See layerDefs in layerdef.go.
func activeFootprint(s *State) []core.MultiTileOffset {
	if fp := layerDefs[s.layer].footprint; fp != nil {
		return fp(s.activeBrush().Char)
	}
	return nil
}

// footprintBlocker returns the first cell-level blocker message for the footprint
// anchored at (x,z), or "" if it fits. checkProp gates the "cell holds a prop"
// rule (on for decor, off for the prop brush's own tail cells). Precedence:
// bounds → wall → prop → player start.
func footprintBlocker(s *State, x, z int, footprint []core.MultiTileOffset, checkProp bool) string {
	for _, off := range footprint {
		fx, fz := x+off.DX, z+off.DZ
		if !s.area.InBounds(fx, fz) {
			return "Footprint extends off the map"
		}
		if s.area.WallAt(fx, fz) {
			return "Footprint cell is a wall"
		}
		if checkProp {
			// Per-floor: only a prop on the active floor blocks (another floor's
			// prop in this column is independent — that's the point of per-floor).
			if core.IsPropChar(s.area.PropAt(fx, s.editLevel, fz)) {
				return "Footprint cell holds a prop"
			}
		}
		if s.area.StartTileX == fx && s.area.StartTileZ == fz {
			return "Footprint cell holds the player start"
		}
	}
	return ""
}

// footprintPlaceable reports whether the active brush's footprint fits at (x,z).
// The hover preview tints red when false.
func footprintPlaceable(s *State, x, z int, footprint []core.MultiTileOffset) bool {
	return footprintBlocker(s, x, z, footprint, s.layer == LayerDecor) == ""
}

// applyTool runs the active layer's selected brush at (x,z): grid layers set the
// char, the entity layer fires the placement tool.
func applyTool(s *State, x, z int) {
	if !s.area.InBounds(x, z) {
		return
	}
	// Reveal the layer so a stroke on a hidden layer isn't invisible.
	revealActiveLayer(s)
	brush := s.activeBrush()
	if brush.Erase {
		eraseAt(s, x, z)
		return
	}
	// Per-layer paint — the descriptor's apply owns the stamp-to-level + dirty tail
	// (content layers via paintContentCell; entities set their own conditional dirty).
	layerDefs[s.layer].apply(s, x, z, brush)
}

// layerStampsActiveLevel reports whether painting on layer lifts the tile to the
// active level. Floor/decor/props/ceiling define a floor's level; walls/elevation/
// entities do not (see the stampsLevel field of layerDefs in layerdef.go).
func layerStampsActiveLevel(layer Layer) bool {
	return layerDefs[layer].stampsLevel
}

// stampActiveLevel lifts tile (x,z) to the active level — shared by applyTool,
// floodFill, and fillEntireLayer. Gated by layerStampsActiveLevel.
func stampActiveLevel(s *State, x, z int) {
	if !layerStampsActiveLevel(s.layer) {
		return
	}
	if !s.area.InBounds(x, z) {
		return
	}
	setTileGroundLevel(s, x, z, s.editLevel)
}

// setTileGroundLevel raises column (x,z) to ground level, honoring voxel mode:
// once Solids is materialized the height write MUST go through SetColumnTop or it
// desyncs the ignored Elevation string. Heightfield maps (Solids nil) write
// Elevation directly. Shared by stamp/flood/fill/ramp.
func setTileGroundLevel(s *State, x, z, level int) {
	if len(s.area.Solids) > 0 {
		s.area.SetColumnTop(x, z, level)
		return
	}
	setLayerCell(&s.area.Elevation, x, z, core.ElevationChar(level))
}

// applyFaceBrush sets the tile's cliff-face skin (cosmetic only — does NOT block
// or clear props/decor/entities; the skin shows only where elevation exposes a face).
func applyFaceBrush(s *State, x, z int, c byte) {
	setLayerCell(&s.area.Walls, x, z, c)
}

// applyDecorBrush paints decor at (x,z) and reports whether a placement actually
// landed — false on every refusal (blocked footprint / wall / prop-occupied / player
// start), so paintContentCell won't lift the column's elevation or dirty the map for
// a stroke that only flashed a rejection.
func applyDecorBrush(s *State, x, z int, c byte) bool {
	// Multi-tile decor anchor: validate the whole footprint before committing any
	// cell, then auto-paint the tail chars.
	if footprint := core.DecorFootprint(c); footprint != nil {
		tail := core.DecorFootprintTail(c)
		if msg := footprintBlocker(s, x, z, footprint, true); msg != "" {
			s.flash(msg)
			return false
		}
		for _, off := range footprint {
			fx, fz := x+off.DX, z+off.DZ
			ch := tail
			if off.DX == 0 && off.DZ == 0 {
				ch = c
			}
			setDecorFloor(s, fx, fz, ch)
		}
		return true
	}
	if s.area.WallAt(x, z) {
		s.flash("Decor needs an open cell")
		return false
	}
	// Per-floor: only a prop on the SAME floor blocks decor (a prop on another
	// floor of this column is independent).
	if core.IsPropChar(s.area.PropAt(x, s.editLevel, z)) {
		s.flash("Decor cell is occupied by a prop")
		return false
	}
	if s.area.StartTileX == x && s.area.StartTileZ == z {
		s.flash("Cell holds the player start")
		return false
	}
	setDecorFloor(s, x, z, c)
	return true
}

// clearPropCell clears the prop on the ACTIVE floor at (x,z). A multi-tile prop
// ANCHOR clears its whole footprint (tail cells included) so no orphan tail
// glyphs are stranded. Floor-aware: only the active floor's prop is removed.
func clearPropCell(s *State, x, z int) {
	if !s.area.InBounds(x, z) {
		return
	}
	propCh := s.area.PropForDisplay(x, z, s.editLevel)
	if footprint := core.PropFootprint(propCh); footprint != nil {
		for _, off := range footprint {
			fx, fz := x+off.DX, z+off.DZ
			if s.area.InBounds(fx, fz) {
				setPropFloor(s, fx, fz, core.TilePropEmpty)
			}
		}
		s.area.SetPropYawStep(x, z, -1) // drop the anchor's facing override
		return
	}
	setPropFloor(s, x, z, core.TilePropEmpty)
	s.area.SetPropYawStep(x, z, -1) // no prop → no facing override
}

// applyPropBrush paints a prop at (x,z) and reports whether the map changed — false
// on every refusal (blocked footprint / wall / player start), so paintContentCell
// won't lift the column's elevation or dirty the map for a stroke that only flashed a
// rejection. An empty-char brush erases and counts as a change.
func applyPropBrush(s *State, x, z int, c byte) bool {
	if c == core.TilePropEmpty {
		clearPropCell(s, x, z)
		return true
	}
	// Multi-tile prop anchor: validate the whole footprint, then auto-paint the
	// tail cells. Any blocked cell refuses the whole placement (no partial commits).
	if footprint := core.PropFootprint(c); footprint != nil {
		tail := core.PropFootprintTail(c)
		if msg := footprintBlocker(s, x, z, footprint, false); msg != "" {
			s.flash(msg)
			return false
		}
		for _, off := range footprint {
			fx, fz := x+off.DX, z+off.DZ
			ch := tail
			if off.DX == 0 && off.DZ == 0 {
				ch = c
			}
			setPropFloor(s, fx, fz, ch)
			// A prop occupies its floor square: reset decor + clear entities there.
			setDecorFloor(s, fx, fz, core.DecorAuto)
			removeAllEntitiesAt(&s.area, fx, fz)
		}
		s.area.SetPropYawStep(x, z, s.brushYaw) // facing on the anchor (tails render nothing)
		return true
	}
	if s.area.WallAt(x, z) {
		s.flash("Props need an open cell (remove the wall first)")
		return false
	}
	if s.area.StartTileX == x && s.area.StartTileZ == z {
		s.flash("Cell holds the player start")
		return false
	}
	setPropFloor(s, x, z, c)
	// A prop occupies the floor square (on its floor); reset decor + clear entities.
	setDecorFloor(s, x, z, core.DecorAuto)
	removeAllEntitiesAt(&s.area, x, z)
	s.area.SetPropYawStep(x, z, s.brushYaw) // apply the brush's pending facing
	return true
}

// propYawLabel renders a facing step for the status line: "Auto" or "150°".
func propYawLabel(step int) string {
	if step < 0 {
		return "Auto"
	}
	return strconv.Itoa(int(core.PropYawDegForStep(step))) + "°"
}

// cyclePropYaw advances a facing cursor: -1 (auto) → 0 → 1 → … → last → -1.
func cyclePropYaw(step int) int {
	step++
	if step >= core.PropYawSteps {
		return -1
	}
	return step
}

// rotatePropFacing handles R on the Props layer: over an existing prop it rotates
// that tile's facing (one undo step) and adopts it as the brush facing; otherwise it
// just cycles the pending brush facing new props will inherit.
func rotatePropFacing(s *State) {
	if s.hoverX >= 0 && s.area.PropForDisplay(s.hoverX, s.hoverZ, s.editLevel) != core.TilePropEmpty {
		next := cyclePropYaw(s.area.PropYawStepAt(s.hoverX, s.hoverZ))
		before := core.CloneArea(s.area)
		s.area.SetPropYawStep(s.hoverX, s.hoverZ, next)
		s.brushYaw = next
		if !core.AreaContentEqual(s.area, before) {
			commitUndoSnapshot(s, before)
			s.dirty = true
		}
		s.flash("Prop facing: " + propYawLabel(next))
		return
	}
	s.brushYaw = cyclePropYaw(s.brushYaw)
	s.flash("Brush facing: " + propYawLabel(s.brushYaw))
}

// setTileLevel records the level a placed thing sits on into a per-tile grid
// (PropLevels / DecorLevels): PropLevelAuto when it matches the auto surface
// (keeps the map lean), else an explicit base-36 level.
func setTileLevel(s *State, grid *[]string, x, z, level int) {
	if !s.area.InBounds(x, z) {
		return
	}
	auto := s.area.LowestStandableLevel(x, z)
	if auto < 0 {
		auto = s.area.ElevationLevelAt(x, z)
	}
	c := byte(core.PropLevelAuto)
	if level != auto {
		c = core.ElevationChar(level)
	}
	ensureLevelGrid(grid, s.area.Width, s.area.Height)
	row := []byte((*grid)[z])
	row[x] = c
	(*grid)[z] = string(row)
}

// clearTileLevel resets (x,z) to the auto surface (paired with clearing the prop/
// decor so a removed deck item leaves no stale level).
func clearTileLevel(grid *[]string, x, z int) {
	if z >= 0 && z < len(*grid) && x >= 0 && x < len((*grid)[z]) {
		row := []byte((*grid)[z])
		row[x] = core.PropLevelAuto
		(*grid)[z] = string(row)
	}
}

// ensureLevelGrid allocates a per-tile level grid (all auto) sized to the area
// when it isn't already.
func ensureLevelGrid(grid *[]string, w, h int) {
	sized := len(*grid) == h
	if sized {
		for _, r := range *grid {
			if len(r) != w {
				sized = false
				break
			}
		}
	}
	if sized {
		return
	}
	blank := make([]byte, w)
	for i := range blank {
		blank[i] = core.PropLevelAuto
	}
	out := make([]string, h)
	for z := range out {
		if z < len(*grid) && len((*grid)[z]) == w {
			out[z] = (*grid)[z]
		} else {
			out[z] = string(blank)
		}
	}
	*grid = out
}

// propUsesStack / decorUsesStack report whether a prop/decor paint on the active
// floor of column (x,z) must route through the per-floor stack: a stack already
// exists, OR the column holds content on a DIFFERENT floor (so a legacy single-
// grid write would overwrite it). A pure single-floor column stays on the legacy
// path, so single-floor maps never materialize a stack (byte-identical saves).
func propUsesStack(s *State, x, z int) bool {
	if len(s.area.PropStack) > 0 {
		return true
	}
	lvl := s.area.PropColumnLevel(x, z)
	return lvl >= 0 && lvl != s.editLevel
}

func decorUsesStack(s *State, x, z int) bool {
	if len(s.area.DecorStack) > 0 {
		return true
	}
	lvl := s.area.DecorColumnLevel(x, z)
	return lvl >= 0 && lvl != s.editLevel
}

// layerHasFloorTags reports whether a per-tile level grid carries any non-auto
// tag — an item parked on a raised floor. Flood/fill-all write only the legacy
// char grid (never the level tags), so they must refuse such a map even when no
// stack has materialized yet, mirroring the brush path's propUsesStack routing.
func layerHasFloorTags(grid []string) bool {
	for _, row := range grid {
		for i := 0; i < len(row); i++ {
			if row[i] != core.PropLevelAuto {
				return true
			}
		}
	}
	return false
}

// layerBlocksBulkFill reports whether flood/fill-all must refuse the active layer
// because it uses per-floor prop/decor semantics (a materialized stack OR a
// raised-floor level tag) that a legacy-grid write would make invisible or
// clobber. The brush tools handle floors per-cell; bulk grid writes don't touch
// the stack/level tags. Callers keep their own flash text.
func layerBlocksBulkFill(s *State) bool {
	return (s.layer == LayerProps && (len(s.area.PropStack) > 0 || layerHasFloorTags(s.area.PropLevels))) ||
		(s.layer == LayerDecor && (len(s.area.DecorStack) > 0 || layerHasFloorTags(s.area.DecorLevels)))
}

// liftCellsToActiveLevel raises each listed cell's ground to the active edit level
// after a PAINT flood/fill (mirror of applyTool's per-cell stamp; erase and Walls
// never call this). Voxel maps lift via SetColumnTop; heightfields batch a single
// Elevation row rewrite. Cells out of the backing rows (ragged map) are skipped.
func liftCellsToActiveLevel(s *State, cells [][2]int) {
	if len(s.area.Solids) > 0 {
		for _, c := range cells {
			setTileGroundLevel(s, c[0], c[1], s.editLevel)
		}
		return
	}
	ch := core.ElevationChar(s.editLevel)
	rewriteLayerRows(&s.area.Elevation, func(rows [][]byte) {
		for _, c := range cells {
			x, z := c[0], c[1]
			if z >= 0 && z < len(rows) && x >= 0 && x < len(rows[z]) {
				rows[z][x] = ch
			}
		}
	})
}

// setPropFloor places prop char c on the ACTIVE floor of column (x,z): through
// the per-floor stack when the column is multi-floor, else the legacy single grid
// + PropLevels tag. TilePropEmpty clears the active floor.
func setPropFloor(s *State, x, z int, c byte) {
	if propUsesStack(s, x, z) {
		s.area.SetProp(x, s.editLevel, z, c)
		return
	}
	setLayerCell(&s.area.Props, x, z, c)
	if c == core.TilePropEmpty {
		clearTileLevel(&s.area.PropLevels, x, z)
		return
	}
	setTileLevel(s, &s.area.PropLevels, x, z, s.editLevel)
}

// setDecorFloor is the decor analogue of setPropFloor.
func setDecorFloor(s *State, x, z int, c byte) {
	if decorUsesStack(s, x, z) {
		s.area.SetDecor(x, s.editLevel, z, c)
		return
	}
	setLayerCell(&s.area.Decor, x, z, c)
	if c == core.DecorAuto || c == core.DecorEmpty {
		clearTileLevel(&s.area.DecorLevels, x, z)
		return
	}
	setTileLevel(s, &s.area.DecorLevels, x, z, s.editLevel)
}

// moveStartTo relocates the player start to (x,z) when startBlockers allows, banking
// ONE undo and dirtying on success (flashing the first blocker otherwise). The single
// home for the three move-start paths — entity brush, start-drag, right-click "Move
// start here" — which had drifted: the entity-brush copy skipped the undo snapshot.
// Returns whether the start actually moved.
func moveStartTo(s *State, x, z int) bool {
	if msg := firstBlocker(startBlockers(&s.area, x, z)...); msg != "" {
		s.flash(msg)
		return false
	}
	pushUndo(s)
	s.area.StartTileX = x
	s.area.StartTileZ = z
	s.dirty = true
	return true
}

func applyEntityBrush(s *State, x, z int, kind entityKind) {
	if !s.area.InBounds(x, z) {
		return
	}
	// Clear runs before the wall/prop guards so a stranded pack on a now-
	// unplaceable tile can still be erased.
	if kind == entityClear {
		clearEntitiesAt(s, x, z)
		return
	}
	// Player start has its own rule set (startBlockers — matches the right-click
	// "Move start here" path), handled before the generic guard for its wording.
	if kind == entityPlayerStart {
		moveStartTo(s, x, z)
		return
	}
	if s.area.WallAt(x, z) {
		s.flash("Entities need an open cell")
		return
	}
	if blkProp(&s.area, x, z).fail {
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

// blockerCheck is one "is this tile illegal for the entity?" predicate, so the
// placement paths read as a flat rule list.
type blockerCheck struct {
	fail bool
	msg  string
}

// firstBlocker returns the first failing check's message, or "". Order matters —
// list rules so the flash names the most obvious obstacle.
func firstBlocker(checks ...blockerCheck) string {
	for _, c := range checks {
		if c.fail {
			return c.msg
		}
	}
	return ""
}

// Named tile-blocker predicates, each one rule + its message. `noun` ("Door" /
// "Chest" / "Pack") surfaces in messages where the entity word clarifies the cause.
func blkStart(a *core.AreaDefinition, x, z int) blockerCheck {
	return blockerCheck{a.StartTileX == x && a.StartTileZ == z, "Cell holds the player start"}
}
func blkWall(a *core.AreaDefinition, x, z int, noun string) blockerCheck {
	return blockerCheck{a.WallAt(x, z), noun + " needs an open cell (remove the wall first)"}
}
func blkProp(a *core.AreaDefinition, x, z int) blockerCheck {
	// Column-level: an entity/start can't share a tile with a prop on ANY floor.
	// PropForDisplay returns the legacy single-grid char on a nil-stack map, so
	// this is unchanged for single-floor maps.
	ch := a.PropForDisplay(x, z, 0)
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

// startBlockers is the player-start placement rule (no wall/prop/deep water/
// chest/door — anything that soft-locks spawn). Shared by the start brush and the
// right-click "Move start here" action.
func startBlockers(a *core.AreaDefinition, x, z int) []blockerCheck {
	return []blockerCheck{
		blkWall(a, x, z, "Player start"),
		blkProp(a, x, z),
		blkDeepWater(a, x, z, "Player start"),
		// Chests/doors block their tile at runtime — a shared start soft-locks
		// the spawn / races the transition.
		blkChestHere(a, x, z, false),
		blkDoorHere(a, x, z),
	}
}

// commonEntityBlockers is the prefix every entity-placement rule shares: no player
// start, wall, prop, or deep water on the target tile (noun labels the rejection).
// Each *PlaceBlockers appends its own one-per-tile tail.
func commonEntityBlockers(a *core.AreaDefinition, x, z int, noun string) []blockerCheck {
	return []blockerCheck{
		blkStart(a, x, z),
		blkWall(a, x, z, noun),
		blkProp(a, x, z),
		blkDeepWater(a, x, z, noun),
	}
}

// doorPlaceBlockers / chestPlaceBlockers are the shared legality rule sets for
// dropping or drag-relocating a door / chest. Used by both the placement brushes
// and the drag-move release. On a relocation the dragged entity is still at its
// OLD tile, so blkChestHere/blkDoorHere flag only a DIFFERENT entity already there.
func doorPlaceBlockers(a *core.AreaDefinition, x, z int) []blockerCheck {
	return append(commonEntityBlockers(a, x, z, "Door"),
		blkPackHere(a, x, z),
		blkChestHere(a, x, z, true),
		blkDoorHere(a, x, z),
		blkCrystalHere(a, x, z),
	)
}

func chestPlaceBlockers(a *core.AreaDefinition, x, z int) []blockerCheck {
	return append(commonEntityBlockers(a, x, z, "Chest"),
		blkPackHere(a, x, z),
		blkChestHere(a, x, z, false),
		blkCrystalHere(a, x, z),
	)
}

// packPlaceBlockers is the shared pack place/relocate rule: no wall/prop/deep
// water/start/chest/door/crystal. Omits blkPackHere — the brush merges into an
// existing pack and the drag replaces it, so a pack there isn't a blocker.
func packPlaceBlockers(a *core.AreaDefinition, x, z int) []blockerCheck {
	return append(commonEntityBlockers(a, x, z, "Pack"),
		blkChestHere(a, x, z, true),
		blkDoorHere(a, x, z),
		blkCrystalHere(a, x, z),
	)
}

// crystalPlaceBlockers is the crystal placement rule (mirrors chestPlaceBlockers:
// one entity per tile). Crystals are non-blocking in play, but walls/props/deep
// water are still refused so the billboard has a clear tile.
func crystalPlaceBlockers(a *core.AreaDefinition, x, z int) []blockerCheck {
	return append(commonEntityBlockers(a, x, z, "Crystal"),
		blkPackHere(a, x, z),
		blkChestHere(a, x, z, true),
		blkDoorHere(a, x, z),
		blkCrystalHere(a, x, z),
	)
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
		Level:      s.editLevel,
		Name:       name,
		TargetMap:  core.SelfMapToken,
		TargetDoor: name,
		// Facing defaults away from an adjacent wall (overridable in the modal).
		Facing: doorFacingForCell(&s.area, x, z),
		Style:  core.DoorStyleBuilding,
	})
	s.dirty = true
}

// doorFacingForCell defaults a door's facing away from the first adjacent wall
// (via core.FacingAwayFromAdjacentWall), or StartFacing when none.
func doorFacingForCell(a *core.AreaDefinition, x, z int) int {
	if f, ok := core.FacingAwayFromAdjacentWall(a, x, z); ok {
		return f
	}
	return a.StartFacing
}

// doorNameFree reports whether name is unused among spawns (so a moved door can
// keep its identity when nothing else claims the name).
func doorNameFree(spawns []core.DoorSpawn, name string) bool {
	for _, sp := range spawns {
		if sp.Name == name {
			return false
		}
	}
	return true
}

// nextDoorName picks an unused "door_N" placeholder. Names must be unique within
// the map so runtime name resolution is unambiguous.
func nextDoorName(spawns []core.DoorSpawn) string {
	taken := make(map[string]bool, len(spawns))
	for _, sp := range spawns {
		taken[sp.Name] = true
	}
	return uniqueID("door_", func(id string) bool { return taken[id] })
}

// removeSpawnsAt drops every spawn on (x, z), generic over spawn types via
// core.TileXZ.
func removeSpawnsAt[T core.TileXZ](spawns []T, x, z int) []T {
	return slices.DeleteFunc(spawns, func(sp T) bool {
		tx, tz := sp.Tile()
		return tx == x && tz == z
	})
}

// deleteSpawnSlice drops every spawn on (x, z) from *slice in place, reporting
// whether the slice shrank — the one-liner body each context-menu delete closure needs.
func deleteSpawnSlice[T core.TileXZ](slice *[]T, x, z int) bool {
	before := len(*slice)
	*slice = removeSpawnsAt(*slice, x, z)
	return len(*slice) != before
}

// removeSpawnsWhere drops every spawn whose tile satisfies pred.
func removeSpawnsWhere[T core.TileXZ](spawns []T, pred func(x, z int) bool) []T {
	return slices.DeleteFunc(spawns, func(sp T) bool {
		return pred(sp.Tile())
	})
}

// placeChestAt drops a chest with default starter loot at (x,z), refusing illegal
// tiles (see chestPlaceBlockers).
func placeChestAt(s *State, x, z int) {
	a := &s.area
	if msg := firstBlocker(chestPlaceBlockers(a, x, z)...); msg != "" {
		s.flash(msg)
		return
	}
	s.area.ChestSpawns = append(s.area.ChestSpawns, core.ChestSpawn{
		TileX: x,
		TileZ: z,
		Level: s.editLevel,
		Items: defaultChestItems(),
	})
	s.dirty = true
}

// defaultChestItems is the seed loot for a new chest. Fresh slice (no shared alias).
func defaultChestItems() []core.ItemKind {
	return []core.ItemKind{core.ItemCheese, core.ItemBatJerky}
}

// placeCrystalAt drops a healing crystal at (x,z), refusing illegal tiles (see
// crystalPlaceBlockers).
func placeCrystalAt(s *State, x, z int) {
	a := &s.area
	if msg := firstBlocker(crystalPlaceBlockers(a, x, z)...); msg != "" {
		s.flash(msg)
		return
	}
	s.area.CrystalSpawns = append(s.area.CrystalSpawns, core.CrystalSpawn{TileX: x, TileZ: z, Level: s.editLevel})
	s.dirty = true
}

// eraseSentinel is the "empty" char a layer resets to when erased (shared by
// flood-erase and per-cell eraseAt). Elevation resets to the ground baseline.
func eraseSentinel(layer Layer) byte {
	d := &layerDefs[layer]
	if !d.hasSentinel {
		// Fail closed like the sibling layer lookups (Entities has no per-tile char).
		panic("editor: eraseSentinel called for a layer with no sentinel")
	}
	return d.sentinel
}

// eraseAt resets the active layer's cell at (x,z) to its eraseSentinel; Props,
// Elevation, and Entities need bespoke handling.
func eraseAt(s *State, x, z int) {
	if !s.area.InBounds(x, z) {
		return
	}
	revealActiveLayer(s) // reveal so an erase isn't invisible
	// Per-layer clear; the returned bool drives dirty so an entity clear that removed
	// nothing leaves the map clean. See layerDefs in layerdef.go.
	if layerDefs[s.layer].erase(s, x, z) {
		s.dirty = true
	}
}

// placeRamp is the smart ramp tool (toolbar Ramp mode): finds the single axis
// whose opposite neighbors differ by ±1 level and stamps the ramp arrow + low
// level. Refuses when no clean ±1 step exists. Snapshots undo only on success.
func placeRamp(s *State, x, z int) {
	if !s.area.InBounds(x, z) {
		return
	}
	// A ramp lives on the floor, but a wall tile never draws its floor — a ramp
	// under a wall would be invisible/non-functional. Refuse.
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
		revealActiveLayer(s) // reveal so the placed ramp isn't invisible (mirrors applyTool)
		setLayerCell(&s.area.Floor, x, z, core.RampCharForFacing(ascend))
		setTileGroundLevel(s, x, z, low)
		s.dirty = true
		return
	}
	s.flash("Ramp needs one neighbor a level higher on a single axis (set heights first)")
}

// clearEntitiesAt removes the pack/chest/door at (x,z), returning true if any was
// removed. Refuses the anchored player-start tile. Shared by right-click erase and
// the entityClear brush.
func clearEntitiesAt(s *State, x, z int) bool {
	if s.area.StartTileX == x && s.area.StartTileZ == z {
		s.flash("Player start can't be erased; place it elsewhere instead")
		return false
	}
	before := totalEntityCount(&s.area)
	removeAllEntitiesAt(&s.area, x, z)
	if totalEntityCount(&s.area) == before {
		return false
	}
	s.dirty = true
	return true
}

// addPackMember appends a member of `kind` to the pack at (x,z). If no pack
// exists at the tile, creates a fresh pack with the single member.
func addPackMember(s *State, x, z int, kind core.EnemyKind) {
	a := &s.area
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
		Level:   s.editLevel,
		Members: []core.PackMemberRef{core.BuiltinPackMember(kind)},
	})
	s.dirty = true
}

// removeAllEntitiesAt strips every pack/chest/door/crystal spawn on (x,z),
// mutating the passed-in area's slices.
func removeAllEntitiesAt(a *core.AreaDefinition, x, z int) {
	a.PackSpawns = removePackAt(a.PackSpawns, x, z)
	a.ChestSpawns = removeSpawnsAt(a.ChestSpawns, x, z)
	a.DoorSpawns = removeSpawnsAt(a.DoorSpawns, x, z)
	a.CrystalSpawns = removeSpawnsAt(a.CrystalSpawns, x, z)
}

// totalEntityCount sums all spawn-list lengths. Single source for the clear-entity
// dirty check and resize drop-count.
func totalEntityCount(a *core.AreaDefinition) int {
	return len(a.PackSpawns) + len(a.ChestSpawns) + len(a.DoorSpawns) + len(a.CrystalSpawns)
}

func removePackAt(packs []core.PackSpawn, x, z int) []core.PackSpawn {
	return removeSpawnsAt(packs, x, z)
}

// packSpawnLeaderKind picks the pack's field icon kind (highest-Tier member, ties
// by order). Delegates to core.PackSpawnLeaderKind.
func packSpawnLeaderKind(a core.AreaDefinition, sp core.PackSpawn) core.EnemyKind {
	return core.PackSpawnLeaderKind(a, sp)
}

// setLayerCell writes byte b at (x,z) in a layer grid. Callers flag reachability
// dirty separately (this write doesn't know the State).
func setLayerCell(layer *[]string, x, z int, b byte) {
	// Mirror cellAt's guard: a ragged area can be shorter than Width/Height, so raw
	// indexing panics even after an InBounds (declared-dims) check upstream.
	if z < 0 || z >= len(*layer) || x < 0 || x >= len((*layer)[z]) {
		return
	}
	row := []byte((*layer)[z])
	row[x] = b
	(*layer)[z] = string(row)
}

// regionEntities is the spawn set captured alongside a copied marquee, coords made
// region-local (0-based from the marquee's top-left) and remapped on paste. Tiles
// alone (core.TileRegion) don't carry entities, so the editor snapshots them here.
type regionEntities struct {
	packs    []core.PackSpawn
	chests   []core.ChestSpawn
	doors    []core.DoorSpawn
	crystals []core.CrystalSpawn
}

func (r regionEntities) empty() bool {
	return len(r.packs)+len(r.chests)+len(r.doors)+len(r.crystals) == 0
}

func (r regionEntities) count() int {
	return len(r.packs) + len(r.chests) + len(r.doors) + len(r.crystals)
}

func inRect(x, z, x0, z0, x1, z1 int) bool { return x >= x0 && x <= x1 && z >= z0 && z <= z1 }

// captureRegionEntities clones every spawn inside the inclusive rectangle with
// region-local coords (member/item slices deep-copied so the snapshot is independent).
func captureRegionEntities(a *core.AreaDefinition, x0, z0, x1, z1 int) regionEntities {
	var out regionEntities
	for _, p := range a.PackSpawns {
		if inRect(p.TileX, p.TileZ, x0, z0, x1, z1) {
			p.Members = append([]core.PackMemberRef(nil), p.Members...)
			p.TileX, p.TileZ = p.TileX-x0, p.TileZ-z0
			out.packs = append(out.packs, p)
		}
	}
	for _, c := range a.ChestSpawns {
		if inRect(c.TileX, c.TileZ, x0, z0, x1, z1) {
			c.Items = append([]core.ItemKind(nil), c.Items...)
			c.TileX, c.TileZ = c.TileX-x0, c.TileZ-z0
			out.chests = append(out.chests, c)
		}
	}
	for _, d := range a.DoorSpawns {
		if inRect(d.TileX, d.TileZ, x0, z0, x1, z1) {
			d.TileX, d.TileZ = d.TileX-x0, d.TileZ-z0
			out.doors = append(out.doors, d)
		}
	}
	for _, cr := range a.CrystalSpawns {
		if inRect(cr.TileX, cr.TileZ, x0, z0, x1, z1) {
			cr.TileX, cr.TileZ = cr.TileX-x0, cr.TileZ-z0
			out.crystals = append(out.crystals, cr)
		}
	}
	return out
}

// stampRegionEntities places the captured spawns with the region's top-left at
// (atX,atZ), replacing any same-kind spawn already on a destination tile and giving
// pasted doors fresh unique names (a self-referencing target is repointed to match).
func stampRegionEntities(s *State, ents regionEntities, atX, atZ int) {
	a := &s.area
	for _, p := range ents.packs {
		x, z := atX+p.TileX, atZ+p.TileZ
		if !a.InBounds(x, z) {
			continue
		}
		a.PackSpawns = removeSpawnsAt(a.PackSpawns, x, z)
		p.TileX, p.TileZ = x, z
		p.Members = append([]core.PackMemberRef(nil), p.Members...)
		a.PackSpawns = append(a.PackSpawns, p)
	}
	for _, c := range ents.chests {
		x, z := atX+c.TileX, atZ+c.TileZ
		if !a.InBounds(x, z) {
			continue
		}
		a.ChestSpawns = removeSpawnsAt(a.ChestSpawns, x, z)
		c.TileX, c.TileZ = x, z
		c.Items = append([]core.ItemKind(nil), c.Items...)
		a.ChestSpawns = append(a.ChestSpawns, c)
	}
	for _, d := range ents.doors {
		x, z := atX+d.TileX, atZ+d.TileZ
		if !a.InBounds(x, z) {
			continue
		}
		a.DoorSpawns = removeSpawnsAt(a.DoorSpawns, x, z)
		old := d.Name
		// Keep the door's name when it's still free — a MOVE (origin already cleared)
		// must preserve identity so inbound links survive. Rename only on a real
		// collision (same-map copy/paste), repointing a self-loop to the new name.
		if old == "" || !doorNameFree(a.DoorSpawns, old) {
			d.Name = nextDoorName(a.DoorSpawns)
			if d.TargetMap == core.SelfMapToken && d.TargetDoor == old {
				d.TargetDoor = d.Name
			}
		}
		d.TileX, d.TileZ = x, z
		a.DoorSpawns = append(a.DoorSpawns, d)
	}
	for _, cr := range ents.crystals {
		x, z := atX+cr.TileX, atZ+cr.TileZ
		if !a.InBounds(x, z) {
			continue
		}
		a.CrystalSpawns = removeSpawnsAt(a.CrystalSpawns, x, z)
		cr.TileX, cr.TileZ = x, z
		a.CrystalSpawns = append(a.CrystalSpawns, cr)
	}
}

// removeRegionEntities drops every spawn inside the inclusive rectangle (the entity
// half of a cut / region-move).
func removeRegionEntities(a *core.AreaDefinition, x0, z0, x1, z1 int) {
	inSel := func(tx, tz int) bool { return inRect(tx, tz, x0, z0, x1, z1) }
	a.PackSpawns = slices.DeleteFunc(a.PackSpawns, func(p core.PackSpawn) bool { return inSel(p.TileX, p.TileZ) })
	a.ChestSpawns = slices.DeleteFunc(a.ChestSpawns, func(c core.ChestSpawn) bool { return inSel(c.TileX, c.TileZ) })
	a.DoorSpawns = slices.DeleteFunc(a.DoorSpawns, func(d core.DoorSpawn) bool { return inSel(d.TileX, d.TileZ) })
	a.CrystalSpawns = slices.DeleteFunc(a.CrystalSpawns, func(cr core.CrystalSpawn) bool { return inSel(cr.TileX, cr.TileZ) })
}

// copySelection snapshots the active marquee (grid layers + the entities on it) into
// the clipboard. core.CopyRegion does the grid transform; entities are captured here.
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
	s.clipEntities = captureRegionEntities(&s.area, s.selX0, s.selZ0, s.selX1, s.selZ1)
	s.flash(copiedRegionFlash("Copied", s.clipboard, s.clipEntities) + " — Ctrl+V to paste at the cursor")
}

// copiedRegionFlash formats a "<verb> WxH region (+N entities)" status line, shared
// by copy / cut so the entity count reads consistently.
func copiedRegionFlash(verb string, r core.TileRegion, ents regionEntities) string {
	msg := fmt.Sprintf("%s %d×%d region", verb, r.W, r.H)
	if n := ents.count(); n > 0 {
		msg += fmt.Sprintf(" (+%d entities)", n)
	}
	return msg
}

// pasteSelection stamps the clipboard (grid + entities) with its top-left at
// (atX,atZ) under one undo step. No-op off-map or with an empty clipboard.
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
	stampRegionEntities(s, s.clipEntities, atX, atZ)
	s.dirty = true
	s.flash(fmt.Sprintf("Pasted %d×%d region", s.clipboard.W, s.clipboard.H))
}

// cutSelection copies the marquee (grid + entities) then clears it — one undo step.
func cutSelection(s *State) {
	if !s.selActive {
		s.flash("Select a region first (Select tool), then Ctrl+X")
		return
	}
	copySelection(s)
	if s.clipboard.Empty() {
		return
	}
	before := core.CloneArea(s.area)
	s.area.ClearRegion(s.selX0, s.selZ0, s.selX1, s.selZ1)
	removeRegionEntities(&s.area, s.selX0, s.selZ0, s.selX1, s.selZ1)
	if core.AreaContentEqual(s.area, before) {
		return // nothing there — leave the copy, bank no undo
	}
	commitUndoSnapshot(s, before)
	s.dirty = true
	s.flash(copiedRegionFlash("Cut", s.clipboard, s.clipEntities))
}

// hasSelection / hasClipboard back the Edit-menu enable predicates for the region
// verbs so a greyed row can't be fired with nothing selected / nothing copied.
func hasSelection(s *State) bool { return s.selActive }
func hasClipboard(s *State) bool { return !s.clipboard.Empty() }

// menuPaste is the Edit ▸ Paste target rule: the grid tile last hovered, else the
// selection origin, else the map center (a menu click may not be over the canvas).
func menuPaste(s *State) {
	x, z := s.hoverX, s.hoverZ
	if !s.area.InBounds(x, z) {
		if s.selActive {
			x, z = s.selX0, s.selZ0
		} else {
			x, z = s.area.Width/2, s.area.Height/2
		}
	}
	pasteSelection(s, x, z)
}

// selectWholeMap sets the marquee to the entire map (Ctrl+A).
func selectWholeMap(s *State) {
	if s.area.Width <= 0 || s.area.Height <= 0 {
		return
	}
	s.selX0, s.selZ0 = 0, 0
	s.selX1, s.selZ1 = s.area.Width-1, s.area.Height-1
	s.selActive = true
	s.flash("Selected whole map — Ctrl+C to copy")
}

// duplicateSelection copies the marquee and pastes it one tile down-right, moving the
// selection onto the copy so a repeated Ctrl+D staggers duplicates (one undo each).
func duplicateSelection(s *State) {
	if !s.selActive {
		s.flash("Select a region first (Select tool), then Ctrl+D")
		return
	}
	copySelection(s)
	if s.clipboard.Empty() {
		return
	}
	atX, atZ := s.selX0+1, s.selZ0+1
	if !s.area.InBounds(atX, atZ) {
		atX, atZ = s.selX0, s.selZ0
	}
	pasteSelection(s, atX, atZ)
	s.selX0, s.selZ0 = atX, atZ
	s.selX1, s.selZ1 = atX+s.clipboard.W-1, atZ+s.clipboard.H-1
}

// moveSelectionBy shifts the committed marquee's contents (grid + entities) by
// (dx,dz) tiles under one undo step: snapshot the region, clear the origin, stamp at
// the offset, and follow the selection to the new location. Clamped so the region
// stays on-map. No-op for a zero shift.
func moveSelectionBy(s *State, dx, dz int) {
	if !s.selActive || (dx == 0 && dz == 0) {
		return
	}
	w, h := s.selX1-s.selX0+1, s.selZ1-s.selZ0+1
	nx := core.Clamp(s.selX0+dx, 0, s.area.Width-w)
	nz := core.Clamp(s.selZ0+dz, 0, s.area.Height-h)
	if nx == s.selX0 && nz == s.selZ0 {
		return
	}
	region := core.CopyRegion(&s.area, s.selX0, s.selZ0, s.selX1, s.selZ1)
	ents := captureRegionEntities(&s.area, s.selX0, s.selZ0, s.selX1, s.selZ1)
	before := core.CloneArea(s.area)
	s.area.ClearRegion(s.selX0, s.selZ0, s.selX1, s.selZ1)
	removeRegionEntities(&s.area, s.selX0, s.selZ0, s.selX1, s.selZ1)
	s.area.PasteRegion(region, nx, nz)
	stampRegionEntities(s, ents, nx, nz)
	if core.AreaContentEqual(s.area, before) {
		return
	}
	commitUndoSnapshot(s, before)
	s.dirty = true
	s.selX0, s.selZ0 = nx, nz
	s.selX1, s.selZ1 = nx+w-1, nz+h-1
}

// clearSelection drops the active marquee but leaves the clipboard intact. Call
// on area replace (New/Open) or resize where the selection bounds would no longer
// match; NOT from undoOne/redoOne (a same-map paint undo keeps its selection).
func clearSelection(s *State) {
	s.selActive = false
}

// revealActiveLayer un-hides the layer being edited so a paint/erase/fill on a hidden
// layer isn't invisible. Shared by applyTool, eraseAt, placeRamp, floodFill, and
// fillEntireLayer so the reveal can't be forgotten on a new edit path.
func revealActiveLayer(s *State) {
	s.layerHidden[s.layer] = false
}

// invalidateContentCaches drops the derived-state caches after any edit that changes
// map content: the reachability cache and the content-epoch-keyed caches (3D preview,
// fade, tooltip, reach badge) that rebuild lazily next frame. Single seam so none of
// the mutation paths (paint, undo/redo, new/open, resize) can forget one.
func invalidateContentCaches(s *State) {
	s.reachValid = false
	s.contentEpoch++
}

// pushUndo snapshots the current area before a mutation, invalidating redo. Capped
// at undoLimit; on trim, the window shifts in place and freed tail slots are nil'd
// for GC (avoids a fresh array alloc every stroke at the cap).
func pushUndo(s *State) {
	commitUndoSnapshot(s, core.CloneArea(s.area))
}

// commitUndoSnapshot banks `before` onto the undo stack, invalidates the
// reachability cache, and clears redo. strokePaint passes a stroke-start snapshot
// so a multi-cell drag banks ONE step (only when it changed something). Every
// forward mutation routes through here.
func commitUndoSnapshot(s *State, before core.AreaDefinition) {
	invalidateContentCaches(s)
	s.undo = append(s.undo, before)
	if excess := len(s.undo) - undoLimit; excess > 0 {
		// Shift the kept window to the front, then release the tail slots for GC.
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
	invalidateContentCaches(s) // area replaced — rebuild reachability + epoch caches lazily
	// Drop the dirty marker if we stepped back to the on-disk baseline.
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
	invalidateContentCaches(s) // area replaced — rebuild reachability + epoch caches lazily
	s.dirty = !core.AreaContentEqual(s.area, s.baseline)
}

// resize grows or shrinks every layer to (w,h). New cells get the layer's blank
// value. Player start is clamped; spawns outside the new bounds are removed.
func resize(s *State, w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	// Clamp to [Min,Max]MapDimension (shared with metadata input + new-map dialog).
	if w < core.MinMapDimension {
		s.flash("Map width too small (min " + strconv.Itoa(core.MinMapDimension) + ")")
		return
	}
	if h < core.MinMapDimension {
		s.flash("Map height too small (min " + strconv.Itoa(core.MinMapDimension) + ")")
		return
	}
	w = core.ClampMapDimension(w)
	h = core.ClampMapDimension(h)
	if w == s.area.Width && h == s.area.Height {
		return
	}
	pushUndo(s)
	// Resize every grid-backed layer from its layerDefs descriptor (slice + sentinel),
	// so a new grid layer needs no edit here — layerDefs' init-assert governs coverage.
	// Uses the OLD dims (s.area.Width/Height); those are updated below after all resizes.
	for l := 0; l < layerCount; l++ {
		grid := layerDefs[l].grid
		if grid == nil { // Entities: gridless
			continue
		}
		gp := grid(s)
		*gp = resizeLayer(*gp, s.area.Width, s.area.Height, w, h, layerDefs[l].sentinel)
	}
	// Resize every voxel plane in lockstep (new cells = air) or the stack desyncs.
	for L := range s.area.Solids {
		s.area.Solids[L] = resizeLayer(s.area.Solids[L], s.area.Width, s.area.Height, w, h, core.SolidAir)
	}
	// Per-floor scatter stacks resize in lockstep too (new cells = blank), exactly
	// like Solids — otherwise the planes keep the old dims and desync from W/H.
	for L := range s.area.PropStack {
		s.area.PropStack[L] = resizeLayer(s.area.PropStack[L], s.area.Width, s.area.Height, w, h, core.TilePropEmpty)
	}
	for L := range s.area.DecorStack {
		s.area.DecorStack[L] = resizeLayer(s.area.DecorStack[L], s.area.Width, s.area.Height, w, h, core.DecorEmpty)
	}
	// Per-tile level grids resize in lockstep too (auto-fill new cells).
	if len(s.area.PropLevels) > 0 {
		s.area.PropLevels = resizeLayer(s.area.PropLevels, s.area.Width, s.area.Height, w, h, core.PropLevelAuto)
	}
	if len(s.area.DecorLevels) > 0 {
		s.area.DecorLevels = resizeLayer(s.area.DecorLevels, s.area.Width, s.area.Height, w, h, core.PropLevelAuto)
	}
	// PropYaw is a per-tile grid too — resize it in lockstep, else it keeps the old
	// dims and validatePropYawGrid rejects the map as unsaveable (and the only recovery,
	// cycling a facing, blanks every authored facing via normalizeOptionalLayer).
	if len(s.area.PropYaw) > 0 {
		s.area.PropYaw = resizeLayer(s.area.PropYaw, s.area.Width, s.area.Height, w, h, core.PropYawAuto)
	}
	// Face skins are per-tile too: drop any now past the new bounds so a later
	// re-grow can't re-expose stale cliff skins (spawns/locations are pruned below).
	s.area.PruneFaceOverridesOutside(w, h)
	s.area.Width = w
	s.area.Height = h
	clearSelection(s) // bounds changed — a stale marquee could outline off-grid
	// Re-seal the perimeter ring so a grown map keeps a complete outer wall.
	sealWallBorder(&s.area)
	// Clamp a now-out-of-range start to the last INTERIOR cell (w-2/h-2), not the
	// guaranteed-wall border (w-1/h-1). Floor at 1 for a degenerate tiny map.
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
	entitiesBefore := totalEntityCount(&s.area)
	outside := outsideBounds(w, h)
	s.area.PackSpawns = removeSpawnsWhere(s.area.PackSpawns, outside)
	s.area.ChestSpawns = removeSpawnsWhere(s.area.ChestSpawns, outside)
	s.area.DoorSpawns = removeSpawnsWhere(s.area.DoorSpawns, outside)
	s.area.CrystalSpawns = removeSpawnsWhere(s.area.CrystalSpawns, outside)
	// removeXOutside drops only spawns PAST the new bounds. A spawn on the tile
	// that just became the sealed border is in-bounds but now walled — drop those
	// too so a chest/door isn't buried unreachable.
	pruneBlockedSpawns(&s.area)
	// Locations are rectangles, not point spawns: drop any whose origin fell past the
	// new bounds, clamp the rest back onto the grid so none references off-map tiles.
	locsBefore := len(s.area.Locations)
	s.area.Locations = pruneLocationsOutside(s, w, h)
	droppedLocs := locsBefore - len(s.area.Locations)
	// Flash a count of dropped spawns/locations so the loss isn't silent.
	dropped := entitiesBefore - totalEntityCount(&s.area)
	switch {
	case dropped > 0 && droppedLocs > 0:
		s.flash(fmt.Sprintf("Resize dropped %d spawn(s) and %d location(s) outside the new bounds", dropped, droppedLocs))
	case dropped > 0:
		s.flash(fmt.Sprintf("Resize dropped %d spawn(s) outside or walled by the new bounds", dropped))
	case droppedLocs > 0:
		s.flash(fmt.Sprintf("Resize dropped %d location(s) outside the new bounds", droppedLocs))
	}
	s.dirty = true
}

// pruneLocationsOutside drops regions whose origin fell past the new (w,h) bounds
// after a shrink and clamps any that merely overhang the new edge, mirroring the
// spawn prune so a region never references off-grid tiles.
func pruneLocationsOutside(s *State, w, h int) []core.Location {
	kept := make([]core.Location, 0, len(s.area.Locations))
	for _, loc := range s.area.Locations {
		if loc.X >= w || loc.Z >= h {
			continue // origin off-grid — the whole region is gone
		}
		clampLocation(s, &loc)
		kept = append(kept, loc)
	}
	return kept
}

// outsideBounds is the shrink-prune predicate: a tile is outside the new (w,h)
// bounds when its column or row falls past the edge. Fed to removeSpawnsWhere for
// each spawn kind in resize.
func outsideBounds(w, h int) func(x, z int) bool {
	return func(x, z int) bool { return x >= w || z >= h }
}

// sealWallBorder raises the outer ring one level (an enclosing wall) with an
// explicit rock face skin, leaving the interior untouched. Stamps the Elevation +
// Faces layers (walls are elevation), matching blankArea's perimeter rule.
func sealWallBorder(a *core.AreaDefinition) {
	wallChar := core.ElevationChar(core.ElevationWallRingLevel)
	for z := 0; z < a.Height; z++ {
		var faceRow, elevRow []byte
		if z < len(a.Walls) {
			faceRow = []byte(a.Walls[z])
		}
		if z < len(a.Elevation) {
			elevRow = []byte(a.Elevation[z])
		}
		for x := 0; x < a.Width; x++ {
			if x != 0 && z != 0 && x != a.Width-1 && z != a.Height-1 {
				continue
			}
			if x < len(faceRow) {
				faceRow[x] = core.TileRock
			}
			if x < len(elevRow) {
				elevRow[x] = wallChar
			}
		}
		if faceRow != nil {
			a.Walls[z] = string(faceRow)
		}
		if elevRow != nil {
			a.Elevation[z] = string(elevRow)
		}
	}
	// Voxel maps read the stack, not Elevation — also raise the border columns there.
	if len(a.Solids) > 0 {
		for z := 0; z < a.Height; z++ {
			for x := 0; x < a.Width; x++ {
				if x == 0 || z == 0 || x == a.Width-1 || z == a.Height-1 {
					a.SetColumnTop(x, z, core.ElevationWallRingLevel)
				}
			}
		}
	}
}

// resizeLayer copies an old grid into a new W'xH' grid, padding extra cells with
// `fill` and dropping cells outside the new bounds.
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

// writeAreaTo serializes the area to path and resets the dirty-tracking baseline.
// Shared serialize/write/baseline core of saveTo / saveCurrent / confirmDirtySave.
func writeAreaTo(s *State, path string) error {
	mf, err := core.MapFileFromArea(s.area)
	if err != nil {
		return err
	}
	if err := mapfile.Save(path, mf); err != nil {
		return err
	}
	s.area.Path = path
	s.baseline = core.CloneArea(s.area)
	s.dirty = false
	rememberLastMap(path) // reopen here next session (NewDefault)
	return nil
}

// saveCurrent writes to the area's existing path. If the area has never been
// saved (Path == ""), open the Save As modal so the user can name it.
func saveCurrent(s *State) {
	if s.area.Path == "" {
		openSaveAsModal(s)
		return
	}
	if err := writeAreaTo(s, s.area.Path); err != nil {
		s.flash("Save failed: " + err.Error())
		return
	}
	// Reachability is an at-will check (Validate modal), not auto-flashed on save.
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
		// Cap before building the next candidate so the just-tested name was
		// stat-checked. Tries _copy, _copy2 … _copy100, then gives up.
		if i > 100 {
			return "", fmt.Errorf("too many copies of %s", id)
		}
		candidate = fmt.Sprintf("%s_copy%d", id, i)
	}
}

func openModal(s *State, m modalKind) {
	s.modal = m
	s.modalCursor = 0
	switch m {
	case modalOpen:
		// Newest-first: the last-touched file is the likeliest Open target.
		paths, _ := mapfile.ListByModTime(core.MapsDir())
		s.modalPaths = paths
	}
}

// openConfirmDirtyModal raises the unsaved-changes prompt, stashing the action to
// run on resolve. Shared by every discard-edits entry point (New/Open/Exit).
func openConfirmDirtyModal(s *State, pending pendingAction) {
	s.pending = pending
	s.modal = modalConfirmDirty
}

// newMap is the user-facing entry: confirm-dirty bounce if dirty, else open the
// new-map setup modal.
func newMap(s *State) {
	if s.dirty {
		openConfirmDirtyModal(s, pendingNew)
		return
	}
	openNewMapModal(s)
}

// openNewMapModal opens the new-map setup dialog with defaults
// (core.DefaultNewMapDimension square, FloorAuto); commits via performNewMap.
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

// floodFill replaces the 4-connected like-cell region around (x,z) with b on the
// active grid. No-op if (x,z) already holds b or on LayerEntities (no grid).
func floodFill(s *State, x, z int, b byte, erase bool) {
	layer := activeGrid(s)
	if layer == nil {
		return
	}
	if layerBlocksBulkFill(s) {
		s.flash("Flood fill isn't available for per-floor props/decor — use the brush")
		return
	}
	if !s.area.InBounds(x, z) {
		return
	}
	// Voxel elevation: the Elevation grid is dead once Solids is explicit, so a BFS
	// over it (and its rewrite) would be a silent no-op that still banks undo/dirty.
	// Flood the ElevationLevelAt-connected region on live column data instead.
	if s.layer == LayerElevation && len(s.area.Solids) > 0 {
		floodFillVoxelElevation(s, x, z, b, erase)
		return
	}
	// InBounds checks declared dims, which can exceed actual slice lengths for a
	// ragged area — guard this raw seed read against the real extents.
	if z >= len(*layer) || x >= len((*layer)[z]) {
		return
	}
	target := (*layer)[z][x]
	if target == b {
		return // no-op fill — snapshot nothing
	}
	// Snapshot only now that the fill will change cells (no useless undo step).
	pushUndo(s)
	revealActiveLayer(s) // reveal so the fill isn't invisible (mirrors applyTool)
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
	})
	switch {
	case erase:
		// Flood-erase: the flooded sentinel IS the cleared state. Do NOT fall through
		// to the active-level lift — erasing must not raise the ground.
	case layerStampsActiveLevel(s.layer):
		// PAINT: the flooded region joins the active floor (mirror of applyTool's
		// per-cell stamp). Walls exempt.
		liftCellsToActiveLevel(s, filled)
	}
	if !erase {
		guardStartTileAfterBulkFill(s)
	}
	s.dirty = true
}

// floodFillVoxelElevation flood-fills the 4-connected region of columns sharing the
// seed's top-solid level (ElevationLevelAt — the live data; the Elevation string is
// dead when Solids is explicit). Paint raises each column to the brush's level via
// SetColumnTop; erase clears the cube at editLevel (mirror of eraseAt → ClearCube).
func floodFillVoxelElevation(s *State, x, z int, b byte, erase bool) {
	target := s.area.ElevationLevelAt(x, z)
	newLevel := core.ElevationLevelFromChar(b)
	if !erase && newLevel == target {
		return // painting a column to its own level changes nothing — bank no undo
	}
	// Collect the connected same-level region first, then apply (SetColumnTop only
	// touches its own column, so applying can't disturb an unvisited neighbor's level).
	var region [][2]int
	visited := make(map[[2]int]bool)
	stack := [][2]int{{x, z}}
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if !s.area.InBounds(p[0], p[1]) || visited[p] {
			continue
		}
		if s.area.ElevationLevelAt(p[0], p[1]) != target {
			continue
		}
		visited[p] = true
		region = append(region, p)
		stack = append(stack, [2]int{p[0] + 1, p[1]}, [2]int{p[0] - 1, p[1]}, [2]int{p[0], p[1] + 1}, [2]int{p[0], p[1] - 1})
	}
	pushUndo(s)
	for _, c := range region {
		if erase {
			s.area.ClearCube(c[0], s.editLevel, c[1])
		} else {
			s.area.SetColumnTop(c[0], c[1], newLevel)
		}
	}
	s.dirty = true
}

// pruneBlockedSpawns drops any spawn now on a BlockedAt tile. Used by resize
// after a shrink.
func pruneBlockedSpawns(a *core.AreaDefinition) {
	blocked := func(x, z int) bool { return a.BlockedAt(x, z) }
	a.PackSpawns = removeSpawnsWhere(a.PackSpawns, blocked)
	a.ChestSpawns = removeSpawnsWhere(a.ChestSpawns, blocked)
	a.DoorSpawns = removeSpawnsWhere(a.DoorSpawns, blocked)
	a.CrystalSpawns = removeSpawnsWhere(a.CrystalSpawns, blocked)
}

// guardStartTileAfterBulkFill reverts a bulk Props/Decor fill's write on the player-
// start tile back to the layer sentinel. The per-cell brush refuses content on the
// start (a prop there BLOCKS movement and silently soft-locks the spawn), but flood/
// fill-all write straight into the grid and would otherwise sneak it in. No-op on
// other layers or when the start is out of bounds.
func guardStartTileAfterBulkFill(s *State) {
	if s.layer != LayerProps && s.layer != LayerDecor {
		return
	}
	sx, sz := s.area.StartTileX, s.area.StartTileZ
	if !s.area.InBounds(sx, sz) {
		return
	}
	setLayerCell(activeGrid(s), sx, sz, layerDefs[s.layer].sentinel)
}

// rewriteLayerRows clones the layer into a mutable [][]byte, runs visit on it,
// and writes back. The shared "build fresh byte rows then commit" idiom.
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

// fillEntireLayer overwrites every cell on the active grid with the active brush.
// Skips entities; pushes a single undo.
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
	if brush.Erase {
		s.flash("Pick a brush to Fill (the eraser fills nothing)")
		return
	}
	if layerBlocksBulkFill(s) {
		s.flash("Fill all isn't available for per-floor props/decor — use the brush")
		return
	}
	// Voxel elevation: the Elevation string is dead once Solids is explicit, so a
	// grid fill wouldn't show. Lift every column to the brush's level via SetColumnTop.
	if s.layer == LayerElevation && len(s.area.Solids) > 0 {
		lvl := core.ElevationLevelFromChar(brush.Char)
		pushUndo(s)
		revealActiveLayer(s) // reveal so the fill isn't invisible (mirrors applyTool)
		for z := 0; z < s.area.Height; z++ {
			for x := 0; x < s.area.Width; x++ {
				s.area.SetColumnTop(x, z, lvl)
			}
		}
		s.dirty = true
		s.flash("Filled " + layerName(s.layer))
		return
	}
	pushUndo(s)
	revealActiveLayer(s) // reveal so the fill isn't invisible (mirrors applyTool)
	var filled [][2]int
	rewriteLayerRows(layer, func(rows [][]byte) {
		for z := 0; z < s.area.Height && z < len(rows); z++ {
			for x := 0; x < s.area.Width && x < len(rows[z]); x++ {
				rows[z][x] = brush.Char
				filled = append(filled, [2]int{x, z})
			}
		}
	})
	// A full content fill lifts the whole map to the active floor (no-op at level
	// 0, so flat maps stay flat). Walls exempt (see stampActiveLevel).
	if layerStampsActiveLevel(s.layer) {
		liftCellsToActiveLevel(s, filled)
	}
	guardStartTileAfterBulkFill(s)
	s.dirty = true
	s.flash("Filled " + layerName(s.layer))
}

// centerViewOnTile recenters the view so (tx, tz) sits in the middle of the grid
// pane (G "center on start"). Zoom untouched.
func centerViewOnTile(s *State, tx, tz int) {
	// 3D view pans the orbit target (not panX/panY): offset the target from the
	// map center so tile (tx,tz) sits under the camera focus. Keeps minimap click,
	// G, and Center-on-Start working in the default 3D view.
	if s.isoView {
		s.isoTargetX = core.TileCenter(tx) - float32(s.area.Width)*core.TileSize/2
		s.isoTargetZ = core.TileCenter(tz) - float32(s.area.Height)*core.TileSize/2
		s.flash("Centered on " + core.TileCoord(tx, tz))
		return
	}
	if s.rect.cellPx <= 0 {
		return
	}
	// Target world-pixel coord (gridX/Y already include panX/panY).
	want := s.rect.grid.X + s.rect.grid.Width/2
	wantY := s.rect.grid.Y + s.rect.grid.Height/2
	have, haveY := s.rect.tileCenter(tx, tz)
	s.panX += want - have
	s.panY += wantY - haveY
	s.flash("Centered on " + core.TileCoord(tx, tz))
}

// paintableCell reports whether a rect/line paint should stamp (x,z): in bounds
// and not the player-start tile when the brush would wall it (TileRock). Shared by
// paintRect/paintRectOutline/paintLine so the start-protection rule can't drift.
func paintableCell(s *State, brushChar byte, x, z int) bool {
	if !s.area.InBounds(x, z) {
		return false
	}
	return !(brushChar == core.TileRock && s.area.StartTileX == x && s.area.StartTileZ == z)
}

// stampFootprintAnchor handles the multi-tile-footprint case for the rect/line
// tools: such a brush can't tile across a region (every cell re-validates against
// its neighbours and refuses), so it collapses to a single anchor stamp at
// (x0,z0). Returns true when it handled the paint (the caller must then return).
func stampFootprintAnchor(s *State, x0, z0 int) bool {
	if !brushHasMultiTileFootprint(s) {
		return false
	}
	if s.area.InBounds(x0, z0) {
		applyTool(s, x0, z0)
	}
	return true
}

// paintRect paints the active brush across the rectangle (x0,z0)-(x1,z1). Player
// start is left in place.
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
	if stampFootprintAnchor(s, x0, z0) {
		return
	}
	for z := z0; z <= z1; z++ {
		for x := x0; x <= x1; x++ {
			if paintableCell(s, brush.Char, x, z) {
				applyTool(s, x, z)
			}
		}
	}
}

// paintRectOutline paints only the border cells of (x0,z0)-(x1,z1) — the Box tool.
// Mirrors paintRect's footprint-collapse + start protection.
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
	if stampFootprintAnchor(s, x0, z0) {
		return
	}
	for z := z0; z <= z1; z++ {
		for x := x0; x <= x1; x++ {
			if x != x0 && x != x1 && z != z0 && z != z1 {
				continue // interior cell — outline only
			}
			if paintableCell(s, brush.Char, x, z) {
				applyTool(s, x, z)
			}
		}
	}
}

// walkLine invokes fn for every cell along the Bresenham line (x0,z0)-(x1,z1),
// both endpoints inclusive. Shared by paintLine and paintLineBetween (input.go).
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

// paintLine paints the active brush along the line (x0,z0)-(x1,z1) — the Line
// tool, a one-shot committed op (finishDrag pushes the undo). Distinct from
// input.go's paintLineBetween (freehand, lazy-undo). Mirrors paintRect's collapse.
func paintLine(s *State, x0, z0, x1, z1 int) {
	if s.layer == LayerEntities {
		return
	}
	brush := s.activeBrush()
	if stampFootprintAnchor(s, x0, z0) {
		return
	}
	walkLine(x0, z0, x1, z1, func(cx, cz int) {
		if paintableCell(s, brush.Char, cx, cz) {
			applyTool(s, cx, cz)
		}
	})
}

// activeGrid returns a pointer to the layer slice being edited, or nil for gridless
// layers (entities — the legitimate answer flood-fill checks for). See layerDefs.
func activeGrid(s *State) *[]string {
	if g := layerDefs[s.layer].grid; g != nil {
		return g(s)
	}
	return nil
}

// startTileBlocker returns the first "player can't spawn" warning, or "". Single
// source for the playtest gate and the reachability start-tile preamble.
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

// blockedTileSet marks every in-bounds spawn tile true in a fresh w*h grid,
// skipping any tile exempt reports (nil exempt = none). Shared pre-mark for the
// reachability BFS's chest- and door-as-wall passes.
func blockedTileSet[T core.TileXZ](w, h int, spawns []T, exempt func(x, z int) bool) []bool {
	mark := make([]bool, w*h)
	for _, sp := range spawns {
		x, z := sp.Tile()
		if x < 0 || x >= w || z < 0 || z >= h {
			continue
		}
		if exempt != nil && exempt(x, z) {
			continue
		}
		mark[z*w+x] = true
	}
	return mark
}

// reachableViaNeighbor reports whether any of (x,z)'s four orthogonal neighbours
// is visited — i.e. the player can stand beside the tile to interact (chests and
// doors are blocked on their own tile, so adjacency is what "reachable" means).
func reachableViaNeighbor(visited []bool, w, h, x, z int) bool {
	for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
		nx, nz := x+d[0], z+d[1]
		if nx < 0 || nx >= w || nz < 0 || nz >= h {
			continue
		}
		if visited[nz*w+nx] {
			return true
		}
	}
	return false
}

// reachabilityWarnings reports playability problems (empty = none). The BFS
// treats chest tiles as impassable like walls, matching the runtime (a chest in a
// chokepoint can sever the map).
func reachabilityWarnings(a core.AreaDefinition) []string {
	var out []string
	if msg := startTileBlocker(a); msg != "" {
		return []string{msg}
	}
	h := a.Height
	w := a.Width
	// Pre-mark chest tiles blocked (start tile is exempt above).
	chestBlock := blockedTileSet(w, h, a.ChestSpawns, nil)
	// Pre-mark door tiles blocked too: stepping onto a door fires a transition, so
	// the player can't walk THROUGH one to reach tiles beyond — matching runtime.
	// Exempt the start tile (the player spawns there even if it's an entrance door).
	doorBlock := blockedTileSet(w, h, a.DoorSpawns, func(x, z int) bool {
		return x == a.StartTileX && z == a.StartTileZ
	})
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
		if a.BlockedAt(px, pz) || chestBlock[idx] || doorBlock[idx] {
			continue
		}
		visited[idx] = true
		// Expand only to elevation-CONNECTED neighbors (a cliff blocks, a ramp
		// bridges; flat maps push all four).
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
	// Check against SNAPPED pack positions (placePacks relocates to the nearest
	// open square at runtime) so the warning matches what the player encounters.
	// Drops are classified: empty roster vs. no open tile (crowded map).
	var unreachable, emptyRoster, noOpenTile int
	for _, snap := range core.SnappedSpawnPositions(&a) {
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
	// A chest is reachable when at least one neighbour is visited (its own tile is
	// marked blocked), i.e. the player can walk up to open it.
	var unreachableChests int
	for _, c := range a.ChestSpawns {
		if c.TileX < 0 || c.TileX >= w || c.TileZ < 0 || c.TileZ >= h {
			unreachableChests++
			continue
		}
		if !reachableViaNeighbor(visited, w, h, c.TileX, c.TileZ) {
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
	// Door checks (local only — cross-map pairing lives in crossMapDoorWarnings):
	// no destination, or unreachable from start.
	var doorsNoTarget, doorsUnreachable int
	for _, d := range a.DoorSpawns {
		if !d.HasTarget() {
			doorsNoTarget++
		}
		if d.TileX < 0 || d.TileX >= w || d.TileZ < 0 || d.TileZ >= h {
			doorsUnreachable++
			continue
		}
		// Door tiles are blocked in the BFS (you step onto one to transition, not
		// through it), so reachable = the tile is the start (visited) OR a neighbour
		// is visited (player can step onto it) — same rule as chests.
		reachable := visited[d.TileZ*w+d.TileX] || reachableViaNeighbor(visited, w, h, d.TileX, d.TileZ)
		if !reachable {
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

// crossMapDoorWarnings validates each door's TargetMap / TargetDoor by loading
// the referenced map from disk. Expensive (file I/O) so it's NOT called per frame
// — only at playtest gating. One warning per dangling reference; same-map portals
// are checked against the in-memory area.
func crossMapDoorWarnings(a core.AreaDefinition) []string {
	if len(a.DoorSpawns) == 0 {
		return nil
	}
	// Cache loaded maps by id so repeated targets read disk once.
	loaded := make(map[string]core.AreaDefinition)
	var out []string
	for _, d := range a.DoorSpawns {
		if !d.HasTarget() {
			continue // already flagged by reachabilityWarnings
		}
		// Same-map portal: verify the named door exists locally.
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
// (broken node/dialog refs, out-of-bounds tiles, unregistered foe kinds). Empty =
// clean. Surfaced in the Map ▸ Validate report.
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
		case core.DialogTriggerEnterLocation:
			if t.LocationID == "" {
				out = append(out, fmt.Sprintf("trigger %q enter-location has no target region", t.ID))
			} else if _, ok := core.LocationByID(a.Locations, t.LocationID); !ok {
				out = append(out, fmt.Sprintf("trigger %q → location %q not found", t.ID, t.LocationID))
			}
		}
	}
	return out
}

// dialogCondWarning returns a problem with one choice condition, or "". Only
// world-referencing kinds are checkable statically (gold/quest gates are runtime).
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

// mapHasDoor reports whether spawns contains a door named `name`. Linear scan
// (door counts are tiny).
func mapHasDoor(spawns []core.DoorSpawn, name string) bool {
	for _, d := range spawns {
		if d.Name == name {
			return true
		}
	}
	return false
}

// performNewMap replaces the current area with a fresh blank one (clamped dims +
// default floor). Called by modalNew on commit and the pendingNew confirm path.
func performNewMap(s *State, w, h int, floor byte) {
	w = core.ClampMapDimension(w)
	h = core.ClampMapDimension(h)
	s.area = materializeEntranceCrystal(blankArea(w, h, floor))
	s.baseline = core.CloneArea(s.area)
	s.undo = nil
	s.redo = nil
	invalidateContentCaches(s) // fresh area — rebuild reachability + epoch caches lazily
	s.dirty = false
	clearSelection(s) // new map — old selection coords no longer apply
	// Reset Levels-panel / active-floor state (shared with openSelectedMap) so a stale
	// editLevel can't lift the first paint onto a nonexistent floor.
	surfaceAreaLevels(s)
	s.zoom = 1
	s.panX, s.panY = 0, 0
	s.flash("New map")
}

// sanitizeFilename wraps core.SanitizeFilename with the editor's "untitled"
// fallback for all-strippable input.
func sanitizeFilename(name string) string {
	return core.SanitizeFilename(name, "untitled")
}

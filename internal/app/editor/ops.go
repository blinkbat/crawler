package editor

import (
	"crawler/internal/app/core"
	"crawler/internal/app/core/mapfile"
	"fmt"
	"os"
	"path/filepath"
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
		if s.area.Walls[fz][fx] == core.TileRock {
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
		s.area.PackSpawns = removePackAt(s.area.PackSpawns, x, z)
		s.area.ChestSpawns = removeChestSpawnAt(s.area.ChestSpawns, x, z)
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
			if s.area.Walls[fz][fx] == core.TileRock {
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
	if s.area.Walls[z][x] == core.TileRock {
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
			if s.area.Walls[fz][fx] == core.TileRock {
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
			s.area.PackSpawns = removePackAt(s.area.PackSpawns, fx, fz)
			s.area.ChestSpawns = removeChestSpawnAt(s.area.ChestSpawns, fx, fz)
		}
		return
	}
	if s.area.Walls[z][x] == core.TileRock {
		s.flash("Props need an open cell (remove the wall first)")
		return
	}
	if s.area.StartTileX == x && s.area.StartTileZ == z {
		s.flash("Cell holds the player start")
		return
	}
	setLayerCell(&s.area.Props, x, z, c)
	// A prop occupies the floor square; auto-clear any decor on it.
	setLayerCell(&s.area.Decor, x, z, core.DecorAuto)
	// And remove a pack / chest that would now be inside the prop.
	s.area.PackSpawns = removePackAt(s.area.PackSpawns, x, z)
	s.area.ChestSpawns = removeChestSpawnAt(s.area.ChestSpawns, x, z)
}

func applyEntityBrush(s *State, x, z int, kind entityKind) {
	if s.area.Walls[z][x] == core.TileRock {
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
		s.area.StartTileX = x
		s.area.StartTileZ = z
		s.dirty = true
	case entityAddEnemy:
		addPackMember(s, x, z, brush.EnemyKind)
	case entityPlaceChest:
		placeChestAt(s, x, z)
	}
}

// placeChestAt drops a chest with the default starter loot at (x,z). If
// a chest is already there, refuse (the author can right-click to clear
// and replace). Chests can't share a tile with a pack — the in-game
// interact prompt would race the pack's start-battle trigger.
func placeChestAt(s *State, x, z int) {
	if s.area.StartTileX == x && s.area.StartTileZ == z {
		s.flash("Cell holds the player start")
		return
	}
	if s.area.Walls[z][x] == core.TileRock {
		s.flash("Chest needs an open cell (remove the wall first)")
		return
	}
	if core.IsPropChar(s.area.Props[z][x]) {
		s.flash("Cell already holds a prop — clear it first")
		return
	}
	if packIndexAt(s.area.PackSpawns, x, z) >= 0 {
		s.flash("Cell already holds a pack — clear it first")
		return
	}
	if chestSpawnIndexAt(s.area.ChestSpawns, x, z) >= 0 {
		s.flash("Cell already holds a chest")
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

// chestSpawnIndexAt is the editor's counterpart to runtime ChestIndexAt
// — searches the authored ChestSpawns slice rather than the runtime
// Chests slice. Returns -1 when no chest is at the tile.
func chestSpawnIndexAt(spawns []core.ChestSpawn, x, z int) int {
	for i, sp := range spawns {
		if sp.TileX == x && sp.TileZ == z {
			return i
		}
	}
	return -1
}

// removeChestSpawnAt drops the chest at (x, z) from spawns (if any),
// returning a fresh slice. Thin wrapper around filterChests so the
// "by predicate" filter pattern is the single source of truth, mirroring
// removePackAt → filterPacks.
func removeChestSpawnAt(spawns []core.ChestSpawn, x, z int) []core.ChestSpawn {
	return filterChests(spawns, func(sp core.ChestSpawn) bool {
		return sp.TileX != x || sp.TileZ != z
	})
}

// filterChests returns a fresh slice of just the chests for which keep
// returns true. Mirrors filterPacks so chest-spawn filtering uses the
// same "allocate-fresh, reuse-backing-storage" contract — callers
// can't accidentally hold a reference to the original slice's tail.
func filterChests(spawns []core.ChestSpawn, keep func(core.ChestSpawn) bool) []core.ChestSpawn {
	out := spawns[:0:0]
	for _, sp := range spawns {
		if keep(sp) {
			out = append(out, sp)
		}
	}
	return out
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
		if s.area.StartTileX == x && s.area.StartTileZ == z {
			s.flash("Player start can't be erased; place it elsewhere instead")
			return
		}
		packsBefore := len(s.area.PackSpawns)
		s.area.PackSpawns = removePackAt(s.area.PackSpawns, x, z)
		chestsBefore := len(s.area.ChestSpawns)
		s.area.ChestSpawns = removeChestSpawnAt(s.area.ChestSpawns, x, z)
		if len(s.area.PackSpawns) == packsBefore && len(s.area.ChestSpawns) == chestsBefore {
			return
		}
	}
	s.dirty = true
}

// addPackMember appends a member of `kind` to the pack at (x,z). If no pack
// exists at the tile, creates a fresh pack with the single member.
func addPackMember(s *State, x, z int, kind core.EnemyKind) {
	if s.area.StartTileX == x && s.area.StartTileZ == z {
		s.flash("Cell holds the player start")
		return
	}
	if chestSpawnIndexAt(s.area.ChestSpawns, x, z) >= 0 {
		s.flash("Cell already holds a chest — clear it first")
		return
	}
	if idx := packIndexAt(s.area.PackSpawns, x, z); idx >= 0 {
		s.area.PackSpawns[idx].Members = append(s.area.PackSpawns[idx].Members, kind)
		s.dirty = true
		return
	}
	s.area.PackSpawns = append(s.area.PackSpawns, core.PackSpawn{
		TileX:   x,
		TileZ:   z,
		Members: []core.EnemyKind{kind},
	})
	s.dirty = true
}

func removePackAt(packs []core.PackSpawn, x, z int) []core.PackSpawn {
	return filterPacks(packs, func(sp core.PackSpawn) bool {
		return sp.TileX != x || sp.TileZ != z
	})
}

// packSpawnLeaderKind picks the kind to render as a pack's field icon —
// the highest-Tier member, ties broken by member order. Delegates to
// core.PackLeaderKind so editor and runtime share one leader rule.
func packSpawnLeaderKind(sp core.PackSpawn) core.EnemyKind {
	return core.PackLeaderKind(sp.Members)
}

// filterPacks returns a fresh slice containing only the packs for which
// keep returns true. Allocates on first append (cap=0 base) so the input
// slice isn't aliased.
func filterPacks(packs []core.PackSpawn, keep func(core.PackSpawn) bool) []core.PackSpawn {
	out := packs[:0:0]
	for _, sp := range packs {
		if keep(sp) {
			out = append(out, sp)
		}
	}
	return out
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
	snap := cloneArea(s.area)
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
	s.redo = append(s.redo, cloneArea(s.area))
	s.area = last
	// Stepping back to a snapshot that matches the on-disk baseline should
	// drop the dirty marker — don't pester the user with the unsaved-changes
	// modal if their working state is identical to what's on disk.
	s.dirty = !areasEqual(s.area, s.baseline)
}

func redoOne(s *State) {
	if len(s.redo) == 0 {
		s.flash("Nothing to redo")
		return
	}
	last := s.redo[len(s.redo)-1]
	s.redo = s.redo[:len(s.redo)-1]
	s.undo = append(s.undo, cloneArea(s.area))
	s.area = last
	s.dirty = !areasEqual(s.area, s.baseline)
}

// areasEqual returns true when two areas have identical content. Used by
// undo/redo to detect "we're back at the on-disk baseline" so the dirty
// marker can clear. Compares headers + per-layer rows + spawns in order.
func areasEqual(a, b core.AreaDefinition) bool {
	if a.Name != b.Name || a.Width != b.Width || a.Height != b.Height ||
		a.Materials != b.Materials ||
		a.StartTileX != b.StartTileX || a.StartTileZ != b.StartTileZ ||
		a.StartFacing != b.StartFacing ||
		a.QuietMessage != b.QuietMessage {
		return false
	}
	// Every grid layer cloneArea copies must be compared here too — drift
	// between the two would let undo's "back at baseline" check return true
	// while real edits sit in the working state. Ceiling was the bug that
	// triggered this audit; reorder additions so both functions iterate
	// matching layer lists.
	if !rowsEqual(a.Walls, b.Walls) || !rowsEqual(a.Floor, b.Floor) ||
		!rowsEqual(a.Decor, b.Decor) || !rowsEqual(a.Props, b.Props) ||
		!rowsEqual(a.Ceiling, b.Ceiling) {
		return false
	}
	if len(a.PackSpawns) != len(b.PackSpawns) {
		return false
	}
	for i := range a.PackSpawns {
		pa, pb := a.PackSpawns[i], b.PackSpawns[i]
		if pa.TileX != pb.TileX || pa.TileZ != pb.TileZ || len(pa.Members) != len(pb.Members) {
			return false
		}
		for j := range pa.Members {
			if pa.Members[j] != pb.Members[j] {
				return false
			}
		}
	}
	if len(a.ChestSpawns) != len(b.ChestSpawns) {
		return false
	}
	for i := range a.ChestSpawns {
		ca, cb := a.ChestSpawns[i], b.ChestSpawns[i]
		if ca.TileX != cb.TileX || ca.TileZ != cb.TileZ || len(ca.Items) != len(cb.Items) {
			return false
		}
		for j := range ca.Items {
			if ca.Items[j] != cb.Items[j] {
				return false
			}
		}
	}
	return true
}

func rowsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func cloneArea(a core.AreaDefinition) core.AreaDefinition {
	out := a
	out.Walls = append([]string(nil), a.Walls...)
	out.Floor = append([]string(nil), a.Floor...)
	out.Decor = append([]string(nil), a.Decor...)
	out.Props = append([]string(nil), a.Props...)
	out.Ceiling = append([]string(nil), a.Ceiling...)
	out.PackSpawns = make([]core.PackSpawn, len(a.PackSpawns))
	for i, sp := range a.PackSpawns {
		out.PackSpawns[i] = core.PackSpawn{
			TileX:   sp.TileX,
			TileZ:   sp.TileZ,
			Members: append([]core.EnemyKind(nil), sp.Members...),
		}
	}
	out.ChestSpawns = make([]core.ChestSpawn, len(a.ChestSpawns))
	for i, sp := range a.ChestSpawns {
		out.ChestSpawns[i] = core.ChestSpawn{
			TileX: sp.TileX,
			TileZ: sp.TileZ,
			Items: append([]core.ItemKind(nil), sp.Items...),
		}
	}
	return out
}

// resize grows or shrinks every layer to (w,h). New cells default to the
// layer's blank value (walls border-only, others auto). Player start and
// enemy spawns outside the new bounds are clamped (start) or removed
// (spawns).
func resize(s *State, w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	// Match the typed-input clamp so the +/- buttons can't grow past the
	// editor's hard ceiling either, and floor at MinMapDimension so the
	// border walls don't consume the whole playable interior.
	if w < core.MinMapDimension {
		s.flash("Map width too small (min " + strconv.Itoa(core.MinMapDimension) + ")")
		w = core.MinMapDimension
	}
	if h < core.MinMapDimension {
		s.flash("Map height too small (min " + strconv.Itoa(core.MinMapDimension) + ")")
		h = core.MinMapDimension
	}
	if w > core.MaxMapDimension {
		w = core.MaxMapDimension
	}
	if h > core.MaxMapDimension {
		h = core.MaxMapDimension
	}
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
	s.area.PackSpawns = filterPacks(s.area.PackSpawns, func(sp core.PackSpawn) bool {
		return sp.TileX < w && sp.TileZ < h
	})
	s.area.ChestSpawns = removeChestSpawnsOutside(s.area.ChestSpawns, w, h)
	s.dirty = true
}

// removeChestSpawnsOutside drops chest entries whose tile sits past
// the new bounds. Thin wrapper around filterChests so the resize path
// reuses the same filter primitive as removeChestSpawnAt.
func removeChestSpawnsOutside(spawns []core.ChestSpawn, w, h int) []core.ChestSpawn {
	return filterChests(spawns, func(sp core.ChestSpawn) bool {
		return sp.TileX < w && sp.TileZ < h
	})
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
		s.modalFilename = sanitizeFilename(s.area.Name)
		s.modal = modalSaveAs
		s.focus = focusFilename
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
	s.baseline = cloneArea(s.area)
	s.dirty = false
	s.flash("Saved " + core.MapIDFromPath(s.area.Path))
	for _, w := range reachabilityWarnings(s.area) {
		s.flash("Warning: " + w)
	}
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
			if err := os.WriteFile(path, data, 0o644); err != nil {
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

// newMap is the user-facing entry: prompts about unsaved changes if the
// current map is dirty, otherwise wipes immediately.
func newMap(s *State) {
	if s.dirty {
		s.pending = pendingNew
		s.modal = modalConfirmDirty
		return
	}
	performNewMap(s)
}

func requestOpen(s *State) {
	if s.dirty {
		s.pending = pendingOpen
		s.modal = modalConfirmDirty
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
		return
	}
	rows := make([][]byte, len(*layer))
	for i, r := range *layer {
		rows[i] = []byte(r)
	}
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
	for i, r := range rows {
		(*layer)[i] = string(r)
	}
	// Wall flood that turns cells into '#' nukes any packs that fell inside.
	if s.layer == LayerWalls && b == core.TileRock {
		s.area.PackSpawns = filterPacks(s.area.PackSpawns, func(sp core.PackSpawn) bool {
			return !s.area.BlockedAt(sp.TileX, sp.TileZ)
		})
	}
	s.dirty = true
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
	}
	return nil
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
	if a.StartTileZ < 0 || a.StartTileZ >= a.Height ||
		a.StartTileX < 0 || a.StartTileX >= a.Width {
		return []string{"start position is out of bounds"}
	}
	if a.BlockedAt(a.StartTileX, a.StartTileZ) {
		return []string{"start tile is blocked (player will spawn inside geometry)"}
	}
	if chestSpawnIndexAt(a.ChestSpawns, a.StartTileX, a.StartTileZ) >= 0 {
		return []string{"start tile holds a chest (the chest will be dropped at runtime)"}
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
	return out
}

func performNewMap(s *State) {
	s.area = blankArea(16, 16)
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

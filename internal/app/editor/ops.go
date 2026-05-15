package editor

import (
	"crawler/internal/app/core"
	"crawler/internal/app/core/mapfile"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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
	}
}

func applyDecorBrush(s *State, x, z int, c byte) {
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
	// And remove a pack that would now be inside the prop.
	s.area.PackSpawns = removePackAt(s.area.PackSpawns, x, z)
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
	switch kind {
	case entityPlayerStart:
		s.area.StartTileX = x
		s.area.StartTileZ = z
		s.dirty = true
	case entityAddRat:
		addPackMember(s, x, z, core.EnemyRat)
	case entityAddBat:
		addPackMember(s, x, z, core.EnemyBat)
	case entityAddDiseasedRat:
		addPackMember(s, x, z, core.EnemyDiseasedRat)
	}
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
		setLayerCell(&s.area.Decor, x, z, core.DecorAuto)
	case LayerProps:
		setLayerCell(&s.area.Props, x, z, core.TilePropEmpty)
	case LayerEntities:
		if s.area.StartTileX == x && s.area.StartTileZ == z {
			s.flash("Player start can't be erased; place it elsewhere instead")
			return
		}
		before := len(s.area.PackSpawns)
		s.area.PackSpawns = removePackAt(s.area.PackSpawns, x, z)
		if len(s.area.PackSpawns) == before {
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
// the highest-Tier member, ties broken by member order. Mirrors
// core.PackLeader's rule so editor and runtime stay in sync.
func packSpawnLeaderKind(sp core.PackSpawn) core.EnemyKind {
	if len(sp.Members) == 0 {
		return core.EnemyRat
	}
	leader := sp.Members[0]
	bestTier := core.EnemyInfo(leader).Tier
	for _, k := range sp.Members[1:] {
		if t := core.EnemyInfo(k).Tier; t > bestTier {
			bestTier = t
			leader = k
		}
	}
	return leader
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
// without each caller threading a reference.
func setLayerCell(layer *[]string, x, z int, b byte) {
	row := []byte((*layer)[z])
	row[x] = b
	(*layer)[z] = string(row)
}


// pushUndo snapshots the current area before a mutation. Any new mutation
// invalidates the redo stack — standard text-editor undo semantics.
// Capped at undoLimit to bound memory.
func pushUndo(s *State) {
	snap := cloneArea(s.area)
	s.undo = append(s.undo, snap)
	if len(s.undo) > undoLimit {
		s.undo = s.undo[len(s.undo)-undoLimit:]
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
	if !rowsEqual(a.Walls, b.Walls) || !rowsEqual(a.Floor, b.Floor) ||
		!rowsEqual(a.Decor, b.Decor) || !rowsEqual(a.Props, b.Props) {
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
	out.PackSpawns = make([]core.PackSpawn, len(a.PackSpawns))
	for i, sp := range a.PackSpawns {
		out.PackSpawns[i] = core.PackSpawn{
			TileX:   sp.TileX,
			TileZ:   sp.TileZ,
			Members: append([]core.EnemyKind(nil), sp.Members...),
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
	// editor's hard ceiling either.
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
	s.dirty = true
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
		paths, _ := mapfile.List(core.MapsDir())
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
// nil for layers that don't have a grid (entities).
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
	}
	return nil
}

// reachabilityWarnings reports playability problems for the area. Empty
// slice means no warnings. Used as a non-blocking check on save.
func reachabilityWarnings(a core.AreaDefinition) []string {
	var out []string
	if a.StartTileZ < 0 || a.StartTileZ >= a.Height ||
		a.StartTileX < 0 || a.StartTileX >= a.Width {
		return []string{"start position is out of bounds"}
	}
	// If the start tile itself is blocked, every enemy will *technically*
	// register as unreachable — but the player would also spawn inside
	// geometry, which is the louder problem. Surface that one warning and
	// skip the count to avoid stacking noise on top of the root cause.
	if a.BlockedAt(a.StartTileX, a.StartTileZ) {
		return []string{"start tile is blocked (player will spawn inside geometry)"}
	}
	h := a.Height
	w := a.Width
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
		if a.BlockedAt(px, pz) {
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
	if unreachable > 0 {
		out = append(out, fmt.Sprintf("%d/%d packs unreachable from start", unreachable, len(a.PackSpawns)))
	}
	if emptyRoster > 0 {
		out = append(out, fmt.Sprintf("%d/%d packs have no members", emptyRoster, len(a.PackSpawns)))
	}
	if noOpenTile > 0 {
		out = append(out, fmt.Sprintf("%d/%d packs can't fit on the map", noOpenTile, len(a.PackSpawns)))
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

func sanitizeFilename(name string) string {
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
		return "untitled"
	}
	return string(cleaned)
}

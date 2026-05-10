package editor

import (
	"crawler/internal/app/core"
	"crawler/internal/app/core/mapfile"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// applyTool runs the active tool against tile (x,z). For tile brushes this
// paints the byte and clears any spawn / start that would now be inside a
// blocking tile. For placement tools it moves the player start or adds an
// enemy spawn to the cell.
func applyTool(s *State, x, z int) {
	if !inBounds(s.area, x, z) {
		return
	}
	cur := toolEntries[s.tool]
	switch s.tool {
	case ToolFloor, ToolWall, ToolTree, ToolTreeXL, ToolBoulder, ToolBush:
		// Refuse to paint a blocking tile on top of the player start: the
		// next game load would spawn the player inside a wall, and there's
		// no good reason to allow it. Move the start first if you want this.
		if isBlockingByte(cur.tileByte) && s.area.StartTileX == x && s.area.StartTileZ == z {
			s.flash("Move the player start before walling its tile")
			return
		}
		setTile(&s.area, x, z, cur.tileByte)
		if isBlockingByte(cur.tileByte) {
			s.area.EnemySpawns = removeSpawnAt(s.area.EnemySpawns, x, z)
		}
	case ToolPlayerStart:
		if isBlockingByte(s.area.Layout[z][x]) {
			s.flash("Player start must be on a floor tile")
			return
		}
		s.area.StartTileX = x
		s.area.StartTileZ = z
	case ToolSpawnRat:
		placeSpawn(s, x, z, core.EnemyRat)
	case ToolSpawnBat:
		placeSpawn(s, x, z, core.EnemyBat)
	}
	s.dirty = true
}

// eraseAt is the right-click action: removes any enemy spawn at the cell, or
// failing that paints a floor tile.
func eraseAt(s *State, x, z int) {
	if !inBounds(s.area, x, z) {
		return
	}
	if removed := removeSpawnAt(s.area.EnemySpawns, x, z); len(removed) != len(s.area.EnemySpawns) {
		s.area.EnemySpawns = removed
		s.dirty = true
		return
	}
	setTile(&s.area, x, z, core.TileFloor)
	s.dirty = true
}

func placeSpawn(s *State, x, z int, kind core.EnemyKind) {
	if isBlockingByte(s.area.Layout[z][x]) {
		s.flash("Spawns must be on a floor tile")
		return
	}
	if s.area.StartTileX == x && s.area.StartTileZ == z {
		s.flash("Cell holds the player start")
		return
	}
	// Replace any existing spawn at this cell (regardless of kind).
	s.area.EnemySpawns = removeSpawnAt(s.area.EnemySpawns, x, z)
	s.area.EnemySpawns = append(s.area.EnemySpawns, core.EnemySpawn{Kind: kind, TileX: x, TileZ: z})
}

func removeSpawnAt(spawns []core.EnemySpawn, x, z int) []core.EnemySpawn {
	out := spawns[:0:0]
	for _, sp := range spawns {
		if sp.TileX == x && sp.TileZ == z {
			continue
		}
		out = append(out, sp)
	}
	return out
}

func setTile(a *core.AreaDefinition, x, z int, b byte) {
	row := []byte(a.Layout[z])
	row[x] = b
	a.Layout[z] = string(row)
}

func inBounds(a core.AreaDefinition, x, z int) bool {
	if z < 0 || z >= len(a.Layout) {
		return false
	}
	if x < 0 || x >= len(a.Layout[z]) {
		return false
	}
	return true
}

func isBlockingByte(b byte) bool {
	switch b {
	case core.TileRock, core.TileTree, core.TileTreeXL, core.TileRockLarge, core.TileBushLarge:
		return true
	}
	return false
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
	s.dirty = true
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
	s.dirty = true
}

func cloneArea(a core.AreaDefinition) core.AreaDefinition {
	out := a
	out.Layout = append([]string(nil), a.Layout...)
	out.EnemySpawns = append([]core.EnemySpawn(nil), a.EnemySpawns...)
	return out
}

// resize grows or shrinks the layout to (w,h). New cells default to floor;
// shrunk cells are dropped. Player start and enemy spawns outside the new
// bounds are clamped (start) or removed (spawns).
func resize(s *State, w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	pushUndo(s)
	rows := make([]string, h)
	for z := 0; z < h; z++ {
		buf := make([]byte, w)
		for x := 0; x < w; x++ {
			if z < len(s.area.Layout) && x < len(s.area.Layout[z]) {
				buf[x] = s.area.Layout[z][x]
			} else {
				buf[x] = core.TileFloor
			}
		}
		rows[z] = string(buf)
	}
	s.area.Layout = rows
	if s.area.StartTileX >= w {
		s.area.StartTileX = w - 1
	}
	if s.area.StartTileZ >= h {
		s.area.StartTileZ = h - 1
	}
	filtered := s.area.EnemySpawns[:0:0]
	for _, sp := range s.area.EnemySpawns {
		if sp.TileX < w && sp.TileZ < h {
			filtered = append(filtered, sp)
		}
	}
	s.area.EnemySpawns = filtered
	s.dirty = true
}

// saveCurrent writes to the area's existing path. If the area has never been
// saved (Path == ""), open the Save As modal so the user can name it. On a
// successful save, surfaces any reachability warnings as flashes — a heads-
// up that the saved map may be broken, but doesn't block the save itself.
func saveCurrent(s *State) {
	if s.area.Path == "" {
		s.modalFilename = sanitizeFilename(s.area.Name)
		s.modal = modalSaveAs
		s.focus = focusFilename
		return
	}
	if err := mapfile.Save(s.area.Path, core.MapFileFromArea(s.area)); err != nil {
		s.flash("Save failed: " + err.Error())
		return
	}
	s.dirty = false
	s.flash("Saved " + core.MapIDFromPath(s.area.Path))
	for _, w := range reachabilityWarnings(s.area) {
		s.flash("Warning: " + w)
	}
}

// renameMapFile renames a .map file on disk. Used by the Open modal's R key.
// Returns the new path on success.
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

// requestOpen is the user-facing Open: dirty-gated like newMap, then opens
// the picker.
func requestOpen(s *State) {
	if s.dirty {
		s.pending = pendingOpen
		s.modal = modalConfirmDirty
		return
	}
	openModal(s, modalOpen)
}

// floodFill replaces the connected region of like-tiles around (x,z) with
// b. 4-connected — diagonals stop the flood. No-op if (x,z) already holds b.
func floodFill(s *State, x, z int, b byte) {
	if !inBounds(s.area, x, z) {
		return
	}
	target := s.area.Layout[z][x]
	if target == b {
		return
	}
	rows := make([][]byte, len(s.area.Layout))
	for i, r := range s.area.Layout {
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
		s.area.Layout[i] = string(r)
	}
	if isBlockingByte(b) {
		// Newly-walled cells eat any spawns that fell inside the flood.
		filtered := s.area.EnemySpawns[:0:0]
		for _, sp := range s.area.EnemySpawns {
			if !isBlockingByte(s.area.Layout[sp.TileZ][sp.TileX]) {
				filtered = append(filtered, sp)
			}
		}
		s.area.EnemySpawns = filtered
	}
	s.dirty = true
}

// paintRect paints the brush's tile across the rectangle bounded by (x0,z0)
// and (x1,z1). Spawns inside the rect are cleared if the brush is blocking.
// Player start at the rect intersection is left in place — the user is more
// likely to want to keep it where it is than to have it silently moved.
func paintRect(s *State, x0, z0, x1, z1 int) {
	cur := toolEntries[s.tool]
	if cur.tileByte == 0 {
		return
	}
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	if z0 > z1 {
		z0, z1 = z1, z0
	}
	for z := z0; z <= z1; z++ {
		for x := x0; x <= x1; x++ {
			if !inBounds(s.area, x, z) {
				continue
			}
			if isBlockingByte(cur.tileByte) && s.area.StartTileX == x && s.area.StartTileZ == z {
				// Skip the start tile so the rect doesn't bury it.
				continue
			}
			setTile(&s.area, x, z, cur.tileByte)
			if isBlockingByte(cur.tileByte) {
				s.area.EnemySpawns = removeSpawnAt(s.area.EnemySpawns, x, z)
			}
		}
	}
	s.dirty = true
}

// reachabilityWarnings reports playability problems for the area. Empty
// slice means no warnings. Used as a non-blocking check on save.
func reachabilityWarnings(a core.AreaDefinition) []string {
	var out []string
	if a.StartTileZ < 0 || a.StartTileZ >= len(a.Layout) ||
		a.StartTileX < 0 || a.StartTileX >= len(a.Layout[0]) {
		return []string{"start position is out of bounds"}
	}
	if isBlockingByte(a.Layout[a.StartTileZ][a.StartTileX]) {
		out = append(out, "start tile is blocked (player will spawn inside geometry)")
	}
	// Flood from the start over floor tiles. Any spawn whose tile is not
	// reachable can never be encountered.
	h := len(a.Layout)
	w := len(a.Layout[0])
	visited := make([]bool, w*h)
	if !isBlockingByte(a.Layout[a.StartTileZ][a.StartTileX]) {
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
			if isBlockingByte(a.Layout[pz][px]) {
				continue
			}
			visited[idx] = true
			stack = append(stack, [2]int{px + 1, pz}, [2]int{px - 1, pz}, [2]int{px, pz + 1}, [2]int{px, pz - 1})
		}
	}
	unreachable := 0
	for _, sp := range a.EnemySpawns {
		if sp.TileZ < 0 || sp.TileZ >= h || sp.TileX < 0 || sp.TileX >= w {
			unreachable++
			continue
		}
		if !visited[sp.TileZ*w+sp.TileX] {
			unreachable++
		}
	}
	if unreachable > 0 {
		out = append(out, fmt.Sprintf("%d/%d enemies unreachable from start", unreachable, len(a.EnemySpawns)))
	}
	return out
}

// performNewMap resets state to a fresh blank map. The dirty-check gate
// happens in the input handler; this function is the unconditional reset.
func performNewMap(s *State) {
	s.area = blankArea(16, 16)
	s.undo = nil
	s.redo = nil
	s.dirty = false
	s.zoom = 1
	s.panX, s.panY = 0, 0
	s.flash("New map")
}

// mapStem returns the filename without directory or .map extension. Used to
// pre-populate Save As from the current path.
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

package core

import (
	"slices"

	"crawler/internal/app/core/mapfile"
)

// Per-floor scatter layers (props / decor) — the cube-model analogue of Solids
// for placed content. The single Props/Decor grid + PropLevels/DecorLevels tag
// can hold only ONE prop/decor per (x,z) column; PropStack/DecorStack lift that
// so each FLOOR of a column carries its own, and per-floor content never
// collides. Stack[level][z] is a width-wide row, one char per x.
//
// Migration discipline (identical to Solids): the stack stays NIL for a legacy
// single-floor map and every read DERIVES from the single grid (the prop/decor
// sits only on its PropLevelAt/DecorLevelAt floor), so old maps answer unchanged
// and round-trip byte-identical. Once a stack is materialized the legacy grid is
// FROZEN (writes route through SetProp/SetDecor into the stack) — the grid is
// still encoded/compared but no longer read for rendering, mirroring how the
// Elevation layer freezes under an explicit Solids stack.
//
// "Blank" (no content) char per layer: props use TilePropEmpty ('.'), decor uses
// DecorEmpty ('_') — so a derived non-surface floor reads as "nothing", never as
// DecorAuto ('.') which would auto-scatter on empty air.

// PropAt returns the prop char occupying floor `level` of column (x,z), or
// TilePropEmpty when none. The single read path: indexes PropStack when set,
// else derives from the Props grid placed at PropLevelAt.
func (a *AreaDefinition) PropAt(x, level, z int) byte {
	return a.scatterCharAt(a.PropStack, a.Props, a.PropLevels, TilePropEmpty, x, level, z)
}

// DecorAt returns the decor char on floor `level` of column (x,z), or DecorEmpty
// when none. Stack when set, else the Decor grid placed at DecorLevelAt.
func (a *AreaDefinition) DecorAt(x, level, z int) byte {
	return a.scatterCharAt(a.DecorStack, a.Decor, a.DecorLevels, DecorEmpty, x, level, z)
}

// scatterCharAt is the shared per-floor read for a scatter layer: the explicit
// stack cell when present, else the legacy grid char (only on the column's
// PropLevels/DecorLevels floor). OOB / out-of-range level read as `blank`.
func (a *AreaDefinition) scatterCharAt(stack [][]string, grid, levelGrid []string, blank byte, x, level, z int) byte {
	if level < 0 || !a.InBounds(x, z) {
		return blank
	}
	if len(stack) > 0 {
		if level >= len(stack) {
			return blank
		}
		if c, ok := a.layerByteAt(stack[level], x, z); ok {
			return c
		}
		return blank
	}
	// Derive: the legacy prop/decor sits only on the tile's auto/explicit floor.
	if level != a.levelGridAt(levelGrid, x, z) {
		return blank
	}
	if c, ok := a.layerByteAt(grid, x, z); ok {
		return c
	}
	return blank
}

// ScatterStackHeight is the floor count to iterate for a per-floor walk of the
// prop/decor layers: the tallest of the explicit/derived prop and decor stacks,
// at least the geometry's own height. Always >= 1.
func (a *AreaDefinition) ScatterStackHeight() int {
	h := a.SolidStackHeight()
	if n := scatterHeight(a.PropStack, a, a.Props, a.PropLevels, TilePropEmpty); n > h {
		h = n
	}
	if n := scatterHeight(a.DecorStack, a, a.Decor, a.DecorLevels, DecorEmpty); n > h {
		h = n
	}
	return h
}

// scatterHeight is the plane count of an explicit stack, else one past the
// highest floor any legacy content derives onto.
func scatterHeight(stack [][]string, a *AreaDefinition, grid, levelGrid []string, blank byte) int {
	if len(stack) > 0 {
		return len(stack)
	}
	top := 0
	a.forEachCell(func(x, z int) {
		if c, ok := a.layerByteAt(grid, x, z); ok && c != blank {
			if lv := a.levelGridAt(levelGrid, x, z); lv > top {
				top = lv
			}
		}
	})
	return top + 1
}

// buildScatterStack materializes a per-floor stack from a legacy grid: each
// content cell (char != blank) placed on its PropLevels/DecorLevels floor, every
// other cell `blank`. Inverse projection lives in scatterGridFromStack.
func buildScatterStack(a *AreaDefinition, grid, levelGrid []string, blank byte) [][]string {
	type placed struct {
		x, z, level int
		c           byte
	}
	var items []placed
	maxL := 0
	a.forEachCell(func(x, z int) {
		c, ok := a.layerByteAt(grid, x, z)
		if !ok || c == blank {
			return
		}
		lv := a.levelGridAt(levelGrid, x, z)
		if lv < 0 {
			lv = 0
		}
		if lv > maxL {
			maxL = lv
		}
		items = append(items, placed{x, z, lv, c})
	})
	planes := make([][]string, maxL+1)
	for L := range planes {
		planes[L] = mapfile.BlankLayer(a.Width, a.Height, blank)
	}
	for _, it := range items {
		row := []byte(planes[it.level][it.z])
		row[it.x] = it.c
		planes[it.level][it.z] = string(row)
	}
	return planes
}

// PropForDisplay / DecorForDisplay return the single char to show for column
// (x,z) in a 2D view (top-down editor, eyedropper, validation): the content on
// preferLevel if any, else the lowest content floor so a column still shows its
// content when a different floor is active. On a legacy (nil-stack) map this is
// exactly the single grid char, so top-down and validation are unchanged.
func (a *AreaDefinition) PropForDisplay(x, z, preferLevel int) byte {
	if len(a.PropStack) == 0 {
		c, _ := a.PropCharAt(x, z)
		return c
	}
	if c := a.PropAt(x, preferLevel, z); c != TilePropEmpty {
		return c
	}
	for L := 0; L < len(a.PropStack); L++ {
		if c := a.PropAt(x, L, z); c != TilePropEmpty {
			return c
		}
	}
	return TilePropEmpty
}

func (a *AreaDefinition) DecorForDisplay(x, z, preferLevel int) byte {
	if len(a.DecorStack) == 0 {
		c, _ := a.DecorCharAt(x, z)
		return c
	}
	if c := a.DecorAt(x, preferLevel, z); c != DecorEmpty {
		return c
	}
	for L := 0; L < len(a.DecorStack); L++ {
		if c := a.DecorAt(x, L, z); c != DecorEmpty {
			return c
		}
	}
	return DecorEmpty
}

// PropColumnLevel returns the floor a legacy column's single prop sits on, or -1
// if the column has no prop / a stack is already present. Lets the editor decide
// whether placing on a new floor would COLLIDE (needing the stack) vs overwrite.
func (a *AreaDefinition) PropColumnLevel(x, z int) int {
	if len(a.PropStack) > 0 {
		return -1
	}
	if c, ok := a.PropCharAt(x, z); ok && c != TilePropEmpty {
		return a.PropLevelAt(x, z)
	}
	return -1
}

func (a *AreaDefinition) DecorColumnLevel(x, z int) int {
	if len(a.DecorStack) > 0 {
		return -1
	}
	if c, ok := a.DecorCharAt(x, z); ok && c != DecorAuto && c != DecorEmpty {
		return a.DecorLevelAt(x, z)
	}
	return -1
}

// --- Editor authoring primitives -------------------------------------------

// EnsurePropStack / EnsureDecorStack materialize the stack from the legacy grid
// so an editor per-floor edit has a stack to write into. No-op when present.
// After this the legacy grid is frozen (see file header).
func (a *AreaDefinition) EnsurePropStack() {
	if len(a.PropStack) == 0 {
		a.PropStack = buildScatterStack(a, a.Props, a.PropLevels, TilePropEmpty)
	}
}

func (a *AreaDefinition) EnsureDecorStack() {
	if len(a.DecorStack) == 0 {
		a.DecorStack = buildScatterStack(a, a.Decor, a.DecorLevels, DecorEmpty)
	}
}

// SetProp / SetDecor place a char on floor `level` of column (x,z), materializing
// and growing the stack first. ClearProp / ClearDecor reset to the blank char.
func (a *AreaDefinition) SetProp(x, level, z int, c byte) {
	if level < 0 || !a.InBounds(x, z) {
		return
	}
	a.EnsurePropStack()
	a.PropStack = setScatterCell(a.PropStack, a.Width, a.Height, TilePropEmpty, x, level, z, c)
	a.PropStack = trimScatterTop(a.PropStack, TilePropEmpty)
}

func (a *AreaDefinition) ClearProp(x, level, z int) { a.SetProp(x, level, z, TilePropEmpty) }

func (a *AreaDefinition) SetDecor(x, level, z int, c byte) {
	if level < 0 || !a.InBounds(x, z) {
		return
	}
	a.EnsureDecorStack()
	a.DecorStack = setScatterCell(a.DecorStack, a.Width, a.Height, DecorEmpty, x, level, z, c)
	a.DecorStack = trimScatterTop(a.DecorStack, DecorEmpty)
}

func (a *AreaDefinition) ClearDecor(x, level, z int) { a.SetDecor(x, level, z, DecorEmpty) }

// setScatterCell writes c at (x,level,z), growing the stack to `level+1` blank
// planes and padding the affected row to width first.
func setScatterCell(stack [][]string, width, height int, blank byte, x, level, z int, c byte) [][]string {
	for len(stack) <= level {
		stack = append(stack, mapfile.BlankLayer(width, height, blank))
	}
	row := []byte(stack[level][z])
	for len(row) < width {
		row = append(row, blank)
	}
	row[x] = c
	stack[level][z] = string(row)
	return stack
}

// trimScatterTop drops trailing all-blank planes so the height tracks the
// highest content floor; never trims below one plane.
func trimScatterTop(stack [][]string, blank byte) [][]string {
	for len(stack) > 1 && planeAllChar(stack[len(stack)-1], blank) {
		stack = stack[:len(stack)-1]
	}
	return stack
}

// planeAllChar reports whether every cell of a plane equals c.
func planeAllChar(rows []string, c byte) bool {
	for _, r := range rows {
		for i := 0; i < len(r); i++ {
			if r[i] != c {
				return false
			}
		}
	}
	return true
}

// --- Equality (absent == derived) ------------------------------------------

// canonicalScatter normalizes a scatter layer for comparison: the explicit stack
// else the derived one, trailing all-blank planes trimmed, rows padded to width.
func (a AreaDefinition) canonicalScatter(stack [][]string, grid, levelGrid []string, blank byte) [][]string {
	s := stack
	if len(s) == 0 {
		s = buildScatterStack(&a, grid, levelGrid, blank)
	}
	return canonicalizeStack(s, a.Width, a.Height, blank)
}

// canonicalizeStack trims trailing all-blank planes then pads/normalizes each
// kept plane to (w,h) — the shared tail of canonicalSolids and canonicalScatter.
func canonicalizeStack(stack [][]string, w, h int, blank byte) [][]string {
	hi := len(stack)
	for hi > 0 && planeAllChar(stack[hi-1], blank) {
		hi--
	}
	stack = stack[:hi]
	out := make([][]string, len(stack))
	for L := range stack {
		out[L] = normalizeOptionalLayer(stack[L], w, h, blank)
	}
	return out
}

// scatterStacksEqual compares two areas' prop+decor stacks with absent==derived
// semantics. Two legacy (nil-stack) areas are fully described by their Props/
// Decor + level grids (compared in AreaContentEqual), so they short-circuit true.
func scatterStacksEqual(a, b AreaDefinition) bool {
	if len(a.PropStack) == 0 && len(b.PropStack) == 0 &&
		len(a.DecorStack) == 0 && len(b.DecorStack) == 0 {
		return true
	}
	return canonicalStacksEqual(
		a.canonicalScatter(a.PropStack, a.Props, a.PropLevels, TilePropEmpty),
		b.canonicalScatter(b.PropStack, b.Props, b.PropLevels, TilePropEmpty),
	) && canonicalStacksEqual(
		a.canonicalScatter(a.DecorStack, a.Decor, a.DecorLevels, DecorEmpty),
		b.canonicalScatter(b.DecorStack, b.Decor, b.DecorLevels, DecorEmpty),
	)
}

// --- Round-trip projection (encode) ----------------------------------------

// scatterExpressibleAsGrid reports whether a stack holds at most one content
// cell per column, so it round-trips losslessly as the single grid + level tag
// (no propstack:/decorstack: needed). A multi-floor column forces the stack.
func scatterExpressibleAsGrid(a *AreaDefinition, stack [][]string, blank byte) bool {
	expressible := true
	a.forEachCell(func(x, z int) {
		if !expressible {
			return
		}
		count := 0
		for L := range stack {
			if c, ok := a.layerByteAt(stack[L], x, z); ok && c != blank {
				count++
			}
		}
		if count > 1 {
			expressible = false
		}
	})
	return expressible
}

// scatterProjectToGrid projects a stack down to a single grid + level tag: each
// column's LOWEST content floor. Exact for an expressible stack; a lossy legacy
// downgrade for a multi-floor one (the stack stays authoritative on load). A
// content floor equal to the column's auto surface writes PropLevelAuto, so an
// all-auto projection omits the *_levels: section and stays byte-identical.
func scatterProjectToGrid(a *AreaDefinition, stack [][]string, blank byte) (grid, levelGrid []string) {
	grid = mapfile.BlankLayer(a.Width, a.Height, blank)
	levelGrid = mapfile.BlankLayer(a.Width, a.Height, PropLevelAuto)
	a.forEachCell(func(x, z int) {
		for L := range stack {
			c, ok := a.layerByteAt(stack[L], x, z)
			if !ok || c == blank {
				continue
			}
			setRowByte(grid, x, z, c)
			if L != a.levelGridAt(nil, x, z) { // nil layer ⇒ the column's auto floor
				setRowByte(levelGrid, x, z, ElevationChar(L))
			}
			return // lowest content floor only
		}
	})
	return grid, levelGrid
}

// EncodeScatterLayer returns the (grid, levelGrid, stack) a converter should
// write for one scatter layer. A nil stack passes the legacy grid through
// unchanged; a materialized stack projects to grid+levels and emits the explicit
// stack only when a column carries content on more than one floor.
func (a *AreaDefinition) EncodeScatterLayer(stack [][]string, grid, levelGrid []string, blank byte) (outGrid, outLevels []string, outStack [][]string) {
	if len(stack) == 0 {
		return cloneRows(grid), levelsForEncode(levelGrid, a.Width, a.Height), nil
	}
	pg, pl := scatterProjectToGrid(a, stack, blank)
	levels := levelsForEncode(pl, a.Width, a.Height)
	if scatterExpressibleAsGrid(a, stack, blank) {
		return pg, levels, nil
	}
	return pg, levels, CloneSolids(stack)
}

// setRowByte writes c at (x,z) in a full-width grid (rows already width-padded).
func setRowByte(rows []string, x, z int, c byte) {
	b := []byte(rows[z])
	b[x] = c
	rows[z] = string(b)
}

func canonicalStacksEqual(ca, cb [][]string) bool {
	if len(ca) != len(cb) {
		return false
	}
	for L := range ca {
		if !slices.Equal(ca[L], cb[L]) {
			return false
		}
	}
	return true
}

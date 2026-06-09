package editor

import (
	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// scrollbar.go is the editor's reusable scrollbar widget — a thin raylib
// draw + mouse-input glue layer over the pure viewport math in core/scroll.go
// (which is where the logic lives and gets unit-tested). It drives four bars:
// the brush palette, the right-hand MAP panel, and the canvas (vertical +
// horizontal). The same primitive serves all four; each call site just wires
// its own content/viewport sizes and offset storage.
//
// The widget is immediate-mode — geometry is recomputed every frame from the
// live panel sizes — with ONE bit of cross-frame memory: scrollDrag, so a
// thumb drag survives the pointer sliding off the thumb.

// scrollbarID identifies each bar so the single drag-capture slot on State can
// remember WHICH bar the mouse grabbed (only one can be dragged at a time).
type scrollbarID int

const (
	scrollNone scrollbarID = iota
	scrollPalette
	scrollMetadata
	scrollCanvasV
	scrollCanvasH
)

// scrollbarIDs is the iteration order for update + draw. Palette / metadata
// first (they sit under the canvas bars only at the panel seams, never
// overlapping), then the two canvas bars.
var scrollbarIDs = [...]scrollbarID{scrollPalette, scrollMetadata, scrollCanvasV, scrollCanvasH}

const (
	// scrollbarThickness is the gutter width (vertical bar) / height
	// (horizontal bar) in px.
	scrollbarThickness = float32(11)
	// scrollbarMinThumb floors the thumb length so it stays grabbable even
	// when the content dwarfs the viewport.
	scrollbarMinThumb = float32(26)
	// scrollbarInset is the gap between the thumb and the gutter edge so the
	// thumb reads as a raised pill rather than filling the gutter.
	scrollbarInset = float32(2)
	// scrollPageFraction is how far a click on the empty track pages the view
	// (fraction of a viewport), matching the common "click above/below the
	// thumb to jump ~a page" behavior.
	scrollPageFraction = float32(0.9)
)

// scrollDragState is the cross-frame memory for an in-flight thumb drag.
// id == scrollNone means no drag is active. grab is the pointer's offset from
// the thumb's leading edge captured at mouse-down, so the thumb tracks the
// cursor without snapping its origin under the pointer.
type scrollDragState struct {
	id   scrollbarID
	grab float32
}

// scrollbarGeom returns the gutter rect, axis, content length, and viewport
// length for scrollbar id under the current layout. ok=false when that bar is
// not applicable this frame (content fits, or the canvas has no cells). Both
// the update and the draw pass call this, so the geometry can't drift between
// hit-testing and rendering — the same single-source discipline the modal
// button helpers use.
func (s *State) scrollbarGeom(id scrollbarID) (gutter rl.Rectangle, vertical bool, contentLen, viewLen float32, ok bool) {
	switch id {
	case scrollPalette:
		viewLen = s.rect.palette.Height - headerReserve
		contentLen = paletteContentHeight(s) - headerReserve
		if viewLen <= 0 || contentLen <= viewLen {
			return
		}
		gutter = rl.NewRectangle(s.rect.palette.X+s.rect.palette.Width-scrollbarThickness,
			s.rect.palette.Y+headerReserve, scrollbarThickness, viewLen)
		return gutter, true, contentLen, viewLen, true

	case scrollMetadata:
		viewLen = s.rect.metadata.Height - headerReserve
		contentLen = metadataContentHeight(s) - headerReserve
		if viewLen <= 0 || contentLen <= viewLen {
			return
		}
		gutter = rl.NewRectangle(s.rect.metadata.X+s.rect.metadata.Width-scrollbarThickness,
			s.rect.metadata.Y+headerReserve, scrollbarThickness, viewLen)
		return gutter, true, contentLen, viewLen, true

	case scrollCanvasV:
		if s.rect.cellPx <= 0 || s.rect.gridH <= s.rect.grid.Height {
			return
		}
		trackLen := s.rect.grid.Height
		if s.rect.gridW > s.rect.grid.Width { // leave the corner for the H bar
			trackLen -= scrollbarThickness
		}
		gutter = rl.NewRectangle(s.rect.grid.X+s.rect.grid.Width-scrollbarThickness,
			s.rect.grid.Y, scrollbarThickness, trackLen)
		// viewLen is the FULL viewport (not the shrunk track) so the pannable
		// range is exact; the slightly-shorter track only affects thumb pixels.
		return gutter, true, s.rect.gridH, s.rect.grid.Height, true

	case scrollCanvasH:
		if s.rect.cellPx <= 0 || s.rect.gridW <= s.rect.grid.Width {
			return
		}
		trackLen := s.rect.grid.Width
		if s.rect.gridH > s.rect.grid.Height {
			trackLen -= scrollbarThickness
		}
		gutter = rl.NewRectangle(s.rect.grid.X,
			s.rect.grid.Y+s.rect.grid.Height-scrollbarThickness, trackLen, scrollbarThickness)
		return gutter, false, s.rect.gridW, s.rect.grid.Width, true
	}
	return
}

// scrollbarOffset reads the current scroll offset for a bar. The canvas bars
// derive theirs from the centered-origin pan; the panels read their stored
// scroll field.
func (s *State) scrollbarOffset(id scrollbarID) float32 {
	switch id {
	case scrollPalette:
		return s.paletteScroll[s.layer]
	case scrollMetadata:
		return s.metadataScroll
	case scrollCanvasV:
		return s.canvasOffset(true)
	case scrollCanvasH:
		return s.canvasOffset(false)
	}
	return 0
}

// setScrollbarOffset writes a bar's offset back to its storage (panels) or
// converts it to a pan (canvas).
func (s *State) setScrollbarOffset(id scrollbarID, v float32) {
	switch id {
	case scrollPalette:
		s.paletteScroll[s.layer] = v
	case scrollMetadata:
		s.metadataScroll = v
	case scrollCanvasV:
		s.setCanvasOffset(true, v)
	case scrollCanvasH:
		s.setCanvasOffset(false, v)
	}
}

// canvasOffset converts the centered-origin pan into a scroll offset ("how
// many px of the map are scrolled off the top/left edge"). Inverse of
// setCanvasOffset. Derivation: gridX = grid.X + (grid.W-gridW)/2 + panX, and
// offset = grid.X - gridX, which collapses to the expression below.
func (s *State) canvasOffset(vertical bool) float32 {
	if vertical {
		return (s.rect.gridH-s.rect.grid.Height)/2 - s.panY
	}
	return (s.rect.gridW-s.rect.grid.Width)/2 - s.panX
}

// setCanvasOffset maps a scroll offset back onto the pan. The layout()
// pan-clamp keeps the result honest; a scrollbar's [0,max] range maps inside
// that clamp, so it never fights the clamp.
func (s *State) setCanvasOffset(vertical bool, off float32) {
	if vertical {
		s.panY = (s.rect.gridH-s.rect.grid.Height)/2 - off
	} else {
		s.panX = (s.rect.gridW-s.rect.grid.Width)/2 - off
	}
}

// updateScrollbars runs every applicable bar's mouse interaction and returns
// true if any of them grabbed the mouse this frame (thumb drag, ongoing drag,
// or a track page-click) so the caller can suppress canvas painting / panning.
func (s *State) updateScrollbars(mp rl.Vector2) bool {
	consumed := false
	overGutter := false
	for _, id := range scrollbarIDs {
		gutter, vertical, contentLen, viewLen, ok := s.scrollbarGeom(id)
		if !ok {
			// Drop a stale drag if this bar vanished mid-drag (e.g. a resize
			// or zoom that made the content fit again).
			if s.scrollDrag.id == id {
				s.scrollDrag.id = scrollNone
			}
			continue
		}
		if pointIn(mp, gutter) {
			overGutter = true
		}
		nv, c := s.updateScrollbar(id, gutter, vertical, contentLen, viewLen, s.scrollbarOffset(id), mp)
		s.setScrollbarOffset(id, nv)
		consumed = consumed || c
	}
	// A right- or middle-click that lands on a gutter must NOT fall through to
	// the tile erase / context-menu / pan-start handlers behind the bar. (Left
	// is already handled interactively above — thumb grab or track page.) We
	// swallow only the press, not a held button, so a left paint-drag that
	// merely sweeps across a gutter isn't interrupted.
	if overGutter && (rl.IsMouseButtonPressed(rl.MouseRightButton) || rl.IsMouseButtonPressed(rl.MouseMiddleButton)) {
		consumed = true
	}
	return consumed
}

// updateScrollbar handles one bar's mouse input and returns the updated offset
// plus whether it consumed the mouse. Pure geometry comes from core/scroll.go;
// this layer is just the raylib mouse plumbing + the drag-capture bookkeeping.
func (s *State) updateScrollbar(id scrollbarID, gutter rl.Rectangle, vertical bool, contentLen, viewLen, offset float32, mp rl.Vector2) (float32, bool) {
	maxOff := core.ScrollMaxOffset(viewLen, contentLen)
	if maxOff <= 0 {
		if s.scrollDrag.id == id {
			s.scrollDrag.id = scrollNone
		}
		return 0, false
	}

	trackLen, gutterStart, mpAxis := gutter.Height, gutter.Y, mp.Y
	if !vertical {
		trackLen, gutterStart, mpAxis = gutter.Width, gutter.X, mp.X
	}
	ext := core.ScrollThumbExtent(trackLen, viewLen, contentLen, scrollbarMinThumb)
	offset = core.Clamp(offset, 0, maxOff)
	consumed := false

	if rl.IsMouseButtonPressed(rl.MouseLeftButton) && pointIn(mp, gutter) {
		thumbStart := gutterStart + core.ScrollThumbPos(trackLen, viewLen, contentLen, scrollbarMinThumb, offset)
		if mpAxis >= thumbStart && mpAxis <= thumbStart+ext {
			// Grab the thumb; remember where inside it we grabbed so it
			// doesn't snap its leading edge under the cursor.
			s.scrollDrag = scrollDragState{id: id, grab: mpAxis - thumbStart}
		} else {
			// Empty-track click: page toward the click.
			page := viewLen * scrollPageFraction
			if mpAxis < thumbStart {
				page = -page
			}
			offset = core.Clamp(offset+page, 0, maxOff)
		}
		consumed = true
	}

	if s.scrollDrag.id == id {
		if rl.IsMouseButtonDown(rl.MouseLeftButton) {
			thumbStart := mpAxis - s.scrollDrag.grab
			offset = core.ScrollOffsetForThumbPos(trackLen, viewLen, contentLen, scrollbarMinThumb, thumbStart-gutterStart)
			consumed = true
		} else {
			s.scrollDrag.id = scrollNone
		}
	}

	return offset, consumed
}

// drawScrollbars paints every applicable bar. Called from Draw after the
// panels + grid so the bars sit on top of the content they scroll.
func drawScrollbars(s *State) {
	for _, id := range scrollbarIDs {
		gutter, vertical, contentLen, viewLen, ok := s.scrollbarGeom(id)
		if !ok {
			continue
		}
		drawScrollbar(s, id, gutter, vertical, contentLen, viewLen, s.scrollbarOffset(id))
	}
}

// drawScrollbar renders one bar: a recessed gutter and a rounded thumb whose
// length and position come from the shared core math, tinted by drag / hover
// state.
func drawScrollbar(s *State, id scrollbarID, gutter rl.Rectangle, vertical bool, contentLen, viewLen, offset float32) {
	trackLen, gutterStart := gutter.Height, gutter.Y
	if !vertical {
		trackLen, gutterStart = gutter.Width, gutter.X
	}
	ext := core.ScrollThumbExtent(trackLen, viewLen, contentLen, scrollbarMinThumb)
	tp := core.ScrollThumbPos(trackLen, viewLen, contentLen, scrollbarMinThumb, offset)

	rl.DrawRectangleRec(gutter, withAlpha(bgFieldInset, 210))

	var thumb rl.Rectangle
	if vertical {
		thumb = rl.NewRectangle(gutter.X+scrollbarInset, gutterStart+tp+scrollbarInset,
			gutter.Width-2*scrollbarInset, ext-2*scrollbarInset)
	} else {
		thumb = rl.NewRectangle(gutterStart+tp+scrollbarInset, gutter.Y+scrollbarInset,
			ext-2*scrollbarInset, gutter.Height-2*scrollbarInset)
	}

	col := bgButton
	if s.scrollDrag.id == id {
		col = bgActive
	} else if pointIn(frameMouse, gutter) {
		col = bgRowHover
	}
	rl.DrawRectangleRec(thumb, col)
	rl.DrawRectangleLinesEx(thumb, 1, editorBorderDim)
}

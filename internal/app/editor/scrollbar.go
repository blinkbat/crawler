package editor

import (
	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// scrollbar.go is the editor's reusable scrollbar widget — raylib draw +
// mouse glue over the viewport math in core/scroll.go. Drives four bars: brush
// palette, MAP panel, and canvas (vertical + horizontal). Immediate-mode;
// scrollDrag is the only cross-frame memory, so a drag survives the pointer
// sliding off the thumb.

// scrollbarID identifies each bar; only one can be dragged at a time.
type scrollbarID int

const (
	scrollNone scrollbarID = iota
	scrollPalette
	scrollMetadata
	scrollCanvasV
	scrollCanvasH
	// scrollbarCount sizes the coverage assert below; keep last.
	scrollbarCount
)

// scrollbarIDs is the iteration order for update + draw.
var scrollbarIDs = [...]scrollbarID{scrollPalette, scrollMetadata, scrollCanvasV, scrollCanvasH}

// init trips at startup if the enum and scrollbarIDs drift: every real bar (all
// but scrollNone) must be listed, or it silently never updates/draws (scrollbarGeom/
// Offset/setOffset switches have no default to catch the omission).
func init() {
	if len(scrollbarIDs) != int(scrollbarCount)-1 {
		panic("editor: scrollbarIDs length must equal scrollbarCount-1 — add the new bar to scrollbarIDs so it updates + draws")
	}
}

const (
	scrollbarThickness = float32(11)  // gutter width (V) / height (H) in px
	scrollbarMinThumb  = float32(26)  // floor on thumb length so it stays grabbable
	scrollbarInset     = float32(2)   // gap between thumb and gutter edge
	scrollPageFraction = float32(0.9) // viewport fraction an empty-track click pages
)

// scrollDragState is cross-frame memory for an in-flight thumb drag. grab is
// the pointer's offset from the thumb's leading edge at mouse-down, so the
// thumb tracks the cursor without snapping its origin under the pointer.
type scrollDragState struct {
	id   scrollbarID
	grab float32
}

// scrollbarGeom returns the gutter rect, axis, content + viewport lengths for
// bar id. ok=false when the bar doesn't apply this frame (content fits / no
// cells). Single source for both hit-test and draw.
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
		// Canvas bars pan the top-down plot (panX/panY); the 3D view scrolls by
		// orbit/right-drag, so suppress them there rather than show a dead bar.
		if s.isoView || s.rect.cellPx <= 0 || s.rect.gridH <= s.rect.grid.Height {
			return
		}
		trackLen := s.rect.grid.Height
		if s.rect.gridW > s.rect.grid.Width { // leave the corner for the H bar
			trackLen -= scrollbarThickness
		}
		gutter = rl.NewRectangle(s.rect.grid.X+s.rect.grid.Width-scrollbarThickness,
			s.rect.grid.Y, scrollbarThickness, trackLen)
		// viewLen is the FULL viewport, not the shrunk track, so the pannable
		// range is exact; the shorter track only affects thumb pixels.
		return gutter, true, s.rect.gridH, s.rect.grid.Height, true

	case scrollCanvasH:
		if s.isoView || s.rect.cellPx <= 0 || s.rect.gridW <= s.rect.grid.Width {
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

// scrollbarOffset reads a bar's scroll offset (canvas bars derive from pan).
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

// setScrollbarOffset writes a bar's offset back (panels) or as a pan (canvas).
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

// canvasOffset converts the centered-origin pan into a scroll offset (px of
// the map scrolled off the top/left edge). Inverse of setCanvasOffset.
func (s *State) canvasOffset(vertical bool) float32 {
	if vertical {
		return (s.rect.gridH-s.rect.grid.Height)/2 - s.panY
	}
	return (s.rect.gridW-s.rect.grid.Width)/2 - s.panX
}

// setCanvasOffset maps a scroll offset back onto the pan. The layout() pan-clamp
// keeps it honest; a bar's [0,max] range maps inside that clamp.
func (s *State) setCanvasOffset(vertical bool, off float32) {
	if vertical {
		s.panY = (s.rect.gridH-s.rect.grid.Height)/2 - off
	} else {
		s.panX = (s.rect.gridW-s.rect.grid.Width)/2 - off
	}
}

// updateScrollbars runs every applicable bar's mouse interaction; returns true
// if any grabbed the mouse this frame so the caller suppresses canvas paint/pan.
func (s *State) updateScrollbars(mp rl.Vector2) bool {
	consumed := false
	overGutter := false
	for _, id := range scrollbarIDs {
		gutter, vertical, contentLen, viewLen, ok := s.scrollbarGeom(id)
		if !ok {
			// Drop a stale drag if this bar vanished mid-drag (resize/zoom).
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
	// A right-click on a gutter must NOT fall through to the context-menu /
	// pan-start handlers behind the bar. Swallow only the press, not a held
	// button, so a left paint-drag sweeping across isn't interrupted. (The
	// mousewheel BUTTON is never bound, so there's nothing else to guard.)
	if overGutter && rl.IsMouseButtonPressed(rl.MouseRightButton) {
		consumed = true
	}
	return consumed
}

// updateScrollbar handles one bar's mouse input, returning the updated offset
// and whether it consumed the mouse. Geometry math lives in core/scroll.go.
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
			// Grab the thumb, remembering the grab offset inside it.
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

// drawScrollbars paints every applicable bar, after panels + grid.
func drawScrollbars(s *State) {
	for _, id := range scrollbarIDs {
		gutter, vertical, contentLen, viewLen, ok := s.scrollbarGeom(id)
		if !ok {
			continue
		}
		drawScrollbar(s, id, gutter, vertical, contentLen, viewLen, s.scrollbarOffset(id))
	}
}

// drawScrollbar renders one bar: a recessed gutter + a thumb (length/pos from
// core math), tinted by drag/hover state.
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

package editor

import (
	"fmt"

	"crawler/internal/app/core"
	"crawler/internal/app/render"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Object Browser (modalObjectView): a paged 3×3 gallery of every placeable object
// (props, decor, and the 3D entities), each a live thumbnail via
// render.DrawObjectPreview. Drag a thumbnail to rotate it; wheel over one to zoom.
// render owns the art + list.

const (
	objViewModalW = float32(1140)
	objViewModalH = float32(850)
	objViewHeader = float32(44) // header band before the grid
	objViewCols   = 3
	objViewThumbH = float32(205)
	objViewLabelH = float32(18)
	objViewRowGap = float32(10) // gap between a label and the next thumb
	objViewBtnW   = float32(104)
	// objViewZoom{Min,Max} clamp a preview's wheel dolly; Rot/Zoom rates tune the
	// drag-rotate and wheel-zoom sensitivity.
	objViewZoomMin  = float32(0.4)
	objViewZoomMax  = float32(4)
	objViewRotRate  = float32(0.01)
	objViewZoomRate = float32(0.12)
)

// objPreview is one thumbnail's view pose: drag-rotate yaw/pitch (radians, added
// to the default three-quarter view) and wheel-zoom dolly (1 = fit). Zero value's
// zoom reads as 1 via objPreviewView.
type objPreview struct {
	yaw, pitch, zoom float32
}

func openObjectViewModal(s *State) {
	openModal(s, modalObjectView)
	s.objectViewPage = 0
	s.objViewView = map[int]objPreview{}
	s.objViewDrag = -1
}

// objPreviewView returns item idx's stored pose, defaulting zoom to 1 (fit).
func (s *State) objPreviewView(idx int) objPreview {
	v := s.objViewView[idx]
	if v.zoom == 0 {
		v.zoom = 1
	}
	return v
}

func (s *State) setObjPreviewView(idx int, v objPreview) {
	if s.objViewView == nil {
		s.objViewView = map[int]objPreview{}
	}
	s.objViewView[idx] = v
}

// objViewThumbsBuf is the reused gallery-cell rect buffer (single-instance modal).
var objViewThumbsBuf []rl.Rectangle

type objectViewLayout struct {
	card             rl.Rectangle
	items            []render.ObjectPreviewItem
	start, end       int
	thumbs           []rl.Rectangle
	page, pageCount  int
	prevBtn, nextBtn rl.Rectangle
	closeBtn         rl.Rectangle
}

// thumbAt returns the LOCAL thumb index (0..len-1) under mp, or -1. The object
// index is l.start + the returned value.
func thumbAt(l objectViewLayout, mp rl.Vector2) int {
	for i, r := range l.thumbs {
		if pointIn(mp, r) {
			return i
		}
	}
	return -1
}

// computeObjectViewLayout builds the current page's geometry (shared by draw +
// input) and clamps s.objectViewPage in place (over-paged values self-heal).
//
// LIFETIME: the returned layout.thumbs aliases the shared objViewThumbsBuf and is
// valid only until the next computeObjectViewLayout call. Safe today because the per-
// frame update + draw calls don't overlap; do NOT retain a layout across a second call.
func computeObjectViewLayout(s *State) objectViewLayout {
	card := centeredCardRect(objViewModalW, objViewModalH)
	items := render.ObjectPreviewItems()

	pad := modalContentInset
	gridX := card.X + pad
	gridY := card.Y + objViewHeader
	gridW := card.Width - 2*pad
	gridBottom := modalGridBottom(card)

	cellW := gridW / float32(objViewCols)
	cellH := objViewThumbH + objViewLabelH + objViewRowGap
	rows := int((gridBottom - gridY) / cellH)
	if rows < 1 {
		rows = 1
	}
	perPage := rows * objViewCols

	pageCount := (len(items) + perPage - 1) / perPage
	if pageCount < 1 {
		pageCount = 1
	}
	setObjectViewPage(s, s.objectViewPage, pageCount) // single clamp+write site
	page := s.objectViewPage

	start := page * perPage
	end := start + perPage
	if end > len(items) {
		end = len(items)
	}

	thumbW := cellW - 16
	// Reuse the buffer so layout doesn't allocate twice per frame.
	thumbs := objViewThumbsBuf[:0]
	for i := start; i < end; i++ {
		idx := i - start
		col := idx % objViewCols
		row := idx / objViewCols
		cx := gridX + float32(col)*cellW + (cellW-thumbW)/2
		cy := gridY + float32(row)*cellH
		thumbs = append(thumbs, rl.NewRectangle(cx, cy, thumbW, objViewThumbH))
	}
	objViewThumbsBuf = thumbs // retain grown capacity for next frame

	by := modalFooterButtonY(card)
	prevBtn := rl.NewRectangle(gridX, by, objViewBtnW, modalBtnH)
	nextBtn := rl.NewRectangle(prevBtn.X+objViewBtnW+modalBtnGap, by, objViewBtnW, modalBtnH)
	closeBtn := rl.NewRectangle(card.X+card.Width-pad-objViewBtnW, by, objViewBtnW, modalBtnH)

	return objectViewLayout{
		card: card, items: items, start: start, end: end, thumbs: thumbs,
		page: page, pageCount: pageCount,
		prevBtn: prevBtn, nextBtn: nextBtn, closeBtn: closeBtn,
	}
}

func updateObjectViewModal(s *State) Action {
	l := computeObjectViewLayout(s)
	if editorCancelPressed() {
		closeModal(s)
		return ActionNone
	}
	mp := rl.GetMousePosition()
	hoverThumb := thumbAt(l, mp) // local index, or -1

	// Wheel over a preview zooms THAT preview; elsewhere it pages.
	if w := rl.GetMouseWheelMove(); w != 0 {
		if hoverThumb >= 0 {
			idx := l.start + hoverThumb
			v := s.objPreviewView(idx)
			v.zoom = wheelZoom(v.zoom, w, objViewZoomRate, objViewZoomMin, objViewZoomMax)
			s.setObjPreviewView(idx, v)
		} else if w < 0 {
			setObjectViewPage(s, s.objectViewPage+1, l.pageCount)
		} else {
			setObjectViewPage(s, s.objectViewPage-1, l.pageCount)
		}
	}
	if rl.IsKeyPressed(rl.KeyRight) || rl.IsKeyPressed(rl.KeyPageDown) {
		setObjectViewPage(s, s.objectViewPage+1, l.pageCount)
	}
	if rl.IsKeyPressed(rl.KeyLeft) || rl.IsKeyPressed(rl.KeyPageUp) {
		setObjectViewPage(s, s.objectViewPage-1, l.pageCount)
	}

	if rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		switch {
		case pointIn(mp, l.prevBtn):
			setObjectViewPage(s, s.objectViewPage-1, l.pageCount)
		case pointIn(mp, l.nextBtn):
			setObjectViewPage(s, s.objectViewPage+1, l.pageCount)
		case pointIn(mp, l.closeBtn):
			closeModal(s)
			return ActionNone
		case hoverThumb >= 0:
			s.objViewDrag = l.start + hoverThumb // grab this thumbnail to drag-rotate
		case !pointIn(mp, l.card): // click-away dismisses
			closeModal(s)
			return ActionNone
		}
	}
	// Drag-rotate the grabbed preview (index survives the cursor leaving the cell).
	if s.objViewDrag >= 0 && rl.IsMouseButtonDown(rl.MouseLeftButton) {
		if d := rl.GetMouseDelta(); d.X != 0 || d.Y != 0 {
			v := s.objPreviewView(s.objViewDrag)
			v.yaw += d.X * objViewRotRate
			v.pitch -= d.Y * objViewRotRate
			s.setObjPreviewView(s.objViewDrag, v)
		}
	}
	if rl.IsMouseButtonReleased(rl.MouseLeftButton) {
		s.objViewDrag = -1
	}
	return ActionNone
}

// setObjectViewPage stores page clamped to [0, pageCount-1] — the single write site.
func setObjectViewPage(s *State, page, pageCount int) {
	s.objectViewPage = core.Clamp(page, 0, pageCount-1)
}

func drawObjectViewModal(s *State, font rl.Font, theme render.Theme) {
	l := computeObjectViewLayout(s)
	drawModalHeaderAt(font, theme, l.card, "OBJECT BROWSER", theme.BorderActive)

	for i, thumb := range l.thumbs {
		item := l.items[l.start+i]
		v := s.objPreviewView(l.start + i)
		rl.DrawRectangleRec(thumb, bgFieldInset)
		render.DrawObjectPreview(thumb, frameAssets, item, v.yaw, v.pitch, v.zoom)
		border := editorBorderDim
		if s.objViewDrag == l.start+i {
			border = editorBorderActive // highlight the thumbnail being rotated
		}
		rl.DrawRectangleLinesEx(thumb, 1, border)

		lw := render.MeasureRichText(font, item.Name, editorFontHint, 1).X
		render.DrawRichText(font, item.Name,
			rl.NewVector2(thumb.X+(thumb.Width-lw)/2, thumb.Y+thumb.Height+3),
			editorFontHint, 1, theme.TextPrimary)
	}

	drawButton(font, l.prevBtn, "‹ Prev", false)
	drawButton(font, l.nextBtn, "Next ›", false)
	drawButton(font, l.closeBtn, "Close", false)

	pageText := fmt.Sprintf("Page %d / %d   ·   %d objects   ·   drag to rotate, wheel to zoom", l.page+1, l.pageCount, len(l.items))
	pw := render.MeasureRichText(font, pageText, editorFontHint, 1).X
	render.DrawRichText(font, pageText,
		rl.NewVector2(l.card.X+(l.card.Width-pw)/2, l.prevBtn.Y+(modalBtnH-editorFontHint)/2),
		editorFontHint, 1, theme.TextHint)
}

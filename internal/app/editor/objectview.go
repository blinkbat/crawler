package editor

import (
	"fmt"

	"crawler/internal/app/render"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Object Browser (modalObjectView): a paged 3D gallery of every placeable prop,
// each a live thumbnail via render.DrawObjectPreview. render owns the art + list.

const (
	objViewModalW = float32(980)
	objViewModalH = float32(660)
	objViewHeader = float32(44) // header band before the grid
	objViewCols   = 5
	objViewThumbH = float32(130)
	objViewLabelH = float32(18)
	objViewRowGap = float32(10) // gap between a label and the next thumb
	objViewBtnW   = float32(96)
)

func openObjectViewModal(s *State) {
	openModal(s, modalObjectView)
	s.objectViewPage = 0
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

// computeObjectViewLayout builds the current page's geometry (shared by draw +
// input) and clamps s.objectViewPage in place (over-paged values self-heal).
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

	// All paging inputs funnel through setObjectViewPage (down/Right/PgDn/Next advances).
	if w := rl.GetMouseWheelMove(); w < 0 {
		setObjectViewPage(s, s.objectViewPage+1, l.pageCount)
	} else if w > 0 {
		setObjectViewPage(s, s.objectViewPage-1, l.pageCount)
	}
	if rl.IsKeyPressed(rl.KeyRight) || rl.IsKeyPressed(rl.KeyPageDown) {
		setObjectViewPage(s, s.objectViewPage+1, l.pageCount)
	}
	if rl.IsKeyPressed(rl.KeyLeft) || rl.IsKeyPressed(rl.KeyPageUp) {
		setObjectViewPage(s, s.objectViewPage-1, l.pageCount)
	}

	if rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		mp := rl.GetMousePosition()
		switch {
		case pointIn(mp, l.prevBtn):
			setObjectViewPage(s, s.objectViewPage-1, l.pageCount)
		case pointIn(mp, l.nextBtn):
			setObjectViewPage(s, s.objectViewPage+1, l.pageCount)
		case pointIn(mp, l.closeBtn):
			closeModal(s)
			return ActionNone
		case !pointIn(mp, l.card): // click-away dismisses
			closeModal(s)
			return ActionNone
		}
	}
	return ActionNone
}

// setObjectViewPage stores page clamped to [0, pageCount-1] — the single write site.
func setObjectViewPage(s *State, page, pageCount int) {
	if page < 0 {
		page = 0
	}
	if page >= pageCount {
		page = pageCount - 1
	}
	s.objectViewPage = page
}

func drawObjectViewModal(s *State, font rl.Font, theme render.Theme) {
	l := computeObjectViewLayout(s)
	drawModalHeaderAt(font, theme, l.card, "OBJECT BROWSER", theme.BorderActive)

	for i, thumb := range l.thumbs {
		item := l.items[l.start+i]
		rl.DrawRectangleRec(thumb, bgFieldInset)
		render.DrawObjectPreview(thumb, frameAssets, item, 1)
		rl.DrawRectangleLinesEx(thumb, 1, editorBorderDim)

		lw := render.MeasureRichText(font, item.Name, editorFontHint, 1).X
		render.DrawRichText(font, item.Name,
			rl.NewVector2(thumb.X+(thumb.Width-lw)/2, thumb.Y+thumb.Height+3),
			editorFontHint, 1, theme.TextPrimary)
	}

	drawButton(font, l.prevBtn, "‹ Prev", false)
	drawButton(font, l.nextBtn, "Next ›", false)
	drawButton(font, l.closeBtn, "Close", false)

	pageText := fmt.Sprintf("Page %d / %d   ·   %d objects", l.page+1, l.pageCount, len(l.items))
	pw := render.MeasureRichText(font, pageText, editorFontHint, 1).X
	render.DrawRichText(font, pageText,
		rl.NewVector2(l.card.X+(l.card.Width-pw)/2, l.prevBtn.Y+(modalBtnH-editorFontHint)/2),
		editorFontHint, 1, theme.TextHint)
}

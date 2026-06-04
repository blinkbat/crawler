package editor

import (
	"crawler/internal/app/core"
	"crawler/internal/app/render"
	"fmt"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// newMapLayout precomputes the rectangles of every interactive region
// of the new-map modal. Built each frame from current screen size so
// the modal recenters on window resize. Returned by newMapModalLayout
// and consumed by both the draw and click paths.
type newMapLayout struct {
	card        rl.Rectangle
	widthValue  rl.Rectangle
	widthMinus  rl.Rectangle
	widthPlus   rl.Rectangle
	heightValue rl.Rectangle
	heightMinus rl.Rectangle
	heightPlus  rl.Rectangle
	// floorSwatches mirrors the order of layerBrushes[LayerFloor]; each
	// rect is the clickable swatch the author picks the default tile
	// from. Index parallels the floor brush list so the byte-to-write is
	// layerBrushes[LayerFloor][i].Char.
	floorSwatches []rl.Rectangle
	createBtn     rl.Rectangle
	cancelBtn     rl.Rectangle
}

const (
	newMapCardWidth  = float32(520)
	newMapCardHeight = float32(420)
	// newMapSwatchCols controls how many floor swatches sit on one row.
	// 4 columns × 3 rows comfortably fits the 11 floor brushes; a 12th
	// would tuck into the last cell without a layout change.
	newMapSwatchCols = 4
)

func newMapModalLayout() newMapLayout {
	card := centeredCardRect(newMapCardWidth, newMapCardHeight)

	l := newMapLayout{card: card}

	// Dimensions row: value field + −/+ buttons for width on one line,
	// then the same for height. Anchors match the metadata panel's
	// dimensions row so the modal reads as the same control family.
	y := card.Y + 64
	xLeft := card.X + 20
	l.widthValue = rl.NewRectangle(xLeft+62, y, 96, 30)
	l.widthMinus = rl.NewRectangle(l.widthValue.X+l.widthValue.Width+6, y, 30, 30)
	l.widthPlus = rl.NewRectangle(l.widthMinus.X+l.widthMinus.Width+6, y, 30, 30)
	y += 42
	l.heightValue = rl.NewRectangle(xLeft+62, y, 96, 30)
	l.heightMinus = rl.NewRectangle(l.heightValue.X+l.heightValue.Width+6, y, 30, 30)
	l.heightPlus = rl.NewRectangle(l.heightMinus.X+l.heightMinus.Width+6, y, 30, 30)

	// Floor swatch grid. Each cell is 110x30 with 8px gutters so a 4×N
	// layout still fits comfortably inside the 520-wide card.
	swatchY := card.Y + 196
	swatchW := float32(110)
	swatchH := float32(30)
	gut := float32(8)
	brushes := layerBrushes[LayerFloor]
	l.floorSwatches = make([]rl.Rectangle, len(brushes))
	for i := range brushes {
		col := i % newMapSwatchCols
		row := i / newMapSwatchCols
		l.floorSwatches[i] = rl.NewRectangle(
			xLeft+float32(col)*(swatchW+gut),
			swatchY+float32(row)*(swatchH+gut),
			swatchW, swatchH)
	}

	// Footer buttons. Anchored to the card's bottom so adding swatch
	// rows doesn't push them off the card; the swatch grid is sized to
	// always leave room above this row.
	btnY := card.Y + card.Height - 50
	l.createBtn = rl.NewRectangle(card.X+card.Width-180, btnY, 80, 32)
	l.cancelBtn = rl.NewRectangle(l.createBtn.X+l.createBtn.Width+10, btnY, 80, 32)
	return l
}

// newMapFieldRect returns the active text-field rect for the new-map
// modal, used by State.activeFieldRect so click-outside-to-defocus and
// the cursor caret position both work the same as for other modals.
func newMapFieldRect(s *State) rl.Rectangle {
	l := newMapModalLayout()
	switch s.focus {
	case focusNewWidth:
		return l.widthValue
	case focusNewHeight:
		return l.heightValue
	}
	return rl.Rectangle{}
}

func drawNewMapModal(s *State, font rl.Font, theme render.Theme) {
	l := newMapModalLayout()
	drawModalHeaderAt(font, theme, l.card, "NEW MAP", theme.BorderStrong)

	// Dimensions section.
	drawLabel(font, "Width", rl.NewRectangle(l.card.X+20, l.widthValue.Y+(l.widthValue.Height-18)/2, 60, 18))
	wText := fmt.Sprintf("%d", s.modalNewWidth)
	if s.focus == focusNewWidth {
		wText = s.numericBuf
	}
	drawTextField(font, l.widthValue, wText, s.focus == focusNewWidth)
	drawStepperButtons(font, l.widthMinus, l.widthPlus)

	drawLabel(font, "Height", rl.NewRectangle(l.card.X+20, l.heightValue.Y+(l.heightValue.Height-18)/2, 60, 18))
	hText := fmt.Sprintf("%d", s.modalNewHeight)
	if s.focus == focusNewHeight {
		hText = s.numericBuf
	}
	drawTextField(font, l.heightValue, hText, s.focus == focusNewHeight)
	drawStepperButtons(font, l.heightMinus, l.heightPlus)

	// Floor section header + swatches.
	drawLabel(font, "Default floor",
		rl.NewRectangle(l.card.X+20, l.card.Y+170, 200, 18))
	brushes := layerBrushes[LayerFloor]
	mp := rl.GetMousePosition()
	for i, br := range brushes {
		r := l.floorSwatches[i]
		drawBrushSwatchRow(font, r, br.Name, LayerFloor, br,
			br.Char == s.modalNewFloor, pointIn(mp, r), 14)
	}

	// Footer buttons + hint row.
	drawButton(font, l.createBtn, "Create", false)
	drawButton(font, l.cancelBtn, "Cancel", false)
	rl.DrawTextEx(font, "Tab cycle fields   Enter create   Esc cancel",
		rl.NewVector2(l.card.X+20, l.card.Y+l.card.Height-24),
		13, 1, theme.TextHint)
}

func updateNewMapModal(s *State) Action {
	// Escape cancels — drop the modal and any pending action that
	// chained into it (the caller's flow already cleared s.dirty's
	// implication if they confirmed-discard; we just abandon the new).
	if editorCancelPressed() {
		closeModal(s)
		return ActionNone
	}

	// Enter commits the new map. closeModal clears the new-map foci
	// and numericBuf for us — no need to repeat them here.
	if editorCommitPressed() {
		commitNumericInput(s)
		performNewMap(s, s.modalNewWidth, s.modalNewHeight, s.modalNewFloor)
		closeModal(s)
		return ActionNone
	}

	if rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		mp := rl.GetMousePosition()
		l := newMapModalLayout()
		switch {
		case pointIn(mp, l.widthValue):
			s.focus = focusNewWidth
			s.numericBuf = ""
		case pointIn(mp, l.heightValue):
			s.focus = focusNewHeight
			s.numericBuf = ""
		case pointIn(mp, l.widthMinus):
			s.modalNewWidth = core.ClampMapDimension(s.modalNewWidth - 1)
		case pointIn(mp, l.widthPlus):
			s.modalNewWidth = core.ClampMapDimension(s.modalNewWidth + 1)
		case pointIn(mp, l.heightMinus):
			s.modalNewHeight = core.ClampMapDimension(s.modalNewHeight - 1)
		case pointIn(mp, l.heightPlus):
			s.modalNewHeight = core.ClampMapDimension(s.modalNewHeight + 1)
		case pointIn(mp, l.createBtn):
			commitNumericInput(s)
			performNewMap(s, s.modalNewWidth, s.modalNewHeight, s.modalNewFloor)
			closeModal(s)
			return ActionNone
		case pointIn(mp, l.cancelBtn):
			closeModal(s)
			return ActionNone
		default:
			brushes := layerBrushes[LayerFloor]
			for i, r := range l.floorSwatches {
				if pointIn(mp, r) {
					s.modalNewFloor = brushes[i].Char
					return ActionNone
				}
			}
			// Click on the card but not on any control — defocus any
			// active text field so a stray click in empty space stops
			// eating keystrokes.
			if pointIn(mp, l.card) {
				if s.focus == focusNewWidth || s.focus == focusNewHeight {
					commitNumericInput(s)
					s.focus = focusNone
					s.numericBuf = ""
				}
			}
		}
	}

	// Tab cycles between the width and height fields — handled by
	// updateNumericInput (via updateTextInput below), which commits the
	// current field then cycleFocus()es to the next. This used to ALSO
	// have an inline width↔height swap here, but it fired on the same
	// Tab press as updateNumericInput's, double-swapping back to a no-op
	// — so Tab silently did nothing. Routing solely through the shared
	// cycleFocus both de-duplicates and makes Tab actually work.
	if s.focus == focusNewWidth || s.focus == focusNewHeight {
		updateTextInput(s)
	}

	return ActionNone
}

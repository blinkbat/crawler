package editor

import (
	"crawler/internal/app/core"
	"crawler/internal/app/render"
	"fmt"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// newMapLayout holds the new-map modal's interactive rects. Rebuilt each frame so
// it recenters on resize; shared by the draw and click paths.
type newMapLayout struct {
	card        rl.Rectangle
	widthValue  rl.Rectangle
	widthMinus  rl.Rectangle
	widthPlus   rl.Rectangle
	heightValue rl.Rectangle
	heightMinus rl.Rectangle
	heightPlus  rl.Rectangle
	// floorSwatches parallels layerBrushes[LayerFloor]: swatch i writes
	// layerBrushes[LayerFloor][i].Char.
	floorSwatches []rl.Rectangle
	createBtn     rl.Rectangle
	cancelBtn     rl.Rectangle
}

const (
	newMapCardWidth  = float32(520)
	newMapCardHeight = float32(420)
	newMapSwatchCols = 4            // floor swatches per row
	newMapDimsTop    = float32(64)  // first dimensions row, below the card title
	newMapDimsRowGap = float32(42)  // pitch between the width and height rows
	newMapFloorLabel = float32(170) // "Default floor" caption, off card.Y
	newMapSwatchTop  = float32(196) // floor swatch grid top, off card.Y
	newMapHintBottom = float32(24)  // footer hint, up from the card's bottom edge
	newMapSwatchW    = float32(110) // floor swatch cell width
)

func newMapModalLayout() newMapLayout {
	card := centeredCardRect(newMapCardWidth, newMapCardHeight)

	l := newMapLayout{card: card}

	// Dimensions: value field + −/+ for width, then the same for height.
	y := card.Y + newMapDimsTop
	xLeft := card.X + 20
	l.widthValue, l.widthMinus, l.widthPlus = stepperRow(xLeft+62, y, dimStepperValueW, tightBtnGap)
	y += newMapDimsRowGap
	l.heightValue, l.heightMinus, l.heightPlus = stepperRow(xLeft+62, y, dimStepperValueW, tightBtnGap)

	// Floor swatch grid: newMapSwatchW × modalBtnH cells, modalBtnGap gutters.
	swatchY := card.Y + newMapSwatchTop
	swatchW := newMapSwatchW
	swatchH := modalBtnH
	gut := modalBtnGap
	brushes := layerBrushes[LayerFloor]
	l.floorSwatches = make([]rl.Rectangle, len(brushes))
	for i := range brushes {
		// The Erase brush (Char 0) is appended to every grid layer; it's not a real
		// floor — picking it would blank the new map's floor with NUL bytes. Leave its
		// swatch a zero rect so the draw + click loops skip it.
		if brushes[i].Erase {
			continue
		}
		col := i % newMapSwatchCols
		row := i / newMapSwatchCols
		l.floorSwatches[i] = rl.NewRectangle(
			xLeft+float32(col)*(swatchW+gut),
			swatchY+float32(row)*(swatchH+gut),
			swatchW, swatchH)
	}

	// Footer buttons, anchored bottom-right via the shared modal-button spec.
	btnY := modalFooterButtonY(card)
	btns := buttonRowAt(card.X+card.Width-modalContentInset-buttonRowWidth(newMapBtnLabels), btnY, newMapBtnLabels)
	l.createBtn, l.cancelBtn = btns[0], btns[1]
	return l
}

// newMapBtnLabels: footer labels, shared by layout and draw.
var newMapBtnLabels = []string{"Create", "Cancel"}

// newMapFieldRect returns the active text-field rect (for click-outside-defocus
// and caret position), like other modals.
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
	drawLabel(font, "Width", rl.NewRectangle(l.card.X+20, l.widthValue.Y+(l.widthValue.Height-metaLabelH)/2, 60, metaLabelH))
	wText := fmt.Sprintf("%d", s.modalNewWidth)
	if s.focus == focusNewWidth {
		wText = s.numericBuf
	}
	drawTextField(font, l.widthValue, wText, s.focus == focusNewWidth)
	drawStepperButtons(font, l.widthMinus, l.widthPlus)

	drawLabel(font, "Height", rl.NewRectangle(l.card.X+20, l.heightValue.Y+(l.heightValue.Height-metaLabelH)/2, 60, metaLabelH))
	hText := fmt.Sprintf("%d", s.modalNewHeight)
	if s.focus == focusNewHeight {
		hText = s.numericBuf
	}
	drawTextField(font, l.heightValue, hText, s.focus == focusNewHeight)
	drawStepperButtons(font, l.heightMinus, l.heightPlus)

	// Floor section header + swatches.
	drawLabel(font, "Default floor",
		rl.NewRectangle(l.card.X+20, l.card.Y+newMapFloorLabel, 200, 18))
	brushes := layerBrushes[LayerFloor]
	mp := rl.GetMousePosition()
	for i, br := range brushes {
		if br.Erase {
			continue // not a default-floor option (see layout)
		}
		r := l.floorSwatches[i]
		drawBrushSwatchRow(font, r, br.Name, LayerFloor, br,
			br.Char == s.modalNewFloor, pointIn(mp, r), 14)
	}

	// Footer buttons + hint row.
	drawModalButtons(font, []rl.Rectangle{l.createBtn, l.cancelBtn}, newMapBtnLabels)
	render.DrawRichText(font, "Click a swatch = floor   ·   Tab cycle fields   ·   Enter create   ·   Esc cancel",
		rl.NewVector2(l.card.X+20, l.card.Y+l.card.Height-newMapHintBottom),
		editorFontHint, 1, theme.TextHint)
}

func updateNewMapModal(s *State) Action {
	if editorCancelPressed() {
		closeModal(s)
		return ActionNone
	}

	// Enter commits. closeModal clears the new-map foci + numericBuf.
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
			// Fold any typed-but-uncommitted digits into the value, then defocus so
			// the field shows the stepped number (not a stale buffer) and Create
			// can't overwrite the step with the old buffer.
			commitNumericInput(s)
			s.focus = focusNone
			s.modalNewWidth = core.ClampMapDimension(s.modalNewWidth - 1)
		case pointIn(mp, l.widthPlus):
			commitNumericInput(s)
			s.focus = focusNone
			s.modalNewWidth = core.ClampMapDimension(s.modalNewWidth + 1)
		case pointIn(mp, l.heightMinus):
			commitNumericInput(s)
			s.focus = focusNone
			s.modalNewHeight = core.ClampMapDimension(s.modalNewHeight - 1)
		case pointIn(mp, l.heightPlus):
			commitNumericInput(s)
			s.focus = focusNone
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
				if brushes[i].Erase {
					continue // erase isn't a default-floor choice (zero rect anyway)
				}
				if pointIn(mp, r) {
					s.modalNewFloor = brushes[i].Char
					return ActionNone
				}
			}
			// Click on the card but not a control — defocus the text field.
			if pointIn(mp, l.card) {
				if s.focus == focusNewWidth || s.focus == focusNewHeight {
					commitNumericInput(s)
					s.focus = focusNone
					s.numericBuf = ""
				}
			}
		}
	}

	// Tab cycles width↔height via updateNumericInput→cycleFocus. Must NOT also
	// swap inline here — that fired on the same Tab press, double-swapping to a no-op.
	if s.focus == focusNewWidth || s.focus == focusNewHeight {
		updateTextInput(s)
	}

	return ActionNone
}

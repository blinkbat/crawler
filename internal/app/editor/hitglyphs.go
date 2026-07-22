package editor

import (
	"math"

	"crawler/internal/app/render"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Hit Glyphs viewer (modalHitGlyphs): read-only looping gallery of the combat
// clarity glyphs (too brief to study in play). render owns the art + names; this
// file is just the modal frame + loop clock.

const (
	hitGlyphModalW     = float32(540)
	hitGlyphCols       = 4
	hitGlyphCellW      = float32(126)
	hitGlyphCellH      = float32(118)
	hitGlyphLabelH     = float32(22)
	hitGlyphPreScale   = float32(1.6)  // gallery glyphs draw larger than in-combat
	hitGlyphLoopSecs   = float64(1.4)  // pop → animate → fade → repeat, per cell
	hitGlyphStagger    = float64(0.18) // per-cell phase offset
	hitGlyphHeaderH    = float32(52)   // header band above the glyph grid
	hitGlyphFooterH    = float32(34)   // footer band (hint row)
	hitGlyphCellInsetX = float32(5)    // backdrop inset per side within a cell
)

func openHitGlyphsModal(s *State) { openModal(s, modalHitGlyphs) }

// updateHitGlyphsModal: read-only viewer, so any key/click dismisses.
func updateHitGlyphsModal(s *State) Action {
	if anyDismissPressed() {
		closeModal(s)
	}
	return ActionNone
}

func drawHitGlyphsModal(s *State, font rl.Font, theme render.Theme) {
	names := render.EditorHitGlyphNames
	rows := (len(names) + hitGlyphCols - 1) / hitGlyphCols
	cellH := hitGlyphCellH + hitGlyphLabelH
	pw := hitGlyphModalW
	ph := hitGlyphHeaderH + float32(rows)*cellH + hitGlyphFooterH
	r := drawModalHeader(font, theme, pw, ph, "HIT GLYPHS", theme.BorderActive)

	// Loop clock: each cell's life fraction t∈[0,1), staggered by index.
	now := rl.GetTime()
	gridW := float32(hitGlyphCols) * hitGlyphCellW
	startX := r.X + (r.Width-gridW)/2
	startY := r.Y + hitGlyphHeaderH // content starts below the declared header band (was a bare 44)

	for i, name := range names {
		cellX, cellY := gridCellXY(i, hitGlyphCols, startX, startY, hitGlyphCellW, cellH)
		cx := cellX + hitGlyphCellW/2
		cy := cellY + hitGlyphCellH/2

		// Inset backdrop so the glyph reads against the card.
		cell := rl.NewRectangle(cellX+hitGlyphCellInsetX, cellY, hitGlyphCellW-2*hitGlyphCellInsetX, hitGlyphCellH)
		drawInsetCell(cell, editorBorderDim)

		t := float32(math.Mod(now+float64(i)*hitGlyphStagger, hitGlyphLoopSecs) / hitGlyphLoopSecs)
		render.EditorDrawHitGlyph(i, cx, cy, t, hitGlyphPreScale)

		lw := render.MeasureRichText(font, name, editorFontLabel, 1).X
		render.DrawRichText(font, name, rl.NewVector2(cx-lw/2, cellY+hitGlyphCellH+4), editorFontLabel, 1, theme.TextPrimary)
	}

	drawModalFooterHint(font, r, "Each plays on a loop   ·   "+dismissHint, theme)
}

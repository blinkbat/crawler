package editor

import (
	"math"

	"crawler/internal/app/render"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// hitglyphs.go is the editor's Hit Glyphs viewer (modalHitGlyphs): a read-only
// gallery of the combat "clarity glyphs" — the short-lived 2D symbols drawn over
// a struck target (slash, impact/POW, frost snowflake, spark, fire, holy, venom).
// In play they flash for ~0.4s mid-attack, so the author never gets a good look;
// this modal plays each on a loop, labeled, so the whole set is visible at once.
// Pure preview: render owns the art (render.EditorDrawHitGlyph) and the name list
// (render.EditorHitGlyphNames); this file is just the modal frame + the loop clock.

const (
	hitGlyphModalW   = float32(540)
	hitGlyphCols     = 4
	hitGlyphCellW    = float32(126)
	hitGlyphCellH    = float32(118)
	hitGlyphLabelH   = float32(22)
	hitGlyphPreScale = float32(1.6)  // gallery glyphs draw a bit larger than the in-combat size
	hitGlyphLoopSecs = float64(1.4)  // pop → animate → fade → repeat, per cell
	hitGlyphStagger  = float64(0.18) // per-cell phase offset so they don't all strike in unison
)

func openHitGlyphsModal(s *State) { openModal(s, modalHitGlyphs) }

// updateHitGlyphsModal: read-only viewer, so any key/click dismisses (mirrors the
// Validate modal). The click that opened it was consumed by the menu dropdown, so
// the modal won't self-close on its opening frame.
func updateHitGlyphsModal(s *State) Action {
	if editorCancelPressed() || editorCommitPressed() || rl.IsKeyPressed(rl.KeySpace) ||
		rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		closeModal(s)
	}
	return ActionNone
}

func drawHitGlyphsModal(s *State, font rl.Font, theme render.Theme) {
	names := render.EditorHitGlyphNames
	rows := (len(names) + hitGlyphCols - 1) / hitGlyphCols
	cellH := hitGlyphCellH + hitGlyphLabelH
	pw := hitGlyphModalW
	ph := 52 + float32(rows)*cellH + 34
	r := drawModalHeader(font, theme, pw, ph, "HIT GLYPHS", theme.BorderActive)

	// Loop clock: each cell's life fraction t∈[0,1) runs off the wall clock,
	// staggered by index so the gallery shimmers rather than pulsing in unison.
	now := rl.GetTime()
	gridW := float32(hitGlyphCols) * hitGlyphCellW
	startX := r.X + (r.Width-gridW)/2
	startY := r.Y + 44

	for i, name := range names {
		col := i % hitGlyphCols
		row := i / hitGlyphCols
		cellX := startX + float32(col)*hitGlyphCellW
		cellY := startY + float32(row)*cellH
		cx := cellX + hitGlyphCellW/2
		cy := cellY + hitGlyphCellH/2

		// Inset backdrop so the glyph reads against the modal card.
		cell := rl.NewRectangle(cellX+5, cellY, hitGlyphCellW-10, hitGlyphCellH)
		rl.DrawRectangleRec(cell, bgFieldInset)
		rl.DrawRectangleLinesEx(cell, 1, editorBorderDim)

		t := float32(math.Mod(now+float64(i)*hitGlyphStagger, hitGlyphLoopSecs) / hitGlyphLoopSecs)
		render.EditorDrawHitGlyph(i, cx, cy, t, hitGlyphPreScale)

		lw := render.MeasureRichText(font, name, editorFontLabel, 1).X
		render.DrawRichText(font, name, rl.NewVector2(cx-lw/2, cellY+hitGlyphCellH+4), editorFontLabel, 1, theme.TextPrimary)
	}

	render.DrawRichText(font, "Each plays on a loop   ·   Esc / Enter / click   close",
		rl.NewVector2(r.X+modalContentInset, r.Y+r.Height-24), editorFontHint, 1, theme.TextHint)
}

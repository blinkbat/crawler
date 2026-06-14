package editor

import (
	"fmt"
	"path/filepath"
	"strings"

	"crawler/internal/app/core"
	"crawler/internal/app/input"
	"crawler/internal/app/render"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Foe Visualizer (modalFoeView). A live combat-preview pane for ANY enemy kind
// plus a full slider stack for that kind's billboard placement, contact shadow,
// target cursor, and tint. Save writes the tuning to maps/sprites/visuals.json
// (core.EnemyVisualOverride), which the game overlays on its code-default
// visuals at load — so the author tunes a foe here and saves it straight into
// the game, no rebuild. The preview mirrors drawBattlePack's exact geometry
// (render.DrawFoePreview), so what reads right here reads right in an encounter.

// foeFields is the complete, ordered set of tunable visual fields — EVERY field
// of core.EnemyVisualOverride is here (tool-completeness: the data model is
// fully authorable from the tool, nothing left hand-edit-only). Order groups by
// concern so the two-column layout splits cleanly: placement, shadow, cursor
// (+size) fill the left column; glyph anchor+size, particle anchor+size, then
// tint fill the right. Each row is a sliderField (slider.go) whose Get/Set
// bridge the typed override fields (some uint8 for tint) to the slider's
// float64 world.
var foeFields = []sliderField[core.EnemyVisualOverride]{
	{Label: "Size X", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.SizeX) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.SizeX = float32(v) }, Min: 0.1, Max: 3.0, Step: 0.05, Format: "%.2f"},
	{Label: "Size Y", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.SizeY) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.SizeY = float32(v) }, Min: 0.1, Max: 3.0, Step: 0.05, Format: "%.2f"},
	{Label: "Y Offset", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.YOffset) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.YOffset = float32(v) }, Min: -2.0, Max: 2.0, Step: 0.02, Format: "%.2f"},
	{Label: "Depth", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.DepthOffset) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.DepthOffset = float32(v) }, Min: -2.0, Max: 3.0, Step: 0.05, Format: "%.2f"},
	{Label: "Shadow R", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.ShadowRadius) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.ShadowRadius = float32(v) }, Min: 0.0, Max: 1.5, Step: 0.02, Format: "%.2f"},
	{Label: "Shadow X", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.ShadowOffsetX) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.ShadowOffsetX = float32(v) }, Min: -1.5, Max: 1.5, Step: 0.02, Format: "%.2f"},
	{Label: "Shadow Z", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.ShadowOffsetZ) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.ShadowOffsetZ = float32(v) }, Min: -1.5, Max: 1.5, Step: 0.02, Format: "%.2f"},
	{Label: "Cursor Y", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.MarkerYOffset) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.MarkerYOffset = float32(v) }, Min: -2.0, Max: 2.0, Step: 0.02, Format: "%.2f"},
	{Label: "Cursor X", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.MarkerXOffset) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.MarkerXOffset = float32(v) }, Min: -1.5, Max: 1.5, Step: 0.02, Format: "%.2f"},
	{Label: "Cursor Sz", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.MarkerScale) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.MarkerScale = float32(v) }, Min: 0.2, Max: 3.0, Step: 0.05, Format: "%.2f"},
	{Label: "Glyph X", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.GlyphXOffset) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.GlyphXOffset = float32(v) }, Min: -1.5, Max: 1.5, Step: 0.02, Format: "%.2f"},
	{Label: "Glyph Y", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.GlyphYOffset) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.GlyphYOffset = float32(v) }, Min: -1.5, Max: 1.5, Step: 0.02, Format: "%.2f"},
	{Label: "Glyph Sz", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.GlyphScale) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.GlyphScale = float32(v) }, Min: 0.2, Max: 3.0, Step: 0.05, Format: "%.2f"},
	{Label: "Ptcl X", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.ParticleXOffset) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.ParticleXOffset = float32(v) }, Min: -1.5, Max: 1.5, Step: 0.02, Format: "%.2f"},
	{Label: "Ptcl Y", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.ParticleYOffset) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.ParticleYOffset = float32(v) }, Min: -1.5, Max: 1.5, Step: 0.02, Format: "%.2f"},
	{Label: "Ptcl Z", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.ParticleZOffset) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.ParticleZOffset = float32(v) }, Min: -1.5, Max: 1.5, Step: 0.02, Format: "%.2f"},
	{Label: "Ptcl Sz", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.ParticleScale) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.ParticleScale = float32(v) }, Min: 0.2, Max: 3.0, Step: 0.05, Format: "%.2f"},
	{Label: "Tint R", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.TintR) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.TintR = clampByte(v) }, Min: 0, Max: 255, Step: 1, Format: "%.0f"},
	{Label: "Tint G", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.TintG) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.TintG = clampByte(v) }, Min: 0, Max: 255, Step: 1, Format: "%.0f"},
	{Label: "Tint B", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.TintB) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.TintB = clampByte(v) }, Min: 0, Max: 255, Step: 1, Format: "%.0f"},
	{Label: "Tint A", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.TintA) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.TintA = clampByte(v) }, Min: 0, Max: 255, Step: 1, Format: "%.0f"},
}

func clampByte(v float64) uint8 {
	return uint8(clampRange(v, 0, 255) + 0.5)
}

// foeDrag holds the in-flight slider drag for the modal. sliderIdx == -1 means
// no drag. Package-level (mirrors soundDrag) since only one modal drags at a
// time and the state is reset on open / mouse-up.
var foeDrag = struct{ sliderIdx int }{sliderIdx: -1}

// Modal geometry. Wide card: preview pane on the left, slider stack on the
// right. Fixed size (the editor runs borderless-fullscreen, so there's room).
const (
	// Wider than the original 940 to fit the slider stack in TWO columns (the
	// field count roughly doubled with the glyph/particle/cursor-size rows); the
	// height is unchanged because two columns keep the stack short.
	foeModalW     = float32(1040)
	foeModalH     = float32(600)
	foeHeaderH    = float32(40)
	foePad        = float32(16)
	foePreviewW   = float32(420)
	foeSliderRowH = float32(30)
	foeLabelW     = float32(86)
	foeValueW     = float32(56)
	foeTrackH     = float32(12)
	foeColGap     = float32(26) // gutter between the two slider columns
)

// foeViewBtnLabels is the action row's single label source — the layout
// (buttonRowAt sizes each button to its label) and the draw read the same
// slice so the painted text and the hit rects can't drift.
var foeViewBtnLabels = []string{"Save", "Reset", "Close"}

// foeImgBtnLabels is the SPRITE-PNG editor button strip (index → applyFoeImageOp
// case). Each bakes one destructive filter into maps/sprites/<slug>.png via the
// render-side engine; "Restore" reverts the last bake from its .bak. (Importing
// a PNG is the drag-drop path handled in updateFoeViewModal, not a button.)
var foeImgBtnLabels = []string{"Tint", "Gray", "Invert", "Gradient", "Bright+", "Dark", "Contr+", "Restore"}

type foeViewLayout struct {
	card         rl.Rectangle
	preview      rl.Rectangle
	prevFoeBtn   rl.Rectangle
	nextFoeBtn   rl.Rectangle
	sliderTracks []rl.Rectangle
	imgBtns      []rl.Rectangle
	saveBtn      rl.Rectangle
	resetBtn     rl.Rectangle
	closeBtn     rl.Rectangle
}

// computeFoeViewLayout is the single geometry source the modal's draw AND its
// click hit-test share, so the painted widgets and the click rects can't drift.
func computeFoeViewLayout() foeViewLayout {
	card := centeredCardRect(foeModalW, foeModalH)
	preview := rl.NewRectangle(
		card.X+foePad,
		card.Y+foeHeaderH+foePad,
		foePreviewW,
		card.Height-foeHeaderH-foePad-50,
	)
	rightX := preview.X + preview.Width + 18
	rightW := card.X + card.Width - foePad - rightX

	nameY := card.Y + foeHeaderH + foePad
	pickBtnW := float32(32)
	prevBtn := rl.NewRectangle(rightX, nameY, pickBtnW, 28)
	nextBtn := rl.NewRectangle(rightX+rightW-pickBtnW, nameY, pickBtnW, 28)

	// Two-column slider stack: split rightW into two equal columns separated by
	// foeColGap, each column laying out label | track | value exactly as the old
	// single column did (drawFoeSlider / handleFoeViewClick read tracks[i]
	// rectangles + position the label/value relative to each, so they stay
	// column-agnostic — only the rectangles move). The left column gets the ceil
	// half so an odd count puts the extra row on the left.
	colW := (rightW - foeColGap) / 2
	colTrackW := colW - foeLabelW - foeValueW
	firstColRows := (len(foeFields) + 1) / 2
	tracks := make([]rl.Rectangle, len(foeFields))
	rowBase := nameY + 28 + 16
	for i := range foeFields {
		col, row := 0, i
		if i >= firstColRows {
			col, row = 1, i-firstColRows
		}
		colX := rightX + float32(col)*(colW+foeColGap)
		y := rowBase + float32(row)*foeSliderRowH + (foeSliderRowH-foeTrackH)/2
		tracks[i] = rl.NewRectangle(colX+foeLabelW, y, colTrackW, foeTrackH)
	}

	// Sprite-PNG editor button strip: two rows of 4, in the open band on the
	// right below the slider columns (the slider stack ends ~firstColRows down;
	// the Save/Reset/Close row is pinned to the card bottom, leaving this gap).
	imgY := rowBase + float32(firstColRows)*foeSliderRowH + 12
	imgBtns := buttonRowAt(rightX, imgY, foeImgBtnLabels[:4])
	imgBtns = append(imgBtns, buttonRowAt(rightX, imgY+float32(modalBtnH)+6, foeImgBtnLabels[4:])...)

	btns := buttonRowAt(rightX, card.Y+card.Height-modalBtnH-modalBottomInset, foeViewBtnLabels)
	saveBtn, resetBtn, closeBtn := btns[0], btns[1], btns[2]

	return foeViewLayout{
		card: card, preview: preview,
		prevFoeBtn: prevBtn, nextFoeBtn: nextBtn,
		sliderTracks: tracks,
		imgBtns:      imgBtns,
		saveBtn:      saveBtn, resetBtn: resetBtn, closeBtn: closeBtn,
	}
}

// openFoeViewModal opens the visualizer. First open seeds the working copy from
// the first kind's LIVE visual (code defaults + any saved override); later opens
// keep the working copy so unsaved tuning survives an accidental close.
func openFoeViewModal(s *State) {
	s.modal = modalFoeView
	if !s.foeInit {
		if defs := core.EnemyKinds(); len(defs) > 0 {
			s.foeKind = defs[0].Kind
		}
		seedFoeVisual(s)
		s.foeInit = true
	}
	foeDrag.sliderIdx = -1
}

// seedFoeVisual loads the current foe's live visual into the working copy and
// snapshots it as the Reset baseline.
func seedFoeVisual(s *State) {
	if ov, ok := render.LiveFoeOverride(frameAssets, s.foeKind); ok {
		s.foeVisual = ov
		s.foeBaseline = ov
	} else {
		// No resolvable visual for this kind (e.g. a missing sprite). Reset the
		// working copy + baseline to a defined override instead of leaving the
		// PREVIOUS foe's slider values in place — otherwise editing and saving
		// here would write that other foe's numbers under this kind's slug.
		// Seed a VISIBLE unit size rather than the zero value: SizeX/Y are
		// absolute billboard sizes with a 0.1 slider floor, so a zero seed would
		// be an invisible foe and a Save would persist size 0. (Defensive —
		// enemyVisualFor falls back to the Rat visual today, so this branch is
		// currently unreachable.)
		base := core.EnemyVisualOverride{SizeX: 1, SizeY: 1}
		s.foeVisual = base
		s.foeBaseline = base
	}
	s.foeCursor = 0
}

// cycleFoe steps to the prev/next enemy kind (wrapping) and re-seeds the working
// copy from that kind's live visual. Shares the registry walk with the
// custom-enemy modal's Base picker via cycleEnemyKind.
func cycleFoe(s *State, dir int) {
	s.foeKind = cycleEnemyKind(s.foeKind, dir)
	seedFoeVisual(s)
}

func saveFoeVisual(s *State) {
	slug := core.EnemySlug(s.foeKind)
	if err := core.SaveEnemyVisualOverride(slug, s.foeVisual); err != nil {
		s.flashWarn("Save failed: " + err.Error())
		return
	}
	// Mirror the save into the live in-memory visual so cycling to another foe
	// and back re-seeds from the SAVED values (LiveFoeOverride reads this map),
	// and the editor's world/preview updates immediately — otherwise the save
	// looked reverted because seedFoeVisual re-read the stale loaded value.
	render.SetLiveFoeOverride(frameAssets, s.foeKind, s.foeVisual)
	// The working copy is now what's on disk — make it the Reset baseline so a
	// later Reset reverts to the just-saved state, not pre-save edits.
	s.foeBaseline = s.foeVisual
	s.flash("Saved " + core.EnemyInfo(s.foeKind).Name + " → " + slug + " (live in editor; restart game to apply)")
}

func updateFoeViewModal(s *State) Action {
	if editorCancelPressed() {
		render.CloseFoePreview()
		closeModal(s)
		return ActionNone
	}

	// "Upload": a PNG dropped onto the window while the modal is open imports as
	// THIS foe's sprite (raylib has no file dialog; drag-drop is the path). Takes
	// the first .png in the drop; non-PNG drops are ignored.
	if rl.IsFileDropped() {
		dropped := rl.LoadDroppedFiles()
		for _, path := range dropped {
			if !strings.EqualFold(filepath.Ext(path), ".png") {
				continue
			}
			slug := core.EnemySlug(s.foeKind)
			if err := render.ImportSpriteFromFile(s.foeKind, path); err != nil {
				s.flashWarn("Import failed: " + err.Error())
			} else {
				s.flash("Imported " + filepath.Base(path) + " → " + slug + ".png (restart game to apply)")
			}
			break
		}
		// Free the C-side FilePathList raylib allocated for the drop.
		rl.UnloadDroppedFiles()
	}

	l := computeFoeViewLayout()
	// Read the cursor live (NOT the cached frameMouse, which is set in Draw and
	// is therefore one frame stale here since Update runs before Draw) so a
	// fast slider drag / click hit-tests against the current position. Matches
	// updateSoundsModal, which also reads rl.GetMousePosition() directly.
	mp := rl.GetMousePosition()
	mouseDown := rl.IsMouseButtonDown(rl.MouseLeftButton)
	mousePressed := rl.IsMouseButtonPressed(rl.MouseLeftButton)
	mouseReleased := rl.IsMouseButtonReleased(rl.MouseLeftButton)

	// Active slider drag: map mouse X within the track to a snapped value.
	if foeDrag.sliderIdx >= 0 {
		if !mouseDown {
			foeDrag.sliderIdx = -1
		} else if foeDrag.sliderIdx < len(foeFields) {
			setFoeFieldFromTrack(s, foeDrag.sliderIdx, l.sliderTracks[foeDrag.sliderIdx], mp.X)
			s.foeCursor = foeDrag.sliderIdx
		}
	}

	if mousePressed {
		handleFoeViewClick(s, &l, mp)
	}
	if mouseReleased {
		foeDrag.sliderIdx = -1
	}

	// Save on Confirm (Enter/Space). NOTE: do NOT bind 'S' here — input's
	// CursorUpDown treats 'S' as "move down" (W/S nav), so an S-save would
	// steal row navigation.
	if editorCommitPressed() {
		saveFoeVisual(s)
		return ActionNone
	}

	// Row navigation + fine value adjust.
	s.foeCursor = input.CursorUpDown(s.foeCursor, len(foeFields))
	if s.foeCursor >= 0 && s.foeCursor < len(foeFields) {
		if delta := input.CursorLeftRight(); delta != 0 {
			f := foeFields[s.foeCursor]
			v := f.Get(&s.foeVisual) + float64(delta)*f.Step
			f.Set(&s.foeVisual, clampRange(v, f.Min, f.Max))
		}
	}
	return ActionNone
}

// handleFoeViewClick dispatches a left-press inside the modal: slider tracks
// (start a drag + set the value immediately), the foe prev/next arrows, and the
// Save/Reset/Close buttons.
func handleFoeViewClick(s *State, l *foeViewLayout, mp rl.Vector2) {
	for i := range foeFields {
		if pointIn(mp, padRect(l.sliderTracks[i], 0, 9)) {
			foeDrag.sliderIdx = i
			setFoeFieldFromTrack(s, i, l.sliderTracks[i], mp.X)
			s.foeCursor = i
			return
		}
	}
	for i := range l.imgBtns {
		if pointIn(mp, l.imgBtns[i]) {
			applyFoeImageOp(s, i)
			return
		}
	}
	if pointIn(mp, l.prevFoeBtn) {
		cycleFoe(s, -1)
		return
	}
	if pointIn(mp, l.nextFoeBtn) {
		cycleFoe(s, +1)
		return
	}
	// Click the foe NAME (the span between the < > arrows) to open a dropdown of
	// every kind — jump straight to one instead of cycling. The arrows still work
	// as a quick prev/next.
	if nameSpan := nameSpanBetween(l.prevFoeBtn, l.nextFoeBtn); pointIn(mp, nameSpan) {
		openDropdownBelow(s, ddFoeKind, nameSpan)
		return
	}
	if pointIn(mp, l.saveBtn) {
		saveFoeVisual(s)
		return
	}
	if pointIn(mp, l.resetBtn) {
		s.foeVisual = s.foeBaseline
		s.flash("Reset to last-saved values")
		return
	}
	if pointIn(mp, l.closeBtn) {
		render.CloseFoePreview()
		closeModal(s)
		return
	}
}

// applyFoeImageOp bakes one sprite-PNG edit (foeImgBtnLabels[i]) for the current
// foe via the render engine, or restores the last backup. The Tint / Gradient
// ops read the modal's live Tint R/G/B sliders so the author picks a color, then
// clicks to bake it destructively into the PNG. Result applies on next launch
// (sprite textures load at boot) — the flash says so, matching the Save UX.
func applyFoeImageOp(s *State, i int) {
	slug := core.EnemySlug(s.foeKind)
	tint := rl.NewColor(s.foeVisual.TintR, s.foeVisual.TintG, s.foeVisual.TintB, 255)
	// Tint / Gradient read the tint sliders. The override's "untinted" state is
	// TintA == 0 (render's resolveTint convention), where the RGB sliders default
	// to 0,0,0 — baking that as a multiply would black the sprite out. Require a
	// deliberate color (TintA > 0) before those two ops touch the PNG.
	if (i == 0 || i == 3) && s.foeVisual.TintA == 0 {
		s.flashWarn("Set a Tint color first (raise Tint A), then Tint / Gradient")
		return
	}
	var f render.SpriteFilter
	switch i {
	case 0: // Tint — multiply by the chosen color
		f = render.SpriteFilter{TintApply: true, Tint: tint}
	case 1: // Gray
		f = render.SpriteFilter{Grayscale: true}
	case 2: // Invert
		f = render.SpriteFilter{Invert: true}
	case 3: // Gradient — wash the chosen color from top, fading to clear
		f = render.SpriteFilter{
			Gradient:   true,
			GradTop:    rl.NewColor(tint.R, tint.G, tint.B, 150),
			GradBottom: rl.NewColor(tint.R, tint.G, tint.B, 0),
		}
	case 4: // Bright+
		f = render.SpriteFilter{Brightness: 30}
	case 5: // Dark
		f = render.SpriteFilter{Brightness: -30}
	case 6: // Contr+
		f = render.SpriteFilter{Contrast: 20}
	case 7: // Restore from .bak
		if err := render.RestoreSpriteBackup(s.foeKind); err != nil {
			s.flashWarn(err.Error())
		} else {
			s.flash("Restored " + slug + ".png from backup (restart game to apply)")
		}
		return
	default:
		return
	}
	if err := render.BakeSpriteFilter(frameAssets, s.foeKind, f); err != nil {
		s.flashWarn("Bake failed: " + err.Error())
		return
	}
	s.flash("Baked " + foeImgBtnLabels[i] + " → " + slug + ".png (restart game to apply)")
}

// setFoeFieldFromTrack maps a mouse X within a slider track to the field's
// range, snapped to its step grain.
func setFoeFieldFromTrack(s *State, i int, track rl.Rectangle, mouseX float32) {
	f := foeFields[i]
	f.Set(&s.foeVisual, sliderSnap(f.Min, f.Max, f.Step, track.X, track.Width, mouseX))
}

// padRect grows r by (dx, dy) on every side — used to give the thin slider
// tracks a fatter, easier-to-hit click band without changing how they draw.
func padRect(r rl.Rectangle, dx, dy float32) rl.Rectangle {
	return rl.NewRectangle(r.X-dx, r.Y-dy, r.Width+2*dx, r.Height+2*dy)
}

func drawFoeViewModal(s *State, font rl.Font, theme render.Theme) {
	l := computeFoeViewLayout()
	drawModalHeaderAt(font, theme, l.card, "FOE VISUALIZER", theme.BorderActive)

	// Live 3D preview pane (the diorama is blitted from an off-screen texture).
	render.DrawFoePreview(l.preview, frameAssets, s.foeKind, s.foeVisual)
	rl.DrawRectangleLinesEx(l.preview, 1, theme.BorderDim)

	// Foe picker header: < Name >.
	drawButton(font, l.prevFoeBtn, "<", false)
	drawButton(font, l.nextFoeBtn, ">", false)
	name := core.EnemyInfo(s.foeKind).Name + "  ▼" // ▼ = click the name to pick from all kinds
	nameSize := rl.MeasureTextEx(font, name, editorFontTopbar, 1)
	nameSpanX := l.prevFoeBtn.X + l.prevFoeBtn.Width
	nameSpanW := l.nextFoeBtn.X - nameSpanX
	rl.DrawTextEx(font, name,
		rl.NewVector2(nameSpanX+(nameSpanW-nameSize.X)/2, l.prevFoeBtn.Y+5),
		editorFontTopbar, 1, theme.TextPrimary)

	for i := range foeFields {
		drawFoeSlider(font, theme, l, i, s)
	}

	// Sprite-PNG editor strip: a label + the bake buttons + a drag-drop hint.
	if len(l.imgBtns) > 0 {
		render.DrawTextWithShadow(font, "Sprite PNG (bakes to maps/sprites — drop a .png to import)",
			l.imgBtns[0].X, l.imgBtns[0].Y-16, 12, theme.TextMuted)
		for i := range l.imgBtns {
			drawButton(font, l.imgBtns[i], foeImgBtnLabels[i], false)
		}
	}

	drawModalButtons(font, []rl.Rectangle{l.saveBtn, l.resetBtn, l.closeBtn}, foeViewBtnLabels)

	// Footer hint + persistence note, under the preview pane (clear of the
	// right-side buttons).
	render.DrawTextWithShadow(font,
		"D-pad row/adjust   |   drag sliders   |   buttons: change foe / save / reset / close",
		l.card.X+foePad, l.preview.Y+l.preview.Height+8, 12, theme.TextHint)
	render.DrawTextWithShadow(font,
		"orange sphere = particle origin   ·   cyan = hit glyph   ·   saves to visuals.json as \""+core.EnemySlug(s.foeKind)+"\"",
		l.card.X+foePad, l.preview.Y+l.preview.Height+26, 12, theme.TextMuted)

}

func drawFoeSlider(font rl.Font, theme render.Theme, l foeViewLayout, i int, s *State) {
	f := foeFields[i]
	track := l.sliderTracks[i]
	focused := s.foeCursor == i
	value := f.Get(&s.foeVisual)
	val := fmt.Sprintf(f.Format, value)
	drawSlider(font, theme, f.Label, val, value, f.Min, f.Max,
		rl.NewVector2(track.X-foeLabelW, track.Y-4), rl.NewVector2(track.X+track.Width+8, track.Y-4),
		13, track, 6, focused)
}

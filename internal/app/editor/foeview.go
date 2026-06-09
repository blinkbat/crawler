package editor

import (
	"fmt"

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

// foeField is one editable scalar on the working override. get/set bridge the
// typed core.EnemyVisualOverride fields (some uint8 for tint) to the slider's
// float64 world; min/max/step bound it and format renders the readout.
type foeField struct {
	label  string
	get    func(*core.EnemyVisualOverride) float64
	set    func(*core.EnemyVisualOverride, float64)
	min    float64
	max    float64
	step   float64
	format string
}

// foeFields is the complete, ordered set of tunable visual fields — EVERY field
// of core.EnemyVisualOverride is here (tool-completeness: the data model is
// fully authorable from the tool, nothing left hand-edit-only). Placement,
// then shadow, then cursor, then tint.
var foeFields = []foeField{
	{label: "Size X", get: func(o *core.EnemyVisualOverride) float64 { return float64(o.SizeX) }, set: func(o *core.EnemyVisualOverride, v float64) { o.SizeX = float32(v) }, min: 0.1, max: 3.0, step: 0.05, format: "%.2f"},
	{label: "Size Y", get: func(o *core.EnemyVisualOverride) float64 { return float64(o.SizeY) }, set: func(o *core.EnemyVisualOverride, v float64) { o.SizeY = float32(v) }, min: 0.1, max: 3.0, step: 0.05, format: "%.2f"},
	{label: "Y Offset", get: func(o *core.EnemyVisualOverride) float64 { return float64(o.YOffset) }, set: func(o *core.EnemyVisualOverride, v float64) { o.YOffset = float32(v) }, min: -2.0, max: 2.0, step: 0.02, format: "%.2f"},
	{label: "Depth", get: func(o *core.EnemyVisualOverride) float64 { return float64(o.DepthOffset) }, set: func(o *core.EnemyVisualOverride, v float64) { o.DepthOffset = float32(v) }, min: -2.0, max: 3.0, step: 0.05, format: "%.2f"},
	{label: "Shadow R", get: func(o *core.EnemyVisualOverride) float64 { return float64(o.ShadowRadius) }, set: func(o *core.EnemyVisualOverride, v float64) { o.ShadowRadius = float32(v) }, min: 0.0, max: 1.5, step: 0.02, format: "%.2f"},
	{label: "Shadow X", get: func(o *core.EnemyVisualOverride) float64 { return float64(o.ShadowOffsetX) }, set: func(o *core.EnemyVisualOverride, v float64) { o.ShadowOffsetX = float32(v) }, min: -1.5, max: 1.5, step: 0.02, format: "%.2f"},
	{label: "Shadow Z", get: func(o *core.EnemyVisualOverride) float64 { return float64(o.ShadowOffsetZ) }, set: func(o *core.EnemyVisualOverride, v float64) { o.ShadowOffsetZ = float32(v) }, min: -1.5, max: 1.5, step: 0.02, format: "%.2f"},
	{label: "Cursor Y", get: func(o *core.EnemyVisualOverride) float64 { return float64(o.MarkerYOffset) }, set: func(o *core.EnemyVisualOverride, v float64) { o.MarkerYOffset = float32(v) }, min: -2.0, max: 2.0, step: 0.02, format: "%.2f"},
	{label: "Cursor X", get: func(o *core.EnemyVisualOverride) float64 { return float64(o.MarkerXOffset) }, set: func(o *core.EnemyVisualOverride, v float64) { o.MarkerXOffset = float32(v) }, min: -1.5, max: 1.5, step: 0.02, format: "%.2f"},
	{label: "Tint R", get: func(o *core.EnemyVisualOverride) float64 { return float64(o.TintR) }, set: func(o *core.EnemyVisualOverride, v float64) { o.TintR = clampByte(v) }, min: 0, max: 255, step: 1, format: "%.0f"},
	{label: "Tint G", get: func(o *core.EnemyVisualOverride) float64 { return float64(o.TintG) }, set: func(o *core.EnemyVisualOverride, v float64) { o.TintG = clampByte(v) }, min: 0, max: 255, step: 1, format: "%.0f"},
	{label: "Tint B", get: func(o *core.EnemyVisualOverride) float64 { return float64(o.TintB) }, set: func(o *core.EnemyVisualOverride, v float64) { o.TintB = clampByte(v) }, min: 0, max: 255, step: 1, format: "%.0f"},
	{label: "Tint A", get: func(o *core.EnemyVisualOverride) float64 { return float64(o.TintA) }, set: func(o *core.EnemyVisualOverride, v float64) { o.TintA = clampByte(v) }, min: 0, max: 255, step: 1, format: "%.0f"},
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
	foeModalW     = float32(940)
	foeModalH     = float32(600)
	foeHeaderH    = float32(40)
	foePad        = float32(16)
	foePreviewW   = float32(420)
	foeSliderRowH = float32(30)
	foeLabelW     = float32(86)
	foeValueW     = float32(56)
	foeTrackH     = float32(12)
)

type foeViewLayout struct {
	card         rl.Rectangle
	preview      rl.Rectangle
	prevFoeBtn   rl.Rectangle
	nextFoeBtn   rl.Rectangle
	sliderTracks []rl.Rectangle
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

	trackX := rightX + foeLabelW
	trackW := rightW - foeLabelW - foeValueW
	tracks := make([]rl.Rectangle, len(foeFields))
	rowTop := nameY + 28 + 16
	for i := range foeFields {
		tracks[i] = rl.NewRectangle(trackX, rowTop+(foeSliderRowH-foeTrackH)/2, trackW, foeTrackH)
		rowTop += foeSliderRowH
	}

	btnY := card.Y + card.Height - 42
	bw, bh, gap := float32(92), float32(30), float32(10)
	saveBtn := rl.NewRectangle(rightX, btnY, bw, bh)
	resetBtn := rl.NewRectangle(rightX+bw+gap, btnY, bw, bh)
	closeBtn := rl.NewRectangle(rightX+2*(bw+gap), btnY, bw, bh)

	return foeViewLayout{
		card: card, preview: preview,
		prevFoeBtn: prevBtn, nextFoeBtn: nextBtn,
		sliderTracks: tracks,
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
	// The working copy is now what's on disk — make it the Reset baseline so a
	// later Reset reverts to the just-saved state, not pre-save edits.
	s.foeBaseline = s.foeVisual
	s.flash("Saved " + core.EnemyInfo(s.foeKind).Name + " → " + slug + " (restart to see in game)")
}

func updateFoeViewModal(s *State) Action {
	if editorCancelPressed() {
		render.CloseFoePreview()
		closeModal(s)
		return ActionNone
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
			v := f.get(&s.foeVisual) + float64(delta)*f.step
			f.set(&s.foeVisual, clampRange(v, f.min, f.max))
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
	if pointIn(mp, l.prevFoeBtn) {
		cycleFoe(s, -1)
		return
	}
	if pointIn(mp, l.nextFoeBtn) {
		cycleFoe(s, +1)
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

// setFoeFieldFromTrack maps a mouse X within a slider track to the field's
// range, snapped to its step grain.
func setFoeFieldFromTrack(s *State, i int, track rl.Rectangle, mouseX float32) {
	f := foeFields[i]
	f.set(&s.foeVisual, sliderSnap(f.min, f.max, f.step, track.X, track.Width, mouseX))
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
	name := core.EnemyInfo(s.foeKind).Name
	nameSize := rl.MeasureTextEx(font, name, editorFontTopbar, 1)
	nameSpanX := l.prevFoeBtn.X + l.prevFoeBtn.Width
	nameSpanW := l.nextFoeBtn.X - nameSpanX
	rl.DrawTextEx(font, name,
		rl.NewVector2(nameSpanX+(nameSpanW-nameSize.X)/2, l.prevFoeBtn.Y+5),
		editorFontTopbar, 1, theme.TextPrimary)

	for i := range foeFields {
		drawFoeSlider(font, theme, l, i, s)
	}

	drawButton(font, l.saveBtn, "Save", false)
	drawButton(font, l.resetBtn, "Reset", false)
	drawButton(font, l.closeBtn, "Close", false)

	// Footer hint + persistence note, under the preview pane (clear of the
	// right-side buttons).
	render.DrawTextWithShadow(font,
		"D-pad row/adjust   |   drag sliders   |   use buttons to change foe, save, reset, or close",
		l.card.X+foePad, l.preview.Y+l.preview.Height+8, 12, theme.TextHint)
	render.DrawTextWithShadow(font,
		"Saves to maps/sprites/visuals.json as \""+core.EnemySlug(s.foeKind)+"\"",
		l.card.X+foePad, l.preview.Y+l.preview.Height+26, 12, theme.TextMuted)
}

func drawFoeSlider(font rl.Font, theme render.Theme, l foeViewLayout, i int, s *State) {
	f := foeFields[i]
	track := l.sliderTracks[i]
	focused := s.foeCursor == i
	value := f.get(&s.foeVisual)
	val := fmt.Sprintf(f.format, value)
	drawSlider(font, theme, f.label, val, value, f.min, f.max,
		rl.NewVector2(track.X-foeLabelW, track.Y-4), rl.NewVector2(track.X+track.Width+8, track.Y-4),
		13, track, 6, focused)
}

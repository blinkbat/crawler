package editor

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"

	"crawler/internal/app/core"
	"crawler/internal/app/input"
	"crawler/internal/app/render"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Foe Visualizer (modalFoeView): live combat-preview pane + slider stack for an
// enemy kind. Save writes to maps/sprites/visuals.json (core.EnemyVisualOverride),
// overlaid on code-default visuals at load. Preview mirrors drawBattlePack's
// geometry (render.DrawFoePreview).

// tintByte snaps a [0,255] slider float to a color byte (shared by the four Tint rows).
func tintByte(v float64) uint8 { return core.ClampByte(core.RoundToInt(v)) }

// foeFields is EVERY field of core.EnemyVisualOverride (tool-completeness).
// Ordered to split cleanly into the two-column layout. Each row's Get/Set bridge
// the typed override fields (uint8 for tint) to the slider's float64.
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
	{Label: "Num X", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.PopupXOffset) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.PopupXOffset = float32(v) }, Min: -1.5, Max: 1.5, Step: 0.02, Format: "%.2f"},
	{Label: "Num Y", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.PopupYOffset) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.PopupYOffset = float32(v) }, Min: -1.5, Max: 2.0, Step: 0.02, Format: "%.2f"},
	{Label: "Tint R", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.TintR) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.TintR = tintByte(v) }, Min: 0, Max: 255, Step: 1, Format: "%.0f"},
	{Label: "Tint G", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.TintG) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.TintG = tintByte(v) }, Min: 0, Max: 255, Step: 1, Format: "%.0f"},
	{Label: "Tint B", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.TintB) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.TintB = tintByte(v) }, Min: 0, Max: 255, Step: 1, Format: "%.0f"},
	{Label: "Tint A", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.TintA) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.TintA = tintByte(v) }, Min: 0, Max: 255, Step: 1, Format: "%.0f"},
}

// sliderDragState is the in-flight slider-drag cursor for the editor's slider
// modals. idx is the field under the held mouse, or -1 for no active drag.
// Package-level is fine: one modal drags at a time, single-threaded draw loop.
type sliderDragState struct {
	idx int
}

// noSliderDrag is the released/idle value (idx == -1).
var noSliderDrag = sliderDragState{idx: -1}

// update advances one drag frame: no-op idle, ends when !held, else apply(idx).
func (d *sliderDragState) update(held bool, fieldCount int, apply func(idx int)) {
	if d.idx < 0 {
		return
	}
	if !held {
		d.idx = -1
		return
	}
	if d.idx < fieldCount {
		apply(d.idx)
	}
}

// visualizerDrag holds a visualizer modal's in-flight drag: slider = Layout-tab
// field drag, asset = Asset-tab param drag (one active at a time, gated by tab).
// Named so the foe/party drag vars, the tab-switch param, and the callbacks field
// can't drift apart.
type visualizerDrag struct{ slider, asset sliderDragState }

// foeDrag holds the Foe Visualizer's in-flight drag.
var foeDrag = visualizerDrag{slider: noSliderDrag, asset: noSliderDrag}

// Modal geometry. Wide card: preview pane left, slider stack right. Despite the
// foe* prefix, these (and computeFoeViewLayout / drawFoeViewTabs below) are the
// SHARED visualizer engine — the Party Visualizer routes through them via
// drawVisualizerModal too. Only foeFields / foeViewCallbacks are foe-specific.
const (
	foeModalW     = float32(1040)
	foeModalH     = float32(600)
	foeHeaderH    = float32(40)
	foePad        = float32(16)
	foePreviewW   = float32(420)
	foeSliderRowH = float32(30)
	foeColGap     = float32(26) // gutter between the two slider columns
	foePickBtnH   = float32(28) // prev/next picker arrow height (< Name >)
	foePickBtnW   = float32(32) // prev/next picker arrow width

	// computeFoeViewLayout vertical/gutter steps.
	foePreviewGap           = float32(18) // gutter between the preview and the right column
	foePreviewBottomReserve = float32(50) // space below the preview for the footer row
	foeTabRowGap            = float32(10) // gap from the name row down to the tab row
	foeContentGap           = float32(14) // gap from the tab row down to the content
	foeAssetBtnGap          = float32(16) // gap below the asset column before its action buttons
	foeFooterHintDY         = float32(8)  // footer hint baseline below the preview pane
	foeFooterNoteDY         = float32(26) // persistence note baseline below the preview pane
	// foePreviewZoomStep is the per-wheel-notch zoom dolly (clamped render-side).
	foePreviewZoomStep = float32(0.2)
	// sliderHitPadY fattens a thin track's click band vertically (draw unchanged).
	sliderHitPadY = float32(9)
)

// foeViewBtnLabels is the single label source for the action row (layout + draw share it).
var foeViewBtnLabels = []string{"Save", "Reset", "Close"}

// foeViewTabLabels names the panes (index == State.foeViewTab).
var foeViewTabLabels = []string{"Layout", "Asset"}

const (
	foeTabLayout = 0
	foeTabAsset  = 1
)

// assetFields is the Asset tab's NON-DESTRUCTIVE image-adjustment slider stack:
// edits the override in place, persists to visuals.json, reverts by zeroing,
// applied to the pristine sprite at texture-build time. Pixelate 0..1 intensity,
// Brightness/Contrast -1..1. Shared by both modals.
var assetFields = []sliderField[core.EnemyVisualOverride]{
	{Label: "Pixelate", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.Pixelate) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.Pixelate = float32(v) }, Min: 0, Max: 1, Step: 0.05, Format: "%.2f"},
	{Label: "Bright", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.Brightness) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.Brightness = float32(v) }, Min: -1, Max: 1, Step: 0.05, Format: "%.2f"},
	{Label: "Contrast", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.Contrast) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.Contrast = float32(v) }, Min: -1, Max: 1, Step: 0.05, Format: "%.2f"},
	{Label: "Posterize", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.Posterize) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.Posterize = float32(v) }, Min: 0, Max: 1, Step: 0.05, Format: "%.2f"},
	{Label: "Saturate", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.Saturation) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.Saturation = float32(v) }, Min: -1, Max: 1, Step: 0.05, Format: "%.2f"},
	{Label: "Dither", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.Dither) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.Dither = float32(v) }, Min: 0, Max: 1, Step: 0.05, Format: "%.2f"},
	{Label: "GameBoy", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.GameBoy) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.GameBoy = float32(v) }, Min: 0, Max: 1, Step: 0.05, Format: "%.2f"},
	// Colors caps the palette to N colors (median-cut); 0 = off, 2..64 active (1 = off).
	{Label: "Colors", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.MaxColors) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.MaxColors = float32(v) }, Min: 0, Max: 64, Step: 1, Format: "%.0f"},
}

// init asserts the visualizer slider stacks (foeFields placement + assetFields FX)
// expose EVERY field of core.EnemyVisualOverride, and that each row writes exactly one
// field. A new override field added without a slider row fires here at startup, so the
// tool-completeness contract ("every field editable") can't silently regress.
func init() {
	var z core.EnemyVisualOverride
	n := reflect.TypeOf(z).NumField()
	covered := make([]bool, n)
	check := func(fields []sliderField[core.EnemyVisualOverride]) {
		for _, f := range fields {
			var probe core.EnemyVisualOverride
			f.Set(&probe, 7) // any non-zero value in every row's range
			rv := reflect.ValueOf(probe)
			hits := 0
			for i := 0; i < n; i++ {
				if !rv.Field(i).IsZero() {
					covered[i] = true
					hits++
				}
			}
			if hits != 1 {
				panic(fmt.Sprintf("foeview slider %q writes %d EnemyVisualOverride fields (want exactly 1)", f.Label, hits))
			}
		}
	}
	check(foeFields)
	check(assetFields)
	for i := 0; i < n; i++ {
		if !covered[i] {
			panic("core.EnemyVisualOverride." + reflect.TypeOf(z).Field(i).Name + " has no foeview slider row (tool-completeness)")
		}
	}
}

// assetAction is one Asset-tab button: its label and what it does to the override,
// in one row (like modalCmd / dropdownEntry) so a label can't drift from its
// behavior. Add an action by appending an entry here — no parallel index const or
// switch case to keep in step. PNG import is the drag-drop path, not a button.
type assetAction struct {
	label string
	apply func(s *State, ov *core.EnemyVisualOverride)
}

var assetActions = []assetAction{
	{"Revert", func(s *State, ov *core.EnemyVisualOverride) {
		clearVisualAdjustments(ov)
		s.assetPreviewStale = true
		s.flash("Reverted sprite FX (" + assetFieldNames() + ")")
	}},
}

// assetActionLabels are the Asset tab's button labels, derived from assetActions.
var assetActionLabels = func() []string {
	out := make([]string, len(assetActions))
	for i, a := range assetActions {
		out[i] = a.label
	}
	return out
}()

// applyAssetAction runs Asset-tab button `i` against override `ov` (shared);
// out-of-range is an explicit no-op.
func applyAssetAction(s *State, ov *core.EnemyVisualOverride, i int) {
	if i < 0 || i >= len(assetActions) {
		return
	}
	assetActions[i].apply(s, ov)
}

// savedVisualFlash is the save toast for both Visualizers (which also re-bake the
// live texture, so the change applies in-session without a restart).
func savedVisualFlash(name, slug string) string {
	return "Saved " + name + " → " + slug + " (applied live)"
}

// visualizerFooterHint is the persistence note under both preview panes. isParty
// selects partyvisuals.json vs visuals.json; slug is the map key.
func visualizerFooterHint(isParty bool, slug string) string {
	file := core.EnemyVisualsFileName
	if isParty {
		file = core.PartyVisualsFileName
	}
	return "orange sphere = particle origin   ·   cyan = hit glyph   ·   saves to " + file + " as \"" + slug + "\""
}

// clearVisualAdjustments zeroes the FX fields (Asset-tab Revert); tint/placement
// untouched. Driven off assetFields so a new slider row clears automatically.
func clearVisualAdjustments(ov *core.EnemyVisualOverride) {
	for _, f := range assetFields {
		f.Set(ov, 0)
	}
}

// assetFieldNames joins the Asset-tab labels for the Revert toast (tracks
// assetFields rather than re-spelling them).
func assetFieldNames() string {
	names := make([]string, len(assetFields))
	for i, f := range assetFields {
		names[i] = f.Label
	}
	return strings.Join(names, " / ")
}

type foeViewLayout struct {
	card         rl.Rectangle
	preview      rl.Rectangle
	prevFoeBtn   rl.Rectangle
	nextFoeBtn   rl.Rectangle
	tabBtns      []rl.Rectangle
	sliderTracks []rl.Rectangle // Layout tab (foeFields)
	assetTracks  []rl.Rectangle // Asset tab (assetFields)
	assetBtns    []rl.Rectangle // Asset tab action buttons (Revert)
	saveBtn      rl.Rectangle
	resetBtn     rl.Rectangle
	closeBtn     rl.Rectangle
}

// twoColTracks lays n slider tracks into two equal columns (left column gets the
// ceil half), each row foeSliderRowH tall from baseY; colX steps from rightX and the
// track insets by labelW. Shared by the Layout and Asset tabs.
func twoColTracks(n int, rightX, colW, colTrackW, baseY float32) []rl.Rectangle {
	firstColRows := (n + 1) / 2
	tracks := make([]rl.Rectangle, n)
	for i := 0; i < n; i++ {
		col, row := 0, i
		if i >= firstColRows {
			col, row = 1, i-firstColRows
		}
		colX := rightX + float32(col)*(colW+foeColGap)
		y := baseY + float32(row)*foeSliderRowH + (foeSliderRowH-foeSliderMetrics.trackH)/2
		tracks[i] = rl.NewRectangle(colX+foeSliderMetrics.labelW, y, colTrackW, foeSliderMetrics.trackH)
	}
	return tracks
}

// computeFoeViewLayout is the single geometry source for the modal's draw and
// hit-test (so widgets and click rects can't drift).
var (
	foeViewLayoutCache      foeViewLayout
	foeViewLayoutCacheW     int32
	foeViewLayoutCacheH     int32
	foeViewLayoutCacheReady bool
)

// computeFoeViewLayout returns the Visualizer geometry, memoized on screen size:
// its inputs are all constants + centeredCardRect(screen), so it only changes on a
// window resize. Both the modal's Update and Draw call it every frame, so caching
// collapses the ~5 slice allocations to once per resize.
func computeFoeViewLayout() foeViewLayout {
	w, h := render.ScreenSize()
	if foeViewLayoutCacheReady && foeViewLayoutCacheW == w && foeViewLayoutCacheH == h {
		return foeViewLayoutCache
	}
	l := buildFoeViewLayout()
	foeViewLayoutCache, foeViewLayoutCacheW, foeViewLayoutCacheH = l, w, h
	foeViewLayoutCacheReady = true
	return l
}

func buildFoeViewLayout() foeViewLayout {
	card := centeredCardRect(foeModalW, foeModalH)
	preview := rl.NewRectangle(
		card.X+foePad,
		card.Y+foeHeaderH+foePad,
		foePreviewW,
		card.Height-foeHeaderH-foePad-foePreviewBottomReserve,
	)
	rightX := preview.X + preview.Width + foePreviewGap
	rightW := card.X + card.Width - foePad - rightX

	nameY := card.Y + foeHeaderH + foePad
	prevBtn := rl.NewRectangle(rightX, nameY, foePickBtnW, foePickBtnH)
	nextBtn := rl.NewRectangle(rightX+rightW-foePickBtnW, nameY, foePickBtnW, foePickBtnH)

	// Tab row (Layout / Asset). Both tabs lay out at the same contentTop; only the
	// active one is drawn/hit-tested (gated on State.foeViewTab).
	tabY := nameY + foePickBtnH + foeTabRowGap
	tabBtns := buttonRowAt(rightX, tabY, foeViewTabLabels)
	contentTop := tabY + float32(modalBtnH) + foeContentGap

	// Two-column slider stacks (Layout + Asset tabs): rightW split into two equal
	// columns, left column getting the ceil half (odd count → extra row left).
	colW := (rightW - foeColGap) / 2
	colTrackW := colW - foeSliderMetrics.trackReserve(0)
	tracks := twoColTracks(len(foeFields), rightX, colW, colTrackW, contentTop)
	assetTracks := twoColTracks(len(assetFields), rightX, colW, colTrackW, contentTop)
	// Action buttons sit below the taller (left) asset column.
	assetBtnY := contentTop + float32((len(assetFields)+1)/2)*foeSliderRowH + foeAssetBtnGap
	assetBtns := buttonRowAt(rightX, assetBtnY, assetActionLabels)

	btns := buttonRowAt(rightX, card.Y+card.Height-modalBtnH-modalBottomInset, foeViewBtnLabels)
	saveBtn, resetBtn, closeBtn := btns[0], btns[1], btns[2]

	return foeViewLayout{
		card: card, preview: preview,
		prevFoeBtn: prevBtn, nextFoeBtn: nextBtn,
		tabBtns:      tabBtns,
		sliderTracks: tracks,
		assetTracks:  assetTracks,
		assetBtns:    assetBtns,
		saveBtn:      saveBtn, resetBtn: resetBtn, closeBtn: closeBtn,
	}
}

// openFoeViewModal opens the visualizer. First open seeds the working copy from
// the first kind's live visual; later opens keep it so unsaved tuning survives.
func openFoeViewModal(s *State) {
	s.modal = modalFoeView
	if !s.foeInit {
		if defs := core.EnemyKinds(); len(defs) > 0 {
			s.foeKind = defs[0].Kind
		}
		seedFoeVisual(s)
		s.foeInit = true
	}
	resetVisualizerView(s, &foeDrag)
}

// enterAssetEditing resets the Asset-tab cursor and flags the preview stale so
// the seeded FX show immediately. Used on open and foe/class cycle. Adjustment
// values live in the override (seeded from disk) and are deliberately NOT cleared
// here — that's what makes editing iterable.
func enterAssetEditing(s *State) {
	s.assetCursor = 0
	s.assetPreviewStale = true
}

// resetVisualizerView restores the shared view state (tab, zoom, asset cursor) and
// clears the given modal's in-flight drag. Run on every Foe/Party visualizer open
// so the two entry points can't drift.
func resetVisualizerView(s *State, drag *visualizerDrag) {
	s.foeViewTab = foeTabLayout
	s.foeViewZoom = 1
	enterAssetEditing(s)
	drag.slider = noSliderDrag
	drag.asset = noSliderDrag
}

// closeVisualizer tears down the live 3D + asset preview and closes the modal,
// shared by the Esc and close-button paths (closeModal repeats the teardown
// idempotently as a safety net).
func closeVisualizer(s *State, cb visualizerCallbacks) {
	cb.closePreview()
	render.ClearAssetPreview()
	closeModal(s)
}

// seedFoeVisual loads the current foe's live visual into the working copy and
// snapshots it as the Reset baseline.
func seedFoeVisual(s *State) {
	if ov, ok := render.LiveFoeOverride(frameAssets, s.foeKind); ok {
		s.foeVisual = ov
		s.foeBaseline = ov
	} else {
		// No resolvable visual: seed a defined override so we don't keep the
		// PREVIOUS foe's values (a Save would write them under this slug). Unit
		// size, not zero — SizeX/Y are absolute (0.1 floor), so zero saves an
		// invisible foe. Defensive: enemyVisualFor falls back to Rat, so unreachable.
		base := core.EnemyVisualOverride{SizeX: 1, SizeY: 1}
		s.foeVisual = base
		s.foeBaseline = base
	}
	s.foeCursor = 0
}

// cycleFoe steps to the prev/next enemy kind (wrapping) and re-seeds.
func cycleFoe(s *State, dir int) {
	s.foeKind = cycleEnemyKind(s.foeKind, dir)
	seedFoeVisual(s)
	enterAssetEditing(s)
}

// cycleByIndex steps cur's index in items by delta, wrapping. Empty list / absent
// cur returns the first item / cur. Shared by the Foe and Party picker arrows.
func cycleByIndex[T comparable](items []T, cur T, delta int) T {
	if len(items) == 0 {
		return cur
	}
	idx := 0
	for i, it := range items {
		if it == cur {
			idx = i
			break
		}
	}
	return items[core.WrapIndex(idx+delta, len(items))]
}

// cycleRegistry steps cur to the prev/next registry entry (wrapping), pulling each
// entry's key via key. Collapses the "copy keys out of defs, then cycleByIndex"
// pattern shared by the Foe and Party pickers.
func cycleRegistry[D any, K comparable](defs []D, key func(D) K, cur K, delta int) K {
	keys := make([]K, len(defs))
	for i, def := range defs {
		keys[i] = key(def)
	}
	return cycleByIndex(keys, cur, delta)
}

// cycleEnemyKind walks the enemy registry by delta (+1/-1), wrapping.
func cycleEnemyKind(cur core.EnemyKind, delta int) core.EnemyKind {
	return cycleRegistry(core.EnemyKinds(), func(d core.EnemyDefinition) core.EnemyKind { return d.Kind }, cur, delta)
}

func saveFoeVisual(s *State) {
	slug := core.EnemySlug(s.foeKind)
	if err := core.SaveEnemyVisualOverride(slug, s.foeVisual); err != nil {
		s.flashWarn("Save failed: " + err.Error())
		return
	}
	// Mirror into the live visual so cycling away and back re-seeds from the saved
	// values and the editor preview updates immediately.
	render.SetLiveFoeOverride(frameAssets, s.foeKind, s.foeVisual)
	// Working copy is now on disk — make it the Reset baseline.
	s.foeBaseline = s.foeVisual
	s.flash(savedVisualFlash(core.EnemyInfo(s.foeKind).Name, slug))
}

// importDroppedPNG imports the first .png dropped on the window (raylib has no
// file dialog): importFn(path) writes it, reloadFn() rebuilds the live sprite.
// Non-PNG drops ignored. Shared by both visualizers.
func importDroppedPNG(s *State, slug string, importFn func(path string) error, reloadFn func()) {
	if !rl.IsFileDropped() {
		return
	}
	dropped := rl.LoadDroppedFiles()
	for _, path := range dropped {
		if !strings.EqualFold(filepath.Ext(path), ".png") {
			continue
		}
		if err := importFn(path); err != nil {
			s.flashWarn("Import failed: " + err.Error())
		} else {
			reloadFn()
			s.flash("Imported " + filepath.Base(path) + " → " + slug + ".png (updated live)")
		}
		break
	}
	// Free the C-side FilePathList raylib allocated for the drop.
	rl.UnloadDroppedFiles()
}

// navAdjustVisualTabs runs keyboard/d-pad row-nav + fine adjust for the active
// tab: Layout walks foeFields via *layoutCursor, Asset walks assetFields via
// s.assetCursor (flagging the preview stale). Both adjust override ov. Shared.
func navAdjustVisualTabs(s *State, layoutCursor *int, ov *core.EnemyVisualOverride) {
	if s.foeViewTab == foeTabLayout {
		*layoutCursor = input.CursorUpDown(*layoutCursor, len(foeFields))
		if *layoutCursor >= 0 && *layoutCursor < len(foeFields) {
			if delta := input.CursorLeftRight(); delta != 0 {
				f := foeFields[*layoutCursor]
				v := f.Get(ov) + float64(delta)*f.Step
				f.Set(ov, core.Clamp(v, f.Min, f.Max))
			}
		}
	} else {
		s.assetCursor = input.CursorUpDown(s.assetCursor, len(assetFields))
		if s.assetCursor >= 0 && s.assetCursor < len(assetFields) {
			if delta := input.CursorLeftRight(); delta != 0 {
				f := assetFields[s.assetCursor]
				v := f.Get(ov) + float64(delta)*f.Step
				f.Set(ov, core.Clamp(v, f.Min, f.Max))
				s.assetPreviewStale = true
			}
		}
	}
}

// foeViewCallbacks builds the Foe Visualizer's per-modal actions for the shared
// driver. Name span opens a dropdown of all kinds (too many to cycle).
func foeViewCallbacks(s *State) visualizerCallbacks {
	return visualizerCallbacks{
		drag:     &foeDrag,
		override: &s.foeVisual,
		cursor:   &s.foeCursor,
		importPNG: func() {
			importDroppedPNG(s, core.EnemySlug(s.foeKind),
				func(path string) error { return render.ImportSpriteFromFile(s.foeKind, path) },
				func() { render.ReloadFoeSprite(frameAssets, s.foeKind) })
		},
		cyclePrev: func() { cycleFoe(s, -1) },
		cycleNext: func() { cycleFoe(s, +1) },
		nameSpan:  func(span rl.Rectangle) { openDropdownBelow(s, ddFoeKind, span) },
		save:      func() { saveFoeVisual(s) },
		reset: func() {
			s.foeVisual = s.foeBaseline
			s.flash("Reset to last-saved values")
		},
		closePreview:   render.CloseFoePreview,
		refreshPreview: func() { render.RefreshFoeAssetPreview(frameAssets, s.foeKind, s.foeVisual) },
		title:          "FOE VISUALIZER",
		name:           core.EnemyInfo(s.foeKind).Name + dropdownArrowSuffix, // ▼ = click name to pick
		drawPreview: func(rect rl.Rectangle, gizmos bool) {
			render.DrawFoePreview(rect, frameAssets, s.foeKind, s.foeVisual, s.foeViewZoom, gizmos, assetPreviewTexFor())
		},
		footerHint: "D-pad row/adjust   |   drag sliders   |   buttons: change foe / save / reset / close",
		footerNote: visualizerFooterHint(false, core.EnemySlug(s.foeKind)),
	}
}

func updateFoeViewModal(s *State) Action {
	return updateVisualizerModal(s, foeViewCallbacks(s))
}

// selectFoeViewTab switches the active tab (shared by both modals), dropping any
// drag. The FX preview is untouched — it shows on BOTH tabs (only the gizmos are
// Layout-only, gated at draw time).
func selectFoeViewTab(s *State, tab int, drag *visualizerDrag) {
	if s.foeViewTab == tab {
		return
	}
	s.foeViewTab = tab
	// Reset the CALLER's drag (foeDrag/partyDrag) so both modals stay symmetric.
	drag.slider = noSliderDrag
	drag.asset = noSliderDrag
}

// setVisualAssetFromTrack maps mouse X within an Asset-tab track to ov's
// adjustment field (snapped) and flags the preview for rebuild. Shared.
func setVisualAssetFromTrack(s *State, ov *core.EnemyVisualOverride, i int, track rl.Rectangle, mouseX float32) {
	f := assetFields[i]
	f.Set(ov, sliderSnap(f.Min, f.Max, f.Step, track.X, track.Width, mouseX))
	s.assetPreviewStale = true
}

// setVisualFieldFromTrack maps mouse X within a track to ov's field range, snapped. Shared.
func setVisualFieldFromTrack(ov *core.EnemyVisualOverride, i int, track rl.Rectangle, mouseX float32) {
	f := foeFields[i]
	f.Set(ov, sliderSnap(f.Min, f.Max, f.Step, track.X, track.Width, mouseX))
}

// visualizerCallbacks captures everything that differs between the Foe and Party
// Visualizers so updateVisualizerModal / handleVisualizerClick can drive both. The
// override pointer (s.foeVisual / s.partyVisual) and cursor (s.foeCursor /
// s.partyCursor) are shared; the rest are per-modal actions.
type visualizerCallbacks struct {
	drag           *visualizerDrag
	override       *core.EnemyVisualOverride
	cursor         *int
	importPNG      func()             // import a dropped PNG as this entity's sprite
	cyclePrev      func()             // prev/next picker arrow
	cycleNext      func()             //
	nameSpan       func(rl.Rectangle) // click the name span (cycle vs open dropdown)
	save           func()             // Save button / Confirm key
	reset          func()             // Reset button
	closePreview   func()             // render close call on Esc / Close button
	refreshPreview func()             // rebuild the asset preview when stale
	// Draw-only fields (drawVisualizerModal): the rest of the per-modal variance.
	title       string                               // modal heading
	name        string                               // picker header label (incl. any ▼ suffix)
	drawPreview func(rect rl.Rectangle, gizmos bool) // blit the live 3D preview
	footerHint  string                               // line-1 controls hint
	footerNote  string                               // line-2 persistence note
}

// updateVisualizerModal drives one frame of either Visualizer from its callbacks.
// Behavior-identical to the old per-modal updaters; only the cb fields differ.
func updateVisualizerModal(s *State, cb visualizerCallbacks) Action {
	if editorCancelPressed() {
		closeVisualizer(s, cb)
		return ActionNone
	}

	cb.importPNG()

	l := computeFoeViewLayout()
	// Read the cursor live — cached frameMouse is set in Draw and is one frame
	// stale here (Update runs before Draw).
	mp := rl.GetMousePosition()
	mouseDown := rl.IsMouseButtonDown(rl.MouseLeftButton)
	mousePressed := rl.IsMouseButtonPressed(rl.MouseLeftButton)
	mouseReleased := rl.IsMouseButtonReleased(rl.MouseLeftButton)

	// Layout-tab field drag (held only while down AND on the Layout tab).
	cb.drag.slider.update(mouseDown && s.foeViewTab == foeTabLayout, len(foeFields), func(idx int) {
		setVisualFieldFromTrack(cb.override, idx, l.sliderTracks[idx], mp.X)
		*cb.cursor = idx
	})
	// Asset-tab drag: same protocol, feeds the live preview.
	cb.drag.asset.update(mouseDown && s.foeViewTab == foeTabAsset, len(assetFields), func(idx int) {
		setVisualAssetFromTrack(s, cb.override, idx, l.assetTracks[idx], mp.X)
		s.assetCursor = idx
	})

	applyPreviewZoomWheel(s, l.preview, mp)

	if mousePressed {
		handleVisualizerClick(s, &l, mp, cb)
	}
	if mouseReleased {
		cb.drag.slider = noSliderDrag
		cb.drag.asset = noSliderDrag
	}

	// Save on Confirm (Enter/Space). Do NOT bind 'S' — CursorUpDown uses it for
	// "move down" (W/S nav), so an S-save would steal row navigation.
	if editorCommitPressed() {
		cb.save()
		return ActionNone
	}

	navAdjustVisualTabs(s, cb.cursor, cb.override)
	// Rebuild the live preview (shown on BOTH tabs) whenever the FX changed.
	if s.assetPreviewStale {
		cb.refreshPreview()
		s.assetPreviewStale = false
	}
	return ActionNone
}

// handleVisualizerClick dispatches a left-press for either Visualizer: tab row,
// active-tab sliders/buttons, the prev/next arrows + name span, then
// Save/Reset/Close. Behavior-identical to the old per-modal click handlers.
func handleVisualizerClick(s *State, l *foeViewLayout, mp rl.Vector2, cb visualizerCallbacks) {
	if i := firstRectHit(mp, l.tabBtns); i >= 0 {
		selectFoeViewTab(s, i, cb.drag)
		return
	}
	if s.foeViewTab == foeTabLayout {
		for i := range foeFields {
			if pointIn(mp, padRect(l.sliderTracks[i], 0, sliderHitPadY)) {
				cb.drag.slider.idx = i
				setVisualFieldFromTrack(cb.override, i, l.sliderTracks[i], mp.X)
				*cb.cursor = i
				return
			}
		}
	}
	if s.foeViewTab == foeTabAsset {
		for i := range assetFields {
			if pointIn(mp, padRect(l.assetTracks[i], 0, sliderHitPadY)) {
				cb.drag.asset.idx = i
				setVisualAssetFromTrack(s, cb.override, i, l.assetTracks[i], mp.X)
				s.assetCursor = i
				return
			}
		}
		if i := firstRectHit(mp, l.assetBtns); i >= 0 {
			applyAssetAction(s, cb.override, i)
			return
		}
	}
	if pointIn(mp, l.prevFoeBtn) {
		cb.cyclePrev()
		return
	}
	if pointIn(mp, l.nextFoeBtn) {
		cb.cycleNext()
		return
	}
	if nameSpan := nameSpanBetween(l.prevFoeBtn, l.nextFoeBtn); pointIn(mp, nameSpan) {
		cb.nameSpan(nameSpan)
		return
	}
	if pointIn(mp, l.saveBtn) {
		cb.save()
		return
	}
	if pointIn(mp, l.resetBtn) {
		cb.reset()
		return
	}
	if pointIn(mp, l.closeBtn) {
		closeVisualizer(s, cb)
		return
	}
}

// applyPreviewZoomWheel dollies the preview on a wheel turn over the pane (shared;
// clamped render-side; scroll-up zooms in).
func applyPreviewZoomWheel(s *State, preview rl.Rectangle, mp rl.Vector2) {
	if !pointIn(mp, preview) {
		return
	}
	w := rl.GetMouseWheelMove()
	if w == 0 {
		return
	}
	z := s.foeViewZoom + w*foePreviewZoomStep
	if z < render.FoePreviewZoomMin {
		z = render.FoePreviewZoomMin
	}
	if z > render.FoePreviewZoomMax {
		z = render.FoePreviewZoomMax
	}
	s.foeViewZoom = z
}

func drawFoeViewModal(s *State, font rl.Font, theme render.Theme) {
	drawVisualizerModal(s, font, theme, foeViewCallbacks(s))
}

// drawVisualizerModal paints either Visualizer; all per-modal variance rides on cb
// (title / picker name / preview blit / footer text + the slider override+cursor).
func drawVisualizerModal(s *State, font rl.Font, theme render.Theme, cb visualizerCallbacks) {
	l := computeFoeViewLayout()
	drawModalHeaderAt(font, theme, l.card, cb.title, theme.BorderActive)

	// Live 3D preview (blitted from an off-screen texture). Gizmos show only on
	// the Layout tab; on the Asset tab the bake preview texture overrides.
	cb.drawPreview(l.preview, s.foeViewTab == foeTabLayout)
	rl.DrawRectangleLinesEx(l.preview, 1, theme.BorderDim)

	// Picker header: < Name >.
	drawButton(font, l.prevFoeBtn, "<", false)
	drawButton(font, l.nextFoeBtn, ">", false)
	nameSize := render.MeasureRichText(font, cb.name, editorFontTopbar, 1)
	span := nameSpanBetween(l.prevFoeBtn, l.nextFoeBtn)
	render.DrawRichText(font, cb.name,
		rl.NewVector2(span.X+(span.Width-nameSize.X)/2, l.prevFoeBtn.Y+5),
		editorFontTopbar, 1, theme.TextPrimary)

	drawFoeViewTabs(font, l, s.foeViewTab)
	if s.foeViewTab == foeTabLayout {
		for i := range foeFields {
			drawVisualSlider(font, theme, l, i, cb.override, *cb.cursor)
		}
	} else {
		drawAssetTab(font, theme, l, cb.override, s.assetCursor)
	}

	drawModalButtons(font, []rl.Rectangle{l.saveBtn, l.resetBtn, l.closeBtn}, foeViewBtnLabels)

	// Footer hint + persistence note, under the preview pane.
	render.DrawTextWithShadow(font, cb.footerHint,
		l.card.X+foePad, l.preview.Y+l.preview.Height+foeFooterHintDY, editorFontHint, theme.TextHint)
	render.DrawTextWithShadow(font, cb.footerNote,
		l.card.X+foePad, l.preview.Y+l.preview.Height+foeFooterNoteDY, editorFontHint, theme.TextMuted)
}

// drawFoeViewTabs paints the tab buttons, active one highlighted. Shared.
func drawFoeViewTabs(font rl.Font, l foeViewLayout, active int) {
	for i := range l.tabBtns {
		drawButton(font, l.tabBtns[i], foeViewTabLabels[i], i == active)
	}
}

// assetPreviewTexFor returns the live-preview texture (shows on BOTH tabs; zero
// texture = no adjustments, pane falls back to the real sprite).
func assetPreviewTexFor() rl.Texture2D {
	return render.AssetPreviewTexture()
}

// drawAssetTab paints the Asset tab: FX sliders editing override `ov` + a Revert
// button + a hint. Shared (override + cursor passed in).
func drawAssetTab(font rl.Font, theme render.Theme, l foeViewLayout, ov *core.EnemyVisualOverride, cursor int) {
	for i := range assetFields {
		drawFoeSliderRow(font, theme, assetFields[i], ov, l.assetTracks[i], cursor == i)
	}
	for i := range l.assetBtns {
		drawButton(font, l.assetBtns[i], assetActionLabels[i], false)
	}
	if len(l.assetBtns) > 0 {
		last := l.assetBtns[len(l.assetBtns)-1]
		render.DrawTextWithShadow(font,
			"Non-destructive FX — live preview, saved to visuals.json, Revert to clear. Drop a .png to replace the art.",
			l.assetBtns[0].X, last.Y+last.Height+12, editorFontHint, theme.TextHint)
	}
}

// drawFoeSliderRow draws one visualizer slider row: label left of the track, value
// right, at the foe metrics + accent font. Shared by the Layout and Asset tabs.
func drawFoeSliderRow(font rl.Font, theme render.Theme, f sliderField[core.EnemyVisualOverride], ov *core.EnemyVisualOverride, track rl.Rectangle, focused bool) {
	drawSliderField(font, theme, f, ov,
		rl.NewVector2(track.X-foeSliderMetrics.labelW, track.Y-foeSliderLabelDY),
		rl.NewVector2(track.X+track.Width+foeSliderValueGap, track.Y-foeSliderLabelDY),
		editorFontAccent, track, foeSliderMetrics.thumbR, focused)
}

// drawVisualSlider draws foeFields row i against override ov and cursor (the
// Layout tab; both Visualizers route here via drawVisualizerModal).
func drawVisualSlider(font rl.Font, theme render.Theme, l foeViewLayout, i int, ov *core.EnemyVisualOverride, cursor int) {
	drawFoeSliderRow(font, theme, foeFields[i], ov, l.sliderTracks[i], cursor == i)
}

// sliderRowMetrics is the shared geometry contract for an editor slider row: the label
// gutter before the track, the value column reserved after it, and the track height.
// The Foe/Party visualizer (foeSliderMetrics) and the sound creator (soundSliderMetrics)
// each instantiate it with their own values — previously two parallel const families.
type sliderRowMetrics struct {
	labelW, valueW, trackH, thumbR float32
}

// trackReserve is the width removed from a row for the label + value columns (+gap), so
// trackWidth = rowWidth - trackReserve(gap). Foe uses gap 0, the sound creator gap 2.
func (m sliderRowMetrics) trackReserve(gap float32) float32 { return m.labelW + m.valueW + gap }

var foeSliderMetrics = sliderRowMetrics{labelW: 86, valueW: 56, trackH: 12, thumbR: 6}

// foe slider label/value placement relative to the track: baseline lifted above the
// track top, value column gapped past the track's right edge.
const (
	foeSliderLabelDY  = float32(4)
	foeSliderValueGap = float32(8)
)

// drawSliderField renders one sliderField row against ov's value. The single
// slider-row draw shared by both Visualizers and the sound creator. Display-aware:
// a custom Display renderer wins, else fmt.Sprintf with the row's Format.
func drawSliderField[T any](font rl.Font, theme render.Theme, f sliderField[T], ov *T,
	labelPos, valuePos rl.Vector2, fontSize float32, track rl.Rectangle, thumbRadius float32, focused bool) {
	value := f.Get(ov)
	val := fmt.Sprintf(f.Format, value)
	if f.Display != nil {
		val = f.Display(value)
	}
	drawSlider(font, theme, f.Label, val, value, f.Min, f.Max,
		labelPos, valuePos, fontSize, track, thumbRadius, focused)
}

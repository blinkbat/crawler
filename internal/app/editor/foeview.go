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
	{Label: "Tint R", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.TintR) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.TintR = core.ClampByte(int(v + 0.5)) }, Min: 0, Max: 255, Step: 1, Format: "%.0f"},
	{Label: "Tint G", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.TintG) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.TintG = core.ClampByte(int(v + 0.5)) }, Min: 0, Max: 255, Step: 1, Format: "%.0f"},
	{Label: "Tint B", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.TintB) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.TintB = core.ClampByte(int(v + 0.5)) }, Min: 0, Max: 255, Step: 1, Format: "%.0f"},
	{Label: "Tint A", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.TintA) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.TintA = core.ClampByte(int(v + 0.5)) }, Min: 0, Max: 255, Step: 1, Format: "%.0f"},
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

// foeDrag holds the visualizer's in-flight drag. slider = Layout-tab field drag,
// asset = Asset-tab param drag (one active at a time, gated by tab).
var foeDrag = struct{ slider, asset sliderDragState }{slider: noSliderDrag, asset: noSliderDrag}

// Modal geometry. Wide card: preview pane left, slider stack right.
const (
	foeModalW     = float32(1040)
	foeModalH     = float32(600)
	foeHeaderH    = float32(40)
	foePad        = float32(16)
	foePreviewW   = float32(420)
	foeSliderRowH = float32(30)
	foeColGap     = float32(26) // gutter between the two slider columns
	foePickBtnH   = float32(28) // prev/next picker arrow height (< Name >)
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

// assetActionLabels is the Asset tab's button row; slice index IS the action
// identity (assetActionRevert, …) the click handlers dispatch on. Add an action
// by appending a label AND a switch case (a caseless label is a visible gap, not
// a silent Revert). PNG import is the drag-drop path, not a button.
var assetActionLabels = []string{"Revert"}

const assetActionRevert = 0

// applyAssetAction runs Asset-tab button `i` against override `ov` (shared).
// default is an explicit no-op so a caseless label reads as a gap, not Revert.
func applyAssetAction(s *State, ov *core.EnemyVisualOverride, i int) {
	switch i {
	case assetActionRevert:
		clearVisualAdjustments(ov)
		s.assetPreviewStale = true
		s.flash("Reverted sprite FX (" + assetFieldNames() + ")")
	default:
	}
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

// computeFoeViewLayout is the single geometry source for the modal's draw and
// hit-test (so widgets and click rects can't drift).
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
	prevBtn := rl.NewRectangle(rightX, nameY, pickBtnW, foePickBtnH)
	nextBtn := rl.NewRectangle(rightX+rightW-pickBtnW, nameY, pickBtnW, foePickBtnH)

	// Tab row (Layout / Asset). Both tabs lay out at the same contentTop; only the
	// active one is drawn/hit-tested (gated on State.foeViewTab).
	tabY := nameY + foePickBtnH + 10
	tabBtns := buttonRowAt(rightX, tabY, foeViewTabLabels)
	contentTop := tabY + float32(modalBtnH) + 14

	// Two-column slider stack (Layout tab): rightW split into two equal columns.
	// Left column gets the ceil half (odd count → extra row left).
	colW := (rightW - foeColGap) / 2
	colTrackW := colW - foeSliderMetrics.trackReserve(0)
	firstColRows := (len(foeFields) + 1) / 2
	tracks := make([]rl.Rectangle, len(foeFields))
	rowBase := contentTop
	for i := range foeFields {
		col, row := 0, i
		if i >= firstColRows {
			col, row = 1, i-firstColRows
		}
		colX := rightX + float32(col)*(colW+foeColGap)
		y := rowBase + float32(row)*foeSliderRowH + (foeSliderRowH-foeSliderMetrics.trackH)/2
		tracks[i] = rl.NewRectangle(colX+foeSliderMetrics.labelW, y, colTrackW, foeSliderMetrics.trackH)
	}

	// Asset tab: same two-column split. Left column gets the ceil half; action
	// buttons sit below the taller column.
	assetFirstColRows := (len(assetFields) + 1) / 2
	assetTracks := make([]rl.Rectangle, len(assetFields))
	for i := range assetFields {
		col, row := 0, i
		if i >= assetFirstColRows {
			col, row = 1, i-assetFirstColRows
		}
		colX := rightX + float32(col)*(colW+foeColGap)
		y := contentTop + float32(row)*foeSliderRowH + (foeSliderRowH-foeSliderMetrics.trackH)/2
		assetTracks[i] = rl.NewRectangle(colX+foeSliderMetrics.labelW, y, colTrackW, foeSliderMetrics.trackH)
	}
	assetBtnY := contentTop + float32(assetFirstColRows)*foeSliderRowH + 16
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
	s.foeViewTab = foeTabLayout
	s.foeViewZoom = 1
	enterAssetEditing(s)
	foeDrag.slider = noSliderDrag
	foeDrag.asset = noSliderDrag
}

// enterAssetEditing resets the Asset-tab cursor and flags the preview stale so
// the seeded FX show immediately. Used on open and foe/class cycle. Adjustment
// values live in the override (seeded from disk) and are deliberately NOT cleared
// here — that's what makes editing iterable.
func enterAssetEditing(s *State) {
	s.assetCursor = 0
	s.assetPreviewStale = true
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
	}
}

func updateFoeViewModal(s *State) Action {
	return updateVisualizerModal(s, foeViewCallbacks(s))
}

// selectFoeViewTab switches the active tab (shared by both modals), dropping any
// drag. The FX preview is untouched — it shows on BOTH tabs (only the gizmos are
// Layout-only, gated at draw time).
func selectFoeViewTab(s *State, tab int, drag *struct{ slider, asset sliderDragState }) {
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
	drag           *struct{ slider, asset sliderDragState }
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
}

// updateVisualizerModal drives one frame of either Visualizer from its callbacks.
// Behavior-identical to the old per-modal updaters; only the cb fields differ.
func updateVisualizerModal(s *State, cb visualizerCallbacks) Action {
	if editorCancelPressed() {
		cb.closePreview()
		render.ClearAssetPreview()
		closeModal(s)
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
	for i := range l.tabBtns {
		if pointIn(mp, l.tabBtns[i]) {
			selectFoeViewTab(s, i, cb.drag)
			return
		}
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
		for i := range l.assetBtns {
			if !pointIn(mp, l.assetBtns[i]) {
				continue
			}
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
		cb.closePreview()
		render.ClearAssetPreview()
		closeModal(s)
		return
	}
}

// padRect grows r by (dx, dy) on every side (fatter click band for thin tracks).
func padRect(r rl.Rectangle, dx, dy float32) rl.Rectangle {
	return rl.NewRectangle(r.X-dx, r.Y-dy, r.Width+2*dx, r.Height+2*dy)
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
	l := computeFoeViewLayout()
	drawModalHeaderAt(font, theme, l.card, "FOE VISUALIZER", theme.BorderActive)

	// Live 3D preview (blitted from an off-screen texture). Gizmos show only on
	// the Layout tab; on the Asset tab the bake preview texture overrides.
	render.DrawFoePreview(l.preview, frameAssets, s.foeKind, s.foeVisual, s.foeViewZoom, s.foeViewTab == foeTabLayout, assetPreviewTexFor())
	rl.DrawRectangleLinesEx(l.preview, 1, theme.BorderDim)

	// Foe picker header: < Name >.
	drawButton(font, l.prevFoeBtn, "<", false)
	drawButton(font, l.nextFoeBtn, ">", false)
	name := core.EnemyInfo(s.foeKind).Name + dropdownArrowSuffix // ▼ = click name to pick
	nameSize := render.MeasureRichText(font, name, editorFontTopbar, 1)
	span := nameSpanBetween(l.prevFoeBtn, l.nextFoeBtn)
	render.DrawRichText(font, name,
		rl.NewVector2(span.X+(span.Width-nameSize.X)/2, l.prevFoeBtn.Y+5),
		editorFontTopbar, 1, theme.TextPrimary)

	drawFoeViewTabs(font, l, s.foeViewTab)
	if s.foeViewTab == foeTabLayout {
		for i := range foeFields {
			drawFoeSlider(font, theme, l, i, s)
		}
	} else {
		drawAssetTab(font, theme, l, &s.foeVisual, s.assetCursor)
	}

	drawModalButtons(font, []rl.Rectangle{l.saveBtn, l.resetBtn, l.closeBtn}, foeViewBtnLabels)

	// Footer hint + persistence note, under the preview pane.
	render.DrawTextWithShadow(font,
		"D-pad row/adjust   |   drag sliders   |   buttons: change foe / save / reset / close",
		l.card.X+foePad, l.preview.Y+l.preview.Height+8, editorFontHint, theme.TextHint)
	render.DrawTextWithShadow(font,
		visualizerFooterHint(false, core.EnemySlug(s.foeKind)),
		l.card.X+foePad, l.preview.Y+l.preview.Height+26, editorFontHint, theme.TextMuted)

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
		f := assetFields[i]
		track := l.assetTracks[i]
		drawSliderField(font, theme, f, ov,
			rl.NewVector2(track.X-foeSliderMetrics.labelW, track.Y-4), rl.NewVector2(track.X+track.Width+8, track.Y-4),
			editorFontAccent, track, 6, cursor == i)
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

// drawVisualSlider draws foeFields row i against override ov and cursor.
// drawFoeSlider/drawPartySlider delegate here (differ only in the override/cursor).
func drawVisualSlider(font rl.Font, theme render.Theme, l foeViewLayout, i int, ov *core.EnemyVisualOverride, cursor int) {
	f := foeFields[i]
	track := l.sliderTracks[i]
	drawSliderField(font, theme, f, ov,
		rl.NewVector2(track.X-foeSliderMetrics.labelW, track.Y-4), rl.NewVector2(track.X+track.Width+8, track.Y-4),
		editorFontAccent, track, 6, cursor == i)
}

func drawFoeSlider(font rl.Font, theme render.Theme, l foeViewLayout, i int, s *State) {
	drawVisualSlider(font, theme, l, i, &s.foeVisual, s.foeCursor)
}

// sliderRowMetrics is the shared geometry contract for an editor slider row: the label
// gutter before the track, the value column reserved after it, and the track height.
// The Foe/Party visualizer (foeSliderMetrics) and the sound creator (soundSliderMetrics)
// each instantiate it with their own values — previously two parallel const families.
type sliderRowMetrics struct {
	labelW, valueW, trackH float32
}

// trackReserve is the width removed from a row for the label + value columns (+gap), so
// trackWidth = rowWidth - trackReserve(gap). Foe uses gap 0, the sound creator gap 2.
func (m sliderRowMetrics) trackReserve(gap float32) float32 { return m.labelW + m.valueW + gap }

var foeSliderMetrics = sliderRowMetrics{labelW: 86, valueW: 56, trackH: 12}

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

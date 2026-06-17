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
	{Label: "Num X", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.PopupXOffset) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.PopupXOffset = float32(v) }, Min: -1.5, Max: 1.5, Step: 0.02, Format: "%.2f"},
	{Label: "Num Y", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.PopupYOffset) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.PopupYOffset = float32(v) }, Min: -1.5, Max: 2.0, Step: 0.02, Format: "%.2f"},
	{Label: "Tint R", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.TintR) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.TintR = core.ClampByte(int(v + 0.5)) }, Min: 0, Max: 255, Step: 1, Format: "%.0f"},
	{Label: "Tint G", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.TintG) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.TintG = core.ClampByte(int(v + 0.5)) }, Min: 0, Max: 255, Step: 1, Format: "%.0f"},
	{Label: "Tint B", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.TintB) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.TintB = core.ClampByte(int(v + 0.5)) }, Min: 0, Max: 255, Step: 1, Format: "%.0f"},
	{Label: "Tint A", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.TintA) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.TintA = core.ClampByte(int(v + 0.5)) }, Min: 0, Max: 255, Step: 1, Format: "%.0f"},
}

// sliderDragState is the shared in-flight slider-drag cursor for the editor's
// slider-stack modals (the Foe/Party Visualizer tabs and the sound creator).
// idx is the field index under the held mouse, or -1 for "no active drag".
// Package-level instances are fine — only one modal drags at a time, and the
// raylib draw loop is single-threaded.
type sliderDragState struct {
	idx int
}

// noSliderDrag is the released/idle value (idx == -1). Use it to seed and to
// reset a drag on modal open and on mouse-up.
var noSliderDrag = sliderDragState{idx: -1}

// update advances one frame of a slider drag. With no active drag it does
// nothing. Otherwise: if held is false (mouse released, or the owning tab/panel
// is no longer showing) it ends the drag; else, when the index is in range, it
// invokes apply(idx) — the caller's per-modal "snap this field to the mouse and
// run any cursor/preview side effects" closure. Centralizes the drag protocol so
// the foe-visualizer and sound modals can't drift on the end-vs-apply logic.
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

// foeDrag holds the in-flight slider drag for the visualizer modal. slider tracks
// a Layout-tab field drag, asset an Asset-tab bake-param drag (only one is ever
// active at a time, gated by the tab). Reset on open / mouse-up.
var foeDrag = struct{ slider, asset sliderDragState }{slider: noSliderDrag, asset: noSliderDrag}

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
	// foePreviewZoomStep is the per-wheel-notch dolly applied to the preview
	// pane's zoom (clamped to the render-side bounds).
	foePreviewZoomStep = float32(0.2)
	// sliderHitPadY fattens a slider track's click band vertically (the thin
	// tracks are easy to miss) without changing how they draw. Shared by both
	// Visualizers' Layout + Asset track hit-tests.
	sliderHitPadY = float32(9)
)

// foeViewBtnLabels is the action row's single label source — the layout
// (buttonRowAt sizes each button to its label) and the draw read the same
// slice so the painted text and the hit rects can't drift.
var foeViewBtnLabels = []string{"Save", "Reset", "Close"}

// foeViewTabLabels names the two visualizer panes (index == State.foeViewTab):
// Layout = the positional/tint slider stack, Asset = the sprite-PNG bake strip.
// One source for the tab buttons' draw and hit-test so they can't drift.
var foeViewTabLabels = []string{"Layout", "Asset"}

// Visualizer tab indices.
const (
	foeTabLayout = 0
	foeTabAsset  = 1
)

// assetFields is the Asset tab's NON-DESTRUCTIVE image-adjustment slider stack.
// Unlike the old destructive bakes, these edit the visual OVERRIDE in place
// (s.foeVisual / s.partyVisual): they persist to visuals.json via the modal's
// Save, reload for further editing on reopen, revert by zeroing, and render
// point-sampled (sharp) — applied to the PRISTINE sprite at texture-build time.
// Pixelate is 0..1 intensity (mosaic), Brightness/Contrast -1..1. Shared by both
// modals (the Get/Set bridge an *EnemyVisualOverride; PartyVisualOverride aliases it).
var assetFields = []sliderField[core.EnemyVisualOverride]{
	{Label: "Pixelate", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.Pixelate) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.Pixelate = float32(v) }, Min: 0, Max: 1, Step: 0.05, Format: "%.2f"},
	{Label: "Bright", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.Brightness) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.Brightness = float32(v) }, Min: -1, Max: 1, Step: 0.05, Format: "%.2f"},
	{Label: "Contrast", Get: func(o *core.EnemyVisualOverride) float64 { return float64(o.Contrast) }, Set: func(o *core.EnemyVisualOverride, v float64) { o.Contrast = float32(v) }, Min: -1, Max: 1, Step: 0.05, Format: "%.2f"},
}

// assetActionLabels is the Asset tab's button row; the slice index IS the
// action identity (assetActionRevert, …) the click handlers dispatch on — not a
// bare [0]. Add an action by appending a label here AND a switch case in the
// Foe/Party asset-click handlers, so a new label with no case reads as a
// visible gap rather than a silent button that still fires Revert. PNG import
// is the drag-drop path, not a button.
var assetActionLabels = []string{"Revert"}

const assetActionRevert = 0

// savedVisualFlash is the shared save-confirmation toast for the Foe/Party
// Visualizers: both persist a visual override the editor applies live but the
// game only picks up on restart, so they show the identical message.
func savedVisualFlash(name, slug string) string {
	return "Saved " + name + " → " + slug + " (live in editor; restart game to apply)"
}

// visualizerFooterHint is the shared orange-sphere/cyan-glyph persistence note
// painted under both Visualizers' preview panes. noun is "foe"/"class" — it
// selects the override file the save writes (foes → visuals.json, classes →
// partyvisuals.json) — and slug is that file's map key ("rat", "warrior").
func visualizerFooterHint(noun, slug string) string {
	file := "visuals.json"
	if noun == "class" {
		file = "partyvisuals.json"
	}
	return "orange sphere = particle origin   ·   cyan = hit glyph   ·   saves to " + file + " as \"" + slug + "\""
}

// clearVisualAdjustments zeroes the non-destructive image-adjustment fields of an
// override (the Asset tab's Revert). Tint and the placement fields are untouched.
func clearVisualAdjustments(ov *core.EnemyVisualOverride) {
	ov.Pixelate, ov.Brightness, ov.Contrast = 0, 0, 0
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

	// Tab row (Layout / Asset) below the foe picker. The active tab decides which
	// content fills the band beneath it — the slider stack or the sprite-PNG
	// strip — so both are laid out at the same contentTop and only one is drawn /
	// hit-tested per tab (gated on State.foeViewTab by the draw + click handlers).
	tabY := nameY + 28 + 10
	tabBtns := buttonRowAt(rightX, tabY, foeViewTabLabels)
	contentTop := tabY + float32(modalBtnH) + 14

	// Two-column slider stack (Layout tab): split rightW into two equal columns
	// separated by foeColGap, each laying out label | track | value exactly as the
	// old single column did (drawFoeSlider / handleFoeViewClick read tracks[i]
	// rectangles + position the label/value relative to each, so they stay
	// column-agnostic — only the rectangles move). The left column gets the ceil
	// half so an odd count puts the extra row on the left.
	colW := (rightW - foeColGap) / 2
	colTrackW := colW - foeLabelW - foeValueW
	firstColRows := (len(foeFields) + 1) / 2
	tracks := make([]rl.Rectangle, len(foeFields))
	rowBase := contentTop
	for i := range foeFields {
		col, row := 0, i
		if i >= firstColRows {
			col, row = 1, i-firstColRows
		}
		colX := rightX + float32(col)*(colW+foeColGap)
		y := rowBase + float32(row)*foeSliderRowH + (foeSliderRowH-foeTrackH)/2
		tracks[i] = rl.NewRectangle(colX+foeLabelW, y, colTrackW, foeTrackH)
	}

	// Asset tab: a single-column slider stack (the gradable bake params) at
	// contentTop, then the action buttons in two rows of three below it.
	assetTracks := make([]rl.Rectangle, len(assetFields))
	assetTrackW := rightW - foeLabelW - foeValueW
	for i := range assetFields {
		y := contentTop + float32(i)*foeSliderRowH + (foeSliderRowH-foeTrackH)/2
		assetTracks[i] = rl.NewRectangle(rightX+foeLabelW, y, assetTrackW, foeTrackH)
	}
	assetBtnY := contentTop + float32(len(assetFields))*foeSliderRowH + 16
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
	s.foeViewTab = foeTabLayout
	s.foeViewZoom = 1
	enterAssetEditing(s)
	foeDrag.slider = noSliderDrag
	foeDrag.asset = noSliderDrag
}

// enterAssetEditing resets the Asset-tab cursor and flags the live preview for a
// rebuild from the freshly-seeded override (so the saved Pixelate/Brightness/
// Contrast show immediately). Used on open and on a foe/class cycle. The
// adjustment VALUES live in the override (seeded from the save file), so they're
// intentionally NOT cleared here — that's what makes editing iterable.
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
// copy from that kind's live visual.
func cycleFoe(s *State, dir int) {
	s.foeKind = cycleEnemyKind(s.foeKind, dir)
	seedFoeVisual(s)
	enterAssetEditing(s) // rebuild the live preview from the new foe's saved FX
}

// cycleByIndex finds cur in items, steps the index by delta, wraps at the ends,
// and returns the item there. An empty list or an absent cur (idx stays 0)
// returns the first item / cur respectively — the shared "step a picker by ±1"
// body for the Foe (enemy kind) and Party (class) visualizer arrows.
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

// cycleEnemyKind walks the enemy registry by delta (+1 / -1), wrapping at the
// ends. Skips nothing — every registered kind is a valid choice. Used by the
// Foe Visualizer's < / > buttons.
func cycleEnemyKind(cur core.EnemyKind, delta int) core.EnemyKind {
	defs := core.EnemyKinds()
	kinds := make([]core.EnemyKind, len(defs))
	for i, def := range defs {
		kinds[i] = def.Kind
	}
	return cycleByIndex(kinds, cur, delta)
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
	s.flash(savedVisualFlash(core.EnemyInfo(s.foeKind).Name, slug))
}

// importDroppedPNG handles a PNG dropped onto the window while a Visualizer
// modal is open (raylib has no file dialog, so drag-drop is the import path).
// It takes the first .png in the drop, runs importFn(path) to write it under
// `slug`.png, then reloadFn() to rebuild the live sprite; non-PNG drops are
// ignored. Shared by the Foe and Party visualizers (they differ only in the
// import/reload targets). No-op when nothing was dropped.
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

// navAdjustVisualTabs runs the shared keyboard/d-pad row-nav + fine value
// adjust for a Visualizer's active tab. On the Layout tab it walks the foeFields
// stack via *layoutCursor; on the Asset tab it walks assetFields via s.assetCursor
// and flags the live preview stale on a change. Both adjust the passed override
// `ov` (s.foeVisual / s.partyVisual). layoutCursor is the caller's per-modal row
// cursor (&s.foeCursor / &s.partyCursor). Shared so the two modals can't drift.
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

func updateFoeViewModal(s *State) Action {
	if editorCancelPressed() {
		render.CloseFoePreview()
		render.ClearAssetPreview()
		closeModal(s)
		return ActionNone
	}

	// "Upload": a PNG dropped onto the window while the modal is open imports as
	// THIS foe's sprite (raylib has no file dialog; drag-drop is the path).
	importDroppedPNG(s, core.EnemySlug(s.foeKind),
		func(path string) error { return render.ImportSpriteFromFile(s.foeKind, path) },
		func() { render.ReloadFoeSprite(frameAssets, s.foeKind) })

	l := computeFoeViewLayout()
	// Read the cursor live (NOT the cached frameMouse, which is set in Draw and
	// is therefore one frame stale here since Update runs before Draw) so a
	// fast slider drag / click hit-tests against the current position. Matches
	// updateSoundsModal, which also reads rl.GetMousePosition() directly.
	mp := rl.GetMousePosition()
	mouseDown := rl.IsMouseButtonDown(rl.MouseLeftButton)
	mousePressed := rl.IsMouseButtonPressed(rl.MouseLeftButton)
	mouseReleased := rl.IsMouseButtonReleased(rl.MouseLeftButton)

	// Active Layout-tab field drag: held only while the mouse is down AND the
	// Layout tab still shows; the apply snaps the field to the mouse X and parks
	// the row cursor on it.
	foeDrag.slider.update(mouseDown && s.foeViewTab == foeTabLayout, len(foeFields), func(idx int) {
		setFoeFieldFromTrack(s, idx, l.sliderTracks[idx], mp.X)
		s.foeCursor = idx
	})
	// Active Asset-tab adjustment drag: same protocol, but feeds the live preview.
	foeDrag.asset.update(mouseDown && s.foeViewTab == foeTabAsset, len(assetFields), func(idx int) {
		setFoeAssetFromTrack(s, idx, l.assetTracks[idx], mp.X)
		s.assetCursor = idx
	})

	applyPreviewZoomWheel(s, l.preview, mp)

	if mousePressed {
		handleFoeViewClick(s, &l, mp)
	}
	if mouseReleased {
		foeDrag.slider = noSliderDrag
		foeDrag.asset = noSliderDrag
	}

	// Save on Confirm (Enter/Space). NOTE: do NOT bind 'S' here — input's
	// CursorUpDown treats 'S' as "move down" (W/S nav), so an S-save would
	// steal row navigation.
	if editorCommitPressed() {
		saveFoeVisual(s)
		return ActionNone
	}

	// Row navigation + fine value adjust, per tab.
	navAdjustVisualTabs(s, &s.foeCursor, &s.foeVisual)
	// Rebuild the live preview (shown on BOTH tabs — the FX are part of the look)
	// from this foe's PRISTINE sprite + the override's current adjustments whenever
	// they changed (slider drag / nav / Revert / foe cycle).
	if s.assetPreviewStale {
		render.RefreshFoeAssetPreview(frameAssets, s.foeKind, s.foeVisual)
		s.assetPreviewStale = false
	}
	return ActionNone
}

// handleFoeViewClick dispatches a left-press inside the modal: slider tracks
// (start a drag + set the value immediately), the foe prev/next arrows, and the
// Save/Reset/Close buttons.
func handleFoeViewClick(s *State, l *foeViewLayout, mp rl.Vector2) {
	for i := range l.tabBtns {
		if pointIn(mp, l.tabBtns[i]) {
			selectFoeViewTab(s, i, &foeDrag)
			return
		}
	}
	if s.foeViewTab == foeTabLayout {
		for i := range foeFields {
			if pointIn(mp, padRect(l.sliderTracks[i], 0, sliderHitPadY)) {
				foeDrag.slider.idx = i
				setFoeFieldFromTrack(s, i, l.sliderTracks[i], mp.X)
				s.foeCursor = i
				return
			}
		}
	}
	if s.foeViewTab == foeTabAsset {
		for i := range assetFields {
			if pointIn(mp, padRect(l.assetTracks[i], 0, sliderHitPadY)) {
				foeDrag.asset.idx = i
				setFoeAssetFromTrack(s, i, l.assetTracks[i], mp.X)
				s.assetCursor = i
				return
			}
		}
		for i := range l.assetBtns {
			if !pointIn(mp, l.assetBtns[i]) {
				continue
			}
			switch i {
			case assetActionRevert:
				clearVisualAdjustments(&s.foeVisual)
				s.assetPreviewStale = true
				s.flash("Reverted sprite FX (Pixelate / Bright / Contrast)")
			}
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
		render.ClearAssetPreview()
		closeModal(s)
		return
	}
}

// selectFoeViewTab switches the active visualizer tab (shared by both modals),
// dropping any in-flight drag. The live FX preview is NOT touched: the
// adjustments are part of the sprite's look, so the preview shows on BOTH tabs
// (only the authoring gizmos are Layout-only, gated separately at draw time).
func selectFoeViewTab(s *State, tab int, drag *struct{ slider, asset sliderDragState }) {
	if s.foeViewTab == tab {
		return
	}
	s.foeViewTab = tab
	// Reset the CALLER's drag state (foeDrag or partyDrag) — passing it in keeps
	// the foe and party modals symmetric through one seam, instead of this
	// resetting only foeDrag while the party modal had to re-reset partyDrag
	// itself (a drift hazard if that "redundant" reset were ever removed).
	drag.slider = noSliderDrag
	drag.asset = noSliderDrag
}

// setFoeAssetFromTrack maps a mouse X within an Asset-tab slider track to the
// override's adjustment field (Pixelate/Bright/Contrast on s.foeVisual), snapped,
// and flags the live preview for rebuild.
func setFoeAssetFromTrack(s *State, i int, track rl.Rectangle, mouseX float32) {
	f := assetFields[i]
	f.Set(&s.foeVisual, sliderSnap(f.Min, f.Max, f.Step, track.X, track.Width, mouseX))
	s.assetPreviewStale = true
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

// applyPreviewZoomWheel dollies the visualizer preview when the wheel turns over
// the preview pane. Shared by both visualizer modals (foeViewZoom is shared);
// clamped to the render-side bounds. A scroll-up zooms IN (positive wheel ⇒
// larger factor ⇒ closer camera in zoomedPreviewCamera).
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

	// Live 3D preview pane (the diorama is blitted from an off-screen texture).
	// Gizmos show only on the Layout tab so the Asset tab reads as a clean sprite;
	// on the Asset tab the non-destructive bake preview texture overrides the sprite.
	render.DrawFoePreview(l.preview, frameAssets, s.foeKind, s.foeVisual, s.foeViewZoom, s.foeViewTab == foeTabLayout, assetPreviewTexFor(s))
	rl.DrawRectangleLinesEx(l.preview, 1, theme.BorderDim)

	// Foe picker header: < Name >.
	drawButton(font, l.prevFoeBtn, "<", false)
	drawButton(font, l.nextFoeBtn, ">", false)
	name := core.EnemyInfo(s.foeKind).Name + "  ▼" // ▼ = click the name to pick from all kinds
	nameSize := render.MeasureRichText(font, name, editorFontTopbar, 1)
	nameSpanX := l.prevFoeBtn.X + l.prevFoeBtn.Width
	nameSpanW := l.nextFoeBtn.X - nameSpanX
	render.DrawRichText(font, name,
		rl.NewVector2(nameSpanX+(nameSpanW-nameSize.X)/2, l.prevFoeBtn.Y+5),
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

	// Footer hint + persistence note, under the preview pane (clear of the
	// right-side buttons).
	render.DrawTextWithShadow(font,
		"D-pad row/adjust   |   drag sliders   |   buttons: change foe / save / reset / close",
		l.card.X+foePad, l.preview.Y+l.preview.Height+8, editorFontHint, theme.TextHint)
	render.DrawTextWithShadow(font,
		visualizerFooterHint("foe", core.EnemySlug(s.foeKind)),
		l.card.X+foePad, l.preview.Y+l.preview.Height+26, editorFontHint, theme.TextMuted)

}

// drawFoeViewTabs paints the Layout / Asset tab buttons, the active one
// highlighted. Shared by both visualizer modals (they share the layout).
func drawFoeViewTabs(font rl.Font, l foeViewLayout, active int) {
	for i := range l.tabBtns {
		drawButton(font, l.tabBtns[i], foeViewTabLabels[i], i == active)
	}
}

// assetPreviewTexFor returns the live-preview texture for the preview pane. The
// non-destructive FX (pixelate/bright/contrast) are part of the sprite's look, so
// the adjusted texture shows on BOTH tabs (zero texture = no adjustments active,
// the pane falls back to the real sprite).
func assetPreviewTexFor(s *State) rl.Texture2D {
	return render.AssetPreviewTexture()
}

// drawAssetTab paints the Asset tab: the non-destructive image-adjustment sliders
// (Pixelate / Bright / Contrast, editing the override `ov`) + a Revert button + a
// hint. Shared by both visualizer modals (the override + cursor are passed in).
// The live preview reflects the values non-destructively; Save persists them.
func drawAssetTab(font rl.Font, theme render.Theme, l foeViewLayout, ov *core.EnemyVisualOverride, cursor int) {
	for i := range assetFields {
		f := assetFields[i]
		track := l.assetTracks[i]
		value := f.Get(ov)
		drawSlider(font, theme, f.Label, fmt.Sprintf(f.Format, value), value, f.Min, f.Max,
			rl.NewVector2(track.X-foeLabelW, track.Y-4), rl.NewVector2(track.X+track.Width+8, track.Y-4),
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

func drawFoeSlider(font rl.Font, theme render.Theme, l foeViewLayout, i int, s *State) {
	f := foeFields[i]
	track := l.sliderTracks[i]
	focused := s.foeCursor == i
	value := f.Get(&s.foeVisual)
	val := fmt.Sprintf(f.Format, value)
	drawSlider(font, theme, f.Label, val, value, f.Min, f.Max,
		rl.NewVector2(track.X-foeLabelW, track.Y-4), rl.NewVector2(track.X+track.Width+8, track.Y-4),
		editorFontAccent, track, 6, focused)
}

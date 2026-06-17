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

// Party Visualizer (modalPartyView) — the party-side twin of the Foe Visualizer
// (foeview.go). A live combat-preview pane for any party CLASS plus the full
// slider stack for that class's billboard placement, contact shadow, target
// cursor, tint, and the sprite-PNG bake/import strip. Save writes the tuning to
// maps/sprites/partyvisuals.json (core.PartyVisualOverride), which the game
// overlays on its code-default party visuals at load.
//
// It reuses the foe modal's geometry (computeFoeViewLayout), field table
// (foeFields — valid because PartyVisualOverride is a type alias of
// EnemyVisualOverride), button-label slices, and the shared sprite-edit engine.
// Only the lookup (party class vs enemy kind), the seed/save targets, and the
// preview call differ — so the two visualizers can't drift on layout or fields.

// partyDrag holds the in-flight slider drag for the party modal. Shares the
// sliderDragState protocol with foeDrag (see foeview.go); kept as a separate
// instance since the two modals are distinct, even though only one is open at a
// time. slider tracks a Layout-tab field drag, asset an Asset-tab param drag.
var partyDrag = struct{ slider, asset sliderDragState }{slider: noSliderDrag, asset: noSliderDrag}

// openPartyViewModal opens the visualizer. First open seeds the working copy
// from the first class's LIVE visual; later opens keep the working copy so
// unsaved tuning survives an accidental close (mirrors openFoeViewModal).
func openPartyViewModal(s *State) {
	s.modal = modalPartyView
	if !s.partyInit {
		if defs := core.PartyClasses(); len(defs) > 0 {
			s.partyClass = defs[0].Class
		}
		seedPartyVisual(s)
		s.partyInit = true
	}
	s.foeViewTab = foeTabLayout
	s.foeViewZoom = 1
	enterAssetEditing(s)
	partyDrag.slider = noSliderDrag
	partyDrag.asset = noSliderDrag
}

// seedPartyVisual loads the current class's live visual into the working copy
// and snapshots it as the Reset baseline.
func seedPartyVisual(s *State) {
	if ov, ok := render.LivePartyOverride(frameAssets, s.partyClass); ok {
		s.partyVisual = ov
		s.partyBaseline = ov
	} else {
		// No resolvable visual (defensive — every class is populated at load).
		// Seed a visible unit size so a Save never persists an invisible size 0.
		base := core.PartyVisualOverride{SizeX: 1, SizeY: 1}
		s.partyVisual = base
		s.partyBaseline = base
	}
	s.partyCursor = 0
}

// cyclePartyClass steps to the prev/next class (wrapping) and re-seeds.
func cyclePartyClass(s *State, dir int) {
	s.partyClass = cyclePartyClassKind(s.partyClass, dir)
	seedPartyVisual(s)
	enterAssetEditing(s) // rebuild the live preview from the new class's saved FX
}

// cyclePartyClassKind walks the class registry by delta (+1 / -1), wrapping.
func cyclePartyClassKind(cur core.PartyClass, delta int) core.PartyClass {
	defs := core.PartyClasses()
	if len(defs) == 0 {
		return cur
	}
	idx := 0
	for i, def := range defs {
		if def.Class == cur {
			idx = i
			break
		}
	}
	idx = core.WrapIndex(idx+delta, len(defs))
	return defs[idx].Class
}

// partyClassName returns the display name for the tuned class (for the header
// label + flash text), falling back to the slug if the class is somehow absent.
func partyClassName(class core.PartyClass) string {
	for _, def := range core.PartyClasses() {
		if def.Class == class {
			return def.Name
		}
	}
	return core.PartyClassSlug(class)
}

func savePartyVisual(s *State) {
	slug := core.PartyClassSlug(s.partyClass)
	if err := core.SavePartyVisualOverride(slug, s.partyVisual); err != nil {
		s.flashWarn("Save failed: " + err.Error())
		return
	}
	// Mirror into the live in-memory visual so cycling away and back re-seeds
	// from the SAVED values and the editor preview updates immediately.
	render.SetLivePartyOverride(frameAssets, s.partyClass, s.partyVisual)
	s.partyBaseline = s.partyVisual
	s.flash("Saved " + partyClassName(s.partyClass) + " → " + slug + " (live in editor; restart game to apply)")
}

func updatePartyViewModal(s *State) Action {
	if editorCancelPressed() {
		render.ClosePartyPreview()
		render.ClearAssetPreview()
		closeModal(s)
		return ActionNone
	}

	// "Upload": a PNG dropped while the modal is open imports as THIS class's
	// sprite (drag-drop is the path; raylib has no file dialog). First .png only.
	if rl.IsFileDropped() {
		dropped := rl.LoadDroppedFiles()
		for _, path := range dropped {
			if !strings.EqualFold(filepath.Ext(path), ".png") {
				continue
			}
			slug := core.PartyClassSlug(s.partyClass)
			if err := render.ImportPartySpriteFromFile(s.partyClass, path); err != nil {
				s.flashWarn("Import failed: " + err.Error())
			} else {
				render.ReloadPartySprite(frameAssets, s.partyClass)
				s.flash("Imported " + filepath.Base(path) + " → " + slug + ".png (updated live)")
			}
			break
		}
		rl.UnloadDroppedFiles()
	}

	l := computeFoeViewLayout()
	// Read the cursor live (not the one-frame-stale cached frameMouse), matching
	// updateFoeViewModal.
	mp := rl.GetMousePosition()
	mouseDown := rl.IsMouseButtonDown(rl.MouseLeftButton)
	mousePressed := rl.IsMouseButtonPressed(rl.MouseLeftButton)
	mouseReleased := rl.IsMouseButtonReleased(rl.MouseLeftButton)

	partyDrag.slider.update(mouseDown && s.foeViewTab == foeTabLayout, len(foeFields), func(idx int) {
		setPartyFieldFromTrack(s, idx, l.sliderTracks[idx], mp.X)
		s.partyCursor = idx
	})
	partyDrag.asset.update(mouseDown && s.foeViewTab == foeTabAsset, len(assetFields), func(idx int) {
		setPartyAssetFromTrack(s, idx, l.assetTracks[idx], mp.X)
		s.assetCursor = idx
	})

	applyPreviewZoomWheel(s, l.preview, mp)

	if mousePressed {
		handlePartyViewClick(s, &l, mp)
	}
	if mouseReleased {
		partyDrag.slider = noSliderDrag
		partyDrag.asset = noSliderDrag
	}

	if editorCommitPressed() {
		savePartyVisual(s)
		return ActionNone
	}

	if s.foeViewTab == foeTabLayout {
		s.partyCursor = input.CursorUpDown(s.partyCursor, len(foeFields))
		if s.partyCursor >= 0 && s.partyCursor < len(foeFields) {
			if delta := input.CursorLeftRight(); delta != 0 {
				f := foeFields[s.partyCursor]
				v := f.Get(&s.partyVisual) + float64(delta)*f.Step
				f.Set(&s.partyVisual, core.Clamp(v, f.Min, f.Max))
			}
		}
	} else {
		s.assetCursor = input.CursorUpDown(s.assetCursor, len(assetFields))
		if s.assetCursor >= 0 && s.assetCursor < len(assetFields) {
			if delta := input.CursorLeftRight(); delta != 0 {
				f := assetFields[s.assetCursor]
				v := f.Get(&s.partyVisual) + float64(delta)*f.Step
				f.Set(&s.partyVisual, core.Clamp(v, f.Min, f.Max))
				s.assetPreviewStale = true
			}
		}
	}
	if s.assetPreviewStale {
		render.RefreshPartyAssetPreview(frameAssets, s.partyClass, s.partyVisual)
		s.assetPreviewStale = false
	}
	return ActionNone
}

// setPartyAssetFromTrack is the party twin of setFoeAssetFromTrack — edits the
// adjustment field on s.partyVisual and flags the preview for rebuild.
func setPartyAssetFromTrack(s *State, i int, track rl.Rectangle, mouseX float32) {
	f := assetFields[i]
	f.Set(&s.partyVisual, sliderSnap(f.Min, f.Max, f.Step, track.X, track.Width, mouseX))
	s.assetPreviewStale = true
}

// handlePartyViewClick dispatches a left-press: slider tracks, sprite-PNG bake
// buttons, the class prev/next arrows (and a click on the name cycles forward —
// with only four classes a dropdown would be overkill), and Save/Reset/Close.
func handlePartyViewClick(s *State, l *foeViewLayout, mp rl.Vector2) {
	for i := range l.tabBtns {
		if pointIn(mp, l.tabBtns[i]) {
			selectFoeViewTab(s, i)
			partyDrag.slider = noSliderDrag
			partyDrag.asset = noSliderDrag
			return
		}
	}
	if s.foeViewTab == foeTabLayout {
		for i := range foeFields {
			if pointIn(mp, padRect(l.sliderTracks[i], 0, 9)) {
				partyDrag.slider.idx = i
				setPartyFieldFromTrack(s, i, l.sliderTracks[i], mp.X)
				s.partyCursor = i
				return
			}
		}
	}
	if s.foeViewTab == foeTabAsset {
		for i := range assetFields {
			if pointIn(mp, padRect(l.assetTracks[i], 0, 9)) {
				partyDrag.asset.idx = i
				setPartyAssetFromTrack(s, i, l.assetTracks[i], mp.X)
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
				clearVisualAdjustments(&s.partyVisual)
				s.assetPreviewStale = true
				s.flash("Reverted sprite FX (Pixelate / Bright / Contrast)")
			}
			return
		}
	}
	if pointIn(mp, l.prevFoeBtn) {
		cyclePartyClass(s, -1)
		return
	}
	if pointIn(mp, l.nextFoeBtn) {
		cyclePartyClass(s, +1)
		return
	}
	if nameSpan := nameSpanBetween(l.prevFoeBtn, l.nextFoeBtn); pointIn(mp, nameSpan) {
		cyclePartyClass(s, +1)
		return
	}
	if pointIn(mp, l.saveBtn) {
		savePartyVisual(s)
		return
	}
	if pointIn(mp, l.resetBtn) {
		s.partyVisual = s.partyBaseline
		s.flash("Reset to last-saved values")
		return
	}
	if pointIn(mp, l.closeBtn) {
		render.ClosePartyPreview()
		render.ClearAssetPreview()
		closeModal(s)
		return
	}
}

// setPartyFieldFromTrack maps a mouse X within a slider track to the field's
// range, snapped to its step grain.
func setPartyFieldFromTrack(s *State, i int, track rl.Rectangle, mouseX float32) {
	f := foeFields[i]
	f.Set(&s.partyVisual, sliderSnap(f.Min, f.Max, f.Step, track.X, track.Width, mouseX))
}

func drawPartyViewModal(s *State, font rl.Font, theme render.Theme) {
	l := computeFoeViewLayout()
	drawModalHeaderAt(font, theme, l.card, "PARTY VISUALIZER", theme.BorderActive)

	// Live 3D preview pane (blitted from an off-screen texture). Gizmos show only
	// on the Layout tab; on the Asset tab the non-destructive bake preview overrides.
	render.DrawPartyPreview(l.preview, frameAssets, s.partyClass, s.partyVisual, s.foeViewZoom, s.foeViewTab == foeTabLayout, assetPreviewTexFor(s))
	rl.DrawRectangleLinesEx(l.preview, 1, theme.BorderDim)

	// Class picker header: < Name >.
	drawButton(font, l.prevFoeBtn, "<", false)
	drawButton(font, l.nextFoeBtn, ">", false)
	name := partyClassName(s.partyClass)
	nameSize := render.MeasureRichText(font, name, editorFontTopbar, 1)
	nameSpanX := l.prevFoeBtn.X + l.prevFoeBtn.Width
	nameSpanW := l.nextFoeBtn.X - nameSpanX
	render.DrawRichText(font, name,
		rl.NewVector2(nameSpanX+(nameSpanW-nameSize.X)/2, l.prevFoeBtn.Y+5),
		editorFontTopbar, 1, theme.TextPrimary)

	drawFoeViewTabs(font, l, s.foeViewTab)
	if s.foeViewTab == foeTabLayout {
		for i := range foeFields {
			drawPartySlider(font, theme, l, i, s)
		}
	} else {
		drawAssetTab(font, theme, l, &s.partyVisual, s.assetCursor)
	}

	drawModalButtons(font, []rl.Rectangle{l.saveBtn, l.resetBtn, l.closeBtn}, foeViewBtnLabels)

	render.DrawTextWithShadow(font,
		"D-pad row/adjust   |   drag sliders   |   buttons: change class / save / reset / close",
		l.card.X+foePad, l.preview.Y+l.preview.Height+8, editorFontHint, theme.TextHint)
	render.DrawTextWithShadow(font,
		"orange sphere = particle origin   ·   cyan = hit glyph   ·   saves to partyvisuals.json as \""+core.PartyClassSlug(s.partyClass)+"\"",
		l.card.X+foePad, l.preview.Y+l.preview.Height+26, editorFontHint, theme.TextMuted)
}

func drawPartySlider(font rl.Font, theme render.Theme, l foeViewLayout, i int, s *State) {
	f := foeFields[i]
	track := l.sliderTracks[i]
	focused := s.partyCursor == i
	value := f.Get(&s.partyVisual)
	val := fmt.Sprintf(f.Format, value)
	drawSlider(font, theme, f.Label, val, value, f.Min, f.Max,
		rl.NewVector2(track.X-foeLabelW, track.Y-4), rl.NewVector2(track.X+track.Width+8, track.Y-4),
		editorFontAccent, track, 6, focused)
}

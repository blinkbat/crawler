package editor

import (
	"crawler/internal/app/core"
	"crawler/internal/app/render"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Party Visualizer (modalPartyView): party-side twin of the Foe Visualizer, for a
// CLASS. Save writes to partyvisuals.json. Reuses the foe modal's geometry and
// foeFields (PartyVisualOverride aliases EnemyVisualOverride); only the lookup,
// seed/save targets, and preview call differ.

// partyDrag is the party modal's in-flight drag (twin of foeDrag).
var partyDrag = struct{ slider, asset sliderDragState }{slider: noSliderDrag, asset: noSliderDrag}

// openPartyViewModal opens the visualizer (mirrors openFoeViewModal).
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

// seedPartyVisual loads the class's live visual into the working copy + baseline.
func seedPartyVisual(s *State) {
	if ov, ok := render.LivePartyOverride(frameAssets, s.partyClass); ok {
		s.partyVisual = ov
		s.partyBaseline = ov
	} else {
		// No resolvable visual (defensive). Unit size so a Save never persists 0.
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
	enterAssetEditing(s)
}

// cyclePartyClassKind walks the class registry by delta (+1 / -1), wrapping.
func cyclePartyClassKind(cur core.PartyClass, delta int) core.PartyClass {
	return cycleRegistry(core.PartyClasses(), func(d core.PartyClassDefinition) core.PartyClass { return d.Class }, cur, delta)
}

func savePartyVisual(s *State) {
	slug := core.PartyClassSlug(s.partyClass)
	if err := core.SavePartyVisualOverride(slug, s.partyVisual); err != nil {
		s.flashWarn("Save failed: " + err.Error())
		return
	}
	// Mirror into the live visual so cycling away and back re-seeds from saved.
	render.SetLivePartyOverride(frameAssets, s.partyClass, s.partyVisual)
	s.partyBaseline = s.partyVisual
	s.flash(savedVisualFlash(core.PartyClassName(s.partyClass), slug))
}

// partyViewCallbacks builds the Party Visualizer's per-modal actions for the
// shared driver. Clicking the name cycles forward (only four classes, no dropdown).
func partyViewCallbacks(s *State) visualizerCallbacks {
	return visualizerCallbacks{
		drag:     &partyDrag,
		override: &s.partyVisual,
		cursor:   &s.partyCursor,
		importPNG: func() {
			importDroppedPNG(s, core.PartyClassSlug(s.partyClass),
				func(path string) error { return render.ImportPartySpriteFromFile(s.partyClass, path) },
				func() { render.ReloadPartySprite(frameAssets, s.partyClass) })
		},
		cyclePrev: func() { cyclePartyClass(s, -1) },
		cycleNext: func() { cyclePartyClass(s, +1) },
		nameSpan:  func(rl.Rectangle) { cyclePartyClass(s, +1) },
		save:      func() { savePartyVisual(s) },
		reset: func() {
			s.partyVisual = s.partyBaseline
			s.flash("Reset to last-saved values")
		},
		closePreview:   render.ClosePartyPreview,
		refreshPreview: func() { render.RefreshPartyAssetPreview(frameAssets, s.partyClass, s.partyVisual) },
	}
}

func updatePartyViewModal(s *State) Action {
	return updateVisualizerModal(s, partyViewCallbacks(s))
}

func drawPartyViewModal(s *State, font rl.Font, theme render.Theme) {
	l := computeFoeViewLayout()
	drawModalHeaderAt(font, theme, l.card, "PARTY VISUALIZER", theme.BorderActive)

	// Live 3D preview. Gizmos only on the Layout tab; Asset tab uses the bake preview.
	render.DrawPartyPreview(l.preview, frameAssets, s.partyClass, s.partyVisual, s.foeViewZoom, s.foeViewTab == foeTabLayout, assetPreviewTexFor())
	rl.DrawRectangleLinesEx(l.preview, 1, theme.BorderDim)

	drawButton(font, l.prevFoeBtn, "<", false)
	drawButton(font, l.nextFoeBtn, ">", false)
	name := core.PartyClassName(s.partyClass)
	nameSize := render.MeasureRichText(font, name, editorFontTopbar, 1)
	span := nameSpanBetween(l.prevFoeBtn, l.nextFoeBtn)
	render.DrawRichText(font, name,
		rl.NewVector2(span.X+(span.Width-nameSize.X)/2, l.prevFoeBtn.Y+5),
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
		visualizerFooterHint("class", core.PartyClassSlug(s.partyClass)),
		l.card.X+foePad, l.preview.Y+l.preview.Height+26, editorFontHint, theme.TextMuted)
}

func drawPartySlider(font rl.Font, theme render.Theme, l foeViewLayout, i int, s *State) {
	drawVisualSlider(font, theme, l, i, &s.partyVisual, s.partyCursor)
}

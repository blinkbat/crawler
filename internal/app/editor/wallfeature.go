package editor

import (
	"crawler/internal/app/core"
	"crawler/internal/app/render"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Wall-feature authoring: place an interactive fixture (switch / bombable / secret)
// on a tile face, then edit its kind, face direction, target switch, and once flag.
// The fixture sets its Switch when the party activates it; author a trigger on that
// switch to do the work (open a wall, spawn a foe, …). See core/wallfeature.go.

const (
	wallFeatureModalW = modalCardNarrowW
	wallFeatureModalH = float32(340)
)

func currentWallFeature(s *State) *core.WallFeature {
	if s.modalWallFeatureIdx < 0 || s.modalWallFeatureIdx >= len(s.area.WallFeatures) {
		return nil
	}
	return &s.area.WallFeatures[s.modalWallFeatureIdx]
}

// uniqueWallFeatureID returns an unused "wf<N>" id for a new fixture.
func uniqueWallFeatureID(s *State) string {
	used := make(map[string]bool, len(s.area.WallFeatures))
	for _, f := range s.area.WallFeatures {
		used[f.ID] = true
	}
	return uniqueID("wf", func(id string) bool { return used[id] })
}

// addWallFeatureAt places a switch fixture on tile (x,z), oriented toward the open
// side (away from an adjacent wall, like a torch), and opens its editor. No-op if a
// fixture already sits on the tile (one per tile keeps placement/rendering simple).
func addWallFeatureAt(s *State, x, z int) {
	if !s.area.InBounds(x, z) || core.WallFeatureAnyAt(s.area.WallFeatures, x, z) >= 0 {
		return
	}
	pushUndo(s)
	dir, _ := core.FacingAwayFromAdjacentWall(&s.area, x, z)
	f := core.WallFeature{ID: uniqueWallFeatureID(s), Kind: core.WallSwitch, X: x, Z: z, Dir: dir}
	s.area.WallFeatures = append(s.area.WallFeatures, f)
	s.dirty = true
	openWallFeatureEditModal(s, len(s.area.WallFeatures)-1)
}

// wallFeatureKindEntries picks the fixture kind (switch / bombable / secret).
func wallFeatureKindEntries(s *State) []dropdownEntry {
	return fieldEntries(core.WallFeatureKinds(), core.WallFeatureKindLabel, func(s *State, k core.WallFeatureKind) {
		if f := currentWallFeature(s); f != nil {
			setIfChanged(s, &f.Kind, k)
		}
	})
}

type wallFeatureLayout struct {
	card        rl.Rectangle
	kindBtn     rl.Rectangle
	dirBtn      rl.Rectangle
	switchField rl.Rectangle
	onceToggle  rl.Rectangle
	deleteBtn   rl.Rectangle
	doneBtn     rl.Rectangle
}

func wallFeatureLayoutFor() wallFeatureLayout {
	r := centeredCardRect(wallFeatureModalW, wallFeatureModalH)
	x, fw := cardContentBox(r)
	y := r.Y + dialogHeaderInset
	fieldH := dialogFieldH
	f := stackRows(x, y, fw, fieldH, dialogRowGap, 4)
	return wallFeatureLayout{
		card:        r,
		kindBtn:     f[0],
		dirBtn:      f[1],
		switchField: f[2],
		onceToggle:  f[3],
		deleteBtn:   bottomLeftBtn(r),
		doneBtn:     bottomRightBtn(r),
	}
}

func openWallFeatureEditModal(s *State, idx int) {
	if idx < 0 || idx >= len(s.area.WallFeatures) {
		return
	}
	s.modal = modalWallFeatureEdit
	s.modalWallFeatureIdx = idx
	s.focus = focusNone
}

func drawWallFeatureEditModal(s *State, font rl.Font, theme render.Theme) {
	f := currentWallFeature(s)
	if f == nil {
		return
	}
	l := wallFeatureLayoutFor()
	drawModalHeaderAt(font, theme, l.card, "WALL FEATURE AT "+core.TileCoord(f.X, f.Z), theme.BorderActive)

	drawLabel(font, "Kind (click to choose)", labelAbove(l.kindBtn))
	drawButton(font, l.kindBtn, core.WallFeatureKindLabel(f.Kind)+dropdownArrowSuffix, s.dropdown.owner == ddWallFeatureKind)

	drawLabel(font, "Face (click to cycle N/E/S/W)", labelAbove(l.dirBtn))
	dirName, _ := core.FacingName(f.Dir)
	drawButton(font, l.dirBtn, dirName, false)

	drawLabel(font, "Sets switch (name)", labelAbove(l.switchField))
	drawTextField(font, l.switchField, f.Switch, s.focus == focusWallFeatureSwitch)

	drawButton(font, l.onceToggle, "Fire once (M): "+render.OnOffLabel(f.Once), f.Once)
	drawButton(font, l.deleteBtn, "Delete (X)", s.deleteArmed == "wallfeature")
	drawButton(font, l.doneBtn, "Done (Esc)", false)
}

func updateWallFeatureEditModal(s *State) Action {
	f := currentWallFeature(s)
	if f == nil {
		closeModal(s)
		return ActionNone
	}
	l := wallFeatureLayoutFor()

	if rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		mp := rl.GetMousePosition()
		switch {
		case pointIn(mp, l.kindBtn):
			openFieldDropdown(s, ddWallFeatureKind, l.kindBtn)
		case pointIn(mp, l.dirBtn):
			pushUndo(s)
			f.Dir = core.NormalizeFacing(f.Dir + 1)
			s.dirty = true
		case pointIn(mp, l.switchField):
			s.focus = focusWallFeatureSwitch
		case pointIn(mp, l.onceToggle):
			toggleWallFeatureOnce(s)
		case pointIn(mp, l.deleteBtn):
			deleteWallFeatureAt(s, s.modalWallFeatureIdx)
		case pointIn(mp, l.doneBtn):
			closeModal(s)
		default:
			s.focus = focusNone
		}
		return ActionNone
	}

	if s.focus == focusWallFeatureSwitch {
		pumpFocusField(s, &f.Switch)
		if editorCommitPressed() {
			s.focus = focusNone
		}
		if editorCancelPressed() {
			closeModal(s)
		}
		return ActionNone
	}
	if editorCancelPressed() || editorCommitPressed() {
		closeModal(s)
		return ActionNone
	}
	if editorDeletePressed() {
		deleteWallFeatureAt(s, s.modalWallFeatureIdx)
		return ActionNone
	}
	if rl.IsKeyPressed(rl.KeyM) {
		toggleWallFeatureOnce(s)
	}
	return ActionNone
}

func toggleWallFeatureOnce(s *State) {
	if f := currentWallFeature(s); f != nil {
		pushUndo(s)
		f.Once = !f.Once
		s.dirty = true
	}
}

func deleteWallFeatureAt(s *State, idx int) {
	if idx < 0 || idx >= len(s.area.WallFeatures) {
		return
	}
	if !armOrConfirmDelete(s, "wallfeature", "Delete this wall feature? Click Delete (or press X) again to confirm") {
		return
	}
	pushUndo(s)
	s.area.WallFeatures = removeModalListItem(s.area.WallFeatures, idx)
	s.dirty = true
	closeModal(s)
}

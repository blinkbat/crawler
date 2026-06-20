package editor

import (
	"crawler/internal/app/core"
	"crawler/internal/app/render"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Wall-faces modal geometry. A compact button-stack card: one row per face
// (base + N/E/S/W), each opening the shared face-skin dropdown for that face.
// Sized for the five rows + the header band.
const (
	wallFacesModalW = float32(380)
	wallFacesModalH = float32(286)
)

// openWallFacesModal opens the per-tile wall-face editor for (x, z). Reached
// from the right-click context menu's "Set wall faces…" row. Faces are a
// per-tile property (a top-down editor can't paint a vertical face), so they
// live in this modal rather than on a paintable layer.
func openWallFacesModal(s *State, x, z int) {
	if !s.area.InBounds(x, z) {
		return
	}
	s.modal = modalWallFaces
	s.wallFaceX, s.wallFaceZ = x, z
	s.modalCursor = 0
}

// wallFaceDirs is the ordered face list the modal exposes: the base skin
// (dir -1, written to the Walls grid via applyFaceSkin) followed by each
// cardinal override. Direction values match core's facing constants.
var wallFaceDirs = []int{-1, core.North, core.East, core.South, core.West}

// wallFaceCmds builds the modal's rows fresh each frame so each label reflects
// the live skin as the author edits. Each row's run sets the shared face
// target (tile + this row's direction) and opens ddFaceSkin anchored at the
// row — reusing faceSkinEntries / applyFaceSkin so the picker logic lives once.
func wallFaceCmds(s *State) []modalCmd {
	x, z := s.wallFaceX, s.wallFaceZ
	labels := make([]string, len(wallFaceDirs))
	for i, d := range wallFaceDirs {
		var skin byte
		var face string
		if d < 0 {
			skin = s.area.FaceSkinAt(x, z)
			face = "Base (all faces)"
		} else {
			skin = s.area.FaceSkinForDir(x, z, d)
			face = core.FacingShortLabels[d] + " face"
		}
		labels[i] = face + ":  " + core.FaceSkinName(skin)
	}
	rects := modalButtonStack(centeredCardRect(wallFacesModalW, wallFacesModalH), labels)
	cmds := make([]modalCmd, len(wallFaceDirs))
	for i, d := range wallFaceDirs {
		dir := d
		anchor := rects[i]
		cmds[i] = modalCmd{label: labels[i], run: func() Action {
			s.faceTargetX, s.faceTargetZ, s.faceTargetDir = x, z, dir
			openDropdownBelow(s, ddFaceSkin, anchor)
			return ActionNone
		}}
	}
	return cmds
}

func drawWallFacesModal(s *State, font rl.Font, theme render.Theme) {
	card := drawModalHeader(font, theme, wallFacesModalW, wallFacesModalH,
		"WALL FACES AT "+core.TileCoord(s.wallFaceX, s.wallFaceZ), theme.BorderActive)
	labels := cmdLabels(wallFaceCmds(s))
	drawModalButtons(font, modalButtonStack(card, labels), labels)
	render.DrawTextWithShadow(font, "Pick a face to set its cliff-face skin · Esc closes",
		card.X+modalContentInset, card.Y+44, editorFontHint, theme.TextHint)
}

func updateWallFacesModal(s *State) Action {
	if editorCancelPressed() {
		closeModal(s)
		return ActionNone
	}
	// The tile can leave the map if it shrank while the modal was open
	// (mirrors validateModalState's index guards for the entity modals).
	if !s.area.InBounds(s.wallFaceX, s.wallFaceZ) {
		closeModal(s)
		return ActionNone
	}
	cmds := wallFaceCmds(s)
	rects := modalButtonStack(centeredCardRect(wallFacesModalW, wallFacesModalH), cmdLabels(cmds))
	if act, ran := runModalCmds(cmds, rects); ran {
		return act
	}
	return ActionNone
}

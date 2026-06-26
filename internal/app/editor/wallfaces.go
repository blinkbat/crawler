package editor

import (
	"crawler/internal/app/core"
	"crawler/internal/app/render"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Wall-faces modal: one row per face (base + N/E/S/W), each opening ddFaceSkin.
const (
	wallFacesModalW = float32(380)
	wallFacesModalH = float32(286)
)

// openWallFacesModal opens the per-tile wall-face editor for (x, z).
func openWallFacesModal(s *State, x, z int) {
	if !s.area.InBounds(x, z) {
		return
	}
	s.modal = modalWallFaces
	s.wallFaceX, s.wallFaceZ = x, z
	s.modalCursor = 0
}

// wallFaceDirs: base skin (dir -1) then each cardinal override (core facing values).
var wallFaceDirs = []int{-1, core.North, core.East, core.South, core.West}

// wallFacesLayout rebuilds the rows (labels reflect the live skin) AND their button
// rects in one place, so the cmd dropdown anchors, the draw pass, and the click
// hit-test all share one geometry instead of each re-deriving the stack.
func wallFacesLayout(s *State) (cmds []modalCmd, rects []rl.Rectangle) {
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
	rects = modalButtonStack(centeredCardRect(wallFacesModalW, wallFacesModalH), labels)
	cmds = make([]modalCmd, len(wallFaceDirs))
	for i, d := range wallFaceDirs {
		dir := d
		anchor := rects[i]
		cmds[i] = modalCmd{label: labels[i], run: func() Action {
			s.faceTargetX, s.faceTargetZ, s.faceTargetDir = x, z, dir
			openDropdownBelow(s, ddFaceSkin, anchor)
			return ActionNone
		}}
	}
	return cmds, rects
}

func drawWallFacesModal(s *State, font rl.Font, theme render.Theme) {
	card := drawModalHeader(font, theme, wallFacesModalW, wallFacesModalH,
		"WALL FACES AT "+core.TileCoord(s.wallFaceX, s.wallFaceZ), theme.BorderActive)
	cmds, rects := wallFacesLayout(s)
	drawModalButtons(font, rects, cmdLabels(cmds))
	render.DrawTextWithShadow(font, "Pick a face to set its cliff-face skin · Esc closes",
		card.X+modalContentInset, card.Y+44, editorFontHint, theme.TextHint)
}

func updateWallFacesModal(s *State) Action {
	if editorCancelPressed() {
		closeModal(s)
		return ActionNone
	}
	// Tile can leave the map if it shrank while the modal was open.
	if !s.area.InBounds(s.wallFaceX, s.wallFaceZ) {
		closeModal(s)
		return ActionNone
	}
	cmds, rects := wallFacesLayout(s)
	if act, ran := runModalCmds(cmds, rects); ran {
		return act
	}
	return ActionNone
}

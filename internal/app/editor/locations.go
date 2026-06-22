package editor

import (
	"crawler/internal/app/core"
	"crawler/internal/app/render"
	"strconv"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// locations.go is the editor authoring + visualization for named regions
// (core.Location). Regions are created from the right-click menu ("New location
// here"), edited in modalLocationEdit (name / bounds / level / delete), and drawn
// on the canvas as labeled translucent rects — level-faded like every other layer.

// locationFillCol / locationLineCol / locationLabelCol style the canvas overlay —
// a soft violet, distinct from the amber Select marquee and entity markers.
var (
	locationFillCol  = rl.NewColor(150, 110, 220, 46)
	locationLineCol  = rl.NewColor(196, 164, 248, 220)
	locationLabelCol = rl.NewColor(224, 210, 250, 255)
)

// newLocationID returns the first free "location_N" id (bg2-style auto-increment).
func newLocationID(locs []core.Location) string {
	for n := 1; ; n++ {
		id := "location_" + strconv.Itoa(n)
		if _, taken := core.LocationByID(locs, id); !taken {
			return id
		}
	}
}

// createLocationAt appends a default region anchored at (x,z) on the active level,
// clamped to the map, then opens its editor. The single create path (right-click).
func createLocationAt(s *State, x, z int) {
	if !s.area.InBounds(x, z) {
		return
	}
	w, h := 3, 3
	if x+w > s.area.Width {
		w = s.area.Width - x
	}
	if z+h > s.area.Height {
		h = s.area.Height - z
	}
	pushUndo(s)
	s.area.Locations = append(s.area.Locations, core.Location{
		ID: newLocationID(s.area.Locations), X: x, Z: z, W: w, H: h, Level: s.editLevel,
	})
	s.dirty = true
	openLocationEditModal(s, len(s.area.Locations)-1)
}

func openLocationEditModal(s *State, idx int) {
	if idx < 0 || idx >= len(s.area.Locations) {
		return
	}
	s.modal = modalLocationEdit
	s.modalLocationIdx = idx
	s.focus = focusLocationName
}

// currentLocation returns the region being edited, or nil if the index is stale.
func currentLocation(s *State) *core.Location {
	if s.modalLocationIdx < 0 || s.modalLocationIdx >= len(s.area.Locations) {
		return nil
	}
	return &s.area.Locations[s.modalLocationIdx]
}

const (
	locModalW = float32(460)
	locModalH = float32(430)
)

// locStepper is one numeric field's hit rects (a value flanked by −/+ buttons).
type locStepper struct {
	row, minus, plus rl.Rectangle
}

type locEditLayout struct {
	card               rl.Rectangle
	nameField          rl.Rectangle
	x, z, w, h, level  locStepper
	deleteBtn, backBtn rl.Rectangle
}

// stepperFor splits a full-width row into a label area (left) + −/+ buttons (right).
func stepperFor(row rl.Rectangle) locStepper {
	const btn = float32(34)
	plus := rl.NewRectangle(row.X+row.Width-btn, row.Y, btn, row.Height)
	minus := rl.NewRectangle(plus.X-btn-6, row.Y, btn, row.Height)
	return locStepper{row: row, minus: minus, plus: plus}
}

func locEditLayoutFor() locEditLayout {
	r := centeredCardRect(locModalW, locModalH)
	x := r.X + modalContentInset
	fw := r.Width - 2*modalContentInset
	fieldH := dialogFieldH
	y := r.Y + dialogHeaderInset
	rows := stackRows(x, y, fw, fieldH, dialogTrigRowGap, 6)
	return locEditLayout{
		card:      r,
		nameField: rows[0],
		x:         stepperFor(rows[1]),
		z:         stepperFor(rows[2]),
		w:         stepperFor(rows[3]),
		h:         stepperFor(rows[4]),
		level:     stepperFor(rows[5]),
		deleteBtn: bottomLeftBtn(r),
		backBtn:   bottomRightBtn(r),
	}
}

func drawLocationEditModal(s *State, font rl.Font, theme render.Theme) {
	loc := currentLocation(s)
	if loc == nil {
		return
	}
	l := locEditLayoutFor()
	drawModalHeaderAt(font, theme, l.card, "LOCATION "+loc.ID, theme.BorderActive)

	drawLabel(font, "Name", labelAbove(l.nameField))
	drawTextField(font, l.nameField, loc.Name, s.focus == focusLocationName)

	drawLocStepper(font, l.x, "Tile X", loc.X)
	drawLocStepper(font, l.z, "Tile Z", loc.Z)
	drawLocStepper(font, l.w, "Width", loc.W)
	drawLocStepper(font, l.h, "Height", loc.H)
	drawLocStepper(font, l.level, "Level (elevation)", loc.Level)

	drawButton(font, l.deleteBtn, "Delete (X)", false)
	drawButton(font, l.backBtn, "Back (Esc)", false)
}

// drawLocStepper renders a "label   value  [−][+]" row.
func drawLocStepper(font rl.Font, st locStepper, label string, value int) {
	drawLabel(font, label, labelAbove(st.row))
	drawTextField(font, rl.NewRectangle(st.row.X, st.row.Y, st.minus.X-st.row.X-6, st.row.Height), strconv.Itoa(value), false)
	drawButton(font, st.minus, "−", false)
	drawButton(font, st.plus, "+", false)
}

func updateLocationEditModal(s *State) Action {
	loc := currentLocation(s)
	if loc == nil {
		closeModal(s)
		return ActionNone
	}
	l := locEditLayoutFor()

	if rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		mp := rl.GetMousePosition()
		switch {
		case pointIn(mp, l.nameField):
			s.focus = focusLocationName
			return ActionNone
		case pointIn(mp, l.deleteBtn):
			deleteCurrentLocation(s)
			return ActionNone
		case pointIn(mp, l.backBtn):
			closeModal(s)
			return ActionNone
		}
		// Steppers: each adjusts its field by ±1, clamped. A click banks one undo.
		for _, st := range []struct {
			s   locStepper
			get func() int
			set func(int)
		}{
			{l.x, func() int { return loc.X }, func(v int) { loc.X = v }},
			{l.z, func() int { return loc.Z }, func(v int) { loc.Z = v }},
			{l.w, func() int { return loc.W }, func(v int) { loc.W = v }},
			{l.h, func() int { return loc.H }, func(v int) { loc.H = v }},
			{l.level, func() int { return loc.Level }, func(v int) { loc.Level = v }},
		} {
			if pointIn(mp, st.s.minus) {
				adjustLocationField(s, st.get, st.set, -1)
				return ActionNone
			}
			if pointIn(mp, st.s.plus) {
				adjustLocationField(s, st.get, st.set, +1)
				return ActionNone
			}
		}
		s.focus = focusNone
	}

	if s.focus == focusLocationName {
		pumpFocusField(s, &loc.Name)
		if editorCommitPressed() {
			s.focus = focusNone
			return ActionNone
		}
		if editorCancelPressed() {
			closeModal(s)
			return ActionNone
		}
		return ActionNone
	}
	if editorCancelPressed() || editorCommitPressed() {
		closeModal(s)
		return ActionNone
	}
	if rl.IsKeyPressed(rl.KeyX) {
		deleteCurrentLocation(s)
	}
	return ActionNone
}

// adjustLocationField applies a clamped ±1 to a region field, banking undo + dirty
// only on a real change. Bounds keep the region on-map and at least 1×1.
func adjustLocationField(s *State, get func() int, set func(int), delta int) {
	loc := currentLocation(s)
	if loc == nil {
		return
	}
	before := core.CloneArea(s.area)
	set(get() + delta)
	clampLocation(s, loc)
	if core.AreaContentEqual(s.area, before) {
		return // clamp ate the change — no undo/dirty churn
	}
	commitUndoSnapshot(s, before)
	s.dirty = true
}

// clampLocation keeps a region within the map and at least 1×1 on a valid level.
func clampLocation(s *State, loc *core.Location) {
	loc.W = core.Clamp(loc.W, 1, s.area.Width)
	loc.H = core.Clamp(loc.H, 1, s.area.Height)
	loc.X = core.Clamp(loc.X, 0, s.area.Width-loc.W)
	loc.Z = core.Clamp(loc.Z, 0, s.area.Height-loc.H)
	loc.Level = core.Clamp(loc.Level, 0, maxEditLevel)
}

func deleteCurrentLocation(s *State) {
	if s.modalLocationIdx < 0 || s.modalLocationIdx >= len(s.area.Locations) {
		closeModal(s)
		return
	}
	pushUndo(s)
	i := s.modalLocationIdx
	s.area.Locations = append(s.area.Locations[:i], s.area.Locations[i+1:]...)
	s.dirty = true
	closeModal(s)
}

// drawLocations paints every region's rect on the canvas: a translucent fill +
// outline + label, faded by distance from the active level (context, like props).
func drawLocations(s *State, font rl.Font) {
	if s.layerHidden[LayerEntities] {
		// Regions ride the entity-layer eye: hiding "objects" hides them too.
		return
	}
	for _, loc := range s.area.Locations {
		fade := levelDistanceFade(s, loc.Level)
		r := s.rect.tileRect(loc.X, loc.Z)
		far := s.rect.tileRect(loc.X+loc.W-1, loc.Z+loc.H-1)
		box := rl.NewRectangle(r.X, r.Y, far.X+far.Width-r.X, far.Y+far.Height-r.Y)
		rl.DrawRectangleRec(box, fadeAlpha(locationFillCol, fade))
		rl.DrawRectangleLinesEx(box, 2, fadeAlpha(locationLineCol, fade))
		render.DrawTextWithShadow(font, "loc: "+locationLabel(loc),
			box.X+4, box.Y+3, editorFontHint, fadeAlpha(locationLabelCol, fade))
	}
}

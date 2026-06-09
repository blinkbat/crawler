package editor

import (
	"crawler/internal/app/core"
	"crawler/internal/app/input"
	"crawler/internal/app/render"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// dropdown.go is the editor's reusable "pick one of N" selector. It replaces
// the old per-kind add-key lists (a wall of mnemonic letters — R add Rat, B
// add Bat, … — that had to dodge the modal's own control keys and was guarded
// by an init collision-assert). Instead the pack/chest editors open ONE
// dropdown and the author picks from a scrollable list driven by the universal
// keys: Up/Down move, Enter chooses, Esc cancels — plus the mouse (click a row,
// click outside to dismiss, wheel to scroll). Adding a new enemy/item is now
// zero editor work: the dropdown lists whatever core.EnemyKinds / core.AllItems
// return, so there's no key to assign and nothing to collide.
//
// The widget itself is content-agnostic: a modal opens it for an `owner`, and
// the per-owner option LABELS (dropdownOptions) and choose ACTION
// (dropdownChoose) live with that modal. Open it for any future selector
// rather than inventing another key list.

// dropdownOwner identifies which modal opened the single dropdown slot on
// State, so it knows whose options to show and whose choose action to run.
type dropdownOwner int

const (
	ddNone     dropdownOwner = iota
	ddPackAdd                // pack editor: pick a builtin enemy kind or a custom enemy to add
	ddChestAdd               // chest editor: pick an item kind to add
)

// dropdownState is the editor's open-dropdown slot (one at a time). owner ==
// ddNone means closed. cursor is the highlighted option; anchor is the button
// the list drops from (the list grows UPWARD from it, since the add buttons sit
// at the bottom of their modal). The visible window is derived from cursor via
// scrollWindow at layout time, so draw and click hit-test never drift.
type dropdownState struct {
	owner  dropdownOwner
	cursor int
	anchor rl.Rectangle
}

const (
	dropdownRowH     = float32(24)
	dropdownMaxRows  = 9 // longer lists scroll rather than running off-screen
	dropdownMinWidth = float32(170)
	dropdownPad      = float32(6)
)

func (s *State) dropdownOpen() bool { return s.dropdown.owner != ddNone }

// openDropdown arms the dropdown for owner, dropping from anchor. Resets the
// analog-stick edge memory so a stick held at open doesn't fire a phantom
// nav on the first frame (mirrors battle's sequence-bar arming).
func openDropdown(s *State, owner dropdownOwner, anchor rl.Rectangle) {
	s.dropdown = dropdownState{owner: owner, anchor: anchor}
	input.ResetStickEdges()
}

func closeDropdown(s *State) { s.dropdown = dropdownState{} }

// dropdownEntry is one selectable row: its display label paired with the action
// that runs when it's chosen. Building a single ordered slice of these — rather
// than a parallel label list plus an index→action switch that have to agree on
// ordering — keeps the two from drifting (the same single-source discipline as
// the modalCmd{label, run} the entity-edit buttons use). dropdownOptions (draw)
// and dropdownChoose (select) both derive from this one list.
type dropdownEntry struct {
	label string
	apply func(*State)
}

// dropdownEntries builds the open dropdown's ordered rows for its owner.
func dropdownEntries(s *State) []dropdownEntry {
	switch s.dropdown.owner {
	case ddPackAdd:
		return packAddEntries(s)
	case ddChestAdd:
		return chestAddEntries(s)
	}
	return nil
}

// dropdownOptions returns just the row labels, in selection order, for layout
// and draw.
func dropdownOptions(s *State) []string {
	entries := dropdownEntries(s)
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.label
	}
	return out
}

// dropdownChoose runs the chosen row's action. idx is re-validated against the
// freshly-built entry list (the option set can change between frames as custom
// enemies are added/removed).
func dropdownChoose(s *State, idx int) {
	entries := dropdownEntries(s)
	if idx < 0 || idx >= len(entries) {
		return
	}
	entries[idx].apply(s)
}

// --- Pack-add entries: builtin enemy kinds, then this map's custom enemies ---

func packAddEntries(s *State) []dropdownEntry {
	defs := core.EnemyKinds()
	out := make([]dropdownEntry, 0, len(defs)+len(s.area.CustomEnemies))
	for _, def := range defs {
		kind := def.Kind
		out = append(out, dropdownEntry{
			label: def.SingularName,
			apply: func(s *State) { packAddMember(s, func(p *core.PackSpawn) { core.AppendBuiltinPackMember(p, kind) }) },
		})
	}
	for _, ce := range s.area.CustomEnemies {
		ce := ce
		out = append(out, dropdownEntry{
			label: ce.Name + " (custom)",
			apply: func(s *State) { packAddMember(s, func(p *core.PackSpawn) { core.AppendCustomPackMember(p, ce) }) },
		})
	}
	return out
}

// packAddMember appends to the edited pack via add, then selects the new member
// and marks dirty — the shared tail every pack-add entry runs (validate index,
// undo, append, cursor, dirty).
func packAddMember(s *State, add func(*core.PackSpawn)) {
	if s.modalPackIdx < 0 || s.modalPackIdx >= len(s.area.PackSpawns) {
		return
	}
	pack := &s.area.PackSpawns[s.modalPackIdx]
	pushUndo(s)
	add(pack)
	s.modalCursor = len(pack.Members) - 1
	s.dirty = true
}

// --- Chest-add entries: every registered item kind ---

func chestAddEntries(s *State) []dropdownEntry {
	defs := core.AllItems()
	out := make([]dropdownEntry, 0, len(defs))
	for _, def := range defs {
		kind := def.Kind
		out = append(out, dropdownEntry{
			label: def.Name,
			apply: func(s *State) {
				if s.modalChestIdx < 0 || s.modalChestIdx >= len(s.area.ChestSpawns) {
					return
				}
				pushUndo(s)
				chest := &s.area.ChestSpawns[s.modalChestIdx]
				chest.Items = append(chest.Items, kind)
				s.modalCursor = len(chest.Items) - 1
				s.dirty = true
			},
		})
	}
	return out
}

// --- Geometry (single source shared by update hit-test + draw) ---

type dropdownLayout struct {
	panel  rl.Rectangle
	topRow int            // first visible option index (scroll window)
	rows   []rl.Rectangle // one rect per visible row, in order
}

// computeDropdownLayout derives the panel rect and the visible row rects from
// the anchor, the option count, and the cursor. Deterministic, so the update
// (click hit-test) and the draw agree without storing layout on State — the
// same single-source discipline the modal button helpers use.
func computeDropdownLayout(s *State, labels []string) dropdownLayout {
	n := len(labels)
	visible := n
	if visible > dropdownMaxRows {
		visible = dropdownMaxRows
	}
	if visible < 1 {
		visible = 1
	}

	w := dropdownMinWidth
	if s.dropdown.anchor.Width > w {
		w = s.dropdown.anchor.Width
	}
	for _, l := range labels {
		if lw := approxTextWidth(l, editorFontBody) + 2*dropdownPad + 12; lw > w {
			w = lw
		}
	}

	sw, _ := render.ScreenSizeF()
	if w > sw-8 {
		w = sw - 8
	}
	h := float32(visible)*dropdownRowH + 2*dropdownPad

	x := s.dropdown.anchor.X
	if x+w > sw-4 {
		x = sw - 4 - w
	}
	if x < 4 {
		x = 4
	}
	// Grow UPWARD from just above the anchor button (the add buttons live at
	// the bottom of their modal). Clamp to the top edge if the list is taller
	// than the space above; it then overlaps the modal body, which is fine —
	// the dropdown is drawn last, on top.
	y := s.dropdown.anchor.Y - 4 - h
	if y < 4 {
		y = 4
	}

	top, _ := scrollWindow(s.dropdown.cursor, n, visible)
	rows := make([]rl.Rectangle, visible)
	for i := 0; i < visible; i++ {
		rows[i] = rl.NewRectangle(x+dropdownPad, y+dropdownPad+float32(i)*dropdownRowH,
			w-2*dropdownPad, dropdownRowH)
	}
	return dropdownLayout{panel: rl.NewRectangle(x, y, w, h), topRow: top, rows: rows}
}

// updateDropdown handles one frame of the open dropdown and returns true while
// it owns input (so the modal behind it stays inert). Universal keys only:
// Up/Down move, Enter chooses, Esc cancels; mouse clicks a row / dismisses on
// an outside click / scrolls with the wheel.
func updateDropdown(s *State) bool {
	if !s.dropdownOpen() {
		return false
	}
	labels := dropdownOptions(s)
	if len(labels) == 0 {
		closeDropdown(s)
		return true
	}
	s.dropdown.cursor = core.Clamp(s.dropdown.cursor, 0, len(labels)-1)
	lay := computeDropdownLayout(s, labels)
	mp := rl.GetMousePosition()

	if w := rl.GetMouseWheelMove(); w != 0 && pointIn(mp, lay.panel) {
		s.dropdown.cursor = core.Clamp(s.dropdown.cursor-int(w), 0, len(labels)-1)
	}

	if rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		for i, rr := range lay.rows {
			if pointIn(mp, rr) {
				dropdownChoose(s, lay.topRow+i)
				closeDropdown(s)
				return true
			}
		}
		if !pointIn(mp, lay.panel) {
			closeDropdown(s) // click outside dismisses
		}
		return true
	}

	if editorCancelPressed() {
		closeDropdown(s)
		return true
	}
	s.dropdown.cursor = input.CursorUpDown(s.dropdown.cursor, len(labels))
	if editorCommitPressed() {
		dropdownChoose(s, s.dropdown.cursor)
		closeDropdown(s)
	}
	return true
}

// drawDropdown paints the open dropdown on top of its modal. Called at the end
// of the owning modal's draw so the list sits above the card.
func drawDropdown(s *State, font rl.Font, theme render.Theme) {
	if !s.dropdownOpen() {
		return
	}
	labels := dropdownOptions(s)
	if len(labels) == 0 {
		return
	}
	lay := computeDropdownLayout(s, labels)
	render.DrawCard(int32(lay.panel.X), int32(lay.panel.Y), int32(lay.panel.Width), int32(lay.panel.Height),
		theme.SurfacePrimary, theme.BorderSoft, theme.BorderActive)

	mp := rl.GetMousePosition()
	for i, rr := range lay.rows {
		idx := lay.topRow + i
		col := theme.TextMuted
		if idx == s.dropdown.cursor {
			rl.DrawRectangleRec(rr, bgActive)
			col = theme.TextPrimary
		} else if pointIn(mp, rr) {
			rl.DrawRectangleRec(rr, bgRowHover)
		}
		render.DrawTextWithShadow(font, labels[idx], rr.X+6, rr.Y+3, editorFontBody, col)
	}

	// ▲/▼ "more" affordances when the list is scrolled.
	if lay.topRow > 0 {
		rl.DrawTextEx(font, "▲", rl.NewVector2(lay.panel.X+lay.panel.Width-16, lay.panel.Y+2), editorFontHint, 1, theme.TextHint)
	}
	if lay.topRow+len(lay.rows) < len(labels) {
		rl.DrawTextEx(font, "▼", rl.NewVector2(lay.panel.X+lay.panel.Width-16, lay.panel.Y+lay.panel.Height-16), editorFontHint, 1, theme.TextHint)
	}
}

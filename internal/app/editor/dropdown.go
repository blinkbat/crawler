package editor

import (
	"crawler/internal/app/core"
	"crawler/internal/app/input"
	"crawler/internal/app/render"
	"fmt"

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
// The widget itself is content-agnostic: an owner opens it, and the per-owner
// rows (dropdownEntries → []dropdownEntry, each carrying its own label + apply
// action) live with that owner. Open it for any future selector rather than
// inventing another key list. The menu bar (menus.go) is also an owner (ddMenu),
// reusing the same list/scroll/keyboard machinery for pull-down menus.

// dropdownOwner identifies which modal opened the single dropdown slot on
// State, so it knows whose options to show and whose choose action to run.
type dropdownOwner int

const (
	ddNone                dropdownOwner = iota
	ddPackAdd                           // pack editor: pick a builtin enemy kind to add
	ddChestAdd                          // chest editor: pick an item kind to add
	ddPackAI                            // pack editor: pick the pack's AI mode (replaces the cycle button)
	ddFoeKind                           // foe visualizer: pick which enemy kind to tune (replaces < > arrows)
	ddMenu                              // menu bar: the open top-level menu (File / Edit / View / …); see menus.go
	ddDialogSpeaker                     // dialog node editor: pick the node's speaker
	ddDialogCondKind                    // condition editor: pick the condition kind
	ddDialogQuestStatus                 // condition editor: pick the required quest status (Active / Complete)
	ddDialogCondFoe                     // condition editor: pick the foe kind for a foeKilled condition
	ddDialogTriggerKind                 // trigger editor: pick the trigger kind (enter-tile / foe-killed)
	ddDialogTriggerDialog               // trigger editor: pick which dialog the trigger starts
	ddDialogTriggerFoe                  // trigger editor: pick the foe kind for a foeKilled trigger
	ddDialogActionKind                  // action editor: pick the end-action (none / start / complete quest / event)
	ddLayer                             // top-bar layer picker: pick the active layer; each row carries a hide/show eye
	ddFaceSkin                          // tile right-click: pick a cliff-face skin for one face (or all) of the tile

	dropdownOwnerCount // sentinel — count of owners; keep last. Every owner in
	// (ddNone, dropdownOwnerCount) must register a dropdownEntryBuilders entry.
)

// dropdownState is the editor's open-dropdown slot (one at a time). owner ==
// ddNone means closed. cursor is the highlighted option; anchor is the button
// the list drops from (the list grows UPWARD from it, since the add buttons sit
// at the bottom of their modal). The visible window is derived from cursor via
// scrollWindow at layout time, so draw and click hit-test never drift.
type dropdownState struct {
	owner    dropdownOwner
	cursor   int
	anchor   rl.Rectangle
	growDown bool // true = list drops BELOW the anchor (top-anchored pickers); false = grows UP (bottom-of-modal add buttons)
	menu     int  // when owner == ddMenu: which editorMenus group is open (index)
}

const (
	dropdownRowH     = float32(24)
	dropdownMaxRows  = 9 // longer lists scroll rather than running off-screen
	dropdownMinWidth = float32(170)
	dropdownPad      = float32(6)
	// dropdownMarkerW is the width of the left gutter reserved for a row's
	// hide/show eye or ✓ toggle marker (0 when no row in the list has one).
	// Referenced by the layout (reservation), the click hit-test, and the draw.
	dropdownMarkerW = float32(16)
	// dropdownEyeGutterSlop widens the eye's click hit-zone a few px past the
	// marker gutter so a click just right of the glyph still toggles visibility
	// rather than selecting the row.
	dropdownEyeGutterSlop = float32(4)
)

// dropdownArrowSuffix is the "opens a picker" affordance appended to a button's
// label whose click opens a dropdown. Kept in one place so the glyph can't
// drift (▼ vs ▾ vs a different spacing) across the ~dozen picker buttons.
const dropdownArrowSuffix = "  ▼"

func (s *State) dropdownOpen() bool { return s.dropdown.owner != ddNone }

// openDropdown arms the dropdown for owner, dropping from anchor. Resets the
// analog-stick edge memory so a stick held at open doesn't fire a phantom
// nav on the first frame (mirrors battle's sequence-bar arming).
func openDropdown(s *State, owner dropdownOwner, anchor rl.Rectangle) {
	s.dropdown = dropdownState{owner: owner, anchor: anchor}
	input.ResetStickEdges()
}

// openDropdownBelow is openDropdown for a TOP-anchored picker (a header/field
// near the top of a modal) — the list drops downward from the anchor instead of
// growing up over the modal body. Used by the foe-kind and custom-enemy base
// pickers; the bottom-of-modal add buttons keep openDropdown (upward).
func openDropdownBelow(s *State, owner dropdownOwner, anchor rl.Rectangle) {
	s.dropdown = dropdownState{owner: owner, anchor: anchor, growDown: true}
	input.ResetStickEdges()
}

func closeDropdown(s *State) { s.dropdown = dropdownState{} }

// dropdownEntry is one selectable row: its display label paired with the action
// that runs when it's chosen. Building a single ordered slice of these — rather
// than a parallel label list plus an index→action switch that have to agree on
// ordering — keeps the two from drifting (the same single-source discipline as
// the modalCmd{label, run} the entity-edit buttons use). drawDropdown and
// dropdownChoose both derive from this one list.
//
// The first two fields cover every picker (pack-add, chest-add, …); the rest are
// OPTIONAL menu-row decoration (see menus.go) that plain pickers leave zero, so
// they render exactly as before:
//   - hotkey:  a right-aligned accelerator hint ("Ctrl+S"); "" = none.
//   - desc:    a one-line explanation shown beneath an open menu; "" = none.
//   - enabled: nil = always; else a disabled (grayed, unselectable) row when false.
//   - active:  nil = never; else a ✓-marked row when true (for toggles).
type dropdownEntry struct {
	label   string
	apply   func(*State)
	hotkey  string
	desc    string
	enabled func(*State) bool
	active  func(*State) bool
	// toggle/toggleOn give a row a clickable visibility EYE in the left gutter
	// (the layer picker's hide/show): a click on the eye runs toggle and LEAVES
	// the list open; a click on the rest of the row still selects via apply. nil =
	// no eye. toggleOn reports the eye's open (visible) state. A row uses either
	// the eye (toggle) or the ✓ (active) gutter marker, not both.
	toggle   func(*State)
	toggleOn func(*State) bool
}

// disabledIn reports whether this entry is a disabled row (enabled set and false).
func (e dropdownEntry) disabledIn(s *State) bool { return e.enabled != nil && !e.enabled(s) }

// dropdownEntryBuilders maps each dropdown owner to the function that builds
// its ordered rows. A map (asserted complete at init below) rather than a
// switch with a silent default, so a new owner that forgets its builder is a
// startup panic — not a dropdown that opens empty and dead. ddNone is the
// "closed" sentinel and intentionally has no builder.
var dropdownEntryBuilders = map[dropdownOwner]func(*State) []dropdownEntry{
	ddPackAdd:             packAddEntries,
	ddChestAdd:            chestAddEntries,
	ddPackAI:              packAIEntries,
	ddFoeKind:             foeKindEntries,
	ddMenu:                menuEntries,
	ddDialogSpeaker:       dialogSpeakerEntries,
	ddDialogCondKind:      dialogCondKindEntries,
	ddDialogQuestStatus:   dialogQuestStatusEntries,
	ddDialogCondFoe:       dialogCondFoeEntries,
	ddDialogTriggerKind:   dialogTriggerKindEntries,
	ddDialogTriggerDialog: dialogTriggerDialogEntries,
	ddDialogTriggerFoe:    dialogTriggerFoeEntries,
	ddDialogActionKind:    dialogActionKindEntries,
	ddLayer:               layerSelectEntries,
	ddFaceSkin:            faceSkinEntries,
}

// faceSkinEntries builds the tile face-skin picker (opened by a "Set … face"
// context row). Lists the FaceSkins roster; for a single direction it also
// offers "Default" to clear that face's override back to the tile's base skin.
// The target tile + direction live on State (faceTarget*), set when the row
// opened the dropdown.
func faceSkinEntries(s *State) []dropdownEntry {
	out := make([]dropdownEntry, 0, len(core.FaceSkins)+1)
	if s.faceTargetDir >= 0 {
		out = append(out, dropdownEntry{label: "Default (base skin)", apply: func(s *State) { applyFaceSkin(s, core.PropLevelAuto) }})
	}
	for _, sk := range core.FaceSkins {
		sk := sk
		out = append(out, dropdownEntry{label: sk.Name, apply: func(s *State) { applyFaceSkin(s, sk.Char) }})
	}
	return out
}

// applyFaceSkin commits the picked skin to the remembered target: the tile's
// base skin (Walls) for "all faces" (dir < 0), else that one face's override.
func applyFaceSkin(s *State, skin byte) {
	x, z, d := s.faceTargetX, s.faceTargetZ, s.faceTargetDir
	if !s.area.InBounds(x, z) {
		return
	}
	pushUndo(s)
	if d < 0 {
		if skin == core.PropLevelAuto {
			skin = core.TileRock
		}
		setLayerCell(&s.area.Walls, x, z, skin)
	} else {
		s.area.SetFaceDir(x, z, d, skin)
	}
	s.dirty = true
}

// layerSelectEntries builds the top-bar layer picker: one row per SELECTABLE
// editor layer (LayerWalls/"Faces" is excluded — faces are set via the
// right-click "Set wall faces…" modal, not a paint layer), label = the layer
// name, selecting sets the active layer. Each row carries a hide/show eye in
// the left gutter (clicking it toggles that layer's visibility and keeps the
// list open), so the per-layer visibility the tab strip used to surface lives
// here now.
func layerSelectEntries(s *State) []dropdownEntry {
	out := make([]dropdownEntry, 0, len(selectableLayers))
	for _, l := range selectableLayers {
		l := l
		out = append(out, dropdownEntry{
			label:    layerName(l),
			apply:    func(s *State) { s.layer = l },
			toggle:   func(s *State) { toggleLayerVisibility(s, int(l), false) },
			toggleOn: func(s *State) bool { return !s.layerHidden[l] },
		})
	}
	return out
}

func init() {
	for owner := ddNone + 1; owner < dropdownOwnerCount; owner++ {
		if dropdownEntryBuilders[owner] == nil {
			panic(fmt.Sprintf("editor: dropdownOwner %d has no dropdownEntryBuilders entry — register its row builder", int(owner)))
		}
	}
}

// dropdownEntries builds the open dropdown's ordered rows for its owner.
func dropdownEntries(s *State) []dropdownEntry {
	if build := dropdownEntryBuilders[s.dropdown.owner]; build != nil {
		return build(s)
	}
	return nil
}

// enemyKindEntries builds one dropdown row per registered enemy kind (label =
// its singular name), each running `apply(s, kind)` when chosen. The shared base
// for every "pick an enemy kind" dropdown — the foe-kind picker and the pack-add
// list — so the EnemyKinds() walk + label rule live once.
func enemyKindEntries(apply func(*State, core.EnemyKind)) []dropdownEntry {
	defs := core.EnemyKinds()
	out := make([]dropdownEntry, 0, len(defs))
	for _, def := range defs {
		kind := def.Kind
		out = append(out, dropdownEntry{
			label: def.SingularName,
			apply: func(s *State) { apply(s, kind) },
		})
	}
	return out
}

// foeKindEntries lists every enemy kind for the Foe Visualizer's kind picker —
// the dropdown replacement for the < > arrows. Choosing re-seeds the working
// visual from that kind.
func foeKindEntries(s *State) []dropdownEntry {
	return enemyKindEntries(func(s *State, kind core.EnemyKind) {
		s.foeKind = kind
		seedFoeVisual(s)
		// Match cycleFoe: re-seed the Asset-tab cursor + rebuild the live preview
		// for the newly-picked foe (the dropdown is the < > arrows' twin).
		enterAssetEditing(s)
	})
}

// nameSpanBetween returns the clickable rect of the value/name shown between a
// pair of < > stepper arrows — the hit-target that opens the kind dropdown in
// the foe visualizer (so the geometry rule isn't copied per modal).
func nameSpanBetween(prev, next rl.Rectangle) rl.Rectangle {
	x := prev.X + prev.Width
	return rl.NewRectangle(x, prev.Y, next.X-x, prev.Height)
}

// packAIEntries lists every pack-AI mode for the pack editor's "AI" picker —
// the dropdown replacement for the old one-at-a-time cycle button, so the author
// sees all modes and jumps straight to one.
func packAIEntries(s *State) []dropdownEntry {
	if s.modalPackIdx < 0 || s.modalPackIdx >= len(s.area.PackSpawns) {
		return nil
	}
	out := make([]dropdownEntry, 0, core.PackAICount)
	for mode := core.PackAI(0); int(mode) < core.PackAICount; mode++ {
		mode := mode
		out = append(out, dropdownEntry{
			label: core.PackAILabel(mode),
			apply: func(s *State) {
				if s.modalPackIdx < 0 || s.modalPackIdx >= len(s.area.PackSpawns) {
					return
				}
				pushUndo(s)
				s.area.PackSpawns[s.modalPackIdx].AI = mode
				s.dirty = true
				s.flash("Pack AI: " + core.PackAILabel(mode))
			},
		})
	}
	return out
}

// chooseDropdownEntry runs the chosen row's action and closes the dropdown. A
// disabled menu row (enabled set and false) is a no-op that LEAVES the menu open
// so the author can pick another row. idx is validated against the passed entry
// list (the option set can change between frames as custom enemies are added).
func chooseDropdownEntry(s *State, entries []dropdownEntry, idx int) {
	if idx < 0 || idx >= len(entries) {
		return
	}
	if entries[idx].disabledIn(s) {
		return
	}
	entries[idx].apply(s)
	closeDropdown(s)
}

// --- Pack-add entries: builtin enemy kinds, then this map's custom enemies ---

func packAddEntries(s *State) []dropdownEntry {
	return enemyKindEntries(func(s *State, kind core.EnemyKind) {
		packAddMember(s, func(p *core.PackSpawn) { core.AppendBuiltinPackMember(p, kind) })
	})
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
	panel   rl.Rectangle
	topRow  int            // first visible option index (scroll window)
	rows    []rl.Rectangle // one rect per visible row, in order
	markerW float32        // left ✓ gutter width (menus with toggles); 0 for plain pickers
}

// computeDropdownLayout derives the panel rect and the visible row rects from
// the anchor, the option count, and the cursor. Deterministic, so the update
// (click hit-test) and the draw agree without storing layout on State — the
// same single-source discipline the modal button helpers use.
func computeDropdownLayout(s *State, entries []dropdownEntry) dropdownLayout {
	n := len(entries)
	if n == 0 {
		// No entries → no rows. Forcing visible up to 1 here would build a row
		// rect for a zero-length slice; a hit-test/draw loop indexing entries[0]
		// would then panic. Live callers guard len==0, but stay safe in isolation.
		return dropdownLayout{panel: s.dropdown.anchor}
	}
	visible := n
	if visible > dropdownMaxRows {
		visible = dropdownMaxRows
	}
	if visible < 1 {
		visible = 1
	}

	// Reserve a ✓ gutter only when some row is a toggle (active != nil), so plain
	// pickers stay exactly as wide as before.
	markerW := float32(0)
	for _, e := range entries {
		if e.active != nil || e.toggle != nil {
			markerW = dropdownMarkerW
			break
		}
	}

	w := dropdownMinWidth
	if s.dropdown.anchor.Width > w {
		w = s.dropdown.anchor.Width
	}
	for _, e := range entries {
		measure := e.label
		if e.hotkey != "" {
			measure += "    " + e.hotkey // reserve room for the right-aligned accelerator
		}
		if lw := approxTextWidth(measure, editorFontBody) + 2*dropdownPad + 12 + markerW; lw > w {
			w = lw
		}
	}

	sw, sh := render.ScreenSizeF()
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
	// Grow UPWARD from just above the anchor (add buttons live at the bottom of
	// their modal) or DOWNWARD from below it (top-anchored pickers). Clamp to the
	// screen edge if the list overflows; it then overlaps the modal body, which is
	// fine — the dropdown is drawn last, on top.
	var y float32
	if s.dropdown.growDown {
		y = s.dropdown.anchor.Y + s.dropdown.anchor.Height + 4
		if y+h > sh-4 {
			y = sh - 4 - h
		}
	} else {
		y = s.dropdown.anchor.Y - 4 - h
	}
	if y < 4 {
		y = 4
	}

	top, _ := scrollWindow(s.dropdown.cursor, n, visible)
	rows := make([]rl.Rectangle, visible)
	for i := 0; i < visible; i++ {
		rows[i] = rl.NewRectangle(x+dropdownPad, y+dropdownPad+float32(i)*dropdownRowH,
			w-2*dropdownPad, dropdownRowH)
	}
	return dropdownLayout{panel: rl.NewRectangle(x, y, w, h), topRow: top, rows: rows, markerW: markerW}
}

// updateDropdown handles one frame of the open dropdown and returns true while
// it owns input (so the modal behind it stays inert). Universal keys only:
// Up/Down move, Enter chooses, Esc cancels; mouse clicks a row / dismisses on
// an outside click / scrolls with the wheel.
func updateDropdown(s *State) bool {
	if !s.dropdownOpen() {
		return false
	}
	entries := dropdownEntries(s)
	if len(entries) == 0 {
		closeDropdown(s)
		return true
	}
	s.dropdown.cursor = core.Clamp(s.dropdown.cursor, 0, len(entries)-1)
	lay := computeDropdownLayout(s, entries)
	mp := rl.GetMousePosition()

	wheel := rl.GetMouseWheelMove()
	if wheel != 0 && pointIn(mp, lay.panel) {
		s.dropdown.cursor = core.Clamp(s.dropdown.cursor-int(wheel), 0, len(entries)-1)
	}

	// Mouse hover drives the cursor so the per-row description caption (and a
	// keyboard Enter) follow the pointer. Without this the desc stuck to the last
	// KEYBOARD row, so hovering menu items showed the wrong/no tooltip — the
	// "dropdown tooltips don't work" report. Skipped on a wheel-scroll frame:
	// otherwise the row the pointer rests on immediately overrides the scroll,
	// so the wheel appears dead whenever the cursor overlaps a row.
	if wheel == 0 {
		for i, rr := range lay.rows {
			if pointIn(mp, rr) {
				s.dropdown.cursor = lay.topRow + i
				break
			}
		}
	}

	if rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		for i, rr := range lay.rows {
			if pointIn(mp, rr) {
				idx := lay.topRow + i
				// A click on the eye gutter toggles that row's visibility and keeps
				// the list open (the layer picker's hide/show); the rest selects.
				if lay.markerW > 0 && idx >= 0 && idx < len(entries) &&
					entries[idx].toggle != nil && mp.X <= rr.X+lay.markerW+dropdownEyeGutterSlop {
					entries[idx].toggle(s)
					return true
				}
				chooseDropdownEntry(s, entries, idx) // disabled rows no-op, keep open
				return true
			}
		}
		if !pointIn(mp, lay.panel) {
			// Menu bar: a click on a DIFFERENT top-level label switches menus so
			// the author needn't click twice (close, then open). Any other outside
			// click dismisses.
			if s.dropdown.owner == ddMenu {
				if hit := buttonStripHit(menuBarBtns, 6, topbarH-12, mp); hit >= 0 && hit != s.dropdown.menu {
					openMenu(s, hit)
					return true
				}
			}
			closeDropdown(s)
		}
		return true
	}

	if editorCancelPressed() {
		closeDropdown(s)
		return true
	}
	s.dropdown.cursor = input.CursorUpDown(s.dropdown.cursor, len(entries))
	if editorCommitPressed() {
		chooseDropdownEntry(s, entries, s.dropdown.cursor)
	}
	return true
}

// drawDropdown paints the open dropdown on top of its modal. Called at the end
// of the owning modal's draw so the list sits above the card.
func drawDropdown(s *State, font rl.Font, theme render.Theme) {
	if !s.dropdownOpen() {
		return
	}
	entries := dropdownEntries(s)
	if len(entries) == 0 {
		return
	}
	lay := computeDropdownLayout(s, entries)
	render.DrawCard(int32(lay.panel.X), int32(lay.panel.Y), int32(lay.panel.Width), int32(lay.panel.Height),
		theme.SurfacePrimary, theme.BorderSoft, theme.BorderActive)

	mp := rl.GetMousePosition()
	for i, rr := range lay.rows {
		idx := lay.topRow + i
		e := entries[idx]
		disabled := e.disabledIn(s)
		// Cursor highlight draws even on a disabled row so keyboard nav never
		// "loses" the cursor on (say) a grayed Undo; the text stays faded and
		// chooseDropdownEntry still no-ops it, so it reads as "selected but
		// unavailable." Hover highlight is suppressed on disabled rows.
		col := theme.TextMuted
		switch {
		case idx == s.dropdown.cursor:
			rl.DrawRectangleRec(rr, bgActive)
			if !disabled {
				col = theme.TextPrimary
			}
		case pointIn(mp, rr) && !disabled:
			rl.DrawRectangleRec(rr, bgRowHover)
		}
		if disabled {
			col = render.FadeColor(theme.TextMuted, 0.45)
		}
		// Left gutter marker: a hide/show EYE (layer picker) or a ✓ for an active
		// toggle (menus). A row uses one or the other.
		if lay.markerW > 0 && e.toggle != nil {
			drawLayerEye(rl.NewRectangle(rr.X+1, rr.Y+3, lay.markerW, rr.Height-6), e.toggleOn == nil || e.toggleOn(s), false)
		} else if lay.markerW > 0 && e.active != nil && e.active(s) {
			render.DrawTextWithShadow(font, "✓", rr.X+4, rr.Y+3, editorFontBody, theme.TextPrimary)
		}
		render.DrawTextWithShadow(font, e.label, rr.X+6+lay.markerW, rr.Y+3, editorFontBody, col)
		// Right-aligned accelerator hint (dim).
		if e.hotkey != "" {
			hw := render.MeasureRichText(font, e.hotkey, editorFontHint, 1).X
			render.DrawRichText(font, e.hotkey, rl.NewVector2(rr.X+rr.Width-hw-6, rr.Y+4), editorFontHint, 1, theme.TextHint)
		}
	}

	// ▲/▼ "more" affordances when the list is scrolled.
	if lay.topRow > 0 {
		render.DrawRichText(font, "▲", rl.NewVector2(lay.panel.X+lay.panel.Width-16, lay.panel.Y+2), editorFontHint, 1, theme.TextHint)
	}
	if lay.topRow+len(lay.rows) < len(entries) {
		render.DrawRichText(font, "▼", rl.NewVector2(lay.panel.X+lay.panel.Width-16, lay.panel.Y+lay.panel.Height-16), editorFontHint, 1, theme.TextHint)
	}

	// Menu rows carry a one-line explanation; show the cursored row's beneath the
	// panel so the author learns what each command does without leaving the menu.
	if s.dropdown.owner == ddMenu {
		if cur := s.dropdown.cursor; cur >= 0 && cur < len(entries) && entries[cur].desc != "" {
			drawMenuDesc(font, theme, lay.panel, entries[cur].desc)
		}
	}
}

// drawMenuDesc paints the open menu's cursored-row explanation as a slim caption
// just below the dropdown panel (clamped to the screen). Keeps the dropdown rows
// compact while still surfacing "what does this do" for every command.
func drawMenuDesc(font rl.Font, theme render.Theme, panel rl.Rectangle, desc string) {
	tw := render.MeasureRichText(font, desc, editorFontHint, 1).X
	w := tw + 2*dropdownPad
	sw, _ := render.ScreenSizeF()
	x := panel.X
	if x+w > sw-4 {
		x = sw - 4 - w
	}
	if x < 4 {
		x = 4
	}
	y := panel.Y + panel.Height + 12 // gap below the panel so the caption reads as separate
	h := editorFontHint + 2*dropdownPad
	render.DrawCard(int32(x), int32(y), int32(w), int32(h), theme.SurfacePrimary, theme.BorderSoft, theme.BorderSoft)
	render.DrawRichText(font, desc, rl.NewVector2(x+dropdownPad, y+dropdownPad), editorFontHint, 1, theme.TextPrimary)
}

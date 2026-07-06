package editor

import (
	"crawler/internal/app/core"
	"crawler/internal/app/core/mapfile"
	"crawler/internal/app/input"
	"crawler/internal/app/render"
	"fmt"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// dropdown.go is the editor's reusable "pick one of N" selector. Content-agnostic:
// an owner opens it, the per-owner rows (each carrying its own label + apply
// action) come from that owner's builder. Keys: Up/Down move, Enter chooses, Esc
// cancels; mouse clicks/scrolls. The menu bar (menus.go) is also an owner (ddMenu).

// dropdownOwner identifies which modal opened the single dropdown slot on State.
type dropdownOwner int

const (
	ddNone                  dropdownOwner = iota
	ddPackAdd                             // pack editor: pick a builtin enemy kind to add
	ddChestAdd                            // chest editor: pick an item kind to add
	ddPackAI                              // pack editor: pick the pack's AI mode (replaces the cycle button)
	ddFoeKind                             // foe visualizer: pick which enemy kind to tune (replaces < > arrows)
	ddMenu                                // menu bar: the open top-level menu (File / Edit / View / …); see menus.go
	ddDialogSpeaker                       // dialog node editor: pick the node's speaker
	ddDialogCondKind                      // condition editor: pick the condition kind
	ddDialogQuestStatus                   // condition editor: pick the required quest status (Active / Complete)
	ddDialogCondFoe                       // condition editor: pick the foe kind for a foeKilled condition
	ddTrigCondKind                        // trigger editor: pick the primary condition kind
	ddTrigActKind                         // trigger editor: pick the primary action kind
	ddDialogTriggerDialog                 // trigger editor: pick which dialog a Start-dialog action starts
	ddDialogTriggerFoe                    // trigger editor: pick the foe kind for a foeKilled condition
	ddDialogTriggerLocation               // trigger editor: pick the region for an atLocation condition
	ddTrigActFoe                          // trigger editor: pick the foe kind for a spawnFoe action
	ddTrigSwitchOp                        // trigger editor: pick the setSwitch action's operation
	ddTrigCounterOp                       // trigger editor: pick the setCounter action's operation
	ddWallFeatureKind                     // wall-feature editor: pick switch / bombable / secret
	ddDialogActionKind                    // action editor: pick the end-action (none / start / complete quest / event)
	ddLayer                               // top-bar layer picker: pick the active layer; each row carries a hide/show eye
	ddFaceSkin                            // tile right-click: pick a cliff-face skin for one face (or all) of the tile
	ddContext                             // grid right-click menu: edit/delete spawns, start facing, regions, erase (see context.go)
	ddDoorFacing                          // door editor: pick the door's facing (replaces the N/E/S/W button/key row)
	ddDoorStyle                           // door editor: pick the door's style (replaces the 1/2/3 button/key row)
	ddDoorTargetMap                       // door editor: pick the target map from all .map ids on disk (+ self)
	ddDoorTargetDoor                      // door editor: pick the target door from the target map's authored doors
	ddSoundAssign                         // sound editor: pick the user sound bound to a built-in cue (replaces the < > steppers)

	dropdownOwnerCount // sentinel; keep last. Every owner above ddNone needs a dropdownEntryBuilders entry.
)

// dropdownState is the open-dropdown slot (one at a time); owner==ddNone is closed.
// anchor is the button the list drops from. Visible window is derived from cursor
// via scrollWindow at layout time, so draw and hit-test never drift.
type dropdownState struct {
	owner    dropdownOwner
	cursor   int
	anchor   rl.Rectangle
	growDown bool       // true = drops BELOW the anchor; false = grows UP
	menu     int        // owner==ddMenu: open editorMenus group index
	at       rl.Vector2 // atPoint: the free-floating top-left (right-click menu)
	atPoint  bool       // true = panel top-left sits at `at` (screen-clamped), not anchor-relative
	rowH     float32    // per-owner row-height override; 0 = dropdownRowH (context menu wants taller rows)
	// filter is the type-to-filter query for a filterable picker (long enemy/item/
	// dialog lists); empty = show all. Reset when the dropdown opens/closes.
	filter string
}

// filterableDropdown reports whether owner is a long "pick one of many" list that
// supports type-to-filter (short fixed lists like AI mode / quest status don't).
func filterableDropdown(o dropdownOwner) bool {
	switch o {
	case ddPackAdd, ddChestAdd, ddFoeKind, ddDialogCondFoe, ddDialogTriggerFoe,
		ddDialogTriggerDialog, ddDialogSpeaker, ddDialogTriggerLocation, ddSoundAssign,
		ddDoorTargetMap, ddDoorTargetDoor:
		return true
	}
	return false
}

// visibleDropdownEntries is dropdownEntries filtered by the live type-to-filter
// query (case-insensitive label substring) for filterable owners. The single seam
// both update and draw read, so cursor indices can't drift from what's shown.
func visibleDropdownEntries(s *State) []dropdownEntry {
	entries := dropdownEntries(s)
	f := strings.TrimSpace(s.dropdown.filter)
	if f == "" || !filterableDropdown(s.dropdown.owner) {
		return entries
	}
	lf := strings.ToLower(f)
	out := make([]dropdownEntry, 0, len(entries))
	for _, e := range entries {
		if strings.Contains(strings.ToLower(e.label), lf) {
			out = append(out, e)
		}
	}
	return out
}

// pumpDropdownFilter appends typed characters / applies backspace to the filterable
// dropdown's query, resetting the cursor so the top match is selected. No-op for a
// non-filterable owner.
func pumpDropdownFilter(s *State) {
	if !filterableDropdown(s.dropdown.owner) {
		return
	}
	for {
		r := rl.GetCharPressed()
		if r == 0 {
			break
		}
		if r >= 32 && r < 127 {
			s.dropdown.filter += string(rune(r))
			s.dropdown.cursor = 0
		}
	}
	if rl.IsKeyPressed(rl.KeyBackspace) && len(s.dropdown.filter) > 0 {
		s.dropdown.filter = s.dropdown.filter[:len(s.dropdown.filter)-1]
		s.dropdown.cursor = 0
	}
}

const (
	dropdownRowH     = float32(24)
	dropdownMaxRows  = 9 // longer lists scroll
	dropdownMinWidth = float32(170)
	dropdownPad      = float32(6)
	// dropdownMarkerW: left-gutter width for a row's eye/✓ marker (0 when none).
	dropdownMarkerW = float32(16)
	// dropdownEyeGutterSlop widens the eye's click hit-zone past the gutter.
	dropdownEyeGutterSlop = float32(4)
)

// dropdownArrowSuffix is the "opens a picker" affordance appended to a picker
// button's label; one source so the glyph can't drift across picker buttons.
const dropdownArrowSuffix = "  ▼"

func (s *State) dropdownOpen() bool { return s.dropdown.owner != ddNone }

// openDropdown arms the dropdown for owner, dropping from anchor. Resets stick
// edge memory so a stick held at open doesn't fire a phantom nav on frame one.
func openDropdown(s *State, owner dropdownOwner, anchor rl.Rectangle) {
	s.dropdown = dropdownState{owner: owner, anchor: anchor}
	input.ResetStickEdges()
}

// openDropdownBelow is openDropdown for a TOP-anchored picker: the list drops
// DOWN from the anchor instead of growing up over the modal body.
func openDropdownBelow(s *State, owner dropdownOwner, anchor rl.Rectangle) {
	s.dropdown = dropdownState{owner: owner, anchor: anchor, growDown: true}
	input.ResetStickEdges()
}

func closeDropdown(s *State) { s.dropdown = dropdownState{} }

// openDropdownAt arms owner as a free-floating menu whose top-left sits at pt (the
// grid right-click context menu), with rowH-tall rows. Screen-clamped at layout.
func openDropdownAt(s *State, owner dropdownOwner, pt rl.Vector2, rowH float32) {
	s.dropdown = dropdownState{owner: owner, at: pt, atPoint: true, rowH: rowH}
	input.ResetStickEdges()
}

// openFieldDropdown clears any lingering text-field focus, then opens dd below anchor.
// Field rows always defocus before opening a picker (so Up/Down drive the list, not a
// stale field) — this keeps that pairing in one place so a call site can't forget it.
func openFieldDropdown(s *State, owner dropdownOwner, anchor rl.Rectangle) {
	s.focus = focusNone
	openDropdownBelow(s, owner, anchor)
}

// dropdownEntry is one selectable row: label + apply, in one ordered slice so
// draw and choose can't drift. The rest are optional menu-row decoration (zero
// for plain pickers): hotkey (accelerator hint), desc (menu explanation),
// enabled (nil=always, else grayed when false), active (nil=never, else ✓ when true).
type dropdownEntry struct {
	label   string
	apply   func(*State)
	hotkey  string
	desc    string
	enabled func(*State) bool
	active  func(*State) bool
	// toggle/toggleOn give a row a clickable visibility EYE: clicking it runs
	// toggle + keeps the list open (rest of the row selects). toggleOn = visible
	// state; nil = no eye. A row uses the eye OR the ✓ marker, not both.
	toggle   func(*State)
	toggleOn func(*State) bool
	// swatch, when opaque (A>0), draws a small color chip before the label — the
	// Layer picker color-codes its rows this way.
	swatch rl.Color
	// danger draws the row's label in BorderDanger (destructive actions — the
	// context menu's Delete rows).
	danger bool
}

// disabledIn reports whether this entry is a disabled row (enabled set and false).
func (e dropdownEntry) disabledIn(s *State) bool { return e.enabled != nil && !e.enabled(s) }

// dropdownEntryBuilders maps each owner to its row builder (asserted complete at init).
var dropdownEntryBuilders = map[dropdownOwner]func(*State) []dropdownEntry{
	ddPackAdd:               packAddEntries,
	ddChestAdd:              chestAddEntries,
	ddPackAI:                packAIEntries,
	ddFoeKind:               foeKindEntries,
	ddMenu:                  menuEntries,
	ddDialogSpeaker:         dialogSpeakerEntries,
	ddDialogCondKind:        dialogCondKindEntries,
	ddDialogQuestStatus:     dialogQuestStatusEntries,
	ddDialogCondFoe:         dialogCondFoeEntries,
	ddTrigCondKind:          trigCondKindEntries,
	ddTrigActKind:           trigActKindEntries,
	ddDialogTriggerDialog:   dialogTriggerDialogEntries,
	ddDialogTriggerFoe:      dialogTriggerFoeEntries,
	ddDialogTriggerLocation: dialogTriggerLocationEntries,
	ddTrigActFoe:            trigActFoeEntries,
	ddTrigSwitchOp:          trigSwitchOpEntries,
	ddTrigCounterOp:         trigCounterOpEntries,
	ddWallFeatureKind:       wallFeatureKindEntries,
	ddDialogActionKind:      dialogActionKindEntries,
	ddLayer:                 layerSelectEntries,
	ddFaceSkin:              faceSkinEntries,
	ddContext:               contextEntries,
	ddDoorFacing:            doorFacingEntries,
	ddDoorStyle:             doorStyleEntries,
	ddDoorTargetMap:         doorTargetMapEntries,
	ddDoorTargetDoor:        doorTargetDoorEntries,
	ddSoundAssign:           soundAssignEntries,
}

// doorFacingEntries / doorStyleEntries build the door editor's facing / style
// pickers (one row per core enum value; re-picking the current value is a no-op via
// setIfChanged). Target door lives on State (modalDoorIdx).
func doorFacingEntries(s *State) []dropdownEntry {
	if s.modalDoorIdx < 0 || s.modalDoorIdx >= len(s.area.DoorSpawns) {
		return nil
	}
	out := make([]dropdownEntry, 0, core.FacingCount)
	for f := 0; f < core.FacingCount; f++ {
		f := f
		name, _ := core.FacingName(f)
		out = append(out, dropdownEntry{
			label: name,
			apply: func(s *State) {
				if s.modalDoorIdx >= 0 && s.modalDoorIdx < len(s.area.DoorSpawns) {
					setIfChanged(s, &s.area.DoorSpawns[s.modalDoorIdx].Facing, f)
				}
			},
		})
	}
	return out
}

func doorStyleEntries(s *State) []dropdownEntry {
	if s.modalDoorIdx < 0 || s.modalDoorIdx >= len(s.area.DoorSpawns) {
		return nil
	}
	out := make([]dropdownEntry, 0, core.DoorStyleCount)
	for i := 0; i < int(core.DoorStyleCount); i++ {
		style := core.DoorStyle(i)
		out = append(out, dropdownEntry{
			label: core.DoorStyleLabel(style),
			apply: func(s *State) {
				if s.modalDoorIdx >= 0 && s.modalDoorIdx < len(s.area.DoorSpawns) {
					setIfChanged(s, &s.area.DoorSpawns[s.modalDoorIdx].Style, style)
				}
			},
		})
	}
	return out
}

// setDoorTarget writes the edited door's TargetMap (mapField) or TargetDoor
// (doorField) via the lazy-undo seam. Picking the current value is a no-op.
func setDoorTarget(s *State, field *string, val string) {
	if s.modalDoorIdx < 0 || s.modalDoorIdx >= len(s.area.DoorSpawns) {
		return
	}
	setIfChanged(s, field, val)
}

// openDoorTargetMapPicker loads every .map id on disk into doorPickMaps (once, at
// open) and opens the target-map dropdown. "self" heads the list; the current map
// is elided (it's what "self" means).
func openDoorTargetMapPicker(s *State, anchor rl.Rectangle) {
	paths, _ := mapfile.ListByModTime(core.MapsDir())
	selfID := ""
	if s.area.Path != "" {
		selfID = core.MapIDFromPath(s.area.Path)
	}
	maps := make([]string, 0, len(paths)+1)
	maps = append(maps, core.SelfMapToken) // "self" — a same-map portal
	for _, p := range paths {
		id := core.MapIDFromPath(p)
		if id == "" || id == selfID {
			continue
		}
		maps = append(maps, id)
	}
	s.doorPickMaps = maps
	openFieldDropdown(s, ddDoorTargetMap, anchor)
}

// openDoorTargetDoorPicker loads the target map's authored door names into
// doorPickDoors (once, at open) and opens the target-door dropdown. A self/current
// -map target reads the in-memory doors; a cross-map target loads that map from disk.
func openDoorTargetDoorPicker(s *State, anchor rl.Rectangle) {
	s.doorPickDoors = nil
	if s.modalDoorIdx >= 0 && s.modalDoorIdx < len(s.area.DoorSpawns) {
		target := s.area.DoorSpawns[s.modalDoorIdx].TargetMap
		var spawns []core.DoorSpawn
		if target == "" || core.IsSelfPortal(s.area, target) {
			spawns = s.area.DoorSpawns
		} else if dest, err := core.LoadArea(core.MapPath(target)); err == nil {
			spawns = dest.DoorSpawns
		}
		names := make([]string, 0, len(spawns))
		for _, d := range spawns {
			if d.Name != "" {
				names = append(names, d.Name)
			}
		}
		s.doorPickDoors = names
	}
	openFieldDropdown(s, ddDoorTargetDoor, anchor)
}

// doorTargetMapEntries builds the target-map picker from the cached map-id list
// (populated by openDoorTargetMapPicker so the builder never touches disk).
func doorTargetMapEntries(s *State) []dropdownEntry {
	if s.modalDoorIdx < 0 || s.modalDoorIdx >= len(s.area.DoorSpawns) {
		return nil
	}
	return fieldEntries(s.doorPickMaps, func(id string) string { return id },
		func(s *State, id string) {
			if s.modalDoorIdx < 0 || s.modalDoorIdx >= len(s.area.DoorSpawns) {
				return
			}
			setDoorTarget(s, &s.area.DoorSpawns[s.modalDoorIdx].TargetMap, id)
		})
}

// doorTargetDoorEntries builds the target-door picker from the cached name list
// (populated by openDoorTargetDoorPicker). Empty when the target map has no doors.
func doorTargetDoorEntries(s *State) []dropdownEntry {
	if s.modalDoorIdx < 0 || s.modalDoorIdx >= len(s.area.DoorSpawns) {
		return nil
	}
	return fieldEntries(s.doorPickDoors, func(name string) string { return name },
		func(s *State, name string) {
			if s.modalDoorIdx < 0 || s.modalDoorIdx >= len(s.area.DoorSpawns) {
				return
			}
			setDoorTarget(s, &s.area.DoorSpawns[s.modalDoorIdx].TargetDoor, name)
		})
}

// faceSkinEntries builds the tile face-skin picker. Lists the FaceSkins roster;
// for a single direction it also offers "Default" to clear that face's override.
// Target tile + direction live on State (faceTarget*).
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

// applyFaceSkin commits the picked skin: the tile's base skin (Walls) for "all
// faces" (dir < 0), else that one face's override.
func applyFaceSkin(s *State, skin byte) {
	x, z, d := s.faceTargetX, s.faceTargetZ, s.faceTargetDir
	if !s.area.InBounds(x, z) {
		return
	}
	// Bank undo / set dirty only when the edit actually changed something:
	// SetFaceDir normalizes a base-equal skin and can drop the override, so
	// re-picking the current skin must be a no-op (lazy-snapshot guard).
	before := core.CloneArea(s.area)
	if d < 0 {
		if skin == core.PropLevelAuto {
			skin = core.TileRock
		}
		setLayerCell(&s.area.Walls, x, z, skin)
	} else {
		s.area.SetFaceDir(x, z, d, skin)
	}
	if core.AreaContentEqual(before, s.area) {
		return
	}
	commitUndoSnapshot(s, before)
	s.dirty = true
}

// layerSelectEntries builds the top-bar layer picker: one row per selectable
// layer (selecting sets the active layer), each with a hide/show eye in the
// left gutter. Faces aren't a paint layer, so they're excluded.
func layerSelectEntries(s *State) []dropdownEntry {
	out := make([]dropdownEntry, 0, len(selectableLayers))
	for _, l := range selectableLayers {
		l := l
		out = append(out, dropdownEntry{
			label:    layerName(l),
			desc:     layerDescription(l),
			apply:    func(s *State) { s.layer = l },
			toggle:   func(s *State) { toggleLayerVisibility(s, int(l), false) },
			toggleOn: func(s *State) bool { return !s.layerHidden[l] },
			swatch:   layerAccent(l),
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

// dropdownEntries builds the open dropdown's rows for its owner.
func dropdownEntries(s *State) []dropdownEntry {
	if build := dropdownEntryBuilders[s.dropdown.owner]; build != nil {
		return build(s)
	}
	return nil
}

// fieldEntries builds one dropdown row per item: label(item) names it, choosing it runs
// apply(s, item). Collapses the hand-written "make+append {label, apply}" loops the dialog
// field pickers (speaker, quest-status, trigger-dialog, location) each repeated.
func fieldEntries[T any](items []T, label func(T) string, apply func(*State, T)) []dropdownEntry {
	out := make([]dropdownEntry, 0, len(items))
	for _, it := range items {
		it := it
		out = append(out, dropdownEntry{
			label: label(it),
			apply: func(s *State) { apply(s, it) },
		})
	}
	return out
}

// enemyKindEntries builds one row per registered enemy kind (label = singular
// name), each running apply(s, kind). Shared by every "pick an enemy kind" dropdown.
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

// itemKindEntries builds one row per registered item kind (label = item name),
// each running apply(s, kind). Symmetric with enemyKindEntries.
func itemKindEntries(apply func(*State, core.ItemKind)) []dropdownEntry {
	defs := core.AllItems()
	out := make([]dropdownEntry, 0, len(defs))
	for _, def := range defs {
		kind := def.Kind
		out = append(out, dropdownEntry{
			label: def.Name,
			apply: func(s *State) { apply(s, kind) },
		})
	}
	return out
}

// foeKindEntries lists every enemy kind for the Foe Visualizer's kind picker.
// Choosing re-seeds the working visual from that kind.
func foeKindEntries(s *State) []dropdownEntry {
	return enemyKindEntries(func(s *State, kind core.EnemyKind) {
		s.foeKind = kind
		seedFoeVisual(s)
		// Match cycleFoe: re-seed the Asset-tab cursor + rebuild the live preview.
		enterAssetEditing(s)
	})
}

// nameSpanBetween returns the clickable rect between a pair of < > stepper
// arrows — the hit-target that opens the kind dropdown.
func nameSpanBetween(prev, next rl.Rectangle) rl.Rectangle {
	x := prev.X + prev.Width
	return rl.NewRectangle(x, prev.Y, next.X-x, prev.Height)
}

// packAIEntries lists every pack-AI mode for the pack editor's "AI" picker.
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
				if s.area.PackSpawns[s.modalPackIdx].AI == mode {
					return // re-picking the current AI — no undo/dirty churn
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
// disabled row is a no-op that LEAVES the menu open. idx is validated against
// entries (the option set can change between frames).
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

// packAddMember appends to the edited pack via add, selects the new member, and
// marks dirty — the shared tail every pack-add entry runs.
func packAddMember(s *State, add func(*core.PackSpawn)) {
	if s.modalPackIdx < 0 || s.modalPackIdx >= len(s.area.PackSpawns) {
		return
	}
	pack := &s.area.PackSpawns[s.modalPackIdx]
	// Enforce the formation caps: at most EnemyFrontRowCap front + EnemyBackRowCap back.
	if len(pack.Members) >= core.EnemyPackCap {
		s.flash(fmt.Sprintf("Pack is full (max %d: %d front, %d back)",
			core.EnemyPackCap, core.EnemyFrontRowCap, core.EnemyBackRowCap))
		return
	}
	pushUndo(s)
	add(pack)
	// New members default to the front row; seat overflow in the back once the front
	// rank is full (total < cap guarantees the back has room).
	if front, _ := core.PackRowCounts(pack.Members); front > core.EnemyFrontRowCap {
		pack.Members[len(pack.Members)-1].Row = core.RowBack
	}
	s.modalCursor = len(pack.Members) - 1
	s.dirty = true
}

// --- Chest-add entries: every registered item kind ---

func chestAddEntries(s *State) []dropdownEntry {
	return itemKindEntries(func(s *State, kind core.ItemKind) {
		if s.modalChestIdx < 0 || s.modalChestIdx >= len(s.area.ChestSpawns) {
			return
		}
		pushUndo(s)
		chest := &s.area.ChestSpawns[s.modalChestIdx]
		chest.Items = append(chest.Items, kind)
		s.modalCursor = len(chest.Items) - 1
		s.dirty = true
	})
}

// --- Geometry (single source shared by update hit-test + draw) ---

type dropdownLayout struct {
	panel   rl.Rectangle
	topRow  int            // first visible option index (scroll window)
	rows    []rl.Rectangle // one rect per visible row, in order
	markerW float32        // left ✓ gutter width (menus with toggles); 0 for plain pickers
}

// computeDropdownLayout derives the panel rect + visible row rects from anchor,
// count, and cursor. Deterministic, so hit-test and draw agree without storing layout.
func computeDropdownLayout(s *State, entries []dropdownEntry) dropdownLayout {
	n := len(entries)
	if n == 0 {
		// No entries → no rows: building a row rect would let a hit-test/draw
		// loop index entries[0] and panic. Callers guard, but stay safe here too.
		return dropdownLayout{panel: s.dropdown.anchor}
	}
	visible := n
	if visible > dropdownMaxRows {
		visible = dropdownMaxRows
	}
	if visible < 1 {
		visible = 1
	}

	// Reserve a marker gutter only when some row has an eye/✓.
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
			measure += "    " + e.hotkey // room for the right-aligned accelerator
		}
		if lw := approxTextWidth(measure, editorFontBody) + 2*dropdownPad + 12 + markerW; lw > w {
			w = lw
		}
	}

	sw, sh := render.ScreenSizeF()
	if w > sw-8 {
		w = sw - 8
	}
	rowH := dropdownRowH
	if s.dropdown.rowH > 0 {
		rowH = s.dropdown.rowH
	}
	h := float32(visible)*rowH + 2*dropdownPad

	var x, y float32
	if s.dropdown.atPoint {
		// Free-floating menu (right-click): top-left at the click point, clamped so
		// an edge click doesn't push it off-screen.
		x, y = s.dropdown.at.X, s.dropdown.at.Y
		if x+w > sw {
			x = sw - w
		}
		if y+h > sh {
			y = sh - h
		}
		if x < 0 {
			x = 0
		}
		if y < 0 {
			y = 0
		}
	} else {
		x = s.dropdown.anchor.X
		if x+w > sw-4 {
			x = sw - 4 - w
		}
		if x < 4 {
			x = 4
		}
		// Grow UP from above the anchor, or DOWN from below it (top-anchored pickers).
		// Clamp to the screen edge; overlapping the modal body is fine (drawn last).
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
	}

	top, _ := scrollWindow(s.dropdown.cursor, n, visible)
	rows := make([]rl.Rectangle, visible)
	for i := 0; i < visible; i++ {
		rows[i] = rl.NewRectangle(x+dropdownPad, y+dropdownPad+float32(i)*rowH,
			w-2*dropdownPad, rowH)
	}
	return dropdownLayout{panel: rl.NewRectangle(x, y, w, h), topRow: top, rows: rows, markerW: markerW}
}

// updateDropdown handles one frame of the open dropdown, returning true while it
// owns input (the modal behind it stays inert).
func updateDropdown(s *State) bool {
	if !s.dropdownOpen() {
		return false
	}
	pumpDropdownFilter(s)
	entries := visibleDropdownEntries(s)
	if len(entries) == 0 {
		// A filter that matches nothing keeps the panel open (so Backspace recovers);
		// a genuinely empty list closes.
		if filterableDropdown(s.dropdown.owner) && s.dropdown.filter != "" {
			if editorCancelPressed() {
				closeDropdown(s)
			}
			return true
		}
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

	// Mouse hover drives the cursor so the per-row desc + Enter follow the
	// pointer. Skipped on a wheel-scroll frame, else the hovered row overrides
	// the scroll and the wheel appears dead.
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
				// A click on the eye gutter toggles visibility and keeps the list open.
				if lay.markerW > 0 && idx >= 0 && idx < len(entries) &&
					entries[idx].toggle != nil && mp.X <= rr.X+lay.markerW+dropdownEyeGutterSlop {
					entries[idx].toggle(s)
					return true
				}
				chooseDropdownEntry(s, entries, idx)
				return true
			}
		}
		if !pointIn(mp, lay.panel) {
			// Menu bar: a click on a DIFFERENT top-level label switches menus;
			// any other outside click dismisses.
			if s.dropdown.owner == ddMenu {
				if hit := buttonStripHit(menuBarBtns, menuBarBtnY, menuBarBtnH, mp); hit >= 0 && hit != s.dropdown.menu {
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

// drawDropdown paints the open dropdown on top of its modal.
func drawDropdown(s *State, font rl.Font, theme render.Theme) {
	if !s.dropdownOpen() {
		return
	}
	entries := visibleDropdownEntries(s)
	if len(entries) == 0 {
		// Filterable picker with no matches: show the query + a "no matches" note so
		// the empty panel isn't a mystery.
		if filterableDropdown(s.dropdown.owner) && s.dropdown.filter != "" {
			drawDropdownFilterCaption(s, font, theme, computeDropdownLayout(s, []dropdownEntry{{label: "(no matches)"}}).panel)
		}
		return
	}
	lay := computeDropdownLayout(s, entries)
	if filterableDropdown(s.dropdown.owner) {
		drawDropdownFilterCaption(s, font, theme, lay.panel)
	}
	// Opaque backing first: DrawCard fills with the translucent theme.SurfacePrimary, so
	// over the map the rows wash out. A solid editor-window tone underneath keeps them legible.
	rl.DrawRectangleRec(lay.panel, bgWindow)
	render.DrawCard(int32(lay.panel.X), int32(lay.panel.Y), int32(lay.panel.Width), int32(lay.panel.Height),
		theme.SurfacePrimary, theme.BorderSoft, theme.BorderActive)

	mp := rl.GetMousePosition()
	for i, rr := range lay.rows {
		idx := lay.topRow + i
		e := entries[idx]
		disabled := e.disabledIn(s)
		// Cursor highlight draws even on a disabled row so keyboard nav never
		// "loses" the cursor; text stays faded and choose still no-ops it. Hover
		// highlight is suppressed on disabled rows.
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
		} else if e.danger {
			col = theme.BorderDanger // destructive row reads red, hovered or not
		}
		// Left gutter marker: a hide/show EYE or an active-toggle ✓, not both.
		if lay.markerW > 0 && e.toggle != nil {
			drawLayerEye(rl.NewRectangle(rr.X+1, rr.Y+3, lay.markerW, rr.Height-6), e.toggleOn == nil || e.toggleOn(s), false)
		} else if lay.markerW > 0 && e.active != nil && e.active(s) {
			render.DrawTextWithShadow(font, "✓", rr.X+4, rr.Y+3, editorFontBody, theme.TextPrimary)
		}
		labelX := rr.X + 6 + lay.markerW
		if e.swatch.A > 0 {
			chip := rl.NewRectangle(labelX, rr.Y+5, 13, rr.Height-10)
			drawSwatch(chip, e.swatch)
			labelX += chip.Width + 6
		}
		render.DrawTextWithShadow(font, e.label, labelX, rr.Y+3, editorFontBody, col)
		if e.hotkey != "" {
			hw := render.MeasureRichText(font, e.hotkey, editorFontHint, 1).X
			render.DrawRichText(font, e.hotkey, rl.NewVector2(rr.X+rr.Width-hw-6, rr.Y+4), editorFontHint, 1, theme.TextHint)
		}
	}

	// ▲/▼ "more" affordances when scrolled.
	if lay.topRow > 0 {
		drawScrollArrow(font, true, rl.NewVector2(lay.panel.X+lay.panel.Width-16, lay.panel.Y+2), editorFontHint, theme.TextHint)
	}
	if lay.topRow+len(lay.rows) < len(entries) {
		drawScrollArrow(font, false, rl.NewVector2(lay.panel.X+lay.panel.Width-16, lay.panel.Y+lay.panel.Height-16), editorFontHint, theme.TextHint)
	}

	// Show the cursored row's one-line explanation beneath the panel — any dropdown
	// whose rows carry a desc (menu bar, layer picker, …), not just the menu bar.
	if cur := s.dropdown.cursor; cur >= 0 && cur < len(entries) && entries[cur].desc != "" {
		drawMenuDesc(font, theme, lay.panel, entries[cur].desc)
	}
}

// drawDropdownFilterCaption paints the type-to-filter query above the panel (below
// it near the top edge), so a filterable picker advertises the feature and shows the
// live query.
func drawDropdownFilterCaption(s *State, font rl.Font, theme render.Theme, panel rl.Rectangle) {
	txt := "Filter: " + s.dropdown.filter
	if s.dropdown.filter == "" {
		txt = "Type to filter…"
	}
	w := render.MeasureRichText(font, txt, editorFontHint, 1).X + 2*dropdownPad
	h := editorFontHint + 2*dropdownPad
	x := panel.X
	y := panel.Y - h - 6
	if y < 4 {
		y = panel.Y + panel.Height + 6
	}
	render.DrawCard(int32(x), int32(y), int32(w), int32(h), theme.SurfacePrimary, theme.BorderSoft, theme.BorderActive)
	render.DrawRichText(font, txt, rl.NewVector2(x+dropdownPad, y+dropdownPad), editorFontHint, 1, theme.TextPrimary)
}

// drawMenuDesc paints the cursored-row explanation as a caption below the panel.
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
	y := panel.Y + panel.Height + 12 // gap so the caption reads as separate
	h := editorFontHint + 2*dropdownPad
	render.DrawCard(int32(x), int32(y), int32(w), int32(h), theme.SurfacePrimary, theme.BorderSoft, theme.BorderSoft)
	render.DrawRichText(font, desc, rl.NewVector2(x+dropdownPad, y+dropdownPad), editorFontHint, 1, theme.TextPrimary)
}

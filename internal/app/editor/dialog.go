package editor

import (
	"crawler/internal/app/core"
	"crawler/internal/app/input"
	"crawler/internal/app/render"
	"fmt"
	"strconv"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// dialog.go is the editor's branching-conversation authoring surface — a
// drill-down that mirrors the established entity-edit / door-edit modal
// patterns:
//
//	modalDialogList       → the area's dialogs (Add / Edit / Delete / Triggers)
//	modalDialogNodes      → one dialog's nodes (Add / Edit / Delete / Set Start)
//	modalDialogNodeEdit   → one node: speaker (dropdown) + text/next/continue
//	                        fields + Is-Menu toggle + End-action + its choice list
//	modalDialogChoiceEdit → one choice: label + next + End-action + condition list
//	modalDialogActionEdit → the node/choice end-action (none / start-quest /
//	                        complete-quest / event + the quest-id / event-id)
//	modalDialogCondEdit   → one choice condition (gold / quest / foe-killed /
//	                        tile-visited)
//	modalDialogTriggerList / …Edit → the area's auto-start triggers
//
// Esc steps UP one level (the top-level list's Esc closes the whole flow);
// closeModal resets every dialog index. Node and choice IDs are
// auto-generated and stable — the author references them by typing into the
// "next" fields, so there's no ID-rename hazard.

// --- accessors -------------------------------------------------------------

func currentDialog(s *State) *core.DialogDefinition {
	if s.modalDialogIdx < 0 || s.modalDialogIdx >= len(s.area.Dialogs) {
		return nil
	}
	return &s.area.Dialogs[s.modalDialogIdx]
}

func currentDialogNode(s *State) *core.DialogNode {
	d := currentDialog(s)
	if d == nil || s.modalDialogNodeIdx < 0 || s.modalDialogNodeIdx >= len(d.Nodes) {
		return nil
	}
	return &d.Nodes[s.modalDialogNodeIdx]
}

func currentDialogChoice(s *State) *core.DialogChoice {
	n := currentDialogNode(s)
	if n == nil || s.modalDialogChoiceIdx < 0 || s.modalDialogChoiceIdx >= len(n.Choices) {
		return nil
	}
	return &n.Choices[s.modalDialogChoiceIdx]
}

func dialogNodeInRange(s *State) bool   { return currentDialogNode(s) != nil }
func dialogChoiceInRange(s *State) bool { return currentDialogChoice(s) != nil }

// clearDialogFocus drops any dialog text-field focus. Called by closeModal
// and every level transition so a stale focus can't pump into the wrong
// (or a freed) field.
func clearDialogFocus(s *State) {
	switch s.focus {
	case focusDialogNodeText, focusDialogNodeNext, focusDialogNodeContinue,
		focusDialogChoiceLabel, focusDialogChoiceNext, focusDialogActionID,
		focusDialogCondQuestID, focusDialogCondMessage, focusDialogCondGold,
		focusDialogCondFoeKills, focusDialogCondTileX, focusDialogCondTileZ,
		focusDialogTrigTileX, focusDialogTrigTileZ, focusDialogTrigFoeKills:
		s.focus = focusNone
	}
}

// currentDialogCond returns the condition the condition editor operates on, or
// nil when the index is out of range.
func currentDialogCond(s *State) *core.DialogChoiceCondition {
	c := currentDialogChoice(s)
	if c == nil || s.modalDialogCondIdx < 0 || s.modalDialogCondIdx >= len(c.Conditions) {
		return nil
	}
	return &c.Conditions[s.modalDialogCondIdx]
}

func dialogCondInRange(s *State) bool { return currentDialogCond(s) != nil }

// currentDialogTrigger returns the area trigger the trigger editor operates on,
// or nil when out of range.
func currentDialogTrigger(s *State) *core.DialogTrigger {
	if s.modalDialogTriggerIdx < 0 || s.modalDialogTriggerIdx >= len(s.area.Triggers) {
		return nil
	}
	return &s.area.Triggers[s.modalDialogTriggerIdx]
}

func dialogTriggerInRange(s *State) bool { return currentDialogTrigger(s) != nil }

// --- id generation ---------------------------------------------------------

// uniqueID returns the first "prefix1", "prefix2", … not already taken. The
// single counter-loop behind every auto-id'd dialog entity (dialogs / nodes /
// choices / triggers) — callers supply only the prefix and the "is this id
// taken?" predicate.
func uniqueID(prefix string, taken func(string) bool) string {
	for i := 1; ; i++ {
		id := prefix + strconv.Itoa(i)
		if !taken(id) {
			return id
		}
	}
}

func uniqueDialogID(s *State) string {
	return uniqueID("dialog_", func(id string) bool {
		_, exists := core.DialogDefByID(s.area, id)
		return exists
	})
}

func uniqueNodeID(d *core.DialogDefinition) string {
	return uniqueID("n", func(id string) bool {
		_, exists := d.NodeByID(id)
		return exists
	})
}

func uniqueChoiceID(n *core.DialogNode) string {
	return uniqueID("c", func(id string) bool {
		for _, c := range n.Choices {
			if c.ID == id {
				return true
			}
		}
		return false
	})
}

// --- ops (each pushes undo + marks dirty) ----------------------------------

func addDialog(s *State) {
	pushUndo(s)
	nodeID := "n1"
	s.area.Dialogs = append(s.area.Dialogs, core.DialogDefinition{
		ID:          uniqueDialogID(s),
		StartNodeID: nodeID,
		Nodes:       []core.DialogNode{{ID: nodeID, SpeakerID: core.SpeakerNarrator}},
	})
	s.modalCursor = len(s.area.Dialogs) - 1
	s.dirty = true
}

func removeDialogAt(s *State, idx int) {
	if idx < 0 || idx >= len(s.area.Dialogs) {
		return
	}
	pushUndo(s)
	s.area.Dialogs = removeModalListItem(s.area.Dialogs, idx)
	s.dirty = true
	clampModalCursor(s, len(s.area.Dialogs))
}

func addDialogNode(s *State) {
	d := currentDialog(s)
	if d == nil {
		return
	}
	pushUndo(s)
	d.Nodes = append(d.Nodes, core.DialogNode{ID: uniqueNodeID(d), SpeakerID: core.SpeakerNarrator})
	s.modalCursor = len(d.Nodes) - 1
	s.dirty = true
}

func removeDialogNode(s *State, idx int) {
	d := currentDialog(s)
	if d == nil || idx < 0 || idx >= len(d.Nodes) {
		return
	}
	pushUndo(s)
	d.Nodes = removeModalListItem(d.Nodes, idx)
	s.dirty = true
	clampModalCursor(s, len(d.Nodes))
}

func setDialogStartNode(s *State, idx int) {
	d := currentDialog(s)
	if d == nil || idx < 0 || idx >= len(d.Nodes) {
		return
	}
	pushUndo(s)
	d.StartNodeID = d.Nodes[idx].ID
	s.dirty = true
	s.flash("Start node: " + d.StartNodeID)
}

func addDialogChoice(s *State) {
	n := currentDialogNode(s)
	if n == nil {
		return
	}
	pushUndo(s)
	n.Choices = append(n.Choices, core.DialogChoice{ID: uniqueChoiceID(n), Label: "..."})
	s.modalCursor = len(n.Choices) - 1
	s.dirty = true
}

func removeDialogChoice(s *State, idx int) {
	n := currentDialogNode(s)
	if n == nil || idx < 0 || idx >= len(n.Choices) {
		return
	}
	pushUndo(s)
	n.Choices = removeModalListItem(n.Choices, idx)
	s.dirty = true
	clampModalCursor(s, len(n.Choices))
}

func clampModalCursor(s *State, count int) {
	if s.modalCursor >= count {
		s.modalCursor = count - 1
	}
	if s.modalCursor < 0 {
		s.modalCursor = 0
	}
}

// restoreModalCursor points the modal list cursor at idx when it's in [0,count),
// else 0 — the shared "re-highlight the row we were editing on the way back up
// (or reset to the top)" rule every dialog open/return helper uses.
func restoreModalCursor(s *State, idx, count int) {
	if idx >= 0 && idx < count {
		s.modalCursor = idx
	} else {
		s.modalCursor = 0
	}
}

// --- open / level transitions ----------------------------------------------

func openDialogListModal(s *State) {
	s.modal = modalDialogList
	restoreModalCursor(s, s.modalDialogIdx, len(s.area.Dialogs))
	clearDialogFocus(s)
}

func openDialogNodesModal(s *State, dialogIdx int) {
	if dialogIdx < 0 || dialogIdx >= len(s.area.Dialogs) {
		return
	}
	s.modal = modalDialogNodes
	s.modalDialogIdx = dialogIdx
	s.modalDialogNodeIdx = -1
	s.modalCursor = 0
	clearDialogFocus(s)
}

// returnToDialogNodes steps back up from the node editor, re-highlighting the
// node that was being edited.
func returnToDialogNodes(s *State) {
	s.modal = modalDialogNodes
	count := 0
	if d := currentDialog(s); d != nil {
		count = len(d.Nodes)
	}
	restoreModalCursor(s, s.modalDialogNodeIdx, count)
	clearDialogFocus(s)
}

func openDialogNodeEditModal(s *State, nodeIdx int) {
	d := currentDialog(s)
	if d == nil || nodeIdx < 0 || nodeIdx >= len(d.Nodes) {
		return
	}
	s.modal = modalDialogNodeEdit
	s.modalDialogNodeIdx = nodeIdx
	s.modalDialogChoiceIdx = -1
	s.modalCursor = 0 // choice-list cursor
	s.focus = focusNone
}

// returnToDialogNodeEdit steps back up from the choice editor, re-highlighting
// the choice that was being edited.
func returnToDialogNodeEdit(s *State) {
	s.modal = modalDialogNodeEdit
	count := 0
	if n := currentDialogNode(s); n != nil {
		count = len(n.Choices)
	}
	restoreModalCursor(s, s.modalDialogChoiceIdx, count)
	clearDialogFocus(s)
}

func openDialogChoiceEditModal(s *State, choiceIdx int) {
	n := currentDialogNode(s)
	if n == nil || choiceIdx < 0 || choiceIdx >= len(n.Choices) {
		return
	}
	s.modal = modalDialogChoiceEdit
	s.modalDialogChoiceIdx = choiceIdx
	s.modalCursor = 0 // condition-list cursor
	// Start UNfocused (like the node editor) so Up/Down drive the condition
	// list immediately and Tab steps into the text fields — otherwise a
	// keyboard user opening on a focused field can't reach the list.
	s.focus = focusNone
}

// --- speaker dropdown ------------------------------------------------------

func dialogSpeakerEntries(s *State) []dropdownEntry {
	ids := core.DialogSpeakerIDs()
	out := make([]dropdownEntry, 0, len(ids))
	for _, id := range ids {
		id := id
		out = append(out, dropdownEntry{
			label: core.DialogSpeakerName(id),
			apply: func(s *State) {
				if n := currentDialogNode(s); n != nil {
					pushUndo(s)
					n.SpeakerID = id
					s.dirty = true
				}
			},
		})
	}
	return out
}

// --- condition / trigger labels --------------------------------------------

func condKindLabel(k core.DialogCondKind) string {
	switch k {
	case core.DialogCondGold:
		return "Gold ≥ amount"
	case core.DialogCondQuest:
		return "Quest status"
	case core.DialogCondFoeKilled:
		return "Foe killed"
	case core.DialogCondTileVisited:
		return "Tile visited"
	}
	return string(k)
}

func questStatusLabel(qs core.QuestStatus) string {
	if qs == core.QuestComplete {
		return "Complete"
	}
	return "Active"
}

func triggerKindLabel(k core.DialogTriggerKind) string {
	switch k {
	case core.DialogTriggerEnterTile:
		return "Enter tile"
	case core.DialogTriggerFoeKilled:
		return "Foe killed"
	}
	return string(k)
}

// init asserts every authorable condition / trigger kind has a real editor
// label (condKindLabel / triggerKindLabel fall back to the raw kind string for
// an unhandled kind). Since the kind dropdowns now iterate core.DialogCondKinds
// / DialogTriggerKinds, a kind added in core auto-appears in the picker; this
// guard makes "forgot the editor label" a startup panic rather than a row that
// renders its raw key — the same panic-at-init discipline the tile / skill /
// grade registries use.
func init() {
	for _, k := range core.DialogCondKinds() {
		if condKindLabel(k) == string(k) {
			panic("editor: condKindLabel missing a case for dialog condition kind " + string(k))
		}
	}
	for _, k := range core.DialogTriggerKinds() {
		if triggerKindLabel(k) == string(k) {
			panic("editor: triggerKindLabel missing a case for dialog trigger kind " + string(k))
		}
	}
}

// condSummary is the one-line row label for a choice's condition list.
func condSummary(c core.DialogChoiceCondition) string {
	switch c.Kind {
	case core.DialogCondGold:
		return fmt.Sprintf("Gold ≥ %d", c.Gold)
	case core.DialogCondQuest:
		return fmt.Sprintf("Quest %q = %s", c.QuestID, questStatusLabel(c.QuestStatus))
	case core.DialogCondFoeKilled:
		return fmt.Sprintf("Killed %s ×%d", core.FoeKindName(c.FoeKind), core.RequiredFoeKills(c.FoeKills))
	case core.DialogCondTileVisited:
		return fmt.Sprintf("Visited tile (%d, %d)", c.TileX, c.TileZ)
	}
	return string(c.Kind)
}

// triggerSummary is the one-line row label for the trigger list.
func triggerSummary(t core.DialogTrigger) string {
	once := ""
	if t.Once {
		once = " · once"
	}
	switch t.Kind {
	case core.DialogTriggerEnterTile:
		return fmt.Sprintf("Enter (%d,%d) → %s%s", t.TileX, t.TileZ, t.DialogID, once)
	case core.DialogTriggerFoeKilled:
		return fmt.Sprintf("Kill %s ×%d → %s%s", core.FoeKindName(t.FoeKind), core.RequiredFoeKills(t.FoeKills), t.DialogID, once)
	}
	return string(t.Kind)
}

// --- condition / trigger dropdown entries ----------------------------------

func dialogCondKindEntries(s *State) []dropdownEntry {
	kinds := core.DialogCondKinds()
	out := make([]dropdownEntry, 0, len(kinds))
	for _, k := range kinds {
		k := k
		out = append(out, dropdownEntry{label: condKindLabel(k), apply: func(s *State) {
			// Act only on an actual kind CHANGE, and reset to a clean condition
			// of the new kind so a stale value from the previous kind (e.g. a
			// Gold amount after switching to TileVisited) can't linger on the
			// struct and serialize. The kind-agnostic DisabledMessage carries over.
			if c := currentDialogCond(s); c != nil && c.Kind != k {
				pushUndo(s)
				*c = core.DialogChoiceCondition{Kind: k, DisabledMessage: c.DisabledMessage}
				s.dirty = true
			}
		}})
	}
	return out
}

func dialogQuestStatusEntries(s *State) []dropdownEntry {
	opts := []core.QuestStatus{core.QuestActive, core.QuestComplete}
	out := make([]dropdownEntry, 0, len(opts))
	for _, qs := range opts {
		qs := qs
		out = append(out, dropdownEntry{label: questStatusLabel(qs), apply: func(s *State) {
			if c := currentDialogCond(s); c != nil {
				pushUndo(s)
				c.QuestStatus = qs
				s.dirty = true
			}
		}})
	}
	return out
}

func dialogCondFoeEntries(s *State) []dropdownEntry {
	return enemyKindEntries(func(s *State, kind core.EnemyKind) {
		if c := currentDialogCond(s); c != nil {
			pushUndo(s)
			c.FoeKind = kind
			s.dirty = true
		}
	})
}

func dialogTriggerKindEntries(s *State) []dropdownEntry {
	kinds := core.DialogTriggerKinds()
	out := make([]dropdownEntry, 0, len(kinds))
	for _, k := range kinds {
		k := k
		out = append(out, dropdownEntry{label: triggerKindLabel(k), apply: func(s *State) {
			// Mirror dialogCondKindEntries: act only on a real kind CHANGE and
			// reset to a clean trigger of the new kind so the params the previous
			// kind owned (enterTile's TileX/TileZ vs foeKilled's FoeKind/FoeKills)
			// can't linger on the struct and serialize. The kind-agnostic
			// ID/DialogID/Once carry over.
			if t := currentDialogTrigger(s); t != nil && t.Kind != k {
				pushUndo(s)
				*t = core.DialogTrigger{ID: t.ID, Kind: k, DialogID: t.DialogID, Once: t.Once}
				s.dirty = true
			}
		}})
	}
	return out
}

func dialogTriggerDialogEntries(s *State) []dropdownEntry {
	out := make([]dropdownEntry, 0, len(s.area.Dialogs))
	for i := range s.area.Dialogs {
		id := s.area.Dialogs[i].ID
		out = append(out, dropdownEntry{label: id, apply: func(s *State) {
			if t := currentDialogTrigger(s); t != nil {
				pushUndo(s)
				t.DialogID = id
				s.dirty = true
			}
		}})
	}
	return out
}

func dialogTriggerFoeEntries(s *State) []dropdownEntry {
	return enemyKindEntries(func(s *State, kind core.EnemyKind) {
		if t := currentDialogTrigger(s); t != nil {
			pushUndo(s)
			t.FoeKind = kind
			s.dirty = true
		}
	})
}

// --- shared numeric-field editing (condition + trigger) --------------------
//
// This (dialogNumericTarget + focusDialogNumeric + pumpDialogNumeric, backed by
// the shared dialogNumBuf) is the CANONICAL way to type into a plain int field
// in the editor — new numeric fields should route through it. It is distinct
// from updateNumericInput (input.go), which is the special-cased path for the
// map's width/height: those clamp to ClampMapDimension and trigger a live area
// resize, so they keep their own focus enums rather than folding in here.

// dialogNumericTarget returns the int field the currently-focused numeric
// dialog field edits, or nil when no numeric field is focused.
func dialogNumericTarget(s *State) *int {
	switch s.focus {
	case focusDialogCondGold:
		if c := currentDialogCond(s); c != nil {
			return &c.Gold
		}
	case focusDialogCondFoeKills:
		if c := currentDialogCond(s); c != nil {
			return &c.FoeKills
		}
	case focusDialogCondTileX:
		if c := currentDialogCond(s); c != nil {
			return &c.TileX
		}
	case focusDialogCondTileZ:
		if c := currentDialogCond(s); c != nil {
			return &c.TileZ
		}
	case focusDialogTrigTileX:
		if t := currentDialogTrigger(s); t != nil {
			return &t.TileX
		}
	case focusDialogTrigTileZ:
		if t := currentDialogTrigger(s); t != nil {
			return &t.TileZ
		}
	case focusDialogTrigFoeKills:
		if t := currentDialogTrigger(s); t != nil {
			return &t.FoeKills
		}
	}
	return nil
}

// focusDialogNumeric focuses a numeric field and seeds the shared edit buffer
// from its current value, so the author edits from the existing number.
func focusDialogNumeric(s *State, focus focusField, value int) {
	s.focus = focus
	s.dialogNumBuf = strconv.Itoa(value)
}

// pumpDialogNumeric routes typed digits into the focused numeric field through
// the shared buffer (parsed back each keystroke; empty = 0). Returns true while
// a numeric field owns input.
func pumpDialogNumeric(s *State) bool {
	target := dialogNumericTarget(s)
	if target == nil {
		return false
	}
	pumpFocusField(s, &s.dialogNumBuf)
	v, _ := strconv.Atoi(s.dialogNumBuf)
	*target = v
	return true
}

// numFieldText shows the live buffer while focused, else the committed value.
func numFieldText(focused bool, value int, buf string) string {
	if focused {
		return buf
	}
	return strconv.Itoa(value)
}

// --- condition / trigger ops (each pushes undo + marks dirty) --------------

func addDialogCond(s *State) {
	c := currentDialogChoice(s)
	if c == nil {
		return
	}
	pushUndo(s)
	c.Conditions = append(c.Conditions, core.DialogChoiceCondition{Kind: core.DialogCondGold})
	s.modalCursor = len(c.Conditions) - 1
	s.dirty = true
}

func removeDialogCond(s *State, idx int) {
	c := currentDialogChoice(s)
	if c == nil || idx < 0 || idx >= len(c.Conditions) {
		return
	}
	pushUndo(s)
	c.Conditions = removeModalListItem(c.Conditions, idx)
	s.dirty = true
	clampModalCursor(s, len(c.Conditions))
}

func uniqueTriggerID(s *State) string {
	return uniqueID("trig_", func(id string) bool {
		for j := range s.area.Triggers {
			if s.area.Triggers[j].ID == id {
				return true
			}
		}
		return false
	})
}

func addDialogTrigger(s *State) {
	pushUndo(s)
	t := core.DialogTrigger{ID: uniqueTriggerID(s), Kind: core.DialogTriggerEnterTile, Once: true}
	if len(s.area.Dialogs) > 0 {
		t.DialogID = s.area.Dialogs[0].ID
	}
	s.area.Triggers = append(s.area.Triggers, t)
	s.modalCursor = len(s.area.Triggers) - 1
	s.dirty = true
}

func removeDialogTriggerAt(s *State, idx int) {
	if idx < 0 || idx >= len(s.area.Triggers) {
		return
	}
	pushUndo(s)
	s.area.Triggers = removeModalListItem(s.area.Triggers, idx)
	s.dirty = true
	clampModalCursor(s, len(s.area.Triggers))
}

// dialogListModalSpec parameterizes the three list-style dialog modals (the
// dialog list, a dialog's node list, and the trigger list) so their draw +
// update flow lives in one driver. They share the same skeleton — header, a
// hint line, an Add/Action command set, the scrolling entity-list window, and
// Up/Down + Enter/A/X handling — and differ only in the fields below.
type dialogListModalSpec struct {
	title    string                     // modal header
	hint     string                     // hint line below the header
	empty    string                     // placeholder when the list is empty
	count    int                        // number of rows
	rowLabel func(i int) string         // one row's label
	cmds     func(*State) (adds, actions []modalCmd)
	commit   func()                     // Enter / "Edit" (only when count > 0)
	cancel   func()                     // Esc (step up / close)
	add      func()                     // A / "+ Add"
	del      func()                     // X / "Delete" (only when count > 0)
	// extraKeys are per-modal extra shortcuts beyond Enter/A/X (e.g. T opens the
	// trigger list, S sets the start node). guarded reports whether the key needs
	// count > 0 before it runs.
	extraKeys []dialogListKey
}

type dialogListKey struct {
	key     int32
	guarded bool
	run     func()
}

// drawDialogListModalGeneric renders one list-style dialog modal from its spec.
func drawDialogListModalGeneric(s *State, font rl.Font, theme render.Theme, spec dialogListModalSpec) {
	r := drawModalHeader(font, theme, entityEditModalW, entityEditModalH, spec.title, theme.BorderActive)
	render.DrawTextWithShadow(font, spec.hint, r.X+modalContentInset, r.Y+40, editorFontTiny, theme.TextHint)
	adds, actions := spec.cmds(s)
	lay := entityModalLayoutFor(s.modalCursor, spec.count, cmdLabels(adds), cmdLabels(actions))
	drawEntityListWindow(font, theme, lay, spec.count, s.modalCursor, spec.empty, spec.rowLabel)
	drawModalButtons(font, lay.actRects, cmdLabels(actions))
	drawModalButtons(font, lay.addRects, cmdLabels(adds))
}

// updateDialogListModalGeneric drives one list-style dialog modal from its spec.
func updateDialogListModalGeneric(s *State, spec dialogListModalSpec) Action {
	count := spec.count
	clampModalCursor(s, count)
	if editorCancelPressed() {
		spec.cancel()
		return ActionNone
	}
	if count > 0 {
		s.modalCursor = input.CursorUpDown(s.modalCursor, count)
	}
	if handleEntityModalClick(s, count, spec.cmds) {
		return ActionNone
	}
	if editorCommitPressed() {
		if count > 0 {
			spec.commit()
		}
		return ActionNone
	}
	if rl.IsKeyPressed(rl.KeyA) {
		spec.add()
		return ActionNone
	}
	for _, k := range spec.extraKeys {
		if rl.IsKeyPressed(k.key) {
			if !k.guarded || count > 0 {
				k.run()
			}
			return ActionNone
		}
	}
	if count > 0 && rl.IsKeyPressed(rl.KeyX) {
		spec.del()
	}
	return ActionNone
}

// =========================== modalDialogList ===============================

func dialogListCmds(s *State) (adds, actions []modalCmd) {
	adds = []modalCmd{
		{label: "+ Add dialog  (A)", run: func() Action { addDialog(s); return ActionNone }},
	}
	actions = []modalCmd{
		{label: "Edit  (Enter)", run: func() Action {
			if len(s.area.Dialogs) > 0 {
				openDialogNodesModal(s, s.modalCursor)
			}
			return ActionNone
		}},
		{label: "Delete  (X)", run: func() Action { removeDialogAt(s, s.modalCursor); return ActionNone }},
		{label: "Triggers  (T)", run: func() Action { openDialogTriggerListModal(s); return ActionNone }},
	}
	return adds, actions
}

// dialogListSpec builds the spec for the top-level dialog list.
func dialogListSpec(s *State) dialogListModalSpec {
	return dialogListModalSpec{
		title:    "DIALOGS",
		hint:     "Enter edit · A add · X delete · T triggers · Esc close",
		empty:    "(no dialogs — Add one)",
		count:    len(s.area.Dialogs),
		rowLabel: func(i int) string { return dialogListRowLabel(s.area.Dialogs[i]) },
		cmds:     dialogListCmds,
		commit:   func() { openDialogNodesModal(s, s.modalCursor) },
		cancel:   func() { closeModal(s) },
		add:      func() { addDialog(s) },
		del:      func() { removeDialogAt(s, s.modalCursor) },
		extraKeys: []dialogListKey{
			{key: rl.KeyT, run: func() { openDialogTriggerListModal(s) }},
		},
	}
}

func drawDialogListModal(s *State, font rl.Font, theme render.Theme) {
	drawDialogListModalGeneric(s, font, theme, dialogListSpec(s))
}

func dialogListRowLabel(d core.DialogDefinition) string {
	return fmt.Sprintf("%s  (%d nodes)", d.ID, len(d.Nodes))
}

func updateDialogListModal(s *State) Action {
	return updateDialogListModalGeneric(s, dialogListSpec(s))
}

// =========================== modalDialogNodes ==============================

func dialogNodesCmds(s *State) (adds, actions []modalCmd) {
	adds = []modalCmd{
		{label: "+ Add node  (A)", run: func() Action { addDialogNode(s); return ActionNone }},
	}
	actions = []modalCmd{
		{label: "Edit  (Enter)", run: func() Action {
			if dialogNodeCount(s) > 0 {
				openDialogNodeEditModal(s, s.modalCursor)
			}
			return ActionNone
		}},
		{label: "Delete  (X)", run: func() Action { removeDialogNode(s, s.modalCursor); return ActionNone }},
		{label: "Set Start  (S)", run: func() Action { setDialogStartNode(s, s.modalCursor); return ActionNone }},
	}
	return adds, actions
}

func dialogNodeCount(s *State) int {
	if d := currentDialog(s); d != nil {
		return len(d.Nodes)
	}
	return 0
}

// dialogNodesSpec builds the spec for one dialog's node list.
func dialogNodesSpec(s *State, d *core.DialogDefinition) dialogListModalSpec {
	return dialogListModalSpec{
		title:    "DIALOG " + d.ID,
		hint:     "Enter edit · A add · X delete · S set start · Esc back",
		empty:    "(no nodes — Add one)",
		count:    len(d.Nodes),
		rowLabel: func(i int) string { return dialogNodeRowLabel(*d, i) },
		cmds:     dialogNodesCmds,
		commit:   func() { openDialogNodeEditModal(s, s.modalCursor) },
		cancel:   func() { openDialogListModal(s) }, // step UP to the dialog list
		add:      func() { addDialogNode(s) },
		del:      func() { removeDialogNode(s, s.modalCursor) },
		extraKeys: []dialogListKey{
			{key: rl.KeyS, guarded: true, run: func() { setDialogStartNode(s, s.modalCursor) }},
		},
	}
}

func drawDialogNodesModal(s *State, font rl.Font, theme render.Theme) {
	d := currentDialog(s)
	if d == nil {
		return
	}
	drawDialogListModalGeneric(s, font, theme, dialogNodesSpec(s, d))
}

func dialogNodeRowLabel(d core.DialogDefinition, i int) string {
	n := d.Nodes[i]
	mark := ""
	if n.ID == d.StartNodeID {
		mark = "★ "
	}
	return fmt.Sprintf("%s%s · %s: %s", mark, n.ID, core.DialogSpeakerName(n.SpeakerID), truncateDialog(n.Text, 24))
}

func updateDialogNodesModal(s *State) Action {
	d := currentDialog(s)
	if d == nil {
		closeModal(s)
		return ActionNone
	}
	return updateDialogListModalGeneric(s, dialogNodesSpec(s, d))
}

// Shared field-layout metrics for the dialog edit modals (node / choice /
// condition / trigger). Their layout funcs stack fixed-height field rows down
// from a common header inset, so these live in one place rather than as a
// repeated `float32(28)` / `r.Y + 56` literal in each.
const (
	dialogFieldH       = float32(28) // standard text-field / button row height
	dialogHeaderInset  = float32(56) // first row's offset below the card's title
	dialogRowGap       = float32(46) // vertical pitch between stacked field rows (node + choice editors)
	dialogCondRowGap   = float32(54) // row pitch in the condition editor
	dialogTrigRowGap   = float32(52) // row pitch in the trigger editor
	dialogActionRowGap = float32(56) // row pitch in the action editor
	dialogListRowH     = dropdownRowH // scrollable choice / condition list rows share the dropdown's row pitch
)

// stackRows lays out n equal-height field rows stacked downward from (x,y) at a
// fixed vertical pitch (rowGap). It is the shared preamble the dialog edit
// modals (node / choice / action / cond / trigger) and the door editor all
// spelled inline as a repeated `row := rl.NewRectangle(x, y, fw, fieldH); y +=
// rowGap` — centralizing it keeps those hit-test rects byte-identical to the
// per-row build they replace. Callers that continue stacking content below the
// returned rows advance their own y by n*rowGap (the pitch this walked).
func stackRows(x, y, fw, fieldH, rowGap float32, n int) []rl.Rectangle {
	rows := make([]rl.Rectangle, n)
	for i := range rows {
		rows[i] = rl.NewRectangle(x, y+float32(i)*rowGap, fw, fieldH)
	}
	return rows
}

// scrollRows lays out the VISIBLE rows of a scrolling, fixed-pitch list: it
// resolves the scroll window over a `count`-item list keeping `cursor` in view
// (at most `visible` rows tall, via scrollWindow) and stacks one rowH-tall rect
// per visible item down from (x,y). It returns the window's top index and the
// visible row rects (one per item in [top, top+len(rows))). The node editor's
// choice list and the choice editor's condition list both build their scrolling
// row stack this way, so they share it rather than re-spelling the loop.
func scrollRows(x, y, fw, rowH float32, cursor, count, visible int) (top int, rows []rl.Rectangle) {
	top, end := scrollWindow(cursor, count, visible)
	rows = make([]rl.Rectangle, end-top)
	for i := range rows {
		rows[i] = rl.NewRectangle(x, y+float32(i)*rowH, fw, rowH)
	}
	return top, rows
}

// ========================= modalDialogNodeEdit =============================

const (
	dialogNodeModalW = float32(540)
	dialogNodeModalH = float32(600)
	// dialogChoiceVisible caps how many choice rows the node editor shows at
	// once; longer choice lists scroll (the cursor stays in view) rather than
	// running off the card.
	dialogChoiceVisible = 6
)

// dialogNodeLayout is the shared geometry for the node editor (draw + hit-test).
type dialogNodeLayout struct {
	card          rl.Rectangle
	speakerBtn    rl.Rectangle
	textField     rl.Rectangle
	nextField     rl.Rectangle
	continueField rl.Rectangle
	menuToggle    rl.Rectangle
	actionBtn     rl.Rectangle
	// choiceRows are the rects of the VISIBLE choice rows; choiceTop is the
	// index of the first visible choice (the scroll window's top). Visible
	// row i maps to choice index choiceTop+i.
	choiceTop     int
	choiceRows    []rl.Rectangle
	addChoiceBtn  rl.Rectangle
	editChoiceBtn rl.Rectangle
	delChoiceBtn  rl.Rectangle
	backBtn       rl.Rectangle
}

func dialogNodeLayoutFor(cursor, choiceCount int) dialogNodeLayout {
	r := centeredCardRect(dialogNodeModalW, dialogNodeModalH)
	x := r.X + modalContentInset
	fw := r.Width - 2*modalContentInset
	fieldH := dialogFieldH
	rowGap := dialogRowGap
	y := r.Y + dialogHeaderInset
	fields := stackRows(x, y, fw, fieldH, rowGap, 6)
	speakerBtn := fields[0]
	textField := fields[1]
	nextField := fields[2]
	continueField := fields[3]
	menuToggle := fields[4]
	actionBtn := fields[5]
	y += 6*rowGap + 6
	// Choice list rows — a scroll window over the full choice list so a node
	// with more than dialogChoiceVisible choices keeps the cursored row in
	// view (and every choice reachable) instead of capping at the first few.
	top, rows := scrollRows(x, y, fw, dialogListRowH, cursor, choiceCount, dialogChoiceVisible)
	// Choice action buttons + Back along the bottom.
	by := r.Y + r.Height - modalBtnH - modalBottomInset
	btns := equalButtonRow(x, by, fw, modalBtnH, 4)
	return dialogNodeLayout{
		card:          r,
		speakerBtn:    speakerBtn,
		textField:     textField,
		nextField:     nextField,
		continueField: continueField,
		menuToggle:    menuToggle,
		actionBtn:     actionBtn,
		choiceTop:     top,
		choiceRows:    rows,
		addChoiceBtn:  btns[0],
		editChoiceBtn: btns[1],
		delChoiceBtn:  btns[2],
		backBtn:       btns[3],
	}
}

func drawDialogNodeEditModal(s *State, font rl.Font, theme render.Theme) {
	n := currentDialogNode(s)
	if n == nil {
		return
	}
	l := dialogNodeLayoutFor(s.modalCursor, len(n.Choices))
	drawModalHeaderAt(font, theme, l.card, "NODE "+n.ID, theme.BorderActive)

	drawLabel(font, "Speaker (click to choose)", labelAbove(l.speakerBtn))
	drawButton(font, l.speakerBtn, core.DialogSpeakerName(n.SpeakerID)+dropdownArrowSuffix, false)

	drawLabel(font, "Text", labelAbove(l.textField))
	drawTextField(font, l.textField, n.Text, s.focus == focusDialogNodeText)

	drawLabel(font, "Next node id (blank = ends after this line)", labelAbove(l.nextField))
	drawTextField(font, l.nextField, n.NextNodeID, s.focus == focusDialogNodeNext)

	drawLabel(font, "Continue label (blank = Continue)", labelAbove(l.continueField))
	drawTextField(font, l.continueField, n.ContinueLabel, s.focus == focusDialogNodeContinue)

	drawButton(font, l.menuToggle, "Menu node (auto-advance): "+render.OnOffLabel(n.IsMenuNode), n.IsMenuNode)

	drawLabel(font, "End action when this line resolves (click to edit)", labelAbove(l.actionBtn))
	drawButton(font, l.actionBtn, "Action: "+dialogActionLabel(n.EndAction)+dropdownArrowSuffix, n.EndAction != nil)

	drawLabel(font, fmt.Sprintf("Choices (%d) — Up/Down select, Enter edit", len(n.Choices)),
		rl.NewRectangle(l.card.X+modalContentInset, l.actionBtn.Y+l.actionBtn.Height+10, l.card.Width, 14))
	upHintY := float32(0)
	if len(l.choiceRows) > 0 {
		upHintY = l.choiceRows[0].Y - 16
	}
	drawScrollList(font, theme, l.choiceRows, l.choiceTop, len(n.Choices), s.modalCursor, 52,
		l.card.X+entityListTextInset, upHintY, l.addChoiceBtn.Y-18,
		func(idx int) string { return n.Choices[idx].Label })
	drawButton(font, l.addChoiceBtn, "+ Choice", false)
	drawButton(font, l.editChoiceBtn, "Edit Choice", false)
	drawButton(font, l.delChoiceBtn, "Del Choice", false)
	drawButton(font, l.backBtn, "Back (Esc)", false)
}

func updateDialogNodeEditModal(s *State) Action {
	n := currentDialogNode(s)
	if n == nil {
		closeModal(s)
		return ActionNone
	}
	l := dialogNodeLayoutFor(s.modalCursor, len(n.Choices))

	// Mouse: focus a field, toggle, open the speaker dropdown, or run a button.
	if rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		mp := rl.GetMousePosition()
		switch {
		case pointIn(mp, l.speakerBtn):
			openDropdownBelow(s, ddDialogSpeaker, l.speakerBtn)
			return ActionNone
		case pointIn(mp, l.textField):
			s.focus = focusDialogNodeText
			return ActionNone
		case pointIn(mp, l.nextField):
			s.focus = focusDialogNodeNext
			return ActionNone
		case pointIn(mp, l.continueField):
			s.focus = focusDialogNodeContinue
			return ActionNone
		case pointIn(mp, l.menuToggle):
			toggleNodeMenu(s)
			return ActionNone
		case pointIn(mp, l.actionBtn):
			openDialogActionEditModal(s, false)
			return ActionNone
		case pointIn(mp, l.addChoiceBtn):
			addDialogChoice(s)
			return ActionNone
		case pointIn(mp, l.editChoiceBtn):
			if len(n.Choices) > 0 {
				openDialogChoiceEditModal(s, s.modalCursor)
			}
			return ActionNone
		case pointIn(mp, l.delChoiceBtn):
			removeDialogChoice(s, s.modalCursor)
			return ActionNone
		case pointIn(mp, l.backBtn):
			returnToDialogNodes(s)
			return ActionNone
		}
		for i, row := range l.choiceRows {
			if pointIn(mp, row) {
				s.modalCursor = l.choiceTop + i
				return ActionNone
			}
		}
		s.focus = focusNone // click elsewhere defocuses fields
	}

	// While a text field is focused: route keys into it. Enter commits the
	// field (defocus, stay in the node editor); Esc steps straight back up to
	// the node list (single-press, matching the door modal).
	if target := dialogNodeTextTarget(s); target != nil {
		pumpFocusField(s, target)
		if editorTabPressed() {
			cycleDialogNodeFocus(s)
			return ActionNone
		}
		if editorCommitPressed() {
			s.focus = focusNone
			return ActionNone
		}
		if editorCancelPressed() {
			returnToDialogNodes(s)
			return ActionNone
		}
		return ActionNone
	}

	// No field focused — list navigation + shortcuts.
	if editorCancelPressed() {
		returnToDialogNodes(s)
		return ActionNone
	}
	if editorTabPressed() {
		s.focus = focusDialogNodeText
		return ActionNone
	}
	if len(n.Choices) > 0 {
		s.modalCursor = input.CursorUpDown(s.modalCursor, len(n.Choices))
	}
	if editorCommitPressed() {
		if len(n.Choices) > 0 {
			openDialogChoiceEditModal(s, s.modalCursor)
		}
		return ActionNone
	}
	if rl.IsKeyPressed(rl.KeyA) {
		addDialogChoice(s)
		return ActionNone
	}
	if len(n.Choices) > 0 && rl.IsKeyPressed(rl.KeyX) {
		removeDialogChoice(s, s.modalCursor)
		return ActionNone
	}
	if rl.IsKeyPressed(rl.KeyM) {
		toggleNodeMenu(s)
	}
	return ActionNone
}

func toggleNodeMenu(s *State) {
	if n := currentDialogNode(s); n != nil {
		pushUndo(s)
		n.IsMenuNode = !n.IsMenuNode
		s.dirty = true
	}
}

// dialogNodeTextTarget returns the field pointer for the active node-edit
// focus, or nil when no node field is focused.
func dialogNodeTextTarget(s *State) *string {
	n := currentDialogNode(s)
	if n == nil {
		return nil
	}
	switch s.focus {
	case focusDialogNodeText:
		return &n.Text
	case focusDialogNodeNext:
		return &n.NextNodeID
	case focusDialogNodeContinue:
		return &n.ContinueLabel
	}
	return nil
}

func cycleDialogNodeFocus(s *State) {
	switch s.focus {
	case focusDialogNodeText:
		s.focus = focusDialogNodeNext
	case focusDialogNodeNext:
		s.focus = focusDialogNodeContinue
	default:
		s.focus = focusDialogNodeText
	}
}

// ========================= modalDialogChoiceEdit ===========================

const (
	dialogChoiceModalW = float32(520)
	dialogChoiceModalH = float32(470)
	// dialogCondVisible caps how many condition rows show at once; longer
	// lists scroll (cursor stays in view) rather than running off the card.
	dialogCondVisible = 4
)

type dialogChoiceLayout struct {
	card        rl.Rectangle
	labelField  rl.Rectangle
	nextField   rl.Rectangle
	actionBtn   rl.Rectangle
	condTop     int
	condRows    []rl.Rectangle
	addCondBtn  rl.Rectangle
	editCondBtn rl.Rectangle
	delCondBtn  rl.Rectangle
	backBtn     rl.Rectangle
}

func dialogChoiceLayoutFor(cursor, condCount int) dialogChoiceLayout {
	r := centeredCardRect(dialogChoiceModalW, dialogChoiceModalH)
	x := r.X + modalContentInset
	fw := r.Width - 2*modalContentInset
	fieldH := dialogFieldH
	rowGap := dialogRowGap
	y := r.Y + dialogHeaderInset
	fields := stackRows(x, y, fw, fieldH, rowGap, 3)
	labelField := fields[0]
	nextField := fields[1]
	actionBtn := fields[2]
	y += 3*rowGap + 8 // gap for the "Conditions (N)" header line
	top, rows := scrollRows(x, y, fw, dialogListRowH, cursor, condCount, dialogCondVisible)
	by := r.Y + r.Height - modalBtnH - modalBottomInset
	btns := equalButtonRow(x, by, fw, modalBtnH, 4)
	return dialogChoiceLayout{
		card: r, labelField: labelField, nextField: nextField, actionBtn: actionBtn,
		condTop: top, condRows: rows,
		addCondBtn: btns[0], editCondBtn: btns[1], delCondBtn: btns[2], backBtn: btns[3],
	}
}

func drawDialogChoiceEditModal(s *State, font rl.Font, theme render.Theme) {
	c := currentDialogChoice(s)
	if c == nil {
		return
	}
	l := dialogChoiceLayoutFor(s.modalCursor, len(c.Conditions))
	drawModalHeaderAt(font, theme, l.card, "CHOICE "+c.ID, theme.BorderActive)

	drawLabel(font, "Label (shown to the player)", labelAbove(l.labelField))
	drawTextField(font, l.labelField, c.Label, s.focus == focusDialogChoiceLabel)

	drawLabel(font, "Next node id (blank = ends the conversation)", labelAbove(l.nextField))
	drawTextField(font, l.nextField, c.NextNodeID, s.focus == focusDialogChoiceNext)

	drawLabel(font, "End action on pick (click to edit)", labelAbove(l.actionBtn))
	drawButton(font, l.actionBtn, "Action: "+dialogActionLabel(c.EndAction)+dropdownArrowSuffix, c.EndAction != nil)

	header := rl.NewRectangle(l.card.X+modalContentInset, l.actionBtn.Y+l.actionBtn.Height+8, l.card.Width, 14)
	drawLabel(font, fmt.Sprintf("Conditions (%d) — gate selectability; Up/Down select, Enter edit", len(c.Conditions)), header)
	upHintY := float32(0)
	if len(l.condRows) > 0 {
		upHintY = l.condRows[0].Y - 14
	}
	drawScrollList(font, theme, l.condRows, l.condTop, len(c.Conditions), s.modalCursor, 56,
		l.card.X+entityListTextInset, upHintY, l.addCondBtn.Y-16,
		func(idx int) string { return condSummary(c.Conditions[idx]) })
	drawButton(font, l.addCondBtn, "+ Cond", false)
	drawButton(font, l.editCondBtn, "Edit Cond", false)
	drawButton(font, l.delCondBtn, "Del Cond", false)
	drawButton(font, l.backBtn, "Back (Esc)", false)
}

func updateDialogChoiceEditModal(s *State) Action {
	c := currentDialogChoice(s)
	if c == nil {
		closeModal(s)
		return ActionNone
	}
	l := dialogChoiceLayoutFor(s.modalCursor, len(c.Conditions))

	if rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		mp := rl.GetMousePosition()
		switch {
		case pointIn(mp, l.labelField):
			s.focus = focusDialogChoiceLabel
			return ActionNone
		case pointIn(mp, l.nextField):
			s.focus = focusDialogChoiceNext
			return ActionNone
		case pointIn(mp, l.actionBtn):
			openDialogActionEditModal(s, true)
			return ActionNone
		case pointIn(mp, l.addCondBtn):
			addDialogCond(s)
			return ActionNone
		case pointIn(mp, l.editCondBtn):
			if len(c.Conditions) > 0 {
				openDialogCondEditModal(s, s.modalCursor)
			}
			return ActionNone
		case pointIn(mp, l.delCondBtn):
			removeDialogCond(s, s.modalCursor)
			return ActionNone
		case pointIn(mp, l.backBtn):
			returnToDialogNodeEdit(s)
			return ActionNone
		}
		for i, row := range l.condRows {
			if pointIn(mp, row) {
				s.modalCursor = l.condTop + i
				return ActionNone
			}
		}
		s.focus = focusNone // click elsewhere defocuses fields
	}

	switch s.focus {
	case focusDialogChoiceLabel:
		pumpFocusField(s, &c.Label)
	case focusDialogChoiceNext:
		pumpFocusField(s, &c.NextNodeID)
	default:
		// No field focused — condition-list navigation + shortcuts.
		if editorCancelPressed() {
			returnToDialogNodeEdit(s)
			return ActionNone
		}
		if editorTabPressed() {
			s.focus = focusDialogChoiceLabel
			return ActionNone
		}
		if len(c.Conditions) > 0 {
			s.modalCursor = input.CursorUpDown(s.modalCursor, len(c.Conditions))
		}
		if editorCommitPressed() {
			if len(c.Conditions) > 0 {
				openDialogCondEditModal(s, s.modalCursor)
			}
			return ActionNone
		}
		if rl.IsKeyPressed(rl.KeyA) {
			addDialogCond(s)
			return ActionNone
		}
		if len(c.Conditions) > 0 && rl.IsKeyPressed(rl.KeyX) {
			removeDialogCond(s, s.modalCursor)
		}
		return ActionNone
	}
	// A text field is focused. Enter commits (defocus); Esc steps up.
	if editorTabPressed() {
		cycleDialogChoiceFocus(s)
		return ActionNone
	}
	if editorCancelPressed() {
		returnToDialogNodeEdit(s)
		return ActionNone
	}
	if editorCommitPressed() {
		s.focus = focusNone
	}
	return ActionNone
}

func cycleDialogChoiceFocus(s *State) {
	if s.focus == focusDialogChoiceLabel {
		s.focus = focusDialogChoiceNext
	} else {
		s.focus = focusDialogChoiceLabel
	}
}

// ========================= modalDialogActionEdit ===========================

const (
	dialogActionModalW = float32(520)
	dialogActionModalH = float32(240)
)

type dialogActionLayout struct {
	card    rl.Rectangle
	kindBtn rl.Rectangle
	idField rl.Rectangle
	backBtn rl.Rectangle
}

func dialogActionLayoutFor() dialogActionLayout {
	r := centeredCardRect(dialogActionModalW, dialogActionModalH)
	x := r.X + modalContentInset
	fw := r.Width - 2*modalContentInset
	fieldH := dialogFieldH
	y := r.Y + dialogHeaderInset
	fields := stackRows(x, y, fw, fieldH, dialogActionRowGap, 2)
	kindBtn := fields[0]
	idField := fields[1]
	backBtn := bottomRightBtn(r)
	return dialogActionLayout{card: r, kindBtn: kindBtn, idField: idField, backBtn: backBtn}
}

// currentDialogActionHolder returns a pointer to the EndAction field the action
// editor targets — the current CHOICE's when modalDialogActionOnChoice, else
// the current NODE's — or nil when that entity is out of range. The double
// pointer lets the kind picker set the holder to nil (no action) or allocate a
// fresh *DialogAction.
func currentDialogActionHolder(s *State) **core.DialogAction {
	if s.modalDialogActionOnChoice {
		if c := currentDialogChoice(s); c != nil {
			return &c.EndAction
		}
		return nil
	}
	if n := currentDialogNode(s); n != nil {
		return &n.EndAction
	}
	return nil
}

// dialogActionLabel is the human label for an end-action (nil = "None").
func dialogActionLabel(a *core.DialogAction) string {
	if a == nil {
		return "None"
	}
	switch a.Kind {
	case core.DialogActionQuest:
		if a.QuestOp == core.DialogQuestStart {
			return "Start quest"
		}
		return "Complete quest"
	case core.DialogActionEvent:
		return "Emit event"
	}
	return "None"
}

// dialogActionIDTarget returns the string field the action's id row edits
// (quest id for quest actions, event id for event actions), or nil when the
// current action has no id field.
func dialogActionIDTarget(s *State) *string {
	holder := currentDialogActionHolder(s)
	if holder == nil || *holder == nil {
		return nil
	}
	switch (*holder).Kind {
	case core.DialogActionQuest:
		return &(*holder).QuestID
	case core.DialogActionEvent:
		return &(*holder).EventID
	}
	return nil
}

// dialogActionKindEntries are the action picker's rows: None clears the action,
// the others ensure a non-nil action and set its kind/op. The flattened list
// (start/complete as separate rows rather than a kind + op pair) keeps the
// common cases one click away.
func dialogActionKindEntries(s *State) []dropdownEntry {
	// set == nil is the "None" row (clears the action); the others mutate a
	// non-nil action, each FULLY specifying the payload (including clearing the
	// OTHER kind's id field) so a stale QuestID / EventID can't linger after a
	// kind switch. Row LABELS are derived from dialogActionLabel against a probe
	// action, so the picker text can't drift from the button text it sets.
	sets := []func(*core.DialogAction){
		nil,
		func(a *core.DialogAction) {
			a.Kind = core.DialogActionQuest
			a.QuestOp = core.DialogQuestStart
			a.EventID = ""
		},
		func(a *core.DialogAction) {
			a.Kind = core.DialogActionQuest
			a.QuestOp = core.DialogQuestComplete
			a.EventID = ""
		},
		func(a *core.DialogAction) { a.Kind = core.DialogActionEvent; a.QuestOp = ""; a.QuestID = "" },
	}
	out := make([]dropdownEntry, 0, len(sets))
	for _, set := range sets {
		set := set
		label := dialogActionLabel(nil) // "None"
		if set != nil {
			var probe core.DialogAction
			set(&probe)
			label = dialogActionLabel(&probe)
		}
		out = append(out, dropdownEntry{label: label, apply: func(s *State) {
			holder := currentDialogActionHolder(s)
			if holder == nil {
				return
			}
			pushUndo(s)
			if set == nil {
				*holder = nil
			} else {
				if *holder == nil {
					*holder = &core.DialogAction{}
				}
				set(*holder)
			}
			s.dirty = true
		}})
	}
	return out
}

func openDialogActionEditModal(s *State, onChoice bool) {
	s.modalDialogActionOnChoice = onChoice
	if currentDialogActionHolder(s) == nil {
		return
	}
	s.modal = modalDialogActionEdit
	s.focus = focusNone
}

// returnFromDialogActionEdit steps back to whichever editor opened the action
// editor (leaving its list cursor untouched — the action editor doesn't use it).
func returnFromDialogActionEdit(s *State) {
	clearDialogFocus(s)
	if s.modalDialogActionOnChoice {
		s.modal = modalDialogChoiceEdit
	} else {
		s.modal = modalDialogNodeEdit
	}
}

func drawDialogActionEditModal(s *State, font rl.Font, theme render.Theme) {
	holder := currentDialogActionHolder(s)
	if holder == nil {
		return
	}
	a := *holder
	l := dialogActionLayoutFor()
	title := "NODE ACTION"
	if s.modalDialogActionOnChoice {
		title = "CHOICE ACTION"
	}
	drawModalHeaderAt(font, theme, l.card, title, theme.BorderActive)

	drawLabel(font, "Action (click to choose)", labelAbove(l.kindBtn))
	drawButton(font, l.kindBtn, dialogActionLabel(a)+dropdownArrowSuffix, a != nil)

	if a != nil {
		switch a.Kind {
		case core.DialogActionQuest:
			drawLabel(font, "Quest id", labelAbove(l.idField))
			drawTextField(font, l.idField, a.QuestID, s.focus == focusDialogActionID)
		case core.DialogActionEvent:
			drawLabel(font, "Event id", labelAbove(l.idField))
			drawTextField(font, l.idField, a.EventID, s.focus == focusDialogActionID)
		}
	}
	drawButton(font, l.backBtn, "Back (Esc)", false)
}

func updateDialogActionEditModal(s *State) Action {
	holder := currentDialogActionHolder(s)
	if holder == nil {
		closeModal(s)
		return ActionNone
	}
	l := dialogActionLayoutFor()

	if rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		mp := rl.GetMousePosition()
		switch {
		case pointIn(mp, l.kindBtn):
			s.focus = focusNone
			openDropdownBelow(s, ddDialogActionKind, l.kindBtn)
			return ActionNone
		case pointIn(mp, l.backBtn):
			returnFromDialogActionEdit(s)
			return ActionNone
		case pointIn(mp, l.idField) && dialogActionIDTarget(s) != nil:
			s.focus = focusDialogActionID
			return ActionNone
		}
		s.focus = focusNone
	}

	if s.focus == focusDialogActionID {
		if target := dialogActionIDTarget(s); target != nil {
			pumpFocusField(s, target)
		} else {
			s.focus = focusNone
		}
		if editorCommitPressed() {
			s.focus = focusNone
			return ActionNone
		}
		if editorCancelPressed() {
			returnFromDialogActionEdit(s)
			return ActionNone
		}
		return ActionNone
	}
	if editorCancelPressed() {
		returnFromDialogActionEdit(s)
	}
	return ActionNone
}

// ========================= modalDialogCondEdit =============================

const (
	dialogCondModalW = float32(540)
	dialogCondModalH = float32(360)
)

type dialogCondLayout struct {
	card     rl.Rectangle
	kindBtn  rl.Rectangle
	row1     rl.Rectangle
	row2     rl.Rectangle
	msgField rl.Rectangle
	backBtn  rl.Rectangle
}

func dialogCondLayoutFor() dialogCondLayout {
	r := centeredCardRect(dialogCondModalW, dialogCondModalH)
	x := r.X + modalContentInset
	fw := r.Width - 2*modalContentInset
	fieldH := dialogFieldH
	y := r.Y + dialogHeaderInset
	fields := stackRows(x, y, fw, fieldH, dialogCondRowGap, 4)
	kindBtn := fields[0]
	row1 := fields[1]
	row2 := fields[2]
	msgField := fields[3]
	backBtn := bottomRightBtn(r)
	return dialogCondLayout{card: r, kindBtn: kindBtn, row1: row1, row2: row2, msgField: msgField, backBtn: backBtn}
}

func openDialogCondEditModal(s *State, condIdx int) {
	c := currentDialogChoice(s)
	if c == nil || condIdx < 0 || condIdx >= len(c.Conditions) {
		return
	}
	s.modal = modalDialogCondEdit
	s.modalDialogCondIdx = condIdx
	s.focus = focusNone
}

// returnToDialogChoiceEdit steps back up from the condition editor, restoring
// the choice editor with the edited condition re-highlighted.
func returnToDialogChoiceEdit(s *State) {
	s.modal = modalDialogChoiceEdit
	count := 0
	if c := currentDialogChoice(s); c != nil {
		count = len(c.Conditions)
	}
	restoreModalCursor(s, s.modalDialogCondIdx, count)
	clearDialogFocus(s)
}

func drawDialogCondEditModal(s *State, font rl.Font, theme render.Theme) {
	c := currentDialogCond(s)
	if c == nil {
		return
	}
	l := dialogCondLayoutFor()
	drawModalHeaderAt(font, theme, l.card, "CONDITION", theme.BorderActive)

	drawLabel(font, "Kind (click to choose)", labelAbove(l.kindBtn))
	drawButton(font, l.kindBtn, condKindLabel(c.Kind)+dropdownArrowSuffix, false)

	switch c.Kind {
	case core.DialogCondGold:
		drawLabel(font, "Minimum gold the party must hold", labelAbove(l.row1))
		drawTextField(font, l.row1, numFieldText(s.focus == focusDialogCondGold, c.Gold, s.dialogNumBuf), s.focus == focusDialogCondGold)
	case core.DialogCondQuest:
		drawLabel(font, "Quest id", labelAbove(l.row1))
		drawTextField(font, l.row1, c.QuestID, s.focus == focusDialogCondQuestID)
		drawLabel(font, "Required status (click to choose)", labelAbove(l.row2))
		drawButton(font, l.row2, questStatusLabel(c.QuestStatus)+dropdownArrowSuffix, false)
	case core.DialogCondFoeKilled:
		drawLabel(font, "Foe (click to choose)", labelAbove(l.row1))
		drawButton(font, l.row1, core.FoeKindName(c.FoeKind)+dropdownArrowSuffix, false)
		drawLabel(font, "Kills required (0 = at least one)", labelAbove(l.row2))
		drawTextField(font, l.row2, numFieldText(s.focus == focusDialogCondFoeKills, c.FoeKills, s.dialogNumBuf), s.focus == focusDialogCondFoeKills)
	case core.DialogCondTileVisited:
		drawLabel(font, "Tile X", labelAbove(l.row1))
		drawTextField(font, l.row1, numFieldText(s.focus == focusDialogCondTileX, c.TileX, s.dialogNumBuf), s.focus == focusDialogCondTileX)
		drawLabel(font, "Tile Z", labelAbove(l.row2))
		drawTextField(font, l.row2, numFieldText(s.focus == focusDialogCondTileZ, c.TileZ, s.dialogNumBuf), s.focus == focusDialogCondTileZ)
	default:
		panic("editor: drawDialogCondEditModal has no case for dialog condition kind " + string(c.Kind))
	}

	drawLabel(font, "Disabled message (blank = auto reason)", labelAbove(l.msgField))
	drawTextField(font, l.msgField, c.DisabledMessage, s.focus == focusDialogCondMessage)
	drawButton(font, l.backBtn, "Back (Esc)", false)
}

func updateDialogCondEditModal(s *State) Action {
	c := currentDialogCond(s)
	if c == nil {
		closeModal(s)
		return ActionNone
	}
	l := dialogCondLayoutFor()

	if rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		mp := rl.GetMousePosition()
		switch {
		case pointIn(mp, l.kindBtn):
			s.focus = focusNone
			openDropdownBelow(s, ddDialogCondKind, l.kindBtn)
			return ActionNone
		case pointIn(mp, l.msgField):
			s.focus = focusDialogCondMessage
			return ActionNone
		case pointIn(mp, l.backBtn):
			returnToDialogChoiceEdit(s)
			return ActionNone
		}
		switch c.Kind {
		case core.DialogCondGold:
			if pointIn(mp, l.row1) {
				focusDialogNumeric(s, focusDialogCondGold, c.Gold)
				return ActionNone
			}
		case core.DialogCondQuest:
			if pointIn(mp, l.row1) {
				s.focus = focusDialogCondQuestID
				return ActionNone
			}
			if pointIn(mp, l.row2) {
				s.focus = focusNone
				openDropdownBelow(s, ddDialogQuestStatus, l.row2)
				return ActionNone
			}
		case core.DialogCondFoeKilled:
			if pointIn(mp, l.row1) {
				s.focus = focusNone
				openDropdownBelow(s, ddDialogCondFoe, l.row1)
				return ActionNone
			}
			if pointIn(mp, l.row2) {
				focusDialogNumeric(s, focusDialogCondFoeKills, c.FoeKills)
				return ActionNone
			}
		case core.DialogCondTileVisited:
			if pointIn(mp, l.row1) {
				focusDialogNumeric(s, focusDialogCondTileX, c.TileX)
				return ActionNone
			}
			if pointIn(mp, l.row2) {
				focusDialogNumeric(s, focusDialogCondTileZ, c.TileZ)
				return ActionNone
			}
		default:
			panic("editor: updateDialogCondEditModal has no case for dialog condition kind " + string(c.Kind))
		}
		s.focus = focusNone
	}

	// Numeric field focused (shared buffer).
	if pumpDialogNumeric(s) {
		if editorCommitPressed() {
			s.focus = focusNone
			return ActionNone
		}
		if editorCancelPressed() {
			returnToDialogChoiceEdit(s)
			return ActionNone
		}
		return ActionNone
	}

	switch s.focus {
	case focusDialogCondQuestID:
		pumpFocusField(s, &c.QuestID)
	case focusDialogCondMessage:
		pumpFocusField(s, &c.DisabledMessage)
	default:
		if editorCancelPressed() {
			returnToDialogChoiceEdit(s)
		}
		return ActionNone
	}
	if editorCommitPressed() {
		s.focus = focusNone
		return ActionNone
	}
	if editorCancelPressed() {
		returnToDialogChoiceEdit(s)
	}
	return ActionNone
}

// ========================= modalDialogTriggerList ==========================

func dialogTriggerListCmds(s *State) (adds, actions []modalCmd) {
	adds = []modalCmd{
		{label: "+ Add trigger  (A)", run: func() Action { addDialogTrigger(s); return ActionNone }},
	}
	actions = []modalCmd{
		{label: "Edit  (Enter)", run: func() Action {
			if len(s.area.Triggers) > 0 {
				openDialogTriggerEditModal(s, s.modalCursor)
			}
			return ActionNone
		}},
		{label: "Delete  (X)", run: func() Action { removeDialogTriggerAt(s, s.modalCursor); return ActionNone }},
	}
	return adds, actions
}

func openDialogTriggerListModal(s *State) {
	s.modal = modalDialogTriggerList
	restoreModalCursor(s, s.modalDialogTriggerIdx, len(s.area.Triggers))
	clearDialogFocus(s)
}

// dialogTriggerListSpec builds the spec for the area's trigger list.
func dialogTriggerListSpec(s *State) dialogListModalSpec {
	return dialogListModalSpec{
		title:    "DIALOG TRIGGERS",
		hint:     "Auto-start a dialog on a world event · Enter edit · A add · X delete · Esc back",
		empty:    "(no triggers — Add one)",
		count:    len(s.area.Triggers),
		rowLabel: func(i int) string { return triggerSummary(s.area.Triggers[i]) },
		cmds:     dialogTriggerListCmds,
		commit:   func() { openDialogTriggerEditModal(s, s.modalCursor) },
		cancel:   func() { openDialogListModal(s) }, // step UP to the dialog list
		add:      func() { addDialogTrigger(s) },
		del:      func() { removeDialogTriggerAt(s, s.modalCursor) },
	}
}

func drawDialogTriggerListModal(s *State, font rl.Font, theme render.Theme) {
	drawDialogListModalGeneric(s, font, theme, dialogTriggerListSpec(s))
}

func updateDialogTriggerListModal(s *State) Action {
	return updateDialogListModalGeneric(s, dialogTriggerListSpec(s))
}

// ========================= modalDialogTriggerEdit ==========================

const (
	dialogTrigModalW = float32(540)
	dialogTrigModalH = float32(400)
)

type dialogTrigLayout struct {
	card       rl.Rectangle
	kindBtn    rl.Rectangle
	dialogBtn  rl.Rectangle
	onceToggle rl.Rectangle
	row1       rl.Rectangle
	row2       rl.Rectangle
	backBtn    rl.Rectangle
}

func dialogTrigLayoutFor() dialogTrigLayout {
	r := centeredCardRect(dialogTrigModalW, dialogTrigModalH)
	x := r.X + modalContentInset
	fw := r.Width - 2*modalContentInset
	fieldH := dialogFieldH
	y := r.Y + dialogHeaderInset
	fields := stackRows(x, y, fw, fieldH, dialogTrigRowGap, 5)
	kindBtn := fields[0]
	dialogBtn := fields[1]
	onceToggle := fields[2]
	row1 := fields[3]
	row2 := fields[4]
	backBtn := bottomRightBtn(r)
	return dialogTrigLayout{card: r, kindBtn: kindBtn, dialogBtn: dialogBtn, onceToggle: onceToggle, row1: row1, row2: row2, backBtn: backBtn}
}

func openDialogTriggerEditModal(s *State, idx int) {
	if idx < 0 || idx >= len(s.area.Triggers) {
		return
	}
	s.modal = modalDialogTriggerEdit
	s.modalDialogTriggerIdx = idx
	s.focus = focusNone
}

func returnToDialogTriggerList(s *State) {
	s.modal = modalDialogTriggerList
	restoreModalCursor(s, s.modalDialogTriggerIdx, len(s.area.Triggers))
	clearDialogFocus(s)
}

func drawDialogTriggerEditModal(s *State, font rl.Font, theme render.Theme) {
	t := currentDialogTrigger(s)
	if t == nil {
		return
	}
	l := dialogTrigLayoutFor()
	drawModalHeaderAt(font, theme, l.card, "TRIGGER "+t.ID, theme.BorderActive)

	drawLabel(font, "Kind (click to choose)", labelAbove(l.kindBtn))
	drawButton(font, l.kindBtn, triggerKindLabel(t.Kind)+dropdownArrowSuffix, false)

	drawLabel(font, "Start dialog (click to choose)", labelAbove(l.dialogBtn))
	dlabel := t.DialogID
	if dlabel == "" {
		dlabel = "(pick a dialog)"
	}
	drawButton(font, l.dialogBtn, dlabel+dropdownArrowSuffix, false)

	drawButton(font, l.onceToggle, "Fire once (M): "+render.OnOffLabel(t.Once), t.Once)

	switch t.Kind {
	case core.DialogTriggerEnterTile:
		drawLabel(font, "Tile X", labelAbove(l.row1))
		drawTextField(font, l.row1, numFieldText(s.focus == focusDialogTrigTileX, t.TileX, s.dialogNumBuf), s.focus == focusDialogTrigTileX)
		drawLabel(font, "Tile Z", labelAbove(l.row2))
		drawTextField(font, l.row2, numFieldText(s.focus == focusDialogTrigTileZ, t.TileZ, s.dialogNumBuf), s.focus == focusDialogTrigTileZ)
	case core.DialogTriggerFoeKilled:
		drawLabel(font, "Foe (click to choose)", labelAbove(l.row1))
		drawButton(font, l.row1, core.FoeKindName(t.FoeKind)+dropdownArrowSuffix, false)
		drawLabel(font, "Kills required (0 = at least one)", labelAbove(l.row2))
		drawTextField(font, l.row2, numFieldText(s.focus == focusDialogTrigFoeKills, t.FoeKills, s.dialogNumBuf), s.focus == focusDialogTrigFoeKills)
	default:
		panic("editor: drawDialogTriggerEditModal has no case for dialog trigger kind " + string(t.Kind))
	}
	drawButton(font, l.backBtn, "Back (Esc)", false)
}

func updateDialogTriggerEditModal(s *State) Action {
	t := currentDialogTrigger(s)
	if t == nil {
		closeModal(s)
		return ActionNone
	}
	l := dialogTrigLayoutFor()

	if rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		mp := rl.GetMousePosition()
		switch {
		case pointIn(mp, l.kindBtn):
			s.focus = focusNone
			openDropdownBelow(s, ddDialogTriggerKind, l.kindBtn)
			return ActionNone
		case pointIn(mp, l.dialogBtn):
			s.focus = focusNone
			openDropdownBelow(s, ddDialogTriggerDialog, l.dialogBtn)
			return ActionNone
		case pointIn(mp, l.onceToggle):
			toggleTriggerOnce(s)
			return ActionNone
		case pointIn(mp, l.backBtn):
			returnToDialogTriggerList(s)
			return ActionNone
		}
		switch t.Kind {
		case core.DialogTriggerEnterTile:
			if pointIn(mp, l.row1) {
				focusDialogNumeric(s, focusDialogTrigTileX, t.TileX)
				return ActionNone
			}
			if pointIn(mp, l.row2) {
				focusDialogNumeric(s, focusDialogTrigTileZ, t.TileZ)
				return ActionNone
			}
		case core.DialogTriggerFoeKilled:
			if pointIn(mp, l.row1) {
				s.focus = focusNone
				openDropdownBelow(s, ddDialogTriggerFoe, l.row1)
				return ActionNone
			}
			if pointIn(mp, l.row2) {
				focusDialogNumeric(s, focusDialogTrigFoeKills, t.FoeKills)
				return ActionNone
			}
		default:
			panic("editor: updateDialogTriggerEditModal has no case for dialog trigger kind " + string(t.Kind))
		}
		s.focus = focusNone
	}

	if pumpDialogNumeric(s) {
		if editorCommitPressed() {
			s.focus = focusNone
			return ActionNone
		}
		if editorCancelPressed() {
			returnToDialogTriggerList(s)
			return ActionNone
		}
		return ActionNone
	}
	if editorCancelPressed() {
		returnToDialogTriggerList(s)
		return ActionNone
	}
	if rl.IsKeyPressed(rl.KeyM) {
		toggleTriggerOnce(s)
	}
	return ActionNone
}

func toggleTriggerOnce(s *State) {
	if t := currentDialogTrigger(s); t != nil {
		pushUndo(s)
		t.Once = !t.Once
		s.dirty = true
	}
}

// --- small shared helpers --------------------------------------------------

// labelAbove returns the label rect sitting just above a field rect.
func labelAbove(field rl.Rectangle) rl.Rectangle {
	return rl.NewRectangle(field.X, field.Y-16, field.Width, 14)
}

// bottomRightBtn returns the bottom-right "Back (Esc)" button rect for a modal
// card — a wide button inset from the card's right edge and pinned above the
// bottom inset. The action / condition / trigger edit modals all place their
// Back button here, so they share this rather than re-spelling the geometry.
func bottomRightBtn(card rl.Rectangle) rl.Rectangle {
	by := card.Y + card.Height - modalBtnH - modalBottomInset
	return rl.NewRectangle(card.X+card.Width-modalWideBtnW-modalContentInset, by, modalWideBtnW, modalBtnH)
}

// drawScrollMoreHint draws a "▲ N more" / "▼ N more" caption at (x,y) when a
// scroll window hides `hidden` rows above (up=true) or below it. No-op when
// nothing is hidden. Shared by the node editor's choice list and the choice
// editor's condition list so the affordance can't drift between them.
func drawScrollMoreHint(font rl.Font, theme render.Theme, x, y float32, hidden int, up bool) {
	if hidden <= 0 {
		return
	}
	arrow := "▼"
	if up {
		arrow = "▲"
	}
	render.DrawRichText(font, fmt.Sprintf("%s %d more", arrow, hidden), rl.NewVector2(x, y), editorFontHint, 1, theme.TextHint)
}

// drawScrollList draws a scrolling row list's body: the "▲ N more" hint above,
// each visible row (the cursored row highlighted with a "> " prefix and the
// active colour, others muted), and the "▼ N more" hint below. `rows` are the
// visible row rects, `top` the index of the first visible item, `count` the
// total item count (for the below-hint's hidden tally), and `cursor` the
// selected item index. `hintX` positions both hints; `upHintY` / `downHintY`
// place them. `rowText(idx)` yields the untruncated text for item idx, trimmed
// to `truncLen` runes. Shared by the node editor's choice list and the choice
// editor's condition list so their visuals can't drift apart.
func drawScrollList(font rl.Font, theme render.Theme, rows []rl.Rectangle, top, count, cursor, truncLen int,
	hintX, upHintY, downHintY float32, rowText func(idx int) string) {
	if len(rows) > 0 {
		drawScrollMoreHint(font, theme, hintX, upHintY, top, true)
	}
	for i, row := range rows {
		idx := top + i
		col := theme.TextMuted
		text := rowText(idx)
		if idx == cursor {
			col = theme.BorderActive
			text = "> " + text
		}
		render.DrawTextWithShadow(font, truncateDialog(text, truncLen), row.X+entityListTextInset, row.Y, editorFontBody, col)
	}
	drawScrollMoreHint(font, theme, hintX, downHintY, count-(top+len(rows)), false)
}

func truncateDialog(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 1 {
		return string(r[:max])
	}
	return string(r[:max-1]) + "…"
}

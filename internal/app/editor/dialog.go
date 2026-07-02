package editor

import (
	"crawler/internal/app/core"
	"crawler/internal/app/input"
	"crawler/internal/app/render"
	"fmt"
	"strconv"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// dialog.go is the branching-conversation authoring surface: a drill-down of
// dialog list → nodes → node edit → choice edit → action/condition/trigger
// editors. Esc steps UP one level (top-level Esc closes); closeModal resets every
// dialog index. Node/choice IDs are auto-generated and stable, so no rename hazard.

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

// clearDialogFocus drops dialog text-field focus so it can't pump into a freed field.
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

// currentDialogCond returns the condition editor's target, or nil if out of range.
func currentDialogCond(s *State) *core.DialogChoiceCondition {
	c := currentDialogChoice(s)
	if c == nil || s.modalDialogCondIdx < 0 || s.modalDialogCondIdx >= len(c.Conditions) {
		return nil
	}
	return &c.Conditions[s.modalDialogCondIdx]
}

func dialogCondInRange(s *State) bool { return currentDialogCond(s) != nil }

// currentDialogTrigger returns the trigger editor's target, or nil if out of range.
func currentDialogTrigger(s *State) *core.DialogTrigger {
	if s.modalDialogTriggerIdx < 0 || s.modalDialogTriggerIdx >= len(s.area.Triggers) {
		return nil
	}
	return &s.area.Triggers[s.modalDialogTriggerIdx]
}

func dialogTriggerInRange(s *State) bool { return currentDialogTrigger(s) != nil }

// uniqueID returns the first "prefix1", "prefix2", … not taken (callers supply the predicate).
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

// --- ops (each pushes undo + marks dirty) ---
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
	if d.StartNodeID == d.Nodes[idx].ID {
		return // already the start node — no undo/dirty/flash churn
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

// restoreModalCursor points the cursor at idx when in [0,count), else 0.
func restoreModalCursor(s *State, idx, count int) {
	if idx >= 0 && idx < count {
		s.modalCursor = idx
	} else {
		s.modalCursor = 0
	}
}

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

// returnToDialogNodes steps back up from the node editor.
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

// returnToDialogNodeEdit steps back up from the choice editor.
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
	// Start UNfocused so Up/Down drive the list and Tab steps into the fields.
	s.focus = focusNone
}

func dialogSpeakerEntries(s *State) []dropdownEntry {
	return fieldEntries(core.DialogSpeakerIDs(), core.DialogSpeakerName, func(s *State, id core.DialogSpeakerID) {
		if n := currentDialogNode(s); n != nil {
			setIfChanged(s, &n.SpeakerID, id)
		}
	})
}

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
	case core.DialogTriggerEnterLocation:
		return "Enter location"
	case core.DialogTriggerFoeKilled:
		return "Foe killed"
	}
	return string(k)
}

// locationLabel is a region's display name (Name if authored, else its ID).
func locationLabel(loc core.Location) string {
	if loc.Name != "" {
		return loc.Name
	}
	return loc.ID
}

// triggerLocationButtonLabel resolves a trigger's LocationID to a display label for
// the picker button (flags a dangling reference, like bg2's "Missing").
func triggerLocationButtonLabel(s *State, id string) string {
	if id == "" {
		return "(pick a location)"
	}
	if loc, ok := core.LocationByID(s.area.Locations, id); ok {
		return locationLabel(loc)
	}
	return id + " (missing)"
}

// init panics if any authorable condition/trigger kind lacks an editor label
// (otherwise condKindLabel/triggerKindLabel fall back to the raw kind string).
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
	// Every authorable end-action kind must be reachable from the action picker's
	// setters (a kind added to core without a picker row would be uneditable).
	for _, k := range core.DialogActionKinds() {
		found := false
		for _, set := range dialogActionSetters {
			var probe core.DialogAction
			set(&probe)
			if probe.Kind == k {
				found = true
				break
			}
		}
		if !found {
			panic("editor: dialog action picker missing a row for action kind " + string(k))
		}
	}
	// Every quest op must likewise be reachable from a quest-kind picker row (a quest
	// op added to core without a picker row would be uneditable + mislabeled).
	for _, op := range core.DialogQuestOps() {
		found := false
		for _, set := range dialogActionSetters {
			var probe core.DialogAction
			set(&probe)
			if probe.Kind == core.DialogActionQuest && probe.QuestOp == op {
				found = true
				break
			}
		}
		if !found {
			panic("editor: dialog action picker missing a row for quest op " + string(op))
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
		return fmt.Sprintf("Visited tile %s", core.TileCoord(c.TileX, c.TileZ))
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
	case core.DialogTriggerEnterLocation:
		return fmt.Sprintf("Enter [%s] → %s%s", t.LocationID, t.DialogID, once)
	case core.DialogTriggerFoeKilled:
		return fmt.Sprintf("Kill %s ×%d → %s%s", core.FoeKindName(t.FoeKind), core.RequiredFoeKills(t.FoeKills), t.DialogID, once)
	}
	return string(t.Kind)
}

func dialogCondKindEntries(s *State) []dropdownEntry {
	return fieldEntries(core.DialogCondKinds(), condKindLabel, func(s *State, k core.DialogCondKind) {
		// Only on a real kind CHANGE; reset to a clean condition so a stale value from
		// the old kind can't serialize. DisabledMessage carries over.
		if c := currentDialogCond(s); c != nil && c.Kind != k {
			pushUndo(s)
			*c = core.DialogChoiceCondition{Kind: k, DisabledMessage: c.DisabledMessage}
			s.dirty = true
		}
	})
}

func dialogQuestStatusEntries(s *State) []dropdownEntry {
	return fieldEntries([]core.QuestStatus{core.QuestActive, core.QuestComplete}, questStatusLabel, func(s *State, qs core.QuestStatus) {
		if c := currentDialogCond(s); c != nil {
			setIfChanged(s, &c.QuestStatus, qs)
		}
	})
}

func dialogCondFoeEntries(s *State) []dropdownEntry {
	return enemyKindEntries(func(s *State, kind core.EnemyKind) {
		if c := currentDialogCond(s); c != nil {
			setIfChanged(s, &c.FoeKind, kind)
		}
	})
}

func dialogTriggerKindEntries(s *State) []dropdownEntry {
	return fieldEntries(core.DialogTriggerKinds(), triggerKindLabel, func(s *State, k core.DialogTriggerKind) {
		// Like dialogCondKindEntries: only on a real CHANGE, reset to a clean trigger
		// so old params can't serialize. ID/DialogID/Once carry over.
		if t := currentDialogTrigger(s); t != nil && t.Kind != k {
			pushUndo(s)
			*t = core.DialogTrigger{ID: t.ID, Kind: k, DialogID: t.DialogID, Once: t.Once}
			s.dirty = true
		}
	})
}

func dialogTriggerDialogEntries(s *State) []dropdownEntry {
	return fieldEntries(s.area.Dialogs, func(d core.DialogDefinition) string { return d.ID }, func(s *State, d core.DialogDefinition) {
		if t := currentDialogTrigger(s); t != nil && t.DialogID != d.ID {
			pushUndo(s)
			t.DialogID = d.ID
			s.dirty = true
		}
	})
}

func dialogTriggerFoeEntries(s *State) []dropdownEntry {
	return enemyKindEntries(func(s *State, kind core.EnemyKind) {
		if t := currentDialogTrigger(s); t != nil {
			setIfChanged(s, &t.FoeKind, kind)
		}
	})
}

// dialogTriggerLocationEntries lists the area's regions for an enterLocation trigger.
func dialogTriggerLocationEntries(s *State) []dropdownEntry {
	return fieldEntries(s.area.Locations, locationLabel, func(s *State, loc core.Location) {
		if t := currentDialogTrigger(s); t != nil && t.LocationID != loc.ID {
			pushUndo(s)
			t.LocationID = loc.ID
			s.dirty = true
		}
	})
}

// Shared numeric-field editing: dialogNumericTarget + focusDialogNumeric +
// pumpDialogNumeric (backed by dialogNumBuf) is the canonical way to type into a
// plain int field. Distinct from updateNumericInput (input.go, map width/height).

// dialogNumericTarget returns the focused numeric field's int, or nil if none.
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

// focusDialogNumeric focuses a numeric field and seeds the edit buffer from its
// value. Snapshots the pre-edit area for pumpDialogNumeric's lazy undo step.
func focusDialogNumeric(s *State, focus focusField, value int) {
	s.focus = focus
	s.dialogNumBuf = strconv.Itoa(value)
	s.dialogNumUndoBefore = core.CloneArea(s.area)
	s.dialogNumSnapDone = false
}

// pumpDialogNumeric routes typed digits into the focused numeric field (empty = 0).
// Returns true while a numeric field owns input. Banks ONE undo step on the first
// keystroke that changes the buffer (lazy, like a paint stroke) so Ctrl+Z steps
// back to the pre-edit value instead of skipping past it.
func pumpDialogNumeric(s *State) bool {
	target := dialogNumericTarget(s)
	if target == nil {
		return false
	}
	before := s.dialogNumBuf
	pumpFocusField(s, &s.dialogNumBuf)
	if s.dialogNumBuf != before && !s.dialogNumSnapDone {
		commitUndoSnapshot(s, s.dialogNumUndoBefore)
		s.dialogNumSnapDone = true
	}
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

// --- condition / trigger ops (each pushes undo + marks dirty) ---
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

// dialogListModalSpec parameterizes the three list-style dialog modals (dialog
// list, node list, trigger list) through one draw + update driver.
type dialogListModalSpec struct {
	title    string             // modal header
	hint     string             // hint line below the header
	empty    string             // placeholder when the list is empty
	count    int                // number of rows
	rowLabel func(i int) string // one row's label
	cmds     func(*State) (adds, actions []modalCmd)
	commit   func() // Enter / "Edit" (only when count > 0)
	cancel   func() // Esc (step up / close)
	add      func() // A / "+ Add"
	del      func() // X / "Delete" (only when count > 0)
	// extraKeys: shortcuts beyond Enter/A/X; guarded means the key needs count > 0.
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
	if editorAddPressed() {
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
	if count > 0 && editorDeletePressed() {
		spec.del()
	}
	return ActionNone
}

// dialogListSpec builds the top-level dialog-list spec. Each handler is declared
// once and fed to both the button cmds and the keyboard fields so they can't drift.
func dialogListSpec(s *State) dialogListModalSpec {
	edit := func() {
		if len(s.area.Dialogs) > 0 {
			openDialogNodesModal(s, s.modalCursor)
		}
	}
	add := func() { addDialog(s) }
	del := func() { removeDialogAt(s, s.modalCursor) }
	triggers := func() { openDialogTriggerListModal(s) }
	return dialogListModalSpec{
		title:    "DIALOGS",
		hint:     "Enter edit · A add · X delete · T triggers · Esc close",
		empty:    "(no dialogs — Add one)",
		count:    len(s.area.Dialogs),
		rowLabel: func(i int) string { return dialogListRowLabel(s.area.Dialogs[i]) },
		cmds: func(*State) (adds, actions []modalCmd) {
			return []modalCmd{
					{label: "+ Add dialog  (A)", run: func() Action { add(); return ActionNone }},
				}, []modalCmd{
					{label: "Edit  (Enter)", run: func() Action { edit(); return ActionNone }},
					{label: "Delete  (X)", run: func() Action { del(); return ActionNone }},
					{label: "Triggers  (T)", run: func() Action { triggers(); return ActionNone }},
				}
		},
		commit:    edit,
		cancel:    func() { closeModal(s) },
		add:       add,
		del:       del,
		extraKeys: []dialogListKey{{key: rl.KeyT, run: triggers}},
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
	return fmt.Sprintf("%s%s · %s: %s", mark, n.ID, core.DialogSpeakerName(n.SpeakerID), truncateDialog(n.Text, dialogNodeSummaryTruncLen))
}

func updateDialogNodesModal(s *State) Action {
	d := currentDialog(s)
	if d == nil {
		closeModal(s)
		return ActionNone
	}
	return updateDialogListModalGeneric(s, dialogNodesSpec(s, d))
}

// Shared field-layout metrics for the dialog edit modals (stack fixed-height rows).
const (
	dialogFieldH       = textFieldH   // text-field / button row height (shared editor input height)
	dialogHeaderInset  = float32(56)  // first row's offset below the title
	dialogRowGap       = float32(46)  // row pitch (node + choice editors)
	dialogCondRowGap   = float32(54)  // row pitch in the condition editor
	dialogTrigRowGap   = float32(52)  // row pitch in the trigger editor
	dialogActionRowGap = float32(56)  // row pitch in the action editor
	dialogListRowH     = dropdownRowH // scrollable list rows share the dropdown's pitch
	scrollMoreHintGap  = float32(16)  // gap above the first row for the ▲ "N more" hint
	dialogStackTailGap = float32(6)   // extra gap below a stacked field group before the next content
)

// stackRows lays out n equal-height rows from (x,y) at pitch rowGap (shared with
// the door editor). Callers stacking below advance y by n*rowGap.
func stackRows(x, y, fw, fieldH, rowGap float32, n int) []rl.Rectangle {
	rows := make([]rl.Rectangle, n)
	for i := range rows {
		rows[i] = rl.NewRectangle(x, y+float32(i)*rowGap, fw, fieldH)
	}
	return rows
}

// scrollRows lays out a scrolling list's VISIBLE rows (≤ visible, cursor in view),
// one rowH-tall rect each from (x,y). Returns the window's top index + the rects.
func scrollRows(x, y, fw, rowH float32, cursor, count, visible int) (top int, rows []rl.Rectangle) {
	top, end := scrollWindow(cursor, count, visible)
	rows = make([]rl.Rectangle, end-top)
	for i := range rows {
		rows[i] = rl.NewRectangle(x, y+float32(i)*rowH, fw, rowH)
	}
	return top, rows
}

// Shared dialog-editor card widths: the standard width and a narrower variant for the
// choice/action editors. Each modal's own *ModalW aliases one of these so retuning "the
// dialog card width" is a single edit instead of five literals.
const (
	dialogEditModalW       = float32(540)
	dialogEditModalNarrowW = float32(520)
)

// --- modalDialogNodeEdit ---
const (
	dialogNodeModalW = dialogEditModalW
	dialogNodeModalH = float32(600)
	// dialogChoiceVisible caps visible choice rows; longer lists scroll.
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
	// choiceRows are the VISIBLE choice-row rects; row i maps to choice choiceTop+i.
	choiceTop     int
	choiceRows    []rl.Rectangle
	addChoiceBtn  rl.Rectangle
	editChoiceBtn rl.Rectangle
	delChoiceBtn  rl.Rectangle
	backBtn       rl.Rectangle
}

func dialogNodeLayoutFor(cursor, choiceCount int) dialogNodeLayout {
	r := centeredCardRect(dialogNodeModalW, dialogNodeModalH)
	x, fw := cardContentBox(r)
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
	y += 6*rowGap + dialogStackTailGap
	// Choice list rows (scroll window over the full list).
	top, rows := scrollRows(x, y, fw, dialogListRowH, cursor, choiceCount, dialogChoiceVisible)
	// Choice action buttons + Back along the bottom.
	by := modalFooterButtonY(r)
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
		listHeaderBelow(l.card, l.actionBtn))
	drawScrollList(font, theme, l.choiceRows, l.choiceTop, len(n.Choices), s.modalCursor, dialogChoiceRowTruncLen,
		l.card.X+entityListTextInset, l.addChoiceBtn.Y-18,
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
			openFieldDropdown(s, ddDialogSpeaker, l.speakerBtn)
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

	// Text field focused: Enter commits (defocus); Esc steps back up to the node list.
	if target := dialogNodeTextTarget(s); target != nil {
		pumpFocusedTextField(s, target, func() { cycleDialogNodeFocus(s) }, func() { returnToDialogNodes(s) })
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
	if editorAddPressed() {
		addDialogChoice(s)
		return ActionNone
	}
	if len(n.Choices) > 0 && editorDeletePressed() {
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

// dialogNodeTextTarget returns the focused node text field, or nil if none.
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

// --- modalDialogChoiceEdit ---
const (
	dialogChoiceModalW = dialogEditModalNarrowW
	dialogChoiceModalH = float32(470)
	// dialogCondVisible caps visible condition rows; longer lists scroll.
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
	x, fw := cardContentBox(r)
	fieldH := dialogFieldH
	rowGap := dialogRowGap
	y := r.Y + dialogHeaderInset
	fields := stackRows(x, y, fw, fieldH, rowGap, 3)
	labelField := fields[0]
	nextField := fields[1]
	actionBtn := fields[2]
	y += 3*rowGap + 8 // gap for the "Conditions (N)" header
	top, rows := scrollRows(x, y, fw, dialogListRowH, cursor, condCount, dialogCondVisible)
	by := modalFooterButtonY(r)
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

	header := listHeaderBelow(l.card, l.actionBtn)
	drawLabel(font, fmt.Sprintf("Conditions (%d) — gate selectability; Up/Down select, Enter edit", len(c.Conditions)), header)
	drawScrollList(font, theme, l.condRows, l.condTop, len(c.Conditions), s.modalCursor, dialogCondRowTruncLen,
		l.card.X+entityListTextInset, l.addCondBtn.Y-16,
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

	var choiceTarget *string
	switch s.focus {
	case focusDialogChoiceLabel:
		choiceTarget = &c.Label
	case focusDialogChoiceNext:
		choiceTarget = &c.NextNodeID
	}
	if choiceTarget != nil {
		// A text field is focused. Tab cycles; Enter commits (defocus); Esc steps up.
		pumpFocusedTextField(s, choiceTarget, func() { cycleDialogChoiceFocus(s) }, func() { returnToDialogNodeEdit(s) })
		return ActionNone
	}

	// No field focused — list nav + shortcuts.
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
	if editorAddPressed() {
		addDialogCond(s)
		return ActionNone
	}
	if len(c.Conditions) > 0 && editorDeletePressed() {
		removeDialogCond(s, s.modalCursor)
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
	dialogActionModalW = dialogEditModalNarrowW
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
	x, fw := cardContentBox(r)
	fieldH := dialogFieldH
	y := r.Y + dialogHeaderInset
	fields := stackRows(x, y, fw, fieldH, dialogActionRowGap, 2)
	kindBtn := fields[0]
	idField := fields[1]
	backBtn := bottomRightBtn(r)
	return dialogActionLayout{card: r, kindBtn: kindBtn, idField: idField, backBtn: backBtn}
}

// currentDialogActionHolder returns the EndAction field the action editor targets
// (choice's when modalDialogActionOnChoice, else node's), nil if out of range.
// Double pointer so the picker can nil it or allocate a fresh one.
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

// dialogActionIDTarget returns the action's id field (quest id or event id), or
// nil when the current action has none.
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

// dialogActionSetters FULLY specify each non-None action-picker row's payload
// (clearing the other kind's id so a stale QuestID/EventID can't linger). Shared by
// the picker builder and the init() coverage assert against core.DialogActionKinds.
var dialogActionSetters = []func(*core.DialogAction){
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

// dialogActionKindEntries are the action picker's rows (None + flattened
// start/complete/event). set == nil is "None"; labels derive from dialogActionLabel
// against a probe so they can't drift.
func dialogActionKindEntries(s *State) []dropdownEntry {
	sets := append([]func(*core.DialogAction){nil}, dialogActionSetters...)
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
			// No-change guard: re-picking the action the holder already has shouldn't
			// bank a no-op undo or dirty the map (mirrors the sibling field pickers).
			if set == nil {
				if *holder == nil {
					return
				}
			} else {
				var probe core.DialogAction
				set(&probe)
				if *holder != nil && **holder == probe {
					return
				}
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
// editor (its list cursor is untouched).
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
			openFieldDropdown(s, ddDialogActionKind, l.kindBtn)
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
		pumpFocusedTextField(s, dialogActionIDTarget(s), nil, func() { returnFromDialogActionEdit(s) })
		return ActionNone
	}
	if editorCancelPressed() {
		returnFromDialogActionEdit(s)
	}
	return ActionNone
}

// ========================= modalDialogCondEdit =============================

const (
	dialogCondModalW = dialogEditModalW
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
	x, fw := cardContentBox(r)
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

// returnToDialogChoiceEdit steps back up from the condition editor.
func returnToDialogChoiceEdit(s *State) {
	s.modal = modalDialogChoiceEdit
	count := 0
	if c := currentDialogChoice(s); c != nil {
		count = len(c.Conditions)
	}
	restoreModalCursor(s, s.modalDialogCondIdx, count)
	clearDialogFocus(s)
}

// dialogEditRowKind selects how one variable row of the cond/trigger edit modal
// renders and responds to a click.
type dialogEditRowKind int

const (
	rowNumeric  dialogEditRowKind = iota // numeric text field (shared s.dialogNumBuf)
	rowText                              // plain text field
	rowDropdown                          // button that opens a field dropdown
)

// dialogEditRow describes one variable row of the cond/trigger edit modal — enough
// for BOTH the draw pass and the click hit-test, so the per-kind field layout lives
// in one builder (dialogCondRows/dialogTrigRows) instead of a switch duplicated
// across draw+update. Add a cond/trigger kind ⇒ edit ONE builder, not four switches.
type dialogEditRow struct {
	kind  dialogEditRowKind
	label string
	focus focusField    // rowNumeric/rowText: the focus this row takes
	value int           // rowNumeric: current value (numFieldText + focusDialogNumeric)
	text  string        // rowText: field text · rowDropdown: button label
	dd    dropdownOwner // rowDropdown: which field dropdown opens
}

// dialogCondRows returns the variable rows for the current condition kind (draw+click).
func dialogCondRows(c *core.DialogChoiceCondition) []dialogEditRow {
	switch c.Kind {
	case core.DialogCondGold:
		return []dialogEditRow{{kind: rowNumeric, label: "Minimum gold the party must hold", focus: focusDialogCondGold, value: c.Gold}}
	case core.DialogCondQuest:
		return []dialogEditRow{
			{kind: rowText, label: "Quest id", focus: focusDialogCondQuestID, text: c.QuestID},
			{kind: rowDropdown, label: "Required status (click to choose)", text: questStatusLabel(c.QuestStatus), dd: ddDialogQuestStatus},
		}
	case core.DialogCondFoeKilled:
		return []dialogEditRow{
			{kind: rowDropdown, label: "Foe (click to choose)", text: core.FoeKindName(c.FoeKind), dd: ddDialogCondFoe},
			{kind: rowNumeric, label: "Kills required (0 = at least one)", focus: focusDialogCondFoeKills, value: c.FoeKills},
		}
	case core.DialogCondTileVisited:
		return []dialogEditRow{
			{kind: rowNumeric, label: "Tile X", focus: focusDialogCondTileX, value: c.TileX},
			{kind: rowNumeric, label: "Tile Z", focus: focusDialogCondTileZ, value: c.TileZ},
		}
	}
	panic("editor: dialogCondRows has no case for dialog condition kind " + string(c.Kind))
}

// dialogTrigRows returns the variable rows for the current trigger kind (draw+click).
func dialogTrigRows(s *State, t *core.DialogTrigger) []dialogEditRow {
	switch t.Kind {
	case core.DialogTriggerEnterTile:
		return []dialogEditRow{
			{kind: rowNumeric, label: "Tile X", focus: focusDialogTrigTileX, value: t.TileX},
			{kind: rowNumeric, label: "Tile Z", focus: focusDialogTrigTileZ, value: t.TileZ},
		}
	case core.DialogTriggerEnterLocation:
		return []dialogEditRow{{kind: rowDropdown, label: "Location (click to choose)", text: triggerLocationButtonLabel(s, t.LocationID), dd: ddDialogTriggerLocation}}
	case core.DialogTriggerFoeKilled:
		return []dialogEditRow{
			{kind: rowDropdown, label: "Foe (click to choose)", text: core.FoeKindName(t.FoeKind), dd: ddDialogTriggerFoe},
			{kind: rowNumeric, label: "Kills required (0 = at least one)", focus: focusDialogTrigFoeKills, value: t.FoeKills},
		}
	}
	panic("editor: dialogTrigRows has no case for dialog trigger kind " + string(t.Kind))
}

// drawDialogEditRows paints rows[i] into rects[i] (label above + the kind's widget).
func drawDialogEditRows(s *State, font rl.Font, rows []dialogEditRow, rects []rl.Rectangle) {
	for i, r := range rows {
		rect := rects[i]
		drawLabel(font, r.label, labelAbove(rect))
		switch r.kind {
		case rowNumeric:
			drawTextField(font, rect, numFieldText(s.focus == r.focus, r.value, s.dialogNumBuf), s.focus == r.focus)
		case rowText:
			drawTextField(font, rect, r.text, s.focus == r.focus)
		case rowDropdown:
			drawButton(font, rect, r.text+dropdownArrowSuffix, false)
		}
	}
}

// clickDialogEditRows dispatches a click at mp against rows[i]/rects[i], returning
// true (and acting) on the first hit. Mirror of drawDialogEditRows for the hit-test.
func clickDialogEditRows(s *State, mp rl.Vector2, rows []dialogEditRow, rects []rl.Rectangle) bool {
	for i, r := range rows {
		rect := rects[i]
		if !pointIn(mp, rect) {
			continue
		}
		switch r.kind {
		case rowNumeric:
			focusDialogNumeric(s, r.focus, r.value)
		case rowText:
			s.focus = r.focus
		case rowDropdown:
			openFieldDropdown(s, r.dd, rect)
		}
		return true
	}
	return false
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

	drawDialogEditRows(s, font, dialogCondRows(c), []rl.Rectangle{l.row1, l.row2})

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
			openFieldDropdown(s, ddDialogCondKind, l.kindBtn)
			return ActionNone
		case pointIn(mp, l.msgField):
			s.focus = focusDialogCondMessage
			return ActionNone
		case pointIn(mp, l.backBtn):
			returnToDialogChoiceEdit(s)
			return ActionNone
		}
		if clickDialogEditRows(s, mp, dialogCondRows(c), []rl.Rectangle{l.row1, l.row2}) {
			return ActionNone
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
	dialogTrigModalW = dialogEditModalW
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
	x, fw := cardContentBox(r)
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

	drawDialogEditRows(s, font, dialogTrigRows(s, t), []rl.Rectangle{l.row1, l.row2})
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
			openFieldDropdown(s, ddDialogTriggerKind, l.kindBtn)
			return ActionNone
		case pointIn(mp, l.dialogBtn):
			openFieldDropdown(s, ddDialogTriggerDialog, l.dialogBtn)
			return ActionNone
		case pointIn(mp, l.onceToggle):
			toggleTriggerOnce(s)
			return ActionNone
		case pointIn(mp, l.backBtn):
			returnToDialogTriggerList(s)
			return ActionNone
		}
		if clickDialogEditRows(s, mp, dialogTrigRows(s, t), []rl.Rectangle{l.row1, l.row2}) {
			return ActionNone
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

// Editor dialog-list layout tokens.
const (
	labelCaptionH   = float32(14) // field caption height
	labelCaptionGap = float32(16) // caption top above its field
	listHeaderGap   = float32(8)  // sub-list header caption below the action button

	// Per-list row text budgets (runes) — distinct values because the lists differ
	// in width, but named so they can't drift unintentionally.
	dialogNodeSummaryTruncLen = 24 // node summary line in the node list
	dialogChoiceRowTruncLen   = 52 // choice rows in the node modal
	dialogCondRowTruncLen     = 56 // condition rows in the condition modal
)

// labelAbove returns the label rect sitting just above a field rect.
func labelAbove(field rl.Rectangle) rl.Rectangle {
	return rl.NewRectangle(field.X, field.Y-labelCaptionGap, field.Width, labelCaptionH)
}

// listHeaderBelow returns the caption rect for a sub-list header sitting
// listHeaderGap below the action button, spanning the card width. Shared by the
// node (choices) and choice (conditions) editors so the gap can't drift.
func listHeaderBelow(card, actionBtn rl.Rectangle) rl.Rectangle {
	return rl.NewRectangle(card.X+modalContentInset, actionBtn.Y+actionBtn.Height+listHeaderGap, card.Width, labelCaptionH)
}

// bottomRightBtn returns the bottom-right "Back (Esc)" button rect, shared by the
// action / condition / trigger edit modals.
func bottomRightBtn(card rl.Rectangle) rl.Rectangle {
	by := modalFooterButtonY(card)
	return rl.NewRectangle(card.X+card.Width-modalWideBtnW-modalContentInset, by, modalWideBtnW, modalBtnH)
}

// bottomLeftBtn mirrors bottomRightBtn for a left-aligned action (e.g. Delete).
func bottomLeftBtn(card rl.Rectangle) rl.Rectangle {
	by := modalFooterButtonY(card)
	return rl.NewRectangle(card.X+modalContentInset, by, modalWideBtnW, modalBtnH)
}

// drawScrollMoreHint draws a "▲/▼ N more" caption (up=true above, else below); no-op when hidden<=0.
func drawScrollMoreHint(font rl.Font, theme render.Theme, x, y float32, hidden int, up bool) {
	if hidden <= 0 {
		return
	}
	render.DrawRichText(font, fmt.Sprintf("%s %d more", scrollArrowGlyph(up), hidden), rl.NewVector2(x, y), editorFontHint, 1, theme.TextHint)
}

// drawScrollList draws a scrolling list's body: the ▲/▼ "N more" hints + each
// visible row (cursored row gets a "> " prefix). rowText trimmed to truncLen runes.
func drawScrollList(font rl.Font, theme render.Theme, rows []rl.Rectangle, top, count, cursor, truncLen int,
	hintX, downHintY float32, rowText func(idx int) string) {
	if len(rows) > 0 {
		drawScrollMoreHint(font, theme, hintX, rows[0].Y-scrollMoreHintGap, top, true)
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

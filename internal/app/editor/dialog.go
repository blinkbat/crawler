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

// clearDialogFocus drops modal text-field focus so it can't pump into a freed field
// after the modal closes (a leaked focus freezes the top-level editor — updateTextInput
// swallows all input while s.focus != focusNone). The dialog/trigger/wall-feature foci
// are one contiguous enum block (focusDialogNodeText…focusWallFeatureSwitch in editor.go)
// — a range test, so a new focus added AT THE END OF that block is covered automatically.
// Keep new modal foci inside the block (the last member is the upper bound here).
func clearDialogFocus(s *State) {
	if s.focus >= focusDialogNodeText && s.focus <= focusWallFeatureSwitch {
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
func currentDialogTrigger(s *State) *core.Trigger {
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
	s.modalCursor = clampCursor(s.modalCursor, count)
}

// clampCursor pins a list cursor to [0,count-1] (0 for an empty list). Shared by
// every modal that tracks its own cursor field (modalCursor, prefabCursor, …).
func clampCursor(cur, count int) int {
	if cur >= count {
		cur = count - 1
	}
	if cur < 0 {
		cur = 0
	}
	return cur
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

// trigCond / trigAct return the editor's primary Condition[0] / Action[0] for the
// current trigger, allocating a default entry when the list is empty (the trigger
// editor edits the first condition and first action; extra entries are hand-authored
// / a future multi-row UI). nil when no trigger is selected.
func trigCond(s *State) *core.Condition {
	t := currentDialogTrigger(s)
	if t == nil {
		return nil
	}
	if len(t.Conditions) == 0 {
		t.Conditions = []core.Condition{{Kind: core.CondAlways}}
	}
	return &t.Conditions[0]
}

func trigAct(s *State) *core.Action {
	t := currentDialogTrigger(s)
	if t == nil {
		return nil
	}
	if len(t.Actions) == 0 {
		t.Actions = []core.Action{{Kind: core.ActionDialog}}
	}
	return &t.Actions[0]
}

// conditionKindLabel / actionKindLabel are the editor labels for the trigger engine's
// condition / action kinds (StarEdit-style). init asserts full coverage.
func conditionKindLabel(k core.ConditionKind) string {
	switch k {
	case core.CondAlways:
		return "Always"
	case core.CondNever:
		return "Never"
	case core.CondSwitch:
		return "Switch set"
	case core.CondCounter:
		return "Counter"
	case core.CondEnterTile:
		return "Party on tile"
	case core.CondAtLocation:
		return "Party in region"
	case core.CondFoeKilled:
		return "Foe killed"
	case core.CondTileVisited:
		return "Tile visited"
	case core.CondQuest:
		return "Quest status"
	case core.CondGold:
		return "Gold"
	}
	return string(k)
}

func actionKindLabel(k core.ActionKind) string {
	switch k {
	case core.ActionDialog:
		return "Start dialog"
	case core.ActionSetSwitch:
		return "Set switch"
	case core.ActionSetCounter:
		return "Set counter"
	case core.ActionSpawnFoe:
		return "Spawn foe"
	case core.ActionSpawnChest:
		return "Spawn chest"
	case core.ActionOpenWall:
		return "Open wall"
	case core.ActionTeleport:
		return "Teleport party"
	case core.ActionGiveGold:
		return "Give gold"
	case core.ActionQuest:
		return "Quest"
	case core.ActionMessage:
		return "Show message"
	case core.ActionEvent:
		return "Emit event"
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
		// condSummary is a parallel switch; guard it too, else a new kind silently
		// falls through to the raw kind string in the choice's condition list.
		if condSummary(core.DialogChoiceCondition{Kind: k}) == string(k) {
			panic("editor: condSummary missing a case for dialog condition kind " + string(k))
		}
	}
	// Every trigger-engine condition / action kind must have an editor label (the
	// kind pickers list all of them; a missing label would show the raw kind string).
	for _, k := range core.ConditionKinds() {
		if conditionKindLabel(k) == string(k) {
			panic("editor: conditionKindLabel missing a case for condition kind " + string(k))
		}
	}
	for _, k := range core.ActionKinds() {
		if actionKindLabel(k) == string(k) {
			panic("editor: actionKindLabel missing a case for action kind " + string(k))
		}
	}
	// The dialog end-action picker exposes a subset (quest/event); assert each of its
	// setters yields a labeled kind (not the "None" fallback).
	for _, set := range dialogActionSetters {
		var probe core.Action
		set(&probe)
		if dialogActionLabel(&probe) == "None" {
			panic("editor: dialogActionLabel missing a case for a dialog-action setter kind " + string(probe.Kind))
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

// conditionSummary / actionSummary are compact one-liners for the trigger list row.
func conditionSummary(c core.Condition) string {
	neg := ""
	if c.Negate {
		neg = "!"
	}
	switch c.Kind {
	case core.CondSwitch:
		return neg + "switch " + c.Switch
	case core.CondCounter:
		return fmt.Sprintf("%scounter %s%s%d", neg, c.Counter, cmpGlyph(c.Cmp), c.Count)
	case core.CondEnterTile:
		return fmt.Sprintf("%son (%d,%d)", neg, c.TileX, c.TileZ)
	case core.CondAtLocation:
		return neg + "in [" + c.LocationID + "]"
	case core.CondFoeKilled:
		return fmt.Sprintf("%skilled %s ×%d", neg, core.FoeKindName(c.FoeKind), core.RequiredFoeKills(c.Count))
	case core.CondTileVisited:
		return fmt.Sprintf("%svisited (%d,%d)", neg, c.TileX, c.TileZ)
	case core.CondQuest:
		return fmt.Sprintf("%squest %s=%s", neg, c.QuestID, questStatusLabel(c.QuestStatus))
	case core.CondGold:
		return fmt.Sprintf("%sgold%s%d", neg, cmpGlyph(c.Cmp), c.Count)
	}
	return neg + conditionKindLabel(c.Kind)
}

func actionSummary(a core.Action) string {
	switch a.Kind {
	case core.ActionDialog:
		return "dialog " + a.DialogID
	case core.ActionSetSwitch:
		return fmt.Sprintf("%s %s", a.SwitchOp, a.Switch)
	case core.ActionSetCounter:
		return fmt.Sprintf("counter %s %s %d", a.Counter, a.CounterOp, a.Count)
	case core.ActionSpawnFoe:
		return fmt.Sprintf("spawn %s @(%d,%d)", core.FoeKindName(a.FoeKind), a.TileX, a.TileZ)
	case core.ActionSpawnChest:
		return fmt.Sprintf("chest @(%d,%d)", a.TileX, a.TileZ)
	case core.ActionOpenWall:
		return fmt.Sprintf("open wall @(%d,%d)", a.TileX, a.TileZ)
	case core.ActionTeleport:
		return fmt.Sprintf("teleport @(%d,%d)", a.TileX, a.TileZ)
	case core.ActionGiveGold:
		return fmt.Sprintf("gold +%d", a.Count)
	case core.ActionQuest:
		return "quest " + a.QuestID
	case core.ActionMessage:
		return "say " + a.Text
	}
	return actionKindLabel(a.Kind)
}

func cmpGlyph(c core.Comparator) string {
	switch c {
	case core.CmpAtMost:
		return "≤"
	case core.CmpExactly:
		return "="
	default:
		return "≥"
	}
}

// triggerSummary is the one-line row label for the trigger list: first condition →
// first action, plus counts of any extra conditions/actions and a preserve marker.
func triggerSummary(t core.Trigger) string {
	cond := "always"
	if len(t.Conditions) > 0 {
		cond = conditionSummary(t.Conditions[0])
	}
	if len(t.Conditions) > 1 {
		cond += fmt.Sprintf(" +%d", len(t.Conditions)-1)
	}
	act := "—"
	if len(t.Actions) > 0 {
		act = actionSummary(t.Actions[0])
	}
	if len(t.Actions) > 1 {
		act += fmt.Sprintf(" +%d", len(t.Actions)-1)
	}
	tag := ""
	if t.Preserve {
		tag = " · preserve"
	}
	return cond + " → " + act + tag
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

// trigCondKindEntries / trigActKindEntries pick the primary condition / action kind,
// resetting that entry to a clean value on a real change (so stale params from the old
// kind can't serialize).
func trigCondKindEntries(s *State) []dropdownEntry {
	return fieldEntries(core.ConditionKinds(), conditionKindLabel, func(s *State, k core.ConditionKind) {
		if c := trigCond(s); c != nil && c.Kind != k {
			pushUndo(s)
			*c = core.Condition{Kind: k}
			s.dirty = true
		}
	})
}

func trigActKindEntries(s *State) []dropdownEntry {
	return fieldEntries(core.ActionKinds(), actionKindLabel, func(s *State, k core.ActionKind) {
		if a := trigAct(s); a != nil && a.Kind != k {
			pushUndo(s)
			*a = core.Action{Kind: k}
			s.dirty = true
		}
	})
}

// dialogTriggerDialogEntries picks the dialog for a Start-dialog ACTION.
func dialogTriggerDialogEntries(s *State) []dropdownEntry {
	return fieldEntries(s.area.Dialogs, func(d core.DialogDefinition) string { return d.ID }, func(s *State, d core.DialogDefinition) {
		if a := trigAct(s); a != nil {
			setIfChanged(s, &a.DialogID, d.ID)
		}
	})
}

// dialogTriggerFoeEntries picks the foe for a foeKilled CONDITION (the spawnFoe action
// uses the separate trigActFoeEntries / ddTrigActFoe).
func dialogTriggerFoeEntries(s *State) []dropdownEntry {
	return enemyKindEntries(func(s *State, kind core.EnemyKind) {
		if c := trigCond(s); c != nil && c.Kind == core.CondFoeKilled {
			setIfChanged(s, &c.FoeKind, kind)
		}
	})
}

// trigActFoeEntries picks the foe for a spawnFoe ACTION.
func trigActFoeEntries(s *State) []dropdownEntry {
	return enemyKindEntries(func(s *State, kind core.EnemyKind) {
		if a := trigAct(s); a != nil {
			setIfChanged(s, &a.FoeKind, kind)
		}
	})
}

// dialogTriggerLocationEntries lists the area's regions for an atLocation CONDITION.
func dialogTriggerLocationEntries(s *State) []dropdownEntry {
	return fieldEntries(s.area.Locations, locationLabel, func(s *State, loc core.Location) {
		if c := trigCond(s); c != nil {
			setIfChanged(s, &c.LocationID, loc.ID)
		}
	})
}

// trigSwitchOpEntries picks the setSwitch action's operation.
func trigSwitchOpEntries(s *State) []dropdownEntry {
	return fieldEntries(core.SwitchOps(), func(op core.SwitchOp) string { return string(op) }, func(s *State, op core.SwitchOp) {
		if a := trigAct(s); a != nil {
			setIfChanged(s, &a.SwitchOp, op)
		}
	})
}

// trigCounterOpEntries picks the setCounter action's operation (set / add).
func trigCounterOpEntries(s *State) []dropdownEntry {
	return fieldEntries(core.CounterOps(), func(op core.CounterOp) string { return string(op) }, func(s *State, op core.CounterOp) {
		if a := trigAct(s); a != nil {
			setIfChanged(s, &a.CounterOp, op)
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
		if c := trigCond(s); c != nil {
			return &c.TileX
		}
	case focusDialogTrigTileZ:
		if c := trigCond(s); c != nil {
			return &c.TileZ
		}
	case focusDialogTrigFoeKills:
		if c := trigCond(s); c != nil {
			return &c.Count
		}
	case focusTrigActTileX:
		if a := trigAct(s); a != nil {
			return &a.TileX
		}
	case focusTrigActTileZ:
		if a := trigAct(s); a != nil {
			return &a.TileZ
		}
	case focusTrigActCount:
		if a := trigAct(s); a != nil {
			return &a.Count
		}
	}
	return nil
}

// dialogTrigTextTarget returns the trigger editor's focused string field (a condition
// or action name/id/text), or nil. Pumped with pumpFocusField in the modal handler.
func dialogTrigTextTarget(s *State) *string {
	switch s.focus {
	case focusTrigCondText:
		c := trigCond(s)
		if c == nil {
			return nil
		}
		switch c.Kind {
		case core.CondSwitch:
			return &c.Switch
		case core.CondCounter:
			return &c.Counter
		case core.CondQuest:
			return &c.QuestID
		}
	case focusTrigActText:
		a := trigAct(s)
		if a == nil {
			return nil
		}
		switch a.Kind {
		case core.ActionSetSwitch:
			return &a.Switch
		case core.ActionSetCounter:
			return &a.Counter
		case core.ActionMessage:
			return &a.Text
		case core.ActionQuest:
			return &a.QuestID
		case core.ActionEvent:
			return &a.EventID
		}
	}
	return nil
}

// focusDialogNumeric focuses a numeric field and seeds the edit buffer from its
// value. Snapshots the pre-edit area for pumpDialogNumeric's lazy undo step.
func focusDialogNumeric(s *State, focus focusField, value int) {
	s.focus = focus
	s.dialogNumBuf = strconv.Itoa(value)
	s.dialogNumUndo.begin(s)
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
	if s.dialogNumBuf != before {
		s.dialogNumUndo.commitOnce(s)
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
	// A fresh trigger: one Always condition and one Start-dialog action (seeded to the
	// first dialog if any), the most common starting shape. Editable in the trigger modal.
	act := core.Action{Kind: core.ActionDialog}
	if len(s.area.Dialogs) > 0 {
		act.DialogID = s.area.Dialogs[0].ID
	}
	t := core.Trigger{
		ID:         uniqueTriggerID(s),
		Conditions: []core.Condition{{Kind: core.CondAlways}},
		Actions:    []core.Action{act},
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
	render.DrawTextWithShadow(font, spec.hint, r.X+modalContentInset, r.Y+modalSubheadingDY, editorFontTiny, theme.TextHint)
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

	// No field focused — shared list nav + shortcuts, then the node-only M toggle.
	if subListNav(s, len(n.Choices), focusDialogNodeText,
		func() { returnToDialogNodes(s) },
		func() { openDialogChoiceEditModal(s, s.modalCursor) },
		func() { addDialogChoice(s) },
		func() { removeDialogChoice(s, s.modalCursor) }) {
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

	// No field focused — shared list nav + shortcuts.
	if subListNav(s, len(c.Conditions), focusDialogChoiceLabel,
		func() { returnToDialogNodeEdit(s) },
		func() { openDialogCondEditModal(s, s.modalCursor) },
		func() { addDialogCond(s) },
		func() { removeDialogCond(s, s.modalCursor) }) {
		return ActionNone
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
func currentDialogActionHolder(s *State) **core.Action {
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

// dialogActionLabel is the human label for an end-action (nil = "None"). The dialog
// end-action UI exposes the quest/event kinds; other Action kinds (spawnFoe, openWall,
// …) are authored from the trigger editor.
func dialogActionLabel(a *core.Action) string {
	if a == nil {
		return "None"
	}
	switch a.Kind {
	case core.ActionQuest:
		if a.QuestOp == core.DialogQuestStart {
			return "Start quest"
		}
		return "Complete quest"
	case core.ActionEvent:
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
	case core.ActionQuest:
		return &(*holder).QuestID
	case core.ActionEvent:
		return &(*holder).EventID
	}
	return nil
}

// dialogActionSetters FULLY specify each non-None action-picker row's payload
// (clearing the other kind's id so a stale QuestID/EventID can't linger). Shared by
// the picker builder and the init() coverage assert.
var dialogActionSetters = []func(*core.Action){
	func(a *core.Action) {
		a.Kind = core.ActionQuest
		a.QuestOp = core.DialogQuestStart
		a.EventID = ""
	},
	func(a *core.Action) {
		a.Kind = core.ActionQuest
		a.QuestOp = core.DialogQuestComplete
		a.EventID = ""
	},
	func(a *core.Action) { a.Kind = core.ActionEvent; a.QuestOp = ""; a.QuestID = "" },
}

// dialogActionKindEntries are the action picker's rows (None + flattened
// start/complete/event). set == nil is "None"; labels derive from dialogActionLabel
// against a probe so they can't drift.
func dialogActionKindEntries(s *State) []dropdownEntry {
	sets := append([]func(*core.Action){nil}, dialogActionSetters...)
	out := make([]dropdownEntry, 0, len(sets))
	for _, set := range sets {
		set := set
		label := dialogActionLabel(nil) // "None"
		if set != nil {
			var probe core.Action
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
			// Probe by applying set to a COPY of the current action, not to a zero value:
			// a setter only rewrites the fields it owns (Kind/QuestOp), leaving QuestID/
			// EventID intact, so a zero-value probe would spuriously differ from a holder
			// that already carries an ID and bank a redundant undo/dirty on a true no-op.
			if set == nil {
				if *holder == nil {
					return
				}
			} else if *holder != nil {
				probe := **holder
				set(&probe)
				if core.ActionsEqual(probe, **holder) {
					return
				}
			}
			pushUndo(s)
			if set == nil {
				*holder = nil
			} else {
				if *holder == nil {
					*holder = &core.Action{}
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
		case core.ActionQuest:
			drawLabel(font, "Quest id", labelAbove(l.idField))
			drawTextField(font, l.idField, a.QuestID, s.focus == focusDialogActionID)
		case core.ActionEvent:
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

// trigCondRows returns the param rows for the trigger's primary CONDITION (its numeric
// foci route through dialogNumericTarget, text through dialogTrigTextTarget).
func trigCondRows(s *State, c *core.Condition) []dialogEditRow {
	switch c.Kind {
	case core.CondEnterTile, core.CondTileVisited:
		return []dialogEditRow{
			{kind: rowNumeric, label: "Tile X", focus: focusDialogTrigTileX, value: c.TileX},
			{kind: rowNumeric, label: "Tile Z", focus: focusDialogTrigTileZ, value: c.TileZ},
		}
	case core.CondFoeKilled:
		return []dialogEditRow{
			{kind: rowDropdown, label: "Foe (click to choose)", text: core.FoeKindName(c.FoeKind), dd: ddDialogTriggerFoe},
			{kind: rowNumeric, label: "Kills required (0 = at least one)", focus: focusDialogTrigFoeKills, value: c.Count},
		}
	case core.CondAtLocation:
		return []dialogEditRow{{kind: rowDropdown, label: "Region (click to choose)", text: triggerLocationButtonLabel(s, c.LocationID), dd: ddDialogTriggerLocation}}
	case core.CondSwitch:
		return []dialogEditRow{{kind: rowText, label: "Switch name (set)", focus: focusTrigCondText, text: c.Switch}}
	case core.CondCounter:
		return []dialogEditRow{
			{kind: rowText, label: "Counter name", focus: focusTrigCondText, text: c.Counter},
			{kind: rowNumeric, label: "At least (value)", focus: focusDialogTrigFoeKills, value: c.Count},
		}
	case core.CondQuest:
		return []dialogEditRow{{kind: rowText, label: "Quest id (active)", focus: focusTrigCondText, text: c.QuestID}}
	case core.CondGold:
		return []dialogEditRow{{kind: rowNumeric, label: "Gold at least", focus: focusDialogTrigFoeKills, value: c.Count}}
	}
	return nil // Always / Never: no params
}

// trigActRows returns the param rows for the trigger's primary ACTION.
func trigActRows(s *State, a *core.Action) []dialogEditRow {
	switch a.Kind {
	case core.ActionDialog:
		dlabel := a.DialogID
		if dlabel == "" {
			dlabel = "(pick a dialog)"
		}
		return []dialogEditRow{{kind: rowDropdown, label: "Dialog (click to choose)", text: dlabel, dd: ddDialogTriggerDialog}}
	case core.ActionSetSwitch:
		return []dialogEditRow{
			{kind: rowText, label: "Switch name", focus: focusTrigActText, text: a.Switch},
			{kind: rowDropdown, label: "Operation", text: string(a.SwitchOp), dd: ddTrigSwitchOp},
		}
	case core.ActionSetCounter:
		op := a.CounterOp
		if op == "" {
			op = core.CounterSet
		}
		return []dialogEditRow{
			{kind: rowText, label: "Counter name", focus: focusTrigActText, text: a.Counter},
			{kind: rowDropdown, label: "Operation", text: string(op), dd: ddTrigCounterOp},
			{kind: rowNumeric, label: "Value", focus: focusTrigActCount, value: a.Count},
		}
	case core.ActionSpawnFoe:
		return []dialogEditRow{
			{kind: rowDropdown, label: "Foe (click to choose)", text: core.FoeKindName(a.FoeKind), dd: ddTrigActFoe},
			{kind: rowNumeric, label: "Tile X", focus: focusTrigActTileX, value: a.TileX},
			{kind: rowNumeric, label: "Tile Z", focus: focusTrigActTileZ, value: a.TileZ},
		}
	case core.ActionSpawnChest, core.ActionOpenWall, core.ActionTeleport:
		return []dialogEditRow{
			{kind: rowNumeric, label: "Tile X", focus: focusTrigActTileX, value: a.TileX},
			{kind: rowNumeric, label: "Tile Z", focus: focusTrigActTileZ, value: a.TileZ},
		}
	case core.ActionGiveGold:
		return []dialogEditRow{{kind: rowNumeric, label: "Gold (+/-)", focus: focusTrigActCount, value: a.Count}}
	case core.ActionQuest:
		return []dialogEditRow{{kind: rowText, label: "Quest id (starts it)", focus: focusTrigActText, text: a.QuestID}}
	case core.ActionMessage:
		return []dialogEditRow{{kind: rowText, label: "Message text", focus: focusTrigActText, text: a.Text}}
	case core.ActionEvent:
		return []dialogEditRow{{kind: rowText, label: "Event id", focus: focusTrigActText, text: a.EventID}}
	}
	return nil
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
	dialogTrigModalH = float32(560)
)

type dialogTrigLayout struct {
	card            rl.Rectangle
	condKindBtn     rl.Rectangle
	condRows        [2]rl.Rectangle
	actKindBtn      rl.Rectangle
	actRows         [3]rl.Rectangle
	preserveToggle  rl.Rectangle
	backBtn         rl.Rectangle
}

func dialogTrigLayoutFor() dialogTrigLayout {
	r := centeredCardRect(dialogTrigModalW, dialogTrigModalH)
	x, fw := cardContentBox(r)
	fieldH := dialogFieldH
	y := r.Y + dialogHeaderInset
	f := stackRows(x, y, fw, fieldH, dialogTrigRowGap, 8)
	return dialogTrigLayout{
		card:           r,
		condKindBtn:    f[0],
		condRows:       [2]rl.Rectangle{f[1], f[2]},
		actKindBtn:     f[3],
		actRows:        [3]rl.Rectangle{f[4], f[5], f[6]},
		preserveToggle: f[7],
		backBtn:        bottomRightBtn(r),
	}
}

func openDialogTriggerEditModal(s *State, idx int) {
	if idx < 0 || idx >= len(s.area.Triggers) {
		return
	}
	s.modal = modalDialogTriggerEdit
	s.modalDialogTriggerIdx = idx
	s.focus = focusNone
	// Normalize a (hand-authored) empty trigger to one primary condition + action here,
	// at the deliberate open point, so trigCond/trigAct never allocate during a draw read.
	// addDialogTrigger already seeds both, so this only touches externally-authored maps.
	t := &s.area.Triggers[idx]
	if len(t.Conditions) == 0 {
		t.Conditions = []core.Condition{{Kind: core.CondAlways}}
	}
	if len(t.Actions) == 0 {
		t.Actions = []core.Action{{Kind: core.ActionDialog}}
	}
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
	c, a := trigCond(s), trigAct(s)
	l := dialogTrigLayoutFor()
	drawModalHeaderAt(font, theme, l.card, "TRIGGER "+t.ID, theme.BorderActive)

	drawLabel(font, "WHEN — condition (click to choose)", labelAbove(l.condKindBtn))
	drawButton(font, l.condKindBtn, conditionKindLabel(c.Kind)+dropdownArrowSuffix, s.dropdown.owner == ddTrigCondKind)
	drawDialogEditRows(s, font, trigCondRows(s, c), l.condRows[:])

	drawLabel(font, "DO — action (click to choose)", labelAbove(l.actKindBtn))
	drawButton(font, l.actKindBtn, actionKindLabel(a.Kind)+dropdownArrowSuffix, s.dropdown.owner == ddTrigActKind)
	drawDialogEditRows(s, font, trigActRows(s, a), l.actRows[:])

	drawButton(font, l.preserveToggle, "Preserve — stay live after firing (M): "+render.OnOffLabel(t.Preserve), t.Preserve)
	drawButton(font, l.backBtn, "Back (Esc)", false)
}

func updateDialogTriggerEditModal(s *State) Action {
	t := currentDialogTrigger(s)
	if t == nil {
		closeModal(s)
		return ActionNone
	}
	c, a := trigCond(s), trigAct(s)
	l := dialogTrigLayoutFor()

	if rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		mp := rl.GetMousePosition()
		switch {
		case pointIn(mp, l.condKindBtn):
			openFieldDropdown(s, ddTrigCondKind, l.condKindBtn)
			return ActionNone
		case pointIn(mp, l.actKindBtn):
			openFieldDropdown(s, ddTrigActKind, l.actKindBtn)
			return ActionNone
		case pointIn(mp, l.preserveToggle):
			togglePreserve(s)
			return ActionNone
		case pointIn(mp, l.backBtn):
			returnToDialogTriggerList(s)
			return ActionNone
		}
		if clickDialogEditRows(s, mp, trigCondRows(s, c), l.condRows[:]) {
			return ActionNone
		}
		if clickDialogEditRows(s, mp, trigActRows(s, a), l.actRows[:]) {
			return ActionNone
		}
		s.focus = focusNone
	}

	// A focused text param (switch/counter/id/text) takes keystrokes.
	if target := dialogTrigTextTarget(s); target != nil {
		pumpFocusField(s, target)
		if editorCommitPressed() {
			s.focus = focusNone
			return ActionNone
		}
		if editorCancelPressed() {
			returnToDialogTriggerList(s)
		}
		return ActionNone
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
		togglePreserve(s)
	}
	return ActionNone
}

func togglePreserve(s *State) {
	if t := currentDialogTrigger(s); t != nil {
		pushUndo(s)
		t.Preserve = !t.Preserve
		s.dirty = true
	}
}

// --- small shared helpers --------------------------------------------------

// subListNav runs the shared "no text field focused" keyboard tail of the dialog
// node/choice edit modals: Esc backs out (onCancel), Tab focuses the first field,
// Up/Down move the row cursor, Enter opens the selected sub-item's editor (onCommit,
// only when count>0), A adds (onAdd), X removes (onDel, only when count>0). Returns
// true when it consumed the frame; the caller handles any extra keys (the node
// modal's M) after a false return.
func subListNav(s *State, count int, focusFirst focusField, onCancel, onCommit, onAdd, onDel func()) bool {
	if editorCancelPressed() {
		onCancel()
		return true
	}
	if editorTabPressed() {
		s.focus = focusFirst
		return true
	}
	if count > 0 {
		s.modalCursor = input.CursorUpDown(s.modalCursor, count)
	}
	if editorCommitPressed() {
		if count > 0 {
			onCommit()
		}
		return true
	}
	if editorAddPressed() {
		onAdd()
		return true
	}
	if count > 0 && editorDeletePressed() {
		onDel()
		return true
	}
	return false
}

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

package core

import (
	"encoding/json"
	"fmt"
)

// Dialog system — branching conversation model + runtime state machine.
// Raylib-free so it round-trips to the .map file (core/areas.go) and is
// testable; render owns chrome, explore owns input.

// DialogSpeakerID is the stable key naming who speaks a node (not a fragile
// index); resolved through the speaker registry.
type DialogSpeakerID string

// DialogSpeaker is one speaker's presentation. Tint channels are raw 0..255 (raylib-free).
type DialogSpeaker struct {
	ID                         DialogSpeakerID
	Name                       string
	TintR, TintG, TintB, TintA uint8
}

// DialogQuestOp is what a quest-kind action does to its target quest.
type DialogQuestOp string

const (
	DialogQuestStart    DialogQuestOp = "start"
	DialogQuestComplete DialogQuestOp = "complete"
)

// DialogQuestOps returns the quest operations in canonical (editor display) order —
// the single source the editor's quest-op picker reads and asserts coverage against,
// mirroring DialogActionKinds / DialogCondKinds.
func DialogQuestOps() []DialogQuestOp {
	return []DialogQuestOp{DialogQuestStart, DialogQuestComplete}
}

// DialogActionKind tags the DialogAction union; empty means "no action".
type DialogActionKind string

const (
	DialogActionQuest DialogActionKind = "quest"
	DialogActionEvent DialogActionKind = "event"
)

// DialogActionKinds returns the non-empty end-action kinds in canonical order — the
// source the editor's action picker asserts coverage against.
func DialogActionKinds() []DialogActionKind {
	return []DialogActionKind{DialogActionQuest, DialogActionEvent}
}

// DialogAction is the optional effect a node/choice fires on resolve (quest add/complete, or event seam).
type DialogAction struct {
	Kind    DialogActionKind `json:"kind,omitempty"`
	QuestID string           `json:"questId,omitempty"`
	QuestOp DialogQuestOp    `json:"questOp,omitempty"`
	EventID string           `json:"eventId,omitempty"`
}

// DialogCondKind tags a choice condition.
type DialogCondKind string

const (
	DialogCondGold  DialogCondKind = "gold"
	DialogCondQuest DialogCondKind = "quest"
	// DialogCondFoeKilled gates on defeating a foe kind at least N times (bestiary kill count).
	DialogCondFoeKilled DialogCondKind = "foeKilled"
	// DialogCondTileVisited gates on having revealed a tile (Visited fog grid).
	DialogCondTileVisited DialogCondKind = "tileVisited"
)

// DialogCondKinds returns the authorable choice-condition kinds in canonical
// (editor display) order — the single source the editor's kind dropdown reads.
func DialogCondKinds() []DialogCondKind {
	return []DialogCondKind{DialogCondGold, DialogCondQuest, DialogCondFoeKilled, DialogCondTileVisited}
}

// DialogChoiceCondition gates a choice; a failure shows it disabled, not hidden.
type DialogChoiceCondition struct {
	Kind DialogCondKind `json:"kind"`
	// Gold: party must hold at least this much gold.
	Gold int `json:"gold,omitempty"`
	// Quest: named quest must be at QuestStatus (zero == QuestActive).
	QuestID     string      `json:"questId,omitempty"`
	QuestStatus QuestStatus `json:"questStatus,omitempty"`
	// FoeKilled: defeated FoeKind >= FoeKills times (<=0 means once). FoeKind NOT
	// omitempty: EnemyRat==0, so a Rat gate must still write the field.
	FoeKind  EnemyKind `json:"foeKind"`
	FoeKills int       `json:"foeKills,omitempty"`
	// TileVisited: player must have revealed (TileX,TileZ) on the Visited grid.
	TileX int `json:"tileX,omitempty"`
	TileZ int `json:"tileZ,omitempty"`
	// DisabledMessage overrides the auto-generated greyed-out label.
	DisabledMessage string `json:"disabledMessage,omitempty"`
}

// DialogChoice is one branch: fires EndAction then advances to NextNodeID (empty ends).
type DialogChoice struct {
	ID         string                  `json:"id"`
	Label      string                  `json:"label"`
	NextNodeID string                  `json:"next,omitempty"`
	Conditions []DialogChoiceCondition `json:"conditions,omitempty"`
	EndAction  *DialogAction           `json:"endAction,omitempty"`
}

// DialogNode is one line: Choices present a pick list, else Continue advances
// to NextNodeID. IsMenuNode fires EndAction and hands off without drawing a line.
type DialogNode struct {
	ID            string          `json:"id"`
	SpeakerID     DialogSpeakerID `json:"speaker,omitempty"`
	Text          string          `json:"text"`
	Choices       []DialogChoice  `json:"choices,omitempty"`
	NextNodeID    string          `json:"next,omitempty"`
	ContinueLabel string          `json:"continueLabel,omitempty"`
	EndAction     *DialogAction   `json:"endAction,omitempty"`
	IsMenuNode    bool            `json:"menu,omitempty"`
}

// DialogDefinition is a complete conversation. Nodes are an ordered slice (not
// a map) for stable editor order + deterministic round-trip; NodeByID is O(n).
type DialogDefinition struct {
	ID          string       `json:"id"`
	StartNodeID string       `json:"start"`
	Nodes       []DialogNode `json:"nodes"`
}

// NodeByID returns the node with the given id, or (zero, false) if absent.
func (d DialogDefinition) NodeByID(id string) (DialogNode, bool) {
	for _, n := range d.Nodes {
		if n.ID == id {
			return n, true
		}
	}
	return DialogNode{}, false
}

// DialogState is the live conversation; Def is a copy of the area's definition (self-contained).
type DialogState struct {
	Def          DialogDefinition
	NodeID       string
	ChoiceCursor int
}

// DialogChoiceView pairs a choice with selectability; Disabled ones draw greyed with Reason.
type DialogChoiceView struct {
	Choice   DialogChoice
	Disabled bool
	Reason   string
}

// menuNodeChainLimit bounds menu-node chaining per step so an authored cycle
// can't spin forever.
const menuNodeChainLimit = 16

// jsonObjectsToLines marshals each item to a one-line JSON object for a .map
// section. Returns nil (not empty) for no items. Shared by dialog/trigger writers.
func jsonObjectsToLines[T any](items []T, label string, id func(T) string) ([]string, error) {
	if len(items) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		blob, err := json.Marshal(item)
		if err != nil {
			return nil, fmt.Errorf("encode %s %q: %w", label, id(item), err)
		}
		out = append(out, string(blob))
	}
	return out, nil
}

// jsonObjectsFromLines unmarshals a .map section (one JSON object per line) into
// a slice of T. Returns nil for no lines. Shared by dialog/trigger readers.
func jsonObjectsFromLines[T any](lines []string, label string) ([]T, error) {
	if len(lines) == 0 {
		return nil, nil
	}
	out := make([]T, 0, len(lines))
	for i, line := range lines {
		var item T
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			return nil, fmt.Errorf("decode %s line %d: %w", label, i+1, err)
		}
		out = append(out, item)
	}
	return out, nil
}

// DialogsToLines marshals dialog definitions for the .map dialogs: section.
func DialogsToLines(dialogs []DialogDefinition) ([]string, error) {
	return jsonObjectsToLines(dialogs, "dialog", func(d DialogDefinition) string { return d.ID })
}

// DialogsFromLines unmarshals the .map dialogs: section into definitions.
func DialogsFromLines(lines []string) ([]DialogDefinition, error) {
	return jsonObjectsFromLines[DialogDefinition](lines, "dialog")
}

// DialogDefByID returns the area's dialog with the given id, or (zero, false).
func DialogDefByID(a AreaDefinition, id string) (DialogDefinition, bool) {
	for _, d := range a.Dialogs {
		if d.ID == id {
			return d, true
		}
	}
	return DialogDefinition{}, false
}

// StartDialog opens the dialog with the given id at its start node (false if
// unknown / no start node). A menu start node may close immediately, so a true
// return does NOT guarantee DialogOpen — re-check if you care.
func StartDialog(g *GameState, dialogID string) bool {
	def, ok := DialogDefByID(g.Area, dialogID)
	if !ok || len(def.Nodes) == 0 {
		return false
	}
	start := def.StartNodeID
	if _, ok := def.NodeByID(start); !ok {
		// Fall back to the first node so a renamed/cleared StartNodeID still opens.
		start = def.Nodes[0].ID
	}
	// Deep-copy so the live conversation shares no slices/pointers with g.Area.Dialogs.
	g.Dialog = DialogState{Def: CloneDialogDef(def)}
	g.DialogOpen = true
	goToDialogNode(g, start)
	return true
}

// CurrentDialogNode returns the node the open dialog is showing.
func CurrentDialogNode(g *GameState) (DialogNode, bool) {
	if !g.DialogOpen {
		return DialogNode{}, false
	}
	return g.Dialog.Def.NodeByID(g.Dialog.NodeID)
}

// DialogChoiceViews returns the current node's choices with selectability (disabled greyed, not hidden).
func DialogChoiceViews(g *GameState) []DialogChoiceView {
	node, ok := CurrentDialogNode(g)
	if !ok || len(node.Choices) == 0 {
		return nil
	}
	out := make([]DialogChoiceView, 0, len(node.Choices))
	for _, c := range node.Choices {
		ok, reason := dialogChoiceSelectable(g, c)
		out = append(out, DialogChoiceView{Choice: c, Disabled: !ok, Reason: reason})
	}
	return out
}

// dialogChoiceSelectable reports whether every condition passes; on the first
// failure it returns the disabled reason.
func dialogChoiceSelectable(g *GameState, c DialogChoice) (bool, string) {
	for _, cond := range c.Conditions {
		if ok, reason := evalDialogCondition(g, cond); !ok {
			return false, reason
		}
	}
	return true, ""
}

// evalDialogCondition checks one condition; on failure returns a reason (DisabledMessage, else default).
func evalDialogCondition(g *GameState, cond DialogChoiceCondition) (bool, string) {
	switch cond.Kind {
	case DialogCondGold:
		if g.Gold >= cond.Gold {
			return true, ""
		}
		return false, dialogCondReason(cond, "Not enough gold")
	case DialogCondQuest:
		idx := QuestIndexByID(g.Quests, cond.QuestID)
		if idx >= 0 && g.Quests[idx].Status == cond.QuestStatus {
			return true, ""
		}
		return false, dialogCondReason(cond, "Quest requirement not met")
	case DialogCondFoeKilled:
		if foeKillCountMet(g, cond.FoeKind, cond.FoeKills) {
			return true, ""
		}
		return false, dialogCondReason(cond, "Requires defeating "+FoeKindName(cond.FoeKind))
	case DialogCondTileVisited:
		if tileVisited(g, cond.TileX, cond.TileZ) {
			return true, ""
		}
		return false, dialogCondReason(cond, "You haven't been there yet")
	default:
		// Unknown kind — not selectable, so an authoring typo fails visibly.
		return false, dialogCondReason(cond, "Unavailable")
	}
}

func dialogCondReason(cond DialogChoiceCondition, fallback string) string {
	if cond.DisabledMessage != "" {
		return cond.DisabledMessage
	}
	return fallback
}

// SelectDialogChoice fires the choice's end action then advances to its next node. Disabled/out-of-range ignored.
func SelectDialogChoice(g *GameState, index int) {
	views := DialogChoiceViews(g)
	if index < 0 || index >= len(views) || views[index].Disabled {
		return
	}
	choice := views[index].Choice
	applyDialogAction(g, choice.EndAction)
	if !g.DialogOpen {
		return // defensive: action closed the dialog (none do today)
	}
	goToDialogNode(g, choice.NextNodeID)
}

// ContinueDialog advances a no-choice node: fires its end action then moves to
// NextNodeID (or closes). A node WITH choices ignores continue.
func ContinueDialog(g *GameState) {
	node, ok := CurrentDialogNode(g)
	if !ok {
		CloseDialog(g)
		return
	}
	if len(node.Choices) > 0 {
		return
	}
	applyDialogAction(g, node.EndAction)
	if !g.DialogOpen {
		return
	}
	goToDialogNode(g, node.NextNodeID)
}

// CloseDialog ends the conversation and clears the overlay state.
func CloseDialog(g *GameState) {
	g.DialogOpen = false
	g.Dialog = DialogState{}
}

// goToDialogNode moves to nodeID (empty/unknown ends). Menu nodes fire + chain
// on, bounded by menuNodeChainLimit so an authored cycle can't hang.
func goToDialogNode(g *GameState, nodeID string) {
	for i := 0; i < menuNodeChainLimit; i++ {
		if nodeID == "" {
			CloseDialog(g)
			return
		}
		node, ok := g.Dialog.Def.NodeByID(nodeID)
		if !ok {
			CloseDialog(g)
			return
		}
		g.Dialog.NodeID = nodeID
		g.Dialog.ChoiceCursor = 0
		if !node.IsMenuNode {
			// Land on the first selectable choice, not a greyed-out row.
			g.Dialog.ChoiceCursor = firstSelectableChoice(g)
			return
		}
		// Menu node: fire its action and hand off to NextNodeID (or close).
		applyDialogAction(g, node.EndAction)
		if !g.DialogOpen {
			return
		}
		nodeID = node.NextNodeID
	}
	// Past the chain limit — almost certainly a menu-node cycle; close.
	CloseDialog(g)
}

// ClampDialogCursor clamps the choice cursor to the node's choice count (defensive backstop for input).
func ClampDialogCursor(g *GameState) {
	if !g.DialogOpen {
		return
	}
	n := len(DialogChoiceViews(g))
	if n == 0 {
		g.Dialog.ChoiceCursor = 0
		return
	}
	g.Dialog.ChoiceCursor = Clamp(g.Dialog.ChoiceCursor, 0, n-1)
}

// applyDialogAction performs a node/choice end action (nil is a no-op).
// Quest actions seed/complete a journal quest; event actions are a future seam.
func applyDialogAction(g *GameState, action *DialogAction) {
	if action == nil {
		return
	}
	switch action.Kind {
	case DialogActionQuest:
		if action.QuestID == "" {
			// Blank id would seed a junk entry or no-op a complete — drop it.
			return
		}
		switch action.QuestOp {
		case DialogQuestStart:
			g.Quests = AddQuest(g.Quests, Quest{
				ID:     action.QuestID,
				Title:  QuestTitleFromID(action.QuestID),
				Status: QuestActive,
			})
		case DialogQuestComplete:
			CompleteQuest(g, action.QuestID)
		default:
			// Unknown op (authoring typo) — do nothing rather than guess.
		}
	case DialogActionEvent:
		// No event registry yet; seam for a future switch on action.EventID.
	default:
		// Unknown action kind (authoring typo / unhandled new kind) — do nothing
		// rather than guess, mirroring the QuestOp default above. Data-driven, so
		// no panic.
	}
}

// MoveDialogCursor steps the cursor by delta's sign, wrapping and SKIPPING
// disabled choices. No-op when there are no choices or all are disabled.
func MoveDialogCursor(g *GameState, delta int) {
	if !g.DialogOpen || delta == 0 {
		return
	}
	views := DialogChoiceViews(g)
	n := len(views)
	if n == 0 {
		g.Dialog.ChoiceCursor = 0
		return
	}
	step := Sign(delta)
	cur := Clamp(g.Dialog.ChoiceCursor, 0, n-1)
	for i := 0; i < n; i++ {
		cur = WrapIndex(cur+step, n)
		if !views[cur].Disabled {
			g.Dialog.ChoiceCursor = cur
			return
		}
	}
	// All disabled — leave the cursor where it is (still clamped).
	g.Dialog.ChoiceCursor = Clamp(g.Dialog.ChoiceCursor, 0, n-1)
}

// firstSelectableChoice returns the first non-disabled choice index, or 0 if none.
func firstSelectableChoice(g *GameState) int {
	for i, v := range DialogChoiceViews(g) {
		if !v.Disabled {
			return i
		}
	}
	return 0
}

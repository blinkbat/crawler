package core

import (
	"encoding/json"
	"fmt"
)

// Dialog system — a branching conversation model + runtime state machine,
// modeled on the bg2-threejs dialog feature (nodes keyed by id, per-node
// text + speaker, choice lists with conditions, and end-of-line UI actions).
// Kept raylib-free so it round-trips to the .map file (via core/areas.go) and
// is unit-testable; render owns the modal chrome, explore owns input.
//
// Shape, mirroring bg2:
//   - A DialogDefinition is a named graph of DialogNodes with a start node.
//   - A node shows one speaker's line, then EITHER a list of choices the
//     player picks from, OR a single "continue" advance to NextNodeID.
//   - A choice can be gated by conditions (gold / quest state); a failing
//     condition leaves the choice visible but disabled with a reason.
//   - A node or choice can carry a DialogAction fired when the line/choice
//     resolves (start or complete a quest, or emit a named event).
//   - A "menu node" fires its action and closes immediately (used to hand off
//     to another system without showing a line).

// DialogSpeakerID is the stable key naming who speaks a node. Authored
// dialogs reference a speaker by id (not a fragile index), resolved through
// the speaker registry below.
type DialogSpeakerID string

// DialogSpeaker is the presentation for one speaker — a display name plus a
// nameplate tint. Portraits are deferred (the crawler draws procedurally);
// the tint is enough to make each speaker visually distinct. Channels are
// raw 0..255 so core stays raylib-free; render converts to rl.Color.
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

// DialogActionKind tags the DialogAction union. The empty value means "no
// action" so an absent action is the zero value.
type DialogActionKind string

const (
	DialogActionQuest DialogActionKind = "quest"
	DialogActionEvent DialogActionKind = "event"
)

// DialogAction is the optional effect a node or choice fires when it
// resolves. Quest actions add/complete a journal quest; event actions emit a
// named string the game can hook later (the bg2 "event" seam — unhandled ids
// are a no-op for now). A pointer field on nodes/choices keeps the common
// no-action case at the JSON zero value.
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
	// DialogCondFoeKilled gates on the party having defeated a foe kind at
	// least N times (bestiary kill count). The bg2 "Dead('foe')" trigger.
	DialogCondFoeKilled DialogCondKind = "foeKilled"
	// DialogCondTileVisited gates on the player having revealed/stepped a tile
	// (the Visited fog grid). The bg2 area/range check, scoped to one tile.
	DialogCondTileVisited DialogCondKind = "tileVisited"
)

// DialogCondKinds returns the authorable choice-condition kinds in canonical
// (editor display) order — the single source the editor's kind dropdown reads
// so adding a kind doesn't mean hand-editing a parallel slice there.
func DialogCondKinds() []DialogCondKind {
	return []DialogCondKind{DialogCondGold, DialogCondQuest, DialogCondFoeKilled, DialogCondTileVisited}
}

// DialogChoiceCondition gates whether a choice is selectable. A failing
// condition does NOT hide the choice — it shows disabled with DisabledMessage
// (or a generated reason), matching bg2's "grey it out, don't remove it" UX.
type DialogChoiceCondition struct {
	Kind DialogCondKind `json:"kind"`
	// Gold condition: the party must hold at least this much gold.
	Gold int `json:"gold,omitempty"`
	// Quest condition: the named quest must be at QuestStatus. The zero
	// QuestStatus is QuestActive, so a quest condition with no status set
	// requires the quest to be active.
	QuestID     string      `json:"questId,omitempty"`
	QuestStatus QuestStatus `json:"questStatus,omitempty"`
	// FoeKilled condition: the party must have defeated FoeKind at least
	// FoeKills times. FoeKills <= 0 is treated as "at least once". FoeKind
	// serializes as its int value (enemy kinds are append-only). NOT
	// omitempty: EnemyRat==0, so a Rat gate must still write the field —
	// otherwise it's indistinguishable on disk from an unauthored condition.
	FoeKind  EnemyKind `json:"foeKind"`
	FoeKills int       `json:"foeKills,omitempty"`
	// TileVisited condition: the player must have revealed tile (TileX,TileZ)
	// on the Visited fog grid. (0,0) round-trips fine despite omitempty (an
	// absent coord decodes back to 0).
	TileX int `json:"tileX,omitempty"`
	TileZ int `json:"tileZ,omitempty"`
	// DisabledMessage overrides the auto-generated greyed-out label.
	DisabledMessage string `json:"disabledMessage,omitempty"`
}

// DialogChoice is one selectable branch shown on a node. Selecting it fires
// EndAction (if any) then advances to NextNodeID (empty NextNodeID ends the
// conversation).
type DialogChoice struct {
	ID         string                  `json:"id"`
	Label      string                  `json:"label"`
	NextNodeID string                  `json:"next,omitempty"`
	Conditions []DialogChoiceCondition `json:"conditions,omitempty"`
	EndAction  *DialogAction           `json:"endAction,omitempty"`
}

// DialogNode is one line of a conversation. With Choices it presents a pick
// list; without, it shows a single Continue that advances to NextNodeID
// (empty ends the conversation). IsMenuNode fires EndAction and closes/hands
// off immediately without drawing a line — the bg2 menu-node behavior.
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

// DialogDefinition is a complete conversation: an id, the node to open on,
// and the node list. Nodes are an ordered slice (not a map like bg2's
// Record) so the editor has a stable display order and JSON round-trips
// deterministically; NodeByID is the O(n) lookup the runtime uses.
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

// DialogState is the live conversation on GameState. Def is a copy of the
// area's authored definition (so the runtime is self-contained); NodeID is
// the node currently shown; ChoiceCursor is the highlighted choice row.
type DialogState struct {
	Def          DialogDefinition
	NodeID       string
	ChoiceCursor int
}

// DialogChoiceView pairs a choice with its current selectability for the
// renderer + input: Disabled choices draw greyed with Reason and can't be
// chosen.
type DialogChoiceView struct {
	Choice   DialogChoice
	Disabled bool
	Reason   string
}

// menuNodeChainLimit bounds how many menu nodes StartDialog/advance will
// chain through in one step, so an authored cycle of menu nodes (each
// pointing at the next) can't spin forever.
const menuNodeChainLimit = 16

// DialogsToLines marshals each dialog definition to a single-line JSON
// object for the .map file's dialogs: section. Used by MapFileFromArea.
func DialogsToLines(dialogs []DialogDefinition) ([]string, error) {
	if len(dialogs) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(dialogs))
	for _, d := range dialogs {
		blob, err := json.Marshal(d)
		if err != nil {
			return nil, fmt.Errorf("encode dialog %q: %w", d.ID, err)
		}
		out = append(out, string(blob))
	}
	return out, nil
}

// DialogsFromLines unmarshals the .map dialogs: section (one JSON object per
// line) back into definitions. Used by AreaFromMapFile.
func DialogsFromLines(lines []string) ([]DialogDefinition, error) {
	if len(lines) == 0 {
		return nil, nil
	}
	out := make([]DialogDefinition, 0, len(lines))
	for i, line := range lines {
		var d DialogDefinition
		if err := json.Unmarshal([]byte(line), &d); err != nil {
			return nil, fmt.Errorf("decode dialog line %d: %w", i+1, err)
		}
		out = append(out, d)
	}
	return out, nil
}

// DialogDefByID returns the area's dialog with the given id.
func DialogDefByID(a AreaDefinition, id string) (DialogDefinition, bool) {
	for _, d := range a.Dialogs {
		if d.ID == id {
			return d, true
		}
	}
	return DialogDefinition{}, false
}

// StartDialog opens the area dialog with the given id at its start node.
// Returns false (and leaves no dialog open) when the id is unknown or the
// definition has no usable start node. If the start node is a menu node its
// action fires and the dialog may close immediately — so a true return does
// NOT guarantee DialogOpen is still set; callers that care should re-check.
func StartDialog(g *GameState, dialogID string) bool {
	def, ok := DialogDefByID(g.Area, dialogID)
	if !ok || len(def.Nodes) == 0 {
		return false
	}
	start := def.StartNodeID
	if _, ok := def.NodeByID(start); !ok {
		// Fall back to the first node so a definition whose StartNodeID was
		// renamed/cleared still opens rather than silently refusing.
		start = def.Nodes[0].ID
	}
	// Deep-copy the authored definition so the live conversation shares no
	// backing slices / action pointers with g.Area.Dialogs — the runtime stays
	// self-contained (and an in-place area edit can't mutate an open dialog).
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

// DialogChoiceViews returns the current node's choices annotated with whether
// each is selectable right now. Every authored choice is returned (disabled
// ones greyed, not hidden), mirroring bg2.
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

// dialogChoiceSelectable reports whether every condition on a choice passes;
// on the first failure it returns the disabled reason to show.
func dialogChoiceSelectable(g *GameState, c DialogChoice) (bool, string) {
	for _, cond := range c.Conditions {
		if ok, reason := evalDialogCondition(g, cond); !ok {
			return false, reason
		}
	}
	return true, ""
}

// evalDialogCondition checks one choice condition, returning a player-facing
// reason when it fails (the authored DisabledMessage if set, else a default).
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
		// Unknown condition kind — treat as not selectable so an authoring
		// typo fails visibly rather than silently allowing the branch.
		return false, dialogCondReason(cond, "Unavailable")
	}
}

func dialogCondReason(cond DialogChoiceCondition, fallback string) string {
	if cond.DisabledMessage != "" {
		return cond.DisabledMessage
	}
	return fallback
}

// SelectDialogChoice resolves the choice at the given index of the current
// node: fires its end action, then advances to its next node (or closes when
// the choice has no next node). A disabled or out-of-range choice is ignored.
func SelectDialogChoice(g *GameState, index int) {
	views := DialogChoiceViews(g)
	if index < 0 || index >= len(views) || views[index].Disabled {
		return
	}
	choice := views[index].Choice
	applyDialogAction(g, choice.EndAction)
	if !g.DialogOpen {
		return // action closed the dialog (defensive — actions don't today)
	}
	goToDialogNode(g, choice.NextNodeID)
}

// ContinueDialog advances a no-choice node: fires the node's end action then
// moves to its NextNodeID (or closes when none). A node WITH choices ignores
// continue — the player must pick.
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

// goToDialogNode moves the open dialog to nodeID (empty / unknown id ends the
// conversation). Menu nodes fire their action and chain on without drawing a
// line; the chain is bounded by menuNodeChainLimit so an authored cycle can't
// hang.
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
			// Land on the first selectable choice so the player doesn't open a
			// node parked on a greyed-out row (mirrors MoveDialogCursor's skip).
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
	// Ran past the chain limit — almost certainly a menu-node cycle. Close
	// rather than spin.
	CloseDialog(g)
}

// ClampDialogCursor clamps the choice cursor to the current node's choice
// count, so a node reached with fewer choices than the previous one can't
// leave the cursor pointing past the end. goToDialogNode already zeroes the
// cursor on every node change; this is the defensive backstop input callers
// use after navigating.
func ClampDialogCursor(g *GameState) {
	if !g.DialogOpen {
		return
	}
	n := len(DialogChoiceViews(g))
	if g.Dialog.ChoiceCursor >= n {
		g.Dialog.ChoiceCursor = n - 1
	}
	if g.Dialog.ChoiceCursor < 0 {
		g.Dialog.ChoiceCursor = 0
	}
}

// applyDialogAction performs a node/choice end action. nil is a no-op.
// Quest actions seed or complete a journal quest; event actions are a seam
// for future hooks (unhandled ids no-op today).
func applyDialogAction(g *GameState, action *DialogAction) {
	if action == nil {
		return
	}
	switch action.Kind {
	case DialogActionQuest:
		if action.QuestID == "" {
			// A blank id would seed a junk journal entry (AddQuest only dedupes
			// by ID) or no-op a complete — drop it rather than corrupt the log.
			return
		}
		switch action.QuestOp {
		case DialogQuestStart:
			g.Quests = AddQuest(g.Quests, Quest{
				ID:     action.QuestID,
				Title:  action.QuestID,
				Status: QuestActive,
			})
		case DialogQuestComplete:
			CompleteQuest(g, action.QuestID)
		}
	case DialogActionEvent:
		// No event registry yet — the seam exists so a future system can
		// switch on action.EventID. Intentionally a no-op for now.
	}
}

// MoveDialogCursor steps the choice cursor in the direction of delta (only its
// sign matters), wrapping and SKIPPING disabled choices so the cursor never
// lands on a greyed-out row the player can't confirm. No-op when the node has
// no choices or every choice is disabled (the cursor stays put, clamped). This
// is the single live cursor-stepping path — the explore input layer calls it.
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
	step := 1
	if delta < 0 {
		step = -1
	}
	cur := Clamp(g.Dialog.ChoiceCursor, 0, n-1)
	for i := 0; i < n; i++ {
		cur = WrapIndex(cur+step, n)
		if !views[cur].Disabled {
			g.Dialog.ChoiceCursor = cur
			return
		}
	}
	// Every choice disabled — leave the cursor where it is (still clamped).
	g.Dialog.ChoiceCursor = Clamp(g.Dialog.ChoiceCursor, 0, n-1)
}

// firstSelectableChoice returns the index of the current node's first
// non-disabled choice, or 0 when the node has no choices or every choice is
// disabled. Used to seat the cursor on an enabled row when a node opens, so the
// player doesn't start parked on a greyed-out (un-confirmable) choice.
func firstSelectableChoice(g *GameState) int {
	for i, v := range DialogChoiceViews(g) {
		if !v.Disabled {
			return i
		}
	}
	return 0
}

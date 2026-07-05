package core

import "testing"

// sampleDialog: start has "go"→second (then ends) and "leave" (completes a quest, ends).
func sampleDialog() DialogDefinition {
	return DialogDefinition{
		ID:          "d1",
		StartNodeID: "start",
		Nodes: []DialogNode{
			{
				ID:        "start",
				SpeakerID: SpeakerStranger,
				Text:      "Well met.",
				Choices: []DialogChoice{
					{ID: "go", Label: "Tell me more", NextNodeID: "second"},
					{
						ID:        "leave",
						Label:     "Farewell",
						EndAction: &Action{Kind: ActionQuest, QuestOp: DialogQuestComplete, QuestID: "q1"},
					},
				},
			},
			{ID: "second", SpeakerID: SpeakerStranger, Text: "There is more."},
		},
	}
}

func newDialogGame() *GameState {
	return &GameState{
		Area:   AreaDefinition{Dialogs: []DialogDefinition{sampleDialog()}},
		Quests: []Quest{{ID: "q1", Title: "Q", Status: QuestActive}},
	}
}

func TestStartDialogOpensAtStart(t *testing.T) {
	g := newDialogGame()
	if !StartDialog(g, "d1") {
		t.Fatal("StartDialog returned false for a known dialog")
	}
	if !g.DialogOpen {
		t.Fatal("DialogOpen should be true after StartDialog")
	}
	node, ok := CurrentDialogNode(g)
	if !ok || node.ID != "start" {
		t.Fatalf("expected current node 'start', got %q ok=%v", node.ID, ok)
	}
}

func TestStartDialogRefusesWhenOpen(t *testing.T) {
	g := newDialogGame()
	StartDialog(g, "d1")
	node, _ := CurrentDialogNode(g)
	if node.ID != "start" {
		t.Fatalf("precondition: expected 'start' node, got %q", node.ID)
	}
	// A second StartDialog (e.g. a node/choice ActionDialog end-action) must NOT replace
	// the live conversation — that would strand the caller's post-action navigation.
	if StartDialog(g, "d1") {
		t.Fatal("StartDialog must refuse when a dialog is already open")
	}
	node, _ = CurrentDialogNode(g)
	if node.ID != "start" {
		t.Fatalf("the open conversation must be untouched, got node %q", node.ID)
	}
}

func TestStartDialogUnknownIDNoOp(t *testing.T) {
	g := newDialogGame()
	if StartDialog(g, "nope") {
		t.Fatal("StartDialog should return false for an unknown id")
	}
	if g.DialogOpen {
		t.Fatal("DialogOpen should stay false when the dialog id is unknown")
	}
}

func TestSelectChoiceAdvances(t *testing.T) {
	g := newDialogGame()
	StartDialog(g, "d1")
	SelectDialogChoice(g, 0)
	node, ok := CurrentDialogNode(g)
	if !ok || node.ID != "second" {
		t.Fatalf("expected node 'second' after choosing go, got %q ok=%v", node.ID, ok)
	}
	ContinueDialog(g) // 'second' is terminal → ends
	if g.DialogOpen {
		t.Fatal("dialog should close after continuing a terminal node")
	}
}

func TestChoiceEndActionCompletesQuest(t *testing.T) {
	g := newDialogGame()
	StartDialog(g, "d1")
	SelectDialogChoice(g, 1) // "leave" completes q1, no next → closes
	if g.DialogOpen {
		t.Fatal("dialog should close when the chosen branch has no next node")
	}
	if idx := QuestIndexByID(g.Quests, "q1"); idx < 0 || g.Quests[idx].Status != QuestComplete {
		t.Fatal("choosing the quest branch should complete q1")
	}
}

func TestGoldConditionDisablesChoice(t *testing.T) {
	g := &GameState{
		Gold: 5,
		Area: AreaDefinition{Dialogs: []DialogDefinition{{
			ID: "d", StartNodeID: "s",
			Nodes: []DialogNode{{ID: "s", Text: "?", Choices: []DialogChoice{
				{ID: "buy", Label: "Buy (10g)", Conditions: []DialogChoiceCondition{{Kind: DialogCondGold, Gold: 10}}},
			}}},
		}}},
	}
	StartDialog(g, "d")
	views := DialogChoiceViews(g)
	if len(views) != 1 || !views[0].Disabled {
		t.Fatalf("gold-gated choice should be disabled with 5 < 10 gold; views=%+v", views)
	}
	SelectDialogChoice(g, 0)
	if !g.DialogOpen {
		t.Fatal("selecting a disabled choice should not advance/close the dialog")
	}
	g.Gold = 20
	if DialogChoiceViews(g)[0].Disabled {
		t.Fatal("choice should be enabled once gold meets the requirement")
	}
}

func TestMenuNodeAutoAdvances(t *testing.T) {
	g := &GameState{
		Quests: []Quest{{ID: "q", Status: QuestActive}},
		Area: AreaDefinition{Dialogs: []DialogDefinition{{
			ID: "d", StartNodeID: "m",
			Nodes: []DialogNode{{
				ID:         "m",
				IsMenuNode: true,
				EndAction:  &Action{Kind: ActionQuest, QuestOp: DialogQuestComplete, QuestID: "q"},
			}},
		}}},
	}
	StartDialog(g, "d")
	if g.DialogOpen {
		t.Fatal("a terminal menu node should fire its action and close immediately")
	}
	if idx := QuestIndexByID(g.Quests, "q"); idx < 0 || g.Quests[idx].Status != QuestComplete {
		t.Fatal("menu node action should have completed the quest")
	}
}

func TestMenuNodeCycleDoesNotHang(t *testing.T) {
	// Two menu nodes pointing at each other — must bail at the chain limit.
	g := &GameState{Area: AreaDefinition{Dialogs: []DialogDefinition{{
		ID: "d", StartNodeID: "a",
		Nodes: []DialogNode{
			{ID: "a", IsMenuNode: true, NextNodeID: "b"},
			{ID: "b", IsMenuNode: true, NextNodeID: "a"},
		},
	}}}}
	StartDialog(g, "d")
	if g.DialogOpen {
		t.Fatal("a menu-node cycle should terminate by closing the dialog")
	}
}

func TestClampDialogCursor(t *testing.T) {
	g := newDialogGame()
	StartDialog(g, "d1")
	// 'start' has two choices; a stale cursor past the end clamps to the last.
	g.Dialog.ChoiceCursor = 5
	ClampDialogCursor(g)
	if g.Dialog.ChoiceCursor != 1 {
		t.Fatalf("cursor should clamp to last choice (1), got %d", g.Dialog.ChoiceCursor)
	}
	// A choiceless node clamps the cursor back to 0.
	SelectDialogChoice(g, 0)
	g.Dialog.ChoiceCursor = 3
	ClampDialogCursor(g)
	if g.Dialog.ChoiceCursor != 0 {
		t.Fatalf("cursor should clamp to 0 on a choiceless node, got %d", g.Dialog.ChoiceCursor)
	}
}

func TestDialogsJSONRoundTrip(t *testing.T) {
	in := []DialogDefinition{sampleDialog()}
	lines, err := DialogsToLines(in)
	if err != nil {
		t.Fatalf("DialogsToLines: %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("expected 1 encoded line, got %d", len(lines))
	}
	out, err := DialogsFromLines(lines)
	if err != nil {
		t.Fatalf("DialogsFromLines: %v", err)
	}
	if !dialogsEqual(in, out) {
		t.Fatalf("round-trip mismatch:\n in=%+v\nout=%+v", in, out)
	}
}

func TestDialogsFromLinesEmpty(t *testing.T) {
	out, err := DialogsFromLines(nil)
	if err != nil || out != nil {
		t.Fatalf("empty input should yield (nil,nil), got (%v,%v)", out, err)
	}
}

// condGame builds a one-choice dialog whose choice carries cond.
func condGame(cond DialogChoiceCondition) *GameState {
	g := &GameState{
		Bestiary: make(Bestiary),
		Area: AreaDefinition{Dialogs: []DialogDefinition{{
			ID: "d", StartNodeID: "s",
			Nodes: []DialogNode{{ID: "s", Text: "?", Choices: []DialogChoice{
				{ID: "c", Label: "Go", Conditions: []DialogChoiceCondition{cond}},
			}}},
		}}},
	}
	StartDialog(g, "d")
	return g
}

func TestFoeKilledCondition(t *testing.T) {
	kind := EnemyKinds()[0].Kind
	g := condGame(DialogChoiceCondition{Kind: DialogCondFoeKilled, FoeKind: kind, FoeKills: 2})
	if !DialogChoiceViews(g)[0].Disabled {
		t.Fatal("choice should be disabled before any kills")
	}
	g.Bestiary.RecordKill(kind)
	if !DialogChoiceViews(g)[0].Disabled {
		t.Fatal("choice should still be disabled at 1/2 kills")
	}
	g.Bestiary.RecordKill(kind)
	if DialogChoiceViews(g)[0].Disabled {
		t.Fatal("choice should enable once kill count meets the requirement")
	}
}

func TestFoeKilledConditionZeroMeansOnce(t *testing.T) {
	kind := EnemyKinds()[0].Kind
	g := condGame(DialogChoiceCondition{Kind: DialogCondFoeKilled, FoeKind: kind}) // FoeKills 0 → "at least once"
	if !DialogChoiceViews(g)[0].Disabled {
		t.Fatal("zero-count foe condition should require at least one kill")
	}
	g.Bestiary.RecordKill(kind)
	if DialogChoiceViews(g)[0].Disabled {
		t.Fatal("one kill should satisfy a zero-count foe condition")
	}
}

func TestTileVisitedCondition(t *testing.T) {
	g := condGame(DialogChoiceCondition{Kind: DialogCondTileVisited, TileX: 1, TileZ: 2})
	// No fog grid → not visited (bounds-checked).
	if !DialogChoiceViews(g)[0].Disabled {
		t.Fatal("tile condition should be disabled when unvisited / out of grid")
	}
	g.Visited = [][]bool{{false, false, false}, {false, false, false}, {false, false, false}}
	if !DialogChoiceViews(g)[0].Disabled {
		t.Fatal("tile condition should stay disabled until the tile is revealed")
	}
	g.Visited[2][1] = true
	if DialogChoiceViews(g)[0].Disabled {
		t.Fatal("tile condition should enable once (1,2) is visited")
	}
}

// enterTileTrigger builds a fire-once trigger: party on (x,z) → start dialog d1.
func enterTileTrigger(id string, x, z int, preserve bool) Trigger {
	return Trigger{
		ID:         id,
		Conditions: []Condition{{Kind: CondEnterTile, TileX: x, TileZ: z}},
		Actions:    []Action{{Kind: ActionDialog, DialogID: "d1"}},
		Preserve:   preserve,
	}
}

func TestEnterTileTriggerFires(t *testing.T) {
	g := &GameState{
		Visited: [][]bool{{true, true}, {true, true}},
		Area: AreaDefinition{
			Dialogs:  []DialogDefinition{sampleDialog()},
			Triggers: []Trigger{enterTileTrigger("t1", 1, 0, false)},
		},
	}
	g.Player.TileX, g.Player.TileZ = 0, 0
	EvaluateTriggers(g)
	if g.DialogOpen {
		t.Fatal("a non-matching tile should not fire the trigger")
	}
	g.Player.TileX, g.Player.TileZ = 1, 0
	EvaluateTriggers(g)
	if !g.DialogOpen {
		t.Fatal("standing on the trigger tile should open the dialog")
	}
	// fire-once: re-evaluating on the same tile must not re-fire.
	CloseDialog(g)
	EvaluateTriggers(g)
	if g.DialogOpen {
		t.Fatal("a fire-once enter-tile trigger should not fire a second time")
	}
}

func TestEnterTileTriggerPreserveRepeats(t *testing.T) {
	g := &GameState{
		Area: AreaDefinition{
			Dialogs:  []DialogDefinition{sampleDialog()},
			Triggers: []Trigger{enterTileTrigger("t1", 0, 0, true)},
		},
	}
	EvaluateTriggers(g)
	if !g.DialogOpen {
		t.Fatal("preserve trigger should fire")
	}
	CloseDialog(g)
	EvaluateTriggers(g)
	if !g.DialogOpen {
		t.Fatal("a preserve enter-tile trigger should fire again on re-evaluation")
	}
}

func TestFoeKilledTriggerFires(t *testing.T) {
	kind := EnemyKinds()[0].Kind
	g := &GameState{
		Bestiary: make(Bestiary),
		Area: AreaDefinition{
			Dialogs: []DialogDefinition{sampleDialog()},
			Triggers: []Trigger{{
				ID:         "t1",
				Conditions: []Condition{{Kind: CondFoeKilled, FoeKind: kind, Count: 1}},
				Actions:    []Action{{Kind: ActionDialog, DialogID: "d1"}},
			}},
		},
	}
	EvaluateTriggers(g)
	if g.DialogOpen {
		t.Fatal("foe-killed trigger should not fire before the kill threshold")
	}
	g.Bestiary.RecordKill(kind)
	EvaluateTriggers(g)
	if !g.DialogOpen {
		t.Fatal("foe-killed trigger should fire once the threshold is met")
	}
	CloseDialog(g)
	EvaluateTriggers(g)
	if g.DialogOpen {
		t.Fatal("a fire-once foe-killed trigger should not re-fire")
	}
}

func TestSwitchActionAndCondition(t *testing.T) {
	// Trigger A (always) sets switch S; trigger B (switch S set) opens a passage by
	// setting a counter we can assert on. Cascades within one EvaluateTriggers call.
	g := &GameState{
		Area: AreaDefinition{Triggers: []Trigger{
			{ID: "a", Actions: []Action{{Kind: ActionSetSwitch, Switch: "S", SwitchOp: SwitchSet}}},
			{ID: "b", Conditions: []Condition{{Kind: CondSwitch, Switch: "S"}}, Actions: []Action{{Kind: ActionSetCounter, Counter: "C", CounterOp: CounterSet, Count: 7}}},
		}},
	}
	EvaluateTriggers(g)
	if !g.Switches["S"] {
		t.Fatal("trigger A should have set switch S")
	}
	if g.Counters["C"] != 7 {
		t.Fatalf("trigger B should have fired off switch S and set C=7, got %d", g.Counters["C"])
	}
}

func TestTriggerNoFireWhileDialogOpen(t *testing.T) {
	g := &GameState{
		Area: AreaDefinition{
			Dialogs:  []DialogDefinition{sampleDialog()},
			Triggers: []Trigger{enterTileTrigger("t1", 0, 0, false)},
		},
	}
	g.DialogOpen = true
	EvaluateTriggers(g)
	// The dialog action would call StartDialog; with a dialog already open it can't
	// stomp it (StartDialog no-ops), and evaluation halts on the open dialog.
	node, _ := CurrentDialogNode(g)
	if node.ID != "" {
		t.Fatal("a trigger must not stomp an already-open dialog")
	}
}

func TestTriggersJSONRoundTrip(t *testing.T) {
	in := []Trigger{
		{ID: "t1", Conditions: []Condition{{Kind: CondEnterTile, TileX: 3, TileZ: 4}}, Actions: []Action{{Kind: ActionDialog, DialogID: "d1"}}},
		{ID: "t2", Conditions: []Condition{{Kind: CondFoeKilled, FoeKind: EnemyKinds()[1].Kind, Count: 5}}, Actions: []Action{{Kind: ActionSpawnChest, TileX: 2, TileZ: 2, Items: []ItemKind{AllItems()[0].Kind}}}, Preserve: true},
	}
	lines, err := TriggersToLines(in)
	if err != nil {
		t.Fatalf("TriggersToLines: %v", err)
	}
	out, err := TriggersFromLines(lines)
	if err != nil {
		t.Fatalf("TriggersFromLines: %v", err)
	}
	if len(in) != len(out) || !triggersEqual(in, out) {
		t.Fatalf("round-trip mismatch:\n in=%+v\nout=%+v", in, out)
	}
}

func TestStartDialogClonesDefinition(t *testing.T) {
	g := newDialogGame()
	StartDialog(g, "d1")
	// Mutating the live copy must not bleed into the area's definition (deep-copied).
	g.Dialog.Def.Nodes[0].Text = "MUTATED"
	if g.Area.Dialogs[0].Nodes[0].Text == "MUTATED" {
		t.Fatal("StartDialog must deep-copy the definition; mutating the live copy changed the area's dialog")
	}
}

func TestRequiredFoeKills(t *testing.T) {
	for in, want := range map[int]int{-3: 1, 0: 1, 1: 1, 2: 2, 7: 7} {
		if got := RequiredFoeKills(in); got != want {
			t.Errorf("RequiredFoeKills(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestFoeKindName(t *testing.T) {
	kind := EnemyKinds()[0].Kind
	if got := FoeKindName(kind); got == "" || got == "unknown foe" {
		t.Fatalf("registered kind should resolve to a real name, got %q", got)
	}
	if got := FoeKindName(EnemyKind(9999)); got != "unknown foe" {
		t.Fatalf("unregistered kind should fall back to %q, got %q", "unknown foe", got)
	}
}

func TestDialogKindListsCoverEval(t *testing.T) {
	// Every advertised condition kind must be handled (no "Unavailable" dead row).
	g := &GameState{Bestiary: make(Bestiary)}
	for _, k := range DialogCondKinds() {
		_, reason := evalDialogCondition(g, DialogChoiceCondition{Kind: k})
		if reason == "Unavailable" {
			t.Errorf("condition kind %q is advertised by DialogCondKinds but unhandled by evalDialogCondition", k)
		}
	}
	if len(ConditionKinds()) == 0 || len(ActionKinds()) == 0 {
		t.Fatal("ConditionKinds / ActionKinds must not be empty")
	}
}

func TestSaveDataTriggersFired(t *testing.T) {
	g := &GameState{
		Area:          AreaDefinition{Path: "maps/x.map"},
		TriggersFired: map[string]bool{"t1": true},
	}
	data := NewSaveData(g)
	if !data.TriggersFired["t1"] {
		t.Fatal("NewSaveData should capture the fired-trigger set")
	}
	g.TriggersFired["t1"] = false
	if !data.TriggersFired["t1"] {
		t.Fatal("save snapshot should be detached from the live TriggersFired map")
	}
}

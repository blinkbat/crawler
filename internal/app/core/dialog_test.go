package core

import "testing"

// sampleDialog is a small graph used across the runtime tests:
//
//	start ──(choice "go")──▶ second ──(continue)──▶ (ends)
//	      └─(choice "leave", completes quest)──▶ (ends)
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
						EndAction: &DialogAction{Kind: DialogActionQuest, QuestOp: DialogQuestComplete, QuestID: "q1"},
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
	SelectDialogChoice(g, 0) // "go" → second
	node, ok := CurrentDialogNode(g)
	if !ok || node.ID != "second" {
		t.Fatalf("expected node 'second' after choosing go, got %q ok=%v", node.ID, ok)
	}
	// 'second' has no choices and no next → Continue ends the conversation.
	ContinueDialog(g)
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
	// Selecting a disabled choice is a no-op (stays open on the same node).
	SelectDialogChoice(g, 0)
	if !g.DialogOpen {
		t.Fatal("selecting a disabled choice should not advance/close the dialog")
	}
	// With enough gold it becomes selectable.
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
				EndAction:  &DialogAction{Kind: DialogActionQuest, QuestOp: DialogQuestComplete, QuestID: "q"},
				// No NextNodeID → fires action then closes.
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
	// Two menu nodes pointing at each other — goToDialogNode must bail at the
	// chain limit instead of looping forever.
	g := &GameState{Area: AreaDefinition{Dialogs: []DialogDefinition{{
		ID: "d", StartNodeID: "a",
		Nodes: []DialogNode{
			{ID: "a", IsMenuNode: true, NextNodeID: "b"},
			{ID: "b", IsMenuNode: true, NextNodeID: "a"},
		},
	}}}}
	StartDialog(g, "d") // must return rather than spin
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
	// Advancing to a node with no choices clamps the cursor back to 0.
	SelectDialogChoice(g, 0) // → 'second' (no choices)
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

// condGame builds a one-choice dialog whose single choice carries the given
// condition, so a test can read DialogChoiceViews()[0].Disabled.
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
	g.Bestiary.RecordKill(kind) // 1 — still short of 2
	if !DialogChoiceViews(g)[0].Disabled {
		t.Fatal("choice should still be disabled at 1/2 kills")
	}
	g.Bestiary.RecordKill(kind) // 2 — meets the bar
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
	// No fog grid → not visited (bounds-checked, no panic).
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

func TestEnterTileTriggerFires(t *testing.T) {
	g := &GameState{
		Visited: [][]bool{{true, true}, {true, true}},
		Area: AreaDefinition{
			Dialogs:  []DialogDefinition{sampleDialog()},
			Triggers: []DialogTrigger{{ID: "t1", Kind: DialogTriggerEnterTile, DialogID: "d1", TileX: 1, TileZ: 0, Once: true}},
		},
	}
	if FireEnterTileTriggers(g, 0, 0) {
		t.Fatal("a non-matching tile should not fire the trigger")
	}
	if !FireEnterTileTriggers(g, 1, 0) || !g.DialogOpen {
		t.Fatal("entering the trigger tile should open the dialog")
	}
	// Once-trigger: after closing, re-entering must not re-fire.
	CloseDialog(g)
	if FireEnterTileTriggers(g, 1, 0) {
		t.Fatal("a Once enter-tile trigger should not fire a second time")
	}
}

func TestEnterTileTriggerNonOnceRepeats(t *testing.T) {
	g := &GameState{
		Area: AreaDefinition{
			Dialogs:  []DialogDefinition{sampleDialog()},
			Triggers: []DialogTrigger{{ID: "t1", Kind: DialogTriggerEnterTile, DialogID: "d1", TileX: 0, TileZ: 0}},
		},
	}
	if !FireEnterTileTriggers(g, 0, 0) {
		t.Fatal("non-Once trigger should fire")
	}
	CloseDialog(g)
	if !FireEnterTileTriggers(g, 0, 0) {
		t.Fatal("a non-Once enter-tile trigger should fire again on re-entry")
	}
}

func TestFoeKilledTriggerFires(t *testing.T) {
	kind := EnemyKinds()[0].Kind
	g := &GameState{
		Bestiary: make(Bestiary),
		Area: AreaDefinition{
			Dialogs:  []DialogDefinition{sampleDialog()},
			Triggers: []DialogTrigger{{ID: "t1", Kind: DialogTriggerFoeKilled, DialogID: "d1", FoeKind: kind, FoeKills: 1, Once: true}},
		},
	}
	if FireFoeKilledTriggers(g) {
		t.Fatal("foe-killed trigger should not fire before the kill threshold")
	}
	g.Bestiary.RecordKill(kind)
	if !FireFoeKilledTriggers(g) || !g.DialogOpen {
		t.Fatal("foe-killed trigger should fire once the threshold is met")
	}
	CloseDialog(g)
	if FireFoeKilledTriggers(g) {
		t.Fatal("a Once foe-killed trigger should not re-fire")
	}
}

func TestTriggerNoFireWhileDialogOpen(t *testing.T) {
	g := &GameState{
		Area: AreaDefinition{
			Dialogs:  []DialogDefinition{sampleDialog()},
			Triggers: []DialogTrigger{{ID: "t1", Kind: DialogTriggerEnterTile, DialogID: "d1", TileX: 0, TileZ: 0}},
		},
	}
	g.DialogOpen = true // a conversation is already up
	if FireEnterTileTriggers(g, 0, 0) {
		t.Fatal("a trigger must not stomp an already-open dialog")
	}
}

func TestTriggersJSONRoundTrip(t *testing.T) {
	in := []DialogTrigger{
		{ID: "t1", Kind: DialogTriggerEnterTile, DialogID: "d1", TileX: 3, TileZ: 4, Once: true},
		{ID: "t2", Kind: DialogTriggerFoeKilled, DialogID: "d2", FoeKind: EnemyKinds()[1].Kind, FoeKills: 5},
	}
	lines, err := TriggersToLines(in)
	if err != nil {
		t.Fatalf("TriggersToLines: %v", err)
	}
	out, err := TriggersFromLines(lines)
	if err != nil {
		t.Fatalf("TriggersFromLines: %v", err)
	}
	if !slicesEqualTriggers(in, out) {
		t.Fatalf("round-trip mismatch:\n in=%+v\nout=%+v", in, out)
	}
}

func slicesEqualTriggers(a, b []DialogTrigger) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestStartDialogClonesDefinition(t *testing.T) {
	g := newDialogGame()
	StartDialog(g, "d1")
	// Mutating the live conversation's copy must not bleed into the area's
	// authored definition — StartDialog deep-copies so the two share no
	// backing array (the audit's shallow-copy fix).
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
	// Every advertised condition kind must be handled by evalDialogCondition
	// (not fall through to the "Unavailable" default) — the kind dropdown is
	// built from this list, so a listed-but-unhandled kind would be a dead row.
	g := &GameState{Bestiary: make(Bestiary)}
	for _, k := range DialogCondKinds() {
		_, reason := evalDialogCondition(g, DialogChoiceCondition{Kind: k})
		if reason == "Unavailable" {
			t.Errorf("condition kind %q is advertised by DialogCondKinds but unhandled by evalDialogCondition", k)
		}
	}
	if len(DialogTriggerKinds()) == 0 {
		t.Fatal("DialogTriggerKinds must not be empty")
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
	// Detached copy: mutating the live map must not change the snapshot.
	g.TriggersFired["t1"] = false
	if !data.TriggersFired["t1"] {
		t.Fatal("save snapshot should be detached from the live TriggersFired map")
	}
}

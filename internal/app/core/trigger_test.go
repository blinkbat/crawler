package core

import "testing"

// TestTriggerConditionKindsCoverEval asserts every kind advertised by
// ConditionKinds() is actually handled by evalConditionRaw. The eval switch has a
// silent default (an authoring typo must be a no-op, not a panic), so a kind added
// to the list but forgotten in the switch would otherwise ship as a dead authoring
// option. Each kind is fed a state that MUST satisfy it if handled. CondNever is the
// one advertised kind that legitimately evaluates false, so it's asserted separately.
func TestTriggerConditionKindsCoverEval(t *testing.T) {
	g := &GameState{
		Bestiary:  Bestiary{EnemyRat: {Kills: 1}},
		Switches:  map[string]bool{"s": true},
		Counters:  map[string]int{"c": 1},
		Gold:      10,
		Quests:    []Quest{{ID: "q", Status: QuestActive}},
		Visited:   [][]bool{{true}},
		TriggersFired: map[string]bool{},
	}
	g.Area.Locations = []Location{{ID: "L", X: 0, Z: 0, W: 1, H: 1}}
	// Player already at (0,0,0) by zero value — satisfies enterTile / atLocation / tileVisited.

	sat := map[ConditionKind]Condition{
		CondAlways:      {Kind: CondAlways},
		CondSwitch:      {Kind: CondSwitch, Switch: "s"},
		CondCounter:     {Kind: CondCounter, Counter: "c", Count: 1},
		CondEnterTile:   {Kind: CondEnterTile},
		CondAtLocation:  {Kind: CondAtLocation, LocationID: "L"},
		CondFoeKilled:   {Kind: CondFoeKilled, FoeKind: EnemyRat},
		CondTileVisited: {Kind: CondTileVisited},
		CondQuest:       {Kind: CondQuest, QuestID: "q", QuestStatus: QuestActive},
		CondGold:        {Kind: CondGold, Count: 5},
	}
	for _, k := range ConditionKinds() {
		if k == CondNever {
			if evalConditionRaw(g, Condition{Kind: CondNever}) {
				t.Errorf("CondNever must evaluate false")
			}
			continue
		}
		c, ok := sat[k]
		if !ok {
			t.Errorf("condition kind %q is advertised by ConditionKinds but has no satisfying case in this test — wire it into evalConditionRaw and add a case here", k)
			continue
		}
		if !evalConditionRaw(g, c) {
			t.Errorf("condition kind %q is advertised by ConditionKinds but not satisfied by evalConditionRaw for a state that should meet it (unhandled in the switch?)", k)
		}
	}
}

// TestTriggerActionKindsCoverEval asserts every kind advertised by ActionKinds() is
// handled by runAction. Like the condition switch, runAction's default is a silent
// no-op, so a forgotten case would ship as a dead authoring option. Each kind is run
// and its observable effect checked. ActionEvent is a documented no-op seam and
// ActionDialog requires an authored def, so both are asserted separately.
func TestTriggerActionKindsCoverEval(t *testing.T) {
	covered := map[ActionKind]func(t *testing.T){
		ActionSetSwitch: func(t *testing.T) {
			g := &GameState{}
			runAction(g, Action{Kind: ActionSetSwitch, Switch: "s", SwitchOp: SwitchSet})
			if !g.Switches["s"] {
				t.Error("ActionSetSwitch did not set the switch")
			}
		},
		ActionSetCounter: func(t *testing.T) {
			g := &GameState{}
			runAction(g, Action{Kind: ActionSetCounter, Counter: "c", CounterOp: CounterSet, Count: 3})
			if g.Counters["c"] != 3 {
				t.Error("ActionSetCounter did not set the counter")
			}
		},
		ActionGiveGold: func(t *testing.T) {
			g := &GameState{}
			runAction(g, Action{Kind: ActionGiveGold, Count: 7})
			if g.Gold != 7 {
				t.Error("ActionGiveGold did not add gold")
			}
		},
		ActionQuest: func(t *testing.T) {
			g := &GameState{}
			runAction(g, Action{Kind: ActionQuest, QuestID: "q", QuestOp: DialogQuestStart})
			if QuestIndexByID(g.Quests, "q") < 0 {
				t.Error("ActionQuest did not start the quest")
			}
		},
		ActionMessage: func(t *testing.T) {
			g := &GameState{}
			runAction(g, Action{Kind: ActionMessage, Text: "hi"})
			if len(g.ActionLog) == 0 {
				t.Error("ActionMessage did not log")
			}
		},
	}
	// Spatial effects (spawnFoe/spawnChest/openWall/teleport) and dialog need a live
	// area/def; they're exercised by their own tests. Here we only assert they're not
	// silently dropped from the advertised list without a home.
	spatialOrModal := map[ActionKind]bool{
		ActionSpawnFoe: true, ActionSpawnChest: true, ActionOpenWall: true,
		ActionTeleport: true, ActionDialog: true, ActionEvent: true,
	}
	for _, k := range ActionKinds() {
		if fn, ok := covered[k]; ok {
			fn(t)
			continue
		}
		if !spatialOrModal[k] {
			t.Errorf("action kind %q is advertised by ActionKinds but has no coverage here — add a check or list it as spatial/modal", k)
		}
	}
}

package core

// ============================================================================
// Trigger system — modeled on the StarEdit (StarCraft) campaign trigger editor.
//
// A Trigger holds CONDITIONS (all must hold — logical AND) and ACTIONS (run in
// order once the conditions hold). By default a trigger fires ONCE then disables;
// Preserve=true keeps it live across evaluations (StarEdit "Preserve Trigger").
//
// The shared state conditions read and actions write — named boolean Switches and
// integer Counters — lives on GameState and persists in the save. Triggers are
// POLLED: EvaluateTriggers runs on world beats (a step landing, a battle win, a
// wall interaction, and after actions mutate switches/counters). "Start a dialog"
// is just one Action kind alongside spawnFoe / spawnChest / openWall / setSwitch /
// setCounter / teleport / giveGold / quest / message — a broad, extensible catalog.
// ============================================================================

// maxTriggerPasses caps the setSwitch→condition cascade within a single
// EvaluateTriggers call so a pair of Preserve triggers that toggle each other's
// switch can't spin forever. Each trigger still fires at most once per call.
const maxTriggerPasses = 16

// Comparator is a numeric test used by count-bearing conditions (counters, foe
// kills, gold). Empty defaults to atLeast (the StarEdit default).
type Comparator string

const (
	CmpAtLeast Comparator = "atLeast"
	CmpAtMost  Comparator = "atMost"
	CmpExactly Comparator = "exactly"
)

// Comparators returns the comparators in canonical (editor display) order.
func Comparators() []Comparator { return []Comparator{CmpAtLeast, CmpAtMost, CmpExactly} }

// compareCount applies cmp to (have, want). Empty cmp = atLeast.
func compareCount(have, want int, cmp Comparator) bool {
	switch cmp {
	case CmpAtMost:
		return have <= want
	case CmpExactly:
		return have == want
	default: // CmpAtLeast / empty
		return have >= want
	}
}

// ---------------------------------------------------------------------------
// Conditions
// ---------------------------------------------------------------------------

// ConditionKind tags one test in a trigger's AND-list.
type ConditionKind string

const (
	CondAlways      ConditionKind = "always"      // unconditional (StarEdit "Always")
	CondNever       ConditionKind = "never"       // disables the trigger (StarEdit "Never")
	CondSwitch      ConditionKind = "switch"      // named Switch is set (Negate → cleared)
	CondCounter     ConditionKind = "counter"     // Counter[Name] <Cmp> Count
	CondEnterTile   ConditionKind = "enterTile"   // player stands on (TileX,TileZ[,Level])
	CondAtLocation  ConditionKind = "atLocation"  // player inside region LocationID
	CondFoeKilled   ConditionKind = "foeKilled"   // bestiary kills of FoeKind <Cmp> Count
	CondTileVisited ConditionKind = "tileVisited" // (TileX,TileZ) revealed on the fog grid
	CondQuest       ConditionKind = "quest"       // quest QuestID at QuestStatus
	CondGold        ConditionKind = "gold"        // party gold <Cmp> Count
)

// ConditionKinds returns the authorable condition kinds in canonical (editor
// display) order — the single source the editor's picker reads and asserts against.
func ConditionKinds() []ConditionKind {
	return []ConditionKind{
		CondAlways, CondSwitch, CondCounter, CondEnterTile, CondAtLocation,
		CondFoeKilled, CondTileVisited, CondQuest, CondGold, CondNever,
	}
}

// Condition is one test in a trigger. The fields are a union keyed by Kind; unused
// fields stay zero (mirrors DialogChoiceCondition's union).
type Condition struct {
	Kind   ConditionKind `json:"kind"`
	Negate bool          `json:"negate,omitempty"` // logical NOT of the whole test
	// spatial (enterTile / tileVisited)
	TileX int `json:"tileX,omitempty"`
	TileZ int `json:"tileZ,omitempty"`
	Level int `json:"level,omitempty"`
	// region (atLocation)
	LocationID string `json:"locationId,omitempty"`
	// foe (foeKilled). FoeKind NOT omitempty — EnemyRat==0.
	FoeKind EnemyKind `json:"foeKind"`
	// switch / counter names
	Switch  string `json:"switch,omitempty"`
	Counter string `json:"counter,omitempty"`
	// quest (quest)
	QuestID     string      `json:"questId,omitempty"`
	QuestStatus QuestStatus `json:"questStatus,omitempty"`
	// numeric threshold + comparator (counter / foeKilled / gold)
	Count int        `json:"count,omitempty"`
	Cmp   Comparator `json:"cmp,omitempty"`
}

// evalCondition reports whether c holds in g (Negate flips the result).
func evalCondition(g *GameState, c Condition) bool {
	return c.Negate != evalConditionRaw(g, c)
}

func evalConditionRaw(g *GameState, c Condition) bool {
	if g == nil {
		return false
	}
	switch c.Kind {
	case CondAlways:
		return true
	case CondNever:
		return false
	case CondSwitch:
		return g.Switches[c.Switch]
	case CondCounter:
		return compareCount(g.Counters[c.Counter], c.Count, c.Cmp)
	case CondEnterTile:
		return g.Player.TileX == c.TileX && g.Player.TileZ == c.TileZ &&
			(c.Level == 0 || g.Player.Level == c.Level)
	case CondAtLocation:
		if loc, ok := LocationByID(g.Area.Locations, c.LocationID); ok {
			return loc.Contains(g.Player.TileX, g.Player.TileZ, g.Player.Level)
		}
		return false
	case CondFoeKilled:
		return foeKillCountMet(g, c.FoeKind, c.Count, c.Cmp)
	case CondTileVisited:
		return tileVisited(g, c.TileX, c.TileZ)
	case CondQuest:
		return questStatusMet(g, c.QuestID, c.QuestStatus)
	case CondGold:
		return goldMet(g, c.Count, c.Cmp)
	default:
		return false // unknown kind (authoring typo) — treat as unmet, don't panic
	}
}

// allConditionsMet reports whether every condition holds (empty list = true, so a
// trigger with no conditions is unconditional — StarEdit treats no-condition as
// Always).
func allConditionsMet(g *GameState, conds []Condition) bool {
	for _, c := range conds {
		if !evalCondition(g, c) {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// Actions
// ---------------------------------------------------------------------------

// ActionKind tags the Action union. Shared by triggers AND dialog node/choice
// end-actions (the consolidation: "start a dialog" is just one kind).
type ActionKind string

const (
	ActionDialog     ActionKind = "dialog"     // start conversation DialogID
	ActionSetSwitch  ActionKind = "setSwitch"  // Switch := SwitchOp
	ActionSetCounter ActionKind = "setCounter" // Counter[Name] := / += Count
	ActionSpawnFoe   ActionKind = "spawnFoe"   // create a pack of FoeKind at (TileX,TileZ[,Level])
	ActionSpawnChest ActionKind = "spawnChest" // create a chest holding Items at (TileX,TileZ[,Level])
	ActionOpenWall   ActionKind = "openWall"   // clear the blocker at (TileX,TileZ[,Level])
	ActionTeleport   ActionKind = "teleport"   // move the party to (TileX,TileZ[,Level])
	ActionGiveGold   ActionKind = "giveGold"   // party gold += Count (may be negative)
	ActionQuest      ActionKind = "quest"      // start/complete QuestID
	ActionMessage    ActionKind = "message"    // log Text to the action log / status line
	ActionEvent      ActionKind = "event"      // generic named seam (EventID)
)

// ActionKinds returns the non-empty action kinds in canonical (editor display)
// order — the source the editor's action picker asserts coverage against.
func ActionKinds() []ActionKind {
	return []ActionKind{
		ActionDialog, ActionSetSwitch, ActionSetCounter, ActionSpawnFoe,
		ActionSpawnChest, ActionOpenWall, ActionTeleport, ActionGiveGold,
		ActionQuest, ActionMessage, ActionEvent,
	}
}

// SwitchOp is what a setSwitch action does to its target switch.
type SwitchOp string

const (
	SwitchSet    SwitchOp = "set"
	SwitchClear  SwitchOp = "clear"
	SwitchToggle SwitchOp = "toggle"
)

// SwitchOps returns the switch operations in canonical (editor) order.
func SwitchOps() []SwitchOp { return []SwitchOp{SwitchSet, SwitchClear, SwitchToggle} }

// CounterOp is what a setCounter action does to its target counter.
type CounterOp string

const (
	CounterSet CounterOp = "set"
	CounterAdd CounterOp = "add"
)

// CounterOps returns the counter operations in canonical (editor) order.
func CounterOps() []CounterOp { return []CounterOp{CounterSet, CounterAdd} }

// Action is the effect a trigger (or dialog node/choice) fires. Fields are a union
// keyed by Kind; unused fields stay zero.
type Action struct {
	Kind ActionKind `json:"kind,omitempty"`
	// dialog
	DialogID string `json:"dialogId,omitempty"`
	// quest
	QuestID string        `json:"questId,omitempty"`
	QuestOp DialogQuestOp `json:"questOp,omitempty"`
	// event seam
	EventID string `json:"eventId,omitempty"`
	// switch / counter
	Switch    string    `json:"switch,omitempty"`
	SwitchOp  SwitchOp  `json:"switchOp,omitempty"`
	Counter   string    `json:"counter,omitempty"`
	CounterOp CounterOp `json:"counterOp,omitempty"`
	// spatial (spawnFoe / spawnChest / openWall / teleport)
	TileX int `json:"tileX,omitempty"`
	TileZ int `json:"tileZ,omitempty"`
	Level int `json:"level,omitempty"`
	// spawnFoe
	FoeKind EnemyKind `json:"foeKind,omitempty"`
	// spawnChest
	Items []ItemKind `json:"items,omitempty"`
	// giveGold / setCounter numeric payload
	Count int `json:"count,omitempty"`
	// message
	Text string `json:"text,omitempty"`
}

// runAction performs one action against g. Data-driven: an unknown/blank kind is a
// no-op (never a panic), mirroring the rest of the authoring surface.
func runAction(g *GameState, a Action) {
	if g == nil {
		return
	}
	switch a.Kind {
	case ActionDialog:
		StartDialog(g, a.DialogID)
	case ActionSetSwitch:
		if a.Switch == "" {
			return
		}
		if g.Switches == nil {
			g.Switches = map[string]bool{}
		}
		switch a.SwitchOp {
		case SwitchClear:
			g.Switches[a.Switch] = false
		case SwitchToggle:
			g.Switches[a.Switch] = !g.Switches[a.Switch]
		default: // SwitchSet / empty
			g.Switches[a.Switch] = true
		}
	case ActionSetCounter:
		if a.Counter == "" {
			return
		}
		if g.Counters == nil {
			g.Counters = map[string]int{}
		}
		if a.CounterOp == CounterAdd {
			g.Counters[a.Counter] += a.Count
		} else {
			g.Counters[a.Counter] = a.Count
		}
	case ActionSpawnFoe:
		SpawnFoeAt(g, a.FoeKind, a.TileX, a.TileZ, a.Level)
	case ActionSpawnChest:
		SpawnChestAt(g, a.TileX, a.TileZ, a.Level, a.Items)
	case ActionOpenWall:
		OpenWallAt(g, a.TileX, a.TileZ, a.Level)
	case ActionTeleport:
		TeleportParty(g, a.TileX, a.TileZ, a.Level)
	case ActionGiveGold:
		g.Gold += a.Count
		if g.Gold < 0 {
			g.Gold = 0
		}
	case ActionQuest:
		applyQuestOp(g, a.QuestID, a.QuestOp)
	case ActionMessage:
		if a.Text != "" {
			g.LogMessage(a.Text)
		}
	case ActionEvent:
		// No event registry yet; seam for a future switch on a.EventID.
	default:
		// Unknown action kind (authoring typo / unhandled new kind) — do nothing.
	}
}

// ApplyAction runs a single optional action (nil = no-op). The public entry the
// dialog node/choice end-action path uses; triggers call runAction in a loop.
func ApplyAction(g *GameState, a *Action) {
	if a == nil {
		return
	}
	runAction(g, *a)
}

// ActionsEqual reports whether two actions are value-equal (the Items slice rules
// out ==). Exported for the editor's no-op-change guards.
func ActionsEqual(a, b Action) bool { return actionValueEqual(a, b) }

// applyQuestOp starts or completes a quest by id (blank id / unknown op = no-op).
func applyQuestOp(g *GameState, questID string, op DialogQuestOp) {
	if questID == "" {
		return
	}
	switch op {
	case DialogQuestComplete:
		CompleteQuest(g, questID)
	default: // DialogQuestStart / empty — an unset op starts the quest (editor default)
		g.Quests = AddQuest(g.Quests, Quest{ID: questID, Title: QuestTitleFromID(questID), Status: QuestActive})
	}
}

// ---------------------------------------------------------------------------
// Trigger + evaluation
// ---------------------------------------------------------------------------

// Trigger fires its Actions (in order) when every Condition holds. By default it
// fires once then disables (recorded in GameState.TriggersFired by ID); Preserve
// keeps it live. ID is the fired-set key and must be unique on the map.
type Trigger struct {
	ID         string      `json:"id"`
	Conditions []Condition `json:"conditions,omitempty"`
	Actions    []Action    `json:"actions,omitempty"`
	Preserve   bool        `json:"preserve,omitempty"`
}

// TriggersToLines marshals triggers for the .map triggers: section.
func TriggersToLines(triggers []Trigger) ([]string, error) {
	return jsonObjectsToLines(triggers, "trigger", func(t Trigger) string { return t.ID })
}

// TriggersFromLines unmarshals the .map triggers: section into triggers.
func TriggersFromLines(lines []string) ([]Trigger, error) {
	return jsonObjectsFromLines[Trigger](lines, "trigger")
}

// triggerDisabled reports whether a trigger can't fire now: a non-Preserve trigger
// that already fired this session (recorded in TriggersFired).
func triggerDisabled(g *GameState, t *Trigger) bool {
	return !t.Preserve && g.TriggersFired[t.ID]
}

// markTriggerFired records a non-Preserve trigger as fired (lazy-init the set).
func markTriggerFired(g *GameState, id string) {
	if g.TriggersFired == nil {
		g.TriggersFired = make(map[string]bool)
	}
	g.TriggersFired[id] = true
}

// EvaluateTriggers polls every enabled trigger: those whose conditions all hold run
// their actions (in order); non-Preserve triggers then disable. A setSwitch/counter
// action can enable another trigger, so it loops until no further trigger fires
// (bounded by maxTriggerPasses), firing each trigger at most once per call. Stops
// early if an action opened a dialog (the modal owns input from here). Call on world
// beats: step landing, battle win, wall interaction, area (re)load.
func EvaluateTriggers(g *GameState) {
	if g == nil || g.DialogOpen || len(g.Area.Triggers) == 0 {
		return // a conversation owns the screen — don't fire triggers under it
	}
	firedThisCall := make(map[int]bool, len(g.Area.Triggers))
	for pass := 0; pass < maxTriggerPasses; pass++ {
		progressed := false
		for i := range g.Area.Triggers {
			if firedThisCall[i] {
				continue
			}
			t := &g.Area.Triggers[i]
			if triggerDisabled(g, t) || !allConditionsMet(g, t.Conditions) {
				continue
			}
			for _, a := range t.Actions {
				runAction(g, a)
			}
			firedThisCall[i] = true
			progressed = true
			if !t.Preserve {
				markTriggerFired(g, t.ID)
			}
			if g.DialogOpen {
				return // an action opened a conversation — let it take over
			}
		}
		if !progressed {
			return
		}
	}
}

// EvaluateWorldTriggers re-applies the WORLD-state effects of Preserve triggers whose
// conditions currently hold — called at load so switch/counter-gated openWall / spawn /
// teleport are reconstructed after the world rebuilds fresh from the .map. It runs ONLY
// Preserve triggers (a fire-once trigger's one-time effect isn't re-applied) and SKIPS
// conversational actions (dialog / message), so loading a save never pops a surprise
// conversation, and it never marks a trigger fired (a not-yet-fired one-shot still fires
// on a later real beat). One pass: the gating switches are already restored from the save.
func EvaluateWorldTriggers(g *GameState) {
	if g == nil {
		return
	}
	for i := range g.Area.Triggers {
		t := &g.Area.Triggers[i]
		if !t.Preserve || !allConditionsMet(g, t.Conditions) {
			continue
		}
		for _, a := range t.Actions {
			if a.Kind == ActionDialog || a.Kind == ActionMessage {
				continue // conversational — not a world-state effect; don't pop it on load
			}
			runAction(g, a)
		}
	}
}

// ---------------------------------------------------------------------------
// Shared predicate helpers (also used by dialog choice-conditions)
// ---------------------------------------------------------------------------

// foeKillCountMet reports whether bestiary kills of kind satisfy want under cmp.
// For the default comparator (atLeast/empty) the legacy "0 = once" rule applies.
func foeKillCountMet(g *GameState, kind EnemyKind, want int, cmp Comparator) bool {
	if g == nil {
		return false
	}
	if cmp == "" || cmp == CmpAtLeast {
		want = RequiredFoeKills(want) // legacy "0 = once" rule for the default cmp
	}
	return compareCount(g.Bestiary.Entry(kind).Kills, want, cmp)
}

// goldMet reports whether party gold satisfies want under cmp (empty cmp = atLeast).
func goldMet(g *GameState, want int, cmp Comparator) bool {
	if g == nil {
		return false
	}
	return compareCount(g.Gold, want, cmp)
}

// questStatusMet reports whether the party's quest questID is exactly at status.
func questStatusMet(g *GameState, questID string, status QuestStatus) bool {
	if g == nil {
		return false
	}
	if idx := QuestIndexByID(g.Quests, questID); idx >= 0 {
		return g.Quests[idx].Status == status
	}
	return false
}

// RequiredFoeKills normalizes a foe-kill threshold: <= 0 means once.
func RequiredFoeKills(kills int) int {
	if kills < 1 {
		return 1
	}
	return kills
}

// tileVisited reports whether (x,z) is revealed on the Visited grid (bounds-checked).
func tileVisited(g *GameState, x, z int) bool {
	if g == nil || z < 0 || z >= len(g.Visited) || x < 0 || x >= len(g.Visited[z]) {
		return false
	}
	return g.Visited[z][x]
}

// FoeKindName resolves an enemy kind to its singular display name (fallback for unregistered kinds).
func FoeKindName(kind EnemyKind) string {
	if def, ok := EnemyInfoOk(kind); ok {
		return def.SingularName
	}
	return "unknown foe"
}

package core

// Dialog triggers — the world-event seam that auto-starts an authored
// conversation (the bg2 "BAF"/area-script trigger, scoped to this crawler).
// A trigger names a dialog id plus the event that fires it:
//
//   - enterTile: the player steps (a completed step) onto tile (X,Z).
//   - foeKilled: the party has defeated FoeKind at least FoeKills times.
//
// Triggers live on the AreaDefinition (authored in the editor, round-tripped
// in the .map `triggers:` section like dialogs). The runtime fires them from
// explore (enter-tile, on step-land) and battle (foe-killed, on a won fight),
// guarded by a fired set (GameState.TriggersFired, persisted via SaveData) so
// a `Once` trigger doesn't repeat.
//
// The foeKilled/tileVisited predicates here are shared with the choice-gating
// conditions in dialog.go (DialogCondFoeKilled / DialogCondTileVisited) — one
// definition of "has this happened?" backs both the gate and the trigger.

// DialogTriggerKind tags which world event fires a trigger.
type DialogTriggerKind string

const (
	DialogTriggerEnterTile DialogTriggerKind = "enterTile"
	DialogTriggerFoeKilled DialogTriggerKind = "foeKilled"
)

// DialogTriggerKinds returns the authorable trigger kinds in canonical (editor
// display) order — the single source the editor's kind dropdown iterates, so
// adding a kind doesn't mean editing a parallel slice in the editor.
func DialogTriggerKinds() []DialogTriggerKind {
	return []DialogTriggerKind{DialogTriggerEnterTile, DialogTriggerFoeKilled}
}

// DialogTrigger auto-starts DialogID when its event fires. Once = fire only
// the first time (tracked in GameState.TriggersFired, keyed by ID, and
// persisted through SaveData so a saved-past Once trigger stays fired on
// reload); a non-Once trigger can re-fire (a recurring greeting on a tile). ID
// is author-stable (auto-generated in the editor) and is the fired-set key.
type DialogTrigger struct {
	ID       string            `json:"id"`
	Kind     DialogTriggerKind `json:"kind"`
	DialogID string            `json:"dialog"`
	Once     bool              `json:"once,omitempty"`
	// enterTile params.
	TileX int `json:"tileX,omitempty"`
	TileZ int `json:"tileZ,omitempty"`
	// foeKilled params (FoeKills <= 0 means "at least once"). FoeKind is NOT
	// omitempty: EnemyRat==0, so a Rat trigger must still serialize the field
	// or it reads back as an unauthored/ambiguous gate.
	FoeKind  EnemyKind `json:"foeKind"`
	FoeKills int       `json:"foeKills,omitempty"`
}

// TriggersToLines marshals each trigger to a single-line JSON object for the
// .map file's triggers: section (mirrors DialogsToLines).
func TriggersToLines(triggers []DialogTrigger) ([]string, error) {
	return jsonObjectsToLines(triggers, "trigger", func(t DialogTrigger) string { return t.ID })
}

// TriggersFromLines unmarshals the .map triggers: section (one JSON object per
// line) back into triggers (mirrors DialogsFromLines).
func TriggersFromLines(lines []string) ([]DialogTrigger, error) {
	return jsonObjectsFromLines[DialogTrigger](lines, "trigger")
}

// foeKillCountMet reports whether the bestiary records at least the required
// kills of kind (kills <= 0 means "at least one"). Shared by the foeKilled
// choice condition and the foeKilled trigger.
func foeKillCountMet(g *GameState, kind EnemyKind, kills int) bool {
	if g == nil {
		return false
	}
	return g.Bestiary.Entry(kind).Kills >= RequiredFoeKills(kills)
}

// RequiredFoeKills normalizes an authored foe-kill threshold: a value <= 0
// means "at least once". The single home for the "0 = 1" rule, shared by the
// foeKilled condition/trigger eval and the editor's summary labels so a label
// can't drift from what the eval actually requires.
func RequiredFoeKills(kills int) int {
	if kills < 1 {
		return 1
	}
	return kills
}

// tileVisited reports whether (x,z) is revealed on the Visited fog grid.
// Bounds-checked, so an out-of-range authored coord reads as "not visited"
// rather than panicking. Shared by the tileVisited condition + trigger.
func tileVisited(g *GameState, x, z int) bool {
	if g == nil || z < 0 || z >= len(g.Visited) || x < 0 || x >= len(g.Visited[z]) {
		return false
	}
	return g.Visited[z][x]
}

// FoeKindName resolves an enemy kind to its singular display name, falling
// back to a generic phrase for an unregistered kind (a hand-edited map could
// carry one). Shared by dialog condition reasons and the editor's foe labels
// so the "kind → name with fallback" rule lives in exactly one place.
func FoeKindName(kind EnemyKind) string {
	if def, ok := EnemyInfoOk(kind); ok {
		return def.SingularName
	}
	return "unknown foe"
}

// triggerAlreadyFired reports whether a Once trigger has fired this session.
func triggerAlreadyFired(g *GameState, t DialogTrigger) bool {
	return t.Once && g.TriggersFired[t.ID]
}

// markTriggerFired records a Once trigger as fired (lazy-initialising the set).
// Non-Once triggers are never recorded — they're allowed to re-fire.
func markTriggerFired(g *GameState, t DialogTrigger) {
	if !t.Once {
		return
	}
	if g.TriggersFired == nil {
		g.TriggersFired = make(map[string]bool)
	}
	g.TriggersFired[t.ID] = true
}

// fireFirstMatchingTrigger starts the first not-yet-fired trigger that satisfies
// pred and returns true if one opened. Shared body behind the per-kind Fire*
// wrappers: no-op (false) when a dialog is already open, so a trigger can't stomp
// an in-progress conversation, and only one dialog starts per call.
func fireFirstMatchingTrigger(g *GameState, pred func(DialogTrigger) bool) bool {
	if g == nil || g.DialogOpen {
		return false
	}
	for _, t := range g.Area.Triggers {
		if !pred(t) || triggerAlreadyFired(g, t) {
			continue
		}
		if StartDialog(g, t.DialogID) {
			markTriggerFired(g, t)
			return true
		}
	}
	return false
}

// FireEnterTileTriggers starts the first eligible enter-tile dialog for tile
// (x,z) and returns true if one opened.
func FireEnterTileTriggers(g *GameState, x, z int) bool {
	return fireFirstMatchingTrigger(g, func(t DialogTrigger) bool {
		return t.Kind == DialogTriggerEnterTile && t.TileX == x && t.TileZ == z
	})
}

// FireFoeKilledTriggers starts the first eligible foe-killed dialog whose
// bestiary threshold is now met and returns true if one opened. Called once a
// battle is won (bestiary kills already credited).
func FireFoeKilledTriggers(g *GameState) bool {
	return fireFirstMatchingTrigger(g, func(t DialogTrigger) bool {
		return t.Kind == DialogTriggerFoeKilled && foeKillCountMet(g, t.FoeKind, t.FoeKills)
	})
}

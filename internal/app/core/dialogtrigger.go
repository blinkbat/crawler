package core

// Dialog triggers — the world-event seam that auto-starts an authored
// conversation. A trigger names a dialog id plus the event that fires it:
//
//   - enterTile: player completes a step onto tile (X,Z).
//   - foeKilled: party has defeated FoeKind at least FoeKills times.
//
// Triggers live on the AreaDefinition (round-tripped in the .map `triggers:`
// section). The runtime fires them from explore (enter-tile, on step-land) and
// battle (foe-killed, on a won fight), guarded by GameState.TriggersFired so a
// `Once` trigger doesn't repeat. The foeKilled/tileVisited predicates are
// shared with dialog.go's choice-gating conditions.

// DialogTriggerKind tags which world event fires a trigger.
type DialogTriggerKind string

const (
	DialogTriggerEnterTile DialogTriggerKind = "enterTile"
	DialogTriggerFoeKilled DialogTriggerKind = "foeKilled"
)

// DialogTriggerKinds returns the authorable trigger kinds in canonical (editor
// display) order — the single source the editor's kind dropdown iterates.
func DialogTriggerKinds() []DialogTriggerKind {
	return []DialogTriggerKind{DialogTriggerEnterTile, DialogTriggerFoeKilled}
}

// DialogTrigger auto-starts DialogID when its event fires. Once = fire only
// the first time (tracked in GameState.TriggersFired by ID, persisted via
// SaveData); a non-Once trigger can re-fire. ID is the fired-set key.
type DialogTrigger struct {
	ID       string            `json:"id"`
	Kind     DialogTriggerKind `json:"kind"`
	DialogID string            `json:"dialog"`
	Once     bool              `json:"once,omitempty"`
	// enterTile params.
	TileX int `json:"tileX,omitempty"`
	TileZ int `json:"tileZ,omitempty"`
	// foeKilled params (FoeKills <= 0 means "at least once"). FoeKind is NOT
	// omitempty: EnemyRat==0, so a Rat trigger must still serialize the field.
	FoeKind  EnemyKind `json:"foeKind"`
	FoeKills int       `json:"foeKills,omitempty"`
}

// TriggersToLines marshals triggers for the .map triggers: section.
func TriggersToLines(triggers []DialogTrigger) ([]string, error) {
	return jsonObjectsToLines(triggers, "trigger", func(t DialogTrigger) string { return t.ID })
}

// TriggersFromLines unmarshals the .map triggers: section into triggers.
func TriggersFromLines(lines []string) ([]DialogTrigger, error) {
	return jsonObjectsFromLines[DialogTrigger](lines, "trigger")
}

// foeKillCountMet reports whether the bestiary records at least the required
// kills of kind. Shared by the foeKilled condition and trigger.
func foeKillCountMet(g *GameState, kind EnemyKind, kills int) bool {
	if g == nil {
		return false
	}
	return g.Bestiary.Entry(kind).Kills >= RequiredFoeKills(kills)
}

// RequiredFoeKills normalizes a foe-kill threshold: <= 0 means "at least
// once". The single home for the "0 = 1" rule (eval + editor labels can't drift).
func RequiredFoeKills(kills int) int {
	if kills < 1 {
		return 1
	}
	return kills
}

// tileVisited reports whether (x,z) is revealed on the Visited fog grid.
// Bounds-checked (out-of-range reads as "not visited"). Shared by the
// tileVisited condition + trigger.
func tileVisited(g *GameState, x, z int) bool {
	if g == nil || z < 0 || z >= len(g.Visited) || x < 0 || x >= len(g.Visited[z]) {
		return false
	}
	return g.Visited[z][x]
}

// FoeKindName resolves an enemy kind to its singular display name, falling
// back to a generic phrase for an unregistered kind. Shared by dialog
// condition reasons and the editor's foe labels.
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

// markTriggerFired records a Once trigger as fired (lazy-init the set).
// Non-Once triggers are never recorded — they may re-fire.
func markTriggerFired(g *GameState, t DialogTrigger) {
	if !t.Once {
		return
	}
	if g.TriggersFired == nil {
		g.TriggersFired = make(map[string]bool)
	}
	g.TriggersFired[t.ID] = true
}

// fireFirstMatchingTrigger starts the first not-yet-fired trigger satisfying
// pred and returns true if one opened. No-op when a dialog is already open (a
// trigger can't stomp an in-progress conversation); only one starts per call.
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

// FireEnterTileTriggers starts the first eligible enter-tile dialog for (x,z)
// and returns true if one opened.
func FireEnterTileTriggers(g *GameState, x, z int) bool {
	return fireFirstMatchingTrigger(g, func(t DialogTrigger) bool {
		return t.Kind == DialogTriggerEnterTile && t.TileX == x && t.TileZ == z
	})
}

// FireFoeKilledTriggers starts the first eligible foe-killed dialog whose
// threshold is now met. Called once a battle is won (kills already credited).
func FireFoeKilledTriggers(g *GameState) bool {
	return fireFirstMatchingTrigger(g, func(t DialogTrigger) bool {
		return t.Kind == DialogTriggerFoeKilled && foeKillCountMet(g, t.FoeKind, t.FoeKills)
	})
}

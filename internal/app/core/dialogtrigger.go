package core

// Dialog triggers auto-start an authored conversation on a world event
// (enterTile: a step onto (X,Z); foeKilled: FoeKind defeated >= FoeKills times).
// Fired from explore/battle, guarded by GameState.TriggersFired so a `Once`
// trigger doesn't repeat. The foeKilled/tileVisited predicates are shared with
// dialog.go's choice-gating conditions.

// DialogTriggerKind tags which world event fires a trigger.
type DialogTriggerKind string

const (
	DialogTriggerEnterTile     DialogTriggerKind = "enterTile"
	DialogTriggerEnterLocation DialogTriggerKind = "enterLocation"
	DialogTriggerFoeKilled     DialogTriggerKind = "foeKilled"
)

// DialogTriggerKinds returns the authorable trigger kinds in canonical order (the editor's dropdown source).
func DialogTriggerKinds() []DialogTriggerKind {
	return []DialogTriggerKind{DialogTriggerEnterTile, DialogTriggerEnterLocation, DialogTriggerFoeKilled}
}

// DialogTrigger auto-starts DialogID when its event fires. Once = fire only the
// first time (tracked in GameState.TriggersFired by ID). ID is the fired-set key.
type DialogTrigger struct {
	ID       string            `json:"id"`
	Kind     DialogTriggerKind `json:"kind"`
	DialogID string            `json:"dialog"`
	Once     bool              `json:"once,omitempty"`
	// enterTile params.
	TileX int `json:"tileX,omitempty"`
	TileZ int `json:"tileZ,omitempty"`
	// enterLocation param: the region (core.Location) whose crossing fires this.
	LocationID string `json:"locationId,omitempty"`
	// foeKilled params (FoeKills <=0 means once). FoeKind NOT omitempty (EnemyRat==0).
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

// foeKillCountMet reports whether the bestiary records the required kills. Shared by condition + trigger.
func foeKillCountMet(g *GameState, kind EnemyKind, kills int) bool {
	if g == nil {
		return false
	}
	return g.Bestiary.Entry(kind).Kills >= RequiredFoeKills(kills)
}

// RequiredFoeKills normalizes a foe-kill threshold: <= 0 means once. Single home for the "0 = 1" rule.
func RequiredFoeKills(kills int) int {
	if kills < 1 {
		return 1
	}
	return kills
}

// tileVisited reports whether (x,z) is revealed on the Visited grid (bounds-checked). Shared by condition + trigger.
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

// triggerAlreadyFired reports whether a Once trigger has fired this session.
func triggerAlreadyFired(g *GameState, t DialogTrigger) bool {
	return t.Once && g.TriggersFired[t.ID]
}

// markTriggerFired records a Once trigger as fired (lazy-init the set); non-Once never recorded.
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
// pred. No-op when a dialog is already open; only one starts per call.
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

// FireEnterTileTriggers starts the first eligible enter-tile dialog for (x,z).
func FireEnterTileTriggers(g *GameState, x, z int) bool {
	return fireFirstMatchingTrigger(g, func(t DialogTrigger) bool {
		return t.Kind == DialogTriggerEnterTile && t.TileX == x && t.TileZ == z
	})
}

// FireFoeKilledTriggers starts the first eligible foe-killed dialog. Called once a battle is won.
func FireFoeKilledTriggers(g *GameState) bool {
	return fireFirstMatchingTrigger(g, func(t DialogTrigger) bool {
		return t.Kind == DialogTriggerFoeKilled && foeKillCountMet(g, t.FoeKind, t.FoeKills)
	})
}

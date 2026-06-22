package core

// Bestiary — the party's knowledge of foes faced. A kind is "identified" (exact HP known, roster
// reveals numbers) by defeating BestiaryIDKills of them OR landing Scan on one. Entries are keyed
// by kind (not positional), so new fields can append without breaking saved data.

// BestiaryIDKills is how many defeats identify a kind the hard way; Scan bypasses it (MarkScanned).
const BestiaryIDKills = 5

// BestiaryEntry is the per-kind record: Kills counts defeats across all battles; Scanned is set on
// the first Scan of any instance. Known once either crosses the bar (Bestiary.Knows).
type BestiaryEntry struct {
	Kills   int  `json:"kills"`
	Scanned bool `json:"scanned,omitempty"`
}

// Bestiary maps an enemy kind to what the party has learned about it. Keyed by EnemyKind, which
// serializes as its integer value, so enemy kinds are APPEND-ONLY (inserting mid-enum mis-attributes
// saved entries). Custom enemies fold into their base Kind's entry for now.
type Bestiary map[EnemyKind]BestiaryEntry

// RecordKill credits one defeat of kind, creating the entry if absent. Pointer receiver lazy-inits a nil map.
func (b *Bestiary) RecordKill(kind EnemyKind) {
	if *b == nil {
		*b = make(Bestiary)
	}
	e := (*b)[kind]
	e.Kills++
	(*b)[kind] = e
}

// MarkScanned flags a kind as identified-by-scan, creating the entry if absent. Idempotent.
func (b *Bestiary) MarkScanned(kind EnemyKind) {
	if *b == nil {
		*b = make(Bestiary)
	}
	e := (*b)[kind]
	e.Scanned = true
	(*b)[kind] = e
}

// Knows reports whether the kind is identified — scanned OR Kills >= BestiaryIDKills. Nil-safe.
func (b Bestiary) Knows(kind EnemyKind) bool {
	e, ok := b[kind]
	return ok && (e.Scanned || e.Kills >= BestiaryIDKills)
}

// Seen reports whether the kind has any record (defeated once or scanned) — gates bestiary listing. Nil-safe.
func (b Bestiary) Seen(kind EnemyKind) bool {
	e, ok := b[kind]
	return ok && (e.Kills > 0 || e.Scanned)
}

// Entry returns the stored record for a kind (zero value if none). Nil-safe.
func (b Bestiary) Entry(kind EnemyKind) BestiaryEntry {
	return b[kind]
}

// SeenKinds returns every kind with a record, in canonical declaration order. Nil-safe.
func (b Bestiary) SeenKinds() []EnemyKind {
	return b.SeenKindsInto(nil)
}

// SeenKindsInto is the buffer-reusing variant of SeenKinds for per-frame callers; pass nil to
// allocate. Order matches SeenKinds.
func (b Bestiary) SeenKindsInto(buf []EnemyKind) []EnemyKind {
	out := buf[:0]
	if len(b) == 0 {
		return out
	}
	// Walk enemyDefinitions directly, not EnemyKinds() (whose defensive copy would allocate per frame).
	for i := range enemyDefinitions {
		if b.Seen(enemyDefinitions[i].Kind) {
			out = append(out, enemyDefinitions[i].Kind)
		}
	}
	return out
}

// SeenCount is the allocation-free counterpart of len(SeenKinds()) for the per-frame input path. Nil-safe.
func (b Bestiary) SeenCount() int {
	if len(b) == 0 {
		return 0
	}
	n := 0
	for i := range enemyDefinitions {
		if b.Seen(enemyDefinitions[i].Kind) {
			n++
		}
	}
	return n
}

// RecordBattleKills credits a defeat for every dead member of the active pack (each instance counts;
// dead summons too). Called from winBattle. No-op when no pack is engaged.
func RecordBattleKills(g *GameState) {
	if g == nil {
		return
	}
	for _, m := range BattleMembers(g) {
		if !m.Alive {
			g.Bestiary.RecordKill(m.Kind)
		}
	}
}

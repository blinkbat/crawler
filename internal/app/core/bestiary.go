package core

// Bestiary — the party's accumulated knowledge of the foes they've faced.
// Two ways to "identify" a kind (after which its exact HP is known and the
// battle roster reveals numbers for it): defeat BestiaryIDKills of them the
// hard way, or land the Thief's Scan on one (an instant shortcut). Kept
// deliberately small for now — kill count + a scanned flag per kind. Richer
// per-kind notes (stats, resistances, lore, drop tables) can append fields
// to BestiaryEntry later without breaking saved data, since entries are
// keyed by kind, not by positional layout.

// BestiaryIDKills is how many confirmed defeats of a kind it takes to
// identify it the hard way. Scan bypasses this entirely (see MarkScanned).
const BestiaryIDKills = 5

// BestiaryEntry is the per-kind knowledge record. Kills counts confirmed
// defeats across every battle; Scanned is set the first time a party member
// lands Scan on any instance of the kind. A kind is "known" once either
// crosses the bar (see Bestiary.Knows).
type BestiaryEntry struct {
	Kills   int  `json:"kills"`
	Scanned bool `json:"scanned,omitempty"`
}

// Bestiary maps an enemy kind to what the party has learned about it.
//
// Keyed by EnemyKind, which serializes as its integer value — so, exactly
// like ItemKind / SkillID, enemy kinds are APPEND-ONLY: inserting a kind
// mid-enum would renumber later kinds and mis-attribute saved bestiary
// entries. Custom enemies (DefinitionOverride) fold into their base Kind's
// entry for now; a future slice can split them out by giving the entry a
// per-custom-name sub-map.
type Bestiary map[EnemyKind]BestiaryEntry

// RecordKill credits one defeat of `kind`, creating the entry if absent.
// Pointer receiver so it can lazy-initialise a nil map (a fresh GameState
// seeds a non-nil one, but load / hand-edited paths may not).
func (b *Bestiary) RecordKill(kind EnemyKind) {
	if *b == nil {
		*b = make(Bestiary)
	}
	e := (*b)[kind]
	e.Kills++
	(*b)[kind] = e
}

// MarkScanned flags a kind as identified-by-scan, creating the entry if
// absent. Idempotent — re-scanning an already-known kind is a no-op beyond
// keeping the flag set.
func (b *Bestiary) MarkScanned(kind EnemyKind) {
	if *b == nil {
		*b = make(Bestiary)
	}
	e := (*b)[kind]
	e.Scanned = true
	(*b)[kind] = e
}

// Knows reports whether the party has identified the kind — scanned, OR
// defeated at least BestiaryIDKills of them. Drives the battle roster's HP
// reveal and the bestiary tab's "identified" state. Nil-safe.
func (b Bestiary) Knows(kind EnemyKind) bool {
	e, ok := b[kind]
	return ok && (e.Scanned || e.Kills >= BestiaryIDKills)
}

// Seen reports whether the kind has any record at all (defeated at least
// once or scanned) — the gate for listing it in the bestiary tab so the
// roster of unmet foes isn't spoiled. Nil-safe.
func (b Bestiary) Seen(kind EnemyKind) bool {
	e, ok := b[kind]
	return ok && (e.Kills > 0 || e.Scanned)
}

// Entry returns the stored record for a kind (zero value if none). Nil-safe.
func (b Bestiary) Entry(kind EnemyKind) BestiaryEntry {
	return b[kind]
}

// SeenKinds returns every kind with a record, in canonical EnemyKinds()
// declaration order — the stable list the bestiary tab draws and its row
// cursor indexes (render and input both call this, so the drawn rows and
// the navigable count can't drift). Nil-safe; nil when nothing's been seen.
func (b Bestiary) SeenKinds() []EnemyKind {
	return b.SeenKindsInto(nil)
}

// SeenKindsInto is the buffer-reusing variant of SeenKinds: it truncates buf
// and refills it, so a per-frame caller (the Bestiary tab draw) can hold one
// scratch slice and avoid a fresh allocation every frame. Pass nil for the
// allocating behaviour. Result order matches SeenKinds (canonical registry order).
func (b Bestiary) SeenKindsInto(buf []EnemyKind) []EnemyKind {
	out := buf[:0]
	if len(b) == 0 {
		return out
	}
	// Walk the registry slice directly rather than EnemyKinds(), whose
	// defensive copy of every (large) EnemyDefinition would allocate on a
	// per-frame draw path. We only read each def's Kind here.
	for i := range enemyDefinitions {
		if b.Seen(enemyDefinitions[i].Kind) {
			out = append(out, enemyDefinitions[i].Kind)
		}
	}
	return out
}

// SeenCount returns how many kinds have a record — the row count the
// bestiary tab's cursor wraps over. Allocation-free counterpart of
// len(SeenKinds()): the per-frame input path only needs the count, so it
// never builds the slice (nor the EnemyKinds() copy SeenKinds avoids).
// Counts the same registered+seen set SeenKinds lists, so the two can't
// drift. Nil-safe.
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

// RecordBattleKills credits a defeat for every dead member of the active
// pack into the bestiary. Called from winBattle once the encounter is won
// (all members are dead at that point); counts each instance, so felling
// three goblins in one fight advances the Goblin entry by three. Mid-battle
// summons (Raise Bones) that die count too. No-op when no pack is engaged.
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

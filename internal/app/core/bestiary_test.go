package core

import (
	"encoding/json"
	"testing"
)

// A kind isn't known until BestiaryIDKills defeats accumulate.
func TestBestiaryKnowsAtKillThreshold(t *testing.T) {
	b := make(Bestiary)
	for i := 1; i < BestiaryIDKills; i++ {
		b.RecordKill(EnemyRat)
		if b.Knows(EnemyRat) {
			t.Fatalf("Rat identified after %d kills, want only at %d", i, BestiaryIDKills)
		}
	}
	b.RecordKill(EnemyRat) // BestiaryIDKills-th
	if !b.Knows(EnemyRat) {
		t.Fatalf("Rat not identified after %d kills", BestiaryIDKills)
	}
	if got := b.Entry(EnemyRat).Kills; got != BestiaryIDKills {
		t.Fatalf("Rat kills = %d, want %d", got, BestiaryIDKills)
	}
}

// Scan shortcuts the kill threshold: one MarkScanned identifies with zero kills.
func TestBestiaryScanShortcutsIdentification(t *testing.T) {
	b := make(Bestiary)
	if b.Knows(EnemyBat) {
		t.Fatal("fresh bestiary should not know the Bat")
	}
	b.MarkScanned(EnemyBat)
	if !b.Knows(EnemyBat) {
		t.Fatal("Scan should identify the Bat immediately")
	}
	if got := b.Entry(EnemyBat).Kills; got != 0 {
		t.Fatalf("scanning shouldn't add kills, got %d", got)
	}
}

// Seen / SeenKinds list only recorded kinds, in canonical EnemyKinds order.
func TestBestiarySeenKinds(t *testing.T) {
	b := make(Bestiary)
	if len(b.SeenKinds()) != 0 {
		t.Fatal("empty bestiary should report no seen kinds")
	}
	b.RecordKill(EnemyBat)
	b.MarkScanned(EnemyRat)
	if !b.Seen(EnemyRat) || !b.Seen(EnemyBat) {
		t.Fatal("recorded kinds should report Seen")
	}
	seen := b.SeenKinds()
	if len(seen) != 2 {
		t.Fatalf("SeenKinds len = %d, want 2", len(seen))
	}
	// EnemyRat is declared before EnemyBat, so it leads regardless of insertion order.
	if seen[0] != EnemyRat || seen[1] != EnemyBat {
		t.Fatalf("SeenKinds not in EnemyKinds() order: %v", seen)
	}
}

// pruneBestiary drops empty records and entries for unregistered kinds.
func TestPruneBestiary(t *testing.T) {
	b := Bestiary{
		EnemyRat:         {Kills: 3},
		EnemyBat:         {Scanned: true},
		EnemyKind(99999): {Kills: 7},  // unregistered kind
		EnemyGoblin:      {Kills: -2}, // floored to 0 → empty → dropped
	}
	out := pruneBestiary(b)
	if _, ok := out[EnemyKind(99999)]; ok {
		t.Error("pruneBestiary kept an unregistered kind")
	}
	if _, ok := out[EnemyGoblin]; ok {
		t.Error("pruneBestiary kept an empty (negative-kill) entry")
	}
	if out[EnemyRat].Kills != 3 || !out[EnemyBat].Scanned {
		t.Errorf("pruneBestiary dropped valid entries: %+v", out)
	}
}

// The bestiary survives a SaveData JSON round-trip intact.
func TestBestiarySaveRoundTrip(t *testing.T) {
	g := NewGameState(bestiaryTestArea())
	g.Bestiary.RecordKill(EnemyRat)
	g.Bestiary.RecordKill(EnemyRat)
	g.Bestiary.MarkScanned(EnemyBat)

	blob, err := json.Marshal(NewSaveData(&g))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var data SaveData
	if err := json.Unmarshal(blob, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := data.Bestiary[EnemyRat].Kills; got != 2 {
		t.Errorf("round-trip Rat kills = %d, want 2", got)
	}
	if !data.Bestiary[EnemyBat].Scanned {
		t.Error("round-trip lost the Bat's scanned flag")
	}
}

// bestiaryTestArea is a minimal 3×3 open area for the save round-trip test (never touches disk).
func bestiaryTestArea() AreaDefinition {
	return AreaDefinition{
		Width:       3,
		Height:      3,
		Walls:       []string{"...", "...", "..."},
		Floor:       []string{"...", "...", "..."},
		Decor:       []string{"...", "...", "..."},
		Props:       []string{"...", "...", "..."},
		StartTileX:  1,
		StartTileZ:  1,
		StartFacing: East,
	}
}

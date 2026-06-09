package core

import (
	"encoding/json"
	"testing"
)

// TestSaveDataJSONRoundTrip guards the serialization shape of a save: the
// integer-keyed SkillTiers map and the fixed-size Equipped array are the
// JSON foot-guns, so the round-trip asserts they survive intact alongside
// the scalar progression fields.
func TestSaveDataJSONRoundTrip(t *testing.T) {
	party := NewParty()
	if len(party) == 0 {
		t.Fatal("NewParty returned no members")
	}
	party[0].Level = 5
	party[0].XP = 123
	party[0].SkillPoints = 2
	party[0].SkillTiers = map[SkillID]int{SkillSwipe: 2, SkillFirebolt: 1}
	party[0].Equipped[EquipRightHand] = ItemIronSword
	party[0].PoisonTurns = 2 // poison persists into exploration, so it can be saved

	orig := SaveData{
		Version:      SaveVersion,
		MapID:        "dungeon",
		PlayerTileX:  3,
		PlayerTileZ:  4,
		PlayerFacing: East,
		StepCount:    42,
		Gold:         250,
		Party:        party,
		Inventory:    []ItemStack{{Kind: ItemCheese, Count: 3}, {Kind: ItemBatJerky, Count: 1}},
		Quests:       StarterQuests(),
	}

	blob, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got SaveData
	if err := json.Unmarshal(blob, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Gold != orig.Gold {
		t.Errorf("gold: got %d want %d", got.Gold, orig.Gold)
	}
	if got.StepCount != orig.StepCount || got.PlayerTileX != 3 || got.PlayerFacing != East {
		t.Errorf("position/step fields drifted: %+v", got)
	}
	if len(got.Party) != len(orig.Party) {
		t.Fatalf("party len: got %d want %d", len(got.Party), len(orig.Party))
	}
	m := got.Party[0]
	if m.Level != 5 || m.XP != 123 || m.SkillPoints != 2 || m.PoisonTurns != 2 {
		t.Errorf("scalar party fields drifted: %+v", m)
	}
	if m.SkillTiers[SkillSwipe] != 2 || m.SkillTiers[SkillFirebolt] != 1 {
		t.Errorf("SkillTiers map did not round-trip: %v", m.SkillTiers)
	}
	if m.Equipped[EquipRightHand] != ItemIronSword {
		t.Errorf("Equipped array did not round-trip: %v", m.Equipped)
	}
	if len(got.Inventory) != len(orig.Inventory) || got.Inventory[0].Count != 3 {
		t.Errorf("inventory drifted: %v", got.Inventory)
	}
	if len(got.Quests) != len(orig.Quests) {
		t.Errorf("quests len: got %d want %d", len(got.Quests), len(orig.Quests))
	}
}

// TestSanitizeLoadedParty_ClearsTransientCombatState guards the load-side
// trust boundary: a hand-edited / older save carrying combat-only statuses
// must not load a member still asleep, stunned, or ingested into exploration.
// Poison is the deliberate exception — it persists out of battle, so it must
// survive the sanitize.
func TestSanitizeLoadedParty_ClearsTransientCombatState(t *testing.T) {
	party := NewParty()
	m := &party[0]
	m.Ingested = true
	m.IngestedBy = 2
	m.SleepTurns = 9
	m.StunTurns = 3
	m.WebbedTurns = 4
	m.ConfusedTurns = 5
	m.Defending = true
	m.PoisonTurns = 3 // must SURVIVE — poison carries into exploration

	sanitizeLoadedParty(party)

	g := &party[0]
	// IngestedBy resets to the -1 "no captor" sentinel (ReleaseAllIngested),
	// not 0 — the load-bearing flag is Ingested itself.
	if g.Ingested || g.SleepTurns != 0 || g.StunTurns != 0 ||
		g.WebbedTurns != 0 || g.ConfusedTurns != 0 || g.Defending {
		t.Errorf("transient combat state not cleared on load: %+v", g)
	}
	if g.PoisonTurns != 3 {
		t.Errorf("PoisonTurns should persist into exploration, got %d", g.PoisonTurns)
	}
}

// TestSanitizeLoadedParty_TwoHandedExclusion guards the load-side mirror of
// EquipFromInventory's two-hander rule: a hand-edited save carrying a
// two-handed weapon beside an off-hand item (or the same two-hander in both
// hands) would double-count bonuses through walkEquipped, so the opposite
// hand must come back empty.
func TestSanitizeLoadedParty_TwoHandedExclusion(t *testing.T) {
	// Two-hander + off-hand shield: the shield is evicted.
	party := NewParty()
	party[0].Equipped[EquipRightHand] = ItemWarHammer
	party[0].Equipped[EquipLeftHand] = ItemWoodenShield
	sanitizeLoadedParty(party)
	if party[0].Equipped[EquipRightHand] != ItemWarHammer || party[0].Equipped[EquipLeftHand] != ItemNone {
		t.Errorf("2H + off-hand survived load: %v", party[0].Equipped)
	}

	// Same two-hander duplicated into both hands: one copy survives.
	party = NewParty()
	party[0].Equipped[EquipRightHand] = ItemWarHammer
	party[0].Equipped[EquipLeftHand] = ItemWarHammer
	sanitizeLoadedParty(party)
	if party[0].Equipped[EquipRightHand] != ItemWarHammer || party[0].Equipped[EquipLeftHand] != ItemNone {
		t.Errorf("duplicated 2H survived load: %v", party[0].Equipped)
	}

	// Two-hander in the LEFT hand beside a right-hand weapon: the
	// two-hander wins (it's the item whose contract is violated).
	party = NewParty()
	party[0].Equipped[EquipRightHand] = ItemIronSword
	party[0].Equipped[EquipLeftHand] = ItemWarHammer
	sanitizeLoadedParty(party)
	if party[0].Equipped[EquipRightHand] != ItemNone || party[0].Equipped[EquipLeftHand] != ItemWarHammer {
		t.Errorf("left-hand 2H beside a weapon survived load: %v", party[0].Equipped)
	}

	// One-handed pairs are untouched.
	party = NewParty()
	party[0].Equipped[EquipRightHand] = ItemIronSword
	party[0].Equipped[EquipLeftHand] = ItemWoodenShield
	sanitizeLoadedParty(party)
	if party[0].Equipped[EquipRightHand] != ItemIronSword || party[0].Equipped[EquipLeftHand] != ItemWoodenShield {
		t.Errorf("legitimate sword+shield pair was disturbed: %v", party[0].Equipped)
	}
}

// TestPruneQuests_ClampsUnknownStatus guards the journal's load hygiene: a
// hand-edited Status outside {Active, Complete} would be a "neither" entry
// both header tallies skip, so it clamps to Active.
func TestPruneQuests_ClampsUnknownStatus(t *testing.T) {
	out := pruneQuests([]Quest{
		{ID: "a", Status: QuestStatus(99)},
		{ID: "b", Status: QuestComplete},
	})
	if len(out) != 2 {
		t.Fatalf("pruneQuests dropped entries: %v", out)
	}
	if out[0].Status != QuestActive {
		t.Errorf("garbage Status not clamped to Active: %v", out[0].Status)
	}
	if out[1].Status != QuestComplete {
		t.Errorf("valid Complete status disturbed: %v", out[1].Status)
	}
}

// TestOverlaySavedParty_Reconciles guards the PartyMemberCount-length,
// class-ordered seating contract against a malformed save: a normal save maps
// 1:1 (progression preserved), a short save keeps fresh defaults for missing
// slots, and an out-of-range Class is dropped rather than leaking in.
func TestOverlaySavedParty_Reconciles(t *testing.T) {
	// Normal save: 1:1, progression preserved, length unchanged.
	base := NewParty()
	saved := NewParty()
	saved[0].Level = 7
	saved[0].SkillPoints = 4
	overlaySavedParty(base, saved)
	if len(base) != PartyMemberCount {
		t.Fatalf("party length changed: %d, want %d", len(base), PartyMemberCount)
	}
	if base[0].Level != 7 || base[0].SkillPoints != 4 {
		t.Errorf("normal overlay lost progression: %+v", base[0])
	}

	// Short save (one member): canonical length preserved; the present member
	// is matched by class, missing slots keep fresh defaults.
	base = NewParty()
	overlaySavedParty(base, []PartyMember{{Class: ClassThief, Level: 9}})
	if len(base) != PartyMemberCount {
		t.Fatalf("short save broke length contract: %d", len(base))
	}
	thiefMatched := false
	for _, m := range base {
		if m.Class == ClassThief {
			thiefMatched = m.Level == 9
		}
	}
	if !thiefMatched {
		t.Errorf("thief progression not overlaid from short save: %+v", base)
	}

	// Out-of-range Class: dropped, no slot inherits its data.
	base = NewParty()
	bad := append([]PartyMember(nil), NewParty()...)
	bad[0].Class = PartyClass(99)
	bad[0].Level = 50
	overlaySavedParty(base, bad)
	for _, m := range base {
		if m.Level == 50 {
			t.Errorf("out-of-range-class member leaked into party: %+v", m)
		}
	}
}

// TestSaveVersionSupported exercises LoadSave's actual version gate (the
// extracted saveVersionSupported predicate) without touching the on-disk
// save. A future version is refused (can't parse it), and a 0/missing version
// is refused too — every real save stamps Version >= 1 via NewSaveData, so a 0
// is a corrupt or partially-written blob, not legitimate v0 content.
func TestSaveVersionSupported(t *testing.T) {
	cases := []struct {
		version int
		want    bool
	}{
		{SaveVersion, true},
		{SaveVersion + 1, false}, // too new — unreadable future format
		{0, false},               // missing/zero — corrupt, not v0 data
		{-1, false},              // garbage
	}
	for _, tc := range cases {
		if got := saveVersionSupported(tc.version); got != tc.want {
			t.Errorf("saveVersionSupported(%d) = %v, want %v", tc.version, got, tc.want)
		}
	}
}

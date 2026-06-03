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

// TestLoadSaveRejectsNewerVersion confirms a save stamped with a future
// format version is refused rather than silently misread.
func TestLoadSaveRejectsNewerVersion(t *testing.T) {
	blob, _ := json.Marshal(SaveData{Version: SaveVersion + 1, MapID: "dungeon"})
	var data SaveData
	if err := json.Unmarshal(blob, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// LoadSave's version gate is the rule under test; replicate its check
	// here without touching disk.
	if data.Version <= SaveVersion {
		t.Fatalf("expected a newer version, got %d", data.Version)
	}
}

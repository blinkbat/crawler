package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Save / load. A single JSON save file capturing the persistent subset of a
// run — party progression, inventory, gold, the quest journal, and where the
// player is standing. On load the destination map is rebuilt FRESH (packs,
// chests, doors, fog all reset from the .map), then the saved progression is
// overlaid on top. So a save behaves like a save POINT, not a world snapshot:
// re-entering a cleared area respawns its packs — the same semantics the
// area-transition path already has (it discards per-area runtime state too).

const (
	saveDirName  = "saves"
	saveFileName = "save.json"
	// SaveVersion stamps the on-disk format. Bump when the SaveData shape
	// changes incompatibly; LoadSave rejects a newer version it can't read.
	SaveVersion = 1
)

// SaveDir resolves the save-file directory (cwd- or exe-relative, like the
// maps dir). SavePath is the full path to the single save file.
func SaveDir() string  { return ResolveAssetDir(saveDirName) }
func SavePath() string { return filepath.Join(SaveDir(), saveFileName) }

// SaveData is the persistent slice of a GameState. Only exported fields
// serialize; transient combat / animation state is intentionally omitted
// (a save is taken in exploration, where those are already cleared). The
// world isn't stored — MapID names the .map and the rest rebuilds on load.
type SaveData struct {
	Version      int           `json:"version"`
	MapID        string        `json:"mapID"`
	PlayerTileX  int           `json:"playerTileX"`
	PlayerTileZ  int           `json:"playerTileZ"`
	PlayerFacing int           `json:"playerFacing"`
	StepCount    int           `json:"stepCount"`
	Gold         int           `json:"gold"`
	Party        []PartyMember `json:"party"`
	Inventory    []ItemStack   `json:"inventory"`
	Quests       []Quest       `json:"quests"`
}

// NewSaveData captures the current run into a serializable snapshot. The
// party is copied with combat-only transient statuses cleared (Sleep / Stun
// / Webbed / Confused / Defending / Ingested + the animation timers): those
// are normally wiped on battle exit, but clearing them on a defensive copy
// here means a save taken from an unexpected mid-battle path can't leak a
// "still asleep" or "still ingested" member into the reloaded run. Lasting
// state (HP / MP / Poison / level / XP / equipment) is preserved.
func NewSaveData(g *GameState) SaveData {
	return SaveData{
		Version:      SaveVersion,
		MapID:        MapIDFromPath(g.Area.Path),
		PlayerTileX:  g.Player.TileX,
		PlayerTileZ:  g.Player.TileZ,
		PlayerFacing: g.Player.Facing,
		StepCount:    g.StepCount,
		Gold:         g.Gold,
		Party:        saveSanitizedParty(g.Party),
		Inventory:    g.Inventory,
		Quests:       g.Quests,
	}
}

// saveSanitizedParty returns a copy of the party with combat-transient and
// animation fields zeroed, so a save never carries battle-only state. HP /
// MP / Poison / progression / equipment are left intact. Reuses the canonical
// clearers (ClearPartyTransientStatuses + ReleaseAllIngested) so the
// definition of "what's combat-transient" lives in one place — adding a new
// status to those helpers covers the save path for free.
func saveSanitizedParty(party []PartyMember) []PartyMember {
	out := make([]PartyMember, len(party))
	copy(out, party)
	ClearPartyTransientStatuses(out) // Sleep / Stun / Webbed / Confused / Defending
	ReleaseAllIngested(out)          // Ingested / IngestedBy
	for i := range out {
		// Animation timers aren't statuses, so they're not in the clearers
		// above; zero them so a save can't carry a mid-lunge offset.
		out[i].AttackBump = 0
		out[i].DamageFlash = 0
		out[i].HitKnockback = 0
	}
	return out
}

// SaveGame writes the current run to disk as indented JSON, creating the
// save directory if needed. Returns any filesystem / encode error so the
// caller can surface it. Refuses to write when the run has no resolvable
// map id (an unsaved editor playtest, Area.Path == "") — that would produce
// a save whose Continue can never reload its map.
func SaveGame(g *GameState) error {
	data := NewSaveData(g)
	if data.MapID == "" {
		return errors.New("can't save: this map hasn't been saved to disk")
	}
	blob, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(SaveDir(), AssetDirMode); err != nil {
		return err
	}
	return os.WriteFile(SavePath(), blob, AssetFileMode)
}

// SaveExists reports whether a readable save file is present — the gate for
// showing the title-screen Continue row.
func SaveExists() bool {
	info, err := os.Stat(SavePath())
	return err == nil && !info.IsDir()
}

// saveVersionSupported reports whether a decoded save's Version is one this
// build can read. Rejects too-new (a future format we can't parse) AND < 1:
// every real save stamps Version >= 1 via NewSaveData, so a 0 means a missing
// version field or a corrupt / partially-written blob that still parsed as
// JSON — not v0 content. Pulled out of LoadSave so the gate is unit-testable
// without touching the on-disk save file.
func saveVersionSupported(v int) bool {
	return v >= 1 && v <= SaveVersion
}

// LoadSave reads and decodes the save file. Returns an error for a missing
// file, malformed JSON, or a version this build can't read.
func LoadSave() (SaveData, error) {
	blob, err := os.ReadFile(SavePath())
	if err != nil {
		return SaveData{}, err
	}
	var data SaveData
	if err := json.Unmarshal(blob, &data); err != nil {
		return SaveData{}, err
	}
	if !saveVersionSupported(data.Version) {
		return SaveData{}, &saveVersionError{got: data.Version, max: SaveVersion}
	}
	return data, nil
}

// GameStateFromSave rebuilds a playable GameState from a save. Loads the
// saved map, builds a fresh area runtime via NewGameState, then overlays the
// saved party / inventory / gold / quests / position. The player is placed
// at the saved tile (clamped in-bounds for safety against a map that shrank
// between save and load) and the fog reveal is re-seeded around them.
func GameStateFromSave(data SaveData) (GameState, error) {
	area, err := LoadArea(MapPath(data.MapID))
	if err != nil {
		return GameState{}, err
	}
	g := NewGameState(area)
	if len(data.Party) > 0 {
		g.Party = data.Party
	}
	// Inventory / quests may legitimately be empty; copy through as-is so a
	// player who sold everything stays empty rather than re-seeding the kit.
	g.Inventory = data.Inventory
	g.Gold = data.Gold
	if len(data.Quests) > 0 {
		g.Quests = data.Quests
	}
	g.StepCount = data.StepCount
	// Place the player at the saved tile, but fall back to the map's
	// authored start if that tile is now blocked — the map may have been
	// edited (geometry changed, shrunk) between save and load, and a
	// save-by-map-id reload must never drop the player inside a wall.
	// NewGameState already seeded g.Player at the validated start, so the
	// fallback just keeps that.
	x := clampStartCoord(data.PlayerTileX, area.Width)
	z := clampStartCoord(data.PlayerTileZ, area.Height)
	if area.BlockedAt(x, z) {
		x, z = g.Player.TileX, g.Player.TileZ
		g.Player = NewPlayer(x, z, area.StartFacing)
	} else {
		g.Player = NewPlayer(x, z, data.PlayerFacing)
	}
	RevealRadius(&g, x, z, SightRadius)
	return g, nil
}

// saveVersionError reports a save file written by a newer build than this
// one can read.
type saveVersionError struct {
	got int
	max int
}

func (e *saveVersionError) Error() string {
	return fmt.Sprintf("unsupported save file version %d (this build reads 1..%d)", e.got, e.max)
}

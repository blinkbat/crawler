package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
)

// Save / load. One JSON save file with the persistent subset of a run. On load
// the map is rebuilt FRESH from the .map (packs/chests/doors/fog reset), then
// saved progression is overlaid — a save POINT, not a world snapshot, so a
// cleared area respawns its packs (matching the area-transition path).

const (
	saveDirName  = "saves"
	saveFileName = "save.json"
	// SaveVersion stamps the on-disk format. Bump on incompatible SaveData
	// changes; LoadSave rejects a newer version.
	SaveVersion = 1
	// MinSaveVersion is the oldest on-disk version still read. Every real save
	// stamps >= this via NewSaveData, so a lower value means a missing/corrupt
	// version field, not legacy content.
	MinSaveVersion = 1
)

// SaveDir resolves the save-file directory (cwd- or exe-relative). SavePath is
// the full path to the single save file.
func SaveDir() string  { return ResolveAssetDir(saveDirName) }
func SavePath() string { return filepath.Join(SaveDir(), saveFileName) }

// SaveData is the persistent slice of a GameState. Only exported fields
// serialize; transient combat/animation state is omitted. The world isn't
// stored — MapID names the .map and the rest rebuilds on load.
type SaveData struct {
	Version      int    `json:"version"`
	MapID        string `json:"mapID"`
	PlayerTileX  int    `json:"playerTileX"`
	PlayerTileZ  int    `json:"playerTileZ"`
	PlayerFacing int    `json:"playerFacing"`
	// PlayerLevel is the standing voxel level (cube-top). omitempty + the
	// standable-level load fallback keeps it save-compatible: a pre-voxel /
	// heightfield save loads onto the spawn tile's lowest standable surface.
	// Only matters where a saved tile has >1 walkable surface (under vs over a bridge).
	PlayerLevel int           `json:"playerLevel,omitempty"`
	StepCount   int           `json:"stepCount"`
	Gold        int           `json:"gold"`
	Party       []PartyMember `json:"party"`
	Inventory   []ItemStack   `json:"inventory"`
	Quests      []Quest       `json:"quests"`
	// Bestiary persists foe knowledge (kills + scanned per kind). omitempty so
	// pre-bestiary saves load with no entries.
	Bestiary Bestiary `json:"bestiary,omitempty"`
	// Crystals persists healing-crystal charge by tile. omitempty + tile overlay:
	// older saves load with fresh (charged) crystals. Without it a reload re-arms
	// a spent crystal — a free heal+save fountain.
	Crystals []CrystalSave `json:"crystals,omitempty"`
	// TriggersFired persists which Once dialog triggers fired (by trigger ID,
	// see dialogtrigger.go) so an intro cutscene doesn't replay on reload. A
	// stale key matches nothing and is inert. omitempty for older saves.
	TriggersFired map[string]bool `json:"triggersFired,omitempty"`
}

// CrystalSave is the persisted charge state of one healing crystal.
type CrystalSave struct {
	// TileX/TileZ pin the charge to a crystal so load matches by position, not
	// index — an edited map can relocate a crystal at the same count, and a
	// positional match avoids re-arming a spent charge onto the wrong tile.
	TileX   int  `json:"tileX,omitempty"`
	TileZ   int  `json:"tileZ,omitempty"`
	Charge  int  `json:"charge"`
	Charged bool `json:"charged"`
	// Saved is always true on current records. Distinguishes a real crystal at
	// (0,0) from a legacy phantom (an old format wrote crystals without
	// TileX/TileZ, decoding as (0,0)); a phantom has Saved false and is ignored.
	Saved bool `json:"saved,omitempty"`
}

// crystalSaves snapshots live crystals' charge for SaveData (nil when none).
func crystalSaves(crystals []Crystal) []CrystalSave {
	if len(crystals) == 0 {
		return nil
	}
	out := make([]CrystalSave, len(crystals))
	for i, c := range crystals {
		out[i] = CrystalSave{TileX: c.TileX, TileZ: c.TileZ, Charge: c.Charge, Charged: c.Charged, Saved: true}
	}
	return out
}

// NewSaveData captures the current run into a serializable snapshot. The party
// copy clears combat-only transients (statuses + anim timers) so a save from a
// mid-battle path can't leak a "still asleep/ingested" member; lasting state
// (HP/MP/Poison/level/XP/equipment) is preserved.
func NewSaveData(g *GameState) SaveData {
	return SaveData{
		Version:      SaveVersion,
		MapID:        MapIDFromPath(g.Area.Path),
		PlayerTileX:  g.Player.TileX,
		PlayerTileZ:  g.Player.TileZ,
		PlayerFacing: g.Player.Facing,
		PlayerLevel:  g.Player.Level,
		StepCount:    g.StepCount,
		Gold:         g.Gold,
		Party:        saveSanitizedParty(g.Party),
		// Detach from live slices/maps (defensive copy vs a future async save).
		// Value types, so a shallow clone fully detaches; nil stays nil.
		Inventory:     slices.Clone(g.Inventory),
		Quests:        slices.Clone(g.Quests),
		Bestiary:      maps.Clone(g.Bestiary),
		Crystals:      crystalSaves(g.Crystals),
		TriggersFired: maps.Clone(g.TriggersFired),
	}
}

// saveSanitizedParty copies the party with combat-transient/animation fields
// zeroed (via the shared clearer, so the definition lives in one place);
// HP/MP/Poison/progression/equipment stay intact.
func saveSanitizedParty(party []PartyMember) []PartyMember {
	out := make([]PartyMember, len(party))
	copy(out, party)
	clearPartyCombatTransients(out)
	for i := range out {
		// Shallow copy above aliases the live progression maps; clone for full
		// independence (maps.Clone preserves nil).
		out[i].SkillTiers = maps.Clone(out[i].SkillTiers)
		out[i].TreeRanks = maps.Clone(out[i].TreeRanks)
	}
	return out
}

// SaveGame writes the current run to disk as indented JSON (creating the save
// dir). Refuses to write with no resolvable map id (unsaved editor playtest,
// Area.Path == "") — Continue could never reload its map.
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
	return atomicWriteFile(SavePath(), blob)
}

// atomicWriteFile stages blob in a sibling ".tmp" then renames it over path.
// os.Rename is atomic on a single volume, so a crash mid-write leaves the PRIOR
// file intact instead of a truncated, undecodable blob. Best-effort temp cleanup
// on rename failure. The parent dir must already exist.
func atomicWriteFile(path string, blob []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, blob, AssetFileMode); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// SaveExists reports whether a readable save file is present — gates the
// title-screen Continue row.
func SaveExists() bool {
	info, err := os.Stat(SavePath())
	return err == nil && !info.IsDir()
}

// saveVersionSupported reports whether a save's Version is readable. Rejects
// too-new AND < MinSaveVersion: every real save stamps >= 1, so a 0 means a
// missing/corrupt version field, not v0 content. Extracted for unit testing.
func saveVersionSupported(v int) bool {
	return v >= MinSaveVersion && v <= SaveVersion
}

// LoadSave reads and decodes the save file. Errors on a missing file, malformed
// JSON, or an unreadable version.
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

// GameStateFromSave rebuilds a playable GameState from a save: loads the map,
// builds a fresh runtime, overlays saved state, and re-seeds the fog reveal.
func GameStateFromSave(data SaveData) (GameState, error) {
	area, err := LoadArea(MapPath(data.MapID))
	if err != nil {
		return GameState{}, err
	}
	g := NewGameState(area)
	if len(data.Party) > 0 {
		// A save is external input (file, possibly hand-edited/older). Clamp
		// numeric fields and drop unknown equipped kinds so corrupt data can't
		// feed battle/level-up/equipment math.
		sanitizeLoadedParty(data.Party)
		// Reconcile against the canonical class-ordered g.Party. A well-formed
		// save maps 1:1; a malformed one can't violate the seating contract —
		// extra/unknown members dropped, unmatched slots keep their fresh default.
		overlaySavedParty(g.Party, data.Party)
	}
	// Copy inventory/quests through as-is even when empty (a player who sold
	// everything stays empty, not re-seeded). Prune drops unregistered/older
	// kinds that'd sit as un-usable "Unknown Item" dead weight.
	g.Inventory = pruneUnknownItems(data.Inventory)
	// Same trust boundary as the party numerics: a hand-edited negative wallet
	// would load as a permanently negative economy (nothing downstream floors it).
	g.Gold = MaxZero(data.Gold)
	// Loaded journal is authoritative — assign unconditionally (even empty) so a
	// cleared journal doesn't re-seed StarterQuests.
	g.Quests = pruneQuests(data.Quests)
	// Overlay bestiary, dropping kinds this build no longer registers. An
	// empty/absent saved bestiary leaves NewGameState's fresh map in place.
	if pruned := pruneBestiary(data.Bestiary); len(pruned) > 0 {
		g.Bestiary = pruned
	}
	g.StepCount = MaxZero(data.StepCount)
	// Detached copy (nil stays nil, lazy-inited on fire).
	g.TriggersFired = maps.Clone(data.TriggersFired)
	// Overlay saved crystal charge by TILE, not index: an edited map can yield
	// the same crystal COUNT at a different tile, and an index overlay would
	// re-arm a spent charge onto a relocated crystal.
	matchUniqueOnce(len(g.Crystals), len(data.Crystals),
		func(i, j int) bool {
			cs := data.Crystals[j]
			// Skip legacy phantoms (old format, no TileX/TileZ, decode as (0,0));
			// the Saved flag distinguishes a real (0,0) crystal from a phantom.
			if !cs.Saved && cs.TileX == 0 && cs.TileZ == 0 {
				return false
			}
			return cs.TileX == g.Crystals[i].TileX && cs.TileZ == g.Crystals[i].TileZ
		},
		func(i, j int) {
			// Clamp the saved charge to [0, ceiling] (trust boundary). Honor the
			// saved Charged flag — it's the authoritative armed state; re-deriving
			// from Charge would discard it.
			cs := data.Crystals[j]
			g.Crystals[i].Charge = Clamp(cs.Charge, 0, CrystalRechargeSteps)
			g.Crystals[i].Charged = cs.Charged
		})
	// Place at the saved tile, falling back to the authored start if it's
	// out-of-bounds or now blocked (map may have shrunk). Check the RAW coords:
	// clamping to an edge first could silently drop the party at a walkable corner.
	x, z := data.PlayerTileX, data.PlayerTileZ
	level := data.PlayerLevel
	// Runtime blockers count too: a map edit can drop a chest or crystal onto the
	// saved tile, and loading inside one is a state normal movement can't reach
	// (the embedded chest can't even be opened — adjacency requires distance 1).
	// g.Crystals (not CrystalSpawns) so the default entrance crystal counts.
	blockedByCrystal := false
	for i := range g.Crystals {
		if g.Crystals[i].TileX == x && g.Crystals[i].TileZ == z {
			blockedByCrystal = true
			break
		}
	}
	if area.BlockedAt(x, z) || blockedByCrystal ||
		ChestSpawnIndexAt(area.ChestSpawns, x, z) >= 0 {
		x, z = g.Player.TileX, g.Player.TileZ
		// The saved level belonged to the blocked tile; on the fallback tile derive
		// its own surface instead, else a coincidentally-standable saved level would
		// drop the party at the wrong elevation.
		level = spawnLevel(&area, x, z)
		g.Player = NewPlayer(x, z, area.StartFacing)
	} else {
		g.Player = NewPlayer(x, z, data.PlayerFacing)
	}
	// Honor the standing level only if still standable at this tile (map may have
	// been edited), else snap to the tile's lowest standable surface. NewPlayer left
	// Level zero, so this is where the loaded level is established.
	if area.Standable(x, level, z) {
		g.Player.Level = level
	} else {
		g.Player.Level = spawnLevel(&area, x, z)
	}
	// Re-seed region presence at the LOADED tile — NewGameState seeded it at the
	// area start, but we just repositioned. Without this, the saved tile's region
	// reads as "outside", and the first step inside spuriously re-fires its
	// enter-location dialog. Mirrors the door-reposition paths in run.go.
	SeedLocationPresence(&g)
	RevealRadius(&g, x, z, SightRadius)
	return g, nil
}

// overlaySavedParty copies each saved member's run state onto the canonical
// base slot of the same Class. `base` is a fresh class-ordered NewParty(), so
// the result always has PartyMemberCount members in seating order: extras and
// unknown-class members are dropped, unmatched slots keep their fresh default.
// `saved` must already be sanitized by sanitizeLoadedParty.
func overlaySavedParty(base, saved []PartyMember) {
	matchUniqueOnce(len(base), len(saved),
		func(i, j int) bool { return saved[j].Class == base[i].Class },
		func(i, j int) { base[i] = saved[j] })
}

// matchUniqueOnce pairs each dst index with the first eligible, not-yet-consumed
// src index, consuming each src at most once: eligible(i,j) gates the pairing and
// apply(i,j) performs it. Shared by the load-time overlays (crystal charge by
// tile, party run-state by class) so the "one source claims one target" rule
// lives in one place.
func matchUniqueOnce(dstLen, srcLen int, eligible func(i, j int) bool, apply func(i, j int)) {
	used := make([]bool, srcLen)
	for i := 0; i < dstLen; i++ {
		for j := 0; j < srcLen; j++ {
			if used[j] || !eligible(i, j) {
				continue
			}
			apply(i, j)
			used[j] = true
			break
		}
	}
}

// sanitizeLoadedParty clamps a loaded party's numeric fields and clears unknown
// equipped kinds in place, so a corrupt/hand-edited save can't feed nonsense
// into battle/level-up/equipment math.
func sanitizeLoadedParty(party []PartyMember) {
	for i := range party {
		m := &party[i]
		// MaxHP is fully VIT-derived (MaxHPFor); re-derive rather than trust the
		// persisted number, which may have drifted out of sync. Floor at 1.
		m.MaxHP = MaxHPFor(m.Stats)
		if m.MaxHP < 1 {
			m.MaxHP = 1
		}
		// MaxMP is class+INT-derived (MaxMPFor); re-derive rather than trust the
		// persisted number. Unknown class has no proto to anchor it → just floor.
		if mp, ok := MaxMPFor(m.Class, m.Stats); ok {
			m.MaxMP = mp
		}
		if m.MaxMP < 0 {
			m.MaxMP = 0
		}
		m.HP = Clamp(m.HP, 0, m.MaxHP)
		m.MP = Clamp(m.MP, 0, m.MaxMP)
		if m.Level < BaseLevel {
			m.Level = BaseLevel
		}
		m.XP = MaxZero(m.XP)
		m.PendingLevelUps = MaxZero(m.PendingLevelUps)
		m.SkillPoints = MaxZero(m.SkillPoints)
		// Clear slots holding an unregistered kind: foldEquipment skips them, so
		// they'd occupy the slot as a silently dead, re-usable-blocking entry.
		for s := range m.Equipped {
			if m.Equipped[s] == ItemNone {
				continue
			}
			if _, ok := ItemInfoOk(m.Equipped[s]); !ok {
				m.Equipped[s] = ItemNone
			}
		}
		// Two-handers occupy BOTH hands. A hand-edited save can carry one beside
		// an off-hand item (or duplicated), and foldEquipment sums all slots, so
		// bonuses would double-count. Re-apply the canonical equip rule.
		NormalizeTwoHandedHands(&m.Equipped)
		// Clone the decoded progression maps so the live party doesn't alias
		// `data.Party`'s (overlaySavedParty shallow-copies; maps.Clone keeps nil).
		m.SkillTiers = maps.Clone(m.SkillTiers)
		m.TreeRanks = maps.Clone(m.TreeRanks)
		// Drop progression keyed to identifiers this build no longer knows: a
		// stale key would apply its tier/rank to the WRONG skill.
		for id := range m.SkillTiers {
			if _, ok := skillInfo(id); !ok {
				delete(m.SkillTiers, id)
			}
		}
		for id := range m.TreeRanks {
			if _, ok := findTreeNode(m.Class, id); !ok {
				delete(m.TreeRanks, id)
			}
		}
		// Bound SkillCursor against the pruned learned-skill list. PartySkill
		// self-heals at read time, so this is hygiene for future readers.
		if skills := PartySkills(m); m.SkillCursor < 0 || m.SkillCursor >= len(skills) {
			m.SkillCursor = 0
		}
	}
	// Strip combat-only state at the load trust boundary too (the write path
	// already does): else an Ingested/asleep member could load permanently
	// locked out of combat with no enemy alive to release them.
	clearPartyCombatTransients(party)
	// Repair the 2×2 formation if the loaded slots aren't a valid grid — notably
	// a pre-formation save (HomeRow/HomeCol decode to zero, all front-left),
	// which would stack every card in one spot. A valid layout is left as-is.
	NormalizePartyFormation(party)
}

// pruneValid filters a save-loaded slice: empty/nil returns nil (a cleared
// slice stays nil), else a fresh slice of the elements `keep` accepts. (pruneBestiary
// is map-shaped, so it keeps its own loop.)
func pruneValid[T any](src []T, keep func(T) bool) []T {
	if len(src) == 0 {
		return nil
	}
	out := make([]T, 0, len(src))
	for _, v := range src {
		if keep(v) {
			out = append(out, v)
		}
	}
	return out
}

// pruneUnknownItems drops inventory stacks with an unregistered kind or
// non-positive count (un-usable "Unknown Item" dead weight). Fresh slice; nil on empty.
func pruneUnknownItems(inv []ItemStack) []ItemStack {
	return pruneValid(inv, func(st ItemStack) bool {
		if st.Count <= 0 || st.Kind == ItemNone {
			return false
		}
		_, ok := ItemInfoOk(st.Kind)
		return ok
	})
}

// pruneQuests drops empty-ID entries and collapses duplicate IDs (keeping the
// first), so QuestIndexByID can't resolve inconsistently. Fresh slice; nil on
// empty. No quest REGISTRY exists yet; when one ships, also drop unregistered IDs.
func pruneQuests(quests []Quest) []Quest {
	// seen-map dedup is captured so pruneValid stays a pure filter; the Status
	// clamp mutates kept entries, so it runs as a second pass.
	seen := make(map[string]bool, len(quests))
	out := pruneValid(quests, func(q Quest) bool {
		if q.ID == "" || seen[q.ID] {
			return false
		}
		seen[q.ID] = true
		return true
	})
	for i := range out {
		// An unrecognized Status is a "neither" entry both header tallies skip;
		// clamp to Active.
		if !out[i].Status.Valid() {
			out[i].Status = QuestActive
		}
	}
	return out
}

// pruneBestiary drops entries for an unregistered EnemyKind and empty records
// (no kills, not scanned); floors negative kills at zero. Fresh map; nil on empty.
func pruneBestiary(b Bestiary) Bestiary {
	if len(b) == 0 {
		return nil
	}
	out := make(Bestiary, len(b))
	for kind, e := range b {
		if e.Kills <= 0 && !e.Scanned {
			continue
		}
		if _, ok := EnemyInfoOk(kind); !ok {
			continue
		}
		if e.Kills < 0 {
			e.Kills = 0
		}
		out[kind] = e
	}
	return out
}

// saveVersionError reports a save written by a newer build than this one reads.
type saveVersionError struct {
	got int
	max int
}

func (e *saveVersionError) Error() string {
	return fmt.Sprintf("unsupported save file version %d (this build reads %d..%d)", e.got, MinSaveVersion, e.max)
}

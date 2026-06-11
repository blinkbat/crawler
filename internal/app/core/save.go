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
	// Bestiary persists foe knowledge (kill counts + scanned flags per
	// kind). omitempty so saves written before the bestiary existed simply
	// load with no entries — adding this optional field is save-compatible,
	// so SaveVersion stays put.
	Bestiary Bestiary `json:"bestiary,omitempty"`
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
		// Detach Inventory / Quests from the live slices (same defensive-copy
		// rationale as saveSanitizedParty): today the snapshot is marshalled
		// synchronously, but an independent copy can't be torn by a future
		// deferred / async save mutating the live run between snapshot and
		// write. ItemStack / Quest are value types, so a shallow clone fully
		// detaches the backing array.
		Inventory: slices.Clone(g.Inventory),
		Quests:    slices.Clone(g.Quests),
		// Detach the bestiary map from the live run (same defensive-copy
		// rationale as Inventory / Quests). BestiaryEntry is a value type,
		// so maps.Clone fully decouples the snapshot; nil stays nil.
		Bestiary: maps.Clone(g.Bestiary),
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
		clearMemberAnimTimers(&out[i])
		// copy() above is shallow, so the snapshot's progression maps still
		// alias the live party's. Clone them so the sanitized copy is fully
		// independent — today it's only marshalled (read-only), but an
		// independent snapshot can't be corrupted by a future deferred /
		// async save that mutates the live party between snapshot and write.
		// maps.Clone preserves nil for an un-invested member.
		out[i].SkillTiers = maps.Clone(out[i].SkillTiers)
		out[i].TreeRanks = maps.Clone(out[i].TreeRanks)
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
	// Atomic write: stage the blob in a sibling temp file, then rename it over
	// the real save. os.Rename is atomic on a single volume (and replaces the
	// destination on Windows via MoveFileEx), so a crash / power loss / disk-full
	// mid-write leaves the PRIOR save fully intact instead of truncating the only
	// copy to a partial, undecodable blob that LoadSave can't recover. Best-effort
	// cleanup of the temp on a rename failure so a failed save doesn't litter.
	tmp := SavePath() + ".tmp"
	if err := os.WriteFile(tmp, blob, AssetFileMode); err != nil {
		return err
	}
	if err := os.Rename(tmp, SavePath()); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
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
		// A save is external input (file on disk, possibly hand-edited or
		// written by an older build). Clamp each member's numeric fields and
		// drop unknown equipped item kinds before overlaying, so corrupt data
		// (HP>MaxHP, MaxHP<=0, Level<BaseLevel, a renumbered ItemKind) can't
		// feed nonsense into battle / level-up / equipment math. Mirrors the
		// bounds guard custom enemies already get at load.
		sanitizeLoadedParty(data.Party)
		// Reconcile against the canonical party. NewGameState seeded g.Party
		// with a full, class-ordered NewParty(); a well-formed save (exactly
		// PartyMemberCount members, classes in order) maps 1:1, so the result
		// is the save unchanged. A save with the wrong member count or an
		// out-of-range / mismatched Class can't violate the binding
		// PartyMemberCount-length, class-ordered seating contract (which render
		// formation + SPD tie-break index into): unmatched saved members are
		// dropped, and any slot with no saved match keeps its fresh default.
		overlaySavedParty(g.Party, data.Party)
	}
	// Inventory / quests may legitimately be empty; copy through as-is so a
	// player who sold everything stays empty rather than re-seeding the kit.
	// Drop stacks whose ItemKind is no longer registered (an older save or a
	// hand-edit) — they'd otherwise sit in the bag as un-usable "Unknown
	// Item" dead weight.
	g.Inventory = pruneUnknownItems(data.Inventory)
	g.Gold = data.Gold
	// The loaded quest journal is authoritative — assign it unconditionally
	// (even when it prunes to empty) so a player who cleared/never-had quests
	// stays empty rather than silently re-seeding NewGameState's StarterQuests.
	// Matches the inventory "copy through as-is even if empty" rule just above;
	// Quests is a slice so a nil/empty result is safe to store (unlike Bestiary
	// below, whose nil-map guard must stay to avoid a write panic on the next
	// kill-record).
	g.Quests = pruneQuests(data.Quests)
	// Overlay the saved bestiary, dropping entries for kinds this build no
	// longer registers (a save predating an EnemyKind renumber) — the same
	// load-boundary hygiene Inventory / Quests / skills get. NewGameState
	// already seeded a fresh empty map, so an empty/absent saved bestiary
	// (older save) just leaves that in place.
	if pruned := pruneBestiary(data.Bestiary); len(pruned) > 0 {
		g.Bestiary = pruned
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

// overlaySavedParty copies each saved member's run state onto the canonical
// base slot of the same Class. `base` is a fresh class-ordered NewParty(), so
// the result always has exactly PartyMemberCount members in seating order no
// matter what `saved` looks like. A normal save maps 1:1 (identical result);
// a save with extra/missing members or an out-of-range Class is repaired —
// extras and unknown-class members find no slot and are dropped, and a slot
// with no saved match keeps its fresh default (1 SkillPoint, nothing learned).
// `saved` must already be sanitized (clamped) by sanitizeLoadedParty.
func overlaySavedParty(base, saved []PartyMember) {
	used := make([]bool, len(saved))
	for i := range base {
		for j := range saved {
			if used[j] || saved[j].Class != base[i].Class {
				continue
			}
			base[i] = saved[j]
			used[j] = true
			break
		}
	}
}

// sanitizeLoadedParty clamps a loaded party's mutable numeric fields into
// sane ranges and clears unknown equipped item kinds, so a corrupt or hand-
// edited save can't feed nonsense (HP>MaxHP, negative HP, MaxHP<=0,
// Level<BaseLevel, a renumbered ItemKind) into battle / level-up /
// equipment math. Mutates in place — mirrors the bounds guard the custom-
// enemy load path applies.
func sanitizeLoadedParty(party []PartyMember) {
	for i := range party {
		m := &party[i]
		if m.MaxHP < 1 {
			m.MaxHP = 1
		}
		if m.MaxMP < 0 {
			m.MaxMP = 0
		}
		m.HP = Clamp(m.HP, 0, m.MaxHP)
		m.MP = Clamp(m.MP, 0, m.MaxMP)
		if m.Level < BaseLevel {
			m.Level = BaseLevel
		}
		if m.XP < 0 {
			m.XP = 0
		}
		if m.PendingLevelUps < 0 {
			m.PendingLevelUps = 0
		}
		if m.SkillPoints < 0 {
			m.SkillPoints = 0
		}
		// Clear any slot holding an unregistered kind: walkEquipped skips
		// unknown kinds, so it would occupy the slot while contributing no
		// bonuses — a silently dead slot. Empty it so the slot is re-usable.
		for s := range m.Equipped {
			if m.Equipped[s] == ItemNone {
				continue
			}
			if _, ok := ItemInfoOk(m.Equipped[s]); !ok {
				m.Equipped[s] = ItemNone
			}
		}
		// Two-handed weapons occupy BOTH hands. EquipFromInventory enforces
		// that exclusion at equip time, but a hand-edited save can carry a
		// two-hander beside an off-hand item — or the same two-hander in both
		// hands — and walkEquipped sums all slots, so the extra item's
		// bonuses would double-count. Mirror the equip rule: a two-hander in
		// either hand empties the other (right hand wins when both qualify).
		switch {
		case ItemIsTwoHanded(m.Equipped[EquipRightHand]):
			m.Equipped[EquipLeftHand] = ItemNone
		case ItemIsTwoHanded(m.Equipped[EquipLeftHand]):
			m.Equipped[EquipRightHand] = ItemNone
		}
		// Clone the decoded progression maps so the live party doesn't alias
		// the maps held by `data.Party` (overlaySavedParty shallow-copies the
		// struct). `data` is discarded today so no live aliasing results, but
		// the write path already clones these (maps.Clone preserves nil) — mirror
		// it so a future caller that retains `data` can't share mutable state.
		m.SkillTiers = maps.Clone(m.SkillTiers)
		m.TreeRanks = maps.Clone(m.TreeRanks)
		// Drop skill/tree progression keyed to identifiers this build no
		// longer knows (a save predating a SkillID or tree-node renumber).
		// Left in, a stale key applies its invested tier/rank to the WRONG
		// skill through EffectiveSkillEffect / LearnedSkills — so prune them
		// at the load trust boundary, exactly like Equipped and the inventory.
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
		// Bound SkillCursor against the learned-skill list the pruned
		// TreeRanks now yield. PartySkill self-heals an out-of-range
		// cursor at read time, so this is hygiene, not a crash fix —
		// but it keeps the persisted value honest for any future reader
		// that indexes without the clamp.
		if skills := PartySkills(*m); m.SkillCursor < 0 || m.SkillCursor >= len(skills) {
			m.SkillCursor = 0
		}
	}
	// Combat-only state must never survive into the reloaded (exploration)
	// run. The WRITE path (saveSanitizedParty) already strips these, but the
	// load path is the trust boundary for hand-edited / older-format saves —
	// clear them here too via the same canonical clearers, so an Ingested
	// member (whose IngestedBy points at a pack slot the freshly-rebuilt area
	// no longer has) or an asleep/stunned member can't load permanently
	// locked out of combat with no enemy alive to ever release them.
	ClearPartyTransientStatuses(party)
	ReleaseAllIngested(party)
	for i := range party {
		clearMemberAnimTimers(&party[i])
	}
}

// pruneUnknownItems drops inventory stacks whose ItemKind isn't registered
// (a save predating an item, or a hand-edit) or whose count is non-positive.
// Such a stack would otherwise sit in the bag as un-usable "Unknown Item"
// dead weight. Returns a fresh slice; nil/empty input returns nil.
func pruneUnknownItems(inv []ItemStack) []ItemStack {
	if len(inv) == 0 {
		return nil
	}
	out := make([]ItemStack, 0, len(inv))
	for _, st := range inv {
		if st.Count <= 0 || st.Kind == ItemNone {
			continue
		}
		if _, ok := ItemInfoOk(st.Kind); !ok {
			continue
		}
		out = append(out, st)
	}
	return out
}

// pruneQuests drops journal entries with an empty ID and collapses duplicate
// IDs (keeping the first), so an older or hand-edited save can't seed the
// journal with blank or doubled quests that QuestIndexByID would then resolve
// inconsistently. Returns a fresh slice; nil/empty input returns nil. There is
// no quest registry to validate IDs against yet (StarterQuests is empty); when
// one ships, also drop unregistered IDs here, mirroring pruneUnknownItems.
func pruneQuests(quests []Quest) []Quest {
	if len(quests) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(quests))
	out := make([]Quest, 0, len(quests))
	for _, q := range quests {
		if q.ID == "" || seen[q.ID] {
			continue
		}
		seen[q.ID] = true
		// A hand-edited Status outside {Active, Complete} would be a
		// "neither" entry both journal-header tallies skip. Clamp it to
		// Active — the safe default for an entry we can't interpret.
		if q.Status != QuestActive && q.Status != QuestComplete {
			q.Status = QuestActive
		}
		out = append(out, q)
	}
	return out
}

// pruneBestiary drops bestiary entries for an EnemyKind this build no longer
// registers (a save predating a kind renumber/removal) and any empty record
// (no kills and not scanned). Negative kill counts from a hand-edit are
// floored at zero. Returns a fresh map; nil/empty input returns nil.
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

// saveVersionError reports a save file written by a newer build than this
// one can read.
type saveVersionError struct {
	got int
	max int
}

func (e *saveVersionError) Error() string {
	return fmt.Sprintf("unsupported save file version %d (this build reads 1..%d)", e.got, e.max)
}

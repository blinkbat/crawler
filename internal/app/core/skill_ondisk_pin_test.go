package core

import "testing"

// SkillOnDiskName derives a skill's persisted mapfile token from its display
// Name (lowercase + space->underscore). That coupling means renaming a
// skill's display Name silently changes the token written into every saved
// .map custom-enemy row, so old saves would fail to resolve the skill on
// load (SkillIDFromOnDiskName misses -> the row is rejected). These pins
// FREEZE the current token for every registered skill so a Name rename fails
// loudly here instead of corrupting saves.
//
// If you intentionally rename a skill's display Name: update the matching pin
// below AND migrate any existing saved maps that reference the old token.
var skillOnDiskPins = map[SkillID]string{
	SkillSwipe:        "swipe",
	SkillPrayer:       "prayer",
	SkillSteal:        "steal",
	SkillFirebolt:     "firebolt",
	SkillCrushingBlow: "crushing_blow",
	SkillWhirlwind:    "whirlwind",
	SkillMassMend:     "mass_mend",
	SkillSmite:        "smite",
	SkillBackstab:     "backstab",
	SkillVenomStrike:  "venom_strike",
	SkillFrostLance:   "frost_lance",
	SkillArcBolt:      "arc_bolt",
	SkillSleep:        "sleep",
	SkillIngest:       "ingest",
	SkillWeb:          "web",
	SkillConfuse:      "confuse",
	SkillStoneslam:    "stoneslam",
	SkillRaiseBones:   "raise_bones",
	SkillScan:         "scan",
	SkillBless:        "bless",
	SkillFireball:     "fireball",
	SkillPoisonCloud:  "poison_cloud",
	SkillCleanse:      "cleanse",
}

func TestSkillOnDiskNameIsPinned(t *testing.T) {
	ids := AllSkillIDs()

	// Every registered skill must have a pin (so a newly-added skill forces
	// the author to capture its frozen token here, not just the renames).
	for _, id := range ids {
		want, ok := skillOnDiskPins[id]
		if !ok {
			t.Errorf("skill id %d (%q) has no on-disk pin; add it to skillOnDiskPins with its current SkillOnDiskName", int(id), SkillName(id))
			continue
		}
		if got := SkillOnDiskName(id); got != want {
			t.Errorf("SkillOnDiskName(%q) = %q, pinned %q; renaming a skill's display Name changes its saved token and corrupts existing maps", SkillName(id), got, want)
		}
	}

	// And no stale pins for skills that no longer exist.
	if len(skillOnDiskPins) != len(ids) {
		t.Errorf("skillOnDiskPins has %d entries but there are %d registered skills; remove stale pins", len(skillOnDiskPins), len(ids))
	}
}

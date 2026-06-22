package core

import "testing"

// skillOnDiskPins freezes each skill's persisted mapfile token. SkillOnDiskName derives from the
// display Name, so a rename silently changes the saved token and breaks old saves — this fails loudly
// instead. On an intentional rename: update the pin AND migrate existing maps referencing the old token.
var skillOnDiskPins = map[SkillID]string{
	SkillSwipe:         "swipe",
	SkillPrayer:        "prayer",
	SkillSteal:         "steal",
	SkillFirebolt:      "firebolt",
	SkillCrushingBlow:  "crushing_blow",
	SkillWhirlwind:     "whirlwind",
	SkillMassMend:      "mass_mend",
	SkillSmite:         "smite",
	SkillBackstab:      "backstab",
	SkillVenomStrike:   "venom_strike",
	SkillFrostLance:    "frost_lance",
	SkillArcBolt:       "arc_bolt",
	SkillSleep:         "sleep",
	SkillIngest:        "ingest",
	SkillWeb:           "web",
	SkillConfuse:       "confuse",
	SkillStoneslam:     "stoneslam",
	SkillRaiseBones:    "raise_bones",
	SkillScan:          "scan",
	SkillBless:         "bless",
	SkillFireball:      "fireball",
	SkillPoisonCloud:   "poison_cloud",
	SkillCleanse:       "cleanse",
	SkillSecondWind:    "second_wind",
	SkillRenewal:       "renewal",
	SkillCripple:       "cripple",
	SkillFrostbite:     "frostbite",
	SkillCorrosiveVial: "corrosive_vial",
	SkillConeOfCold:    "cone_of_cold",
	SkillSunder:        "sunder",
	SkillTaunt:         "taunt",
	SkillWarBanner:     "war_banner",
	SkillStoneSkin:     "stone_skin",
	SkillBlind:         "blind",
	SkillAegis:         "aegis",
	SkillSmokeBomb:     "smoke_bomb",
	SkillIceArmor:      "ice_armor",
	SkillRend:          "rend",
	SkillLacerate:      "lacerate",
}

func TestSkillOnDiskNameIsPinned(t *testing.T) {
	ids := AllSkillIDs()

	// Every registered skill must have a pin (a new skill must capture its token here).
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

package core

import (
	"reflect"
	"testing"
)

// TestCustomEnemyDefMapRoundTrip pins the encode<->decode lockstep pair (sites 3 & 4; see
// CustomEnemyDef.Definition): build a def with every field set distinct, encode to the mapfile
// shape and decode back, and assert deep-equal. A struct+schema field missing the encode/decode
// copy drops here. The fixture name is already sanitized so the round trip is the identity.
func TestCustomEnemyDefMapRoundTrip(t *testing.T) {
	orig := CustomEnemyDef{
		Name:     "roundtrip_enemy", // pre-sanitized -> no-op
		BaseKind: EnemyGoblin,
		HP:       33, // must be > 0
		MP:       7,
		Stats:    Stats{STR: 11, DEX: 12, INT: 13, WIS: 14, VIT: 15, SPD: 16},
		Armor:    4,
		MDef:     5,
		XPValue:  88,
		Tier:     6,
		// Sentinels stay inside validateEnemyStatBounds: non-negative, SkillCastChance in [0,1].
		AttackDamage:    9,
		SkillCastChance: 0.75,
		SpellPower:      8,
		Skills:          []SkillID{SkillFirebolt, SkillSleep},
	}

	row, err := MapCustomEnemyFromDef(orig)
	if err != nil {
		t.Fatalf("MapCustomEnemyFromDef: %v", err)
	}
	got, err := CustomEnemyDefFromMap(row)
	if err != nil {
		t.Fatalf("CustomEnemyDefFromMap: %v", err)
	}

	if !reflect.DeepEqual(got, orig) {
		t.Errorf("custom enemy def did not survive the mapfile round trip — a field is likely missing from MapCustomEnemyFromDef or CustomEnemyDefFromMap\n got: %+v\nwant: %+v", got, orig)
	}

	// Confirm the fixture set every field non-zero, so DeepEqual can't pass on a shared zero.
	rv := reflect.ValueOf(orig)
	for i := 0; i < rv.NumField(); i++ {
		f := rv.Field(i)
		if f.IsZero() {
			t.Errorf("round-trip fixture leaves field %q at its zero value; set it to a distinct non-default value so the round trip actually covers it", rv.Type().Field(i).Name)
		}
	}
}

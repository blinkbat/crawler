package core

import (
	"reflect"
	"testing"
)

// TestCustomEnemyDefMapRoundTrip is a fan-out guard. A custom enemy field
// must be copied in SIX lockstep sites (see CustomEnemyDef.Definition's
// comment): the CustomEnemyDef struct, the mapfile.MapCustomEnemy struct,
// the encode copy (MapCustomEnemyFromDef), the decode copy
// (CustomEnemyDefFromMap), Definition(), and Instantiate(). This test pins
// the encode<->decode pair: build a def with EVERY field set to a distinct
// non-default value, encode it to the mapfile shape and decode it back, then
// assert the result deep-equals the original. If someone adds a struct +
// schema field but forgets the encode/decode copy, the field drops here.
//
// Name carries one transform: MapCustomEnemyFromDef runs it through
// SanitizeCustomEnemyName (whitespace -> underscore). The fixture uses an
// already-sanitized name so the round trip is the identity — that transform
// is exercised separately by the SanitizeCustomEnemyName tests.
func TestCustomEnemyDefMapRoundTrip(t *testing.T) {
	orig := CustomEnemyDef{
		Name:     "roundtrip_enemy", // no whitespace -> sanitize is a no-op
		BaseKind: EnemyGoblin,       // a non-default base kind
		HP:       33,                // must be > 0 (load rejects <= 0)
		MP:       7,
		Stats:    Stats{STR: 11, DEX: 12, INT: 13, WIS: 14, VIT: 15, SPD: 16},
		Armor:    4,
		MDef:     5,
		XPValue:  88,
		Tier:     6,
		// AttackDamage / XPValue / SpellPower must be non-negative and
		// SkillCastChance in [0,1] (validateEnemyStatBounds), so the
		// distinct sentinels stay inside those contracts.
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

	// Belt-and-suspenders: confirm the fixture really set every field to a
	// non-zero value, so a future zero-valued field doesn't pass the
	// DeepEqual above only because both sides defaulted to the same zero.
	rv := reflect.ValueOf(orig)
	for i := 0; i < rv.NumField(); i++ {
		f := rv.Field(i)
		if f.IsZero() {
			t.Errorf("round-trip fixture leaves field %q at its zero value; set it to a distinct non-default value so the round trip actually covers it", rv.Type().Field(i).Name)
		}
	}
}

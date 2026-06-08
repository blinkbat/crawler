package core

import (
	"reflect"
	"testing"
)

// SkillEffectDelta mirrors SkillEffect's shape, and EffectiveSkillEffect
// (plus SkillTierMod for the two tier-only "extension" fields) hand-sums
// each delta field onto a base SkillEffect. Nothing structurally pins the
// delta struct to those apply loops, so a NEW delta field that someone adds
// without an apply step would silently never reach combat.
//
// This test walks every field of SkillEffectDelta by reflection. For each
// numeric field it installs a temporary skill-tier upgrade carrying ONLY
// that field set to a sentinel, then asserts the field is actually carried:
//   - numeric fields except the two tier-only extensions must change some
//     SkillEffect field (observed as the tier-1 vs tier-0 difference from
//     EffectiveSkillEffect), and
//   - StealBonusDamage / CritDoubleOnExcellent must change SkillTierMod's
//     matching field.
//
// If a delta field is left unsummed by both apply paths, the corresponding
// sub-test fails — turning a silent "added a field, forgot to wire it" into
// a loud test failure.
func TestSkillEffectDeltaFieldsAreCarried(t *testing.T) {
	const probeSkill = SkillSmite // any PlayerCastable skill works

	// Install a temporary single-tier ladder for probeSkill so we control
	// the delta the apply path sees, then restore the real table.
	origRows := skillTierTable[probeSkill]
	defer func() { skillTierTable[probeSkill] = origRows }()

	tier0Member := &PartyMember{SkillTiers: map[SkillID]int{probeSkill: 0}}
	tier1Member := &PartyMember{SkillTiers: map[SkillID]int{probeSkill: 1}}

	// Fields that EffectiveSkillEffect does NOT carry — they ride
	// SkillTierMod instead. Checked separately below.
	tierModOnly := map[string]bool{
		"StealBonusDamage":      true,
		"CritDoubleOnExcellent": true,
	}

	deltaType := reflect.TypeOf(SkillEffectDelta{})
	for i := 0; i < deltaType.NumField(); i++ {
		field := deltaType.Field(i)
		t.Run(field.Name, func(t *testing.T) {
			// Build a delta with only this one field set to a sentinel.
			var delta SkillEffectDelta
			dv := reflect.ValueOf(&delta).Elem().Field(i)
			setSentinel(dv)

			// Install it as the skill's tier-1 upgrade.
			skillTierTable[probeSkill] = []SkillTierUpgrade{
				{Tier: 1, Label: "probe", Cost: 1, Effect: delta},
			}

			if tierModOnly[field.Name] {
				// These reach combat only through SkillTierMod.
				mod := SkillTierMod(tier1Member, probeSkill)
				base := SkillTierMod(tier0Member, probeSkill)
				if reflect.DeepEqual(mod, base) {
					t.Fatalf("SkillEffectDelta.%s is never carried by SkillTierMod — add an apply step", field.Name)
				}
				return
			}

			// Everything else must surface as a change in EffectiveSkillEffect
			// between tier 0 and tier 1.
			eff0 := EffectiveSkillEffect(tier0Member, probeSkill)
			eff1 := EffectiveSkillEffect(tier1Member, probeSkill)
			if reflect.DeepEqual(eff0, eff1) {
				t.Fatalf("SkillEffectDelta.%s is never summed into EffectiveSkillEffect — add an apply step in EffectiveSkillEffect", field.Name)
			}
		})
	}
}

// setSentinel writes a distinct non-zero value into a SkillEffectDelta field
// so applying it provably changes the result.
func setSentinel(v reflect.Value) {
	switch v.Kind() {
	case reflect.Int:
		v.SetInt(7)
	case reflect.Float64:
		v.SetFloat(0.5)
	case reflect.Bool:
		v.SetBool(true)
	default:
		panic("skilleffect_delta_test: unhandled SkillEffectDelta field kind " + v.Kind().String() + " — extend setSentinel")
	}
}

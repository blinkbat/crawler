package core

import (
	"reflect"
	"testing"
)

// TestSkillEffectDeltaFieldsAreCarried walks every SkillEffectDelta field by reflection, installs a
// temp tier upgrade with only that field set to a sentinel, and asserts the apply path carries it:
// most fields must change EffectiveSkillEffect (tier 1 vs 0); the tier-only extensions
// (StealBonusDamage, CritDoubleOnExcellent) must change SkillTierMod. Catches an unwired new field.
func TestSkillEffectDeltaFieldsAreCarried(t *testing.T) {
	const probeSkill = SkillSmite // any PlayerCastable skill works

	// Install a temp ladder for probeSkill so we control the delta, then restore.
	origRows := skillTierTable[probeSkill]
	defer func() { skillTierTable[probeSkill] = origRows }()

	tier0Member := &PartyMember{SkillTiers: map[SkillID]int{probeSkill: 0}}
	tier1Member := &PartyMember{SkillTiers: map[SkillID]int{probeSkill: 1}}

	// Fields that ride SkillTierMod, not EffectiveSkillEffect; checked separately below.
	tierModOnly := map[string]bool{
		"StealBonusDamage":      true,
		"CritDoubleOnExcellent": true,
	}

	deltaType := reflect.TypeOf(SkillEffectDelta{})
	for i := 0; i < deltaType.NumField(); i++ {
		field := deltaType.Field(i)
		t.Run(field.Name, func(t *testing.T) {
			// Delta with only this field set to a sentinel, installed as tier-1 upgrade.
			var delta SkillEffectDelta
			dv := reflect.ValueOf(&delta).Elem().Field(i)
			setSentinel(dv)

			skillTierTable[probeSkill] = []SkillTierUpgrade{
				{Tier: 1, Label: "probe", Cost: 1, Effect: delta},
			}

			if tierModOnly[field.Name] {
				mod := SkillTierMod(tier1Member, probeSkill)
				base := SkillTierMod(tier0Member, probeSkill)
				if reflect.DeepEqual(mod, base) {
					t.Fatalf("SkillEffectDelta.%s is never carried by SkillTierMod — add an apply step", field.Name)
				}
				return
			}

			// Everything else must surface as a tier-0 vs tier-1 change in EffectiveSkillEffect.
			eff0 := EffectiveSkillEffect(tier0Member, probeSkill)
			eff1 := EffectiveSkillEffect(tier1Member, probeSkill)
			if reflect.DeepEqual(eff0, eff1) {
				t.Fatalf("SkillEffectDelta.%s is never summed into EffectiveSkillEffect — add an apply step in EffectiveSkillEffect", field.Name)
			}
		})
	}
}

// setSentinel writes a distinct non-zero value into a SkillEffectDelta field so applying it changes the result.
func setSentinel(v reflect.Value) {
	switch v.Kind() {
	case reflect.Int:
		v.SetInt(7)
	case reflect.Float64:
		v.SetFloat(0.5)
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Struct:
		// Seed the first member; SumStats folds every member uniformly, so one is enough.
		if v.NumField() == 0 {
			panic("skilleffect_delta_test: empty struct field — extend setSentinel")
		}
		setSentinel(v.Field(0))
	default:
		panic("skilleffect_delta_test: unhandled SkillEffectDelta field kind " + v.Kind().String() + " — extend setSentinel")
	}
}

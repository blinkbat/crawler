package core

import (
	"math"
	"regexp"
	"strconv"
	"testing"
)

// TestSkillTierLabelMatchesDelta pins the skillTierTable prose against its numeric
// SkillEffectDelta: a tier Label like "+3 damage" or "+15% Stun" must agree with the
// field it advertises, so retuning a delta can't silently leave the on-screen "+N"
// copy stale. Only the unambiguous single-field label forms are checked (turn/base/
// "chance"/non-numeric labels are skipped — there's no lossless field mapping for them).
func TestSkillTierLabelMatchesDelta(t *testing.T) {
	// pct labels store the number as a fraction on the delta (15 → 0.15); flat labels
	// store it verbatim. Each check's field extracts the value the label should equal.
	checks := []struct {
		re    *regexp.Regexp
		field func(SkillEffectDelta) float64
		pct   bool
	}{
		{regexp.MustCompile(`(?i)^\+(\d+) damage`), func(d SkillEffectDelta) float64 { return float64(d.Damage) }, false},
		{regexp.MustCompile(`(?i)^\+(\d+) heal$`), func(d SkillEffectDelta) float64 { return float64(d.Heal) }, false},
		{regexp.MustCompile(`(?i)^\+(\d+) shield$`), func(d SkillEffectDelta) float64 { return float64(d.ShieldHP) }, false},
		{regexp.MustCompile(`(?i)^\+(\d+) armor break$`), func(d SkillEffectDelta) float64 { return float64(d.ArmorReduction) }, false},
		{regexp.MustCompile(`(?i)^\+(\d+)% stun$`), func(d SkillEffectDelta) float64 { return d.StunChance }, true},
		{regexp.MustCompile(`(?i)^\+(\d+)% poison$`), func(d SkillEffectDelta) float64 { return d.PoisonChance }, true},
		{regexp.MustCompile(`(?i)^\+(\d+)% burn$`), func(d SkillEffectDelta) float64 { return d.BurnChance }, true},
		{regexp.MustCompile(`(?i)^\+(\d+)% bleed$`), func(d SkillEffectDelta) float64 { return d.BleedChance }, true},
		{regexp.MustCompile(`(?i)^\+(\d+)% hp$`), func(d SkillEffectDelta) float64 { return d.PercentCurrentHP }, true},
	}
	for skill, tiers := range skillTierTable {
		for _, up := range tiers {
			for _, c := range checks {
				m := c.re.FindStringSubmatch(up.Label)
				if m == nil {
					continue
				}
				n, err := strconv.Atoi(m[1])
				if err != nil {
					t.Fatalf("skill %v tier %d: unparseable number in label %q", skill, up.Tier, up.Label)
				}
				want := float64(n)
				if c.pct {
					want /= 100
				}
				if got := c.field(up.Effect); math.Abs(got-want) > 1e-9 {
					t.Errorf("skill %v tier %d label %q advertises %v but delta field = %v",
						skill, up.Tier, up.Label, want, got)
				}
			}
		}
	}
}

package core

import (
	"fmt"
	"os"
)

// BattleTuning holds the live-adjustable combat-scene geometry exposed by the
// Debug ▸ Combat Tuning sliders. The render battle geometry reads these instead of
// hardcoded constants; DefaultBattleTuning reproduces the hand-tuned look, and the
// panel can dump the current values back as a Go literal to bake in as the new
// default (see BattleTuneGoLiteral / DumpBattleTuning).
type BattleTuning struct {
	CamPitch float32 // battle camera downward tilt, radians (negative looks down)
	CamLift  float32 // battle camera eye lift, world units up
	CamFOV   float32 // battle field-of-view, degrees (lower = zoomed in)

	FoeDistance float32 // foe formation center, forward of the camera
	FoeFloorY   float32 // foe foot line — feet rest here (foot-anchored)
	FoeFrontGapX, FoeBackGapX   float32 // per-rank horizontal slot spacing
	FoeFrontMaxW, FoeBackMaxW   float32 // per-rank total-width cap
	FoeFrontDepth, FoeBackDepth float32 // per-rank depth offset from center
	FoeZigzag                   float32 // alternate-slot fore/aft step
	FoeFrontScale, FoeBackScale float32 // per-rank sprite size multiplier (1 = authored)
	FoeBackDarken               float32 // back-rank tint darken 0..1 (depth cue; 0 = off)

	PartyFrontFwd, PartyBackFwd     float32 // per-rank distance forward of camera
	PartyFrontGapX, PartyBackGapX   float32 // per-rank horizontal column spacing
	PartyBaseY                      float32 // party sprite center height
	PartyFrontScale, PartyBackScale float32 // per-rank party sprite size multiplier (1 = base)
}

// DefaultBattleTuning is the shipped combat-scene composition. Keep in sync with
// the values dialed in via Debug ▸ Combat Tuning ▸ Dump.
func DefaultBattleTuning() BattleTuning {
	return BattleTuning{
		CamPitch: -0.3, CamLift: -0.2, CamFOV: 55,
		FoeDistance: 1.95, FoeFloorY: 0.04,
		FoeFrontGapX: 0.7, FoeBackGapX: 0.8,
		FoeFrontMaxW: 3.0, FoeBackMaxW: 4.8,
		FoeFrontDepth: 0.28, FoeBackDepth: 0.84, FoeZigzag: 0.12,
		FoeFrontScale: 0.7, FoeBackScale: 0.9, FoeBackDarken: 0.3,
		PartyFrontFwd: 1.1, PartyBackFwd: 0.85,
		PartyFrontGapX: 0.4, PartyBackGapX: 0.72,
		PartyBaseY: 0.32,
		PartyFrontScale: 0.9, PartyBackScale: 1.0,
	}
}

// BattleTuneSlider describes one adjustable row: a label, its range/step, and a
// pointer accessor into a BattleTuning (so adjust/read share one source).
type BattleTuneSlider struct {
	Label          string
	Min, Max, Step float32
	Ptr            func(*BattleTuning) *float32
}

// battleTuneSliders is the ordered slider list driving the panel's slider rows.
var battleTuneSliders = []BattleTuneSlider{
	{"Cam tilt", -0.7, 0.1, 0.01, func(t *BattleTuning) *float32 { return &t.CamPitch }},
	{"Cam height", -0.5, 2.0, 0.05, func(t *BattleTuning) *float32 { return &t.CamLift }},
	{"Cam zoom (FOV)", 30, 110, 1, func(t *BattleTuning) *float32 { return &t.CamFOV }},
	{"Foe distance", 1.0, 4.0, 0.05, func(t *BattleTuning) *float32 { return &t.FoeDistance }},
	{"Foe floor Y", -0.5, 1.0, 0.02, func(t *BattleTuning) *float32 { return &t.FoeFloorY }},
	{"Foe front gap", 0.4, 2.5, 0.05, func(t *BattleTuning) *float32 { return &t.FoeFrontGapX }},
	{"Foe back gap", 0.4, 2.5, 0.05, func(t *BattleTuning) *float32 { return &t.FoeBackGapX }},
	{"Foe front maxW", 1.5, 6.0, 0.1, func(t *BattleTuning) *float32 { return &t.FoeFrontMaxW }},
	{"Foe back maxW", 1.5, 6.0, 0.1, func(t *BattleTuning) *float32 { return &t.FoeBackMaxW }},
	{"Foe front depth", -1.0, 1.0, 0.02, func(t *BattleTuning) *float32 { return &t.FoeFrontDepth }},
	{"Foe back depth", -1.0, 1.5, 0.02, func(t *BattleTuning) *float32 { return &t.FoeBackDepth }},
	{"Foe zigzag", 0, 0.5, 0.02, func(t *BattleTuning) *float32 { return &t.FoeZigzag }},
	{"Foe front size", 0.4, 2.0, 0.05, func(t *BattleTuning) *float32 { return &t.FoeFrontScale }},
	{"Foe back size", 0.4, 2.0, 0.05, func(t *BattleTuning) *float32 { return &t.FoeBackScale }},
	{"Foe back darken", 0, 0.8, 0.05, func(t *BattleTuning) *float32 { return &t.FoeBackDarken }},
	{"Party front fwd", 0.4, 2.5, 0.05, func(t *BattleTuning) *float32 { return &t.PartyFrontFwd }},
	{"Party back fwd", 0.3, 2.5, 0.05, func(t *BattleTuning) *float32 { return &t.PartyBackFwd }},
	{"Party front gap", 0.4, 2.0, 0.04, func(t *BattleTuning) *float32 { return &t.PartyFrontGapX }},
	{"Party back gap", 0.4, 2.5, 0.04, func(t *BattleTuning) *float32 { return &t.PartyBackGapX }},
	{"Party base Y", 0.0, 1.5, 0.02, func(t *BattleTuning) *float32 { return &t.PartyBaseY }},
	{"Party front size", 0.4, 2.0, 0.05, func(t *BattleTuning) *float32 { return &t.PartyFrontScale }},
	{"Party back size", 0.4, 2.0, 0.05, func(t *BattleTuning) *float32 { return &t.PartyBackScale }},
}

// BattleTuneSliderCount is the number of slider rows (the trailing action rows
// follow). Menu indices: [0, count) sliders, then Reset / Dump / Close.
func BattleTuneSliderCount() int { return len(battleTuneSliders) }

// Trailing action-row indices + total row count for the Combat Tuning submenu.
func BattleTuneResetRow() int  { return len(battleTuneSliders) }
func BattleTuneDumpRow() int   { return len(battleTuneSliders) + 1 }
func BattleTuneCloseRow() int  { return len(battleTuneSliders) + 2 }
func BattleTuneMenuCount() int { return len(battleTuneSliders) + 3 }

// BattleTuneSliderAt returns slider i (ok=false out of range).
func BattleTuneSliderAt(i int) (BattleTuneSlider, bool) {
	if i < 0 || i >= len(battleTuneSliders) {
		return BattleTuneSlider{}, false
	}
	return battleTuneSliders[i], true
}

// BattleTuneValue is slider i's current value (0 if out of range).
func BattleTuneValue(t *BattleTuning, i int) float32 {
	if s, ok := BattleTuneSliderAt(i); ok {
		return *s.Ptr(t)
	}
	return 0
}

// BattleTuneFrac is slider i's value normalized to [0,1] for the gauge fill.
func BattleTuneFrac(t *BattleTuning, i int) float32 {
	s, ok := BattleTuneSliderAt(i)
	if !ok || s.Max == s.Min {
		return 0
	}
	return Clamp((*s.Ptr(t)-s.Min)/(s.Max-s.Min), 0, 1)
}

// AdjustBattleTuneSlider nudges slider i by dir (±1) steps, clamped to its range.
func AdjustBattleTuneSlider(t *BattleTuning, i, dir int) {
	if s, ok := BattleTuneSliderAt(i); ok {
		p := s.Ptr(t)
		*p = Clamp(*p+float32(dir)*s.Step, s.Min, s.Max)
	}
}

// BattleTuneGoLiteral renders the tuning as the body of DefaultBattleTuning so a
// dialed-in look can be pasted straight in as the new default.
func BattleTuneGoLiteral(t *BattleTuning) string {
	return fmt.Sprintf(`return BattleTuning{
	CamPitch: %g, CamLift: %g, CamFOV: %g,
	FoeDistance: %g, FoeFloorY: %g,
	FoeFrontGapX: %g, FoeBackGapX: %g,
	FoeFrontMaxW: %g, FoeBackMaxW: %g,
	FoeFrontDepth: %g, FoeBackDepth: %g, FoeZigzag: %g,
	FoeFrontScale: %g, FoeBackScale: %g, FoeBackDarken: %g,
	PartyFrontFwd: %g, PartyBackFwd: %g,
	PartyFrontGapX: %g, PartyBackGapX: %g,
	PartyBaseY: %g,
	PartyFrontScale: %g, PartyBackScale: %g,
}`,
		t.CamPitch, t.CamLift, t.CamFOV, t.FoeDistance, t.FoeFloorY,
		t.FoeFrontGapX, t.FoeBackGapX, t.FoeFrontMaxW, t.FoeBackMaxW,
		t.FoeFrontDepth, t.FoeBackDepth, t.FoeZigzag,
		t.FoeFrontScale, t.FoeBackScale, t.FoeBackDarken,
		t.PartyFrontFwd, t.PartyBackFwd, t.PartyFrontGapX, t.PartyBackGapX, t.PartyBaseY,
		t.PartyFrontScale, t.PartyBackScale)
}

// BattleTuneDumpFileName is where DumpBattleTuning writes (cwd-relative).
const BattleTuneDumpFileName = "battle_tuning.txt"

// DumpBattleTuning writes the current tuning's Go literal to BattleTuneDumpFileName,
// returning the path so the caller can surface it. For baking a dialed-in look back
// into DefaultBattleTuning.
func DumpBattleTuning(t *BattleTuning) (string, error) {
	return BattleTuneDumpFileName, os.WriteFile(BattleTuneDumpFileName, []byte(BattleTuneGoLiteral(t)+"\n"), AssetFileMode)
}

package core

import (
	"fmt"
	"os"
	"reflect"
	"strings"
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
	// Camera-relative position truck/dolly (eye AND look-target translate together, so
	// the view pans without rotating). CamShiftX slides screen-right (+) / left (−),
	// CamShiftZ dollies forward (+) / back (−) along the ground. Vertical framing is
	// CamLift above. Both fade in on the battle blend like the rest of the camera.
	CamShiftX float32
	CamShiftZ float32

	FoeDistance                 float32 // foe formation center, forward of the camera
	FoeFloorY                   float32 // foe foot line — feet rest here (foot-anchored)
	FoeFrontGapX, FoeBackGapX   float32 // per-rank horizontal slot spacing
	FoeFrontMaxW, FoeBackMaxW   float32 // per-rank total-width cap
	FoeFrontDepth, FoeBackDepth float32 // per-rank depth offset from center
	FoeZigzag                   float32 // alternate-slot fore/aft step
	FoeFrontScale, FoeBackScale float32 // per-rank sprite size multiplier (1 = authored)
	FoeBackDarken               float32 // back-rank atmospheric recede 0..1: desaturate+cool+darken wash (0 = off)

	// Fake billboard lighting (sprites bypass the world lit shader). SpriteShade darkens
	// feet vs head (vertical value ramp); SpriteWarmCool splits a warm key / cool fill
	// across the sprite; SpriteOutline is the dark rim alpha that detaches it from the
	// backdrop. All 0..1; zero = flat sprite. Applies to both foes and party.
	SpriteShade, SpriteWarmCool, SpriteOutline float32

	PartyFrontFwd, PartyBackFwd     float32 // per-rank distance forward of camera
	PartyFrontGapX, PartyBackGapX   float32 // per-rank horizontal column spacing
	PartyBaseY                      float32 // party sprite center height
	PartyFrontScale, PartyBackScale float32 // per-rank party sprite size multiplier (1 = base)
}

// DefaultBattleTuning is the shipped combat-scene composition. Keep in sync with
// the values dialed in via Debug ▸ Combat Tuning ▸ Dump.
func DefaultBattleTuning() BattleTuning {
	return BattleTuning{
		CamPitch: -0.35, CamLift: -0.2, CamFOV: 58,
		CamShiftX: 0, CamShiftZ: -0.55,
		FoeDistance: 1.95, FoeFloorY: 0.04,
		FoeFrontGapX: 0.7, FoeBackGapX: 0.75,
		FoeFrontMaxW: 3.0, FoeBackMaxW: 4.8,
		FoeFrontDepth: 0.28, FoeBackDepth: 0.98, FoeZigzag: 0.12,
		FoeFrontScale: 0.85, FoeBackScale: 0.85, FoeBackDarken: 0.35,
		SpriteShade: 0.35, SpriteWarmCool: 0.12, SpriteOutline: 0.9,
		PartyFrontFwd: 1.1, PartyBackFwd: 0.85,
		PartyFrontGapX: 0.4, PartyBackGapX: 0.72,
		PartyBaseY:      0.32,
		PartyFrontScale: 0.9, PartyBackScale: 1.0,
	}
}

// BattleTuneSlider describes one adjustable row: a label, its range/step, and a
// pointer accessor into a BattleTuning (so adjust/read share one source).
type BattleTuneSlider struct {
	Label          string
	Field          string // BattleTuning Go field name (drives BattleTuneGoLiteral; init-asserted to exist)
	Min, Max, Step float32
	Ptr            func(*BattleTuning) *float32
}

// battleTuneSliders is the ordered slider list driving the panel's slider rows.
// Single source of truth: the panel rows, the adjust accessors, and the Go-literal
// dump all read it. init() asserts it covers every (float32) BattleTuning field, so a
// new field can't ship silently un-tunable.
var battleTuneSliders = []BattleTuneSlider{
	{"Cam tilt", "CamPitch", -0.7, 0.1, 0.01, func(t *BattleTuning) *float32 { return &t.CamPitch }},
	{"Cam height", "CamLift", -0.5, 2.0, 0.05, func(t *BattleTuning) *float32 { return &t.CamLift }},
	{"Cam shift X", "CamShiftX", -2.5, 2.5, 0.05, func(t *BattleTuning) *float32 { return &t.CamShiftX }},
	{"Cam shift Z (fwd)", "CamShiftZ", -2.5, 2.5, 0.05, func(t *BattleTuning) *float32 { return &t.CamShiftZ }},
	{"Cam zoom (FOV)", "CamFOV", 30, 110, 1, func(t *BattleTuning) *float32 { return &t.CamFOV }},
	{"Foe distance", "FoeDistance", 1.0, 4.0, 0.05, func(t *BattleTuning) *float32 { return &t.FoeDistance }},
	{"Foe floor Y", "FoeFloorY", -0.5, 1.0, 0.02, func(t *BattleTuning) *float32 { return &t.FoeFloorY }},
	{"Foe front gap", "FoeFrontGapX", 0.4, 2.5, 0.05, func(t *BattleTuning) *float32 { return &t.FoeFrontGapX }},
	{"Foe back gap", "FoeBackGapX", 0.4, 2.5, 0.05, func(t *BattleTuning) *float32 { return &t.FoeBackGapX }},
	{"Foe front maxW", "FoeFrontMaxW", 1.5, 6.0, 0.1, func(t *BattleTuning) *float32 { return &t.FoeFrontMaxW }},
	{"Foe back maxW", "FoeBackMaxW", 1.5, 6.0, 0.1, func(t *BattleTuning) *float32 { return &t.FoeBackMaxW }},
	{"Foe front depth", "FoeFrontDepth", -1.0, 1.0, 0.02, func(t *BattleTuning) *float32 { return &t.FoeFrontDepth }},
	{"Foe back depth", "FoeBackDepth", -1.0, 1.5, 0.02, func(t *BattleTuning) *float32 { return &t.FoeBackDepth }},
	{"Foe zigzag", "FoeZigzag", 0, 0.5, 0.02, func(t *BattleTuning) *float32 { return &t.FoeZigzag }},
	{"Foe front size", "FoeFrontScale", 0.4, 2.0, 0.05, func(t *BattleTuning) *float32 { return &t.FoeFrontScale }},
	{"Foe back size", "FoeBackScale", 0.4, 2.0, 0.05, func(t *BattleTuning) *float32 { return &t.FoeBackScale }},
	{"Foe back darken", "FoeBackDarken", 0, 0.8, 0.05, func(t *BattleTuning) *float32 { return &t.FoeBackDarken }},
	{"Sprite shade", "SpriteShade", 0, 0.8, 0.05, func(t *BattleTuning) *float32 { return &t.SpriteShade }},
	{"Sprite warm/cool", "SpriteWarmCool", 0, 0.5, 0.02, func(t *BattleTuning) *float32 { return &t.SpriteWarmCool }},
	{"Sprite outline", "SpriteOutline", 0, 1.0, 0.05, func(t *BattleTuning) *float32 { return &t.SpriteOutline }},
	{"Party front fwd", "PartyFrontFwd", 0.4, 2.5, 0.05, func(t *BattleTuning) *float32 { return &t.PartyFrontFwd }},
	{"Party back fwd", "PartyBackFwd", 0.3, 2.5, 0.05, func(t *BattleTuning) *float32 { return &t.PartyBackFwd }},
	{"Party front gap", "PartyFrontGapX", 0.4, 2.0, 0.04, func(t *BattleTuning) *float32 { return &t.PartyFrontGapX }},
	{"Party back gap", "PartyBackGapX", 0.4, 2.5, 0.04, func(t *BattleTuning) *float32 { return &t.PartyBackGapX }},
	{"Party base Y", "PartyBaseY", 0.0, 1.5, 0.02, func(t *BattleTuning) *float32 { return &t.PartyBaseY }},
	{"Party front size", "PartyFrontScale", 0.4, 2.0, 0.05, func(t *BattleTuning) *float32 { return &t.PartyFrontScale }},
	{"Party back size", "PartyBackScale", 0.4, 2.0, 0.05, func(t *BattleTuning) *float32 { return &t.PartyBackScale }},
}

// init asserts battleTuneSliders stays in lockstep with BattleTuning: every field is
// a float32, every field is covered by exactly one slider, and every slider names a
// real field. Catches a field added without a row (silently un-tunable) at startup.
func init() {
	var t BattleTuning
	v := reflect.ValueOf(&t).Elem()
	tp := v.Type()
	covered := make(map[uintptr]bool, len(battleTuneSliders))
	for _, s := range battleTuneSliders {
		if _, ok := tp.FieldByName(s.Field); !ok {
			panic("battleTuneSliders: Field " + s.Field + " is not a BattleTuning field")
		}
		covered[reflect.ValueOf(s.Ptr(&t)).Pointer()] = true
	}
	for i := 0; i < v.NumField(); i++ {
		name := tp.Field(i).Name
		if v.Field(i).Kind() != reflect.Float32 {
			panic("BattleTuning." + name + " is not float32; tuning assumes all-float32 fields")
		}
		if !covered[v.Field(i).Addr().Pointer()] {
			panic("BattleTuning." + name + " has no battleTuneSliders row (combat-tuning drift)")
		}
	}
	if len(battleTuneSliders) != v.NumField() {
		panic("battleTuneSliders count != BattleTuning field count")
	}
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
// dialed-in look can be pasted straight in as the new default. Generated from
// battleTuneSliders (field order = slider order), so it can never drift from the
// fields the panel actually exposes.
func BattleTuneGoLiteral(t *BattleTuning) string {
	var b strings.Builder
	b.WriteString("return BattleTuning{\n")
	for _, s := range battleTuneSliders {
		fmt.Fprintf(&b, "\t%s: %g,\n", s.Field, *s.Ptr(t))
	}
	b.WriteString("}")
	return b.String()
}

// BattleTuneDumpFileName is where DumpBattleTuning writes. Deliberately cwd-relative
// (NOT routed through ResolveAssetDir): this is a dev dump meant to land in the repo
// root you run from so the literal can be copied straight into DefaultBattleTuning.
const BattleTuneDumpFileName = "battle_tuning.txt"

// DumpBattleTuning writes the current tuning's Go literal to BattleTuneDumpFileName,
// returning the path so the caller can surface it. For baking a dialed-in look back
// into DefaultBattleTuning.
func DumpBattleTuning(t *BattleTuning) (string, error) {
	return BattleTuneDumpFileName, os.WriteFile(BattleTuneDumpFileName, []byte(BattleTuneGoLiteral(t)+"\n"), AssetFileMode)
}

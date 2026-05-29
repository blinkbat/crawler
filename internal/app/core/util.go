package core

import (
	"cmp"
	"image/color"
	"math"
)

// BuildRegistry collapses the four "build O(1) lookup map from a
// definition slice" helpers (partyClassByID, skillByID, enemyByKind,
// itemByKind) into one generic builder. The registry slice stays the
// source of truth (iteration order matters for the editor's listings
// and for stable test fixtures); the map is just a read cache for
// per-frame ItemInfo / EnemyInfo / SkillInfo lookups.
//
// New registries pass the slice plus a key extractor:
//
//	var skillByID = BuildRegistry(skillDefinitions, func(d skillDefinition) SkillID { return d.Skill })
//
// Additional validation (e.g. enemies.go's [0, 1] probability gate)
// lives in a sibling init() block — keeping the builder shape clean.
func BuildRegistry[K comparable, V any](defs []V, key func(V) K) map[K]V {
	m := make(map[K]V, len(defs))
	for _, def := range defs {
		m[key(def)] = def
	}
	return m
}

func FlashTint(base color.RGBA, timer float32) color.RGBA {
	if timer <= 0 {
		return base
	}
	strength := math.Min(0.86, float64(timer/FlashDuration)*0.86)
	return MixColor(base, color.RGBA{R: 255, G: 255, B: 255, A: base.A}, strength)
}

func BumpOffset(timer, distance float32) float32 {
	if timer <= 0 {
		return 0
	}
	t := 1 - timer/BumpDuration
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return float32(math.Sin(float64(t)*math.Pi)) * distance
}

// KnockbackOffset returns the per-frame world-units offset for a
// receiver's hit-recoil. Same sine-curve shape as BumpOffset but
// scaled by HitKnockbackDuration so the recoil plays its own
// timing. Distance is the peak displacement at the curve's apex.
// Used by the renderer to push the hit sprite AWAY from its
// attacker — sign of the application is the caller's choice.
func KnockbackOffset(timer, distance float32) float32 {
	if timer <= 0 {
		return 0
	}
	t := 1 - timer/HitKnockbackDuration
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return float32(math.Sin(float64(t)*math.Pi)) * distance
}

func ApproachZero(v, amount float32) float32 {
	v -= amount
	if v < 0 {
		return 0
	}
	return v
}

func Approach(v, target, amount float32) float32 {
	if amount < 0 {
		amount = -amount
	}
	if v < target {
		v += amount
		if v > target {
			return target
		}
		return v
	}
	if v > target {
		v -= amount
		if v < target {
			return target
		}
		return v
	}
	return target
}

func TileCenter(tile int) float32 {
	return (float32(tile) + 0.5) * TileSize
}

// ClampFrameTime clips a per-frame delta time to MaxFrameStep so a long
// stall can't advance the simulation by an unbounded leap. Shared by
// explore.Update and battle.Update so the floor can't drift between them.
func ClampFrameTime(dt float32) float32 {
	if dt > MaxFrameStep {
		return MaxFrameStep
	}
	return dt
}

// facingTable carries the per-direction unit vector + camera yaw used
// by FacingVector / FacingYaw. Indexed by NormalizeFacing(facing) so
// the two helpers below stay in lockstep — earlier passes had two
// parallel switch statements on the same enum that could drift when a
// new direction was added. One row per direction; the helpers are
// one-line lookups.
var facingTable = [FacingCount]struct {
	DX, DZ int
	Yaw    float32
}{
	North: {DX: 0, DZ: -1, Yaw: -math.Pi / 2},
	East:  {DX: 1, DZ: 0, Yaw: 0},
	South: {DX: 0, DZ: 1, Yaw: math.Pi / 2},
	West:  {DX: -1, DZ: 0, Yaw: math.Pi},
}

func FacingVector(facing int) (int, int) {
	row := facingTable[NormalizeFacing(facing)]
	return row.DX, row.DZ
}

func FacingYaw(facing int) float32 {
	return facingTable[NormalizeFacing(facing)].Yaw
}

func NormalizeFacing(facing int) int {
	facing %= FacingCount
	if facing < 0 {
		facing += FacingCount
	}
	return facing
}

func WrapIndex(index, count int) int {
	if count <= 0 {
		return 0
	}
	index %= count
	if index < 0 {
		index += count
	}
	return index
}

func AbsInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// Sign returns -1, 0, or 1 depending on the sign of v. Sits next to
// AbsInt / Clamp / Min / Max (the builtin generics) so callers find
// the small spatial helpers in one place — the pack AI's chase step
// picks a direction with this, and any future spatial helper that
// needs a step vector should too.
func Sign(v int) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	default:
		return 0
	}
}

// ChebyshevDistance returns the king-move distance between two tiles —
// the max of the per-axis absolute deltas. Used by leash / chase /
// AoE radius checks where diagonal-equivalent steps should count the
// same as cardinal ones. Manhattan callers stay on `AbsInt(a-b) +
// AbsInt(c-d)` since that's a different shape (diamond vs square).
func ChebyshevDistance(ax, az, bx, bz int) int {
	dx := AbsInt(ax - bx)
	dz := AbsInt(az - bz)
	if dx > dz {
		return dx
	}
	return dz
}

func Smoothstep(t float32) float32 {
	return t * t * (3 - 2*t)
}

func Lerp(a, b, t float32) float32 {
	return a + (b-a)*t
}

// Clamp keeps v inside [min, max] for any cmp.Ordered type. Replaces
// the three typed variants (Clamp float32, ClampInt int, ClampFloat64
// float64) that used to live here with one generic. ClampByte and
// ClampMapDimension intentionally remain — they're not "keep v in a
// caller-supplied range" clamps but type-converting / fixed-bound
// clippers, which the generic can't model cleanly.
func Clamp[T cmp.Ordered](v, min, max T) T {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func ClampByte(v int) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

func MixColor(a, b color.RGBA, t float64) color.RGBA {
	t = math.Max(0, math.Min(1, t))
	return color.RGBA{
		R: uint8(float64(a.R)*(1-t) + float64(b.R)*t),
		G: uint8(float64(a.G)*(1-t) + float64(b.G)*t),
		B: uint8(float64(a.B)*(1-t) + float64(b.B)*t),
		A: uint8(float64(a.A)*(1-t) + float64(b.A)*t),
	}
}

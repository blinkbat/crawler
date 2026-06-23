package core

import (
	"cmp"
	"image/color"
	"math"
	"math/rand"
)

// BuildRegistry builds an O(1) lookup map from a definition slice + key
// extractor. The slice stays the source of truth (iteration order matters for
// editor listings + test fixtures); the map is a read cache.
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
	strength := math.Min(FlashTintStrength, float64(timer/FlashDuration)*FlashTintStrength)
	return MixColor(base, color.RGBA{R: 255, G: 255, B: 255, A: base.A}, strength)
}

// sinePulse maps a countdown timer (duration→0) onto a half-sine arc scaled by
// distance: 0 at start, peak distance at midpoint, 0 at end. Shared by
// BumpOffset (lunge) and KnockbackOffset (recoil) so they can't drift.
func sinePulse(timer, duration, distance float32) float32 {
	if timer <= 0 {
		return 0
	}
	t := Clamp(1-timer/duration, 0, 1)
	return float32(math.Sin(float64(t)*math.Pi)) * distance
}

func BumpOffset(timer, distance float32) float32 {
	return sinePulse(timer, BumpDuration, distance)
}

// KnockbackOffset returns the per-frame recoil offset (world units). Same shape
// as BumpOffset but scaled by HitKnockbackDuration. Sign is the caller's choice.
func KnockbackOffset(timer, distance float32) float32 {
	return sinePulse(timer, HitKnockbackDuration, distance)
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

// ClampFrameTime clips a per-frame dt to [0, MaxFrameStep] so a stall can't
// leap the sim forward and a negative dt can't step it backward.
func ClampFrameTime(dt float32) float32 {
	if dt > MaxFrameStep {
		return MaxFrameStep
	}
	if dt < 0 {
		return 0
	}
	return dt
}

// facingTable: per-direction unit vector + camera yaw, indexed by
// NormalizeFacing(facing) so FacingVector / FacingYaw stay in lockstep.
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

// FacingFromDelta returns the cardinal facing for a unit step (dx,dz) — inverse
// of FacingVector. ok=false for a zero or non-cardinal delta.
func FacingFromDelta(dx, dz int) (int, bool) {
	switch {
	case dx == 0 && dz == -1:
		return North, true
	case dx == 1 && dz == 0:
		return East, true
	case dx == 0 && dz == 1:
		return South, true
	case dx == -1 && dz == 0:
		return West, true
	}
	return 0, false
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

// WrapEnum shifts an enum-typed index by delta and wraps it into [0, count) via
// WrapIndex. Returns v unchanged when count <= 0.
func WrapEnum[T ~int](v T, delta, count int) T {
	if count <= 0 {
		return v
	}
	return T(WrapIndex(int(v)+delta, count))
}

// RandRangeI returns a uniform int in [lo, hi] (inclusive) from rng — integer
// twin of (*GameState).RandRangeF. Degenerate-bounds policy: hi <= lo RETURNS
// LO (so Intn can't panic). Callers wrapping this (rollGold, rollDuration) layer
// their own intentional guards on top — don't unify them.
func RandRangeI(rng *rand.Rand, lo, hi int) int {
	if hi <= lo {
		return lo
	}
	return lo + rng.Intn(hi-lo+1)
}

// assertAppendOnly panics if any listed enum constant isn't at its declaration
// index — the shared guard behind the ItemKind / EnemyKind / SkillID append-only
// pins (each serializes as its int value, so a mid-enum insert renumbers later
// entries and corrupts saves). List every constant in order; `what` names the
// enum + what it corrupts, for the panic message.
func assertAppendOnly[T ~int](what string, ordered ...T) {
	for i, v := range ordered {
		if int(v) != i {
			panic("core: " + what + " serialization value drifted — never insert mid-enum; append new entries at the end")
		}
	}
}

func AbsInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// AbsF is the float32 twin of AbsInt.
func AbsF(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

// Sign returns -1, 0, or 1 for the sign of v.
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

// ChebyshevDistance returns the king-move distance (max per-axis delta) — for
// leash / chase / AoE radius. ManhattanDistance is the L1 sibling.
func ChebyshevDistance(ax, az, bx, bz int) int {
	dx := AbsInt(ax - bx)
	dz := AbsInt(az - bz)
	if dx > dz {
		return dx
	}
	return dz
}

// ManhattanDistance returns the L1 grid-step distance (sum of per-axis deltas)
// — for adjacency / nearest-free-tile. Sibling to ChebyshevDistance.
func ManhattanDistance(ax, az, bx, bz int) int {
	return AbsInt(ax-bx) + AbsInt(az-bz)
}

func Smoothstep(t float32) float32 {
	return t * t * (3 - 2*t)
}

// EaseOutQuad maps t (clamped to [0,1]) by 1-(1-t)^2 — quick start, easing to 1.
// Single source for the expand/fill curves (glyph grow, victory XP fill).
func EaseOutQuad(t float32) float32 {
	t = Clamp(t, 0, 1)
	inv := 1 - t
	return 1 - inv*inv
}

// DistanceFade indexes a precomputed fade table by |d| (level/depth distance from
// the observer), returning min once past the table's end. Single source for the
// minimap depth fade and the editor level-distance fade.
func DistanceFade(d int, table []float32, min float32) float32 {
	if d < 0 {
		d = -d
	}
	if d >= len(table) {
		return min
	}
	return table[d]
}

func Lerp(a, b, t float32) float32 {
	return a + (b-a)*t
}

// Clamp keeps v inside [min, max] for any cmp.Ordered type. (ClampByte /
// ClampMapDimension stay separate — they're type-converting / fixed-bound.)
func Clamp[T cmp.Ordered](v, min, max T) T {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// ValidChance reports whether p is a probability in [0, 1] — the contract every
// proc / drop field is init-asserted against.
func ValidChance(p float64) bool {
	return p >= 0 && p <= 1
}

// GainUpTo adds delta to *cur, clamped so it never exceeds max. Negative delta
// just lowers cur; callers needing a 0 floor clamp that separately.
func GainUpTo(cur *int, max, delta int) {
	*cur += delta
	if *cur > max {
		*cur = max
	}
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
	t = Clamp(t, 0, 1)
	return color.RGBA{
		R: uint8(float64(a.R)*(1-t) + float64(b.R)*t),
		G: uint8(float64(a.G)*(1-t) + float64(b.G)*t),
		B: uint8(float64(a.B)*(1-t) + float64(b.B)*t),
		A: uint8(float64(a.A)*(1-t) + float64(b.A)*t),
	}
}

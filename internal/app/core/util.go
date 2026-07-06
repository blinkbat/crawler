package core

import (
	"cmp"
	"fmt"
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

// indexByID returns the index of the first element whose key EXACTLY matches id, or
// -1. The value→index scan (case-sensitive, -1 sentinel). Distinct from areas.go's
// indexByName, which case-FOLDS and returns (int, bool) for the on-disk name decoders;
// the value→row form is findByValue (findByID folded into it — a string is just one V).
func indexByID[T any](slice []T, id string, key func(T) string) int {
	for i := range slice {
		if key(slice[i]) == id {
			return i
		}
	}
	return -1
}

// enumLabel returns labels[t], or "" if t is out of range. The shared bounds-checked
// accessor behind ShopTabLabel / PanelTabLabel / JournalSubtabLabel / etc. — each an
// enum→[]string table that used to hand-roll this identical range check.
func enumLabel[T ~int](labels []string, t T) string {
	if t < 0 || int(t) >= len(labels) {
		return ""
	}
	return labels[t]
}

// assertNoEmptyLabels panics if any of labels is "", naming the table. The shared init
// guard so every enum→label table fails loudly at startup on a gap, rather than one
// table (satietyStageLabels) silently shipping a "" entry because it lacked the loop.
func assertNoEmptyLabels(name string, labels []string) {
	for i, s := range labels {
		if s == "" {
			panic(fmt.Sprintf("core: %s has an empty entry at index %d — label every value", name, i))
		}
	}
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

// WrapAngle folds a radian angle into [-π, π] — the shortest signed form, so a
// difference of two yaws eases the short way around (no 350° spins). A non-finite
// input folds to 0 (±Inf would otherwise loop forever).
func WrapAngle(a float32) float32 {
	if a != a || math.IsInf(float64(a), 0) {
		return 0
	}
	const twoPi = 2 * math.Pi
	for a > math.Pi {
		a -= twoPi
	}
	for a < -math.Pi {
		a += twoPi
	}
	return a
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

// QuarterTurn is a 90° rotation in radians — the cardinal turn step. One source for
// the per-facing yaw shared by facingTable and the explore turn animation.
const QuarterTurn = math.Pi / 2

// facingTable: per-direction unit vector + camera yaw, indexed by
// NormalizeFacing(facing) so FacingVector / FacingYaw stay in lockstep.
var facingTable = [FacingCount]struct {
	DX, DZ int
	Yaw    float32
}{
	North: {DX: 0, DZ: -1, Yaw: -QuarterTurn},
	East:  {DX: 1, DZ: 0, Yaw: 0},
	South: {DX: 0, DZ: 1, Yaw: QuarterTurn},
	West:  {DX: -1, DZ: 0, Yaw: math.Pi},
}

func FacingVector(facing int) (int, int) {
	row := facingTable[NormalizeFacing(facing)]
	return row.DX, row.DZ
}

// FacingSteps returns the four [dx,dz] cardinal steps as a fresh value array
// (callers may shuffle it in place), derived from facingTable so AI and
// player-step code share one direction source.
func FacingSteps() [FacingCount][2]int {
	var out [FacingCount][2]int
	for i := 0; i < FacingCount; i++ {
		out[i] = [2]int{facingTable[i].DX, facingTable[i].DZ}
	}
	return out
}

func FacingYaw(facing int) float32 {
	return facingTable[NormalizeFacing(facing)].Yaw
}

// FacingFromDelta returns the cardinal facing for a unit step (dx,dz) — inverse of
// FacingVector, derived by scanning facingTable so the two can't desync (a retuned
// direction vector updates both). ok=false for a zero or non-cardinal delta.
func FacingFromDelta(dx, dz int) (int, bool) {
	for f := 0; f < FacingCount; f++ {
		if facingTable[f].DX == dx && facingTable[f].DZ == dz {
			return f, true
		}
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

// rollGold / rollDuration sit beside RandRangeI on purpose: all three draw a
// uniform int range but with DELIBERATELY DIFFERENT degenerate-bounds policies
// (RandRangeI returns lo; rollGold swaps inverted bounds then clamps >=0; rollDuration
// fails open to 0). Co-located so the "don't unify these" contract is visible at once.

// rollGold returns a uniform int in [lo, hi], tolerant of (0,0) and inverted
// bounds so an authoring slip can't panic the loot award: SWAP inverted bounds,
// then clamp to >= 0. (Differs from RandRangeI / rollDuration on degenerate input.)
func rollGold(rng *rand.Rand, lo, hi int) int {
	if hi < lo {
		lo, hi = hi, lo
	}
	if hi <= 0 {
		return 0
	}
	if lo < 0 {
		lo = 0
	}
	return RandRangeI(rng, lo, hi)
}

// rollDuration is the shared uniform [min, max] draw behind every SkillEffect
// status-duration helper. Degenerate bounds (min <= 0 || max < min) return 0
// (fail open to "no status") — note this differs from RandRangeI (returns lo)
// and rollGold (swaps then clamps).
func rollDuration(rng *rand.Rand, min, max int) int {
	if min <= 0 || max < min {
		return 0
	}
	return RandRangeI(rng, min, max)
}

// assertAppendOnly panics if any listed enum constant isn't at its declaration
// index — the shared guard behind the ItemKind / EnemyKind / SkillID append-only
// pins (each serializes as its int value, so a mid-enum insert renumbers later
// entries and corrupts saves). `count` is the enum's cardinality (its trailing
// xxxCount sentinel); the len check catches a NEW value appended to the enum but
// forgotten here — without it the pin list silently passes as a valid prefix.
// List every constant in order; `what` names the enum + what it corrupts.
func assertAppendOnly[T ~int](what string, count int, ordered ...T) {
	if len(ordered) != count {
		panic("core: " + what + " append-only pin list is incomplete — list every enum value in order (a new value was appended without a pin line)")
	}
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

// EaseOutCubic maps t (clamped to [0,1]) by 1-(1-t)^3 — quick start, a softer and
// longer settle than EaseOutQuad. Used by the crystal spin-burst.
func EaseOutCubic(t float32) float32 {
	t = Clamp(t, 0, 1)
	inv := 1 - t
	return 1 - inv*inv*inv
}

// EaseOutBack overshoots past 1 near the end then settles back — a springy "pop"
// (battle splash intro). Endpoints pin to exactly 0/1; the mid-range overshoot is
// deliberate, so the OUTPUT is intentionally NOT clamped (unlike the -Quad/-Cubic).
func EaseOutBack(t float32) float32 {
	if t <= 0 {
		return 0
	}
	if t >= 1 {
		return 1
	}
	const c1 = 1.70158
	const c3 = c1 + 1
	x := float64(t) - 1
	return float32(1 + c3*math.Pow(x, 3) + c1*math.Pow(x, 2))
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

// SubFloorZero subtracts delta from *cur, flooring at 0 — the subtractive mirror
// of GainUpTo (drain readiness/armor/shield, eat into hunger). Negative delta is
// a no-op floor wouldn't change.
func SubFloorZero(cur *int, delta int) {
	if *cur -= delta; *cur < 0 {
		*cur = 0
	}
}

// ClampIndex keeps an index in [0, n-1], returning 0 for an empty list (n<=0) —
// the shared "clamp a cursor into a possibly-empty slice" guard.
func ClampIndex(i, n int) int {
	if n <= 0 {
		return 0
	}
	return Clamp(i, 0, n-1)
}

// MaxZero returns v clamped up to 0 — the value-returning floor-at-zero used
// wherever a debuff/decrement/load must not drive an int negative (the mirror of
// the pointer-mutating SubFloorZero).
func MaxZero(v int) int {
	if v < 0 {
		return 0
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

// RoundToInt rounds a non-negative float to the nearest int (round-half-up).
// One home for the scattered `int(f + 0.5)` slider/threshold snaps; not valid
// for negatives (callers here only feed it clamped, non-negative values).
func RoundToInt[T ~float32 | ~float64](f T) int {
	return int(f + 0.5)
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

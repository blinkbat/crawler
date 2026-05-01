package core

import (
	"image/color"
	"math"
)

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

func ApproachZero(v, amount float32) float32 {
	v -= amount
	if v < 0 {
		return 0
	}
	return v
}

func TileCenter(tile int) float32 {
	return (float32(tile) + 0.5) * TileSize
}

func FacingVector(facing int) (int, int) {
	switch NormalizeFacing(facing) {
	case North:
		return 0, -1
	case East:
		return 1, 0
	case South:
		return 0, 1
	case West:
		return -1, 0
	}
	return 0, 0
}

func FacingYaw(facing int) float32 {
	switch NormalizeFacing(facing) {
	case North:
		return -math.Pi / 2
	case East:
		return 0
	case South:
		return math.Pi / 2
	case West:
		return math.Pi
	}
	return 0
}

func FacingName(facing int) string {
	switch NormalizeFacing(facing) {
	case North:
		return "N"
	case East:
		return "E"
	case South:
		return "S"
	case West:
		return "W"
	}
	return "?"
}

func NormalizeFacing(facing int) int {
	facing %= 4
	if facing < 0 {
		facing += 4
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

func MinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func MaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func Smoothstep(t float32) float32 {
	return t * t * (3 - 2*t)
}

func Lerp(a, b, t float32) float32 {
	return a + (b-a)*t
}

func Clamp(v, min, max float32) float32 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func ClampFloat64(v, min, max float64) float64 {
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

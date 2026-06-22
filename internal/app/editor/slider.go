package editor

import (
	"crawler/internal/app/core"
	"crawler/internal/app/render"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// sliderField describes one row in a slider-stack modal: a bounded scalar on
// value T, bridged through Get/Set. Shared by the sound creator and Foe Visualizer.
type sliderField[T any] struct {
	Label   string
	Min     float64
	Max     float64
	Step    float64
	Get     func(*T) float64
	Set     func(*T, float64)
	Format  string               // fmt verb / suffix — used when Display is nil
	Display func(float64) string // optional custom value renderer; nil uses Format
}

func sliderSnap(min, max, step float64, trackX, trackW, mouseX float32) float64 {
	if trackW <= 0 {
		return core.Clamp(min, min, max)
	}
	t := float64((mouseX - trackX) / trackW)
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	raw := min + t*(max-min)
	if step > 0 {
		steps := (raw - min) / step
		raw = min + float64(int(steps+0.5))*step
	}
	return core.Clamp(raw, min, max)
}

func drawSlider(font rl.Font, theme render.Theme, label, valueText string, value, min, max float64, labelPos, valuePos rl.Vector2, fontSize float32, track rl.Rectangle, thumbRadius float32, focused bool) {
	textCol := theme.TextMuted
	if focused {
		textCol = theme.BorderActive
	}
	render.DrawRichText(font, label, labelPos, fontSize, 1, textCol)

	rl.DrawRectangleRec(track, theme.SurfaceLog)
	t := float32(0)
	if max > min {
		t = float32((value - min) / (max - min))
	}
	fillCol := theme.TextLabel
	if focused {
		fillCol = theme.BorderActive
	}
	fillW := track.Width * t
	rl.DrawRectangleRec(rl.NewRectangle(track.X, track.Y, fillW, track.Height), fillCol)
	rl.DrawRectangleLinesEx(track, 1, theme.BorderDim)

	thumbCol := theme.BorderStrong
	if focused {
		thumbCol = theme.BorderActive
	}
	rl.DrawCircle(int32(track.X+fillW), int32(track.Y+track.Height/2), thumbRadius, thumbCol)
	render.DrawRichText(font, valueText, valuePos, fontSize, 1, textCol)
}

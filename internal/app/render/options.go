package render

import rl "github.com/gen2brain/raylib-go/raylib"

type optionStyle struct {
	highlightOffsetX int32
	highlightOffsetY int32
	highlightWidth   int32
	highlightHeight  int32
	roundness        float32
	textOffsetX      int32
	textSize         float32
	triangleTipX     int32
	triangleBaseX    int32
	triangleHalfH    int32
}

var (
	menuOptionStyle = optionStyle{
		highlightOffsetX: -18,
		highlightOffsetY: -5,
		highlightWidth:   248,
		highlightHeight:  34,
		roundness:        0.25,
		textOffsetX:      10,
		textSize:         24,
		triangleTipX:     -7,
		triangleBaseX:    -15,
		triangleHalfH:    8,
	}
	battleOptionStyle = optionStyle{
		highlightOffsetX: -14,
		highlightOffsetY: -3,
		highlightWidth:   210,
		highlightHeight:  25,
		roundness:        0.28,
		textOffsetX:      4,
		textSize:         18,
		triangleTipX:     -4,
		triangleBaseX:    -10,
		triangleHalfH:    6,
	}
)

func drawOption(font rl.Font, text string, x, y int32, selected bool, style optionStyle) {
	if selected {
		drawRoundedRect(
			x+style.highlightOffsetX,
			y+style.highlightOffsetY,
			style.highlightWidth,
			style.highlightHeight,
			style.roundness,
			rl.NewColor(72, 76, 110, 145),
		)
		centerY := y + style.highlightOffsetY + style.highlightHeight/2
		rl.DrawTriangle(
			rl.NewVector2(float32(x+style.triangleTipX), float32(centerY)),
			rl.NewVector2(float32(x+style.triangleBaseX), float32(centerY-style.triangleHalfH)),
			rl.NewVector2(float32(x+style.triangleBaseX), float32(centerY+style.triangleHalfH)),
			rl.NewColor(118, 235, 136, 255),
		)
	}
	drawHUDText(font, text, x+style.textOffsetX, y, style.textSize)
}

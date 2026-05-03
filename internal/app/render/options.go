package render

import rl "github.com/gen2brain/raylib-go/raylib"

type optionStyle struct {
	highlightOffsetX int32
	highlightOffsetY int32
	highlightWidth   int32
	highlightHeight  int32
	textOffsetX      int32
	textSize         float32
	triangleTipX     int32
	triangleBaseX    int32
	triangleHalfH    int32
}

var menuOptionStyle = optionStyle{
	highlightOffsetX: -18,
	highlightOffsetY: -6,
	highlightWidth:   316,
	highlightHeight:  40,
	textOffsetX:      12,
	textSize:         26,
	triangleTipX:     -7,
	triangleBaseX:    -16,
	triangleHalfH:    9,
}

func drawOption(font rl.Font, text string, x, y int32, selected bool, style optionStyle) {
	if selected {
		drawSmallPanel(
			x+style.highlightOffsetX,
			y+style.highlightOffsetY,
			style.highlightWidth,
			style.highlightHeight,
			surfaceActiveTint,
		)
		drawSmallPanelOutline(
			x+style.highlightOffsetX,
			y+style.highlightOffsetY,
			style.highlightWidth,
			style.highlightHeight,
			borderActive,
		)
		centerY := y + style.highlightOffsetY + style.highlightHeight/2
		rl.DrawTriangle(
			rl.NewVector2(float32(x+style.triangleTipX), float32(centerY)),
			rl.NewVector2(float32(x+style.triangleBaseX), float32(centerY-style.triangleHalfH)),
			rl.NewVector2(float32(x+style.triangleBaseX), float32(centerY+style.triangleHalfH)),
			borderActive,
		)
	}
	drawHUDText(font, text, x+style.textOffsetX, y, style.textSize)
}

package render

import (
	"crawler/internal/app/core"
	"fmt"
	rl "github.com/gen2brain/raylib-go/raylib"
	"image/color"
)

func drawPartyStatLabel(font rl.Font, member core.PartyMember, centerX, y float32, active, selected, down bool) {
	nameSize := float32(20)
	nameMeasure := rl.MeasureTextEx(font, member.Name, nameSize, 1)
	labelW := float32(122)
	if nameMeasure.X+18 > labelW {
		labelW = nameMeasure.X + 18
	}
	labelH := float32(76)
	x := centerX - labelW/2
	screenW := float32(rl.GetScreenWidth())
	screenH := float32(rl.GetScreenHeight())
	if x < 6 {
		x = 6
	}
	if x+labelW > screenW-6 {
		x = screenW - labelW - 6
	}
	if y+labelH > screenH-8 {
		y = screenH - labelH - 8
	}
	if y < 6 {
		y = 6
	}
	centerX = x + labelW/2

	bg := rl.NewColor(6, 9, 15, 165)
	border := rl.NewColor(77, 208, 232, 185)
	nameCol := rl.RayWhite
	if active {
		bg = rl.NewColor(48, 48, 83, 175)
		border = rl.NewColor(255, 224, 126, 230)
	}
	if selected {
		bg = rl.NewColor(28, 60, 72, 180)
		border = rl.NewColor(118, 235, 136, 235)
	}
	if down {
		bg = rl.NewColor(28, 20, 24, 155)
		border = rl.NewColor(144, 90, 96, 170)
		nameCol = rl.NewColor(170, 162, 166, 235)
	}

	ix := int32(x)
	iy := int32(y)
	iw := int32(labelW)
	ih := int32(labelH)
	drawRoundedRect(ix, iy, iw, ih, 0.1, bg)
	drawRoundedRectLines(ix, iy, iw, ih, 0.1, border)
	drawTextCentered(font, member.Name, centerX, y+6, nameSize, nameCol)
	drawResourceBar(font, x+10, y+34, labelW-20, 14, "HP", member.HP, member.MaxHP, rl.NewColor(205, 65, 72, 255), down)
	drawResourceBar(font, x+10, y+54, labelW-20, 14, "MP", member.MP, member.MaxMP, rl.NewColor(70, 134, 218, 255), down)
}

func drawTextCentered(font rl.Font, text string, centerX, y, size float32, col color.RGBA) {
	measure := rl.MeasureTextEx(font, text, size, 1)
	pos := rl.NewVector2(centerX-measure.X/2, y)
	shadow := rl.NewVector2(pos.X+1, pos.Y+1)
	rl.DrawTextEx(font, text, shadow, size, 1, rl.NewColor(0, 0, 0, 190))
	rl.DrawTextEx(font, text, pos, size, 1, col)
}

func drawResourceBar(font rl.Font, x, y, width, height float32, label string, value, maxValue int, fill color.RGBA, muted bool) {
	if maxValue <= 0 {
		maxValue = 1
	}
	percent := core.ClampFloat64(float64(value)/float64(maxValue), 0, 1)
	bg := rl.NewColor(8, 12, 18, 175)
	border := rl.NewColor(215, 220, 230, 120)
	textCol := rl.RayWhite
	if muted {
		fill = rl.NewColor(92, 82, 88, 230)
		border = rl.NewColor(128, 106, 112, 120)
		textCol = rl.NewColor(190, 176, 181, 235)
	}
	ix := int32(x)
	iy := int32(y)
	iw := int32(width)
	ih := int32(height)
	drawRoundedRect(ix, iy, iw, ih, 0.35, bg)
	drawRoundedRect(ix+1, iy+1, int32((width-2)*float32(percent)), ih-2, 0.35, fill)
	drawRoundedRectLines(ix, iy, iw, ih, 0.35, border)
	drawTextCentered(font, fmt.Sprintf("%s %d/%d", label, value, maxValue), x+width/2, y, 12, textCol)
}

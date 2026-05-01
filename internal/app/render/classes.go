package render

import (
	"image/color"
	"math"

	"crawler/internal/app/core"
)

type partyClassPresentation struct {
	turnColor   color.RGBA
	textureSeed int
	drawPixels  func([]color.RGBA, int, int)
	dance       func(float32) (float32, float32, float32, float32)
}

var partyClassPresentations = map[core.PartyClass]partyClassPresentation{
	core.ClassWarrior: {
		turnColor:   color.RGBA{R: 235, G: 88, B: 78, A: 255},
		textureSeed: 1,
		drawPixels:  drawWarriorPartyPixels,
		dance:       warriorVictoryDance,
	},
	core.ClassCleric: {
		turnColor:   color.RGBA{R: 244, G: 222, B: 138, A: 255},
		textureSeed: 2,
		drawPixels:  drawClericPartyPixels,
		dance:       clericVictoryDance,
	},
	core.ClassThief: {
		turnColor:   color.RGBA{R: 94, G: 214, B: 148, A: 255},
		textureSeed: 3,
		drawPixels:  drawThiefPartyPixels,
		dance:       thiefVictoryDance,
	},
	core.ClassWizard: {
		turnColor:   color.RGBA{R: 120, G: 152, B: 255, A: 255},
		textureSeed: 4,
		drawPixels:  drawWizardPartyPixels,
		dance:       wizardVictoryDance,
	},
}

func partyClassPresentationFor(class core.PartyClass) partyClassPresentation {
	if presentation, ok := partyClassPresentations[class]; ok {
		return presentation
	}
	return partyClassPresentation{
		turnColor:   color.RGBA{R: 245, G: 245, B: 245, A: 255},
		textureSeed: 0,
		drawPixels:  drawDefaultPartyPixels,
		dance:       defaultVictoryDance,
	}
}

func drawWarriorPartyPixels(pixels []color.RGBA, w, h int) {
	armor := color.RGBA{R: 97, G: 113, B: 128, A: 255}
	armorDark := color.RGBA{R: 55, G: 64, B: 78, A: 255}
	red := color.RGBA{R: 157, G: 55, B: 63, A: 255}
	hair := color.RGBA{R: 98, G: 58, B: 34, A: 255}
	fillEllipsePixels(pixels, w, h, 20, 41, 8, 9, armorDark)
	fillEllipsePixels(pixels, w, h, 44, 41, 8, 9, armorDark)
	fillRectPixels(pixels, w, h, 18, 37, 29, 26, red)
	fillEllipsePixels(pixels, w, h, 32, 39, 18, 12, armor)
	fillRectPixels(pixels, w, h, 23, 46, 18, 17, armorDark)
	drawLinePixels(pixels, w, h, 24, 47, 41, 47, adjust(armor, 42), 2)
	fillEllipsePixels(pixels, w, h, 32, 24, 15, 14, armor)
	fillEllipsePixels(pixels, w, h, 32, 29, 12, 7, hair)
	drawLinePixels(pixels, w, h, 19, 25, 45, 25, adjust(armorDark, 10), 2)
}

func drawClericPartyPixels(pixels []color.RGBA, w, h int) {
	robe := color.RGBA{R: 218, G: 219, B: 202, A: 255}
	robeDark := color.RGBA{R: 151, G: 151, B: 139, A: 255}
	gold := color.RGBA{R: 222, G: 184, B: 86, A: 255}
	hood := color.RGBA{R: 238, G: 234, B: 214, A: 255}
	fillEllipsePixels(pixels, w, h, 32, 48, 19, 23, robeDark)
	fillRectPixels(pixels, w, h, 17, 35, 30, 31, robe)
	fillEllipsePixels(pixels, w, h, 32, 65, 15, 7, robe)
	drawLinePixels(pixels, w, h, 32, 38, 32, 63, gold, 2)
	drawLinePixels(pixels, w, h, 25, 48, 39, 48, gold, 2)
	fillEllipsePixels(pixels, w, h, 32, 24, 15, 15, robeDark)
	fillEllipsePixels(pixels, w, h, 32, 23, 13, 14, hood)
	fillEllipsePixels(pixels, w, h, 32, 31, 8, 6, adjust(hood, -22))
}

func drawThiefPartyPixels(pixels []color.RGBA, w, h int) {
	cloak := color.RGBA{R: 40, G: 109, B: 89, A: 255}
	cloakDark := color.RGBA{R: 25, G: 56, B: 57, A: 255}
	trim := color.RGBA{R: 92, G: 171, B: 128, A: 255}
	hood := color.RGBA{R: 31, G: 45, B: 52, A: 255}
	fillEllipsePixels(pixels, w, h, 32, 50, 18, 23, cloakDark)
	fillRectPixels(pixels, w, h, 18, 36, 28, 29, cloak)
	fillTrianglePixels(pixels, w, h, 18, 35, 46, 35, 32, 68, cloak)
	drawLinePixels(pixels, w, h, 25, 38, 20, 62, trim, 2)
	drawLinePixels(pixels, w, h, 39, 38, 44, 62, trim, 2)
	fillEllipsePixels(pixels, w, h, 32, 24, 15, 15, hood)
	fillTrianglePixels(pixels, w, h, 21, 22, 43, 22, 32, 39, hood)
	fillEllipsePixels(pixels, w, h, 32, 30, 9, 5, adjust(hood, -18))
}

func drawWizardPartyPixels(pixels []color.RGBA, w, h int) {
	robe := color.RGBA{R: 64, G: 78, B: 155, A: 255}
	robeDark := color.RGBA{R: 34, G: 43, B: 90, A: 255}
	hat := color.RGBA{R: 86, G: 74, B: 172, A: 255}
	trim := color.RGBA{R: 226, G: 196, B: 93, A: 255}
	fillEllipsePixels(pixels, w, h, 32, 49, 18, 24, robeDark)
	fillTrianglePixels(pixels, w, h, 16, 66, 48, 66, 32, 34, robe)
	fillRectPixels(pixels, w, h, 20, 37, 24, 27, robe)
	drawLinePixels(pixels, w, h, 22, 42, 42, 42, trim, 2)
	drawLinePixels(pixels, w, h, 32, 42, 32, 63, trim, 2)
	fillTrianglePixels(pixels, w, h, 22, 24, 42, 24, 34, 3, hat)
	fillEllipsePixels(pixels, w, h, 32, 26, 17, 5, adjust(hat, -10))
	drawLinePixels(pixels, w, h, 25, 24, 42, 24, trim, 1)
}

func drawDefaultPartyPixels(pixels []color.RGBA, w, h int) {
	fillRectPixels(pixels, w, h, 18, 36, 28, 30, color.RGBA{R: 110, G: 110, B: 120, A: 255})
	fillEllipsePixels(pixels, w, h, 32, 24, 14, 14, color.RGBA{R: 90, G: 70, B: 55, A: 255})
}

func warriorVictoryDance(elapsed float32) (float32, float32, float32, float32) {
	height := danceBounce(elapsed, 1.55, 0) * 0.075
	return danceWave(elapsed, 0.78, 0) * 0.02, danceWave(elapsed, 1.55, math.Pi/2) * 0.016, height, 1 + height*0.045
}

func clericVictoryDance(elapsed float32) (float32, float32, float32, float32) {
	bob := danceWave(elapsed, 1.05, 0)
	return danceWave(elapsed, 0.82, math.Pi/5) * 0.045, 0, (bob + 1) * 0.026, 1 + bob*0.012
}

func thiefVictoryDance(elapsed float32) (float32, float32, float32, float32) {
	height := danceBounce(elapsed, 2.15, 0) * 0.045
	return danceWave(elapsed, 1.95, 0) * 0.065, danceWave(elapsed, 1.35, math.Pi/2) * 0.024, height, 1 + height*0.12
}

func wizardVictoryDance(elapsed float32) (float32, float32, float32, float32) {
	floatBob := danceWave(elapsed, 0.72, math.Pi/3)
	return danceWave(elapsed, 0.58, math.Pi/2) * 0.035, danceWave(elapsed, 0.7, 0) * 0.026, 0.055 + floatBob*0.026, 1 + floatBob*0.014
}

func defaultVictoryDance(elapsed float32) (float32, float32, float32, float32) {
	height := danceBounce(elapsed, 1.2, 0) * 0.045
	return danceWave(elapsed, 1, 0) * 0.03, 0, height, 1
}

func danceWave(elapsed float32, freq, phase float64) float32 {
	return float32(math.Sin(float64(elapsed)*math.Pi*2*freq + phase))
}

func danceBounce(elapsed float32, freq, phase float64) float32 {
	return (danceWave(elapsed, freq, phase) + 1) * 0.5
}

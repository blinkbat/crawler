package render

import (
	"image/color"
	"math"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
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

// init asserts every core.PartyClass has a partyClassPresentations
// entry — mirrors the panic-at-init pattern AGENTS.md documents for
// the skill handler / tile label / prop coverage tables. Adding a
// PartyClass without registering its presentation now panics at
// startup instead of silently rendering a default sprite with the
// wrong turn color.
func init() {
	for _, def := range core.PartyClasses() {
		if _, ok := partyClassPresentations[def.Class]; !ok {
			panic("render: party class " + def.Name + " is missing a partyClassPresentations entry")
		}
	}
}

func partyClassPresentationFor(class core.PartyClass) partyClassPresentation {
	presentation, ok := partyClassPresentations[class]
	if !ok {
		panic("render: partyClassPresentationFor called with unregistered class — init guard should have caught this at startup")
	}
	return presentation
}

// drawClassGlyph paints a small sigil identifying a party-class — the
// kind of pictograph 90s D&D box art used to flank a character name.
// Glyphs are geometric (no per-class texture asset) so they render
// crisp at any DPI:
//
//   - Warrior: crossed swords (an X of two narrow rotated bars)
//   - Cleric:  Greek cross (centred + symbol)
//   - Thief:   downward dagger silhouette (long shaft + crossguard)
//   - Wizard:  five-pointed star sigil
//
// (cx, cy) is the glyph centre; `r` is the glyph's half-extent.
// `col` is the ink colour — callers typically pass the class accent
// tint so the sigil reads as the class's own emblem.
func drawClassGlyph(cx, cy, r float32, class core.PartyClass, col color.RGBA) {
	switch class {
	case core.ClassWarrior:
		drawClassGlyphWarrior(cx, cy, r, col)
	case core.ClassCleric:
		drawClassGlyphCleric(cx, cy, r, col)
	case core.ClassThief:
		drawClassGlyphThief(cx, cy, r, col)
	case core.ClassWizard:
		drawClassGlyphWizard(cx, cy, r, col)
	}
}

// crossed swords — two thick bars rotated ±45°, drawn as two
// parallelograms via triangle pairs so the result is a solid X
// without depending on raylib's rotated-rectangle helper (which
// doesn't expose a fill+rotate primitive in this binding version).
func drawClassGlyphWarrior(cx, cy, r float32, col color.RGBA) {
	const halfThick = float32(1.4)
	// Bar one: top-left ↔ bottom-right.
	a := rl.NewVector2(cx-r-halfThick, cy-r+halfThick)
	b := rl.NewVector2(cx-r+halfThick, cy-r-halfThick)
	c := rl.NewVector2(cx+r+halfThick, cy+r-halfThick)
	d := rl.NewVector2(cx+r-halfThick, cy+r+halfThick)
	drawTriangleCCW(a, d, c, col)
	drawTriangleCCW(a, c, b, col)
	// Bar two: top-right ↔ bottom-left.
	e := rl.NewVector2(cx+r-halfThick, cy-r-halfThick)
	f := rl.NewVector2(cx+r+halfThick, cy-r+halfThick)
	g := rl.NewVector2(cx-r-halfThick, cy+r+halfThick)
	h := rl.NewVector2(cx-r+halfThick, cy+r-halfThick)
	drawTriangleCCW(e, h, g, col)
	drawTriangleCCW(e, g, f, col)
}

// cleric — Greek cross with slightly chunky arms; reads as the
// "+" sigil at any size down to ~12 px.
func drawClassGlyphCleric(cx, cy, r float32, col color.RGBA) {
	arm := r * 0.35
	rl.DrawRectangle(int32(cx-arm), int32(cy-r), int32(arm*2), int32(r*2), col)
	rl.DrawRectangle(int32(cx-r), int32(cy-arm), int32(r*2), int32(arm*2), col)
}

// thief — vertical dagger silhouette: short crossguard + tapered
// blade pointed downward. Tapering done via a triangle so the tip
// reads sharper than a plain rectangle.
func drawClassGlyphThief(cx, cy, r float32, col color.RGBA) {
	// Crossguard (horizontal bar near the top).
	guardW := r * 1.4
	guardH := float32(2)
	rl.DrawRectangle(int32(cx-guardW/2), int32(cy-r*0.65), int32(guardW), int32(guardH), col)
	// Blade — narrow rectangle plus a tip triangle.
	bladeHalfW := float32(1.6)
	bladeTop := cy - r*0.65 + guardH
	bladeBottom := cy + r*0.55
	rl.DrawRectangle(int32(cx-bladeHalfW), int32(bladeTop), int32(bladeHalfW*2), int32(bladeBottom-bladeTop), col)
	tip := rl.NewVector2(cx, cy+r)
	left := rl.NewVector2(cx-bladeHalfW, bladeBottom)
	right := rl.NewVector2(cx+bladeHalfW, bladeBottom)
	drawTriangleCCW(tip, right, left, col)
	// Pommel knob at the top of the hilt.
	rl.DrawRectangle(int32(cx-bladeHalfW), int32(cy-r), int32(bladeHalfW*2), int32(2), col)
}

// wizard — five-pointed star drawn as ten triangles fanned around
// the centre. Each pair of triangles fills one "ray" of the star;
// the arrangement is the classic pentagram with the top point up.
func drawClassGlyphWizard(cx, cy, r float32, col color.RGBA) {
	const points = 5
	outer := r
	inner := r * 0.42
	angleStart := -math.Pi / 2 // start at top
	verts := make([]rl.Vector2, points*2)
	for i := 0; i < points*2; i++ {
		angle := angleStart + float64(i)*math.Pi/float64(points)
		radius := outer
		if i%2 == 1 {
			radius = inner
		}
		verts[i] = rl.NewVector2(
			cx+float32(math.Cos(angle))*radius,
			cy+float32(math.Sin(angle))*radius,
		)
	}
	centre := rl.NewVector2(cx, cy)
	for i := 0; i < len(verts); i++ {
		next := (i + 1) % len(verts)
		// drawTriangleCCW expects CCW vertices in screen-Y-down;
		// (centre, v[i+1], v[i]) is the correct winding for a fan
		// around the centre in this convention.
		drawTriangleCCW(centre, verts[next], verts[i], col)
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

func danceWave(elapsed float32, freq, phase float64) float32 {
	return float32(math.Sin(float64(elapsed)*math.Pi*2*freq + phase))
}

func danceBounce(elapsed float32, freq, phase float64) float32 {
	return (danceWave(elapsed, freq, phase) + 1) * 0.5
}

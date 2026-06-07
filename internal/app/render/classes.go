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
	// turnColor is the member's accent "slot color" — the single source
	// for every HUD/UI/log tint keyed to a class (turn-order panel, party
	// stat cards, panels member cards, combat-log names, target markers).
	// Per-class hues: Warrior gold, Cleric off-white, Thief purple,
	// Wizard pale blue. Tuned bright enough to read on the dark glass HUD
	// and mutually distinct at a glance.
	core.ClassWarrior: {
		turnColor:   color.RGBA{R: 232, G: 184, B: 82, A: 255}, // gold
		textureSeed: 1,
		drawPixels:  drawWarriorPartyPixels,
		dance:       warriorVictoryDance,
	},
	core.ClassCleric: {
		turnColor:   color.RGBA{R: 238, G: 236, B: 226, A: 255}, // off-white
		textureSeed: 2,
		drawPixels:  drawClericPartyPixels,
		dance:       clericVictoryDance,
	},
	core.ClassThief: {
		turnColor:   color.RGBA{R: 182, G: 132, B: 236, A: 255}, // purple
		textureSeed: 3,
		drawPixels:  drawThiefPartyPixels,
		dance:       thiefVictoryDance,
	},
	core.ClassWizard: {
		turnColor:   color.RGBA{R: 148, G: 198, B: 244, A: 255}, // pale blue
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

// classAccent is the per-class accent color (the class's turn color) used for
// card spines, name ticks, and the turn-order panel. One accessor so the six
// draw sites don't each two-hop through partyClassPresentationFor(...).turnColor
// — a single seam if the accent ever stops being the turn color.
func classAccent(class core.PartyClass) rl.Color {
	return partyClassPresentationFor(class).turnColor
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
	default:
		// Parallel to partyClassPresentations' init guard: a new class
		// that forgets a glyph case fails loudly instead of drawing a
		// blank sigil.
		panic("render: drawClassGlyph missing case for party class")
	}
}

// Warrior — two crossed longswords: blades tapered, crossguards
// thicker than the blades, round pommels on the hilt ends, plus a
// central knot pip where the swords meet. Built from triangle pairs
// (blades) + filled circles (pommels) + small rectangles
// (crossguards). The pommels and the meeting-knot use giltBright so
// they catch the eye as cast-metal highlights against the duller
// blade body.
func drawClassGlyphWarrior(cx, cy, r float32, col color.RGBA) {
	highlight := giltBright

	// Each sword has: pommel (top-end), crossguard, tapered blade.
	// Diagonal axes point top-left→bottom-right (sword A) and
	// top-right→bottom-left (sword B). For each, define unit vector
	// along blade (`ax`,`ay`) and perpendicular (`px`,`py`).
	swords := [2]struct {
		ax, ay, px, py float32
	}{
		{ax: sqrt2Inv, ay: sqrt2Inv, px: -sqrt2Inv, py: sqrt2Inv},
		{ax: -sqrt2Inv, ay: sqrt2Inv, px: -sqrt2Inv, py: -sqrt2Inv},
	}
	for _, s := range swords {
		hiltEndX := cx - s.ax*r // sword pointing "inward" tip → outward hilt
		hiltEndY := cy - s.ay*r
		bladeTipX := cx + s.ax*r
		bladeTipY := cy + s.ay*r
		// Tapered blade: start half-width 1.6 near the guard, taper to 0 at the tip.
		guardBaseX := hiltEndX + s.ax*r*0.30
		guardBaseY := hiltEndY + s.ay*r*0.30
		halfW := float32(1.5)
		gL := rl.NewVector2(guardBaseX+s.px*halfW, guardBaseY+s.py*halfW)
		gR := rl.NewVector2(guardBaseX-s.px*halfW, guardBaseY-s.py*halfW)
		tip := rl.NewVector2(bladeTipX, bladeTipY)
		// Two triangles per blade (taper from guard width to a point).
		drawTriangleCCW(gL, tip, gR, col)
		// Crossguard: a thin perpendicular bar sitting just above the guard base.
		guardHalf := r * 0.55
		guardThick := float32(1.4)
		gcL := rl.NewVector2(guardBaseX+s.px*guardHalf, guardBaseY+s.py*guardHalf)
		gcR := rl.NewVector2(guardBaseX-s.px*guardHalf, guardBaseY-s.py*guardHalf)
		gcLi := rl.NewVector2(gcL.X-s.ax*guardThick, gcL.Y-s.ay*guardThick)
		gcRi := rl.NewVector2(gcR.X-s.ax*guardThick, gcR.Y-s.ay*guardThick)
		drawTriangleCCW(gcL, gcLi, gcRi, col)
		drawTriangleCCW(gcL, gcRi, gcR, col)
		// Pommel: small bright disc at the hilt end.
		rl.DrawCircleV(rl.NewVector2(hiltEndX, hiltEndY), 1.8, highlight)
	}
	// Central knot — bright pip where the two swords cross.
	rl.DrawCircleV(rl.NewVector2(cx, cy), 1.6, highlight)
}

// Cleric — fleur-tipped Greek cross with a centre disc, like the
// altar emblem on a 90s D&D paladin sigil. The arms flare slightly
// at their tips (small caps wider than the arm) and the centre
// catches a bright pip. Reads as "holy symbol" at any size down to
// ~12 px without depending on an italic font asset.
func drawClassGlyphCleric(cx, cy, r float32, col color.RGBA) {
	armHalf := r * 0.32
	// Vertical bar.
	rl.DrawRectangle(int32(cx-armHalf), int32(cy-r), int32(armHalf*2), int32(r*2), col)
	// Horizontal bar.
	rl.DrawRectangle(int32(cx-r), int32(cy-armHalf), int32(r*2), int32(armHalf*2), col)
	// Flared tip caps — slightly wider than the arms, near each
	// arm's outer end.
	capHalf := r * 0.5
	capThick := r * 0.18
	if capThick < 1.5 {
		capThick = 1.5
	}
	rl.DrawRectangle(int32(cx-capHalf), int32(cy-r), int32(capHalf*2), int32(capThick), col)        // top
	rl.DrawRectangle(int32(cx-capHalf), int32(cy+r-capThick), int32(capHalf*2), int32(capThick), col) // bottom
	rl.DrawRectangle(int32(cx-r), int32(cy-capHalf), int32(capThick), int32(capHalf*2), col)         // left
	rl.DrawRectangle(int32(cx+r-capThick), int32(cy-capHalf), int32(capThick), int32(capHalf*2), col) // right
	// Centre disc + bright pip.
	rl.DrawCircleV(rl.NewVector2(cx, cy), r*0.30, col)
	rl.DrawCircleV(rl.NewVector2(cx, cy), r*0.16, giltBright)
}

// Thief — single down-pointing dagger: round pommel, broad
// crossguard with downturned ends, tapered double-edged blade with a
// visible centre fuller (a thinner stripe down the middle in a
// darker tone). Reads as a finished weapon sigil rather than a
// silhouette.
func drawClassGlyphThief(cx, cy, r float32, col color.RGBA) {
	bladeHalfW := r * 0.18
	if bladeHalfW < 1.6 {
		bladeHalfW = 1.6
	}
	// Pommel — disc at the top, bright highlight.
	pommelY := cy - r + 1
	rl.DrawCircleV(rl.NewVector2(cx, pommelY), bladeHalfW*1.4, col)
	rl.DrawCircleV(rl.NewVector2(cx, pommelY), bladeHalfW*0.6, giltBright)
	// Crossguard — horizontal bar with small downturned tips, like
	// a baselard / parrying dagger.
	guardY := cy - r*0.55
	guardHalfW := r * 0.75
	guardH := float32(1.8)
	rl.DrawRectangle(int32(cx-guardHalfW), int32(guardY), int32(guardHalfW*2), int32(guardH), col)
	rl.DrawRectangle(int32(cx-guardHalfW), int32(guardY), int32(2), int32(guardH+2), col)        // left horn
	rl.DrawRectangle(int32(cx+guardHalfW-2), int32(guardY), int32(2), int32(guardH+2), col)      // right horn
	// Blade — body rectangle.
	bladeTop := guardY + guardH
	bladeBottom := cy + r*0.62
	rl.DrawRectangle(int32(cx-bladeHalfW), int32(bladeTop), int32(bladeHalfW*2), int32(bladeBottom-bladeTop), col)
	// Centre fuller — a thin darker stripe down the blade.
	fuller := fadeColor(col, 0.5)
	rl.DrawRectangle(int32(cx)-1, int32(bladeTop+2), 1, int32(bladeBottom-bladeTop-4), fuller)
	// Tapered tip triangle.
	tip := rl.NewVector2(cx, cy+r)
	left := rl.NewVector2(cx-bladeHalfW, bladeBottom)
	right := rl.NewVector2(cx+bladeHalfW, bladeBottom)
	drawTriangleCCW(tip, right, left, col)
}

// Wizard — five-pointed star with an inset pentagonal field and a
// bright centre pip, like a scryer's sigil pressed into a brass
// medallion. The inner field reads as the negative space inside the
// star's five "rays" so the silhouette doesn't go solid at small
// sizes.
func drawClassGlyphWizard(cx, cy, r float32, col color.RGBA) {
	const points = 5
	outer := r
	inner := r * 0.45
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
		drawTriangleCCW(centre, verts[next], verts[i], col)
	}
	// Inset darker pentagon — a centre disc that visually pulls the
	// five rays out from a "hub" instead of letting the star sit as
	// a solid silhouette.
	rl.DrawCircleV(centre, r*0.22, fadeColor(col, 0.5))
	// Bright cast-metal pip at the very centre.
	rl.DrawCircleV(centre, r*0.12, giltBright)
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

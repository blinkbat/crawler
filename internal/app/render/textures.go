package render

import (
	"image/color"
	"math"

	"crawler/internal/app/core"
)

// makeSoftShadowPixels builds the radial-gradient sprite used for
// every prop's ground shadow: a dark cool-grey disc whose alpha
// falls from a soft maximum at the centre to fully transparent at
// the disc edge, with the texture's corners fully clear. Painted
// onto a flat ground plane (see loadGroundShadowModel) it reads as
// a soft contact shadow — round, gradient-faded, genuinely
// translucent — instead of the old hard flat square.
//
// The falloff is (1-d)^1.6: nearly flat-dark across the inner
// half, then a gentle feathered edge. Max alpha is deliberately
// modest so the shadow grounds the prop without reading as a black
// hole punched in the floor.
func makeSoftShadowPixels(size int) []color.RGBA {
	pixels := make([]color.RGBA, size*size)
	center := float64(size-1) / 2
	maxR := float64(size) / 2
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx := (float64(x) - center) / maxR
			dy := (float64(y) - center) / maxR
			d := math.Sqrt(dx*dx + dy*dy)
			a := 0.0
			if d < 1.0 {
				a = math.Pow(1.0-d, 1.6)
			}
			pixels[y*size+x] = color.RGBA{R: 22, G: 24, B: 30, A: uint8(a * 135)}
		}
	}
	return pixels
}

func makeRockWallPixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	// Pastel cream-stone wall — warm cliff face catching the
	// daytime sky, like Outset Island's limestone bluffs. The
	// darker / dungeon side gets its mood from the lighting
	// profile (cool ambient + heavy shadowStrength) rather than
	// from the texture going grey.
	base := color.RGBA{R: 188, G: 178, B: 160, A: 255}
	shadow := color.RGBA{R: 120, G: 112, B: 102, A: 255}
	highlight := color.RGBA{R: 226, G: 218, B: 198, A: 255}
	moss := color.RGBA{R: 158, G: 192, B: 148, A: 255}
	mossDeep := color.RGBA{R: 124, G: 168, B: 124, A: 255}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			// Two-octave painted noise — low-frequency band
			// paints big tonal patches like worn weathering, the
			// fine band carries the rocky grain on top.
			broad := fbmNoise(float64(x), float64(y), 0.012, 3)
			fine := fbmNoise(float64(x)*1.4+97, float64(y)*1.4-211, 0.046, 4)
			n := broad*0.55 + fine*0.45
			c := base
			c = core.MixColor(c, highlight, math.Max(0, n)*0.72)
			c = core.MixColor(c, shadow, math.Max(0, -n)*0.55)

			// Painted crack scatter — sparser than the prior
			// hash-grid lines, drawn as small soft pits.
			pit := hashByteXY(x/3, y/3)
			if pit%64 == 0 {
				c = core.MixColor(c, shadow, 0.38)
			}
			// Block divisions: instead of a hard 24-px hash
			// grid, paint a soft seam ONLY where the broad
			// noise is at a low — gives the feeling of cracks
			// running along the rock's natural folds rather
			// than a tiled grid.
			cellX, cellY := x/28, y/28
			seam := hashByteXY(cellX, cellY) % 6
			if ((x+seam)%28 == 0 || (y+seam)%28 == 0) && broad < 0.05 {
				c = core.MixColor(c, shadow, 0.42)
			}
			// Moss creeps up the bottom third of the wall in
			// painterly patches — clusters of bright moss with
			// deeper shadows in the gaps so the moss reads as
			// three-dimensional growth, not a flat green wash.
			if y > h*9/16 {
				dy := float64(y-h*9/16) / float64(h)
				mossNoise := fbmNoise(float64(x)+13, float64(y)*0.7+71, 0.05, 3)
				strength := math.Max(0, mossNoise+dy*0.5-0.18)
				if strength > 0 {
					c = core.MixColor(c, moss, strength*0.65)
					if mossNoise > 0.35 {
						c = core.MixColor(c, mossDeep, (mossNoise-0.35)*0.4)
					}
				}
			}
			pixels[y*w+x] = c
		}
	}
	return pixels
}

func makeStoneBrickPixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	brickW := 32
	brickH := 16
	mortar := 2
	// Pastel cream-stone brick — same painted family as the
	// outdoor rock wall so a dungeon and an outdoor cliff read
	// as the same world. The dungeon's eerie mood comes from the
	// LIGHTING override (dim cool ambient + heavy shadow), not
	// from cold stone colour.
	base := color.RGBA{R: 178, G: 170, B: 154, A: 255}
	warm := color.RGBA{R: 208, G: 184, B: 144, A: 255}
	cool := color.RGBA{R: 152, G: 158, B: 168, A: 255}
	mortarColor := color.RGBA{R: 74, G: 68, B: 60, A: 255}
	mortarLight := color.RGBA{R: 118, G: 110, B: 96, A: 255}
	moss := color.RGBA{R: 148, G: 184, B: 132, A: 255}

	for y := 0; y < h; y++ {
		row := y / brickH
		offset := 0
		if row%2 == 1 {
			offset = brickW / 2
		}
		for x := 0; x < w; x++ {
			localX := (x + offset) % brickW
			localY := y % brickH
			if localX < mortar || localY < mortar {
				c := core.MixColor(mortarColor, mortarLight, float64(hashByteXY(x, y)%64)/200.0)
				if hashByteXY(x/2, y) < 20 {
					c = core.MixColor(c, moss, 0.18)
				}
				pixels[y*w+x] = c
				continue
			}

			brickX := (x + offset) / brickW
			tone := hashByteXY(brickX*7, row*13) % 100
			c := base
			if tone < 32 {
				c = core.MixColor(c, warm, 0.25+float64(tone)/200.0)
			} else if tone < 60 {
				c = core.MixColor(c, cool, 0.18+float64(tone-32)/200.0)
			}

			n := fbmNoise(float64(x)*1.4, float64(y)*1.4, 0.16, 4)
			// Muted highlight (cream → soft stone-white) so the
			// brick crests don't flare against the muted base.
			c = core.MixColor(c, color.RGBA{R: 188, G: 184, B: 172, A: 255}, math.Max(0, n)*0.30)
			c = core.MixColor(c, color.RGBA{R: 30, G: 28, B: 24, A: 255}, math.Max(0, -n)*0.42)

			edgeDist := core.MinInt(localX-mortar, core.MinInt(localY-mortar, core.MinInt(brickW-mortar-1-localX, brickH-mortar-1-localY)))
			if edgeDist <= 2 {
				c = core.MixColor(c, mortarColor, 0.45-float64(edgeDist)*0.12)
			}

			if hashByteXY(brickX*17+localX/3, row*31+localY/3)%80 < 4 {
				c = adjust(c, -36)
			}
			if (localY > brickH-4) && hashByteXY(x, y)%18 < 5 {
				c = core.MixColor(c, moss, 0.32)
			}
			pixels[y*w+x] = c
		}
	}
	return pixels
}

func makeGrassPixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	// Pastel meadow — soft mint base with painted tonal patches.
	// Low-poly philosophy: the texture is a calm wash, NOT a busy
	// noise field. Bloom flowers are scattered very sparsely
	// (1 in ~520) so the discrete flower props carry the floral
	// detail and the floor reads as smooth painted ground from
	// the player's POV. One broad noise band paints big patches;
	// no fine-grain brushstrokes, no per-pixel hash speckle.
	// Pastel but with the chroma pushed back up — soft yet
	// colourful spring green, not washed grey-green. Lightness
	// stays high; saturation comes back by widening the gap
	// between the green channel and the red/blue.
	base := color.RGBA{R: 132, G: 196, B: 102, A: 255}
	light := color.RGBA{R: 186, G: 224, B: 134, A: 255}
	dark := color.RGBA{R: 98, G: 162, B: 92, A: 255}
	dirt := color.RGBA{R: 184, G: 150, B: 100, A: 255}
	bloomYellow := color.RGBA{R: 244, G: 218, B: 120, A: 255}
	bloomWhite := color.RGBA{R: 244, G: 240, B: 224, A: 255}
	bloomPink := color.RGBA{R: 238, G: 174, B: 196, A: 255}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			broad := fbmNoise(float64(x), float64(y), 0.020, 3)
			c := base
			c = core.MixColor(c, light, math.Max(0, broad)*0.55)
			c = core.MixColor(c, dark, math.Max(0, -broad)*0.40)
			// Optional dirt scuff — lazy large patches.
			m := fbmNoise(float64(x)+512, float64(y)-271, 0.014, 2)
			if m > 0.50 {
				c = core.MixColor(c, dirt, (m-0.50)*0.65)
			}
			// Very sparse bloom scatter — the prop-painted
			// flowers carry the floral detail; the texture
			// just hints at "a few wildflowers in the grass."
			seed := hashByteXY(x*7, y*11)
			if seed%520 < 3 {
				switch seed % 3 {
				case 0:
					c = core.MixColor(c, bloomYellow, 0.70)
				case 1:
					c = core.MixColor(c, bloomWhite, 0.62)
				case 2:
					c = core.MixColor(c, bloomPink, 0.66)
				}
			}
			pixels[y*w+x] = c
		}
	}
	return pixels
}

func makeStoneFloorPixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	slab := 32
	grout := 2
	// Pastel cream-slab floor — same painted family as the brick
	// wall. Dungeon mood comes from the lighting override, not
	// from cold stone colour.
	base := color.RGBA{R: 176, G: 172, B: 166, A: 255}
	warm := color.RGBA{R: 206, G: 188, B: 156, A: 255}
	cold := color.RGBA{R: 158, G: 166, B: 176, A: 255}
	groutColor := color.RGBA{R: 96, G: 92, B: 86, A: 255}
	highlight := color.RGBA{R: 224, G: 218, B: 204, A: 255}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			localX := x % slab
			localY := y % slab
			if localX < grout || localY < grout {
				pixels[y*w+x] = jitter(groutColor, x, y, 7)
				continue
			}

			slabX := x / slab
			slabY := y / slab
			tone := hashByteXY(slabX*5, slabY*7) % 100
			c := base
			if tone < 38 {
				c = core.MixColor(c, warm, 0.18+float64(tone)/240.0)
			} else if tone < 70 {
				c = core.MixColor(c, cold, 0.16+float64(tone-38)/260.0)
			}

			n := fbmNoise(float64(x)*1.6, float64(y)*1.6, 0.18, 4)
			c = core.MixColor(c, highlight, math.Max(0, n)*0.32)
			c = core.MixColor(c, color.RGBA{R: 24, G: 22, B: 20, A: 255}, math.Max(0, -n)*0.40)

			edgeDist := core.MinInt(localX-grout, core.MinInt(localY-grout, core.MinInt(slab-1-localX, slab-1-localY)))
			if edgeDist <= 3 {
				c = core.MixColor(c, groutColor, 0.45-float64(edgeDist)*0.10)
			}
			if hashByteXY(slabX*11+localX/4, slabY*19+localY/4)%72 < 3 {
				c = adjust(c, -32)
			}
			pixels[y*w+x] = c
		}
	}
	return pixels
}

// makeDirtPixels paints a warm earth texture for dirt patches mixed into
// the field's grass. Painted-brushstroke noise (matching grass) plus
// scattered pebbles and the occasional sprout of returning green so the
// dirt feels lived-in rather than scorched.
func makeDirtPixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	// Pastel earth — warm peach-tan rather than dark muddy brown.
	// The dirt now reads as soft "sun-warmed path" beside the
	// pastel grass.
	base := color.RGBA{R: 184, G: 152, B: 116, A: 255}
	light := color.RGBA{R: 218, G: 190, B: 148, A: 255}
	dark := color.RGBA{R: 142, G: 110, B: 82, A: 255}
	pebble := color.RGBA{R: 196, G: 188, B: 170, A: 255}
	sprout := color.RGBA{R: 168, G: 198, B: 132, A: 255}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			broad := fbmNoise(float64(x)+91, float64(y)-203, 0.024, 3)
			brush := fbmNoise(float64(x)*1.3+47, float64(y)*1.3+131, 0.078, 4)
			n := broad*0.5 + brush*0.5
			c := base
			c = core.MixColor(c, light, math.Max(0, n)*0.70)
			c = core.MixColor(c, dark, math.Max(0, -n)*0.55)
			if broad > 0.18 && brush > 0.28 {
				c = core.MixColor(c, light, 0.18)
			}
			seed := hashByteXY(x*7, y*11)
			if seed%180 < 3 {
				c = core.MixColor(c, pebble, 0.72)
			} else if seed%420 < 2 {
				// Rare sprout of returning grass — single
				// bright-green speck so the dirt isn't a dead
				// monoculture.
				c = core.MixColor(c, sprout, 0.65)
			}
			pixels[y*w+x] = c
		}
	}
	return pixels
}

// makeDarkGrassPixels paints a deeper-green grass texture for shaded patches
// of the field. Same painterly two-octave brushwork as makeGrassPixels but
// with a pastel forest-mint palette so a shaded glade still reads gentle
// and storybook rather than damp / dim. The variant difference is HUE
// (mint-green vs the lit grass's spring-green) more than VALUE — the day
// cycle handles darkness, the textures stay pastel.
func makeDarkGrassPixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	base := color.RGBA{R: 96, G: 162, B: 116, A: 255}
	light := color.RGBA{R: 150, G: 200, B: 138, A: 255}
	dark := color.RGBA{R: 64, G: 124, B: 92, A: 255}
	moss := color.RGBA{R: 120, G: 184, B: 152, A: 255}
	bloomBlue := color.RGBA{R: 158, G: 196, B: 234, A: 255}
	bloomWhite := color.RGBA{R: 234, G: 238, B: 230, A: 255}
	bloomMagenta := color.RGBA{R: 216, G: 166, B: 208, A: 255}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			broad := fbmNoise(float64(x)+411, float64(y)+97, 0.020, 3)
			m := fbmNoise(float64(x)-227, float64(y)+311, 0.014, 2)
			c := base
			c = core.MixColor(c, light, math.Max(0, broad)*0.55)
			c = core.MixColor(c, dark, math.Max(0, -broad)*0.40)
			if m > 0.46 {
				c = core.MixColor(c, moss, (m-0.46)*0.60)
			}
			seed := hashByteXY(x*5, y*9)
			if seed%520 < 3 {
				switch seed % 3 {
				case 0:
					c = core.MixColor(c, bloomBlue, 0.62)
				case 1:
					c = core.MixColor(c, bloomWhite, 0.58)
				case 2:
					c = core.MixColor(c, bloomMagenta, 0.60)
				}
			}
			// Per-pixel scatter dropped — too busy for the
			// low-poly soft look we're going for.
			pixels[y*w+x] = c
		}
	}
	return pixels
}

// makeCobblePixels paints a mortared cobblestone path: irregular rounded
// stones nudged into a quasi-grid by a hash, with mossy gaps between them
// and subtle wet-spot highlights. Reads as "worn footpath laid by hand."
func makeCobblePixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	// Pastel cobblestone path — soft cream stones over a warm
	// mortar grout. Lifted into the same pastel-cream family
	// as the rock wall so a cobble path through a dungeon
	// (under the spooky lighting override) and one across a
	// field both share the same painted base.
	mortar := color.RGBA{R: 124, G: 116, B: 102, A: 255}
	moss := color.RGBA{R: 148, G: 184, B: 132, A: 255}
	base := color.RGBA{R: 192, G: 186, B: 168, A: 255}
	warm := color.RGBA{R: 218, G: 198, B: 162, A: 255}
	cool := color.RGBA{R: 178, G: 184, B: 188, A: 255}
	dark := color.RGBA{R: 140, G: 134, B: 122, A: 255}
	light := color.RGBA{R: 230, G: 222, B: 200, A: 255}

	const cell = 22
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			row := y / cell
			offset := 0
			if row%2 == 1 {
				offset = cell / 2
			}
			cellX := (x + offset) / cell
			cellY := row
			localX := (x + offset) % cell
			localY := y % cell

			// Per-stone center jitter so the cobbles don't read as a uniform
			// grid. Each stone's "center" walks a couple of pixels off the
			// cell center and its radius wobbles too.
			h0 := hashByteXY(cellX*7, cellY*13)
			cx := cell/2 + (h0%5 - 2)
			cy := cell/2 + ((h0>>3)%5 - 2)
			rx := float64(cell/2 - 2 - (h0>>6)%2)
			ry := float64(cell/2 - 2 - (h0>>8)%2)
			dx := float64(localX - cx)
			dy := float64(localY - cy)
			d := dx*dx/(rx*rx) + dy*dy/(ry*ry)

			if d > 1.0 {
				c := mortar
				if hashByteXY(x/2, y/2)%18 < 7 {
					c = core.MixColor(c, moss, 0.55)
				}
				pixels[y*w+x] = jitter(c, x, y, 5)
				continue
			}

			tone := hashByteXY(cellX*11, cellY*17) % 100
			c := base
			if tone < 38 {
				c = core.MixColor(c, warm, 0.30+float64(tone)/220.0)
			} else if tone < 72 {
				c = core.MixColor(c, cool, 0.22+float64(tone-38)/240.0)
			}

			// Per-pixel cobble shading: pretend each stone is a tiny dome —
			// brighter near its center, darker at the rim. Cheap, gives the
			// path a wet-rounded read.
			lighting := 1.0 - d*0.75
			if lighting > 0 {
				c = core.MixColor(c, light, lighting*0.32)
			}
			if d > 0.78 {
				c = core.MixColor(c, dark, (d-0.78)*1.6)
			}

			n := fbmNoise(float64(x)*1.4, float64(y)*1.4, 0.20, 4)
			c = core.MixColor(c, light, math.Max(0, n)*0.20)
			c = core.MixColor(c, dark, math.Max(0, -n)*0.30)

			// Sparse darker pits in each stone — chips and weather marks.
			if hashByteXY(cellX*23+localX/3, cellY*29+localY/3)%88 < 3 {
				c = adjust(c, -34)
			}
			pixels[y*w+x] = c
		}
	}
	return pixels
}

// makePlankPixels paints a horizontal wooden plank floor: alternating wide
// boards with darker gaps, with a grain noise across each board and a
// scatter of darker knots. The board offset shifts by row group so the
// gaps between boards stagger like a real laid floor.
func makePlankPixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	const boardH = 22
	const gap = 2
	gapColor := color.RGBA{R: 96, G: 70, B: 48, A: 255}
	// Pastel honey-wood plank floor — soft warm timber matching
	// the pastel bark family. Knots / grain kept subtle for the
	// low-poly soft look.
	base := color.RGBA{R: 186, G: 146, B: 102, A: 255}
	warm := color.RGBA{R: 212, G: 176, B: 128, A: 255}
	cool := color.RGBA{R: 162, G: 122, B: 84, A: 255}
	grain := color.RGBA{R: 138, G: 102, B: 68, A: 255}
	knot := color.RGBA{R: 112, G: 80, B: 52, A: 255}

	for y := 0; y < h; y++ {
		boardRow := y / boardH
		localY := y % boardH
		offset := (boardRow * 17) % 32
		for x := 0; x < w; x++ {
			localX := (x + offset) % 96
			// Plank-to-plank gap (vertical seam). Each board on the row is
			// 96 px wide so the seams don't line up between rows of boards.
			if localX < gap || localY < gap {
				pixels[y*w+x] = jitter(gapColor, x, y, 6)
				continue
			}
			tone := hashByteXY((x+offset)/96, boardRow*5) % 100
			c := base
			if tone < 38 {
				c = core.MixColor(c, warm, 0.25+float64(tone)/240.0)
			} else if tone < 72 {
				c = core.MixColor(c, cool, 0.22+float64(tone-38)/240.0)
			}
			// Long horizontal grain: low-frequency stretch along x, higher
			// frequency in y so it reads as wood fibers not stone veins.
			n := fbmNoise(float64(x)*0.15, float64(y)*1.6, 0.20, 4)
			c = core.MixColor(c, warm, math.Max(0, n)*0.35)
			c = core.MixColor(c, grain, math.Max(0, -n)*0.55)

			// Edge of the board: darken slightly so each plank reads as
			// raised against its neighbor.
			edge := core.MinInt(localY-gap, boardH-1-localY)
			if edge <= 2 {
				c = core.MixColor(c, gapColor, 0.30-float64(edge)*0.10)
			}

			// Knots: small disc darker spots, ~1 per board.
			if hashByteXY((x+offset)/8, y/3)%420 < 3 {
				c = core.MixColor(c, knot, 0.70)
			}
			pixels[y*w+x] = c
		}
	}
	return pixels
}

// makeWaterPixels paints shallow water: a banded blue gradient with rolling
// FBM ripples and a few brighter highlight peaks. No animation — but the
// gentle banded shimmer reads as still water catching ambient light. Sits
// at the same Y as floor cubes (slightly recessed) so the player walks
// through, not over. Palette is intentionally light/airy so the tile
// reads as wadeable next to the darker FloorDeepWater variant.
func makeWaterPixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	// Pastel painted water — soft but clearly aqua, the gentle
	// pond in a sunlit meadow. Chroma bumped so the water reads
	// blue-green rather than pale grey-blue.
	deep := color.RGBA{R: 96, G: 168, B: 192, A: 255}
	mid := color.RGBA{R: 150, G: 204, B: 214, A: 255}
	shine := color.RGBA{R: 220, G: 238, B: 232, A: 255}
	sand := color.RGBA{R: 214, G: 190, B: 144, A: 255}
	weed := color.RGBA{R: 108, G: 170, B: 110, A: 255}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			n := fbmNoise(float64(x), float64(y), 0.04, 4)
			band := math.Sin(float64(y)*0.08 + n*1.8)
			c := core.MixColor(deep, mid, 0.45+band*0.35+n*0.20)

			// Bright crests where the noise hits a peak.
			peak := fbmNoise(float64(x)*1.3+311, float64(y)*1.3-91, 0.10, 3)
			if peak > 0.55 {
				c = core.MixColor(c, shine, (peak-0.55)*1.4)
			}
			// Hint of sandy bottom where the FBM dips deep — reads as water
			// that you can almost see through.
			if n < -0.45 {
				c = core.MixColor(c, sand, (-n-0.45)*0.5)
			}
			// Rare strands of weed for life.
			if hashByteXY(x/2, y*3)%560 < 4 {
				c = core.MixColor(c, weed, 0.45)
			}
			pixels[y*w+x] = c
		}
	}
	return pixels
}

// makeDeepWaterPixels paints the blocking deep-water variant: same banded
// shimmer shape as makeWaterPixels but a noticeably darker, cooler palette
// and no sandy bottom hint (the floor below isn't visible at depth). The
// contrast against shallow water is the visual cue that this tile can't
// be waded into — see FloorDeepWater in core/map.go.
func makeDeepWaterPixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	// Pastel deep water — soft teal that's clearly deeper / cooler
	// than the wadeable shallow water but still painted-pastel
	// rather than near-black navy. The "you can't enter" read
	// comes from the colder, more saturated tone vs shallow.
	deep := color.RGBA{R: 92, G: 138, B: 160, A: 255}
	mid := color.RGBA{R: 124, G: 168, B: 184, A: 255}
	shine := color.RGBA{R: 196, G: 220, B: 222, A: 255}
	weed := color.RGBA{R: 96, G: 148, B: 120, A: 255}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			n := fbmNoise(float64(x), float64(y), 0.04, 4)
			band := math.Sin(float64(y)*0.08 + n*1.8)
			c := core.MixColor(deep, mid, 0.45+band*0.35+n*0.20)

			peak := fbmNoise(float64(x)*1.3+311, float64(y)*1.3-91, 0.10, 3)
			if peak > 0.62 {
				c = core.MixColor(c, shine, (peak-0.62)*0.9)
			}
			if hashByteXY(x/2, y*3)%620 < 3 {
				c = core.MixColor(c, weed, 0.40)
			}
			pixels[y*w+x] = c
		}
	}
	return pixels
}

// makeSandPixels paints pale dune sand: warm cream base with finer noise
// grain than the dirt texture and very sparse darker pebbles. Reads as
// dry, sun-bleached sand rather than wet beach sand (which would want a
// cooler tone).
func makeSandPixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	// Pastel dune sand — soft warm cream. Low-poly soft pass:
	// dropped the per-pixel grain speckle, kept only the gentle
	// dune-roll noise so the sand reads as a smooth painted
	// surface rather than a gritty stipple.
	base := color.RGBA{R: 224, G: 206, B: 168, A: 255}
	warm := color.RGBA{R: 240, G: 224, B: 188, A: 255}
	dark := color.RGBA{R: 196, G: 174, B: 134, A: 255}
	pebble := color.RGBA{R: 176, G: 156, B: 128, A: 255}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dune := fbmNoise(float64(x)+47, float64(y)-83, 0.018, 3)
			c := base
			c = core.MixColor(c, warm, math.Max(0, dune)*0.40)
			c = core.MixColor(c, dark, math.Max(0, -dune)*0.32)
			// Very sparse pebble — a single soft fleck here and
			// there for character.
			if hashByteXY(x*7, y*11)%680 < 2 {
				c = core.MixColor(c, pebble, 0.45)
			}
			pixels[y*w+x] = c
		}
	}
	return pixels
}

// makeSnowPixels paints packed snow: near-white with very faint blue
// shadow noise and a sparkle of brighter specks. Looks washed-out under
// neutral light by design — the day/night cycle's bluer phases tint it
// into something atmospheric.
func makeSnowPixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	base := color.RGBA{R: 232, G: 240, B: 248, A: 255}
	shadow := color.RGBA{R: 168, G: 188, B: 218, A: 255}
	deepShadow := color.RGBA{R: 132, G: 156, B: 192, A: 255}
	sparkle := color.RGBA{R: 255, G: 255, B: 255, A: 255}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			n := fbmNoise(float64(x), float64(y), 0.05, 4)
			drift := fbmNoise(float64(x)+201, float64(y)+311, 0.015, 3)
			c := base
			c = core.MixColor(c, shadow, math.Max(0, -n)*0.22)
			c = core.MixColor(c, deepShadow, math.Max(0, -drift)*0.18)
			// Sparkle specks: very rare bright pixels read as light
			// glinting off ice crystals.
			if hashByteXY(x*5, y*7)%900 < 3 {
				c = core.MixColor(c, sparkle, 0.85)
			}
			pixels[y*w+x] = c
		}
	}
	return pixels
}

func makeBarkPixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	// Pastel bark — warm pecan in the lit zones, soft umber in
	// the creases. Reads as a sun-warm trunk in a meadow rather
	// than the prior near-black umber that looked like a burnt
	// log when the lighting fell off.
	base := color.RGBA{R: 168, G: 130, B: 96, A: 255}
	deep := color.RGBA{R: 104, G: 76, B: 56, A: 255}
	light := color.RGBA{R: 214, G: 184, B: 142, A: 255}
	moss := color.RGBA{R: 154, G: 188, B: 138, A: 255}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			// Sinuous ridge wave — same trick as before but the
			// noise warp is gentler so the ridges flow as
			// painted brush-strokes, not jagged sawteeth.
			ridge := math.Sin(float64(x)*0.40 + fbmNoise(float64(x), float64(y), 0.04, 3)*3.2)
			ridge = math.Abs(ridge)
			n := fbmNoise(float64(x)*0.9, float64(y)*0.4, 0.22, 4)
			c := base
			c = core.MixColor(c, light, math.Max(0, ridge-0.42)*1.5)
			c = core.MixColor(c, deep, math.Max(0, 0.38-ridge)*0.7)
			c = core.MixColor(c, light, math.Max(0, n)*0.32)
			c = core.MixColor(c, deep, math.Max(0, -n)*0.40)

			// Sparser pit + denser moss patches — moss looks
			// vertical (creeping up the trunk's shaded side)
			// rather than randomly speckled.
			if hashByteXY(x/3, y/3)%180 < 2 {
				c = core.MixColor(c, deep, 0.65)
			}
			mossNoise := fbmNoise(float64(x)*1.4+71, float64(y)*0.5+213, 0.10, 3)
			if mossNoise > 0.28 {
				c = core.MixColor(c, moss, (mossNoise-0.28)*0.55)
			}
			pixels[y*w+x] = c
		}
	}
	return pixels
}

func makeLeafPixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	// Pastel canopy leaves — soft but saturated spring green with
	// cream sun-dapples and lemon-gold accents. Chroma pushed
	// back up from the washed pass so the canopy reads as lush.
	base := color.RGBA{R: 142, G: 204, B: 110, A: 255}
	light := color.RGBA{R: 200, G: 232, B: 144, A: 255}
	deep := color.RGBA{R: 96, G: 160, B: 96, A: 255}
	gold := color.RGBA{R: 238, G: 224, B: 138, A: 255}
	hotspot := color.RGBA{R: 236, G: 244, B: 180, A: 255}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			// Two-octave painted noise — clumpy patches plus a
			// finer brushstroke so the foliage feels hand-
			// painted across a sphere mesh.
			broad := fbmNoise(float64(x)*0.7, float64(y)*0.7, 0.06, 3)
			fine := fbmNoise(float64(x)*1.2, float64(y)*1.2, 0.18, 4)
			n := broad*0.55 + fine*0.45
			m := fbmNoise(float64(x)+183, float64(y)-77, 0.05, 3)
			c := base
			c = core.MixColor(c, light, math.Max(0, n)*0.80)
			c = core.MixColor(c, deep, math.Max(0, -n)*0.55)
			if m > 0.50 {
				c = core.MixColor(c, gold, (m-0.50)*0.70)
			}
			// Sunlit hotspots — small bright kisses where the
			// broad and fine noise crests align. Like sunlight
			// finding a gap in the canopy.
			if broad > 0.32 && fine > 0.38 {
				c = core.MixColor(c, hotspot, 0.25)
			}
			pixels[y*w+x] = c
		}
	}
	return pixels
}

// makeMarblePixels paints pale veined marble for upright props — pillars,
// the statue, stalagmites, fountain basins. Two veins worth of noise
// woven through a creamy off-white base, with hairline dark cracks so the
// stone reads as quarried rather than blank.
func makeMarblePixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	// Muted marble — softer cream-grey base so pillars and the
	// statue don't flare against the muted floor / wall palette.
	base := color.RGBA{R: 206, G: 200, B: 188, A: 255}
	warm := color.RGBA{R: 216, G: 208, B: 192, A: 255}
	cool := color.RGBA{R: 180, G: 182, B: 188, A: 255}
	vein := color.RGBA{R: 116, G: 110, B: 102, A: 255}
	deep := color.RGBA{R: 76, G: 72, B: 66, A: 255}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			n := fbmNoise(float64(x), float64(y), 0.04, 4)
			m := fbmNoise(float64(x)+177, float64(y)-91, 0.10, 3)
			c := base
			c = core.MixColor(c, warm, math.Max(0, n)*0.35)
			c = core.MixColor(c, cool, math.Max(0, -n)*0.30)

			// Veins: pixel-thin streaks where two FBM samples cross zero.
			vt := math.Abs(m + n*0.4)
			if vt < 0.06 {
				c = core.MixColor(c, vein, 0.45)
			}
			if vt < 0.02 {
				c = core.MixColor(c, deep, 0.55)
			}
			pixels[y*w+x] = c
		}
	}
	return pixels
}

// makeGranitePixels paints a dark, faintly speckled granite for the
// obelisk. The mix is denser and cooler than the marble palette so an
// obelisk reads as a different stone class against an adjacent pillar.
func makeGranitePixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	base := color.RGBA{R: 60, G: 64, B: 76, A: 255}
	light := color.RGBA{R: 112, G: 116, B: 132, A: 255}
	dark := color.RGBA{R: 24, G: 26, B: 36, A: 255}
	flake := color.RGBA{R: 188, G: 188, B: 200, A: 255}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			n := fbmNoise(float64(x)*1.3, float64(y)*1.3, 0.18, 4)
			c := base
			c = core.MixColor(c, light, math.Max(0, n)*0.40)
			c = core.MixColor(c, dark, math.Max(0, -n)*0.45)
			// Mica flecks: rare bright pixels for sparkle.
			if hashByteXY(x*5, y*5)%420 < 3 {
				c = core.MixColor(c, flake, 0.55)
			}
			pixels[y*w+x] = c
		}
	}
	return pixels
}

// makeTerracottaPixels paints a warm clay sidewall for the urn. Light
// horizontal banding (potter's wheel marks) plus subtle vertical
// gradient so the surface reads as fired clay rather than painted plastic.
func makeTerracottaPixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	// Pastel terracotta — soft warm clay / apricot.
	base := color.RGBA{R: 212, G: 150, B: 116, A: 255}
	light := color.RGBA{R: 234, G: 184, B: 148, A: 255}
	dark := color.RGBA{R: 170, G: 110, B: 82, A: 255}
	rim := color.RGBA{R: 138, G: 88, B: 64, A: 255}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			n := fbmNoise(float64(x)*0.8, float64(y)*0.4, 0.14, 3)
			band := math.Sin(float64(y)*0.45+n*1.2) * 0.5
			c := base
			c = core.MixColor(c, light, math.Max(0, band)*0.35+math.Max(0, n)*0.25)
			c = core.MixColor(c, dark, math.Max(0, -band)*0.30+math.Max(0, -n)*0.25)
			// Sparse darker pits and chips.
			if hashByteXY(x, y)%240 < 2 {
				c = core.MixColor(c, rim, 0.55)
			}
			pixels[y*w+x] = c
		}
	}
	return pixels
}

// fbmNoise returns fractal Brownian motion in roughly [-1, 1] using a hashed
// value-noise base. We use it to add organic variation to procedural textures
// so the lit surfaces don't look flat under directional lighting.
func fbmNoise(x, y, frequency float64, octaves int) float64 {
	value := 0.0
	amplitude := 1.0
	totalAmplitude := 0.0
	for i := 0; i < octaves; i++ {
		value += valueNoise(x*frequency, y*frequency) * amplitude
		totalAmplitude += amplitude
		amplitude *= 0.5
		frequency *= 2.0
	}
	if totalAmplitude == 0 {
		return 0
	}
	return value / totalAmplitude
}

func valueNoise(x, y float64) float64 {
	xi := int(math.Floor(x))
	yi := int(math.Floor(y))
	xf := x - math.Floor(x)
	yf := y - math.Floor(y)

	c00 := hashFloat(xi, yi)
	c10 := hashFloat(xi+1, yi)
	c01 := hashFloat(xi, yi+1)
	c11 := hashFloat(xi+1, yi+1)

	u := xf * xf * (3 - 2*xf)
	v := yf * yf * (3 - 2*yf)

	x1 := c00 + u*(c10-c00)
	x2 := c01 + u*(c11-c01)
	return (x1+v*(x2-x1))*2.0 - 1.0
}

func hashFloat(x, y int) float64 {
	return float64(hashXY(x, y)&0xFFFF) / 65535.0
}

func makeSkyPixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	// Pastel painted sky — soft baby-blue at the zenith, warm
	// peach-cream at the horizon. The classic Wind-Waker
	// daytime gradient that says "calm afternoon on the open
	// water" rather than dramatic HDR sky.
	top := color.RGBA{R: 132, G: 188, B: 230, A: 255}
	horizon := color.RGBA{R: 248, G: 222, B: 198, A: 255}
	clouds := []struct {
		X  float64
		y  float64
		rx float64
		ry float64
	}{
		{90, 126, 145, 34},
		{314, 170, 178, 42},
		{560, 112, 160, 38},
		{812, 192, 210, 48},
		{980, 132, 130, 32},
	}
	// Cloud tint pulled off pure white so the painted clouds
	// don't glare against the muted sky.
	cloudCol := color.RGBA{R: 232, G: 234, B: 232, A: 255}

	for y := 0; y < h; y++ {
		t := float64(y) / float64(h-1)
		t = t * t * (3 - 2*t)
		for x := 0; x < w; x++ {
			c := core.MixColor(top, horizon, t)
			cover := 0.0
			for _, cloud := range clouds {
				dx := math.Abs(float64(x) - cloud.X)
				if wrapped := float64(w) - dx; wrapped < dx {
					dx = wrapped
				}
				dy := float64(y) - cloud.y
				d := (dx*dx)/(cloud.rx*cloud.rx) + (dy*dy)/(cloud.ry*cloud.ry)
				cover += math.Exp(-d*2.6) * 0.34
			}
			if cover > 0 {
				cover = math.Min(cover, 0.5)
				c = core.MixColor(c, cloudCol, cover)
			}
			pixels[y*w+x] = c
		}
	}
	return pixels
}

// makeStarPixels builds the transparent star-field overlay sampled by
// DrawSkyBackground at night. The texture is mostly RGBA(0,0,0,0); a
// sparse scatter of single bright pixels (with a 4-neighbor halo at
// lower alpha) reads as pinpoint stars when drawn at screen scale.
// Star density tapers from the top of the texture down toward the
// horizon — stars near the horizon are washed out by atmospheric
// scattering even at midnight, so the bottom 30% of the texture stays
// nearly empty. A handful of "bright" stars get a slightly larger
// halo + warm-white tint so the field doesn't read as a uniform
// noise field. Star colors walk between cool white, pale blue, and
// warm yellow — the standard star-temperature trio — so a careful
// look at the field reveals subtle variety.
//
// The randomness is hash-driven (hashFloat) rather than rand.* so the
// star map is stable across runs and platforms — a player who looks
// up at midnight tonight sees the same constellation tomorrow.
func makeStarPixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	transparent := color.RGBA{R: 0, G: 0, B: 0, A: 0}
	for i := range pixels {
		pixels[i] = transparent
	}

	// Star palette: cool white (most common), pale blue (hot stars),
	// warm yellow (cooler stars). Pulled by a separate hash byte so
	// color and brightness are independent.
	coolWhite := color.RGBA{R: 240, G: 244, B: 252, A: 255}
	paleBlue := color.RGBA{R: 198, G: 218, B: 252, A: 255}
	warmYellow := color.RGBA{R: 252, G: 234, B: 192, A: 255}
	colorFor := func(b int) color.RGBA {
		switch {
		case b < 24:
			return paleBlue
		case b < 56:
			return warmYellow
		default:
			return coolWhite
		}
	}
	setPx := func(x, y int, c color.RGBA) {
		if x < 0 || x >= w || y < 0 || y >= h {
			return
		}
		// Overwrite, not blend — stars don't overlap meaningfully at
		// this density and "max over" would otherwise dim a bright
		// star sitting on top of a dim one's halo.
		pixels[y*w+x] = c
	}

	// Density falloff: the upper sky (low y) is the densest; the
	// horizon (high y) is nearly clear. A quadratic taper sells the
	// effect without being a hard cutoff. Total star count is roughly
	// w*h*baseProb*avg(densityAt) — at the current 0.0035 base, ~1024×512
	// gives ~700 stars before halos, sparse enough to read as a real
	// night sky rather than a noise field. The previous 0.008 pass
	// produced a cluttered patch even at midnight; halving the rate
	// AND adding per-star opacity variance below gives the layer
	// breathing room.
	const baseProb = 0.0035
	densityAt := func(y int) float64 {
		t := float64(y) / float64(h-1)
		// 1 at the top, tapering to ~0.05 at the bottom.
		falloff := 1.0 - t*t*1.05
		if falloff < 0.05 {
			falloff = 0.05
		}
		return falloff
	}

	// Per-star brightness curve: most stars are faint, a few are
	// medium, a handful are bright. Mapped from the brightness byte
	// (0..255) to a core alpha so the eye reads a starfield with
	// depth rather than a flat dot pattern. Curve buckets:
	//
	//   < 160 (63%)  → core alpha 70   (dim, barely visible)
	//   < 220 (24%)  → core alpha 130  (medium, the bulk of the field)
	//   < 248 (11%)  → core alpha 200  (bright)
	//        else (3%) → core alpha 255  (the brightest pinpoints)
	//
	// Halo + sparkle alphas scale off the core so a dim star has a
	// dim halo and the brightest stars get the strongest glow.
	coreAlphaFor := func(b int) uint8 {
		switch {
		case b < 160:
			return 70
		case b < 220:
			return 130
		case b < 248:
			return 200
		default:
			return 255
		}
	}

	for y := 0; y < h; y++ {
		dens := densityAt(y)
		thresh := uint16(baseProb * dens * 65535)
		for x := 0; x < w; x++ {
			// hashXY already multiplies by 73856093 / 19349663 — don't
			// pre-multiply at the call site or the inner math overflows
			// before mix32 sees it and the distribution skews.
			h0 := uint16(hashXY(x, y))
			if h0 >= thresh {
				continue
			}
			// Secondary hash drives brightness + color + halo. Offset
			// by a constant pair so it's decorrelated from h0.
			h1 := hashXY(x+91317, y+58271)
			brightness := int(h1 & 0xFF)
			coreAlpha := coreAlphaFor(brightness)
			col := colorFor(brightness)
			col.A = coreAlpha

			// Core pixel.
			setPx(x, y, col)

			// Halo + sparkle only on the brighter half of the
			// brightness curve — dim stars stay as a single pixel so
			// the field has breathing room. Halo alpha is ~55% of the
			// core, sparkle is ~30%, so brightness propagates from
			// pinpoint outward.
			if coreAlpha >= 130 {
				haloRoll := int((h1 >> 8) & 0xFF)
				if haloRoll < 140 { // ~55% of the brighter stars
					halo := col
					halo.A = uint8(int(coreAlpha) * 55 / 100)
					setPx(x-1, y, halo)
					setPx(x+1, y, halo)
					setPx(x, y-1, halo)
					setPx(x, y+1, halo)
				}
				if coreAlpha == 255 && haloRoll < 80 { // sparkle on the brightest, ~30% of those
					sparkle := col
					sparkle.A = uint8(int(coreAlpha) * 30 / 100)
					setPx(x-1, y-1, sparkle)
					setPx(x+1, y-1, sparkle)
					setPx(x-1, y+1, sparkle)
					setPx(x+1, y+1, sparkle)
				}
			}
		}
	}
	return pixels
}

// ratPalette is the recolorable palette used by makeRatPixels. Body / ear /
// tail tones differ between the plain rat and the diseased rat, but the
// silhouette is shared.
type ratPalette struct {
	body, bodyDark, bodyLight color.RGBA
	ear, tail                 color.RGBA
	eye, nose                 color.RGBA
	// Poison drip color: when non-zero alpha, makeRatPixels paints a few
	// dripping poison drops under the snout. Used by the diseased rat so
	// the field figure reads as "leaking something nasty."
	poison color.RGBA
}

var defaultRatPalette = ratPalette{
	body:      color.RGBA{R: 104, G: 107, B: 104, A: 255},
	bodyDark:  color.RGBA{R: 68, G: 72, B: 72, A: 255},
	bodyLight: color.RGBA{R: 138, G: 142, B: 136, A: 255},
	ear:       color.RGBA{R: 172, G: 116, B: 122, A: 255},
	tail:      color.RGBA{R: 178, G: 118, B: 125, A: 255},
	eye:       color.RGBA{R: 10, G: 12, B: 12, A: 255},
	nose:      color.RGBA{R: 232, G: 150, B: 162, A: 255},
}

// diseasedRatPalette swaps the rat to mottled sickly-green tones with a
// jaundiced yellow eye and a fleshy nose. The poison field paints visible
// drips under the snout.
var diseasedRatPalette = ratPalette{
	body:      color.RGBA{R: 86, G: 118, B: 64, A: 255},
	bodyDark:  color.RGBA{R: 48, G: 76, B: 36, A: 255},
	bodyLight: color.RGBA{R: 138, G: 168, B: 92, A: 255},
	ear:       color.RGBA{R: 142, G: 118, B: 110, A: 255},
	tail:      color.RGBA{R: 140, G: 116, B: 108, A: 255},
	eye:       color.RGBA{R: 220, G: 200, B: 60, A: 255},
	nose:      color.RGBA{R: 170, G: 116, B: 132, A: 255},
	poison:    color.RGBA{R: 156, G: 220, B: 88, A: 255},
}

func makeRatPixels(w, h int) []color.RGBA {
	return makeRatPixelsWithPalette(w, h, defaultRatPalette)
}

func makeDiseasedRatPixels(w, h int) []color.RGBA {
	return makeRatPixelsWithPalette(w, h, diseasedRatPalette)
}

func makeRatPixelsWithPalette(w, h int, p ratPalette) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	body := p.body
	bodyDark := p.bodyDark
	bodyLight := p.bodyLight
	ear := p.ear
	tail := p.tail
	eye := p.eye
	nose := p.nose

	fillEllipsePixels(pixels, w, h, 36, 87, 21, 4, color.RGBA{R: 0, G: 0, B: 0, A: 75})
	drawLinePixels(pixels, w, h, 19, 72, 7, 62, tail, 3)
	drawLinePixels(pixels, w, h, 7, 62, 15, 49, tail, 3)

	fillEllipsePixels(pixels, w, h, 35, 56, 21, 28, bodyDark)
	fillEllipsePixels(pixels, w, h, 38, 53, 18, 27, body)
	fillEllipsePixels(pixels, w, h, 40, 58, 10, 19, bodyLight)
	fillEllipsePixels(pixels, w, h, 25, 82, 10, 5, bodyDark)
	fillEllipsePixels(pixels, w, h, 49, 82, 10, 5, bodyDark)

	drawLinePixels(pixels, w, h, 25, 51, 15, 43, bodyDark, 5)
	drawLinePixels(pixels, w, h, 50, 51, 60, 44, bodyDark, 5)
	fillEllipsePixels(pixels, w, h, 14, 42, 4, 4, bodyLight)
	fillEllipsePixels(pixels, w, h, 60, 43, 4, 4, bodyLight)

	fillEllipsePixels(pixels, w, h, 28, 17, 7, 9, ear)
	fillEllipsePixels(pixels, w, h, 48, 16, 7, 9, ear)
	fillEllipsePixels(pixels, w, h, 28, 18, 4, 6, adjust(ear, 22))
	fillEllipsePixels(pixels, w, h, 48, 17, 4, 6, adjust(ear, 22))
	fillEllipsePixels(pixels, w, h, 38, 32, 18, 16, body)
	fillEllipsePixels(pixels, w, h, 49, 35, 9, 7, bodyLight)
	fillEllipsePixels(pixels, w, h, 45, 29, 2, 2, eye)
	fillEllipsePixels(pixels, w, h, 57, 36, 3, 3, nose)
	drawLinePixels(pixels, w, h, 55, 38, 66, 34, bodyLight, 1)
	drawLinePixels(pixels, w, h, 55, 39, 67, 40, bodyLight, 1)
	drawLinePixels(pixels, w, h, 55, 40, 64, 47, bodyLight, 1)

	// Poison drips: only on palettes with a non-zero poison color (the
	// diseased rat). Three drops trailing down from the snout.
	if p.poison.A != 0 {
		poison := p.poison
		poisonDark := adjust(poison, -28)
		// Drip 1 — biggest, hanging from the nose.
		fillEllipsePixels(pixels, w, h, 60, 42, 2, 3, poison)
		fillEllipsePixels(pixels, w, h, 60, 45, 1, 1, poisonDark)
		// Drip 2 — mid-size, slightly offset.
		fillEllipsePixels(pixels, w, h, 56, 48, 2, 2, poison)
		// Drip 3 — small puddle below.
		fillEllipsePixels(pixels, w, h, 58, 52, 3, 1, poisonDark)
	}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*w + x
			if pixels[i].A == 0 {
				continue
			}
			if hashByteXY(x, y)%7 == 0 {
				pixels[i] = adjust(pixels[i], -12)
			}
		}
	}
	return pixels
}

// makeBatPixels paints a wing-spread cave bat silhouette: dark body with
// red eye accents and lighter wing membranes that scallop out to either
// side. Sized to the loadEnemyVisuals dimensions (80x88) so the wings
// nearly fill horizontally and the body sits in the lower-center.
func makeBatPixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	body := color.RGBA{R: 32, G: 28, B: 36, A: 255}
	bodyMid := color.RGBA{R: 56, G: 50, B: 62, A: 255}
	bodyLight := color.RGBA{R: 84, G: 76, B: 92, A: 255}
	wing := color.RGBA{R: 60, G: 50, B: 70, A: 255}
	wingMid := color.RGBA{R: 92, G: 78, B: 102, A: 255}
	wingDark := color.RGBA{R: 28, G: 22, B: 32, A: 255}
	eye := color.RGBA{R: 230, G: 90, B: 86, A: 255}
	fang := color.RGBA{R: 232, G: 224, B: 218, A: 255}

	cx := w / 2
	bodyTop := h * 28 / 100

	// Wing membranes: build each side from three overlapping triangles that
	// scallop the trailing edge. Drawn before the body so the body sits on
	// top.
	leftAnchor := cx - 4
	rightAnchor := cx + 4
	wingY := bodyTop + 6

	// Left wing primary surface.
	fillTrianglePixels(pixels, w, h, leftAnchor, wingY-4, leftAnchor, wingY+24, 4, wingY+8, wing)
	fillTrianglePixels(pixels, w, h, leftAnchor, wingY-4, 4, wingY+8, 16, wingY-2, wing)
	fillTrianglePixels(pixels, w, h, leftAnchor, wingY+8, leftAnchor, wingY+22, 6, wingY+24, wing)
	// Left wing secondary lighter band.
	fillTrianglePixels(pixels, w, h, leftAnchor, wingY-2, 18, wingY+6, 8, wingY+18, wingMid)
	// Trailing-edge dark tips (scallops).
	fillTrianglePixels(pixels, w, h, 4, wingY+8, 8, wingY+14, 2, wingY+16, wingDark)
	fillTrianglePixels(pixels, w, h, 8, wingY+18, 14, wingY+22, 6, wingY+26, wingDark)

	// Right wing — mirror.
	fillTrianglePixels(pixels, w, h, rightAnchor, wingY-4, rightAnchor, wingY+24, w-5, wingY+8, wing)
	fillTrianglePixels(pixels, w, h, rightAnchor, wingY-4, w-5, wingY+8, w-17, wingY-2, wing)
	fillTrianglePixels(pixels, w, h, rightAnchor, wingY+8, rightAnchor, wingY+22, w-7, wingY+24, wing)
	fillTrianglePixels(pixels, w, h, rightAnchor, wingY-2, w-19, wingY+6, w-9, wingY+18, wingMid)
	fillTrianglePixels(pixels, w, h, w-5, wingY+8, w-9, wingY+14, w-3, wingY+16, wingDark)
	fillTrianglePixels(pixels, w, h, w-9, wingY+18, w-15, wingY+22, w-7, wingY+26, wingDark)

	// Body — chunky teardrop. Two ellipses stacked + ear nubs.
	fillEllipsePixels(pixels, w, h, cx, bodyTop+18, 10, 16, body)
	fillEllipsePixels(pixels, w, h, cx, bodyTop+8, 8, 8, bodyMid)
	fillEllipsePixels(pixels, w, h, cx-3, bodyTop+9, 3, 3, bodyLight)
	fillEllipsePixels(pixels, w, h, cx+3, bodyTop+9, 3, 3, bodyLight)
	// Pointy ears.
	fillTrianglePixels(pixels, w, h, cx-7, bodyTop+2, cx-3, bodyTop-6, cx-2, bodyTop+4, body)
	fillTrianglePixels(pixels, w, h, cx+7, bodyTop+2, cx+3, bodyTop-6, cx+2, bodyTop+4, body)

	// Eyes — one-pixel red dots, with surrounding 1px frame so they read
	// against the dark face.
	fillEllipsePixels(pixels, w, h, cx-3, bodyTop+8, 1, 1, eye)
	fillEllipsePixels(pixels, w, h, cx+3, bodyTop+8, 1, 1, eye)
	// Tiny fangs — two single-pixel triangles at the bottom of the face.
	fillTrianglePixels(pixels, w, h, cx-2, bodyTop+13, cx-1, bodyTop+15, cx, bodyTop+13, fang)
	fillTrianglePixels(pixels, w, h, cx+2, bodyTop+13, cx+1, bodyTop+15, cx, bodyTop+13, fang)

	// Cast shadow blob under the bat for contact.
	fillEllipsePixels(pixels, w, h, cx, h-6, 14, 3, color.RGBA{R: 0, G: 0, B: 0, A: 90})

	// Subtle per-pixel darkening across the whole sprite for the same
	// pixel-art texture feel as the rat.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*w + x
			if pixels[i].A == 0 {
				continue
			}
			if hashByteXY(x, y)%9 == 0 {
				pixels[i] = adjust(pixels[i], -10)
			}
		}
	}
	return pixels
}

// makeGoblinPixels paints a stocky humanoid goblin: pot-bellied green body,
// loincloth, club gripped in one hand, two pointed ears and yellow eyes.
// Sized for 72×112 in loadEnemyVisuals so the silhouette reads as "taller
// than a rat, shorter than a goblin mage."
func makeGoblinPixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	skin := color.RGBA{R: 108, G: 156, B: 80, A: 255}
	skinDark := color.RGBA{R: 72, G: 116, B: 56, A: 255}
	skinLight := color.RGBA{R: 138, G: 184, B: 102, A: 255}
	cloth := color.RGBA{R: 132, G: 86, B: 58, A: 255}
	clothDark := color.RGBA{R: 92, G: 60, B: 42, A: 255}
	wood := color.RGBA{R: 118, G: 84, B: 52, A: 255}
	woodDark := color.RGBA{R: 80, G: 56, B: 36, A: 255}
	eye := color.RGBA{R: 232, G: 196, B: 80, A: 255}
	pupil := color.RGBA{R: 18, G: 16, B: 12, A: 255}
	tooth := color.RGBA{R: 232, G: 224, B: 200, A: 255}

	cx := w / 2

	// Ground shadow.
	fillEllipsePixels(pixels, w, h, cx, h-6, 22, 4, color.RGBA{R: 0, G: 0, B: 0, A: 80})

	// Legs — two stubby pillars.
	fillRectPixels(pixels, w, h, cx-12, 78, 8, 22, skinDark)
	fillRectPixels(pixels, w, h, cx+4, 78, 8, 22, skinDark)
	// Feet — wide flat oval pads.
	fillEllipsePixels(pixels, w, h, cx-8, 100, 7, 3, clothDark)
	fillEllipsePixels(pixels, w, h, cx+8, 100, 7, 3, clothDark)

	// Loincloth.
	fillRectPixels(pixels, w, h, cx-14, 72, 28, 14, cloth)
	fillTrianglePixels(pixels, w, h, cx-14, 86, cx-4, 86, cx-10, 94, cloth)
	fillTrianglePixels(pixels, w, h, cx+14, 86, cx+4, 86, cx+10, 94, cloth)
	fillRectPixels(pixels, w, h, cx-14, 72, 28, 3, clothDark)

	// Body — pot belly. Bigger lower ellipse + smaller chest above.
	fillEllipsePixels(pixels, w, h, cx, 64, 19, 14, skin)
	fillEllipsePixels(pixels, w, h, cx-4, 60, 14, 8, skinLight)
	fillEllipsePixels(pixels, w, h, cx, 70, 18, 8, skinDark)

	// Arms — left hangs free, right grips the club.
	fillEllipsePixels(pixels, w, h, cx-22, 60, 5, 14, skin)
	fillEllipsePixels(pixels, w, h, cx-22, 74, 4, 4, skin) // hand
	fillEllipsePixels(pixels, w, h, cx+22, 56, 5, 12, skin)
	fillEllipsePixels(pixels, w, h, cx+22, 68, 4, 4, skin) // grip hand

	// Club — diagonal along the right side. Shaft + knob.
	drawLinePixels(pixels, w, h, cx+22, 68, cx+30, 30, wood, 4)
	fillEllipsePixels(pixels, w, h, cx+30, 28, 6, 7, wood)
	fillEllipsePixels(pixels, w, h, cx+28, 26, 2, 2, woodDark) // shading peg
	fillEllipsePixels(pixels, w, h, cx+32, 30, 2, 2, woodDark)

	// Head — wider than tall, blunt jaw.
	fillEllipsePixels(pixels, w, h, cx, 40, 14, 13, skin)
	fillEllipsePixels(pixels, w, h, cx-3, 36, 10, 7, skinLight)
	// Ears — pointed cones jutting outward.
	fillTrianglePixels(pixels, w, h, cx-14, 38, cx-22, 32, cx-12, 42, skin)
	fillTrianglePixels(pixels, w, h, cx+14, 38, cx+22, 32, cx+12, 42, skin)
	fillTrianglePixels(pixels, w, h, cx-14, 38, cx-19, 34, cx-12, 42, skinDark)
	fillTrianglePixels(pixels, w, h, cx+14, 38, cx+19, 34, cx+12, 42, skinDark)
	// Eyes — yellow with dark pupils.
	fillEllipsePixels(pixels, w, h, cx-5, 39, 3, 2, eye)
	fillEllipsePixels(pixels, w, h, cx+5, 39, 3, 2, eye)
	fillEllipsePixels(pixels, w, h, cx-5, 39, 1, 1, pupil)
	fillEllipsePixels(pixels, w, h, cx+5, 39, 1, 1, pupil)
	// Nose — small dark bump.
	fillEllipsePixels(pixels, w, h, cx, 44, 2, 2, skinDark)
	// Mouth with one fang.
	drawLinePixels(pixels, w, h, cx-4, 49, cx+4, 49, skinDark, 1)
	fillTrianglePixels(pixels, w, h, cx-2, 49, cx, 53, cx+1, 49, tooth)

	// Per-pixel texture dither to match the rat/bat surface feel.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*w + x
			if pixels[i].A == 0 {
				continue
			}
			if hashByteXY(x, y)%8 == 0 {
				pixels[i] = adjust(pixels[i], -10)
			}
		}
	}
	return pixels
}

// makeGoblinMagePixels paints a robed goblin caster: hooded purple robe,
// glowing staff, sharper greener face peeking out from the hood. Same
// body class as the goblin but the robe + staff sell "magic user."
func makeGoblinMagePixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	skin := color.RGBA{R: 96, G: 148, B: 76, A: 255}
	skinDark := color.RGBA{R: 60, G: 102, B: 48, A: 255}
	robe := color.RGBA{R: 96, G: 70, B: 138, A: 255}
	robeDark := color.RGBA{R: 60, G: 44, B: 96, A: 255}
	robeLight := color.RGBA{R: 142, G: 112, B: 180, A: 255}
	gold := color.RGBA{R: 226, G: 192, B: 96, A: 255}
	staffWood := color.RGBA{R: 110, G: 78, B: 50, A: 255}
	staffDark := color.RGBA{R: 72, G: 52, B: 36, A: 255}
	gem := color.RGBA{R: 196, G: 136, B: 232, A: 255}
	gemBright := color.RGBA{R: 248, G: 220, B: 252, A: 255}
	eye := color.RGBA{R: 240, G: 232, B: 152, A: 255}
	pupil := color.RGBA{R: 14, G: 14, B: 12, A: 255}

	cx := w / 2

	fillEllipsePixels(pixels, w, h, cx, h-6, 24, 4, color.RGBA{R: 0, G: 0, B: 0, A: 80})

	// Robe — broad triangular skirt from shoulders to feet.
	fillTrianglePixels(pixels, w, h, cx-26, 102, cx+26, 102, cx, 44, robe)
	fillTrianglePixels(pixels, w, h, cx-22, 100, cx+22, 100, cx, 52, robeDark)
	fillRectPixels(pixels, w, h, cx-26, 100, 52, 4, robeDark)
	// Gold trim along the bottom hem.
	fillRectPixels(pixels, w, h, cx-26, 96, 52, 2, gold)

	// Sleeves drooping at sides.
	fillTrianglePixels(pixels, w, h, cx-20, 60, cx-28, 84, cx-10, 80, robe)
	fillTrianglePixels(pixels, w, h, cx+20, 60, cx+28, 84, cx+10, 80, robe)
	fillTrianglePixels(pixels, w, h, cx-20, 62, cx-26, 80, cx-12, 78, robeLight)
	fillTrianglePixels(pixels, w, h, cx+20, 62, cx+26, 80, cx+12, 78, robeLight)
	// Hands poking out of the sleeves.
	fillEllipsePixels(pixels, w, h, cx-22, 84, 4, 4, skin)
	fillEllipsePixels(pixels, w, h, cx+22, 84, 4, 4, skin)

	// Staff — diagonal across the right side, gem floating at the top.
	drawLinePixels(pixels, w, h, cx+22, 84, cx+30, 22, staffWood, 3)
	drawLinePixels(pixels, w, h, cx+22, 84, cx+30, 22, staffDark, 1)
	fillEllipsePixels(pixels, w, h, cx+30, 20, 5, 6, gem)
	fillEllipsePixels(pixels, w, h, cx+30, 19, 2, 2, gemBright)
	// Faint glow ring.
	fillEllipsePixels(pixels, w, h, cx+30, 20, 8, 9, color.RGBA{R: 196, G: 136, B: 232, A: 40})

	// Hood — covers the head, leaving only a small face hole. Dark robe-
	// color outer shell with a lighter inner shadow.
	fillTrianglePixels(pixels, w, h, cx-14, 60, cx+14, 60, cx, 28, robe)
	fillEllipsePixels(pixels, w, h, cx, 46, 14, 12, robeDark)
	// Face peek — small ellipse of skin inside the hood.
	fillEllipsePixels(pixels, w, h, cx, 46, 8, 7, skin)
	fillEllipsePixels(pixels, w, h, cx-2, 44, 5, 4, color.RGBA{R: 128, G: 178, B: 96, A: 255})

	// Glowing yellow eyes deep in the hood.
	fillEllipsePixels(pixels, w, h, cx-3, 46, 2, 2, eye)
	fillEllipsePixels(pixels, w, h, cx+3, 46, 2, 2, eye)
	fillEllipsePixels(pixels, w, h, cx-3, 46, 1, 1, pupil)
	fillEllipsePixels(pixels, w, h, cx+3, 46, 1, 1, pupil)
	// Nose tip + thin mouth.
	fillEllipsePixels(pixels, w, h, cx, 50, 1, 1, skinDark)
	drawLinePixels(pixels, w, h, cx-3, 53, cx+3, 53, skinDark, 1)

	// Hood gold rim.
	drawLinePixels(pixels, w, h, cx-14, 58, cx+14, 58, gold, 1)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*w + x
			if pixels[i].A == 0 {
				continue
			}
			if hashByteXY(x, y)%9 == 0 {
				pixels[i] = adjust(pixels[i], -10)
			}
		}
	}
	return pixels
}

// makeAmoebaPixels paints a squat translucent-looking blob: an outer
// gel-edge halo, a brighter inner core, a darker nucleus, and a few
// floating specks suggesting absorbed mineral grit. Reads as a tank
// (squashed silhouette, dense core) rather than as a jellyfish.
func makeAmoebaPixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	outer := color.RGBA{R: 152, G: 178, B: 200, A: 180}
	mid := color.RGBA{R: 170, G: 196, B: 216, A: 220}
	core := color.RGBA{R: 196, G: 218, B: 232, A: 240}
	nucleus := color.RGBA{R: 86, G: 102, B: 124, A: 240}
	nucleusDark := color.RGBA{R: 54, G: 68, B: 92, A: 240}
	highlight := color.RGBA{R: 240, G: 248, B: 252, A: 230}
	grit := color.RGBA{R: 100, G: 92, B: 82, A: 240}

	cx := w / 2
	cy := h / 2

	// Ground shadow.
	fillEllipsePixels(pixels, w, h, cx, h-8, 28, 5, color.RGBA{R: 0, G: 0, B: 0, A: 95})

	// Gel halo — three nested squashed ellipses.
	fillEllipsePixels(pixels, w, h, cx, cy+4, 36, 22, outer)
	fillEllipsePixels(pixels, w, h, cx-2, cy+3, 30, 18, mid)
	fillEllipsePixels(pixels, w, h, cx-3, cy+2, 22, 14, core)

	// A few amoeba pseudopod bulges — small ellipses pushing out the
	// silhouette.
	fillEllipsePixels(pixels, w, h, cx-30, cy+8, 6, 4, mid)
	fillEllipsePixels(pixels, w, h, cx+28, cy-4, 7, 5, mid)
	fillEllipsePixels(pixels, w, h, cx+12, cy+14, 6, 4, mid)

	// Nucleus — dense darker center with one bright specular dot.
	fillEllipsePixels(pixels, w, h, cx-2, cy+2, 9, 7, nucleus)
	fillEllipsePixels(pixels, w, h, cx-4, cy, 4, 3, nucleusDark)
	fillEllipsePixels(pixels, w, h, cx-6, cy-2, 2, 1, highlight)

	// Floating grit — single-pixel dark specks inside the gel.
	fillRectPixels(pixels, w, h, cx+8, cy-2, 1, 1, grit)
	fillRectPixels(pixels, w, h, cx-12, cy+8, 1, 1, grit)
	fillRectPixels(pixels, w, h, cx+14, cy+8, 1, 1, grit)
	fillRectPixels(pixels, w, h, cx-18, cy-6, 1, 1, grit)
	fillRectPixels(pixels, w, h, cx+20, cy+4, 1, 1, grit)

	// Top-edge specular highlight — a thin curved bright strip so the
	// blob reads as wet.
	for ox := -16; ox <= 16; ox++ {
		x := cx - 2 + ox
		y := cy - 14 + int(math.Abs(float64(ox))/4)
		if x >= 0 && x < w && y >= 0 && y < h {
			pixels[y*w+x] = highlight
		}
	}

	// Subtle dither (slightly less than other sprites — the amoeba reads
	// smoother than a hairy rat).
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*w + x
			if pixels[i].A == 0 {
				continue
			}
			if hashByteXY(x, y)%12 == 0 {
				pixels[i] = adjust(pixels[i], -8)
			}
		}
	}
	return pixels
}

// makeVenusMantrapPixels paints a Venus-flytrap-on-a-stem: a thick green
// stalk rising from a leafy base, two pink toothed jaws flaring open at
// the top, and the dim suggestion of a tongue / interior. The silhouette
// is intentionally top-heavy so the "this thing could eat you" read is
// instant — Ingest is its signature, and the sprite needs to sell it.
func makeVenusMantrapPixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	stem := color.RGBA{R: 86, G: 132, B: 70, A: 255}
	stemDark := color.RGBA{R: 60, G: 96, B: 50, A: 255}
	stemLight := color.RGBA{R: 116, G: 168, B: 90, A: 255}
	leaf := color.RGBA{R: 70, G: 116, B: 60, A: 255}
	leafDark := color.RGBA{R: 48, G: 86, B: 44, A: 255}
	jawOuter := color.RGBA{R: 112, G: 168, B: 92, A: 255}
	jawInner := color.RGBA{R: 196, G: 90, B: 124, A: 255}
	jawDeep := color.RGBA{R: 132, G: 44, B: 70, A: 255}
	maw := color.RGBA{R: 60, G: 18, B: 30, A: 255}
	tooth := color.RGBA{R: 252, G: 246, B: 230, A: 255}
	eye := color.RGBA{R: 252, G: 220, B: 88, A: 255}
	pupil := color.RGBA{R: 14, G: 12, B: 10, A: 255}

	cx := w / 2

	// Ground shadow.
	fillEllipsePixels(pixels, w, h, cx, h-6, 26, 4, color.RGBA{R: 0, G: 0, B: 0, A: 90})

	// Leafy base — three overlapping flat ellipses fanning out at floor level.
	fillEllipsePixels(pixels, w, h, cx, h-12, 26, 9, leafDark)
	fillEllipsePixels(pixels, w, h, cx-12, h-14, 14, 7, leaf)
	fillEllipsePixels(pixels, w, h, cx+12, h-14, 14, 7, leaf)
	fillEllipsePixels(pixels, w, h, cx, h-18, 10, 5, stemLight)

	// Stem — thick vertical stalk with a small bend.
	drawLinePixels(pixels, w, h, cx, h-20, cx-4, 60, stem, 9)
	drawLinePixels(pixels, w, h, cx, h-20, cx-4, 60, stemDark, 3)
	// Two small leaf nodes coming off the stem.
	fillEllipsePixels(pixels, w, h, cx-10, 80, 6, 3, leaf)
	fillEllipsePixels(pixels, w, h, cx+8, 70, 5, 3, leaf)
	drawLinePixels(pixels, w, h, cx-4, 82, cx-14, 80, stem, 2)
	drawLinePixels(pixels, w, h, cx-2, 72, cx+10, 70, stem, 2)

	// Bulbous trap base — the green cup the jaws hinge from.
	fillEllipsePixels(pixels, w, h, cx-4, 56, 12, 7, jawOuter)
	fillEllipsePixels(pixels, w, h, cx-4, 58, 10, 5, stemDark)

	// Upper jaw — broad open clamshell pointing up-left, hinged at the cup.
	upperApex := [2]int{cx - 20, 26}
	upperHinge := [2]int{cx - 4, 52}
	upperFront := [2]int{cx + 4, 38}
	fillTrianglePixels(pixels, w, h, upperApex[0], upperApex[1], upperHinge[0], upperHinge[1], upperFront[0], upperFront[1], jawOuter)
	// Inner pink lining.
	fillTrianglePixels(pixels, w, h, upperApex[0]+3, upperApex[1]+5, upperHinge[0]+1, upperHinge[1]-2, upperFront[0]-2, upperFront[1]-2, jawInner)
	// Deep red mouth shadow.
	fillTrianglePixels(pixels, w, h, upperApex[0]+8, upperApex[1]+10, upperHinge[0]+3, upperHinge[1]-4, upperFront[0]-4, upperFront[1]-4, jawDeep)

	// Lower jaw — mirror, opening down-right.
	lowerApex := [2]int{cx + 22, 30}
	lowerHinge := [2]int{cx - 2, 52}
	lowerFront := [2]int{cx - 6, 42}
	fillTrianglePixels(pixels, w, h, lowerApex[0], lowerApex[1], lowerHinge[0], lowerHinge[1], lowerFront[0], lowerFront[1], jawOuter)
	fillTrianglePixels(pixels, w, h, lowerApex[0]-3, lowerApex[1]+4, lowerHinge[0]-1, lowerHinge[1]-2, lowerFront[0]+2, lowerFront[1]-2, jawInner)

	// Throat / maw — dark hole behind the jaws.
	fillEllipsePixels(pixels, w, h, cx-2, 44, 6, 7, maw)

	// Teeth — small white triangles along the outer edges of both jaws.
	for i := 0; i < 5; i++ {
		t := float64(i+1) / 6.0
		ux := upperApex[0] + int(float64(upperFront[0]-upperApex[0])*t)
		uy := upperApex[1] + int(float64(upperFront[1]-upperApex[1])*t)
		fillTrianglePixels(pixels, w, h, ux, uy, ux+2, uy+4, ux-2, uy+4, tooth)
		lx := lowerApex[0] + int(float64(lowerFront[0]-lowerApex[0])*t)
		ly := lowerApex[1] + int(float64(lowerFront[1]-lowerApex[1])*t)
		fillTrianglePixels(pixels, w, h, lx, ly, lx-2, ly-4, lx+2, ly-4, tooth)
	}

	// One predatory yellow eye nestled at the back of the maw.
	fillEllipsePixels(pixels, w, h, cx-2, 46, 3, 3, eye)
	fillEllipsePixels(pixels, w, h, cx-2, 46, 1, 1, pupil)

	// Per-pixel dither to match the rest of the bestiary.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*w + x
			if pixels[i].A == 0 {
				continue
			}
			if hashByteXY(x, y)%9 == 0 {
				pixels[i] = adjust(pixels[i], -10)
			}
		}
	}
	return pixels
}

// makeCaveSpiderPixels paints the cave spider: a low-slung
// arachnid with a bulbous purple abdomen, a smaller cephalothorax up
// front, eight legs splayed out in jointed pairs, six red eyes
// clustered between the mandibles, and downward-pointing fangs.
// Sized wide and short (88×72) so the silhouette reads "thing on the
// ground" instead of "tall humanoid." Designed as a tier-3 ambusher
// — menacing eye cluster + visible fangs > pure cute round body.
func makeCaveSpiderPixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	bodyDark := color.RGBA{R: 44, G: 28, B: 60, A: 255}
	body := color.RGBA{R: 76, G: 50, B: 102, A: 255}
	bodyLight := color.RGBA{R: 116, G: 80, B: 148, A: 255}
	legDark := color.RGBA{R: 30, G: 18, B: 42, A: 255}
	leg := color.RGBA{R: 60, G: 42, B: 86, A: 255}
	fang := color.RGBA{R: 240, G: 226, B: 200, A: 255}
	eye := color.RGBA{R: 232, G: 48, B: 56, A: 255}
	eyeGlow := color.RGBA{R: 255, G: 128, B: 132, A: 255}

	cx := w / 2

	// Ground shadow — wider than tall, fits the squat silhouette.
	fillEllipsePixels(pixels, w, h, cx, h-4, 30, 4, color.RGBA{R: 0, G: 0, B: 0, A: 110})

	// Eight legs in four jointed pairs, fanning out from the
	// cephalothorax. Each leg is a two-segment poly-line: shoulder
	// up + out, then knee down to a foot near the floor. Outer
	// legs reach further than inner ones so the silhouette feels
	// spread, not stacked.
	legY := h - 18 // shoulder anchor for all legs
	footY := h - 8
	// Back-left pair.
	drawLinePixels(pixels, w, h, cx-8, legY, cx-26, 24, legDark, 3)
	drawLinePixels(pixels, w, h, cx-26, 24, cx-34, footY, legDark, 3)
	drawLinePixels(pixels, w, h, cx-8, legY, cx-26, 24, leg, 1)
	drawLinePixels(pixels, w, h, cx-6, legY+2, cx-20, 32, legDark, 3)
	drawLinePixels(pixels, w, h, cx-20, 32, cx-26, footY, legDark, 3)
	drawLinePixels(pixels, w, h, cx-6, legY+2, cx-20, 32, leg, 1)
	// Front-left pair.
	drawLinePixels(pixels, w, h, cx-4, legY+4, cx-16, 38, legDark, 3)
	drawLinePixels(pixels, w, h, cx-16, 38, cx-20, footY, legDark, 3)
	drawLinePixels(pixels, w, h, cx-2, legY+6, cx-12, 44, legDark, 3)
	drawLinePixels(pixels, w, h, cx-12, 44, cx-14, footY, legDark, 3)
	// Back-right pair (mirror).
	drawLinePixels(pixels, w, h, cx+8, legY, cx+26, 24, legDark, 3)
	drawLinePixels(pixels, w, h, cx+26, 24, cx+34, footY, legDark, 3)
	drawLinePixels(pixels, w, h, cx+8, legY, cx+26, 24, leg, 1)
	drawLinePixels(pixels, w, h, cx+6, legY+2, cx+20, 32, legDark, 3)
	drawLinePixels(pixels, w, h, cx+20, 32, cx+26, footY, legDark, 3)
	drawLinePixels(pixels, w, h, cx+6, legY+2, cx+20, 32, leg, 1)
	// Front-right pair.
	drawLinePixels(pixels, w, h, cx+4, legY+4, cx+16, 38, legDark, 3)
	drawLinePixels(pixels, w, h, cx+16, 38, cx+20, footY, legDark, 3)
	drawLinePixels(pixels, w, h, cx+2, legY+6, cx+12, 44, legDark, 3)
	drawLinePixels(pixels, w, h, cx+12, 44, cx+14, footY, legDark, 3)

	// Abdomen — big bulbous oval at the back (lower / further from
	// viewer). Two-tone shading so the bulb reads as 3D.
	fillEllipsePixels(pixels, w, h, cx, h-22, 24, 16, bodyDark)
	fillEllipsePixels(pixels, w, h, cx, h-24, 22, 14, body)
	fillEllipsePixels(pixels, w, h, cx-4, h-28, 12, 6, bodyLight)
	// Faint marking on the abdomen — a pale crescent.
	fillEllipsePixels(pixels, w, h, cx+2, h-22, 8, 3, bodyLight)

	// Cephalothorax — smaller round front body (upper / closer).
	fillEllipsePixels(pixels, w, h, cx, h-38, 14, 10, bodyDark)
	fillEllipsePixels(pixels, w, h, cx, h-40, 12, 8, body)
	fillEllipsePixels(pixels, w, h, cx-2, h-42, 8, 4, bodyLight)

	// Six red eyes — two rows of three (3+3 pattern). Outer eyes
	// slightly smaller for depth. Glow halo behind each.
	for _, e := range [][3]int{
		{cx - 6, h - 41, 2}, {cx, h - 41, 2}, {cx + 6, h - 41, 2},
		{cx - 4, h - 37, 1}, {cx, h - 37, 1}, {cx + 4, h - 37, 1},
	} {
		fillEllipsePixels(pixels, w, h, e[0], e[1], e[2]+1, e[2]+1, eyeGlow)
		fillEllipsePixels(pixels, w, h, e[0], e[1], e[2], e[2], eye)
	}

	// Mandibles / fangs — two small triangles hanging from the
	// cephalothorax's front edge.
	fillTrianglePixels(pixels, w, h, cx-5, h-32, cx-2, h-32, cx-3, h-28, fang)
	fillTrianglePixels(pixels, w, h, cx+5, h-32, cx+2, h-32, cx+3, h-28, fang)

	// Per-pixel dither — match the bestiary's surface feel.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*w + x
			if pixels[i].A == 0 {
				continue
			}
			if hashByteXY(x, y)%9 == 0 {
				pixels[i] = adjust(pixels[i], -12)
			}
		}
	}
	return pixels
}

// makeVampireBatPixels paints the vampire bat: larger than the cave
// bat, deeper crimson-black wings, glowing red eyes, prominent
// fangs, and a small blood-drip at the mouth that sells the
// lifesteal identity. Wing silhouette mirrors the cave bat so the
// player recognizes the upgrade at a glance — same silhouette
// family, more menacing color story.
func makeVampireBatPixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	body := color.RGBA{R: 52, G: 20, B: 28, A: 255}
	bodyDark := color.RGBA{R: 28, G: 8, B: 16, A: 255}
	bodyLight := color.RGBA{R: 96, G: 36, B: 50, A: 255}
	wingMembrane := color.RGBA{R: 76, G: 24, B: 36, A: 255}
	wingDark := color.RGBA{R: 36, G: 12, B: 20, A: 255}
	wingBone := color.RGBA{R: 24, G: 8, B: 14, A: 255}
	eye := color.RGBA{R: 248, G: 36, B: 48, A: 255}
	eyeGlow := color.RGBA{R: 255, G: 140, B: 140, A: 255}
	fang := color.RGBA{R: 240, G: 226, B: 200, A: 255}
	blood := color.RGBA{R: 196, G: 32, B: 44, A: 255}

	cx := w / 2

	// Ground shadow — wider for the wingspan.
	fillEllipsePixels(pixels, w, h, cx, h-4, 34, 4, color.RGBA{R: 0, G: 0, B: 0, A: 100})

	// Wing membranes — broad triangles fanning out from the body.
	// Left wing.
	fillTrianglePixels(pixels, w, h, cx-2, 36, cx-44, 28, cx-30, 60, wingMembrane)
	fillTrianglePixels(pixels, w, h, cx-2, 36, cx-30, 60, cx-12, 56, wingMembrane)
	// Inner-membrane shading — darker patch nearer to the body.
	fillTrianglePixels(pixels, w, h, cx-2, 40, cx-26, 44, cx-12, 56, wingDark)
	// Wing-finger bones — three diverging dark lines per wing.
	drawLinePixels(pixels, w, h, cx-4, 38, cx-44, 28, wingBone, 2)
	drawLinePixels(pixels, w, h, cx-4, 38, cx-36, 50, wingBone, 2)
	drawLinePixels(pixels, w, h, cx-4, 38, cx-26, 58, wingBone, 2)
	// Right wing (mirror).
	fillTrianglePixels(pixels, w, h, cx+2, 36, cx+44, 28, cx+30, 60, wingMembrane)
	fillTrianglePixels(pixels, w, h, cx+2, 36, cx+30, 60, cx+12, 56, wingMembrane)
	fillTrianglePixels(pixels, w, h, cx+2, 40, cx+26, 44, cx+12, 56, wingDark)
	drawLinePixels(pixels, w, h, cx+4, 38, cx+44, 28, wingBone, 2)
	drawLinePixels(pixels, w, h, cx+4, 38, cx+36, 50, wingBone, 2)
	drawLinePixels(pixels, w, h, cx+4, 38, cx+26, 58, wingBone, 2)

	// Tiny claws at the wingtips.
	fillTrianglePixels(pixels, w, h, cx-44, 26, cx-46, 22, cx-40, 28, wingBone)
	fillTrianglePixels(pixels, w, h, cx+44, 26, cx+46, 22, cx+40, 28, wingBone)

	// Body — fuzzy oval torso, darker at the bottom.
	fillEllipsePixels(pixels, w, h, cx, 46, 12, 14, bodyDark)
	fillEllipsePixels(pixels, w, h, cx, 44, 10, 12, body)
	fillEllipsePixels(pixels, w, h, cx-2, 40, 7, 6, bodyLight)

	// Feet — tiny claws hanging beneath the body.
	fillTrianglePixels(pixels, w, h, cx-6, 60, cx-3, 64, cx-3, 60, wingBone)
	fillTrianglePixels(pixels, w, h, cx+6, 60, cx+3, 64, cx+3, 60, wingBone)

	// Head — round, slightly tilted forward.
	fillEllipsePixels(pixels, w, h, cx, 32, 11, 10, bodyDark)
	fillEllipsePixels(pixels, w, h, cx, 30, 9, 8, body)

	// Ears — two pointed cones jutting up.
	fillTrianglePixels(pixels, w, h, cx-7, 24, cx-3, 16, cx-2, 26, bodyDark)
	fillTrianglePixels(pixels, w, h, cx+7, 24, cx+3, 16, cx+2, 26, bodyDark)
	fillTrianglePixels(pixels, w, h, cx-6, 24, cx-3, 18, cx-3, 26, body)
	fillTrianglePixels(pixels, w, h, cx+6, 24, cx+3, 18, cx+3, 26, body)

	// Eyes — BIG glowing red, the headline detail.
	fillEllipsePixels(pixels, w, h, cx-4, 30, 3, 3, eyeGlow)
	fillEllipsePixels(pixels, w, h, cx+4, 30, 3, 3, eyeGlow)
	fillEllipsePixels(pixels, w, h, cx-4, 30, 2, 2, eye)
	fillEllipsePixels(pixels, w, h, cx+4, 30, 2, 2, eye)

	// Fangs — two pointed white teeth below the mouth.
	fillTrianglePixels(pixels, w, h, cx-3, 36, cx-1, 36, cx-2, 41, fang)
	fillTrianglePixels(pixels, w, h, cx+3, 36, cx+1, 36, cx+2, 41, fang)

	// Blood drip — a small bead trailing from one fang. Pure ID
	// flavor; reads at any zoom because of the saturated red.
	fillEllipsePixels(pixels, w, h, cx-2, 44, 1, 2, blood)

	// Per-pixel dither.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*w + x
			if pixels[i].A == 0 {
				continue
			}
			if hashByteXY(x, y)%9 == 0 {
				pixels[i] = adjust(pixels[i], -10)
			}
		}
	}
	return pixels
}

// makeWispPixels paints the will-o'-wisp: a floating ghostly orb of
// cold cyan-white light with concentric halo rings dimming outward,
// trailing wispy tendrils below. No solid body — the sprite is all
// glow + atmosphere. Sized narrow + tall (56×72) so it reads as
// "drifting light" rather than "creature with a body."
func makeWispPixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	core1 := color.RGBA{R: 246, G: 252, B: 255, A: 255} // brightest center
	core2 := color.RGBA{R: 188, G: 226, B: 252, A: 255} // inner halo
	core3 := color.RGBA{R: 124, G: 184, B: 232, A: 255} // mid halo
	mist1 := color.RGBA{R: 88, G: 144, B: 208, A: 220}  // outer mist
	mist2 := color.RGBA{R: 56, G: 108, B: 176, A: 160}  // wispy edge
	tendril := color.RGBA{R: 96, G: 156, B: 220, A: 200}
	tendrilDim := color.RGBA{R: 60, G: 116, B: 180, A: 140}
	eye := color.RGBA{R: 14, G: 22, B: 40, A: 255}

	cx := w / 2
	cy := h/2 - 8 // float anchor above the center for the orb body

	// Soft ground shadow — small + diffuse since the wisp floats.
	fillEllipsePixels(pixels, w, h, cx, h-4, 14, 3, color.RGBA{R: 0, G: 0, B: 0, A: 60})

	// Outer mist clouds — irregular tendrils trailing downward
	// like a ghostly flame. Three drifting blobs at different y
	// offsets so the silhouette looks alive rather than symmetric.
	fillEllipsePixels(pixels, w, h, cx-3, cy+14, 7, 9, mist2)
	fillEllipsePixels(pixels, w, h, cx+4, cy+18, 5, 7, mist2)
	fillEllipsePixels(pixels, w, h, cx, cy+22, 4, 6, tendrilDim)
	fillEllipsePixels(pixels, w, h, cx-2, cy+28, 3, 5, tendrilDim)

	// Halo layers — concentric rings around the core, each
	// progressively brighter and smaller. Painted big-to-small so
	// the inner layers overwrite the outer ones.
	fillEllipsePixels(pixels, w, h, cx, cy, 16, 18, mist1)
	fillEllipsePixels(pixels, w, h, cx, cy, 13, 14, mist2)
	fillEllipsePixels(pixels, w, h, cx, cy, 10, 11, core3)
	fillEllipsePixels(pixels, w, h, cx, cy, 7, 8, core2)
	fillEllipsePixels(pixels, w, h, cx, cy-1, 4, 5, core1)

	// Two tiny dark "eye" pinpricks inside the bright core so the
	// wisp reads as faintly malevolent rather than a benign light.
	fillEllipsePixels(pixels, w, h, cx-2, cy-1, 1, 1, eye)
	fillEllipsePixels(pixels, w, h, cx+2, cy-1, 1, 1, eye)

	// Side-trailing wisps — two small curling arcs out the sides.
	drawLinePixels(pixels, w, h, cx-10, cy+4, cx-16, cy+12, tendril, 2)
	drawLinePixels(pixels, w, h, cx+10, cy+4, cx+16, cy+12, tendril, 2)
	drawLinePixels(pixels, w, h, cx-16, cy+12, cx-14, cy+18, tendrilDim, 2)
	drawLinePixels(pixels, w, h, cx+16, cy+12, cx+14, cy+18, tendrilDim, 2)

	// No dither pass — the wisp's silhouette is intentionally
	// smooth gradient. A dither here would make the glow look
	// noisy instead of ethereal.
	return pixels
}

// makeStoneGolemPixels paints the stone golem: a blocky humanoid
// hewn from weathered stone, with a horizontal glowing eye slit,
// broad square shoulders, heavy arms hanging at its sides, and
// cracked-stone detailing throughout. Sized big (96×120) — the
// golem is the biggest silhouette in the bestiary so it visually
// anchors a pack. Designed for the "active armor wall" identity:
// blocky enough to read as a stone construct, glowing eye sells
// "animated" rather than statuary.
func makeStoneGolemPixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	stone := color.RGBA{R: 132, G: 124, B: 110, A: 255}
	stoneDark := color.RGBA{R: 88, G: 82, B: 72, A: 255}
	stoneShadow := color.RGBA{R: 60, G: 56, B: 50, A: 255}
	stoneLight := color.RGBA{R: 172, G: 162, B: 144, A: 255}
	moss := color.RGBA{R: 92, G: 124, B: 68, A: 255}
	crack := color.RGBA{R: 32, G: 28, B: 24, A: 255}
	eyeGlow := color.RGBA{R: 248, G: 216, B: 112, A: 255}
	eyeBright := color.RGBA{R: 252, G: 244, B: 200, A: 255}

	cx := w / 2

	// Ground shadow — wide for the bulk.
	fillEllipsePixels(pixels, w, h, cx, h-4, 36, 5, color.RGBA{R: 0, G: 0, B: 0, A: 120})

	// Legs — two thick stone pillars. Slightly tapered at the
	// ankle so the silhouette doesn't read as a perfect cube.
	fillRectPixels(pixels, w, h, cx-22, 86, 16, 28, stoneDark)
	fillRectPixels(pixels, w, h, cx+6, 86, 16, 28, stoneDark)
	fillRectPixels(pixels, w, h, cx-22, 86, 16, 22, stone)
	fillRectPixels(pixels, w, h, cx+6, 86, 16, 22, stone)
	// Leg highlights — left edge of each pillar catches light.
	fillRectPixels(pixels, w, h, cx-22, 86, 3, 22, stoneLight)
	fillRectPixels(pixels, w, h, cx+6, 86, 3, 22, stoneLight)
	// Feet — wide flat slabs.
	fillRectPixels(pixels, w, h, cx-26, 110, 22, 6, stoneShadow)
	fillRectPixels(pixels, w, h, cx+4, 110, 22, 6, stoneShadow)

	// Hip / waist block — slightly narrower than the torso.
	fillRectPixels(pixels, w, h, cx-22, 76, 44, 14, stoneDark)
	fillRectPixels(pixels, w, h, cx-22, 76, 44, 10, stone)
	fillRectPixels(pixels, w, h, cx-22, 76, 6, 10, stoneLight)

	// Torso — broad rectangular slab.
	fillRectPixels(pixels, w, h, cx-26, 42, 52, 36, stoneDark)
	fillRectPixels(pixels, w, h, cx-26, 42, 52, 30, stone)
	fillRectPixels(pixels, w, h, cx-26, 42, 8, 30, stoneLight)
	// Chest cracks — three jagged dark lines telegraphing damage.
	drawLinePixels(pixels, w, h, cx-12, 48, cx-4, 60, crack, 1)
	drawLinePixels(pixels, w, h, cx-4, 60, cx+2, 70, crack, 1)
	drawLinePixels(pixels, w, h, cx+8, 50, cx+14, 64, crack, 1)
	drawLinePixels(pixels, w, h, cx-18, 64, cx-10, 74, crack, 1)

	// Arms — big square blocks hanging at the sides, slightly
	// raised at the shoulder to suggest a fighter's stance.
	// Left arm.
	fillRectPixels(pixels, w, h, cx-40, 42, 14, 38, stoneDark)
	fillRectPixels(pixels, w, h, cx-40, 42, 14, 32, stone)
	fillRectPixels(pixels, w, h, cx-40, 42, 4, 32, stoneLight)
	// Left fist — bigger block at the end.
	fillRectPixels(pixels, w, h, cx-42, 78, 18, 12, stoneDark)
	fillRectPixels(pixels, w, h, cx-42, 78, 18, 10, stone)
	// Right arm.
	fillRectPixels(pixels, w, h, cx+26, 42, 14, 38, stoneDark)
	fillRectPixels(pixels, w, h, cx+26, 42, 14, 32, stone)
	fillRectPixels(pixels, w, h, cx+26, 42, 4, 32, stoneLight)
	// Right fist.
	fillRectPixels(pixels, w, h, cx+24, 78, 18, 12, stoneDark)
	fillRectPixels(pixels, w, h, cx+24, 78, 18, 10, stone)

	// Head — square block at the top, slightly recessed shoulders.
	fillRectPixels(pixels, w, h, cx-16, 14, 32, 28, stoneDark)
	fillRectPixels(pixels, w, h, cx-16, 14, 32, 24, stone)
	fillRectPixels(pixels, w, h, cx-16, 14, 5, 24, stoneLight)
	// Head crack — single horizontal damage line.
	drawLinePixels(pixels, w, h, cx-10, 22, cx+4, 26, crack, 1)
	// Glowing eye slit — horizontal bright bar across the head.
	fillRectPixels(pixels, w, h, cx-12, 26, 24, 4, crack)
	fillRectPixels(pixels, w, h, cx-10, 27, 20, 2, eyeGlow)
	fillRectPixels(pixels, w, h, cx-4, 27, 8, 2, eyeBright)

	// Moss patches — small green specks on the shoulders and feet
	// to sell "ancient" rather than "freshly carved."
	fillEllipsePixels(pixels, w, h, cx-22, 46, 4, 2, moss)
	fillEllipsePixels(pixels, w, h, cx+22, 46, 4, 2, moss)
	fillEllipsePixels(pixels, w, h, cx-18, 112, 4, 1, moss)
	fillEllipsePixels(pixels, w, h, cx+18, 112, 4, 1, moss)

	// Heavy stone-grain dither — denser than the soft-creature
	// sprites so the surface reads as actual rock, not skin.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*w + x
			if pixels[i].A == 0 {
				continue
			}
			if hashByteXY(x, y)%5 == 0 {
				pixels[i] = adjust(pixels[i], -14)
			}
		}
	}
	return pixels
}

// makeNecromancerPixels paints the necromancer: a tall hooded
// figure in deep indigo robes, pale skull face peeking out from
// the shadow of the hood with glowing green sockets, a bone staff
// topped with a small skull held to one side, and bony fingers
// gripping the shaft. Sized tall + narrow (72×112) so the
// silhouette reads as a robed humanoid distinct from the goblin
// mage's stout pose.
func makeNecromancerPixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	robe := color.RGBA{R: 38, G: 36, B: 68, A: 255}
	robeDark := color.RGBA{R: 20, G: 20, B: 44, A: 255}
	robeLight := color.RGBA{R: 64, G: 60, B: 100, A: 255}
	hoodShadow := color.RGBA{R: 8, G: 8, B: 18, A: 255}
	skull := color.RGBA{R: 224, G: 218, B: 196, A: 255}
	skullDark := color.RGBA{R: 168, G: 160, B: 132, A: 255}
	eyeSocket := color.RGBA{R: 12, G: 20, B: 14, A: 255}
	eyeGlow := color.RGBA{R: 124, G: 240, B: 132, A: 255}
	eyeBright := color.RGBA{R: 220, G: 252, B: 192, A: 255}
	staff := color.RGBA{R: 196, G: 184, B: 152, A: 255}
	staffDark := color.RGBA{R: 132, G: 122, B: 96, A: 255}
	bone := color.RGBA{R: 218, G: 210, B: 184, A: 255}
	trim := color.RGBA{R: 132, G: 96, B: 168, A: 255} // purple hem

	cx := w / 2

	// Ground shadow — long and thin under a robed figure.
	fillEllipsePixels(pixels, w, h, cx, h-4, 22, 4, color.RGBA{R: 0, G: 0, B: 0, A: 110})

	// Robe skirt — wide triangle from shoulders to feet, dark
	// underneath, lighter on the left edge to suggest folds.
	fillTrianglePixels(pixels, w, h, cx-26, h-6, cx+26, h-6, cx, 34, robeDark)
	fillTrianglePixels(pixels, w, h, cx-22, h-8, cx+22, h-8, cx, 38, robe)
	// Left fold highlight.
	fillTrianglePixels(pixels, w, h, cx-22, h-8, cx-14, h-8, cx-6, 40, robeLight)
	// Hem — purple trim along the bottom edge of the robe.
	fillRectPixels(pixels, w, h, cx-26, h-10, 52, 3, trim)

	// Torso — narrower above the skirt where the body sits.
	fillRectPixels(pixels, w, h, cx-14, 50, 28, 24, robe)
	fillRectPixels(pixels, w, h, cx-14, 50, 28, 4, robeDark)
	// Robe sash crossing the chest.
	drawLinePixels(pixels, w, h, cx-14, 58, cx+14, 64, trim, 2)

	// Sleeves — flaring out at the shoulders, narrowing toward
	// the wrists. Left arm visible holding the staff.
	// Left sleeve.
	fillTrianglePixels(pixels, w, h, cx-14, 50, cx-22, 72, cx-12, 76, robe)
	fillTrianglePixels(pixels, w, h, cx-14, 50, cx-22, 72, cx-18, 60, robeDark)
	// Right sleeve.
	fillTrianglePixels(pixels, w, h, cx+14, 50, cx+22, 72, cx+12, 76, robe)
	fillTrianglePixels(pixels, w, h, cx+14, 50, cx+18, 60, cx+22, 72, robeLight)

	// Hood — dark cone framing the face. Wide at the shoulders,
	// peaked above the head.
	fillTrianglePixels(pixels, w, h, cx-18, 48, cx+18, 48, cx, 10, robeDark)
	fillTrianglePixels(pixels, w, h, cx-14, 46, cx+14, 46, cx, 14, robe)
	// Inner hood shadow — deep black wells the face peeks out of.
	fillEllipsePixels(pixels, w, h, cx, 32, 10, 13, hoodShadow)

	// Skull face — bone-pale oval inside the hood shadow.
	fillEllipsePixels(pixels, w, h, cx, 30, 8, 10, skull)
	fillEllipsePixels(pixels, w, h, cx-1, 28, 6, 6, skullDark)
	// Eye sockets — two black hollows with glowing green pupils.
	fillEllipsePixels(pixels, w, h, cx-3, 30, 2, 3, eyeSocket)
	fillEllipsePixels(pixels, w, h, cx+3, 30, 2, 3, eyeSocket)
	fillEllipsePixels(pixels, w, h, cx-3, 30, 1, 1, eyeBright)
	fillEllipsePixels(pixels, w, h, cx+3, 30, 1, 1, eyeBright)
	// Outer eye glow — a faint green halo bleeds through the
	// hood shadow.
	fillEllipsePixels(pixels, w, h, cx-3, 30, 3, 3, color.RGBA{R: eyeGlow.R, G: eyeGlow.G, B: eyeGlow.B, A: 90})
	fillEllipsePixels(pixels, w, h, cx+3, 30, 3, 3, color.RGBA{R: eyeGlow.R, G: eyeGlow.G, B: eyeGlow.B, A: 90})
	// Nasal cavity — small triangle below the eyes.
	fillTrianglePixels(pixels, w, h, cx-1, 34, cx+1, 34, cx, 37, eyeSocket)
	// Tooth line — five small dark notches at the jaw.
	for i := 0; i < 5; i++ {
		px := cx - 4 + i*2
		fillRectPixels(pixels, w, h, px, 38, 1, 2, eyeSocket)
	}

	// Staff — diagonal shaft from the left hand reaching above
	// the head. Bone-colored shaft, skull topper.
	drawLinePixels(pixels, w, h, cx-22, 78, cx-32, 16, staffDark, 4)
	drawLinePixels(pixels, w, h, cx-22, 78, cx-32, 16, staff, 2)
	// Skull topper.
	fillEllipsePixels(pixels, w, h, cx-32, 14, 5, 6, bone)
	fillEllipsePixels(pixels, w, h, cx-33, 14, 2, 2, eyeSocket)
	fillEllipsePixels(pixels, w, h, cx-31, 14, 2, 2, eyeSocket)
	// Bony hand gripping the shaft.
	fillEllipsePixels(pixels, w, h, cx-22, 76, 4, 4, skull)
	fillEllipsePixels(pixels, w, h, cx-22, 76, 3, 3, skullDark)

	// Per-pixel dither.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*w + x
			if pixels[i].A == 0 {
				continue
			}
			if hashByteXY(x, y)%9 == 0 {
				pixels[i] = adjust(pixels[i], -12)
			}
		}
	}
	return pixels
}

// makeSkeletonPixels paints the skeleton grunt: a stripped-down
// humanoid frame with a skull head (hollow eye sockets glowing
// dim red), visible ribcage, bony arms with claw-fingers, and
// femur/tibia legs. Sized as a regular humanoid (72×112) — same
// proportions as the goblin so packs read as a mixed front line.
// Designed for the "expendable raised summon" identity: clearly
// undead, clearly cheap, paired naturally with the Necromancer.
func makeSkeletonPixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	bone := color.RGBA{R: 224, G: 218, B: 196, A: 255}
	boneDark := color.RGBA{R: 168, G: 160, B: 132, A: 255}
	boneLight := color.RGBA{R: 244, G: 240, B: 216, A: 255}
	socket := color.RGBA{R: 16, G: 10, B: 12, A: 255}
	eyeGlow := color.RGBA{R: 232, G: 96, B: 88, A: 255}
	eyeBright := color.RGBA{R: 255, G: 196, B: 180, A: 255}
	rust := color.RGBA{R: 108, G: 60, B: 36, A: 255}
	rustDark := color.RGBA{R: 72, G: 38, B: 22, A: 255}

	cx := w / 2

	// Ground shadow.
	fillEllipsePixels(pixels, w, h, cx, h-4, 18, 3, color.RGBA{R: 0, G: 0, B: 0, A: 110})

	// Femurs.
	drawLinePixels(pixels, w, h, cx-7, 76, cx-10, h-12, boneDark, 4)
	drawLinePixels(pixels, w, h, cx+7, 76, cx+10, h-12, boneDark, 4)
	drawLinePixels(pixels, w, h, cx-7, 76, cx-10, h-12, bone, 2)
	drawLinePixels(pixels, w, h, cx+7, 76, cx+10, h-12, bone, 2)
	// Knee joints — small dark bumps.
	fillEllipsePixels(pixels, w, h, cx-9, 92, 3, 2, boneDark)
	fillEllipsePixels(pixels, w, h, cx+9, 92, 3, 2, boneDark)
	// Tibias.
	drawLinePixels(pixels, w, h, cx-10, 92, cx-12, h-8, boneDark, 3)
	drawLinePixels(pixels, w, h, cx+10, 92, cx+12, h-8, boneDark, 3)
	drawLinePixels(pixels, w, h, cx-10, 92, cx-12, h-8, bone, 1)
	drawLinePixels(pixels, w, h, cx+10, 92, cx+12, h-8, bone, 1)
	// Feet — wide flat ovals.
	fillEllipsePixels(pixels, w, h, cx-13, h-6, 5, 2, boneDark)
	fillEllipsePixels(pixels, w, h, cx+13, h-6, 5, 2, boneDark)

	// Pelvis — flared boomerang shape with central socket.
	fillTrianglePixels(pixels, w, h, cx-12, 70, cx+12, 70, cx, 80, boneDark)
	fillTrianglePixels(pixels, w, h, cx-10, 70, cx+10, 70, cx, 77, bone)
	fillEllipsePixels(pixels, w, h, cx, 75, 3, 3, socket)

	// Spine — vertical chain of small dark bone segments.
	for sy := 44; sy < 70; sy += 4 {
		fillEllipsePixels(pixels, w, h, cx, sy, 2, 2, boneDark)
		fillEllipsePixels(pixels, w, h, cx, sy-1, 1, 1, bone)
	}

	// Rib cage — five curved ribs per side, arching from the
	// spine. Drawn as small ellipses connected by darker shadow.
	for i, sy := range []int{46, 50, 54, 58, 62} {
		span := 12 - i // outer ribs reach further than inner
		fillEllipsePixels(pixels, w, h, cx-span/2, sy, span/2+1, 2, boneDark)
		fillEllipsePixels(pixels, w, h, cx+span/2, sy, span/2+1, 2, boneDark)
		fillEllipsePixels(pixels, w, h, cx-span/2, sy, span/2, 1, bone)
		fillEllipsePixels(pixels, w, h, cx+span/2, sy, span/2, 1, bone)
	}
	// Sternum — vertical bone strip along the centerline.
	fillRectPixels(pixels, w, h, cx-1, 46, 2, 16, boneLight)

	// Clavicle / shoulder bones — horizontal bar above the ribs.
	drawLinePixels(pixels, w, h, cx-14, 42, cx+14, 42, boneDark, 2)
	drawLinePixels(pixels, w, h, cx-14, 42, cx+14, 42, bone, 1)
	// Shoulder joints.
	fillEllipsePixels(pixels, w, h, cx-14, 42, 3, 3, boneDark)
	fillEllipsePixels(pixels, w, h, cx+14, 42, 3, 3, boneDark)

	// Arms — humerus + radius/ulna with claw hands. Slightly
	// outstretched stance.
	// Left arm.
	drawLinePixels(pixels, w, h, cx-14, 44, cx-22, 64, boneDark, 3)
	drawLinePixels(pixels, w, h, cx-14, 44, cx-22, 64, bone, 1)
	fillEllipsePixels(pixels, w, h, cx-22, 64, 2, 2, boneDark) // elbow
	drawLinePixels(pixels, w, h, cx-22, 64, cx-26, 82, boneDark, 3)
	drawLinePixels(pixels, w, h, cx-22, 64, cx-26, 82, bone, 1)
	// Left claw hand — three finger lines fanning out.
	drawLinePixels(pixels, w, h, cx-26, 82, cx-30, 88, bone, 1)
	drawLinePixels(pixels, w, h, cx-26, 82, cx-26, 90, bone, 1)
	drawLinePixels(pixels, w, h, cx-26, 82, cx-22, 88, bone, 1)
	// Right arm — mirror, holds a small rusty cleaver.
	drawLinePixels(pixels, w, h, cx+14, 44, cx+22, 64, boneDark, 3)
	drawLinePixels(pixels, w, h, cx+14, 44, cx+22, 64, bone, 1)
	fillEllipsePixels(pixels, w, h, cx+22, 64, 2, 2, boneDark)
	drawLinePixels(pixels, w, h, cx+22, 64, cx+26, 82, boneDark, 3)
	drawLinePixels(pixels, w, h, cx+22, 64, cx+26, 82, bone, 1)
	// Rusty cleaver — small triangle blade jutting from the right hand.
	fillTrianglePixels(pixels, w, h, cx+26, 82, cx+34, 76, cx+32, 90, rust)
	fillTrianglePixels(pixels, w, h, cx+26, 82, cx+32, 80, cx+30, 88, rustDark)

	// Skull — slightly wider than tall, with a recessed jaw.
	fillEllipsePixels(pixels, w, h, cx, 28, 12, 13, boneDark)
	fillEllipsePixels(pixels, w, h, cx, 26, 10, 11, bone)
	fillEllipsePixels(pixels, w, h, cx-1, 22, 7, 5, boneLight)
	// Eye sockets — two deep hollows with glowing red.
	fillEllipsePixels(pixels, w, h, cx-4, 28, 3, 3, socket)
	fillEllipsePixels(pixels, w, h, cx+4, 28, 3, 3, socket)
	fillEllipsePixels(pixels, w, h, cx-4, 28, 1, 1, eyeBright)
	fillEllipsePixels(pixels, w, h, cx+4, 28, 1, 1, eyeBright)
	fillEllipsePixels(pixels, w, h, cx-4, 28, 2, 2, color.RGBA{R: eyeGlow.R, G: eyeGlow.G, B: eyeGlow.B, A: 110})
	fillEllipsePixels(pixels, w, h, cx+4, 28, 2, 2, color.RGBA{R: eyeGlow.R, G: eyeGlow.G, B: eyeGlow.B, A: 110})
	// Nasal cavity.
	fillTrianglePixels(pixels, w, h, cx-1, 33, cx+1, 33, cx, 36, socket)
	// Tooth line — alternating notches at the jaw.
	for i := 0; i < 6; i++ {
		px := cx - 5 + i*2
		fillRectPixels(pixels, w, h, px, 37, 1, 3, socket)
	}
	// Jaw crack — a single damage line across the lower skull.
	drawLinePixels(pixels, w, h, cx-6, 35, cx+2, 37, socket, 1)

	// Per-pixel dither — denser than the soft creatures so bone
	// reads as a brittle, pitted surface.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*w + x
			if pixels[i].A == 0 {
				continue
			}
			if hashByteXY(x, y)%7 == 0 {
				pixels[i] = adjust(pixels[i], -12)
			}
		}
	}
	return pixels
}

func makePartyPixels(w, h int, class core.PartyClass) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	shadow := color.RGBA{R: 0, G: 0, B: 0, A: 78}
	skin := color.RGBA{R: 219, G: 165, B: 124, A: 255}
	boot := color.RGBA{R: 33, G: 34, B: 42, A: 255}

	fillEllipsePixels(pixels, w, h, 32, 73, 18, 4, shadow)
	fillRectPixels(pixels, w, h, 23, 57, 9, 12, boot)
	fillRectPixels(pixels, w, h, 33, 57, 9, 12, boot)
	fillEllipsePixels(pixels, w, h, 31, 38, 7, 7, skin)

	presentation := partyClassPresentationFor(class)
	presentation.drawPixels(pixels, w, h)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*w + x
			if pixels[i].A == 0 {
				continue
			}
			if hashByteXY(x+presentation.textureSeed*17, y)%9 == 0 {
				pixels[i] = adjust(pixels[i], -10)
			}
		}
	}
	return pixels
}

func fillEllipsePixels(pixels []color.RGBA, w, h, cx, cy, rx, ry int, col color.RGBA) {
	for y := cy - ry; y <= cy+ry; y++ {
		if y < 0 || y >= h {
			continue
		}
		for x := cx - rx; x <= cx+rx; x++ {
			if x < 0 || x >= w {
				continue
			}
			dx := float64(x-cx) / float64(rx)
			dy := float64(y-cy) / float64(ry)
			if dx*dx+dy*dy <= 1 {
				pixels[y*w+x] = col
			}
		}
	}
}

func fillRectPixels(pixels []color.RGBA, w, h, x, y, rw, rh int, col color.RGBA) {
	for py := y; py < y+rh; py++ {
		if py < 0 || py >= h {
			continue
		}
		for px := x; px < x+rw; px++ {
			if px >= 0 && px < w {
				pixels[py*w+px] = col
			}
		}
	}
}

func fillTrianglePixels(pixels []color.RGBA, w, h, x1, y1, x2, y2, x3, y3 int, col color.RGBA) {
	minX := core.MinInt(x1, core.MinInt(x2, x3))
	maxX := core.MaxInt(x1, core.MaxInt(x2, x3))
	minY := core.MinInt(y1, core.MinInt(y2, y3))
	maxY := core.MaxInt(y1, core.MaxInt(y2, y3))
	area := edgeFunction(x1, y1, x2, y2, x3, y3)
	if area == 0 {
		return
	}
	for y := minY; y <= maxY; y++ {
		if y < 0 || y >= h {
			continue
		}
		for x := minX; x <= maxX; x++ {
			if x < 0 || x >= w {
				continue
			}
			w0 := edgeFunction(x2, y2, x3, y3, x, y)
			w1 := edgeFunction(x3, y3, x1, y1, x, y)
			w2 := edgeFunction(x1, y1, x2, y2, x, y)
			if (w0 >= 0 && w1 >= 0 && w2 >= 0) || (w0 <= 0 && w1 <= 0 && w2 <= 0) {
				pixels[y*w+x] = col
			}
		}
	}
}

func edgeFunction(ax, ay, bx, by, cx, cy int) int {
	return (cx-ax)*(by-ay) - (cy-ay)*(bx-ax)
}

func drawLinePixels(pixels []color.RGBA, w, h, x0, y0, x1, y1 int, col color.RGBA, thickness int) {
	dx := x1 - x0
	dy := y1 - y0
	steps := int(math.Max(math.Abs(float64(dx)), math.Abs(float64(dy))))
	if steps == 0 {
		return
	}
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := int(math.Round(float64(x0) + float64(dx)*t))
		y := int(math.Round(float64(y0) + float64(dy)*t))
		for oy := -thickness / 2; oy <= thickness/2; oy++ {
			for ox := -thickness / 2; ox <= thickness/2; ox++ {
				px := x + ox
				py := y + oy
				if px >= 0 && px < w && py >= 0 && py < h {
					pixels[py*w+px] = col
				}
			}
		}
	}
}

// hashByteXY is hashXY masked to a byte, suitable for "% N" style
// bucketing in pixel-painting loops. Lives next to its callers so the
// per-byte intent is local; deeper hash callers (world.go's tile-yaw /
// floor-variant selectors) use hashXY directly with their own masks.
func hashByteXY(x, y int) int {
	return int(hashXY(x, y) & 0xff)
}

func jitter(c color.RGBA, x, y, amount int) color.RGBA {
	return adjust(c, hashByteXY(x, y)%(amount*2+1)-amount)
}

func adjust(c color.RGBA, delta int) color.RGBA {
	return color.RGBA{
		R: core.ClampByte(int(c.R) + delta),
		G: core.ClampByte(int(c.G) + delta),
		B: core.ClampByte(int(c.B) + delta),
		A: c.A,
	}
}

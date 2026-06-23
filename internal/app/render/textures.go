package render

import (
	"image/color"
	"math"

	"crawler/internal/app/core"
)

// makeSoftShadowPixels builds the radial ground-shadow sprite: a dark disc whose alpha falls (1-d)^1.6 to a transparent edge.
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
	// Pastel cream-stone wall. The dungeon mood comes from the lighting profile
	// (cool ambient + heavy shadow), not from the texture going grey.
	base := color.RGBA{R: 188, G: 178, B: 160, A: 255}
	shadow := color.RGBA{R: 120, G: 112, B: 102, A: 255}
	highlight := color.RGBA{R: 226, G: 218, B: 198, A: 255}
	moss := color.RGBA{R: 158, G: 192, B: 148, A: 255}
	mossDeep := color.RGBA{R: 124, G: 168, B: 124, A: 255}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			// Two-octave noise: broad band = weathering patches, fine = grain.
			broad := fbmNoise(float64(x), float64(y), 0.012, 3)
			fine := fbmNoise(float64(x)*1.4+97, float64(y)*1.4-211, 0.046, 4)
			n := broad*0.55 + fine*0.45
			c := base
			c = core.MixColor(c, highlight, math.Max(0, n)*0.72)
			c = core.MixColor(c, shadow, math.Max(0, -n)*0.55)

			// Crack scatter as soft pits, each with a lit lower lip so it reads concave.
			pit := hashByteXY(x/3, y/3)
			if pit%64 == 0 {
				c = core.MixColor(c, shadow, 0.38)
			} else if y >= 3 && hashByteXY(x/3, (y-3)/3)%64 == 0 {
				c = core.MixColor(c, highlight, 0.30)
			}
			// Block seams only where broad noise is low, so cracks follow the
			// rock's folds rather than a tiled grid.
			cellX, cellY := x/28, y/28
			seam := hashByteXY(cellX, cellY) % 6
			if ((x+seam)%28 == 0 || (y+seam)%28 == 0) && broad < 0.05 {
				c = core.MixColor(c, shadow, 0.42)
			}
			// Moss creeps up the bottom third in patches (bright + deep) so it
			// reads as 3D growth, not a flat wash.
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
			// Vertical form: occlusion at the foot, sky kiss at the top — large-
			// scale value structure so the wall reads as standing.
			vy := float64(y) / float64(h)
			if vy > 0.66 {
				c = core.MixColor(c, shadow, (vy-0.66)*0.50)
			} else if vy < 0.14 {
				c = core.MixColor(c, highlight, (0.14-vy)*0.55)
			}
			pixels[y*w+x] = c
		}
	}
	return pixels
}

// makeRockIvyPixels paints the rock wall, then grows ivy leaves along wandering vine cords. `heavy` adds more/larger leaves.
func makeRockIvyPixels(w, h int, heavy bool) []color.RGBA {
	pixels := makeRockWallPixels(w, h)
	// Layered ivy palette — several greens plus a woody vine.
	vine := color.RGBA{R: 78, G: 84, B: 54, A: 255}
	leafA := color.RGBA{R: 74, G: 122, B: 60, A: 255}
	leafB := color.RGBA{R: 92, G: 142, B: 70, A: 255}
	leafLit := color.RGBA{R: 132, G: 178, B: 92, A: 255}
	leafDark := color.RGBA{R: 46, G: 90, B: 50, A: 255}

	strands := 4
	leafEvery := 10
	leafR := 2.7
	if heavy {
		strands = 8
		leafEvery = 6
		leafR = 3.3
	}
	// plotLeaf stamps one near-oval leaf at (cx,cy), wrapping in x (texture tiles).
	plotLeaf := func(cx, cy int, r float64, base color.RGBA) {
		ri := int(r) + 1
		for dy := -ri; dy <= ri; dy++ {
			for dx := -ri; dx <= ri; dx++ {
				fx := float64(dx) / r
				fy := float64(dy) / (r * 1.35) // taller than wide
				if fx*fx+fy*fy > 1 {
					continue
				}
				y := cy + dy
				if y < 0 || y >= h {
					continue
				}
				x := ((cx+dx)%w + w) % w
				c := base
				// Lit upper-left, shadowed lower-right.
				switch s := fx + fy; {
				case s < -0.35:
					c = core.MixColor(c, leafLit, 0.55)
				case s > 0.45:
					c = core.MixColor(c, leafDark, 0.55)
				}
				// Midrib.
				if dx == 0 {
					c = core.MixColor(c, leafDark, 0.3)
				}
				pixels[y*w+x] = core.MixColor(pixels[y*w+x], c, 0.94)
			}
		}
	}
	for s := 0; s < strands; s++ {
		x := float64(int(hashByteXY(s*23+5, 7)) % w)
		for y := 0; y < h; y++ {
			// Gentle wander: sine plus per-row hash jitter.
			x += math.Sin(float64(y)*0.07+float64(s)*1.7)*0.45 + (float64(int(hashByteXY(s, y))%3)-1)*0.28
			cx := ((int(x))%w + w) % w
			// Woody vine cord (2px, soft).
			pixels[y*w+cx] = core.MixColor(pixels[y*w+cx], vine, 0.65)
			if cx+1 < w {
				pixels[y*w+cx+1] = core.MixColor(pixels[y*w+cx+1], vine, 0.32)
			}
			if y%leafEvery == 0 {
				// Big leaf to one alternating side, smaller one half a step down.
				side := 1.0
				if (y/leafEvery)%2 == 0 {
					side = -1.0
				}
				base := leafA
				if hashByteXY(s, y)%2 == 0 {
					base = leafB
				}
				plotLeaf(cx+int(side*(leafR+1)), y, leafR, base)
				plotLeaf(cx, y+leafEvery/2, leafR*0.66, leafDark)
			}
		}
	}
	return pixels
}

// makeRockCrackedPixels paints the rock wall with a couple of wandering near-vertical cracks + a faint caught-light lip.
func makeRockCrackedPixels(w, h int) []color.RGBA {
	pixels := makeRockWallPixels(w, h)
	crack := color.RGBA{R: 104, G: 98, B: 88, A: 255} // mid-tone
	lip := color.RGBA{R: 208, G: 200, B: 182, A: 255}

	plot := func(x, y int, depth float64) {
		if x < 0 || x >= w || y < 0 || y >= h {
			return
		}
		pixels[y*w+x] = core.MixColor(pixels[y*w+x], crack, depth)
		if x+1 < w {
			pixels[y*w+x+1] = core.MixColor(pixels[y*w+x+1], lip, depth*0.2)
		}
	}
	walkCrack := func(startX, startY, endY, seed int) {
		x := float64(startX)
		for y := startY; y < endY; y++ {
			x += fbmNoise(float64(y)*0.5+float64(seed)*31, float64(seed)*7, 0.4, 3) * 1.1
			plot(int(x), y, 0.45)
			if hashByteXY(int(x), y)%52 == 0 {
				bx := x
				for by := y; by < y+h/8 && by < endY; by++ {
					bx += 1.0
					plot(int(bx), by, 0.32)
				}
			}
		}
	}
	for i := 0; i < 2; i++ {
		sx := int(hashByteXY(i*17+5, 3)) % w
		walkCrack(sx, 0, h, i+1)
	}
	return pixels
}

// makeRockCrumblingPixels paints weathered/chipped stone: soft shadow in low
// noise patches, a pale lit edge above each, rubble specks near the base.
func makeRockCrumblingPixels(w, h int) []color.RGBA {
	pixels := makeRockWallPixels(w, h)
	shadow := color.RGBA{R: 104, G: 98, B: 88, A: 255}
	chipLit := color.RGBA{R: 206, G: 198, B: 180, A: 255}
	rubble := color.RGBA{R: 168, G: 158, B: 142, A: 255}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			n := fbmNoise(float64(x)*0.9+41, float64(y)*0.9+17, 0.03, 4)
			if n < -0.18 {
				pixels[y*w+x] = core.MixColor(pixels[y*w+x], shadow, math.Min(0.5, (-0.18-n)*2))
			} else if y >= 2 {
				if above := fbmNoise(float64(x)*0.9+41, float64(y-2)*0.9+17, 0.03, 4); above < -0.18 {
					pixels[y*w+x] = core.MixColor(pixels[y*w+x], chipLit, 0.26)
				}
			}
			if y > h*3/4 && hashByteXY(x, y)%6 == 0 {
				pixels[y*w+x] = core.MixColor(pixels[y*w+x], rubble, 0.4)
			}
		}
	}
	return pixels
}

// bevelParams tunes bevelCell: per-edge (base, falloff) shade strengths, the lit
// (top/left) and dark (bottom/right) tints, the edge widths under which each side
// is shaded, and a per-cell jitter multiplier (1 = none).
type bevelParams struct {
	lit, dark            color.RGBA
	topBase, topFall     float64
	leftBase, leftFall   float64
	botBase, botFall     float64
	rightBase, rightFall float64
	litWidth, darkWidth  int
	jitter               float64
}

// bevelCell shades c for a high-left key light: lit on the top/left edges, dark on
// bottom/right, falling off across the edge width. localX/localY are the position
// within a cellW×cellH cell whose mortar/grout inset is `inset`. Shared by the
// stone brick + floor painters.
func bevelCell(c color.RGBA, localX, localY, cellW, cellH, inset int, p bevelParams) color.RGBA {
	distTop := localY - inset
	distLeft := localX - inset
	distBottom := cellH - 1 - localY
	distRight := cellW - 1 - localX
	if distTop <= p.litWidth {
		c = core.MixColor(c, p.lit, (p.topBase-float64(distTop)*p.topFall)*p.jitter)
	} else if distLeft <= p.litWidth {
		c = core.MixColor(c, p.lit, (p.leftBase-float64(distLeft)*p.leftFall)*p.jitter)
	}
	if distBottom <= p.darkWidth {
		c = core.MixColor(c, p.dark, (p.botBase-float64(distBottom)*p.botFall)*p.jitter)
	} else if distRight <= p.darkWidth {
		c = core.MixColor(c, p.dark, (p.rightBase-float64(distRight)*p.rightFall)*p.jitter)
	}
	return c
}

func makeStoneBrickPixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	brickW := 32
	brickH := 16
	mortar := 2
	// Pastel cream-stone brick, same family as the rock wall. Dungeon mood comes
	// from the lighting override, not cold stone colour.
	base := color.RGBA{R: 178, G: 170, B: 154, A: 255}
	warm := color.RGBA{R: 208, G: 184, B: 144, A: 255}
	cool := color.RGBA{R: 152, G: 158, B: 168, A: 255}
	mortarColor := color.RGBA{R: 74, G: 68, B: 60, A: 255}
	mortarLight := color.RGBA{R: 118, G: 110, B: 96, A: 255}
	moss := color.RGBA{R: 148, G: 184, B: 132, A: 255}

	// One key light, high-left: every brick lit top/left, shadowed bottom/right, casting into the seam below.
	lip := color.RGBA{R: 232, G: 220, B: 196, A: 255}
	pitDark := color.RGBA{R: 30, G: 28, B: 24, A: 255}
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
				// Seam shading: horizontal seams shadow at top, lit sill below;
				// vertical seams shadow on their left. Recessed, not a drawn line.
				if localY < mortar {
					if localY == 0 {
						c = core.MixColor(c, pitDark, 0.52)
					} else {
						c = core.MixColor(c, mortarLight, 0.30)
					}
				} else if localX == 0 {
					c = core.MixColor(c, pitDark, 0.38)
				}
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
			// Muted highlight so crests don't flare against the base.
			c = core.MixColor(c, color.RGBA{R: 188, G: 184, B: 172, A: 255}, math.Max(0, n)*0.30)
			c = core.MixColor(c, pitDark, math.Max(0, -n)*0.42)

			// Directional bevel: top lip brightest, left dimmer; bottom shadowed,
			// right less. Per-brick hash jitters lip strength.
			bevelJitter := 0.85 + float64(hashByteXY(brickX*5, row*11)%64)/210.0
			c = bevelCell(c, localX, localY, brickW, brickH, mortar, bevelParams{
				lit: lip, dark: pitDark,
				topBase: 0.44, topFall: 0.18, leftBase: 0.28, leftFall: 0.12,
				botBase: 0.46, botFall: 0.18, rightBase: 0.26, rightFall: 0.10,
				litWidth: 1, darkWidth: 1, jitter: bevelJitter,
			})

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
	// Pastel meadow — calm mint wash; blooms are very sparse (the flower props carry the floral detail).
	base := color.RGBA{R: 132, G: 196, B: 102, A: 255}
	light := color.RGBA{R: 186, G: 224, B: 134, A: 255}
	dark := color.RGBA{R: 98, G: 162, B: 92, A: 255}
	dirt := color.RGBA{R: 184, G: 150, B: 100, A: 255}
	bloomYellow := color.RGBA{R: 244, G: 218, B: 120, A: 255}
	bloomWhite := color.RGBA{R: 244, G: 240, B: 224, A: 255}
	bloomPink := color.RGBA{R: 238, G: 174, B: 196, A: 255}

	dapple := color.RGBA{R: 214, G: 234, B: 148, A: 255}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			broad := fbmNoise(float64(x), float64(y), 0.020, 3)
			c := base
			c = core.MixColor(c, light, math.Max(0, broad)*0.55)
			c = core.MixColor(c, dark, math.Max(0, -broad)*0.40)
			// Canopy dapple — broad warm-lemon pools for large-scale light structure.
			dap := fbmNoise(float64(x)-340, float64(y)+209, 0.008, 2)
			if dap > 0.28 {
				c = core.MixColor(c, dapple, (dap-0.28)*0.45)
			}
			// Dirt scuff — large patches.
			m := fbmNoise(float64(x)+512, float64(y)-271, 0.014, 2)
			if m > 0.50 {
				c = core.MixColor(c, dirt, (m-0.50)*0.65)
			}
			// Sparse paired blade strokes — dark rooted dash + lit dash on the sun side, on a 4-px grain.
			if hashByteXY(x*3, y/4)%140 < 2 {
				c = core.MixColor(c, dark, 0.55)
			} else if x > 0 && hashByteXY((x-1)*3, y/4)%140 < 2 {
				c = core.MixColor(c, light, 0.50)
			}
			// Very sparse bloom scatter — just a hint of wildflowers.
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
	// Pastel cream-slab floor, same family as the brick wall.
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

			// Foot-worn sheen — soft radial lift peaking at the slab center.
			fx := float64(localX) - float64(slab)/2
			fy := float64(localY) - float64(slab)/2
			wear := 1.0 - (fx*fx+fy*fy)/(float64(slab*slab)/4.0)
			if wear > 0 {
				c = core.MixColor(c, highlight, wear*0.12)
			}

			// Directional bevel (high-left key): lit top/left, shadowed bottom/right.
			c = bevelCell(c, localX, localY, slab, slab, grout, bevelParams{
				lit: highlight, dark: groutColor,
				topBase: 0.30, topFall: 0.12, leftBase: 0.20, leftFall: 0.08,
				botBase: 0.40, botFall: 0.12, rightBase: 0.28, rightFall: 0.09,
				litWidth: 1, darkWidth: 2, jitter: 1,
			})
			if hashByteXY(slabX*11+localX/4, slabY*19+localY/4)%72 < 3 {
				c = adjust(c, -32)
			}
			pixels[y*w+x] = c
		}
	}
	return pixels
}

// makeDirtPixels paints warm earth: brushstroke noise plus scattered pebbles and occasional grass sprouts.
func makeDirtPixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	// Pastel peach-tan earth — a sun-warmed path beside the pastel grass.
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
				// Rare grass sprout.
				c = core.MixColor(c, sprout, 0.65)
			}
			pixels[y*w+x] = c
		}
	}
	return pixels
}

// makeDarkGrassPixels paints forest-mint grass for shaded patches — like makeGrassPixels but differing in HUE, not value.
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
			pixels[y*w+x] = c
		}
	}
	return pixels
}

// makeCobblePixels paints a mortared cobblestone path: hash-nudged rounded stones with mossy gaps and wet-spot highlights.
func makeCobblePixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	// Pastel cream stones over warm mortar, same family as the rock wall.
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

			// Per-stone center + radius jitter so the cobbles aren't a uniform grid.
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

			// Per-stone dome shading (high-left key): highlight off-center
			// upper-left, rim shadow on the lower-right.
			ldx := (dx + 3.0) / rx
			ldy := (dy + 3.0) / ry
			dlit := ldx*ldx + ldy*ldy
			lighting := 1.0 - dlit*0.62
			if lighting > 0 {
				c = core.MixColor(c, light, lighting*0.38)
			}
			if d > 0.78 {
				rimT := (d - 0.78) * 1.6
				// Shadow side (lower-right) rims darker.
				if dx+dy > 0 {
					rimT *= 1.35
				} else {
					rimT *= 0.7
				}
				c = core.MixColor(c, dark, rimT)
			}

			n := fbmNoise(float64(x)*1.4, float64(y)*1.4, 0.20, 4)
			c = core.MixColor(c, light, math.Max(0, n)*0.20)
			c = core.MixColor(c, dark, math.Max(0, -n)*0.30)

			// Sparse darker pits — chips and weather.
			if hashByteXY(cellX*23+localX/3, cellY*29+localY/3)%88 < 3 {
				c = adjust(c, -34)
			}
			pixels[y*w+x] = c
		}
	}
	return pixels
}

// makePlankPixels paints a plank floor: boards with darker gaps, grain noise, and scattered knots, staggered by row group.
func makePlankPixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	const boardH = 22
	const gap = 2
	gapColor := color.RGBA{R: 96, G: 70, B: 48, A: 255}
	// Pastel honey-wood, matching the bark family.
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
			// Plank gap (vertical seam). Boards are 96px wide so seams don't align row-to-row.
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
			// Horizontal grain: stretched along x, finer in y, so it reads as wood fibers.
			n := fbmNoise(float64(x)*0.15, float64(y)*1.6, 0.20, 4)
			c = core.MixColor(c, warm, math.Max(0, n)*0.35)
			c = core.MixColor(c, grain, math.Max(0, -n)*0.55)

			// Board edge darkening so each plank reads as raised.
			edge := min(localY-gap, boardH-1-localY)
			if edge <= 2 {
				c = core.MixColor(c, gapColor, 0.30-float64(edge)*0.10)
			}

			// Knots, ~1 per board.
			if hashByteXY((x+offset)/8, y/3)%420 < 3 {
				c = core.MixColor(c, knot, 0.70)
			}
			pixels[y*w+x] = c
		}
	}
	return pixels
}

// waterParams tunes makeWaterBase per variant. sand with zero alpha disables the
// sandy bottom; sunGlints adds shallow-only horizontal dashes.
type waterParams struct {
	deep, mid, shine, weed color.RGBA
	sand                   color.RGBA // zero A → no sandy bottom (the deep-water "can't wade" cue)
	peakCut, peakGain      float64    // crest threshold + brightness gain
	weedMod, weedHit       int        // weed scatter: hashByteXY(x/2,y*3)%weedMod < weedHit
	weedMix                float64
	sunGlints              bool
}

// makeWaterBase paints the shared banded-blue water field (FBM gradient + crest
// peaks + weed scatter); makeWaterPixels / makeDeepWaterPixels differ only by p.
func makeWaterBase(w, h int, p waterParams) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			n := fbmNoise(float64(x), float64(y), 0.04, 4)
			band := math.Sin(float64(y)*0.08 + n*1.8)
			c := core.MixColor(p.deep, p.mid, 0.45+band*0.35+n*0.20)

			// Bright crests at noise peaks.
			peak := fbmNoise(float64(x)*1.3+311, float64(y)*1.3-91, 0.10, 3)
			if peak > p.peakCut {
				c = core.MixColor(c, p.shine, (peak-p.peakCut)*p.peakGain)
			}
			// Sun glints — short horizontal near-white dashes (x/7 = 7-px streak), crest-gated to the lit side.
			if p.sunGlints && band > 0.25 && hashByteXY(x/7, y)%170 < 2 {
				c = core.MixColor(c, p.shine, 0.85)
			}
			// Sandy bottom where the FBM dips deep.
			if p.sand.A != 0 && n < -0.45 {
				c = core.MixColor(c, p.sand, (-n-0.45)*0.5)
			}
			// Rare weed strands.
			if hashByteXY(x/2, y*3)%p.weedMod < p.weedHit {
				c = core.MixColor(c, p.weed, p.weedMix)
			}
			pixels[y*w+x] = c
		}
	}
	return pixels
}

// makeWaterPixels paints shallow water: banded blue gradient with FBM ripples + peaks, light enough to read as wadeable.
func makeWaterPixels(w, h int) []color.RGBA {
	return makeWaterBase(w, h, waterParams{
		deep:    color.RGBA{R: 96, G: 168, B: 192, A: 255}, // pastel aqua
		mid:     color.RGBA{R: 150, G: 204, B: 214, A: 255},
		shine:   color.RGBA{R: 220, G: 238, B: 232, A: 255},
		weed:    color.RGBA{R: 108, G: 170, B: 110, A: 255},
		sand:    color.RGBA{R: 214, G: 190, B: 144, A: 255},
		peakCut: 0.55, peakGain: 1.4,
		weedMod: 560, weedHit: 4, weedMix: 0.45,
		sunGlints: true,
	})
}

// makeDeepWaterPixels paints the blocking deep-water variant: like makeWaterPixels but darker/cooler, no sandy bottom (the "can't wade" cue).
func makeDeepWaterPixels(w, h int) []color.RGBA {
	return makeWaterBase(w, h, waterParams{
		deep:    color.RGBA{R: 92, G: 138, B: 160, A: 255}, // pastel teal, colder/more saturated
		mid:     color.RGBA{R: 124, G: 168, B: 184, A: 255},
		shine:   color.RGBA{R: 196, G: 220, B: 222, A: 255},
		weed:    color.RGBA{R: 96, G: 148, B: 120, A: 255},
		peakCut: 0.62, peakGain: 0.9,
		weedMod: 620, weedHit: 3, weedMix: 0.40,
	})
}

// makeSandPixels paints dry dune sand: warm cream with gentle dune-roll noise and very sparse pebbles.
func makeSandPixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	// Pastel warm-cream sand, smooth (no per-pixel speckle).
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
			// Very sparse pebble flecks.
			if hashByteXY(x*7, y*11)%680 < 2 {
				c = core.MixColor(c, pebble, 0.45)
			}
			pixels[y*w+x] = c
		}
	}
	return pixels
}

// makeSnowPixels paints packed snow: near-white with faint blue shadow noise and sparkle specks.
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
			// Sparkle specks — light off ice crystals.
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
	// Pastel bark — warm pecan lit, soft umber in the creases.
	base := color.RGBA{R: 168, G: 130, B: 96, A: 255}
	deep := color.RGBA{R: 104, G: 76, B: 56, A: 255}
	light := color.RGBA{R: 214, G: 184, B: 142, A: 255}
	moss := color.RGBA{R: 154, G: 188, B: 138, A: 255}

	rim := color.RGBA{R: 236, G: 208, B: 162, A: 255}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			// Sinuous ridge wave, gently warped so ridges flow not sawtooth.
			warp := fbmNoise(float64(x), float64(y), 0.04, 3) * 3.2
			ridge := math.Sin(float64(x)*0.40 + warp)
			ridge = math.Abs(ridge)
			n := fbmNoise(float64(x)*0.9, float64(y)*0.4, 0.22, 4)
			c := base
			c = core.MixColor(c, light, math.Max(0, ridge-0.42)*1.5)
			c = core.MixColor(c, deep, math.Max(0, 0.38-ridge)*0.7)
			// Side-lit fissures: where the wave climbs out of a crease, the
			// ridge's left flank catches the key light as a warm rim.
			prevRidge := math.Abs(math.Sin(float64(x-1)*0.40 + warp))
			if ridge > 0.46 && prevRidge < 0.40 {
				c = core.MixColor(c, rim, 0.45)
			}
			c = core.MixColor(c, light, math.Max(0, n)*0.32)
			c = core.MixColor(c, deep, math.Max(0, -n)*0.40)

			// Sparse pits + vertical moss patches (up the shaded side).
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
	// Pastel-saturated canopy — spring green with cream dapples and gold accents.
	base := color.RGBA{R: 142, G: 204, B: 110, A: 255}
	light := color.RGBA{R: 200, G: 232, B: 144, A: 255}
	deep := color.RGBA{R: 96, G: 160, B: 96, A: 255}
	gold := color.RGBA{R: 238, G: 224, B: 138, A: 255}
	hotspot := color.RGBA{R: 236, G: 244, B: 180, A: 255}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			// Two-octave noise — clumpy patches plus finer brushstroke.
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
			// Sunlit hotspots where broad + fine crests align.
			if broad > 0.32 && fine > 0.38 {
				c = core.MixColor(c, hotspot, 0.25)
			}
			// No v-axis gradient: GenMeshSphere's poles run along ±Z (horizontal),
			// so a v-gradient would paint a sideways dark hemisphere. Top/under
			// shading comes from the sun shader's NdotL. Keep this isotropic.
			pixels[y*w+x] = c
		}
	}
	return pixels
}

// makeMarblePixels paints veined marble for upright props: noise veins through a cream base with hairline cracks.
func makeMarblePixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	// Muted cream-grey so it doesn't flare against the floor/wall palette.
	base := color.RGBA{R: 206, G: 200, B: 188, A: 255}
	warm := color.RGBA{R: 216, G: 208, B: 192, A: 255}
	cool := color.RGBA{R: 180, G: 182, B: 188, A: 255}
	vein := color.RGBA{R: 116, G: 110, B: 102, A: 255}
	deep := color.RGBA{R: 76, G: 72, B: 66, A: 255}

	polish := color.RGBA{R: 238, G: 232, B: 218, A: 255}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			n := fbmNoise(float64(x), float64(y), 0.04, 4)
			m := fbmNoise(float64(x)+177, float64(y)-91, 0.10, 3)
			c := base
			c = core.MixColor(c, warm, math.Max(0, n)*0.35)
			c = core.MixColor(c, cool, math.Max(0, -n)*0.30)

			// Veins: thin streaks where two FBM samples cross zero, wrapped in a
			// warm bruise (mineral staining) before the dark crack core.
			vt := math.Abs(m + n*0.4)
			if vt < 0.12 {
				c = core.MixColor(c, warm, (0.12-vt)*2.2)
			}
			if vt < 0.06 {
				c = core.MixColor(c, vein, 0.45)
			}
			if vt < 0.02 {
				c = core.MixColor(c, deep, 0.55)
			}
			// Polish sheen — a faint diagonal light-band, sine-based so it tiles.
			s := math.Sin((float64(x) + float64(y)*0.7) * 0.045)
			if s > 0.55 {
				c = core.MixColor(c, polish, (s-0.55)*0.40)
			}
			pixels[y*w+x] = c
		}
	}
	return pixels
}

// makeGranitePixels paints dark speckled granite for the obelisk — denser/cooler than marble (a different stone class).
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
			// Mica flecks.
			if hashByteXY(x*5, y*5)%420 < 3 {
				c = core.MixColor(c, flake, 0.55)
			}
			pixels[y*w+x] = c
		}
	}
	return pixels
}

// makeTerracottaPixels paints warm clay for the urn: horizontal wheel-mark banding plus a vertical gradient.
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
			// Sparse pits and chips.
			if hashByteXY(x, y)%240 < 2 {
				c = core.MixColor(c, rim, 0.55)
			}
			pixels[y*w+x] = c
		}
	}
	return pixels
}

// fbmNoise returns fractal Brownian motion in roughly [-1, 1] over hashed value noise.
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

// makeHudGrainPixels builds a tileable HUD-glass grain overlay: sparse dark specks, warm flecks, faint fibers; mostly alpha 0.
func makeHudGrainPixels(w, h int) []color.RGBA {
	px := make([]color.RGBA, w*h)
	for y := 0; y < h; y++ {
		// Faint horizontal fiber at coprime spacings so they don't beat into a stripe.
		fiber := 0
		if y%7 == 0 {
			fiber = -1
		} else if y%11 == 0 {
			fiber = 1
		}
		for x := 0; x < w; x++ {
			r := hash01(hashXY(x, y))
			c := color.RGBA{0, 0, 0, 0}
			switch {
			case r < 0.06:
				c = color.RGBA{R: 0, G: 0, B: 0, A: uint8(16 + r/0.06*26)} // 16..42
			case r > 0.965:
				c = color.RGBA{R: 255, G: 240, B: 214, A: uint8(12 + (r-0.965)/0.035*18)} // 12..30
			}
			if fiber == -1 && c.A < 14 {
				c = color.RGBA{R: 0, G: 0, B: 0, A: 12}
			} else if fiber == 1 && c.A < 12 {
				c = color.RGBA{R: 255, G: 238, B: 210, A: 9}
			}
			px[y*w+x] = c
		}
	}
	return px
}

func makeSkyPixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	// Pastel sky — baby-blue zenith to peach-cream horizon.
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
	// Cloud tint off pure white so clouds don't glare.
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

// makeStarPixels builds the transparent night star-field overlay. Mostly
// RGBA(0,0,0,0); a sparse scatter of bright pixels (with halos) reads as stars.
// Density tapers toward the horizon. Hash-driven (not rand.*) so the star map is
// stable across runs and platforms.
func makeStarPixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	transparent := color.RGBA{R: 0, G: 0, B: 0, A: 0}
	for i := range pixels {
		pixels[i] = transparent
	}

	// Star palette: cool white (common), pale blue, warm yellow. Pulled by a
	// separate hash byte so color and brightness are independent.
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
		// Overwrite, not blend — "max over" would dim a bright star on a halo.
		pixels[y*w+x] = c
	}

	// Density falloff: quadratic taper, dense at top to nearly clear at horizon.
	// At baseProb 0.0035, ~1024×512 gives ~700 stars before halos.
	const baseProb = 0.0035
	densityAt := func(y int) float64 {
		t := float64(y) / float64(h-1)
		// 1 at the top, ~0.05 at the bottom.
		falloff := 1.0 - t*t*1.05
		if falloff < 0.05 {
			falloff = 0.05
		}
		return falloff
	}

	// Per-star brightness curve mapping the brightness byte to a core alpha:
	//   <160 (63%)→70, <220 (24%)→130, <248 (11%)→200, else (3%)→255.
	// Halo + sparkle alphas scale off the core.
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
			// Don't pre-multiply at the call site — hashXY already does, and the
			// inner math would overflow before mix32 sees it.
			h0 := uint16(hashXY(x, y))
			if h0 >= thresh {
				continue
			}
			// Secondary hash (offset to decorrelate from h0) drives brightness/color/halo.
			h1 := hashXY(x+91317, y+58271)
			brightness := int(h1 & 0xFF)
			coreAlpha := coreAlphaFor(brightness)
			col := colorFor(brightness)
			col.A = coreAlpha

			// Core pixel.
			setPx(x, y, col)

			// Halo (~55% of core) + sparkle (~30%) only on the brighter half;
			// dim stars stay a single pixel.
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

// ratPalette is the recolorable palette for makeRatPixels; plain and diseased
// rats share the silhouette, differ in tones.
type ratPalette struct {
	body, bodyDark, bodyLight color.RGBA
	ear, tail                 color.RGBA
	eye, nose                 color.RGBA
	// poison: when alpha != 0, makeRatPixels paints drip drops under the snout.
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

// diseasedRatPalette: sickly-green tones, jaundiced eye, poison drips.
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

	// Poison drips (diseased rat only) — three drops from the snout.
	if p.poison.A != 0 {
		poison := p.poison
		poisonDark := adjust(poison, -28)
		fillEllipsePixels(pixels, w, h, 60, 42, 2, 3, poison)
		fillEllipsePixels(pixels, w, h, 60, 45, 1, 1, poisonDark)
		fillEllipsePixels(pixels, w, h, 56, 48, 2, 2, poison)
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

// makeBatPixels paints a wing-spread cave bat: dark body, red eyes, scalloped
// wing membranes. Sized 80x88 (loadEnemyVisuals) so wings nearly fill horizontally.
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

	// Wing membranes: three overlapping triangles per side, drawn before the body.
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

	// Eyes — red dots.
	fillEllipsePixels(pixels, w, h, cx-3, bodyTop+8, 1, 1, eye)
	fillEllipsePixels(pixels, w, h, cx+3, bodyTop+8, 1, 1, eye)
	// Tiny fangs — two single-pixel triangles at the bottom of the face.
	fillTrianglePixels(pixels, w, h, cx-2, bodyTop+13, cx-1, bodyTop+15, cx, bodyTop+13, fang)
	fillTrianglePixels(pixels, w, h, cx+2, bodyTop+13, cx+1, bodyTop+15, cx, bodyTop+13, fang)

	// Cast shadow blob under the bat for contact.
	fillEllipsePixels(pixels, w, h, cx, h-6, 14, 3, color.RGBA{R: 0, G: 0, B: 0, A: 90})

	// Per-pixel dither for the pixel-art feel.
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

// makeGoblinPixels paints a pot-bellied green goblin with loincloth, club,
// pointed ears, yellow eyes. Sized 72×112 (loadEnemyVisuals).
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

	// Body — pot belly: lower ellipse + chest above.
	fillEllipsePixels(pixels, w, h, cx, 64, 19, 14, skin)
	fillEllipsePixels(pixels, w, h, cx-4, 60, 14, 8, skinLight)
	fillEllipsePixels(pixels, w, h, cx, 70, 18, 8, skinDark)

	// Arms — left hangs free, right grips the club.
	fillEllipsePixels(pixels, w, h, cx-22, 60, 5, 14, skin)
	fillEllipsePixels(pixels, w, h, cx-22, 74, 4, 4, skin) // hand
	fillEllipsePixels(pixels, w, h, cx+22, 56, 5, 12, skin)
	fillEllipsePixels(pixels, w, h, cx+22, 68, 4, 4, skin) // grip hand

	// Club — diagonal shaft + knob on the right.
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

	// Per-pixel dither.
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
// glowing staff, face peeking from the hood.
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

	// Hood — dark outer shell, lighter inner shadow, small face hole.
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

// makeAmoebaPixels paints a squat gel blob: outer halo, bright core, dark
// nucleus, grit specks. Squashed silhouette reads as a tank, not a jellyfish.
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

	// Pseudopod bulges.
	fillEllipsePixels(pixels, w, h, cx-30, cy+8, 6, 4, mid)
	fillEllipsePixels(pixels, w, h, cx+28, cy-4, 7, 5, mid)
	fillEllipsePixels(pixels, w, h, cx+12, cy+14, 6, 4, mid)

	// Nucleus — dark center with a specular dot.
	fillEllipsePixels(pixels, w, h, cx-2, cy+2, 9, 7, nucleus)
	fillEllipsePixels(pixels, w, h, cx-4, cy, 4, 3, nucleusDark)
	fillEllipsePixels(pixels, w, h, cx-6, cy-2, 2, 1, highlight)

	// Floating grit specks.
	fillRectPixels(pixels, w, h, cx+8, cy-2, 1, 1, grit)
	fillRectPixels(pixels, w, h, cx-12, cy+8, 1, 1, grit)
	fillRectPixels(pixels, w, h, cx+14, cy+8, 1, 1, grit)
	fillRectPixels(pixels, w, h, cx-18, cy-6, 1, 1, grit)
	fillRectPixels(pixels, w, h, cx+20, cy+4, 1, 1, grit)

	// Top-edge specular strip so the blob reads as wet.
	for ox := -16; ox <= 16; ox++ {
		x := cx - 2 + ox
		y := cy - 14 + int(math.Abs(float64(ox))/4)
		if x >= 0 && x < w && y >= 0 && y < h {
			pixels[y*w+x] = highlight
		}
	}

	// Subtle dither (less than other sprites).
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

// makeVenusMantrapPixels paints a Venus-flytrap-on-a-stem: leafy base, green
// stalk, two pink toothed jaws flaring open. Top-heavy silhouette.
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

	// Stem.
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

// makeCaveSpiderPixels paints a low-slung spider: bulbous purple abdomen,
// smaller cephalothorax, eight jointed legs, six red eyes, fangs. Wide+short (88×72).
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

	// Eight legs in four jointed pairs; each a two-segment poly-line (shoulder
	// up+out, knee down to a foot). Outer legs reach further.
	legY := h - 18 // shoulder anchor
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

	// Abdomen — bulbous oval at the back, two-tone for 3D.
	fillEllipsePixels(pixels, w, h, cx, h-22, 24, 16, bodyDark)
	fillEllipsePixels(pixels, w, h, cx, h-24, 22, 14, body)
	fillEllipsePixels(pixels, w, h, cx-4, h-28, 12, 6, bodyLight)
	// Faint marking on the abdomen — a pale crescent.
	fillEllipsePixels(pixels, w, h, cx+2, h-22, 8, 3, bodyLight)

	// Cephalothorax — smaller front body.
	fillEllipsePixels(pixels, w, h, cx, h-38, 14, 10, bodyDark)
	fillEllipsePixels(pixels, w, h, cx, h-40, 12, 8, body)
	fillEllipsePixels(pixels, w, h, cx-2, h-42, 8, 4, bodyLight)

	// Six red eyes (3+3), outer smaller, glow halo behind each.
	for _, e := range [][3]int{
		{cx - 6, h - 41, 2}, {cx, h - 41, 2}, {cx + 6, h - 41, 2},
		{cx - 4, h - 37, 1}, {cx, h - 37, 1}, {cx + 4, h - 37, 1},
	} {
		fillEllipsePixels(pixels, w, h, e[0], e[1], e[2]+1, e[2]+1, eyeGlow)
		fillEllipsePixels(pixels, w, h, e[0], e[1], e[2], e[2], eye)
	}

	// Mandibles / fangs.
	fillTrianglePixels(pixels, w, h, cx-5, h-32, cx-2, h-32, cx-3, h-28, fang)
	fillTrianglePixels(pixels, w, h, cx+5, h-32, cx+2, h-32, cx+3, h-28, fang)

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

// makeVampireBatPixels paints a larger crimson-black bat with glowing red eyes,
// fangs, and a mouth blood-drip. Wing silhouette mirrors the cave bat.
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

	// Left wing membranes.
	fillTrianglePixels(pixels, w, h, cx-2, 36, cx-44, 28, cx-30, 60, wingMembrane)
	fillTrianglePixels(pixels, w, h, cx-2, 36, cx-30, 60, cx-12, 56, wingMembrane)
	// Inner-membrane shading.
	fillTrianglePixels(pixels, w, h, cx-2, 40, cx-26, 44, cx-12, 56, wingDark)
	// Wing-finger bones.
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

	// Eyes — big glowing red.
	fillEllipsePixels(pixels, w, h, cx-4, 30, 3, 3, eyeGlow)
	fillEllipsePixels(pixels, w, h, cx+4, 30, 3, 3, eyeGlow)
	fillEllipsePixels(pixels, w, h, cx-4, 30, 2, 2, eye)
	fillEllipsePixels(pixels, w, h, cx+4, 30, 2, 2, eye)

	// Fangs — two pointed white teeth below the mouth.
	fillTrianglePixels(pixels, w, h, cx-3, 36, cx-1, 36, cx-2, 41, fang)
	fillTrianglePixels(pixels, w, h, cx+3, 36, cx+1, 36, cx+2, 41, fang)

	// Blood drip from one fang.
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

// makeWispPixels paints a floating cyan-white orb with halo rings and trailing
// tendrils. No solid body. Narrow+tall (56×72).
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
	cy := h/2 - 8 // float anchor above center

	// Soft ground shadow — small, since the wisp floats.
	fillEllipsePixels(pixels, w, h, cx, h-4, 14, 3, color.RGBA{R: 0, G: 0, B: 0, A: 60})

	// Outer mist tendrils trailing down, at varied y offsets.
	fillEllipsePixels(pixels, w, h, cx-3, cy+14, 7, 9, mist2)
	fillEllipsePixels(pixels, w, h, cx+4, cy+18, 5, 7, mist2)
	fillEllipsePixels(pixels, w, h, cx, cy+22, 4, 6, tendrilDim)
	fillEllipsePixels(pixels, w, h, cx-2, cy+28, 3, 5, tendrilDim)

	// Halo layers — concentric rings, painted big-to-small so inner overwrites outer.
	fillEllipsePixels(pixels, w, h, cx, cy, 16, 18, mist1)
	fillEllipsePixels(pixels, w, h, cx, cy, 13, 14, mist2)
	fillEllipsePixels(pixels, w, h, cx, cy, 10, 11, core3)
	fillEllipsePixels(pixels, w, h, cx, cy, 7, 8, core2)
	fillEllipsePixels(pixels, w, h, cx, cy-1, 4, 5, core1)

	// Two dark eye pinpricks so the wisp reads as faintly malevolent.
	fillEllipsePixels(pixels, w, h, cx-2, cy-1, 1, 1, eye)
	fillEllipsePixels(pixels, w, h, cx+2, cy-1, 1, 1, eye)

	// Side-trailing arcs.
	drawLinePixels(pixels, w, h, cx-10, cy+4, cx-16, cy+12, tendril, 2)
	drawLinePixels(pixels, w, h, cx+10, cy+4, cx+16, cy+12, tendril, 2)
	drawLinePixels(pixels, w, h, cx-16, cy+12, cx-14, cy+18, tendrilDim, 2)
	drawLinePixels(pixels, w, h, cx+16, cy+12, cx+14, cy+18, tendrilDim, 2)

	// No dither — the wisp's glow is intentionally a smooth gradient.
	return pixels
}

// makeStoneGolemPixels paints a blocky stone humanoid: glowing eye slit, square
// shoulders, heavy arms, cracked-stone detail. Biggest sprite (96×120).
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

	// Legs — two thick pillars, tapered at the ankle.
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
	// Chest cracks.
	drawLinePixels(pixels, w, h, cx-12, 48, cx-4, 60, crack, 1)
	drawLinePixels(pixels, w, h, cx-4, 60, cx+2, 70, crack, 1)
	drawLinePixels(pixels, w, h, cx+8, 50, cx+14, 64, crack, 1)
	drawLinePixels(pixels, w, h, cx-18, 64, cx-10, 74, crack, 1)

	// Arms — square blocks at the sides. Left arm.
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

	// Moss patches on shoulders + feet.
	fillEllipsePixels(pixels, w, h, cx-22, 46, 4, 2, moss)
	fillEllipsePixels(pixels, w, h, cx+22, 46, 4, 2, moss)
	fillEllipsePixels(pixels, w, h, cx-18, 112, 4, 1, moss)
	fillEllipsePixels(pixels, w, h, cx+18, 112, 4, 1, moss)

	// Heavy stone-grain dither (denser) so the surface reads as rock.
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

// makeNecromancerPixels paints a tall hooded figure in indigo robes: skull face
// with glowing green sockets, bone staff with skull topper. Tall+narrow (72×112).
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

	// Robe skirt — wide triangle, dark under, lighter left edge for folds.
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

	// Sleeves flaring at the shoulders. Left sleeve.
	fillTrianglePixels(pixels, w, h, cx-14, 50, cx-22, 72, cx-12, 76, robe)
	fillTrianglePixels(pixels, w, h, cx-14, 50, cx-22, 72, cx-18, 60, robeDark)
	// Right sleeve.
	fillTrianglePixels(pixels, w, h, cx+14, 50, cx+22, 72, cx+12, 76, robe)
	fillTrianglePixels(pixels, w, h, cx+14, 50, cx+18, 60, cx+22, 72, robeLight)

	// Hood — dark cone framing the face.
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
	// Outer green eye glow through the hood shadow.
	fillEllipsePixels(pixels, w, h, cx-3, 30, 3, 3, color.RGBA{R: eyeGlow.R, G: eyeGlow.G, B: eyeGlow.B, A: 90})
	fillEllipsePixels(pixels, w, h, cx+3, 30, 3, 3, color.RGBA{R: eyeGlow.R, G: eyeGlow.G, B: eyeGlow.B, A: 90})
	// Nasal cavity — small triangle below the eyes.
	fillTrianglePixels(pixels, w, h, cx-1, 34, cx+1, 34, cx, 37, eyeSocket)
	// Tooth line — five small dark notches at the jaw.
	for i := 0; i < 5; i++ {
		px := cx - 4 + i*2
		fillRectPixels(pixels, w, h, px, 38, 1, 2, eyeSocket)
	}

	// Staff — diagonal shaft with skull topper.
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

// makeSkeletonPixels paints a skeleton: skull with dim-red sockets, ribcage,
// claw-finger arms, femur/tibia legs. Humanoid (72×112), goblin-sized.
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

	// Pelvis — flared boomerang with central socket.
	fillTrianglePixels(pixels, w, h, cx-12, 70, cx+12, 70, cx, 80, boneDark)
	fillTrianglePixels(pixels, w, h, cx-10, 70, cx+10, 70, cx, 77, bone)
	fillEllipsePixels(pixels, w, h, cx, 75, 3, 3, socket)

	// Spine — vertical chain of small dark bone segments.
	for sy := 44; sy < 70; sy += 4 {
		fillEllipsePixels(pixels, w, h, cx, sy, 2, 2, boneDark)
		fillEllipsePixels(pixels, w, h, cx, sy-1, 1, 1, bone)
	}

	// Rib cage — five ribs per side, as small ellipses.
	for i, sy := range []int{46, 50, 54, 58, 62} {
		span := 12 - i // outer ribs reach further
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

	// Arms — humerus + radius/ulna with claw hands. Left arm.
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

	// Skull — wider than tall, recessed jaw.
	fillEllipsePixels(pixels, w, h, cx, 28, 12, 13, boneDark)
	fillEllipsePixels(pixels, w, h, cx, 26, 10, 11, bone)
	fillEllipsePixels(pixels, w, h, cx-1, 22, 7, 5, boneLight)
	// Eye sockets with glowing red.
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

	// Per-pixel dither (denser) so bone reads as pitted.
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
	minX := min(x1, min(x2, x3))
	maxX := max(x1, max(x2, x3))
	minY := min(y1, min(y2, y3))
	maxY := max(y1, max(y2, y3))
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

// hashByteXY is hashXY masked to a byte, for "% N" bucketing in pixel loops.
func hashByteXY(x, y int) int {
	return int(hashXY(x, y) & 0xff)
}

func jitter(c color.RGBA, x, y, amount int) color.RGBA {
	return adjust(c, hashByteXY(x, y)%(amount*2+1)-amount)
}

func adjust(c color.RGBA, delta int) color.RGBA {
	return mapRGB(c, func(v uint8) uint8 {
		return core.ClampByte(int(v) + delta)
	})
}

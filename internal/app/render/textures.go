package render

import (
	"image/color"
	"math"

	"crawler/internal/app/core"
)

func makeRockWallPixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	base := color.RGBA{R: 96, G: 100, B: 94, A: 255}
	shadow := color.RGBA{R: 44, G: 48, B: 46, A: 255}
	highlight := color.RGBA{R: 138, G: 140, B: 128, A: 255}
	moss := color.RGBA{R: 70, G: 102, B: 68, A: 255}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			n := fbmNoise(float64(x), float64(y), 0.038, 4)
			c := base
			c = core.MixColor(c, highlight, math.Max(0, n)*0.55)
			c = core.MixColor(c, shadow, math.Max(0, -n)*0.55)

			cellX, cellY := x/24, y/24
			cellOffset := hash2(cellX, cellY) % 6
			if (x+cellOffset)%24 < 2 || (y+cellOffset)%24 < 2 {
				c = core.MixColor(c, shadow, 0.55)
			}
			if hash2(x/4, y/4)%23 == 0 {
				c = core.MixColor(c, shadow, 0.45)
			}
			if y > h*4/7 && hash2(x/3, y/2)%14 < 3 {
				c = core.MixColor(c, moss, 0.40)
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
	base := color.RGBA{R: 118, G: 120, B: 118, A: 255}
	warm := color.RGBA{R: 156, G: 138, B: 110, A: 255}
	cool := color.RGBA{R: 86, G: 96, B: 110, A: 255}
	mortarColor := color.RGBA{R: 32, G: 32, B: 36, A: 255}
	mortarLight := color.RGBA{R: 78, G: 76, B: 70, A: 255}
	moss := color.RGBA{R: 68, G: 96, B: 64, A: 255}

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
				c := core.MixColor(mortarColor, mortarLight, float64(hash2(x, y)%64)/200.0)
				if hash2(x/2, y) < 20 {
					c = core.MixColor(c, moss, 0.18)
				}
				pixels[y*w+x] = c
				continue
			}

			brickX := (x + offset) / brickW
			tone := hash2(brickX*7, row*13) % 100
			c := base
			if tone < 32 {
				c = core.MixColor(c, warm, 0.25+float64(tone)/200.0)
			} else if tone < 60 {
				c = core.MixColor(c, cool, 0.18+float64(tone-32)/200.0)
			}

			n := fbmNoise(float64(x)*1.4, float64(y)*1.4, 0.16, 4)
			c = core.MixColor(c, color.RGBA{R: 244, G: 238, B: 226, A: 255}, math.Max(0, n)*0.32)
			c = core.MixColor(c, color.RGBA{R: 30, G: 28, B: 24, A: 255}, math.Max(0, -n)*0.42)

			edgeDist := minInt(localX-mortar, minInt(localY-mortar, minInt(brickW-mortar-1-localX, brickH-mortar-1-localY)))
			if edgeDist <= 2 {
				c = core.MixColor(c, mortarColor, 0.45-float64(edgeDist)*0.12)
			}

			if hash2(brickX*17+localX/3, row*31+localY/3)%80 < 4 {
				c = adjust(c, -36)
			}
			if (localY > brickH-4) && hash2(x, y)%18 < 5 {
				c = core.MixColor(c, moss, 0.32)
			}
			pixels[y*w+x] = c
		}
	}
	return pixels
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func makeGrassPixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	base := color.RGBA{R: 64, G: 132, B: 60, A: 255}
	light := color.RGBA{R: 122, G: 178, B: 88, A: 255}
	dark := color.RGBA{R: 36, G: 90, B: 44, A: 255}
	dirt := color.RGBA{R: 102, G: 82, B: 56, A: 255}
	bloom := color.RGBA{R: 232, G: 220, B: 110, A: 255}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			n := fbmNoise(float64(x), float64(y), 0.075, 4)
			m := fbmNoise(float64(x)+512, float64(y)-271, 0.022, 3)
			c := base
			c = core.MixColor(c, light, math.Max(0, n)*0.65)
			c = core.MixColor(c, dark, math.Max(0, -n)*0.55)
			if m > 0.42 {
				c = core.MixColor(c, dirt, (m-0.42)*0.85)
			}
			if hash2(x/2, y/2)%9 == 0 {
				bx, by := x%4, y%4
				if (bx == 0 && by == 0) || (bx == 2 && by == 1) {
					c = core.MixColor(c, light, 0.4)
				}
			}
			if hash2(x*7, y*11)%320 < 3 {
				c = core.MixColor(c, bloom, 0.7)
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
	base := color.RGBA{R: 92, G: 92, B: 96, A: 255}
	warm := color.RGBA{R: 130, G: 116, B: 96, A: 255}
	cold := color.RGBA{R: 64, G: 76, B: 96, A: 255}
	groutColor := color.RGBA{R: 26, G: 26, B: 30, A: 255}
	highlight := color.RGBA{R: 212, G: 208, B: 198, A: 255}

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
			tone := hash2(slabX*5, slabY*7) % 100
			c := base
			if tone < 38 {
				c = core.MixColor(c, warm, 0.18+float64(tone)/240.0)
			} else if tone < 70 {
				c = core.MixColor(c, cold, 0.16+float64(tone-38)/260.0)
			}

			n := fbmNoise(float64(x)*1.6, float64(y)*1.6, 0.18, 4)
			c = core.MixColor(c, highlight, math.Max(0, n)*0.32)
			c = core.MixColor(c, color.RGBA{R: 24, G: 22, B: 20, A: 255}, math.Max(0, -n)*0.40)

			edgeDist := minInt(localX-grout, minInt(localY-grout, minInt(slab-1-localX, slab-1-localY)))
			if edgeDist <= 3 {
				c = core.MixColor(c, groutColor, 0.45-float64(edgeDist)*0.10)
			}
			if hash2(slabX*11+localX/4, slabY*19+localY/4)%72 < 3 {
				c = adjust(c, -32)
			}
			pixels[y*w+x] = c
		}
	}
	return pixels
}

// makeDirtPixels paints a brown earth texture for dirt patches mixed into the
// field's grass. Same FBM-noise spine as grass, but warmer base / lower
// chroma so it reads as bare earth.
func makeDirtPixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	base := color.RGBA{R: 116, G: 86, B: 56, A: 255}
	light := color.RGBA{R: 162, G: 124, B: 80, A: 255}
	dark := color.RGBA{R: 76, G: 54, B: 36, A: 255}
	pebble := color.RGBA{R: 140, G: 130, B: 116, A: 255}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			n := fbmNoise(float64(x)+91, float64(y)-203, 0.078, 4)
			c := base
			c = core.MixColor(c, light, math.Max(0, n)*0.55)
			c = core.MixColor(c, dark, math.Max(0, -n)*0.55)
			if hash2(x*5, y*5)%160 < 3 {
				c = core.MixColor(c, dark, 0.6)
			}
			if hash2(x*7, y*11)%280 < 2 {
				c = core.MixColor(c, pebble, 0.7)
			}
			pixels[y*w+x] = c
		}
	}
	return pixels
}

// makeDarkGrassPixels paints a deeper-green grass texture for shaded patches
// of the field. Slightly higher contrast than the regular grass and a cooler
// hue so the variation reads as "in-shadow / damp" rather than just "darker".
func makeDarkGrassPixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	base := color.RGBA{R: 38, G: 92, B: 50, A: 255}
	light := color.RGBA{R: 84, G: 138, B: 70, A: 255}
	dark := color.RGBA{R: 18, G: 56, B: 32, A: 255}
	moss := color.RGBA{R: 70, G: 124, B: 92, A: 255}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			n := fbmNoise(float64(x)+411, float64(y)+97, 0.072, 4)
			m := fbmNoise(float64(x)-227, float64(y)+311, 0.022, 3)
			c := base
			c = core.MixColor(c, light, math.Max(0, n)*0.55)
			c = core.MixColor(c, dark, math.Max(0, -n)*0.55)
			if m > 0.45 {
				c = core.MixColor(c, moss, (m-0.45)*0.6)
			}
			if hash2(x*3, y*3)%9 == 0 {
				bx, by := x%4, y%4
				if (bx == 1 && by == 0) || (bx == 3 && by == 2) {
					c = core.MixColor(c, light, 0.30)
				}
			}
			pixels[y*w+x] = c
		}
	}
	return pixels
}

func makeBarkPixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	base := color.RGBA{R: 96, G: 64, B: 38, A: 255}
	deep := color.RGBA{R: 46, G: 30, B: 18, A: 255}
	light := color.RGBA{R: 152, G: 110, B: 70, A: 255}
	moss := color.RGBA{R: 80, G: 110, B: 70, A: 255}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			ridge := math.Sin(float64(x)*0.45 + fbmNoise(float64(x), float64(y), 0.05, 3)*4.5)
			ridge = math.Abs(ridge)
			n := fbmNoise(float64(x)*0.9, float64(y)*0.4, 0.22, 4)
			c := base
			c = core.MixColor(c, light, math.Max(0, ridge-0.45)*1.4)
			c = core.MixColor(c, deep, math.Max(0, 0.4-ridge)*0.8)
			c = core.MixColor(c, light, math.Max(0, n)*0.25)
			c = core.MixColor(c, deep, math.Max(0, -n)*0.40)

			if hash2(x/2, y/2)%160 < 3 {
				c = core.MixColor(c, deep, 0.7)
			}
			if hash2(x*3, y/4)%240 < 2 {
				c = core.MixColor(c, moss, 0.55)
			}
			pixels[y*w+x] = c
		}
	}
	return pixels
}

func makeLeafPixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	base := color.RGBA{R: 86, G: 152, B: 90, A: 255}
	light := color.RGBA{R: 158, G: 208, B: 124, A: 255}
	deep := color.RGBA{R: 38, G: 86, B: 50, A: 255}
	gold := color.RGBA{R: 218, G: 220, B: 130, A: 255}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			n := fbmNoise(float64(x)*1.2, float64(y)*1.2, 0.16, 4)
			m := fbmNoise(float64(x)+183, float64(y)-77, 0.05, 3)
			c := base
			c = core.MixColor(c, light, math.Max(0, n)*0.7)
			c = core.MixColor(c, deep, math.Max(0, -n)*0.65)
			if m > 0.55 {
				c = core.MixColor(c, gold, (m-0.55)*0.6)
			}
			if hash2(x*5, y*5)%180 < 3 {
				c = core.MixColor(c, deep, 0.55)
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
	return (x1 + v*(x2-x1)) * 2.0 - 1.0
}

func hashFloat(x, y int) float64 {
	n := uint32(x)*uint32(73856093) ^ uint32(y)*uint32(19349663)
	n = (n ^ (n >> 13)) * 1274126177
	n ^= n >> 16
	return float64(n&0xFFFF) / 65535.0
}

func makeSkyPixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	top := color.RGBA{R: 64, G: 150, B: 238, A: 255}
	horizon := color.RGBA{R: 190, G: 229, B: 255, A: 255}
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
				c = core.MixColor(c, color.RGBA{R: 249, G: 252, B: 255, A: 255}, cover)
			}
			pixels[y*w+x] = c
		}
	}
	return pixels
}

func makeRatPixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	body := color.RGBA{R: 104, G: 107, B: 104, A: 255}
	bodyDark := color.RGBA{R: 68, G: 72, B: 72, A: 255}
	bodyLight := color.RGBA{R: 138, G: 142, B: 136, A: 255}
	ear := color.RGBA{R: 172, G: 116, B: 122, A: 255}
	tail := color.RGBA{R: 178, G: 118, B: 125, A: 255}
	eye := color.RGBA{R: 10, G: 12, B: 12, A: 255}
	nose := color.RGBA{R: 232, G: 150, B: 162, A: 255}

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

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*w + x
			if pixels[i].A == 0 {
				continue
			}
			if hash2(x, y)%7 == 0 {
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
			if hash2(x, y)%9 == 0 {
				pixels[i] = adjust(pixels[i], -10)
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
			if hash2(x+presentation.textureSeed*17, y)%9 == 0 {
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

func hash2(x, y int) int {
	n := uint32(x*73856093) ^ uint32(y*19349663)
	n ^= n >> 13
	n *= 1274126177
	n ^= n >> 16
	return int(n & 0xff)
}

func jitter(c color.RGBA, x, y, amount int) color.RGBA {
	return adjust(c, hash2(x, y)%(amount*2+1)-amount)
}

func adjust(c color.RGBA, delta int) color.RGBA {
	return color.RGBA{
		R: core.ClampByte(int(c.R) + delta),
		G: core.ClampByte(int(c.G) + delta),
		B: core.ClampByte(int(c.B) + delta),
		A: c.A,
	}
}

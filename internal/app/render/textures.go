package render

import (
	"image/color"
	"math"

	"crawler/internal/app/core"
)

func makeStoneBrickPixels(w, h int) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	brickW := 32
	brickH := 16
	mortar := 2
	base := color.RGBA{R: 106, G: 112, B: 110, A: 255}
	mortarColor := color.RGBA{R: 48, G: 51, B: 53, A: 255}

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
				pixels[y*w+x] = jitter(mortarColor, x, y, 8)
				continue
			}

			brickX := (x + offset) / brickW
			variation := hash2(brickX, row)%28 - 14
			edge := 0
			if localX < mortar+3 || localY < mortar+3 || localX > brickW-mortar-4 || localY > brickH-mortar-4 {
				edge = -15
			}
			c := adjust(base, variation+edge+(hash2(x, y)%13)-6)
			if hash2(brickX*17+localX/3, row*31+localY/3) < 5 {
				c = adjust(c, -28)
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
	base := color.RGBA{R: 76, G: 78, B: 80, A: 255}
	groutColor := color.RGBA{R: 38, G: 40, B: 42, A: 255}

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
			variation := hash2(slabX, slabY)%24 - 12
			edge := 0
			if localX < grout+3 || localY < grout+3 {
				edge = -10
			}
			c := adjust(base, variation+edge+(hash2(x, y)%17)-8)
			if hash2(slabX*11+localX/4, slabY*19+localY/4) < 4 {
				c = adjust(c, -30)
			}
			pixels[y*w+x] = c
		}
	}
	return pixels
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

func makePartyPixels(w, h int, class core.PartyClass) []color.RGBA {
	pixels := make([]color.RGBA, w*h)
	shadow := color.RGBA{R: 0, G: 0, B: 0, A: 78}
	skin := color.RGBA{R: 219, G: 165, B: 124, A: 255}
	boot := color.RGBA{R: 33, G: 34, B: 42, A: 255}

	fillEllipsePixels(pixels, w, h, 32, 73, 18, 4, shadow)
	fillRectPixels(pixels, w, h, 23, 57, 9, 12, boot)
	fillRectPixels(pixels, w, h, 33, 57, 9, 12, boot)
	fillEllipsePixels(pixels, w, h, 31, 38, 7, 7, skin)

	switch class {
	case core.ClassWarrior:
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
	case core.ClassCleric:
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
	case core.ClassThief:
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
	case core.ClassWizard:
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
	default:
		fillRectPixels(pixels, w, h, 18, 36, 28, 30, color.RGBA{R: 110, G: 110, B: 120, A: 255})
		fillEllipsePixels(pixels, w, h, 32, 24, 14, 14, color.RGBA{R: 90, G: 70, B: 55, A: 255})
	}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*w + x
			if pixels[i].A == 0 {
				continue
			}
			if hash2(x+partyClassTextureSeed(class)*17, y)%9 == 0 {
				pixels[i] = adjust(pixels[i], -10)
			}
		}
	}
	return pixels
}

func partyClassTextureSeed(class core.PartyClass) int {
	switch class {
	case core.ClassWarrior:
		return 1
	case core.ClassCleric:
		return 2
	case core.ClassThief:
		return 3
	case core.ClassWizard:
		return 4
	default:
		return 0
	}
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

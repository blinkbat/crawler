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

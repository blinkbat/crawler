package render

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"  // register decoders for image.Decode
	_ "image/jpeg" //
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
	"unsafe"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Sprite editor: bakes destructive edits into maps/sprites/<slug>.png (loaded at
// boot) or imports a PNG. Every bake first copies the PNG to <slug>.png.bak for
// one-step Restore. All image work pairs with UnloadImage.

// SpriteFilter is one destructive edit pass. Zero value is a no-op; enabled ops
// apply in order: tint, brightness, contrast, grayscale, invert, gradient,
// pixelate. Each bake re-reads the PNG, so re-clicking an op compounds it.
type SpriteFilter struct {
	TintApply  bool     // multiply by Tint (ImageColorTint)
	Tint       rl.Color //
	Brightness int32    // -255..255 (ImageColorBrightness); 0 = skip
	Contrast   float32  // -100..100 (ImageColorContrast); 0 = skip
	Grayscale  bool     // ImageColorGrayscale
	Invert     bool     // ImageColorInvert
	Gradient   bool     // alpha-blend a vertical gradient
	GradTop    rl.Color // gradient top color (alpha < 255 for a wash)
	GradBottom rl.Color // gradient bottom color
	// Pixelate: NN down-and-up block size in source px; <=1 = skip. Baked.
	Pixelate int32
	// Palette/retro passes (CPU per-pixel; mirror retrofilter.go's shader math so
	// bake and screen filter agree). Posterize 0..1 (48→4 levels); Saturation -1..1;
	// Dither 0..1 Bayer; GameBoy 0..1 green ramp. 0 = skip each.
	Posterize  float32
	Saturation float32
	Dither     float32
	GameBoy    float32
	// MaxColors caps the palette via median-cut (distinct from Posterize's per-
	// channel crush). Rounded; <2 = skip. Applied after tonal passes, BEFORE pixelate.
	MaxColors int32
}

// IsNoop reports whether the filter changes nothing (gates a stray write + .bak).
func (f SpriteFilter) IsNoop() bool {
	return !f.TintApply && f.Brightness == 0 && f.Contrast == 0 &&
		!f.Grayscale && !f.Invert && !f.Gradient && f.Pixelate <= 1 &&
		f.Posterize == 0 && f.Saturation == 0 && f.Dither == 0 && f.GameBoy == 0 &&
		f.MaxColors < 2
}

// BakeSpriteFilter applies f to the foe's sprite, writing <slug>.png (backing up
// first). Source is the PNG, else the live texture read back (promoting a
// procedural foe to an editable PNG). No-op for an empty filter.
func BakeSpriteFilter(assets Resources, kind core.EnemyKind, f SpriteFilter) error {
	if f.IsNoop() {
		return nil
	}
	img, err := loadEditableSpriteImage(assets, kind)
	if err != nil {
		return err
	}
	defer rl.UnloadImage(img)
	applySpriteFilter(img, f)
	return exportSpritePNG(kind, img)
}

// ImportSpriteFromFile decodes an image and writes it as the foe's <slug>.png
// (backing up first). An import with no transparency gets its opaque matte
// border-flood-keyed to alpha (the billboard tint needs a real alpha channel).
func ImportSpriteFromFile(kind core.EnemyKind, srcPath string) error {
	img, err := decodeToNRGBA(srcPath)
	if err != nil {
		return err
	}
	keyOutOpaqueMatte(img)
	return writeSpritePNG(kind, img)
}

// decodeToNRGBA loads srcPath into a straight-alpha image.NRGBA: Go's decoders
// (png/jpeg/gif) first, else raylib's loader (bmp/tga/psd/hdr/pnm/…).
func decodeToNRGBA(srcPath string) (*image.NRGBA, error) {
	if data, err := os.ReadFile(srcPath); err == nil {
		if src, _, derr := image.Decode(bytes.NewReader(data)); derr == nil {
			b := src.Bounds()
			if b.Dx() > 0 && b.Dy() > 0 {
				// NRGBA = straight (non-premultiplied) alpha.
				dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
				draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
				return dst, nil
			}
		}
	}
	// Fallback: raylib decodes formats stdlib can't; copy straight-alpha pixels into
	// NRGBA via one LoadImageColors (not a per-pixel cgo crossing).
	rimg := rl.LoadImage(srcPath)
	if rimg == nil || rimg.Width <= 0 || rimg.Height <= 0 {
		if rimg != nil {
			rl.UnloadImage(rimg)
		}
		return nil, fmt.Errorf("not a loadable image: %s", filepath.Base(srcPath))
	}
	defer rl.UnloadImage(rimg)
	rl.ImageFormat(rimg, rl.UncompressedR8g8b8a8)
	w, h := int(rimg.Width), int(rimg.Height)
	colors := rl.LoadImageColors(rimg)
	defer rl.UnloadImageColors(colors)
	if len(colors) < w*h {
		return nil, fmt.Errorf("could not read pixels: %s", filepath.Base(srcPath))
	}
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < w*h; i++ {
		c := colors[i]
		o := i * 4
		dst.Pix[o], dst.Pix[o+1], dst.Pix[o+2], dst.Pix[o+3] = c.R, c.G, c.B, c.A
	}
	return dst, nil
}

// writeSpritePNG backs up then writes img to <slug>.png. Go-image sibling of exportSpritePNG.
func writeSpritePNG(kind core.EnemyKind, img image.Image) error {
	return writeSpritePNGSlug(core.EnemySlug(kind), img)
}

// writeSpritePNGSlug is the slug-keyed core for foe/party imports. Encodes to a
// buffer first so a failed encode can't truncate the sprite.
func writeSpritePNGSlug(slug string, img image.Image) error {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return fmt.Errorf("encode failed: %w", err)
	}
	path, err := prepareSpriteWrite(slug)
	if err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), core.AssetFileMode)
}

// prepareSpriteWrite ensures the sprites dir, backs up <slug>.png to .bak, returns
// the dest path. Callers must have encoded bytes ready first so a write failure
// can't truncate a sprite this already backed up.
func prepareSpriteWrite(slug string) (path string, err error) {
	dir := core.ResolveAssetDir(core.SpritesDirName)
	if err := os.MkdirAll(dir, core.AssetDirMode); err != nil {
		return "", err
	}
	path = spritePathSlug(slug)
	if _, statErr := os.Stat(path); statErr == nil {
		_ = copyFile(path, path+".bak") // best-effort safety net
	}
	// The PNG is about to change; drop any cached pristine base for this slug so the
	// next preview reload reads the new file (single-threaded: no draw runs mid-write).
	invalidateAssetPreviewBase(slug)
	return path, nil
}

// Matte-keying tunables for keyOutOpaqueMatte: matteTol = per-channel RGB distance
// counting as matte; matteAlphaFloor = alpha below which a pixel is transparent;
// matteTransparentPct (*50 ⇒ 2%) = already-transparent threshold to leave it alone.
const (
	matteTol            = 40
	matteAlphaFloor     = 16
	matteTransparentPct = 50 // transparent*50 >= total ⇔ ≥2% already transparent
)

// keyOutOpaqueMatte clears an opaque background matte to transparency when the
// image has ~no alpha. Flood-fills inward from the border (so an interior region
// matching the matte can't be punched out), within tolerance of the corner color.
func keyOutOpaqueMatte(img *image.NRGBA) bool {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	total := w * h
	if total == 0 {
		return false
	}
	// ≥2% already transparent ⇒ real authored cutout, leave it alone.
	transparent := 0
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] < matteAlphaFloor {
			transparent++
		}
	}
	if transparent*matteTransparentPct >= total {
		return false
	}

	const tol = matteTol
	bg := cornerMatteColor(img, w, h)
	// Only key when the four corners agree; else flooding would erode content.
	for _, c := range [4]color.NRGBA{
		nrgbaAt(img, 0, 0), nrgbaAt(img, w-1, 0),
		nrgbaAt(img, 0, h-1), nrgbaAt(img, w-1, h-1),
	} {
		if absDiffU8(c.R, bg.R) > tol || absDiffU8(c.G, bg.G) > tol || absDiffU8(c.B, bg.B) > tol {
			return false
		}
	}
	matches := func(x, y int) bool {
		o := img.PixOffset(x, y)
		return img.Pix[o+3] > matteAlphaFloor &&
			absDiffU8(img.Pix[o], bg.R) <= tol &&
			absDiffU8(img.Pix[o+1], bg.G) <= tol &&
			absDiffU8(img.Pix[o+2], bg.B) <= tol
	}
	visited := make([]bool, total)
	stack := make([]int, 0, 256)
	push := func(x, y int) {
		if x < 0 || y < 0 || x >= w || y >= h {
			return
		}
		idx := y*w + x
		if visited[idx] {
			return
		}
		visited[idx] = true
		if matches(x, y) {
			stack = append(stack, idx)
		}
	}
	for x := 0; x < w; x++ {
		push(x, 0)
		push(x, h-1)
	}
	for y := 0; y < h; y++ {
		push(0, y)
		push(w-1, y)
	}
	cleared := 0
	for len(stack) > 0 {
		idx := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		x, y := idx%w, idx/w
		img.Pix[img.PixOffset(x, y)+3] = 0
		cleared++
		push(x+1, y)
		push(x-1, y)
		push(x, y+1)
		push(x, y-1)
	}
	return cleared > 0
}

// cornerMatteColor returns the most common of the four corners (ties → top-left).
func cornerMatteColor(img *image.NRGBA, w, h int) color.NRGBA {
	corners := [4]color.NRGBA{
		nrgbaAt(img, 0, 0),
		nrgbaAt(img, w-1, 0),
		nrgbaAt(img, 0, h-1),
		nrgbaAt(img, w-1, h-1),
	}
	best, bestCount := corners[0], 0
	for _, c := range corners {
		cnt := 0
		for _, d := range corners {
			if c == d {
				cnt++
			}
		}
		if cnt > bestCount {
			best, bestCount = c, cnt
		}
	}
	return best
}

func nrgbaAt(img *image.NRGBA, x, y int) color.NRGBA {
	o := img.PixOffset(x, y)
	return color.NRGBA{R: img.Pix[o], G: img.Pix[o+1], B: img.Pix[o+2], A: img.Pix[o+3]}
}

func absDiffU8(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}

// RestoreSpriteBackup restores <slug>.png from its .bak. Errors if none.
func RestoreSpriteBackup(kind core.EnemyKind) error {
	return restoreSpriteBackupSlug(core.EnemySlug(kind))
}

// SpriteHasBackup reports whether a restorable .bak exists.
func SpriteHasBackup(kind core.EnemyKind) bool {
	return spriteHasBackupSlug(core.EnemySlug(kind))
}

// restoreSpriteBackupSlug / spriteHasBackupSlug: slug-keyed cores for foe + party.
func restoreSpriteBackupSlug(slug string) error {
	bak := spritePathSlug(slug) + ".bak"
	if _, err := os.Stat(bak); err != nil {
		return fmt.Errorf("no backup to restore")
	}
	if err := copyFile(bak, spritePathSlug(slug)); err != nil {
		return err
	}
	// Restored PNG differs from the cached base — drop it so the preview reloads.
	invalidateAssetPreviewBase(slug)
	return nil
}

func spriteHasBackupSlug(slug string) bool {
	_, err := os.Stat(spritePathSlug(slug) + ".bak")
	return err == nil
}

// spritePathSlug is the single <slug>.png path source (foe + party).
func spritePathSlug(slug string) string {
	return filepath.Join(core.ResolveAssetDir(core.SpritesDirName), slug+".png")
}

// loadEditableSpriteImage returns an owned *rl.Image for the foe (authored PNG,
// else live texture read back). Caller must UnloadImage it.
func loadEditableSpriteImage(assets Resources, kind core.EnemyKind) (*rl.Image, error) {
	v, _ := enemyVisualFor(assets, kind)
	return loadEditableSpriteImageSlug(core.EnemySlug(kind), v.texture)
}

// loadEditableSpriteImageSlug is the slug-keyed core: authored <slug>.png, else
// the fallback texture read back. Caller must UnloadImage the result.
func loadEditableSpriteImageSlug(slug string, fallback rl.Texture2D) (*rl.Image, error) {
	path := spritePathSlug(slug)
	if _, err := os.Stat(path); err == nil {
		if img := rl.LoadImage(path); img != nil && img.Width > 0 && img.Height > 0 {
			return img, nil
		} else if img != nil {
			rl.UnloadImage(img)
		}
	}
	if fallback.ID == 0 {
		return nil, fmt.Errorf("no sprite source for %s", slug)
	}
	img := rl.LoadImageFromTexture(fallback)
	if img == nil || img.Width <= 0 || img.Height <= 0 {
		if img != nil {
			rl.UnloadImage(img)
		}
		return nil, fmt.Errorf("could not read %s texture", slug)
	}
	return img, nil
}

// applySpriteFilter runs the enabled ops in fixed order on img.
func applySpriteFilter(img *rl.Image, f SpriteFilter) {
	if f.TintApply {
		rl.ImageColorTint(img, f.Tint)
	}
	if f.Brightness != 0 {
		rl.ImageColorBrightness(img, f.Brightness)
	}
	if f.Contrast != 0 {
		rl.ImageColorContrast(img, f.Contrast)
	}
	if f.Grayscale {
		rl.ImageColorGrayscale(img)
	}
	if f.Invert {
		rl.ImageColorInvert(img)
	}
	if f.Gradient {
		grad := rl.GenImageGradientLinear(int(img.Width), int(img.Height), 0, f.GradTop, f.GradBottom)
		if grad != nil {
			// Alpha-blend the gradient, scaled to the sprite's dimensions.
			rl.ImageDraw(img, grad,
				rl.NewRectangle(0, 0, float32(grad.Width), float32(grad.Height)),
				rl.NewRectangle(0, 0, float32(img.Width), float32(img.Height)),
				rl.White)
			rl.UnloadImage(grad)
		}
	}
	// Palette/retro before pixelate so the mosaic blocks carry the result.
	applyPaletteFilter(img, f)
	if f.MaxColors >= 2 {
		quantizeImageColors(img, int(f.MaxColors))
	}
	// Pixelate last: NN down then up bakes chunky blocks; clamp to 1 so a big block
	// on a small sprite can't collapse to 0px.
	if f.Pixelate > 1 {
		w, h := img.Width, img.Height
		dw, dh := w/f.Pixelate, h/f.Pixelate
		if dw < 1 {
			dw = 1
		}
		if dh < 1 {
			dh = 1
		}
		rl.ImageResizeNN(img, dw, dh)
		rl.ImageResizeNN(img, w, h)
	}
}

// Bayer 4×4 + Game Boy green ramp — Go twins of retrofilter.go's GLSL bayer/gbRamp.
// GLSL arrays can't be shared, so a change to one MUST be mirrored in the other.
var (
	bayer4x4 = [16]float32{
		0, 8, 2, 10,
		12, 4, 14, 6,
		3, 11, 1, 9,
		15, 7, 13, 5,
	}
	gbGreenRamp = [4][3]float32{
		{0.055, 0.149, 0.055},
		{0.188, 0.384, 0.188},
		{0.545, 0.675, 0.059},
		{0.741, 0.890, 0.420},
	}
)

const ditherQuantLevels = 6.0

// applyPaletteFilter runs saturation/posterize/dither/GameBoy in one CPU pixel
// walk (skipped at zero, transparent pixels untouched). Posterize/dither/GameBoy
// use the SAME math as the retro shader so they match pixel-for-pixel; Saturation
// has no shader counterpart.
func applyPaletteFilter(img *rl.Image, f SpriteFilter) {
	sat, post, dith, gb := f.Saturation, f.Posterize, f.Dither, f.GameBoy
	if sat == 0 && post == 0 && dith == 0 && gb == 0 {
		return
	}
	mapImagePixels(img, func(x, y int, c color.RGBA) color.RGBA {
		if c.A == 0 {
			return c
		}
		r, g, b := float32(c.R)/255, float32(c.G)/255, float32(c.B)/255
		// Saturation: lerp each channel around luminance.
		if sat != 0 {
			l := lumaf(r, g, b)
			k := 1 + sat
			r, g, b = l+(r-l)*k, l+(g-l)*k, l+(b-l)*k
		}
		// Posterize: crush color depth (48→4 at full).
		if post > 0 {
			levels := core.Lerp(48, 4, post)
			r, g, b = quantizeChannel(r, levels), quantizeChannel(g, levels), quantizeChannel(b, levels)
		}
		// Ordered Bayer dither toward a 6-level quantize.
		if dith > 0 {
			t := (bayer4x4[(y%4)*4+(x%4)]+0.5)/16 - 0.5
			off := t * (1.5 / ditherQuantLevels)
			r = core.Lerp(r, quantizeChannel(r+off, ditherQuantLevels), dith)
			g = core.Lerp(g, quantizeChannel(g+off, ditherQuantLevels), dith)
			b = core.Lerp(b, quantizeChannel(b+off, ditherQuantLevels), dith)
		}
		// Game Boy: luminance onto the green ramp.
		if gb > 0 {
			step := int(core.Clamp(lumaf(r, g, b), 0, 1) * 4)
			if step > 3 {
				step = 3
			}
			r = core.Lerp(r, gbGreenRamp[step][0], gb)
			g = core.Lerp(g, gbGreenRamp[step][1], gb)
			b = core.Lerp(b, gbGreenRamp[step][2], gb)
		}
		return color.RGBA{R: toByte(r), G: toByte(g), B: toByte(b), A: c.A}
	})
}

// quantizeImageColors reduces img to ≤maxColors via median-cut: split the
// opaque-pixel color set along each bucket's widest channel, average each, snap
// pixels to nearest (cached). Transparent untouched. maxColors < 2 = no-op.
func quantizeImageColors(img *rl.Image, maxColors int) {
	if maxColors < 2 || img == nil || img.Width <= 0 || img.Height <= 0 {
		return
	}
	rl.ImageFormat(img, rl.UncompressedR8g8b8a8)
	if img.Data == nil {
		return
	}
	w, h := int(img.Width), int(img.Height)
	pix := unsafe.Slice((*uint8)(img.Data), w*h*4)

	type rgb struct{ r, g, b uint8 }
	colors := make([]rgb, 0, w*h)
	for i := 0; i < w*h; i++ {
		o := i * 4
		if pix[o+3] == 0 {
			continue
		}
		colors = append(colors, rgb{pix[o], pix[o+1], pix[o+2]})
	}
	if len(colors) == 0 {
		return
	}

	// Median cut: each box is a [lo,hi) range partitioning `colors`, so an in-place
	// sort never disturbs another. Split the widest-span box at its median.
	type box struct{ lo, hi int }
	channelRange := func(b box) (widestCh, span int) {
		mn := [3]uint8{255, 255, 255}
		mx := [3]uint8{0, 0, 0}
		for i := b.lo; i < b.hi; i++ {
			ch := [3]uint8{colors[i].r, colors[i].g, colors[i].b}
			for k := 0; k < 3; k++ {
				if ch[k] < mn[k] {
					mn[k] = ch[k]
				}
				if ch[k] > mx[k] {
					mx[k] = ch[k]
				}
			}
		}
		for k := 0; k < 3; k++ {
			if s := int(mx[k]) - int(mn[k]); s > span {
				span, widestCh = s, k
			}
		}
		return widestCh, span
	}
	boxes := []box{{0, len(colors)}}
	for len(boxes) < maxColors {
		bestIdx, bestSpan, bestCh := -1, 0, 0
		for i, b := range boxes {
			if b.hi-b.lo < 2 {
				continue
			}
			ch, span := channelRange(b)
			if bestIdx < 0 || span > bestSpan {
				bestIdx, bestSpan, bestCh = i, span, ch
			}
		}
		if bestIdx < 0 || bestSpan == 0 {
			break
		}
		b := boxes[bestIdx]
		sub := colors[b.lo:b.hi]
		sort.Slice(sub, func(a, c int) bool {
			switch bestCh {
			case 0:
				return sub[a].r < sub[c].r
			case 1:
				return sub[a].g < sub[c].g
			default:
				return sub[a].b < sub[c].b
			}
		})
		mid := b.lo + (b.hi-b.lo)/2
		boxes[bestIdx] = box{b.lo, mid}
		boxes = append(boxes, box{mid, b.hi})
	}

	// Palette = each box's average.
	palette := make([]rgb, 0, len(boxes))
	for _, b := range boxes {
		if b.hi <= b.lo {
			continue
		}
		var sr, sg, sb int
		for i := b.lo; i < b.hi; i++ {
			sr, sg, sb = sr+int(colors[i].r), sg+int(colors[i].g), sb+int(colors[i].b)
		}
		n := b.hi - b.lo
		palette = append(palette, rgb{uint8(sr / n), uint8(sg / n), uint8(sb / n)})
	}
	if len(palette) == 0 {
		return
	}

	// Snap each opaque pixel to nearest palette color (cached).
	cache := map[uint32]rgb{}
	for i := 0; i < w*h; i++ {
		o := i * 4
		if pix[o+3] == 0 {
			continue
		}
		key := uint32(pix[o])<<16 | uint32(pix[o+1])<<8 | uint32(pix[o+2])
		nc, ok := cache[key]
		if !ok {
			best, bestD := palette[0], 1<<30
			for _, p := range palette {
				dr, dg, db := int(pix[o])-int(p.r), int(pix[o+1])-int(p.g), int(pix[o+2])-int(p.b)
				if d := dr*dr + dg*dg + db*db; d < bestD {
					bestD, best = d, p
				}
			}
			nc = best
			cache[key] = nc
		}
		pix[o], pix[o+1], pix[o+2] = nc.r, nc.g, nc.b
	}
}

// mapImagePixels applies fn to every pixel in place. img is normalized to 32-bit
// RGBA first (one cgo crossing); fn gets coords (for dither) + straight-alpha color.
func mapImagePixels(img *rl.Image, fn func(x, y int, c color.RGBA) color.RGBA) {
	if img == nil || img.Width <= 0 || img.Height <= 0 {
		return
	}
	rl.ImageFormat(img, rl.UncompressedR8g8b8a8)
	if img.Data == nil {
		return
	}
	w, h := int(img.Width), int(img.Height)
	pix := unsafe.Slice((*uint8)(img.Data), w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			o := (y*w + x) * 4
			c := fn(x, y, color.RGBA{R: pix[o], G: pix[o+1], B: pix[o+2], A: pix[o+3]})
			pix[o], pix[o+1], pix[o+2], pix[o+3] = c.R, c.G, c.B, c.A
		}
	}
}

// lumaf is Rec.601 luminance of a 0..1 RGB triple (matches the shader dot).
func lumaf(r, g, b float32) float32 { return 0.299*r + 0.587*g + 0.114*b }

// quantizeChannel snaps v to the nearest of `levels` even steps.
func quantizeChannel(v, levels float32) float32 {
	return float32(math.Floor(float64(v*levels)+0.5)) / levels
}

// toByte maps a 0..1 float to a rounded, clamped 0..255 byte.
func toByte(v float32) uint8 {
	v = v*255 + 0.5
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(v)
}

// exportSpritePNG backs up then writes img to <slug>.png.
func exportSpritePNG(kind core.EnemyKind, img *rl.Image) error {
	return exportSpritePNGSlug(core.EnemySlug(kind), img)
}

// exportSpritePNGSlug is the slug-keyed core for foe/party bakes.
func exportSpritePNGSlug(slug string, img *rl.Image) error {
	path, err := prepareSpriteWrite(slug)
	if err != nil {
		return err
	}
	if !rl.ExportImage(*img, path) {
		return fmt.Errorf("export failed: %s", path)
	}
	return nil
}

// editorSpriteReloads holds textures the editor re-loaded after a bake/import/
// restore so the edit shows immediately. Keyed by slug; prior reload unloaded on
// replace. Boot textures (owned by Resources) untouched, so no double-free.
var editorSpriteReloads = map[string]rl.Texture2D{}

// closeEditorSpriteReloads frees every in-session reload texture (mirrors
// closeEditorFXTextures). Called by Resources.Unload — without it each reload's
// GPU texture would outlive the resource teardown.
func closeEditorSpriteReloads() {
	for slug, tex := range editorSpriteReloads {
		if tex.ID != 0 {
			rl.UnloadTexture(tex)
		}
		delete(editorSpriteReloads, slug)
	}
}

// reloadSpriteTextureSlug loads <slug>.png into a fresh GPU texture (mipmaps +
// trilinear + clamp, as boot), replacing any prior reload. ok=false if missing/undecodable.
func reloadSpriteTextureSlug(slug string) (rl.Texture2D, bool) {
	path := spritePathSlug(slug)
	if _, err := os.Stat(path); err != nil {
		return rl.Texture2D{}, false
	}
	tex := rl.LoadTexture(path)
	if tex.ID == 0 || tex.Width <= 0 || tex.Height <= 0 {
		if tex.ID != 0 {
			rl.UnloadTexture(tex)
		}
		return rl.Texture2D{}, false
	}
	rl.GenTextureMipmaps(&tex)
	rl.SetTextureFilter(tex, rl.FilterTrilinear)
	rl.SetTextureWrap(tex, rl.WrapClamp)
	if old, ok := editorSpriteReloads[slug]; ok {
		rl.UnloadTexture(old)
	}
	editorSpriteReloads[slug] = tex
	return tex, true
}

// ReloadFoeSprite re-reads kind's PNG into the live enemyVisuals texture so the
// change shows in-session. enemyVisuals is shared by reference through by-value
// Resources, so the write reaches preview + billboards. false if OOB or PNG won't load.
func ReloadFoeSprite(assets Resources, kind core.EnemyKind) bool {
	base, ok := visualAt(assets.enemyVisuals, int(kind))
	if !ok {
		return false
	}
	tex, ok := reloadSpriteTextureSlug(core.EnemySlug(kind))
	if !ok {
		return false
	}
	// Imported PNG is the new pristine base; drop the cached base so preview reloads.
	base.texture = tex
	base.pristineTexture = tex
	assets.enemyVisuals[kind] = base
	clearAssetPreviewBase()
	return true
}

// ReloadPartySprite is the party-class twin of ReloadFoeSprite.
func ReloadPartySprite(assets Resources, class core.PartyClass) bool {
	base, ok := visualAt(assets.partyVisuals, int(class))
	if !ok {
		return false
	}
	tex, ok := reloadSpriteTextureSlug(core.PartyClassSlug(class))
	if !ok {
		return false
	}
	base.texture = tex
	base.pristineTexture = tex
	assets.partyVisuals[class] = base
	clearAssetPreviewBase()
	return true
}

// visualAdjustFilter builds the non-destructive filter from an override (shared by
// boot + preview). Pixelate 0..1 → NN block 1..14; Brightness ±1 → ±255; Contrast ±1 → ±100.
func visualAdjustFilter(ov core.EnemyVisualOverride) SpriteFilter {
	f := SpriteFilter{}
	if ov.Pixelate > 0 {
		f.Pixelate = int32(ov.Pixelate*13 + 1.5)
	}
	if ov.Brightness != 0 {
		f.Brightness = int32(ov.Brightness * 255)
	}
	if ov.Contrast != 0 {
		f.Contrast = ov.Contrast * 100
	}
	// Palette/retro knobs pass straight through.
	f.Posterize = ov.Posterize
	f.Saturation = ov.Saturation
	f.Dither = ov.Dither
	f.GameBoy = ov.GameBoy
	if n, ok := ov.ColorCap(); ok {
		f.MaxColors = int32(n)
	}
	return f
}

// deriveAdjustedTexture builds the display texture from pristine + adjustments
// (readback → filter → upload). ok=false for a no-op filter or unreadable base;
// appends to owned. Used at boot.
func deriveAdjustedTexture(pristine rl.Texture2D, ov core.EnemyVisualOverride, owned *[]rl.Texture2D) (rl.Texture2D, bool) {
	f := visualAdjustFilter(ov)
	if f.IsNoop() || pristine.ID == 0 {
		return rl.Texture2D{}, false
	}
	img := rl.LoadImageFromTexture(pristine)
	if img == nil || img.Width <= 0 || img.Height <= 0 {
		if img != nil {
			rl.UnloadImage(img)
		}
		return rl.Texture2D{}, false
	}
	defer rl.UnloadImage(img)
	applySpriteFilter(img, f)
	tex := rl.LoadTextureFromImage(img)
	if tex.ID == 0 {
		return rl.Texture2D{}, false
	}
	applySpriteDisplayFilter(tex, f)
	if owned != nil {
		*owned = append(*owned, tex)
	}
	return tex, true
}

// applySpriteDisplayFilter sets GPU sampling: POINT when pixelating (crisp blocks),
// else mipmapped trilinear; wrap always clamp. One source so boot/preview/reload match.
func applySpriteDisplayFilter(tex rl.Texture2D, f SpriteFilter) {
	if f.Pixelate > 1 {
		rl.SetTextureFilter(tex, rl.FilterPoint)
	} else {
		rl.GenTextureMipmaps(&tex)
		rl.SetTextureFilter(tex, rl.FilterTrilinear)
	}
	rl.SetTextureWrap(tex, rl.WrapClamp)
}

// editorFXTextures holds display textures re-derived from pristine + live FX after
// an in-session Save. Keyed by slug; prior derive freed on replace / when FX go
// neutral. Boot textures (owned by Resources) never stored here, so no double-free.
var editorFXTextures = map[string]rl.Texture2D{}

// closeEditorFXTextures frees every derived FX texture and empties the map. Called
// once at shutdown from Resources.Unload — these aren't in Resources.owned, so
// nothing else would reclaim them before process exit.
func closeEditorFXTextures() {
	for slug, tex := range editorFXTextures {
		rl.UnloadTexture(tex)
		delete(editorFXTextures, slug)
	}
}

// displayTextureForSlug returns the texture a live billboard should draw after FX
// change: a re-baked texture (pristine → filter → upload) when any FX is active,
// else pristine. Frees the prior derive first. Tracked in editorFXTextures (not
// Resources.owned) so freeing can't double-free a boot texture. Always from
// PRISTINE so repeated saves don't compound.
func displayTextureForSlug(slug string, pristine rl.Texture2D, ov core.EnemyVisualOverride) rl.Texture2D {
	if old, ok := editorFXTextures[slug]; ok {
		rl.UnloadTexture(old)
		delete(editorFXTextures, slug)
	}
	f := visualAdjustFilter(ov)
	if f.IsNoop() || pristine.ID == 0 {
		return pristine
	}
	img := rl.LoadImageFromTexture(pristine)
	if img == nil || img.Width <= 0 || img.Height <= 0 {
		if img != nil {
			rl.UnloadImage(img)
		}
		return pristine
	}
	defer rl.UnloadImage(img)
	applySpriteFilter(img, f)
	tex := rl.LoadTextureFromImage(img)
	if tex.ID == 0 {
		return pristine
	}
	applySpriteDisplayFilter(tex, f)
	editorFXTextures[slug] = tex
	return tex
}

// Asset-tab live preview: applies the override filter to a COPY of pristine into
// one managed texture (same transform boot bakes, so dragging never compounds).
var (
	assetPreviewTex    rl.Texture2D
	assetPreviewLoaded bool
)

// assetPreviewBase caches the CPU-side pristine base so a slider drag is a cheap
// ImageCopy + filter + upload, not a disk read / readback. Keyed by slug.
var (
	assetPreviewBase     *rl.Image
	assetPreviewBaseSlug string
)

// assetPreviewBaseFor returns the cached pristine base for slug, loading once
// (disk PNG or fallback readback). nil on failure.
func assetPreviewBaseFor(slug string, fallback rl.Texture2D) *rl.Image {
	if assetPreviewBase != nil && assetPreviewBaseSlug == slug {
		return assetPreviewBase
	}
	clearAssetPreviewBase()
	img, err := loadEditableSpriteImageSlug(slug, fallback)
	if err != nil {
		return nil
	}
	assetPreviewBase = img
	assetPreviewBaseSlug = slug
	return img
}

func clearAssetPreviewBase() {
	if assetPreviewBase != nil {
		rl.UnloadImage(assetPreviewBase)
		assetPreviewBase = nil
		assetPreviewBaseSlug = ""
	}
}

// invalidateAssetPreviewBase drops the cached base if it belongs to slug. Call
// after any path that rewrites <slug>.png (bake/import/restore) so a slider drag
// re-loads the new PNG instead of re-filtering the stale in-memory base.
func invalidateAssetPreviewBase(slug string) {
	if assetPreviewBase != nil && assetPreviewBaseSlug == slug {
		clearAssetPreviewBase()
	}
}

// setAssetPreviewSlug rebuilds the preview from slug's cached pristine base with f
// applied. No-op filter clears the preview (false). baseTex is the GPU fallback.
func setAssetPreviewSlug(slug string, baseTex rl.Texture2D, f SpriteFilter) bool {
	if f.IsNoop() {
		ClearAssetPreview()
		return false
	}
	base := assetPreviewBaseFor(slug, baseTex)
	if base == nil {
		return false
	}
	img := rl.ImageCopy(base) // filter mutates; keep the cached pristine intact
	if img == nil {
		return false
	}
	defer rl.UnloadImage(img)
	applySpriteFilter(img, f)
	tex := rl.LoadTextureFromImage(img)
	if tex.ID == 0 {
		return false
	}
	applySpriteDisplayFilter(tex, f)
	// Free only the prior preview TEXTURE, not the cached base (that cache is what
	// makes a drag a cheap ImageCopy); ClearAssetPreview drops both.
	if assetPreviewLoaded {
		rl.UnloadTexture(assetPreviewTex)
	}
	assetPreviewTex = tex
	assetPreviewLoaded = true
	return true
}

// RefreshFoeAssetPreview / RefreshPartyAssetPreview rebuild the preview from the
// kind/class's PRISTINE base + override (so drags don't compound). true when showing.
func RefreshFoeAssetPreview(assets Resources, kind core.EnemyKind, ov core.EnemyVisualOverride) bool {
	v, _ := enemyVisualFor(assets, kind)
	return setAssetPreviewSlug(core.EnemySlug(kind), pristineOrTexture(v), visualAdjustFilter(ov))
}

func RefreshPartyAssetPreview(assets Resources, class core.PartyClass, ov core.EnemyVisualOverride) bool {
	v, _ := partyVisualFor(assets, class)
	return setAssetPreviewSlug(core.PartyClassSlug(class), pristineOrTexture(v), visualAdjustFilter(ov))
}

// pristineOrTexture returns the pristine base, falling back to display texture if unset.
func pristineOrTexture(v enemyVisual) rl.Texture2D {
	if v.pristineTexture.ID != 0 {
		return v.pristineTexture
	}
	return v.texture
}

// ClearAssetPreview frees the preview texture + cached base. Idempotent.
func ClearAssetPreview() {
	if assetPreviewLoaded {
		rl.UnloadTexture(assetPreviewTex)
		assetPreviewTex = rl.Texture2D{}
		assetPreviewLoaded = false
	}
	clearAssetPreviewBase()
}

// AssetPreviewTexture returns the live-preview texture, or zero when none active.
func AssetPreviewTexture() rl.Texture2D {
	if assetPreviewLoaded {
		return assetPreviewTex
	}
	return rl.Texture2D{}
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, core.AssetFileMode)
}

// Party-side sprite editing: class-keyed twins of the foe functions, reusing the
// slug-keyed cores via core.PartyClassSlug. Identical behavior to the foe side.

// BakePartySpriteFilter applies f destructively to a class's sprite PNG.
func BakePartySpriteFilter(assets Resources, class core.PartyClass, f SpriteFilter) error {
	if f.IsNoop() {
		return nil
	}
	v, _ := partyVisualFor(assets, class)
	img, err := loadEditableSpriteImageSlug(core.PartyClassSlug(class), v.texture)
	if err != nil {
		return err
	}
	defer rl.UnloadImage(img)
	applySpriteFilter(img, f)
	return exportSpritePNGSlug(core.PartyClassSlug(class), img)
}

// ImportPartySpriteFromFile imports an image as the class's sprite (same matte
// safety net as the foe import).
func ImportPartySpriteFromFile(class core.PartyClass, srcPath string) error {
	img, err := decodeToNRGBA(srcPath)
	if err != nil {
		return err
	}
	keyOutOpaqueMatte(img)
	return writeSpritePNGSlug(core.PartyClassSlug(class), img)
}

// RestorePartySpriteBackup / SpriteHasPartyBackup are the class-keyed undo pair.
func RestorePartySpriteBackup(class core.PartyClass) error {
	return restoreSpriteBackupSlug(core.PartyClassSlug(class))
}

func SpriteHasPartyBackup(class core.PartyClass) bool {
	return spriteHasBackupSlug(core.PartyClassSlug(class))
}

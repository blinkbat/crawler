package render

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"  // register decoders so image.Decode handles common drops
	_ "image/jpeg" //
	"image/png"
	"os"
	"path/filepath"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Sprite editor (engine). The editor's Foe Visualizer (editor/foeview.go) calls
// these to bake destructive image edits — tint / gradient / brightness /
// contrast / grayscale / invert — into a foe's sprite PNG, or to import a PNG as
// a foe's sprite. Output lands at maps/sprites/<slug>.png (the same file the game
// loads at boot), so a bake here authors real art rather than the non-destructive
// visuals.json tint. Like every texture in this codebase the result applies on
// the NEXT game/editor launch (sprite textures load once at boot — see
// loadEnemyVisuals); the bake flashes that, mirroring the visuals.json save UX.
//
// Every bake first copies any existing PNG to <slug>.png.bak so a destructive
// edit is one Restore away from undo. All image work is paired with UnloadImage
// to keep the CPU-side raylib images from leaking.

// SpriteFilter is one destructive edit pass. Zero value is a no-op; set the
// fields the caller wants and they apply in this order: tint, brightness,
// contrast, grayscale, invert, gradient overlay. The Foe Visualizer composes a
// single-op filter per button so edits stack one click at a time (each re-reads
// the current PNG), but a filter may carry several ops at once.
type SpriteFilter struct {
	TintApply  bool     // multiply every pixel by Tint (ImageColorTint)
	Tint       rl.Color //
	Brightness int32    // -255..255 add (ImageColorBrightness); 0 = skip
	Contrast   float32  // -100..100 (ImageColorContrast); 0 = skip
	Grayscale  bool     // desaturate (ImageColorGrayscale)
	Invert     bool     // photo-negative (ImageColorInvert)
	Gradient   bool     // alpha-blend a vertical gradient over the sprite
	GradTop    rl.Color // gradient color at the top (use alpha < 255 for a wash)
	GradBottom rl.Color // gradient color at the bottom
}

// IsNoop reports whether the filter would change nothing — the UI uses it to
// avoid writing an identical PNG (and a pointless .bak) on a stray click.
func (f SpriteFilter) IsNoop() bool {
	return !f.TintApply && f.Brightness == 0 && f.Contrast == 0 &&
		!f.Grayscale && !f.Invert && !f.Gradient
}

// BakeSpriteFilter applies f to the foe's current sprite image and writes it to
// maps/sprites/<slug>.png (backing up any existing PNG first). The source is the
// existing PNG if present, else the kind's live billboard texture read back from
// the GPU (so a procedural-only foe is "promoted" into an editable PNG on its
// first bake). No-ops cleanly for an empty filter.
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

// ImportSpriteFromFile copies an external image in as the foe's sprite (the
// "upload" path — the Foe Visualizer feeds it a drag-dropped file). The source
// is decoded, validated, normalized to RGBA, and written to <slug>.png (backing
// up any existing one).
//
// Transparency safety net: a billboard tint is a multiplicative wash over the
// WHOLE sprite quad, so it only stays on the creature if the texture has a real
// alpha channel (transparent background). A PNG flattened with an opaque matte
// (a common "export lost transparency" mistake) would otherwise render — and
// tint — as a solid rectangle ("tints the whole canvas"). So when an import has
// essentially no transparency, we border-flood-key its background matte to
// alpha before saving, restoring the see-through background.
func ImportSpriteFromFile(kind core.EnemyKind, srcPath string) error {
	img, err := decodeToNRGBA(srcPath)
	if err != nil {
		return err
	}
	keyOutOpaqueMatte(img)
	return writeSpritePNG(kind, img)
}

// decodeToNRGBA loads srcPath into a straight-alpha image.NRGBA. It tries Go's
// native decoders first (png/jpeg/gif), then falls back to raylib's loader for
// the other formats raylib supports (bmp/tga/psd/hdr/pnm/…) so the import path
// accepts the same breadth rl.LoadImage did, while still handing the keying and
// PNG-encode steps a Go image.
func decodeToNRGBA(srcPath string) (*image.NRGBA, error) {
	if data, err := os.ReadFile(srcPath); err == nil {
		if src, _, derr := image.Decode(bytes.NewReader(data)); derr == nil {
			b := src.Bounds()
			if b.Dx() > 0 && b.Dy() > 0 {
				// Normalize to NRGBA so alpha edits are straight (non-premultiplied).
				dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
				draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
				return dst, nil
			}
		}
	}
	// Fallback: let raylib decode the formats Go's stdlib can't, then copy its
	// (straight-alpha) pixels into an NRGBA. One LoadImageColors call crosses the
	// cgo/purego boundary once, not per pixel.
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

// writeSpritePNG backs up any existing PNG, then writes img to <slug>.png. The
// Go-image sibling of exportSpritePNG (used by the import path, which works in
// image.NRGBA rather than rl.Image). Encodes to a buffer first so a failed
// encode never truncates the existing sprite.
func writeSpritePNG(kind core.EnemyKind, img image.Image) error {
	dir := core.ResolveAssetDir(core.SpritesDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return fmt.Errorf("encode failed: %w", err)
	}
	path := spritePath(kind)
	if _, err := os.Stat(path); err == nil {
		_ = copyFile(path, path+".bak") // best-effort safety net
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// keyOutOpaqueMatte clears an opaque background matte to transparency, but only
// when the image has essentially no alpha already (so it never disturbs a
// properly-authored transparent sprite). It flood-fills inward from the border,
// clearing pixels within tolerance of the corner matte color — border-seeded so
// an interior region that happens to match the matte color can't be punched out.
// Returns whether it changed anything.
func keyOutOpaqueMatte(img *image.NRGBA) bool {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	total := w * h
	if total == 0 {
		return false
	}
	// Respect authored alpha: if ≥2% of pixels are already transparent, the
	// sprite has a real cutout — leave it alone.
	transparent := 0
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] < 16 {
			transparent++
		}
	}
	if transparent*50 >= total {
		return false
	}

	const tol = 40
	bg := cornerMatteColor(img, w, h)
	// Only key when the four corners agree (a uniform border matte). If they
	// differ, there's no distinct background to remove — e.g. a full-bleed
	// opaque sprite — and flooding inward would erode content, so skip.
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
		return img.Pix[o+3] > 16 &&
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

// cornerMatteColor returns the most common of the four corner pixels (ties →
// top-left) — the presumed flat background matte.
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

// RestoreSpriteBackup copies <slug>.png.bak back over <slug>.png — the one-step
// undo for the last bake/import. Errors if there's no backup.
func RestoreSpriteBackup(kind core.EnemyKind) error {
	bak := spritePath(kind) + ".bak"
	if _, err := os.Stat(bak); err != nil {
		return fmt.Errorf("no backup to restore")
	}
	return copyFile(bak, spritePath(kind))
}

// SpriteHasBackup reports whether a restorable .bak exists for the kind.
func SpriteHasBackup(kind core.EnemyKind) bool {
	_, err := os.Stat(spritePath(kind) + ".bak")
	return err == nil
}

func spritePath(kind core.EnemyKind) string {
	return filepath.Join(core.ResolveAssetDir(core.SpritesDirName), core.EnemySlug(kind)+".png")
}

// loadEditableSpriteImage returns a freshly-loaded, owned *rl.Image for the foe:
// the authored PNG if one exists, otherwise the live billboard texture read back
// from the GPU. Caller must UnloadImage the result.
func loadEditableSpriteImage(assets Resources, kind core.EnemyKind) (*rl.Image, error) {
	path := spritePath(kind)
	if _, err := os.Stat(path); err == nil {
		if img := rl.LoadImage(path); img != nil && img.Width > 0 && img.Height > 0 {
			return img, nil
		} else if img != nil {
			rl.UnloadImage(img)
		}
	}
	// No (valid) authored PNG — promote the live billboard texture into an image.
	v, ok := enemyVisualFor(assets, kind)
	if !ok || v.texture.ID == 0 {
		return nil, fmt.Errorf("no sprite source for %s", core.EnemySlug(kind))
	}
	img := rl.LoadImageFromTexture(v.texture)
	if img == nil || img.Width <= 0 || img.Height <= 0 {
		if img != nil {
			rl.UnloadImage(img)
		}
		return nil, fmt.Errorf("could not read %s texture", core.EnemySlug(kind))
	}
	return img, nil
}

// applySpriteFilter runs the filter's enabled ops in a fixed order on img.
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
			// Alpha-blend the gradient over the sprite (src alpha drives the wash),
			// scaled to the sprite's exact dimensions.
			rl.ImageDraw(img, grad,
				rl.NewRectangle(0, 0, float32(grad.Width), float32(grad.Height)),
				rl.NewRectangle(0, 0, float32(img.Width), float32(img.Height)),
				rl.White)
			rl.UnloadImage(grad)
		}
	}
}

// exportSpritePNG backs up any existing PNG, then writes img to <slug>.png.
func exportSpritePNG(kind core.EnemyKind, img *rl.Image) error {
	dir := core.ResolveAssetDir(core.SpritesDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := spritePath(kind)
	// Best-effort backup (don't block the edit if the copy fails — the export is
	// the operation that matters; the .bak is a safety net).
	if _, err := os.Stat(path); err == nil {
		_ = copyFile(path, path+".bak")
	}
	if !rl.ExportImage(*img, path) {
		return fmt.Errorf("export failed: %s", path)
	}
	return nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

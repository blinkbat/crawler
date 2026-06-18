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
// contrast / grayscale / invert / pixelate — into a foe's sprite PNG, or to
// import a PNG as a foe's sprite. Output lands at maps/sprites/<slug>.png (the same file the game
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
// contrast, grayscale, invert, gradient overlay, pixelate. The Foe Visualizer
// composes a single-op filter per button so edits stack one click at a time
// (each re-reads the current PNG), but a filter may carry several ops at once.
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
	// Pixelate down-and-up-samples the sprite by this block size (in source
	// pixels) with nearest-neighbor, baking a chunky mosaic into the PNG. <=1 =
	// skip. Because each bake re-reads the current PNG, re-clicking compounds
	// the chunking — that's how the author "adjusts" the strength. Baked (not a
	// runtime shader), so it costs nothing in-game: the texture just loads coarser.
	Pixelate int32
}

// IsNoop reports whether the filter would change nothing — the UI uses it to
// avoid writing an identical PNG (and a pointless .bak) on a stray click.
func (f SpriteFilter) IsNoop() bool {
	return !f.TintApply && f.Brightness == 0 && f.Contrast == 0 &&
		!f.Grayscale && !f.Invert && !f.Gradient && f.Pixelate <= 1
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
	return writeSpritePNGSlug(core.EnemySlug(kind), img)
}

// writeSpritePNGSlug is the slug-keyed core shared by the foe and party import
// paths (works in Go's image.Image, the import format). Encodes to a buffer
// first so a failed encode never truncates the existing sprite.
func writeSpritePNGSlug(slug string, img image.Image) error {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return fmt.Errorf("encode failed: %w", err)
	}
	// Encode to the buffer FIRST (above), then prepare the destination so a
	// failed encode never touches the existing sprite or its backup.
	path, err := prepareSpriteWrite(slug)
	if err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// prepareSpriteWrite ensures the sprites dir exists, backs up any existing
// <slug>.png to <slug>.png.bak (best-effort safety net), and returns the
// destination path. Shared by the Go-image (writeSpritePNGSlug) and rl.Image
// (exportSpritePNGSlug) write paths so the dir-ensure + backup preamble can't
// drift between them. Callers must have their encoded bytes ready before
// calling so a later write failure can't truncate a sprite this already backed up.
func prepareSpriteWrite(slug string) (path string, err error) {
	dir := core.ResolveAssetDir(core.SpritesDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path = spritePathSlug(slug)
	if _, statErr := os.Stat(path); statErr == nil {
		_ = copyFile(path, path+".bak") // best-effort safety net
	}
	return path, nil
}

// Matte-keying tunables for keyOutOpaqueMatte. matteTol is the per-channel
// RGB distance (0-255) within which a pixel counts as "the background matte";
// matteAlphaFloor is the alpha below which a pixel reads as already-transparent
// (so authored cutouts are respected and only opaque matte is flooded);
// matteTransparentPct works with `>= total` as a *50 ⇒ 2% threshold on the
// share of already-transparent pixels above which the sprite is left alone.
const (
	matteTol            = 40
	matteAlphaFloor     = 16
	matteTransparentPct = 50 // transparent*50 >= total ⇔ ≥2% already transparent
)

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
		if img.Pix[i] < matteAlphaFloor {
			transparent++
		}
	}
	if transparent*matteTransparentPct >= total {
		return false
	}

	const tol = matteTol
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
	return restoreSpriteBackupSlug(core.EnemySlug(kind))
}

// SpriteHasBackup reports whether a restorable .bak exists for the kind.
func SpriteHasBackup(kind core.EnemyKind) bool {
	return spriteHasBackupSlug(core.EnemySlug(kind))
}

// restoreSpriteBackupSlug / spriteHasBackupSlug are the slug-keyed cores shared
// by the foe (EnemySlug) and party (PartyClassSlug) restore paths, so both
// sprite editors get one-step undo without duplicating the .bak handling.
func restoreSpriteBackupSlug(slug string) error {
	bak := spritePathSlug(slug) + ".bak"
	if _, err := os.Stat(bak); err != nil {
		return fmt.Errorf("no backup to restore")
	}
	return copyFile(bak, spritePathSlug(slug))
}

func spriteHasBackupSlug(slug string) bool {
	_, err := os.Stat(spritePathSlug(slug) + ".bak")
	return err == nil
}

// spritePathSlug is the single sprite-PNG path source for any slug — the foe
// kinds (EnemySlug) and the party classes (PartyClassSlug) both resolve their
// <slug>.png here, so the two sprite editors can't drift on the on-disk layout.
func spritePathSlug(slug string) string {
	return filepath.Join(core.ResolveAssetDir(core.SpritesDirName), slug+".png")
}

// loadEditableSpriteImage returns a freshly-loaded, owned *rl.Image for the foe:
// the authored PNG if one exists, otherwise the live billboard texture read back
// from the GPU. Caller must UnloadImage the result.
func loadEditableSpriteImage(assets Resources, kind core.EnemyKind) (*rl.Image, error) {
	v, _ := enemyVisualFor(assets, kind)
	return loadEditableSpriteImageSlug(core.EnemySlug(kind), v.texture)
}

// loadEditableSpriteImageSlug is the slug-keyed core: the authored <slug>.png if
// one exists, otherwise the supplied live billboard texture read back from the
// GPU (promoting a procedural-only sprite into an editable image on first bake).
// Shared by the foe and party sprite editors. Caller must UnloadImage the result.
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
	// Pixelate last so the mosaic rides on top of whatever color work ran above.
	// Nearest-neighbor down to (w/block, h/block) then back up to the original
	// size — the round-trip is what bakes the chunky blocks in. Clamp the small
	// dimension to 1 so a big block on a small sprite can't collapse to 0px.
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

// exportSpritePNG backs up any existing PNG, then writes img to <slug>.png.
func exportSpritePNG(kind core.EnemyKind, img *rl.Image) error {
	return exportSpritePNGSlug(core.EnemySlug(kind), img)
}

// exportSpritePNGSlug is the slug-keyed core shared by the foe and party bake
// paths: ensure the sprites dir, back up any existing PNG (best-effort), then
// export img to <slug>.png.
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

// editorSpriteReloads holds textures the editor re-loaded from disk after a bake
// /import/restore so the visualizer preview (and the in-session game) reflects
// the edit immediately instead of waiting for the next launch. Keyed by sprite
// slug; the prior reload for a slug is unloaded when a new edit replaces it, so
// re-baking the same sprite never holds more than one extra texture. The boot
// textures (owned by Resources) are never touched here. These persist for the
// rest of the process — the OS reclaims them at exit, and the count is bounded
// by the distinct sprites an author edits in one session.
var editorSpriteReloads = map[string]rl.Texture2D{}

// reloadSpriteTextureSlug loads <slug>.png into a fresh GPU texture configured
// exactly like the boot loader (mipmaps + trilinear + clamp), replacing any
// prior editor reload for the same slug. Returns ok=false if the PNG is missing
// or fails to decode, in which case the caller keeps its existing texture.
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

// ReloadFoeSprite re-reads kind's just-edited PNG into the live enemyVisuals
// texture so the Foe Visualizer preview updates instantly (and the change shows
// in-session, not only after a restart). The enemyVisuals slice is shared by
// reference through the by-value Resources, so writing the entry here updates the
// same slice the editor's preview and the in-game billboards read. No-op (false) if
// the kind is out of range or the PNG can't be loaded — the existing texture stays.
func ReloadFoeSprite(assets Resources, kind core.EnemyKind) bool {
	base, ok := visualAt(assets.enemyVisuals, int(kind))
	if !ok {
		return false
	}
	tex, ok := reloadSpriteTextureSlug(core.EnemySlug(kind))
	if !ok {
		return false
	}
	// An imported PNG is the new PRISTINE base; the non-destructive FX overlay
	// re-derives on top of it via RefreshFoeAssetPreview / the next boot. Drop the
	// cached base image so the preview reloads from the new art.
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

// visualAdjustFilter builds the non-destructive image filter from an override's
// Pixelate/Brightness/Contrast. Shared by the boot texture-derivation and the
// editor live preview so in-game and the editor match exactly. Pixelate 0..1 maps
// to nearest-neighbor block 1..14 (matching the in-game retro pixelate's range);
// Brightness -1..1 → ±255; Contrast -1..1 → ±100.
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
	return f
}

// deriveAdjustedTexture builds the DISPLAY texture from a pristine base plus the
// override's non-destructive image adjustments (readback → filter → re-upload).
// A pixelated result is POINT-filtered so the mosaic stays crisp in-game (the
// "blurry pixelation" fix); others keep mipmapped trilinear. Returns ok=false for
// a no-op filter or unreadable base; appends the new texture to owned for the
// resource lifecycle. Used at boot (see loadEnemyVisuals).
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

// applySpriteDisplayFilter sets the GPU sampling for a sprite texture: POINT (no
// mipmaps) when the filter pixelates, so the baked blocks read crisp rather than
// blurred; otherwise mipmapped trilinear for smooth minification. Wrap is always
// clamp. Single source so the boot, preview, and reload paths sample identically.
func applySpriteDisplayFilter(tex rl.Texture2D, f SpriteFilter) {
	if f.Pixelate > 1 {
		rl.SetTextureFilter(tex, rl.FilterPoint)
	} else {
		rl.GenTextureMipmaps(&tex)
		rl.SetTextureFilter(tex, rl.FilterTrilinear)
	}
	rl.SetTextureWrap(tex, rl.WrapClamp)
}

// Asset-tab LIVE PREVIEW. The visualizer's Asset tab drives Pixelate/Brightness/
// Contrast as non-destructive sliders; the editor renders the preview by applying
// the SpriteFilter (derived from the override) to a COPY of the sprite's PRISTINE
// base image and uploading it to this one managed texture — the exact same image
// transform the boot pass bakes into the in-game texture, so the preview matches.
// Built from the pristine (on-disk PNG / procedural readback), so dragging the
// sliders never compounds. One texture (one modal open at a time); replaced on
// each update, freed on ClearAssetPreview.
var (
	assetPreviewTex    rl.Texture2D
	assetPreviewLoaded bool
)

// assetPreviewBase caches the current sprite's PRISTINE base IMAGE (CPU-side) so a
// slider DRAG re-derives the preview with only a cheap ImageCopy + filter + GPU
// upload each frame — instead of a disk PNG read or GPU readback per frame, which
// is what made "filter every sprite live" look expensive. Keyed by slug; reloaded
// when the slug changes (foe cycle) and invalidated on import (clearAssetPreviewBase).
var (
	assetPreviewBase     *rl.Image
	assetPreviewBaseSlug string
)

// assetPreviewBaseFor returns the cached pristine base image for slug, loading it
// once (disk PNG or the supplied GPU-texture readback) and reusing it across
// drags. Returns nil on failure.
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

// setAssetPreviewSlug rebuilds the preview texture from slug's cached pristine base
// image with f applied. A no-op filter clears the preview (returns false) so the
// editor falls back to the live texture. baseTex is the GPU fallback for a
// procedural-only sprite with no PNG yet (read back once, then cached).
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
	applySpriteDisplayFilter(tex, f) // point-sampled when pixelated → sharp preview
	ClearAssetPreview()              // free the prior preview before replacing it
	assetPreviewTex = tex
	assetPreviewLoaded = true
	return true
}

// RefreshFoeAssetPreview / RefreshPartyAssetPreview rebuild the visualizer's live
// preview texture from the kind/class's PRISTINE base sprite plus the override's
// non-destructive image adjustments. Reading the pristine (not the already-shown
// texture) keeps slider drags non-compounding. A no-op adjustment clears the
// preview so the kind's real texture shows. Return true when a preview is showing.
func RefreshFoeAssetPreview(assets Resources, kind core.EnemyKind, ov core.EnemyVisualOverride) bool {
	v, _ := enemyVisualFor(assets, kind)
	return setAssetPreviewSlug(core.EnemySlug(kind), pristineOrTexture(v), visualAdjustFilter(ov))
}

func RefreshPartyAssetPreview(assets Resources, class core.PartyClass, ov core.EnemyVisualOverride) bool {
	v, _ := partyVisualFor(assets, class)
	return setAssetPreviewSlug(core.PartyClassSlug(class), pristineOrTexture(v), visualAdjustFilter(ov))
}

// pristineOrTexture returns the visual's pristine base texture, falling back to
// its display texture when pristine is unset (a defensive guard — boot populates
// pristine for every kind).
func pristineOrTexture(v enemyVisual) rl.Texture2D {
	if v.pristineTexture.ID != 0 {
		return v.pristineTexture
	}
	return v.texture
}

// ClearAssetPreview frees the live-preview texture AND the cached base image
// (modal close, or when the adjustments go fully neutral). Idempotent.
func ClearAssetPreview() {
	if assetPreviewLoaded {
		rl.UnloadTexture(assetPreviewTex)
		assetPreviewTex = rl.Texture2D{}
		assetPreviewLoaded = false
	}
	clearAssetPreviewBase()
}

// AssetPreviewTexture returns the current live-preview texture, or the zero
// texture (ID 0) when none is active — the editor passes it to DrawFoe/PartyPreview.
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
	return os.WriteFile(dst, data, 0o644)
}

// Party-side sprite editing — the class-keyed twins of the foe functions above,
// for the editor's Party Visualizer. They reuse the same slug-keyed cores
// (keyed by core.PartyClassSlug, writing maps/sprites/<class>.png), so the bake
// filters, PNG import + matte-keying, backup, and restore behave identically to
// the foe side. A party class's billboard is procedural by default, so the first
// bake/import promotes it into an authored PNG just like a procedural foe.

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

// ImportPartySpriteFromFile copies an external image in as the class's sprite,
// with the same transparency-matte safety net the foe import uses.
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

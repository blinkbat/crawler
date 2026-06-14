package render

import (
	"fmt"
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

// ImportSpriteFromFile copies an external PNG in as the foe's sprite (the
// "upload" path — the Foe Visualizer feeds it a drag-dropped file). The source
// is loaded, validated, and re-exported to <slug>.png (backing up any existing
// one), so the import normalizes whatever PNG variant the file was.
func ImportSpriteFromFile(kind core.EnemyKind, srcPath string) error {
	img := rl.LoadImage(srcPath)
	if img == nil || img.Width <= 0 || img.Height <= 0 {
		if img != nil {
			rl.UnloadImage(img)
		}
		return fmt.Errorf("not a loadable image: %s", filepath.Base(srcPath))
	}
	defer rl.UnloadImage(img)
	return exportSpritePNG(kind, img)
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

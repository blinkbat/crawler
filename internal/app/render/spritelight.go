package render

import (
	"math"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Fake directional light for battle billboards. Sprites bypass the world lighting
// shader (a flat blit), so by default they read as evenly-lit cutouts pasted on a lit
// scene. These helpers bake the missing volume into the per-vertex color: a vertical
// value ramp (head lighter, feet darker), a warm/cool side split, a dark rim that
// detaches the sprite from the busy backdrop, and an atmospheric wash that sinks the
// back rank. All knob-driven from Debug ▸ Combat Tuning; a zero-value spriteLight
// (all knobs 0) falls straight through to the plain billboard blit.

// spriteLight carries the per-billboard fake-light knobs (live from BattleTuning).
type spriteLight struct {
	shade     float32 // foot-darken depth, 0..1 (0 = flat)
	warmCool  float32 // warm-left / cool-right split strength, 0..1
	outline   float32 // dark rim alpha, 0..1 (0 = none)
	outlinePx float32 // rim thickness in screen pixels (≈ one retro pixel block)
}

// battleSpriteLight reads the live combat-tuning sprite-light knobs. The rim is sized
// to one retro pixelate block so it reads as a single pixel-art-pixel outline at any
// pixelate intensity (and ~1px with pixelate off).
func battleSpriteLight(g *core.GameState) spriteLight {
	px := float32(1)
	if f := float32(g.RetroFilters[core.RetroFilterPixelate]); f > 0 {
		px = 1 + 13*f // mirror the retro shader's mix(1, 14, fPixelate) block size
	}
	return spriteLight{
		shade:     battleTune.SpriteShade,
		warmCool:  battleTune.SpriteWarmCool,
		outline:   battleTune.SpriteOutline,
		outlinePx: px,
	}
}

// Warm/cool side targets the split leans toward. Multiplied into the tint, so only
// values ≤255 matter (a multiply can darken/shift hue, never brighten). Subtle.
var (
	spriteWarmTarget = rl.NewColor(255, 236, 208, 255) // key side (warm)
	spriteCoolTarget = rl.NewColor(206, 220, 255, 255) // fill side (cool)
)

const spriteOutlineDepthBias = float32(0.012) // push the rim just behind the sprite

// tanHalfFovYCache memoizes tan(Fovy/2) keyed on the camera's vertical FOV so the
// per-sprite outline pass doesn't recompute the same trig for every billboard in a
// frame (Fovy is constant across the pass; it only eases on battle enter/exit).
var tanHalfFovYCache struct {
	fovy float32
	tan  float32
}

func tanHalfFovY(fovy float32) float32 {
	if tanHalfFovYCache.fovy != fovy {
		tanHalfFovYCache.fovy = fovy
		tanHalfFovYCache.tan = float32(math.Tan(float64(fovy) * 0.5 * degToRad64)) // Fovy/2 in radians
	}
	return tanHalfFovYCache.tan
}

// recedeMul is the back-rank recede as a MULTIPLICATIVE tint — folded into the sprite
// color BEFORE the (opaque) draw, so the foe stays fully solid. An earlier alpha-overlay
// wash read as a translucent film. Darkens by strength and leans cool (red dimmed most,
// blue least) for atmospheric depth. strength 0 = no-op (white).
func recedeMul(strength float32) rl.Color {
	s := core.Clamp(strength, 0, 1)
	v := 1 - 0.8*s // overall brightness after darken
	return rl.NewColor(toByte(v*(1-0.18*s)), toByte(v*(1-0.08*s)), toByte(v), 255)
}

// drawShadedBillboard draws a battle billboard with the spriteLight treatment: a dark
// rim first (behind the sprite), then the sprite itself with the value ramp + warm/cool
// split baked into its corners. Falls back to the plain blit when sh carries no shading
// so non-battle callers are unaffected.
func drawShadedBillboard(camera rl.Camera3D, tex rl.Texture2D, pos rl.Vector3, size rl.Vector2, tint rl.Color, sh spriteLight) {
	if sh.outline > 0 {
		// Rim fades with the sprite (a death-fading corpse shouldn't keep a hard edge).
		rim := uint8(255 * core.Clamp(sh.outline, 0, 1) * float32(tint.A) / 255)
		drawSpriteOutline(camera, tex, pos, size, sh.outlinePx, rim)
	}
	if sh.shade <= 0 && sh.warmCool <= 0 {
		drawTextureBillboard(camera, tex, pos, size, tint)
		return
	}
	top := tint
	bot := shadeRGB(tint, 1-core.Clamp(sh.shade, 0, 1))
	warm := lerpToWhite(spriteWarmTarget, sh.warmCool)
	cool := lerpToWhite(spriteCoolTarget, sh.warmCool)
	drawGradientBillboard(camera, tex, pos, size,
		tintMul(top, warm), // top-left  — head, warm key
		tintMul(top, cool), // top-right — head, cool fill
		tintMul(bot, cool), // bottom-right — feet, cool fill
		tintMul(bot, warm), // bottom-left  — feet, warm key
	)
}

// billboardBasis returns the camera-facing forward + right axes shared by the
// billboard draws (matches raylib's DrawBillboardRec basis: world-up vertical).
func billboardBasis(camera rl.Camera3D) (fwd, right rl.Vector3) {
	fwd = rl.Vector3Normalize(rl.Vector3Subtract(camera.Position, camera.Target))
	right = rl.Vector3Normalize(rl.Vector3CrossProduct(camera.Up, fwd))
	return fwd, right
}

// drawSpriteOutline stamps the sprite's silhouette in black at four cardinal offsets,
// just behind the sprite, to ring it with a dark rim. Cardinal (not scale-up) so it
// reads as an even edge and doesn't fill interior gaps. Offset scales with sprite size
// so the rim stays ~constant in screen pixels near vs far.
func drawSpriteOutline(camera rl.Camera3D, tex rl.Texture2D, pos rl.Vector3, size rl.Vector2, px float32, alpha uint8) {
	if alpha == 0 || px <= 0 {
		return
	}
	fwd, right := billboardBasis(camera)
	// Convert the target screen thickness (px) into a world offset at this sprite's
	// depth, so the rim stays ~px wide near or far — a fixed world pad would balloon up
	// close (the old sprite-size scaling did exactly that). tan(Fovy/2)/scrH is the
	// same for every sprite this frame; only the per-sprite distance varies.
	_, scrH := screenSizeF()
	dist := rl.Vector3Distance(pos, camera.Position)
	pad := px * 2 * dist * tanHalfFovY(camera.Fovy) / scrH
	base := rl.Vector3Subtract(pos, rl.Vector3Scale(fwd, spriteOutlineDepthBias))
	col := rl.NewColor(0, 0, 0, alpha)
	offsets := [4]rl.Vector3{
		rl.Vector3Scale(right, pad), rl.Vector3Scale(right, -pad),
		{X: 0, Y: pad, Z: 0}, {X: 0, Y: -pad, Z: 0},
	}
	for _, o := range offsets {
		drawTextureBillboard(camera, tex, rl.Vector3Add(base, o), size, col)
	}
}

// drawGradientBillboard draws a camera-facing billboard like drawTextureBillboard, but
// with independent per-corner colors so a caller can bake a vertical ramp + side split
// into the sprite. Matches raylib's DrawBillboardRec basis exactly: world-up vertical
// axis, camera right, centered on pos. Corner args are top-left, top-right, bottom-
// right, bottom-left.
func drawGradientBillboard(camera rl.Camera3D, tex rl.Texture2D, pos rl.Vector3, size rl.Vector2, tl, tr, br, bl rl.Color) {
	_, right := billboardBasis(camera)
	rx := rl.Vector3Scale(right, size.X/2)
	uy := rl.NewVector3(0, size.Y/2, 0)
	p0 := rl.Vector3Subtract(rl.Vector3Subtract(pos, rx), uy) // bottom-left
	p1 := rl.Vector3Subtract(rl.Vector3Add(pos, rx), uy)      // bottom-right
	p2 := rl.Vector3Add(rl.Vector3Add(pos, rx), uy)           // top-right
	p3 := rl.Vector3Add(rl.Vector3Subtract(pos, rx), uy)      // top-left
	rl.SetTexture(tex.ID)
	rl.Begin(rl.Quads)
	// Order + texcoords mirror DrawBillboardPro: BL(0,1) TL(0,0) TR(1,0) BR(1,1).
	rl.Color4ub(bl.R, bl.G, bl.B, bl.A)
	rl.TexCoord2f(0, 1)
	rl.Vertex3f(p0.X, p0.Y, p0.Z)
	rl.Color4ub(br.R, br.G, br.B, br.A)
	rl.TexCoord2f(1, 1)
	rl.Vertex3f(p1.X, p1.Y, p1.Z)
	rl.Color4ub(tr.R, tr.G, tr.B, tr.A)
	rl.TexCoord2f(1, 0)
	rl.Vertex3f(p2.X, p2.Y, p2.Z)
	rl.Color4ub(tl.R, tl.G, tl.B, tl.A)
	rl.TexCoord2f(0, 0)
	rl.Vertex3f(p3.X, p3.Y, p3.Z)
	rl.End()
	rl.SetTexture(0)
}

// shadeRGB scales a color's RGB by f (alpha untouched) — the vertical value ramp's
// foot multiplier. Darken-only (f clamped to [0,1]); routes through shadeColor so
// the RGB-scale math lives in one place (the mapRGB seam).
func shadeRGB(c rl.Color, f float32) rl.Color {
	return shadeColor(c, core.Clamp(f, 0, 1))
}

// lerpToWhite blends from white toward target by t (t=0 → white/no-op, t=1 → target),
// yielding a subtle multiplicative hue shift. Routes the per-channel math through
// core.MixColor (the one home for color lerps); alpha is pinned opaque.
func lerpToWhite(target rl.Color, t float32) rl.Color {
	out := core.MixColor(rl.White, target, float64(core.Clamp(t, 0, 1)))
	out.A = 255
	return out
}

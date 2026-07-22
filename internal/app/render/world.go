package render

import (
	"fmt"
	"math"
	"reflect"
	"sort"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// tileHalf is half a tile in world units — the XZ extent from a tile center to its
// edge, shared by the cliff/voxel/floor-quad geometry.
const tileHalf = float32(core.TileSize) / 2

// init guards the enemyVisual<->core.EnemyVisualOverride round-trip: fills every
// override field non-zero, round-trips it, and panics on any dropped field (a
// field added to one half but not the other). Scale fields non-zero so the
// effective*Scale 0->1 fold can't mask a real drop. In init not _test.go because
// render tests can't run without raylib.dll.
func init() {
	var ov core.EnemyVisualOverride
	rv := reflect.ValueOf(&ov).Elem()
	for i := 0; i < rv.NumField(); i++ {
		f := rv.Field(i)
		switch f.Kind() {
		case reflect.Float32:
			f.SetFloat(float64(i + 1))
		case reflect.Uint8:
			f.SetUint(uint64(i%255 + 1))
		default:
			panic(fmt.Sprintf("render: EnemyVisualOverride field %s has unexpected kind %s — extend the round-trip init guard", rv.Type().Field(i).Name, f.Kind()))
		}
	}
	if got := enemyVisualOverride(applyEnemyVisualOverride(enemyVisual{}, ov)); !reflect.DeepEqual(got, ov) {
		panic(fmt.Sprintf("render: enemyVisual<->EnemyVisualOverride round-trip dropped a field — wire it into BOTH enemyVisualOverride and applyEnemyVisualOverride:\n  in:  %+v\n  out: %+v", ov, got))
	}
}

type enemyVisual struct {
	texture rl.Texture2D
	// pristineTexture is the UNADJUSTED base sprite; `texture` is derived from it
	// plus the override's image adjustments at load. The editor re-derives its
	// preview from pristine so dragging FX sliders never compounds.
	pristineTexture rl.Texture2D
	size            rl.Vector2
	// shadowRadius is the contact-disc half-extent (world units). Zero = no
	// shadow (every procedural sprite's default; only opt-in kinds get a disc).
	shadowRadius float32
	// yOffset shifts this billboard up(+)/down(−) from enemyBillboardY (world
	// units); the shadow stays anchored to the tile. Zero for procedural sprites.
	// Calibrated against the BATTLE center (battleFormationCenterY); drawFieldPacks
	// adds the framing-lift delta back so a grounded sprite plants in both views.
	yOffset float32
	// markerYOffset/markerXOffset nudge the selector pyramid from its default
	// anchor (world units). X is camera-right so it reads the same whichever way
	// the camera faces. Zero = shared default.
	markerYOffset float32
	markerXOffset float32
	// depthOffset pushes the BATTLE billboard back into the arena (camera-forward),
	// for a sprite whose art sits forward in its box. Battle-only; zero = default.
	depthOffset float32
	// shadowOffsetX/shadowOffsetZ nudge the contact disc from the sprite's final
	// footprint along camera-right(+)/camera-forward(+). Zero = dead under it.
	shadowOffsetX float32
	shadowOffsetZ float32
	// markerScale multiplies the target-chevron silhouette. Zero = unset → 1.0
	// (effectiveMarkerScale).
	markerScale float32
	// glyphXOffset/glyphYOffset nudge the hit-clarity glyph from sprite center
	// (camera-right(+)/world-up(+)); glyphScale multiplies its radius. Zero scale
	// = unset → 1.0.
	glyphXOffset float32
	glyphYOffset float32
	glyphScale   float32
	// particleXOffset/Y/Z nudge the impact-burst origin (camera-right/world-up/
	// camera-forward); particleScale scales spread + dot size. Zero scale = unset
	// → 1.0 (effectiveParticleScale).
	particleXOffset float32
	particleYOffset float32
	particleZOffset float32
	particleScale   float32
	// popupXOffset/popupYOffset nudge the floating damage-number spawn
	// (camera-right(+)/world-up(+)), additive on the baked default rise.
	popupXOffset float32
	popupYOffset float32
	// tint is a per-kind base color multiplied (ColorTint: a*b/255 per channel)
	// into the runtime tint AFTER the combat branches, so a tinted sprite stays
	// proportionally tinted in every state. Zero value (A==0) = untinted; set
	// A==255 for a real tint. See resolveTint.
	tint rl.Color
	// Non-destructive image adjustments mirroring the override fields (round-trip
	// lossless; editor seeds sliders from these). They drive how `texture` derives
	// from `pristineTexture` at build time — they don't alter the draw directly.
	pixelate   float32
	brightness float32
	contrast   float32
	// Palette / retro FX — same build-time-only role; carried for lossless
	// round-trip. See visualAdjustFilter.
	posterize  float32
	saturation float32
	dither     float32
	gameBoy    float32
	// maxColors mirrors the override's palette cap (median-cut quantization).
	maxColors float32
}

// resolveTint returns the per-kind base tint, treating the zero-value Color
// (A==0, unset) as White so untinted kinds draw at full color.
func (v enemyVisual) resolveTint() rl.Color {
	if v.tint.A == 0 {
		return rl.White
	}
	return v.tint
}

// effective*Scale resolve a per-kind size multiplier, treating the zero value
// (unset, or absent in an old visuals.json) as 1.0.
func (v enemyVisual) effectiveMarkerScale() float32   { return scaleOrDefault(v.markerScale) }
func (v enemyVisual) effectiveGlyphScale() float32    { return scaleOrDefault(v.glyphScale) }
func (v enemyVisual) effectiveParticleScale() float32 { return scaleOrDefault(v.particleScale) }

func scaleOrDefault(s float32) float32 {
	if s <= 0 {
		return 1
	}
	return s
}

// shadowFootprint returns the world XZ for the contact disc: the sprite's drawn
// footprint (depthOffset already in `position`) plus the camera-relative
// shadowOffset nudge, so the disc tracks the sprite however it's moved.
func shadowFootprint(camera rl.Camera3D, position rl.Vector3, v *enemyVisual) (float32, float32) {
	x, z := position.X, position.Z
	if v.shadowOffsetX != 0 || v.shadowOffsetZ != 0 {
		fwd := horizontalForward(camera)
		right := horizontalRight(fwd)
		x += right.X*v.shadowOffsetX + fwd.X*v.shadowOffsetZ
		z += right.Z*v.shadowOffsetX + fwd.Z*v.shadowOffsetZ
	}
	return x, z
}

// cameraRelativeOffset nudges p by dx (camera-right), dy (world-up), dz
// (camera-forward), so the nudge reads the same whichever way the camera faces.
// Zero nudge returns p untouched (skips the trig).
func cameraRelativeOffset(camera rl.Camera3D, p rl.Vector3, dx, dy, dz float32) rl.Vector3 {
	if dx == 0 && dy == 0 && dz == 0 {
		return p
	}
	fwd := horizontalForward(camera)
	right := horizontalRight(fwd)
	p.X += right.X*dx + fwd.X*dz
	p.Z += right.Z*dx + fwd.Z*dz
	p.Y += dy
	return p
}

// enemyVisualOverride snapshots an enemyVisual's tunable fields into the
// raylib-free core.EnemyVisualOverride. Texture excluded (always from the asset).
// Inverse: applyEnemyVisualOverride.
func enemyVisualOverride(v enemyVisual) core.EnemyVisualOverride {
	return core.EnemyVisualOverride{
		SizeX:         v.size.X,
		SizeY:         v.size.Y,
		YOffset:       v.yOffset,
		DepthOffset:   v.depthOffset,
		ShadowRadius:  v.shadowRadius,
		ShadowOffsetX: v.shadowOffsetX,
		ShadowOffsetZ: v.shadowOffsetZ,
		MarkerYOffset: v.markerYOffset,
		MarkerXOffset: v.markerXOffset,
		// Snapshot the EFFECTIVE scale (unset 0 → 1.0) so the editor seeds sliders
		// at full size. Offsets snapshot raw (0 = no nudge).
		MarkerScale:     v.effectiveMarkerScale(),
		GlyphXOffset:    v.glyphXOffset,
		GlyphYOffset:    v.glyphYOffset,
		GlyphScale:      v.effectiveGlyphScale(),
		ParticleXOffset: v.particleXOffset,
		ParticleYOffset: v.particleYOffset,
		ParticleZOffset: v.particleZOffset,
		ParticleScale:   v.effectiveParticleScale(),
		PopupXOffset:    v.popupXOffset,
		PopupYOffset:    v.popupYOffset,
		TintR:           v.tint.R,
		TintG:           v.tint.G,
		TintB:           v.tint.B,
		TintA:           v.tint.A,
		Pixelate:        v.pixelate,
		Brightness:      v.brightness,
		Contrast:        v.contrast,
		Posterize:       v.posterize,
		Saturation:      v.saturation,
		Dither:          v.dither,
		GameBoy:         v.gameBoy,
		MaxColors:       v.maxColors,
	}
}

// applyEnemyVisualOverride returns v with every tunable field replaced by the
// override's value, preserving the texture. Used by loadEnemyVisuals and the
// editor's live preview.
func applyEnemyVisualOverride(v enemyVisual, ov core.EnemyVisualOverride) enemyVisual {
	v.size = rl.NewVector2(ov.SizeX, ov.SizeY)
	v.yOffset = ov.YOffset
	v.depthOffset = ov.DepthOffset
	v.shadowRadius = ov.ShadowRadius
	v.shadowOffsetX = ov.ShadowOffsetX
	v.shadowOffsetZ = ov.ShadowOffsetZ
	v.markerYOffset = ov.MarkerYOffset
	v.markerXOffset = ov.MarkerXOffset
	// Scales direct-assign; effective*Scale folds an unset 0 back to 1.0 at the
	// draw site, so a 0 here never means "invisible."
	v.markerScale = ov.MarkerScale
	v.glyphXOffset = ov.GlyphXOffset
	v.glyphYOffset = ov.GlyphYOffset
	v.glyphScale = ov.GlyphScale
	v.particleXOffset = ov.ParticleXOffset
	v.particleYOffset = ov.ParticleYOffset
	v.particleZOffset = ov.ParticleZOffset
	v.particleScale = ov.ParticleScale
	v.popupXOffset = ov.PopupXOffset
	v.popupYOffset = ov.PopupYOffset
	v.tint = rl.NewColor(ov.TintR, ov.TintG, ov.TintB, ov.TintA)
	v.pixelate = ov.Pixelate
	v.brightness = ov.Brightness
	v.contrast = ov.Contrast
	v.posterize = ov.Posterize
	v.saturation = ov.Saturation
	v.dither = ov.Dither
	v.gameBoy = ov.GameBoy
	v.maxColors = ov.MaxColors
	return v
}

// tintMul multiplies two colors channel-wise (ColorTint: a*b/255, alpha included).
func tintMul(a, b rl.Color) rl.Color {
	return rl.NewColor(
		uint8(int(a.R)*int(b.R)/255),
		uint8(int(a.G)*int(b.G)/255),
		uint8(int(a.B)*int(b.B)/255),
		uint8(int(a.A)*int(b.A)/255),
	)
}

// exploreFOV is the wide walking FOV; favors situational awareness over edge
// perspective distortion (battle eases toward the narrower battleTune.CamFOV).
const exploreFOV = float32(100)

// exploreCamDrop lowers the walking eye (~a quarter of EyeHeight) for a grounded
// over-the-shoulder feel. Applied only out of battle — it fades out as battleCamBlend
// rises, so the battle's own tuned eye-lift isn't disturbed.
const exploreCamDrop = float32(-0.34)

// battleCamBlendRate eases the explore↔battle camera blend at ~1/TurnDuration per
// second, so every battle camera param (tilt, eye-lift, FOV) transitions together in
// about a player-turn's time — no snap on battle enter/exit.
const battleCamBlendRate = float32(1.0 / core.TurnDuration)

// battleCamBlend is the eased explore→battle factor (0 = explore, 1 = battle), held
// across draws so the transition survives frame to frame.
var battleCamBlend float32

// battleTune mirrors g.BattleTuning, synced at the top of Camera() each frame so the
// battle geometry helpers (which don't all receive g) can read the live values the
// Debug ▸ Combat Tuning panel edits. Package default keeps it valid before the first
// sync. The combat FOV, camera tilt/lift, and foe/party placement all read it.
var battleTune = core.DefaultBattleTuning()

// Chest peek: while a chest modal is open the camera eases down + toward the chest
// to peer inside, easing back out on close. chestPeekYaw caches the chest's world
// bearing so the ease-OUT (after ChestOpen clears) keeps aiming the right way.
const (
	chestPeekRate  = float32(1.0 / 0.22) // ease in/out over ~0.22s
	chestPeekPitch = float32(-0.95)      // steep downward tilt to look into the box
	chestPeekLean  = float32(0.85)       // forward dolly well into the chest tile
	chestPeekDrop  = float32(-0.22)      // eye drop so it leans over the rim
)

var (
	chestPeekBlend float32
	chestPeekYaw   float32
	chestPeekHave  bool
)

// camShakeHzX/Y are the two incommensurate wall-clock freqs of the combat screen
// shake (sibling of torchFlickerHzA/B) — named so the jitter isn't bare magic.
const (
	camShakeHzX = 47.0
	camShakeHzY = 61.0
)

func Camera(g *core.GameState) rl.Camera3D {
	// Sync the live combat tuning before any battle geometry (incl. this camera)
	// reads it. Guard against a zero-value GameState (struct-literal in a test) so a
	// stray call can't clobber the package default with zeros.
	if g.BattleTuning.CamFOV > 0 {
		battleTune = g.BattleTuning
	}
	// Ease the explore↔battle blend so the tilt, eye-lift, and FOV animate in together
	// rather than snapping when a fight starts or ends.
	battleTarget := float32(0)
	if g.Battle.Active() {
		battleTarget = 1
	}
	battleCamBlend = core.Approach(battleCamBlend, battleTarget, battleCamBlendRate*rl.GetFrameTime())

	p := g.Player
	// Chest peek: aim the view down + toward the open chest. Cache the bearing while
	// open so the ease-out keeps aiming after ChestOpen clears.
	peekTarget := float32(0)
	if g.ChestOpen >= 0 && g.ChestOpen < len(g.Chests) {
		ch := g.Chests[g.ChestOpen]
		cpos := tileWorldPos(ch.TileX, ch.TileZ, 0)
		chestPeekYaw = float32(math.Atan2(float64(cpos.Z-p.Z), float64(cpos.X-p.X)))
		chestPeekHave = true
		peekTarget = 1
	}
	chestPeekBlend = core.Approach(chestPeekBlend, peekTarget, chestPeekRate*rl.GetFrameTime())
	if chestPeekBlend <= 0.001 {
		chestPeekHave = false
	}

	yaw := p.Yaw + p.LookYaw
	pitch := p.LookPitch + battleTune.CamPitch*battleCamBlend
	if chestPeekHave {
		// Ease yaw the short way toward the chest bearing; tilt down to look in.
		yaw += chestPeekBlend * core.WrapAngle(chestPeekYaw-yaw)
		pitch += chestPeekBlend * chestPeekPitch
	}
	cp := float32(math.Cos(float64(pitch)))
	direction := rl.NewVector3(
		cp*float32(math.Cos(float64(yaw))),
		float32(math.Sin(float64(pitch))),
		cp*float32(math.Sin(float64(yaw))),
	)
	// Eye rides the ground height underfoot: StandGroundY at rest, the eased
	// Player.GroundY mid-step so the climb is smooth across a ramp.
	groundY := g.Area.StandGroundY(p.TileX, p.TileZ)
	if len(g.Area.Solids) > 0 {
		// Voxel map: ride the resolved standing level, not the column top.
		groundY = g.Area.StandGroundYAt(p.TileX, p.Level, p.TileZ)
	}
	if p.Anim.Kind == core.AnimStep {
		groundY = p.GroundY
	}
	// Eye rides the ground. Out of battle the explore drop lowers it; in battle that
	// fades out and the tuned eye-lift fades in — both ride the one blend.
	eyeY := core.EyeHeight + groundY + exploreCamDrop*(1-battleCamBlend) + battleTune.CamLift*battleCamBlend
	position := rl.NewVector3(p.X, eyeY, p.Z)
	// Chest peek: lean the eye toward the chest and dip it slightly so the tilt reads
	// as peering over the rim into the box.
	if chestPeekHave {
		lean := chestPeekBlend * chestPeekLean
		position.X += float32(math.Cos(float64(chestPeekYaw))) * lean
		position.Z += float32(math.Sin(float64(chestPeekYaw))) * lean
		position.Y += chestPeekBlend * chestPeekDrop
	}
	// Camera-relative truck/dolly (battle framing). Translating only `position` moves
	// the whole frustum — the look-target (position+direction) rides along, so the view
	// pans without rotating. Ground-plane only (X/Z); vertical framing is CamLift.
	if battleTune.CamShiftX != 0 || battleTune.CamShiftZ != 0 {
		right := rl.Vector3Normalize(rl.Vector3CrossProduct(direction, worldUp))
		shiftX := battleTune.CamShiftX * battleCamBlend
		shiftZ := battleTune.CamShiftZ * battleCamBlend
		position.X += right.X*shiftX + direction.X*shiftZ
		position.Z += right.Z*shiftX + direction.Z*shiftZ
	}
	// Combat screen shake: positional jitter eased out by ShakeTimer. Wall-clock-
	// driven (two incommensurate freqs) so it's visible even while hit-stop freezes
	// the sim. Battle-only.
	if g.Battle.Active() && g.Battle.ShakeTimer > 0 && g.Battle.ShakeDur > 0 {
		amp := g.Battle.ShakePeak * core.Clamp(g.Battle.ShakeTimer/g.Battle.ShakeDur, 0, 1)
		t := rl.GetTime()
		position.X += float32(math.Sin(t*camShakeHzX)) * amp
		position.Y += float32(math.Sin(t*camShakeHzY)) * amp
	}
	// FOV eases between the wide walking FOV and the combat FOV on the same blend.
	fov := exploreFOV + (battleTune.CamFOV-exploreFOV)*battleCamBlend
	// Screen-wipe camera FX (Zoom/Spin/Wobble) ride on top during the entry/preview
	// window; identity otherwise. direction is unit, so it's a valid roll axis.
	up, fov := battleWipeCamera(g, direction, fov)
	return rl.NewCamera3D(
		position,
		rl.NewVector3(position.X+direction.X, position.Y+direction.Y, position.Z+direction.Z),
		up,
		fov,
		rl.CameraPerspective,
	)
}

// SkyClearColor is the ClearBackground color before DrawSkyBackground paints
// over it. The hue is overdrawn immediately — the clear is load-bearing for the
// DEPTH wipe that rides with it (see run.go). Single-sources the literal.
var SkyClearColor = rl.NewColor(87, 172, 244, 255)

func DrawSkyBackground(assets Resources, g *core.GameState) {
	texW := float32(assets.skyTexture.Width)
	texH := float32(assets.skyTexture.Height)
	screenW, screenH := screenSizeF()
	// Crop the source to the screen aspect so the 2:1 sky texture doesn't stretch
	// (clouds stay round).
	srcX, srcW := float32(0), texW
	srcY, srcH := float32(0), texH
	if screenH <= 0 || texH <= 0 {
		return // minimized/zero-size window: aspect math would be Inf/NaN
	}
	screenAspect := screenW / screenH
	texAspect := texW / texH
	if texAspect > screenAspect {
		// Wider than screen: crop the sides.
		srcW = texH * screenAspect
		srcX = (texW - srcW) / 2
	} else if texAspect < screenAspect {
		// Taller than screen: crop more off the top to keep the cloud band in view.
		srcH = texW / screenAspect
		srcY = (texH - srcH) * 0.35
	}
	source := rl.NewRectangle(srcX, srcY, srcW, srcH)
	dest := rl.NewRectangle(0, 0, screenW, screenH)
	// Sky tint follows the time-of-day profile for every material — no indoor
	// gate, since CeilingAt slabs cap roofed rooms so a varying sky is invisible
	// there anyway (and the old gate left forest-on-dungeon maps starless).
	profile := timeProfileAt(g.StepCount)
	tint := skyColor(profile.SkyTint)
	rl.DrawTexturePro(assets.skyTexture, source, dest, rl.NewVector2(0, 0), 0, tint)
	// Cloud planes drift on the wind at parallax speeds — the base animation is
	// two extra textured quads whose SOURCE rect slides in x (repeat wrap makes
	// the offset seamless). Mod by texW keeps the offset small over long sessions.
	now := rl.GetTime()
	farSrc := source
	farSrc.X = srcX + float32(math.Mod(now*float64(cloudDriftFar), float64(texW)))
	nearSrc := source
	nearSrc.X = srcX + float32(math.Mod(now*float64(cloudDriftNear), float64(texW)))
	// On top of the drift, the warp shader (cloudwarp.go) domain-warps each plane
	// through a slow flow field so the cumulus billow and reshape as they cross —
	// subtle, trippy, ~free. Fall back to a plain draw if the shader didn't compile.
	if assets.cloudWarp.shader.ID != 0 {
		assets.cloudWarp.begin(screenW, screenH, float32(now))
		assets.cloudWarp.layer(cloudWarpFar)
		rl.DrawTexturePro(assets.cloudFarTexture, farSrc, dest, rl.NewVector2(0, 0), 0, tint)
		rl.DrawRenderBatchActive() // flush FAR with its params before NEAR overwrites them
		assets.cloudWarp.layer(cloudWarpNear)
		rl.DrawTexturePro(assets.cloudNearTexture, nearSrc, dest, rl.NewVector2(0, 0), 0, tint)
		rl.EndShaderMode()
	} else {
		rl.DrawTexturePro(assets.cloudFarTexture, farSrc, dest, rl.NewVector2(0, 0), 0, tint)
		rl.DrawTexturePro(assets.cloudNearTexture, nearSrc, dest, rl.NewVector2(0, 0), 0, tint)
	}
	// Star layer rides the same source/dest. Alpha = StarAlpha * per-pixel alpha
	// (sparse pinpoints).
	if profile.StarAlpha > 0 {
		alpha := uint8(profile.StarAlpha * 255)
		rl.DrawTexturePro(assets.starTexture, source, dest, rl.NewVector2(0, 0), 0, colorWithAlpha(rl.White, alpha))
	}
}

// Cloud drift speeds in texture px/sec: the near plane outruns the far one, so
// the bank reads as depth, not a sliding painting. A near cloud crosses the
// full panorama in ~4 minutes — present but never distracting.
const (
	cloudDriftFar  = float32(1.7)
	cloudDriftNear = float32(4.4)
)

// behindCullSlack is how far behind the camera a tile center can project before
// it's skipped. Generous so the tile underfoot and half-behind tiles stay drawn
// through any rotation. No hard distance cap — fog handles far falloff and pop-in
// would show against its 85%-clamped tail.
const behindCullSlack = float32(-2.5)

// viewCull is the per-frame horizontal view-frustum test: camera basis + side-
// plane half-tangent hoisted out of per-item loops. Culls points behind the
// camera (behindCullSlack) OR outside the horizontal FOV wedge. Built once per
// draw via newViewCull; shared by the tile loop and the chest/door/crystal draws.
type viewCull struct {
	pos     rl.Vector3
	fwd     rl.Vector3
	right   rl.Vector3
	tanHalf float32
}

const (
	// viewCullApexBack pushes the cone apex behind the camera so near/side tiles
	// stay inside the wedge. Kept >= |behindCullSlack| so the side test never fires
	// inside the band the back-plane test keeps.
	viewCullApexBack = float32(3.0)
	// viewCullSlack widens the half-tangent so the boundary sits outside the screen
	// edge (margin for a tile center just off-screen with a still-visible slab/prop).
	viewCullSlack = float32(1.3)
)

func newViewCull(camera rl.Camera3D) viewCull {
	fwd := horizontalForward(camera)
	// Fovy is VERTICAL fov (deg); horizontal half-tangent = tan(Fovy/2)·aspect,
	// widened by viewCullSlack. Fovy*Pi/360 == (Fovy/2)·deg2rad.
	sw, sh := screenSizeF()
	aspect := float32(1)
	if sh > 0 {
		aspect = sw / sh
	}
	tanHalf := tanHalfFovY(camera.Fovy) * aspect * viewCullSlack
	return viewCull{pos: camera.Position, fwd: fwd, right: horizontalRight(fwd), tanHalf: tanHalf}
}

// cullXZ reports whether (px,pz) is behind the camera or beyond the wedge.
func (v viewCull) cullXZ(px, pz float32) bool {
	dx := px - v.pos.X
	dz := pz - v.pos.Z
	f := dx*v.fwd.X + dz*v.fwd.Z
	if f < behindCullSlack {
		return true // behind the camera
	}
	r := dx*v.right.X + dz*v.right.Z
	halfWidth := (f + viewCullApexBack) * v.tanHalf
	return r > halfWidth || r < -halfWidth
}

func (v viewCull) cull(p rl.Vector3) bool { return v.cullXZ(p.X, p.Z) }

// DrawWorld draws the full lit environment pass for the live game — see drawAreaWorld.
func DrawWorld(camera rl.Camera3D, g *core.GameState, assets Resources) {
	drawAreaWorld(camera, &g.Area, assets, g.StepCount, g.Crystals, g, nil)
}

// DrawArea draws the same static lit environment as DrawWorld but from an
// AreaDefinition alone (no GameState) — the map editor's 3D render path. stepCount
// drives the day/night lighting dial; crystals (may be nil) light any placed
// healing crystals. Live-game-only diagnostics are skipped.
func DrawArea(camera rl.Camera3D, m *core.AreaDefinition, assets Resources, stepCount int, crystals []core.Crystal, levelVisible func(int) bool) {
	drawAreaWorld(camera, m, assets, stepCount, crystals, nil, levelVisible)
}

// worldFrameClock is rl.GetTime() sampled once at the top of the world render
// (drawWorld, DrawObjectPreview) and read by all per-tile sway/flicker math, so
// hundreds of props don't each make their own GetTime() cgo call. Set before any
// prop draw on every path, so it's never stale.
var worldFrameClock float32

// drawDecorFloor draws the decor char anchored to floor L of column (x,z),
// reporting whether anything drew (for the diagnostics counter). No-op on empty.
// Shared by the nil-stack fast path and the per-floor stacked loop.
func drawDecorFloor(assets Resources, m *core.AreaDefinition, decor byte, x, z int, cx, cz float32, L int) bool {
	if decor == core.DecorEmpty {
		return false
	}
	decorCenter := rl.NewVector3(cx, m.StandGroundYAt(x, L, z), cz)
	drawDecor(assets, decor, x, z, cx, cz, decorCenter)
	return true
}

// drawPropFloor draws the prop char anchored to floor L of column (x,z) via the
// inline/footprint/model dispatch, reporting whether a model actually drew.
func drawPropFloor(assets Resources, m *core.AreaDefinition, prop byte, x, z int, cx, cz float32, L int) bool {
	if prop == core.TilePropEmpty {
		return false
	}
	// Authored per-tile facing wins over the procedural hash yaw when set.
	propYaw := propYawDeg(x, z)
	if o, ok := m.PropYawOverride(x, z); ok {
		propYaw = o
	}
	propCenter := rl.NewVector3(cx, m.StandGroundYAt(x, L, z), cz)
	if handler := inlinePropTable[prop]; handler != nil {
		handler(assets, m, x, z, propCenter, propYaw)
		return true
	}
	if footprint := core.PropFootprint(prop); footprint != nil {
		if pm := &assets.propModelTable[prop]; pm.registered() {
			anchor := footprintAnchor(propCenter, footprint)
			if r := propShadowRadiusTable[prop]; r > 0 {
				drawGroundShadowElev(anchor.X, anchor.Z, anchor.Y, r)
			}
			pm.draw(anchor, propWorldScale, propYaw)
			return true
		}
		return false
	}
	if pm := &assets.propModelTable[prop]; pm.registered() {
		if r := propShadowRadiusTable[prop]; r > 0 {
			drawGroundShadowElev(propCenter.X, propCenter.Z, propCenter.Y, r)
		}
		pm.draw(propCenter, propWorldScale, propYaw)
		return true
	}
	return false
}

// levelShown reports whether elevation level L should render: a nil filter
// (in-game) shows everything; the editor passes its per-level eye state. One
// spelling so the gate can't drift in polarity across the floor/ceiling/cliff/
// voxel/decor/prop call sites.
func levelShown(levelVisible func(int) bool, L int) bool {
	return levelVisible == nil || levelVisible(L)
}

// drawWorld rasterizes the sky-less environment geometry, uploads lighting
// uniforms, and (when the render log is on) gathers per-tile diagnostics.
func drawAreaWorld(camera rl.Camera3D, m *core.AreaDefinition, assets Resources, stepCount int, crystals []core.Crystal, g *core.GameState, levelVisible func(int) bool) {
	if editorFreezeAnim {
		worldFrameClock = 0 // editor still-scene: freeze sway/flicker
	} else {
		worldFrameClock = float32(rl.GetTime())
	}
	material := assets.worldMaterial(m.Materials)
	// One content fingerprint per frame, shared by the enclosure, elevGrid, and
	// torch-site caches (each previously re-folded its own overlapping layer subset —
	// the Ceiling layer alone was folded three times/frame). Any editor edit changes
	// it and busts all three; in-game it's constant so none rebuild.
	contentToken := areaContentToken(m)
	profile := applyTimeOfDay(lightingFor(m.Materials), timeProfileAt(stepCount), areaIsEnclosed(m, contentToken))
	if editorClearView {
		profile = clarifyForEditor(profile)
	}
	cacheLightingProfile(profile)
	assets.lighting.applyUniforms(camera, profile)
	// Torch point lights: collect nearest braziers, flicker, upload. Must run
	// after applyUniforms (same shader) and before the tile loop draws.
	torches := collectTorches(m, crystals, camera, contentToken)
	assets.lighting.uploadTorches(torches)

	camPos := camera.Position
	vc := newViewCull(camera)

	// Diagnostics: collect counters only when the render log is on, so the hot
	// path stays increment-free otherwise.
	logActive := IsRenderLogActive()
	var stats renderFrameStats
	if logActive {
		stats.MapW = m.Width
		stats.MapH = m.Height
	}

	// Decode every tile's elevation/ramp/face-skin ONCE into a reused flat grid,
	// so the hot loop reads ints/bytes instead of re-decoding ~5× per tile per pass.
	gw, gh := m.Width, m.Height
	grid := elevGrid(m, gw, gh, contentToken)
	// Per-floor scatter: a legacy (nil-stack) column holds at most one prop + one
	// decor, each on its cached placed level (te.decorLevel/propLevel) — drawn ONCE
	// below, the pre-voxel cost. Only a materialized stack walks every floor, and
	// only then do we pay ScatterStackHeight's grid scan.
	hasScatter := len(m.PropStack) > 0 || len(m.DecorStack) > 0
	scatterH := 1
	if hasScatter {
		scatterH = m.ScatterStackHeight()
	}

	rl.BeginShaderMode(assets.lighting.shader)
	for z := 0; z < m.Height; z++ {
		for x := 0; x < m.Width; x++ {
			if logActive {
				stats.TilesIterated++
			}
			cx := core.TileCenter(x)
			cz := core.TileCenter(z)
			if vc.cullXZ(cx, cz) {
				if logActive {
					stats.TilesCulled++
				}
				continue
			}
			// Elevation: the tile's floor rides up by its level. The world is a
			// heightfield — a "wall" is the rendered vertical FACE of an elevation
			// step (drawCliffFaces), not a solid tile.
			te := grid[z*gw+x]
			elevY := core.ElevationWorldY(te.level)
			// Editor per-level hiding: an eye-toggled-off floor hides its TERRAIN
			// (floor slab, ramp wedge, cliff faces, ceiling) too — not just its
			// props/decor. levelVisible is nil in-game, so everything shows there.
			terrainVisible := levelShown(levelVisible, te.level)
			if m.CeilingAt(x, z) && terrainVisible {
				drawTileCube(material.ceilingModel, cx, core.LevelStep+elevY, cz, tileYawDeg(x, z))
				if logActive {
					stats.CeilingsDrawn++
				}
			}
			if len(m.Solids) > 0 {
				// Voxel path (gapped maps only): floors per standable surface, side
				// faces per solid run, floating-cube undersides. Per-level hiding is
				// resolved inside, cube by cube (a column spans many levels).
				nf, nw := drawVoxelColumn(camPos, material, assets, m, x, z, cx, cz, levelVisible)
				if logActive {
					stats.FloorsDrawn += nf
					stats.WallsDrawn += nw
				}
			} else if terrainVisible {
				drawFloorTile(material, assets, m.Floor[z][x], x, z, cx, cz, elevY)
				if logActive {
					stats.FloorsDrawn++
				}
				// Cliff faces for every edge above the neighbour/map edge.
				if n := drawCliffFaces(camPos, material, assets, grid, gw, gh, x, z, cx, cz, te.level, te.ramp); logActive {
					stats.WallsDrawn += n
				}
			}
			// Decor + props. A nil-stack column draws its single prop/decor once at
			// the cached placed level (fast path, identical to the pre-voxel cost);
			// a materialized stack walks every floor drawing each floor's content.
			if !hasScatter {
				if levelShown(levelVisible, te.decorLevel) {
					if drawDecorFloor(assets, m, m.DecorAt(x, te.decorLevel, z), x, z, cx, cz, te.decorLevel) && logActive {
						stats.DecorDrawn++
					}
				}
				if levelShown(levelVisible, te.propLevel) {
					if drawPropFloor(assets, m, m.PropAt(x, te.propLevel, z), x, z, cx, cz, te.propLevel) && logActive {
						stats.PropsDrawn++
					}
				}
			} else {
				for L := 0; L < scatterH; L++ {
					// Editor per-floor visibility: an eye-toggled-off floor hides its
					// props/decor (levelVisible is nil in-game — everything shows).
					if !levelShown(levelVisible, L) {
						continue
					}
					if drawDecorFloor(assets, m, m.DecorAt(x, L, z), x, z, cx, cz, L) && logActive {
						stats.DecorDrawn++
					}
					if drawPropFloor(assets, m, m.PropAt(x, L, z), x, z, cx, cz, L) && logActive {
						stats.PropsDrawn++
					}
				}
			}
		}
	}
	rl.EndShaderMode()

	if logActive {
		stats.FrameDT = rl.GetFrameTime()
		stats.TorchCount = len(torches)
		stats.CamPos = camera.Position
		stats.CamDir = rl.NewVector3(camera.Target.X-camera.Position.X, camera.Target.Y-camera.Position.Y, camera.Target.Z-camera.Position.Z)
		stats.CamFOV = camera.Fovy
		stats.StepCount = stepCount
		stats.LightingShaderID = assets.lighting.shader.ID
		stats.BillboardFogID = assets.billboardFog.shader.ID
		stats.FogDensity = profile.FogDensity
		stats.FogColor = profile.FogColor
		stats.AmbientColor = profile.AmbientColor
		stats.SunColor = profile.SunColor
		// Player/battle diagnostics only exist for the live game (nil in the editor).
		if g != nil {
			stats.PlayerYaw = g.Player.Yaw
			stats.PlayerLookYaw = g.Player.LookYaw
			stats.PlayerLookPitch = g.Player.LookPitch
			stats.BattleActive = g.Battle.Active()
		}
		LogRenderFrame(stats)
	}
}

// footprintAnchor returns the world centroid of a multi-tile footprint given the
// anchor tile's center. Wraps core.FootprintWorldOffset.
func footprintAnchor(center rl.Vector3, footprint []core.MultiTileOffset) rl.Vector3 {
	ox, oz := core.FootprintWorldOffset(footprint)
	return rl.NewVector3(center.X+ox, center.Y, center.Z+oz)
}

// drawFloorTile draws the floor variant for `cell`. Resolution order: universal
// special floors (any material) → material-specific dirt/dark-grass → per-tile
// hash variant / base floor. All render at the base-floor y so tiles meet flush.
func drawFloorTile(material worldMaterialResources, assets Resources, cell byte, x, z int, cx, cz, elevY float32) {
	yaw := tileYawDeg(x, z)
	// Slabs sit a hair below the elevation height to meet cliff faces without
	// z-fighting; shared by all three slab draws below.
	floorY := elevY - 0.03
	// Ramp tiles draw a solid earth wedge (elevY is their LOW edge) instead of a slab.
	if facing, ok := core.RampAscentFacing(cell); ok {
		drawRampWedge(assets.rampModel, cx, cz, elevY, facing)
		return
	}
	if t := assets.specialFloorTable; t.present[cell] {
		drawTileCube(t.model[cell], cx, floorY, cz, yaw)
		return
	}
	if !material.hasFloorVariant {
		drawTileCube(material.floorModel, cx, floorY, cz, yaw)
		return
	}
	// What reaches here is FloorAuto/unrecognized — pick a per-tile variant by hash.
	model := material.floorModel
	switch floorVariantHash(x, z) {
	case 1:
		model = material.floorDirtModel
	case 2:
		model = material.floorDarkModel
	}
	drawTileCube(model, cx, floorY, cz, yaw)
}

// tileElev is the per-tile elevation data the world loop needs, decoded once
// into elevGridBuf so the hot loop reads ints/bytes instead of re-running string
// lookups for every tile and its four neighbours.
type tileElev struct {
	level int
	ramp  int  // ascent facing, or core.NoRamp on a flat tile
	skin  byte // cliff-face skin char (core.FaceSkinAt)
	// faceSkins is the resolved skin char per cardinal direction (N=0/E=1/S=2/W=3):
	// the per-direction override (FaceSkinForDir) or base skin. Cached so the
	// FaceOverrides scan runs once per area, not per exposed edge per frame.
	faceSkins [core.FacingCount]byte
	// decorLevel/propLevel are the surfaces decor/props anchor to. Cached because
	// on a VOXEL map an auto-level tile resolves through an O(stackHeight) column
	// rescan that would otherwise run per visible tile per frame.
	decorLevel int
	propLevel  int
	// grassTop: the tile's floor reads as turf — gates the grass-crest face band
	// on the cliff below it.
	grassTop bool
}

// floorIsGrassy reports whether a floor char reads as turf (the auto variant
// resolves to grass-family in the field material).
func floorIsGrassy(c byte) bool {
	return c == core.FloorAuto || c == core.FloorGrass || c == core.FloorDarkGrass
}

// elevGridBuf is reused across frames + passes to avoid an allocation per draw.
var elevGridBuf []tileElev

// elevGridKey fingerprints the area elevGridBuf was decoded for so the full
// decode runs once per area, not per frame. Key is a CONTENT HASH of Floor +
// Elevation + Walls (+ Solids) plus name/dims — hashed in full (unlike the
// boundary-sampling sibling caches) since a stale grid would mis-render every
// height. The hash is an allocation-free byte fold, far cheaper than the decode.
var elevGridKey struct {
	primed        bool
	name          string
	width, height int
	hash          uint64
}

// foldLayer folds one layer's bytes into FNV-1a digest h with row + layer
// separators so ragged splits can't collide. Allocation-free.
func foldLayer(h uint64, layer []string) uint64 {
	for _, row := range layer {
		h = core.FoldLayerRow(h, row)
	}
	return (h ^ 0xfe) * core.FNVPrime64 // layer separator
}

// layersHash folds the given layers into one FNV-1a digest. Allocation-free.
func layersHash(layers ...[]string) uint64 {
	h := core.FNVOffset64
	for _, layer := range layers {
		h = foldLayer(h, layer)
	}
	return h
}

// foldFaceOverrides folds the per-tile face-skin overrides into digest h —
// they feed tileElev.faceSkins, so an override edit must bust the cache.
func foldFaceOverrides(h uint64, overrides []core.FaceOverride) uint64 {
	for _, o := range overrides {
		h = (h ^ uint64(o.X)) * core.FNVPrime64
		h = (h ^ uint64(o.Z)) * core.FNVPrime64
		for _, s := range o.Skins {
			h = (h ^ uint64(s)) * core.FNVPrime64
		}
	}
	return (h ^ 0xfd) * core.FNVPrime64 // layer separator
}

// areaContentToken folds every authorable layer any per-frame area cache derives
// from into ONE FNV digest, computed once per frame and shared by elevGrid and
// collectTorches — previously each re-folded its own overlapping subset (Floor/
// Elevation/Solids twice per frame). It's a superset of both dependency sets, so
// any editor edit busts both caches; in-game content never changes, so the token
// is constant and neither rebuilds. Allocation-free byte fold.
func areaContentToken(m *core.AreaDefinition) uint64 {
	h := foldLayer(foldLayer(foldLayer(core.FNVOffset64, m.Floor), m.Elevation), m.Walls)
	h = foldLayer(foldLayer(foldLayer(h, m.Decor), m.Props), m.Ceiling)
	for _, plane := range m.Solids {
		h = foldLayer(h, plane)
	}
	for _, plane := range m.PropStack {
		h = foldLayer(h, plane)
	}
	for _, plane := range m.DecorStack {
		h = foldLayer(h, plane)
	}
	h = foldLayer(foldLayer(h, m.PropLevels), m.DecorLevels)
	return foldFaceOverrides(h, m.FaceOverrides)
}

// elevGrid decodes every tile's level/ramp/skin into the reused flat buffer,
// rebuilding only when the content it caches (or dims/name) changes.
func elevGrid(m *core.AreaDefinition, w, h int, token uint64) []tileElev {
	// token (areaContentToken) fingerprints every layer a tileElev decodes FROM,
	// so an editor edit to any of them invalidates the cached grid.
	k := &elevGridKey
	if k.primed && k.name == m.Name && k.width == w && k.height == h &&
		k.hash == token && cap(elevGridBuf) >= w*h {
		return elevGridBuf[:w*h]
	}
	n := w * h
	if cap(elevGridBuf) < n {
		elevGridBuf = make([]tileElev, n)
	}
	elevGridBuf = elevGridBuf[:n]
	for z := 0; z < h; z++ {
		for x := 0; x < w; x++ {
			ramp := core.NoRamp
			if f, ok := m.RampAt(x, z); ok {
				ramp = f
			}
			// Resolve each face's skin once (override-or-base) so the cliff pass
			// never re-scans FaceOverrides. Index = direction (N=0/E=1/S=2/W=3).
			var faces [core.FacingCount]byte
			for d := 0; d < core.FacingCount; d++ {
				faces[d] = m.FaceSkinForDir(x, z, d)
			}
			elevGridBuf[z*w+x] = tileElev{
				level:      m.ElevationLevelAt(x, z),
				ramp:       ramp,
				skin:       m.FaceSkinAt(x, z),
				faceSkins:  faces,
				decorLevel: m.DecorLevelAt(x, z),
				propLevel:  m.PropLevelAt(x, z),
				grassTop:   floorIsGrassy(m.Floor[z][x]),
			}
		}
	}
	k.name, k.width, k.height, k.hash, k.primed = m.Name, w, h, token, true
	return elevGridBuf
}

// drawCliffFaces renders the vertical faces of tile (x,z) — one per cardinal
// edge where this tile sits above its neighbour (or the map edge). Ramp tiles
// draw their own wedge and are skipped. Returns the face count (WallsDrawn tally).
func drawCliffFaces(camPos rl.Vector3, material worldMaterialResources, assets Resources, grid []tileElev, w, h, x, z int, cx, cz float32, myLevel, myRamp int) int {
	if myRamp != core.NoRamp {
		return 0 // the ramp wedge supplies its own faces
	}
	drawn := 0
	tint := cliffFaceTint(x, z)
	for _, d := range core.CardinalDirs {
		dx, dz := core.FacingVector(d)
		// CPU backface cull: a face is only visible from its outward side, and a
		// dense heightfield issues one per exposed edge — skipping the DrawModelEx
		// saves the per-call cost.
		fdx, fdz := float32(dx), float32(dz)
		if faceBackfaceCulled(camPos, cx, cz, fdx, fdz, tileHalf) {
			continue
		}
		nx, nz := x+dx, z+dz
		// Neighbour edge level: ramp-aware (EdgeLevelOf) else flat level; off-map
		// = baseline so a raised border shows a clean lip, not a deep cliff.
		nLevel := core.ElevationBaseline
		if nx >= 0 && nx < w && nz >= 0 && nz < h {
			nt := grid[nz*w+nx]
			if l, ok := core.EdgeLevelOf(nt.level, nt.ramp, core.NormalizeFacing(d+2)); ok {
				nLevel = l
			} else {
				nLevel = nt.level
			}
		}
		if myLevel <= nLevel {
			continue
		}
		yaw := southFacingYaw(d)
		// Per-direction skin from the prebuilt grid (override or base).
		if sc := grid[z*w+x].faceSkins[d]; assets.faceVariantTable.present[sc] {
			// Skinned faces draw one quad PER LEVEL (their textures tile
			// vertically), matching the voxel path's density.
			skin := assets.faceVariantTable.model[sc]
			for L := nLevel; L < myLevel; L++ {
				drawCliffFace(skin, cx, core.ElevationWorldY(L), cz, yaw, 1, tint)
				drawn++
			}
			continue
		}
		if material.hasFaceBands {
			// Banded plain rock: turf crest on the top level, mossy foot on the
			// bottom, neutral mids — the cliff's vertical structure lands once.
			grassy := grid[z*w+x].grassTop
			for L := nLevel; L < myLevel; L++ {
				band := material.faceBands[cliffBandIndex(L == myLevel-1, L == nLevel, grassy)]
				drawCliffFace(band, cx, core.ElevationWorldY(L), cz, yaw, 1, tint)
				drawn++
			}
			continue
		}
		drawCliffFace(material.faceModel, cx, core.ElevationWorldY(nLevel), cz, yaw, float32(myLevel-nLevel), tint)
		drawn++
	}
	return drawn
}

// cliffTintTable holds the subtle warm/cool multiplier tints drawCliffFace mixes
// per tile — enough tonal drift to break the repeat across a long cliff wall,
// far too little to read as painted color.
var cliffTintTable = [8]rl.Color{
	rl.NewColor(255, 251, 244, 255), // faint warm
	rl.NewColor(244, 248, 255, 255), // faint cool
	rl.NewColor(255, 255, 255, 255), // neutral
	rl.NewColor(240, 235, 226, 255), // dim warm
	rl.NewColor(252, 248, 240, 255),
	rl.NewColor(236, 239, 246, 255), // dim cool
	rl.NewColor(247, 247, 247, 255),
	rl.NewColor(255, 249, 238, 255),
}

// cliffFaceTint picks a tile's cliff tint. One tint per tile (not per level) so
// a whole column stays coherent while neighbouring columns drift apart.
func cliffFaceTint(x, z int) rl.Color {
	return cliffTintTable[(tileHash(x, z)>>7)&7]
}

// faceYaw maps the dropping-edge direction to the Y-rotation orienting the
// face-quad (built on +Z/south) outward. +Z→(sinθ,cosθ): 0=S, 90=E, 180=N, 270=W.
// southFacingYaw maps a facing to the Y-rotation that points a +Z/south-authored
// mesh that way (S=0, E=90, N=180, W=270). Shared by the cliff face-quads and the
// doorframe mesh (doorYawDeg), which use the same +Z base orientation.
func southFacingYaw(d int) float32 {
	switch core.NormalizeFacing(d) {
	case core.South:
		return 0
	case core.East:
		return 90
	case core.North:
		return 180
	case core.West:
		return 270
	}
	return 0
}

// drawCliffFace draws one face-quad at tile center (cx,cz) with its base at
// baseY, yaw-rotated to the dropping edge and scaled vertically by the level
// delta. tint is the per-tile repetition-breaking wash (cliffFaceTint).
func drawCliffFace(model rl.Model, cx, baseY, cz, yaw, levels float32, tint rl.Color) {
	rl.DrawModelEx(model,
		rl.NewVector3(cx, baseY, cz),
		worldUp, yaw,
		rl.NewVector3(1, levels, 1), tint)
}

// triNormal returns the unit normal of triangle (a,b,c) by the right-hand rule
// (CCW → outward).
func triNormal(a, b, c rl.Vector3) rl.Vector3 {
	ux, uy, uz := b.X-a.X, b.Y-a.Y, b.Z-a.Z
	vx, vy, vz := c.X-a.X, c.Y-a.Y, c.Z-a.Z
	nx, ny, nz := uy*vz-uz*vy, uz*vx-ux*vz, ux*vy-uy*vx
	l := float32(math.Sqrt(float64(nx*nx + ny*ny + nz*nz)))
	if l > 0 {
		nx, ny, nz = nx/l, ny/l, nz/l
	}
	return rl.NewVector3(nx, ny, nz)
}

// rampFacingYaw maps a ramp's ascent facing to the Y-rotation orienting the
// wedge (built ascending toward -Z/north) that way.
func rampFacingYaw(facing int) float32 {
	switch facing {
	case core.North:
		return 0
	case core.West:
		return 90
	case core.South:
		return 180
	case core.East:
		return 270
	}
	return 0
}

// drawRampWedge draws the solid ramp wedge at (cx,cz) with its low edge at lowY,
// ascending toward `facing`; the high edge lands one LevelStep up, flush.
func drawRampWedge(model rl.Model, cx, cz, lowY float32, facing int) {
	drawYawedModel(model, cx, lowY, cz, rampFacingYaw(facing), rl.NewVector3(1, 1, 1))
}

// drawDecor renders a tile's floor-layer decoration. DecorAuto hash-scatters;
// explicit chars draw a specific prop. Bush/mushroom/pebble stay inline to keep
// their hand-tuned scales; the rest dispatch through decorModelTable.
func drawDecor(assets Resources, cell byte, x, z int, cx, cz float32, center rl.Vector3) {
	switch cell {
	case core.DecorEmpty:
		return
	case core.DecorAuto:
		drawFloorDecoration(assets, x, z, cx, cz, center.Y)
		return
	}
	// Inline decor (bush/mushroom/pebble) via inlineDecorTable — a [256] array so
	// the hot path is an index, not a map hash.
	if handler := inlineDecorTable[cell]; handler != nil {
		handler(assets, x, z, cx, cz, center.Y)
		return
	}
	if footprint := core.DecorFootprint(cell); footprint != nil {
		if dm := &assets.decorModelTable[cell]; dm.registered() {
			dm.draw(footprintAnchor(center, footprint), 1.0, 0)
		}
		return
	}
	if dm := &assets.decorModelTable[cell]; dm.registered() {
		dm.draw(center, 1.0, propYawDeg(x, z))
	}
}

// propWorldScale shrinks every world prop a touch (the foliage + the model props)
// for a less oversized scene. The giant tree (TileTreeXL) is exempt — it stays the
// full-size canopy landmark — and the wall torch is exempt (a wall-mounted fixture,
// not free-standing clutter).
const propWorldScale = float32(0.9)

// treePropScales is the scale-per-char table for tree variants that share
// assets.tree at different sizes (Tree/TreeXL/TreeTall/TreeYoung dispatch through
// drawPropTreeScaled; TileTreeTwin is separate, drawing two per tile).
var treePropScales = map[byte]float32{
	core.TileTree:      1.00,
	core.TileTreeXL:    1.75,
	core.TileTreeTall:  1.40,
	core.TileTreeYoung: 0.92,
}

// foliageTrunkShadowFactor is the fraction of a foliage prop's scale its
// ground-shadow disc spans (before per-prop slack); shared by the tree props.
const foliageTrunkShadowFactor = 0.34

// foliageShadowRadius returns a foliage prop's ground-shadow radius at the given
// scale plus slack.
func foliageShadowRadius(scale, slack float32) float32 {
	return foliageTrunkShadowFactor*scale + slack
}

// drawPropTreeScaled draws assets.tree at the char's treePropScales scale.
// Per-tile shape variance is seeded from tileHash so a stand of identical-char
// trees doesn't read as a stamped grid.
func drawPropTreeScaled(char byte) inlinePropRenderer {
	scale := treePropScales[char]
	if char != core.TileTreeXL { // giant trees keep full size; everything else shrinks
		scale *= propWorldScale
	}
	return func(assets Resources, _ *core.AreaDefinition, x, z int, center rl.Vector3, propYaw float32) {
		drawGroundShadowElev(center.X, center.Z, center.Y, foliageShadowRadius(scale, 0.10))
		assets.tree.drawVaried(center, scale, propYaw, tileHash(x, z))
	}
}

// drawPropTreeTwin renders two diagonally-offset trees of different scales in one
// tile ("big tree with a younger one beside it"). Both reuse assets.tree.
func drawPropTreeTwin(assets Resources, _ *core.AreaDefinition, x, z int, center rl.Vector3, propYaw float32) {
	const offset = 0.32
	const scaleBig = 0.82 * propWorldScale
	const scaleSmall = 0.58 * propWorldScale
	seed := tileHash(x, z)
	left := rl.NewVector3(center.X-offset, center.Y, center.Z-offset)
	right := rl.NewVector3(center.X+offset, center.Y, center.Z+offset)
	drawGroundShadowElev(left.X, left.Z, center.Y, foliageShadowRadius(scaleBig, 0.08))
	drawGroundShadowElev(right.X, right.Z, center.Y, foliageShadowRadius(scaleSmall, 0.08))
	if seed&1 == 0 {
		assets.tree.drawVaried(left, scaleBig, propYaw, seed)
		assets.tree.drawVaried(right, scaleSmall, propYaw+1.1, seed^hashSalt)
	} else {
		assets.tree.drawVaried(left, scaleSmall, propYaw, seed)
		assets.tree.drawVaried(right, scaleBig, propYaw+1.1, seed^hashSalt)
	}
}

func drawPropRockLarge(assets Resources, _ *core.AreaDefinition, _, _ int, center rl.Vector3, propYaw float32) {
	drawGroundShadowElev(center.X, center.Z, center.Y, 0.42)
	assets.rockProp.draw(center, propWorldScale, propYaw)
}

func drawPropBushLarge(assets Resources, _ *core.AreaDefinition, _, _ int, center rl.Vector3, propYaw float32) {
	drawGroundShadowElev(center.X, center.Z, center.Y, 0.48)
	assets.bushProp.draw(center, 1.3*propWorldScale, propYaw)
}

// Wall-torch fixture geometry — shared by drawWallTorch and rebuildTorchSites so
// the visible fixture and its light origin can't drift apart on a retune.
const (
	wallTorchMount      = float32(0.40) // bracket distance from tile center toward the wall
	wallTorchSconceY    = float32(1.30) // bracket/sconce height
	wallTorchLightY     = float32(1.42) // light-pool origin height (just above the flame cup)
	wallTorchLightInset = float32(0.30) // light origin offset from tile center toward the wall
)

// drawWallTorch is the inline handler for TileTorch: auto-orients to the adjacent
// wall (facing into the room), draws an unlit iron bracket/sconce and an animated
// emissive flame. The point light itself comes from collectTorches. Non-blocking.
func drawWallTorch(assets Resources, m *core.AreaDefinition, x, z int, center rl.Vector3, _ float32) {
	fx, fz := wallTorchFacing(m, x, z)
	// Mount against the wall behind the torch (opposite the facing direction).
	wallX := center.X - fx*wallTorchMount
	wallZ := center.Z - fz*wallTorchMount
	// Heights ride the tile's elevation floor so a raised torch stays wall-mounted.
	baseY := center.Y

	// Iron bracket: dark cube flush on the wall + a short arm holding the sconce.
	bracket := rl.NewVector3(wallX, baseY+wallTorchSconceY-0.12, wallZ)
	rl.DrawCube(bracket, 0.10, 0.22, 0.10, torchIron)
	armX := wallX + fx*0.10
	armZ := wallZ + fz*0.10
	rl.DrawCube(rl.NewVector3(armX, baseY+wallTorchSconceY, armZ), 0.08, 0.06, 0.08, torchIron)
	// Sconce cup at the arm tip.
	cupX := wallX + fx*0.16
	cupZ := wallZ + fz*0.16
	rl.DrawCube(rl.NewVector3(cupX, baseY+wallTorchSconceY+0.04, cupZ), 0.16, 0.08, 0.16, torchIronLight)

	// Animated flame: three emissive blobs above the cup, each bobbing on its own
	// offset. Unlit model (default shader) so they glow against the dark ambient.
	if !torchFlameReady {
		return
	}
	t := worldFrameClock
	phase := hashPhase(tileHash(x, z))
	flameBaseX := cupX
	flameBaseZ := cupZ
	for i := 0; i < 3; i++ {
		fp := phase + float32(i)*2.1
		bob := float32(math.Sin(float64(t*7.0+fp))) * 0.04
		swayA := float32(math.Sin(float64(t*5.3+fp*1.4))) * 0.05
		// Higher blobs are smaller and lean more — a wavering teardrop.
		y := baseY + wallTorchSconceY + 0.09 + float32(i)*0.07 + bob
		lean := float32(i) * 0.03
		px := flameBaseX + fx*lean + swayA*fz
		pz := flameBaseZ + fz*lean - swayA*fx
		size := (0.11 - float32(i)*0.025) * (1 + 0.12*float32(math.Sin(float64(t*11.0+fp))))
		tint := torchFlameTints[i]
		rl.DrawModelEx(torchFlameModel, rl.NewVector3(px, y, pz),
			worldUp, 0, rl.NewVector3(size, size*1.4, size), tint)
	}
}

// wallTorchFacing returns the unit (x,z) the torch faces — away from the first
// adjacent wall (N→E→S→W), or south when the tile has no adjacent wall.
func wallTorchFacing(m *core.AreaDefinition, x, z int) (float32, float32) {
	if f, ok := core.FacingAwayFromAdjacentWall(m, x, z); ok {
		dx, dz := core.FacingVector(f)
		return float32(dx), float32(dz)
	}
	return 0, 1 // no adjacent wall → face south (toward the usual approach)
}

// groundShadowModel is the soft disc painted under every prop: an UNLIT XZ plane
// (keeps the default shader, untouched by the lighting pass) alpha-blended over
// the floor. A package singleton because many free-function call sites paint
// shadows without a Resources handle. groundShadowReady guards the pre-init window.
var (
	groundShadowModel rl.Model
	groundShadowReady bool
)

// propShadowRadius is the per-prop ground-shadow half-extent (world units) for
// table-dispatched props (inline-handled props paint their own). Roughly the
// prop's footprint plus slack; 2×2 props get a wider radius.
var propShadowRadius = map[byte]float32{
	core.TileCrate:             0.42,
	core.TileBarrel:            0.36,
	core.TileUrn:               0.28,
	core.TileStalagmite:        0.30,
	core.TilePillar:            0.34,
	core.TileBrokenPillar:      0.34,
	core.TileStatue:            0.42,
	core.TileObelisk:           0.34,
	core.TileFountain:          0.50,
	core.TileRockCairn:         0.36,
	core.TileRockFormation:     0.95,
	core.TileRockFormationTail: 0,
	core.TileWell:              0.42,
	core.TileGravestone:        0.28,
	core.TileSignPost:          0.18,
	core.TileHayBale:           0.40,
	core.TileScarecrow:         0.26,
	core.TileBookshelf:         0.36,
	core.TileTable:             0.42,
	core.TileBed:               0.46,
	core.TileBrazier:           0.32,
	core.TileSarcophagus:       0.50,
}

// propShadowRadiusTable is the [256]float32 mirror of propShadowRadius so the
// world loop does an O(1) index, not a map hash. Chars with no entry read 0.
var propShadowRadiusTable = func() [256]float32 {
	var t [256]float32
	for ch, r := range propShadowRadius {
		t[ch] = r
	}
	return t
}()

// areaKey identifies the area a per-area cache was built for. Matched on name +
// dims PLUS the per-frame areaContentToken (a superset of every authorable layer),
// so two same-named/sized areas with different content (the editor "untitled" case)
// can't share a stale verdict — and the caller's already-computed token is reused
// instead of re-folding layers here. Used by enclosureCache and torchSiteCache.
type areaKey struct {
	name          string
	width, height int
	contentHash   uint64
	primed        bool
}

func (k *areaKey) matches(m *core.AreaDefinition, token uint64) bool {
	return k.primed && k.name == m.Name && k.width == m.Width && k.height == m.Height &&
		k.contentHash == token
}

func (k *areaKey) set(m *core.AreaDefinition, token uint64) {
	k.contentHash = token
	k.name, k.width, k.height, k.primed = m.Name, m.Width, m.Height, true
}

// enclosureCache memoizes the enclosure result so the ceiling scan runs once per
// area, not per frame.
var enclosureCache struct {
	areaKey
	enclosed bool
}

// areaIsEnclosed reports whether the area is a roofed interior (gates the
// dungeon lighting override), memoizing core.AreaIsOutdoor per area.
func areaIsEnclosed(m *core.AreaDefinition, token uint64) bool {
	if enclosureCache.matches(m, token) {
		return enclosureCache.enclosed
	}
	enclosed := !core.AreaIsOutdoor(m)
	enclosureCache.set(m, token)
	enclosureCache.enclosed = enclosed
	return enclosed
}

// torchFlameModel is the unlit emissive sphere for flame blobs (default shader so
// it glows at full tint against the dark dungeon). Set by LoadResources.
var (
	torchFlameModel rl.Model
	torchFlameReady bool
)

// torchFlameHeight is the world Y a brazier's point light sits at — up at the
// fire bowl so the pool radiates outward and down.
const torchFlameHeight = float32(1.05)

type torchCandidate struct {
	pos     rl.Vector3
	dist    float32
	hash    uint32
	bright  float32    // brightness multiplier — braziers > wall torches
	color   rl.Vector3 // base tint before flicker/breathe (warm flame vs crystal cyan)
	crystal bool       // a charged crystal: cyan, breathes with the gem (no flicker)
}

// torchCandidateBuf / torchResultBuf are reused so the per-frame scan + build
// don't allocate.
var (
	torchCandidateBuf []torchCandidate
	torchResultBuf    []torchLight
)

// torchSite is the static (camera/time-independent) data for one brazier/torch
// tile: light origin, tile center for ranking, flicker seed, base brightness.
// Fixed for the area's lifetime, so it's cached rather than rescanned per frame.
type torchSite struct {
	pos    rl.Vector3
	cx, cz float32
	hash   uint32
	bright float32
}

// torchSiteCache memoizes the brazier/torch tile list so the grid scan runs once
// per area; per-frame work is then just distance + flicker over the cached sites.
// The embedded areaKey fingerprints content via the shared token, so an editor
// prop/elevation edit (same name/dims) rebuilds rather than serving stale lights.
var torchSiteCache struct {
	areaKey
	sites []torchSite
}

func rebuildTorchSites(m *core.AreaDefinition, token uint64) {
	torchSiteCache.sites = torchSiteCache.sites[:0]
	// Per-floor scan via PropAt — the legacy Props grid is FROZEN once a
	// PropStack is materialized (floors.go), so indexing it directly would both
	// miss editor-placed torches and light stale entries.
	floors := max(m.ScatterStackHeight(), 1)
	for z := 0; z < m.Height; z++ {
		for x := 0; x < m.Width; x++ {
			for level := 0; level < floors; level++ {
				prop := m.PropAt(x, level, z)
				isBrazier := prop == core.TileBrazier
				isTorch := prop == core.TileTorch
				if !isBrazier && !isTorch {
					continue
				}
				cx := core.TileCenter(x)
				cz := core.TileCenter(z)
				// Light origin rides the fixture's OWN floor height so a raised
				// torch lights at its actual flame height.
				elevY := m.StandGroundYAt(x, level, z)
				var pos rl.Vector3
				bright := float32(0.85) // wall torch — dimmer
				if isBrazier {
					// Floor brazier: flame at the bowl, brighter pool.
					pos = rl.NewVector3(cx, elevY+torchFlameHeight, cz)
					bright = 1.45
				} else {
					// Wall torch: light at the sconce, offset toward the wall + up.
					fx, fz := wallTorchFacing(m, x, z)
					pos = rl.NewVector3(cx-fx*wallTorchLightInset, elevY+wallTorchLightY, cz-fz*wallTorchLightInset)
				}
				torchSiteCache.sites = append(torchSiteCache.sites, torchSite{
					pos: pos, cx: cx, cz: cz, hash: tileHash(x, z), bright: bright,
				})
			}
		}
	}
	torchSiteCache.set(m, token)
}

// torchFlicker is the warm light-pool brightness in ~0.72..1.0 from two desynced
// sines over the per-torch phase — the glow magnitude only (the flame-blob bob in
// drawWallTorch is a separate animation). Named so the freqs/weights live in one place.
const (
	torchFlickerBias    = 0.86
	torchFlickerWeightA = 0.09
	torchFlickerWeightB = 0.05
	torchFlickerHzA     = 9.3
	torchFlickerHzB     = 17.1
)

func torchFlicker(t, phase float32) float32 {
	return torchFlickerBias +
		torchFlickerWeightA*float32(math.Sin(float64(t*torchFlickerHzA+phase))) +
		torchFlickerWeightB*float32(math.Sin(float64(t*torchFlickerHzB+phase*1.7)))
}

// collectTorches returns the maxTorches nearest point lights — braziers/torches AND
// charged healing crystals — as the camera sees them (the rest are fog-swallowed).
// Torches flicker warm (per-torch desynced sines); crystals breathe cool cyan in
// lockstep with the gem body. Both share one distance-ranked pool, so a near crystal
// can out-prioritise a far torch (Grimrock-style: a live crystal is just a light).
func collectTorches(m *core.AreaDefinition, crystals []core.Crystal, camera rl.Camera3D, token uint64) []torchLight {
	if !torchSiteCache.matches(m, token) {
		rebuildTorchSites(m, token)
	}
	torchCandidateBuf = torchCandidateBuf[:0]
	for _, s := range torchSiteCache.sites {
		dx := s.cx - camera.Position.X
		dz := s.cz - camera.Position.Z
		torchCandidateBuf = append(torchCandidateBuf, torchCandidate{
			pos:    s.pos,
			dist:   dx*dx + dz*dz,
			hash:   s.hash,
			bright: s.bright,
			color:  torchBaseColor,
		})
	}
	// Charged crystals are dynamic (state lives in g, not the area cache), so append
	// them fresh each frame. Light origin rides the gem's float height.
	for _, c := range crystals {
		if !c.Charged {
			continue
		}
		base := tileWorldPos(c.TileX, c.TileZ, m.StandGroundYAt(c.TileX, c.Level, c.TileZ))
		dx := base.X - camera.Position.X
		dz := base.Z - camera.Position.Z
		torchCandidateBuf = append(torchCandidateBuf, torchCandidate{
			pos:     rl.NewVector3(base.X, base.Y+crystalGeo.FloatY, base.Z),
			dist:    dx*dx + dz*dz,
			bright:  1,
			color:   crystalLightColor,
			crystal: true,
		})
	}
	torchResultBuf = torchResultBuf[:0]
	if len(torchCandidateBuf) == 0 {
		return torchResultBuf
	}
	// Sort only when over the cap — most dungeons skip it.
	if len(torchCandidateBuf) > maxTorches {
		sort.Slice(torchCandidateBuf, func(a, b int) bool {
			return torchCandidateBuf[a].dist < torchCandidateBuf[b].dist
		})
	}
	n := len(torchCandidateBuf)
	if n > maxTorches {
		n = maxTorches
	}
	t := worldFrameClock // set by drawWorld before this collect runs
	breathe := crystalBreathe() // crystals breathe with the gem body
	for i := 0; i < n; i++ {
		c := torchCandidateBuf[i]
		var mag float32
		if c.crystal {
			mag = breathe * c.bright
		} else {
			mag = torchFlicker(t, hashPhase(c.hash)) * c.bright
		}
		torchResultBuf = append(torchResultBuf, torchLight{
			Pos: c.pos,
			Color: rl.NewVector3(
				c.color.X*mag,
				c.color.Y*mag,
				c.color.Z*mag,
			),
		})
	}
	return torchResultBuf
}

// groundShadowFloorClearance lifts the contact disc just above the floor so it
// composites without z-fighting.
const groundShadowFloorClearance = float32(0.02)

// drawGroundShadow paints the soft contact disc on the ground plane at `radius`
// half-extent (world units), anchoring a prop so it reads as planted.
func drawGroundShadow(cx, cz, radius float32) {
	drawGroundShadowAt(cx, groundShadowFloorClearance, cz, radius)
}

// drawGroundShadowAt is drawGroundShadow with an explicit Y, for a disc on a
// raised tile's floor.
func drawGroundShadowAt(cx, cy, cz, radius float32) {
	if !groundShadowReady || radius <= 0 {
		return
	}
	rl.DrawModelEx(
		groundShadowModel,
		rl.NewVector3(cx, cy, cz),
		worldUp, 0,
		rl.NewVector3(radius*2, 1, radius*2),
		rl.White,
	)
}

// drawGroundShadowElev draws a contact disc on a tile whose floor sits at groundY
// (its elevation), so props on a raised tile don't cast onto the floor below.
func drawGroundShadowElev(cx, cz, groundY, radius float32) {
	drawGroundShadowAt(cx, groundY+groundShadowFloorClearance, cz, radius)
}

// drawDecorBush / drawDecorMushroom / drawDecorPebble are the inline-decor
// handlers (uniform signature). groundY is the tile's elevation floor.
func drawDecorBush(assets Resources, x, z int, cx, cz, groundY float32) {
	drawGroundShadowElev(cx, cz, groundY, 0.36)
	assets.bushProp.draw(rl.NewVector3(cx, groundY, cz), 0.75, propYawDeg(x, z))
}

func drawDecorMushroom(assets Resources, x, z int, cx, cz, groundY float32) {
	drawGroundShadowElev(cx, cz, groundY, 0.20)
	assets.mushroomProp.draw(rl.NewVector3(cx, groundY, cz), 1.0, propYawDeg(x, z))
}

func drawDecorPebble(assets Resources, x, z int, cx, cz, groundY float32) {
	// Same hash as drawFloorDecoration's pebble scatter so an author-placed pebble
	// tile and a hash-scattered one share the identical cluster layout per tile.
	drawPebbleCluster(assets, cx, cz, groundY, hashXY(x, z))
}

// faceBackfaceCulled reports whether a vertical face (edge center cx+fdx*half,
// cz+fdz*half, outward normal fdx,fdz) faces away from the camera. Shared by
// drawCliffFaces and the voxel side-face pass.
func faceBackfaceCulled(camPos rl.Vector3, cx, cz, fdx, fdz, half float32) bool {
	return (camPos.X-(cx+fdx*half))*fdx+(camPos.Z-(cz+fdz*half))*fdz <= 0
}

// drawYawedModel draws model at (cx,cy,cz) rotated yawDeg about the vertical axis
// and scaled by `scale`, untinted. Shared by drawTileCube / drawCliffFace /
// drawRampWedge, which differ only in the scale vector.
func drawYawedModel(model rl.Model, cx, cy, cz, yawDeg float32, scale rl.Vector3) {
	rl.DrawModelEx(model,
		rl.NewVector3(cx, cy, cz),
		worldUp, yawDeg,
		scale, rl.White)
}

// drawTileCube draws a square-footprint cube at (cx,cy,cz) yaw-rotated about its
// vertical axis — 90° steps spin the texture to break tiling without changing the
// silhouette.
func drawTileCube(model rl.Model, cx, cy, cz, yawDeg float32) {
	drawYawedModel(model, cx, cy, cz, yawDeg, rl.NewVector3(1, 1, 1))
}

// tileHash is the stable per-tile uint32 mixer for orientation/variant selection.
// Stronger avalanche than hashXY (three xorshift+multiply rounds) so neighbouring
// tiles feel independent.
func tileHash(x, z int) uint32 {
	h := uint32(x*374761393) ^ uint32(z*668265263)
	h ^= h >> 16
	h *= 2246822519
	h ^= h >> 13
	h *= 3266489917
	h ^= h >> 16
	return h
}

// hashXY is the cheaper per-tile hash where tileHash's avalanche is overkill
// (pixel jitter, region-bucketed variant picks).
func hashXY(x, y int) uint32 {
	return mix32(uint32(x*73856093) ^ uint32(y*19349663))
}

// mix32 finalizes a uint32 with one xorshift + odd-prime multiply round —
// enough for visual variation (tileHash uses three).
func mix32(n uint32) uint32 {
	n ^= n >> 13
	n *= 1274126177
	n ^= n >> 16
	return n
}

// hash01 maps an index to a stable pseudo-random [0,1) via mix32 + low-24-bit
// normalize.
func hash01(n uint32) float32 {
	return float32(mix32(n)&0xffffff) / float32(0x1000000)
}

// tileYawDeg returns 0/90/180/270 for a tile — spins the texture to kill the
// tiling pattern (the square cube looks identical at any of these).
func tileYawDeg(x, z int) float32 {
	return float32(tileHash(x, z)&0x03) * 90
}

// steppedYaw30 maps a hash to one of core.PropYawSteps facings (30° steps) in
// [0,360) so a prop reads as a deliberate facing rather than noise. Shares the
// step count with authored PropYaw overrides so both land on the same grid.
// Caller pre-shifts to pick bits.
func steppedYaw30(h uint32) float32 {
	return core.PropYawDegForStep(int(h % uint32(core.PropYawSteps)))
}

// propYawDeg returns a per-tile yaw in 30° steps, in [0,360) — stepped so each
// prop reads as a deliberate facing rather than noise.
func propYawDeg(x, z int) float32 {
	return steppedYaw30(tileHash(x, z) >> 3)
}

// floorVariantHash picks 0 (grass) / 1 (dirt) / 2 (dark grass), region-bucketed
// (every 3 tiles) so variants form patches, not per-tile speckle.
func floorVariantHash(x, z int) int {
	region := hashXY(x/3, z/3)
	switch {
	case region&0xFF < 38: // ~15% dirt patches
		return 1
	case (region>>8)&0xFF < 55: // ~21% dark grass patches
		return 2
	default:
		return 0
	}
}

// scatterOffsetDivisor maps a signed int8 hash byte into a sub-tile offset of
// ~[-0.55, 0.55] world units. Shared by the floor-decoration + pebble scatterers.
const scatterOffsetDivisor = float32(230)

// scatterOffset turns a hash byte into a signed sub-tile offset via scatterOffsetDivisor.
func scatterOffset(b byte) float32 { return float32(int8(b)) / scatterOffsetDivisor }

// drawFloorDecoration scatters small passable props (rocks/bushes/mushrooms) on
// ~16% of plain floor tiles by per-tile hash; rocks weighted heaviest so the
// floor reads as pebble-strewn.
func drawFloorDecoration(assets Resources, x, z int, cx, cz, groundY float32) {
	h := hashXY(x, z)
	chance := byte(h)
	if chance > 42 { // ~16.5% rate
		return
	}
	// Weighted kind dispatch: 4/8 pebbles, 1/8 + 1/8 bush, 2/8 mushrooms.
	kind := int((h >> 8) & 7)
	// Sub-tile offset so the prop isn't dead-center.
	pos := rl.NewVector3(cx+scatterOffset(byte(h>>16)), groundY, cz+scatterOffset(byte(h>>24)))

	// Stable yaw from the same hash so clustered props aren't aligned.
	decoYaw := steppedYaw30(h >> 12)
	switch kind {
	case 0, 1, 2, 3: // pebble cluster — see drawPebbleCluster comment
		drawPebbleCluster(assets, cx, cz, groundY, h)
	case 4: // small bush
		assets.bushProp.draw(pos, 0.75, decoYaw)
	case 5: // tiny mushroom
		assets.mushroomProp.draw(pos, 0.65, decoYaw)
	case 6: // small mushroom
		assets.mushroomProp.draw(pos, 1.05, decoYaw)
	case 7: // small bush variant
		assets.bushProp.draw(pos, 0.6, decoYaw)
	}
}

// pebblePaletteTints is the light "weathered surface stone" palette for ground
// pebble scatter, indexed by per-pebble hash.
var pebblePaletteTints = [4]rl.Color{
	rl.NewColor(228, 224, 214, 255),
	rl.NewColor(216, 212, 202, 255),
	rl.NewColor(232, 226, 214, 255),
	rl.NewColor(220, 216, 208, 255),
}

// drawPebbleCluster paints a 2..4 low-profile pebble scatter (boulder base cube,
// lighter "surface stone" tint), each member's footprint/height/yaw from a
// per-pebble hash so it reads as ground detail, not dropped boulders.
func drawPebbleCluster(assets Resources, cx, cz, groundY float32, tileHash uint32) {
	if len(assets.rockProp.models) == 0 {
		return
	}
	baseModel := assets.rockProp.models[0]
	rotationAxis := worldUp

	// 2..4 per cluster, 25/50/25 center-weighted.
	count := 2 + int(tileHash&0x01) + int((tileHash>>1)&0x01)

	for i := 0; i < count; i++ {
		// Salt with the index so each member looks independent.
		ih := mix32(tileHash ^ uint32(i+1)*hashSalt)

		ox := scatterOffset(byte(ih))
		oz := scatterOffset(byte(ih >> 8))

		// Height ~1/3 of footprint so pebbles sit flat / walkable.
		foot := 0.18 + float32((ih>>16)&0x07)*0.012   // 0.18 .. 0.27
		hght := 0.07 + float32((ih>>20)&0x03)*0.012   // 0.07 .. 0.106
		rot := float32((ih>>24)&0xff) * (360.0 / 256) // 0..360°
		// x/z asymmetry from a different hash bit so it's uncorrelated to size.
		stretch := 0.85 + float32((ih>>4)&0x07)*0.04 // 0.85 .. 1.13

		// Replicate propModel's half-height ground offset since we draw the mesh
		// directly: RockMeshBaseHalfHeight * hght.
		pos := rl.NewVector3(cx+ox, groundY+RockMeshBaseHalfHeight*hght, cz+oz)
		scale := rl.NewVector3(foot, hght, foot*stretch)
		tint := pebblePaletteTints[(ih>>28)&0x03]
		rl.DrawModelEx(baseModel, pos, rotationAxis, rot, scale, tint)
	}
}

func DrawEnemies(camera rl.Camera3D, g *core.GameState, assets Resources) {
	if g.Battle.Phase == core.BattleNone {
		// Debug enemies-off hides field packs so the map can be walked clean.
		if g.EnemiesDisabled {
			return
		}
		drawFieldPacks(camera, g, assets)
		return
	}
	drawBattlePack(camera, g, assets)
}

// enemyBillboardY is the y-anchor for every enemy/pack billboard — its
// half-height so the sprite's bottom meets the floor.
const enemyBillboardY = float32(0.68)

// battleFormationCenterY is the vertical center enemy billboards render at in
// battle, lifted above enemyBillboardY so the roster sits screen-centered under
// the narrow FOV. enemyFieldLift is the delta the field draw adds back for
// yOffset-grounded sprites (calibrated against the battle center).
const battleFormationCenterY = float32(1.0)
const enemyFieldLift = battleFormationCenterY - enemyBillboardY

// downedToppleDegrees rotates a downed party member's billboard flat on the ground
// (90° = lying on its side, read as collapsed).
const downedToppleDegrees = float32(90)

// The foe foot line lives in battleTune.FoeFloorY (Debug ▸ Combat Tuning): drawBattlePack
// foot-anchors every foe to it — sprite center = FoeFloorY + size.Y/2 — so a short rat
// and a tall goblin both stand on the ground instead of sharing a center that
// levitates the short one.

// Party billboard sizes: idle silhouette + the active-actor "your turn" bump.
var (
	partyBillboardSize       = rl.NewVector2(0.38, enemyBillboardY)
	partyBillboardSizeActive = rl.NewVector2(0.42, 0.72)
	// partyActiveScale is the active/idle size ratio; DrawPartySprites multiplies
	// the (possibly overridden) base size by it so the bump scales the tuned
	// sprite and reproduces partyBillboardSizeActive when the base is default.
	partyActiveScale = rl.NewVector2(partyBillboardSizeActive.X/partyBillboardSize.X, partyBillboardSizeActive.Y/partyBillboardSize.Y)
)

// drawFieldPacks renders one billboard per pack (the highest-tier member, at the
// pack's tile). Empty/all-dead packs are skipped.
func drawFieldPacks(camera rl.Camera3D, g *core.GameState, assets Resources) {
	// Billboard distance fog needs a custom shader — multiplicative tint (raylib's
	// only billboard knob) can't lerp toward the fog color. beginBillboardFogPass
	// uploads the uniforms; the returned func is the matching EndShaderMode.
	defer beginBillboardFogPass(camera, g, assets)()
	for _, pack := range g.Packs {
		if !core.PackAlive(pack) {
			continue
		}
		visual, ok := enemyVisualFor(assets, core.PackLeaderKind(pack))
		if !ok {
			continue
		}
		// pack.X/Z is always authoritative (seeded + eased + snapped elsewhere).
		// StandGroundYAt(pack.Level) keeps a pack on the surface it walks.
		groundY := g.Area.StandGroundYAt(pack.TileX, pack.Level, pack.TileZ)
		position := rl.NewVector3(pack.X, enemyBillboardY+groundY, pack.Z)
		if visual.shadowRadius > 0 {
			sx, sz := shadowFootprint(camera, position, &visual)
			drawGroundShadowAt(sx, groundY+groundShadowFloorClearance, sz, visual.shadowRadius)
		}
		billboardPos := position
		billboardPos.Y += visual.yOffset
		// yOffset is calibrated against the lifted battle center; the field anchor
		// sits enemyFieldLift lower, so add it back (only when yOffset is set).
		if visual.yOffset != 0 {
			billboardPos.Y += enemyFieldLift
		}
		// Same fake-light treatment as battle (rim + volume) so field foes read solid
		// against the lit world, not flat cutouts.
		drawShadedBillboard(camera, visual.texture, billboardPos, visual.size, visual.resolveTint(), battleSpriteLight(g))
	}
}

// billboardPlacement bundles the derived draw positions for a per-kind billboard
// once depthOffset/markerOffset/yOffset are applied. Shared by drawBattlePack and
// the editor Foe Visualizer so the placement sequence lives in one place.
type billboardPlacement struct {
	base    rl.Vector3 // formation position after the depthOffset push-back
	shadowX float32    // contact-disc footprint (camera-relative shadowOffset folded in)
	shadowZ float32
	chevron rl.Vector3 // target-cursor anchor (markerY/X applied)
	sprite  rl.Vector3 // billboard center (yOffset applied)
}

// resolveBillboardPlacement applies depthOffset, then derives the shadow
// footprint, chevron anchor (markerY/X), and sprite center (yOffset) — the
// ordering drawBattlePack and DrawFoePreview share.
func resolveBillboardPlacement(camera rl.Camera3D, position rl.Vector3, v *enemyVisual) billboardPlacement {
	base := cameraRelativeOffset(camera, position, 0, 0, v.depthOffset)
	sx, sz := shadowFootprint(camera, base, v)
	chevron := cameraRelativeOffset(camera, base, v.markerXOffset, v.markerYOffset, 0)
	sprite := base
	sprite.Y += v.yOffset
	return billboardPlacement{base: base, shadowX: sx, shadowZ: sz, chevron: chevron, sprite: sprite}
}

// drawBattlePack renders every member of the active pack in battle
// formation: living and recently-defeated (still fading) alike.
func drawBattlePack(camera rl.Camera3D, g *core.GameState, assets Resources) {
	defer beginBillboardFogPass(camera, g, assets)()
	members := core.BattleMembers(g)
	// Resolve every slot in one O(n) pass and index it, vs a per-member
	// enemyRowSlot that re-walked the pack (O(n²)).
	placements := enemyRowPlacements(members)
	// Painter's order for the depth-test-off battle pass: the farther BACK rank first,
	// then the nearer FRONT rank over it, so a (bigger) back foe can't paint over a
	// front foe it's actually behind.
	for _, wantRow := range [...]core.Row{core.RowBack, core.RowFront} {
		for i := range members {
			if placements[i].Row != wantRow {
				continue
			}
			drawBattleFoe(camera, g, assets, &members[i], placements[i], i)
		}
	}
}

// drawBattleFoe renders one pack member at its resolved placement (extracted so the
// row-ordered loop above can call it per rank).
func drawBattleFoe(camera rl.Camera3D, g *core.GameState, assets Resources, enemy *core.Enemy, p core.EnemySlot, i int) {
	{
		visual, ok := enemyVisualFor(assets, enemy.Kind)
		if !ok {
			return
		}
		if !enemy.Alive && enemy.DeathFade <= 0 {
			return
		}
		// Per-rank size multiplier (Debug ▸ Combat Tuning) — applied before the
		// foot-anchor so a scaled foe still rests on the floor.
		rowScale := battleTune.FoeFrontScale
		if p.Row == core.RowBack {
			rowScale = battleTune.FoeBackScale
		}
		visual.size.X *= rowScale
		visual.size.Y *= rowScale
		position := enemyFormationPos(camera, g, p.Row, p.Slot, p.Count, enemy)
		// Foot-anchor: seat the billboard's BOTTOM on the battle floor regardless of
		// its height, so short foes (rats, bats) stand on the ground instead of
		// floating at a shared center. yOffset (added in resolveBillboardPlacement) is
		// then a deliberate hover, not a grounding fudge.
		position.Y = battleTune.FoeFloorY + visual.size.Y/2
		// Per-kind depth/marker/yOffset placement via the shared helper.
		place := resolveBillboardPlacement(camera, position, &visual)
		tint := rl.White
		if !enemy.Alive {
			alpha := uint8(220 * core.Clamp(float64(enemy.DeathFade/core.DeathFadeDuration), 0, 1))
			tint = colorWithAlpha(rl.White, alpha)
		}
		// Yellow chevron + tint only in the enemy-target picker (targetingEnemy
		// gates on Phase==BattlePlayer so it drops when the timing bar arms). The
		// AoE preview chevrons every REACHABLE living enemy (a melee AoE like Swipe
		// is front-gated, so back-row/Flying foes get no chevron).
		if enemy.Alive && ((targetingEnemy(g) && i == g.Battle.EnemyIndex) || aoeEnemyTargetPreview(g, i)) {
			tint = tintEnemyTargeted
			drawTargetChevron(camera, place.chevron, visual.effectiveMarkerScale())
		}
		// In BattleEnemyTiming the warm attacker tint reads "this one is swinging";
		// the red pyramid moved to the targeted party member.
		if enemy.Alive && isEnemyAttackerSlot(g, i) {
			tint = tintEnemyAttacker
		}
		if enemy.DamageFlash > 0 {
			tint = core.FlashTint(tint, enemy.DamageFlash)
		}
		// Fold in the per-kind base tint last (untinted = White, no-op).
		tint = tintMul(tint, visual.resolveTint())
		// Contact disc before the billboard, only for opt-in kinds. Keeps the
		// default shader so the billboard-fog pass doesn't tint it.
		if visual.shadowRadius > 0 {
			drawGroundShadow(place.shadowX, place.shadowZ, visual.shadowRadius)
		}
		// Back-rank atmospheric recede (Debug ▸ Combat Tuning "Foe back darken"):
		// darken + cool the tint so the back row reads as set behind the front. Folded
		// into the tint (multiplicative) so the sprite stays fully opaque.
		if p.Row == core.RowBack && battleTune.FoeBackDarken > 0 {
			tint = tintMul(tint, recedeMul(battleTune.FoeBackDarken))
		}
		// Fake directional light (vertical value ramp + warm/cool split + dark rim)
		// gives the flat sprite volume and detaches it from the busy backdrop. Distance
		// fog still rides the billboard-fog shader.
		drawShadedBillboard(camera, visual.texture, place.sprite, visual.size, tint, battleSpriteLight(g))
	}
}

// aoeEnemyTargetPreview reports whether an all-enemy AoE skill highlighted in the
// Skill submenu would land on the enemy at `slot` (the cue to chevron that foe).
// Delegates to core.PreviewAoEHitsEnemy so the menu-mode gating lives with the
// commit path (cf. enemyAttackTarget → core.PeekEnemyAttackerTarget).
func aoeEnemyTargetPreview(g *core.GameState, slot int) bool {
	return core.PreviewAoEHitsEnemy(g, slot)
}

// isEnemyAttackerSlot reports whether `slot` is the one lunging at the party
// (during BattleEnemyTiming).
func isEnemyAttackerSlot(g *core.GameState, slot int) bool {
	if g.Battle.Phase != core.BattleEnemyTiming {
		return false
	}
	return g.Battle.EnemyAttacker == slot
}

// enemyAttackTarget returns the party slot the lunging enemy will hit (ok=false
// when no marker should show), driving the red "incoming hit" marker during
// BattleEnemyTiming. Shares core.PeekEnemyAttackerTarget with the commit path so
// the marker honors the same melee front-row gate + Taunt override and can't drift
// from who's actually hit.
func enemyAttackTarget(g *core.GameState) (int, bool) {
	if g.Battle.Phase != core.BattleEnemyTiming {
		return -1, false
	}
	// AoE/summon casts have no single target — no incoming-hit marker.
	if g.Battle.EnemyPendingSkill != core.SkillNone {
		effect := core.SkillEffectFor(g.Battle.EnemyPendingSkill)
		if effect.AppliesAOEParty || effect.AppliesSummonSkeleton {
			return -1, false
		}
	}
	target := core.PeekEnemyAttackerTarget(g)
	if target < 0 {
		return -1, false
	}
	return target, true
}

// markerStyle bundles the parameters distinguishing one selector pyramid: tip
// anchor, silhouette (height + base radius), tint, and rotation phase (so two
// don't lock-step). One row per gameplay role.
type markerStyle struct {
	tipYOffset float32
	height     float32
	baseRadius float32
	color      rl.Color
	phase      float32
}

// Marker sizes target ~25% of a billboard's height — readable as a cursor under
// the magnifying battle FOV, not a billboard accessory.
var (
	// markerEnemyTarget: the selected enemy (yellow, paired with the roster row).
	markerEnemyTarget = markerStyle{
		// Sits low + fully opaque so the target reads, but modest. Per-kind
		// markerY/XOffset fine-tune where it lands.
		tipYOffset: 0.56,
		height:     0.20,
		baseRadius: 0.085,
		color:      selectorEnemyTargetColor,
		phase:      0.0,
	}
	// markerFriendlyTarget: the selected ally (green, smaller — closer to camera).
	markerFriendlyTarget = markerStyle{
		tipYOffset: smallMarkerTipYOffset,
		height:     smallMarkerHeight,
		baseRadius: smallMarkerBaseRadius,
		color:      selectorFriendlyTargetColor,
		phase:      0.3,
	}
	// markerEnemyAttackTarget tags the party member the lunging enemy will hit
	// while the defend bar is up. Shares the small-marker dims (paired look).
	markerEnemyAttackTarget = markerStyle{
		tipYOffset: smallMarkerTipYOffset,
		height:     smallMarkerHeight,
		baseRadius: smallMarkerBaseRadius,
		color:      selectorEnemyAttackColor,
		phase:      0.9,
	}
)

// Shared silhouette for the two party-side selector pyramids (closer to camera,
// so a touch smaller); pinned here so the pair can't drift.
const (
	smallMarkerTipYOffset = float32(0.36)
	smallMarkerHeight     = float32(0.13)
	smallMarkerBaseRadius = float32(0.055)
)

// drawMarkerOnTop draws a selector pyramid on a depth-disabled overlay layer so
// it always renders above world geometry and never clips.
func drawMarkerOnTop(unitPos rl.Vector3, style markerStyle) {
	drawDepthIndependent(func() { drawMarker(unitPos, style) })
}

// drawDepthIndependent runs draw with depth test AND writes disabled so it paints
// above all geometry and can't occlude later draws. rlgl batches, so the active
// batch is flushed before AND after the toggle; state is restored.
func drawDepthIndependent(draw func()) {
	rl.DrawRenderBatchActive() // flush prior depth-tested geometry
	rl.DisableDepthTest()
	rl.DisableDepthMask()
	draw()
	rl.DrawRenderBatchActive() // flush the overlay draw with depth off
	rl.EnableDepthMask()
	rl.EnableDepthTest()
}

// battleCursorScale shrinks every selector pyramid (enemy-target / friendly /
// incoming-hit) uniformly — half size reads as a cursor, not a billboard accessory.
const battleCursorScale = float32(0.5)

// drawMarker anchors the pyramid tip at unitPos + style.tipYOffset and forwards
// to drawSelectorPyramid (shrunk by battleCursorScale).
func drawMarker(unitPos rl.Vector3, style markerStyle) {
	tip := rl.NewVector3(unitPos.X, unitPos.Y+style.tipYOffset, unitPos.Z)
	drawSelectorPyramid(tip, style.height*battleCursorScale, style.baseRadius*battleCursorScale, style.color, style.phase)
}

func enemyVisualFor(assets Resources, kind core.EnemyKind) (enemyVisual, bool) {
	if v, ok := visualAt(assets.enemyVisuals, int(kind)); ok && v.texture.ID != 0 {
		return v, true
	}
	if v, ok := visualAt(assets.enemyVisuals, int(core.EnemyRat)); ok && v.texture.ID != 0 {
		return v, true
	}
	return enemyVisual{}, false
}

// visualAt indexes a dense kind/class→visual slice with a bounds guard, returning
// (zero, false) out of range.
func visualAt(s []enemyVisual, idx int) (enemyVisual, bool) {
	if idx < 0 || idx >= len(s) {
		return enemyVisual{}, false
	}
	return s[idx], true
}

// drawTargetChevron draws the yellow enemy-target pyramid, scaled by the kind's
// markerScale (1 = default). Only this enemy-side marker is kind-scaled.
func drawTargetChevron(camera rl.Camera3D, position rl.Vector3, scale float32) {
	drawScaledMarker(position, markerEnemyTarget, scale)
}

// drawScaledMarker draws a marker at position, scaling a copy of baseStyle by
// scale (0 or 1 = default size). Shared by drawTargetChevron + the friendly one.
func drawScaledMarker(position rl.Vector3, baseStyle markerStyle, scale float32) {
	style := baseStyle
	if scale > 0 && scale != 1 {
		style.height *= scale
		style.baseRadius *= scale
	}
	drawMarkerOnTop(position, style)
}

// drawSelectorPyramid renders the floating tip-down cursor over a unit, slowly
// spinning. Base sits `height` above `tip`, `baseRadius` out; `phase` offsets the
// rotation so two markers don't lock-step. Per-face shading reads as a 3D solid;
// all faces are wound CCW-from-outside so backface culling stays on (else the back
// faces z-fight the front).
func drawSelectorPyramid(tip rl.Vector3, height, baseRadius float32, col rl.Color, phase float32) {
	t := float64(worldFrameClock) // cached world clock (set by drawWorld), not a fresh GetTime() per marker
	yaw := t*0.9 + float64(phase) // gentler spin than before
	bob := float32(math.Sin(t*math.Pi*1.2)) * 0.04

	tipP := rl.NewVector3(tip.X, tip.Y+bob, tip.Z)
	baseY := tip.Y + height + bob

	var corners [4]rl.Vector3
	for i := 0; i < 4; i++ {
		a := yaw + float64(i)*tau/4
		corners[i] = rl.NewVector3(
			tip.X+float32(math.Cos(a))*baseRadius,
			baseY,
			tip.Z+float32(math.Sin(a))*baseRadius,
		)
	}

	// Per-face shading: sides walk light→mid→dim→mid, cap brightest. Rotates with
	// the pyramid so a fixed camera sees a shaded solid.
	sideShades := [4]float32{1.05, 0.85, 0.65, 0.85}
	for i := 0; i < 4; i++ {
		j := (i + 1) % 4
		// tip → c[i+1] → c[i] is CCW from outside (front face, +Y up).
		rl.DrawTriangle3D(tipP, corners[j], corners[i], shadeColor(col, sideShades[i]))
	}
	// Top cap (normal +Y); 0→1→2 / 0→2→3 are CCW from above.
	capCol := shadeColor(col, 1.18)
	rl.DrawTriangle3D(corners[0], corners[1], corners[2], capCol)
	rl.DrawTriangle3D(corners[0], corners[2], corners[3], capCol)
}

// shadeColor multiplies a color's RGB by factor (clamped), preserving alpha.
func shadeColor(c rl.Color, factor float32) rl.Color {
	return mapRGB(c, func(v uint8) uint8 {
		return core.ClampByte(int(float32(v) * factor))
	})
}

// ShadeColor is the exported form of shadeColor for the editor's 3D view.
func ShadeColor(c rl.Color, factor float32) rl.Color { return shadeColor(c, factor) }

// partyDrawOrderBuf backs partyDrawOrder so the once-per-frame ordering doesn't
// allocate. Reused across frames (re-sliced [:0]); the result is consumed within
// the single DrawPartySprites loop before the next frame overwrites it.
var partyDrawOrderBuf = make([]int, 0, core.PartyMemberCount)

// partyDrawOrder returns party indices ordered far-rank-first — the FRONT rank
// (farther from the camera) before the nearer BACK rank — for the depth-test-off
// battle pass, so the nearer rank paints over the farther at any overlap.
func partyDrawOrder(party []core.PartyMember) []int {
	order := partyDrawOrderBuf[:0]
	for _, want := range [...]core.Row{core.RowFront, core.RowBack} {
		for i := range party {
			if party[i].Row == want {
				order = append(order, i)
			}
		}
	}
	partyDrawOrderBuf = order
	return order
}

func DrawPartySprites(camera rl.Camera3D, g *core.GameState, assets Resources) {
	if g.Battle.Phase == core.BattleNone {
		return
	}
	defer beginBillboardFogPass(camera, g, assets)()
	victoryDance := victoryDanceElapsed(g)
	incomingSlot, hasIncoming := enemyAttackTarget(g)
	// Painter's order for the depth-test-off battle pass: the FRONT rank sits farther
	// from the camera than the back rank, so draw front first and let the nearer back
	// rank paint over it at any overlap.
	for _, i := range partyDrawOrder(g.Party) {
		// Ingested members are tucked inside a mantrap — don't draw them.
		if g.Party[i].Ingested {
			continue
		}
		visual, ok := partyVisualFor(assets, g.Party[i].Class)
		if !ok {
			continue
		}
		memberDance := float32(0)
		if g.Party[i].HP > 0 {
			memberDance = victoryDance
		}
		position := partySpritePosition(camera, g.Party, i, g.Party[i].AttackBump, memberDance, g.Party[i].HitKnockback)
		// Per-class placement via the shared helper (no battle/editor drift).
		place := resolveBillboardPlacement(camera, position, &visual)
		size := visual.size
		// Per-rank party size multiplier (Debug ▸ Combat Tuning), applied to the base
		// before the active-bump / victory-dance scaling below.
		partyRowScale := battleTune.PartyFrontScale
		if g.Party[i].Row == core.RowBack {
			partyRowScale = battleTune.PartyBackScale
		}
		size.X *= partyRowScale
		size.Y *= partyRowScale
		tint := rl.White
		if g.Party[i].HP <= 0 {
			tint = tintPartyDown
		} else if inPlayerTurn(g) && i == g.Battle.CurrentParty {
			tint = tintPartyActive
			size = rl.NewVector2(size.X*partyActiveScale.X, size.Y*partyActiveScale.Y)
		} else if memberDance > 0 {
			_, _, _, scale := victoryDanceMotion(g.Party[i].Class, memberDance)
			size.X *= scale
			size.Y *= scale
		}
		if g.Party[i].DamageFlash > 0 {
			tint = core.FlashTint(tint, g.Party[i].DamageFlash)
		}
		// Fold in the per-class base tint last (untinted = White, no-op).
		tint = tintMul(tint, visual.resolveTint())
		// Optional authored contact disc before the billboard (default radius 0).
		if visual.shadowRadius > 0 {
			drawGroundShadow(place.shadowX, place.shadowZ, visual.shadowRadius)
		}
		// Distance fog comes from the billboard-fog shader; the "your turn" read
		// lives in the party card now (plus the warm tint + bump above).
		if g.Party[i].HP <= 0 {
			// Downed: topple the sprite flat (rotated 90°) + dimmed. Lower the center
			// by half the (height−width) so the now-horizontal sprite still rests on
			// the floor instead of hovering at standing-center height.
			downed := place.sprite
			downed.Y -= (size.Y - size.X) / 2
			drawTextureBillboardRotated(camera, visual.texture, downed, size, downedToppleDegrees, tint)
		} else {
			// Same fake-light treatment as the foes so both sides read with volume.
			drawShadedBillboard(camera, visual.texture, place.sprite, size, tint, battleSpriteLight(g))
		}
		// Friendly marker only in the menu phase, not the timing bar that follows
		// (inPlayerTurn includes BattleAttackTiming and would linger it).
		if g.Battle.Phase == core.BattlePlayer && targetingAlly(g) && i == g.Battle.PartyTarget && g.Party[i].HP > 0 {
			drawFriendlyTargetMarker(camera, place.chevron, visual.effectiveMarkerScale())
		}
		// Red "incoming hit" marker; enemyAttackTarget gates the phase.
		if g.Party[i].HP > 0 && hasIncoming && i == incomingSlot {
			drawEnemyAttackTargetMarker(camera, place.chevron)
		}
	}
}

// partyVisualFor returns the billboard visual for a class (false when no usable
// texture — defensive, the table is fully populated at load).
func partyVisualFor(assets Resources, class core.PartyClass) (enemyVisual, bool) {
	if v, ok := visualAt(assets.partyVisuals, int(class)); ok && v.texture.ID != 0 {
		return v, true
	}
	return enemyVisual{}, false
}

// drawFriendlyTargetMarker draws the ally-target pyramid scaled by the class's
// markerScale — the friendly twin of drawTargetChevron.
func drawFriendlyTargetMarker(camera rl.Camera3D, position rl.Vector3, scale float32) {
	drawScaledMarker(position, markerFriendlyTarget, scale)
}

func drawEnemyAttackTargetMarker(camera rl.Camera3D, position rl.Vector3) {
	drawMarkerOnTop(position, markerEnemyAttackTarget)
}

// formationSlotXZ projects a camera-relative formation slot to world X/Z:
// camera.Position + forward*distance + right*lateral (horizontal plane only). Shared
// base for the party trapezoid and the foe rank center; each caller layers its own
// clamp/zigzag/depth on top. Term order is verbatim so output stays bit-identical.
func formationSlotXZ(camera rl.Camera3D, forward, right rl.Vector3, distance, lateral float32) (x, z float32) {
	return camera.Position.X + forward.X*distance + right.X*lateral,
		camera.Position.Z + forward.Z*distance + right.Z*lateral
}

func partySpritePosition(camera rl.Camera3D, party []core.PartyMember, index int, bump, victoryDance float32, knockback float32) rl.Vector3 {
	forward := horizontalForward(camera)
	right := horizontalRight(forward)
	class := core.PartyClass(0)
	if core.PartyIndexInRange(party, index) {
		class = party[index].Class
	}
	// Layout uses the LIVE combat slot (Row/Col), so ambush rotation and death-driven
	// swaps physically MOVE the sprite — the front line you see is the one that fights.
	// The live slot is seated from the home formation at battle start and reverts after.
	visRow, visCol := core.RowFront, core.ColLeft
	if core.PartyIndexInRange(party, index) {
		visRow, visCol = party[index].Row, party[index].Col
	}
	// slotXZ resolves a (row,col) to its world X/Z on the 2×2 trapezoid widening
	// toward the viewer (Debug ▸ Combat Tuning: Party rows): front tight/further,
	// back wide/nearer.
	baseY := battleTune.PartyBaseY
	slotXZ := func(row core.Row, col core.Col) (x, z float32) {
		rowForward, rowSpacing := battleTune.PartyFrontFwd, battleTune.PartyFrontGapX
		if row == core.RowBack {
			rowForward, rowSpacing = battleTune.PartyBackFwd, battleTune.PartyBackGapX
		}
		colSign := float32(-0.5)
		if col == core.ColRight {
			colSign = 0.5
		}
		offset := colSign * rowSpacing
		return formationSlotXZ(camera, forward, right, rowForward, offset)
	}
	slotX, slotZ := slotXZ(visRow, visCol)
	// Formation-Swap glide: ease from the pre-swap slot to the live slot over the
	// SwapSlide countdown so the trade reads as movement, not a teleport.
	if core.PartyIndexInRange(party, index) && party[index].SwapSlide > 0 {
		fromX, fromZ := slotXZ(party[index].SwapFromRow, party[index].SwapFromCol)
		t := core.Smoothstep(core.Clamp(1-party[index].SwapSlide/core.SwapSlideDuration, 0, 1))
		slotX = core.Lerp(fromX, slotX, t)
		slotZ = core.Lerp(fromZ, slotZ, t)
	}
	base := rl.NewVector3(slotX, baseY, slotZ)
	offset := float32(0) // column offset already folded into base via slotXZ
	depth := float32(0)
	danceSide, danceDepth, danceHeight, _ := victoryDanceMotion(class, victoryDance)
	bumpDepth := core.BumpOffset(bump, 0.22)
	// AttackBump adds forward (lunge); HitKnockback subtracts (recoil toward the
	// camera). The two timers don't overlap in practice.
	knockDepth := core.KnockbackOffset(knockback, core.HitKnockbackDist)
	return rl.NewVector3(
		base.X+right.X*(offset+danceSide)+forward.X*(depth+bumpDepth-knockDepth+danceDepth),
		base.Y+danceHeight,
		base.Z+right.Z*(offset+danceSide)+forward.Z*(depth+bumpDepth-knockDepth+danceDepth),
	)
}

func victoryDanceElapsed(g *core.GameState) float32 {
	if g.Battle.Phase != core.BattleWon {
		return 0
	}
	remaining := core.Clamp(g.Battle.Timer, 0, core.VictoryDanceDuration)
	return core.VictoryDanceDuration - remaining
}

func victoryDanceMotion(class core.PartyClass, elapsed float32) (float32, float32, float32, float32) {
	if elapsed <= 0 {
		return 0, 0, 0, 1
	}
	return partyClassPresentationFor(class).dance(elapsed)
}

// enemyDrawPosition returns where to render a pack member, resolving its slot
// from g. The single-shot path for popup/VFX anchors; the battle loop precomputes
// slots and calls enemyFormationPos directly. g by pointer to avoid a copy.
func enemyDrawPosition(camera rl.Camera3D, g *core.GameState, slot int, enemy *core.Enemy) rl.Vector3 {
	if g.Battle.Phase == core.BattleNone || g.Battle.ActivePack < 0 {
		// Fallback during a phase transition.
		pack := rl.NewVector3(0, enemyBillboardY, 0)
		if g.Battle.ActivePack >= 0 && g.Battle.ActivePack < len(g.Packs) {
			p := g.Packs[g.Battle.ActivePack]
			pack = tileWorldPos(p.TileX, p.TileZ, enemyBillboardY)
		}
		return pack
	}
	row, rowSlot, rowCount := enemyRowSlot(core.BattleMembers(g), slot)
	return enemyFormationPos(camera, g, row, rowSlot, rowCount, enemy)
}

// enemyRowSlot resolves enemy `index`'s placement among the VISIBLE (alive or
// death-fading) members: its row, left-to-right slot, and the row's visible count.
// Foes lay out by their live row (the shunt keeps the front packed).
func enemyRowSlot(members []core.Enemy, index int) (row core.Row, slot, count int) {
	if index < 0 || index >= len(members) {
		return core.RowFront, 0, 1
	}
	row = members[index].Row
	for j := range members {
		if !(members[j].Alive || members[j].DeathFade > 0) || members[j].Row != row {
			continue
		}
		if j == index {
			slot = count
		}
		count++
	}
	if count == 0 {
		count = 1 // the queried member isn't visible; place it solo rather than /0
	}
	return row, slot, count
}

// enemyPlacementsBuf backs enemyRowPlacements (reused, single-threaded path).
var enemyPlacementsBuf []core.EnemySlot

// enemyRowPlacements resolves EVERY member's placement in one O(n) pass — the batch
// form of enemyRowSlot. Delegates to core.ResolveEnemySlots so the drawn slot is the
// SAME one the battle tick armed the formation slide against. The returned slice
// aliases a reused buffer (valid until the next call).
func enemyRowPlacements(members []core.Enemy) []core.EnemySlot {
	enemyPlacementsBuf = core.ResolveEnemySlots(members, enemyPlacementsBuf)
	return enemyPlacementsBuf
}

// enemyFormationBase resolves a foe's resting world position for a (row, slot, count)
// placement — the formation geometry WITHOUT the per-action bump/knock offsets, so it
// can be the lerp endpoint for the slot-slide glide. count<=0 returns the transition
// fallback (the pack's tile center).
func enemyFormationBase(camera rl.Camera3D, g *core.GameState, row core.Row, slot, count int) rl.Vector3 {
	if count <= 0 {
		// Defensive: re-check the ActivePack bound so a malformed state can't panic.
		// ActivePack < 0 is the "no pack" sentinel — guard it too, else g.Packs[-1].
		if g.Battle.ActivePack < 0 || g.Battle.ActivePack >= len(g.Packs) {
			return rl.NewVector3(0, enemyBillboardY, 0)
		}
		p := g.Packs[g.Battle.ActivePack]
		return tileWorldPos(p.TileX, p.TileZ, enemyBillboardY)
	}
	if slot < 0 {
		slot = 0
	}
	forward := horizontalForward(camera)
	right := horizontalRight(forward)
	// Distance forward of the camera (Debug ▸ Combat Tuning: Foe distance). Lateral 0:
	// per-slot X offset + zigzag/depth layer on below; center is the rank's midline.
	centerX, centerZ := formationSlotXZ(camera, forward, right, battleTune.FoeDistance, 0)
	// Per-row X spacing (tunable): front packs tight, back fans wide so back foes
	// spill past the front gaps and read between them. maxWidth caps the total spread
	// so a full row can't run off the stage.
	baseSpacing, formationMaxWidth := battleTune.FoeFrontGapX, battleTune.FoeFrontMaxW
	if row == core.RowBack {
		baseSpacing, formationMaxWidth = battleTune.FoeBackGapX, battleTune.FoeBackMaxW
	}
	spacing := baseSpacing
	if count > 1 {
		if fit := formationMaxWidth / float32(count-1); fit < spacing {
			spacing = fit
		}
	}
	offset := (float32(slot) - float32(count-1)/2) * spacing
	// Two ranks: front nearer the party; back deeper, aligned in the SAME columns as
	// the front (no half-slot shift) so a front foe visibly shields the back foe
	// behind it — the column-cover rule made literal, and the back row reads centered
	// rather than skewed right. The back rank reads as behind from the extra depth +
	// the downward battle pitch — NOT from lifting it (drawBattlePack foot-anchors
	// every foe to the floor, so any lift would just levitate them again).
	// Ranks sit close in depth (gap ~0.4) so the back row reads just over/between the
	// front rather than shrinking off into the distance.
	rowDepth := battleTune.FoeFrontDepth
	if row == core.RowBack {
		rowDepth = battleTune.FoeBackDepth
	}
	// Depth zigzag: alternate slots step a little fore/aft of their rank so a wide
	// row reads as a milling crowd, not a flat cardboard wall, and neighbors overlap
	// less.
	if slot%2 == 1 {
		rowDepth += battleTune.FoeZigzag
	} else {
		rowDepth -= battleTune.FoeZigzag
	}
	return rl.NewVector3(
		centerX+right.X*offset+forward.X*rowDepth,
		battleFormationCenterY,
		centerZ+right.Z*offset+forward.Z*rowDepth,
	)
}

func enemyFormationPos(camera rl.Camera3D, g *core.GameState, row core.Row, slot, count int, enemy *core.Enemy) rl.Vector3 {
	base := enemyFormationBase(camera, g, row, slot, count)
	// Formation glide: ease from the pre-reshuffle slot to the live slot over the
	// SlotSlide countdown so a death/shunt reads as the survivors stepping across to
	// close the gap, not teleporting. Mirrors the party SwapSlide. Only the formation
	// base slides; the per-action bump/knock below stay instant.
	if enemy.SlotSlide > 0 && count > 0 && enemy.SlideFromCount > 0 {
		from := enemyFormationBase(camera, g, enemy.SlideFromRow, enemy.SlideFromSlot, enemy.SlideFromCount)
		t := core.Smoothstep(core.Clamp(1-enemy.SlotSlide/core.SlotSlideDuration, 0, 1))
		base.X = core.Lerp(from.X, base.X, t)
		base.Z = core.Lerp(from.Z, base.Z, t)
	}
	forward := horizontalForward(camera)
	bump := core.BumpOffset(enemy.AttackBump, 0.2)
	// Knockback pushes away from the camera; AttackBump lunges toward the party
	// (opposite signs).
	knock := core.KnockbackOffset(enemy.HitKnockback, core.HitKnockbackDist)
	axial := knock - bump
	return rl.NewVector3(
		base.X+forward.X*axial,
		base.Y,
		base.Z+forward.Z*axial,
	)
}

// horizForwardCache memoizes horizontalForward — the camera is identical across
// every billboard/marker draw in a frame, so the ~6 Hypot/normalize calls collapse
// to one (rest hit a 2-float compare). Single-threaded; exact compare is fine.
var horizForwardCache struct {
	dx, dz float32
	result rl.Vector3
	primed bool
}

// horizontalRight is the camera right vector on the XZ plane, perpendicular to
// horizontalForward — the single home for the (-fwd.Z, 0, fwd.X) expression.
func horizontalRight(forward rl.Vector3) rl.Vector3 {
	return rl.NewVector3(-forward.Z, 0, forward.X)
}

func horizontalForward(camera rl.Camera3D) rl.Vector3 {
	x := camera.Target.X - camera.Position.X
	z := camera.Target.Z - camera.Position.Z
	if horizForwardCache.primed && horizForwardCache.dx == x && horizForwardCache.dz == z {
		return horizForwardCache.result
	}
	length := float32(math.Hypot(float64(x), float64(z)))
	result := rl.NewVector3(1, 0, 0)
	if length != 0 {
		result = rl.NewVector3(x/length, 0, z/length)
	}
	horizForwardCache.dx, horizForwardCache.dz = x, z
	horizForwardCache.result = result
	horizForwardCache.primed = true
	return result
}

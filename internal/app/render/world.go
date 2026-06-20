package render

import (
	"math"
	"sort"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

type enemyVisual struct {
	texture rl.Texture2D
	// pristineTexture is the UNADJUSTED base sprite (procedural art or authored
	// PNG, before any non-destructive Pixelate/Brightness/Contrast override is
	// applied). `texture` is what's drawn — derived from this plus the override's
	// image adjustments at load. The editor re-derives its live preview from the
	// pristine so dragging the FX sliders never compounds onto an already-adjusted
	// image. Equals `texture` when the override has no image adjustments.
	pristineTexture rl.Texture2D
	size            rl.Vector2
	// shadowRadius is the half-extent (world units) of the soft contact
	// disc painted on the floor beneath this billboard, matching the
	// prop grounding signature (see propShadowRadius). Zero = no shadow,
	// which is every procedural sprite's default — only kinds that opt in
	// (currently the file-textured Feral Rat) get a disc so the existing
	// roster's look is unchanged.
	shadowRadius float32
	// yOffset shifts THIS billboard up (+) or down (−) from the shared
	// enemyBillboardY center-anchor, in world units. The contact shadow
	// stays anchored to the unaltered tile position; the selector pyramid
	// is nudged separately via markerYOffset (below). A negative offset
	// lowers a sprite that floats without dragging the shadow with it.
	// Zero for every procedural sprite (their art is already
	// bottom-weighted); only center-weighted authored PNGs need it (the
	// Feral Rat). NOTE: yOffset is calibrated against the BATTLE formation
	// center (battleFormationCenterY); drawFieldPacks adds the framing-lift
	// delta back so a grounded sprite plants on the floor in both views.
	yOffset float32
	// markerYOffset / markerXOffset nudge THIS kind's selector pyramid from
	// the default anchor (formation center + markerStyle.tipYOffset), in
	// world units. Y is up(+)/down(−); X is along the camera-right axis
	// (screen right(+)/left(−)) so the nudge reads the same regardless of
	// which way the battle camera faces. Per-unit-type so a sprite whose
	// visible head sits off the billboard center (a yOffset-lowered or
	// asymmetric PNG) can place its cursor cleanly. Zero = the shared
	// default, unchanged for every procedural kind.
	markerYOffset float32
	markerXOffset float32
	// depthOffset pushes THIS kind's BATTLE billboard further from the camera
	// (+ = back into the arena) along the camera-forward axis. Lets a sprite
	// whose authored art sits forward in its box (the square Feral Rat PNG)
	// sit back in line with the procedural roster instead of looming. Zero for
	// every procedural kind. Battle-only; the field render is world-anchored.
	depthOffset float32
	// shadowOffsetX / shadowOffsetZ nudge the contact disc from the sprite's
	// FINAL footprint (after depthOffset), along the camera-right(+ = screen
	// right) and camera-forward(+ = into the arena) axes respectively. The disc
	// always rides the sprite's drawn XZ — these are an extra welded nudge so
	// an author can park the shadow exactly under the visible feet of an
	// off-center PNG without it drifting when the sprite is moved. Zero = dead
	// under the billboard. World units; camera-relative so the nudge reads the
	// same regardless of which way the battle camera faces.
	shadowOffsetX float32
	shadowOffsetZ float32
	// markerScale multiplies the selector-pyramid silhouette (height + base
	// radius) for THIS kind's target chevron, so a big foe can wear a bigger
	// cursor and a small one a daintier cursor. Zero = unset → 1.0 (see
	// effectiveMarkerScale); the position is still tuned by markerY/XOffset.
	markerScale float32
	// glyphXOffset / glyphYOffset nudge THIS kind's hit-clarity glyph from the
	// struck sprite's center along camera-right(+ = screen right) and world-up(+)
	// before it projects to screen, so the glyph sits over the visible body of an
	// off-center PNG. glyphScale multiplies its on-screen radius. Zero offsets =
	// centered (plus the shared hitGlyphRise lift); zero scale = unset → 1.0.
	glyphXOffset float32
	glyphYOffset float32
	glyphScale   float32
	// particleXOffset / particleYOffset / particleZOffset nudge THIS kind's
	// impact particle burst origin along camera-right(+), world-up(+), and
	// camera-forward(+ = into the arena); particleScale uniformly scales the
	// burst's spread + dot size around that origin. Lets a tiny rat get a tight
	// little puff and a Stone Golem a big slam. Zero offsets = at the sprite
	// center; zero scale = unset → 1.0 (see effectiveParticleScale).
	particleXOffset float32
	particleYOffset float32
	particleZOffset float32
	particleScale   float32
	// popupXOffset / popupYOffset nudge THIS kind's floating damage-NUMBER spawn
	// from the sprite center along camera-right(+) and world-up(+), ADDITIVE on
	// the baked default rise (zero = historical spot). Separate from the glyph
	// anchor above — this moves the number, not the clarity symbol.
	popupXOffset float32
	popupYOffset float32
	// tint is a per-kind base color multiplied (raylib ColorTint semantics:
	// a*b/255 per channel) into the runtime billboard tint — darken with a
	// gray, recolor with a hue, etc. It folds in AFTER the combat tint
	// branches (death fade / targeting / attacker / damage flash), so a
	// tinted sprite stays proportionally tinted in every state. The zero
	// value (A==0) means "untinted"; use A==255 when setting one. See
	// resolveTint / the draw sites.
	tint rl.Color
	// Non-destructive image adjustments, mirroring the override fields so they
	// round-trip through enemyVisualOverride/applyEnemyVisualOverride (the editor
	// seeds its sliders from these). They drive how `texture` is derived from
	// `pristineTexture` at build time — they do NOT alter the draw directly.
	pixelate   float32
	brightness float32
	contrast   float32
}

// resolveTint returns the per-kind base tint, treating the zero-value Color
// (A==0, what an unset `tint` field is) as White so untinted kinds draw at
// full texture color. Any real tint sets A==255, so the alpha test cleanly
// separates "unset" from a deliberate color — and folding it in never
// reduces the runtime tint's own alpha (death fade) since White's A is 255.
func (v enemyVisual) resolveTint() rl.Color {
	if v.tint.A == 0 {
		return rl.White
	}
	return v.tint
}

// effectiveMarkerScale / effectiveGlyphScale / effectiveParticleScale resolve a
// per-kind size multiplier, treating the zero value (an unset field, or a
// missing field in a visuals.json written before these existed) as 1.0 — full
// size. Mirrors resolveTint's "zero = sensible default" handling so the code
// defaults never need to spell out a 1.0 for every kind, and a non-negative
// authored value (the editor's slider floor is 0.1) is honored verbatim. A
// negative value can't be authored but is clamped up to 1.0 defensively.
func (v enemyVisual) effectiveMarkerScale() float32   { return scaleOrDefault(v.markerScale) }
func (v enemyVisual) effectiveGlyphScale() float32    { return scaleOrDefault(v.glyphScale) }
func (v enemyVisual) effectiveParticleScale() float32 { return scaleOrDefault(v.particleScale) }

func scaleOrDefault(s float32) float32 {
	if s <= 0 {
		return 1
	}
	return s
}

// shadowFootprint returns the world XZ where THIS visual's contact disc should
// land: the sprite's drawn footprint plus the visual's camera-relative
// shadowOffset nudge. Welding the disc to the same XZ the billboard draws at
// (depthOffset already folded into `position`) is what keeps "the shadow stays
// under the feet" true no matter how the sprite is pushed around — the offsets
// are an explicit extra placement, not a separate anchor that can drift.
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

// cameraRelativeOffset nudges world point p by dx along camera-right(+ = screen
// right), dy along world-up(+), and dz along camera-forward(+ = into the scene).
// Camera-relative XZ (same basis as shadowFootprint / markerXOffset) so the
// nudge reads identically regardless of which way the battle camera faces. A
// zero nudge returns p untouched (and skips the trig). Used to place the
// per-kind hit-glyph and particle-burst anchors over a struck enemy.
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

// enemyVisualOverride snapshots the tunable fields of an enemyVisual into the
// raylib-free core.EnemyVisualOverride the editor edits and the save file
// stores. The texture is intentionally excluded — it always comes from the
// sprite asset, never the override. The inverse (applying an override onto a
// visual) is applyEnemyVisualOverride.
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
		// Snapshot the EFFECTIVE scale (resolves an unset 0 → 1.0) so the editor
		// seeds its sliders at full size, not a confusing 0. Offsets snapshot raw
		// (0 = no nudge, which is also their default).
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
	}
}

// applyEnemyVisualOverride returns v with every tunable field replaced by the
// override's value, preserving the texture (and any non-overridable internals).
// Used both by loadEnemyVisuals (overlay the save file onto code defaults) and
// by the editor's live preview (draw the in-progress edit without a reload).
func applyEnemyVisualOverride(v enemyVisual, ov core.EnemyVisualOverride) enemyVisual {
	v.size = rl.NewVector2(ov.SizeX, ov.SizeY)
	v.yOffset = ov.YOffset
	v.depthOffset = ov.DepthOffset
	v.shadowRadius = ov.ShadowRadius
	v.shadowOffsetX = ov.ShadowOffsetX
	v.shadowOffsetZ = ov.ShadowOffsetZ
	v.markerYOffset = ov.MarkerYOffset
	v.markerXOffset = ov.MarkerXOffset
	// Scales direct-assign; the effective*Scale accessors fold an unset 0 (or a
	// pre-existing visuals.json that lacks these fields) back to 1.0 at the draw
	// site, so a 0 here never means "invisible."
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
	return v
}

// tintMul multiplies two colors channel-wise (raylib's ColorTint: a*b/255
// per channel, alpha included). Used to fold a sprite's per-kind base tint
// into the runtime billboard tint without a shader.
func tintMul(a, b rl.Color) rl.Color {
	return rl.NewColor(
		uint8(int(a.R)*int(b.R)/255),
		uint8(int(a.G)*int(b.G)/255),
		uint8(int(a.B)*int(b.B)/255),
		uint8(int(a.A)*int(b.A)/255),
	)
}

// exploreFOV is the wide field-of-view used while walking the world.
// 112° trades some perspective distortion at the edges for situational
// awareness — corridor turns and adjacent props are visible without
// having to free-look around.
const exploreFOV = float32(112)

// battleFOV is the narrower FOV used the moment battle becomes active.
// "Zooms in" by reducing the angle subtended, which (a) makes enemy
// billboards take up more screen pixels at the same world distance,
// and (b) packs the formation into a smaller projected width so the
// arena reads as a focused stage instead of a wide open field. The
// narrower the angle, the bigger the enemies appear; tuned so a
// six-enemy pack still fits comfortably without horizontal scroll.
const battleFOV = float32(72)

// fovTweenRate sets how fast the camera eases between exploreFOV and
// battleFOV, in degrees per second. 80°/s lands the full 40° swing in
// about half a second — slightly faster than BattleSplashDuration so
// the zoom feels like part of the encounter's "drop into combat"
// punctuation without lagging the splash banner.
const fovTweenRate = float32(80)

// currentFOV is the visible FOV right now, eased toward the target
// each frame. Package-local so the tween survives across draw calls
// without leaking visual state onto GameState — a fresh game starts
// at exploreFOV thanks to the var init, and the lerp converges
// quickly enough that a reset mid-battle (which is rare) settles in
// the next two-three frames.
var currentFOV = exploreFOV

// targetFOV returns the FOV the camera should be tweening toward this
// frame. Split out so the tween logic in Camera doesn't have to
// branch on `g.Battle.Active()` inline.
func targetFOV(g *core.GameState) float32 {
	if g.Battle.Active() {
		return battleFOV
	}
	return exploreFOV
}

// Camera builds the per-frame perspective camera. FOV smoothly tweens
// toward exploreFOV or battleFOV (see targetFOV) so dropping into and
// out of combat reads as a deliberate zoom rather than a snap. The
// tween uses rl.GetFrameTime so it's framerate-independent.
// battlePitchOffset tilts the camera downward when battle is active so
// the arena floor takes up more of the lower half of the screen and
// the combat ribbon doesn't feel like it's pasted onto a sky shot.
// Applied additively to the player's LookPitch — small enough (-0.18
// rad ≈ -10°) that the enemy sprites stay visible while the floor
// pulls into view.
const battlePitchOffset = float32(-0.18)

func Camera(g *core.GameState) rl.Camera3D {
	p := g.Player
	yaw := p.Yaw + p.LookYaw
	pitch := p.LookPitch
	if g.Battle.Active() {
		pitch += battlePitchOffset
	}
	cp := float32(math.Cos(float64(pitch)))
	direction := rl.NewVector3(
		cp*float32(math.Cos(float64(yaw))),
		float32(math.Sin(float64(pitch))),
		cp*float32(math.Sin(float64(yaw))),
	)
	// Elevation: the eye rides the ground height of the tile underfoot. At
	// rest that's StandGroundY(tile); mid-step it's the eased Player.GroundY
	// the movement tick interpolates across a ramp (so the camera climbs
	// smoothly instead of snapping a level at the tile boundary).
	groundY := g.Area.StandGroundY(p.TileX, p.TileZ)
	if len(g.Area.Solids) > 0 {
		// Voxel map: ride the resolved standing level (under a deck vs on it),
		// not the column top StandGroundY reports.
		groundY = g.Area.StandGroundYAt(p.TileX, p.Level, p.TileZ)
	}
	if p.Anim.Kind == core.AnimStep {
		groundY = p.GroundY
	}
	position := rl.NewVector3(p.X, core.EyeHeight+groundY, p.Z)
	// Combat screen shake: a small positional jitter on a well-timed hit,
	// scaled by the remaining ShakeTimer so it eases out. Oscillation is
	// wall-clock-driven (two incommensurate frequencies so it wobbles rather
	// than sliding) so the shake is visible even while hit-stop freezes the
	// sim. Battle-only — never perturbs the exploration camera.
	if g.Battle.Active() && g.Battle.ShakeTimer > 0 && g.Battle.ShakeDur > 0 {
		amp := g.Battle.ShakePeak * core.Clamp(g.Battle.ShakeTimer/g.Battle.ShakeDur, 0, 1)
		t := rl.GetTime()
		position.X += float32(math.Sin(t*47.0)) * amp
		position.Y += float32(math.Sin(t*61.0)) * amp
	}
	// Frame-time-driven approach: each frame, push currentFOV toward
	// the target by at most fovTweenRate*dt degrees. The Approach
	// helper from core stops at the target without overshooting, so a
	// long frame doesn't whip past the destination.
	currentFOV = core.Approach(currentFOV, targetFOV(g), fovTweenRate*rl.GetFrameTime())
	return rl.NewCamera3D(
		position,
		rl.NewVector3(position.X+direction.X, position.Y+direction.Y, position.Z+direction.Z),
		rl.NewVector3(0, 1, 0),
		currentFOV,
		rl.CameraPerspective,
	)
}

// SkyClearColor is the backdrop ClearBackground color the adventure scene wipes
// to before DrawSkyBackground paints over it. The color is overdrawn
// immediately — the clear is load-bearing for the DEPTH wipe that rides with it
// (see run.go) — so this exists mainly to single-source the literal across the
// scene's two clear arms rather than for its visible hue.
var SkyClearColor = rl.NewColor(87, 172, 244, 255)

func DrawSkyBackground(assets Resources, g *core.GameState) {
	texW := float32(assets.skyTexture.Width)
	texH := float32(assets.skyTexture.Height)
	screenW, screenH := screenSizeF()
	// Crop the source rect to the screen's aspect ratio so the sky doesn't
	// stretch when the window isn't a 2:1 letterbox. The sky texture is 2:1
	// (1024×512); on a typical 16:10 screen we sample a centered slice that
	// matches the screen's aspect, so clouds stay round instead of squashed.
	srcX, srcW := float32(0), texW
	srcY, srcH := float32(0), texH
	screenAspect := screenW / screenH
	texAspect := texW / texH
	if texAspect > screenAspect {
		// Texture wider than screen: crop the sides.
		srcW = texH * screenAspect
		srcX = (texW - srcW) / 2
	} else if texAspect < screenAspect {
		// Texture taller (in aspect terms) than screen: crop top/bottom.
		// The horizon usually sits in the lower-middle of the sky texture,
		// so we crop more off the top to keep the cloud band in view.
		srcH = texW / screenAspect
		srcY = (texH - srcH) * 0.35
	}
	source := rl.NewRectangle(srcX, srcY, srcW, srcH)
	dest := rl.NewRectangle(0, 0, screenW, screenH)
	// Sky tint follows the time-of-day profile for every material.
	// Even "indoor" maps (dungeon-walled forests, roofless ruins on the
	// stone palette) want the sunset / sunrise / starfield arc — the
	// CeilingAt slabs render an opaque cap above tiles that should
	// block the sky entirely, so painting a varying backdrop for an
	// actually-roofed dungeon room is invisible to the player anyway.
	// Removing the old `MaterialIsIndoor` gate fixes a foot-gun where
	// a forest map authored on the dungeon palette had a static slate
	// sky and no stars at night.
	profile := timeProfileAt(g.StepCount)
	tint := skyColor(profile.SkyTint)
	rl.DrawTexturePro(assets.skyTexture, source, dest, rl.NewVector2(0, 0), 0, tint)
	// Star layer rides the same source/dest as the sky so the stars
	// crop with the same aspect logic. Alpha = profile.StarAlpha *
	// the texture's per-pixel alpha (mostly transparent, so the
	// resulting blend is sparse pinpoints). Same "ceiling caps the
	// view anyway" rationale applies here — no indoor gate.
	if profile.StarAlpha > 0 {
		alpha := uint8(profile.StarAlpha * 255)
		rl.DrawTexturePro(assets.starTexture, source, dest, rl.NewVector2(0, 0), 0, rl.NewColor(255, 255, 255, alpha))
	}
}

// behindCullSlack is how far behind the camera a tile's center can project
// before we skip it. Set generously enough that the tile we're standing on
// (dot ≈ 0) and tiles half-behind us (dot ≈ -tile/2) stay drawn through any
// reasonable rotation. The fog handles distance falloff on the far side, so
// we deliberately don't add a hard distance cap — pop-in there would be very
// visible against the fog's gentle tail (which clamps at 85% saturation).
const behindCullSlack = float32(-2.5)

// behindCull reports whether world point p sits far enough behind the camera
// to skip drawing it. `camPos` is the camera position and `forward` the
// caller's already-computed horizontal forward — both hoisted out of per-item
// loops so this stays a cheap dot per call. The single home for the cull rule:
// the DrawWorld tile loop, the chest draw, and the door draw all call it so
// they cull consistently with the floor under them.
func behindCull(camPos, forward, p rl.Vector3) bool {
	return behindCullXZ(camPos, forward, p.X, p.Z)
}

// behindCullXZ is behindCull taking the point's X/Z as scalars — the tile loop
// calls it per tile, so this avoids building a throwaway rl.Vector3 (with a
// dummy Y) just to pass two floats through the hottest loop in the renderer.
func behindCullXZ(camPos, forward rl.Vector3, px, pz float32) bool {
	dx := px - camPos.X
	dz := pz - camPos.Z
	return dx*forward.X+dz*forward.Z < behindCullSlack
}

// viewCull is the per-frame horizontal view-frustum test — camera position,
// horizontal basis, and the side-plane half-tangent hoisted out of the per-item
// loops. A point is culled when it sits behind the camera (the original
// back-plane rule, behindCullSlack) OR outside the horizontal FOV wedge; both
// are off-screen, so dropping them costs nothing visible. It extends behindCull
// (which only checks the back plane) with the two side planes — on a wide map
// the old test kept every tile abreast of and beside the camera even when it
// projected far off the screen edge. Built once per draw via newViewCull; the
// world tile loop and the chest/door/crystal draws share it so they cull
// consistently.
type viewCull struct {
	pos     rl.Vector3
	fwd     rl.Vector3
	right   rl.Vector3
	tanHalf float32
}

const (
	// viewCullApexBack pushes the cone apex this far behind the camera, so near
	// and just-off-to-the-side tiles (small forward component) stay well inside
	// the wedge — the near-field half-width at the camera plane is
	// viewCullApexBack*tanHalf. Kept >= |behindCullSlack| so the side test never
	// fires inside the band the back-plane test deliberately keeps.
	viewCullApexBack = float32(3.0)
	// viewCullSlack widens the horizontal half-tangent so the cull boundary sits
	// comfortably outside the true screen edge — margin for a tile whose center
	// is just past the edge but whose 1-unit slab / overhanging prop is still
	// partly visible. Conservative on purpose: a 30% wider cone never drops
	// anything on screen.
	viewCullSlack = float32(1.3)
)

func newViewCull(camera rl.Camera3D) viewCull {
	fwd := horizontalForward(camera)
	// camera.Fovy is the VERTICAL fov in degrees; the horizontal half-angle
	// scales its tangent by the screen aspect (tan(Fovy/2)·aspect), then widens
	// by viewCullSlack. Fovy*Pi/360 == (Fovy/2)·deg2rad.
	sw, sh := screenSizeF()
	aspect := float32(1)
	if sh > 0 {
		aspect = sw / sh
	}
	tanHalf := float32(math.Tan(float64(camera.Fovy)*math.Pi/360)) * aspect * viewCullSlack
	return viewCull{pos: camera.Position, fwd: fwd, right: horizontalRight(fwd), tanHalf: tanHalf}
}

// cullXZ reports whether the world point (px,pz) is outside the view — behind
// the camera or beyond the horizontal wedge — and can be skipped.
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

// DrawWorld draws the full lit environment pass — see drawWorld.
func DrawWorld(camera rl.Camera3D, g *core.GameState, assets Resources) {
	drawWorld(camera, g, assets, false)
}

// drawWorld rasterizes the environment geometry (sky-less: floors, walls,
// ceilings, elevation columns, props, decor, ramps).
//
// depthOnly=false is the normal pass: recompute the lighting profile, upload
// the sun/fog/torch uniforms, and (when the render log is on) gather per-tile
// diagnostics.
//
// depthOnly=true is the retro-filter depth prepass (see RetroDepthPrepass). It
// SKIPS all of that lighting CPU setup and the diagnostics, drawing only the
// geometry. The prepass runs in the SAME frame immediately after a full
// DrawWorld whose uniforms still bind the lighting shader, with the SAME camera
// and time-of-day profile — so collectTorches / applyUniforms / uploadTorches /
// cacheLightingProfile would recompute byte-identical values. The geometry
// drawn (same models, same transforms, ground shadows and torch flames
// included) is identical to the full pass, so the rebuilt depth buffer matches
// the captured one exactly and the crisp sprite pass can't z-fight it. The
// per-pixel lighting shader still runs (it's attached to every model's
// material, not switchable via BeginShaderMode), but the redundant CPU lighting
// re-setup and the torch grid-scan are elided.
func drawWorld(camera rl.Camera3D, g *core.GameState, assets Resources, depthOnly bool) {
	m := &g.Area
	material := assets.worldMaterial(m.Materials)
	var profile lightingProfile
	var torches []torchLight
	if !depthOnly {
		profile = applyTimeOfDay(lightingFor(m.Materials), timeProfileAt(g.StepCount), areaIsEnclosed(m))
		cacheLightingProfile(profile)
		assets.lighting.applyUniforms(camera, profile)
		// Torch point lights — collect the brazier props nearest the
		// camera, flicker them, and upload before the geometry pass so
		// walls / floors / props pick up the warm pools of light. Must
		// run after applyUniforms (same shader) and before the tile
		// loop's BeginShaderMode draws.
		torches = collectTorches(m, camera)
		assets.lighting.uploadTorches(torches)
	}

	camPos := camera.Position
	vc := newViewCull(camera)

	// Diagnostics: only collect counters when the render log is on,
	// so the hot path stays a plain increment-free loop the rest of
	// the time. logActive is a single function-call check. The depth
	// prepass never logs — it's a duplicate of the full pass that ran
	// this frame, so its counts would double-report.
	logActive := IsRenderLogActive() && !depthOnly
	var stats renderFrameStats
	if logActive {
		stats.MapW = m.Width
		stats.MapH = m.Height
	}

	// Decode the elevation + ramp + face-skin of every tile ONCE into a reused
	// flat grid, instead of re-deriving each tile's (and its 4 neighbours')
	// level/ramp through value-receiver string lookups inside the hot loop —
	// which previously decoded each tile's level ~5× per pass. The loop and the
	// cliff-face pass then read plain ints/bytes from this grid.
	gw, gh := m.Width, m.Height
	grid := elevGrid(m, gw, gh)

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
			// Elevation: this tile's floor (and everything on it) rides up by
			// its level. The world is a heightfield — a "wall" is the rendered
			// vertical FACE of an elevation step (drawCliffFaces below), not a
			// separate solid tile. A raised tile reads as a plateau/mesa; the
			// faces on its lower-facing edges are its cliff. Read level+ramp from
			// the prebuilt grid (computed once, above) rather than re-decoding.
			te := grid[z*gw+x]
			elevY := core.ElevationWorldY(te.level)
			// Scenery anchors to the level it was PLACED on: props via PropLevelAt
			// and decor via DecorLevelAt, each computed at their own draw call below
			// (StandGroundYAt handles the ramp mid-slope + the voxel ground default).
			if m.CeilingAt(x, z) {
				drawTileCube(material.ceilingModel, cx, core.LevelStep+elevY, cz, tileYawDeg(x, z))
				if logActive {
					stats.CeilingsDrawn++
				}
			}
			if len(m.Solids) > 0 {
				// Voxel path: floors on every standable surface, side faces per
				// solid run, and floating-cube undersides. Only gapped maps take
				// this branch — heightfields keep the original path below.
				n := drawVoxelColumn(camPos, material, assets, m, x, z, cx, cz)
				if logActive {
					stats.FloorsDrawn++
					stats.WallsDrawn += n
				}
			} else {
				drawFloorTile(material, assets, m.Floor[z][x], x, z, cx, cz, elevY)
				if logActive {
					stats.FloorsDrawn++
				}
				// Cliff faces for every edge where this tile sits above its
				// neighbour (or the map edge). Counted as WallsDrawn for the log.
				if n := drawCliffFaces(camPos, material, assets, grid, gw, gh, x, z, cx, cz, te.level, te.ramp); logActive {
					stats.WallsDrawn += n
				}
			}
			// Decor sits on its placed level too (deck vs ground); on a heightfield
			// column DecorLevelAt is the single surface, so flat maps are unchanged.
			// Guard on non-empty so the per-tile StandGroundYAt + DecorLevelAt anchor
			// math is skipped for the (common) empty-decor tiles drawDecor would
			// no-op on anyway.
			if decor := m.Decor[z][x]; decor != core.DecorEmpty {
				decorCenter := rl.NewVector3(cx, m.StandGroundYAt(x, te.decorLevel, z), cz)
				drawDecor(assets, decor, x, z, cx, cz, decorCenter)
				if logActive {
					// DecorAuto still counts — the floor scatter is decor.
					stats.DecorDrawn++
				}
			}
			if prop := m.Props[z][x]; prop != core.TilePropEmpty {
				propYaw := propYawDeg(x, z)
				// A prop sits on the level it was placed on (PropLevelAt) — the
				// ground by default, but a deck/overhang level for a prop authored
				// up there. On a heightfield column this equals `center` (the single
				// surface), so flat maps are unchanged.
				propCenter := rl.NewVector3(cx, m.StandGroundYAt(x, te.propLevel, z), cz)
				drawn := false
				if handler := inlinePropTable[prop]; handler != nil {
					handler(assets, m, x, z, propCenter, propYaw)
					drawn = true
				} else if footprint := core.PropFootprint(prop); footprint != nil {
					if pm := &assets.propModelTable[prop]; len(pm.parts) > 0 {
						anchor := footprintAnchor(propCenter, footprint)
						if r := propShadowRadiusTable[prop]; r > 0 {
							drawGroundShadowElev(anchor.X, anchor.Z, anchor.Y, r)
						}
						pm.draw(anchor, 1.0, propYaw)
						drawn = true
					}
				} else if pm := &assets.propModelTable[prop]; len(pm.parts) > 0 {
					if r := propShadowRadiusTable[prop]; r > 0 {
						drawGroundShadowElev(propCenter.X, propCenter.Z, propCenter.Y, r)
					}
					pm.draw(propCenter, 1.0, propYaw)
					drawn = true
				}
				if logActive && drawn {
					stats.PropsDrawn++
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
		stats.PlayerYaw = g.Player.Yaw
		stats.PlayerLookYaw = g.Player.LookYaw
		stats.PlayerLookPitch = g.Player.LookPitch
		stats.StepCount = g.StepCount
		stats.LightingShaderID = assets.lighting.shader.ID
		stats.BillboardFogID = assets.billboardFog.shader.ID
		stats.FogDensity = profile.FogDensity
		stats.FogColor = profile.FogColor
		stats.AmbientColor = profile.AmbientColor
		stats.SunColor = profile.SunColor
		stats.BattleActive = g.Battle.Active()
		LogRenderFrame(stats)
	}
}

// drawFloorTile picks a floor variant for the given tile and draws it.
// footprintAnchor returns the world position of the centroid of a
// multi-tile footprint, given the anchor tile's center. Wraps
// core.FootprintWorldOffset so the per-call-site addition + Vector3
// construction lives in one place — both the props branch and the
// decor branch of the world renderer would otherwise repeat the same
// `center + (offX, 0, offZ)` arithmetic.
func footprintAnchor(center rl.Vector3, footprint []core.MultiTileOffset) rl.Vector3 {
	ox, oz := core.FootprintWorldOffset(footprint)
	return rl.NewVector3(center.X+ox, center.Y, center.Z+oz)
}

// `cell` is the floor-layer character. Resolution order:
//
//  1. Universal floor variants (cobble, plank, water, sand, snow) live
//     in assets.specialFloors and apply to any material set.
//  2. Material-specific variants (dirt / dark grass on the field) come
//     from the material's worldMaterialResources.
//  3. Auto/unrecognized chars fall back to the per-tile hash for variant
//     selection on materials that support it; otherwise the base floor.
//
// Universal variants render at the same y as the base floor so adjacent
// tiles meet flush without visible seams.
func drawFloorTile(material worldMaterialResources, assets Resources, cell byte, x, z int, cx, cz, elevY float32) {
	yaw := tileYawDeg(x, z)
	// Floor slabs sit a hair below the tile's elevation height so they meet the
	// cliff faces flush without z-fighting; one offset shared by all three slab
	// draws below.
	floorY := elevY - 0.03
	// Ramp tiles draw a solid earth wedge (their Elevation cell is the LOW
	// level, so elevY is the low edge height) instead of a flat floor slab.
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
	// All explicit floor chars (grass, dirt, dark grass, stone, cobble,
	// plank, water, sand, snow) route through specialFloors above. What
	// reaches here is FloorAuto (or anything unrecognized) — pick a
	// per-tile variant by hash so the field reads as varied terrain.
	model := material.floorModel
	switch floorVariantHash(x, z) {
	case 1:
		model = material.floorDirtModel
	case 2:
		model = material.floorDarkModel
	}
	drawTileCube(model, cx, floorY, cz, yaw)
}

// drawCliffFaces renders the vertical rock faces of tile (x,z) — one per
// cardinal edge where this tile's ground sits ABOVE the neighbour's (or the
// map edge), which is exactly where StepElevationOK forbids a step. A wall is
// just these faces. Ramp tiles draw their own solid wedge (with side/back
// walls) instead, so they're skipped here. Returns the number of faces drawn
// (for the render-log's WallsDrawn tally).
// tileElev is the per-tile elevation data the world loop needs, decoded once
// into elevGridBuf so the hot loop reads ints/bytes instead of re-running
// value-receiver string lookups for every tile and its four neighbours.
type tileElev struct {
	level int
	ramp  int  // ascent facing, or core.NoRamp on a flat tile
	skin  byte // cliff-face skin char (core.FaceSkinAt)
	// faceSkins is the resolved cliff-face skin char per cardinal direction
	// (index = direction constant N=0/E=1/S=2/W=3): the tile's per-direction
	// override (FaceSkinForDir) when set, else its base skin. Cached here so the
	// per-frame drawCliffFaces reads a byte instead of re-scanning the whole
	// FaceOverrides slice for every exposed edge every frame on maps that use
	// face overrides — the linear scan now runs once per area at decode time.
	faceSkins [4]byte
	// decorLevel / propLevel are the surfaces decor and props anchor to
	// (DecorLevelAt / PropLevelAt). Cached here because on a VOXEL map an
	// auto-level tile resolves through LowestStandableLevel — an O(stackHeight)
	// column rescan — which would otherwise run per visible decor/prop tile
	// every frame. Decoded once with the rest of the grid; the cache key already
	// hashes Solids, so a runtime cube edit rebuilds these too.
	decorLevel int
	propLevel  int
}

// elevGridBuf is reused across frames + passes to avoid an allocation per draw.
var elevGridBuf []tileElev

// elevGridKey fingerprints the area elevGridBuf was last decoded for, so the
// full Width×Height decode runs once per area entry instead of every frame (and
// every depthOnly re-pass within a frame) — the same once-per-area idea as
// torchSiteCache / enclosureCache. The grid derives from Floor (ramps),
// Elevation (levels) and Walls (face skins); the key is a CONTENT HASH of all
// three layers plus name+dims, so it rebuilds whenever any of them actually
// changes and can never serve a stale grid. (The sibling caches sample only
// boundary rows because they gate an invisible verdict; a stale elevation grid
// would mis-render every wall/floor height, so it's worth hashing in full.)
// Hashing is a plain allocation-free byte fold with no per-tile method calls or
// struct copies, so validation stays far cheaper than the decode it guards —
// and in-game these layers are static, so the decode runs once per area entry.
var elevGridKey struct {
	primed        bool
	name          string
	width, height int
	hash          uint64
}

// fnvOffsetBasis is the FNV-1a 64-bit offset basis the layer hash seeds from.
const fnvOffsetBasis = uint64(1469598103934665603)

// foldLayer folds one grid layer's bytes into the running FNV-1a digest h, with
// a per-row separator and a trailing layer separator so ragged splits
// ([{"ab"},{"c"}] vs [{"a"},{"bc"}]) can't collide. Pulled out of layersHash so
// elevGrid can fold the heightfield layers plus every Solids plane in sequence
// without allocating a wrapper [][]string each frame just to pass them
// variadically. Allocation-free.
func foldLayer(h uint64, layer []string) uint64 {
	const prime = 1099511628211
	for _, row := range layer {
		for i := 0; i < len(row); i++ {
			h = (h ^ uint64(row[i])) * prime
		}
		h = (h ^ 0xff) * prime // row separator
	}
	return (h ^ 0xfe) * prime // layer separator
}

// layersHash folds the bytes of the given layers into one FNV-1a digest — the
// content fingerprint elevGridKey validates against. Allocation-free.
func layersHash(layers ...[]string) uint64 {
	h := fnvOffsetBasis
	for _, layer := range layers {
		h = foldLayer(h, layer)
	}
	return h
}

// elevGrid decodes every tile's level/ramp/skin into the reused flat buffer,
// rebuilding only when the Floor/Elevation/Walls content (or dims/name) change.
func elevGrid(m *core.AreaDefinition, w, h int) []tileElev {
	// Hash Floor/Elevation/Walls (the heightfield inputs) plus every Solids
	// plane, so a runtime edit to the voxel stack invalidates the cache too.
	// Folded in sequence (no wrapper slice) so the per-frame validity check
	// allocates nothing.
	hash := foldLayer(foldLayer(foldLayer(fnvOffsetBasis, m.Floor), m.Elevation), m.Walls)
	for _, plane := range m.Solids {
		hash = foldLayer(hash, plane)
	}
	k := &elevGridKey
	if k.primed && k.name == m.Name && k.width == w && k.height == h &&
		k.hash == hash && cap(elevGridBuf) >= w*h {
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
			// Resolve each cardinal face's skin once here (override-or-base) so the
			// per-frame cliff pass never re-scans FaceOverrides. Direction index
			// matches the constants (N=0/E=1/S=2/W=3); FaceSkinForDir falls back to
			// the base skin when there's no override, so flat/un-overridden tiles
			// just carry their base skin on every face.
			var faces [4]byte
			for d := 0; d < 4; d++ {
				faces[d] = m.FaceSkinForDir(x, z, d)
			}
			elevGridBuf[z*w+x] = tileElev{
				level:      m.ElevationLevelAt(x, z),
				ramp:       ramp,
				skin:       m.FaceSkinAt(x, z),
				faceSkins:  faces,
				decorLevel: m.DecorLevelAt(x, z),
				propLevel:  m.PropLevelAt(x, z),
			}
		}
	}
	k.name, k.width, k.height, k.hash, k.primed = m.Name, w, h, hash, true
	return elevGridBuf
}

func drawCliffFaces(camPos rl.Vector3, material worldMaterialResources, assets Resources, grid []tileElev, w, h, x, z int, cx, cz float32, myLevel, myRamp int) int {
	if myRamp != core.NoRamp {
		return 0 // the ramp wedge supplies its own faces
	}
	const half = float32(core.TileSize) / 2
	drawn := 0
	// Per-edge mirror of core.TileExposesFace (the editor gates its "Set face"
	// menu on that authority) — kept inline here so it reads the per-frame grid
	// instead of re-decoding the area, and resolves each edge's exact drop for
	// the draw. Off-map default + EdgeLevelOf fallback match core.NeighbourEdgeLevel.
	for _, d := range core.CardinalDirs {
		dx, dz := core.FacingVector(d)
		// CPU backface cull: a vertical cliff face is only visible from its
		// outward (d) side. Skip issuing the DrawModelEx when the camera sits
		// behind the face's plane — the GPU would discard those triangles
		// anyway, but the per-call overhead is the real cost, and a dense
		// heightfield generates one face per exposed edge. Pure win, no visual
		// change (you can never stand on the solid side of a cliff face).
		fdx, fdz := float32(dx), float32(dz)
		if faceBackfaceCulled(camPos, cx, cz, fdx, fdz, half) {
			continue
		}
		nx, nz := x+dx, z+dz
		// Neighbour ground level across the shared edge from the grid: ramp-
		// aware (EdgeLevelOf) when it presents a walkable edge, else its flat
		// level. Off-map reads as the baseline, so a raised border shows a clean
		// lip at the map edge rather than a cliff plunging to the range bottom.
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
		// Per-DIRECTION skin: this edge's override or the tile's base skin, both
		// resolved once at decode into the grid's faceSkins, so the per-frame draw
		// reads a byte instead of re-scanning FaceOverrides for every exposed edge.
		skin := material.faceModel
		if sc := grid[z*w+x].faceSkins[d]; assets.faceVariantTable.present[sc] {
			skin = assets.faceVariantTable.model[sc]
		}
		drawCliffFace(skin, cx, core.ElevationWorldY(nLevel), cz, faceYaw(d), float32(myLevel-nLevel))
		drawn++
	}
	return drawn
}

// faceYaw maps the dropping-edge direction to the Y-rotation that turns the
// face-quad model (built on the +Z / south edge, normal +Z) so it sits on that
// edge with its skin pointing outward toward the lower neighbour. From raylib's
// Y-rotation +Z → (sinθ,0,cosθ): θ=0→+Z(S), 90→+X(E), 180→-Z(N), 270→-X(W).
func faceYaw(d int) float32 {
	switch d {
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
// delta so the single LevelStep-tall model covers the whole cliff.
func drawCliffFace(model rl.Model, cx, baseY, cz, yaw, levels float32) {
	rl.DrawModelEx(model,
		rl.NewVector3(cx, baseY, cz),
		rl.NewVector3(0, 1, 0), yaw,
		rl.NewVector3(1, levels, 1), rl.White)
}

// triNormal returns the unit normal of triangle (a,b,c) by the right-hand
// rule (CCW → outward). Used by the ramp-wedge mesh builder to orient faces.
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

// rampFacingYaw maps a ramp's ascent facing to the Y-rotation (degrees) that
// turns the wedge model (built ascending toward -Z / north) to face that way.
// Verified against the FacingVector convention (North 0,-1; East 1,0; etc.).
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

// drawRampWedge draws the shared solid ramp wedge at tile (cx,cz) with its low
// edge resting at lowY (= lowLevel·LevelStep), yaw-rotated to ascend toward
// `facing`. The model's geometry guarantees the high edge lands one LevelStep
// up — flush with the higher floor — and the footprint fills the tile.
func drawRampWedge(model rl.Model, cx, cz, lowY float32, facing int) {
	rl.DrawModelEx(model,
		rl.NewVector3(cx, lowY, cz),
		rl.NewVector3(0, 1, 0), rampFacingYaw(facing),
		rl.NewVector3(1, 1, 1), rl.White)
}

// drawDecor renders the floor-layer decoration for a tile. '.' falls through
// to the existing auto-scatter (hash decides whether to draw and what);
// '_' suppresses the auto-scatter entirely; explicit chars draw a specific
// small prop centered on the tile.
//
// The new decor set (tall grass, flowers, clover, reeds, bones, scorch,
// blood, cobweb, stump, log, leaf pile) lives in assets.decorModels keyed
// by char. The legacy bush / mushroom / pebble cases stay inline so their
// per-call scales and the pebble-cluster scatter helper keep their hand
// tuning.
func drawDecor(assets Resources, cell byte, x, z int, cx, cz float32, center rl.Vector3) {
	switch cell {
	case core.DecorEmpty:
		return
	case core.DecorAuto:
		drawFloorDecoration(assets, x, z, cx, cz, center.Y)
		return
	}
	// Inline-handled decor (bush / mushroom / pebble) dispatches
	// through the inlineDecorTable in resources.go — a [256] array
	// mirror of inlineDecorHandlers so the per-tile-per-frame hot path
	// is an array index instead of a map hash.
	if handler := inlineDecorTable[cell]; handler != nil {
		handler(assets, x, z, cx, cz, center.Y)
		return
	}
	if footprint := core.DecorFootprint(cell); footprint != nil {
		if dm := &assets.decorModelTable[cell]; len(dm.parts) > 0 {
			dm.draw(footprintAnchor(center, footprint), 1.0, 0)
		}
		return
	}
	if dm := &assets.decorModelTable[cell]; len(dm.parts) > 0 {
		dm.draw(center, 1.0, propYawDeg(x, z))
	}
}

// drawPropTreeScaled / drawPropTreeTwin / drawPropRockLarge / drawPropBushLarge
// are the inline-prop implementations registered in inlinePropHandlers. The
// four scaled-tree chars share assets.tree through drawPropTreeScaled (see
// treePropScales below); the other two wrap dedicated propModel fields.
// Pre-resolved propYaw is passed in by the caller so all handlers stay uniform.
// treePropScales is the scale-per-char table for tree variants that
// share assets.tree at different sizes. Four of the five tree props
// differ only in their scale factor — Tree / TreeXL / TreeTall /
// TreeYoung — so they all dispatch through drawPropTreeScaled keyed
// by char rather than each having a one-line wrapper. TileTreeTwin
// stays separate because it draws two instances per tile.
//
// Young trees used to read as bonsai at 0.65 — bumped to 0.92 so they
// still feel smaller than a grown tree but actually occupy their tile.
var treePropScales = map[byte]float32{
	core.TileTree:      1.00,
	core.TileTreeXL:    1.75,
	core.TileTreeTall:  1.40,
	core.TileTreeYoung: 0.92,
}

// foliageTrunkShadowFactor is the fraction of a foliage prop's scale its
// ground-shadow disc spans before per-prop slack — shared by the tree props so
// the disc tracks the trunk footprint consistently instead of each loader
// re-deriving the 0.34 factor.
const foliageTrunkShadowFactor = 0.34

// foliageShadowRadius returns the ground-shadow disc radius for a foliage prop
// at the given scale, plus a little slack so the painted disc sits a touch
// wider than the trunk's projected footprint.
func foliageShadowRadius(scale, slack float32) float32 {
	return foliageTrunkShadowFactor*scale + slack
}

// drawPropTreeScaled draws assets.tree at the scale registered in
// treePropScales for the given char. The inline-prop dispatcher binds
// each per-char closure at init so the prop-renderer call site stays
// "table lookup → invoke" with no branch on char. Per-tile shape
// variance is seeded from tileHash so a stand of identical-char trees
// no longer reads as a stamped grid.
func drawPropTreeScaled(char byte) inlinePropRenderer {
	scale := treePropScales[char]
	return func(assets Resources, _ *core.AreaDefinition, x, z int, center rl.Vector3, propYaw float32) {
		// Tree shadow scales with the tree's overall scale, plus
		// a touch of slack so the painted disc sits a little wider
		// than the trunk's projected footprint.
		drawGroundShadowElev(center.X, center.Z, center.Y, foliageShadowRadius(scale, 0.10))
		assets.tree.drawVaried(center, scale, propYaw, tileHash(x, z))
	}
}

// drawPropTreeTwin renders two trees stacked into one tile, offset
// diagonally so neither sits in the dead center. The two instances
// use different scales so the silhouette reads as "big tree with a
// younger one beside it" rather than a mirrored pair. Yaw is staggered
// and each gets its own variance seed. Pure visual variant — both
// reuse assets.tree.
func drawPropTreeTwin(assets Resources, _ *core.AreaDefinition, x, z int, center rl.Vector3, propYaw float32) {
	const offset = 0.32
	const scaleBig = 0.82
	const scaleSmall = 0.58
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
	assets.rockProp.draw(center, 1.0, propYaw)
}

func drawPropBushLarge(assets Resources, _ *core.AreaDefinition, _, _ int, center rl.Vector3, propYaw float32) {
	drawGroundShadowElev(center.X, center.Z, center.Y, 0.48)
	assets.bushProp.draw(center, 1.3, propYaw)
}

// Wall-torch fixture geometry — shared by drawWallTorch (the visible
// bracket/sconce) and rebuildTorchSites (the light-pool origin) so the
// fixture height and its light source can't silently drift apart on a
// retune. The light origin sits a touch above the sconce cup (where the
// flame burns) and is inset toward the wall like the bracket.
const (
	wallTorchMount      = float32(0.40) // bracket distance from tile center toward the wall
	wallTorchSconceY    = float32(1.30) // bracket/sconce height
	wallTorchLightY     = float32(1.42) // light-pool origin height (just above the flame cup)
	wallTorchLightInset = float32(0.30) // light origin offset from tile center toward the wall
)

// drawWallTorch is the inline handler for TileTorch. It auto-orients
// the torch to the adjacent wall (facing away from it into the room),
// draws an unlit iron bracket + sconce on the wall, and an animated
// emissive flame made of a few jittering fire-tinted spheres. The
// point light itself is added by collectTorches; this is purely the
// visible fixture + flame. Non-blocking: the floor tile stays clear.
func drawWallTorch(assets Resources, m *core.AreaDefinition, x, z int, center rl.Vector3, _ float32) {
	fx, fz := wallTorchFacing(m, x, z)
	// Mount point: against the wall behind the torch, up at sconce
	// height. The torch faces (fx,fz) into the room, so the wall is
	// in the opposite direction.
	wallX := center.X - fx*wallTorchMount
	wallZ := center.Z - fz*wallTorchMount
	// All fixture heights ride the tile's elevation floor (center.Y) so a
	// torch on a raised tile stays mounted to its wall instead of hanging at
	// world-ground height.
	baseY := center.Y

	// Iron bracket — a small dark cube flush on the wall, plus a
	// short arm reaching out toward the room holding the sconce.
	// Drawn lit (immediate mode under the world shader); the torch's
	// own light pool keeps it visible.
	bracket := rl.NewVector3(wallX, baseY+wallTorchSconceY-0.12, wallZ)
	rl.DrawCube(bracket, 0.10, 0.22, 0.10, torchIron)
	armX := wallX + fx*0.10
	armZ := wallZ + fz*0.10
	rl.DrawCube(rl.NewVector3(armX, baseY+wallTorchSconceY, armZ), 0.08, 0.06, 0.08, torchIron)
	// Sconce cup at the arm tip.
	cupX := wallX + fx*0.16
	cupZ := wallZ + fz*0.16
	rl.DrawCube(rl.NewVector3(cupX, baseY+wallTorchSconceY+0.04, cupZ), 0.16, 0.08, 0.16, torchIronLight)

	// Animated flame — three emissive blobs above the cup, each
	// bobbing on its own time offset so the flame flickers and
	// dances. Drawn via the unlit flame model (default shader) so
	// they glow regardless of the near-black dungeon ambient.
	if !torchFlameReady {
		return
	}
	t := float32(rl.GetTime())
	phase := hashPhase(tileHash(x, z))
	flameBaseX := cupX
	flameBaseZ := cupZ
	for i := 0; i < 3; i++ {
		fp := phase + float32(i)*2.1
		bob := float32(math.Sin(float64(t*7.0+fp))) * 0.04
		swayA := float32(math.Sin(float64(t*5.3+fp*1.4))) * 0.05
		// Higher blobs are smaller and lean more — a teardrop
		// flame shape that wavers.
		y := baseY + wallTorchSconceY + 0.09 + float32(i)*0.07 + bob
		lean := float32(i) * 0.03
		px := flameBaseX + fx*lean + swayA*fz
		pz := flameBaseZ + fz*lean - swayA*fx
		size := (0.11 - float32(i)*0.025) * (1 + 0.12*float32(math.Sin(float64(t*11.0+fp))))
		tint := torchFlameTints[i]
		rl.DrawModelEx(torchFlameModel, rl.NewVector3(px, y, pz),
			rl.NewVector3(0, 1, 0), 0, rl.NewVector3(size, size*1.4, size), tint)
	}
}

// wallTorchFacing returns the unit (x,z) direction the torch faces —
// away from the first adjacent wall found, checked N→E→S→W. Falls
// back to facing south (toward the camera's usual approach) when the
// tile has no adjacent wall (a torch placed in the open).
func wallTorchFacing(m *core.AreaDefinition, x, z int) (float32, float32) {
	if f, ok := core.FacingAwayFromAdjacentWall(*m, x, z); ok {
		dx, dz := core.FacingVector(f)
		return float32(dx), float32(dz)
	}
	return 0, 1 // no adjacent wall → face south (toward the usual approach)
}

// groundShadowModel is the soft radial-gradient disc painted under
// every prop. A flat XZ plane textured with makeSoftShadowPixels'
// dark-centre / transparent-edge sprite, drawn UNLIT (it keeps the
// default material shader, so the lighting pass bound around the
// world draw never touches it) and alpha-blended over the floor.
// Set once by NewResources; drawGroundShadow reads it as a package
// singleton because shadows are painted from many free-function
// call sites (tree handlers, the prop branch, DrawChests) that don't
// thread Resources through. groundShadowReady guards the pre-init
// window so an early draw is a no-op rather than a crash.
var (
	groundShadowModel rl.Model
	groundShadowReady bool
)

// propShadowRadius is the per-prop ground-shadow half-extent (world
// units). drawWorld's prop branch looks each prop up here and paints
// a soft dark disc at the prop's base before drawing the prop itself —
// the Wind-Waker grounding signature so every painted prop reads as
// planted in the floor instead of floating on the lighting gradient.
// Inline-handled props (trees, big bushes, big rocks, small mushrooms,
// small bushes) paint their own shadows directly inside their
// handlers; only the table-dispatched props live here.
//
// Sizes are roughly the prop's projected footprint plus a touch of
// slack so the painted disc reads slightly wider than the silhouette.
// 2x2 footprint props (rock formation) get a wider radius matched to
// their multi-tile span.
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

// propShadowRadiusTable is the [256]float32 mirror of propShadowRadius,
// indexed by tile char so the per-tile world loop does an O(1) array
// index instead of a map hash for every prop tile every frame (mirrors
// inlinePropTable / decorModelTable). Built once at init; chars with no
// entry read 0 (no shadow). propShadowRadius stays the authoring source.
var propShadowRadiusTable = func() [256]float32 {
	var t [256]float32
	for ch, r := range propShadowRadius {
		t[ch] = r
	}
	return t
}()

// areaKey identifies the area a per-area cache was built for, so the cache
// rebuilds only when the player enters a different area. Matched on name +
// dimensions PLUS a ceiling fingerprint (core.CeilingFingerprint) — without
// the fingerprint, two distinct same-named, same-sized areas with different
// roofs would share a stale enclosure/torch verdict (the editor "untitled"
// case). Shares the fingerprint with core's outdoorVerdictCache so the
// lighting/torch gates and the rain gate can't drift. Used by enclosureCache
// and torchSiteCache.
type areaKey struct {
	name          string
	width, height int
	rows          int
	top, bot      string
	primed        bool
}

func (k *areaKey) matches(m *core.AreaDefinition) bool {
	rows, top, bot := core.CeilingFingerprint(m)
	return k.primed && k.name == m.Name && k.width == m.Width && k.height == m.Height &&
		k.rows == rows && k.top == top && k.bot == bot
}

func (k *areaKey) set(m *core.AreaDefinition) {
	k.rows, k.top, k.bot = core.CeilingFingerprint(m)
	k.name, k.width, k.height, k.primed = m.Name, m.Width, m.Height, true
}

// enclosureCache memoizes the last area's enclosure result so the
// ceiling-coverage scan runs once per area, not once per frame.
var enclosureCache struct {
	areaKey
	enclosed bool
}

// areaIsEnclosed reports whether the area is a roofed interior — used to
// gate the spooky-dungeon lighting override. The ceiling-coverage rule
// (and the OutdoorCeilingThreshold it tests against) lives in
// core.AreaIsOutdoor so the lighting gate and the rain gate share one
// definition of "has a roof"; this just memoizes its result per area so
// the scan runs once per area entry rather than once per frame.
func areaIsEnclosed(m *core.AreaDefinition) bool {
	if enclosureCache.matches(m) {
		return enclosureCache.enclosed
	}
	enclosed := !core.AreaIsOutdoor(m)
	enclosureCache.set(m)
	enclosureCache.enclosed = enclosed
	return enclosed
}

// Wall-torch fixture + flame palette. Iron tones are lit by the
// world shader; the flame tints are applied to the unlit
// torchFlameModel so they glow. Three flame tints (hot core →
// mid → tip) layer the bobbing blobs into a teardrop fire.
var (
	torchIron       = rl.NewColor(54, 50, 46, 255)
	torchIronLight  = rl.NewColor(92, 84, 76, 255)
	torchFlameTints = [3]rl.Color{
		rl.NewColor(255, 226, 150, 255), // hot core — pale gold
		rl.NewColor(252, 162, 70, 255),  // mid — orange
		rl.NewColor(228, 110, 52, 255),  // tip — deep ember
	}
)

// torchFlameModel is the unlit emissive sphere used for wall-torch
// flame blobs. Default material shader (like groundShadowModel) so
// it renders at full tint colour, glowing against the near-black
// dungeon. Set by NewResources.
var (
	torchFlameModel rl.Model
	torchFlameReady bool
)

// torchFlameHeight is the world Y at which a brazier's torch point
// light sits — up at the fire bowl, not the floor, so the light
// pool radiates outward and down across the surrounding tiles.
const torchFlameHeight = float32(1.05)

// torchBaseColor is the warm flame tint at full brightness, before
// per-torch flicker. Deliberately bright (R well over 1) so a
// torch-lit wall reads as a strong warm-orange pool against the
// dim dungeon while the space between torches falls into shadow.
var torchBaseColor = rl.NewVector3(2.3, 1.35, 0.7)

type torchCandidate struct {
	pos    rl.Vector3
	dist   float32
	hash   uint32
	bright float32 // brightness multiplier — braziers > wall torches
}

// torchCandidateBuf / torchResultBuf are reused across frames so the
// per-frame brazier scan + torch build don't allocate.
var (
	torchCandidateBuf []torchCandidate
	torchResultBuf    []torchLight
)

// collectTorches scans the area's props for brazier tiles, keeps the
// maxTorches nearest the camera, and returns them as flickering torch
// point lights for the lighting shader. Braziers beyond the cap are
// dropped — they'd be fog-swallowed in the dark anyway. Flicker is a
// per-torch pair of desynced sines seeded from the tile hash so
// neighbouring torches wobble independently instead of pulsing in
// lockstep. Returns an empty slice on areas with no braziers (every
// field map), so the shader's torch loop contributes nothing.
// torchSite is the static (camera- and time-independent) data for one
// brazier/torch tile: its light origin, the tile center used for camera
// ranking, the flicker seed, and base brightness. All of it is fixed for
// the lifetime of an area, so it's cached rather than rediscovered by a
// full-grid scan every frame.
type torchSite struct {
	pos    rl.Vector3
	cx, cz float32
	hash   uint32
	bright float32
}

// torchSiteCache memoizes the brazier/torch tile list for the current
// area so the Width×Height grid scan runs once per area, not per frame
// (mirrors enclosureCache). Per-frame work then reduces to distance +
// flicker over the cached handful of sites.
var torchSiteCache struct {
	areaKey
	sites []torchSite
}

func rebuildTorchSites(m *core.AreaDefinition) {
	torchSiteCache.sites = torchSiteCache.sites[:0]
	for z := 0; z < m.Height; z++ {
		for x := 0; x < m.Width; x++ {
			prop := m.Props[z][x]
			isBrazier := prop == core.TileBrazier
			isTorch := prop == core.TileTorch
			if !isBrazier && !isTorch {
				continue
			}
			cx := core.TileCenter(x)
			cz := core.TileCenter(z)
			// Light origin rides the tile's elevation so a raised torch/brazier
			// lights at its actual flame height, matching the visible fixture.
			// Use the walkable-surface height (mid-slope on a ramp tile, same as
			// the fixture's scenery anchor) rather than the low-edge level.
			elevY := m.StandGroundY(x, z)
			var pos rl.Vector3
			bright := float32(0.85) // wall torch — dimmer
			if isBrazier {
				// Floor brazier: flame at the bowl, brighter pool.
				pos = rl.NewVector3(cx, elevY+torchFlameHeight, cz)
				bright = 1.45
			} else {
				// Wall torch: light originates at the sconce, offset
				// toward the wall + up at flame height.
				fx, fz := wallTorchFacing(m, x, z)
				pos = rl.NewVector3(cx-fx*wallTorchLightInset, elevY+wallTorchLightY, cz-fz*wallTorchLightInset)
			}
			torchSiteCache.sites = append(torchSiteCache.sites, torchSite{
				pos: pos, cx: cx, cz: cz, hash: tileHash(x, z), bright: bright,
			})
		}
	}
	torchSiteCache.set(m)
}

func collectTorches(m *core.AreaDefinition, camera rl.Camera3D) []torchLight {
	if !torchSiteCache.matches(m) {
		rebuildTorchSites(m)
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
		})
	}
	torchResultBuf = torchResultBuf[:0]
	if len(torchCandidateBuf) == 0 {
		return torchResultBuf
	}
	// Only sort when there are more braziers than slots — most
	// dungeons have a handful, so the common path skips the sort.
	if len(torchCandidateBuf) > maxTorches {
		sort.Slice(torchCandidateBuf, func(a, b int) bool {
			return torchCandidateBuf[a].dist < torchCandidateBuf[b].dist
		})
	}
	n := len(torchCandidateBuf)
	if n > maxTorches {
		n = maxTorches
	}
	t := float32(rl.GetTime())
	for i := 0; i < n; i++ {
		c := torchCandidateBuf[i]
		phase := hashPhase(c.hash)
		// Organic flicker in ~0.72..1.0 from two desynced sines.
		flick := 0.86 +
			0.09*float32(math.Sin(float64(t*9.3+phase))) +
			0.05*float32(math.Sin(float64(t*17.1+phase*1.7)))
		mag := flick * c.bright
		torchResultBuf = append(torchResultBuf, torchLight{
			Pos: c.pos,
			Color: rl.NewVector3(
				torchBaseColor.X*mag,
				torchBaseColor.Y*mag,
				torchBaseColor.Z*mag,
			),
		})
	}
	return torchResultBuf
}

// drawGroundShadow paints a soft radial-gradient disc on the floor —
// the Wind-Waker grounding signature that anchors a tree / bush /
// rock / statue / chest to the ground. The disc is the shared
// groundShadowModel plane (dark centre fading to transparent at the
// rim) scaled to `radius` and laid just above the floor at y=0.02 so
// it composites over the floor texture without z-fighting. `radius`
// is the half-extent in world units (a tile is 1.0 across).
const groundShadowFloorClearance = float32(0.02)

func drawGroundShadow(cx, cz, radius float32) {
	drawGroundShadowAt(cx, groundShadowFloorClearance, cz, radius)
}

// drawGroundShadowAt is drawGroundShadow with an explicit Y, so a contact disc
// can sit on a raised tile's floor (a pack/chest on an elevation plateau)
// instead of the world ground plane.
func drawGroundShadowAt(cx, cy, cz, radius float32) {
	if !groundShadowReady || radius <= 0 {
		return
	}
	rl.DrawModelEx(
		groundShadowModel,
		rl.NewVector3(cx, cy, cz),
		rl.NewVector3(0, 1, 0), 0,
		rl.NewVector3(radius*2, 1, radius*2),
		rl.White,
	)
}

// drawGroundShadowElev draws a contact disc on a tile whose floor sits at
// groundY (its elevation), keeping the same small floor clearance as the
// ground-plane drawGroundShadow. Without this, props/decor/trees on a raised
// tile cast their shadow on the world floor below — a shadow floating in the
// gap under the plateau now that raised tiles draw no support column.
func drawGroundShadowElev(cx, cz, groundY, radius float32) {
	drawGroundShadowAt(cx, groundY+groundShadowFloorClearance, cz, radius)
}

// drawDecorBush / drawDecorMushroom / drawDecorPebble are the
// inline-decor implementations registered in inlineDecorHandlers.
// Each one is a thin wrapper around the dedicated propModel field /
// scatter helper on Resources so the dispatch signature stays uniform
// across every handler. groundY is the tile's elevation floor height so
// decoration rides a raised tile instead of sinking to the world floor.
func drawDecorBush(assets Resources, x, z int, cx, cz, groundY float32) {
	drawGroundShadowElev(cx, cz, groundY, 0.36)
	assets.bushProp.draw(rl.NewVector3(cx, groundY, cz), 0.75, propYawDeg(x, z))
}

func drawDecorMushroom(assets Resources, x, z int, cx, cz, groundY float32) {
	drawGroundShadowElev(cx, cz, groundY, 0.20)
	assets.mushroomProp.draw(rl.NewVector3(cx, groundY, cz), 1.0, propYawDeg(x, z))
}

func drawDecorPebble(assets Resources, x, z int, cx, cz, groundY float32) {
	drawPebbleCluster(assets, cx, cz, groundY, tileHash(x, z))
}

// faceBackfaceCulled reports whether a vertical tile face — centered at the
// edge (cx+fdx*half, cz+fdz*half) with outward normal (fdx,fdz) — faces away
// from the camera. A vertical cliff/voxel face is only visible from its outward
// side, so the caller can skip issuing the DrawModelEx when the camera sits
// behind the face's plane. Shared by drawCliffFaces and the voxel side-face
// pass so the cull test can't drift between them.
func faceBackfaceCulled(camPos rl.Vector3, cx, cz, fdx, fdz, half float32) bool {
	return (camPos.X-(cx+fdx*half))*fdx+(camPos.Z-(cz+fdz*half))*fdz <= 0
}

// drawTileCube draws a square-footprint cube model at (cx,cy,cz) with a yaw
// rotation around its vertical axis. Used for floor and wall tiles so each
// instance can spin its texture by 90° steps without changing the cube's
// silhouette (the x and z extents are equal). Breaks up obvious tiling
// patterns in the texture without needing per-tile mesh variants.
func drawTileCube(model rl.Model, cx, cy, cz, yawDeg float32) {
	rl.DrawModelEx(model,
		rl.NewVector3(cx, cy, cz),
		rl.NewVector3(0, 1, 0),
		yawDeg,
		rl.NewVector3(1, 1, 1),
		rl.White)
}

// tileHash is the per-tile uint32 mixer used by orientation/variant
// selectors. Stable for a given (x,z) so the same tile always reads the
// same way between frames. Stronger avalanche than hashXY — three rounds
// of xorshift+multiply with widely-spaced primes — for cases where
// neighboring tiles need to feel independent.
func tileHash(x, z int) uint32 {
	h := uint32(x*374761393) ^ uint32(z*668265263)
	h ^= h >> 16
	h *= 2246822519
	h ^= h >> 13
	h *= 3266489917
	h ^= h >> 16
	return h
}

// hashXY is the cheaper per-tile hash used where tileHash's stronger
// avalanche is overkill — texture-gen pixel jitter, region-bucketed
// variant picks, etc. Same shape across all callers (textures.go,
// floorVariantHash, drawFloorDecoration) so they sample one mixer.
func hashXY(x, y int) uint32 {
	return mix32(uint32(x*73856093) ^ uint32(y*19349663))
}

// mix32 finalizes a uint32 into a well-distributed bit pattern with one
// round of xorshift + odd-prime multiply. Sufficient for visual variation
// at our texture/tile scales; tileHash uses three rounds when stronger
// avalanche is needed.
func mix32(n uint32) uint32 {
	n ^= n >> 13
	n *= 1274126177
	n ^= n >> 16
	return n
}

// hash01 maps an index to a stable pseudo-random float in [0, 1) by
// finalizing it through mix32 and normalizing the low 24 bits. The
// single-uint sibling of textures.go's hashFloat (which normalizes the
// two-int hashXY) — used where a per-item deterministic [0,1) is wanted
// without a particle pool (the rain streaks' per-streak traits).
func hash01(n uint32) float32 {
	return float32(mix32(n)&0xffffff) / float32(0x1000000)
}

// tileYawDeg returns 0/90/180/270 for floor and wall tiles. Square-footprint
// cubes look identical at any of those rotations; what changes is the
// texture, which kills the visible tiling pattern from same-orientation
// repeats.
func tileYawDeg(x, z int) float32 {
	return float32(tileHash(x, z)&0x03) * 90
}

// propYawDeg returns a per-tile yaw in 30° steps, in [0, 360). Stepped
// rather than fully continuous so each prop reads as having a deliberate
// facing instead of looking like jittered noise.
func propYawDeg(x, z int) float32 {
	return float32(((tileHash(x, z) >> 3) % 12) * 30)
}

// floorVariantHash picks 0 (grass) / 1 (dirt) / 2 (dark grass) for a given
// tile. Uses a region-bucketed hash (every 3 tiles snap to the same bucket)
// so variants form patches rather than per-tile speckle. Two independent
// byte samples drive dirt vs dark-grass selection so they don't perfectly
// mask each other.
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

// scatterOffsetDivisor maps a signed int8 hash byte (-128..127) into a sub-tile
// offset of roughly [-0.55, 0.55] world units. Shared by the floor-decoration
// and pebble-cluster scatterers so both jitter props by the same spread.
const scatterOffsetDivisor = float32(230)

// drawFloorDecoration scatters small props (rocks, bushes, mushrooms) on
// plain floor tiles using a deterministic per-tile hash. ~16% of plain floor
// tiles get a decoration; small rocks are weighted heavier than the others
// so the field reads as pebble-strewn ground. Props are passable (don't
// update BlockedAt) and small rocks are squashed in Y so they look walkable.
func drawFloorDecoration(assets Resources, x, z int, cx, cz, groundY float32) {
	h := hashXY(x, z)
	chance := byte(h)
	if chance > 42 { // ~16.5% rate
		return
	}
	// Weighted kind dispatch: 4/8 small rocks (low-profile pebbles), 1/8 small
	// bush, 2/8 mushrooms (split tiny / small), 1/8 small bush variant. Rocks
	// dominate so the floor looks pebble-strewn rather than mushroom-spotted.
	kind := int((h >> 8) & 7)
	// Sub-tile offset in [-0.55, 0.55] so the prop doesn't always sit dead-
	// center. int8 conversion gives signed -128..127, scaled to ~tile.
	offX := float32(int8(h>>16)) / scatterOffsetDivisor
	offZ := float32(int8(h>>24)) / scatterOffsetDivisor
	pos := rl.NewVector3(cx+offX, groundY, cz+offZ)

	// Reuse the orientation hash so floor decorations also pick up a stable
	// yaw — keeps clusters of small props from looking aligned when a few
	// land in the same neighborhood.
	decoYaw := float32(((h >> 12) % 12) * 30)
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

// drawPebbleCluster paints a small grouping of low-profile pebbles distributed
// across the given tile. Each pebble is just the boulder mesh's base cube
// drawn with a scattered position, slight size jitter, random yaw, and a
// lighter "weathered surface stone" tint than the chunky-boulder palette so
// the cluster reads as ground detail rather than dropped boulders. A
// per-pebble hash gives every member its own footprint/height/rotation.
// pebblePaletteTints is the light "weathered surface stone" palette for
// ground pebble scatter, indexed by per-pebble hash. Package-level so the
// four colors aren't reconstructed on every drawPebbleCluster call.
var pebblePaletteTints = [4]rl.Color{
	rl.NewColor(228, 224, 214, 255),
	rl.NewColor(216, 212, 202, 255),
	rl.NewColor(232, 226, 214, 255),
	rl.NewColor(220, 216, 208, 255),
}

func drawPebbleCluster(assets Resources, cx, cz, groundY float32, tileHash uint32) {
	if len(assets.rockProp.models) == 0 {
		return
	}
	baseModel := assets.rockProp.models[0]
	rotationAxis := rl.NewVector3(0, 1, 0)

	// 2..4 pebbles per cluster — small enough to read as a scatter, not a pile.
	// Sum of two independent hash bits gives a 25% / 50% / 25% distribution
	// for 2 / 3 / 4 — center-weighted so most clusters feel balanced.
	count := 2 + int(tileHash&0x01) + int((tileHash>>1)&0x01)

	for i := 0; i < count; i++ {
		// Salt the tile hash with the pebble index so each member looks
		// independent. Same finalizer as the other render hashes (mix32).
		ih := mix32(tileHash ^ uint32(i+1)*hashSalt)

		// Sub-tile offset in [-0.55, 0.55] — pebbles spread across the tile,
		// not bunched at the center.
		ox := float32(int8(ih)) / scatterOffsetDivisor
		oz := float32(int8(ih>>8)) / scatterOffsetDivisor

		// Footprint and height vary independently. Heights are ~1/3 of
		// footprint so the pebbles sit flat — see drawFloorDecoration's
		// original comment about reading as walkable.
		foot := 0.18 + float32((ih>>16)&0x07)*0.012   // 0.18 .. 0.27
		hght := 0.07 + float32((ih>>20)&0x03)*0.012   // 0.07 .. 0.106
		rot := float32((ih>>24)&0xff) * (360.0 / 256) // 0..360°
		// Slight x/z asymmetry so each pebble's silhouette breaks alignment
		// with its neighbors. Sourcing from a different hash bit keeps the
		// asymmetry uncorrelated to size.
		stretch := 0.85 + float32((ih>>4)&0x07)*0.04 // 0.85 .. 1.13

		// Y placement: the underlying cube is RockMeshBaseHeight tall and
		// propModel's base part offsets it half its height to clear the
		// ground. We draw the mesh directly, not via the prop, so we
		// replicate that math: RockMeshBaseHalfHeight * hght.
		pos := rl.NewVector3(cx+ox, groundY+RockMeshBaseHalfHeight*hght, cz+oz)
		scale := rl.NewVector3(foot, hght, foot*stretch)
		tint := pebblePaletteTints[(ih>>28)&0x03]
		rl.DrawModelEx(baseModel, pos, rotationAxis, rot, scale, tint)
	}
}

func DrawEnemies(camera rl.Camera3D, g *core.GameState, assets Resources) {
	if g.Battle.Phase == core.BattleNone {
		// Debug enemies-off hides field packs entirely. A battle in
		// progress still draws (you'd only toggle mid-explore), but on
		// the field the packs vanish so the map can be walked clean.
		if g.EnemiesDisabled {
			return
		}
		drawFieldPacks(camera, g, assets)
		return
	}
	drawBattlePack(camera, g, assets)
}

// enemyBillboardY is the y-anchor for every enemy/pack billboard — an
// empirically-tuned footing height (matches the billboard's half-height,
// partyBillboardSize.Y below) so the sprite's bottom edge meets the floor
// rather than floating or sinking. Named so the four call sites that used
// to inline 0.68 can't drift.
const enemyBillboardY = float32(0.68)

// battleFormationCenterY is the vertical center enemy billboards render at
// during battle — lifted above the walking-field enemyBillboardY so the
// roster sits screen-centered under the narrow battle FOV (see the comment
// in enemyDrawPosition). enemyFieldLift is the delta between the two; the
// field draw adds it back for yOffset-grounded sprites whose yOffset was
// calibrated against this battle center, so they plant on the floor in the
// field instead of sinking by the lift amount.
const battleFormationCenterY = float32(1.0)
const enemyFieldLift = battleFormationCenterY - enemyBillboardY

// Party billboard sizes. partyBillboardSize is the idle silhouette; the
// active actor bumps up to partyBillboardSizeActive for a soft "your
// turn" emphasis. Named so the size and the active-state highlight
// stay tunable in one place instead of grepping across world.go,
// timing.go (which reads partyBillboardSize indirectly through
// partySpritePosition's y-anchor), and any future minimap badge.
var (
	partyBillboardSize       = rl.NewVector2(0.38, enemyBillboardY)
	partyBillboardSizeActive = rl.NewVector2(0.42, 0.72)
	// partyActiveScale is the per-axis bump the active member's billboard gets,
	// expressed as a ratio of the active size to the idle size. DrawPartySprites
	// multiplies the member's (possibly author-overridden) base size by this so
	// the "your turn" emphasis scales the tuned sprite rather than snapping to a
	// fixed constant — and reproduces partyBillboardSizeActive exactly when the
	// base is the default partyBillboardSize.
	partyActiveScale = rl.NewVector2(partyBillboardSizeActive.X/partyBillboardSize.X, partyBillboardSizeActive.Y/partyBillboardSize.Y)
)

// drawFieldPacks renders one billboard per pack — the highest-tier member,
// at the pack's authored tile. Empty/all-dead packs are skipped (they're
// cleaned up by the battle-win path anyway).
func drawFieldPacks(camera rl.Camera3D, g *core.GameState, assets Resources) {
	// Distance fog for billboards goes through a custom fragment
	// shader — multiplicative tint (the only knob raylib's billboard
	// draw exposes) can darken or color-filter but can't lerp
	// toward the fog color. beginBillboardFogPass uploads the fog
	// uniforms and switches into the shader; the returned func is
	// the matching EndShaderMode.
	defer beginBillboardFogPass(camera, g, assets)()
	for _, pack := range g.Packs {
		if !core.PackAlive(pack) {
			continue
		}
		visual, ok := enemyVisualFor(assets, core.PackLeaderKind(pack))
		if !ok {
			continue
		}
		// Read the interpolated visual coords directly — placePacks
		// seeds them to the tile center, TickPackAnimations eases
		// them mid-step, and engagement/win paths snap them. No
		// fallback to tileWorldPos is needed; pack.X/Z is always
		// authoritative for the field render.
		// StandGroundYAt(pack.Level) keeps a pack on the surface it walks; on a
		// heightfield pack.Level is the column top, so this equals StandGroundY.
		groundY := g.Area.StandGroundYAt(pack.TileX, pack.Level, pack.TileZ)
		position := rl.NewVector3(pack.X, enemyBillboardY+groundY, pack.Z)
		if visual.shadowRadius > 0 {
			sx, sz := shadowFootprint(camera, position, &visual)
			drawGroundShadowAt(sx, groundY+groundShadowFloorClearance, sz, visual.shadowRadius)
		}
		billboardPos := position
		billboardPos.Y += visual.yOffset
		// yOffset is calibrated against the lifted battle formation center;
		// the field anchor sits enemyFieldLift lower, so a grounded sprite
		// would sink by that delta here. Add it back (only matters when
		// yOffset is set — procedural sprites keep yOffset 0 and are
		// unaffected) so the rat plants on the field floor exactly as it
		// does in battle.
		if visual.yOffset != 0 {
			billboardPos.Y += enemyFieldLift
		}
		drawTextureBillboard(camera, visual.texture, billboardPos, visual.size, visual.resolveTint())
	}
}

// billboardPlacement bundles the derived draw positions for a per-kind enemy
// billboard once depthOffset / markerOffset / yOffset are applied. Shared by
// the battle roster draw (drawBattlePack) and the editor Foe Visualizer preview
// (DrawFoePreview) so the placement SEQUENCE both depend on lives in one place.
// (drawFieldPacks uses a different ground-anchored, no-chevron path.)
type billboardPlacement struct {
	base    rl.Vector3 // formation position after the depthOffset push-back
	shadowX float32    // contact-disc footprint (camera-relative shadowOffset folded in)
	shadowZ float32
	chevron rl.Vector3 // target-cursor anchor (markerY/X applied)
	sprite  rl.Vector3 // billboard center (yOffset applied)
}

// resolveBillboardPlacement applies v.depthOffset (push back along camera-
// forward) to position, then derives the contact-shadow footprint, the target-
// chevron anchor (markerY/X), and the sprite center (yOffset) from that
// adjusted base — the exact ordering drawBattlePack and DrawFoePreview share.
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
	// Same fog-shader gate as drawFieldPacks — billboards recede
	// with the world geometry around them.
	defer beginBillboardFogPass(camera, g, assets)()
	members := core.BattleMembers(g)
	for i := range members {
		enemy := &members[i]
		visual, ok := enemyVisualFor(assets, enemy.Kind)
		if !ok {
			continue
		}
		if !enemy.Alive && enemy.DeathFade <= 0 {
			continue
		}
		// Lay the enemy out by its formation row (front 3 / back 4 staggered),
		// resolving its slot among the visible members of that row.
		row, slot, rowCount := enemyRowSlot(members, i)
		position := enemyFormationPos(camera, g, row, slot, rowCount, enemy)
		// Per-kind depth/marker/yOffset placement (depth push-back for the
		// square Feral Rat PNG, the chevron anchor, the contact-shadow
		// footprint, and the lowered sprite center) all derive from one shared
		// helper so battle + the editor Foe Visualizer can't drift on ordering.
		place := resolveBillboardPlacement(camera, position, &visual)
		tint := rl.White
		if !enemy.Alive {
			alpha := uint8(220 * core.Clamp(float64(enemy.DeathFade/core.DeathFadeDuration), 0, 1))
			tint = colorWithAlpha(rl.White, alpha)
		}
		// Yellow target chevron + tint render only while the player is
		// in the enemy-target picker. targetingEnemy gates on
		// Phase==BattlePlayer so the chevron drops the moment the
		// timing bar arms — shared with the roster row's `targetable`
		// flag so both yellow indicators behave identically. The AoE
		// preview (an AoE skill highlighted in the Skill submenu) chevrons
		// EVERY living enemy so the player sees the cast hits the whole
		// line, not one target — same body, broader guard.
		if enemy.Alive && ((targetingEnemy(g) && i == g.Battle.EnemyIndex) || aoeEnemyTargetPreview(g)) {
			tint = tintEnemyTargeted
			drawTargetChevron(camera, place.chevron, visual.effectiveMarkerScale())
		}
		// During BattleEnemyTiming the warm tint on the attacker carries
		// the "this one is swinging" read; the red pyramid moved over to
		// the targeted party member (drawEnemyAttackTargetMarker) so the
		// player sees "they're hitting ME" instead of "they're acting."
		if enemy.Alive && isEnemyAttackerSlot(g, i) {
			tint = tintEnemyAttacker
		}
		if enemy.DamageFlash > 0 {
			tint = core.FlashTint(tint, enemy.DamageFlash)
		}
		// Fold in the per-kind base tint last, so a tinted sprite stays
		// proportionally tinted in every combat state (idle / targeted /
		// attacking / flashing). Untinted kinds resolve to White (no-op).
		tint = tintMul(tint, visual.resolveTint())
		// Soft contact disc under the billboard so the sprite reads as
		// planted rather than floating on the lighting gradient. Drawn
		// before the billboard (ground first, then the upright sprite
		// over it) and only for kinds that opt in via shadowRadius. The
		// disc keeps the default material shader, so the active
		// billboard-fog BeginShaderMode doesn't tint it — same unlit
		// behaviour the prop shadows rely on under the world lighting pass.
		if visual.shadowRadius > 0 {
			drawGroundShadow(place.shadowX, place.shadowZ, visual.shadowRadius)
		}
		// Distance fog is applied by the active billboard-fog shader
		// (BeginShaderMode at the top of this function), not by a
		// CPU tint pass — multiplicative tint can't lerp toward the
		// fog color, only darken or color-filter the texture.
		drawTextureBillboard(camera, visual.texture, place.sprite, visual.size, tint)
	}
}

// aoeEnemyTargetPreview reports whether the player is highlighting an
// all-enemy AoE skill in the Skill submenu — the cue to fan the target
// chevron across every living enemy so the AoE reads before it fires.
// Gated on Phase==BattlePlayer + ActionMode==ActionSkillMenu so it only
// previews during selection, not once the timing bar arms.
// aoePreviewSkillsBuf is the reused scratch slice for the AoE target-preview
// check — refilled via LearnedSkillsInto each frame the skill menu is open
// instead of allocating, mirroring skillMenuSkillsBuf.
var aoePreviewSkillsBuf []core.SkillID

func aoeEnemyTargetPreview(g *core.GameState) bool {
	if g.Battle.Phase != core.BattlePlayer || g.Battle.ActionMode != core.ActionSkillMenu {
		return false
	}
	if g.Battle.CurrentParty < 0 || g.Battle.CurrentParty >= len(g.Party) {
		return false
	}
	aoePreviewSkillsBuf = core.LearnedSkillsInto(&g.Party[g.Battle.CurrentParty], aoePreviewSkillsBuf)
	skills := aoePreviewSkillsBuf
	idx := g.Battle.SkillMenuIndex
	if idx < 0 || idx >= len(skills) {
		return false
	}
	return core.SkillTargetsAllEnemies(skills[idx])
}

// isEnemyAttackerSlot reports whether the given active-pack member slot
// is the one currently lunging at the party (during BattleEnemyTiming).
func isEnemyAttackerSlot(g *core.GameState, slot int) bool {
	if g.Battle.Phase != core.BattleEnemyTiming {
		return false
	}
	return g.Battle.EnemyAttacker == slot
}

// enemyAttackTarget returns the party-member slot the currently lunging
// enemy will hit when the defend bar resolves, plus ok=false when no
// marker should show. Drives the red "incoming hit" marker above the
// threatened head during BattleEnemyTiming. Every current enemy action
// (plain melee, Firebolt, Sleep, Ingest) is single-target and shares
// core.PeekNextEnemyTarget — the same non-mutating peek the battle side
// commits via pickEnemyAttackTarget — so the marker can't drift from who
// actually gets hit. Returns a scalar (not a per-frame []int) to stay
// allocation-free on the draw path; a future AoE enemy skill would change
// this to a set + the caller's single `==` check back to a membership
// test.
func enemyAttackTarget(g *core.GameState) (int, bool) {
	if g.Battle.Phase != core.BattleEnemyTiming {
		return -1, false
	}
	target := core.PeekNextEnemyTarget(g)
	if target < 0 {
		return -1, false
	}
	return target, true
}

// markerStyle bundles every parameter that distinguishes one selector
// pyramid from another: where to anchor its tip relative to the unit's
// billboard, its silhouette (height + base radius), tint, and rotation
// phase offset (so two markers on screen at once don't lock-step). One
// row per gameplay role keeps the three call sites visually consistent
// — change "the enemy attacker marker is too tall" in one place.
type markerStyle struct {
	tipYOffset float32
	height     float32
	baseRadius float32
	color      rl.Color
	phase      float32
}

// Marker sizes resized for the narrower battle FOV (72° vs 112°
// exploration). At the wider FOV the pyramids needed to be a real
// silhouette to read at a distance; the battle FOV magnifies them
// ~1.5× so the old sizes overpowered the sprite. New sizes target
// roughly 25% the height of a party/enemy billboard — readable as
// a cursor, not a billboard accessory.
var (
	// markerEnemyTarget is the player's currently-selected enemy.
	// Yellow — paired with the in-roster row highlight via
	// targetingEnemy().
	markerEnemyTarget = markerStyle{
		// Sits lower (nearer the enemy's head, not floating high above)
		// and a touch bigger + fully opaque so the current target reads
		// at a glance — but kept modest so it doesn't overpower the
		// sprite. Per-kind nudges (markerYOffset / markerXOffset on
		// enemyVisual) fine-tune where it lands over each enemy.
		tipYOffset: 0.56,
		height:     0.20,
		baseRadius: 0.085,
		color:      selectorEnemyTargetColor,
		phase:      0.0,
	}
	// markerFriendlyTarget is the player's currently-selected ally
	// (heal / item targeting). Green, slightly smaller than the
	// enemy markers since party billboards sit closer to the camera.
	markerFriendlyTarget = markerStyle{
		tipYOffset: smallMarkerTipYOffset,
		height:     smallMarkerHeight,
		baseRadius: smallMarkerBaseRadius,
		color:      selectorFriendlyTargetColor,
		phase:      0.3,
	}
	// markerEnemyAttackTarget tags the party member(s) the lunging enemy
	// is about to hit — drawn above the threatened head while the defend
	// bar is up. Shares the small-marker dims with markerFriendlyTarget so
	// the two indicators read as visually paired even when the colors differ.
	markerEnemyAttackTarget = markerStyle{
		tipYOffset: smallMarkerTipYOffset,
		height:     smallMarkerHeight,
		baseRadius: smallMarkerBaseRadius,
		color:      selectorEnemyAttackColor,
		phase:      0.9,
	}
)

// Shared silhouette for the two party-side selector pyramids (friendly target
// + enemy-attack target). They sit closer to the camera than the enemy-target
// marker, so they're a touch smaller; pinning the dims here keeps the pair from
// drifting when one is tuned.
const (
	smallMarkerTipYOffset = float32(0.36)
	smallMarkerHeight     = float32(0.13)
	smallMarkerBaseRadius = float32(0.055)
)

// drawMarkerOnTop draws a selector pyramid on a depth-disabled "overlay"
// layer so it ALWAYS renders above world geometry and billboards — it can
// never clip into the unit it hovers over or scenery between it and the
// camera. rlgl batches draws, so the depth-state toggle only lands cleanly
// when the active batch is flushed before AND after (DrawRenderBatchActive);
// depth WRITES are disabled too so the marker can't occlude billboards drawn
// after it in the same pass. State is restored so later draws are unaffected.
func drawMarkerOnTop(unitPos rl.Vector3, style markerStyle) {
	drawDepthIndependent(func() { drawMarker(unitPos, style) })
}

// drawDepthIndependent runs draw with depth test AND depth writes disabled, so
// whatever it paints renders above all world geometry and can't occlude later
// draws in the same pass. rlgl batches, so the active batch is flushed before
// AND after the toggle for it to land cleanly; state is restored afterward.
// Shared by the selector pyramid (drawMarkerOnTop) and the visualizer anchor
// gizmos (drawAnchorGizmo).
func drawDepthIndependent(draw func()) {
	rl.DrawRenderBatchActive() // flush prior depth-tested geometry
	rl.DisableDepthTest()
	rl.DisableDepthMask()
	draw()
	rl.DrawRenderBatchActive() // flush the overlay draw with depth off
	rl.EnableDepthMask()
	rl.EnableDepthTest()
}

// drawMarker is the single entry point for every selector-pyramid call
// site. `unitPos` is the unit's billboard center; the helper anchors
// the pyramid tip according to the style's tipYOffset and forwards the
// rest to drawSelectorPyramid.
func drawMarker(unitPos rl.Vector3, style markerStyle) {
	tip := rl.NewVector3(unitPos.X, unitPos.Y+style.tipYOffset, unitPos.Z)
	drawSelectorPyramid(tip, style.height, style.baseRadius, style.color, style.phase)
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

// visualAt indexes a dense kind/class→visual slice (enemyVisuals / partyVisuals)
// with a bounds guard, returning (zero, false) for an out-of-range index —
// the slice-backed replacement for the old map comma-ok read, so the lookup is
// an array index, not a hash. Shared by enemyVisualFor / partyVisualFor and the
// editor's live-preview read/write sites.
func visualAt(s []enemyVisual, idx int) (enemyVisual, bool) {
	if idx < 0 || idx >= len(s) {
		return enemyVisual{}, false
	}
	return s[idx], true
}

// drawTargetChevron draws the yellow enemy-target selector pyramid at position,
// scaled by the struck kind's per-kind markerScale (1 = default size). Folding
// the scale into a local copy of the shared markerEnemyTarget style keeps the
// style table itself per-role (not per-kind) while letting a big foe wear a
// bigger cursor — only this enemy-side marker is kind-scaled; the friendly /
// incoming-attack markers keep their fixed role sizes.
func drawTargetChevron(camera rl.Camera3D, position rl.Vector3, scale float32) {
	drawScaledMarker(position, markerEnemyTarget, scale)
}

// drawScaledMarker draws a marker-style cursor at position, optionally scaling a
// copy of baseStyle's height/baseRadius by scale (scale 0 or 1 leaves it at the
// role's default size). The shared body of drawTargetChevron and
// drawFriendlyTargetMarker, which differ only by which markerStyle they pass.
func drawScaledMarker(position rl.Vector3, baseStyle markerStyle, scale float32) {
	style := baseStyle
	if scale > 0 && scale != 1 {
		style.height *= scale
		style.baseRadius *= scale
	}
	drawMarkerOnTop(position, style)
}

// drawSelectorPyramid renders the JRPG-classic floating cursor: a square-
// base pyramid hanging tip-down over the unit, slowly spinning around the
// vertical axis through its tip. The tip is anchored at `tip`; the base
// (4 corners forming a square) sits `height` above the tip, `baseRadius`
// out from the tip's vertical axis. `phase` offsets the rotation so two
// markers on screen at once don't lock-step.
//
// Each face gets its own shade so the pyramid reads as a 3D solid instead
// of a flat silhouette: top cap brightest (it's the "lit from above" face),
// side faces walk a brighter→dimmer→brighter→dimmer pattern around the
// pyramid so the spin gives a "rotating shaded crystal" feel. All faces
// are wound CCW-from-outside so backface culling stays on — without that
// the back faces overdraw the front and z-fight produced flicker.
func drawSelectorPyramid(tip rl.Vector3, height, baseRadius float32, col rl.Color, phase float32) {
	t := rl.GetTime()
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

	// Per-face shading. Sides walk light→mid→dim→mid as you go around the
	// base, and the cap is the brightest face. The pattern rotates with the
	// pyramid (since the corners are recomputed from yaw each frame), so a
	// fixed camera sees a rotating shaded solid rather than a flat blob.
	sideShades := [4]float32{1.05, 0.85, 0.65, 0.85}
	for i := 0; i < 4; i++ {
		j := (i + 1) % 4
		// Side face viewed from outside: tip at bottom, c[i] upper-left,
		// c[i+1] upper-right relative to that view. tip → c[i+1] → c[i]
		// is CCW from outside (front face for OpenGL with +Y up).
		rl.DrawTriangle3D(tipP, corners[j], corners[i], shadeColor(col, sideShades[i]))
	}
	// Top cap (square base, normal +Y). Corners go CCW around +Y when
	// listed 0,1,2,3 (cos/sin sweep), so 0→1→2 and 0→2→3 are CCW from
	// above — front faces.
	capCol := shadeColor(col, 1.18)
	rl.DrawTriangle3D(corners[0], corners[1], corners[2], capCol)
	rl.DrawTriangle3D(corners[0], corners[2], corners[3], capCol)
}

// shadeColor multiplies a color's RGB by factor (clamped 0..255) while
// preserving alpha. Used to derive shaded variants of a base tint without
// authoring a new color per face.
func shadeColor(c rl.Color, factor float32) rl.Color {
	return mapRGB(c, func(v uint8) uint8 {
		return core.ClampByte(int(float32(v) * factor))
	})
}

// ShadeColor is the exported form of shadeColor for the editor's 3D view, which
// can't reach the unexported helper. Multiplies RGB by factor (clamped),
// preserving alpha — the one shading primitive for the render + editor surface.
func ShadeColor(c rl.Color, factor float32) rl.Color { return shadeColor(c, factor) }

func DrawPartySprites(camera rl.Camera3D, g *core.GameState, assets Resources) {
	if g.Battle.Phase == core.BattleNone {
		return
	}
	// Same fog-shader gate as drawBattlePack so the party
	// billboards receive the same distance fog the enemies they
	// face do.
	defer beginBillboardFogPass(camera, g, assets)()
	victoryDance := victoryDanceElapsed(g)
	incomingSlot, hasIncoming := enemyAttackTarget(g)
	for i := range g.Party {
		// Ingested members are tucked away inside a mantrap — don't
		// draw their billboard on the field. The status badge on the
		// party card is the player's "where did they go?" signal.
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
		// Per-class depth / yOffset / marker / shadow placement all derive from
		// the same shared helper the foe billboards and the Party Visualizer
		// preview use, so authored alignment can't drift between battle and editor.
		place := resolveBillboardPlacement(camera, position, &visual)
		size := visual.size
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
		// Fold in the per-class authored base tint last (untinted classes resolve
		// to White = no-op), so a tinted member stays proportionally tinted in
		// every combat state — symmetric with the foe-side tintMul.
		tint = tintMul(tint, visual.resolveTint())
		// Optional authored contact disc, drawn before the billboard like the
		// foe side. Default party visual carries shadowRadius 0 (no disc), so the
		// existing look is unchanged until an author opts one in.
		if visual.shadowRadius > 0 {
			drawGroundShadow(place.shadowX, place.shadowZ, visual.shadowRadius)
		}
		// Distance fog is applied by the active billboard-fog shader
		// (BeginShaderMode at the top of this function). The "your turn"
		// read lives in the party card now — lifted + brightened, others
		// dimmed (see DrawPartyRibbon). The in-world additive glow and the
		// floating pyramid were removed: they read as noise over the
		// sprite. The active fighter still gets the subtle warm tint + size
		// bump applied above so it reads in 3D too.
		drawTextureBillboard(camera, visual.texture, place.sprite, size, tint)
		// Same gate as the enemy chevron: target marker only during the
		// menu phase, NOT during the timing bar that follows the
		// confirm. inPlayerTurn includes BattleAttackTiming and would
		// linger the marker through the press. Markers anchor to the authored
		// chevron position (markerY/XOffset folded in by resolveBillboardPlacement).
		if g.Battle.Phase == core.BattlePlayer && targetingAlly(g) && i == g.Battle.PartyTarget && g.Party[i].HP > 0 {
			drawFriendlyTargetMarker(camera, place.chevron, visual.effectiveMarkerScale())
		}
		// Red "incoming hit" marker above the party member the lunging
		// enemy is about to strike. Phase gating lives in
		// enemyAttackTarget — it returns ok=false outside BattleEnemyTiming.
		if g.Party[i].HP > 0 && hasIncoming && i == incomingSlot {
			drawEnemyAttackTargetMarker(camera, place.chevron)
		}
	}
}

// partyVisualFor returns the billboard visual for a class, false when the class
// has no usable texture (mirrors enemyVisualFor's guard). The party table is
// fully populated at load (every class gets at least the procedural fallback),
// so the false branch is defensive.
func partyVisualFor(assets Resources, class core.PartyClass) (enemyVisual, bool) {
	if v, ok := visualAt(assets.partyVisuals, int(class)); ok && v.texture.ID != 0 {
		return v, true
	}
	return enemyVisual{}, false
}

// drawFriendlyTargetMarker draws the ally-target selector pyramid at position,
// scaled by the member's per-class markerScale (1 = default size) — the
// friendly twin of drawTargetChevron, so the Party Visualizer's "Cursor Sz"
// slider is honored in battle as well as the preview instead of being a dead
// knob. Folds the scale into a local copy of the shared style (kept per-role).
func drawFriendlyTargetMarker(camera rl.Camera3D, position rl.Vector3, scale float32) {
	drawScaledMarker(position, markerFriendlyTarget, scale)
}

func drawEnemyAttackTargetMarker(camera rl.Camera3D, position rl.Vector3) {
	drawMarkerOnTop(position, markerEnemyAttackTarget)
}

// partyRowSlot resolves party member `index`'s formation placement: its row
// (front/back), its left-to-right slot WITHIN that row, and how many members
// share the row — so the billboard layout reflects the front/back formation.
func partyRowSlot(party []core.PartyMember, index int) (row core.Row, slot, count int) {
	if index < 0 || index >= len(party) {
		return core.RowFront, 0, 1
	}
	row = party[index].Row
	for j := range party {
		if party[j].Row == row {
			if j == index {
				slot = count
			}
			count++
		}
	}
	if count == 0 {
		count = 1
	}
	return row, slot, count
}

func partySpritePosition(camera rl.Camera3D, party []core.PartyMember, index int, bump, victoryDance float32, knockback float32) rl.Vector3 {
	forward := horizontalForward(camera)
	right := horizontalRight(forward)
	class := core.PartyClass(0)
	if index >= 0 && index < len(party) {
		class = party[index].Class
	}
	row, slot, count := partyRowSlot(party, index)
	// Two-rank formation, viewed from behind/above. Both rows sit well back off
	// the foes (low rowForward) and low in frame (baseY). The FRONT row is a touch
	// nearer the foes and packs TIGHTER; the BACK row sits nearer the camera,
	// lifted slightly to peek over the front, and spreads WIDE — a trapezoid that
	// widens toward the viewer:
	//       x  x      (front, tight)
	//     x      x    (back, wide)
	baseY := float32(0.42)      // sit low in frame (was riding too high)
	rowForward := float32(1.5)  // front rank — off the foes
	rowSpacing := float32(0.95) // front: spread out (was cramped)
	rowLift := float32(0)
	if row == core.RowBack {
		// BIG depth gap from the front (0.7 vs 1.5) so the two rows clearly STACK
		// under the pitched camera instead of reading as one inline x x x x — the
		// back rank is much nearer the lens, projecting lower + larger. Its wide
		// spacing puts the front pair between the back pair (staggered trapezoid).
		rowForward = 0.7
		rowSpacing = 2.6
	}
	base := rl.NewVector3(
		camera.Position.X+forward.X*rowForward,
		baseY+rowLift,
		camera.Position.Z+forward.Z*rowForward,
	)
	// Centre the row's members left-to-right around the formation axis.
	offset := (float32(slot) - float32(count-1)/2) * rowSpacing
	depth := float32(0)
	danceSide, danceDepth, danceHeight, _ := victoryDanceMotion(class, victoryDance)
	bumpDepth := core.BumpOffset(bump, 0.22)
	// Reactionary knockback: push the member TOWARD the camera (i.e.
	// SUBTRACT forward) when they just took a hit. AttackBump adds
	// forward (lunge into the enemy); HitKnockback subtracts forward
	// (recoil away from the enemy). The two timers shouldn't overlap
	// in practice — a member is either swinging or being swung at.
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

// enemyDrawPosition returns the 3D position to render the given member of
// the active pack at, resolving its visible slot from g. This is the
// single-shot path used by popup / VFX anchors; the per-frame battle draw
// loop (drawBattlePack) precomputes the slot mapping once and calls
// enemyFormationPos directly so it doesn't re-walk the pack per enemy
// (which made the loop O(n²)). Takes g by pointer so the per-enemy call
// doesn't copy the whole GameState.
func enemyDrawPosition(camera rl.Camera3D, g *core.GameState, slot int, enemy *core.Enemy) rl.Vector3 {
	if g.Battle.Phase == core.BattleNone || g.Battle.ActivePack < 0 {
		// Fallback for any stray caller during a phase transition; use the
		// active pack's tile if we still know it.
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

// enemyFormationPos is the formation-placement math for one enemy given its
// already-resolved (visibleSlot, count). Split out of enemyDrawPosition so the
// per-frame battle loop can compute the slot mapping ONCE (a single O(n) pass)
// and call this per member, instead of each call re-walking the pack to find
// its slot — turning a per-frame O(n²) into O(n).
// enemyRowSlot resolves enemy `index`'s formation placement among the VISIBLE
// (alive or death-fading) pack members: its row, left-to-right slot within that
// row, and the row's visible count. The foe-side mirror of partyRowSlot.
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

func enemyFormationPos(camera rl.Camera3D, g *core.GameState, row core.Row, slot, count int, enemy *core.Enemy) rl.Vector3 {
	if count <= 0 {
		// Defensive: re-check the ActivePack bound before indexing so a malformed
		// (Phase!=None, ActivePack out of range) state can't panic.
		if g.Battle.ActivePack >= len(g.Packs) {
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
	center := rl.NewVector3(
		camera.Position.X+forward.X*2.55,
		battleFormationCenterY,
		camera.Position.Z+forward.Z*2.55,
	)
	// Per-row width cap (the front holds 3, the back 4): pack the row's slots
	// inside formationMaxWidth so a full back row doesn't spill across the arena,
	// keeping the generous spacing for small rows.
	const baseSpacing = float32(1.12)
	const formationMaxWidth = float32(2.9)
	spacing := baseSpacing
	if count > 1 {
		if fit := formationMaxWidth / float32(count-1); fit < spacing {
			spacing = fit
		}
	}
	offset := (float32(slot) - float32(count-1)/2) * spacing
	// Two ranks: the FRONT row stands nearer the party (pulled toward the camera
	// along forward); the BACK row sits deeper in the arena, lifted so it reads
	// over the front, and staggered half a slot so it peeks between the front
	// fighters (the classic 3-over / 4-under formation).
	rowDepth := float32(-0.45)
	rowLift := float32(0)
	if row == core.RowBack {
		rowDepth = 0.55
		rowLift = 0.28
		offset += spacing * 0.5
	}
	bump := core.BumpOffset(enemy.AttackBump, 0.2)
	// Reactionary knockback pushes AWAY from the camera; AttackBump lunges toward
	// the party — opposite signs so a hit snaps the enemy opposite its lunge.
	knock := core.KnockbackOffset(enemy.HitKnockback, core.HitKnockbackDist)
	return rl.NewVector3(
		center.X+right.X*offset+forward.X*(rowDepth-bump+knock),
		center.Y+rowLift,
		center.Z+right.Z*offset+forward.Z*(rowDepth-bump+knock),
	)
}

// horizForwardCache memoizes the last horizontalForward result. The
// camera is identical across every billboard / marker draw within a
// frame (and usually across still frames), so this turns the ~6
// per-frame Hypot/normalize calls into one — the rest hit the cache via
// a 2-float compare. Single-threaded render path; exact float compare is
// fine because the inputs are recomputed identically each call.
var horizForwardCache struct {
	dx, dz float32
	result rl.Vector3
	primed bool
}

// horizontalRight is the camera's right vector projected onto the XZ plane —
// perpendicular to horizontalForward. Billboard formation layout and the
// per-kind marker nudge both derive screen-right from this, so the
// (-fwd.Z, 0, fwd.X) expression lives in exactly one place.
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


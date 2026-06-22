package render

import (
	"fmt"
	"math"
	"reflect"
	"sort"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

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

// exploreFOV is the wide walking FOV; 112° favors situational awareness over
// edge perspective distortion.
const exploreFOV = float32(112)

// battleFOV is the narrower combat FOV: enemies fill more pixels and the
// formation packs into a focused stage. Tuned so a six-enemy pack still fits.
const battleFOV = float32(72)

// fovTweenRate eases the camera between explore/battle FOV (deg/sec). 80°/s
// lands the 40° swing in ~half a second, slightly ahead of BattleSplashDuration.
const fovTweenRate = float32(80)

// currentFOV is the eased FOV; package-local so the tween survives across draws
// without leaking onto GameState.
var currentFOV = exploreFOV

// targetFOV returns the FOV to tween toward this frame.
func targetFOV(g *core.GameState) float32 {
	if g.Battle.Active() {
		return battleFOV
	}
	return exploreFOV
}

// battlePitchOffset tilts the camera down in battle (added to LookPitch) so the
// arena floor fills more of the lower screen; -0.18 rad ≈ -10°, small enough that
// enemy sprites stay visible.
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
	position := rl.NewVector3(p.X, core.EyeHeight+groundY, p.Z)
	// Combat screen shake: positional jitter eased out by ShakeTimer. Wall-clock-
	// driven (two incommensurate freqs) so it's visible even while hit-stop freezes
	// the sim. Battle-only.
	if g.Battle.Active() && g.Battle.ShakeTimer > 0 && g.Battle.ShakeDur > 0 {
		amp := g.Battle.ShakePeak * core.Clamp(g.Battle.ShakeTimer/g.Battle.ShakeDur, 0, 1)
		t := rl.GetTime()
		position.X += float32(math.Sin(t*47.0)) * amp
		position.Y += float32(math.Sin(t*61.0)) * amp
	}
	// Push currentFOV toward target by ≤ fovTweenRate*dt; Approach won't overshoot.
	currentFOV = core.Approach(currentFOV, targetFOV(g), fovTweenRate*rl.GetFrameTime())
	return rl.NewCamera3D(
		position,
		rl.NewVector3(position.X+direction.X, position.Y+direction.Y, position.Z+direction.Z),
		rl.NewVector3(0, 1, 0),
		currentFOV,
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
	// Star layer rides the same source/dest. Alpha = StarAlpha * per-pixel alpha
	// (sparse pinpoints).
	if profile.StarAlpha > 0 {
		alpha := uint8(profile.StarAlpha * 255)
		rl.DrawTexturePro(assets.starTexture, source, dest, rl.NewVector2(0, 0), 0, rl.NewColor(255, 255, 255, alpha))
	}
}

// behindCullSlack is how far behind the camera a tile center can project before
// it's skipped. Generous so the tile underfoot and half-behind tiles stay drawn
// through any rotation. No hard distance cap — fog handles far falloff and pop-in
// would show against its 85%-clamped tail.
const behindCullSlack = float32(-2.5)

// behindCull reports whether p sits far enough behind the camera to skip. camPos
// + forward are hoisted out of per-item loops so this stays a cheap dot. Shared by
// the tile/chest/door draws so they cull consistently.
func behindCull(camPos, forward, p rl.Vector3) bool {
	return behindCullXZ(camPos, forward, p.X, p.Z)
}

// behindCullXZ is behindCull with X/Z as scalars, avoiding a throwaway Vector3 in
// the hottest loop.
func behindCullXZ(camPos, forward rl.Vector3, px, pz float32) bool {
	dx := px - camPos.X
	dz := pz - camPos.Z
	return dx*forward.X+dz*forward.Z < behindCullSlack
}

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
	tanHalf := float32(math.Tan(float64(camera.Fovy)*math.Pi/360)) * aspect * viewCullSlack
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

// DrawWorld draws the full lit environment pass — see drawWorld.
func DrawWorld(camera rl.Camera3D, g *core.GameState, assets Resources) {
	drawWorld(camera, g, assets)
}

// worldFrameClock is rl.GetTime() sampled once at the top of the world render
// (drawWorld, DrawObjectPreview) and read by all per-tile sway/flicker math, so
// hundreds of props don't each make their own GetTime() cgo call. Set before any
// prop draw on every path, so it's never stale.
var worldFrameClock float32

// drawWorld rasterizes the sky-less environment geometry, uploads lighting
// uniforms, and (when the render log is on) gathers per-tile diagnostics.
func drawWorld(camera rl.Camera3D, g *core.GameState, assets Resources) {
	worldFrameClock = float32(rl.GetTime())
	m := &g.Area
	material := assets.worldMaterial(m.Materials)
	profile := applyTimeOfDay(lightingFor(m.Materials), timeProfileAt(g.StepCount), areaIsEnclosed(m))
	cacheLightingProfile(profile)
	assets.lighting.applyUniforms(camera, profile)
	// Torch point lights: collect nearest braziers, flicker, upload. Must run
	// after applyUniforms (same shader) and before the tile loop draws.
	torches := collectTorches(m, camera)
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
			// Elevation: the tile's floor rides up by its level. The world is a
			// heightfield — a "wall" is the rendered vertical FACE of an elevation
			// step (drawCliffFaces), not a solid tile.
			te := grid[z*gw+x]
			elevY := core.ElevationWorldY(te.level)
			if m.CeilingAt(x, z) {
				drawTileCube(material.ceilingModel, cx, core.LevelStep+elevY, cz, tileYawDeg(x, z))
				if logActive {
					stats.CeilingsDrawn++
				}
			}
			if len(m.Solids) > 0 {
				// Voxel path (gapped maps only): floors per standable surface, side
				// faces per solid run, floating-cube undersides.
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
				// Cliff faces for every edge above the neighbour/map edge.
				if n := drawCliffFaces(camPos, material, assets, grid, gw, gh, x, z, cx, cz, te.level, te.ramp); logActive {
					stats.WallsDrawn += n
				}
			}
			// Decor anchors to its placed level (DecorLevelAt). Guard on non-empty
			// so the anchor math is skipped for the common empty tiles.
			if decor := m.Decor[z][x]; decor != core.DecorEmpty {
				decorCenter := rl.NewVector3(cx, m.StandGroundYAt(x, te.decorLevel, z), cz)
				drawDecor(assets, decor, x, z, cx, cz, decorCenter)
				if logActive {
					stats.DecorDrawn++
				}
			}
			if prop := m.Props[z][x]; prop != core.TilePropEmpty {
				propYaw := propYawDeg(x, z)
				// Prop anchors to its placed level (PropLevelAt).
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
	faceSkins [4]byte
	// decorLevel/propLevel are the surfaces decor/props anchor to. Cached because
	// on a VOXEL map an auto-level tile resolves through an O(stackHeight) column
	// rescan that would otherwise run per visible tile per frame.
	decorLevel int
	propLevel  int
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

// fnvOffsetBasis is the FNV-1a 64-bit offset basis the layer hash seeds from.
const fnvOffsetBasis = uint64(1469598103934665603)

// foldLayer folds one layer's bytes into FNV-1a digest h with row + layer
// separators so ragged splits can't collide. Allocation-free.
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

// layersHash folds the given layers into one FNV-1a digest. Allocation-free.
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
	// Hash Floor/Elevation/Walls plus every Solids plane (so a voxel edit
	// invalidates the cache), folded in sequence so the check allocates nothing.
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
			// Resolve each face's skin once (override-or-base) so the cliff pass
			// never re-scans FaceOverrides. Index = direction (N=0/E=1/S=2/W=3).
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

// drawCliffFaces renders the vertical faces of tile (x,z) — one per cardinal
// edge where this tile sits above its neighbour (or the map edge). Ramp tiles
// draw their own wedge and are skipped. Returns the face count (WallsDrawn tally).
func drawCliffFaces(camPos rl.Vector3, material worldMaterialResources, assets Resources, grid []tileElev, w, h, x, z int, cx, cz float32, myLevel, myRamp int) int {
	if myRamp != core.NoRamp {
		return 0 // the ramp wedge supplies its own faces
	}
	const half = float32(core.TileSize) / 2
	drawn := 0
	for _, d := range core.CardinalDirs {
		dx, dz := core.FacingVector(d)
		// CPU backface cull: a face is only visible from its outward side, and a
		// dense heightfield issues one per exposed edge — skipping the DrawModelEx
		// saves the per-call cost.
		fdx, fdz := float32(dx), float32(dz)
		if faceBackfaceCulled(camPos, cx, cz, fdx, fdz, half) {
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
		// Per-direction skin from the prebuilt grid (override or base).
		skin := material.faceModel
		if sc := grid[z*w+x].faceSkins[d]; assets.faceVariantTable.present[sc] {
			skin = assets.faceVariantTable.model[sc]
		}
		drawCliffFace(skin, cx, core.ElevationWorldY(nLevel), cz, faceYaw(d), float32(myLevel-nLevel))
		drawn++
	}
	return drawn
}

// faceYaw maps the dropping-edge direction to the Y-rotation orienting the
// face-quad (built on +Z/south) outward. +Z→(sinθ,cosθ): 0=S, 90=E, 180=N, 270=W.
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
	rl.DrawModelEx(model,
		rl.NewVector3(cx, lowY, cz),
		rl.NewVector3(0, 1, 0), rampFacingYaw(facing),
		rl.NewVector3(1, 1, 1), rl.White)
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
		if dm := &assets.decorModelTable[cell]; len(dm.parts) > 0 {
			dm.draw(footprintAnchor(center, footprint), 1.0, 0)
		}
		return
	}
	if dm := &assets.decorModelTable[cell]; len(dm.parts) > 0 {
		dm.draw(center, 1.0, propYawDeg(x, z))
	}
}

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
	return func(assets Resources, _ *core.AreaDefinition, x, z int, center rl.Vector3, propYaw float32) {
		drawGroundShadowElev(center.X, center.Z, center.Y, foliageShadowRadius(scale, 0.10))
		assets.tree.drawVaried(center, scale, propYaw, tileHash(x, z))
	}
}

// drawPropTreeTwin renders two diagonally-offset trees of different scales in one
// tile ("big tree with a younger one beside it"). Both reuse assets.tree.
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
			rl.NewVector3(0, 1, 0), 0, rl.NewVector3(size, size*1.4, size), tint)
	}
}

// wallTorchFacing returns the unit (x,z) the torch faces — away from the first
// adjacent wall (N→E→S→W), or south when the tile has no adjacent wall.
func wallTorchFacing(m *core.AreaDefinition, x, z int) (float32, float32) {
	if f, ok := core.FacingAwayFromAdjacentWall(*m, x, z); ok {
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
// dims PLUS a ceiling fingerprint, so two same-named/sized areas with different
// roofs can't share a stale verdict (the editor "untitled" case). Used by
// enclosureCache and torchSiteCache.
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

// enclosureCache memoizes the enclosure result so the ceiling scan runs once per
// area, not per frame.
var enclosureCache struct {
	areaKey
	enclosed bool
}

// areaIsEnclosed reports whether the area is a roofed interior (gates the
// dungeon lighting override), memoizing core.AreaIsOutdoor per area.
func areaIsEnclosed(m *core.AreaDefinition) bool {
	if enclosureCache.matches(m) {
		return enclosureCache.enclosed
	}
	enclosed := !core.AreaIsOutdoor(m)
	enclosureCache.set(m)
	enclosureCache.enclosed = enclosed
	return enclosed
}

// Shared iron-fixture + flame palette for every torch/brazier. Iron is lit by the
// world shader; flame tints (hot core → mid → tip) are applied to unlit models so
// they glow. Shared by drawWallTorch and the brazier prop so they don't drift.
var (
	torchIron       = rl.NewColor(54, 50, 46, 255)
	torchIronLight  = rl.NewColor(92, 84, 76, 255)
	torchFlameTints = [3]rl.Color{
		rl.NewColor(255, 226, 150, 255), // hot core — pale gold
		rl.NewColor(252, 162, 70, 255),  // mid — orange
		rl.NewColor(228, 110, 52, 255),  // tip — deep ember
	}
)

// torchFlameModel is the unlit emissive sphere for flame blobs (default shader so
// it glows at full tint against the dark dungeon). Set by NewResources.
var (
	torchFlameModel rl.Model
	torchFlameReady bool
)

// torchFlameHeight is the world Y a brazier's point light sits at — up at the
// fire bowl so the pool radiates outward and down.
const torchFlameHeight = float32(1.05)

// torchBaseColor is the warm flame tint at full brightness, before flicker.
// Deliberately bright (R > 1) so a torch-lit wall reads as a strong pool.
var torchBaseColor = rl.NewVector3(2.3, 1.35, 0.7)

type torchCandidate struct {
	pos    rl.Vector3
	dist   float32
	hash   uint32
	bright float32 // brightness multiplier — braziers > wall torches
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
			// Light origin rides the walkable-surface height so a raised torch
			// lights at its actual flame height.
			elevY := m.StandGroundY(x, z)
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
	torchSiteCache.set(m)
}

// collectTorches returns the maxTorches braziers/torches nearest the camera as
// flickering point lights (the rest are fog-swallowed). Flicker is per-torch
// desynced sines so neighbours don't pulse in lockstep. Empty slice if none.
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
		rl.NewVector3(0, 1, 0), 0,
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
	drawPebbleCluster(assets, cx, cz, groundY, tileHash(x, z))
}

// faceBackfaceCulled reports whether a vertical face (edge center cx+fdx*half,
// cz+fdz*half, outward normal fdx,fdz) faces away from the camera. Shared by
// drawCliffFaces and the voxel side-face pass.
func faceBackfaceCulled(camPos rl.Vector3, cx, cz, fdx, fdz, half float32) bool {
	return (camPos.X-(cx+fdx*half))*fdx+(camPos.Z-(cz+fdz*half))*fdz <= 0
}

// drawTileCube draws a square-footprint cube at (cx,cy,cz) yaw-rotated about its
// vertical axis — 90° steps spin the texture to break tiling without changing the
// silhouette.
func drawTileCube(model rl.Model, cx, cy, cz, yawDeg float32) {
	rl.DrawModelEx(model,
		rl.NewVector3(cx, cy, cz),
		rl.NewVector3(0, 1, 0),
		yawDeg,
		rl.NewVector3(1, 1, 1),
		rl.White)
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

// propYawDeg returns a per-tile yaw in 30° steps, in [0,360) — stepped so each
// prop reads as a deliberate facing rather than noise.
func propYawDeg(x, z int) float32 {
	return float32(((tileHash(x, z) >> 3) % 12) * 30)
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
	offX := float32(int8(h>>16)) / scatterOffsetDivisor
	offZ := float32(int8(h>>24)) / scatterOffsetDivisor
	pos := rl.NewVector3(cx+offX, groundY, cz+offZ)

	// Stable yaw from the same hash so clustered props aren't aligned.
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
	rotationAxis := rl.NewVector3(0, 1, 0)

	// 2..4 per cluster, 25/50/25 center-weighted.
	count := 2 + int(tileHash&0x01) + int((tileHash>>1)&0x01)

	for i := 0; i < count; i++ {
		// Salt with the index so each member looks independent.
		ih := mix32(tileHash ^ uint32(i+1)*hashSalt)

		ox := float32(int8(ih)) / scatterOffsetDivisor
		oz := float32(int8(ih>>8)) / scatterOffsetDivisor

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
		drawTextureBillboard(camera, visual.texture, billboardPos, visual.size, visual.resolveTint())
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
	for i := range members {
		enemy := &members[i]
		visual, ok := enemyVisualFor(assets, enemy.Kind)
		if !ok {
			continue
		}
		if !enemy.Alive && enemy.DeathFade <= 0 {
			continue
		}
		p := placements[i]
		position := enemyFormationPos(camera, g, p.row, p.slot, p.count, enemy)
		// Per-kind depth/marker/yOffset placement via the shared helper.
		place := resolveBillboardPlacement(camera, position, &visual)
		tint := rl.White
		if !enemy.Alive {
			alpha := uint8(220 * core.Clamp(float64(enemy.DeathFade/core.DeathFadeDuration), 0, 1))
			tint = colorWithAlpha(rl.White, alpha)
		}
		// Yellow chevron + tint only in the enemy-target picker (targetingEnemy
		// gates on Phase==BattlePlayer so it drops when the timing bar arms). The
		// AoE preview chevrons EVERY living enemy so the line read is clear.
		if enemy.Alive && ((targetingEnemy(g) && i == g.Battle.EnemyIndex) || aoeEnemyTargetPreview(g)) {
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
		// Distance fog comes from the billboard-fog shader, not a CPU tint.
		drawTextureBillboard(camera, visual.texture, place.sprite, visual.size, tint)
	}
}

// aoeEnemyTargetPreview reports whether an all-enemy AoE skill is highlighted in
// the Skill submenu (the cue to chevron every living enemy). Gated on
// Phase==BattlePlayer + ActionSkillMenu, reading the live SkillMenuList.
func aoeEnemyTargetPreview(g *core.GameState) bool {
	if g.Battle.Phase != core.BattlePlayer || g.Battle.ActionMode != core.ActionSkillMenu {
		return false
	}
	if g.Battle.CurrentParty < 0 || g.Battle.CurrentParty >= len(g.Party) {
		return false
	}
	// Index the SAME list the cursor walks (SkillMenuList) — re-deriving the
	// learned set here would index a different list (DebugAllSkills) and preview
	// the wrong skill.
	skills := g.Battle.SkillMenuList
	idx := g.Battle.SkillMenuIndex
	if idx < 0 || idx >= len(skills) {
		return false
	}
	return core.SkillTargetsAllEnemies(skills[idx])
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
// BattleEnemyTiming. Shares core.PeekNextEnemyTarget with the commit path so the
// marker can't drift from who's actually hit.
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

// drawMarker anchors the pyramid tip at unitPos + style.tipYOffset and forwards
// to drawSelectorPyramid.
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

func DrawPartySprites(camera rl.Camera3D, g *core.GameState, assets Resources) {
	if g.Battle.Phase == core.BattleNone {
		return
	}
	defer beginBillboardFogPass(camera, g, assets)()
	victoryDance := victoryDanceElapsed(g)
	incomingSlot, hasIncoming := enemyAttackTarget(g)
	for i := range g.Party {
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
		drawTextureBillboard(camera, visual.texture, place.sprite, size, tint)
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

func partySpritePosition(camera rl.Camera3D, party []core.PartyMember, index int, bump, victoryDance float32, knockback float32) rl.Vector3 {
	forward := horizontalForward(camera)
	right := horizontalRight(forward)
	class := core.PartyClass(0)
	if index >= 0 && index < len(party) {
		class = party[index].Class
	}
	// Layout uses the STANDING home slot (HomeRow/HomeCol), not the live combat
	// row, so the party always renders as a stable 2×2 trapezoid (Reposition/ambush
	// change reach only, not the sprites).
	visRow, visCol := core.RowFront, core.ColLeft
	if index >= 0 && index < len(party) {
		visRow, visCol = party[index].HomeRow, party[index].HomeCol
	}
	// 2×2 trapezoid widening toward the viewer (mostly via width so both ranks
	// stay on-screen): front tight/further, back wide/nearer.
	baseY := float32(0.58)
	rowForward := float32(1.55) // front rank — off the foes
	rowSpacing := float32(0.95) // front: tight pair
	if visRow == core.RowBack {
		rowForward = 1.12 // nearer the camera, still in frame
		rowSpacing = 1.5  // back: only a touch wider (gentle trapezoid)
	}
	base := rl.NewVector3(
		camera.Position.X+forward.X*rowForward,
		baseY,
		camera.Position.Z+forward.Z*rowForward,
	)
	// Left/right column around the formation axis.
	colSign := float32(-0.5)
	if visCol == core.ColRight {
		colSign = 0.5
	}
	offset := colSign * rowSpacing
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

// enemyPlacement is one member's resolved (row, slot, count) — the same triple
// enemyRowSlot returns.
type enemyPlacement struct {
	row   core.Row
	slot  int
	count int
}

// enemyPlacementsBuf backs enemyRowPlacements (reused, single-threaded path).
var enemyPlacementsBuf []enemyPlacement

// enemyRowPlacements resolves EVERY member's placement in one O(n) pass — the
// batch form of enemyRowSlot, replacing a per-member O(n²) call. The returned
// slice aliases a reused buffer (valid until the next call).
func enemyRowPlacements(members []core.Enemy) []enemyPlacement {
	n := len(members)
	if cap(enemyPlacementsBuf) < n {
		enemyPlacementsBuf = make([]enemyPlacement, n)
	}
	out := enemyPlacementsBuf[:n]
	rowIdx := func(r core.Row) int {
		if r == core.RowBack {
			return 1
		}
		return 0
	}
	var counts [2]int
	for j := range members {
		if members[j].Alive || members[j].DeathFade > 0 {
			counts[rowIdx(members[j].Row)]++
		}
	}
	var running [2]int
	for j := range members {
		ri := rowIdx(members[j].Row)
		cnt := counts[ri]
		if cnt == 0 {
			cnt = 1 // not visible; place solo rather than /0 (matches enemyRowSlot)
		}
		slot := 0
		if members[j].Alive || members[j].DeathFade > 0 {
			slot = running[ri]
			running[ri]++
		}
		out[j] = enemyPlacement{row: members[j].Row, slot: slot, count: cnt}
	}
	return out
}

func enemyFormationPos(camera rl.Camera3D, g *core.GameState, row core.Row, slot, count int, enemy *core.Enemy) rl.Vector3 {
	if count <= 0 {
		// Defensive: re-check the ActivePack bound so a malformed state can't panic.
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
	// Per-row width cap: pack slots inside formationMaxWidth so a full back row
	// doesn't spill, keeping generous spacing for small rows.
	const baseSpacing = float32(1.12)
	const formationMaxWidth = float32(2.9)
	spacing := baseSpacing
	if count > 1 {
		if fit := formationMaxWidth / float32(count-1); fit < spacing {
			spacing = fit
		}
	}
	offset := (float32(slot) - float32(count-1)/2) * spacing
	// Two ranks: front nearer the party; back deeper, lifted to read over the
	// front and staggered half a slot to peek between them.
	rowDepth := float32(-0.45)
	rowLift := float32(0)
	if row == core.RowBack {
		rowDepth = 0.55
		rowLift = 0.28
		offset += spacing * 0.5
	}
	bump := core.BumpOffset(enemy.AttackBump, 0.2)
	// Knockback pushes away from the camera; AttackBump lunges toward the party
	// (opposite signs).
	knock := core.KnockbackOffset(enemy.HitKnockback, core.HitKnockbackDist)
	return rl.NewVector3(
		center.X+right.X*offset+forward.X*(rowDepth-bump+knock),
		center.Y+rowLift,
		center.Z+right.Z*offset+forward.Z*(rowDepth-bump+knock),
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


package render

import (
	"math"
	"sort"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

type enemyVisual struct {
	texture rl.Texture2D
	size    rl.Vector2
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
func targetFOV(g core.GameState) float32 {
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

func Camera(g core.GameState) rl.Camera3D {
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
	position := rl.NewVector3(p.X, core.EyeHeight, p.Z)
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

func DrawSkyBackground(assets Resources, g core.GameState) {
	m := g.Area
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
	_ = m
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

func DrawWorld(camera rl.Camera3D, g core.GameState, assets Resources) {
	m := g.Area
	material := assets.worldMaterial(m.Materials)
	profile := applyTimeOfDay(lightingFor(m.Materials), timeProfileAt(g.StepCount), areaIsEnclosed(m))
	cacheLightingProfile(profile)
	assets.lighting.applyUniforms(camera, profile)
	// Torch point lights — collect the brazier props nearest the
	// camera, flicker them, and upload before the geometry pass so
	// walls / floors / props pick up the warm pools of light. Must
	// run after applyUniforms (same shader) and before the tile
	// loop's BeginShaderMode draws.
	torches := collectTorches(m, camera)
	assets.lighting.uploadTorches(torches)

	camPos := camera.Position
	forward := horizontalForward(camera)

	// Diagnostics: only collect counters when the render log is on,
	// so the hot path stays a plain increment-free loop the rest of
	// the time. logActive is a single function-call check.
	logActive := IsRenderLogActive()
	var stats renderFrameStats
	if logActive {
		stats.MapW = m.Width
		stats.MapH = m.Height
	}

	rl.BeginShaderMode(assets.lighting.shader)
	for z := 0; z < m.Height; z++ {
		for x := 0; x < m.Width; x++ {
			if logActive {
				stats.TilesIterated++
			}
			cx := core.TileCenter(x)
			cz := core.TileCenter(z)
			dx := cx - camPos.X
			dz := cz - camPos.Z
			if dx*forward.X+dz*forward.Z < behindCullSlack {
				if logActive {
					stats.TilesCulled++
				}
				continue
			}
			center := rl.NewVector3(cx, 0, cz)
			if m.CeilingAt(x, z) {
				drawTileCube(material.ceilingModel, cx, core.WallHeight, cz, tileYawDeg(x, z))
				if logActive {
					stats.CeilingsDrawn++
				}
			}
			if m.Walls[z][x] == core.TileRock {
				drawTileCube(material.wallModel, cx, core.WallHeight/2, cz, tileYawDeg(x, z))
				if logActive {
					stats.WallsDrawn++
				}
				continue
			}
			drawFloorTile(material, assets, m.Floor[z][x], x, z, cx, cz)
			if logActive {
				stats.FloorsDrawn++
			}
			drawDecor(assets, m.Decor[z][x], x, z, cx, cz, center)
			if logActive && m.Decor[z][x] != core.DecorEmpty {
				// DecorAuto still counts — the floor scatter is decor.
				stats.DecorDrawn++
			}
			if prop := m.Props[z][x]; prop != core.TilePropEmpty {
				propYaw := propYawDeg(x, z)
				drawn := false
				if handler := inlinePropTable[prop]; handler != nil {
					handler(assets, m, x, z, center, propYaw)
					drawn = true
				} else if footprint := core.PropFootprint(prop); footprint != nil {
					if pm := &assets.propModelTable[prop]; len(pm.parts) > 0 {
						anchor := footprintAnchor(center, footprint)
						if r := propShadowRadius[prop]; r > 0 {
							drawGroundShadow(anchor.X, anchor.Z, r)
						}
						pm.draw(anchor, 1.0, propYaw)
						drawn = true
					}
				} else if pm := &assets.propModelTable[prop]; len(pm.parts) > 0 {
					if r := propShadowRadius[prop]; r > 0 {
						drawGroundShadow(center.X, center.Z, r)
					}
					pm.draw(center, 1.0, propYaw)
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
func drawFloorTile(material worldMaterialResources, assets Resources, cell byte, x, z int, cx, cz float32) {
	yaw := tileYawDeg(x, z)
	if special, ok := assets.specialFloors[cell]; ok {
		drawTileCube(special, cx, -0.03, cz, yaw)
		return
	}
	if !material.hasFloorVariant {
		drawTileCube(material.floorModel, cx, -0.03, cz, yaw)
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
	drawTileCube(model, cx, -0.03, cz, yaw)
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
		drawFloorDecoration(assets, x, z, cx, cz)
		return
	}
	// Inline-handled decor (bush / mushroom / pebble) dispatches
	// through the inlineDecorTable in resources.go — a [256] array
	// mirror of inlineDecorHandlers so the per-tile-per-frame hot path
	// is an array index instead of a map hash.
	if handler := inlineDecorTable[cell]; handler != nil {
		handler(assets, x, z, cx, cz)
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

// drawPropTree / drawPropTreeXL / drawPropRockLarge / drawPropBushLarge
// are the inline-prop implementations registered in
// inlinePropHandlers. Tree and TreeXL share assets.tree at different
// scales; the other two wrap dedicated propModel fields. Pre-resolved
// propYaw is passed in by the caller so all four handlers stay
// uniform.
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

// drawPropTreeScaled draws assets.tree at the scale registered in
// treePropScales for the given char. The inline-prop dispatcher binds
// each per-char closure at init so the prop-renderer call site stays
// "table lookup → invoke" with no branch on char. Per-tile shape
// variance is seeded from tileHash so a stand of identical-char trees
// no longer reads as a stamped grid.
func drawPropTreeScaled(char byte) inlinePropRenderer {
	scale := treePropScales[char]
	return func(assets Resources, _ core.AreaDefinition, x, z int, center rl.Vector3, propYaw float32) {
		// Tree shadow scales with the tree's overall scale, plus
		// a touch of slack so the painted disc sits a little wider
		// than the trunk's projected footprint.
		drawGroundShadow(center.X, center.Z, 0.34*scale+0.10)
		assets.tree.drawVaried(center, scale, propYaw, tileHash(x, z))
	}
}

// drawPropTreeTwin renders two trees stacked into one tile, offset
// diagonally so neither sits in the dead center. The two instances
// use different scales so the silhouette reads as "big tree with a
// younger one beside it" rather than a mirrored pair. Yaw is staggered
// and each gets its own variance seed. Pure visual variant — both
// reuse assets.tree.
func drawPropTreeTwin(assets Resources, _ core.AreaDefinition, x, z int, center rl.Vector3, propYaw float32) {
	const offset = 0.32
	const scaleBig = 0.82
	const scaleSmall = 0.58
	seed := tileHash(x, z)
	left := rl.NewVector3(center.X-offset, center.Y, center.Z-offset)
	right := rl.NewVector3(center.X+offset, center.Y, center.Z+offset)
	drawGroundShadow(left.X, left.Z, 0.34*scaleBig+0.08)
	drawGroundShadow(right.X, right.Z, 0.34*scaleSmall+0.08)
	if seed&1 == 0 {
		assets.tree.drawVaried(left, scaleBig, propYaw, seed)
		assets.tree.drawVaried(right, scaleSmall, propYaw+1.1, seed^0x9E3779B9)
	} else {
		assets.tree.drawVaried(left, scaleSmall, propYaw, seed)
		assets.tree.drawVaried(right, scaleBig, propYaw+1.1, seed^0x9E3779B9)
	}
}

func drawPropRockLarge(assets Resources, _ core.AreaDefinition, _, _ int, center rl.Vector3, propYaw float32) {
	drawGroundShadow(center.X, center.Z, 0.42)
	assets.rockProp.draw(center, 1.0, propYaw)
}

func drawPropBushLarge(assets Resources, _ core.AreaDefinition, _, _ int, center rl.Vector3, propYaw float32) {
	drawGroundShadow(center.X, center.Z, 0.48)
	assets.bushProp.draw(center, 1.3, propYaw)
}

// drawWallTorch is the inline handler for TileTorch. It auto-orients
// the torch to the adjacent wall (facing away from it into the room),
// draws an unlit iron bracket + sconce on the wall, and an animated
// emissive flame made of a few jittering fire-tinted spheres. The
// point light itself is added by collectTorches; this is purely the
// visible fixture + flame. Non-blocking: the floor tile stays clear.
func drawWallTorch(assets Resources, m core.AreaDefinition, x, z int, center rl.Vector3, _ float32) {
	fx, fz := wallTorchFacing(m, x, z)
	// Mount point: against the wall behind the torch, up at sconce
	// height. The torch faces (fx,fz) into the room, so the wall is
	// in the opposite direction.
	const mount = 0.40
	const sconceY = 1.30
	wallX := center.X - fx*mount
	wallZ := center.Z - fz*mount

	// Iron bracket — a small dark cube flush on the wall, plus a
	// short arm reaching out toward the room holding the sconce.
	// Drawn lit (immediate mode under the world shader); the torch's
	// own light pool keeps it visible.
	bracket := rl.NewVector3(wallX, sconceY-0.12, wallZ)
	rl.DrawCube(bracket, 0.10, 0.22, 0.10, torchIron)
	armX := wallX + fx*0.10
	armZ := wallZ + fz*0.10
	rl.DrawCube(rl.NewVector3(armX, sconceY, armZ), 0.08, 0.06, 0.08, torchIron)
	// Sconce cup at the arm tip.
	cupX := wallX + fx*0.16
	cupZ := wallZ + fz*0.16
	rl.DrawCube(rl.NewVector3(cupX, sconceY+0.04, cupZ), 0.16, 0.08, 0.16, torchIronLight)

	// Animated flame — three emissive blobs above the cup, each
	// bobbing on its own time offset so the flame flickers and
	// dances. Drawn via the unlit flame model (default shader) so
	// they glow regardless of the near-black dungeon ambient.
	if !torchFlameReady {
		return
	}
	t := float32(rl.GetTime())
	phase := float32(tileHash(x, z)&0xFFFF) / 65535.0 * 6.2831853
	flameBaseX := cupX
	flameBaseZ := cupZ
	for i := 0; i < 3; i++ {
		fp := phase + float32(i)*2.1
		bob := float32(math.Sin(float64(t*7.0+fp))) * 0.04
		swayA := float32(math.Sin(float64(t*5.3+fp*1.4))) * 0.05
		// Higher blobs are smaller and lean more — a teardrop
		// flame shape that wavers.
		y := sconceY + 0.09 + float32(i)*0.07 + bob
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
func wallTorchFacing(m core.AreaDefinition, x, z int) (float32, float32) {
	if f, ok := core.FacingAwayFromAdjacentWall(m, x, z); ok {
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

// enclosureCache memoizes the last area's enclosure result so the
// ceiling-coverage scan runs once per area, not once per frame.
var enclosureCache struct {
	name     string
	width    int
	height   int
	enclosed bool
	primed   bool
}

// areaIsEnclosed reports whether the area is a roofed interior —
// used to gate the spooky-dungeon lighting override. An outdoor
// area (field, or a forest authored on the dungeon palette) has no
// ceiling layer and reads as open-sky; a real dungeon has ceiling
// slabs over most of its tiles. Threshold is 30 % coverage so a
// dungeon with a few open-air courtyard tiles still counts as
// enclosed, while a forest with zero ceilings never does.
func areaIsEnclosed(m core.AreaDefinition) bool {
	if enclosureCache.primed && enclosureCache.name == m.Name &&
		enclosureCache.width == m.Width && enclosureCache.height == m.Height {
		return enclosureCache.enclosed
	}
	covered, total := 0, 0
	for z := 0; z < m.Height; z++ {
		for x := 0; x < m.Width; x++ {
			if !m.InBounds(x, z) {
				continue
			}
			total++
			if m.CeilingAt(x, z) {
				covered++
			}
		}
	}
	enclosed := total > 0 && float64(covered)/float64(total) > 0.30
	enclosureCache.name = m.Name
	enclosureCache.width = m.Width
	enclosureCache.height = m.Height
	enclosureCache.enclosed = enclosed
	enclosureCache.primed = true
	return enclosed
}

// Wall-torch fixture + flame palette. Iron tones are lit by the
// world shader; the flame tints are applied to the unlit
// torchFlameModel so they glow. Three flame tints (hot core →
// mid → tip) layer the bobbing blobs into a teardrop fire.
var (
	torchIron      = rl.NewColor(54, 50, 46, 255)
	torchIronLight = rl.NewColor(92, 84, 76, 255)
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
func collectTorches(m core.AreaDefinition, camera rl.Camera3D) []torchLight {
	torchCandidateBuf = torchCandidateBuf[:0]
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
			var pos rl.Vector3
			bright := float32(0.85) // wall torch — dimmer
			if isBrazier {
				// Floor brazier: flame at the bowl, brighter pool.
				pos = rl.NewVector3(cx, torchFlameHeight, cz)
				bright = 1.45
			} else {
				// Wall torch: light originates at the sconce, offset
				// toward the wall + up at flame height.
				fx, fz := wallTorchFacing(m, x, z)
				pos = rl.NewVector3(cx-fx*0.30, 1.42, cz-fz*0.30)
			}
			dx := cx - camera.Position.X
			dz := cz - camera.Position.Z
			torchCandidateBuf = append(torchCandidateBuf, torchCandidate{
				pos:    pos,
				dist:   dx*dx + dz*dz,
				hash:   tileHash(x, z),
				bright: bright,
			})
		}
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
		phase := float32(c.hash&0xFFFF) / 65535.0 * 6.2831853
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
func drawGroundShadow(cx, cz, radius float32) {
	if !groundShadowReady || radius <= 0 {
		return
	}
	rl.DrawModelEx(
		groundShadowModel,
		rl.NewVector3(cx, 0.02, cz),
		rl.NewVector3(0, 1, 0), 0,
		rl.NewVector3(radius*2, 1, radius*2),
		rl.White,
	)
}

// drawDecorBush / drawDecorMushroom / drawDecorPebble are the
// inline-decor implementations registered in inlineDecorHandlers.
// Each one is a thin wrapper around the dedicated propModel field /
// scatter helper on Resources so the dispatch signature stays uniform
// across every handler.
func drawDecorBush(assets Resources, x, z int, cx, cz float32) {
	drawGroundShadow(cx, cz, 0.36)
	assets.bushProp.draw(rl.NewVector3(cx, 0, cz), 0.75, propYawDeg(x, z))
}

func drawDecorMushroom(assets Resources, x, z int, cx, cz float32) {
	drawGroundShadow(cx, cz, 0.20)
	assets.mushroomProp.draw(rl.NewVector3(cx, 0, cz), 1.0, propYawDeg(x, z))
}

func drawDecorPebble(assets Resources, x, z int, cx, cz float32) {
	drawPebbleCluster(assets, cx, cz, tileHash(x, z))
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

// drawFloorDecoration scatters small props (rocks, bushes, mushrooms) on
// plain floor tiles using a deterministic per-tile hash. ~16% of plain floor
// tiles get a decoration; small rocks are weighted heavier than the others
// so the field reads as pebble-strewn ground. Props are passable (don't
// update BlockedAt) and small rocks are squashed in Y so they look walkable.
func drawFloorDecoration(assets Resources, x, z int, cx, cz float32) {
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
	offX := float32(int8(h>>16)) / 230
	offZ := float32(int8(h>>24)) / 230
	pos := rl.NewVector3(cx+offX, 0, cz+offZ)

	// Reuse the orientation hash so floor decorations also pick up a stable
	// yaw — keeps clusters of small props from looking aligned when a few
	// land in the same neighborhood.
	decoYaw := float32(((h >> 12) % 12) * 30)
	switch kind {
	case 0, 1, 2, 3: // pebble cluster — see drawPebbleCluster comment
		drawPebbleCluster(assets, cx, cz, h)
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
func drawPebbleCluster(assets Resources, cx, cz float32, tileHash uint32) {
	if len(assets.rockProp.models) == 0 {
		return
	}
	baseModel := assets.rockProp.models[0]
	rotationAxis := rl.NewVector3(0, 1, 0)

	// 2..4 pebbles per cluster — small enough to read as a scatter, not a pile.
	// Sum of two independent hash bits gives a 25% / 50% / 25% distribution
	// for 2 / 3 / 4 — center-weighted so most clusters feel balanced.
	count := 2 + int(tileHash&0x01) + int((tileHash>>1)&0x01)

	// Light pebble palette. Indexed by per-pebble hash.
	tints := [4]rl.Color{
		rl.NewColor(228, 224, 214, 255),
		rl.NewColor(216, 212, 202, 255),
		rl.NewColor(232, 226, 214, 255),
		rl.NewColor(220, 216, 208, 255),
	}

	for i := 0; i < count; i++ {
		// Salt the tile hash with the pebble index so each member looks
		// independent. Same finalizer as the other render hashes (mix32).
		ih := mix32(tileHash ^ uint32(i+1)*2654435761)

		// Sub-tile offset in [-0.55, 0.55] — pebbles spread across the tile,
		// not bunched at the center.
		ox := float32(int8(ih)) / 230
		oz := float32(int8(ih>>8)) / 230

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
		pos := rl.NewVector3(cx+ox, RockMeshBaseHalfHeight*hght, cz+oz)
		scale := rl.NewVector3(foot, hght, foot*stretch)
		tint := tints[(ih>>28)&0x03]
		rl.DrawModelEx(baseModel, pos, rotationAxis, rot, scale, tint)
	}
}

func DrawEnemies(camera rl.Camera3D, g core.GameState, assets Resources) {
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

// enemyBillboardY is the y-anchor for every enemy/pack billboard. Half
// of TileSize sits the billboard centered on the tile vertically so its
// bottom edge meets the floor when the sprite size's Y is ~tile-height.
// Named so the four call sites that used to inline 0.68 can't drift.
const enemyBillboardY = float32(0.68)

// Party billboard sizes. partyBillboardSize is the idle silhouette; the
// active actor bumps up to partyBillboardSizeActive for a soft "your
// turn" emphasis. Named so the size and the active-state highlight
// stay tunable in one place instead of grepping across world.go,
// timing.go (which reads partyBillboardSize indirectly through
// partySpritePosition's y-anchor), and any future minimap badge.
var (
	partyBillboardSize       = rl.NewVector2(0.38, 0.68)
	partyBillboardSizeActive = rl.NewVector2(0.42, 0.72)
)

// drawFieldPacks renders one billboard per pack — the highest-tier member,
// at the pack's authored tile. Empty/all-dead packs are skipped (they're
// cleaned up by the battle-win path anyway).
func drawFieldPacks(camera rl.Camera3D, g core.GameState, assets Resources) {
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
		leader := core.PackLeader(pack)
		visual, ok := enemyVisualFor(assets, leader.Kind)
		if !ok {
			continue
		}
		// Read the interpolated visual coords directly — placePacks
		// seeds them to the tile center, TickPackAnimations eases
		// them mid-step, and engagement/win paths snap them. No
		// fallback to tileWorldPos is needed; pack.X/Z is always
		// authoritative for the field render.
		position := rl.NewVector3(pack.X, enemyBillboardY, pack.Z)
		drawTextureBillboard(camera, visual.texture, position, visual.size, rl.White)
	}
}

// drawBattlePack renders every member of the active pack in battle
// formation: living and recently-defeated (still fading) alike.
func drawBattlePack(camera rl.Camera3D, g core.GameState, assets Resources) {
	// Same fog-shader gate as drawFieldPacks — billboards recede
	// with the world geometry around them.
	defer beginBillboardFogPass(camera, g, assets)()
	members := core.BattleMembers(&g)
	for i, enemy := range members {
		visual, ok := enemyVisualFor(assets, enemy.Kind)
		if !ok {
			continue
		}
		if !enemy.Alive && enemy.DeathFade <= 0 {
			continue
		}
		position := enemyDrawPosition(camera, g, i, enemy)
		tint := rl.White
		if !enemy.Alive {
			alpha := uint8(220 * core.Clamp(float64(enemy.DeathFade/core.DeathFadeDuration), 0, 1))
			tint = rl.NewColor(255, 255, 255, alpha)
		}
		// Yellow target chevron + tint render only while the player is
		// in the enemy-target picker. targetingEnemy gates on
		// Phase==BattlePlayer so the chevron drops the moment the
		// timing bar arms — shared with the roster row's `targetable`
		// flag so both yellow indicators behave identically.
		if enemy.Alive && targetingEnemy(g) && i == g.Battle.EnemyIndex {
			tint = tintEnemyTargeted
			drawTargetChevron(camera, position)
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
		// Distance fog is applied by the active billboard-fog shader
		// (BeginShaderMode at the top of this function), not by a
		// CPU tint pass — multiplicative tint can't lerp toward the
		// fog color, only darken or color-filter the texture.
		drawTextureBillboard(camera, visual.texture, position, visual.size, tint)
	}
}

// isEnemyAttackerSlot reports whether the given active-pack member slot
// is the one currently lunging at the party (during BattleEnemyTiming).
func isEnemyAttackerSlot(g core.GameState, slot int) bool {
	if g.Battle.Phase != core.BattleEnemyTiming {
		return false
	}
	return g.Battle.EnemyAttacker == slot
}

// enemyAttackTargets returns the party-member indices the currently
// lunging enemy will hit when the defend bar resolves. Drives the red
// "incoming hit" marker above threatened heads during BattleEnemyTiming.
// Every current enemy action (plain melee, Firebolt, Sleep, Ingest) is
// single-target and routes through pickEnemyAttackTarget, which peeks
// non-mutating via WrapNextAvailablePartyMember(EnemyAttackCursor+1).
// Returning a slice (not a lone int) leaves room for AoE enemy skills
// to extend the marker without touching the render call site.
func enemyAttackTargets(g core.GameState) []int {
	if g.Battle.Phase != core.BattleEnemyTiming {
		return nil
	}
	target := core.WrapNextAvailablePartyMember(g.Party, g.Battle.EnemyAttackCursor+1)
	if target < 0 {
		return nil
	}
	return []int{target}
}

// slotInIntList is a tiny linear-scan membership check for the small
// per-frame slices returned by enemyAttackTargets — pulling in slices.Contains
// would mean a new stdlib import for one cold call. Stays inline at the
// caller's risk profile (≤ party-size N).
func slotInIntList(slot int, list []int) bool {
	for _, v := range list {
		if v == slot {
			return true
		}
	}
	return false
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
		tipYOffset: 0.74,
		height:     0.20,
		baseRadius: 0.085,
		color:      rl.NewColor(255, 222, 94, 245),
		phase:      0.0,
	}
	// markerFriendlyTarget is the player's currently-selected ally
	// (heal / item targeting). Green, slightly smaller than the
	// enemy markers since party billboards sit closer to the camera.
	markerFriendlyTarget = markerStyle{
		tipYOffset: 0.36,
		height:     0.13,
		baseRadius: 0.055,
		color:      rl.NewColor(118, 235, 136, 245),
		phase:      0.3,
	}
	// markerEnemyAttackTarget tags the party member(s) the lunging enemy
	// is about to hit — drawn above the threatened head while the defend
	// bar is up. Sized identically to markerFriendlyTarget so the two
	// indicators read as visually paired even when the colors differ.
	markerEnemyAttackTarget = markerStyle{
		tipYOffset: 0.36,
		height:     0.13,
		baseRadius: 0.055,
		color:      rl.NewColor(255, 96, 96, 245),
		phase:      0.9,
	}
)

// drawMarker is the single entry point for every selector-pyramid call
// site. `unitPos` is the unit's billboard center; the helper anchors
// the pyramid tip according to the style's tipYOffset and forwards the
// rest to drawSelectorPyramid.
func drawMarker(unitPos rl.Vector3, style markerStyle) {
	tip := rl.NewVector3(unitPos.X, unitPos.Y+style.tipYOffset, unitPos.Z)
	drawSelectorPyramid(tip, style.height, style.baseRadius, style.color, style.phase)
}

func enemyVisualFor(assets Resources, kind core.EnemyKind) (enemyVisual, bool) {
	if visual, ok := assets.enemyVisuals[kind]; ok && visual.texture.ID != 0 {
		return visual, true
	}
	visual, ok := assets.enemyVisuals[core.EnemyRat]
	return visual, ok && visual.texture.ID != 0
}

func drawTargetChevron(camera rl.Camera3D, position rl.Vector3) {
	drawMarker(position, markerEnemyTarget)
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
		a := yaw + float64(i)*math.Pi/2
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
	clamp := func(v float32) uint8 {
		if v < 0 {
			return 0
		}
		if v > 255 {
			return 255
		}
		return uint8(v)
	}
	return rl.NewColor(
		clamp(float32(c.R)*factor),
		clamp(float32(c.G)*factor),
		clamp(float32(c.B)*factor),
		c.A,
	)
}

func DrawPartySprites(camera rl.Camera3D, g core.GameState, assets Resources) {
	if g.Battle.Phase == core.BattleNone {
		return
	}
	// Same fog-shader gate as drawBattlePack so the party
	// billboards receive the same distance fog the enemies they
	// face do.
	defer beginBillboardFogPass(camera, g, assets)()
	victoryDance := victoryDanceElapsed(g)
	incomingTargets := enemyAttackTargets(g)
	for i := range g.Party {
		// Ingested members are tucked away inside a mantrap — don't
		// draw their billboard on the field. The status badge on the
		// party card is the player's "where did they go?" signal.
		if g.Party[i].Ingested {
			continue
		}
		texture, ok := partyTextureFor(assets, g.Party[i])
		if !ok {
			continue
		}
		memberDance := float32(0)
		if g.Party[i].HP > 0 {
			memberDance = victoryDance
		}
		position := partySpritePosition(camera, i, g.Party[i].Class, g.Party[i].AttackBump, memberDance, g.Party[i].HitKnockback)
		size := partyBillboardSize
		tint := rl.White
		if g.Party[i].HP <= 0 {
			tint = rl.NewColor(110, 110, 120, 190)
		} else if inPlayerTurn(g) && i == g.Battle.CurrentParty {
			tint = rl.NewColor(255, 245, 204, 255)
			size = partyBillboardSizeActive
		} else if memberDance > 0 {
			_, _, _, scale := victoryDanceMotion(g.Party[i].Class, memberDance)
			size.X *= scale
			size.Y *= scale
		}
		if g.Party[i].DamageFlash > 0 {
			tint = core.FlashTint(tint, g.Party[i].DamageFlash)
		}
		// Distance fog is applied by the active billboard-fog
		// shader (BeginShaderMode at the top of this function).
		drawTextureBillboard(camera, texture, position, size, tint)
		// Same gate as the enemy chevron: target marker only during the
		// menu phase, NOT during the timing bar that follows the
		// confirm. inPlayerTurn includes BattleAttackTiming and would
		// linger the marker through the press.
		if g.Battle.Phase == core.BattlePlayer && targetingAlly(g) && i == g.Battle.PartyTarget && g.Party[i].HP > 0 {
			drawFriendlyTargetMarker(camera, position)
		}
		// Red "incoming hit" marker above every party member the lunging
		// enemy is about to strike. Phase gating lives in
		// enemyAttackTargets — it returns nil outside BattleEnemyTiming.
		if g.Party[i].HP > 0 && slotInIntList(i, incomingTargets) {
			drawEnemyAttackTargetMarker(camera, position)
		}
	}
}

func partyTextureFor(assets Resources, member core.PartyMember) (rl.Texture2D, bool) {
	texture, ok := assets.partyTexture[member.Class]
	if !ok || texture.ID == 0 {
		return rl.Texture2D{}, false
	}
	return texture, true
}

func drawFriendlyTargetMarker(camera rl.Camera3D, position rl.Vector3) {
	drawMarker(position, markerFriendlyTarget)
}

func drawEnemyAttackTargetMarker(camera rl.Camera3D, position rl.Vector3) {
	drawMarker(position, markerEnemyAttackTarget)
}

func partySpritePosition(camera rl.Camera3D, index int, class core.PartyClass, bump, victoryDance float32, knockback float32) rl.Vector3 {
	forward := horizontalForward(camera)
	right := rl.NewVector3(-forward.Z, 0, forward.X)
	// Party billboards pushed FURTHER from the camera (0.96 → 1.45)
	// so they sit visibly inside the arena instead of pressed up
	// against the lens. Pairs with the battle-FOV zoom + the
	// downward camera pitch — together the party reads as a row
	// of fighters standing on the floor of the arena rather than
	// a HUD strip pasted onto a sky shot.
	base := rl.NewVector3(
		camera.Position.X+forward.X*1.45,
		0.62,
		camera.Position.Z+forward.Z*1.45,
	)
	offset := (float32(index) - 1.5) * 0.42
	depth := float32(0.02)
	if index == 1 || index == 2 {
		depth = -0.04
	}
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

func victoryDanceElapsed(g core.GameState) float32 {
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
// the active pack at, given its slot in the battle formation.
func enemyDrawPosition(camera rl.Camera3D, g core.GameState, slot int, enemy core.Enemy) rl.Vector3 {
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

	visibleSlot, count := battleEnemySlot(g, slot)
	if count <= 0 {
		p := g.Packs[g.Battle.ActivePack]
		return tileWorldPos(p.TileX, p.TileZ, enemyBillboardY)
	}
	forward := horizontalForward(camera)
	right := rl.NewVector3(-forward.Z, 0, forward.X)
	// Formation center.Y lifted from 0.7 → 1.0 so enemy billboards
	// sit centered vertically in the screen instead of hugging the
	// bottom third under the narrower battle FOV. Pairs with the
	// shrunken targeting markers (markerEnemyTarget etc.) so the
	// roster reads at the same vertical band as the party ribbon's
	// HP bars.
	center := rl.NewVector3(
		camera.Position.X+forward.X*2.55,
		1.0,
		camera.Position.Z+forward.Z*2.55,
	)
	// Adaptive spacing: a tile is ~2.05 world units wide, so a six-
	// enemy formation at the old 1.12 spacing was 5*1.12 = 5.6 units
	// across — well past two tiles, clipping any encounter in a
	// corridor or small room. The new rule caps the TOTAL formation
	// width at formationMaxWidth and packs slots inside that cap,
	// while preserving the original generous spacing for small packs
	// where there's no width concern.
	const baseSpacing = float32(1.12)
	const formationMaxWidth = float32(2.6) // a touch wider than one tile
	spacing := baseSpacing
	if count > 1 {
		fit := formationMaxWidth / float32(count-1)
		if fit < spacing {
			spacing = fit
		}
	}
	offset := (float32(visibleSlot) - float32(count-1)/2) * spacing
	// Depth stagger: a slight forward-back offset on the middle
	// slot(s) so enemies don't render in a perfectly flat line. The
	// previous code only staggered the middle of a 3-pack; extend it
	// to anything 3+ so denser formations get readable depth too.
	depth := float32(0)
	if count >= 3 {
		// Push slots in the interior third forward by a small amount;
		// edge slots stay flat. The middle reads as a "front rank"
		// in the now-tighter formation.
		mid := float32(count-1) / 2
		dist := float32(visibleSlot) - mid
		if dist < 0 {
			dist = -dist
		}
		if dist < mid/2 {
			depth = 0.22
		}
	}
	bump := core.BumpOffset(enemy.AttackBump, 0.2)
	// Reactionary knockback: when the enemy just took damage, push
	// it AWAY from the camera (further into the arena). AttackBump
	// subtracts from depth (lunge forward toward party); knockback
	// adds (recoil backward) — opposite signs so a hit visibly
	// snaps the enemy in the opposite direction of its lunge.
	knock := core.KnockbackOffset(enemy.HitKnockback, core.HitKnockbackDist)
	return rl.NewVector3(
		center.X+right.X*offset+forward.X*(depth-bump+knock),
		center.Y,
		center.Z+right.Z*offset+forward.Z*(depth-bump+knock),
	)
}

func horizontalForward(camera rl.Camera3D) rl.Vector3 {
	x := camera.Target.X - camera.Position.X
	z := camera.Target.Z - camera.Position.Z
	length := float32(math.Hypot(float64(x), float64(z)))
	if length == 0 {
		return rl.NewVector3(1, 0, 0)
	}
	return rl.NewVector3(x/length, 0, z/length)
}

// battleEnemySlot maps a member slot to (visible slot, total visible
// count) — visible meaning alive or still death-fading. Returns -1 for the
// visible slot when the queried member isn't currently visible.
func battleEnemySlot(g core.GameState, memberSlot int) (int, int) {
	visible := 0
	count := 0
	found := -1
	for i, m := range core.BattleMembers(&g) {
		if !m.Alive && m.DeathFade <= 0 {
			continue
		}
		if i == memberSlot {
			found = visible
		}
		visible++
		count++
	}
	return found, count
}

package render

import (
	"math"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

type enemyVisual struct {
	texture rl.Texture2D
	size    rl.Vector2
}

func Camera(p core.Player) rl.Camera3D {
	yaw := p.Yaw + p.LookYaw
	pitch := p.LookPitch
	cp := float32(math.Cos(float64(pitch)))
	direction := rl.NewVector3(
		cp*float32(math.Cos(float64(yaw))),
		float32(math.Sin(float64(pitch))),
		cp*float32(math.Sin(float64(yaw))),
	)
	position := rl.NewVector3(p.X, core.EyeHeight, p.Z)
	return rl.NewCamera3D(
		position,
		rl.NewVector3(position.X+direction.X, position.Y+direction.Y, position.Z+direction.Z),
		rl.NewVector3(0, 1, 0),
		112,
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
	// Dungeon sky stays a fixed dim — the player isn't supposed to see
	// open sky in there anyway, so the day/night cycle doesn't apply.
	// Other materials sample the time profile so the backdrop tracks the
	// same arc as the in-world lighting.
	tint := rl.White
	if m.Materials == core.MaterialDungeon {
		tint = rl.NewColor(54, 56, 70, 255)
	} else {
		tint = skyColor(timeProfileAt(g.StepCount).SkyTint)
	}
	rl.DrawTexturePro(assets.skyTexture, source, dest, rl.NewVector2(0, 0), 0, tint)
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
	tree := assets.tree
	profile := applyTimeOfDay(lightingFor(m.Materials), timeProfileAt(g.StepCount))
	assets.lighting.applyUniforms(camera, profile)

	camPos := camera.Position
	forward := horizontalForward(camera)

	rl.BeginShaderMode(assets.lighting.shader)
	for z := 0; z < m.Height; z++ {
		for x := 0; x < m.Width; x++ {
			cx := core.TileCenter(x)
			cz := core.TileCenter(z)
			dx := cx - camPos.X
			dz := cz - camPos.Z
			if dx*forward.X+dz*forward.Z < behindCullSlack {
				continue
			}
			center := rl.NewVector3(cx, 0, cz)
			// Walls layer wins — solid blocker, no floor underneath.
			if m.Walls[z][x] == core.TileRock {
				drawTileCube(material.wallModel, cx, core.WallHeight/2, cz, tileYawDeg(x, z))
				continue
			}
			// Floor variant comes from the floor layer (auto = hash).
			drawFloorTile(material, assets, m.Floor[z][x], x, z, cx, cz)
			// Decor: explicit char overrides auto-scatter; '_' suppresses.
			drawDecor(assets, m.Decor[z][x], x, z, cx, cz)
			// Props: render by char on the props layer. Built-in cases
			// (T/X/O/B) keep their per-char scale tuning here; everything
			// else looks itself up in assets.propModels at scale 1.0.
			if prop := m.Props[z][x]; prop != core.TilePropEmpty {
				propYaw := propYawDeg(x, z)
				switch prop {
				case core.TileTree:
					tree.draw(center, 1.0, propYaw)
				case core.TileTreeXL:
					tree.draw(center, 1.75, propYaw)
				case core.TileRockLarge:
					assets.rockProp.draw(center, 1.0, propYaw)
				case core.TileBushLarge:
					assets.bushProp.draw(center, 1.3, propYaw)
				default:
					if pm, ok := assets.propModels[prop]; ok {
						pm.draw(center, 1.0, propYaw)
					}
				}
			}
		}
	}
	rl.EndShaderMode()
}

// drawFloorTile picks a floor variant for the given tile and draws it.
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
	model := material.floorModel
	switch cell {
	case core.FloorDirt:
		model = material.floorDirtModel
	case core.FloorDarkGrass:
		model = material.floorDarkModel
	case core.FloorGrass, core.FloorStone:
		model = material.floorModel
	default: // FloorAuto / unrecognized — fall back to the existing hash.
		switch floorVariantHash(x, z) {
		case 1:
			model = material.floorDirtModel
		case 2:
			model = material.floorDarkModel
		}
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
func drawDecor(assets Resources, cell byte, x, z int, cx, cz float32) {
	switch cell {
	case core.DecorEmpty:
		return
	case core.DecorAuto:
		drawFloorDecoration(assets, x, z, cx, cz)
		return
	case core.DecorBush:
		assets.bushProp.draw(rl.NewVector3(cx, 0, cz), 0.75, propYawDeg(x, z))
		return
	case core.DecorMushroom:
		assets.mushroomProp.draw(rl.NewVector3(cx, 0, cz), 1.0, propYawDeg(x, z))
		return
	case core.DecorPebble:
		drawPebbleCluster(assets, cx, cz, tileHash(x, z))
		return
	}
	if dm, ok := assets.decorModels[cell]; ok {
		dm.draw(rl.NewVector3(cx, 0, cz), 1.0, propYawDeg(x, z))
	}
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
		foot := 0.18 + float32((ih>>16)&0x07)*0.012  // 0.18 .. 0.27
		hght := 0.07 + float32((ih>>20)&0x03)*0.012  // 0.07 .. 0.106
		rot := float32((ih>>24)&0xff) * (360.0 / 256) // 0..360°
		// Slight x/z asymmetry so each pebble's silhouette breaks alignment
		// with its neighbors. Sourcing from a different hash bit keeps the
		// asymmetry uncorrelated to size.
		stretch := 0.85 + float32((ih>>4)&0x07)*0.04 // 0.85 .. 1.13

		// Y placement: the underlying cube is 0.42 tall (rockMeshBase in
		// models.go) and propModel's base part offsets it half its height to
		// clear the ground. We draw the mesh directly, not via the prop, so
		// we replicate that math: 0.42/2 = 0.21 → scaled by hght.
		pos := rl.NewVector3(cx+ox, 0.21*hght, cz+oz)
		scale := rl.NewVector3(foot, hght, foot*stretch)
		tint := tints[(ih>>28)&0x03]
		rl.DrawModelEx(baseModel, pos, rotationAxis, rot, scale, tint)
	}
}

func DrawEnemies(camera rl.Camera3D, g core.GameState, assets Resources) {
	if g.Battle.Phase == core.BattleNone {
		drawFieldPacks(camera, g, assets)
		return
	}
	drawBattlePack(camera, g, assets)
}

// drawFieldPacks renders one billboard per pack — the highest-tier member,
// at the pack's authored tile. Empty/all-dead packs are skipped (they're
// cleaned up by the battle-win path anyway).
func drawFieldPacks(camera rl.Camera3D, g core.GameState, assets Resources) {
	for _, pack := range g.Packs {
		if !core.PackAlive(pack) {
			continue
		}
		leader := core.PackLeader(pack)
		visual, ok := enemyVisualFor(assets, leader.Kind)
		if !ok {
			continue
		}
		source := rl.NewRectangle(0, 0, float32(visual.texture.Width), float32(visual.texture.Height))
		position := rl.NewVector3(core.TileCenter(pack.TileX), 0.68, core.TileCenter(pack.TileZ))
		rl.DrawBillboardRec(camera, visual.texture, source, position, visual.size, rl.White)
	}
}

// drawBattlePack renders every member of the active pack in battle
// formation: living and recently-defeated (still fading) alike.
func drawBattlePack(camera rl.Camera3D, g core.GameState, assets Resources) {
	members := core.BattleMembers(&g)
	for i, enemy := range members {
		visual, ok := enemyVisualFor(assets, enemy.Kind)
		if !ok {
			continue
		}
		if !enemy.Alive && enemy.DeathFade <= 0 {
			continue
		}
		source := rl.NewRectangle(0, 0, float32(visual.texture.Width), float32(visual.texture.Height))
		position := enemyDrawPosition(camera, g, i, enemy)
		tint := rl.White
		if !enemy.Alive {
			alpha := uint8(220 * core.ClampFloat64(float64(enemy.DeathFade/core.DeathFadeDuration), 0, 1))
			tint = rl.NewColor(255, 255, 255, alpha)
		}
		if enemy.Alive && g.Battle.ActionMode == core.ActionEnemyTarget && i == g.Battle.EnemyIndex {
			tint = tintEnemyTargeted
			drawTargetChevron(camera, position)
		}
		// During BattleEnemyTiming, mark the currently-attacking enemy with a
		// red chevron + warm tint so the player knows which foe is lunging at
		// them — useful when 2-3 enemies share the screen and only one acts.
		if enemy.Alive && isEnemyAttackerSlot(g, i) {
			tint = tintEnemyAttacker
			drawAttackerChevron(camera, position)
		}
		if enemy.DamageFlash > 0 {
			tint = core.FlashTint(tint, enemy.DamageFlash)
		}
		rl.DrawBillboardRec(camera, visual.texture, source, position, visual.size, tint)
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

// drawAttackerChevron paints the JRPG-style selector pyramid above the
// lunging enemy, tip pointing down at them. Red so it reads as "this one's
// about to swing at you" — the player-target marker is yellow.
func drawAttackerChevron(camera rl.Camera3D, position rl.Vector3) {
	tip := rl.NewVector3(position.X, position.Y+0.80, position.Z)
	drawSelectorPyramid(tip, 0.42, 0.18, rl.NewColor(255, 96, 96, 245), 0.6)
}

func enemyVisualFor(assets Resources, kind core.EnemyKind) (enemyVisual, bool) {
	if visual, ok := assets.enemyVisuals[kind]; ok && visual.texture.ID != 0 {
		return visual, true
	}
	visual, ok := assets.enemyVisuals[core.EnemyRat]
	return visual, ok && visual.texture.ID != 0
}

func drawTargetChevron(camera rl.Camera3D, position rl.Vector3) {
	tip := rl.NewVector3(position.X, position.Y+0.74, position.Z)
	drawSelectorPyramid(tip, 0.38, 0.16, rl.NewColor(255, 222, 94, 245), 0.0)
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
	victoryDance := victoryDanceElapsed(g)
	for i := range g.Party {
		texture, ok := partyTextureFor(assets, g.Party[i])
		if !ok {
			continue
		}
		source := rl.NewRectangle(0, 0, float32(texture.Width), float32(texture.Height))
		memberDance := float32(0)
		if g.Party[i].HP > 0 {
			memberDance = victoryDance
		}
		position := partySpritePosition(camera, i, g.Party[i].Class, g.Party[i].AttackBump, memberDance)
		size := rl.NewVector2(0.38, 0.68)
		tint := rl.White
		if g.Party[i].HP <= 0 {
			tint = rl.NewColor(110, 110, 120, 190)
		} else if inPlayerTurn(g) && i == g.Battle.CurrentParty {
			tint = rl.NewColor(255, 245, 204, 255)
			size = rl.NewVector2(0.42, 0.72)
		} else if memberDance > 0 {
			_, _, _, scale := victoryDanceMotion(g.Party[i].Class, memberDance)
			size.X *= scale
			size.Y *= scale
		}
		if g.Party[i].DamageFlash > 0 {
			tint = core.FlashTint(tint, g.Party[i].DamageFlash)
		}
		rl.DrawBillboardRec(camera, texture, source, position, size, tint)
		if inPlayerTurn(g) && targetingAlly(g) && i == g.Battle.PartyTarget && g.Party[i].HP > 0 {
			drawFriendlyTargetMarker(camera, position)
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
	tip := rl.NewVector3(position.X, position.Y+0.42, position.Z)
	drawSelectorPyramid(tip, 0.34, 0.14, rl.NewColor(118, 235, 136, 245), 0.3)
}

func partySpritePosition(camera rl.Camera3D, index int, class core.PartyClass, bump, victoryDance float32) rl.Vector3 {
	forward := horizontalForward(camera)
	right := rl.NewVector3(-forward.Z, 0, forward.X)
	base := rl.NewVector3(
		camera.Position.X+forward.X*0.96,
		0.62,
		camera.Position.Z+forward.Z*0.96,
	)
	offset := (float32(index) - 1.5) * 0.42
	depth := float32(0.02)
	if index == 1 || index == 2 {
		depth = -0.04
	}
	danceSide, danceDepth, danceHeight, _ := victoryDanceMotion(class, victoryDance)
	bumpDepth := core.BumpOffset(bump, 0.22)
	return rl.NewVector3(
		base.X+right.X*(offset+danceSide)+forward.X*(depth+bumpDepth+danceDepth),
		base.Y+danceHeight,
		base.Z+right.Z*(offset+danceSide)+forward.Z*(depth+bumpDepth+danceDepth),
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
		pack := rl.NewVector3(0, 0.68, 0)
		if g.Battle.ActivePack >= 0 && g.Battle.ActivePack < len(g.Packs) {
			p := g.Packs[g.Battle.ActivePack]
			pack = rl.NewVector3(core.TileCenter(p.TileX), 0.68, core.TileCenter(p.TileZ))
		}
		return pack
	}

	visibleSlot, count := battleEnemySlot(g, slot)
	if count <= 0 {
		p := g.Packs[g.Battle.ActivePack]
		return rl.NewVector3(core.TileCenter(p.TileX), 0.68, core.TileCenter(p.TileZ))
	}
	forward := horizontalForward(camera)
	right := rl.NewVector3(-forward.Z, 0, forward.X)
	center := rl.NewVector3(
		camera.Position.X+forward.X*2.55,
		0.7,
		camera.Position.Z+forward.Z*2.55,
	)
	spacing := float32(1.12)
	offset := (float32(visibleSlot) - float32(count-1)/2) * spacing
	depth := float32(0)
	if count == 3 && visibleSlot == 1 {
		depth = 0.22
	}
	bump := core.BumpOffset(enemy.AttackBump, 0.2)
	return rl.NewVector3(
		center.X+right.X*offset+forward.X*(depth-bump),
		center.Y,
		center.Z+right.Z*offset+forward.Z*(depth-bump),
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

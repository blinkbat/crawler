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

func DrawSkyBackground(assets Resources, m core.GameMap) {
	texW := float32(assets.skyTexture.Width)
	texH := float32(assets.skyTexture.Height)
	screenW := float32(rl.GetScreenWidth())
	screenH := float32(rl.GetScreenHeight())
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
	tint := rl.White
	if m.Materials == core.MaterialDungeon {
		tint = rl.NewColor(54, 56, 70, 255)
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

func DrawWorld(camera rl.Camera3D, m core.GameMap, assets Resources) {
	material := assets.worldMaterial(m.Materials)
	tree := assets.tree
	profile := lightingFor(m.Materials)
	assets.lighting.applyUniforms(camera, profile)

	camPos := camera.Position
	forward := horizontalForward(camera)

	rl.BeginShaderMode(assets.lighting.shader)
	for z, row := range m.Rows {
		// Index loop avoids the UTF-8 decode that `range row` would do over
		// the layout strings — they're ASCII but the runtime doesn't know that.
		for x := 0; x < len(row); x++ {
			cx := core.TileCenter(x)
			cz := core.TileCenter(z)
			dx := cx - camPos.X
			dz := cz - camPos.Z
			if dx*forward.X+dz*forward.Z < behindCullSlack {
				continue
			}
			tile := row[x]
			center := rl.NewVector3(cx, 0, cz)
			if tile == core.TileRock {
				rl.DrawModel(material.wallModel, rl.NewVector3(cx, core.WallHeight/2, cz), 1, rl.White)
				continue
			}
			// Floor under everything that isn't a wall. Pick a variant by
			// hash if this material has alt floors (field's grass/dirt/dark).
			drawFloorTile(material, x, z, cx, cz)
			switch tile {
			case core.TileTree:
				tree.draw(center, 1.0)
			case core.TileTreeXL:
				tree.draw(center, 1.75)
			case core.TileRockLarge:
				assets.rockProp.draw(center, 1.0)
			case core.TileBushLarge:
				assets.bushProp.draw(center, 1.3)
			case core.TileFloor:
				drawFloorDecoration(assets, x, z, cx, cz)
			}
		}
	}
	rl.EndShaderMode()
}

// drawFloorTile picks a floor variant for the given tile and draws it. Tiles
// without alt variants (dungeon) get the base floor.
func drawFloorTile(material worldMaterialResources, x, z int, cx, cz float32) {
	pos := rl.NewVector3(cx, -0.03, cz)
	if !material.hasFloorVariant {
		rl.DrawModel(material.floorModel, pos, 1, rl.White)
		return
	}
	switch floorVariantHash(x, z) {
	case 1:
		rl.DrawModel(material.floorDirtModel, pos, 1, rl.White)
	case 2:
		rl.DrawModel(material.floorDarkModel, pos, 1, rl.White)
	default:
		rl.DrawModel(material.floorModel, pos, 1, rl.White)
	}
}

// floorVariantHash picks 0 (grass) / 1 (dirt) / 2 (dark grass) for a given
// tile. Uses a region-bucketed hash (every 3 tiles snap to the same bucket)
// so variants form patches rather than per-tile speckle. Two independent
// byte samples drive dirt vs dark-grass selection so they don't perfectly
// mask each other.
func floorVariantHash(x, z int) int {
	region := uint32((x/3)*73856093) ^ uint32((z/3)*19349663)
	region ^= region >> 13
	region *= 1274126177
	region ^= region >> 16
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
	h := uint32(x*73856093) ^ uint32(z*19349663)
	h ^= h >> 13
	h *= 1274126177
	h ^= h >> 16
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

	switch kind {
	case 0, 1, 2, 3: // pebble cluster — see drawPebbleCluster comment
		drawPebbleCluster(assets, cx, cz, h)
	case 4: // small bush
		assets.bushProp.draw(pos, 0.75)
	case 5: // tiny mushroom
		assets.mushroomProp.draw(pos, 0.65)
	case 6: // small mushroom
		assets.mushroomProp.draw(pos, 1.05)
	case 7: // small bush variant
		assets.bushProp.draw(pos, 0.6)
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
		// independent. xorshift-style mix, same shape as the other hashes
		// in this file.
		ih := tileHash ^ uint32(i+1)*2654435761
		ih ^= ih >> 13
		ih *= 1274126177
		ih ^= ih >> 16

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
	for i, enemy := range g.Enemies {
		visual, ok := enemyVisualFor(assets, enemy.Kind)
		if !ok {
			continue
		}
		source := rl.NewRectangle(0, 0, float32(visual.texture.Width), float32(visual.texture.Height))
		deathFade := g.Battle.Phase != core.BattleNone && enemy.DeathFade > 0 && core.BattleContainsEnemy(g.Battle, i)
		if !enemy.Alive && !deathFade {
			continue
		}
		position := enemyDrawPosition(camera, g, i, enemy)
		tint := rl.White
		if !enemy.Alive {
			alpha := uint8(220 * core.ClampFloat64(float64(enemy.DeathFade/core.DeathFadeDuration), 0, 1))
			tint = rl.NewColor(255, 255, 255, alpha)
		}
		if enemy.Alive && g.Battle.Phase != core.BattleNone && g.Battle.ActionMode == core.ActionEnemyTarget && i == g.Battle.EnemyIndex {
			tint = rl.NewColor(255, 228, 190, 255)
			drawTargetChevron(camera, position)
		}
		// During BattleEnemyTiming, mark the currently-attacking enemy with a
		// red chevron + warm tint so the player knows which foe is lunging at
		// them — useful when 2-3 enemies share the screen and only one acts.
		if enemy.Alive && isEnemyAttackerSlot(g, i) {
			tint = rl.NewColor(255, 196, 156, 255)
			drawAttackerChevron(camera, position)
		}
		if enemy.DamageFlash > 0 {
			tint = core.FlashTint(tint, enemy.DamageFlash)
		}
		rl.DrawBillboardRec(camera, visual.texture, source, position, visual.size, tint)
	}
}

// isEnemyAttackerSlot reports whether the given absolute enemy index is the
// one currently lunging at the party (during BattleEnemyTiming).
func isEnemyAttackerSlot(g core.GameState, enemyIndex int) bool {
	if g.Battle.Phase != core.BattleEnemyTiming {
		return false
	}
	slot := g.Battle.EnemyAttacker
	if slot < 0 || slot >= len(g.Battle.EnemyGroup) {
		return false
	}
	return g.Battle.EnemyGroup[slot] == enemyIndex
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
// Backface culling is disabled around the draw because we don't fuss over
// per-face winding — the pyramid's faces should render visible from any
// camera angle, and culling-driven invisibility is the same class of bug
// that broke the pickpocket arrows.
func drawSelectorPyramid(tip rl.Vector3, height, baseRadius float32, col rl.Color, phase float32) {
	rl.DisableBackfaceCulling()
	defer rl.EnableBackfaceCulling()

	t := rl.GetTime()
	yaw := t*1.4 + float64(phase)
	bob := float32(math.Sin(t*math.Pi*1.4)) * 0.04

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

	// Four side faces sweep from the tip out to each adjacent pair of base
	// corners. Order doesn't matter for visibility (cull is off) but we go
	// CCW-from-outside so any future per-face shading would behave.
	for i := 0; i < 4; i++ {
		j := (i + 1) % 4
		rl.DrawTriangle3D(tipP, corners[i], corners[j], col)
	}
	// Top cap (the square base) — two triangles.
	rl.DrawTriangle3D(corners[0], corners[2], corners[1], col)
	rl.DrawTriangle3D(corners[0], corners[3], corners[2], col)
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

func enemyDrawPosition(camera rl.Camera3D, g core.GameState, index int, enemy core.Enemy) rl.Vector3 {
	if g.Battle.Phase == core.BattleNone || !core.BattleContainsEnemy(g.Battle, index) {
		return rl.NewVector3(core.TileCenter(enemy.TileX), 0.68, core.TileCenter(enemy.TileZ))
	}

	slot, count := battleEnemySlot(g, index)
	if count <= 0 {
		return rl.NewVector3(core.TileCenter(enemy.TileX), 0.68, core.TileCenter(enemy.TileZ))
	}
	forward := horizontalForward(camera)
	right := rl.NewVector3(-forward.Z, 0, forward.X)
	center := rl.NewVector3(
		camera.Position.X+forward.X*2.55,
		0.7,
		camera.Position.Z+forward.Z*2.55,
	)
	spacing := float32(1.12)
	offset := (float32(slot) - float32(count-1)/2) * spacing
	depth := float32(0)
	if count == 3 && slot == 1 {
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

func battleEnemySlot(g core.GameState, index int) (int, int) {
	slot := 0
	count := 0
	found := -1
	for _, enemyIndex := range g.Battle.EnemyGroup {
		if enemyIndex < 0 || enemyIndex >= len(g.Enemies) {
			continue
		}
		enemy := g.Enemies[enemyIndex]
		if !enemy.Alive && enemy.DeathFade <= 0 {
			continue
		}
		if enemyIndex == index {
			found = slot
		}
		slot++
		count++
	}
	return found, count
}

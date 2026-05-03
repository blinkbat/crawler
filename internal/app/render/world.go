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

func DrawSkyBackground(assets Resources) {
	source := rl.NewRectangle(0, 0, float32(assets.skyTexture.Width), float32(assets.skyTexture.Height))
	dest := rl.NewRectangle(0, 0, float32(rl.GetScreenWidth()), float32(rl.GetScreenHeight()))
	rl.DrawTexturePro(assets.skyTexture, source, dest, rl.NewVector2(0, 0), 0, rl.White)
}

func DrawWorld(m core.GameMap, assets Resources) {
	material := assets.worldMaterial(m.Materials)
	for z, row := range m.Rows {
		for x := range row {
			center := rl.NewVector3(core.TileCenter(x), 0, core.TileCenter(z))
			tile := m.TileAt(x, z)
			if tile != core.TileRock {
				rl.DrawModel(material.floorModel, rl.NewVector3(center.X, -0.03, center.Z), 1, rl.White)
			}
			switch tile {
			case core.TileRock:
				rl.DrawModel(material.wallModel, rl.NewVector3(center.X, core.WallHeight/2, center.Z), 1, rl.White)
			case core.TileTree:
				drawTreeBlocker(center)
			}
		}
	}
}

func drawTreeBlocker(center rl.Vector3) {
	trunk := rl.NewColor(112, 78, 45, 255)
	trunkDark := rl.NewColor(62, 42, 28, 255)
	barkLight := rl.NewColor(146, 102, 58, 255)
	leaf := rl.NewColor(42, 130, 54, 255)
	leafDark := rl.NewColor(22, 86, 41, 255)
	leafLight := rl.NewColor(83, 161, 74, 255)
	leafHighlight := rl.NewColor(124, 190, 92, 255)

	base := rl.NewVector3(center.X, 0.02, center.Z)
	top := rl.NewVector3(center.X, 1.42, center.Z)
	rl.DrawCylinderEx(base, top, 0.18, 0.13, 8, trunk)

	rl.DrawCube(rl.NewVector3(center.X-0.06, 0.48, center.Z-0.18), 0.045, 0.62, 0.035, barkLight)
	rl.DrawCube(rl.NewVector3(center.X+0.07, 0.78, center.Z+0.17), 0.04, 0.54, 0.035, trunkDark)
	rl.DrawCube(rl.NewVector3(center.X+0.16, 0.18, center.Z), 0.35, 0.08, 0.08, trunkDark)
	rl.DrawCube(rl.NewVector3(center.X-0.18, 0.16, center.Z+0.08), 0.32, 0.07, 0.08, trunkDark)

	rl.DrawSphereEx(rl.NewVector3(center.X, 1.62, center.Z), 0.72, 7, 9, leaf)
	rl.DrawSphereEx(rl.NewVector3(center.X-0.38, 1.48, center.Z+0.16), 0.46, 6, 8, leafDark)
	rl.DrawSphereEx(rl.NewVector3(center.X+0.36, 1.5, center.Z-0.12), 0.5, 6, 8, leaf)
	rl.DrawSphereEx(rl.NewVector3(center.X-0.08, 1.98, center.Z-0.08), 0.48, 6, 8, leafLight)
	rl.DrawSphereEx(rl.NewVector3(center.X+0.08, 1.18, center.Z+0.38), 0.38, 5, 7, leafDark)
	rl.DrawSphereEx(rl.NewVector3(center.X-0.24, 1.82, center.Z-0.26), 0.12, 4, 5, leafHighlight)
	rl.DrawSphereEx(rl.NewVector3(center.X+0.32, 1.7, center.Z+0.18), 0.1, 4, 5, leafHighlight)
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
		if enemy.DamageFlash > 0 {
			tint = core.FlashTint(tint, enemy.DamageFlash)
		}
		rl.DrawBillboardRec(camera, visual.texture, source, position, visual.size, tint)
	}
}

func enemyVisualFor(assets Resources, kind core.EnemyKind) (enemyVisual, bool) {
	if visual, ok := assets.enemyVisuals[kind]; ok && visual.texture.ID != 0 {
		return visual, true
	}
	visual, ok := assets.enemyVisuals[core.EnemyRat]
	return visual, ok && visual.texture.ID != 0
}

func drawTargetChevron(camera rl.Camera3D, position rl.Vector3) {
	forward := horizontalForward(camera)
	right := rl.NewVector3(-forward.Z, 0, forward.X)
	center := rl.NewVector3(position.X, position.Y+0.82, position.Z)
	tip := rl.NewVector3(center.X, center.Y-0.16, center.Z)
	left := rl.NewVector3(center.X-right.X*0.18, center.Y+0.1, center.Z-right.Z*0.18)
	rightPt := rl.NewVector3(center.X+right.X*0.18, center.Y+0.1, center.Z+right.Z*0.18)
	rl.DisableBackfaceCulling()
	rl.DrawTriangle3D(tip, left, rightPt, rl.NewColor(255, 222, 94, 245))
	rl.DrawTriangle3D(tip, rightPt, left, rl.NewColor(255, 222, 94, 245))
	rl.EnableBackfaceCulling()
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
		} else if g.Battle.Phase == core.BattlePlayer && i == g.Battle.CurrentParty {
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
		if g.Battle.Phase == core.BattlePlayer && g.Battle.ActionMode == core.ActionPartyTarget && i == g.Battle.PartyTarget && g.Party[i].HP > 0 {
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
	forward := horizontalForward(camera)
	right := rl.NewVector3(-forward.Z, 0, forward.X)
	center := rl.NewVector3(position.X, position.Y+0.62, position.Z)
	left := rl.NewVector3(center.X-right.X*0.16, center.Y+0.11, center.Z-right.Z*0.16)
	rightPt := rl.NewVector3(center.X+right.X*0.16, center.Y+0.11, center.Z+right.Z*0.16)
	tip := rl.NewVector3(center.X, center.Y-0.1, center.Z)
	color := rl.NewColor(118, 235, 136, 245)
	rl.DisableBackfaceCulling()
	rl.DrawTriangle3D(tip, rightPt, left, color)
	rl.DrawTriangle3D(tip, left, rightPt, color)
	rl.DrawCube(rl.NewVector3(center.X, center.Y-0.18, center.Z), 0.08, 0.02, 0.08, color)
	rl.EnableBackfaceCulling()
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

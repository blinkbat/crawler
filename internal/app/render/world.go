package render

import (
	"math"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

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

func DrawWorld(m core.GameMap, assets Resources) {
	for z, row := range m.Rows {
		for x, tile := range row {
			center := rl.NewVector3(core.TileCenter(x), 0, core.TileCenter(z))
			if tile == '#' {
				rl.DrawModel(assets.wallModel, rl.NewVector3(center.X, core.WallHeight/2, center.Z), 1, rl.White)
				continue
			}
			rl.DrawModel(assets.floorModel, rl.NewVector3(center.X, -0.03, center.Z), 1, rl.White)
		}
	}
}

func DrawEnemies(camera rl.Camera3D, g core.GameState, assets Resources) {
	source := rl.NewRectangle(0, 0, float32(assets.ratTexture.Width), float32(assets.ratTexture.Height))
	size := rl.NewVector2(0.82, 1.22)
	for i, enemy := range g.Enemies {
		deathFade := g.Battle.Phase != core.BattleNone && enemy.DeathFade > 0 && battleContainsEnemy(g.Battle, i)
		if !enemy.Alive && !deathFade {
			continue
		}
		position := enemyDrawPosition(camera, g, i, enemy)
		tint := rl.White
		if !enemy.Alive {
			alpha := uint8(220 * core.ClampFloat64(float64(enemy.DeathFade/core.DeathFadeDuration), 0, 1))
			tint = rl.NewColor(255, 255, 255, alpha)
		}
		if enemy.Alive && g.Battle.Phase != core.BattleNone && i == g.Battle.EnemyIndex {
			tint = rl.NewColor(255, 228, 190, 255)
			drawTargetChevron(camera, position)
		}
		if enemy.DamageFlash > 0 {
			tint = core.FlashTint(tint, enemy.DamageFlash)
		}
		rl.DrawBillboardRec(camera, assets.ratTexture, source, position, size, tint)
	}
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
	source := rl.NewRectangle(0, 0, float32(assets.partyTexture[0].Width), float32(assets.partyTexture[0].Height))
	for i := range g.Party {
		if i >= len(assets.partyTexture) {
			break
		}
		position := partySpritePosition(camera, i, g.Party[i].AttackBump)
		size := rl.NewVector2(0.38, 0.68)
		tint := rl.White
		if g.Party[i].HP <= 0 {
			tint = rl.NewColor(110, 110, 120, 190)
		} else if g.Battle.Phase == core.BattlePlayer && i == g.Battle.CurrentParty {
			tint = rl.NewColor(255, 245, 204, 255)
			size = rl.NewVector2(0.42, 0.72)
		}
		if g.Party[i].DamageFlash > 0 {
			tint = core.FlashTint(tint, g.Party[i].DamageFlash)
		}
		rl.DrawBillboardRec(camera, assets.partyTexture[i], source, position, size, tint)
	}
}

func DrawBattlePartyLabels(camera rl.Camera3D, g core.GameState, assets Resources) {
	if g.MenuOpen || g.Battle.Phase == core.BattleNone {
		return
	}
	labelY := float32(rl.GetScreenHeight()) - 86
	for i, member := range g.Party {
		position := partySpritePosition(camera, i, member.AttackBump)
		screen := rl.GetWorldToScreen(rl.NewVector3(position.X, position.Y, position.Z), camera)
		if screen.X < -80 || screen.X > float32(rl.GetScreenWidth())+80 || screen.Y < -80 || screen.Y > float32(rl.GetScreenHeight())+80 {
			continue
		}
		drawPartyStatLabel(
			assets.hudFont,
			member,
			screen.X,
			labelY,
			g.Battle.Phase == core.BattlePlayer && i == g.Battle.CurrentParty && member.HP > 0,
			g.Battle.Phase == core.BattlePlayer && g.Battle.ActionMode == core.ActionPartyTarget && i == g.Battle.PartyTarget,
			member.HP <= 0,
		)
	}
}

func partySpritePosition(camera rl.Camera3D, index int, bump float32) rl.Vector3 {
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
	return rl.NewVector3(
		base.X+right.X*offset+forward.X*(depth+core.BumpOffset(bump, 0.22)),
		base.Y,
		base.Z+right.Z*offset+forward.Z*(depth+core.BumpOffset(bump, 0.22)),
	)
}

func enemyDrawPosition(camera rl.Camera3D, g core.GameState, index int, enemy core.Enemy) rl.Vector3 {
	if g.Battle.Phase == core.BattleNone || !battleContainsEnemy(g.Battle, index) {
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

func battleContainsEnemy(b core.Battle, index int) bool {
	for _, enemyIndex := range b.EnemyGroup {
		if enemyIndex == index {
			return true
		}
	}
	return false
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

func DrawSkybox(assets Resources, position rl.Vector3) {
	rl.DisableBackfaceCulling()
	rl.DrawModelEx(
		assets.skyModel,
		position,
		rl.NewVector3(0, 1, 0),
		0,
		rl.NewVector3(80, 80, 80),
		rl.White,
	)
	rl.EnableBackfaceCulling()
}

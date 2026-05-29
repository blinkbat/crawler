package core

import "slices"

// AreaContentEqual reports whether two areas have identical authorable
// content. Path is intentionally ignored: saving a map under a new file name
// should not mark the tile/entity data dirty.
func AreaContentEqual(a, b AreaDefinition) bool {
	if a.Name != b.Name || a.Width != b.Width || a.Height != b.Height ||
		a.Materials != b.Materials ||
		a.StartTileX != b.StartTileX || a.StartTileZ != b.StartTileZ ||
		a.StartFacing != b.StartFacing ||
		a.QuietMessage != b.QuietMessage {
		return false
	}
	if !slices.Equal(a.Walls, b.Walls) ||
		!slices.Equal(a.Floor, b.Floor) ||
		!slices.Equal(a.Decor, b.Decor) ||
		!slices.Equal(a.Props, b.Props) ||
		!slices.Equal(a.Ceiling, b.Ceiling) {
		return false
	}
	if !packSpawnsEqual(a.PackSpawns, b.PackSpawns) ||
		!chestSpawnsEqual(a.ChestSpawns, b.ChestSpawns) ||
		!slices.Equal(a.DoorSpawns, b.DoorSpawns) ||
		!customEnemiesEqual(a.CustomEnemies, b.CustomEnemies) {
		return false
	}
	return true
}

func packSpawnsEqual(a, b []PackSpawn) bool {
	return slices.EqualFunc(a, b, func(ap, bp PackSpawn) bool {
		return ap.TileX == bp.TileX && ap.TileZ == bp.TileZ &&
			slices.Equal(ap.Members, bp.Members)
	})
}

func chestSpawnsEqual(a, b []ChestSpawn) bool {
	return slices.EqualFunc(a, b, func(ap, bp ChestSpawn) bool {
		return ap.TileX == bp.TileX && ap.TileZ == bp.TileZ &&
			slices.Equal(ap.Items, bp.Items)
	})
}

func customEnemiesEqual(a, b []CustomEnemyDef) bool {
	return slices.EqualFunc(a, b, func(ap, bp CustomEnemyDef) bool {
		return ap.Name == bp.Name && ap.BaseKind == bp.BaseKind &&
			ap.HP == bp.HP && ap.MP == bp.MP &&
			ap.Stats == bp.Stats && ap.Armor == bp.Armor &&
			ap.XPValue == bp.XPValue && ap.Tier == bp.Tier &&
			ap.AttackDamage == bp.AttackDamage &&
			ap.SkillCastChance == bp.SkillCastChance &&
			ap.SpellPower == bp.SpellPower &&
			slices.Equal(ap.Skills, bp.Skills)
	})
}

// CloneArea deep-copies an AreaDefinition for editor undo/redo snapshots.
func CloneArea(a AreaDefinition) AreaDefinition {
	out := a
	out.Walls = append([]string(nil), a.Walls...)
	out.Floor = append([]string(nil), a.Floor...)
	out.Decor = append([]string(nil), a.Decor...)
	out.Props = append([]string(nil), a.Props...)
	out.Ceiling = append([]string(nil), a.Ceiling...)
	out.PackSpawns = make([]PackSpawn, len(a.PackSpawns))
	for i, sp := range a.PackSpawns {
		out.PackSpawns[i] = PackSpawn{
			TileX:   sp.TileX,
			TileZ:   sp.TileZ,
			Members: append([]PackMemberRef(nil), sp.Members...),
			AI:      sp.AI,
		}
	}
	out.ChestSpawns = make([]ChestSpawn, len(a.ChestSpawns))
	for i, sp := range a.ChestSpawns {
		out.ChestSpawns[i] = ChestSpawn{
			TileX: sp.TileX,
			TileZ: sp.TileZ,
			Items: append([]ItemKind(nil), sp.Items...),
		}
	}
	out.DoorSpawns = append([]DoorSpawn(nil), a.DoorSpawns...)
	out.CustomEnemies = make([]CustomEnemyDef, len(a.CustomEnemies))
	for i, ce := range a.CustomEnemies {
		out.CustomEnemies[i] = ce
		out.CustomEnemies[i].Skills = append([]SkillID(nil), ce.Skills...)
	}
	return out
}

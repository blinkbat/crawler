package core

import "slices"

// gridLayers returns pointers to the area's five authored grid-layer
// slices in canonical order (walls, floor, decor, props, ceiling). The
// single place that enumerates the grid layers for bulk operations —
// CloneArea and AreaContentEqual walk this instead of hand-listing the
// five fields, so a sixth layer is one row here, not several lockstep
// edits. (The MapFile↔Area converters still list the fields explicitly:
// they pair each layer with the separate mapfile.MapFile type and apply
// the ceiling blank-default, which this accessor can't model.)
func (a *AreaDefinition) gridLayers() []*[]string {
	return []*[]string{&a.Walls, &a.Floor, &a.Decor, &a.Props, &a.Ceiling}
}

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
	al, bl := a.gridLayers(), b.gridLayers()
	for i := range al {
		if !slices.Equal(*al[i], *bl[i]) {
			return false
		}
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
			ap.AI == bp.AI &&
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
			ap.Stats == bp.Stats && ap.Armor == bp.Armor && ap.MDef == bp.MDef &&
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
	dst := out.gridLayers()
	src := a.gridLayers()
	for i := range dst {
		*dst[i] = append([]string(nil), *src[i]...)
	}
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

package core

// Runtime effects invoked by trigger/dialog Actions (see trigger.go). Each mutates
// the live GameState in place; movement/rendering read g.Area + the runtime lists
// every frame, so the effects take hold immediately. None persist across save/load
// on their own (the world rebuilds from the .map) — durable effects should be gated
// on a persisted Switch/Counter via a Preserve trigger, which re-applies on load.

// placementOffMap reports whether a runtime placement at (x,z) must bail: no game or
// off the map. The shared guard prologue for the Spawn*/OpenWall/Teleport effects.
func placementOffMap(g *GameState, x, z int) bool {
	return g == nil || !g.Area.InBounds(x, z)
}

// onPlayerTile reports whether (x,z) is the party's tile — a placement there would
// embed a pack/chest under the party, so the spawn effects skip it.
func (g *GameState) onPlayerTile(x, z int) bool {
	return g.Player.TileX == x && g.Player.TileZ == z
}

// SpawnFoeAt drops a one-member pack of kind at (x,z) on the given level (0 = resolve
// to the tile's standable surface). No-op off-map, on the player's tile, or where a
// pack already stands. The pack engages by the normal step-into / AI rules.
func SpawnFoeAt(g *GameState, kind EnemyKind, x, z, level int) {
	if placementOffMap(g, x, z) || g.onPlayerTile(x, z) {
		return
	}
	if PackIndexAtTile(g.Packs, x, z) >= 0 {
		return
	}
	lvl := g.Area.resolveEntityLevel(x, z, level)
	g.Packs = append(g.Packs, Pack{
		TileX:     x,
		TileZ:     z,
		Level:     lvl,
		HomeX:     x,
		HomeZ:     z,
		X:         TileCenter(x),
		Z:         TileCenter(z),
		Members:   []Enemy{NewEnemy(kind)},
		PatrolDir: 1,
	})
}

// SpawnChestAt drops a chest holding items at (x,z). No-op off-map, on the player's
// tile, or where a chest already stands (chests block their tile). An empty item
// list yields a pre-looted chest (matches placeChests).
func SpawnChestAt(g *GameState, x, z, level int, items []ItemKind) {
	if placementOffMap(g, x, z) || g.onPlayerTile(x, z) {
		return
	}
	if ChestIndexAt(g.Chests, x, z) >= 0 {
		return
	}
	var stacks []ItemStack
	for _, kind := range items {
		stacks = AddItem(stacks, kind, 1)
	}
	g.Chests = append(g.Chests, Chest{
		TileX:  x,
		TileZ:  z,
		Level:  g.Area.resolveEntityLevel(x, z, level),
		Items:  stacks,
		Looted: len(stacks) == 0,
	})
}

// OpenWallAt makes (x,z) walkable by lowering the wall there to the party's standing
// level: a voxel column collapses to a solid top the party can stand on; a heightfield
// tile drops to the party's elevation. An explicit level>0 overrides "match the party"
// (an authored openWall using the map's own level convention). No-op off-map or on a
// flat map with no wall layer. Rendering/movement caches key on the grid layers, so
// the passage opens the same frame.
func OpenWallAt(g *GameState, x, z, level int) {
	if placementOffMap(g, x, z) {
		return
	}
	a := &g.Area
	if len(a.Solids) > 0 {
		top := level
		if top <= 0 {
			top = g.Player.Level
		}
		a.SetColumnTop(x, z, top)
		return
	}
	if len(a.Elevation) == 0 {
		return // flat map — nothing raised to open
	}
	elev := level
	if elev <= 0 {
		elev = a.ElevationLevelAt(g.Player.TileX, g.Player.TileZ)
	}
	setElevationCell(a, x, z, elev)
}

// TeleportParty moves the party to (x,z) on level (0 = resolve to the tile's surface),
// centering the sprite. No animation — an instant blink (the trigger-action move).
func TeleportParty(g *GameState, x, z, level int) {
	if placementOffMap(g, x, z) {
		return
	}
	g.Player.TileX = x
	g.Player.TileZ = z
	g.Player.Level = g.Area.resolveEntityLevel(x, z, level)
	g.Player.X = TileCenter(x)
	g.Player.Z = TileCenter(z)
	g.Player.Anim = Animation{}
}

// setElevationCell rewrites the elevation grid cell (x,z) to `level` via an immutable
// -string row rebuild (the canonical layer-edit pattern). No-op if the row is missing.
func setElevationCell(a *AreaDefinition, x, z, level int) {
	if z < 0 || z >= len(a.Elevation) {
		return
	}
	row := []byte(a.Elevation[z])
	if x < 0 || x >= len(row) {
		return
	}
	row[x] = ElevationChar(level)
	a.Elevation[z] = string(row)
}

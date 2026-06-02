package render

import (
	"crawler/internal/app/core"
	rl "github.com/gen2brain/raylib-go/raylib"
	"image/color"
	"os"
	"runtime"
)

type Resources struct {
	materials  map[core.MaterialSet]worldMaterialResources
	skyTexture rl.Texture2D
	// starTexture is a transparent overlay sampled by DrawSkyBackground
	// on top of skyTexture. Alpha varies with the time-of-day curve
	// (timeProfile.StarAlpha), so the layer fades in through evening,
	// peaks at midnight, and washes out through dawn.
	starTexture rl.Texture2D
	// enemyVisuals is the kind→billboard lookup. Multiple kinds can
	// alias the same texture (placeholder sprites for new monsters),
	// so it does NOT own the underlying handles — Unload would
	// double-free them. enemyTextures is the canonical owning list
	// the renderer minted at load time; Unload walks that.
	enemyVisuals  map[core.EnemyKind]enemyVisual
	enemyTextures []rl.Texture2D
	partyTexture  map[core.PartyClass]rl.Texture2D
	hudFont       rl.Font
	hudFontOwned  bool

	lighting     lightingShader
	billboardFog billboardFogShaderPipe
	tree         treeModel

	// Field-only props. Boulders / bushes / mushrooms are scattered as
	// blockers (large) or procedural decorations (small/tiny) when the
	// active area is the field.
	rockProp     propModel
	bushProp     propModel
	mushroomProp propModel
	// chestBody / chestLid are the painted-wood chest pieces. Drawn
	// in DrawChests as two parts so the looted-chest path can lift
	// the lid straight up + tilt back without re-posing the body.
	chestBody propModel
	chestLid  propModel

	// Universal floor variants — keyed by their floor-layer char so the
	// renderer can swap in a cobblestone, plank, water, sand or snow tile
	// regardless of the area's material set. Built once on load and shared
	// across materials.
	specialFloors map[byte]rl.Model

	// New decor models keyed by decor-layer char (tall grass, flowers,
	// clover, reeds, bones, scorch, blood, cobweb, stump, log, leaf pile).
	// Authoring + Unload iteration uses the map; the per-tile-per-frame
	// renderer reads decorModelTable below for array-indexed dispatch.
	decorModels map[byte]propModel
	// decorModelTable is the [256]propModel mirror of decorModels,
	// held by pointer so passing Resources by value (drawDecor,
	// DrawWorld, ...) stays cheap — we'd otherwise copy ~12KB of
	// table per call. The world-draw hot path reads it once per
	// tile per frame; an array index is cheaper than the map hash.
	// "Registered" check is len(parts) > 0 — every load* helper builds
	// at least one part, so an empty slice means "not registered for
	// this char." The map stays as the authored source and as the
	// Unload iteration target so freeing handles is map-safe.
	decorModelTable *[256]propModel

	// New blocking props keyed by props-layer char (crate, barrel, urn,
	// stalagmite, pillar, broken pillar, statue, obelisk, fountain). Same
	// dispatch shape as decorModels — the renderer falls back to the
	// existing tree/boulder/bush cases when a char isn't here.
	propModels map[byte]propModel
	// propModelTable is the [256]propModel mirror of propModels, by
	// pointer for the same Resources-pass-by-value reason as
	// decorModelTable. Indexed dispatch on the hot path; map remains
	// for authoring and Unload.
	propModelTable *[256]propModel

	// doorProps holds one model per core.DoorStyle (building / cave /
	// field). DrawDoors indexes by the door's Style and rotates the chosen
	// model by the authored facing so the opening points the right way.
	doorProps [core.DoorStyleCount]propModel
}

type worldMaterialResources struct {
	// Each model owns its own diffuse texture via its material. Unload via
	// rl.UnloadModel only — don't keep separate texture handles.
	wallModel    rl.Model
	floorModel   rl.Model
	ceilingModel rl.Model // thin slab textured with the wall pixels, drawn over ceiling-flagged tiles
	// Optional secondary floor variants for the field (dirt + dark grass).
	// Picked per-tile by hash so the field reads as varied terrain instead
	// of one uniform grass texture. Empty for the dungeon material.
	floorDirtModel  rl.Model
	floorDarkModel  rl.Model
	hasFloorVariant bool
}

// LoadResources builds every procedural texture/model/font/shader the
// renderer needs. Staged cleanup: each handle is committed to `r` as it's
// created, and a deferred recover() calls r.Unload() if construction
// panics partway. That way a mid-load failure (texture upload OOM, prop
// builder panic) doesn't leak the handles that DID make it onto the GPU.
//
// On success we re-panic after cleanup so the caller still sees the
// failure — this isn't a graceful degradation, just a leak-safe abort.
func LoadResources() (r Resources) {
	committed := false
	defer func() {
		if committed {
			return
		}
		if rec := recover(); rec != nil {
			// Walk every field we managed to populate and unload it. r.Unload
			// is tolerant of zero values: ranging nil maps is a no-op, and
			// raylib's UnloadModel/UnloadTexture on a zero-ID handle skips
			// cleanly (the underlying GL deleters guard on id == 0).
			r.Unload()
			panic(rec)
		}
	}()

	r.lighting = loadLightingShader()
	r.billboardFog = loadBillboardFogShader()

	dungeonMat := loadWorldMaterial(makeStoneBrickPixels(128, 128), makeStoneFloorPixels(128, 128), r.lighting.shader)
	fieldMat := loadWorldMaterial(makeRockWallPixels(128, 128), makeGrassPixels(128, 128), r.lighting.shader)
	// Commit both base materials BEFORE building the variants so a panic in
	// the variants doesn't leak the base wall/floor models.
	r.materials = map[core.MaterialSet]worldMaterialResources{
		core.MaterialDungeon: dungeonMat,
		core.MaterialField:   fieldMat,
	}
	// Field gets two extra floor variants (dirt + dark grass), procedurally
	// chosen per tile by hash for terrain variation. Built using the same
	// path as the primary floor so they share filter / mipmap settings.
	// Commit after EACH variant load so a panic in the second one leaves
	// the first in r.materials for the recover-path Unload to free. (Unload
	// frees the variant handles unconditionally, so the variant-less commit
	// at line ~130 wouldn't have freed a half-built pair on its own.)
	fieldMat.floorDirtModel = loadFloorModel(makeDirtPixels(128, 128), r.lighting.shader)
	r.materials[core.MaterialField] = fieldMat
	fieldMat.floorDarkModel = loadFloorModel(makeDarkGrassPixels(128, 128), r.lighting.shader)
	fieldMat.hasFloorVariant = true
	r.materials[core.MaterialField] = fieldMat
	// Parallel to assertDecorCoverage / assertPropCoverage: every
	// core.MaterialSet must have a loaded worldMaterial. Without this a
	// new material would silently fall back to Field in worldMaterial().
	assertMaterialCoverage(r.materials)

	r.skyTexture = loadTexture(makeSkyPixels(1024, 512), 1024, 512, rl.FilterTrilinear)
	rl.GenTextureMipmaps(&r.skyTexture)
	rl.SetTextureFilter(r.skyTexture, rl.FilterTrilinear)
	rl.SetTextureWrap(r.skyTexture, rl.WrapClamp)

	// Star overlay: same dimensions as the sky so source-rect math
	// in DrawSkyBackground works on either texture without a per-call
	// branch. Point filter (no mipmaps, no trilinear) keeps the
	// pinpoint stars from blurring into wide smudges at small dest
	// scales — a star is a 1- or 2-pixel highlight by design.
	r.starTexture = loadTexture(makeStarPixels(1024, 512), 1024, 512, rl.FilterPoint)
	rl.SetTextureWrap(r.starTexture, rl.WrapClamp)

	r.enemyVisuals, r.enemyTextures = loadEnemyVisuals()

	r.partyTexture = make(map[core.PartyClass]rl.Texture2D)
	for _, def := range core.PartyClasses() {
		texture := loadTexture(makePartyPixels(64, 80, def.Class), 64, 80, rl.FilterPoint)
		rl.SetTextureWrap(texture, rl.WrapClamp)
		r.partyTexture[def.Class] = texture
	}

	r.hudFont, r.hudFontOwned = loadHUDFont()

	// Bark and leaf textures are authored at non-tile sizes (64×128 and 96×96)
	// so they go through loadRepeatTexture; the rock-wall pixels live at
	// the standard 128×128 tile size and use the mipmapped pipeline.
	// These textures are about to be handed to loadTreeModel, which assumes
	// ownership via the model. We don't commit them to r between create and
	// hand-off (small leak window), but staged cleanup catches panics inside
	// loadTreeModel itself via r.tree if it gets assigned.
	barkTex := loadRepeatTexture(makeBarkPixels(64, 128), 64, 128)
	leafTex := loadRepeatTexture(makeLeafPixels(96, 96), 96, 96)
	r.tree = loadTreeModel(r.lighting.shader, barkTex, leafTex)

	// Field props get their own texture instances so the prop models own
	// them outright (UnloadModel handles the texture). Sharing would either
	// double-unload or require external ownership tracking.
	rockTex := loadTiledTexture(makeRockWallPixels(128, 128))
	bushTex := loadRepeatTexture(makeLeafPixels(96, 96), 96, 96)

	r.rockProp = loadRockProp(r.lighting.shader, rockTex)
	r.bushProp = loadBushProp(r.lighting.shader, bushTex)
	r.mushroomProp = loadMushroomProp(r.lighting.shader)
	// Chest props share the bark texture family but mint two
	// distinct atlas instances so each model owns its texture
	// outright (setModelTexture → unload-with-model contract).
	chestBodyWoodTex := loadRepeatTexture(makeBarkPixels(64, 128), 64, 128)
	chestLidWoodTex := loadRepeatTexture(makeBarkPixels(64, 128), 64, 128)
	r.chestBody = loadChestBodyProp(r.lighting.shader, chestBodyWoodTex)
	r.chestLid = loadChestLidProp(r.lighting.shader, chestLidWoodTex)

	// Soft ground-shadow disc — a flat plane textured with the
	// radial-gradient shadow sprite, drawn UNLIT (default material
	// shader) so the lighting pass never touches it. Stored as a
	// package singleton (groundShadowModel) because drawGroundShadow
	// is called from many free-function prop draws.
	groundShadowModel = loadGroundShadowModel()
	groundShadowReady = true

	// Wall-torch flame blob — a small unlit emissive sphere (default
	// material shader, like the ground-shadow disc) tinted to fire
	// colours and animated per-frame by drawWallTorch.
	torchFlameModel = rl.LoadModelFromMesh(rl.GenMeshSphere(1, 8, 10))
	torchFlameReady = true

	// Universal floor variants — built once and shared across every material
	// set so a cobblestone path through a dungeon and one across a field
	// read identically. Initialize the map first so a panic mid-way still
	// unloads the variants that did land.
	r.specialFloors = make(map[byte]rl.Model)
	r.specialFloors[core.FloorCobble] = loadFloorModel(makeCobblePixels(128, 128), r.lighting.shader)
	r.specialFloors[core.FloorPlank] = loadFloorModel(makePlankPixels(128, 128), r.lighting.shader)
	r.specialFloors[core.FloorWater] = loadFloorModel(makeWaterPixels(128, 128), r.lighting.shader)
	r.specialFloors[core.FloorDeepWater] = loadFloorModel(makeDeepWaterPixels(128, 128), r.lighting.shader)
	r.specialFloors[core.FloorSand] = loadFloorModel(makeSandPixels(128, 128), r.lighting.shader)
	r.specialFloors[core.FloorSnow] = loadFloorModel(makeSnowPixels(128, 128), r.lighting.shader)
	// Grass / dirt / dark grass / stone are also universal — without these,
	// painting "Grass" or "Stone" inside a dungeon-material map silently
	// reuses the material's base floorModel and looks identical to default,
	// because the per-material variant switch in drawFloorTile only kicks
	// in when hasFloorVariant is true (field-only).
	r.specialFloors[core.FloorGrass] = loadFloorModel(makeGrassPixels(128, 128), r.lighting.shader)
	r.specialFloors[core.FloorDirt] = loadFloorModel(makeDirtPixels(128, 128), r.lighting.shader)
	r.specialFloors[core.FloorDarkGrass] = loadFloorModel(makeDarkGrassPixels(128, 128), r.lighting.shader)
	r.specialFloors[core.FloorStone] = loadFloorModel(makeStoneFloorPixels(128, 128), r.lighting.shader)

	// Stone family textures for the new prop set. Each loader owns the
	// texture handle outright via setModelTexture so unload-by-model is
	// enough — no separate UnloadTexture call required here.
	marbleTex := loadTiledTexture(makeMarblePixels(128, 128))
	graniteTex := loadTiledTexture(makeGranitePixels(128, 128))
	terracottaTex := loadTiledTexture(makeTerracottaPixels(128, 128))
	// Crates and barrels reuse the bark wood-grain palette. Stumps and
	// logs do too; leaf piles reuse the existing leaf texture. We mint
	// fresh texture instances per loader since each propModel owns its
	// textures via setModelTexture and unloads them when the model unloads
	// — sharing would double-unload.
	crateWoodTex := loadRepeatTexture(makeBarkPixels(64, 128), 64, 128)
	barrelWoodTex := loadRepeatTexture(makeBarkPixels(64, 128), 64, 128)
	stumpBarkTex := loadRepeatTexture(makeBarkPixels(64, 128), 64, 128)
	logBarkTex := loadRepeatTexture(makeBarkPixels(64, 128), 64, 128)
	logMossTex := loadRepeatTexture(makeLeafPixels(96, 96), 96, 96)
	leafPileTex := loadRepeatTexture(makeLeafPixels(96, 96), 96, 96)

	// Commit propModels incrementally so each prop is owned by r before
	// the next one starts loading.
	r.propModels = make(map[byte]propModel)
	r.propModels[core.TileCrate] = loadCrateProp(r.lighting.shader, crateWoodTex)
	r.propModels[core.TileBarrel] = loadBarrelProp(r.lighting.shader, barrelWoodTex)
	r.propModels[core.TileUrn] = loadUrnProp(r.lighting.shader, terracottaTex)
	r.propModels[core.TileStalagmite] = loadStalagmiteProp(r.lighting.shader, marbleTex)
	r.propModels[core.TilePillar] = loadPillarProp(r.lighting.shader, marbleTex)
	r.propModels[core.TileBrokenPillar] = loadBrokenPillarProp(r.lighting.shader, marbleTex)
	r.propModels[core.TileStatue] = loadStatueProp(r.lighting.shader, marbleTex)
	r.propModels[core.TileObelisk] = loadObeliskProp(r.lighting.shader, graniteTex)
	r.propModels[core.TileFountain] = loadFountainProp(r.lighting.shader, marbleTex)

	// Larger rock formations. Each owns its own rock texture instance for
	// the same single-ownership reason as the field props above.
	cairnRockTex := loadTiledTexture(makeRockWallPixels(128, 128))
	formationRockTex := loadTiledTexture(makeRockWallPixels(128, 128))
	r.propModels[core.TileRockCairn] = loadRockCairnProp(r.lighting.shader, cairnRockTex)
	r.propModels[core.TileRockFormation] = loadRockFormationProp(r.lighting.shader, formationRockTex)

	// Turn B outdoor batch — well/gravestone use the rock texture
	// family; signpost/scarecrow use the bark wood-grain. Each prop
	// owns its texture so propModel.unload() in Resources.Unload
	// frees them.
	wellRockTex := loadTiledTexture(makeRockWallPixels(128, 128))
	graveRockTex := loadTiledTexture(makeRockWallPixels(128, 128))
	signWoodTex := loadRepeatTexture(makeBarkPixels(64, 128), 64, 128)
	scarecrowWoodTex := loadRepeatTexture(makeBarkPixels(64, 128), 64, 128)
	r.propModels[core.TileWell] = loadWellProp(r.lighting.shader, wellRockTex)
	r.propModels[core.TileGravestone] = loadGravestoneProp(r.lighting.shader, graveRockTex)
	r.propModels[core.TileSignPost] = loadSignPostProp(r.lighting.shader, signWoodTex)
	r.propModels[core.TileHayBale] = loadHayBaleProp(r.lighting.shader)
	r.propModels[core.TileScarecrow] = loadScarecrowProp(r.lighting.shader, scarecrowWoodTex)

	// Turn B dungeon-interior batch — bookshelf/table/bed share the
	// wood texture family; brazier is shader-only metal; sarcophagus
	// uses marble.
	bookshelfWoodTex := loadRepeatTexture(makeBarkPixels(64, 128), 64, 128)
	tableWoodTex := loadRepeatTexture(makeBarkPixels(64, 128), 64, 128)
	bedWoodTex := loadRepeatTexture(makeBarkPixels(64, 128), 64, 128)
	sarcoMarbleTex := loadTiledTexture(makeMarblePixels(128, 128))
	r.propModels[core.TileBookshelf] = loadBookshelfProp(r.lighting.shader, bookshelfWoodTex)
	r.propModels[core.TileTable] = loadTableProp(r.lighting.shader, tableWoodTex)
	r.propModels[core.TileBed] = loadBedProp(r.lighting.shader, bedWoodTex)
	r.propModels[core.TileBrazier] = loadBrazierProp(r.lighting.shader)
	r.propModels[core.TileSarcophagus] = loadSarcophagusProp(r.lighting.shader, sarcoMarbleTex)

	r.decorModels = make(map[byte]propModel)
	r.decorModels[core.DecorTallGrass] = loadTallGrassProp(r.lighting.shader)
	r.decorModels[core.DecorFlowers] = loadFlowerProp(r.lighting.shader)
	r.decorModels[core.DecorClover] = loadCloverProp(r.lighting.shader)
	r.decorModels[core.DecorReeds] = loadReedProp(r.lighting.shader)
	r.decorModels[core.DecorBones] = loadBoneProp(r.lighting.shader)
	r.decorModels[core.DecorScorch] = loadScorchProp(r.lighting.shader)
	r.decorModels[core.DecorBlood] = loadBloodProp(r.lighting.shader)
	r.decorModels[core.DecorCobweb] = loadCobwebProp(r.lighting.shader)
	r.decorModels[core.DecorStump] = loadStumpProp(r.lighting.shader, stumpBarkTex)
	r.decorModels[core.DecorLog] = loadLogProp(r.lighting.shader, logBarkTex, logMossTex)
	r.decorModels[core.DecorLeafPile] = loadLeafPileProp(r.lighting.shader, leafPileTex)
	r.decorModels[core.DecorLilypad] = loadLilypadProp(r.lighting.shader)
	// Archway uses marble palette to match the existing pillars/statues.
	archMarbleTex := loadTiledTexture(makeMarblePixels(128, 128))
	r.decorModels[core.DecorArchway] = loadArchwayDecor(r.lighting.shader, archMarbleTex)

	// Turn B atmospheric decor batch. Rug / candle / footprints /
	// ash heap / puddle have no textures (procedural color only);
	// rootCluster uses bark so it picks up the wood-grain shading.
	rootBarkTex := loadRepeatTexture(makeBarkPixels(64, 128), 64, 128)
	r.decorModels[core.DecorRug] = loadRugProp(r.lighting.shader)
	r.decorModels[core.DecorCandle] = loadCandleProp(r.lighting.shader)
	r.decorModels[core.DecorBootprints] = loadBootprintsProp(r.lighting.shader)
	r.decorModels[core.DecorAshHeap] = loadAshHeapProp(r.lighting.shader)
	r.decorModels[core.DecorPuddle] = loadPuddleProp(r.lighting.shader)
	r.decorModels[core.DecorRootCluster] = loadRootClusterProp(r.lighting.shader, rootBarkTex)

	// Door props — one model per style, drawn at every g.Doors entry and
	// rotated by authored facing. Each owns its texture via setModelTexture;
	// freed by the doorProps unload loop in Resources.Unload below.
	doorWoodTex := loadRepeatTexture(makeBarkPixels(64, 128), 64, 128)
	doorStoneTex := loadRepeatTexture(makeRockWallPixels(64, 64), 64, 64)
	r.doorProps[core.DoorStyleBuilding] = loadDoorProp(r.lighting.shader, doorWoodTex)
	r.doorProps[core.DoorStyleCave] = loadCaveDoorProp(r.lighting.shader, doorStoneTex)
	r.doorProps[core.DoorStyleField] = loadFieldDoorProp(r.lighting.shader, doorWoodTex)

	assertDecorCoverage(r.decorModels)
	assertPropCoverage(r.propModels)

	// Flatten the maps into [256]propModel tables so the per-tile draw
	// path is an array index instead of a map hash. Built once after
	// the maps are fully populated; the maps remain authoritative for
	// Unload + assertion paths.
	r.decorModelTable = flattenModelTable(r.decorModels)
	r.propModelTable = flattenModelTable(r.propModels)

	LogRenderInit("resources loaded: propModels=%d decorModels=%d doorProps=%d inlineProps=%d inlineDecor=%d",
		len(r.propModels), len(r.decorModels), len(r.doorProps), len(inlinePropHandlers), len(inlineDecorHandlers))
	tableNonEmpty := func(t *[256]propModel) int {
		c := 0
		for i := 0; i < 256; i++ {
			if len(t[i].parts) > 0 {
				c++
			}
		}
		return c
	}
	LogRenderInit("flattened tables: propModelTable entries=%d decorModelTable entries=%d", tableNonEmpty(r.propModelTable), tableNonEmpty(r.decorModelTable))

	committed = true
	return r
}

// flattenModelTable copies every (char, propModel) pair from the
// authored map into a heap-allocated [256]propModel and returns a
// pointer to it. Callers store the pointer on Resources so passing
// Resources by value remains cheap; the underlying array doesn't move
// after this returns.
func flattenModelTable(src map[byte]propModel) *[256]propModel {
	t := new([256]propModel)
	for c, pm := range src {
		t[c] = pm
	}
	return t
}

// inlineDecorRenderer is the per-tile drawing closure for a decor
// char rendered through a specialized path (dedicated propModel field
// on Resources, hand-tuned scatter helper) rather than the generic
// decorModels map. Same dispatch signature as the generic map keeps
// world.go's drawDecor uniform.
type inlineDecorRenderer func(assets Resources, x, z int, cx, cz float32)

// inlineDecorHandlers is the SINGLE source of truth for inline decor
// dispatch — assertDecorCoverage reads from it. Adding a new inline-
// rendered decor char is one map entry plus one helper function; the
// coverage assert and the renderer (via inlineDecorTable below) both
// pick it up without further edits. Replaces the older parallel
// char-set + open-coded switch in world.go that could drift silently.
var inlineDecorHandlers = map[byte]inlineDecorRenderer{
	core.DecorBush:     drawDecorBush,
	core.DecorMushroom: drawDecorMushroom,
	core.DecorPebble:   drawDecorPebble,
}

// inlineDecorTable is the [256]inlineDecorRenderer mirror of
// inlineDecorHandlers, indexed by the decor-layer char. The world-draw
// hot path probes one cell per tile per frame; at 64x64 maps that's
// 4096 lookups, so array index beats map hash. The map stays as the
// authored source of truth; this table is a generated mirror so adding
// a handler is still a single-place edit.
var inlineDecorTable = buildInlineDecorTable()

func buildInlineDecorTable() [256]inlineDecorRenderer {
	var t [256]inlineDecorRenderer
	for c, fn := range inlineDecorHandlers {
		t[c] = fn
	}
	return t
}

// assertDecorCoverage panics if any char in core.DecorTileChars is
// missing from decorModels AND not inline-handled. Mirrors the shape
// of render/minimap.go's init: catches "added a const, forgot to
// register a model" at boot instead of as a silent no-op in drawDecor.
func assertDecorCoverage(models map[byte]propModel) {
	for _, c := range core.DecorTileChars() {
		if _, ok := models[c]; ok {
			continue
		}
		if _, ok := inlineDecorHandlers[c]; ok {
			continue
		}
		// Multi-tile-decor tails (e.g. DecorArchwayTail) deliberately
		// have no decorModels entry — the anchor on the partner tile
		// draws the spanning mesh. DecorFootprintTail catches them.
		if isDecorFootprintTail(c) {
			continue
		}
		panic("render: decor char '" + string(c) + "' has no decorModels entry and is not inline-handled — register a loadXxxProp in NewResources or add to inlineDecorHandlers")
	}
}

// isDecorFootprintTail mirrors isFootprintTail for the decor layer:
// reports whether c is the tail char for a multi-tile decor anchor.
// Tails render nothing; the anchor's mesh covers them.
func isDecorFootprintTail(c byte) bool {
	for _, anchor := range core.DecorTileChars() {
		if core.DecorFootprintTail(anchor) == c {
			return true
		}
	}
	return false
}

// inlinePropRenderer is the per-tile drawing closure for a prop char
// rendered through a specialized path. Same shape as
// inlineDecorRenderer but tied to the props-layer dispatch — props
// pre-compute `propYawDeg` once per tile, so the handler takes it
// pre-resolved. `x, z` are the tile coords so handlers can seed
// per-tile shape variance (canopy fullness, scale jitter, tint walk)
// from `tileHash(x, z)` without rederiving coordinates from `center`.
type inlinePropRenderer func(assets Resources, m core.AreaDefinition, x, z int, center rl.Vector3, propYaw float32)

// inlinePropHandlers is the SINGLE source of truth for inline prop
// dispatch — both world.go's drawWorld switch and assertPropCoverage
// read from it. Tree / TreeXL share a tree model with different
// scales; the rest are single-prop wrappers around dedicated
// Resources fields. Adding a new inline-rendered prop is one map
// entry plus one helper function.
var inlinePropHandlers = map[byte]inlinePropRenderer{
	// The four scaled-tree variants share one closure factory keyed
	// by char (treePropScales table). drawPropTreeTwin stays separate
	// because it draws two instances per tile with offsets.
	core.TileTree:      drawPropTreeScaled(core.TileTree),
	core.TileTreeXL:    drawPropTreeScaled(core.TileTreeXL),
	core.TileTreeTall:  drawPropTreeScaled(core.TileTreeTall),
	core.TileTreeYoung: drawPropTreeScaled(core.TileTreeYoung),
	core.TileTreeTwin:  drawPropTreeTwin,
	core.TileRockLarge: drawPropRockLarge,
	core.TileBushLarge: drawPropBushLarge,
	// Wall torch — needs the area to find the adjacent wall, which
	// the inline signature now carries. drawWallTorch ignores the
	// propYaw (it derives its own facing from the wall).
	core.TileTorch: drawWallTorch,
}

// inlinePropTable is the [256]inlinePropRenderer index built once from
// inlinePropHandlers above. The world-draw hot path looks up tens of
// thousands of tiles per frame on a large map; array indexing skips
// the map hash + bounds check the map lookup needed. Nil entries
// mean "not an inline-handled prop" — caller falls through to the
// footprint / propModels paths as before. The map stays as the
// authored source of truth (assertPropCoverage reads it); the table
// is a generated mirror.
var inlinePropTable = buildInlinePropTable()

func buildInlinePropTable() [256]inlinePropRenderer {
	var t [256]inlinePropRenderer
	for c, fn := range inlinePropHandlers {
		t[c] = fn
	}
	return t
}

// assertPropCoverage is the props-layer analogue of
// assertDecorCoverage. PropTileChars enumerates every blocking prop
// char; each must either be in propModels or be inline-handled by
// inlinePropHandlers.
func assertPropCoverage(models map[byte]propModel) {
	for _, c := range core.PropTileChars() {
		if _, ok := models[c]; ok {
			continue
		}
		if _, ok := inlinePropHandlers[c]; ok {
			continue
		}
		// Multi-tile-prop tails (e.g. TileRockFormationTail) deliberately
		// have no propModels entry — the anchor on the partner tile draws
		// the spanning mesh. PropFootprintTail catches them.
		if isFootprintTail(c) {
			continue
		}
		panic("render: prop char '" + string(c) + "' has no propModels entry and is not inline-handled — register a loadXxxProp in NewResources or add to inlinePropHandlers")
	}
}

// isFootprintTail reports whether c is a tail char for a multi-tile
// prop anchor. Tails render nothing; the anchor's mesh covers them.
// Walks the known anchor set instead of a hand-maintained tail list
// so adding a future multi-tile prop only touches PropFootprint.
func isFootprintTail(c byte) bool {
	for _, anchor := range core.PropTileChars() {
		if core.PropFootprintTail(anchor) == c {
			return true
		}
	}
	return false
}

func (r Resources) Unload() {
	// UnloadModel walks the model's materials and unloads each map's texture,
	// so wall/floor textures are freed here — no separate UnloadTexture call.
	for _, material := range r.materials {
		rl.UnloadModel(material.wallModel)
		rl.UnloadModel(material.floorModel)
		rl.UnloadModel(material.ceilingModel)
		// Unconditional, NOT gated on hasFloorVariant: UnloadModel on a
		// zero-ID handle skips cleanly, and decoupling cleanup from the
		// draw-path flag means a partial init (one variant built, the
		// second panicking) still frees the model that did load.
		rl.UnloadModel(material.floorDirtModel)
		rl.UnloadModel(material.floorDarkModel)
	}
	rl.UnloadTexture(r.skyTexture)
	rl.UnloadTexture(r.starTexture)
	// Walk enemyTextures (the owning list), NOT enemyVisuals — the map
	// aliases the same handle at multiple keys (placeholder sprites for
	// the new monsters) and iterating it would double-free every shared
	// texture at game exit.
	for _, tex := range r.enemyTextures {
		rl.UnloadTexture(tex)
	}
	for _, texture := range r.partyTexture {
		rl.UnloadTexture(texture)
	}
	r.tree.unload()
	r.rockProp.unload()
	r.bushProp.unload()
	r.mushroomProp.unload()
	r.chestBody.unload()
	r.chestLid.unload()
	if groundShadowReady {
		rl.UnloadModel(groundShadowModel)
		groundShadowReady = false
	}
	if torchFlameReady {
		rl.UnloadModel(torchFlameModel)
		torchFlameReady = false
	}
	for i := range r.doorProps {
		r.doorProps[i].unload()
	}
	for _, model := range r.specialFloors {
		rl.UnloadModel(model)
	}
	for _, p := range r.decorModels {
		p.unload()
	}
	for _, p := range r.propModels {
		p.unload()
	}
	r.lighting.unload()
	r.billboardFog.unload()
	if r.hudFontOwned {
		rl.UnloadFont(r.hudFont)
	}
}

// loadTiledTexture is the shared pipeline for "128x128 RGBA pixels →
// mipmapped, trilinear-filtered, repeat-wrapped texture." Used by every
// tile-class texture (walls, floor variants, prop diffuse), so the
// filter/wrap settings stay identical across the world.
func loadTiledTexture(pixels []color.RGBA) rl.Texture2D {
	tex := loadTexture(pixels, 128, 128, rl.FilterBilinear)
	rl.GenTextureMipmaps(&tex)
	rl.SetTextureFilter(tex, rl.FilterTrilinear)
	rl.SetTextureWrap(tex, rl.WrapRepeat)
	return tex
}

// loadRepeatTexture is the sized variant of loadTiledTexture for textures
// that aren't 128x128. Bark (64x128) and leaf (96x96) are the existing
// callers — the function exists so each one doesn't repeat the bilinear-
// filter + repeat-wrap boilerplate inline.
func loadRepeatTexture(pixels []color.RGBA, width, height int) rl.Texture2D {
	tex := loadTexture(pixels, width, height, rl.FilterBilinear)
	rl.SetTextureWrap(tex, rl.WrapRepeat)
	return tex
}

// loadTileModel builds one tile-sized cube model (TileSize × height ×
// TileSize), textures it with the given pixels via loadTiledTexture, and
// binds the lighting shader. Used for both wall cubes and floor cubes —
// only the height differs.
func loadTileModel(pixels []color.RGBA, height float32, shader rl.Shader) rl.Model {
	tex := loadTiledTexture(pixels)
	model := rl.LoadModelFromMesh(rl.GenMeshCube(core.TileSize, height, core.TileSize))
	setModelTexture(&model, tex)
	attachShader(&model, shader)
	return model
}

// loadFloorModel is the alt-floor variant entry point (dirt / dark grass).
// Floor cubes use a flat 0.06 height so the wall geometry overlaps cleanly.
func loadFloorModel(pixels []color.RGBA, shader rl.Shader) rl.Model {
	return loadTileModel(pixels, 0.06, shader)
}

// loadGroundShadowModel builds the soft contact-shadow plane: a 1×1
// XZ quad textured with the radial-gradient shadow sprite. The
// texture is clamped (the gradient is fully transparent at the
// disc edge anyway) and bilinear-filtered so the falloff stays
// smooth when the disc is scaled up under a big prop. NO lighting
// shader is attached — the model keeps raylib's default material
// shader so DrawModelEx renders it unlit even when the world's
// lighting shader is the active BeginShaderMode shader, leaving the
// shadow a flat dark wash with the texture's own alpha gradient.
func loadGroundShadowModel() rl.Model {
	tex := loadTexture(makeSoftShadowPixels(64), 64, 64, rl.FilterBilinear)
	rl.SetTextureWrap(tex, rl.WrapClamp)
	model := rl.LoadModelFromMesh(rl.GenMeshPlane(1, 1, 1, 1))
	setModelTexture(&model, tex)
	return model
}

func loadWorldMaterial(wallPixels, floorPixels []color.RGBA, shader rl.Shader) worldMaterialResources {
	return worldMaterialResources{
		wallModel:  loadTileModel(wallPixels, core.WallHeight, shader),
		floorModel: loadFloorModel(floorPixels, shader),
		// Ceiling reuses the wall texture but at a thin (0.2) slab height
		// so its bottom face shows beneath the player while the top sits
		// just above WallHeight. Same shader so the lighting profile and
		// time-of-day uniforms apply consistently with walls and floors.
		ceilingModel: loadTileModel(wallPixels, 0.2, shader),
	}
}

// Font returns the HUD font, used by scenes outside the main render package
// (title screen, editor) for consistent typography.
func (r Resources) Font() rl.Font { return r.hudFont }

func (r Resources) worldMaterial(material core.MaterialSet) worldMaterialResources {
	if resources, ok := r.materials[material]; ok {
		return resources
	}
	return r.materials[core.MaterialField]
}

// assertMaterialCoverage panics unless every core.MaterialSet has a loaded
// worldMaterial entry. Mirrors the assertDecorCoverage / assertPropCoverage
// init-time contracts AGENTS.md documents so a new material can't silently
// render as Field via worldMaterial's fallback.
func assertMaterialCoverage(materials map[core.MaterialSet]worldMaterialResources) {
	for i := 0; i < int(core.MaterialCount); i++ {
		mat := core.MaterialSet(i)
		if _, ok := materials[mat]; !ok {
			name, _ := core.MaterialName(mat)
			panic("render: material " + name + " has no worldMaterialResources — load it in NewResources")
		}
	}
}

// lightingFor picks the per-area lighting profile. Profiles are package-level
// constants in lighting.go — this just routes by material.
func lightingFor(material core.MaterialSet) lightingProfile {
	if core.MaterialIsIndoor(material) {
		return dungeonLighting
	}
	return fieldLighting
}

// loadEnemySprite is the per-enemy texture-creation helper. Mints a
// point-filtered, clamped sprite texture from the given pixel slice and
// appends it to `owned` so Resources.Unload can free it at shutdown.
// Centralizes the "create + filter + clamp + register for cleanup"
// boilerplate that every new enemy texture used to repeat — adding a
// new sprite is now one line instead of three, and forgetting to
// register the texture for cleanup is impossible.
func loadEnemySprite(pixels []color.RGBA, w, h int, owned *[]rl.Texture2D) rl.Texture2D {
	tex := loadTexture(pixels, w, h, rl.FilterPoint)
	rl.SetTextureWrap(tex, rl.WrapClamp)
	*owned = append(*owned, tex)
	return tex
}

func loadEnemyVisuals() (map[core.EnemyKind]enemyVisual, []rl.Texture2D) {
	var owned []rl.Texture2D
	ratTexture := loadEnemySprite(makeRatPixels(72, 96), 72, 96, &owned)
	batTexture := loadEnemySprite(makeBatPixels(80, 88), 80, 88, &owned)
	diseasedRatTexture := loadEnemySprite(makeDiseasedRatPixels(72, 96), 72, 96, &owned)
	goblinTexture := loadEnemySprite(makeGoblinPixels(72, 112), 72, 112, &owned)
	goblinMageTexture := loadEnemySprite(makeGoblinMagePixels(72, 112), 72, 112, &owned)
	amoebaTexture := loadEnemySprite(makeAmoebaPixels(96, 80), 96, 80, &owned)
	mantrapTexture := loadEnemySprite(makeVenusMantrapPixels(88, 128), 88, 128, &owned)
	// Roster-expansion sprites. Dimensions sized to each kind's
	// silhouette role: spider is wide+short, golem is the biggest
	// (tier-5 anchor), wisp is narrow+tall (floating), bat/necro/
	// skeleton are humanoid-ish at ~72×112.
	caveSpiderTexture := loadEnemySprite(makeCaveSpiderPixels(88, 72), 88, 72, &owned)
	vampireBatTexture := loadEnemySprite(makeVampireBatPixels(96, 88), 96, 88, &owned)
	wispTexture := loadEnemySprite(makeWispPixels(56, 72), 56, 72, &owned)
	stoneGolemTexture := loadEnemySprite(makeStoneGolemPixels(96, 120), 96, 120, &owned)
	necromancerTexture := loadEnemySprite(makeNecromancerPixels(72, 112), 72, 112, &owned)
	skeletonTexture := loadEnemySprite(makeSkeletonPixels(72, 112), 72, 112, &owned)
	visuals := map[core.EnemyKind]enemyVisual{
		core.EnemyRat: {
			texture: ratTexture,
			size:    rl.NewVector2(0.82, 1.22),
		},
		core.EnemyBat: {
			texture: batTexture,
			// Wider than tall: bat reads as a wing-spread silhouette rather
			// than a vertical pillar like the rat. Width is kept just under
			// the multi-enemy slot spacing in enemyDrawPosition (1.12) so
			// adjacent bats in three-bat encounters don't merge silhouettes.
			size: rl.NewVector2(0.98, 0.84),
		},
		core.EnemyDiseasedRat: {
			texture: diseasedRatTexture,
			// Slightly bulkier silhouette than a clean rat — sells the
			// "meaner, bloated, sicker" read at a glance.
			size: rl.NewVector2(0.92, 1.30),
		},
		core.EnemyGoblin: {
			texture: goblinTexture,
			// Stockier, taller than a rat — humanoid frame with a club.
			size: rl.NewVector2(1.05, 1.55),
		},
		core.EnemyGoblinMage: {
			texture: goblinMageTexture,
			// Robed silhouette is slightly slimmer than the goblin but the
			// staff bumps the read up another half-tile so the caster looks
			// the part.
			size: rl.NewVector2(1.10, 1.65),
		},
		core.EnemyAmoeba: {
			texture: amoebaTexture,
			// Squat and wide — the blob is closer to a wet pancake than a
			// pillar. Width nudged up so it visually fills the tile.
			size: rl.NewVector2(1.20, 0.95),
		},
		core.EnemyVenusMantrap: {
			texture: mantrapTexture,
			// Taller than the goblin family — the open trap-jaw silhouette
			// is the showpiece, so the sprite needs the vertical room.
			size: rl.NewVector2(1.20, 1.80),
		},
		// Roster expansion — each new kind now has a dedicated sprite
		// painted in makeXxxPixels (textures.go). Sizes tuned to each
		// silhouette's identity rather than copying a placeholder
		// alias's ratio.
		core.EnemyCaveSpider: {
			texture: caveSpiderTexture,
			// Wide + low — the abdomen-front silhouette reads as
			// "ground-skittering ambusher" at any zoom.
			size: rl.NewVector2(1.10, 0.95),
		},
		core.EnemyVampireBat: {
			texture: vampireBatTexture,
			// Slightly larger than the cave bat so the wingspan
			// reads as the upgraded variant at a glance.
			size: rl.NewVector2(1.12, 1.00),
		},
		core.EnemyWisp: {
			texture: wispTexture,
			// Narrow + tall — sells the floating-orb identity and
			// distinguishes the silhouette from the goblin family.
			size: rl.NewVector2(0.65, 0.92),
		},
		core.EnemyStoneGolem: {
			texture: stoneGolemTexture,
			// Biggest in the roster — anchors a pack and reads as
			// a tier-5 threat across the arena.
			size: rl.NewVector2(1.55, 1.95),
		},
		core.EnemyNecromancer: {
			texture: necromancerTexture,
			// Tall robed silhouette — distinct from the goblin mage
			// (stouter robe + bone-staff vs orb-staff).
			size: rl.NewVector2(1.05, 1.70),
		},
		core.EnemySkeleton: {
			texture: skeletonTexture,
			// Tier-2 grunt — sized to match the goblin family so
			// mixed packs read as a unified front line.
			size: rl.NewVector2(0.95, 1.50),
		},
	}
	// Coverage assert: every EnemyKind registered in core must have a
	// visual entry here. Without this, a new enemy silently rendered as
	// a rat via enemyVisualFor's fallback. Mirrors the editor's
	// entityBrushColors init check.
	for _, def := range core.EnemyKinds() {
		v, ok := visuals[def.Kind]
		if !ok || v.texture.ID == 0 {
			panic("render: missing enemyVisuals entry for " + def.Name + " — author a sprite and register it in loadEnemyVisuals")
		}
	}
	return visuals, owned
}

// hudFontCodepoints is the rune set baked into the HUD font atlas.
// LoadFontEx(path, size, nil) only loads the default ASCII range
// (32..126); anything outside that — arrows, triangles, ×, °, ±, é —
// renders as a missing-glyph box that reads on screen as "?". We
// pre-enumerate every non-ASCII rune the codebase actually uses
// (turn-order arrows, pack-edit ▲/▼/×, action-row ▶, stat ±, etc.)
// plus standard ASCII so the atlas covers everything the renderer
// might pass at runtime.
//
// Adding a new symbol to the HUD requires extending this list — the
// font atlas is built once at LoadResources, so missing entries
// silently fall back to "?".
var hudFontCodepoints = buildHUDFontCodepoints()

func buildHUDFontCodepoints() []rune {
	runes := make([]rune, 0, 128)
	// Standard printable ASCII (space through tilde).
	for r := rune(32); r <= 126; r++ {
		runes = append(runes, r)
	}
	// Extras used across HUD, editor, and combat surfaces. Source for
	// each: enumerated from the codebase's non-ASCII runes; comment
	// describes where it appears so future readers can extend with
	// confidence.
	runes = append(runes,
		'°', // degrees — debug overlay, facing readouts
		'±', // stat deltas (level-up modal, future tuning UI)
		'×', // pack-size badge ("×4"), pack-edit remove button
		'é', // foreign names ("Mêlée" if ever added; here for cushion)
		'–', // en-dash (modal headers / range "1–10")
		'—', // em-dash (long-form labels)
		'…', // ellipsis (truncated names / "loading…")
		'←', // editor pan / movement hints
		'↑', // turn-order / list cursor
		'→', // action arrows ("Attack → Rat")
		'↓', // turn-order / list cursor
		'↔', // bidirectional links (door pair)
		'∈', // set membership (future stat / skill notation)
		'−', // unicode minus (distinct from ASCII hyphen)
		'≈', // approximate (timing-grade hints)
		'≤', // less-than-or-equal (thresholds)
		'▲', // pack-edit reorder up, list arrows
		'▶', // action-row submenu indicator
		'▼', // pack-edit reorder down, list arrows
		'●', // bullet / active marker
	)
	return runes
}

func loadHUDFont() (rl.Font, bool) {
	// Try a per-OS list of well-known system font paths. First valid hit wins.
	// Fallback is raylib's bitmap default font, which has different metrics —
	// good for "ran on a server with no fonts," not great as a target.
	for _, path := range systemFontCandidates() {
		if _, err := os.Stat(path); err != nil {
			continue
		}
		// 64 pt bake: the five UI_STANDARDS.md sizes (13/16/20/26/36)
		// all downsample from this without subpixel mush. The previous
		// 32 pt bake was sharp at Body (20) but slightly soft at
		// Heading (26) and noticeably blurry at Title (36).
		font := rl.LoadFontEx(path, 64, hudFontCodepoints)
		if rl.IsFontValid(font) {
			// Mipmaps + trilinear so the small UI sizes (Tiny 13 / Small
			// 16 / Body 20) downsample from the 64 pt atlas smoothly
			// instead of aliasing into hard, "pixely" edges — plain
			// bilinear only samples the base level's 2×2 neighbourhood,
			// which shimmers and jaggies under heavy minification. Mirrors
			// the sky texture's mipmap+trilinear setup above; the 64 pt
			// bake's glyph padding absorbs the minor cross-glyph bleed the
			// lower mips introduce.
			rl.GenTextureMipmaps(&font.Texture)
			rl.SetTextureFilter(font.Texture, rl.FilterTrilinear)
			return font, true
		}
	}
	return rl.GetFontDefault(), false
}

// systemFontCandidates returns the per-OS preferred-font paths in
// priority order. The Library aesthetic (see UI_STANDARDS.md "Type")
// uses a refined SERIF face — Constantia is the first choice on
// Windows because it was specifically designed for body copy at
// small sizes; the rest of the chain falls back to other readable
// serifs and finally to the platform's default sans if none exist.
// LoadFontEx is called with a 64 pt bake so the five standard sizes
// (Tiny/Small/Body/Heading/Title) all render sharp.
func systemFontCandidates() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{
			`C:\Windows\Fonts\constan.ttf`,  // Constantia — refined serif, the headline choice
			`C:\Windows\Fonts\cambria.ttc`,  // Cambria — second-best serif
			`C:\Windows\Fonts\georgia.ttf`,  // Georgia — classic web serif fallback
			`C:\Windows\Fonts\pala.ttf`,     // Palatino Linotype — older alternate
			`C:\Windows\Fonts\seguisb.ttf`,  // Segoe UI Semibold — final sans fallback
		}
	case "darwin":
		return []string{
			"/System/Library/Fonts/Supplemental/Georgia.ttf",
			"/Library/Fonts/Georgia.ttf",
			"/System/Library/Fonts/Supplemental/Palatino.ttc",
			"/System/Library/Fonts/Times.ttc",
			"/System/Library/Fonts/SFNS.ttf",
		}
	default: // linux / *bsd / others — try the most common distro paths.
		return []string{
			"/usr/share/fonts/truetype/dejavu/DejaVuSerif.ttf",
			"/usr/share/fonts/truetype/dejavu/DejaVuSerif-Bold.ttf",
			"/usr/share/fonts/truetype/liberation/LiberationSerif-Regular.ttf",
			"/usr/share/fonts/truetype/liberation/LiberationSerif-Bold.ttf",
			"/usr/share/fonts/noto/NotoSerif-Regular.ttf",
			"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf", // final sans fallback
		}
	}
}

func setModelTexture(model *rl.Model, texture rl.Texture2D) {
	materials := model.GetMaterials()
	if len(materials) == 0 {
		return
	}
	rl.SetMaterialTexture(&materials[0], rl.MapDiffuse, texture)
}

func loadTexture(pixels []color.RGBA, width, height int, filter rl.TextureFilterMode) rl.Texture2D {
	img := rl.GenImageColor(width, height, rl.White)
	texture := rl.LoadTextureFromImage(img)
	rl.UnloadImage(img)
	rl.UpdateTexture(texture, pixels)
	rl.SetTextureFilter(texture, filter)
	rl.SetTextureWrap(texture, rl.WrapRepeat)
	return texture
}

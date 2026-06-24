package render

import (
	"crawler/internal/app/core"
	_ "embed"
	rl "github.com/gen2brain/raylib-go/raylib"
	"image/color"
	"os"
	"path/filepath"
	"runtime"
	"unsafe"
)

// uiFontTTF is the embedded UI face (Della Respira, SIL OFL 1.1). Missing glyphs are drawn procedurally (richtext.go).
//
//go:embed fonts/DellaRespira-Regular.ttf
var uiFontTTF []byte

type Resources struct {
	materials  map[core.MaterialSet]worldMaterialResources
	skyTexture rl.Texture2D
	// starTexture: night overlay; alpha follows time-of-day (timeProfile.StarAlpha).
	starTexture rl.Texture2D
	// enemyVisuals: kind→billboard, dense slice indexed by EnemyKind. Aliases textures, so
	// enemyTextures is the owning list Unload walks. Slice (not array) so editor live-preview propagates.
	enemyVisuals  []enemyVisual
	enemyTextures []rl.Texture2D
	// partyVisuals: class→billboard, symmetric with enemyVisuals; partyTextures owns the handles.
	partyVisuals  []enemyVisual
	partyTextures []rl.Texture2D
	hudFont       rl.Font
	hudFontOwned  bool

	lighting     lightingShader
	billboardFog billboardFogShaderPipe
	tree         treeModel

	// Field-only props, scattered as blockers (large) or decoration (small).
	rockProp     propModel
	bushProp     propModel
	mushroomProp propModel
	// chestBody / chestLid: two parts — the lid draws only on a closed chest; an
	// open/looted chest drops it for a dark open mouth (drawChestOpenMouth).
	chestBody propModel
	chestLid  propModel

	// Universal floor variants keyed by floor-layer char, shared across materials.
	specialFloors map[byte]rl.Model
	// specialFloorTable: char-indexed hot-path mirror; map stays authoritative for Unload.
	specialFloorTable *modelTable256

	// Face variants keyed by face-skin char (ivy / cracked / crumbling). Plain TileRock uses the per-material face.
	faceVariants map[byte]rl.Model
	// faceVariantTable: char-indexed mirror; map stays authoritative for Unload.
	faceVariantTable *modelTable256

	// rampModel: solid earth-textured wedge for ramp tiles, sized one tile × LevelStep to meet both floors flush.
	rampModel rl.Model

	// underModel: downward -Y quad for floating-cube undersides (voxel maps). See drawVoxelColumn in voxel.go.
	underModel rl.Model

	// decorModels keyed by decor-layer char; drives authoring + Unload. decorModelTable is the hot-path mirror.
	decorModels map[byte]propModel
	// decorModelTable: [256] mirror by pointer (cheap by-value Resources copy). "Registered" = len(parts) > 0.
	decorModelTable *[256]propModel

	// propModels keyed by props-layer char; renderer falls back to tree/boulder/bush when a char isn't here.
	propModels map[byte]propModel
	// propModelTable: [256] mirror by pointer; map remains for authoring and Unload.
	propModelTable *[256]propModel

	// doorProps: one model per core.DoorStyle; DrawDoors indexes by Style and rotates by authored facing.
	doorProps [core.DoorStyleCount]propModel
}

type worldMaterialResources struct {
	// Each model owns its diffuse texture via its material; Unload via UnloadModel only.
	faceModel    rl.Model // per-material plain-rock cliff-face quad
	floorModel   rl.Model
	ceilingModel rl.Model // thin wall-textured slab over ceiling-flagged tiles
	// Field-only secondary floor variants, picked per-tile by hash. Empty for dungeon.
	floorDirtModel  rl.Model
	floorDarkModel  rl.Model
	hasFloorVariant bool
}

// LoadResources builds every procedural texture/model/font/shader. Each handle is committed to
// `r` as created; a deferred recover() calls r.Unload() and re-panics so a mid-load failure leaks nothing.
func LoadResources() (r Resources) {
	committed := false
	defer func() {
		if committed {
			return
		}
		if rec := recover(); rec != nil {
			// r.Unload tolerates zero values (nil-map ranges no-op; UnloadModel/Texture skip on id == 0).
			r.Unload()
			panic(rec)
		}
	}()

	r.lighting = loadLightingShader()
	r.billboardFog = loadBillboardFogShader()

	// Commit each material the instant it's built so the recover-path Unload finds earlier models on a later panic.
	dungeonMat := loadWorldMaterial(makeStoneBrickPixels(128, 128), makeStoneFloorPixels(128, 128), r.lighting.shader)
	r.materials = map[core.MaterialSet]worldMaterialResources{
		core.MaterialDungeon: dungeonMat,
	}
	fieldMat := loadWorldMaterial(makeRockWallPixels(128, 128), makeGrassPixels(128, 128), r.lighting.shader)
	r.materials[core.MaterialField] = fieldMat
	// Field's two extra floor variants (dirt + dark grass), per-tile by hash. Commit after EACH for Unload.
	fieldMat.floorDirtModel = loadFloorModel(makeDirtPixels(128, 128), r.lighting.shader)
	r.materials[core.MaterialField] = fieldMat
	fieldMat.floorDarkModel = loadFloorModel(makeDarkGrassPixels(128, 128), r.lighting.shader)
	fieldMat.hasFloorVariant = true
	r.materials[core.MaterialField] = fieldMat
	// Every core.MaterialSet must have a loaded worldMaterial, else worldMaterial() falls back to Field.
	assertMaterialCoverage(r.materials)

	r.skyTexture = loadTexture(makeSkyPixels(1024, 512), 1024, 512, rl.FilterTrilinear)
	rl.GenTextureMipmaps(&r.skyTexture)
	rl.SetTextureFilter(r.skyTexture, rl.FilterTrilinear)
	rl.SetTextureWrap(r.skyTexture, rl.WrapClamp)

	// Star overlay: same dims as the sky (shared source-rect math); point-filtered so stars don't blur.
	r.starTexture = loadTexture(makeStarPixels(1024, 512), 1024, 512, rl.FilterPoint)
	rl.SetTextureWrap(r.starTexture, rl.WrapClamp)

	enemyVis, enemyTex := loadEnemyVisuals()
	r.enemyVisuals, r.enemyTextures = enemyVisualsToSlice(enemyVis), enemyTex

	partyVis, partyTex := loadPartyVisuals()
	r.partyVisuals, r.partyTextures = partyVisualsToSlice(partyVis), partyTex

	r.hudFont, r.hudFontOwned = loadHUDFont()

	// Bark/leaf are non-tile sizes → loadRepeatTexture. The model takes ownership (UnloadModel frees them),
	// so they're NOT in an owned list (would double-free). A panic before ownership orphans them — accepted.
	barkTex := loadRepeatTexture(makeBarkPixels(64, 128), 64, 128)
	leafTex := loadRepeatTexture(makeLeafPixels(96, 96), 96, 96)
	r.tree = loadTreeModel(r.lighting.shader, barkTex, leafTex)

	// Each field prop gets its OWN texture instance. UnloadModel frees meshes but NOT bound GL textures
	// (held until process exit; Unload runs once). Per-prop instancing avoids a shared-texture double-free.
	rockTex := loadTiledTexture(makeRockWallPixels(128, 128))
	bushTex := loadRepeatTexture(makeLeafPixels(96, 96), 96, 96)

	r.rockProp = loadRockProp(r.lighting.shader, rockTex)
	r.bushProp = loadBushProp(r.lighting.shader, bushTex)
	r.mushroomProp = loadMushroomProp(r.lighting.shader)
	// Chest body + lid mint distinct bark instances so each model owns its texture.
	chestBodyWoodTex := loadRepeatTexture(makeBarkPixels(64, 128), 64, 128)
	chestLidWoodTex := loadRepeatTexture(makeBarkPixels(64, 128), 64, 128)
	r.chestBody = loadChestBodyProp(r.lighting.shader, chestBodyWoodTex)
	r.chestLid = loadChestLidProp(r.lighting.shader, chestLidWoodTex)

	// Soft ground-shadow disc, drawn UNLIT. Package singleton (drawGroundShadow is a free function).
	groundShadowModel = loadGroundShadowModel()
	groundShadowReady = true

	// HUD glass-grain overlay (drawGlassRelief). Package singleton like the shadow disc.
	hudGrainTex = loadTexture(makeHudGrainPixels(64, 64), 64, 64, rl.FilterBilinear)
	hudGrainReady = true

	// Wall-torch flame — unlit emissive sphere, animated by drawWallTorch.
	torchFlameModel = rl.LoadModelFromMesh(rl.GenMeshSphere(1, 8, 10))
	torchFlameReady = true

	// Universal floor variants, shared across material sets. Init the map first for the panic path.
	r.specialFloors = make(map[byte]rl.Model)
	r.specialFloors[core.FloorCobble] = loadFloorModel(makeCobblePixels(128, 128), r.lighting.shader)
	r.specialFloors[core.FloorPlank] = loadFloorModel(makePlankPixels(128, 128), r.lighting.shader)
	r.specialFloors[core.FloorWater] = loadFloorModel(makeWaterPixels(128, 128), r.lighting.shader)
	r.specialFloors[core.FloorDeepWater] = loadFloorModel(makeDeepWaterPixels(128, 128), r.lighting.shader)
	r.specialFloors[core.FloorSand] = loadFloorModel(makeSandPixels(128, 128), r.lighting.shader)
	r.specialFloors[core.FloorSnow] = loadFloorModel(makeSnowPixels(128, 128), r.lighting.shader)
	// Grass/dirt/dark grass/stone are also universal — without these, painting them in a dungeon
	// reuses the base floorModel (per-material variant switch only fires on hasFloorVariant, field-only).
	r.specialFloors[core.FloorGrass] = loadFloorModel(makeGrassPixels(128, 128), r.lighting.shader)
	r.specialFloors[core.FloorDirt] = loadFloorModel(makeDirtPixels(128, 128), r.lighting.shader)
	r.specialFloors[core.FloorDarkGrass] = loadFloorModel(makeDarkGrassPixels(128, 128), r.lighting.shader)
	r.specialFloors[core.FloorStone] = loadFloorModel(makeStoneFloorPixels(128, 128), r.lighting.shader)
	// Face variants — cliff-face quads with per-char rock skins (ivy / cracked / crumbling). Init first for panic cleanup.
	r.faceVariants = make(map[byte]rl.Model)
	r.faceVariants[core.TileWallRockIvyLight] = buildFaceQuadModel(makeRockIvyPixels(128, 128, false), r.lighting.shader)
	r.faceVariants[core.TileWallRockIvyHeavy] = buildFaceQuadModel(makeRockIvyPixels(128, 128, true), r.lighting.shader)
	r.faceVariants[core.TileWallRockCracked] = buildFaceQuadModel(makeRockCrackedPixels(128, 128), r.lighting.shader)
	r.faceVariants[core.TileWallRockCrumbling] = buildFaceQuadModel(makeRockCrumblingPixels(128, 128), r.lighting.shader)
	// Coverage: every non-rock skin in core.FaceSkins needs a variant here (plain Rock uses faceModel).
	for _, s := range core.FaceSkins {
		if s.Char == core.TileRock {
			continue
		}
		if _, ok := r.faceVariants[s.Char]; !ok {
			panic("render: face skin '" + string(s.Char) + "' in core.FaceSkins has no faceVariants model")
		}
	}

	// Earth-textured solid ramp wedge, shared by every ramp floor tile.
	r.rampModel = buildRampWedgeModel(makeDirtPixels(128, 128), r.lighting.shader)
	// Earth-textured downward quad for floating-cube undersides (voxel maps).
	r.underModel = buildUnderQuadModel(makeDirtPixels(128, 128), r.lighting.shader)

	// Stone family textures; each loader owns its handle via setModelTexture.
	marbleTex := loadTiledTexture(makeMarblePixels(128, 128))
	graniteTex := loadTiledTexture(makeGranitePixels(128, 128))
	terracottaTex := loadTiledTexture(makeTerracottaPixels(128, 128))
	// Wood/leaf props mint fresh texture instances per loader (sharing would double-unload).
	crateWoodTex := loadRepeatTexture(makeBarkPixels(64, 128), 64, 128)
	barrelWoodTex := loadRepeatTexture(makeBarkPixels(64, 128), 64, 128)
	stumpBarkTex := loadRepeatTexture(makeBarkPixels(64, 128), 64, 128)
	logBarkTex := loadRepeatTexture(makeBarkPixels(64, 128), 64, 128)
	logMossTex := loadRepeatTexture(makeLeafPixels(96, 96), 96, 96)
	leafPileTex := loadRepeatTexture(makeLeafPixels(96, 96), 96, 96)

	// Commit propModels incrementally so each is owned by r before the next loads.
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

	// Larger rock formations, each with its own rock texture instance.
	cairnRockTex := loadTiledTexture(makeRockWallPixels(128, 128))
	formationRockTex := loadTiledTexture(makeRockWallPixels(128, 128))
	r.propModels[core.TileRockCairn] = loadRockCairnProp(r.lighting.shader, cairnRockTex)
	r.propModels[core.TileRockFormation] = loadRockFormationProp(r.lighting.shader, formationRockTex)

	// Turn B outdoor batch — well/gravestone use rock, signpost/scarecrow bark.
	wellRockTex := loadTiledTexture(makeRockWallPixels(128, 128))
	graveRockTex := loadTiledTexture(makeRockWallPixels(128, 128))
	signWoodTex := loadRepeatTexture(makeBarkPixels(64, 128), 64, 128)
	scarecrowWoodTex := loadRepeatTexture(makeBarkPixels(64, 128), 64, 128)
	r.propModels[core.TileWell] = loadWellProp(r.lighting.shader, wellRockTex)
	r.propModels[core.TileGravestone] = loadGravestoneProp(r.lighting.shader, graveRockTex)
	r.propModels[core.TileSignPost] = loadSignPostProp(r.lighting.shader, signWoodTex)
	r.propModels[core.TileHayBale] = loadHayBaleProp(r.lighting.shader)
	r.propModels[core.TileScarecrow] = loadScarecrowProp(r.lighting.shader, scarecrowWoodTex)

	// Turn B dungeon-interior batch — bookshelf/table/bed wood, brazier shader-
	// only, sarcophagus marble.
	bookshelfWoodTex := loadRepeatTexture(makeBarkPixels(64, 128), 64, 128)
	tableWoodTex := loadRepeatTexture(makeBarkPixels(64, 128), 64, 128)
	bedWoodTex := loadRepeatTexture(makeBarkPixels(64, 128), 64, 128)
	sarcoMarbleTex := loadTiledTexture(makeMarblePixels(128, 128))
	r.propModels[core.TileBookshelf] = loadBookshelfProp(r.lighting.shader, bookshelfWoodTex)
	r.propModels[core.TileTable] = loadTableProp(r.lighting.shader, tableWoodTex)
	r.propModels[core.TileBed] = loadBedProp(r.lighting.shader, bedWoodTex)
	r.propModels[core.TileBrazier] = loadBrazierProp(r.lighting.shader)
	r.propModels[core.TileSarcophagus] = loadSarcophagusProp(r.lighting.shader, sarcoMarbleTex)

	// Non-blocking decorative plant props (shader-only); don't block movement (core.PropIsNonBlocking).
	r.propModels[core.TilePropExoticFlower] = loadExoticFlowerProp(r.lighting.shader)
	r.propModels[core.TilePropTallFern] = loadTallFernProp(r.lighting.shader)
	r.propModels[core.TilePropGrassTuft] = loadGrassTuftProp(r.lighting.shader)

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
	// Archway uses marble to match the pillars/statues.
	archMarbleTex := loadTiledTexture(makeMarblePixels(128, 128))
	r.decorModels[core.DecorArchway] = loadArchwayDecor(r.lighting.shader, archMarbleTex)

	// Turn B atmospheric decor — most are shader-only; rootCluster uses bark.
	rootBarkTex := loadRepeatTexture(makeBarkPixels(64, 128), 64, 128)
	r.decorModels[core.DecorRug] = loadRugProp(r.lighting.shader)
	r.decorModels[core.DecorCandle] = loadCandleProp(r.lighting.shader)
	r.decorModels[core.DecorBootprints] = loadBootprintsProp(r.lighting.shader)
	r.decorModels[core.DecorAshHeap] = loadAshHeapProp(r.lighting.shader)
	r.decorModels[core.DecorPuddle] = loadPuddleProp(r.lighting.shader)
	r.decorModels[core.DecorRootCluster] = loadRootClusterProp(r.lighting.shader, rootBarkTex)

	// Door props — one per style, rotated by authored facing. Each owns its texture.
	doorWoodTex := loadRepeatTexture(makeBarkPixels(64, 128), 64, 128)
	doorStoneTex := loadRepeatTexture(makeRockWallPixels(64, 64), 64, 64)
	r.doorProps[core.DoorStyleBuilding] = loadDoorProp(r.lighting.shader, doorWoodTex)
	r.doorProps[core.DoorStyleCave] = loadCaveDoorProp(r.lighting.shader, doorStoneTex)
	r.doorProps[core.DoorStyleField] = loadFieldDoorProp(r.lighting.shader, doorWoodTex)

	assertDecorCoverage(r.decorModels)
	assertPropCoverage(r.propModels)
	assertDoorProps(r.doorProps)

	// Flatten the maps into [256] tables for array-indexed per-tile dispatch; maps stay authoritative.
	r.decorModelTable = flattenModelTable(r.decorModels)
	r.propModelTable = flattenModelTable(r.propModels)
	r.specialFloorTable = flattenRLModelTable(r.specialFloors)
	r.faceVariantTable = flattenRLModelTable(r.faceVariants)

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

// flattenModelTable copies the map into a heap [256]propModel (by pointer; cheap by-value Resources copy).
func flattenModelTable(src map[byte]propModel) *[256]propModel {
	t := new([256]propModel)
	for c, pm := range src {
		t[c] = pm
	}
	return t
}

// modelTable256 is a char-indexed [256]rl.Model mirror with a parallel presence bitmap
// (rl.Model has no cheap "absent" sentinel), for array-indexed floor / cliff-face lookups.
type modelTable256 struct {
	model   [256]rl.Model
	present [256]bool
}

// flattenRLModelTable builds a modelTable256 from a byte-keyed map (by pointer; map stays authoritative for Unload).
func flattenRLModelTable(src map[byte]rl.Model) *modelTable256 {
	t := new(modelTable256)
	for c, m := range src {
		t.model[c] = m
		t.present[c] = true
	}
	return t
}

// inlineDecorRenderer is the per-tile draw closure for a decor char on a specialized path
// (dedicated Resources field / scatter helper) rather than the generic decorModels map.
type inlineDecorRenderer func(assets Resources, x, z int, cx, cz, groundY float32)

// inlineDecorHandlers is the single source of truth for inline decor dispatch — assertDecorCoverage
// and inlineDecorTable both read it, so adding a char is one map entry plus one helper.
var inlineDecorHandlers = map[byte]inlineDecorRenderer{
	core.DecorBush:     drawDecorBush,
	core.DecorMushroom: drawDecorMushroom,
	core.DecorPebble:   drawDecorPebble,
}

// inlineDecorTable is the [256] mirror of inlineDecorHandlers for the hot path (array index beats map hash).
var inlineDecorTable = buildInlineDecorTable()

func buildInlineDecorTable() [256]inlineDecorRenderer {
	var t [256]inlineDecorRenderer
	for c, fn := range inlineDecorHandlers {
		t[c] = fn
	}
	return t
}

// assertDecorCoverage panics if any core.DecorTileChars char is missing from decorModels AND not inline-handled.
func assertDecorCoverage(models map[byte]propModel) {
	for _, c := range core.DecorTileChars() {
		if _, ok := models[c]; ok {
			continue
		}
		if _, ok := inlineDecorHandlers[c]; ok {
			continue
		}
		// Multi-tile-decor tails have no decorModels entry — the anchor draws the spanning mesh.
		if isDecorFootprintTail(c) {
			continue
		}
		panic("render: decor char '" + string(c) + "' has no decorModels entry and is not inline-handled — register a loadXxxProp in LoadResources or add to inlineDecorHandlers")
	}
}

// isDecorFootprintTail reports whether c is the tail char for a multi-tile decor anchor (decor-layer isFootprintTail).
func isDecorFootprintTail(c byte) bool {
	for _, anchor := range core.DecorTileChars() {
		if core.DecorFootprintTail(anchor) == c {
			return true
		}
	}
	return false
}

// inlinePropRenderer is the per-tile draw closure for a prop char on a specialized path. props-layer
// variant of inlineDecorRenderer: propYaw is pre-resolved; x,z seed per-tile shape variance via tileHash.
type inlinePropRenderer func(assets Resources, m *core.AreaDefinition, x, z int, center rl.Vector3, propYaw float32)

// inlinePropHandlers is the single source of truth for inline prop dispatch (drawWorld + assertPropCoverage read it).
var inlinePropHandlers = map[byte]inlinePropRenderer{
	// Four scaled-tree variants share one closure factory (treePropScales). Twin is separate (two offset instances).
	core.TileTree:      drawPropTreeScaled(core.TileTree),
	core.TileTreeXL:    drawPropTreeScaled(core.TileTreeXL),
	core.TileTreeTall:  drawPropTreeScaled(core.TileTreeTall),
	core.TileTreeYoung: drawPropTreeScaled(core.TileTreeYoung),
	core.TileTreeTwin:  drawPropTreeTwin,
	core.TileRockLarge: drawPropRockLarge,
	core.TileBushLarge: drawPropBushLarge,
	// Wall torch — uses the area to find the adjacent wall; ignores propYaw (derives facing from the wall).
	core.TileTorch: drawWallTorch,
}

// inlinePropTable is the [256] mirror of inlinePropHandlers for the hot path. Nil = not inline-handled
// (caller falls through to footprint / propModels paths). Array index beats map hash.
var inlinePropTable = buildInlinePropTable()

func buildInlinePropTable() [256]inlinePropRenderer {
	var t [256]inlinePropRenderer
	for c, fn := range inlinePropHandlers {
		t[c] = fn
	}
	return t
}

// assertPropCoverage: every core.PropTileChars char must be in propModels or inline-handled.
func assertPropCoverage(models map[byte]propModel) {
	for _, c := range core.PropTileChars() {
		if _, ok := models[c]; ok {
			continue
		}
		if _, ok := inlinePropHandlers[c]; ok {
			continue
		}
		// Multi-tile-prop tails have no propModels entry — the anchor draws the spanning mesh.
		if isFootprintTail(c) {
			continue
		}
		panic("render: prop char '" + string(c) + "' has no propModels entry and is not inline-handled — register a loadXxxProp in LoadResources or add to inlinePropHandlers")
	}
}

func assertDoorProps(models [core.DoorStyleCount]propModel) {
	for i := range models {
		if len(models[i].parts) == 0 {
			panic("render: door style " + core.DoorStyleName(core.DoorStyle(i)) + " has no doorProps entry")
		}
	}
}

// isFootprintTail reports whether c is a tail char for a multi-tile prop anchor (walks the anchor set, no tail list).
func isFootprintTail(c byte) bool {
	for _, anchor := range core.PropTileChars() {
		if core.PropFootprintTail(anchor) == c {
			return true
		}
	}
	return false
}

func (r Resources) Unload() {
	// Retro-filter and menu-fade RTs are package-level lazy singletons, not Resources fields — free them here too.
	UnloadRetroFilter()
	closeFadeRT()
	// raylib's UnloadModel frees meshes but NOT the bound GL textures (rmodels.c), so wall/floor/ceiling/prop
	// textures are held until process exit. Acceptable only because Unload runs exactly once, at shutdown (run.go).
	// If that changes, track + UnloadTexture them, de-duping aliased handles first (see enemyTextures note below).
	for _, material := range r.materials {
		rl.UnloadModel(material.faceModel)
		rl.UnloadModel(material.floorModel)
		rl.UnloadModel(material.ceilingModel)
		// Unconditional, NOT gated on hasFloorVariant: zero-ID UnloadModel skips cleanly,
		// and a partial init (one variant built, the second panicking) still frees the one that loaded.
		rl.UnloadModel(material.floorDirtModel)
		rl.UnloadModel(material.floorDarkModel)
	}
	rl.UnloadTexture(r.skyTexture)
	rl.UnloadTexture(r.starTexture)
	// Walk enemyTextures (the owning list), NOT enemyVisuals — that slice aliases handles and would double-free.
	for _, tex := range r.enemyTextures {
		rl.UnloadTexture(tex)
	}
	// partyTextures is the owning list (partyVisuals aliases handles) — walk it to avoid a double-free.
	for _, tex := range r.partyTextures {
		rl.UnloadTexture(tex)
	}
	r.tree.unload()
	r.rockProp.unload()
	r.bushProp.unload()
	r.mushroomProp.unload()
	r.chestBody.unload()
	r.chestLid.unload()
	rl.UnloadModel(r.rampModel)
	if groundShadowReady {
		rl.UnloadModel(groundShadowModel)
		groundShadowReady = false
	}
	if hudGrainReady {
		rl.UnloadTexture(hudGrainTex)
		hudGrainReady = false
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
	for _, model := range r.faceVariants {
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

// loadTiledTexture: 128x128 RGBA → mipmapped, trilinear, repeat-wrapped. Shared by every tile-class texture.
func loadTiledTexture(pixels []color.RGBA) rl.Texture2D {
	tex := loadTexture(pixels, 128, 128, rl.FilterBilinear)
	rl.GenTextureMipmaps(&tex)
	rl.SetTextureFilter(tex, rl.FilterTrilinear)
	rl.SetTextureWrap(tex, rl.WrapRepeat)
	return tex
}

// loadRepeatTexture: sized (non-128x128) variant of loadTiledTexture for bark (64x128) / leaf (96x96).
func loadRepeatTexture(pixels []color.RGBA, width, height int) rl.Texture2D {
	tex := loadTexture(pixels, width, height, rl.FilterBilinear)
	rl.SetTextureWrap(tex, rl.WrapRepeat)
	return tex
}

// loadTileModel builds a tile-sized cube (TileSize × height × TileSize), tiled-textured + lit. Wall/floor cubes; only height differs.
func loadTileModel(pixels []color.RGBA, height float32, shader rl.Shader) rl.Model {
	tex := loadTiledTexture(pixels)
	model := rl.LoadModelFromMesh(rl.GenMeshCube(core.TileSize, height, core.TileSize))
	setModelTexture(&model, tex)
	attachShader(&model, shader)
	return model
}

// loadFloorModel builds a floor cube at flat 0.06 height so wall geometry overlaps cleanly.
func loadFloorModel(pixels []color.RGBA, shader rl.Shader) rl.Model {
	return loadTileModel(pixels, 0.06, shader)
}

// rampWedgePins keeps each ramp-wedge mesh's CPU arrays alive for the process lifetime so the Mesh's
// *float32 pointers can't dangle. APPEND (not overwrite) so a second call can't orphan the first's slices.
var rampWedgePins [][]float32

// buildRampWedgeModel builds a SOLID wedge (right triangular prism) sized one tile (TileSize × LevelStep),
// ascending toward -Z: low edge (south,+Z) at y=0, high edge (north,-Z) at y=LevelStep, filled solid + opaque.
// drawRampWedge yaw-rotates this one model per ascent direction; it meets both floors flush, fills the whole tile.
func buildRampWedgeModel(pixels []color.RGBA, shader rl.Shader) rl.Model {
	H := core.LevelStep
	lowSL := rl.NewVector3(-tileHalf, 0, tileHalf) // low edge (south), left
	lowSR := rl.NewVector3(tileHalf, 0, tileHalf)  // low edge (south), right
	hiNL := rl.NewVector3(-tileHalf, H, -tileHalf) // high edge (north), left
	hiNR := rl.NewVector3(tileHalf, H, -tileHalf)  // high edge (north), right
	botNL := rl.NewVector3(-tileHalf, 0, -tileHalf)
	botNR := rl.NewVector3(tileHalf, 0, -tileHalf)
	// Interior reference point so each face's winding can be flipped to face outward (correct normals).
	center := rl.NewVector3(0, H/3, -tileHalf/3)

	var verts, normals, uvs []float32
	addTri := func(a, b, c rl.Vector3) {
		n := triNormal(a, b, c)
		mid := rl.NewVector3((a.X+b.X+c.X)/3, (a.Y+b.Y+c.Y)/3, (a.Z+b.Z+c.Z)/3)
		if n.X*(mid.X-center.X)+n.Y*(mid.Y-center.Y)+n.Z*(mid.Z-center.Z) < 0 {
			b, c = c, b // flip winding so the normal points outward
			n = triNormal(a, b, c)
		}
		for _, p := range [3]rl.Vector3{a, b, c} {
			verts = append(verts, p.X, p.Y, p.Z)
			normals = append(normals, n.X, n.Y, n.Z)
		}
		uvs = append(uvs, 0, 0, 1, 0, 0.5, 1)
	}
	addQuad := func(a, b, c, d rl.Vector3) { addTri(a, b, c); addTri(a, c, d) }

	addQuad(lowSL, lowSR, hiNR, hiNL)   // sloped top
	addQuad(lowSL, lowSR, botNR, botNL) // bottom (y=0 footprint)
	addQuad(botNL, botNR, hiNR, hiNL)   // north (high) wall
	addTri(lowSL, hiNL, botNL)          // west side
	addTri(lowSR, hiNR, botNR)          // east side

	rampWedgePins = append(rampWedgePins, verts, normals, uvs)
	mesh := rl.Mesh{
		VertexCount:   int32(len(verts) / 3),
		TriangleCount: int32(len(verts) / 9),
	}
	mesh.Vertices = (*float32)(unsafe.Pointer(&verts[0]))
	mesh.Normals = (*float32)(unsafe.Pointer(&normals[0]))
	mesh.Texcoords = (*float32)(unsafe.Pointer(&uvs[0]))
	rl.UploadMesh(&mesh, false)
	model := rl.LoadModelFromMesh(mesh)
	setModelTexture(&model, loadTiledTexture(pixels))
	attachShader(&model, shader)
	return model
}

// facePins keeps each cliff-face quad's CPU mesh arrays alive for the process lifetime (see rampWedgePins).
var facePins [][]float32

// buildFaceQuadModel builds ONE vertical cliff-face quad (TileSize × LevelStep) on the +Z tile edge, normal +Z.
// drawCliffFace translates/yaw-rotates it and scales Y by level delta, so one model skins every cliff.
// Texture maps 0..1 over one LevelStep, so multi-level faces stretch it vertically (acceptable for rock).
func buildFaceQuadModel(pixels []color.RGBA, shader rl.Shader) rl.Model {
	H := core.LevelStep
	// CCW as seen from +Z (outward) so the front face survives raylib's back-face cull.
	bl := rl.NewVector3(-tileHalf, 0, tileHalf)
	br := rl.NewVector3(tileHalf, 0, tileHalf)
	tr := rl.NewVector3(tileHalf, H, tileHalf)
	tl := rl.NewVector3(-tileHalf, H, tileHalf)

	verts := []float32{
		bl.X, bl.Y, bl.Z, br.X, br.Y, br.Z, tr.X, tr.Y, tr.Z, // tri 1
		bl.X, bl.Y, bl.Z, tr.X, tr.Y, tr.Z, tl.X, tl.Y, tl.Z, // tri 2
	}
	normals := make([]float32, 0, len(verts))
	for i := 0; i < 6; i++ {
		normals = append(normals, 0, 0, 1) // +Z outward
	}
	uvs := []float32{
		0, 1, 1, 1, 1, 0, // bl, br, tr
		0, 1, 1, 0, 0, 0, // bl, tr, tl
	}

	facePins = append(facePins, verts, normals, uvs)
	mesh := rl.Mesh{
		VertexCount:   int32(len(verts) / 3),
		TriangleCount: int32(len(verts) / 9),
	}
	mesh.Vertices = (*float32)(unsafe.Pointer(&verts[0]))
	mesh.Normals = (*float32)(unsafe.Pointer(&normals[0]))
	mesh.Texcoords = (*float32)(unsafe.Pointer(&uvs[0]))
	rl.UploadMesh(&mesh, false)
	model := rl.LoadModelFromMesh(mesh)
	setModelTexture(&model, loadTiledTexture(pixels))
	attachShader(&model, shader)
	return model
}

// loadGroundShadowModel builds the soft contact-shadow plane: a 1×1 XZ quad with the radial-gradient sprite,
// clamped + bilinear. NO lighting shader attached, so DrawModelEx renders it unlit even under BeginShaderMode.
func loadGroundShadowModel() rl.Model {
	tex := loadTexture(makeSoftShadowPixels(64), 64, 64, rl.FilterBilinear)
	rl.SetTextureWrap(tex, rl.WrapClamp)
	model := rl.LoadModelFromMesh(rl.GenMeshPlane(1, 1, 1, 1))
	setModelTexture(&model, tex)
	return model
}

func loadWorldMaterial(wallPixels, floorPixels []color.RGBA, shader rl.Shader) worldMaterialResources {
	return worldMaterialResources{
		faceModel:  buildFaceQuadModel(wallPixels, shader),
		floorModel: loadFloorModel(floorPixels, shader),
		// Ceiling reuses the wall texture at a thin 0.2 slab. Same shader so lighting/time-of-day apply consistently.
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

// assertMaterialCoverage panics unless every core.MaterialSet has a worldMaterial (else it renders as Field).
func assertMaterialCoverage(materials map[core.MaterialSet]worldMaterialResources) {
	for i := 0; i < int(core.MaterialCount); i++ {
		mat := core.MaterialSet(i)
		if _, ok := materials[mat]; !ok {
			name, _ := core.MaterialName(mat)
			panic("render: material " + name + " has no worldMaterialResources — load it in LoadResources")
		}
	}
}

// lightingFor routes a material to its lighting profile (profiles are package consts in lighting.go).
func lightingFor(material core.MaterialSet) lightingProfile {
	if core.MaterialIsIndoor(material) {
		return dungeonLighting
	}
	return fieldLighting
}

// loadEnemySprite mints a point-filtered, clamped sprite texture and registers it in `owned` for Unload.
func loadEnemySprite(pixels []color.RGBA, w, h int, owned *[]rl.Texture2D) rl.Texture2D {
	tex := loadTexture(pixels, w, h, rl.FilterPoint)
	rl.SetTextureWrap(tex, rl.WrapClamp)
	*owned = append(*owned, tex)
	return tex
}

// loadEnemySpriteFile loads an authored sprite PNG as a billboard-ready texture with mipmaps + trilinear
// (high-res art is heavily minified as a small billboard). ok=false (registers nothing) on missing/bad file,
// so the caller can fall back to procedural art. Registered textures go to owned for Unload.
func loadEnemySpriteFile(name string, owned *[]rl.Texture2D) (rl.Texture2D, bool) {
	path := filepath.Join(core.ResolveAssetDir(core.SpritesDirName), name)
	if _, err := os.Stat(path); err != nil {
		return rl.Texture2D{}, false
	}
	tex := rl.LoadTexture(path)
	if tex.ID == 0 || tex.Width <= 0 || tex.Height <= 0 {
		// Decode failure or zero-dimension image: fall back to procedural. Unload a non-zero handle so it doesn't leak.
		if tex.ID != 0 {
			rl.UnloadTexture(tex)
		}
		return rl.Texture2D{}, false
	}
	rl.GenTextureMipmaps(&tex)
	rl.SetTextureFilter(tex, rl.FilterTrilinear)
	rl.SetTextureWrap(tex, rl.WrapClamp)
	*owned = append(*owned, tex)
	return tex, true
}

// enemyVisualsToSlice / partyVisualsToSlice flatten the kind/class→visual maps into dense slices indexed by
// the iota, so the runtime lookup is an array index. Slice (a reference type) so editor live-preview propagates.
func enemyVisualsToSlice(m map[core.EnemyKind]enemyVisual) []enemyVisual {
	out := make([]enemyVisual, core.EnemyKindCount())
	for kind, v := range m {
		if int(kind) >= 0 && int(kind) < len(out) {
			out[kind] = v
		}
	}
	return out
}

func partyVisualsToSlice(m map[core.PartyClass]enemyVisual) []enemyVisual {
	out := make([]enemyVisual, len(core.PartyClasses()))
	for class, v := range m {
		if int(class) >= 0 && int(class) < len(out) {
			out[class] = v
		}
	}
	return out
}

func loadEnemyVisuals() (visuals map[core.EnemyKind]enemyVisual, owned []rl.Texture2D) {
	// LoadResources assigns r.enemyTextures only on a clean return, so free the accumulated handles here
	// (and re-panic) to stay leak-safe if we panic before returning (coverage assert / sprite upload OOM).
	defer func() {
		if rec := recover(); rec != nil {
			for _, t := range owned {
				rl.UnloadTexture(t)
			}
			panic(rec)
		}
	}()
	// Feral Rat: authored PNG, falling back to procedural makeRatPixels so a checkout without the asset still renders.
	ratTexture, ratFromFile := loadEnemySpriteFile(core.EnemySlug(core.EnemyRat)+".png", &owned)
	if !ratFromFile {
		ratTexture = loadEnemySprite(makeRatPixels(72, 96), 72, 96, &owned)
	}
	batTexture := loadEnemySprite(makeBatPixels(80, 88), 80, 88, &owned)
	diseasedRatTexture := loadEnemySprite(makeDiseasedRatPixels(72, 96), 72, 96, &owned)
	goblinTexture := loadEnemySprite(makeGoblinPixels(72, 112), 72, 112, &owned)
	goblinMageTexture := loadEnemySprite(makeGoblinMagePixels(72, 112), 72, 112, &owned)
	amoebaTexture := loadEnemySprite(makeAmoebaPixels(96, 80), 96, 80, &owned)
	mantrapTexture := loadEnemySprite(makeVenusMantrapPixels(88, 128), 88, 128, &owned)
	// Roster-expansion sprites, dimensions sized to each kind's silhouette role.
	caveSpiderTexture := loadEnemySprite(makeCaveSpiderPixels(88, 72), 88, 72, &owned)
	vampireBatTexture := loadEnemySprite(makeVampireBatPixels(96, 88), 96, 88, &owned)
	wispTexture := loadEnemySprite(makeWispPixels(56, 72), 56, 72, &owned)
	stoneGolemTexture := loadEnemySprite(makeStoneGolemPixels(96, 120), 96, 120, &owned)
	necromancerTexture := loadEnemySprite(makeNecromancerPixels(72, 112), 72, 112, &owned)
	skeletonTexture := loadEnemySprite(makeSkeletonPixels(72, 112), 72, 112, &owned)
	visuals = map[core.EnemyKind]enemyVisual{
		core.EnemyRat: {
			texture: ratTexture,
			// Neutral defaults pending new authored PNG; re-tune in the Foe Visualizer.
			size: rl.NewVector2(1.0, 1.0),
		},
		core.EnemyBat: {
			texture: batTexture,
			// Width kept just under enemyDrawPosition's 1.12 slot spacing so adjacent bats don't merge.
			size: rl.NewVector2(0.98, 0.84),
		},
		core.EnemyDiseasedRat: {
			texture: diseasedRatTexture,
			size:    rl.NewVector2(0.92, 1.30),
		},
		core.EnemyGoblin: {
			texture: goblinTexture,
			size:    rl.NewVector2(1.05, 1.55),
		},
		core.EnemyGoblinMage: {
			texture: goblinMageTexture,
			size:    rl.NewVector2(1.10, 1.65),
		},
		core.EnemyAmoeba: {
			texture: amoebaTexture,
			size:    rl.NewVector2(1.20, 0.95),
		},
		core.EnemyVenusMantrap: {
			texture: mantrapTexture,
			size:    rl.NewVector2(1.20, 1.80),
		},
		core.EnemyCaveSpider: {
			texture: caveSpiderTexture,
			size:    rl.NewVector2(1.10, 0.95),
		},
		core.EnemyVampireBat: {
			texture: vampireBatTexture,
			size:    rl.NewVector2(1.12, 1.00),
		},
		core.EnemyWisp: {
			texture: wispTexture,
			size:    rl.NewVector2(0.65, 0.92),
		},
		core.EnemyStoneGolem: {
			texture: stoneGolemTexture,
			size:    rl.NewVector2(1.55, 1.95),
		},
		core.EnemyNecromancer: {
			texture: necromancerTexture,
			size:    rl.NewVector2(1.05, 1.70),
		},
		core.EnemySkeleton: {
			texture: skeletonTexture,
			size:    rl.NewVector2(0.95, 1.50),
		},
	}
	// Coverage assert: every EnemyKind must have a visual, else enemyVisualFor falls back to a rat.
	for _, def := range core.EnemyKinds() {
		v, ok := visuals[def.Kind]
		if !ok || v.texture.ID == 0 {
			panic("render: missing enemyVisuals entry for " + def.Name + " — author a sprite and register it in loadEnemyVisuals")
		}
	}
	// Record each kind's PRISTINE base texture before overlaying overrides — the editor re-derives its FX
	// preview from this (never the adjusted display texture) so slider drags don't compound. Every kind, so
	// a freshly-edited one has a pristine base. Overrides come from maps/sprites/visuals.json next; missing
	// or malformed file is a no-op (defaults stand), keyed by core.EnemySlug to agree with the editor.
	for kind, v := range visuals {
		v.pristineTexture = v.texture
		visuals[kind] = v
	}
	if overrides, err := core.LoadEnemyVisualOverrides(); err == nil {
		for kind, v := range visuals {
			if ov, ok := overrides[core.EnemySlug(kind)]; ok {
				v = applyEnemyVisualOverride(v, ov)
				// Bake the non-destructive adjustments into the DISPLAY texture (point-sampled when pixelated).
				if tex, ok := deriveAdjustedTexture(v.pristineTexture, ov, &owned); ok {
					v.texture = tex
				}
				visuals[kind] = v
			}
		}
	}
	return visuals, owned
}

// loadPartyVisuals builds the per-class party billboard table, mirroring loadEnemyVisuals (default size,
// authored PNG or procedural makePartyPixels, partyvisuals.json overlay). Reuses the foe-side enemyVisual
// struct + apply helper (PartyVisualOverride aliases EnemyVisualOverride); owned owns the handles for Unload.
func loadPartyVisuals() (visuals map[core.PartyClass]enemyVisual, owned []rl.Texture2D) {
	defer func() {
		if rec := recover(); rec != nil {
			for _, t := range owned {
				rl.UnloadTexture(t)
			}
			panic(rec)
		}
	}()
	visuals = map[core.PartyClass]enemyVisual{}
	for _, def := range core.PartyClasses() {
		tex, ok := loadEnemySpriteFile(core.PartyClassSlug(def.Class)+".png", &owned)
		if !ok {
			tex = loadEnemySprite(makePartyPixels(64, 80, def.Class), 64, 80, &owned)
		}
		visuals[def.Class] = enemyVisual{
			texture: tex,
			size:    partyBillboardSize,
		}
	}
	// Overlay overrides from maps/sprites/partyvisuals.json; missing/malformed is a no-op (foe-side discipline).
	for class, v := range visuals {
		v.pristineTexture = v.texture
		visuals[class] = v
	}
	if overrides, err := core.LoadPartyVisualOverrides(); err == nil {
		for class, v := range visuals {
			if ov, ok := overrides[core.PartyClassSlug(class)]; ok {
				v = applyEnemyVisualOverride(v, ov)
				if tex, ok := deriveAdjustedTexture(v.pristineTexture, ov, &owned); ok {
					v.texture = tex
				}
				visuals[class] = v
			}
		}
	}
	return visuals, owned
}

// hudFontCodepoints is the rune set baked into the HUD font atlas. The atlas is built once at LoadResources,
// so any glyph not enumerated here renders as a "?" box at runtime — adding a new HUD symbol means extending this list.
var hudFontCodepoints = buildHUDFontCodepoints()

func buildHUDFontCodepoints() []rune {
	runes := make([]rune, 0, 128)
	// Standard printable ASCII (space through tilde).
	for r := rune(32); r <= 126; r++ {
		runes = append(runes, r)
	}
	// Non-ASCII extras used across HUD, editor, and combat surfaces.
	runes = append(runes,
		'°', // degrees
		'±', // stat deltas
		'×', // pack-size badge, pack-edit remove
		'é', // foreign names (cushion)
		'–', // en-dash
		'—', // em-dash
		'…', // ellipsis
		'·', // middle dot — hint-bar separators
		'←',
		'↑',
		'→',
		'↓',
		'↔', // door-pair links
		'∈', // set membership
		'−', // unicode minus (distinct from ASCII hyphen)
		'≈',
		'≤',
		'≥',
		'▲', // pack-edit reorder up
		'▶', // action-row submenu indicator
		'▸', // submenu chevron
		'◂', // left chevron (cushion)
		'▼', // pack-edit reorder down
		'●', // bullet / active marker
		'★', // dialog start-node marker
		'✓', // dropdown active-toggle check
		'’', // typographic apostrophe
		'‘', // left single quote (cushion)
	)
	return runes
}

// hudFontBake is the glyph-atlas bake size in px. Generous (128) so every UI size DOWN-samples from a
// high-res master (crisp small serifs, >2× Title headroom); sharpenFontAtlas's mipmaps+trilinear kill shimmer.
const hudFontBake = int32(128)

// loadHUDFont builds the single UI face from the embedded Della Respira TTF, baked once at hudFontBake.
// Glyphs it omits are drawn procedurally (richtext.go). System-serif scan + raylib default are a defensive fallback.
func loadHUDFont() (rl.Font, bool) {
	if font := rl.LoadFontFromMemory(".ttf", uiFontTTF, hudFontBake, hudFontCodepoints); rl.IsFontValid(font) {
		sharpenFontAtlas(&font)
		return font, true
	}
	for _, path := range systemFontCandidates() {
		if _, err := os.Stat(path); err != nil {
			continue
		}
		font := rl.LoadFontEx(path, hudFontBake, hudFontCodepoints)
		if rl.IsFontValid(font) {
			sharpenFontAtlas(&font)
			return font, true
		}
	}
	return rl.GetFontDefault(), false
}

// sharpenFontAtlas applies mipmaps + trilinear so small sizes down-sample smoothly instead of shimmering.
// The bake's glyph padding absorbs the minor cross-glyph bleed the lower mips introduce.
func sharpenFontAtlas(font *rl.Font) {
	rl.GenTextureMipmaps(&font.Texture)
	rl.SetTextureFilter(font.Texture, rl.FilterTrilinear)
}

// systemFontCandidates returns per-OS serif paths used ONLY as a defensive fallback (loadHUDFont),
// tried in priority order: readable serifs first, platform sans last.
func systemFontCandidates() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{
			`C:\Windows\Fonts\constan.ttf`, // Constantia — refined serif, first fallback
			`C:\Windows\Fonts\cambria.ttc`, // Cambria — second-best serif
			`C:\Windows\Fonts\georgia.ttf`, // Georgia — classic web serif fallback
			`C:\Windows\Fonts\pala.ttf`,    // Palatino Linotype — older alternate
			`C:\Windows\Fonts\seguisb.ttf`, // Segoe UI Semibold — final sans fallback
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

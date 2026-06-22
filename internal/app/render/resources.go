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

// uiFontTTF is the embedded UI face — Della Respira (SIL OFL 1.1, OFL.txt under
// fonts/). The ONE font the whole UI draws with; glyphs it lacks (arrows, …, etc.)
// are drawn procedurally (richtext.go).
//
//go:embed fonts/DellaRespira-Regular.ttf
var uiFontTTF []byte

type Resources struct {
	materials  map[core.MaterialSet]worldMaterialResources
	skyTexture rl.Texture2D
	// starTexture is the night overlay sampled by DrawSkyBackground; its alpha
	// follows the time-of-day curve (timeProfile.StarAlpha).
	starTexture rl.Texture2D
	// enemyVisuals is the kind→billboard lookup, a dense slice indexed by EnemyKind
	// (array index, not map hash, on the hot path). Kinds can alias one texture, so
	// it does NOT own the handles; enemyTextures is the owning list Unload walks.
	// Still a slice, so editor live-preview writes propagate across by-value copies.
	enemyVisuals  []enemyVisual
	enemyTextures []rl.Texture2D
	// partyVisuals is the class→billboard lookup, symmetric with enemyVisuals;
	// partyTextures is its owning list.
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
	// chestBody / chestLid — drawn as two parts so the looted path can lift+tilt
	// the lid without re-posing the body.
	chestBody propModel
	chestLid  propModel

	// Universal floor variants keyed by floor-layer char, shared across materials.
	specialFloors map[byte]rl.Model
	// specialFloorTable is the char-indexed mirror, probed per floor tile per
	// frame (array index beats the map hash). The map stays authoritative for Unload.
	specialFloorTable *modelTable256

	// Face variants keyed by face-skin char (ivy / cracked / crumbling). Plain
	// TileRock is NOT here — it uses the per-material face. Shared across materials.
	faceVariants map[byte]rl.Model
	// faceVariantTable is the char-indexed mirror; map stays authoritative for Unload.
	faceVariantTable *modelTable256

	// rampModel is the solid wedge for ramp tiles, earth-textured, drawn yaw-
	// rotated. Sized one tile × LevelStep so it meets both floors flush.
	rampModel rl.Model

	// underModel is the downward -Y quad for floating-cube undersides (voxel
	// maps only). One tile, earth-textured. See drawVoxelColumn in voxel.go.
	underModel rl.Model

	// decorModels keyed by decor-layer char. The map drives authoring + Unload;
	// decorModelTable below is the hot-path mirror.
	decorModels map[byte]propModel
	// decorModelTable is the [256]propModel mirror, held by pointer so passing
	// Resources by value stays cheap. "Registered" = len(parts) > 0. The map
	// stays authoritative for Unload.
	decorModelTable *[256]propModel

	// propModels keyed by props-layer char. Same dispatch shape as decorModels;
	// the renderer falls back to tree/boulder/bush cases when a char isn't here.
	propModels map[byte]propModel
	// propModelTable is the [256]propModel mirror, by pointer for the same reason
	// as decorModelTable. Map remains for authoring and Unload.
	propModelTable *[256]propModel

	// doorProps holds one model per core.DoorStyle; DrawDoors indexes by Style
	// and rotates by the authored facing.
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

// LoadResources builds every procedural texture/model/font/shader. Staged
// cleanup: each handle is committed to `r` as created; a deferred recover()
// calls r.Unload() then re-panics if construction fails partway, so a mid-load
// failure doesn't leak handles already on the GPU.
func LoadResources() (r Resources) {
	committed := false
	defer func() {
		if committed {
			return
		}
		if rec := recover(); rec != nil {
			// r.Unload tolerates zero values (nil-map ranges no-op; UnloadModel/
			// Texture skip on id == 0).
			r.Unload()
			panic(rec)
		}
	}()

	r.lighting = loadLightingShader()
	r.billboardFog = loadBillboardFogShader()

	// Commit each material the instant it's built so the recover-path Unload can
	// find every earlier model if a later load panics.
	dungeonMat := loadWorldMaterial(makeStoneBrickPixels(128, 128), makeStoneFloorPixels(128, 128), r.lighting.shader)
	r.materials = map[core.MaterialSet]worldMaterialResources{
		core.MaterialDungeon: dungeonMat,
	}
	fieldMat := loadWorldMaterial(makeRockWallPixels(128, 128), makeGrassPixels(128, 128), r.lighting.shader)
	r.materials[core.MaterialField] = fieldMat
	// Field's two extra floor variants (dirt + dark grass), per-tile by hash.
	// Commit after EACH so a panic in the second leaves the first for Unload.
	fieldMat.floorDirtModel = loadFloorModel(makeDirtPixels(128, 128), r.lighting.shader)
	r.materials[core.MaterialField] = fieldMat
	fieldMat.floorDarkModel = loadFloorModel(makeDarkGrassPixels(128, 128), r.lighting.shader)
	fieldMat.hasFloorVariant = true
	r.materials[core.MaterialField] = fieldMat
	// Every core.MaterialSet must have a loaded worldMaterial, else it silently
	// falls back to Field in worldMaterial().
	assertMaterialCoverage(r.materials)

	r.skyTexture = loadTexture(makeSkyPixels(1024, 512), 1024, 512, rl.FilterTrilinear)
	rl.GenTextureMipmaps(&r.skyTexture)
	rl.SetTextureFilter(r.skyTexture, rl.FilterTrilinear)
	rl.SetTextureWrap(r.skyTexture, rl.WrapClamp)

	// Star overlay: same dimensions as the sky (shared source-rect math).
	// Point filter so the pinpoint stars don't blur at small dest scales.
	r.starTexture = loadTexture(makeStarPixels(1024, 512), 1024, 512, rl.FilterPoint)
	rl.SetTextureWrap(r.starTexture, rl.WrapClamp)

	enemyVis, enemyTex := loadEnemyVisuals()
	r.enemyVisuals, r.enemyTextures = enemyVisualsToSlice(enemyVis), enemyTex

	partyVis, partyTex := loadPartyVisuals()
	r.partyVisuals, r.partyTextures = partyVisualsToSlice(partyVis), partyTex

	r.hudFont, r.hudFontOwned = loadHUDFont()

	// Bark (64×128) and leaf (96×96) are non-tile sizes → loadRepeatTexture.
	// loadTreeModel takes ownership via the model (UnloadModel frees them), so
	// they're NOT tracked in an owned list (that would double-free on teardown).
	// A panic before the model takes ownership orphans them — accepted, since the
	// process is already crashing.
	barkTex := loadRepeatTexture(makeBarkPixels(64, 128), 64, 128)
	leafTex := loadRepeatTexture(makeLeafPixels(96, 96), 96, 96)
	r.tree = loadTreeModel(r.lighting.shader, barkTex, leafTex)

	// Each field prop gets its OWN texture instance (no sharing). UnloadModel
	// frees meshes but NOT bound GL textures, so these are held until process
	// exit (acceptable: Unload runs once). Per-prop instancing avoids the alias
	// double-free a shared texture would invite.
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

	// Soft ground-shadow disc, drawn UNLIT (default shader). Package singleton
	// since drawGroundShadow is called from many free-function prop draws.
	groundShadowModel = loadGroundShadowModel()
	groundShadowReady = true

	// HUD glass-grain overlay (drawGlassRelief). Package singleton like the
	// shadow disc — the theme helpers are free functions.
	hudGrainTex = loadTexture(makeHudGrainPixels(64, 64), 64, 64, rl.FilterBilinear)
	hudGrainReady = true

	// Wall-torch flame — unlit emissive sphere (default shader), animated by drawWallTorch.
	torchFlameModel = rl.LoadModelFromMesh(rl.GenMeshSphere(1, 8, 10))
	torchFlameReady = true

	// Universal floor variants, shared across material sets. Init the map first
	// so a panic mid-way still unloads what landed.
	r.specialFloors = make(map[byte]rl.Model)
	r.specialFloors[core.FloorCobble] = loadFloorModel(makeCobblePixels(128, 128), r.lighting.shader)
	r.specialFloors[core.FloorPlank] = loadFloorModel(makePlankPixels(128, 128), r.lighting.shader)
	r.specialFloors[core.FloorWater] = loadFloorModel(makeWaterPixels(128, 128), r.lighting.shader)
	r.specialFloors[core.FloorDeepWater] = loadFloorModel(makeDeepWaterPixels(128, 128), r.lighting.shader)
	r.specialFloors[core.FloorSand] = loadFloorModel(makeSandPixels(128, 128), r.lighting.shader)
	r.specialFloors[core.FloorSnow] = loadFloorModel(makeSnowPixels(128, 128), r.lighting.shader)
	// Grass / dirt / dark grass / stone are also universal — without these,
	// painting them in a dungeon map silently reuses the base floorModel (the
	// per-material variant switch only fires when hasFloorVariant, field-only).
	r.specialFloors[core.FloorGrass] = loadFloorModel(makeGrassPixels(128, 128), r.lighting.shader)
	r.specialFloors[core.FloorDirt] = loadFloorModel(makeDirtPixels(128, 128), r.lighting.shader)
	r.specialFloors[core.FloorDarkGrass] = loadFloorModel(makeDarkGrassPixels(128, 128), r.lighting.shader)
	r.specialFloors[core.FloorStone] = loadFloorModel(makeStoneFloorPixels(128, 128), r.lighting.shader)
	// Face variants — cliff-face quads with per-char rock skins (ivy / cracked /
	// crumbling), flat quads carrying their damaged texture. Init first for the
	// panic-cleanup path.
	r.faceVariants = make(map[byte]rl.Model)
	r.faceVariants[core.TileWallRockIvyLight] = buildFaceQuadModel(makeRockIvyPixels(128, 128, false), r.lighting.shader)
	r.faceVariants[core.TileWallRockIvyHeavy] = buildFaceQuadModel(makeRockIvyPixels(128, 128, true), r.lighting.shader)
	r.faceVariants[core.TileWallRockCracked] = buildFaceQuadModel(makeRockCrackedPixels(128, 128), r.lighting.shader)
	r.faceVariants[core.TileWallRockCrumbling] = buildFaceQuadModel(makeRockCrumblingPixels(128, 128), r.lighting.shader)
	// Coverage: every non-rock skin in core.FaceSkins needs a variant model here
	// (plain Rock uses the per-material faceModel). Fail fast at load.
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
	// Wood/leaf props mint fresh texture instances per loader (each propModel
	// owns its textures; sharing would double-unload).
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

	// Non-blocking decorative plant props (shader-only). Registered for
	// assertPropCoverage; don't block movement (core.PropIsNonBlocking).
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

	// Flatten the maps into [256] tables for array-indexed per-tile dispatch.
	// Maps remain authoritative for Unload + assertions.
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

// flattenModelTable copies the authored map into a heap-allocated [256]propModel.
// Stored by pointer so passing Resources by value stays cheap.
func flattenModelTable(src map[byte]propModel) *[256]propModel {
	t := new([256]propModel)
	for c, pm := range src {
		t[c] = pm
	}
	return t
}

// modelTable256 is a char-indexed [256]rl.Model mirror with a parallel presence
// bitmap (rl.Model has no cheap "absent" sentinel), for array-indexed floor /
// cliff-face lookups.
type modelTable256 struct {
	model   [256]rl.Model
	present [256]bool
}

// flattenRLModelTable builds a modelTable256 from a byte-keyed model map. Held
// by pointer on Resources so passing Resources by value stays cheap. The source
// map stays authoritative for Unload iteration; this mirror is read-only.
func flattenRLModelTable(src map[byte]rl.Model) *modelTable256 {
	t := new(modelTable256)
	for c, m := range src {
		t.model[c] = m
		t.present[c] = true
	}
	return t
}

// inlineDecorRenderer is the per-tile drawing closure for a decor
// char rendered through a specialized path (dedicated propModel field
// on Resources, hand-tuned scatter helper) rather than the generic
// decorModels map. Same dispatch signature as the generic map keeps
// world.go's drawDecor uniform.
type inlineDecorRenderer func(assets Resources, x, z int, cx, cz, groundY float32)

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
type inlinePropRenderer func(assets Resources, m *core.AreaDefinition, x, z int, center rl.Vector3, propYaw float32)

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

func assertDoorProps(models [core.DoorStyleCount]propModel) {
	for i := range models {
		if len(models[i].parts) == 0 {
			panic("render: door style " + core.DoorStyleName(core.DoorStyle(i)) + " has no doorProps entry")
		}
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
	// Retro-filter pass (shader + capture RT) is a package-level lazy
	// singleton, not a Resources field — free it alongside everything else.
	UnloadRetroFilter()
	// Menu-fade cross-fade capture RT — same package-level lazy-singleton
	// lifecycle as the retro filter; free it here too.
	closeFadeRT()
	// NOTE: raylib's UnloadModel frees the model's meshes + the materials' maps
	// array, but NOT the GL textures bound into those maps (rmodels.c). So the
	// wall/floor/ceiling textures (and the prop textures freed via .unload()
	// below) are NOT reclaimed here — they're held until process exit, where
	// the driver releases all GPU memory. This is acceptable only because
	// Unload runs exactly once, at shutdown (run.go); it is NOT a per-scene
	// teardown. If that ever changes, these textures must be tracked and
	// UnloadTexture'd explicitly — and any texture aliased across models (see
	// the enemyTextures dedup note below) de-duped first to avoid a double-free.
	for _, material := range r.materials {
		rl.UnloadModel(material.faceModel)
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
	// Walk enemyTextures (the owning list), NOT enemyVisuals — that slice
	// aliases the same handle at multiple indices (placeholder sprites for
	// the new monsters) and iterating it would double-free every shared
	// texture at game exit.
	for _, tex := range r.enemyTextures {
		rl.UnloadTexture(tex)
	}
	// partyTextures is the owning list (partyVisuals aliases handles, like
	// enemyVisuals) — walk it, not the slice, to avoid a double-free.
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

// rampWedgePins keeps every ramp-wedge mesh's CPU-side arrays alive for the
// process lifetime. gen2brain's UploadMesh pins + go-manages these (so
// UnloadModel won't C-free them), but holding a package reference guarantees
// the Mesh's *float32 pointers can never dangle. We APPEND each call's arrays
// rather than overwrite a fixed set of vars, so a second call can't orphan the
// first model's still-referenced slices.
var rampWedgePins [][]float32

// buildRampWedgeModel builds a SOLID wedge (right triangular prism) sized to
// one tile (TileSize wide/deep × LevelStep tall), ascending toward -Z (north):
// the low edge (south, +Z) sits at local y=0, the high edge (north, -Z) at
// y=LevelStep, filled solid down to y=0. Drawn at (cx, lowLevel·LevelStep, cz)
// the low edge meets the low floor and the high edge the one-higher floor
// exactly flush, the footprint fills the whole tile (no neighbor gap), and the
// bottom/back/side faces make it opaque earth (no see-through). drawRampWedge
// yaw-rotates this one model to face each ascent direction.
func buildRampWedgeModel(pixels []color.RGBA, shader rl.Shader) rl.Model {
	const half = float32(core.TileSize) / 2
	H := core.LevelStep
	lowSL := rl.NewVector3(-half, 0, half) // low edge (south), left
	lowSR := rl.NewVector3(half, 0, half)  // low edge (south), right
	hiNL := rl.NewVector3(-half, H, -half) // high edge (north), left
	hiNR := rl.NewVector3(half, H, -half)  // high edge (north), right
	botNL := rl.NewVector3(-half, 0, -half)
	botNR := rl.NewVector3(half, 0, -half)
	// Interior reference point so each face's winding can be flipped to face
	// outward (correct normals + no back-face culling surprises).
	center := rl.NewVector3(0, H/3, -half/3)

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

// facePins keeps every cliff-face quad's CPU-side mesh arrays alive for the
// process lifetime (same rationale as rampWedgePins).
var facePins [][]float32

// buildFaceQuadModel builds ONE vertical cliff-face quad sized to a tile edge —
// TileSize wide, LevelStep tall — sitting on the +Z (south) edge of the tile in
// model space (x∈[-half,half], y∈[0,LevelStep], z=+half), its normal facing +Z
// (outward, toward the lower neighbour). drawCliffFace translates it to a tile,
// yaw-rotates it to the dropping edge, and scales Y by the level delta so one
// model skins every cliff regardless of height. The texture maps 0..1 over one
// LevelStep, so a multi-level face stretches it vertically (acceptable for
// rock; revisit with tiled UVs if a cleaner repeat is wanted).
func buildFaceQuadModel(pixels []color.RGBA, shader rl.Shader) rl.Model {
	const half = float32(core.TileSize) / 2
	H := core.LevelStep
	// CCW as seen from +Z (the outward/viewer side) so the front face survives
	// raylib's default back-face cull.
	bl := rl.NewVector3(-half, 0, half)
	br := rl.NewVector3(half, 0, half)
	tr := rl.NewVector3(half, H, half)
	tl := rl.NewVector3(-half, H, half)

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
		faceModel:  buildFaceQuadModel(wallPixels, shader),
		floorModel: loadFloorModel(floorPixels, shader),
		// Ceiling reuses the wall texture but at a thin (0.2) slab height
		// so its bottom face shows beneath the player while the top sits
		// one level up. Same shader so the lighting profile and time-of-day
		// uniforms apply consistently with faces and floors.
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

// loadEnemySpriteFile loads an authored sprite PNG from the sprites asset
// dir and returns it as a billboard-ready texture. Unlike loadEnemySprite
// (which mints tiny pixel-art atlases drawn at FilterPoint), a high-res
// authored PNG is heavily minified when drawn as a small world billboard,
// so it gets mipmaps + trilinear filtering — the same smooth-minification
// setup the HUD font and sky texture use — to avoid shimmering/aliasing at
// distance. Returns ok=false (and registers nothing) when the file is
// missing or fails to decode, so the caller can fall back to procedural
// art rather than rendering a blank quad. Registered textures are appended
// to owned for Unload, matching loadEnemySprite.
func loadEnemySpriteFile(name string, owned *[]rl.Texture2D) (rl.Texture2D, bool) {
	path := filepath.Join(core.ResolveAssetDir(core.SpritesDirName), name)
	if _, err := os.Stat(path); err != nil {
		return rl.Texture2D{}, false
	}
	tex := rl.LoadTexture(path)
	if tex.ID == 0 || tex.Width <= 0 || tex.Height <= 0 {
		// Decode failure (ID 0) or a corrupt-but-loadable zero-dimension
		// image: fall back to procedural art. Unload a non-zero handle first
		// so the reject path doesn't leak it.
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

// enemyVisualsToSlice / partyVisualsToSlice flatten the kind/class→visual maps
// the loaders build into dense slices indexed by the EnemyKind / PartyClass
// iota, so the per-sprite runtime lookup (enemyVisualFor / partyVisualFor) is an
// array index instead of a map hash. The slice is a reference type like the map
// it replaces, so the editor's live-preview writes still propagate across the
// by-value Resources copies that share it. Every registered kind/class has an
// entry (loadEnemyVisuals asserts coverage), so the dense slice has no holes.
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
	// The caller (LoadResources) assigns r.enemyTextures only on a clean
	// RETURN, so its recover-path Unload can't free textures we uploaded
	// before panicking (the coverage assert below, or a sprite upload OOM).
	// Free the accumulated handles here and re-propagate so the abort stays
	// leak-safe — matching LoadResources' own recover discipline.
	defer func() {
		if rec := recover(); rec != nil {
			for _, t := range owned {
				rl.UnloadTexture(t)
			}
			panic(rec)
		}
	}()
	// Feral Rat: authored billboard PNG from maps/sprites. If the file is
	// missing or won't decode we fall back to the procedural makeRatPixels
	// art (the previous sprite, preserved below) so a checkout without the
	// asset still renders a rat and the coverage assert never trips.
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
	visuals = map[core.EnemyKind]enemyVisual{
		core.EnemyRat: {
			texture: ratTexture,
			// Reset to neutral defaults pending a new authored PNG: no contact
			// shadow, no y/depth offset, no tint or image adjustments — just the
			// sprite at a square 1×1 world size. Re-tune in the editor's Foe
			// Visualizer (which writes maps/sprites/visuals.json) once the new
			// art lands.
			size: rl.NewVector2(1.0, 1.0),
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
	// Overlay author-tuned overrides from maps/sprites/visuals.json on top of
	// the code defaults above (authored in the editor's Foe Visualizer). A
	// missing file is a clean no-op; a kind present in the file gets its
	// tunable fields replaced while keeping its code-default texture. Keyed by
	// core.EnemySlug so this loader and the editor agree on the key. A
	// malformed file is swallowed (defaults stand) rather than crashing the
	// whole game on boot over a bad tuning file.
	// Record each kind's PRISTINE base texture (the editor re-derives its FX
	// preview from this, never the adjusted display texture, so slider drags don't
	// compound). Done for every kind, not just overridden ones, so a freshly-edited
	// kind has a pristine base.
	for kind, v := range visuals {
		v.pristineTexture = v.texture
		visuals[kind] = v
	}
	if overrides, err := core.LoadEnemyVisualOverrides(); err == nil {
		for kind, v := range visuals {
			if ov, ok := overrides[core.EnemySlug(kind)]; ok {
				v = applyEnemyVisualOverride(v, ov)
				// Bake the non-destructive image adjustments (pixelate/brightness/
				// contrast) into the DISPLAY texture at load — point-sampled when
				// pixelated so it reads crisp, not blurred.
				if tex, ok := deriveAdjustedTexture(v.pristineTexture, ov, &owned); ok {
					v.texture = tex
				}
				visuals[kind] = v
			}
		}
	}
	return visuals, owned
}

// loadPartyVisuals builds the per-class party billboard table, mirroring
// loadEnemyVisuals: a code default (the uniform party billboard size, no shadow
// / offset / tint), an authored PNG from maps/sprites/<class-slug>.png when one
// exists (else the procedural makePartyPixels art), and an overlay of any author
// tuning from maps/sprites/partyvisuals.json. Keyed by PartyClass; the returned
// owned list owns the minted handles for Unload (partyVisuals aliases them, the
// same ownership split enemyVisuals/enemyTextures uses). Reuses the foe-side
// enemyVisual struct + apply helper since the party billboard shares the exact
// same alignment knobs (PartyVisualOverride is an alias of EnemyVisualOverride).
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
	// Overlay author-tuned overrides from maps/sprites/partyvisuals.json. Missing
	// file or class ⇒ the code default stands; a malformed file is swallowed
	// (defaults stand) rather than crashing boot — same discipline as the foe side.
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
		'·', // middle dot — editor hint-bar separators ("Enter edit · A add · …")
		'←', // editor pan / movement hints
		'↑', // turn-order / list cursor
		'→', // action arrows ("Attack → Rat")
		'↓', // turn-order / list cursor
		'↔', // bidirectional links (door pair)
		'∈', // set membership (future stat / skill notation)
		'−', // unicode minus (distinct from ASCII hyphen)
		'≈', // approximate (timing-grade hints)
		'≤', // less-than-or-equal (thresholds)
		'≥', // greater-than-or-equal (dialog "Gold ≥ amount" condition)
		'▲', // pack-edit reorder up, list arrows
		'▶', // action-row submenu indicator
		'▸', // small submenu chevron — pause-menu "Options ▸" / "Debug ▸" rows
		'◂', // small left chevron (cushion for ▸'s mirror)
		'▼', // pack-edit reorder down, list arrows
		'●', // bullet / active marker
		'★', // dialog start-node marker (editor node list)
		'✓', // dropdown active-toggle check (editor menus)
		'’', // right single quote — typographic apostrophe in editor menu copy ("foe's")
		'‘', // left single quote (cushion for its mirror)
	)
	return runes
}

// hudFontBake is the glyph-atlas bake size in pixels. Generous (128) so the
// five UI sizes (now 17/21/26/36/48) all DOWN-sample from a high-res master —
// Della Respira's fine serif strokes stay crisp at the small sizes instead
// of baking soft, and Title (48) keeps >2× headroom so it stays sharp on
// high-DPI displays. Mipmaps + trilinear (sharpenFontAtlas) then kill the
// minification jaggies/shimmer. Atlas memory cost is trivial (a single
// ~1–2 K² texture).
const hudFontBake = int32(128)

// loadHUDFont builds the single UI face from the embedded Della Respira
// TTF, baked once at hudFontBake over the full codepoint set. Embedding is
// the point — the typography is identical everywhere and can't go missing.
// The glyphs Della Respira omits (▶ → ↑ ↓ ● ★ ✓ ≤ ≥ … ’ ≈ …) are drawn
// PROCEDURALLY at the text layer (richtext.go), so the game ships exactly
// one font. The system-serif scan + raylib default remain only as a
// defensive fallback for the (practically impossible) case where the
// embedded bytes fail to load.
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

// sharpenFontAtlas applies mipmaps + trilinear filtering so the small UI
// sizes down-sample from the high-res atlas smoothly (sharp, not aliased)
// rather than shimmering under heavy minification — plain bilinear only
// samples the base level's 2×2 neighbourhood and jaggies. Mirrors the sky
// texture's mipmap+trilinear setup; the bake's glyph padding absorbs the
// minor cross-glyph bleed the lower mips introduce.
func sharpenFontAtlas(font *rl.Font) {
	rl.GenTextureMipmaps(&font.Texture)
	rl.SetTextureFilter(font.Texture, rl.FilterTrilinear)
}

// systemFontCandidates returns the per-OS serif paths used ONLY as a
// defensive fallback now that the UI face is the embedded Della Respira
// (loadHUDFont). They're tried in priority order if the embedded bytes
// somehow fail to load — readable serifs first (Constantia / Cambria /
// Georgia on Windows), the platform default sans last. Baked at hudFontBake
// like the primary so the five standard sizes still render sharp.
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

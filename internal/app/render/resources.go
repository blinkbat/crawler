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

	lighting lightingShader
	tree     treeModel

	// Field-only props. Boulders / bushes / mushrooms are scattered as
	// blockers (large) or procedural decorations (small/tiny) when the
	// active area is the field.
	rockProp     propModel
	bushProp     propModel
	mushroomProp propModel

	// Universal floor variants — keyed by their floor-layer char so the
	// renderer can swap in a cobblestone, plank, water, sand or snow tile
	// regardless of the area's material set. Built once on load and shared
	// across materials.
	specialFloors map[byte]rl.Model

	// New decor models keyed by decor-layer char (tall grass, flowers,
	// clover, reeds, bones, scorch, blood, cobweb, stump, log, leaf pile).
	// world.go checks this map before falling back to the existing
	// bush/mushroom/pebble auto-scatter cases.
	decorModels map[byte]propModel

	// New blocking props keyed by props-layer char (crate, barrel, urn,
	// stalagmite, pillar, broken pillar, statue, obelisk, fountain). Same
	// dispatch shape as decorModels — the renderer falls back to the
	// existing tree/boulder/bush cases when a char isn't here.
	propModels map[byte]propModel
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
	fieldMat.floorDirtModel = loadFloorModel(makeDirtPixels(128, 128), r.lighting.shader)
	fieldMat.floorDarkModel = loadFloorModel(makeDarkGrassPixels(128, 128), r.lighting.shader)
	fieldMat.hasFloorVariant = true
	r.materials[core.MaterialField] = fieldMat

	r.skyTexture = loadTexture(makeSkyPixels(1024, 512), 1024, 512, rl.FilterTrilinear)
	rl.GenTextureMipmaps(&r.skyTexture)
	rl.SetTextureFilter(r.skyTexture, rl.FilterTrilinear)
	rl.SetTextureWrap(r.skyTexture, rl.WrapClamp)

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

	// Universal floor variants — built once and shared across every material
	// set so a cobblestone path through a dungeon and one across a field
	// read identically. Initialize the map first so a panic mid-way still
	// unloads the variants that did land.
	r.specialFloors = make(map[byte]rl.Model)
	r.specialFloors[core.FloorCobble] = loadFloorModel(makeCobblePixels(128, 128), r.lighting.shader)
	r.specialFloors[core.FloorPlank] = loadFloorModel(makePlankPixels(128, 128), r.lighting.shader)
	r.specialFloors[core.FloorWater] = loadFloorModel(makeWaterPixels(128, 128), r.lighting.shader)
	r.specialFloors[core.FloorSand] = loadFloorModel(makeSandPixels(128, 128), r.lighting.shader)
	r.specialFloors[core.FloorSnow] = loadFloorModel(makeSnowPixels(128, 128), r.lighting.shader)

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
	// Archway uses marble palette to match the existing pillars/statues.
	archMarbleTex := loadTiledTexture(makeMarblePixels(128, 128))
	r.decorModels[core.DecorArchway] = loadArchwayDecor(r.lighting.shader, archMarbleTex)

	committed = true
	return r
}

func (r Resources) Unload() {
	// UnloadModel walks the model's materials and unloads each map's texture,
	// so wall/floor textures are freed here — no separate UnloadTexture call.
	for _, material := range r.materials {
		rl.UnloadModel(material.wallModel)
		rl.UnloadModel(material.floorModel)
		rl.UnloadModel(material.ceilingModel)
		if material.hasFloorVariant {
			rl.UnloadModel(material.floorDirtModel)
			rl.UnloadModel(material.floorDarkModel)
		}
	}
	rl.UnloadTexture(r.skyTexture)
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

// lightingFor picks the per-area lighting profile. Profiles are package-level
// constants in lighting.go — this just routes by material.
func lightingFor(material core.MaterialSet) lightingProfile {
	if material == core.MaterialDungeon {
		return dungeonLighting
	}
	return fieldLighting
}

func loadEnemyVisuals() (map[core.EnemyKind]enemyVisual, []rl.Texture2D) {
	ratTexture := loadTexture(makeRatPixels(72, 96), 72, 96, rl.FilterPoint)
	rl.SetTextureWrap(ratTexture, rl.WrapClamp)
	batTexture := loadTexture(makeBatPixels(80, 88), 80, 88, rl.FilterPoint)
	rl.SetTextureWrap(batTexture, rl.WrapClamp)
	diseasedRatTexture := loadTexture(makeDiseasedRatPixels(72, 96), 72, 96, rl.FilterPoint)
	rl.SetTextureWrap(diseasedRatTexture, rl.WrapClamp)
	owned := []rl.Texture2D{ratTexture, batTexture, diseasedRatTexture}
	// Placeholder textures for the new monster set — the procedural
	// pixel art for goblin / goblin mage / amoeba hasn't been authored
	// yet, so reuse existing sprites with size/aspect tweaks so the
	// silhouettes still read as distinct at distance:
	//   goblin     — rat texture, taller and beefier
	//   goblinMage — diseased-rat texture (the "bigger, scarier rat"
	//                already reads as a tier-up); replace once a
	//                dedicated mage sprite exists
	//   amoeba     — bat texture, squat and wide (the bat's "spread"
	//                silhouette doubles for a blob until a dedicated
	//                amoeba sprite is authored)
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
			texture: ratTexture,
			size:    rl.NewVector2(1.0, 1.5),
		},
		core.EnemyGoblinMage: {
			texture: diseasedRatTexture,
			size:    rl.NewVector2(1.05, 1.55),
		},
		core.EnemyAmoeba: {
			texture: batTexture,
			size:    rl.NewVector2(1.1, 0.9),
		},
	}
	return visuals, owned
}

func loadHUDFont() (rl.Font, bool) {
	// Try a per-OS list of well-known system font paths. First valid hit wins.
	// Fallback is raylib's bitmap default font, which has different metrics —
	// good for "ran on a server with no fonts," not great as a target.
	for _, path := range systemFontCandidates() {
		if _, err := os.Stat(path); err != nil {
			continue
		}
		font := rl.LoadFontEx(path, 32, nil)
		if rl.IsFontValid(font) {
			rl.SetTextureFilter(font.Texture, rl.FilterBilinear)
			return font, true
		}
	}
	return rl.GetFontDefault(), false
}

// systemFontCandidates returns the per-OS preferred-font paths in priority
// order. Picks "UI sans-serif" fonts for the body of the HUD on each platform.
func systemFontCandidates() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{
			`C:\Windows\Fonts\seguisb.ttf`,
			`C:\Windows\Fonts\segoeui.ttf`,
			`C:\Windows\Fonts\bahnschrift.ttf`,
			`C:\Windows\Fonts\consola.ttf`,
		}
	case "darwin":
		return []string{
			"/System/Library/Fonts/SFNS.ttf",
			"/System/Library/Fonts/SFNSDisplay.ttf",
			"/System/Library/Fonts/Helvetica.ttc",
			"/Library/Fonts/Arial.ttf",
		}
	default: // linux / *bsd / others — try the most common distro paths.
		return []string{
			"/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf",
			"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
			"/usr/share/fonts/TTF/DejaVuSans-Bold.ttf",
			"/usr/share/fonts/TTF/DejaVuSans.ttf",
			"/usr/share/fonts/truetype/liberation/LiberationSans-Bold.ttf",
			"/usr/share/fonts/truetype/liberation/LiberationSans-Regular.ttf",
			"/usr/share/fonts/noto/NotoSans-Regular.ttf",
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

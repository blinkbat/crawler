package render

import (
	"crawler/internal/app/core"
	rl "github.com/gen2brain/raylib-go/raylib"
	"image/color"
	"os"
	"runtime"
)

type Resources struct {
	materials    map[core.MaterialSet]worldMaterialResources
	skyTexture   rl.Texture2D
	enemyVisuals map[core.EnemyKind]enemyVisual
	partyTexture map[core.PartyClass]rl.Texture2D
	hudFont      rl.Font
	hudFontOwned bool

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
	wallModel  rl.Model
	floorModel rl.Model
	// Optional secondary floor variants for the field (dirt + dark grass).
	// Picked per-tile by hash so the field reads as varied terrain instead
	// of one uniform grass texture. Empty for the dungeon material.
	floorDirtModel  rl.Model
	floorDarkModel  rl.Model
	hasFloorVariant bool
}

func LoadResources() Resources {
	lighting := loadLightingShader()
	dungeonMat := loadWorldMaterial(makeStoneBrickPixels(128, 128), makeStoneFloorPixels(128, 128), lighting.shader)
	fieldMat := loadWorldMaterial(makeRockWallPixels(128, 128), makeGrassPixels(128, 128), lighting.shader)
	// Field gets two extra floor variants (dirt + dark grass), procedurally
	// chosen per tile by hash for terrain variation. Built using the same
	// path as the primary floor so they share filter / mipmap settings.
	fieldMat.floorDirtModel = loadFloorModel(makeDirtPixels(128, 128), lighting.shader)
	fieldMat.floorDarkModel = loadFloorModel(makeDarkGrassPixels(128, 128), lighting.shader)
	fieldMat.hasFloorVariant = true
	materials := map[core.MaterialSet]worldMaterialResources{
		core.MaterialDungeon: dungeonMat,
		core.MaterialField:   fieldMat,
	}
	skyTexture := loadTexture(makeSkyPixels(1024, 512), 1024, 512, rl.FilterTrilinear)
	rl.GenTextureMipmaps(&skyTexture)
	rl.SetTextureFilter(skyTexture, rl.FilterTrilinear)
	rl.SetTextureWrap(skyTexture, rl.WrapClamp)
	enemyVisuals := loadEnemyVisuals()
	partyTexture := make(map[core.PartyClass]rl.Texture2D)
	for _, def := range core.PartyClasses() {
		texture := loadTexture(makePartyPixels(64, 80, def.Class), 64, 80, rl.FilterPoint)
		rl.SetTextureWrap(texture, rl.WrapClamp)
		partyTexture[def.Class] = texture
	}
	hudFont, hudFontOwned := loadHUDFont()

	barkTex := loadTexture(makeBarkPixels(64, 128), 64, 128, rl.FilterBilinear)
	rl.SetTextureWrap(barkTex, rl.WrapRepeat)
	leafTex := loadTexture(makeLeafPixels(96, 96), 96, 96, rl.FilterBilinear)
	rl.SetTextureWrap(leafTex, rl.WrapRepeat)
	tree := loadTreeModel(lighting.shader, barkTex, leafTex)

	// Field props get their own texture instances so the prop models own
	// them outright (UnloadModel handles the texture). Sharing would either
	// double-unload or require external ownership tracking.
	rockTex := loadTexture(makeRockWallPixels(128, 128), 128, 128, rl.FilterBilinear)
	rl.GenTextureMipmaps(&rockTex)
	rl.SetTextureFilter(rockTex, rl.FilterTrilinear)
	rl.SetTextureWrap(rockTex, rl.WrapRepeat)
	bushTex := loadTexture(makeLeafPixels(96, 96), 96, 96, rl.FilterBilinear)
	rl.SetTextureWrap(bushTex, rl.WrapRepeat)

	rockProp := loadRockProp(lighting.shader, rockTex)
	bushProp := loadBushProp(lighting.shader, bushTex)
	mushroomProp := loadMushroomProp(lighting.shader)

	// Universal floor variants — built once and shared across every material
	// set so a cobblestone path through a dungeon and one across a field
	// read identically.
	specialFloors := map[byte]rl.Model{
		core.FloorCobble: loadFloorModel(makeCobblePixels(128, 128), lighting.shader),
		core.FloorPlank:  loadFloorModel(makePlankPixels(128, 128), lighting.shader),
		core.FloorWater:  loadFloorModel(makeWaterPixels(128, 128), lighting.shader),
		core.FloorSand:   loadFloorModel(makeSandPixels(128, 128), lighting.shader),
		core.FloorSnow:   loadFloorModel(makeSnowPixels(128, 128), lighting.shader),
	}

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
	//
	// Bark is authored at 64x128 and leaf at 96x96 (matches the existing
	// tree pipeline). loadTiledTexture is fixed to 128x128 so we call
	// loadTexture directly here with the right dimensions, then opt into
	// the same mipmap / repeat-wrap settings.
	crateWoodTex := loadRepeatTexture(makeBarkPixels(64, 128), 64, 128)
	barrelWoodTex := loadRepeatTexture(makeBarkPixels(64, 128), 64, 128)
	stumpBarkTex := loadRepeatTexture(makeBarkPixels(64, 128), 64, 128)
	logBarkTex := loadRepeatTexture(makeBarkPixels(64, 128), 64, 128)
	logMossTex := loadRepeatTexture(makeLeafPixels(96, 96), 96, 96)
	leafPileTex := loadRepeatTexture(makeLeafPixels(96, 96), 96, 96)

	propModels := map[byte]propModel{
		core.TileCrate:        loadCrateProp(lighting.shader, crateWoodTex),
		core.TileBarrel:       loadBarrelProp(lighting.shader, barrelWoodTex),
		core.TileUrn:          loadUrnProp(lighting.shader, terracottaTex),
		core.TileStalagmite:   loadStalagmiteProp(lighting.shader, marbleTex),
		core.TilePillar:       loadPillarProp(lighting.shader, marbleTex),
		core.TileBrokenPillar: loadBrokenPillarProp(lighting.shader, marbleTex),
		core.TileStatue:       loadStatueProp(lighting.shader, marbleTex),
		core.TileObelisk:      loadObeliskProp(lighting.shader, graniteTex),
		core.TileFountain:     loadFountainProp(lighting.shader, marbleTex),
	}

	decorModels := map[byte]propModel{
		core.DecorTallGrass: loadTallGrassProp(lighting.shader),
		core.DecorFlowers:   loadFlowerProp(lighting.shader),
		core.DecorClover:    loadCloverProp(lighting.shader),
		core.DecorReeds:     loadReedProp(lighting.shader),
		core.DecorBones:     loadBoneProp(lighting.shader),
		core.DecorScorch:    loadScorchProp(lighting.shader),
		core.DecorBlood:     loadBloodProp(lighting.shader),
		core.DecorCobweb:    loadCobwebProp(lighting.shader),
		core.DecorStump:     loadStumpProp(lighting.shader, stumpBarkTex),
		core.DecorLog:       loadLogProp(lighting.shader, logBarkTex, logMossTex),
		core.DecorLeafPile:  loadLeafPileProp(lighting.shader, leafPileTex),
	}

	return Resources{
		materials:     materials,
		skyTexture:    skyTexture,
		enemyVisuals:  enemyVisuals,
		partyTexture:  partyTexture,
		hudFont:       hudFont,
		hudFontOwned:  hudFontOwned,
		lighting:      lighting,
		tree:          tree,
		rockProp:      rockProp,
		bushProp:      bushProp,
		mushroomProp:  mushroomProp,
		specialFloors: specialFloors,
		decorModels:   decorModels,
		propModels:    propModels,
	}
}


func (r Resources) Unload() {
	// UnloadModel walks the model's materials and unloads each map's texture,
	// so wall/floor textures are freed here — no separate UnloadTexture call.
	for _, material := range r.materials {
		rl.UnloadModel(material.wallModel)
		rl.UnloadModel(material.floorModel)
		if material.hasFloorVariant {
			rl.UnloadModel(material.floorDirtModel)
			rl.UnloadModel(material.floorDarkModel)
		}
	}
	rl.UnloadTexture(r.skyTexture)
	for _, visual := range r.enemyVisuals {
		rl.UnloadTexture(visual.texture)
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

func loadEnemyVisuals() map[core.EnemyKind]enemyVisual {
	ratTexture := loadTexture(makeRatPixels(72, 96), 72, 96, rl.FilterPoint)
	rl.SetTextureWrap(ratTexture, rl.WrapClamp)
	batTexture := loadTexture(makeBatPixels(80, 88), 80, 88, rl.FilterPoint)
	rl.SetTextureWrap(batTexture, rl.WrapClamp)
	diseasedRatTexture := loadTexture(makeDiseasedRatPixels(72, 96), 72, 96, rl.FilterPoint)
	rl.SetTextureWrap(diseasedRatTexture, rl.WrapClamp)
	return map[core.EnemyKind]enemyVisual{
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
	}
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

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

	return Resources{
		materials:    materials,
		skyTexture:   skyTexture,
		enemyVisuals: enemyVisuals,
		partyTexture: partyTexture,
		hudFont:      hudFont,
		hudFontOwned: hudFontOwned,
		lighting:     lighting,
		tree:         tree,
		rockProp:     rockProp,
		bushProp:     bushProp,
		mushroomProp: mushroomProp,
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
	r.lighting.unload()
	if r.hudFontOwned {
		rl.UnloadFont(r.hudFont)
	}
}

// loadFloorModel builds a single tinted floor cube model with mipmapped
// trilinear filtering. Used for the primary floor and the alt variants
// (dirt / dark grass) on the field.
func loadFloorModel(pixels []color.RGBA, shader rl.Shader) rl.Model {
	tex := loadTexture(pixels, 128, 128, rl.FilterBilinear)
	rl.GenTextureMipmaps(&tex)
	rl.SetTextureFilter(tex, rl.FilterTrilinear)
	rl.SetTextureWrap(tex, rl.WrapRepeat)
	model := rl.LoadModelFromMesh(rl.GenMeshCube(core.TileSize, 0.06, core.TileSize))
	setModelTexture(&model, tex)
	attachShader(&model, shader)
	return model
}

func loadWorldMaterial(wallPixels, floorPixels []color.RGBA, shader rl.Shader) worldMaterialResources {
	wallTexture := loadTexture(wallPixels, 128, 128, rl.FilterBilinear)
	rl.GenTextureMipmaps(&wallTexture)
	rl.SetTextureFilter(wallTexture, rl.FilterTrilinear)
	rl.SetTextureWrap(wallTexture, rl.WrapRepeat)
	floorTexture := loadTexture(floorPixels, 128, 128, rl.FilterBilinear)
	rl.GenTextureMipmaps(&floorTexture)
	rl.SetTextureFilter(floorTexture, rl.FilterTrilinear)
	rl.SetTextureWrap(floorTexture, rl.WrapRepeat)
	wallModel := rl.LoadModelFromMesh(rl.GenMeshCube(core.TileSize, core.WallHeight, core.TileSize))
	floorModel := rl.LoadModelFromMesh(rl.GenMeshCube(core.TileSize, 0.06, core.TileSize))
	setModelTexture(&wallModel, wallTexture)
	setModelTexture(&floorModel, floorTexture)
	attachShader(&wallModel, shader)
	attachShader(&floorModel, shader)
	return worldMaterialResources{
		wallModel:  wallModel,
		floorModel: floorModel,
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

func drawHUDText(font rl.Font, text string, x, y int32, size float32, col color.RGBA) {
	pos := rl.NewVector2(float32(x), float32(y))
	shadow := rl.NewVector2(float32(x)+2, float32(y)+2)
	rl.DrawTextEx(font, text, shadow, size, 1, rl.NewColor(0, 0, 0, 190))
	rl.DrawTextEx(font, text, pos, size, 1, col)
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

package render

import (
	"crawler/internal/app/core"
	rl "github.com/gen2brain/raylib-go/raylib"
	"image/color"
	"os"
)

type Resources struct {
	wallTexture  rl.Texture2D
	floorTexture rl.Texture2D
	skyTexture   rl.Texture2D
	ratTexture   rl.Texture2D
	partyTexture map[core.PartyClass]rl.Texture2D
	wallModel    rl.Model
	floorModel   rl.Model
	hudFont      rl.Font
	hudFontOwned bool
}

func LoadResources() Resources {
	wallTexture := loadTexture(makeStoneBrickPixels(128, 128), 128, 128, rl.FilterPoint)
	floorTexture := loadTexture(makeStoneFloorPixels(128, 128), 128, 128, rl.FilterPoint)
	skyTexture := loadTexture(makeSkyPixels(1024, 512), 1024, 512, rl.FilterTrilinear)
	rl.GenTextureMipmaps(&skyTexture)
	rl.SetTextureFilter(skyTexture, rl.FilterTrilinear)
	rl.SetTextureWrap(skyTexture, rl.WrapClamp)
	ratTexture := loadTexture(makeRatPixels(72, 96), 72, 96, rl.FilterPoint)
	rl.SetTextureWrap(ratTexture, rl.WrapClamp)
	partyTexture := make(map[core.PartyClass]rl.Texture2D)
	for _, def := range core.PartyClasses() {
		texture := loadTexture(makePartyPixels(64, 80, def.Class), 64, 80, rl.FilterPoint)
		rl.SetTextureWrap(texture, rl.WrapClamp)
		partyTexture[def.Class] = texture
	}
	hudFont, hudFontOwned := loadHUDFont()

	wallModel := rl.LoadModelFromMesh(rl.GenMeshCube(core.TileSize, core.WallHeight, core.TileSize))
	floorModel := rl.LoadModelFromMesh(rl.GenMeshCube(core.TileSize, 0.06, core.TileSize))

	setModelTexture(&wallModel, wallTexture)
	setModelTexture(&floorModel, floorTexture)

	return Resources{
		wallTexture:  wallTexture,
		floorTexture: floorTexture,
		skyTexture:   skyTexture,
		ratTexture:   ratTexture,
		partyTexture: partyTexture,
		wallModel:    wallModel,
		floorModel:   floorModel,
		hudFont:      hudFont,
		hudFontOwned: hudFontOwned,
	}
}

func (r Resources) Unload() {
	rl.UnloadModel(r.wallModel)
	rl.UnloadModel(r.floorModel)
	rl.UnloadTexture(r.wallTexture)
	rl.UnloadTexture(r.floorTexture)
	rl.UnloadTexture(r.skyTexture)
	rl.UnloadTexture(r.ratTexture)
	for _, texture := range r.partyTexture {
		rl.UnloadTexture(texture)
	}
	if r.hudFontOwned {
		rl.UnloadFont(r.hudFont)
	}
}

func loadHUDFont() (rl.Font, bool) {
	for _, path := range []string{
		`C:\Windows\Fonts\seguisb.ttf`,
		`C:\Windows\Fonts\segoeui.ttf`,
		`C:\Windows\Fonts\bahnschrift.ttf`,
		`C:\Windows\Fonts\consola.ttf`,
	} {
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

func drawHUDText(font rl.Font, text string, x, y int32, size float32) {
	drawHUDTextColor(font, text, x, y, size, rl.RayWhite)
}

func drawHUDTextColor(font rl.Font, text string, x, y int32, size float32, col color.RGBA) {
	pos := rl.NewVector2(float32(x), float32(y))
	shadow := rl.NewVector2(float32(x)+2, float32(y)+2)
	rl.DrawTextEx(font, text, shadow, size, 1, rl.NewColor(0, 0, 0, 190))
	rl.DrawTextEx(font, text, pos, size, 1, col)
}

func drawRoundedRect(x, y, w, h int32, roundness float32, col color.RGBA) {
	if w <= 0 || h <= 0 {
		return
	}
	rl.DrawRectangleRounded(rl.NewRectangle(float32(x), float32(y), float32(w), float32(h)), roundness, 8, col)
}

func drawRoundedRectLines(x, y, w, h int32, roundness float32, col color.RGBA) {
	if w <= 0 || h <= 0 {
		return
	}
	rl.DrawRectangleRoundedLinesEx(rl.NewRectangle(float32(x), float32(y), float32(w), float32(h)), roundness, 8, 1, col)
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
